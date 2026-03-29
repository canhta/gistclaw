package web

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/canhta/gistclaw/internal/memory"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/runtime"
)

type knowledgeIndexResponse struct {
	Notice   string                   `json:"notice,omitempty"`
	Headline string                   `json:"headline"`
	Filters  knowledgeFilterResponse  `json:"filters"`
	Summary  knowledgeSummaryResponse `json:"summary"`
	Items    []knowledgeItemResponse  `json:"items"`
	Paging   knowledgePagingResponse  `json:"paging"`
}

type knowledgeFilterResponse struct {
	Scope   string `json:"scope"`
	AgentID string `json:"agent_id"`
	Query   string `json:"query"`
	Limit   int    `json:"limit"`
}

type knowledgeSummaryResponse struct {
	VisibleCount int `json:"visible_count"`
}

type knowledgeItemResponse struct {
	ID             string  `json:"id"`
	AgentID        string  `json:"agent_id"`
	Scope          string  `json:"scope"`
	Content        string  `json:"content"`
	Source         string  `json:"source"`
	Provenance     string  `json:"provenance"`
	Confidence     float64 `json:"confidence"`
	CreatedAtLabel string  `json:"created_at_label"`
	UpdatedAtLabel string  `json:"updated_at_label"`
}

type knowledgePagingResponse struct {
	NextCursor string `json:"next_cursor,omitempty"`
	PrevCursor string `json:"prev_cursor,omitempty"`
	HasNext    bool   `json:"has_next"`
	HasPrev    bool   `json:"has_prev"`
}

type knowledgeEditRequest struct {
	Content string `json:"content"`
}

type knowledgeForgetResponse struct {
	ID        string `json:"id"`
	Forgotten bool   `json:"forgotten"`
}

func (s *Server) handleKnowledgeIndex(w http.ResponseWriter, r *http.Request) {
	if s.rt == nil {
		http.Error(w, "runtime not configured", http.StatusServiceUnavailable)
		return
	}

	scope := strings.TrimSpace(r.URL.Query().Get("scope"))
	agentID := strings.TrimSpace(r.URL.Query().Get("agent_id"))
	keyword := strings.TrimSpace(r.URL.Query().Get("q"))
	limit := requestNamedLimit(r, "limit", 20)
	project, err := runtime.ActiveProject(r.Context(), s.db)
	if err != nil {
		http.Error(w, "failed to load active project", http.StatusInternalServerError)
		return
	}
	if strings.TrimSpace(project.ID) == "" {
		writeJSON(w, http.StatusOK, emptyKnowledgeIndexResponse(
			scope,
			agentID,
			keyword,
			limit,
			"Choose an active project to load saved knowledge.",
		))
		return
	}

	page, err := s.rt.Memory().SearchPage(r.Context(), memory.SearchPageQuery{
		ProjectID: project.ID,
		AgentID:   agentID,
		Scope:     scope,
		Keyword:   keyword,
		Limit:     limit,
		Cursor:    strings.TrimSpace(r.URL.Query().Get("cursor")),
		Direction: strings.TrimSpace(r.URL.Query().Get("direction")),
	})
	if err != nil {
		http.Error(w, "failed to load knowledge surface", http.StatusInternalServerError)
		return
	}

	resp := knowledgeIndexResponse{
		Headline: buildKnowledgeHeadline(len(page.Items), scope, agentID, keyword),
		Filters: knowledgeFilterResponse{
			Scope:   scope,
			AgentID: agentID,
			Query:   keyword,
			Limit:   limit,
		},
		Summary: knowledgeSummaryResponse{
			VisibleCount: len(page.Items),
		},
		Items:  make([]knowledgeItemResponse, 0, len(page.Items)),
		Paging: knowledgePagingResponse{NextCursor: page.NextCursor, PrevCursor: page.PrevCursor, HasNext: page.HasNext, HasPrev: page.HasPrev},
	}
	for _, item := range page.Items {
		resp.Items = append(resp.Items, buildKnowledgeItemResponse(item))
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleKnowledgeEdit(w http.ResponseWriter, r *http.Request) {
	if s.rt == nil {
		http.Error(w, "runtime not configured", http.StatusServiceUnavailable)
		return
	}

	var req knowledgeEditRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		http.Error(w, "content is required", http.StatusBadRequest)
		return
	}

	project, err := runtime.ActiveProject(r.Context(), s.db)
	if err != nil {
		http.Error(w, "failed to load active project", http.StatusInternalServerError)
		return
	}
	if err := s.rt.Memory().Edit(r.Context(), project.ID, r.PathValue("id"), content); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "failed to edit knowledge item", http.StatusInternalServerError)
		return
	}

	item, err := s.rt.Memory().GetByID(r.Context(), project.ID, r.PathValue("id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "failed to reload knowledge item", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, buildKnowledgeItemResponse(item))
}

func (s *Server) handleKnowledgeForget(w http.ResponseWriter, r *http.Request) {
	if s.rt == nil {
		http.Error(w, "runtime not configured", http.StatusServiceUnavailable)
		return
	}

	project, err := runtime.ActiveProject(r.Context(), s.db)
	if err != nil {
		http.Error(w, "failed to load active project", http.StatusInternalServerError)
		return
	}
	if err := s.rt.Memory().Forget(r.Context(), project.ID, r.PathValue("id")); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "failed to forget knowledge item", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, knowledgeForgetResponse{
		ID:        r.PathValue("id"),
		Forgotten: true,
	})
}

func buildKnowledgeHeadline(itemCount int, scope, agentID, keyword string) string {
	switch {
	case itemCount == 0:
		return "No saved knowledge is shaping work yet."
	case scope != "" || agentID != "" || keyword != "":
		return "Filtered knowledge for the current project."
	default:
		return "Knowledge shaping future work in this project."
	}
}

func emptyKnowledgeIndexResponse(scope, agentID, keyword string, limit int, notice string) knowledgeIndexResponse {
	if limit <= 0 {
		limit = 20
	}

	return knowledgeIndexResponse{
		Notice:   notice,
		Headline: buildKnowledgeHeadline(0, scope, agentID, keyword),
		Filters: knowledgeFilterResponse{
			Scope:   scope,
			AgentID: agentID,
			Query:   keyword,
			Limit:   limit,
		},
		Summary: knowledgeSummaryResponse{
			VisibleCount: 0,
		},
		Items:  []knowledgeItemResponse{},
		Paging: knowledgePagingResponse{HasNext: false, HasPrev: false},
	}
}

func buildKnowledgeItemResponse(item model.MemoryItem) knowledgeItemResponse {
	return knowledgeItemResponse{
		ID:             item.ID,
		AgentID:        item.AgentID,
		Scope:          item.Scope,
		Content:        item.Content,
		Source:         item.Source,
		Provenance:     item.Provenance,
		Confidence:     item.Confidence,
		CreatedAtLabel: formatWebTimestamp(item.CreatedAt),
		UpdatedAtLabel: formatWebTimestamp(item.UpdatedAt),
	}
}
