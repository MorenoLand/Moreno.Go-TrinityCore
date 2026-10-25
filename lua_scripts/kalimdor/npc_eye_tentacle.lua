-- Temple of Ahn'Qiraj: Eye Tentacle — Lua port of
-- src/server/scripts/Kalimdor/TempleOfAhnQiraj/boss_cthun.cpp
-- (npc_eye_tentacleAI : public ScriptedAI; GetAI via
-- GetAQ40AI<eye_tentacleAI>(creature); registered in
-- AddSC_boss_cthun()).
-- Entry: 15726 (worker-verified from C++ NPC_EYE_TENTACLE).
-- Ported: Mind Flay 26143 on a random threat-list any-unit alive
-- (500ms init -> 10s repeat; C++ SelectTarget Random,0 defaults —
-- the old instance-player pool wrongly excluded pets/totems and
-- included unengaged bystanders).
-- The 35s KillSelf is PORTED (boss_cthun.cpp:897,939-942 — UpdateAI
-- KillSelfTimer, victim-gated in C++; the Lua one-shot runs only
-- while in combat since ev2/ev4 cancel it, kj fiend precedent).
-- The portal summon/JustDied cross-kill legs stay out (no GUID->
-- creature bridge — documented, not wired). The JustEngagedWith DoZoneInCombat leg PORTED
-- 2026-10-23 (creature:DoZoneInCombat bridge live,
-- engine/world/lua_creature_events.go).
local ENTRY = 15726
local SPELL_MIND_FLAY = 26143

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

-- C++ MindflayTimer (boss_cthun.cpp:948): SelectTarget Random,0
-- defaults — threat-list any-unit alive — then the Digestive Acid
-- 26476 gate applies to the PICK (no cast that cycle when the pick
-- carries it); the old pool pre-filtered the aura out and re-picked,
-- so an all-acid cycle wrongly cast instead of skipping.
local function onMindFlay(creature, guid)
    local candidates = {}
    for _, p in ipairs(creature:GetAITargets()) do
        if not p:IsDead() then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates > 0 then
        local target = candidates[math.random(#candidates)]
        if not target:HasAura(26476) then
            creature:CastSpell(target, SPELL_MIND_FLAY)
        end
    end
    local per = timers[guid]
    if per then
        per["flay"] = CreateLuaEvent(function()
            onMindFlay(creature, guid)
        end, 10000)
    end
end

local function onEnterCombat(event, creature, target)
    creature:DoZoneInCombat() -- boss_cthun.cpp:927-930 (eye_tentacleAI::JustEngagedWith — sole arm; bridge live 2026-10-23)
    creature:SetMeleeAttack(false) -- boss_cthun.cpp:932: custom UpdateAI never calls DoMeleeAttackIfReady (cast-only in C++)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    timers[guid] = {}
    timers[guid]["flay"] = CreateLuaEvent(function()
        onMindFlay(creature, guid)
    end, 500)
    timers[guid]["killself"] = CreateLuaEvent(function()
        creature:Kill()
    end, 35000)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 4, onDied)
