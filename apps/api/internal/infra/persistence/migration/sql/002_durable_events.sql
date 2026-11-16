CREATE TABLE IF NOT EXISTS event_outbox (
 id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
 event_id VARCHAR(128) NOT NULL,
 topic VARCHAR(64) NOT NULL,
 payload LONGTEXT NOT NULL,
 attempts BIGINT NOT NULL DEFAULT 0,
 available_at DATETIME(6) NOT NULL,
 lease_until DATETIME(6) NOT NULL,
 lease_token VARCHAR(64) NOT NULL DEFAULT '',
 processed_at DATETIME(6) NULL,
 failed_at DATETIME(6) NULL,
 last_error VARCHAR(1000) NOT NULL DEFAULT '',
 created_at DATETIME(6) NULL,
 UNIQUE KEY uk_outbox_event (event_id),
 KEY idx_outbox_pending (processed_at, failed_at, available_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS interaction_action_request (
 id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
 user_id BIGINT NOT NULL,
 video_id BIGINT NOT NULL,
 action_type VARCHAR(16) NOT NULL,
 request_key VARCHAR(128) NOT NULL,
 status BIGINT NOT NULL,
 version BIGINT UNSIGNED NOT NULL,
 `count` BIGINT NOT NULL,
 like_count BIGINT NULL,
 favorite_count BIGINT NULL,
 created_at DATETIME(3) NULL,
 UNIQUE KEY uk_action_request (user_id, video_id, action_type, request_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS interaction_action_receipt (
 event_id VARCHAR(128) NOT NULL PRIMARY KEY,
 version BIGINT UNSIGNED NULL,
 created_at DATETIME(3) NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
