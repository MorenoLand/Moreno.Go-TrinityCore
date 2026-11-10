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
-- takeoff-land points are PORTED (below), and POINT_LAND_P_ONE
-- is PORTED this tick (type-16 arrival, below); the rest has no
-- motion bridge. boss_malygos.cpp:720 POINT_IDLE_P_THREE
-- SetControlled(true, UNIT_STATE_ROOT) rides that same unbridged
-- MovementInform: stays out, except the vortex points (PORTED
-- below). DamageTaken is only the lethal clamp (damage > health &&
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
-- + gift beam visual) — no SpellScript bridge (the varos
-- precedents); AuraScript OnApply/OnRemove ride RegisterAuraEvent
-- (ported in this file, below). The SpellScript legs join the
-- standing queues, except the gift
-- beam visual OnApply (PORTED tick 204, below), the portal
-- beam (ported, below), and the vortex visual OnRemove
-- (PORTED tick 429, below), and the scion arcane-barrage OnHit
-- (PORTED this tick, below); the vortex dummy (56105) is PORTED
-- 2026-11-08 (tick 431, below) via the InstanceData:SetData bridge.
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
        -- group-1 events (EVENT_SAY_INTRO + EVENT_START_FIRST_RANDOM_PORTAL,
        -- PHASE_NOT_STARTED) are not scheduled in Lua, so the cancel
        -- is vacuous here — then MoveLand to the near point of the
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
    if id == POINT_LIFT_IN_AIR_P_ONE then
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
-- the creature target summons Alexstrasza's Gift at its own position,
-- 10-man or 25-man by raid difficulty. The OnRemove leg is PORTED
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
-- target-selection leg stays out (no hover-disk / vehicle-passenger /
-- malygos-SetGUID bridges); the scion AI itself is unported (entry
-- unverifiable from repo sources), so this is dormant until a scion
-- casts 56397 in Go.
local function scionArcaneBarrageDamage(event, spell, target)
    if target == nil or target:GetObjectType() ~= "Player" then
        return
    end
    target:CastSpell(target, SPELL_ARCANE_BARRAGE_DAMAGE, true)
end
RegisterSpellEvent(SPELL_ARCANE_BARRAGE, 2, scionArcaneBarrageDamage)
