-- extensions store: track install source, checksum, ref, and trust tier.
-- Requires the `extensions` table (created by GORM AutoMigrate and/or 0001 init).
ALTER TABLE `extensions` ADD COLUMN `source_uri` text;
ALTER TABLE `extensions` ADD COLUMN `checksum` varchar(64);
ALTER TABLE `extensions` ADD COLUMN `installed_ref` varchar(128);
ALTER TABLE `extensions` ADD COLUMN `trust_level` varchar(16) NOT NULL DEFAULT 'local';
CREATE INDEX `idx_extensions_source_uri` ON `extensions`(`source_uri`);