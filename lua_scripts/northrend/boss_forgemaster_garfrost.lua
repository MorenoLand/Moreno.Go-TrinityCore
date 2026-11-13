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
-- JustEngagedWith (:107-114) — BossAI base first: SetBossState
--   (DATA_GARFROST 0, IN_PROGRESS) via creature:SetBossState, then
--   Talk(SAY_AGGRO 0) (:110) unconditional; non-triggered self-cast
--   DoCast(me, SPELL_PERMAFROST 70326) (:111); CallForHelp(70.0f)
--   (:112, bridged — see header); EVENT_THROW_SARONITE seed at
--   7000ms (:113).
-- KilledUnit (:116-120) — Talk(SAY_SLAY 4) player-gated (:119), the
--   razuvious player-gated variant precedent.
-- JustDied (:122-130) — _JustDied() first: SetBossState
--   (DATA_GARFROST 0, DONE) via creature:SetBossState, then
--   Talk(SAY_DEATH 3) (:125) unconditional, the sjonnir
--   JustDied-Talk precedent; the tyrannus->AI()->Talk
--   (SAY_TYRANNUS_DEATH 0) leg (:128-129) is PORTED via the
--   instanceData mirror (see garfrostJustDied).
-- Reset (:99-105) — _Reset() first: SetBossState(DATA_GARFROST 0,
--   NOT_STARTED) != DONE-gated, then phase/equipment/init legs
--   (see garfrostReset; SetPhase + SetEquipmentSlots(true) stay out).
-- DamageTaken (:132-152) — simple latch: event 9 fires pre-application
--   (DealDamage calls DamageTaken at Unit.cpp:701, before ModifyHealth
--   at :894), so CURRENT health decides — !HealthAbovePct(66/33) is
--   health <= floor(max*pct/100) (Unit.h:874, CalculatePct truncates);
--   the code comment at garfrostDamageTaken states this correctly.
--   Phase 1, current-health <= 66% (:134): flip to PHASE_TWO, Talk(1)
--   (:137), non-triggered self-cast THUNDERING_STOMP 68771 (:139),
--   DelayEvents approximation (see deviations), forge-jump seed at
--   1500ms (:140). Phase 2, current-health <= 33% (:144): flip to
--   PHASE_THREE, Talk(2) (:147), self-cast 68771 (:149), same
--   scheduling (:150). The first leg returns after flipping (:141),
--   so one massive hit from >66% to <33% flips only to phase 2 —
--   replicated by the early return.
-- EVENT_THROW_SARONITE (:206-213) — all phases: random threat-list
--   player (SelectTarget Random, 0, 0.0f, playerOnly — GetAITargets +
--   Player + alive, tharon_ja precedent; 0.0f = no distance filter),
--   Talk(5, target)
--   (:209, 2-arg Talk — the buru precedent), non-triggered cast 68788
--   on the target (:210); re-arm {12500,20000}ms unconditional (:212,
--   outside the target-if).
-- EVENT_CHILLING_WAVE (:214-217) — PHASE_TWO-gated: DoCast(me, 68778)
--   self-cast non-triggered (:215); re-arm 4000ms phase-2-gated (:216)
--   — the guard kills the chain on a phase-3 flip, mirroring the
--   inert queued event.
-- EVENT_DEEP_FREEZE (:218-225) — PHASE_THREE-gated: random threat-list
--   player, Talk(6, target) (:221), non-triggered cast 70381 on the target
--   (:222); re-arm 35000ms phase-3-gated (:224).
-- EVENT_FORGE_JUMP (:226-232) / EVENT_RESUME_ATTACK (:233-239) —
-- PORTED 2026-11-08 (tick 490) on the real MoveJump bridge
-- (lua_creature_events.go:5143; MoveJump(x, y, z, speedXY, speedZ, id)
-- maps positionally onto C++ MoveJump(pos, 25.0f, 15.0f, POINT_FORGE),
-- the faction-champions precedent): the jump fires at 1500ms
-- (:140/:150) as AttackStop + MoveJump to the phase forge, and the
-- MovementInform landing legs fire on Eluna event 28
-- (EFFECT_MOTION_TYPE 16, creaturemotion.go:4310/:5723-5726 — the jump
-- spline stamps the effect type and the arrival fires ev28 with
-- (type, id)); the 5000ms MovementInform -> EVENT_RESUME_ATTACK delay
-- (:171) is preserved, and resume seeds the first phase arm at 5000ms
-- chilling (:235) / 10000ms deep-freeze (:237), phase-gated. The old
-- forge-jump collapse is retired — the flight time is now real.
--
-- DOCUMENTED-ONLY (no registration beyond entry 36494):
-- MovementInform (:155-172) — the EFFECT_MOTION_TYPE / POINT_FORGE (0)
-- landing of the bridged MoveJump is PORTED on event 28
-- (garfrostMovementInform below): PHASE_TWO DoCast(me,
-- SPELL_FORGE_BLADE 68774) + sword 49345 (:160-164); PHASE_THREE
-- RemoveAurasDueToSpell(SPELL_FORGE_BLADE_HELPER) +
-- DoCast(me, SPELL_FORGE_MACE 68785) + mace 49344 (:165-170).
--   (FORGE_BLADE_HELPER is RAID_MODE(68774, 70334); the shared
--   Difficulty enum maps dungeon normal 0 -> 68774 and heroic 1 ->
--   70334, so IsHeroic selects it exactly. The old "no-equipment
--   bridge" claim is stale: SetEquipmentSlots is bridged with
--   0xFFFFFFFF = untouched.)
-- Reset (:99-105) — SetEquipmentSlots(true) (:103) is the template-default
--   form (creature_equip_template default set); the Go bridge only takes
--   explicit IDs, so the form has no expressible mapping — that is the
--   sole blocker (the old "arm is inert" clause is stale: the
--   forge-sword/forge-mace landing legs ARE ported in
--   garfrostMovementInform, so equipment can diverge from template
--   after a phase flip; the template IDs themselves are DB-side).
--   Documented-only; SetPhase(PHASE_ONE) (:102) is the phase init.
-- JustDied :126 me->RemoveAllGameObjects() — no bridge (Go engine
--   owns object cleanup; whole-engine grep, 2026-11-08 tick 490).
-- CallForHelp(70.0f) (:112) — PORTED 2026-11-08 (tick 490):
--   creature:CallForHelp is bridged (lua_creature_events.go:1781,
--   Creature.cpp:2432-2456 via callCreatureForHelp).
-- JustDied SetBossState DONE-spawn leg (instance_pit_of_saron.cpp
--   :157-172) — PORTED 2026-11-08 (tick 490) in garfrostJustDied
--   below, C++ order (inside SetBossState, before Talk(DEATH)):
--   Martin Victus 1 (37591, Alliance) / Gorkun Ironskull 1 (37592,
--   Horde) at SlaveLeaderPos, TEMPSUMMON_MANUAL_DESPAWN (7).
-- SpellHitTarget (:174-186) — SPELL_PERMAFROST_HELPER aura-stack
--   capture into _permafrostStack: PORTED 2026-11-08 (tick 416) in
--   garfrostSpellHitTarget below (the achievement bridge now exists).
--   (2026-11-13: helper selector Is25ManRaid -> IsHeroic — C++
--   RAID_MODE(68786, 70336) takes 70336 on heroic dungeons
--   (Difficulty 1 = the 25-man slot); Is25ManRaid is raid-only, so
--   the old code never matched 70336 hits on heroic.)
-- GetData (:187-190) — creature:GetData IS bridged (ScriptData store,
-- 2026-10-24); the blocker is the achievement bridge (the boss_zuramat
-- precedent).
-- UpdateAI :198/:244 HasUnitState(UNIT_STATE_CASTING) gates — no
--   unit-state bridge; timers fire unconditionally (the maiden
--   precedent; creature casts are packet-visual in Go).
-- UpdateAI :227 AttackStop / :238 AttackStart(me->GetVictim()) —
--   melee targeting is engine-driven in Go, no bridge needed.
-- spell_garfrost_permafrost (:261-333, SpellScriptLoader: BeforeHit
--   saronite-rock LoS check + ApplySpellImmune/PreventHit legs,
--   AfterHit immunity restore) — PORTED 2026-11-12 below
--   (permafrostBeforeHit / permafrostAfterHit, ev6/ev3 on 70326;
--   the old "no SpellScript bridge" claim is retired — ev6/ev3 fire
--   on the creature path, boss_ai.go:3829/:4351).
-- achievement_doesnt_go_to_eleven (:335-353, OnCheck GetData <= 10) —
--   PORTED 2026-11-08 (tick 416): the SpellHitTarget peak latch rides
--   garfrostSpellHitTarget below; Go case in
--   engine/world/achievement_criteria_scripts.go.
--
-- DEVIATIONS FROM C++ (documented; every other value C++-exact):
-- DelayEvents(8s) (:138/:148): C++ shifts the pending saronite timer's
--   remaining by +8s, suppressing throws during the forge-jump
--   intermission. The remaining is unknowable from Lua (the saronite
--   arm is a randtime {12500,20000} roll), so the beat restarts at the
--   flip with the C++-exact {12500,20000} re-arm — throws stay
--   suppressed through the intermission, beat phase may differ by up
--   to one period (the devourer 18s-beat deviation precedent).
-- Double-flip corner: two phase flips inside 1.5s would queue two
--   FORGE_JUMP events in C++; the replace-per-key timer convention
--   (bronjahm) collapses them to one chain.
-- SelectTarget(Random, 0, 0.0f, true): random alive threat-list player
--   via GetAITargets (tharon_ja precedent; old GetPlayersInWorld pool
--   could pick threat-less players); C++ applies no distance filter
--   (0.0f), so none is applied.
-- DamageTaken hook is pre-application: CURRENT health decides,
--   strictly at/below 66 / 33 like C++ !HealthAbovePct (Unit.cpp:701
--   DamageTaken precedes :894 ModifyHealth).
--
-- UPSTREAM C++ FINDINGS: none — no typos or dead code on the
-- name-by-name read. (SAY_TYRANNUS_DEATH = 0 reusing the value 0 is
-- intentional — it is Talked on tyrannus's own AI, not Garfrost's;
-- SPELL_FORGE_BLADE_HELPER's 10-man id 68774 intentionally matches
-- SPELL_FORGE_BLADE so the phase-3 removal clears the phase-2 buff.)

