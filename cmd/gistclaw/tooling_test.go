package main

import (
	"io/fs"
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
			wantSnips: []string{"git config core.hooksPath", ".githooks"},
		},
		{
			name:      "precommit helper",
			target:    "precommit",
			args:      []string{"FILES=cmd/gistclaw/main.go"},
			wantSnips: []string{"goimports", "golangci-lint", "--fast-only"},
		},
		{
			name:      "precommit helper frontend",
			target:    "precommit",
			args:      []string{"FILES=frontend/src/routes/+page.svelte"},
			wantSnips: []string{"cd frontend && bun run lint", "cd frontend && bun run check"},
		},
		{
			name:   "prepush helper",
			target: "prepush",
			wantSnips: []string{
				"golangci-lint",
				"go test ./... -coverprofile=coverage.out",
				"cd frontend && bun run check",
				"cd frontend && bun run lint",
				"cd frontend && bun run test:unit -- --run",
				"cd frontend && bun run build",
			},
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
				"make prepush",
			},
		},
		{
			path: filepath.Join(root, ".githooks", "run"),
			wantSnips: []string{
				".bin/lefthook",
				"--no-auto-install",
				"make dev",
			},
		},
		{
			path: filepath.Join(root, ".githooks", "pre-commit"),
			wantSnips: []string{
				`/run" pre-commit`,
			},
		},
		{
			path: filepath.Join(root, ".githooks", "pre-push"),
			wantSnips: []string{
				`/run" pre-push`,
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

func TestRepoTooling_ReleaseContract(t *testing.T) {
	root := findModuleRoot(t)

	files := []struct {
		path      string
		wantSnips []string
	}{
		{
			path: filepath.Join(root, ".github", "workflows", "ci.yml"),
			wantSnips: []string{
				"actions/checkout@v6",
				"actions/setup-go@v6",
				"go test ./...",
				"go test -cover ./...",
				"go vet ./...",
				"bash scripts/gistclaw-install-smoke.sh",
			},
		},
		{
			path: filepath.Join(root, ".github", "workflows", "release.yml"),
			wantSnips: []string{
				"actions/checkout@v6",
				"actions/setup-go@v6",
				"refs/tags/v",
				"GOOS=darwin",
				"GOARCH=arm64",
				"GOOS=linux",
				"GOARCH=amd64",
				"-X github.com/canhta/gistclaw/cmd/gistclaw.version=",
				"SHA256SUMS.txt",
				"scripts/gistclaw-install.sh",
				"gh release create",
			},
		},
		{
			path: filepath.Join(root, "scripts", "gistclaw-install.sh"),
			wantSnips: []string{
				"/etc/gistclaw/config.yaml",
				"/usr/local/bin/gistclaw",
				"/etc/systemd/system/gistclaw.service",
				"--config-file",
				"--public-domain",
				"Caddyfile",
				"inspect config-paths",
				"storage_root:",
				"extract_config_value \"storage_root\"",
				"gistclaw inspect systemd-unit",
				"chown -R gistclaw:gistclaw",
				"chown root:gistclaw",
				"chmod 640",
				"systemctl enable gistclaw",
				"systemctl is-active --quiet gistclaw",
				"systemctl restart gistclaw",
				"systemctl start gistclaw",
			},
		},
		{
			path: filepath.Join(root, "scripts", "gistclaw-install-smoke.sh"),
			wantSnips: []string{
				"sha256sum",
				"systemctl",
				"curl",
				"checksum mismatch",
				"public-domain",
				"storage_root:",
				"GISTCLAW_FAKE_STORAGE_ROOT",
			},
		},
		{
			path: filepath.Join(root, "README.md"),
			wantSnips: []string{
				"GitHub Releases",
				"docs/install-ubuntu.md",
				"docs/install-macos.md",
				"--config-file",
				"gistclaw version",
				"gistclaw inspect systemd-unit",
				"gistclaw inspect token",
			},
		},
		{
			path: filepath.Join(root, "docs", "install-ubuntu.md"),
			wantSnips: []string{
				"Ubuntu 24",
				"self-contained",
				"/var/lib/gistclaw",
				"--config-file",
				"--public-domain",
				"`storage_root`",
				"/etc/caddy/Caddyfile",
				"exact config file",
				"systemctl status gistclaw",
				"journalctl -u gistclaw",
				"gistclaw doctor",
				"gistclaw security audit",
			},
		},
		{
			path: filepath.Join(root, "docs", "install-macos.md"),
			wantSnips: []string{
				"Apple Silicon",
				"self-contained",
				"gistclaw serve",
				"127.0.0.1:8080",
			},
		},
		{
			path: filepath.Join(root, "docs", "recovery.md"),
			wantSnips: []string{
				"gistclaw backup",
				"restore",
				"rollback",
				"GitHub release URL",
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
			if strings.HasSuffix(tc.path, filepath.Join(".github", "workflows", "release.yml")) {
				for _, forbidden := range []string{
					"actions/checkout@v4",
					"actions/setup-go@v5",
					"softprops/action-gh-release",
				} {
					if strings.Contains(content, forbidden) {
						t.Fatalf("%s still contains deprecated release dependency %q", tc.path, forbidden)
					}
				}
			}
		})
	}
}

func TestRepoTooling_DocsUseRepoRelativeLinks(t *testing.T) {
	root := findModuleRoot(t)

	files, err := filepath.Glob(filepath.Join(root, "*.md"))
	if err != nil {
		t.Fatalf("Glob root markdown failed: %v", err)
	}

	err = filepath.WalkDir(filepath.Join(root, "docs"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) == ".md" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir docs failed: %v", err)
	}

	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile %s failed: %v", path, err)
		}
		if strings.Contains(string(data), root) {
			t.Fatalf("%s contains workspace-absolute links rooted at %q", path, root)
		}
	}
}
