-- Thaddius (Naxxramas) — Lua port of
-- src/server/scripts/Northrend/Naxxramas/boss_thaddius.cpp
-- (boss_thaddius (BossAI, BOSS_THADDIUS); npc_stalagg /
-- npc_feugen / npc_tesla (ScriptedAI); spell_thaddius_polarity_charge /
-- spell_thaddius_polarity_shift / spell_thaddius_magnetic_pull
-- (SpellScripts via SpellScriptLoader); at_thaddius_entrance
-- (OnlyOnceAreaTriggerScript); achievement_thaddius_shocking
-- (AchievementCriteriaScript); AddSC_boss_thaddius registers all nine;
-- loader decl 73 / call 268 per northrend_script_loader.cpp — the
-- FIFTEENTH Naxxramas group in AddNorthrendScripts(), immediately after
-- AddSC_boss_gothik() (decl 72 / call 267), under the "// Naxxramas"
-- marker; the call after it is AddSC_naxxramas() (decl 74 / call 269),
-- then AddSC_instance_naxxramas() (decl 75 / call 270) — loader order
-- confirmed this run; the checkpoint sequence (gothik -> thaddius) is
-- followed).
-- Entry: 15928 Thaddius (naxxramas.h NPC_THADDIUS line 98 — kalecgos
-- pass; instance_naxxramas.cpp binds NPC_THADDIUS -> ThaddiusGUID
-- (line 164) and DATA_THADDIUS -> ThaddiusGUID (line 324); the
-- CreatureScript ScriptName binding is instance-shimmed, the
-- creature_template binding DB-side as usual).
-- Sole-source verified: whole-server-tree grep for each of the nine
-- script names hits boss_thaddius.cpp only (loader carries only the
-- decl/call lines); zero sql/ hits. No thaddius lua existed.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady. Victim
-- gating: C++ KilledUnit checks TYPEID_PLAYER — Lua expresses it via
-- victim:GetObjectType() (nalorakk / kelthuzad / gothik player-gate
-- precedent). C++ has no JustEngagedWith override for Thaddius —
-- SAY_AGGRO (1) fires from EVENT_TRANSITION_3, not from combat start —
-- so the inherited BossAI::JustEngagedWith runs at first AtEngage
-- (ScriptedCreature.h:329): SetCombatPulseDelay(5) + me-form DoZoneInCombat
-- PORTED 2026-11-03 (ScriptedCreature.cpp:540/542; anubrekhan/gothik/
-- sapphiron precedent); SAY_AGGRO still rides EVENT_TRANSITION_3, not
-- combat start; the setActive legs have no bridge. EVENT_SHIFT is DoCastAOE
-- (bridged as self-cast per the boss_ai.go DoCastAOE lowering —
-- selin_fireheart / mr_smite precedent), and EVENT_CHAIN /
-- EVENT_BERSERK are scheduled only from EVENT_TRANSITION_3, which
-- belongs to the phase machine (ported 2026-11-05 — porting them from
-- combat start would deviate from C++: pre-transition Thaddius casts
-- nothing).
-- Ported arms (C++-exact for all modeled arms) — phase machine PORTED
-- 2026-11-05 (tick 248: creature:DoAction IS bridged — ev38 dispatch,
-- lua_creature_events.go:814, kelthuzad/razorgore precedent — the "no
-- cross-AI bridge" claims are retracted):
-- boss_thaddius: inherited BossAI::JustEngagedWith base arms (event 1:
-- SetCombatPulseDelay(5) + me-form DoZoneInCombat) PORTED 2026-11-03;
-- KilledUnit Talk(SAY_SLAY 2) (event 3, player-gated); JustDied (event 4:
-- _JustDied SetBossState DONE + SetFarVisible(false) on thaddius/stalagg/
-- feugen via GetInstanceCreature + Talk(SAY_DEATH 4); the setActive legs
-- have no bridge); InitializeAI SetPhase(PHASE_NOT_ENGAGED) +
-- SetCombatMovement(false) + JustAppeared ResetEncounter (_Reset
-- SetBossState NOT_STARTED + REACT_PASSIVE + phase reset) on ev5 (the
-- Respawn(130958/130959) leg has no bridge); DoAction ev38: AGGRO legs
-- (SetPhase PETS + shockingEligibility SetData(1,1) + SetBossState
-- IN_PROGRESS + SetFarVisible(true) + DoZoneInCombat; the setActive legs
-- have no bridge; CheckRequiredBosses is vacuous — base returns true,
-- instance_naxxramas.cpp has no override), DIED legs (cross-AI
-- REVIVING_FX + 5s revive schedules or Transition()), RESET legs
-- (BeginResetEncounter), POLARITY_CROSSED (SetData(1,0)); Transition()
-- (NOT_SELECTABLE removal + 10s/12s/14s schedules); TRANSITION_1/2/3
-- (coil DoActions + spark visual + aura/flag/immunity legs +
-- DoZoneInCombat + SetPhase THADDIUS + Talk(SAY_AGGRO 1)); EVENT_ENGAGE
-- 2s (REACT_AGGRESSIVE), EVENT_ENABLE_BALL_LIGHTNING 5s, EVENT_SHIFT 10s/
-- 30s (CastStop + DoCastAOE POLARITY_SHIFT 28089 self-cast + 3s SHIFT_TALK
-- Talk(SAY_ELECT 3) + Talk(EMOTE_POLARITY_SHIFTED 6)), EVENT_CHAIN 10s/20s
-- init + randtime(10,20) repeat with the polarity-shift-cast delay leg
-- (GetCurrentSpell(1) entry check, 3s defer), EVENT_BERSERK 6min
-- (CastStop + DoCastSelf 27680); EnterEvadeMode ev2 (NO_HOSTILES retry leg:
-- ballLightningEnabled=true, no reset — in-dungeon evades are always
-- no-hostiles, the leash arm is skipped in dungeons — else
-- BeginResetEncounter when TRANSITION or (THADDIUS and alive)); UpdateAI
-- ball-lightning leg on ev7 (100ms tick: melee-range clears
-- ballLightningEnabled and lets the engine melee; out of melee with
-- ballLightningUnlocked casts BALL_LIGHTNING 28299 on a random threat
-- target; IsCasting gates the cast, isAttackReady has no bridge and is
-- vacuous out of melee — the swing timer is elapsed and never resets
-- without a melee swing); GetData(DATA_POLARITY_CROSSED) via the
-- SetData/GetData latch (kelthuzad tick-235 convention). Timers are
-- cancelled in BeginResetEncounter and on death (ev4); ev2 does NOT cancel
-- (the EnterEvadeMode retry must keep the machine) and ev23 has no handler
-- (C++ Reset() is empty; ev23 fires before ev2 in triggerCreatureEvade).
-- SAY_GREET (0) fires from at_thaddius_entrance — no area-trigger bridge
-- — documented-only. Of the three SpellScripts, polarity_shift and
-- magnetic_pull are PORTED (polarityShiftHit ev5 + magneticPullCast
-- ev1 in npc_feugen.lua — the "no SpellScript binding bridge
-- (razelikh precedent)" rationale is STALE: ev1/ev5 fire on the
-- creature cast path, boss_ai.go:3154/3427); polarity_charge stays
-- documented-only (ev7 is creature-path-only, but the caster is the
-- charged player; no GetTriggeringSpell bridge). The area trigger (no
-- area-trigger bridge) and the achievement (no achievement bridge —
-- kelthuzad precedent; the GetData latch it reads IS ported above)
-- stay documented-only.
-- Unmodeled (no bridges — documented, not wired):
-- boss_thaddius: setActive(true/false) on thaddius/stalagg/feugen (no
-- bridge); BeginResetEncounter DoRemoveAurasDueToSpellOnPlayers(28059/
-- 28084) (no bridge) + the DespawnOrUnsummon 30s respawn leg (the bridge
-- takes the delay only) + the ResetEncounter Respawn(130958/130959)
-- map-level spawn-ID respawn leg (no bridge); CanAIAttack — no hook
-- bridge (the melee/ball-lightning gate is modeled via the ev7
-- IsWithinMeleeRange leg above); isAttackReady (no bridge — vacuous out
-- of melee, see above); EVENT_ENGAGE / EVENT_ENABLE_BALL_LIGHTNING fire
-- ungated on victim (one-shot state flags; C++ would delay them until a
-- victim exists — unobservable); the pets' and tesla's arms live in
-- npc_stalagg.lua / npc_feugen.lua / npc_tesla.lua.
local ENTRY_THADDIUS = 15928

local SAY_SLAY = 2
local SAY_DEATH = 4

local BOSS_THADDIUS = 9 -- naxxramas.h NAXEncounter (THADDIUS 9)
local DATA_THADDIUS = 9 -- naxxramas.h NAXData64
local DATA_FEUGEN = 11
local DATA_STALAGG = 12
local BOSS_STATE_NOT_STARTED = 0
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_DONE = 3

local PHASE_NOT_ENGAGED = 1
local PHASE_PETS = 2
local PHASE_TRANSITION = 3
local PHASE_THADDIUS = 4

local ACTION_BEGIN_RESET_ENCOUNTER = 0
local ACTION_FEUGEN_DIED = 1
local ACTION_STALAGG_DIED = 2
local ACTION_FEUGEN_RESET = 3
local ACTION_STALAGG_RESET = 4
local ACTION_FEUGEN_AGGRO = 5
local ACTION_STALAGG_AGGRO = 6
local ACTION_FEUGEN_REVIVING_FX = 7
local ACTION_STALAGG_REVIVING_FX = 8
local ACTION_FEUGEN_REVIVED = 9
local ACTION_STALAGG_REVIVED = 10
local ACTION_TRANSITION = 11
local ACTION_TRANSITION_2 = 12
local ACTION_TRANSITION_3 = 13
local ACTION_POLARITY_CROSSED = 14

local DATA_POLARITY_CROSSED = 1

local SAY_AGGRO = 1
local SAY_ELECT = 3
local EMOTE_POLARITY_SHIFTED = 6

local SPELL_THADDIUS_INACTIVE_VISUAL = 28160
local SPELL_THADDIUS_SPARK_VISUAL = 28136
local SPELL_BALL_LIGHTNING = 28299
local SPELL_CHAIN_LIGHTNING = 28167
local SPELL_BERSERK = 27680
local SPELL_POLARITY_SHIFT = 28089
local SPELL_POSITIVE_CHARGE_APPLY = 28059
local SPELL_NEGATIVE_CHARGE_APPLY = 28084
local SPELL_POSITIVE_CHARGE_AMP = 29659
local SPELL_NEGATIVE_CHARGE_AMP = 29660

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000
local UNIT_FLAG_STUNNED = 0x00040000
local UNIT_FLAG_IMMUNE_TO_PC = 0x00000100
local REACT_PASSIVE = 0
local REACT_AGGRESSIVE = 2

-- Phase-machine state (C++ boss_thaddius members, Lua-local per GUID).
local thaddiusPhase = {}
local thaddiusStalaggAlive = {}
local thaddiusFeugenAlive = {}
local thaddiusBallUnlocked = {}
local thaddiusBallEnabled = {}
local thaddiusResetting = {}

-- CreateLuaEvent timer harness (GUID-keyed timers table) — the maiden /
-- gargolmar convention. Cancelled in BeginResetEncounter and on death
-- (ev4); ev2 does NOT cancel (the EnterEvadeMode retry must keep the
-- machine) and ev23 has no handler (C++ Reset() is empty; ev23 fires
-- before ev2 in triggerCreatureEvade).
local timers = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

local function schedule(guid, key, delay, fn)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

-- C++ SelectTarget(SelectTargetMethod::Random): random alive unit from
-- the threat list (playerOnly defaults false).
local function randomThreatTarget(creature, maxDist)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:IsAlive() then
            if maxDist <= 0 or creature:GetDistance2d(t) <= maxDist then
                candidates[#candidates + 1] = t
            end
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ inherited BossAI::JustEngagedWith (no override in boss_thaddius):
-- SetCombatPulseDelay(5) + me-form DoZoneInCombat at first engage; no Talk
-- here (SAY_AGGRO rides EVENT_TRANSITION_3) and no timers (all ride the
-- phase machine below, ported 2026-11-05).
local function thaddiusEnterCombat(event, creature, target)
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542)
end

-- C++ KilledUnit: Talk(SAY_SLAY 2) only when victim->GetTypeId() ==
-- TYPEID_PLAYER (nalorakk / kelthuzad / gothik player-gate variant).
local function thaddiusTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ JustDied: Talk(SAY_DEATH 4) + _JustDied / SetFarVisible legs
-- (ported in thaddiusDied below; the setActive legs have no bridge).
local GAMEOBJECT_FLAGS = 9
local GO_FLAG_NOT_SELECTABLE = 16
local GO_NAXX_PORTAL_CONSTRUCT = 181576 -- naxxramas.h:166

local function thaddiusDied(event, creature, killer)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    -- C++ JustDied (boss_thaddius.cpp:208-222): _JustDied (SetBossState
    -- DONE, ScriptedCreature.cpp:513-520; moroes/faerlina precedent) +
    -- SetFarVisible(false) on thaddius/stalagg/feugen (the setActive
    -- legs have no bridge).
    creature:SetBossState(BOSS_THADDIUS, BOSS_STATE_DONE)
    creature:SetFarVisible(false)
    local stalagg = creature:GetInstanceCreature(DATA_STALAGG)
    if stalagg then
        stalagg:SetFarVisible(false)
    end
    local feugen = creature:GetInstanceCreature(DATA_FEUGEN)
    if feugen then
        feugen:SetFarVisible(false)
    end
    creature:Talk(SAY_DEATH)
    -- C++ SetBossState(BOSS_THADDIUS, DONE) legs (instance_naxxramas.cpp):
    -- construct portal flag clear + DONE flag for the instance pump's
    -- wing taunt (create-time form cannot fire — see instance file).
    local portal = creature:FindNearestGameObject(GO_NAXX_PORTAL_CONSTRUCT, 1000)
    if portal then
        portal:RemoveFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE)
    end
    local mapObj = creature:GetMap()
    if mapObj then
        local data = mapObj:GetInstanceData()
        if data then
            data.thaddiusDone = true
        end
    end
