-- name: ListTags :many
SELECT t.id, t.name,
       (SELECT count(*)
        FROM photo_tags pt JOIN photos p ON p.id = pt.photo_id
        WHERE pt.tag_id = t.id AND p.deleted_at IS NULL)::bigint AS photo_count
FROM tags t
WHERE t.owner_id = @owner_id
ORDER BY lower(t.name);

-- name: GetTagByName :one
SELECT id, name FROM tags WHERE owner_id = @owner_id AND lower(name) = lower(@name);

-- name: CreateTag :one
INSERT INTO tags (owner_id, name) VALUES (@owner_id, @name)
ON CONFLICT (owner_id, lower(name)) DO NOTHING
RETURNING id, name;

-- name: RenameTag :execrows
UPDATE tags SET name = @name WHERE id = @id AND owner_id = @owner_id;

-- name: DeleteTag :execrows
DELETE FROM tags WHERE id = @id AND owner_id = @owner_id;

-- name: ListPhotoTags :many
SELECT t.id, t.name
FROM photo_tags pt JOIN tags t ON t.id = pt.tag_id
WHERE pt.photo_id = @photo_id AND t.owner_id = @owner_id
ORDER BY lower(t.name);

-- name: AddPhotoTags :exec
INSERT INTO photo_tags (tag_id, photo_id)
SELECT t.id, p.id
FROM tags t
JOIN photos p ON p.owner_id = t.owner_id
WHERE t.owner_id = @owner_id
  AND t.id = ANY (@tag_ids::bigint[])
  AND p.id = ANY (@photo_ids::bigint[])
ON CONFLICT DO NOTHING;

-- name: RemovePhotoTags :exec
DELETE FROM photo_tags pt
USING tags t
WHERE pt.tag_id = t.id
  AND t.owner_id = @owner_id
  AND t.id = ANY (@tag_ids::bigint[])
  AND pt.photo_id = ANY (@photo_ids::bigint[]);
