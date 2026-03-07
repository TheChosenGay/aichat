CREATE TABLE IF NOT EXISTS messages (
    msg_id       VARCHAR(36)  NOT NULL PRIMARY KEY,
    from_id      VARCHAR(36)  NOT NULL,
    to_id        VARCHAR(36)  NOT NULL,
    type         TINYINT      NOT NULL DEFAULT 0,
    content      VARCHAR(1000) NOT NULL,
    send_at      BIGINT       NOT NULL,
    is_delivered BOOLEAN      NOT NULL DEFAULT FALSE,
    INDEX idx_to_id_send_at (to_id, send_at),
    INDEX idx_from_id_send_at (from_id, send_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
