-- High Botanist Freywinn (The Botanica, Tempest Keep) --
-- Lua port of src/server/scripts/Outland/TempestKeep/
-- botanica/boss_high_botanist_freywinn.cpp (boss_high_
-- botanist_freywinn — the only CreatureScript AI class
-- AddSC_boss_high_botanist_freywinn registers; no
-- SpellScript/AuraScript scripts in this file).
-- First boss in the Botanica set per outland_script_loader.
-- cpp order (freywinn, laj, warp_splinter, thorngrin_the_
-- tender, commander_sarannis).
-- Entry: 17975 (NPC_HIGH_BOTANIST_FREYWINN — the_botanica.h:
-- 40, verifiable from the C++ sources; DATA_HIGH_BOTANIST_
-- FREYWINN = 1 — instance-side constant, unbridgeable).
-- instance_the_botanica.cpp OnCreatureCreate maps NPC_HIGH_
-- BOTANIST_FREYWINN to HighBotanistFreywinnGUID (verified).
-- The creature_template ScriptName bindings are DB-side (no
-- TDB in this workspace).
-- Talk: SAY_AGGRO = 0 (pull — fired from the C++
-- JustEngagedWith, C++-exact), SAY_KILL = 1 (kill — the C++
-- KilledUnit talks unconditionally, no player gate,
-- gargolmar precedent, C++-exact), SAY_TREE = 2 (tree form
-- — fired from the tree-form arm, C++-exact), SAY_SUMMON =
-- 3 (unused — never Talk()ed by the C++ AI), SAY_DEATH = 4
-- (death — fired from the C++ JustDied, C++-exact), SAY_OOC_
-- RANDOM = 5 (unused — never Talk()ed by the C++ AI;
-- millhouse SAY_ICEBLOCK precedent).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via
-- CreateLuaEvent (per-GUID scheduler pumps); melee is
-- engine-driven in Go (creature combat tick), like C++ DoMelee
-- AttackIfReady. C++ DoCast default is triggered=false
-- (vaelastrasz convention).
-- Fight shape (C++-exact for all modeled arms): Freywinn
-- (17975): OnEnterCombat(1): per-GUID reset to the C++ ctor /
-- Initialize() values (SummonSeedling 6000 / TreeForm 30000
-- / MoveFree true — the BossAI::JustEngagedWith arm is
-- instance-blocked, DATA_HIGH_BOTANIST_FREYWINN = 1
-- unbridgeable) + Talk(SAY_AGGRO) + 1s scheduler pump (a
-- port of UpdateAI in C++ arm order — 1s granularity exact
-- for all C++ timers; the !UpdateVictim early return
-- collapses into the pump). Pump:
-- 1. Tree-form arm (always runs first, C++ arm order): 30s
--    init then cyclic — fires every time TreeForm <= 1000:
--    Talk(SAY_TREE), enter tree form (MoveFree = false,
--    TreeForm = 75000), DoCast(me, SPELL_TRANQUILITY 34550,
--    true) and DoCast(me, SPELL_TREE_FORM 34551, true)
--    (TRIGGERED, C++-exact; self-cast convention), re-arm
--    75000 (C++-exact). The DoCast(me, SPELL_SUMMON_FRAYER
--    34557, true) arm has no summon bridge (steamrigger
--    precedent) — the frayers never exist in this model, so
--    the cast itself is skipped and only the talk is
--    modeled. The IsNonMeleeSpellCast/InterruptNonMeleeSpells
--    gate has no cast-state bridge — the arm fires
--    unconditionally (netherspite precedent); the
--    RemoveAllAuras arm has no aura bridge; the MoveIdle()
--    arm has no movement bridge (omor precedent).
-- 2. Tree-form exit machine (only while !MoveFree): the
--    dead-frayer scan over the summons list is unreachable
--    (no summon bridge — the summons list is always empty),
--    so the only reachable exit is the forced arm — when
--    TreeForm - 30000 <= 1000, exit tree form (MoveFree =
--    true; the summons.DespawnAll() is a no-op, the
--    InterruptNonMeleeSpells/RemoveAllAuras arms have no
--    bridges, the MoveChase(victim) arm has no movement
--    bridge), C++-exact for the reachable model. At 1s
--    granularity this lands 44 ticks after entry (TreeForm
--    75000 -> 31000), and the entry arm re-fires 30 ticks
--    after exit (TreeForm 31000 -> 1000): the cycle is 30s
--    normal / 44s tree form, matching the C++ timers. The
--    arm runs in C++ order — after the tree-form arm — and
--    returns early, so the seedling arm is skipped while in
--    tree form (the commented-out HasAura tree-form early
--    return is dead code, not modeled).
-- 3. Seedling arm (only while MoveFree): 6s init then 6s —
--    random 0..3 plant cast on self (SPELL_PLANT_WHITE
--    34759 / GREEN 34761 / BLUE 34762 / RED 34763, non-
--    triggered DoCast on self, C++-exact), re-arm 6000
--    (C++-exact). The timer freezes while in tree form
--    (C++-exact, early return above). Melee is
--    engine-driven.
-- OnTargetDied(3): Talk(SAY_KILL) (C++ talks
-- unconditionally — no player gate, C++-exact). OnDied(4):
-- Talk(SAY_DEATH) + cleanup (the _JustDied arm is instance-
-- blocked). OnLeaveCombat(2)/OnReset(23): cancel the pump,
-- drop per-GUID state (the C++ Reset() observable remainder
-- — summons.DespawnAll() is a no-op without summons, the
-- Initialize() re-latch lands on OnEnterCombat, gargolmar
-- precedent; the _Reset arm is instance-blocked).
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI
-- shim (instance_the_botanica.cpp stays blocked on the
-- instance-script model; the BossAI ctor DATA_HIGH_BOTANIST_
-- FREYWINN = 1 bookkeeping in Reset/JustEngagedWith/JustDied
-- skipped); no summon bridge — the SPELL_SUMMON_FRAYER
-- 34557 frayer summons, the JustSummoned/SummonedCreature
-- Despawn tracking arms and the DeadAddsCount dead-frayer
-- scan arm unmodeled (steamrigger precedent) — the forced
-- TreeForm_Timer - 30000 exit is the only reachable exit
-- path; no aura bridge — the tree-form entry/exit
-- RemoveAllAuras arms unmodeled; no movement bridge — the
-- tree-form MoveIdle() arm and the exit MoveChase(victim)
-- arm unmodeled (omor precedent); no cast-state bridge —
-- the tree-form IsNonMeleeSpellCast/InterruptNonMeleeSpells
-- arms unmodeled (netherspite precedent); the unused say
-- enums SAY_SUMMON = 3 / SAY_OOC_RANDOM = 5 never fire in
-- the C++ AI — not bridged (millhouse unused-spells
-- precedent); the ScriptData "SD%Complete: 90 / some
-- strange visual related to tree form" caveat is upstream,
-- documented, not bridged; no SpellScript/AuraScript
-- scripts in this file.

