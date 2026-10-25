-- Temple of Ahn'Qiraj: Giant Eye Tentacle — Lua port of
-- src/server/scripts/Kalimdor/TempleOfAhnQiraj/boss_cthun.cpp
-- (npc_giant_eye_tentacleAI : public ScriptedAI; GetAI via
-- GetAQ40AI<giant_eye_tentacleAI>(creature); registered in
-- AddSC_boss_cthun()).
-- Entry: 15334 (worker-verified from C++ NPC_GIANT_EYE_TENTACLE).
-- Ported: Green Beam 26134 on a random threat-list any-unit alive
-- (500ms init -> 2.1s repeat; C++ SelectTarget Random,0 defaults —
-- the Digestive Acid 26476 exclusion applies to the pick, not the
-- pool, per the C++ cast leg).
-- Summon/positioning legs have no bridge — documented, not wired.
local ENTRY = 15334
local SPELL_GREEN_BEAM = 26134

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

-- C++ BeamTimer (boss_cthun.cpp:1252): SelectTarget Random,0
-- defaults — threat-list any-unit alive — then the Digestive Acid
-- 26476 gate applies to the PICK (no cast that cycle when the pick
-- carries it); the old pool pre-filtered the aura out and re-picked.
local function onGreenBeam(creature, guid)
    local candidates = {}
    for _, p in ipairs(creature:GetAITargets()) do
        if not p:IsDead() then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates > 0 then
        local target = candidates[math.random(#candidates)]
        if not target:HasAura(26476) then
            creature:CastSpell(target, SPELL_GREEN_BEAM)
        end
    end
    local per = timers[guid]
    if per then
        per["beam"] = CreateLuaEvent(function()
            onGreenBeam(creature, guid)
        end, 2100)
    end
end

local function onEnterCombat(event, creature, target)
    creature:SetMeleeAttack(false) -- boss_cthun.cpp:1243: custom UpdateAI never calls DoMeleeAttackIfReady (cast-only in C++)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    timers[guid] = {}
    timers[guid]["beam"] = CreateLuaEvent(function()
        onGreenBeam(creature, guid)
    end, 500)
    -- boss_cthun.cpp:1240: npc_giant_eye_tentacleAI JustEngagedWith
    -- tail — me-form DoZoneInCombat() on the live bridge
    -- (algalon/mandokir precedent; 2026-10-23 re-triage). C++ runtime
    -- order: Reset arms (the beam timer above) run before the engage
    -- arm, so it sits last.
    creature:DoZoneInCombat()
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
