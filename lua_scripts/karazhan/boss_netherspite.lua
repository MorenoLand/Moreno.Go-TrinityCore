-- Netherspite (Karazhan) — Lua port of
-- src/server/scripts/EasternKingdoms/Karazhan/boss_netherspite.cpp
-- Creature entry: 15689 (TDB creature_template ScriptName "boss_netherspite").
-- Instance data: DATA_NETHERSPITE = 8, DATA_GO_MASSIVE_DOOR = 22
-- (karazhan.h:38,50); the encounter never reads encounter state — the
-- door/instance arms have no Go model (see deviations).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Fight shape: 60s portal phase / 30s banish phase. Portal phase arms the
-- three colored portals at random positions (rand32()%4 placement), picks
-- beam targets every 1s (closest alive player standing between boss and
-- portal, not exhausted and not on another beam; boss himself otherwise),
-- and applies the color buffs; empowerment every 90s. Banish phase roots
-- the boss and casts Netherbreath every 5-7s. Void Zone every 15s and the
-- 9-minute berserk tick in both phases.
-- Deviations from C++: the engine has no summon model, so the portal
-- creatures (PortalID 17369/17367/17368), their PortalVisual auras, and the
-- beamer workaround creatures never exist — portal positions are tracked
-- as virtual coordinates and the PortalBeam visual casts are skipped.
-- No instance-script model, so HandleDoors (the massive door) is skipped
-- and DATA_NETHERSPITE is admission-only via the luaBossAI shim in
-- engine/world/boss_ai.go. No threat model, so the red beam's
-- AddThreat(target, 100000) arm is skipped (the beam buff itself is
-- applied). No UNIT_STATE_CASTING model, so the phase switch fires
-- unconditionally instead of waiting for IsNonMeleeSpellCast(false)
-- (same convention as the other Karazhan Lua ports).

local ENTRY_NETHERSPITE = 15689

local EMOTE_PHASE_PORTAL = 0
local EMOTE_PHASE_BANISH = 1

local SPELL_NETHERBURN_AURA = 30522
local SPELL_VOIDZONE = 37063
local SPELL_NETHER_INFUSION = 38688
local SPELL_NETHERBREATH = 38523
local SPELL_BANISH_VISUAL = 39833
local SPELL_BANISH_ROOT = 42716
local SPELL_EMPOWERMENT = 38549
local SPELL_NETHERSPITE_ROAR = 38684

-- j = 1 red (Perseverence), 2 green (Serenity), 3 blue (Dominance)
local PORTAL_COORD = {
    { -11195.353516, -1613.237183, 278.237258 }, -- left side
    { -11137.846680, -1685.607422, 278.239258 }, -- right side
    { -11094.493164, -1591.969238, 279.949188 }, -- back side
}
local PLAYER_BUFF = { 30421, 30422, 30423 }
local NETHER_BUFF = { 30466, 30467, 30468 }
local PLAYER_DEBUFF = { 38637, 38638, 38639 }

local PHASE_PORTAL_MS = 60000
local PHASE_BANISH_MS = 30000

local timers = {}
local netherState = {}

local onPhaseSwitch, onNetherbreath

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
    per[key] = CreateLuaEvent(fn, delay)
end

local function playersInInstance(creature)
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

local function randomAlivePlayerInRange(creature, maxDist)
    local candidates = {}
    for _, p in ipairs(playersInInstance(creature)) do
        if creature:GetDistance(p) <= maxDist then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

local function dist2d(xa, ya, xb, yb)
    local dx, dy = xa - xb, ya - yb
    return math.sqrt(dx * dx + dy * dy)
end

-- C++ boss_netherspiteAI::IsBetween: target must lie strictly inside the
-- boss->portal segment and within 1.5 yd of the beam line (2D).
local function isBetween(bx, by, px, py, portalX, portalY)
    local bossPortal = dist2d(bx, by, portalX, portalY)
    if bossPortal == 0 then
        return false
    end
    if dist2d(bx, by, px, py) >= bossPortal then
        return false
    end
    if dist2d(portalX, portalY, px, py) >= bossPortal then
        return false
    end
    return math.abs((bx - portalX) * py + (portalY - by) * px
        - bx * portalY + portalX * by) / bossPortal < 1.5
end

-- C++ SummonPortals placement: r = rand32() % 4; blue never on the left.
local function summonPortals(guid)
    local state = netherState[guid]
    if state == nil then
        return
    end
    local r = math.random(0, 3)
    local pos = {}
    pos[1] = (r % 2 == 1) and (r > 1 and 3 or 2) or 1       -- red
    pos[2] = (r % 2 == 1) and 1 or (r > 1 and 3 or 2)        -- green
    pos[3] = (r > 1) and 2 or 3                             -- blue
    state.portals = {}
    for j = 1, 3 do
        state.portals[j] = PORTAL_COORD[pos[j]]
    end
end

local function destroyPortals(guid)
    local state = netherState[guid]
    if state ~= nil then
        state.portals = nil
    end
end

-- C++ UpdatePortals, every 1s during the portal phase.
local function updatePortals(creature, guid)
    local state = netherState[guid]
    if state == nil or not state.portalPhase or state.portals == nil then
        return
    end
    local bx, by = creature:GetX(), creature:GetY()
    for j = 1, 3 do
        local portal = state.portals[j]
        local bossDist = dist2d(bx, by, portal[1], portal[2])
        local best, bestDist = nil, bossDist
        for _, p in ipairs(playersInInstance(creature)) do
            if not p:HasAura(PLAYER_DEBUFF[j])
                    and not p:HasAura(PLAYER_BUFF[(j % 3) + 1])
                    and not p:HasAura(PLAYER_BUFF[((j + 1) % 3) + 1]) then
                local px, py = p:GetX(), p:GetY()
                if isBetween(bx, by, px, py, portal[1], portal[2]) then
                    local d = dist2d(px, py, portal[1], portal[2])
                    if d < bestDist then
                        best, bestDist = p, d
                    end
                end
            end
        end
        if best ~= nil then
            best:AddAura(PLAYER_BUFF[j])
            -- C++ AddThreat(best, 100000) on the red beam has no bridge.
        else
            creature:AddAura(NETHER_BUFF[j])
        end
        -- The beamer visual cast (PortalBeam) needs a summoned portal
        -- creature; no summon model, skipped.
    end
    schedule(guid, "portalTick", 1000, function() updatePortals(creature, guid) end)
