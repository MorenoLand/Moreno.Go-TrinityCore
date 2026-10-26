-- Pathaleon the Calculator (The Mechanar) --
-- Lua port of src/server/scripts/Outland/TempestKeep/
-- Mechanar/boss_pathaleon_the_calculator.cpp
-- (boss_pathaleon_the_calculator + npc_nether_wraith — the
-- two CreatureScript AI classes
-- AddSC_boss_pathaleon_the_calculator registers).
-- Fourth unit of the Mechanar block per
-- outland_script_loader.cpp order (iron_hand, gyrokill,
-- nethermancer_sepethrea, pathaleon, capacitus,
-- instance_mechanar).
-- Entries: 19220 (Pathaleon), 21062 (Nether Wraith — C++
-- Creatures enum). No NPC_ constants in the C++ tree;
-- DB-side ScriptName bindings, entries per standard TBC
-- creature data (not verifiable from this workspace).
-- DATA_PATHALEON_THE_CALCULATOR = 4 is instance-side,
-- unbridgeable. SD%Complete is 50 ("Event missing") —
-- the pre-fight wave event is instance/data-driven and
-- unmodeled.
-- Talk: SAY_AGGRO = 0 (engage — OnEnterCombat, C++-exact),
-- SAY_DOMINATION = 1 (BEFORE the domination cast —
-- C++-exact order), SAY_SUMMON = 2 (summon timer —
-- C++-exact: fires after the summon loop), SAY_ENRAGE = 3
-- (frenzy latch — C++-exact),
-- SAY_SLAY = 4 (kill — OnTargetDied, C++-exact, no roll),
-- SAY_DEATH = 5 (death — OnDied, C++-exact).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat,
-- 3 OnTargetDied, 4 OnDied, 9 OnDamageTaken, 23 OnReset.
-- Timers via CreateLuaEvent (per-GUID scheduler pump); melee
-- is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. C++ DoCast default is triggered=false
-- (vaelastrasz convention): creature:CastSpell(nil, spell,
-- triggered) = DoCastVictim, creature:CastSpell(creature,
-- spell) = DoCast(me), creature:CastSpell(target, spell) =
-- DoCast(target).
-- Fight shape (C++-exact for all modeled arms):
-- 1. Summon 30s init, randtime 30-45s re-arm: the 3x
--    SelectTarget(Random,0)-gated SummonCreature(NPC_NETHER_WRAITH
--    21062, boss pos, o=0, TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT=3,
--    25s) legs are PORTED 2026-10-24 (creature:SummonCreature bridge
--    live — entry/coords/type/duration all C++-verifiable); the
--    Wraith->AI()->AttackStart(target) relay has no bridge
--    (keleseth precedent) — the wraiths engage via the engine's own
--    aggro (moroes precedent). Talk(SAY_SUMMON) fires unconditionally
--    after the loop (C++-exact). The summon spell IDs 35285-35288
--    are "Not scripted" in the C++ (no SpellScript) and are never
--    cast by the AI anyway.
-- 2. Mana Tap 36021: randtime 12-20s init, randtime 14-22s
--    re-arm — TRIGGERED DoCastVictim (C++-exact).
-- 3. Arcane Torrent 36022: randtime 16-25s init, randtime
--    12-18s re-arm — TRIGGERED DoCastVictim (C++-exact).
-- 4. Domination 35280: randtime 25-40s init, randtime
--    25-30s re-arm — Talk(SAY_DOMINATION) then TRIGGERED
--    DoCastVictim (C++-exact order).
-- 5. Arcane Explosion 15453: randtime 8-13s init, randtime
--    10-14s re-arm — TRIGGERED DoCastVictim (C++-exact).
--    The C++ comment says "Heroic only" but the arm has NO
--    IsHeroic gate in the code — the port fires it always
--    (C++-exact, not IsHeroic-gated; iron-hand reverse
--    precedent).
-- 6. Frenzy 36992: OnDamageTaken(9) — HealthBelowPctDamaged
--    (20, damage) && !HasAura(SPELL_FRENZY) — post-damage
--    latch via (GetHealth() - damage) < 20% max (sarannis /
--    majordomo precedent) + creature:HasAura (bridged,
--    lua_objects.go) — DoCast(me, SPELL_FRENZY) self-cast +
--    Talk(SAY_ENRAGE) (C++-exact).
-- OnDied(4): Talk(SAY_DEATH) (C++-exact); the BossAI
-- _JustDied arm is instance-blocked, unmodeled.
-- OnReset(23) / OnLeaveCombat(2): per-GUID state cleared (the
-- C++ Reset() / BossAI _Reset() arms are instance-blocked).
-- The C++ UNIT_STATE_CASTING early-return (top of UpdateAI)
-- and the post-switch cast-state re-check have no cast-state
-- bridge — the arms fire unconditionally (zereketh
-- precedent).
--
-- npc_nether_wraith (entry 21062) — UpdateAI bridgeable arms:
-- arcane missiles 35034: urand 1-4s init, urand 5-10s
-- re-arm — SelectTarget(Random,1) then DoCast(target) else
-- DoCastVictim (C++-exact shape): the C++ random pick skips
-- the top-threat target, but Lua picks any alive player in
-- the instance (gruul/vashj precedent) — documented
-- deviation; detonation 35058: 20s — non-triggered
-- DoCast(me) self-cast, one-shot latch (C++-exact); Die_Timer
-- 2200ms after detonation — DespawnOrUnsummon via
-- creature:Despawn() (midnight precedent). Melee is
-- engine-driven.