local ENTRY_GARFROST = 36494
local DATA_GARFROST = 0 -- pit_of_saron.h:32

local SAY_AGGRO = 0
local SAY_PHASE2 = 1
local SAY_PHASE3 = 2
local SAY_DEATH = 3
local SAY_SLAY = 4
local SAY_THROW_SARONITE = 5
local SAY_CAST_DEEP_FREEZE = 6
local SAY_TYRANNUS_DEATH = 0 -- garfrost's own enum (boss_forgemaster_garfrost.cpp:38), Talked by tyrannus

local SPELL_PERMAFROST = 70326
local SPELL_PERMAFROST_HELPER_10 = 68786 -- RAID_MODE 10-man
local SPELL_PERMAFROST_HELPER_25 = 70336 -- RAID_MODE 25-man (heroic dungeon selects it)
local ACHIEV_DOESNT_GO_TO_ELEVEN = 0 -- boss_forgemaster_garfrost.cpp:66
local SPELL_THROW_SARONITE = 68788
local SPELL_THUNDERING_STOMP = 68771
local SPELL_CHILLING_WAVE = 68778
local SPELL_DEEP_FREEZE = 70381
local SPELL_FORGE_BLADE = 68774
local SPELL_FORGE_MACE = 68785
local SPELL_FORGE_BLADE_HELPER = 68774
local SPELL_FORGE_BLADE_HELPER_HEROIC = 70334
local EQUIP_ID_SWORD = 49345
local EQUIP_ID_MACE = 49344

