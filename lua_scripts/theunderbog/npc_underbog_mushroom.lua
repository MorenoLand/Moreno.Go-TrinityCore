-- Underbog Mushroom (The Underbog, Hungarfen encounter) — Lua
-- port of src/server/scripts/Outland/CoilfangReservoir/
-- TheUnderbog/boss_hungarfen.cpp (npc_underbog_mushroom — the
-- second AI class AddSC_boss_hungarfen registers; the mushroom
-- is spawned by the boss's summon arm, which has no summon
-- bridge and stays unmodeled — steamrigger mechanic precedent;
-- no other creature/GO scripts in this file).
-- Entry 17990 is the file's own SummonCreature(17990, ...)
-- constant (verified externally — wowhead npc=17990/underbog-
-- mushroom, kalithresh distiller precedent); the_underbog.h
-- defines NO NPC_ entries and instance_the_underbog.cpp is a
-- placeholder with NO OnCreatureCreate mappings at all. The
-- creature_template ScriptName bindings are DB-side (no TDB in
-- this workspace).
-- The AI never engages (MoveInLineOfSight/AttackStart/JustEng
-- agedWith are all empty overrides) — no combat events
-- registered. Eluna creature events: 5 OnSpawn, 2
-- OnLeaveCombat, 4 OnDied, 23 OnReset. Timers via
-- CreateLuaEvent (per-GUID scheduler pump). C++ DoCast with
-- the triggered=true arg is a triggered self-cast (gruul
-- reverberation precedent); C++ DoCast default is triggered=
-- false (vaelastrasz convention). Fight shape (C++-exact for
-- all modeled arms): npc_underbog_mushroom (17990): OnSpawn(5)
-- — the engine fires this (lua_creature_events.go; the frozen-
-- core port in boss_ahune.lua is the precedent): per-GUID
-- reset to the C++ Initialize() values (stop false / grow 0 /
-- shrink 20000) + the C++ Reset self casts — triggered
-- DoCastSelf SPELL_PUTRID_MUSHROOM 31690 + triggered DoCastSelf
-- SPELL_SPORE_CLOUD 34168 (C++-exact arm order) + 1s scheduler
-- pump (a port of UpdateAI — 1s granularity exact for the 3s
-- grow timer and the 20s shrink timer; the Stop early return
-- collapses into the pump). Pump: grow 31698 non-triggered
-- self-cast — fires immediately on the first tick (C++ Grow_
-- Timer=0) then every 3s (C++-exact); shrink 20s -> Stop=true
-- (the me->RemoveAurasDueToSpell(SPELL_GROW 31698) arm has no
-- aura-removal bridge, documented only) — the pump is
-- cancelled and per-GUID state dropped, ending the grow
-- machine (timer bookkeeping exact). OnDied(4): cleanup. OnLea
-- veCombat(2)/OnReset(23): cleanup (the C++ Reset arms are
-- covered by OnSpawn — the mushroom never engages, so evade
-- Reset is unreachable).
-- Deliberate deviations (all await engine bridges): no aura-
-- removal bridge — the shrink-time RemoveAurasDueToSpell arm
-- unmodeled (the Stop=true arm is modeled as pump cancel);
-- no instance-script model — instance_the_underbog.cpp is a
-- placeholder and stays blocked on the instance-script model;
-- no summon bridge — the boss's SummonCreature(17990) arm
-- unmodeled, so these mushrooms spawn only via DB/world
-- placement or engine summon paths outside this port; no
-- SpellScript/AuraScript scripts in this file.

local SPELL_PUTRID_MUSHROOM = 31690
local SPELL_SPORE_CLOUD = 34168
local SPELL_GROW = 31698

local ENTRY_UNDERBOG_MUSHROOM = 17990

local timers = {}
local states = {}

local function cancelPump(guid)
    local id = timers[guid]
    if id then
        RemoveEventById(id)
        timers[guid] = nil
    end
end

local function resetMushroom(guid)
    cancelPump(guid)
    states[guid] = nil
end

-- C++ Initialize() values: Stop=false, Grow_Timer=0,
-- Shrink_Timer=20000.
local function freshState()
    return {
        stop = false,
        grow = 0,
        shrink = 20000,
    }
end

local function mushroomTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end

    -- Grow 31698: fires immediately (C++ Grow_Timer=0), then
    -- every 3s — non-triggered self-cast (C++ DoCast default,
    -- vaelastrasz convention).
    if st.grow <= 1000 then
        creature:CastSpell(nil, SPELL_GROW)
        st.grow = 3000
    else
        st.grow = st.grow - 1000
    end

    -- Shrink 20s: C++ RemoveAurasDueToSpell(SPELL_GROW) + Stop
    -- =true. The aura-removal arm has no bridge (documented
    -- only); the Stop arm is modeled as pump cancel (timer
    -- bookkeeping exact).
    if st.shrink <= 1000 then
        resetMushroom(guid)
    else
        st.shrink = st.shrink - 1000
    end
end

RegisterCreatureEvent(ENTRY_UNDERBOG_MUSHROOM, 5, function(_, creature)
    local guid = creature:GetGUID()
    resetMushroom(guid)
    states[guid] = freshState()
    -- C++ Reset arms, fired on spawn (frozen-core precedent):
    -- triggered self casts of putrid mushroom and spore cloud
    -- (C++-exact arm order).
    creature:CastSpell(nil, SPELL_PUTRID_MUSHROOM, true)
    creature:CastSpell(nil, SPELL_SPORE_CLOUD, true)
    timers[guid] = CreateLuaEvent(function()
        mushroomTick(creature, guid)
    end, 1000, 0)
end)

RegisterCreatureEvent(ENTRY_UNDERBOG_MUSHROOM, 2, function(_, creature)
    resetMushroom(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_UNDERBOG_MUSHROOM, 4, function(_, creature)
    resetMushroom(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_UNDERBOG_MUSHROOM, 23, function(_, creature)
    resetMushroom(creature:GetGUID())
end)