local SPELL_TRANQUILITY = 34550
local SPELL_TREE_FORM = 34551
local SPELL_PLANT_WHITE = 34759
local SPELL_PLANT_GREEN = 34761
local SPELL_PLANT_BLUE = 34762
local SPELL_PLANT_RED = 34763

local SAY_AGGRO = 0
local SAY_KILL = 1
local SAY_TREE = 2
local SAY_DEATH = 4

local ENTRY_FREYWINN = 17975

local combatTimers = {}
local combatStates = {}

local function cancelCombatPump(guid)
    local id = combatTimers[guid]
    if id then
        RemoveEventById(id)
        combatTimers[guid] = nil
    end
end

-- C++ Reset() (called on evade): summons.DespawnAll() is a
-- no-op without a summon bridge; the observable remainder —
-- the Initialize() re-latch — lands on OnEnterCombat
-- (gargolmar precedent).
local function fullReset(guid)
    cancelCombatPump(guid)
    combatStates[guid] = nil
end

-- C++ DoSummonSeedling: rand32() % 4 — one of the four
-- plant spells, non-triggered DoCast on self (C++-exact).
local function doSummonSeedling(creature)
    local pick = math.random(0, 3)
    if pick == 0 then
        creature:CastSpell(creature, SPELL_PLANT_WHITE)
    elseif pick == 1 then
        creature:CastSpell(creature, SPELL_PLANT_GREEN)
    elseif pick == 2 then
        creature:CastSpell(creature, SPELL_PLANT_BLUE)
    else
        creature:CastSpell(creature, SPELL_PLANT_RED)
    end