end

-- C++ UpdateAI EVENT_ENGAGE (boss_thaddius.cpp:406-408): 2s after
-- TRANSITION_3, REACT_AGGRESSIVE.
local function engageTick(creature, guid)
    creature:SetReactState(REACT_AGGRESSIVE)
end

-- C++ UpdateAI EVENT_ENABLE_BALL_LIGHTNING (:403-405): 5s after
-- TRANSITION_3, ballLightningUnlocked = true.
local function enableBallLightningTick(creature, guid)
    thaddiusBallUnlocked[guid] = true
end

-- C++ UpdateAI EVENT_SHIFT_TALK (:415-418): Talk(SAY_ELECT 3) +
-- Talk(EMOTE_POLARITY_SHIFTED 6).
local function shiftTalkTick(creature, guid)
    creature:Talk(SAY_ELECT)
    creature:Talk(EMOTE_POLARITY_SHIFTED)
end

-- C++ UpdateAI EVENT_SHIFT (:409-414): CastStop + DoCastAOE(POLARITY_SHIFT
-- 28089, self-cast) + SHIFT_TALK in 3s + SHIFT in 30s. The C++ UpdateAI
-- freezes while victimless in PHASE_THADDIUS (UpdateVictim gate) — the
-- 1s defer below is that freeze.
local function shiftTick(creature, guid)
    if not creature:GetVictim() then
        schedule(guid, "shift", 1000, function()
            shiftTick(creature, guid)
        end)
        return
    end
    creature:StopSpellCast() -- shift overrides all other spells
    creature:CastSpell(creature, SPELL_POLARITY_SHIFT)
    schedule(guid, "shifttalk", 3000, function()
        shiftTalkTick(creature, guid)
    end)
    schedule(guid, "shift", 30000, function()
        shiftTick(creature, guid)
    end)
