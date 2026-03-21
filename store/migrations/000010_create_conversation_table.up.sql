CREATE TABLE conversations (
    cid VARCHAR(36) PRIMARY KEY,
    user_id VARCHAR(36) NOT NULL,
    peer_id VARCHAR(36),
    room_id VARCHAR(36),
    last_msg_id VARCHAR(36),
    last_sender_name VARCHAR(255),
    last_msg_time BIGINT NOT NULL,
    last_msg_content VARCHAR(500),
    unread_count INT NOT NULL DEFAULT 0,
    CONSTRAINT chk_conv_type CHECK (
        (peer_id IS NOT NULL AND room_id IS NULL) OR
        (peer_id IS NULL AND room_id IS NOT NULL)
    ),
    UNIQUE(last_msg_id),
    UNIQUE(user_id, peer_id),
    UNIQUE(user_id, room_id),
    INDEX(user_id, last_msg_time DESC)
)
