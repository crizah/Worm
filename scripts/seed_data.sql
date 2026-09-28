-- adds a batch of new rows to whatever's already there (safe to run more than
-- once - each run generates fresh random/unique values, nothing gets wiped).
-- run against scripts/dev_schema.sql (via `make dev-up`), not querier/testdata/schema.sql
--
-- NOTE: parent ids are picked by materializing each parent table's ids into an
-- array once (via a CTE), then indexing into that array with random() called
-- directly in the SELECT list. A bare scalar subquery (or even LATERAL, if it
-- doesn't truly reference the outer row) can get planned as a single InitPlan
-- and evaluated ONCE for the whole statement - every generated row then ends
-- up with the exact same "random" parent. A plain volatile-function call in
-- the target list has no such ambiguity: it's guaranteed to run per row.

INSERT INTO organizations (name, domain, attendance_enabled)
SELECT
    'Org ' || substr(md5(random()::text || clock_timestamp()::text), 1, 10),
    'org-' || substr(md5(random()::text || clock_timestamp()::text), 1, 10) || '.example.com',
    (random() < 0.3)
FROM generate_series(1, 20);

WITH pool AS (SELECT array_agg(id) AS ids FROM organizations)
INSERT INTO users (org_id, email, role)
SELECT
    pool.ids[1 + floor(random() * array_length(pool.ids, 1))::int],
    'user-' || substr(md5(random()::text || clock_timestamp()::text), 1, 12) || '@example.com',
    (ARRAY['admin', 'member', 'viewer']::user_role[])[floor(random() * 3 + 1)]
FROM generate_series(1, 300) AS gs
CROSS JOIN pool
ON CONFLICT DO NOTHING;

WITH pool AS (SELECT array_agg(id) AS ids FROM organizations)
INSERT INTO teams (org_id, name)
SELECT
    pool.ids[1 + floor(random() * array_length(pool.ids, 1))::int],
    'Team ' || substr(md5(random()::text || clock_timestamp()::text), 1, 10)
FROM generate_series(1, 60) AS gs
CROSS JOIN pool
ON CONFLICT DO NOTHING;

WITH team_pool AS (SELECT array_agg(id) AS ids FROM teams),
     user_pool AS (SELECT array_agg(id) AS ids FROM users)
INSERT INTO team_members (team_id, user_id)
SELECT
    team_pool.ids[1 + floor(random() * array_length(team_pool.ids, 1))::int],
    user_pool.ids[1 + floor(random() * array_length(user_pool.ids, 1))::int]
FROM generate_series(1, 400) AS gs
CROSS JOIN team_pool
CROSS JOIN user_pool
ON CONFLICT DO NOTHING;

WITH pool AS (SELECT array_agg(id) AS ids FROM teams)
INSERT INTO projects (team_id, name, is_archived)
SELECT
    pool.ids[1 + floor(random() * array_length(pool.ids, 1))::int],
    'Project ' || substr(md5(random()::text || clock_timestamp()::text), 1, 10),
    (random() < 0.15)
FROM generate_series(1, 150) AS gs
CROSS JOIN pool
ON CONFLICT DO NOTHING;

-- 1200 > the 500-row batch limit in data-migrator, so backfill has to do
-- multiple pages on this table
WITH proj_pool AS (SELECT array_agg(id) AS ids FROM projects),
     user_pool AS (SELECT array_agg(id) AS ids FROM users)
INSERT INTO tasks (project_id, assignee_id, title, status, metadata)
SELECT
    proj_pool.ids[1 + floor(random() * array_length(proj_pool.ids, 1))::int],
    CASE WHEN random() < 0.8
        THEN user_pool.ids[1 + floor(random() * array_length(user_pool.ids, 1))::int]
        ELSE NULL
    END,
    'Task ' || substr(md5(random()::text || clock_timestamp()::text), 1, 12),
    (ARRAY['todo', 'in_progress', 'done', 'archived']::task_status[])[floor(random() * 4 + 1)],
    jsonb_build_object('priority', floor(random() * 3 + 1))
FROM generate_series(1, 1200) AS gs
CROSS JOIN proj_pool
CROSS JOIN user_pool
ON CONFLICT (assignee_id) WHERE status = 'in_progress' DO NOTHING;

WITH pool AS (SELECT array_agg(id) AS ids FROM organizations)
INSERT INTO tags (org_id, name)
SELECT
    pool.ids[1 + floor(random() * array_length(pool.ids, 1))::int],
    'tag-' || substr(md5(random()::text || clock_timestamp()::text), 1, 8)
FROM generate_series(1, 80) AS gs
CROSS JOIN pool
ON CONFLICT DO NOTHING;

WITH task_pool AS (SELECT array_agg(id) AS ids FROM tasks),
     tag_pool AS (SELECT array_agg(id) AS ids FROM tags)
INSERT INTO task_tags (task_id, tag_id)
SELECT
    task_pool.ids[1 + floor(random() * array_length(task_pool.ids, 1))::int],
    tag_pool.ids[1 + floor(random() * array_length(tag_pool.ids, 1))::int]
FROM generate_series(1, 2500) AS gs
CROSS JOIN task_pool
CROSS JOIN tag_pool
ON CONFLICT DO NOTHING;

WITH task_pool AS (SELECT array_agg(id) AS ids FROM tasks),
     user_pool AS (SELECT array_agg(id) AS ids FROM users)
INSERT INTO task_comments (task_id, user_id, body)
SELECT
    task_pool.ids[1 + floor(random() * array_length(task_pool.ids, 1))::int],
    user_pool.ids[1 + floor(random() * array_length(user_pool.ids, 1))::int],
    'Comment ' || substr(md5(random()::text || clock_timestamp()::text), 1, 20)
FROM generate_series(1, 2000) AS gs
CROSS JOIN task_pool
CROSS JOIN user_pool;

WITH org_pool AS (SELECT array_agg(id) AS ids FROM organizations),
     user_pool AS (SELECT array_agg(id) AS ids FROM users)
INSERT INTO audit_logs (org_id, user_id, action, detail)
SELECT
    org_pool.ids[1 + floor(random() * array_length(org_pool.ids, 1))::int],
    CASE WHEN random() < 0.9
        THEN user_pool.ids[1 + floor(random() * array_length(user_pool.ids, 1))::int]
        ELSE NULL
    END,
    (ARRAY['created', 'updated', 'deleted']::audit_action[])[floor(random() * 3 + 1)],
    'auto-generated seed row'
FROM generate_series(1, 800) AS gs
CROSS JOIN org_pool
CROSS JOIN user_pool;
