-- Hidden Cultist (Borean Tundra, quest 11794) --
-- Lua port of src/server/scripts/Northrend/zone_borean_tundra.cpp
-- (npc_hidden_cultistAI — Reset shroud arm only).
-- Zone-script unit per northrend_script_loader.cpp order (decl 199 /
-- call 394; dalaran done, borean_tundra: 20 registrations).
-- Retraction re-audit of the 15:34 "CLOSED document-only" entry: that
-- audit dismissed the whole file under stale "no-timer" claims. The
-- Reset arm below is ungated and bridgeable.
-- Entries (HiddenCultist enum :1468-1470, C++-verifiable): Tom Hegger
-- 25827, Salty John Thorpe 25248, Guard Mitchells 25828 — all three
-- share this one AI.
-- Eluna creature events: 5 OnSpawn, 23 OnReset, 4 OnDied,
-- gossip 1 OnHello / 2 OnSelect.
-- Ported arm (C++ Reset() :1508-1518): DoCast(
-- SPELL_SHROUD_OF_THE_DEATH_CULTIST 46077, :1459) — the 1-arg DoCast
-- is the non-triggered self-cast; the emote-state / npc-flags restores
-- and RestoreFaction() are DB-template idempotent (bridges exist —
-- creature:SetNPCFlags, creature:EmoteState — but the restores produce
-- no visible delta). One-shot per spawn/reset, no timers, no per-GUID
-- state (the blessed_banner / storm_cloud shape).
-- Ported 2026-11-03: the OnGossipSelect -> DoAction(1) -> uiEventPhase
-- Talk chain (:1651-1660 -> :1519 -> :1546-1605). The old "no
-- faction/attack bridges" bar is stale: SetFaction
-- (lua_creature_events.go:5216), AttackStart = EngageWithTarget
-- (:5665), StopMoving (:2609), SetNPCFlags (:5178), EmoteState
-- (:3695), Talk-with-player (beryl precedent) and the gossip hooks
-- (RegisterCreatureGossipEvent, runtime.go:902) are all live. The
-- phase machine rides a CreateLuaEvent chain; the C++ UpdateAI timer
-- block has no UpdateVictim gate, so no LeaveCombat cancel — death
-- (event 4) and reset (event 23) cancel. Select returns true (C++
-- returns true; gossip.go vetoes native arms on Lua true, the vael
-- false-returns-proceed contrast).
-- Unmodeled: Reset's emote-state restore — EmoteState is setter-only
-- (no getter), so the ctor-captured template value can't be read;
-- faction/flags restore via spawn-captured values (template proxy:
-- nothing mutates them before the first reset).
-- Go: RegisterLuaBoss("npc_hidden_cultist", <each entry>) — the no-op
-- AI keeps these quest NPCs passive pre-ambush (storm_cloud /
-- blessed_banner zone-script precedent).

local SPELL_SHROUD_OF_THE_DEATH_CULTIST = 46077
local SPELL_RIGHTEOUS_VISION = 46078
local QUEST_THE_HUNT_IS_ON = 11794
local FACTION_MONSTER = 14

local QUEST_STATUS_INCOMPLETE = 3

local GOSSIP_ICON_CHAT = 0
local GOSSIP_ICON_VENDOR = 1
local GOSSIP_SENDER_MAIN = 1
local GOSSIP_ACTION_INFO_DEF = 1000
local GOSSIP_ACTION_TRADE = 1
local GOSSIP_TEXT_BROWSE_GOODS = "I'd like to browse your goods."

local ENTRIES_HIDDEN_CULTIST = { 25827, 25248, 25828 }

-- Per-entry gossip text, item text, and phase-1 Talk id
-- (zone_borean_tundra.cpp:1468-1470, :1568-1590, :1546-1560).
local CULTIST_DATA = {
    [25827] = { text = 12528, item = "What do you know about the Cult of the Damned?", talk1 = 2 },
    [25248] = { text = 12529, item = "I have a reason to believe you're involved in the cultist activity", talk1 = 0 },
    [25828] = { text = 12530, item = "How long have you worked for the Cult of the Damned?", talk1 = 1 },
}

local chainTimers = {}
local templateState = {}

local function cancelChain(guid)
    local id = chainTimers[guid]
    if id then
        RemoveEventById(id)
        chainTimers[guid] = nil
    end
end

-- Forward declaration (tharon_ja / gal_darah idiom): the phase
-- chain's scheduler references runPhase, which is assigned below
-- with a plain assignment, never a second local function.
local runPhase

local function schedulePhase(guid, delay, phase, creature, player)
    cancelChain(guid)
    chainTimers[guid] = CreateLuaEvent(function() runPhase(creature, player, guid, phase) end, delay)
end

-- SetFacingToObject(player) via the atan2 + SetFacingTo twin
-- (lake-frog precedent).
local function facePlayer(creature, player)
    local dx = player:GetX() - creature:GetX()
    local dy = player:GetY() - creature:GetY()
    local face = math.atan2(dy, dx)
    if face < 0 then face = face + 2 * math.pi end
    creature:SetFacingTo(face)
