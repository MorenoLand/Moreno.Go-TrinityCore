-- Warp Splinter (The Botanica, Tempest Keep) --
-- Lua port of src/server/scripts/Outland/TempestKeep/
-- botanica/boss_warp_splinter.cpp (boss_warp_splinter and
-- npc_warp_splinter_treant — the two CreatureScript AI
-- classes AddSC_boss_warp_splinter registers; no SpellScript/
-- AuraScript scripts in this file).
-- Third boss in the Botanica set per outland_script_loader.
-- cpp order (freywinn, laj, warp_splinter, thorngrin_the_
-- tender, commander_sarannis).
-- Entry: 17977 (NPC_WARP_SPLINTER — the_botanica.h,
-- verifiable from the C++ sources; DATA_WARP_SPLINTER = 4
-- — instance-side constant, unbridgeable). instance_the_
-- botanica.cpp OnCreatureCreate maps NPC_WARP_SPLINTER to
-- WarpSplinterGUID (verified). The creature_template
-- ScriptName bindings are DB-side (no TDB in this
-- workspace).
-- Talk: SAY_AGGRO = 0 (pull — fired from the C++
-- JustEngagedWith, C++-exact), SAY_SLAY = 1 (kill — the C++
-- KilledUnit talks unconditionally, no player gate,
-- gargolmar precedent, C++-exact), SAY_SUMMON = 2 (summon
-- treants — fired at the end of the C++ SummonTreants(),
-- C++-exact), SAY_DEATH = 3 (death — fired from the C++
-- JustDied, C++-exact).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via
-- CreateLuaEvent (per-GUID scheduler pumps); melee is
-- engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. C++ DoCast default is triggered=
-- false (vaelastrasz convention).
-- Fight shape (C++-exact for all modeled arms): Warp
-- Splinter (17977): OnEnterCombat(1): per-GUID reset to the
-- C++ ctor / Initialize() values (War Stomp {25s,40s} /
-- Arcane Volley {8s,20s} / Summon Treants 45s — the BossAI
-- ctor DATA_WARP_SPLINTER = 4 arm is instance-blocked,
-- unbridgeable) + Talk(SAY_AGGRO) + 1s scheduler pump (a
-- port of UpdateAI in C++ arm order — 1s granularity exact
-- for all C++ timers; the !UpdateVictim early return
-- collapses into the pump). Pump:
-- 1. War stomp 34716 — {25s,40s} init then {25s,40s}:
--    non-triggered DoCastVictim (C++-exact), re-arm
--    {25s,40s} regardless (C++-exact).
-- 2. Arcane volley 36705 — {8s,20s} init then {20s,35s}:
--    non-triggered DoCastVictim (C++-exact), re-arm
--    {20s,35s} regardless (C++-exact).
-- 3. Summon treants 34727 — 45s init then 45s: the six
--    me->SummonCreature(CREATURE_TREANT 19949, treant_pos
--    [i]..., TEMPSUMMON_TIMED_OR_CORPSE_DESPAWN 25s) casts
--    have no summon bridge (steamrigger precedent) — only
--    Talk(SAY_SUMMON) is modeled — re-arm 45s (C++-exact).
--    Melee is engine-driven.
-- OnTargetDied(3): Talk(SAY_SLAY) unconditionally (no
-- player gate, C++-exact). OnDied(4): Talk(SAY_DEATH) +
-- cleanup (the _JustDied arm is instance-blocked). OnLeave
-- Combat(2)/OnReset(23): cancel the pump, drop per-GUID
-- state (the C++ Reset() observable remainder —
-- Initialize() re-latch + me->SetSpeedRate(MOVE_RUN, 0.7f),
-- no speed bridge — lands nowhere; the Initialize()
-- re-latch lands on OnEnterCombat, gargolmar precedent;
-- the _Reset arm is instance-blocked; Eluna's On_Reset
-- fires ahead of OnDied and OnSpawn — millhouse note — so
-- both hooks land the same reset).
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI
-- shim (instance_the_botanica.cpp stays blocked on the
-- instance-script model; the BossAI ctor DATA_WARP_SPLINTER
-- = 4 bookkeeping in Reset/JustEngagedWith/JustDied
-- skipped); no summon bridge — the six SummonTreants
-- SummonCreature calls unmodeled (steamrigger precedent);
-- no movement bridge — the treant MoveFollow(Warp) arm and
-- the boss-side SetSpeedRate(MOVE_RUN, 0.7f) unmodeled
-- (omor precedent); no cross-creature bridge — the treant
-- WarpGuid / ObjectAccessor::GetUnit lookup unmodeled; no
-- cast-bp0 bridge — the treant's SPELL_HEAL_FATHER 6262
-- cast (base points = treant's current HP) unmodeled; the
-- npc_warp_splinter_treant (19949) AI is documented only,
-- not registered (omor-heads / skyriss-illusion precedent):
-- its reachable C++ arms — the WarpGuid proximity check,
-- the Warp->CastSpell(Warp, SPELL_HEAL_FATHER) +
-- KillSelf heal arm, the MoveFollow arm, the melee arm —
-- all sit behind movement / cross-creature / cast-bp0 /
-- suicide bridges that do not exist, so there is nothing
-- bridgeable; the six treant_pos[] spawn coordinates and
-- the Treant_Spawn_Pos_X/Y bookkeeping feed only the
-- summon orientation, so they land nowhere; the
-- ScriptData "SD%Complete: 80 / Includes Sapling (need
-- some better control with these)" caveat is upstream,
-- documented, not bridged; no SpellScript/AuraScript
-- scripts in this file.

