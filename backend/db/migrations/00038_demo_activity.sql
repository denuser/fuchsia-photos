-- +goose Up

-- Demo baseline restores can replay this migration while preserving the journal.
CREATE SCHEMA IF NOT EXISTS activity;

CREATE TABLE IF NOT EXISTS activity.reset_state (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    generation bigint NOT NULL DEFAULT 1
);

INSERT INTO activity.reset_state DEFAULT VALUES ON CONFLICT DO NOTHING;

CREATE TABLE IF NOT EXISTS activity.events (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    occurred_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    action text NOT NULL CHECK (action IN ('login', 'upload')),
    outcome text NOT NULL,
    http_status integer NOT NULL,
    account_id bigint,
    session_id text,
    browser text NOT NULL,
    os text NOT NULL,
    media_type text,
    size_bytes bigint,
    media_id bigint,
    generation bigint
);

CREATE INDEX IF NOT EXISTS activity_events_occurred_at ON activity.events (occurred_at);

ALTER TABLE sessions ADD COLUMN activity_id text;

-- +goose Down

ALTER TABLE sessions DROP COLUMN activity_id;
DROP TABLE activity.events;
DROP TABLE activity.reset_state;
DROP SCHEMA activity;
