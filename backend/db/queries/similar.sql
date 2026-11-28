-- Photos whose viewer preview has not been hashed yet, oldest first.
-- name: ListPhotosToHash :many
SELECT p.id, p.preview_format::text AS preview_format, p.preview_version
FROM photos p
WHERE p.status = 'ready'
  AND p.purged_at IS NULL
  AND p.preview_rotation = p.rotation
  AND p.phash IS NULL
  AND p.preview_format IS NOT NULL
  AND p.mime LIKE 'image/%'
  AND p.id > @after_id
ORDER BY p.id
LIMIT @max_rows;

-- A version, when given, keeps the hash only for the previews it was made
-- from, so that a photo turned meanwhile is hashed again.
-- name: SavePerceptualHash :exec
UPDATE photos SET phash = @phash
WHERE id = @id AND purged_at IS NULL
  AND (sqlc.narg(preview_version)::integer IS NULL OR preview_version = sqlc.narg(preview_version)::integer);

-- Photos with a vector that the search has not looked at since the vector was
-- made. Hidden photos, the trash and videos are left out.
-- name: ListPhotosToCheck :many
SELECT e.photo_id, e.created_at AS embedding_at
FROM photo_embeddings e
JOIN photos p ON p.id = e.photo_id
LEFT JOIN similar_checks c ON c.photo_id = e.photo_id AND c.model = e.model
WHERE e.model = @model
  AND p.status = 'ready'
  AND p.deleted_at IS NULL
  AND p.purged_at IS NULL
  AND p.mime LIKE 'image/%'
  AND NOT EXISTS (SELECT 1 FROM photo_links l WHERE l.hidden_id = p.id)
  AND (c.photo_id IS NULL OR c.embedding_at <> e.created_at)
  AND e.photo_id > @after_id
ORDER BY e.photo_id
LIMIT @max_rows;

-- The other photos of the same owner whose vector is within max_distance of
-- this photo's, nearest first, with what the hash and the capture time add.
-- name: FindSimilarCandidates :many
SELECT p2.id AS other_id,
       (e2.embedding <=> e1.embedding)::float8 AS distance,
       p1.phash AS phash,
       p2.phash AS other_phash,
       p1.taken_at AS taken_at,
       p2.taken_at AS other_taken_at
FROM photos p1
JOIN photo_embeddings e1 ON e1.photo_id = p1.id AND e1.model = @model
JOIN photo_embeddings e2 ON e2.model = e1.model AND e2.photo_id <> e1.photo_id
JOIN photos p2 ON p2.id = e2.photo_id
WHERE p1.id = @photo_id
  AND p2.owner_id = p1.owner_id
  AND p2.status = 'ready'
  AND p2.deleted_at IS NULL
  AND p2.purged_at IS NULL
  AND p2.mime LIKE 'image/%'
  AND NOT EXISTS (SELECT 1 FROM photo_links l WHERE l.hidden_id = p2.id)
  AND (e2.embedding <=> e1.embedding) <= @max_distance::float8
ORDER BY e2.embedding <=> e1.embedding, p2.id
LIMIT @max_rows;

-- A fresh look at a photo replaces its open pairs. A pair the owner has marked
-- as not the same stays.
-- name: DeleteOpenSimilarPairsOf :exec
DELETE FROM similar_pairs
WHERE (photo_a = @photo_id OR photo_b = @photo_id) AND NOT dismissed;

-- name: UpsertSimilarPair :exec
INSERT INTO similar_pairs (photo_a, photo_b, tier, distance, hash_bits, seconds_apart)
VALUES (least(@photo_id::bigint, @other_id::bigint), greatest(@photo_id::bigint, @other_id::bigint),
        @tier, @distance, @hash_bits, @seconds_apart)
ON CONFLICT (photo_a, photo_b) DO UPDATE
SET tier = EXCLUDED.tier, distance = EXCLUDED.distance,
    hash_bits = EXCLUDED.hash_bits, seconds_apart = EXCLUDED.seconds_apart;

-- name: SaveSimilarCheck :exec
INSERT INTO similar_checks (photo_id, model, embedding_at)
VALUES (@photo_id, @model, @embedding_at)
ON CONFLICT (photo_id, model) DO UPDATE SET embedding_at = EXCLUDED.embedding_at;

