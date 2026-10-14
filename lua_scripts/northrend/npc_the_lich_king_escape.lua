-- The Lich King escape (Halls of Reflection) — Lua port of the
-- DamageTaken arm of npc_the_lich_king_escape_hor in
-- src/server/scripts/Northrend/FrozenHalls/HallsOfReflection/halls_of_reflection.cpp
-- (C++ :1183-1390; part of the 25 scripts registered from inside
-- AddSC_halls_of_reflection(); loader decl 167 / call 362 per
-- northrend_script_loader.cpp — the SECOND group of the "// Halls of
-- Reflection" block in AddNorthrendScripts(), immediately after
-- AddSC_instance_halls_of_reflection() (call 361), before
-- AddSC_boss_falric() (call 363)).
-- Entry: 36954 NPC_THE_LICH_KING_ESCAPE (halls_of_reflection.h line 84)
-- — the CreatureScript ScriptName binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_halls_of_reflection" hits halls_of_reflection.cpp only (+ the
-- loader decl/call lines).
-- Eluna creature events: 9 OnDamageTaken. The handler returns the
-- rewritten damage as the second return (the
-- engine/world/lua_creature_events.go fireCreatureDamageTaken
-- convention); the boolean first return only gates the empty
-- ScriptedAI::DamageTaken base, a provable no-op.
--
-- PORTED ARMS (C++ line refs; C++-exact for the modeled arm):
-- DamageTaken (:1198-1202) — ungated clamp: if damage >= current
--   health, damage = health - 1 (the Lich King cannot be killed during
--   the escape chase). Event 9 fires pre-application, so
--   creature:GetHealth() is the pre-damage value, exactly like C++'s
--   me->GetHealth() in the hook.
--
-- DOCUMENTED-ONLY:
-- The whole SetData(DATA_ICEWALL) event machine (:1215-1298) —
--   EVENT_REMORSELESS_WINTER (:1330-1336, self-cast 69780 + Talk),
--   EVENT_ESCAPE_SUMMON_GHOULS / _WITCH_DOCTOR /
--   _LUMBERING_ABOMINATION (:1337-1358, summon-spell casts + Talk) —
--   rides instance SetData legs with no instance bridge, and the
--   summon legs have no summon bridge; the FURY_OF_FROSTMOURNE AoE
--   (:1311-1314) rides the same machine.
-- MovementInform (:1204-1231) — MoveChase / Harvest Soul on the
--   instance-GUID escape leader + gunship movement: no-MovementInform
--   / no-instance-GetGuidData bridges.
-- KilledUnit (:1252-1257) — DoPlaySoundToSet(SOUND_LK_SLAY_1/2): no
--   sound bridge.
-- EnterEvadeMode (:1305-1312) — instance SetBossState(FAIL) leg: no
--   instance bridge.
-- JustSummoned / SummonedCreatureDies (:1233-1250) — summon bookkeeping
--   with no summon-list bridge.

local ENTRY_LICH_KING_ESCAPE = 36954

-- C++ DamageTaken :1198-1202 — the clamp keeps the Lich King alive
-- through the escape; return the replacement damage as the second
-- return per the fireCreatureDamageTaken convention.
local function lichKingEscapeDamageTaken(event, creature, attacker, damage)
    local health = creature:GetHealth()
    if damage >= health then
        return true, health - 1
    end
end

RegisterCreatureEvent(ENTRY_LICH_KING_ESCAPE, 9, lichKingEscapeDamageTaken)