local PHASE_ONE = 1
local PHASE_TWO = 2
local PHASE_THREE = 3

-- boss_forgemaster_garfrost.cpp:79-80 forge positions; :66 POINT_FORGE.
local NORTH_FORGE_X = 722.5643
local NORTH_FORGE_Y = -234.1615
local NORTH_FORGE_Z = 527.182
local SOUTH_FORGE_X = 639.257
local SOUTH_FORGE_Y = -210.1198
local SOUTH_FORGE_Z = 529.015
local EFFECT_MOTION_TYPE = 16 -- MotionMaster.h: EFFECT_MOTION_TYPE
local POINT_FORGE = 0

-- instance_pit_of_saron.cpp:29 SlaveLeaderPos; pit_of_saron.h:97-98.
local NPC_MARTIN_VICTUS_1 = 37591
local NPC_GORKUN_IRONSKULL_1 = 37592
local SLAVE_LEADER_X = 689.7158
local SLAVE_LEADER_Y = -104.8736
local SLAVE_LEADER_Z = 513.7360
local ALLIANCE = 0 -- TeamId TEAM_ALLIANCE; player:GetTeam() returns it

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
-- alive player from the threat list (UnitAI.h: SelectTarget draws the
-- threat list; dead targets are offline refs, Go SortedEntries returns
-- them too, so IsAlive filters); no distance filter (0.0f) — the
-- tharon_ja / violet-hold threat-pool fix class (2026-10-25).
local function randomPlayer(creature)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:GetObjectType() == "Player" and t:IsAlive() then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- Cast-state gate (2026-11-04): own UpdateAI pre-loop (:199) +
-- in-loop (:244) UNIT_STATE_CASTING returns — each event-loop arm
-- below defers 500ms via creature:IsCasting()
-- (lua_creature_events.go:1950) without consuming its re-arm (the
-- mal_ganis idiom); the defer precedes target picks, Talk legs and
-- the forge-jump AttackStop. Phase guards keep precedence (a
-- phase-killed chain returns without re-arm before the defer).
-- DamageTaken phase flips are a separate hook and stay ungated.
-- C++ :206-213 EVENT_THROW_SARONITE (all phases): random player,
-- Talk(SAY_THROW_SARONITE), DoCast(target, 68788) non-triggered;
-- re-arm {12500,20000}ms unconditional (outside the target-if).
local function onThrowSaronite(creature, guid)
    local st = garfrostState[guid]
    if not st then
        return
    end
    if creature:IsCasting() then
        schedule(guid, "saronite", 500, function() onThrowSaronite(creature, guid) end)
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
    if creature:IsCasting() then
        schedule(guid, "chilling", 500, function() onChillingWave(creature, guid) end)
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
    if creature:IsCasting() then
        schedule(guid, "deepfreeze", 500, function() onDeepFreeze(creature, guid) end)
        return
    end
    local target = randomPlayer(creature)
    if target then
        creature:Talk(SAY_CAST_DEEP_FREEZE, target)
        creature:CastSpell(target, SPELL_DEEP_FREEZE)
    end
    schedule(guid, "deepfreeze", 35000, function() onDeepFreeze(creature, guid) end)
