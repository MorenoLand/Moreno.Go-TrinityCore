-- Yazzai (Magister's Terrace) --
-- Lua port of the boss_yazzai class in
-- src/server/scripts/EasternKingdoms/MagistersTerrace/
-- boss_priestess_delrissa.cpp. File's fourth lackey slice per the
-- CHECKPOINT plan (boss class done: entry 24560 registered; eramas_
-- brightblaze done: entry 24554 registered; kagani_nightstrike done:
-- entry 24557 registered; ellris_duskhallow done: entry 24558
-- registered; the other four lackey classes remain queued —
-- warlord_salaris 24559, garaxxas 24555, apoko 24553, zelfan 24556 —
-- entries all from the file's own m_auiAddEntries; instance_
-- magisters_terrace.cpp stays blocked on the instance-script model).
-- Entry (verifiable from the C++ sources): the file's own
-- m_auiAddEntries array names 24561 //Yazzaj (line 101). The
-- creature_template ScriptName binding is DB-side (no TDB in this
-- workspace), but the entry itself is C++-verifiable so the port is
-- registered (boss_felblood_kaelthas precedent). Script name is the
-- stringified class name (CreatureScript("boss_yazzai");
-- RegisterCreatureAIWithFactory convention, felblood precedent). AI
-- struct boss_yazzaiAI derives from boss_priestess_lackey_commonAI
-- (the file's shared lackey base).
-- Verifiable numbers (file's own enums, MageSpells, lines 745-754):
-- SPELL_POLYMORPH = 13323 / SPELL_ICE_BLOCK = 27619 / SPELL_BLIZZARD
-- = 44178 / SPELL_ICE_LANCE = 46194 / SPELL_CONE_OF_COLD = 38384 /
-- SPELL_FROSTBOLT = 15043 / SPELL_BLINK = 14514 / SPELL_HEALING_
-- POTION = 15503 (lackey-common enum, line 348).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 9 OnDamageTaken (lua_creature_events.go fires it with (event,
-- creature, attacker, damage)), 23 OnReset. Driven by CreateLuaEvent
-- timers; melee is engine-driven in Go (creature combat tick), like
-- C++ DoMeleeAttackIfReady.
-- Ported arms (C++ UpdateAI frostbolt/cone-of-cold/ice-lance timers +
-- the mage's own sub-35% ice-block self-latch + the common AI's
-- sub-25% healing-potion latch):
-- Frostbolt machine: DoCastVictim(SPELL_FROSTBOLT 15043) 3000ms init
-- -> 8000ms loop, GetVictim nil-guarded (incarcerator convention).
-- Cone-of-cold machine: DoCastVictim(SPELL_CONE_OF_COLD 38384)
-- 10000ms init -> 10000ms loop, GetVictim nil-guarded (incarcerator
-- convention).
-- Ice-lance machine: DoCastVictim(SPELL_ICE_LANCE 46194) 12000ms
-- init -> 12000ms loop, GetVictim nil-guarded (incarcerator
-- convention).
-- Ice-block latch (UpdateAI arm: HealthBelowPct(35) && !HasIceBlocked
-- -> DoCast(me, SPELL_ICE_BLOCK 27619), HasIceBlocked = true): via
-- OnDamageTaken(9), per-guid one-shot, post-damage health check (gyth
-- convention), reset on OnEnterCombat(1) (C++ Initialize runs in the
-- constructor and Reset — per-engagement reset is the gyth/
-- vaelastrasz convention).
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
-- - Polymorph machine (DoCast(SelectTarget(Random, 0), 13323) 1000ms
--   init -> 20000ms loop) — gates on SelectTarget (the_beast /
--   doomwalker precedent), so no timer was ported for it.
-- - Blizzard machine (DoCast(SelectTarget(Random, 0), 44178) 8000ms
--   init -> 8000ms loop) — same SelectTarget bridge absent.
-- - Blink machine (GetCombatManager().GetPvECombatRefs() loop:
--   anybody IsWithinMeleeRange -> DoCast(me, SPELL_BLINK 14514),
--   8000ms init -> 8000ms loop) — no combat-manager / melee-range
--   bridge on the Lua surface, so no timer was ported for it.
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
local SPELL_FROSTBOLT = 15043
local SPELL_CONE_OF_COLD = 38384
local SPELL_ICE_LANCE = 46194
local SPELL_ICE_BLOCK = 27619
local SPELL_HEALING_POTION = 15503

local ENTRY_YAZZAI = 24561

local timers = {}
local usedPotion = {}
local hasIceBlocked = {}

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

local function onFrostbolt(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FROSTBOLT)
    end
    schedule(guid, "frostbolt", 8000, function() onFrostbolt(creature, guid) end)
end

local function onConeOfCold(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CONE_OF_COLD)
    end
    schedule(guid, "coneofcold", 10000, function() onConeOfCold(creature, guid) end)
end

local function onIceLance(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_ICE_LANCE)
    end
    schedule(guid, "icelance", 12000, function() onIceLance(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    usedPotion[guid] = false
    hasIceBlocked[guid] = false
    schedule(guid, "frostbolt", 3000, function() onFrostbolt(creature, guid) end)
    schedule(guid, "coneofcold", 10000, function() onConeOfCold(creature, guid) end)
    schedule(guid, "icelance", 12000, function() onIceLance(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

local function onDamageTaken(_, creature, _, damage)
    local guid = creature:GetGUID()
    local postDamageHealth = creature:GetHealth() - damage
    if not hasIceBlocked[guid] then
        if postDamageHealth < creature:GetMaxHealth() * 35 / 100 then
            hasIceBlocked[guid] = true
            creature:CastSpell(creature, SPELL_ICE_BLOCK)
        end
    end
    if usedPotion[guid] then
        return
    end
    if postDamageHealth < creature:GetMaxHealth() * 25 / 100 then
        usedPotion[guid] = true
        creature:CastSpell(creature, SPELL_HEALING_POTION)
    end
end

local function onDied(_, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_YAZZAI, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_YAZZAI, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_YAZZAI, 4, onDied)
RegisterCreatureEvent(ENTRY_YAZZAI, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY_YAZZAI, 23, onCombatEnd)
