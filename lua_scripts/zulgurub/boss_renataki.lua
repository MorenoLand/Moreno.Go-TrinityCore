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
-- InterruptSpell(CURRENT_GENERIC_SPELL) is a no-op (no cast state in
-- Go) — skipped, the state flips still happen; the invisible/visible
-- visual arms are PORTED (2026-10-21): SetEquipmentSlots main-hand
-- swap + SetDisplayId(11686/15268) + UNIT_FLAG_NOT_SELECTABLE
-- SetFlag/RemoveFlag (all bridged) — the boss actually goes
-- invisible + unselectable C++-exact; DoTeleportTo ports via the
-- live creature:Teleport bridge (2026-10-24); the aggro-switch
-- GetThreat/ModifyThreatByPercent(-50) arm is PORTED 2026-10-24; the AttackStart arm has no movement
-- bridge (grilek convention) — only the 7-20s re-arm cycle is kept for that leg; no instance-script model — boss admission via the luaBossAI
-- shim (_Reset/BossAI::JustEngagedWith and the GetZulGurubAI
-- bookkeeping arms skipped); the C++ frozen-timer behavior (ambush,
-- aggro, thousand-blades timers hold their remainder while the gate is
-- closed) is approximated as a full re-arm on gate close.

local ENTRY_RENATAKI = 15084

local SPELL_AMBUSH = 34794
local SPELL_THOUSANDBLADES = 34799

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000

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

-- C++ SelectTarget(Random, 0, 100.0f, true) (boss_renataki.cpp:83/:116):
-- threat-list players alive within 100 yd.
local function randomThreatPlayerInRange(creature, range)
    local targets = creature:GetAITargets()
    local out = {}
    for _, unit in ipairs(targets) do
        if unit:GetObjectType() == "Player" and not unit:IsDead()
                and creature:GetDistance(unit) <= range then
            out[#out + 1] = unit
        end
    end
    if #out == 0 then
        return nil
    end
    return out[math.random(#out)]
end

-- C++ invisible timer :67-74: InterruptSpell is a no-op (no cast state
-- in Go) + SetEquipmentSlots(main-hand unequip) + SetDisplayId(11686) +
-- SetFlag(UNIT_FLAG_NOT_SELECTABLE); the state flip is C++-exact,
-- re-arm {15s,30s}.
local function onInvisible(creature, guid)
    local st = renatakiState[guid]
    if st then
        st.invisible = true
    end
    creature:SetEquipmentSlots(0, 0xFFFFFFFF, 0xFFFFFFFF)
    creature:SetDisplayId(11686)
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    schedule(guid, "invisible", math.random(15000, 30000), function()
        onInvisible(creature, guid)
    end)
end

-- C++ ambush timer: fires every 3s, gated on _invisible; SelectTarget
-- random alive player within 100 yd -> DoTeleportTo(target pos)
-- (:85, self-teleport; the bridge defaults orientation to the current
-- facing like the C++ default) + DoCast(target, 34794) non-triggered;
-- _ambushed = true unconditionally on fire; re-arm 3s C++-exact.
-- Gate-closed ticks do nothing (C++ freeze approximated by continuing
-- the cycle).
local function onAmbush(creature, guid)
    local st = renatakiState[guid]
    if st and st.invisible then
        local target = randomThreatPlayerInRange(creature, 100)
        if target then
            creature:Teleport(target:GetX(), target:GetY(), target:GetZ())
            creature:CastSpell(target, SPELL_AMBUSH)
        end
        st.ambushed = true
    end
    schedule(guid, "ambush", 3000, function()
        onAmbush(creature, guid)
    end)
end

-- C++ visible timer :99-106: 4s, gated on _ambushed; InterruptSpell is a
-- no-op (no cast state in Go) + SetDisplayId(15268) + SetEquipmentSlots
-- (EQUIP_ID_MAIN_HAND = 0, boss_renataki.cpp:30) + RemoveFlag(
-- UNIT_FLAG_NOT_SELECTABLE); clears _invisible C++-exact; re-arm 4000
-- C++-exact (idempotent while visible).
local function onVisible(creature, guid)
    local st = renatakiState[guid]
    if st and st.ambushed and st.invisible then
        st.invisible = false
        creature:SetDisplayId(15268)
        creature:SetEquipmentSlots(0, 0xFFFFFFFF, 0xFFFFFFFF)
        creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    end
    schedule(guid, "visible", 4000, function()
        onVisible(creature, guid)
    end)
end

-- C++ aggro-switch timer: frozen while invisible; the GetThreat/
-- ModifyThreatByPercent(-50, victim) arm is PORTED 2026-10-24 (a random
-- 100-yd player pick gates the cut, C++ order) and the AttackStart(target) arm has no movement bridge.
-- Re-arm {7s,20s}.
local function onAggroSwitch(creature, guid)
    local st = renatakiState[guid]
    if not (st and st.invisible) then
        if randomThreatPlayerInRange(creature, 100) then
            local victim = creature:GetVictim()
            if victim and creature:GetThreat(victim) ~= 0 then
                creature:ModifyThreatByPercent(victim, -50)
            end
        end
    end
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
