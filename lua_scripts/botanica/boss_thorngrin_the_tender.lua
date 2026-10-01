-- Thorngrin the Tender (The Botanica, Tempest Keep) --
-- Lua port of src/server/scripts/Outland/TempestKeep/
-- botanica/boss_thorngrin_the_tender.cpp (boss_thorngrin_
-- the_tender — the only CreatureScript AI class AddSC_boss_
-- thorngrin_the_tender registers; no SpellScript/
-- AuraScript scripts in this file).
-- Fourth boss in the Botanica set per outland_script_loader.
-- cpp order (freywinn, laj, warp_splinter, thorngrin_the_
-- tender, commander_sarannis).
-- Entry: 17978 (NPC_THORNGRIN_THE_TENDER — the_botanica.h,
-- verifiable from the C++ sources; DATA_THORNGRIN_THE_
-- TENDER = 2 — instance-side constant, unbridgeable).
-- instance_the_botanica.cpp OnCreatureCreate maps NPC_
-- THORNGRIN_THE_TENDER (verified). The creature_template
-- ScriptName bindings are DB-side (no TDB in this
-- workspace).
-- Talk: SAY_AGGRO = 0 (pull — fired from the C++
-- JustEngagedWith, C++-exact), SAY_20_PERCENT_HP = 1
-- (20% HP — fired from the C++ DamageTaken latch, C++-
-- exact), SAY_KILL = 2 (kill — the C++ KilledUnit talks
-- unconditionally, no player gate, gargolmar precedent,
-- C++-exact), SAY_CAST_SACRIFICE = 3 (sacrifice — fired
-- from the EVENT_SACRIFICE arm, C++-exact), SAY_50_PERCENT_
-- HP = 4 (50% HP — fired from the C++ DamageTaken latch,
-- C++-exact), SAY_CAST_HELLFIRE = 5 (hellfire — fired from
-- the EVENT_HELLFIRE arm, C++-exact), SAY_DEATH = 6 (death
-- — fired from the C++ JustDied, C++-exact), EMOTE_ENRAGE
-- = 7 (enrage — fired from the EVENT_ENRAGE arm, C++-
-- exact).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 9 OnDamageTaken, 23 OnReset.
-- Timers via CreateLuaEvent (per-GUID scheduler pumps);
-- melee is engine-driven in Go (creature combat tick), like
-- C++ DoMeleeAttackIfReady. C++ DoCast default is triggered=
-- false (vaelastrasz convention).
-- Fight shape (C++-exact for all modeled arms): Thorngrin
-- (17978): OnEnterCombat(1): per-GUID reset to the C++ ctor
-- / Initialize() values (sacrifice 5.7s / hellfire 18s /
-- enrage 12s / phase1 = true / phase2 = true — the BossAI
-- ctor DATA_THORNGRIN_THE_TENDER = 2 arm is instance-
-- blocked, unbridgeable; the non-heroic hellfire schedule
-- is modeled — the heroic {17.4s,19.3s} schedule has no
-- difficulty bridge, thespia precedent) + Talk(SAY_AGGRO) +
-- 1s scheduler pump (a port of UpdateAI in C++ event arm
-- order — 1s granularity exact for all C++ timers; the
-- !UpdateVictim early return collapses into the pump; the
-- UNIT_STATE_CASTING early-return gates before and after
-- the event loop have no cast-state bridge — the arms fire
-- unconditionally, netherspite precedent). Pump:
-- 1. Sacrifice arm — 5.7s init then 29.4s: target =
--    SelectTarget(Random, 1) — random alive player excluding
--    the current victim (skyriss / thespia precedent); if a
--    target exists (the arm is skipped when the C++ picks
--    none, C++-exact): Talk(SAY_CAST_SACRIFICE) +
--    TRIGGERED DoCast(target, SPELL_SACRIFICE 34661, true)
--    (C++-exact); re-arm 29400 regardless of whether a
--    target was picked (the C++ ScheduleEvent sits outside
--    the target if, C++-exact).
-- 2. Hellfire arm — 18s init then 18s (non-heroic; the
--    heroic {17.4s,19.3s} re-arm has no difficulty bridge,
--    unmodeled — thespia precedent): Talk(SAY_CAST_HELLFIRE)
--    + TRIGGERED DoCastVictim(SPELL_HELLFIRE 34659, true)
--    (C++-exact), re-arm 18s (C++-exact).
-- 3. Enrage arm — 12s init then 33s: Talk(EMOTE_ENRAGE) +
--    non-triggered DoCast(me, SPELL_ENRAGE 34670) (C++-
--    exact), re-arm 33000 (C++-exact). Melee is engine-
--    driven.
-- OnDamageTaken(9): the C++ DamageTaken latches — 50% HP
-- (phase1 latch — the C++ checks _phase1 first, C++ arm
-- order): HealthBelowPctDamaged(50, damage) — health after
-- this hit strictly below 50% (majordomo / shahraz
-- precedent) — phase1 = false + Talk(SAY_50_PERCENT_HP);
-- 20% HP (phase2 latch): health after this hit strictly
-- below 20% — phase2 = false + Talk(SAY_20_PERCENT_HP). A
-- single hit dropping the boss from above 50% to below 20%
-- fires both talks in C++ arm order.
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
-- instance-script model; the BossAI ctor DATA_THORNGRIN_
-- THE_TENDER = 2 bookkeeping in Reset/JustEngagedWith/
-- JustDied skipped); no difficulty bridge — the heroic
-- EVENT_HELLFIRE {17400,19300} init / re-arm schedule
-- unmodeled (thespia precedent); no cast-state bridge —
-- the UpdateAI UNIT_STATE_CASTING early-return gates
-- unmodeled, the arms fire unconditionally (netherspite
-- precedent); no SpellScript/AuraScript scripts in this
-- file.

