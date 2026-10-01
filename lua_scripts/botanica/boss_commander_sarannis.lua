-- Commander Sarannis (The Botanica, Tempest Keep) --
-- Lua port of src/server/scripts/Outland/TempestKeep/
-- botanica/boss_commander_sarannis.cpp (boss_commander_
-- sarannis — the only CreatureScript AI class AddSC_boss_
-- commander_sarannis registers; the SpellScriptLoader
-- spell_commander_sarannis_summon_reinforcements has no
-- SpellScript bridge — SpellScript handlers are not
-- modeled, standing blocker, documented only).
-- Fifth and final boss in the Botanica set per outland_
-- script_loader.cpp order (freywinn, laj, warp_splinter,
-- thorngrin_the_tender, commander_sarannis).
-- Entry: 17976 (NPC_COMMANDER_SARANNIS — the_botanica.h,
-- verifiable from the C++ sources; DATA_COMMANDER_
-- SARANNIS = 0 — instance-side constant, unbridgeable).
-- instance_the_botanica.cpp OnCreatureCreate maps NPC_
-- COMMANDER_SARANNIS to CommanderSarannisGUID (verified).
-- The creature_template ScriptName bindings are DB-side
-- (no TDB in this workspace).
-- Talk: SAY_AGGRO = 0 (pull — fired from the C++
-- JustEngagedWith, C++-exact), SAY_KILL = 1 (kill — the
-- C++ KilledUnit talks unconditionally, no player gate,
-- gargolmar precedent, C++-exact), SAY_ARCANE_RESONANCE
-- = 2 (arcane resonance — fired from the EVENT_ARCANE_
-- RESONANCE arm, C++-exact), SAY_ARCANE_DEVASTATION = 3
-- (arcane devastation — fired from the EVENT_ARCANE_
-- DEVASTATION arm, C++-exact), EMOTE_SUMMON = 4 (summon
-- — fired from the DamageTaken latch, C++-exact), SAY_
-- SUMMON = 5 (summon — fired from the DamageTaken latch,
-- C++-exact), SAY_DEATH = 6 (death — fired from the C++
-- JustDied, C++-exact).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 9 OnDamageTaken, 23 OnReset.
-- Timers via CreateLuaEvent (per-GUID scheduler pumps);
-- melee is engine-driven in Go (creature combat tick), like
-- C++ DoMeleeAttackIfReady. C++ DoCast default is triggered=
-- false (vaelastrasz convention).
-- Fight shape (C++-exact for all modeled arms): Sarannis
-- (17976): OnEnterCombat(1): per-GUID reset to the C++ ctor
-- / Initialize() values (arcane resonance 42.7s / arcane
-- devastation 15.2s / phase = true — the BossAI ctor DATA_
-- COMMANDER_SARANNIS = 0 arm is instance-blocked,
-- unbridgeable) + Talk(SAY_AGGRO) + 1s scheduler pump (a
-- port of UpdateAI in C++ arm order — 1s granularity exact
-- for all C++ timers; the !UpdateVictim early return
-- collapses into the pump; the UNIT_STATE_CASTING early-
-- return gates before and after the event loop have no
-- cast-state bridge — the arms fire unconditionally,
-- netherspite precedent). Pump:
-- 1. Arcane resonance arm — 42.7s init then 42.7s: Talk(SAY_
--    ARCANE_RESONANCE) + TRIGGERED DoCastVictim(SPELL_ARCANE_
--    RESONANCE 34794, true) (C++-exact), re-arm 42700 (C++-
--    exact).
-- 2. Arcane devastation arm — 15.2s init then {11s,19.2s}:
--    Talk(SAY_ARCANE_DEVASTATION) + TRIGGERED DoCastVictim
--    (SPELL_ARCANE_DEVASTATION 34799, true) (C++-exact),
--    re-arm {11000,19200} (C++-exact). Melee is engine-
--    driven.
-- OnDamageTaken(9): the C++ DamageTaken latch — HealthBelow
-- PctDamaged(50, damage) — health after this hit strictly
-- below 50% (majordomo / shahraz precedent): phase = false
-- + Talk(EMOTE_SUMMON) + Talk(SAY_SUMMON) (C++-exact). The
-- non-triggered DoCast(me, SPELL_SUMMON_REINFORCEMENTS
-- 34803) arm is a dummy spell whose only effect is the
-- unmodeled SpellScript's four SummonCreature calls —
-- the summons have no summon bridge (steamrigger precedent),
-- so the cast itself is unmodeled; only the talks and the
-- latch are bridged. The C++ JustSummoned override is the
-- unreachable summon bookkeeping — unmodeled.
-- OnTargetDied(3): Talk(SAY_KILL) unconditionally (no player
-- gate, C++-exact). OnDied(4): Talk(SAY_DEATH) + cleanup
-- (the _JustDied arm is instance-blocked). OnLeave
-- Combat(2)/OnReset(23): cancel the pump, drop per-GUID
-- state (the C++ Reset() observable remainder — the
-- Initialize() re-latch — lands on OnEnterCombat,
-- gargolmar precedent; the _Reset arm is instance-blocked;
-- Eluna's On_Reset fires ahead of OnDied and OnSpawn —
-- millhouse note — so both hooks land the same reset).
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI
-- shim (instance_the_botanica.cpp stays blocked on the
-- instance-script model; the BossAI ctor DATA_COMMANDER_
-- SARANNIS = 0 bookkeeping in Reset/JustEngagedWith/
-- JustDied skipped); no summon bridge — the SPELL_SUMMON_
-- REINFORCEMENTS dummy spell's four SummonCreature calls
-- (NPC_SUMMONED_BLOODWARDER_MENDER 20083 + two/three NPC_
-- SUMMONED_BLOODWARDER_RESERVIST 20078 at the
-- PosSummonReinforcements coordinates) and the C++
-- JustSummoned summon bookkeeping unmodeled (steamrigger
-- precedent); no SpellScript bridge — spell_commander_
-- sarannis_summon_reinforcements (HandleCast dummy-effect
-- summons, heroic fourth summon at position [3]) documented
-- only, not bridged; no cast-state bridge — the UpdateAI
-- UNIT_STATE_CASTING early-return gates unmodeled, the arms
-- fire unconditionally (netherspite precedent).

