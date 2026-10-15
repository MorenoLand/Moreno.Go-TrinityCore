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
--   (:358) -> Talk(SAY_SVALNA_AGGRO 3) (:499-502; the SetReactState
--   leg has no bridge) modeled as a 9000ms one-shot.
--   The crok->AI()->Talk(SAY_CROK_COMBAT_SVALNA 5) leg (:355-356)
--   is cross-AI Talk (no bridge).
-- boss_sister_svalna::KilledUnit (:364-388) — victim->GetTypeId()
--   == TYPEID_PLAYER -> Talk(SAY_SVALNA_KILL 4); TYPEID_UNIT with
--   a captain entry (37122-37125) -> Talk(SAY_SVALNA_KILL_CAPTAIN
--   1) — the razuvious player-gated variant precedent plus
--   victim:GetEntry() for creatures.
-- boss_sister_svalna::JustDied (:334-351) — Talk(SAY_SVALNA_DEATH
--   6) (:337) unconditional, the sjonnir JustDied-Talk precedent;
--   the CaptainSurviveTalk cross-AI event legs (:340-350) ride the
--   absent instance / cross-AI bridges.
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
--   (:359, 40-50s, re-arm 20-25s, :500-505) — the SelectTarget(
--   Random, 1, ...) leg has no bridge; the DoCast(me,
--   SPELL_AETHER_SHIELD 71463) sub-leg rides the same event.
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
-- boss_sister_svalna SpellHit (:430-437) — Talk(
--   EMOTE_SVALNA_BROKEN_SHIELD 8, caster) on SPELL_HURL_SPEAR
--   71466 rides the SpellHit arm (event 15 never fires; the
--   anub_arak precedent); the RemoveAurasDueToSpell leg has no
--   aura bridge on this path.
-- boss_sister_svalna SpellHitTarget (:452-476) — Talk(
--   EMOTE_SVALNA_IMPALE 7, unitTarget) on SPELL_IMPALING_SPEAR
--   71443 rides the SpellHitTarget arm (no bridge); the
--   SPELL_IMPALING_SPEAR_KILL Unit::Kill leg and the
--   NPC_IMPALING_SPEAR summon + vehicle-cast legs are summon
--   machines.
-- boss_sister_svalna JustReachedHome (:389-395) /
--   MovementInform (:439-450) — SetReactState / SetDisableGravity
--   / SetHover legs, no Talk of Svalna's own (no MovementInform
--   bridge).
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
--   latch; the event-9 damage-rewrite bridge cannot express the
--   health-modify / re-arm legs.
-- npc_crok_scourgebane SetGUID / WaypointReached / WaypointStarted
--   (:591-689) — gauntlet waypoint machine, no bridge.
-- npc_frostwing_ymirjar_vrykul UpdateAI random-target arms —
--   EVENT_YMIRJAR_TWISTED_WINDS (71306),
--   EVENT_YMIRJAR_ICE_TRAP (71249), EVENT_YMIRJAR_VOLLEY (71252),
--   EVENT_YMIRJAR_BANISH (71298): all SelectTarget(Random, ...)
--   legs, no bridge; EVENT_YMIRJAR_SUMMON (71303) is a summon
--   machine (+ _events.DelayEvents(2s) leg, no bridge).
-- npc_frostwing_ymirjar_vrykul Reset (:1189-1213) — the
--   RemoveAurasDueToSpell(SPELL_ARCTIC_CHILL) leg has no aura
--   bridge on this path; the OOC EVENT_YMIRJAR_SPIRIT_STREAM
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

local SAY_SVALNA_KILL_CAPTAIN = 1
local SAY_SVALNA_AGGRO = 3
local SAY_SVALNA_KILL = 4
local SAY_SVALNA_DEATH = 6

local SAY_CAPTAIN_DEATH = 0
local SAY_CAPTAIN_SECOND_DEATH = 3

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

-- C++ boss_sister_svalna::JustEngagedWith :353-362 — DoCastSelf(
-- SPELL_DIVINE_SURGE, true) is a TRIGGERED self-cast; the
-- EVENT_SVALNA_COMBAT seed (9s) is a one-shot Talk arm. The
-- EVENT_IMPALING_SPEAR (40-50s) arm is random-target gated (no
-- bridge); EVENT_AETHER_SHIELD (100-110s) is a dead event (no
-- case in the UpdateAI switch).
local function onSvalnaCombat(creature)
    creature:Talk(SAY_SVALNA_AGGRO)
end

local function svalnaEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:CastSpell(creature, SPELL_DIVINE_SURGE, true)
    schedule(guid, "combat", 9000, function() onSvalnaCombat(creature) end)
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

local function svalnaReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_SISTER_SVALNA, 1, svalnaEnterCombat)
RegisterCreatureEvent(ENTRY_SISTER_SVALNA, 2, svalnaLeaveCombat)
RegisterCreatureEvent(ENTRY_SISTER_SVALNA, 3, svalnaKilledUnit)
RegisterCreatureEvent(ENTRY_SISTER_SVALNA, 4, svalnaJustDied)
RegisterCreatureEvent(ENTRY_SISTER_SVALNA, 23, svalnaReset)

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
    if creature:GetHealthPct() < 20 then
        creature:CastSpell(nil, SPELL_DEATH_STRIKE)
    end
    schedule(guid, "death", {5000, 10000},
        function() onCrokDeathStrike(creature, guid) end)
end

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
-- ARMS header note for the C++-exact arm list). Single-arg
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

local function ymirjarEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    local entry = creature:GetEntry()
    if entry == ENTRY_YMIRJAR_FROSTBINDER then
        creature:CastSpell(creature, SPELL_ARCTIC_CHILL)
        schedule(guid, "frozen", {5000, 10000},
            function() onYmirjarFrozenOrb(creature, guid) end)
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
        schedule(guid, "shoot", 1000,
            function() onYmirjarShoot(creature, guid) end)
    elseif entry == ENTRY_YMIRJAR_DEATHBRINGER then
        schedule(guid, "shadowbolt", {6000, 12000},
            function() onYmirjarShadowBolt(creature, guid) end)
        schedule(guid, "embrace", {5000, 10000},
            function() onYmirjarDeathEmbrace(creature, guid) end)
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

-- C++ npc_argent_captainAI::JustDied (:820-829): the _firstDeath latch
-- (true at construction) — first death Talk(SAY_CAPTAIN_DEATH 0) and
-- flips the latch; later deaths Talk(SAY_CAPTAIN_SECOND_DEATH 3). The
-- latch is per-AI-instance and survives in-place resurrection, so it is
-- keyed by GUID and never cleared on events 2/23 (a fresh spawn gets a
-- fresh GUID, matching the ctor). The 4 captain entries are Go-side
-- registered (boss_ai.go RegisterLuaBoss lines).
local captainFirstDeath = {}

local CAPTAIN_ENTRIES = {
    NPC_CAPTAIN_ARNATH,
    NPC_CAPTAIN_BRANDON,
    NPC_CAPTAIN_GRONDEL,
    NPC_CAPTAIN_RUPERT,
}

local function captainDied(event, creature, killer)
    local guid = creature:GetGUID()
    if captainFirstDeath[guid] ~= false then
        captainFirstDeath[guid] = false
        creature:Talk(SAY_CAPTAIN_DEATH)
    else
        creature:Talk(SAY_CAPTAIN_SECOND_DEATH)
    end
end

for _, entry in ipairs(CAPTAIN_ENTRIES) do
    RegisterCreatureEvent(entry, 4, captainDied)
end
