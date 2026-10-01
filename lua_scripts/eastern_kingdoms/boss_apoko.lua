-- Apoko (Magister's Terrace) --
-- Lua port of the boss_apoko class in
-- src/server/scripts/EasternKingdoms/MagistersTerrace/
-- boss_priestess_delrissa.cpp. File's seventh lackey slice per the
-- CHECKPOINT plan (boss class done: entry 24560 registered; eramas_
-- brightblaze done: entry 24554 registered; kagani_nightstrike done:
-- entry 24557 registered; ellris_duskhallow done: entry 24558
-- registered; yazzai done: entry 24561 registered; warlord_salaris
-- done: entry 24559 registered; garaxxas done: entry 24555
-- registered; the one remaining lackey class remains queued —
-- zelfan 24556 — entries all from the file's own m_auiAddEntries;
-- instance_magisters_terrace.cpp stays blocked on the instance-
-- script model).
-- Entry (verifiable from the C++ sources): the file's own
-- m_auiAddEntries array names 24553 //Apoko (line 104).
-- The creature_template ScriptName binding is DB-side (no TDB in
-- this workspace), but the entry itself is C++-verifiable so the
-- port is registered (boss_felblood_kaelthas precedent). Script
-- name is the stringified class name (CreatureScript(
-- "boss_apoko"); RegisterCreatureAIWithFactory convention,
-- felblood precedent). AI struct boss_apokoAI derives from
-- boss_priestess_lackey_commonAI (the file's shared lackey base).
-- Verifiable numbers (file's own enums, Apoko block, lines 70-78):
-- SPELL_WINDFURY_TOTEM = 27621 / SPELL_WAR_STOMP = 46026 /
-- SPELL_PURGE = 27626 / SPELL_LESSER_HEALING_WAVE = 44256 /
-- SPELL_FROST_SHOCK = 21401 / SPELL_FIRE_NOVA_TOTEM = 44257 /
-- SPELL_EARTHBIND_TOTEM = 15786 / SPELL_HEALING_POTION = 15503
-- (lackey-common enum, line 348).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 9 OnDamageTaken (lua_creature_events.go fires it with (event,
-- creature, attacker, damage)), 23 OnReset. Driven by CreateLuaEvent
-- timers; melee is engine-driven in Go (creature combat tick), like
-- C++ DoMeleeAttackIfReady.
-- Ported arms (C++ UpdateAI timers + the common AI's sub-25%
-- healing-potion latch):
-- Totem machine: DoCast(me, RAND(SPELL_WINDFURY_TOTEM 27621,
-- SPELL_FIRE_NOVA_TOTEM 44257, SPELL_EARTHBIND_TOTEM 15786))
-- 2000ms init; after each fire ++Totem_Amount and re-arm
-- Totem_Amount*2000 (2000, 4000, 6000, ...) (C++-exact; RAND over
-- the three totem IDs via math.random, boss_the_beast precedent).
-- War-stomp machine: DoCast(me, SPELL_WAR_STOMP 46026) 10000ms
-- init -> 10000ms loop.
-- Frost-shock machine: DoCastVictim(SPELL_FROST_SHOCK 21401)
-- 7000ms init -> 7000ms loop via GetVictim nil-guarded
-- (incarcerator convention).
-- Lesser-healing-wave machine: DoCast(me, SPELL_LESSER_HEALING_WAVE
-- 44256) 5000ms init -> 5000ms loop.
-- Healing-potion latch (common-AI UpdateAI arm: !UsedPotion &&
-- HealthBelowPct(25) -> DoCast(me, SPELL_HEALING_POTION 15503),
-- UsedPotion = true): via OnDamageTaken(9), per-guid one-shot,
-- post-damage health check (gyth convention), reset on
-- OnEnterCombat(1) (C++ Initialize runs in the constructor and
-- Reset — per-engagement reset is the gyth/vaelastrasz convention).
-- Timers start on OnEnterCombat(1), cancelled on OnLeaveCombat(2)/
-- OnDied(4)/OnReset(23) (maiden convention).
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (casts are
-- packet-visual), so the timers fire unconditionally and the in-loop
-- casting gates are dropped (maiden precedent). The C++ scheduler is
-- driven from UpdateAI after UpdateVictim; the port schedules from
-- OnEnterCombat(1) and cancels on 2/4/23 (maiden convention).
-- Reset() Initialize() / boss_priestess_lackey_commonAI::Reset() /
-- ::UpdateAI() are internal machinery covered by the cancel/re-arm on
-- combat events (halycon precedent).
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - Purge machine (DoCast(SelectTarget(Random, 0), SPELL_PURGE
--   27626) 8000ms init -> 15000ms loop — no SelectTarget bridge).
-- - Common-AI JustEngagedWith: the lackey AddThreat(who, 0.0f, pAdd)
--   ring over m_auiLackeyGUIDs via ObjectAccessor (no GUID-list
--   bridge) and the Delrissa threat add via instance->GetCreature
--   (no instance bridge — standing blocker).
-- - Common-AI JustDied: the DATA_DELRISSA_DEATH_COUNT death-count
--   talk to Delrissa, instance->SetData, and the lootable-flag / boss-
--   state-DONE machine — no instance bridge (standing blocker).
-- - Common-AI KilledUnit: forwards to Delrissa's KilledUnit via
--   instance->GetCreature — no instance bridge.
-- - Common-AI Reset: AcquireGUIDs (instance bridge) and the respawn-
--   of-Delrissa leg (instance bridge).
-- - Common-AI ResetThreatTimer (urand(5000, 20000) -> ResetThreatList)
--   — no threat-list bridge.
local SPELL_WINDFURY_TOTEM = 27621
local SPELL_FIRE_NOVA_TOTEM = 44257
local SPELL_EARTHBIND_TOTEM = 15786
local SPELL_WAR_STOMP = 46026
local SPELL_FROST_SHOCK = 21401
local SPELL_LESSER_HEALING_WAVE = 44256
local SPELL_HEALING_POTION = 15503

local ENTRY_APOKO = 24553

local TOTEM_SPELLS = {SPELL_WINDFURY_TOTEM, SPELL_FIRE_NOVA_TOTEM, SPELL_EARTHBIND_TOTEM}

local timers = {}
local totemAmount = {}
local usedPotion = {}

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

local function onTotem(creature, guid)
    creature:CastSpell(creature, TOTEM_SPELLS[math.random(1, 3)])
    totemAmount[guid] = totemAmount[guid] + 1
    schedule(guid, "totem", totemAmount[guid] * 2000, function() onTotem(creature, guid) end)
end

local function onWarStomp(creature, guid)
    creature:CastSpell(creature, SPELL_WAR_STOMP)
    schedule(guid, "warstomp", 10000, function() onWarStomp(creature, guid) end)
end

local function onFrostShock(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FROST_SHOCK)
    end
    schedule(guid, "frostshock", 7000, function() onFrostShock(creature, guid) end)
end

local function onHealingWave(creature, guid)
    creature:CastSpell(creature, SPELL_LESSER_HEALING_WAVE)
    schedule(guid, "healingwave", 5000, function() onHealingWave(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    totemAmount[guid] = 1
    usedPotion[guid] = false
    schedule(guid, "totem", 2000, function() onTotem(creature, guid) end)
    schedule(guid, "warstomp", 10000, function() onWarStomp(creature, guid) end)
    schedule(guid, "frostshock", 7000, function() onFrostShock(creature, guid) end)
    schedule(guid, "healingwave", 5000, function() onHealingWave(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

local function onDamageTaken(_, creature, _, damage)
    local guid = creature:GetGUID()
    if usedPotion[guid] then
        return
    end
    local postDamageHealth = creature:GetHealth() - damage
    if postDamageHealth < creature:GetMaxHealth() * 25 / 100 then
        usedPotion[guid] = true
        creature:CastSpell(creature, SPELL_HEALING_POTION)
    end
end

local function onDied(_, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_APOKO, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_APOKO, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_APOKO, 4, onDied)
RegisterCreatureEvent(ENTRY_APOKO, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY_APOKO, 23, onCombatEnd)