local SPELL_ARCANE_RESONANCE = 34794
local SPELL_ARCANE_DEVASTATION = 34799
local SPELL_SUMMON_REINFORCEMENTS = 34803

local SAY_AGGRO = 0
local SAY_KILL = 1
local SAY_ARCANE_RESONANCE = 2
local SAY_ARCANE_DEVASTATION = 3
local EMOTE_SUMMON = 4
local SAY_SUMMON = 5
local SAY_DEATH = 6

local ENTRY_COMMANDER_SARANNIS = 17976

local combatTimers = {}
local combatStates = {}

local function cancelCombatPump(guid)
    local id = combatTimers[guid]
    if id then
        RemoveEventById(id)
        combatTimers[guid] = nil
    end
end

-- C++ Reset() (called on evade): the observable remainder —
-- the Initialize() re-latch — lands nowhere; the re-latch
-- lands on OnEnterCombat (gargolmar precedent).
local function fullReset(guid)
    cancelCombatPump(guid)
    combatStates[guid] = nil
end

-- C++ UpdateAI in event arm order — 1s granularity exact
-- for all C++ timers; the !UpdateVictim early return
-- collapses into the pump; the UNIT_STATE_CASTING gates
-- have no cast-state bridge, so the arms fire
-- unconditionally (netherspite precedent).
local function combatTick(creature, guid)
    local st = combatStates[guid]
    if not st then
        return
    end

    -- 1. Arcane resonance arm: Talk(SAY_ARCANE_RESONANCE) +
    -- TRIGGERED DoCastVictim (C++-exact), re-arm 42700.
    if st.resonance <= 1000 then
        creature:Talk(SAY_ARCANE_RESONANCE)
        creature:CastSpell(nil, SPELL_ARCANE_RESONANCE, true)
        st.resonance = 42700
    else
        st.resonance = st.resonance - 1000
    end

    -- 2. Arcane devastation arm: Talk(SAY_ARCANE_DEVASTATION)
    -- + TRIGGERED DoCastVictim (C++-exact), re-arm {11s,19.2s}.
    if st.devastation <= 1000 then
        creature:Talk(SAY_ARCANE_DEVASTATION)
        creature:CastSpell(nil, SPELL_ARCANE_DEVASTATION, true)
        st.devastation = 11000 + math.random(0, 19200 - 11000)
    else
        st.devastation = st.devastation - 1000
    end
