-- Silithus: Anachronos Quest Trigger — Lua port of the Reset arm of
-- src/server/scripts/Kalimdor/zone_silithus.cpp
-- (class npc_anachronos_quest_trigger : CreatureScript("npc_anachronos_quest_trigger");
-- struct npc_anachronos_quest_triggerAI : public ScriptedAI).
-- Entry: 15454 (TDB 335.26091 creature_template ScriptName
-- npc_anachronos_quest_trigger; one world spawn, creature guid 42922,
-- map 1 at (-8064.17, 1534.23, 2.69) — the Scepter event site).
-- Eluna creature events: 23 OnReset.
-- Ported arms (zone_silithus.cpp:831-835):
-- - Reset: SetVisible(false). The Initialize() half zeroes only
--   SummonNextWave-machine state (WaveTimer/AnnounceTimer/LiveCount/
--   WaveCount/EventStarted/Announced/Failed + PlayerGUID — unported,
--   see below), so the hide is the whole observable. The spawn row
--   carries modelid 13069 with unit_flags 0 / dynamicflags 0, so
--   without this the trigger spawns visible in Go; C++ hides it on
--   spawn. ev23 fires on spawn (lua_creature_events.go
--   fireCreatureSpawned pre-fire), so this matches C++ spawn
--   behavior. SetVisible bridge: lua_creature_events.go:6621
--   (boss_midnight.lua live precedent).
-- Unported scope: the whole Scepter of the Shifting Sands event
-- machine (zone_silithus.cpp:836-1020: OnQuestAccept start,
-- SummonNextWave waves, HandleAnimation 20-step cinematic) — no
-- bridge for the quest-accept start; the trigger stands hidden
-- until that machine lands.
-- Scanner note: zone_silithus.cpp ev23 is a mixed row — the
-- qiraj_war_spawn Reset (:698-701) is ported in
-- npc_qiraj_war_spawn.lua, and Anachronos himself (15381, :362-366)
-- stays out: zero world spawns, summoned only by the unported
-- machine (:984), so his Reset can never fire in the ported model.

local ENTRY_QUEST_TRIGGER = 15454

-- C++ npc_anachronos_quest_triggerAI::Reset (zone_silithus.cpp:831-835).
local function questTriggerReset(_, creature)
    creature:SetVisible(false)
end

RegisterCreatureEvent(ENTRY_QUEST_TRIGGER, 23, questTriggerReset)
