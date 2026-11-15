-- Zul'Aman zone script — Lua port of
-- src/server/scripts/EasternKingdoms/ZulAman/zulaman.cpp
-- (npc_zulaman_hostageAI). Hostage entries 23790, 23999, 24024, 24001
-- (C++ HostageEntry[]; AddSC_zulaman registers "npc_zulaman_hostage").
-- Eluna gossip events: 1 OnGossipHello (offers "I am glad to help you."),
-- 2 OnGossipSelect (closes the menu on the GOSSIP_ACTION_INFO_DEF + 1
-- action). No combat AI, no timers; melee is engine-driven in Go.
-- Deviations from C++: instance->SetData(DATA_CHESTLOOTED, 0) has no
-- bearer (no instance-script model); the chest SummonGameObject IS
-- ported (creature:SummonGameObject bridged — onSelect above);
-- player->GetGossipTextId is bridged (lua_player.go:5954) and onHello
-- passes it (C++ zulaman.cpp:62 SendGossipMenuFor with GetGossipTextId).
-- (The old "no HasFlag/RemoveFlag on the creature Lua object" claim was
-- stale — the Eluna gossip creature has GetNPCFlags/SetNPCFlags,
-- engine/world/lua_objects.go:3265/3274 — so the one-shot below is the
-- C++-exact UNIT_NPC_FLAG_GOSSIP removal.)
-- The npc_harrison_jones script from the same C++ file is PARTLY ported:
-- its OnGossipSelect gates on the creature's GossipMenuId and its entire
-- gong-event machine is MovePath-by-ID work (no bridge; MovePoint IS
-- bridged via creature:MoveTo, lua_creature_events.go:5417, but the chain
-- cannot start without MovePath); the SetFacingTo / equipment
-- (SetEquipmentSlots bridged) / emote (PerformEmote bridged) arms ride
-- that unported machine, not a missing bridge — verdict unchanged. Its
-- Reset() (zulaman.cpp:188) only zeroes _gongEvent/_gongTimer, state
-- owned by that same unported machine, so ev23 is a no-op without it
-- and is not registered either (round-28 triage). Instance
-- GetGameObject arms (no instance-script GetGameObject bridge; SetData
-- itself rides Map:GetInstanceData + InstanceData:SetData,
-- scripting/globals.go:1442, instance_hooks.go:51, but no Zul'Aman
-- instance consumer exists), and
-- GetCreatureListWithEntryInGrid/SetImmuneToPC/AI SetData
-- arms for the Amanishi guardians (no bridge). Its SpellHit arm IS
-- ported below (harrisonSpellHit): it fires
-- only for SPELL_COSMETIC_SPEAR_THROW 43647, which HAS a DB-side caster
-- (10:41 'zero casters / trigger-absence' claim corrected 2026-11-15:
-- TDB smart_scripts (23597,0,9) Amani'shi Guardian OOC cast 43647,
-- target_type 1 SELF; no conditions row for it in the dump, so the
-- 10:41 'conditions row gates target to Harrison 24375' detail did NOT
-- verify — the spell's own implicit targets decide the hit, pending the
-- 43647 wago read; 43647 is still unseeded (sibling seed in flight).
-- The 2026-11-15 "no creature:SetByteValue bridge" blocker was STALE:
-- SetByteValue IS bridged on event creatures via
-- registerIndexedValueMethods (lua_creature_events.go:7121), and the
-- stand-state byte it writes is the SetStandState twin (:5190), used
-- here. Registered for BOTH 24358 and 24375: the C++ AI persists across
-- the GONG_EVENT_5 SetEntry (24358 -> 24375, zulaman.cpp:274), while Go
-- dispatches ev14 on the live motion.Entry (fireCreatureLuaEvent,
-- lua_creature_events.go:8060), so a 24358-only registration would never
-- fire at spear time. (ev14 fires on creature-cast hits too,
-- boss_ai.go:4267/4282 fireCreatureLuaEvent OnHitBySpell inside
-- castCreatureSpellInternalWithBasePoints :1791; the earlier "ev14
-- fires only for player casts" rationale was stale.)

local HOSTAGE_ENTRIES = {23790, 23999, 24024, 24001}

local GOSSIP_ICON_CHAT = 0
local GOSSIP_SENDER_MAIN = 1
local GOSSIP_ACTION_INFO_DEF = 1000
local GOSSIP_HOSTAGE1 = "I am glad to help you."

-- UNIT_NPC_FLAG_GOSSIP (UnitDefines.h:186) — the engine runs the gossip
-- hooks only while this flag is set (gossip.go helloCreatureNPC /
-- select flag gates), so clearing it is the C++-exact one-shot.
local UNIT_NPC_FLAG_GOSSIP = 0x00000001

-- C++ HostageEntry[] -> ChestEntry[] (zulaman.cpp:46): 23790 -> 186648,
-- 23999 -> 187021, 24024 -> 186672, 24001 -> 186667.
local CHEST_BY_HOSTAGE = { [23790] = 186648, [23999] = 187021, [24024] = 186672, [24001] = 186667 }

local function onHello(event, player, creature)
    -- C++ adds the item unconditionally; the hook only fires while the
    -- flag is set, so the option is inherently one-shot.
    player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_HOSTAGE1, GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 1)
    player:GossipSendMenu(player:GetGossipTextId(creature), creature)
end

local function onSelect(event, player, creature, sender, action)
    player:GossipClearMenu()
    if action == GOSSIP_ACTION_INFO_DEF + 1 then
        player:GossipComplete()
    end
    -- C++-exact order: the close arms run first; then the flag gate.
    if bit32.band(creature:GetNPCFlags(), UNIT_NPC_FLAG_GOSSIP) == 0 then
        return
    end
    -- C++ me->RemoveFlag(UNIT_NPC_FLAGS, UNIT_NPC_FLAG_GOSSIP).
    creature:SetNPCFlags(bit32.band(creature:GetNPCFlags(), bit32.bnot(UNIT_NPC_FLAG_GOSSIP)))
    -- C++ arm order (zulaman.cpp:82-89): instance->SetData(DATA_CHESTLOOTED,
    -- 0) [strands — no instance-script model], then SummonGameObject(
    -- ChestEntry[i], x-2, y, z, o 0, 0s). QuaternionData cosmetic-dropped.
    local chest = CHEST_BY_HOSTAGE[creature:GetEntry()]
    if chest then
        creature:SummonGameObject(chest, creature:GetX() - 2, creature:GetY(), creature:GetZ(), 0, 0)
    end
end

for _, entry in ipairs(HOSTAGE_ENTRIES) do
    RegisterCreatureGossipEvent(entry, 1, onHello)
    RegisterCreatureGossipEvent(entry, 2, onSelect)
end

-- C++ zulaman.h EntryIds / DisplayIds / ZADataTypes; InstanceScript.h:73
-- DONE = 3; zulaman.h DATA_GONGEVENT = 7 (BOSS_* 0-5, MAX_ENCOUNTER 6).
-- 24358 is the only entry carrying ScriptName npc_harrison_jones in TDB
-- (26091); 24375 is the GONG_EVENT_5 SetEntry target, 24365 the spear
-- target (its TDB name is Willie's Corpse — the entry, not the name, is
-- what C++ sets).
local ENTRY_HARRISON_JONES = 24358
local ENTRY_HARRISON_JONES_1 = 24375
local ENTRY_HARRISON_JONES_2 = 24365
local MODEL_HARRISON_JONES_2 = 22347
local SPELL_COSMETIC_SPEAR_THROW = 43647
local DATA_GONGEVENT = 7
local ENCOUNTER_DONE = 3
local UNIT_DYNAMIC_FLAGS_FIELD = 79
local UNIT_DYNFLAG_DEAD = 0x20
local UNIT_STAND_STATE_DEAD = 7

-- The boss files reach the instance script through the shared
-- instanceData table (razorgore precedent:
-- creature:GetMap():GetInstanceData()); nil when no instance binding
-- exists for the map (no instance_zulaman.lua yet).
local function instanceDataOf(creature)
    local mapObj = creature:GetMap()
    return mapObj and mapObj:GetInstanceData() or nil
end

-- C++ npc_harrison_jonesAI::SpellHit (zulaman.cpp:209-224): the
-- Amani'shi Guardian's spear (43647, smart_scripts 23597,0,9, OOC
-- one-shot) drops Harrison where he stands and starts the 21-minute
-- quest timer (instance SetData DATA_GONGEVENT DONE -> QuestMinute = 21,
-- instance_zulaman.cpp:203-210). Arm order is C++-exact; SetStandState
-- is the SetByteValue(UNIT_FIELD_BYTES_1, 0, UNIT_STAND_STATE_DEAD)
-- twin, SetTarget(0) the SetTarget(ObjectGuid::Empty) twin, and the
-- bare SetEntry the Object::SetEntry (Object.h:85) twin — no template
-- reload, unlike UpdateEntry. The instance write strands until an
-- instance_zulaman binding exists; the 43647 seed is still pending, so
-- the arm is live only once the spell resolves.
local function harrisonSpellHit(event, creature, caster, spellId)
    if spellId ~= SPELL_COSMETIC_SPEAR_THROW then
        return
    end
    creature:RemoveAllAuras()
    creature:SetEntry(ENTRY_HARRISON_JONES_2)
    creature:SetDisplayId(MODEL_HARRISON_JONES_2)
    creature:SetTarget(0)
    creature:SetStandState(UNIT_STAND_STATE_DEAD)
    creature:SetFlag(UNIT_DYNAMIC_FLAGS_FIELD, UNIT_DYNFLAG_DEAD)
    local data = instanceDataOf(creature)
    if data ~= nil then
        data:SetData(DATA_GONGEVENT, ENCOUNTER_DONE)
    end
end

RegisterCreatureEvent(ENTRY_HARRISON_JONES, 14, harrisonSpellHit)
RegisterCreatureEvent(ENTRY_HARRISON_JONES_1, 14, harrisonSpellHit)
