-- Phalanx (Blackrock Depths) --
-- Lua port of src/server/scripts/EasternKingdoms/BlackrockMountain/
-- BlackrockDepths/blackrock_depths.cpp (npc_phalanx — ScriptedAI combat
-- scheduler). Boss/zone-script unit per
-- eastern_kingdoms_script_loader.cpp order (AddSC_blackrock_depths
-- follows the closed Alterac Valley set; instance_blackrock_depths.cpp
-- stays blocked on the instance-script model).
-- Entry (verifiable from the C++ sources): NPC_PHALANX = 9502 in
-- instance_blackrock_depths.cpp:36 (wired to DATA_PHALANX = 11 in
-- blackrock_depths.h; used by npc_rocknot's bar-door arm). The
-- creature_template ScriptName binding is DB-side (no TDB in this
-- workspace), but the entry itself is C++-verifiable so the port is
-- registered (storm_cloud / av_marshal / balinda / drekthar /
-- galvangar / vanndar precedent).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Driven by CreateLuaEvent timers; melee is
-- engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
-- Ported arms (C++ Reset init + UpdateAI combat machine):
-- SPELL_THUNDERCLAP: victim-cast THUNDERCLAP 8732, 12s init -> 10s;
-- SPELL_FIREBALLVOLLEY: victim-cast FIREBALLVOLLEY 22425, 15s, gated
--   on HealthBelowPct(51) (GetHealthPct resolves on the motion object
--   via the generic-field path in scripting/object.go);
-- SPELL_MIGHTYBLOW: victim-cast MIGHTYBLOW 14099, 15s init -> 10s.
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (creature
-- casts are packet-visual), so timers fire unconditionally (maiden
-- precedent); the C++ FireballVolley timer only ticks while health is
-- below 51% — the port re-arms every 15s unconditionally and gates
-- the cast, which is behaviorally equivalent for cast timing.

local SPELL_THUNDERCLAP = 8732
local SPELL_FIREBALLVOLLEY = 22425
local SPELL_MIGHTYBLOW = 14099

local ENTRY_PHALANX = 9502

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

local function onThunderclap(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_THUNDERCLAP)
    end
    schedule(guid, "thunderclap", 10000, function() onThunderclap(creature, guid) end)
end

local function onFireballVolley(creature, guid)
    local victim = creature:GetVictim()
    if victim and creature:GetHealthPct() < 51 then
        creature:CastSpell(victim, SPELL_FIREBALLVOLLEY)
    end
    schedule(guid, "fireballvolley", 15000, function() onFireballVolley(creature, guid) end)
end

local function onMightyBlow(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_MIGHTYBLOW)
    end
    schedule(guid, "mightyblow", 10000, function() onMightyBlow(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "thunderclap", 12000, function() onThunderclap(creature, guid) end)
    schedule(guid, "fireballvolley", 15000, function() onFireballVolley(creature, guid) end)
    schedule(guid, "mightyblow", 15000, function() onMightyBlow(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(_, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_PHALANX, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_PHALANX, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_PHALANX, 4, onCombatEnd)
RegisterCreatureEvent(ENTRY_PHALANX, 23, onReset)
