-- internal/store/schema.sql
-- WAL mode is set programmatically on Open(); not in this file.

CREATE TABLE IF NOT EXISTS sessions (
    id          TEXT PRIMARY KEY,
    agent       TEXT NOT NULL,
    status      TEXT NOT NULL,
    prompt      TEXT,
    created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    finished_at DATETIME
);

CREATE TABLE IF NOT EXISTS hitl_pending (
    id          TEXT PRIMARY KEY,
    agent       TEXT NOT NULL,
    tool_name   TEXT,
    status      TEXT NOT NULL DEFAULT 'pending',
    created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    resolved_at DATETIME
);

CREATE TABLE IF NOT EXISTS cost_daily (
    date        TEXT PRIMARY KEY,
    total_usd   REAL NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS channel_state (
    channel_id      TEXT PRIMARY KEY,
    last_update_id  INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS provider_credentials (
    provider    TEXT PRIMARY KEY,
    data        TEXT NOT NULL,
    updated_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS messages (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    chat_id    INTEGER NOT NULL,
    role       TEXT    NOT NULL, -- 'user' or 'assistant'
    content    TEXT    NOT NULL,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_messages_chat_id ON messages (chat_id, created_at DESC);

CREATE TABLE IF NOT EXISTS jobs (
    id               TEXT PRIMARY KEY,
    kind             TEXT NOT NULL,
    target           TEXT NOT NULL,
    prompt           TEXT NOT NULL,
    schedule         TEXT NOT NULL,
    next_run_at      DATETIME NOT NULL,
    last_run_at      DATETIME,
    enabled          INTEGER NOT NULL DEFAULT 1,
    delete_after_run INTEGER NOT NULL DEFAULT 0,
    created_at       DATETIME NOT NULL DEFAULT (datetime('now'))
);