end

-- C++ :226-232 EVENT_FORGE_JUMP: me->AttackStop() then
-- GetMotionMaster()->MoveJump(north/southForgePos, 25.0f, 15.0f,
-- POINT_FORGE) — the bridged MoveJump (lua_creature_events.go:5143),
-- phase-gated like C++. The landing legs live in
-- garfrostMovementInform (event 28) below, not here: the jump spline
-- stamps EFFECT_MOTION_TYPE and the arrival fires ev28 with
-- (type, id), so the 5s landing -> resume gap is real flight +
-- real delay, not a collapse.
-- Forward declaration: onForgeJump's resume closure references
-- onResumeAttack before its local def below; without this the
-- closure would bind a nil global (festergut/moam nil-call class).
local onResumeAttack
local function onForgeJump(creature, guid)
    local st = garfrostState[guid]
    if not st then
        return
    end
    if creature:IsCasting() then
        schedule(guid, "forgejump", 500, function() onForgeJump(creature, guid) end)
        return
    end
    -- C++ boss_forgemaster_garfrost.cpp:227: me->AttackStop() on
    -- EVENT_FORGE_JUMP, then the phase forge MoveJump (:229/:231).
    creature:AttackStop()
    if st.phase == PHASE_TWO then
        creature:MoveJump(NORTH_FORGE_X, NORTH_FORGE_Y, NORTH_FORGE_Z, 25.0, 15.0, POINT_FORGE)
    elseif st.phase == PHASE_THREE then
        creature:MoveJump(SOUTH_FORGE_X, SOUTH_FORGE_Y, SOUTH_FORGE_Z, 25.0, 15.0, POINT_FORGE)
    end
