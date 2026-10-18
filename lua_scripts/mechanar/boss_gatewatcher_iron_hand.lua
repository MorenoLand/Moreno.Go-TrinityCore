-- Gatewatcher Iron-Hand (The Mechanar) --
-- Lua port of src/server/scripts/Outland/TempestKeep/
-- Mechanar/boss_gatewatcher_ironhand.cpp
-- (boss_gatewatcher_iron_hand — the only CreatureScript AI
-- class AddSC_boss_gatewatcher_iron_hand registers).
-- First unit of the Mechanar block per
-- outland_script_loader.cpp order (iron_hand, gyrokill,
-- nethermancer_sepethrea, pathaleon, capacitus,
-- instance_mechanar).
-- Entry: 19710 (wowhead-verified — mechanar.h carries no
-- NPC_ constant for Iron-Hand; the creature_template
-- ScriptName bindings are DB-side, no TDB in this
-- workspace). DATA_GATEWATCHER_IRON_HAND = 1 is
-- instance-side, unbridgeable.
-- Talk: SAY_AGGRO = 0 (engage — OnEnterCombat, C++-exact),
-- SAY_HAMMER = 1 (50% roll with the jackhammer arm —
-- KilledUnit-style roll_chance_i(50), C++-exact), SAY_SLAY =
-- 2 (50% roll on kill — OnTargetDied, C++-exact), SAY_DEATH
-- = 3 (death — OnDied, C++-exact), EMOTE_HAMMER = 4 (fires
-- before the jackhammer cast, C++-exact).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat,
-- 3 OnTargetDied, 4 OnDied, 23 OnReset.
-- Timers via CreateLuaEvent (per-GUID scheduler pump); melee
-- is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. C++ DoCast default is triggered=false
-- (vaelastrasz convention): creature:CastSpell(nil, spell,
-- triggered) = DoCastVictim, creature:CastSpell(creature,
-- spell) = DoCast(me).
-- Fight shape (C++-exact for all modeled arms):
-- 1. Stream of Machine Fluid 35311: 55s init then randtime
--    35-50s — TRIGGERED DoCastVictim (C++-exact).
-- 2. Jackhammer 35327: 45s init then 30s — EMOTE_HAMMER
--    Talk + TRIGGERED DoCastVictim (C++-exact) + 50% Talk
--    (SAY_HAMMER); re-arm 30s regardless (C++-exact).
--    35326 is SPELL_HAMMER_PUNCH, a dead enum the C++ never
--    casts.
-- 3. Shadow Power 35322: 25s init then randtime 20-28s —
--    non-triggered DoCast(me) self-cast (C++-exact). The
--    heroic variants H_SPELL_SHADOW_POWER 39193 /
--    H_SPELL_JACKHAMMER 39194 exist in the C++ enum but are
--    NEVER used by UpdateAI — the port fires the normal
--    spells always (C++-exact, not IsHeroic-gated).
-- OnDied(4): Talk(SAY_DEATH) (C++-exact); the BossAI
-- _JustDied arm is instance-blocked, unmodeled.
-- OnReset(23) / OnLeaveCombat(2): per-GUID state cleared (the
-- C++ Reset() / BossAI _Reset() arms are instance-blocked).
-- The C++ UNIT_STATE_CASTING early-return (top of UpdateAI)
-- and the post-switch cast-state re-check have no cast-state
-- bridge — the arms fire unconditionally (zereketh
-- precedent).

local SPELL_STREAM_OF_MACHINE_FLUID = 35311
local SPELL_JACKHAMMER = 35327
local SPELL_SHADOW_POWER = 35322

local SAY_AGGRO = 0
local SAY_HAMMER = 1
local SAY_SLAY = 2
local SAY_DEATH = 3
local EMOTE_HAMMER = 4

local ENTRY_GATEWATCHER_IRON_HAND = 19710

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

-- C++ JustEngagedWith schedules: STREAM 55s, JACKHAMMER 45s,
-- SHADOW_POWER 25s (iron_hand engagement-schedule
-- precedent, matching alar/void_reaver).
local function freshState()
    return {
        stream = 55000,
        jackhammer = 45000,
        shadowPower = 25000,
    }
end

local function bossTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end

    -- Stream of Machine Fluid 35311: 55s init then randtime
    -- 35-50s — TRIGGERED DoCastVictim (C++-exact).
    if st.stream <= 1000 then
        creature:CastSpell(nil, SPELL_STREAM_OF_MACHINE_FLUID, true)
        st.stream = math.random(35000, 50000)
    else
        st.stream = st.stream - 1000
    end

    -- Jackhammer 35327: 45s init then 30s — EMOTE_HAMMER Talk
    -- + TRIGGERED DoCastVictim + 50% Talk(SAY_HAMMER)
    -- (C++-exact order); re-arm 30s regardless (C++-exact).
    if st.jackhammer <= 1000 then
        creature:Talk(EMOTE_HAMMER)
        creature:CastSpell(nil, SPELL_JACKHAMMER, true)
        if math.random(2) == 1 then
            creature:Talk(SAY_HAMMER)
        end
        st.jackhammer = 30000
    else
        st.jackhammer = st.jackhammer - 1000
    end

    -- Shadow Power 35322: 25s init then randtime 20-28s —
    -- non-triggered DoCast(me) self-cast (C++-exact); the
    -- H_SPELL_* heroic variants are never fired by the C++,
    -- so the port never gates on IsHeroic.
    if st.shadowPower <= 1000 then
        creature:CastSpell(creature, SPELL_SHADOW_POWER)
        st.shadowPower = math.random(20000, 28000)
    else
        st.shadowPower = st.shadowPower - 1000
    end
end

RegisterCreatureEvent(ENTRY_GATEWATCHER_IRON_HAND, 1, function(_, creature)
    local guid = creature:GetGUID()
    resetBoss(guid)
    states[guid] = freshState()
    creature:Talk(SAY_AGGRO)
    timers[guid] = CreateLuaEvent(function()
        bossTick(creature, guid)
    end, 1000, 0)
end)

RegisterCreatureEvent(ENTRY_GATEWATCHER_IRON_HAND, 2, function(_, creature)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_GATEWATCHER_IRON_HAND, 3, function(_, creature)
    -- C++ KilledUnit rolls 50% before talking (C++-exact).
    if math.random(2) == 1 then
        creature:Talk(SAY_SLAY)
    end
end)

RegisterCreatureEvent(ENTRY_GATEWATCHER_IRON_HAND, 4, function(_, creature)
    creature:Talk(SAY_DEATH)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_GATEWATCHER_IRON_HAND, 23, function(_, creature)
    resetBoss(creature:GetGUID())
end)
