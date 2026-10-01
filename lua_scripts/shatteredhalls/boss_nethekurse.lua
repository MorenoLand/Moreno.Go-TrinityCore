-- Grand Warlock Nethekurse (Shattered Halls, Hellfire Citadel) — Lua
-- port of src/server/scripts/Outland/HellfireCitadel/
-- ShatteredHalls/boss_nethekurse.cpp (boss_grand_warlock_
-- nethekurse — the only boss AI class AddSC registers in
-- this file; npc_fel_orc_convert and npc_lesser_shadow_
-- fissure are registered alongside).
-- First boss in the Shattered Halls set per
-- outland_script_loader.cpp order (instance_shattered_halls.
-- cpp stays blocked on the instance-script model).
-- Entry: NPC_GRAND_WARLOCK_NETHEKURSE = 16807 in shattered_
-- halls.h (mapped in instance_shattered_halls.cpp
-- OnCreatureCreate — the door/peon instance machine around
-- it is instance-blocked); the creature_template ScriptName
-- bindings are DB-side (no TDB in this workspace).
-- Talk: SAY_AGGRO = 4 (pull — fired from the C++ JustEngaged
-- With, C++-exact), SAY_SLAY = 5 (kill — the C++ KilledUnit
-- override talks unconditionally, C++-exact), SAY_DIE = 6
-- (death); SAY_INTRO = 0 / SAY_PEON_ATTACKED = 1 / SAY_PEON_
-- DIES = 2 / SAY_TAUNT = 3 fire only from the unbridgeable
-- intro/peon SetData machine (cross-creature/instance
-- gated) — unreachable in this model.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLua
-- Event (per-GUID scheduler pump); melee is engine-driven in
-- Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- C++ DoCast default is triggered=false (vaelastrasz
-- convention).
-- Fight shape (C++-exact for all modeled arms): Nethekurse
-- (16807): OnEnterCombat(1): per-GUID reset to the C++
-- Initialize() values (deathCoil 20000 / shadowFissure 8000 /
-- cleave 5000 — the C++ JustEngagedWith override only talks;
-- the whole intro/peon machine (MoveInLineOfSight SAY_INTRO
-- + SetBossState IN_PROGRESS, the 90s DoTauntPeons timer,
-- the convert SetData PEON_AGGRO/DEATH relay, the NON_
-- ATTACKABLE gate until all four peons die) is instance/
-- cross-creature/flag blocked, so the Initialize() schedule
-- lands on the only engagement hook this model has, broggok
-- activate-schedule precedent) + Talk(SAY_AGGRO) + 1s
-- scheduler pump (a port of UpdateAI — 1s granularity exact
-- for all C++ timers; the !UpdateVictim early return
-- collapses into the pump; IsMainEvent is true at the only
-- hook this model has).
-- Pump: phase 1 (health > 20%): shadow fissure 30496: 8s
-- init then {7.5s,15s} — target = random alive player in
-- the instance (C++ SelectTarget(Random, 0), thespia
-- precedent), nil pick casts nothing, non-triggered DoCast
-- on the target (C++-exact), re-arm {7.5s,15s} regardless
-- (C++-exact); death coil 30500: 20s init then {15s,20s} —
-- same targeting (C++-exact), nil pick casts nothing,
-- re-arm {15s,20s} regardless (C++-exact); then the phase
-- check GetHealthPct() <= 20 sets phase (C++ !HealthAbovePct
-- (20), C++-exact arm order). Phase 2: dark spin 30502 once
-- (non-triggered DoCastVictim, C++-exact) then shadow cleave
-- 30495: 5s init then {6s,8.5s} — non-triggered DoCastVictim
-- (C++-exact), re-arm {6s,8.5s} regardless (C++-exact);
-- the cleave timer decrements only in the phase-2 branch, so
-- the first cleave fires 5s after the phase flip
-- (bookkeeping exact).
-- OnTargetDied(3): Talk(SAY_SLAY) — C++ KilledUnit talks
-- with no player gate (C++-exact). OnDied(4): Talk(SAY_DIE)
-- + cleanup (the _JustDied arm is instance-blocked). OnLeave
-- Combat(2)/OnReset(23): cancel the pump, drop per-GUID
-- state (the Reset() NON_ATTACKABLE re-flag arm has no flag
-- bridge; the _Reset arm is instance-blocked).
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI
-- shim (instance_shattered_halls.cpp stays blocked on the
-- instance-script model; the BossAI ctor DATA_NETHEKURSE =
-- 0 bookkeeping in Reset/JustEngagedWith/JustDied skipped —
-- DATA_NETHEKURSE is an instance-side constant,
-- unbridgeable; the MoveInLineOfSight SAY_INTRO + SetBoss
-- State IN_PROGRESS arm, the 90s IntroEvent_Timer ->
-- DoTauntPeons machine (the "@todo kill the peons first"
-- arm), and the door GO arms are instance gated); no
-- cross-creature bridge — the peon SetData(SETDATA_PEON_
-- AGGRO/DEATH) relay from npc_fel_orc_convert (ObjectAccessor
-- GetCreature via instance GetGuidData), the resulting
-- SAY_PEON_ATTACKED/DIES counts and the all-four-dead
-- activation (IsMainEvent + NON_ATTACKABLE removal) are
-- unmodeled, so SAY_INTRO/PEON_ATTACKED/PEON_DIES/TAUNT are
-- unreachable; no flag bridge — the Reset/activation NON_
-- ATTACKABLE arms unmodeled (thespia/kalithresh precedent);
-- no summon bridge — the shadow-fissure summon of the
-- fissure NPC never fires and the JustSummoned setup
-- (SetFaction + NON_ATTACKABLE/NOT_SELECTABLE flags +
-- triggered TEMPORARY_VISUAL 39312 + CONSUMPTION 30497 with
-- original-caster args) is unreachable (steamrigger
-- precedent); npc_lesser_shadow_fissure documented only,
-- not registered — all four of its AI overrides are empty
-- (ahune bunny empty-skeleton precedent); no difficulty
-- bridge — the H_SPELL_DEATH_COIL 30741 and H_SPELL_SHADOW_
-- SLAM 35953 variants unmodeled (thespia precedent); no
-- movement bridge — the AttackStart DoStartNoMovement/
-- DoStartMovement arms unmodeled (thespia precedent); no
-- unit-state bridge — the C++ phase-2 branch skips DoMelee
-- AttackIfReady entirely (no melee during the dark-spin
-- phase) and there is no bridge to suppress engine melee;
-- no SpellScript/AuraScript scripts in this file.

