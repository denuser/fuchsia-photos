-- +goose Up

-- What photo metadata the API returns, edited on the administration page. The
-- table holds exactly one row. Locations stay hidden unless an administrator
-- turns them on, because they can reveal where people live.
CREATE TABLE metadata_settings (
    id            smallint PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    show_location boolean NOT NULL DEFAULT false
);

INSERT INTO metadata_settings DEFAULT VALUES;

-- +goose Down

DROP TABLE metadata_settings;
