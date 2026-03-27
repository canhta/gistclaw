package sessions

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/projectscope"
	"github.com/canhta/gistclaw/internal/store"
)

type Service struct {
	db   *store.DB
	conv *conversations.ConversationStore
}

var ErrThreadMailboxNotFound = fmt.Errorf("sessions: no active session bound to thread")
var ErrSessionNotFound = fmt.Errorf("sessions: session not found")
var ErrSessionRouteNotFound = fmt.Errorf("sessions: no active route bound to session")
var ErrOutboundIntentNotFound = fmt.Errorf("sessions: outbound intent not found")

type OpenFrontSession struct {
	ConversationID string
	AgentID        string
}

type SpawnWorkerSession struct {
	ConversationID      string
	ParentSessionID     string
	ControllerSessionID string
	AgentID             string
	InitialPrompt       string
}

type BindFollowUp struct {
	ConversationID string
	ThreadID       string
	SessionID      string
	ConnectorID    string
	AccountID      string
	ExternalID     string
}

type DeliveryQueueFilter struct {
	ProjectID   string
	ConnectorID string
	SessionID   string
	Status      string
	Query       string
	Cursor      string
	Direction   string
	Limit       int
}

type RouteListFilter struct {
	ProjectID   string
	ConnectorID string
	Status      string
	Query       string
	Cursor      string
	Direction   string
	Limit       int
}

type SessionListFilter struct {
	ProjectID      string
	ConversationID string
	AgentID        string
	Role           string
	Status         string
	ConnectorID    string
	Query          string
	Binding        string
	Cursor         string
	Direction      string
	Limit          int
}

func NewService(db *store.DB, conv *conversations.ConversationStore) *Service {
	return &Service{db: db, conv: conv}
}