-- The open pairs of one tier among a user's own photos that are in the library:
-- not in the trash, not hidden under another photo.
-- name: ListSimilarPairs :many
SELECT sp.photo_a, sp.photo_b, sp.distance
FROM similar_pairs sp
JOIN photos a ON a.id = sp.photo_a
JOIN photos b ON b.id = sp.photo_b
WHERE sp.tier = @tier
  AND NOT sp.dismissed
  AND a.owner_id = @owner_id
  AND b.owner_id = @owner_id
  AND a.deleted_at IS NULL AND b.deleted_at IS NULL
  AND a.purged_at IS NULL AND b.purged_at IS NULL
  AND NOT EXISTS (SELECT 1 FROM photo_links l WHERE l.hidden_id = a.id)
  AND NOT EXISTS (SELECT 1 FROM photo_links l WHERE l.hidden_id = b.id)
ORDER BY sp.photo_a, sp.photo_b;

-- Marks every pair among the given photos of one owner as not the same.
-- name: DismissSimilarPairs :execrows
UPDATE similar_pairs sp
SET dismissed = true
FROM photos a, photos b
WHERE a.id = sp.photo_a AND b.id = sp.photo_b
  AND a.owner_id = @owner_id AND b.owner_id = @owner_id
  AND sp.photo_a = ANY (@ids::bigint[]) AND sp.photo_b = ANY (@ids::bigint[])
  AND NOT sp.dismissed;

-- The photos of one owner among the given ones that are in the library: not in
-- the trash and not hidden under another photo.
-- name: ListLibraryPhotosByIDs :many
SELECT p.*
FROM photos p
WHERE p.id = ANY (@ids::bigint[])
  AND p.owner_id = @owner_id
  AND p.deleted_at IS NULL AND p.purged_at IS NULL
  AND NOT EXISTS (SELECT 1 FROM photo_links l WHERE l.hidden_id = p.id)
ORDER BY p.id;

-- What the owner did with the photos that go away, moved to the photo that
-- stays: albums (and a cover), tags, favorites, shares, the choice to show it to
-- everyone, and a description it has none of. Archive marks stay where they are.
-- name: MovePhotoMemberships :exec
WITH albums_moved AS (
    INSERT INTO album_photos (album_id, photo_id, added_at)
    SELECT ap.album_id, @keep_id::bigint, ap.added_at
    FROM album_photos ap
    WHERE ap.photo_id = ANY (@remove_ids::bigint[])
    ON CONFLICT DO NOTHING
),
covers_moved AS (
    UPDATE albums a SET cover_photo_id = @keep_id::bigint
    WHERE a.cover_photo_id = ANY (@remove_ids::bigint[])
),
tags_moved AS (
    INSERT INTO photo_tags (tag_id, photo_id)
    SELECT pt.tag_id, @keep_id::bigint
    FROM photo_tags pt
    WHERE pt.photo_id = ANY (@remove_ids::bigint[])
    ON CONFLICT DO NOTHING
),
favorites_moved AS (
    INSERT INTO photo_favorites (user_id, photo_id, created_at)
    SELECT f.user_id, @keep_id::bigint, f.created_at
    FROM photo_favorites f
    WHERE f.photo_id = ANY (@remove_ids::bigint[])
    ON CONFLICT DO NOTHING
),
shares_moved AS (
    INSERT INTO photo_shares (photo_id, user_id, created_at)
    SELECT @keep_id::bigint, s.user_id, s.created_at
    FROM photo_shares s
    WHERE s.photo_id = ANY (@remove_ids::bigint[])
    ON CONFLICT DO NOTHING
)
UPDATE photos k
SET visibility = CASE
        WHEN EXISTS (SELECT 1 FROM photos r WHERE r.id = ANY (@remove_ids::bigint[]) AND r.visibility = 'all')
        THEN 'all' ELSE k.visibility END,
    description = CASE
        WHEN k.description = '' THEN coalesce(
            (SELECT r.description FROM photos r
             WHERE r.id = ANY (@remove_ids::bigint[]) AND r.description <> '' ORDER BY r.id LIMIT 1), '')
        ELSE k.description END,
    updated_at = now()
WHERE k.id = @keep_id::bigint;
