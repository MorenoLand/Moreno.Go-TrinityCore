-- Razorscale (Ulduar) — Lua port of
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_razorscale.cpp
-- (boss_razorscale (CreatureScript) via
-- RegisterUlduarCreatureAI<boss_razorscale> (BossAI),
-- BOSS_RAZORSCALE = 2 (ulduar.h:38); registered from inside
-- AddSC_boss_razorscale() — the FOURTH group of the "// Ulduar"
-- block (loader decl :113 / call :308 in
-- northrend_script_loader.cpp, verified this run; preceded by
-- AddSC_boss_ignis() decl :112 / call :307; followed by
-- AddSC_boss_xt002() decl :114 / call :309).
-- Entry: 33186 Razorscale (ulduar.h NPC_RAZORSCALE, line 63 —
-- entry-verifiable; the CreatureScript ScriptName binding is
-- DB-side as usual).
-- Talk ids (Says, boss_razorscale.cpp:36-55, verified this run):
-- Razorscale EMOTE_PERMA_GROUND 0, EMOTE_BREATH 1, EMOTE_BERSERK 2
-- (the Razorscale Talk arm uses the Razorscale ids, not the
-- expedition commander/engineer Say ids).
-- Sole-source verified: whole-server-tree grep for each of the
-- twenty script names ("boss_razorscale",
-- "npc_expedition_commander", "npc_expedition_defender",
-- "npc_expedition_trapper", "npc_expedition_engineer",
-- "npc_razorscale_spawner", "npc_darkrune_watcher",
-- "npc_darkrune_guardian", "npc_darkrune_sentinel",
-- "npc_razorscale_harpoon_fire_state",
-- "npc_razorscale_devouring_flame", "go_razorscale_harpoon",
-- "go_razorscale_mole_machine", "spell_razorscale_flame_breath",
-- "spell_razorscale_summon_iron_dwarves",
-- "spell_razorscale_fuse_armor", "spell_razorscale_firebolt",
-- "achievement_iron_dwarf_medium_rare", "achievement_quick_shave")
-- hits boss_razorscale.cpp (+ the loader decl/call lines for
-- AddSC_boss_razorscale) only; zero sql/ hits for all twenty —
-- the audited artifact this run. No second razorscale lua exists.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4
-- OnDied, 23 OnReset. Timer-driven casts ride the CreateLuaEvent
-- convention (baltharus / maiden / auriaya precedent):
-- module-local GUID-keyed timers table, cancelTimers on events
-- 2/4/23, schedule(guid, key, delayMs, fn);
-- creature:CastSpell(nil, spell) = DoCastVictim (2-arg,
-- non-triggered), creature:CastSpell(creature, spell) =
-- DoCastSelf / DoCastAOE, third arg true = triggered,
-- {minMs, maxMs} = randtime re-arm. Melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms; line refs are
-- boss_razorscale.cpp):
-- JustEngagedWith (:356-365) — the BossAI::JustEngagedWith
-- passthrough ports via SetBossState(DATA_RAZORSCALE, IN_PROGRESS)
-- (ScriptedCreature.cpp:527-543, base-first; PORTED 2026-11-05 tick
-- 258 via creature:SetBossState, lua_creature_events.go:5917 — the
-- header never claimed "no instance bridge", the leg was simply
-- unported; the CheckRequiredBosses gate stays out — EnterEvadeMode
-- has NO Lua bridge) + SetCombatPulseDelay + DoZoneInCombat, the
-- SendEncounterUnit engage leg ports via creature:SendEncounterUnit
-- (bridge live 2026-11-05); the ScheduleAirPhaseEvents leg (phase
-- machine) is PORTED 2026-11-08 (tick 453) and the
-- summons.DoAction(ACTION_START_FIGHT) leg is PORTED 2026-11-08
-- (tick 454) as a grid relay to the four expedition entries below;
-- event 1 models the EVENT_BERSERK timer leg (the auriaya
-- engage-port precedent) plus the HandleMusic(true) leg
-- (boss_razorscale.cpp:363) via the new creature:UpdateWorldState
-- bridge (2026-10-22).
-- EVENT_BERSERK (:572-575) — DoCastSelf(SPELL_BERSERK 47008,
-- TRIGGERED) + Talk(EMOTE_BERSERK 2, me), 900s init (:362), no
-- re-arm, no phase gate (scheduled bare in JustEngagedWith, never
-- canceled — the auriaya/ignis triggered-berserk precedent;
-- cast-before-Talk order kept).
-- JustDied (:526-531) — _JustDied() + SendEncounterUnit disengage
-- + HandleMusic(false); no Talk arms; event 4 models the _JustDied
-- SetBossState(DATA_RAZORSCALE, DONE) leg (PORTED 2026-11-05 tick
-- 258, base-first: state before disengage/music) plus the timer
-- wipe plus the HandleMusic(false) leg via
-- creature:UpdateWorldState (2026-10-22) plus the disengage leg
-- (bridge live 2026-11-05). The EnterEvadeMode disengage (:519)
-- stays out: it is gated on the evade reason (boundary evades
-- outside PHASE_PERMA_GROUND return early) and Lua has no
-- evade-reason bridge.
-- Reset (:317-329) — _Reset() SetBossState(DATA_RAZORSCALE,
-- NOT_STARTED) PORTED 2026-11-05 tick 258 on ev23, C++-exact with
-- the _Reset guards (ScriptedCreature.cpp:499-511: early return if
-- !IsAlive; write only if GetBossState != DONE — GetBossState
-- bridged lua_creature_events.go:5936, hakkar precedent); the
-- Initialize engineers=3 + SummonCreatureGroup(1/2/3) + SetCombat-
-- Movement(false) legs were already ported on ev23/ev1; the
-- Initialize SetDisableGravity(true) leg is PORTED 2026-11-08
-- (tick 471, rides ev23).
-- DOCUMENTED-ONLY (in this header; no registration beyond entry
-- 33186):
-- The whole air/ground/perma-ground phase machine — PORTED
-- 2026-11-08 (ticks 453/454): Lua-side RS.phase twin (RS.phase
-- table, phase checks at fire time); DoAction ACTION_START_FIGHT /
-- ACTION_GROUND_PHASE / ACTION_START_PERMA_GROUND (ev38;
-- ACTION_RETURN_TO_BASE is a trapper arm, not a boss arm);
-- MovementInform FLIGHT / TAKEOFF / FLIGHT_2 / LAND / GROUND
-- (ev28; MovePoint / MoveLand / MoveTakeoff legs live);
-- SetCombatMovement(true) rides EVENT_RESUME_MOVE_CHASE.
-- HandleInitialMovement stays out: dead code in C++ (Reset's
-- MovePoint to POINT_START_WAYPOINT is commented out).
-- EVENT_FIREBALL (:576-580) — SelectTarget(Random) + DoCast
-- (SPELL_FIREBALL 63815), re-arm {2s,3s}, PHASE_AIR-gated —
-- PORTED 2026-11-08 (tick 453, RS.fireballTick).
-- EVENT_DEVOURING_FLAME (:581-588) — SelectTarget(Random) +
-- DoCast(SPELL_DEVOURING_FLAME 63236), re-arm {6s,12s} /
-- {10s,12s} — PORTED 2026-11-08 (tick 453,
-- RS.devouringFlameTick).
-- EVENT_SUMMON_MINIONS / EVENT_SUMMON_MINIONS_2 (:589-603) —
-- SummonMinions() (:540-546) SummonCreature(NPC_RAZORSCALE_SPAWNER,
-- frand x/y, 391.517z, TIMED_DESPAWN 15s) — PORTED 2026-11-08
-- (tick 453, RS.summonMinionsTick); the spawner Reset is ported
-- below, its mole-machine/trigger casts stay DBC-gated.
-- EVENT_FLAME_BREATH (:604-609) — Talk(EMOTE_BREATH 1, me) +
-- DoCastVictim(FLAME_BREATH 63317/64021) + EVENT_WING_BUFFET in
-- 2s, PHASE_GROUND-gated 30s one-shot — PORTED 2026-11-08 (tick 453).
-- EVENT_FLAME_BREATH_GROUND (:610-614) — Talk(EMOTE_BREATH 1, me)
-- + DoCastVictim(FLAME_BREATH), re-arm {15s,18s},
-- PHASE_PERMA_GROUND-gated — PORTED 2026-11-08 (tick 453).
-- EVENT_WING_BUFFET (:615-623) — DoCastSelf(SPELL_WING_BUFFET
-- 62666) + EVENT_FIREBOLT in 2s + EVENT_RESUME_AIR_PHASE in 4s —
-- PORTED 2026-11-08 (tick 453, RS.wingBuffetTick).
-- EVENT_RESUME_AIR_PHASE (:624-634) — SetDisableGravity /
-- SetPhase(PHASE_AIR) / MoveTakeoff / engineer
-- ACTION_FIX_HARPOONS DoAction — PORTED 2026-11-08 (tick 453 send
-- leg, tick 454 engineer receiver below); the receiver's spline
-- movement is DB-gated (no script_spline_chain rows in world.db).
-- Boss-side SetReactState arms — PORTED 2026-11-08 (tick 453):
-- MovementInform POINT_RAZORSCALE_FLIGHT_2 ->
-- SetReactState(REACT_AGGRESSIVE) and EVENT_RESUME_AIR_PHASE ->
-- SetReactState(REACT_PASSIVE) both ride the ported phase machine.
-- DamageTaken (:552) -> SetReactState(REACT_AGGRESSIVE) is vacuous: she is
-- already AGGRESSIVE since :448, and the Lua port never sets PASSIVE.
-- EVENT_FIREBOLT (:636-638) — DoCastSelf(SPELL_FIREBOLT 62669)
-- one-shot — PORTED 2026-11-08 (tick 453, RS.fireboltTick).
-- EVENT_FUSE_ARMOR (:639-643) — GetVictim + !HasAura
-- (SPELL_FUSED_ARMOR 64774) gate + DoCast(SPELL_FUSE_ARMOR 64821),
-- re-arm {10s,15s}, PHASE_PERMA_GROUND-gated — PORTED 2026-11-08
-- (tick 453, RS.fuseArmorTick).
-- EVENT_RESUME_MOVE_CHASE (:645-649) — SetCombatMovement(true) +
-- MoveChase(victim) — PORTED 2026-11-08 (tick 453).
-- DamageTaken (:547-555) — HealthBelowPctDamaged(50) +
-- PHASE_GROUND-gated perma-ground latch + DoAction
-- (ACTION_START_PERMA_GROUND) — PORTED 2026-11-08 (tick 453, ev9).
-- SpellHit (:497-506) — SPELL_HARPOON_TRIGGER 62505 counter at
-- RAID_MODE(2, 4) fires DoAction(ACTION_GROUND_PHASE) —
-- PORTED 2026-11-08 (tick 453, ev14 RS.razorscaleSpellHit);
-- the ground-phase downstream is ported, so the counter is live.
-- CanAIAttack / JustSummoned / EnterEvadeMode / SummonMinions —
-- JustSummoned PORTED (ev19 expedition numbers), SummonMinions
-- PORTED (tick 453), EnterEvadeMode disengage PORTED 2026-11-08
-- (tick 471, ev2 — the boundary guard is vacuous in Go, see the
-- leaveCombat comment; summons.DespawnAll stays out, no bridge).
-- CanAIAttack stays out (no hook — vacuous in practice, the
-- expedition adds never enter her threat list). The HandleMusic
-- legs are ported. SummonedCreatureDies ev21 IS bridged — the
-- stale "no bridge" claim is retracted 2026-11-03
-- (amanitar/loatheb/coren precedent); the engineer-death relay
-- is PORTED below.
-- npc_expedition_commander (33210 ulduar.h:90): OnGossipSelect ->
-- DoAction (ACTION_START_FIGHT) is PORTED below (tick 213): the
-- gossip select hook (RegisterCreatureGossipEvent, listID == 0 is
-- the C++ gate — no menuId check in C++) relays via
-- GetCreaturesInGrid to razorscale:DoAction(1), and the ev38
-- receiver arm (RemoveFlag IMMUNE_TO_PC + run speed 3.0 +
-- StopMoving + MoveTo flight position) is PORTED below.
-- DoAction Talk(SAY_COMMANDER_AGGRO 0) / Talk
-- (SAY_COMMANDER_GROUND_PHASE 1) / Talk(SAY_COMMANDER_ENGINEERS_DEAD
-- 2) / ACTION_BUILD_HARPOON_1..4 (18s build timers + GO summons) /
-- ACTION_DESTROY_HARPOONS (_destroy latch + DestroyHarpoons) are all
-- PORTED 2026-11-08 (tick 454) in RS.commanderDoAction below
-- (ENGINEER_DEAD rides the ev21 relay, not a direct Talk).
-- GO_RAZOR_HARPOON / broken-harpoon summon legs PORTED 2026-11-08
-- (tick 454): SummonGameObject IS bridged (lua_creature_events.go:
-- 3707) — BuildHarpoon + BuildBrokenHarpoons below.
-- npc_expedition_defender (33816 ulduar.h:92): Reset one-shot
-- DoCastSelf(SPELL_THREAT 65146) PORTED (latent, below); ctor
-- SetRegenerateHealth(false) PORTED 2026-11-08 (tick 470, rides the
-- ev23 Reset handler — the ctor runs before Reset in C++);
-- ACTION_START_FIGHT -> MovePoint(DefendersPosition[number]) +
-- MovementInform -> SetHomePosition + RemoveFlag IMMUNE_TO_NPC
-- PORTED 2026-11-08 (tick 454) below.
-- npc_expedition_trapper (33259 ulduar.h:93): DoAction machine +
-- MovementInform -> DoCastSelf(SPELL_SHACKLE 62646) PORTED below;
-- ACTION_START_FIGHT -> RemoveFlag IMMUNE_TO_NPC PORTED 2026-11-08
-- (tick 454); ACTION_RETURN_TO_BASE was dead until tick 454
-- (RS.ACTION_RETURN_TO_BASE was never defined — nil comparison).
-- Ctor SetCombatMovement(false) + SetReactState(REACT_PASSIVE)
-- PORTED 2026-11-08 (tick 470, trapperReset ev23 — no Reset
-- override exists, the ctor is the spawn-time arm; Go defaults
-- trappers to AGGRESSIVE, so both legs are load-bearing).
-- npc_expedition_engineer (33287 ulduar.h:91): DoAction machine —
-- Talk(SAY_AGGRO 0) (engineer-east-gated) and Talk
-- (SAY_START_REPAIR 1) + spline-chain harpoon walks + MovementInform
-- commander BUILD_HARPOON relay PORTED 2026-11-08 (tick 454) below;
-- the walks are LATENT (MoveAlongSplineChain bridged,
-- lua_creature_events.go:4791, but no script_spline_chain rows in
-- world.db — the call returns false until the deployment DB ships
-- them). Reset SetReactState(REACT_PASSIVE) PORTED 2026-11-08
-- (tick 470, engineerReset ev23 — Go defaults engineers to
-- AGGRESSIVE, so the leg is load-bearing); the _scheduler.CancelAll
-- leg stays out (the 28s harpoon leg is a raw CreateLuaEvent whose id
-- is discarded at schedule time, so there is no handle to cancel —
-- the leg is IsAlive-guarded at fire time).
-- npc_razorscale_spawner (33245 ulduar.h:89): Reset REACT_PASSIVE +
-- 1s DoCastSelf(SPELL_SUMMON_MOLE_MACHINE 62899) + 6s DoCastSelf
-- (SummonMinionsSpells[urand(0,3)] 63968/63970/63969/63798) PORTED
-- 2026-11-08 (RS.spawnerReset below, ev23); setActive/SetFarVisible
-- have no bridge; the 6s trigger cast is dormant until a SpellScript
-- bridge lands.
-- npc_darkrune_watcher (33453 ulduar.h:88): Reset REACT_PASSIVE +
-- EVENT_START_COMBAT 2s (ev1-scheduled twin) + JustEngagedWith
-- EVENT_LIGHTNING_BOLT (LIGHTNING_BOLT 63809/64696) 5s init, 3s
-- repeat + EVENT_CHAIN_LIGHTNING (64758/64759) 34s init, re-arm
-- {9s,15s} PORTED 2026-11-08 (RS.darkRuneWatcherEngaged below);
-- dormant until the spawner trigger SpellScript lands (63968/63970/
-- 63969 -> 63135) — the AI itself is fully bridged, the old
-- "spawner-machine-blocked / no registration" stay-out is retired.
-- npc_darkrune_guardian (33388 ulduar.h:86): Reset REACT_PASSIVE +
-- EVENT_START_COMBAT 2s (ev1-scheduled twin) + JustEngagedWith
-- EVENT_STORMSTRIKE — DoCastVictim (SPELL_STORMSTRIKE 64757) 23s init,
-- re-arm {13s,25s} PORTED 2026-11-08 (RS.darkRuneGuardianEngaged
-- below); same dormant summon-chain note as the watcher.
-- npc_darkrune_sentinel (33846 ulduar.h:87): Reset REACT_PASSIVE +
-- EVENT_START_COMBAT 2s (ev1-scheduled twin) + JustEngagedWith
-- EVENT_HEROIC_STRIKE — DoCastVictim (SPELL_HEROIC_STRIKE 45026) 9s
-- init, re-arm {5s,9s}; EVENT_BATTLE_SHOUT — DoCastSelf
-- (SPELL_BATTLE_SHOUT 46763) 15s init, 25s repeat; EVENT_WHIRLWIND —
-- DoCastSelf (SPELL_WHIRLWIND 63808) 17s init, re-arm {10s,13s}
-- PORTED 2026-11-08 (RS.darkRuneSentinelEngaged below); same dormant
-- summon-chain note as the watcher.
-- npc_razorscale_harpoon_fire_state (33282 ulduar.h:95):
-- SpellHit(SPELL_FIREBOLT 62669) machine — stale "no bridge"
-- retracted (2026-10-21): ev14 IS bridged (fires from the Go spell
-- path) — PORTED 2026-11-08 (tick 454) below: DoCastSelf(
-- HARPOON_FIRE_STATE 62696) + commander DoAction(
-- ACTION_DESTROY_HARPOONS) via the grid relay; the 62669 trigger
-- itself is spell-data-gated (see the handler comment).
-- npc_razorscale_devouring_flame (34188, difficulty 34189): Reset
-- one-shot DoCastSelf (DEVOURING_FLAME_GROUND 64709/64734,
-- triggered) PORTED 2026-11-08 (tick 470, devouringFlameReset ev23
-- below). Entry verified this tick: AzerothCore creature_template.sql
-- row (34188, 'Razorscale Devouring Flame Stalker', level 83, faction
-- 14) + the 63236 -> 63308 -> 34188 DBC summon chain + the identical
-- struct/registration in both TC and AC boss_razorscale.cpp. AC's row
-- carries ScriptName '' (an AC-DB omission — its C++ still registers
-- the script), so the TDB row itself stays unobserved, but the entry
-- identity is verified and Go keys handlers by entry, not ScriptName.
-- The 25-man template 34189 shares the difficulty-0 ScriptName in TC
-- (ObjectMgr.cpp:966) but Go has no difficulty-entry substitution, so
-- only 34188 is registered; the 64734 pick rides Is25ManRaid, the
-- RAID_MODE twin. ev23 only: Go fires ev23 on the temp-summon path
-- via motionFor/fireCreatureSpawned, and the C++ IsSummonedBy is a
-- no-op — an ev22 twin would double-cast the ground flame. The empty
-- C++ EnterEvadeMode override needs no leg: the stalker is
-- REACT_PASSIVE and never enters combat, so triggerCreatureEvade's
-- wasInCombat gate never fires ev23/ev2 for it. Dormant until the
-- 63236 -> 63308 summon chain lands (spell-data-gated) and Spell.dbc
-- ships 64709/64734.
-- go_razorscale_harpoon / go_razorscale_mole_machine —
-- GameObjectScripts — no registrations; no creature-event
-- handlers fire for GOs. The harpoon Reset arm is PORTED below via
-- GO OnAdd (event 12, dynamic spawns — gameobjects.go:1700); the
-- broken-harpoon RemoveFromWorld leg PORTED below (a4a20ffd; the GO
-- FindNearestGameObject bridge lives at lua_objects.go:1361), and the
-- OnGossipHello arm (:1516-1530) PORTED below: SetFlag +
-- controller:IsCasting() gate (constant-false twin, caststate axiom;
-- double-click race unmodeled, halazzi precedent) + controller
-- CastSpell(nil, entry spell, true).
-- spell_razorscale_flame_breath (63317/64021) — the lethal-hit latch
-- leg is PORTED 2026-11-08 (tick 417, darkRuneGuardianSpellHit below;
-- ev14 post-damage inversion, LATENT until EVENT_FLAME_BREATH ports;
-- fail-closed);
-- spell_razorscale_summon_iron_dwarves (63968/63970/63969),
-- spell_razorscale_fuse_armor (64821), spell_razorscale_firebolt
-- (62669) — no SpellScript / AuraScript bridges; all three join
-- the no-SpellScript / no-AuraScript-bridge queues;
-- achievement_iron_dwarf_medium_rare — PORTED 2026-11-08 (tick 417,
-- latent; Go case reads the killed guardian's ScriptData slot
-- 29232924, fail-closed). achievement_quick_shave — Lua legs PORTED
-- 2026-11-08 (tick 417, razorscaleMovementInform below); flight cycle
-- PORTED 2026-11-08 (tick 453), latch goes live and the Go gate is
-- un-held (achievement_criteria_scripts.go).

