CREATE TABLE IF NOT EXISTS upload_asset (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    owner_id BIGINT NOT NULL,
    url VARCHAR(512) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    kind VARCHAR(16) NOT NULL,
    size BIGINT NOT NULL,
    created_at DATETIME(6) NOT NULL,
    UNIQUE KEY uk_upload_asset_url (url),
    KEY idx_upload_asset_owner (owner_id, created_at)
) ENGINE=InnoDB;
