-- Create sequence for auto-incrementing collection IDs.
-- The ID is reserved via NEXTVAL before writing vinfo rows, so the collection
-- record can reference the same ID when it is written at the end of the run.
CREATE SEQUENCE IF NOT EXISTS collections_id_seq START 1;

-- Create collections table: source of truth for lifecycle and publication state
CREATE TABLE IF NOT EXISTS collections (
    id BIGINT PRIMARY KEY DEFAULT NEXTVAL('collections_id_seq'),
    vcenter_id VARCHAR NOT NULL,
    vcenter VARCHAR,
    state VARCHAR NOT NULL CHECK (state IN ('running', 'done', 'failed')),
    active BOOLEAN NOT NULL DEFAULT false,
    vm_count_migratable INTEGER NOT NULL DEFAULT 0,
    vm_count_non_migratable INTEGER NOT NULL DEFAULT 0,
    vm_count_total INTEGER NOT NULL DEFAULT 0,
    cluster_count_total INTEGER NOT NULL DEFAULT 0,
    vm_count_new_since_previous INTEGER NOT NULL DEFAULT 0,
    vm_count_missing_since_previous INTEGER NOT NULL DEFAULT 0,
    vm_count_delta_since_previous INTEGER NOT NULL DEFAULT 0,
    vm_count_migratable_delta_since_previous INTEGER NOT NULL DEFAULT 0,
    started_at TIMESTAMP,
    finished_at TIMESTAMP,
    error VARCHAR,
    created_at TIMESTAMP NOT NULL DEFAULT now(),
    updated_at TIMESTAMP NOT NULL DEFAULT now()
);
