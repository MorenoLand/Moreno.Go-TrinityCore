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
-- valanar JustSummoned NPC_KINETIC_BOMB (:778-785) — RemoveFlag(59,
-- UNIT_FLAG_NOT_SELECTABLE) immediately after the MovePoint call
-- (not arrival-gated); MovePoint + ground-Z stay out (no motion /
-- terrain-height bridge).
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
-- EVENT_INVOCATION_OF_BLOOD seed, and the intro legs stay out
-- (instance GetData(DATA_BLOOD_PRINCE_COUNCIL_INTRO 43) gate — no
-- instance GetData bridge; creature:GetData is the per-motion latch,
-- not instance data). The Reset SummonCreatureGroup leg is PORTED
-- below (controllerReset, event 23). UpdateAI EVENT_INVOCATION_OF_BLOOD
-- (:373-398) cross-AI DoAction + Talk(textId) on the rotated prince is
-- the invocation rotation machine, driven by the unregistered council
-- controller (:352/:382/:540 — dispatch IS bridged, ev38);
-- boss_blood_council_
-- controller::JustDied (:322-336, prince kill sweep — the prince->
-- LowerPlayerDamageReq(:331) + Unit::Kill legs, SetBossState(DONE)
-- :323) stays out — controller death handling unported.
-- BloodPrincesBossAI::DoAction (:549-580) — Talk(SelectInvocationSay())
-- + DoCastSelf(SPELL_INVOCATION_VISUAL_ACTIVE 71596, true) ride
-- ACTION_CAST_INVOCATION, which arrives via SpellHit (:535-539) from the
-- controller's unbridged invocation cast (no bridge); the
-- ACTION_REMOVE_INVOCATION health/aura reset rides the controller's
-- cross-AI DoAction (dispatch IS bridged, ev38 — the controller
-- itself is unregistered). ACTION_STAND_UP (:552-563) rides the
-- same unregistered-controller machine: its 1s StandUpEvent (HandleEmoteCommand +
-- SetReactState(REACT_AGGRESSIVE), :177-189) is the un-gating of the
-- unported feign-death intro machine (JustAppeared :488), so the
-- AGGRESSIVE arm is vacuous in the Lua model too — nothing ever sets
-- the princes passive here.
-- BloodPrincesBossAI::JustAppeared (:488) — instance DATA_INTRO gate
-- (feign-death 71598 self-cast while the intro is not done) + SetHealth
-- (_spawnHealth) has no instance bridge; NOT registered (the intro
-- machine itself is unbridged — bronjahm npc_corrupted_soul_fragment
-- precedent).
-- BloodPrincesBossAI::DamageTaken (:494-501) — non-empowered damage
-- nullification (damage = 0 unless _isEmpowered) is gated on the
-- unbridged invocation machine — unmodeled (event 9 is NOT read-only:
-- numeric-return damage rewrite exists, halazzi precedent — the arm
-- stays out for the invocation-machine gate); all three princes take
-- full damage in the port (deviation, documented below).
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
-- EFFECT_0, aura 73001) — NOT PORTED tick 189: the gate is
-- GetTarget()->isMoving() on a prince creature (the aura is
-- DoCastSelf by the princes), and IsMoving exists only on the
-- player surface (lua_player.go:1620). Porting the 72999 cast
-- ungated would fabricate damage ticks while stationary.
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
-- emote + SetGUID store (ball chase DoAction out), valanar
-- bomb-target react/cast + vortex dummy/periodic casts (bomb
-- MovePoint ground-Z + DoZoneInCombat(summon) out).
-- npc_blood_queen_lana_thel — Talk(SAY_INTRO_1 0) rides
-- ACTION_START_INTRO (:866-880, sender is the unported controller /
-- blood-queen intro machine — dispatch IS bridged, ev38);
-- Talk(SAY_INTRO_2 1) rides EVENT_INTRO_1 (:896-905, intro scheduler
-- leg, no-timer-bridge) — NOT registered.
-- npc_ball_of_flame / npc_kinetic_bomb / npc_dark_nucleus — zero Talk
-- lines; Reset one-shot aura casts (:919-924, :987-988, :1047) are
-- aura/visual setup, not timer arms; the bomb's explode trigger now
-- rides the 72080 aura tick (ported tick 189) and its despawn leg
-- rides the same tick's DoAction(72052) -> ev38 handler (PORTED tick
-- 232; bomb registered in boss_ai.go). The chase leg
-- (ACTION_KINETIC_BOMB_JUMP, MoveCharge — no bridge) stays OUT.
-- (Their JustSummoned legs port via the boss ev19 handlers above,
-- not via AI registration.)
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
-- aura's periodic tick, ported tick 189 via RegisterAuraEvent; the
-- shadow-prison aura stays out — creature IsMoving unbridged, see
-- above), and
-- at_blood_prince_council_start_intro the no-AreaTrigger bridge queue
-- (the boss_anubrekhan at_anubrekhan_entrance precedent).
--
-- DEVIATIONS FROM C++ (documented; every modeled value C++-exact):
-- _isEmpowered never flips (invocation machine unbridged): keleseth
-- shadow lance always casts the base 71405 (never empowered 71815);
-- taldaram conjure flame always casts base 71718 with the base re-arm
-- {20000,30000}ms (never empowered 72040 / {15000,25000}ms).
-- BloodPrincesBossAI::DamageTaken's non-empowered nullification is
-- unmodeled — all three princes take full damage in the port.
-- keleseth's C++ "// does not melee" is unmodeled — Go melees.

