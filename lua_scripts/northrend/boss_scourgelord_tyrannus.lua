-- Scourgelord Tyrannus (Pit of Saron) — Lua port of
-- src/server/scripts/Northrend/FrozenHalls/PitOfSaron/boss_scourgelord_tyrannus.cpp
-- (571 lines incl. license; 6 scripts — boss_tyrannus (CreatureScript via
-- GetPitOfSaronAI (BossAI), DATA_TYRANNUS = 6); boss_rimefang (CreatureScript
-- via GetPitOfSaronAI (ScriptedAI)); player_overlord_brandAI (PlayerAI pushed
-- onto overlord-brand victims: DamageDealt -> SPELL_OVERLORD_BRAND_DAMAGE,
-- HealDone -> SPELL_OVERLORD_BRAND_HEAL);
-- spell_tyrannus_overlord_brand / spell_tyrannus_mark_of_rimefang
-- (AuraScripts); spell_tyrannus_rimefang_icy_blast (SpellScript: EFFECT_1
-- SPELL_EFFECT_TRIGGER_MISSILE -> SummonCreature(NPC_ICY_BLAST) + aura);
-- at_tyrannus_event_starter (AreaTriggerScript: tyrannus->AI()->
-- DoAction(ACTION_START_INTRO)); all registered from inside
-- AddSC_boss_tyrannus(); loader decl 164 / call 359 per
-- northrend_script_loader.cpp — the FIFTH and LAST group of the
-- "// Pit of Saron" block in AddNorthrendScripts(), immediately after
-- AddSC_boss_ick() (call 358); the block CLOSES with it — the call after
-- it opens the next Northrend dungeon block per the loader (verified from
-- the loader this run); the checkpoint sequence (boss_ick -> boss_tyrannus)
-- is followed).
-- Entries: 36658 Tyrannus (pit_of_saron.h NPC_TYRANNUS, line 52;
-- instance_pit_of_saron.cpp OnCreatureCreate binds case NPC_TYRANNUS,
-- line 83) — entry-verifiable, registration proceeds (the
-- nexus_commanders kalecgos precedent); the CreatureScript ScriptName
-- binding is DB-side as usual. 36661 Rimefang (pit_of_saron.h NPC_RIMEFANG,
-- line 53; OnCreatureCreate-bound line 86) is entry-verifiable but carries
-- ZERO own Talk lines — NOT registered (the bronjahm
-- npc_corrupted_soul_fragment precedent). NPC_ICY_BLAST = 36731
-- (pit_of_saron.h, line 94); DATA_RIMEFANG = 7 (line 40);
-- DATA_TYRANNUS_EVENT = 7 (line 41).
-- Sole-source verified: whole-server-tree grep for "AddSC_boss_tyrannus"
-- hits boss_scourgelord_tyrannus.cpp only (+ the loader decl/call lines);
-- this clone carries no sql/ tree, so ScriptName bindings are DB-side by
-- construction. No tyrannus lua existed.
-- Eluna creature events: 1 OnEnterCombat, 3 OnKill (player-gated), 4 OnDied.
-- Ported arms (C++-exact for all modeled arms):
-- boss_tyrannus JustEngagedWith — Talk(SAY_AGGRO 8) (event 1 — the auriaya
-- engage-port precedent; the AttackStart NON_ATTACKABLE gate,
-- REACT_PASSIVE/AGGRESSIVE legs and Phase flips are unbridged).
-- boss_tyrannus KilledUnit — Talk(SAY_SLAY 9) player-gated (event 3 — the
-- razuvious player-gated variant precedent; the ninth ported variant,
-- seventh identical to nalorakk/kelthuzad/gothik/thaddius/garfrost/krick).
-- boss_tyrannus JustDied — Talk(SAY_DEATH 10) (event 4 — the sjonnir
-- JustDied-Talk precedent). Melee engine-driven.
-- DOCUMENTED-ONLY (in this header; only entry 36658 registered):
-- boss_tyrannus Reset / InitializeAI — REACT_PASSIVE +
-- UNIT_FLAG_NON_ATTACKABLE gate + instance->SetBossState(DATA_TYRANNUS,
-- NOT_STARTED/DONE) — no react-state / instance bridges.
-- boss_tyrannus EnterEvadeMode — instance SetBossState(FAIL) +
-- rimefang->AI()->EnterEvadeMode() + DespawnOrUnsummon — no instance /
-- evade bridges.
-- boss_tyrannus DoAction(ACTION_START_INTRO) — Talk(SAY_TYRANNUS_INTRO_1 6)
-- rides a DoAction-triggered intro machine (no-DoAction bridge by
-- construction; the krick ACTION_OUTRO precedent).
-- boss_tyrannus UpdateAI intro machine — EVENT_INTRO_2
-- Talk(SAY_TYRANNUS_INTRO_3 7) rides a timer leg (no-timer-bridge queue;
-- the boss_toravon precedent); the EVENT_INTRO_3 ExitVehicle / MovePoint
-- leg and the EVENT_COMBAT_START rimefang->AI()->DoAction /
-- DoZoneInCombat / FULL_HEAL / ScheduleEvent legs have no bridges.
-- boss_tyrannus UpdateAI combat machine — EVENT_OVERLORD_BRAND /
-- EVENT_FORCEFUL_SMASH / EVENT_UNHOLY_POWER / EVENT_MARK_OF_RIMEFANG:
-- the Talk(SAY_DARK_MIGHT_1 13) / Talk(SAY_DARK_MIGHT_2 14) /
-- Talk(SAY_MARK_RIMEFANG_1 11) / Talk(SAY_MARK_RIMEFANG_2 12, target) legs
-- ride timer legs (no-timer-bridge); the
-- SelectTarget(SelectTargetMethod::Random, 1, 0.0f, true) legs join the
-- no-random-target-SelectTarget queue; the DoCast / DoZoneInCombat /
-- UNIT_STATE_CASTING / DoMeleeAttackIfReady legs have no bridge.
-- JustDied instance legs — instance->SetBossState(DATA_TYRANNUS, DONE) +
-- TEMPSUMMON_DEAD_DESPAWN + rimefang->AI()->DoAction(ACTION_END_COMBAT) —
-- no instance / no-DoAction bridges.
-- boss_rimefang (all legs): Reset flight/react/non-attackable gate,
-- JustReachedHome InstallAllAccessories, DoAction(ACTION_START_RIMEFANG)
-- combat-start + ACTION_END_COMBAT evade, SetGUID(GUID_HOARFROST) hoarfrost
-- handoff, UpdateAI EVENT_MOVE_NEXT waypoint patrol / EVENT_ICY_BLAST
-- random-target cast / EVENT_HOARFROST targeted cast — no bridges
-- (no-timer-bridge / no-random-target-SelectTarget / no-motion /
-- no-DoAction / no-vehicle queues); zero Talk lines, so 36661 is NOT
-- registered.
-- player_overlord_brandAI + the three spell scripts
-- (spell_tyrannus_overlord_brand, spell_tyrannus_mark_of_rimefang,
-- spell_tyrannus_rimefang_icy_blast) join the no-SpellScript /
-- no-AuraScript-bridge queues (the boss_moragg optic-link precedent);
-- the AuraScript OnApply player_overlord_brandAI push leg and the
-- icy-blast NPC_ICY_BLAST summon leg have no bridge by construction.
-- at_tyrannus_event_starter joins the unbridged-area-trigger queue
-- (the pit_of_saron cavern triggers precedent).

local ENTRY_TYRANNUS = 36658

local SAY_AGGRO = 8
local SAY_SLAY = 9
local SAY_DEATH = 10

-- C++ boss_tyrannusAI::JustEngagedWith: Talk(SAY_AGGRO) — the auriaya
-- engage-port precedent.
local function tyrannusJustEngagedWith(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ boss_tyrannusAI::KilledUnit: if (victim->GetTypeId() != TYPEID_PLAYER)
-- return; Talk(SAY_SLAY) — the razuvious player-gated variant precedent.
local function tyrannusKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ boss_tyrannusAI::JustDied: Talk(SAY_DEATH) — the sjonnir
-- JustDied-Talk precedent.
local function tyrannusJustDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_TYRANNUS, 1, tyrannusJustEngagedWith)
RegisterCreatureEvent(ENTRY_TYRANNUS, 3, tyrannusKilledUnit)
RegisterCreatureEvent(ENTRY_TYRANNUS, 4, tyrannusJustDied)
