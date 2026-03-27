package web

import (
	"net/http"
	"strings"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/runtime"
)

type runSubmitPageData struct {
	Error string
	Task  string
}

func (s *Server) handleRunForm(w http.ResponseWriter, r *http.Request) {
	s.renderTemplate(w, r, "Start Task", "run_submit_body", runSubmitPageData{})
}

func (s *Server) handleRunSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	task := strings.TrimSpace(r.FormValue("task"))
	if task == "" {
		s.renderTemplate(w, r, "Start Task", "run_submit_body", runSubmitPageData{
			Error: "Task is required.",
		})
		return
	}

	if s.rt == nil {
		http.Error(w, "runtime not configured", http.StatusInternalServerError)
		return
	}

	activeProject, err := runtime.ActiveProject(r.Context(), s.db)
	if err != nil {
		http.Error(w, "failed to load active project", http.StatusInternalServerError)
		return
	}
	run, err := s.rt.ReceiveInboundMessageAsync(r.Context(), runtime.InboundMessageCommand{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "web",
			AccountID:   "local",
			ExternalID:  "assistant",
			ThreadID:    "main",
		},
		FrontAgentID: "assistant",
		Body:         task,
		ProjectID:    activeProject.ID,
		CWD:          activeProject.PrimaryPath,
	})
	if err != nil {
		http.Error(w, "failed to start run: "+err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, runDetailPath(run.ID), http.StatusSeeOther)
}
