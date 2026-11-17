-- +goose Up

-- A photo's description, which everyone who can see the photo sees and only
-- its owner edits. Empty means none.
ALTER TABLE photos
    ADD COLUMN description text NOT NULL DEFAULT ''
        CONSTRAINT photos_description_length CHECK (char_length(description) <= 2000);

-- Cleans a description as it is stored: line breaks are kept, \r\n becoming
-- \n, other control characters become spaces, spaces and line breaks at both
-- ends go, and the text is cut at 2000 characters.
-- +goose StatementBegin
CREATE FUNCTION clean_description(value text)
RETURNS text
LANGUAGE sql IMMUTABLE
AS $$
    SELECT left(btrim(regexp_replace(replace(coalesce(value, ''), E'\r\n', E'\n'),
                                     E'[\\x01-\\x09\\x0b-\\x1f\\x7f]', ' ', 'g'),
                      E' \n'),
                2000)
$$;
-- +goose StatementEnd

-- Photos imported before descriptions were kept take theirs from the JSON
-- the import saved.
UPDATE photos p
SET description = clean_description(m.sidecar ->> 'description')
FROM imported_metadata m
WHERE m.photo_id = p.id
  AND p.purged_at IS NULL
  AND jsonb_typeof(m.sidecar -> 'description') = 'string'
  AND clean_description(m.sidecar ->> 'description') <> '';

-- +goose Down

DROP FUNCTION clean_description(text);
ALTER TABLE photos DROP COLUMN description;
