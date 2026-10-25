-- Void Reaver (The Eye, Tempest Keep) --
-- Lua port of src/server/scripts/Outland/TempestKeep/Eye/
-- boss_void_reaver.cpp (boss_void_reaver — the only CreatureScript
-- AI class AddSC_boss_void_reaver registers; no SpellScript /
-- AuraScript scripts in this file).
-- Third boss in The Eye set per outland_script_loader.cpp order
-- (boss_alar, boss_kaelthas, boss_void_reaver, boss_high_
-- astromancer_solarian); instance_the_eye.cpp stays blocked on
-- the instance-script model.
-- Entry: NPC_VOID_REAVER = 19516 (wowhead-TBC-verified — the C++
-- sources carry no NPC_ constant for Void Reaver; DATA_VOID_
-- REAVER = 3 is instance-side, unbridgeable). The creature_
-- template ScriptName bindings are DB-side (no TDB in this
-- workspace).
-- Talk: SAY_AGGRO = 0 (engage — OnEnterCombat, C++-exact),
-- SAY_SLAY = 1 (kill — C++ KilledUnit talks unconditionally, no
-- player gate, gargolmar precedent, C++-exact), SAY_DEATH = 2
-- (death — OnDied, C++-exact), SAY_POUNDING = 3 (pounding arm —
-- fired after the DoCastVictim, C++-exact).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset (no DamageTaken override —
-- no event 9).
-- Timers via CreateLuaEvent (per-GUID scheduler pump); melee is
-- engine-driven in Go (creature combat tick), like C++ DoMelee-
-- AttackIfReady. C++ DoCast default is triggered=false
-- (vaelastrasz convention): creature:CastSpell(nil, spell) =
-- DoCastVictim, creature:CastSpell(creature, spell) = DoCast
-- (me).
-- Fight shape (C++-exact for all modeled arms): Void Reaver
-- (19516): OnEnterCombat(1): per-GUID reset to the C++ ctor /
-- Initialize() values (Enraged = false — the BossAI ctor DATA_
-- VOID_REAVER = 3 arm is instance-blocked, unbridgeable) +
-- Talk(SAY_AGGRO) + 1s scheduler pump; all four C++ event
-- timers start in JustEngagedWith (alar engagement-schedule
-- precedent): EVENT_POUNDING 15000, EVENT_ARCANE_ORB 3000,
-- EVENT_KNOCK_AWAY 30000, EVENT_BERSERK 600000 (10min).
-- Pump (C++ UpdateAI event order):
-- 1. Pounding 34162: 15s init then 15s — non-triggered
--    DoCastVictim (C++-exact) + Talk(SAY_POUNDING), re-arm 15s
--    regardless (C++-exact). The C++ UNIT_STATE_CASTING
--    early-return gates have no live cast-state read bridge (IsCasting is constant-false — Go casts are fire-and-forget) — the arm
--    fires unconditionally (zereketh precedent).
-- 2. Arcane orb 34172: 3s init then 3s — me->CastSpell(target)
--    (non-triggered, C++-exact), re-arm 3s regardless
--    (C++-exact). Target: C++ builds the list from the threat
--    manager (TYPEID_PLAYER, alive, >18yd from the boss),
--    random pick, empty -> me->GetVictim(). No threat-list
--    bridge — the pick runs over alive players in the instance
--    (vashj playersInInstance convention) gated on distance
--    2D > 18yd (C++ IsWithinDist(me, 18, false)), empty -> creature:GetVictim() (the C++ fallback).
--    The C++ `if (target)` nil guard is ported (C++-exact).
-- 3. Knock away 25778: 30s init then 30s — non-triggered
--    DoCastVictim (C++-exact), re-arm 30s regardless
--    (C++-exact). The 25% threat drop (C++ GetThreat /
--    ModifyThreatByPercent) is PORTED 2026-10-24 (victim gate + cut
--    after the cast).
-- 4. Berserk 27680: 600000 one-shot — Enraged latch (C++
--    ctor/Reset() Initialize() sets false, C++-exact), DoCast
--    (me) self-cast (C++-exact); one-shot arm, no re-arm.
-- OnDied(4): Talk(SAY_DEATH) (C++-exact); the C++ DoZoneInCombat
-- and BossAI _JustDied arms are instance-blocked, unmodeled.
-- OnReset(23) / OnLeaveCombat(2): per-GUID state cleared (the
-- C++ Reset() re-latches Enraged and the BossAI _Reset() arm is
-- instance-blocked).

local SPELL_POUNDING = 34162
local SPELL_ARCANE_ORB = 34172
local SPELL_KNOCK_AWAY = 25778
local SPELL_BERSERK = 27680

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2
local SAY_POUNDING = 3

local ARCANE_ORB_MIN_DIST = 18

