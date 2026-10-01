-- Gurtogg Bloodboil (Black Temple) — Lua port of
-- src/server/scripts/Outland/BlackTemple/boss_gurtogg_bloodboil.cpp
-- (boss_gurtogg_bloodboil only; npc_fel_geyser,
-- spell_gurtogg_bloodboil_bloodboil and
-- spell_gurtogg_bloodboil_insignificance documented below, not
-- registered); black_temple.h:35 (DATA_GURTOGG_BLOODBOIL = 4, fifth
-- boss), :75 (NPC_GURTOGG_BLOODBOIL = 22948). Creature entry: 22948
-- Gurtogg Bloodboil (C++ ScriptName "boss_gurtogg_bloodboil" per
-- RegisterBlackTempleCreatureAI in AddSC_boss_gurtogg_bloodboil;
-- the creature_template ScriptName binding is DB-side — no TDB in
-- this workspace, so only the C++-side naming is verified). Talk
-- lines used: SAY_AGGRO=0 (pull), SAY_SLAY=1 (kill, player-only),
-- SAY_SPECIAL=2 (eject), SAY_ENRAGE=3 (berserk, 50%); JustDied has
-- no Talk — only the unmodeled death sound (11439).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent;
-- melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnEnterCombat:
-- Talk(SAY_AGGRO), arm berserk 45078 10min one-shot, non-triggered
-- self-cast (C++ DoCast resolves AITARGET_SELF — UnitAI.cpp) + 50%
-- Talk(SAY_ENRAGE) (the alternative DoPlaySoundToSet(11437) arm has
-- no sound bridge, opera convention) / change phase 60s / phase 1
-- bloodboil 42005 10s then 10s, non-triggered self-cast (the
-- bloodboil target-filter SpellScript, 5 nearest, has no SpellScript
-- bridge) / arcing smash 40457 10s then 10s, non-triggered
-- DoCastVictim (nil falls back to the victim on the bridge; nil
-- ticks cast nothing but keep the schedule, jeklik convention) /
-- fel acid breath 40508 25s then {25s,30s}, non-triggered on a
-- random alive player in the instance within combat reach (no
-- combat-reach bridge — approximated as 10 yd, documented) /
-- eject 40486 35s one-shot, Talk(SAY_SPECIAL) + non-triggered
-- DoCastVictim / bewildering strike 40491 47s one-shot,
-- non-triggered DoCastVictim. Phase 2 (fel rage, on the 60s change-
-- phase arm): cancel phase-1 timers, re-arm change phase 30s, arm
-- start phase 2 100ms one-shot / eject 40597 14s one-shot,
-- non-triggered DoCastVictim / fel acid breath 40595 16s one-shot,
-- non-triggered DoCastVictim / arcing smash 40599 8s then 13s,
-- non-triggered DoCastVictim. Start phase 2: pick a random alive
-- player in the instance excluding the current victim (C++
-- SelectTarget(Random, 1), position 1, C++-exact); if found, store
-- its GUID (fel rage target), triggered self-cast fel rage 40594 +
-- triggered casts fel rage target 40604 / fel rage 40616 / fel rage
-- 41625 / fel geyser 40569 / fel rage target 46787 on the target,
-- DoCastAOE insignificance 40618 triggered
-- (the insignificance target-filter SpellScript has no
-- SpellScript bridge), arm charge player 40602 2s one-shot,
-- non-triggered on the stored GUID (ObjectAccessor::GetUnit arm —
-- bridged by GUID match over the instance players); the taunt /
-- attack-me ApplySpellImmune arms have no bridge. If no target is
-- found: drop back to phase 1, re-arm the phase-1 schedule and the
-- change-phase arm at 60s (C++-exact). The two player self-casts
-- (target->CastSpell(target, 40617/40603, true), pure aura
-- application) have no player-side CastSpell bridge — modeled as
-- AddAura(40617)/AddAura(40603), which reproduces the resulting
-- aura state (deliberate deviation, documented). Back in phase 1
-- (on the 30s change-phase arm): cancel phase-2 timers, re-arm
-- change phase 60s, the taunt/attack-me immunity-off arms have no
-- bridge, re-arm the phase-1 schedule; the
-- ModifyThreatByPercent(-100) / AttackStart(oldTarget) /
-- AddThreat(oldThreat) victim-restore arms have no threat bridge
-- and are unmodeled, so the GUID state is simply cleared
-- (C++ Initialize). OnTargetDied(3): Talk(SAY_SLAY), TYPEID_PLAYER
-- gate (terestian convention). OnDied(4): nothing modeled — the
-- _JustDied instance arm is blocked on the instance-script model
-- and the DoPlaySoundToSet(11439) arm has no sound bridge. OnLeave-
-- Combat(2)/OnReset(23): cancel timers, drop per-GUID state. The
-- Reset ApplySpellImmune(false) arms have no bridge; the
-- EnterEvadeMode _DespawnAtEvade arm has no despawn bridge
-- (evade-side cleanup is engine-side). The CanAIAttack gate
-- (!who->HasAura(SPELL_BEWILDERING_STRIKE 40491)) has no attack-
-- gating bridge — the engine keeps attacking normally. The
-- npc_fel_geyser AI (Reset: triggered self-casts 40593 + 40031)
-- is not registered — no NPC_FEL_GEYSER constant exists in the C++
-- tree and the ScriptName binding is DB-side (garr firesworn
-- convention); the 40569 fel-geyser summon arm has no summon model
-- in any case.
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING queue gate
-- and the post-event casting check have no UNIT_STATE bridge —
-- timers fire unconditionally (jeklik convention); no instance-
-- script model — boss admission via the luaBossAI shim
-- (BossAI::JustEngagedWith/JustDied/Reset and the
-- DATA_GURTOGG_BLOODBOIL bookkeeping arms skipped); no threat
-- model — the GetThreat/ModifyThreatByPercent/AddThreat and
-- AttackStart(oldTarget) arms have no bridge; no flag/immune
-- model — the ApplySpellImmune taunt/attack-me arms have no
-- bridge.

