CREATE TABLE IF NOT EXISTS friend_requests (
    user_id VARCHAR(36) NOT NULL,
    req_user_id VARCHAR(36) NOT NULL,
    req_status INT NOT NULL,
    create_at BIGINT NOT NULL,
    PRIMARY KEY (user_id, req_user_id),
    INDEX idx_create_at(create_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;