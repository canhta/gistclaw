package protocol

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
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

func makeURL(_ *Session, baseURL string, params map[string]any, includeDefaults bool) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	query := u.Query()
	for key, value := range params {
		if !query.Has(key) {
			query.Set(key, convertToString(value))
		}
	}
	if includeDefaults {
		if !query.Has("zpw_ver") {
			query.Set("zpw_ver", convertToString(DefaultAPIVersion))
		}
		if !query.Has("zpw_type") {
			query.Set("zpw_type", convertToString(DefaultAPIType))
		}
	}
	u.RawQuery = query.Encode()
	return u.String()
}

func buildFormBody(data map[string]string) *strings.Reader {
	form := url.Values{}
	for key, value := range data {
		form.Set(key, value)
	}
	return strings.NewReader(form.Encode())
}

func getEncryptParam(sess *Session, typeStr string) (params map[string]any, enk *string, err error) {
	data := map[string]any{
		"computer_name": DefaultComputerName,
		"imei":          sess.IMEI,
		"language":      sess.Language,
		"ts":            time.Now().UnixMilli(),
	}

	zcid, zcidExt, encKey, err := encryptParams(sess.IMEI, data)
	if err != nil {
		return nil, nil, err
	}

	params = map[string]any{
		"zcid":           zcid,
		"enc_ver":        DefaultEncryptVer,
		"zcid_ext":       zcidExt,
		"params":         encKey.encData,
		"type":           DefaultAPIType,
		"client_version": DefaultAPIVersion,
	}

	if typeStr == "getserverinfo" {
		params["signkey"] = generateSignKey(typeStr, map[string]any{
			"imei":           sess.IMEI,
			"type":           DefaultAPIType,
			"client_version": DefaultAPIVersion,
			"computer_name":  DefaultComputerName,
		})
	} else {
		params["signkey"] = generateSignKey(typeStr, params)
	}

	return params, &encKey.key, nil
}

type encryptResult struct {
	key     string
	encData string
}

func encryptParams(imei string, data map[string]any) (zcid, zcidExt string, result *encryptResult, err error) {
	ts := time.Now().UnixMilli()
	zcidExt = randomHexString(6, 12)

	zcidData := fmt.Sprintf("%d,%s,%d", DefaultAPIType, imei, ts)
	zcidRaw, err := EncodeAESCBC([]byte(DefaultZCIDKey), zcidData, true)
	if err != nil {
		return "", "", nil, fmt.Errorf("zalo personal protocol: create zcid: %w", err)
	}
	zcid = strings.ToUpper(zcidRaw)

	encKey, err := deriveEncryptKey(zcidExt, zcid)
	if err != nil {
		return "", "", nil, fmt.Errorf("zalo personal protocol: derive key: %w", err)
	}

	blob, err := json.Marshal(data)
	if err != nil {
		return "", "", nil, err
	}
	encData, err := EncodeAESCBC([]byte(encKey), string(blob), false)
	if err != nil {
		return "", "", nil, fmt.Errorf("zalo personal protocol: encrypt data: %w", err)
	}

	return zcid, zcidExt, &encryptResult{key: encKey, encData: encData}, nil
}

func deriveEncryptKey(ext, id string) (string, error) {
	sum := md5.Sum([]byte(ext))
	nUpper := strings.ToUpper(hex.EncodeToString(sum[:]))

	evenE, _ := processStr(nUpper)
	evenI, oddI := processStr(id)
	if len(evenE) == 0 || len(evenI) == 0 || len(oddI) == 0 {
		return "", fmt.Errorf("zalo personal protocol: invalid key derivation params")
	}

	var b strings.Builder
	b.WriteString(joinFirst(evenE, 8))
	b.WriteString(joinFirst(evenI, 12))
	b.WriteString(joinFirst(reverseCopy(oddI), 12))
	return b.String(), nil
}

func processStr(s string) (even, odd []string) {
	for i, r := range s {
		if i%2 == 0 {
			even = append(even, string(r))
		} else {
			odd = append(odd, string(r))
		}
	}
	return even, odd
}

func joinFirst(parts []string, n int) string {
	if n > len(parts) {
		n = len(parts)
	}
	return strings.Join(parts[:n], "")
}

func reverseCopy(input []string) []string {
	output := make([]string, len(input))
	copy(output, input)
	for i, j := 0, len(output)-1; i < j; i, j = i+1, j-1 {
		output[i], output[j] = output[j], output[i]
	}
	return output
}

func randomHexString(minLen, maxLen int) string {
	length := minLen
	if maxLen > minLen {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(maxLen-minLen+1)))
		length = minLen + int(n.Int64())
	}
	byteLen := (length + 1) / 2
	buf := make([]byte, byteLen)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)[:length]
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