local SPELL_MANA_TAP = 36021
local SPELL_ARCANE_TORRENT = 36022
local SPELL_DOMINATION = 35280
local SPELL_ARCANE_EXPLOSION = 15453
local SPELL_FRENZY = 36992

local SPELL_ARCANE_MISSILES = 35034
local SPELL_DETONATION = 35058

local SAY_AGGRO = 0
local SAY_DOMINATION = 1
local SAY_SUMMON = 2
local SAY_ENRAGE = 3
local SAY_SLAY = 4
local SAY_DEATH = 5

local ENTRY_PATHALEON = 19220
local ENTRY_NETHER_WRAITH = 21062

local timers = {}
local bossStates = {}
local wraithStates = {}

local function cancelPump(guid)
    local id = timers[guid]
    if id then
        RemoveEventById(id)
        timers[guid] = nil
    end
end

local function resetBoss(guid)
    cancelPump(guid)
    bossStates[guid] = nil
end

local function resetWraith(guid)
    cancelPump(guid)
    wraithStates[guid] = nil
end

-- C++ SelectTarget pools are threat-list pools (UnitAI.h), not the
-- instance roster: Random, 0 draws any threat-list unit alive, and
-- Random, 1 skips the first MaxThreat entry before filters.
local function randomThreatTarget(creature)
    local candidates = {}
    for _, unit in ipairs(creature:GetAITargets()) do
        if not unit:IsDead() then
            candidates[#candidates + 1] = unit
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

local function randomThreatTargetPastHead(creature)
    local victim = creature:GetVictim()
    local victimGuid = nil
    if victim ~= nil then
        victimGuid = victim:GetGUID()
    end
    local targets = creature:GetAITargets()
    local candidates = {}
    local aliveSeen = 0
    for _, unit in ipairs(targets) do
        local skip = false
        if victimGuid ~= nil then
            skip = unit:GetGUID() == victimGuid
        elseif not unit:IsDead() then
            aliveSeen = aliveSeen + 1
            skip = aliveSeen == 1
        end
        if not skip
                and not unit:IsDead() then
            candidates[#candidates + 1] = unit
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ JustEngagedWith schedules: SUMMON 30s, MANA_TAP
-- randtime 12-20s, ARCANE_TORRENT randtime 16-25s,
-- DOMINATION randtime 25-40s, ARCANE_EXPLOSION randtime
-- 8-13s (mechanar engagement-schedule precedent).
local function freshBossState()
    return {
        summon = 30000,
        manaTap = math.random(12000, 20000),
        arcaneTorrent = math.random(16000, 25000),
        domination = math.random(25000, 40000),
        arcaneExplosion = math.random(8000, 13000),
    }
end

local function bossTick(creature, guid)
    local st = bossStates[guid]
    if not st then
        return
    end

    -- Summon timer: 3x SelectTarget(Random,0)-gated SummonCreature(
    -- NPC_NETHER_WRAITH 21062, boss pos, o=0,
    -- TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT=3, 25s) — C++ arm order
    -- (boss_pathaleon_the_calculator.cpp:124-133: loop, then Talk);
    -- the Wraith->AI()->AttackStart(target) relay has no bridge
    -- (keleseth precedent) — the wraiths engage via the engine's own
    -- aggro (moroes precedent); Talk(SAY_SUMMON) fires unconditionally
    -- after the loop (C++-exact); re-arm randtime 30-45s.
    if st.summon <= 1000 then
        for _ = 1, 3 do
            if randomThreatTarget(creature) then
                creature:SummonCreature(ENTRY_NETHER_WRAITH,
                    creature:GetX(), creature:GetY(), creature:GetZ(), 0, 3, 25000)
            end
        end
        creature:Talk(SAY_SUMMON)
        st.summon = math.random(30000, 45000)
    else
        st.summon = st.summon - 1000
    end

    -- Mana Tap 36021: randtime 12-20s init, randtime 14-22s
    -- re-arm — TRIGGERED DoCastVictim (C++-exact).
    if st.manaTap <= 1000 then
        creature:CastSpell(nil, SPELL_MANA_TAP, true)
        st.manaTap = math.random(14000, 22000)
    else
        st.manaTap = st.manaTap - 1000
    end

    -- Arcane Torrent 36022: randtime 16-25s init, randtime
    -- 12-18s re-arm — TRIGGERED DoCastVictim (C++-exact).
    if st.arcaneTorrent <= 1000 then
        creature:CastSpell(nil, SPELL_ARCANE_TORRENT, true)
        st.arcaneTorrent = math.random(12000, 18000)
    else
        st.arcaneTorrent = st.arcaneTorrent - 1000
    end

    -- Domination 35280: randtime 25-40s init, randtime
    -- 25-30s re-arm — Talk(SAY_DOMINATION) BEFORE the cast,
    -- then TRIGGERED DoCastVictim (C++-exact order).
    if st.domination <= 1000 then
        creature:Talk(SAY_DOMINATION)
        creature:CastSpell(nil, SPELL_DOMINATION, true)
        st.domination = math.random(25000, 30000)
    else
        st.domination = st.domination - 1000
    end

    -- Arcane Explosion 15453: randtime 8-13s init, randtime
    -- 10-14s re-arm — TRIGGERED DoCastVictim (C++-exact).
    -- The "Heroic only" C++ comment has no IsHeroic gate in
    -- the code; the port fires it always (C++-exact).
    if st.arcaneExplosion <= 1000 then
        creature:CastSpell(nil, SPELL_ARCANE_EXPLOSION, true)
        st.arcaneExplosion = math.random(10000, 14000)
    else
        st.arcaneExplosion = st.arcaneExplosion - 1000
    end
end

RegisterCreatureEvent(ENTRY_PATHALEON, 1, function(_, creature)
    local guid = creature:GetGUID()
    resetBoss(guid)
    bossStates[guid] = freshBossState()
    creature:Talk(SAY_AGGRO)
    timers[guid] = CreateLuaEvent(function()
        bossTick(creature, guid)
    end, 1000, 0)
end)

RegisterCreatureEvent(ENTRY_PATHALEON, 2, function(_, creature)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_PATHALEON, 3, function(_, creature)
    -- C++ KilledUnit talks unconditionally (C++-exact).
    creature:Talk(SAY_SLAY)
end)

RegisterCreatureEvent(ENTRY_PATHALEON, 4, function(_, creature)
    creature:Talk(SAY_DEATH)
    resetBoss(creature:GetGUID())
end)

-- C++ DamageTaken: HealthBelowPctDamaged(20, damage) — post-
-- damage health strictly below 20% (sarannis / majordomo
-- precedent) — && !HasAura(SPELL_FRENZY): DoCast(me,
-- SPELL_FRENZY) self-cast + Talk(SAY_ENRAGE) (C++-exact).
-- The aura itself is the latch; no Lua state needed. Event 9
-- fires before damage is applied, so GetHealth() is the
-- pre-damage value; nothing is returned (no damage rewrite).
RegisterCreatureEvent(ENTRY_PATHALEON, 9, function(_, creature, attacker, damage)
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    local afterHit = creature:GetHealth() - damage
    if afterHit < math.floor(maxHealth * 20 / 100)
            and not creature:HasAura(SPELL_FRENZY) then
        creature:CastSpell(creature, SPELL_FRENZY)
        creature:Talk(SAY_ENRAGE)
    end
end)

RegisterCreatureEvent(ENTRY_PATHALEON, 23, function(_, creature)
    resetBoss(creature:GetGUID())
end)

-- npc_nether_wraith: C++ Initialize() timers — arcane
-- missiles urand 1-4s, detonation 20s, die 2200ms
-- (mechanar add-schedule precedent).
local function freshWraithState()
    return {
        arcaneMissiles = math.random(1000, 4000),
        detonation = 20000,
        detonationFired = false,
        die = 2200,
    }
end

local function wraithTick(creature, guid)
    local st = wraithStates[guid]
    if not st then
        return
    end

    -- Arcane Missiles 35034: urand 1-4s init, urand 5-10s
    -- re-arm — DoCast(target) on a random pick else
    -- DoCastVictim (C++-exact shape). C++ SelectTarget
    -- (Random,1) skips the top-threat target; Lua picks any
    -- threat-list unit alive past the head (C++-exact).
    if st.arcaneMissiles <= 1000 then
        local target = randomThreatTargetPastHead(creature)
        if target then
            creature:CastSpell(target, SPELL_ARCANE_MISSILES)
        else
            creature:CastSpell(nil, SPELL_ARCANE_MISSILES)
        end
        st.arcaneMissiles = math.random(5000, 10000)
    else
        st.arcaneMissiles = st.arcaneMissiles - 1000
    end

    -- Detonation 35058: 20s — non-triggered DoCast(me)
    -- self-cast, one-shot latch (C++-exact).
    if not st.detonationFired then
        if st.detonation <= 1000 then
            creature:CastSpell(creature, SPELL_DETONATION)
            st.detonationFired = true
        else
            st.detonation = st.detonation - 1000
        end
    end

    -- Die_Timer 2200ms after detonation — DespawnOrUnsummon
    -- via creature:Despawn() (midnight precedent).
    if st.detonationFired then
        if st.die <= 1000 then
            creature:Despawn()
            resetWraith(guid)
        else
            st.die = st.die - 1000
        end
    end
end

RegisterCreatureEvent(ENTRY_NETHER_WRAITH, 1, function(_, creature)
    local guid = creature:GetGUID()
    resetWraith(guid)
    wraithStates[guid] = freshWraithState()
    timers[guid] = CreateLuaEvent(function()
        wraithTick(creature, guid)
    end, 1000, 0)
end)

RegisterCreatureEvent(ENTRY_NETHER_WRAITH, 2, function(_, creature)
    resetWraith(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_NETHER_WRAITH, 4, function(_, creature)
    resetWraith(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_NETHER_WRAITH, 23, function(_, creature)
    resetWraith(creature:GetGUID())
end)
