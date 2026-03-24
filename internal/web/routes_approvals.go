package web

import (
	"net/http"
	"time"
)

type approvalItem struct {
	ID         string
	RunID      string
	ToolName   string
	TargetPath string
	CreatedAt  time.Time
}

type approvalsPageData struct {
	Approvals []approvalItem
	Error     string
}

func (s *Server) handleApprovals(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.RawDB().QueryContext(r.Context(),
		`SELECT id, run_id, tool_name, COALESCE(target_path, ''), created_at
		 FROM approvals WHERE status = 'pending'
		 ORDER BY created_at DESC`,
	)
	if err != nil {
		http.Error(w, "failed to load approvals", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	items := make([]approvalItem, 0)
	for rows.Next() {
		var item approvalItem
		if err := rows.Scan(&item.ID, &item.RunID, &item.ToolName, &item.TargetPath, &item.CreatedAt); err != nil {
			http.Error(w, "failed to load approvals", http.StatusInternalServerError)
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "failed to load approvals", http.StatusInternalServerError)
		return
	}

	s.renderTemplate(w, "Approvals", "approvals_body", approvalsPageData{Approvals: items})
}

func (s *Server) handleApprovalResolve(w http.ResponseWriter, r *http.Request) {
	ticketID := r.PathValue("id")

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	decision := r.FormValue("decision")
	if decision != "approved" && decision != "denied" {
		http.Error(w, "decision must be approved or denied", http.StatusBadRequest)
		return
	}

	if s.rt == nil {
		http.Error(w, "runtime not configured", http.StatusInternalServerError)
		return
	}

	if err := s.rt.ResolveApproval(r.Context(), ticketID, decision); err != nil {
		s.renderTemplate(w, "Approvals", "approvals_body", approvalsPageData{
			Error: err.Error(),
		})
		return
	}

	http.Redirect(w, r, "/approvals", http.StatusSeeOther)
}