local ENTRY_GURTOGG_BLOODBOIL = 22948

local PHASE_1 = 1
local PHASE_2 = 2

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_SPECIAL = 2
local SAY_ENRAGE = 3

local SPELL_BLOODBOIL = 42005
local SPELL_ARCING_SMASH = 40457
local SPELL_FEL_ACID_BREATH = 40508
local SPELL_EJECT = 40486
local SPELL_BEWILDERING_STRIKE = 40491
local SPELL_FEL_RAGE_SELF = 40594
local SPELL_INSIGNIFIGANCE = 40618
local SPELL_FEL_RAGE_TARGET = 40604
local SPELL_FEL_RAGE_2 = 40616
local SPELL_FEL_RAGE_3 = 41625
local SPELL_FEL_RAGE_TARGET_2 = 46787
local SPELL_FEL_GEYSER = 40569
local SPELL_CHARGE = 40602
local SPELL_EJECT_2 = 40597
local SPELL_FEL_ACID_BREATH_2 = 40595
local SPELL_ARCING_SMASH_2 = 40599
local SPELL_BERSERK = 45078
-- Player-side self-casts (no player CastSpell bridge — applied via
-- AddAura; their entire C++ effect is the aura).
local SPELL_TAUNT_GURTOGG = 40603
local SPELL_FEL_RAGE_P = 40617

local GROUP_PHASE_1 = { "bloodboil", "arcingsmash", "felacidbreath", "eject", "bewilderingstrike" }
local GROUP_PHASE_2 = { "startphase2", "eject2", "felacidbreath2", "arcingsmash2", "chargeplayer" }

local timers = {}
local state = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
    state[guid] = nil
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

local function cancelGroup(guid, keys)
    local per = timers[guid]
    if not per then
        return
    end
    for _, key in ipairs(keys) do
        if per[key] then
            RemoveEventById(per[key])
            per[key] = nil
        end
    end
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

-- C++ SelectTarget(Random, 1): any alive player in the instance,
-- excluding the current victim (position 1), no range cap.
local function randomAnyTarget(creature)
    local victim = creature:GetVictim()
    local candidates = {}
    for _, p in ipairs(playersInInstance(creature)) do
        if not victim or p:GetGUID() ~= victim:GetGUID() then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ SelectTarget(Random, 0, me->GetCombatReach()): any alive
-- player in the instance within combat reach, victim included. No
-- combat-reach bridge — approximated as 10 yd (documented).
local function randomTargetWithinReach(creature)
    local candidates = {}
    for _, p in ipairs(playersInInstance(creature)) do
        if creature:GetDistance(p) <= 10 then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ ObjectAccessor::GetUnit(*me, guid): GUID-follow over the
-- instance players (no instance-creature bridge needed — the fel
-- rage target is always a player).
local function findPlayerByGuid(creature, guid)
    for _, p in ipairs(playersInInstance(creature)) do
        if p:GetGUID() == guid then
            return p
        end
    end
    return nil
end

