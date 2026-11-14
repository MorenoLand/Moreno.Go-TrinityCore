-- Maghar / Kurenai captive escort path binding (Nagrand, quests 9868/9879).
-- TDB 335.26091 moved the C++ script_waypoint paths into waypoint_data
-- under synthetic ids -- 145674 (kurenai, pts 1-10, pt7 delay 7500) /
-- 145682 (maghar, pts 0-19, pt7 delay 2000, pt16 delay 7500, pts 16-19
-- run) -- with no binding row: creature_template_addon.path_id is 0
-- for both entries and no creature_addon rows exist for the spawn GUIDs
-- 65108/84459, so loadCreaturePathID (creaturemotion.go) falls back to
-- the spawn GUID, loadWaypoints finds no rows, and MoveWaypoint() is a
-- no-op -- the lua_scripts/outland/npc_nagrand_captive.lua ev6 arms
-- (ambush summons, quest credit) can never fire. These UPDATEs bind the
-- template rows to the orphaned paths; the existing loader then walks
-- the escort with zero engine changes. The AND path_id = 0 guard keeps
-- the migration a no-op on a deployment DB that already binds these
-- entries (e.g. a future full TDB import that sets path_id). Other
-- creature_template_addon columns are untouched (notably StandState 8:
-- the captives kneel until OnQuestAccept stands them). Precondition: the
-- rows exist (any full TDB import carries them); a deployment DB missing
-- the rows needs them from the import, the UPDATE then affects 0 rows.
UPDATE creature_template_addon SET path_id = 145674 WHERE entry = 18209 AND path_id = 0;
UPDATE creature_template_addon SET path_id = 145682 WHERE entry = 18210 AND path_id = 0;