local ENTRY_RAZORSCALE = 33186

-- DATA_RAZORSCALE = 2 (ulduar.h:38; BOSS_RAZORSCALE).
local DATA_RAZORSCALE = 2

local ENTRY_EXPEDITION_COMMANDER = 33210
local ENTRY_EXPEDITION_ENGINEER = 33287
local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_IMMUNE_TO_PC = 0x100
local POINT_RAZORSCALE_FLIGHT = 10 -- MovePoints enum: POINT_DEFENDER_ATTACK = 1, +9 to FLIGHT
local SAY_COMMANDER_ENGINEERS_DEAD = 2

local engineersRemaining = {}

local EMOTE_BERSERK = 2

local SPELL_BERSERK = 47008

-- Achievement legs (tick 417): DATA_QUICK_SHAVE 29192921 and
-- DATA_IRON_DWARF_MEDIUM_RARE 29232924 (boss_razorscale.cpp:163-164,
-- Misc enum); SPELL_FLAME_BREATH 63317 / 64021 (:103, RAID_MODE);
-- ENTRY_DARK_RUNE_GUARDIAN 33388 (ulduar.h:86);
-- POINT_RAZORSCALE_FLIGHT_2 12 (MovePoints enum: DEFENDER_ATTACK 1,
-- SHACKLE 2, BASE 3, HARPOON_1 4, HARPOON_1_25 5, HARPOON_2 6,
-- HARPOON_2_25 7, HARPOON_3 8, HARPOON_4 9, FLIGHT 10, TAKEOFF 11,
-- FLIGHT_2 12); POINT_MOTION_TYPE 8 (hadronox precedent).
-- razorscaleFlyCount is the GUID-keyed twin of C++ _flyCount.
local DATA_QUICK_SHAVE = 29192921
local DATA_IRON_DWARF_MEDIUM_RARE = 29232924
local SPELL_FLAME_BREATH = 63317
local SPELL_FLAME_BREATH_25 = 64021
local ENTRY_DARK_RUNE_GUARDIAN = 33388
local POINT_RAZORSCALE_FLIGHT_2 = 12
local POINT_MOTION_TYPE = 8
local razorscaleFlyCount = {}

-- Flight-cycle kit (tick 453): ONE table local (the LK-table
-- precedent) — the file sits at 199 main-chunk locals, so every new
-- constant, per-GUID state table and helper rides RS instead of a
-- fresh top-level local. All C++ refs are boss_razorscale.cpp unless
-- noted. Phases (RazorscalePhases enum :242-249): NONE 0, COMBAT 1,
-- GROUND 2, AIR 3, PERMA_GROUND 4. Points (MovePoints enum :217-234):
-- SHACKLE 2, TAKEOFF 11, FLIGHT_2 12 (top-level local already),
-- LAND 13, GROUND 14. Actions (Actions enum :133-149).
local RS = {
    PHASE_GROUND = 2,
    PHASE_AIR = 3,
    PHASE_PERMA_GROUND = 4,
    POINT_RAZORSCALE_TAKEOFF = 11,
    POINT_RAZORSCALE_LAND = 13,
    POINT_RAZORSCALE_GROUND = 14,
    POINT_SHACKLE_RAZORSCALE = 2,
    EFFECT_MOTION_TYPE = 16,
    SPELL_FIREBALL = 63815,
    SPELL_DEVOURING_FLAME = 63236,
    ENTRY_DEVOURING_FLAME = 34188,
    SPELL_DEVOURING_FLAME_GROUND = 64709,
    SPELL_DEVOURING_FLAME_GROUND_25 = 64734,
    SPELL_WING_BUFFET = 62666,
    SPELL_FIREBOLT = 62669,
    SPELL_FUSE_ARMOR = 64821,
    SPELL_FUSED_ARMOR = 64774,
    SPELL_STUN_SELF = 62794,
    SPELL_HARPOON_TRIGGER = 62505,
    SPELL_SHACKLE = 62646,
    NPC_RAZORSCALE_SPAWNER = 33245,
    ENTRY_EXPEDITION_TRAPPER = 33259,
    ENTRY_EXPEDITION_DEFENDER = 33816,
    ENTRY_RAZORSCALE_CONTROLLER = 33233,
    ACTION_START_FIGHT = 1,
    ACTION_FIX_HARPOONS = 2,
    ACTION_GROUND_PHASE = 3,
    ACTION_ENGINEER_DEAD = 4,
    ACTION_SHACKLE_RAZORSCALE = 5,
    ACTION_START_PERMA_GROUND = 6,
    ACTION_RETURN_TO_BASE = 7,
    ACTION_BUILD_HARPOON_1 = 8,
    ACTION_BUILD_HARPOON_2 = 9,
    ACTION_BUILD_HARPOON_3 = 10,
    ACTION_BUILD_HARPOON_4 = 11,
    ACTION_DESTROY_HARPOONS = 12,
    ACTION_STOP_CONTROLLERS = 13,
    ACTION_STOP_CAST = 14,
    SAY_COMMANDER_AGGRO = 0,
    SAY_COMMANDER_ENGINEERS_DEAD = 2,
    SAY_ENGINEER_AGGRO = 0,
    SAY_ENGINEER_START_REPAIR = 1,
    ENGINEER_NORTH = 0,
    ENGINEER_EAST = 1,
    ENGINEER_WEST = 2,
    SPLINE_CHAIN_MOTION_TYPE = 17,
    POINT_DEFENDER_ATTACK = 1,
    POINT_BASE = 3,
    POINT_HARPOON_1 = 4,
    POINT_HARPOON_1_25 = 5,
    POINT_HARPOON_2 = 6,
    POINT_HARPOON_2_25 = 7,
    POINT_HARPOON_3 = 8,
    POINT_HARPOON_4 = 9,
    EMOTE_STATE_USE_STANDING = 69,
    UNIT_FLAG_IMMUNE_TO_NPC = 0x200,
    ENTRY_HARPOON_FIRE_STATE = 33282,
    SPELL_HARPOON_FIRE_STATE = 62696,
    -- Darkrune kit (tick 455): watcher 33453 / sentinel 33846
    -- (ulduar.h:88/:87); guardian reuses the top-level
    -- ENTRY_DARK_RUNE_GUARDIAN 33388. Spells: spawner :79/:268-274,
    -- watcher RAID_MODE :102-103, guardian :85, sentinel :96-98.
    ENTRY_DARK_RUNE_WATCHER = 33453,
    ENTRY_DARK_RUNE_SENTINEL = 33846,
    SPELL_SUMMON_MOLE_MACHINE = 62899,
    SUMMON_MINIONS_SPELLS = {63968, 63970, 63969, 63798},
    -- Summon-irondwarf trigger fan-out (tick 456): trigger ids :81-83,
    -- summon spells :80/:85 (SPELL_SUMMON_IRON_DWARF_GUARDIAN 62926,
    -- SPELL_SUMMON_IRON_DWARF_WATCHER 63135).
    SPELL_TRIGGER_SUMMON_IRON_DWARVES = 63968,
    SPELL_TRIGGER_SUMMON_IRON_DWARVES_2 = 63970,
    SPELL_TRIGGER_SUMMON_IRON_DWARVES_3 = 63969,
    SPELL_SUMMON_IRON_DWARF_GUARDIAN = 62926,
    SPELL_SUMMON_IRON_DWARF_WATCHER = 63135,
    SPELL_LIGHTNING_BOLT = 63809,
    SPELL_LIGHTNING_BOLT_25 = 64696,
    SPELL_CHAIN_LIGHTNING = 64758,
    SPELL_CHAIN_LIGHTNING_25 = 64759,
    SPELL_STORMSTRIKE = 64757,
    SPELL_HEROIC_STRIKE = 45026,
    SPELL_BATTLE_SHOUT = 46763,
    SPELL_WHIRLWIND = 63808,
    GO_HARPOON_1 = 194542,
    GO_HARPOON_2 = 194541,
    GO_HARPOON_3 = 194543,
    GO_HARPOON_4 = 194519,
    GO_BROKEN_HARPOON = 194565,
    -- Spline-chain ids (EngineersSplineMovements :235-264):
    -- [position+1] = {10-man, 25-man}; H3/H4 are 25-man only;
    -- BASE_25 east (index 2) is 0 — east MovePoints home instead.
    CHAIN_H1 = {{1, 4}, {9, 12}, {16, 19}},
    CHAIN_H2 = {{2, 5}, {10, 13}, {17, 20}},
    CHAIN_H3 = {6, 14, 21},
    CHAIN_H4 = {7, 15, 22},
    CHAIN_BASE_10 = {3, 11, 18},
    CHAIN_BASE_25 = {8, 0, 23},
    DEFENDER_POSITIONS = {
        {624.3065, -154.4163, 391.6442},
        {611.6274, -170.9375, 391.8087},
        {572.1548, -167.4471, 391.8087},
        {558.464, -165.0114, 391.8087},
        {603.3345, -164.4297, 391.8087},
        {549.1727, -159.118, 391.8087},
    },
    POS_HARPOON = {
        {571.9012, -136.5541, 391.5171, 4.921829},
        {589.9233, -133.6223, 391.8968, 4.81711},
        {559.1199, -140.5058, 391.1803, 5.061456},
        {606.2297, -136.7212, 391.1803, 4.537859},
    },
    POS_BROKEN_HARPOON = {
        {571.9465, -136.0118, 391.5171, 2.286379},
        {589.9233, -133.6223, 391.8968, 3.298687},
        {559.1199, -140.5058, 391.1803, 4.049168},
        {606.2297, -136.7212, 391.1803, 5.131269},
    },
    EMOTE_BREATH = 1,
    EMOTE_PERMA_GROUND = 0,
    SAY_COMMANDER_GROUND_PHASE = 1,
    REACT_PASSIVE = 0,
    REACT_AGGRESSIVE = 1,
    RAZORSCALE_LAND = {585.401, -173.543, 408.508},
    RAZORSCALE_GROUND = {585.401, -173.543, 391.6421},
    RAZORSCALE_FLIGHT_2 = {619.145, -238.078, 475.18},
    TRAPPER_POSITIONS = {
        {574.9293, -184.515, 391.8921},
        {539.7838, -178.5337, 391.3053},
        {627.1754, -177.9639, 391.5553},
    },
    phase = {},
    permaGround = {},
    harpoonHits = {},
    summonCounts = {},
    trapperNumber = {},
    defenderNumber = {},
    engineerNumber = {},
    commanderBuilding = {},
    commanderDestroy = {},
    commanderHarpoons = {},
}

