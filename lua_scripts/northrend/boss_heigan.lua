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
-- seeds (event 1; BossAI::JustEngagedWith SetBossState IN_PROGRESS
-- PORTED 2026-11-05 + DoZoneInCombat (moroes precedent); the
-- CheckRequiredBosses leg and the eruption-tile GO enumeration
-- have no bridge — tharon_ja precedent; EVENT_ERUPT's 15s seed is
-- GO-based, stays out);
-- KilledUnit Talk(SAY_SLAY 1) (event 3 —
-- C++-GATED: the Talk fires only when victim->GetTypeId()==
-- TYPEID_PLAYER — Lua expresses it via victim:GetObjectType() ==
-- "Player" (nalorakk / kelthuzad player-gate precedent); the
-- _safetyDance latch is PORTED tick 417 via creature:SetData
-- (DATA_SAFETY_DANCE 19962139): set at engage (C++ :127), cleared
-- on a player kill (C++ KilledUnit :98-104). It is NOT written in
-- the ev23 Reset handler: ev23 pre-fires on death (fireCreatureDied,
-- lua_creature_events.go) ahead of the kill-credit achievement read
-- (kill.go), so a Reset re-arm would clobber the fight's latch
-- before the criteria check reads it — engage is the re-arm site,
-- Reset stays latch-free.
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
-- callbacks converges to the same next-fire times), CastStop PORTED
-- 2026-11-05 (creature:StopSpellCast(), gothik teleport precedent —
-- the old "vacuous at the 45s channel-expiry boundary" note assumed
-- exact channel/timer alignment; C++ calls CastStop unconditionally,
-- so the port does too), SetReactState(REACT_AGGRESSIVE) (bridged).
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
-- Reset's _Reset() SetBossState NOT_STARTED leg PORTED 2026-11-05
-- (moroes precedent, != DONE-gated per ScriptedCreature.cpp:509-510);
-- SetReactState(REACT_AGGRESSIVE) IS ported (event 23).
-- GetData(DATA_SAFETY_DANCE 19962139) + the _safetyDance latch —
-- PORTED tick 417 (see above): the "no GetData bridge" claim is
-- stale — creature:SetData is bridged (lua_creature_events.go:792)
-- and the Go gate reads it via bossLatchData
-- (engine/world/achievement_criteria_scripts.go).
-- FIDELITY DELTA: the eruption lethal-hit relay
-- (spell_heigan_eruption -> AI()->KilledUnit, :235-245) has no Go
-- twin — eruptions are unported (GO arm above), so no eruption
-- damage exists in Go to relay; every death the Go engine can
-- produce in this fight (melee / fever / plague cloud / disrupt)
-- is Heigan-attributed and reaches ev3 via kill.go:1081-1089.
-- spell_heigan_eruption (SpellScriptLoader, eruption lethal-hit ->
-- instance GetGuidData(DATA_HEIGAN) -> AI()->KilledUnit) — no
-- SpellScript binding bridge (razelikh precedent) + instance /
-- cross-AI bridges absent.
-- achievement_safety_dance — no achievement-criteria bridge (snakes
-- precedent) + cross-AI GetData absent.

local ENTRY_HEIGAN = 15936

local BOSS_HEIGAN = 4 -- naxxramas.h NAXEncounter (ANUBREKHAN 0 ... HEIGAN 4)
local BOSS_STATE_NOT_STARTED = 0
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_DONE = 3

local DATA_SAFETY_DANCE = 19962139 -- boss_heigan.cpp:64

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
    -- C++ :186 (EVENT_DANCE_END) — CastStop then SetReactState(
    -- REACT_AGGRESSIVE) then me-form DoZoneInCombat (C++ arm order).
    creature:StopSpellCast()
    creature:SetReactState(REACT_AGGRESSIVE)
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
    -- C++ BossAI::JustEngagedWith (ScriptedCreature.cpp:527-542) sets
    -- IN_PROGRESS before the pulse + DoZoneInCombat; the instance
    -- CheckRequiredBosses leg stays out.
    creature:SetBossState(BOSS_HEIGAN, BOSS_STATE_IN_PROGRESS)
    creature:SetCombatPulseDelay(5) -- BossAI::JustEngagedWith arm 1 (ScriptedCreature.cpp:540)
    creature:DoZoneInCombat()
    creature:Talk(SAY_AGGRO)
    -- C++ JustEngagedWith (:127): _safetyDance = true.
    creature:SetData(DATA_SAFETY_DANCE, 1)
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
        -- C++ KilledUnit (:98-104): _safetyDance = false.
        creature:SetData(DATA_SAFETY_DANCE, 0)
    end
end

local function heiganDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    -- C++ JustDied (:110-114): _JustDied SetBossState DONE
    -- (ScriptedCreature.cpp:513-520; moroes precedent).
    creature:SetBossState(BOSS_HEIGAN, BOSS_STATE_DONE)
    cancelTimers(creature:GetGUID())
end

local function heiganReset(event, creature)
    -- C++ Reset (:90-94): SetReactState(REACT_AGGRESSIVE) + _Reset
    -- (SetBossState NOT_STARTED when state != DONE,
    -- ScriptedCreature.cpp:499-511).
    if creature:GetBossState(BOSS_HEIGAN) ~= BOSS_STATE_DONE then
        creature:SetBossState(BOSS_HEIGAN, BOSS_STATE_NOT_STARTED)
    end
    cancelTimers(creature:GetGUID())
    creature:SetReactState(REACT_AGGRESSIVE)
end

RegisterCreatureEvent(ENTRY_HEIGAN, 1, heiganEnterCombat)
RegisterCreatureEvent(ENTRY_HEIGAN, 2, heiganLeaveCombat)
RegisterCreatureEvent(ENTRY_HEIGAN, 3, heiganTargetDied)
RegisterCreatureEvent(ENTRY_HEIGAN, 4, heiganDied)
RegisterCreatureEvent(ENTRY_HEIGAN, 23, heiganReset)
