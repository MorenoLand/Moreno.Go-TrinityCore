-- Hadronox crusher-pack summon groups (Azjol-Nerub, boss_hadronox.cpp
-- SummonCrusherPack, groups 1-3). Rows transcribed verbatim from TDB
-- 335.26091 creature_summon_groups for summoner 28921 (Hadronox),
-- summonerType 0 (SUMMONER_TYPE_CREATURE). The SummonCreatureGroup
-- bridge (lua_creature_events.go:4218) reads exactly these rows at
-- summon time; without them the producer returns an empty array. The
-- NOT EXISTS guard keeps the migration a no-op on a deployment DB
-- already populated by a full TDB import (the table carries no primary
-- key, so a plain INSERT would duplicate the rows). The summoned
-- entries (28922 crusher, 29117 champion, 29118 crypt fiend, 29119
-- necromancer) still need their creature_template rows in the
-- deployment world DB — summonTempCreature returns nil without one
-- (the 50415 precedent in tools/spelldbc_seed/main.go).
INSERT INTO creature_summon_groups (summonerId, summonerType, groupId, entry, position_x, position_y, position_z, orientation, summonType, summonTime, Comment)
SELECT 28921, 0, 1, 28922, 529.691, 547.126, 731.916, 4.79965, 6, 45000, 'Hadronox - Group 1 - Anub''ar Crusher'
WHERE NOT EXISTS (SELECT 1 FROM creature_summon_groups WHERE summonerId = 28921 AND summonerType = 0 AND groupId = 1 AND entry = 28922);
INSERT INTO creature_summon_groups (summonerId, summonerType, groupId, entry, position_x, position_y, position_z, orientation, summonType, summonTime, Comment)
SELECT 28921, 0, 1, 29117, 539.208, 549.754, 732.867, 4.55531, 6, 30000, 'Hadronox - Group 1 - Anub''ar Champion'
WHERE NOT EXISTS (SELECT 1 FROM creature_summon_groups WHERE summonerId = 28921 AND summonerType = 0 AND groupId = 1 AND entry = 29117);
INSERT INTO creature_summon_groups (summonerId, summonerType, groupId, entry, position_x, position_y, position_z, orientation, summonType, summonTime, Comment)
SELECT 28921, 0, 1, 29118, 520.391, 548.789, 732.012, 5.0091, 6, 30000, 'Hadronox - Group 1 - Anub''ar Crypt Fiend'
WHERE NOT EXISTS (SELECT 1 FROM creature_summon_groups WHERE summonerId = 28921 AND summonerType = 0 AND groupId = 1 AND entry = 29118);
INSERT INTO creature_summon_groups (summonerId, summonerType, groupId, entry, position_x, position_y, position_z, orientation, summonType, summonTime, Comment)
SELECT 28921, 0, 2, 28922, 493.477, 603.344, 760.563, 5.44024, 6, 45000, 'Hadronox - Group 2 - Anub''ar Crusher'
WHERE NOT EXISTS (SELECT 1 FROM creature_summon_groups WHERE summonerId = 28921 AND summonerType = 0 AND groupId = 2 AND entry = 28922);
INSERT INTO creature_summon_groups (summonerId, summonerType, groupId, entry, position_x, position_y, position_z, orientation, summonType, summonTime, Comment)
SELECT 28921, 0, 2, 29117, 490.442, 604.335, 763.182, 5.6256, 6, 30000, 'Hadronox - Group 2 - Anub''ar Champion'
WHERE NOT EXISTS (SELECT 1 FROM creature_summon_groups WHERE summonerId = 28921 AND summonerType = 0 AND groupId = 2 AND entry = 29117);
INSERT INTO creature_summon_groups (summonerId, summonerType, groupId, entry, position_x, position_y, position_z, orientation, summonType, summonTime, Comment)
SELECT 28921, 0, 2, 29119, 488.825, 609.282, 767.588, 5.59029, 6, 30000, 'Hadronox - Group 2 - Anub''ar Necromancer'
WHERE NOT EXISTS (SELECT 1 FROM creature_summon_groups WHERE summonerId = 28921 AND summonerType = 0 AND groupId = 2 AND entry = 29119);
INSERT INTO creature_summon_groups (summonerId, summonerType, groupId, entry, position_x, position_y, position_z, orientation, summonType, summonTime, Comment)
SELECT 28921, 0, 3, 28922, 566.979, 602.571, 759.642, 3.88597, 6, 45000, 'Hadronox - Group 3 - Anub''ar Crusher'
WHERE NOT EXISTS (SELECT 1 FROM creature_summon_groups WHERE summonerId = 28921 AND summonerType = 0 AND groupId = 3 AND entry = 28922);
INSERT INTO creature_summon_groups (summonerId, summonerType, groupId, entry, position_x, position_y, position_z, orientation, summonType, summonTime, Comment)
SELECT 28921, 0, 3, 29118, 569.348, 604.999, 763.214, 4.17983, 6, 30000, 'Hadronox - Group 3 - Anub''ar Crypt Fiend'
WHERE NOT EXISTS (SELECT 1 FROM creature_summon_groups WHERE summonerId = 28921 AND summonerType = 0 AND groupId = 3 AND entry = 29118);
INSERT INTO creature_summon_groups (summonerId, summonerType, groupId, entry, position_x, position_y, position_z, orientation, summonType, summonTime, Comment)
SELECT 28921, 0, 3, 29119, 572.474, 607.411, 767.178, 3.94417, 6, 30000, 'Hadronox - Group 3 - Anub''ar Necromancer'
WHERE NOT EXISTS (SELECT 1 FROM creature_summon_groups WHERE summonerId = 28921 AND summonerType = 0 AND groupId = 3 AND entry = 29119);
