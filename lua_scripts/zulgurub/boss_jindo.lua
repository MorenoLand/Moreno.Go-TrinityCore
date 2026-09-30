-- Jin'do the Hexxer (Zul'Gurub) — Lua port of
-- src/server/scripts/EasternKingdoms/ZulGurub/boss_jindo.cpp
-- (boss_jindoAI + npc_healing_wardAI + npc_shade_of_jindoAI);
-- zulgurub.h:43 (DATA_JINDO = 7), :53 (NPC_JINDO_THE_HEXXER = 11380),
-- :55 (NPC_SHADE_OF_JINDO = 14986), :56 (NPC_SACRIFICED_TROLL = 14826).
-- Creature entries: 11380 Jin'do the Hexxer (C++ ScriptName
-- "boss_jindo" per AddSC_boss_jindo); 14986 Shade of Jindo (C++
-- ScriptName "npc_shade_of_jindo"); 14987 Powerful Healing Ward (C++
-- ScriptName "npc_healing_ward"; zulgurub.h defines no NPC constant —
-- entry verified against classic databases: wowhead/classic spell
-- 24309 "Powerful Healing Ward" summons NPC 14987; classicdb.ch /
-- tauri.hu list NPC 14987 as "Powerful Healing Ward" in Zul'Gurub).
-- Jin'do is a boss, so 11380 is registered via RegisterLuaBoss in
-- engine/world/boss_ai.go; the ward and the shade are trash/adds and
-- are registered directly with RegisterCreatureEvent.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Fight shape (Jin'do): JustEngagedWith Talks SAY_AGGRO=1 and arms
-- brain wash totem 24262 20s then {18s,26s} (non-triggered self-cast,
-- C++-exact DoCast(me)) / powerful healing ward 24309 15s then
-- {14s,20s} (non-triggered self-cast, C++-exact DoCast(me); the ward
-- itself never spawns — no summon model) / hex 24053 8s then
-- {12s,20s} on the victim (triggered; the -80% threat cut has no
-- threat-model bridge — skipped, cast still lands) / delusions of
-- jindo 10s then {4s,12s} on a random alive player within 100 yd
-- (triggered 24308 shade-of-jindo summon, then non-triggered 24306
-- delusion curse, C++-exact order; nil-target ticks cast nothing but
-- keep the schedule) / teleport 5s then {15s,23s} on a random alive
-- player within 100 yd (the whole effect has no bridge — player
-- teleport, the -100% threat wipe, and the 10 sacrificed-troll
-- 14826 summons are all skipped — only the re-arm is modeled).
-- Ward 14987: the C++ AI ticks its heal timer even out of combat
-- (2s first, then every 3s: DoCast(GetCreature(DATA_JINDO), 24311));
-- with no instance-script bridge the Jin'do lookup has no bearer, so
-- only the 3s re-arm is modeled (hexlord friendly-scan convention).
-- Shade 14986: OnEnterCombat self-casts invisibility 24307 (triggered,
-- C++-exact Reset cast) and arms shadow shock 19460 1s then every 2s
-- on the victim (non-triggered DoCastVictim, C++-exact); nil-victim
-- ticks cast nothing but keep the schedule.
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING gate (skip the
-- rest of the event queue while casting) has no UNIT_STATE bridge —
-- timers fire unconditionally (jeklik convention); no instance-script
-- model — boss admission via the luaBossAI shim (_Reset/_JustDied/
-- BossAI::JustEngagedWith and the GetZulGurubAI bookkeeping arms
-- skipped); no summon model — the brain-wash totem, the healing ward,
-- the shades, and the sacrificed trolls never spawn (the ward and
-- shade are still registered so any that exist fight/react
-- correctly); the ward constructor's REACT_PASSIVE arm and its
-- AttackStart no-op have no bridge (the ward never spawns in this
-- build); no teleport bridge — DoTeleportPlayer and the formation
-- coordinates are not used; no threat model — the hex -80% and
-- teleport -100% ModifyThreatByPercent arms are skipped; no Lua spawn
-- event exists, so the ward's always-on UpdateAI heal loop is
-- approximated with the combat-lifecycle events (1/2/4/23).

local ENTRY_JINDO = 11380
local ENTRY_SHADE = 14986
local ENTRY_WARD = 14987

local SPELL_BRAIN_WASH_TOTEM = 24262
local SPELL_HEALING_WARD = 24309
local SPELL_HEX = 24053
local SPELL_SHADE_OF_JINDO = 24308
local SPELL_DELUSIONS_OF_JINDO = 24306
local SPELL_HEAL = 24311
local SPELL_SHADOWSHOCK = 19460
local SPELL_INVISIBLE = 24307

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
    per[key] = CreateLuaEvent(fn, delay)
end

local function alivePlayersInInstance(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            found[#found + 1] = p
        end
    end
    return found
end

-- C++ SelectTarget(Random, 0, 100.0f, true): random alive player in
-- range, nil when none qualify.
local function randomPlayer100(creature)
    local candidates = {}
    for _, p in ipairs(alivePlayersInInstance(creature)) do
        if creature:GetDistance(p) <= 100 then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EVENT_BRAIN_WASH_TOTEM: non-triggered DoCast(me, 24262);
-- re-arm {18s,26s}.
local function onBrainWash(creature, guid)
    creature:CastSpell(creature, SPELL_BRAIN_WASH_TOTEM)
    schedule(guid, "brainwash", math.random(18000, 26000), function()
        onBrainWash(creature, guid)
    end)
end

-- C++ EVENT_POWERFULL_HEALING_WARD: non-triggered DoCast(me, 24309);
-- re-arm {14s,20s}.
local function onHealingWard(creature, guid)
    creature:CastSpell(creature, SPELL_HEALING_WARD)
    schedule(guid, "ward", math.random(14000, 20000), function()
        onHealingWard(creature, guid)
    end)
end

-- C++ EVENT_HEX: triggered DoCast(victim, 24053) (nil-victim tick casts
-- nothing); the -80% threat arm has no bridge — skipped.
-- Re-arm {12s,20s} unconditional (C++ schedules outside the target
-- gate).
local function onHex(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_HEX, true)
    end
    schedule(guid, "hex", math.random(12000, 20000), function()
        onHex(creature, guid)
    end)
end

-- C++ EVENT_DELUSIONS_OF_JINDO: random 100-yd player -> triggered
-- DoCast(target, 24308), then non-triggered DoCast(target, 24306);
-- nil-target tick casts nothing; re-arm {4s,12s} unconditional.
local function onDelusions(creature, guid)
    local target = randomPlayer100(creature)
    if target then
        creature:CastSpell(target, SPELL_SHADE_OF_JINDO, true)
        creature:CastSpell(target, SPELL_DELUSIONS_OF_JINDO)
    end
    schedule(guid, "delusions", math.random(4000, 12000), function()
        onDelusions(creature, guid)
    end)
end

-- C++ EVENT_TELEPORT: teleport + threat wipe + 10 troll summons all
-- have no bridge (no teleport, no threat, no summon models) — only the
-- {15s,23s} re-arm is modeled.
local function onTeleport(creature, guid)
    schedule(guid, "teleport", math.random(15000, 23000), function()
        onTeleport(creature, guid)
    end)
end

local function jindoResetState(guid)
    cancelTimers(guid)
end

local function jindoEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(1)
    schedule(guid, "brainwash", 20000, function()
        onBrainWash(creature, guid)
    end)
    schedule(guid, "ward", 15000, function()
        onHealingWard(creature, guid)
    end)
    schedule(guid, "hex", 8000, function()
        onHex(creature, guid)
    end)
    schedule(guid, "delusions", 10000, function()
        onDelusions(creature, guid)
    end)
    schedule(guid, "teleport", 5000, function()
        onTeleport(creature, guid)
    end)
end

local function jindoLeaveCombat(event, creature)
    jindoResetState(creature:GetGUID())
end

local function jindoDied(event, creature, killer)
    jindoResetState(creature:GetGUID())
end

local function jindoReset(event, creature)
    -- C++ Reset's _Reset bookkeeping has no bridge (no instance-script
    -- model) — only the timer cancel is modeled.
    jindoResetState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_JINDO, 1, jindoEnterCombat)
RegisterCreatureEvent(ENTRY_JINDO, 2, jindoLeaveCombat)
RegisterCreatureEvent(ENTRY_JINDO, 4, jindoDied)
RegisterCreatureEvent(ENTRY_JINDO, 23, jindoReset)

-- npc_shade_of_jindo (14986): C++ Reset self-casts 24307 (triggered)
-- and arms shadow shock 19460 (1s, then every 2s, non-triggered
-- DoCastVictim). Melee is engine-driven.
local function onShadowShock(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SHADOWSHOCK)
    end
    schedule(guid, "shock", 2000, function()
        onShadowShock(creature, guid)
    end)
end

local function shadeEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:CastSpell(creature, SPELL_INVISIBLE, true)
    schedule(guid, "shock", 1000, function()
        onShadowShock(creature, guid)
    end)
end

local function shadeLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function shadeDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function shadeReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_SHADE, 1, shadeEnterCombat)
RegisterCreatureEvent(ENTRY_SHADE, 2, shadeLeaveCombat)
RegisterCreatureEvent(ENTRY_SHADE, 4, shadeDied)
RegisterCreatureEvent(ENTRY_SHADE, 23, shadeReset)

-- npc_healing_ward (14987): C++ heals GetCreature(DATA_JINDO) with
-- 24311 every 3s (first tick 2s). The instance lookup has no bridge —
-- only the 3s re-arm cycle is modeled (cast skipped). The ward never
-- spawns in this build (no summon model), so these hooks exist for
-- completeness; the combat-lifecycle events approximate the C++
-- always-on UpdateAI loop (no spawn-event bridge).
local function onWardHeal(creature, guid)
    schedule(guid, "heal", 3000, function()
        onWardHeal(creature, guid)
    end)
end

local function wardEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "heal", 2000, function()
        onWardHeal(creature, guid)
    end)
end

local function wardLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function wardDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function wardReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_WARD, 1, wardEnterCombat)
RegisterCreatureEvent(ENTRY_WARD, 2, wardLeaveCombat)
RegisterCreatureEvent(ENTRY_WARD, 4, wardDied)
RegisterCreatureEvent(ENTRY_WARD, 23, wardReset)