end

-- C++ :155-172 MovementInform, event 28: EFFECT_MOTION_TYPE /
-- POINT_FORGE landing. Phase is read at LANDING time, like C++
-- (events.IsInPhase at :160/:164). PHASE_TWO DoCast(FORGE_BLADE) +
-- sword; PHASE_THREE RemoveAuras(helper) + DoCast(FORGE_MACE) +
-- mace. The helper is RAID_MODE(68774, 70334): dungeon difficulty
-- 0 = 10MAN_NORMAL and 1 = 25MAN_NORMAL (shared Difficulty enum),
-- so IsHeroic selects it exactly; 0xFFFFFFFF leaves a slot
-- untouched. Then the C++-exact 5000ms MovementInform ->
-- EVENT_RESUME_ATTACK delay (:171).
local function garfrostMovementInform(event, creature, moveType, id)
    local guid = creature:GetGUIDLow()
    local st = garfrostState[guid]
    if not st then
        return
    end
    if moveType ~= EFFECT_MOTION_TYPE or id ~= POINT_FORGE then
        return
    end
    if st.phase == PHASE_TWO then
        creature:CastSpell(creature, SPELL_FORGE_BLADE)
        creature:SetEquipmentSlots(EQUIP_ID_SWORD, 0xFFFFFFFF, 0xFFFFFFFF)
    elseif st.phase == PHASE_THREE then
        if creature:IsHeroic() then
            creature:RemoveAura(SPELL_FORGE_BLADE_HELPER_HEROIC)
        else
            creature:RemoveAura(SPELL_FORGE_BLADE_HELPER)
        end
        creature:CastSpell(creature, SPELL_FORGE_MACE)
        creature:SetEquipmentSlots(EQUIP_ID_MACE, 0xFFFFFFFF, 0xFFFFFFFF)
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
    if creature:IsCasting() then
        schedule(guid, "resume", 500, function() onResumeAttack(creature, guid) end)
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
-- non-triggered self-cast DoCast(me, SPELL_PERMAFROST) (:111);
-- CallForHelp(70.0f) (:112, bridged — lua_creature_events.go:1781);
-- the EVENT_THROW_SARONITE seed at 7000ms (:113).
local function garfrostEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    initState(guid)
    -- BossAI::JustEngagedWith (:107-115, base-first) — SetBossState
    -- (DATA_GARFROST 0, IN_PROGRESS) via creature:SetBossState, then
    -- SetCombatPulseDelay(5) + me-form DoZoneInCombat
    -- (ScriptedCreature.cpp:540/542).
    creature:SetBossState(DATA_GARFROST, 1)
    creature:SetCombatPulseDelay(5)
    creature:DoZoneInCombat()
    creature:Talk(SAY_AGGRO)
    creature:CastSpell(creature, SPELL_PERMAFROST)
    creature:CallForHelp(70.0)
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
    local guid = creature:GetGUIDLow()
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

