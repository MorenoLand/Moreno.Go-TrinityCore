-- Wintergrasp queue NPCs (Dalaran battle-magi) — Lua port of
-- npc_wg_queueAI in
-- src/server/scripts/Northrend/zone_wintergrasp.cpp (lines 264-350).
-- Entries (wowhead, no C++ NPC_ constant exists for these — the
-- cthun/telestra wowhead-only precedent, never guessed): 32169
-- "Arcanist Braedin" <Wintergrasp Battle-Mage> and 32170
-- "Magister Surdiel" <Wintergrasp Battle-Mage>, the Dalaran
-- Alliance/Horde pair (sequential ids, both list Frost Armor as
-- their ability, both attested as the Dalaran Wintergrasp
-- battlemasters); plus three verified capital-city
-- Wintergrasp Battle-Mages, all sharing this one AI: 35600
-- "Arcanist Laniria" (Ironforge, wowhead <Wintergrasp
-- Battle-Mage>), 35601 "Arcanist Nazalia" (Exodar, wowhead
-- <Wintergrasp Battle-Mage>) and 35602 "Magister Savarin"
-- (Orgrimmar, WotLK DB <Wintergrasp Battle-Mage>). The
-- remaining capital WG battle-magi (Stormwind Paharin,
-- Darnassus Dulial, Shattrath Iramhir/Aldessia, Thunder Bluff
-- Dalhyr, Undercity Erembria, Silvermoon Caradess) are NOT
-- registered here: individually unverified, and the sequential
-- 35600-35602 block is a hint only, never an entry source.
local NPC_WG_QUEUE_ALLIANCE = 32169
local NPC_WG_QUEUE_HORDE = 32170
local NPC_WG_QUEUE_IRONFORGE = 35600
local NPC_WG_QUEUE_EXODAR = 35601
local NPC_WG_QUEUE_ORGRIMMAR = 35602

local SPELL_FROST_ARMOR = 12544

-- GUID-keyed CreateLuaEvent timers (C++ Reset seeds the timer at
-- 0; spawn/reset-started here with cancel-first idempotence —
-- the cthun precedent — since a battle-mage's Reset is the
-- schedule point).
local timers = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
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
    per[key] = CreateLuaEvent(fn, delay, 0)
end

-- C++ UpdateAI: DoCast(me, SPELL_FROST_ARMOR) at 0s, then every
-- 180s. Ungated in C++ (no combat check; DoMeleeAttackIfReady
-- after it is engine-driven and not modeled). The seed casts
-- immediately, matching the C++ 0s first tick, and the tick
-- re-arms at 180s.
local function frostArmorTick(creature, guid)
    creature:CastSpell(creature, SPELL_FROST_ARMOR)
    schedule(guid, "frost", 180000, function()
        frostArmorTick(creature, guid)
    end)
end

local function wgQueueSeed(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    frostArmorTick(creature, guid)
end

local function wgQueueCancel(event, creature)
    cancelTimers(creature:GetGUID())
end

for _, entry in ipairs({NPC_WG_QUEUE_ALLIANCE, NPC_WG_QUEUE_HORDE, NPC_WG_QUEUE_IRONFORGE, NPC_WG_QUEUE_EXODAR, NPC_WG_QUEUE_ORGRIMMAR}) do
    RegisterCreatureEvent(entry, 5, wgQueueSeed)
    RegisterCreatureEvent(entry, 23, wgQueueSeed)
    RegisterCreatureEvent(entry, 4, wgQueueCancel)
end

-- Unmodeled: OnGossipHello / OnGossipSelect (quest menu +
-- battlefield queue: IsWarTime / GetTimer / InvitePlayerToWar /
-- InvitePlayerToQueue via sBattlefieldMgr) — no battlefield
-- bridge. JustEngagedWith is empty in C++.