end

-- C++ UpdateAI EVENT_CHAIN (:419-428): if thaddius is currently casting
-- POLARITY_SHIFT, delay 3s; else CastStop + DoCastVictim(CHAIN_LIGHTNING
-- 28167) + repeat randtime(10s,20s). FindCurrentSpellBySpellId rides the
-- GetCurrentSpell(1) (CURRENT_GENERIC_SPELL) entry check.
local function chainTick(creature, guid)
    local victim = creature:GetVictim()
    if not victim then
        schedule(guid, "chain", 1000, function()
            chainTick(creature, guid)
        end)
        return
    end
    local cur = creature:GetCurrentSpell(1)
    if cur and cur:GetEntry() == SPELL_POLARITY_SHIFT then
        schedule(guid, "chain", 3000, function()
            chainTick(creature, guid)
        end)
        return
    end
    creature:StopSpellCast()
    creature:CastSpell(victim, SPELL_CHAIN_LIGHTNING)
    schedule(guid, "chain", {10000, 20000}, function()
        chainTick(creature, guid)
    end)
end

-- C++ UpdateAI EVENT_BERSERK (:429-432): CastStop + DoCast(me, BERSERK
-- 27680), one-shot at 6min.
local function berserkTick(creature, guid)
    if not creature:GetVictim() then
        schedule(guid, "berserk", 1000, function()
            berserkTick(creature, guid)
        end)
        return
    end
    creature:StopSpellCast()
    creature:CastSpell(creature, SPELL_BERSERK)
