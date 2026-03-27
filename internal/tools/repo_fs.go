package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/canhta/gistclaw/internal/model"
)

type ListDirTool struct{}

func NewListDirTool() *ListDirTool { return &ListDirTool{} }

func (t *ListDirTool) Name() string { return "list_dir" }

func (t *ListDirTool) Spec() model.ToolSpec {
	return model.ToolSpec{
		Name:            t.Name(),
		Description:     "List direct children under one path from the run directory or an explicit host path when authority allows.",
		InputSchemaJSON: `{"type":"object","properties":{"path":{"type":"string"}}}`,
		Risk:            model.RiskLow,
		SideEffect:      effectRead,
		Approval:        "never",
	}
}

func (t *ListDirTool) Invoke(ctx context.Context, call model.ToolCall) (model.ToolResult, error) {
	root, err := cwdFromContext(ctx)
	if err != nil {
		return model.ToolResult{}, err
	}
	env := authorityFromContext(ctx)
	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(call.InputJSON, &input); err != nil {
		return model.ToolResult{}, fmt.Errorf("list_dir: decode input: %w", err)
	}
	absPath, relPath, err := resolveToolPath(root, input.Path, env)
	if err != nil {
		return model.ToolResult{}, fmt.Errorf("list_dir: %w", err)
	}
	entries, err := os.ReadDir(absPath)
	if err != nil {
		return model.ToolResult{}, fmt.Errorf("list_dir: read dir: %w", err)
	}

	type entry struct {
		Name string `json:"name"`
		Path string `json:"path"`
		Kind string `json:"kind"`
		Size int64  `json:"size"`
	}
	items := make([]entry, 0, len(entries))
	for _, item := range entries {
		info, err := item.Info()
		if err != nil {
			return model.ToolResult{}, fmt.Errorf("list_dir: stat %s: %w", item.Name(), err)
		}
		entryPath := item.Name()
		if relPath != "." {
			entryPath = filepath.ToSlash(filepath.Join(relPath, item.Name()))
		}
		kind := "file"
		if item.IsDir() {
			kind = "dir"
		}
		items = append(items, entry{
			Name: item.Name(),
			Path: filepath.ToSlash(entryPath),
			Kind: kind,
			Size: info.Size(),
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })

	output, err := json.Marshal(struct {
		Path    string  `json:"path"`
		Entries []entry `json:"entries"`
	}{
		Path:    filepath.ToSlash(relPath),
		Entries: items,
	})
	if err != nil {
		return model.ToolResult{}, fmt.Errorf("list_dir: encode output: %w", err)
	}
	return model.ToolResult{Output: string(output)}, nil
}

type ReadFileTool struct {
	maxBytes int64
}

func NewReadFileTool(maxBytes int64) *ReadFileTool {
	if maxBytes <= 0 {
		maxBytes = 1 << 20
	}
	return &ReadFileTool{maxBytes: maxBytes}
}

func (t *ReadFileTool) Name() string { return "read_file" }

func (t *ReadFileTool) Spec() model.ToolSpec {
	return model.ToolSpec{
		Name:            t.Name(),
		Description:     "Read one text file from the run directory or an explicit host path, optionally restricted to a line range.",
		InputSchemaJSON: `{"type":"object","properties":{"path":{"type":"string"},"start_line":{"type":"integer","minimum":1},"end_line":{"type":"integer","minimum":1},"max_bytes":{"type":"integer","minimum":1}},"required":["path"]}`,
		Risk:            model.RiskLow,
		SideEffect:      effectRead,
		Approval:        "never",
	}
}

func (t *ReadFileTool) Invoke(ctx context.Context, call model.ToolCall) (model.ToolResult, error) {
	root, err := cwdFromContext(ctx)
	if err != nil {
		return model.ToolResult{}, err
	}
	env := authorityFromContext(ctx)
	var input struct {
		Path      string `json:"path"`
		StartLine int    `json:"start_line"`
		EndLine   int    `json:"end_line"`
		MaxBytes  int64  `json:"max_bytes"`
	}
	if err := json.Unmarshal(call.InputJSON, &input); err != nil {
		return model.ToolResult{}, fmt.Errorf("read_file: decode input: %w", err)
	}
	absPath, relPath, err := resolveToolPath(root, input.Path, env)
	if err != nil {
		return model.ToolResult{}, fmt.Errorf("read_file: %w", err)
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return model.ToolResult{}, fmt.Errorf("read_file: read file: %w", err)
	}
	if strings.ContainsRune(string(data), 0) {
		return model.ToolResult{}, fmt.Errorf("read_file: binary file not supported")
	}
	lines := readTextLines(data)
	totalLines := len(lines)
	startLine := input.StartLine
	if startLine <= 0 {
		startLine = 1
	}
	endLine := input.EndLine
	if endLine <= 0 || endLine > totalLines {
		endLine = totalLines
	}
	if endLine < startLine {
		endLine = startLine
	}
	if totalLines == 0 {
		startLine = 0
		endLine = 0
	}
	var builder strings.Builder
	if startLine > 0 && totalLines > 0 {
		for _, line := range lines[startLine-1 : endLine] {
			builder.WriteString(line)
		}
	}
	content := builder.String()
	maxBytes := t.maxBytes
	if input.MaxBytes > 0 && input.MaxBytes < maxBytes {
		maxBytes = input.MaxBytes
	}
	truncated := false
	if int64(len(content)) > maxBytes {
		content = content[:maxBytes]
		truncated = true
	}

	output, err := json.Marshal(struct {
		Path       string `json:"path"`
		Content    string `json:"content"`
		Truncated  bool   `json:"truncated"`
		StartLine  int    `json:"start_line"`
		EndLine    int    `json:"end_line"`
		TotalLines int    `json:"total_lines"`
	}{
		Path:       filepath.ToSlash(relPath),
		Content:    content,
		Truncated:  truncated,
		StartLine:  startLine,
		EndLine:    endLine,
		TotalLines: totalLines,
	})
	if err != nil {
		return model.ToolResult{}, fmt.Errorf("read_file: encode output: %w", err)
	}
	return model.ToolResult{Output: string(output)}, nil
}

type GrepSearchTool struct {
	maxBytes int64
}

func NewGrepSearchTool(maxBytes int64) *GrepSearchTool {
	if maxBytes <= 0 {
		maxBytes = 1 << 20
	}
	return &GrepSearchTool{maxBytes: maxBytes}
}

func (t *GrepSearchTool) Name() string { return "grep_search" }

func (t *GrepSearchTool) Spec() model.ToolSpec {
	return model.ToolSpec{
		Name:            t.Name(),
		Description:     "Recursively search files under the run directory or an explicit host path for a substring and return matching lines.",
		InputSchemaJSON: `{"type":"object","properties":{"query":{"type":"string"},"path":{"type":"string"},"glob":{"type":"string"},"max_matches":{"type":"integer","minimum":1}},"required":["query"]}`,
		Risk:            model.RiskLow,
		SideEffect:      effectRead,
		Approval:        "never",
	}
}

func (t *GrepSearchTool) Invoke(ctx context.Context, call model.ToolCall) (model.ToolResult, error) {
	root, err := cwdFromContext(ctx)
	if err != nil {
		return model.ToolResult{}, err
	}
	env := authorityFromContext(ctx)
	var input struct {
		Query      string `json:"query"`
		Path       string `json:"path"`
		Glob       string `json:"glob"`
		MaxMatches int    `json:"max_matches"`
	}
	if err := json.Unmarshal(call.InputJSON, &input); err != nil {
		return model.ToolResult{}, fmt.Errorf("grep_search: decode input: %w", err)
	}
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return model.ToolResult{}, fmt.Errorf("grep_search: query is required")
	}
	absPath, relPath, err := resolveToolPath(root, input.Path, env)
	if err != nil {
		return model.ToolResult{}, fmt.Errorf("grep_search: %w", err)
	}
	maxMatches := input.MaxMatches
	if maxMatches <= 0 {
		maxMatches = 20
	}

	type match struct {
		Path       string `json:"path"`
		LineNumber int    `json:"line_number"`
		Line       string `json:"line"`
	}
	matches := make([]match, 0, maxMatches)
	err = filepath.WalkDir(absPath, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if len(matches) >= maxMatches {
			return filepath.SkipAll
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if input.Glob != "" {
			ok, err := filepath.Match(input.Glob, rel)
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.ContainsRune(string(data), 0) {
			return nil
		}
		if int64(len(data)) > t.maxBytes {
			data = data[:t.maxBytes]
		}
		lineNo := 0
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		for scanner.Scan() {
			lineNo++
			if strings.Contains(scanner.Text(), query) {
				matches = append(matches, match{
					Path:       rel,
					LineNumber: lineNo,
					Line:       scanner.Text(),
				})
				if len(matches) >= maxMatches {
					return filepath.SkipAll
				}
			}
		}
		return scanner.Err()
	})
	if err != nil && err != filepath.SkipAll {
		return model.ToolResult{}, fmt.Errorf("grep_search: walk: %w", err)
	}

	output, err := json.Marshal(struct {
		Query   string  `json:"query"`
		Path    string  `json:"path"`
		Matches []match `json:"matches"`
	}{
		Query:   query,
		Path:    filepath.ToSlash(relPath),
		Matches: matches,
	})
	if err != nil {
		return model.ToolResult{}, fmt.Errorf("grep_search: encode output: %w", err)
	}
	return model.ToolResult{Output: string(output)}, nil
}

type WriteNewFileTool struct{}

func NewWriteNewFileTool() *WriteNewFileTool { return &WriteNewFileTool{} }

func (t *WriteNewFileTool) Name() string { return "write_new_file" }

func (t *WriteNewFileTool) Spec() model.ToolSpec {
	return model.ToolSpec{
		Name:            t.Name(),
		Description:     "Create one new file under the run directory or an explicit host path and fail if it already exists.",
		InputSchemaJSON: `{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path","content"]}`,
		Risk:            model.RiskMedium,
		SideEffect:      effectCreate,
		Approval:        "required",
	}
}

func (t *WriteNewFileTool) Invoke(ctx context.Context, call model.ToolCall) (model.ToolResult, error) {
	root, err := cwdFromContext(ctx)
	if err != nil {
		return model.ToolResult{}, err
	}
	env := authorityFromContext(ctx)
	var input struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(call.InputJSON, &input); err != nil {
		return model.ToolResult{}, fmt.Errorf("write_new_file: decode input: %w", err)
	}
	absPath, relPath, err := resolveToolPath(root, input.Path, env)
	if err != nil {
		return model.ToolResult{}, fmt.Errorf("write_new_file: %w", err)
	}
	if _, err := os.Stat(absPath); err == nil {
		return model.ToolResult{}, fmt.Errorf("write_new_file: %s already exists", relPath)
	} else if !os.IsNotExist(err) {
		return model.ToolResult{}, fmt.Errorf("write_new_file: stat %s: %w", relPath, err)
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return model.ToolResult{}, fmt.Errorf("write_new_file: mkdir parent: %w", err)
	}
	if err := os.WriteFile(absPath, []byte(input.Content), 0o644); err != nil {
		return model.ToolResult{}, fmt.Errorf("write_new_file: write file: %w", err)
	}
	output, err := json.Marshal(struct {
		Path    string `json:"path"`
		Created bool   `json:"created"`
	}{
		Path:    filepath.ToSlash(relPath),
		Created: true,
	})
	if err != nil {
		return model.ToolResult{}, fmt.Errorf("write_new_file: encode output: %w", err)
	}
	return model.ToolResult{Output: string(output)}, nil
}

type DeletePathTool struct{}

func NewDeletePathTool() *DeletePathTool { return &DeletePathTool{} }

func (t *DeletePathTool) Name() string { return "delete_path" }

func (t *DeletePathTool) Spec() model.ToolSpec {
	return model.ToolSpec{
		Name:            t.Name(),
		Description:     "Delete one file or directory tree under the run directory or an explicit host path.",
		InputSchemaJSON: `{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`,
		Risk:            model.RiskHigh,
		SideEffect:      effectDelete,
		Approval:        "required",
	}
}

func (t *DeletePathTool) Invoke(ctx context.Context, call model.ToolCall) (model.ToolResult, error) {
	root, err := cwdFromContext(ctx)
	if err != nil {
		return model.ToolResult{}, err
	}
	env := authorityFromContext(ctx)
	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(call.InputJSON, &input); err != nil {
		return model.ToolResult{}, fmt.Errorf("delete_path: decode input: %w", err)
	}
	absPath, relPath, err := resolveToolPath(root, input.Path, env)
	if err != nil {
		return model.ToolResult{}, fmt.Errorf("delete_path: %w", err)
	}
	if err := os.RemoveAll(absPath); err != nil {
		return model.ToolResult{}, fmt.Errorf("delete_path: remove: %w", err)
	}
	output, err := json.Marshal(struct {
		Path    string `json:"path"`
		Deleted bool   `json:"deleted"`
	}{
		Path:    filepath.ToSlash(relPath),
		Deleted: true,
	})
	if err != nil {
		return model.ToolResult{}, fmt.Errorf("delete_path: encode output: %w", err)
	}
	return model.ToolResult{Output: string(output)}, nil
}

type MovePathTool struct{}

func NewMovePathTool() *MovePathTool { return &MovePathTool{} }

func (t *MovePathTool) Name() string { return "move_path" }

func (t *MovePathTool) Spec() model.ToolSpec {
	return model.ToolSpec{
		Name:            t.Name(),
		Description:     "Move or rename one file or directory under the run directory or an explicit host path.",
		InputSchemaJSON: `{"type":"object","properties":{"from":{"type":"string"},"to":{"type":"string"}},"required":["from","to"]}`,
		Risk:            model.RiskHigh,
		SideEffect:      effectMove,
		Approval:        "required",
	}
}

func (t *MovePathTool) Invoke(ctx context.Context, call model.ToolCall) (model.ToolResult, error) {
	root, err := cwdFromContext(ctx)
	if err != nil {
		return model.ToolResult{}, err
	}
	env := authorityFromContext(ctx)
	var input struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := json.Unmarshal(call.InputJSON, &input); err != nil {
		return model.ToolResult{}, fmt.Errorf("move_path: decode input: %w", err)
	}
	fromAbs, fromRel, err := resolveToolPath(root, input.From, env)
	if err != nil {
		return model.ToolResult{}, fmt.Errorf("move_path: from: %w", err)
	}
	toAbs, toRel, err := resolveToolPath(root, input.To, env)
	if err != nil {
		return model.ToolResult{}, fmt.Errorf("move_path: to: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(toAbs), 0o755); err != nil {
		return model.ToolResult{}, fmt.Errorf("move_path: mkdir target parent: %w", err)
	}
	if err := os.Rename(fromAbs, toAbs); err != nil {
		return model.ToolResult{}, fmt.Errorf("move_path: rename: %w", err)
	}
	output, err := json.Marshal(struct {
		From  string `json:"from"`
		To    string `json:"to"`
		Moved bool   `json:"moved"`
	}{
		From:  filepath.ToSlash(fromRel),
		To:    filepath.ToSlash(toRel),
		Moved: true,
	})
	if err != nil {
		return model.ToolResult{}, fmt.Errorf("move_path: encode output: %w", err)
	}
	return model.ToolResult{Output: string(output)}, nil
}

func readTextLines(data []byte) []string {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	lines := make([]string, 0, 32)
	for scanner.Scan() {
		lines = append(lines, scanner.Text()+"\n")
	}
	if len(data) > 0 && !strings.HasSuffix(string(data), "\n") && len(lines) > 0 {
		lines[len(lines)-1] = strings.TrimSuffix(lines[len(lines)-1], "\n")
	}
	if len(lines) == 0 && len(data) > 0 {
		lines = append(lines, string(data))
	}
	return lines
}
