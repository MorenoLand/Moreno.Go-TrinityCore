-- Halion (Ruby Sanctum) — Lua port of
-- src/server/scripts/Northrend/ChamberOfAspects/RubySanctum/boss_halion.cpp
-- (10 CreatureScripts: boss_halion (BossAI, DATA_HALION) +
-- boss_twilight_halion (BossAI, DATA_TWILIGHT_HALION) +
-- npc_halion_controller (ScriptedAI, NPC_HALION_CONTROLLER = 40146) +
-- npc_meteor_strike_flame / npc_meteor_strike_initial / npc_meteor_strike
-- / npc_combustion_consumption / npc_orb_carrier / npc_living_inferno /
-- npc_living_ember (ScriptedAI, zero Talk); 1 GameObjectScript
-- (go_twilight_portal); 15 SpellScriptLoaders
-- (spell_halion_meteor_strike_marker,
-- spell_halion_combustion_consumption x2, spell_halion_marks x2,
-- spell_halion_combustion_consumption_periodic,
-- spell_halion_damage_aoe_summon, spell_halion_twilight_realm_handlers
-- x2, spell_halion_summon_exit_portals, spell_halion_twilight_phasing,
-- spell_halion_twilight_cutter, spell_halion_clear_debuffs,
-- spell_halion_spawn_living_embers, spell_halion_blazing_aura); all
-- registered from inside AddSC_boss_halion() (line 1906); loader decl
-- 193 / call 388 per northrend_script_loader.cpp — the SIXTH and FINAL
-- group of the "// Ruby Sanctum" block in AddNorthrendScripts(),
-- immediately after AddSC_boss_general_zarithrian() (call 387) —
-- verified from the loader this run; the checkpoint sequence
-- (zarithrian -> halion) is followed).
-- Entries: 39863 Halion (ruby_sanctum.h NPC_HALION, line 80) and 40142
-- Twilight Halion (ruby_sanctum.h NPC_TWILIGHT_HALION, line 81);
-- instance_ruby_sanctum.cpp creatureData binds NPC_HALION ->
-- DATA_HALION (line 54), NPC_TWILIGHT_HALION -> DATA_TWILIGHT_HALION
-- (line 55), NPC_HALION_CONTROLLER -> DATA_HALION_CONTROLLER (line 56)
-- — entry-verifiable, registration proceeds (the nexus_commanders
-- kalecgos precedent); the ScriptName bindings are DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_halion" hits boss_halion.cpp only (+ the loader
-- decl/call lines); this clone carries no sql/ tree, so ScriptName
-- bindings are DB-side by construction. Pre-existing lua coverage
-- AUDITED this run — COMPLETE (see accounting below).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnKill, 4 OnDied, 9 OnDamageTaken, 23 OnReset.
-- SetBossState legs (tick 270; DATA ids from ruby_sanctum.h:
-- DATA_HALION 3 (:34), DATA_TWILIGHT_HALION 4 (:37); EncounterState
-- NOT_STARTED 0 / IN_PROGRESS 1 / FAIL 2 / DONE 3, InstanceScript.h:
-- 68-76; bridges creature:SetBossState / GetBossState,
-- lua_creature_events.go:5917/:5936):
-- material halion ev1 IN_PROGRESS (base-first, :242-248), ev4 DONE
-- (_JustDied first, :263-268), ev23 FAIL-if-IN_PROGRESS else
-- NOT_STARTED-if-!= DONE (skadi idiom; _Reset :499-510 + controller
-- FAIL :619-634), NO ev24 (no C++ JustReachedHome FAIL leg) and NO
-- ev2 state leg (EnterEvadeMode :234-239 sets no state);
-- twilight halion ev1 IN_PROGRESS (base-first, :424-428), ev4 NO
-- state leg (no _JustDied call :448-466), ev23 NO state leg (Reset
-- empty :436). Full per-leg reasoning sits on the handlers below.
-- Ported arms (C++-exact for all modeled arms):
-- boss_halion JustEngagedWith — Talk(SAY_AGGRO 2) + the BossAI base
-- (SetBossState(DATA_HALION 3, IN_PROGRESS) PORTED tick 270,
-- base-first per C++ :242-248; pulse-5 + DoZoneInCombat already
-- ported) + AddAura SPELL_TWILIGHT_PRECISION 78243 (PORTED tick 270;
-- the old "absent bridge" claim was stale — creature:AddAura is
-- self-only, lua_creature_events.go:2416) + the phase-ungated
-- timer arms (event 1 — the auriaya precedent):
-- EVENT_CLEAVE — DoCastVictim(SPELL_CLEAVE 74524) non-triggered,
-- randtime(6s,10s) init / randtime(8s,10s) re-arm ->
-- creature:CastSpell(nil, SPELL_CLEAVE); EVENT_TAIL_LASH —
-- DoCastAOE(SPELL_TAIL_LASH 74531) non-triggered, randtime(7s,12s)
-- init / randtime(11s,16s) re-arm -> creature:CastSpell(creature,
-- SPELL_TAIL_LASH) (the DoCastAOE self-cast lowering); EVENT_BREATH —
-- DoCastSelf(SPELL_FLAME_BREATH 74525) non-triggered, randtime(5s,15s)
-- init / randtime(16s,25s) re-arm -> creature:CastSpell(creature,
-- SPELL_FLAME_BREATH) (boss_halion.cpp:251-255/:335-368); the 75%
-- DamageTaken PHASE_TWO transition stops all five phase-one arms via
-- halionDamageTaken (event 9 — the bronjahm latch precedent).
-- EVENT_FIERY_COMBUSTION / EVENT_METEOR_STRIKE PORTED tick 270 (see
-- UpdateAI below); the events.Reset / events.SetPhase(PHASE_ONE) legs
-- ride the phaseTwo latch, and the controller->AI()->SetData
-- (DATA_FIGHT_PHASE, PHASE_ONE) leg has no bridge (no cross-AI
-- SetData dispatch). SendEncounterUnit ENGAGE (param1 1) PORTED in
-- halionEnterCombat below (tick 222).
-- boss_halion JustDied — _JustDied() SetBossState(DATA_HALION 3,
-- DONE) PORTED tick 270, FIRST before Talk(SAY_DEATH 5) (event 4 —
-- the sjonnir precedent); the cross-creature
-- twilightHalion->KillSelf() / controller->KillSelf() legs PORTED
-- tick 270 (creature:Kill + grid scan, hexlord precedent — the old
-- "absent bridges" claim was stale). SendEncounterUnit DISENGAGE
-- PORTED in halionDied below (tick 222).
-- boss_twilight_halion KilledUnit — Talk(SAY_KILL 6) gated on
-- victim->GetTypeId() == TYPEID_PLAYER (event 3 — the razuvious
-- player-gated variant precedent) + the UNGATED triggered victim-cast
-- SPELL_LEAVE_TWILIGHT_REALM 74812 ("victims should not be in the
-- twilight realm" — the arlokk CastSpell(victim, id, true) idiom).
-- Material halion has no KilledUnit arm — SAY_KILL lives on the
-- twilight AI only.
-- boss_twilight_halion timer arms — PORTED via the CreateLuaEvent
-- harness (GUID-keyed timers table, cancel on events 2/4/23 — the
-- krystallus / bjarngrim-lieutenant convention), armed from
-- twilightHalionEnterCombat (entry 40142 registered events 1/2/3/4/23):
-- EVENT_CLEAVE — DoCastVictim(SPELL_CLEAVE 74524) non-triggered,
-- 3s init / randtime(7s,10s) re-arm (boss_halion.cpp:525-528) ->
-- creature:CastSpell(nil, SPELL_CLEAVE) — the DoCastVictim lowering.
-- EVENT_BREATH — DoCastSelf(SPELL_DARK_BREATH 74806) non-triggered,
-- 12s init / randtime(10s,14s) re-arm (:533-536) ->
-- creature:CastSpell(creature, SPELL_DARK_BREATH).
-- EVENT_TAIL_LASH — DoCastAOE(SPELL_TAIL_LASH 74531) non-triggered,
-- 12s init (armed in the C++ constructor, :420) / randtime(12s,16s)
-- re-arm (:529-532) -> creature:CastSpell(creature, SPELL_TAIL_LASH)
-- — the DoCastAOE self-cast lowering (boss_ai.go convention).
-- DOCUMENTED-ONLY (in this header; entries 39863 and 40142
-- registered):
-- boss_halion DamageTaken — PORTED (event 9, pre-damage): the 75%
-- PHASE_TWO transition — Talk(SAY_PHASE_TWO 4) + stop of the five
-- phase-one timer arms on post-damage health < 75% max
-- (boss_halion.cpp:286-296; HealthBelowPctDamaged strict-<, signed,
-- unclamped — the zum_rah pre-damage latch precedent). PORTED this run:
-- the DoCastSelf(SPELL_TWILIGHT_PHASING 74808) leg at :293 is now cast
-- as creature:CastSpell(creature, SPELL_TWILIGHT_PHASING) at the latch
-- (non-triggered DoCastSelf lowering — the header's old "phasing-cast
-- rides absent bridges" claim was stale; CastSpell is bridged),
-- plus the twilight-phasing SpellScript summon legs (74809 portal
-- cast, triggered, + 40142 map summon at HalionSpawnPos, Go type 7
-- MANUAL_DESPAWN per TemporarySummon.cpp:34). The
-- never-die damage cap (damage = health-1 while not PHASE_THREE) rides
-- the unbridged phase machine and stays out; the UNIT_FLAG_NOT_
-- SELECTABLE set leg is bridgeable (SetFlag live) but UNPAIRED —
-- its clear rides the controller/instance phase-three action, so a
-- set-alone port would leave the boss untargetable through phase 3;
-- the CastStop leg is ported (creature:StopSpellCast() in the 75%
-- latch, 2026-11-05); the controller SetData(PHASE_TWO) leg rides
-- the no-cross-AI bridge.
-- boss_halion SpellHit — Talk(SAY_REGENERATE 0) rides
-- SPELL_TWILIGHT_MENDING 75509 — stays out: the trigger is the
-- unported npc_halion_controller's EVENT_TWILIGHT_MENDING cast
-- (boss_halion.cpp:732-735), so event 14 can never fire with
-- this ID (the old "SpellHit-15-never-fires" label was stale —
-- event 14 fires from the Go spell path).
-- boss_halion UpdateAI — CASTING gates (material :327 pre-loop,
-- PHASE_TWO early-return handled by the 75% latch; twilight
-- pre-loop-only :513) ported as 500ms IsCasting defer on material
-- cleave/tail-lash/breath/meteor/fiery/firewall + twilight
-- cleave/breath/tail-lash/soul-consumption; the firewall GO leg is
-- non-cast but sits inside the gated loop (:347-351, after the :327
-- gate), so it carries the same 500ms defer (gated-loop non-cast-leg
-- precedent).
-- EVENT_METEOR_STRIKE / EVENT_FIERY_COMBUSTION PORTED tick 270
-- (meteorStrikeTick / fieryCombustionTick below): the old
-- "phase-machine-gated" stay-out is retracted — EventMap
-- ScheduleEvent defaults its phase arg to 0 (all phases,
-- EventMap.cpp), so the ONLY phase gate on these arms is the
-- UpdateAI PHASE_TWO early-return (:319-320), which the 75% latch
-- + cancelTimers reproduces (plus a phaseTwo guard in each tick for
-- the 500ms defer window); the target selection rides the
-- randomTwilightPlayer idiom with the aura filter inverted, the
-- meteor position cast rides creature:CastSpellAoF (non-triggered,
-- 5th arg false), the fiery cast rides the triggered arlokk idiom.
-- EVENT_ACTIVATE_FIREWALL is
-- PORTED below: its C++ schedule carries no phase group, so the 5s
-- engage-anchored one-shot fires regardless of phase. EVENT_BREATH /
-- EVENT_CLEAVE / EVENT_TAIL_LASH are PORTED above (phase-ungated
-- engage arms, stopped by the 75% latch).
-- boss_twilight_halion JustEngagedWith — zero Talk; the BossAI base
-- (SetBossState(DATA_TWILIGHT_HALION 4, IN_PROGRESS) PORTED tick 270,
-- base-first per C++ :424-428; pulse-5 + DoZoneInCombat already
-- ported) + AddAura SPELL_TWILIGHT_PRECISION (PORTED tick 270, as
-- above) + the four phase-ungated timer arms are PORTED (see
-- twilightHalionEnterCombat below — EVENT_SOUL_CONSUMPTION's ctor
-- schedule re-anchored on engage). C++ UpdateAI has no phase
-- early-return (boss_halion.cpp: 511-544), so the timers run
-- identically in PHASE_TWO and PHASE_THREE — engage-armed
-- CreateLuaEvent timers are faithful across both. The
-- SendEncounterUnit ENGAGE (param1 2) leg is PORTED in
-- twilightHalionEnterCombat (tick 222).
-- EVENT_SOUL_CONSUMPTION 74792 — PORTED (soulConsumptionTick, below):
-- SelectTarget(Random,1,0.0f,true,true,SPELL_TWILIGHT_REALM 74807) ->
-- CastSpell(target, SOUL_CONSUMPTION 74792, TRIGGERED_IGNORE_SET_FACING),
-- 15s init / 20s re-arm (:421/:537-540) — the stale "no random-target /
-- target-filter bridges" claim is retracted: the random-target leg rides
-- the threat-list skip-first pool (eck precedent, 2026-10-25), the aura
-- filter rides plr:HasAura (bridged,
-- engine/world/lua_player.go:203 — hakkar precedent), the triggered flag
-- rides the arlokk CastSpell(target, id, true) idiom.
-- Constructor-side legs (boss_twilight_halionAI ctor, :405-421): DUSK_
-- SHROUD 75476's triggered self-cast is bridgeable
-- (creature:CastSpell(creature, id, true)) but the ctor fires at
-- creature creation and there is no faithful construction-time hook:
-- CREATURE_EVENT_ON_SPAWN (5) fires on first grid activation since
-- 2026-10-19 (creaturemotion.go:1204) but temp summons bypass that
-- funnel, and twilight halion is a temp summon — so it stays out
-- (the old "ev5 fires only on the respawn path" sentence was stale).
-- COPY_DAMAGE auras stay out: the C++ uses explicit cross-creature
-- AddAura (halion->AddAura(SPELL_COPY_DAMAGE, me) to dodge
-- SPELL_ATTR6_CANT_TARGET_SELF) and the other body is unreachable from
-- Lua (no cross-creature bridge; the Lua AddAura bridge is self-only).
-- Health sync / phase-mask / react-state / combat-flag legs: the
-- phase-mask leg stays out for want of the same construction-time
-- hook, NOT for want of a bridge — creature:SetPhaseMask IS bridged
-- (lua_creature_events.go:3176), retracting the old "no creature
-- SetPhaseMask method" claim.
-- SetReactState and SetFlag ARE bridged (lua_creature_events.go:524;
-- e.g. SetFlag(59,0x300) in the incarcerator port) — the react-state /
-- IN_COMBAT-flag legs stay out for want of a construction-time hook, not
-- a bridge: CREATURE_EVENT_ON_SPAWN (5) fires on first grid activation
-- since 2026-10-19 but temp summons bypass that funnel
-- (creaturemotion.go), and the twilight phase machine is unmodeled
-- (75063's division summon never fires; 40142 now spawns via the
-- 75% phasing latch instead);
-- TAIL_LASH's 12s constructor-arm is documented as a spawn->engage drift
-- on the first tick only (twilight halion spawns in combat, so the delta
-- is near-zero).
-- boss_twilight_halion DamageTaken — PORTED (event 9, pre-damage):
-- the 50% transition (boss_halion.cpp:469-482) — the stale
-- "(no-DamageTaken bridge)" claim is retracted (event 9 is bridged and
-- already used by the material 75% latch in this file): latch-gated
-- DoCastSelf(SPELL_TWILIGHT_DIVISION 75063) (non-triggered) +
-- Talk(SAY_PHASE_THREE 2), C++ arm order (cast then Talk); the
-- never-die cap (damage = health - 1 while not PHASE_THREE) is ported
-- with C++ evaluation order (cap applied before the 50% check, so the
-- crossing blow is capped; the cap drops in phase three so the boss is
-- killable — the shared fightPhaseThree latch, gurtogg onChangePhase
-- idiom); CastStop ported (creature:StopSpellCast() before the Division
-- cast, 2026-11-05); the
-- PHASE_THREE controller->AI()->SetData(DATA_TWILIGHT_DAMAGE_TAKEN)
-- routing leg rides the no-cross-AI bridge. The :472-473 engage
-- fallback (first damage runs JustEngagedWith when it never fired)
-- PORTED tick 270 via the twilightEngaged latch.
-- boss_halion EnterEvadeMode (:236) — forwards the evade to the
-- controller creature when why == EVADE_REASON_BOUNDARY or the phase
-- machine is in PHASE_ONE — stays out (EnterEvadeMode has no Lua
-- bridge, boss_ai.go:5753-5754; the cross-creature forward rides the
-- no-cross-AI bridge). Its encounter-state outcome is NOT lost: the
-- controller's engagement-over JustExitedCombat sets DATA_HALION
-- FAIL (:619-634), and that leg is borne by halionReset's ev23
-- evade-fire (skadi idiom — see the SetBossState block above). No
-- ev24 handler: C++ sets no FAIL on halion's JustReachedHome
-- (BossAI::_JustReachedHome is setActive(false) only,
-- ScriptedCreature.cpp:522-523).
-- boss_twilight_halion SpellHit — Talk(SAY_REGENERATE 0) rides
-- SPELL_TWILIGHT_MENDING 75509 — stays out on the same unported-
-- controller trigger (boss_halion.cpp:732-735); the SPELL_TWILIGHT_
-- DIVISION -> controller DoAction(ACTION_MONITOR_CORPOREALITY)
-- leg stays out: the DoAction bridge (event 38) is live, but the
-- receiver (controller 40146) is unregistered — see triage above.
-- boss_twilight_halion JustDied — zero Talk; NO _JustDied() call in
-- C++ (:448-466), so DATA_TWILIGHT_HALION is never set DONE — no
-- state leg on ev4 (verified by whole-Ruby-Sanctum grep). The
-- halion->Unit::Kill / controller->KillSelf legs PORTED tick 270
-- (creature:Kill + grid scan, as in halionDied); the
-- LowerPlayerDamageReq leg (:454) stays out (no bridge). The
-- DoCastSelf(SPELL_CLEAR_DEBUFFS 75396) arm at boss_halion.cpp:599-605
-- is npc_halion_controllerAI::JustDied (entry 40146, unregistered),
-- NOT twilight halion's — stays out.
-- boss_twilight_halion Reset — EMPTY in C++ (boss_halion.cpp:436),
-- so no _Reset leg and NO SetBossState(DATA_TWILIGHT_HALION,
-- NOT_STARTED) on ev23 (contrast material halion, whose inherited
-- BossAI::Reset runs _Reset); ev23 stays timer-cancel only. The
-- DoCastSelf(SPELL_CLEAR_DEBUFFS 75396) arm at :586-592 is
-- npc_halion_controllerAI::Reset (entry 40146, unregistered) — stays
-- out (the _summons / _events / Initialize legs ride absent bridges).
-- npc_halion_controller (40146) — its intro machine issues cross-AI
-- Talk(SAY_INTRO 1) on the summoned halion, plus the corporeality
-- machine's Talk(EMOTE_CORPOREALITY_TOT 4 / TIT 3 on twilight halion,
-- POT 8 / PIP 9 on material halion) and the orb-shoot machine's
-- cross-AI Talk(EMOTE_WARN_LASER 0) on the orb-carrier passenger and
-- Talk(SAY_SPHERE_PULSE 1) on twilight halion (no cross-AI-Talk
-- bridge); the DoCastSelf / scheduler / DoAction legs ride absent
-- bridges. Zero Talk on its own unit — NOT registered (the bronjahm
-- npc_corrupted_soul_fragment precedent).
-- CONTROLLER TRIAGE (tick 222): JustExitedCombat :620-635 arms are
-- individually bridgeable (GetBossState DONE-guard + GetInstanceCreature
-- DATA 3/4 + DespawnOrUnsummon + SendEncounterUnit DISENGAGE per half +
-- SetBossState(DATA_HALION 3, FAIL 2) — all live), but the controller
-- can never enter combat in Go: its only engagement path is halion's
-- JustEngagedWith cross-AI SetData(DATA_FIGHT_PHASE, PHASE_ONE) ->
-- DoZoneInCombat (:259-260/:791), and creature:SetData is a generic
-- per-motion store, NOT virtual AI dispatch
-- (lua_creature_events.go:767) — the write would never engage it.
-- Registering 40146 for an ev2 handler that can never fire is dead
-- code; stays unregistered until the cross-AI SetData engagement chain
-- is bridged. The 30s respawn leg of me->DespawnOrUnsummon(0s, 30s)
-- also has no bridge (Eluna takes no forceRespawnTimer arg).
-- The other 8 CreatureScripts / go_twilight_portal / the 15
-- SpellScriptLoaders — zero Talk (npc_orb_carrier /
-- npc_meteor_strike* / npc_combustion_consumption) or spell-model arms
-- (no-SpellScript / no-AuraScript bridge, the sindragosa
-- ice_tomb_target precedent) — NOT registered. EXCEPTION (tick 194):
-- spell_halion_combustion_consumption's OnPeriodic leg (re-cast the
-- mark on the target each tick) is PORTED below via RegisterAuraEvent;
-- its OnApply / OnRemove legs are PORTED below too (ev2/ev3 — the
-- old "no apply/remove hooks" claim was stale; see the
-- combustion_consumption section).
-- npc_living_inferno
-- (40681) / npc_living_ember (40683): only their JustDied
-- DespawnOrUnsummon(1ms) legs port (event 4, below — 1ms ≈ immediate
-- creature:Despawn); the rest stays out (SetFlag IS bridged but no
-- cross-creature-SetData bridge; CastStop IS bridged — StopSpellCast
-- ported in the 75%/50% latches, 2026-11-05).
-- SAY_BERSERK 7 — dead text, no Talk() call in the file.
-- All 13 Talk() calls in the file accounted for (SAY_AGGRO 2 +
-- SAY_DEATH 5 + SAY_KILL 6 + SAY_METEOR_STRIKE 3 ported; SAY_PHASE_TWO 4
-- + SAY_PHASE_THREE 2 ported via the DamageTaken latches;
-- SAY_REGENERATE 0 x2 + SAY_INTRO 1 +
-- EMOTE_CORPOREALITY_TOT/TIT/POT/PIP + EMOTE_WARN_LASER 0 +
-- SAY_SPHERE_PULSE 1 documented above).
-- Missing-cast audit 2026-10-16 (all 11): PORTED — 74808
-- SPELL_TWILIGHT_PHASING (:293, halionDamageTaken 75% latch); 75063
-- SPELL_TWILIGHT_DIVISION (:479, twilightHalionDamageTaken 50% latch —
-- the 2026-10-16 "twilight phase machine unmodeled" claim is retracted:
-- the transition is ported with the 50% latch + Talk). BLOCKED —
-- 75476 DUSK_SHROUD (:414, twilight ctor, no construction hook);
-- 74805 SUMMON_EXIT_PORTALS (:677), 76006 COSMETIC_FIRE_
-- PILLAR (:710), 76010 FIERY_EXPLOSION (:726) — npc_halion_controller
-- (40146, unregistered) DoAction/event arms, triggers instance-side;
-- 74648 AOE_DAMAGE (:1006), 74713 FIRE_AURA_1 (:1007), 74641 COUNTDOWN
-- (:1024), 40031 BIRTH_NO_VISUAL (:1025), 74718 FIRE_AURA_2 (:1079) —
-- npc_meteor_strike_initial / npc_meteor_strike (unregistered), all
-- hanging off the boss EVENT_METEOR_STRIKE arm (ported tick 270;
-- the NPC scripts themselves stay unregistered).

