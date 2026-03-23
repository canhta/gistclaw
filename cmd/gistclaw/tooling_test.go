package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepoTooling_MakeTargets(t *testing.T) {
	root := findModuleRoot(t)

	targets := []struct {
		name      string
		target    string
		args      []string
		wantSnips []string
	}{
		{
			name:      "fmt",
			target:    "fmt",
			wantSnips: []string{"goimports", ".go"},
		},
		{
			name:      "lint",
			target:    "lint",
			wantSnips: []string{"golangci-lint", "run"},
		},
		{
			name:      "test",
			target:    "test",
			wantSnips: []string{"go test ./..."},
		},
		{
			name:      "coverage",
			target:    "coverage",
			wantSnips: []string{"go test ./... -coverprofile=coverage.out", "go tool cover -func=coverage.out", "COVERAGE_MIN"},
		},
		{
			name:      "run",
			target:    "run",
			wantSnips: []string{"go run ./cmd/gistclaw"},
		},
		{
			name:      "dev",
			target:    "dev",
			wantSnips: []string{"lefthook", "goimports", "golangci-lint.run/install.sh"},
		},
		{
			name:      "hooks-install",
			target:    "hooks-install",
			wantSnips: []string{"lefthook install"},
		},
		{
			name:      "precommit helper",
			target:    "precommit",
			args:      []string{"FILES=cmd/gistclaw/main.go"},
			wantSnips: []string{"goimports", "golangci-lint", "--fast-only"},
		},
	}

	for _, tc := range targets {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{"-B", "-n", "-f", filepath.Join(root, "Makefile"), tc.target}
			args = append(args, tc.args...)

			cmd := exec.Command("make", args...)
			cmd.Dir = root
			cmd.Env = append(os.Environ(), "GOFLAGS=")

			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("make -n %s failed: %v\n%s", tc.target, err, out)
			}

			output := string(out)
			for _, snippet := range tc.wantSnips {
				if !strings.Contains(output, snippet) {
					t.Fatalf("make -n %s output missing %q:\n%s", tc.target, snippet, output)
				}
			}
		})
	}
}

func TestRepoTooling_ConfigFiles(t *testing.T) {
	root := findModuleRoot(t)

	files := []struct {
		path      string
		wantSnips []string
	}{
		{
			path: filepath.Join(root, ".golangci.yml"),
			wantSnips: []string{
				`version: "2"`,
				"linters:",
				"govet",
			},
		},
		{
			path: filepath.Join(root, "lefthook.yml"),
			wantSnips: []string{
				"pre-commit:",
				"pre-push:",
				"make precommit",
				"make coverage",
			},
		},
		{
			path: filepath.Join(root, "README.md"),
			wantSnips: []string{
				"make dev",
				"make hooks-install",
				"make fmt",
				"make lint",
				"make test",
				"make coverage",
				"70%",
			},
		},
	}

	for _, tc := range files {
		t.Run(filepath.Base(tc.path), func(t *testing.T) {
			data, err := os.ReadFile(tc.path)
			if err != nil {
				t.Fatalf("ReadFile failed: %v", err)
			}

			content := string(data)
			for _, snippet := range tc.wantSnips {
				if !strings.Contains(content, snippet) {
					t.Fatalf("%s missing %q", tc.path, snippet)
				}
			}
		})
	}
}