func (s *Service) OpenFrontSession(ctx context.Context, cmd OpenFrontSession) (model.Session, error) {
	now := time.Now().UTC()
	session := model.Session{
		ID:             generateID(),
		ConversationID: cmd.ConversationID,
		Key:            BuildFrontSessionKey(cmd.ConversationID),
		AgentID:        cmd.AgentID,
		Role:           model.SessionRoleFront,
		Status:         "active",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := s.appendSessionOpened(ctx, "", session); err != nil {
		return model.Session{}, fmt.Errorf("open front session: %w", err)
	}

	return session, nil
}

func (s *Service) SpawnWorkerSession(ctx context.Context, cmd SpawnWorkerSession) (model.Session, error) {
	now := time.Now().UTC()
	sessionID := generateID()
	session := model.Session{
		ID:                  sessionID,
		ConversationID:      cmd.ConversationID,
		Key:                 BuildWorkerSessionKey(cmd.ParentSessionID, cmd.AgentID, sessionID),
		AgentID:             cmd.AgentID,
		Role:                model.SessionRoleWorker,
		ParentSessionID:     cmd.ParentSessionID,
		ControllerSessionID: cmd.ControllerSessionID,
		Status:              "active",
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	if err := s.appendSessionOpened(ctx, "", session); err != nil {
		return model.Session{}, fmt.Errorf("spawn worker session: %w", err)
	}

	if err := s.AppendMessage(ctx, model.SessionMessage{
		ID:              generateID(),
		SessionID:       session.ID,
		SenderSessionID: cmd.ControllerSessionID,
		Kind:            model.MessageSpawn,
		Body:            cmd.InitialPrompt,
		Provenance: model.SessionMessageProvenance{
			Kind:            model.MessageProvenanceInterSession,
			SourceSessionID: cmd.ControllerSessionID,
		},
		CreatedAt: now,
	}); err != nil {
		return model.Session{}, err
	}

	return session, nil
}

func (s *Service) AppendMessage(ctx context.Context, msg model.SessionMessage) error {
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now().UTC()
	}
	if msg.ID == "" {
		msg.ID = generateID()
	}

	if err := s.appendSessionMessage(ctx, "", msg); err != nil {
		return fmt.Errorf("append session message: %w", err)
	}
	return nil
}

func (s *Service) BindFollowUp(ctx context.Context, cmd BindFollowUp) error {
	if err := s.appendSessionBinding(ctx, "", cmd); err != nil {
		return fmt.Errorf("bind follow up: %w", err)
	}
	return nil
}

func (s *Service) LoadThreadMailbox(
	ctx context.Context,
	conversationID string,
	threadID string,
	limit int,
) (model.Session, []model.SessionMessage, error) {
	session, err := s.loadBoundSession(ctx, conversationID, normalizeThreadID(threadID))
	if err != nil {
		return model.Session{}, nil, err
	}

	messages, err := s.listSessionMessages(ctx, session.ID, limit)
	if err != nil {
		return model.Session{}, nil, err
	}

	return session, messages, nil
}

func (s *Service) LoadSessionMailbox(
	ctx context.Context,
	sessionID string,
	limit int,
) (model.Session, []model.SessionMessage, error) {
	session, err := s.loadSession(ctx, sessionID)
	if err != nil {
		return model.Session{}, nil, err
	}
	messages, err := s.listSessionMessages(ctx, session.ID, limit)
	if err != nil {
		return model.Session{}, nil, err
	}
	return session, messages, nil
}

func (s *Service) LoadSession(ctx context.Context, sessionID string) (model.Session, error) {
	session, err := s.loadSession(ctx, sessionID)
	if err != nil {
		return model.Session{}, err
	}
	return session, nil
}

func (s *Service) LoadActiveRunRef(ctx context.Context, sessionID string) (model.RunRef, error) {
	var ref model.RunRef
	var status string
	err := s.db.RawDB().QueryRowContext(ctx,
		`SELECT id, status
		 FROM runs
		 WHERE session_id = ?
		   AND status IN ('pending', 'active', 'needs_approval')
		 ORDER BY updated_at DESC, created_at DESC, id DESC
		 LIMIT 1`,
		sessionID,
	).Scan(&ref.ID, &status)
	if err == sql.ErrNoRows {
		return model.RunRef{}, nil
	}
	if err != nil {
		return model.RunRef{}, fmt.Errorf("load active session run: %w", err)
	}
	ref.Status = model.RunStatus(status)
	return ref, nil
}

func (s *Service) ListConversationSessions(ctx context.Context, conversationID string, limit int) ([]model.Session, error) {
	return s.ListSessions(ctx, SessionListFilter{
		ConversationID: conversationID,
		Limit:          limit,
	})
}

func (s *Service) ListSessions(ctx context.Context, filter SessionListFilter) ([]model.Session, error) {
	page, err := s.ListSessionsPage(ctx, filter)
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

func (s *Service) ListSessionsPage(ctx context.Context, filter SessionListFilter) (PageResult[model.Session], error) {
	if filter.Limit <= 0 {
		filter.Limit = 100
	}
	filter.Binding = normalizeSessionBinding(filter.Binding)

	createdMicrosExpr := sqliteMicros("sess.created_at")
	updatedMicrosExpr := "COALESCE(activity.updated_at_micros, " + createdMicrosExpr + ")"
	roleRankExpr := `CASE sess.role WHEN 'front' THEN 0 ELSE 1 END`
	orderUpdated := "DESC"
	orderRole := "ASC"
	orderCreated := "DESC"
	orderID := "DESC"
	reverseResults := false

	query := `SELECT sess.id, sess.conversation_id, sess.key, sess.agent_id, sess.role,
		        COALESCE(sess.parent_session_id, ''), COALESCE(sess.controller_session_id, ''),
		        sess.status, sess.created_at,
		        ` + updatedMicrosExpr + ` AS updated_at_micros,
		        ` + roleRankExpr + ` AS role_rank,
		        ` + createdMicrosExpr + ` AS created_at_micros
		 FROM sessions sess
		 LEFT JOIN (
		     SELECT session_id, MAX(activity_at_micros) AS updated_at_micros
		     FROM (
		         SELECT session_id, ` + sqliteMicros("created_at") + ` AS activity_at_micros
		         FROM session_messages
		         UNION ALL
		         SELECT session_id, ` + sqliteMicros("updated_at") + ` AS activity_at_micros
		         FROM runs
		         WHERE session_id IS NOT NULL AND session_id != ''
		     )
		     GROUP BY session_id
		 ) activity ON activity.session_id = sess.id
		 WHERE 1 = 1`
	args := make([]any, 0, 24)
	if filter.ProjectID != "" {
		scopeCondition, scopeArgs := projectscope.RunCondition(model.Project{
			ID: filter.ProjectID,
		}, "scope_runs")
		query += `
		   AND EXISTS (
		       SELECT 1
		       FROM runs scope_runs
		       WHERE scope_runs.conversation_id = sess.conversation_id
		         AND ` + scopeCondition + `
		   )`
		args = append(args, scopeArgs...)
	}
	if filter.ConversationID != "" {
		query += `
		   AND sess.conversation_id = ?`
		args = append(args, filter.ConversationID)
	}
	if filter.AgentID != "" {
		query += `
		   AND sess.agent_id = ?`
		args = append(args, filter.AgentID)
	}
	if filter.Role != "" {
		query += `
		   AND sess.role = ?`
		args = append(args, filter.Role)
	}
	if filter.Status != "" {
		query += `
		   AND sess.status = ?`
		args = append(args, filter.Status)
	}
	switch {
	case filter.Binding == "unbound":
		query += `
		   AND NOT EXISTS (
		       SELECT 1
		       FROM session_bindings bind
		       WHERE bind.session_id = sess.id
		         AND bind.status = 'active'
		   )`
		if filter.ConnectorID != "" {
			query += `
		   AND 1 = 0`
		}
	case filter.Binding == "bound" || filter.ConnectorID != "":
		query += `
		   AND EXISTS (
		       SELECT 1
		       FROM session_bindings bind
		       WHERE bind.session_id = sess.id
		         AND bind.status = 'active'`
		if filter.ConnectorID != "" {
			query += `
		         AND bind.connector_id = ?`
			args = append(args, filter.ConnectorID)
		}
		query += `
		   )`
	}
	if filter.Query != "" {
		q := "%" + strings.ToLower(strings.TrimSpace(filter.Query)) + "%"
		query += `
		   AND (
		       LOWER(sess.id) LIKE ?
		       OR LOWER(sess.conversation_id) LIKE ?
		       OR LOWER(sess.key) LIKE ?
		       OR LOWER(sess.agent_id) LIKE ?
		       OR EXISTS (
		           SELECT 1
		           FROM session_bindings bind
		           WHERE bind.session_id = sess.id
		             AND bind.status = 'active'
		             AND (
		                 LOWER(bind.id) LIKE ?
		                 OR LOWER(bind.connector_id) LIKE ?
		                 OR LOWER(bind.external_id) LIKE ?
		                 OR LOWER(bind.thread_id) LIKE ?
		             )
		       )
		   )`
		args = append(args, q, q, q, q, q, q, q, q)
	}

	direction := normalizePageDirection(filter.Direction)
	if strings.TrimSpace(filter.Cursor) != "" {
		var cursor sessionPageCursor
		if err := decodePageCursor(filter.Cursor, &cursor); err != nil {
			return PageResult[model.Session]{}, fmt.Errorf("list sessions page: %w", err)
		}
		if direction == "prev" {
			query += `
		   AND (
		       ` + updatedMicrosExpr + ` > ?
		       OR (` + updatedMicrosExpr + ` = ? AND ` + roleRankExpr + ` < ?)
		       OR (` + updatedMicrosExpr + ` = ? AND ` + roleRankExpr + ` = ? AND ` + createdMicrosExpr + ` > ?)
		       OR (` + updatedMicrosExpr + ` = ? AND ` + roleRankExpr + ` = ? AND ` + createdMicrosExpr + ` = ? AND sess.id > ?)
		   )`
			args = append(args,
				cursor.UpdatedAtMicros,
				cursor.UpdatedAtMicros, cursor.RoleRank,
				cursor.UpdatedAtMicros, cursor.RoleRank, cursor.CreatedAtMicros,
				cursor.UpdatedAtMicros, cursor.RoleRank, cursor.CreatedAtMicros, cursor.ID,
			)
			orderUpdated = "ASC"
			orderRole = "DESC"
			orderCreated = "ASC"
			orderID = "ASC"
			reverseResults = true
		} else {
			query += `
		   AND (
		       ` + updatedMicrosExpr + ` < ?
		       OR (` + updatedMicrosExpr + ` = ? AND ` + roleRankExpr + ` > ?)
		       OR (` + updatedMicrosExpr + ` = ? AND ` + roleRankExpr + ` = ? AND ` + createdMicrosExpr + ` < ?)
		       OR (` + updatedMicrosExpr + ` = ? AND ` + roleRankExpr + ` = ? AND ` + createdMicrosExpr + ` = ? AND sess.id < ?)
		   )`
			args = append(args,
				cursor.UpdatedAtMicros,
				cursor.UpdatedAtMicros, cursor.RoleRank,
				cursor.UpdatedAtMicros, cursor.RoleRank, cursor.CreatedAtMicros,
				cursor.UpdatedAtMicros, cursor.RoleRank, cursor.CreatedAtMicros, cursor.ID,
			)
		}
	}

	query += `
		 ORDER BY ` + updatedMicrosExpr + ` ` + orderUpdated + `,
		          ` + roleRankExpr + ` ` + orderRole + `,
		          ` + createdMicrosExpr + ` ` + orderCreated + `,
		          sess.id ` + orderID + `
		 LIMIT ?`
	args = append(args, filter.Limit+1)

	rows, err := s.db.RawDB().QueryContext(ctx, query, args...)
	if err != nil {
		return PageResult[model.Session]{}, fmt.Errorf("list sessions page: %w", err)
	}
	defer rows.Close()

	type sessionRow struct {
		session model.Session
		cursor  sessionPageCursor
	}

	list := make([]sessionRow, 0, filter.Limit+1)
	for rows.Next() {
		var session model.Session
		var role string
		var updatedAtMicros int64
		var roleRank int
		var createdAtMicros int64
		if err := rows.Scan(
			&session.ID,
			&session.ConversationID,
			&session.Key,
			&session.AgentID,
			&role,
			&session.ParentSessionID,
			&session.ControllerSessionID,
			&session.Status,
			&session.CreatedAt,
			&updatedAtMicros,
			&roleRank,
			&createdAtMicros,
		); err != nil {
			return PageResult[model.Session]{}, fmt.Errorf("scan conversation session: %w", err)
		}
		session.UpdatedAt = time.UnixMicro(updatedAtMicros).UTC()
		session.Role = model.SessionRole(role)
		list = append(list, sessionRow{
			session: session,
			cursor: sessionPageCursor{
				UpdatedAtMicros: updatedAtMicros,
				RoleRank:        roleRank,
				CreatedAtMicros: createdAtMicros,
				ID:              session.ID,
			},
		})
	}
	if err := rows.Err(); err != nil {
		return PageResult[model.Session]{}, fmt.Errorf("iterate sessions: %w", err)
	}

	hasMore := len(list) > filter.Limit
	if hasMore {
		list = list[:filter.Limit]
	}
	if reverseResults {
		reverseSlice(list)
	}

	page := PageResult[model.Session]{
		Items: make([]model.Session, 0, len(list)),
	}
	for _, item := range list {
		page.Items = append(page.Items, item.session)
	}
	if len(list) == 0 {
		return page, nil
	}

	if strings.TrimSpace(filter.Cursor) != "" {
		if direction == "prev" {
			page.HasNext = true
		} else {
			page.HasPrev = true
		}
	}
	if hasMore {
		if direction == "prev" {
			page.HasPrev = true
		} else {
			page.HasNext = true
		}
	}
	if page.HasPrev {
		page.PrevCursor, err = encodePageCursor(list[0].cursor)
		if err != nil {
			return PageResult[model.Session]{}, fmt.Errorf("encode previous session cursor: %w", err)
		}
	}
	if page.HasNext {
		page.NextCursor, err = encodePageCursor(list[len(list)-1].cursor)
		if err != nil {
			return PageResult[model.Session]{}, fmt.Errorf("encode next session cursor: %w", err)
		}
	}

	return page, nil
}

func normalizeSessionBinding(value string) string {
	switch strings.TrimSpace(value) {
	case "", "any":
		return "any"
	case "bound":
		return "bound"
	case "unbound":
		return "unbound"
	default:
		return "any"
	}
}

func (s *Service) LoadRouteBySession(ctx context.Context, sessionID string) (model.SessionRoute, error) {
	var route model.SessionRoute
	var deactivatedAt sql.NullString
	var deactivationReason string
	var replacedByRouteID string
	err := s.db.RawDB().QueryRowContext(ctx,
		`SELECT id, session_id, thread_id, connector_id, account_id, external_id, status, created_at, deactivated_at,
		        deactivation_reason, replaced_by_route_id
		 FROM session_bindings
		 WHERE session_id = ? AND status = 'active'
		 ORDER BY created_at DESC, id DESC
		 LIMIT 1`,
		sessionID,
	).Scan(
		&route.ID,
		&route.SessionID,
		&route.ThreadID,
		&route.ConnectorID,
		&route.AccountID,
		&route.ExternalID,
		&route.Status,
		&route.CreatedAt,
		&deactivatedAt,
		&deactivationReason,
		&replacedByRouteID,
	)
	if err == sql.ErrNoRows {
		return model.SessionRoute{}, ErrSessionRouteNotFound
	}
	if err != nil {
		return model.SessionRoute{}, fmt.Errorf("load route by session: %w", err)
	}
	if deactivatedAt.Valid && deactivatedAt.String != "" {
		parsed, err := parseActivityTime(deactivatedAt.String)
		if err != nil {
			return model.SessionRoute{}, fmt.Errorf("parse route deactivated_at: %w", err)
		}
		route.DeactivatedAt = &parsed
	}
	route.DeactivationReason = deactivationReason
	route.ReplacedByRouteID = replacedByRouteID
	return route, nil
}

func (s *Service) LoadRoute(ctx context.Context, routeID string) (model.RouteDirectoryItem, error) {
	rows, err := s.db.RawDB().QueryContext(ctx,
		`SELECT bind.id, bind.session_id, bind.thread_id, bind.connector_id, bind.account_id, bind.external_id,
		        bind.status, bind.created_at, bind.deactivated_at, bind.deactivation_reason, bind.replaced_by_route_id,
		        bind.conversation_id, sess.agent_id, sess.role
		 FROM session_bindings bind
		 JOIN sessions sess ON sess.id = bind.session_id
		 WHERE bind.id = ?
		 LIMIT 1`,
		routeID,
	)
	if err != nil {
		return model.RouteDirectoryItem{}, fmt.Errorf("load route: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return model.RouteDirectoryItem{}, fmt.Errorf("iterate route: %w", err)
		}
		return model.RouteDirectoryItem{}, ErrSessionRouteNotFound
	}

	route, err := scanRouteDirectoryItem(rows)
	if err == sql.ErrNoRows {
		return model.RouteDirectoryItem{}, ErrSessionRouteNotFound
	}
	if err != nil {
		return model.RouteDirectoryItem{}, err
	}
	return route, nil
}

func (s *Service) ListRoutes(ctx context.Context, filter RouteListFilter) ([]model.RouteDirectoryItem, error) {
	page, err := s.ListRoutesPage(ctx, filter)
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

func (s *Service) ListRoutesPage(ctx context.Context, filter RouteListFilter) (PageResult[model.RouteDirectoryItem], error) {
	if filter.Limit <= 0 {
		filter.Limit = 100
	}

	createdMicrosExpr := sqliteMicros("bind.created_at")
	orderDirection := "DESC"
	reverseResults := false

	query := strings.Builder{}
	query.WriteString(
		`SELECT bind.id, bind.session_id, bind.thread_id, bind.connector_id, bind.account_id, bind.external_id,
		        bind.status, bind.created_at, bind.deactivated_at, bind.deactivation_reason, bind.replaced_by_route_id,
		        bind.conversation_id, sess.agent_id, sess.role, ` + createdMicrosExpr + ` AS created_at_micros
		 FROM session_bindings bind
		 JOIN sessions sess ON sess.id = bind.session_id
		 WHERE 1 = 1`,
	)

	args := make([]any, 0, 12)
	if filter.ProjectID != "" {
		scopeCondition, scopeArgs := projectscope.RunCondition(model.Project{
			ID: filter.ProjectID,
		}, "scope_runs")
		query.WriteString(` AND EXISTS (
			SELECT 1
			FROM runs scope_runs
			WHERE scope_runs.conversation_id = bind.conversation_id
			  AND ` + scopeCondition + `
		)`)
		args = append(args, scopeArgs...)
	}
	if filter.ConnectorID != "" {
		query.WriteString(" AND bind.connector_id = ?")
		args = append(args, filter.ConnectorID)
	}
	status := filter.Status
	if status == "" {
		status = "active"
	}
	if status != "all" {
		query.WriteString(" AND bind.status = ?")
		args = append(args, status)
	}
	if filter.Query != "" {
		q := "%" + strings.ToLower(strings.TrimSpace(filter.Query)) + "%"
		query.WriteString(` AND (
			LOWER(bind.id) LIKE ?
			OR LOWER(bind.session_id) LIKE ?
			OR LOWER(bind.thread_id) LIKE ?
			OR LOWER(bind.connector_id) LIKE ?
			OR LOWER(bind.account_id) LIKE ?
			OR LOWER(bind.external_id) LIKE ?
			OR LOWER(bind.conversation_id) LIKE ?
			OR LOWER(sess.agent_id) LIKE ?
		)`)
		args = append(args, q, q, q, q, q, q, q, q)
	}
	direction := normalizePageDirection(filter.Direction)
	if strings.TrimSpace(filter.Cursor) != "" {
		var cursor routePageCursor
		if err := decodePageCursor(filter.Cursor, &cursor); err != nil {
			return PageResult[model.RouteDirectoryItem]{}, fmt.Errorf("list routes page: %w", err)
		}
		if direction == "prev" {
			query.WriteString(` AND (` + createdMicrosExpr + ` > ? OR (` + createdMicrosExpr + ` = ? AND bind.id > ?))`)
			args = append(args, cursor.CreatedAtMicros, cursor.CreatedAtMicros, cursor.ID)
			orderDirection = "ASC"
			reverseResults = true
		} else {
			query.WriteString(` AND (` + createdMicrosExpr + ` < ? OR (` + createdMicrosExpr + ` = ? AND bind.id < ?))`)
			args = append(args, cursor.CreatedAtMicros, cursor.CreatedAtMicros, cursor.ID)
		}
	}
	query.WriteString(`
		 ORDER BY ` + createdMicrosExpr + ` ` + orderDirection + `, bind.id ` + orderDirection + `
		 LIMIT ?`)
	args = append(args, filter.Limit+1)

	rows, err := s.db.RawDB().QueryContext(ctx, query.String(), args...)
	if err != nil {
		return PageResult[model.RouteDirectoryItem]{}, fmt.Errorf("list routes page: %w", err)
	}
	defer rows.Close()

	type routeRow struct {
		route  model.RouteDirectoryItem
		cursor routePageCursor
	}

	routes := make([]routeRow, 0, filter.Limit+1)
	for rows.Next() {
		var createdAtMicros int64
		route, err := scanRouteDirectoryItemWithCursor(rows, &createdAtMicros)
		if err != nil {
			return PageResult[model.RouteDirectoryItem]{}, err
		}
		routes = append(routes, routeRow{
			route: route,
			cursor: routePageCursor{
				CreatedAtMicros: createdAtMicros,
				ID:              route.ID,
			},
		})
	}
	if err := rows.Err(); err != nil {
		return PageResult[model.RouteDirectoryItem]{}, fmt.Errorf("iterate routes: %w", err)
	}

	hasMore := len(routes) > filter.Limit
	if hasMore {
		routes = routes[:filter.Limit]
	}
	if reverseResults {
		reverseSlice(routes)
	}

	page := PageResult[model.RouteDirectoryItem]{
		Items: make([]model.RouteDirectoryItem, 0, len(routes)),
	}
	for _, item := range routes {
		page.Items = append(page.Items, item.route)
	}
	if len(routes) == 0 {
		return page, nil
	}

	if strings.TrimSpace(filter.Cursor) != "" {
		if direction == "prev" {
			page.HasNext = true
		} else {
			page.HasPrev = true
		}
	}
	if hasMore {
		if direction == "prev" {
			page.HasPrev = true
		} else {
			page.HasNext = true
		}
	}
	if page.HasPrev {
		page.PrevCursor, err = encodePageCursor(routes[0].cursor)
		if err != nil {
			return PageResult[model.RouteDirectoryItem]{}, fmt.Errorf("encode previous route cursor: %w", err)
		}
	}
	if page.HasNext {
		page.NextCursor, err = encodePageCursor(routes[len(routes)-1].cursor)
		if err != nil {
			return PageResult[model.RouteDirectoryItem]{}, fmt.Errorf("encode next route cursor: %w", err)
		}
	}

	return page, nil
}

func (s *Service) LoadSessionOutboundIntent(ctx context.Context, sessionID string, intentID string) (model.OutboundIntent, error) {
	var intent model.OutboundIntent
	var lastAttempt sql.NullString
	err := s.db.RawDB().QueryRowContext(ctx,
		`SELECT oi.id, COALESCE(oi.run_id, ''), oi.connector_id, oi.chat_id, oi.message_text,
		        COALESCE(oi.metadata_json, x'7B7D'), COALESCE(oi.dedupe_key, ''), oi.status, oi.attempts, oi.created_at, oi.last_attempt_at
		 FROM outbound_intents oi
		 JOIN runs r ON r.id = oi.run_id
		 WHERE r.session_id = ? AND oi.id = ?`,
		sessionID,
		intentID,
	).Scan(
		&intent.ID,
		&intent.RunID,
		&intent.ConnectorID,
		&intent.ChatID,
		&intent.MessageText,
		&intent.MetadataJSON,
		&intent.DedupeKey,
		&intent.Status,
		&intent.Attempts,
		&intent.CreatedAt,
		&lastAttempt,
	)
	if err == sql.ErrNoRows {
		return model.OutboundIntent{}, ErrOutboundIntentNotFound
	}
	if err != nil {
		return model.OutboundIntent{}, fmt.Errorf("load session outbound intent: %w", err)
	}
	if lastAttempt.Valid && lastAttempt.String != "" {
		parsed, err := parseActivityTime(lastAttempt.String)
		if err != nil {
			return model.OutboundIntent{}, fmt.Errorf("parse outbound intent last_attempt_at: %w", err)
		}
		intent.LastAttemptAt = &parsed
	}
	return intent, nil
}

func (s *Service) ListSessionOutboundIntents(ctx context.Context, sessionID string, limit int) ([]model.OutboundIntent, error) {
	var (
		rows *sql.Rows
		err  error
	)

	if limit > 0 {
		rows, err = s.db.RawDB().QueryContext(ctx,
			`SELECT id, run_id, connector_id, chat_id, message_text, metadata_json, dedupe_key, status, attempts, created_at, last_attempt_at
			 FROM (
			     SELECT oi.id, COALESCE(oi.run_id, '') AS run_id, oi.connector_id, oi.chat_id, oi.message_text,
			            COALESCE(oi.metadata_json, x'7B7D') AS metadata_json, COALESCE(oi.dedupe_key, '') AS dedupe_key, oi.status, oi.attempts, oi.created_at, oi.last_attempt_at
			     FROM outbound_intents oi
			     JOIN runs r ON r.id = oi.run_id
			     WHERE r.session_id = ?
			     ORDER BY oi.created_at DESC, oi.id DESC
			     LIMIT ?
			 )
			 ORDER BY created_at ASC, id ASC`,
			sessionID, limit,
		)
	} else {
		rows, err = s.db.RawDB().QueryContext(ctx,
			`SELECT oi.id, COALESCE(oi.run_id, ''), oi.connector_id, oi.chat_id, oi.message_text,
			        COALESCE(oi.metadata_json, x'7B7D'), COALESCE(oi.dedupe_key, ''), oi.status, oi.attempts, oi.created_at, oi.last_attempt_at
			 FROM outbound_intents oi
			 JOIN runs r ON r.id = oi.run_id
			 WHERE r.session_id = ?
			 ORDER BY oi.created_at ASC, oi.id ASC`,
			sessionID,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("list session outbound intents: %w", err)
	}
	defer rows.Close()

	intents := make([]model.OutboundIntent, 0)
	for rows.Next() {
		var intent model.OutboundIntent
		var lastAttempt sql.NullString
		if err := rows.Scan(
			&intent.ID,
			&intent.RunID,
			&intent.ConnectorID,
			&intent.ChatID,
			&intent.MessageText,
			&intent.MetadataJSON,
			&intent.DedupeKey,
			&intent.Status,
			&intent.Attempts,
			&intent.CreatedAt,
			&lastAttempt,
		); err != nil {
			return nil, fmt.Errorf("scan session outbound intent: %w", err)
		}
		if lastAttempt.Valid && lastAttempt.String != "" {
			parsed, err := parseActivityTime(lastAttempt.String)
			if err != nil {
				return nil, fmt.Errorf("parse outbound intent last_attempt_at: %w", err)
			}
			intent.LastAttemptAt = &parsed
		}
		intents = append(intents, intent)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate session outbound intents: %w", err)
	}
	return intents, nil
}

func (s *Service) ListDeliveryQueue(ctx context.Context, filter DeliveryQueueFilter) ([]model.DeliveryQueueItem, error) {
	page, err := s.ListDeliveryQueuePage(ctx, filter)
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

func (s *Service) ListDeliveryQueuePage(ctx context.Context, filter DeliveryQueueFilter) (PageResult[model.DeliveryQueueItem], error) {
	if filter.Limit <= 0 {
		filter.Limit = 100
	}

	statusRankExpr := `CASE oi.status
	         WHEN 'retrying' THEN 0
	         WHEN 'pending' THEN 1
	         WHEN 'terminal' THEN 2
	         ELSE 3
	     END`
	createdMicrosExpr := sqliteMicros("oi.created_at")
	orderDirection := "ASC"
	reverseResults := false

	query := strings.Builder{}
	query.WriteString(
		`SELECT oi.id, COALESCE(oi.run_id, ''), r.session_id, r.conversation_id,
		        oi.connector_id, oi.chat_id, oi.message_text, COALESCE(oi.metadata_json, x'7B7D'), COALESCE(oi.dedupe_key, ''),
		        oi.status, oi.attempts, oi.created_at, oi.last_attempt_at,
		        ` + statusRankExpr + ` AS status_rank,
		        ` + createdMicrosExpr + ` AS created_at_micros
		 FROM outbound_intents oi
		 JOIN runs r ON r.id = oi.run_id`,
	)

	args := make([]any, 0, 3)
	conditions := make([]string, 0, 2)
	if filter.ProjectID != "" {
		scopeCondition, scopeArgs := projectscope.RunCondition(model.Project{
			ID: filter.ProjectID,
		}, "r")
		conditions = append(conditions, scopeCondition)
		args = append(args, scopeArgs...)
	}
	if filter.ConnectorID != "" {
		conditions = append(conditions, "oi.connector_id = ?")
		args = append(args, filter.ConnectorID)
	}
	if filter.SessionID != "" {
		conditions = append(conditions, "r.session_id = ?")
		args = append(args, filter.SessionID)
	}
	if filter.Status != "" && filter.Status != "all" {
		conditions = append(conditions, "oi.status = ?")
		args = append(args, filter.Status)
	} else {
		conditions = append(conditions, "oi.status IN ('pending', 'retrying', 'terminal')")
	}
	if filter.Query != "" {
		q := "%" + strings.ToLower(strings.TrimSpace(filter.Query)) + "%"
		conditions = append(conditions, `(LOWER(oi.id) LIKE ? OR LOWER(COALESCE(oi.run_id, '')) LIKE ? OR LOWER(r.session_id) LIKE ? OR LOWER(r.conversation_id) LIKE ? OR LOWER(oi.connector_id) LIKE ? OR LOWER(oi.chat_id) LIKE ? OR LOWER(oi.message_text) LIKE ?)`)
		args = append(args, q, q, q, q, q, q, q)
	}
	direction := normalizePageDirection(filter.Direction)
	if strings.TrimSpace(filter.Cursor) != "" {
		var cursor deliveryPageCursor
		if err := decodePageCursor(filter.Cursor, &cursor); err != nil {
			return PageResult[model.DeliveryQueueItem]{}, fmt.Errorf("list delivery queue page: %w", err)
		}
		if direction == "prev" {
			conditions = append(conditions, `(`+statusRankExpr+` < ? OR (`+statusRankExpr+` = ? AND `+createdMicrosExpr+` < ?) OR (`+statusRankExpr+` = ? AND `+createdMicrosExpr+` = ? AND oi.id < ?))`)
			args = append(args,
				cursor.StatusRank,
				cursor.StatusRank, cursor.CreatedAtMicros,
				cursor.StatusRank, cursor.CreatedAtMicros, cursor.ID,
			)
			orderDirection = "DESC"
			reverseResults = true
		} else {
			conditions = append(conditions, `(`+statusRankExpr+` > ? OR (`+statusRankExpr+` = ? AND `+createdMicrosExpr+` > ?) OR (`+statusRankExpr+` = ? AND `+createdMicrosExpr+` = ? AND oi.id > ?))`)
			args = append(args,
				cursor.StatusRank,
				cursor.StatusRank, cursor.CreatedAtMicros,
				cursor.StatusRank, cursor.CreatedAtMicros, cursor.ID,
			)
		}
	}
	if len(conditions) > 0 {
		query.WriteString(" WHERE ")
		query.WriteString(strings.Join(conditions, " AND "))
	}
	query.WriteString(`
		 ORDER BY
		     ` + statusRankExpr + ` ` + orderDirection + `,
		     ` + createdMicrosExpr + ` ` + orderDirection + `,
		     oi.id ` + orderDirection + `
		 LIMIT ?`)
	args = append(args, filter.Limit+1)

	rows, err := s.db.RawDB().QueryContext(ctx, query.String(), args...)
	if err != nil {
		return PageResult[model.DeliveryQueueItem]{}, fmt.Errorf("list delivery queue page: %w", err)
	}
	defer rows.Close()

	type deliveryRow struct {
		item   model.DeliveryQueueItem
		cursor deliveryPageCursor
	}

	items := make([]deliveryRow, 0, filter.Limit+1)
	for rows.Next() {
		var statusRank int
		var createdAtMicros int64
		item, err := scanDeliveryQueueItemWithCursor(rows, &statusRank, &createdAtMicros)
		if err != nil {
			return PageResult[model.DeliveryQueueItem]{}, err
		}
		items = append(items, deliveryRow{
			item: item,
			cursor: deliveryPageCursor{
				StatusRank:      statusRank,
				CreatedAtMicros: createdAtMicros,
				ID:              item.ID,
			},
		})
	}
	if err := rows.Err(); err != nil {
		return PageResult[model.DeliveryQueueItem]{}, fmt.Errorf("iterate delivery queue: %w", err)
	}

	hasMore := len(items) > filter.Limit
	if hasMore {
		items = items[:filter.Limit]
	}
	if reverseResults {
		reverseSlice(items)
	}

	page := PageResult[model.DeliveryQueueItem]{
		Items: make([]model.DeliveryQueueItem, 0, len(items)),
	}
	for _, item := range items {
		page.Items = append(page.Items, item.item)
	}
	if len(items) == 0 {
		return page, nil
	}

	if strings.TrimSpace(filter.Cursor) != "" {
		if direction == "prev" {
			page.HasNext = true
		} else {
			page.HasPrev = true
		}
	}
	if hasMore {
		if direction == "prev" {
			page.HasPrev = true
		} else {
			page.HasNext = true
		}
	}
	if page.HasPrev {
		page.PrevCursor, err = encodePageCursor(items[0].cursor)
		if err != nil {
			return PageResult[model.DeliveryQueueItem]{}, fmt.Errorf("encode previous delivery cursor: %w", err)
		}
	}
	if page.HasNext {
		page.NextCursor, err = encodePageCursor(items[len(items)-1].cursor)
		if err != nil {
			return PageResult[model.DeliveryQueueItem]{}, fmt.Errorf("encode next delivery cursor: %w", err)
		}
	}

	return page, nil
}

func (s *Service) ListSessionDeliveryFailures(ctx context.Context, sessionID string, limit int) ([]model.DeliveryFailure, error) {
	var (
		rows *sql.Rows
		err  error
	)

	if limit > 0 {
		rows, err = s.db.RawDB().QueryContext(ctx,
			`SELECT id, run_id, payload_json, created_at
			 FROM (
			     SELECT e.id, COALESCE(e.run_id, '') AS run_id, COALESCE(e.payload_json, x'') AS payload_json, e.created_at
			     FROM events e
			     JOIN runs r ON r.id = e.run_id
			     WHERE e.kind = 'delivery_failed' AND r.session_id = ?
			     ORDER BY e.created_at DESC, e.id DESC
			     LIMIT ?
			 )
			 ORDER BY created_at ASC, id ASC`,
			sessionID, limit,
		)
	} else {
		rows, err = s.db.RawDB().QueryContext(ctx,
			`SELECT e.id, COALESCE(e.run_id, ''), COALESCE(e.payload_json, x''), e.created_at
			 FROM events e
			 JOIN runs r ON r.id = e.run_id
			 WHERE e.kind = 'delivery_failed' AND r.session_id = ?
			 ORDER BY e.created_at ASC, e.id ASC`,
			sessionID,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("list session delivery failures: %w", err)
	}
	defer rows.Close()

	failures := make([]model.DeliveryFailure, 0)
	for rows.Next() {
		var failure model.DeliveryFailure
		var payloadJSON []byte
		if err := rows.Scan(
			&failure.ID,
			&failure.RunID,
			&payloadJSON,
			&failure.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan session delivery failure: %w", err)
		}
		var payload struct {
			IntentID    string `json:"intent_id"`
			ConnectorID string `json:"connector_id"`
			ChatID      string `json:"chat_id"`
			EventKind   string `json:"event_kind"`
			Error       string `json:"error"`
		}
		if len(payloadJSON) > 0 {
			if err := json.Unmarshal(payloadJSON, &payload); err != nil {
				return nil, fmt.Errorf("unmarshal session delivery failure payload: %w", err)
			}
		}
		failure.IntentID = payload.IntentID
		failure.ConnectorID = payload.ConnectorID
		failure.ChatID = payload.ChatID
		failure.EventKind = payload.EventKind
		failure.Error = payload.Error
		failures = append(failures, failure)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate session delivery failures: %w", err)
	}

	actionable := make([]model.DeliveryFailure, 0, len(failures))
	for _, failure := range failures {
		if failure.IntentID != "" {
			status, err := s.loadOutboundIntentStatus(ctx, failure.IntentID)
			if err == nil && status != "terminal" {
				continue
			}
			if err == ErrOutboundIntentNotFound {
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("load delivery failure intent status: %w", err)
			}
		}
		actionable = append(actionable, failure)
	}

	return actionable, nil
}

func (s *Service) ListConnectorDeliveryHealth(ctx context.Context) ([]model.ConnectorDeliveryHealth, error) {
	rows, err := s.db.RawDB().QueryContext(ctx,
		`SELECT connector_id,
		        SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END) AS pending_count,
		        SUM(CASE WHEN status = 'retrying' THEN 1 ELSE 0 END) AS retrying_count,
		        SUM(CASE WHEN status = 'terminal' THEN 1 ELSE 0 END) AS terminal_count,
		        MIN(CASE WHEN status = 'pending' THEN created_at END) AS oldest_pending_at,
		        MIN(CASE WHEN status = 'retrying' THEN last_attempt_at END) AS oldest_retrying_at
		 FROM outbound_intents
		 WHERE status IN ('pending', 'retrying', 'terminal')
		 GROUP BY connector_id
		 ORDER BY connector_id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list connector delivery health: %w", err)
	}
	defer rows.Close()

	summaries := make([]model.ConnectorDeliveryHealth, 0)
	for rows.Next() {
		var summary model.ConnectorDeliveryHealth
		var oldestPending sql.NullString
		var oldestRetrying sql.NullString
		if err := rows.Scan(
			&summary.ConnectorID,
			&summary.PendingCount,
			&summary.RetryingCount,
			&summary.TerminalCount,
			&oldestPending,
			&oldestRetrying,
		); err != nil {
			return nil, fmt.Errorf("scan connector delivery health: %w", err)
		}
		if oldestPending.Valid && oldestPending.String != "" {
			parsed, err := parseActivityTime(oldestPending.String)
			if err != nil {
				return nil, fmt.Errorf("parse oldest pending time: %w", err)
			}
			summary.OldestPendingAt = &parsed
		}
		if oldestRetrying.Valid && oldestRetrying.String != "" {
			parsed, err := parseActivityTime(oldestRetrying.String)
			if err != nil {
				return nil, fmt.Errorf("parse oldest retrying time: %w", err)
			}
			summary.OldestRetryingAt = &parsed
		}
		summaries = append(summaries, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate connector delivery health: %w", err)
	}

	return summaries, nil
}

func (s *Service) LoadDeliveryQueueItem(ctx context.Context, intentID string) (model.DeliveryQueueItem, error) {
	rows, err := s.db.RawDB().QueryContext(ctx,
		`SELECT oi.id, COALESCE(oi.run_id, ''), r.session_id, r.conversation_id,
		        oi.connector_id, oi.chat_id, oi.message_text, COALESCE(oi.metadata_json, x'7B7D'), COALESCE(oi.dedupe_key, ''),
		        oi.status, oi.attempts, oi.created_at, oi.last_attempt_at
		 FROM outbound_intents oi
		 JOIN runs r ON r.id = oi.run_id
		 WHERE oi.id = ?
		 LIMIT 1`,
		intentID,
	)
	if err != nil {
		return model.DeliveryQueueItem{}, fmt.Errorf("load delivery queue item: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return model.DeliveryQueueItem{}, fmt.Errorf("iterate delivery queue item: %w", err)
		}
		return model.DeliveryQueueItem{}, ErrOutboundIntentNotFound
	}

	item, err := scanDeliveryQueueItem(rows)
	if err != nil {
		return model.DeliveryQueueItem{}, err
	}
	return item, nil
}

func scanRouteDirectoryItem(scanner interface {
	Scan(dest ...any) error
}) (model.RouteDirectoryItem, error) {
	return scanRouteDirectoryItemWithCursor(scanner, nil)
}

func scanRouteDirectoryItemWithCursor(scanner interface {
	Scan(dest ...any) error
}, createdAtMicros *int64) (model.RouteDirectoryItem, error) {
	var route model.RouteDirectoryItem
	var role string
	var deactivatedAt sql.NullString
	var deactivationReason string
	var replacedByRouteID string
	dest := []any{
		&route.ID,
		&route.SessionID,
		&route.ThreadID,
		&route.ConnectorID,
		&route.AccountID,
		&route.ExternalID,
		&route.Status,
		&route.CreatedAt,
		&deactivatedAt,
		&deactivationReason,
		&replacedByRouteID,
		&route.ConversationID,
		&route.AgentID,
		&role,
	}
	if createdAtMicros != nil {
		dest = append(dest, createdAtMicros)
	}
	if err := scanner.Scan(dest...); err != nil {
		return model.RouteDirectoryItem{}, fmt.Errorf("scan route directory item: %w", err)
	}
	route.Role = model.SessionRole(role)
	if deactivatedAt.Valid && deactivatedAt.String != "" {
		parsed, err := parseActivityTime(deactivatedAt.String)
		if err != nil {
			return model.RouteDirectoryItem{}, fmt.Errorf("parse route deactivated_at: %w", err)
		}
		route.DeactivatedAt = &parsed
	}
	route.DeactivationReason = deactivationReason
	route.ReplacedByRouteID = replacedByRouteID
	return route, nil
}

func (s *Service) loadOutboundIntentStatus(ctx context.Context, intentID string) (string, error) {
	var status string
	err := s.db.RawDB().QueryRowContext(ctx,
		"SELECT status FROM outbound_intents WHERE id = ?",
		intentID,
	).Scan(&status)
	if err == sql.ErrNoRows {
		return "", ErrOutboundIntentNotFound
	}
	if err != nil {
		return "", fmt.Errorf("load outbound intent status: %w", err)
	}
	return status, nil
}

func scanDeliveryQueueItem(scanner interface {
	Scan(dest ...any) error
}) (model.DeliveryQueueItem, error) {
	return scanDeliveryQueueItemWithCursor(scanner, nil, nil)
}

func scanDeliveryQueueItemWithCursor(scanner interface {
	Scan(dest ...any) error
}, statusRank *int, createdAtMicros *int64) (model.DeliveryQueueItem, error) {
	var item model.DeliveryQueueItem
	var lastAttempt sql.NullString
	dest := []any{
		&item.ID,
		&item.RunID,
		&item.SessionID,
		&item.ConversationID,
		&item.ConnectorID,
		&item.ChatID,
		&item.MessageText,
		&item.MetadataJSON,
		&item.DedupeKey,
		&item.Status,
		&item.Attempts,
		&item.CreatedAt,
		&lastAttempt,
	}
	if statusRank != nil {
		dest = append(dest, statusRank)
	}
	if createdAtMicros != nil {
		dest = append(dest, createdAtMicros)
	}
	if err := scanner.Scan(dest...); err != nil {
		return model.DeliveryQueueItem{}, fmt.Errorf("scan delivery queue item: %w", err)
	}
	if lastAttempt.Valid && lastAttempt.String != "" {
		parsed, err := parseActivityTime(lastAttempt.String)
		if err != nil {
			return model.DeliveryQueueItem{}, fmt.Errorf("parse delivery queue last_attempt_at: %w", err)
		}
		item.LastAttemptAt = &parsed
	}
	return item, nil
}

func parseActivityTime(value string) (time.Time, error) {
	layouts := []string{
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
		time.RFC3339Nano,
	}
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported time format %q", value)
}

func (s *Service) loadBoundSession(ctx context.Context, conversationID string, threadID string) (model.Session, error) {
	var session model.Session
	var role string
	err := s.db.RawDB().QueryRowContext(ctx,
		`SELECT sess.id, sess.conversation_id, sess.key, sess.agent_id, sess.role,
		        COALESCE(sess.parent_session_id, ''), COALESCE(sess.controller_session_id, ''),
		        sess.status, sess.created_at
		 FROM session_bindings bind
		 JOIN sessions sess ON sess.id = bind.session_id
		 WHERE bind.conversation_id = ? AND bind.thread_id = ? AND bind.status = 'active'
		 ORDER BY bind.created_at DESC, bind.id DESC
		 LIMIT 1`,
		conversationID, threadID,
	).Scan(
		&session.ID,
		&session.ConversationID,
		&session.Key,
		&session.AgentID,
		&role,
		&session.ParentSessionID,
		&session.ControllerSessionID,
		&session.Status,
		&session.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return model.Session{}, ErrThreadMailboxNotFound
	}
	if err != nil {
		return model.Session{}, fmt.Errorf("load bound session: %w", err)
	}

	session.Role = model.SessionRole(role)
	session.UpdatedAt = session.CreatedAt
	return session, nil
}

func (s *Service) loadSession(ctx context.Context, sessionID string) (model.Session, error) {
	var session model.Session
	var role string
	err := s.db.RawDB().QueryRowContext(ctx,
		`SELECT id, conversation_id, key, agent_id, role,
		        COALESCE(parent_session_id, ''), COALESCE(controller_session_id, ''),
		        status, created_at
		 FROM sessions
		 WHERE id = ?`,
		sessionID,
	).Scan(
		&session.ID,
		&session.ConversationID,
		&session.Key,
		&session.AgentID,
		&role,
		&session.ParentSessionID,
		&session.ControllerSessionID,
		&session.Status,
		&session.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return model.Session{}, ErrSessionNotFound
	}
	if err != nil {
		return model.Session{}, fmt.Errorf("load session: %w", err)
	}
	session.Role = model.SessionRole(role)
	session.UpdatedAt = session.CreatedAt
	return session, nil
}

func (s *Service) listSessionMessages(ctx context.Context, sessionID string, limit int) ([]model.SessionMessage, error) {
	var (
		rows *sql.Rows
		err  error
	)

	if limit > 0 {
		rows, err = s.db.RawDB().QueryContext(ctx,
			`SELECT id, session_id, sender_session_id, kind, body, COALESCE(provenance_json, '{}'), created_at
				 FROM (
				     SELECT id, session_id, COALESCE(sender_session_id, '') AS sender_session_id, kind, body, COALESCE(provenance_json, '{}') AS provenance_json, created_at
				     FROM session_messages
				     WHERE session_id = ?
				     ORDER BY created_at DESC, id DESC
			     LIMIT ?
			 )
			 ORDER BY created_at ASC, id ASC`,
			sessionID, limit,
		)
	} else {
		rows, err = s.db.RawDB().QueryContext(ctx,
			`SELECT id, session_id, COALESCE(sender_session_id, ''), kind, body, COALESCE(provenance_json, '{}'), created_at
				 FROM session_messages
				 WHERE session_id = ?
				 ORDER BY created_at ASC, id ASC`,
			sessionID,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("list session messages: %w", err)
	}
	defer rows.Close()

	messages := make([]model.SessionMessage, 0)
	for rows.Next() {
		var msg model.SessionMessage
		var kind string
		var provenanceJSON []byte
		if err := rows.Scan(
			&msg.ID,
			&msg.SessionID,
			&msg.SenderSessionID,
			&kind,
			&msg.Body,
			&provenanceJSON,
			&msg.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan session message: %w", err)
		}
		msg.Kind = model.SessionMessageKind(kind)
		if len(provenanceJSON) > 0 {
			if err := json.Unmarshal(provenanceJSON, &msg.Provenance); err != nil {
				return nil, fmt.Errorf("unmarshal session message provenance: %w", err)
			}
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate session messages: %w", err)
	}

	return messages, nil
}

func normalizeThreadID(threadID string) string {
	if threadID == "" {
		return "main"
	}
	return threadID
}

func (s *Service) appendSessionOpened(ctx context.Context, runID string, session model.Session) error {
	payload := map[string]any{
		"session_id":            session.ID,
		"key":                   session.Key,
		"agent_id":              session.AgentID,
		"role":                  session.Role,
		"parent_session_id":     session.ParentSessionID,
		"controller_session_id": session.ControllerSessionID,
		"status":                session.Status,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal session_opened payload: %w", err)
	}
	return s.conv.AppendEvent(ctx, model.Event{
		ID:             generateID(),
		ConversationID: session.ConversationID,
		RunID:          runID,
		Kind:           "session_opened",
		PayloadJSON:    body,
		CreatedAt:      session.CreatedAt,
	})
}

func (s *Service) appendSessionMessage(ctx context.Context, runID string, msg model.SessionMessage) error {
	conversationID, err := s.conversationIDForSession(ctx, msg.SessionID)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"message_id":        msg.ID,
		"session_id":        msg.SessionID,
		"sender_session_id": msg.SenderSessionID,
		"kind":              msg.Kind,
		"body":              msg.Body,
		"provenance":        msg.Provenance,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal session_message_added payload: %w", err)
	}
	return s.conv.AppendEvent(ctx, model.Event{
		ID:             generateID(),
		ConversationID: conversationID,
		RunID:          runID,
		Kind:           "session_message_added",
		PayloadJSON:    body,
		CreatedAt:      msg.CreatedAt,
	})
}

func (s *Service) appendSessionBinding(ctx context.Context, runID string, cmd BindFollowUp) error {
	now := time.Now().UTC()
	payload := map[string]any{
		"thread_id":    normalizeThreadID(cmd.ThreadID),
		"session_id":   cmd.SessionID,
		"connector_id": cmd.ConnectorID,
		"account_id":   cmd.AccountID,
		"external_id":  cmd.ExternalID,
		"status":       "active",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal session_bound payload: %w", err)
	}
	return s.conv.AppendEvent(ctx, model.Event{
		ID:             generateID(),
		ConversationID: cmd.ConversationID,
		RunID:          runID,
		Kind:           "session_bound",
		PayloadJSON:    body,
		CreatedAt:      now,
	})
}

func (s *Service) conversationIDForSession(ctx context.Context, sessionID string) (string, error) {
	var conversationID string
	err := s.db.RawDB().QueryRowContext(ctx,
		"SELECT conversation_id FROM sessions WHERE id = ?",
		sessionID,
	).Scan(&conversationID)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("lookup session conversation: session %s not found", sessionID)
	}
	if err != nil {
		return "", fmt.Errorf("lookup session conversation: %w", err)
	}
	return conversationID, nil
}

func generateID() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}