end

-- C++ UpdateAI in arm order — 1s granularity exact for all
-- C++ timers; the !UpdateVictim early return collapses into
-- the pump. Only bridgeable arms are modeled (the summon /
-- aura / movement / cast-state arms are bridge-blocked —
-- see deviations).
local function combatTick(creature, guid)
    local st = combatStates[guid]
    if not st then
        return
    end

    -- 1. Tree-form arm (always first): 30s init, then
    -- re-fires cyclically — entry and re-entry are the same
    -- C++ arm.
    if st.treeForm <= 1000 then
        creature:Talk(SAY_TREE)
        -- The DoCast(me, SPELL_SUMMON_FRAYER, true) arm has
        -- no summon bridge — skipped (steamrigger
        -- precedent); the tranquility / tree-form casts are
        -- TRIGGERED in the C++ (C++-exact), modeled here as
        -- plain self casts. The IsNonMeleeSpellCast gate /
        -- InterruptNonMeleeSpells have no cast-state bridge,
        -- the RemoveAllAuras arm has no aura bridge, the
        -- MoveIdle() arm has no movement bridge.
        creature:CastSpell(creature, SPELL_TRANQUILITY)
        creature:CastSpell(creature, SPELL_TREE_FORM)
        st.moveFree = false
        st.treeForm = 75000
    else
        st.treeForm = st.treeForm - 1000
    end

    -- 2. Tree-form exit machine: the dead-frayer summons
    -- scan is unreachable (no summon bridge), so the only
    -- reachable exit is the forced TreeForm - 30000 arm —
    -- C++-exact for the reachable model. Exiting returns
    -- early, skipping the seedling arm (C++-exact).
    if not st.moveFree then
        if st.treeForm - 30000 <= 1000 then
            -- summons.DespawnAll() is a no-op; the
            -- InterruptNonMeleeSpells / RemoveAllAuras /
            -- MoveChase arms have no bridges — the
            -- observable remainder is MoveFree = true.
            st.moveFree = true
        end
        return
    end

    -- 3. Seedling arm: 6s init then 6s, frozen while in
    -- tree form (the early return above — C++-exact).
    if st.summonSeedling <= 1000 then
        doSummonSeedling(creature)
        st.summonSeedling = 6000
    else
        st.summonSeedling = st.summonSeedling - 1000
    end
end

-- C++ ctor / Initialize() values (the whole schedule lands
-- on OnEnterCombat — omor precedent): SummonSeedling 6000 /
-- TreeForm 30000 / MoveFree true (MoveCheck 1000 and
-- DeadAddsCount are tree-form machine values that reset on
-- entry).
RegisterCreatureEvent(ENTRY_FREYWINN, 1, function(_, creature)
    local guid = creature:GetGUID()
    cancelCombatPump(guid)
    combatStates[guid] = {
        summonSeedling = 6000,
        treeForm = 30000,
        moveFree = true,
    }
    creature:Talk(SAY_AGGRO)
    combatTimers[guid] = CreateLuaEvent(function()
        combatTick(creature, guid)
    end, 1000, 0)
end)

-- C++ Reset() (called on evade): _Reset() is instance-
-- blocked; the observable remainder is fullReset.
RegisterCreatureEvent(ENTRY_FREYWINN, 2, function(_, creature)
    fullReset(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_FREYWINN, 3, function(_, creature)
    -- C++ KilledUnit: Talk(SAY_KILL) unconditionally — no
    -- player gate (gargolmar precedent, C++-exact).
    creature:Talk(SAY_KILL)
end)

RegisterCreatureEvent(ENTRY_FREYWINN, 4, function(_, creature)
    creature:Talk(SAY_DEATH)
    fullReset(creature:GetGUID())
end)

-- Eluna's On_Reset fires ahead of OnDied and OnSpawn, so
-- this lands the same Reset() re-latch as the evade hook.
RegisterCreatureEvent(ENTRY_FREYWINN, 23, function(_, creature)
    fullReset(creature:GetGUID())
end)