-- boss_razorscale.cpp:177
local WORLD_STATE_RAZORSCALE_MUSIC = 4162

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

-- Flight-cycle helpers (tick 453). RS-field fns resolve at call time,
-- so no forward-declare ordering is needed (malown/tharon_ja rule).
-- Phase is a Lua-side twin of the C++ EventMap phase: arms check it at
-- fire time and drop without re-arming out of phase, matching the
-- phase-grouped schedules (:354-381/:620-640). Repeating arms use the
-- 500ms IsCasting defer (berserkTick idiom) for the UpdateAI pre-loop
-- CASTING gate (:565). Random targets ride GetPlayersInRange +
-- math.random (gothik precedent); C++ SelectTarget(Random) reads the
-- threat list, so adds/players off-threat are the standing delta.
RS.cancelKey = function(guid, key)
    local per = timers[guid]
    if per and per[key] then
        RemoveEventById(per[key])
        per[key] = nil
    end
end

RS.randomPlayer = function(creature)
    local players = creature:GetPlayersInRange()
    if #players == 0 then
        return nil
    end
    return players[math.random(#players)]
end

RS.scheduleAirPhaseEvents = function(creature, guid)
    schedule(guid, "fireball", 3000, function() RS.fireballTick(creature, guid) end)
    schedule(guid, "devouring", 9000, function() RS.devouringFlameTick(creature, guid) end)
    schedule(guid, "summonMinions", 1000, function() RS.summonMinionsTick(creature, guid) end)
end

RS.scheduleGroundPhaseEvents = function(creature, guid)
    schedule(guid, "firebolt", 3000, function() RS.fireboltTick(creature, guid) end)
    schedule(guid, "fuseArmor", 15000, function() RS.fuseArmorTick(creature, guid) end)
    schedule(guid, "flameBreathGround", 18000, function() RS.flameBreathGroundTick(creature, guid) end)
    schedule(guid, "devouring", 22000, function() RS.devouringFlameTick(creature, guid) end)
end

-- C++ EVENT_FIREBALL (:583-587): random target, SPELL_FIREBALL 63815,
-- repeat {2s,3s} unconditional.
RS.fireballTick = function(creature, guid)
    if RS.phase[guid] ~= RS.PHASE_AIR then
        return
    end
    if creature:IsCasting() then
        schedule(guid, "fireball", 500, function() RS.fireballTick(creature, guid) end)
        return
    end
    local target = RS.randomPlayer(creature)
    if target then
        creature:CastSpell(target, RS.SPELL_FIREBALL)
    end
    schedule(guid, "fireball", math.random(2000, 3000), function() RS.fireballTick(creature, guid) end)
end

-- C++ EVENT_DEVOURING_FLAME (:588-596): random target, 63236; repeat
-- {6s,12s} in air, {10s,12s} perma-ground.
RS.devouringFlameTick = function(creature, guid)
    local phase = RS.phase[guid]
    if phase ~= RS.PHASE_AIR and phase ~= RS.PHASE_PERMA_GROUND then
        return
    end
    if creature:IsCasting() then
        schedule(guid, "devouring", 500, function() RS.devouringFlameTick(creature, guid) end)
        return
    end
    local target = RS.randomPlayer(creature)
    if target then
        creature:CastSpell(target, RS.SPELL_DEVOURING_FLAME)
    end
    if phase == RS.PHASE_PERMA_GROUND then
        schedule(guid, "devouring", math.random(10000, 12000), function() RS.devouringFlameTick(creature, guid) end)
    else
        schedule(guid, "devouring", math.random(6000, 12000), function() RS.devouringFlameTick(creature, guid) end)
    end
end

-- C++ SummonMinions (:547-553): NPC_RAZORSCALE_SPAWNER 33245 at
-- frand x 540-640, y -230..-195, z 391.517, TEMPSUMMON_TIMED_DESPAWN
-- (2) 15s.
RS.summonMinionsOnce = function(creature)
    local x = 540 + math.random() * 100
    local y = -230 + math.random() * 35
    creature:SummonCreature(RS.NPC_RAZORSCALE_SPAWNER, x, y, 391.517, 0, 2, 15000)
end

-- C++ EVENT_SUMMON_MINIONS (:597-607): 2 spawns (10-man) or urand
-- (2,4) (25-man), staggered 5s + urand(2,5) steps; repeat 40s.
RS.summonMinionsTick = function(creature, guid)
    if RS.phase[guid] ~= RS.PHASE_AIR then
        return
    end
    local count = 2
    if creature:Is25ManRaid() then
        count = math.random(2, 4)
    end
    local delay = 5000
    for i = 1, count do
        schedule(guid, "minions2_" .. i, delay, function() RS.summonMinionsOnce(creature) end)
        delay = delay + math.random(2000, 5000)
    end
    schedule(guid, "summonMinions", 40000, function() RS.summonMinionsTick(creature, guid) end)
end

-- C++ EVENT_FLAME_BREATH (:610-615): RemoveAurasDueToSpell(STUN_SELF),
-- Talk EMOTE_BREATH, DoCastVictim FLAME_BREATH (63317/64021), then
-- EVENT_WING_BUFFET 2s (PHASE_GROUND). One-shot, no repeat.
RS.flameBreathTick = function(creature, guid)
    if RS.phase[guid] ~= RS.PHASE_GROUND then
        return
    end
    if creature:IsCasting() then
        schedule(guid, "flameBreath", 500, function() RS.flameBreathTick(creature, guid) end)
        return
    end
    creature:RemoveAura(RS.SPELL_STUN_SELF)
    creature:Talk(RS.EMOTE_BREATH, creature)
    creature:CastSpell(nil, creature:Is25ManRaid() and SPELL_FLAME_BREATH_25 or SPELL_FLAME_BREATH)
    schedule(guid, "wingBuffet", 2000, function() RS.wingBuffetTick(creature, guid) end)
end

-- C++ EVENT_WING_BUFFET (:620-628): DoCastSelf 62666, EVENT_FIREBOLT
-- 2s + EVENT_RESUME_AIR_PHASE 4s (PHASE_GROUND), trappers
-- DoAction(ACTION_STOP_CAST).
RS.wingBuffetTick = function(creature, guid)
    if RS.phase[guid] ~= RS.PHASE_GROUND then
        return
    end
    if creature:IsCasting() then
        schedule(guid, "wingBuffet", 500, function() RS.wingBuffetTick(creature, guid) end)
        return
    end
    creature:CastSpell(creature, RS.SPELL_WING_BUFFET)
    schedule(guid, "firebolt", 2000, function() RS.fireboltTick(creature, guid) end)
    schedule(guid, "resumeAir", 4000, function() RS.resumeAirTick(creature, guid) end)
    for _, trapper in ipairs(creature:GetCreaturesInGrid(RS.ENTRY_EXPEDITION_TRAPPER, 100)) do
        if trapper:IsAlive() then
            trapper:DoAction(RS.ACTION_STOP_CAST)
        end
    end
end

-- C++ EVENT_RESUME_AIR_PHASE (:630-640): SetDisableGravity(true),
-- phase AIR, REACT_PASSIVE, MoveTakeoff(TAKEOFF, pos.z + 10),
-- engineers DoAction(ACTION_FIX_HARPOONS). The engineer rebuild
-- machine is unported — the relay call is live for its handler.
RS.resumeAirTick = function(creature, guid)
    if RS.phase[guid] ~= RS.PHASE_GROUND then
        return
    end
    creature:SetDisableGravity(true)
    RS.phase[guid] = RS.PHASE_AIR
    creature:SetReactState(RS.REACT_PASSIVE)
    creature:MoveTakeoff(RS.POINT_RAZORSCALE_TAKEOFF, creature:GetX(), creature:GetY(), creature:GetZ() + 10)
    for _, engineer in ipairs(creature:GetCreaturesInGrid(ENTRY_EXPEDITION_ENGINEER, 100)) do
        if engineer:IsAlive() then
            engineer:DoAction(RS.ACTION_FIX_HARPOONS)
        end
    end
end

-- C++ EVENT_FIREBOLT (:642-644): DoCastSelf 62669. One-shot, no
-- repeat — scheduled once per wing-buffet (ground) or per perma-ground
-- schedule.
RS.fireboltTick = function(creature, guid)
    local phase = RS.phase[guid]
    if phase ~= RS.PHASE_GROUND and phase ~= RS.PHASE_PERMA_GROUND then
        return
    end
    if creature:IsCasting() then
        schedule(guid, "firebolt", 500, function() RS.fireboltTick(creature, guid) end)
        return
    end
    creature:CastSpell(creature, RS.SPELL_FIREBOLT)
end

-- C++ EVENT_FUSE_ARMOR (:645-650): victim without FUSED_ARMOR 64774
-- gets FUSE_ARMOR 64821; repeat {10s,15s}.
RS.fuseArmorTick = function(creature, guid)
    if RS.phase[guid] ~= RS.PHASE_PERMA_GROUND then
        return
    end
    if creature:IsCasting() then
        schedule(guid, "fuseArmor", 500, function() RS.fuseArmorTick(creature, guid) end)
        return
    end
    local victim = creature:GetVictim()
    if victim and not victim:HasAura(RS.SPELL_FUSED_ARMOR) then
        creature:CastSpell(victim, RS.SPELL_FUSE_ARMOR)
    end
    schedule(guid, "fuseArmor", math.random(10000, 15000), function() RS.fuseArmorTick(creature, guid) end)
end

-- C++ EVENT_FLAME_BREATH_GROUND (:616-619): Talk EMOTE_BREATH,
-- DoCastVictim FLAME_BREATH, repeat {15s,18s}.
RS.flameBreathGroundTick = function(creature, guid)
    if RS.phase[guid] ~= RS.PHASE_PERMA_GROUND then
        return
    end
    if creature:IsCasting() then
        schedule(guid, "flameBreathGround", 500, function() RS.flameBreathGroundTick(creature, guid) end)
        return
    end
    creature:Talk(RS.EMOTE_BREATH, creature)
    creature:CastSpell(nil, creature:Is25ManRaid() and SPELL_FLAME_BREATH_25 or SPELL_FLAME_BREATH)
    schedule(guid, "flameBreathGround", math.random(15000, 18000), function() RS.flameBreathGroundTick(creature, guid) end)
end

-- C++ EVENT_RESUME_MOVE_CHASE (:651-655): SetCombatMovement(true),
-- MoveChase(victim). One-shot 1ms.
RS.resumeMoveChaseTick = function(creature, guid)
    creature:SetCombatMovement(true)
    local victim = creature:GetVictim()
    if victim then
        creature:MoveChase(victim)
    end
end

-- C++ DoAction ACTION_GROUND_PHASE (:392-400): InterruptNonMeleeSpells,
-- phase GROUND, _harpoonHitCount = 0, SetSpeedRate RUN 3.0, MovePoint
-- LAND (585.401, -173.543, 408.508). Air-phase timers are cancelled —
-- the C++ phase-grouped events stop firing out of phase, and the
-- staggered minions2 spawns must not land while grounded.
RS.groundPhase = function(creature, guid)
    creature:InterruptNonMeleeSpells(false)
    RS.phase[guid] = RS.PHASE_GROUND
    RS.harpoonHits[guid] = 0
    creature:SetSpeedRate(1, 3.0)
    creature:MoveTo(RS.POINT_RAZORSCALE_LAND, RS.RAZORSCALE_LAND[1], RS.RAZORSCALE_LAND[2], RS.RAZORSCALE_LAND[3])
    RS.cancelKey(guid, "fireball")
    RS.cancelKey(guid, "devouring")
    RS.cancelKey(guid, "summonMinions")
    for i = 1, 4 do
        RS.cancelKey(guid, "minions2_" .. i)
    end
end

-- C++ DoAction ACTION_START_PERMA_GROUND (:401-413): SetDisableGravity
-- (false), RemoveAurasDueToSpell(STUN_SELF), Talk EMOTE_PERMA_GROUND,
-- DoCastSelf WING_BUFFET, trappers ACTION_STOP_CAST,
-- EVENT_RESUME_MOVE_CHASE 1ms, ScheduleGroundPhaseEvents. Ground-phase
-- keys are cancelled (phase-grouped in C++); the devouring key is
-- overwritten by the 22s perma schedule via schedule().
RS.startPermaGround = function(creature, guid)
    creature:SetDisableGravity(false)
    creature:RemoveAura(RS.SPELL_STUN_SELF)
    creature:Talk(RS.EMOTE_PERMA_GROUND, creature)
    creature:CastSpell(creature, RS.SPELL_WING_BUFFET)
    for _, trapper in ipairs(creature:GetCreaturesInGrid(RS.ENTRY_EXPEDITION_TRAPPER, 100)) do
        if trapper:IsAlive() then
            trapper:DoAction(RS.ACTION_STOP_CAST)
        end
    end
    RS.cancelKey(guid, "flameBreath")
    RS.cancelKey(guid, "wingBuffet")
    RS.cancelKey(guid, "resumeAir")
    RS.cancelKey(guid, "firebolt")
    schedule(guid, "resumeMoveChase", 1, function() RS.resumeMoveChaseTick(creature, guid) end)
    RS.scheduleGroundPhaseEvents(creature, guid)
end

-- C++ SpellHit (:500-506): SPELL_HARPOON_TRIGGER 62505 increments
-- _harpoonHitCount; at RAID_MODE(2, 4) it fires DoAction
-- (ACTION_GROUND_PHASE). No phase gate in C++ — mirrored exactly.
RS.razorscaleSpellHit = function(event, creature, caster, spellId)
    if spellId ~= RS.SPELL_HARPOON_TRIGGER then
        return
    end
    local guid = creature:GetGUID()
    local count = (RS.harpoonHits[guid] or 0) + 1
    RS.harpoonHits[guid] = count
    local threshold = 2
    if creature:Is25ManRaid() then
        threshold = 4
    end
    if count == threshold then
        RS.groundPhase(creature, guid)
    end
end

-- C++ DamageTaken (:555-563): !_permaGround + HealthBelowPctDamaged
-- (50, damage) + phase GROUND -> _permaGround, REACT_AGGRESSIVE,
-- phase PERMA_GROUND, DoAction(ACTION_START_PERMA_GROUND). DamageTaken
-- arms fire on lethal hits (nefarian-form rule), so no postDamage > 0
-- guard; postDamage = GetHealth() - damage (algalon precedent).
RS.razorscaleDamageTaken = function(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    if RS.permaGround[guid] or RS.phase[guid] ~= RS.PHASE_GROUND then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if creature:GetHealth() - damage < math.floor(maxHealth * 50 / 100) then
        RS.permaGround[guid] = true
        creature:SetReactState(RS.REACT_AGGRESSIVE)
        RS.phase[guid] = RS.PHASE_PERMA_GROUND
        RS.startPermaGround(creature, guid)
    end
end

-- C++ JustSummoned (:482-495): defenders/engineers/trappers get
-- DATA_EXPEDITION_NUMBER from their per-entry counters (first summon
-- gets 0, then the counter increments). All three numbers are consumed
-- now: trapper (shackle position), defender (attack position),
-- engineer (spline-chain position). Group summons reach ev19 via the
-- summon funnel (lua_creature_events.go:190).
RS.razorscaleJustSummoned = function(event, creature, summon)
    local guid = creature:GetGUID()
    local counts = RS.summonCounts[guid]
    if not counts then
        counts = {defender = 0, engineer = 0, trapper = 0}
        RS.summonCounts[guid] = counts
    end
    local entry = summon:GetEntry()
    if entry == RS.ENTRY_EXPEDITION_DEFENDER then
        RS.defenderNumber[summon:GetGUID()] = counts.defender
        counts.defender = counts.defender + 1
    elseif entry == ENTRY_EXPEDITION_ENGINEER then
        RS.engineerNumber[summon:GetGUID()] = counts.engineer
        counts.engineer = counts.engineer + 1
    elseif entry == RS.ENTRY_EXPEDITION_TRAPPER then
        RS.trapperNumber[summon:GetGUID()] = counts.trapper
        counts.trapper = counts.trapper + 1
    end
end

-- C++ npc_expedition_commander::DoAction (:773-810): the _building
-- guard (:773-774) suppresses every action except ACTION_ENGINEER_DEAD
-- while a harpoon build is running; ACTION_START_FIGHT -> Talk(0);
-- ACTION_GROUND_PHASE -> Talk(1); ACTION_ENGINEER_DEAD -> Talk(2) +
-- _events.Reset + _building = false; ACTION_BUILD_HARPOON_1..4 ->
-- _building = true + EVENT_BUILD_HARPOON_N in 18s (builds the GO and
-- clears _building); ACTION_DESTROY_HARPOONS -> _destroy latch +
-- DestroyHarpoons + 10s _destroy reset; ACTION_STOP_CONTROLLERS ->
-- interrupt every NPC_RAZORSCALE_CONTROLLER 33233 in 100 yd (the C++
-- _stopControllers latch is set-then-cleared inside the same call, so
-- no Lua latch is kept).
RS.commanderDoAction = function(event, creature, action)
    local guid = creature:GetGUID()
    if RS.commanderBuilding[guid] and action ~= RS.ACTION_ENGINEER_DEAD then
        return
    end
    if action == RS.ACTION_START_FIGHT then
        creature:Talk(RS.SAY_COMMANDER_AGGRO)
    elseif action == RS.ACTION_GROUND_PHASE then
        creature:Talk(RS.SAY_COMMANDER_GROUND_PHASE)
    elseif action == RS.ACTION_ENGINEER_DEAD then
        creature:Talk(RS.SAY_COMMANDER_ENGINEERS_DEAD)
        cancelTimers(guid)
        RS.commanderBuilding[guid] = false
    elseif action >= RS.ACTION_BUILD_HARPOON_1 and action <= RS.ACTION_BUILD_HARPOON_4 then
        RS.commanderBuilding[guid] = true
        local n = action - RS.ACTION_BUILD_HARPOON_1 + 1
        schedule(guid, "buildHarpoon" .. n, 18000, function()
            RS.commanderBuildHarpoon(creature, guid, n)
        end)
    elseif action == RS.ACTION_DESTROY_HARPOONS then
        if RS.commanderDestroy[guid] then
            return
        end
        RS.commanderDestroy[guid] = true
        RS.commanderDestroyHarpoons(creature, guid)
    elseif action == RS.ACTION_STOP_CONTROLLERS then
        -- C++ :790-795 + HandleControllersStopCast (:717-726):
        -- interrupt every NPC_RAZORSCALE_CONTROLLER 33233 in 100 yd.
        -- The C++ _stopControllers latch is set-then-cleared inside the
        -- same call (re-entrancy guard only), so no Lua latch is kept.
        for _, controller in ipairs(creature:GetCreaturesInGrid(RS.ENTRY_RAZORSCALE_CONTROLLER, 100)) do
            if controller:IsAlive() then
                controller:InterruptNonMeleeSpells(false)
            end
        end
    end
end

-- C++ npc_expedition_trapper::DoAction (:905-935): dead-guard first;
-- ACTION_SHACKLE_RAZORSCALE -> MovePoint(POINT_SHACKLE_RAZORSCALE,
-- TrapperPosition[number]); ACTION_RETURN_TO_BASE -> MoveTargetedHome;
-- ACTION_STOP_CAST -> InterruptNonMeleeSpells + MoveTargetedHome 2s
-- later + commander ACTION_STOP_CONTROLLERS.
RS.trapperDoAction = function(event, creature, action)
    if not creature:IsAlive() then
        return
    end
    if action == RS.ACTION_SHACKLE_RAZORSCALE then
        local pos = RS.TRAPPER_POSITIONS[(RS.trapperNumber[creature:GetGUID()] or 0) + 1]
        creature:MoveTo(RS.POINT_SHACKLE_RAZORSCALE, pos[1], pos[2], pos[3])
    elseif action == RS.ACTION_RETURN_TO_BASE then
        creature:MoveHome()
    elseif action == RS.ACTION_START_FIGHT then
        -- C++ :927-929: RemoveFlag(UNIT_FIELD_FLAGS,
        -- UNIT_FLAG_IMMUNE_TO_NPC).
        creature:RemoveFlag(UNIT_FIELD_FLAGS, RS.UNIT_FLAG_IMMUNE_TO_NPC)
    elseif action == RS.ACTION_STOP_CAST then
        creature:InterruptNonMeleeSpells(false)
        CreateLuaEvent(function()
            if creature:IsAlive() then
                creature:MoveHome()
            end
        end, 2000)
        for _, commander in ipairs(creature:GetCreaturesInGrid(ENTRY_EXPEDITION_COMMANDER, 100)) do
            if commander:IsAlive() then
                commander:DoAction(RS.ACTION_STOP_CONTROLLERS)
                break
            end
        end
    end
end

-- C++ npc_expedition_trapper::MovementInform (:943-949): DoCastSelf
-- (SPELL_SHACKLE 62646) on arrival. C++ gates with && (return only
-- when type is not POINT *and* id is not SHACKLE) — mirrored
-- literally; trappers only ever MovePoint to SHACKLE, so it is
-- equivalent in practice.
RS.trapperMovementInform = function(event, creature, moveType, id)
    if moveType ~= POINT_MOTION_TYPE and id ~= RS.POINT_SHACKLE_RAZORSCALE then
        return
    end
    creature:CastSpell(creature, RS.SPELL_SHACKLE)
end

-- C++ npc_expedition_defender::DoAction (:905-909): ACTION_START_FIGHT
-- -> MovePoint(POINT_DEFENDER_ATTACK, DefendersPosition[number]).
-- No dead-guard in C++ (the boss relay guards instead); the number
-- rides ev19 (RS.defenderNumber).
RS.defenderDoAction = function(event, creature, action)
    if action == RS.ACTION_START_FIGHT then
        local pos = RS.DEFENDER_POSITIONS[(RS.defenderNumber[creature:GetGUID()] or 0) + 1]
        creature:MoveTo(RS.POINT_DEFENDER_ATTACK, pos[1], pos[2], pos[3])
    end
end

-- C++ npc_expedition_defender::MovementInform (:911-917): the C++
-- gate is && (return only when type is not POINT *and* id is not
-- DEFENDER_ATTACK) — mirrored literally; defenders only ever
-- MovePoint to DEFENDER_ATTACK, so it is arrival-only in practice.
-- SetHomePosition takes the position's orientation as 0 (the C++
-- Position has no orientation field); RemoveFlag drops IMMUNE_TO_NPC.
RS.defenderMovementInform = function(event, creature, moveType, id)
    if moveType ~= POINT_MOTION_TYPE and id ~= RS.POINT_DEFENDER_ATTACK then
        return
    end
    local pos = RS.DEFENDER_POSITIONS[(RS.defenderNumber[creature:GetGUID()] or 0) + 1]
    creature:SetHomePosition(pos[1], pos[2], pos[3], 0)
    creature:RemoveFlag(UNIT_FIELD_FLAGS, RS.UNIT_FLAG_IMMUNE_TO_NPC)
end

-- C++ npc_expedition_engineer::DoAction (:984-1005): dead-guard;
-- ACTION_START_FIGHT -> _canUpdateAI (no Lua-visible effect — no
-- engineer UpdateAI port; the 28s leg below is the scheduled work),
-- east Talk SAY_AGGRO, 28s scheduler -> HandleHarpoonMovement +
-- RemoveFlag IMMUNE_TO_NPC; ACTION_FIX_HARPOONS -> east Talk +
-- 28s scheduler -> HandleHarpoonMovement (no flag leg). The C++
-- TaskScheduler only ticks while the engineer is alive and updating,
-- so the Lua leg is IsAlive-guarded at fire time (trapper STOP_CAST
-- idiom). The east Talk rides both receipts (C++ :990/:1001).
RS.engineerDoAction = function(event, creature, action)
    if not creature:IsAlive() then
        return
    end
    if action == RS.ACTION_START_FIGHT or action == RS.ACTION_FIX_HARPOONS then
        if (RS.engineerNumber[creature:GetGUID()] or 0) == RS.ENGINEER_EAST then
            creature:Talk(RS.SAY_ENGINEER_AGGRO)
        end
        CreateLuaEvent(function()
            if not creature:IsAlive() then
                return
            end
            RS.engineerHandleHarpoonMovement(creature)
            if action == RS.ACTION_START_FIGHT then
                creature:RemoveFlag(UNIT_FIELD_FLAGS, RS.UNIT_FLAG_IMMUNE_TO_NPC)
            end
        end, 28000)
    end
end

-- C++ npc_expedition_engineer::HandleHarpoonMovement (:1013-1040):
-- east Talks SAY_START_REPAIR first; the chain id is position x
-- 10/25-man. MoveAlongSplineChain is bridged
-- (lua_creature_events.go:4791, spline_chain.go) but DB-driven:
-- the repo world.db carries no script_spline_chain rows, so the
-- call returns false and the chain stays LATENT until the deployment
-- DB ships the rows — same class as the mole-machine GO leg.
RS.engineerHandleHarpoonMovement = function(creature)
    local pos = (RS.engineerNumber[creature:GetGUID()] or 0) + 1
    if pos == RS.ENGINEER_EAST + 1 then
        creature:Talk(RS.SAY_ENGINEER_START_REPAIR)
    end
    local chain, point
    if creature:Is25ManRaid() then
        chain = RS.CHAIN_H1[pos][2]
        point = RS.POINT_HARPOON_1_25
    else
        chain = RS.CHAIN_H1[pos][1]
        point = RS.POINT_HARPOON_1
    end
    creature:MoveAlongSplineChain(point, chain, false)
end

-- C++ HandleSecondHarpoonMovement (:1042-1066), HandleThird (:1068-
-- 1090), HandleFourth (:1092-1112), HandleBaseMovement (:1114-1136).
-- Third: 25-man -> harpoon 3, 10-man -> base chain. Fourth: 25-man
-- only (10-man is a no-op in C++ — no else branch). Base: 25-man
-- only; north/west walk the base chain, east MovePoints home
-- (GetHomePosition bridge, lua_creature_events.go:6161).
RS.engineerHandleSecondHarpoonMovement = function(creature)
    local pos = (RS.engineerNumber[creature:GetGUID()] or 0) + 1
    local chain, point
    if creature:Is25ManRaid() then
        chain = RS.CHAIN_H2[pos][2]
        point = RS.POINT_HARPOON_2_25
    else
        chain = RS.CHAIN_H2[pos][1]
        point = RS.POINT_HARPOON_2
    end
    creature:MoveAlongSplineChain(point, chain, false)
end

RS.engineerHandleThirdHarpoonMovement = function(creature)
    local pos = (RS.engineerNumber[creature:GetGUID()] or 0) + 1
    if creature:Is25ManRaid() then
        creature:MoveAlongSplineChain(RS.POINT_HARPOON_3, RS.CHAIN_H3[pos], false)
    else
        creature:MoveAlongSplineChain(RS.POINT_BASE, RS.CHAIN_BASE_10[pos], false)
    end
end

RS.engineerHandleFourthHarpoonMovement = function(creature)
    if not creature:Is25ManRaid() then
        return
    end
    local pos = (RS.engineerNumber[creature:GetGUID()] or 0) + 1
    creature:MoveAlongSplineChain(RS.POINT_HARPOON_4, RS.CHAIN_H4[pos], false)
end

RS.engineerHandleBaseMovement = function(creature)
    if not creature:Is25ManRaid() then
        return
    end
    local pos = (RS.engineerNumber[creature:GetGUID()] or 0) + 1
    if pos == RS.ENGINEER_EAST + 1 then
        local hx, hy, hz = creature:GetHomePosition()
        creature:MoveTo(RS.POINT_BASE, hx, hy, hz)
    else
        creature:MoveAlongSplineChain(RS.POINT_BASE, RS.CHAIN_BASE_25[pos], false)
    end
end

-- C++ npc_expedition_engineer::MovementInform (:1168-1218): gate is
-- POINT *or* SPLINE_CHAIN (type 17) — mirrored with or (the trapper
-- handler's &&-literal form is the same gate for a single type).
-- HARPOON_1 -> commander BUILD_HARPOON_1 + 3s EmoteState
-- USE_STANDING (69) + 18s second movement; HARPOON_2 -> BUILD_2 +
-- 18s third; HARPOON_3 -> BUILD_3 + 18s fourth; HARPOON_4 -> BUILD_4
-- + 18s base; BASE -> ChangeOrientation 1ms -> SetFacingTo 4.61684.
-- The 3s/18s legs are TaskScheduler tasks in C++ (alive-gated), so
-- the Lua legs guard IsAlive at fire time.
RS.engineerMovementInform = function(event, creature, moveType, id)
    if moveType ~= POINT_MOTION_TYPE and moveType ~= RS.SPLINE_CHAIN_MOTION_TYPE then
        return
    end
    if id == RS.POINT_HARPOON_1 or id == RS.POINT_HARPOON_1_25 then
        RS.tellCommander(creature, RS.ACTION_BUILD_HARPOON_1)
        CreateLuaEvent(function()
            if creature:IsAlive() then
                creature:EmoteState(RS.EMOTE_STATE_USE_STANDING)
            end
        end, 3000)
        CreateLuaEvent(function()
            if creature:IsAlive() then
                RS.engineerHandleSecondHarpoonMovement(creature)
            end
        end, 18000)
    elseif id == RS.POINT_HARPOON_2 or id == RS.POINT_HARPOON_2_25 then
        RS.tellCommander(creature, RS.ACTION_BUILD_HARPOON_2)
        CreateLuaEvent(function()
            if creature:IsAlive() then
                RS.engineerHandleThirdHarpoonMovement(creature)
            end
        end, 18000)
    elseif id == RS.POINT_HARPOON_3 then
        RS.tellCommander(creature, RS.ACTION_BUILD_HARPOON_3)
        CreateLuaEvent(function()
            if creature:IsAlive() then
                RS.engineerHandleFourthHarpoonMovement(creature)
            end
        end, 18000)
    elseif id == RS.POINT_HARPOON_4 then
        RS.tellCommander(creature, RS.ACTION_BUILD_HARPOON_4)
        CreateLuaEvent(function()
            if creature:IsAlive() then
                RS.engineerHandleBaseMovement(creature)
            end
        end, 18000)
    elseif id == RS.POINT_BASE then
        CreateLuaEvent(function()
            if creature:IsAlive() then
                creature:SetFacingTo(4.61684)
            end
        end, 1)
    end
end

-- C++ commander relay (engineer MovementInform :1177-1180 etc.,
-- harpoon fire-state SpellHit :1454): _instance->GetCreature(
-- DATA_EXPEDITION_COMMANDER) -> DoAction. Grid twin (commanderDoAction
-- precedent above).
RS.tellCommander = function(creature, action)
    for _, commander in ipairs(creature:GetCreaturesInGrid(ENTRY_EXPEDITION_COMMANDER, 100)) do
        if commander:IsAlive() then
            commander:DoAction(action)
            break
        end
    end
end

-- C++ npc_expedition_commander::BuildHarpoon (:734-771): the 25-man
-- mapping is rotated (H1 -> GO_3 at PosHarpoon[3], H2 -> GO_1 at
-- PosHarpoon[1], H3 -> GO_2 at PosHarpoon[2], H4 -> GO_4 at
-- PosHarpoon[4]); 10-man is straight (H1 -> GO_1, H2 -> GO_2).
-- Respawn 7 days (604800s) on 25-man, 0 (persist) on 10-man. The
-- C++ quaternion rotations match the PosHarpoon orientations, so the
-- Lua o arg carries the same facing. GUIDs ride
-- RS.commanderHarpoons for DestroyHarpoons (legoso round-trip).
RS.commanderBuildHarpoon = function(creature, guid, n)
    if not creature:IsAlive() then
        return
    end
    local is25 = creature:Is25ManRaid()
    local entry, pos
    if is25 then
        if n == 1 then
            entry, pos = RS.GO_HARPOON_3, RS.POS_HARPOON[3]
        elseif n == 2 then
            entry, pos = RS.GO_HARPOON_1, RS.POS_HARPOON[1]
        elseif n == 3 then
            entry, pos = RS.GO_HARPOON_2, RS.POS_HARPOON[2]
        else
            entry, pos = RS.GO_HARPOON_4, RS.POS_HARPOON[4]
        end
    else
        if n == 1 then
            entry, pos = RS.GO_HARPOON_1, RS.POS_HARPOON[1]
        elseif n == 2 then
            entry, pos = RS.GO_HARPOON_2, RS.POS_HARPOON[2]
        else
            RS.commanderBuilding[guid] = false
            return
        end
    end
    local goGuid = creature:SummonGameObject(entry, pos[1], pos[2], pos[3], pos[4], is25 and 604800 or 0)
    if goGuid then
        local list = RS.commanderHarpoons[guid]
        if not list then
            list = {}
            RS.commanderHarpoons[guid] = list
        end
        list[#list + 1] = goGuid
    end
    RS.commanderBuilding[guid] = false
end

-- C++ npc_expedition_commander::BuildBrokenHarpoons (:699-706):
-- 2 (10-man) or 4 (25-man) broken harpoons at PosBrokenHarpoon,
-- 7-day respawn. The four C++ calls share one quaternion whose yaw
-- (~4.049) matches only PosBrokenHarpoon[3]'s orientation — Go takes
-- no quaternion, so the Position orientation is passed (documented
-- delta; the harpoon GO's Reset leg removes the nearest broken one
-- when a built harpoon spawns, so the cycle self-balances).
RS.commanderBuildBrokenHarpoons = function(creature)
    local count = creature:Is25ManRaid() and 4 or 2
    for i = 1, count do
        local pos = RS.POS_BROKEN_HARPOON[i]
        creature:SummonGameObject(RS.GO_BROKEN_HARPOON, pos[1], pos[2], pos[3], pos[4], 604800)
    end
end

-- C++ npc_expedition_commander::DestroyHarpoons (:708-715): remove
-- every tracked harpoon GO by GUID (GetGameObjectByGUID bridge,
-- lua_creature_events.go:3780), clear the list, rebuild broken
-- harpoons, 10s EVENT_HANDLE_DESTROY_HARPOON -> _destroy = false.
RS.commanderDestroyHarpoons = function(creature, guid)
    local list = RS.commanderHarpoons[guid]
    if list then
        for _, goGuid in ipairs(list) do
            local go = creature:GetGameObjectByGUID(goGuid)
            if go then
                go:RemoveFromWorld()
            end
        end
        RS.commanderHarpoons[guid] = nil
    end
    RS.commanderBuildBrokenHarpoons(creature)
    schedule(guid, "destroyHarpoonReset", 10000, function()
        RS.commanderDestroy[guid] = false
    end)
end

-- C++ npc_expedition_commander::Reset (:677-682): _events.Reset +
-- phase NONE + BuildBrokenHarpoons. The timer wipe rides
-- cancelTimers (the build/destroy schedules share the module table);
-- the latches reset with it (C++ _building/_destroy are plain
-- members, re-initialised only by the constructor — Reset does NOT
-- clear them, but a reset commander is a fresh group spawn, so the
-- GUID-keyed Lua state is fresh too; the explicit false writes keep
-- a re-fired ev23 on the same GUID from stranding a build).
RS.commanderReset = function(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    RS.commanderBuilding[guid] = false
    RS.commanderDestroy[guid] = false
    RS.commanderBuildBrokenHarpoons(creature)
end

-- C++ npc_razorscale_harpoon_fire_state::SpellHit (:1449-1457):
-- SPELL_FIREBOLT 62669 -> DoCastSelf(SPELL_HARPOON_FIRE_STATE 62696)
-- + commander DoAction(ACTION_DESTROY_HARPOONS). ev14 is bridged;
-- the trigger's live-ness rides the 62669 targeting data (the boss
-- casts firebolt on herself in C++ — the fire-state hit is
-- spell-data-gated, same class as the DBC-gated spell legs).
RS.harpoonFireStateSpellHit = function(event, creature, caster, spellId)
    if spellId ~= RS.SPELL_FIREBOLT then
        return
    end
    creature:CastSpell(creature, RS.SPELL_HARPOON_FIRE_STATE)
    RS.tellCommander(creature, RS.ACTION_DESTROY_HARPOONS)
end

-- Boss UpdateAI CASTING gates (boss_razorscale.cpp pre-loop
-- :565 + in-loop :654) PORTED as a 500ms IsCasting defer in
-- berserkTick (mal_ganis idiom); the darkrune watcher/guardian/
-- sentinel UpdateAI gates guard unported spawner-machine arms only.
-- C++ EVENT_BERSERK (:572-575): DoCastSelf(SPELL_BERSERK 47008,
-- TRIGGERED) + Talk(EMOTE_BERSERK 2, me); 900s init (:362), no
-- re-arm, no phase gate — cast-before-Talk order kept.
local function berserkTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "berserk", 500, function()
            berserkTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_BERSERK, true)
    creature:Talk(EMOTE_BERSERK, creature)
end

-- C++ JustEngagedWith (:356-365): BossAI passthrough +
-- SendEncounterUnit engage + ScheduleAirPhaseEvents (phase
-- machine) + summons DoAction + HandleMusic — the engage leg and
-- the EVENT_BERSERK timer leg are bridgeable.
local function razorscaleEnterCombat(event, creature, target)
    -- C++ JustEngagedWith base-first (boss_razorscale.cpp:358):
    -- BossAI::JustEngagedWith (ScriptedCreature.cpp:527-543) —
    -- SetBossState(DATA_RAZORSCALE, IN_PROGRESS) first, before
    -- SetCombatPulseDelay(5) + me-form DoZoneInCombat; the
    -- CheckRequiredBosses gate stays out (EnterEvadeMode has no
    -- Lua bridge).
    creature:SetBossState(DATA_RAZORSCALE, 1)
    creature:SetCombatPulseDelay(5) -- BossAI::JustEngagedWith arm 1 (ScriptedCreature.cpp:540) -- base-first (boss_razorscale.cpp:358)
    creature:DoZoneInCombat() -- BossAI::JustEngagedWith me-form arm (ScriptedCreature.cpp:542)
    creature:SendEncounterUnit(0, creature) -- ENCOUNTER_FRAME_ENGAGE (boss_razorscale.cpp:359)
    creature:SetCombatMovement(false) -- Reset arm, boss_razorscale.cpp:329 (no AttackStart chase in air phase)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    -- C++ HandleMusic(true) (JustEngagedWith :363) — encounter music on.
    creature:UpdateWorldState(WORLD_STATE_RAZORSCALE_MUSIC, 1)
    local guid = creature:GetGUID()
    RS.phase[guid] = RS.PHASE_AIR
    RS.permaGround[guid] = false
    RS.harpoonHits[guid] = 0
    RS.scheduleAirPhaseEvents(creature, guid)
    -- C++ JustEngagedWith (:361): summons.DoAction(ACTION_START_FIGHT)
    -- to every boss summon (DummyEntryCheckPredicate matches all).
    -- Lua twin: grid relay to the four expedition entries — the only
    -- summons with a START_FIGHT arm (fire-state / devouring-flame /
    -- spawner / darkrune AIs have no DoAction; the relay is range-500
    -- so the air-phase boss height cannot clip it). Dead-guard mirrors
    -- the trapper/engineer DoAction guards; a dead defender would take
    -- no MovePoint in C++ either (its MotionMaster is idle).
    for _, entry in ipairs({RS.ENTRY_EXPEDITION_DEFENDER, RS.ENTRY_EXPEDITION_TRAPPER, ENTRY_EXPEDITION_ENGINEER, ENTRY_EXPEDITION_COMMANDER}) do
        for _, add in ipairs(creature:GetCreaturesInGrid(entry, 500)) do
            if add:IsAlive() then
                add:DoAction(RS.ACTION_START_FIGHT)
            end
        end
    end
    schedule(guid, "berserk", 900000, function()
        berserkTick(creature, guid)
    end)
end

-- C++ Reset (:317-329): _Reset() first (ScriptedCreature.cpp:499-511)
-- — events.Reset (the timer wipe) then SetBossState(DATA_RAZORSCALE,
-- NOT_STARTED), guarded: early return if !IsAlive, write only if
-- GetBossState != DONE (GetBossState bridged
-- lua_creature_events.go:5936, hakkar precedent) — then Initialize
-- (_engineersCount = 3), SummonCreatureGroup(1/2/3) and
-- SetCombatMovement(false) (:329, modeled on ev1 where it persists
-- into the air phase). Event 23 never calls HandleMusic.
local function cancelCombatTimers(event, creature)
    cancelTimers(creature:GetGUID())
    -- C++ _Reset state leg (boss_razorscale.cpp:319, ScriptedCreature.cpp:499-511).
    if creature:IsAlive() and creature:GetBossState(DATA_RAZORSCALE) ~= 3 then
        creature:SetBossState(DATA_RAZORSCALE, 0)
    end
    -- C++ Reset -> Initialize (boss_razorscale.cpp:307/317-319):
    -- _engineersCount = 3.
    engineersRemaining[creature:GetGUID()] = 3
    -- C++ Reset -> Initialize (boss_razorscale.cpp:307/317-319):
    -- _flyCount = 0, so GetData(DATA_QUICK_SHAVE) reads 1 (0 <= 1).
    razorscaleFlyCount[creature:GetGUID()] = 0
    -- C++ Reset -> Initialize (boss_razorscale.cpp:307-314): phase
    -- NONE, _permaGround false, _harpoonHitCount 0, summon counters 0.
    RS.phase[creature:GetGUID()] = 0
    RS.permaGround[creature:GetGUID()] = false
    RS.harpoonHits[creature:GetGUID()] = 0
    RS.summonCounts[creature:GetGUID()] = nil
    -- C++ Reset -> Initialize (boss_razorscale.cpp:314):
    -- SetDisableGravity(true) — the boss hovers again after a wipe;
    -- without it a perma-ground Reset would leave her gravity-bound
    -- into the next air phase. PORTED 2026-11-08 (tick 471, rides
    -- ev23 — the ctor twin fires here too).
    creature:SetDisableGravity(true)
    -- Alive-gated: ev23 fires on death too (fireCreatureDied runs
    -- Reset before the kill-credit criteria read, kill.go:1091/:1400),
    -- and a death-time re-arm would clobber the fight's latch before
    -- the achievement gate reads it. (Gate HELD this tick — the air/
    -- ground machine is unported — so this guards the future unblock.)
    if creature:IsAlive() then
        creature:SetData(DATA_QUICK_SHAVE, 1)
    end
    -- C++ Reset (boss_razorscale.cpp:322-325): SummonCreatureGroup(
    -- RAZORSCALE_EXPEDITION_GROUP 1) + SummonCreatureGroup(
    -- RAZORSCALE_FIRE_STATE_10_GROUP 2) + 25-man SummonCreatureGroup(
    -- RAZORSCALE_FIRE_STATE_25_GROUP 3) — group coords DB-side;
    -- creature:SummonCreatureGroup bridged 2026-11-05 (tick 238).
    creature:SummonCreatureGroup(1)
    creature:SummonCreatureGroup(2)
    if creature:Is25ManRaid() then
        creature:SummonCreatureGroup(3)
    end
end

-- C++ EnterEvadeMode (:514-523) handler (event 2): the disengage
-- (:519) + HandleMusic(false) (:521) legs — no SetBossState leg in
-- the evade path (the state returns to NOT_STARTED via Reset/ev23).
-- The boundary early-return needs no evade-reason bridge: Go fires
-- ev2 only from triggerCreatureEvade (creaturemotion.go:4699), and
-- the leash (boundary) evade is gated !inDungeon (creaturemotion.go:
-- 5918) — Ulduar is a dungeon, so every Go evade here is the
-- no-hostiles kind, for which C++ runs both legs.
-- summons.DespawnAll (:520) stays out (no Lua bridge).
-- Split from the died handler 2026-11-05 tick 258 (ev2 leave-combat
-- does NOT call Reset in C++ — sjonnir precedent).
local function leaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
    -- C++ SendEncounterUnit disengage (EnterEvadeMode :519).
    creature:SendEncounterUnit(1, creature)
    -- C++ HandleMusic(false) (EnterEvadeMode :521).
    creature:UpdateWorldState(WORLD_STATE_RAZORSCALE_MUSIC, 0)
end

-- C++ JustDied (:526-531): _JustDied() first (ScriptedCreature.cpp:513-519
-- — SetBossState(DATA_RAZORSCALE, DONE)), then SendEncounterUnit
-- disengage (:529) + HandleMusic(false) (:530); no Talk arms.
local function razorscaleDied(event, creature)
    creature:SetBossState(DATA_RAZORSCALE, 3)
    cancelTimers(creature:GetGUID())
    creature:SendEncounterUnit(1, creature) -- ENCOUNTER_FRAME_DISENGAGE (boss_razorscale.cpp:529)
    -- C++ HandleMusic(false) (JustDied :530).
    creature:UpdateWorldState(WORLD_STATE_RAZORSCALE_MUSIC, 0)
end

-- C++ boss_razorscale::SummonedCreatureDies (:486-494): an engineer
-- death decrements _engineersCount (Initialize :307 = 3); at 0 the
-- commander gets DoAction(ACTION_ENGINEER_DEAD) = Talk(2) +
-- _events.Reset + _building=false (the commander ev38 arm above —
-- the relay, not a direct Talk, so the build-timer reset rides it).
-- The stale "no SummonedCreatureDies bridge" header claim is retracted
-- 2026-11-03: ev21 IS bridged (amanitar/loatheb/coren precedent; args
-- are event, creature, summon, killer). The fire-once guard mirrors
-- C++: past 0 the counter wraps to 255 and never refires.
local function razorscaleSummonedCreatureDies(event, creature, summon, killer)
    if summon:GetEntry() ~= ENTRY_EXPEDITION_ENGINEER then
        return
    end
    local guid = creature:GetGUID()
    local remaining = engineersRemaining[guid] or 3
    if remaining > 0 then
        remaining = remaining - 1
        engineersRemaining[guid] = remaining
        if remaining == 0 then
            for _, commander in ipairs(creature:GetCreaturesInGrid(ENTRY_EXPEDITION_COMMANDER, 100)) do
                if commander:IsAlive() then
                    commander:DoAction(RS.ACTION_ENGINEER_DEAD)
                    break
                end
            end
        end
    end
end

-- C++ npc_expedition_commander::OnGossipSelect
-- (boss_razorscale.cpp:684-696): gossipListId ==
-- GOSSIP_START_ENCOUNTER (0) -> CloseGossipMenuFor,
-- _events.SetPhase(PHASE_COMBAT) (commander phase machine, stays
-- out), RemoveFlag(UNIT_NPC_FLAGS, UNIT_NPC_FLAG_GOSSIP),
-- instance GetCreature(BOSS_RAZORSCALE) ->
-- DoAction(ACTION_START_FIGHT = 1). The menu has a single option,
-- so listID == 0 is the C++ gate (no menuId check in C++; the
-- creature select hook forwards listID as the 7th arg, gossip.go).
-- The commander reaches razorscale via GetCreaturesInGrid (same
-- arena; the boss Lua already reaches the commander the other way
-- at 100 yd above). Returns true when handled (C++ returns true),
-- false otherwise (majordomo/nefarian convention).
local function onCommanderGossipSelect(event, player, creature, sender, action, code, listID)
    if listID ~= 0 then
        return false
    end
    player:GossipClearMenu()
    player:GossipComplete()
    creature:RemoveFlag(82, 1) -- UNIT_NPC_FLAGS, UNIT_NPC_FLAG_GOSSIP (majordomo precedent)
    for _, razorscale in ipairs(creature:GetCreaturesInGrid(ENTRY_RAZORSCALE, 500)) do
        if razorscale:IsAlive() then
            razorscale:DoAction(1) -- ACTION_START_FIGHT
            break
        end
    end
    return true
end

-- C++ boss_razorscaleAI::DoAction ACTION_START_FIGHT
-- (boss_razorscale.cpp:381-391): RemoveFlag(UNIT_FIELD_FLAGS,
-- UNIT_FLAG_IMMUNE_TO_PC), SetSpeedRate(MOVE_RUN, 3.0f),
-- StopMoving, MovePoint(POINT_RAZORSCALE_FLIGHT,
-- RazorFlightPosition 585.3610, -173.5592, 456.8430, 1.526665).
-- MoveTo takes (id, x, y, z) only — no orientation arg.
local function razorscaleDoAction(event, creature, action)
    if action == 1 then -- ACTION_START_FIGHT
        creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC)
        creature:SetSpeedRate(1, 3.0) -- MOVE_RUN
        creature:StopMoving()
        creature:MoveTo(POINT_RAZORSCALE_FLIGHT, 585.361, -173.5592, 456.843)
    elseif action == RS.ACTION_GROUND_PHASE then -- ACTION_GROUND_PHASE
        RS.groundPhase(creature, creature:GetGUID())
    elseif action == RS.ACTION_START_PERMA_GROUND then -- ACTION_START_PERMA_GROUND
        RS.startPermaGround(creature, creature:GetGUID())
    end
end

-- C++ boss_razorscaleAI::MovementInform (boss_razorscale.cpp:440-475):
-- accepts POINT_MOTION_TYPE (8) and EFFECT_MOTION_TYPE (16) — MoveLand
-- and MoveTakeoff are GenericMovementGenerator EFFECT legs
-- (MotionMaster.cpp:674-696), MovePoint legs are POINT. FLIGHT_2
-- arrival ++_flyCount; GetData(DATA_QUICK_SHAVE) returns 1 iff
-- _flyCount <= 1 (:507-512). ev28 fires on Lua MoveTo/MoveLand/
-- MoveTakeoff arrival with (type, id). LIVE this tick: the air/ground
-- cycle is ported, so the latch can flip and the Go gate is un-held.
local function razorscaleMovementInform(event, creature, moveType, id)
    if moveType ~= POINT_MOTION_TYPE and moveType ~= RS.EFFECT_MOTION_TYPE then
        return
    end
    local guid = creature:GetGUID()
    if id == POINT_RAZORSCALE_FLIGHT then
        -- C++ :443-448: UpdateSpeed (no bridge — the run rate was
        -- already applied by SetSpeedRate in ACTION_START_FIGHT),
        -- SetFacingTo, DoZoneInCombat.
        creature:SetFacingTo(1.526665)
        creature:DoZoneInCombat()
    elseif id == RS.POINT_RAZORSCALE_TAKEOFF then
        -- C++ :468-471: SetSpeedRate RUN 3.0, MovePoint FLIGHT_2.
        creature:SetSpeedRate(1, 3.0)
        creature:MoveTo(POINT_RAZORSCALE_FLIGHT_2, RS.RAZORSCALE_FLIGHT_2[1], RS.RAZORSCALE_FLIGHT_2[2], RS.RAZORSCALE_FLIGHT_2[3])
    elseif id == POINT_RAZORSCALE_FLIGHT_2 then
        -- C++ :472-478: SetFacingTo, SetReactState AGGRESSIVE,
        -- ScheduleAirPhaseEvents, ++_flyCount; UpdateSpeed skipped
        -- (no bridge).
        creature:SetFacingTo(1.423917)
        creature:SetReactState(RS.REACT_AGGRESSIVE)
        RS.scheduleAirPhaseEvents(creature, guid)
        local count = (razorscaleFlyCount[guid] or 0) + 1
        razorscaleFlyCount[guid] = count
        if count > 1 then
            creature:SetData(DATA_QUICK_SHAVE, 0)
        end
    elseif id == RS.POINT_RAZORSCALE_LAND then
        -- C++ :479-483: UpdateSpeed skipped, SetFacingTo, MoveLand
        -- GROUND (new MoveLand bridge — EFFECT_MOTION_TYPE arrival).
        creature:SetFacingTo(1.570796)
        creature:MoveLand(RS.POINT_RAZORSCALE_GROUND, RS.RAZORSCALE_GROUND[1], RS.RAZORSCALE_GROUND[2], RS.RAZORSCALE_GROUND[3])
    elseif id == RS.POINT_RAZORSCALE_GROUND then
        -- C++ :484-496: SetDisableGravity(false); if !_permaGround:
        -- STUN_SELF, trappers shackle, commander ground-phase Talk,
        -- EVENT_FLAME_BREATH 30s (PHASE_GROUND).
        creature:SetDisableGravity(false)
        if not RS.permaGround[guid] then
            creature:CastSpell(creature, RS.SPELL_STUN_SELF, true)
            for _, trapper in ipairs(creature:GetCreaturesInGrid(RS.ENTRY_EXPEDITION_TRAPPER, 100)) do
                if trapper:IsAlive() then
                    trapper:DoAction(RS.ACTION_SHACKLE_RAZORSCALE)
                end
            end
            for _, commander in ipairs(creature:GetCreaturesInGrid(ENTRY_EXPEDITION_COMMANDER, 100)) do
                if commander:IsAlive() then
                    commander:DoAction(RS.ACTION_GROUND_PHASE)
                    break
                end
            end
            schedule(guid, "flameBreath", 30000, function() RS.flameBreathTick(creature, guid) end)
        end
    end
end

-- C++ spell_razorscale_flame_breath::CheckDamage
-- (boss_razorscale.cpp:1583-1600): a flame-breath hit that kills a Dark
-- Rune Guardian (GetHitDamage() >= target health, target alive at
-- script time) sets the guardian's DATA_IRON_DWARF_MEDIUM_RARE latch.
-- ev14 fires on the hit target AFTER the damage loop (boss_ai.go:
-- DoDamageAndTriggers comment), so the Lua twin inverts the check:
-- lethal ⟺ the guardian is dead (or at 0 health) at handler time.
-- LIVE since tick 453: EVENT_FLAME_BREATH / FLAME_BREATH_GROUND are
-- ported, so breath casts reach guardians; the latch defaults false,
-- so the Go gate stays fail-closed until a lethal breath lands.
local function darkRuneGuardianSpellHit(event, creature, caster, spellId)
    if spellId ~= SPELL_FLAME_BREATH and spellId ~= SPELL_FLAME_BREATH_25 then
        return
    end
    if not creature:IsAlive() or creature:GetHealth() == 0 then
        creature:SetData(DATA_IRON_DWARF_MEDIUM_RARE, 1)
    end
end

RegisterCreatureEvent(ENTRY_RAZORSCALE, 1, razorscaleEnterCombat)
RegisterCreatureEvent(ENTRY_RAZORSCALE, 2, leaveCombat)
RegisterCreatureEvent(ENTRY_RAZORSCALE, 4, razorscaleDied)
RegisterCreatureEvent(ENTRY_RAZORSCALE, 23, cancelCombatTimers)
RegisterCreatureEvent(ENTRY_RAZORSCALE, 21, razorscaleSummonedCreatureDies)
RegisterCreatureEvent(ENTRY_RAZORSCALE, 38, razorscaleDoAction)
RegisterCreatureEvent(ENTRY_RAZORSCALE, 28, razorscaleMovementInform)
RegisterCreatureEvent(ENTRY_RAZORSCALE, 14, RS.razorscaleSpellHit)
RegisterCreatureEvent(ENTRY_RAZORSCALE, 9, RS.razorscaleDamageTaken)
RegisterCreatureEvent(ENTRY_RAZORSCALE, 19, RS.razorscaleJustSummoned)
RegisterCreatureEvent(ENTRY_EXPEDITION_COMMANDER, 38, RS.commanderDoAction)
RegisterCreatureEvent(ENTRY_EXPEDITION_COMMANDER, 23, RS.commanderReset)
RegisterCreatureEvent(RS.ENTRY_EXPEDITION_TRAPPER, 38, RS.trapperDoAction)
RegisterCreatureEvent(RS.ENTRY_EXPEDITION_TRAPPER, 28, RS.trapperMovementInform)
RegisterCreatureEvent(RS.ENTRY_EXPEDITION_DEFENDER, 38, RS.defenderDoAction)
RegisterCreatureEvent(RS.ENTRY_EXPEDITION_DEFENDER, 28, RS.defenderMovementInform)
RegisterCreatureEvent(ENTRY_EXPEDITION_ENGINEER, 38, RS.engineerDoAction)
RegisterCreatureEvent(ENTRY_EXPEDITION_ENGINEER, 28, RS.engineerMovementInform)
RegisterCreatureEvent(RS.ENTRY_HARPOON_FIRE_STATE, 14, RS.harpoonFireStateSpellHit)
RegisterCreatureEvent(ENTRY_DARK_RUNE_GUARDIAN, 14, darkRuneGuardianSpellHit)

-- C++ npc_razorscale_spawner::Reset (boss_razorscale.cpp:1227-1243):
-- setActive(true) + SetFarVisible(true) (no bridge) + REACT_PASSIVE +
-- 1s DoCastSelf(SPELL_SUMMON_MOLE_MACHINE 62899) + 6s DoCastSelf(
-- SummonMinionsSpells[urand(0,3)] :268-274 — 63968/63970/63969 trigger
-- spells + 63798 vrykul trigger). The spawner arrives via the ported
-- boss summon leg (RS.summonMinionsOnce, TEMPSUMMON_TIMED_DESPAWN 15s),
-- so this Reset is live: the mole-machine cast summons the GO via its
-- DBC effect (GO leg ported below via event 12) and the 6s trigger
-- cast is dormant until a SpellScript bridge lands (its script effect
-- casts 62926/63135). PORTED 2026-11-08.
RS.spawnerReset = function(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:SetReactState(RS.REACT_PASSIVE)
    schedule(guid, "mole_machine", 1000, function()
        creature:CastSpell(creature, RS.SPELL_SUMMON_MOLE_MACHINE)
    end)
    schedule(guid, "summon_minions", 6000, function()
        creature:CastSpell(creature, RS.SUMMON_MINIONS_SPELLS[math.random(1, 4)])
    end)
end

-- C++ darkrune EVENT_START_COMBAT (watcher :1284-1286, guardian
-- :1355-1357, sentinel :1408-1410 — identical bodies):
-- REACT_AGGRESSIVE + DoZoneInCombat. Reset schedules it 2s out, but
-- the C++ EventMap only ticks once UpdateVictim() succeeds, so the 2s
-- runs from first victim acquisition — the Lua twin schedules it from
-- ev1 (JustEngagedWith), never from Reset, so an untouched add stays
-- passive. The shared Reset below is the identical C++ body for all
-- three AIs (REACT_PASSIVE + EVENT_START_COMBAT 2s; the
-- razorscale->AI()->JustSummoned(me) leg is a no-op — boss
-- JustSummoned :463-479 is BossAI::JustSummoned + expedition-only
-- SetData, darkrune entries hit default). PORTED 2026-11-08.
RS.darkRuneReset = function(event, creature)
    cancelTimers(creature:GetGUID())
    creature:SetReactState(RS.REACT_PASSIVE)
end

RS.darkRuneLeaveCombat = function(event, creature)
    cancelTimers(creature:GetGUID())
end

RS.darkRuneStartCombat = function(creature, guid)
    creature:SetReactState(RS.REACT_AGGRESSIVE)
    creature:DoZoneInCombat()
end

-- C++ npc_darkrune_watcher (boss_razorscale.cpp:1251-1310, entry 33453
-- ulduar.h:88): JustEngagedWith (:1264-1268) EVENT_LIGHTNING_BOLT 5s +
-- EVENT_CHAIN_LIGHTNING 34s. EVENT_LIGHTNING_BOLT (:1288-1290):
-- DoCastVictim(LIGHTNING_BOLT 63809/64696 RAID_MODE :102-103), repeat
-- 3s. EVENT_CHAIN_LIGHTNING (:1292-1294): DoCastVictim(CHAIN_LIGHTNING
-- 64758/64759), re-arm {9s,15s}. UpdateAI pre-loop CASTING gate rides
-- the 500ms IsCasting defer (mal_ganis idiom); melee is engine-side.
-- DORMANT until the spawner trigger SpellScript lands (63968/63970/
-- 63969 -> 63135); the AI itself is fully bridged. PORTED 2026-11-08.
RS.darkRuneWatcherEngaged = function(event, creature, target)
    local guid = creature:GetGUID()
    schedule(guid, "start_combat", 2000, function() RS.darkRuneStartCombat(creature, guid) end)
    schedule(guid, "lightning_bolt", 5000, function() RS.darkRuneWatcherLightningBolt(creature, guid) end)
    schedule(guid, "chain_lightning", 34000, function() RS.darkRuneWatcherChainLightning(creature, guid) end)
end

RS.darkRuneWatcherLightningBolt = function(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "lightning_bolt", 500, function() RS.darkRuneWatcherLightningBolt(creature, guid) end)
        return
    end
    creature:CastSpell(nil, creature:Is25ManRaid() and RS.SPELL_LIGHTNING_BOLT_25 or RS.SPELL_LIGHTNING_BOLT)
    schedule(guid, "lightning_bolt", 3000, function() RS.darkRuneWatcherLightningBolt(creature, guid) end)
end

RS.darkRuneWatcherChainLightning = function(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "chain_lightning", 500, function() RS.darkRuneWatcherChainLightning(creature, guid) end)
        return
    end
    creature:CastSpell(nil, creature:Is25ManRaid() and RS.SPELL_CHAIN_LIGHTNING_25 or RS.SPELL_CHAIN_LIGHTNING)
    schedule(guid, "chain_lightning", math.random(9000, 15000), function() RS.darkRuneWatcherChainLightning(creature, guid) end)
end

-- C++ npc_darkrune_guardian (boss_razorscale.cpp:1312-1378, entry 33388
-- ulduar.h:86): JustEngagedWith (:1324-1327) EVENT_STORMSTRIKE 23s.
-- EVENT_STORMSTRIKE (:1359-1361): DoCastVictim(SPELL_STORMSTRIKE
-- 64757 :85), re-arm {13s,25s}. GetData/SetData
-- DATA_IRON_DWARF_MEDIUM_RARE (:1330-1339) need no leg here: the
-- SetData writer is darkRuneGuardianSpellHit above (flame-breath
-- lethal-hit latch, tick 417) and the GetData reader is the Go
-- achievement case (ScriptData slot 29232924). Same dormant
-- summon-chain note as the watcher. PORTED 2026-11-08.
RS.darkRuneGuardianEngaged = function(event, creature, target)
    local guid = creature:GetGUID()
    schedule(guid, "start_combat", 2000, function() RS.darkRuneStartCombat(creature, guid) end)
    schedule(guid, "stormstrike", 23000, function() RS.darkRuneGuardianStormstrike(creature, guid) end)
end

RS.darkRuneGuardianStormstrike = function(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "stormstrike", 500, function() RS.darkRuneGuardianStormstrike(creature, guid) end)
        return
    end
    creature:CastSpell(nil, RS.SPELL_STORMSTRIKE)
    schedule(guid, "stormstrike", math.random(13000, 25000), function() RS.darkRuneGuardianStormstrike(creature, guid) end)
end

-- C++ npc_darkrune_sentinel (boss_razorscale.cpp:1380-1445, entry 33846
-- ulduar.h:87): JustEngagedWith (:1392-1397) EVENT_HEROIC_STRIKE 9s +
-- EVENT_BATTLE_SHOUT 15s + EVENT_WHIRLWIND 17s. EVENT_HEROIC_STRIKE
-- (:1411-1414): DoCastVictim(SPELL_HEROIC_STRIKE 45026 :97), re-arm
-- {5s,9s}. EVENT_BATTLE_SHOUT (:1415-1418): DoCastSelf(
-- SPELL_BATTLE_SHOUT 46763 :96), repeat 25s. EVENT_WHIRLWIND
-- (:1419-1422): DoCastSelf(SPELL_WHIRLWIND 63808 :98), re-arm
-- {10s,13s}. Same dormant summon-chain note as the watcher.
-- PORTED 2026-11-08.
RS.darkRuneSentinelEngaged = function(event, creature, target)
    local guid = creature:GetGUID()
    schedule(guid, "start_combat", 2000, function() RS.darkRuneStartCombat(creature, guid) end)
    schedule(guid, "heroic_strike", 9000, function() RS.darkRuneSentinelHeroicStrike(creature, guid) end)
    schedule(guid, "battle_shout", 15000, function() RS.darkRuneSentinelBattleShout(creature, guid) end)
    schedule(guid, "whirlwind", 17000, function() RS.darkRuneSentinelWhirlwind(creature, guid) end)
end

RS.darkRuneSentinelHeroicStrike = function(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "heroic_strike", 500, function() RS.darkRuneSentinelHeroicStrike(creature, guid) end)
        return
    end
    creature:CastSpell(nil, RS.SPELL_HEROIC_STRIKE)
    schedule(guid, "heroic_strike", math.random(5000, 9000), function() RS.darkRuneSentinelHeroicStrike(creature, guid) end)
end

RS.darkRuneSentinelBattleShout = function(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "battle_shout", 500, function() RS.darkRuneSentinelBattleShout(creature, guid) end)
        return
    end
    creature:CastSpell(creature, RS.SPELL_BATTLE_SHOUT)
    schedule(guid, "battle_shout", 25000, function() RS.darkRuneSentinelBattleShout(creature, guid) end)
end

RS.darkRuneSentinelWhirlwind = function(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "whirlwind", 500, function() RS.darkRuneSentinelWhirlwind(creature, guid) end)
        return
    end
    creature:CastSpell(creature, RS.SPELL_WHIRLWIND)
    schedule(guid, "whirlwind", math.random(10000, 13000), function() RS.darkRuneSentinelWhirlwind(creature, guid) end)
end

RegisterCreatureEvent(RS.NPC_RAZORSCALE_SPAWNER, 23, RS.spawnerReset)
RegisterCreatureEvent(RS.ENTRY_DARK_RUNE_WATCHER, 23, RS.darkRuneReset)
RegisterCreatureEvent(RS.ENTRY_DARK_RUNE_WATCHER, 1, RS.darkRuneWatcherEngaged)
RegisterCreatureEvent(RS.ENTRY_DARK_RUNE_WATCHER, 2, RS.darkRuneLeaveCombat)
RegisterCreatureEvent(ENTRY_DARK_RUNE_GUARDIAN, 23, RS.darkRuneReset)
RegisterCreatureEvent(ENTRY_DARK_RUNE_GUARDIAN, 1, RS.darkRuneGuardianEngaged)
RegisterCreatureEvent(ENTRY_DARK_RUNE_GUARDIAN, 2, RS.darkRuneLeaveCombat)
RegisterCreatureEvent(RS.ENTRY_DARK_RUNE_SENTINEL, 23, RS.darkRuneReset)
RegisterCreatureEvent(RS.ENTRY_DARK_RUNE_SENTINEL, 1, RS.darkRuneSentinelEngaged)
RegisterCreatureEvent(RS.ENTRY_DARK_RUNE_SENTINEL, 2, RS.darkRuneLeaveCombat)
RegisterCreatureGossipEvent(ENTRY_EXPEDITION_COMMANDER, 2, onCommanderGossipSelect)

-- C++ go_razorscale_mole_machine::Reset (boss_razorscale.cpp:1559-1566):
-- SetFlag(GO_FLAG_NOT_SELECTABLE), 1s UseDoorOrButton, 10s Delete.
-- GO OnAdd is event 12 (gameobjects.go:1659, dynamic spawns only) —
-- latent until the spawner chain ports (same latent-add precedent as
-- the XT-002 life spark). Delete ports as RemoveFromWorld (the GO
-- destroy twin, lua_objects.go:1407; Despawn only hides).
local GO_MOLE_MACHINE = 194316 -- ulduar.h:250
local GO_FLAG_NOT_SELECTABLE = 0x10 -- SharedDefines.h:1641
local GAMEOBJECT_FLAGS_FIELD = 9 -- gameobjects.go gameObjectFlags

local function moleMachineAdd(event, go)
    go:SetFlag(GAMEOBJECT_FLAGS_FIELD, GO_FLAG_NOT_SELECTABLE)
    CreateLuaEvent(function()
        go:UseDoorOrButton()
    end, 1000)
    CreateLuaEvent(function()
        go:RemoveFromWorld()
    end, 10000)
end

RegisterGameObjectEvent(GO_MOLE_MACHINE, 12, moleMachineAdd)

-- C++ npc_expedition_defender::Reset (boss_razorscale.cpp:876-878):
-- DoCastSelf(SPELL_THREAT 65146), non-triggered. Latent until the
-- expedition adds arrive, same class as the mole-machine GO leg.
-- The ctor (boss_razorscale.cpp:869-872) runs first:
-- SetRegenerateHealth(false).
local ENTRY_EXPEDITION_DEFENDER = 33816 -- ulduar.h:92
local SPELL_THREAT = 65146

local function expeditionDefenderReset(_, creature)
    creature:SetRegenerateHealth(false)
    creature:CastSpell(creature, SPELL_THREAT)
end

RegisterCreatureEvent(ENTRY_EXPEDITION_DEFENDER, 23, expeditionDefenderReset)

-- C++ npc_expedition_engineer::Reset (boss_razorscale.cpp:982-986):
-- SetReactState(REACT_PASSIVE) + _scheduler.CancelAll(). The
-- CancelAll leg stays out (see the header note — the 28s harpoon leg
-- is a raw CreateLuaEvent whose id is discarded at schedule time).
local function engineerReset(_, creature)
    creature:SetReactState(RS.REACT_PASSIVE)
end

RegisterCreatureEvent(ENTRY_EXPEDITION_ENGINEER, 23, engineerReset)

-- C++ npc_expedition_trapper ctor (boss_razorscale.cpp:917-921):
-- SetCombatMovement(false) + SetReactState(REACT_PASSIVE). No Reset
-- override exists — the ctor is the spawn-time arm, so it rides ev23.
local function trapperReset(_, creature)
    creature:SetCombatMovement(false)
    creature:SetReactState(RS.REACT_PASSIVE)
end

RegisterCreatureEvent(RS.ENTRY_EXPEDITION_TRAPPER, 23, trapperReset)

-- C++ npc_razorscale_devouring_flame::Reset (boss_razorscale.cpp:1467-1471):
-- SetReactState(REACT_PASSIVE) + one-shot DoCastSelf(
-- DEVOURING_FLAME_GROUND, triggered). The 64734 pick rides
-- Is25ManRaid, the RAID_MODE twin (see the header note for the
-- entry-verification chain). Dormant until the 63236 -> 63308 summon
-- chain lands and Spell.dbc ships 64709/64734.
local function devouringFlameReset(_, creature)
    creature:SetReactState(RS.REACT_PASSIVE)
    creature:CastSpell(creature, creature:Is25ManRaid() and RS.SPELL_DEVOURING_FLAME_GROUND_25 or RS.SPELL_DEVOURING_FLAME_GROUND, true)
end

RegisterCreatureEvent(RS.ENTRY_DEVOURING_FLAME, 23, devouringFlameReset)

-- C++ go_razorscale_harpoon::Reset (boss_razorscale.cpp:1488-1498),
-- 1s leg only: FindNearestCreature(NPC_RAZORSCALE_CONTROLLER 33233,
-- 5.0f) -> Talk(EMOTE_HARPOON 0) + FindNearestGameObject(
-- GO_RAZOR_BROKEN_HARPOON 194565, 5.0f) -> RemoveFromWorld. The
-- earlier "no GO FindNearestGameObject" stay-out was stale: the
-- bridge lives on the GO surface (lua_objects.go:1361), same as
-- the magmus GO-anchor precedent.
local ENTRY_RAZORSCALE_CONTROLLER = 33233 -- ulduar.h:94
local EMOTE_HARPOON = 0
local GO_RAZOR_HARPOONS = {194542, 194541, 194543, 194519} -- ulduar.h:251-254
local GO_RAZOR_BROKEN_HARPOON = 194565 -- ulduar.h:255

local function harpoonAdd(_, go)
    CreateLuaEvent(function()
        local controller = go:FindNearestCreature(ENTRY_RAZORSCALE_CONTROLLER, 5.0)
        if controller then
            controller:Talk(EMOTE_HARPOON, controller)
        end
        local broken = go:FindNearestGameObject(GO_RAZOR_BROKEN_HARPOON, 5.0)
        if broken then
            broken:RemoveFromWorld()
        end
    end, 1000)
end

for _, entry in ipairs(GO_RAZOR_HARPOONS) do
    RegisterGameObjectEvent(entry, 12, harpoonAdd)
end

-- C++ go_razorscale_harpoon::OnGossipHello (boss_razorscale.cpp:1516-1530):
-- SetFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE) unconditional, then
-- controller = FindNearestCreature(NPC_RAZORSCALE_CONTROLLER, 5.0f);
-- if the controller is casting, return true (no cast); else controller->
-- CastSpell(nullptr, SelectRightSpell(), true), spell by harpoon entry
-- (boss_razorscale.cpp:73-76, ulduar.h:251-254). The casting gate ports
-- as controller:IsCasting() (lua_creature_events.go:1950, live);
-- CastSpell(nil, ...) = victim-cast,
-- a no-op when the trigger controller holds no victim in Go's motion
-- state. The SetFlag write is the live anti-reclick effect.
local HARPOON_SHOT_BY_ENTRY = {
    [194542] = 63658, -- GO_RAZOR_HARPOON_1 -> SPELL_HARPOON_SHOT_1
    [194541] = 63657, -- GO_RAZOR_HARPOON_2 -> SPELL_HARPOON_SHOT_2
    [194543] = 63659, -- GO_RAZOR_HARPOON_3 -> SPELL_HARPOON_SHOT_3
    [194519] = 63524, -- GO_RAZOR_HARPOON_4 -> SPELL_HARPOON_SHOT_4
}

local function onHarpoonGossip(_, _, go)
    go:SetFlag(GAMEOBJECT_FLAGS_FIELD, GO_FLAG_NOT_SELECTABLE)
    local controller = go:FindNearestCreature(ENTRY_RAZORSCALE_CONTROLLER, 5.0)
    if controller then
        if controller:IsCasting() then
            return true
        end
        local spellId = HARPOON_SHOT_BY_ENTRY[go:GetEntry()]
        if spellId then
            controller:CastSpell(nil, spellId, true)
        end
    end
    return true
end

for _, entry in ipairs(GO_RAZOR_HARPOONS) do
    RegisterGameObjectGossipEvent(entry, 1, onHarpoonGossip)
end

-- SpellScript/AuraScript legs (boss_razorscale.cpp:1583-1700; PORTED
-- 2026-11-08 tick 456). The head's "spell scripts (no SpellScript
-- bridge)" stay-out is RETIRED: the RegisterSpellEvent bridge (ev2/4/5/7,
-- ticks 445-447) and the RegisterAuraEvent bridge (ev1, tick 439) are live,
-- and all four scripts below ride them. The flame-breath CheckDamage OnHit
-- leg is NOT re-ported here: the ev14 darkRuneGuardianSpellHit above (tick
-- 417) already carries the lethal-breath latch with the inverted
-- post-damage check. Handlers are inline (zero new top-level locals).

