-- Lord Marrowgar (Icecrown Citadel) — Lua port of
-- src/server/scripts/Northrend/FrozenHalls-relative path
-- src/server/scripts/Northrend/IcecrownCitadel/boss_lord_marrowgar.cpp
-- (703 lines incl. license; 10 scripts — 3 CreatureScripts
-- (boss_lord_marrowgar (BossAI, DATA_LORD_MARROWGAR = 0),
-- npc_coldflame (ScriptedAI summon, SPELL_IMPALED vehicle legs),
-- npc_bone_spike (ScriptedAI trap summon, vehicle passenger legs)) +
-- 6 SpellScripts (spell_marrowgar_coldflame,
-- spell_marrowgar_coldflame_bonestorm, spell_marrowgar_coldflame_damage
-- (AuraScript), spell_marrowgar_bone_spike_graveyard,
-- spell_marrowgar_bone_storm, spell_marrowgar_bone_slice) + 1
-- AreaTriggerScript (at_lord_marrowgar_entrance); all registered from
-- inside AddSC_boss_lord_marrowgar(); loader decl 171 / call 366 per
-- northrend_script_loader.cpp — the FIRST group of the
-- "// Icecrown Citadel" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_marwyn() (call 364 — closes the "// Halls of
-- Reflection" block); the call after it is
-- AddSC_boss_lady_deathwhisper() — verified from the loader this
-- run; the checkpoint sequence (boss_marwyn ->
-- boss_lord_marrowgar) is followed).
-- Entry: 36612 Lord Marrowgar (icecrown_citadel.h NPC_LORD_MARROWGAR,
-- line 169; instance_icecrown_citadel.cpp OnCreatureCreate binds case
-- NPC_LORD_MARROWGAR, line 221) — entry-verifiable, registration
-- proceeds (the nexus_commanders kalecgos precedent); the
-- CreatureScript ScriptName binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_lord_marrowgar" hits boss_lord_marrowgar.cpp only (+
-- the loader decl/call lines); this clone carries no sql/ tree, so
-- ScriptName bindings are DB-side by construction. No marrowgar lua
-- existed.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnKill
-- (player-gated), 4 OnDied, 23 OnReset, 24 OnReachHome. Timers via CreateLuaEvent
-- (GUID-keyed, cancelled on events 2/4/23); melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady (:276).
--
-- DOCTRINE RETRACTION (2026-10-13): the earlier port of this file
-- treated the whole EventMap machine as unbridgeable under the
-- "no-timer-bridge" doctrine and ported Talk arms only. That doctrine is
-- disproved and stale — CreateLuaEvent is a real registered bridge
-- (engine/scripting/runtime.go:798), and timer-driven cast arms port on
-- the maiden_of_virtue / moroes / curator / nightbane convention:
-- timers table keyed by creature GUID, cancelled on Eluna events 2
-- (leave combat), 4 (death), 23 (reset); CastSpell(nil, spellId) =
-- victim-cast (bridge falls back to TargetGUID); CastSpell(creature,
-- spellId) = self-cast (the DoCastAOE convention, boss_ai.go DoCastAOE);
-- CastSpell(target, spellId) = explicit-target cast; TRIGGERED casts pass
-- true as the third CastSpell arg. Phase/machine gating is the real
-- blocker (the gothik precedent), not timers.
--
-- Ported arms (C++-exact for all modeled arms):
-- boss_lord_marrowgar JustEngagedWith — Talk(SAY_AGGRO 1) (event 1 —
-- the auriaya engage-port precedent; PORTED 2026-10-23:
-- DoZoneInCombat (:162, me-form) via the live bridge — C++ arm
-- order, after Talk; instance->SetBossState IN_PROGRESS (:163)
-- PORTED 2026-11-05 via creature:SetBossState (lua_creature_events.go:
-- 5917) — the old "no bridge" claim is retracted. The
-- me->setActive(true) leg has no bridge; the five ScheduleEvent legs
-- port 2026-11-08 (tick 493, below).
-- boss_lord_marrowgar KilledUnit — Talk(SAY_KILL 4) player-gated
-- (event 3 — the razuvious player-gated variant precedent; the
-- twelfth ported variant, tenth identical to
-- nalorakk/kelthuzad/gothik/thaddius/garfrost/krick/tyrannus/falric/
-- marwyn).
-- boss_lord_marrowgar JustDied — Talk(SAY_DEATH 5) (event 4 — the
-- sjonnir JustDied-Talk precedent); the _JustDied() DONE leg (:170)
-- PORTED 2026-11-05 via creature:SetBossState, in C++ order AFTER
-- Talk. Melee engine-driven.
-- boss_lord_marrowgar UpdateAI EVENT_ENRAGE (:261-263) — DoCast(me,
-- SPELL_BERSERK 26662, true) + Talk(SAY_BERSERK 6), cast-before-Talk
-- kept; 10min one-shot seeded on engage, no phase/machine/random-
-- target gates (the festergut/rotface/putricide/sindragosa berserk
-- precedent; TRIGGERED self-cast).
-- DOCUMENTED-ONLY (in this header; entries 36612 + 36619 registered):
-- npc_coldflame (36672) exposes zero own Talk lines; PORTED 2026-11-08
-- (tick 493, below): registered in this chunk, ev22 DoCast(SPELL_COLDFLAME_
-- SUMMON 69147); the IsSummonedBy reposition legs (GetLastColdflamePosition /
-- DATA_COLDFLAME_GUID player lookup + GetNearPoint2D) and the UpdateAI
-- EVENT_COLDFLAME_TRIGGER teleport leg stay out — GetAbsoluteAngle /
-- GetNearPoint2D / GetPosition absent from the creature surface, only GetZ
-- exists (re-checked 2026-11-08, still true). The summon itself rides the
-- target-player SummonCreature fallback (see onColdflame).
-- The vehicle legs
-- (RIDE_VEHICLE cast + PassengerBoarded spline) belong to bone_spike's
-- IsSummonedBy below, not to coldflame. npc_bone_spike (36619): the JustDied/KilledUnit
-- DespawnOrUnsummon legs PORT (event 4 / event 3, below); the JustDied
-- trapped-unit aura-removal leg PORTS via creature:GetSummonerUnit()
-- (bridged 2026-11-05, below); the IsSummonedBy DoCast(summoner,
-- SPELL_IMPALED) + summoner->CastSpell(me, SPELL_RIDE_VEHICLE, true)
-- legs PORT (event 22, below — player:CastSpell live, lua_player.go:
-- 2752); the 8s EVENT_FAIL_BONED timer leg PORTS via the shared
-- instanceData bonedEligible key (below, Go 12775/12962/13393/13394
-- read it); PassengerBoarded spline leg; UpdateAI machine)
-- has no bridges.
-- boss_lord_marrowgar Reset scheduler legs (:148-152) — PORTED
-- 2026-11-08 (tick 493, below): ENABLE_BONE_SLICE / BONE_SPIKE_GRAVEYARD /
-- COLDFLAME / WARN_BONE_STORM all seed at engage (the enrage precedent);
-- the RemoveAurasDueToSpell(SPELL_BONE_STORM / SPELL_BERSERK) legs (:145-146)
-- PORT (creature:RemoveAura on event 23, C++ order kept); the SetSpeedRate
-- legs ride the bone-storm machine (below) — the old "no SetSpeedRate
-- bridge" claim is stale (live, lua_creature_events.go:735) and the old
-- "only EVENT_ENRAGE ports" claim is retracted.
-- boss_lord_marrowgar UpdateAI — the EventMap machine: PORTED 2026-11-08
-- (tick 493, below). WARN_BONE_STORM (Talk EMOTE 7 + BONE_STORM self-cast +
-- BEGIN @3050ms + WARN re-arm {90s,95s}); BEGIN (GetAura SetDuration 20s/30s
-- + speed x3 + Talk SAY_BONE_STORM 2 + END @duration+1ms, fallthrough MOVE);
-- MOVE (re-arm duration/3 + random non-tank via GetAITargets + MoveChase
-- approx — no GetX/GetY on handler creatures/players, so the MovePoint
-- position leg and the MovementInform MoveIdle leg stay out); END (MoveClear
-- + MoveChase victim + speed restore + ENABLE_BONE_SLICE @10s + graveyard
-- reschedule @15s non-heroic); ENABLE_BONE_SLICE latch; GRAVEYARD (SpellScript
-- body replicated: 1/3 random valid players, target:SummonCreature — the
-- old "no summon bridge" claim is stale, SummonCreature live at
-- lua_creature_events.go:3718 / lua_player.go:6765); COLDFLAME (script body
-- replicated: random non-tank player, 1 summon, 4 in storm); bone-slice
-- DoCastVictim as a 1.5s poller (CastSpell(nil) victim-cast + spike-immune
-- marking — the old "DoCastVictim legs have no bridge" claim is retracted).
-- Stay-outs: DelayEvents(3s) at WARN (timers keep cadence), the bone-slice
-- damage split (a C++ no-op: SetHitDamage(GetHitDamage() / _targetCount)
-- with GetHitDamage() 0 at OnHit, Spell.cpp:2377 — ports as nothing);
-- the CLEAR_SPIKE_IMMUNITIES sender ports as the poller's pre-cast clear
-- and the per-hit-target marking rides the 69055 ev2 handler (dormant
-- until Spell.dbc ships), the coldflame
-- reposition/trigger-teleport legs (no position bridge), the dead-target
-- spike-despawn leg (no aura-caster-GUID bridge), DoMeleeAttackIfReady
-- (engine-driven melee is not suppressed during bone storm). JustReachedHome
-- instance-SetBossState(FAIL) (:175) PORTED 2026-11-05 via
-- creature:SetBossState on ev24 OnReachHome (the opera ev24 precedent);
-- the DATA_BONED_ACHIEVEMENT-reset leg PORTS via the shared instanceData
-- bonedEligible key (marrowgarReset, below).
-- Talk(SAY_ENTER_ZONE 0) rides DoAction(ACTION_TALK_ENTER_ZONE),
-- which only fires from at_lord_marrowgar_entrance — ev38 dispatch bridged;
-- sender area-trigger TryHandleOnce (boss_lord_marrowgar.cpp:675-681) unported,
-- no area-trigger bridge (krick relay N/A — no ported sender hook); the area trigger joins
-- the unbridged-area-trigger queue (the pit_of_saron cavern-triggers
-- precedent).
-- Talk(SAY_BONESPIKE 3) is called by marrowgarAI->Talk from inside
-- spell_marrowgar_bone_spike_graveyard SpellScript HandleSpikes —
-- no-SpellScript bridge (the boss_moragg optic-link precedent);
-- spell_marrowgar_coldflame / spell_marrowgar_coldflame_bonestorm /
-- spell_marrowgar_bone_storm / spell_marrowgar_bone_slice join the
-- no-SpellScript-bridge queue, and spell_marrowgar_coldflame_damage
-- joins the no-AuraScript-bridge queue.
-- All 8 Talk() calls in the file accounted for (8 = ported: aggro 1
-- + kill 4 + death 5 + berserk 6; documented-only: enter-zone 0 +
-- bone-storm 2 + bonespike 3 + emote-bone-storm 7).

