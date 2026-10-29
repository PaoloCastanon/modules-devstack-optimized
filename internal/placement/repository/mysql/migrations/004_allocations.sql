CREATE TABLE consumers (
 uuid VARCHAR(36) CHARACTER SET ascii COLLATE ascii_general_ci NOT NULL PRIMARY KEY,
 project_id VARCHAR(255) NOT NULL,
 user_id VARCHAR(255) NOT NULL,
 generation BIGINT NOT NULL DEFAULT 0,
 CONSTRAINT consumer_generation_valid CHECK (generation >= 0)
) ENGINE=InnoDB;

CREATE TABLE allocations (
 consumer_uuid VARCHAR(36) CHARACTER SET ascii COLLATE ascii_general_ci NOT NULL,
 provider_uuid CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 resource_class VARCHAR(255) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 used BIGINT NOT NULL,
 PRIMARY KEY(consumer_uuid,provider_uuid,resource_class),
 KEY allocations_usage (provider_uuid,resource_class),
 CONSTRAINT allocation_consumer_fk FOREIGN KEY(consumer_uuid) REFERENCES consumers(uuid) ON DELETE CASCADE,
 CONSTRAINT allocation_inventory_fk FOREIGN KEY(provider_uuid,resource_class) REFERENCES inventories(provider_uuid,resource_class) ON UPDATE CASCADE,
 CONSTRAINT allocation_used_valid CHECK (used > 0)
) ENGINE=InnoDB;