-- C++ spell_razorscale_summon_iron_dwarves::HandleScriptEffect
-- (boss_razorscale.cpp:1621-1660): OnEffectHitTarget EFFECT_0
-- SPELL_EFFECT_SCRIPT_EFFECT. The trigger spell id selects the fan-out:
-- 63968 -> 2x guardian summon (62926) + 1x watcher summon (63135);
-- 63970/63969 -> 1x guardian + 2x watcher; anything else (e.g. the
-- spawner's 63798 cast) hits default: break. spell:GetEntry() is the
-- GetSpellInfo()->Id twin (lua_spell.go). The effectIndex == 0 gate
-- mirrors the EFFECT_0 registration: the Go fire runs once per (effect,
-- hit target) for every non-zero effect (boss_ai.go), so without it a
-- second effect would re-run the fan-out. DORMANT until Spell.dbc ships:
-- the trigger casts are DBC-gated (creature path needs spellKnown).
RegisterSpellEvent(RS.SPELL_TRIGGER_SUMMON_IRON_DWARVES, 5, function(event, spell, target, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    caster:CastSpell(caster, RS.SPELL_SUMMON_IRON_DWARF_GUARDIAN, true)
    caster:CastSpell(caster, RS.SPELL_SUMMON_IRON_DWARF_GUARDIAN, true)
    caster:CastSpell(caster, RS.SPELL_SUMMON_IRON_DWARF_WATCHER, true)
end)

RegisterSpellEvent(RS.SPELL_TRIGGER_SUMMON_IRON_DWARVES_2, 5, function(event, spell, target, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    caster:CastSpell(caster, RS.SPELL_SUMMON_IRON_DWARF_GUARDIAN, true)
    caster:CastSpell(caster, RS.SPELL_SUMMON_IRON_DWARF_WATCHER, true)
    caster:CastSpell(caster, RS.SPELL_SUMMON_IRON_DWARF_WATCHER, true)
end)

RegisterSpellEvent(RS.SPELL_TRIGGER_SUMMON_IRON_DWARVES_3, 5, function(event, spell, target, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    caster:CastSpell(caster, RS.SPELL_SUMMON_IRON_DWARF_GUARDIAN, true)
    caster:CastSpell(caster, RS.SPELL_SUMMON_IRON_DWARF_WATCHER, true)
    caster:CastSpell(caster, RS.SPELL_SUMMON_IRON_DWARF_WATCHER, true)
end)

-- C++ spell_razorscale_firebolt::FilterTargets (boss_razorscale.cpp:1687-1700):
-- OnObjectAreaTargetSelect EFFECT_0 TARGET_UNIT_SRC_AREA_ENTRY (7) -> drop
-- every target whose entry is not NPC_RAZORSCALE_HARPOON_FIRE_STATE (33282).
-- The Go fire (lua_spell.go fireSpellTargetSelects) opens for entry-area legs
-- (isEntryAreaTargetType, 7/8) and hands the effect's list; target:GetEntry()
-- reads 0 for players via the generic fallback (object.go), exactly the C++
-- Object::GetEntry() value, so players are dropped like in C++. DORMANT until
-- Spell.dbc ships: the 62669 cast is DBC-gated.
RegisterSpellEvent(RS.SPELL_FIREBOLT, 7, function(event, spell, effectIndex, targets)
    if effectIndex ~= 0 then
        return
    end
    for _, target in ipairs(targets:GetTargets()) do
        if target:GetEntry() ~= RS.ENTRY_HARPOON_FIRE_STATE then
            targets:Remove(target)
        end
    end
end)

-- C++ spell_razorscale_flame_breath::FilterTargets (boss_razorscale.cpp:1602-1614):
-- OnObjectAreaTargetSelect EFFECT_1 TARGET_UNIT_CONE_ENTRY (60) -> drop trigger
-- creatures from the cone list. The Go fire opens for cone-entry legs
-- (isFriendlyConeTargetType covers 59/60, friendly_nearby_cone_targets.go:77;
-- C++ SpellInfo.cpp:280 classifies 60 as CONE/ENTRY) and hands the effect's
-- per-effect partition. The GetObjectType gate is the ToCreature() nil-guard
-- twin: players have no IsTrigger method. DORMANT until Spell.dbc ships: the
-- 63317/64021 casts are DBC-gated (and the fire depends on the DBC carrying
-- target 60 on effect 1, as the C++ registration states).
RegisterSpellEvent(SPELL_FLAME_BREATH, 7, function(event, spell, effectIndex, targets)
    if effectIndex ~= 1 then
        return
    end
    for _, target in ipairs(targets:GetTargets()) do
        if target:GetObjectType() == "Creature" and target:IsTrigger() then
            targets:Remove(target)
        end
    end
end)

RegisterSpellEvent(SPELL_FLAME_BREATH_25, 7, function(event, spell, effectIndex, targets)
    if effectIndex ~= 1 then
        return
    end
    for _, target in ipairs(targets:GetTargets()) do
        if target:GetObjectType() == "Creature" and target:IsTrigger() then
            targets:Remove(target)
        end
    end
end)

-- C++ spell_razorscale_fuse_armor::HandleFused (boss_razorscale.cpp:1662-1685):
-- AuraScript OnEffectPeriodic EFFECT_0 SPELL_AURA_PERIODIC_DUMMY (64821).
-- At full stacks the bearer casts SPELL_FUSED_ARMOR (64774) on itself (C++
-- CastSpell(nullptr, ...) = self-targeted spell, the DoCastSelf twin) and the
-- aura removes itself. aura:GetSpellStackAmount() is the
-- GetSpellInfo()->StackAmount twin (lua_aura.go:356, DBC field 49); it reads 0
-- until Spell.dbc ships, so the gate stays fail-closed and the cast never fires
-- early. No PreventDefault: the C++ handler has none (no true return).
-- DORMANT until Spell.dbc ships: the 64821 cast is DBC-gated (victim cast :642).
RegisterAuraEvent(RS.SPELL_FUSE_ARMOR, 1, function(event, aura, target)
    if aura:GetStackAmount() ~= aura:GetSpellStackAmount() then
        return
    end
    if target ~= nil then
        target:CastSpell(target, RS.SPELL_FUSED_ARMOR, true)
    end
    aura:Remove()
end)
