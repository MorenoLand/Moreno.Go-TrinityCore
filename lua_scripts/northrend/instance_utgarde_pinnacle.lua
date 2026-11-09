-- The Utgarde Pinnacle instance script — Lua port of
-- src/server/scripts/Northrend/UtgardeKeep/UtgardePinnacle/instance_utgarde_pinnacle.cpp
-- (class instance_utgarde_pinnacle : public InstanceMapScript, map 575).
-- RegisterMapEvent binds the map-level Eluna instance hooks (the
-- instance_naxxramas.lua / instance_halls_of_lightning.lua precedent);
-- its presence opens the Map:GetInstanceData gate for map 575
-- (engine/world/instance_hooks.go) that the boss onDied handlers
-- share with this file.
-- Shared instanceData keys (one table per (map, instance)):
-- svala / palehoof / skadi / ymiron — the four boss binds (the C++
-- OnCreatureCreate GUID latches via LoadObjectData creatureData,
-- instance_utgarde_pinnacle.cpp:36-50, :72); frenziedWorgen /
-- ravenousFurbolg / massiveJormungar / ferociousRhino / palehoofOrb /
-- svalaAdd / grauf — the remaining LoadObjectData creature entries
-- (same table), bound for the same mirror; no Lua file reads them
-- today — the boss files re-express every cross-creature lookup via
-- FindNearestCreature (boss_skadi.lua Grauf, boss_palehoof.lua
-- miniboss/orb) — but the binds are the C++-exact object-data store.
-- Ported arms (C++-exact unless noted):
-- - OnCreatureCreate: the eleven binds above. Fires on materialization
--   (motionFor funnel, instance_hooks.go:311-320); a static template
--   spawn never materializes server-side and is never bound (naxxramas
--   LoadGrid delta).
-- - GetGuidData (instance_utgarde_pinnacle.cpp:93-99): the binds
--   above are write-only (tick 519 correction: zero instanceData /
--   GetInstanceData references exist in the four boss files) — the
--   C++ consumers ride unported cross-creature machines in those
--   boss files — the Skadi Grauf lookup (boss_skadi.cpp:171/:249),
--   the Svala mirror (boss_svala.cpp:196/:339) and sacrificed-player
--   GUID (boss_svala.cpp:442/:495), the Palehoof sphere
--   (boss_palehoof.cpp:207) and self-lookup from the miniboss AIs
--   (boss_palehoof.cpp:355/:361/:571) — so the GUID legs as consumed
--   stay out there, re-expressed via FindNearestCreature /
--   FindNearestGameObject where the boss files port them.
-- Unmodeled (documented-only, no bridges):
-- - Door data (GO_SKADI_THE_RUTHLESS_DOOR 192173 /
--   GO_KING_YMIRON_DOOR 192174, DOOR_TYPE_PASSAGE tied to
--   DATA_SKADI_THE_RUTHLESS / DATA_KING_YMIRON,
--   instance_utgarde_pinnacle.cpp:29-34): no boss-state door machine
--   in Go (oculus/naxxramas precedent).
-- - Boss boundaries (DATA_KING_YMIRON RectangleBoundary 340/443 /
--   -412/-275, instance_utgarde_pinnacle.cpp:24-27): no boundary
--   bridge; LoadBossBoundaries (:70) stays out with it.
-- - OnGameObjectCreate sphere latch (instance_utgarde_pinnacle.cpp:
--   76-83): static template GOs never pass the Go GO-create funnel
--   (instance_hooks.go:291-306, oculus precedent), so the latch is
--   vacuous here — and the leg itself is already ported at DONE time
--   in boss_palehoof.lua (palehoofDied, FindNearestGameObject
--   GO_GORTOK_PALEHOOF_SPHERE 188593 + SetFlag NOT_SELECTABLE), with
--   the Reset re-arm in the same file.
-- - OnGameObjectCreate gameObjectData latches (GO_UTGARDE_MIRROR
--   191745 / GO_GORTOK_PALEHOOF_SPHERE 188593,
--   instance_utgarde_pinnacle.cpp:52-56): same static-GO funnel
--   delta as above; the mirror is resolved via FindNearestGameObject
--   in boss_svala.lua.
-- - SetGuidData DATA_SACRIFICED_PLAYER
--   (instance_utgarde_pinnacle.cpp:85-91, :102): the instance-side
--   GUID store exists and is bridged (SetGuidData/GetGuidData/
--   GetInstanceCreature, lua_creature_events.go:6448-6490) but has no
--   bearer — SetGuidData latches only the caller's GUID and players
--   expose no SetGuidData, so Svala cannot latch the sacrifice
--   target's player GUID; the sacrificed latch lives in
--   boss_svala.lua's per-GUID state table (boss_svala.cpp:170/:362
--   writes, :442/:495 reads).
-- - SetHeaders(DataHeader "UP") / SetBossNumber(EncounterCount 4) /
--   LoadDoorData / LoadObjectData: no bridges; the boss-state store is
--   written via creature:SetBossState (lua_creature_events.go:5908) and
--   read via creature:GetBossState (lua_creature_events.go:5932).
-- - SaveToDB / instance-data persistence across restarts: the table
--   lives for the runtime lifetime only (black-morass delta).
-- No ON_UPDATE poll: unlike the Loken globe in
-- instance_halls_of_lightning.lua, this instance script has no
-- SetBossState override and no DONE-gated leg of its own — the only
-- DONE-time leg (the Palehoof sphere flag) already runs at DONE time
-- in boss_palehoof.lua, and the remaining boss-state writes/reads
-- (Skadi IN_PROGRESS, boss_skadi.cpp:232/:763/:965) live in the boss
-- files — so there is nothing for a poll to drive.

