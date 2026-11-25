-- +goose Up

-- A 64-bit difference hash of a photo's viewer preview (nine by eight grey
-- pixels, one bit for each pixel brighter than its right neighbour). Copies
-- that were resized or saved again differ from the original in a few bits.
-- NULL until the photo's preview has been read.
ALTER TABLE photos ADD COLUMN phash bigint;

-- The photos the search for similar photos has looked at, with the time of
-- the vector it used. A new vector for the same photo (another model, or the
-- file changed) has another time, so the photo is looked at again.
CREATE TABLE similar_checks (
    photo_id     bigint NOT NULL REFERENCES photos (id) ON DELETE CASCADE,
    model        text NOT NULL,
    embedding_at timestamptz NOT NULL,
    PRIMARY KEY (photo_id, model)
);

-- Two photos of one owner that look alike. A duplicate is a copy, an edit or a
-- frame that is nearly identical; similar is another shot of the same scene.
-- Keyed by the photos and not by the model, so a pair the owner has marked as
-- not the same stays that way when the model changes.
CREATE TABLE similar_pairs (
    photo_a      bigint NOT NULL REFERENCES photos (id) ON DELETE CASCADE,
    photo_b      bigint NOT NULL REFERENCES photos (id) ON DELETE CASCADE,
    tier         text NOT NULL CHECK (tier IN ('duplicate', 'similar')),
    distance     double precision NOT NULL,
    hash_bits    smallint,
    seconds_apart double precision,
    dismissed    boolean NOT NULL DEFAULT false,
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (photo_a, photo_b),
    CHECK (photo_a < photo_b)
);

CREATE INDEX similar_pairs_photo_b_idx ON similar_pairs (photo_b);

-- +goose Down

DROP TABLE similar_pairs;
DROP TABLE similar_checks;
ALTER TABLE photos DROP COLUMN phash;
