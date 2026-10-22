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
--   (C++ Repeat outside any gate — faithful). Death cancels the Lua-side
--   timer (the C++ AI object dies with its EventMap — a fresh 10s on the
--   next engage); evade preserves it (C++ Reset leaves Bomb_Timer
--   untouched).
-- Unmodeled (documented-only, no bridges):
-- - AttackStart: AttackStartCaster(victim, 10) (keep back & toss
--   bombs/shoot) — no AttackStart bridge.
-- - Shoot 6660 + SetSheath(SHEATH_STATE_RANGED/MELEE): PORTED 2026-10-20
--   (zulfarrak.cpp:311-322) via the new creature:SetSheath bridge
--   (Unit::SetSheath, Unit.h:903). The isAttackReady swing gate has no
--   bridge, so the Lua check re-arms at SWING_MS=2000 (engine default
--   base attack time); melee-range gate via IsWithinDist(victim, 5.0)
--   (C++ IsWithinMeleeRange = max(reach+reach+4/3, 5.0) -> 5.0 here).
--   Melee stays engine-driven (DoMeleeAttackIfReady twin); the shoot
--   fires at most once per swing, only when the victim is out of range.
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
-- - go_troll_cage OnGossipHello crew-arouse machine (zulfarrak.cpp:218-232):
--   initBlyCrewMember per crew member (incl. weegli) — instance
--   GUID-lookup + SetReactState(REACT_AGGRESSIVE) + SetWalk +
--   SetHomePosition + MovePoint + SetFaction(FACTION_ESCORTEE_N_NEUTRAL_ACTIVE).
--   The GO script is unregistered (no Lua); cross-creature legs have no
--   bridges.

local ENTRY = 7607

local SPELL_BOMB = 8858
local SPELL_SHOOT = 6660

-- C++ melee/ranged decision (zulfarrak.cpp:311-322): when the swing timer is
-- ready and the victim is beyond melee range, Weegli shoots 6660 and shows
-- the ranged weapon (SHEATH_STATE_RANGED); otherwise he fights melee and
-- shows the melee weapon (SHEATH_STATE_MELEE). IsWithinMeleeRange is
-- max(reach+reach+4/3, NOMINAL_MELEE_RANGE=5.0); weegli-vs-player resolves
-- to 5.0 (1.5+1.5+1.333 = 4.33 < 5.0).
local MELEE_RANGE_YD = 5.0
-- isAttackReady pacing: the Lua side has no swing-timer bridge, so the
-- check re-arms at the engine's default base attack time (2000ms —
-- NULLIF(t.BaseAttackTime, 0) fallback in creaturemotion.go / combat.go);
-- the shoot then fires at most once per swing, never faster than C++.
local SWING_MS = 2000

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

-- C++ UpdateAI ranged/melee arm (zulfarrak.cpp:311-322): swing-ready and
-- victim beyond melee range -> DoCastVictim(SPELL_SHOOT) non-triggered +
-- SetSheath(SHEATH_STATE_RANGED); otherwise SetSheath(SHEATH_STATE_MELEE).
-- Melee itself is engine-driven in Go (DoMeleeAttackIfReady twin, a no-op
-- out of range), so only the shoot+sheath decision is ported; the 2s
-- re-arm paces the isAttackReady swing gate (SWING_MS above).
local function onSwingCheck(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        if creature:IsWithinDist(victim, MELEE_RANGE_YD) then
            creature:SetSheath(1) -- SHEATH_STATE_MELEE
        else
            creature:CastSpell(victim, SPELL_SHOOT) -- DoCastVictim, non-triggered
            creature:SetSheath(2) -- SHEATH_STATE_RANGED
        end
    end
    schedule(guid, "swing", SWING_MS, function()
        onSwingCheck(creature, guid)
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
    -- The C++ UpdateAI evaluates the shoot/sheath decision every tick while
    -- a victim exists: run one immediate check, then re-arm on the swing
    -- cadence. The poll self-cancels at nil victim only by timer removal;
    -- it is cancelled on leave-combat and death below.
    if not (per and per["swing"]) then
        onSwingCheck(creature, guid)
    end
end

-- C++ Reset() leaves Bomb_Timer untouched, so leave-combat and reset
-- must not cancel the pending bomb; the timer resumes on re-engage.
-- The swing poll has no C++ timer state — it is evaluated only while a
-- victim exists — so leave-combat cancels it and re-engage restarts it.
local function onLeaveCombat(event, creature)
    local per = timers[creature:GetGUID()]
    if per and per["swing"] then
        RemoveEventById(per["swing"])
        per["swing"] = nil
    end
end

local function onReset(event, creature)
end

-- The C++ AI object dies with its EventMap (JustDied commented out, but
-- the creature object is destroyed), so a death must cancel the Lua-side
-- bomb timer: otherwise it re-arms forever post-death and the stale
-- per["bomb"] blocks the fresh 10s ctor re-arm on the next engage.
local function onDied(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)