local ENTRY_KELESETH = 37972
local ENTRY_TALDARAM = 37973
local ENTRY_VALANAR = 37970
local ENTRY_KINETIC_BOMB = 38454 -- icecrown_citadel.h:254 (NPC_KINETIC_BOMB)
local ENTRY_KINETIC_BOMB_TARGET = 38458 -- icecrown_citadel.h:253 (NPC_KINETIC_BOMB_TARGET)
local ENTRY_SHOCK_VORTEX = 38422 -- icecrown_citadel.h:255 (NPC_SHOCK_VORTEX)
local ENTRY_BALL_OF_INFERNO_FLAME = 38451 -- icecrown_citadel.h:251 (NPC_BALL_OF_INFERNO_FLAME)
local SPELL_KINETIC_BOMB = 72080 -- boss_blood_prince_council.cpp:107
local SPELL_KINETIC_BOMB_EXPLOSION = 72052 -- boss_blood_prince_council.cpp:115
local SPELL_KINETIC_BOMB_VISUAL = 72054 -- boss_blood_prince_council.cpp:114
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

local function despawnTaldaramSummons(guid)
    local list = taldaramSummons[guid]
    if list then
        for _, summon in pairs(list) do
            summon:Despawn()
        end
        ballTargets[guid] = nil
        taldaramSummons[guid] = nil
    end
end
local ENTRY_DARK_NUCLEUS = 38369 -- icecrown_citadel.h:250 (NPC_DARK_NUCLEUS)
local ENTRY_BLOOD_ORB_CONTROLLER = 38008 -- icecrown_citadel.h:248 (NPC_BLOOD_ORB_CONTROLLER)
local DATA_BLOOD_PRINCE_COUNCIL = 7 -- icecrown_citadel.h:81
local DATA_PRINCE_KELESETH = 19 -- icecrown_citadel.h:95
local DATA_PRINCE_TALDARAM = 20 -- icecrown_citadel.h:96
local DATA_PRINCE_VALANAR = 21 -- icecrown_citadel.h:97
local PRINCES_DATA = { DATA_PRINCE_KELESETH, DATA_PRINCE_TALDARAM, DATA_PRINCE_VALANAR } -- boss_blood_prince_council.cpp:212-217
local SPELL_INVOCATION_OF_BLOOD_VALANAR = 70952 -- boss_blood_prince_council.cpp:105
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
    creature:CastSpell(nil, SPELL_SHADOW_LANCE)
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
    creature:CastSpell(creature, SPELL_CONJURE_FLAME)
    creature:Talk(SAY_TALDARAM_SPECIAL)
    schedule(guid, "conjure", {20000, 30000}, function() onTaldaramConjure(creature, guid) end)
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
    local guid = creature:GetGUID()
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
    local guid = creature:GetGUID()
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
-- SpellScript, :1262). Without the despawn leg the bomb lingers
-- until its 60s Reset despawn (:990, Reset arms not ported).
local function kineticBombAuraTick(event, aura, target)
    local bomb = target:FindNearestCreature(ENTRY_KINETIC_BOMB, 1.0)
    if bomb then
        bomb:CastSpell(bomb, SPELL_KINETIC_BOMB_EXPLOSION, true)
        bomb:RemoveAura(SPELL_KINETIC_BOMB_VISUAL)
        target:RemoveAura(SPELL_KINETIC_BOMB)
        bomb:DoAction(SPELL_KINETIC_BOMB_EXPLOSION)
    end
