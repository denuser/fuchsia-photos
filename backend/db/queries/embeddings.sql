-- Processed photos that have no vector for the model and have not failed too
-- often, oldest first. The ML job works through them in batches.
-- name: ListPhotosToEmbed :many
SELECT p.id, p.preview_format::text AS preview_format
FROM photos p
WHERE p.status = 'ready'
  AND p.purged_at IS NULL
  AND p.live_photo_id IS NULL
  AND p.preview_format IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM photo_embeddings e WHERE e.photo_id = p.id AND e.model = @model)
  AND NOT EXISTS (SELECT 1 FROM photo_embedding_failures f
                  WHERE f.photo_id = p.id AND f.model = @model AND f.attempts >= @max_attempts::integer)
  AND p.id > @after_id
ORDER BY p.id
LIMIT @max_rows;

-- The vector arrives as text, such as '[0.1,0.2]', so that no Go type for
-- pgvector is needed.
-- name: SaveEmbedding :exec
WITH failure_cleared AS (
    DELETE FROM photo_embedding_failures f WHERE f.photo_id = @photo_id AND f.model = @model
)
INSERT INTO photo_embeddings (photo_id, model, embedding)
SELECT p.id, @model, CAST(@embedding::text AS halfvec)
FROM photos p
WHERE p.id = @photo_id AND p.purged_at IS NULL
ON CONFLICT (photo_id, model) DO UPDATE SET embedding = EXCLUDED.embedding, created_at = now();

-- name: RecordEmbeddingFailure :exec
INSERT INTO photo_embedding_failures (photo_id, model, attempts, error)
VALUES (@photo_id, @model, 1, @error)
ON CONFLICT (photo_id, model) DO UPDATE
SET attempts = photo_embedding_failures.attempts + 1, error = EXCLUDED.error, failed_at = now();

-- name: CountEmbeddings :one
SELECT
    (SELECT count(*) FROM photos p
     WHERE p.status = 'ready' AND p.purged_at IS NULL AND p.preview_format IS NOT NULL
       AND p.live_photo_id IS NULL)::bigint AS photos,
    (SELECT count(*) FROM photo_embeddings e JOIN photos p ON p.id = e.photo_id
     WHERE e.model = @model AND p.purged_at IS NULL AND p.live_photo_id IS NULL)::bigint AS embedded,
    (SELECT count(*) FROM photo_embedding_failures f
     WHERE f.model = @model AND f.attempts >= @max_attempts::integer)::bigint AS failed;

-- name: ListEmbeddingFailures :many
SELECT f.photo_id, f.attempts, f.error, f.failed_at
FROM photo_embedding_failures f
WHERE f.model = @model
ORDER BY f.failed_at DESC
LIMIT @max_rows;

-- Photos the viewer can see, nearest to a vector first, and no farther than
-- max_distance if it is set. The administrator searches like any other user,
-- as on the timeline; all users' photos stay on the administration page.
-- name: SearchPhotosByVector :many
SELECT sqlc.embed(p), (e.embedding <=> CAST(@query::text AS halfvec))::float8 AS distance
FROM photo_embeddings e
JOIN photos p ON p.id = e.photo_id
WHERE e.model = @model
  AND p.status = 'ready'
  AND p.deleted_at IS NULL
  AND p.live_photo_id IS NULL
  AND photo_visible_to(p, @viewer_id::bigint, false)
  AND (sqlc.narg(max_distance)::float8 IS NULL
       OR e.embedding <=> CAST(@query::text AS halfvec) <= sqlc.narg(max_distance)::float8)
ORDER BY e.embedding <=> CAST(@query::text AS halfvec), p.id
LIMIT @max_rows;

-- Photos the viewer can see, nearest to the given photo first, without it.
-- name: SearchSimilarPhotos :many
SELECT sqlc.embed(p), (e.embedding <=> s.embedding)::float8 AS distance
FROM photo_embeddings s
JOIN photo_embeddings e ON e.model = s.model AND e.photo_id <> s.photo_id
JOIN photos p ON p.id = e.photo_id
WHERE s.photo_id = @photo_id
  AND s.model = @model
  AND p.status = 'ready'
  AND p.deleted_at IS NULL
  AND p.live_photo_id IS NULL
  AND photo_visible_to(p, @viewer_id::bigint, false)
ORDER BY e.embedding <=> s.embedding, p.id
LIMIT @max_rows;

-- name: HasEmbedding :one
SELECT EXISTS (SELECT 1 FROM photo_embeddings WHERE photo_id = @photo_id AND model = @model);
