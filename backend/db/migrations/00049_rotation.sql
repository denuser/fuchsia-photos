-- +goose Up

-- A turn the owner chose, clockwise on top of the EXIF orientation, and the
-- turn the preview files on disk were made with. The original never changes.
ALTER TABLE photos
    ADD COLUMN rotation smallint NOT NULL DEFAULT 0 CHECK (rotation IN (0, 90, 180, 270)),
    ADD COLUMN preview_rotation smallint NOT NULL DEFAULT 0 CHECK (preview_rotation IN (0, 90, 180, 270));

-- A photo turned after its faces were found is scanned again; the scan
-- matches the new faces to the stored ones instead of replacing them.
ALTER TABLE face_scans ADD COLUMN rescan boolean NOT NULL DEFAULT false;

-- A face the owner placed or named that a rescan found no match for keeps its
-- old vector; the photo's scan lookup lists it.
ALTER TABLE photo_faces ADD COLUMN unmatched boolean NOT NULL DEFAULT false;

-- A new file, such as a library file edited in another program, shows the
-- way it was saved: the turn chosen for the old file no longer applies.
-- +goose StatementBegin
CREATE FUNCTION reset_photo_rotation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    NEW.rotation := 0;
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER reset_photo_rotation BEFORE UPDATE OF sha256 ON photos
FOR EACH ROW WHEN (NEW.sha256 IS DISTINCT FROM OLD.sha256)
EXECUTE FUNCTION reset_photo_rotation();

-- +goose Down

DROP TRIGGER reset_photo_rotation ON photos;
DROP FUNCTION reset_photo_rotation();
ALTER TABLE photo_faces DROP COLUMN unmatched;
ALTER TABLE face_scans DROP COLUMN rescan;
ALTER TABLE photos DROP COLUMN preview_rotation, DROP COLUMN rotation;
