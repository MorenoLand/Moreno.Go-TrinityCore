-- Argent Captains (Icecrown Citadel, Frostwing Halls gauntlet) —
-- Lua port of the npc_argent_captainAI base and the four derived
-- structs in src/server/scripts/Northrend/IcecrownCitadel/
-- boss_sister_svalna.cpp (npc_captain_arnath / npc_captain_brandon
-- / npc_captain_grondel / npc_captain_rupert, all deriving from
-- npc_argent_captainAI (ScriptedAI); registered from inside
-- AddSC_boss_sister_svalna() (:1495-1509); loader decl 180 / call
-- 375 per northrend_script_loader.cpp — the TENTH group of the
-- "// Icecrown Citadel" block).
-- Entries: 37122 Captain Arnath / 37123 Captain Brandon /
-- 37124 Captain Grondel / 37125 Captain Rupert
-- (icecrown_citadel.h NPC_CAPTAIN_ARNATH..RUPERT, lines 270–273;
-- instance_icecrown_citadel.cpp OnCreatureCreate binds all four,
-- lines 282–286) plus the undead variants 37491 / 37493 / 37494 /
-- 37495 (icecrown_citadel.h NPC_CAPTAIN_*_UNDEAD, lines 274–277).
-- C++ switches entry via me->UpdateEntry() on
-- SPELL_REVIVE_CHAMPION hit (:894-934) without replacing the AI;
-- Eluna dispatches by current entry (lua_creature_events.go
-- fireCreatureLuaEvent uses motion.Entry at event time), so the
-- undead entries share these handlers — entry-verifiable,
-- registration proceeds (the nexus_commanders kalecgos
-- precedent); the CreatureScript ScriptName bindings are DB-side
-- as usual.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent
-- ({min,max} table = randtime range); GUID-keyed timers table,
-- cancelled on events 2/4/23 (the falric / lady_deathwhisper
-- idiom); melee is engine-driven in Go (creature combat tick),
-- like C++ DoMeleeAttackIfReady.
--
-- DOCTRINE RETRACTION (2026-10-13): the earlier port of this file
-- treated the whole EventMap machine as unbridgeable under the
-- "no-timer-bridge" doctrine and ported the KilledUnit Talk arm
-- only, and claimed the per-AI _firstDeath state had no port
-- precedent. That doctrine is disproved and stale —
-- CreateLuaEvent is a real registered bridge
-- (engine/scripting/runtime.go), timer-driven cast arms port on
-- the maiden_of_virtue / moroes / curator / nightbane convention,
-- and per-GUID state tables are established (the falric /
-- garfrost phase-latch precedent): CastSpell(nil, spellId) =
-- victim-cast (bridge falls back to TargetGUID);
-- CastSpell(creature, spellId) = self-cast; {min,max} delay =
-- randtime range; Reset()-scheduled timers re-key to event 1.
-- The undead spell variants (the IsUndead macro legs) resolve at
-- cast time via creature:GetEntry() — the undead entry is the
-- observable IsUndead state — since the SPELL_REVIVE_CHAMPION
-- SpellHit flip itself has no bridge. CAVEAT: IsUndead is a private
-- AI bool set only by the unbridged SpellHit (:898); a
-- directly-spawned undead entry behaves as living in C++ (skips all
-- the IsUndead Reset gates) while the Lua entry-resolution treats it
-- as undead — no bridge can read the private bool, entry-resolution
-- is the best available approximation; and revived captains skip
-- C++ Reset entirely (setDeathState(JUST_RESPAWNED),
-- Creature.cpp:2117-2153 — Reset only via Creature::Respawn :2198)
-- so they gain no new timers until the next evade/respawn, while Lua
-- re-seeds the event-1 timers — documented, no bridge for the
-- SpellHit revive signal.
--
-- PORTED ARMS (C++ line refs; all C++-exact for the modeled arms):
-- npc_argent_captainAI::KilledUnit (:832-836) — victim->GetTypeId()
--   == TYPEID_PLAYER -> Talk(SAY_CAPTAIN_KILL 2) — the razuvious
--   player-gated variant precedent (event 3).
-- npc_argent_captainAI::JustDied (:821-831) — the per-AI
--   _firstDeath flag: first death -> Talk(SAY_CAPTAIN_DEATH 0),
--   second death -> Talk(SAY_CAPTAIN_SECOND_DEATH 3) (event 4,
--   the sjonnir JustDied-Talk precedent; per-GUID state seeded on
--   event 1, cleared on event 23 — the event-23 analogue of the
--   ACTION_RESET_EVENT _firstDeath reset, :853-855; DIVERGENCE: C++
--   clears _firstDeath ONLY on DoAction(ACTION_RESET_EVENT) from the
--   gauntlet respawner on wipe (:853-855), so after a plain evade with
--   no gauntlet reset C++'s next death Talks 3 while Lua re-seeds
--   firstDeath on event 1 and Talks 0 — documented, no bridge for the
--   gauntlet-wipe signal).
-- npc_captain_arnath::Reset (:942-950) + UpdateAI (:952-1006) —
--   Reset-seeded EVENT_ARNATH_SMITE {3000,6000}ms re-keyed to
--   event 1: DoCastVictim(SPELL_SMITE) — SPELL_SMITE_NORMAL 71546
--   / SPELL_SMITE_UNDEAD 71778 selected by current entry at cast
--   time — re-arm {4000,7000}ms unconditional.
-- npc_captain_brandon::Reset (:1015-1023) + UpdateAI (:1025-1066)
--   — Reset-seeded timers re-keyed to event 1:
--   EVENT_BRANDON_CRUSADER_STRIKE {6000,10000}ms ->
--   DoCastVictim(SPELL_CRUSADER_STRIKE 71549), re-arm
--   {6000,12000}ms; EVENT_BRANDON_DIVINE_SHIELD 500ms re-arm ->
--   gated on HealthBelowPct(20) -> DoCast(me, SPELL_DIVINE_SHIELD
--   71550) (the 500ms re-arm sits outside the health-if,
--   :1043-1046); EVENT_BRANDON_JUDGEMENT_OF_COMMAND
--   {8000,13000}ms -> DoCastVictim(SPELL_JUDGEMENT_OF_COMMAND
--   71551), re-arm {8000,13000}ms.
-- npc_captain_grondel::Reset (:1075-1083) + UpdateAI (:1085-1125)
--   — EVENT_GRONDEL_CONFLAGRATION (IsUndead-gated Reset seed
--   {12000,17000}ms, re-keyed to event 1) — SelectTarget(Random, 0,
--   0.0f, true) DoCast(target, SPELL_CONFLAGRATION 71785) (2-arg,
--   non-triggered, offset 0 — tank INCLUDED, the_beast /
--   opera-Julianne random-pool convention with the
--   threat-list-vs-player-pool caveat) — re-arm {10000,15000}ms
--   unconditional (outside the target-if); resolved at event 1 by
--   current entry (the undead entry is the observable IsUndead
--   state — the smite-leg precedent).
-- npc_captain_rupert::Reset (:1134-1140) + UpdateAI (:1142-1180)
--   — EVENT_RUPERT_FEL_IRON_BOMB (Reset seed {15000,20000}ms,
--   re-keyed to event 1) — SelectTarget(Random, 0) DoCast(target,
--   SPELL_FEL_IRON_BOMB) (2-arg, non-triggered, offset 0) —
--   SPELL_FEL_IRON_BOMB_NORMAL 71592 / SPELL_FEL_IRON_BOMB_UNDEAD
--   71787 selected by current entry at cast time — re-arm
--   {15000,20000}ms unconditional (outside the target-if).
-- npc_captain_grondel::Reset (:1075-1083) + UpdateAI (:1085-1125)
--   — Reset-seeded timers re-keyed to event 1:
--   EVENT_GRONDEL_CHARGE_CHECK 500ms re-arm ->
--   DoCastVictim(SPELL_CHARGE 71553); EVENT_GRONDEL_MORTAL_STRIKE
--   {8000,14000}ms -> DoCastVictim(SPELL_MORTAL_STRIKE 71552),
--   re-arm {10000,15000}ms; EVENT_GRONDEL_SUNDER_ARMOR
--   {3000,12000}ms -> DoCastVictim(SPELL_SUNDER_ARMOR 71554),
--   re-arm {5000,17000}ms.
--
-- DOCUMENTED-ONLY (no bridgeable arm):
-- npc_argent_captainAI::SpellHit (:894-922) — stale "no SpellHit
--   bridge" retracted (2026-10-21): ev14 IS bridged (fires from the Go
--   spell path, boss_ai.go:2125/2134 + pet_combat.go:5222 +
--   spells.go:11765); the old "event 14 never fires" parenthetical
--   (runtime.go:195-214, the anub_arak precedent) misattributed the
--   events-constant block — but the arm stays out: SPELL_REVIVE_
--   CHAMPION 70053 is never cast in Go (Svalna's EVENT_SVALNA_
--   RESURRECT DoCast rides the unbridged gauntlet DoAction machine —
--   boss_sister_svalna.lua DOCUMENTED-ONLY), and the arm's core legs
--   have no bridge: UpdateEntry (zero Lua hits) and setDeathState(
--   JUST_RESPAWNED). Talk(1) + the UNDEATH 70089 self-cast alone would
--   Talk a living-entry captain the revive never flips — C++-inexact.
-- npc_argent_captainAI::DoAction (:838-857) —
--   ACTION_START_GAUNTLET follow / active / far-visible legs and
--   ACTION_RESET_EVENT bookkeeping legs (no-DoAction bridge).
-- npc_argent_captainAI::JustEngagedWith (:859-864) —
--   SetHomePosition(*me) PORTED 2026-10-20 (event-surface
--   SetHomePosition + GetX/GetY/GetZ/GetO, first in C++ order);
--   the IsUndead-gated DoZoneInCombat leg stays out — 2026-10-23
--   re-evaluation against the live bridge: IsUndead flips true only via
--   the Svalna REVIVE_CHAMPION SpellHit -> UpdateEntry leg
--   (boss_sister_svalna.cpp:894-898), which never fires in Go (the
--   resurrect rides the unbridged gauntlet DoAction machine), so the
--   gate is dead in the ported model.
-- npc_argent_captainAI::EnterEvadeMode (:869-892) —
--   MoveFollow + far-visible/active legs (no movement bridge).
-- npc_captain_arnath EVENT_ARNATH_FLASH_HEAL (:966-969,
--   {4000,7000}ms, re-arm {6000,9000}ms) — DoCast on the
--   FindFriendlyCreature() most-HP-missing search (:990-997, no
--   friendly search bridge); EVENT_ARNATH_PW_SHIELD (:971-975,
--   {8000,14000}ms, re-arm {15000,20000}ms) — DoFindFriendlyMissingBuff
--   list + random element (no bridge); EVENT_ARNATH_DOMINATE_MIND
--   (:982-985, {22000,27000}ms, re-arm {28000,37000}ms) —
--   SelectTarget(Random, 1) offset-1 tank-skip + Is25ManRaid() +
--   IsUndead Reset gate (:948-949) — stays out.
-- npc_captain_brandon EVENT_BRANDON_HAMMER_OF_BETRAYAL
--   (:1052-1055, {25000,30000}ms, re-arm {45000,60000}ms) —
--   SelectTarget(Random, 1) offset-1 tank-skip + IsUndead Reset gate
--   (:1021-1022) — stays out.
-- npc_captain_grondel EVENT_GRONDEL_CONFLAGRATION is PORTED
--   above (offset 0, bridgeable); the remaining grondel arms are
--   all ported.
-- npc_captain_rupert UpdateAI (:1142-1180) —
--   EVENT_RUPERT_MACHINE_GUN (:1161-1164, 71594/71788,
--   {25000,30000}ms init) and EVENT_RUPERT_ROCKET_LAUNCH
--   (:1166-1169, 71590/71786, {10000,15000}ms init) — both
--   SelectTarget(Random, 1) offset-1 tank-skip casts (the offset-1
--   tank-skip-no-bridge blocker, netherspite precedent) — stay out.
--   EVENT_RUPERT_FEL_IRON_BOMB is PORTED above (offset 0,
--   bridgeable).
-- All base-AI Talk() calls accounted for (4 = SAY_CAPTAIN_DEATH
--   0 (1) + SAY_CAPTAIN_SECOND_DEATH 3 (1) + SAY_CAPTAIN_KILL 2
--   (1) + SAY_CAPTAIN_RESURRECTED 1 (1, SpellHit)); the 4
--   derived structs add no Talk of their own.
-- Go registration check this run: RegisterLuaBoss(
--   "npc_captain_arnath", 37122) / brendon 37123 / grondel 37124 /
--   rupert 37125 exist (boss_ai.go); the 4 undead entries
--   (37491/37493/37494/37495) were missing and are added by this
--   tick.

