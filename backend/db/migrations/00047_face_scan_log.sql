-- +goose Up
ALTER TABLE face_scans
    ADD COLUMN scanned_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN error text;

-- The detector now retries a preview with stretched contrast, which finds faces in
-- old film scans. Scan again the photos where nothing was found.
DELETE FROM face_scans s
WHERE s.retry_at IS NULL AND NOT EXISTS (SELECT FROM photo_faces f WHERE f.photo_id = s.photo_id);

-- +goose Down
ALTER TABLE face_scans DROP COLUMN error, DROP COLUMN scanned_at;
