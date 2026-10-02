-- Blackheart the Inciter (Shadow Labyrinth, Auchindoun) — Lua port of
-- src/server/scripts/Outland/Auchindoun/ShadowLabyrinth/boss_blackheart_the_inciter.cpp
-- (1 CreatureScript — boss_blackheart_the_inciter (BossAI via
-- GetShadowLabyrinthAI, DATA_BLACKHEART_THE_INCITER = 1); registered
-- from inside AddSC_boss_blackheart_the_inciter(); loader decl 37 /
-- call 160 per outland_script_loader.cpp — the SECOND group of the
-- "// Auchindoun - Shadow Labyrinth" sub-block in AddOutlandScripts(),
-- immediately after AddSC_boss_ambassador_hellmaw() (call 159) —
-- verified from the loader this run; the checkpoint sequence
-- (ambassador_hellmaw -> blackheart_the_inciter) is followed).
-- Entry: 18667 NPC_BLACKHEART — from shadow_labyrinth.h's Creatures
-- enum (line 44); the .cpp file itself carries zero NPC_ constants
-- (the boss entry is verifiable from the C++ tree —
-- NPC_ANZU-in-sethekk_halls.h / ambassador_hellmaw precedents).
-- ScriptName bindings are DB-side as usual. Sole-source verified:
-- whole-server-tree grep for "AddSC_boss_blackheart_the_inciter"
-- hits boss_blackheart_the_inciter.cpp (+ the loader decl/call
-- lines) only. No blackheart_the_inciter lua existed.
-- Eluna creature events: 1 OnEnterCombat, 3 OnTargetDied, 4 OnDied.
-- Ported arms (C++-exact for all modeled arms):
-- boss_blackheart_the_inciter::JustEngagedWith — Talk(SAY_AGGRO 1)
-- (line 92; event 1 — the auriaya precedent; the ScheduleEvent legs
-- ride the no-timer bridge and are documented-only below).
-- boss_blackheart_the_inciter::KilledUnit — Talk(SAY_SLAY 2) (line
-- 98) gated on who->GetTypeId() == TYPEID_PLAYER — the razuvious
-- player-gated precedent (event 3).
-- boss_blackheart_the_inciter::JustDied — Talk(SAY_DEATH 4) (line
-- 104) — the sjonnir self-Talk precedent (event 4; the _JustDied()
-- instance leg rides the no-instance bridge — documented-only).
-- DOCUMENTED-ONLY (in this header; only entry 18667 registered):
-- boss_blackheart_the_inciter::Reset — me->SetReactState
-- (REACT_AGGRESSIVE) + _Reset() — no-instance / no-react-state
-- bridges — documented-only.
-- boss_blackheart_the_inciter::SetData — charm-count machine (++
-- on charmed, sanity-check EnterEvadeMode(EVADE_REASON_OTHER) when
-- -- on zero, REACT_PASSIVE while any charmed player) driven from
-- BlackheartCharmedPlayerAI::OnCharmed — no charmed-player-actor /
-- no-instance bridges — documented-only.
-- boss_blackheart_the_inciter::UpdateAI — the whole timer machine
-- rides no-timer / no-cast / no-random-target bridges:
-- EVENT_INCITE_CHAOS (threat-list-size > 1 gate → ResetThreatList
-- + DoCast(me, SPELL_INCITE_CHAOS 33676), 40s cycle);
-- EVENT_CHARGE_ATTACK (SelectTarget random → DoCast(target,
-- SPELL_CHARGE 33709), 15-25s cycle); EVENT_WAR_STOMP (DoCast(me,
-- SPELL_WAR_STOMP 33707), 18-24s cycle); DoMeleeAttackIfReady —
-- documented-only.
-- BlackheartCharmedPlayerAI (SimpleCharmedPlayerAI) —
-- OnCharmed: instance GetGuidData(DATA_BLACKHEART_THE_INCITER) →
-- blackheart->AI()->SetData(0, charmed) + AddThreat — no
-- charmed-player / no-instance bridges — documented-only.
-- boss_blackheart_the_inciter_mc_dummy (NullCreatureAI;
-- NPC_BLACKHEART_DUMMY1..5 = 19300-19304, shadow_labyrinth.h lines
-- 45-49) — IsSummonedBy casts SPELL_INCITE_CHAOS_B 33684 on the
-- summoner + dummy-threat juggling across all dummies and their
-- m_Controlled; DespawnOrUnsummon when m_Controlled empties;
-- GetAIForCharmedPlayer returns BlackheartCharmedPlayerAI —
-- zero Talk arms anywhere in the dummy script; no-summon /
-- no-threat / no-cast bridges — documented-only (dummies NOT
-- registered).
-- spell_blackheart_incite_chaos SpellScript — HandleDummy cycles
-- INCITE_SPELLS {33677, 33680, 33681, 33682, 33683} on hit units
-- (EFFECT_0 SPELL_EFFECT_SCRIPT_EFFECT) — no SpellScript bridge
-- (instance_ulduar precedent) — documented-only.
-- All Talk() calls in the file accounted for (3 ported above;
-- SAY_INTRO 0 / SAY_HELP 3 / SAY2_* 5-9 are never called anywhere
-- in the file).

local ENTRY_BLACKHEART_THE_INCITER = 18667 -- NPC_BLACKHEART (shadow_labyrinth.h)

local SAY_AGGRO = 1
local SAY_SLAY  = 2
local SAY_DEATH = 4

-- C++ boss_blackheart_the_inciter::JustEngagedWith:
-- events.ScheduleEvent(EVENT_INCITE_CHAOS, 20s);
-- events.ScheduleEvent(EVENT_CHARGE_ATTACK, 5s);
-- events.ScheduleEvent(EVENT_WAR_STOMP, 15s);
-- Talk(SAY_AGGRO); — the auriaya precedent (event 1; the
-- ScheduleEvent legs ride the no-timer bridge — documented-only).
local function blackheartEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ boss_blackheart_the_inciter::KilledUnit:
-- if (who->GetTypeId() == TYPEID_PLAYER) Talk(SAY_SLAY) — the
-- razuvious player-gated precedent (event 3 — the illidan
-- victim:IsPlayer() convention).
local function blackheartTargetDied(event, creature, victim)
    if victim:IsPlayer() then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ boss_blackheart_the_inciter::JustDied:
-- _JustDied(); Talk(SAY_DEATH); — the sjonnir self-Talk precedent
-- (event 4; the _JustDied() instance leg rides the no-instance
-- bridge — documented-only).
local function blackheartDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_BLACKHEART_THE_INCITER, 1, blackheartEnterCombat)
RegisterCreatureEvent(ENTRY_BLACKHEART_THE_INCITER, 3, blackheartTargetDied)
RegisterCreatureEvent(ENTRY_BLACKHEART_THE_INCITER, 4, blackheartDied)
