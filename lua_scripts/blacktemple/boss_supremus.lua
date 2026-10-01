-- Supremus (Black Temple) — Lua port of
-- src/server/scripts/Outland/BlackTemple/boss_supremus.cpp
-- (boss_supremus only; npc_molten_flame / npc_volcano documented
-- below, not registered); black_temple.h:32 (DATA_SUPREMUS = 1,
-- second boss), :72 (NPC_SUPREMUS = 22898), :89
-- (NPC_SUPREMUS_VOLCANO = 23085). Creature entry: 22898 Supremus
-- (C++ ScriptName "boss_supremus" per AddSC_boss_supremus). Talk
-- lines used: EMOTE_NEW_TARGET=0 (charge target), EMOTE_GROUND_
-- CRACK=2 (volcano); EMOTE_PUNCH_GROUND=1 is declared but never
-- Talked by the C++ AI.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in
-- Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnEnterCombat runs
-- ChangePhase (INITIAL or CHASE -> STRIKE), then arms EVENT_BERSERK
-- (berserk 45078 15min one-shot, triggered self-cast) and
-- EVENT_FLAME (molten punch 40126 20s then {15s,20s}, non-triggered
-- on the victim — C++ DoCast(spellId) resolves AITARGET_VICTIM).
-- STRIKE phase: hateful strike 41926 2s then 5s, non-triggered on
-- the highest-health melee-range threat target (see deviations) +
-- the snare aura 41922 is removed and the taunt immunes drop (the
-- immune arms have no bridge). CHASE phase: volcanic summon 40276
-- 5s then 10s, triggered cast at the boss's own location (C++
-- DoCastAOE) + Talk(EMOTE_GROUND_CRACK) / charge 41581 10s then 10s,
-- non-triggered on a random alive player within 100 yd excluding
-- the current victim (C++ SelectTarget(Random, 1, 100.0f, true)
-- skips position 0, player-only) + Talk(EMOTE_NEW_TARGET) / snare
-- 41922 non-triggered self-cast (C++ DoCast(spellId) resolves
-- AITARGET_SELF for the snare aura) and the taunt immunes go up (no
-- bridge). EVENT_SWITCH_PHASE 60s then 60s alternates the phases
-- (C++-exact). Nil-victim / nil-target ticks cast nothing but keep
-- the schedule (jeklik convention). OnDied(4)/OnLeaveCombat(2)/
-- OnReset(23): cancel timers.
-- The npc_molten_flame and npc_volcano AIs in the same C++ file are
-- not registered — the C++ ScriptNames "npc_molten_flame" /
-- "npc_volcano" bind DB-side in creature_template (no TDB in this
-- workspace), so the entries cannot be verified against the
-- bindings from the C++ sources alone (garr firesworn convention;
-- NPC_SUPREMUS_VOLCANO=23085 exists in the header but the binding
-- is still DB-side).
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING queue gate
-- and the post-event casting check have no UNIT_STATE bridge —
-- timers fire unconditionally (jeklik convention); no instance-
-- script model — boss admission via the luaBossAI shim
-- (BossAI::JustEngagedWith and the DATA_SUPREMUS bookkeeping arms
-- skipped); no threat model — the ResetThreatList/AddThreat
-- (1,000,000 on the charge target) arms and the threat-list pick
-- for hateful strike have no bridge, so the strike target is
-- approximated by the highest-health alive player in the instance
-- within 5 yd (melee range) of the boss; no summon model — the
-- volcanic summon 40276 never spawns a volcano add and the molten
-- flame 40980 / volcanic eruption 40117 / volcanic geyser 42055
-- add arms are unregistered with it; the summons.DoAction(ACTION_
-- DISABLE_VULCANO) aura-cleanup arm has no bearer (no volcano
-- summons exist to address); the ApplySpellImmune taunt/attack-me
-- arms have no bridge; the EnterEvadeMode summons.DespawnAll/
-- _DespawnAtEvade arm has no despawn bridge (evade-side cleanup is
-- engine-side).

local ENTRY_SUPREMUS = 22898

local PHASE_INITIAL = 1
local PHASE_STRIKE = 2
local PHASE_CHASE = 3

local EMOTE_NEW_TARGET = 0
local EMOTE_GROUND_CRACK = 2

local SPELL_MOLTEN_PUNCH = 40126
local SPELL_HATEFUL_STRIKE = 41926
local SPELL_VOLCANIC_SUMMON = 40276
local SPELL_BERSERK = 45078
local SPELL_SNARE_SELF = 41922
local SPELL_CHARGE = 41581

local MELEE_RANGE = 5

local timers = {}
local state = {}

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

local function cancelTimer(guid, key)
    local per = timers[guid]
    if per and per[key] then
        RemoveEventById(per[key])
        per[key] = nil
    end
end

local function supremusState(guid)
    local st = state[guid]
    if not st then
        st = { phase = PHASE_INITIAL }
        state[guid] = st
    end
    return st
end

