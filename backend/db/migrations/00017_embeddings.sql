-- +goose Up

CREATE EXTENSION IF NOT EXISTS vector;

-- Image embeddings from the ML service. The model name includes the version of
-- the service's preprocessing, so vectors made in different ways never mix.
-- Models differ in their vector size, so the column has none.
CREATE TABLE photo_embeddings (
    photo_id   bigint NOT NULL REFERENCES photos (id) ON DELETE CASCADE,
    model      text NOT NULL,
    embedding  halfvec NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (photo_id, model)
);

-- Photos the ML service could not embed. After a few attempts a photo is
-- skipped, so a file the service cannot read does not stop the others.
CREATE TABLE photo_embedding_failures (
    photo_id  bigint NOT NULL REFERENCES photos (id) ON DELETE CASCADE,
    model     text NOT NULL,
    attempts  integer NOT NULL,
    error     text NOT NULL,
    failed_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (photo_id, model)
);

-- +goose Down

DROP TABLE photo_embedding_failures;
DROP TABLE photo_embeddings;
DROP EXTENSION vector;
