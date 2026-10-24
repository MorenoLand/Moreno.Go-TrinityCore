-- Sister Svalna (Icecrown Citadel) — Lua port of
-- src/server/scripts/Northrend/IcecrownCitadel/boss_sister_svalna.cpp
-- (1509 lines incl. license; boss_sister_svalna (CreatureScript,
-- BossAI, DATA_SISTER_SVALNA = 9) + npc_crok_scourgebane (EscortAI)
-- + npc_frostwing_ymirjar_vrykul (ScriptedAI) + npc_impaling_spear
-- (CreatureAI) + 3 SpellScripts (spell_svalna_caress_of_death
-- (spell_trigger_spell_from_caster), spell_svalna_revive_champion,
-- spell_svalna_remove_spear) + 1 AreaTriggerScript
-- (at_icc_start_frostwing_gauntlet); all registered from inside
-- AddSC_boss_sister_svalna() (:1495-1509); loader decl 180 / call
-- 375 per northrend_script_loader.cpp — the TENTH group of the
-- "// Icecrown Citadel" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_blood_queen_lana_thel() (call 374); the call
-- after it is AddSC_boss_valithria_dreamwalker() (call 376) —
-- verified from the loader this run; the checkpoint sequence
-- (blood_queen_lana_thel -> sister_svalna) is followed).
-- The 4 argent captains (npc_captain_arnath / npc_captain_brandon /
-- npc_captain_grondel / npc_captain_rupert over the shared
-- npc_argent_captainAI base) are also registered from this AddSC
-- unit and live in npc_argent_captains.lua.
-- Entries: 37126 Sister Svalna (icecrown_citadel.h NPC_SISTER_SVALNA,
-- line 283; DATA_SISTER_SVALNA = 9, line 83;
-- instance_icecrown_citadel.cpp OnCreatureCreate binds case
-- NPC_SISTER_SVALNA, line 288); 37129 Crok Scourgebane
-- (NPC_CROK_SCOURGEBANE, line 269; OnCreatureCreate line 278);
-- 37132 / 38125 / 37127 / 37134 / 37133 ymirjar (NPC_YMIRJAR_*,
-- lines 278-282; OnCreatureCreate lines 452-456); 38248 impaling
-- spear (NPC_IMPALING_SPEAR, line 284) — entry-verifiable,
-- registration proceeds (the nexus_commanders kalecgos
-- precedent); the CreatureScript ScriptName bindings are DB-side
-- as usual.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_sister_svalna" hits boss_sister_svalna.cpp (+ the
-- loader decl/call lines) only; this clone carries no sql/ tree,
-- so ScriptName bindings are DB-side by construction.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent
-- ({min,max} table = randtime range); GUID-keyed timers table,
-- cancelled on events 2/4/23 (the falric / lady_deathwhisper
-- idiom); melee is engine-driven in Go (creature combat tick),
-- like C++ DoMeleeAttackIfReady.
--
-- DOCTRINE RETRACTION (2026-10-13): the earlier port of this file
-- treated the whole EventMap machine as unbridgeable under the
-- "no-timer-bridge" doctrine and ported Talk arms only. That
-- doctrine is disproved and stale — CreateLuaEvent is a real
-- registered bridge (engine/scripting/runtime.go), and
-- timer-driven cast arms port on the maiden_of_virtue / moroes /
-- curator / nightbane convention: timers keyed by creature GUID,
-- cancelled on events 2/4/23; CastSpell(nil, spellId) =
-- victim-cast (bridge falls back to TargetGUID);
-- CastSpell(creature, spellId) = self-cast (the DoCastAOE
-- convention, boss_ai.go DoCastAOE); {min,max} delay = randtime
-- range; TRIGGERED casts pass true as the third CastSpell arg;
-- Reset()-scheduled timers re-key to event 1; UNIT_STATE_CASTING
-- gates unmodeled. Phase-machine gating is still the real
-- blocker (the gothik precedent): only arms whose gating context
-- is engage / Reset are ported, never arms gated behind the
-- unbridged gauntlet/action machines.
--
-- PORTED ARMS (C++ line refs; all C++-exact for the modeled arms):
-- boss_sister_svalna::JustEngagedWith (:353-362) — DoCastSelf(
--   SPELL_DIVINE_SURGE 71465, true) (:357) = TRIGGERED self-cast
--   on engage; events.ScheduleEvent(EVENT_SVALNA_COMBAT, 9s)
--   (:358) -> Talk(SAY_SVALNA_AGGRO 3) + SetReactState(
--   REACT_DEFENSIVE 1) (:499-502; bridge live — the 2026-10-16
--   "no bridge" note was stale, retracted; lana'thel precedent)
--   modeled as a 9000ms one-shot.
--   The crok->AI()->Talk(SAY_CROK_COMBAT_SVALNA 5) leg (:355-356)
--   is PORTED in svalnaEnterCombat (cross-creature Talk bridge, the
--   mrfloppy/emily precedent — the "no bridge" note below is stale).
-- boss_sister_svalna::JustReachedHome (:389-395) — _JustReachedHome()
--   then SetReactState(REACT_PASSIVE 0); the SetDisableGravity /
--   SetHover legs have no bridge and stay out. Ported as event-24
--   reach-home; pairs with the 9s one-shot DEFENSIVE restore
--   (evade -> PASSIVE, re-engage -> DEFENSIVE, C++-exact).
-- boss_sister_svalna::KilledUnit (:364-388) — victim->GetTypeId()
--   == TYPEID_PLAYER -> Talk(SAY_SVALNA_KILL 4); TYPEID_UNIT with
--   a captain entry (37122-37125) -> Talk(SAY_SVALNA_KILL_CAPTAIN
--   1) — the razuvious player-gated variant precedent plus
--   victim:GetEntry() for creatures.
-- boss_sister_svalna::JustDied (:334-351) — Talk(SAY_SVALNA_DEATH
--   6) (:337) unconditional, the sjonnir JustDied-Talk precedent;
--   the CaptainSurviveTalk cross-AI event legs (:340-350) ride the
--   absent instance / cross-AI bridges.
-- PORTED (2026-10-16): npc_captain_brandon::Reset :1015-1023 +
--   UpdateAI :1039-1051 and npc_captain_grondel::Reset :1075-1082 +
--   UpdateAI :1099-1111 — Reset-seeded timers re-keyed to event 1,
--   all 8 captain entries (living + undead) registered;
--   captain KilledUnit -> Talk(SAY_CAPTAIN_KILL 2) ported this
--   run (:832-835); captainDied cancels timers on event 4 (the
--   brandon/grondel phantom-cast timer leak is fixed); arnath
--   smite (:947/:978-980) + rupert bomb/gun/rocket
--   (:1138-1140/:1157-1169) ported this run with entry-gated
--   undead spell picks; arnath flash-heal/PW-shield stay out on
--   friendly-search legs, dominate-mind on the Is25ManRaid gate,
--   registered for both the living and undead entries (same
--   struct/AI survives UpdateEntry): brandon
--   EVENT_BRANDON_CRUSADER_STRIKE {6s,10s} -> DoCastVictim(
--   SPELL_CRUSADER_STRIKE 71549), re-arm {6s,12s};
--   EVENT_BRANDON_JUDGEMENT_OF_COMMAND {8s,13s} -> DoCastVictim(
--   SPELL_JUDGEMENT_OF_COMMAND 71551), re-arm {8s,13s}; grondel
--   EVENT_GRONDEL_CHARGE_CHECK 500ms -> DoCastVictim(SPELL_CHARGE
--   71553), re-arm 500ms unconditional;
--   EVENT_GRONDEL_MORTAL_STRIKE {8s,14s} -> DoCastVictim(
--   SPELL_MORTAL_STRIKE 71552), re-arm {10s,15s};
--   EVENT_GRONDEL_SUNDER_ARMOR {3s,12s} -> DoCastVictim(
--   SPELL_SUNDER_ARMOR 71554), re-arm {5s,17s}. The
--   npc_argent_captainAI SpellHit SPELL_UNDEATH 70053->70089 arm
--   (:895-921: setDeathState(JUST_RESPAWNED) + UpdateEntry remap +
--   DoCastSelf(SPELL_UNDEATH 70089, true)) is blocked on the
--   setDeathState leg (no bridge) + the never-cast trigger — event 14
--   OnHitBySpell is bridged (boss_ai.go:2052-2096) and UpdateEntry
--   is bridged since 2026-10-22, but the cast alone, without the
--   entry swap + death-state legs, would be C++-inexact — documented-only.
-- EVENT_BRANDON_DIVINE_SHIELD (500ms poll +
--   HealthBelowPct(20) gate) and the IsUndead random-target legs
--   (EVENT_BRANDON_HAMMER_OF_BETRAYAL, EVENT_GRONDEL_CONFLAGRATION)
--   stay parked (not in the docast-missing scan set).
-- (CORRECTION 2026-10-23: boss_sister_svalna.cpp:859-863 is
-- npc_argent_captainAI::JustEngagedWith, NOT crok's — crok
-- (npc_crok_scourgebane :526-809, EscortAI) has NO JustEngagedWith
-- override in C++; the SetHomePosition port formerly in crokEnterCombat
-- was a fabricated leg and is REMOVED. The captain leg itself —
-- SetHomePosition :861 (ported) + the IsUndead-gated DoZoneInCombat
-- :862-863 (stays out: dead latch, set-arm rides the unported
-- setDeathState/UpdateEntry machine) — lives in
-- npc_argent_captains.lua.)
-- npc_crok_scourgebane::CanAIAttack x2 (:792-796, :866-870) — the
--   Y > 2660 Frostwing-Halls gate ("do not see targets inside
--   Frostwing Halls when we are not there") stays out: creature:GetY
--   is bridged (object.go fallback) but the CanAIAttack hook itself
--   has no Lua bridge — documented-only.
-- npc_crok_scourgebane::Reset (:544-551) + UpdateEscortAI
--   (:754-761) — Reset-seeded timers re-keyed to event 1:
--   EVENT_SCOURGE_STRIKE {7500,12500}ms -> DoCastVictim(
--   SPELL_SCOURGE_STRIKE 71488), re-arm {10000,14000}ms;
--   EVENT_DEATH_STRIKE {25000,30000}ms -> gated on
--   HealthBelowPct(20) -> DoCastVictim(SPELL_DEATH_STRIKE 71489),
--   re-arm {5000,10000}ms unconditional (the re-arm sits outside
--   the health-if, :760-761).
-- npc_frostwing_ymirjar_vrykul::JustEngagedWith (:1237-1273) +
--   UpdateAI — per-entry timer seeds, all re-keyed to event 1:
--   frostbinder (37127): DoCast(SPELL_ARCTIC_CHILL 71270) is
--   AITARGET_SELF -> self-cast; EVENT_YMIRJAR_FROZEN_ORB
--   {5000,10000}ms -> DoCast(me, SPELL_FROZEN_ORB_CAST 71274),
--   re-arm {10000,20000}ms.
--   battle_maiden (37132): EVENT_YMIRJAR_BARBARIC_STRIKE
--   {5000,8000}ms -> DoCastVictim(SPELL_BARBARIC_STRIKE 71257),
--   re-arm {2000,4000}ms; EVENT_YMIRJAR_ADRENALINE_RUSH
--   {10000,20000}ms -> me->AddAura(SPELL_ADRENALINE_RUSH 71258,
--   me), re-arm {10000,20000}ms.
--   warlord (37133): EVENT_YMIRJAR_WHIRLWIND {5000,8000}ms ->
--   DoCastAOE(SPELL_WHIRLWIND 41056) = self-cast, re-arm
--   {6000,12000}ms; JustDied (:1215-1220) ->
--   RemoveAurasDueToSpell(SPELL_WARLORDS_PRESENCE 71244) (event 4).
--   huntress (37134): EVENT_YMIRJAR_RAPID_SHOT {10000,20000}ms ->
--   DoCast(SPELL_RAPID_SHOT 71251), AITARGET_VICTIM -> victim-cast,
--   re-arm {20000,30000}ms; EVENT_YMIRJAR_SHOOT 1000ms ->
--   DoCastVictim(SPELL_YMIRJAR_SHOOT 71253), re-arm 2000ms.
--   deathbringer (38125): EVENT_YMIRJAR_SHADOW_BOLT {6000,12000}ms
--   -> DoCastVictim(SPELL_YMIRJAR_SHADOW_BOLT 71296), re-arm
--   {5000,8000}ms; EVENT_YMIRJAR_DEATH_EMBRACE {5000,10000}ms ->
--   DoCast(SPELL_DEATH_EMBRACE 71299), AITARGET_VICTIM ->
--   victim-cast, re-arm {10000,20000}ms.
--
-- DOCUMENTED-ONLY (no bridgeable arm):
-- boss_sister_svalna UpdateAI EVENT_SVALNA_START (:489-491) /
--   EVENT_SVALNA_RESURRECT (:492-495) — Talk(SAY_SVALNA_EVENT_START
--   0) / Talk(SAY_SVALNA_RESURRECT_CAPTAINS 2) + DoCast(me,
--   SPELL_REVIVE_CHAMPION 70053) ride the gauntlet/action
--   machine (scheduled from DoAction legs, :411/:417) — the
--   gothik phase-machine precedent.
-- boss_sister_svalna JustEngagedWith EVENT_IMPALING_SPEAR
--   (:359, 40-50s, re-arm 20-25s, :500-509) — PORTED: self-cast
--   AETHER_SHIELD 71463 then victim-cast IMPALING_SPEAR 71443 on
--   a map-wide random player; the -71443 aura-exclusion leg and
--   the offset-1 tank-skip are documented unbridged caveats.
-- boss_sister_svalna JustEngagedWith EVENT_AETHER_SHIELD
--   (:360, 100-110s) — scheduled but has NO case in the UpdateAI
--   switch: dead event, nothing to port (upstream finding).
-- boss_sister_svalna DoAction (:397-428) — ACTION_KILL_CAPTAIN
--   (DoCastSelf SPELL_CARESS_OF_DEATH 70078 triggered + max
--   targets 1), ACTION_START_GAUNTLET (EVENT_SVALNA_START seed),
--   ACTION_RESURRECT_CAPTAINS (EVENT_SVALNA_RESURRECT seed),
--   ACTION_CAPTAIN_DIES (Talk(SAY_SVALNA_CAPTAIN_DEATH 5)),
--   ACTION_RESET_EVENT — no-DoAction bridge (the krick
--   ACTION_OUTRO precedent).
-- boss_sister_svalna SpellHit (:430-437) — PORTED this run:
--   Talk(EMOTE_SVALNA_BROKEN_SHIELD 8, caster) + RemoveAura(
--   SPELL_AETHER_SHIELD 71463) on SPELL_HURL_SPEAR 71466 via
--   event 14 (the stale "event 15 never fires / no aura bridge"
--   claim is retracted — event 14, HasAura, RemoveAura and
--   Talk(id, target) are all bridged).
-- boss_sister_svalna SpellHitTarget (:452-476) — Talk(
--   EMOTE_SVALNA_IMPALE 7, unitTarget) on SPELL_IMPALING_SPEAR
--   71443 rides the SpellHitTarget arm — event 15 IS bridged
--   (boss_ai.go:2196-2206) but the stay-out holds: the
--   SPELL_IMPALING_SPEAR_KILL Unit::Kill leg and the
--   NPC_IMPALING_SPEAR summon + vehicle-cast legs have no bridge
--   (DespawnOrUnsummon/GetVehicle zero hits), and the Talk is
--   conditional on summon success — Talk-alone would be
--   C++-inexact.
-- boss_sister_svalna JustReachedHome (:389-395) /
--   MovementInform (:439-450) — SetReactState / SetDisableGravity
--   / SetHover legs, no Talk of Svalna's own (no MovementInform
--   bridge).
-- boss_sister_svalna EnterEvadeMode (:872-885) — the
--   GetVehicle-gated MoveFollow(NPC_CROK_SCOURGEBANE) leg (:882-885)
--   rides the unbridged EnterEvadeMode hook (event 25 unused;
--   boss_ai.go:5753-5754) plus the unported vehicle/motion machines —
--   stays out. The _vehicleCheckTimer 500ms UpdateAI leg (:1403-1410,
--   DespawnOrUnsummon(100ms) when !me->GetVehicle()) rides the same
--   unported vehicle machine — stays out.
-- npc_crok_scourgebane DoAction (:553-589) — ACTION_START_GAUNTLET
--   intro Talk(SAY_CROK_INTRO_1 0) + EVENT_ARNATH_INTRO_2 (7s,
--   cross-AI Talk SAY_ARNATH_INTRO_2 5) + EVENT_CROK_INTRO_3 (14s,
--   Talk(SAY_CROK_INTRO_3 1)) + EVENT_START_PATHING (35s,
--   Start(true,true)) — machine-gated escort legs, no escort /
--   DoAction bridge (joins the escort queue).
-- npc_crok_scourgebane DamageTaken (:691-731) — the wipe check
--   (player search, damage *= 100, FrostwingGauntletRespawner
--   machine) and the 10%-latch chew machine (damage = 0 latch +
--   1s EVENT_HEALTH_CHECK loop with ModifyHealth(+5% max) and
--   HealthAbovePct(15) re-arm) are not a single-flip floored
--   latch. Stays parked — the corrected blockers are the
--   FrostwingGauntletRespawner mass-DespawnOrUnsummon+reposition
--   machine (DespawnOrUnsummon unbridged), cross-AI
--   DoAction(ACTION_RESET_EVENT), ModifyHealth (zero hits) and
--   the _isEventActive-gated Talk; event 9 DOES support numeric
--   damage rewrite (lua_creature_events.go:303-324), so the old
--   "event-9 cannot express damage rewrite" claim was stale.
-- npc_crok_scourgebane SetGUID / WaypointReached / WaypointStarted
--   (:591-689) — gauntlet waypoint machine, no bridge.
-- npc_frostwing_ymirjar_vrykul UpdateAI random-target arms —
--   EVENT_YMIRJAR_TWISTED_WINDS (71306), EVENT_YMIRJAR_ICE_TRAP
--   (71249), EVENT_YMIRJAR_VOLLEY (71252), EVENT_YMIRJAR_BANISH
--   (71298) PORTED via the map-wide randomPlayerTarget pool
--   (the skadi precedent); EVENT_YMIRJAR_SUMMON (71303) stays
--   parked (summon machine + _events.DelayEvents(2s) leg).
-- npc_frostwing_ymirjar_vrykul Reset (:1189-1213) — the
--   RemoveAurasDueToSpell(SPELL_ARCTIC_CHILL) leg is PORTED (stale "no aura
--   bridge on this path" claim retracted — creature:RemoveAura bridged);
--   the OOC EVENT_YMIRJAR_SPIRIT_STREAM
--   (69929, {10000,20000}ms, re-arm {40000,60000}ms) fires while
--   !UpdateVictim() — no OOC-tick bridge; the huntress
--   Is25ManRaid() DoCast(SPELL_SUMMON_WARHAWK 71705) leg is a
--   summon machine + difficulty gate.
-- npc_impaling_spear (:1385-1413) — the whole AI is a 500ms
--   vehicle-check despawn timer (no bridgeable arm; NOT
--   registered).
-- The 3 spell scripts join the no-SpellScript-bridge queue (the
--   boss_moragg optic-link precedent);
--   at_icc_start_frostwing_gauntlet joins the no-AreaTrigger
--   bridge queue (the boss_anubrekhan at_anubrekhan_entrance
--   precedent).
-- All svalna Talk() calls accounted for (10 = SAY_SVALNA_KILL 4
--   (1) + SAY_SVALNA_KILL_CAPTAIN 1 (1) + SAY_SVALNA_DEATH 6 (1)
--   + SAY_SVALNA_CAPTAIN_DEATH 5 (1, DoAction) +
--   EMOTE_SVALNA_BROKEN_SHIELD 8 (1, SpellHit) + EMOTE_SVALNA_IMPALE
--   7 (1, SpellHitTarget) + SAY_SVALNA_EVENT_START 0 (1,
--   scheduler) + SAY_SVALNA_RESURRECT_CAPTAINS 2 (1, scheduler) +
--   SAY_SVALNA_AGGRO 3 (1, scheduler)).
-- Go registration check this run: RegisterLuaBoss(
--   "boss_sister_svalna", 37126) exists (boss_ai.go:10508);
--   npc_crok_scourgebane (37129), the 5 ymirjar entries
--   (37132/38125/37127/37134/37133) and the 4 undead captain
--   entries (37491/37493/37494/37495) were missing and are added
--   by this tick — the Lua handlers below are inert without
--   them.

local ENTRY_SISTER_SVALNA = 37126
local ENTRY_CROK_SCOURGEBANE = 37129

local ENTRY_YMIRJAR_BATTLE_MAIDEN = 37132
local ENTRY_YMIRJAR_DEATHBRINGER = 38125
local ENTRY_YMIRJAR_FROSTBINDER = 37127
local ENTRY_YMIRJAR_HUNTRESS = 37134
local ENTRY_YMIRJAR_WARLORD = 37133

local NPC_CAPTAIN_ARNATH = 37122
local NPC_CAPTAIN_BRANDON = 37123
local NPC_CAPTAIN_GRONDEL = 37124
local NPC_CAPTAIN_RUPERT = 37125
local NPC_CAPTAIN_ARNATH_UNDEAD = 37491 -- icecrown_citadel.h:274
local NPC_CAPTAIN_BRANDON_UNDEAD = 37493 -- icecrown_citadel.h:275
local NPC_CAPTAIN_GRONDEL_UNDEAD = 37494 -- icecrown_citadel.h:276
local NPC_CAPTAIN_RUPERT_UNDEAD = 37495 -- icecrown_citadel.h:277

local SAY_SVALNA_KILL_CAPTAIN = 1
local SAY_SVALNA_AGGRO = 3
local SAY_SVALNA_KILL = 4
local SAY_SVALNA_DEATH = 6
local SAY_CROK_COMBAT_SVALNA = 5 -- boss_sister_svalna.cpp:58

local SAY_CAPTAIN_DEATH = 0
local SAY_CAPTAIN_SECOND_DEATH = 3
local SAY_CAPTAIN_KILL = 2 -- :62

local SPELL_DIVINE_SURGE = 71465

local SPELL_SCOURGE_STRIKE = 71488
local SPELL_DEATH_STRIKE = 71489

local SPELL_ARCTIC_CHILL = 71270
local SPELL_FROZEN_ORB_CAST = 71274
local SPELL_BARBARIC_STRIKE = 71257
local SPELL_ADRENALINE_RUSH = 71258
local SPELL_WHIRLWIND = 41056
local SPELL_RAPID_SHOT = 71251
local SPELL_YMIRJAR_SHOOT = 71253
local SPELL_YMIRJAR_SHADOW_BOLT = 71296
local SPELL_DEATH_EMBRACE = 71299
local SPELL_WARLORDS_PRESENCE = 71244

local SPELL_CRUSADER_STRIKE = 71549 -- :1040
local SPELL_JUDGEMENT_OF_COMMAND = 71551 -- :1049
local SPELL_DIVINE_SHIELD = 71550 -- :98
local SPELL_HAMMER_OF_BETRAYAL = 71784 -- :100
local SPELL_SMITE_NORMAL = 71546 -- :91
local SPELL_SMITE_UNDEAD = 71778 -- :94
local SPELL_FEL_IRON_BOMB_NORMAL = 71592 -- :109
local SPELL_FEL_IRON_BOMB_UNDEAD = 71787 -- :112
local SPELL_MACHINE_GUN_NORMAL = 71594 -- :110
local SPELL_MACHINE_GUN_UNDEAD = 71788 -- :113
local SPELL_ROCKET_LAUNCH_NORMAL = 71590 -- :111
local SPELL_ROCKET_LAUNCH_UNDEAD = 71786 -- :114
local SPELL_CONFLAGRATION = 71785 -- :106
local SPELL_CHARGE = 71553 -- :1100
local SPELL_MORTAL_STRIKE = 71552 -- :1104
local SPELL_SUNDER_ARMOR = 71554 -- :1108

local SPELL_TWISTED_WINDS = 71306
local SPELL_ICE_TRAP = 71249
local SPELL_VOLLEY = 71252
local SPELL_BANISH = 71298
local SPELL_IMPALING_SPEAR = 71443
local SPELL_AETHER_SHIELD = 71463
local SPELL_HURL_SPEAR = 71466 -- :84
local EMOTE_SVALNA_BROKEN_SHIELD = 8 -- :48

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

-- bronjahm-style: re-scheduling a key replaces the pending timer,
-- so each named arm has exactly one live timer at a time.
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

-- boss_skadi randomPlayerTarget precedent: map/instance-scoped
-- alive-player pool, optional max-distance filter. C++
-- SelectTarget offset-1 tank-skip and aura-exclusion legs have
-- no bridge; the pool caveat is documented, not blocking.
local function randomPlayerTarget(creature, maxDist)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local candidates = {}
    for _, plr in ipairs(GetPlayersInWorld()) do
        if plr:GetMapId() == mapId and plr:GetInstanceId() == instanceId
                and not plr:IsDead()
                and (not maxDist or creature:IsWithinDist(plr, maxDist)) then
            candidates[#candidates + 1] = plr
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ boss_sister_svalna::JustEngagedWith :353-362 — DoCastSelf(
-- SPELL_DIVINE_SURGE, true) is a TRIGGERED self-cast; the
-- EVENT_SVALNA_COMBAT seed (9s) is a one-shot Talk arm.
-- EVENT_IMPALING_SPEAR (:359 40-50s; :500-509) — SelectTarget(
-- Random, 1, 0.0f, true, true, -SPELL_IMPALING_SPEAR 71443) ->
-- DoCast(me, SPELL_AETHER_SHIELD 71463) then DoCast(target,
-- SPELL_IMPALING_SPEAR 71443), re-arm 20-25s unconditional
-- (re-scheduled outside the target guard). The -71443
-- aura-exclusion leg and the offset-1 tank-skip are documented
-- unbridged caveats, not blockers.
-- C++ :499-502: SetReactState(REACT_DEFENSIVE 1) then Talk(
-- SAY_SVALNA_AGGRO 3) — arm order kept.
local function onSvalnaCombat(creature)
    creature:SetReactState(1)
    creature:Talk(SAY_SVALNA_AGGRO)
end

local function onSvalnaImpalingSpear(creature, guid)
    local target = randomPlayerTarget(creature)
    if target then
        creature:CastSpell(creature, SPELL_AETHER_SHIELD)
        creature:CastSpell(target, SPELL_IMPALING_SPEAR)
    end
    schedule(guid, "impaling", {20000, 25000},
        function() onSvalnaImpalingSpear(creature, guid) end)
end

local function svalnaEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    -- C++ JustEngagedWith (boss_sister_svalna.cpp:355-356):
    -- crok->AI()->Talk(SAY_CROK_COMBAT_SVALNA) — cross-creature Talk
    -- bridged via FindNearestCreature (mrfloppy/emily precedent); the
    -- stale "cross-AI Talk (no bridge)" header note is retracted for
    -- this arm (crok IS registered, ENTRY_CROK_SCOURGEBANE 37129).
    local crok = creature:FindNearestCreature(ENTRY_CROK_SCOURGEBANE, 40.0)
    if crok ~= nil then
        crok:Talk(SAY_CROK_COMBAT_SVALNA)
    end
    creature:CastSpell(creature, SPELL_DIVINE_SURGE, true)
    schedule(guid, "combat", 9000, function() onSvalnaCombat(creature) end)
    schedule(guid, "impaling", {40000, 50000},
        function() onSvalnaImpalingSpear(creature, guid) end)
end

local function svalnaLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ boss_sister_svalna::KilledUnit :364-388:
-- if (victim->GetTypeId() == TYPEID_PLAYER) Talk(SAY_SVALNA_KILL);
-- if (victim->GetTypeId() == TYPEID_UNIT and GetEntry() in the 4
-- captain entries) Talk(SAY_SVALNA_KILL_CAPTAIN) — the razuvious
-- player-gated variant precedent plus victim:GetEntry() for
-- creatures (the razuvious DK-understudy leg).
local function isArgentCaptain(entry)
    return entry == NPC_CAPTAIN_ARNATH or entry == NPC_CAPTAIN_BRANDON
        or entry == NPC_CAPTAIN_GRONDEL or entry == NPC_CAPTAIN_RUPERT
end

local function svalnaKilledUnit(event, creature, victim)
    if victim then
        local otype = victim:GetObjectType()
        if otype == "Player" then
            creature:Talk(SAY_SVALNA_KILL)
        elseif otype == "Creature" and isArgentCaptain(victim:GetEntry()) then
            creature:Talk(SAY_SVALNA_KILL_CAPTAIN)
        end
    end
end

-- C++ boss_sister_svalna::JustDied :334-351:
-- Talk(SAY_SVALNA_DEATH) — the sjonnir JustDied-Talk precedent;
-- the CaptainSurviveTalk cross-AI legs have no bridge.
local function svalnaJustDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(SAY_SVALNA_DEATH)
end

-- C++ boss_sister_svalna::Reset :326-331 — _Reset() then
-- SetReactState(REACT_DEFENSIVE 1); the _isEventInProgress latch has
-- no bridge. Ported in C++ arm order (the venoxis ev23 precedent).
-- The InitializeAI (:320-324) spawn-time REACT_PASSIVE stays out:
-- the intro DoAction machine is unported and porting PASSIVE alone
-- would strand her passive (the tyrannus precedent).
local function svalnaReset(event, creature)
    cancelTimers(creature:GetGUID())
    creature:SetReactState(1) -- REACT_DEFENSIVE
end

-- C++ boss_sister_svalna::JustReachedHome :389-395 —
-- _JustReachedHome() then SetReactState(REACT_PASSIVE 0),
-- SetDisableGravity(false) (:393), SetHover(false) (:394); bridges live
-- (lua_creature_events.go).
local function svalnaReachHome(event, creature)
    creature:SetReactState(0)
    creature:SetDisableGravity(false)
    creature:SetHover(false)
end

RegisterCreatureEvent(ENTRY_SISTER_SVALNA, 1, svalnaEnterCombat)
RegisterCreatureEvent(ENTRY_SISTER_SVALNA, 2, svalnaLeaveCombat)
RegisterCreatureEvent(ENTRY_SISTER_SVALNA, 3, svalnaKilledUnit)
RegisterCreatureEvent(ENTRY_SISTER_SVALNA, 4, svalnaJustDied)
RegisterCreatureEvent(ENTRY_SISTER_SVALNA, 23, svalnaReset)
RegisterCreatureEvent(ENTRY_SISTER_SVALNA, 24, svalnaReachHome)

-- C++ boss_sister_svalna::SpellHit :430-437 — ungated arm:
-- if (spellInfo->Id == SPELL_HURL_SPEAR (71466) &&
--     me->HasAura(SPELL_AETHER_SHIELD (71463)))
--   { me->RemoveAurasDueToSpell(SPELL_AETHER_SHIELD);
--     Talk(EMOTE_SVALNA_BROKEN_SHIELD (8), caster); }
-- Event 14 is bridged (boss_ai.go:2052-2096 creature hits, spells.go:11709-11730
-- player hits; args = (event, creature, caster, spellId)); HasAura /
-- RemoveAura / Talk(id, target) all bridged. Idempotent: the aura is
-- gone on re-fire. The stale "event 15 never fires / no aura bridge"
-- header claim is retracted.
local function svalnaSpellHit(event, creature, caster, spellId)
    if spellId == SPELL_HURL_SPEAR and creature:HasAura(SPELL_AETHER_SHIELD) then
        creature:RemoveAura(SPELL_AETHER_SHIELD)
        creature:Talk(EMOTE_SVALNA_BROKEN_SHIELD, caster)
    end
end

RegisterCreatureEvent(ENTRY_SISTER_SVALNA, 14, svalnaSpellHit)

-- C++ npc_crok_scourgebane::Reset :544-551 seeds
-- EVENT_SCOURGE_STRIKE {7500,12500}ms and EVENT_DEATH_STRIKE
-- {25000,30000}ms; UpdateEscortAI :754-761: scourge strike ->
-- DoCastVictim(71488), re-arm {10000,14000}ms; death strike ->
-- gated on HealthBelowPct(20) -> DoCastVictim(71489), re-arm
-- {5000,10000}ms unconditional. Re-keyed to event 1 (the Reset
-- re-key convention).
local function onCrokScourgeStrike(creature, guid)
    creature:CastSpell(nil, SPELL_SCOURGE_STRIKE)
    schedule(guid, "scourge", {10000, 14000},
        function() onCrokScourgeStrike(creature, guid) end)
end

local function onCrokDeathStrike(creature, guid)
    local maxHealth = creature:GetMaxHealth()
    if maxHealth > 0
            and creature:GetHealth() < math.floor(maxHealth * 20 / 100) then
        creature:CastSpell(nil, SPELL_DEATH_STRIKE)
    end
    schedule(guid, "death", {5000, 10000},
        function() onCrokDeathStrike(creature, guid) end)
end

-- crok has NO JustEngagedWith in C++ (npc_crok_scourgebane :526-809,
-- EscortAI, no override); the scourge/death-strike seeds below re-key
-- the DoAction gauntlet arms (:547-548) to combat start (svalna precedent).
local function crokEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "scourge", {7500, 12500},
        function() onCrokScourgeStrike(creature, guid) end)
    schedule(guid, "death", {25000, 30000},
        function() onCrokDeathStrike(creature, guid) end)
end

local function crokLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function crokDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function crokReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_CROK_SCOURGEBANE, 1, crokEnterCombat)
RegisterCreatureEvent(ENTRY_CROK_SCOURGEBANE, 2, crokLeaveCombat)
RegisterCreatureEvent(ENTRY_CROK_SCOURGEBANE, 4, crokDied)
RegisterCreatureEvent(ENTRY_CROK_SCOURGEBANE, 23, crokReset)

-- C++ npc_frostwing_ymirjar_vrykul::JustEngagedWith :1237-1273 —
-- per-entry timer seeds, all re-keyed to event 1 (see the PORTED
-- ARMS header note for the C++-exact arm list). The
-- InterruptNonMeleeSpells(false) (:1242) leg is dropped: StopSpellCast
-- is an explicit no-op for creatures (lua_objects.go:332 — packet-only
-- casts, no cast state to interrupt). Single-arg
-- DoCast() resolves via UnitAI::DoCast: AITARGET_SELF (arctic
-- chill) -> self-cast; AITARGET_VICTIM (rapid shot, death
-- embrace) -> victim-cast.
local function onYmirjarFrozenOrb(creature, guid)
    creature:CastSpell(creature, SPELL_FROZEN_ORB_CAST)
    schedule(guid, "frozen", {10000, 20000},
        function() onYmirjarFrozenOrb(creature, guid) end)
end

local function onYmirjarBarbaricStrike(creature, guid)
    creature:CastSpell(nil, SPELL_BARBARIC_STRIKE)
    schedule(guid, "barbaric", {2000, 4000},
        function() onYmirjarBarbaricStrike(creature, guid) end)
end

local function onYmirjarAdrenalineRush(creature, guid)
    creature:AddAura(SPELL_ADRENALINE_RUSH)
    schedule(guid, "adrenaline", {10000, 20000},
        function() onYmirjarAdrenalineRush(creature, guid) end)
end

local function onYmirjarWhirlwind(creature, guid)
    creature:CastSpell(creature, SPELL_WHIRLWIND)
    schedule(guid, "whirlwind", {6000, 12000},
        function() onYmirjarWhirlwind(creature, guid) end)
end

local function onYmirjarRapidShot(creature, guid)
    creature:CastSpell(nil, SPELL_RAPID_SHOT)
    schedule(guid, "rapid", {20000, 30000},
        function() onYmirjarRapidShot(creature, guid) end)
end

local function onYmirjarShoot(creature, guid)
    creature:CastSpell(nil, SPELL_YMIRJAR_SHOOT)
    schedule(guid, "shoot", 2000,
        function() onYmirjarShoot(creature, guid) end)
end

local function onYmirjarShadowBolt(creature, guid)
    creature:CastSpell(nil, SPELL_YMIRJAR_SHADOW_BOLT)
    schedule(guid, "shadowbolt", {5000, 8000},
        function() onYmirjarShadowBolt(creature, guid) end)
end

local function onYmirjarDeathEmbrace(creature, guid)
    creature:CastSpell(nil, SPELL_DEATH_EMBRACE)
    schedule(guid, "embrace", {10000, 20000},
        function() onYmirjarDeathEmbrace(creature, guid) end)
end

-- C++ EVENT_YMIRJAR_TWISTED_WINDS :1315-1317 — SelectTarget(
-- Random, 0, 0.f, true) -> DoCast(target, SPELL_TWISTED_WINDS
-- 71306), Repeat {10s,20s} unconditional (outside the target
-- guard); seeded :1249 {10s,20s} on the frostbinder.
local function onYmirjarTwistedWinds(creature, guid)
    local target = randomPlayerTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_TWISTED_WINDS)
    end
    schedule(guid, "twisted", {10000, 20000},
        function() onYmirjarTwistedWinds(creature, guid) end)
