-- Grand Warlock Nethekurse (Shattered Halls, Hellfire Citadel) — Lua
-- port of src/server/scripts/Outland/HellfireCitadel/
-- ShatteredHalls/boss_nethekurse.cpp (boss_grand_warlock_
-- nethekurse — the only boss AI class AddSC registers in
-- this file; npc_fel_orc_convert and npc_lesser_shadow_
-- fissure are registered alongside).
-- First boss in the Shattered Halls set per
-- outland_script_loader.cpp order (instance_shattered_halls.
-- lua ported 3fe088b3 — the door machine reads this file's
-- SetBossState(0, 3) write on death).
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
-- OnTargetDied, 4 OnDied, 19 OnJustSummoned, 23 OnReset. Timers via CreateLua
-- Event (per-GUID scheduler pump); melee is engine-driven in
-- Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- C++ DoCast default is triggered=false (vaelastrasz
-- convention).
-- Cast-state gates: NONE on ported arms — own UpdateAI override, raw timers, zero HasUnitState(UNIT_STATE_CASTING) gates (BossAI base gates inapplicable); fissure/coil/spin/cleave fire ungated, C++-exact.
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
-- activate-schedule precedent) + SetBossState(0, 1) IN_PROGRESS
-- (BossAI::_JustEngagedWith, ScriptedCreature.cpp:537) +
-- Talk(SAY_AGGRO) + 1s
-- scheduler pump (a port of UpdateAI — 1s granularity exact
-- for all C++ timers; the !UpdateVictim early return
-- collapses into the pump; IsMainEvent is true at the only
-- hook this model has).
-- Pump: phase 1 (health > 20%): shadow fissure 30496: 8s
-- init then {7.5s,15s} — target = random threat-list entry
-- (C++ SelectTarget(Random, 0), playerOnly defaults false,
-- UnitAI.h:170; GetAITargets idiom), nil pick casts nothing,
-- non-triggered DoCast on the target (C++-exact), re-arm
-- {7.5s,15s} regardless (C++-exact); death coil 30500: 20s
-- init then {15s,20s} — same targeting (C++-exact), nil pick
-- casts nothing, re-arm {15s,20s} regardless (C++-exact);
-- then the phase
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
-- + SetBossState(0, 3) DONE (the _JustDied twin — drives the
-- instance door machine) + cleanup. OnLeave
-- Combat(2)/OnReset(23): cancel the pump, drop per-GUID
-- state; ev23 also writes SetBossState(0, 0) NOT_STARTED (the
-- _Reset twin, ScriptedCreature.cpp:499-510 — lands before the
-- ev4 DONE write, so the final state stays DONE). The Reset()
-- NON_ATTACKABLE re-flag arm stays out on the
-- paired-arm rule — its removal rides the unported peon-relay /
-- intro-timer machine and porting SetFlag alone would strand the bit
-- (thespia/kalithresh precedent); SetFlag(59) itself is bridged;
-- only the flag leg stays out).
-- Deliberate deviations (all await engine bridges): no
-- intro/peon machine — boss admission via the luaBossAI
-- shim (the BossAI ctor DATA_NETHEKURSE = 0 bookkeeping is
-- PORTED as the SetBossState writes above, not skipped; the
-- MoveInLineOfSight SAY_INTRO + SetBossState IN_PROGRESS arm and the 90s
-- IntroEvent_Timer -> DoTauntPeons machine (the "@todo kill
-- the peons first" arm) stay out on sighting-bridge grounds
-- — ev27 is engage-only, no 30yd sighting twin; the peon
-- SetData counts and the all-four-dead activation live in
-- npc_fel_orc_convert.lua as direct Talk relays, the
-- NON_ATTACKABLE removal moot in-port); no
-- cross-creature bridge — the peon SetData(SETDATA_PEON_
-- AGGRO/DEATH) relay from npc_fel_orc_convert (ObjectAccessor
-- GetCreature via instance GetGuidData), the resulting
-- SAY_PEON_ATTACKED/DIES counts and the all-four-dead
-- activation (IsMainEvent + NON_ATTACKABLE removal) are
-- unmodeled, so SAY_INTRO/PEON_ATTACKED/PEON_DIES/TAUNT are
-- unreachable; the Reset/activation NON_ATTACKABLE arms stay
-- unmodeled — their paired removal rides the unported peon-relay /
-- intro-timer machine, and porting SetFlag alone would strand the
-- bit (thespia/kalithresh precedent);
-- summon bridge live, and the JustSummoned setup (SetFaction +
-- NON_ATTACKABLE/NOT_SELECTABLE flags + triggered TEMPORARY_
-- VISUAL 39312 + CONSUMPTION 30497 with original-caster
-- args) is PORTED below as the ev19 handler — the old "spell
-- 30496 has no Go spell-effect producer (zero engine hits)"
-- claim was stale: the producer is the generic
-- SPELL_EFFECT_SUMMON path (creatureEffectSummonType,
-- boss_ai.go:4866), DBC-driven, and summonTempCreature fires
-- ev19 on the summoner (lua_creature_events.go:190); only the
-- original-caster leg is unmodeled (CastCustomSpell accepts and
-- ignores it, lua_creature_events.go:983-986). The fissure
-- spawn itself still rides the DBC row + world-DB template for
-- the fissure entry, as does the 30496 cast in the pump;
-- npc_lesser_shadow_fissure
-- documented only, not registered — all four of its AI
-- overrides are empty (ahune bunny empty-skeleton precedent); H_SPELL_DEATH_COIL
-- 30741 / H_SPELL_SHADOW_SLAM 35953 are unused C++ enum
-- members (both casts are unconditional, C++-exact); no
-- movement bridge — the AttackStart DoStartNoMovement/
-- DoStartMovement arms unmodeled: DoStartMovement is the
-- engine-default chase, and DoStartNoMovement is
-- AttackStart-gated in C++ (fires only on the next
-- AttackStart, not at the phase flip), so a mid-combat
-- SetCombatMovement(false) would diverge until retarget
-- (SetCombatMovement itself is bridged — novos engage-time
-- precedent, thespia precedent); phase-2 melee suppression
-- PORTED 2026-10-24 via SetMeleeAttack(false) at the flip —
-- the old "no bridge to suppress engine melee" claim was
-- stale (SetMeleeAttack is bridged and honored in the
-- combat-tick swing gate);
-- no SpellScript/AuraScript scripts in this file.

local SPELL_DEATH_COIL = 30500
local SPELL_SHADOW_FISSURE = 30496
local SPELL_DARK_SPIN = 30502
local SPELL_SHADOW_CLEAVE = 30495
local SPELL_TEMPORARY_VISUAL = 39312
local SPELL_CONSUMPTION = 30497

local SAY_AGGRO = 4
local SAY_SLAY = 5
local SAY_DIE = 6

local ENTRY_NETHEKURSE = 16807

local FACTION_MONSTER_2 = 16
local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NON_ATTACKABLE = 0x2
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000

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

-- C++ SelectTarget(Random, 0) draws from the threat list
-- (UnitAI.h:170, playerOnly defaults false) — GetAITargets is
-- the bridged threat list (murmur/thaurissan precedent), not
-- the instance player pool.
local function randomThreatTarget(creature)
    local targets = creature:GetAITargets()
    if targets == nil or #targets == 0 then
        return nil
    end
    local alive = {}
    for _, t in ipairs(targets) do
        if not t:IsDead() then
            alive[#alive + 1] = t
        end
    end
    if #alive == 0 then
        return nil
    end
    return alive[math.random(#alive)]
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
            -- C++: Cleave_Timer = 6000 + rand32() % 2500 = [6000,8499].
            st.cleave = math.random(6000, 8499)
        else
            st.cleave = st.cleave - 1000
        end
    else
        -- Shadow fissure 30496: 8s init then {7.5s,15s} —
        -- target = random threat-list entry (C++
        -- SelectTarget(Random, 0), playerOnly defaults false),
        -- nil pick casts nothing, non-triggered DoCast on the
        -- target (C++-exact), re-arm {7.5s,15s} regardless
        -- (C++-exact).
        if st.shadowFissure <= 1000 then
            local target = randomThreatTarget(creature)
            if target ~= nil then
                creature:CastSpell(target, SPELL_SHADOW_FISSURE)
            end
            st.shadowFissure = math.random(7500, 15000)
        else
            st.shadowFissure = st.shadowFissure - 1000
        end

        -- Death coil 30500: 20s init then {15s,20s} — same
        -- threat-list targeting (C++-exact), nil pick casts
        -- nothing, re-arm {15s,20s} regardless (C++-exact).
        -- (H_SPELL 30741 is an unused C++ enum member.)
        if st.deathCoil <= 1000 then
            local target = randomThreatTarget(creature)
            if target ~= nil then
                creature:CastSpell(target, SPELL_DEATH_COIL)
            end
            st.deathCoil = math.random(15000, 20000)
        else
            st.deathCoil = st.deathCoil - 1000
        end

        -- Phase check (C++ !HealthAbovePct(20), C++-exact
        -- arm order — after the timers, same tick).
        if creature:GetHealthPct() <= 20 then
            st.phase = true
            -- C++ phase-2 branch skips DoMeleeAttackIfReady every
            -- tick (boss_nethekurse.cpp:240-256 has no melee arm);
            -- SetMeleeAttack(false) is the bridged twin, honored
            -- in the combat-tick swing gate (novos precedent).
            creature:SetMeleeAttack(false)
        end
    end
end

RegisterCreatureEvent(ENTRY_NETHEKURSE, 1, function(_, creature)
    local guid = creature:GetGUIDLow()
    resetBoss(guid)
    states[guid] = freshState()
    creature:SetBossState(0, 1) -- DATA_NETHEKURSE, IN_PROGRESS (BossAI::_JustEngagedWith, ScriptedCreature.cpp:537; gruul precedent)
    -- Restore phase-1 melee: the SetMeleeAttack(false) flag
    -- persists on the motion past evade, so re-engage must
    -- re-allow swings (phase 1 calls DoMeleeAttackIfReady).
    creature:SetMeleeAttack(true)
    creature:Talk(SAY_AGGRO)
    timers[guid] = CreateLuaEvent(function()
        bossTick(creature, guid)
    end, 1000, 0)
end)

RegisterCreatureEvent(ENTRY_NETHEKURSE, 2, function(_, creature)
    resetBoss(creature:GetGUIDLow())
end)

RegisterCreatureEvent(ENTRY_NETHEKURSE, 3, function(_, creature)
    -- C++ KilledUnit talks unconditionally — no player gate
    -- (C++-exact).
    creature:Talk(SAY_SLAY)
end)

RegisterCreatureEvent(ENTRY_NETHEKURSE, 4, function(_, creature)
    creature:SetBossState(0, 3) -- DATA_NETHEKURSE, DONE (boss_nethekurse.cpp:118-121: _JustDied then Talk)
    creature:Talk(SAY_DIE)
    resetBoss(creature:GetGUIDLow())
end)

RegisterCreatureEvent(ENTRY_NETHEKURSE, 23, function(_, creature)
    -- C++ _Reset (ScriptedCreature.cpp:499-510): SetBossState
    -- DATA_NETHEKURSE, NOT_STARTED — ev23 fires on death too
    -- (engine reset-then-died order); the NOT_STARTED write lands
    -- before the ev4 DONE write, so the final state stays DONE
    -- (gruul precedent). The Reset() NON_ATTACKABLE re-flag arm
    -- stays out on the paired-arm rule (see header).
    creature:SetBossState(0, 0) -- DATA_NETHEKURSE, NOT_STARTED
    resetBoss(creature:GetGUIDLow())
end)

-- C++ JustSummoned (boss_nethekurse.cpp:208-216): the fissure
-- summoned by SPELL_SHADOW_FISSURE 30496 gets the monster
-- faction, both flags, then the triggered temporary visual and
-- Consumption with Nethekurse as original caster. The producer
-- chain is live in the runtime: the pump's 30496 cast runs the
-- generic SPELL_EFFECT_SUMMON producer (creatureEffectSummonType,
-- boss_ai.go:4866, inside the castCreatureSpell funnel,
-- boss_ai.go:1791) and summonTempCreature fires ev19 on the
-- summoner (lua_creature_events.go:190). The original-caster leg
-- has no Go model — CastCustomSpell accepts and ignores it
-- (lua_creature_events.go:983-986), the cast itself still fires;
-- kill-credit attribution only. C++ applies to any summon and
-- Nethekurse's only summon source is 30496, so no entry gate.
local function nethekurseJustSummoned(_, creature, summoned)
    if summoned == nil then
        return
    end
    summoned:SetFaction(FACTION_MONSTER_2)
    summoned:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
    summoned:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    summoned:CastSpell(summoned, SPELL_TEMPORARY_VISUAL, true)
    summoned:CastCustomSpell(summoned, SPELL_CONSUMPTION, false, nil, nil, nil, nil, creature)
end

RegisterCreatureEvent(ENTRY_NETHEKURSE, 19, nethekurseJustSummoned)
