-- Gal'darah (Gundrak) — Lua port of
-- src/server/scripts/Northrend/Gundrak/boss_gal_darah.cpp
-- (boss_gal_darah + spell_gal_darah_impaling_charge +
-- spell_gal_darah_stampede_charge + spell_gal_darah_clear_puncture +
-- achievement_share_the_love).
-- Gundrak dungeon-script unit per northrend_script_loader.cpp order
-- (decl 23 / call 218, immediately after AddSC_boss_drakkari_colossus();
-- next: boss_eck).
-- Entry: 29306 Gal'darah (gundrak.h NPC_GAL_DARAH — the
-- RegisterCreatureAIWithFactory(GetGundrakAI) ScriptName binding is
-- instance-shimmed, the creature_template binding DB-side as usual).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady. C++ DoCast
-- default is triggered=false (vaelastrasz convention):
-- creature:CastSpell(nil, spell) = DoCastVictim,
-- creature:CastSpell(creature, spell[, triggered]) = DoCastSelf
-- (phase_hunter / apothecary_hanes precedent — the trailing true is
-- the triggered port). DoCastAOE lowers to self-cast
-- creature:CastSpell(creature, spell) (boss_ai.go DoCastAOE lowering;
-- selin_fireheart / mr_smite precedent). Timer casts ride the
-- CreateLuaEvent convention (sartharion / maiden precedent):
-- GUID-keyed timers table, cancel on events 2/4/23.
-- Ported arms (C++-exact for all modeled arms):
-- boss_gal_darah: JustEngagedWith Talk(SAY_AGGRO 0) (event 1);
-- KilledUnit Talk(SAY_SLAY 1) player-gated (event 3,
-- victim:GetObjectType() == "Player" — nalorakk precedent);
-- JustDied Talk(SAY_DEATH 2) + DoCastSelf(Clear Puncture 60022,
-- triggered) (event 4).
-- Combat rotation — the phase machine is emulated in Lua-local state
-- (phase + _phaseCounter per GUID) because the flip is deterministic
-- and driven entirely by bridged timer arms: the troll->rhino /
-- rhino->troll flip is scheduled 5s after the SECOND
-- EVENT_WHIRLING_SLASH / EVENT_IMPALING_CHARGE respectively
-- (:187-:192 / :207-:211), and both trigger arms are timer-driven
-- and bridgeable — unlike moorabi's transformation flip, which is
-- SpellHit-driven and PORTED via event 14 (moorabiSpellHit; the
-- moorabi precedent does NOT apply here — this flip is timer-driven).
-- SetPhase schedules the new phase's event set fresh; the old
-- phase's pending events are dropped in C++ when they come due
-- (EventMap::ExecuteEvent discards phase-mismatched events) — the Lua
-- port cancels them at flip time, which is observationally identical
-- (no cast ever fires).
-- PHASE_TROLL (engage; :133-:136): EVENT_STAMPEDE — Talk(SAY_SUMMON_RHINO 3)
-- + DoCastAOE(Stampede 55218) non-triggered, 10s init / 15s re-arm
-- (:202-:205); EVENT_WHIRLING_SLASH — DoCastVictim(Whirling Slash 55250)
-- non-triggered, 21s init / 21s re-arm, _phaseCounter++ unconditional,
-- EVENT_TRANSFORM at 5s once counter >= 2 (:207-:211).
-- PHASE_RHINO (:137-:141): EVENT_STOMP — DoCastAOE(Stomp 55292)
-- non-triggered, 25s init / 20s re-arm (:194-:196); EVENT_IMPALING_CHARGE —
-- SelectTarget(Random, 0, 60.0f, true) threat-list player pool (randomThreatPlayerInRange) -> DoCast(target, Impaling Charge 54956)
-- non-triggered, null-guarded like the C++ if, 21s init / 31s re-arm
-- unconditional, _phaseCounter++ unconditional, EVENT_TRANSFORM at 5s
-- once counter >= 2 (:187-:192); EVENT_ENRAGE — DoCastSelf(Enrage 55285)
-- non-triggered, 15s init / 20s re-arm (:213-:215); EVENT_PUNCTURE —
-- DoCastVictim(Puncture 55276) non-triggered, 10s init / 8s re-arm
-- (:198-:200).
-- EVENT_TRANSFORM (:217-:231): PHASE_TROLL -> Talk(SAY_TRANSFORM_1 4) +
-- DoCast(me, Transform Rhino 55297) non-triggered self-cast +
-- SetPhase(PHASE_RHINO); PHASE_RHINO -> Talk(SAY_TRANSFORM_2 5) +
-- DoCast(me, Transform Back 55299) non-triggered self-cast +
-- SetPhase(PHASE_TROLL); _phaseCounter = 0 unconditional at the end.
-- Reset — DoCastAOE(Hearth Beam Visual 54988, triggered) (event 23; :98).
-- Cast-state gates PORTED: gal_darah has no UpdateAI override, so
-- BossAI::UpdateAI (ScriptedCreature.cpp:571-586) pre-loop + in-loop
-- CASTING returns apply to all 7 timer arms above (stampede /
-- whirling / stomp / impaling / puncture / enrage / transform) — each
-- tick defers 500ms while casting, no repeat re-arm (mal_ganis
-- idiom); non-UpdateAI arms (JustEngaged / JustDied / SpellHit /
-- KilledUnit / Reset) ungated per C++.
-- Unmodeled (no bridges — documented, not wired):
-- EVENT_STAMPEDE's rhino-spirit summon leg (JustSummoned
-- NPC_RHINO_SPIRIT 29791 -> Stampede Spirit 55221/55219 self-casts +
-- Stampede Spirit Charge 59823 on a random target, :101-:109) —
-- PORTED below (galDarahJustSummoned, ev19; dormant until Spell.dbc
-- ships, see the handler note). The cast of 55218 itself is ported.
-- spell_gal_darah_impaling_charge (SpellScript 54956/59827: charge hit ->
-- target casts Impaling Charge Control Vehicle 54958 on the boss +
-- caster AI()->SetGUID(target, DATA_SHARE_THE_LOVE)) — the SetGUID leg
-- is PORTED tick 417 via event 15 (galDarahSpellHitTarget below): the
-- charge landing on a target records the target GUID in the per-GUID
-- impaled set and writes the distinct count into the boss motion's
-- SetData store (the C++ _impaledPlayers set, boss_gal_darah.cpp:147-160),
-- and Talks EMOTE_IMPALE (6) with the target on every hit (this tick —
-- the Talk bridge takes a target, lua_creature_events.go:1129).
-- No SpellScript binding is needed — ev15 is the SpellHitTarget twin
-- (boss_ai.go fireCreatureSpellHitTarget; sindragosa precedent). The
-- 54958 control-vehicle cast is PORTED below (galDarahSpellHitTarget,
-- before the Talk, the C++ cast-then-SetGUID order); the aura-236
-- control-vehicle apply legs exist engine-side (spells.go), DORMANT
-- until Spell.dbc ships the 54958 effect rows (stampede-charge ev5
-- precedent). The cast of 54956 itself is ported.
-- spell_gal_darah_stampede_charge (SpellScript 55220/59823: rhino spirit
-- despawns 1s after its charge hits) — PORTED below
-- (stampedeChargeHit, ev5 effectIndex == 1; dormant until Spell.dbc
-- ships, see the handler note).
-- spell_gal_darah_clear_puncture (SpellScript 60022: remove Puncture
-- 55276 / heroic 59826) — PORTED engine-side in creatureEffectScriptEffect
-- (boss_ai.go); the JustDied cast of 60022 itself was already ported.
-- achievement_share_the_love (AchievementCriteriaScript: GetData
-- (DATA_SHARE_THE_LOVE) >= 5) — PORTED tick 417 via the SetData write
-- leg + Go gate (bossLatchData, achievement_criteria_scripts.go).
-- SpellHit(SPELL_TRANSFORM_BACK 55299) -> RemoveAurasDueToSpell(55297)
-- (:177-:181) — PORTED: the victim-side arm rides event 14
-- (CREATURE_EVENT_ON_HIT_BY_SPELL, faerlina precedent) and the aura
-- removal rides creature:RemoveAura (the drakkari / festergut
-- RemoveAura bridge — the "no aura-removal bridge (aeranas precedent)"
-- header claim is stale). The cast of 55299 itself is the transform
-- tick's DoCast, exactly as in C++.
-- EnterEvadeMode summons.DespawnAll + _DespawnAtEvade (:111-:115) — the
-- boss despawn is PORTED via creature:Despawn on event 2 (drakkari
-- colossus precedent; the old "no despawn bridge" claim is stale);
-- summons.DespawnAll stays out (no summon tracking in Lua).
-- JustEngagedWith InterruptNonMeleeSpells(false) — PORTED below
-- (no-op twin, curator precedent); BossAI _Reset/_JustDied
-- instance bookkeeping has no bridge.
-- C++ threat-list caveat (shared with the keleseth/thorim/beast ports):
-- C++ SelectTarget draws from the threat list; the impaling-charge arm
-- now draws the same pool via GetAITargets (see randomThreatPlayerInRange).