local SPELL_WAR_STOMP = 34716
local SPELL_ARCANE_VOLLEY = 36705

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_SUMMON = 2
local SAY_DEATH = 3

local ENTRY_WARP_SPLINTER = 17977

local combatTimers = {}
local combatStates = {}

local function cancelCombatPump(guid)
    local id = combatTimers[guid]
    if id then
        RemoveEventById(id)
        combatTimers[guid] = nil
    end
end

-- C++ Reset() (called on evade): the observable remainder
-- — the Initialize() re-latch and the SetSpeedRate(MOVE_
-- RUN, 0.7f) arm (no speed bridge) — lands nowhere; the
-- re-latch lands on OnEnterCombat (gargolmar precedent).
local function fullReset(guid)
    cancelCombatPump(guid)
    combatStates[guid] = nil
end

-- C++ UpdateAI in arm order — 1s granularity exact for all
-- C++ timers; the !UpdateVictim early return collapses
-- into the pump. Only bridgeable arms are modeled (the
-- summon arms are bridge-blocked — see deviations).
local function combatTick(creature, guid)
    local st = combatStates[guid]
    if not st then
        return
    end

    -- 1. War stomp arm: non-triggered DoCastVictim
    -- (C++-exact), re-arm {25s,40s} regardless.
    if st.warStomp <= 1000 then
        creature:CastSpell(nil, SPELL_WAR_STOMP)
        st.warStomp = math.random(25000, 40000)
    else
        st.warStomp = st.warStomp - 1000
    end

    -- 2. Arcane volley arm: non-triggered DoCastVictim
    -- (C++-exact), re-arm {20s,35s} regardless.
    if st.arcaneVolley <= 1000 then
        creature:CastSpell(nil, SPELL_ARCANE_VOLLEY)
        st.arcaneVolley = math.random(20000, 35000)
    else
        st.arcaneVolley = st.arcaneVolley - 1000
    end

    -- 3. Summon treants arm: the six SummonCreature casts
    -- are skipped (no summon bridge — steamrigger
    -- precedent); only Talk(SAY_SUMMON) is modeled (the C++
    -- talks at the end of SummonTreants, C++-exact), re-arm
    -- 45s.
    if st.summonTreants <= 1000 then
        creature:Talk(SAY_SUMMON)
        st.summonTreants = 45000
    else
        st.summonTreants = st.summonTreants - 1000
    end
end

-- C++ ctor / Initialize() values (the whole schedule lands
-- on OnEnterCombat): War Stomp {25s,40s} / Arcane Volley
-- {8s,20s} / Summon Treants 45s. The C++ JustEngagedWith
-- only talks (the BossAI arm is instance-blocked) —
-- Talk(SAY_AGGRO) (C++-exact).
RegisterCreatureEvent(ENTRY_WARP_SPLINTER, 1, function(_, creature)
    local guid = creature:GetGUID()
    cancelCombatPump(guid)
    combatStates[guid] = {
        warStomp = math.random(25000, 40000),
        arcaneVolley = math.random(8000, 20000),
        summonTreants = 45000,
    }
    creature:Talk(SAY_AGGRO)
    combatTimers[guid] = CreateLuaEvent(function()
        combatTick(creature, guid)
    end, 1000, 0)
end)

-- C++ Reset() (called on evade): fullReset — see above.
RegisterCreatureEvent(ENTRY_WARP_SPLINTER, 2, function(_, creature)
    fullReset(creature:GetGUID())
end)

-- C++ KilledUnit: Talk(SAY_SLAY) unconditionally (no player
-- gate, C++-exact).
RegisterCreatureEvent(ENTRY_WARP_SPLINTER, 3, function(_, creature)
    creature:Talk(SAY_SLAY)
end)

-- C++ JustDied: Talk(SAY_DEATH) + cleanup (the _JustDied
-- arm is instance-blocked).
RegisterCreatureEvent(ENTRY_WARP_SPLINTER, 4, function(_, creature)
    creature:Talk(SAY_DEATH)
    fullReset(creature:GetGUID())
end)

-- Eluna's On_Reset fires ahead of OnDied and OnSpawn, so
-- this lands the same Reset() reset as the evade hook.
RegisterCreatureEvent(ENTRY_WARP_SPLINTER, 23, function(_, creature)
    fullReset(creature:GetGUID())
end)