end

-- C++ EVENT_YMIRJAR_ICE_TRAP :1336-1339 — SelectTarget(Random, 1,
-- 20.0f, true) -> DoCast(target, SPELL_ICE_TRAP 71249), Repeat
-- {15s,20s} unconditional; seeded :1260 {5s,10s} on the
-- huntress.
local function onYmirjarIceTrap(creature, guid)
    local target = randomPlayerTarget(creature, 20)
    if target then
        creature:CastSpell(target, SPELL_ICE_TRAP)
    end
    schedule(guid, "icetrap", {15000, 20000},
        function() onYmirjarIceTrap(creature, guid) end)
end

-- C++ EVENT_YMIRJAR_VOLLEY :1341-1344 — SelectTarget(Random, 1,
-- 40.0f, true) -> DoCast(target, SPELL_VOLLEY 71252), Repeat
-- {10s,20s} unconditional; seeded :1261 {20s,30s} on the
-- huntress.
local function onYmirjarVolley(creature, guid)
    local target = randomPlayerTarget(creature, 40)
    if target then
        creature:CastSpell(target, SPELL_VOLLEY)
    end
    schedule(guid, "volley", {10000, 20000},
        function() onYmirjarVolley(creature, guid) end)
end

-- C++ EVENT_YMIRJAR_BANISH :1358-1361 — SelectTarget(Random, 1,
-- 20.0f, true) -> DoCast(target, SPELL_BANISH 71298), Repeat
-- {10s,20s} unconditional; seeded :1267 {10s,20s} on the
-- deathbringer.
local function onYmirjarBanish(creature, guid)
    local target = randomPlayerTarget(creature, 20)
    if target then
        creature:CastSpell(target, SPELL_BANISH)
    end
    schedule(guid, "banish", {10000, 20000},
        function() onYmirjarBanish(creature, guid) end)
