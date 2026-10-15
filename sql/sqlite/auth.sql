-- schema-only template
CREATE TABLE IF NOT EXISTS `account` (
`id` INTEGER  NOT NULL,
`username` TEXT NOT NULL DEFAULT '',
`salt` BLOB NOT NULL,
`verifier` BLOB NOT NULL,
`session_key_auth` BLOB DEFAULT NULL,
`session_key_bnet` BLOB DEFAULT NULL,
`totp_secret` BLOB DEFAULT NULL,
`email` TEXT NOT NULL DEFAULT '',
`reg_mail` TEXT NOT NULL DEFAULT '',
`joindate` TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
`last_ip` TEXT NOT NULL DEFAULT '127.0.0.1',
`last_attempt_ip` TEXT NOT NULL DEFAULT '127.0.0.1',
`failed_logins` INTEGER  NOT NULL DEFAULT '0',
`locked` INTEGER  NOT NULL DEFAULT '0',
`lock_country` TEXT NOT NULL DEFAULT '00',
`last_login` TEXT NULL DEFAULT NULL,
`online` INTEGER  NOT NULL DEFAULT '0',
`expansion` INTEGER  NOT NULL DEFAULT '2',
`mutetime` INTEGER NOT NULL DEFAULT '0',
`mutereason` TEXT NOT NULL DEFAULT '',
`muteby` TEXT NOT NULL DEFAULT '',
`locale` INTEGER  NOT NULL DEFAULT '0',
`os` TEXT NOT NULL DEFAULT '',
`recruiter` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`id`)
);
CREATE UNIQUE INDEX IF NOT EXISTS `account__idx_username` ON `account` (`username`);
CREATE TABLE IF NOT EXISTS `account_access` (
`AccountID` INTEGER  NOT NULL,
`SecurityLevel` INTEGER  NOT NULL,
`RealmID` INTEGER NOT NULL DEFAULT '-1',
`Comment` TEXT DEFAULT NULL,
PRIMARY KEY (`AccountID`,`RealmID`)
);
CREATE TABLE IF NOT EXISTS `account_banned` (
`id` INTEGER  NOT NULL DEFAULT '0',
`bandate` INTEGER  NOT NULL DEFAULT '0',
`unbandate` INTEGER  NOT NULL DEFAULT '0',
`bannedby` TEXT NOT NULL,
`banreason` TEXT NOT NULL,
`active` INTEGER  NOT NULL DEFAULT '1',
PRIMARY KEY (`id`,`bandate`)
);
CREATE TABLE IF NOT EXISTS `account_muted` (
`guid` INTEGER  NOT NULL DEFAULT '0',
`mutedate` INTEGER  NOT NULL DEFAULT '0',
`mutetime` INTEGER  NOT NULL DEFAULT '0',
`mutedby` TEXT NOT NULL,
`mutereason` TEXT NOT NULL,
PRIMARY KEY (`guid`,`mutedate`)
);
CREATE TABLE IF NOT EXISTS `autobroadcast` (
`realmid` INTEGER NOT NULL DEFAULT '-1',
`id` INTEGER  NOT NULL,
`weight` INTEGER  DEFAULT '1',
`text` TEXT NOT NULL,
PRIMARY KEY (`id`,`realmid`)
);
CREATE TABLE IF NOT EXISTS `build_info` (
`build` INTEGER NOT NULL,
`majorVersion` INTEGER DEFAULT NULL,
`minorVersion` INTEGER DEFAULT NULL,
`bugfixVersion` INTEGER DEFAULT NULL,
`hotfixVersion` TEXT DEFAULT NULL,
`winAuthSeed` TEXT DEFAULT NULL,
`win64AuthSeed` TEXT DEFAULT NULL,
`mac64AuthSeed` TEXT DEFAULT NULL,
`winChecksumSeed` TEXT DEFAULT NULL,
`macChecksumSeed` TEXT DEFAULT NULL,
PRIMARY KEY (`build`)
);
CREATE TABLE IF NOT EXISTS `ip_banned` (
`ip` TEXT NOT NULL DEFAULT '127.0.0.1',
`bandate` INTEGER  NOT NULL,
`unbandate` INTEGER  NOT NULL,
`bannedby` TEXT NOT NULL DEFAULT '[Console]',
`banreason` TEXT NOT NULL DEFAULT 'no reason',
PRIMARY KEY (`ip`,`bandate`)
);
CREATE TABLE IF NOT EXISTS `logs` (
`time` INTEGER  NOT NULL,
`realm` INTEGER  NOT NULL,
`type` TEXT NOT NULL,
`level` INTEGER  NOT NULL DEFAULT '0',
`string` TEXT
);
CREATE TABLE IF NOT EXISTS `logs_ip_actions` (
`id` INTEGER  NOT NULL,
`account_id` INTEGER  NOT NULL,
`character_guid` INTEGER  NOT NULL,
`realm_id` INTEGER  NOT NULL DEFAULT '0',
`type` INTEGER  NOT NULL,
`ip` TEXT NOT NULL DEFAULT '127.0.0.1',
`systemnote` TEXT,
`unixtime` INTEGER  NOT NULL,
`time` TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
`comment` TEXT,
PRIMARY KEY (`id`)
);
CREATE TABLE IF NOT EXISTS `rbac_permissions` (
`id` INTEGER  NOT NULL DEFAULT '0',
`name` TEXT NOT NULL,
PRIMARY KEY (`id`)
);
CREATE TABLE IF NOT EXISTS `rbac_account_permissions` (
`accountId` INTEGER  NOT NULL,
`permissionId` INTEGER  NOT NULL,
`granted` INTEGER NOT NULL DEFAULT '1',
`realmId` INTEGER NOT NULL DEFAULT '-1',
PRIMARY KEY (`accountId`,`permissionId`,`realmId`),
CONSTRAINT `fk__rbac_account_permissions__account` FOREIGN KEY (`accountId`) REFERENCES `account` (`id`) ON DELETE CASCADE ON UPDATE RESTRICT,
CONSTRAINT `fk__rbac_account_roles__rbac_permissions` FOREIGN KEY (`permissionId`) REFERENCES `rbac_permissions` (`id`) ON DELETE CASCADE ON UPDATE RESTRICT
);
CREATE INDEX IF NOT EXISTS `rbac_account_permissions__fk__rbac_account_roles__rbac_permissions` ON `rbac_account_permissions` (`permissionId`);
CREATE TABLE IF NOT EXISTS `rbac_default_permissions` (
`secId` INTEGER  NOT NULL,
`permissionId` INTEGER  NOT NULL,
`realmId` INTEGER NOT NULL DEFAULT '-1',
PRIMARY KEY (`secId`,`permissionId`,`realmId`),
CONSTRAINT `fk__rbac_default_permissions__rbac_permissions` FOREIGN KEY (`permissionId`) REFERENCES `rbac_permissions` (`id`) ON DELETE RESTRICT ON UPDATE RESTRICT
);
CREATE INDEX IF NOT EXISTS `rbac_default_permissions__fk__rbac_default_permissions__rbac_permissions` ON `rbac_default_permissions` (`permissionId`);
CREATE TABLE IF NOT EXISTS `rbac_linked_permissions` (
`id` INTEGER  NOT NULL,
`linkedId` INTEGER  NOT NULL,
PRIMARY KEY (`id`,`linkedId`),
CONSTRAINT `fk__rbac_linked_permissions__rbac_permissions1` FOREIGN KEY (`id`) REFERENCES `rbac_permissions` (`id`) ON DELETE CASCADE ON UPDATE RESTRICT,
CONSTRAINT `fk__rbac_linked_permissions__rbac_permissions2` FOREIGN KEY (`linkedId`) REFERENCES `rbac_permissions` (`id`) ON DELETE CASCADE ON UPDATE RESTRICT
);
CREATE INDEX IF NOT EXISTS `rbac_linked_permissions__fk__rbac_linked_permissions__rbac_permissions1` ON `rbac_linked_permissions` (`id`);
CREATE INDEX IF NOT EXISTS `rbac_linked_permissions__fk__rbac_linked_permissions__rbac_permissions2` ON `rbac_linked_permissions` (`linkedId`);