end

-- C++ ctor / Initialize() values (the whole schedule lands
-- on OnEnterCombat): arcane resonance 42.7s / arcane
-- devastation 15.2s / phase = true (the BossAI ctor DATA_
-- COMMANDER_SARANNIS = 0 arm is instance-blocked,
-- unbridgeable). The C++ JustEngagedWith talks SAY_AGGRO
-- (the BossAI arm is instance-blocked) — Talk(SAY_AGGRO)
-- (C++-exact).
RegisterCreatureEvent(ENTRY_COMMANDER_SARANNIS, 1, function(_, creature)
    local guid = creature:GetGUID()
    cancelCombatPump(guid)
    combatStates[guid] = {
        resonance = 42700,
        devastation = 15200,
        phase = true,
    }
    creature:Talk(SAY_AGGRO)
    combatTimers[guid] = CreateLuaEvent(function()
        combatTick(creature, guid)
    end, 1000, 0)
end)

-- C++ Reset() (called on evade): fullReset — see above.
RegisterCreatureEvent(ENTRY_COMMANDER_SARANNIS, 2, function(_, creature)
    fullReset(creature:GetGUID())
end)

-- C++ KilledUnit: Talk(SAY_KILL) unconditionally (no player
-- gate, C++-exact).
RegisterCreatureEvent(ENTRY_COMMANDER_SARANNIS, 3, function(_, creature)
    creature:Talk(SAY_KILL)
end)

-- C++ JustDied: Talk(SAY_DEATH) + cleanup (the _JustDied
-- arm is instance-blocked).
RegisterCreatureEvent(ENTRY_COMMANDER_SARANNIS, 4, function(_, creature)
    creature:Talk(SAY_DEATH)
    fullReset(creature:GetGUID())
end)

-- C++ DamageTaken: the phase latch — HealthBelowPctDamaged
-- (50, damage) — health after this hit strictly below 50%
-- (majordomo / shahraz precedent): phase = false + Talk(
-- EMOTE_SUMMON) + Talk(SAY_SUMMON) (C++-exact). The DoCast
-- (me, SPELL_SUMMON_REINFORCEMENTS) arm is unmodeled — the
-- dummy spell's only effect is the unmodeled SpellScript's
-- SummonCreature calls (no summon bridge, steamrigger
-- precedent).
RegisterCreatureEvent(ENTRY_COMMANDER_SARANNIS, 9, function(_, creature, attacker, damage)
    local guid = creature:GetGUID()
    local st = combatStates[guid]
    if not st then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    local afterHitPct = (creature:GetHealth() - damage) * 100 / maxHealth
    if st.phase and afterHitPct < 50 then
        st.phase = false
        creature:Talk(EMOTE_SUMMON)
        creature:Talk(SAY_SUMMON)
    end
end)

-- Eluna's On_Reset fires ahead of OnDied and OnSpawn, so
-- this lands the same Reset() reset as the evade hook.
RegisterCreatureEvent(ENTRY_COMMANDER_SARANNIS, 23, function(_, creature)
    fullReset(creature:GetGUID())
end)