local ENTRY_VOID_REAVER = 19516

local timers = {}
local states = {}

local function cancelPump(guid)
    local id = timers[guid]
    if id then
        RemoveEventById(id)
        timers[guid] = nil
    end
end

local function resetBoss(guid)
    cancelPump(guid)
    states[guid] = nil
end

-- C++ ctor / Reset() Initialize(): Enraged = false. The C++
-- event timers all start from JustEngagedWith (alar engagement-
-- schedule precedent).
local function freshState()
    return {
        pounding = 15000,
        arcaneOrb = 3000,
        knockAway = 30000,
        berserk = 600000,
        enraged = false,
    }
end

-- C++ EVENT_ARCANE_ORB target leg: random alive player >18yd 2D
-- from the boss, drawn from the threat list (boss_void_reaver.cpp:115-125:
-- GetUnsortedThreatList filtered TYPEID_PLAYER + IsAlive + !IsWithinDist(18)),
-- empty -> me->GetVictim() fallback. creature:GetAITargets() is the bridged
-- threat list (lua_creature_events.go); the old "no threat-list bridge"
-- instance-player approx was retired 2026-10-23.
local function pickArcaneOrbTarget(creature)
    local found = {}
    for _, unit in ipairs(creature:GetAITargets()) do
        if unit:GetObjectType() == "Player" and not unit:IsDead()
                and not creature:IsWithinDist2d(unit, ARCANE_ORB_MIN_DIST) then
            found[#found + 1] = unit
        end
    end
    if #found == 0 then
        return creature:GetVictim()
    end
    return found[math.random(#found)]
end

local function bossTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end

    -- Pounding 34162: 15s init then 15s — non-triggered
    -- DoCastVictim + Talk(SAY_POUNDING) (C++-exact), re-arm
    -- 15s regardless (C++-exact). The C++ UNIT_STATE_CASTING
    -- gates have no live cast-state read bridge (IsCasting is constant-false — Go casts are fire-and-forget) — fires unconditionally.
    if st.pounding <= 1000 then
        creature:CastSpell(nil, SPELL_POUNDING)
        creature:Talk(SAY_POUNDING)
        st.pounding = 15000
    else
        st.pounding = st.pounding - 1000
    end

    -- Arcane orb 34172: 3s init then 3s — non-triggered
    -- me->CastSpell(target) (C++-exact), re-arm 3s regardless
    -- (C++-exact). The C++ `if (target)` nil guard is ported
    -- (C++-exact).
    if st.arcaneOrb <= 1000 then
        local target = pickArcaneOrbTarget(creature)
        if target then
            creature:CastSpell(target, SPELL_ARCANE_ORB)
        end
        st.arcaneOrb = 3000
    else
        st.arcaneOrb = st.arcaneOrb - 1000
    end

    -- Knock away 25778: 30s init then 30s — non-triggered
    -- DoCastVictim (C++-exact), re-arm 30s regardless
    -- (C++-exact). The 25% threat drop (GetThreat /
    -- ModifyThreatByPercent) is PORTED 2026-10-24 (victim gate + cut after the cast).
    if st.knockAway <= 1000 then
        creature:CastSpell(nil, SPELL_KNOCK_AWAY)
        local victim = creature:GetVictim()
        if victim and creature:GetThreat(victim) ~= 0 then
            creature:ModifyThreatByPercent(victim, -25)
        end
        st.knockAway = 30000
    else
        st.knockAway = st.knockAway - 1000
    end

    -- Berserk 27680: 600000 one-shot — Enraged latch,
    -- DoCast(me) self-cast (C++-exact); one-shot, no re-arm.
    if st.berserk <= 1000 and not st.enraged then
        creature:CastSpell(creature, SPELL_BERSERK)
        st.enraged = true
    else
        st.berserk = st.berserk - 1000
    end
end

RegisterCreatureEvent(ENTRY_VOID_REAVER, 1, function(_, creature)
    local guid = creature:GetGUID()
    resetBoss(guid)
    states[guid] = freshState()
    creature:Talk(SAY_AGGRO)
    timers[guid] = CreateLuaEvent(function()
        bossTick(creature, guid)
    end, 1000, 0)
end)

RegisterCreatureEvent(ENTRY_VOID_REAVER, 2, function(_, creature)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_VOID_REAVER, 3, function(_, creature)
    -- C++ KilledUnit talks unconditionally — no player gate
    -- (gargolmar precedent, C++-exact).
    creature:Talk(SAY_SLAY)
end)

RegisterCreatureEvent(ENTRY_VOID_REAVER, 4, function(_, creature)
    creature:Talk(SAY_DEATH)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_VOID_REAVER, 23, function(_, creature)
    resetBoss(creature:GetGUID())
end)
