package web

import (
	"net/http"
	"net/url"
	"strconv"
)

type pageLinks struct {
	NextURL string
	PrevURL string
	HasNext bool
	HasPrev bool
}

func requestNamedLimit(r *http.Request, key string, fallback int) int {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func cloneQuery(values url.Values) url.Values {
	cloned := make(url.Values, len(values))
	for key, vals := range values {
		copied := make([]string, len(vals))
		copy(copied, vals)
		cloned[key] = copied
	}
	return cloned
}

func buildPageLinks(basePath string, query url.Values, cursorKey, directionKey string, nextCursor, prevCursor string, hasNext, hasPrev bool) pageLinks {
	links := pageLinks{
		HasNext: hasNext,
		HasPrev: hasPrev,
	}

	if hasNext && nextCursor != "" {
		nextQuery := cloneQuery(query)
		nextQuery.Set(cursorKey, nextCursor)
		nextQuery.Set(directionKey, "next")
		links.NextURL = basePath + "?" + nextQuery.Encode()
	}
	if hasPrev && prevCursor != "" {
		prevQuery := cloneQuery(query)
		prevQuery.Set(cursorKey, prevCursor)
		prevQuery.Set(directionKey, "prev")
		links.PrevURL = basePath + "?" + prevQuery.Encode()
	}

	return links
}
