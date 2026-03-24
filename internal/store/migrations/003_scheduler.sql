-- 003_scheduler.sql: adds the schedules table.
-- The scheduler feature is NOT active in Milestone 3. This migration exists
-- so downstream packages (e.g. internal/telegram) can compile against a
-- complete schema. No triggers, no views, no seed data.

CREATE TABLE IF NOT EXISTS schedules (
    id          TEXT PRIMARY KEY,
    team_id     TEXT NOT NULL,
    cron_expr   TEXT NOT NULL,
    enabled     INTEGER NOT NULL DEFAULT 0,
    created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);
