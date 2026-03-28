package web

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/canhta/gistclaw/internal/runtime"
)

type onboardingResponse struct {
	Completed      bool                          `json:"completed"`
	EntryHref      string                        `json:"entry_href"`
	Project        *bootstrapProjectResponse     `json:"project"`
	SuggestedTasks []onboardingTaskCandidateView `json:"suggested_tasks"`
}

type onboardingTaskCandidateView struct {
	Kind        string `json:"kind"`
	Description string `json:"description"`
	Signal      string `json:"signal"`
}

type onboardingProjectRequest struct {
	Source      string `json:"source"`
	ProjectPath string `json:"project_path,omitempty"`
}

type onboardingPreviewRequest struct {
	Task string `json:"task"`
}

type onboardingPreviewResponse struct {
	RunID    string `json:"run_id"`
	NextHref string `json:"next_href"`
}

func (s *Server) handleOnboardingAPI(w http.ResponseWriter, r *http.Request) {
	resp, err := s.loadOnboardingResponse(r)
	if err != nil {
		http.Error(w, "failed to load onboarding state", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleOnboardingProjectAPI(w http.ResponseWriter, r *http.Request) {
	var req onboardingProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid JSON body"})
		return
	}

	switch strings.TrimSpace(req.Source) {
	case "starter":
		project, err := runtime.ActiveProject(r.Context(), s.db)
		if err != nil {
			http.Error(w, "failed to load starter project", http.StatusInternalServerError)
			return
		}
		if strings.TrimSpace(project.PrimaryPath) == "" {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"message": "starter project is not ready yet"})
			return
		}
	case "new_project":
		projectPath := strings.TrimSpace(req.ProjectPath)
		if errMsg := validateNewProjectPath(projectPath); errMsg != "" {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"message": errMsg})
			return
		}
		if err := ensureNewProjectPath(projectPath); err != nil {
			http.Error(w, "failed to create project directory", http.StatusInternalServerError)
			return
		}
		if _, err := runtime.ActivateProjectPath(r.Context(), s.db, projectPath, "", "operator"); err != nil {
			http.Error(w, "failed to save project", http.StatusInternalServerError)
			return
		}
	case "existing_repo", "":
		projectPath := strings.TrimSpace(req.ProjectPath)
		if projectPath == "" {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"message": "project path is required"})
			return
		}
		if errMsg := validateProjectPath(projectPath); errMsg != "" {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"message": errMsg})
			return
		}
		if _, err := runtime.ActivateProjectPath(r.Context(), s.db, projectPath, "", "operator"); err != nil {
			http.Error(w, "failed to save project", http.StatusInternalServerError)
			return
		}
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "unknown onboarding project source"})
		return
	}

	if err := markOnboardingCompleted(r.Context(), s.db); err != nil {
		http.Error(w, "failed to save onboarding state", http.StatusInternalServerError)
		return
	}

	resp, err := s.loadOnboardingResponse(r)
	if err != nil {
		http.Error(w, "failed to load onboarding state", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleOnboardingPreviewAPI(w http.ResponseWriter, r *http.Request) {
	var req onboardingPreviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid JSON body"})
		return
	}

	task := strings.TrimSpace(req.Task)
	if task == "" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"message": "Choose a preview task before starting the run."})
		return
	}
	if s.rt == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"message": "Preview runs are unavailable right now. Check the runtime configuration and try again."})
		return
	}

	project, err := runtime.ActiveProject(r.Context(), s.db)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"message": "Unable to load the active project. Check the runtime configuration and try again."})
		return
	}
	if strings.TrimSpace(project.ID) == "" || strings.TrimSpace(project.PrimaryPath) == "" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"message": "Bind a project before starting a preview run."})
		return
	}

	frontAgentID, err := s.rt.FrontAgentID(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"message": "Unable to resolve the front assistant for preview runs. Check the team configuration and try again."})
		return
	}
	run, err := s.rt.StartAsync(r.Context(), runtime.StartRun{
		ConversationID: "onboarding",
		AgentID:        frontAgentID,
		Objective:      task,
		ProjectID:      project.ID,
		CWD:            project.PrimaryPath,
		AccountID:      "local",
		PreviewOnly:    true,
	})
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"message": onboardingPreviewStartError(err)})
		return
	}

	writeJSON(w, http.StatusAccepted, onboardingPreviewResponse{
		RunID:    run.ID,
		NextHref: workPagePath(run.ID),
	})
}

func (s *Server) loadOnboardingResponse(r *http.Request) (onboardingResponse, error) {
	project, err := runtime.ActiveProject(r.Context(), s.db)
	if err != nil {
		return onboardingResponse{}, err
	}

	resp := onboardingResponse{
		Completed: onboardingCompleted(s.db),
		EntryHref: s.defaultEntryPath(),
		Project:   bootstrapProjectPointer(project),
	}
	if strings.TrimSpace(project.PrimaryPath) == "" {
		return resp, nil
	}

	candidates := scanRepoSignals(project.PrimaryPath)
	resp.SuggestedTasks = make([]onboardingTaskCandidateView, 0, len(candidates))
	for _, candidate := range candidates {
		resp.SuggestedTasks = append(resp.SuggestedTasks, onboardingTaskCandidateView(candidate))
	}
	return resp, nil
}