end

-- C++ UpdateAI EVENT_REVIVE_FEUGEN (:356-360): feugenAlive = true +
-- feugen DoAction(ACTION_FEUGEN_REVIVED). Phase-gated: C++ schedules
-- the event with phase PHASE_PETS and EventMap skips phase-mismatched
-- events, so a second pet death inside the 5s window (Transition()
-- sets PHASE_TRANSITION) drops the pending revive.
local function reviveFeugenTick(creature, guid)
    if thaddiusPhase[guid] ~= PHASE_PETS then
        return
    end
    thaddiusFeugenAlive[guid] = true
    local feugen = creature:GetInstanceCreature(DATA_FEUGEN)
    if feugen then
        feugen:DoAction(ACTION_FEUGEN_REVIVED)
    end
end

-- C++ UpdateAI EVENT_REVIVE_STALAGG (:361-365): stalaggAlive = true +
-- stalagg DoAction(ACTION_STALAGG_REVIVED). Phase-gated as above
-- (PHASE_PETS event, dropped once Transition() sets PHASE_TRANSITION).
local function reviveStalaggTick(creature, guid)
    if thaddiusPhase[guid] ~= PHASE_PETS then
        return
    end
    thaddiusStalaggAlive[guid] = true
    local stalagg = creature:GetInstanceCreature(DATA_STALAGG)
    if stalagg then
        stalagg:DoAction(ACTION_STALAGG_REVIVED)
    end