local function playersInInstance(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            found[#found + 1] = p
        end
    end
    return found
end

-- C++ CalculateHatefulStrikeTarget: highest-health threat-list unit
-- within melee range. No threat-list bridge — approximated by the
-- highest-health alive player in the instance within melee range.
local function hatefulStrikeTarget(creature)
    local best, bestHealth = nil, 0
    for _, p in ipairs(playersInInstance(creature)) do
        local health = p:GetHealth()
        if health > bestHealth and creature:GetDistance(p) <= MELEE_RANGE then
            best, bestHealth = p, health
        end
    end
    return best
end

-- C++ SelectTarget(Random, 1, 100.0f, true): a random alive player in
-- the instance within 100 yd, excluding the current victim (position
-- 0), player-only.
local function chargeTarget(creature)
    local victim = creature:GetVictim()
    local candidates = {}
    for _, p in ipairs(playersInInstance(creature)) do
        if (not victim or p:GetGUID() ~= victim:GetGUID())
                and creature:GetDistance(p) <= 100 then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EVENT_FLAME: non-triggered molten punch on the victim; repeat
-- {15s,20s}. Phase-independent (C++-exact).
local function onFlame(creature, guid)
    if creature:GetVictim() then
        creature:CastSpell(nil, SPELL_MOLTEN_PUNCH)
    end
    schedule(guid, "flame", 15000 + math.random(0, 5000), function()
        onFlame(creature, guid)
    end)
end

-- C++ EVENT_BERSERK: triggered self-cast 45078; one-shot
-- (C++-exact — no re-arm). Phase-independent.
local function onBerserk(creature, guid)
    creature:CastSpell(creature, SPELL_BERSERK, true)
end

-- C++ EVENT_HATEFUL_STRIKE: non-triggered 41926 on the strike
-- target; repeat 5s unconditional (C++-exact — the repeat runs even
-- when no target was found). Strike-phase only.
local function onHatefulStrike(creature, guid)
    local target = hatefulStrikeTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_HATEFUL_STRIKE)
    end
    schedule(guid, "hateful", 5000, function()
        onHatefulStrike(creature, guid)
    end)
end

-- C++ EVENT_VOLCANO: triggered DoCastAOE(40276) at the boss's own
-- location — modeled as a triggered self-cast — + Talk(EMOTE_GROUND_
-- CRACK); repeat 10s unconditional (C++-exact). The volcano add
-- never spawns (no summon model). Chase-phase only.
local function onVolcano(creature, guid)
    creature:CastSpell(creature, SPELL_VOLCANIC_SUMMON, true)
    creature:Talk(EMOTE_GROUND_CRACK)
    schedule(guid, "volcano", 10000, function()
        onVolcano(creature, guid)
    end)
end

-- C++ EVENT_SWITCH_TARGET: non-triggered charge 41581 on the charge
-- target + Talk(EMOTE_NEW_TARGET); repeat 10s unconditional
-- (C++-exact). The ResetThreatList/AddThreat arms have no threat
-- bridge — only the cast and the Talk are kept. Chase-phase only.
local function onSwitchTarget(creature, guid)
    local target = chargeTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_CHARGE)
        creature:Talk(EMOTE_NEW_TARGET)
    end
    schedule(guid, "switchtarget", 10000, function()
        onSwitchTarget(creature, guid)
    end)
end

-- C++ ChangePhase: STRIKE from INITIAL or CHASE (removes the snare
-- aura; the taunt-immune drops and the volcano summons' DoAction
-- have no bridges); CHASE otherwise (arms volcano/switch-target,
-- non-triggered self-cast snare; the taunt-immune raise has no
-- bridge). The ResetThreatList/DoZoneInCombat arms have no bridge.
-- EVENT_SWITCH_PHASE is re-armed 60s unconditional (C++-exact).
local function changePhase(creature, guid)
    local st = supremusState(guid)
    if st.phase == PHASE_INITIAL or st.phase == PHASE_CHASE then
        st.phase = PHASE_STRIKE
        cancelTimer(guid, "volcano")
        cancelTimer(guid, "switchtarget")
        creature:RemoveAura(SPELL_SNARE_SELF)
        schedule(guid, "hateful", 2000, function()
            onHatefulStrike(creature, guid)
        end)
    else
        st.phase = PHASE_CHASE
        cancelTimer(guid, "hateful")
        creature:CastSpell(creature, SPELL_SNARE_SELF)
        schedule(guid, "volcano", 5000, function()
            onVolcano(creature, guid)
        end)
        schedule(guid, "switchtarget", 10000, function()
            onSwitchTarget(creature, guid)
        end)
    end
    schedule(guid, "switchphase", 60000, function()
        changePhase(creature, guid)
    end)
end

local function supremusEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    state[guid] = { phase = PHASE_INITIAL }
    changePhase(creature, guid)
    schedule(guid, "flame", 20000, function()
        onFlame(creature, guid)
    end)
    schedule(guid, "berserk", 900000, function()
        onBerserk(creature, guid)
    end)
end

local function supremusResetState(guid)
    cancelTimers(guid)
    state[guid] = nil
end

local function supremusLeaveCombat(event, creature)
    supremusResetState(creature:GetGUID())
end

local function supremusDied(event, creature, killer)
    supremusResetState(creature:GetGUID())
end

local function supremusReset(event, creature)
    supremusResetState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_SUPREMUS, 1, supremusEnterCombat)
RegisterCreatureEvent(ENTRY_SUPREMUS, 2, supremusLeaveCombat)
RegisterCreatureEvent(ENTRY_SUPREMUS, 4, supremusDied)
RegisterCreatureEvent(ENTRY_SUPREMUS, 23, supremusReset)
