package protocol

import (
	"bytes"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
)

const maxUploadSize = 25 * 1024 * 1024

func checkFileSize(filePath string) error {
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("zalo personal protocol: stat file: %w", err)
	}
	if info.Size() > maxUploadSize {
		return fmt.Errorf("zalo personal protocol: file too large: %d bytes (max %d)", info.Size(), maxUploadSize)
	}
	return nil
}

type FlexBool bool

func (b *FlexBool) UnmarshalJSON(data []byte) error {
	switch string(data) {
	case "true", "1":
		*b = true
	default:
		*b = false
	}
	return nil
}

type FlexNumber string

func (n *FlexNumber) UnmarshalJSON(data []byte) error {
	var asString string
	if err := json.Unmarshal(data, &asString); err == nil {
		*n = FlexNumber(asString)
		return nil
	}
	var asNumber json.Number
	if err := json.Unmarshal(data, &asNumber); err == nil {
		*n = FlexNumber(asNumber.String())
		return nil
	}
	return fmt.Errorf("invalid flexible number %q", string(data))
}

func (n FlexNumber) String() string {
	return string(n)
}

func buildMultipartBody(fieldName, fileName string, data []byte) (io.Reader, string, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	boundary := "----GistClaw" + randomBoundary()
	if err := writer.SetBoundary(boundary); err != nil {
		return nil, "", err
	}

	part, err := writer.CreateFormFile(fieldName, filepath.Base(fileName))
	if err != nil {
		return nil, "", err
	}
	if _, err := part.Write(data); err != nil {
		return nil, "", err
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}

	return &buf, writer.FormDataContentType(), nil
}

func randomBoundary() string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

func md5Hash(data []byte) string {
	sum := md5.Sum(data)
	return hex.EncodeToString(sum[:])
}

func parseMessageID(raw json.RawMessage) (string, error) {
	var msgID string
	if err := json.Unmarshal(raw, &msgID); err == nil {
		return msgID, nil
	}

	var numericID json.Number
	if err := json.Unmarshal(raw, &numericID); err == nil {
		return numericID.String(), nil
	}

	return "", fmt.Errorf("unsupported msgId format")
}
