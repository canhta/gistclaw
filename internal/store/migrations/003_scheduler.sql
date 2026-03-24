-- 003_scheduler.sql: adds the schedules table.

CREATE TABLE IF NOT EXISTS schedules (
    id          TEXT PRIMARY KEY,
    team_id     TEXT NOT NULL,
    agent_id    TEXT NOT NULL DEFAULT 'coordinator',
    objective   TEXT NOT NULL DEFAULT 'scheduled run',
    cron_expr   TEXT NOT NULL,
    enabled     INTEGER NOT NULL DEFAULT 0,
    last_run_at DATETIME,
    created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);