local ENTRY_GAL_DARAH = 29306
local DATA_SHARE_THE_LOVE = 1 -- boss_gal_darah.cpp:81 (Misc enum)

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2
local SAY_SUMMON_RHINO = 3
local SAY_TRANSFORM_1 = 4
local SAY_TRANSFORM_2 = 5
local EMOTE_IMPALE = 6

local SPELL_IMPALING_CHARGE = 54956
local H_SPELL_IMPALING_CHARGE = 59827 -- boss_gal_darah.cpp:247
local SPELL_IMPALING_CHARGE_CONTROL_VEHICLE = 54958 -- boss_gal_darah.cpp:29
local SPELL_STOMP = 55292
local SPELL_PUNCTURE = 55276
local SPELL_STAMPEDE = 55218
local SPELL_WHIRLING_SLASH = 55250
local SPELL_ENRAGE = 55285
local SPELL_HEARTH_BEAM_VISUAL = 54988
local SPELL_TRANSFORM_RHINO = 55297
local SPELL_TRANSFORM_BACK = 55299
local SPELL_CLEAR_PUNCTURE = 60022
local SPELL_STAMPEDE_SPIRIT = 55221
local SPELL_STAMPEDE_SPIRIT_2 = 55219
local SPELL_STAMPEDE_SPIRIT_CHARGE = 59823
local SPELL_STAMPEDE_CHARGE = 55220 -- normal-mode stampede charge