CREATE TABLE IF NOT EXISTS `realmcharacters` (
`realmid` INTEGER  NOT NULL DEFAULT '0',
`acctid` INTEGER  NOT NULL,
`numchars` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`realmid`,`acctid`)
);
CREATE INDEX IF NOT EXISTS `realmcharacters__acctid` ON `realmcharacters` (`acctid`);
CREATE TABLE IF NOT EXISTS `realmlist` (
`id` INTEGER  NOT NULL,
`name` TEXT NOT NULL DEFAULT '',
`address` TEXT NOT NULL DEFAULT '127.0.0.1',
`localAddress` TEXT NOT NULL DEFAULT '127.0.0.1',
`localSubnetMask` TEXT NOT NULL DEFAULT '255.255.255.0',
`port` INTEGER  NOT NULL DEFAULT '8085',
`icon` INTEGER  NOT NULL DEFAULT '0',
`flag` INTEGER  NOT NULL DEFAULT '2',
`timezone` INTEGER  NOT NULL DEFAULT '0',
`allowedSecurityLevel` INTEGER  NOT NULL DEFAULT '0',
`population` REAL  NOT NULL DEFAULT '0',
`gamebuild` INTEGER  NOT NULL DEFAULT '12340',
PRIMARY KEY (`id`)
);
CREATE UNIQUE INDEX IF NOT EXISTS `realmlist__idx_name` ON `realmlist` (`name`);
CREATE TABLE IF NOT EXISTS `secret_digest` (
`id` INTEGER  NOT NULL,
`digest` TEXT NOT NULL,
PRIMARY KEY (`id`)
);
CREATE TABLE IF NOT EXISTS `updates` (
`name` TEXT NOT NULL,
`hash` TEXT DEFAULT '',
`state` TEXT NOT NULL DEFAULT 'RELEASED',
`timestamp` TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
`speed` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`name`)
);
CREATE TABLE IF NOT EXISTS `updates_include` (
`path` TEXT NOT NULL,
`state` TEXT NOT NULL DEFAULT 'RELEASED',
PRIMARY KEY (`path`)
);
CREATE TABLE IF NOT EXISTS `uptime` (
`realmid` INTEGER  NOT NULL,
`starttime` INTEGER  NOT NULL DEFAULT '0',
`uptime` INTEGER  NOT NULL DEFAULT '0',
`maxplayers` INTEGER  NOT NULL DEFAULT '0',
`revision` TEXT NOT NULL DEFAULT 'Trinitycore',
PRIMARY KEY (`realmid`,`starttime`)
);
