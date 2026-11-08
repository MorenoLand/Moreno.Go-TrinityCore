-- Old Hillsbrad instance script — Lua port of
-- src/server/scripts/Kalimdor/CavernsOfTime/EscapeFromDurnholdeKeep/
-- instance_old_hillsbrad.cpp (243 lines; class instance_old_hillsbrad :
-- public InstanceMapScript, map 560 — instance_old_hillsbrad.cpp:44;
-- AddSC_instance_old_hillsbrad at end registers the one script;
-- kalimdor loader decl 38 / call 151 per kalimdor_script_loader.cpp).
-- Whole-server-tree quoted-name grep confirms the cpp as the sole source
-- of "instance_old_hillsbrad" (loader lines only otherwise).
-- RegisterMapEvent binds the map-level Eluna instance hooks (the
-- instance_violet_hold.lua / instance_mount_hyjal.lua precedent); its
-- presence opens the Map:GetInstanceData gate for map 560
-- (engine/world/instance_hooks.go) that the Old Hillsbrad files share
-- with this file.
-- Shared instanceData keys (one table per (map, instance)):
-- thrall / taretha / epoch — the creature objects bound by
-- OnCreatureCreate (the C++ OnCreatureCreate GUID latches,
-- instance_old_hillsbrad.cpp:108-121); the GetGuidData legs
-- (:223-235, DATA_THRALL 7 / DATA_TARETHA 8 / DATA_EPOCH 9,
-- old_hillsbrad.h:42-44) are mirrored by these binds, read as
-- instanceData.<key> (oculus precedent).
-- Ported arms (C++-exact unless noted):
-- - OnCreatureCreate: the binds above. Fires on materialization
--   (motionFor funnel, instance_hooks.go:311-320); a static template
--   spawn never materializes server-side and is never bound (naxxramas
--   / ruby-sanctum delta).
-- Unmodeled (documented-only, no bridges):
-- - SetData (:127-219): no instance SetData bridge — creature:SetData
--   is a generic per-motion ScriptData store, NOT virtual AI dispatch
--   and NOT the instance encounter array (lua_creature_events.go:767-795;
--   the ruby-sanctum header settles the same point). No fire site exists
--   for any SetData arm, so none can be re-expressed here and no
--   ON_UPDATE poll is added (the instance_icecrown_citadel.lua pattern):
--   - TYPE_BARREL_DIVERSION (:133-157): PORTED 2026-11-07 — the
--     5-barrel counter + DoUpdateWorldState(WORLD_STATE_OH 2436) +
--     UpdateQuestCredit (KilledMonsterCredit(LODGE_QUEST_TRIGGER 20155)
--     to every player in map) + SummonCreature(DRAKE_ENTRY 17848) at
--     count 5 lives in the go_barrel GO gossip handler
--     (boss_lieutenant_drake.lua, GO entry 182589), against the shared
--     instanceData table (barrelCount / barrelDiversion) — the
--     tick-395 writer+reader-both-Lua precedent retires the "no
--     instance SetData bridge" stay-out for this leg.
--   - TYPE_THRALL_EVENT (:158-184): the FAIL leg resets all five
--     sub-events to NOT_STARTED (fail-count-capped at 20, then FAIL);
--     the senders are Thrall's JustDied / gossip legs in
--     old_hillsbrad.cpp, which ride the unported escort machine.
--   - TYPE_THRALL_PART1..4 (:185-205): plain encounter-array writes
--     from the Thrall escort WaypointReached / gossip machine.
-- - GetData (:207-221): no instance GetData bridge; the Lua mirror is
--   the instanceData binds above (GUIDs only, plus the barrelDiversion /
--   barrelCount slots written by the go_barrel handler — see SetData
--   above). The readers are
--   the boss files (boss_captain_skarloc.cpp:108,
--   boss_epoch_hunter.cpp:100, boss_leutenant_drake.cpp:50) and the
--   Thrall/Taretha gossip gates in old_hillsbrad.cpp — all documented
--   in their own files, not re-litigated here (GetData/SetData consumer
--   legs are a CLOSED axis except the barrel leg, ported 2026-11-07).
-- - The GetGuidData consumers in old_hillsbrad.cpp are all inside the
--   unported escort/gossip machine (Thrall WaypointReached :332/:354
--   Taretha Talk; Taretha gossip action 1002 :614/:617 Epoch-empty
--   gate + Thrall StartWP) — the binds above are the live half, for a
--   future consumer.
-- - SetHeaders(DataHeader "OH") / MAX_ENCOUNTER 6: no bridges.
-- - No SetBossState override exists in this C++ file; the encounter
--   array is m_auiEncounter, written only via SetData above.
-- No ON_UPDATE poll: see the SetData note above.

local MAP_OLD_HILLSBRAD = 560
local INSTANCE_EVENT_ON_CREATURE_CREATE = 5

local NPC_THRALL = 17876 -- old_hillsbrad.h:35
local NPC_TARETHA = 18887 -- instance_old_hillsbrad.cpp:36
local NPC_EPOCH = 18096 -- instance_old_hillsbrad.cpp:37

local creatureBinds = {
    [NPC_THRALL] = "thrall",
    [NPC_TARETHA] = "taretha",
    [NPC_EPOCH] = "epoch",
}

RegisterMapEvent(MAP_OLD_HILLSBRAD, INSTANCE_EVENT_ON_CREATURE_CREATE, function(event, instanceData, mapObj, creature)
    local key = creatureBinds[creature:GetEntry()]
    if key then
        instanceData[key] = creature
    end
end)
