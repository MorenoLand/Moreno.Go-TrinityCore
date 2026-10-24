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
-- drops are no-op removes of rows the raises never inserted; the
-- bridge exists: creature:ApplySpellImmune). CHASE phase: volcanic summon 40276
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
-- The npc_molten_flame AI in the same C++ file is not registered — its
-- C++ ScriptName binds DB-side in creature_template (no TDB in this
-- workspace), so the entry cannot be verified against the binding from
-- the C++ sources alone (garr firesworn convention). npc_volcano needs
-- no standalone registration: its AI is relayed from the boss event-19
-- handler below (NPC_SUPREMUS_VOLCANO=23085, black_temple.h:89).
-- Cast gating: boss_supremus has no UpdateAI override, so it runs
-- under BossAI::UpdateAI (ScriptedCreature.cpp:571-589), which gates
-- event execution on HasUnitState(UNIT_STATE_CASTING). That gate has
-- no Lua bridge, so timers fire unconditionally (jeklik convention) —
-- a standing deviation (the old boss_supremus.cpp:213-216 citation was
-- npc_volcano's UpdateAI, not the boss's). Deviations from C++: no instance-
-- script model — boss admission via the luaBossAI shim
-- (BossAI::JustEngagedWith and the DATA_SUPREMUS bookkeeping arms
-- skipped); no threat model — the ResetThreatList/AddThreat
-- (1,000,000 on the charge target) arms and the threat-list pick
-- for hateful strike: ResetAllThreat IS bridged (lua_objects.go)
-- but the arm is intentionally skipped — the C++ pairs it with the
-- unbridged AddThreat(1e6) top-up, and zeroing alone would not fixate
-- the charge target the way the C++ 1e6 entry does; the strike target is
-- approximated by the highest-health alive player in the instance
-- within 5 yd (melee range) of the boss; no summon model — the
-- volcanic summon 40276 spawns volcano adds through the Go spell-effect
-- summon (creatureEffectSummonType) when the DBC props pass, and the volcano
-- AI is relayed from the boss event-19 handler below (3s eruption cast); the
-- molten flame 40980 arm stays out (its summon source is not in this file);
-- the summons.DoAction(ACTION_DISABLE_VULCANO) aura-cleanup arm is ported
-- via GetCreaturesInGrid in ChangePhase; the ApplySpellImmune taunt/attack-me
-- Reset arms stay out — they are no-op removes undoing the
-- ChangePhase adds, which ride the unported phase machine (the
-- immunity bridge itself exists: creature:ApplySpellImmune); the
-- EnterEvadeMode summons.DespawnAll arm is ported via GetCreaturesInGrid below, while the
-- _DespawnAtEvade() -> DespawnOrUnsummon() leg is PORTED below
-- (stale "no despawn bridge" claim retracted: creature:Despawn()
-- is bridged, engine/world/lua_creature_events.go:214).

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

local ENTRY_VOLCANO = 23085
local SPELL_VOLCANIC_ERUPTION = 40117
local SPELL_VOLCANIC_GEYSER = 42055

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

-- C++ CalculateHatefulStrikeTarget (boss_supremus.cpp:118-136):
-- highest-health threat-list unit within melee range (no type filter —
-- creatures count). creature:GetAITargets() is the bridged threat list
-- (lua_creature_events.go); melee range is the 5-yd GetDistance stand-in
-- (venoxis/gruul convention), replacing the retired "no threat-list
-- bridge" instance-player approx 2026-10-23.
local function hatefulStrikeTarget(creature)
    local best, bestHealth = nil, 0
    for _, unit in ipairs(creature:GetAITargets()) do
        local health = unit:GetHealth()
        if health > bestHealth and creature:GetDistance(unit) <= MELEE_RANGE then
            best, bestHealth = unit, health
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
-- CRACK); repeat 10s unconditional (C++-exact). The volcano add's AI is relayed via the boss event-19 handler below.
-- Chase-phase only.
local function onVolcano(creature, guid)
    creature:CastSpell(creature, SPELL_VOLCANIC_SUMMON, true)
    creature:Talk(EMOTE_GROUND_CRACK)
    schedule(guid, "volcano", 10000, function()
        onVolcano(creature, guid)
    end)
end

