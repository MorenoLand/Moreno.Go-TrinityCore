-- Violet Hold — Lua port of
-- src/server/scripts/Northrend/VioletHold/violet_hold.cpp
-- (AddSC_violet_hold — loader decl 153 / call 348, the NINTH and LAST
-- group of the "// Violet Hold" block).
-- Ported: the two AuraScripts, npc_sinclari_vh Reset + outro receiver,
-- and the three teleportation-portal AIs. The trash combat arms live in
-- violet_hold_trash.lua, the defense system in
-- npc_violet_hold_defense_system.lua, the activation crystal in
-- npc_violet_hold_activation_crystal.lua, the instance script in
-- instance_violet_hold.lua.
--
-- Convention (boss_emalon.lua / violet_hold_trash.lua): module-local
-- GUID-keyed timers table, wipe on events 4 (died), 23 (reset), 37
-- (removed); CreateLuaEvent takes plain ms or a {minMs, maxMs} table
-- for randtime re-arms; creature:CastSpell(nil, id) = DoCastVictim;
-- creature:CastSpell(creature, id) = DoCast(me)/DoCastAOE self-cast.
-- instanceDataOf (the hyjal_trash.lua precedent) reads the shared
-- per-(map, instance) table behind Map:GetInstanceData; its
-- instanceData:SetData(type, data) writes fire the instance script's
-- OnSetData switch (instance_hooks.go:51, tick 487).
-- DOCUMENTED DEVIATIONS:
-- - The portal AIs' C++ SetData(DATA_PORTAL_PERIODIC_TICK) dispatch is
--   a virtual call that runs the tick leg immediately; Go's
--   creature:SetData is write-only ScriptData transport with no
--   dispatch event, so the normal portal polls its own ScriptData on a
--   1s CreateLuaEvent (the erekem-guard / ichoron-frenzy poll
--   precedent) and runs each missed tick number in order.
-- - DoSummon(entry, me, 2.0f, ...) placement rides SummonCreature
--   with 0,0,0 coords, the GetClosePoint fallback (2x reach in facing
--   direction) — the bridge's documented close-point twin, not the
--   C++ random-angle near point.
-- - Sinclari Reset's _summons.DespawnAll() rides a
--   GetCreaturesInGrid(31011) sweep: intro portals are only ever
--   summoned by Sinclari, so the sweep is the same set.
-- - Go Respawn() is a no-op on a live guard (C++ Respawn(true) resets
--   even live ones); the following EnterEvadeMode() sends live guards
--   home anyway, so the net effect matches.
-- STAY-OUTS (no bridges, re-verified this tick):
-- - npc_sinclari_vh OnGossipHello: the IN_PROGRESS leg needs
--   player->PrepareGossipMenu(me, GOSSIP_MENU_SEND_ME_IN 10275) — no
--   twin (GossipSendMenu takes a title + source object, not a DB menu
--   id; hardcoding the DB option text would fabricate it). DONE
--   returns true with no leg; other states fall through to default
--   gossip, which is the no-handler behaviour.
-- - npc_sinclari_vh OnGossipSelect + ScheduleIntro: menuID is
--   intentionally NOT forwarded (gossip.go:359-363, re-verified tick
--   487) and both options are list 0 in different menus, so a Lua
--   handler cannot tell start-encounter from teleport; the intro legs
--   (MovePoint, guard grid sweep, main-door GO state) are bridged but
--   have no live trigger.
-- - npc_azure_saboteur: StartMovement rides MoveSmoothPath — no Go
--   bridge (whole-tree grep this tick); MovementInform ev28 is
--   bridged but its sender is the unported smooth path.
-- - spell_violet_hold_teleport_player (62138 SpellScript): no
--   SpellScript bridge.
-- - Trash EscortAI waypoint machine (SetData(DATA_PORTAL_LOCATION)
--   paths, CreatureStartAttackDoor): no AddWaypoint bridge — the
--   portal JustSummoned -> summon:SetData(12, location) write IS
--   ported below, so the trash reads its location via GetData(12)
--   once the path machine lands (next unit).
-- - instance_violet_hold AddWave / StartBossEncounter /
--   ScheduleCyanigosaIntro / SpawnPortal: MoveSmoothPath + portal
--   intros; the portal AIs below are live and waiting for AddWave's
--   summons (their _portalLocation reads 0, the C++ ctor default,
--   until SpawnPortal sets it).

