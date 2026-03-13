// cmd/gistclaw-hook/main.go
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"io"
	"net/http"
	"os"
	"time"
)

// hookTimeout is the maximum time gistclaw-hook will wait for a response from
// the gistclaw hook server. Claude Code's default hook timeout is 10 minutes;
// we use 6 minutes to allow the operator time to respond with a small margin.
const hookTimeout = 6 * time.Minute

func main() {
	hookType := flag.String("type", "pretool", "Hook type: pretool | notification | stop")
	addr := flag.String("addr", "127.0.0.1:8765", "Address of the gistclaw hook server")
	flag.Parse()

	// Read JSON from stdin.
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		writeStderr(denyResponse("failed to read stdin: " + err.Error()))
		os.Exit(2)
	}

	// POST to the hook server.
	url := "http://" + *addr + "/hook/" + *hookType
	client := &http.Client{Timeout: hookTimeout}
	resp, err := client.Post(url, "application/json", bytes.NewReader(input))
	if err != nil {
		writeStderr(denyResponse("hook server unreachable: " + err.Error()))
		os.Exit(2)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		writeStderr(denyResponse("failed to read response: " + err.Error()))
		os.Exit(2)
	}

	// For non-pretool types, just exit 0 — no decision needed.
	if *hookType != "pretool" {
		os.Exit(0)
	}

	// Parse decision from response.
	var result struct {
		HookSpecificOutput struct {
			PermissionDecision string `json:"permissionDecision"`
		} `json:"hookSpecificOutput"`
		SystemMessage string `json:"systemMessage"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		writeStderr(denyResponse("malformed response from hook server: " + err.Error()))
		os.Exit(2)
	}

	if result.HookSpecificOutput.PermissionDecision == "allow" {
		_, _ = os.Stdout.Write(body)
		os.Exit(0)
	}

	// Deny.
	writeStderr(body)
	os.Exit(2)
}

// denyResponse returns a JSON deny payload as a byte slice.
func denyResponse(reason string) []byte {
	b, _ := json.Marshal(map[string]interface{}{
		"hookSpecificOutput": map[string]string{
			"permissionDecision": "deny",
		},
		"systemMessage": reason,
	})
	return b
}

// writeStderr writes data to stderr, ignoring write errors.
func writeStderr(data []byte) {
	_, _ = os.Stderr.Write(data)
}