local ENTRY_LORD_MARROWGAR = 36612
local ENTRY_BONE_SPIKE = 36619 -- icecrown_citadel.h:171

local DATA_LORD_MARROWGAR = 0 -- icecrown_citadel.h:74
local BOSS_STATE_NOT_STARTED = 0 -- InstanceScript.h:68-76
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_FAIL = 2
local BOSS_STATE_DONE = 3

local SPELL_IMPALED = 69065 -- boss_lord_marrowgar.cpp:54
local SPELL_RIDE_VEHICLE = 46598 -- boss_lord_marrowgar.cpp:55

local SAY_AGGRO = 1
local SAY_KILL = 4
local SAY_DEATH = 5
local SAY_BERSERK = 6

local SPELL_BERSERK = 26662 -- icecrown_citadel.h:33
local SPELL_BONE_STORM = 69076 -- boss_lord_marrowgar.cpp spells enum
local SPELL_BONE_SPIKE_GRAVEYARD = 69057 -- boss_lord_marrowgar.cpp:49
local SPELL_BONE_SLICE = 69055 -- boss_lord_marrowgar.cpp:47
local SPELL_COLDFLAME_NORMAL = 69140 -- boss_lord_marrowgar.cpp:50
local SPELL_COLDFLAME_BONE_STORM = 72705 -- boss_lord_marrowgar.cpp:51
local SPELL_COLDFLAME_SUMMON = 69147 -- boss_lord_marrowgar.cpp:59
local ENTRY_COLDFLAME = 36672 -- boss_lord_marrowgar.cpp npc_coldflame
local POINT_TARGET_BONESTORM_PLAYER = 36612631 -- boss_lord_marrowgar.cpp:83
local SAY_BONE_STORM = 2
local SAY_BONESPIKE = 3
local EMOTE_BONE_STORM = 7
local BoneSpikeSummonId = {69062, 72669, 72670} -- boss_lord_marrowgar.cpp:62

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