end

local function ymirjarEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    local entry = creature:GetEntry()
    if entry == ENTRY_YMIRJAR_FROSTBINDER then
        creature:CastSpell(creature, SPELL_ARCTIC_CHILL)
        schedule(guid, "frozen", {5000, 10000},
            function() onYmirjarFrozenOrb(creature, guid) end)
        schedule(guid, "twisted", {10000, 20000},
            function() onYmirjarTwistedWinds(creature, guid) end)
    elseif entry == ENTRY_YMIRJAR_BATTLE_MAIDEN then
        schedule(guid, "barbaric", {5000, 8000},
            function() onYmirjarBarbaricStrike(creature, guid) end)
        schedule(guid, "adrenaline", {10000, 20000},
            function() onYmirjarAdrenalineRush(creature, guid) end)
    elseif entry == ENTRY_YMIRJAR_WARLORD then
        schedule(guid, "whirlwind", {5000, 8000},
            function() onYmirjarWhirlwind(creature, guid) end)
    elseif entry == ENTRY_YMIRJAR_HUNTRESS then
        schedule(guid, "rapid", {10000, 20000},
            function() onYmirjarRapidShot(creature, guid) end)
        schedule(guid, "icetrap", {5000, 10000},
            function() onYmirjarIceTrap(creature, guid) end)
        schedule(guid, "volley", {20000, 30000},
            function() onYmirjarVolley(creature, guid) end)
        schedule(guid, "shoot", 1000,
            function() onYmirjarShoot(creature, guid) end)
    elseif entry == ENTRY_YMIRJAR_DEATHBRINGER then
        schedule(guid, "shadowbolt", {6000, 12000},
            function() onYmirjarShadowBolt(creature, guid) end)
        schedule(guid, "embrace", {5000, 10000},
            function() onYmirjarDeathEmbrace(creature, guid) end)
        schedule(guid, "banish", {10000, 20000},
            function() onYmirjarBanish(creature, guid) end)
    end