local ENTRY_HALION = 39863
local ENTRY_TWILIGHT_HALION = 40142
local GO_FLAME_RING = 203007 -- ruby_sanctum.h:112
local GO_TWILIGHT_FLAME_RING = 203624 -- ruby_sanctum.h:113
local ENTRY_LIVING_INFERNO = 40681 -- ruby_sanctum.h:83
local ENTRY_LIVING_EMBER = 40683 -- ruby_sanctum.h:84
local DATA_HALION = 3 -- ruby_sanctum.h:34 (RSDataTypes)
local DATA_TWILIGHT_HALION = 4 -- ruby_sanctum.h:37
local ENTRY_HALION_CONTROLLER = 40146 -- ruby_sanctum.h:82

local SAY_INTRO    = 1 -- (documented-only, rides the controller's cross-AI intro)
local SAY_AGGRO    = 2
local SAY_METEOR_STRIKE = 3
local SAY_PHASE_TWO = 4
local SAY_PHASE_THREE = 2
local SAY_DEATH    = 5
local SAY_KILL     = 6 -- twilight halion only

local SPELL_CLEAVE      = 74524
local SPELL_DARK_BREATH = 74806
local SPELL_FLAME_BREATH = 74525
local SPELL_TAIL_LASH   = 74531
local SPELL_LEAVE_TWILIGHT_REALM = 74812
local SPELL_TWILIGHT_PHASING = 74808 -- C++ enum :111 ("Phase spell from phase 1 to phase 2")
local SPELL_SUMMON_TWILIGHT_PORTAL = 74809 -- C++ enum :112 (summons GO 202794)
local SPELL_SOUL_CONSUMPTION = 74792 -- C++ enum :81
local SPELL_TWILIGHT_REALM = 74807 -- C++ enum :115 (SelectTarget aura filter)
local SPELL_TWILIGHT_DIVISION = 75063 -- C++ enum :109 ("Phase spell from phase 2 to phase 3")
local SPELL_TWILIGHT_PRECISION = 78243 -- C++ enum :117 (AddAura self-cast, both engages)
local SPELL_METEOR_STRIKE = 74637 -- C++ enum :64
local SPELL_METEOR_STRIKE_COUNTDOWN = 74641 -- C++ enum :96
local ACTION_METEOR_STRIKE_AOE = 2 -- boss_halion.cpp:154
local AURA_REMOVE_BY_EXPIRE = 4 -- SpellAuraDefines.h:67
local AURA_REMOVE_BY_DEATH = 5 -- SpellAuraDefines.h:68
local SPELL_FIERY_COMBUSTION = 74562 -- C++ enum :67
local GO_HALION_PORTAL_1 = 202794 -- ruby_sanctum.h:107
local GO_HALION_PORTAL_2 = 202795 -- ruby_sanctum.h:108
local GO_HALION_PORTAL_EXIT = 202796 -- ruby_sanctum.h:109

-- CreateLuaEvent timer harness (GUID-keyed timers table, cancel on
-- events 2/4/23) — the krystallus / bjarngrim-lieutenant convention.
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

-- Material-halion phase-one timer arms (C++ UpdateAI :335-344):
-- EVENT_CLEAVE — DoCastVictim(SPELL_CLEAVE 74524) non-triggered,
-- randtime(6s,10s) init / randtime(8s,10s) re-arm.
local function matCleaveTick(creature, guid)
    creature:CastSpell(nil, SPELL_CLEAVE)
    if creature:IsCasting() then
        schedule(guid, "mat_cleave", 500, function()
            matCleaveTick(creature, guid)
        end)
        return
    end
    schedule(guid, "mat_cleave", {8000, 10000}, function()
        matCleaveTick(creature, guid)
    end)
end

-- EVENT_TAIL_LASH — DoCastAOE(SPELL_TAIL_LASH 74531) non-triggered,
-- randtime(7s,12s) init / randtime(11s,16s) re-arm — the DoCastAOE
-- self-cast lowering (boss_ai.go convention).
local function matTailLashTick(creature, guid)
    creature:CastSpell(creature, SPELL_TAIL_LASH)
    if creature:IsCasting() then
        schedule(guid, "mat_tail_lash", 500, function()
            matTailLashTick(creature, guid)
        end)
        return
    end
    schedule(guid, "mat_tail_lash", {11000, 16000}, function()
        matTailLashTick(creature, guid)
    end)
end

-- EVENT_BREATH — DoCastSelf(SPELL_FLAME_BREATH 74525) non-triggered,
-- randtime(5s,15s) init / randtime(16s,25s) re-arm (distinct from the
-- twilight DARK_BREATH 74806 below).
local function matBreathTick(creature, guid)
    creature:CastSpell(creature, SPELL_FLAME_BREATH)
    if creature:IsCasting() then
        schedule(guid, "mat_breath", 500, function()
            matBreathTick(creature, guid)
        end)
        return
    end
    schedule(guid, "mat_breath", {16000, 25000}, function()
        matBreathTick(creature, guid)
    end)
end

-- 75% phase-two latch (C++ boss_halion::DamageTaken :286-296 —
-- pre-damage event 9; the zum_rah latch precedent).
local phaseTwo = {}

-- Encounter-wide phase-three latch (C++ boss_halion.cpp:283/:469 —
-- the never-die damage cap drops in PHASE_THREE, i.e. the boss
-- becomes killable). The controller SetData(DATA_FIGHT_PHASE) phase
-- machine has no bridge, so the latch is set by the ported twilight
-- 50% transition (twilightHalionDamageTaken below — the gurtogg
-- onChangePhase idiom) and read by both never-die caps; cleared on
-- every combat-end / reset / died path of both halves.
local fightPhaseThree = false

-- C++ UpdateAI EVENT_METEOR_STRIKE (:352-361) / EVENT_FIERY_COMBUSTION
-- (:363-368) target selection: SelectTarget(Random, pos, 0.0f, true,
-- true, -SPELL_TWILIGHT_REALM 74807) — random alive player WITHOUT the
-- twilight-realm aura (material realm), position 0 (meteor, victim
-- included) or 1 (fiery, victim skipped). Mirrors
-- randomTwilightPlayer below with the aura filter inverted (eck
-- precedent); the position-1 offset pops the MaxThreat-ordered first
-- entry before the filters (UnitAI.h offset-before-filter).
local function randomMaterialPlayer(creature, skipVictim)
    local targets = creature:GetAITargets()
    local victim = creature:GetVictim()
    local victimGuid = nil
    if victim ~= nil then
        victimGuid = victim:GetGUID()
    end
    local candidates = {}
    local aliveSeen = 0
    for _, t in ipairs(targets) do
        local skip = false
        if skipVictim then
            if victimGuid ~= nil then
                skip = t:GetGUID() == victimGuid
            elseif not t:IsDead() then
                aliveSeen = aliveSeen + 1
                skip = aliveSeen == 1
            end
        end
        if not skip
                and t:GetObjectType() == "Player" and t:IsAlive()
                and not t:HasAura(SPELL_TWILIGHT_REALM) then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ UpdateAI EVENT_METEOR_STRIKE (:352-361): random material-realm
-- player -> _meteorStrikePos = target position -> CastSpell(pos,
-- SPELL_METEOR_STRIKE 74637, guid) NON-triggered -> Talk
-- (SAY_METEOR_STRIKE 3), 18s init / 38s re-arm, the re-arm
-- unconditional (outside the target-if). The position cast rides
-- creature:CastSpellAoF (lua_creature_events.go; 5th arg false =
-- non-triggered, the default true would wrongly trigger it); the
-- target position rides target:GetX/Y/Z. Phase-one-only: C++
-- UpdateAI early-returns in PHASE_TWO (:319-320), so the 75% latch's
-- cancelTimers kills this arm and the phaseTwo guard below covers the
-- 500ms defer window.
local function meteorStrikeTick(creature, guid)
    if phaseTwo[guid] then
        return
    end
    local target = randomMaterialPlayer(creature, false)
    if creature:IsCasting() then
        schedule(guid, "meteor_strike", 500, function()
            meteorStrikeTick(creature, guid)
        end)
        return
    end
    if target then
        creature:CastSpellAoF(target:GetX(), target:GetY(), target:GetZ(), SPELL_METEOR_STRIKE, false)
        creature:Talk(SAY_METEOR_STRIKE)
    end
    schedule(guid, "meteor_strike", 38000, function()
        meteorStrikeTick(creature, guid)
    end)
end

-- C++ spell_halion_meteor_strike_marker::OnRemove
-- (boss_halion.cpp:1410-1422): AfterEffectRemove EFFECT_0
-- SPELL_AURA_DUMMY on SPELL_METEOR_STRIKE_COUNTDOWN 74641. On
-- expiry only, the aura's caster creature gets
-- DoAction(ACTION_METEOR_STRIKE_AOE 2) — the npc_meteor_strike_initial
-- leg (DoCastSelf 74648 + 74713, triggered, + BURN relay to its four
-- meteors, :1005-1010). PORTED 2026-11-11 via RegisterAuraEvent ev3
-- + creature:DoAction (lua_creature_events.go:827, event-38 dispatch).
-- DORMANT twice over: the countdown aura is applied by the
-- initial's unported IsSummonedBy (:1024), and the DoAction consumer
-- stays out — the initial's entry is DB-bound (ScriptName
-- npc_meteor_strike_initial; NPC_METEOR_STRIKE_MARK 40029 is defined
-- in ruby_sanctum.h:91 but never referenced in the C++ file, so no
-- entry is verifiable from C++ evidence) — until that entry lands,
-- the dispatch fires into no registered ev38 handler.
local function meteorStrikeMarkerOnRemove(event, aura, target)
    if aura:GetRemoveMode() ~= AURA_REMOVE_BY_EXPIRE then
        return
    end
    local caster = aura:GetCaster()
    if caster ~= nil and caster:GetObjectType() == "Creature" then
        caster:DoAction(ACTION_METEOR_STRIKE_AOE)
    end