end

-- C++ UpdateAI EVENT_TRANSITION_1 (:366-371): tesla coils overload —
-- feugen/stalagg DoAction(ACTION_TRANSITION).
local function transition1Tick(creature, guid)
    local feugen = creature:GetInstanceCreature(DATA_FEUGEN)
    if feugen then
        feugen:DoAction(ACTION_TRANSITION)
    end
    local stalagg = creature:GetInstanceCreature(DATA_STALAGG)
    if stalagg then
        stalagg:DoAction(ACTION_TRANSITION)
    end
end

-- C++ UpdateAI EVENT_TRANSITION_2 (:372-378): CastSpell(me,
-- THADDIUS_SPARK_VISUAL 28136, triggered) + feugen/stalagg
-- DoAction(ACTION_TRANSITION_2).
local function transition2Tick(creature, guid)
    creature:CastSpell(creature, SPELL_THADDIUS_SPARK_VISUAL, true)
    local feugen = creature:GetInstanceCreature(DATA_FEUGEN)
    if feugen then
        feugen:DoAction(ACTION_TRANSITION_2)
    end
    local stalagg = creature:GetInstanceCreature(DATA_STALAGG)
    if stalagg then
        stalagg:DoAction(ACTION_TRANSITION_2)
    end
end

-- C++ UpdateAI EVENT_TRANSITION_3 (:379-402): spark visual +
-- ballLightningUnlocked = false + RemoveAura(INACTIVE_VISUAL) +
-- RemoveFlag(STUNNED) + SetImmuneToPC(false) + DoZoneInCombat +
-- feugen/stalagg DoAction(ACTION_TRANSITION_3) + SetPhase(THADDIUS) +
-- Talk(SAY_AGGRO 1) + the ENGAGE/ENABLE_BALL_LIGHTNING/SHIFT/CHAIN/
-- BERSERK schedules.
local function transition3Tick(creature, guid)
    creature:CastSpell(creature, SPELL_THADDIUS_SPARK_VISUAL, true)
    thaddiusBallUnlocked[guid] = false
    creature:RemoveAura(SPELL_THADDIUS_INACTIVE_VISUAL)
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_STUNNED)
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC) -- SetImmuneToPC(false)
    creature:DoZoneInCombat()
    local feugen = creature:GetInstanceCreature(DATA_FEUGEN)
    if feugen then
        feugen:DoAction(ACTION_TRANSITION_3)
    end
    local stalagg = creature:GetInstanceCreature(DATA_STALAGG)
    if stalagg then
        stalagg:DoAction(ACTION_TRANSITION_3)
    end
    thaddiusPhase[guid] = PHASE_THADDIUS
    creature:Talk(SAY_AGGRO)
    schedule(guid, "engage", 2000, function()
        engageTick(creature, guid)
    end)
    schedule(guid, "enableball", 5000, function()
        enableBallLightningTick(creature, guid)
    end)
    schedule(guid, "shift", 10000, function()
        shiftTick(creature, guid)
    end)
    schedule(guid, "chain", {10000, 20000}, function()
        chainTick(creature, guid)
    end)
    schedule(guid, "berserk", 360000, function()
        berserkTick(creature, guid)
    end)
