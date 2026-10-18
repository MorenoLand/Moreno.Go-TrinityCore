-- High Interrogator Gerstahn (Blackrock Depths, Detention Block) -- Lua
-- port of src/server/scripts/EasternKingdoms/BlackrockMountain/
-- BlackrockDepths/boss_high_interrogator_gerstahn.cpp
-- (boss_high_interrogator_gerstahnAI : public ScriptedAI). Boss-script
-- unit per eastern_kingdoms_script_loader.cpp order
-- (AddSC_boss_high_interrogator_gerstahn follows
-- AddSC_boss_general_angerforge).
-- Entry (no NPC_ constant in the C++ tree — the creature_template
-- ScriptName binding is DB-side; the BRD instance creatures enum in
-- instance_blackrock_depths.cpp names the neighbor NPC_EMPEROR 9019
-- but not Gerstahn): 9018 "High Interrogator Gerstahn", verified
-- against the classic NPC ID list (inven.co.kr/board/wow/49/34580:
-- "9018 High Interrogator Gerstahn"; 9019 is the C++-verified
-- NPC_EMPEROR) — same wowhead-level verification as the flamelash
-- 9156 precedent.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset (the C++ overrides no KilledUnit/DamageTaken/SpellHit).
-- Timers via CreateLuaEvent per-GUID (cancel on leave combat / died /
-- reset); melee is engine-driven in Go (creature combat tick), like
-- C++ DoMeleeAttackIfReady.
-- Ported arms (C++-exact, all non-triggered casts):
-- JustEngagedWith: EVENT_SHADOW_WORD_PAIN 4s->7s DoCast(target, 10894)
-- on SelectTarget(Random, 0, 100.0f, true) — random alive player
-- within 100.0f via the bridged threat list (GetAITargets, murmur
-- precedent) with the playerOnly filter honored (illidan
-- victim:GetObjectType() convention) and the 100.0f band via
-- GetDistance2d (maiden range convention).
-- EVENT_MANABURN 14s->10s DoCast(target, 10876) on the same random
-- player selector.
-- EVENT_PSYCHIC_SCREAM 32s->30s DoCastVictim(8122).
-- EVENT_SHADOWSHIELD 8s->25s DoCast(me, 22417) self-cast.
-- Deviations: no UNIT_STATE_CASTING gate in the C++ UpdateAI (maiden
-- precedent — timers fire unconditionally). If the random-target pool
-- is empty the cast is skipped but the event still re-arms (C++-exact:
-- the ScheduleEvent is outside the if (target) block).
-- OnLeaveCombat(2)/OnDied(4)/OnReset(23): cancel the timers (the C++
-- Reset() _events.Reset() has no other observable arms). No Talk
-- lines exist in the C++.

local SPELL_SHADOW_WORD_PAIN = 10894
local SPELL_MANABURN = 10876
local SPELL_PSYCHIC_SCREAM = 8122
local SPELL_SHADOWSHIELD = 22417

local ENTRY = 9018

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

-- C++ SelectTarget(SelectTargetMethod::Random, 0, 100.0f, true):
-- random alive player within 100.0f (murmur randomThreatTarget
-- precedent — GetAITargets is the bridged threat list).
local function randomPlayerTarget(creature, maxDist)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:GetObjectType() == "Player" and t:IsAlive() then
            if maxDist <= 0 or creature:GetDistance2d(t) <= maxDist then
                candidates[#candidates + 1] = t
            end
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

local function onShadowWordPain(creature, guid)
    local target = randomPlayerTarget(creature, 100.0)
    if target then
        creature:CastSpell(target, SPELL_SHADOW_WORD_PAIN)
    end
    schedule(guid, "shadowwordpain", 7000, function()
        onShadowWordPain(creature, guid)
    end)
end

local function onManaBurn(creature, guid)
    local target = randomPlayerTarget(creature, 100.0)
    if target then
        creature:CastSpell(target, SPELL_MANABURN)
    end
    schedule(guid, "manaburn", 10000, function()
        onManaBurn(creature, guid)
    end)
end

local function onPsychicScream(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_PSYCHIC_SCREAM)
    end
    schedule(guid, "psychicscream", 30000, function()
        onPsychicScream(creature, guid)
    end)
end

local function onShadowShield(creature, guid)
    creature:CastSpell(creature, SPELL_SHADOWSHIELD)
    schedule(guid, "shadowshield", 25000, function()
        onShadowShield(creature, guid)
    end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    -- boss_high_interrogator_gerstahn.cpp JustEngagedWith schedule.
    schedule(guid, "shadowwordpain", 4000, function()
        onShadowWordPain(creature, guid)
    end)
    schedule(guid, "manaburn", 14000, function()
        onManaBurn(creature, guid)
    end)
    schedule(guid, "psychicscream", 32000, function()
        onPsychicScream(creature, guid)
    end)
    schedule(guid, "shadowshield", 8000, function()
        onShadowShield(creature, guid)
    end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(_, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY, 4, onCombatEnd)
RegisterCreatureEvent(ENTRY, 23, onReset)
