package protocol

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type ImageUploadResult struct {
	NormalURL    string     `json:"normalUrl"`
	HDUrl        string     `json:"hdUrl"`
	ThumbURL     string     `json:"thumbUrl"`
	PhotoID      FlexNumber `json:"photoId"`
	ClientFileID FlexNumber `json:"clientFileId"`
	ChunkID      int        `json:"chunkId"`
	Finished     FlexBool   `json:"finished"`
	Width        int        `json:"-"`
	Height       int        `json:"-"`
	TotalSize    int        `json:"-"`
}

func UploadImage(ctx context.Context, sess *Session, threadID string, threadType ThreadType, filePath string) (*ImageUploadResult, error) {
	fileURL := getServiceURL(sess, "file")
	if fileURL == "" {
		return nil, fmt.Errorf("zalo personal protocol: no file service URL")
	}
	if err := checkFileSize(filePath); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("zalo personal protocol: read image: %w", err)
	}

	fileName := filepath.Base(filePath)
	totalSize := len(data)
	width, height := imageDimensions(data)
	params := map[string]any{
		"totalChunk": 1,
		"fileName":   fileName,
		"clientId":   time.Now().UnixMilli(),
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
		return nil, fmt.Errorf("zalo personal protocol: encrypt upload params: %w", err)
	}

	uploadURL := makeURL(sess, fileURL+pathPrefix+"photo_original/upload", map[string]any{
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
		return nil, fmt.Errorf("zalo personal protocol: upload image: %w", err)
	}
	defer resp.Body.Close()

	var envelope Response[*string]
	if err := readJSON(resp, &envelope); err != nil {
		return nil, fmt.Errorf("zalo personal protocol: parse upload response: %w", err)
	}
	if envelope.ErrorCode != 0 {
		return nil, fmt.Errorf("zalo personal protocol: upload error code %d", envelope.ErrorCode)
	}
	if envelope.Data == nil {
		return nil, fmt.Errorf("zalo personal protocol: empty upload response")
	}

	plain, err := decryptDataField(sess, *envelope.Data)
	if err != nil {
		return nil, fmt.Errorf("zalo personal protocol: decrypt upload response: %w", err)
	}

	var result ImageUploadResult
	if err := json.Unmarshal(plain, &result); err != nil {
		return nil, fmt.Errorf("zalo personal protocol: parse upload result: %w", err)
	}
	result.Width = width
	result.Height = height
	result.TotalSize = totalSize
	return &result, nil
}

func SendImage(ctx context.Context, sess *Session, threadID string, threadType ThreadType, upload *ImageUploadResult, caption string) (string, error) {
	fileURL := getServiceURL(sess, "file")
	if fileURL == "" {
		return "", fmt.Errorf("zalo personal protocol: no file service URL")
	}
	params := map[string]any{
		"photoId":  upload.PhotoID,
		"clientId": strconv.FormatInt(time.Now().UnixMilli(), 10),
		"desc":     caption,
		"width":    upload.Width,
		"height":   upload.Height,
		"rawUrl":   upload.NormalURL,
		"hdUrl":    upload.HDUrl,
		"thumbUrl": upload.ThumbURL,
		"hdSize":   strconv.Itoa(upload.TotalSize),
		"zsource":  -1,
		"ttl":      0,
		"jcp":      `{"convertible":"jxl"}`,
	}
	pathPrefix := "/api/message/"
	if threadType == ThreadTypeGroup {
		params["grid"] = threadID
		params["oriUrl"] = upload.NormalURL
		pathPrefix = "/api/group/"
	} else {
		params["toid"] = threadID
		params["normalUrl"] = upload.NormalURL
	}

	encData, err := encryptPayload(sess, params)
	if err != nil {
		return "", fmt.Errorf("zalo personal protocol: encrypt image send params: %w", err)
	}
	sendURL := makeURL(sess, fileURL+pathPrefix+"photo_original/send", map[string]any{"nretry": 0}, true)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sendURL, buildFormBody(map[string]string{"params": encData}))
	if err != nil {
		return "", err
	}
	setDefaultHeaders(req, sess)

	resp, err := sess.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("zalo personal protocol: send image: %w", err)
	}
	defer resp.Body.Close()

	var envelope Response[*string]
	if err := readJSON(resp, &envelope); err != nil {
		return "", fmt.Errorf("zalo personal protocol: parse image send response: %w", err)
	}
	if envelope.ErrorCode != 0 {
		return "", fmt.Errorf("zalo personal protocol: image send error code %d", envelope.ErrorCode)
	}
	if envelope.Data == nil {
		return "", nil
	}

	plain, err := decryptDataField(sess, *envelope.Data)
	if err != nil {
		return "", fmt.Errorf("zalo personal protocol: decrypt image send response: %w", err)
	}
	var result struct {
		MsgID json.RawMessage `json:"msgId"`
	}
	if err := json.Unmarshal(plain, &result); err != nil {
		return "", fmt.Errorf("zalo personal protocol: parse image send result: %w", err)
	}
	msgID, err := parseMessageID(result.MsgID)
	if err != nil {
		return "", fmt.Errorf("zalo personal protocol: parse image send result: %w", err)
	}
	return msgID, nil
}

func imageDimensions(data []byte) (int, int) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

func IsImageFile(filePath string) bool {
	switch strings.ToLower(filepath.Ext(filePath)) {
	case ".jpg", ".jpeg", ".png", ".webp":
		return true
	default:
		return false
	}
}