end

-- C++ npc_kinetic_bomb::DoAction (boss_blood_prince_council.cpp:995-998)
-- + UpdateAI EVENT_BOMB_DESPAWN (:1019-1022): action 72052 schedules
-- the despawn 1s out; on fire the bomb goes invisible and despawns
-- 5s later. Bridges: SetVisible (lua_creature_events.go:4977),
-- DespawnOrUnsummon (:6011); 1s delay via the file's GUID-keyed
-- schedule (CreateLuaEvent, the princes' timer idiom).
local function kineticBombDoAction(event, creature, action)
    if action ~= SPELL_KINETIC_BOMB_EXPLOSION then
        return
    end
    local guid = creature:GetGUID()
    schedule(guid, "bombdespawn", 1000, function()
        creature:SetVisible(false)
        creature:DespawnOrUnsummon(5000)
    end)
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
    local target = randomNonTankPlayerAnywhere(creature)
    if target ~= nil then
        creature:CastSpell(target, SPELL_SHOCK_VORTEX)
    end
    schedule(guid, "shockvortex", {18000, 23000}, function() onValanarShockVortex(creature, guid) end)
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
    local guid = creature:GetGUID()
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
    cancelTimers(creature:GetGUID())
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
    local guid = creature:GetGUID()
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
local function princesKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(KILL_TEXT[creature:GetEntry()])
    end
end

-- C++ BloodPrincesBossAI::JustDied :466-486:
-- switch (me->GetEntry()) Talk(death text) — the sjonnir
-- JustDied-Talk precedent.
-- C++ BloodPrincesBossAI SetData legs: Reset :452 instance->SetData(DATA_ORB_WHISPERER_ACHIEVEMENT, true)
-- and DamageDealt :549/:965 set it false — achievement criteria, no achievement bridge. JustReachedHome :533
-- controller->AI()->SetData(DATA_PRINCE_EVADE, 1) — cross-AI write into the unported BloodPrinceController;
-- no cross-AI SetData bridge. All three dropped, documented.
local function princesJustDied(event, creature)
    cancelTimers(creature:GetGUID())
    despawnValanarSummons(creature:GetGUID())
    despawnKelesethSummons(creature:GetGUID())
    despawnTaldaramSummons(creature:GetGUID())
    creature:Talk(DEATH_TEXT[creature:GetEntry()])
end

local function princesLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
    despawnValanarSummons(creature:GetGUID())
    despawnKelesethSummons(creature:GetGUID())
    despawnTaldaramSummons(creature:GetGUID())
end

local function princesReset(event, creature)
    cancelTimers(creature:GetGUID())
    despawnValanarSummons(creature:GetGUID())
    despawnKelesethSummons(creature:GetGUID())
    despawnTaldaramSummons(creature:GetGUID())
    creature:SetCombatPulseDelay(0) -- boss_blood_prince_council.cpp:447 (base Reset arm; pulse disable)
    creature:RemoveFlag(59, 0x100) -- boss_blood_prince_council.cpp:449 (base Reset arm; SetImmuneToPC(false) = clear UNIT_FLAG_IMMUNE_TO_PC)
    creature:SetHealth(1) -- boss_blood_prince_council.cpp:451 (base Reset arm; SetHealth(_spawnHealth), _spawnHealth = 1 per InitializeAI :438)
end

-- C++ boss_prince_valanar_icc::JustSummoned NPC_KINETIC_BOMB case
-- (boss_blood_prince_council.cpp:778-785): the MovePoint to the
-- ground position stays out (no motion bridge; ground-Z needs the
-- terrain height query), but the RemoveFlag(UNIT_FIELD_FLAGS,
-- UNIT_FLAG_NOT_SELECTABLE) leg fires IMMEDIATELY after the MovePoint
-- call in the same handler — not arrival-gated — so it ports here.
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
        vortexTimers[summoned:GetGUID()] = CreateLuaEvent(function()
            summoned:CastSpell(summoned, SPELL_SHOCK_VORTEX_PERIODIC, true)
        end, 5000)
    end
    if summoned:GetEntry() == ENTRY_KINETIC_BOMB then
        summoned:RemoveFlag(59, 0x02000000) -- UNIT_FLAG_NOT_SELECTABLE
    end
    if entry == ENTRY_KINETIC_BOMB_TARGET or entry == ENTRY_KINETIC_BOMB or entry == ENTRY_SHOCK_VORTEX then
        local guid = creature:GetGUID()
        local list = valanarSummons[guid]
        if not list then
            list = {}
            valanarSummons[guid] = list
        end
        list[summoned:GetGUID()] = summoned
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
-- SetBossState(IN_PROGRESS), DoCastSelf INVOCATION_OF_BLOOD_VALANAR
-- 70952, then the PrincesData ENGAGE loop (C++ order). The
-- EVENT_INVOCATION_OF_BLOOD seed (:289) and the _invocationOrder build
-- (:291-301) stay out — the invocation rotation machine is unported,
-- so the order would be a dead store. setActive has no bridge.
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
end

-- C++ EnterEvadeMode (boss_blood_prince_council.cpp:252-260): the
-- PrincesData DISENGAGE loop; summons.DespawnAll + _DespawnAtEvade
-- have no bridges — stay out.
local function controllerLeaveCombat(event, creature)
    for _, bossData in ipairs(PRINCES_DATA) do
        local prince = creature:GetInstanceCreature(bossData)
        if prince then
            creature:SendEncounterUnit(1, prince) -- ENCOUNTER_FRAME_DISENGAGE
        end
    end
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
    creature:SummonCreatureGroup(1) -- SUMMON_PRINCES_GROUP
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
    local guid = creature:GetGUID()
    local list = kelesethSummons[guid]
    if not list then
        list = {}
        kelesethSummons[guid] = list
    end
    list[summoned:GetGUID()] = summoned
end)
RegisterCreatureEvent(ENTRY_TALDARAM, 19, function(_, creature, summoned)
    if summoned:GetEntry() ~= ENTRY_BALL_OF_INFERNO_FLAME then
        return
    end
    local guid = creature:GetGUID()
    local list = taldaramSummons[guid]
    if not list then
        list = {}
        taldaramSummons[guid] = list
    end
    list[summoned:GetGUID()] = summoned
    -- C++ JustSummoned (:697-710): random-target pick + Talk emote
    -- + SetGUID, in that order (latch above is the summons.Summon
    -- leg). The ball chase itself rides the aura-removal DoAction
    -- leg (unbridged) and stays out.
    local target = randomBallTarget(creature)
    if target then
        creature:Talk(EMOTE_TALDARAM_FLAME, target)
        local per = ballTargets[guid]
        if not per then
            per = {}
            ballTargets[guid] = per
        end
        per[summoned:GetGUID()] = target
    end
end)

RegisterCreatureEvent(ENTRY_KELESETH, 2, princesLeaveCombat)
RegisterCreatureEvent(ENTRY_TALDARAM, 2, princesLeaveCombat)
RegisterCreatureEvent(ENTRY_VALANAR, 2, princesLeaveCombat)

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
RegisterCreatureEvent(ENTRY_KINETIC_BOMB, 38, kineticBombDoAction)
RegisterCreatureEvent(ENTRY_BLOOD_ORB_CONTROLLER, 1, controllerEnterCombat)
RegisterCreatureEvent(ENTRY_BLOOD_ORB_CONTROLLER, 2, controllerLeaveCombat)
RegisterCreatureEvent(ENTRY_BLOOD_ORB_CONTROLLER, 23, controllerReset)
RegisterCreatureEvent(ENTRY_DARK_NUCLEUS, 1, nucleusEnterCombat)
RegisterCreatureEvent(ENTRY_DARK_NUCLEUS, 9, nucleusDamageTaken)
RegisterCreatureEvent(ENTRY_DARK_NUCLEUS, 23, nucleusReset)

RegisterAuraEvent(SPELL_KINETIC_BOMB, 1, kineticBombAuraTick)
