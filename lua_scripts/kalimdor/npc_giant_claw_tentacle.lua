-- Temple of Ahn'Qiraj: Giant Claw Tentacle — Lua port of
-- src/server/scripts/Kalimdor/TempleOfAhnQiraj/boss_cthun.cpp
-- (npc_giant_claw_tentacleAI : public ScriptedAI; GetAI via
-- GetAQ40AI<giant_claw_tentacleAI>(creature); registered in
-- AddSC_boss_cthun()).
-- Entry: 15728 (worker-verified from C++ NPC_GIANT_CLAW_TENTACLE).
-- Ported: Ground Rupture 26139 on victim (500ms init -> 30s
-- repeat); Thrash 3391 on victim (5s init -> 10s repeat per C++
-- ThrashTimer reset, kurinnaxx convention); Hamstring 26141 on
-- victim (2s init -> 10s repeat per C++ HamstringTimer reset —
-- the giant claw's repeat is 10s, unlike the claw tentacle's
-- 5s). The EvadeTimer leg is PORTED 2026-10-26 (onEvadeCheck: 1s
-- out-of-melee-range poll, pause semantics; hide/teleport/portal/
-- EngageWithTarget, acid-pick skip). The ctor-summoned portal's
-- JustDied/old-portal KillSelf legs stay out (no GUID->creature
-- bridge; Lua never summoned the ctor portal).
local ENTRY = 15728
local ENTRY_GIANT_PORTAL = 15910
local SPELL_GROUND_RUPTURE = 26139
local SPELL_THRASH = 3391
local SPELL_HAMSTRING = 26141
local SPELL_DIGESTIVE_ACID = 26476
local REACT_PASSIVE = 0

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

local function onGroundRupture(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_GROUND_RUPTURE)
    end
    schedule(guid, "rupture", 30000, function()
        onGroundRupture(creature, guid)
    end)
end

local function onThrash(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_THRASH)
    end
    schedule(guid, "thrash", 10000, function()
        onThrash(creature, guid)
    end)
end

local function onHamstring(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_HAMSTRING)
    end
    schedule(guid, "hamstring", 10000, function()
        onHamstring(creature, guid)
    end)
end

-- C++ EvadeTimer leg (boss_cthun.cpp:1137-1175): same shape as the
-- claw tentacle leg (see npc_claw_tentacle.lua), plus the Thrash
-- 5000ms re-arm on fire and the giant portal 15910 in place of the
-- small portal; rupture 500ms / hamstring 2000ms re-arms as in C++.
local evadeElapsed = {}

local function onEvadeCheck(creature, guid)
    local victim = creature:GetVictim()
    if victim and not creature:IsWithinMeleeRange(victim) then
        local elapsed = (evadeElapsed[guid] or 0) + 1000
        if elapsed >= 5000 then
            evadeElapsed[guid] = 0
            creature:SetVisible(false)
            local candidates = {}
            for _, unit in ipairs(creature:GetAITargets()) do
                if not unit:IsDead() then
                    candidates[#candidates + 1] = unit
                end
            end
            local target = nil
            if #candidates > 0 then
                target = candidates[math.random(#candidates)]
            end
            if not target then
                creature:Kill()
                return
            end
            if not target:HasAura(SPELL_DIGESTIVE_ACID) then
                creature:Teleport(target:GetX(), target:GetY(), target:GetZ(), 0)
                local portal = creature:SummonCreature(ENTRY_GIANT_PORTAL,
                    creature:GetX(), creature:GetY(), creature:GetZ(), 0, 4)
                if portal then
                    portal:SetReactState(REACT_PASSIVE)
                end
                schedule(guid, "rupture", 500, function()
                    onGroundRupture(creature, guid)
                end)
                schedule(guid, "thrash", 5000, function()
                    onThrash(creature, guid)
                end)
                schedule(guid, "hamstring", 2000, function()
                    onHamstring(creature, guid)
                end)
                creature:EngageWithTarget(target)
            end
            creature:SetVisible(true)
        else
            evadeElapsed[guid] = elapsed
        end
    end
    if timers[guid] then
        schedule(guid, "evade", 1000, function()
            onEvadeCheck(creature, guid)
        end)
    end
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    evadeElapsed[guid] = 0
    schedule(guid, "rupture", 500, function()
        onGroundRupture(creature, guid)
    end)
    schedule(guid, "thrash", 5000, function()
        onThrash(creature, guid)
    end)
    schedule(guid, "hamstring", 2000, function()
        onHamstring(creature, guid)
    end)
    schedule(guid, "evade", 1000, function()
        onEvadeCheck(creature, guid)
    end)
    -- boss_cthun.cpp:1125: npc_giant_claw_tentacleAI JustEngagedWith
    -- tail — me-form DoZoneInCombat() on the live bridge
    -- (algalon/mandokir precedent; 2026-10-23 re-triage). C++ runtime
    -- order: Reset arms (the timers above) run before the engage arm,
    -- so it sits last.
    creature:DoZoneInCombat()
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
    evadeElapsed[creature:GetGUID()] = nil
end

local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    evadeElapsed[creature:GetGUID()] = nil
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 4, onDied)
