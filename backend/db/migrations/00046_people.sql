-- +goose Up
CREATE TABLE people (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    owner_id bigint NOT NULL REFERENCES users ON DELETE CASCADE,
    name text NOT NULL DEFAULT ''
);

CREATE TABLE face_scans (
    photo_id bigint PRIMARY KEY REFERENCES photos ON DELETE CASCADE,
    model text NOT NULL,
    retry_at timestamptz,
    sha256 bytea NOT NULL
);

CREATE TABLE photo_faces (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    photo_id bigint NOT NULL REFERENCES face_scans ON DELETE CASCADE,
    person_id bigint REFERENCES people ON DELETE SET NULL,
    box real[] NOT NULL CHECK (cardinality(box) = 4),
    embedding halfvec(128) NOT NULL
);
CREATE INDEX photo_faces_person_idx ON photo_faces(person_id);
CREATE INDEX photo_faces_photo_idx ON photo_faces(photo_id);

CREATE VIEW visible_photo_faces AS
SELECT f.*, p.owner_id, s.model
FROM photo_faces f
JOIN face_scans s ON s.photo_id = f.photo_id
JOIN photos p ON p.id = f.photo_id AND p.sha256 = s.sha256
WHERE p.status = 'ready' AND p.deleted_at IS NULL AND p.purged_at IS NULL
AND NOT EXISTS (SELECT FROM photo_links l WHERE l.hidden_id = p.id);

-- +goose StatementBegin
CREATE FUNCTION clear_photo_faces() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    DELETE FROM face_scans WHERE photo_id = NEW.id;
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER clear_photo_faces AFTER UPDATE OF purged_at, sha256 ON photos
FOR EACH ROW WHEN (NEW.purged_at IS DISTINCT FROM OLD.purged_at OR NEW.sha256 IS DISTINCT FROM OLD.sha256)
EXECUTE FUNCTION clear_photo_faces();

-- +goose Down
DROP TRIGGER clear_photo_faces ON photos;
DROP FUNCTION clear_photo_faces();
DROP VIEW visible_photo_faces;
DROP TABLE photo_faces;
DROP TABLE face_scans;
DROP TABLE people;
