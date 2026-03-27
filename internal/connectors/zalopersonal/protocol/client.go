package protocol

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

func generateSignKey(typeStr string, params map[string]any) string {
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString("zsecure" + typeStr)
	for _, key := range keys {
		if value := params[key]; value != nil {
			b.WriteString(convertToString(value))
		}
	}
	sum := md5.Sum([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

func convertToString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case int:
		return strconv.Itoa(v)
	case int8, int16, int32, int64:
		return strconv.FormatInt(reflect.ValueOf(v).Int(), 10)
	case uint, uint8, uint16, uint32, uint64:
		return strconv.FormatUint(reflect.ValueOf(v).Uint(), 10)
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	default:
		return fmt.Sprint(v)
	}
}

func defaultHeaders(sess *Session) http.Header {
	headers := http.Header{}
	headers.Set("Accept", "application/json, text/plain, */*")
	headers.Set("Accept-Language", sess.Language)
	headers.Set("Origin", DefaultBaseURL.String())
	headers.Set("Referer", DefaultBaseURL.String()+"/")
	headers.Set("User-Agent", sess.UserAgent)
	if strings.TrimSpace(sess.Cookie) != "" {
		headers.Set("Cookie", sess.Cookie)
	}
	return headers
}
