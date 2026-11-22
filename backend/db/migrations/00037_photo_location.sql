-- +goose Up

-- The location a photo's EXIF holds, for the map. Keep both in step with
-- location() in backend/internal/metadata/metadata.go: both coordinates are
-- numbers within 90 and 180, a south or west reference letter turns a positive
-- value negative, and 0,0 means the camera had no fix. Unlike location(), they
-- do not read a coordinate written as text; exiftool is asked for numbers.
-- Both are null together.
-- +goose StatementBegin
CREATE FUNCTION exif_latitude(exif jsonb)
RETURNS double precision
LANGUAGE sql IMMUTABLE PARALLEL SAFE
AS $$
    SELECT CASE
        WHEN jsonb_typeof(exif -> 'GPSLatitude') = 'number' AND jsonb_typeof(exif -> 'GPSLongitude') = 'number'
             AND abs((exif -> 'GPSLatitude')::double precision) <= 90
             AND abs((exif -> 'GPSLongitude')::double precision) <= 180
             AND NOT ((exif -> 'GPSLatitude')::double precision = 0 AND (exif -> 'GPSLongitude')::double precision = 0)
        THEN CASE WHEN exif ->> 'GPSLatitudeRef' = 'S' AND (exif -> 'GPSLatitude')::double precision > 0
                  THEN -(exif -> 'GPSLatitude')::double precision
                  ELSE (exif -> 'GPSLatitude')::double precision END
    END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION exif_longitude(exif jsonb)
RETURNS double precision
LANGUAGE sql IMMUTABLE PARALLEL SAFE
AS $$
    SELECT CASE
        WHEN jsonb_typeof(exif -> 'GPSLatitude') = 'number' AND jsonb_typeof(exif -> 'GPSLongitude') = 'number'
             AND abs((exif -> 'GPSLatitude')::double precision) <= 90
             AND abs((exif -> 'GPSLongitude')::double precision) <= 180
             AND NOT ((exif -> 'GPSLatitude')::double precision = 0 AND (exif -> 'GPSLongitude')::double precision = 0)
        THEN CASE WHEN exif ->> 'GPSLongitudeRef' = 'W' AND (exif -> 'GPSLongitude')::double precision > 0
                  THEN -(exif -> 'GPSLongitude')::double precision
                  ELSE (exif -> 'GPSLongitude')::double precision END
    END
$$;
-- +goose StatementEnd

-- Where a photo was taken: its EXIF location, or else the one an import
-- recorded. Whether the API may show it is the administrator's setting.
ALTER TABLE photos
    ADD COLUMN latitude  double precision GENERATED ALWAYS AS (COALESCE(exif_latitude(exif), imported_latitude)) STORED,
    ADD COLUMN longitude double precision GENERATED ALWAYS AS (COALESCE(exif_longitude(exif), imported_longitude)) STORED;

CREATE INDEX photos_location_idx ON photos (latitude, longitude) WHERE latitude IS NOT NULL AND deleted_at IS NULL;

-- +goose Down
DROP INDEX photos_location_idx;
ALTER TABLE photos DROP COLUMN longitude, DROP COLUMN latitude;
DROP FUNCTION exif_longitude(jsonb);
DROP FUNCTION exif_latitude(jsonb);
