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
-- Eluna creature events: 2 OnLeaveCombat, 3 OnTargetDied,
-- 9 OnDamageTaken. The handler returns the
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
-- EnterEvadeMode (:1305-1316) — the _despawn once-latch (GUID-keyed, the
--   svalna captain precedent), DoPlaySoundToSet(SOUND_LK_FURY_OF_FROSTMOURNE
--   17224) = creature:PlayDirectSound (ScriptedAI::DoPlaySoundToSet calls
--   source->PlayDirectSound — ScriptedCreature.cpp:188-200, broadcast-to-set
--   SMSG_PLAY_SOUND, the engine/world/lua_objects.go PlayDirectSound bridge),
--   and DoCastAOE(SPELL_FURY_OF_FROSTMOURNE 70063) = self-cast (the file's
--   DoCastAOE idiom), in C++ order. StopMoving + the 12s delayed Despawn are
--   PORTED below; only the instance SetBossState(FAIL) leg has no bridge.
-- KilledUnit (:1251-1255) — player-gated
--   (victim:GetObjectType() == "Player", the razuvious precedent)
--   DoPlaySoundToSet(RAND(SOUND_LK_SLAY_1 17214, SOUND_LK_SLAY_2 17215)) =
--   creature:PlayDirectSound(math.random(SOUND_LK_SLAY_1, SOUND_LK_SLAY_2)).
--   The old "no sound bridge" note was wrong: the bridge is PlayDirectSound.
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
-- EnterEvadeMode (:1305-1316) — the _despawn latch, Fury-of-Frostmourne
--   sound, cast, StopMoving and 12s delayed Despawn are ported above;
--   only the instance SetBossState(FAIL) leg has no bridge.
-- JustSummoned / SummonedCreatureDies (:1233-1250) — summon bookkeeping
--   with no summon-list bridge.

local ENTRY_LICH_KING_ESCAPE = 36954

local SPELL_FURY_OF_FROSTMOURNE = 70063

local SOUND_LK_FURY_OF_FROSTMOURNE = 17224
local SOUND_LK_SLAY_1 = 17214
local SOUND_LK_SLAY_2 = 17215

-- C++ _despawn latch (member, ctor-false, set on first evade-fire, never
-- reset): GUID-keyed, never cleared on 2/23 — a respawn mints a fresh GUID,
-- matching the ctor (the svalna captain precedent).
local evadeFired = {}

-- C++ DamageTaken :1198-1202 — the clamp keeps the Lich King alive
-- through the escape; return the replacement damage as the second
-- return per the fireCreatureDamageTaken convention.
local function lichKingEscapeDamageTaken(event, creature, attacker, damage)
    local health = creature:GetHealth()
    if damage >= health then
        return true, health - 1
    end
end

-- C++ npc_the_lich_king_escape_horAI::EnterEvadeMode (:1305-1316),
-- C++ order: instance SetBossState(FAIL) [no bridge] -> StopMoving ->
-- DoPlaySoundToSet(SOUND_LK_FURY_OF_FROSTMOURNE 17224) — PlayDirectSound
-- (the DoPlaySoundToSet ≡ PlayDirectSound identity above) -> DoCastAOE
-- (SPELL_FURY_OF_FROSTMOURNE 70063) non-triggered — self-cast (the file's
-- DoCastAOE idiom) -> DespawnOrUnsummon(12s), modeled as a 12s
-- CreateLuaEvent one-shot calling creature:Despawn (immediate bridge).
-- All ungated apart from the _despawn once-latch. Only the instance
-- SetBossState(FAIL) leg has no bridge.
local function lichKingEscapeLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    if evadeFired[guid] then
        return
    end
    evadeFired[guid] = true
    creature:StopMoving()
    creature:PlayDirectSound(SOUND_LK_FURY_OF_FROSTMOURNE)
    creature:CastSpell(creature, SPELL_FURY_OF_FROSTMOURNE)
    CreateLuaEvent(function()
        creature:Despawn()
    end, 12000)
end

-- C++ npc_the_lich_king_escape_horAI::KilledUnit (:1251-1255):
-- player-gated DoPlaySoundToSet(RAND(SOUND_LK_SLAY_1 17214,
-- SOUND_LK_SLAY_2 17215)) — PlayDirectSound; math.random(17214, 17215)
-- matches RAND's two-sound pick. Event 3 is CREATURE_EVENT_ON_TARGET_DIED,
-- live in Go (fireCreatureTargetDied in engine/world/lua_creature_events.go).
local function lichKingEscapeTargetDied(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:PlayDirectSound(math.random(SOUND_LK_SLAY_1, SOUND_LK_SLAY_2))
    end
end

RegisterCreatureEvent(ENTRY_LICH_KING_ESCAPE, 2, lichKingEscapeLeaveCombat)
RegisterCreatureEvent(ENTRY_LICH_KING_ESCAPE, 3, lichKingEscapeTargetDied)
RegisterCreatureEvent(ENTRY_LICH_KING_ESCAPE, 9, lichKingEscapeDamageTaken)