local NPC_SINCLARI = 30658
local NPC_VIOLET_HOLD_GUARD = 30659
local NPC_TELEPORTATION_PORTAL = 30679
local NPC_TELEPORTATION_PORTAL_ELITE = 32174
local NPC_TELEPORTATION_PORTAL_INTRO = 31011
local NPC_PORTAL_GUARDIAN = 30660
local NPC_PORTAL_KEEPER = 30695

local SPELL_PORTAL_PERIODIC = 58008
local SPELL_PORTAL_CHANNEL = 58012
local SPELL_DESTROY_DOOR_SEAL = 58040

local DATA_MAIN_EVENT_STATE = 9
local DATA_WAVE_COUNT = 10
local DATA_DOOR_INTEGRITY = 11
local DATA_PORTAL_LOCATION = 12
local DATA_SINCLARI_TRIGGER = 27
local DATA_PORTAL_PERIODIC_TICK = 1

local ENCOUNTER_NOT_STARTED = 0

local ACTION_SINCLARI_OUTRO = 1

local SAY_SINCLARI_OUTRO = 2
local SAY_SINCLARI_ELITE_SQUAD = 0
local SAY_SINCLARI_PORTAL_GUARDIAN = 1
local SAY_SINCLARI_PORTAL_KEEPER = 2

local UNIT_NPC_FLAGS = 82
local UNIT_NPC_FLAG_GOSSIP = 1
local REACT_AGGRESSIVE = 2

local TEMPSUMMON_DEAD_DESPAWN = 6
local TEMPSUMMON_MANUAL_DESPAWN = 7

-- PortalIntroPositions[0..2] (instance_violet_hold.cpp:60-65); [3..4]
-- are the boss-wave portals, summoned by the unported AddWave.
local PORTAL_INTRO_POSITIONS = {
    {1877.51, 850.1042, 44.65989, 4.782202},
    {1890.637, 753.4705, 48.72239, 1.710423},
    {1936.073, 803.1979, 53.37491, 3.124139},
}

-- SinclariPositions[3], the outro spot (violet_hold.cpp:351).
local SINCLARI_OUTRO_POS = {1827.886, 804.0555, 44.36467}

-- Portal tick-2+ trash pool (violet_hold.cpp:765-766).
local PORTAL_TRASH_ENTRIES = {30661, 30961, 30662, 30962, 30664, 30963, 30663, 30918}
-- Elite-portal 15s pool (violet_hold.cpp:806-807).
local ELITE_TRASH_ENTRIES = {30666, 30668, 32191, 30667}
-- Intro-portal trickle pool (violet_hold.cpp:847-848).
local INTRO_TRASH_ENTRIES = {30661, 30664, 30663}

local timers = {}
local portalLastTick = {}
local eliteFired = {}
local eliteSummons = {}
local introSummons = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
    portalLastTick[guid] = nil
    eliteFired[guid] = nil
    eliteSummons[guid] = nil
    introSummons[guid] = nil
end

local function schedule(guid, key, delay, fn)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

-- The hyjal_trash.lua instanceDataOf precedent: the shared
-- per-(map, instance) table behind Map:GetInstanceData, nil outside
-- an instance with a registered map script.
local function instanceDataOf(creature)
    local mapObj = creature:GetMap()
    if not mapObj then
        return nil
    end
    return mapObj:GetInstanceData()
end

local function waveCountOf(creature)
    local data = instanceDataOf(creature)
    if not data then
        return 0
    end
    return data.waveCount or 0
end