local NPC_RHINO_SPIRIT = 29791 -- gundrak.h NPC_RHINO_SPIRIT

local PHASE_TROLL = 1
local PHASE_RHINO = 2

-- GUID-key rule: per-GUID tables are keyed on GetGUIDLow() (stable
-- number). GetGUID() returns a fresh UInt64 userdata per call and
-- Lua table keys compare userdata by identity, so a key stored
-- under one call never matches a lookup from another
-- (engine/scripting/object.go pushUInt64; instance_gundrak.lua
-- GUID-KEY FIX).
local timers = {}
local phaseState = {}
local impaledPlayers = {} -- [boss guid] = { [player guid] = true }
local impaledCount = {} -- [boss guid] = distinct impaled player count

local transformTick

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
    phaseState[guid] = nil
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

-- C++ SelectTarget(Random, 0, 60.0f, true) draws the threat list
-- filtered to players (UnitAI.h:162-184, playerOnly=true) — the
-- GetPlayersInWorld pool this replaces could pick instance players
-- with no threat entry, which C++ never does (the 2026-10-25
-- Utgarde/Gruul threat-pool fix class; eck spring precedent).
local function randomThreatPlayerInRange(creature, range)
    local candidates = {}
    for _, tgt in ipairs(creature:GetAITargets()) do
        if tgt:GetObjectType() == "Player" and not tgt:IsDead()
                and creature:GetDistance(tgt) - creature:GetCombatReach() - 1.5 < range then -- IsWithinCombatRange twin (Unit.cpp:583; player reach 1.5, ObjectDefines.h:40)
            candidates[#candidates + 1] = tgt
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EVENT_STAMPEDE (PHASE_TROLL; :202-:205): Talk(SAY_SUMMON_RHINO) +
-- DoCastAOE(SPELL_STAMPEDE) non-triggered, 10s init / 15s re-arm.
local function stampedeTick(creature, guid)
    -- BossAI::UpdateAI CASTING gates (ScriptedCreature.cpp:578/:584,
    -- no UpdateAI override): while casting the event stays queued —
    -- 500ms defer, no repeat re-arm (mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "stampede", 500, function()
            stampedeTick(creature, guid)
        end)
        return
    end
    creature:Talk(SAY_SUMMON_RHINO)
    creature:CastSpell(creature, SPELL_STAMPEDE)
    schedule(guid, "stampede", 15000, function()
        stampedeTick(creature, guid)
    end)
end

-- C++ EVENT_WHIRLING_SLASH (PHASE_TROLL; :207-:211):
-- DoCastVictim(SPELL_WHIRLING_SLASH) non-triggered, 21s init / 21s re-arm;
-- _phaseCounter++ unconditional; EVENT_TRANSFORM at 5s once >= 2.
local function whirlingTick(creature, guid)
    -- BossAI::UpdateAI CASTING gates (ScriptedCreature.cpp:578/:584,
    -- no UpdateAI override): while casting the event stays queued —
    -- 500ms defer, no repeat re-arm (mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "whirling", 500, function()
            whirlingTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_WHIRLING_SLASH)
    local st = phaseState[guid]
    if st then
        st.counter = st.counter + 1
        if st.counter >= 2 then
            schedule(guid, "transform", 5000, function()
                transformTick(creature, guid)
            end)
        end
    end
    schedule(guid, "whirling", 21000, function()
        whirlingTick(creature, guid)
    end)
end

-- C++ EVENT_STOMP (PHASE_RHINO; :194-:196): DoCastAOE(SPELL_STOMP)
-- non-triggered, 25s init / 20s re-arm.
local function stompTick(creature, guid)
    -- BossAI::UpdateAI CASTING gates (ScriptedCreature.cpp:578/:584,
    -- no UpdateAI override): while casting the event stays queued —
    -- 500ms defer, no repeat re-arm (mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "stomp", 500, function()
            stompTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_STOMP)
    schedule(guid, "stomp", 20000, function()
        stompTick(creature, guid)
    end)
end

-- C++ EVENT_IMPALING_CHARGE (PHASE_RHINO; :187-:192): SelectTarget(Random,
-- 0, 60.0f, true) -> DoCast(target, SPELL_IMPALING_CHARGE) non-triggered,
-- null-guarded; 21s init / 31s re-arm unconditional; _phaseCounter++
-- unconditional; EVENT_TRANSFORM at 5s once >= 2.
local function impalingTick(creature, guid)
    -- BossAI::UpdateAI CASTING gates (ScriptedCreature.cpp:578/:584,
    -- no UpdateAI override): while casting the event stays queued —
    -- 500ms defer, no repeat re-arm (mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "impaling", 500, function()
            impalingTick(creature, guid)
        end)
        return
    end
    local target = randomThreatPlayerInRange(creature, 60)
    if target then
        creature:CastSpell(target, SPELL_IMPALING_CHARGE)
    end
    local st = phaseState[guid]
    if st then
        st.counter = st.counter + 1
        if st.counter >= 2 then
            schedule(guid, "transform", 5000, function()
                transformTick(creature, guid)
            end)
        end
    end
    schedule(guid, "impaling", 31000, function()
        impalingTick(creature, guid)
    end)
end

-- C++ EVENT_PUNCTURE (PHASE_RHINO; :198-:200):
-- DoCastVictim(SPELL_PUNCTURE) non-triggered, 10s init / 8s re-arm.
local function punctureTick(creature, guid)
    -- BossAI::UpdateAI CASTING gates (ScriptedCreature.cpp:578/:584,
    -- no UpdateAI override): while casting the event stays queued —
    -- 500ms defer, no repeat re-arm (mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "puncture", 500, function()
            punctureTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_PUNCTURE)
    schedule(guid, "puncture", 8000, function()
        punctureTick(creature, guid)
    end)
end

-- C++ EVENT_ENRAGE (PHASE_RHINO; :213-:215): DoCastSelf(SPELL_ENRAGE)
-- non-triggered, 15s init / 20s re-arm.
local function enrageTick(creature, guid)
    -- BossAI::UpdateAI CASTING gates (ScriptedCreature.cpp:578/:584,
    -- no UpdateAI override): while casting the event stays queued —
    -- 500ms defer, no repeat re-arm (mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "enrage", 500, function()
            enrageTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_ENRAGE)
    schedule(guid, "enrage", 20000, function()
        enrageTick(creature, guid)
    end)
end

local function scheduleTrollPhase(creature, guid)
    local per = timers[guid]
    if per then
        for _, key in ipairs({ "stomp", "impaling", "puncture", "enrage" }) do
            if per[key] then
                RemoveEventById(per[key])
                per[key] = nil
            end
        end
    end
    schedule(guid, "stampede", 10000, function()
        stampedeTick(creature, guid)
    end)
    schedule(guid, "whirling", 21000, function()
        whirlingTick(creature, guid)
    end)
end

local function scheduleRhinoPhase(creature, guid)
    local per = timers[guid]
    if per then
        for _, key in ipairs({ "stampede", "whirling" }) do
            if per[key] then
                RemoveEventById(per[key])
                per[key] = nil
            end
        end
    end
    schedule(guid, "stomp", 25000, function()
        stompTick(creature, guid)
    end)
    schedule(guid, "impaling", 21000, function()
        impalingTick(creature, guid)
    end)
    schedule(guid, "enrage", 15000, function()
        enrageTick(creature, guid)
    end)
    schedule(guid, "puncture", 10000, function()
        punctureTick(creature, guid)
    end)
end

-- C++ EVENT_TRANSFORM (:217-:231): PHASE_TROLL -> Talk(SAY_TRANSFORM_1) +
-- DoCast(me, SPELL_TRANSFORM_RHINO) + SetPhase(PHASE_RHINO); PHASE_RHINO ->
-- Talk(SAY_TRANSFORM_2) + DoCast(me, SPELL_TRANSFORM_BACK) +
-- SetPhase(PHASE_TROLL); _phaseCounter = 0 unconditional.
transformTick = function(creature, guid)
    local st = phaseState[guid]
    if not st then
        return
    end
    -- BossAI::UpdateAI CASTING gates (ScriptedCreature.cpp:578/:584,
    -- no UpdateAI override): while casting the event stays queued —
    -- 500ms defer, no repeat re-arm (mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "transform", 500, function()
            transformTick(creature, guid)
        end)
        return
    end
    if st.phase == PHASE_TROLL then
        creature:Talk(SAY_TRANSFORM_1)
        creature:CastSpell(creature, SPELL_TRANSFORM_RHINO)
        st.phase = PHASE_RHINO
        scheduleRhinoPhase(creature, guid)
    elseif st.phase == PHASE_RHINO then
        creature:Talk(SAY_TRANSFORM_2)
        creature:CastSpell(creature, SPELL_TRANSFORM_BACK)
        st.phase = PHASE_TROLL
        scheduleTrollPhase(creature, guid)
    end
    st.counter = 0
end

-- C++ JustEngagedWith: BossAI::JustEngagedWith + Talk(SAY_AGGRO) +
-- InterruptNonMeleeSpells(false) (ported below, no-op twin) +
-- SetPhase(PHASE_TROLL) (:121-:126).
local function galDarahEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    creature:SetCombatPulseDelay(5)
    creature:DoZoneInCombat()
    creature:Talk(SAY_AGGRO)
    creature:InterruptNonMeleeSpells(false)
    phaseState[guid] = { phase = PHASE_TROLL, counter = 0 }
    impaledPlayers[guid] = {}
    impaledCount[guid] = 0
    scheduleTrollPhase(creature, guid)
end

-- C++ EnterEvadeMode (:111-:115): _EnterEvadeMode +
-- summons.DespawnAll + _DespawnAtEvade. The boss despawn rides
-- creature:Despawn (drakkari colossus precedent): event 2 fires only
-- on the evade path (triggerCreatureEvade), and DB-spawned creatures
-- respawn on their normal spawntime. The summons.DespawnAll leg stays
-- out (no summon tracking in Lua).
local function galDarahLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
    creature:Despawn()
end

local function galDarahTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ boss_gal_darah::SpellHit (:177-:181): if (spellInfo->Id ==
-- SPELL_TRANSFORM_BACK 55299) me->RemoveAurasDueToSpell(
-- SPELL_TRANSFORM_RHINO 55297). Victim-side arm on event 14
-- CREATURE_EVENT_ON_HIT_BY_SPELL (faerlina precedent); the aura
-- removal rides creature:RemoveAura (drakkari/festergut idiom).
local function galDarahSpellHit(event, creature, caster, spellId)
    if spellId == SPELL_TRANSFORM_BACK then
        creature:RemoveAura(SPELL_TRANSFORM_RHINO)
    end
end

local function galDarahDied(event, creature, killer)
    cancelTimers(creature:GetGUIDLow())
    creature:Talk(SAY_DEATH)
    creature:CastSpell(creature, SPELL_CLEAR_PUNCTURE, true)
end

-- C++ Reset: _Reset() + _impaledPlayers.clear() + _phaseCounter = 0 +
-- DoCastAOE(SPELL_HEARTH_BEAM_VISUAL, true) (:93-:99) — only the triggered
-- self-cast is bridgeable.
local function galDarahReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    -- C++ Reset(): _impaledPlayers.clear() + _phaseCounter = 0
    -- (boss_gal_darah.cpp:93-99) -> GetData(DATA_SHARE_THE_LOVE) = 0;
    -- achievement_share_the_love read leg (tick 417).
    impaledPlayers[creature:GetGUIDLow()] = nil
    impaledCount[creature:GetGUIDLow()] = nil
    -- Alive-gated: ev23 fires on death too (fireCreatureDied runs
    -- Reset before the kill-credit criteria read, kill.go:1091/:1400),
    -- and a death-time zero would clobber the fight's count before
    -- the achievement gate reads it.
    if creature:IsAlive() then
        creature:SetData(DATA_SHARE_THE_LOVE, 0)
    end
    -- C++ Reset(): DoCastAOE(SPELL_HEARTH_BEAM_VISUAL, true) is Reset-only
    -- (boss_gal_darah.cpp). Alive-gated; the timer cancel rides the death
    -- leg deliberately.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_HEARTH_BEAM_VISUAL, true)
    end
