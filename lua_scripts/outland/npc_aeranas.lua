-- Aeranas (Hellfire Peninsula, Avruu's Orb summon) — Lua port of
-- npc_aeranasAI in
-- src/server/scripts/Outland/zone_hellfire_peninsula.cpp (lines 43-133).
-- Entry (wowhead, no C++ NPC_ constant exists — the cthun/telestra
-- wowhead-only precedent, never guessed): 17085, via the wowwiki
-- Aeranas page's Wowhead external link (wowhead.com/?npc=17085).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat,
-- 4 OnDied, 9 OnDamageTaken, 5 OnSpawn, 23 OnReset.
-- Ported arms (C++-exact):
-- - Reset: RemoveFlag(UNIT_NPC_FLAGS, QUESTGIVER) + SetFaction
--   (FRIENDLY) + Talk(SAY_SUMMON 0) + faction timer 8s ->
--   SetFaction(MONSTER_2). The faction timer runs pre-victim like
--   C++ (checked before UpdateVictim), so it is scheduled on
--   Reset, not on enter-combat.
-- - Combat pumps (enter-combat): shock 12553 victim-cast, 5s then
--   10s re-arm; enveloping winds 15535 victim-cast, 9s then 25s
--   re-arm. CastSpell(nil, spell) = victim-cast (in-tree convention).
-- - 30% arm (C++ UpdateAI HealthBelowPct, post-damage read via
--   event 9 with the nefarian postDamage > 0 guard, so it never
--   fires on the killing blow): SetFaction(FRIENDLY) + SetFlag
--   (QUESTGIVER) + RemoveAllAuras + CombatStop(true) + Talk
--   (SAY_FREE 1) + combat pumps cancelled. Fires once per
--   engagement (freed latch, cleared on Reset).
-- Unmodeled (documented-only, no bridges):
-- - EngagementOver is the inline tail of the ported 30% arm
--   (zone_hellfire_peninsula.cpp calls it right after CombatStop):
--   it only flips _isEngaged + AtDisengage (ATTACK_PLAYER clear,
--   tapped-flag restore — no Go model, no effect for this
--   non-pet creature). The disengage core (InCombat=false,
--   threat clear, target drop) is the ported CombatStop(true)
--   above, so there is no separate missing arm; the friendly
--   faction flip keeps the creature from re-engaging.
-- - The C++ faction-timer quirk: if the 30% arm fires before the
--   8s faction timer, C++ still flips the faction to MONSTER_2
--   when it expires. The Lua timer is left running likewise, so
--   the quirk is preserved, not papered over.

local ENTRY_AERANAS = 17085

local SAY_SUMMON = 0
local SAY_FREE = 1

local SPELL_SHOCK = 12553
local SPELL_ENVELOPING_WINDS = 15535

local FACTION_FRIENDLY = 35
local FACTION_MONSTER_2 = 16

local UNIT_NPC_FLAGS_FIELD = 82
local UNIT_NPC_FLAG_QUESTGIVER = 2

local timers = {}
local freed = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

local function cancelKeys(guid, keys)
    local per = timers[guid]
    if per then
        for _, key in ipairs(keys) do
            if per[key] then
                RemoveEventById(per[key])
                per[key] = nil
            end
        end
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

local function shockPump(creature, guid)
    creature:CastSpell(nil, SPELL_SHOCK)
    schedule(guid, "shock", 10000, function()
        shockPump(creature, guid)
    end)
end

local function windsPump(creature, guid)
    creature:CastSpell(nil, SPELL_ENVELOPING_WINDS)
    schedule(guid, "winds", 25000, function()
        windsPump(creature, guid)
    end)
end

local function aeranasReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    freed[guid] = nil
    creature:RemoveFlag(UNIT_NPC_FLAGS_FIELD, UNIT_NPC_FLAG_QUESTGIVER)
    creature:SetFaction(FACTION_FRIENDLY)
    creature:Talk(SAY_SUMMON)
    schedule(guid, "faction", 8000, function()
        creature:SetFaction(FACTION_MONSTER_2)
    end)
end

local function aeranasEnterCombat(event, creature)
    local guid = creature:GetGUID()
    schedule(guid, "shock", 5000, function()
        shockPump(creature, guid)
    end)
    schedule(guid, "winds", 9000, function()
        windsPump(creature, guid)
    end)
end

local function aeranasLeaveCombat(event, creature)
    cancelKeys(creature:GetGUID(), { "shock", "winds" })
end

local function aeranasDied(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    freed[guid] = nil
end

local function aeranasDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local health = creature:GetHealth()
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 or health == 0 or freed[guid] then
        return false, damage
    end
    local postHealth = health - damage
    if postHealth <= 0 then
        return false, damage
    end
    if postHealth < math.floor(maxHealth * 30 / 100) then
        freed[guid] = true
        cancelKeys(guid, { "shock", "winds" })
        creature:SetFaction(FACTION_FRIENDLY)
        creature:SetFlag(UNIT_NPC_FLAGS_FIELD, UNIT_NPC_FLAG_QUESTGIVER)
        creature:RemoveAllAuras()
        creature:CombatStop(true)
        creature:AttackStop()
        creature:Talk(SAY_FREE)
    end
    return false, damage
end

RegisterCreatureEvent(ENTRY_AERANAS, 5, aeranasReset)
RegisterCreatureEvent(ENTRY_AERANAS, 23, aeranasReset)
RegisterCreatureEvent(ENTRY_AERANAS, 1, aeranasEnterCombat)
RegisterCreatureEvent(ENTRY_AERANAS, 2, aeranasLeaveCombat)
RegisterCreatureEvent(ENTRY_AERANAS, 4, aeranasDied)
RegisterCreatureEvent(ENTRY_AERANAS, 9, aeranasDamageTaken)