local ENTRY_CAPTAIN_ARNATH = 37122
local ENTRY_CAPTAIN_BRANDON = 37123
local ENTRY_CAPTAIN_GRONDEL = 37124
local ENTRY_CAPTAIN_RUPERT = 37125

local ENTRY_CAPTAIN_ARNATH_UNDEAD = 37491
local ENTRY_CAPTAIN_BRANDON_UNDEAD = 37493
local ENTRY_CAPTAIN_GRONDEL_UNDEAD = 37494
local ENTRY_CAPTAIN_RUPERT_UNDEAD = 37495

local SAY_CAPTAIN_DEATH = 0
local SAY_CAPTAIN_KILL = 2
local SAY_CAPTAIN_SECOND_DEATH = 3

local SPELL_SMITE_NORMAL = 71546
local SPELL_SMITE_UNDEAD = 71778

local SPELL_CRUSADER_STRIKE = 71549
local SPELL_DIVINE_SHIELD = 71550
local SPELL_JUDGEMENT_OF_COMMAND = 71551

local SPELL_CHARGE = 71553
local SPELL_MORTAL_STRIKE = 71552
local SPELL_SUNDER_ARMOR = 71554

local SPELL_CONFLAGRATION = 71785
local SPELL_FEL_IRON_BOMB_NORMAL = 71592
local SPELL_FEL_IRON_BOMB_UNDEAD = 71787

