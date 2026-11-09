-- The Drak'Tharon Keep instance script — Lua port of
-- src/server/scripts/Northrend/DraktharonKeep/instance_drak_tharon_keep.cpp
-- (class instance_drak_tharon_keep : public InstanceMapScript, map 600).
-- RegisterMapEvent binds the map-level Eluna instance hook (the
-- instance_ulduar.lua precedent).
-- PORTED (tick 466) — OnUnitDeath (instance_drak_tharon_keep.cpp:149-153):
-- a dead NPC_CRYSTAL_HANDLER (26627) forwards DoAction(
-- ACTION_CRYSTAL_HANDLER_DIED) to Novos (drak_tharon_keep.h:52 = 15,
-- counted from DATA_TROLLGORE = 0). The Novos object rides the
-- ON_CREATURE_CREATE binding (the instance_ulduar.lua voice/sara/brain
-- precedent); the fire itself rides the Go-only INSTANCE_EVENT_ON_
-- UNIT_DEATH (9, engine/scripting/runtime.go — the C++ ZoneScript::
-- OnUnitDeath has no Eluna hook, fired from Unit::setDeathState,
-- Unit.cpp:8956, ahead of the JustDied hooks).
-- PORTED (tick 510) — OnCreatureCreate (instance_drak_tharon_keep.cpp:38-63):
-- the 4 boss latches + the position-classified summoner tables
-- (InitializeTrollgoreInvaderSummoner :97-113, InitializeNovosSummoner
-- :115-132), stored as live creature objects on the shared instanceData
-- table (the instance_gundrak.lua dweller-set precedent). Consumers:
-- trollgore EVENT_SPAWN (boss_trollgore.lua) reads
-- instanceData.trollgoreInvaderSummoners; novos SetCrystalStatus beam
-- legs (boss_novos.lua) read instanceData.novosSummoners. The C++
-- stores ObjectGuids and resolves them via ObjectAccessor::GetCreature
-- — the Lua twin stores the objects directly (no GUID->creature bridge
-- is needed, and the GUID-key identity hazard of tick 508 never arises).
-- The DATA_NOVOS_SUMMONER_4 box (instance_drak_tharon_keep.cpp:129)
-- reads `y > -727.0f && y < 721.0f` — ported verbatim; the 721.0f upper
-- bound is almost certainly an upstream typo for -721.0f, but the C++
-- text is the authority and the box still classifies the summoner.
-- PORTED (tick 510) — OnGameObjectCreate (instance_drak_tharon_keep.cpp:65-83):
-- the 4 Novos crystal GO latches. Dynamic-only in Go (instance_hooks.go:
-- triggerInstanceGameObjectCreate fires from dynamic spawns; static DB
-- GOs never fire event 6) — the crystals are static DB spawns, so the
-- live resolution path is boss-side: novos resolves them by entry via
-- FindNearestGameObject (boss_novos.lua). The handler below still
-- latches any dynamic crystal that appears.
-- DOCUMENTED-ONLY: player deaths also fire INSTANCE_EVENT_ON_UNIT_DEATH
-- (9) now — the Go player-death sender (killPlayer /
-- completeSpiritOfRedemptionDeath, tick 467) — but no DTK leg reads
-- player deaths, so the handler below returns early for them (players
-- carry no GetEntry method; the guard must run before the entry read).
-- GetGuidData (instance_drak_tharon_keep.cpp:134-160) has no Lua twin:
-- consumers read the latched objects straight off instanceData
-- (trollgore/novos), and BossAI SetBossState bookkeeping rides the
-- creature:SetBossState bridge, not instance GUIDs. SetHeaders /
-- SetBossNumber / doorData / boundaries have no bridges (standing).
-- The old "GetGuidData GUID latching ... stay out — no Go instance
-- script model beyond the instanceData table" claim is retired: the
-- instanceData table IS the model, and the latches ride it.

local MAP_DRAK_THARON_KEEP = 600

local ENTRY_TROLLGORE = 26630
local ENTRY_NOVOS = 26631
local ENTRY_KING_DRED = 27483
local ENTRY_THARON_JA = 26632
local ENTRY_CRYSTAL_HANDLER = 26627
local ENTRY_WORLD_TRIGGER = 22515
local ENTRY_CRYSTAL_CHANNEL_TARGET = 26712

