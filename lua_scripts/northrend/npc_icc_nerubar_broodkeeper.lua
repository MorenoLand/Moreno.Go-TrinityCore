-- Icecrown Citadel: npc_icc_nerubar_broodkeeper — Lua port of
-- src/server/scripts/Northrend/IcecrownCitadel/icecrown_citadel.cpp
-- (npc_icc_nerubar_broodkeeper : public ScriptedAI, :1017-1114).
-- Entry: NPC_NERUBAR_BROODKEEPER 36725 (icecrown_citadel.h:152).
-- Ported: InitializeAI spawn arm (event 5), JustEngagedWith timers
-- (event 1), Reset/leave-combat/died timer cancel (events 23/2/4).
-- InitializeAI: SetDisableGravity(true) + EmoteState(420) port with
-- real effect (creature:SetDisableGravity, creature:EmoteState);
-- SetImmuneToAll(true) stays out (no bridge).
-- JustEngagedWith (:1035-1039): CRYPT_SCARABS {1s,3s} then {4s,10s},
-- DARK_MENDING 5s then {3s,10s}, WEB_WRAP 20s then {16s,20s}.
-- CRYPT_SCARABS is DoCastVictim -> creature:CastSpell(nil, spell).
-- WEB_WRAP is SelectTarget(Random, 0, 40.0f) -> random alive
-- threat-list target; the 40yd narrowing stays out (no creature
-- GetDistance bridge on the handler surface).
-- DARK_MENDING is MostHPPercentMissingInRange (GridNotifiers.h:834):
-- alive, in combat, non-hostile, within 40yd, health% in [1,75],
-- lowest health% wins. Lua has no generic grid enumeration, so the
-- sweep covers the broodkeeper's own entry (GetCreaturesInGrid
-- includes self, matching C++ self-eligibility); allied entries
-- outside it stay out. creature:IsHostileTo is bridged.
-- Casting guard: C++ UpdateAI returns while UNIT_STATE_CASTING
-- (due events fire on the next non-casting tick); each Lua tick
-- defers itself 500ms while creature:IsCasting() instead of firing.
-- Melee rides the engine combat tick (anub'arak precedent).
-- Documented-only: DoAction ACTION_NERUBAR_FALL (:1041-1056) —
-- no DoAction bridge, and its driver is instance SetData
-- (DATA_NERUBAR_BROODKEEPER_EVENT, at_icc_nerubar_broodkeeper),
-- so WEB_BEAM 69887, SetHomePosition, MoveLand and the
-- MovementInform EFFECT_MOTION_TYPE leg (gravity/immune/emote
-- clear) all stay out; the spawn gravity/emote legs therefore
-- persist in Lua where C++ would clear them after the fall.

local NPC_ICC_NERUBAR_BROODKEEPER = 36725

local SPELL_CRYPT_SCARABS = 70965
local SPELL_DARK_MENDING = 71020
local SPELL_WEB_WRAP = 70980

local EMOTE_STATE_CUSTOM_SPELL_03 = 420

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
    per[key] = CreateLuaEvent(fn, delay, 0)
end

local function broodkeeperSpawn(event, creature)
    creature:SetDisableGravity(true)
    creature:EmoteState(EMOTE_STATE_CUSTOM_SPELL_03)
end

local function scarabsTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "scarabs", 500, function()
            scarabsTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_CRYPT_SCARABS)
    schedule(guid, "scarabs", {4000, 10000}, function()
        scarabsTick(creature, guid)
    end)
end

local function mendingTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "mending", 500, function()
            mendingTick(creature, guid)
        end)
        return
    end
    local best, bestPct = nil, 76
    for _, c in pairs(creature:GetCreaturesInGrid(NPC_ICC_NERUBAR_BROODKEEPER, 40.0)) do
        if c:IsAlive() and c:IsInCombat() and not creature:IsHostileTo(c) then
            local pct = c:GetHealthPct()
            if pct >= 1 and pct <= 75 and pct < bestPct then
                best, bestPct = c, pct
            end
        end
    end
    if best then
        creature:CastSpell(best, SPELL_DARK_MENDING)
    end
    schedule(guid, "mending", {3000, 10000}, function()
        mendingTick(creature, guid)
    end)
end

local function webWrapTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "wrap", 500, function()
            webWrapTick(creature, guid)
        end)
        return
    end
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if not t:IsDead() then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates > 0 then
        creature:CastSpell(candidates[math.random(#candidates)], SPELL_WEB_WRAP)
    end
    schedule(guid, "wrap", {16000, 20000}, function()
        webWrapTick(creature, guid)
    end)
end

local function broodkeeperEngage(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "scarabs", {1000, 3000}, function()
        scarabsTick(creature, guid)
    end)
    schedule(guid, "mending", 5000, function()
        mendingTick(creature, guid)
    end)
    schedule(guid, "wrap", 20000, function()
        webWrapTick(creature, guid)
    end)
end

local function broodkeeperLeave(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(NPC_ICC_NERUBAR_BROODKEEPER, 5, broodkeeperSpawn)
RegisterCreatureEvent(NPC_ICC_NERUBAR_BROODKEEPER, 1, broodkeeperEngage)
RegisterCreatureEvent(NPC_ICC_NERUBAR_BROODKEEPER, 2, broodkeeperLeave)
RegisterCreatureEvent(NPC_ICC_NERUBAR_BROODKEEPER, 4, broodkeeperLeave)
RegisterCreatureEvent(NPC_ICC_NERUBAR_BROODKEEPER, 23, broodkeeperLeave)
