-- Leotheras the Blind (Serpentshrine Cavern) — Lua port of
-- src/server/scripts/Outland/CoilfangReservoir/SerpentShrine/
-- boss_leotheras_the_blind.cpp (boss_leotheras_the_blind,
-- boss_leotheras_the_blind_demonform, npc_greyheart_spellbinder,
-- npc_inner_demon — all four registered; AddSC_boss_leotheras_
-- the_blind registers those four C++ ScriptNames).
-- Entry 21215 verified from instance_serpent_shrine.cpp's
-- OnCreatureCreate (case 21215 -> LeotherasTheBlind, DATA_
-- LEOTHERAS); the other entries come from the file's own enum
-- constants (DEMON_FORM = 21875, NPC_SPELLBINDER = 21806, INNER_
-- DEMON_ID = 21857). The creature_template ScriptName bindings
-- are DB-side (no TDB in this workspace). Talk lines used: SAY_
-- AGGRO=0 (pull — fired by the banish-release StartEvent in C++;
-- the modeled fight starts on pull, magtheridon/ragnaros-intro
-- convention), SAY_SWITCH_TO_DEMON=1 (demon switch), SAY_INNER_
-- DEMONS=2 (inner-demon summon), SAY_DEMON_SLAY=3 / SAY_NIGHTELF_
-- SLAY=4 (kill — TYPEID_PLAYER gate, modeled via victim:
-- IsPlayer()), SAY_FINAL_FORM=5 (15% split), SAY_FREE=6 (demon
-- copy engage), SAY_DEATH=7 (death). All eight are referenced in
-- the file.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent
-- (per-GUID scheduler pump); melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady. C++
-- DoCast default is triggered=false (vaelastrasz convention).
-- Fight shape (C++-exact for the modeled arms):
-- boss_leotheras_the_blind (21215): OnEnterCombat(1): per-GUID
-- scheduler reset to the C++ Initialize() values (whirlwind
-- 15000 / chaosBlast 1000 / switchToDemon 45000 / switchToHuman
-- 60000 / berserk 600000 / innerDemons 30000 / demonForm false /
-- isFinalForm false / needThreatReset false / enrageUsed false)
-- + Talk(SAY_AGGRO) + start of a 1s scheduler pump (a port of
-- UpdateAI — 1s granularity is exact for all C++ timers; the C++
-- banish-aura/no-victim early return collapses into the pump,
-- since the banish pre-phase and the CheckBanish poll are
-- instance/summon/cross-creature blocked — below). Pump:
-- whirlwind-aura arm: while creature:HasAura(SPELL_WHIRLWIND
-- 37640), the 2s target-switch (ResetThreatList + MotionMaster
-- MovePoint to a random target) has no threat/movement bridges —
-- unmodeled, below; needThreatReset arm (fires when the
-- whirlwind aura lapses or a form switches): demonForm ->
-- innerDemons=30000, else whirlwind=15000, needThreatReset=false
-- (the ResetThreatList + MotionMaster Clear/MoveChase arms have
-- no threat/movement bridges — unmodeled, below). Berserk
-- 26662 600s one-shot (EnrageUsed-guarded): non-triggered self-
-- cast (C++-exact; the InterruptNonMeleeSpells arm has no
-- interrupt bridge — krosh convention). Nightelf form: whirlwind
-- 37640 — if the aura is absent and the timer fires -> non-
-- triggered self-cast 37640, whirlwind=2000, needThreatReset=true
-- (C++-exact); switch-to-demon (only while !isFinalForm) 45s ->
-- RemoveAura(37640), Talk(SAY_SWITCH_TO_DEMON), demonForm=true,
-- needThreatReset=true, switchToDemon=45000 (the SetDisplayId
-- demon + virtual-item-slot zeroing arms have no display/item
-- bridges — unmodeled, below). Demon form: chaos blast 37674 —
-- 1s init then 3s: if the victim is within 30 yd (GetDistance
-- bridge for the IsWithinDist gate) -> CastSpell(victim, 37674);
-- re-arm 3000 outside the range gate (C++-exact; the BP0=100
-- bonus-point arg and the original-caster arg have no bridge —
-- quake/blaze SPELLVALUE precedent, and the StopMoving arm has
-- no movement bridge — unmodeled, below). Inner demons 30s one-
-- shot -> Talk(SAY_INNER_DEMONS), timer pinned to 999999 (C++-
-- exact; the threat-list pick of up to 5 non-victim players, the
-- SummonCreature(INNER_DEMON_ID) arms and the player-side
-- AddAura(INSIDIOUS_WHISPER 37676) have no threat/summon/player-
-- aura bridges — unmodeled, below). Switch-to-human 60s ->
-- demonForm=false, needThreatReset=true, switchToHuman=60000
-- (the CastConsumingMadness arm is demon-cast — no inner demons
-- exist in this model — and the DespawnDemon/SetDisplayId/
-- LoadEquipment arms have no bridges — unmodeled, below). Final
-- form: once-guarded GetHealthPct() < 15 (C++ HealthBelowPct(15))
-- -> isFinalForm=true, demonForm=false, Talk(SAY_FINAL_FORM)
-- (the DoSpawnCreature(DEMON_FORM 21875) copy arm has no summon
-- bridge — unmodeled, below). OnTargetDied(3): victim:IsPlayer()
-- -> Talk(demonForm and SAY_DEMON_SLAY or SAY_NIGHTELF_SLAY)
-- (C++-exact per-form slay). OnDied(4): Talk(SAY_DEATH) +
-- cleanup (the demon-copy despawn is cross-creature blocked; the
-- SetData(DONE) arm is instance-blocked). OnLeaveCombat(2)/
-- OnReset(23): cancel the pump, drop per-GUID state (the Reset
-- arms — CheckChannelers summon, dual-wield/2.0f speed/display/
-- item-slot/corpse-delay setup, SPELL_DUAL_WIELD 42459 triggered
-- self-cast, SetData(NOT_STARTED) — are summon/flag/display/
-- item/instance blocked).
-- boss_leotheras_the_blind_demonform (21875): OnEnterCombat(1):
-- per-GUID reset (chaosBlast 1000) + Talk(SAY_FREE) (the C++
-- JustEngagedWith -> StartEvent arm) + 1s pump. Pump: chaos
-- blast — if the timer fires AND the victim is within 30 yd ->
-- CastSpell(victim, 37674), re-arm 3000 (the re-arm sits inside
-- the range gate here, unlike the boss form — C++-exact). On-
-- TargetDied(3): victim:IsPlayer() -> Talk(SAY_DEMON_SLAY).
-- OnDied(4): cleanup (the post-mortem triggered self-cast of
-- 8149 — the blizzlike disappear — is unmodeled; a dead unit has
-- no cast bearer in the Lua model). OnLeaveCombat(2)/OnReset(23):
-- cleanup (the "deal no melee damage" arm has no bridge —
-- engine melee applies).
-- npc_greyheart_spellbinder (21806): OnEnterCombat(1): per-GUID
-- reset (mindblast {3s,8s} / earthshock {5s,10s}) + 1s pump
-- (the JustEngagedWith InterruptNonMeleeSpells + instance Set-
-- GuidData(DATA_LEOTHERAS_EVENT_STARTER) arms are interrupt/
-- instance blocked). Pump: mind blast 37531 — timer {3s,8s} init
-- then {10s,15s}: CastSpell(random alive player, 37531) (C++
-- SelectTarget(Random, 0) — no player-only gate, teron/krosh
-- convention; nil pick casts nothing, re-arm regardless —
-- C++-exact). The earthshock arm is unmodeled: the C++ scans
-- every player for a non-null GetCurrentSpell (cast detection)
-- and no cast-Spell bridge exists (standing gap) — a random
-- cast would break the anti-caster intent. The out-of-combat
-- CastChanneling arm (BANISH_BEAM 38909 on leotheras via the
-- instance DATA_LEOTHERAS GUID) and the no-event-starter evade
-- arm are instance/cross-creature blocked (hydross precedent —
-- no out-of-combat update hook in the Lua model). OnDied(4):
-- cleanup (C++ JustDied is empty). OnLeaveCombat(2)/OnReset(23):
-- cleanup (the Reset re-summon relay into leotheras's
-- CheckChannelers is cross-creature blocked).
-- npc_inner_demon (21857): OnEnterCombat(1): per-GUID reset
-- (shadowBolt 10000 / link 1000) + 1s pump. Pump: soul link
-- 38007 1s — CastSpell(nil, 38007, true) triggered (the C++
-- DoCastVictim nil-victim arm — felmyst convention); demonic
-- alignment 37713 — if the creature lacks the aura ->
-- CastSpell(creature, 37713, true) triggered (C++-exact upkeep);
-- shadow bolt 39309 10s — CastSpell(nil, 39309) (C++ DoCastVictim
-- non-triggered — vaelastrasz convention). The victim binding
-- (SetGUID(INNER_DEMON_VICTIM) from leotheras — cross-creature
-- blocked), the damage-immunity arm (damage=0 unless the
-- attacker is the bound victim or self — no damage-modify
-- bridge, standing gap) and the threat retarget/owner-death
-- arms (no threat bridge) are unmodeled; the JustDied whisper-
-- aura removal is cross-creature blocked. OnDied(4)/
-- OnLeaveCombat(2)/OnReset(23): cleanup.
-- Deliberate deviations (all await engine bridges): no instance-
-- script model — boss admission via the luaBossAI shim (the
-- DATA_LEOTHERASTHEBLINDEVENT NOT_STARTED/IN_PROGRESS/DONE
-- bookkeeping, DATA_LEOTHERAS_EVENT_STARTER starter bookkeeping,
-- the DATA_LEOTHERAS GUID lookups and the whole banish-release
-- machine — CheckBanish's 3x SpellBinderGUID liveness poll,
-- AURA_BANISH 37833 apply/remove, ApplySpellImmune banish
-- arms, LoadEquipment, starter-victim AddThreat — skipped; the
-- banished pre-phase collapses into the pull per the
-- magtheridon convention); no summon bridge — the 3x Greyheart
-- Spellbinder (21806) CheckChannelers summons, the inner-demon
-- (21857) summon arms and the final-form demon-copy (21875)
-- spawn unmodeled (firesworn/hydross convention); no movement/
-- threat bridge — the whirlwind target-switch MovePoint, the
-- NeedThreatReset ResetThreatList/MoveChase arms, the chaos-
-- blast StopMoving arm and the leotheras MoveInLineOfSight
-- intro/aggro machine unmodeled; no display/item/flag bridges —
-- the MODEL_DEMON(20125)/MODEL_NIGHTELF(20514) SetDisplayId
-- swaps, the UNIT_VIRTUAL_ITEM_SLOT zeroing, LoadEquipment, the
-- dual-wield/speed/corpse-delay setup and the SPELL_DUAL_WIELD
-- 42459 cast unmodeled; no cross-creature bridge — the spell-
-- binder leotherasGUID relay, the inner-demon victim binding,
-- the ConsumingMadness 37749 demon casts and the demon-copy
-- despawn unmodeled; no interrupt bridge (krosh convention) —
-- the berserk InterruptNonMeleeSpells and the spellbinder
-- JustEngagedWith interrupt unmodeled; no cast-detection/player-
-- aura bridges — the earthshock caster scan, the whisper
-- AddAura(37676) and the cube-note player-CastSpell gap.

