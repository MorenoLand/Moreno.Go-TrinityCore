-- Arugal Voidwalker (Shadowfang Keep trash)
-- Lua port of src/server/scripts/EasternKingdoms/ShadowfangKeep/
-- shadowfang_keep.cpp
-- (npc_arugal_voidwalkerAI — ScriptedAI via GetShadowfangKeepAI ->
-- GetInstanceAI; registered by AddSC_shadowfang_keep in
-- eastern_kingdoms_script_loader.cpp (declaration line 123, call
-- line 301)). The Shadowfang Keep block is OPEN.
-- Entry (verifiable from the C++ sources): NPC_ARUGAL_VOIDWALKER =
-- 4627 in the SKCreatures enum in ShadowfangKeep/shadowfang_keep.h
-- (kalecgos precedent). Whole-server-tree grep confirms
-- shadowfang_keep.cpp as the only source of "arugal_voidwalker" for
-- the script (loader lines only otherwise).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Driven by CreateLuaEvent timers; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Verifiable numbers (file's own enums): SPELL_DARK_OFFERING = 7154.
-- Ported arm (C++ UpdateAI combat machine):
-- - SPELL_DARK_OFFERING: {200, 1000} init -> {4400, 12500} re-arm.
--   The C++ arm casts on the nearest same-entry creature within 25y
--   and falls back to a self-cast when none is found; there is no
--   FindNearestCreature bridge on the Lua surface (nefarian
--   precedent), so the friend-selection leg is skipped and the
--   self-cast fallback always fires.
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - JustDied: instance->SetData(TYPE_FENRUS, instance->GetData(
--   TYPE_FENRUS) + 1) (instance-script model blocked, standing).
-- - GetShadowfangKeepAI -> GetInstanceAI leg (instance-model
--   blocked, standing).

local ENTRY_ARUGAL_VOIDWALKER = 4627

local SPELL_DARK_OFFERING = 7154

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
    per[key] = CreateLuaEvent(fn, delay)
end

local function onDarkOffering(creature, guid)
    creature:CastSpell(creature, SPELL_DARK_OFFERING)
    schedule(guid, "darkoffering", math.random(4400, 12500), function() onDarkOffering(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "darkoffering", math.random(200, 1000), function() onDarkOffering(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(_, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_ARUGAL_VOIDWALKER, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_ARUGAL_VOIDWALKER, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_ARUGAL_VOIDWALKER, 4, onCombatEnd)
RegisterCreatureEvent(ENTRY_ARUGAL_VOIDWALKER, 23, onReset)