end

local function onEmpowerment(creature, guid)
    local state = netherState[guid]
    if state == nil or not state.portalPhase then
        return
    end
    creature:CastSpell(creature, SPELL_EMPOWERMENT)
    creature:AddAura(SPELL_NETHERBURN_AURA)
    schedule(guid, "empowerment", 90000, function() onEmpowerment(creature, guid) end)
end

local function switchToPortal(creature, guid)
    local state = netherState[guid]
    if state == nil then
        return
    end
    creature:RemoveAura(SPELL_BANISH_ROOT)
    creature:RemoveAura(SPELL_BANISH_VISUAL)
    summonPortals(guid)
    state.portalPhase = true
    cancelTimers(guid)
    schedule(guid, "portalTick", 10000, function() updatePortals(creature, guid) end)
    schedule(guid, "empowerment", 10000, function() onEmpowerment(creature, guid) end)
    schedule(guid, "phase", PHASE_PORTAL_MS, function() onPhaseSwitch(creature, guid) end)
    creature:Talk(EMOTE_PHASE_PORTAL)
end

local function switchToBanish(creature, guid)
    local state = netherState[guid]
    if state == nil then
        return
    end
    creature:RemoveAura(SPELL_EMPOWERMENT)
    creature:RemoveAura(SPELL_NETHERBURN_AURA)
    creature:CastSpell(creature, SPELL_BANISH_VISUAL, true)
    creature:CastSpell(creature, SPELL_BANISH_ROOT, true)
    destroyPortals(guid)
    state.portalPhase = false
    for j = 1, 3 do
        creature:RemoveAura(NETHER_BUFF[j])
    end
    cancelTimers(guid)
    local firstBreath = 3000
    if state.breathPrimed then
        firstBreath = { 5000, 7000 }
    end
    schedule(guid, "netherbreath", firstBreath, function() onNetherbreath(creature, guid) end)
    schedule(guid, "phase", PHASE_BANISH_MS, function() onPhaseSwitch(creature, guid) end)
    creature:Talk(EMOTE_PHASE_BANISH)
end

onPhaseSwitch = function(creature, guid)
    local state = netherState[guid]
    if state == nil then
        return
    end
    -- C++ waits for !IsNonMeleeSpellCast(false); no cast-state model, so
    -- the switch fires unconditionally (other Karazhan ports do the same).
    if state.portalPhase then
        switchToBanish(creature, guid)
    else
        switchToPortal(creature, guid)
    end
end

onNetherbreath = function(creature, guid)
    local state = netherState[guid]
    if state == nil or state.portalPhase then
        return
    end
    local target = randomAlivePlayerInRange(creature, 40)
    if target ~= nil then
        creature:CastSpell(target, SPELL_NETHERBREATH)
    end
    state.breathPrimed = true
    schedule(guid, "netherbreath", { 5000, 7000 }, function() onNetherbreath(creature, guid) end)
end

local function onVoidZone(creature, guid)
    if netherState[guid] == nil then
        return
    end
    local target = randomAlivePlayerInRange(creature, 45)
    if target ~= nil then
        creature:CastSpell(target, SPELL_VOIDZONE, true)
    end
    schedule(guid, "voidzone", 15000, function() onVoidZone(creature, guid) end)
end

local function onBerserk(creature, guid)
    local state = netherState[guid]
    if state == nil or state.berserk then
        return
    end
    state.berserk = true
    creature:AddAura(SPELL_NETHER_INFUSION)
    creature:CastSpell(creature, SPELL_NETHERSPITE_ROAR)
end

local function netherspiteInitState(guid)
    -- C++ Initialize(): Berserk = false, NetherInfusionTimer = 540000,
    -- VoidZoneTimer = 15000, NetherbreathTimer = 3000 (first banish only).
    netherState[guid] = { portalPhase = false, berserk = false,
        breathPrimed = false, portals = nil }
end

local function netherspiteEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    netherspiteInitState(guid)
    -- C++ JustEngagedWith: HandleDoors(false) (no instance-script model,
    -- skipped) then SwitchToPortalPhase.
    switchToPortal(creature, guid)
    schedule(guid, "voidzone", 15000, function() onVoidZone(creature, guid) end)
    schedule(guid, "berserk", 540000, function() onBerserk(creature, guid) end)
end

local function netherspiteLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    netherspiteInitState(guid)
end

local function netherspiteDied(event, creature, killer)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    destroyPortals(guid)
    -- C++ JustDied: HandleDoors(true) (no instance-script model, skipped).
end

local function netherspiteReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    netherspiteInitState(guid)
    destroyPortals(guid)
end

RegisterCreatureEvent(ENTRY_NETHERSPITE, 1, netherspiteEnterCombat)
RegisterCreatureEvent(ENTRY_NETHERSPITE, 2, netherspiteLeaveCombat)
RegisterCreatureEvent(ENTRY_NETHERSPITE, 4, netherspiteDied)
RegisterCreatureEvent(ENTRY_NETHERSPITE, 23, netherspiteReset)
