-- schema-only template
CREATE TABLE IF NOT EXISTS `account_data` (
`accountId` INTEGER  NOT NULL DEFAULT '0',
`type` INTEGER  NOT NULL DEFAULT '0',
`time` INTEGER  NOT NULL DEFAULT '0',
`data` BLOB NOT NULL,
PRIMARY KEY (`accountId`,`type`)
);
CREATE TABLE IF NOT EXISTS `account_instance_times` (
`accountId` INTEGER  NOT NULL,
`instanceId` INTEGER  NOT NULL DEFAULT '0',
`releaseTime` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`accountId`,`instanceId`)
);
CREATE TABLE IF NOT EXISTS `account_tutorial` (
`accountId` INTEGER  NOT NULL DEFAULT '0',
`tut0` INTEGER  NOT NULL DEFAULT '0',
`tut1` INTEGER  NOT NULL DEFAULT '0',
`tut2` INTEGER  NOT NULL DEFAULT '0',
`tut3` INTEGER  NOT NULL DEFAULT '0',
`tut4` INTEGER  NOT NULL DEFAULT '0',
`tut5` INTEGER  NOT NULL DEFAULT '0',
`tut6` INTEGER  NOT NULL DEFAULT '0',
`tut7` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`accountId`)
);
CREATE TABLE IF NOT EXISTS `addons` (
`name` TEXT NOT NULL DEFAULT '',
`crc` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`name`)
);
CREATE TABLE IF NOT EXISTS `arena_team` (
`arenaTeamId` INTEGER  NOT NULL DEFAULT '0',
`name` TEXT NOT NULL,
`captainGuid` INTEGER  NOT NULL DEFAULT '0',
`type` INTEGER  NOT NULL DEFAULT '0',
`rating` INTEGER  NOT NULL DEFAULT '0',
`seasonGames` INTEGER  NOT NULL DEFAULT '0',
`seasonWins` INTEGER  NOT NULL DEFAULT '0',
`weekGames` INTEGER  NOT NULL DEFAULT '0',
`weekWins` INTEGER  NOT NULL DEFAULT '0',
`rank` INTEGER  NOT NULL DEFAULT '0',
`backgroundColor` INTEGER  NOT NULL DEFAULT '0',
`emblemStyle` INTEGER  NOT NULL DEFAULT '0',
`emblemColor` INTEGER  NOT NULL DEFAULT '0',
`borderStyle` INTEGER  NOT NULL DEFAULT '0',
`borderColor` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`arenaTeamId`)
);
CREATE TABLE IF NOT EXISTS `arena_team_member` (
`arenaTeamId` INTEGER  NOT NULL DEFAULT '0',
`guid` INTEGER  NOT NULL DEFAULT '0',
`weekGames` INTEGER  NOT NULL DEFAULT '0',
`weekWins` INTEGER  NOT NULL DEFAULT '0',
`seasonGames` INTEGER  NOT NULL DEFAULT '0',
`seasonWins` INTEGER  NOT NULL DEFAULT '0',
`personalRating` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`arenaTeamId`,`guid`)
);
CREATE TABLE IF NOT EXISTS `auctionbidders` (
`id` INTEGER  NOT NULL DEFAULT '0',
`bidderguid` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`id`,`bidderguid`)
);
CREATE TABLE IF NOT EXISTS `auctionhouse` (
`id` INTEGER  NOT NULL DEFAULT '0',
`houseid` INTEGER  NOT NULL DEFAULT '7',
`itemguid` INTEGER  NOT NULL DEFAULT '0',
`itemowner` INTEGER  NOT NULL DEFAULT '0',
`buyoutprice` INTEGER  NOT NULL DEFAULT '0',
`time` INTEGER  NOT NULL DEFAULT '0',
`buyguid` INTEGER  NOT NULL DEFAULT '0',
`lastbid` INTEGER  NOT NULL DEFAULT '0',
`startbid` INTEGER  NOT NULL DEFAULT '0',
`deposit` INTEGER  NOT NULL DEFAULT '0',
`Flags` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`id`)
);
CREATE UNIQUE INDEX IF NOT EXISTS `auctionhouse__item_guid` ON `auctionhouse` (`itemguid`);
CREATE TABLE IF NOT EXISTS `banned_addons` (
`Id` INTEGER  NOT NULL,
`Name` TEXT NOT NULL,
`Version` TEXT NOT NULL DEFAULT '',
`Timestamp` TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
PRIMARY KEY (`Id`)
);
CREATE UNIQUE INDEX IF NOT EXISTS `banned_addons__idx_name_ver` ON `banned_addons` (`Name`,`Version`);
CREATE TABLE IF NOT EXISTS `battleground_deserters` (
`guid` INTEGER  NOT NULL,
`type` INTEGER  NOT NULL,
`datetime` TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS `bugreport` (
`id` INTEGER  NOT NULL,
`type` TEXT NOT NULL,
`content` TEXT NOT NULL,
PRIMARY KEY (`id`)
);
CREATE TABLE IF NOT EXISTS `calendar_events` (
`id` INTEGER  NOT NULL DEFAULT '0',
`creator` INTEGER  NOT NULL DEFAULT '0',
`title` TEXT NOT NULL DEFAULT '',
`description` TEXT NOT NULL DEFAULT '',
`type` INTEGER  NOT NULL DEFAULT '4',
`dungeon` INTEGER NOT NULL DEFAULT '-1',
`eventtime` INTEGER  NOT NULL DEFAULT '0',
`flags` INTEGER  NOT NULL DEFAULT '0',
`time2` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`id`)
);
CREATE TABLE IF NOT EXISTS `calendar_invites` (
`id` INTEGER  NOT NULL DEFAULT '0',
`event` INTEGER  NOT NULL DEFAULT '0',
`invitee` INTEGER  NOT NULL DEFAULT '0',
`sender` INTEGER  NOT NULL DEFAULT '0',
`status` INTEGER  NOT NULL DEFAULT '0',
`statustime` INTEGER  NOT NULL DEFAULT '0',
`rank` INTEGER  NOT NULL DEFAULT '0',
`text` TEXT NOT NULL DEFAULT '',
PRIMARY KEY (`id`)
);
CREATE TABLE IF NOT EXISTS `channels` (
`name` TEXT NOT NULL,
`team` INTEGER  NOT NULL,
`announce` INTEGER  NOT NULL DEFAULT '1',
`ownership` INTEGER  NOT NULL DEFAULT '1',
`password` TEXT DEFAULT NULL,
`bannedList` TEXT,
`lastUsed` INTEGER  NOT NULL,
PRIMARY KEY (`name`,`team`)
);
CREATE TABLE IF NOT EXISTS `character_account_data` (
`guid` INTEGER  NOT NULL DEFAULT '0',
`type` INTEGER  NOT NULL DEFAULT '0',
`time` INTEGER  NOT NULL DEFAULT '0',
`data` BLOB NOT NULL,
PRIMARY KEY (`guid`,`type`)
);
CREATE TABLE IF NOT EXISTS `character_achievement` (
`guid` INTEGER  NOT NULL,
`achievement` INTEGER  NOT NULL,
`date` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`guid`,`achievement`)
);
CREATE TABLE IF NOT EXISTS `character_achievement_progress` (
`guid` INTEGER  NOT NULL,
`criteria` INTEGER  NOT NULL,
`counter` INTEGER  NOT NULL,
`date` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`guid`,`criteria`)
);
CREATE TABLE IF NOT EXISTS `character_action` (
`guid` INTEGER  NOT NULL DEFAULT '0',
`spec` INTEGER  NOT NULL DEFAULT '0',
`button` INTEGER  NOT NULL DEFAULT '0',
`action` INTEGER  NOT NULL DEFAULT '0',
`type` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`guid`,`spec`,`button`)
);
CREATE TABLE IF NOT EXISTS `character_arena_stats` (
`guid` INTEGER  NOT NULL DEFAULT '0',
`slot` INTEGER  NOT NULL DEFAULT '0',
`matchMakerRating` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`guid`,`slot`)
);
CREATE TABLE IF NOT EXISTS `character_aura` (
`guid` INTEGER  NOT NULL DEFAULT '0',
`casterGuid` TEXT NOT NULL DEFAULT '0',
`itemGuid` INTEGER  NOT NULL DEFAULT '0',
`spell` INTEGER  NOT NULL DEFAULT '0',
`effectMask` INTEGER  NOT NULL DEFAULT '0',
`recalculateMask` INTEGER  NOT NULL DEFAULT '0',
`stackCount` INTEGER  NOT NULL DEFAULT '1',
`amount0` INTEGER NOT NULL DEFAULT '0',
`amount1` INTEGER NOT NULL DEFAULT '0',
`amount2` INTEGER NOT NULL DEFAULT '0',
`base_amount0` INTEGER NOT NULL DEFAULT '0',
`base_amount1` INTEGER NOT NULL DEFAULT '0',
`base_amount2` INTEGER NOT NULL DEFAULT '0',
`maxDuration` INTEGER NOT NULL DEFAULT '0',
`remainTime` INTEGER NOT NULL DEFAULT '0',
`remainCharges` INTEGER  NOT NULL DEFAULT '0',
`critChance` REAL NOT NULL DEFAULT '0',
`applyResilience` INTEGER NOT NULL DEFAULT '0',
PRIMARY KEY (`guid`,`casterGuid`,`itemGuid`,`spell`,`effectMask`)
);
CREATE TABLE IF NOT EXISTS `character_banned` (
`guid` INTEGER  NOT NULL DEFAULT '0',
`bandate` INTEGER  NOT NULL DEFAULT '0',
`unbandate` INTEGER  NOT NULL DEFAULT '0',
`bannedby` TEXT NOT NULL,
`banreason` TEXT NOT NULL,
`active` INTEGER  NOT NULL DEFAULT '1',
PRIMARY KEY (`guid`,`bandate`)
);
CREATE TABLE IF NOT EXISTS `character_battleground_data` (
`guid` INTEGER  NOT NULL DEFAULT '0',
`instanceId` INTEGER  NOT NULL,
`team` INTEGER  NOT NULL,
`joinX` REAL NOT NULL DEFAULT '0',
`joinY` REAL NOT NULL DEFAULT '0',
`joinZ` REAL NOT NULL DEFAULT '0',
`joinO` REAL NOT NULL DEFAULT '0',
`joinMapId` INTEGER  NOT NULL DEFAULT '0',
`taxiStart` INTEGER  NOT NULL DEFAULT '0',
`taxiEnd` INTEGER  NOT NULL DEFAULT '0',
`mountSpell` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`guid`)
);
CREATE TABLE IF NOT EXISTS `character_battleground_random` (
`guid` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`guid`)
);
CREATE TABLE IF NOT EXISTS `character_declinedname` (
`guid` INTEGER  NOT NULL DEFAULT '0',
`genitive` TEXT NOT NULL DEFAULT '',
`dative` TEXT NOT NULL DEFAULT '',
`accusative` TEXT NOT NULL DEFAULT '',
`instrumental` TEXT NOT NULL DEFAULT '',
`prepositional` TEXT NOT NULL DEFAULT '',
PRIMARY KEY (`guid`)
);
CREATE TABLE IF NOT EXISTS `character_equipmentsets` (
`guid` INTEGER  NOT NULL DEFAULT '0',
`setguid` INTEGER  NOT NULL,
`setindex` INTEGER  NOT NULL DEFAULT '0',
`name` TEXT NOT NULL,
`iconname` TEXT NOT NULL,
`ignore_mask` INTEGER  NOT NULL DEFAULT '0',
`item0` INTEGER  NOT NULL DEFAULT '0',
`item1` INTEGER  NOT NULL DEFAULT '0',
`item2` INTEGER  NOT NULL DEFAULT '0',
`item3` INTEGER  NOT NULL DEFAULT '0',
`item4` INTEGER  NOT NULL DEFAULT '0',
`item5` INTEGER  NOT NULL DEFAULT '0',
`item6` INTEGER  NOT NULL DEFAULT '0',
`item7` INTEGER  NOT NULL DEFAULT '0',
`item8` INTEGER  NOT NULL DEFAULT '0',
`item9` INTEGER  NOT NULL DEFAULT '0',
`item10` INTEGER  NOT NULL DEFAULT '0',
`item11` INTEGER  NOT NULL DEFAULT '0',
`item12` INTEGER  NOT NULL DEFAULT '0',
`item13` INTEGER  NOT NULL DEFAULT '0',
`item14` INTEGER  NOT NULL DEFAULT '0',
`item15` INTEGER  NOT NULL DEFAULT '0',
`item16` INTEGER  NOT NULL DEFAULT '0',
`item17` INTEGER  NOT NULL DEFAULT '0',
`item18` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`setguid`)
);
CREATE UNIQUE INDEX IF NOT EXISTS `character_equipmentsets__idx_set` ON `character_equipmentsets` (`guid`,`setguid`,`setindex`);
CREATE INDEX IF NOT EXISTS `character_equipmentsets__Idx_setindex` ON `character_equipmentsets` (`setindex`);
CREATE TABLE IF NOT EXISTS `character_fishingsteps` (
`guid` INTEGER  NOT NULL DEFAULT '0',
`fishingSteps` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`guid`)
);
CREATE TABLE IF NOT EXISTS `character_gifts` (
`guid` INTEGER  NOT NULL DEFAULT '0',
`item_guid` INTEGER  NOT NULL DEFAULT '0',
`entry` INTEGER  NOT NULL DEFAULT '0',
`flags` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`item_guid`)
);
CREATE INDEX IF NOT EXISTS `character_gifts__idx_guid` ON `character_gifts` (`guid`);
CREATE TABLE IF NOT EXISTS `character_glyphs` (
`guid` INTEGER  NOT NULL,
`talentGroup` INTEGER  NOT NULL DEFAULT '0',
`glyph1` INTEGER  DEFAULT '0',
`glyph2` INTEGER  DEFAULT '0',
`glyph3` INTEGER  DEFAULT '0',
`glyph4` INTEGER  DEFAULT '0',
`glyph5` INTEGER  DEFAULT '0',
`glyph6` INTEGER  DEFAULT '0',
PRIMARY KEY (`guid`,`talentGroup`)
);
CREATE TABLE IF NOT EXISTS `character_homebind` (
`guid` INTEGER  NOT NULL DEFAULT '0',
`mapId` INTEGER  NOT NULL DEFAULT '0',
`zoneId` INTEGER  NOT NULL DEFAULT '0',
`posX` REAL NOT NULL DEFAULT '0',
`posY` REAL NOT NULL DEFAULT '0',
`posZ` REAL NOT NULL DEFAULT '0',
PRIMARY KEY (`guid`)
);
CREATE TABLE IF NOT EXISTS `character_instance` (
`guid` INTEGER  NOT NULL DEFAULT '0',
`instance` INTEGER  NOT NULL DEFAULT '0',
`permanent` INTEGER  NOT NULL DEFAULT '0',
`extendState` INTEGER  NOT NULL DEFAULT '1',
PRIMARY KEY (`guid`,`instance`)
);
CREATE INDEX IF NOT EXISTS `character_instance__instance` ON `character_instance` (`instance`);
CREATE TABLE IF NOT EXISTS `character_inventory` (
`guid` INTEGER  NOT NULL DEFAULT '0',
`bag` INTEGER  NOT NULL DEFAULT '0',
`slot` INTEGER  NOT NULL DEFAULT '0',
`item` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`item`)
);
CREATE UNIQUE INDEX IF NOT EXISTS `character_inventory__guid` ON `character_inventory` (`guid`,`bag`,`slot`);
CREATE INDEX IF NOT EXISTS `character_inventory__idx_guid` ON `character_inventory` (`guid`);
CREATE TABLE IF NOT EXISTS `character_pet` (
`id` INTEGER  NOT NULL DEFAULT '0',
`entry` INTEGER  NOT NULL DEFAULT '0',
`owner` INTEGER  NOT NULL DEFAULT '0',
`modelid` INTEGER  DEFAULT '0',
`CreatedBySpell` INTEGER  NOT NULL DEFAULT '0',
`PetType` INTEGER  NOT NULL DEFAULT '0',
`level` INTEGER  NOT NULL DEFAULT '1',
`exp` INTEGER  NOT NULL DEFAULT '0',
`Reactstate` INTEGER  NOT NULL DEFAULT '0',
`name` TEXT NOT NULL DEFAULT 'Pet',
`renamed` INTEGER  NOT NULL DEFAULT '0',
`slot` INTEGER  NOT NULL DEFAULT '0',
`curhealth` INTEGER  NOT NULL DEFAULT '1',
`curmana` INTEGER  NOT NULL DEFAULT '0',
`curhappiness` INTEGER  NOT NULL DEFAULT '0',
`savetime` INTEGER  NOT NULL DEFAULT '0',
`abdata` TEXT,
PRIMARY KEY (`id`)
);
CREATE INDEX IF NOT EXISTS `character_pet__owner` ON `character_pet` (`owner`);
CREATE INDEX IF NOT EXISTS `character_pet__idx_slot` ON `character_pet` (`slot`);
CREATE TABLE IF NOT EXISTS `character_declinedname` (
`guid` INTEGER NOT NULL DEFAULT '0',
`genitive` TEXT NOT NULL DEFAULT '',
`dative` TEXT NOT NULL DEFAULT '',
`accusative` TEXT NOT NULL DEFAULT '',
`instrumental` TEXT NOT NULL DEFAULT '',
`prepositional` TEXT NOT NULL DEFAULT '',
PRIMARY KEY (`guid`)
);
CREATE TABLE IF NOT EXISTS `character_pet_declinedname` (
`id` INTEGER  NOT NULL DEFAULT '0',
`owner` INTEGER  NOT NULL DEFAULT '0',
`genitive` TEXT NOT NULL DEFAULT '',
`dative` TEXT NOT NULL DEFAULT '',
`accusative` TEXT NOT NULL DEFAULT '',
`instrumental` TEXT NOT NULL DEFAULT '',
`prepositional` TEXT NOT NULL DEFAULT '',
PRIMARY KEY (`id`)
);
CREATE INDEX IF NOT EXISTS `character_pet_declinedname__owner_key` ON `character_pet_declinedname` (`owner`);
CREATE TABLE IF NOT EXISTS `character_queststatus` (
`guid` INTEGER  NOT NULL DEFAULT '0',
`quest` INTEGER  NOT NULL DEFAULT '0',
`status` INTEGER  NOT NULL DEFAULT '0',
`explored` INTEGER  NOT NULL DEFAULT '0',
`timer` INTEGER  NOT NULL DEFAULT '0',
`mobcount1` INTEGER  NOT NULL DEFAULT '0',
`mobcount2` INTEGER  NOT NULL DEFAULT '0',
`mobcount3` INTEGER  NOT NULL DEFAULT '0',
`mobcount4` INTEGER  NOT NULL DEFAULT '0',
`itemcount1` INTEGER  NOT NULL DEFAULT '0',
`itemcount2` INTEGER  NOT NULL DEFAULT '0',
`itemcount3` INTEGER  NOT NULL DEFAULT '0',
`itemcount4` INTEGER  NOT NULL DEFAULT '0',
`itemcount5` INTEGER  NOT NULL DEFAULT '0',
`itemcount6` INTEGER  NOT NULL DEFAULT '0',
`playercount` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`guid`,`quest`)
);
CREATE TABLE IF NOT EXISTS `character_queststatus_daily` (
`guid` INTEGER  NOT NULL DEFAULT '0',
`quest` INTEGER  NOT NULL DEFAULT '0',
`time` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`guid`,`quest`)
);
CREATE INDEX IF NOT EXISTS `character_queststatus_daily__idx_guid` ON `character_queststatus_daily` (`guid`);
CREATE TABLE IF NOT EXISTS `character_queststatus_monthly` (
`guid` INTEGER  NOT NULL DEFAULT '0',
`quest` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`guid`,`quest`)
);
CREATE INDEX IF NOT EXISTS `character_queststatus_monthly__idx_guid` ON `character_queststatus_monthly` (`guid`);
CREATE TABLE IF NOT EXISTS `character_queststatus_rewarded` (
`guid` INTEGER  NOT NULL DEFAULT '0',
`quest` INTEGER  NOT NULL DEFAULT '0',
`active` INTEGER  NOT NULL DEFAULT '1',
PRIMARY KEY (`guid`,`quest`)
);
CREATE TABLE IF NOT EXISTS `character_queststatus_seasonal` (
`guid` INTEGER  NOT NULL DEFAULT '0',
`quest` INTEGER  NOT NULL DEFAULT '0',
`event` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`guid`,`quest`)
);
CREATE INDEX IF NOT EXISTS `character_queststatus_seasonal__idx_guid` ON `character_queststatus_seasonal` (`guid`);
CREATE TABLE IF NOT EXISTS `character_queststatus_weekly` (
`guid` INTEGER  NOT NULL DEFAULT '0',
`quest` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`guid`,`quest`)
);
CREATE INDEX IF NOT EXISTS `character_queststatus_weekly__idx_guid` ON `character_queststatus_weekly` (`guid`);
CREATE TABLE IF NOT EXISTS `character_reputation` (
`guid` INTEGER  NOT NULL DEFAULT '0',
`faction` INTEGER  NOT NULL DEFAULT '0',
`standing` INTEGER NOT NULL DEFAULT '0',
`flags` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`guid`,`faction`)
);
CREATE TABLE IF NOT EXISTS `character_skills` (
`guid` INTEGER  NOT NULL,
`skill` INTEGER  NOT NULL,
`value` INTEGER  NOT NULL,
`max` INTEGER  NOT NULL,
PRIMARY KEY (`guid`,`skill`)
);
CREATE TABLE IF NOT EXISTS `character_social` (
`guid` INTEGER  NOT NULL DEFAULT '0',
`friend` INTEGER  NOT NULL DEFAULT '0',
`flags` INTEGER  NOT NULL DEFAULT '0',
`note` TEXT NOT NULL DEFAULT '',
PRIMARY KEY (`guid`,`friend`,`flags`)
);
CREATE INDEX IF NOT EXISTS `character_social__friend` ON `character_social` (`friend`);
CREATE TABLE IF NOT EXISTS `character_spell` (
`guid` INTEGER  NOT NULL DEFAULT '0',
`spell` INTEGER  NOT NULL DEFAULT '0',
`active` INTEGER  NOT NULL DEFAULT '1',
`disabled` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`guid`,`spell`)
);
CREATE TABLE IF NOT EXISTS `character_spell_cooldown` (
`guid` INTEGER  NOT NULL DEFAULT '0',
`spell` INTEGER  NOT NULL DEFAULT '0',
`item` INTEGER  NOT NULL DEFAULT '0',
`time` INTEGER  NOT NULL DEFAULT '0',
`categoryId` INTEGER  NOT NULL DEFAULT '0',
`categoryEnd` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`guid`,`spell`)
);
CREATE TABLE IF NOT EXISTS `character_stats` (
`guid` INTEGER  NOT NULL DEFAULT '0',
`maxhealth` INTEGER  NOT NULL DEFAULT '0',
`maxpower1` INTEGER  NOT NULL DEFAULT '0',
`maxpower2` INTEGER  NOT NULL DEFAULT '0',
`maxpower3` INTEGER  NOT NULL DEFAULT '0',
`maxpower4` INTEGER  NOT NULL DEFAULT '0',
`maxpower5` INTEGER  NOT NULL DEFAULT '0',
`maxpower6` INTEGER  NOT NULL DEFAULT '0',
`maxpower7` INTEGER  NOT NULL DEFAULT '0',
`strength` INTEGER  NOT NULL DEFAULT '0',
`agility` INTEGER  NOT NULL DEFAULT '0',
`stamina` INTEGER  NOT NULL DEFAULT '0',
`intellect` INTEGER  NOT NULL DEFAULT '0',
`spirit` INTEGER  NOT NULL DEFAULT '0',
`armor` INTEGER  NOT NULL DEFAULT '0',
`resHoly` INTEGER  NOT NULL DEFAULT '0',
`resFire` INTEGER  NOT NULL DEFAULT '0',
`resNature` INTEGER  NOT NULL DEFAULT '0',
`resFrost` INTEGER  NOT NULL DEFAULT '0',
`resShadow` INTEGER  NOT NULL DEFAULT '0',
`resArcane` INTEGER  NOT NULL DEFAULT '0',
`blockPct` REAL  NOT NULL DEFAULT '0',
`dodgePct` REAL  NOT NULL DEFAULT '0',
`parryPct` REAL  NOT NULL DEFAULT '0',
`critPct` REAL  NOT NULL DEFAULT '0',
`rangedCritPct` REAL  NOT NULL DEFAULT '0',
`spellCritPct` REAL  NOT NULL DEFAULT '0',
`attackPower` INTEGER  NOT NULL DEFAULT '0',
`rangedAttackPower` INTEGER  NOT NULL DEFAULT '0',
`spellPower` INTEGER  NOT NULL DEFAULT '0',
`resilience` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`guid`)
);
CREATE TABLE IF NOT EXISTS `character_talent` (
`guid` INTEGER  NOT NULL,
`spell` INTEGER  NOT NULL,
`talentGroup` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`guid`,`spell`,`talentGroup`)
);
CREATE TABLE IF NOT EXISTS `characters` (
`guid` INTEGER  NOT NULL DEFAULT '0',
`account` INTEGER  NOT NULL DEFAULT '0',
`name` TEXT   NOT NULL,
`race` INTEGER  NOT NULL DEFAULT '0',
`class` INTEGER  NOT NULL DEFAULT '0',
`gender` INTEGER  NOT NULL DEFAULT '0',
`level` INTEGER  NOT NULL DEFAULT '0',
`xp` INTEGER  NOT NULL DEFAULT '0',
`money` INTEGER  NOT NULL DEFAULT '0',
`skin` INTEGER  NOT NULL DEFAULT '0',
`face` INTEGER  NOT NULL DEFAULT '0',
`hairStyle` INTEGER  NOT NULL DEFAULT '0',
`hairColor` INTEGER  NOT NULL DEFAULT '0',
`facialStyle` INTEGER  NOT NULL DEFAULT '0',
`bankSlots` INTEGER  NOT NULL DEFAULT '0',
`restState` INTEGER  NOT NULL DEFAULT '0',
`playerFlags` INTEGER  NOT NULL DEFAULT '0',
`position_x` REAL NOT NULL DEFAULT '0',
`position_y` REAL NOT NULL DEFAULT '0',
`position_z` REAL NOT NULL DEFAULT '0',
`map` INTEGER  NOT NULL DEFAULT '0',
`instance_id` INTEGER  NOT NULL DEFAULT '0',
`instance_mode_mask` INTEGER  NOT NULL DEFAULT '0',
`orientation` REAL NOT NULL DEFAULT '0',
`taximask` TEXT NOT NULL,
`online` INTEGER  NOT NULL DEFAULT '0',
`cinematic` INTEGER  NOT NULL DEFAULT '0',
`totaltime` INTEGER  NOT NULL DEFAULT '0',
`leveltime` INTEGER  NOT NULL DEFAULT '0',
`logout_time` INTEGER  NOT NULL DEFAULT '0',
`is_logout_resting` INTEGER  NOT NULL DEFAULT '0',
`rest_bonus` REAL NOT NULL DEFAULT '0',
`resettalents_cost` INTEGER  NOT NULL DEFAULT '0',
`resettalents_time` INTEGER  NOT NULL DEFAULT '0',
`trans_x` REAL NOT NULL DEFAULT '0',
`trans_y` REAL NOT NULL DEFAULT '0',
`trans_z` REAL NOT NULL DEFAULT '0',
`trans_o` REAL NOT NULL DEFAULT '0',
`transguid` INTEGER  NOT NULL DEFAULT '0',
`extra_flags` INTEGER  NOT NULL DEFAULT '0',
`stable_slots` INTEGER  NOT NULL DEFAULT '0',
`at_login` INTEGER  NOT NULL DEFAULT '0',
`zone` INTEGER  NOT NULL DEFAULT '0',
`death_expire_time` INTEGER  NOT NULL DEFAULT '0',
`taxi_path` TEXT,
`arenaPoints` INTEGER  NOT NULL DEFAULT '0',
`totalHonorPoints` INTEGER  NOT NULL DEFAULT '0',
`todayHonorPoints` INTEGER  NOT NULL DEFAULT '0',
`yesterdayHonorPoints` INTEGER  NOT NULL DEFAULT '0',
`totalKills` INTEGER  NOT NULL DEFAULT '0',
`todayKills` INTEGER  NOT NULL DEFAULT '0',
`yesterdayKills` INTEGER  NOT NULL DEFAULT '0',
`chosenTitle` INTEGER  NOT NULL DEFAULT '0',
`knownCurrencies` INTEGER  NOT NULL DEFAULT '0',
`watchedFaction` INTEGER  NOT NULL DEFAULT '0',
`drunk` INTEGER  NOT NULL DEFAULT '0',
`health` INTEGER  NOT NULL DEFAULT '0',
`power1` INTEGER  NOT NULL DEFAULT '0',
`power2` INTEGER  NOT NULL DEFAULT '0',
`power3` INTEGER  NOT NULL DEFAULT '0',
`power4` INTEGER  NOT NULL DEFAULT '0',
`power5` INTEGER  NOT NULL DEFAULT '0',
`power6` INTEGER  NOT NULL DEFAULT '0',
`power7` INTEGER  NOT NULL DEFAULT '0',
`latency` INTEGER  NOT NULL DEFAULT '0',
`talentGroupsCount` INTEGER  NOT NULL DEFAULT '1',
`activeTalentGroup` INTEGER  NOT NULL DEFAULT '0',
`exploredZones` TEXT,
`equipmentCache` TEXT,
`ammoId` INTEGER  NOT NULL DEFAULT '0',
`knownTitles` TEXT,
`actionBars` INTEGER  NOT NULL DEFAULT '0',
`grantableLevels` INTEGER  NOT NULL DEFAULT '0',
`deleteInfos_Account` INTEGER  DEFAULT NULL,
`deleteInfos_Name` TEXT DEFAULT NULL,
`deleteDate` INTEGER  DEFAULT NULL,
PRIMARY KEY (`guid`)
);
CREATE INDEX IF NOT EXISTS `characters__idx_account` ON `characters` (`account`);
CREATE INDEX IF NOT EXISTS `characters__idx_online` ON `characters` (`online`);
CREATE INDEX IF NOT EXISTS `characters__idx_name` ON `characters` (`name`);
CREATE TABLE IF NOT EXISTS `characters_npcbot` (
`entry` INTEGER  NOT NULL,
`owner` INTEGER  NOT NULL DEFAULT '0',
`roles` INTEGER  NOT NULL,
`spec` INTEGER  NOT NULL DEFAULT '1',
`faction` INTEGER  NOT NULL DEFAULT '35',
`equipMhEx` INTEGER  NOT NULL DEFAULT '0',
`equipOhEx` INTEGER  NOT NULL DEFAULT '0',
`equipRhEx` INTEGER  NOT NULL DEFAULT '0',
`equipHead` INTEGER  NOT NULL DEFAULT '0',
`equipShoulders` INTEGER  NOT NULL DEFAULT '0',
`equipChest` INTEGER  NOT NULL DEFAULT '0',
`equipWaist` INTEGER  NOT NULL DEFAULT '0',
`equipLegs` INTEGER  NOT NULL DEFAULT '0',
`equipFeet` INTEGER  NOT NULL DEFAULT '0',
`equipWrist` INTEGER  NOT NULL DEFAULT '0',
`equipHands` INTEGER  NOT NULL DEFAULT '0',
`equipBack` INTEGER  NOT NULL DEFAULT '0',
`equipBody` INTEGER  NOT NULL DEFAULT '0',
`equipFinger1` INTEGER  NOT NULL DEFAULT '0',
`equipFinger2` INTEGER  NOT NULL DEFAULT '0',
`equipTrinket1` INTEGER  NOT NULL DEFAULT '0',
`equipTrinket2` INTEGER  NOT NULL DEFAULT '0',
`equipNeck` INTEGER  NOT NULL DEFAULT '0',
`spells_disabled` TEXT,
PRIMARY KEY (`entry`)
);
CREATE TABLE IF NOT EXISTS `corpse` (
`guid` INTEGER  NOT NULL DEFAULT '0',
`posX` REAL NOT NULL DEFAULT '0',
`posY` REAL NOT NULL DEFAULT '0',
`posZ` REAL NOT NULL DEFAULT '0',
`orientation` REAL NOT NULL DEFAULT '0',
`mapId` INTEGER  NOT NULL DEFAULT '0',
`phaseMask` INTEGER  NOT NULL DEFAULT '1',
`displayId` INTEGER  NOT NULL DEFAULT '0',
`itemCache` TEXT NOT NULL,
`bytes1` INTEGER  NOT NULL DEFAULT '0',
`bytes2` INTEGER  NOT NULL DEFAULT '0',
`guildId` INTEGER  NOT NULL DEFAULT '0',
`flags` INTEGER  NOT NULL DEFAULT '0',
`dynFlags` INTEGER  NOT NULL DEFAULT '0',
`time` INTEGER  NOT NULL DEFAULT '0',
`corpseType` INTEGER  NOT NULL DEFAULT '0',
`instanceId` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`guid`)
);
CREATE INDEX IF NOT EXISTS `corpse__idx_type` ON `corpse` (`corpseType`);
CREATE INDEX IF NOT EXISTS `corpse__idx_instance` ON `corpse` (`instanceId`);
CREATE INDEX IF NOT EXISTS `corpse__idx_time` ON `corpse` (`time`);
CREATE TABLE IF NOT EXISTS `custom_item_enchant_visuals` (
`iguid` INTEGER  NOT NULL,
`display` INTEGER  NOT NULL,
PRIMARY KEY (`iguid`)
);
CREATE TABLE IF NOT EXISTS `custom_transmogrification` (
`GUID` INTEGER  NOT NULL,
`FakeEntry` INTEGER  NOT NULL,
`Owner` INTEGER  NOT NULL,
PRIMARY KEY (`GUID`)
);
CREATE TABLE IF NOT EXISTS `game_event_condition_save` (
`eventEntry` INTEGER  NOT NULL,
`condition_id` INTEGER  NOT NULL DEFAULT '0',
`done` REAL DEFAULT '0',
PRIMARY KEY (`eventEntry`,`condition_id`)
);
CREATE TABLE IF NOT EXISTS `game_event_save` (
`eventEntry` INTEGER  NOT NULL,
`state` INTEGER  NOT NULL DEFAULT '1',
`next_start` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`eventEntry`)
);
CREATE TABLE IF NOT EXISTS `gm_subsurvey` (
`surveyId` INTEGER  NOT NULL,
`questionId` INTEGER  NOT NULL DEFAULT '0',
`answer` INTEGER  NOT NULL DEFAULT '0',
`answerComment` TEXT NOT NULL,
PRIMARY KEY (`surveyId`,`questionId`)
);
CREATE TABLE IF NOT EXISTS `gm_survey` (
`surveyId` INTEGER  NOT NULL,
`guid` INTEGER  NOT NULL DEFAULT '0',
`mainSurvey` INTEGER  NOT NULL DEFAULT '0',
`comment` TEXT NOT NULL,
`createTime` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`surveyId`)
);
CREATE TABLE IF NOT EXISTS `gm_ticket` (
`id` INTEGER  NOT NULL,
`type` INTEGER  NOT NULL DEFAULT '0',
`playerGuid` INTEGER  NOT NULL DEFAULT '0',
`name` TEXT NOT NULL,
`description` TEXT NOT NULL,
`createTime` INTEGER  NOT NULL DEFAULT '0',
`mapId` INTEGER  NOT NULL DEFAULT '0',
`posX` REAL NOT NULL DEFAULT '0',
`posY` REAL NOT NULL DEFAULT '0',
`posZ` REAL NOT NULL DEFAULT '0',
`lastModifiedTime` INTEGER  NOT NULL DEFAULT '0',
`closedBy` INTEGER NOT NULL DEFAULT '0',
`assignedTo` INTEGER  NOT NULL DEFAULT '0',
`comment` TEXT NOT NULL,
`response` TEXT NOT NULL,
`completed` INTEGER  NOT NULL DEFAULT '0',
`escalated` INTEGER  NOT NULL DEFAULT '0',
`viewed` INTEGER  NOT NULL DEFAULT '0',
`needMoreHelp` INTEGER  NOT NULL DEFAULT '0',
`resolvedBy` INTEGER NOT NULL DEFAULT '0',
PRIMARY KEY (`id`)
);
CREATE TABLE IF NOT EXISTS `group_instance` (
`guid` INTEGER  NOT NULL DEFAULT '0',
`instance` INTEGER  NOT NULL DEFAULT '0',
`permanent` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`guid`,`instance`)
);
CREATE INDEX IF NOT EXISTS `group_instance__instance` ON `group_instance` (`instance`);
CREATE TABLE IF NOT EXISTS `group_member` (
`guid` INTEGER  NOT NULL,
`memberGuid` INTEGER  NOT NULL,
`memberFlags` INTEGER  NOT NULL DEFAULT '0',
`subgroup` INTEGER  NOT NULL DEFAULT '0',
`roles` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`memberGuid`)
);
CREATE TABLE IF NOT EXISTS `groups` (
`guid` INTEGER  NOT NULL,
`leaderGuid` INTEGER  NOT NULL,
`lootMethod` INTEGER  NOT NULL,
`looterGuid` INTEGER  NOT NULL,
`lootThreshold` INTEGER  NOT NULL,
`icon1` INTEGER  NOT NULL,
`icon2` INTEGER  NOT NULL,
`icon3` INTEGER  NOT NULL,
`icon4` INTEGER  NOT NULL,
`icon5` INTEGER  NOT NULL,
`icon6` INTEGER  NOT NULL,
`icon7` INTEGER  NOT NULL,
`icon8` INTEGER  NOT NULL,
`groupType` INTEGER  NOT NULL,
`difficulty` INTEGER  NOT NULL DEFAULT '0',
`raidDifficulty` INTEGER  NOT NULL DEFAULT '0',
`masterLooterGuid` INTEGER  NOT NULL,
PRIMARY KEY (`guid`)
);
CREATE INDEX IF NOT EXISTS `groups__leaderGuid` ON `groups` (`leaderGuid`);
CREATE TABLE IF NOT EXISTS `guild` (
`guildid` INTEGER  NOT NULL DEFAULT '0',
`name` TEXT NOT NULL DEFAULT '',
`leaderguid` INTEGER  NOT NULL DEFAULT '0',
`EmblemStyle` INTEGER  NOT NULL DEFAULT '0',
`EmblemColor` INTEGER  NOT NULL DEFAULT '0',
`BorderStyle` INTEGER  NOT NULL DEFAULT '0',
`BorderColor` INTEGER  NOT NULL DEFAULT '0',
`BackgroundColor` INTEGER  NOT NULL DEFAULT '0',
`info` TEXT NOT NULL DEFAULT '',
`motd` TEXT NOT NULL DEFAULT '',
`createdate` INTEGER  NOT NULL DEFAULT '0',
`BankMoney` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`guildid`)
);
CREATE TABLE IF NOT EXISTS `guild_bank_eventlog` (
`guildid` INTEGER  NOT NULL DEFAULT '0',
`LogGuid` INTEGER  NOT NULL DEFAULT '0',
`TabId` INTEGER  NOT NULL DEFAULT '0',
`EventType` INTEGER  NOT NULL DEFAULT '0',
`PlayerGuid` INTEGER  NOT NULL DEFAULT '0',
`ItemOrMoney` INTEGER  NOT NULL DEFAULT '0',
`ItemStackCount` INTEGER  NOT NULL DEFAULT '0',
`DestTabId` INTEGER  NOT NULL DEFAULT '0',
`TimeStamp` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`guildid`,`LogGuid`,`TabId`)
);
CREATE INDEX IF NOT EXISTS `guild_bank_eventlog__guildid_key` ON `guild_bank_eventlog` (`guildid`);
CREATE INDEX IF NOT EXISTS `guild_bank_eventlog__Idx_PlayerGuid` ON `guild_bank_eventlog` (`PlayerGuid`);
CREATE INDEX IF NOT EXISTS `guild_bank_eventlog__Idx_LogGuid` ON `guild_bank_eventlog` (`LogGuid`);
CREATE TABLE IF NOT EXISTS `guild_bank_item` (
`guildid` INTEGER  NOT NULL DEFAULT '0',
`TabId` INTEGER  NOT NULL DEFAULT '0',
`SlotId` INTEGER  NOT NULL DEFAULT '0',
`item_guid` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`guildid`,`TabId`,`SlotId`)
);
CREATE INDEX IF NOT EXISTS `guild_bank_item__guildid_key` ON `guild_bank_item` (`guildid`);
CREATE INDEX IF NOT EXISTS `guild_bank_item__Idx_item_guid` ON `guild_bank_item` (`item_guid`);
CREATE TABLE IF NOT EXISTS `guild_bank_right` (
`guildid` INTEGER  NOT NULL DEFAULT '0',
`TabId` INTEGER  NOT NULL DEFAULT '0',
`rid` INTEGER  NOT NULL DEFAULT '0',
`gbright` INTEGER  NOT NULL DEFAULT '0',
`SlotPerDay` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`guildid`,`TabId`,`rid`)
);
CREATE INDEX IF NOT EXISTS `guild_bank_right__guildid_key` ON `guild_bank_right` (`guildid`);
CREATE TABLE IF NOT EXISTS `guild_bank_tab` (
`guildid` INTEGER  NOT NULL DEFAULT '0',
`TabId` INTEGER  NOT NULL DEFAULT '0',
`TabName` TEXT NOT NULL DEFAULT '',
`TabIcon` TEXT NOT NULL DEFAULT '',
`TabText` TEXT DEFAULT NULL,
PRIMARY KEY (`guildid`,`TabId`)
);
CREATE INDEX IF NOT EXISTS `guild_bank_tab__guildid_key` ON `guild_bank_tab` (`guildid`);
CREATE TABLE IF NOT EXISTS `guild_eventlog` (
`guildid` INTEGER  NOT NULL,
`LogGuid` INTEGER  NOT NULL,
`EventType` INTEGER  NOT NULL,
`PlayerGuid1` INTEGER  NOT NULL,
`PlayerGuid2` INTEGER  NOT NULL,
`NewRank` INTEGER  NOT NULL,
`TimeStamp` INTEGER  NOT NULL,
PRIMARY KEY (`guildid`,`LogGuid`)
);
CREATE INDEX IF NOT EXISTS `guild_eventlog__Idx_PlayerGuid1` ON `guild_eventlog` (`PlayerGuid1`);
CREATE INDEX IF NOT EXISTS `guild_eventlog__Idx_PlayerGuid2` ON `guild_eventlog` (`PlayerGuid2`);
CREATE INDEX IF NOT EXISTS `guild_eventlog__Idx_LogGuid` ON `guild_eventlog` (`LogGuid`);
CREATE TABLE IF NOT EXISTS `guild_member` (
`guildid` INTEGER  NOT NULL,
`guid` INTEGER  NOT NULL,
`rank` INTEGER  NOT NULL,
`pnote` TEXT NOT NULL DEFAULT '',
`offnote` TEXT NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX IF NOT EXISTS `guild_member__guid_key` ON `guild_member` (`guid`);
CREATE INDEX IF NOT EXISTS `guild_member__guildid_key` ON `guild_member` (`guildid`);
CREATE INDEX IF NOT EXISTS `guild_member__guildid_rank_key` ON `guild_member` (`guildid`,`rank`);
CREATE TABLE IF NOT EXISTS `guild_member_withdraw` (
`guid` INTEGER  NOT NULL,
`tab0` INTEGER  NOT NULL DEFAULT '0',
`tab1` INTEGER  NOT NULL DEFAULT '0',
`tab2` INTEGER  NOT NULL DEFAULT '0',
`tab3` INTEGER  NOT NULL DEFAULT '0',
`tab4` INTEGER  NOT NULL DEFAULT '0',
`tab5` INTEGER  NOT NULL DEFAULT '0',
`money` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`guid`)
);
CREATE TABLE IF NOT EXISTS `guild_rank` (
`guildid` INTEGER  NOT NULL DEFAULT '0',
`rid` INTEGER  NOT NULL,
`rname` TEXT NOT NULL DEFAULT '',
`rights` INTEGER  NOT NULL DEFAULT '0',
`BankMoneyPerDay` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`guildid`,`rid`)
);
CREATE INDEX IF NOT EXISTS `guild_rank__Idx_rid` ON `guild_rank` (`rid`);
CREATE TABLE IF NOT EXISTS `instance` (
`id` INTEGER  NOT NULL DEFAULT '0',
`map` INTEGER  NOT NULL DEFAULT '0',
`resettime` INTEGER  NOT NULL DEFAULT '0',
`difficulty` INTEGER  NOT NULL DEFAULT '0',
`completedEncounters` INTEGER  NOT NULL DEFAULT '0',
`data` TEXT NOT NULL,
PRIMARY KEY (`id`)
);
CREATE INDEX IF NOT EXISTS `instance__map` ON `instance` (`map`);
CREATE INDEX IF NOT EXISTS `instance__resettime` ON `instance` (`resettime`);
CREATE INDEX IF NOT EXISTS `instance__difficulty` ON `instance` (`difficulty`);
CREATE TABLE IF NOT EXISTS `instance_reset` (
`mapid` INTEGER  NOT NULL DEFAULT '0',
`difficulty` INTEGER  NOT NULL DEFAULT '0',
`resettime` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`mapid`,`difficulty`)
);
CREATE INDEX IF NOT EXISTS `instance_reset__difficulty` ON `instance_reset` (`difficulty`);
CREATE TABLE IF NOT EXISTS `item_instance` (
`guid` INTEGER  NOT NULL DEFAULT '0',
`itemEntry` INTEGER  NOT NULL DEFAULT '0',
`owner_guid` INTEGER  NOT NULL DEFAULT '0',
`creatorGuid` INTEGER  NOT NULL DEFAULT '0',
`giftCreatorGuid` INTEGER  NOT NULL DEFAULT '0',
`count` INTEGER  NOT NULL DEFAULT '1',
`duration` INTEGER NOT NULL DEFAULT '0',
`charges` TEXT,
`flags` INTEGER  NOT NULL DEFAULT '0',
`enchantments` TEXT NOT NULL,
`randomPropertyId` INTEGER NOT NULL DEFAULT '0',
`durability` INTEGER  NOT NULL DEFAULT '0',
`playedTime` INTEGER  NOT NULL DEFAULT '0',
`text` TEXT,
PRIMARY KEY (`guid`)
);
CREATE INDEX IF NOT EXISTS `item_instance__idx_owner_guid` ON `item_instance` (`owner_guid`);
CREATE TABLE IF NOT EXISTS `item_loot_items` (
`container_id` INTEGER  NOT NULL DEFAULT '0',
`item_id` INTEGER  NOT NULL DEFAULT '0',
`item_count` INTEGER NOT NULL DEFAULT '0',
`follow_rules` INTEGER NOT NULL DEFAULT '0',
`ffa` INTEGER NOT NULL DEFAULT '0',
`blocked` INTEGER NOT NULL DEFAULT '0',
`counted` INTEGER NOT NULL DEFAULT '0',
`under_threshold` INTEGER NOT NULL DEFAULT '0',
`needs_quest` INTEGER NOT NULL DEFAULT '0',
`rnd_prop` INTEGER NOT NULL DEFAULT '0',
`rnd_suffix` INTEGER NOT NULL DEFAULT '0'
);
CREATE TABLE IF NOT EXISTS `item_loot_money` (
`container_id` INTEGER  NOT NULL DEFAULT '0',
`money` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`container_id`)
);
CREATE TABLE IF NOT EXISTS `item_refund_instance` (
`item_guid` INTEGER  NOT NULL,
`player_guid` INTEGER  NOT NULL,
`paidMoney` INTEGER  NOT NULL DEFAULT '0',
`paidExtendedCost` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`item_guid`,`player_guid`)
);
CREATE TABLE IF NOT EXISTS `item_soulbound_trade_data` (
`itemGuid` INTEGER  NOT NULL,
`allowedPlayers` TEXT NOT NULL,
PRIMARY KEY (`itemGuid`)
);
CREATE TABLE IF NOT EXISTS `lag_reports` (
`reportId` INTEGER  NOT NULL,
`guid` INTEGER  NOT NULL DEFAULT '0',
`lagType` INTEGER  NOT NULL DEFAULT '0',
`mapId` INTEGER  NOT NULL DEFAULT '0',
`posX` REAL NOT NULL DEFAULT '0',
`posY` REAL NOT NULL DEFAULT '0',
`posZ` REAL NOT NULL DEFAULT '0',
`latency` INTEGER  NOT NULL DEFAULT '0',
`createTime` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`reportId`)
);
CREATE TABLE IF NOT EXISTS `lfg_data` (
`guid` INTEGER  NOT NULL DEFAULT '0',
`dungeon` INTEGER  NOT NULL DEFAULT '0',
`state` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`guid`)
);
CREATE TABLE IF NOT EXISTS `mail` (
`id` INTEGER  NOT NULL DEFAULT '0',
`messageType` INTEGER  NOT NULL DEFAULT '0',
`stationery` INTEGER NOT NULL DEFAULT '41',
`mailTemplateId` INTEGER  NOT NULL DEFAULT '0',
`sender` INTEGER  NOT NULL DEFAULT '0',
`receiver` INTEGER  NOT NULL DEFAULT '0',
`subject` TEXT,
`body` TEXT,
`has_items` INTEGER  NOT NULL DEFAULT '0',
`expire_time` INTEGER  NOT NULL DEFAULT '0',
`deliver_time` INTEGER  NOT NULL DEFAULT '0',
`money` INTEGER  NOT NULL DEFAULT '0',
`cod` INTEGER  NOT NULL DEFAULT '0',
`checked` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`id`)
);
CREATE INDEX IF NOT EXISTS `mail__idx_receiver` ON `mail` (`receiver`);
CREATE TABLE IF NOT EXISTS `mail_items` (
`mail_id` INTEGER  NOT NULL DEFAULT '0',
`item_guid` INTEGER  NOT NULL DEFAULT '0',
`receiver` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`item_guid`)
);
CREATE INDEX IF NOT EXISTS `mail_items__idx_receiver` ON `mail_items` (`receiver`);
CREATE INDEX IF NOT EXISTS `mail_items__idx_mail_id` ON `mail_items` (`mail_id`);
CREATE TABLE IF NOT EXISTS `pet_aura` (
`guid` INTEGER  NOT NULL DEFAULT '0',
`casterGuid` TEXT NOT NULL DEFAULT '0',
`spell` INTEGER  NOT NULL DEFAULT '0',
`effectMask` INTEGER  NOT NULL DEFAULT '0',
`recalculateMask` INTEGER  NOT NULL DEFAULT '0',
`stackCount` INTEGER  NOT NULL DEFAULT '1',
`amount0` INTEGER NOT NULL,
`amount1` INTEGER NOT NULL,
`amount2` INTEGER NOT NULL,
`base_amount0` INTEGER NOT NULL,
`base_amount1` INTEGER NOT NULL,
`base_amount2` INTEGER NOT NULL,
`maxDuration` INTEGER NOT NULL DEFAULT '0',
`remainTime` INTEGER NOT NULL DEFAULT '0',
`remainCharges` INTEGER  NOT NULL DEFAULT '0',
`critChance` REAL NOT NULL DEFAULT '0',
`applyResilience` INTEGER NOT NULL DEFAULT '0',
PRIMARY KEY (`guid`,`casterGuid`,`spell`,`effectMask`)
);
CREATE TABLE IF NOT EXISTS `pet_spell` (
`guid` INTEGER  NOT NULL DEFAULT '0',
`spell` INTEGER  NOT NULL DEFAULT '0',
`active` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`guid`,`spell`)
);
CREATE TABLE IF NOT EXISTS `pet_spell_cooldown` (
`guid` INTEGER  NOT NULL DEFAULT '0',
`spell` INTEGER  NOT NULL DEFAULT '0',
`time` INTEGER  NOT NULL DEFAULT '0',
`categoryId` INTEGER  NOT NULL DEFAULT '0',
`categoryEnd` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`guid`,`spell`)
);
CREATE TABLE IF NOT EXISTS `petition` (
`ownerguid` INTEGER  NOT NULL,
`petitionguid` INTEGER  DEFAULT '0',
`name` TEXT NOT NULL,
`type` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`ownerguid`,`type`)
);
CREATE UNIQUE INDEX IF NOT EXISTS `petition__index_ownerguid_petitionguid` ON `petition` (`ownerguid`,`petitionguid`);
CREATE TABLE IF NOT EXISTS `petition_sign` (
`ownerguid` INTEGER  NOT NULL,
`petitionguid` INTEGER  NOT NULL DEFAULT '0',
`playerguid` INTEGER  NOT NULL DEFAULT '0',
`player_account` INTEGER  NOT NULL DEFAULT '0',
`type` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`petitionguid`,`playerguid`)
);
CREATE INDEX IF NOT EXISTS `petition_sign__Idx_playerguid` ON `petition_sign` (`playerguid`);
CREATE INDEX IF NOT EXISTS `petition_sign__Idx_ownerguid` ON `petition_sign` (`ownerguid`);
CREATE TABLE IF NOT EXISTS `pool_quest_save` (
`pool_id` INTEGER  NOT NULL DEFAULT '0',
`quest_id` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`pool_id`,`quest_id`)
);
CREATE TABLE IF NOT EXISTS `pvpstats_battlegrounds` (
`id` INTEGER  NOT NULL,
`winner_faction` INTEGER NOT NULL,
`bracket_id` INTEGER  NOT NULL,
`type` INTEGER  NOT NULL,
`date` TEXT NOT NULL,
PRIMARY KEY (`id`)
);
CREATE TABLE IF NOT EXISTS `pvpstats_players` (
`battleground_id` INTEGER  NOT NULL,
`character_guid` INTEGER  NOT NULL,
`winner` INTEGER NOT NULL,
`score_killing_blows` INTEGER  NOT NULL,
`score_deaths` INTEGER  NOT NULL,
`score_honorable_kills` INTEGER  NOT NULL,
`score_bonus_honor` INTEGER  NOT NULL,
`score_damage_done` INTEGER  NOT NULL,
`score_healing_done` INTEGER  NOT NULL,
`attr_1` INTEGER  NOT NULL DEFAULT '0',
`attr_2` INTEGER  NOT NULL DEFAULT '0',
`attr_3` INTEGER  NOT NULL DEFAULT '0',
`attr_4` INTEGER  NOT NULL DEFAULT '0',
`attr_5` INTEGER  NOT NULL DEFAULT '0',
PRIMARY KEY (`battleground_id`,`character_guid`)
);
CREATE TABLE IF NOT EXISTS `quest_tracker` (
`id` INTEGER  NOT NULL DEFAULT '0',
`character_guid` INTEGER  NOT NULL DEFAULT '0',
`quest_accept_time` TEXT NOT NULL,
`quest_complete_time` TEXT DEFAULT NULL,
`quest_abandon_time` TEXT DEFAULT NULL,
`completed_by_gm` INTEGER NOT NULL DEFAULT '0',
`core_hash` TEXT NOT NULL DEFAULT '0',
`core_revision` TEXT NOT NULL DEFAULT '0'
);
CREATE TABLE IF NOT EXISTS `reserved_name` (
`name` TEXT NOT NULL DEFAULT '',
PRIMARY KEY (`name`)
);
CREATE TABLE IF NOT EXISTS `respawn` (
`type` INTEGER  NOT NULL,
`spawnId` INTEGER  NOT NULL,
`respawnTime` INTEGER  NOT NULL,
`mapId` INTEGER  NOT NULL,
`instanceId` INTEGER  NOT NULL,
PRIMARY KEY (`type`,`spawnId`,`instanceId`)
);
CREATE INDEX IF NOT EXISTS `respawn__idx_instance` ON `respawn` (`instanceId`);
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
CREATE TABLE IF NOT EXISTS `warden_action` (
`wardenId` INTEGER  NOT NULL,
`action` INTEGER  DEFAULT NULL,
PRIMARY KEY (`wardenId`)
);
CREATE TABLE IF NOT EXISTS `worldstates` (
`entry` INTEGER  NOT NULL DEFAULT '0',
`value` INTEGER  NOT NULL DEFAULT '0',
`comment` TEXT,
PRIMARY KEY (`entry`)
);
CREATE TABLE IF NOT EXISTS `character_pet` (
`id` INTEGER PRIMARY KEY,
`entry` INTEGER NOT NULL DEFAULT '0',
`owner` INTEGER NOT NULL DEFAULT '0',
`modelid` INTEGER DEFAULT '0',
`CreatedBySpell` INTEGER NOT NULL DEFAULT '0',
`PetType` INTEGER NOT NULL DEFAULT '0',
`level` INTEGER NOT NULL DEFAULT '1',
`exp` INTEGER NOT NULL DEFAULT '0',
`Reactstate` INTEGER NOT NULL DEFAULT '0',
`name` TEXT NOT NULL DEFAULT 'Pet',
`renamed` INTEGER NOT NULL DEFAULT '0',
`slot` INTEGER NOT NULL DEFAULT '0',
`curhealth` INTEGER NOT NULL DEFAULT '1',
`curmana` INTEGER NOT NULL DEFAULT '0',
`curhappiness` INTEGER NOT NULL DEFAULT '0',
`savetime` INTEGER NOT NULL DEFAULT '0',
`abdata` TEXT
);
CREATE INDEX IF NOT EXISTS `character_pet__idx_owner` ON `character_pet` (`owner`);
CREATE INDEX IF NOT EXISTS `character_pet__idx_slot` ON `character_pet` (`slot`);
CREATE TABLE IF NOT EXISTS `character_pet_declinedname` (
`id` INTEGER NOT NULL DEFAULT '0',
`owner` INTEGER NOT NULL DEFAULT '0',
`genitive` TEXT NOT NULL DEFAULT '',
`dative` TEXT NOT NULL DEFAULT '',
`accusative` TEXT NOT NULL DEFAULT '',
`instrumental` TEXT NOT NULL DEFAULT '',
`prepositional` TEXT NOT NULL DEFAULT '',
PRIMARY KEY (`id`)
);
CREATE INDEX IF NOT EXISTS `character_pet_declinedname__idx_owner` ON `character_pet_declinedname` (`owner`);
CREATE TABLE IF NOT EXISTS `pet_spell` (
`guid` INTEGER NOT NULL DEFAULT '0',
`spell` INTEGER NOT NULL DEFAULT '0',
`active` INTEGER NOT NULL DEFAULT '0',
PRIMARY KEY (`guid`,`spell`)
);
CREATE TABLE IF NOT EXISTS `pet_spell_cooldown` (
`guid` INTEGER NOT NULL DEFAULT '0',
`spell` INTEGER NOT NULL DEFAULT '0',
`time` INTEGER NOT NULL DEFAULT '0',
`categoryId` INTEGER NOT NULL DEFAULT '0',
`categoryEnd` INTEGER NOT NULL DEFAULT '0',
PRIMARY KEY (`guid`,`spell`)
);
