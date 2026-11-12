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
-- at_hor_intro_start (C++ :2036-2061) and at_hor_waves_restarter
--   (C++ :2062-2096) — no trigger IDs in the C++ file (the DB rows
--   bind them by script name); dispatching on a guessed ID would
--   invent an identifier (gelihast precedent). Their legs are
--   documented in the instance file (DATA_INTRO_EVENT store,
--   SPAWN_WAVES machine); the intro chain's fire site is the
--   LK_9 leg in halls_of_reflection_intro.lua and the waves leg is
--   the HOR_SETDATA_SPAWN_WAVES signal.
-- spell_hor_evasion (70190, C++ :2785-2822) — OnDestinationTargetSelect
--   has no SpellEvent twin (runtime.go:506-601 lists only cast/hit/
--   effect/area-select hooks); stays out.
-- spell_hor_quel_delars_will (70698) — PORTED in
--   npc_uther_quel_delar.lua via RegisterSpellEvent(70698, 5) +
--   spell:GetTriggerSpell (DBC-side trigger id, 0 without the row).

local TRIGGER_SHADOW_THRONE = 5605 -- halls_of_reflection.cpp:2117
local TRIGGER_UTHER_QUEL_DELAR_START = 5660 -- halls_of_reflection.cpp:2734

local DATA_THE_LICH_KING_ESCAPE = 2 -- halls_of_reflection.h:39
local DATA_QUEL_DELAR_EVENT = 14 -- halls_of_reflection.h:55

local BOSS_STATE_NOT_STARTED = 0 -- InstanceScript.h:68-76
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_TO_BE_DECIDED = 5 -- Go unset-boss read (lua_creature_events.go:6434-6437)

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

RegisterServerEvent(24, function(event, player, triggerId)
    if triggerId == TRIGGER_SHADOW_THRONE then
        onShadowThrone(player)
    elseif triggerId == TRIGGER_UTHER_QUEL_DELAR_START then
        onUtherQuelDelarStart(player)
    end
end)