local marrowgarState = {}

local function getMarrowgarState(guid)
    local st = marrowgarState[guid]
    if not st then
        st = {boneSlice = false, spikeImmune = {}, baseRunRate = 1.0, stormDuration = 20000}
        marrowgarState[guid] = st
    end
    return st
end

local function cancelTimer(guid, key)
    local per = timers[guid]
    if per and per[key] then
        RemoveEventById(per[key])
        per[key] = nil
    end
end

-- C++ SelectTarget(Random, NonTankTargetSelector) twin: random entry from
-- the threat-sorted GetAITargets list excluding the current victim (the
-- tank); falls back to any threatened unit (C++ :234-235).
local function pickRandomNonTank(creature)
    local targets = creature:GetAITargets()
    local victim = creature:GetVictim()
    local victimGuid = victim and victim:GetGUID() or 0
    local pool = {}
    for _, unit in ipairs(targets) do
        if unit:GetGUID() ~= victimGuid then
            pool[#pool + 1] = unit
        end
    end
    if #pool == 0 then
        pool = targets
    end
    if #pool == 0 then
        return nil
    end
    return pool[math.random(#pool)]
end

-- C++ spell_marrowgar_coldflame SelectTarget (:482-495) twin: random
-- living player without IMPALED, excluding the victim (the spell's
-- -combatReach minimum-distance leg); falls back to any living player
-- (C++ :489-491).
local function pickColdflameTarget(creature)
    local targets = creature:GetAITargets()
    local victim = creature:GetVictim()
    local victimGuid = victim and victim:GetGUID() or 0
    local pool = {}
    for _, unit in ipairs(targets) do
        if unit:GetObjectType() == "Player" and not unit:IsDead() and not unit:HasAura(SPELL_IMPALED) and unit:GetGUID() ~= victimGuid then
            pool[#pool + 1] = unit
        end
    end
    if #pool == 0 then
        for _, unit in ipairs(targets) do
            if unit:GetObjectType() == "Player" and not unit:IsDead() then
                pool[#pool + 1] = unit
            end
        end
    end
    if #pool == 0 then
        return nil
    end
    return pool[math.random(#pool)]
end

-- Forward declarations (tharon_ja/gal_darah idiom): the storm machine
-- functions reference each other, so plain assignments, never a second
-- `local function` for the same name (that shadows the forward slot).
local onWarnBoneStorm, onBoneStormBegin, onBoneStormMove, onBoneStormEnd
local onEnableBoneSlice, onBoneSliceTick, onGraveyard, onColdflame

-- C++ EVENT_WARN_BONE_STORM (:214-221): _boneSlice = false, Talk
-- (EMOTE_BONE_STORM 7), DoCast(me, SPELL_BONE_STORM) self-cast, BEGIN
-- @3050ms, WARN re-arm {90s,95s}. The DelayEvents(3s, GROUP_SPECIAL)
-- leg is a documented no-op: the graveyard/coldflame timers keep their
-- cadence through the storm (their gates already read the storm aura at
-- fire time). FinishSpell has no bridge.
onWarnBoneStorm = function(creature, guid)
    local st = getMarrowgarState(guid)
    st.boneSlice = false
    creature:Talk(EMOTE_BONE_STORM)
    creature:CastSpell(creature, SPELL_BONE_STORM)
    schedule(guid, "beginStorm", 3050, function() onBoneStormBegin(creature, guid) end)
    schedule(guid, "warnStorm", {90000, 95000}, function() onWarnBoneStorm(creature, guid) end)
end

-- C++ EVENT_BONE_STORM_BEGIN (:223-229): aura SetDuration
-- (_boneStormDuration), SetSpeedRate x3, Talk(SAY_BONE_STORM 2), END @
-- duration+1ms, fallthrough to MOVE. Duration RAID_MODE(20s, 30s, 20s,
-- 30s) = 10-man 20s / 25-man 30s (:136); the x3 rate multiplies the
-- GetSpeed(1)/7.0 rate captured at engage.
onBoneStormBegin = function(creature, guid)
    local st = getMarrowgarState(guid)
    local aura = creature:GetAura(SPELL_BONE_STORM)
    if aura then
        aura:SetDuration(st.stormDuration)
    end
    creature:SetSpeedRate(1, st.baseRunRate * 3.0)
    creature:Talk(SAY_BONE_STORM)
    schedule(guid, "endStorm", st.stormDuration + 1, function() onBoneStormEnd(creature, guid) end)
    onBoneStormMove(creature, guid)
end

-- C++ EVENT_BONE_STORM_MOVE (:230-238): re-arm @ duration/3, random
-- non-tank target, MovePoint(POINT_TARGET_BONESTORM_PLAYER, *unit).
-- Documented approx: MoveChase(target) — no GetX/GetY exists on handler
-- creatures or players, so the target's position is unreadable and no
-- point arrival ever fires (the MovementInform MoveIdle leg stays out);
-- the boss still runs at a fresh random non-tank target every
-- duration/3, and END re-chases the victim.
onBoneStormMove = function(creature, guid)
    local st = getMarrowgarState(guid)
    schedule(guid, "moveStorm", math.floor(st.stormDuration / 3), function() onBoneStormMove(creature, guid) end)
    local target = pickRandomNonTank(creature)
    if target then
        creature:MoveChase(target)
    end
end

-- C++ EVENT_BONE_STORM_END (:240-258): drop the point movement,
-- MoveChase victim, speed restore, cancel MOVE, ENABLE_BONE_SLICE @10s,
-- reschedule GRAVEYARD @15s when not heroic.
onBoneStormEnd = function(creature, guid)
    cancelTimer(guid, "moveStorm")
    creature:MoveClear()
    local victim = creature:GetVictim()
    if victim then
        creature:MoveChase(victim)
    end
    creature:SetSpeedRate(1, 0)
    schedule(guid, "enableSlice", 10000, function() onEnableBoneSlice(creature, guid) end)
    if not creature:IsHeroic() then
        schedule(guid, "graveyard", 15000, function() onGraveyard(creature, guid) end)
    end
end

-- C++ EVENT_ENABLE_BONE_SLICE (:259-261): _boneSlice = true.
onEnableBoneSlice = function(creature, guid)
    getMarrowgarState(guid).boneSlice = true
end

-- C++ UpdateAI tail (:274-277): while _boneSlice and no current melee
-- spell, DoCastVictim(SPELL_BONE_SLICE). No per-tick UpdateAI hook
-- exists in Lua, so a 1.5s poller (the spell GCD) stands in, gated on
-- IsCasting() for the CURRENT_MELEE_SPELL + casting gates; each cast
-- marks the victim spike-immune (the bone-slice SpellScript SplitDamage
-- leg, :647-656 — its BeforeCast CLEAR_SPIKE_IMMUNITIES sender ports
-- here as the pre-cast clear, and the per-hit-target marking rides
-- the ev2 handler below once Spell.dbc ships). The damage split is a
-- C++ no-op (SetHitDamage(GetHitDamage() / _targetCount) with
-- GetHitDamage() 0 at OnHit, Spell.cpp:2377) and ports as nothing.
-- BUGFIX 2026-11-09: SPELL_BONE_SLICE was never defined in this file,
-- so the cast below passed a nil spell id, CastSpell raised "value is
-- not unsigned", and the handler unwound before the marking AND the
-- re-arm schedule — the poller died on its first eligible tick and
-- bone slice never fired (silent since tick 493).
onBoneSliceTick = function(creature, guid)
    local st = getMarrowgarState(guid)
    if st.boneSlice and not creature:HasAura(SPELL_BONE_STORM) and not creature:IsCasting() then
        local victim = creature:GetVictim()
        if victim then
            st.spikeImmune = {}
            creature:CastSpell(nil, SPELL_BONE_SLICE)
            local vg = victim:GetGUID()
            local immune = false
            for i = 1, math.min(#st.spikeImmune, 3) do
                if st.spikeImmune[i] == vg then immune = true end
            end
            if not immune then
                st.spikeImmune[#st.spikeImmune + 1] = vg
            end
        end
    end
    schedule(guid, "boneSlice", 1500, function() onBoneSliceTick(creature, guid) end)
end

-- C++ spell_marrowgar_bone_slice::SplitDamage
-- (boss_lord_marrowgar.cpp:647-656, OnHit, spell 69055): every hit unit
-- is handed to the caster AI via SetGUID(DATA_SPIKE_IMMUNE 1), which
-- pushes the GUID into _boneSpikeImmune (read back by the graveyard
-- BoneSpikeTargetSelector as DATA_SPIKE_IMMUNE+0..2, MAX_BONE_SPIKE_IMMUNE
-- 3, :98/:107-126). The ev2 twin fires once per hit target, pre-damage,
-- miss-inclusive with a nil target (boss_ai.go pass-1); a nil target is
-- skipped — Go has no GUID for a missed unit, where C++ marks it anyway,
-- but the poller above still marks the victim unconditionally, matching
-- the C++ "even if the spell missed" comment for the main target.
-- DORMANT until Spell.dbc ships: 69055 is not spellKnown, so the engine
-- fires no ev2 today; the poller's victim marking is the live leg until
-- then. Handler is inline: the file sits near the 200-local limit.
RegisterSpellEvent(SPELL_BONE_SLICE, 2, function(event, spell, target)
    if target == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local st = getMarrowgarState(caster:GetGUIDLow())
    local tg = target:GetGUID()
    local immune = false
    for i = 1, math.min(#st.spikeImmune, 3) do
        if st.spikeImmune[i] == tg then immune = true end
    end
    if not immune then
        st.spikeImmune[#st.spikeImmune + 1] = tg
    end
end)

-- C++ EVENT_BONE_SPIKE_GRAVEYARD (:200-203) + spell_marrowgar_bone_spike_
-- graveyard HandleSpikes (:575-600) replicated in Lua: the 69057 cast
-- itself is skipped (its SpellScript has no bridge and Go runs no
-- SPELL_EFFECT_SUMMON executor) and the script body ports directly —
-- gate IsHeroic() or no bone-storm aura, boneSpikeCount = 25-man and 3
-- or 1 (spawnMode & 1, :585), BoneSpikeTargetSelector (:107-126):
-- living players without IMPALED and not in the bone-slice immune list,
-- random picks; each target summons its own spike (target:SummonCreature,
-- the target-cast-summon-on-self :590 twin — the spike's ev22 impaled +
-- ride-vehicle + 8s fail-boned legs below are now live); Talk
-- SAY_BONESPIKE after the loop. Re-arm {15s,20s} always (C++ :203).
-- The dead-target spike-despawn leg stays out (no aura-caster-GUID bridge).
onGraveyard = function(creature, guid)
    local st = getMarrowgarState(guid)
    if creature:IsHeroic() or not creature:HasAura(SPELL_BONE_STORM) then
        local targets = creature:GetAITargets()
        local pool = {}
        for _, unit in ipairs(targets) do
            if unit:GetObjectType() == "Player" and not unit:IsDead() and not unit:HasAura(SPELL_IMPALED) then
                local ug = unit:GetGUID()
                local immune = false
                for i = 1, math.min(#st.spikeImmune, 3) do
                    if st.spikeImmune[i] == ug then immune = true end
                end
                if not immune then
                    pool[#pool + 1] = unit
                end
            end
        end
        local count = 1
        if creature:Is25ManRaid() then count = 3 end
        local picks = {}
        while #picks < count and #pool > 0 do
            picks[#picks + 1] = table.remove(pool, math.random(#pool))
        end
        if #picks > 0 then
            for _, target in ipairs(picks) do
                target:SummonCreature(ENTRY_BONE_SPIKE, 0, 0, 0, 0)
            end
            creature:Talk(SAY_BONESPIKE)
        end
    end
    schedule(guid, "graveyard", {15000, 20000}, function() onGraveyard(creature, guid) end)
end

-- C++ EVENT_COLDFLAME (:205-212) + spell_marrowgar_coldflame SelectTarget
-- (:482-495) replicated in Lua: the 69140/72705 casts themselves are
-- skipped (their SpellScripts have no bridge) and the script body ports
-- directly — the coldflame summon lands via the target player
-- (target:SummonCreature, the spell's dest-area summon twin; the
-- summoner is the player, not marrowgar — the C++ IsSummonedBy
-- owner-guard + GetNearPoint2D reposition legs stay out, no position
-- bridge — so the spawn sits at the 2x-reach fallback point beside the
-- target instead of 15y from marrowgar toward it). Storm: 4 coldflames
-- (the 72705 script's 4 casts, :518-521). Re-arm 5s always (C++ :212).
onColdflame = function(creature, guid)
    local target = pickColdflameTarget(creature)
    if target then
        local count = 1
        if creature:HasAura(SPELL_BONE_STORM) then count = 4 end
        for _ = 1, count do
            target:SummonCreature(ENTRY_COLDFLAME, 0, 0, 0, 0)
        end
    end
    schedule(guid, "coldflame", 5000, function() onColdflame(creature, guid) end)
end

-- C++ :261-263 EVENT_ENRAGE: DoCast(me, SPELL_BERSERK, true) +
-- Talk(SAY_BERSERK 6) — 10min one-shot, no phase/machine/random-target
-- gates (the festergut/rotface/putricide/sindragosa berserk precedent);
-- TRIGGERED self-cast, cast-before-Talk kept.
local function onEnrage(creature, guid)
    -- C++ UpdateAI pre-loop gate :193 sits BEFORE ExecuteEvent (:196),
    -- so a due enrage stays queued while casting — 500ms IsCasting
    -- defer, no re-arm consumed (the mal_ganis idiom; the in-loop
    -- gate :267 covers subsequent queued events in the same C++ tick,
    -- nothing to port for this single Lua tick).
    if creature:IsCasting() then
        schedule(guid, "enrage", 500, function() onEnrage(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_BERSERK, true)
    creature:Talk(SAY_BERSERK)
end

-- C++ boss_lord_marrowgar::JustEngagedWith: Talk(SAY_AGGRO) — the
-- auriaya engage-port precedent. The EventMap seeds (:148-152) —
-- all five port 2026-11-08 (tick 493, below).
local function marrowgarJustEngagedWith(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    -- C++ :162 — me-form DoZoneInCombat after Talk(SAY_AGGRO) and
    -- me->setActive(true) (C++ arm order); :163 SetBossState
    -- IN_PROGRESS PORTED 2026-11-05 via creature:SetBossState
    -- (lua_creature_events.go:5917).
    creature:DoZoneInCombat()
    creature:SetBossState(DATA_LORD_MARROWGAR, BOSS_STATE_IN_PROGRESS)
    schedule(guid, "enrage", 600000, function() onEnrage(creature, guid) end)
    -- C++ Reset seeds (:148-152): ENABLE_BONE_SLICE 10s, GRAVEYARD 15s,
    -- COLDFLAME 5s, WARN_BONE_STORM {45s,50s}, ENRAGE 10min (above).
    -- Seeded here (engage follows reset; the enrage precedent). The
    -- bone-slice poller starts now and reads the _boneSlice latch.
    local runSpeed = creature:GetSpeed(1)
    if runSpeed <= 0 then runSpeed = 7.0 end
    local st = getMarrowgarState(guid)
    st.baseRunRate = runSpeed / 7.0
    st.stormDuration = 20000
    if creature:Is25ManRaid() then st.stormDuration = 30000 end
    schedule(guid, "graveyard", 15000, function() onGraveyard(creature, guid) end)
    schedule(guid, "coldflame", 5000, function() onColdflame(creature, guid) end)
    schedule(guid, "warnStorm", {45000, 50000}, function() onWarnBoneStorm(creature, guid) end)
    schedule(guid, "enableSlice", 10000, function() onEnableBoneSlice(creature, guid) end)
    schedule(guid, "boneSlice", 1500, function() onBoneSliceTick(creature, guid) end)
end

-- C++ Reset() event machinery — cancel/re-arm on combat events (the
-- maiden convention).
local function marrowgarLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

local function marrowgarReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    -- C++ Reset() :144 — _Reset() first: SetBossState NOT_STARTED
    -- only when the stored state != DONE (BossAI::_Reset,
    -- ScriptedCreature.cpp:499-509; the falric precedent — ev23
    -- fires on spawn/death/evade too, and a DONE boss must stay
    -- DONE). PORTED 2026-11-05 via creature:SetBossState.
    if creature:GetBossState(DATA_LORD_MARROWGAR) ~= BOSS_STATE_DONE then
        creature:SetBossState(DATA_LORD_MARROWGAR, BOSS_STATE_NOT_STARTED)
    end
    -- C++ Reset() :144-146: SetSpeedRate(MOVE_RUN, _baseSpeed) restores the
    -- template rate (_baseSpeed is GetSpeedRate(MOVE_RUN) captured in the AI
    -- constructor); rate <= 0 clears the script override, restoring the
    -- template rate. Then RemoveAurasDueToSpell(SPELL_BONE_STORM) and
    -- RemoveAurasDueToSpell(SPELL_BERSERK) — order kept (RemoveAura bridge,
    -- boss_lady_deathwhisper.lua precedent). Without this an evade during
    -- bone storm re-seeds the storm phase with the aura still on.
    creature:SetSpeedRate(1, 0)
    creature:RemoveAura(SPELL_BONE_STORM)
    creature:RemoveAura(SPELL_BERSERK)
    -- C++ Reset() :153-154: _boneSlice = false, _boneSpikeImmune.clear().
    local rst = getMarrowgarState(creature:GetGUIDLow())
    rst.boneSlice = false
    rst.spikeImmune = {}
    -- C++ Reset() :177: instance->SetData(DATA_BONED_ACHIEVEMENT 14,
    -- true) — re-arms the Boned eligibility latch on every reset
    -- (wipe or evade); the Go 12775/12962/13393/13394 check reads
    -- bonedEligible from the shared instanceData table (default 1,
    -- the C++ ctor :156 value), cleared by the bone-spike 8s fail
    -- timer below. Written as 1/0, not booleans —
    -- InstanceDataNumber reads numbers only (instance_data.go:107),
    -- so a boolean write reads absent and the Go default 1 would
    -- fail-open the gate (the 2026-11-08 fix; the Onyxia 1/0
    -- precedent).
    local mapObj = creature:GetMap()
    if mapObj then
        local data = mapObj:GetInstanceData()
        if data then
            data.bonedEligible = 1
        end
    end
end

-- C++ boss_lord_marrowgar::KilledUnit:
-- if (victim->GetTypeId() == TYPEID_PLAYER) Talk(SAY_KILL) — the
-- razuvious player-gated variant precedent.
local function marrowgarKilledUnit(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

-- C++ boss_lord_marrowgar::JustDied: Talk(SAY_DEATH) — the sjonnir
-- JustDied-Talk precedent; _JustDied() (:170) PORTED 2026-11-05 via
-- creature:SetBossState DONE, in C++ order AFTER Talk. cancelTimers
-- stops the 10-min berserk timer from firing on the corpse.
local function marrowgarJustDied(event, creature, killer)
    cancelTimers(creature:GetGUIDLow())
    creature:Talk(SAY_DEATH)
    creature:SetBossState(DATA_LORD_MARROWGAR, BOSS_STATE_DONE)
end

-- C++ boss_lord_marrowgar::JustReachedHome (:174-177): _JustReachedHome()
-- (setActive(false), unmodeled) then SetBossState(FAIL) PORTED
-- 2026-11-05 via creature:SetBossState on ev24 OnReachHome
-- (creaturemotion.go home-move finalize, the opera ev24 precedent).
-- The SetData(DATA_BONED_ACHIEVEMENT 14, true) reset leg PORTS via
-- the shared instanceData bonedEligible key (marrowgarReset, above).
local function marrowgarReachHome(event, creature)
    creature:SetBossState(DATA_LORD_MARROWGAR, BOSS_STATE_FAIL)
end

RegisterCreatureEvent(ENTRY_LORD_MARROWGAR, 1, marrowgarJustEngagedWith)
RegisterCreatureEvent(ENTRY_LORD_MARROWGAR, 2, marrowgarLeaveCombat)
RegisterCreatureEvent(ENTRY_LORD_MARROWGAR, 3, marrowgarKilledUnit)
RegisterCreatureEvent(ENTRY_LORD_MARROWGAR, 4, marrowgarJustDied)
RegisterCreatureEvent(ENTRY_LORD_MARROWGAR, 23, marrowgarReset)
RegisterCreatureEvent(ENTRY_LORD_MARROWGAR, 24, marrowgarReachHome)

-- C++ npc_bone_spike::JustDied (boss_lord_marrowgar.cpp:424-431): the
-- trapped-unit aura-removal leg rides ToTempSummon()->GetSummonerUnit()
-- (creature:GetSummonerUnit, bridged 2026-11-05 — TemporarySummon.cpp:
-- 48-53 over the summoner's CreatedByGUID; the summoner is the trapped
-- player); me->DespawnOrUnsummon() ports (creature:Despawn, bridged).
-- C++ order kept: aura removal, then despawn.
local function boneSpikeJustDied(event, creature, killer)
    cancelTimers(creature:GetGUIDLow())
    local summoner = creature:GetSummonerUnit()
    if summoner then
        summoner:RemoveAura(SPELL_IMPALED)
    end
    creature:Despawn()
end

-- C++ npc_bone_spike::KilledUnit (:433-437): me->DespawnOrUnsummon() +
-- victim->RemoveAurasDueToSpell(SPELL_IMPALED), C++ order kept;
-- victim:RemoveAura is the player/creature RemoveAura bridge.
local function boneSpikeKilledUnit(event, creature, victim)
    cancelTimers(creature:GetGUIDLow())
    creature:Despawn()
    victim:RemoveAura(SPELL_IMPALED)
end

-- C++ npc_bone_spike::IsSummonedBy (boss_lord_marrowgar.cpp:439-447):
-- DoCast(summoner, SPELL_IMPALED 69065) + summoner->CastSpell(me,
-- SPELL_RIDE_VEHICLE 46598, true); the summoner is the trapped player
-- (the graveyard SpellScript has the target cast the summon spell on
-- itself, :590) and ev22 passes it as the second arg (the
-- summonTempCreature player-summoner fire site,
-- lua_creature_events.go:200-201; player:CastSpell live at
-- lua_player.go:2752, inside luaPlayer()). C++ order kept: impaled,
-- then ride-vehicle (triggered). The 8s EVENT_FAIL_BONED timer PORTS
-- (:444-445 schedule, :470-475 fire while a unit is trapped, only
-- effect instance SetData(DATA_BONED_ACHIEVEMENT 14, false) — the
-- shared instanceData bonedEligible key is the Go-side twin; the
-- timer is cancelled on spike death/kill above because in C++ the
-- timer is cancelled on spike death/kill above because in C++ the
-- EventMap dies with the spike AI. LIVE 2026-11-08 (tick 493): the
-- graveyard summon chain ports in onGraveyard below
-- (target:SummonCreature, the target-cast-summon-on-self :590 twin).
local function boneSpikeIsSummonedBy(event, creature, summoner)
    if not summoner then
        return
    end
    creature:CastSpell(summoner, SPELL_IMPALED)
    summoner:CastSpell(creature, SPELL_RIDE_VEHICLE, true)
    schedule(creature:GetGUIDLow(), "failBoned", 8000, function()
        local mapObj = creature:GetMap()
        if mapObj then
            local data = mapObj:GetInstanceData()
            if data then
                data.bonedEligible = 0
            end
        end
    end)
end

RegisterCreatureEvent(ENTRY_BONE_SPIKE, 3, boneSpikeKilledUnit)
RegisterCreatureEvent(ENTRY_BONE_SPIKE, 4, boneSpikeJustDied)
RegisterCreatureEvent(ENTRY_BONE_SPIKE, 22, boneSpikeIsSummonedBy)

-- C++ npc_coldflame::IsSummonedBy (boss_lord_marrowgar.cpp:369-395): the
-- reposition legs (GetLastColdflamePosition / GetAbsoluteAngle /
-- GetNearPoint2D / NearTeleportTo) stay out — no position bridge — and
-- the 500ms EVENT_COLDFLAME_TRIGGER teleport leg stays out with them;
-- the DoCast(SPELL_COLDFLAME_SUMMON 69147) leg PORTS. Registered here
-- (same chunk as the summoner, sharing the entry const).
local function coldflameIsSummonedBy(event, creature, summoner)
    creature:CastSpell(creature, SPELL_COLDFLAME_SUMMON)
end

RegisterCreatureEvent(ENTRY_COLDFLAME, 22, coldflameIsSummonedBy)