local SPELL_WHIRLWIND = 37640
local SPELL_CHAOS_BLAST = 37674
local SPELL_BERSERK = 26662
local SPELL_MINDBLAST = 37531
local SPELL_SOUL_LINK = 38007
local SPELL_SHADOWBOLT = 39309
local SPELL_DEMONIC_ALIGNMENT = 37713

local SAY_AGGRO = 0
local SAY_SWITCH_TO_DEMON = 1
local SAY_INNER_DEMONS = 2
local SAY_DEMON_SLAY = 3
local SAY_NIGHTELF_SLAY = 4
local SAY_FINAL_FORM = 5
local SAY_FREE = 6
local SAY_DEATH = 7

local ENTRY_LEOTHERAS = 21215
local ENTRY_DEMON_FORM = 21875
local ENTRY_SPELLBINDER = 21806
local ENTRY_INNER_DEMON = 21857

local CHAOS_BLAST_RANGE = 30

local leotherasTimers = {}
local leotherasState = {}
local demonTimers = {}
local demonState = {}
local binderTimers = {}
local binderState = {}
local innerTimers = {}
local innerState = {}

-- Alive players sharing the creature's map+instance (teron
-- convention).
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

-- C++ SelectTarget(Random, 0): random alive player in the instance.
local function pickRandomAlivePlayer(creature)
    local found = playersInInstance(creature)
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
end

