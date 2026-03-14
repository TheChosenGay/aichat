CREATE TABLE IF NOT EXISTS friend_relationships (
    user_id VARCHAR(36) NOT NULL,
    friend_id VARCHAR(36) NOT NULL,
    nick_name VARCHAR(255) NOT NULL,
    create_at BIGINT NOT NULL,
    PRIMARY KEY (user_id, friend_id),
    INDEX idx_create_at(create_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;