-- Nethermancer Sepethrea (The Mechanar) --
-- Lua port of src/server/scripts/Outland/TempestKeep/
-- Mechanar/boss_nethermancer_sepethrea.cpp
-- (boss_nethermancer_sepethrea + npc_ragin_flames — the two
-- CreatureScript AI classes AddSC_boss_nethermancer_sepethrea
-- registers).
-- Third unit of the Mechanar block per
-- outland_script_loader.cpp order (iron_hand, gyrokill,
-- nethermancer_sepethrea, pathaleon, capacitus,
-- instance_mechanar).
-- Entries: 17271 (Sepethrea), 22323 (Ragin Flames) — no NPC_
-- constants in the C++ tree; DB-side ScriptName bindings,
-- entries per standard TBC creature data (not verifiable
-- from this workspace; SDComment notes the initial summons
-- need adjustments). DATA_NETHERMANCER_SEPRETHREA = 3 is
-- instance-side, unbridgeable.
-- Sepethrea Talk: SAY_AGGRO = 0 (engage — OnEnterCombat,
-- C++-exact), SAY_SUMMON = 1 (with the summon cast —
-- C++-exact), SAY_DRAGONS_BREATH = 2 (50% roll with the
-- dragon's-breath arm — roll_chance_i(50), C++-exact),
-- SAY_SLAY = 3 (kill — OnTargetDied, C++-exact, no roll),
-- SAY_DEATH = 4 (death — OnDied, C++-exact).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat,
-- 3 OnTargetDied, 4 OnDied, 23 OnReset.
-- Timers via CreateLuaEvent (per-GUID scheduler pump); melee
-- is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. C++ DoCast default is triggered=false
-- (vaelastrasz convention): creature:CastSpell(nil, spell,
-- triggered) = DoCastVictim, creature:CastSpell(creature,
-- spell) = DoCast(me).
-- Sepethrea fight shape (C++-exact for all modeled arms):
-- 1. Summon Ragin Flames 35275: on engage — non-triggered
--    DoCast(who) + Talk(SAY_SUMMON) (C++-exact). The actual
--    summoning is the spell's own effect (DB-side); the
--    "Not scripted" comment on the spell means no SpellScript
--    in the core, not that the cast arm is skipped.
-- 2. Frost Attack 35263: randtime 7-10s init and re-arm —
--    TRIGGERED DoCastVictim (C++-exact).
-- 3. Arcane Blast 35314: randtime 12-18s init, 15s re-arm —
--    TRIGGERED DoCastVictim (C++-exact).
-- 4. Dragon's Breath 35250: randtime 18-22s init, randtime
--    12-22s re-arm — TRIGGERED DoCastVictim (C++-exact) +
--    50% Talk(SAY_DRAGONS_BREATH) after the cast
--    (C++-exact order).
-- 5. Knockback 37317: randtime 22-28s init, randtime 15-25s
--    re-arm — TRIGGERED DoCastVictim (C++-exact).
-- 6. Solarburn 35267: 30s init and re-arm — TRIGGERED
--    DoCastVictim (C++-exact).
-- OnDied(4): Talk(SAY_DEATH) (C++-exact); the BossAI
-- _JustDied arm is instance-blocked, unmodeled.
-- OnReset(23) / OnLeaveCombat(2): per-GUID state cleared (the
-- C++ Reset() / BossAI _Reset() arms are instance-blocked).
-- The C++ UNIT_STATE_CASTING early-return (top of UpdateAI)
-- and the post-switch cast-state re-check have no cast-state
-- bridge — the arms fire unconditionally (zereketh
-- precedent).
--
-- npc_ragin_flames (entry 22323) — UpdateAI bridgeable arms:
-- inferno 35268: 10s init and re-arm — non-triggered
-- DoCastVictim (C++-exact; "Not scripted" on the spell means
-- no SpellScript, not a skipped cast); fire tail 35278:
-- 500ms init and re-arm — non-triggered DoCast(me)
-- self-cast (C++-exact) on a 500ms pump (shattered-
-- executioner precedent): the old 1000ms pump fired fire
-- tail every 1000ms, half the C++ cadence. The rest of the C++ has no bridges:
-- Reset()'s ApplySpellImmune legs PORTED 2026-10-22 (0, IMMUNITY_
-- DAMAGE=3, SPELL_SCHOOL_MASK_MAGIC=126 / NORMAL=1, true — C++ :181-182,
-- creature:ApplySpellImmune bridge, lua_creature_events.go:860;
-- the stale "stay unbridged" claim is retracted); the SetSpeedRate(
-- MOVE_RUN, DUNGEON_MODE(0.5f,0.7f)) arm is ported via
-- creature:SetSpeedRate), the Check_Timer
-- instance-boss-state despawn check is instance-blocked, and
-- the onlyonce MoveChase(random-target) leg has no
-- MoveChase bridge (a random pick alone would be dead code —
-- black_stalker precedent, not half-ported). The flame's
-- melee is engine-driven.

