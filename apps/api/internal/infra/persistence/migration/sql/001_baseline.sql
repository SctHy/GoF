-- Frozen baseline: schema only; CREATE IF NOT EXISTS preserves existing data.
CREATE TABLE IF NOT EXISTS `account` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `account` varchar(64) NOT NULL,
  `password` varchar(255) NOT NULL,
  `nickname` varchar(128) NOT NULL,
  `avatar_url` varchar(512) DEFAULT NULL,
  `bio` varchar(255) DEFAULT NULL,
  `status` bigint NOT NULL DEFAULT '1',
  `role` varchar(32) NOT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_account_account` (`account`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `exposures` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `user_id` bigint NOT NULL,
  `video_id` bigint NOT NULL,
  `first_exposed_at` datetime(3) NOT NULL,
  `last_exposed_at` datetime(3) NOT NULL,
  `exposure_count` bigint NOT NULL DEFAULT '1',
  `last_scene` varchar(32) NOT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_video` (`user_id`,`video_id`),
  KEY `idx_user_last_exposed` (`user_id`,`last_exposed_at`),
  KEY `idx_video_last_exposed` (`video_id`,`last_exposed_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `feed_inbox` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `user_id` bigint NOT NULL,
  `video_id` bigint NOT NULL,
  `author_id` bigint NOT NULL,
  `published_at` datetime(3) NOT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_video` (`user_id`,`video_id`),
  KEY `idx_user_published_video` (`user_id`,`published_at`,`video_id`),
  KEY `idx_video` (`video_id`),
  KEY `idx_author` (`author_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `interaction_action` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `user_id` bigint NOT NULL,
  `video_id` bigint NOT NULL,
  `action_type` varchar(16) NOT NULL,
  `status` tinyint NOT NULL DEFAULT '1',
  `idempotency_key` varchar(128) DEFAULT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_video_type` (`user_id`,`video_id`,`action_type`),
  KEY `idx_user_type_status` (`user_id`,`action_type`,`status`),
  KEY `idx_video_type_status` (`video_id`,`action_type`,`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `interaction_comment` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `video_id` bigint NOT NULL,
  `user_id` bigint NOT NULL,
  `content` varchar(1000) NOT NULL,
  `status` tinyint NOT NULL DEFAULT '1',
  `idempotency_key` varchar(128) DEFAULT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_idempotency` (`user_id`,`idempotency_key`),
  KEY `idx_video_status_created` (`video_id`,`status`,`created_at`,`updated_at`),
  KEY `idx_user_created` (`user_id`,`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `playback_config` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `platform` varchar(16) NOT NULL,
  `network_type` varchar(16) NOT NULL,
  `preload_count` bigint NOT NULL,
  `buffer_ms` bigint NOT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_platform_network` (`platform`,`network_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `playback_qos_log` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `user_id` bigint NOT NULL,
  `video_id` bigint NOT NULL,
  `first_frame_ms` bigint DEFAULT NULL,
  `stutter_count` bigint NOT NULL DEFAULT '0',
  `watch_ms` bigint NOT NULL DEFAULT '0',
  `idempotency_key` varchar(128) DEFAULT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_idempotency` (`user_id`,`idempotency_key`),
  KEY `idx_user_video_time` (`user_id`,`video_id`,`created_at`),
  KEY `idx_video_time` (`video_id`,`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `user_follow` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `user_id` bigint NOT NULL,
  `target_user_id` bigint NOT NULL,
  `status` tinyint NOT NULL DEFAULT '1',
  `idempotency_key` varchar(128) DEFAULT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_target` (`user_id`,`target_user_id`),
  KEY `idx_user_status_updated` (`user_id`,`status`,`updated_at`,`target_user_id`),
  KEY `idx_target_status_updated` (`target_user_id`,`status`,`updated_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `user_message` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `user_id` bigint NOT NULL,
  `type` varchar(16) NOT NULL,
  `title` varchar(128) NOT NULL,
  `content` varchar(1024) NOT NULL,
  `actor_id` bigint NOT NULL DEFAULT '0',
  `actor_nickname` varchar(128) DEFAULT NULL,
  `actor_avatar_url` varchar(512) DEFAULT NULL,
  `event_id` varchar(64) DEFAULT NULL,
  `idempotency_key` varchar(128) DEFAULT NULL,
  `is_read` tinyint(1) NOT NULL DEFAULT '0',
  `created_at` datetime(3) DEFAULT NULL,
  `read_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_event` (`user_id`,`event_id`),
  UNIQUE KEY `uk_user_idempotency` (`user_id`,`idempotency_key`),
  KEY `idx_user_read_created` (`user_id`,`is_read`,`created_at`),
  KEY `idx_user_created` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `user_relation_stat` (
  `user_id` bigint NOT NULL AUTO_INCREMENT,
  `following_count` bigint NOT NULL DEFAULT '0',
  `follower_count` bigint NOT NULL DEFAULT '0',
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `video` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `author_id` bigint NOT NULL,
  `title` varchar(128) NOT NULL,
  `description` varchar(512) DEFAULT NULL,
  `media_url` varchar(512) NOT NULL,
  `cover_url` varchar(512) NOT NULL,
  `status` tinyint NOT NULL DEFAULT '2',
  `published_at` datetime(3) DEFAULT NULL,
  `idempotency_key` varchar(128) DEFAULT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_author_idempotency` (`author_id`,`idempotency_key`),
  KEY `idx_author_status` (`author_id`,`status`,`created_at`),
  KEY `idx_status_published` (`status`,`published_at`),
  KEY `idx_video_timeline` (`status`,`published_at` DESC,`id` DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `video_embedding` (
  `video_id` bigint NOT NULL,
  `model` varchar(64) NOT NULL,
  `dimension` bigint NOT NULL,
  `embedding_json` json NOT NULL,
  `text_hash` varchar(64) NOT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`video_id`,`model`),
  UNIQUE KEY `uk_video_model` (`video_id`,`model`),
  KEY `idx_model_updated` (`model`,`updated_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `video_stat` (
  `video_id` bigint NOT NULL AUTO_INCREMENT,
  `like_count` bigint NOT NULL DEFAULT '0',
  `comment_count` bigint NOT NULL DEFAULT '0',
  `favorite_count` bigint NOT NULL DEFAULT '0',
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`video_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `video_view_events` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `user_id` bigint NOT NULL,
  `video_id` bigint NOT NULL,
  `scene` varchar(32) NOT NULL,
  `request_id` varchar(64) DEFAULT NULL,
  `event_type` varchar(32) NOT NULL,
  `watch_ms` bigint NOT NULL DEFAULT '0',
  `completed` tinyint(1) NOT NULL DEFAULT '0',
  `created_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_user_created` (`user_id`,`created_at`),
  KEY `idx_video_created` (`video_id`,`created_at`),
  KEY `idx_user_scene_created` (`scene`,`created_at`),
  KEY `idx_request_event` (`request_id`,`event_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
