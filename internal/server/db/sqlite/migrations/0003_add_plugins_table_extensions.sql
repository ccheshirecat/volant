PRAGMA foreign_keys = OFF;

CREATE TABLE plugins_tmp (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    version TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    metadata TEXT,
    installed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO plugins_tmp (id, name, version, enabled, metadata, installed_at, updated_at)
SELECT id, name, version, 1, metadata, installed_at, CURRENT_TIMESTAMP
FROM plugins;

DROP TABLE plugins;
ALTER TABLE plugins_tmp RENAME TO plugins;

PRAGMA foreign_keys = ON;
