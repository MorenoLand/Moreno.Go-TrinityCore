-- Zul'Farrak: Sergeant Bly — Lua port of
-- src/server/scripts/Kalimdor/ZulFarrak/zulfarrak.cpp (class
-- npc_sergeant_bly : CreatureScript("npc_sergeant_bly") {
-- npc_sergeant_blyAI : public ScriptedAI }; GetAI via
-- GetZulFarrakAI<npc_sergeant_blyAI> (zulfarrak.h — GetInstanceAI<AI>(obj,
-- "instance_zulfarrak") gate, ZFScriptName "instance_zulfarrak",
-- DataHeader "ZF"); AddSC_zulfarrak at end registers it alongside
-- npc_weegli_blastfuse, go_shallow_grave, at_zumrah, go_troll_cage;
-- kalimdor loader decl 103 / call 216 per kalimdor_script_loader.cpp —
-- the SECOND "// Zul'Farrak" loader group, right after
-- AddSC_boss_zum_rah()). Whole-server-tree "npc_sergeant_bly" grep hits
-- only the ZulFarrak dir files + loader (sole-source verified); zero
-- sql/ hits.
-- Entry: zulfarrak.h ZFEntries names ENTRY_BLY = 7604 (alongside the
-- pyramid ENTRY_RAVEN/ORO/WEEGLI/MURTA entries) — kalecgos check PASSES;
-- creature_template ScriptName binding stays DB-side. Eluna creature
-- events: 1 OnEnterCombat, 2 OnLeaveCombat, 23 OnReset. Timers via
-- CreateLuaEvent; melee is engine-driven in Go (creature combat tick),
-- like C++ DoMeleeAttackIfReady. No C++ HasUnitState(UNIT_STATE_CASTING)
-- gates in this AI.
-- Ported arms (combat only):
-- - Shield Bash 11972: DoCastVictim non-triggered via creature:GetVictim()
--   (boss_onyxia precedent), init 5s, reschedule 15s unconditional
--   (C++ Repeat outside any gate — faithful).
-- - Revenge 12170: DoCastVictim non-triggered, init 8s, reschedule 10s
--   unconditional. (C++ carries a code comment that Revenge should only
--   fire after a dodge/parry/block and needs Trinity support — the C++
--   itself casts it unconditionally, and this port mirrors the code, not
--   the comment.)
-- Unmodeled (documented-only, no bridges):
-- - Reset: me->SetFaction(FACTION_FRIENDLY) — no SetFaction bridge
--   (the same block as boss_zum_rah's Reset arm).
-- - postGossipStep 1..3 event machine in UpdateAI: case 1 drives
--   weegli->AI()->DoAction(0) via instance->GetGuidData(ENTRY_WEEGLI)
--   (no instance GUID-lookup / cross-creature DoAction bridges);
--   Talk(SAY_1=0) / Talk(SAY_2=1) bridgeable but the whole machine's
--   driver is the blocked gossip path; case 3 SetFaction(FACTION_MONSTER)
--   + AttackStart(target) + switchFactionIfAlive(ENTRY_RAVEN/ORO/MURTA)
--   (no SetFaction bridge).
-- - OnGossipHello/OnGossipSelect: instance->GetData(EVENT_PYRAMID)
--   pyramid-phase gating — instance-model gossip blocked (standing).
-- - DoAction(0) (sets postGossipStep=1): no DoAction bridge.

local ENTRY = 7604

local SPELL_SHIELD_BASH = 11972
local SPELL_REVENGE     = 12170

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

-- C++ EVENT_SHIELD_BASH (5s init): DoCastVictim(SPELL_SHIELD_BASH),
-- non-triggered; reschedule 15s unconditional.
local function onShieldBash(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SHIELD_BASH)
    end
    schedule(guid, "shieldbash", 15000, function()
        onShieldBash(creature, guid)
    end)
end

-- C++ EVENT_REVENGE (8s init): DoCastVictim(SPELL_REVENGE),
-- non-triggered; reschedule 10s unconditional.
local function onRevenge(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_REVENGE)
    end
    schedule(guid, "revenge", 10000, function()
        onRevenge(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "shieldbash", 5000, function()
        onShieldBash(creature, guid)
    end)
    schedule(guid, "revenge", 8000, function()
        onRevenge(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 23, onReset)
