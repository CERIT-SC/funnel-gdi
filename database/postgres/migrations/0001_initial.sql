-- 0001: initial schema (tasks and nodes).
--
-- Migrations run in order of their number, each once and in its own
-- transaction; applied ones are recorded in the schema_version table. Never
-- change a migration that has been released: add a new file instead.
--
-- IF NOT EXISTS: databases created by Funnel versions before migrations
-- already have these tables and indices.

CREATE TABLE IF NOT EXISTS tasks (
    id VARCHAR(255) PRIMARY KEY,
    state VARCHAR(50) NOT NULL,
    owner VARCHAR(255),
    creation_time TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    version BIGINT DEFAULT 0 NOT NULL,
    data JSONB
);

CREATE TABLE IF NOT EXISTS nodes (
    id VARCHAR(255) PRIMARY KEY,
    state VARCHAR(50) NOT NULL,
    owner VARCHAR(255),
    version BIGINT DEFAULT 0 NOT NULL,
    last_heartbeat TIMESTAMP WITH TIME ZONE,
    data JSONB
);

CREATE INDEX IF NOT EXISTS idx_tasks_state ON tasks (state);
CREATE INDEX IF NOT EXISTS idx_tasks_owner ON tasks (owner);
CREATE INDEX IF NOT EXISTS idx_tasks_creation_time ON tasks (creation_time DESC);
CREATE INDEX IF NOT EXISTS idx_nodes_state ON nodes (state);
CREATE INDEX IF NOT EXISTS idx_nodes_owner ON nodes (owner);