local function cancelPump(timers, guid)
    local id = timers[guid]
    if id then
        RemoveEventById(id)
        timers[guid] = nil
    end
end

local function resetCreature(timers, states, guid)
    cancelPump(timers, guid)
    states[guid] = nil
end

-- C++ Initialize(): whirlwind 15000, chaosBlast 1000, switchToDemon
-- 45000, switchToHuman 60000, berserk 600000, innerDemons 30000,
-- demonForm/isFinalForm/needThreatReset/enrageUsed false.
local function freshLeotherasState()
    return {
        whirlwind = 15000,
        chaosBlast = 1000,
        switchToDemon = 45000,
        switchToHuman = 60000,
        berserk = 600000,
        innerDemons = 30000,
        demonForm = false,
        isFinalForm = false,
        needThreatReset = false,
        enrageUsed = false,
    }
end

local function leotherasTick(creature, guid)
    local st = leotherasState[guid]
    if not st then
        return
    end

    -- NeedThreatReset arm: fires once the whirlwind aura lapses or
    -- a form switches (the ResetThreatList + MotionMaster Clear/
    -- MoveChase arms have no threat/movement bridges — timer
    -- bookkeeping only, below).
    if st.needThreatReset and not creature:HasAura(SPELL_WHIRLWIND) then
        if st.demonForm then
            st.innerDemons = 30000
        else
            st.whirlwind = 15000
        end
        st.needThreatReset = false
    end

    -- Berserk: 10min one-shot, EnrageUsed-guarded (C++-exact),
    -- non-triggered self-cast.
    if st.berserk <= 1000 and not st.enrageUsed then
        creature:CastSpell(creature, SPELL_BERSERK)
        st.enrageUsed = true
    elseif st.berserk > 1000 then
        st.berserk = st.berserk - 1000
    end

    if not st.demonForm then
        -- Whirlwind: cast once the aura is absent and the timer
        -- fires; while the aura holds, the C++ 2s target-switch
        -- (ResetThreatList + MovePoint to a random target) is
        -- threat/movement blocked — unmodeled, header.
        if not creature:HasAura(SPELL_WHIRLWIND) then
            if st.whirlwind <= 1000 then
                creature:CastSpell(creature, SPELL_WHIRLWIND)
                st.whirlwind = 2000
                st.needThreatReset = true
            else
                st.whirlwind = st.whirlwind - 1000
            end
        end

        -- Switch to demon form (nightelf form only, never after
        -- the final form).
        if not st.isFinalForm then
            if st.switchToDemon <= 1000 then
                creature:RemoveAura(SPELL_WHIRLWIND)
                creature:Talk(SAY_SWITCH_TO_DEMON)
                st.demonForm = true
                st.needThreatReset = true
                st.switchToDemon = 45000
            else
                st.switchToDemon = st.switchToDemon - 1000
            end
        end
    else
        -- Chaos blast: 1s init then 3s — cast only when the victim
        -- is within 30 yd; re-arm sits outside the range gate
        -- (C++-exact).
        if st.chaosBlast <= 1000 then
            local victim = creature:GetVictim()
            if victim and creature:GetDistance(victim) <= CHAOS_BLAST_RANGE then
                creature:CastSpell(victim, SPELL_CHAOS_BLAST)
            end
            st.chaosBlast = 3000
        else
            st.chaosBlast = st.chaosBlast - 1000
        end

        -- Inner demons: 30s one-shot, then pinned (C++-exact).
        -- The summon + player whisper-aura arms are summon/player-
        -- aura blocked — unmodeled, header.
        if st.innerDemons <= 1000 then
            creature:Talk(SAY_INNER_DEMONS)
            st.innerDemons = 999999
        else
            st.innerDemons = st.innerDemons - 1000
        end

        -- Switch back to nightelf form (the ConsumingMadness/
        -- DespawnDemon/display/equipment arms are blocked — header).
        if st.switchToHuman <= 1000 then
            st.demonForm = false
            st.needThreatReset = true
            st.switchToHuman = 60000
        else
            st.switchToHuman = st.switchToHuman - 1000
        end
    end

    -- Final form: once-guarded <15% health (C++ HealthBelowPct(15)).
    -- The demon-copy summon is summon-blocked — header.
    if not st.isFinalForm and creature:GetHealthPct() < 15 then
        st.isFinalForm = true
        st.demonForm = false
        creature:Talk(SAY_FINAL_FORM)
    end