end

local function ymirjarLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ npc_frostwing_ymirjar_vrykul::JustDied :1215-1220 — only
-- the warlord does anything: RemoveAurasDueToSpell(
-- SPELL_WARLORDS_PRESENCE).
local function ymirjarDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    if creature:GetEntry() == ENTRY_YMIRJAR_WARLORD then
        creature:RemoveAura(SPELL_WARLORDS_PRESENCE)
    end
end

local function ymirjarReset(event, creature)
    cancelTimers(creature:GetGUID())
    -- cpp:1196-1199 — Reset entry-gated RemoveAurasDueToSpell(SPELL_ARCTIC_CHILL)
    -- (ungated in C++ — no HasAura query); the stale "no aura bridge on this
    -- path" claim is retracted — creature:RemoveAura is bridged. The C++
    -- !IsAlive() early return guards the unbridged OOC-event/summon legs
    -- below; the strip leg itself is safe on reset.
    if creature:GetEntry() == ENTRY_YMIRJAR_FROSTBINDER then
        creature:RemoveAura(SPELL_ARCTIC_CHILL)
    end
end

local YMIRJAR_ENTRIES = {
    ENTRY_YMIRJAR_BATTLE_MAIDEN,
    ENTRY_YMIRJAR_DEATHBRINGER,
    ENTRY_YMIRJAR_FROSTBINDER,
    ENTRY_YMIRJAR_HUNTRESS,
    ENTRY_YMIRJAR_WARLORD,
}