end

RegisterAuraEvent(SPELL_METEOR_STRIKE_COUNTDOWN, 3, meteorStrikeMarkerOnRemove)

-- C++ UpdateAI EVENT_FIERY_COMBUSTION (:363-368): random material-realm
-- player (victim skipped, position 1) -> CastSpell(target,
-- SPELL_FIERY_COMBUSTION 74562, TRIGGERED_IGNORE_SET_FACING),
-- randtime(15s,18s) init / 25s re-arm, the re-arm unconditional
-- (outside the target-if). Triggered rides the arlokk
-- CastSpell(target, id, true) idiom. Phase-one-only, same gate as
-- meteorStrikeTick above.
local function fieryCombustionTick(creature, guid)
    if phaseTwo[guid] then
        return
    end
    local target = randomMaterialPlayer(creature, true)
    if creature:IsCasting() then
        schedule(guid, "fiery_combustion", 500, function()
            fieryCombustionTick(creature, guid)
        end)
        return
    end
    if target then
        creature:CastSpell(target, SPELL_FIERY_COMBUSTION, true)
    end
    schedule(guid, "fiery_combustion", 25000, function()
        fieryCombustionTick(creature, guid)
    end)
end

-- C++ UpdateAI EVENT_ACTIVATE_FIREWALL (:347-351): flame-ring GO
-- state flip (HandleGameObject false = READY 1 on both rings), 5s
-- engage-anchored one-shot. Non-cast but INSIDE the gated loop (the
-- :327 pre-loop gate delays the EVENT, not just casts), so it carries
-- the same 500ms IsCasting defer as the cast arms — check before the
-- GO leg, re-schedule consumes no re-arm (a one-shot has none).
-- Cancelled on combat end by halionCombatEnd's cancelTimers, matching
-- events.Reset; the 75% latch's cancelTimers likewise kills a
-- deferred re-schedule, so no phaseTwo guard is needed.
local function firewallTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "firewall", 500, function()
            firewallTick(creature, guid)
        end)
        return
    end
    local flameRing = creature:FindNearestGameObject(GO_FLAME_RING, 100)
    if flameRing then flameRing:SetGoState(1) end
    local twilightRing = creature:FindNearestGameObject(GO_TWILIGHT_FLAME_RING, 100)
    if twilightRing then twilightRing:SetGoState(1) end
