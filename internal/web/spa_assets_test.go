package web

import (
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
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

func TestSPAStaticAssetHandlerServesBuiltAppFiles(t *testing.T) {
	t.Parallel()

	body, err := readSPAAsset("index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}

	assetPath := firstSPAAssetPath(t, string(body))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, assetPath, nil)

	serveSPAAssets().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("GET %s status = %d, want %d body=%s", assetPath, rr.Code, http.StatusOK, rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); !strings.Contains(got, "javascript") {
		t.Fatalf("GET %s Content-Type = %q, want javascript asset", assetPath, got)
	}
}

func firstSPAAssetPath(t *testing.T, indexHTML string) string {
	t.Helper()

	re := regexp.MustCompile(`/_app/immutable/[^"]+\.(js|css)`)
	match := re.FindString(indexHTML)
	if match == "" {
		t.Fatalf("expected index.html to reference a built app asset, got %q", indexHTML)
	}
	return match
}
