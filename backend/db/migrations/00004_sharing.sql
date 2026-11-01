-- +goose Up
ALTER TABLE users ADD COLUMN shared_includes_albums boolean NOT NULL DEFAULT false;

ALTER TABLE albums
    ADD COLUMN visibility text NOT NULL DEFAULT 'private' CHECK (visibility IN ('private', 'all'));

CREATE TABLE photo_shares (
    photo_id   bigint NOT NULL REFERENCES photos (id) ON DELETE CASCADE,
    user_id    bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (photo_id, user_id)
);

CREATE INDEX photo_shares_user_id_idx ON photo_shares (user_id);

CREATE TABLE album_shares (
    album_id   bigint NOT NULL REFERENCES albums (id) ON DELETE CASCADE,
    user_id    bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (album_id, user_id)
);

CREATE INDEX album_shares_user_id_idx ON album_shares (user_id);

-- For lists that span several owners: shared photos and the administrator's view.
CREATE INDEX photos_all_timeline_idx ON photos (sort_at DESC, id DESC) WHERE deleted_at IS NULL;

-- +goose StatementBegin
CREATE FUNCTION album_visible_to(album albums, viewer_id bigint, viewer_is_admin boolean)
RETURNS boolean
LANGUAGE sql STABLE
AS $$
    SELECT viewer_is_admin
        OR album.owner_id = viewer_id
        OR album.visibility = 'all'
        OR EXISTS (SELECT 1 FROM album_shares s WHERE s.album_id = album.id AND s.user_id = viewer_id)
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION photo_shared_directly(photo photos, viewer_id bigint)
RETURNS boolean
LANGUAGE sql STABLE
AS $$
    SELECT photo.visibility = 'all'
        OR EXISTS (SELECT 1 FROM photo_shares s WHERE s.photo_id = photo.id AND s.user_id = viewer_id)
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION photo_in_visible_album(photo photos, viewer_id bigint)
RETURNS boolean
LANGUAGE sql STABLE
AS $$
    SELECT EXISTS (
        SELECT 1
        FROM album_photos ap
        JOIN albums a ON a.id = ap.album_id
        WHERE ap.photo_id = photo.id AND album_visible_to(a, viewer_id, false)
    )
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION photo_visible_to(photo photos, viewer_id bigint, viewer_is_admin boolean)
RETURNS boolean
LANGUAGE sql STABLE
AS $$
    SELECT viewer_is_admin
        OR photo.owner_id = viewer_id
        OR photo_shared_directly(photo, viewer_id)
        OR photo_in_visible_album(photo, viewer_id)
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION photo_visible_to(photo photos, viewer_id bigint, viewer_is_admin boolean)
RETURNS boolean
LANGUAGE sql STABLE
AS $$
    SELECT viewer_is_admin OR photo.owner_id = viewer_id OR photo.visibility = 'all'
$$;
-- +goose StatementEnd

DROP FUNCTION photo_in_visible_album(photos, bigint);
DROP FUNCTION photo_shared_directly(photos, bigint);
DROP FUNCTION album_visible_to(albums, bigint, boolean);
DROP INDEX photos_all_timeline_idx;
DROP TABLE album_shares;
DROP TABLE photo_shares;
ALTER TABLE albums DROP COLUMN visibility;
ALTER TABLE users DROP COLUMN shared_includes_albums;