end

-- C++ Transition() (boss_thaddius.cpp:298-307): SetPhase(TRANSITION) +
-- NOT_SELECTABLE removal + the TRANSITION_1/2/3 schedules (10s/12s/14s).
local function transition(creature, guid)
    thaddiusPhase[guid] = PHASE_TRANSITION
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    schedule(guid, "transition1", 10000, function()
        transition1Tick(creature, guid)
    end)
    schedule(guid, "transition2", 12000, function()
        transition2Tick(creature, guid)
    end)
    schedule(guid, "transition3", 14000, function()
        transition3Tick(creature, guid)
    end)
end

-- C++ BeginResetEncounter (boss_thaddius.cpp:309-328): DONE gate +
-- DoRemoveAurasDueToSpellOnPlayers (no bridge) + DespawnOrUnsummon(0s,
-- 30s) + NOT_SELECTABLE|STUNNED + SetImmuneToPC(true) + setActive(false,
-- no bridge) + SetFarVisible(false) + feugen/stalagg
-- DoAction(ACTION_BEGIN_RESET_ENCOUNTER). The 30s respawn leg has no
-- bridge (DespawnOrUnsummon takes the delay only). C++ despawns first;
-- Go's Despawn deletes the motion synchronously, so the self-legs run
-- before the despawn here (unobservable on a despawning creature).
local function beginResetEncounter(creature, guid)
    if thaddiusResetting[guid] then
        return
    end
    if creature:GetBossState(BOSS_THADDIUS) == BOSS_STATE_DONE then
        return
    end
    thaddiusResetting[guid] = true
    cancelTimers(guid)
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE + UNIT_FLAG_STUNNED)
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC) -- SetImmuneToPC(true)
    creature:SetFarVisible(false)
    local feugen = creature:GetInstanceCreature(DATA_FEUGEN)
    if feugen then
        feugen:DoAction(ACTION_BEGIN_RESET_ENCOUNTER)
    end
    local stalagg = creature:GetInstanceCreature(DATA_STALAGG)
    if stalagg then
        stalagg:DoAction(ACTION_BEGIN_RESET_ENCOUNTER)
    end
    creature:DespawnOrUnsummon(0)
end