for _, entry in ipairs(YMIRJAR_ENTRIES) do
    RegisterCreatureEvent(entry, 1, ymirjarEnterCombat)
    RegisterCreatureEvent(entry, 2, ymirjarLeaveCombat)
    RegisterCreatureEvent(entry, 4, ymirjarDied)
    RegisterCreatureEvent(entry, 23, ymirjarReset)
end

-- C++ npc_captain_brandon::Reset :1015-1023 + UpdateAI :1039-1051 —
-- Reset-seeded timers re-keyed to event 1 (the Reset re-key
-- convention): EVENT_BRANDON_CRUSADER_STRIKE {6s,10s} ->
-- DoCastVictim(SPELL_CRUSADER_STRIKE 71549), re-arm {6s,12s};
-- EVENT_BRANDON_JUDGEMENT_OF_COMMAND {8s,13s} ->
-- DoCastVictim(SPELL_JUDGEMENT_OF_COMMAND 71551), re-arm
-- {8s,13s}. The undead entry (37493) runs the same struct/AI
-- (UpdateEntry keeps the AI instance) — same seeds, same arms.
-- PORTED this run: EVENT_BRANDON_DIVINE_SHIELD 500ms poll
-- (HealthBelowPct(20) -> self-cast 71550, unconditional 500ms
-- re-arm) + EVENT_BRANDON_HAMMER_OF_BETRAYAL {25s,30s} ->
-- randomPlayerTarget -> 71784, re-arm {45s,60s} (undead-entry
-- gated; the position-1 offset has no bridge — tyrannus
-- precedent).
local function onBrandonCrusaderStrike(creature, guid)
    creature:CastSpell(nil, SPELL_CRUSADER_STRIKE)
    schedule(guid, "crusader", {6000, 12000},
        function() onBrandonCrusaderStrike(creature, guid) end)