-- C++ SelectTarget(Random, ...) — the map-wide random-player pool
-- (the_beast / opera-Julianne convention): C++ picks from the threat
-- list, the Lua pool is all alive players on the same map+instance —
-- threat-list-vs-player-pool caveat applies (netherspite precedent).
local function randomPlayer(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local candidates = {}
    for _, plr in ipairs(GetPlayersInWorld()) do
        if plr:GetMapId() == mapId and plr:GetInstanceId() == instanceId
                and not plr:IsDead() then
            candidates[#candidates + 1] = plr
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

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

-- C++ npc_argent_captainAI::_firstDeath — per-GUID state (the
-- falric phase-latch precedent); seeded on event 1, cleared on
-- event 23 (the event-23 analogue of the ACTION_RESET_EVENT
-- reset).
local captainState = {}

local function getCaptainState(guid)
    local st = captainState[guid]
    if not st then
        st = { firstDeath = true }
        captainState[guid] = st
    end
    return st
end

-- C++ npc_captain_arnath::Reset :942-950 seeds
-- EVENT_ARNATH_SMITE {3000,6000}ms; UpdateAI :974-977:
-- DoCastVictim(SPELL_SMITE), re-arm {4000,7000}ms unconditional.
-- The SPELL_SMITE macro leg (IsUndead ? UNDEAD : NORMAL) resolves
-- at cast time via the current entry — the undead entry is the
-- observable IsUndead state.
local function onArnathSmite(creature, guid)
    local spell = SPELL_SMITE_NORMAL
    if creature:GetEntry() == ENTRY_CAPTAIN_ARNATH_UNDEAD then
        spell = SPELL_SMITE_UNDEAD
    end
    creature:CastSpell(nil, spell)
    schedule(guid, "smite", {4000, 7000},
        function() onArnathSmite(creature, guid) end)
end

local function arnathEnterCombat(event, creature, target)
    -- C++ npc_argent_captainAI::JustEngagedWith (:859): me->SetHomePosition(*me)
    -- is the base-AI engage arm (the svalna :386 precedent); the IsUndead
    -- DoZoneInCombat leg stays out (2026-10-23 re-eval — the IsUndead
    -- gate is dead in Go; see header).
    creature:SetHomePosition(creature:GetX(), creature:GetY(), creature:GetZ(), creature:GetO())
    local guid = creature:GetGUID()
    cancelTimers(guid)
    getCaptainState(guid)
    schedule(guid, "smite", {3000, 6000},
        function() onArnathSmite(creature, guid) end)
end

-- C++ npc_captain_brandon::Reset :1015-1023 seeds
-- EVENT_BRANDON_CRUSADER_STRIKE {6000,10000}ms,
-- EVENT_BRANDON_DIVINE_SHIELD 500ms,
-- EVENT_BRANDON_JUDGEMENT_OF_COMMAND {8000,13000}ms; UpdateAI
-- :1031-1055: crusader strike -> DoCastVictim(71549), re-arm
-- {6000,12000}ms; divine shield -> gated on HealthBelowPct(20)
-- -> DoCast(me, 71550), re-arm 500ms unconditional; judgement ->
-- DoCastVictim(71551), re-arm {8000,13000}ms.
local function onBrandonCrusaderStrike(creature, guid)
    creature:CastSpell(nil, SPELL_CRUSADER_STRIKE)
    schedule(guid, "crusader", {6000, 12000},
        function() onBrandonCrusaderStrike(creature, guid) end)
end

-- C++ HealthBelowPct(20) is health < floor(max*20/100) (Unit.h:872);
-- the float GetHealthPct() bridge fires up to 1 HP early, so the
-- integer floor form is used (the valithria / garfrost boundary
-- precedent).
local function onBrandonDivineShield(creature, guid)
    if creature:GetHealth() < math.floor(creature:GetMaxHealth() * 20 / 100) then
        creature:CastSpell(creature, SPELL_DIVINE_SHIELD)
    end
    schedule(guid, "divine", 500,
        function() onBrandonDivineShield(creature, guid) end)
end

local function onBrandonJudgement(creature, guid)
    creature:CastSpell(nil, SPELL_JUDGEMENT_OF_COMMAND)
    schedule(guid, "judgement", {8000, 13000},
        function() onBrandonJudgement(creature, guid) end)
end

local function brandonEnterCombat(event, creature, target)
    -- C++ npc_argent_captainAI::JustEngagedWith (:859): me->SetHomePosition(*me)
    -- is the base-AI engage arm (the svalna :386 precedent); the IsUndead
    -- DoZoneInCombat leg stays out (2026-10-23 re-eval — the IsUndead
    -- gate is dead in Go; see header).
    creature:SetHomePosition(creature:GetX(), creature:GetY(), creature:GetZ(), creature:GetO())
    local guid = creature:GetGUID()
    cancelTimers(guid)
    getCaptainState(guid)
    schedule(guid, "crusader", {6000, 10000},
        function() onBrandonCrusaderStrike(creature, guid) end)
    schedule(guid, "divine", 500,
        function() onBrandonDivineShield(creature, guid) end)
    schedule(guid, "judgement", {8000, 13000},
        function() onBrandonJudgement(creature, guid) end)
end

-- C++ npc_captain_grondel::Reset :1075-1083 seeds
-- EVENT_GRONDEL_CHARGE_CHECK 500ms, EVENT_GRONDEL_MORTAL_STRIKE
-- {8000,14000}ms, EVENT_GRONDEL_SUNDER_ARMOR {3000,12000}ms;
-- UpdateAI :1091-1114: charge -> DoCastVictim(71553), re-arm
-- 500ms; mortal strike -> DoCastVictim(71552), re-arm
-- {10000,15000}ms; sunder armor -> DoCastVictim(71554), re-arm
-- {5000,17000}ms.
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

-- C++ npc_captain_grondel::Reset :1081-1082 (IsUndead-gated) seeds
-- EVENT_GRONDEL_CONFLAGRATION {12000,17000}ms; UpdateAI :1111-1114:
-- if (Unit* target = SelectTarget(SelectTargetMethod::Random, 0,
-- 0.0f, true)) DoCast(target, SPELL_CONFLAGRATION 71785) (2-arg,
-- non-triggered, offset 0 — tank INCLUDED), re-arm {10000,15000}ms
-- unconditional (outside the target-if). The IsUndead Reset gate
-- resolves via the current entry (the smite-leg precedent).
local function onGrondelConflagration(creature, guid)
    local target = randomPlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_CONFLAGRATION)
    end
    schedule(guid, "conflagration", {10000, 15000},
        function() onGrondelConflagration(creature, guid) end)
end

local function grondelEnterCombat(event, creature, target)
    -- C++ npc_argent_captainAI::JustEngagedWith (:859): me->SetHomePosition(*me)
    -- is the base-AI engage arm (the svalna :386 precedent); the IsUndead
    -- DoZoneInCombat leg stays out (2026-10-23 re-eval — the IsUndead
    -- gate is dead in Go; see header).
    creature:SetHomePosition(creature:GetX(), creature:GetY(), creature:GetZ(), creature:GetO())
    local guid = creature:GetGUID()
    cancelTimers(guid)
    getCaptainState(guid)
    schedule(guid, "charge", 500,
        function() onGrondelCharge(creature, guid) end)
    schedule(guid, "mortal", {8000, 14000},
        function() onGrondelMortalStrike(creature, guid) end)
    schedule(guid, "sunder", {3000, 12000},
        function() onGrondelSunderArmor(creature, guid) end)
    -- C++ Reset :1081: if (IsUndead) ScheduleEvent(EVENT_GRONDEL_
    -- CONFLAGRATION, 12s, 17s) — entry-resolved (the smite-leg
    -- precedent).
    if creature:GetEntry() == ENTRY_CAPTAIN_GRONDEL_UNDEAD then
        schedule(guid, "conflagration", {12000, 17000},
            function() onGrondelConflagration(creature, guid) end)
    end
end

-- C++ npc_captain_rupert::Reset :1137 seeds EVENT_RUPERT_FEL_IRON_BOMB
-- {15000,20000}ms; UpdateAI :1157-1159: if (Unit* target =
-- SelectTarget(SelectTargetMethod::Random, 0)) DoCast(target,
-- SPELL_FEL_IRON_BOMB) (2-arg, non-triggered, offset 0 — tank
-- INCLUDED), re-arm {15000,20000}ms unconditional (outside the
-- target-if). SPELL_FEL_IRON_BOMB macro (IsUndead ? 71787 : 71592)
-- resolves by current entry at cast time (the smite-leg precedent).
local function onRupertFelIronBomb(creature, guid)
    local spell = SPELL_FEL_IRON_BOMB_NORMAL
    if creature:GetEntry() == ENTRY_CAPTAIN_RUPERT_UNDEAD then
        spell = SPELL_FEL_IRON_BOMB_UNDEAD
    end
    local target = randomPlayer(creature)
    if target then
        creature:CastSpell(target, spell)
    end
    schedule(guid, "felironbomb", {15000, 20000},
        function() onRupertFelIronBomb(creature, guid) end)
end

-- C++ npc_captain_rupert::Reset :1134-1140 + UpdateAI :1142-1180
-- — EVENT_RUPERT_FEL_IRON_BOMB is PORTED above (offset 0,
-- bridgeable); EVENT_RUPERT_MACHINE_GUN and
-- EVENT_RUPERT_ROCKET_LAUNCH stay out (offset-1 tank-skip).
local function rupertEnterCombat(event, creature, target)
    -- C++ npc_argent_captainAI::JustEngagedWith (:859): me->SetHomePosition(*me)
    -- is the base-AI engage arm (the svalna :386 precedent); the IsUndead
    -- DoZoneInCombat leg stays out (2026-10-23 re-eval — the IsUndead
    -- gate is dead in Go; see header).
    creature:SetHomePosition(creature:GetX(), creature:GetY(), creature:GetZ(), creature:GetO())
    local guid = creature:GetGUID()
    cancelTimers(guid)
    getCaptainState(guid)
    schedule(guid, "felironbomb", {15000, 20000},
        function() onRupertFelIronBomb(creature, guid) end)
end

local function captainLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function captainReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    captainState[guid] = nil
end

-- C++ npc_argent_captainAI::KilledUnit :832-836:
-- if (victim->GetTypeId() == TYPEID_PLAYER) Talk(SAY_CAPTAIN_KILL)
-- — the razuvious player-gated variant precedent.
local function captainKilledUnit(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_CAPTAIN_KILL)
    end
end

-- C++ npc_argent_captainAI::JustDied :821-831 — the per-AI
-- _firstDeath flag: first death Talk(SAY_CAPTAIN_DEATH),
-- second death Talk(SAY_CAPTAIN_SECOND_DEATH) — the sjonnir
-- JustDied-Talk precedent.
local function captainJustDied(event, creature, killer)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    local st = getCaptainState(guid)
    if st.firstDeath then
        st.firstDeath = false
        creature:Talk(SAY_CAPTAIN_DEATH)
    else
        creature:Talk(SAY_CAPTAIN_SECOND_DEATH)
    end
end

local CAPTAIN_HANDLERS = {
    { ENTRY_CAPTAIN_ARNATH, arnathEnterCombat },
    { ENTRY_CAPTAIN_ARNATH_UNDEAD, arnathEnterCombat },
    { ENTRY_CAPTAIN_BRANDON, brandonEnterCombat },
    { ENTRY_CAPTAIN_BRANDON_UNDEAD, brandonEnterCombat },
    { ENTRY_CAPTAIN_GRONDEL, grondelEnterCombat },
    { ENTRY_CAPTAIN_GRONDEL_UNDEAD, grondelEnterCombat },
    { ENTRY_CAPTAIN_RUPERT, rupertEnterCombat },
    { ENTRY_CAPTAIN_RUPERT_UNDEAD, rupertEnterCombat },
}

for _, pair in ipairs(CAPTAIN_HANDLERS) do
    local entry, onEnter = pair[1], pair[2]
    RegisterCreatureEvent(entry, 1, onEnter)
    RegisterCreatureEvent(entry, 2, captainLeaveCombat)
    RegisterCreatureEvent(entry, 3, captainKilledUnit)
    RegisterCreatureEvent(entry, 4, captainJustDied)
    RegisterCreatureEvent(entry, 23, captainReset)
end
