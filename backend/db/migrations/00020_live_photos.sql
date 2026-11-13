-- +goose Up

-- An iPhone Live Photo is a photo and a video that share Apple's
-- ContentIdentifier tag. Kept by PostgreSQL from the stored EXIF, so files
-- processed before this migration can be paired too, and a purged one loses it
-- with its EXIF.
ALTER TABLE photos
    ADD COLUMN content_identifier text GENERATED ALWAYS AS (exif_text(exif, 'ContentIdentifier')) STORED;

-- The photo whose motion a paired video is. Such a video is left out of every
-- list and follows its photo into the trash and out of it, and the photo
-- plays it as its motion. A photo has at most one video.
ALTER TABLE photos
    ADD COLUMN live_photo_id bigint REFERENCES photos (id) ON DELETE SET NULL;

CREATE UNIQUE INDEX photos_live_photo_id_key ON photos (live_photo_id);
CREATE INDEX photos_owner_content_identifier_idx ON photos (owner_id, content_identifier)
    WHERE content_identifier IS NOT NULL;

-- +goose Down

ALTER TABLE photos DROP COLUMN live_photo_id, DROP COLUMN content_identifier;
