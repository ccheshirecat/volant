CREATE TABLE IF NOT EXISTS plugin_artifacts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    plugin_name TEXT NOT NULL,
    version TEXT NOT NULL,
    artifact_name TEXT NOT NULL,
    kind TEXT NOT NULL,
    source_url TEXT,
    checksum TEXT,
    format TEXT NOT NULL,
    local_path TEXT NOT NULL,
    size_bytes INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(plugin_name, version, artifact_name)
);

CREATE INDEX IF NOT EXISTS idx_plugin_artifacts_plugin_kind ON plugin_artifacts(plugin_name, version, kind);

CREATE TABLE IF NOT EXISTS vm_cloudinit (
    vm_id INTEGER PRIMARY KEY REFERENCES vms(id) ON DELETE CASCADE,
    user_data TEXT,
    meta_data TEXT,
    network_config TEXT,
    seed_path TEXT,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
