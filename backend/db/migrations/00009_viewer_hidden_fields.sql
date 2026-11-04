-- +goose Up

-- Photo details the user chose not to see in the viewer. The list holds hidden
-- fields rather than shown ones, so that fields added later appear by default.
ALTER TABLE users ADD COLUMN viewer_hidden_fields text[] NOT NULL DEFAULT '{}';

-- +goose Down

ALTER TABLE users DROP COLUMN viewer_hidden_fields;