local SPELL_SUMMON_RAGIN_FLAMES = 35275
local SPELL_FROST_ATTACK = 35263
local SPELL_ARCANE_BLAST = 35314
local SPELL_DRAGONS_BREATH = 35250
local SPELL_KNOCKBACK = 37317
local SPELL_SOLARBURN = 35267

local SPELL_INFERNO = 35268
local SPELL_FIRE_TAIL = 35278

local SAY_AGGRO = 0
local SAY_SUMMON = 1
local SAY_DRAGONS_BREATH = 2
local SAY_SLAY = 3
local SAY_DEATH = 4

local ENTRY_SEPRETHREA = 17271
local ENTRY_RAGIN_FLAMES = 22323

local timers = {}
local bossStates = {}
local flameStates = {}

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

local function resetFlame(guid)
    cancelPump(guid)
    flameStates[guid] = nil
end

-- C++ JustEngagedWith schedules: FROST randtime 7-10s,
-- ARCANE_BLAST randtime 12-18s, DRAGONS_BREATH randtime
-- 18-22s, KNOCKBACK randtime 22-28s, SOLARBURN 30s
-- (mechanar engagement-schedule precedent).
local function freshBossState()
    return {
        frost = math.random(7000, 10000),
        arcaneBlast = math.random(12000, 18000),
        dragonsBreath = math.random(18000, 22000),
        knockback = math.random(22000, 28000),
        solarburn = 30000,
    }
end

local function bossTick(creature, guid)
    local st = bossStates[guid]
    if not st then
        return
    end

    -- Frost Attack 35263: randtime 7-10s init and re-arm —
    -- TRIGGERED DoCastVictim (C++-exact).
    if st.frost <= 1000 then
        creature:CastSpell(nil, SPELL_FROST_ATTACK, true)
        st.frost = math.random(7000, 10000)
    else
        st.frost = st.frost - 1000
    end

    -- Arcane Blast 35314: randtime 12-18s init, 15s re-arm —
    -- TRIGGERED DoCastVictim (C++-exact).
    if st.arcaneBlast <= 1000 then
        creature:CastSpell(nil, SPELL_ARCANE_BLAST, true)
        st.arcaneBlast = 15000
    else
        st.arcaneBlast = st.arcaneBlast - 1000
    end

    -- Dragon's Breath 35250: randtime 18-22s init, randtime
    -- 12-22s re-arm — TRIGGERED DoCastVictim + 50%
    -- Talk(SAY_DRAGONS_BREATH) after the cast (C++-exact
    -- order).
    if st.dragonsBreath <= 1000 then
        creature:CastSpell(nil, SPELL_DRAGONS_BREATH, true)
        st.dragonsBreath = math.random(12000, 22000)
        if math.random(2) == 1 then
            creature:Talk(SAY_DRAGONS_BREATH)
        end
    else
        st.dragonsBreath = st.dragonsBreath - 1000
    end

    -- Knockback 37317: randtime 22-28s init, randtime 15-25s
    -- re-arm — TRIGGERED DoCastVictim (C++-exact).
    if st.knockback <= 1000 then
        creature:CastSpell(nil, SPELL_KNOCKBACK, true)
        st.knockback = math.random(15000, 25000)
    else
        st.knockback = st.knockback - 1000
    end

    -- Solarburn 35267: 30s init and re-arm — TRIGGERED
    -- DoCastVictim (C++-exact).
    if st.solarburn <= 1000 then
        creature:CastSpell(nil, SPELL_SOLARBURN, true)
        st.solarburn = 30000
    else
        st.solarburn = st.solarburn - 1000
    end
