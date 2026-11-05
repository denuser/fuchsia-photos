-- +goose Up

-- The functions that call exif_text keep the search path they were created
-- with: pg_dump restores with an empty one, and computes the generated columns
-- below while it loads the rows.

-- The text of an EXIF tag, trimmed, or NULL when the tag is missing, empty or
-- not a string.
-- +goose StatementBegin
CREATE FUNCTION exif_text(exif jsonb, tag text)
RETURNS text
LANGUAGE sql IMMUTABLE PARALLEL SAFE
AS $$
    SELECT CASE WHEN jsonb_typeof(exif -> tag) = 'string'
                THEN NULLIF(btrim(exif ->> tag, E' \t\r\n'), '')
           END
$$;
-- +goose StatementEnd

-- The camera's name from its make and model. Many cameras repeat the maker in
-- the model, as in "Canon" and "Canon EOS R5", so the make is left out then.
-- The viewer builds the same name in cameraName of PhotoViewer.vue.
-- +goose StatementBegin
CREATE FUNCTION exif_camera(exif jsonb)
RETURNS text
LANGUAGE sql IMMUTABLE PARALLEL SAFE
SET search_path FROM CURRENT
AS $$
    SELECT CASE
               WHEN make IS NULL OR model IS NULL THEN COALESCE(model, make)
               WHEN starts_with(lower(model), lower(split_part(make, ' ', 1))) THEN model
               ELSE make || ' ' || model
           END
    FROM (SELECT exif_text(exif, 'Make') AS make, exif_text(exif, 'Model') AS model) AS t
$$;
-- +goose StatementEnd

-- The lens's name. LensModel is the standard tag; older cameras keep the name
-- in Lens or LensID instead. Without print conversion exiftool gives some of
-- these as numbers, such as Canon's lens type or focal range ("24 105"), which
-- are left out.
-- +goose StatementBegin
CREATE FUNCTION exif_lens(exif jsonb)
RETURNS text
LANGUAGE sql IMMUTABLE PARALLEL SAFE
SET search_path FROM CURRENT
AS $$
    SELECT name
    FROM unnest(ARRAY[exif_text(exif, 'LensModel'), exif_text(exif, 'Lens'), exif_text(exif, 'LensID')])
         WITH ORDINALITY AS t(name, n)
    WHERE name !~ '^[0-9.\s]+$'
    ORDER BY n
    LIMIT 1
$$;
-- +goose StatementEnd

-- Kept by PostgreSQL itself, so photos processed before this migration get
-- them too, and a purged photo loses them with its EXIF.
ALTER TABLE photos
    ADD COLUMN camera text GENERATED ALWAYS AS (exif_camera(exif)) STORED,
    ADD COLUMN lens   text GENERATED ALWAYS AS (exif_lens(exif)) STORED;

CREATE INDEX photos_owner_camera_idx ON photos (owner_id, camera) WHERE deleted_at IS NULL;
CREATE INDEX photos_owner_lens_idx ON photos (owner_id, lens) WHERE deleted_at IS NULL;

-- +goose Down

ALTER TABLE photos DROP COLUMN camera, DROP COLUMN lens;
DROP FUNCTION exif_lens(jsonb);
DROP FUNCTION exif_camera(jsonb);
DROP FUNCTION exif_text(jsonb, text);
