CREATE TABLE IF NOT EXISTS messages (
    id              VARCHAR(255) PRIMARY KEY,
    version         INT NOT NULL DEFAULT 1,
    input           VARCHAR(255) NOT NULL,
    payload         MEDIUMBLOB NOT NULL,
    created_at      DATETIME(6) NOT NULL,
    status          VARCHAR(32) NOT NULL DEFAULT 'PENDING',
    retry_count     INT NOT NULL DEFAULT 0,
    last_attempt_at DATETIME(6) NULL,
    INDEX idx_messages_input (input),
    INDEX idx_messages_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