end

local function onBrandonJudgementOfCommand(creature, guid)
    creature:CastSpell(nil, SPELL_JUDGEMENT_OF_COMMAND)
    schedule(guid, "judgement", {8000, 13000},
        function() onBrandonJudgementOfCommand(creature, guid) end)
end

-- C++ EVENT_BRANDON_DIVINE_SHIELD :1043-1046 — 500ms poll, ungated:
-- if (HealthBelowPct(20)) DoCast(me, SPELL_DIVINE_SHIELD 71550);
-- re-arm 500ms unconditional (outside the health-if, crok
-- death-strike pattern).
local function onBrandonDivineShield(creature, guid)
    local maxHealth = creature:GetMaxHealth()
    if maxHealth > 0
            and creature:GetHealth() < math.floor(maxHealth * 20 / 100) then
        creature:CastSpell(creature, SPELL_DIVINE_SHIELD)
    end
    schedule(guid, "divine", 500,
        function() onBrandonDivineShield(creature, guid) end)
end

-- C++ EVENT_BRANDON_HAMMER_OF_BETRAYAL :1053-1055 — seeded only when
-- IsUndead (Reset :1021-1022; IsUndead flips exactly with the entry
-- remap to 37493, :896-898) — SelectTarget(Random, 1, 0.0f, true) ->
-- DoCast(target, SPELL_HAMMER_OF_BETRAYAL 71784) non-triggered;
-- re-arm {45s,60s} unconditional. The position-1 offset has no
-- bridge — plain random alive player (the tyrannus precedent).
local function onBrandonHammerOfBetrayal(creature, guid)
    local target = randomPlayerTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_HAMMER_OF_BETRAYAL)
    end
    schedule(guid, "hammer", {45000, 60000},
        function() onBrandonHammerOfBetrayal(creature, guid) end)
