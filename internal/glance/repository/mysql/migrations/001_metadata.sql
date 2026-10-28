CREATE TABLE images (
    id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
    owner VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
    name VARCHAR(255) NULL,
    status VARCHAR(30) NOT NULL,
    visibility VARCHAR(20) NOT NULL,
    disk_format VARCHAR(20) NULL,
    container_format VARCHAR(20) NULL,
    min_disk INT NOT NULL DEFAULT 0,
    min_ram INT NOT NULL DEFAULT 0,
    protected BOOLEAN NOT NULL DEFAULT FALSE,
    os_hidden BOOLEAN NOT NULL DEFAULT FALSE,
    size BIGINT NULL,
    virtual_size BIGINT NULL,
    checksum VARCHAR(32) NULL,
    os_hash_algo VARCHAR(64) NULL,
    os_hash_value VARCHAR(128) NULL,
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    deleted BOOLEAN NOT NULL DEFAULT FALSE,
    deleted_at DATETIME(6) NULL,
    CONSTRAINT valid_visibility CHECK (visibility IN ('private','public','shared','community')),
    CONSTRAINT valid_minimums CHECK (min_disk >= 0 AND min_ram >= 0),
    CONSTRAINT valid_sizes CHECK ((size IS NULL OR size >= 0) AND (virtual_size IS NULL OR virtual_size >= 0)),
    CONSTRAINT valid_deleted CHECK ((deleted = FALSE AND deleted_at IS NULL) OR (deleted = TRUE AND deleted_at IS NOT NULL)),
    INDEX images_owner_page (owner, deleted, created_at, id),
    INDEX images_visibility_page (visibility, deleted, created_at, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE image_properties (
    image_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    name VARCHAR(255) NOT NULL,
    value TEXT NOT NULL,
    PRIMARY KEY (image_id, name),
    CONSTRAINT properties_image_fk FOREIGN KEY (image_id) REFERENCES images(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE image_tags (
    image_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    value VARCHAR(255) NOT NULL,
    PRIMARY KEY (image_id, value),
    CONSTRAINT tags_image_fk FOREIGN KEY (image_id) REFERENCES images(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;
