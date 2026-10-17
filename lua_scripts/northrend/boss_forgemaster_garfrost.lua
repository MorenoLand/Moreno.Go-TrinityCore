-- Forgemaster Garfrost (Pit of Saron) — Lua port of
-- src/server/scripts/Northrend/FrozenHalls/PitOfSaron/boss_forgemaster_garfrost.cpp
-- (356 lines incl. license; 3 scripts — boss_garfrost
-- (CreatureScript via GetPitOfSaronAI (BossAI), DATA_GARFROST = 0);
-- spell_garfrost_permafrost (SpellScriptLoader: BeforeHit saronite-rock
-- LoS check -> ApplySpellImmune + PreventHit legs; AfterHit immunity
-- restore); achievement_doesnt_go_to_eleven (AchievementCriteriaScript
-- OnCheck target->ToCreature AI()->GetData(ACHIEV_DOESNT_GO_TO_ELEVEN)
-- <= 10); all registered from inside AddSC_boss_garfrost(); loader
-- decl 162 / call 357 per northrend_script_loader.cpp — the THIRD group
-- of the "// Pit of Saron" block in AddNorthrendScripts(), immediately
-- after AddSC_pit_of_saron() (call 356); the calls after it are
-- AddSC_boss_ick() (call 358) / AddSC_boss_tyrannus() (call 359).
-- Entries: 36494 Forgemaster Garfrost (pit_of_saron.h NPC_GARFROST,
-- line 49; instance_pit_of_saron.cpp OnCreatureCreate binds case
-- NPC_GARFROST, line 74) — entry-verifiable, registration proceeds (the
-- nexus_commanders / malygos / sartharion kalecgos precedent); the
-- CreatureScript ScriptName binding is DB-side as usual. DATA_GARFROST
-- = 0 (pit_of_saron.h line 28); DATA_TYRANNUS = 2 (line 30).
-- Sole-source verified: whole-server-tree grep for "boss_garfrost"
-- hits boss_forgemaster_garfrost.cpp only (+ the loader decl/call
-- lines); this clone carries no sql/ tree, so ScriptName bindings are
-- DB-side by construction.
-- Registered in Go: engine/world/boss_ai.go
-- RegisterLuaBoss("boss_forgemaster_garfrost", 36494) (line 10164) —
-- no Go registration changes needed for this re-port.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 9 OnDamageTaken, 23 OnReset. Timers via CreateLuaEvent
-- ({min,max} table = randtime range); melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady (:248).
--
-- DOCTRINE RETRACTION (2026-10-13): the Oct-2 port of this file treated
-- the whole EventMap machine as unbridgeable under the "no-timer-bridge"
-- doctrine and ported Talk arms only. That doctrine is disproved and
-- stale — CreateLuaEvent is a real registered bridge
-- (engine/scripting/runtime.go:798), and timer-driven cast arms port on
-- the maiden_of_virtue / moroes / curator / nightbane convention:
-- timers table keyed by creature GUID, cancelled on Eluna events 2
-- (leave combat), 4 (death), 23 (reset); CastSpell(nil, spellId) =
-- victim-cast (bridge falls back to TargetGUID); CastSpell(creature,
-- spellId) = self-cast; CastSpell(target, spellId) = explicit-target
-- cast; {min,max} delay = randtime range; TRIGGERED casts pass true as
-- the third CastSpell arg. None of the ported arms here is a TRIGGERED
-- cast (every C++ DoCast is 1-2 arg, non-triggered).
-- The phase machine is a SIMPLE LATCH (DamageTaken flips a per-GUID
-- phase flag; no summon lists, MoveFollow, or DoAction
-- cross-creature infrastructure), so it ports — the bronjahm 30%-hp
-- event-9 latch precedent. Phase-gated arms stay phase-gated (the
-- phase-gating rule): CHILLING_WAVE fires only in phase 2, DEEP_FREEZE
-- only in phase 3; no phase-1 casts are fabricated for phase-2/3 arms.
--
-- PORTED ARMS (C++ line refs; all C++-exact for the modeled arms):
-- JustEngagedWith (:107-114) — Talk(SAY_AGGRO 0) (:110) unconditional;
--   non-triggered self-cast DoCast(me, SPELL_PERMAFROST 70326) (:111);
--   EVENT_THROW_SARONITE seed at 7000ms (:113).
-- KilledUnit (:116-120) — Talk(SAY_SLAY 4) player-gated (:119), the
--   razuvious player-gated variant precedent.
-- JustDied (:122-130) — Talk(SAY_DEATH 3) (:125) unconditional, the
--   sjonnir JustDied-Talk precedent.
-- DamageTaken (:132-152) — simple latch: event 9 fires pre-application,
--   so the post-damage health decides ((GetHealth()-damage), floored
--   thresholds like C++ !HealthAbovePct — the moroes convention).
--   Phase 1, post-damage <= 66% (:134): flip to PHASE_TWO, Talk(1)
--   (:137), non-triggered self-cast THUNDERING_STOMP 68771 (:139),
--   DelayEvents approximation (see deviations), forge-jump seed at
--   1500ms (:140). Phase 2, post-damage <= 33% (:144): flip to
--   PHASE_THREE, Talk(2) (:147), self-cast 68771 (:149), same
--   scheduling (:150). The first leg returns after flipping (:141),
--   so one massive hit from >66% to <33% flips only to phase 2 —
--   replicated by the early return.
-- EVENT_THROW_SARONITE (:206-213) — all phases: random player
--   (SelectTarget Random, 0, 0.0f, playerOnly — the GetPlayersInWorld
--   alive-player precedent; 0.0f = no distance filter), Talk(5, target)
--   (:209, 2-arg Talk — the buru precedent), non-triggered cast 68788
--   on the target (:210); re-arm {12500,20000}ms unconditional (:212,
--   outside the target-if).
-- EVENT_CHILLING_WAVE (:214-217) — PHASE_TWO-gated: DoCast(me, 68778)
--   self-cast non-triggered (:215); re-arm 4000ms phase-2-gated (:216)
--   — the guard kills the chain on a phase-3 flip, mirroring the
--   inert queued event.
-- EVENT_DEEP_FREEZE (:218-225) — PHASE_THREE-gated: random player,
--   Talk(6, target) (:221), non-triggered cast 70381 on the target
--   (:222); re-arm 35000ms phase-3-gated (:224).
-- EVENT_FORGE_JUMP (:226-232) / EVENT_RESUME_ATTACK (:233-239) —
--   timer skeleton ported C++-exact: forge-jump fires at 1500ms
--   (:140/:150), schedules the resume leg at 5000ms (the
--   MovementInform -> EVENT_RESUME_ATTACK delay, :171); resume fires
--   the first phase arm at 5000ms chilling (:235) / 10000ms
--   deep-freeze (:237), phase-gated. The physical jump and its
--   landing legs do not port (see documented-only); only the jump
--   flight time is collapsed (landing assumed at forge-jump fire).
--
-- DOCUMENTED-ONLY (no registration beyond entry 36494):
-- MovementInform (:155-172) — the EFFECT_MOTION_TYPE / POINT_FORGE (0)
--   landing of the unbridged MoveJump (:79-80 forge positions, 25.0f
--   speed, 15.0f height): PHASE_TWO DoCast(me, SPELL_FORGE_BLADE 68774)
--   + SetEquipmentSlots(false, EQUIP_ID_SWORD 49345) (:160-164);
--   PHASE_THREE RemoveAurasDueToSpell(SPELL_FORGE_BLADE_HELPER) +
--   DoCast(me, SPELL_FORGE_MACE 68785) + SetEquipmentSlots(false,
--   EQUIP_ID_MACE 49344) (:165-170). Firing these on a bare timer
--   would fabricate a landing that never happens — the casts ride the
--   no-movement bridge out, the equipment changes ride the
--   no-equipment bridge. (FORGE_BLADE_HELPER is RAID_MODE(68774,
--   70334): the 10-man removal target is the same 68774 that was cast.)
-- Reset (:99-105) — SetEquipmentSlots(true) (:103) has no equipment
--   bridge; SetPhase(PHASE_ONE) (:102) is the phase init.
-- JustDied :126 me->RemoveAllGameObjects() — no bridge (Go engine
--   owns object cleanup).
-- JustDied :128-129 tyrannus->AI()->Talk(SAY_TYRANNUS_DEATH 0) via
--   instance->GetGuidData(DATA_TYRANNUS) — instance-driven Talk on
--   another creature's AI, no bridge (the instance_violet_hold
--   ScheduleCyanigosaIntro precedent).
-- CallForHelp(70.0f) (:112) — no bridge.
-- SpellHitTarget (:174-186) — SPELL_PERMAFROST_HELPER aura-stack
--   capture into _permafrostStack: the hook bridge EXISTS
--   (CREATURE_EVENT_ON_SPELL_HIT_TARGET, event 15 — boss_ai.go,
--   the faerlina SpellHit port), but the arm stays out because its
--   only consumer is GetData -> the achievement leg (the zuramat /
--   devourer_of_souls precedent).
-- GetData (:187-190) — no GetData bridge (the boss_zuramat precedent).
-- UpdateAI :198/:244 HasUnitState(UNIT_STATE_CASTING) gates — no
--   unit-state bridge; timers fire unconditionally (the maiden
--   precedent; creature casts are packet-visual in Go).
-- UpdateAI :227 AttackStop / :238 AttackStart(me->GetVictim()) —
--   melee targeting is engine-driven in Go, no bridge needed.
-- spell_garfrost_permafrost (:261-333, SpellScriptLoader: BeforeHit
--   saronite-rock LoS check + ApplySpellImmune/PreventHit legs,
--   AfterHit immunity restore) — no SpellScript bridge (the
--   boss_moragg optic-link precedent), stays in the standing queue.
-- achievement_doesnt_go_to_eleven (:335-353, OnCheck GetData <= 10) —
--   no GetData bridge; joins the unmodeled-achievement queue (the
--   cyanigosa / ichoron precedent).
--
-- DEVIATIONS FROM C++ (documented; every other value C++-exact):
-- DelayEvents(8s) (:138/:148): C++ shifts the pending saronite timer's
--   remaining by +8s, suppressing throws during the forge-jump
--   intermission. The remaining is unknowable from Lua (the saronite
--   arm is a randtime {12500,20000} roll), so the beat restarts at the
--   flip with the C++-exact {12500,20000} re-arm — throws stay
--   suppressed through the intermission, beat phase may differ by up
--   to one period (the devourer 18s-beat deviation precedent).
-- Forge-jump collapse: EVENT_FORGE_JUMP (:226-232) is AttackStop +
--   MoveJump in C++; with no movement bridge the jump flight time is
--   collapsed (landing assumed at forge-jump fire) while every timer
--   value stays C++-exact (1500 / 5000 / 5000 / 10000ms).
-- Double-flip corner: two phase flips inside 1.5s would queue two
--   FORGE_JUMP events in C++; the replace-per-key timer convention
--   (bronjahm) collapses them to one chain.
-- SelectTarget(Random, 0, 0.0f, true): approximated with a random
--   alive player on the same map/instance (the keleseth / thorim
--   GetPlayersInWorld precedent); C++ applies no distance filter
--   (0.0f), so none is applied.
-- DamageTaken hook is pre-application: post-damage health decides,
--   strictly at/below 66 / 33 like C++ !HealthAbovePct (the moroes
--   convention).
--
-- UPSTREAM C++ FINDINGS: none — no typos or dead code on the
-- name-by-name read. (SAY_TYRANNUS_DEATH = 0 reusing the value 0 is
-- intentional — it is Talked on tyrannus's own AI, not Garfrost's;
-- SPELL_FORGE_BLADE_HELPER's 10-man id 68774 intentionally matches
-- SPELL_FORGE_BLADE so the phase-3 removal clears the phase-2 buff.)

