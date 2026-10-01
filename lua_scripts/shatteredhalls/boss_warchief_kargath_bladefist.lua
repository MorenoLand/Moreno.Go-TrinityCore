-- Warchief Kargath Bladefist (Shattered Halls, Hellfire
-- Citadel) — Lua port of src/server/scripts/Outland/
-- HellfireCitadel/ShatteredHalls/boss_warchief_kargath_
-- bladefist.cpp (boss_warchief_kargath_bladefist — the only
-- creature AI class AddSC registers; GetShatteredHallsAI is
-- an AI-factory helper, not a script).
-- Final boss in the Shattered Halls set per
-- outland_script_loader.cpp order (instance_shattered_halls.
-- cpp stays blocked on the instance-script model).
-- Entry: NPC_KARGATH_BLADEFIST = 16808 from shattered_halls.
-- h (SHCreatureIds), mapped in instance_shattered_halls.cpp
-- OnCreatureCreate (line 111) plus GetGuidData (line 170).
-- The creature_template ScriptName bindings are DB-side (no
-- TDB in this workspace).
-- Talk: SAY_AGGRO = 0 fires from the C++ JustEngagedWith
-- (C++-exact); SAY_SLAY = 1 fires from the C++ KilledUnit
-- override gated on the victim being a player (C++-exact);
-- SAY_DEATH = 2 fires from the C++ JustDied (the _JustDied
-- arm is instance-blocked); SAY_CALL_EXECUTIONER_A = 3 /
-- SAY_CALL_EXECUTIONER_H = 4 fire only from the DoAction
-- (ACTION_EXECUTIONER_TAUNT = 1, shattered_halls.h) arm,
-- which reads instance->GetData(DATA_TEAM_IN_INSTANCE) —
-- instance gated, unreachable in this model.
-- Eluna creature events: 1 OnEnterCombat, 3 OnTargetDied,
-- 4 OnDied. No scheduler pump: every
-- combat timer's fire arm is blocked on a missing bridge (see
-- below), so nothing observable remains to tick; melee is
-- engine-driven in Go (creature combat tick), like C++.
-- Fight shape (C++-exact for all modeled arms): Kargath
-- (16808): OnEnterCombat(1) — Talk(SAY_AGGRO); the C++
-- JustEngagedWith override does nothing else, and the
-- Initialize() schedule (Blade_Dance_Timer 45000 /
-- Summon_Assistant_Timer 30000 / Assassins_Timer 5000 /
-- resetcheck_timer 5000 / summoned 2) drives only
-- bridge-blocked arms, so no per-GUID state is kept.
-- OnTargetDied(3): Talk(SAY_SLAY) gated on the killed unit
-- being a player (C++ `victim->GetTypeId() == TYPEID_PLAYER`
-- via the terestian/gurtogg 3-arg handler, C++-exact).
-- OnDied(4): Talk(SAY_DEATH) (the _JustDied arm is
-- instance-blocked; the removeAdds arm is summon/
-- ObjectAccessor-blocked — with no summon bridge the GUID
-- lists are always empty). No events 2/23: the C++ Reset/
-- leave-combat arms have no observable remainder in this
-- model — removeAdds is summon/ObjectAccessor-blocked with
-- always-empty GUID lists, _Reset is instance-blocked, the
-- speed-rate arms have no speed bridge, and Initialize() has
-- no observable remainder without a scheduler pump (omrogg
-- "no event 3" precedent). Melee is
-- engine-driven.
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI
-- shim (instance_shattered_halls.cpp stays blocked on the
-- instance-script model; the BossAI ctor DATA_KARGATH = 2
-- bookkeeping in Reset/JustEngagedWith/JustDied skipped —
-- DATA_KARGATH is an instance-side constant, unbridgeable);
-- no summon bridge — the Assassins_Timer 5s arm (four
-- NPC_SHATTERED_ASSASSIN 17695 summons at AssassEntrance/
-- AssassExit with TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT)
-- never fires, and the Summon_Assistant_Timer {25s,35s} arm
-- (summoned NPC_HEARTHEN_GUARD 17621 / NPC_SHARPSHOOTER_
-- GUARD 17622 / NPC_REAVER_GUARD 17623 at AddsEntrance, with
-- the summoned-counter growth arm) never fires
-- (steamrigger precedent); no movement bridge — the whole
-- blade-dance machine unmodeled: Blade_Dance_Timer 45s init
-- then 30s sets target_num = 5 / Wait_Timer = 1 / InBlade =
-- true / SetSpeedRate(MOVE_RUN, 4), the Wait_Timer arm drives
-- MovePoint(1) hops to random points around (210, -60) and
-- MovementInform(POINT_MOTION_TYPE, 1) fires the triggered
-- DoCast(me, SPELL_BLADE_DANCE 30739, true) with target_num--
-- until target_num hits 0, then InBlade = false /
-- SetSpeedRate(MOVE_RUN, 2) / MoveChase(victim) /
-- Blade_Dance_Timer = 30000 — none of it can run without a
-- movement bridge, and the speed-rate arms have no speed
-- bridge; no difficulty bridge — the IsHeroic()-gated
-- Charge_timer = 5000 arm at blade-dance end, and its
-- DoCast(SelectTarget(Random, 0), H_SPELL_CHARGE 25821) fire
-- arm, unmodeled (thespia precedent); no cross-creature/
-- ObjectAccessor bridge — the JustSummoned AttackStart relay
-- for guard adds and the removeAdds DespawnOrUnsummon sweeps
-- unmodeled; no position/home bridge — the resetcheck_timer
-- 5s evade arm (EnterEvadeMode when x is outside [205, 255])
-- unmodeled (bookkeeping exact: 5s init then 5s); no
-- unit-state bridge — the C++ UpdateAI skips
-- DoMeleeAttackIfReady entirely while InBlade (no melee
-- during blade dance) and there is no bridge to suppress
-- engine melee; the ScriptData "SD%Complete: 90" is an
-- upstream caveat, documented, not bridged; no SpellScript/
-- AuraScript scripts in this file.

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2

local ENTRY_KARGATH_BLADEFIST = 16808

RegisterCreatureEvent(ENTRY_KARGATH_BLADEFIST, 1, function(_, creature)
    creature:Talk(SAY_AGGRO)
end)

RegisterCreatureEvent(ENTRY_KARGATH_BLADEFIST, 3, function(_, creature, victim)
    -- C++ `if (victim->GetTypeId() == TYPEID_PLAYER)
    -- Talk(SAY_SLAY)` — terestian/gurtogg 3-arg-handler
    -- convention (C++-exact).
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end)

RegisterCreatureEvent(ENTRY_KARGATH_BLADEFIST, 4, function(_, creature)
    creature:Talk(SAY_DEATH)
end)
