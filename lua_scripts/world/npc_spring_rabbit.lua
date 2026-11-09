-- World: Spring Rabbit — Lua port of npc_spring_rabbitAI
-- (src/server/scripts/World/npcs_special.cpp:2002-2100).
-- Entry: 32791 (NPC_SPRING_RABBIT, :2000, in-tree).
-- Eluna creature events: 2 OnLeaveCombat, 4 OnDied, 23 OnReset.
-- Timers via CreateLuaEvent (GUID-keyed, replaced per key, cancelled
-- on events 2/4/23); the C++ UpdateAI countdowns map to re-armed
-- one-shot ticks (maiden idiom).
--
-- Ported arms (C++ order):
-- - Reset (:2036-2041): Initialize (inLove=false, timers
--   {5,10}s search / {5,10}s jump / {10,20}s bunny) + owner
--   MoveFollow(PET_FOLLOW_DIST 1.0, PET_FOLLOW_ANGLE pi/2) when the
--   creature has an owner (GetOwner is player-owner-only; nil-gated
--   exactly like C++).
-- - DoAction(1) (:2044-2049): inLove latch + owner self-cast of
--   SPELL_SPRING_FLING 61875 (triggered). Reached two ways: the
--   self pair-up below, and the cross-creature leg — the sender
--   writes the found rabbit's shared-table state directly (same file,
--   same Lua state), the ribbon-pole SetData-relay idiom without a
--   dispatch bridge; there is no creature:DoAction method.
-- - UpdateAI search branch (:2068-2088): FindNearestCreature(32791,
--   10.0) (bridge excludes self, so the C++ rabbit==me reject cannot
--   fire); a partner carrying SPELL_SPRING_RABBIT_IN_LOVE 61728 is
--   rejected with a 100ms retry (C++ returns without re-arming, so
--   the next UpdateAI tick retries); no partner re-arms {5,10}s.
--   Pair-up order kept: self AddAura(IN_LOVE), self DoAction(1),
--   partner AddAura(IN_LOVE), partner DoAction(1), partner
--   self-cast SPELL_SPRING_RABBIT_JUMP 61724 (triggered).
-- - UpdateAI inLove branch (:2051-2066): jump tick casts JUMP at
--   the partner, re-arm {5,10}s; bunny tick self-casts
--   SPELL_SUMMON_BABY_BUNNY 61727, re-arm {20,40}s.
-- - JustEngagedWith (:2042): empty in C++; no handler.
--
-- Deviations (documented, no bridge):
-- - Partner jump target: C++ pins the stored rabbitGUID via
--   ObjectAccessor::GetUnit; Go has no GUID-to-creature bridge, so
--   each jump tick re-finds the nearest spring rabbit within 10yd
--   (halazzi/jindo precedent). Partners never move except on
--   owner-follow, so the re-find matches while pairs stay adjacent;
--   a third rabbit closer than the partner would steal the cosmetic
--   jump cast.
-- - Pair-up jump/bunny timers: C++ keeps the partner's residual
--   countdowns from its own spawn; Lua rolls fresh {5,10}s /
--   {10,20}s timers at pair-up (same ranges, unobservable on
--   cosmetic timers).
-- - The ev23 handler dead-guards: Go fires ev23 on spawn, evade
--   and death, while C++ Reset runs on spawn/evade only, so a
--   death-time ev23 clears state instead of scheduling.

local ENTRY_SPRING_RABBIT = 32791
local SPELL_SPRING_FLING = 61875
local SPELL_SPRING_RABBIT_JUMP = 61724
local SPELL_SPRING_RABBIT_IN_LOVE = 61728
local SPELL_SUMMON_BABY_BUNNY = 61727
local PET_FOLLOW_DIST = 1.0
local PET_FOLLOW_ANGLE = 1.5708

local timers = {}
local state = {}

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

local function doRabbitAction(creature, guid)
    local st = state[guid]
    if not st then
        return
    end
    st.inLove = true
    local owner = creature:GetOwner()
    if owner then
        owner:CastSpell(owner, SPELL_SPRING_FLING, true)
    end
end

local onSearchTick, onJumpTick, onBunnyTick

onJumpTick = function(creature, guid)
    local st = state[guid]
    if not st or not st.inLove or not creature:IsAlive() then
        return
    end
    local partner = creature:FindNearestCreature(ENTRY_SPRING_RABBIT, 10.0)
    if partner then
        creature:CastSpell(partner, SPELL_SPRING_RABBIT_JUMP)
    end
    schedule(guid, "jump", {5000, 10000}, function() onJumpTick(creature, guid) end)
end

onBunnyTick = function(creature, guid)
    local st = state[guid]
    if not st or not st.inLove or not creature:IsAlive() then
        return
    end
    creature:CastSpell(creature, SPELL_SUMMON_BABY_BUNNY)
    schedule(guid, "bunny", {20000, 40000}, function() onBunnyTick(creature, guid) end)
end

onSearchTick = function(creature, guid)
    local st = state[guid]
    if not st or st.inLove or not creature:IsAlive() then
        return
    end
    local partner = creature:FindNearestCreature(ENTRY_SPRING_RABBIT, 10.0)
    if partner and not partner:HasAura(SPELL_SPRING_RABBIT_IN_LOVE) then
        creature:AddAura(SPELL_SPRING_RABBIT_IN_LOVE)
        doRabbitAction(creature, guid)
        local partnerGuid = partner:GetGUIDLow()
        if not state[partnerGuid] then
            state[partnerGuid] = { inLove = false }
        end
        partner:AddAura(SPELL_SPRING_RABBIT_IN_LOVE)
        doRabbitAction(partner, partnerGuid)
        partner:CastSpell(partner, SPELL_SPRING_RABBIT_JUMP, true)
        cancelTimers(partnerGuid)
        schedule(partnerGuid, "jump", {5000, 10000}, function() onJumpTick(partner, partnerGuid) end)
        schedule(partnerGuid, "bunny", {10000, 20000}, function() onBunnyTick(partner, partnerGuid) end)
        schedule(guid, "jump", {5000, 10000}, function() onJumpTick(creature, guid) end)
        schedule(guid, "bunny", {10000, 20000}, function() onBunnyTick(creature, guid) end)
    elseif partner then
        schedule(guid, "search", 100, function() onSearchTick(creature, guid) end)
    else
        schedule(guid, "search", {5000, 10000}, function() onSearchTick(creature, guid) end)
    end
end

local function onRabbitReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    if not creature:IsAlive() then
        state[guid] = nil
        return
    end
    state[guid] = { inLove = false }
    local owner = creature:GetOwner()
    if owner then
        creature:MoveFollow(owner, PET_FOLLOW_DIST, PET_FOLLOW_ANGLE)
    end
    schedule(guid, "search", {5000, 10000}, function() onSearchTick(creature, guid) end)
end

local function onRabbitDied(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    state[guid] = nil
end

RegisterCreatureEvent(ENTRY_SPRING_RABBIT, 23, onRabbitReset)
RegisterCreatureEvent(ENTRY_SPRING_RABBIT, 2, onRabbitReset)
RegisterCreatureEvent(ENTRY_SPRING_RABBIT, 4, onRabbitDied)
