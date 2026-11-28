-- +goose Up

-- How each face was found, and whether its owner placed it by hand. A face
-- without a group is either ignored by its owner (manual) or left out by the
-- retry setting (not manual).
ALTER TABLE photo_faces
    ADD COLUMN score real,
    ADD COLUMN retry boolean NOT NULL DEFAULT false,
    ADD COLUMN manual boolean NOT NULL DEFAULT false;

-- Until now a face had no group only when its owner ignored it: the scan
-- always assigned one, and a merge moved the faces before deleting the group.
UPDATE photo_faces SET manual = true WHERE person_id IS NULL;

-- The least detector score a face found only by the contrast retry needs to
-- join a group. Faces under it keep their vector and get no group.
CREATE TABLE face_settings (
    id              smallint PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    retry_min_score real NOT NULL DEFAULT 0.75 CHECK (retry_min_score > 0 AND retry_min_score < 1),
    -- Set when every owner's faces need grouping again; the application
    -- queues the regroupings at its next start and clears it.
    regroup_pending boolean NOT NULL DEFAULT false
);
-- The faces this migration keeps are still in the groups of the old rule.
INSERT INTO face_settings (regroup_pending) VALUES (true);

-- Faces scanned before have no score, and were grouped by a rule that split
-- most people. Scan the photos with faces again, except those with a face the
-- owner placed or ignored, or one in a named group: those keep their faces.
-- Photos without faces keep their scan, as a new one would find none again.
DELETE FROM face_scans s
WHERE EXISTS (SELECT FROM photo_faces f WHERE f.photo_id = s.photo_id)
AND NOT EXISTS (
    SELECT FROM photo_faces f LEFT JOIN people p ON p.id = f.person_id
    WHERE f.photo_id = s.photo_id AND (f.manual OR p.name <> '')
);
DELETE FROM people p
WHERE p.name = '' AND NOT EXISTS (SELECT FROM photo_faces f WHERE f.person_id = p.id);

-- +goose Down

DROP TABLE face_settings;
ALTER TABLE photo_faces DROP COLUMN manual, DROP COLUMN retry, DROP COLUMN score;
