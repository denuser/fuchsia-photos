-- +goose Up

-- The size of tiles in photo grids, in both modes: the height a row that
-- keeps each photo's shape aims at, and the side of a square. The interface
-- turns the level into pixels for the width of the grid.
ALTER TABLE users ADD COLUMN grid_size text NOT NULL DEFAULT 'medium'
    CHECK (grid_size IN ('small', 'medium', 'large'));

-- +goose Down

ALTER TABLE users DROP COLUMN grid_size;
