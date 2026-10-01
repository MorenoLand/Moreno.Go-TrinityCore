-- npc_unworthy_initiate (The Scarlet Enclave, Acherus DK intro, Quest 12848) --
-- Lua port of the npc_unworthy_initiate class in
-- src/server/scripts/EasternKingdoms/ScarletEnclave/chapter1.cpp
-- (class npc_unworthy_initiate line 98; AI struct npc_unworthy_
-- initiateAI line 103 : ScriptedAI). AddSC_the_scarlet_enclave_c1
-- registers it (line 1111; fwd-decl/call in eastern_kingdoms_
-- script_loader.cpp lines 95/273 belong to the parent file's
-- loader call). Whole-server-tree grep confirms chapter1.cpp +
-- its loader reference as the only sources of "npc_unworthy_
-- initiate" (the anchor class npc_unworthy_initiate_anchor line
-- 298, the go_acherus_soul_prison gameobject script line 325, and
-- spell_death_knight_initiate_visual line 349 are separate scripts
-- in the same file, not this class).
-- Entries (verifiable from the C++ sources): the file's own
-- acherus_unworthy_initiate[5] array (lines 89-96) names the five
-- initiate entries 29519, 29520, 29565, 29566, 29567. The creature_
-- template ScriptName binding is DB-side (no TDB in this workspace),
-- but the entries themselves are C++-verifiable so the port is
-- registered for all five (boss_felblood_kaelthas precedent).
-- Script name is the stringified class name (CreatureScript(
-- "npc_unworthy_initiate"); RegisterCreatureAIWithFactory
-- convention, felblood precedent).
-- Verifiable numbers (file's own UnworthyInitiate enum, lines
-- 43-58): SPELL_ICY_TOUCH = 52372 / SPELL_PLAGUE_STRIKE = 52373 /
-- SPELL_BLOOD_STRIKE = 52374 / SPELL_DEATH_COIL = 52375 /
-- SPELL_SOUL_PRISON_CHAIN = 54612 / SPELL_DK_INITIATE_VISUAL =
-- 51519 / SAY_EVENT_START = 0 / SAY_EVENT_ATTACK = 1 /
-- EVENT_ICY_TOUCH = 1 / EVENT_PLAGUE_STRIKE = 2 / EVENT_BLOOD_
-- STRIKE = 3 / EVENT_DEATH_COIL = 4; anchor entry 29521 and the
-- twelve soul-prison gameobject entries 191577/191580-191590
-- (lines 64-77) belong to the unmodeled phase-machine arms.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Driven by CreateLuaEvent timers; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++ PHASE_ATTACKING machine; the four events are
-- scheduled once from JustEngagedWith with the inits below and
-- re-arm at 5s):
-- Icy-touch machine: DoCastVictim(SPELL_ICY_TOUCH 52372) 1000ms
-- init -> 5000ms loop via GetVictim nil-guarded (incarcerator
-- convention).
-- Plague-strike machine: DoCastVictim(SPELL_PLAGUE_STRIKE 52373)
-- 3000ms init -> 5000ms loop, same conventions.
-- Blood-strike machine: DoCastVictim(SPELL_BLOOD_STRIKE 52374)
-- 2000ms init -> 5000ms loop, same conventions.
-- Death-coil machine: DoCastVictim(SPELL_DEATH_COIL 52375) 5000ms
-- init -> 5000ms loop, same conventions.
-- Timers start on OnEnterCombat(1), cancelled on OnLeaveCombat(2)/
-- OnDied(4)/OnReset(23) (maiden convention).
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (casts are
-- packet-visual), so the timers fire unconditionally and the in-loop
-- casting gates are dropped (maiden precedent). Each C++ event arm
-- also calls events.DelayEvents(1s, GCD_CAST) — a 1s global-cooldown
-- cross-delay — which has no Go model and is dropped with the
-- casting gates. The C++ scheduler is driven from UpdateAI after
-- UpdateVictim; the port schedules from OnEnterCombat(1) and cancels
-- on 2/4/23 (maiden convention).
-- Reset()/Initialize()/::UpdateAI() machinery is internal to the
-- unmodeled phase machine and covered by the cancel/re-arm on combat
-- events (halycon precedent).
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - The whole pre-combat phase state machine: PHASE_CHAINED's
--   FindNearestCreature(29521, 30) anchor acquisition + anchor
--   SetGUID/GetGUID GUID-passing (SetGUID/GetGUID and
--   FindNearestCreature have no bridges) + the anchor's triggered
--   CastSpell(SOUL_PRISON_CHAIN 54612) on the prisoner (no
--   cross-creature cast bridge) + the 12-entry soul-prison
--   gameobject ResetDoorOrButton ring (no gameobject bridges);
--   PHASE_TO_EQUIP's EventStart trigger chain (fires only from
--   go_acherus_soul_prison's OnGossipHello via the anchor — no
--   gameobject-gossip bridge; razorgore precedent) with its
--   SetStandState(UNIT_STAND_STATE_STAND) (no stand-state bridge),
--   RemoveAurasDueToSpell(54612) (no bridge), and Talk(SAY_EVENT_
--   START 0) (Talk exists on the surface but its only trigger is
--   the unbridged event start); the 5s wait -> MovePoint(1, ...)
--   (no MotionMaster bridge); MovementInform(POINT_ID 1)'s
--   LoadEquipment(1) (no bridge), triggered CastSpell(DK_INITIATE_
--   VISUAL 51519) + Talk(SAY_EVENT_ATTACK 1) (stranded on the
--   MovementInform leg), and PHASE_TO_ATTACK's 5s wait ->
--   SetFaction(FACTION_MONSTER) / SetImmuneToPC(false) /
--   SetReactState(REACT_AGGRESSIVE) (no faction/immune/react-state
--   bridges) + AttackStart(player) (no bridge).
-- - npc_unworthy_initiate_anchor (PassiveAI SetGUID/GetGUID
--   prisoner-GUID holder) is ported nowhere: a GUID store with no
--   bridgeable behavior (never mark stubs as completed).
-- - go_acherus_soul_prison (gameobject gossip -> anchor -> prisoner
--   EventStart): no gameobject-gossip bridge.
-- - spell_death_knight_initiate_visual's displayId -> 51552/
--   equipment-dressup leg: no bridges (SpellScript model).
local SPELL_ICY_TOUCH = 52372
local SPELL_PLAGUE_STRIKE = 52373
local SPELL_BLOOD_STRIKE = 52374
local SPELL_DEATH_COIL = 52375

