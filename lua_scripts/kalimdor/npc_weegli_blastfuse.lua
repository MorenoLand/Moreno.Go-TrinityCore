-- Zul'Farrak: Weegli Blastfuse — Lua port of
-- src/server/scripts/Kalimdor/ZulFarrak/zulfarrak.cpp (class
-- npc_weegli_blastfuse : CreatureScript("npc_weegli_blastfuse") {
-- npc_weegli_blastfuseAI : public ScriptedAI }; GetAI via
-- GetZulFarrakAI<npc_weegli_blastfuseAI> (zulfarrak.h — GetInstanceAI<AI>(obj,
-- "instance_zulfarrak") gate, ZFScriptName "instance_zulfarrak",
-- DataHeader "ZF"); AddSC_zulfarrak at end registers it alongside
-- npc_sergeant_bly, go_shallow_grave, at_zumrah, go_troll_cage;
-- kalimdor loader decl 103 / call 216 per kalimdor_script_loader.cpp —
-- the SECOND "// Zul'Farrak" loader group, right after
-- AddSC_boss_zum_rah()). Whole-server-tree "npc_weegli_blastfuse" grep
-- hits only the ZulFarrak dir files + loader (sole-source verified);
-- zero sql/ hits.
-- Entry: zulfarrak.h ZFEntries names ENTRY_WEEGLI = 7607 (alongside the
-- pyramid ENTRY_BLY/RAVEN/ORO/MURTA entries) — kalecgos check PASSES;
-- creature_template ScriptName binding stays DB-side. Eluna creature
-- events: 1 OnEnterCombat, 2 OnLeaveCombat, 23 OnReset. Timers via
-- CreateLuaEvent; melee is engine-driven in Go (creature combat tick),
-- like C++ DoMeleeAttackIfReady. No C++ HasUnitState(UNIT_STATE_CASTING)
-- gates in this AI.
-- Ported arms (combat only):
-- - Bomb 8858: DoCastVictim non-triggered via creature:GetVictim()
--   (boss_onyxia precedent), init 10s, reschedule 10s unconditional
--   (C++ Repeat outside any gate — faithful).
-- Unmodeled (documented-only, no bridges):
-- - AttackStart: AttackStartCaster(victim, 10) (keep back & toss
--   bombs/shoot) — no AttackStart bridge.
-- - Shoot 6660 + SetSheath(SHEATH_STATE_RANGED/MELEE): the
--   isAttackReady && !IsWithinMeleeRange(victim) decision gate has no
--   bridge (magtheridon precedent); SetSheath has no bridge.
-- - MovementInform: PYRAMID_CAGES_OPEN -> SetData(EVENT_PYRAMID,
--   PYRAMID_ARRIVED_AT_STAIR) + Talk(SAY_WEEGLI_OHNO=0) +
--   SetHomePosition; destroyingDoor -> DoUseDoorOrButton(GO_END_DOOR via
--   GetGuidData) + DespawnOrUnsummon — no instance SetData/GetData /
--   MovementInform-instance bridges.
-- - DestroyDoor (door-bomb run driven by Bly's DoAction(0) via gossip):
--   SetFaction(FACTION_FRIENDLY) + MovePoint(0, 1858.57, 1146.35, 14.745)
--   + Talk(SAY_WEEGLI_OK_I_GO=1) — no SetFaction / point-move bridges;
--   the gossip driver is instance-model-blocked anyway (see below).
-- - OnGossipHello/OnGossipSelect: instance->GetData(EVENT_PYRAMID)
--   pyramid-phase gating — instance-model gossip blocked (standing).
-- - LandMine_Timer (30s init, never fires in the C++ UpdateAI — dead
--   leg); SPELL_GOBLIN_LAND_MINE 21688 and SPELL_WEEGLIS_BARREL 10772
--   are dead enum entries (never cast). Reset and JustDied are
--   commented-out in C++ — nothing to port.

local ENTRY = 7607

local SPELL_BOMB = 8858

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
    per[key] = CreateLuaEvent(fn, delay, 1)
end

-- C++ EVENT_BOMB (10s init): DoCastVictim(SPELL_BOMB), non-triggered;
-- reschedule 10s unconditional.
local function onBomb(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_BOMB)
    end
    schedule(guid, "bomb", 10000, function()
        onBomb(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    -- C++ Reset() is effectively empty (body commented out): Bomb_Timer
    -- is NOT re-armed on evade, it freezes and resumes. Preserve a
    -- pending bomb timer across re-engages instead of restarting 10s.
    local per = timers[guid]
    if not (per and per["bomb"]) then
        schedule(guid, "bomb", 10000, function()
            onBomb(creature, guid)
        end)
    end
end

-- C++ Reset() leaves Bomb_Timer untouched, so leave-combat and reset
-- must not cancel the pending bomb; the timer resumes on re-engage.
local function onLeaveCombat(event, creature)
end

local function onReset(event, creature)
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 23, onReset)
