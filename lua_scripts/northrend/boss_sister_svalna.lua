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
-- unit and live in THIS file (npc_argent_captains.lua was removed
-- 2026-10-25: it registered the same 8 captain entries, and hooks
-- append, so every captain timer/Talk fired twice — the tree-wide
-- duplicate-registration census found this as the only collision).
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
-- gates PORTED 2026-11-04 (500ms IsCasting defer, mal_ganis idiom):
-- svalna :485/:515, crok UpdateEscortAI :745 pre-loop (no in-loop
-- return — the loop ends into DoMeleeAttackIfReady), captains
-- arnath :959/:991, brandon :1032/:1061, grondel :1092/:1120,
-- rupert :1149/:1175, ymirjar combat :1304/:1372 (OOC :1281/:1296
-- covers only the unported spirit-stream event). Phase-machine gating is still the real
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
--   SetHover legs port with it (bridges live). Ported as event-24
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
--   undead spell picks; arnath flash-heal stays out on its
--   FindFriendlyCreature leg; arnath PW-shield PORTED this run
--   (DoFindFriendlyMissingBuff pool via entry sweep, empty-guard
--   added over C++'s unguarded pick); arnath dominate-mind PORTED
--   this run
--   (Is25ManRaid + undead-entry gates, skip-first player pool),
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
-- SetHomePosition :861 (ported, in all four captain enterCombat
-- handlers below) + the IsUndead-gated DoZoneInCombat
-- :862-863 (stays out: dead latch, set-arm rides the unported
-- setDeathState/UpdateEntry machine) — lives in this file's
-- captain handlers.)
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
--   EVENT_SVALNA_RESURRECT (:492-495) — PORTED 2026-11-15 as the
--   DoAction-seeded timers (svalnaDoAction, ev38): Talk(
--   SAY_SVALNA_EVENT_START 0) / Talk(SAY_SVALNA_RESURRECT_CAPTAINS
--   2) + DoCast(me, SPELL_REVIVE_CHAMPION 70053). The old
--   "rides the gauntlet/action machine — gothik precedent" note is
--   retracted: the machine's receivers are ported, only the
--   waypoint-machine senders stay out.
-- boss_sister_svalna JustEngagedWith EVENT_IMPALING_SPEAR
--   (:359, 40-50s, re-arm 20-25s, :500-509) — PORTED: self-cast
--   AETHER_SHIELD 71463 then victim-cast IMPALING_SPEAR 71443 on a
--   threat-list target (victim-first pop, playerOnly,
--   -71443 aura exclusion via randomNonHeadPlayerTarget).
-- boss_sister_svalna JustEngagedWith EVENT_AETHER_SHIELD
--   (:360, 100-110s) — scheduled but has NO case in the UpdateAI
--   switch: dead event, nothing to port (upstream finding).
-- boss_sister_svalna DoAction (:397-428) — PORTED 2026-11-15
--   (svalnaDoAction, ev38): KILL_CAPTAIN (triggered self-cast
--   70078; max-targets-1 mod has no bridge), START_GAUNTLET
--   (SetFarVisible + SetImmuneToAll + 25s EVENT_SVALNA_START seed;
--   setActive + _isEventInProgress latch drop — no bridge / no Lua
--   twin, no behavioral delta), RESURRECT_CAPTAINS (7s seed),
--   CAPTAIN_DIES (Talk 5; zero callers tree-wide, ported dead),
--   RESET_EVENT (SetFarVisible(false) + svalnaReset twin).
--   START_GAUNTLET fires live (AT sender + crok forward);
--   KILL_CAPTAIN / RESURRECT_CAPTAINS / RESET_EVENT receivers are
--   dormant until the crok waypoint machine / wipe leg land.
-- boss_sister_svalna SpellHit (:430-437) — PORTED this run:
--   Talk(EMOTE_SVALNA_BROKEN_SHIELD 8, caster) + RemoveAura(
--   SPELL_AETHER_SHIELD 71463) on SPELL_HURL_SPEAR 71466 via
--   event 14 (the stale "event 15 never fires / no aura bridge"
--   claim is retracted — event 14, HasAura, RemoveAura and
--   Talk(id, target) are all bridged).
-- boss_sister_svalna SpellHitTarget (:452-476) — 71443 arm PORTED
--   2026-11-13 (ev15, svalnaSpellHitTarget below): SummonCreature(
--   NPC_IMPALING_SPEAR 38248) at the target + Talk(EMOTE_SVALNA_IMPALE
--   7, unitTarget) + summon CastCustomSpell(VEHICLE_SPELL_RIDE_HARDCODED
--   46598, bp0=1) + SetFlag(UNIT_FIELD_FLAGS_2, 0x0002|0x4000); the
--   2026-10-20 "no bridge" rationale is retracted — SummonCreature,
--   CastCustomSpell(bp0), SetFlag(field 60) and Talk(id, target) are
--   all bridged. The SPELL_IMPALING_SPEAR_KILL 70196 Unit::Kill leg
--   stays out: its only caster is the spell_svalna_caress_of_death
--   SpellScript trigger (:1505), no SpellScript bridge, so 70196 is
--   never cast in Go.
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
-- npc_crok_scourgebane DoAction (:553-589) — PORTED 2026-11-15
--   (crokDoAction, ev38): ACTION_START_GAUNTLET latches +
--   Svalna forward + intro Talks (0 / Arnath 5 @7s / 1 @14s) +
--   SetFarVisible + 4-captain fan-out; LoadGrid / setActive /
--   EVENT_START_PATHING (escort Start, no bridge) stay out.
--   ACTION_RESET_EVENT leg ported (latches + SetFarVisible(false)),
--   dormant until the wipe leg lands. The old "machine-gated
--   escort legs, no escort / DoAction bridge (joins the escort
--   queue)" note is retracted for the DoAction receivers — only
--   the escort/waypoint machine itself stays out.
-- npc_argent_captainAI DoAction (:838-856) — PORTED 2026-11-15
--   (captainDoAction, ev38, all 8 entries): START_GAUNTLET
--   (REACT_DEFENSIVE + MoveFollow(crok, dist, angle) +
--   SetFarVisible; setActive drops, no bridge) and RESET_EVENT
--   (_firstDeath latch clear).
-- npc_crok_scourgebane DamageTaken (:691-731) — SPLIT 2026-11-13:
--   the 10%-health latch is PORTED (crokDamageTaken, ev9): damage = 0
--   + Talk(SAY_CROK_WEAKENING_*) + DoCast(70714) + 1s
--   EVENT_HEALTH_CHECK loop (HealthAbovePct(15) -> remove aura +
--   clear latch; else SetHealth(+CountPctFromMaxHealth(5)) + re-arm).
--   The ModifyHealth leg rides a SetHealth emulation — exact, not a
--   fabrication: C++ ModifyHealth clamps at max (Unit.cpp:8519) and
--   Go SetHealth clamps at MaxHealth (lua_creature_events.go:2863),
--   and CountPctFromMaxHealth is bridged. The _isEventActive Talk
--   gate rides a boss-state proxy (NOT_STARTED -> GAUNTLET talk,
--   else SVALNA talk; faithful in every reachable context —
--   gauntlet vs Svalna fight vs post-fight; pre-gauntlet crok takes
--   no damage, so the proxy's one blind spot is unreachable).
--   The wipe-check leg STAYS OUT: damage *= 100 + lethal ->
--   FrostwingGauntletRespawner (:258-300) + Talk(SAY_CROK_DEATH 8)
--   needs SetCorpseDelay (no bridge), UpdatePosition(DB spawnPoint)
--   (no bridge — GetHomePosition is NOT the spawn point: home is
--   re-stamped on engage for escorted creatures,
--   creaturemotion.go:1483 atEngageSetHomePosition, the C++
--   Creature::AtEngage twin) and the FrostwingGauntletRespawner
--   DoAction fan-out itself (the Svalna/crok/captain RESET_EVENT
--   receivers ARE ported via ev38 since 2026-11-15 — only the
--   respawner sender stays out). DoAction ev38 dispatch,
--   GetCreaturesInRange(333), SetRespawnDelay and
--   DespawnOrUnsummon are all bridged — the blockers are the three
--   legs above, not the enumeration. Known cost of the stayed-out
--   wipe leg: on a true wipe (no player within 60 yd) C++ x100-kills
--   crok and resets the gauntlet while Lua holds him at <10% under
--   the armor loop; event 9 DOES support numeric damage rewrite
--   (lua_creature_events.go:303-324), so the old "event-9 cannot
--   express damage rewrite" claim was stale.
-- npc_crok_scourgebane SetGUID / WaypointReached / WaypointStarted
--   (:591-689) — gauntlet waypoint machine, no bridge.
-- npc_frostwing_ymirjar_vrykul UpdateAI random-target arms —
--   EVENT_YMIRJAR_TWISTED_WINDS (71306), EVENT_YMIRJAR_ICE_TRAP
--   (71249), EVENT_YMIRJAR_VOLLEY (71252), EVENT_YMIRJAR_BANISH
--   (71298) PORTED via the threat-list randomPlayerTarget pool
--   (offset-1 arms ride skipFirst); EVENT_YMIRJAR_SUMMON (71303) stays
--   parked (summon machine + _events.DelayEvents(2s) leg).
-- npc_frostwing_ymirjar_vrykul Reset (:1189-1213) — the
--   RemoveAurasDueToSpell(SPELL_ARCTIC_CHILL) leg is PORTED (stale "no aura
--   bridge on this path" claim retracted — creature:RemoveAura bridged);
--   the OOC EVENT_YMIRJAR_SPIRIT_STREAM
--   (69929, {10000,20000}ms, re-arm {40000,60000}ms) fires while
--   !UpdateVictim() — no OOC-tick bridge; the huntress
--   Is25ManRaid() DoCast(SPELL_SUMMON_WARHAWK 71705) leg is a
--   summon machine (difficulty gate now bridged, lua_creature_events.go).
-- npc_impaling_spear (:1385-1413) — the whole AI is a 500ms
--   vehicle-check despawn timer (no bridgeable arm; NOT
--   registered).
-- The 3 spell scripts join the no-SpellScript-bridge queue (the
--   boss_moragg optic-link precedent);
--   at_icc_start_frostwing_gauntlet PORTED 2026-11-15 (at_*
--   census; at_icc_start_frostwing_gauntlet.lua) — the old
--   "joins the no-AreaTrigger bridge queue" note is stale. Its
--   receiver (crok ev38 DoAction) landed this run, so the
--   trigger -> crok -> Svalna -> captains chain is live through
--   the intro Talks; only the escort Start/waypoints stay out.
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

local DATA_SISTER_SVALNA = 9 -- icecrown_citadel.h:83
local DATA_CROK_SCOURGEBANE = 30 -- icecrown_citadel.h:106
local DATA_CAPTAIN_ARNATH = 31 -- icecrown_citadel.h:107 (brandon 32, grondel 33, rupert 34)
local ACTION_KILL_CAPTAIN = 1 -- boss_sister_svalna.cpp:197
local ACTION_START_GAUNTLET = 2 -- :198
local ACTION_RESURRECT_CAPTAINS = 3 -- :199
local ACTION_CAPTAIN_DIES = 4 -- :200
local ACTION_RESET_EVENT = 5 -- :201
local BOSS_STATE_NOT_STARTED = 0 -- InstanceScript.h:68-76
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_DONE = 3

local ENTRY_YMIRJAR_BATTLE_MAIDEN = 37132
local ENTRY_YMIRJAR_DEATHBRINGER = 38125
local ENTRY_YMIRJAR_WARHAWK = 38154
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

local SAY_SVALNA_EVENT_START = 0 -- boss_sister_svalna.cpp:53
local SAY_SVALNA_KILL_CAPTAIN = 1
local SAY_SVALNA_RESURRECT_CAPTAINS = 2 -- :57
local SAY_SVALNA_AGGRO = 3
local SAY_SVALNA_KILL = 4
local SAY_SVALNA_CAPTAIN_DEATH = 5 -- :45
local SAY_SVALNA_DEATH = 6
local SAY_CROK_INTRO_1 = 0 -- :50
local SAY_CROK_INTRO_3 = 1 -- :52
local SAY_CROK_COMBAT_SVALNA = 5 -- boss_sister_svalna.cpp:58
local SAY_ARNATH_INTRO_2 = 5 -- :51
local SAY_CROK_WEAKENING_GAUNTLET = 6 -- :65
local SAY_CROK_WEAKENING_SVALNA = 7 -- :66

local SAY_CAPTAIN_DEATH = 0
local SAY_CAPTAIN_SECOND_DEATH = 3
local SAY_CAPTAIN_KILL = 2 -- :62

local SPELL_DIVINE_SURGE = 71465

local SPELL_CARESS_OF_DEATH = 70078 -- boss_sister_svalna.cpp:78
local SPELL_REVIVE_CHAMPION = 70053 -- :80

local SPELL_ICEBOUND_ARMOR = 70714 -- boss_sister_svalna.cpp:73

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
local SPELL_DOMINATE_MIND = 14515 -- :88
local SPELL_POWER_WORD_SHIELD_NORMAL = 71548 -- :90
local SPELL_POWER_WORD_SHIELD_UNDEAD = 71780 -- :93
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
local NPC_IMPALING_SPEAR = 38248 -- icecrown_citadel.h:284
local EMOTE_SVALNA_IMPALE = 7 -- :47
local VEHICLE_SPELL_RIDE_HARDCODED = 46598 -- VehicleDefines.h:52
local UNIT_FIELD_FLAGS_2 = 60 -- UpdateFields.h:118
local UNIT_FLAG2_UNK1 = 0x2 -- UnitDefines.h:162
local UNIT_FLAG2_ALLOW_ENEMY_INTERACT = 0x4000 -- UnitDefines.h:174
local SPELL_AETHER_SHIELD = 71463
local SPELL_HURL_SPEAR = 71466 -- :84
local EMOTE_SVALNA_BROKEN_SHIELD = 8 -- :48

local timers = {}

-- GUID-keyed _didUnderTenPercentText twin (boss_sister_svalna.cpp:808):
-- set on the first Talk of a dip, cleared by the health-check loop when
-- health recovers above 15% (:774-775) and by Reset via Initialize()
-- (:540). NOT cleared on death/leave — C++ keeps the member until Reset.
local crokWeakeningSaid = {}

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

-- C++ SelectTarget(Random, offset, dist, true) over the threat list
-- (UnitAI.h offset-before-filter): skipFirst drops the current victim
-- BEFORE the playerOnly/alive/distance filters; maxDist is the C++ dist
-- gate (IsWithinDist approximation). The old map-player pool is retired;
-- the stale "offset-1 has no bridge" claim is retracted.
local function randomPlayerTarget(creature, maxDist, skipFirst)
    local targets = creature:GetAITargets()
    local victimGuid = nil
    if skipFirst then
        local victim = creature:GetVictim()
        if victim ~= nil then
            victimGuid = victim:GetGUID()
        end
    end
    local candidates = {}
    local aliveSeen = 0
    for _, t in ipairs(targets) do
        local skip = false
        if skipFirst then
            if victimGuid ~= nil then
                skip = t:GetGUID() == victimGuid
            elseif not t:IsDead() then
                aliveSeen = aliveSeen + 1
                skip = aliveSeen == 1
            end
        end
        if not skip
                and t:GetObjectType() == "Player" and not t:IsDead()
                and (not maxDist or creature:GetDistance(t) - creature:GetCombatReach() - 1.5 < maxDist) then -- IsWithinCombatRange twin (Unit.cpp:583; player reach 1.5, ObjectDefines.h:40)
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ SelectTarget(Random, 1, 0.0f, playerOnly=TRUE) offset-1
-- tank-skip twin: C++ SelectTargetList builds victim-first
-- (UnitAI.h), so the offset-1 pop removes the current victim, not
-- the threat-sorted head; GetVictim exclusion models that (index
-- drop diverges when fixate/taunt puts the victim below top
-- threat, threat.go reselectVictimLocked). The playerOnly gate
-- applies AFTER the skip, matching DefaultTargetSelector order
-- (UnitAI.cpp:254-289);
-- excludeAura (nil or a spell id) drops carriers (the -aura arg);
-- dead entries are offline refs, skipped (SelectTargetList skip).
local function randomNonHeadPlayerTarget(creature, excludeAura)
    local targets = creature:GetAITargets()
    local victim = creature:GetVictim()
    local victimGuid = victim and victim:GetGUID() or nil
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
                and t:GetObjectType() == "Player" and not t:IsDead()
                and (excludeAura == nil or not t:HasAura(excludeAura)) then
            candidates[#candidates + 1] = t
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
-- (re-scheduled outside the target guard). The offset-1 tank-skip +
-- playerOnly + -71443 aura-exclusion legs ride
-- randomNonHeadPlayerTarget (threat-list pool, skip-first, aura
-- filter); the old "documented unbridged caveats" claim is retracted.
-- C++ :499-502: SetReactState(REACT_DEFENSIVE 1) then Talk(
-- SAY_SVALNA_AGGRO 3) — arm order kept.
local function onSvalnaCombat(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :485 (+ in-loop :515, single-tick
-- no-op per the marrowgar precedent): 500ms IsCasting defer —
-- one-shot, so the defer re-schedules a fresh 500ms entry rather
-- than dropping the combat Talk/react arm.
    if creature:IsCasting() then
        schedule(guid, "combat", 500, function() onSvalnaCombat(creature, guid) end)
        return
    end
    creature:SetReactState(1)
    creature:Talk(SAY_SVALNA_AGGRO)
end

local function onSvalnaImpalingSpear(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :485 (+ in-loop :515, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "impaling", 500, function() onSvalnaImpalingSpear(creature, guid) end)
        return
    end
    local target = randomNonHeadPlayerTarget(creature, SPELL_IMPALING_SPEAR)
    if target then
        creature:CastSpell(creature, SPELL_AETHER_SHIELD)
        creature:CastSpell(target, SPELL_IMPALING_SPEAR)
    end
    schedule(guid, "impaling", {20000, 25000},
        function() onSvalnaImpalingSpear(creature, guid) end)
end

local function svalnaEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    -- C++ JustEngagedWith base call first (boss_sister_svalna.cpp:355):
    -- BossAI::JustEngagedWith arms (ScriptedCreature.cpp:527-544) —
    -- SetBossState(IN_PROGRESS) (:537, after the CheckRequiredBosses
    -- gate — no bridge, stays out) + SetCombatPulseDelay(5) + me-form
    -- DoZoneInCombat, before the crok Talk leg (C++ arm order);
    -- instance/setActive/ScheduleTasks legs out. The IN_PROGRESS leg
    -- is PORTED 2026-11-06 via creature:SetBossState.
    creature:SetBossState(DATA_SISTER_SVALNA, BOSS_STATE_IN_PROGRESS)
    creature:SetCombatPulseDelay(5)
    creature:DoZoneInCombat()
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
    schedule(guid, "combat", 9000, function() onSvalnaCombat(creature, guid) end)
    schedule(guid, "impaling", {40000, 50000},
        function() onSvalnaImpalingSpear(creature, guid) end)
end

local function svalnaLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
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
    cancelTimers(creature:GetGUIDLow())
    -- C++ JustDied :336 — _JustDied() sets DONE, first arm (before
    -- Talk). PORTED 2026-11-06 via creature:SetBossState.
    creature:SetBossState(DATA_SISTER_SVALNA, BOSS_STATE_DONE)
    creature:Talk(SAY_SVALNA_DEATH)
end

-- C++ boss_sister_svalna::Reset :326-331 — _Reset() then
-- SetReactState(REACT_DEFENSIVE 1); the _isEventInProgress latch has
-- no bridge. Ported in C++ arm order (the venoxis ev23 precedent).
-- The InitializeAI (:320-324) spawn-time REACT_PASSIVE stays out:
-- the intro DoAction machine is unported and porting PASSIVE alone
-- would strand her passive (the tyrannus precedent).
local function svalnaReset(event, creature)
    -- C++ Reset :329 — _Reset() first: SetBossState NOT_STARTED only
    -- when the stored state != DONE (BossAI::_Reset,
    -- ScriptedCreature.cpp:499-509; the falric precedent). PORTED
    -- 2026-11-06 via creature:SetBossState.
    if creature:GetBossState(DATA_SISTER_SVALNA) ~= BOSS_STATE_DONE then
        creature:SetBossState(DATA_SISTER_SVALNA, BOSS_STATE_NOT_STARTED)
    end
    cancelTimers(creature:GetGUIDLow())
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

-- C++ npc_frostwing_ymirjar_vrykul::JustSummoned (boss_sister_svalna.cpp:1221-1230):
-- warhawk summons go REACT_DEFENSIVE (1); _summons.Summon is
-- bookkeeping only; engaged summons get DoZoneInCombat. RE-HOMED
-- 2026-11-13: the handler was registered on Svalna (37126), which
-- summons nothing — the arm belongs to the ymirjar vrykul (only the
-- huntress 37134 summons, via Reset DoCast 71705, 25-man only; that
-- summon leg is still unported, so this handler is dormant until it
-- lands). IsInCombat is the IsEngaged twin.
local function ymirjarJustSummoned(_, creature, summoned)
    if summoned:GetEntry() == ENTRY_YMIRJAR_WARHAWK then
        summoned:SetReactState(1) -- REACT_DEFENSIVE
    end
    if creature:IsInCombat() then
        summoned:DoZoneInCombat()
    end
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

-- C++ boss_sister_svalna::SpellHitTarget (:452-476), SPELL_IMPALING_SPEAR
-- 71443 arm — PORTED 2026-11-13 (ev15): the 71443 cast is live in Go
-- (onSvalnaImpalingSpear scheduler, 20-25s), so ev15 fires on Svalna
-- when it hits the target. SummonCreature at the target's position
-- (Position overload, *unitTarget); default summon type is the C++
-- 2-arg default (MANUAL_DESPAWN, Object.h:412 — Go SummonCreature
-- default 7 = MANUAL). The impaling-spear despawn rides the unported
-- npc_impaling_spear vehicle-check AI, documented. The 70196 KILL leg
-- stays out (SpellScript trigger only, no bridge).
local function svalnaSpellHitTarget(event, creature, target, spellId)
    if spellId ~= SPELL_IMPALING_SPEAR then
        return
    end
    if target == nil then
        return
    end
    local summon = target:SummonCreature(NPC_IMPALING_SPEAR, target:GetX(), target:GetY(), target:GetZ(), target:GetO())
    if summon then
        creature:Talk(EMOTE_SVALNA_IMPALE, target)
        summon:CastCustomSpell(target, VEHICLE_SPELL_RIDE_HARDCODED, false, 1)
        -- C++ UNIT_FLAG2_UNK1 | UNIT_FLAG2_ALLOW_ENEMY_INTERACT: disjoint
        -- bits, so + is the Lua 5.1 OR twin.
        summon:SetFlag(UNIT_FIELD_FLAGS_2, UNIT_FLAG2_UNK1 + UNIT_FLAG2_ALLOW_ENEMY_INTERACT)
    end
end

RegisterCreatureEvent(ENTRY_SISTER_SVALNA, 15, svalnaSpellHitTarget)

-- C++ boss_sister_svalna::DoAction :397-428 + UpdateAI :489-497 —
-- PORTED 2026-11-15 (crok gauntlet receivers, ev38 dispatch bridged,
-- lua_creature_events.go:929; creature:DoAction live, razorgore
-- precedent). ACTION_KILL_CAPTAIN :401-402 — DoCastSelf(
-- SPELL_CARESS_OF_DEATH 70078, TRIGGERED_FULL_MASK) with
-- SPELLVALUE_MAX_TARGETS 1: the triggered self-cast ports; the
-- max-targets mod has NO bridge (no CastSpellExtraArgs surface —
-- only the DBC MaxTargets field, store.go:245), so a multi-target
-- 70078 is the known deviation (the SpellScript trigger that
-- consumes the hit is unported anyway). ACTION_START_GAUNTLET
-- :404-412 — IsAlive gate, SetFarVisible(true) + SetImmuneToAll(
-- true) (both bridged), EVENT_SVALNA_START 25s -> Talk(
-- SAY_SVALNA_EVENT_START 0) (:489-491); setActive(true) has NO
-- bridge (no SetActive surface in engine/) and the
-- _isEventInProgress latch has no Lua twin — it only gates the C++
-- UpdateAI early-return, which has no Lua counterpart (timers fire
-- regardless), so both legs drop with no behavioral delta.
-- ACTION_RESURRECT_CAPTAINS :414-415 — EVENT_SVALNA_RESURRECT 7s ->
-- Talk(SAY_SVALNA_RESURRECT_CAPTAINS 2) + DoCast(me,
-- SPELL_REVIVE_CHAMPION 70053) non-triggered (:492-495).
-- ACTION_CAPTAIN_DIES :417-418 — Talk(SAY_SVALNA_CAPTAIN_DEATH 5);
-- whole-tree grep finds ZERO callers (enum :200 + case :417 only),
-- so the case is ported dead, matching C++. ACTION_RESET_EVENT
-- :420-424 — SetFarVisible(false) + Reset() (the svalnaReset twin:
-- boss-state NOT_STARTED unless DONE + cancelTimers +
-- REACT_DEFENSIVE); setActive(false) drops on the same no-bridge
-- leg. Senders: START_GAUNTLET now fires (AT sender ported
-- 2026-11-15, crok ev38 forwards); KILL_CAPTAIN /
-- RESURRECT_CAPTAINS ride the unported crok waypoint machine
-- (:606,635,661,684); RESET_EVENT rides the unported wipe leg
-- (FrostwingGauntletRespawner :276-279) — receivers ported, dormant
-- until those machines land.
local function onSvalnaStart(creature, guid)
    -- C++ UpdateAI pre-loop gate :485 (single-tick no-op per the
    -- marrowgar precedent): 500ms IsCasting defer, no re-arm
    -- consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "svalna_start", 500, function() onSvalnaStart(creature, guid) end)
        return
    end
    creature:Talk(SAY_SVALNA_EVENT_START)
end

local function onSvalnaResurrect(creature, guid)
    -- C++ UpdateAI pre-loop gate :485 (single-tick no-op per the
    -- marrowgar precedent): 500ms IsCasting defer, no re-arm
    -- consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "svalna_resurrect", 500, function() onSvalnaResurrect(creature, guid) end)
        return
    end
    creature:Talk(SAY_SVALNA_RESURRECT_CAPTAINS)
    creature:CastSpell(creature, SPELL_REVIVE_CHAMPION)
end

local function svalnaDoAction(event, creature, action)
    local guid = creature:GetGUIDLow()
    if action == ACTION_KILL_CAPTAIN then
        creature:CastSpell(creature, SPELL_CARESS_OF_DEATH, true)
    elseif action == ACTION_START_GAUNTLET then
        if creature:IsAlive() then
            creature:SetFarVisible(true)
            creature:SetImmuneToAll(true)
            schedule(guid, "svalna_start", 25000,
                function() onSvalnaStart(creature, guid) end)
        end
    elseif action == ACTION_RESURRECT_CAPTAINS then
        schedule(guid, "svalna_resurrect", 7000,
            function() onSvalnaResurrect(creature, guid) end)
    elseif action == ACTION_CAPTAIN_DIES then
        creature:Talk(SAY_SVALNA_CAPTAIN_DEATH)
    elseif action == ACTION_RESET_EVENT then
        creature:SetFarVisible(false)
        svalnaReset(event, creature)
    end
end

RegisterCreatureEvent(ENTRY_SISTER_SVALNA, 38, svalnaDoAction)

-- C++ npc_crok_scourgebane::Reset :544-551 seeds
-- EVENT_SCOURGE_STRIKE {7500,12500}ms and EVENT_DEATH_STRIKE
-- {25000,30000}ms; UpdateEscortAI :754-761: scourge strike ->
-- DoCastVictim(71488), re-arm {10000,14000}ms; death strike ->
-- gated on HealthBelowPct(20) -> DoCastVictim(71489), re-arm
-- {5000,10000}ms unconditional. Re-keyed to event 1 (the Reset
-- re-key convention).
local function onCrokScourgeStrike(creature, guid)
    -- C++ UpdateEscortAI pre-loop gate :745 (no in-loop return — the
-- loop ends into DoMeleeAttackIfReady): 500ms IsCasting defer, no
-- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "scourge", 500, function() onCrokScourgeStrike(creature, guid) end)
        return
    end
    creature:CastSpell(nil, SPELL_SCOURGE_STRIKE)
    schedule(guid, "scourge", {10000, 14000},
        function() onCrokScourgeStrike(creature, guid) end)
end

local function onCrokDeathStrike(creature, guid)
    -- C++ UpdateEscortAI pre-loop gate :745 (no in-loop return — the
-- loop ends into DoMeleeAttackIfReady): 500ms IsCasting defer, no
-- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "death", 500, function() onCrokDeathStrike(creature, guid) end)
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth > 0
            and creature:GetHealth() < math.floor(maxHealth * 20 / 100) then
        creature:CastSpell(nil, SPELL_DEATH_STRIKE)
    end
    schedule(guid, "death", {5000, 10000},
        function() onCrokDeathStrike(creature, guid) end)
end

-- C++ npc_crok_scourgebane::DamageTaken (:691-731), 10%-health latch
-- only (the wipe-check leg stays out — see the header note): while
-- health is below 10%, every hit is zeroed, ICEBOUND_ARMOR (70714) is
-- re-cast, and a 1s EVENT_HEALTH_CHECK loop heals 5% of max per tick
-- until health is above 15%, when the aura is removed and the Talk
-- latch clears (so the Talk can re-fire on a later dip). The Talk
-- fires once per dip: GAUNTLET (6) while the gauntlet is in progress,
-- SVALNA (7) once Svalna is engaged or done — the _isEventActive gate
-- rides the boss-state proxy (NOT_STARTED -> GAUNTLET, else SVALNA;
-- faithful in every reachable context, see header). DamageTaken is
-- pre-damage (ev9), so no postDamage guard. The schedule-by-key loop
-- replaces rather than stacks: under sustained fire C++ queues a
-- health-check per hit (EventMap allows duplicates), so C++ heals
-- faster than this single 5%/s loop — known minor divergence, the
-- in-tree single-loop idiom.
local function onCrokHealthCheck(creature, guid)
    if creature:IsDead() then
        return
    end
    if creature:HealthAbovePct(15) then
        creature:RemoveAura(SPELL_ICEBOUND_ARMOR)
        crokWeakeningSaid[guid] = false
    else
        -- C++ me->ModifyHealth(me->CountPctFromMaxHealth(5)) (:779):
        -- the SetHealth emulation is exact, not a fabrication —
        -- ModifyHealth clamps at max (Unit.cpp:8519) and Go SetHealth
        -- clamps at MaxHealth (lua_creature_events.go:2863).
        creature:SetHealth(creature:GetHealth() + creature:CountPctFromMaxHealth(5))
        schedule(guid, "healthcheck", 1000,
            function() onCrokHealthCheck(creature, guid) end)
    end
end

local function crokDamageTaken(event, creature, attacker, damage)
    if not creature:HealthBelowPct(10) then
        return
    end
    local guid = creature:GetGUIDLow()
    if not crokWeakeningSaid[guid] then
        crokWeakeningSaid[guid] = true
        if creature:GetBossState(DATA_SISTER_SVALNA) == BOSS_STATE_NOT_STARTED then
            creature:Talk(SAY_CROK_WEAKENING_GAUNTLET)
        else
            creature:Talk(SAY_CROK_WEAKENING_SVALNA)
        end
    end
    creature:CastSpell(creature, SPELL_ICEBOUND_ARMOR)
    schedule(guid, "healthcheck", 1000,
        function() onCrokHealthCheck(creature, guid) end)
    return false, 0
end

-- crok has NO JustEngagedWith in C++ (npc_crok_scourgebane :526-809,
-- EscortAI, no override); the scourge/death-strike seeds below re-key
-- the DoAction gauntlet arms (:547-548) to combat start (svalna precedent).
local function crokEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "scourge", {7500, 12500},
        function() onCrokScourgeStrike(creature, guid) end)
    schedule(guid, "death", {25000, 30000},
        function() onCrokDeathStrike(creature, guid) end)
