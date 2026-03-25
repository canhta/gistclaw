package tools

import (
	"net/http"
	"time"
)

const defaultResearchUserAgent = "gistclaw/0 research-tool"

func newBoundedHTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &http.Client{Timeout: timeout}
}