local MAP_UTGARDE_PINNACLE = 575
local INSTANCE_EVENT_ON_CREATURE_CREATE = 5

local NPC_SVALA_SORROWGRAVE = 26668 -- utgarde_pinnacle.h:39
local NPC_GORTOK_PALEHOOF = 26687 -- utgarde_pinnacle.h:40
local NPC_SKADI_THE_RUTHLESS = 26693 -- utgarde_pinnacle.h:41
local NPC_KING_YMIRON = 26861 -- utgarde_pinnacle.h:42
local NPC_FRENZIED_WORGEN = 26683 -- utgarde_pinnacle.h:62
local NPC_RAVENOUS_FURBOLG = 26684 -- utgarde_pinnacle.h:63
local NPC_MASSIVE_JORMUNGAR = 26685 -- utgarde_pinnacle.h:64
local NPC_FEROCIOUS_RHINO = 26686 -- utgarde_pinnacle.h:65
local NPC_PALEHOOF_ORB = 22515 -- utgarde_pinnacle.h:66
local NPC_SVALA = 29281 -- utgarde_pinnacle.h:48
local NPC_GRAUF = 26893 -- utgarde_pinnacle.h:70

local bossBinds = {
    [NPC_SVALA_SORROWGRAVE] = "svala",
    [NPC_GORTOK_PALEHOOF] = "palehoof",
    [NPC_SKADI_THE_RUTHLESS] = "skadi",
    [NPC_KING_YMIRON] = "ymiron",
    [NPC_FRENZIED_WORGEN] = "frenziedWorgen",
    [NPC_RAVENOUS_FURBOLG] = "ravenousFurbolg",
    [NPC_MASSIVE_JORMUNGAR] = "massiveJormungar",
    [NPC_FEROCIOUS_RHINO] = "ferociousRhino",
    [NPC_PALEHOOF_ORB] = "palehoofOrb",
    [NPC_SVALA] = "svalaAdd",
    [NPC_GRAUF] = "grauf",
}

RegisterMapEvent(MAP_UTGARDE_PINNACLE, INSTANCE_EVENT_ON_CREATURE_CREATE, function(event, instanceData, mapObj, creature)
    local key = bossBinds[creature:GetEntry()]
    if key then
        instanceData[key] = creature
    end
end)
