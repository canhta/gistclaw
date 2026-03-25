package runtime

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	workspaceTreeMaxDepth     = 2
	workspaceTreeMaxEntries   = 200
	workspaceFileMaxChars     = 4000
	workspaceTotalFileChars   = 16000
	workspaceTruncationNotice = "\n\n[truncated, inspect the file in the workspace for the full contents]"
)

var workspaceContextCandidates = []string{
	"AGENTS.md",
	"README.md",
	"README",
	"README.txt",
	"DESIGN.md",
	"go.mod",
	"package.json",
	"pyproject.toml",
	"Cargo.toml",
	"requirements.txt",
	"requirements-dev.txt",
	"requirements.in",
	"Makefile",
	"justfile",
	"Dockerfile",
	"docker-compose.yml",
	"docker-compose.yaml",
	".github/workflows/*.yml",
	".github/workflows/*.yaml",
}

var workspaceTreeSkipDirs = map[string]bool{
	".git":         true,
	".next":        true,
	".turbo":       true,
	".venv":        true,
	"build":        true,
	"coverage":     true,
	"dist":         true,
	"node_modules": true,
	"venv":         true,
	"vendor":       true,
}

type WorkspaceContextLoader interface {
	Load(ctx context.Context, workspaceRoot string) (WorkspaceContext, error)
}

type WorkspaceContext struct {
	Root  string
	Tree  []string
	Files []WorkspaceContextFile
}

type WorkspaceContextFile struct {
	Path    string
	Content string
}

type workspaceContextLoader struct{}

func newWorkspaceContextLoader() *workspaceContextLoader {
	return &workspaceContextLoader{}
}

func (l *workspaceContextLoader) Load(ctx context.Context, workspaceRoot string) (WorkspaceContext, error) {
	root := strings.TrimSpace(workspaceRoot)
	if root == "" {
		return WorkspaceContext{}, nil
	}

	root = filepath.Clean(root)
	info, err := os.Stat(root)
	if err != nil {
		return WorkspaceContext{}, fmt.Errorf("stat %q: %w", root, err)
	}
	if !info.IsDir() {
		return WorkspaceContext{}, fmt.Errorf("workspace root %q is not a directory", root)
	}

	tree, err := buildWorkspaceTree(ctx, root)
	if err != nil {
		return WorkspaceContext{}, err
	}
	files, err := loadWorkspaceContextFiles(ctx, root)
	if err != nil {
		return WorkspaceContext{}, err
	}

	return WorkspaceContext{
		Root:  root,
		Tree:  tree,
		Files: files,
	}, nil
}

func buildWorkspaceTree(ctx context.Context, workspaceRoot string) ([]string, error) {
	lines := make([]string, 0, workspaceTreeMaxEntries)
	err := filepath.WalkDir(workspaceRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if path == workspaceRoot {
			return nil
		}

		rel, err := filepath.Rel(workspaceRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		depth := strings.Count(rel, "/") + 1
		if d.IsDir() && workspaceTreeSkipDirs[d.Name()] {
			return filepath.SkipDir
		}
		if depth > workspaceTreeMaxDepth {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if len(lines) >= workspaceTreeMaxEntries {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		entry := "- " + rel
		if d.IsDir() {
			entry += "/"
		}
		lines = append(lines, entry)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("build workspace tree: %w", err)
	}
	sort.Strings(lines)
	return lines, nil
}

func loadWorkspaceContextFiles(ctx context.Context, workspaceRoot string) ([]WorkspaceContextFile, error) {
	paths, err := resolveWorkspaceContextPaths(workspaceRoot)
	if err != nil {
		return nil, err
	}

	files := make([]WorkspaceContextFile, 0, len(paths))
	remainingChars := workspaceTotalFileChars
	for _, absPath := range paths {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if remainingChars <= 0 {
			break
		}
		content, err := os.ReadFile(absPath)
		if err != nil {
			return nil, fmt.Errorf("read workspace context file %q: %w", absPath, err)
		}
		relPath, err := filepath.Rel(workspaceRoot, absPath)
		if err != nil {
			return nil, fmt.Errorf("rel workspace context path %q: %w", absPath, err)
		}
		text := truncateWorkspaceContent(string(content), remainingChars)
		if text == "" {
			continue
		}
		files = append(files, WorkspaceContextFile{
			Path:    filepath.ToSlash(relPath),
			Content: text,
		})
		remainingChars -= len(text)
	}

	return files, nil
}

func resolveWorkspaceContextPaths(workspaceRoot string) ([]string, error) {
	seen := make(map[string]bool)
	paths := make([]string, 0, len(workspaceContextCandidates))
	for _, pattern := range workspaceContextCandidates {
		matches, err := filepath.Glob(filepath.Join(workspaceRoot, pattern))
		if err != nil {
			return nil, fmt.Errorf("glob workspace context files %q: %w", pattern, err)
		}
		sort.Strings(matches)
		for _, match := range matches {
			if seen[match] {
				continue
			}
			info, err := os.Stat(match)
			if err != nil {
				return nil, fmt.Errorf("stat workspace context file %q: %w", match, err)
			}
			if info.IsDir() {
				continue
			}
			seen[match] = true
			paths = append(paths, match)
		}
	}
	return paths, nil
}

func truncateWorkspaceContent(content string, budget int) string {
	if budget <= 0 {
		return ""
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}

	limit := workspaceFileMaxChars
	if budget < limit {
		limit = budget
	}
	if len(content) <= limit {
		return content
	}
	if limit <= len(workspaceTruncationNotice) {
		return content[:limit]
	}
	return content[:limit-len(workspaceTruncationNotice)] + workspaceTruncationNotice
}

func renderWorkspaceFileBlock(path string, content string) string {
	return fmt.Sprintf("<file name=\"%s\">\n%s\n</file>", escapeWorkspaceFileAttr(path), escapeWorkspaceFileContent(content))
}

func escapeWorkspaceFileAttr(value string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(strings.TrimSpace(value))
}

func escapeWorkspaceFileContent(value string) string {
	value = strings.ReplaceAll(value, "</file>", "&lt;/file&gt;")
	value = strings.ReplaceAll(value, "<file", "&lt;file")
	return value
}
