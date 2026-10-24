-- Magister Aledis (Hellfire Peninsula, Arelion's Secret duel) — Lua
-- port of npc_magister_aledisAI in
-- src/server/scripts/Outland/zone_hellfire_peninsula.cpp (lines 822-927).
-- Entry (wowhead, no C++ NPC_ constant exists — the cthun/telestra
-- wowhead-only precedent, never guessed): 20159, via the wowwiki
-- Magister Aledis page's Wowhead external link (wowhead.com/?npc=20159).
-- Eluna creature events: 5 OnSpawn, 23 OnReset, 2 OnLeaveCombat,
-- 4 OnDied, 9 OnDamageTaken; gossip select 2.
-- Ported arms (C++-exact unless noted):
-- - Reset: faction restore (ctor-captured factionNorm, the fizzule
--   precedent — RestoreFaction has no Lua bridge) + RemoveFlag
--   (QUESTGIVER) + SetFlag(GOSSIP) + SetFlag(UNIT_FIELD_FLAGS,
--   IMMUNE_TO_PC 0x100) as the SetImmuneToPC(true) twin (opera
--   precedent: SetImmuneToPC(false) = RemoveFlag(59, 0x100)).
-- - Gossip select: CloseGossipMenuFor twin (GossipClearMenu +
--   GossipComplete, vaelastrasz precedent) + Dismount +
--   SetFacingToObject twin (math.atan2 facing, the documented
--   lua_creature_events.go:1745 idiom) + RemoveFlag(GOSSIP) +
--   2s Talk(SAY_CHALLENGE 0) + 2s immune-off + SetFaction
--   (MONSTER_2) + fireball 20823 victim-cast (1ms, 10s re-arm)
--   + frostnova 11831 self-cast (5s, 20s re-arm).
-- - Defeat (C++ DamageTaken: lethal-or-below-20%): timers reset +
--   faction restore + RemoveAllAuras + CombatStop(true) +
--   AttackStop + SetFlag(QUESTGIVER) + SetFlag(IMMUNE_TO_PC) +
--   Talk(SAY_DEFEATED 1) + damage rewritten to 0 (midnight
--   convention: return false, 0). Defeated latch blocks re-fire
--   (C++ is immune after defeat, so no second trigger exists).
-- Unmodeled (documented-only, no bridges):
-- - EVENT_ATTACK's EngageWithTarget (no Lua bridge; DoZoneInCombat
--   is dungeon-gated, so it no-ops in Hellfire Peninsula). The
--   cast pumps still run from the gossip chain and land once the
--   player attacks (player-initiated combat supplies the victim).
-- - EVENT_EVADE's EnterEvadeMode (no Lua bridge, 2026-10-21
--   standing note): the 60s post-defeat evade never fires, so the
--   defeated flags persist until Reset.
-- - EngagementOver (no Lua bridge anywhere in-tree): CombatStop +
--   AttackStop are the twins.

local ENTRY_ALEDIS = 20159

local SAY_CHALLENGE = 0
local SAY_DEFEATED = 1

local SPELL_FIREBALL = 20823
local SPELL_FROSTNOVA = 11831

local FACTION_MONSTER_2 = 16

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_IMMUNE_TO_PC = 0x100
local UNIT_NPC_FLAGS_FIELD = 82
local UNIT_NPC_FLAG_GOSSIP = 1
local UNIT_NPC_FLAG_QUESTGIVER = 2

local timers = {}
local factionNorm = {}
local defeated = {}

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
    per[key] = CreateLuaEvent(fn, delay)
end

local function fireballPump(creature, guid)
    creature:CastSpell(nil, SPELL_FIREBALL)
    schedule(guid, "fireball", 10000, function()
        fireballPump(creature, guid)
    end)
end

local function frostnovaPump(creature, guid)
    creature:CastSpell(creature, SPELL_FROSTNOVA)
    schedule(guid, "frostnova", 20000, function()
        frostnovaPump(creature, guid)
    end)
end

local function aledisSpawn(event, creature)
    factionNorm[creature:GetGUID()] = creature:GetFaction()
end

local function aledisReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    defeated[guid] = nil
    local norm = factionNorm[guid]
    if norm ~= nil then
        creature:SetFaction(norm)
    end
    creature:RemoveFlag(UNIT_NPC_FLAGS_FIELD, UNIT_NPC_FLAG_QUESTGIVER)
    creature:SetFlag(UNIT_NPC_FLAGS_FIELD, UNIT_NPC_FLAG_GOSSIP)
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC)
end

local function aledisGossipSelect(event, player, creature, sender, action)
    player:GossipClearMenu()
    player:GossipComplete()
    creature:Dismount()
    local cx, cy = creature:GetX(), creature:GetY()
    local px, py = player:GetX(), player:GetY()
    if cx ~= nil and cy ~= nil and px ~= nil and py ~= nil then
        local angle = math.atan2(py - cy, px - cx)
        if angle < 0 then
            angle = angle + 2 * math.pi
        end
        creature:SetFacingTo(angle)
    end
    creature:RemoveFlag(UNIT_NPC_FLAGS_FIELD, UNIT_NPC_FLAG_GOSSIP)
    local guid = creature:GetGUID()
    schedule(guid, "talk", 2000, function()
        creature:Talk(SAY_CHALLENGE)
        schedule(guid, "attack", 2000, function()
            creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC)
            creature:SetFaction(FACTION_MONSTER_2)
            schedule(guid, "fireball", 1, function()
                fireballPump(creature, guid)
            end)
            schedule(guid, "frostnova", 5000, function()
                frostnovaPump(creature, guid)
            end)
        end)
    end)
    return false
end

local function aledisLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function aledisDied(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    defeated[guid] = nil
end

local function aledisDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local health = creature:GetHealth()
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 or health == 0 or defeated[guid] then
        return false, damage
    end
    local postHealth = health - damage
    if damage > health or postHealth < math.floor(maxHealth * 20 / 100) then
        defeated[guid] = true
        cancelTimers(guid)
        local norm = factionNorm[guid]
        if norm ~= nil then
            creature:SetFaction(norm)
        end
        creature:RemoveAllAuras()
        creature:CombatStop(true)
        creature:AttackStop()
        creature:SetFlag(UNIT_NPC_FLAGS_FIELD, UNIT_NPC_FLAG_QUESTGIVER)
        creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC)
        creature:Talk(SAY_DEFEATED)
        return false, 0
    end
    return false, damage
end

RegisterCreatureEvent(ENTRY_ALEDIS, 5, aledisSpawn)
RegisterCreatureEvent(ENTRY_ALEDIS, 23, aledisReset)
RegisterCreatureGossipEvent(ENTRY_ALEDIS, 2, aledisGossipSelect)
RegisterCreatureEvent(ENTRY_ALEDIS, 2, aledisLeaveCombat)
RegisterCreatureEvent(ENTRY_ALEDIS, 4, aledisDied)
RegisterCreatureEvent(ENTRY_ALEDIS, 9, aledisDamageTaken)
