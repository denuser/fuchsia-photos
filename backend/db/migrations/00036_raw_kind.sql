-- +goose Up

-- Whether a camera RAW is hidden under the photo: its own RAW of a RAW + JPEG
-- pair, or the RAW its original carried when the photo is an edited copy.
-- LinkPhotoVersion sets it on the photo shown at the top.
ALTER TABLE photos ADD COLUMN has_raw boolean NOT NULL DEFAULT false;

UPDATE photos SET has_raw = true
WHERE id IN (SELECT photo_top(shown_id) FROM photo_links WHERE kind = 'raw');

-- Whether a MIME type is a camera RAW. Keep it in step with
-- backend/internal/storage/raw.go.
-- +goose StatementBegin
CREATE FUNCTION is_raw_mime(mime text)
RETURNS boolean
LANGUAGE sql IMMUTABLE PARALLEL SAFE
AS $$
    SELECT mime IN ('image/x-nikon-nef', 'image/x-nikon-nrw', 'image/x-canon-cr2', 'image/x-canon-cr3',
                    'image/x-sony-arw', 'image/x-adobe-dng', 'image/x-fuji-raf', 'image/x-olympus-orf',
                    'image/x-panasonic-rw2', 'image/x-pentax-pef', 'image/x-samsung-srw')
$$;
-- +goose StatementEnd

-- The kinds an item of the library has, for the media kind filter of the
-- timeline: one of photo_kind's, and raw for a camera RAW or a photo with
-- one hidden under it. A RAW + JPEG pair with motion is both motion and raw.
-- +goose StatementBegin
CREATE FUNCTION photo_kinds(mime text, has_motion boolean, has_raw boolean)
RETURNS text[]
LANGUAGE sql IMMUTABLE PARALLEL SAFE
AS $$
    SELECT CASE
               WHEN has_raw OR is_raw_mime(mime) THEN ARRAY[photo_kind(mime, has_motion), 'raw']
               ELSE ARRAY[photo_kind(mime, has_motion)]
           END
$$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION photo_kinds(text, boolean, boolean);
DROP FUNCTION is_raw_mime(text);
ALTER TABLE photos DROP COLUMN has_raw;
