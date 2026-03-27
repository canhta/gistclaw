package protocol

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
)

var (
	errInvalidBlockSize    = errors.New("zalo personal protocol: invalid block size")
	errInvalidPKCS7Data    = errors.New("zalo personal protocol: invalid pkcs7 data")
	errInvalidPKCS7Padding = errors.New("zalo personal protocol: invalid pkcs7 padding")
)

func EncodeAESCBC(key []byte, data string, encHex bool) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("zalo personal protocol: new cipher: %w", err)
	}

	plain, err := pkcs7Pad([]byte(data), aes.BlockSize)
	if err != nil {
		return "", fmt.Errorf("zalo personal protocol: pkcs7 pad: %w", err)
	}

	iv := make([]byte, aes.BlockSize)
	ciphertext := make([]byte, len(plain))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ciphertext, plain)

	if encHex {
		return hex.EncodeToString(ciphertext), nil
	}
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func DecodeAESCBC(key []byte, data string) ([]byte, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, fmt.Errorf("zalo personal protocol: base64 decode: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("zalo personal protocol: new cipher: %w", err)
	}

	iv := make([]byte, aes.BlockSize)
	plain := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, ciphertext)

	plain, err = pkcs7Unpad(plain, aes.BlockSize)
	if err != nil {
		return nil, fmt.Errorf("zalo personal protocol: pkcs7 unpad: %w", err)
	}
	return plain, nil
}

func pkcs7Pad(data []byte, blockSize int) ([]byte, error) {
	if blockSize <= 0 {
		return nil, errInvalidBlockSize
	}
	padLen := blockSize - (len(data) % blockSize)
	if padLen == 0 {
		padLen = blockSize
	}
	return append(data, bytes.Repeat([]byte{byte(padLen)}, padLen)...), nil
}

func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if blockSize <= 0 {
		return nil, errInvalidBlockSize
	}
	if len(data) == 0 || len(data)%blockSize != 0 {
		return nil, errInvalidPKCS7Data
	}

	padLen := int(data[len(data)-1])
	if padLen == 0 || padLen > blockSize || padLen > len(data) {
		return nil, errInvalidPKCS7Padding
	}
	if !bytes.Equal(bytes.Repeat([]byte{byte(padLen)}, padLen), data[len(data)-padLen:]) {
		return nil, errInvalidPKCS7Padding
	}
	return data[:len(data)-padLen], nil
}