local ENTRY_GARFROST = 36494

local SAY_AGGRO = 0
local SAY_PHASE2 = 1
local SAY_PHASE3 = 2
local SAY_DEATH = 3
local SAY_SLAY = 4
local SAY_THROW_SARONITE = 5
local SAY_CAST_DEEP_FREEZE = 6

local SPELL_PERMAFROST = 70326
local SPELL_THROW_SARONITE = 68788
local SPELL_THUNDERING_STOMP = 68771
local SPELL_CHILLING_WAVE = 68778
local SPELL_DEEP_FREEZE = 70381

local PHASE_ONE = 1
local PHASE_TWO = 2
local PHASE_THREE = 3

local timers = {}
local garfrostState = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

-- bronjahm-style: re-scheduling a key replaces the pending timer, so each
-- named arm has exactly one live timer at a time.
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

local function initState(guid)
    cancelTimers(guid)
    garfrostState[guid] = { phase = PHASE_ONE }
end

local function clearState(guid)
    cancelTimers(guid)
    garfrostState[guid] = nil
end

-- C++ SelectTarget(SelectTargetMethod::Random, 0, 0.0f, true) — random
-- alive player on the same map/instance; no distance filter (0.0f) —
-- the keleseth / thorim GetPlayersInWorld precedent.
local function randomPlayer(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local candidates = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ :206-213 EVENT_THROW_SARONITE (all phases): random player,
-- Talk(SAY_THROW_SARONITE), DoCast(target, 68788) non-triggered;
-- re-arm {12500,20000}ms unconditional (outside the target-if).
local function onThrowSaronite(creature, guid)
    local st = garfrostState[guid]
    if not st then
        return
    end
    local target = randomPlayer(creature)
    if target then
        creature:Talk(SAY_THROW_SARONITE, target)
        creature:CastSpell(target, SPELL_THROW_SARONITE)
    end
    schedule(guid, "saronite", {12500, 20000}, function() onThrowSaronite(creature, guid) end)
end

-- C++ :214-217 EVENT_CHILLING_WAVE (PHASE_TWO only): DoCast(me, 68778)
-- self-cast non-triggered; re-arm 4000ms phase-2-gated — the guard kills
-- the chain on a phase-3 flip, mirroring the inert queued event.
local function onChillingWave(creature, guid)
    local st = garfrostState[guid]
    if not st then
        return
    end
    if st.phase ~= PHASE_TWO then
        return
    end
    creature:CastSpell(creature, SPELL_CHILLING_WAVE)
    schedule(guid, "chilling", 4000, function() onChillingWave(creature, guid) end)
end

-- C++ :218-225 EVENT_DEEP_FREEZE (PHASE_THREE only): random player,
-- Talk(SAY_CAST_DEEP_FREEZE), DoCast(target, 70381) non-triggered;
-- re-arm 35000ms phase-3-gated.
local function onDeepFreeze(creature, guid)
    local st = garfrostState[guid]
    if not st then
        return
    end
    if st.phase ~= PHASE_THREE then
        return
    end
    local target = randomPlayer(creature)
    if target then
        creature:Talk(SAY_CAST_DEEP_FREEZE, target)
        creature:CastSpell(target, SPELL_DEEP_FREEZE)
    end
    schedule(guid, "deepfreeze", 35000, function() onDeepFreeze(creature, guid) end)
end

-- C++ :226-232 EVENT_FORGE_JUMP is AttackStop + MoveJump (no movement
-- bridge): the physical jump and its MovementInform landing legs do not
-- port (see header); the C++-exact 5000ms MovementInform ->
-- EVENT_RESUME_ATTACK delay (:171) is preserved, with the landing
-- assumed at forge-jump fire.
-- Forward declaration: onForgeJump's resume closure references
-- onResumeAttack before its local def below; without this the
-- closure would bind a nil global (festergut/moam nil-call class).
local onResumeAttack
local function onForgeJump(creature, guid)
    local st = garfrostState[guid]
    if not st then
        return
    end
    schedule(guid, "resume", 5000, function() onResumeAttack(creature, guid) end)
end

-- C++ :233-239 EVENT_RESUME_ATTACK: phase-2 seeds EVENT_CHILLING_WAVE at
-- 5s (:235), phase-3 seeds EVENT_DEEP_FREEZE at 10s (:237); the
-- AttackStart(me->GetVictim()) leg (:238) is engine-driven in Go.
onResumeAttack = function(creature, guid)
    local st = garfrostState[guid]
    if not st then
        return
    end
    if st.phase == PHASE_TWO then
        schedule(guid, "chilling", 5000, function() onChillingWave(creature, guid) end)
    elseif st.phase == PHASE_THREE then
        schedule(guid, "deepfreeze", 10000, function() onDeepFreeze(creature, guid) end)
    end
end

-- Shared DamageTaken phase-flip body (:134-141 / :144-151): Talk, the
-- non-triggered self-cast THUNDERING_STOMP, the DelayEvents
-- approximation (saronite beat restart — see header), and the
-- EVENT_FORGE_JUMP seed at 1500ms.
local function doPhaseFlip(creature, guid, newPhase, sayId)
    local st = garfrostState[guid]
    if not st then
        return
    end
    st.phase = newPhase
    creature:Talk(sayId)
    creature:CastSpell(creature, SPELL_THUNDERING_STOMP)
    schedule(guid, "saronite", {12500, 20000}, function() onThrowSaronite(creature, guid) end)
    schedule(guid, "forgejump", 1500, function() onForgeJump(creature, guid) end)
end

-- C++ :107-114 JustEngagedWith: Talk(SAY_AGGRO) unconditional; the
-- non-triggered self-cast DoCast(me, SPELL_PERMAFROST) (:111); the
-- EVENT_THROW_SARONITE seed at 7000ms (:113). CallForHelp(70.0f)
-- (:112) has no bridge.
local function garfrostEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    initState(guid)
    creature:Talk(SAY_AGGRO)
    creature:CastSpell(creature, SPELL_PERMAFROST)
    schedule(guid, "saronite", 7000, function() onThrowSaronite(creature, guid) end)
end

-- C++ :132-152 DamageTaken: simple phase latch (the bronjahm precedent).
-- C++ takes uint32& /*damage*/ (unnamed) — the !HealthAbovePct(66/33)
-- checks are on CURRENT health, no damage projection.
-- HealthAbovePct(pct) = health > CountPctFromMaxHealth(pct)
-- (Unit.h:874), so !above <=> health <= floor(0.pct*max): the floored
-- <= forms below are exact; a strict < would fire one hit late at the
-- exact boundary. The first leg returns after flipping (:141), so one
-- hit from >66% to <=33% flips only to phase 2 — replicated here.
local function garfrostDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local st = garfrostState[guid]
    if not st then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    local health = creature:GetHealth()
    if st.phase == PHASE_ONE and health <= math.floor(maxHealth * 66 / 100) then
        doPhaseFlip(creature, guid, PHASE_TWO, SAY_PHASE2)
        return
    end
    if st.phase == PHASE_TWO and health <= math.floor(maxHealth * 33 / 100) then
        doPhaseFlip(creature, guid, PHASE_THREE, SAY_PHASE3)
        return
    end
end

-- C++ :116-120 KilledUnit: if (victim->GetTypeId() == TYPEID_PLAYER)
-- Talk(SAY_SLAY) — the razuvious player-gated variant precedent.
local function garfrostKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ :122-130 JustDied: Talk(SAY_DEATH) unconditional — the sjonnir
-- JustDied-Talk precedent; the _JustDied() passthrough,
-- RemoveAllGameObjects(), and tyrannus->AI()->Talk legs have no bridge.
local function garfrostJustDied(event, creature)
    clearState(creature:GetGUID())
    creature:Talk(SAY_DEATH)
end

local function garfrostLeaveCombat(event, creature)
    clearState(creature:GetGUID())
end

local function garfrostReset(event, creature)
    clearState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_GARFROST, 1, garfrostEnterCombat)
RegisterCreatureEvent(ENTRY_GARFROST, 2, garfrostLeaveCombat)
RegisterCreatureEvent(ENTRY_GARFROST, 3, garfrostKilledUnit)
RegisterCreatureEvent(ENTRY_GARFROST, 4, garfrostJustDied)
RegisterCreatureEvent(ENTRY_GARFROST, 9, garfrostDamageTaken)
RegisterCreatureEvent(ENTRY_GARFROST, 23, garfrostReset)
