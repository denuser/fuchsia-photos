-- +goose Up

-- Whether the navigation drawer stays hidden on a phone until the menu button
-- opens it. On by default: a permanent rail takes a sixth of a phone's width.
ALTER TABLE users ADD COLUMN navigation_hidden_on_phones boolean NOT NULL DEFAULT true;

-- +goose Down

ALTER TABLE users DROP COLUMN navigation_hidden_on_phones;
