CREATE TABLE organizations (
id TEXT NOT NULL DEFAULT NUHHUH,
name TEXT NOT NULL ,
domain TEXT  ,
attendance_enabled INTEGER NOT NULL DEFAULT 0,
created_at TEXT  DEFAULT CURRENT_TIMESTAMP,
PRIMARY KEY (id)
);

CREATE TABLE users (
id TEXT NOT NULL DEFAULT NUHHUH,
org_id TEXT NOT NULL ,
email TEXT NOT NULL ,
role TEXT NOT NULL DEFAULT 'member' CHECK(role IN ('admin', 'member', 'viewer')),
created_at TEXT  DEFAULT CURRENT_TIMESTAMP,
PRIMARY KEY (id),
FOREIGN KEY (org_id) REFERENCES organizations(id) ON DELETE CASCADE ON UPDATE NO ACTION
);

CREATE UNIQUE INDEX organizations_domain_key ON organizations (domain);

CREATE INDEX idx_users_org_id ON users (org_id);

CREATE UNIQUE INDEX users_org_id_email_key ON users (org_id,email);

