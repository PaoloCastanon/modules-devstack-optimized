CREATE TABLE resource_providers (
    uuid CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
    name VARCHAR(200) NOT NULL UNIQUE,
    generation BIGINT NOT NULL DEFAULT 0,
    CONSTRAINT valid_generation CHECK (generation >= 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE resource_classes (
    name VARCHAR(255) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY
) ENGINE=InnoDB;

INSERT INTO resource_classes (name) VALUES ('VCPU'), ('MEMORY_MB'), ('DISK_GB');

CREATE TABLE inventories (
    provider_uuid CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    resource_class VARCHAR(255) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    total INT NOT NULL,
    reserved INT NOT NULL,
    min_unit INT NOT NULL,
    max_unit INT NOT NULL,
    step_size INT NOT NULL,
    allocation_ratio DOUBLE NOT NULL,
    PRIMARY KEY (provider_uuid, resource_class),
    CONSTRAINT inventories_provider_fk FOREIGN KEY (provider_uuid) REFERENCES resource_providers(uuid) ON DELETE CASCADE,
    CONSTRAINT inventories_class_fk FOREIGN KEY (resource_class) REFERENCES resource_classes(name),
    CONSTRAINT valid_inventory CHECK (total >= 1 AND reserved >= 0 AND min_unit >= 1 AND max_unit >= 1 AND step_size >= 1 AND allocation_ratio >= 0)
) ENGINE=InnoDB;
