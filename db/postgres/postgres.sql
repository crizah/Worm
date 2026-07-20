CREATE TABLE IF NOT EXIST worm (
    version VARCHAR NOT NULL PRIMARY KEY,
    description VARCHAR NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)

INSERT INTO worm(
    version, description
) VALUES ($1, $2)