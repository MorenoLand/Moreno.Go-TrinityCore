-- Azshara: Azuregos (world boss) — Lua port of
-- src/server/scripts/Kalimdor/boss_azuregos.cpp
-- (boss_azuregosAI : public WorldBossAI; AddSC_boss_azuregos also
-- registers spell_mark_of_frost; kalimdor loader decl/call per
-- kalimdor_script_loader.cpp).
-- Entry 6109 — "Azuregos", Azshara world boss (spell IDs 21097-21099
-- mana storm/chill/frost breath + 23182/23184 mark of frost match the
-- WotLK Azuregos kit; creature_template ScriptName binding is DB-side,
-- no TDB in this workspace).
--
-- Ported arms (C++-exact):
-- - JustEngagedWith: triggered self-cast Mark of Frost Aura 23184;
--   the _enraged latch is reset here; event schedule below.
-- - UpdateAI timer arms (CreateLuaEvent per-GUID timers; cancel on
--   leave combat / died / reset; C++-exact init -> repeat ranges;
--   nil-target ticks cast nothing but keep the schedule):
--   - Mana storm 21097: C++ SelectTarget(Random, 0, 60.0f,
--     playerOnly=true) from the threat list -> random alive player
--     within 60yd of GetAITargets (cairne idiom + 60yd filter, C++
--     UnitAI.h:170 signature verified); init 5-17s, repeat 7.5-12.5s,
--     non-triggered.
--   - Chill 21098: victim-cast; init 10-30s, repeat 13-25s.
--   - Frost breath 21099: victim-cast; init 2-8s, repeat 10-15s.
--   - Teleport (SAY_TELEPORT 0, Talk fired unconditionally on timer
--     expiry): every player on the threat list (C++ iterates
--     GetPvECombatRefs with ToPlayer, i.e. players only — ported as
--     GetAITargets filtered to players) teleported to the boss
--     position +3z via player:TeleportNear, keeping each player's own
--     orientation (p:GetO() — the Orientation field resolves through
--     the generic object fallback); init 30s, repeat 30s.
--     C++ ResetThreatList PORTED 2026-10-19 (creature:ResetAllThreat,
--     bridged lua_objects.go:389) —
--     skipped.
--   - Reflect 22067: self-cast; init 15-30s, repeat 20-35s.
--   - Cleave 8255: victim-cast; init 7s, repeat 7s.
--   - EVENT_MARK_OF_FROST (35s): scheduled in C++ JustEngagedWith but
--     absent from the UpdateAI switch (default: break) — provable dead
--     event, not ported.
-- - KilledUnit (event 3): player victim -> Mark of Frost 23182
--   triggered. C++ is who->CastSpell(who, ...) (player self-cast);
--   player:CastSpell has no bridge (anetheron sleep precedent), so the
--   boss casts it on the victim instead (caster deviates from C++).
-- - Enrage: HealthBelowPct(26) one-shot latch -> self-cast Enrage
--   23537; checked by a 1s per-GUID pump (sironas convention; C++
--   checks every UpdateAI tick, i.e. combat only).
-- - DoMeleeAttackIfReady: engine-driven in Go (ouro precedent).
-- - The UpdateAI casting-state early return (UNIT_STATE_CASTING skips
--   remaining events + the enrage/melee legs for the tick) has no
--   casting-state bridge (aeonus case); the ported pumps fire each arm
--   independently.
-- Documented-only (no bridges):
-- - spell_mark_of_frost SpellScript (MarkOfFrostTargetSelector target
--   filter + hit-effect aura 23186 application): no SpellScript
--   bridge (standing).

local SAY_TELEPORT = 0

local SPELL_MARK_OF_FROST = 23182
local SPELL_MARK_OF_FROST_AURA = 23184
local SPELL_MANA_STORM = 21097
local SPELL_CHILL = 21098
local SPELL_FROST_BREATH = 21099
local SPELL_REFLECT = 22067
local SPELL_CLEAVE = 8255
local SPELL_ENRAGE = 23537

