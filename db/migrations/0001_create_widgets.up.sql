-- gen_random_uuid() is native since Postgres 13; kept for older targets.
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS widgets (
    namespace        TEXT NOT NULL,
    name             TEXT NOT NULL,
    uid              UUID NOT NULL DEFAULT gen_random_uuid(),
    resource_version BIGINT NOT NULL DEFAULT 1,
    size             TEXT NOT NULL,
    color            TEXT NOT NULL DEFAULT '',
    phase            TEXT NOT NULL DEFAULT 'Pending',
    message          TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at       TIMESTAMPTZ,
    PRIMARY KEY (namespace, name)
);

-- Soft-deleted rows keep their (namespace, name) history briefly for audit;
-- a partial unique index still prevents live duplicates.
CREATE UNIQUE INDEX IF NOT EXISTS widgets_live_unique
    ON widgets (namespace, name)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS widgets_namespace_idx ON widgets (namespace) WHERE deleted_at IS NULL;