end

local function crokLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

local function crokDied(event, creature, killer)
    cancelTimers(creature:GetGUIDLow())
end

local function crokReset(event, creature)
    -- C++ Reset -> Initialize() (:540) clears _didUnderTenPercentText.
    crokWeakeningSaid[creature:GetGUIDLow()] = false
    cancelTimers(creature:GetGUIDLow())
end

-- C++ npc_crok_scourgebane::DoAction :553-589 — PORTED 2026-11-15
-- (crok gauntlet receivers, ev38). ACTION_START_GAUNTLET :555-579 —
-- _isEventDone / !IsAlive guard (the ctor reads _isEventDone from
-- the Svalna boss state == DONE, :536; the Lua tables lazy-init
-- from the same boss state on first read — the only reader before
-- any setter is this guard, so lazy init is exact); latches set,
-- then the Svalna DoAction forward via GetInstanceCreature(
-- DATA_SISTER_SVALNA 9) (:566-567), Talk(SAY_CROK_INTRO_1 0),
-- EVENT_ARNATH_INTRO_2 7s (cross-AI Talk on Arnath,
-- SAY_ARNATH_INTRO_2 5, :752-755), EVENT_CROK_INTRO_3 14s (Talk
-- SAY_CROK_INTRO_3 1), SetFarVisible(true), and the 4-captain
-- DoAction(ACTION_START_GAUNTLET) fan-out via GetInstanceCreature(
-- DATA_CAPTAIN_ARNATH + itr, 31-34) (:577-578). LoadGrid(4356.71,
-- 2484.33) (:564) has NO bridge (no LoadGrid surface in engine/);
-- EVENT_START_PATHING 35s -> Start(true, true) (:572, :760-762)
-- has NO escort bridge — the timer is NOT scheduled (a no-op timer
-- would be a fabrication); setActive(true) has NO bridge. The
-- _isEventActive latch now has a real Lua table, but crokDamageTaken
-- keeps its boss-state proxy: the latch's only clearer besides
-- ACTION_RESET_EVENT is the unported waypoint machine (wp4 :630-637),
-- so the proxy stays more accurate in the post-wp4 window.
-- ACTION_RESET_EVENT :580-588 — _isEventActive = false,
-- _isEventDone re-read from the boss state, setActive(false) drops
-- (no bridge), SetFarVisible(false); _aliveTrash.clear() +
-- _currentWPid = 0 are escort-machine state, unported. Sender is
-- the unported wipe leg (FrostwingGauntletRespawner :268-271), so
-- the receiver is ported but dormant. The latches are NOT cleared on
-- ev23: C++ Reset() -> Initialize() (:540) touches only
-- _didUnderTenPercentText / _wipeCheckTimer.
local crokEventDone = {}
local crokEventActive = {}