-- C++ EVENT_BLOODBOIL: non-triggered self-cast 42005; repeat 10s
-- unconditional (C++-exact).
local function onBloodboil(creature, guid)
    creature:CastSpell(creature, SPELL_BLOODBOIL, false)
    schedule(guid, "bloodboil", 10000, function()
        onBloodboil(creature, guid)
    end)
end

-- C++ EVENT_ARCING_SMASH: non-triggered DoCastVictim 40457;
-- repeat 10s. Nil victim falls back on the bridge; nil ticks cast
-- nothing but keep the schedule (jeklik convention).
local function onArcingSmash(creature, guid)
    creature:CastSpell(nil, SPELL_ARCING_SMASH, false)
    schedule(guid, "arcingsmash", 10000, function()
        onArcingSmash(creature, guid)
    end)
end

-- C++ EVENT_FEL_ACID_BREATH: non-triggered 40508 on a random
-- target within combat reach; repeat {25s,30s} unconditional
-- (C++-exact).
local function onFelAcidBreath(creature, guid)
    local target = randomTargetWithinReach(creature)
    if target then
        creature:CastSpell(target, SPELL_FEL_ACID_BREATH, false)
    end
    schedule(guid, "felacidbreath", 25000 + math.random(0, 5000), function()
        onFelAcidBreath(creature, guid)
    end)
end

-- C++ EVENT_EJECT: Talk(SAY_SPECIAL), non-triggered DoCastVictim
-- 40486; one-shot (no re-arm, C++-exact).
local function onEject(creature, guid)
    creature:Talk(SAY_SPECIAL)
    creature:CastSpell(nil, SPELL_EJECT, false)
end

-- C++ EVENT_BEWILDERING_STRIKE: non-triggered DoCastVictim 40491;
-- one-shot (no re-arm, C++-exact).
local function onBewilderingStrike(creature, guid)
    creature:CastSpell(nil, SPELL_BEWILDERING_STRIKE, false)
end

-- C++ EVENT_BERSERK: non-triggered self-cast 45078 (AITARGET_SELF
-- per UnitAI.cpp), one-shot; 50% Talk(SAY_ENRAGE), else the
-- DoPlaySoundToSet(11437) arm which has no sound bridge.
local function onBerserk(creature, guid)
    creature:CastSpell(creature, SPELL_BERSERK, false)
    if math.random(2) == 1 then
        creature:Talk(SAY_ENRAGE)
    end
end

-- Phase-1 ScheduleEvents(): the C++ initial delays.
local function schedulePhase1Events(creature, guid)
    schedule(guid, "bloodboil", 10000, function()
        onBloodboil(creature, guid)
    end)
    schedule(guid, "arcingsmash", 10000, function()
        onArcingSmash(creature, guid)
    end)
    schedule(guid, "felacidbreath", 25000, function()
        onFelAcidBreath(creature, guid)
    end)
    schedule(guid, "eject", 35000, function()
        onEject(creature, guid)
    end)
    schedule(guid, "bewilderingstrike", 47000, function()
        onBewilderingStrike(creature, guid)
    end)
end

-- C++ EVENT_EJECT_2: non-triggered DoCastVictim 40597; one-shot
-- (no re-arm, C++-exact).
local function onEject2(creature, guid)
    creature:CastSpell(nil, SPELL_EJECT_2, false)
end

-- C++ EVENT_FEL_ACID_BREATH_2: non-triggered DoCastVictim 40595;
-- one-shot (no re-arm, C++-exact).
local function onFelAcidBreath2(creature, guid)
    creature:CastSpell(nil, SPELL_FEL_ACID_BREATH_2, false)
end

-- C++ EVENT_ARCING_SMASH_2: non-triggered DoCastVictim 40599;
-- repeat 13s.
local function onArcingSmash2(creature, guid)
    creature:CastSpell(nil, SPELL_ARCING_SMASH_2, false)
    schedule(guid, "arcingsmash2", 13000, function()
        onArcingSmash2(creature, guid)
    end)
end

-- C++ EVENT_CHARGE_PLAYER: non-triggered 40602 on the stored fel
-- rage GUID; one-shot. Nil/no-longer-present target casts nothing
-- (jeklik convention).
local function onChargePlayer(creature, guid)
    local st = state[guid]
    local rageGuid = st and st.felRageTarget
    if rageGuid then
        local target = findPlayerByGuid(creature, rageGuid)
        if target then
            creature:CastSpell(target, SPELL_CHARGE, false)
        end
    end
end

