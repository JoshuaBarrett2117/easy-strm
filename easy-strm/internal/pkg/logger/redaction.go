package logger

import (
	"encoding/json"
	"math"
	"reflect"
	"regexp"
	"strings"
	"time"
	"unicode"
)

const redacted = "[REDACTED]"

var urlPattern = regexp.MustCompile(`(?i)\b(?:https?|ftp)://[^\s<>"']+`)
var queryPattern = regexp.MustCompile(`\?[^\s<>"']+`)
var headerPattern = regexp.MustCompile(`(?im)\b(?:authorization|proxy[-_ ]authorization|cookie|set[-_ ]cookie)\s*[:=]\s*[^\r\n]+`)
var identityPattern = regexp.MustCompile(`(?i)\b(?:user|username|account|password)\s+(?:"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*')`)
var payloadPattern = regexp.MustCompile(`(?is)\b(?:body|raw|payload)\s*[:=]\s*.*`)
var credentialPattern = regexp.MustCompile(`(?i)["']?\b(?:[a-z0-9_.-]*(?:api[-_. ]?key|token|password|passwd|secret|user[-_. ]?name|account|credential|signature|authorization|cookie)[a-z0-9_.-]*|pwd|user|auth|sign|receive[-_. ]?code)["']?\s*[:=]\s*(?:"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|[^\s,;&}\]]+)`)

func sanitizeText(text string) string {
	text = urlPattern.ReplaceAllString(text, "[REDACTED_URL]")
	text = queryPattern.ReplaceAllString(text, "?[REDACTED_QUERY]")
	text = headerPattern.ReplaceAllString(text, "[REDACTED_HEADER]")
	text = identityPattern.ReplaceAllString(text, "[REDACTED_IDENTITY]")
	text = payloadPattern.ReplaceAllString(text, "[REDACTED_PAYLOAD]")
	return credentialPattern.ReplaceAllStringFunc(text, func(match string) string {
		separator := strings.IndexAny(match, ":=")
		return match[:separator+1] + redacted
	})
}

func sensitiveKey(key string) bool {
	canonical := strings.Map(func(character rune) rune {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			return unicode.ToLower(character)
		}
		return -1
	}, key)
	for _, fragment := range []string{"apikey", "cookie", "account", "username", "token", "password", "passwd", "authorization", "secret", "credential", "signature", "receivecode", "pickcode", "header", "body", "payload"} {
		if strings.Contains(canonical, fragment) {
			return true
		}
	}
	return canonical == "user" || canonical == "pwd" || canonical == "auth" || canonical == "sign" || canonical == "密码" || canonical == "账号"
}

func sanitizeValue(value interface{}) interface{} {
	return sanitizeReflect(reflect.ValueOf(value), 0)
}

func sanitizeReflect(value reflect.Value, depth int) interface{} {
	if !value.IsValid() {
		return nil
	}
	if depth >= 16 {
		return "[TRUNCATED]"
	}
	if (value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface) && value.IsNil() {
		return nil
	}
	if value.CanInterface() {
		switch item := value.Interface().(type) {
		case time.Time:
			return item.Format(TimestampLayout)
		case error:
			return sanitizeText(item.Error())
		case json.RawMessage:
			var decoded interface{}
			if json.Unmarshal(item, &decoded) != nil {
				return "[OMITTED_BYTES]"
			}
			return sanitizeReflect(reflect.ValueOf(decoded), depth+1)
		}
	}
	if value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface {
		return sanitizeReflect(value.Elem(), depth+1)
	}
	switch value.Kind() {
	case reflect.String:
		return sanitizeText(value.String())
	case reflect.Bool:
		return value.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return value.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return value.Uint()
	case reflect.Float32, reflect.Float64:
		if math.IsNaN(value.Float()) || math.IsInf(value.Float(), 0) {
			return "[NON_FINITE]"
		}
		return value.Float()
	case reflect.Map:
		if value.Type().Key().Kind() != reflect.String {
			return "[OMITTED_MAP]"
		}
		result := map[string]interface{}{}
		iterator := value.MapRange()
		for iterator.Next() {
			key := iterator.Key().String()
			if sensitiveKey(key) {
				result[sanitizeText(key)] = redacted
			} else {
				result[sanitizeText(key)] = sanitizeReflect(iterator.Value(), depth+1)
			}
		}
		return result
	case reflect.Slice, reflect.Array:
		if value.Type().Elem().Kind() == reflect.Uint8 {
			return "[OMITTED_BYTES]"
		}
		result := make([]interface{}, value.Len())
		for index := 0; index < value.Len(); index++ {
			result[index] = sanitizeReflect(value.Index(index), depth+1)
		}
		return result
	case reflect.Struct:
		result := map[string]interface{}{}
		for index := 0; index < value.NumField(); index++ {
			field := value.Type().Field(index)
			if !field.IsExported() {
				continue
			}
			key := strings.Split(field.Tag.Get("json"), ",")[0]
			if key == "-" {
				continue
			}
			if key == "" {
				key = field.Name
			}
			if sensitiveKey(key) || sensitiveKey(field.Name) {
				result[sanitizeText(key)] = redacted
			} else {
				result[sanitizeText(key)] = sanitizeReflect(value.Field(index), depth+1)
			}
		}
		return result
	default:
		return "[UNSUPPORTED]"
	}
}
