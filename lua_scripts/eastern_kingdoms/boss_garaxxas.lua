-- Garaxxas (Magister's Terrace) --
-- Lua port of the boss_garaxxas class in
-- src/server/scripts/EasternKingdoms/MagistersTerrace/
-- boss_priestess_delrissa.cpp. File's sixth lackey slice per the
-- CHECKPOINT plan (boss class done: entry 24560 registered; eramas_
-- brightblaze done: entry 24554 registered; kagani_nightstrike done:
-- entry 24557 registered; ellris_duskhallow done: entry 24558
-- registered; yazzai done: entry 24561 registered; warlord_salaris
-- done: entry 24559 registered; the two remaining lackey classes
-- remain queued — apoko 24553, zelfan 24556 — entries all from the
-- file's own m_auiAddEntries; instance_magisters_terrace.cpp stays
-- blocked on the instance-script model).
-- Entry (verifiable from the C++ sources): the file's own
-- m_auiAddEntries array names 24555 //Garaxxas (line 102).
-- The creature_template ScriptName binding is DB-side (no TDB in
-- this workspace), but the entry itself is C++-verifiable so the
-- port is registered (boss_felblood_kaelthas precedent). Script
-- name is the stringified class name (CreatureScript(
-- "boss_garaxxas"); RegisterCreatureAIWithFactory convention,
-- felblood precedent). AI struct boss_garaxxasAI derives from
-- boss_priestess_lackey_commonAI (the file's shared lackey base).
-- Verifiable numbers (file's own enums, HunterSpells, lines
-- 991-1000): SPELL_AIMED_SHOT = 44271 / SPELL_SHOOT = 15620 /
-- SPELL_CONCUSSIVE_SHOT = 27634 / SPELL_MULTI_SHOT = 31942 /
-- SPELL_WING_CLIP = 44286 / SPELL_FREEZING_TRAP = 44136 /
-- NPC_SLIVER = 24552 / SPELL_HEALING_POTION = 15503 (lackey-common
-- enum, line 348).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 9 OnDamageTaken (lua_creature_events.go fires it with (event,
-- creature, attacker, damage)), 23 OnReset. Driven by CreateLuaEvent
-- timers; melee is engine-driven in Go (creature combat tick), like
-- C++ DoMeleeAttackIfReady.
-- Ported arms (C++ UpdateAI shot timers + the common AI's sub-25%
-- healing-potion latch):
-- Aimed-shot machine: DoCastVictim(SPELL_AIMED_SHOT 44271) 6000ms
-- init -> 6000ms loop, GetVictim nil-guarded (incarcerator
-- convention).
-- Shoot machine: DoCastVictim(SPELL_SHOOT 15620) 2500ms init ->
-- 2500ms loop, same conventions.
-- Concussive-shot machine: DoCastVictim(SPELL_CONCUSSIVE_SHOT
-- 27634) 8000ms init -> 8000ms loop, same conventions.
-- Multi-shot machine: DoCastVictim(SPELL_MULTI_SHOT 31942) 10000ms
-- init -> 10000ms loop, same conventions.
-- Healing-potion latch (common-AI UpdateAI arm: !UsedPotion &&
-- HealthBelowPct(25) -> DoCast(me, SPELL_HEALING_POTION 15503),
-- UsedPotion = true): via OnDamageTaken(9), per-guid one-shot,
-- post-damage health check (gyth convention), reset on
-- OnEnterCombat(1) (C++ Initialize runs in the constructor and
-- Reset — per-engagement reset is the gyth/vaelastrasz convention).
-- Timers start on OnEnterCombat(1), cancelled on OnLeaveCombat(2)/
-- OnDied(4)/OnReset(23) (maiden convention).
-- Deviations from C++: in C++ the shot machines fire only in the
-- else branch (not within ATTACK_DISTANCE of the victim), while
-- wing-clip / freezing-trap / melee fire within ATTACK_DISTANCE.
-- No distance model exists on the Lua surface, so the four
-- DoCastVictim loops fire regardless of range. Also no
-- UNIT_STATE_CASTING model in Go (casts are packet-visual), so the
-- timers fire unconditionally and the in-loop casting gates are
-- dropped (maiden precedent). The C++ scheduler is driven from
-- UpdateAI after UpdateVictim; the port schedules from
-- OnEnterCombat(1) and cancels on 2/4/23 (maiden convention).
-- Reset() Initialize() / boss_priestess_lackey_commonAI::Reset() /
-- ::UpdateAI() are internal machinery covered by the cancel/re-arm on
-- combat events (halycon precedent).
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - Wing-clip machine (DoCastVictim(SPELL_WING_CLIP 44286) 4000ms
--   init -> 4000ms loop) — fires only within ATTACK_DISTANCE (no
--   distance bridge).
-- - Freezing-trap machine (DoCastVictim(SPELL_FREEZING_TRAP 44136)
--   15000ms init -> 15000ms loop, with the me->GetGameObject(
--   SPELL_FREEZING_TRAP) one-trap-at-a-time re-arm to 2500ms) —
--   within-ATTACK_DISTANCE gate AND the gameobject lookup have no
--   bridges.
-- - Reset's sliver-pet leg (me->SummonCreature(NPC_SLIVER 24552, ...)
--   + JustSummoned m_uiPetGUID tracking + ObjectAccessor re-acquire)
--   — no summon bridge (razorgore precedent).
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
local SPELL_AIMED_SHOT = 44271
local SPELL_SHOOT = 15620
local SPELL_CONCUSSIVE_SHOT = 27634
local SPELL_MULTI_SHOT = 31942
local SPELL_HEALING_POTION = 15503

local ENTRY_GARAXXAS = 24555

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

local function onAimedShot(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_AIMED_SHOT)
    end
    schedule(guid, "aimedshot", 6000, function() onAimedShot(creature, guid) end)
end

local function onShoot(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SHOOT)
    end
    schedule(guid, "shoot", 2500, function() onShoot(creature, guid) end)
end

local function onConcussiveShot(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CONCUSSIVE_SHOT)
    end
    schedule(guid, "concussiveshot", 8000, function() onConcussiveShot(creature, guid) end)
end

local function onMultiShot(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_MULTI_SHOT)
    end
    schedule(guid, "multishot", 10000, function() onMultiShot(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    usedPotion[guid] = false
    schedule(guid, "aimedshot", 6000, function() onAimedShot(creature, guid) end)
    schedule(guid, "shoot", 2500, function() onShoot(creature, guid) end)
    schedule(guid, "concussiveshot", 8000, function() onConcussiveShot(creature, guid) end)
    schedule(guid, "multishot", 10000, function() onMultiShot(creature, guid) end)
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

RegisterCreatureEvent(ENTRY_GARAXXAS, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_GARAXXAS, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_GARAXXAS, 4, onDied)
RegisterCreatureEvent(ENTRY_GARAXXAS, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY_GARAXXAS, 23, onCombatEnd)
