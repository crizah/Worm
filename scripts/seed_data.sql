-- adds a batch of new rows to whatever's already there (safe to run more than
-- once - each run generates fresh random/unique values, nothing gets wiped).
-- run against scripts/dev_schema.sql (via `make dev-up`), not querier/testdata/schema.sql

INSERT INTO organizations (name, domain, attendance_enabled)
SELECT
    'Org ' || substr(md5(random()::text || clock_timestamp()::text), 1, 10),
    'org-' || substr(md5(random()::text || clock_timestamp()::text), 1, 10) || '.example.com',
    (random() < 0.3)
FROM generate_series(1, 20);

INSERT INTO users (org_id, email, role)
SELECT
    (SELECT id FROM organizations ORDER BY random() LIMIT 1),
    'user-' || substr(md5(random()::text || clock_timestamp()::text), 1, 12) || '@example.com',
    (ARRAY['admin', 'member', 'viewer']::user_role[])[floor(random() * 3 + 1)]
FROM generate_series(1, 300)
ON CONFLICT DO NOTHING;

INSERT INTO teams (org_id, name)
SELECT
    (SELECT id FROM organizations ORDER BY random() LIMIT 1),
    'Team ' || substr(md5(random()::text || clock_timestamp()::text), 1, 10)
FROM generate_series(1, 60)
ON CONFLICT DO NOTHING;

INSERT INTO team_members (team_id, user_id)
SELECT
    (SELECT id FROM teams ORDER BY random() LIMIT 1),
    (SELECT id FROM users ORDER BY random() LIMIT 1)
FROM generate_series(1, 400)
ON CONFLICT DO NOTHING;

INSERT INTO projects (team_id, name, is_archived)
SELECT
    (SELECT id FROM teams ORDER BY random() LIMIT 1),
    'Project ' || substr(md5(random()::text || clock_timestamp()::text), 1, 10),
    (random() < 0.15)
FROM generate_series(1, 150)
ON CONFLICT DO NOTHING;

-- 1200 > the 500-row batch limit in data-migrator, so backfill has to do
-- multiple pages on this table
INSERT INTO tasks (project_id, assignee_id, title, status, metadata)
SELECT
    (SELECT id FROM projects ORDER BY random() LIMIT 1),
    CASE WHEN random() < 0.8 THEN (SELECT id FROM users ORDER BY random() LIMIT 1) ELSE NULL END,
    'Task ' || substr(md5(random()::text || clock_timestamp()::text), 1, 12),
    (ARRAY['todo', 'in_progress', 'done', 'archived']::task_status[])[floor(random() * 4 + 1)],
    jsonb_build_object('priority', floor(random() * 3 + 1))
FROM generate_series(1, 1200)
ON CONFLICT (assignee_id) WHERE status = 'in_progress' DO NOTHING;

INSERT INTO tags (org_id, name)
SELECT
    (SELECT id FROM organizations ORDER BY random() LIMIT 1),
    'tag-' || substr(md5(random()::text || clock_timestamp()::text), 1, 8)
FROM generate_series(1, 80)
ON CONFLICT DO NOTHING;

INSERT INTO task_tags (task_id, tag_id)
SELECT
    (SELECT id FROM tasks ORDER BY random() LIMIT 1),
    (SELECT id FROM tags ORDER BY random() LIMIT 1)
FROM generate_series(1, 2500)
ON CONFLICT DO NOTHING;

INSERT INTO task_comments (task_id, user_id, body)
SELECT
    (SELECT id FROM tasks ORDER BY random() LIMIT 1),
    (SELECT id FROM users ORDER BY random() LIMIT 1),
    'Comment ' || substr(md5(random()::text || clock_timestamp()::text), 1, 20)
FROM generate_series(1, 2000);

INSERT INTO audit_logs (org_id, user_id, action, detail)
SELECT
    (SELECT id FROM organizations ORDER BY random() LIMIT 1),
    CASE WHEN random() < 0.9 THEN (SELECT id FROM users ORDER BY random() LIMIT 1) ELSE NULL END,
    (ARRAY['created', 'updated', 'deleted']::audit_action[])[floor(random() * 3 + 1)],
    'auto-generated seed row'
FROM generate_series(1, 800);
