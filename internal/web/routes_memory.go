package web

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/canhta/gistclaw/internal/memory"
	"github.com/canhta/gistclaw/internal/model"
)

type memoryPageData struct {
	Facts   []model.MemoryItem
	Filter  memoryFilterForm
	Paging  pageLinks
	Error   string
	Confirm *model.MemoryItem // non-nil when showing forget confirmation
}

type memoryFilterForm struct {
	Scope   string
	AgentID string
	Query   string
	Limit   int
}

func (s *Server) handleMemoryList(w http.ResponseWriter, r *http.Request) {
	if s.rt == nil {
		http.Error(w, "runtime not configured", http.StatusServiceUnavailable)
		return
	}

	scope := strings.TrimSpace(r.URL.Query().Get("scope"))
	agentID := strings.TrimSpace(r.URL.Query().Get("agent_id"))
	keyword := strings.TrimSpace(r.URL.Query().Get("q"))
	limit := requestNamedLimit(r, "limit", 20)

	page, err := s.rt.Memory().SearchPage(r.Context(), memory.SearchPageQuery{
		Scope:     scope,
		AgentID:   agentID,
		Keyword:   keyword,
		Limit:     limit,
		Cursor:    strings.TrimSpace(r.URL.Query().Get("cursor")),
		Direction: strings.TrimSpace(r.URL.Query().Get("direction")),
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("load memory: %v", err), http.StatusInternalServerError)
		return
	}

	data := memoryPageData{
		Facts: page.Items,
		Filter: memoryFilterForm{
			Scope:   scope,
			AgentID: agentID,
			Query:   keyword,
			Limit:   limit,
		},
		Paging: buildPageLinks(pageConfigureMemory, cloneQuery(r.URL.Query()), "cursor", "direction", page.NextCursor, page.PrevCursor, page.HasNext, page.HasPrev),
	}
	s.renderTemplate(w, r, "Memory", "memory_body", data)
}

func (s *Server) handleMemoryForget(w http.ResponseWriter, r *http.Request) {
	if s.rt == nil {
		http.Error(w, "runtime not configured", http.StatusServiceUnavailable)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	factID := r.PathValue("id")
	confirm := r.FormValue("confirm")

	if confirm != "yes" {
		// Show confirmation view — use targeted GetByID instead of a full scan.
		item, err := s.rt.Memory().GetByID(r.Context(), factID)
		if err != nil {
			http.Error(w, fmt.Sprintf("load memory item: %v", err), http.StatusInternalServerError)
			return
		}
		data := memoryPageData{Confirm: &item}
		s.renderTemplate(w, r, "Memory", "memory_body", data)
		return
	}

	if err := s.rt.Memory().Forget(r.Context(), factID); err != nil {
		http.Error(w, fmt.Sprintf("forget: %v", err), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, pageConfigureMemory, http.StatusSeeOther)
}

func (s *Server) handleMemoryEdit(w http.ResponseWriter, r *http.Request) {
	if s.rt == nil {
		http.Error(w, "runtime not configured", http.StatusServiceUnavailable)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	factID := r.PathValue("id")
	value := r.FormValue("value")

	if err := s.rt.Memory().Edit(r.Context(), factID, value); err != nil {
		http.Error(w, fmt.Sprintf("edit: %v", err), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, pageConfigureMemory, http.StatusSeeOther)
}
