
--   docker run --rm -d --name worm-pg -e POSTGRES_PASSWORD=postgres -p 5432:5432 postgres:16
--   psql "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable" -f querier/testdata/schema.sql
CREATE TYPE user_role AS ENUM ('admin', 'member', 'viewer');

CREATE TABLE organizations (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name                TEXT NOT NULL,
    domain              TEXT UNIQUE,
    attendance_enabled  BOOLEAN NOT NULL DEFAULT FALSE,
    created_at          TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE users (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    email       TEXT NOT NULL,
    role        user_role NOT NULL DEFAULT 'member',
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (org_id, email)
);
