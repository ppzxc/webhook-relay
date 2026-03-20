CREATE TABLE IF NOT EXISTS messages (
    id              TEXT PRIMARY KEY,
    version         INTEGER NOT NULL DEFAULT 1,
    input           TEXT NOT NULL,
    payload         BLOB NOT NULL,
    created_at      DATETIME NOT NULL,
    status          TEXT NOT NULL DEFAULT 'PENDING',
    retry_count     INTEGER NOT NULL DEFAULT 0,
    last_attempt_at DATETIME
);