local function onCrokArnathIntro2(creature, guid)
    -- C++ UpdateEscortAI pre-loop gate :745 (single-tick no-op per
    -- the marrowgar precedent): 500ms IsCasting defer, no re-arm
    -- consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "arnath2", 500, function() onCrokArnathIntro2(creature, guid) end)
        return
    end
    local arnath = creature:GetInstanceCreature(DATA_CAPTAIN_ARNATH)
    if arnath ~= nil then
        arnath:Talk(SAY_ARNATH_INTRO_2)
    end
end

local function onCrokIntro3(creature, guid)
    -- C++ UpdateEscortAI pre-loop gate :745 (single-tick no-op per
    -- the marrowgar precedent): 500ms IsCasting defer, no re-arm
    -- consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "crok3", 500, function() onCrokIntro3(creature, guid) end)
        return
    end
    creature:Talk(SAY_CROK_INTRO_3)
end

local function crokDoAction(event, creature, action)
    local guid = creature:GetGUIDLow()
    if action == ACTION_START_GAUNTLET then
        if crokEventDone[guid] == nil then
            crokEventDone[guid] = creature:GetBossState(DATA_SISTER_SVALNA) == BOSS_STATE_DONE
        end
        if crokEventDone[guid] or not creature:IsAlive() then
            return
        end
        crokEventActive[guid] = true
        crokEventDone[guid] = true
        local svalna = creature:GetInstanceCreature(DATA_SISTER_SVALNA)
        if svalna ~= nil then
            svalna:DoAction(ACTION_START_GAUNTLET)
        end
        creature:Talk(SAY_CROK_INTRO_1)
        schedule(guid, "arnath2", 7000,
            function() onCrokArnathIntro2(creature, guid) end)
        schedule(guid, "crok3", 14000,
            function() onCrokIntro3(creature, guid) end)
        creature:SetFarVisible(true)
        for itr = 0, 3 do
            local captain = creature:GetInstanceCreature(DATA_CAPTAIN_ARNATH + itr)
            if captain ~= nil then
                captain:DoAction(ACTION_START_GAUNTLET)
            end
        end
    elseif action == ACTION_RESET_EVENT then
        crokEventActive[guid] = false
        crokEventDone[guid] = creature:GetBossState(DATA_SISTER_SVALNA) == BOSS_STATE_DONE
        creature:SetFarVisible(false)
    end