end

local function leotherasEnterCombat(event, creature)
    local guid = creature:GetGUID()
    resetCreature(leotherasTimers, leotherasState, guid)
    leotherasState[guid] = freshLeotherasState()
    creature:Talk(SAY_AGGRO)
    leotherasTimers[guid] = CreateLuaEvent(function()
        leotherasTick(creature, guid)
    end, 1000)
end

local function leotherasLeaveCombat(event, creature)
    resetCreature(leotherasTimers, leotherasState, creature:GetGUID())
end

-- C++ KilledUnit: TYPEID_PLAYER gate -> per-form slay line.
local function leotherasTargetDied(event, creature, victim)
    local st = leotherasState[creature:GetGUID()]
    if victim:IsPlayer() then
        if st and st.demonForm then
            creature:Talk(SAY_DEMON_SLAY)
        else
            creature:Talk(SAY_NIGHTELF_SLAY)
        end
    end
end

local function leotherasDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    resetCreature(leotherasTimers, leotherasState, creature:GetGUID())
end

local function leotherasReset(event, creature)
    resetCreature(leotherasTimers, leotherasState, creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_LEOTHERAS, 1, leotherasEnterCombat)
RegisterCreatureEvent(ENTRY_LEOTHERAS, 2, leotherasLeaveCombat)
RegisterCreatureEvent(ENTRY_LEOTHERAS, 3, leotherasTargetDied)
RegisterCreatureEvent(ENTRY_LEOTHERAS, 4, leotherasDied)
RegisterCreatureEvent(ENTRY_LEOTHERAS, 23, leotherasReset)

