package logstream

import (
	"context"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	LevelAll   = "all"
	LevelInfo  = "info"
	LevelWarn  = "warn"
	LevelError = "error"
)

type Entry struct {
	ID        int64
	CreatedAt time.Time
	Source    string
	Level     string
	Message   string
	Raw       string
}

type Query struct {
	Query  string
	Level  string
	Source string
	Limit  int
}

type Snapshot struct {
	BufferedEntries int
	Sources         []string
	Entries         []Entry
	Query           Query
}

type Sink struct {
	mu          sync.RWMutex
	maxEntries  int
	nextID      int64
	buffer      []byte
	entries     []Entry
	subscribers map[chan Entry]Query
}

func New(maxEntries int) *Sink {
	if maxEntries <= 0 {
		maxEntries = 500
	}
	return &Sink{
		maxEntries:  maxEntries,
		subscribers: make(map[chan Entry]Query),
	}
}

func (s *Sink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.buffer = append(s.buffer, p...)
	for {
		idx := bytesIndexByte(s.buffer, '\n')
		if idx < 0 {
			break
		}
		line := strings.TrimSpace(string(s.buffer[:idx]))
		s.buffer = s.buffer[idx+1:]
		if line == "" {
			continue
		}
		s.appendLocked(parseLine(line))
	}

	return len(p), nil
}

func (s *Sink) Append(source, level, message string) Entry {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry := Entry{
		CreatedAt: time.Now().UTC(),
		Source:    normalizeSource(source),
		Level:     normalizeLevel(level),
		Message:   strings.TrimSpace(message),
		Raw:       strings.TrimSpace(strings.Join([]string{normalizeSource(source), normalizeLevel(level), strings.TrimSpace(message)}, " ")),
	}
	s.appendLocked(entry)
	return entry
}

func (s *Sink) Emit(_ context.Context, source, level, message string) error {
	s.Append(source, level, message)
	return nil
}

func (s *Sink) Snapshot(query Query) Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	normalized := normalizeQuery(query)
	filtered := filterEntries(s.entries, normalized)
	if len(filtered) > normalized.Limit {
		filtered = append([]Entry(nil), filtered[len(filtered)-normalized.Limit:]...)
	} else {
		filtered = append([]Entry(nil), filtered...)
	}

	sources := make([]string, 0, len(s.entries))
	seen := make(map[string]struct{}, len(s.entries))
	for _, entry := range s.entries {
		if _, ok := seen[entry.Source]; ok {
			continue
		}
		seen[entry.Source] = struct{}{}
		sources = append(sources, entry.Source)
	}
	sort.Strings(sources)

	return Snapshot{
		BufferedEntries: len(s.entries),
		Sources:         sources,
		Entries:         filtered,
		Query:           normalized,
	}
}

func (s *Sink) Subscribe(query Query) chan Entry {
	ch := make(chan Entry, 16)

	s.mu.Lock()
	s.subscribers[ch] = normalizeQuery(query)
	s.mu.Unlock()

	return ch
}

func (s *Sink) Unsubscribe(target chan Entry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.subscribers, target)
}

func (s *Sink) SubscriberCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.subscribers)
}

func (s *Sink) appendLocked(entry Entry) {
	entry.Source = normalizeSource(entry.Source)
	entry.Level = normalizeLevel(entry.Level)
	entry.Message = strings.TrimSpace(entry.Message)
	entry.Raw = strings.TrimSpace(entry.Raw)
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	s.nextID++
	entry.ID = s.nextID
	if entry.Raw == "" {
		entry.Raw = strings.TrimSpace(strings.Join([]string{entry.Source, entry.Level, entry.Message}, " "))
	}

	s.entries = append(s.entries, entry)
	if len(s.entries) > s.maxEntries {
		s.entries = append([]Entry(nil), s.entries[len(s.entries)-s.maxEntries:]...)
	}

	for ch, query := range s.subscribers {
		if !matches(entry, query) {
			continue
		}
		select {
		case ch <- entry:
		default:
		}
	}
}