local ENTRIES = { 29519, 29520, 29565, 29566, 29567 }

local timers = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

local function schedule(guid, key, delay, fn)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

local function onIcyTouch(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_ICY_TOUCH)
    end
    schedule(guid, "icytouch", 5000, function() onIcyTouch(creature, guid) end)
end

local function onPlagueStrike(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_PLAGUE_STRIKE)
    end
    schedule(guid, "plaguestrike", 5000, function() onPlagueStrike(creature, guid) end)
end

local function onBloodStrike(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_BLOOD_STRIKE)
    end
    schedule(guid, "bloodstrike", 5000, function() onBloodStrike(creature, guid) end)
end

local function onDeathCoil(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_DEATH_COIL)
    end
    schedule(guid, "deathcoil", 5000, function() onDeathCoil(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "icytouch", 1000, function() onIcyTouch(creature, guid) end)
    schedule(guid, "plaguestrike", 3000, function() onPlagueStrike(creature, guid) end)
    schedule(guid, "bloodstrike", 2000, function() onBloodStrike(creature, guid) end)
    schedule(guid, "deathcoil", 5000, function() onDeathCoil(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

local function onDied(_, creature)
    cancelTimers(creature:GetGUID())
end

for _, entry in ipairs(ENTRIES) do
    RegisterCreatureEvent(entry, 1, onEnterCombat)
    RegisterCreatureEvent(entry, 2, onCombatEnd)
    RegisterCreatureEvent(entry, 4, onDied)
    RegisterCreatureEvent(entry, 23, onCombatEnd)
end
