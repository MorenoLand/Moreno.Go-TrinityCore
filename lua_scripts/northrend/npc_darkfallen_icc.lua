-- Darkfallen (Icecrown Citadel) — Lua port of DarkFallenAI +
-- npc_darkfallen_blood_knight / npc_darkfallen_noble /
-- npc_darkfallen_archmage / npc_darkfallen_advisor /
-- npc_darkfallen_tactician + npc_vampiric_fiend
-- (src/server/scripts/Northrend/IcecrownCitadel/icecrown_citadel.cpp:776-1024).
-- Registration is entry-keyed (icecrown_citadel.cpp:1602-1608) for:
local NPC_DARKFALLEN_BLOOD_KNIGHT = 37595
local NPC_DARKFALLEN_NOBLE = 37663
local NPC_DARKFALLEN_ARCHMAGE = 37664
local NPC_DARKFALLEN_ADVISOR = 37571
local NPC_DARKFALLEN_TACTICIAN = 37666
local NPC_VAMPIRIC_FIEND = 37901

local SPELL_POLYMORPH_ALLY = 72106

-- Blood knight
local SPELL_VAMPIRIC_AURA = 71736
local SPELL_UNHOLY_STRIKE = 70437
local SPELL_BLOOD_MIRROR = 70450
-- Noble
local SPELL_SHADOW_BOLT = 72960 -- AttackSpellId (DoSpellAttackIfReady — unmodeled, illidari-council precedent)
local SPELL_CHAINS_OF_SHADOW = 72960
-- Archmage
local SPELL_FIREBALL = 70409 -- AttackSpellId (DoSpellAttackIfReady — unmodeled, illidari-council precedent)
local SPELL_AMPLIFY_MAGIC = 70408
local SPELL_BLAST_WAVE = 70407
local SPELL_POLYMORPH = 70410
-- Advisor
local SPELL_LICH_SLAP = 72057
local SPELL_SHROUD_OF_SPELL_WARDING = 72066
-- Tactician
local SPELL_SHADOWSTEP = 70431
local SPELL_BLOOD_SAP = 70432
-- Vampiric fiend
local SPELL_DISEASE_CLOUD = 41290
local SPELL_LEECHING_ROOT = 70671

-- C++ DarkFallensEmotes (icecrown_citadel.cpp:652-659) — SharedDefines.h ids.
local DARKFALLEN_EMOTES = {1, 5, 6, 11, 273, 274}

local DARKFALLEN_ENTRIES = {
    NPC_DARKFALLEN_BLOOD_KNIGHT,
    NPC_DARKFALLEN_NOBLE,
    NPC_DARKFALLEN_ARCHMAGE,
    NPC_DARKFALLEN_ADVISOR,
    NPC_DARKFALLEN_TACTICIAN,
}

-- GUID-keyed CreateLuaEvent timers, C++ Reset/JustEngagedWith/JustDied/
-- EnterEvadeMode lifecycle modeled on events 5/1/2/4/23 (engaging cancels
-- the out-of-combat emote machine, per DarkFallenAI::JustEngagedWith).
local timers = {}
local emoteActive = {} -- per-GUID: out-of-combat emote machine latch (C++ IsDoingEmotes)

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

local function schedule(guid, key, delay, repeats, fn)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay, repeats or 0)
end