end

local function brandonEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "crusader", {6000, 10000},
        function() onBrandonCrusaderStrike(creature, guid) end)
    schedule(guid, "judgement", {8000, 13000},
        function() onBrandonJudgementOfCommand(creature, guid) end)
    schedule(guid, "divine", 500,
        function() onBrandonDivineShield(creature, guid) end)
    if creature:GetEntry() == NPC_CAPTAIN_BRANDON_UNDEAD then
        schedule(guid, "hammer", {25000, 30000},
            function() onBrandonHammerOfBetrayal(creature, guid) end)
    end
end

-- C++ npc_captain_grondel::Reset :1075-1082 + UpdateAI :1099-1111 —
-- Reset-seeded timers re-keyed to event 1:
-- EVENT_GRONDEL_CHARGE_CHECK 500ms -> DoCastVictim(SPELL_CHARGE
-- 71553), re-arm 500ms unconditional (the poll sits outside any
-- guard); EVENT_GRONDEL_MORTAL_STRIKE {8s,14s} ->
-- DoCastVictim(SPELL_MORTAL_STRIKE 71552), re-arm {10s,15s};
-- EVENT_GRONDEL_SUNDER_ARMOR {3s,12s} -> DoCastVictim(
-- SPELL_SUNDER_ARMOR 71554), re-arm {5s,17s}. The undead entry
-- (37494) runs the same struct/AI — same seeds, same arms.
-- PORTED this run: EVENT_GRONDEL_CONFLAGRATION {12s,17s} ->
-- randomPlayerTarget -> 71785, re-arm {10s,15s} (undead-entry
-- gated).
local function onGrondelCharge(creature, guid)
    creature:CastSpell(nil, SPELL_CHARGE)
    schedule(guid, "charge", 500,
        function() onGrondelCharge(creature, guid) end)
end

local function onGrondelMortalStrike(creature, guid)
    creature:CastSpell(nil, SPELL_MORTAL_STRIKE)
    schedule(guid, "mortal", {10000, 15000},
        function() onGrondelMortalStrike(creature, guid) end)
end

local function onGrondelSunderArmor(creature, guid)
    creature:CastSpell(nil, SPELL_SUNDER_ARMOR)
    schedule(guid, "sunder", {5000, 17000},
        function() onGrondelSunderArmor(creature, guid) end)
end

-- C++ EVENT_GRONDEL_CONFLAGRATION :1112-1114 — seeded only when
-- IsUndead (Reset :1080-1081; entry remap to 37494) —
-- SelectTarget(Random, 0, 0.0f, true) -> DoCast(target,
-- SPELL_CONFLAGRATION 71785) non-triggered; re-arm {10s,15s}
-- unconditional.
local function onGrondelConflagration(creature, guid)
    local target = randomPlayerTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_CONFLAGRATION)
    end
    schedule(guid, "conflagration", {10000, 15000},
        function() onGrondelConflagration(creature, guid) end)
end

local function grondelEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "charge", 500,
        function() onGrondelCharge(creature, guid) end)
    schedule(guid, "mortal", {8000, 14000},
        function() onGrondelMortalStrike(creature, guid) end)
    schedule(guid, "sunder", {3000, 12000},
        function() onGrondelSunderArmor(creature, guid) end)
    if creature:GetEntry() == NPC_CAPTAIN_GRONDEL_UNDEAD then
        schedule(guid, "conflagration", {12000, 17000},
            function() onGrondelConflagration(creature, guid) end)
    end
end

local function captainCombatLeave(event, creature)
    cancelTimers(creature:GetGUID())
end

