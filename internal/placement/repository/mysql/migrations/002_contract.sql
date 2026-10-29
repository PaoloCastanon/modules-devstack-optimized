ALTER TABLE resource_providers MODIFY name VARCHAR(200) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL;
ALTER TABLE inventories MODIFY allocation_ratio FLOAT NOT NULL;
