-- Halls of Reflection area triggers — Lua port of the AreaTriggerScript
-- classes in
-- src/server/scripts/Northrend/FrozenHalls/HallsOfReflection/halls_of_reflection.cpp
-- (AddSC_halls_of_reflection(); loader decl 167 / call 362 per
-- northrend_script_loader.cpp).
-- Fire site: engine/world/areatrigger.go:64 (TriggerServerEvent ctx,
-- scripting.ServerEventAreaTrigger (=24), luaPlayer, triggerID);
-- handlers registered via RegisterServerEvent(24, fn) receive
-- (event, player, triggerID) per engine/scripting/runtime.go:225-232
-- (the sartura / at_scripts.lua precedent). Trigger IDs are the
-- in-file comments; a class with no in-file ID stays out (the
-- gelihast precedent: IDs living only in the DB areatrigger_scripts
-- rows are never guessed).
--
-- PORTED ARMS (C++ line refs):
-- at_hor_shadow_throne (trigger 5605, C++ :2117-2134) — GM gate
--   ported via player:IsGM() (lua_player.go:1411, security > 0 =
--   Player::IsGameMaster twin); GetBossState(DATA_THE_LICH_KING_ESCAPE 2)
--   == NOT_STARTED -> SetBossState(2, IN_PROGRESS). The read/write go
--   through the first bound creature in the instance (the instance
--   file's worldStateCaller idiom): creature:GetBossState /
--   creature:SetBossState key the shared (map, instance) store
--   (lua_creature_events.go:6410-6448). Go reads TO_BE_DECIDED (5) for
--   a never-set boss where C++ reads NOT_STARTED (0)
--   (lua_creature_events.go:6434-6437), so the leg treats both as
--   "not started"; with no bound creature at all there is no writer
--   and the leg is a no-op (practically unreachable: the intro
--   creature materializes at instance start).
-- at_hor_uther_quel_delar_start (trigger 5660, C++ :2734-2752) — GM
--   gate ported via player:IsGM() (same twin); GetData(DATA_QUEL_DELAR_EVENT 14)
--   == IN_PROGRESS reads the instance file's quelDelarState store
--   (event-8 handler, instance_halls_of_reflection.lua); the Uther
--   fetch reads the bound data.uther (instance OnCreatureCreate bind);
--   uther:DoAction(ACTION_UTHER_START_SCREAM 1) fires the ev38
--   receiver in npc_uther_quel_delar.lua (the START_SCREAM arm,
--   ported this tick). The old "area-trigger sender, no bridge" note
--   in that file is retracted.
-- at_hor_intro_start (trigger 5632, C++ :2036-2061) — GM gate
--   ported via player:IsGM(); GetData(DATA_INTRO_EVENT 3) ==
--   NOT_STARTED -> SetData(3, IN_PROGRESS) via the instance
--   file's introState store; the Quel'Delar leg (HasAura 70013
--   + quest 24480/24561 INCOMPLETE + quelDelarState == NOT_STARTED)
--   -> SetData(14, IN_PROGRESS). The SetGuidData(17) invoker
--   write stays out (no bridge).
-- at_hor_waves_restarter (trigger 5697, C++ :2062-2096) — GM gate;
--   waveCount != 0 -> return; intro DONE + Marwyn != DONE ->
--   SetData(HOR_SETDATA_SPAWN_WAVES 1001, 0) (the ProcessEvent
--   twin) + Falric/Marwyn triggered self-cast 72712 +
--   SetVisible(true). Boss state reads through the first bound
--   creature (the worldStateCaller idiom).
--
-- DOCUMENTED-ONLY:
-- at_hor_impenetrable_door (trigger 5740, C++ :2097-2115) — its whole
--   payload is the veto: Marwyn DONE -> return true, else return
--   false ("return false to handle teleport by db"). Go's event-24
--   fire site DISCARDS the handler return (engine/world/
--   areatrigger.go:64, `_, _ = TriggerServerEvent(...)`; "the Lua
--   hook fires but never vetoes", :58-63), so no Lua leg can
--   reproduce the veto — the DB teleport arm would run either way.
--   Stays out until the fire site honors a boolean return.
-- at_hor_intro_start (trigger 5632, C++ :2036-2061) and
-- at_hor_waves_restarter (trigger 5697, C++ :2062-2096) —
-- PORTED 2026-11-15 (at_* census) below. Their IDs live only
-- in the TDB areatrigger_scripts rows (5632 / 5697, verified
-- in TDB_full_world_335.26091_2026_09_09.sql) — the old
-- gelihast stay-out ("IDs living only in the DB are never
-- guessed") is retracted for these two: the IDs are verified
-- rows, not guesses (the tyrannus / ICC TDB-ID ruling). The
-- intro chain's fire site is the LK_9 leg in
-- halls_of_reflection_intro.lua and the waves leg is the
-- HOR_SETDATA_SPAWN_WAVES signal; the SetGuidData
-- (DATA_QUEL_DELAR_INVOKER 17) leg of the intro arm stays out
-- (no instance SetGuidData bridge).
-- spell_hor_evasion (70190, C++ :2785-2822) — OnDestinationTargetSelect
--   has no SpellEvent twin (runtime.go:506-601 lists only cast/hit/
--   effect/area-select hooks); stays out.
-- spell_hor_quel_delars_will (70698) — PORTED in
--   npc_uther_quel_delar.lua via RegisterSpellEvent(70698, 5) +
--   spell:GetTriggerSpell (DBC-side trigger id, 0 without the row).

local TRIGGER_SHADOW_THRONE = 5605 -- halls_of_reflection.cpp:2117
local TRIGGER_UTHER_QUEL_DELAR_START = 5660 -- halls_of_reflection.cpp:2734
local TRIGGER_INTRO_START = 5632 -- TDB areatrigger_scripts row
local TRIGGER_WAVES_RESTARTER = 5697 -- TDB areatrigger_scripts row

local DATA_THE_LICH_KING_ESCAPE = 2 -- halls_of_reflection.h:39
local DATA_MARWYN = 1 -- halls_of_reflection.h:38
local DATA_INTRO_EVENT = 3 -- halls_of_reflection.h:41
local DATA_WAVE_COUNT = 6 -- halls_of_reflection.h:45
local DATA_QUEL_DELAR_EVENT = 14 -- halls_of_reflection.h:55

local HOR_SETDATA_SPAWN_WAVES = 1001 -- instance_halls_of_reflection.lua:240
local SPELL_QUEL_DELAR_COMPULSION = 70013 -- halls_of_reflection.h:176
local SPELL_BOSS_SPAWN_AURA = 72712 -- halls_of_reflection.cpp:225
local QUEST_HALLS_OF_REFLECTION_ALLIANCE = 24480 -- halls_of_reflection.h:182
local QUEST_HALLS_OF_REFLECTION_HORDE = 24561 -- halls_of_reflection.h:183

local BOSS_STATE_NOT_STARTED = 0 -- InstanceScript.h:68-76
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_DONE = 3
local BOSS_STATE_TO_BE_DECIDED = 5 -- Go unset-boss read (lua_creature_events.go:6434-6437)
local QUEST_STATUS_INCOMPLETE = 3 -- lua_player.go GetQuestStatus

local ACTION_UTHER_START_SCREAM = 1 -- halls_of_reflection.cpp:2388

-- First bound creature wins (the instance file's worldStateCaller
-- idiom): any creature in the instance reads/writes the shared
-- boss-state store.
local function firstBoundCreature(data)
    return data.falric or data.marwyn or data.jainaOrSylvanasIntro
        or data.jainaOrSylvanasEscape or data.lichkingIntro
        or data.lichkingEscape or data.uther
end

-- C++ :2117-2134 at_hor_shadow_throne.
local function onShadowThrone(player)
    if player:IsGM() then
        return
    end
    local mapObj = player:GetMap()
    if not mapObj then
        return
    end
    local data = mapObj:GetInstanceData()
    if not data then
        return
    end
    local caller = firstBoundCreature(data)
    if not caller then
        return
    end
    local state = caller:GetBossState(DATA_THE_LICH_KING_ESCAPE)
    if state == BOSS_STATE_NOT_STARTED or state == BOSS_STATE_TO_BE_DECIDED then
        caller:SetBossState(DATA_THE_LICH_KING_ESCAPE, BOSS_STATE_IN_PROGRESS)
    end
end

-- C++ :2734-2752 at_hor_uther_quel_delar_start.
local function onUtherQuelDelarStart(player)
    if player:IsGM() then
        return
    end
    local mapObj = player:GetMap()
    if not mapObj then
        return
    end
    local data = mapObj:GetInstanceData()
    if not data then
        return
    end
    if (data.quelDelarState or 0) ~= BOSS_STATE_IN_PROGRESS then
        return
    end
    local uther = data.uther
    if not uther then
        return
    end
    uther:DoAction(ACTION_UTHER_START_SCREAM)
end

-- C++ :2036-2061 at_hor_intro_start. The SetGuidData
-- (DATA_QUEL_DELAR_INVOKER 17, player GUID) leg stays out —
-- no instance SetGuidData bridge; its only C++ reader is the
-- Uther completion leg (halls_of_reflection.cpp:2531).
local function onIntroStart(player)
    if player:IsGM() then
        return
    end
    local mapObj = player:GetMap()
    if not mapObj then
        return
    end
    local data = mapObj:GetInstanceData()
    if not data then
        return
    end
    if (data.introState or 0) == BOSS_STATE_NOT_STARTED then
        data:SetData(DATA_INTRO_EVENT, BOSS_STATE_IN_PROGRESS)
    end
    if player:HasAura(SPELL_QUEL_DELAR_COMPULSION)
        and (player:GetQuestStatus(QUEST_HALLS_OF_REFLECTION_ALLIANCE) == QUEST_STATUS_INCOMPLETE
            or player:GetQuestStatus(QUEST_HALLS_OF_REFLECTION_HORDE) == QUEST_STATUS_INCOMPLETE)
        and (data.quelDelarState or 0) == BOSS_STATE_NOT_STARTED then
        data:SetData(DATA_QUEL_DELAR_EVENT, BOSS_STATE_IN_PROGRESS)
    end
end

-- C++ :2062-2096 at_hor_waves_restarter.
local function onWavesRestarter(player)
    if player:IsGM() then
        return
    end
    local mapObj = player:GetMap()
    if not mapObj then
        return
    end
    local data = mapObj:GetInstanceData()
    if not data then
        return
    end
    if (data.waveCount or 0) ~= 0 then
        return
    end
    local caller = firstBoundCreature(data)
    if not caller then
        return
    end
    if (data.introState or 0) == BOSS_STATE_DONE
        and caller:GetBossState(DATA_MARWYN) ~= BOSS_STATE_DONE then
        data:SetData(HOR_SETDATA_SPAWN_WAVES, 0)
        local falric = data.falric
        if falric then
            falric:CastSpell(falric, SPELL_BOSS_SPAWN_AURA, true)
            falric:SetVisible(true)
        end
        local marwyn = data.marwyn
        if marwyn then
            marwyn:CastSpell(marwyn, SPELL_BOSS_SPAWN_AURA, true)
            marwyn:SetVisible(true)
        end
    end
end

RegisterServerEvent(24, function(event, player, triggerId)
    if triggerId == TRIGGER_SHADOW_THRONE then
        onShadowThrone(player)
    elseif triggerId == TRIGGER_UTHER_QUEL_DELAR_START then
        onUtherQuelDelarStart(player)
    elseif triggerId == TRIGGER_INTRO_START then
        onIntroStart(player)
    elseif triggerId == TRIGGER_WAVES_RESTARTER then
        onWavesRestarter(player)
    end
end)
