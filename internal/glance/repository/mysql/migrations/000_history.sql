CREATE TABLE IF NOT EXISTS schema_migrations (
    version BIGINT NOT NULL PRIMARY KEY,
    checksum CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    dirty BOOLEAN NOT NULL,
    applied_at DATETIME(6) NULL
) ENGINE=InnoDB;