-- boss_leotheras_the_blind_demonform (21875) — see header.

-- C++ Initialize(): chaosBlast 1000.
local function freshDemonState()
    return {
        chaosBlast = 1000,
    }
end

local function demonTick(creature, guid)
    local st = demonState[guid]
    if not st then
        return
    end

    -- Chaos blast: 1s init then 3s — cast AND re-arm only when the
    -- victim is within 30 yd (the re-arm sits inside the range
    -- gate here, unlike the boss form — C++-exact).
    if st.chaosBlast <= 1000 then
        local victim = creature:GetVictim()
        if victim and creature:GetDistance(victim) <= CHAOS_BLAST_RANGE then
            creature:CastSpell(victim, SPELL_CHAOS_BLAST)
            st.chaosBlast = 3000
        end
    else
        st.chaosBlast = st.chaosBlast - 1000
    end
end

local function demonEnterCombat(event, creature)
    local guid = creature:GetGUID()
    resetCreature(demonTimers, demonState, guid)
    demonState[guid] = freshDemonState()
    creature:Talk(SAY_FREE)
    demonTimers[guid] = CreateLuaEvent(function()
        demonTick(creature, guid)
    end, 1000)
end

local function demonCleanup(event, creature)
    resetCreature(demonTimers, demonState, creature:GetGUID())
end

-- C++ KilledUnit: TYPEID_PLAYER gate -> Talk(SAY_DEMON_SLAY).
local function demonTargetDied(event, creature, victim)
    if victim:IsPlayer() then
        creature:Talk(SAY_DEMON_SLAY)
    end
end

RegisterCreatureEvent(ENTRY_DEMON_FORM, 1, demonEnterCombat)
RegisterCreatureEvent(ENTRY_DEMON_FORM, 2, demonCleanup)
RegisterCreatureEvent(ENTRY_DEMON_FORM, 3, demonTargetDied)
RegisterCreatureEvent(ENTRY_DEMON_FORM, 4, demonCleanup)
RegisterCreatureEvent(ENTRY_DEMON_FORM, 23, demonCleanup)

