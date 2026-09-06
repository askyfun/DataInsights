-- +goose Up
-- Initial schema, transcribed from the former model.CreateTables DDL.
-- IF NOT EXISTS is kept so that existing databases are compatible.

CREATE TABLE IF NOT EXISTS bi_datasource (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    type VARCHAR(50) NOT NULL DEFAULT 'postgresql',
    host VARCHAR(255) NOT NULL,
    port INTEGER NOT NULL,
    database_name VARCHAR(255) NOT NULL,
    username VARCHAR(255) NOT NULL,
    password VARCHAR(255) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS bi_dataset (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    datasource_id INTEGER REFERENCES bi_datasource(id) ON DELETE CASCADE,
    table_name VARCHAR(255),
    query_sql TEXT,
    query_type VARCHAR(50) NOT NULL DEFAULT 'table',
    mode VARCHAR(50) NOT NULL DEFAULT 'direct',
    accelerate_config JSONB,
    description TEXT,
    tags JSONB DEFAULT '[]',
    refresh_strategy JSONB,
    preview_data JSONB,
    quality_rules JSONB DEFAULT '[]',
    columns JSONB DEFAULT '[]',
    shard_enabled BOOLEAN DEFAULT FALSE,
    shard_keys JSONB DEFAULT '[]',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS bi_dataset_lineage (
    id SERIAL PRIMARY KEY,
    upstream_dataset_id INTEGER REFERENCES bi_dataset(id) ON DELETE CASCADE,
    downstream_dataset_id INTEGER REFERENCES bi_dataset(id) ON DELETE CASCADE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS bi_chart (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    dataset_id INTEGER REFERENCES bi_dataset(id) ON DELETE CASCADE,
    chart_type VARCHAR(50) NOT NULL,
    config JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS bi_share (
    id SERIAL PRIMARY KEY,
    token VARCHAR(255) NOT NULL UNIQUE,
    chart_id INTEGER REFERENCES bi_chart(id) ON DELETE CASCADE,
    password VARCHAR(255),
    expires_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Legacy scattered ALTERs (model.go / database.go) folded into the baseline.
ALTER TABLE bi_dataset ADD COLUMN IF NOT EXISTS shard_enabled BOOLEAN DEFAULT FALSE;
ALTER TABLE bi_dataset ADD COLUMN IF NOT EXISTS shard_keys JSONB DEFAULT '[]';
ALTER TABLE bi_datasource ADD COLUMN IF NOT EXISTS type VARCHAR(50) NOT NULL DEFAULT 'postgresql';

CREATE INDEX IF NOT EXISTS idx_dataset_datasource_id ON bi_dataset(datasource_id);
CREATE INDEX IF NOT EXISTS idx_chart_dataset_id ON bi_chart(dataset_id);
CREATE INDEX IF NOT EXISTS idx_share_token ON bi_share(token);

-- +goose Down
DROP INDEX IF EXISTS idx_share_token;
DROP INDEX IF EXISTS idx_chart_dataset_id;
DROP INDEX IF EXISTS idx_dataset_datasource_id;
DROP TABLE IF EXISTS bi_share;
DROP TABLE IF EXISTS bi_chart;
DROP TABLE IF EXISTS bi_dataset_lineage;
DROP TABLE IF EXISTS bi_dataset;
DROP TABLE IF EXISTS bi_datasource;