local ENTRY = 6109

local timers = {}
local enraged = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
    enraged[guid] = nil
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

local function castOnVictim(creature, guid, key, spell, rearm)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, spell)
    end
    schedule(guid, key, rearm, function()
        castOnVictim(creature, guid, key, spell, rearm)
    end)
end

-- C++ SelectTarget(Random, 0, 60.0f, playerOnly=true): random alive
-- player within 60yd from the threat list.
local function onManaStorm(creature, guid)
    local candidates = {}
    for _, unit in ipairs(creature:GetAITargets()) do
        if unit:GetObjectType() == "Player" and not unit:IsDead()
                and creature:GetDistance(unit) <= 60 then
            candidates[#candidates + 1] = unit
        end
    end
    if #candidates > 0 then
        creature:CastSpell(candidates[math.random(#candidates)], SPELL_MANA_STORM)
    end
    schedule(guid, "manastorm", math.random(7500, 12500), function()
        onManaStorm(creature, guid)
    end)
end

local function onTeleport(creature, guid)
    creature:Talk(SAY_TELEPORT)
    local bx, by, bz = creature:GetX(), creature:GetY(), creature:GetZ()
    for _, unit in ipairs(creature:GetAITargets()) do
        if unit:GetObjectType() == "Player" then
            unit:TeleportNear(bx, by, bz + 3, unit:GetO())
        end
    end
    -- C++ EVENT_TELEPORT: ResetThreatList() after the player teleports
    -- (bridged lua_objects.go:389; the old "no threat bridge" header
    -- claim was stale).
    creature:ResetAllThreat()
    schedule(guid, "teleport", 30000, function()
        onTeleport(creature, guid)
    end)
end

local function onReflect(creature, guid)
    creature:CastSpell(creature, SPELL_REFLECT)
    schedule(guid, "reflect", math.random(20000, 35000), function()
        onReflect(creature, guid)
    end)
end

local function onEnragePump(creature, guid)
    if not enraged[guid] then
        local maxHealth = creature:GetMaxHealth()
        if maxHealth > 0
                and creature:GetHealth() < math.floor(maxHealth * 26 / 100) then
            creature:CastSpell(creature, SPELL_ENRAGE)
            enraged[guid] = true
        end
    end
    schedule(guid, "enrage", 1000, function()
        onEnragePump(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    -- boss_azuregos.cpp JustEngagedWith: triggered self-cast of the
    -- mark-of-frost aura, then the event schedule.
    creature:CastSpell(creature, SPELL_MARK_OF_FROST_AURA, true)
    schedule(guid, "manastorm", math.random(5000, 17000), function()
        onManaStorm(creature, guid)
    end)
    schedule(guid, "chill", math.random(10000, 30000), function()
        castOnVictim(creature, guid, "chill", SPELL_CHILL, math.random(13000, 25000))
    end)
    schedule(guid, "breath", math.random(2000, 8000), function()
        castOnVictim(creature, guid, "breath", SPELL_FROST_BREATH, math.random(10000, 15000))
    end)
    schedule(guid, "teleport", 30000, function()
        onTeleport(creature, guid)
    end)
    schedule(guid, "reflect", math.random(15000, 30000), function()
        onReflect(creature, guid)
    end)
    schedule(guid, "cleave", 7000, function()
        castOnVictim(creature, guid, "cleave", SPELL_CLEAVE, 7000)
    end)
    schedule(guid, "enrage", 1000, function()
        onEnragePump(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)
-- C++ KilledUnit: player victim -> Mark of Frost 23182 triggered; the
-- C++ caster is the victim itself (who->CastSpell(who, ...)) but
-- player:CastSpell has no bridge, so the boss casts it (anetheron
-- sleep precedent).
RegisterCreatureEvent(ENTRY, 3, function(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:CastSpell(victim, SPELL_MARK_OF_FROST, true)
    end
end)
