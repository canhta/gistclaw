package protocol

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type UploadCallbackRegistrar interface {
	RegisterUploadCallback(fileID string) <-chan string
	CancelUploadCallback(fileID string)
}

type FileUploadResult struct {
	FileID       string     `json:"fileId"`
	FileURL      string     `json:"fileUrl"`
	ClientFileID FlexNumber `json:"clientFileId"`
	ChunkID      int        `json:"chunkId"`
	Finished     int        `json:"finished"`
	TotalSize    int        `json:"-"`
	FileName     string     `json:"-"`
	Checksum     string     `json:"-"`
}

func UploadFile(ctx context.Context, sess *Session, callbacks UploadCallbackRegistrar, threadID string, threadType ThreadType, filePath string) (*FileUploadResult, error) {
	fileURL := getServiceURL(sess, "file")
	if fileURL == "" {
		return nil, fmt.Errorf("zalo personal protocol: no file service URL")
	}
	if err := checkFileSize(filePath); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("zalo personal protocol: read file: %w", err)
	}

	fileName := filepath.Base(filePath)
	totalSize := len(data)
	clientID := time.Now().UnixMilli()
	params := map[string]any{
		"totalChunk": 1,
		"fileName":   fileName,
		"clientId":   clientID,
		"totalSize":  totalSize,
		"imei":       sess.IMEI,
		"isE2EE":     0,
		"jxl":        0,
		"chunkId":    1,
	}
	pathPrefix := "/api/message/"
	typeParam := "2"
	if threadType == ThreadTypeGroup {
		params["grid"] = threadID
		pathPrefix = "/api/group/"
		typeParam = "11"
	} else {
		params["toid"] = threadID
	}

	encParams, err := encryptPayload(sess, params)
	if err != nil {
		return nil, fmt.Errorf("zalo personal protocol: encrypt file upload params: %w", err)
	}
	uploadURL := makeURL(sess, fileURL+pathPrefix+"asyncfile/upload", map[string]any{
		"type":   typeParam,
		"params": encParams,
	}, true)
	body, contentType, err := buildMultipartBody("chunkContent", fileName, data)
	if err != nil {
		return nil, fmt.Errorf("zalo personal protocol: build multipart: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, body)
	if err != nil {
		return nil, err
	}
	setDefaultHeaders(req, sess)
	req.Header.Set("Content-Type", contentType)

	resp, err := sess.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("zalo personal protocol: upload file: %w", err)
	}
	defer resp.Body.Close()

	var envelope Response[*string]
	if err := readJSON(resp, &envelope); err != nil {
		return nil, fmt.Errorf("zalo personal protocol: parse file upload response: %w", err)
	}
	if envelope.ErrorCode != 0 {
		return nil, fmt.Errorf("zalo personal protocol: file upload error code %d", envelope.ErrorCode)
	}
	if envelope.Data == nil {
		return nil, fmt.Errorf("zalo personal protocol: empty file upload response")
	}

	plain, err := decryptDataField(sess, *envelope.Data)
	if err != nil {
		return nil, fmt.Errorf("zalo personal protocol: decrypt file upload response: %w", err)
	}
	var result FileUploadResult
	if err := json.Unmarshal(plain, &result); err != nil {
		return nil, fmt.Errorf("zalo personal protocol: parse file upload result: %w", err)
	}
	result.TotalSize = totalSize
	result.FileName = fileName
	result.Checksum = md5Hash(data)

	if callbacks != nil && result.FileID != "" && result.FileID != "-1" {
		urlCh := callbacks.RegisterUploadCallback(result.FileID)
		select {
		case result.FileURL = <-urlCh:
		case <-time.After(30 * time.Second):
			callbacks.CancelUploadCallback(result.FileID)
			return nil, fmt.Errorf("zalo personal protocol: timeout waiting for file upload callback (fileId=%s)", result.FileID)
		case <-ctx.Done():
			callbacks.CancelUploadCallback(result.FileID)
			return nil, ctx.Err()
		}
	}

	return &result, nil
}

func SendFile(ctx context.Context, sess *Session, threadID string, threadType ThreadType, upload *FileUploadResult) (string, error) {
	fileURL := getServiceURL(sess, "file")
	if fileURL == "" {
		return "", fmt.Errorf("zalo personal protocol: no file service URL")
	}
	ext := strings.TrimPrefix(filepath.Ext(upload.FileName), ".")
	params := map[string]any{
		"fileId":      upload.FileID,
		"checksum":    upload.Checksum,
		"checksumSha": "",
		"extention":   ext,
		"totalSize":   upload.TotalSize,
		"fileName":    upload.FileName,
		"clientId":    upload.ClientFileID.String(),
		"fType":       1,
		"fileCount":   0,
		"fdata":       "{}",
		"fileUrl":     upload.FileURL,
		"zsource":     -1,
		"ttl":         0,
	}
	pathPrefix := "/api/message/"
	if threadType == ThreadTypeGroup {
		params["grid"] = threadID
		pathPrefix = "/api/group/"
	} else {
		params["toid"] = threadID
	}

	encData, err := encryptPayload(sess, params)
	if err != nil {
		return "", fmt.Errorf("zalo personal protocol: encrypt file send params: %w", err)
	}
	sendURL := makeURL(sess, fileURL+pathPrefix+"asyncfile/msg", map[string]any{"nretry": 0}, true)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sendURL, buildFormBody(map[string]string{"params": encData}))
	if err != nil {
		return "", err
	}
	setDefaultHeaders(req, sess)

	resp, err := sess.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("zalo personal protocol: send file: %w", err)
	}
	defer resp.Body.Close()

	var envelope Response[*string]
	if err := readJSON(resp, &envelope); err != nil {
		return "", fmt.Errorf("zalo personal protocol: parse file send response: %w", err)
	}
	if envelope.ErrorCode != 0 {
		return "", fmt.Errorf("zalo personal protocol: file send error code %d", envelope.ErrorCode)
	}
	if envelope.Data == nil {
		return "", nil
	}

	plain, err := decryptDataField(sess, *envelope.Data)
	if err != nil {
		return "", fmt.Errorf("zalo personal protocol: decrypt file send response: %w", err)
	}
	var result struct {
		MsgID json.RawMessage `json:"msgId"`
	}
	if err := json.Unmarshal(plain, &result); err != nil {
		return "", fmt.Errorf("zalo personal protocol: parse file send result: %w", err)
	}
	msgID, err := parseMessageID(result.MsgID)
	if err != nil {
		return "", fmt.Errorf("zalo personal protocol: parse file send result: %w", err)
	}
	return msgID, nil
}