end

-- C++ boss_halion::JustEngagedWith :242-262:
-- Talk(SAY_AGGRO); events.Reset(); events.SetPhase(PHASE_ONE);
-- BossAI::JustEngagedWith(who); me->AddAura(SPELL_TWILIGHT_PRECISION, me);
-- events.ScheduleEvent(EVENT_ACTIVATE_FIREWALL, 5s);
-- events.ScheduleEvent(EVENT_BREATH, randtime(5s,15s));
-- events.ScheduleEvent(EVENT_CLEAVE, randtime(6s,10s));
-- events.ScheduleEvent(EVENT_TAIL_LASH, randtime(7s,12s));
-- events.ScheduleEvent(EVENT_FIERY_COMBUSTION, randtime(15s,18s));
-- events.ScheduleEvent(EVENT_METEOR_STRIKE, 18s) + SendEncounterUnit +
-- controller->AI()->SetData(DATA_FIGHT_PHASE, PHASE_ONE). The five
-- phase-one timer arms port below (cleave / tail-lash / breath /
-- fiery-combustion / meteor-strike), plus the firewall ring-close
-- (HandleGameObject false = READY 1 on both rings, cancelled on combat
-- end by halionCombatEnd's cancelTimers, matching events.Reset); the
-- encounter-frame leg is PORTED in halionEnterCombat (tick 222); the
-- controller leg has no bridge (documented above).
local function halionEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    phaseTwo[guid] = nil
    fightPhaseThree = false
    -- C++ JustEngagedWith base call (boss_halion.cpp:248):
    -- BossAI::JustEngagedWith (ScriptedCreature.cpp:527-539) — the
    -- instance SetBossState(DATA_HALION 3, IN_PROGRESS) leg lands
    -- first, then SetCombatPulseDelay(5) + me-form DoZoneInCombat,
    -- after Talk and phase setup, before the timer schedules (C++
    -- arm order); CheckRequiredBosses / setActive / ScheduleTasks
    -- legs out. The AddAura SPELL_TWILIGHT_PRECISION leg (:249)
    -- follows the base call (creature:AddAura is self-only,
    -- lua_creature_events.go:2416 — state-only in Go's aura map).
    creature:SetBossState(DATA_HALION, 1) -- IN_PROGRESS (EncounterState, InstanceScript.h:68-76)
    creature:SetCombatPulseDelay(5)
    creature:DoZoneInCombat()
    creature:AddAura(SPELL_TWILIGHT_PRECISION)
    schedule(guid, "mat_cleave", {6000, 10000}, function()
        matCleaveTick(creature, guid)
    end)
    schedule(guid, "mat_tail_lash", {7000, 12000}, function()
        matTailLashTick(creature, guid)
    end)
    schedule(guid, "mat_breath", {5000, 15000}, function()
        matBreathTick(creature, guid)
    end)
    schedule(guid, "firewall", 5000, function()
        firewallTick(creature, guid)
    end)
    -- C++ :254-255 — EVENT_FIERY_COMBUSTION randtime(15s,18s),
    -- EVENT_METEOR_STRIKE 18s (see fieryCombustionTick /
    -- meteorStrikeTick above for the phase-one gate).
    schedule(guid, "fiery_combustion", {15000, 18000}, function()
        fieryCombustionTick(creature, guid)
    end)
    schedule(guid, "meteor_strike", 18000, function()
        meteorStrikeTick(creature, guid)
    end)
    -- C++ :257 — SendEncounterUnit(ENCOUNTER_FRAME_ENGAGE, me, 1), after
    -- the timer schedules, before the controller SetData leg (out);
    -- param1 1 = material halion (bridge live 2026-11-05, tick 220).
    creature:SendEncounterUnit(0, creature, 1)
end

local function halionCombatEnd(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    phaseTwo[guid] = nil
    fightPhaseThree = false
end

-- C++ Reset for material halion is the inherited BossAI::Reset ->
-- _Reset() (ScriptedCreature.cpp:499-510): SetCombatPulseDelay(0),
-- events.Reset, summons.DespawnAll, scheduler.CancelAll, then
-- SetBossState(DATA_HALION 3, NOT_STARTED) gated on the stored state
-- != DONE (:509-510), alive-gated (:500-501). Go fires ev23 on evade
-- (triggerCreatureEvade fires 23 then 2, creaturemotion.go), on
-- respawn (fireCreatureSpawned pre-fire), and on the death path
-- before ev4 (fireCreatureDied fires 23 then 4) — the three fires
-- are told apart by the stored state, the skadi / tyrannus /
-- palehoof idiom: the evade fire finds the state IN_PROGRESS and
-- sets FAIL — C++ halion's own EnterEvadeMode (:234-239) calls
-- neither _DespawnAtEvade nor Reset, but it forwards the evade to
-- the controller (BOUNDARY or PHASE_ONE), whose engagement-over
-- JustExitedCombat sets DATA_HALION FAIL, != DONE gated
-- (:619-634); phase-two/three wipes reach the same FAIL via the
-- controller's 5s EVENT_EVADE_CHECK (:614/:755-773). The
-- spawn/respawn fire finds any other non-DONE state and sets
-- NOT_STARTED (the _Reset leg). The death pre-fire is skipped via
-- the IsDead guard (C++ _Reset never runs on death; DONE lands in
-- ev4). NO ev24 handler: C++ has no JustReachedHome FAIL leg for
-- halion (BossAI::_JustReachedHome is setActive(false) only,
-- ScriptedCreature.cpp:522-523; halion has no override), so a
-- reach-home FAIL would fabricate a leg — the FAIL rides this
-- ev23 evade-fire instead.
local function halionReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    phaseTwo[guid] = nil
    fightPhaseThree = false
    if creature:IsDead() then
        return
    end
    creature:SetCombatPulseDelay(0) -- _Reset arm 1 (ScriptedCreature.cpp:502)
    if creature:GetBossState(DATA_HALION) == 1 then
        creature:SetBossState(DATA_HALION, 2) -- FAIL: evade path (controller :619-634)
    elseif creature:GetBossState(DATA_HALION) ~= 3 then
        creature:SetBossState(DATA_HALION, 0) -- NOT_STARTED: _Reset (:509-510), != DONE-gated
    end
end

-- C++ boss_halion::DamageTaken :283-296:
-- Lethal-damage cap (damage = health - 1) applies while NOT in
-- PHASE_THREE (C++ evaluation order: the cap is applied BEFORE the
-- 75% check, so the latch above fires on the killing blow exactly as
-- in C++ — DamageTaken-class, fires on lethal by design); in
-- PHASE_THREE the cap drops and the boss is killable. The phase gate
-- rides the shared fightPhaseThree latch (set by the twilight 50%
-- transition below; the controller SetData(DATA_FIGHT_PHASE) phase
-- machine has no bridge — the gurtogg onChangePhase idiom).
-- Talk(SAY_PHASE_TWO) + the stop of the phase-one timer arms + the
-- DoCastSelf SPELL_TWILIGHT_PHASING 74808 cast port (non-triggered ->
-- creature:CastSpell(creature, SPELL_TWILIGHT_PHASING) — the DoCastSelf
-- lowering, same idiom as the BREATH self-casts); the flag set leg is
-- bridgeable (SetFlag live) but UNPAIRED — its clear rides the
-- controller/instance phase-three action; the CastStop leg is ported
-- (creature:StopSpellCast() before the Phasing cast, 2026-11-05);
-- the controller SetData leg rides the no-cross-AI bridge.
local function halionDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUIDLow()
    -- C++ :283 reads IsInPhase(PHASE_THREE) before the transition
    -- checks, so the phase-crossing blow is still capped.
    local wasPhaseThree = fightPhaseThree
    local health = creature:GetHealth()
    local capped = damage
    if damage >= health and not wasPhaseThree then
        capped = health - 1
    end
    if not phaseTwo[guid] then
        local postHealth = 0
        if health > capped then
            postHealth = health - capped
        end
        -- HealthBelowPctDamaged strict-< floor form (Unit.h:873):
        -- postDamage < floor(maxHealth * 75 / 100). The old
        -- cross-multiplied form fired 1 HP early when maxHealth * 75
        -- mod 100 != 0 (baltharus/anubarak 2026-10-25 precedent).
        if postHealth < math.floor(creature:GetMaxHealth() * 75 / 100) then
            phaseTwo[guid] = true
            creature:Talk(SAY_PHASE_TWO)
            creature:StopSpellCast()
            creature:CastSpell(creature, SPELL_TWILIGHT_PHASING)
            -- The twilight-phasing SpellScript (boss_halion.cpp:1776-1809):
            -- on hit, caster casts Summon Twilight Portal 74809
            -- (triggered, at its position) and map-summons Twilight
            -- Halion 40142 at HalionSpawnPos (3156.67, 533.8108,
            -- 72.98822, 3.159046). The map summon uses no properties,
            -- so C++ defaults to TEMPSUMMON_MANUAL_DESPAWN
            -- (TemporarySummon.cpp:34) = Go SummonCreature type 7,
            -- despawn 0. No AttackStart: C++ has none for this
            -- summon — engagement rides engine aggro. The portal
            -- leg depends on Go executing 74809's DB summon-GO
            -- effect on the self-cast.
            creature:CastSpell(creature, SPELL_SUMMON_TWILIGHT_PORTAL, true)
            creature:SummonCreature(ENTRY_TWILIGHT_HALION, 3156.67, 533.8108, 72.98822, 3.159046, 7, 0)
            cancelTimers(guid)
        end
    end
    -- C++ never-die cap (boss_halion.cpp:283): lethal damage while not
    -- in phase three is clamped to health - 1 (phases 1 AND 2); in
    -- phase three the cap drops and the boss is killable.
    if capped ~= damage then
        return false, capped
    end
end

-- C++ boss_halion::JustDied :263-276:
-- _JustDied() (:265) — events.Reset + summons.DespawnAll +
-- SetBossState(DATA_HALION 3, DONE) (ScriptedCreature.cpp:513-518) —
-- FIRST, before Talk(SAY_DEATH) (:267); SendEncounterUnit
-- DISENGAGE (:268); twilight KillSelf (:270-272) + controller
-- KillSelf (:274-276), each alive-guarded. The KillSelf legs ride
-- creature:Kill (lua_creature_events.go:2971) over the grid scan
-- (the hexlord JustDied precedent — no GUID->creature bridge, and
-- the instance guid store is never latched for these temp summons).
local function halionDied(event, creature, killer)
    cancelTimers(creature:GetGUIDLow())
    fightPhaseThree = false
    creature:SetBossState(DATA_HALION, 3) -- DONE (_JustDied, first arm)
    creature:Talk(SAY_DEATH)
    -- C++ :268 — SendEncounterUnit(ENCOUNTER_FRAME_DISENGAGE, me), after
    -- Talk, before the twilight/controller KillSelf legs.
    creature:SendEncounterUnit(1, creature)
    -- C++ :270-272 — twilight KillSelf if alive; its JustDied runs
    -- nested (halion already dead, so its halion-Kill is skipped;
    -- the controller KillSelf lands inside that call).
    for _, other in ipairs(creature:GetCreaturesInGrid(ENTRY_TWILIGHT_HALION, 500)) do
        if not other:IsDead() then
            other:Kill()
        end
    end
    -- C++ :274-276 — controller KillSelf if alive (no Lua handlers on
    -- 40146, so this only runs the engine death path).
    for _, other in ipairs(creature:GetCreaturesInGrid(ENTRY_HALION_CONTROLLER, 500)) do
        if not other:IsDead() then
            other:Kill()
        end
    end
end

-- C++ boss_twilight_halion::KilledUnit :439-445:
-- if (victim->GetTypeId() == TYPEID_PLAYER)
--     Talk(SAY_KILL) — the razuvious player-gated variant precedent;
-- me->CastSpell(victim, SPELL_LEAVE_TWILIGHT_REALM, true) — UNGATED
-- (fires for any victim type), triggered victim-cast — the arlokk idiom.
-- Material halion has no KilledUnit arm.
local function twilightHalionKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
    creature:CastSpell(victim, SPELL_LEAVE_TWILIGHT_REALM, true)
end

RegisterCreatureEvent(ENTRY_HALION, 1, halionEnterCombat)
RegisterCreatureEvent(ENTRY_HALION, 2, halionCombatEnd)
RegisterCreatureEvent(ENTRY_HALION, 4, halionDied)
RegisterCreatureEvent(ENTRY_HALION, 9, halionDamageTaken)
RegisterCreatureEvent(ENTRY_HALION, 23, halionReset)
RegisterCreatureEvent(ENTRY_TWILIGHT_HALION, 3, twilightHalionKilledUnit)

-- C++ UpdateAI EVENT_CLEAVE: DoCastVictim(SPELL_CLEAVE 74524)
-- non-triggered, 3s init / randtime(7s,10s) re-arm
-- (boss_halion.cpp:525-528) — no phase gating on the arm.
local function cleaveTick(creature, guid)
    creature:CastSpell(nil, SPELL_CLEAVE)
    if creature:IsCasting() then
        schedule(guid, "cleave", 500, function()
            cleaveTick(creature, guid)
        end)
        return
    end
    schedule(guid, "cleave", {7000, 10000}, function()
        cleaveTick(creature, guid)
    end)
end

-- C++ UpdateAI EVENT_BREATH: DoCastSelf(SPELL_DARK_BREATH 74806)
-- non-triggered, 12s init / randtime(10s,14s) re-arm
-- (boss_halion.cpp:533-536) — no phase gating on the arm.
local function breathTick(creature, guid)
    creature:CastSpell(creature, SPELL_DARK_BREATH)
    if creature:IsCasting() then
        schedule(guid, "breath", 500, function()
            breathTick(creature, guid)
        end)
        return
    end
    schedule(guid, "breath", {10000, 14000}, function()
        breathTick(creature, guid)
    end)
end

-- C++ UpdateAI EVENT_TAIL_LASH: DoCastAOE(SPELL_TAIL_LASH 74531)
-- non-triggered, 12s init armed in the constructor (:420) /
-- randtime(12s,16s) re-arm (:529-532) — no phase gating on the arm.
-- The constructor arm is re-anchored on OnEnterCombat: twilight
-- halion spawns in combat (UNIT_FLAG_IN_COMBAT at :414), so the
-- spawn->engage delta on the first tick is near-zero — documented.
local function tailLashTick(creature, guid)
    creature:CastSpell(creature, SPELL_TAIL_LASH)
    if creature:IsCasting() then
        schedule(guid, "tail_lash", 500, function()
            tailLashTick(creature, guid)
        end)
        return
    end
    schedule(guid, "tail_lash", {12000, 16000}, function()
        tailLashTick(creature, guid)
    end)
end

-- C++ boss_twilight_halion::JustEngagedWith:
-- events.SetPhase(PHASE_TWO); BossAI::JustEngagedWith(who);
-- me->AddAura(SPELL_TWILIGHT_PRECISION, me);
-- events.ScheduleEvent(EVENT_CLEAVE, 3s);
-- events.ScheduleEvent(EVENT_BREATH, 12s);
-- instance->SendEncounterUnit(ENCOUNTER_FRAME_ENGAGE, me, 2) — PORTED
-- in twilightHalionEnterCombat below (tick 222).
-- — the four timer arms bridge (the TAIL_LASH arm moves here from
-- its constructor schedule; SOUL_CONSUMPTION's constructor schedule
-- is re-anchored the same way — tailLashTick precedent).
-- C++ boss_twilight_halionAI UpdateAI EVENT_SOUL_CONSUMPTION
-- (:537-540): SelectTarget(Random,1,0.0f,true,true,
-- SPELL_TWILIGHT_REALM 74807) -> CastSpell(target, SPELL_SOUL_CONSUMPTION
-- 74792, TRIGGERED_IGNORE_SET_FACING), 20s re-arm — re-armed
-- unconditionally (outside the target-if — sjonnir precedent). The
-- position-1 offset pops the MaxThreat-ordered first entry before
-- the player/aura filters (UnitAI.h offset-before-filter, eck
-- precedent); the aura filter rides plr:HasAura (player HasAura
-- bridged — engine/world/lua_player.go:203, hakkar precedent); the
-- triggered flag rides the arlokk CastSpell(target, id, true) idiom.
-- C++ SelectTarget(Random,1,0.0f,true,true,SPELL_TWILIGHT_REALM
-- 74807): threat list, position-1 offset pops the current victim
-- BEFORE the player/aura filters (C++ builds the list victim-first,
-- UnitAI.h
-- offset-before-filter; eck precedent), then Player + alive +
-- HasAura(74807). The old GetPlayersInWorld instance pool + victim
-- exclusion could pick players never engaged with the boss
-- (2026-10-25 fix, same class as the obsidian/baltharus pools).
local function randomTwilightPlayer(creature)
    local targets = creature:GetAITargets()
    local victim = creature:GetVictim()
    local victimGuid = nil
    if victim ~= nil then
        victimGuid = victim:GetGUID()
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
                and t:GetObjectType() == "Player" and t:IsAlive()
                and t:HasAura(SPELL_TWILIGHT_REALM) then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

local function soulConsumptionTick(creature, guid)
    local target = randomTwilightPlayer(creature)
    if creature:IsCasting() then
        schedule(guid, "soul_consumption", 500, function()
            soulConsumptionTick(creature, guid)
        end)
        return
    end
    if target then
        creature:CastSpell(target, SPELL_SOUL_CONSUMPTION, true)
    end
    schedule(guid, "soul_consumption", 20000, function()
        soulConsumptionTick(creature, guid)
    end)
end

local twilightEngaged = {}

-- C++ boss_twilight_halion::JustEngagedWith :424-434, shared by the
-- ev1 handler below and the DamageTaken engage fallback (:472-473):
-- events.SetPhase(PHASE_TWO) (:426); BossAI::JustEngagedWith (:428)
-- = SetBossState(DATA_TWILIGHT_HALION 4, IN_PROGRESS) (base-first,
-- ScriptedCreature.cpp:527-539) + SetCombatPulseDelay(5) + me-form
-- DoZoneInCombat, after the phase set, before the timer schedules
-- (C++ arm order); instance/setActive/ScheduleTasks legs out.
-- AddAura SPELL_TWILIGHT_PRECISION (:429) follows the base call
-- (creature:AddAura is self-only, lua_creature_events.go:2416 —
-- state-only in Go's aura map). The four timer arms bridge (the
-- TAIL_LASH arm moves here from its constructor schedule;
-- SOUL_CONSUMPTION's constructor schedule is re-anchored the same
-- way — tailLashTick precedent).
local function twilightEngage(creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    fightPhaseThree = false
    twilightEngaged[guid] = true
    creature:SetBossState(DATA_TWILIGHT_HALION, 1) -- IN_PROGRESS (EncounterState, InstanceScript.h:68-76)
    creature:SetCombatPulseDelay(5)
    creature:DoZoneInCombat()
    creature:AddAura(SPELL_TWILIGHT_PRECISION)
    schedule(guid, "cleave", 3000, function()
        cleaveTick(creature, guid)
    end)
    schedule(guid, "breath", 12000, function()
        breathTick(creature, guid)
    end)
    schedule(guid, "tail_lash", 12000, function()
        tailLashTick(creature, guid)
    end)
    -- C++ ctor schedules EVENT_SOUL_CONSUMPTION at 15s (:421);
    -- re-anchored on OnEnterCombat — twilight halion spawns in combat
    -- (UNIT_FLAG_IN_COMBAT at :414), so the spawn->engage delta on the
    -- first tick is near-zero (tailLashTick precedent).
    schedule(guid, "soul_consumption", 15000, function()
        soulConsumptionTick(creature, guid)
    end)
    -- C++ :433 — SendEncounterUnit(ENCOUNTER_FRAME_ENGAGE, me, 2), after
    -- the timer schedules; param1 2 = twilight halion.
    creature:SendEncounterUnit(0, creature, 2)
end

local function twilightHalionEnterCombat(event, creature, target)
    twilightEngage(creature)
end

local twilightPhaseThree = {}

local function twilightHalionClearPhase(guid)
    twilightPhaseThree[guid] = nil
end

local function twilightHalionLeaveCombat(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    twilightHalionClearPhase(guid)
    twilightEngaged[guid] = nil
    fightPhaseThree = false
end

-- C++ boss_twilight_halion::Reset is EMPTY (boss_halion.cpp:436) —
-- the DoCastSelf(SPELL_CLEAR_DEBUFFS) arm at :586-592 is
-- npc_halion_controllerAI::Reset (entry 40146, unregistered). Event 23
-- only cancels timers.
local function twilightHalionReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    twilightHalionClearPhase(guid)
    twilightEngaged[guid] = nil
    fightPhaseThree = false
end

-- C++ boss_twilight_halion::JustDied (boss_halion.cpp:448-466):
-- NO _JustDied() call — unlike material halion, twilight's JustDied
-- never sets DATA_TWILIGHT_HALION DONE (verified: no SetBossState
-- for DATA_TWILIGHT_HALION anywhere in the Ruby Sanctum files, and
-- the instance SetBossState override only handles
-- BALTHARUS/SAVIANA/ZARITHRIAN/HALION), so ev4 carries NO state
-- leg. Order: halion Kill (:450-457) -> controller KillSelf
-- (:460-462) -> SendEncounterUnit DISENGAGE last (:464). The
-- LowerPlayerDamageReq leg (:454) has no bridge (loot-credit
-- bookkeeping only). The Kill legs ride creature:Kill over the grid
-- scan (hexlord precedent, as in halionDied above).
local function twilightHalionDied(event, creature, killer)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    twilightHalionClearPhase(guid)
    twilightEngaged[guid] = nil
    fightPhaseThree = false
    -- C++ :450-457 — Unit::Kill(killer, halion) if alive; material's
    -- JustDied runs nested (DONE + Talk + disengage; its twilight
    -- KillSelf is skipped — already dead — and its controller
    -- KillSelf lands inside that call).
    for _, other in ipairs(creature:GetCreaturesInGrid(ENTRY_HALION, 500)) do
        if not other:IsDead() then
            other:Kill(killer)
        end
    end
    -- C++ :460-462 — controller KillSelf if alive (no Lua handlers on
    -- 40146, so this only runs the engine death path).
    for _, other in ipairs(creature:GetCreaturesInGrid(ENTRY_HALION_CONTROLLER, 500)) do
        if not other:IsDead() then
            other:Kill()
        end
    end
    -- C++ :464 — SendEncounterUnit(ENCOUNTER_FRAME_DISENGAGE, me), last
    -- in JustDied, after the halion-Kill / controller-KillSelf legs.
    creature:SendEncounterUnit(1, creature)
end

-- C++ boss_twilight_halion::DamageTaken :469-482 (pre-damage event 9 —
-- the zum_rah latch precedent): lethal-damage cap (damage = health -
-- 1) while NOT in PHASE_THREE (C++ evaluation order: the cap is
-- applied BEFORE the 50% check, so the crossing blow is capped and the
-- boss survives it at 1 hp); HealthBelowPctDamaged(50, damage) &&
-- IsInPhase(PHASE_TWO) -> SetPhase(PHASE_THREE); me->CastStop();
-- DoCastSelf(SPELL_TWILIGHT_DIVISION 75063) (non-triggered);
-- Talk(SAY_PHASE_THREE 2). The PHASE_TWO gate is entered in
-- JustEngagedWith (twilightHalionEnterCombat below), so the latch below
-- is the faithful substitute; the transition also sets the shared
-- fightPhaseThree latch that drops both halves' never-die caps (the
-- gurtogg onChangePhase idiom). The CastStop leg is ported
-- (creature:StopSpellCast() before the Division cast, 2026-11-05);
-- the PHASE_THREE
-- damage-routing leg (controller->AI()->SetData
-- (DATA_TWILIGHT_DAMAGE_TAKEN)) rides the no-cross-AI bridge.
local function twilightHalionDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUIDLow()
    -- C++ :469 reads IsInPhase(PHASE_THREE) before the 50% check, so
    -- the phase-crossing blow is still capped.
    local wasPhaseThree = twilightPhaseThree[guid]
    local health = creature:GetHealth()
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 or health == 0 then
        return
    end
    local capped = damage
    if damage >= health and not wasPhaseThree then
        capped = health - 1
    end
    -- C++ :472-473 — twilight spawns with UNIT_FLAG_IN_COMBAT, so
    -- JustEngagedWith may never have fired ("otherwise
    -- JustEngagedWith won't ever be called"); first damage engages
    -- it. The latch keeps this to the one fallback call (ev1 sets it
    -- in twilightEngage); the PHASE_THREE guard matches the C++
    -- !IsInPhase(PHASE_TWO) && !IsInPhase(PHASE_THREE) condition
    -- (PHASE_TWO here ⟺ engaged and not phase-three).
    if not twilightEngaged[guid] and not twilightPhaseThree[guid] then
        twilightEngage(creature)
    end
    -- HealthBelowPctDamaged strict-<, signed, unclamped
    -- (Unit.h:873; the zum_rah precedent) — evaluated on the capped
    -- damage, C++ :469-:475 order.
    -- HealthBelowPctDamaged strict-< floor form (Unit.h:873):
    -- postDamage < floor(maxHealth * 50 / 100), signed, unclamped
    -- (the zum_rah precedent) — evaluated on the capped damage,
    -- C++ :469-:475 order. The old cross-multiplied form fired 1 HP
    -- early when maxHealth * 50 mod 100 != 0 (baltharus precedent).
    if not wasPhaseThree and (health - capped) < math.floor(maxHealth * 50 / 100) then
        twilightPhaseThree[guid] = true
        fightPhaseThree = true
        creature:StopSpellCast()
        creature:CastSpell(creature, SPELL_TWILIGHT_DIVISION)
        creature:Talk(SAY_PHASE_THREE)
    end
    -- C++ never-die cap (boss_halion.cpp:469): lethal damage while not
    -- in phase three is clamped to health - 1 (phases 1 AND 2); in
    -- phase three the cap drops and the boss is killable (the akama /
    -- illidan health-1 precedent).
    if capped ~= damage then
        return false, capped
    end
end

RegisterCreatureEvent(ENTRY_TWILIGHT_HALION, 1, twilightHalionEnterCombat)
RegisterCreatureEvent(ENTRY_TWILIGHT_HALION, 2, twilightHalionLeaveCombat)
RegisterCreatureEvent(ENTRY_TWILIGHT_HALION, 4, twilightHalionDied)
RegisterCreatureEvent(ENTRY_TWILIGHT_HALION, 9, twilightHalionDamageTaken)
RegisterCreatureEvent(ENTRY_TWILIGHT_HALION, 23, twilightHalionReset)

-- C++ npc_living_infernoAI::JustDied (boss_halion.cpp:1292-1296) and
-- npc_living_emberAI::JustDied (:1329-1333): me->DespawnOrUnsummon(1ms)
-- ports as immediate creature:Despawn() (1ms ≈ immediate, bridged).
local function livingAddJustDied(event, creature, killer)
    creature:Despawn()
end

RegisterCreatureEvent(ENTRY_LIVING_INFERNO, 4, livingAddJustDied)
RegisterCreatureEvent(ENTRY_LIVING_EMBER, 4, livingAddJustDied)

-- go_twilight_portal::OnGossipHello (boss_halion.cpp:1370-1375):
-- portals 1/2 cast SPELL_TWILIGHT_REALM 74807 (C++ ctor :1358-1363
-- falls back to 74807 since the WDB template spell is nonexistent);
-- the exit portal casts its goober spellId, which ruby_sanctum.h:109
-- documents as Leave Twilight Realm 74812. C++ returns true (handled)
-- == Lua true (kirtonos polarity). NOT ported: the ctor SetPhaseMask
-- legs (0x20 exit / 0x1 portals — no SetPhaseMask bridge on the GO
-- surface) and the UpdateAI delete leg (:1377-1387 — instance
-- GetBossState gate + no GO-AI UpdateAI fire site, broodlord
-- suppression precedent); without phase masks all three portals are
-- clickable in either phase in Go, but the wrong-realm casts are
-- benign (re-entering / leaving when already there is a no-op on
-- the aura state).
local function onTwilightPortalGossip(_, player, go)
    if go:GetEntry() == GO_HALION_PORTAL_EXIT then
        player:CastSpell(player, SPELL_LEAVE_TWILIGHT_REALM, true)
    else
        player:CastSpell(player, SPELL_TWILIGHT_REALM, true)
    end
    return true
end

RegisterGameObjectGossipEvent(GO_HALION_PORTAL_1, 1, onTwilightPortalGossip)
RegisterGameObjectGossipEvent(GO_HALION_PORTAL_2, 1, onTwilightPortalGossip)
RegisterGameObjectGossipEvent(GO_HALION_PORTAL_EXIT, 1, onTwilightPortalGossip)

-- C++ spell_halion_combustion_consumption::AddMarkStack
-- (boss_halion.cpp:1460-1463): EFFECT_0 SPELL_AURA_PERIODIC_DAMAGE.
-- No PreventDefaultAction, so the Go default damage tick still
-- fires. Each tick the aura target re-casts its mark spell on
-- itself, triggered. The loader is instantiated twice
-- (boss_halion.cpp:1923-1924): "spell_halion_soul_consumption"
-- with SPELL_MARK_OF_CONSUMPTION 74795 on the consumption damage
-- aura 74803, and "spell_halion_fiery_combustion" with SPELL_MARK_
-- OF_COMBUSTION 74567 on the combustion damage aura 74629. The
-- OnApply leg (same self-cast once) and the OnRemove leg (strip
-- the mark unless removed by death) are ported below (ev2/ev3;
-- the old "no apply/remove hooks" claim was stale — the kalecgos
-- port already uses both).
local SPELL_COMBUSTION_DAMAGE_AURA = 74629
local SPELL_MARK_OF_COMBUSTION = 74567
local SPELL_CONSUMPTION_DAMAGE_AURA = 74803
local SPELL_MARK_OF_CONSUMPTION = 74795

local function combustionTick(event, aura, target)
    target:CastSpell(target, SPELL_MARK_OF_COMBUSTION, true)
end

local function consumptionTick(event, aura, target)
    target:CastSpell(target, SPELL_MARK_OF_CONSUMPTION, true)
end

RegisterAuraEvent(SPELL_COMBUSTION_DAMAGE_AURA, 1, combustionTick)
RegisterAuraEvent(SPELL_CONSUMPTION_DAMAGE_AURA, 1, consumptionTick)

-- C++ spell_halion_combustion_consumption::OnApply /
-- OnRemove (boss_halion.cpp:1456-1473): EFFECT_0
-- SPELL_AURA_PERIODIC_DAMAGE. OnApply the target casts its mark
-- spell on itself, triggered. OnRemove, unless the removal is by
-- death, the target's mark is stripped with AURA_REMOVE_BY_EXPIRE —
-- which is what arms the mark's own expire-only OnRemove summon
-- leg below. Same loader pairing as the periodic tick above.

local function combustionDamageOnApply(event, aura, target)
    target:CastSpell(target, SPELL_MARK_OF_COMBUSTION, true)
end

local function combustionDamageOnRemove(event, aura, target)
    if aura:GetRemoveMode() == AURA_REMOVE_BY_DEATH then
        return
    end
    if target:HasAura(SPELL_MARK_OF_COMBUSTION) then
        target:RemoveAura(SPELL_MARK_OF_COMBUSTION, AURA_REMOVE_BY_EXPIRE)
    end
end

local function consumptionDamageOnApply(event, aura, target)
    target:CastSpell(target, SPELL_MARK_OF_CONSUMPTION, true)
end

local function consumptionDamageOnRemove(event, aura, target)
    if aura:GetRemoveMode() == AURA_REMOVE_BY_DEATH then
        return
    end
    if target:HasAura(SPELL_MARK_OF_CONSUMPTION) then
        target:RemoveAura(SPELL_MARK_OF_CONSUMPTION, AURA_REMOVE_BY_EXPIRE)
    end
end

RegisterAuraEvent(SPELL_COMBUSTION_DAMAGE_AURA, 2, combustionDamageOnApply)
RegisterAuraEvent(SPELL_COMBUSTION_DAMAGE_AURA, 3, combustionDamageOnRemove)
RegisterAuraEvent(SPELL_CONSUMPTION_DAMAGE_AURA, 2, consumptionDamageOnApply)
RegisterAuraEvent(SPELL_CONSUMPTION_DAMAGE_AURA, 3, consumptionDamageOnRemove)

-- C++ spell_halion_marks (boss_halion.cpp:1530-1589): the mark auras
-- themselves. BeforeDispel (:1551-1559) zeroes the removed charges
-- (the mark survives the dispel) and strips the matching
-- combustion/consumption debuff with AURA_REMOVE_BY_EXPIRE; OnRemove
-- (:1561-1570) fires only on expiry and casts the summon spell on
-- the target with BP1 = the mark's stack amount. The loader is
-- instantiated twice (boss_halion.cpp:1925-1926):
-- "spell_halion_mark_of_combustion" (summon 74610, remove 74562) and
-- "spell_halion_mark_of_consumption" (summon 74800, remove 74792).
-- DORMANT until Spell.dbc ships (no aura-engine application of
-- 74567/74795 yet); the BeforeDispel veto rides the new event-7
-- bridge (engine/world/lua_aura.go). Residual: the summon cast
-- carries no original-caster slot (Go has none), so it originates
-- from the target rather than the mark's caster.
local SPELL_FIERY_COMBUSTION_SUMMON = 74610 -- C++ enum :70
local SPELL_SOUL_CONSUMPTION_SUMMON = 74800 -- C++ enum :83

local function markOfCombustionBeforeDispel(event, aura, target, dispeller)
    if target:HasAura(SPELL_FIERY_COMBUSTION) then
        target:RemoveAura(SPELL_FIERY_COMBUSTION, AURA_REMOVE_BY_EXPIRE)
    end
    return true
end

local function markOfCombustionOnRemove(event, aura, target)
    if aura:GetRemoveMode() ~= AURA_REMOVE_BY_EXPIRE then
        return
    end
    target:CastCustomSpell(target, SPELL_FIERY_COMBUSTION_SUMMON, false, nil, aura:GetStackAmount())
end

local function markOfConsumptionBeforeDispel(event, aura, target, dispeller)
    if target:HasAura(SPELL_SOUL_CONSUMPTION) then
        target:RemoveAura(SPELL_SOUL_CONSUMPTION, AURA_REMOVE_BY_EXPIRE)
    end
    return true
end

local function markOfConsumptionOnRemove(event, aura, target)
    if aura:GetRemoveMode() ~= AURA_REMOVE_BY_EXPIRE then
        return
    end
    target:CastCustomSpell(target, SPELL_SOUL_CONSUMPTION_SUMMON, false, nil, aura:GetStackAmount())
end

RegisterAuraEvent(SPELL_MARK_OF_COMBUSTION, 7, markOfCombustionBeforeDispel)
RegisterAuraEvent(SPELL_MARK_OF_COMBUSTION, 3, markOfCombustionOnRemove)
RegisterAuraEvent(SPELL_MARK_OF_CONSUMPTION, 7, markOfConsumptionBeforeDispel)
RegisterAuraEvent(SPELL_MARK_OF_CONSUMPTION, 3, markOfConsumptionOnRemove)

-- C++ spell_halion_combustion_consumption_periodic::HandleTick
-- (boss_halion.cpp:1502-1519): EFFECT_0 SPELL_AURA_PERIODIC_TRIGGER_SPELL
-- on SPELL_FIERY_COMBUSTION 74562 / SPELL_SOUL_CONSUMPTION 74792.
-- PreventDefaultAction, so the Go default trigger-spell cast is skipped
-- (return true). The caster (Halion) casts the trigger spell — the
-- aura's own Effects[0].TriggerSpell (DBC-side, read via
-- aura:GetTriggerSpell(0); 0 without Spell.dbc means no cast) — on
-- itself (C++ CastSpell(nullptr, ...) — self-centered area, DoCastAOE
-- convention), with the radius mod from the caster's object scale:
-- scale * pi * 10000 / 3 (C++ int32 truncation). The C++
-- OriginalCaster = caster leg has no Go model (CastCustomSpell
-- precedent) and is moot here — the caster IS the original caster.
-- DORMANT until Spell.dbc ships: the trigger spell id and the
-- 74562/74792 auras themselves are DBC-side.
local function combustionConsumptionTick(event, aura, target)
    local caster = aura:GetCaster()
    if caster == nil then
        return true
    end
    local triggerSpell = aura:GetTriggerSpell(0)
    if triggerSpell == 0 then
        return true
    end
    local radius = math.floor(caster:GetScale() * math.pi * 10000 / 3)
    caster:CastSpellWithRadiusMod(caster, triggerSpell, true, radius)
    return true
end

RegisterAuraEvent(SPELL_FIERY_COMBUSTION, 1, combustionConsumptionTick)
RegisterAuraEvent(SPELL_SOUL_CONSUMPTION, 1, combustionConsumptionTick)
