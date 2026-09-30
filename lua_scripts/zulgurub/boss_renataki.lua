-- Renataki of the Thousand Blades (Zul'Gurub, Edge of Madness) — Lua
-- port of src/server/scripts/EasternKingdoms/ZulGurub/boss_renataki.cpp
-- (boss_renatakiAI); zulgurub.h:39 (DATA_EDGE_OF_MADNESS = 9, optional
-- event; no NPC_RENATAKI constant — verified by grep). Creature entry:
-- 15084 Renataki (C++ ScriptName "boss_renataki" per AddSC_boss_renataki;
-- classic.wowhead.com/npc=15084/renataki-of-the-thousand-blades).
-- Renataki is summoned by the Edge of Madness brazier event (no summon
-- model — he simply starts combat already spawned).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady. No Talk lines
-- exist in the C++ AI.
-- Fight shape: enter combat arms five timers — invisible at
-- {8s,18s} then {15s,30s} / ambush every 3s (fires only while invisible:
-- teleport + ambush 34794 non-triggered on a random alive player within
-- 100 yd, then _ambushed = true) / visible 4s (fires only once
-- _ambushed: restores the model, clears invisible) / aggro switch at
-- {15s,25s} then {7s,20s} (frozen while invisible) / thousand blades
-- 34799 at {4s,8s} then {7s,12s} on the victim (frozen while invisible;
-- triggered per C++ default DoCastVictim non-triggered).
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING gate has no
-- UNIT_STATE bridge — timers fire unconditionally (jeklik convention);
-- InterruptSpell(CURRENT_GENERIC_SPELL) has no bridge — skipped, the
-- state flips still happen; SetDisplayId(11686/15268), the
-- SetEquipmentSlots main-hand swap, and the UNIT_FLAG_NOT_SELECTABLE
-- flag arms have no bridges (arlokk convention) — invisibility is
-- modeled as state only, the boss stays targetable; DoTeleportTo has no
-- teleport bridge (jindo convention) — the ambush teleport is skipped
-- and the cast lands on the picked player directly; the aggro-switch
-- GetThreat/ModifyThreatByPercent(-50) + AttackStart arms have no threat
-- or movement bridge (grilek convention) — only the 7-20s re-arm cycle
-- is kept; no instance-script model — boss admission via the luaBossAI
-- shim (_Reset/BossAI::JustEngagedWith and the GetZulGurubAI
-- bookkeeping arms skipped); the C++ frozen-timer behavior (ambush,
-- aggro, thousand-blades timers hold their remainder while the gate is
-- closed) is approximated as a full re-arm on gate close.

local ENTRY_RENATAKI = 15084

local SPELL_AMBUSH = 34794
local SPELL_THOUSANDBLADES = 34799

local timers = {}
local renatakiState = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
    renatakiState[guid] = nil
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

-- C++ invisible timer: InterruptSpell + unequip + display 11686 +
-- NOT_SELECTABLE have no bridge; the state flip is C++-exact, re-arm
-- {15s,30s}.
local function onInvisible(creature, guid)
    local st = renatakiState[guid]
    if st then
        st.invisible = true
    end
    schedule(guid, "invisible", math.random(15000, 30000), function()
        onInvisible(creature, guid)
    end)
end

-- C++ ambush timer: fires every 3s, gated on _invisible; SelectTarget
-- random alive player within 100 yd -> DoTeleportTo (skipped, no
-- teleport bridge) + DoCast(target, 34794) non-triggered; _ambushed =
-- true unconditionally on fire; re-arm 3s C++-exact. Gate-closed ticks
-- do nothing (C++ freeze approximated by continuing the cycle).
local function onAmbush(creature, guid)
    local st = renatakiState[guid]
    if st and st.invisible then
        local candidates = {}
        for _, p in ipairs(alivePlayersInInstance(creature)) do
            if creature:GetDistance(p) <= 100 then
                candidates[#candidates + 1] = p
            end
        end
        if #candidates > 0 then
            creature:CastSpell(candidates[math.random(#candidates)],
                SPELL_AMBUSH)
        end
        st.ambushed = true
    end
    schedule(guid, "ambush", 3000, function()
        onAmbush(creature, guid)
    end)
end

-- C++ visible timer: 4s, gated on _ambushed; InterruptSpell +
-- SetDisplayId(15268) + re-equip + RemoveFlag have no bridge; clears
-- _invisible C++-exact; re-arm 4000 C++-exact (idempotent while
-- visible).
local function onVisible(creature, guid)
    local st = renatakiState[guid]
    if st and st.ambushed and st.invisible then
        st.invisible = false
    end
    schedule(guid, "visible", 4000, function()
        onVisible(creature, guid)
    end)
end

-- C++ aggro-switch timer: frozen while invisible; the GetThreat/
-- ModifyThreatByPercent(-50, victim) and AttackStart(target) arms have
-- no threat/movement bridge — only the re-arm cycle is kept
-- (grilek convention). Re-arm {7s,20s}.
local function onAggroSwitch(creature, guid)
    schedule(guid, "aggro", math.random(7000, 20000), function()
        onAggroSwitch(creature, guid)
    end)
end

-- C++ thousand-blades timer: frozen while invisible; non-triggered
-- DoCastVictim(34799); nil-victim ticks cast nothing but keep the
-- schedule (jeklik convention). Re-arm {7s,12s}.
local function onThousandBlades(creature, guid)
    local st = renatakiState[guid]
    if not (st and st.invisible) then
        local victim = creature:GetVictim()
        if victim then
            creature:CastSpell(victim, SPELL_THOUSANDBLADES)
        end
    end
    schedule(guid, "thousandblades", math.random(7000, 12000), function()
        onThousandBlades(creature, guid)
    end)
end

local function resetState(guid)
    renatakiState[guid] = { invisible = false, ambushed = false }
end

RegisterCreatureEvent(ENTRY_RENATAKI, 1, function(event, creature)
    local guid = creature:GetGUID()
    resetState(guid)
    schedule(guid, "invisible", math.random(8000, 18000), function()
        onInvisible(creature, guid)
    end)
    schedule(guid, "ambush", 3000, function()
        onAmbush(creature, guid)
    end)
    schedule(guid, "visible", 4000, function()
        onVisible(creature, guid)
    end)
    schedule(guid, "aggro", math.random(15000, 25000), function()
        onAggroSwitch(creature, guid)
    end)
    schedule(guid, "thousandblades", math.random(4000, 8000), function()
        onThousandBlades(creature, guid)
    end)
end)

RegisterCreatureEvent(ENTRY_RENATAKI, 2, function(event, creature)
    cancelTimers(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_RENATAKI, 4, function(event, creature)
    cancelTimers(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_RENATAKI, 23, function(event, creature)
    cancelTimers(creature:GetGUID())
end)