-- C++ DarkFallenAI::Reset out-of-combat emote machine
-- (icecrown_citadel.cpp:790-812): initial {1s,10s}, then every {15s,30s}:
-- 20% chance to cast SPELL_POLYMORPH_ALLY on a random fellow darkfallen
-- within 10.0f (ICCOrbControllerMinionSearch: alive, no-waypoint, the 5
-- darkfallen entries — the HasUnitState(CASTING) leg is vacuous in Go,
-- fire-and-forget creature casts; the waypoint check has no bridge, DB
-- trash never paths), then 1s later a random emote from
-- DarkFallensEmotes. IsDoingEmotes is true unless the creature has a
-- waypoint path (no bridge; always true in Lua).
local function emoteTick(creature, guid)
    if not emoteActive[guid] then
        return
    end
    if math.random(100) <= 20 then
        local friends = {}
        for _, entry in ipairs(DARKFALLEN_ENTRIES) do
            local near = creature:GetCreaturesInGrid(entry, 10.0)
            for _, c in pairs(near) do
                if c:IsAlive() and c:GetGUID() ~= guid then
                    friends[#friends + 1] = c
                end
            end
        end
        if #friends > 0 then
            creature:CastSpell(friends[math.random(#friends)], SPELL_POLYMORPH_ALLY)
        end
    end
    CreateLuaEvent(function()
        if emoteActive[guid] then
            creature:PerformEmote(DARKFALLEN_EMOTES[math.random(#DARKFALLEN_EMOTES)])
        end
    end, 1000, 0)
    schedule(guid, "emote", {15000, 30000}, 0, function()
        emoteTick(creature, guid)
    end)
end

local function startEmoteMachine(creature)
    local guid = creature:GetGUID()
    emoteActive[guid] = true
    schedule(guid, "emote", {1000, 10000}, 0, function()
        emoteTick(creature, guid)
    end)
end

local function stopEmoteMachine(guid)
    emoteActive[guid] = false
    cancelTimers(guid)
end

-- SelectTarget(Random, 0, 0.0f, true, false[, -auraId]) twin: random alive
-- player in the map/instance, optionally excluding auras (the
-- maiden-of-grief randomPlayerTarget idiom).
local function randomDarkfallenTarget(creature, excludeAura)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local candidates = {}
    for _, plr in ipairs(GetPlayersInWorld()) do
        if plr:GetMapId() == mapId and plr:GetInstanceId() == instanceId
                and not plr:IsDead()
                and (excludeAura == nil or not plr:HasAura(excludeAura)) then
            candidates[#candidates + 1] = plr
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ JustEngagedWith for each subclass (:815-822): cancel the emote
-- machine + class ScheduleSpells; the TriggerGuid ACTION_COMBAT signal to
-- the orb controller has no cross-creature AI bridge — documented.
local function darkfallenEngage(entry, creature)
    local guid = creature:GetGUID()
    stopEmoteMachine(guid)
    if entry == NPC_DARKFALLEN_BLOOD_KNIGHT then
        -- C++ npc_darkfallen_blood_knight::ScheduleSpells (:864-881):
        -- 500ms one-shot DoCastSelf(VAMPIRIC_AURA 71736); 8s
        -- DoCastVictim(UNHOLY_STRIKE 70437) then {8s,9s}; 6s
        -- DoCastSelf(BLOOD_MIRROR 70450) then 34s.
        schedule(guid, "aura", 500, 0, function()
            creature:CastSpell(creature, SPELL_VAMPIRIC_AURA)
        end)
        schedule(guid, "unholy", 8000, 0, function()
            local function unholyTick()
                creature:CastSpell(nil, SPELL_UNHOLY_STRIKE)
                schedule(guid, "unholy", {8000, 9000}, 0, unholyTick)
            end
            unholyTick()
        end)
        schedule(guid, "mirror", 6000, 0, function()
            local function mirrorTick()
                creature:CastSpell(creature, SPELL_BLOOD_MIRROR)
                schedule(guid, "mirror", 34000, 0, mirrorTick)
            end
            mirrorTick()
        end)
    elseif entry == NPC_DARKFALLEN_NOBLE then
        -- C++ npc_darkfallen_noble::ScheduleSpells (:887-903):
        -- AttackSpellId = SHADOW_BOLT (DoSpellAttackIfReady — unmodeled,
        -- illidari-council precedent, so melee is disabled C++-exact);
        -- 500ms one-shot chains of shadow on a random target without the
        -- 72960 aura (the -SPELL_CHAINS_OF_SHADOW selector leg).
        creature:SetMeleeAttack(false)
        schedule(guid, "chains", 500, 0, function()
            local target = randomDarkfallenTarget(creature, SPELL_CHAINS_OF_SHADOW)
            if target then
                creature:CastSpell(target, SPELL_CHAINS_OF_SHADOW)
            end
        end)
        -- The 11s/30s vampiric-fiend summon (SummonCreature + AttackStart)
        -- has no summon bridge — documented.
    elseif entry == NPC_DARKFALLEN_ARCHMAGE then
        -- C++ npc_darkfallen_archmage::ScheduleSpells (:947-970):
        -- AttackSpellId = FIREBALL (DoSpellAttackIfReady — unmodeled,
        -- melee disabled C++-exact); 1s amplify magic on a random target
        -- then {15s,24s}; 10s self blast wave then {25s,30s}; 17s
        -- polymorph on a random target without the 70410 aura then
        -- {25s,35s}.
        creature:SetMeleeAttack(false)
        schedule(guid, "amplify", 1000, 0, function()
            local function amplifyTick()
                local target = randomDarkfallenTarget(creature, nil)
                if target then
                    creature:CastSpell(target, SPELL_AMPLIFY_MAGIC)
                end
                schedule(guid, "amplify", {15000, 24000}, 0, amplifyTick)
            end
            amplifyTick()
        end)
        schedule(guid, "blast", 10000, 0, function()
            local function blastTick()
                creature:CastSpell(creature, SPELL_BLAST_WAVE)
                schedule(guid, "blast", {25000, 30000}, 0, blastTick)
            end
            blastTick()
        end)
        schedule(guid, "poly", 17000, 0, function()
            local function polyTick()
                local target = randomDarkfallenTarget(creature, SPELL_POLYMORPH)
                if target then
                    creature:CastSpell(target, SPELL_POLYMORPH)
                end
                schedule(guid, "poly", {25000, 35000}, 0, polyTick)
            end
            polyTick()
        end)
    elseif entry == NPC_DARKFALLEN_ADVISOR then
        -- C++ npc_darkfallen_advisor::ScheduleSpells (:974-990): 8s
        -- DoCastVictim(LICH_SLAP 72057) then 12s; 50s shroud of spell
        -- warding on the lowest-HP friendly within 40.0f then {20s,25s}
        -- (DoSelectLowestHpFriendly has no bridge — approximated by the
        -- entry sweep of darkfallen allies, all same-faction).
        schedule(guid, "slap", 8000, 0, function()
            local function slapTick()
                creature:CastSpell(nil, SPELL_LICH_SLAP)
                schedule(guid, "slap", 12000, 0, slapTick)
            end
            slapTick()
        end)
        schedule(guid, "shroud", 50000, 0, function()
            local function shroudTick()
                local best, bestPct = nil, 100
                for _, e in ipairs(DARKFALLEN_ENTRIES) do
                    for _, c in pairs(creature:GetCreaturesInGrid(e, 40.0)) do
                        if c:IsAlive() then
                            local pct = c:GetHealthPct()
                            if pct < bestPct then
                                best, bestPct = c, pct
                            end
                        end
                    end
                end
                if best then
                    creature:CastSpell(best, SPELL_SHROUD_OF_SPELL_WARDING)
                end
                schedule(guid, "shroud", {20000, 25000}, 0, shroudTick)
            end
            shroudTick()
        end)
    elseif entry == NPC_DARKFALLEN_TACTICIAN then
        -- C++ npc_darkfallen_tactician::ScheduleSpells (:994-1013): 8s
        -- DoCastVictim(UNHOLY_STRIKE) then {8s,11s}; 10s shadowstep +
        -- blood sap on a random target then 20s.
        schedule(guid, "unholy", 8000, 0, function()
            local function unholyTick()
                creature:CastSpell(nil, SPELL_UNHOLY_STRIKE)
                schedule(guid, "unholy", {8000, 11000}, 0, unholyTick)
            end
            unholyTick()
        end)
        schedule(guid, "shadowstep", 10000, 0, function()
            local function stepTick()
                local target = randomDarkfallenTarget(creature, nil)
                if target then
                    creature:CastSpell(target, SPELL_SHADOWSTEP)
                    creature:CastSpell(target, SPELL_BLOOD_SAP)
                end
                schedule(guid, "shadowstep", 20000, 0, stepTick)
            end
            stepTick()
        end)
    end
end

local function darkfallenLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    emoteActive[guid] = false
    -- C++ EnterEvadeMode legs (ACTION_EVADE signal, orb GO-flag leg,
    -- _EnterEvadeMode) ride unbridged cross-AI/GO machines; the emote
    -- machine re-arms on Reset — modeled here via ev2 (teron evade
    -- precedent) since Reset fires on evade in C++.
    startEmoteMachine(creature)
end

local function darkfallenDied(event, creature, killer)
    stopEmoteMachine(creature:GetGUID())
end

local function darkfallenReset(event, creature)
    if not creature:IsAlive() then
        return
    end
    startEmoteMachine(creature)
end

-- Vampiric fiend (npc_vampiric_fiend, icecrown_citadel.cpp:905-931):
-- JustEngagedWith -> DoCastSelf(DISEASE_CLOUD 41290), 9s one-shot
-- DoCastVictim(LEECHING_ROOT 70671), 38s one-shot DespawnOrUnsummon;
-- EnterEvadeMode -> cancel + despawn (ev2 per the teron precedent).
local function fiendEngage(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:CastSpell(creature, SPELL_DISEASE_CLOUD)
    schedule(guid, "root", 9000, 0, function()
        creature:CastSpell(nil, SPELL_LEECHING_ROOT)
    end)
    schedule(guid, "despawn", 38000, 0, function()
        creature:Despawn()
    end)
end

local function fiendLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Despawn()
end

local function fiendDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

for _, entry in ipairs(DARKFALLEN_ENTRIES) do
    RegisterCreatureEvent(entry, 1, function(event, creature, target)
        darkfallenEngage(entry, creature)
    end)
    RegisterCreatureEvent(entry, 2, darkfallenLeaveCombat)
    RegisterCreatureEvent(entry, 4, darkfallenDied)
    RegisterCreatureEvent(entry, 5, function(event, creature)
        darkfallenReset(event, creature)
    end)
    RegisterCreatureEvent(entry, 23, darkfallenReset)
end

RegisterCreatureEvent(NPC_VAMPIRIC_FIEND, 1, fiendEngage)
RegisterCreatureEvent(NPC_VAMPIRIC_FIEND, 2, fiendLeaveCombat)
RegisterCreatureEvent(NPC_VAMPIRIC_FIEND, 4, fiendDied)
