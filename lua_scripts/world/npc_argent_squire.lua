-- npc_argent_squire_gruntling (Argent Squire 33238 + Argent Gruntling 33239) --
-- Lua port of src/server/scripts/World/npcs_special.cpp:2398-2543
-- (AddSC_npcs_special), both halves: the gruntling (entry 33239,
-- TDB-verified; no C++ const, same standing as the squire's own
-- TDB-verified entry) uses the G column (BANK_G 68849 / POSTMAN_G
-- 68850 / SHOP_G 68851 / TIRED_G 68852; pennants 63446/63441/63444/
-- 63438/63445). The squire materializes via a guardian-summon spell
-- effect; Go's player-cast summon-effect path spawns nothing
-- (lua_player.go:2773-2782), so the script binds by entry and works for
-- any squire/gruntling that exists, but players can't obtain one normally.
--
-- NOT ported: the tired-copy arm (:2406-2416) — creature:AddAura returns
-- nil and GetAura can't see Lua-added auras, so there is no aura object
-- to call SetDuration on (the bridge itself, lua_aura.go:450, is live).
-- Gossip menu is DB gossip; only the select arm is scripted.

local ENTRY_ARGENT_SQUIRE = 33238
local ENTRY_ARGENT_GRUNTLING = 33239
local UNIT_NPC_FLAGS = 82
local NPC_FLAG_VENDOR = 0x80
local NPC_FLAG_BANKER = 0x20000
local NPC_FLAG_MAILBOX = 0x4000000

local SPELL_TIRED_PLAYER = 67334
local SPELL_AURA_POSTMAN_S = 67376
local SPELL_AURA_SHOP_S = 67377
local SPELL_AURA_BANK_S = 67368
local SPELL_AURA_POSTMAN_G = 68850
local SPELL_AURA_SHOP_G = 68851
local SPELL_AURA_BANK_G = 68849
local SPELL_AURA_TIRED_S = 67401
local SPELL_AURA_TIRED_G = 68852

-- Gossip list ids (npcs_special.cpp:2378-2387) -> pennant spells
-- (bannerSpells, :2392-2398; squire column first, gruntling second).
local PENNANT_BY_LIST = {
    [3] = 63443,
    [4] = 63439,
    [5] = 63442,
    [6] = 63440,
    [7] = 62727,
}

local PENNANT_BY_LIST_G = {
    [3] = 63446,
    [4] = 63441,
    [5] = 63444,
    [6] = 63438,
    [7] = 63445,
}

local squirePump = {}

local function cancelPump(guid)
    local id = squirePump[guid]
    if id then
        RemoveEventById(id)
        squirePump[guid] = nil
    end
end

local function flagStripTick(creature, guid)
    if (creature:HasAura(SPELL_AURA_TIRED_S) or creature:HasAura(SPELL_AURA_TIRED_G))
        and creature:HasFlag(UNIT_NPC_FLAGS, NPC_FLAG_BANKER + NPC_FLAG_MAILBOX + NPC_FLAG_VENDOR) then
        creature:RemoveFlag(UNIT_NPC_FLAGS, NPC_FLAG_BANKER + NPC_FLAG_MAILBOX + NPC_FLAG_VENDOR)
    end
end

local function onSpawn(event, creature)
    -- C++ ScheduleTasks runs once in the ctor (:2402-2404); the 1s
    -- flag-strip pump re-arms via CreateLuaEvent repeats (earth-elemental
    -- idiom). cancelPump first: ev23 pre-fires on spawn, so ev5+ev23
    -- would otherwise double-seed.
    local guid = creature:GetGUIDLow()
    cancelPump(guid)
    squirePump[guid] = CreateLuaEvent(function()
        flagStripTick(creature, guid)
    end, 1000, 0)
end

local function onReset(event, creature)
    onSpawn(event, creature)
end

local function onDeath(event, creature)
    cancelPump(creature:GetGUIDLow())
end

local function onGossipSelect(event, player, creature, sender, action, code, listID)
    local isGruntling = creature:GetEntry() == ENTRY_ARGENT_GRUNTLING
    local bankAura = SPELL_AURA_BANK_S
    local shopAura = SPELL_AURA_SHOP_S
    local postmanAura = SPELL_AURA_POSTMAN_S
    local pennants = PENNANT_BY_LIST
    if isGruntling then
        bankAura = SPELL_AURA_BANK_G
        shopAura = SPELL_AURA_SHOP_G
        postmanAura = SPELL_AURA_POSTMAN_G
        pennants = PENNANT_BY_LIST_G
    end
    if listID == 0 then
        creature:SetFlag(UNIT_NPC_FLAGS, NPC_FLAG_BANKER)
        if not creature:HasAura(bankAura) then
            creature:CastSpell(creature, bankAura)
        end
        if not player:HasAura(SPELL_TIRED_PLAYER) then
            player:CastSpell(player, SPELL_TIRED_PLAYER, true)
        end
    elseif listID == 1 then
        creature:SetFlag(UNIT_NPC_FLAGS, NPC_FLAG_VENDOR)
        if not creature:HasAura(shopAura) then
            creature:CastSpell(creature, shopAura)
        end
        if not player:HasAura(SPELL_TIRED_PLAYER) then
            player:CastSpell(player, SPELL_TIRED_PLAYER, true)
        end
    elseif listID == 2 then
        creature:SetFlag(UNIT_NPC_FLAGS, NPC_FLAG_MAILBOX)
        player:SendShowMailBox(creature:GetGUID())
        if not creature:HasAura(postmanAura) then
            creature:CastSpell(creature, postmanAura)
        end
        if not player:HasAura(SPELL_TIRED_PLAYER) then
            player:CastSpell(player, SPELL_TIRED_PLAYER, true)
        end
    elseif pennants[listID] ~= nil then
        creature:CastSpell(creature, pennants[listID], true)
    end
    player:GossipComplete()
    -- C++ returns false; in Eluna polarity false lets the native arms
    -- proceed (gossip.go), matching the C++ OnGossipSelect contract.
    return false
end

RegisterCreatureEvent(ENTRY_ARGENT_SQUIRE, 5, onSpawn)
RegisterCreatureEvent(ENTRY_ARGENT_SQUIRE, 23, onReset)
RegisterCreatureEvent(ENTRY_ARGENT_SQUIRE, 4, onDeath)
RegisterCreatureGossipEvent(ENTRY_ARGENT_SQUIRE, 2, onGossipSelect)
RegisterCreatureEvent(ENTRY_ARGENT_GRUNTLING, 5, onSpawn)
RegisterCreatureEvent(ENTRY_ARGENT_GRUNTLING, 23, onReset)
RegisterCreatureEvent(ENTRY_ARGENT_GRUNTLING, 4, onDeath)
RegisterCreatureGossipEvent(ENTRY_ARGENT_GRUNTLING, 2, onGossipSelect)
