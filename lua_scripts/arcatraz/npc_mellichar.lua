-- Warden Mellichar (The Arcatraz, Tempest Keep) --
-- Lua port of src/server/scripts/Outland/TempestKeep/
-- arcatraz/arcatraz.cpp (npc_warden_mellichar — the AI class
-- AddSC_arcatraz registers alongside npc_millhouse_manastorm)
-- and arcatraz.h (AZDataTypes / AZCreatureIds /
-- AZGameObjectIds). Entry: NPC_MELLICHAR = 20904.
-- The event machine is PORTED whole: MoveInLOS trigger
-- (ev27), JustEngagedWith, the 7-phase UpdateAI poller
-- (1s CreateLuaEvent, millhouse precedent — every C++
-- timer is a multiple of 1000), DoPrepareForPhase, the
-- summon arms, JustSummoned, the FAIL -> Reset leg, and
-- the Reset arms (ev23). No RegisterLuaBoss: Mellichar
-- is not a BossAI creature.
-- Instance-data twin: there is no instance GetData/SetData
-- bridge (instance_arcatraz.lua), so DATA_WARDEN_1..5 live
-- on Mellichar's own per-creature ScriptData store
-- (creature:SetData/GetData, lua_creature_events.go:792/
-- 809) with every C++ instance writer redirected onto it:
-- Mellichar's DoPrepareForPhase writes IN_PROGRESS;
-- Millhouse's phase-7 SetData(DATA_WARDEN_2, DONE)
-- (arcatraz.cpp:180) writes it from npc_millhouse_
-- manastorm.lua; the wave-mob deaths write DONE per TDB
-- 25101 smart_scripts (20905/20906 ev6 -> 5 = 3,
-- 20908/20909 ev6 -> 7 = 3, 20910/20911 ev6 -> 8 = 3).
-- The only other C++ readers are Millhouse's Reset gate
-- (replay model in its file; the ev5 respawn check reads
-- Mellichar's store) and the instance SetData override
-- itself. The override's pod-open side effect
-- (instance_arcatraz.cpp:102-115: IN_PROGRESS opens the
-- phase's stasis pod) is ported explicitly in
-- doPrepareForPhase via FindNearestGameObject + SetGoState
-- (ACTIVE = 0, the instance_arcatraz.lua door precedent);
-- its SetBossState NOT_STARTED/FAIL warden-reset leg
-- (:167-177) is ported as the store clear inside doReset
-- (Reset() calls SetBossState(NOT_STARTED) in C++).
-- Trigger (ev27): the Go sight scan enforces the C++
-- CanStartAttack/Z/LOS/dist arms before firing (the
-- NON_ATTACKABLE flag is not a scan gate — creaturemotion.
-- go:9438), so the handler adds only the C++-only gates:
-- !IsRunning, no victim, player-only, and the attack-
-- radius arm (GetAttackDistance/10, arcatraz.cpp:339-343;
-- GetDistance twin per the maiden/moroes precedent).
-- Every path returns true (veto): C++ MoveInLOS calls
-- JustEngagedWith instead of aggroing, and AttackStart is
-- an empty override — Mellichar never takes a victim.
-- JustSummoned (ev19): DoZoneInCombat on the summon +
-- AttackStart on a random alive player in 100yd
-- (SelectTarget(Random, 0, 100.0f, true); GetPlayersInRange
-- (100) defaults dead = 1 = alive only).
-- Talk: YELL_INTRO1 = 0 / INTRO2 = 1 / RELEASE1 = 2 /
-- RELEASE2A = 3 / RELEASE2B = 4 / RELEASE3 = 5 /
-- RELEASE4 = 6 / WELCOME = 7. Target spells are self-casts
-- (DoCastAOE convention); the 36852/36854/36856/36857/36858
-- + 36849 casts are dormant until Spell.dbc ships, like
-- the pre-existing OMEGA cast.
-- STAY-OUTS: wave-mob SmartAI combat casts (36906/39023/
-- 36907, 36908/36909/36910, 38642/36915/39028/36914/19714/
-- 32691, 36917, 22560/39033/22559/39037/22642/39036/22561/
-- 22558/39034/39038/13737) — SmartAI has no Lua model;
-- wave-mob aggro arms (SetData x = 1) — redundant, the
-- prepare arm already writes IN_PROGRESS before the
-- summon; wave-mob evade arms (SetData(DATA_CONVERSATION,
-- 4) = 2) — the only reader is Soccothrates's MoveInLOS
-- gate (== NOT_STARTED), unreachable once the waves exist
-- (both wardens are dead before the event starts).

local ENTRY_MELLICHAR = 20904
local DATA_HARBINGER_SKYRISS = 3 -- arcatraz.h:34
local DATA_WARDEN_1 = 5 -- arcatraz.h:42
local DATA_WARDEN_2 = 6
local DATA_WARDEN_3 = 7
local DATA_WARDEN_4 = 8
local DATA_WARDEN_5 = 9
local DATA_MELLICHAR = 10 -- arcatraz.h:43

local ENTRY_TRICKSTER = 20905 -- arcatraz.cpp:248
local ENTRY_PH_HUNTER = 20906
local ENTRY_MILLHOUSE = 20977
local ENTRY_AKKIRIS = 20908
local ENTRY_SULFURON = 20909
local ENTRY_TW_DRAK = 20910
local ENTRY_BL_DRAK = 20911
local ENTRY_SKYRISS = 20912

local SPELL_TARGET_ALPHA = 36856 -- arcatraz.cpp:263
local SPELL_TARGET_BETA = 36854
local SPELL_TARGET_DELTA = 36857
local SPELL_TARGET_GAMMA = 36858
local SPELL_TARGET_OMEGA = 36852
local SPELL_BUBBLE_VISUAL = 36849

local GO_STASIS_POD_ALPHA = 183961 -- arcatraz.h:59
local GO_STASIS_POD_BETA = 183963
local GO_STASIS_POD_DELTA = 183964
local GO_STASIS_POD_GAMMA = 183962
local GO_STASIS_POD_OMEGA = 183965
local GO_WARDENS_SHIELD = 184802

local GO_STATE_ACTIVE = 0 -- door precedent, instance_arcatraz.lua
local GO_STATE_READY = 1

local SPELL_AURA_DUMMY = 4 -- SpellAuraDefines.h:84

local UNIT_FIELD_FLAGS = 59 -- OBJECT_END(0x6)+0x35, UpdateFields.h:117
local UNIT_FLAG_NON_ATTACKABLE = 0x2 -- UnitDefines.h:125

local BOSS_NOT_STARTED = 0 -- InstanceScript.h:68-76
local BOSS_IN_PROGRESS = 1
local BOSS_FAIL = 2
local BOSS_DONE = 3

local SUMMON_TIMED_OR_DEAD_DESPAWN = 0 -- TempSummonType
local SUMMON_DESPAWN_MS = 600000 -- 10min, arcatraz.cpp:432-476

local YELL_INTRO1 = 0 -- arcatraz.cpp:234-244
local YELL_INTRO2 = 1
local YELL_RELEASE1 = 2
local YELL_RELEASE2A = 3
local YELL_RELEASE2B = 4
local YELL_RELEASE3 = 5
local YELL_RELEASE4 = 6
local YELL_WELCOME = 7

-- Per-GUID event state (C++ Initialize() locals): running =
-- IsRunning, canSpawn = CanSpawn, timer = EventProgress_Timer,
-- phase = Phase. The C++ ctor/Initialize() values are
-- running = false / canSpawn = false / timer = 22000 /
-- phase = 1 (arcatraz.cpp:284-292).
local phaseState = {}
local poller = {}

local function cancelPoller(guid)
    local id = poller[guid]
    if id then
        RemoveEventById(id)
        poller[guid] = nil
    end
end

local function setGoState(creature, entry, state)
    local go = creature:FindNearestGameObject(entry, 1000)
    if go then
        go:SetGoState(state)
    end
end

-- C++ Reset() (arcatraz.cpp:303-312): Initialize() + SetFlag
-- (NON_ATTACKABLE) + DoCast(OMEGA) + SetBossState(NOT_STARTED).
-- The warden-store clear rides the instance SetBossState
-- override (instance_arcatraz.cpp:167-177: NOT_STARTED/FAIL
-- resets DATA_WARDEN_1..5). The ~= DONE guard is the
-- pre-existing ev23 death pre-fire guard (C++ Reset() never
-- runs on death; see the ev23 handler below).
local function doReset(creature, guid)
    cancelPoller(guid)
    phaseState[guid] = nil
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
    creature:CastSpell(creature, SPELL_TARGET_OMEGA)
    if creature:GetBossState(DATA_HARBINGER_SKYRISS) ~= BOSS_DONE then
        creature:SetBossState(DATA_HARBINGER_SKYRISS, BOSS_NOT_STARTED)
        creature:SetData(DATA_WARDEN_1, BOSS_NOT_STARTED)
        creature:SetData(DATA_WARDEN_2, BOSS_NOT_STARTED)
        creature:SetData(DATA_WARDEN_3, BOSS_NOT_STARTED)
        creature:SetData(DATA_WARDEN_4, BOSS_NOT_STARTED)
        creature:SetData(DATA_WARDEN_5, BOSS_NOT_STARTED)
    end
end

-- C++ DoPrepareForPhase() (arcatraz.cpp:371-400): interrupt +
-- drop DUMMY auras, then per-phase the target-spell self-cast,
-- the instance SetData(IN_PROGRESS) (here: the per-creature
-- store), and the stasis-pod open (the instance SetData
-- override's HandleGameObject leg, instance_arcatraz.cpp:
-- 106-113). Phase 2 also closes the Wardens Shield
-- (HandleGameObject(shield, false), :380-382 — the same arm
-- runs in JustEngagedWith below). Phase 4 has no switch arm.
-- CanSpawn = true is set at the end for every phase (:397).
local function doPrepareForPhase(creature, st)
    local phase = st.phase
    creature:InterruptNonMeleeSpells(false)
    creature:RemoveAurasByType(SPELL_AURA_DUMMY)
    if phase == 2 then
        creature:CastSpell(creature, SPELL_TARGET_ALPHA)
        creature:SetData(DATA_WARDEN_1, BOSS_IN_PROGRESS)
        setGoState(creature, GO_WARDENS_SHIELD, GO_STATE_READY)
        setGoState(creature, GO_STASIS_POD_ALPHA, GO_STATE_ACTIVE)
    elseif phase == 3 then
        creature:CastSpell(creature, SPELL_TARGET_BETA)
        creature:SetData(DATA_WARDEN_2, BOSS_IN_PROGRESS)
        setGoState(creature, GO_STASIS_POD_BETA, GO_STATE_ACTIVE)
    elseif phase == 5 then
        creature:CastSpell(creature, SPELL_TARGET_DELTA)
        creature:SetData(DATA_WARDEN_3, BOSS_IN_PROGRESS)
        setGoState(creature, GO_STASIS_POD_DELTA, GO_STATE_ACTIVE)
    elseif phase == 6 then
        creature:CastSpell(creature, SPELL_TARGET_GAMMA)
        creature:SetData(DATA_WARDEN_4, BOSS_IN_PROGRESS)
        setGoState(creature, GO_STASIS_POD_GAMMA, GO_STATE_ACTIVE)
    elseif phase == 7 then
        creature:SetData(DATA_WARDEN_5, BOSS_IN_PROGRESS)
        setGoState(creature, GO_STASIS_POD_OMEGA, GO_STATE_ACTIVE)
    end
    st.canSpawn = true
end

-- C++ CanProgress() (arcatraz.cpp:350-369): phase 4 is
-- unconditional; phases 1/2 read the Skyriss boss state,
-- phases 3/5/6/7 read the warden store (DONE = 3).
local function canProgress(creature, phase)
    if phase == 7 then
        return creature:GetData(DATA_WARDEN_4) == BOSS_DONE
    elseif phase == 6 then
        return creature:GetData(DATA_WARDEN_3) == BOSS_DONE
    elseif phase == 5 then
        return creature:GetData(DATA_WARDEN_2) == BOSS_DONE
    elseif phase == 4 then
        return true
    elseif phase == 3 then
        return creature:GetData(DATA_WARDEN_1) == BOSS_DONE
    elseif phase == 2 then
        return creature:GetBossState(DATA_HARBINGER_SKYRISS) == BOSS_IN_PROGRESS
    elseif phase == 1 then
        return creature:GetBossState(DATA_HARBINGER_SKYRISS) == BOSS_IN_PROGRESS
    end
    return false
end

-- C++ UpdateAI (arcatraz.cpp:402-481) at 1s granularity: the
-- timer is checked once per tick (diff = 1000); the FAIL leg
-- runs first on expiry, then the CanSpawn summon leg (which
-- advances the phase), then the CanProgress leg (phase 1
-- advances the phase itself; phases 2-7 advance via the
-- next CanSpawn leg). When neither leg fires the timer
-- stays expired, so the next tick re-evaluates immediately
-- — the C++ shape, where UpdateAI runs every server tick.
local function updateTick(creature, guid)
    local st = phaseState[guid]
    if not st or not st.running then
        return
    end
    if creature:IsDead() then
        cancelPoller(guid)
        phaseState[guid] = nil
        return
    end
    if st.timer > 1000 then
        st.timer = st.timer - 1000
        return
    end
    if creature:GetBossState(DATA_HARBINGER_SKYRISS) == BOSS_FAIL then
        doReset(creature, guid)
        return
    end
    if st.canSpawn then
        -- Continue beam omega pod, unless about to summon skyriss.
        if st.phase ~= 7 then
            creature:CastSpell(creature, SPELL_TARGET_OMEGA)
        end
        if st.phase == 2 then
            if math.random(0, 1) == 0 then
                creature:SummonCreature(ENTRY_TRICKSTER, 472.231, -150.86, 42.6573, 3.10669, SUMMON_TIMED_OR_DEAD_DESPAWN, SUMMON_DESPAWN_MS)
            else
                creature:SummonCreature(ENTRY_PH_HUNTER, 472.231, -150.86, 42.6573, 3.10669, SUMMON_TIMED_OR_DEAD_DESPAWN, SUMMON_DESPAWN_MS)
            end
        elseif st.phase == 3 then
            creature:SummonCreature(ENTRY_MILLHOUSE, 417.242, -149.795, 42.6548, 0.191986, SUMMON_TIMED_OR_DEAD_DESPAWN, SUMMON_DESPAWN_MS)
        elseif st.phase == 4 then
            creature:Talk(YELL_RELEASE2B)
        elseif st.phase == 5 then
            if math.random(0, 1) == 0 then
                creature:SummonCreature(ENTRY_AKKIRIS, 420.851, -174.337, 42.6655, 0.122173, SUMMON_TIMED_OR_DEAD_DESPAWN, SUMMON_DESPAWN_MS)
            else
                creature:SummonCreature(ENTRY_SULFURON, 420.851, -174.337, 42.6655, 0.122173, SUMMON_TIMED_OR_DEAD_DESPAWN, SUMMON_DESPAWN_MS)
            end
        elseif st.phase == 6 then
            if math.random(0, 1) == 0 then
                creature:SummonCreature(ENTRY_TW_DRAK, 470.364, -174.656, 42.6753, 3.59538, SUMMON_TIMED_OR_DEAD_DESPAWN, SUMMON_DESPAWN_MS)
            else
                creature:SummonCreature(ENTRY_BL_DRAK, 470.364, -174.656, 42.6753, 3.59538, SUMMON_TIMED_OR_DEAD_DESPAWN, SUMMON_DESPAWN_MS)
            end
        elseif st.phase == 7 then
            creature:SummonCreature(ENTRY_SKYRISS, 446.086, -182.506, 44.0852, 1.5708, SUMMON_TIMED_OR_DEAD_DESPAWN, SUMMON_DESPAWN_MS)
            creature:Talk(YELL_WELCOME)
        end
        st.canSpawn = false
        st.phase = st.phase + 1
    end
    if canProgress(creature, st.phase) then
        if st.phase == 1 then
            creature:Talk(YELL_INTRO2)
            st.timer = 10000
            st.phase = st.phase + 1
        elseif st.phase == 2 then
            creature:Talk(YELL_RELEASE1)
            doPrepareForPhase(creature, st)
            st.timer = 7000
        elseif st.phase == 3 then
            creature:Talk(YELL_RELEASE2A)
            doPrepareForPhase(creature, st)
            st.timer = 10000
        elseif st.phase == 4 then
            doPrepareForPhase(creature, st)
            st.timer = 15000
        elseif st.phase == 5 then
            creature:Talk(YELL_RELEASE3)
            doPrepareForPhase(creature, st)
            st.timer = 15000
        elseif st.phase == 6 then
            creature:Talk(YELL_RELEASE4)
            doPrepareForPhase(creature, st)
            st.timer = 15000
        elseif st.phase == 7 then
            doPrepareForPhase(creature, st)
            st.timer = 15000
        end
    end
end

RegisterCreatureEvent(ENTRY_MELLICHAR, 5, function(_, creature)
    creature:SetGuidData(DATA_MELLICHAR)
end)

RegisterCreatureEvent(ENTRY_MELLICHAR, 23, function(_, creature)
    if creature:IsDead() then
        return
    end
    doReset(creature, creature:GetGUIDLow())
end)

-- C++ MoveInLineOfSight (arcatraz.cpp:324-342) -> JustEngagedWith
-- (:344-352). Every path vetoes the engine engage: C++ calls
-- JustEngagedWith instead of aggroing, and AttackStart is an
-- empty override. The Go sight scan already enforces the
-- CanStartAttack/Z/LOS arms, so the handler adds the C++-only
-- gates: not already running, no victim, player-only, and the
-- attack-radius arm (GetAttackDistance/10; the GetDistance
-- twin follows the maiden/moroes precedent: exact distance
-- minus both combat reaches, player reach 1.5).
RegisterCreatureEvent(ENTRY_MELLICHAR, 27, function(_, creature, who)
    local guid = creature:GetGUIDLow()
    local st = phaseState[guid]
    if st and st.running then
        return true
    end
    if creature:IsDead() then
        return true
    end
    if creature:GetVictim() ~= nil then
        return true
    end
    if who:GetObjectType() ~= "Player" then
        return true
    end
    local attackRadius = creature:GetAttackDistance(who) / 10
    local dist = creature:GetDistance(who) - creature:GetCombatReach() - 1.5
    if dist > attackRadius then
        return true
    end
    if not creature:IsWithinLOS(who) then
        return true
    end
    creature:Talk(YELL_INTRO1)
    creature:CastSpell(creature, SPELL_BUBBLE_VISUAL)
    creature:SetBossState(DATA_HARBINGER_SKYRISS, BOSS_IN_PROGRESS)
    setGoState(creature, GO_WARDENS_SHIELD, GO_STATE_READY)
    phaseState[guid] = { running = true, canSpawn = false, timer = 22000, phase = 1 }
    poller[guid] = CreateLuaEvent(function()
        updateTick(creature, guid)
    end, 1000, 0)
    return true
end)

-- C++ JustSummoned (arcatraz.cpp:354-360): DoZoneInCombat on the
-- summon, then AttackStart on a random player in 100yd.
RegisterCreatureEvent(ENTRY_MELLICHAR, 19, function(_, creature, summon)
    summon:DoZoneInCombat()
    local players = creature:GetPlayersInRange(100)
    if players ~= nil and #players > 0 then
        summon:AttackStart(players[math.random(#players)])
    end
end)

-- C++ JustDied: none — but a dead creature's UpdateAI stops
-- running, so the poller is cancelled here (Skyriss kills
-- Mellichar in his intro; boss_harbinger_skyriss.lua).
RegisterCreatureEvent(ENTRY_MELLICHAR, 4, function(_, creature)
    local guid = creature:GetGUIDLow()
    cancelPoller(guid)
    phaseState[guid] = nil
end)

-- Wave-mob death relays (TDB 25101 smart_scripts): each wave
-- mob's On Just Died arm writes its warden state DONE to the
-- instance store — redirected here to Mellichar's store (the
-- dalliah relay precedent: FindNearestCreature + SetData).
-- 20905/20906 -> DATA_WARDEN_1, 20908/20909 -> DATA_WARDEN_3,
-- 20910/20911 -> DATA_WARDEN_4.
local WAVE_DONE = {}
WAVE_DONE[ENTRY_TRICKSTER] = DATA_WARDEN_1
WAVE_DONE[ENTRY_PH_HUNTER] = DATA_WARDEN_1
WAVE_DONE[ENTRY_AKKIRIS] = DATA_WARDEN_3
WAVE_DONE[ENTRY_SULFURON] = DATA_WARDEN_3
WAVE_DONE[ENTRY_TW_DRAK] = DATA_WARDEN_4
WAVE_DONE[ENTRY_BL_DRAK] = DATA_WARDEN_4

for _, entry in ipairs({ ENTRY_TRICKSTER, ENTRY_PH_HUNTER, ENTRY_AKKIRIS, ENTRY_SULFURON, ENTRY_TW_DRAK, ENTRY_BL_DRAK }) do
    local waveEntry = entry
    RegisterCreatureEvent(waveEntry, 4, function(_, creature)
        local mellichar = creature:FindNearestCreature(ENTRY_MELLICHAR, 200)
        if mellichar ~= nil then
            mellichar:SetData(WAVE_DONE[waveEntry], BOSS_DONE)
        end
    end)
end