-- C++ EVENT_START_PHASE_2: pick the fel rage target; the whole
-- fel rage series follows. On no target: drop back to phase 1 and
-- re-arm the phase-1 schedule + change-phase at 60s (C++-exact).
local function onStartPhase2(creature, guid)
    local target = randomAnyTarget(creature)
    if not target then
        state[guid].phase = PHASE_1
        cancelGroup(guid, GROUP_PHASE_2)
        schedulePhase1Events(creature, guid)
        schedule(guid, "changephase", 60000, function()
            onChangePhase(creature, guid)
        end)
        return
    end
    state[guid].felRageTarget = target:GetGUID()
    creature:CastSpell(creature, SPELL_FEL_RAGE_SELF, true)
    creature:CastSpell(target, SPELL_FEL_RAGE_TARGET, true)
    creature:CastSpell(target, SPELL_FEL_RAGE_2, true)
    creature:CastSpell(target, SPELL_FEL_RAGE_3, true)
    creature:CastSpell(target, SPELL_FEL_GEYSER, true)
    creature:CastSpell(target, SPELL_FEL_RAGE_TARGET_2, true)
    target:AddAura(SPELL_FEL_RAGE_P)
    target:AddAura(SPELL_TAUNT_GURTOGG)
    creature:CastSpell(creature, SPELL_INSIGNIFIGANCE, true)
    schedule(guid, "chargeplayer", 2000, function()
        onChargePlayer(creature, guid)
    end)
end

-- Phase-2 ScheduleEvents(): the C++ phase-2 initial delays.
local function schedulePhase2Events(creature, guid)
    schedule(guid, "startphase2", 100, function()
        onStartPhase2(creature, guid)
    end)
    schedule(guid, "eject2", 14000, function()
        onEject2(creature, guid)
    end)
    schedule(guid, "felacidbreath2", 16000, function()
        onFelAcidBreath2(creature, guid)
    end)
    schedule(guid, "arcingsmash2", 8000, function()
        onArcingSmash2(creature, guid)
    end)
end

-- C++ EVENT_CHANGE_PHASE -> ChangePhase(): phase 1 -> 2 cancels
-- group 1, re-arms change phase 30s, and arms the phase-2 set;
-- phase 2 -> 1 cancels group 2, re-arms change phase 60s, arms
-- the phase-1 set; the threat/AttackStart victim-restore and the
-- ApplySpellImmune arms have no bridges — the GUID state is
-- cleared (C++ Initialize).
local function onChangePhase(creature, guid)
    local st = state[guid]
    if not st then
        return
    end
    if st.phase == PHASE_1 then
        st.phase = PHASE_2
        st.felRageTarget = nil
        cancelGroup(guid, GROUP_PHASE_1)
        schedule(guid, "changephase", 30000, function()
            onChangePhase(creature, guid)
        end)
        schedulePhase2Events(creature, guid)
    else
        st.phase = PHASE_1
        st.felRageTarget = nil
        cancelGroup(guid, GROUP_PHASE_2)
        schedule(guid, "changephase", 60000, function()
            onChangePhase(creature, guid)
        end)
        schedulePhase1Events(creature, guid)
    end
end

local function gurtoggEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    state[guid] = { phase = PHASE_1 }
    creature:Talk(SAY_AGGRO)
    schedule(guid, "berserk", 600000, function()
        onBerserk(creature, guid)
    end)
    schedule(guid, "changephase", 60000, function()
        onChangePhase(creature, guid)
    end)
    schedulePhase1Events(creature, guid)
end

local function gurtoggResetState(guid)
    cancelTimers(guid)
end

local function gurtoggLeaveCombat(event, creature)
    gurtoggResetState(creature:GetGUID())
end

-- C++ KilledUnit: Talk(SAY_SLAY), TYPEID_PLAYER gate.
local function gurtoggTargetDied(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ JustDied: _JustDied (instance bookkeeping — blocked on the
-- instance-script model) + DoPlaySoundToSet(11439) (no sound
-- bridge). Nothing modeled — cleanup only.
local function gurtoggDied(event, creature, killer)
    gurtoggResetState(creature:GetGUID())
end

local function gurtoggReset(event, creature)
    gurtoggResetState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_GURTOGG_BLOODBOIL, 1, gurtoggEnterCombat)
RegisterCreatureEvent(ENTRY_GURTOGG_BLOODBOIL, 2, gurtoggLeaveCombat)
RegisterCreatureEvent(ENTRY_GURTOGG_BLOODBOIL, 3, gurtoggTargetDied)
RegisterCreatureEvent(ENTRY_GURTOGG_BLOODBOIL, 4, gurtoggDied)
RegisterCreatureEvent(ENTRY_GURTOGG_BLOODBOIL, 23, gurtoggReset)