-- C++ npc_violet_hold_teleportation_portalAI::SetData tick leg
-- (violet_hold.cpp:737-772). Tick 1 summons a portal guardian or
-- keeper (RAND), channels it, and has the Sinclari trigger Talk;
-- later ticks summon 3 (wave < 12) or 4 trash of the portal pool.
local function portalTick(creature, guid, tick)
    if tick == 1 then
        local entry = NPC_PORTAL_GUARDIAN
        if math.random(0, 1) == 1 then
            entry = NPC_PORTAL_KEEPER
        end
        local keeper = creature:SummonCreature(entry, 0, 0, 0, 0, TEMPSUMMON_DEAD_DESPAWN, 0)
        if keeper then
            creature:CastSpell(keeper, SPELL_PORTAL_CHANNEL)
        end
        local trigger = creature:GetInstanceCreature(DATA_SINCLARI_TRIGGER)
        if trigger then
            if entry == NPC_PORTAL_GUARDIAN then
                trigger:Talk(SAY_SINCLARI_PORTAL_GUARDIAN)
            else
                trigger:Talk(SAY_SINCLARI_PORTAL_KEEPER)
            end
        end
        return
    end
    local k = 3
    if waveCountOf(creature) >= 12 then
        k = 4
    end
    for _ = 1, k do
        creature:SummonCreature(PORTAL_TRASH_ENTRIES[math.random(#PORTAL_TRASH_ENTRIES)], 0, 0, 0, 0, TEMPSUMMON_DEAD_DESPAWN, 20000)
    end
end

-- 1s SetData poll (see header): runs each newly-seen periodic tick
-- in order, then re-arms. Cancelled on ev37 (removed).
local function portalPoll(creature, guid)
    local tick = creature:GetData(DATA_PORTAL_PERIODIC_TICK)
    local last = portalLastTick[guid] or 0
    if tick > last then
        for t = last + 1, tick do
            portalTick(creature, guid, t)
        end
        portalLastTick[guid] = tick
    end
    schedule(guid, "poll", 1000, function() portalPoll(creature, guid) end)
end

-- C++ spell_violet_hold_portal_periodic_AuraScript::PeriodicTick
-- (violet_hold.cpp:1410-1420, aura 58008): PreventDefaultAction;
-- the portal's AI gets SetData(DATA_PORTAL_PERIODIC_TICK 1,
-- tickNumber). creature:SetData is the CreatureAI::SetData
-- virtual-dispatch twin (lua_creature_events.go:767 — the
-- per-creature ScriptData store, each handler interpreting its own
-- types). Returns true, the PreventDefaultAction twin, so the Go
-- default trigger cast does not also fire.
local function portalPeriodicTick(event, aura, target)
    target:SetData(DATA_PORTAL_PERIODIC_TICK, aura:GetTickNumber())
    return true
end

-- C++ spell_violet_hold_destroy_door_seal_AuraScript::PeriodicTick
-- (violet_hold.cpp:1385-1393, aura 58040): PreventDefaultAction;
-- instance GetData(DATA_DOOR_INTEGRITY) - 1 -> SetData, while the
-- integrity is nonzero. The instance write fires the instance
-- script's OnSetData arm (defenseless = 0 + prison-state push);
-- the only writer of the aura is the trash door-attack leg
-- (DoCastAOE 58040, violet_hold_trash.lua path machine — next unit),
-- so this is dormant until that lands.
local function destroyDoorSealTick(event, aura, target)
    local data = instanceDataOf(target)
    if not data then
        return true
    end
    local integrity = data.doorIntegrity or 100
    if integrity > 0 then
        data:SetData(DATA_DOOR_INTEGRITY, integrity - 1)
    end
    return true
end

-- C++ npc_sinclariAI::Reset (violet_hold.cpp:372-392): despawn old
-- intro portals, summon the 3 intro portals + SetData
-- (DATA_PORTAL_LOCATION, i), visible + gossip flag, guard sweep
-- (respawn, visible, aggressive, evade home).
local function sinclariReset(event, creature)
    for _, portal in ipairs(creature:GetCreaturesInGrid(NPC_TELEPORTATION_PORTAL_INTRO, 150)) do
        portal:DespawnOrUnsummon()
    end
    for i = 0, 2 do
        local pos = PORTAL_INTRO_POSITIONS[i + 1]
        local portal = creature:SummonCreature(NPC_TELEPORTATION_PORTAL_INTRO, pos[1], pos[2], pos[3], pos[4], TEMPSUMMON_MANUAL_DESPAWN, 0)
        if portal then
            portal:SetData(DATA_PORTAL_LOCATION, i)
        end
    end
    creature:SetVisible(true)
    creature:SetFlag(UNIT_NPC_FLAGS, UNIT_NPC_FLAG_GOSSIP)
    for _, guard in ipairs(creature:GetCreaturesInGrid(NPC_VIOLET_HOLD_GUARD, 100)) do
        guard:Respawn()
        guard:SetVisible(true)
        guard:SetReactState(REACT_AGGRESSIVE)
        guard:EnterEvadeMode()
    end
end

-- C++ npc_sinclariAI::DoAction(ACTION_SINCLARI_OUTRO) + ScheduleOutro
-- (violet_hold.cpp:437-442/:517-529): visible, then 4s -> Talk(2) +
-- MovePoint(0, outro spot), then 10s -> gossip flag back. Fired by
-- the instance script's MAIN_EVENT_STATE DONE arm
-- (instance_violet_hold.lua), dormant until then.
local function sinclariDoAction(event, creature, action)
    if action ~= ACTION_SINCLARI_OUTRO then
        return
    end
    creature:SetVisible(true)
    local guid = creature:GetGUID()
    schedule(guid, "outro", 4000, function()
        creature:Talk(SAY_SINCLARI_OUTRO)
        creature:MoveTo(0, SINCLARI_OUTRO_POS[1], SINCLARI_OUTRO_POS[2], SINCLARI_OUTRO_POS[3])
        schedule(guid, "outroflag", 10000, function()
            creature:SetFlag(UNIT_NPC_FLAGS, UNIT_NPC_FLAG_GOSSIP)
        end)
    end)
end

-- C++ npc_violet_hold_teleportation_portal_commonAI::JustSummoned
-- (violet_hold.cpp:695-699): track the summon and pass it this
-- portal's location, so the trash escort machine can start its path.
-- Also the elite/intro _summons counts.
local function portalJustSummoned(event, creature, summon)
    summon:SetData(DATA_PORTAL_LOCATION, creature:GetData(DATA_PORTAL_LOCATION))
    local guid = creature:GetGUID()
    local entry = creature:GetEntry()
    if entry == NPC_TELEPORTATION_PORTAL_ELITE then
        local set = eliteSummons[guid]
        if not set then
            set = {}
            eliteSummons[guid] = set
        end
        set[summon:GetGUID()] = true
    elseif entry == NPC_TELEPORTATION_PORTAL_INTRO then
        local set = introSummons[guid]
        if not set then
            set = {}
            introSummons[guid] = set
        end
        set[summon:GetGUID()] = true
    end
end

-- C++ npc_violet_hold_teleportation_portalAI::SummonedCreatureDies
-- (violet_hold.cpp:775-782): a dead portal guardian/keeper advances
-- the wave count and despawns the portal. Trash deaths only drop
-- from _summons (no Lua action).
local function portalSummonedDied(event, creature, summon)
    local entry = summon:GetEntry()
    if entry ~= NPC_PORTAL_GUARDIAN and entry ~= NPC_PORTAL_KEEPER then
        return
    end
    local data = instanceDataOf(creature)
    if data then
        data:SetData(DATA_WAVE_COUNT, waveCountOf(creature) + 1)
    end
    cancelTimers(creature:GetGUID())
    creature:DespawnOrUnsummon()
end

-- Shared summon-loss leg for the elite/intro portals: drop the GUID
-- from the _summons set; the elite portal advances the wave and
-- despawns once its set empties after the 15s task fired.
local function portalSummonLost(event, creature, summon)
    local guid = creature:GetGUID()
    local entry = creature:GetEntry()
    local set = nil
    if entry == NPC_TELEPORTATION_PORTAL_ELITE then
        set = eliteSummons[guid]
    elseif entry == NPC_TELEPORTATION_PORTAL_INTRO then
        set = introSummons[guid]
    end
    if set then
        set[summon:GetGUID()] = nil
    end
    if entry == NPC_TELEPORTATION_PORTAL_ELITE and eliteFired[guid] and set and next(set) == nil then
        local data = instanceDataOf(creature)
        if data then
            data:SetData(DATA_WAVE_COUNT, waveCountOf(creature) + 1)
        end
        cancelTimers(guid)
        creature:DespawnOrUnsummon()
    end
end

-- C++ npc_violet_hold_teleportation_portalAI::InitializeAI
-- (violet_hold.cpp:708-712): self-cast the periodic aura, then the
-- SetData poll (see header). Only the base portal casts 58008;
-- elite/intro do not.
local function portalSpawn(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:CastSpell(creature, SPELL_PORTAL_PERIODIC, true)
    schedule(guid, "poll", 1000, function() portalPoll(creature, guid) end)
end

-- C++ npc_violet_hold_teleportation_portal_eliteAI::ScheduleTasks
-- (violet_hold.cpp:801-820): 15s -> summon 3 (wave < 12) or 4 elite
-- trash, Sinclari-trigger elite-squad Talk, 1s later hide the
-- portal. The wave advances when the last summon dies (ev21/ev20,
-- portalSummonLost above).
local function eliteTask(creature, guid)
    local k = 3
    if waveCountOf(creature) >= 12 then
        k = 4
    end
    for _ = 1, k do
        creature:SummonCreature(ELITE_TRASH_ENTRIES[math.random(#ELITE_TRASH_ENTRIES)], 0, 0, 0, 0, TEMPSUMMON_DEAD_DESPAWN, 20000)
    end
    local trigger = creature:GetInstanceCreature(DATA_SINCLARI_TRIGGER)
    if trigger then
        trigger:Talk(SAY_SINCLARI_ELITE_SQUAD)
    end
    eliteFired[guid] = true
    schedule(guid, "hide", 1000, function() creature:SetVisible(false) end)
end

local function eliteSpawn(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "elite", 15000, function() eliteTask(creature, guid) end)
end

-- C++ npc_violet_hold_teleportation_portal_introAI::ScheduleTasks
-- (violet_hold.cpp:838-857): only when the event has not started —
-- 15s repeat, summon one trickle trash while fewer than 3 of its
-- summons live. The event-state gate is read once at spawn, exactly
-- as C++ reads it once in InitializeAI; once scheduled the trickle
-- repeats unconditionally.
local function introTick(creature, guid)
    local count = 0
    for _ in pairs(introSummons[guid] or {}) do
        count = count + 1
    end
    if count < 3 then
        creature:SummonCreature(INTRO_TRASH_ENTRIES[math.random(#INTRO_TRASH_ENTRIES)], 0, 0, 0, 0, TEMPSUMMON_DEAD_DESPAWN, 20000)
    end
    schedule(guid, "trickle", 15000, function() introTick(creature, guid) end)
end

local function introSpawn(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    local data = instanceDataOf(creature)
    if data and (data.eventState or 0) ~= ENCOUNTER_NOT_STARTED then
        return
    end
    schedule(guid, "trickle", 15000, function() introTick(creature, guid) end)
end

local function portalCancel(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterAuraEvent(SPELL_PORTAL_PERIODIC, 1, portalPeriodicTick)
RegisterAuraEvent(SPELL_DESTROY_DOOR_SEAL, 1, destroyDoorSealTick)

RegisterCreatureEvent(NPC_SINCLARI, 23, sinclariReset)
RegisterCreatureEvent(NPC_SINCLARI, 38, sinclariDoAction)

RegisterCreatureEvent(NPC_TELEPORTATION_PORTAL, 23, portalSpawn)
RegisterCreatureEvent(NPC_TELEPORTATION_PORTAL, 37, portalCancel)
RegisterCreatureEvent(NPC_TELEPORTATION_PORTAL, 19, portalJustSummoned)
RegisterCreatureEvent(NPC_TELEPORTATION_PORTAL, 21, portalSummonedDied)

RegisterCreatureEvent(NPC_TELEPORTATION_PORTAL_ELITE, 23, eliteSpawn)
RegisterCreatureEvent(NPC_TELEPORTATION_PORTAL_ELITE, 37, portalCancel)
RegisterCreatureEvent(NPC_TELEPORTATION_PORTAL_ELITE, 19, portalJustSummoned)
RegisterCreatureEvent(NPC_TELEPORTATION_PORTAL_ELITE, 20, portalSummonLost)
RegisterCreatureEvent(NPC_TELEPORTATION_PORTAL_ELITE, 21, portalSummonLost)

RegisterCreatureEvent(NPC_TELEPORTATION_PORTAL_INTRO, 23, introSpawn)
RegisterCreatureEvent(NPC_TELEPORTATION_PORTAL_INTRO, 37, portalCancel)
RegisterCreatureEvent(NPC_TELEPORTATION_PORTAL_INTRO, 19, portalJustSummoned)
RegisterCreatureEvent(NPC_TELEPORTATION_PORTAL_INTRO, 20, portalSummonLost)
RegisterCreatureEvent(NPC_TELEPORTATION_PORTAL_INTRO, 21, portalSummonLost)
