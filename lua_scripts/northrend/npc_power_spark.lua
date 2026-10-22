-- npc_power_spark (Eye of Eternity) — Lua port of
-- src/server/scripts/Northrend/Nexus/EyeOfEternity/boss_malygos.cpp
-- (npc_power_spark (CreatureScript) via
-- GetEyeOfEternityAI<npc_power_sparkAI>, registered from inside
-- AddSC_boss_malygos()).
-- Entry: 30084 NPC_POWER_SPARK (eye_of_eternity.h :52); the C++
-- class decl is boss_malygos.cpp :1065.
-- Eluna creature events: 4 OnDied — args (event, creature, killer).
-- Ported arms (C++-exact):
-- JustDied: me->CastSpell(me, SPELL_POWER_SPARK_DEATH, true)
-- (boss_malygos.cpp :1111-:1114) — triggered self-cast of 55852,
-- no phase gate and no instance access in the C++ arm, so the
-- faithful port is event 4 + creature:CastSpell(creature, spell,
-- true) with the exact triggered flag.
-- Unmodeled (no bridges — documented, not wired):
-- constructor Talk(EMOTE_POWER_SPARK_SUMMONED 0) +
-- MoveToMalygos (MoveIdle / MoveFollow Malygos) — no spawn /
-- motion bridges; UpdateAI: phase-one/vortex despawn gates
-- (instance GetGuidData DATA_MALYGOS + GetData DATA_PHASE /
-- GetBossState FAIL gates) and CHASE_MOTION_TYPE MoveFollow
-- maintenance — no per-tick UpdateAI / motion / instance
-- bridges.

local ENTRY_POWER_SPARK = 30084

local SPELL_POWER_SPARK_DEATH = 55852

-- C++ npc_power_sparkAI::JustDied (boss_malygos.cpp :1111-:1114):
-- me->CastSpell(me, SPELL_POWER_SPARK_DEATH, true);
local function powerSparkDied(event, creature, killer)
    creature:CastSpell(creature, SPELL_POWER_SPARK_DEATH, true)
end

RegisterCreatureEvent(ENTRY_POWER_SPARK, 4, powerSparkDied)
RegisterCreatureEvent(ENTRY_POWER_SPARK, 1, function(_, creature)
    creature:SetMeleeAttack(false) -- boss_malygos.cpp:1087: custom UpdateAI never calls DoMeleeAttackIfReady (movement-only UpdateAI in C++, never melees)
end)
