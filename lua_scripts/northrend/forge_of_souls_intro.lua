-- Forge of Souls intro dialogues — Lua port of
-- src/server/scripts/Northrend/FrozenHalls/ForgeOfSouls/forge_of_souls.cpp
-- (npc_jaina_fos + npc_sylvanas_fos; the FIRST group of the
-- "// Forge of Souls" block in AddNorthrendScripts(), before
-- AddSC_boss_bronjahm()).
-- Entries (forge_of_souls.h:44-45): 37596 Sylvanas part 1, 37597 Jaina
-- part 1 — the CreatureScript ScriptName bindings are DB-side as usual;
-- the instance team-swap (instance_forge_of_souls.cpp GetCreatureEntry)
-- only decides which of the two the player meets, both chains port here.
--
-- PORTED ARMS (C++-exact for the modeled legs):
-- OnGossipSelect (menuId 10943 Jaina / 10971 Sylvanas, option 0):
--   CloseGossipMenuFor + phase INTRO + RemoveFlag(UNIT_NPC_FLAGS,
--   UNIT_NPC_FLAG_GOSSIP) + schedule INTRO_1 at 1000ms.
-- Jaina chain (8 Talks): INTRO_1 Talk(0) at 1s, then 8s -> Talk(1),
--   8.5s -> Talk(2), 8s -> Talk(3), 10s -> Talk(4), 8s -> Talk(5),
--   12s -> Talk(6), 8s -> Talk(7), then phase NORMAL (end of intro).
-- Sylvanas chain (6 Talks): INTRO_1 Talk(0) at 1s, then 11.5s -> Talk(1),
--   10.5s -> Talk(2), 9.5s -> Talk(3), 10.5s -> Talk(4), 9.5s -> Talk(5),
--   then phase NORMAL (end of intro).
-- Reset: events.Reset + phase NORMAL — the chain is cancelled, so a
--   reset NPC stops mid-intro exactly like C++.
--
-- DOCUMENTED-ONLY:
-- The C++ (menuId, gossipListId) gate: the menuId half is encoded by
-- entry (each part-1 creature offers only its own menu DB-side) and
-- the gossipListId == 0 half is now gated on the hook's listID arg
-- (gossip.go creature select dispatch); non-zero selects return false
-- with no side effects, exactly like the C++ gate failing.
-- The ctor SetFlag(UNIT_NPC_FLAGS, UNIT_NPC_FLAG_GOSSIP) leg is
-- template/DB-side in Go; only the select-time removal ports (the
-- vaelastrasz SetNPCFlags bit-clear precedent). Reset does NOT restore
-- the flag — C++ restores it nowhere (ctor-only), so a reset NPC keeps
-- its post-select flag state.
-- The soul-sickness AuraScript (69131 -> lethargy 69133 on expire)
-- joins the no-AuraScript-bridge queue; the instance script
-- (team latch, GUID latching, GetGuidData, boss boundaries) joins the
-- no-instance-script-bridge queue.

local ENTRY_SYLVANAS_PART1 = 37596
local ENTRY_JAINA_PART1 = 37597

local UNIT_NPC_FLAG_GOSSIP = 0x00000001

-- delays[i] = ms from Talk(i) to Talk(i+1); the first Talk fires 1000ms
-- after the gossip select in both chains (C++ ScheduleEvent INTRO_1 1s).
local JAINA_DELAYS = { 8000, 8500, 8000, 10000, 8000, 12000, 8000 }
local SYLVANAS_DELAYS = { 11500, 10500, 9500, 10500, 9500 }

local introTimers = {}

local function cancelIntro(guid)
    local id = introTimers[guid]
    if id then
        RemoveEventById(id)
        introTimers[guid] = nil
    end
end

local function introStep(creature, guid, delays, index)
    creature:Talk(index)
    local nextDelay = delays[index + 1]
    if nextDelay then
        introTimers[guid] = CreateLuaEvent(function()
            introTimers[guid] = nil
            introStep(creature, guid, delays, index + 1)
        end, nextDelay)
    else
        introTimers[guid] = nil
    end
end

local function startIntro(creature, delays)
    local guid = creature:GetGUID()
    cancelIntro(guid)
    creature:SetNPCFlags(bit32.band(creature:GetNPCFlags(), bit32.bnot(UNIT_NPC_FLAG_GOSSIP)))
    introTimers[guid] = CreateLuaEvent(function()
        introTimers[guid] = nil
        introStep(creature, guid, delays, 0)
    end, 1000)
end

local function onGossipSelect(event, player, creature, sender, action, code, listID)
    if listID ~= 0 then
        return false
    end
    player:GossipClearMenu()
    player:GossipComplete()
    if creature:GetEntry() == ENTRY_JAINA_PART1 then
        startIntro(creature, JAINA_DELAYS)
    else
        startIntro(creature, SYLVANAS_DELAYS)
    end
end

local function onReset(event, creature)
    cancelIntro(creature:GetGUID())
end

RegisterCreatureGossipEvent(ENTRY_JAINA_PART1, 2, onGossipSelect)
RegisterCreatureGossipEvent(ENTRY_SYLVANAS_PART1, 2, onGossipSelect)
RegisterCreatureEvent(ENTRY_JAINA_PART1, 23, onReset)
RegisterCreatureEvent(ENTRY_SYLVANAS_PART1, 23, onReset)
RegisterCreatureEvent(ENTRY_JAINA_PART1, 4, onReset)
RegisterCreatureEvent(ENTRY_SYLVANAS_PART1, 4, onReset)
