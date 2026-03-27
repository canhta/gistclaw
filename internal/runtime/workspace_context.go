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
	directoryTreeMaxDepth     = 2
	directoryTreeMaxEntries   = 200
	directoryFileMaxChars     = 4000
	directoryTotalFileChars   = 16000
	directoryTruncationNotice = "\n\n[truncated, inspect the file in the working directory for the full contents]"
)

var directoryContextCandidates = []string{
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

var directoryTreeSkipDirs = map[string]bool{
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

type DirectoryContextLoader interface {
	Load(ctx context.Context, root string) (DirectoryContext, error)
}

type DirectoryContext struct {
	Root  string
	Tree  []string
	Files []DirectoryContextFile
}

type DirectoryContextFile struct {
	Path    string
	Content string
}

type directoryContextLoader struct{}

func newDirectoryContextLoader() *directoryContextLoader {
	return &directoryContextLoader{}
}

func (l *directoryContextLoader) Load(ctx context.Context, root string) (DirectoryContext, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return DirectoryContext{}, nil
	}

	root = filepath.Clean(root)
	info, err := os.Stat(root)
	if err != nil {
		return DirectoryContext{}, fmt.Errorf("stat %q: %w", root, err)
	}
	if !info.IsDir() {
		return DirectoryContext{}, fmt.Errorf("working directory %q is not a directory", root)
	}

	tree, err := buildDirectoryTree(ctx, root)
	if err != nil {
		return DirectoryContext{}, err
	}
	files, err := loadDirectoryContextFiles(ctx, root)
	if err != nil {
		return DirectoryContext{}, err
	}

	return DirectoryContext{
		Root:  root,
		Tree:  tree,
		Files: files,
	}, nil
}

func buildDirectoryTree(ctx context.Context, root string) ([]string, error) {
	lines := make([]string, 0, directoryTreeMaxEntries)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if path == root {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		depth := strings.Count(rel, "/") + 1
		if d.IsDir() && directoryTreeSkipDirs[d.Name()] {
			return filepath.SkipDir
		}
		if depth > directoryTreeMaxDepth {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if len(lines) >= directoryTreeMaxEntries {
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
		return nil, fmt.Errorf("build directory tree: %w", err)
	}
	sort.Strings(lines)
	return lines, nil
}

func loadDirectoryContextFiles(ctx context.Context, root string) ([]DirectoryContextFile, error) {
	paths, err := resolveDirectoryContextPaths(root)
	if err != nil {
		return nil, err
	}

	files := make([]DirectoryContextFile, 0, len(paths))
	remainingChars := directoryTotalFileChars
	for _, absPath := range paths {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if remainingChars <= 0 {
			break
		}
		content, err := os.ReadFile(absPath)
		if err != nil {
			return nil, fmt.Errorf("read directory context file %q: %w", absPath, err)
		}
		relPath, err := filepath.Rel(root, absPath)
		if err != nil {
			return nil, fmt.Errorf("rel directory context path %q: %w", absPath, err)
		}
		text := truncateDirectoryContent(string(content), remainingChars)
		if text == "" {
			continue
		}
		files = append(files, DirectoryContextFile{
			Path:    filepath.ToSlash(relPath),
			Content: text,
		})
		remainingChars -= len(text)
	}

	return files, nil
}

func resolveDirectoryContextPaths(root string) ([]string, error) {
	seen := make(map[string]bool)
	paths := make([]string, 0, len(directoryContextCandidates))
	for _, pattern := range directoryContextCandidates {
		matches, err := filepath.Glob(filepath.Join(root, pattern))
		if err != nil {
			return nil, fmt.Errorf("glob directory context files %q: %w", pattern, err)
		}
		sort.Strings(matches)
		for _, match := range matches {
			if seen[match] {
				continue
			}
			info, err := os.Stat(match)
			if err != nil {
				return nil, fmt.Errorf("stat directory context file %q: %w", match, err)
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

func truncateDirectoryContent(content string, budget int) string {
	if budget <= 0 {
		return ""
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}

	limit := directoryFileMaxChars
	if budget < limit {
		limit = budget
	}
	if len(content) <= limit {
		return content
	}
	if limit <= len(directoryTruncationNotice) {
		return content[:limit]
	}
	return content[:limit-len(directoryTruncationNotice)] + directoryTruncationNotice
}

func renderDirectoryFileBlock(path string, content string) string {
	return fmt.Sprintf("<file name=\"%s\">\n%s\n</file>", escapeDirectoryFileAttr(path), escapeDirectoryFileContent(content))
}

func escapeDirectoryFileAttr(value string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(strings.TrimSpace(value))
}

func escapeDirectoryFileContent(value string) string {
	value = strings.ReplaceAll(value, "</file>", "&lt;/file&gt;")
	value = strings.ReplaceAll(value, "<file", "&lt;file")
	return value
}
