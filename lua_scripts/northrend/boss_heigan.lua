-- Heigan the Unclean (Naxxramas) — Lua port of
-- src/server/scripts/Northrend/Naxxramas/boss_heigan.cpp
-- (boss_heigan (BossAI); spell_heigan_eruption (SpellScriptLoader);
-- achievement_safety_dance (AchievementCriteriaScript);
-- AddSC_boss_heigan registers all three; loader decl 71 / call 266 per
-- northrend_script_loader.cpp — the THIRTEENTH Naxxramas group in
-- AddNorthrendScripts(), under the "// Naxxramas" marker; the call
-- before it is AddSC_boss_faerlina() (decl 70 / call 265), the call
-- after is AddSC_boss_gothik() (decl 72 / call 267 — lua exists,
-- audit-and-close when its group comes up)).
-- Entry: 15936 Heigan the Unclean (naxxramas.h NPC_HEIGAN :97 —
-- kalecgos pass; the CreatureScript ScriptName binding is
-- instance-shimmed, the creature_template binding DB-side as usual).
-- Sole-source verified: whole-scripts-tree grep for "boss_heigan" /
-- "spell_heigan_eruption" / "achievement_safety_dance" hits
-- boss_heigan.cpp only (loader carries only the decl/call lines);
-- zero sql/ hits. No heigan lua existed. Talk() count = 6
-- grep-confirmed (zero spaced "Talk (" variant) — ALL SIX accounted
-- below. BOSS_HEIGAN (naxxramas.h :34), DATA_HEIGAN (:73).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent;
-- melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady (jedoga bar — engagement-only leg,
-- documented). All four C++ scheduled arms are ported below (the
-- erupt schedule is GO-based and stays out); timers cancel on
-- 2/4/23 (timers table keyed by GUID, nightbane precedent).
-- Ported arms (C++-exact for all modeled arms):
-- boss_heigan: JustEngagedWith Talk(SAY_AGGRO 0) + the four event
-- seeds (event 1; BossAI::JustEngagedWith instance-bookkeeping leg and
-- the eruption-tile GO enumeration have no bridge — tharon_ja
-- precedent; EVENT_ERUPT's 15s seed is GO-based, stays out);
-- KilledUnit Talk(SAY_SLAY 1) (event 3 —
-- C++-GATED: the Talk fires only when victim->GetTypeId()==
-- TYPEID_PLAYER — Lua expresses it via victim:GetObjectType() ==
-- "Player" (nalorakk / kelthuzad player-gate precedent); the
-- _safetyDance=false latch has no GetData bridge — documented
-- below); JustDied Talk(SAY_DEATH 3) (event 4; _JustDied instance
-- bookkeeping has no bridge — tharon_ja precedent).
-- EVENT_DISRUPT (init randtime(15s,20s), repeat 11s) —
-- DoCastAOE(SPELL_SPELL_DISRUPTION 29310) 1-arg (non-triggered) is the
-- established self-cast: creature:CastSpell(creature, 29310)
-- (selin_fireheart / mr_smite DoCastAOE convention); PHASE_FIGHT-only
-- in C++ — the dance latch below is the SetPhase twin.
-- EVENT_FEVER (init randtime(10s,20s), repeat randtime(20s,25s)) —
-- DoCastAOE(SPELL_DECREPIT_FEVER 29998) 1-arg (non-triggered)
-- self-cast, same convention. FIDELITY NOTE: the C++ hard-codes 29998
-- on both difficulties — the enum comment "// 25-man: 55011" is stale
-- vs the C++ text; a future port must NOT raid-mode it.
-- EVENT_DANCE (90s, PHASE_FIGHT-only) — PORTED with a per-GUID
-- `dancing` latch as the SetPhase(PHASE_DANCE) twin (tharon_ja /
-- gal_darah cyclic-chain idiom; the only phase flips are these two
-- timer-driven arms, so the latch is C++-exact): Talk(SAY_TAUNT 2) +
-- Talk(EMOTE_DANCE 4) in C++ arm order, SetReactState(REACT_PASSIVE)
-- (bridged — the old "no react-state bridge" note is superseded),
-- AttackStop (bridged 2026-10-22), StopMoving (bridged this tick),
-- DoCast(SPELL_TELEPORT_SELF 30211) 1-arg non-triggered self-cast
-- (the positional leg rides the Go spell system), DoCastAOE(
-- SPELL_PLAGUE_CLOUD 29350) 1-arg non-triggered self-cast
-- (DoCastAOE convention), then the 45s DANCE_END seed. The
-- RescheduleEvent(EVENT_ERUPT, 10s) leg is moot — erupt's whole arm
-- rides GO bridges (below) and is unported. Talk(SAY_TAUNT 2) and
-- Talk(EMOTE_DANCE 4) no longer ride an unbridged bar — the king_dred
-- genuinely-bridgeable-bar objection is lifted by the StopMoving /
-- SetReactState / AttackStop bridges.
-- EVENT_DANCE_END (45s after dance start) — PORTED in C++ arm order:
-- Talk(EMOTE_DANCE_END 5), re-seed DISRUPT randtime(10s,25s) / FEVER
-- randtime(15s,20s) / DANCE 90s (C++ ScheduleEvent overwrites the
-- phase-blocked pendings — the latch-skip in the disrupt/fever
-- callbacks converges to the same next-fire times), SetReactState(
-- REACT_AGGRESSIVE) (bridged). CastStop() is vacuous — every cast in
-- this file is instant, nothing channels; DoZoneInCombat() (:186)
-- is PORTED 2026-10-23 in onDanceEnd via the live bridge
-- (engine/world/lua_creature_events.go:426) — the old "no bridge"
-- claim is retracted (combat never left Lua-side — threat list
-- intact — but C++ explicitly zones-in the whole map here, so the
-- port is C++-exact).
-- The old "no DoCastAOE bridge" claims are superseded by the
-- phase-gating verdict (Oct-13 ICC strip convention).
-- Unmodeled (no bridges — documented, not wired):
-- EVENT_ERUPT (init 15s, re-armed by the dance arms) — the entire arm
-- rides GO bridges: TeleportCheaters, _eruptTiles populated from the
-- map GO-by-spawnId store (firstEruptionDBGUID 84980, 4 sections of
-- 15/25/23/13), ObjectAccessor GO lookup + tile->CastSpell(trap
-- spell) — no GO-spawnId-store / ObjectAccessor / GO-cast bridges —
-- documented-only.
-- Reset's _Reset() (instance bookkeeping — tharon_ja precedent);
-- SetReactState(REACT_AGGRESSIVE) IS ported (event 23).
-- GetData(DATA_SAFETY_DANCE 19962139) + the _safetyDance latch — no
-- GetData bridge (drakkari_colossus precedent).
-- spell_heigan_eruption (SpellScriptLoader, eruption lethal-hit ->
-- instance GetGuidData(DATA_HEIGAN) -> AI()->KilledUnit) — no
-- SpellScript binding bridge (razelikh precedent) + instance /
-- cross-AI bridges absent.
-- achievement_safety_dance — no achievement-criteria bridge (snakes
-- precedent) + cross-AI GetData absent.

local ENTRY_HEIGAN = 15936

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_TAUNT = 2
local SAY_DEATH = 3
local EMOTE_DANCE = 4
local EMOTE_DANCE_END = 5

local SPELL_SPELL_DISRUPTION = 29310
local SPELL_DECREPIT_FEVER = 29998
local SPELL_TELEPORT_SELF = 30211
local SPELL_PLAGUE_CLOUD = 29350

local REACT_PASSIVE = 0 -- UnitDefines.h:340
local REACT_AGGRESSIVE = 2

local timers = {}
local dancing = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
    dancing[guid] = nil
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

local function onDisrupt(creature, guid)
    if dancing[guid] then
        return
    end
    creature:CastSpell(creature, SPELL_SPELL_DISRUPTION)
    schedule(guid, "disrupt", 11000, function() onDisrupt(creature, guid) end)
end

local function onFever(creature, guid)
    if dancing[guid] then
        return
    end
    creature:CastSpell(creature, SPELL_DECREPIT_FEVER)
    schedule(guid, "fever", {20000, 25000}, function() onFever(creature, guid) end)
end

local onDance

local function onDanceEnd(creature, guid)
    dancing[guid] = nil
    creature:Talk(EMOTE_DANCE_END)
    schedule(guid, "disrupt", {10000, 25000}, function() onDisrupt(creature, guid) end)
    schedule(guid, "fever", {15000, 20000}, function() onFever(creature, guid) end)
    schedule(guid, "dance", 90000, function() onDance(creature, guid) end)
    creature:SetReactState(REACT_AGGRESSIVE)
    -- C++ :186 (EVENT_DANCE_END) — me-form DoZoneInCombat after
    -- SetReactState(REACT_AGGRESSIVE) (C++ arm order; CastStop vacuous here).
    creature:DoZoneInCombat()
end

onDance = function(creature, guid)
    dancing[guid] = true
    creature:Talk(SAY_TAUNT)
    creature:Talk(EMOTE_DANCE)
    creature:SetReactState(REACT_PASSIVE)
    creature:AttackStop()
    creature:StopMoving()
    creature:CastSpell(creature, SPELL_TELEPORT_SELF)
    creature:CastSpell(creature, SPELL_PLAGUE_CLOUD)
    schedule(guid, "dance_end", 45000, function() onDanceEnd(creature, guid) end)
end

local function heiganEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "disrupt", {15000, 20000}, function() onDisrupt(creature, guid) end)
    schedule(guid, "fever", {10000, 20000}, function() onFever(creature, guid) end)
    schedule(guid, "dance", 90000, function() onDance(creature, guid) end)
end

local function heiganLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function heiganTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

local function heiganDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    cancelTimers(creature:GetGUID())
end

local function heiganReset(event, creature)
    cancelTimers(creature:GetGUID())
    creature:SetReactState(REACT_AGGRESSIVE)
end

RegisterCreatureEvent(ENTRY_HEIGAN, 1, heiganEnterCombat)
RegisterCreatureEvent(ENTRY_HEIGAN, 2, heiganLeaveCombat)
RegisterCreatureEvent(ENTRY_HEIGAN, 3, heiganTargetDied)
RegisterCreatureEvent(ENTRY_HEIGAN, 4, heiganDied)
RegisterCreatureEvent(ENTRY_HEIGAN, 23, heiganReset)