end

RegisterCreatureEvent(ENTRY_SEPRETHREA, 1, function(_, creature)
    local guid = creature:GetGUID()
    resetBoss(guid)
    bossStates[guid] = freshBossState()
    -- C++ JustEngagedWith: Talk(SAY_AGGRO), DoCast(who,
    -- SPELL_SUMMON_RAGIN_FLAMES), Talk(SAY_SUMMON) — the
    -- summon cast is non-triggered DoCastVictim (C++-exact).
    creature:Talk(SAY_AGGRO)
    creature:CastSpell(nil, SPELL_SUMMON_RAGIN_FLAMES)
    creature:Talk(SAY_SUMMON)
    timers[guid] = CreateLuaEvent(function()
        bossTick(creature, guid)
    end, 1000, 0)
end)

RegisterCreatureEvent(ENTRY_SEPRETHREA, 2, function(_, creature)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_SEPRETHREA, 3, function(_, creature)
    -- C++ KilledUnit talks unconditionally (C++-exact).
    creature:Talk(SAY_SLAY)
end)

RegisterCreatureEvent(ENTRY_SEPRETHREA, 4, function(_, creature)
    creature:Talk(SAY_DEATH)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_SEPRETHREA, 23, function(_, creature)
    resetBoss(creature:GetGUID())
end)

-- npc_ragin_flames: C++ Initialize() timers — inferno 10s,
-- flame 500ms (mechanar add-schedule precedent).
local function freshFlameState()
    return {
        inferno = 10000,
        flame = 500,
    }
end

local function flameTick(creature, guid)
    local st = flameStates[guid]
    if not st then
        return
    end

    -- Inferno 35268: 10s init and re-arm — non-triggered
    -- DoCastVictim (C++-exact).
    if st.inferno <= 500 then
        creature:CastSpell(nil, SPELL_INFERNO)
        st.inferno = 10000
    else
        st.inferno = st.inferno - 500
    end

    -- Fire Tail 35278: 500ms init and re-arm —
    -- non-triggered DoCast(me) self-cast (C++-exact).
    -- 500ms pump: the 500ms init fires on the first tick, as C++.
    if st.flame <= 500 then
        creature:CastSpell(creature, SPELL_FIRE_TAIL)
        st.flame = 500
    else
        st.flame = st.flame - 500
    end
end

RegisterCreatureEvent(ENTRY_RAGIN_FLAMES, 1, function(_, creature)
    local guid = creature:GetGUID()
    resetFlame(guid)
    flameStates[guid] = freshFlameState()
    timers[guid] = CreateLuaEvent(function()
        flameTick(creature, guid)
    end, 500, 0)
end)

RegisterCreatureEvent(ENTRY_RAGIN_FLAMES, 2, function(_, creature)
    resetFlame(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_RAGIN_FLAMES, 4, function(_, creature)
    resetFlame(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_RAGIN_FLAMES, 23, function(_, creature)
    resetFlame(creature:GetGUID())
    -- C++ npc_ragin_flames::Reset() :181-182: ApplySpellImmune(0,
    -- IMMUNITY_DAMAGE, SPELL_SCHOOL_MASK_MAGIC=126 / NORMAL=1, true).
    creature:ApplySpellImmune(0, 3, 126, true)
    creature:ApplySpellImmune(0, 3, 1, true)
    -- C++ :183: SetSpeedRate(MOVE_RUN, DUNGEON_MODE(0.5f, 0.7f)).
    creature:SetSpeedRate(1, creature:IsHeroic() and 0.7 or 0.5)
end)
