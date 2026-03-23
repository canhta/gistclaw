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

CREATE TABLE IF NOT EXISTS runs (
    id TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    team_id TEXT,
    parent_run_id TEXT,
    objective TEXT,
    workspace_root TEXT,
    status TEXT NOT NULL DEFAULT 'pending',
    execution_snapshot_json BLOB,
    input_tokens INTEGER DEFAULT 0,
    output_tokens INTEGER DEFAULT 0,
    model_lane TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS delegations (
    id TEXT PRIMARY KEY,
    root_run_id TEXT NOT NULL,
    parent_run_id TEXT NOT NULL,
    child_run_id TEXT,
    target_agent_id TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued',
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
    target_path TEXT,
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
    verification_status TEXT,
    approval_count INTEGER DEFAULT 0,
    budget_status TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS memory_items (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,
    scope TEXT NOT NULL DEFAULT 'local',
    content TEXT NOT NULL,
    source TEXT NOT NULL,
    provenance TEXT,
    confidence REAL DEFAULT 1.0,
    dedupe_key TEXT,
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

CREATE TABLE IF NOT EXISTS run_summaries (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL UNIQUE,
    content TEXT NOT NULL,
    token_count INTEGER DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_events_run_id_created_at ON events(run_id, created_at);
CREATE INDEX IF NOT EXISTS idx_runs_conversation_id_status ON runs(conversation_id, status);
CREATE INDEX IF NOT EXISTS idx_delegations_parent_run_id_status ON delegations(parent_run_id, status);
CREATE INDEX IF NOT EXISTS idx_approvals_run_id_status ON approvals(run_id, status);
CREATE INDEX IF NOT EXISTS idx_memory_items_agent_id_scope ON memory_items(agent_id, scope);