local GO_NOVOS_CRYSTAL_1 = 189299
local GO_NOVOS_CRYSTAL_2 = 189300
local GO_NOVOS_CRYSTAL_3 = 189301
local GO_NOVOS_CRYSTAL_4 = 189302

local ACTION_CRYSTAL_HANDLER_DIED = 15

local INSTANCE_EVENT_ON_CREATURE_CREATE = 5
local INSTANCE_EVENT_ON_GAMEOBJECT_CREATE = 6
local INSTANCE_EVENT_ON_UNIT_DEATH = 9

RegisterMapEvent(MAP_DRAK_THARON_KEEP, INSTANCE_EVENT_ON_CREATURE_CREATE, function(event, instanceData, mapObj, creature)
    local entry = creature:GetEntry()
    if entry == ENTRY_NOVOS then
        instanceData.novos = creature
    elseif entry == ENTRY_TROLLGORE then
        instanceData.trollgore = creature
    elseif entry == ENTRY_KING_DRED then
        instanceData.kingDred = creature
    elseif entry == ENTRY_THARON_JA then
        instanceData.tharonJa = creature
    elseif entry == ENTRY_WORLD_TRIGGER then
        -- C++ InitializeTrollgoreInvaderSummoner
        -- (instance_drak_tharon_keep.cpp:97-113): z < 50 returns early;
        -- the y bands pick the summoner slot.
        local y = creature:GetY()
        local z = creature:GetZ()
        if z >= 50.0 then
            local summoners = instanceData.trollgoreInvaderSummoners
            if not summoners then
                summoners = {}
                instanceData.trollgoreInvaderSummoners = summoners
            end
            if y < -650.0 and y > -660.0 then
                summoners[1] = creature
            elseif y < -660.0 and y > -670.0 then
                summoners[2] = creature
            elseif y < -675.0 and y > -685.0 then
                summoners[3] = creature
            end
        end
    elseif entry == ENTRY_CRYSTAL_CHANNEL_TARGET then
        -- C++ InitializeNovosSummoner (instance_drak_tharon_keep.cpp:115-132):
        -- the x/y/z boxes pick the summoner slot.
        local x = creature:GetX()
        local y = creature:GetY()
        local z = creature:GetZ()
        local summoners = instanceData.novosSummoners
        if not summoners then
            summoners = {}
            instanceData.novosSummoners = summoners
        end
        if x < -374.0 and x > -379.0 and y > -820.0 and y < -815.0 and z < 60.0 and z > 58.0 then
            summoners[1] = creature
        elseif x < -379.0 and x > -385.0 and y > -820.0 and y < -815.0 and z < 60.0 and z > 58.0 then
            summoners[2] = creature
        elseif x < -374.0 and x > -385.0 and y > -827.0 and y < -820.0 and z < 60.0 and z > 58.0 then
            summoners[3] = creature
        elseif x < -338.0 and x > -344.0 and y > -727.0 and y < 721.0 and z < 30.0 and z > 26.0 then
            summoners[4] = creature
        end
    end
end)

RegisterMapEvent(MAP_DRAK_THARON_KEEP, INSTANCE_EVENT_ON_GAMEOBJECT_CREATE, function(event, instanceData, mapObj, go)
    local entry = go:GetEntry()
    local crystals = instanceData.novosCrystals
    if not crystals then
        crystals = {}
        instanceData.novosCrystals = crystals
    end
    if entry == GO_NOVOS_CRYSTAL_1 then
        crystals[1] = go
    elseif entry == GO_NOVOS_CRYSTAL_2 then
        crystals[2] = go
    elseif entry == GO_NOVOS_CRYSTAL_3 then
        crystals[3] = go
    elseif entry == GO_NOVOS_CRYSTAL_4 then
        crystals[4] = go
    end
end)

RegisterMapEvent(MAP_DRAK_THARON_KEEP, INSTANCE_EVENT_ON_UNIT_DEATH, function(event, instanceData, mapObj, unit)
    if unit == nil or unit:GetObjectType() == "Player" or unit:GetEntry() ~= ENTRY_CRYSTAL_HANDLER then
        return
    end
    local novos = instanceData.novos
    if novos ~= nil then
        novos:DoAction(ACTION_CRYSTAL_HANDLER_DIED)
    end
end)
