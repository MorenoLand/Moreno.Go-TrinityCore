-- Ellris Duskhallow (Magister's Terrace) --
-- Lua port of the boss_ellris_duskhallow class in
-- src/server/scripts/EasternKingdoms/MagistersTerrace/
-- boss_priestess_delrissa.cpp. File's third lackey slice per the
-- CHECKPOINT plan (boss class done: entry 24560 registered; eramas_
-- brightblaze done: entry 24554 registered; kagani_nightstrike done:
-- entry 24557 registered; the other five lackey classes remain queued
-- — yazzai 24561, warlord_salaris 24559, garaxxas 24555, apoko 24553,
-- zelfan 24556 — entries all from the file's own m_auiAddEntries;
-- instance_magisters_terrace.cpp stays blocked on the instance-script
-- model).
-- Entry (verifiable from the C++ sources): the file's own
-- m_auiAddEntries array names 24558 //Elris Duskhallow (line 99).
-- The creature_template ScriptName binding is DB-side (no TDB in
-- this workspace), but the entry itself is C++-verifiable so the
-- port is registered (boss_felblood_kaelthas precedent). Script name
-- is the stringified class name (CreatureScript("boss_ellris_
-- duskhallow"); RegisterCreatureAIWithFactory convention, felblood
-- precedent). AI struct boss_ellris_duskhallowAI derives from
-- boss_priestess_lackey_commonAI (the file's shared lackey base).
-- Verifiable numbers (file's own enums, WarlockSpells): SPELL_
-- IMMOLATE = 44267 / SPELL_SHADOW_BOLT = 12471 / SPELL_SEED_OF_
-- CORRUPTION = 44141 / SPELL_CURSE_OF_AGONY = 14875 / SPELL_FEAR =
-- 38595 / SPELL_IMP_FIREBALL = 44164 (imp pet only) / SPELL_SUMMON_
-- IMP = 44163 / SPELL_HEALING_POTION = 15503 (lackey-common enum,
-- line 348) / MAX_ACTIVE_LACKEY = 4.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 9 OnDamageTaken (lua_creature_events.go fires it with (event,
-- creature, attacker, damage)), 23 OnReset. Driven by CreateLuaEvent
-- timers; melee is engine-driven in Go (creature combat tick), like
-- C++ DoMeleeAttackIfReady.
-- Ported arms (C++ UpdateAI immolate/shadow-bolt timers + the common
-- AI's sub-25% healing-potion latch):
-- Immolate machine: DoCastVictim(SPELL_IMMOLATE 44267) 6000ms init
-- -> 6000ms loop, GetVictim nil-guarded (incarcerator convention).
-- Shadow-bolt machine: DoCastVictim(SPELL_SHADOW_BOLT 12471) 3000ms
-- init -> 5000ms loop, GetVictim nil-guarded (incarcerator
-- convention).
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
-- - JustEngagedWith's DoCast(me, SPELL_SUMMON_IMP 44163) — summon
--   bridge absent (razorgore precedent), so no imp was ported.
-- - Seed-of-corruption machine (DoCast(SelectTarget(Random, 0), 44141)
--   2000ms init -> 10000ms loop) — gates on SelectTarget (the_beast /
--   doomwalker precedent), so no timer was ported for it.
-- - Curse-of-agony machine (DoCast(SelectTarget(Random, 0), 14875)
--   1000ms init -> 13000ms loop) — same SelectTarget bridge absent.
-- - Fear machine (DoCast(SelectTarget(Random, 0), 38595) 10000ms init
--   -> 10000ms loop) — same SelectTarget bridge absent.
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
local SPELL_IMMOLATE = 44267
local SPELL_SHADOW_BOLT = 12471
local SPELL_HEALING_POTION = 15503

local ENTRY_ELLRIS_DUSKHALLOW = 24558

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

local function onImmolate(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_IMMOLATE)
    end
    schedule(guid, "immolate", 6000, function() onImmolate(creature, guid) end)
end

local function onShadowBolt(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SHADOW_BOLT)
    end
    schedule(guid, "shadowbolt", 5000, function() onShadowBolt(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    usedPotion[guid] = false
    schedule(guid, "immolate", 6000, function() onImmolate(creature, guid) end)
    schedule(guid, "shadowbolt", 3000, function() onShadowBolt(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

local function onDamageTaken(_, creature, _, damage)
    local guid = creature:GetGUID()
    if usedPotion[guid] then
        return
    end
    if creature:GetHealth() - damage < creature:GetMaxHealth() * 25 / 100 then
        usedPotion[guid] = true
        creature:CastSpell(creature, SPELL_HEALING_POTION)
    end
end

local function onDied(_, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_ELLRIS_DUSKHALLOW, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_ELLRIS_DUSKHALLOW, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_ELLRIS_DUSKHALLOW, 4, onDied)
RegisterCreatureEvent(ENTRY_ELLRIS_DUSKHALLOW, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY_ELLRIS_DUSKHALLOW, 23, onCombatEnd)