local SPELL_SACRIFICE = 34661
local SPELL_HELLFIRE = 34659
local SPELL_ENRAGE = 34670

local SAY_AGGRO = 0
local SAY_20_PERCENT_HP = 1
local SAY_KILL = 2
local SAY_CAST_SACRIFICE = 3
local SAY_50_PERCENT_HP = 4
local SAY_CAST_HELLFIRE = 5
local SAY_DEATH = 6
local EMOTE_ENRAGE = 7

local ENTRY_THORNGRIN_THE_TENDER = 17978

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

-- C++ SelectTarget(Random, 1, 0.0f, true): a random alive
-- player in the instance excluding the current victim
-- (thespia / skyriss precedent), player-only. The C++
-- skips the sacrifice arm when it picks no target.
local function randomNonVictimPlayer(creature)
    local victim = creature:GetVictim()
    local candidates = {}
    for _, p in ipairs(playersInInstance(creature)) do
        if not victim or p:GetGUID() ~= victim:GetGUID() then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
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

    -- 1. Sacrifice arm: TRIGGERED DoCast on a random
    -- non-victim player (C++-exact); re-arm 29400 regardless
    -- of target pick (the C++ ScheduleEvent is outside the
    -- target if).
    if st.sacrifice <= 1000 then
        local target = randomNonVictimPlayer(creature)
        if target then
            creature:Talk(SAY_CAST_SACRIFICE)
            creature:CastSpell(target, SPELL_SACRIFICE, true)
        end
        st.sacrifice = 29400
    else
        st.sacrifice = st.sacrifice - 1000
    end

    -- 2. Hellfire arm: Talk(SAY_CAST_HELLFIRE) +
    -- TRIGGERED DoCastVictim (C++-exact), re-arm 18s (the
    -- heroic {17.4s,19.3s} schedule has no difficulty
    -- bridge, unmodeled).
    if st.hellfire <= 1000 then
        creature:Talk(SAY_CAST_HELLFIRE)
        creature:CastSpell(nil, SPELL_HELLFIRE, true)
        st.hellfire = 18000
    else
        st.hellfire = st.hellfire - 1000
    end

    -- 3. Enrage arm: Talk(EMOTE_ENRAGE) + non-triggered
    -- DoCast on self (C++-exact), re-arm 33000.
    if st.enrage <= 1000 then
        creature:Talk(EMOTE_ENRAGE)
        creature:CastSpell(creature, SPELL_ENRAGE)
        st.enrage = 33000
    else
        st.enrage = st.enrage - 1000
    end
end

-- C++ ctor / Initialize() values (the whole schedule lands
-- on OnEnterCombat): sacrifice 5.7s / hellfire 18s /
-- enrage 12s / phase1 = true / phase2 = true (non-heroic
-- hellfire schedule modeled — the heroic {17.4s,19.3s}
-- schedule has no difficulty bridge, thespia precedent).
-- The C++ JustEngagedWith talks SAY_AGGRO (the BossAI arm is
-- instance-blocked) — Talk(SAY_AGGRO) (C++-exact).
RegisterCreatureEvent(ENTRY_THORNGRIN_THE_TENDER, 1, function(_, creature)
    local guid = creature:GetGUID()
    cancelCombatPump(guid)
    combatStates[guid] = {
        sacrifice = 5700,
        hellfire = 18000,
        enrage = 12000,
        phase1 = true,
        phase2 = true,
    }
    creature:Talk(SAY_AGGRO)
    combatTimers[guid] = CreateLuaEvent(function()
        combatTick(creature, guid)
    end, 1000, 0)
end)

-- C++ Reset() (called on evade): fullReset — see above.
RegisterCreatureEvent(ENTRY_THORNGRIN_THE_TENDER, 2, function(_, creature)
    fullReset(creature:GetGUID())
end)

-- C++ KilledUnit: Talk(SAY_KILL) unconditionally (no player
-- gate, C++-exact).
RegisterCreatureEvent(ENTRY_THORNGRIN_THE_TENDER, 3, function(_, creature)
    creature:Talk(SAY_KILL)
end)

-- C++ JustDied: Talk(SAY_DEATH) + cleanup (the _JustDied
-- arm is instance-blocked).
RegisterCreatureEvent(ENTRY_THORNGRIN_THE_TENDER, 4, function(_, creature)
    creature:Talk(SAY_DEATH)
    fullReset(creature:GetGUID())
end)

-- C++ DamageTaken: the phase latches in C++ arm order (50%
-- first, then 20%) — HealthBelowPctDamaged(pct, damage) is
-- health after this hit strictly below pct% (majordomo /
-- shahraz precedent).
RegisterCreatureEvent(ENTRY_THORNGRIN_THE_TENDER, 9, function(_, creature, attacker, damage)
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
    if st.phase1 and afterHitPct < 50 then
        st.phase1 = false
        creature:Talk(SAY_50_PERCENT_HP)
    end
    if st.phase2 and afterHitPct < 20 then
        st.phase2 = false
        creature:Talk(SAY_20_PERCENT_HP)
    end
end)

-- Eluna's On_Reset fires ahead of OnDied and OnSpawn, so
-- this lands the same Reset() reset as the evade hook.
RegisterCreatureEvent(ENTRY_THORNGRIN_THE_TENDER, 23, function(_, creature)
    fullReset(creature:GetGUID())
end)
