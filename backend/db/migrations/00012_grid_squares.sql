-- +goose Up

-- Whether photo grids show square tiles of one size instead of rows that keep
-- each photo's shape. The size comes from the grid preview size.
ALTER TABLE users ADD COLUMN grid_squares boolean NOT NULL DEFAULT false;

-- +goose Down

ALTER TABLE users DROP COLUMN grid_squares;