-- C++ :122-130 JustDied: _JustDied() first — SetBossState
-- (DATA_GARFROST 0, DONE) via creature:SetBossState — then the
-- instance SetBossState-override DONE-spawn leg
-- (instance_pit_of_saron.cpp:157-172, fired inside SetBossState in
-- C++): Martin Victus 1 (37591, Alliance) / Gorkun Ironskull 1
-- (37592, Horde) at SlaveLeaderPos, TEMPSUMMON_MANUAL_DESPAWN (7) —
-- team read as instanceData.teamInInstance or 0 (the C++ ctor
-- default); the instanceData.garfrost nil-guard mirrors the C++
-- instance->GetCreature(_garfrostGUID) check. Then Talk(SAY_DEATH)
-- (:125) unconditional. The tyrannus->AI()->Talk
-- (SAY_TYRANNUS_DEATH 0) leg (:128-129) is PORTED via the instance
-- GetGuidData mirror (instance_pit_of_saron.lua binds tyrannus;
-- kelthuzad GetInstanceData precedent), nil-guarded like the C++
-- GetCreature check. RemoveAllGameObjects() stays out (Go engine
-- owns object cleanup).
local function garfrostJustDied(event, creature)
    clearState(creature:GetGUIDLow())
    creature:SetBossState(DATA_GARFROST, 3)
    local mapObj = creature:GetMap()
    local data = nil
    if mapObj then
        data = mapObj:GetInstanceData()
    end
    if data and data.garfrost then
        local team = data.teamInInstance or ALLIANCE
        local entry = NPC_GORKUN_IRONSKULL_1
        if team == ALLIANCE then
            entry = NPC_MARTIN_VICTUS_1
        end
        creature:SummonCreature(entry, SLAVE_LEADER_X, SLAVE_LEADER_Y, SLAVE_LEADER_Z, 0.0, 7, 0)
    end
    creature:Talk(SAY_DEATH)
    if data and data.tyrannus then
        data.tyrannus:Talk(SAY_TYRANNUS_DEATH)
    end
end

local function garfrostLeaveCombat(event, creature)
    clearState(creature:GetGUIDLow())
end

-- C++ Reset (:99-105): _Reset() first — SetBossState(DATA_GARFROST 0,
-- NOT_STARTED) only when the stored state != DONE (the bronjahm /
-- cyanigosa precedent: ev23 fires on spawn too, and a DONE boss must
-- stay DONE) — then SetPhase(PHASE_ONE) / SetEquipmentSlots(true) /
-- Initialize (phase init + template-equipment legs stay out, see
-- header).
local function garfrostReset(event, creature)
    if creature:GetBossState(DATA_GARFROST) ~= 3 then
        creature:SetBossState(DATA_GARFROST, 0)
    end
    clearState(creature:GetGUIDLow())
    -- Alive-gated: ev23 fires on death too (fireCreatureDied runs
    -- Reset before the kill-credit criteria read, kill.go:1091/:1400),
    -- and a death-time zero would clobber the fight's peak before
    -- the achievement gate reads it; the wipe re-arm is preserved.
    if creature:IsAlive() then
        creature:SetData(ACHIEV_DOESNT_GO_TO_ELEVEN, 0) -- C++ Reset -> Initialize :96
    end
end

RegisterCreatureEvent(ENTRY_GARFROST, 1, garfrostEnterCombat)
RegisterCreatureEvent(ENTRY_GARFROST, 2, garfrostLeaveCombat)
RegisterCreatureEvent(ENTRY_GARFROST, 3, garfrostKilledUnit)
RegisterCreatureEvent(ENTRY_GARFROST, 4, garfrostJustDied)
RegisterCreatureEvent(ENTRY_GARFROST, 28, garfrostMovementInform)
-- C++ SpellHitTarget (boss_forgemaster_garfrost.cpp:174-186): when the
-- Permafrost helper aura hits a unit, _permafrostStack = max(stack of
-- that aura on the target) — the achievement_doesnt_go_to_eleven latch
-- (Go case in engine/world/achievement_criteria_scripts.go). Dormant
-- until the DBC aura trigger (70326 -> helper) runs: without Spell.dbc
-- the helper hits never occur and the slot stays 0.
local function garfrostSpellHitTarget(event, creature, target, spellId)
    local helper = SPELL_PERMAFROST_HELPER_10
    if creature:IsHeroic() then
        helper = SPELL_PERMAFROST_HELPER_25
    end
    if spellId ~= helper or not target then
        return
    end
    local aura = target:GetAura(helper)
    if not aura then
        return
    end
    local stack = aura:GetStackAmount()
    if stack > creature:GetData(ACHIEV_DOESNT_GO_TO_ELEVEN) then
        creature:SetData(ACHIEV_DOESNT_GO_TO_ELEVEN, stack)
    end
