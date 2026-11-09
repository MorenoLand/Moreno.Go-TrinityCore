-- Blood Prince Council (Icecrown Citadel) — Lua port of
-- src/server/scripts/Northrend/IcecrownCitadel/boss_blood_prince_council.cpp
-- (1369 lines incl. license; 8 CreatureScripts
-- (boss_blood_council_controller (BossAI);
-- boss_prince_keleseth_icc / boss_prince_taldaram_icc /
-- boss_prince_valanar_icc (BloodPrincesBossAI : BossAI);
-- npc_blood_queen_lana_thel (PassiveAI); npc_ball_of_flame
-- (ScriptedAI); npc_kinetic_bomb (ScriptedAI); npc_dark_nucleus
-- (ScriptedAI)) + 6 SpellScripts + 5 AuraScripts + 1
-- AreaTriggerScript (at_blood_prince_council_start_intro); all
-- registered from inside AddSC_boss_blood_prince_council();
-- loader decl 178 / call 373 per northrend_script_loader.cpp — the
-- EIGHTH group of the "// Icecrown Citadel" block in
-- AddNorthrendScripts(), immediately after
-- AddSC_boss_professor_putricide() (call 372); the call after it is
-- AddSC_boss_blood_queen_lana_thel() (call 374) — verified from the
-- loader during the Oct-13 Talk-only port.
-- Entries: 37972 Prince Keleseth (icecrown_citadel.h NPC_PRINCE_KELESETH,
-- line 245), 37973 Prince Taldaram (NPC_PRINCE_TALDARAM, line 246),
-- 37970 Prince Valanar (NPC_PRINCE_VALANAR, line 247);
-- instance_icecrown_citadel.cpp OnCreatureCreate binds all three
-- (case NPC_PRINCE_KELESETH line 255; DATA_PRINCE_* cases line 789) —
-- entry-verifiable; Go registration already exists
-- (engine/world/boss_ai.go, eighth ICC group):
-- RegisterLuaBoss("boss_prince_keleseth_icc", 37972),
-- RegisterLuaBoss("boss_prince_taldaram_icc", 37973),
-- RegisterLuaBoss("boss_prince_valanar_icc", 37970) — no new
-- registration needed. The CreatureScript ScriptName bindings are
-- DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_blood_prince_council" hits
-- boss_blood_prince_council.cpp (+ the loader decl/call lines) only;
-- this clone carries no sql/ tree, so ScriptName bindings are
-- DB-side by construction.
--
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnKill,
-- 4 OnDied, 23 OnReset. Timers via CreateLuaEvent ({min,max} table =
-- randtime range), GUID-keyed, cancelled on events 2/4/23; melee is
-- engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
--
-- DOCTRINE RETRACTION (2026-10-13): the earlier port of this file
-- ported Talk arms only under the "no-timer-bridge" doctrine. That
-- doctrine is disproved and stale — CreateLuaEvent is a real registered
-- bridge (engine/scripting/runtime.go:798), and timer-driven cast arms
-- port on the maiden_of_virtue / moroes / curator / nightbane convention
-- (see boss_falric.lua for the exact idiom): timers table keyed by
-- creature GUID, cancelled on Eluna events 2 (leave combat), 4 (death),
-- 23 (reset); CastSpell(nil, spellId) = victim-cast (bridge falls back
-- to TargetGUID); CastSpell(creature, spellId) = self-cast (the
-- DoCastAOE convention, boss_ai.go DoCastAOE); {min,max} delay =
-- randtime range; TRIGGERED casts pass true as the third CastSpell arg.
-- None of the ported arms here is a TRIGGERED cast (every ported C++
-- DoCast is 1-2 arg, non-triggered). _isEmpowered state is NOT modeled:
-- it flips only via the controller's unbridged invocation rotation
-- (SpellHit -> ACTION_CAST_INVOCATION and the controller's cross-AI
-- ACTION_REMOVE_INVOCATION DoAction), so every empowered-variant arm
-- stays out (the gothik precedent).
--
-- PORTED ARMS (C++-exact for all modeled arms):
-- BloodPrincesBossAI::JustEngagedWith (:455) — the scheduler seeds
-- plus the SetCombatPulseDelay(5) arm (PORTED — creature:SetCombatPulseDelay
-- bridge, lua_creature_events.go; event 1, C++ arm order)
-- and the Reset's SetCombatPulseDelay(0) + SetImmuneToPC(false) + SetHealth(1) (:447/:449/:451)
-- arms (PORTED — event 23, C++ arm order); the setActive leg has no bridge
-- (CORRECTION 2026-10-23: the DoZoneInCombat(controller) :459-460 leg IS
-- ported in each member's event-1 handler — controller gathered via
-- GetCreaturesInGrid, illidari-council precedent):
-- keleseth: EVENT_BERSERK 600000ms (:622), EVENT_SHADOW_RESONANCE
-- {10000,15000}ms (:623), EVENT_SHADOW_LANCE 2000ms (:624);
-- taldaram: EVENT_BERSERK 600000ms (:689), EVENT_GLITTERING_SPARKS
-- {12000,15000}ms (:690), EVENT_CONJURE_FLAME 20000ms (:691);
-- valanar: EVENT_BERSERK 600000ms (:763) + the valanar-override
-- targetAuraCheck scheduler (:1050-1068, 1000ms seed/re-arm).
-- keleseth EVENT_BERSERK (:658-661) — DoCastSelf(SPELL_BERSERK 50334)
-- non-triggered + Talk(EMOTE_KELESETH_BERSERK 4), cast before Talk
-- (C++ order), one-shot.
-- keleseth EVENT_SHADOW_RESONANCE (:662-666) —
-- Talk(SAY_KELESETH_SPECIAL 2) BEFORE
-- DoCastSelf(SPELL_SHADOW_RESONANCE 71943) (C++ order), non-triggered;
-- re-arm {10000,15000}ms.
-- keleseth EVENT_SHADOW_LANCE (:667-670) —
-- DoCastVictim(SPELL_SHADOW_LANCE 71405) = victim-cast; re-arm 2000ms.
-- taldaram EVENT_BERSERK (:724-727) — DoCastSelf(SPELL_BERSERK 50334) +
-- Talk(EMOTE_TALDARAM_BERSERK 5), cast before Talk (C++ order),
-- one-shot.
-- taldaram EVENT_GLITTERING_SPARKS (:728-731) —
-- DoCastVictim(SPELL_GLITTERING_SPARKS 71806) = victim-cast;
-- re-arm {15000,50000}ms.
-- taldaram EVENT_CONJURE_FLAME (:732-744) —
-- DoCastSelf(SPELL_CONJURE_FLAME 71718) non-triggered +
-- Talk(SAY_TALDARAM_SPECIAL 2) AFTER the cast (C++ order);
-- re-arm {20000,30000}ms.
-- valanar EVENT_BERSERK (:814-817) — DoCastSelf(SPELL_BERSERK 50334) +
-- Talk(SAY_VALANAR_BERSERK 5), cast before Talk (C++ order), one-shot.
-- valanar JustSummoned NPC_KINETIC_BOMB (:778-785) — MovePoint to
-- the ground-Z + RemoveFlag(59, UNIT_FLAG_NOT_SELECTABLE)
-- immediately after the MovePoint call (not arrival-gated) — PORTED
-- this tick (MoveTo bridge + map:GetHeight).
-- BloodPrincesBossAI::KilledUnit (:505-521) — Talk(kill text)
-- victim->GetTypeId() == TYPEID_PLAYER gated, entry-switched (event 3,
-- the razuvious player-gated variant precedent): keleseth
-- SAY_KELESETH_KILL 3, taldaram SAY_TALDARAM_KILL 4, valanar
-- SAY_VALANAR_KILL 4.
-- BloodPrincesBossAI::JustDied (:466-486) — Talk(death text)
-- entry-switched (event 4, the sjonnir JustDied-Talk precedent):
-- keleseth SAY_KELESETH_DEATH 5, taldaram EMOTE_TALDARAM_DEATH 6,
-- valanar SAY_VALANAR_DEATH 6.
--
-- DOCUMENTED-ONLY (except the controller, registered + ported tick 221):
-- boss_blood_council_controller — zero own Talk lines; its
-- JustEngagedWith frame legs (:266-302, SetCombatPulseDelay /
-- DoZoneInCombat / SetBossState IN_PROGRESS / DoCastSelf 70952 /
-- PrincesData ENGAGE loop) and EnterEvadeMode DISENGAGE loop (:252-260)
-- are PORTED via the instance guid store; the CheckRequiredBosses
-- sequence-break leg, the roll_chance_i(50) _invocationOrder build +
-- EVENT_INVOCATION_OF_BLOOD seed, and the START_INTRO leg stay out
-- (instance GetData(DATA_BLOOD_PRINCE_COUNCIL_INTRO 43) gate — no
-- instance GetData bridge; creature:GetData is the per-motion latch,
-- not instance data — plus the unregistered queen relay). The
-- INTRO_DONE relay is PORTED tick 558 (controllerDoAction, ev38).
-- The Reset SummonCreatureGroup leg is PORTED
-- below (controllerReset, event 23). UpdateAI EVENT_INVOCATION_OF_BLOOD
-- (:373-398) cross-AI DoAction + Talk(textId) on the rotated prince is
-- the invocation rotation machine, driven by the unregistered council
-- controller (:352/:382/:540 — dispatch IS bridged, ev38);
-- boss_blood_council_
-- controller::JustDied (:322-336, prince kill sweep — the prince->
-- LowerPlayerDamageReq(:331) + Unit::Kill legs, SetBossState(DONE)
-- :323) stays out — controller death handling unported; re-verified
-- 2026-11-06: no script anywhere in-tree kills the controller, so
-- the DONE leg has no live fire path (the princes' own JustDied
-- :466-486 calls no _JustDied, and the instance OnUnitDeath has no
-- prince case).
-- BloodPrincesBossAI::DoAction (:549-580) — Talk(SelectInvocationSay())
-- + DoCastSelf(SPELL_INVOCATION_VISUAL_ACTIVE 71596, true) ride
-- ACTION_CAST_INVOCATION, which arrives via SpellHit (:535-539) from the
-- controller's unbridged invocation cast (no bridge); the
-- ACTION_REMOVE_INVOCATION health/aura reset rides the controller's
-- cross-AI DoAction (dispatch IS bridged, ev38 — the controller
-- itself is unregistered). ACTION_STAND_UP (:552-563) is PORTED tick
-- 558 as a receiver (princesDoAction, ev38; sender is the controller
-- INTRO_DONE relay above) — aura/flag strips + 1s StandUpEvent
-- (PerformEmote ROAR + SetReactState AGGRESSIVE, :177-189). It is a
-- no-op until the feign-death intro machine lands (JustAppeared
-- :488): nothing ever sets the princes passive in Lua today.
-- BloodPrincesBossAI::JustAppeared (:488) — instance DATA_INTRO gate
-- (feign-death 71598 self-cast while the intro is not done) + SetHealth
-- (_spawnHealth) has no instance bridge; NOT registered (the intro
-- machine itself is unbridged — bronjahm npc_corrupted_soul_fragment
-- precedent).
-- BloodPrincesBossAI::DamageTaken (:494-501) — non-empowered damage
-- nullification (damage = 0 unless _isEmpowered) — PORTED via ev9
-- (princesDamageTaken, numeric-return rewrite, halazzi precedent).
-- keleseth "// does not melee" (:676) — melee is engine-driven in Go
-- (deviation, documented below).
-- keleseth EVENT_SHADOW_LANCE empowered variant
-- DoCastVictim(SPELL_EMPOWERED_SHADOW_LANCE 71815) — _isEmpowered-gated
-- (modeled as base variant 71405 always; deviation, documented below).
-- taldaram EVENT_CONJURE_FLAME empowered variant
-- DoCastSelf(SPELL_CONJURE_EMPOWERED_FLAME 72040) re-arm
-- {15000,25000}ms (:735-737) — _isEmpowered-gated, out (modeled as
-- base 71718 with the base re-arm {20000,30000}ms).
-- valanar EVENT_KINETIC_BOMB (:818-824) — SelectTarget(Random, 1, 0.0f,
-- true) -> DoCast(target, SPELL_KINETIC_BOMB_TARGET 72053) (:822,
-- 2-arg = non-triggered) + Talk(SAY_VALANAR_SPECIAL 2) (:822, Talk
-- AFTER the cast, C++ order); re-arm {18000,24000}ms unconditional
-- (:823, the Repeat sits outside the target-pick if); seed
-- {18000,24000}ms (:763). NOT _isEmpowered-gated — PORTED this tick
-- (kineticBombTick, below). Pool caveat: C++ offset 1 skips the
-- MaxThreat-ordered head (UnitAI.h:172-174, i.e. the tank); the port
-- uses the alive same-map/instance player pool excluding the victim
-- (the netherspite precedent; only threat-listed units qualify in
-- practice).
-- spell_valanar_kinetic_bomb_aura HandleDummyTick (:1230-1244,
-- EFFECT_1, aura 72080) — PORTED tick 189 (kineticBombAuraTick,
-- RegisterAuraEvent 72080 / 1): live bomb within 1.0f -> bomb
-- self-casts explosion 72052 triggered, drops visual 72054, tick
-- aura removes itself. DoAction(72052) bomb-despawn leg PORTED tick
-- 232 (kineticBombDoAction, ev38; bomb registered in boss_ai.go) —
-- the 1s EVENT_BOMB_DESPAWN leg (:1018-1022) fires from the ported
-- aura tick; see the handler comment.
-- spell_blood_council_shadow_prison HandleDummyTick (:1300-1304,
-- EFFECT_0, aura 73001) — PORTED tick 366 (shadowPrisonTick,
-- RegisterAuraEvent 73001 / 1): moving target casts 72999 on self.
-- Bridge: creature:IsMoving (lua_creature_events.go, Unit.h:1641
-- twin — the tick-189 "IsMoving exists only on the player surface"
-- blocker is closed). 72999's AddExtraDamage SpellScript leg
-- (:1316-1324) stays out — no SpellScript bridge.
-- valanar EVENT_SHOCK_VORTEX (:826-838) — the EMPOWERED leg
-- DoCastSelf(SPELL_EMPOWERED_SHOCK_VORTEX 72039) +
-- Talk(EMOTE_VALANAR_SHOCK_VORTEX 3) + Repeat(30s) is
-- _isEmpowered-gated — stays OUT; the BASE leg DoCast(target,
-- SPELL_SHOCK_VORTEX 72037) (:834, 2-arg = non-triggered, no Talk)
-- on the same random pick — re-arm {18000,23000}ms unconditional
-- (:835); seed {15000,20000}ms (:764) — PORTED this tick
-- (shockVortexTick, below) with the same tank-excluding pool.
-- keleseth/taldaram/valanar JustSummoned (:634-642, :697-710,
-- :770-789) — PARTLY PORTED via the ev19 handlers: keleseth
-- teleport (open-space exact), taldaram random-target pick +
-- emote + SetGUID store (ball chase DoAction PORTED 2026-11-09:
-- aura-remove -> ev38 chase via ballChaseTarget; base ball 38332
-- takes the SetGUID store with no emote), valanar
-- bomb-target react/cast + vortex dummy/periodic casts (bomb
-- MovePoint ground-Z PORTED this tick; DoZoneInCombat(summon) out).
-- npc_blood_queen_lana_thel — Talk(SAY_INTRO_1 0) rides
-- ACTION_START_INTRO (:866-880, sender is the unported controller /
-- blood-queen intro machine — dispatch IS bridged, ev38);
-- Talk(SAY_INTRO_2 1) rides EVENT_INTRO_1 (:896-905, intro scheduler
-- leg, no-timer-bridge) — NOT registered.
-- npc_ball_of_flame — PORTED 2026-11-09 (registered in boss_ai.go):
-- Reset one-shot aura casts (:919-924) via ev23; the
-- flame-ball-visual AuraScript OnRemove (:1148-1160) via
-- RegisterAuraEvent kind 3 (55891 -> triggered 71714 + DoAction
-- chase; 55947 -> despawn); the DoAction chase (:949-957) via ev38
-- (ClearUnitState + Attack + MoveChase). MovementInform arrival
-- (:926-939, periodic-strip + FLAMES + 2s death-effect cast) stays
-- OUT — no chase finalize bridge — so the ball chases but never
-- detonates; DamageDealt (:965-975, orb-whisperer achievement
-- flag) stays out — no DamageDealt hook in Go (the criteria case
-- reads the shared instanceData key; only the Reset set leg is
-- ported, so the key can over-grant).
-- npc_kinetic_bomb / npc_dark_nucleus — zero Talk lines; Reset
-- one-shot aura casts (:987-988, :1047) are aura/visual setup, not
-- timer arms; the bomb's explode trigger now rides the 72080 aura
-- tick (ported tick 189) and its despawn leg rides the same tick's
-- DoAction(72052) -> ev38 handler (PORTED tick 232; bomb registered
-- in boss_ai.go). The chase leg (ACTION_KINETIC_BOMB_JUMP +
-- CONTINUE_FALLING, MoveCharge) PORTED this tick (kineticBombDoAction
-- below); its sender stays out (knockback SpellScript 72087
-- BeforeHit, no SpellScript bridge). (Their JustSummoned legs port
-- via the boss ev19 handlers above, not via AI registration.)
-- BloodPrincesBossAI::JustReachedHome (:525-534) — PORTED 2026-11-09
-- via ev24 (DISENGAGE frame + SetHealth(1) + empowered clear +
-- NON_ATTACKABLE set + evade-counter DoAction to the controller;
-- the counter evades the controller at 3, :306-310). The 0x2 clear
-- rides controllerReset (the :239-250 intro-loop twin), NOT
-- princesReset — ev23 fires before ev24 in the wipe sequence.
-- UpdateAI UNIT_STATE_CASTING gates — PORTED 2026-11-04 (creature:IsCasting
-- 500ms defer, mal_ganis idiom; the old "no live unit-state read bridge /
-- timers fire unconditionally" claim is retracted): controller :366/:401
-- (covers only the unported EVENT_INVOCATION_OF_BLOOD), keleseth :651/:675,
-- taldaram :717/:749, valanar :807/:844. Controller/intro UpdateAIs
-- (:890/:968/:1011) and the nucleus UpdateAI (:1082, TaskScheduler only)
-- carry no CASTING gate — the nucleus aura-check tick stays ungated.
-- Heroic-gated engage casts: keleseth :628-629
-- DoCastSelf(SPELL_SHADOW_PRISON 72998, true) +
-- DoCastSelf(SPELL_SHADOW_PRISON_DUMMY 73001); taldaram :693 and
-- valanar :767 DoCastSelf(SPELL_SHADOW_PRISON 72998, true) —
-- creature:IsHeroic() bridges ScriptedAI::_isHeroic (per-instance
-- difficulty from the instance table row; the rotface heroic-gated
-- precedent now has a bridge).
-- The 6 spell scripts join the no-SpellScript-bridge queue, the 5 aura
-- scripts the no-AuraScript-bridge queue (except the kinetic-bomb
-- aura's periodic tick, ported tick 189 via RegisterAuraEvent, and
-- the shadow-prison aura's periodic tick, ported tick 366 via
-- RegisterAuraEvent + the new creature:IsMoving bridge), and
-- at_blood_prince_council_start_intro the no-AreaTrigger bridge queue
-- (the boss_anubrekhan at_anubrekhan_entrance precedent).
--
-- DEVIATIONS FROM C++ (documented; every modeled value C++-exact):
-- _isEmpowered flips on the invocation rotation (46.5s, ported this
-- tick): keleseth shadow lance casts empowered 71815 while empowered,
-- taldaram conjure flame casts empowered 72040 with the {15000,25000}ms
-- re-arm, valanar shock vortex casts empowered 72039 + Talk(3) with the
-- 30s repeat; DamageTaken nullifies non-empowered damage (ev9).
-- keleseth's C++ "// does not melee" is unmodeled — Go melees.

local ENTRY_KELESETH = 37972
local ENTRY_TALDARAM = 37973
local ENTRY_VALANAR = 37970
local ENTRY_KINETIC_BOMB = 38454 -- icecrown_citadel.h:254 (NPC_KINETIC_BOMB)
local ENTRY_KINETIC_BOMB_TARGET = 38458 -- icecrown_citadel.h:253 (NPC_KINETIC_BOMB_TARGET)
local ENTRY_SHOCK_VORTEX = 38422 -- icecrown_citadel.h:255 (NPC_SHOCK_VORTEX)
local ENTRY_BALL_OF_INFERNO_FLAME = 38451 -- icecrown_citadel.h:251 (NPC_BALL_OF_INFERNO_FLAME)
local ENTRY_BALL_OF_FLAME = 38332 -- icecrown_citadel.h:251 (NPC_BALL_OF_FLAME)
local SPELL_FLAME_SPHERE_SPAWN_EFFECT = 55891 -- boss_blood_prince_council.cpp:94
local SPELL_BALL_OF_FLAMES_VISUAL = 71706 -- boss_blood_prince_council.cpp:95
local SPELL_BALL_OF_FLAMES = 71714 -- boss_blood_prince_council.cpp:96
local SPELL_BALL_OF_FLAMES_PROC = 71756 -- boss_blood_prince_council.cpp:101
local SPELL_BALL_OF_FLAMES_PERIODIC = 71709 -- boss_blood_prince_council.cpp:102
local SPELL_FLAME_SPHERE_DEATH_EFFECT = 55947 -- boss_blood_prince_council.cpp:98
local SPELL_REMOVE_EMPOWERED_BLOOD = 72131 -- boss_blood_prince_council.cpp:110
local ACTION_FLAME_BALL_CHASE = 7 -- boss_blood_prince_council.cpp:159
local ACTION_PRINCE_EVADE = 100 -- Lua-only carrier (see princesReachHome)
local DATA_BLOOD_PRINCES_CONTROL = 22 -- icecrown_citadel.h:98
local UNIT_STATE_CASTING = 0x8000 -- Unit.h:226
local UNIT_STATE_STUNNED = 0x08 -- Unit.h:214
local SPELL_KINETIC_BOMB = 72080 -- boss_blood_prince_council.cpp:107
local SPELL_KINETIC_BOMB_EXPLOSION = 72052 -- boss_blood_prince_council.cpp:115
local SPELL_KINETIC_BOMB_VISUAL = 72054 -- boss_blood_prince_council.cpp:114
local SPELL_UNSTABLE = 72059 -- boss_blood_prince_council.cpp:113
local SPELL_KINETIC_BOMB_KNOCKBACK = 72087 -- boss_blood_prince_council.cpp:116
local SPELL_SHOCK_VORTEX_DUMMY = 72633 -- boss_blood_prince_council.cpp:120
local SPELL_SHOCK_VORTEX_PERIODIC = 71945 -- boss_blood_prince_council.cpp:119
local EMOTE_TALDARAM_FLAME = 3 -- boss_blood_prince_council.cpp:51
local REACT_PASSIVE = 0

-- C++ BloodPrincesBossAI::JustDied (:466-469) + Reset (:443-446):
-- summons.DespawnAll(); Valanar's JustSummoned (:770-800) latches
-- every summon (bomb targets, bombs, shock vortices) via the
-- trailing summons.Summon, so the ev19 handler below tracks them
-- (ionar idiom) and the died/reset/leave handlers despawn them.
-- Keleseth's JustSummoned (:634-642) latches every summon (dark
-- nuclei from SHADOW_RESONANCE) and Taldaram's (:697-710) latches
-- every summon (inferno balls from CONJURE_FLAME), so their ev19
-- handlers track them the same way and the shared died/reset/
-- leave handlers despawn them.
local valanarSummons = {}

local vortexTimers = {}

local function cancelVortexTimer(vortexGuid)
    local id = vortexTimers[vortexGuid]
    if id then
        RemoveEventById(id)
        vortexTimers[vortexGuid] = nil
    end
end

local function despawnValanarSummons(guid)
    local list = valanarSummons[guid]
    if list then
        for summonGuid, summon in pairs(list) do
            cancelVortexTimer(summonGuid)
            summon:Despawn()
        end
        valanarSummons[guid] = nil
    end
end

local kelesethSummons = {}

local function despawnKelesethSummons(guid)
    local list = kelesethSummons[guid]
    if list then
        for _, summon in pairs(list) do
            summon:Despawn()
        end
        kelesethSummons[guid] = nil
    end
end

local taldaramSummons = {}

-- C++ ICC taldaram JustSummoned SetGUID leg (:697-710): the ball AI
-- stores the target for its DoAction chase (aura-removal leg,
-- unbridged) — the stored target ports here; the Talk emote is the
-- observable leg. Cleared with the despawn list.
local ballTargets = {}

-- Ball-GUID-keyed chase target for the ball's own ev38 DoAction
-- receiver (the taldaram ev19 handler stores the same target in
-- ballTargets[taldaramGuid] for the despawn sweep; this flat map is
-- what the ball reads when its aura-remove leg fires DoAction).
local ballChaseTarget = {}

local function despawnTaldaramSummons(guid)
    local list = taldaramSummons[guid]
    if list then
        for summonGuid, summon in pairs(list) do
            ballChaseTarget[summonGuid] = nil
            summon:Despawn()
        end
        ballTargets[guid] = nil
        taldaramSummons[guid] = nil
    end
end

-- C++ npc_ball_of_flame::Reset (:919-924): triggered self-casts of
-- the spawn effect + visual; inferno flames additionally cast the
-- proc + periodic auras triggered. ev23 fires on first spawn too
-- (fireCreatureSpawned, creaturemotion.go:1240-1262), matching the
-- C++ spawn-Reset.
local function ballOfFlameReset(event, creature)
    creature:CastSpell(creature, SPELL_FLAME_SPHERE_SPAWN_EFFECT, true)
    creature:CastSpell(creature, SPELL_BALL_OF_FLAMES_VISUAL, true)
    if creature:GetEntry() == ENTRY_BALL_OF_INFERNO_FLAME then
        creature:CastSpell(creature, SPELL_BALL_OF_FLAMES_PROC, true)
        creature:CastSpell(creature, SPELL_BALL_OF_FLAMES_PERIODIC, true)
    end
end

-- C++ spell_taldaram_flame_ball_visual::OnRemove (:1148-1160,
-- AfterEffectRemove, SPELL_AURA_MOD_STUN, AURA_EFFECT_HANDLE_REAL):
-- removal of the spawn effect casts Ball of Flames (71714) on the
-- ball triggered and sends ACTION_FLAME_BALL_CHASE; removal of the
-- death effect despawns the ball. Ported via RegisterAuraEvent kind
-- 3 (OnRemove, lua_aura.go fireAuraRemove — the AfterEffectRemove
-- legs run in the same single fire). The Load() entry gate
-- (38332/38451) is carried in the handler. The 55947 leg is dormant:
-- its only caster is the unported MovementInform arrival leg.
local function flameBallVisualRemove(spellId, event, aura, target)
    if target:GetEntry() ~= ENTRY_BALL_OF_FLAME
        and target:GetEntry() ~= ENTRY_BALL_OF_INFERNO_FLAME then
        return
    end
    if spellId == SPELL_FLAME_SPHERE_SPAWN_EFFECT then
        target:CastSpell(target, SPELL_BALL_OF_FLAMES, true)
        target:DoAction(ACTION_FLAME_BALL_CHASE)
    else
        target:DespawnOrUnsummon()
    end
end

-- C++ npc_ball_of_flame::DoAction ACTION_FLAME_BALL_CHASE (:949-957):
-- clear CASTING|STUNNED, Attack(target, true), MoveChase(target,
-- 1.0f). The chase target is the taldaram ev19 SetGUID store
-- (ballChaseTarget). MovementInform arrival (:926-939, remove
-- periodic + FLAMES + 2s death-effect cast) stays out — no chase
-- finalize bridge — so the ball chases but never detonates.
local function ballOfFlameDoAction(event, creature, action)
    if action ~= ACTION_FLAME_BALL_CHASE then
        return
    end
    local target = ballChaseTarget[creature:GetGUIDLow()]
    if target == nil then
        return
    end
    creature:ClearUnitState(UNIT_STATE_CASTING + UNIT_STATE_STUNNED)
    if creature:Attack(target, true) then
        creature:MoveChase(target, 1.0)
    end
end
local ENTRY_DARK_NUCLEUS = 38369 -- icecrown_citadel.h:250 (NPC_DARK_NUCLEUS)
local ENTRY_BLOOD_ORB_CONTROLLER = 38008 -- icecrown_citadel.h:248 (NPC_BLOOD_ORB_CONTROLLER)
local DATA_BLOOD_PRINCE_COUNCIL = 7 -- icecrown_citadel.h:81
local BOSS_STATE_NOT_STARTED = 0 -- InstanceScript.h:68-76
local BOSS_STATE_FAIL = 2
local BOSS_STATE_DONE = 3
local DATA_PRINCE_KELESETH = 19 -- icecrown_citadel.h:95
local DATA_PRINCE_TALDARAM = 20 -- icecrown_citadel.h:96
local DATA_PRINCE_VALANAR = 21 -- icecrown_citadel.h:97
local PRINCES_DATA = { DATA_PRINCE_KELESETH, DATA_PRINCE_TALDARAM, DATA_PRINCE_VALANAR } -- boss_blood_prince_council.cpp:212-217
local SPELL_INVOCATION_OF_BLOOD_VALANAR = 70952 -- boss_blood_prince_council.cpp:105
local SPELL_INVOCATION_OF_BLOOD_KELESETH = 70981 -- boss_blood_prince_council.cpp:78
local SPELL_INVOCATION_OF_BLOOD_TALDARAM = 70982 -- boss_blood_prince_council.cpp:88
local SPELL_INVOCATION_VISUAL_ACTIVE = 71596 -- boss_blood_prince_council.cpp:70
local SPELL_INVOCATION_VISUAL_VALANAR = 71070 -- boss_blood_prince_council.cpp:123
local SPELL_INVOCATION_VISUAL_TALDARAM = 71081 -- boss_blood_prince_council.cpp:124
local SPELL_INVOCATION_VISUAL_KELESETH = 71080 -- boss_blood_prince_council.cpp:125
local SPELL_EMPOWERED_SHADOW_LANCE = 71815 -- boss_blood_prince_council.cpp:80
local SPELL_CONJURE_EMPOWERED_FLAME = 72040 -- boss_blood_prince_council.cpp:97
local SPELL_EMPOWERED_SHOCK_VORTEX = 72039 -- boss_blood_prince_council.cpp:109
local ACTION_CAST_INVOCATION = 4 -- boss_blood_prince_council.cpp:156
local ACTION_REMOVE_INVOCATION = 5 -- boss_blood_prince_council.cpp:157
local ACTION_START_INTRO = 1 -- boss_blood_prince_council.cpp:154
local ACTION_INTRO_DONE = 2 -- :155
local ACTION_STAND_UP = 3 -- :156
local ACTION_KINETIC_BOMB_JUMP = 6 -- :159
local POINT_KINETIC_BOMB_IMPACT = 384540 -- :165
local SPELL_FEIGN_DEATH = 71598 -- boss_blood_prince_council.cpp:68
local UNIT_FIELD_FLAGS = 59
local UNIT_FIELD_FLAGS_2 = 60
local UNIT_DYNAMIC_FLAGS = 79
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000
local UNIT_FLAG2_FEIGN_DEATH = 0x1 -- UnitDefines.h:161
local UNIT_DYNFLAG_DEAD = 0x20 -- SharedDefines.h:3133
local EMOTE_ONESHOT_ROAR = 15 -- SharedDefines.h:1950
local EMOTE_VALANAR_SHOCK_VORTEX = 3 -- boss_blood_prince_council.cpp:56
-- Per-prince invocation spell, for the ACTION_REMOVE_INVOCATION aura
-- strip (C++ SelectInvocationSpell :596-606).
local INVOCATION_SPELL = {
    [ENTRY_KELESETH] = SPELL_INVOCATION_OF_BLOOD_KELESETH,
    [ENTRY_TALDARAM] = SPELL_INVOCATION_OF_BLOOD_TALDARAM,
    [ENTRY_VALANAR] = SPELL_INVOCATION_OF_BLOOD_VALANAR,
}
-- BloodPrincesBossAI::_isEmpowered (:431/:612), per prince GUID —
-- flipped only by the invocation rotation's ev38 dispatches below.
local empowered = {}
-- boss_blood_council_controller::_invocationOrder + _invocationStage
-- (:291-301/:373-398), per controller GUID.
local invocationState = {}
-- Instance guid store (engine/world/instance_guid_store.go): each prince
-- latches its own GUID under its DATA_X on spawn (CREATURE_EVENT_ON_SPAWN
-- 5) — the Lua mirror of instance_icecrown_citadel.cpp:255-261.
local PRINCE_DATA = {
    [ENTRY_KELESETH] = DATA_PRINCE_KELESETH,
    [ENTRY_TALDARAM] = DATA_PRINCE_TALDARAM,
    [ENTRY_VALANAR] = DATA_PRINCE_VALANAR,
}
local function princeSpawn(event, creature)
    creature:SetGuidData(PRINCE_DATA[creature:GetEntry()])
end

-- Texts enum: SAY_KELESETH_SPECIAL = 2, EMOTE_KELESETH_BERSERK = 4,
-- EMOTE_TALDARAM_BERSERK = 5, SAY_TALDARAM_SPECIAL = 2,
-- SAY_VALANAR_BERSERK = 5.
local SAY_KELESETH_SPECIAL = 2
local EMOTE_KELESETH_BERSERK = 4
local EMOTE_TALDARAM_BERSERK = 5
local SAY_TALDARAM_SPECIAL = 2
local SAY_VALANAR_SPECIAL = 2
local SAY_VALANAR_BERSERK = 5

-- Spells enum: SPELL_BERSERK = 26662 (icecrown_citadel.h:33 shared
-- enum — the owning C++ script resolves DoCastSelf(SPELL_BERSERK)
-- through that header; 50334 was the PlayerAI troll-racial Berserk,
-- never a boss cast), SPELL_SHADOW_RESONANCE = 71943,
-- SPELL_SHADOW_LANCE = 71405, SPELL_GLITTERING_SPARKS = 71806,
-- SPELL_CONJURE_FLAME = 71718.
local SPELL_BERSERK = 26662
local SPELL_SHADOW_RESONANCE = 71943
local SPELL_SHADOW_RESONANCE_RESIST = 71822
local SPELL_SHADOW_RESONANCE_AURA = 72980
local SPELL_SHADOW_LANCE = 71405
local SPELL_GLITTERING_SPARKS = 71806
local SPELL_CONJURE_FLAME = 71718
local SPELL_KINETIC_BOMB_TARGET = 72053
local SPELL_SHOCK_VORTEX = 72037
local SPELL_SHADOW_PRISON = 72998
local SPELL_SHADOW_PRISON_DUMMY = 73001
local SPELL_SHADOW_PRISON_DAMAGE = 72999 -- boss_blood_prince_council.cpp:1312

-- entry-switched kill texts (C++ BloodPrincesBossAI::KilledUnit :505):
-- keleseth SAY_KELESETH_KILL = 3, taldaram SAY_TALDARAM_KILL = 4,
-- valanar SAY_VALANAR_KILL = 4.
local KILL_TEXT = {
    [ENTRY_KELESETH] = 3,
    [ENTRY_TALDARAM] = 4,
    [ENTRY_VALANAR] = 4,
}

-- entry-switched death texts (C++ BloodPrincesBossAI::JustDied :469):
-- keleseth SAY_KELESETH_DEATH = 5, taldaram EMOTE_TALDARAM_DEATH = 6,
-- valanar SAY_VALANAR_DEATH = 6.
local DEATH_TEXT = {
    [ENTRY_KELESETH] = 5,
    [ENTRY_TALDARAM] = 6,
    [ENTRY_VALANAR] = 6,
}

local timers = {}

-- Kinetic-bomb home positions, keyed by bomb GUIDLow: the C++
-- npc_kinetic_bomb _x/_y/_groundZ snapshot (Reset :983-994),
-- consumed by the JUMP + CONTINUE_FALLING MoveCharge legs.
local kineticBombHome = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

-- bronjahm-style: re-scheduling a key replaces the pending timer, so
-- each named arm has exactly one live timer at a time.
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

-- C++ SelectTarget(Random, 1, 0.0f, true) (boss_blood_prince_council.cpp:819
-- kinetic bomb, :835 shock vortex): offset 1 pops the current victim
-- (UnitAI.h non-distance list is victim-first), not the threat-sorted
-- head: exclude by victim GUID, then random alive threat player,
-- no distance gate. The old map-player minus-victim pool is retired.
local function randomNonTankPlayerAnywhere(creature)
    local victim = creature:GetVictim()
    local victimGuid = nil
    if victim ~= nil then
        victimGuid = victim:GetGUID()
    end
    local targets = creature:GetAITargets()
    if #targets <= 1 then
        return nil
    end
    local candidates = {}
    local aliveSeen = 0
    for _, t in ipairs(targets) do
        local skip = false
        if victimGuid ~= nil then
            skip = t:GetGUID() == victimGuid
        elseif not t:IsDead() then
            aliveSeen = aliveSeen + 1
            skip = aliveSeen == 1
        end
        if not skip
                and t:GetObjectType() == "Player" and not t:IsDead() then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ :658-661 keleseth EVENT_BERSERK: DoCastSelf(SPELL_BERSERK)
-- non-triggered + Talk(EMOTE_KELESETH_BERSERK), cast before Talk (C++
-- order), one-shot (no repeat).
local function onKelesethBerserk(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :651 (+ in-loop :675, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "berserk", 500, function() onKelesethBerserk(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_BERSERK)
    creature:Talk(EMOTE_KELESETH_BERSERK)
end

-- C++ :662-666 keleseth EVENT_SHADOW_RESONANCE:
-- Talk(SAY_KELESETH_SPECIAL) BEFORE DoCastSelf(SPELL_SHADOW_RESONANCE)
-- (C++ order), non-triggered; re-arm {10000,15000}ms.
local function onKelesethResonance(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :651 (+ in-loop :675, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "resonance", 500, function() onKelesethResonance(creature, guid) end)
        return
    end
    creature:Talk(SAY_KELESETH_SPECIAL)
    creature:CastSpell(creature, SPELL_SHADOW_RESONANCE)
    schedule(guid, "resonance", {10000, 15000}, function() onKelesethResonance(creature, guid) end)
end

-- C++ :667-670 keleseth EVENT_SHADOW_LANCE:
-- _isEmpowered ? DoCastVictim(SPELL_EMPOWERED_SHADOW_LANCE) :
-- DoCastVictim(SPELL_SHADOW_LANCE) = victim-cast; re-arm 2000ms. The
-- empowered variant (71815) is _isEmpowered-gated (unbridged machine)
-- — modeled as the base variant always (documented deviation).
local function onKelesethLance(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :651 (+ in-loop :675, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "lance", 500, function() onKelesethLance(creature, guid) end)
        return
    end
    if empowered[guid] then
        creature:CastSpell(nil, SPELL_EMPOWERED_SHADOW_LANCE)
    else
        creature:CastSpell(nil, SPELL_SHADOW_LANCE)
    end
    schedule(guid, "lance", 2000, function() onKelesethLance(creature, guid) end)
end

-- C++ :724-727 taldaram EVENT_BERSERK: DoCastSelf(SPELL_BERSERK) +
-- Talk(EMOTE_TALDARAM_BERSERK), cast before Talk (C++ order),
-- one-shot.
local function onTaldaramBerserk(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :717 (+ in-loop :749, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "berserk", 500, function() onTaldaramBerserk(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_BERSERK)
    creature:Talk(EMOTE_TALDARAM_BERSERK)
end

-- C++ :728-731 taldaram EVENT_GLITTERING_SPARKS:
-- DoCastVictim(SPELL_GLITTERING_SPARKS) = victim-cast;
-- re-arm {15000,50000}ms.
local function onTaldaramSparks(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :717 (+ in-loop :749, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "sparks", 500, function() onTaldaramSparks(creature, guid) end)
        return
    end
    creature:CastSpell(nil, SPELL_GLITTERING_SPARKS)
    schedule(guid, "sparks", {15000, 50000}, function() onTaldaramSparks(creature, guid) end)
end

-- C++ :732-744 taldaram EVENT_CONJURE_FLAME: DoCastSelf(
-- SPELL_CONJURE_FLAME) non-triggered + Talk(SAY_TALDARAM_SPECIAL)
-- AFTER the cast (C++ order); re-arm {20000,30000}ms. The empowered
-- variant DoCastSelf(SPELL_CONJURE_EMPOWERED_FLAME 72040) re-arm
-- {15000,25000}ms is _isEmpowered-gated (unbridged machine) — modeled
-- as the base variant always (documented deviation).
local function onTaldaramConjure(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :717 (+ in-loop :749, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "conjure", 500, function() onTaldaramConjure(creature, guid) end)
        return
    end
    if empowered[guid] then
        creature:CastSpell(creature, SPELL_CONJURE_EMPOWERED_FLAME)
        creature:Talk(SAY_TALDARAM_SPECIAL)
        schedule(guid, "conjure", {15000, 25000}, function() onTaldaramConjure(creature, guid) end)
    else
        creature:CastSpell(creature, SPELL_CONJURE_FLAME)
        creature:Talk(SAY_TALDARAM_SPECIAL)
        schedule(guid, "conjure", {20000, 30000}, function() onTaldaramConjure(creature, guid) end)
    end
end

-- C++ :814-817 valanar EVENT_BERSERK: DoCastSelf(SPELL_BERSERK) +
-- Talk(SAY_VALANAR_BERSERK), cast before Talk (C++ order), one-shot.
local function onValanarBerserk(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :807 (+ in-loop :844, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "berserk", 500, function() onValanarBerserk(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_BERSERK)
    creature:Talk(SAY_VALANAR_BERSERK)
end

-- C++ boss_prince_keleseth_icc::ScheduleEvents :622-624 —
-- EVENT_BERSERK 600000ms, EVENT_SHADOW_RESONANCE {10000,15000}ms,
-- EVENT_SHADOW_LANCE 2000ms + the IsHeroic-gated shadow-prison
-- legs :628-629 (creature:IsHeroic() now bridged).
-- JustEngagedWith (:455): the DoZoneInCombat(controller) leg is PORTED
-- in the member handlers (below); setActive/ScheduleEvents have no bridge.
local function kelesethEnterCombat(event, creature)
    creature:SetCombatPulseDelay(5) -- boss_blood_prince_council.cpp:457 (base JustEngagedWith arm 1; the setActive leg still has no bridge)
    -- C++ :459-460: DoZoneInCombat(controller) — engage the controller
    -- (entry 38008) in the zone; gathered via GetCreaturesInGrid (the
    -- illidari-council member precedent); the C++ ObjectAccessor nil-guard
    -- is the empty-gather no-op; bridge live 2026-10-23; C++ arm order kept.
    for _, controller in ipairs(creature:GetCreaturesInGrid(ENTRY_BLOOD_ORB_CONTROLLER, 100)) do
        controller:DoZoneInCombat()
    end
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    if creature:IsHeroic() then
        creature:CastSpell(creature, SPELL_SHADOW_PRISON, true)
        creature:CastSpell(creature, SPELL_SHADOW_PRISON_DUMMY)
    end
    schedule(guid, "berserk", 600000, function() onKelesethBerserk(creature, guid) end)
    schedule(guid, "resonance", {10000, 15000}, function() onKelesethResonance(creature, guid) end)
    schedule(guid, "lance", 2000, function() onKelesethLance(creature, guid) end)
end

-- C++ boss_prince_taldaram_icc::ScheduleEvents :687 —
-- EVENT_BERSERK 600000ms, EVENT_GLITTERING_SPARKS {12000,15000}ms,
-- EVENT_CONJURE_FLAME 20000ms + the IsHeroic-gated shadow-prison
-- leg :693 (creature:IsHeroic() now bridged).
local function taldaramEnterCombat(event, creature)
    creature:SetCombatPulseDelay(5) -- boss_blood_prince_council.cpp:457 (base JustEngagedWith arm 1; the setActive leg still has no bridge)
    -- C++ :459-460: DoZoneInCombat(controller) — engage the controller
    -- (entry 38008) in the zone; gathered via GetCreaturesInGrid (the
    -- illidari-council member precedent); the C++ ObjectAccessor nil-guard
    -- is the empty-gather no-op; bridge live 2026-10-23; C++ arm order kept.
    for _, controller in ipairs(creature:GetCreaturesInGrid(ENTRY_BLOOD_ORB_CONTROLLER, 100)) do
        controller:DoZoneInCombat()
    end
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    if creature:IsHeroic() then
        creature:CastSpell(creature, SPELL_SHADOW_PRISON, true)
    end
    schedule(guid, "berserk", 600000, function() onTaldaramBerserk(creature, guid) end)
    schedule(guid, "sparks", {12000, 15000}, function() onTaldaramSparks(creature, guid) end)
    schedule(guid, "conjure", 20000, function() onTaldaramConjure(creature, guid) end)
end

-- C++ :818-824 valanar EVENT_KINETIC_BOMB: if (target =
-- SelectTarget(Random, 1, 0.0f, true)) { DoCast(target,
-- SPELL_KINETIC_BOMB_TARGET 72053) (2-arg = non-triggered);
-- Talk(SAY_VALANAR_SPECIAL 2) }; events.Repeat(Seconds(18),
-- Seconds(24)) — unconditional re-arm (outside the if); seed
-- {18000,24000}ms (:763).
local function onValanarKineticBomb(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :807 (+ in-loop :844, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "kineticbomb", 500, function() onValanarKineticBomb(creature, guid) end)
        return
    end
    local target = randomNonTankPlayerAnywhere(creature)
    if target ~= nil then
        creature:CastSpell(target, SPELL_KINETIC_BOMB_TARGET)
        creature:Talk(SAY_VALANAR_SPECIAL)
    end
    schedule(guid, "kineticbomb", {18000, 24000}, function() onValanarKineticBomb(creature, guid) end)
end

-- C++ spell_valanar_kinetic_bomb_aura::HandleDummyTick
-- (boss_blood_prince_council.cpp:1230-1244, EFFECT_1
-- SPELL_AURA_PERIODIC_DUMMY; aura 72080, tick 189 port): each tick,
-- if a live kinetic bomb (38454) is within 1.0f of the aura target
-- (the bomb-target dummy), the bomb casts
-- SPELL_KINETIC_BOMB_EXPLOSION (72052) triggered on itself, drops
-- its SPELL_KINETIC_BOMB_VISUAL (72054) aura, and the tick aura
-- removes itself. No PreventDefaultAction in C++, so the handler
-- returns nothing. Bridges (checked against the CURRENT runtime):
-- target:FindNearestCreature(entry, dist)
-- (engine/world/lua_creature_events.go:5275; the C++ alive-only
-- third arg is subsumed — the bridge skips dead creatures and
-- returns the full motion-creature surface);
-- bomb:CastSpell(bomb, spell, true) triggered (:846);
-- bomb:RemoveAura(72054) is the RemoveAurasDueToSpell arm by spell
-- id; target:RemoveAura(72080) is the RemoveAura(GetAura()) arm.
-- The C++ TYPEID_UNIT guard is subsumed: Go aura targets are unit
-- objects by construction (spells.go:39385/41592).
-- PORTED tick 232: bomb->AI()->DoAction(72052) (C++ order — after the
-- visual-strip and aura-remove legs, :1240-1244) fires the bomb's ev38
-- handler below (dispatch bridged, creature:DoAction,
-- lua_creature_events.go:802); the 1s EVENT_BOMB_DESPAWN leg
-- (:1018-1022, SetVisible(false) + DespawnOrUnsummon(5s)) runs on the
-- file's GUID-keyed schedule. ACTION_KINETIC_BOMB_JUMP stays OUT
-- (MoveCharge — no bridge; caller is the unported knockback
-- SpellScript, :1262). The 60s Reset despawn (:991) PORTED 2026-11-09
-- via kineticBombReset (ev23) below, so an unexploded bomb now
-- despawns at 60s instead of lingering.
local function kineticBombAuraTick(event, aura, target)
    local bomb = target:FindNearestCreature(ENTRY_KINETIC_BOMB, 1.0)
    if bomb then
        bomb:CastSpell(bomb, SPELL_KINETIC_BOMB_EXPLOSION, true)
        bomb:RemoveAura(SPELL_KINETIC_BOMB_VISUAL)
        target:RemoveAura(SPELL_KINETIC_BOMB)
        bomb:DoAction(SPELL_KINETIC_BOMB_EXPLOSION)
    end
end

-- C++ spell_blood_council_shadow_prison (boss_blood_prince_council.cpp:1290-1310):
-- EFFECT_0 SPELL_AURA_PERIODIC_DUMMY. Each tick, a moving target casts
-- the damage spell on itself (CastSpell with the aura effect as the
-- triggered-by-aura attribution, not a triggered cast). No
-- PreventDefaultAction in C++ — the handler returns nothing, so the
-- default PERIODIC_DUMMY tick (a no-op in both engines) runs.
-- Bridge: creature:IsMoving (lua_creature_events.go, Unit.h:1641 twin).
-- DEVIATION: 72999's AddExtraDamage SpellScript leg (:1316-1324, the
-- existing 72999 aura's EFFECT_1 amount added to the damage) is
-- unported — no SpellScript bridge; the Lua cast reproduces the DBC
-- damage only.
local function shadowPrisonTick(event, aura, target)
    if target:IsMoving() then
        target:CastSpell(target, SPELL_SHADOW_PRISON_DAMAGE)
    end
end

-- C++ npc_kinetic_bomb::DoAction (boss_blood_prince_council.cpp:995-1010)
-- + UpdateAI EVENT_BOMB_DESPAWN (:1019-1022) / EVENT_CONTINUE_FALLING
-- (:1023-1027): action 72052 schedules the despawn 1s out; on fire
-- the bomb goes invisible and despawns 5s later. ACTION_KINETIC_BOMB_
-- JUMP (6) — sender is the knockback SpellScript (72087 BeforeHit,
-- :1252-1266, no SpellScript bridge) — knocks the bomb 100y up at
-- run speed unless the knockback aura is up, then 3s later the fall
-- leg charges it back to the Reset ground-Z; both charges ride the
-- MoveCharge bridge (lua_creature_events.go) with the
-- POINT_KINETIC_BOMB_IMPACT id (384540). The 3s reschedule is
-- unconditional in C++ (outside the HasAura gate) and fires once
-- (no repeat). Bridges: SetVisible (lua_creature_events.go:4977),
-- DespawnOrUnsummon (:6011), HasAura, GetSpeed(1) = MOVE_RUN,
-- MoveClear (MotionMaster::Clear twin); 1s/3s delays via the file's
-- GUID-keyed schedule (CreateLuaEvent, the princes' timer idiom).
-- PORTED this tick (JUMP + fall legs; explosion leg tick 232).
local function kineticBombDoAction(event, creature, action)
    local guid = creature:GetGUIDLow()
    if action == SPELL_KINETIC_BOMB_EXPLOSION then
        schedule(guid, "bombdespawn", 1000, function()
            creature:SetVisible(false)
            creature:DespawnOrUnsummon(5000)
            kineticBombHome[guid] = nil
        end)
        return
    end
    if action ~= ACTION_KINETIC_BOMB_JUMP then
        return
    end
    if not creature:HasAura(SPELL_KINETIC_BOMB_KNOCKBACK) then
        creature:MoveClear()
        local home = kineticBombHome[guid]
        if home then
            creature:MoveCharge(POINT_KINETIC_BOMB_IMPACT, home.x, home.y, creature:GetZ() + 100.0, creature:GetSpeed(1))
        end
    end
    schedule(guid, "bombfall", 3000, function()
        creature:MoveClear()
        local home = kineticBombHome[guid]
        if home then
            creature:MoveCharge(POINT_KINETIC_BOMB_IMPACT, home.x, home.y, home.groundZ, creature:GetSpeed(1))
        end
    end)
end

-- C++ npc_kinetic_bomb::Reset (:983-994): _events.Reset() (cancel
-- the bomb's timers — cancelTimers, the file's GUID-keyed idiom; the
-- bomb holds the bombdespawn + bombfall keys) + SetWalk(true)
-- + triggered self-casts SPELL_UNSTABLE 72059 + SPELL_KINETIC_BOMB_
-- VISUAL 72054 + SetReactState(REACT_PASSIVE) + DespawnOrUnsummon(
-- 60s) + the _x/_y/_groundZ snapshot (GetPosition + GetHeight, max
-- search 500) for the MoveCharge legs — map:GetHeight returns nil
-- off-grid, so the spawn Z is the fallback. The 60s
-- despawn and the explosion's 5s despawn do not conflict: Go's
-- DespawnOrUnsummon is a plain time.AfterFunc
-- (lua_creature_events.go:6637-6656) and a later fire on an
-- already-despawned motion is a no-op, so an exploded bomb still
-- despawns at 5s and an unexploded one at 60s, exactly like C++.
local function kineticBombReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    creature:SetWalk(true)
    creature:CastSpell(creature, SPELL_UNSTABLE, true)
    creature:CastSpell(creature, SPELL_KINETIC_BOMB_VISUAL, true)
    creature:SetReactState(0) -- REACT_PASSIVE
    creature:DespawnOrUnsummon(60000)
    local bx, by = creature:GetX(), creature:GetY()
    local mapObj = creature:GetMap()
    local groundZ = mapObj and mapObj:GetHeight(bx, by) or nil
    kineticBombHome[guid] = {x = bx, y = by, groundZ = groundZ or creature:GetZ()}
end

-- C++ :826-838 valanar EVENT_SHOCK_VORTEX base leg (else branch,
-- _isEmpowered false): if (target = SelectTarget(Random, 1, 0.0f,
-- true)) DoCast(target, SPELL_SHOCK_VORTEX 72037) (2-arg =
-- non-triggered, no Talk on the base leg); events.Repeat(Seconds(18),
-- Seconds(23)) — unconditional re-arm; seed {15000,20000}ms (:764).
-- The empowered leg (DoCastSelf 72039 + Talk(EMOTE 3) + 30s) is
-- _isEmpowered-gated and stays out.
local function onValanarShockVortex(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :807 (+ in-loop :844, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "shockvortex", 500, function() onValanarShockVortex(creature, guid) end)
        return
    end
    if empowered[guid] then
        creature:CastSpell(creature, SPELL_EMPOWERED_SHOCK_VORTEX)
        creature:Talk(EMOTE_VALANAR_SHOCK_VORTEX)
        schedule(guid, "shockvortex", 30000, function() onValanarShockVortex(creature, guid) end)
    else
        local target = randomNonTankPlayerAnywhere(creature)
        if target ~= nil then
            creature:CastSpell(target, SPELL_SHOCK_VORTEX)
        end
        schedule(guid, "shockvortex", {18000, 23000}, function() onValanarShockVortex(creature, guid) end)
    end
end

-- C++ boss_prince_valanar_icc::ScheduleEvents :761-765 —
-- EVENT_BERSERK 600000ms, EVENT_KINETIC_BOMB {18000,24000}ms,
-- EVENT_SHOCK_VORTEX {15000,20000}ms + the IsHeroic-gated
-- shadow-prison leg :767 (creature:IsHeroic() now bridged).
-- C++ npc_dark_nucleus::JustEngagedWith :1050-1069 (NOT valanar —
-- the prior header misattributed this arm): the targetAuraCheck
-- scheduler: 1000ms seed, targetAuraCheck.Repeat() 1000ms; if the
-- victim is within 15.0f and lacks the nucleus's Shadow Resonance
-- Resist aura (71822), DoCast(victim, 71822) non-triggered; plus the
-- immediate leg — distance(who) >= 15 -> DoStartMovement (no
-- movement bridge, dropped), else immediate DoCast(who, 71822).
-- The scheduler else-leg MoveInLineOfSight(victim) has no bridge
-- (movement machine) and is dropped; the C++ caster-GUID leg of
-- HasAura is unrepresentable (bridged HasAura takes only the spell
-- id) — only the nucleus applies 71822, so the plain check is exact.
-- C++ npc_dark_nucleus::DamageTaken (:1072-1078) — ResetAllThreat +
-- AddThreat(attacker, 500000000) PORTED below (nucleusDamageTaken);
-- the self-hit early-out is the GUID compare.
-- C++ npc_dark_nucleus::Reset :1043-1048: SetReactState(
-- REACT_DEFENSIVE) + triggered DoCastSelf(SPELL_SHADOW_RESONANCE_AURA
-- 72980).
-- No CASTING gate in C++: npc_dark_nucleus::UpdateAI (:1082) is a bare
-- _scheduler.Update(diff) and the aura check rides a JustEngagedWith
-- TaskScheduler lambda (:1050-1069) with no HasUnitState guard — this
-- tick stays ungated by design (checked 2026-11-04).
local function onNucleusAuraCheck(creature, guid)
    local victim = creature:GetVictim()
    local victimReach = 1.5
    if victim ~= nil and victim:GetObjectType() ~= "Player" then
        victimReach = victim:GetCombatReach()
    end
    if victim ~= nil and creature:GetDistance(victim) - creature:GetCombatReach() - victimReach < 15.0 -- C++ GetDistance gate (Object.cpp:1169, both reaches)
            and not victim:HasAura(SPELL_SHADOW_RESONANCE_RESIST) then
        creature:CastSpell(victim, SPELL_SHADOW_RESONANCE_RESIST)
    end
    schedule(guid, "auracheck", 1000, function() onNucleusAuraCheck(creature, guid) end)
end

local function nucleusEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    -- C++ npc_dark_nucleus::JustEngagedWith immediate leg
    -- (boss_blood_prince_council.cpp:1061-1066): >= 15.0f ->
    -- DoStartMovement (no bridge, dropped); else immediate
    -- DoCast(who, 71822), non-triggered.
    local targetReach = 1.5
    if target ~= nil and target:GetObjectType() ~= "Player" then
        targetReach = target:GetCombatReach()
    end
    if target ~= nil and creature:GetDistance(target) - creature:GetCombatReach() - targetReach < 15.0 then -- C++ GetDistance gate (Object.cpp:1169, both reaches)
        creature:CastSpell(target, SPELL_SHADOW_RESONANCE_RESIST)
    end
    schedule(guid, "auracheck", 1000, function() onNucleusAuraCheck(creature, guid) end)
end

-- C++ npc_dark_nucleus::DamageTaken (boss_blood_prince_council.cpp:1072-1078):
-- self-hit no-op (GUID compare, kalecgos precedent); otherwise
-- ResetAllThreat (ThreatManager::ResetAllThreat twin) +
-- AddThreat(attacker, 500000000). Damage passes through unchanged
-- (C++ leaves the damage param alone); no return rewrite.
local function nucleusDamageTaken(event, creature, attacker, damage)
    if attacker == nil or attacker:GetGUID() == creature:GetGUID() then
        return
    end
    creature:ResetAllThreat()
    creature:AddThreat(attacker, 500000000)
end

local function nucleusReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    -- C++ npc_dark_nucleus::Reset (boss_blood_prince_council.cpp:
    -- 1045-1048): REACT_DEFENSIVE = 1 (lua_creature_events.go:524)
    -- + triggered self-cast 72980.
    creature:SetReactState(1)
    creature:CastSpell(creature, SPELL_SHADOW_RESONANCE_AURA, true)
end

local function valanarEnterCombat(event, creature)
    creature:SetCombatPulseDelay(5) -- boss_blood_prince_council.cpp:457 (base JustEngagedWith arm 1; the setActive leg still has no bridge)
    -- C++ :459-460: DoZoneInCombat(controller) — engage the controller
    -- (entry 38008) in the zone; gathered via GetCreaturesInGrid (the
    -- illidari-council member precedent); the C++ ObjectAccessor nil-guard
    -- is the empty-gather no-op; bridge live 2026-10-23; C++ arm order kept.
    for _, controller in ipairs(creature:GetCreaturesInGrid(ENTRY_BLOOD_ORB_CONTROLLER, 100)) do
        controller:DoZoneInCombat()
    end
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    if creature:IsHeroic() then
        creature:CastSpell(creature, SPELL_SHADOW_PRISON, true)
    end
    schedule(guid, "berserk", 600000, function() onValanarBerserk(creature, guid) end)
    schedule(guid, "kineticbomb", {18000, 24000}, function() onValanarKineticBomb(creature, guid) end)
    schedule(guid, "shockvortex", {15000, 20000}, function() onValanarShockVortex(creature, guid) end)
end

-- C++ BloodPrincesBossAI::KilledUnit :505-521:
-- if (victim->GetTypeId() == TYPEID_PLAYER) switch (me->GetEntry())
-- Talk(kill text) — the razuvious player-gated variant precedent.
-- C++ BloodPrincesBossAI::DoAction (:549-580): ACTION_CAST_INVOCATION
-- (Talk(SelectInvocationSay()) + DoCastSelf(SPELL_INVOCATION_VISUAL_ACTIVE
-- 71596, true) + _isEmpowered = true) and ACTION_REMOVE_INVOCATION
-- (SetHealth(_spawnHealth = 1, InitializeAI :437) + RemoveAuras 71596 +
-- invocation spell + _isEmpowered = false). SelectInvocationSay returns
-- 0 for all three entries (the valanar SAY_KELESETH_INVOCATION quirk
-- :585-590). Arrives via ev38 from the controller rotation below
-- (the C++ SpellHit relay :535-539 is folded into the direct dispatch:
-- the visual/invocation casts carry no Lua-visible payload beyond it).
-- ACTION_STAND_UP (:552-563) PORTED tick 558 below (receiver only;
-- sender is the controller INTRO_DONE relay, ported in
-- controllerDoAction): RemoveAurasDueToSpell(FEIGN_DEATH) +
-- RemoveFlag(NOT_SELECTABLE) + RemoveFlag(DYNFLAG_DEAD) +
-- RemoveFlag(FLAG2_FEIGN_DEATH) + 1s StandUpEvent (PerformEmote
-- ROAR + SetReactState AGGRESSIVE, :177-189). The SetImmuneToPC(false)
-- and ForceValuesUpdateAtIndex legs stay out (no bridges). The arm is
-- a no-op until the feign-death intro machine lands — nothing ever
-- sets the princes passive/feign-death in Lua today.
local function princesDoAction(event, creature, action)
    local guid = creature:GetGUIDLow()
    if action == ACTION_CAST_INVOCATION then
        creature:Talk(0)
        creature:CastSpell(creature, SPELL_INVOCATION_VISUAL_ACTIVE, true)
        empowered[guid] = true
    elseif action == ACTION_REMOVE_INVOCATION then
        creature:SetHealth(1)
        creature:RemoveAura(SPELL_INVOCATION_VISUAL_ACTIVE)
        local inv = INVOCATION_SPELL[creature:GetEntry()]
        if inv then
            creature:RemoveAura(inv)
        end
        empowered[guid] = false
    elseif action == ACTION_STAND_UP then
        creature:RemoveAura(SPELL_FEIGN_DEATH)
        creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
        creature:RemoveFlag(UNIT_DYNAMIC_FLAGS, UNIT_DYNFLAG_DEAD)
        creature:RemoveFlag(UNIT_FIELD_FLAGS_2, UNIT_FLAG2_FEIGN_DEATH)
        schedule(guid, "standup", 1000, function()
            creature:PerformEmote(EMOTE_ONESHOT_ROAR)
            creature:SetReactState(2) -- REACT_AGGRESSIVE
        end)
    end
end

-- C++ BloodPrincesBossAI::DamageTaken (:494-501): a non-empowered prince
-- nullifies the damage but still takes threat from it. ev9 numeric-return
-- rewrite (lua_creature_events.go fireCreatureDamageTaken, halazzi
-- precedent): the boolean only gates the no-op base, the second return
-- rewrites the damage.
local function princesDamageTaken(event, creature, attacker, damage)
    if empowered[creature:GetGUIDLow()] then
        return
    end
    if attacker ~= nil then
        creature:AddThreat(attacker, damage)
    end
    return false, 0
end

local function princesKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(KILL_TEXT[creature:GetEntry()])
    end
end

-- C++ BloodPrincesBossAI::JustDied :466-486:
-- events.Reset + summons.DespawnAll (ported as the cancelTimers /
-- despawn*Summons calls below) + instance->SendEncounterUnit
-- (ENCOUNTER_FRAME_DISENGAGE, me) — PORTED tick 386 via
-- creature:SendEncounterUnit(1, creature) (lua_creature_events.go:6185;
-- the controllerLeaveCombat precedent in this file) — then the
-- switch (me->GetEntry()) Talk(death text) — the sjonnir
-- JustDied-Talk precedent.
-- C++ BloodPrincesBossAI SetData legs: Reset :452 instance->SetData(DATA_ORB_WHISPERER_ACHIEVEMENT, true)
-- PORTED this tick via the shared instanceData orbWhispererEligible key (princesReset); the DamageDealt
-- clear legs :549 (princes) / :965 (ball of flame) stay out — no DamageDealt hook in Go. JustReachedHome
-- :533 controller->AI()->SetData(DATA_PRINCE_EVADE, 1) PORTED via the ACTION_PRINCE_EVADE DoAction fold
-- (controllerDoAction below) — verified this tick vs C++ :304-310.
local function princesJustDied(event, creature)
    cancelTimers(creature:GetGUIDLow())
    despawnValanarSummons(creature:GetGUIDLow())
    despawnKelesethSummons(creature:GetGUIDLow())
    despawnTaldaramSummons(creature:GetGUIDLow())
    -- C++ boss_blood_prince_council.cpp:469: SendEncounterUnit
    -- (ENCOUNTER_FRAME_DISENGAGE) after the summon despawns,
    -- before Talk.
    creature:SendEncounterUnit(1, creature) -- ENCOUNTER_FRAME_DISENGAGE
    creature:Talk(DEATH_TEXT[creature:GetEntry()])
end

local function princesLeaveCombat(event, creature)
    empowered[creature:GetGUIDLow()] = false
    cancelTimers(creature:GetGUIDLow())
    despawnValanarSummons(creature:GetGUIDLow())
    despawnKelesethSummons(creature:GetGUIDLow())
    despawnTaldaramSummons(creature:GetGUIDLow())
end

local function princesReset(event, creature)
    empowered[creature:GetGUIDLow()] = false
    cancelTimers(creature:GetGUIDLow())
    despawnValanarSummons(creature:GetGUIDLow())
    despawnKelesethSummons(creature:GetGUIDLow())
    despawnTaldaramSummons(creature:GetGUIDLow())
    creature:SetCombatPulseDelay(0) -- boss_blood_prince_council.cpp:447 (base Reset arm; pulse disable)
    creature:RemoveFlag(59, 0x100) -- boss_blood_prince_council.cpp:449 (base Reset arm; SetImmuneToPC(false) = clear UNIT_FLAG_IMMUNE_TO_PC)
    creature:SetHealth(1) -- boss_blood_prince_council.cpp:451 (base Reset arm; SetHealth(_spawnHealth), _spawnHealth = 1 per InitializeAI :438)
    -- C++ :452 instance->SetData(DATA_ORB_WHISPERER_ACHIEVEMENT 18, true)
    -- via the shared instanceData orbWhispererEligible key (rotface
    -- oozeDanceEligible precedent); the criteria case reads it with
    -- the C++ ctor default true. The DamageDealt clear legs (:549
    -- princes, :965 ball of flame) stay out — no DamageDealt hook in
    -- Go — so the key can over-grant when a prince deals >23k/25k
    -- direct damage.
    local mapObj = creature:GetMap()
    if mapObj then
        local data = mapObj:GetInstanceData()
        if data then
            data.orbWhispererEligible = 1
        end
    end
    -- NOTE (2026-11-09): the UNIT_FLAG_NON_ATTACKABLE (0x2) clear is
    -- NOT here. C++ clears it only in the controller-Reset intro loop
    -- (:239-250), which runs AFTER the reach-home set in the wipe
    -- sequence — clearing here (ev23 fires before ev24) would be a
    -- no-op against the wipe flag. The clear rides controllerReset
    -- below, the loop's Lua twin.
end

-- Per-controller evade counter: C++ boss_blood_council_controller::
-- SetData(DATA_PRINCE_EVADE 4, 1) (:306-310) accumulates _resetCounter
-- and evades the controller at 3 (all three princes home). The
-- controller has no SetData override consumer beyond that counter, so
-- the cross-AI write folds into a DoAction dispatch (the invocation
-- rotation precedent): each prince's reach-home sends
-- ACTION_PRINCE_EVADE (100, Lua-only) and the controller's ev38
-- receiver counts it. Reset in controllerReset (the Initialize twin,
-- :228-232).
local evadeCounter = {}

-- C++ boss_blood_council_controller::DoAction (:338-360), PORTED
-- tick 558 (receiver only; sender is the queen's EVENT_INTRO_1,
-- unported): ACTION_INTRO_DONE relays ACTION_STAND_UP to all three
-- princes (via the instance GUID store) and heals Valanar to full.
-- ACTION_START_INTRO stays out: its gate reads instance
-- GetData(DATA_BLOOD_PRINCE_COUNCIL_INTRO 43) (no instance GetData
-- bridge) and its leg relays to the unregistered blood queen.
local function controllerDoAction(event, creature, action)
    local guid = creature:GetGUIDLow()
    if action == ACTION_PRINCE_EVADE then
        local count = (evadeCounter[guid] or 0) + 1
        evadeCounter[guid] = count
        if count == 3 then
            creature:EnterEvadeMode()
        end
    elseif action == ACTION_INTRO_DONE then
        for _, bossData in ipairs(PRINCES_DATA) do
            local prince = creature:GetInstanceCreature(bossData)
            if prince then
                prince:DoAction(ACTION_STAND_UP)
                if bossData == DATA_PRINCE_VALANAR then
                    prince:SetHealth(prince:GetMaxHealth())
                end
            end
        end
    end
end

-- C++ BloodPrincesBossAI::JustReachedHome (:525-534): DISENGAGE frame
-- + SetHealth(_spawnHealth) + _isEmpowered = false + (controller
-- present) SetFlag(NON_ATTACKABLE 0x2) + controller SetData(
-- DATA_PRINCE_EVADE, 1). ev24 OnReachHome (lua_creature_events.go,
-- the anubarak/lana-thel/bronjahm precedent). The controller SetData
-- leg rides the ACTION_PRINCE_EVADE DoAction above — the controller
-- evades once all three princes report home.
local function princesReachHome(event, creature)
    creature:SendEncounterUnit(1, creature) -- ENCOUNTER_FRAME_DISENGAGE
    creature:SetHealth(1)
    empowered[creature:GetGUIDLow()] = false
    local controller = creature:GetInstanceCreature(DATA_BLOOD_PRINCES_CONTROL)
    if controller ~= nil then
        creature:SetFlag(59, 0x2) -- UNIT_FLAG_NON_ATTACKABLE
        controller:DoAction(ACTION_PRINCE_EVADE)
    end
end

-- C++ boss_prince_valanar_icc::JustSummoned NPC_KINETIC_BOMB case
-- (boss_blood_prince_council.cpp:778-785): MovePoint to the ground
-- position (map:GetHeight, spawn-Z fallback) + RemoveFlag(
-- UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE) IMMEDIATELY after the
-- MovePoint call in the same handler — not arrival-gated. PORTED
-- this tick (MoveTo bridge + map:GetHeight).
local function valanarJustSummoned(_, creature, summoned)
    local entry = summoned:GetEntry()
    if entry == ENTRY_KINETIC_BOMB_TARGET then
        -- C++ JustSummoned NPC_KINETIC_BOMB_TARGET case (:774-777):
        -- passive react + self-cast of SPELL_KINETIC_BOMB. The C++
        -- cast credits valanar as original caster (3rd arg is the
        -- caster GUID); Lua CastSpell has no caster-credit arg, so
        -- the cast credits the target itself — documented delta.
        summoned:SetReactState(REACT_PASSIVE)
        summoned:CastSpell(summoned, SPELL_KINETIC_BOMB)
    elseif entry == ENTRY_SHOCK_VORTEX then
        -- C++ JustSummoned NPC_SHOCK_VORTEX case (:786-789):
        -- triggered dummy self-cast + VortexEvent at 5s (periodic
        -- self-cast, :198-202).
        summoned:CastSpell(summoned, SPELL_SHOCK_VORTEX_DUMMY, true)
        vortexTimers[summoned:GetGUIDLow()] = CreateLuaEvent(function()
            summoned:CastSpell(summoned, SPELL_SHOCK_VORTEX_PERIODIC, true)
        end, 5000)
    end
    if summoned:GetEntry() == ENTRY_KINETIC_BOMB then
        local bx, by = summoned:GetX(), summoned:GetY()
        local mapObj = summoned:GetMap()
        local groundZ = mapObj and mapObj:GetHeight(bx, by) or nil
        summoned:MoveTo(POINT_KINETIC_BOMB_IMPACT, bx, by, groundZ or summoned:GetZ())
        summoned:RemoveFlag(59, 0x02000000) -- UNIT_FLAG_NOT_SELECTABLE
    end
    if entry == ENTRY_KINETIC_BOMB_TARGET or entry == ENTRY_KINETIC_BOMB or entry == ENTRY_SHOCK_VORTEX then
        local guid = creature:GetGUIDLow()
        local list = valanarSummons[guid]
        if not list then
            list = {}
            valanarSummons[guid] = list
        end
        list[summoned:GetGUIDLow()] = summoned
    end
end

-- C++ ICC taldaram JustSummoned target pick (:699-701):
-- SelectTarget(Random, 1, -10.0f, true), fallback Random, 0, 0.0f.
-- UnitAI.cpp DefaultTargetSelector: a negative dist excludes
-- targets within combat range of that distance (vanishTick idiom:
-- GetDistance - GetCombatReach() - 1.5); position is ignored for
-- Random. withTank defaults true, so the victim stays eligible.
-- Alive filter follows the vanishTick precedent (GetAITargets may
-- list dead players; C++ has no alive gate — documented delta).
local function randomBallTarget(creature)
    local targets = creature:GetAITargets()
    local reach = creature:GetCombatReach()
    local far = {}
    local any = {}
    for _, t in ipairs(targets) do
        if t:GetObjectType() == "Player" and not t:IsDead() then
            any[#any + 1] = t
            if creature:GetDistance(t) - reach - 1.5 >= 10 then
                far[#far + 1] = t
            end
        end
    end
    if #far > 0 then
        return far[math.random(#far)]
    end
    if #any > 0 then
        return any[math.random(#any)]
    end
    return nil
end

-- boss_blood_council_controller (38008) — registered tick 221
-- (boss_ai.go); its JustEngagedWith/EnterEvadeMode frame arms port
-- below. The controller enters combat via the princes'
-- DoZoneInCombat(controller) leg (ported in each member's event-1
-- handler) — the C++ chain: a prince engages ->
-- DoZoneInCombat(controller) -> controller JustEngagedWith.
-- C++ JustEngagedWith (boss_blood_prince_council.cpp:266-302): the
-- IN_PROGRESS once-guard (:267-268) via the instance boss-state store;
-- CheckRequiredBosses (:270-275) has no bridge — the sequence-break
-- evade leg stays out; SetCombatPulseDelay(5), DoZoneInCombat,
-- C++ boss_blood_council_controller::UpdateAI EVENT_INVOCATION_OF_BLOOD
-- (:373-398): old prince DoAction(ACTION_REMOVE_INVOCATION), old casts
-- the new prince's visual spell on the new prince, new prince takes the
-- controller's health + Talk(textId), controller DoCastSelf(spellId),
-- re-arm 46.5s. The C++ SpellHit relay (:535-539) is folded into direct
-- ev38 dispatches (creature:DoAction, lua_creature_events.go:827): the
-- new prince's ACTION_CAST_INVOCATION is sent here instead of relying
-- on the spell hit, which carries no Lua-visible payload beyond it.
local function onInvocationOfBlood(controller, guid)
    local st = invocationState[guid]
    if not st then
        return
    end
    local oldEntry = st.order[st.stage]
    st.stage = st.stage % 3 + 1
    local newEntry = st.order[st.stage]
    local oldPrince = controller:GetInstanceCreature(oldEntry.data)
    local newPrince = controller:GetInstanceCreature(newEntry.data)
    if oldPrince then
        oldPrince:DoAction(ACTION_REMOVE_INVOCATION)
        if newPrince then
            oldPrince:CastSpell(newPrince, newEntry.visual, true)
        end
    end
    if newPrince then
        newPrince:SetHealth(controller:GetHealth())
        newPrince:Talk(newEntry.text)
        newPrince:DoAction(ACTION_CAST_INVOCATION)
    end
    controller:CastSpell(controller, newEntry.spell)
    schedule(guid, "invocation", 46500, function() onInvocationOfBlood(controller, guid) end)
end

-- SetBossState(IN_PROGRESS), DoCastSelf INVOCATION_OF_BLOOD_VALANAR
-- 70952, then the PrincesData ENGAGE loop (C++ order), then the
-- _invocationOrder build (:291-301: valanar first, taldaram/keleseth
-- by a 50/50 roll) + the EVENT_INVOCATION_OF_BLOOD seed (:289, 46.5s)
-- + the initial empowerment of valanar (the net effect of the
-- controller's 70952 self-cast, whose SpellHit leg has no Lua bearer).
-- setActive has no bridge. The CheckRequiredBosses sequence-break leg
-- (:270-275) stays out — no DoCastSpellOnPlayers bridge (the
-- GetPlayersInWorld + per-player 70781 workaround is not C++-exact
-- and is not applied to festergut/rotface/putricide either).
local function controllerEnterCombat(event, creature)
    if creature:GetBossState(DATA_BLOOD_PRINCE_COUNCIL) == 1 then -- IN_PROGRESS
        return
    end
    creature:SetCombatPulseDelay(5)
    creature:DoZoneInCombat()
    creature:SetBossState(DATA_BLOOD_PRINCE_COUNCIL, 1) -- IN_PROGRESS
    creature:CastSpell(creature, SPELL_INVOCATION_OF_BLOOD_VALANAR)
    for _, bossData in ipairs(PRINCES_DATA) do
        local prince = creature:GetInstanceCreature(bossData)
        if prince then
            creature:SendEncounterUnit(0, prince) -- ENCOUNTER_FRAME_ENGAGE
            prince:DoZoneInCombat()
        end
    end
    local order = {
        { data = DATA_PRINCE_VALANAR, spell = SPELL_INVOCATION_OF_BLOOD_VALANAR, text = 1, visual = SPELL_INVOCATION_VISUAL_VALANAR },
    }
    if math.random(2) == 1 then
        order[2] = { data = DATA_PRINCE_TALDARAM, spell = SPELL_INVOCATION_OF_BLOOD_TALDARAM, text = 1, visual = SPELL_INVOCATION_VISUAL_TALDARAM }
        order[3] = { data = DATA_PRINCE_KELESETH, spell = SPELL_INVOCATION_OF_BLOOD_KELESETH, text = 1, visual = SPELL_INVOCATION_VISUAL_KELESETH }
    else
        order[2] = { data = DATA_PRINCE_KELESETH, spell = SPELL_INVOCATION_OF_BLOOD_KELESETH, text = 1, visual = SPELL_INVOCATION_VISUAL_KELESETH }
        order[3] = { data = DATA_PRINCE_TALDARAM, spell = SPELL_INVOCATION_OF_BLOOD_TALDARAM, text = 1, visual = SPELL_INVOCATION_VISUAL_TALDARAM }
    end
    local guid = creature:GetGUIDLow()
    invocationState[guid] = { order = order, stage = 1 }
    local valanar = creature:GetInstanceCreature(DATA_PRINCE_VALANAR)
    if valanar then
        valanar:DoAction(ACTION_CAST_INVOCATION)
    end
    schedule(guid, "invocation", 46500, function() onInvocationOfBlood(creature, guid) end)
end

-- C++ EnterEvadeMode (boss_blood_prince_council.cpp:252-260): the
-- PrincesData DISENGAGE loop; summons.DespawnAll stays out (no
-- bridge); _DespawnAtEvade (:260) sets FAIL (BossAI::_DespawnAtEvade,
-- ScriptedCreature.cpp:617) — PORTED 2026-11-06 via
-- creature:SetBossState on ev2 (the deathwhisper ev2 evade-FAIL
-- precedent).
local function controllerLeaveCombat(event, creature)
    invocationState[creature:GetGUIDLow()] = nil
    cancelTimers(creature:GetGUIDLow())
    for _, bossData in ipairs(PRINCES_DATA) do
        local prince = creature:GetInstanceCreature(bossData)
        if prince then
            creature:SendEncounterUnit(1, prince) -- ENCOUNTER_FRAME_DISENGAGE
        end
    end
    creature:SetBossState(DATA_BLOOD_PRINCE_COUNCIL, BOSS_STATE_FAIL)
end

-- C++ boss_blood_council_controller::Reset (boss_blood_prince_council.cpp:233-250):
-- _Reset + Initialize (invocation counters — the rotation machine is unported,
-- so no Lua twin) + SummonCreatureGroup(SUMMON_PRINCES_GROUP 1) (group coords
-- DB-side; creature:SummonCreatureGroup bridged 2026-11-05, tick 238). The
-- intro loop (:240-249, instance GetData(DATA_BLOOD_PRINCE_COUNCIL_INTRO 43)
-- gate + per-prince RemoveFlag(NON_ATTACKABLE) / SetImmuneToPC(false) /
-- valanar SetHealth(max) + CastSpell(REMOVE_EMPOWERED_BLOOD)) stays out —
-- no instance GetData bridge (creature:GetData is the per-motion latch).
local function controllerReset(event, creature)
    invocationState[creature:GetGUIDLow()] = nil
    evadeCounter[creature:GetGUIDLow()] = nil
    cancelTimers(creature:GetGUIDLow())
    -- C++ boss_blood_council_controller::Reset :235 — _Reset() first:
    -- SetBossState NOT_STARTED only when the stored state != DONE
    -- (BossAI::_Reset, ScriptedCreature.cpp:499-509; the falric
    -- precedent). PORTED 2026-11-06 via creature:SetBossState.
    if creature:GetBossState(DATA_BLOOD_PRINCE_COUNCIL) ~= BOSS_STATE_DONE then
        creature:SetBossState(DATA_BLOOD_PRINCE_COUNCIL, BOSS_STATE_NOT_STARTED)
    end
    creature:SummonCreatureGroup(1) -- SUMMON_PRINCES_GROUP
    -- C++ controller Reset intro loop (:239-250): per-prince
    -- RemoveFlag(NON_ATTACKABLE) + SetImmuneToPC(false) + (valanar)
    -- SetHealth(max) + triggered Remove-Empowered-Blood self-cast.
    -- The loop is gated on instance GetData(DATA_BLOOD_PRINCE_COUNCIL
    -- _INTRO 43) == 0; the data starts 1, is set 0 by the first
    -- ACTION_START_INTRO (:342) and is never set back (no other
    -- SetData site in-tree), so on every wipe-reset the loop runs —
    -- and it is the ONLY 0x2-clear site (princesReset does not clear
    -- it). The Lua model has no intro-data store, so the legs run
    -- unconditionally here; SetImmuneToPC(false) rides princesReset
    -- (RemoveFlag 0x100) and is not repeated. At fresh-instance spawn
    -- the legs are no-ops (no flag set, valanar already at max).
    for _, bossData in ipairs(PRINCES_DATA) do
        local prince = creature:GetInstanceCreature(bossData)
        if prince ~= nil then
            prince:RemoveFlag(59, 0x2) -- UNIT_FLAG_NON_ATTACKABLE
            if bossData == DATA_PRINCE_VALANAR then
                prince:SetHealth(prince:GetMaxHealth())
                prince:CastSpell(prince, SPELL_REMOVE_EMPOWERED_BLOOD, true)
            end
        end
    end
end

RegisterCreatureEvent(ENTRY_KELESETH, 1, kelesethEnterCombat)
RegisterCreatureEvent(ENTRY_TALDARAM, 1, taldaramEnterCombat)
RegisterCreatureEvent(ENTRY_VALANAR, 1, valanarEnterCombat)
RegisterCreatureEvent(ENTRY_VALANAR, 19, valanarJustSummoned)
RegisterCreatureEvent(ENTRY_KELESETH, 19, function(_, creature, summoned)
    if summoned:GetEntry() ~= ENTRY_DARK_NUCLEUS then
        return
    end
    -- C++ ICC keleseth JustSummoned (:634-642): MovePositionToFirstCollision
    -- from the boss position along the boss->summon angle for the
    -- boss->summon distance, then NearTeleportTo. No collision-query
    -- bridge, so the open-space result ports: the computed point is
    -- the summon's own position (a wall would only shorten it).
    -- Orientation is the boss's (C++ pos = me->GetPosition(), :635-640).
    -- SetTempSummonType(CORPSE_DESPAWN) stays out.
    local dx = summoned:GetX() - creature:GetX()
    local dy = summoned:GetY() - creature:GetY()
    local dist = math.sqrt(dx * dx + dy * dy)
    local angle = math.atan2(dy, dx)
    summoned:NearTeleport(creature:GetX() + dist * math.cos(angle),
        creature:GetY() + dist * math.sin(angle), summoned:GetZ(),
        creature:GetO())
    local guid = creature:GetGUIDLow()
    local list = kelesethSummons[guid]
    if not list then
        list = {}
        kelesethSummons[guid] = list
    end
    list[summoned:GetGUIDLow()] = summoned
end)
RegisterCreatureEvent(ENTRY_TALDARAM, 19, function(_, creature, summoned)
    local entry = summoned:GetEntry()
    if entry ~= ENTRY_BALL_OF_INFERNO_FLAME and entry ~= ENTRY_BALL_OF_FLAME then
        return
    end
    local guid = creature:GetGUIDLow()
    local list = taldaramSummons[guid]
    if not list then
        list = {}
        taldaramSummons[guid] = list
    end
    list[summoned:GetGUIDLow()] = summoned
    -- C++ JustSummoned (:697-710): random-target pick + (inferno
    -- flame only) Talk emote + SetGUID, in that order (latch above is
    -- the summons.Summon leg). The base ball (38332, Conjure Flame
    -- 71718) takes the SetGUID store with NO emote (:703 gates the
    -- Talk on the inferno entry); the chase itself rides the
    -- aura-removal DoAction leg (ported above: flameBallVisualRemove
    -- -> ballOfFlameDoAction via ballChaseTarget).
    local target = randomBallTarget(creature)
    if target then
        if entry == ENTRY_BALL_OF_INFERNO_FLAME then
            creature:Talk(EMOTE_TALDARAM_FLAME, target)
        end
        local per = ballTargets[guid]
        if not per then
            per = {}
            ballTargets[guid] = per
        end
        per[summoned:GetGUIDLow()] = target
        ballChaseTarget[summoned:GetGUIDLow()] = target
    end
end)

RegisterCreatureEvent(ENTRY_KELESETH, 2, princesLeaveCombat)
RegisterCreatureEvent(ENTRY_TALDARAM, 2, princesLeaveCombat)
RegisterCreatureEvent(ENTRY_VALANAR, 2, princesLeaveCombat)

RegisterCreatureEvent(ENTRY_KELESETH, 38, princesDoAction)
RegisterCreatureEvent(ENTRY_TALDARAM, 38, princesDoAction)
RegisterCreatureEvent(ENTRY_VALANAR, 38, princesDoAction)
RegisterCreatureEvent(ENTRY_KELESETH, 9, princesDamageTaken)
RegisterCreatureEvent(ENTRY_TALDARAM, 9, princesDamageTaken)
RegisterCreatureEvent(ENTRY_VALANAR, 9, princesDamageTaken)

RegisterCreatureEvent(ENTRY_KELESETH, 3, princesKilledUnit)
RegisterCreatureEvent(ENTRY_TALDARAM, 3, princesKilledUnit)
RegisterCreatureEvent(ENTRY_VALANAR, 3, princesKilledUnit)

RegisterCreatureEvent(ENTRY_KELESETH, 4, princesJustDied)
RegisterCreatureEvent(ENTRY_TALDARAM, 4, princesJustDied)
RegisterCreatureEvent(ENTRY_VALANAR, 4, princesJustDied)

RegisterCreatureEvent(ENTRY_KELESETH, 23, princesReset)
RegisterCreatureEvent(ENTRY_TALDARAM, 23, princesReset)
RegisterCreatureEvent(ENTRY_VALANAR, 23, princesReset)
RegisterCreatureEvent(ENTRY_KELESETH, 5, princeSpawn)
RegisterCreatureEvent(ENTRY_TALDARAM, 5, princeSpawn)
RegisterCreatureEvent(ENTRY_VALANAR, 5, princeSpawn)
RegisterCreatureEvent(ENTRY_KINETIC_BOMB, 23, kineticBombReset)
RegisterCreatureEvent(ENTRY_KINETIC_BOMB, 38, kineticBombDoAction)
RegisterCreatureEvent(ENTRY_BLOOD_ORB_CONTROLLER, 1, controllerEnterCombat)
RegisterCreatureEvent(ENTRY_BLOOD_ORB_CONTROLLER, 2, controllerLeaveCombat)
RegisterCreatureEvent(ENTRY_BLOOD_ORB_CONTROLLER, 23, controllerReset)
RegisterCreatureEvent(ENTRY_BLOOD_ORB_CONTROLLER, 38, controllerDoAction)
RegisterCreatureEvent(ENTRY_KELESETH, 24, princesReachHome)
RegisterCreatureEvent(ENTRY_TALDARAM, 24, princesReachHome)
RegisterCreatureEvent(ENTRY_VALANAR, 24, princesReachHome)
RegisterCreatureEvent(ENTRY_BALL_OF_FLAME, 23, ballOfFlameReset)
RegisterCreatureEvent(ENTRY_BALL_OF_INFERNO_FLAME, 23, ballOfFlameReset)
RegisterCreatureEvent(ENTRY_BALL_OF_FLAME, 38, ballOfFlameDoAction)
RegisterCreatureEvent(ENTRY_BALL_OF_INFERNO_FLAME, 38, ballOfFlameDoAction)
RegisterAuraEvent(SPELL_FLAME_SPHERE_SPAWN_EFFECT, 3, function(event, aura, target)
    flameBallVisualRemove(SPELL_FLAME_SPHERE_SPAWN_EFFECT, event, aura, target)
end)
RegisterAuraEvent(SPELL_FLAME_SPHERE_DEATH_EFFECT, 3, function(event, aura, target)
    flameBallVisualRemove(SPELL_FLAME_SPHERE_DEATH_EFFECT, event, aura, target)
end)
RegisterCreatureEvent(ENTRY_DARK_NUCLEUS, 1, nucleusEnterCombat)
RegisterCreatureEvent(ENTRY_DARK_NUCLEUS, 9, nucleusDamageTaken)
RegisterCreatureEvent(ENTRY_DARK_NUCLEUS, 23, nucleusReset)

RegisterAuraEvent(SPELL_KINETIC_BOMB, 1, kineticBombAuraTick)
RegisterAuraEvent(SPELL_SHADOW_PRISON_DUMMY, 1, shadowPrisonTick)
