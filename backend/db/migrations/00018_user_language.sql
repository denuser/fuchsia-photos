-- +goose Up

-- The interface language the user chose. "auto" follows the browser's
-- language, falling back to English when the interface does not support it.
ALTER TABLE users ADD COLUMN language text NOT NULL DEFAULT 'auto'
    CHECK (language IN ('auto', 'en', 'ru'));

-- +goose Down

ALTER TABLE users DROP COLUMN language;
