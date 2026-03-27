CREATE TABLE IF NOT EXISTS conversations (
    id TEXT PRIMARY KEY,
    key TEXT NOT NULL UNIQUE,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS events (
    id TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL,
    run_id TEXT,
    parent_run_id TEXT,
    kind TEXT NOT NULL,
    payload_json BLOB,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS projects (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    primary_path TEXT NOT NULL DEFAULT '',
    roots_json BLOB NOT NULL DEFAULT '[]',
    policy_json BLOB NOT NULL DEFAULT '{}',
    source TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    last_used_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS runs (
    id TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    session_id TEXT,
    team_id TEXT,
    project_id TEXT,
    parent_run_id TEXT,
    objective TEXT,
    cwd TEXT NOT NULL DEFAULT '',
    authority_json BLOB NOT NULL DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'pending',
    execution_snapshot_json BLOB,
    input_tokens INTEGER DEFAULT 0,
    output_tokens INTEGER DEFAULT 0,
    model_lane TEXT,
    model_id TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL,
    key TEXT NOT NULL UNIQUE,
    agent_id TEXT NOT NULL,
    role TEXT NOT NULL,
    parent_session_id TEXT,
    controller_session_id TEXT,
    status TEXT NOT NULL DEFAULT 'active',
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS session_messages (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    sender_session_id TEXT,
    kind TEXT NOT NULL,
    body TEXT NOT NULL,
    provenance_json BLOB,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS session_bindings (
    id TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL,
    thread_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    connector_id TEXT NOT NULL DEFAULT '',
    account_id TEXT NOT NULL DEFAULT '',
    external_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'active',
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    deactivated_at DATETIME,
    deactivation_reason TEXT NOT NULL DEFAULT '',
    replaced_by_route_id TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS inbound_receipts (
    id TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL,
    connector_id TEXT NOT NULL,
    account_id TEXT NOT NULL DEFAULT '',
    thread_id TEXT NOT NULL,
    source_message_id TEXT NOT NULL,
    run_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    session_message_id TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS tool_calls (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL,
    tool_name TEXT NOT NULL,
    input_json BLOB,
    output_json BLOB,
    decision TEXT NOT NULL,
    approval_id TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS approvals (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL,
    tool_name TEXT NOT NULL,
    args_json BLOB,
    binding_json BLOB NOT NULL DEFAULT '{}',
    fingerprint TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    resolved_by TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    resolved_at DATETIME
);

CREATE TABLE IF NOT EXISTS receipts (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL UNIQUE,
    input_tokens INTEGER DEFAULT 0,
    output_tokens INTEGER DEFAULT 0,
    cost_usd REAL DEFAULT 0,
    model_lane TEXT,
    model_id TEXT,
    verification_status TEXT,
    approval_count INTEGER DEFAULT 0,
    budget_status TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS memory_items (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    scope TEXT NOT NULL DEFAULT 'local',
    content TEXT NOT NULL,
    source TEXT NOT NULL,
    provenance TEXT,
    confidence REAL DEFAULT 1.0,
    dedupe_key TEXT,
    forgotten_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS outbound_intents (
    id TEXT PRIMARY KEY,
    run_id TEXT,
    connector_id TEXT NOT NULL,
    chat_id TEXT NOT NULL,
    message_text TEXT NOT NULL,
    dedupe_key TEXT,
    status TEXT NOT NULL DEFAULT 'pending',
    attempts INTEGER DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    last_attempt_at DATETIME
);

CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS auth_devices (
    id TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    browser TEXT NOT NULL DEFAULT '',
    platform TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    last_seen_at DATETIME NOT NULL DEFAULT (datetime('now')),
    last_ip TEXT NOT NULL DEFAULT '',
    last_user_agent TEXT NOT NULL DEFAULT '',
    blocked_at DATETIME
);

CREATE TABLE IF NOT EXISTS auth_sessions (
    id TEXT PRIMARY KEY,
    device_id TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    last_seen_at DATETIME NOT NULL DEFAULT (datetime('now')),
    expires_at DATETIME NOT NULL,
    revoked_at DATETIME,
    revoke_reason TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (device_id) REFERENCES auth_devices(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_auth_devices_last_seen_at
    ON auth_devices(last_seen_at);

CREATE INDEX IF NOT EXISTS idx_auth_sessions_token_hash
    ON auth_sessions(token_hash);

CREATE INDEX IF NOT EXISTS idx_auth_sessions_device_id_revoked_expires_at
    ON auth_sessions(device_id, revoked_at, expires_at, last_seen_at);

CREATE INDEX IF NOT EXISTS idx_auth_sessions_expires_at_revoked_at
    ON auth_sessions(expires_at, revoked_at);

-- Conservative default budget limits. Raising or disabling any cap requires
-- an explicit operator action via the settings page.
-- per_run_token_budget: 50,000 tokens per run
-- per_run_cost_cap_usd: $0.50 USD per run
-- daily_cost_cap_usd: $5.00 USD rolling 24-hour window
INSERT OR IGNORE INTO settings (key, value, updated_at) VALUES
    ('per_run_token_budget', '50000',  datetime('now')),
    ('per_run_cost_cap_usd', '0.50',   datetime('now')),
    ('daily_cost_cap_usd',   '5.00',   datetime('now')),
    ('storage_root',         '.gistclaw', datetime('now')),
    ('approval_mode',        'prompt', datetime('now')),
    ('host_access_mode',     'standard', datetime('now'));

CREATE TABLE IF NOT EXISTS run_summaries (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL UNIQUE,
    project_id TEXT NOT NULL,
    content TEXT NOT NULL,
    token_count INTEGER DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS schedules (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    project_id TEXT NOT NULL DEFAULT '',
    objective TEXT NOT NULL,
    cwd TEXT NOT NULL DEFAULT '',
    authority_json BLOB NOT NULL DEFAULT '{}',
    schedule_kind TEXT NOT NULL,
    schedule_at TEXT NOT NULL DEFAULT '',
    schedule_every_seconds INTEGER NOT NULL DEFAULT 0,
    schedule_cron_expr TEXT NOT NULL DEFAULT '',
    timezone TEXT NOT NULL DEFAULT '',
    enabled INTEGER NOT NULL DEFAULT 1,
    next_run_at DATETIME,
    last_run_at DATETIME,
    last_status TEXT NOT NULL DEFAULT '',
    last_error TEXT NOT NULL DEFAULT '',
    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    schedule_error_count INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS schedule_occurrences (
    id TEXT PRIMARY KEY,
    schedule_id TEXT NOT NULL,
    slot_at DATETIME NOT NULL,
    thread_id TEXT NOT NULL,
    status TEXT NOT NULL,
    skip_reason TEXT NOT NULL DEFAULT '',
    run_id TEXT NOT NULL DEFAULT '',
    conversation_id TEXT NOT NULL DEFAULT '',
    error TEXT NOT NULL DEFAULT '',
    started_at DATETIME,
    finished_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (schedule_id) REFERENCES schedules(id) ON DELETE CASCADE,
    UNIQUE (schedule_id, slot_at)
);

CREATE INDEX IF NOT EXISTS idx_events_run_id_created_at ON events(run_id, created_at);
CREATE INDEX IF NOT EXISTS idx_projects_last_used_at ON projects(last_used_at, created_at);
CREATE INDEX IF NOT EXISTS idx_runs_conversation_id_status ON runs(conversation_id, status);
CREATE INDEX IF NOT EXISTS idx_runs_project_id_status_updated_at ON runs(project_id, status, updated_at);
CREATE INDEX IF NOT EXISTS idx_sessions_conversation_id_status ON sessions(conversation_id, status);
CREATE INDEX IF NOT EXISTS idx_session_messages_session_id_created_at ON session_messages(session_id, created_at);
CREATE INDEX IF NOT EXISTS idx_session_bindings_conversation_id_thread_id_status ON session_bindings(conversation_id, thread_id, status);
CREATE INDEX IF NOT EXISTS idx_session_bindings_session_id_status_created_at ON session_bindings(session_id, status, created_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_inbound_receipts_conversation_source_message
    ON inbound_receipts(conversation_id, connector_id, account_id, thread_id, source_message_id);
CREATE INDEX IF NOT EXISTS idx_approvals_run_id_status ON approvals(run_id, status);
CREATE INDEX IF NOT EXISTS idx_memory_items_project_id_agent_id_scope ON memory_items(project_id, agent_id, scope);
CREATE INDEX IF NOT EXISTS idx_runs_session_id_status_updated_at ON runs(session_id, status, updated_at);
CREATE INDEX IF NOT EXISTS idx_run_summaries_project_id_run_id ON run_summaries(project_id, run_id);
CREATE INDEX IF NOT EXISTS idx_schedules_enabled_next_run_at ON schedules(enabled, next_run_at);
CREATE INDEX IF NOT EXISTS idx_schedule_occurrences_active_schedule_created_at
    ON schedule_occurrences(schedule_id, created_at DESC)
    WHERE status IN ('dispatching', 'active', 'needs_approval');
CREATE INDEX IF NOT EXISTS idx_schedule_occurrences_status_updated_at ON schedule_occurrences(status, updated_at);
CREATE INDEX IF NOT EXISTS idx_schedule_occurrences_run_id ON schedule_occurrences(run_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_runs_one_active_root_per_conversation
    ON runs(conversation_id)
    WHERE parent_run_id IS NULL AND status IN ('pending', 'active', 'needs_approval');