-- C++ boss_thaddius::DoAction (boss_thaddius.cpp:226-296), event 38.
local function thaddiusDoAction(event, creature, action)
    local guid = creature:GetGUIDLow()
    if action == ACTION_FEUGEN_RESET or action == ACTION_STALAGG_RESET then
        if thaddiusPhase[guid] ~= PHASE_NOT_ENGAGED then
            beginResetEncounter(creature, guid)
        end
    elseif action == ACTION_FEUGEN_AGGRO or action == ACTION_STALAGG_AGGRO then
        if thaddiusPhase[guid] ~= PHASE_NOT_ENGAGED then
            return
        end
        thaddiusPhase[guid] = PHASE_PETS
        creature:SetData(DATA_POLARITY_CROSSED, 1) -- shockingEligibility = true
        -- instance->CheckRequiredBosses(BOSS_THADDIUS): the base returns true
        -- and instance_naxxramas.cpp has no override — vacuous gate, no leg.
        creature:SetBossState(BOSS_THADDIUS, BOSS_STATE_IN_PROGRESS)
        -- me->setActive(true) has no bridge.
        creature:SetFarVisible(true)
        creature:DoZoneInCombat()
        local stalagg = creature:GetInstanceCreature(DATA_STALAGG)
        if stalagg then
            stalagg:SetFarVisible(true)
        end
        local feugen = creature:GetInstanceCreature(DATA_FEUGEN)
        if feugen then
            feugen:SetFarVisible(true)
        end
    elseif action == ACTION_FEUGEN_DIED then
        local feugen = creature:GetInstanceCreature(DATA_FEUGEN)
        if feugen then
            feugen:DoAction(ACTION_FEUGEN_REVIVING_FX)
        end
        thaddiusFeugenAlive[guid] = false
        if thaddiusStalaggAlive[guid] then
            schedule(guid, "revivefeugen", 5000, function()
                reviveFeugenTick(creature, guid)
            end)
        else
            transition(creature, guid)
        end
    elseif action == ACTION_STALAGG_DIED then
        local stalagg = creature:GetInstanceCreature(DATA_STALAGG)
        if stalagg then
            stalagg:DoAction(ACTION_STALAGG_REVIVING_FX)
        end
        thaddiusStalaggAlive[guid] = false
        if thaddiusFeugenAlive[guid] then
            schedule(guid, "revivestalagg", 5000, function()
                reviveStalaggTick(creature, guid)
            end)
        else
            transition(creature, guid)
        end
    elseif action == ACTION_POLARITY_CROSSED then
        creature:SetData(DATA_POLARITY_CROSSED, 0) -- shockingEligibility = false
    end
end

-- C++ InitializeAI (:166-174) + JustAppeared -> ResetEncounter
-- (:202-206, :330-342): if boss state != DONE, SetPhase(NOT_ENGAGED) +
-- SetCombatMovement(false) + _Reset (SetBossState NOT_STARTED) +
-- REACT_PASSIVE + pet-alive reset. The Respawn(130958/130959) leg
-- (map-level spawn-ID respawn) has no bridge.
local function thaddiusSpawn(event, creature)
    creature:SetGuidData(DATA_THADDIUS)
    if creature:GetBossState(BOSS_THADDIUS) ~= BOSS_STATE_DONE then
        local guid = creature:GetGUIDLow()
        thaddiusPhase[guid] = PHASE_NOT_ENGAGED
        thaddiusStalaggAlive[guid] = true
        thaddiusFeugenAlive[guid] = true
        thaddiusBallUnlocked[guid] = false
        thaddiusBallEnabled[guid] = false
        thaddiusResetting[guid] = nil
        creature:SetData(DATA_POLARITY_CROSSED, 1) -- shockingEligibility = true (ctor)
        creature:SetCombatMovement(false)
        creature:SetReactState(REACT_PASSIVE)
        creature:SetBossState(BOSS_THADDIUS, BOSS_STATE_NOT_STARTED)
    end
end

