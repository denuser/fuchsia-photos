-- name: GetPreviewSettings :one
SELECT * FROM preview_settings;

-- name: UpdatePreviewSettings :one
UPDATE preview_settings
SET format = @format,
    webp_quality = @webp_quality,
    webp_effort = @webp_effort,
    jpeg_quality = @jpeg_quality,
    grid_edge = @grid_edge,
    viewer_edge = @viewer_edge
RETURNING *;
