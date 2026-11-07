-- Razorfen Downs instance script — Lua port of the bridgeable arm of
-- src/server/scripts/Kalimdor/RazorfenDowns/instance_razorfen_downs.cpp
-- (class instance_razorfen_downs : public InstanceMapScript
-- (RFDScriptName "instance_razorfen_downs", map 129); AddSC at end
-- registers the one script; kalimdor loader call follows
-- AddSC_instance_onyxias_lair(), opening the "// Razorfen Downs"
-- loader block). DataHeader "RFD", EncounterCount = 5
-- (razorfen_downs.h). RegisterMapEvent binds the map-level Eluna
-- instance hooks (the instance_mount_hyjal.lua precedent); its
-- presence opens the Map:GetInstanceData gate for map 129 that the
-- boss files share with this file.
-- Shared instanceData keys (one table per (map, instance)):
-- gong — the GO object bound by ON_GAMEOBJECT_CREATE (the C++
-- OnGameObjectCreate GO_GONG latch, :72-76); the live part for
-- future consumers, the flag leg below stays out. Also stashed by
-- the gong gossip handler (npc_belnistrasz.lua) — the static gong
-- never fires the create hook, so the gossip stash is what makes
-- instanceData.gong live for the tomb-death re-arm.
-- gongWave / fiendsKilled / reaversKilled / summonLowRange /
-- summonHighRange / summonCreature — the DATA_WAVE machine counters
-- (npc_belnistrasz.lua gossip leg, npc_tomb_creature.lua death leg).
-- Ported arms (C++-exact unless noted):
-- - OnGameObjectCreate (:68-86), gong bind only: GO_GONG (148917)
--   binds instanceData.gong. Static-spawn delta: the Go GO-create
--   funnel only fires for dynamic/instance spawns
--   (instance_hooks.go, triggerInstanceGameObjectCreate) — the
--   static template gong never fires this hook, so the bind is live
--   only for a dynamically-spawned gong (same documented delta as
--   the hyjal gate binds).
-- Unmodeled (documented-only, no bridges):
-- - OnGameObjectCreate gong flag leg (:74-75): if
--   GetBossState(DATA_TUTEN_KASH) == DONE the gong gets
--   GO_FLAG_NOT_SELECTABLE (SharedDefines.h:1641, 0x10) — stays out
--   twice over: the GO object has no GetBossState reader
--   (creature:GetBossState is creature-only,
--   lua_creature_events.go:5936; instanceData is a plain table), and
--   the static gong never fires the hook. The boss file writes the
--   state the gate would read (boss_tuten_kash.lua writes
--   SetBossState(DATA_TUTEN_KASH, DONE) on death), so the leg is
--   re-portable the moment a GO-side boss-state reader lands.
-- - OnGameObjectCreate idol-fire legs (:77-83): GO_IDOL_OVEN_FIRE
--   (151951) / GO_IDOL_CUP_FIRE (151952) / GO_IDOL_MOUTH_FIRE
--   (151973) are Delete()d once GetBossState(DATA_EXTINGUISHING_THE_
--   IDOL) == DONE — stays out three times over: no GO Delete method
--   (only Despawn/Respawn, lua_objects.go:4520), no GO-side boss-
--   state reader (above), and the static fires never fire the hook.
--   The gate can never be DONE in C++ anyway: DATA_EXTINGUISHING_
--   THE_IDOL = 6 is out of range for SetBossState (EncounterCount =
--   5, bosses.size() 5 — see below), so no write ever lands it.
-- - SetBossState override (:88-107): calls the base, then a switch
--   over all seven DATA ids whose every arm is an empty break — a
--   pure no-op switch beyond the base call, which the boss files
--   already drive via creature:SetBossState. Nothing to port.
-- - SetData (:109-190), the gong wave machine: PORTED — the DATA_WAVE
--   IN_PROGRESS leg lives in the go_gong gossip handler
--   (npc_belnistrasz.lua: summons at PosSummonTutenkash + MovePoint,
--   gong flag + gongWave counter) and the tomb-creature death leg
--   lives in npc_tomb_creature.lua (fiends/reavers kill counters +
--   gong re-arm), both via the shared instanceData table (no
--   instance SetData hook in Go; the table is the writer+reader).
-- - This C++ file has NO OnCreatureCreate and NO GetGuidData
--   override — there are no creature binds or GUID mirrors to port.
-- - DATA_GONG (4) is in range for SetBossState but nothing ever
--   writes it; DATA_WAVE (5) and DATA_EXTINGUISHING_THE_IDOL (6)
--   are OUT of range (EncounterCount = 5, InstanceScript::
--   SetBossState returns false for id >= bosses.size(),
--   InstanceScript.cpp) — their state lives in SetData, closed
--   axis above. Do NOT write SetBossState(5/6, ...) from Lua: Go's
--   setInstanceBossState takes any boss id
--   (instance_boss_states.go), so such a write would fabricate
--   state C++ never has. The belnistrasz JustDied
--   SetBossState(6, DONE) call in razorfen_downs.cpp:125-128 is
--   dropped by C++ for exactly this reason (documented in
--   npc_belnistrasz.lua, not ported).
-- - The boss-state writes themselves live in the boss files:
--   boss_tuten_kash.lua / boss_mordresh_fire_eye.lua /
--   boss_glutton.lua / boss_amnennar_the_coldbringer.lua write
--   SetBossState(DATA_*, IN_PROGRESS / NOT_STARTED / DONE) via
--   creature:SetBossState.

local MAP_RAZORFEN_DOWNS = 129
local INSTANCE_EVENT_ON_GAMEOBJECT_CREATE = 6

local GO_GONG = 148917 -- razorfen_downs.h:50

RegisterMapEvent(MAP_RAZORFEN_DOWNS, INSTANCE_EVENT_ON_GAMEOBJECT_CREATE, function(event, instanceData, mapObj, go)
    if go:GetEntry() == GO_GONG then
        instanceData.gong = go
    end
end)
