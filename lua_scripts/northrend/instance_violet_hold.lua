-- The Violet Hold instance script — Lua port of
-- src/server/scripts/Northrend/VioletHold/instance_violet_hold.cpp
-- (class instance_violet_hold : public InstanceMapScript, map 608).
-- RegisterMapEvent binds the map-level Eluna instance hooks (the
-- instance_vault_of_archavon.lua / instance_utgarde_keep.lua
-- precedent); its presence opens the Map:GetInstanceData gate for map
-- 608 (engine/world/instance_hooks.go) that the boss files share with
-- this file.
-- Shared instanceData keys (one table per (map, instance)):
-- xevozz / lavanthor / ichoron / zuramat / erekem / moragg /
-- cyanigosa / sinclari / sinclariTrigger — the nine creatureData
-- binds (instance_violet_hold.cpp:196-208, LoadObjectData).
-- defenseless — 1/0 number (instance_violet_hold.cpp:474-475
-- DATA_DEFENSELESS 14, C++ ctor default true): cleared to 0 by the
-- activation-crystal gossip handler
-- (npc_violet_hold_activation_crystal.lua, tick 417); the
-- door-integrity clear (Destroy Door Seal 58040 AuraScript,
-- violet_hold.cpp:1385) and the wipe re-arm (instance :893) stay out
-- (no AuraScript / instance-Update bridges).
-- Ported arms (C++-exact unless noted):
-- - OnCreatureCreate: the nine boss/Sinclari binds above. Fires on
--   materialization (motionFor funnel, instance_hooks.go:311-320); a
--   static template spawn never materializes server-side and is never
--   bound (naxxramas LoadGrid delta).
-- - GetGuidData (instance_violet_hold.cpp: no LoadObjectData override
--   beyond the base store): the Lua mirror is the instanceData binds
--   above, read as instanceData.<boss> (oculus precedent); the C++
--   consumers ride unported cross-creature machines in the boss files
--   — the Erekem guard legs (boss_erekem.cpp:137-166/:226-241) and the
--   Sinclari-trigger Talks (violet_hold.cpp portal AIs) — so the GUID
--   legs as consumed stay out there.
-- - SetBossState override DONE legs
--   (instance_violet_hold.cpp:328-357): NOT ported at DONE time in
--   the boss onDied handlers, and there is NO ON_UPDATE poll (the
--   palehoof/oculus precedent inverted — a poll only drives legs with
--   a live consumer, and every override leg's consumer machine is
--   itself unported, so there is nothing for a poll to drive):
--   - DATA_1ST_BOSS (0) / DATA_2ND_BOSS (1) DONE ->
--     UpdateEncounterStateForKilledCreature(NPC_EREKEM / NPC_MORAGG):
--     no Go bridge (whole-tree grep: zero hits) — stays out.
--   - DATA_CYANIGOSA (2) DONE -> SetData(DATA_MAIN_EVENT_STATE,
--     DONE): no Lua twin for the main-event state machine (Sinclari
--     intro/outro, AddWave, CheckEventState all unported — see
--     violet_hold.lua), and its effects (main-door GO state, world
--     states, crystal NOT_SELECTABLE flags, Sinclari outro DoAction)
--     have no bridges.
--   - DATA_MORAGG..DATA_ZURAMAT (3..8): WaveCount == 6 ->
--     SetBossState(DATA_1ST_BOSS, state), WaveCount == 12 ->
--     SetBossState(DATA_2ND_BOSS, state), and DONE ->
--     SetData(DATA_WAVE_COUNT, WaveCount + 1): no Lua twin for
--     DATA_WAVE_COUNT — the wave machine (AddWave, the portal
--     SummonedCreatureDies increments, SetData DATA_WAVE_COUNT) is
--     unported and instance GetData/SetData has no bridge — so the
--     mirror condition is unresolvable and slots 0/1 stay unwritten.
--     (C++ itself notes the alias leg "won't work correctly because
--     bossstate was initializd with TO_BE_DECIDED".)
-- Unmodeled (documented-only, no bridges):
-- - OnCreatureCreate Erekem-guard slot-fill
--   (instance_violet_hold.cpp:234-247, DATA_EREKEM_GUARD_1/2): the
--   GetGuidData override's only consumers are the unported Erekem
--   guard legs, so the slots would be write-only (the zuramat
--   _voidDance precedent) — not bound.
-- - OnGameObjectCreate GO_ACTIVATION_CRYSTAL slot-fill (:266-283):
--   static GOs never pass the Go funnel (the halls-of-lightning
--   globe-latch precedent).
-- - DoorData (gameObjectData :211-222, 9 cell/main doors): no door
--   machine; the HandleCells legs ride the unported
--   JustReachedHome SetData(DATA_HANDLE_CELLS) arms in the boss
--   files.
-- - MinionData (:225-229, NPC_EREKEM_GUARD -> DATA_EREKEM): no
--   minion registry in Go.
-- - CheckRequiredBosses (:313-336, wave-6/12 + First/SecondBossId
--   gates, cyanigosa wave-18 gate): no bridge (the koralon header
--   precedent); FirstBossId/SecondBossId are picked by the unported
--   AddWave machine.
-- - FillInitialWorldStates (:299-305, WORLD_STATE_VH_SHOW 3816 /
--   PRISON_STATE 3815 / WAVE_COUNT 3810): no world-state packet
--   bridge from Lua.
-- - Update (:841-860, no-players -> FAIL + CheckEventState, door
--   integrity 0 -> FAIL) / CheckEventState (:862-881,
--   ResetBossEncounter + SetData MAIN_EVENT_STATE NOT_STARTED) /
--   ResetBossEncounter (:695-739, respawn + UpdateEntry dummy swap +
--   MoveTargetedHome + SetImmuneToAll): no MoveTargetedHome /
--   UpdateEntry / instance SetData bridges.
-- - StartBossEncounter (:625-693) / AddWave (:741-781) /
--   ScheduleCyanigosaIntro (:883-905) / SpawnPortal (:560-578):
--   MoveSmoothPath / MoveJump / SummonCreature-via-Sinclari /
--   instance SetData legs have no bridges.
-- - ProcessEvent (:907-916, EVENT_ACTIVATE_CRYSTAL 20001 -> summon
--   defense system): no instance-script bridge (the activation
--   crystal header documents the same gap). The Defenseless=false leg
--   rides the crystal gossip handler instead (tick 417); only the
--   defense-system summon stays out.
-- - SetHeaders(DataHeader "VH") / SetBossNumber(EncounterCount 9) /
--   LoadObjectData / LoadMinionData: no bridges; the boss-state store
--   is written via creature:SetBossState
--   (lua_creature_events.go:5917) and read via creature:GetBossState
--   (lua_creature_events.go:5936).
-- - SaveToDB / WriteSaveDataMore / ReadSaveDataMore (FirstBossId /
--   SecondBossId): the table lives for the runtime lifetime only
--   (black-morass delta).
-- No ON_UPDATE poll: see the SetBossState-override note above.

local MAP_VIOLET_HOLD = 608
local INSTANCE_EVENT_ON_CREATURE_CREATE = 5

local NPC_XEVOZZ = 29266 -- violet_hold.h:96
local NPC_LAVANTHOR = 29312 -- violet_hold.h:97
local NPC_ICHORON = 29313 -- violet_hold.h:98
local NPC_ZURAMAT = 29314 -- violet_hold.h:101
local NPC_EREKEM = 29315 -- violet_hold.h:104
local NPC_MORAGG = 29316 -- violet_hold.h:106
local NPC_CYANIGOSA = 31134 -- violet_hold.h:116
local NPC_SINCLARI = 30658 -- violet_hold.h:118
local NPC_SINCLARI_TRIGGER = 32204 -- violet_hold.h:119

local bossBinds = {
    [NPC_XEVOZZ] = "xevozz",
    [NPC_LAVANTHOR] = "lavanthor",
    [NPC_ICHORON] = "ichoron",
    [NPC_ZURAMAT] = "zuramat",
    [NPC_EREKEM] = "erekem",
    [NPC_MORAGG] = "moragg",
    [NPC_CYANIGOSA] = "cyanigosa",
    [NPC_SINCLARI] = "sinclari",
    [NPC_SINCLARI_TRIGGER] = "sinclariTrigger",
}

RegisterMapEvent(MAP_VIOLET_HOLD, INSTANCE_EVENT_ON_CREATURE_CREATE, function(event, instanceData, mapObj, creature)
    local key = bossBinds[creature:GetEntry()]
    if key then
        instanceData[key] = creature
    end
end)