-- npc_greyheart_spellbinder (21806) — see header.

-- C++ Initialize(): mindblast urand(3000,8000), earthshock
-- urand(5000,10000).
local function freshBinderState()
    return {
        mindblast = 3000 + math.random(0, 4999),
        earthshock = 5000 + math.random(0, 4999),
    }
end

local function binderTick(creature, guid)
    local st = binderState[guid]
    if not st then
        return
    end

    -- Mind blast: {3s,8s} init then {10s,15s} — random alive
    -- player pick (C++ SelectTarget(Random, 0)); nil pick casts
    -- nothing, timer re-arms (C++-exact).
    if st.mindblast <= 1000 then
        local pick = pickRandomAlivePlayer(creature)
        if pick then
            creature:CastSpell(pick, SPELL_MINDBLAST)
        end
        st.mindblast = 10000 + math.random(0, 4999)
    else
        st.mindblast = st.mindblast - 1000
    end

    -- Earthshock arm: the C++ scans every player for a non-null
    -- GetCurrentSpell (cast detection) and no cast-Spell bridge
    -- exists — unmodeled, header. Timer bookkeeping kept so the
    -- schedule stays C++-shaped if a bridge lands.
    if st.earthshock <= 1000 then
        st.earthshock = 8000 + math.random(0, 6999)
    else
        st.earthshock = st.earthshock - 1000
    end
end

local function binderEnterCombat(event, creature)
    local guid = creature:GetGUID()
    resetCreature(binderTimers, binderState, guid)
    binderState[guid] = freshBinderState()
    binderTimers[guid] = CreateLuaEvent(function()
        binderTick(creature, guid)
    end, 1000)
end

local function binderCleanup(event, creature)
    resetCreature(binderTimers, binderState, creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_SPELLBINDER, 1, binderEnterCombat)
RegisterCreatureEvent(ENTRY_SPELLBINDER, 2, binderCleanup)
RegisterCreatureEvent(ENTRY_SPELLBINDER, 4, binderCleanup)
RegisterCreatureEvent(ENTRY_SPELLBINDER, 23, binderCleanup)

-- npc_inner_demon (21857) — see header.

-- C++ Initialize(): shadowBolt 10000, link 1000.
local function freshInnerState()
    return {
        shadowBolt = 10000,
        link = 1000,
    }
end

local function innerTick(creature, guid)
    local st = innerState[guid]
    if not st then
        return
    end

    -- Soul link 1s: triggered DoCastVictim (felmyst convention —
    -- nil takes the victim arm).
    if st.link <= 1000 then
        creature:CastSpell(nil, SPELL_SOUL_LINK, true)
        st.link = 1000
    else
        st.link = st.link - 1000
    end

    -- Demonic alignment upkeep: re-cast while the aura is absent
    -- (C++-exact), triggered.
    if not creature:HasAura(SPELL_DEMONIC_ALIGNMENT) then
        creature:CastSpell(creature, SPELL_DEMONIC_ALIGNMENT, true)
    end

    -- Shadow bolt 10s: non-triggered DoCastVictim.
    if st.shadowBolt <= 1000 then
        creature:CastSpell(nil, SPELL_SHADOWBOLT)
        st.shadowBolt = 10000
    else
        st.shadowBolt = st.shadowBolt - 1000
    end
end

local function innerEnterCombat(event, creature)
    local guid = creature:GetGUID()
    resetCreature(innerTimers, innerState, guid)
    innerState[guid] = freshInnerState()
    innerTimers[guid] = CreateLuaEvent(function()
        innerTick(creature, guid)
    end, 1000)
end

local function innerCleanup(event, creature)
    resetCreature(innerTimers, innerState, creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_INNER_DEMON, 1, innerEnterCombat)
RegisterCreatureEvent(ENTRY_INNER_DEMON, 2, innerCleanup)
RegisterCreatureEvent(ENTRY_INNER_DEMON, 4, innerCleanup)
RegisterCreatureEvent(ENTRY_INNER_DEMON, 23, innerCleanup)
