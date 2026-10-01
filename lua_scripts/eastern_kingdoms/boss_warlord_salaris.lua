-- Warlord Salaris (Magister's Terrace) --
-- Lua port of the boss_warlord_salaris class in
-- src/server/scripts/EasternKingdoms/MagistersTerrace/
-- boss_priestess_delrissa.cpp. File's fifth lackey slice per the
-- CHECKPOINT plan (boss class done: entry 24560 registered; eramas_
-- brightblaze done: entry 24554 registered; kagani_nightstrike done:
-- entry 24557 registered; ellris_duskhallow done: entry 24558
-- registered; yazzai done: entry 24561 registered; the other three
-- lackey classes remain queued — garaxxas 24555, apoko 24553,
-- zelfan 24556 — entries all from the file's own m_auiAddEntries;
-- instance_magisters_terrace.cpp stays blocked on the instance-
-- script model).
-- Entry (verifiable from the C++ sources): the file's own
-- m_auiAddEntries array names 24559 //Warlord Salaris (line 102).
-- The creature_template ScriptName binding is DB-side (no TDB in
-- this workspace), but the entry itself is C++-verifiable so the
-- port is registered (boss_felblood_kaelthas precedent). Script
-- name is the stringified class name (CreatureScript(
-- "boss_warlord_salaris"); RegisterCreatureAIWithFactory
-- convention, felblood precedent). AI struct
-- boss_warlord_salarisAI derives from boss_priestess_lackey_
-- commonAI (the file's shared lackey base).
-- Verifiable numbers (file's own enums, WarriorSpells, lines
-- 876-885): SPELL_INTERCEPT_STUN = 27577 / SPELL_DISARM = 27581 /
-- SPELL_PIERCING_HOWL = 23600 / SPELL_FRIGHTENING_SHOUT = 19134 /
-- SPELL_HAMSTRING = 27584 / SPELL_BATTLE_SHOUT = 27578 /
-- SPELL_MORTAL_STRIKE = 44268 / SPELL_HEALING_POTION = 15503
-- (lackey-common enum, line 348).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 9 OnDamageTaken (lua_creature_events.go fires it with (event,
-- creature, attacker, damage)), 23 OnReset. Driven by CreateLuaEvent
-- timers; melee is engine-driven in Go (creature combat tick), like
-- C++ DoMeleeAttackIfReady.
-- Ported arms (C++ JustEngagedWith battle-shout self-cast +
-- UpdateAI disarm/hamstring/mortal-strike/piercing-howl/
-- frightening-shout timers + the common AI's sub-25% healing-
-- potion latch):
-- Battle-shout: JustEngagedWith DoCast(me, SPELL_BATTLE_SHOUT 27578)
-- is a one-shot self-cast per engagement, so it fires in
-- OnEnterCombat(1) directly (C++ Initialize runs in the constructor
-- and Reset — per-engagement reset is the gyth/vaelastrasz
-- convention).
-- Disarm machine: DoCastVictim(SPELL_DISARM 27581) 6000ms init ->
-- 6000ms loop, GetVictim nil-guarded (incarcerator convention).
-- Hamstring machine: DoCastVictim(SPELL_HAMSTRING 27584) 4500ms
-- init -> 4500ms loop, same conventions.
-- Mortal-strike machine: DoCastVictim(SPELL_MORTAL_STRIKE 44268)
-- 8000ms init -> 4500ms loop, same conventions (C++ re-arms
-- Mortal_Strike_Timer to 4500, not 8000 — C++-exact).
-- Piercing-howl machine: DoCastVictim(SPELL_PIERCING_HOWL 23600)
-- 10000ms init -> 10000ms loop, same conventions.
-- Frightening-shout machine: DoCastVictim(SPELL_FRIGHTENING_SHOUT
-- 19134) 18000ms init -> 18000ms loop, same conventions.
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
-- - Intercept machine (GetCombatManager().GetPvECombatRefs() loop:
--   nobody IsWithinMeleeRange -> DoCast(SelectTarget(Random, 0),
--   SPELL_INTERCEPT_STUN 27577), 500ms init -> 10000ms loop) — gates
--   on the combat-manager melee-range loop AND SelectTarget (the_
--   beast / doomwalker precedent), so no timer was ported for it.
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
local SPELL_DISARM = 27581
local SPELL_PIERCING_HOWL = 23600
local SPELL_FRIGHTENING_SHOUT = 19134
local SPELL_HAMSTRING = 27584
local SPELL_BATTLE_SHOUT = 27578
local SPELL_MORTAL_STRIKE = 44268
local SPELL_HEALING_POTION = 15503

local ENTRY_WARLORD_SALARIS = 24559

local timers = {}
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

local function onDisarm(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_DISARM)
    end
    schedule(guid, "disarm", 6000, function() onDisarm(creature, guid) end)
end

local function onHamstring(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_HAMSTRING)
    end
    schedule(guid, "hamstring", 4500, function() onHamstring(creature, guid) end)
end

local function onMortalStrike(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_MORTAL_STRIKE)
    end
    schedule(guid, "mortalstrike", 4500, function() onMortalStrike(creature, guid) end)
end

local function onPiercingHowl(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_PIERCING_HOWL)
    end
    schedule(guid, "piercinghowl", 10000, function() onPiercingHowl(creature, guid) end)
end

local function onFrighteningShout(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FRIGHTENING_SHOUT)
    end
    schedule(guid, "frighteningshout", 18000, function() onFrighteningShout(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    usedPotion[guid] = false
    creature:CastSpell(creature, SPELL_BATTLE_SHOUT)
    schedule(guid, "disarm", 6000, function() onDisarm(creature, guid) end)
    schedule(guid, "hamstring", 4500, function() onHamstring(creature, guid) end)
    schedule(guid, "mortalstrike", 8000, function() onMortalStrike(creature, guid) end)
    schedule(guid, "piercinghowl", 10000, function() onPiercingHowl(creature, guid) end)
    schedule(guid, "frighteningshout", 18000, function() onFrighteningShout(creature, guid) end)
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

RegisterCreatureEvent(ENTRY_WARLORD_SALARIS, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_WARLORD_SALARIS, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_WARLORD_SALARIS, 4, onDied)
RegisterCreatureEvent(ENTRY_WARLORD_SALARIS, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY_WARLORD_SALARIS, 23, onCombatEnd)
