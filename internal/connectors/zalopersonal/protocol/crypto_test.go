package protocol

import (
	"strings"
	"testing"
)

func TestEncodeDecodeAESCBCRoundTrip(t *testing.T) {
	t.Parallel()

	key := []byte("0123456789abcdef0123456789abcdef")
	plain := `{"hello":"zalo"}`

	encoded, err := EncodeAESCBC(key, plain, false)
	if err != nil {
		t.Fatalf("EncodeAESCBC: %v", err)
	}
	if encoded == "" {
		t.Fatal("expected ciphertext")
	}

	decoded, err := DecodeAESCBC(key, encoded)
	if err != nil {
		t.Fatalf("DecodeAESCBC: %v", err)
	}
	if string(decoded) != plain {
		t.Fatalf("expected %q, got %q", plain, string(decoded))
	}
}

func TestGenerateIMEIIncludesStableMD5Suffix(t *testing.T) {
	t.Parallel()

	first := GenerateIMEI("test-agent")
	second := GenerateIMEI("test-agent")

	if first == second {
		t.Fatal("expected unique IMEI prefix per call")
	}

	firstParts := strings.Split(first, "-")
	secondParts := strings.Split(second, "-")
	if len(firstParts) < 6 || len(secondParts) < 6 {
		t.Fatalf("expected uuid-md5 format, got %q and %q", first, second)
	}

	firstSuffix := firstParts[len(firstParts)-1]
	secondSuffix := secondParts[len(secondParts)-1]
	if len(firstSuffix) != 32 || len(secondSuffix) != 32 {
		t.Fatalf("expected 32-char md5 suffix, got %q and %q", firstSuffix, secondSuffix)
	}
	if firstSuffix != secondSuffix {
		t.Fatalf("expected stable md5 suffix, got %q and %q", firstSuffix, secondSuffix)
	}
}