func normalizeQuery(query Query) Query {
	normalized := Query{
		Query:  strings.TrimSpace(query.Query),
		Level:  strings.TrimSpace(strings.ToLower(query.Level)),
		Source: normalizeSource(query.Source),
		Limit:  query.Limit,
	}
	if normalized.Level == "" {
		normalized.Level = LevelAll
	} else {
		switch normalized.Level {
		case LevelAll, LevelInfo, LevelWarn, LevelError:
		default:
			normalized.Level = LevelAll
		}
	}
	if normalized.Source == "" {
		normalized.Source = "all"
	}
	if normalized.Limit <= 0 {
		normalized.Limit = 200
	}
	if normalized.Limit > 500 {
		normalized.Limit = 500
	}
	return normalized
}

func filterEntries(entries []Entry, query Query) []Entry {
	filtered := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		if matches(entry, query) {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func matches(entry Entry, query Query) bool {
	if query.Level != "" && query.Level != LevelAll && entry.Level != query.Level {
		return false
	}
	if query.Source != "" && query.Source != "all" && entry.Source != query.Source {
		return false
	}
	if query.Query == "" {
		return true
	}
	needle := strings.ToLower(query.Query)
	haystack := strings.ToLower(entry.Message + "\n" + entry.Raw)
	return strings.Contains(haystack, needle)
}

func parseLine(line string) Entry {
	trimmed := strings.TrimSpace(stripTimestampPrefix(line))
	if trimmed == "" {
		return Entry{
			Source:  "runtime",
			Level:   LevelInfo,
			Message: "",
			Raw:     strings.TrimSpace(line),
		}
	}

	parts := strings.Fields(trimmed)
	entry := Entry{
		Source:  "runtime",
		Level:   inferLevel(trimmed),
		Message: trimmed,
		Raw:     strings.TrimSpace(line),
	}
	if len(parts) >= 1 {
		source := normalizeSource(parts[0])
		if source != "all" && source != "" && source != LevelAll && source != LevelInfo && source != LevelWarn && source != LevelError {
			entry.Source = source
			if len(parts) >= 2 {
				level := normalizeLevel(parts[1])
				if level != LevelAll && level != "" {
					entry.Level = level
					entry.Message = strings.TrimSpace(strings.Join(parts[2:], " "))
					return entry
				}
			}
			entry.Message = strings.TrimSpace(strings.Join(parts[1:], " "))
		}
	}
	return entry
}

func normalizeSource(source string) string {
	value := strings.TrimSpace(strings.ToLower(source))
	if value == "" {
		return ""
	}
	switch value {
	case "all":
		return "all"
	case "web", "runtime", "scheduler", "connector":
		return value
	default:
		return value
	}
}

func normalizeLevel(level string) string {
	switch strings.TrimSpace(strings.ToLower(level)) {
	case "", LevelInfo:
		return LevelInfo
	case LevelWarn, "warning":
		return LevelWarn
	case LevelError, "panic", "failed":
		return LevelError
	case LevelAll:
		return LevelAll
	default:
		return LevelInfo
	}
}

func inferLevel(message string) string {
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, " panic "), strings.HasPrefix(lower, "panic "), strings.Contains(lower, " error "), strings.Contains(lower, " failed"), strings.HasSuffix(lower, " failed"):
		return LevelError
	case strings.Contains(lower, " warning"), strings.Contains(lower, " warn "), strings.HasPrefix(lower, "warn "):
		return LevelWarn
	default:
		return LevelInfo
	}
}

func stripTimestampPrefix(line string) string {
	const pattern = "2006/01/02 15:04:05 "
	if len(line) < len(pattern) {
		return line
	}
	for i := 0; i < len(pattern); i++ {
		switch {
		case i < 4 || (i > 4 && i < 7) || (i > 7 && i < 10) || (i > 10 && i < 13) || (i > 13 && i < 16) || (i > 16 && i < 19):
			if line[i] < '0' || line[i] > '9' {
				return line
			}
		case line[i] != pattern[i]:
			return line
		}
	}
	return line[len(pattern):]
}

func bytesIndexByte(buf []byte, target byte) int {
	for i, b := range buf {
		if b == target {
			return i
		}
	}
	return -1
}

var _ io.Writer = (*Sink)(nil)