end

RegisterCreatureEvent(ENTRY_GARFROST, 15, garfrostSpellHitTarget)
RegisterCreatureEvent(ENTRY_GARFROST, 9, garfrostDamageTaken)
RegisterCreatureEvent(ENTRY_GARFROST, 23, garfrostReset)

-- C++ spell_garfrost_permafrost (boss_forgemaster_garfrost.cpp:261-333):
-- BeforeHit (miss-inclusive; returns unless missInfo == SPELL_MISS_NONE
-- :277-279): for each hit unit, gather saronite rocks (GO_SARONITE_ROCK
-- 196485, pit_of_saron.h:100) within 100.0f; if a rock IsInBetween(caster,
-- target, 4.0f), the target gains spell-immunity to 70326 for the cast
-- and PreventHitDefaultEffect(0/1/2) fires (:294-301). AfterHit: the
-- immunity is removed (:308-315). DORMANT until Spell.dbc ships 70326.
-- Gaps vs C++ (documented): PreventHitDamage() has no Go model (likely
-- vacuous — 70326's effect list is DBC-bound); PreventHitAura() + the
-- per-cast `prevented` quirk are not reproduced — the flag has no
-- Go-observable consumer, its aura-suppression job rides
-- PreventHitDefaultEffect via the ApplyAura arm (boss_ai.go:9255);
-- IsInvisibleDueToDespawn has no Lua method, the grid list is trusted;
-- PreventHitDefaultEffect collapses to per-effect in Go, so a blocked
-- target suppresses the effect for every hit target of that effect.
local GO_SARONITE_ROCK = 196485

-- WorldObject::IsInBetween (Object.cpp:1395-1410), 2D: the rock must lie
-- strictly inside the caster->target segment (dist < segment length)
-- and within size of the point on the caster->target ray at the rock's
-- distance from the caster.
local function rockInBetween(rock, caster, target, size)
    local rx, ry = rock:GetX(), rock:GetY()
    local cx, cy = caster:GetX(), caster:GetY()
    local tx, ty = target:GetX(), target:GetY()
    local dx, dy = rx - cx, ry - cy
    local distSq = dx * dx + dy * dy
    local segX, segY = tx - cx, ty - cy
    local segSq = segX * segX + segY * segY
    if distSq >= segSq then
        return false
    end
    local dist = math.sqrt(distSq)
    local angle = math.atan2(segY, segX)
    local px, py = cx + math.cos(angle) * dist, cy + math.sin(angle) * dist
    local pdx, pdy = rx - px, ry - py
    return size * size >= pdx * pdx + pdy * pdy
end

local function permafrostBeforeHit(event, spell, target, missInfo)
    if missInfo ~= 0 or target == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local rocks = caster:GetGameObjectsInGrid(GO_SARONITE_ROCK, 100.0)
    for _, rock in ipairs(rocks) do
        if rockInBetween(rock, caster, target, 4.0) then
            target:ApplySpellImmune(SPELL_PERMAFROST, 6, SPELL_PERMAFROST, true)
            spell:PreventHitDefaultEffect(0)
            spell:PreventHitDefaultEffect(1)
            spell:PreventHitDefaultEffect(2)
            return
        end
    end
end

local function permafrostAfterHit(event, spell, target)
    if target == nil then
        return
    end
    target:ApplySpellImmune(SPELL_PERMAFROST, 6, SPELL_PERMAFROST, false)
end

RegisterSpellEvent(SPELL_PERMAFROST, 6, permafrostBeforeHit)
RegisterSpellEvent(SPELL_PERMAFROST, 3, permafrostAfterHit)
