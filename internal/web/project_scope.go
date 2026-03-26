package web

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/canhta/gistclaw/internal/projectscope"
	"github.com/canhta/gistclaw/internal/runtime"
)

func (s *Server) runVisibleInActiveProject(ctx context.Context, runID string) (bool, error) {
	project, err := runtime.ActiveProject(ctx, s.db)
	if err != nil {
		return false, err
	}
	condition, args := projectscope.RunCondition(project, "runs")
	queryArgs := append([]any{runID}, args...)
	query := "SELECT 1 FROM runs WHERE id = ? AND " + condition + " LIMIT 1"
	var sentinel int
	err = s.db.RawDB().QueryRowContext(ctx, query, queryArgs...).Scan(&sentinel)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load run visibility: %w", err)
	}
	return true, nil
}

func (s *Server) sessionVisibleInActiveProject(ctx context.Context, sessionID string) (bool, error) {
	project, err := runtime.ActiveProject(ctx, s.db)
	if err != nil {
		return false, err
	}
	condition, args := projectscope.RunCondition(project, "scope_runs")
	queryArgs := append([]any{sessionID}, args...)
	query := `SELECT 1
		FROM sessions sess
		WHERE sess.id = ?
		  AND EXISTS (
		      SELECT 1
		      FROM runs scope_runs
		      WHERE scope_runs.conversation_id = sess.conversation_id
		        AND ` + condition + `
		  )
		LIMIT 1`
	var sentinel int
	err = s.db.RawDB().QueryRowContext(ctx, query, queryArgs...).Scan(&sentinel)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load session visibility: %w", err)
	}
	return true, nil
}

func (s *Server) routeVisibleInActiveProject(ctx context.Context, routeID string) (bool, error) {
	project, err := runtime.ActiveProject(ctx, s.db)
	if err != nil {
		return false, err
	}
	condition, args := projectscope.RunCondition(project, "scope_runs")
	queryArgs := append([]any{routeID}, args...)
	query := `SELECT 1
		FROM session_bindings bind
		WHERE bind.id = ?
		  AND EXISTS (
		      SELECT 1
		      FROM runs scope_runs
		      WHERE scope_runs.conversation_id = bind.conversation_id
		        AND ` + condition + `
		  )
		LIMIT 1`
	var sentinel int
	err = s.db.RawDB().QueryRowContext(ctx, query, queryArgs...).Scan(&sentinel)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load route visibility: %w", err)
	}
	return true, nil
}

func (s *Server) deliveryVisibleInActiveProject(ctx context.Context, deliveryID string) (bool, error) {
	project, err := runtime.ActiveProject(ctx, s.db)
	if err != nil {
		return false, err
	}
	condition, args := projectscope.RunCondition(project, "runs")
	queryArgs := append([]any{deliveryID}, args...)
	query := `SELECT 1
		FROM outbound_intents oi
		JOIN runs ON runs.id = oi.run_id
		WHERE oi.id = ?
		  AND ` + condition + `
		LIMIT 1`
	var sentinel int
	err = s.db.RawDB().QueryRowContext(ctx, query, queryArgs...).Scan(&sentinel)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load delivery visibility: %w", err)
	}
	return true, nil
}

func (s *Server) approvalVisibleInActiveProject(ctx context.Context, approvalID string) (bool, error) {
	project, err := runtime.ActiveProject(ctx, s.db)
	if err != nil {
		return false, err
	}
	condition, args := projectscope.RunCondition(project, "runs")
	queryArgs := append([]any{approvalID}, args...)
	query := `SELECT 1
		FROM approvals
		JOIN runs ON runs.id = approvals.run_id
		WHERE approvals.id = ?
		  AND ` + condition + `
		LIMIT 1`
	var sentinel int
	err = s.db.RawDB().QueryRowContext(ctx, query, queryArgs...).Scan(&sentinel)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load approval visibility: %w", err)
	}
	return true, nil
}
