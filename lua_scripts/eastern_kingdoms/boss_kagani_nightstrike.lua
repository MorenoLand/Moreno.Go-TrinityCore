-- Kagani Nightstrike (Magister's Terrace) --
-- Lua port of the boss_kagani_nightstrike class in
-- src/server/scripts/EasternKingdoms/MagistersTerrace/
-- boss_priestess_delrissa.cpp. File's second lackey slice per the
-- CHECKPOINT plan (boss class done: entry 24560 registered; eramas_
-- brightblaze done: entry 24554 registered; the other six lackey
-- classes remain queued — ellris_duskhallow 24558, yazzai 24561,
-- warlord_salaris 24559, garaxxas 24555, apoko 24553, zelfan 24556 —
-- entries all from the file's own m_auiAddEntries; instance_
-- magisters_terrace.cpp stays blocked on the instance-script model).
-- Entry (verifiable from the C++ sources): the file's own
-- m_auiAddEntries array names 24557 //Kagani Nightstrike (line 97).
-- The creature_template ScriptName binding is DB-side (no TDB in
-- this workspace), but the entry itself is C++-verifiable so the
-- port is registered (boss_felblood_kaelthas precedent). Script name
-- is the stringified class name (CreatureScript("boss_kagani_
-- nightstrike"); RegisterCreatureAIWithFactory convention, felblood
-- precedent). AI struct boss_kagani_nightstrikeAI derives from
-- boss_priestess_lackey_commonAI (the file's shared lackey base).
-- Verifiable numbers (file's own enums, RogueSpells): SPELL_KIDNEY_
-- SHOT = 27615 / SPELL_GOUGE = 12540 / SPELL_KICK = 27613 / SPELL_
-- VANISH = 44290 / SPELL_BACKSTAB = 15657 / SPELL_EVISCERATE = 27611
-- / SPELL_HEALING_POTION = 15503 (lackey-common enum, line 348) /
-- MAX_ACTIVE_LACKEY = 4.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 9 OnDamageTaken (lua_creature_events.go fires it with (event,
-- creature, attacker, damage)), 23 OnReset. Driven by CreateLuaEvent
-- timers; melee is engine-driven in Go (creature combat tick), like
-- C++ DoMeleeAttackIfReady.
-- Ported arms (C++ UpdateAI gouge/kick/eviscerate timers + the common
-- AI's sub-25% healing-potion latch):
-- Gouge machine: DoCastVictim(SPELL_GOUGE 12540) 5500ms init ->
-- 5500ms loop, GetVictim nil-guarded (incarcerator convention).
-- Kick machine: DoCastVictim(SPELL_KICK 27613) 7000ms init ->
-- 7000ms loop, GetVictim nil-guarded (incarcerator convention).
-- Eviscerate machine: DoCastVictim(SPELL_EVISCERATE 27611) 6000ms
-- init -> 4000ms loop, GetVictim nil-guarded (incarcerator
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
-- - The vanish machine (DoCast(me, SPELL_VANISH 44290) 2000ms init
--   -> 30000ms loop; SelectTarget(Random, 0) -> ResetThreatList() +
--   AddThreat(unit, 1000.0f), InVanish = true, Wait_Timer = 10000)
--   — gates on SelectTarget + threat-list bridges (the_beast /
--   doomwalker precedent), so no timer was ported for it.
-- - The InVanish leg (Wait_Timer -> DoCastVictim(SPELL_BACKSTAB
--   15657, true) + DoCastVictim(SPELL_KIDNEY_SHOT 27615, true) +
--   me->SetVisible(true), InVanish = false; melee skipped while
--   InVanish) — rides on the vanish machine's bridges; no
--   creature SetVisible bridge on the Lua surface either.
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
--   of-Delrissa leg (instance bridge); me->SetVisible(true) on the
--   Reset override shares the missing SetVisible bridge with the
--   vanish leg.
-- - Common-AI ResetThreatTimer (urand(5000, 20000) -> ResetThreatList)
--   — no threat-list bridge.
local SPELL_GOUGE = 12540
local SPELL_KICK = 27613
local SPELL_EVISCERATE = 27611
local SPELL_HEALING_POTION = 15503

local ENTRY_KAGANI_NIGHTSTRIKE = 24557

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

local function onGouge(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_GOUGE)
    end
    schedule(guid, "gouge", 5500, function() onGouge(creature, guid) end)
end

local function onKick(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_KICK)
    end
    schedule(guid, "kick", 7000, function() onKick(creature, guid) end)
end

local function onEviscerate(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_EVISCERATE)
    end
    schedule(guid, "eviscerate", 4000, function() onEviscerate(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    usedPotion[guid] = false
    schedule(guid, "gouge", 5500, function() onGouge(creature, guid) end)
    schedule(guid, "kick", 7000, function() onKick(creature, guid) end)
    schedule(guid, "eviscerate", 6000, function() onEviscerate(creature, guid) end)
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

RegisterCreatureEvent(ENTRY_KAGANI_NIGHTSTRIKE, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_KAGANI_NIGHTSTRIKE, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_KAGANI_NIGHTSTRIKE, 4, onDied)
RegisterCreatureEvent(ENTRY_KAGANI_NIGHTSTRIKE, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY_KAGANI_NIGHTSTRIKE, 23, onCombatEnd)
