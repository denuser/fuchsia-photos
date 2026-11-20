-- +goose Up
ALTER TABLE photo_links DROP CONSTRAINT photo_links_kind_check;
ALTER TABLE photo_links ADD CONSTRAINT photo_links_kind_check CHECK (kind IN ('live', 'edited', 'raw'));
CREATE UNIQUE INDEX photo_links_one_raw ON photo_links (shown_id) WHERE kind = 'raw';

CREATE INDEX photos_raw_pair_name ON photos (owner_id, (lower(regexp_replace(original_name, '\.[^.]+$', ''))))
WHERE mime IN ('image/jpeg', 'image/x-nikon-nef', 'image/x-nikon-nrw', 'image/x-canon-cr2',
               'image/x-canon-cr3', 'image/x-sony-arw', 'image/x-adobe-dng', 'image/x-fuji-raf',
               'image/x-olympus-orf', 'image/x-panasonic-rw2', 'image/x-pentax-pef', 'image/x-samsung-srw')
  AND status = 'ready' AND deleted_at IS NULL;

-- +goose Down
DROP INDEX photos_raw_pair_name;
DROP INDEX photo_links_one_raw;
DELETE FROM photo_links WHERE kind = 'raw';
ALTER TABLE photo_links DROP CONSTRAINT photo_links_kind_check;
ALTER TABLE photo_links ADD CONSTRAINT photo_links_kind_check CHECK (kind IN ('live', 'edited'));
