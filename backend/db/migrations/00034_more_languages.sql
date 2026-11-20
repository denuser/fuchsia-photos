-- +goose Up

-- The interface has ten languages since 2026-11-20.
ALTER TABLE users DROP CONSTRAINT users_language_check;
ALTER TABLE users ADD CONSTRAINT users_language_check
    CHECK (language IN ('auto', 'en', 'ru', 'zh', 'es', 'pt', 'de', 'fr', 'ja', 'ko', 'it'));

-- +goose Down

UPDATE users SET language = 'auto' WHERE language NOT IN ('auto', 'en', 'ru');
ALTER TABLE users DROP CONSTRAINT users_language_check;
ALTER TABLE users ADD CONSTRAINT users_language_check CHECK (language IN ('auto', 'en', 'ru'));
