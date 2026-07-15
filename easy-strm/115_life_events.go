package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"easy-strm/internal/service"
	driver "github.com/SheltonZhu/115driver/pkg/driver"
)

// GetLifeEvents 拉取 115 生活事件流。
// 参考 p115client 的 life_behavior_detail_app，使用 Android 端事件明细接口。
func (c *Client) GetLifeEvents(offset int, limit int, behaviorType string, date string, cloud115ID int, cookie string) (*service.Cloud115LifeEventResp, error) {
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	if offset < 0 {
		offset = 0
	}

	endpoint, err := url.Parse("https://proapi.115.com/android/behavior/detail")
	if err != nil {
		return nil, err
	}
	query := endpoint.Query()
	query.Set("offset", strconv.Itoa(offset))
	query.Set("limit", strconv.Itoa(limit))
	if strings.TrimSpace(behaviorType) != "" {
		query.Set("type", strings.TrimSpace(behaviorType))
	}
	if strings.TrimSpace(date) != "" {
		query.Set("date", strings.TrimSpace(date))
	}
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequest(http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cookie", cookie)
	req.Header.Set("User-Agent", driver.UA115Disk)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Referer", "https://115.com/")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request 115 life events failed: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read 115 life events failed: %v", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("115 life events status=%d body=%s", resp.StatusCode, string(body))
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse 115 life events failed: %v", err)
	}
	if !cloud115ResponseOK(raw) {
		return nil, fmt.Errorf("115 life events returned error: %v", raw)
	}

	data, _ := raw["data"].(map[string]interface{})
	if data == nil {
		return nil, fmt.Errorf("115 life events response missing data: %v", raw)
	}
	result := &service.Cloud115LifeEventResp{
		Count:    int(jsonNumberToInt64(data["count"])),
		NextPage: jsonBool(data["next_page"]),
		Raw:      raw,
	}
	rawList, _ := data["list"].([]interface{})
	result.Events = make([]service.Cloud115LifeEvent, 0, len(rawList))
	for _, entry := range rawList {
		eventMap, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		event := service.Cloud115LifeEvent{
			ID:          jsonNumberToInt64(eventMap["id"]),
			UpdateTime:  jsonNumberToInt64(eventMap["update_time"]),
			Type:        int(jsonNumberToInt64(eventMap["type"])),
			EventName:   jsonString(eventMap["event_name"]),
			FileID:      firstJSONText(eventMap, "file_id", "fid", "cid"),
			PickCode:    firstJSONText(eventMap, "pick_code", "pickcode", "pc"),
			ParentID:    firstJSONText(eventMap, "parent_id", "pid", "category_id", "cid"),
			FileName:    firstJSONText(eventMap, "file_name", "name", "title"),
			IsDirectory: jsonBool(eventMap["is_dir"]) || jsonBool(eventMap["is_directory"]),
			Raw:         eventMap,
		}
		if event.EventName == "" {
			event.EventName = cloud115LifeEventName(event.Type)
		}
		result.Events = append(result.Events, event)
	}
	return result, nil
}

func cloud115ResponseOK(raw map[string]interface{}) bool {
	if raw == nil {
		return false
	}
	if state, ok := raw["state"]; ok {
		switch v := state.(type) {
		case bool:
			return v
		case float64:
			return v != 0
		case string:
			return v == "true" || v == "1"
		}
	}
	return true
}

func jsonNumberToInt64(value interface{}) int64 {
	switch v := value.(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	case json.Number:
		n, _ := v.Int64()
		return n
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return n
	default:
		return 0
	}
}

func jsonBool(value interface{}) bool {
	switch v := value.(type) {
	case bool:
		return v
	case float64:
		return v != 0
	case int:
		return v != 0
	case string:
		text := strings.ToLower(strings.TrimSpace(v))
		return text == "true" || text == "1" || text == "yes"
	default:
		return false
	}
}

func jsonString(value interface{}) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return strconv.FormatInt(int64(v), 10)
	case int64:
		return strconv.FormatInt(v, 10)
	case int:
		return strconv.Itoa(v)
	case json.Number:
		return v.String()
	default:
		return ""
	}
}

func firstJSONText(raw map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value := jsonString(raw[key]); value != "" {
			return value
		}
	}
	return ""
}

func cloud115LifeEventName(eventType int) string {
	names := map[int]string{
		1:  "upload_image_file",
		2:  "upload_file",
		3:  "star_image",
		4:  "star_file",
		5:  "move_image_file",
		6:  "move_file",
		7:  "browse_image",
		8:  "browse_video",
		9:  "browse_audio",
		10: "browse_document",
		14: "receive_files",
		17: "new_folder",
		18: "copy_folder",
		19: "folder_label",
		20: "folder_rename",
		22: "delete_file",
		23: "copy_file",
		24: "file_rename",
	}
	return names[eventType]
}