end

-- C++ spell_gal_darah_impaling_charge::HandleScript
-- (boss_gal_darah.cpp:259-276): the Impaling Charge (54956 / heroic
-- 59827) landing on a target records the target GUID in
-- _impaledPlayers. Event 15 fires on the caster's bindings with
-- (event, creature, target, spellId) per hit target
-- (boss_ai.go fireCreatureSpellHitTarget; sindragosa precedent); the
-- distinct-player count is the GetData(DATA_SHARE_THE_LOVE) value.
local function galDarahSpellHitTarget(event, creature, target, spellId)
    if spellId ~= SPELL_IMPALING_CHARGE and spellId ~= H_SPELL_IMPALING_CHARGE then
        return
    end
    if target == nil then
        return
    end
    -- C++ HandleScript (boss_gal_darah.cpp:269-277): the hit target
    -- casts 54958 triggered on the caster BEFORE SetGUID — the cast
    -- is unconditional per hit, so it sits outside the dedupe below.
    -- Player/creature CastSpell bridges (lua_player.go:2897,
    -- moroes precedent); the aura-236 board leg is DORMANT until
    -- Spell.dbc ships the 54958 effect rows (spells.go).
    target:CastSpell(creature, SPELL_IMPALING_CHARGE_CONTROL_VEHICLE, true)
    -- C++ SetGUID (boss_gal_darah.cpp:147-160) Talks EMOTE_IMPALE (6)
    -- with the target on EVERY charge hit, including a repeat impale
    -- of the same player — the Talk is unconditional in C++, so it
    -- sits outside the dedupe below. Bridge: creature:Talk(id, target),
    -- lua_creature_events.go:1129.
    creature:Talk(EMOTE_IMPALE, target)
    local guid = creature:GetGUIDLow()
    local set = impaledPlayers[guid]
    if set == nil then
        set = {}
        impaledPlayers[guid] = set
        impaledCount[guid] = 0
    end
    local pguid = target:GetGUIDLow()
    if pguid ~= nil and not set[pguid] then
        set[pguid] = true
        impaledCount[guid] = impaledCount[guid] + 1
        creature:SetData(DATA_SHARE_THE_LOVE, impaledCount[guid])
    end
