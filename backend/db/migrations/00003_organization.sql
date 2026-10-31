-- +goose Up
CREATE TABLE albums (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    owner_id       bigint NOT NULL REFERENCES users (id),
    name           text NOT NULL,
    -- The cover chosen by the owner. Without one, the newest photo is the cover.
    cover_photo_id bigint REFERENCES photos (id) ON DELETE SET NULL,
    created_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX albums_owner_id_idx ON albums (owner_id, lower(name));

CREATE TABLE album_photos (
    album_id bigint NOT NULL REFERENCES albums (id) ON DELETE CASCADE,
    photo_id bigint NOT NULL REFERENCES photos (id) ON DELETE CASCADE,
    added_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (album_id, photo_id)
);

CREATE INDEX album_photos_photo_id_idx ON album_photos (photo_id);

CREATE TABLE tags (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    owner_id   bigint NOT NULL REFERENCES users (id),
    name       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX tags_owner_name_key ON tags (owner_id, lower(name));

CREATE TABLE photo_tags (
    tag_id   bigint NOT NULL REFERENCES tags (id) ON DELETE CASCADE,
    photo_id bigint NOT NULL REFERENCES photos (id) ON DELETE CASCADE,
    PRIMARY KEY (tag_id, photo_id)
);

CREATE INDEX photo_tags_photo_id_idx ON photo_tags (photo_id);

CREATE INDEX photos_trash_idx ON photos (owner_id, deleted_at DESC, id DESC) WHERE deleted_at IS NOT NULL;

-- +goose Down
DROP INDEX photos_trash_idx;
DROP TABLE photo_tags;
DROP TABLE tags;
DROP TABLE album_photos;
DROP TABLE albums;
