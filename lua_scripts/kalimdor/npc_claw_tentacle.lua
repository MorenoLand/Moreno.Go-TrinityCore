-- Temple of Ahn'Qiraj: Claw Tentacle — Lua port of
-- src/server/scripts/Kalimdor/TempleOfAhnQiraj/boss_cthun.cpp
-- (npc_claw_tentacleAI : public ScriptedAI; GetAI via
-- GetAQ40AI<claw_tentacleAI>(creature); registered in
-- AddSC_boss_cthun()).
-- Entry: 15725 (worker-verified from C++ NPC_CLAW_TENTACLE).
-- Hamstring 26141 on victim (2s init -> 5s repeat per C++
-- HamstringTimer reset; the giant-claw port (C++ repeat 10s) is
-- a different AI — not copied here). The EvadeTimer leg is PORTED
-- 2026-10-26 (onEvadeCheck: 1s out-of-melee-range poll, pause
-- semantics; hide/teleport/portal/EngageWithTarget, acid-pick skip).
-- The ctor-summoned portal's JustDied/old-portal KillSelf legs stay
-- out (no GUID->creature bridge; Lua never summoned the ctor portal).
local ENTRY = 15725
local ENTRY_SMALL_PORTAL = 15904
local SPELL_GROUND_RUPTURE = 26139
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

local function onHamstring(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_HAMSTRING)
    end
    schedule(guid, "hamstring", 5000, function()
        onHamstring(creature, guid)
    end)
end

-- C++ EvadeTimer leg (boss_cthun.cpp:1016-1056): while out of melee
-- range the 5s timer ticks down (it PAUSES, never resets, back in
-- range); on fire the tentacle hides, picks a random threat-list
-- any-unit alive, kills itself on an empty pick, and — unless the pick
-- carries Digestive Acid 26476 — teleports onto the pick, summons a
-- small portal (15904, CORPSE_DESPAWN type 4, passive) at its new
-- position, re-arms rupture 500ms / hamstring 2000ms, and
-- AttackStart(pick) via EngageWithTarget; it unhides either way.
-- Lua form: a 1s poll accumulating out-of-range time (pause semantics
-- kept: in-range polls neither add nor reset); the acid-pick leg
-- keeps the elapsed count so the next poll re-fires, as C++ re-fires
-- every tick with an unreset timer. The ctor-summoned portal's
-- JustDied/old-portal KillSelf legs stay out (no GUID->creature
-- bridge; Lua never summoned the ctor portal).
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
                local portal = creature:SummonCreature(ENTRY_SMALL_PORTAL,
                    creature:GetX(), creature:GetY(), creature:GetZ(), 0, 4)
                if portal then
                    portal:SetReactState(REACT_PASSIVE)
                end
                schedule(guid, "rupture", 500, function()
                    onGroundRupture(creature, guid)
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
    schedule(guid, "hamstring", 2000, function()
        onHamstring(creature, guid)
    end)
    schedule(guid, "evade", 1000, function()
        onEvadeCheck(creature, guid)
    end)
    -- boss_cthun.cpp:1008: npc_claw_tentacleAI JustEngagedWith tail —
    -- me-form DoZoneInCombat() on the live bridge (algalon/mandokir
    -- precedent; 2026-10-23 re-triage). C++ runtime order: Reset arms
    -- (the timers above) run before the engage arm, so it sits last.
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