-- C++ EVENT_SWITCH_TARGET: non-triggered charge 41581 on the charge
-- target + Talk(EMOTE_NEW_TARGET); repeat 10s unconditional
-- (C++-exact). The ResetThreatList/AddThreat arms are skipped:
-- ResetAllThreat is bridged (lua_objects.go) but the C++ arm
-- pairs it with the unbridged AddThreat(target, 1e6) top-up — zeroing
-- alone would not fixate the charge target, a state the C++ never
-- produces; only the cast and the Talk are kept. Chase-phase only.
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
-- aura, drops the taunt-immune rows, and the volcano summons'
-- DoAction is ported via grid below); CHASE otherwise (arms volcano/
-- switch-target, non-triggered self-cast snare, raises the
-- taunt-immune rows). The rows are LIVE: creature:ApplySpellImmune
-- feeds the script-immunity rows the Go hit/aura gates consult
-- (immunity.go:1390 IMMUNITY_EFFECT, :1440 IMMUNITY_STATE), so a
-- raised row blocks effect-114 taunts (IMMUNE2 strip) and aura-11
-- taunts (aura-apply strip) — gurtogg precedent ported at
-- boss_gurtogg_bloodboil.lua:357-358. The ChangePhase tail
-- (supremus.cpp:113-115) is PORTED below: ResetThreatList ->
-- creature:DoZoneInCombat (C++ order; bridge live 2026-10-23 — the
-- phase machine is ported in this file, so the me->DoZoneInCombat()
-- tail is bridgeable). The taunt-immune row arms are ported via
-- ApplySpellImmune.
-- EVENT_SWITCH_PHASE is re-armed 60s unconditional (C++-exact).
-- NOTE (2026-10-13): the opposite-phase timer cancels above are
-- LOCAL-TREE-exact. Stock TC's EventMap keeps phase-mismatched
-- pending events for immediate fire on phase return, but this
-- tree's EventMap::ExecuteEvent (src/common/Utilities/EventMap.cpp:81)
-- erases expired phase-mismatched events (marli precedent) — the
-- Lua cancels model that erase exactly.
local function changePhase(creature, guid)
    local st = supremusState(guid)
    if st.phase == PHASE_INITIAL or st.phase == PHASE_CHASE then
        st.phase = PHASE_STRIKE
        cancelTimer(guid, "volcano")
        cancelTimer(guid, "switchtarget")
        -- C++ ChangePhase STRIKE: summons.DoAction(ACTION_DISABLE_VULCANO)
        -- (boss_supremus.cpp) — volcanoes sit at the boss location (DoCastAOE
        -- spawn, NullCreatureAI, no motion), so the grid sweep addresses them all.
        for _, volcano in ipairs(creature:GetCreaturesInGrid(ENTRY_VOLCANO, 100)) do
            volcano:RemoveAura(SPELL_VOLCANIC_ERUPTION)
            volcano:RemoveAura(SPELL_VOLCANIC_GEYSER)
        end
        creature:RemoveAura(SPELL_SNARE_SELF)
        schedule(guid, "hateful", 2000, function()
            onHatefulStrike(creature, guid)
        end)
        -- C++ ChangePhase STRIKE (supremus.cpp:101-102): drop the
        -- taunt-immune rows (op 1 = IMMUNITY_STATE, 11 =
        -- SPELL_AURA_MOD_TAUNT; op 0 = IMMUNITY_EFFECT, 114 =
        -- SPELL_EFFECT_ATTACK_ME).
        creature:ApplySpellImmune(0, 1, 11, false)
        creature:ApplySpellImmune(0, 0, 114, false)
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
        -- C++ ChangePhase CHASE (supremus.cpp:109-110): raise the
        -- taunt-immune rows.
        creature:ApplySpellImmune(0, 1, 11, true)
        creature:ApplySpellImmune(0, 0, 114, true)
    end
    -- C++ ChangePhase tail (supremus.cpp:113-115): ResetThreatList() ->
    -- DoZoneInCombat() (me, PORTED 2026-10-23 — bridge live; the
    -- ChangePhase machine is ported in this file, so the me->DoZoneInCombat()
    -- tail is bridgeable) -> 60s EVENT_SWITCH_PHASE re-arm (C++ order).
    creature:ResetAllThreat()
    creature:DoZoneInCombat()
    schedule(guid, "switchphase", 60000, function()
        changePhase(creature, guid)
    end)
end

-- C++ npc_volcano::Reset (boss_supremus.cpp): 3s self-cast of
-- VOLCANIC_ERUPTION 40117; DoAction(DISABLE) removes eruption + geyser
-- auras (ported in ChangePhase via grid). Temp summons bypass the spawn
-- Reset funnel, so the Reset arm is driven here from the boss event-19
-- relay (terestian precedent): the 40276 spell-effect summon fires 19 on
-- the boss with the volcano as the summon arg. If the spell-effect summon
-- does not spawn (DBC props), this never fires and nothing is fabricated.
-- The pending eruption is cancelled with the boss timers on LeaveCombat,
-- Died, or Reset.
local function supremusJustSummoned(event, creature, summon)
    if summon:GetEntry() == ENTRY_VOLCANO then
        local key = "volcanoEruption:" .. summon:GetGUID()
        schedule(creature:GetGUID(), key, 3000, function()
            summon:CastSpell(summon, SPELL_VOLCANIC_ERUPTION)
        end)
    end
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
    -- C++ EnterEvadeMode: summons.DespawnAll() before _DespawnAtEvade().
    for _, volcano in ipairs(creature:GetCreaturesInGrid(ENTRY_VOLCANO, 100)) do
        volcano:Despawn()
    end
    -- C++ EnterEvadeMode: _DespawnAtEvade() -> DespawnOrUnsummon()
    -- (summons.DespawnAll is ported via GetCreaturesInGrid above).
    creature:Despawn()
end

local function supremusDied(event, creature, killer)
    supremusResetState(creature:GetGUID())
end

local function supremusReset(event, creature)
    supremusResetState(creature:GetGUID())
    -- C++ Reset (supremus.cpp:73-75): drop the taunt-immune rows.
    creature:ApplySpellImmune(0, 1, 11, false)
    creature:ApplySpellImmune(0, 0, 114, false)
end

RegisterCreatureEvent(ENTRY_SUPREMUS, 1, supremusEnterCombat)
RegisterCreatureEvent(ENTRY_SUPREMUS, 19, supremusJustSummoned)
RegisterCreatureEvent(ENTRY_SUPREMUS, 2, supremusLeaveCombat)
RegisterCreatureEvent(ENTRY_SUPREMUS, 4, supremusDied)
RegisterCreatureEvent(ENTRY_SUPREMUS, 23, supremusReset)
