-- Malygos (Eye of Eternity) — Lua port of
-- src/server/scripts/Northrend/Nexus/EyeOfEternity/boss_malygos.cpp
-- (boss_malygos (CreatureScript) via
-- GetEyeOfEternityAI<boss_malygosAI> (BossAI, DATA_MALYGOS_EVENT);
-- npc_portal_eoe / npc_power_spark / npc_melee_hover_disk /
-- npc_caster_hover_disk / npc_nexus_lord / npc_scion_of_eternity /
-- npc_arcane_overload / npc_wyrmrest_skytalon / npc_static_field
-- (CreatureScripts); spell_malygos_portal_beam / spell_malygos_-
-- random_portal / spell_malygos_arcane_storm / spell_malygos_-
-- vortex_dummy / spell_malygos_vortex_visual / spell_arcane_-
-- overload / spell_nexus_lord_align_disk_aggro /
-- spell_scion_of_eternity_arcane_barrage /
-- spell_malygos_destroy_platform_channel /
-- spell_alexstrasza_bunny_destroy_platform_boom_visual /
-- spell_alexstrasza_bunny_destroy_platform_event /
-- spell_wyrmrest_skytalon_summon_red_dragon_buddy /
-- spell_wyrmrest_skytalon_ride_red_dragon_buddy_trigger /
-- spell_malygos_surge_of_power_warning_selector_25 /
-- spell_malygos_surge_of_power_25 / spell_alexstrasza_gift_beam /
-- spell_alexstrasza_gift_beam_visual (SpellScriptLoaders ->
-- SpellScript / AuraScript); achievement_denyin_the_scion
-- (AchievementCriteriaScript); all registered from inside
-- AddSC_boss_malygos(); loader decl 91 / call 286 per
-- northrend_script_loader.cpp — the FIRST group of the
-- "// The Nexus: Eye of Eternity" block in AddNorthrendScripts(),
-- immediately after AddSC_oculus() (decl 89 / call 284), under
-- the "// The Nexus: Eye of Eternity" marker; the call after it
-- is AddSC_instance_eye_of_eternity() (decl 92 / call 287) —
-- loader order confirmed this run; the checkpoint sequence
-- (oculus -> boss_malygos) is followed).
-- Entry: 28859 Malygos (eye_of_eternity.h NPC_MALYGOS, line 49;
-- DATA_MALYGOS_EVENT = 0, line 28;
-- instance_eye_of_eternity.cpp OnCreatureCreate binds case
-- NPC_MALYGOS (line 134) — entry-verifiable, registration
-- proceeds (the nexus_commanders kalecgos precedent); the
-- CreatureScript ScriptName binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for each of the
-- 28 script names hits boss_malygos.cpp (+ the loader decl/call
-- lines for AddSC_boss_malygos) only; zero sql/ hits. No malygos
-- lua existed.
-- Eluna creature events: 1 OnEnterCombat, 3 OnTargetDied, 4
-- OnDied. No timers in the ported arms; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — SetBossState(IN_PROGRESS) + Talk(SAY_START_P_ONE 1) +
-- DoCast(SPELL_BERSERK 60670) (event 1; the setActive /
-- CheckRequiredBosses / DoStartTimedAchievement legs have no
-- bridges (EnterEvadeMode IS bridged, lua_creature_events.go:5014,
-- but the gate needs CheckRequiredBosses) —
-- the tharon_ja BossAI-bookkeeping precedent; the state leg is
-- PORTED 2026-11-05 in malygosEnterCombat, C++ order: state
-- before Talk + berserk). The berserk is a
-- non-triggered self-cast at engage (1-arg DoCast, UnitAI.cpp default
-- target self) -- the aura carries the 10-min timer itself.
-- KilledUnit — C++-GATED on who->GetTypeId() == TYPEID_PLAYER,
-- then Talk(SAY_KILLED_PLAYER_P_ONE 3 / P_TWO 8 / P_THREE 15)
-- selected by the AI's internal _phase (PHASE_ONE / PHASE_TWO /
-- PHASE_THREE) with a 5s kill-spam filter — DOCUMENTED-ONLY:
-- the phase-select has no bridge (no phase model in Lua), so no
-- faithful port exists.
-- npc_arcane_overload (NPC_ARCANE_OVERLOAD 30282, own file
-- npc_arcane_overload.lua, registered via
-- engine/world/boss_ai.go:RegisterLuaBoss("npc_arcane_overload",
-- 30282)): SpellHit SPELL_ARCANE_BOMB_TRIGGER (56430) ->
-- DoCastAOE SPELL_ARCANE_BOMB_KNOCKBACK_DAMAGE (56431, triggered) +
-- DoCastSelf SPELL_ARCANE_OVERLOAD_1 (56432, triggered), in C++ order
-- via event 14 (event, creature, caster, spellid) with the exact
-- triggered flags; gated on the trigger spell ID (the C++ SpellHit
-- if) — C++ boss_malygos.cpp :1478-:1484. Note: the engine fires event
-- 14 once per hit target — the true C++ DoDamageAndTriggers twin — so
-- this is 1:1 with the C++ per-hit SpellHit (the earlier per-effect
-- caveat is retracted).
-- JustDied — SetBossState(DONE) + Talk(SAY_DEATH 17) (event 4; the
-- _JustDied bookkeeping is the state leg, PORTED 2026-11-05 in
-- malygosDied, C++ order: state first; the gift-box-bunny
-- SummonGameObject GO_HEART_OF_MAGIC leg is PORTED the same tick
-- via the instanceData.giftBoxBunny bind
-- (instance_eye_of_eternity.lua) — the GetGuidData claim below
-- is stale; the NPC_ALEXSTRASZA 32295 summon is ported.
-- The 5s
-- DespawnOrUnsummon IS ported (malygosDied below — stale
-- "no despawn bridge" retracted; creature:Despawn() is bridged,
-- lua_objects.go).
-- Vortex machine (PORTED tick 429, below): EVENT_VORTEX
-- CreateLuaEvent timer (30-35s first, 60-80s re-arm, C++
-- boss_malygos.cpp:547/:776-779) seeded at engage — engage
-- follows the iris use + land now that the intro land machine
-- is PORTED this tick, so the kit arms at first engage after
-- the land (C++ arms phase one at land+7s; residual deviation:
-- if engage fires before the land completes, the first vortex
-- is up to ~7s early); the phase-one latch is Lua-side (the
-- tick no-ops and stops re-arming at 50% health, the C++
-- UpdateAI phase-two flip — the transition itself stays out).
-- ACTION_LIFT_IN_AIR
-- (z + 20 MoveTo twin of MoveTakeoff) + EVENT_MOVE_TO_VORTEX_POINT
-- (REACT_PASSIVE + AttackStop + MoveTo vortex point) +
-- ACTION_EXECUTE_VORTEX (56237/55873/56105 triggered self-casts,
-- C++ order) run through ev38/ev28 (POINT_LIFT_IN_AIR_P_ONE 5 /
-- POINT_VORTEX_P_ONE 3 / POINT_LAND_AFTER_VORTEX_P_ONE 4).
-- spell_malygos_vortex_visual::OnRemove (55873, ev3) is PORTED
-- (threat-list players -> vortex trigger casts 73040, triggered;
-- MoveLand twin + RemoveAura 56237) but DORMANT until the aura
-- system applies 55873 (no Spell.dbc); the 10s fallback land timer
-- (fight-documented vortex duration, wowpedia/guiaswow) carries
-- the land until then, latch-guarded against double-run. GM skip
-- has no bridge (no IsGameMaster on the player surface).
-- spell_malygos_vortex_dummy (56105) is PORTED 2026-11-08
-- (tick 431, below): its HandleScript writes instance
-- SetData(DATA_VORTEX_HANDLING) — the Go-only InstanceData:SetData
-- bridge (event 8) carries it to instance_eye_of_eternity.lua,
-- which runs VortexHandling in Lua.
-- Unmodeled (no bridges — documented, not wired):
-- Reset / Initialize (gravity / immunity / flags / flight-speed
-- legs, SetPhase(PHASE_NOT_STARTED), REACT_PASSIVE,
-- SetBossState NOT_STARTED) — the gravity leg + flag-clear +
-- REACT_PASSIVE legs are ported (malygosReset), and the
-- SetBossState NOT_STARTED leg is PORTED 2026-11-05 in
-- malygosReset (!= DONE-gated, _Reset() position first in C++
-- Reset :372); immunity / flight-speed / phase legs stay out
-- (SetImmuneToAll IS bridged, lua_creature_events.go:5966 — the
-- Reset apply below writes the same 0x100|0x200 flags; no speed
-- literal, no phase bridge).
-- DoAction (the ~60-action event/phase
-- machine: ACTION_LAND_ENCOUNTER_START, platform-destroy
-- intro, surge-of-power legs — all gated on
-- MotionMaster / instance GUIDs / timed events) — dispatch IS
-- bridged (creature:DoAction fires ev38,
-- lua_creature_events.go:802); the VORTEX actions are PORTED
-- (ACTION_EXECUTE_VORTEX + ACTION_LIFT_IN_AIR, below) and
-- ACTION_HANDLE_RESPAWN is PORTED 2026-11-08 (tick 432, below —
-- dormant, FAIL-gated); ACTION_LAND_ENCOUNTER_START is PORTED
-- this tick (below — sender: the focusing-iris GO use in
-- instance_eye_of_eternity.lua; EVENT_LAND_START_ENCOUNTER 7s +
-- POINT_LAND_P_ONE arrival below). ACTION_HANDLE_P_THREE_INTRO
-- (2) + ACTION_CYCLIC_MOVEMENT (5) stay out on unported
-- drivers (the SetData summon-deaths machine /
-- EVENT_PATHING_AROUND_PLATFORM; no MoveCirclePath bridge),
-- not dispatch. MovementInform
-- (POINT_LAND_P_ONE / cyclic movement points) — the vortex
-- takeoff-land points are PORTED (below), POINT_LAND_P_ONE
-- is PORTED this tick (type-16 arrival, below), and
-- POINT_NEAR_RANDOM_PORTAL_P_NONE is PORTED 2026-11-11 (below);
-- the rest has no motion bridge. boss_malygos.cpp:720
-- POINT_IDLE_P_THREE SetControlled(true, UNIT_STATE_ROOT) rides
-- that same unbridged MovementInform: stays out, except the
-- vortex + random-portal points (PORTED below). DamageTaken is only the lethal clamp (damage > health &&
-- _phase != PHASE_THREE -> damage = health - 1); ev9 IS bridged
-- pre-damage with a rewrite (lua_creature_events.go:6955) but the
-- arm stays out — no phase model to tell PHASE_THREE. UpdateAI (phase-one arcane event machine,
-- phase-three disk/scion machine) — no timer-event / cast /
-- phase bridges; EVENT_VORTEX + EVENT_POWER_SPARKS are PORTED
-- (CreateLuaEvent timers + DoAction / instance SetData, below).
-- SpellHit — SPELL_POWER_SPARK_MALYGOS 56152
-- (Talk SAY_BUFF_SPARK 14 + despawn the spark) stays out: the
-- trigger is Malygos's MoveInLineOfSight arm (event 27 exists,
-- but the arm is PHASE_ONE-gated with no phase model in Lua —
-- next bullet), so event 14 can never fire with this ID;
-- SPELL_MALYGOS_BERSERK 47008 -> Talk(EMOTE_HIT_BERSERKER_TIMER
-- 19) stays out: the trigger is the self-cast SPELL_BERSERK 60670
-- aura's 10-minute tick delivering 47008, but the Go creature-cast
-- path applies no aura effects ("no aura-application site at cast
-- completion", boss_ai.go) — the 60670 engage cast applies nothing,
-- so 47008 is never delivered and event 14 can never fire with
-- this ID (re-audit 2026-10-15: the 22:34 "trigger live in Go"
-- claim was wrong).
-- MoveInLineOfSight (power-spark proximity -> cast
-- SPELL_POWER_SPARK_MALYGOS) — event 27 exists, but the arm is
-- gated on _phase == PHASE_ONE (no phase model in Lua) and the
-- spark's approach movement is unported — stays out (2026-10-23
-- re-triage; the stale "no LOS bridge" wording is retracted).
-- npc_portal_eoe (NPC_PORTAL_TRIGGER 30118) — SpellHit
-- SPELL_PORTAL_OPENED -> phase-one-gated DoCast
-- SPELL_SUMMON_POWER_PARK is PORTED 2026-11-08 (tick 432, own
-- file npc_portal_eoe.lua, ev14 — DORMANT until Spell.dbc ships,
-- the hit tail is spellKnown-gated; the phase gate rides the
-- in-combat + >50% health proxy); UpdateAI per-tick aura
-- maintenance stays out (no per-tick UpdateAI bridge). npc_power_spark
-- (NPC_POWER_SPARK 30084) — UpdateAI despawn-when-reached legs
-- documented-only (no motion bridge); JustDied casts
-- SPELL_POWER_SPARK_DEATH 55852 triggered self-cast PORTED as
-- lua_scripts/northrend/npc_power_spark.lua (event 4 — no phase
-- gate in the C++ arm). npc_melee_hover_disk (NPC_HOVER_DISK_MELEE 30234) /
-- npc_caster_hover_disk (NPC_HOVER_DISK_CASTER 30248) —
-- VehicleAI: PassengerBoarded (player enters -> welcome /
-- special-attack / take-off event machine; UNIT-type passenger
-- -> SPELL_TELEPORT_VISUAL_ONLY), MovementInform gravity
-- legs — no vehicle / passenger / motion bridges.
-- npc_nexus_lord (NPC_HOVER disk adds' lord) — DoAction ->
-- EVENT_NUKE_DUMMY / EVENT_ARCANE_SHOCK / EVENT_HASTE_BUFF
-- event machine (DoCast SPELL_ARCANE_SHOCK / haste, nuke the
-- disk the player rides) — DoAction-gated, but dispatch IS bridged
-- (creature:DoAction fires ev38, lua_creature_events.go:802): the
-- sender is the melee hover disk's MovementInform waypoint arrival
-- (boss_malygos.cpp:1222), an unported vehicle/motion machine, so
-- the machine never starts (stale "no timer-event bridge" re-keyed:
-- the timer bridge is live but the machine never starts) —
-- documented-only.
-- npc_scion_of_eternity (entry-UNVERIFIABLE from repo sources:
-- NPC_SURGE_OF_POWER 30334 is defined at eye_of_eternity.h :60 but
-- never referenced anywhere in the C++ tree; the ScriptName->entry
-- binding is DB-side — the npc_azure_ring_captain /
-- gothik-minions precedent) —
-- IsSummonedBy schedules EVENT_ARCANE_BARRAGE (target-list
-- arcane barrage machine), JustDied increments Malygos
-- DATA_SUMMON_DEATHS — no timer-event / instance /
-- target-selection bridges. npc_arcane_overload
-- (NPC_ARCANE_OVERLOAD 30282) — the SpellHit SPELL_ARCANE_BOMB_TRIGGER
-- -> SPELL_ARCANE_BOMB_KNOCKBACK_DAMAGE + SPELL_ARCANE_OVERLOAD_1
-- arm IS PORTED (event 14, own file npc_arcane_overload.lua — see
-- the Ported arms section above); IsSummonedBy -> SetGUID
-- DATA_LAST_OVERLOAD_GUID and the phase-gated DespawnOrUnsummon
-- legs — no summon / despawn / instance bridges. npc_wyrmrest_-
-- skytalon (NPC_WYRMREST_SKYTALON 30161) — VehicleAI phase-three
-- disk machine: IsSummonedBy -> MovePoint cycle; SpellHit
-- shock-lance legs; PassengerBoarded player machine — no
-- vehicle / motion / passenger bridges. npc_static_field
-- (entry-UNVERIFIABLE from repo sources — the 30090
-- attribution is WRONG: boss_malygos.cpp:1762 binds 30090 to
-- the vortex trigger vehicle, and the ScriptName->entry
-- binding is DB-side) — IsSummonedBy ->
-- DespawnOrUnsummon(30s) (no periodic logic in the C++) —
-- stays out: creature:Despawn() IS bridged, and ev5 DOES fire on
-- summoned creatures (motionFor fires ev23+ev5 on creation) — but the
-- entry is unverifiable from repo sources, so no registration can be
-- written, and the summoner-side event 19 cannot be gated without
-- the entry.
-- The 17 spell scripts (SpellScript / AuraScript handlers:
-- portal beam relocating, random portal target selection,
-- arcane storm target filters, vortex dummy destination
-- selection, vortex visual periodic, arcane overload
-- immunity-window application, nexus-lord align-disk aggro
-- transfer, scion arcane-barrage target-count scaling (filter leg;
-- the OnHit leg is PORTED this tick, below),
-- destroy-platform channel aura removal, alexstrasza bunny
-- platform boom visual + event, wyrmrest skytalon red-dragon
-- buddy summon + ride trigger, surge-of-power 25 warning
-- target selection + the surge itself, alexstrasza gift beam
-- + gift beam visual) — SpellScript legs ride RegisterSpellEvent
-- (ev1/ev2/ev3/ev4/ev5/ev7 live, runtime.go:601-607; the varos
-- "no SpellScript bridge" claim is stale, retired 2026-11-11);
-- AuraScript OnApply/OnRemove ride RegisterAuraEvent (ported in
-- this file, below). The SpellScript legs join the
-- standing queues, except the gift
-- beam visual OnApply (PORTED tick 204, below), the portal
-- beam (ported, below), and the vortex visual OnRemove
-- (PORTED tick 429, below), the scion arcane-barrage OnHit
-- (PORTED this tick, below), the random portal dummy (PORTED
-- 2026-11-11, below), and the arcane storm target filter
-- (PORTED 2026-11-11, below — the HandleVisual launch leg stays
-- out, no OnEffectLaunchTarget Lua event); the vortex dummy
-- (56105) is PORTED 2026-11-08 (tick 431, below) via the
-- InstanceData:SetData bridge. The arcane overload radius filter
-- stays out: its binding spell is DB-side (56435 only a commented
-- enum line) and the DBC radius has no Lua bridge (no CalcRadius).
-- achievement_denyin_the_scion — OnCheck: player's vehicle
-- base entry == NPC_HOVER_DISK_MELEE (30234) — no achievement
-- bridge (the kelthuzad / thaddius precedent); joins the
-- unmodeled-achievement queue.

local ENTRY_MALYGOS = 28859
local DATA_MALYGOS_EVENT = 0 -- eye_of_eternity.h:28
local DATA_VORTEX_HANDLING = 2 -- eye_of_eternity.h:31
local DATA_POWER_SPARKS_HANDLING = 3 -- eye_of_eternity.h:32
local DATA_RESPAWN_IRIS = 4 -- eye_of_eternity.h:33
local BOSS_STATE_FAILED = 2 -- EncounterState (InstanceScript.h:68-76)
local BOSS_STATE_NOT_STARTED = 0 -- EncounterState (InstanceScript.h:68-76)
local BOSS_STATE_IN_PROGRESS = 1 -- EncounterState (InstanceScript.h:68-76)
local BOSS_STATE_DONE = 3 -- EncounterState (InstanceScript.h:68-76)
local GO_HEART_OF_MAGIC_10 = 194158 -- eye_of_eternity.h:70
local GO_HEART_OF_MAGIC_25 = 194159 -- eye_of_eternity.h:71
local GO_FOCUSING_IRIS_10 = 193958 -- eye_of_eternity.h:66
local GO_FOCUSING_IRIS_25 = 193960 -- eye_of_eternity.h:67
local HEART_OF_MAGIC_SPAWN_X = 755.351 -- boss_malygos.cpp:291
local HEART_OF_MAGIC_SPAWN_Y = 1298.31 -- boss_malygos.cpp:291
local HEART_OF_MAGIC_SPAWN_Z = 223.909 -- boss_malygos.cpp:291
local HEART_OF_MAGIC_SPAWN_O = 0.0 -- boss_malygos.cpp:291
local NPC_ALEXSTRASZA = 32295 -- eye_of_eternity.h:57

local SAY_START_P_ONE = 1
local SAY_DEATH = 17

-- C++ SPELL_BERSERK = 60670 (boss_malygos.cpp :105): DoCast(SPELL_BERSERK)
-- in JustEngagedWith (:577) -- 2-arg DoCast, non-triggered self-cast
-- (UnitAI.cpp default target self); the periodic aura itself carries the
-- 10-minute berserk timer, so the faithful port casts it once at engage.
local SPELL_BERSERK = 60670
local SPELL_MALYGOS_PORTAL_BEAM = 56046 -- boss_malygos.cpp:102
local SPELL_PORTAL_OPENED = 61236 -- eye_of_eternity.h:84
local SPELL_ARCANE_BARRAGE = 56397 -- boss_malygos.cpp:133 (used by Scions of Eternity)
local SPELL_ARCANE_BARRAGE_DAMAGE = 63934 -- boss_malygos.cpp:134 (the actual damage - cast by affected player by script spell)
local POINT_LAND_P_ONE = 2 -- Movements enum (boss_malygos.cpp:162)
local POINT_VORTEX_P_ONE = 3 -- Movements enum (boss_malygos.cpp:164-166)
local POINT_LAND_AFTER_VORTEX_P_ONE = 4
local POINT_LIFT_IN_AIR_P_ONE = 5
local ACTION_LAND_ENCOUNTER_START = 0 -- Actions enum (boss_malygos.cpp:180)
local ACTION_EXECUTE_VORTEX = 1 -- Actions enum (boss_malygos.cpp:183-185)
local ACTION_LIFT_IN_AIR = 3
local ACTION_HANDLE_RESPAWN = 4 -- Actions enum (boss_malygos.cpp:186)
local SPELL_VORTEX_1 = 56237 -- boss_malygos.cpp:113 (freeze-object animation)
local SPELL_VORTEX_2 = 55873 -- boss_malygos.cpp:114 (visual effect)
local SPELL_VORTEX_3 = 56105 -- boss_malygos.cpp:115 (dummy; the vortex script)
local SPELL_VORTEX_6 = 73040 -- boss_malygos.cpp:116 (teleport, cast by vortex bunnies)
local VORTEX_POINT_X = 754.393 -- MalygosPositions[1] (boss_malygos.cpp:283)
local VORTEX_POINT_Y = 1301.27
local VORTEX_POINT_Z = 292.91
local LAND_AFTER_VORTEX_X = 754.362 -- MalygosPositions[2] (boss_malygos.cpp:284)
local LAND_AFTER_VORTEX_Y = 1301.61
local LAND_AFTER_VORTEX_Z = 266.17
local REACT_PASSIVE = 0
local REACT_AGGRESSIVE = 2
-- Vortex fallback land delay: the C++ land rides the SPELL_VORTEX_2
-- removal (see vortexLand below), whose DBC duration is not in the
-- repo; the fight-documented vortex lasts 10s (wowpedia Malygos
-- tactics: 2000 damage every second for 10 seconds, "When Vortex
-- ends, everyone will be dropped"; guiaswow concurs). The timer is
-- the expiry emulation until the aura system lands; the ev3 handler
-- is authoritative then and the landed latch makes the timer a no-op.
local VORTEX_LAND_FALLBACK_MS = 10000

local timers = {} -- per-GUID {key -> CreateLuaEvent id} (eregos idiom)

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

-- Per-key cancel twin of EventMap::CancelEventGroup for one key
-- (the portal driver keys below); cancelTimers above is the
-- whole-map twin used by Reset/JustDied.
local function cancelTimer(guid, key)
    local per = timers[guid]
    if per and per[key] then
        RemoveEventById(per[key])
        per[key] = nil
    end
end

-- Per-GUID vortex machine state: executing is the C++ _executingVortex
-- latch (set at EVENT_VORTEX, cleared at the land-after-vortex
-- arrival); landed guards the land body so the ev3 removal handler
-- and the fallback timer can never run it twice.
local vortexState = {}

local function getVortexState(guid)
    local state = vortexState[guid]
    if not state then
        state = { executing = false, landed = false }
        vortexState[guid] = state
    end
    return state
end

local SPELL_RANDOM_PORTAL = 56047 -- boss_malygos.cpp:101
local SPELL_PORTAL_BEAM = 56046 -- boss_malygos.cpp:102
local POINT_NEAR_RANDOM_PORTAL_P_NONE = 1 -- Movements enum (boss_malygos.cpp:162)
local NPC_PORTAL_TRIGGER = 30118 -- eye_of_eternity.h:51

-- C++ boss_malygosAI::UpdateAI EVENT_START_FIRST_RANDOM_PORTAL /
-- EVENT_RANDOM_PORTAL (boss_malygos.cpp:756-758): DoCastAOE
-- (SPELL_RANDOM_PORTAL, { SPELLVALUE_MAX_TARGETS, 1 }). Twin: the
-- DoCastAOE self-cast (creature:CastSpell(creature, spell));
-- SPELLVALUE_MAX_TARGETS has no bridge, so the 1-target cap is not
-- modeled — the cast is dormant until Spell.dbc ships in any case.
-- PORTED 2026-11-11 (row-401 follow-up).
local function firstRandomPortalTick(creature, guid)
    creature:CastSpell(creature, SPELL_RANDOM_PORTAL)
end

-- C++ EVENT_STOP_PORTAL_BEAM (boss_malygos.cpp:759-761):
-- InterruptNonMeleeSpells(true). The Lua bridge takes no args
-- (lua_creature_events.go:2320) — the delayed flag has no model;
-- the interrupt itself is the leg. PORTED 2026-11-11.
local function stopPortalBeamTick(creature, guid)
    creature:InterruptNonMeleeSpells()
end

-- C++ boss_malygosAI::MovementInform POINT_NEAR_RANDOM_PORTAL_P_NONE
-- (boss_malygos.cpp:665-672): the nearest portal trigger in 31 yd
-- (alive-only — the Go bridge filters dead motions) re-arms the
-- intro machine: EVENT_STOP_PORTAL_BEAM in 10s + EVENT_RANDOM_PORTAL
-- in 14s, both group 1 PHASE_NOT_STARTED (the keyed schedule keeps
-- the C++ single-pending invariant), then DoCast(portal,
-- SPELL_PORTAL_BEAM). PORTED 2026-11-11 (row-401 follow-up).
local function portalArrivalTick(creature, guid)
    local portal = creature:FindNearestCreature(NPC_PORTAL_TRIGGER, 31.0)
    if portal == nil then
        return
    end
    schedule(guid, "stopBeam", 10000, function()
        stopPortalBeamTick(creature, guid)
    end)
    schedule(guid, "randomPortal", 14000, function()
        firstRandomPortalTick(creature, guid)
    end)
    creature:CastSpell(portal, SPELL_PORTAL_BEAM)
end

-- Random-resize twin of Trinity::Containers::RandomResize: keeps a
-- uniform random subset of n from list (Fisher-Yates partial).
-- Shared by the arcane-storm and scion barrage filters below.
local function randomResize(list, n)
    local count = #list
    if count <= n then
        return list
    end
    for i = 1, n do
        local j = math.random(i, count)
        list[i], list[j] = list[j], list[i]
    end
    local out = {}
    for i = 1, n do
        out[i] = list[i]
    end
    return out
end

-- C++ spell_malygos_vortex_visual::OnRemove (boss_malygos.cpp:1800-1829,
-- AfterEffectRemove EFFECT_0 SPELL_AURA_DUMMY): every threat-list
-- player (the GM skip has no bridge — the player surface exposes no
-- IsGameMaster; documented deviation) is teleported by the vortex
-- trigger (instance GetGuidData(DATA_TRIGGER); the Go guid store is
-- never latched for it, so the live twin is the instanceData
-- vortexTriggers list bound by instance_eye_of_eternity.lua), then
-- Malygos lands (MoveLand twin: MoveTo + the arrival leg below) and
-- drops SPELL_VORTEX_1. Shared by the ev3 handler and the fallback
-- timer; the landed latch makes the second caller a no-op.
local function vortexLand(caster)
    local guid = caster:GetGUIDLow()
    local state = getVortexState(guid)
    if state.landed then
        return
    end
    state.landed = true
    local trigger = nil
    local mapObj = caster:GetMap()
    if mapObj then
        local data = mapObj:GetInstanceData()
        if data and data.vortexTriggers then
            trigger = data.vortexTriggers[1]
        end
    end
    if trigger ~= nil then
        for _, target in ipairs(caster:GetAITargets()) do
            if target:GetObjectType() == "Player" then
                trigger:CastSpell(target, SPELL_VORTEX_6, true)
            end
        end
    end
    caster:MoveTo(POINT_LAND_AFTER_VORTEX_P_ONE, LAND_AFTER_VORTEX_X, LAND_AFTER_VORTEX_Y, LAND_AFTER_VORTEX_Z)
    caster:RemoveAura(SPELL_VORTEX_1)
end

-- C++ EVENT_VORTEX (boss_malygos.cpp:776-779): _executingVortex =
-- true + DoAction(ACTION_LIFT_IN_AIR) + re-arm 60-80s in PHASE_ONE.
-- The phase-one gate is the Lua-side latch (no phase model in Lua):
-- the C++ UpdateAI flips to PHASE_TWO at 50% health, dropping the
-- pending phase-one event, so the tick no-ops and stops re-arming
-- once health is at or below 50% (the phase-two transition itself —
-- Talk SAY_END_P_ONE, REACT_PASSIVE, AttackStop — stays out).
local function vortexTick(creature, guid)
    -- C++ UpdateAI pre-loop (:740) + in-loop (:970) CASTING gates,
    -- PHASE_NOT_STARTED-exempt (ticks seed on engage, so live only in
    -- phase one via the health latch below): 500ms IsCasting defer,
    -- mal_ganis idiom.
    if creature:IsCasting() then
        schedule(guid, "vortex", 500, function()
            vortexTick(creature, guid)
        end)
        return
    end
    if creature:GetHealthPct() <= 50 then
        return
    end
    local state = getVortexState(guid)
    state.executing = true
    creature:DoAction(ACTION_LIFT_IN_AIR)
    schedule(guid, "vortex", math.random(60000, 80000), function()
        vortexTick(creature, guid)
    end)
end

-- C++ EVENT_POWER_SPARKS (boss_malygos.cpp:786-789): instance
-- SetData(DATA_POWER_SPARKS_HANDLING, 0) + re-arm 30-35s in
-- PHASE_ONE. The phase-one gate is the same Lua-side health latch
-- as vortexTick (the C++ UpdateAI flips to PHASE_TWO at 50%
-- health, dropping the pending phase-one event), so the tick
-- no-ops and stops re-arming at or below 50%. The SetData write
-- rides the InstanceData:SetData bridge (event 8); the rotation
-- runs in instance_eye_of_eternity.lua. PORTED 2026-11-08
-- (tick 432).
local function powerSparksTick(creature, guid)
    -- Same C++ CASTING gates as vortexTick (:740/:970,
    -- PHASE_NOT_STARTED-exempt): 500ms IsCasting defer.
    if creature:IsCasting() then
        schedule(guid, "powerSparks", 500, function()
            powerSparksTick(creature, guid)
        end)
        return
    end
    if creature:GetHealthPct() <= 50 then
        return
    end
    local mapObj = creature:GetMap()
    if mapObj then
        local instanceData = mapObj:GetInstanceData()
        if instanceData then
            instanceData:SetData(DATA_POWER_SPARKS_HANDLING, 0)
        end
    end
    schedule(guid, "powerSparks", math.random(30000, 35000), function()
        powerSparksTick(creature, guid)
    end)
end

-- C++ EVENT_MOVE_TO_VORTEX_POINT (boss_malygos.cpp:781-785):
-- REACT_PASSIVE + AttackStop + MovePoint(POINT_VORTEX_P_ONE,
-- MalygosPositions[1]).
local function moveToVortexPoint(creature, guid)
    creature:SetReactState(REACT_PASSIVE)
    creature:AttackStop()
    creature:MoveTo(POINT_VORTEX_P_ONE, VORTEX_POINT_X, VORTEX_POINT_Y, VORTEX_POINT_Z)
end

-- C++ EVENT_LAND_START_ENCOUNTER (boss_malygos.cpp:763-771): face
-- the focusing iris and delete it, REACT_AGGRESSIVE,
-- SetPhase(PHASE_ONE, true). The iris resolves via the instanceData
-- bind with the FindNearestGameObject fallback (the DONE-leg
-- precedent in malygosDied); the iris is a static DB spawn, so the
-- C++ Delete() rides go:Despawn() (sapphiron precedent — Go
-- Delete() only removes dynamic objects, lua_objects.go:4567).
-- The SetPhase leg arms the phase-one event kit — in Lua the
-- ported legs (vortex + power sparks) arm at first engage after
-- the land (malygosEnterCombat), so this tick only faces, hides
-- the iris, and aggressives; the 7s schedule is the DoAction(0)
-- arm below. PORTED this tick.
local function landStartEncounterTick(creature, guid)
    local mapObj = creature:GetMap()
    local iris = nil
    if mapObj then
        local data = mapObj:GetInstanceData()
        if data then
            iris = data.iris
        end
    end
    if iris == nil then
        iris = creature:FindNearestGameObject(GO_FOCUSING_IRIS_10, 1000)
        if iris == nil then
            iris = creature:FindNearestGameObject(GO_FOCUSING_IRIS_25, 1000)
        end
    end
    if iris ~= nil then
        creature:SetFacingToObject(iris)
        iris:Despawn()
    end
    creature:SetReactState(REACT_AGGRESSIVE)
end

-- C++ boss_malygosAI::DoAction (boss_malygos.cpp:473-491): the
-- vortex actions only. ACTION_EXECUTE_VORTEX casts the three vortex
-- spells on self, all triggered, in C++ order; the landed latch is
-- reset and the fallback land timer seeded (see
-- VORTEX_LAND_FALLBACK_MS). ACTION_LIFT_IN_AIR, phase-one leg only
-- (z + 20 takeoff; the phase-two z = 300.1 leg rides the unported
-- phase machine) — MoveTo is the MoveTakeoff twin (no takeoff
-- bridge); the gravity leg rides the arrival handler below. The
-- phase-one gate is the health latch (vortexTick is the only live
-- caller and already checked it).
local function malygosDoAction(event, creature, action)
    if action == ACTION_LAND_ENCOUNTER_START then
        -- C++ boss_malygosAI::DoAction ACTION_LAND_ENCOUNTER_START
        -- (boss_malygos.cpp:460-472): CancelEventGroup(1) — the
        -- group-1 portal keys are cancelled per-key below (the
        -- intro portal machine is ported this tick); EVENT_SAY_INTRO
        -- is still unscheduled in Lua, so no cancel is needed for
        -- it — then MoveLand to the near point of the
        -- Alexstrasza bunny (30 yd at the bunny->Malygos angle,
        -- GetNearPoint2D twin via math.atan2/cos/sin, bunny Z),
        -- SetImmuneToAll(false), DoZoneInCombat, and the 7s
        -- EVENT_LAND_START_ENCOUNTER schedule (landStartEncounterTick
        -- above). Sender: the focusing-iris GO use in
        -- instance_eye_of_eternity.lua (the instance ProcessEvent
        -- EVENT_FOCUSING_IRIS body). The whole arm is gated on the
        -- bunny bind, C++-exact. PORTED this tick.
        local mapObj = creature:GetMap()
        local bunny = nil
        if mapObj then
            local data = mapObj:GetInstanceData()
            if data then
                bunny = data.alexstraszaBunny
            end
        end
        if bunny == nil then
            return
        end
        local bx, by, bz = bunny:GetX(), bunny:GetY(), bunny:GetZ()
        local mx, my = creature:GetX(), creature:GetY()
        local angle = math.atan2(my - by, mx - bx)
        creature:MoveLand(POINT_LAND_P_ONE, bx + 30.0 * math.cos(angle), by + 30.0 * math.sin(angle), bz)
        creature:SetImmuneToAll(false)
        creature:DoZoneInCombat()
        local guid = creature:GetGUIDLow()
        cancelTimer(guid, "firstPortal")
        cancelTimer(guid, "stopBeam")
        cancelTimer(guid, "randomPortal")
        schedule(guid, "landStartEncounter", 7000, function()
            landStartEncounterTick(creature, guid)
        end)
    elseif action == ACTION_EXECUTE_VORTEX then
        creature:CastSpell(creature, SPELL_VORTEX_1, true)
        creature:CastSpell(creature, SPELL_VORTEX_2, true)
        creature:CastSpell(creature, SPELL_VORTEX_3, true)
        -- C++ spell_malygos_vortex_dummy::HandleScript
        -- (boss_malygos.cpp:1760-1770): OnEffectHitTarget, EFFECT_0
        -- SPELL_EFFECT_DUMMY — the 56105 cast above has no Go
        -- SpellScript bridge, so its only leg (instance SetData
        -- DATA_VORTEX_HANDLING, 0) is emulated here via the
        -- InstanceData:SetData bridge; instance_eye_of_eternity.lua
        -- runs VortexHandling on the write. PORTED 2026-11-08
        -- (tick 431). DORMANT until the 56105 cast lands (no
        -- Spell.dbc); the write itself fires whenever the machine
        -- executes.
        local mapObj = creature:GetMap()
        if mapObj then
            local instanceData = mapObj:GetInstanceData()
            if instanceData then
                instanceData:SetData(DATA_VORTEX_HANDLING, 0)
            end
        end
        local guid = creature:GetGUIDLow()
        local state = getVortexState(guid)
        state.landed = false
        schedule(guid, "land", VORTEX_LAND_FALLBACK_MS, function()
            vortexLand(creature)
        end)
    elseif action == ACTION_LIFT_IN_AIR then
        if creature:GetHealthPct() > 50 then
            local x, y, z = creature:GetX(), creature:GetY(), creature:GetZ()
            creature:MoveTo(POINT_LIFT_IN_AIR_P_ONE, x, y, z + 20.0)
        end
    elseif action == ACTION_HANDLE_RESPAWN then
        -- C++ boss_malygosAI::DoAction ACTION_HANDLE_RESPAWN
        -- (boss_malygos.cpp:512-520): teleport to the respawn
        -- position (GetRespawnPosition — the spawn/home position;
        -- the Go twin is creature:GetHomePosition,
        -- lua_creature_events.go:6053; home is never re-set by the
        -- ported arms, so it is the spawn position) + instance
        -- SetData(DATA_RESPAWN_IRIS, 0) + _despawned = false (the
        -- flag is re-read from boss state on each Reset in Lua, so
        -- no latch is kept). PORTED 2026-11-08 (tick 432).
        -- DORMANT: the caller (Reset, below) fires it only when the
        -- boss state is FAIL, and FAIL is unobservable in Go
        -- (boss_ai.go:195 — evade clears the encounter, never sets
        -- FAIL), so the arm never runs today.
        local hx, hy, hz, ho = creature:GetHomePosition()
        creature:NearTeleport(hx, hy, hz, ho)
        local mapObj = creature:GetMap()
        if mapObj then
            local instanceData = mapObj:GetInstanceData()
            if instanceData then
                instanceData:SetData(DATA_RESPAWN_IRIS, 0)
            end
        end
    end
end

-- C++ boss_malygosAI::MovementInform (boss_malygos.cpp:673-685):
-- POINT_LIFT_IN_AIR_P_ONE -> SetDisableGravity(true) + schedule
-- EVENT_MOVE_TO_VORTEX_POINT in 1ms (the 1ms deferral is C++-exact:
-- new movement cannot start from MovementInform);
-- POINT_VORTEX_P_ONE -> MoveIdle (no bridge; the spline already
-- ended) + DoAction(ACTION_EXECUTE_VORTEX);
-- POINT_LAND_AFTER_VORTEX_P_ONE -> SetDisableGravity(false) +
-- _executingVortex = false + REACT_AGGRESSIVE.
local function malygosMovementInform(_, creature, moveType, id)
    if id == POINT_LAND_P_ONE and moveType == 16 then
        -- C++ MovementInform POINT_LAND_P_ONE (boss_malygos.cpp:670-672):
        -- SetDisableGravity(false). MoveLand stamps EFFECT_MOTION_TYPE
        -- (16, lua_creature_events.go:5002), not POINT (8), so this leg
        -- rides the type-16 arrival; the C++ switch is id-only.
        -- PORTED this tick.
        creature:SetDisableGravity(false)
        return
    end
    if moveType ~= 8 then
        return
    end
    local guid = creature:GetGUIDLow()
    if id == POINT_NEAR_RANDOM_PORTAL_P_NONE then
        portalArrivalTick(creature, guid)
    elseif id == POINT_LIFT_IN_AIR_P_ONE then
        creature:SetDisableGravity(true)
        schedule(guid, "moveToVortex", 1, function()
            moveToVortexPoint(creature, guid)
        end)
    elseif id == POINT_VORTEX_P_ONE then
        creature:DoAction(ACTION_EXECUTE_VORTEX)
    elseif id == POINT_LAND_AFTER_VORTEX_P_ONE then
        creature:SetDisableGravity(false)
        local state = getVortexState(guid)
        state.executing = false
        creature:SetReactState(REACT_AGGRESSIVE)
    end
end

-- C++ spell_malygos_vortex_visual::OnRemove (boss_malygos.cpp:1795-1829,
-- AfterEffectRemove): Load admits creature casters only; the body is
-- vortexLand above. PORTED 2026-11-08 (tick 429). DORMANT until the
-- aura system applies 55873 from creature casts (no Spell.dbc): the
-- fallback timer in malygosDoAction carries the land until then.
local function vortexVisualRemove(event, aura, target)
    local caster = aura:GetCaster()
    if caster == nil or caster:GetObjectType() ~= "Creature" then
        return
    end
    vortexLand(caster)
end


local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000

-- C++ JustEngagedWith (boss_malygos.cpp :563-:579): me->setActive(true) +
-- instance->CheckRequiredBosses gate +
-- instance->SetBossState(IN_PROGRESS) + Talk(SAY_START_P_ONE)
-- + DoCast(SPELL_BERSERK) + DoStartTimedAchievement. The Talk and the
-- berserk self-cast are bridgeable; the instance legs are
-- BossAI-bookkeeping with no bridge (tharon_ja precedent).
local function malygosEnterCombat(event, creature, target)
    creature:SetBossState(DATA_MALYGOS_EVENT, BOSS_STATE_IN_PROGRESS)
    creature:Talk(SAY_START_P_ONE)
    creature:CastSpell(creature, SPELL_BERSERK)
    local guid = creature:GetGUIDLow()
    local state = getVortexState(guid)
    state.executing = false
    state.landed = false
    -- C++ SetPhaseEvents PHASE_ONE (boss_malygos.cpp:547): first
    -- EVENT_VORTEX 30-35s after the phase-one start; the intro land
    -- machine is PORTED this tick, so engage follows the iris use +
    -- land and the kit arms at first engage after the land (C++
    -- arms phase one at land+7s; residual deviation: if engage
    -- fires before the land completes, the first vortex is up to
    -- ~7s early).
    schedule(guid, "vortex", math.random(30000, 35000), function()
        vortexTick(creature, guid)
    end)
    -- C++ SetPhaseEvents PHASE_ONE (boss_malygos.cpp:548): first
    -- EVENT_POWER_SPARKS 20-30s after the phase-one start; same
    -- engage-after-land arming (and the same residual ~7s-early
    -- deviation) as the vortex seed above. PORTED 2026-11-08
    -- (tick 432).
    schedule(guid, "powerSparks", math.random(20000, 30000), function()
        powerSparksTick(creature, guid)
    end)
end

-- C++ JustDied (boss_malygos.cpp:979-987): _JustDied() +
-- Talk(SAY_DEATH) + gift-box-bunny GUID ->
-- SummonGameObject(HEART_OF_MAGIC) + SummonCreature
-- NPC_ALEXSTRASZA + DespawnOrUnsummon(5s). All legs PORTED
-- 2026-11-05: state DONE first (the _JustDied twin), Talk,
-- gift-box-bunny SummonGameObject via the instanceData bind
-- (instance_eye_of_eternity.lua), Alexstrasza summon, 5s despawn.
-- The SummonCreature(NPC_ALEXSTRASZA 32295) leg IS ported
-- 2026-10-23 (summoner is me — no instance leg; AlexstraszaSpawnPos
-- boss_malygos.cpp:290; TEMPSUMMON_MANUAL_DESPAWN = 7, the bridge
-- default); Alexstrasza has no C++ AI script, so no behavior is
-- lost. The 5s despawn is a fire-and-forget one-shot
-- CreateLuaEvent — the creature is dead, so no timer bookkeeping
-- is needed (C++ DespawnOrUnsummon rides the creature's own
-- despawn timer, not the EventMap).
local function malygosDied(event, creature, killer)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    vortexState[guid] = nil
    creature:SetBossState(DATA_MALYGOS_EVENT, BOSS_STATE_DONE)
    creature:Talk(SAY_DEATH)
    -- C++ arm order: the gift-box-bunny SummonGameObject leg
    -- (boss_malygos.cpp:982-983) resolves via the instanceData
    -- bind (instance_eye_of_eternity.lua) — the GetGuidData
    -- claim in the header is stale since this tick.
    local mapObj = creature:GetMap()
    if mapObj then
        local data = mapObj:GetInstanceData()
        if data then
            data.malygosDone = true
            if data.giftBoxBunny then
                local heart = GO_HEART_OF_MAGIC_10
                if creature:Is25ManRaid() then
                    heart = GO_HEART_OF_MAGIC_25
                end
                data.giftBoxBunny:SummonGameObject(heart, HEART_OF_MAGIC_SPAWN_X, HEART_OF_MAGIC_SPAWN_Y, HEART_OF_MAGIC_SPAWN_Z, HEART_OF_MAGIC_SPAWN_O, 0)
            end
        end
    end
    creature:SummonCreature(NPC_ALEXSTRASZA, 854.551, 1225.31, 300.901, 0.0, 7, 0)
    CreateLuaEvent(function()
        creature:Despawn()
    end, 5000)
end

-- C++ Reset (boss_malygos.cpp:368-390): _Reset() first (:372, the
-- SetBossState NOT_STARTED != DONE-gated twin, PORTED 2026-11-05
-- below) + Initialize() + SetDisableGravity(true) + SetImmuneToAll
-- + RemoveFlag(NOT_SELECTABLE) + SetReactState(REACT_PASSIVE) +
-- SetBossState(NOT_STARTED) re-asserted last (redundant with the
-- _Reset() leg — same value, same gate outcome when not DONE).
-- The flight-speed (member value, no literal), SetPhase, and
-- achievement legs stay out. The DoAction land clear for the
-- immune flag rides the unported land machine; the 0x100/0x200
-- bits have no Go consumer today, so the Reset apply below is a
-- faithful write with no behavior change until one lands.
local function malygosReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    vortexState[guid] = nil
    -- C++ Reset (boss_malygos.cpp:345/:371/:381): _despawned is the
    -- boss state == FAIL read at AI construction (the ctor; the AI
    -- is rebuilt on respawn, so the read below is the ctor's read);
    -- when true, _Reset() is skipped and
    -- DoAction(ACTION_HANDLE_RESPAWN) runs instead. FAIL is
    -- unobservable in Go (boss_ai.go:195), so despawned is always
    -- false today and the DoAction leg is dormant — the NOT_STARTED
    -- leg keeps its established != DONE gate. PORTED 2026-11-08
    -- (tick 432).
    local despawned = creature:GetBossState(DATA_MALYGOS_EVENT) == BOSS_STATE_FAILED
    if creature:GetBossState(DATA_MALYGOS_EVENT) ~= BOSS_STATE_DONE then
        creature:SetBossState(DATA_MALYGOS_EVENT, BOSS_STATE_NOT_STARTED)
    end
    creature:SetDisableGravity(true)
    creature:SetFlag(59, 0x300) -- SetImmuneToAll(true), C++ order before the NOT_SELECTABLE clear
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    if despawned then
        creature:DoAction(ACTION_HANDLE_RESPAWN)
    end
    creature:SetReactState(0) -- REACT_PASSIVE
    -- C++ SetPhaseEvents PHASE_NOT_STARTED (boss_malygos.cpp:538-541):
    -- EVENT_START_FIRST_RANDOM_PORTAL 2s, group 1 (Reset ->
    -- SetPhase(PHASE_NOT_STARTED, true) re-seeds on every Reset, so
    -- the seed rides the reset, not the spawn); EVENT_SAY_INTRO 1s
    -- stays out (SAY_INTRO_EVENT Talk leg, unbound in Lua).
    -- PORTED 2026-11-11 (row-401 follow-up).
    schedule(guid, "firstPortal", 2000, function()
        firstRandomPortalTick(creature, guid)
    end)
end

RegisterCreatureEvent(ENTRY_MALYGOS, 1, malygosEnterCombat)
RegisterCreatureEvent(ENTRY_MALYGOS, 4, malygosDied)
RegisterCreatureEvent(ENTRY_MALYGOS, 23, malygosReset)
RegisterCreatureEvent(ENTRY_MALYGOS, 28, malygosMovementInform)
RegisterCreatureEvent(ENTRY_MALYGOS, 38, malygosDoAction)

-- C++ spell_malygos_portal_beam::OnApply
-- (boss_malygos.cpp:1607-1611): the Load() gate (:1597-1600)
-- admits creature casters only, and the target must be a creature
-- (ToCreature); the portal target then casts SPELL_PORTAL_OPENED
-- (61236) on itself, non-triggered. The engine fires this hook
-- from the aura-apply path (RegisterAuraEvent, AuraEventOnApply =
-- 2; tick-196 hook). No PreventDefaultAction in C++.
local function portalBeamApply(event, aura, target)
    local caster = aura:GetCaster()
    if caster == nil or caster:GetObjectType() ~= "Creature" then
        return
    end
    if target:GetObjectType() ~= "Creature" then
        return
    end
    target:CastSpell(target, SPELL_PORTAL_OPENED, false)
end

RegisterAuraEvent(SPELL_MALYGOS_PORTAL_BEAM, 2, portalBeamApply)

-- C++ spell_malygos_portal_beam::OnRemove
-- (boss_malygos.cpp:1613-1617): AfterEffectRemove — same Load /
-- ToCreature gates; the portal target removes SPELL_PORTAL_OPENED
-- from itself. The hook fires at the end of the remove funnel,
-- which is the AfterEffectRemove ordering.
local function portalBeamRemove(event, aura, target)
    local caster = aura:GetCaster()
    if caster == nil or caster:GetObjectType() ~= "Creature" then
        return
    end
    if target:GetObjectType() ~= "Creature" then
        return
    end
    target:RemoveAura(SPELL_PORTAL_OPENED)
end

RegisterAuraEvent(SPELL_MALYGOS_PORTAL_BEAM, 3, portalBeamRemove)

local SPELL_ALEXSTRASZAS_GIFT_BEAM_VISUAL = 61023 -- boss_malygos.cpp:157
local GO_ALEXSTRASZA_S_GIFT_10 = 193905 -- eye_of_eternity.h:69
local GO_ALEXSTRASZA_S_GIFT_25 = 193967 -- eye_of_eternity.h:70
local GAMEOBJECT_FLAGS = 9 -- arlokk precedent: OBJECT_END 6 + 3 (Go gameObjectFlags twin)
local GO_FLAG_NOT_SELECTABLE = 16 -- SharedDefines.h:1641
local GO_FLAG_INTERACT_COND = 4 -- SharedDefines.h:1639
local DATA_HEART_OF_MAGIC_GUID = 4 -- eye_of_eternity.h:35-44 (EOEInstanceData64)
local giftBeamGifts = {} -- target GUID -> summoned gift GO GUID (OnRemove leg, below)

-- C++ spell_alexstrasza_gift_beam_visual::OnApply
-- (boss_malygos.cpp:2380-2391): Load admits creature casters only;
-- the creature target summons Alexstrasza's Gift at its own position
-- on exact 10-man-normal / 25-man-normal difficulty only — heroic
-- modes summon nothing (IsHeroic() is the exact-difficulty twin).
-- The OnRemove leg is PORTED
-- below (tick 427) — the GO RemoveFlag bridge is live
-- (engine/world/lua_objects.go:4488), retiring the "no GO flag
-- bridge" stay-out. The outer beam aura's own spell ID is DB-side
-- only (spell_script_names), so its apply (cast 61023) / remove
-- (drop 61023) legs cannot be registered.
local function alexstraszaGiftBeamVisualApply(event, aura, target)
    local caster = aura:GetCaster()
    if caster == nil or caster:GetObjectType() ~= "Creature" then
        return
    end
    if target:GetObjectType() ~= "Creature" then
        return
    end
    -- CORRECTED 2026-11-11: C++ summons only on exact normal
    -- difficulties (boss_malygos.cpp:2384-2387); the old
    -- Is25ManRaid-only pick summoned a gift on heroic modes too.
    if target:IsHeroic() then
        return
    end
    local gift = GO_ALEXSTRASZA_S_GIFT_10
    if target:Is25ManRaid() then
        gift = GO_ALEXSTRASZA_S_GIFT_25
    end
    local giftGuid = target:SummonGameObject(gift, target:GetX(), target:GetY(), target:GetZ(), target:GetO(), 0)
    if giftGuid ~= nil then
        giftBeamGifts[target:GetGUIDLow()] = giftGuid
    end
end

RegisterAuraEvent(SPELL_ALEXSTRASZAS_GIFT_BEAM_VISUAL, 2, alexstraszaGiftBeamVisualApply)

-- C++ spell_alexstrasza_gift_beam_visual::OnRemove
-- (boss_malygos.cpp:2393-2406): no remove-mode guard in C++; the
-- stored gift GO loses GO_FLAG_NOT_SELECTABLE, and the Heart of
-- Magic GO (instance GetGuidData(DATA_HEART_OF_MAGIC_GUID)) loses
-- NOT_SELECTABLE + GO_FLAG_INTERACT_COND (the INTERACT_COND clear
-- is the C++ TO-DO hack line — core lacks flag support; kept, the
-- C++ text is the behavior). PORTED 2026-11-08 (tick 427). The gift
-- GUID is the one stored by the apply handler above, keyed by
-- target GUID; both legs are nil-guarded (the C++ gift pointer is
-- never nil when OnApply ran, but the Lua table is empty if the
-- apply leg never fired). DORMANT: the 61023 aura is applied only
-- by the unported outer beam (DB-side spell id).
local function alexstraszaGiftBeamVisualRemove(event, aura, target)
    if target == nil or target:GetObjectType() ~= "Creature" then
        return
    end
    local guid = target:GetGUIDLow()
    local giftGuid = giftBeamGifts[guid]
    giftBeamGifts[guid] = nil
    if giftGuid ~= nil then
        local gift = target:GetGameObjectByGUID(giftGuid)
        if gift ~= nil then
            gift:RemoveFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE)
        end
    end
    local heartGuid = target:GetGuidData(DATA_HEART_OF_MAGIC_GUID)
    if heartGuid ~= nil and heartGuid ~= 0 then
        local heart = target:GetGameObjectByGUID(heartGuid)
        if heart ~= nil then
            heart:RemoveFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE)
            heart:RemoveFlag(GAMEOBJECT_FLAGS, GO_FLAG_INTERACT_COND)
        end
    end
end

RegisterAuraEvent(SPELL_ALEXSTRASZAS_GIFT_BEAM_VISUAL, 3, alexstraszaGiftBeamVisualRemove)

-- C++ spell_malygos_destroy_platform_channel_AuraScript::OnRemove
-- (boss_malygos.cpp:2025-2057): AfterEffectRemove, EFFECT_0,
-- SPELL_AURA_DUMMY, AURA_EFFECT_HANDLE_REAL. No remove-mode guard in
-- C++: fires on ANY remove. Load() admits creature casters only
-- (:2030-2033); target must be a creature (ToCreature); the
-- Alexstrasza bunny is resolved via instance GetGuidData +
-- map GetCreature and self-casts SPELL_DESTROY_PLATFORM_BOOM_VISUAL,
-- NON-triggered (C++ passes no triggered flag). PORTED 2026-11-08
-- (tick 425). DORMANT twice over: (1) the sender DoCast(me, 58842) at
-- boss_malygos.cpp:716 (POINT_DESTROY_PLATFORM_P_TWO) is unported, so
-- the aura never applies; (2) DATA_ALEXSTRASZA_BUNNY_GUID is never
-- latched in the Go instance guid store — instance_eye_of_eternity.lua
-- binds the bunny to its own Lua table (instanceData.alexstraszaBunny),
-- not via SetGuidData — so GetInstanceCreature(3) reads nil until a
-- latch lands. Nil-guarded, fail-closed.
local SPELL_DESTROY_PLATFORM_CHANNEL = 58842 -- boss_malygos.cpp:139
local SPELL_DESTROY_PLATFORM_BOOM_VISUAL = 59084 -- boss_malygos.cpp:140
local DATA_ALEXSTRASZA_BUNNY_GUID = 3 -- eye_of_eternity.h:35-41, EOEInstanceData64 (DATA_TRIGGER 0, DATA_MALYGOS 1, DATA_PLATFORM 2)

local function destroyPlatformChannelRemove(event, aura, target)
    local caster = aura:GetCaster()
    if caster == nil or caster:GetObjectType() ~= "Creature" then
        return
    end
    if target:GetObjectType() ~= "Creature" then
        return
    end
    local platformTrigger = target:GetInstanceCreature(DATA_ALEXSTRASZA_BUNNY_GUID)
    if platformTrigger then
        platformTrigger:CastSpell(platformTrigger, SPELL_DESTROY_PLATFORM_BOOM_VISUAL)
    end
end

RegisterAuraEvent(SPELL_DESTROY_PLATFORM_CHANNEL, 3, destroyPlatformChannelRemove)
RegisterAuraEvent(SPELL_VORTEX_2, 3, vortexVisualRemove)

-- C++ spell_scion_of_eternity_arcane_barrage::TriggerDamageSpellFromPlayer
-- (boss_malygos.cpp:1940-2020): OnHit. The hit player self-casts
-- Arcane Barrage damage 63934, triggered, with the scion as original
-- caster (Go CastSpell has no orig-caster param — the mimiron
-- accepted-and-ignored precedent). The FilterMeleeHoverDiskPassangers
-- target-selection leg is ported below (ev7); the scion AI itself is
-- unported (entry unverifiable from repo sources), so this is dormant
-- until a scion casts 56397 in Go.
local function scionArcaneBarrageDamage(event, spell, target)
    if target == nil or target:GetObjectType() ~= "Player" then
        return
    end
    target:CastSpell(target, SPELL_ARCANE_BARRAGE_DAMAGE, true)
end
RegisterSpellEvent(SPELL_ARCANE_BARRAGE, 2, scionArcaneBarrageDamage)

local NPC_HOVER_DISK_MELEE = 30234 -- eye_of_eternity.h:53

-- Module-level twin of the malygos-AI GUID store
-- (DATA_LAST_TARGET_BARRAGE_GUID, boss_malygos.cpp:323/432/450):
-- the malygos AI lives in this same file, so a file-local GUIDLow
-- (the magnetic-pull GUIDLow-identity precedent) is the exact store;
-- nil until the first filter pass picks a target.
local lastBarrageTargetGuid = nil

-- C++ IsPlayerOnHoverDisk (boss_malygos.cpp:1922-1937): the unit is
-- on a hover disk iff its vehicle base has entry 30234. Twin:
-- IsOnVehicle returns the vehicle object (or nil) on the player and
-- creature surfaces (lua_player.go:2841, lua_creature_events.go:3288);
-- the vehicle's own GetEntry is the Vehicle.dbc id, so the entry check
-- must go through GetOwner, the base unit (lua_vehicle.go:60).
local function isPlayerOnHoverDisk(target)
    local vehicle = target:IsOnVehicle()
    if vehicle == nil then
        return false
    end
    local base = vehicle:GetOwner()
    if base == nil then
        return false
    end
    return base:GetEntry() == NPC_HOVER_DISK_MELEE
end

-- C++ spell_scion_of_eternity_arcane_barrage::FilterMeleeHoverDiskPassangers
-- (boss_malygos.cpp:1955-2000): OnObjectAreaTargetSelect, EFFECT_0
-- TARGET_UNIT_SRC_AREA_ENEMY. (1) With >1 target, drop the previous
-- pick (malygos AI DATA_LAST_TARGET_BARRAGE_GUID). (2) Prefer targets
-- NOT on a hover disk — only if every target rides one do disk riders
-- stay. (3) Drop targets still carrying the 2s barrage-damage aura
-- 63934 (UnitAuraCheck(true, ...) removes aura bearers; the gothik
-- volley HasAura idiom). (4) Random-resize to 1 and store the pick's
-- GUIDLow in the malygos-AI store twin. Load() admits creature casters
-- with an instance script; the caster-creature leg is checked here,
-- the instance leg is implied (scions exist only in EoE) and the store
-- starts nil. PORTED 2026-11-11. DORMANT until a scion casts 56397.
local function scionArcaneBarrageFilter(event, spell, effectIndex, targets)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetObjectType() ~= "Creature" then
        return
    end
    local kept = targets:GetTargets()
    if #kept == 0 then
        return
    end
    if #kept > 1 and lastBarrageTargetGuid ~= nil then
        local withoutPrevious = {}
        for _, target in ipairs(kept) do
            if target:GetGUIDLow() ~= lastBarrageTargetGuid then
                withoutPrevious[#withoutPrevious + 1] = target
            end
        end
        kept = withoutPrevious
    end
    local withoutDisk = {}
    for _, target in ipairs(kept) do
        if not isPlayerOnHoverDisk(target) then
            withoutDisk[#withoutDisk + 1] = target
        end
    end
    if #withoutDisk > 0 then
        kept = withoutDisk
    end
    local unauraed = {}
    for _, target in ipairs(kept) do
        if not target:HasAura(SPELL_ARCANE_BARRAGE_DAMAGE) then
            unauraed[#unauraed + 1] = target
        end
    end
    kept = unauraed
    if #kept == 0 then
        targets:Clear()
        return
    end
    kept = randomResize(kept, 1)
    lastBarrageTargetGuid = kept[1]:GetGUIDLow()
    targets:Clear()
    for _, target in ipairs(kept) do
        targets:Add(target)
    end
end

RegisterSpellEvent(SPELL_ARCANE_BARRAGE, 7, scionArcaneBarrageFilter)

-- C++ spell_malygos_random_portal::HandleDummy
-- (boss_malygos.cpp:1645-1658): OnEffectHitTarget, EFFECT_0
-- SPELL_EFFECT_DUMMY. Load admits creature casters only; the hit
-- creature is the portal the boss relocates toward — a near point
-- 29.1-30.0 yd out at the target->Malygos angle, target Z — and
-- Malygos MovePoints there. The decimate ev5 idiom
-- (spell:GetCaster, hitTarget, effectIndex). PORTED 2026-11-11.
-- DORMANT: the driver (boss UpdateAI EVENT_RANDOM_PORTAL 14s cycle
-- + EVENT_START_FIRST_RANDOM_PORTAL 2s seed,
-- boss_malygos.cpp:542/:666/:756-758) is the unported intro phase
-- machine is PORTED 2026-11-11 (firstRandomPortalTick seed +
-- portalArrivalTick re-portal, above), but the cast flows through
-- the Go spell path only once Spell.dbc ships, so 56047 is never
-- cast in Go yet.
local function randomPortalDummy(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetObjectType() ~= "Creature" then
        return
    end
    if caster:GetEntry() ~= ENTRY_MALYGOS then
        return
    end
    if hitTarget:GetObjectType() ~= "Creature" then
        return
    end
    local tx, ty, tz = hitTarget:GetX(), hitTarget:GetY(), hitTarget:GetZ()
    local mx, my = caster:GetX(), caster:GetY()
    local angle = math.atan2(my - ty, mx - tx)
    local dist = math.random(291, 300) / 10.0
    caster:MoveTo(POINT_NEAR_RANDOM_PORTAL_P_NONE, tx + dist * math.cos(angle), ty + dist * math.sin(angle), tz)
end

RegisterSpellEvent(SPELL_RANDOM_PORTAL, 5, randomPortalDummy)

local SPELL_ARCANE_STORM_P_I = 61693 -- boss_malygos.cpp:112
local SPELL_ARCANE_STORM_P_III = 57459 -- boss_malygos.cpp:151

-- C++ spell_malygos_arcane_storm::FilterTargets
-- (boss_malygos.cpp:1707-1722): OnObjectAreaTargetSelect, EFFECT_0
-- TARGET_UNIT_SRC_AREA_ENEMY. The list is randomly resized to 4
-- (10-man normal) or 10 (any other difficulty) targets; for
-- SPELL_ARCANE_STORM_P_III only vehicle creatures are kept first
-- (IsCreatureVehicleCheck: TYPEID_UNIT + a vehicle kit). The ev7
-- gothik-volley idiom (targets:GetTargets/Clear/Add). The 10-man
-- heroic leg rides creature:IsHeroic() (C++ counts only
-- RAID_DIFFICULTY_10MAN_NORMAL as 4). PORTED 2026-11-11.
-- DORMANT until Spell.dbc ships (no cast flows through the Go
-- spell path yet). The HandleVisual leg (OnEffectLaunchTarget,
-- caster casts 57473 triggered per hit unit) STAYS OUT — no
-- launch-target Lua event exists (runtime.go spell events end at
-- ev7) and an ev5 substitution would mistime the lead missile.
local function makeArcaneStormFilter(phaseThree)
    return function(event, spell, effectIndex, targets)
        if effectIndex ~= 0 then
            return
        end
        local all = targets:GetTargets()
        if #all == 0 then
            return
        end
        local caster = spell:GetCaster()
        local count = 10
        if caster ~= nil and caster:GetObjectType() == "Creature" and not caster:Is25ManRaid() and not caster:IsHeroic() then
            count = 4
        end
        local kept = all
        if phaseThree then
            kept = {}
            for _, target in ipairs(all) do
                if target:GetObjectType() == "Creature" and target:GetVehicleKit() ~= nil then
                    kept[#kept + 1] = target
                end
            end
        end
        kept = randomResize(kept, count)
        targets:Clear()
        for _, target in ipairs(kept) do
            targets:Add(target)
        end
    end
end

RegisterSpellEvent(SPELL_ARCANE_STORM_P_I, 7, makeArcaneStormFilter(false))
RegisterSpellEvent(SPELL_ARCANE_STORM_P_III, 7, makeArcaneStormFilter(true))

local SPELL_ALIGN_DISK_AGGRO = 61210 -- boss_malygos.cpp:121

-- C++ spell_nexus_lord_align_disk_aggro::HandleScript
-- (boss_malygos.cpp:1905-1910): OnEffectHitTarget, EFFECT_0
-- SPELL_EFFECT_SCRIPT_EFFECT. Load admits creature casters only;
-- the hit creature chases the caster's victim (MoveChase twin).
-- PORTED 2026-11-11. DORMANT: npc_nexus_lord is unported in Go
-- (only header mentions), so 61210 is never cast yet; the 1s
-- EVENT_NUKE_DUMMY driver (boss_malygos.cpp:1350) rides with it.
local function alignDiskAggro(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetObjectType() ~= "Creature" then
        return
    end
    if hitTarget:GetObjectType() ~= "Creature" then
        return
    end
    local victim = caster:GetVictim()
    if victim == nil then
        return
    end
    hitTarget:MoveChase(victim)
end

RegisterSpellEvent(SPELL_ALIGN_DISK_AGGRO, 5, alignDiskAggro)

local SPELL_DESTROY_PLATFORM_EVENT = 59099 -- boss_malygos.cpp:141

-- C++ spell_alexstrasza_bunny_destroy_platform_boom_visual::HandleDummy
-- (boss_malygos.cpp:2078-2082): OnEffectHitTarget, EFFECT_0
-- SPELL_EFFECT_DUMMY. Load admits creature casters only; the hit
-- creature self-casts SPELL_DESTROY_PLATFORM_EVENT, NON-triggered
-- (C++ passes no triggered flag). PORTED 2026-11-11. DORMANT:
-- the chain starts at the 58842 channel removal (row 408, itself
-- dormant on the unported destroy-platform driver), so 59084 is
-- never cast yet.
local function boomVisualDummy(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetObjectType() ~= "Creature" then
        return
    end
    if hitTarget:GetObjectType() ~= "Creature" then
        return
    end
    hitTarget:CastSpell(hitTarget, SPELL_DESTROY_PLATFORM_EVENT)
end

RegisterSpellEvent(SPELL_DESTROY_PLATFORM_BOOM_VISUAL, 5, boomVisualDummy)

local DATA_PLATFORM = 2 -- eye_of_eternity.h:35-41, EOEInstanceData64 (DATA_TRIGGER 0, DATA_MALYGOS 1, DATA_PLATFORM 2)
local GO_FLAG_DESTROYED = 1024 -- SharedDefines.h:1646
local SPELL_SUMMON_RED_DRAGON_BUDDY_F_CAST = 58846 -- boss_malygos.cpp:138

-- C++ spell_alexstrasza_bunny_destroy_platform_event::HandleSendEvent /
-- HandleScript (boss_malygos.cpp:2094-2124): OnEffectHit, EFFECT_0
-- SPELL_EFFECT_SEND_EVENT + EFFECT_2 SPELL_EFFECT_SCRIPT_EFFECT. Load
-- admits creature casters only. eff0: the platform GO (instance
-- GetGuidData(DATA_PLATFORM)) gains GO_FLAG_DESTROYED — the
-- GetGuidData (lua_creature_events.go:6695) + GetGameObjectByGUID
-- (:4025) + GO SetFlag (lua_objects.go:4474, field 9) bridges are all
-- live, the 11:21 sibling stay-out's "no bridge" claim is wrong. eff2:
-- the caster casts SPELL_SUMMON_RED_DRAGON_BUDDY_F_CAST with no
-- explicit target (C++ CastSpell(nullptr, ...)); Go has no no-target
-- cast — nil falls back to victim (vaelastrasz convention) — so the
-- self-cast twin is used: for this implicit-area spell the explicit
-- target only seeds the dest location, which is the caster's either
-- way. PORTED 2026-11-11. DORMANT twice over: (1) the chain starts
-- at the 58842 channel removal (row 408, itself dormant on the
-- unported destroy-platform driver); (2) DATA_PLATFORM is never
-- latched in the Go instance guid store — instance_eye_of_eternity.lua
-- binds the platform to its own Lua table (instanceData.platform),
-- not via SetGuidData, and SetGuidData is creature-surface only
-- (lua_creature_events.go:6680), so the GO cannot latch itself —
-- GetGuidData(2) reads 0 until a latch lands. Nil/zero-guarded,
-- fail-closed (the gift-beam GetGuidData guard precedent).
-- CORRECTED 2026-11-11: the 11:28 landing registered ev5; C++ binds
-- OnEffectHit (boss_malygos.cpp:2125-2126), args (event, spell,
-- effectIndex) (lua_spell.go:585-592) — ev5 multi-fired the eff2 leg
-- per hit target and dead-fired with zero targets.
local function destroyPlatformEvent(event, spell, effectIndex)
    local caster = spell:GetCaster()
    if caster == nil or caster:GetObjectType() ~= "Creature" then
        return
    end
    if effectIndex == 0 then
        local platformGuid = caster:GetGuidData(DATA_PLATFORM)
        if platformGuid == nil or platformGuid == 0 then
            return
        end
        local platform = caster:GetGameObjectByGUID(platformGuid)
        if platform ~= nil then
            platform:SetFlag(GAMEOBJECT_FLAGS, GO_FLAG_DESTROYED)
        end
    elseif effectIndex == 2 then
        caster:CastSpell(caster, SPELL_SUMMON_RED_DRAGON_BUDDY_F_CAST)
    end
end

RegisterSpellEvent(SPELL_DESTROY_PLATFORM_EVENT, 4, destroyPlatformEvent)

local SPELL_RIDE_RED_DRAGON_BUDDY = 56071 -- boss_malygos.cpp:137

-- C++ spell_wyrmrest_skytalon_ride_red_dragon_buddy_trigger::HandleScript
-- (boss_malygos.cpp:2183-2187): OnEffectHitTarget, EFFECT_0
-- SPELL_EFFECT_SCRIPT_EFFECT. Load admits unit casters; the hit unit
-- casts the spell's effect value on the caster, triggered. The unit
-- gate (not the file's usual creature-only gate) is the faithful Load
-- twin here — the body never ToCreatures the caster, and whether the
-- trigger is cast by the buddy or the skytalon rider is DB-side.
-- The binding ID rests on the enum-name match (loader name ==
-- SPELL_RIDE_RED_DRAGON_BUDDY; final confirmation is DB-side,
-- spell_script_names) — no conflicting candidate: 56070 binds the
-- summon script, 56072 is SPELL_RIDE_RED_DRAGON_TRIGGERED, cast
-- directly by the skytalon AI (:1530). GetEffectValue(0) reads the
-- DBC value dynamically (lua_spell.go:395-400, 0 without a DBC row —
-- fail-closed). PORTED 2026-11-11. DORMANT: Spell.dbc not shipped;
-- the driver chain 58846 -> 56070 -> 56071 starts at the
-- destroy-platform event, itself dormant on the unported phase-2
-- machine. Census OnObjectAreaTargetSelect label is bogus — no such
-- hook exists in this class.
local function rideRedDragonBuddyTrigger(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or (caster:GetObjectType() ~= "Creature" and caster:GetObjectType() ~= "Player") then
        return
    end
    local value = spell:GetEffectValue(0)
    if value == nil or value == 0 then
        return
    end
    hitTarget:CastSpell(caster, value, true)
end

RegisterSpellEvent(SPELL_RIDE_RED_DRAGON_BUDDY, 5, rideRedDragonBuddyTrigger)

local SPELL_SURGE_OF_POWER_WARNING_SELECTOR_25 = 60939 -- boss_malygos.cpp:150
local SPELL_SURGE_OF_POWER_PHASE_3_25 = 60936 -- boss_malygos.cpp:149
local EMOTE_SURGE_OF_POWER_WARNING_P3 = 18 -- boss_malygos.cpp:217

-- Module-level twin of the malygos-AI surge GUID store
-- (DATA_FIRST_SURGE_TARGET_GUID 14/15/16, boss_malygos.cpp:320):
-- no SetGUID twin exists for the AI store (SetGuidData latches the
-- calling creature's own GUID into the instance store), so the triple
-- lives here keyed by caster GUIDLow (the giftBeamGifts precedent),
-- reset at each selector cast — the boss AI's pre-cast
-- _surgeTargetGUID clear (boss_malygos.cpp:947-949).
local surgeTargets = {}

-- C++ spell_malygos_surge_of_power_warning_selector_25::SendThreeTargets
-- (boss_malygos.cpp:2219-2246): OnObjectAreaTargetSelect, EFFECT_0
-- TARGET_UNIT_SRC_AREA_ENEMY. Keep only vehicle creatures
-- (IsCreatureVehicleCheck(false) drops everything else — the
-- arcane-storm GetVehicleKit twin); an empty list stays empty (the
-- Clear() below mirrors the post-remove_if list, it does not add
-- targets); random-resize to 3, store each pick's GUIDLow in the
-- surge table in pick order, and whisper the EMOTE_SURGE_OF_POWER
-- warning to the seat-0 player passenger of each picked vehicle
-- (Talk(id, player) whisper-target bridge,
-- lua_creature_events.go:1196; GetPassenger(0), lua_vehicle.go:74).
-- PORTED 2026-11-11. DORMANT: 60939 is cast only by the unported
-- phase-3 machine (DoCastAOE 60939, boss_malygos.cpp:952, 25-man
-- normal) plus Spell.dbc. Census OnEffectApply label is bogus
-- (SpellScript, not AuraScript); AfterHit is the second leg, below.
local function surgeWarningSelectorTargets(event, spell, effectIndex, targets)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetObjectType() ~= "Creature" then
        return
    end
    local vehicles = {}
    for _, target in ipairs(targets:GetTargets()) do
        if target:GetObjectType() == "Creature" and target:GetVehicleKit() ~= nil then
            vehicles[#vehicles + 1] = target
        end
    end
    if #vehicles == 0 then
        targets:Clear()
        return
    end
    local selected = randomResize(vehicles, 3)
    local stored = {}
    for i, target in ipairs(selected) do
        stored[i] = target:GetGUIDLow()
        local passenger = target:GetVehicleKit():GetPassenger(0)
        if passenger ~= nil and passenger:GetObjectType() == "Player" then
            caster:Talk(EMOTE_SURGE_OF_POWER_WARNING_P3, passenger)
        end
    end
    surgeTargets[caster:GetGUIDLow()] = stored
    targets:Clear()
    for _, target in ipairs(selected) do
        targets:Add(target)
    end
end

RegisterSpellEvent(SPELL_SURGE_OF_POWER_WARNING_SELECTOR_25, 7, surgeWarningSelectorTargets)

-- C++ spell_malygos_surge_of_power_warning_selector_25::ExecuteMainSpell
-- (boss_malygos.cpp:2248-2251): AfterHit — the caster casts
-- SPELL_SURGE_OF_POWER_PHASE_3_25 with no explicit target,
-- non-triggered. Twin: the self-cast (the 58846 precedent);
-- AfterHit fires once per hit target (boss_ai.go:4206-4228), so the
-- main spell casts up to 3x per selector cast, faithful to C++
-- (Spell.cpp:2659 per-target AfterHit). PORTED 2026-11-11. DORMANT
-- with the selector leg above.
local function surgeWarningSelectorAfterHit(event, spell, hitTarget)
    local caster = spell:GetCaster()
    if caster == nil or caster:GetObjectType() ~= "Creature" then
        return
    end
    caster:CastSpell(caster, SPELL_SURGE_OF_POWER_PHASE_3_25)
end

RegisterSpellEvent(SPELL_SURGE_OF_POWER_WARNING_SELECTOR_25, 3, surgeWarningSelectorAfterHit)

-- C++ spell_malygos_surge_of_power_25::FilterTargets
-- (boss_malygos.cpp:2281-2301): OnObjectAreaTargetSelect, EFFECT_0
-- TARGET_UNIT_SRC_AREA_ENEMY. Erase every target whose GUID is not
-- one of the AI's stored surge GUIDs (slots 14-16) — the surgeTargets
-- table the selector leg above writes; an unset table keeps nothing,
-- the C++ all-zero-slots twin. PORTED 2026-11-11. DORMANT: 60936 is
-- cast only via the selector's AfterHit, so this rides 412 (plus the
-- phase-3 machine + Spell.dbc). Census OnEffectApply label is bogus.
local function surgeOfPower25Filter(event, spell, effectIndex, targets)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetObjectType() ~= "Creature" then
        return
    end
    local stored = surgeTargets[caster:GetGUIDLow()] or {}
    local kept = {}
    for _, target in ipairs(targets:GetTargets()) do
        for _, guid in ipairs(stored) do
            if target:GetGUIDLow() == guid then
                kept[#kept + 1] = target
                break
            end
        end
    end
    targets:Clear()
    for _, target in ipairs(kept) do
        targets:Add(target)
    end
end

RegisterSpellEvent(SPELL_SURGE_OF_POWER_PHASE_3_25, 7, surgeOfPower25Filter)
