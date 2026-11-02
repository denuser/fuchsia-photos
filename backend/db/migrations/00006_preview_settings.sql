-- +goose Up

-- Settings for generating new previews, edited on the administration page. The
-- table holds exactly one row. The defaults come from the benchmark.
CREATE TABLE preview_settings (
    id           smallint PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    format       text NOT NULL DEFAULT 'webp' CHECK (format IN ('webp', 'jpeg')),
    webp_quality integer NOT NULL DEFAULT 75 CHECK (webp_quality BETWEEN 1 AND 100),
    webp_effort  integer NOT NULL DEFAULT 2 CHECK (webp_effort BETWEEN 0 AND 6),
    jpeg_quality integer NOT NULL DEFAULT 80 CHECK (jpeg_quality BETWEEN 1 AND 100),
    grid_edge    integer NOT NULL DEFAULT 400 CHECK (grid_edge BETWEEN 200 AND 800),
    viewer_edge  integer NOT NULL DEFAULT 2048 CHECK (viewer_edge BETWEEN 1024 AND 4096)
);

INSERT INTO preview_settings DEFAULT VALUES;

-- +goose Down

DROP TABLE preview_settings;
