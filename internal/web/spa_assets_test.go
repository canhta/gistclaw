package web

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
)

func TestSPAIndexServesCommittedPlaceholder(t *testing.T) {
	t.Parallel()

	body, err := readSPAAsset("index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}

	content := string(body)
	if !strings.Contains(strings.ToLower(content), "<!doctype html>") {
		t.Fatalf("expected index.html to be an html document, got %q", content)
	}
}

func TestSPAStaticAssetLookupRejectsMissingFiles(t *testing.T) {
	t.Parallel()

	_, err := readSPAAsset("missing.js")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("read missing.js error = %v, want %v", err, fs.ErrNotExist)
	}
}