-- C++ EnterEvadeMode (:183-192), event 2. The NO_HOSTILES retry leg
-- (ballLightningEnabled = true, return — try again) has no reason arg
-- in Lua, so the first evade always takes the retry: in-dungeon evades
-- are always no-hostiles (the leash arm is skipped in dungeons,
-- creaturemotion.go:5598). C++ returns early on the retry leg without
-- calling base CreatureAI::EnterEvadeMode, so the ev2 handler returns
-- true to veto the Go base evade (threat-clear / walk-home / full-heal);
-- timers are NOT cancelled here (the C++ EventMap survives evade).
local function thaddiusLeaveCombat(event, creature)
    local guid = creature:GetGUIDLow()
    if not thaddiusBallEnabled[guid] then
        thaddiusBallEnabled[guid] = true
        return true
    end
    local phase = thaddiusPhase[guid]
    if phase == PHASE_TRANSITION or (phase == PHASE_THADDIUS and creature:IsAlive()) then
        beginResetEncounter(creature, guid)
    end
end

-- C++ UpdateAI ball-lightning leg (:444-454), event 7 (100ms tick):
-- PHASE_THADDIUS + not casting + attack-ready: in melee range, clear
-- ballLightningEnabled and let the engine melee (return nil — a true
-- return would veto the engine melee step); out of melee with
-- ballLightningUnlocked, DoCast(BALL_LIGHTNING 28299) on a random
-- threat target. isAttackReady has no bridge and is vacuous out of
-- melee (the swing timer is elapsed and never resets without a melee
-- swing); the IsCasting gate is the operative one.
local function thaddiusAIUpdate(event, creature, diff)
    local guid = creature:GetGUIDLow()
    if thaddiusPhase[guid] ~= PHASE_THADDIUS then
        return
    end
    if creature:IsCasting() then
        return
    end
    local victim = creature:GetVictim()
    if not victim then
        return
    end
    if creature:IsWithinMeleeRange(victim) then
        thaddiusBallEnabled[guid] = false
    elseif thaddiusBallUnlocked[guid] then
        local target = randomThreatTarget(creature, 0)
        if target then
            creature:CastSpell(target, SPELL_BALL_LIGHTNING)
        end
    end
end

-- C++ spell_thaddius_polarity_shift::HandleDummy
-- (boss_thaddius.cpp:1107-1124): OnEffectHitTarget EFFECT_0
-- SPELL_EFFECT_DUMMY on POLARITY_SHIFT 28089 — the hit target must
-- be a player; roll_chance_i(50) picks the charge: positive casts
-- 28059 on the target (triggered) and removes the positive amp
-- 29659, negative casts 28084 and removes 29660. PORTED (ev5;
-- gluth decimate precedent for the effectIndex-0 gate and the
-- spell:GetCaster() entry check). The player CastSpell 3rd arg is
-- ignored by the player funnel, which is triggered-only — matching
-- C++ triggered=true. Dormant until Spell.dbc ships 28089 (the
-- spellKnown gate, rows 261-265 precedent).
local function polarityShiftHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetEntry() ~= ENTRY_THADDIUS then
        return
    end
    if hitTarget:GetObjectType() ~= "Player" then
        return
    end
    if math.random() < 0.5 then
        hitTarget:CastSpell(hitTarget, SPELL_POSITIVE_CHARGE_APPLY, true)
        hitTarget:RemoveAura(SPELL_POSITIVE_CHARGE_AMP)
    else
        hitTarget:CastSpell(hitTarget, SPELL_NEGATIVE_CHARGE_APPLY, true)
        hitTarget:RemoveAura(SPELL_NEGATIVE_CHARGE_AMP)
    end
end

RegisterSpellEvent(SPELL_POLARITY_SHIFT, 5, polarityShiftHit)

RegisterCreatureEvent(ENTRY_THADDIUS, 1, thaddiusEnterCombat)
RegisterCreatureEvent(ENTRY_THADDIUS, 2, thaddiusLeaveCombat)
RegisterCreatureEvent(ENTRY_THADDIUS, 3, thaddiusTargetDied)
RegisterCreatureEvent(ENTRY_THADDIUS, 4, thaddiusDied)
RegisterCreatureEvent(ENTRY_THADDIUS, 5, thaddiusSpawn)
RegisterCreatureEvent(ENTRY_THADDIUS, 7, thaddiusAIUpdate)
RegisterCreatureEvent(ENTRY_THADDIUS, 38, thaddiusDoAction)