local function captainCombatReset(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ npc_argent_captainAI::KilledUnit :832-835 — victim player ->
-- Talk(SAY_CAPTAIN_KILL 2) — the razuvious player-gated variant
-- precedent; entirely absent from the file until this run.
local function captainKilledUnit(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_CAPTAIN_KILL)
    end
end

-- C++ npc_argent_captainAI::JustDied (:820-829): the _firstDeath latch
-- (true at construction) — first death Talk(SAY_CAPTAIN_DEATH 0) and
-- flips the latch; later deaths Talk(SAY_CAPTAIN_SECOND_DEATH 3). The
-- latch is per-AI-instance and survives in-place resurrection, so it is
-- keyed by GUID and never cleared on events 2/23 (a fresh spawn gets a
-- fresh GUID, matching the ctor). TIMER-LEAK FIX: cancelTimers runs
-- first — brandon/grondel previously registered no event 4, so their
-- GUID-keyed timers kept firing phantom casts on the corpse.
local captainFirstDeath = {}

local function captainDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    local guid = creature:GetGUID()
    if captainFirstDeath[guid] ~= false then
        captainFirstDeath[guid] = false
        creature:Talk(SAY_CAPTAIN_DEATH)
    else
        creature:Talk(SAY_CAPTAIN_SECOND_DEATH)
    end
end

local BRANDON_ENTRIES = {
    NPC_CAPTAIN_BRANDON,
    NPC_CAPTAIN_BRANDON_UNDEAD,
}

for _, entry in ipairs(BRANDON_ENTRIES) do
    RegisterCreatureEvent(entry, 1, brandonEnterCombat)
    RegisterCreatureEvent(entry, 2, captainCombatLeave)
    RegisterCreatureEvent(entry, 3, captainKilledUnit)
    RegisterCreatureEvent(entry, 4, captainDied)
    RegisterCreatureEvent(entry, 23, captainCombatReset)
end

local GRONDEL_ENTRIES = {
    NPC_CAPTAIN_GRONDEL,
    NPC_CAPTAIN_GRONDEL_UNDEAD,
}

for _, entry in ipairs(GRONDEL_ENTRIES) do
    RegisterCreatureEvent(entry, 1, grondelEnterCombat)
    RegisterCreatureEvent(entry, 2, captainCombatLeave)
    RegisterCreatureEvent(entry, 3, captainKilledUnit)
    RegisterCreatureEvent(entry, 4, captainDied)
    RegisterCreatureEvent(entry, 23, captainCombatReset)
end

-- C++ npc_captain_arnath::Reset :947 — Reset-seeded timers re-keyed to
-- event 1: EVENT_ARNATH_SMITE {3s,6s} -> DoCastVictim(SPELL_SMITE)
-- non-triggered, re-arm {4s,7s} (:978-980). SPELL_SMITE is IsUndead ?
-- 71778 : 71546 (:218) — the entry flips exactly with IsUndead
-- (:896-898), so the entry gates the spell pick. Flash heal / PW
-- shield stay out on friendly-search legs (FindFriendlyCreature /
-- DoFindFriendlyMissingBuff, no bridge); dominate mind stays out on
-- the Is25ManRaid gate (no 25-man-raid bridge).
local function onArnathSmite(creature, guid)
    local spell = SPELL_SMITE_NORMAL
    if creature:GetEntry() == NPC_CAPTAIN_ARNATH_UNDEAD then
        spell = SPELL_SMITE_UNDEAD
    end
    creature:CastSpell(nil, spell)
    schedule(guid, "smite", {4000, 7000},
        function() onArnathSmite(creature, guid) end)
end

local function arnathEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "smite", {3000, 6000},
        function() onArnathSmite(creature, guid) end)
end

-- C++ SelectTarget(Random, N) with playerOnly=FALSE (UnitAI.h:170):
-- random over the threat list, pets/totems eligible. Bridged via
-- creature:GetAITargets() (threat-ordered; lua_creature_events.go:345),
-- the boss_midnight randomTargetAnywhere precedent.
local function randomThreatTarget(creature)
    local targets = creature:GetAITargets()
    if #targets == 0 then
        return nil
    end
    return targets[math.random(#targets)]
end

-- C++ SelectTarget(Random, 1) (UnitAI.h:170-174): like randomThreatTarget
-- but the offset-1 skips the first MaxThreat-ordered entry (the tank) —
-- the netherspite/auriaya tank-skip idiom on the bridged threat list.
local function randomNonHeadThreatTarget(creature)
    local targets = creature:GetAITargets()
    if #targets <= 1 then
        return nil
    end
    return targets[math.random(2, #targets)]
end

-- C++ npc_captain_rupert::Reset :1138-1140 — Reset-seeded timers
-- re-keyed to event 1:
-- EVENT_RUPERT_FEL_IRON_BOMB {15s,20s} -> SelectTarget(Random, 0) ->
--   DoCast(target, SPELL_FEL_IRON_BOMB) non-triggered, re-arm
--   {15s,20s} (:1157-1159);
-- EVENT_RUPERT_MACHINE_GUN {25s,30s} -> SelectTarget(Random, 1) ->
--   DoCast(target, SPELL_MACHINE_GUN) non-triggered, re-arm
--   {25s,30s} (:1162-1164);
-- EVENT_RUPERT_ROCKET_LAUNCH {10s,15s} -> SelectTarget(Random, 1) ->
--   DoCast(target, SPELL_ROCKET_LAUNCH) non-triggered, re-arm
--   {10s,15s} (:1167-1169).
-- Undead spell picks via the entry (IsUndead flip, :896-898). All three
-- arms are SelectTarget(Random, N) with playerOnly=FALSE (UnitAI.h:170),
-- so the pools are the threat list via GetAITargets
-- (lua_creature_events.go:345 — the midnight precedent); machine gun /
-- rocket launch skip the MaxThreat head (Random, 1, UnitAI.h:173-174 —
-- the netherspite/auriaya tank-skip idiom on the bridged threat list).
local function onRupertFelIronBomb(creature, guid)
    local spell = SPELL_FEL_IRON_BOMB_NORMAL
    if creature:GetEntry() == NPC_CAPTAIN_RUPERT_UNDEAD then
        spell = SPELL_FEL_IRON_BOMB_UNDEAD
    end
    local target = randomThreatTarget(creature)
    if target then
        creature:CastSpell(target, spell)
    end
    schedule(guid, "bomb", {15000, 20000},
        function() onRupertFelIronBomb(creature, guid) end)
end

local function onRupertMachineGun(creature, guid)
    local spell = SPELL_MACHINE_GUN_NORMAL
    if creature:GetEntry() == NPC_CAPTAIN_RUPERT_UNDEAD then
        spell = SPELL_MACHINE_GUN_UNDEAD
    end
    local target = randomNonHeadThreatTarget(creature)
    if target then
        creature:CastSpell(target, spell)
    end
    schedule(guid, "gun", {25000, 30000},
        function() onRupertMachineGun(creature, guid) end)
end

local function onRupertRocketLaunch(creature, guid)
    local spell = SPELL_ROCKET_LAUNCH_NORMAL
    if creature:GetEntry() == NPC_CAPTAIN_RUPERT_UNDEAD then
        spell = SPELL_ROCKET_LAUNCH_UNDEAD
    end
    local target = randomNonHeadThreatTarget(creature)
    if target then
        creature:CastSpell(target, spell)
    end
    schedule(guid, "rocket", {10000, 15000},
        function() onRupertRocketLaunch(creature, guid) end)
end

local function rupertEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "bomb", {15000, 20000},
        function() onRupertFelIronBomb(creature, guid) end)
    schedule(guid, "gun", {25000, 30000},
        function() onRupertMachineGun(creature, guid) end)
    schedule(guid, "rocket", {10000, 15000},
        function() onRupertRocketLaunch(creature, guid) end)
end

local ARNATH_ENTRIES = {
    NPC_CAPTAIN_ARNATH,
    NPC_CAPTAIN_ARNATH_UNDEAD,
}

for _, entry in ipairs(ARNATH_ENTRIES) do
    RegisterCreatureEvent(entry, 1, arnathEnterCombat)
    RegisterCreatureEvent(entry, 2, captainCombatLeave)
    RegisterCreatureEvent(entry, 3, captainKilledUnit)
    RegisterCreatureEvent(entry, 4, captainDied)
    RegisterCreatureEvent(entry, 23, captainCombatReset)
end

local RUPERT_ENTRIES = {
    NPC_CAPTAIN_RUPERT,
    NPC_CAPTAIN_RUPERT_UNDEAD,
}

for _, entry in ipairs(RUPERT_ENTRIES) do
    RegisterCreatureEvent(entry, 1, rupertEnterCombat)
    RegisterCreatureEvent(entry, 2, captainCombatLeave)
    RegisterCreatureEvent(entry, 3, captainKilledUnit)
    RegisterCreatureEvent(entry, 4, captainDied)
    RegisterCreatureEvent(entry, 23, captainCombatReset)
end