end

-- C++ AttackPlayer() (:1541-1545): SetFaction(FACTION_MONSTER) +
-- AttackStart(player).
local function attackPlayer(creature, player)
    creature:SetFaction(FACTION_MONSTER)
    creature:AttackStart(player)
end

runPhase = function(creature, player, guid, phase)
    local entry = creature:GetEntry()
    if phase == 1 then
        local data = CULTIST_DATA[entry]
        if not data then
            chainTimers[guid] = nil
            return
        end
        if entry == 25827 then
            creature:Talk(data.talk1, player)
        else
            creature:Talk(data.talk1)
        end
        schedulePhase(guid, 5000, 2, creature, player)
    elseif phase == 2 then
        if entry == 25248 then
            creature:Talk(3)
            facePlayer(creature, player)
            schedulePhase(guid, 3000, 3, creature, player)
        else
            attackPlayer(creature, player)
            chainTimers[guid] = nil
        end
    elseif phase == 3 then
        attackPlayer(creature, player)
        chainTimers[guid] = nil
    end
end

-- C++ Reset() :1514 — non-triggered self-cast of the shroud visual,
-- one-shot per spawn / reset.
local function hiddenCultistSpawnOrReset(event, creature)
    -- C++ npc_hidden_cultistAI::Reset(): DoCast(SPELL_SHROUD_OF_THE_DEATH_CULTIST)
    -- (zone_borean_tundra.cpp). Alive-gated so the death-leg ev23 fire is not phantom.
    if event == 23 and creature:IsDead() then
        return
    end
    local guid = creature:GetGUID()
    if event == 5 then
        -- Template proxy for the C++ ctor-captured values and
        -- RestoreFaction(): spawn-time faction/flags; nothing
        -- mutates them before the first reset.
        templateState[guid] = { faction = creature:GetFaction(), flags = creature:GetNPCFlags() }
    elseif event == 23 then
        cancelChain(guid)
        local tpl = templateState[guid]
        if tpl then
            creature:SetFaction(tpl.faction)
            creature:SetNPCFlags(tpl.flags)
        end
    end
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_SHROUD_OF_THE_DEATH_CULTIST)
    end
end

local function hiddenCultistDied(event, creature)
    local guid = creature:GetGUID()
    cancelChain(guid)
    templateState[guid] = nil
end

-- C++ OnGossipHello (:1564-1601): entry-gated text/item; the cult
-- item only with the Righteous Vision aura (46078) and quest 11794
-- incomplete; the vendor item only when the NPC is a vendor.
local function hiddenCultistGossipHello(event, player, creature)
    local data = CULTIST_DATA[creature:GetEntry()]
    if not data then
        return
    end
    if player:HasAura(SPELL_RIGHTEOUS_VISION) and player:GetQuestStatus(QUEST_THE_HUNT_IS_ON) == QUEST_STATUS_INCOMPLETE then
        player:GossipMenuAddItem(GOSSIP_ICON_CHAT, data.item, GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 1)
    end
    if creature:IsVendor() then
        player:GossipMenuAddItem(GOSSIP_ICON_VENDOR, GOSSIP_TEXT_BROWSE_GOODS, GOSSIP_SENDER_MAIN, GOSSIP_ACTION_TRADE)
    end
    player:GossipSendMenu(data.text, creature)
end

-- C++ OnGossipSelect (:1603-1617): menu cleared unconditionally;
-- info action -> close menu + DoAction(1); trade action -> vendor
-- list. Returns true (C++ returns true).
local function hiddenCultistGossipSelect(event, player, creature, sender, action)
    player:GossipClearMenu()
    if sender == GOSSIP_SENDER_MAIN and action == GOSSIP_ACTION_INFO_DEF + 1 then
        player:GossipComplete()
        -- C++ DoAction (:1519-1530): stop, clear gossip flag +
        -- emote state, face the player, 3000ms to phase 1.
        local guid = creature:GetGUID()
        cancelChain(guid)
        creature:StopMoving()
        creature:SetNPCFlags(0)
        creature:EmoteState(0)
        facePlayer(creature, player)
        schedulePhase(guid, 3000, 1, creature, player)
    elseif sender == GOSSIP_SENDER_MAIN and action == GOSSIP_ACTION_TRADE then
        player:SendListInventory(creature:GetGUID())
    end
    return true
end

for _, entry in ipairs(ENTRIES_HIDDEN_CULTIST) do
    RegisterCreatureEvent(entry, 5, hiddenCultistSpawnOrReset)
    RegisterCreatureEvent(entry, 23, hiddenCultistSpawnOrReset)
    RegisterCreatureEvent(entry, 4, hiddenCultistDied)
    RegisterCreatureGossipEvent(entry, 1, hiddenCultistGossipHello)
    RegisterCreatureGossipEvent(entry, 2, hiddenCultistGossipSelect)
end