local SPELL_DEATH_COIL = 30500
local SPELL_SHADOW_FISSURE = 30496
local SPELL_DARK_SPIN = 30502
local SPELL_SHADOW_CLEAVE = 30495

local SAY_AGGRO = 4
local SAY_SLAY = 5
local SAY_DIE = 6

local ENTRY_NETHEKURSE = 16807

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

-- C++ Initialize() values (the peon/intro machine is
-- instance/cross-creature/flag blocked, so the schedule
-- lands on OnEnterCombat — broggok precedent): deathCoil
-- 20000, shadowFissure 8000, cleave 5000, phase false,
-- spinOnce false.
local function freshState()
    return {
        deathCoil = 20000,
        shadowFissure = 8000,
        cleave = 5000,
        phase = false,
        spinOnce = false,
    }
end

local function bossTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end

    if st.phase then
        -- Dark spin 30502 once — non-triggered DoCastVictim
        -- (C++-exact).
        if not st.spinOnce then
            creature:CastSpell(nil, SPELL_DARK_SPIN)
            st.spinOnce = true
        end

        -- Shadow cleave 30495: 5s init then {6s,8.5s} —
        -- non-triggered DoCastVictim (C++-exact), re-arm
        -- regardless (C++-exact). The timer decrements only
        -- in the phase-2 branch, so the first cleave fires
        -- 5s after the phase flip.
        if st.cleave <= 1000 then
            creature:CastSpell(nil, SPELL_SHADOW_CLEAVE)
            st.cleave = math.random(6000, 8500)
        else
            st.cleave = st.cleave - 1000
        end
    else
        -- Shadow fissure 30496: 8s init then {7.5s,15s} —
        -- target = random alive player in the instance (C++
        -- SelectTarget(Random, 0), thespia precedent), nil
        -- pick casts nothing, non-triggered DoCast on the
        -- target (C++-exact), re-arm {7.5s,15s} regardless
        -- (C++-exact).
        if st.shadowFissure <= 1000 then
            local players = playersInInstance(creature)
            if #players > 0 then
                creature:CastSpell(players[math.random(#players)],
                    SPELL_SHADOW_FISSURE)
            end
            st.shadowFissure = math.random(7500, 15000)
        else
            st.shadowFissure = st.shadowFissure - 1000
        end

        -- Death coil 30500: 20s init then {15s,20s} — same
        -- targeting (C++-exact), nil pick casts nothing,
        -- re-arm {15s,20s} regardless (C++-exact). The
        -- heroic 30741 variant has no difficulty bridge
        -- (thespia precedent).
        if st.deathCoil <= 1000 then
            local players = playersInInstance(creature)
            if #players > 0 then
                creature:CastSpell(players[math.random(#players)],
                    SPELL_DEATH_COIL)
            end
            st.deathCoil = math.random(15000, 20000)
        else
            st.deathCoil = st.deathCoil - 1000
        end

        -- Phase check (C++ !HealthAbovePct(20), C++-exact
        -- arm order — after the timers, same tick).
        if creature:GetHealthPct() <= 20 then
            st.phase = true
        end
    end
end

RegisterCreatureEvent(ENTRY_NETHEKURSE, 1, function(_, creature)
    local guid = creature:GetGUID()
    resetBoss(guid)
    states[guid] = freshState()
    creature:Talk(SAY_AGGRO)
    timers[guid] = CreateLuaEvent(function()
        bossTick(creature, guid)
    end, 1000, 0)
end)

RegisterCreatureEvent(ENTRY_NETHEKURSE, 2, function(_, creature)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_NETHEKURSE, 3, function(_, creature)
    -- C++ KilledUnit talks unconditionally — no player gate
    -- (C++-exact).
    creature:Talk(SAY_SLAY)
end)

RegisterCreatureEvent(ENTRY_NETHEKURSE, 4, function(_, creature)
    creature:Talk(SAY_DIE)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_NETHEKURSE, 23, function(_, creature)
    resetBoss(creature:GetGUID())
end)
