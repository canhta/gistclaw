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
	Error   string
	Confirm *model.MemoryItem // non-nil when showing forget confirmation
}

type memoryFilterForm struct {
	Scope   string
	AgentID string
}

func (s *Server) handleMemoryList(w http.ResponseWriter, r *http.Request) {
	if s.rt == nil {
		http.Error(w, "runtime not configured", http.StatusServiceUnavailable)
		return
	}

	scope := strings.TrimSpace(r.URL.Query().Get("scope"))
	agentID := strings.TrimSpace(r.URL.Query().Get("agent_id"))

	facts, err := s.rt.Memory().Filter(r.Context(), memory.MemoryFilter{
		Scope:   scope,
		AgentID: agentID,
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("load memory: %v", err), http.StatusInternalServerError)
		return
	}

	data := memoryPageData{
		Facts: facts,
		Filter: memoryFilterForm{
			Scope:   scope,
			AgentID: agentID,
		},
	}
	s.renderTemplate(w, "Memory", "memory_body", data)
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
		// Show confirmation view.
		facts, err := s.rt.Memory().Filter(r.Context(), memory.MemoryFilter{})
		if err != nil {
			http.Error(w, fmt.Sprintf("load memory: %v", err), http.StatusInternalServerError)
			return
		}
		var target *model.MemoryItem
		for i := range facts {
			if facts[i].ID == factID {
				target = &facts[i]
				break
			}
		}
		data := memoryPageData{Confirm: target}
		s.renderTemplate(w, "Memory", "memory_body", data)
		return
	}

	if err := s.rt.Memory().Forget(r.Context(), factID); err != nil {
		http.Error(w, fmt.Sprintf("forget: %v", err), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/memory", http.StatusSeeOther)
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

	http.Redirect(w, r, "/memory", http.StatusSeeOther)
}
