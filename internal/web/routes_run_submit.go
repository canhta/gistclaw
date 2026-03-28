package web

import (
	"context"
	"fmt"
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

func (s *Server) startWorkRun(ctx context.Context, task string) (string, error) {
	if s.rt == nil {
		return "", fmt.Errorf("runtime not configured")
	}

	activeProject, err := runtime.ActiveProject(ctx, s.db)
	if err != nil {
		return "", err
	}
	run, err := s.rt.ReceiveInboundMessageAsync(ctx, runtime.InboundMessageCommand{
		ConversationKey: conversations.LocalWebConversationKey("", ""),
		Body:            task,
		ProjectID:       activeProject.ID,
		CWD:             activeProject.PrimaryPath,
	})
	if err != nil {
		return "", err
	}
	return run.ID, nil
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

	runID, err := s.startWorkRun(r.Context(), task)
	if err != nil {
		http.Error(w, "failed to start run: "+err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, runDetailPath(runID), http.StatusSeeOther)
}