end

RegisterCreatureEvent(ENTRY_CROK_SCOURGEBANE, 1, crokEnterCombat)
RegisterCreatureEvent(ENTRY_CROK_SCOURGEBANE, 2, crokLeaveCombat)
RegisterCreatureEvent(ENTRY_CROK_SCOURGEBANE, 4, crokDied)
RegisterCreatureEvent(ENTRY_CROK_SCOURGEBANE, 9, crokDamageTaken)
RegisterCreatureEvent(ENTRY_CROK_SCOURGEBANE, 23, crokReset)
RegisterCreatureEvent(ENTRY_CROK_SCOURGEBANE, 38, crokDoAction)

-- C++ npc_frostwing_ymirjar_vrykul::JustEngagedWith :1237-1273 —
-- per-entry timer seeds, all re-keyed to event 1 (see the PORTED
-- ARMS header note for the C++-exact arm list). The
-- InterruptNonMeleeSpells(false) (:1242) leg is PORTED first in
-- ymirjarEnterCombat below (gal_darah precedent). The old "dropped —
-- StopSpellCast is an explicit no-op" note conflated two methods and a
-- stale line cite: InterruptNonMeleeSpells is a live bridge
-- (lua_creature_events.go:1995, cancels pending/channel casts). Single-arg
-- DoCast() resolves via UnitAI::DoCast: AITARGET_SELF (arctic
-- chill) -> self-cast; AITARGET_VICTIM (rapid shot, death
-- embrace) -> victim-cast.
local function onYmirjarFrozenOrb(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :1304 (+ in-loop :1372, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "frozen", 500, function() onYmirjarFrozenOrb(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_FROZEN_ORB_CAST)
    schedule(guid, "frozen", {10000, 20000},
        function() onYmirjarFrozenOrb(creature, guid) end)
end

local function onYmirjarBarbaricStrike(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :1304 (+ in-loop :1372, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "barbaric", 500, function() onYmirjarBarbaricStrike(creature, guid) end)
        return
    end
    creature:CastSpell(nil, SPELL_BARBARIC_STRIKE)
    schedule(guid, "barbaric", {2000, 4000},
        function() onYmirjarBarbaricStrike(creature, guid) end)
end

local function onYmirjarAdrenalineRush(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :1304 (+ in-loop :1372, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "adrenaline", 500, function() onYmirjarAdrenalineRush(creature, guid) end)
        return
    end
    creature:AddAura(SPELL_ADRENALINE_RUSH)
    schedule(guid, "adrenaline", {10000, 20000},
        function() onYmirjarAdrenalineRush(creature, guid) end)
end

local function onYmirjarWhirlwind(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :1304 (+ in-loop :1372, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "whirlwind", 500, function() onYmirjarWhirlwind(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_WHIRLWIND)
    schedule(guid, "whirlwind", {6000, 12000},
        function() onYmirjarWhirlwind(creature, guid) end)
end

local function onYmirjarRapidShot(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :1304 (+ in-loop :1372, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "rapid", 500, function() onYmirjarRapidShot(creature, guid) end)
        return
    end
    creature:CastSpell(nil, SPELL_RAPID_SHOT)
    schedule(guid, "rapid", {20000, 30000},
        function() onYmirjarRapidShot(creature, guid) end)
end

local function onYmirjarShoot(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :1304 (+ in-loop :1372, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "shoot", 500, function() onYmirjarShoot(creature, guid) end)
        return
    end
    creature:CastSpell(nil, SPELL_YMIRJAR_SHOOT)
    schedule(guid, "shoot", 2000,
        function() onYmirjarShoot(creature, guid) end)
end

local function onYmirjarShadowBolt(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :1304 (+ in-loop :1372, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "shadowbolt", 500, function() onYmirjarShadowBolt(creature, guid) end)
        return
    end
    creature:CastSpell(nil, SPELL_YMIRJAR_SHADOW_BOLT)
    schedule(guid, "shadowbolt", {5000, 8000},
        function() onYmirjarShadowBolt(creature, guid) end)
end

local function onYmirjarDeathEmbrace(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :1304 (+ in-loop :1372, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "embrace", 500, function() onYmirjarDeathEmbrace(creature, guid) end)
        return
    end
    creature:CastSpell(nil, SPELL_DEATH_EMBRACE)
    schedule(guid, "embrace", {10000, 20000},
        function() onYmirjarDeathEmbrace(creature, guid) end)
end

-- C++ EVENT_YMIRJAR_TWISTED_WINDS :1315-1317 — SelectTarget(
-- Random, 0, 0.f, true) -> DoCast(target, SPELL_TWISTED_WINDS
-- 71306), Repeat {10s,20s} unconditional (outside the target
-- guard); seeded :1249 {10s,20s} on the frostbinder.
local function onYmirjarTwistedWinds(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :1304 (+ in-loop :1372, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "twisted", 500, function() onYmirjarTwistedWinds(creature, guid) end)
        return
    end
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
    -- C++ boss UpdateAI pre-loop gate :1304 (+ in-loop :1372, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "icetrap", 500, function() onYmirjarIceTrap(creature, guid) end)
        return
    end
    local target = randomPlayerTarget(creature, 20, true)
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
    -- C++ boss UpdateAI pre-loop gate :1304 (+ in-loop :1372, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "volley", 500, function() onYmirjarVolley(creature, guid) end)
        return
    end
    local target = randomPlayerTarget(creature, 40, true)
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
    -- C++ boss UpdateAI pre-loop gate :1304 (+ in-loop :1372, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "banish", 500, function() onYmirjarBanish(creature, guid) end)
        return
    end
    local target = randomPlayerTarget(creature, 20, true)
    if target then
        creature:CastSpell(target, SPELL_BANISH)
    end
    schedule(guid, "banish", {10000, 20000},
        function() onYmirjarBanish(creature, guid) end)
end

local function ymirjarEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    creature:InterruptNonMeleeSpells(false)
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
    cancelTimers(creature:GetGUIDLow())
end

-- C++ npc_frostwing_ymirjar_vrykul::JustDied :1215-1220 — only
-- the warlord does anything: RemoveAurasDueToSpell(
-- SPELL_WARLORDS_PRESENCE).
local function ymirjarDied(event, creature, killer)
    cancelTimers(creature:GetGUIDLow())
    if creature:GetEntry() == ENTRY_YMIRJAR_WARLORD then
        creature:RemoveAura(SPELL_WARLORDS_PRESENCE)
    end
end

local function ymirjarReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
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
    RegisterCreatureEvent(entry, 19, ymirjarJustSummoned)
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
-- gated; the position-1 offset rides randomNonHeadPlayerTarget).
local function onBrandonCrusaderStrike(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :1032 (+ in-loop :1061, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "crusader", 500, function() onBrandonCrusaderStrike(creature, guid) end)
        return
    end
    creature:CastSpell(nil, SPELL_CRUSADER_STRIKE)
    schedule(guid, "crusader", {6000, 12000},
        function() onBrandonCrusaderStrike(creature, guid) end)
end

local function onBrandonJudgementOfCommand(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :1032 (+ in-loop :1061, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "judgement", 500, function() onBrandonJudgementOfCommand(creature, guid) end)
        return
    end
    creature:CastSpell(nil, SPELL_JUDGEMENT_OF_COMMAND)
    schedule(guid, "judgement", {8000, 13000},
        function() onBrandonJudgementOfCommand(creature, guid) end)
end

-- C++ EVENT_BRANDON_DIVINE_SHIELD :1043-1046 — 500ms poll
-- (UpdateAI CASTING-gated :1032/:1061; the health-if is the only
-- in-event guard):
-- if (HealthBelowPct(20)) DoCast(me, SPELL_DIVINE_SHIELD 71550);
-- re-arm 500ms unconditional (outside the health-if, crok
-- death-strike pattern).
local function onBrandonDivineShield(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :1032 (+ in-loop :1061, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "divine", 500, function() onBrandonDivineShield(creature, guid) end)
        return
    end
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
-- re-arm {45s,60s} unconditional. The position-1 offset rides
-- randomNonHeadPlayerTarget (threat-list pool, skip-first,
-- playerOnly); the old "offset has no bridge — tyrannus precedent"
-- claim is retracted.
local function onBrandonHammerOfBetrayal(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :1032 (+ in-loop :1061, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "hammer", 500, function() onBrandonHammerOfBetrayal(creature, guid) end)
        return
    end
    local target = randomNonHeadPlayerTarget(creature, nil)
    if target then
        creature:CastSpell(target, SPELL_HAMMER_OF_BETRAYAL)
    end
    schedule(guid, "hammer", {45000, 60000},
        function() onBrandonHammerOfBetrayal(creature, guid) end)
end

local function brandonEnterCombat(event, creature, target)
    -- C++ npc_argent_captainAI::JustEngagedWith (:859):
    -- SetHomePosition(*me) — base-AI engage arm, first in C++ order.
    creature:SetHomePosition(creature:GetX(), creature:GetY(), creature:GetZ(), creature:GetO())
    local guid = creature:GetGUIDLow()
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
    -- C++ boss UpdateAI pre-loop gate :1092 (+ in-loop :1120, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "charge", 500, function() onGrondelCharge(creature, guid) end)
        return
    end
    creature:CastSpell(nil, SPELL_CHARGE)
    schedule(guid, "charge", 500,
        function() onGrondelCharge(creature, guid) end)
end

local function onGrondelMortalStrike(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :1092 (+ in-loop :1120, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "mortal", 500, function() onGrondelMortalStrike(creature, guid) end)
        return
    end
    creature:CastSpell(nil, SPELL_MORTAL_STRIKE)
    schedule(guid, "mortal", {10000, 15000},
        function() onGrondelMortalStrike(creature, guid) end)
end

local function onGrondelSunderArmor(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :1092 (+ in-loop :1120, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "sunder", 500, function() onGrondelSunderArmor(creature, guid) end)
        return
    end
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
    -- C++ boss UpdateAI pre-loop gate :1092 (+ in-loop :1120, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "conflagration", 500, function() onGrondelConflagration(creature, guid) end)
        return
    end
    local target = randomPlayerTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_CONFLAGRATION)
    end
    schedule(guid, "conflagration", {10000, 15000},
        function() onGrondelConflagration(creature, guid) end)
end

local function grondelEnterCombat(event, creature, target)
    -- C++ npc_argent_captainAI::JustEngagedWith (:859):
    -- SetHomePosition(*me) — base-AI engage arm, first in C++ order.
    creature:SetHomePosition(creature:GetX(), creature:GetY(), creature:GetZ(), creature:GetO())
    local guid = creature:GetGUIDLow()
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
    cancelTimers(creature:GetGUIDLow())
end

local function captainCombatReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
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
    cancelTimers(creature:GetGUIDLow())
    local guid = creature:GetGUIDLow()
    if captainFirstDeath[guid] ~= false then
        captainFirstDeath[guid] = false
        creature:Talk(SAY_CAPTAIN_DEATH)
    else
        creature:Talk(SAY_CAPTAIN_SECOND_DEATH)
    end
end

-- C++ npc_argent_captainAI::DoAction :838-856 — PORTED 2026-11-15
-- (crok gauntlet receivers, ev38; all 8 captain entries, living +
-- undead — the AI instance survives UpdateEntry, same struct).
-- ACTION_START_GAUNTLET :840-851 — SetReactState(REACT_DEFENSIVE
-- 1) + MoveFollow(crok, FollowDist, FollowAngle) with FollowAngle
-- = GetAbsoluteAngle(crok) + GetOrientation() (:845-846) and
-- FollowDist = GetDistance2d(crok) (:846); GetAngle /
-- GetDistance2d / GetO are bridged (object.go fallback) and
-- MoveFollow is bridged (lua_creature_events.go:6119). The Go
-- GetAngle returns raw atan2 [-pi, pi] while C++
-- GetAbsoluteAngle normalizes to [0, 2pi) — the difference is a
-- multiple of 2pi, so the follow near-point (cos/sin) is identical.
-- SetFarVisible(true) (:850); setActive(true) drops (no bridge).
-- ACTION_RESET_EVENT :853-855 — _firstDeath = true, i.e. the
-- captainFirstDeath latch clears (nil = first death, matching
-- captainDied's `~= false` read). Sender is crok's DoAction
-- fan-out above, which fires from the ported AT.
local function captainDoAction(event, creature, action)
    local guid = creature:GetGUIDLow()
    if action == ACTION_START_GAUNTLET then
        local crok = creature:GetInstanceCreature(DATA_CROK_SCOURGEBANE)
        if crok ~= nil then
            creature:SetReactState(1) -- REACT_DEFENSIVE
            local angle = creature:GetAngle(crok) + creature:GetO()
            local dist = creature:GetDistance2d(crok)
            creature:MoveFollow(crok, dist, angle)
        end
        creature:SetFarVisible(true)
    elseif action == ACTION_RESET_EVENT then
        captainFirstDeath[guid] = nil
    end
end

local CAPTAIN_DO_ACTION_ENTRIES = {
    NPC_CAPTAIN_ARNATH,
    NPC_CAPTAIN_ARNATH_UNDEAD,
    NPC_CAPTAIN_BRANDON,
    NPC_CAPTAIN_BRANDON_UNDEAD,
    NPC_CAPTAIN_GRONDEL,
    NPC_CAPTAIN_GRONDEL_UNDEAD,
    NPC_CAPTAIN_RUPERT,
    NPC_CAPTAIN_RUPERT_UNDEAD,
}

for _, entry in ipairs(CAPTAIN_DO_ACTION_ENTRIES) do
    RegisterCreatureEvent(entry, 38, captainDoAction)
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
-- DoFindFriendlyMissingBuff, no bridge); dominate mind PORTED
-- below (Is25ManRaid() + undead-entry gates, both bridged).
local function onArnathSmite(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :959 (+ in-loop :991, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "smite", 500, function() onArnathSmite(creature, guid) end)
        return
    end
    local spell = SPELL_SMITE_NORMAL
    if creature:GetEntry() == NPC_CAPTAIN_ARNATH_UNDEAD then
        spell = SPELL_SMITE_UNDEAD
    end
    creature:CastSpell(nil, spell)
    schedule(guid, "smite", {4000, 7000},
        function() onArnathSmite(creature, guid) end)
end

-- C++ npc_captain_arnath::Reset :948-949 — if (Is25ManRaid() &&
-- IsUndead) ScheduleEvent(EVENT_ARNATH_DOMINATE_MIND, 22s, 27s);
-- UpdateAI :982-985 — SelectTarget(Random, 1, 0.0f, true) ->
-- DoCast(target, SPELL_DOMINATE_MIND 14515) non-triggered, re-arm
-- {28s,37s} unconditional (outside the target-if). The IsUndead Reset
-- gate resolves via the current entry (the smite-leg precedent);
-- the offset-1 tank-skip + playerOnly ride randomNonHeadPlayerTarget.
local function onArnathDominateMind(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :959 (+ in-loop :991, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "dominate", 500, function() onArnathDominateMind(creature, guid) end)
        return
    end
    local target = randomNonHeadPlayerTarget(creature, nil)
    if target then
        creature:CastSpell(target, SPELL_DOMINATE_MIND)
    end
    schedule(guid, "dominate", {28000, 37000},
        function() onArnathDominateMind(creature, guid) end)
end

-- C++ npc_captain_arnath::Reset :948 — ScheduleEvent(EVENT_ARNATH_PW_SHIELD,
-- 8s, 14s); UpdateAI :972-975 — DoFindFriendlyMissingBuff(40.0f,
-- SPELL_POWER_WORD_SHIELD) -> DoCast(random pick, shield), re-arm {15s, 20s}.
-- The C++ pool (GridNotifiers.h FriendlyMissingBuffInRange) is alive +
-- in-combat + non-hostile + within 40y + missing the aura, over creatures
-- only (players are not creatures); the Lua pool sweeps exactly the
-- in-encounter non-hostile creature entries (8 captains + Crok 37129),
-- each filtered by those same predicates — hostile ymirjar/Svalna fail
-- the IsHostileTo filter, so the entry sweep is equivalent in-room.
-- Deviation: C++ picks from an unguarded list (empty = UB); Lua guards
-- the empty pool and still re-arms, matching C++'s unconditional re-arm.
local function onArnathPWShield(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :959 (+ in-loop :991, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "pw_shield", 500, function() onArnathPWShield(creature, guid) end)
        return
    end
    local shield = SPELL_POWER_WORD_SHIELD_NORMAL
    if creature:GetEntry() == NPC_CAPTAIN_ARNATH_UNDEAD then
        shield = SPELL_POWER_WORD_SHIELD_UNDEAD
    end
    local pool = {}
    for _, entry in ipairs({NPC_CAPTAIN_ARNATH, NPC_CAPTAIN_BRANDON, NPC_CAPTAIN_GRONDEL, NPC_CAPTAIN_RUPERT, NPC_CAPTAIN_ARNATH_UNDEAD, NPC_CAPTAIN_BRANDON_UNDEAD, NPC_CAPTAIN_GRONDEL_UNDEAD, NPC_CAPTAIN_RUPERT_UNDEAD, ENTRY_CROK_SCOURGEBANE}) do
        for _, c in ipairs(creature:GetCreaturesInGrid(entry, 40.0)) do
            if c:IsAlive() and c:IsInCombat() and not creature:IsHostileTo(c) and not c:HasAura(shield) then
                pool[#pool + 1] = c
            end
        end
    end
    if #pool > 0 then
        creature:CastSpell(pool[math.random(#pool)], shield)
    end
    schedule(guid, "pw_shield", {15000, 20000},
        function() onArnathPWShield(creature, guid) end)
end

local function arnathEnterCombat(event, creature, target)
    -- C++ npc_argent_captainAI::JustEngagedWith (:859):
    -- SetHomePosition(*me) — base-AI engage arm, first in C++ order.
    creature:SetHomePosition(creature:GetX(), creature:GetY(), creature:GetZ(), creature:GetO())
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "smite", {3000, 6000},
        function() onArnathSmite(creature, guid) end)
    schedule(guid, "pw_shield", {8000, 14000},
        function() onArnathPWShield(creature, guid) end)
    if creature:Is25ManRaid() and creature:GetEntry() == NPC_CAPTAIN_ARNATH_UNDEAD then
        schedule(guid, "dominate", {22000, 27000},
            function() onArnathDominateMind(creature, guid) end)
    end
end

-- C++ SelectTarget(Random, N) with playerOnly=FALSE (UnitAI.h:170):
-- random over the threat list, pets/totems eligible. Bridged via
-- creature:GetAITargets() (threat-ordered; lua_creature_events.go:345),
-- the boss_midnight randomTargetAnywhere precedent.
local function randomThreatTarget(creature)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if not t:IsDead() then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ SelectTarget(Random, 1) (UnitAI.h:170-174): like randomThreatTarget
-- but the offset-1 pops the current victim from the victim-first list —
-- the alive-minus-victim idiom on the bridged threat list.
local function randomNonVictimThreatTarget(creature)
    local victim = creature:GetVictim()
    local victimGuid = nil
    if victim ~= nil then
        victimGuid = victim:GetGUID()
    end
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if not t:IsDead() and (victimGuid == nil or t:GetGUID() ~= victimGuid) then
            candidates[#candidates + 1] = t
        end
    end
    if victimGuid == nil then
        -- No victim: the offset pop removes the top-threat entry.
        if #candidates <= 1 then
            return nil
        end
        return candidates[math.random(2, #candidates)]
    end
    -- C++ shortcut: threat list size <= offset ignores ALL elements.
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
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
-- rocket launch exclude the current victim (Random, 1, UnitAI.h —
-- the alive-minus-victim idiom on the bridged threat list).
local function onRupertFelIronBomb(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :1149 (+ in-loop :1175, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "bomb", 500, function() onRupertFelIronBomb(creature, guid) end)
        return
    end
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
    -- C++ boss UpdateAI pre-loop gate :1149 (+ in-loop :1175, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "gun", 500, function() onRupertMachineGun(creature, guid) end)
        return
    end
    local spell = SPELL_MACHINE_GUN_NORMAL
    if creature:GetEntry() == NPC_CAPTAIN_RUPERT_UNDEAD then
        spell = SPELL_MACHINE_GUN_UNDEAD
    end
    local target = randomNonVictimThreatTarget(creature)
    if target then
        creature:CastSpell(target, spell)
    end
    schedule(guid, "gun", {25000, 30000},
        function() onRupertMachineGun(creature, guid) end)
end

local function onRupertRocketLaunch(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :1149 (+ in-loop :1175, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "rocket", 500, function() onRupertRocketLaunch(creature, guid) end)
        return
    end
    local spell = SPELL_ROCKET_LAUNCH_NORMAL
    if creature:GetEntry() == NPC_CAPTAIN_RUPERT_UNDEAD then
        spell = SPELL_ROCKET_LAUNCH_UNDEAD
    end
    local target = randomNonVictimThreatTarget(creature)
    if target then
        creature:CastSpell(target, spell)
    end
    schedule(guid, "rocket", {10000, 15000},
        function() onRupertRocketLaunch(creature, guid) end)
end

local function rupertEnterCombat(event, creature, target)
    -- C++ npc_argent_captainAI::JustEngagedWith (:859):
    -- SetHomePosition(*me) — base-AI engage arm, first in C++ order.
    creature:SetHomePosition(creature:GetX(), creature:GetY(), creature:GetZ(), creature:GetO())
    local guid = creature:GetGUIDLow()
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

