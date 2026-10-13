package service

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"easy-strm/internal/pkg/logger"
)

const embyDirectPlayBodyLimit = 8 << 20

type embyDirectPlayReplayBody struct {
	io.Reader
	io.Closer
}

func forceEmbyDirectPlay(request *http.Request) {
	itemID := embyDirectPlayItemID(request)
	if itemID == "" {
		return
	}
	log := logger.WithContext(request.Context(), "emby_proxy")
	warn := func(reason string) {
		log.Log(logger.WARN, "强制直接播放未改写，保留原始请求", logger.Fields{"item_id": itemID, "reason": reason}, nil)
	}
	for _, encoding := range request.Header.Values("Content-Encoding") {
		if encoding != "" && !strings.EqualFold(strings.TrimSpace(encoding), "identity") {
			warn("encoded_body")
			return
		}
	}
	if request.Body == nil || request.Body == http.NoBody {
		warn("empty_body")
		return
	}
	if request.ContentLength > embyDirectPlayBodyLimit {
		warn("body_too_large")
		return
	}
	original := request.Body
	body, err := readEmbyDirectPlayBody(original)
	request.Body = &embyDirectPlayReplayBody{Reader: io.MultiReader(bytes.NewReader(body), original), Closer: original}
	if err != nil {
		warn("body_read_failed")
		return
	}
	if len(body) > embyDirectPlayBodyLimit {
		warn("body_too_large")
		return
	}
	rewritten, changed, reason := rewriteEmbyDirectPlayJSON(body)
	if reason != "" {
		warn(reason)
		return
	}
	if err := original.Close(); err != nil {
		log.Log(logger.WARN, "强制直接播放已改写，原始请求体关闭失败", logger.Fields{"item_id": itemID, "reason": "body_close_failed"}, nil)
	}
	request.Body = io.NopCloser(bytes.NewReader(rewritten))
	request.ContentLength = int64(len(rewritten))
	request.Header.Set("Content-Length", strconv.Itoa(len(rewritten)))
	request.TransferEncoding = nil
	request.Header.Del("Transfer-Encoding")
	if request.GetBody != nil {
		request.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(rewritten)), nil
		}
	}
	log.Log(logger.INFO, "强制直接播放已改写客户端能力", logger.Fields{"item_id": itemID, "changed_fields": changed}, nil)
}

func embyDirectPlayItemID(request *http.Request) string {
	if request.Method != http.MethodPost {
		return ""
	}
	parts := strings.Split(request.URL.Path, "/")
	if len(parts) < 4 {
		return ""
	}
	tail := parts[len(parts)-3:]
	if !strings.EqualFold(tail[0], "Items") || tail[1] == "" || !strings.EqualFold(tail[2], "PlaybackInfo") {
		return ""
	}
	return tail[1]
}

func readEmbyDirectPlayBody(reader io.Reader) ([]byte, error) {
	buffer := make([]byte, 32<<10)
	used, emptyReads := 0, 0
	for {
		count, err := reader.Read(buffer[used:])
		used += count
		if err != nil {
			if err == io.EOF {
				return buffer[:used], nil
			}
			return buffer[:used], err
		}
		if count == 0 {
			emptyReads++
			if emptyReads >= 100 {
				return buffer[:used], io.ErrNoProgress
			}
		} else {
			emptyReads = 0
		}
		if used == len(buffer) {
			if used == embyDirectPlayBodyLimit+1 {
				return buffer, nil
			}
			expanded := make([]byte, min(len(buffer)*2, embyDirectPlayBodyLimit+1))
			copy(expanded, buffer)
			buffer = expanded
		}
	}
}

func rewriteEmbyDirectPlayJSON(body []byte) ([]byte, []string, string) {
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, nil, "empty_body"
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil || root == nil {
		return nil, nil, "invalid_json"
	}
	rawProfile, exists := root["DeviceProfile"]
	if !exists {
		return nil, nil, "missing_device_profile"
	}
	var profile map[string]json.RawMessage
	if err := json.Unmarshal(rawProfile, &profile); err != nil || profile == nil {
		return nil, nil, "invalid_device_profile"
	}
	fields := []struct {
		name, value string
	}{
		{"EnableTranscoding", "false"},
		{"TranscodingProfiles", "[]"},
		{"EnableDirectPlay", "true"},
		{"EnableDirectStream", "true"},
	}
	changed := make([]string, 0, len(fields)+1)
	for _, field := range fields {
		value, present := profile[field.name]
		value = bytes.TrimSpace(value)
		if present {
			if field.name == "TranscodingProfiles" {
				if len(value) == 0 || value[0] != '[' {
					return nil, nil, "invalid_field_type:" + field.name
				}
				if len(bytes.TrimSpace(value[1:len(value)-1])) == 0 {
					continue
				}
			} else {
				if !bytes.Equal(value, []byte("true")) && !bytes.Equal(value, []byte("false")) {
					return nil, nil, "invalid_field_type:" + field.name
				}
				if string(value) == field.value {
					continue
				}
			}
		}
		profile[field.name] = json.RawMessage(field.value)
		changed = append(changed, field.name)
	}
	bitrate, present := profile["MaxStreamingBitrate"]
	bitrate = bytes.TrimSpace(bitrate)
	if present && (len(bitrate) == 0 || (bitrate[0] != '-' && (bitrate[0] < '0' || bitrate[0] > '9'))) {
		return nil, nil, "invalid_field_type:MaxStreamingBitrate"
	}
	if !present || embyDirectPlayBitrateBelowMinimum(string(bitrate)) {
		profile["MaxStreamingBitrate"] = json.RawMessage("200000000")
		changed = append(changed, "MaxStreamingBitrate")
	}
	encodedProfile, err := json.Marshal(profile)
	if err != nil {
		return nil, nil, "json_encode_failed"
	}
	root["DeviceProfile"] = encodedProfile
	rewritten, err := json.Marshal(root)
	if err != nil {
		return nil, nil, "json_encode_failed"
	}
	return rewritten, changed, ""
}

func embyDirectPlayBitrateBelowMinimum(number string) bool {
	if strings.HasPrefix(number, "-") {
		return true
	}
	mantissa, exponent := number, int64(0)
	if position := strings.IndexAny(number, "eE"); position >= 0 {
		mantissa = number[:position]
		parsed, err := strconv.ParseInt(number[position+1:], 10, 64)
		if err != nil {
			return strings.HasPrefix(number[position+1:], "-") || strings.Trim(mantissa, "0.") == ""
		}
		exponent = parsed
	}
	integerDigits := len(mantissa)
	if position := strings.IndexByte(mantissa, '.'); position >= 0 {
		integerDigits = position
	}
	digits := strings.ReplaceAll(mantissa, ".", "")
	significant := strings.TrimLeft(digits, "0")
	if significant == "" {
		return true
	}
	minimumExponent := int64(9 - integerDigits + len(digits) - len(significant))
	if exponent != minimumExponent {
		return exponent < minimumExponent
	}
	return significant[0] < '2'
}