end

-- C++ SelectTarget(Random, 0) (boss_gal_darah.cpp:107): threat-list
-- draw with no range and no player-only filter (DefaultTargetSelector,
-- UnitAI.h:59 — dist 0 ignored, playerOnly false); the alive filter
-- is the in-tree GetAITargets idiom (impaling-charge arm above).
local function randomThreatTarget(creature)
    local candidates = {}
    for _, tgt in ipairs(creature:GetAITargets()) do
        if not tgt:IsDead() then
            candidates[#candidates + 1] = tgt
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ JustSummoned (boss_gal_darah.cpp:101-109): a rhino spirit
-- (NPC_RHINO_SPIRIT 29791) self-casts Stampede Spirit 55221 + 55219
-- triggered, then charges a random threat target with 59823
-- non-triggered. DORMANT until Spell.dbc ships: the spirit arrives
-- via the SPELL_EFFECT_SUMMON effect of Stampede 55218, delivered
-- for creature casters by creatureEffectSummonType (boss_ai.go:3451,
-- spellKnown-gated), which fires ev19 on the summoner through
-- summonTempCreature (lua_creature_events.go:190) — the old "no
-- SPELL_EFFECT_SUMMON delivery on creature casts" header claim is
-- stale.
local function galDarahJustSummoned(event, creature, summon)
    if summon == nil or summon:GetEntry() ~= NPC_RHINO_SPIRIT then
        return
    end
    summon:CastSpell(summon, SPELL_STAMPEDE_SPIRIT, true)
    summon:CastSpell(summon, SPELL_STAMPEDE_SPIRIT_2, true)
    local target = randomThreatTarget(creature)
    if target then
        summon:CastSpell(target, SPELL_STAMPEDE_SPIRIT_CHARGE)
    end
end

-- C++ spell_gal_darah_stampede_charge::OnHit (boss_gal_darah.cpp:291-298):
-- the rhino spirit despawns 1s after its charge (55220 / 59823) hits —
-- OnEffectHitTarget EFFECT_1 SPELL_EFFECT_SCHOOL_DAMAGE. ev5 fires
-- once per (effect, hit target) from the creature cast path
-- (boss_ai.go:3417), gated here on effectIndex == 1; the caster is
-- the spirit (spell:GetCaster, lua_spell.go:478) and
-- DespawnOrUnsummon(1000) is the 1s delay twin
-- (lua_creature_events.go:6637). DORMANT until Spell.dbc ships
-- (spellKnown gate on the charge spells).
local function stampedeChargeHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 1 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    caster:DespawnOrUnsummon(1000)
end

RegisterSpellEvent(SPELL_STAMPEDE_CHARGE, 5, stampedeChargeHit)
RegisterSpellEvent(SPELL_STAMPEDE_SPIRIT_CHARGE, 5, stampedeChargeHit)

RegisterCreatureEvent(ENTRY_GAL_DARAH, 1, galDarahEnterCombat)
RegisterCreatureEvent(ENTRY_GAL_DARAH, 2, galDarahLeaveCombat)
RegisterCreatureEvent(ENTRY_GAL_DARAH, 3, galDarahTargetDied)
RegisterCreatureEvent(ENTRY_GAL_DARAH, 4, galDarahDied)
RegisterCreatureEvent(ENTRY_GAL_DARAH, 14, galDarahSpellHit)
RegisterCreatureEvent(ENTRY_GAL_DARAH, 15, galDarahSpellHitTarget)
RegisterCreatureEvent(ENTRY_GAL_DARAH, 19, galDarahJustSummoned)
RegisterCreatureEvent(ENTRY_GAL_DARAH, 23, galDarahReset)
