-- Zelfan (Magister's Terrace) --
-- Lua port of the boss_zelfan class in
-- src/server/scripts/EasternKingdoms/MagistersTerrace/
-- boss_priestess_delrissa.cpp. File's eighth and last lackey slice
-- per the CHECKPOINT plan (boss class done: entry 24560 registered;
-- eramas_brightblaze done: entry 24554 registered; kagani_
-- nightstrike done: entry 24557 registered; ellris_duskhallow done:
-- entry 24558 registered; yazzai done: entry 24561 registered;
-- warlord_salaris done: entry 24559 registered; garaxxas done:
-- entry 24555 registered; apoko done: entry 24553 registered —
-- entries all from the file's own m_auiAddEntries; instance_
-- magisters_terrace.cpp stays blocked on the instance-script
-- model).
-- Entry (verifiable from the C++ sources): the file's own
-- m_auiAddEntries array names 24556 //Zelfan (line 105).
-- The creature_template ScriptName binding is DB-side (no TDB in
-- this workspace), but the entry itself is C++-verifiable so the
-- port is registered (boss_felblood_kaelthas precedent). Script
-- name is the stringified class name (CreatureScript(
-- "boss_zelfan"); RegisterCreatureAIWithFactory convention,
-- felblood precedent). AI struct boss_zelfanAI derives from
-- boss_priestess_lackey_commonAI (the file's shared lackey base).
-- Verifiable numbers (file's own enums, Engineer block, lines
-- 1217-1224): SPELL_GOBLIN_DRAGON_GUN = 44272 / SPELL_ROCKET_
-- LAUNCH = 44137 / SPELL_RECOMBOBULATE = 44274 / SPELL_HIGH_
-- EXPLOSIVE_SHEEP = 44276 / SPELL_FEL_IRON_BOMB = 46024 /
-- SPELL_HEALING_POTION = 15503 (lackey-common enum, line 348).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 9 OnDamageTaken (lua_creature_events.go fires it with (event,
-- creature, attacker, damage)), 23 OnReset. Driven by CreateLuaEvent
-- timers; melee is engine-driven in Go (creature combat tick), like
-- C++ DoMeleeAttackIfReady.
-- Ported arms (C++ UpdateAI timers + the common AI's sub-25%
-- healing-potion latch):
-- Goblin-dragon-gun machine: DoCastVictim(SPELL_GOBLIN_DRAGON_GUN
-- 44272) 20000ms init -> 10000ms loop via GetVictim nil-guarded
-- (incarcerator convention).
-- Rocket-launch machine: DoCastVictim(SPELL_ROCKET_LAUNCH 44137)
-- 7000ms init -> 9000ms loop, same conventions.
-- Fel-iron-bomb machine: DoCastVictim(SPELL_FEL_IRON_BOMB 46024)
-- 15000ms init -> 15000ms loop, same conventions.
-- High-explosive-sheep machine: DoCast(me, SPELL_HIGH_EXPLOSIVE_
-- SHEEP 44276) 10000ms init -> 65000ms loop.
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
-- - Recombobulate machine (4000ms init -> 2000ms loop: rings over
--   m_auiLackeyGUIDs via ObjectAccessor and casts 44274 on the
--   first polymorphed lackey — no GUID-list / IsPolymorphed
--   bridges).
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
local SPELL_GOBLIN_DRAGON_GUN = 44272
local SPELL_ROCKET_LAUNCH = 44137
local SPELL_HIGH_EXPLOSIVE_SHEEP = 44276
local SPELL_FEL_IRON_BOMB = 46024
local SPELL_HEALING_POTION = 15503

local ENTRY_ZELFAN = 24556

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

local function onGoblinDragonGun(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_GOBLIN_DRAGON_GUN)
    end
    schedule(guid, "dragongun", 10000, function() onGoblinDragonGun(creature, guid) end)
end

local function onRocketLaunch(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_ROCKET_LAUNCH)
    end
    schedule(guid, "rocketlaunch", 9000, function() onRocketLaunch(creature, guid) end)
end

local function onFelIronBomb(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FEL_IRON_BOMB)
    end
    schedule(guid, "felironbomb", 15000, function() onFelIronBomb(creature, guid) end)
end

local function onHighExplosiveSheep(creature, guid)
    creature:CastSpell(creature, SPELL_HIGH_EXPLOSIVE_SHEEP)
    schedule(guid, "sheep", 65000, function() onHighExplosiveSheep(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    usedPotion[guid] = false
    schedule(guid, "dragongun", 20000, function() onGoblinDragonGun(creature, guid) end)
    schedule(guid, "rocketlaunch", 7000, function() onRocketLaunch(creature, guid) end)
    schedule(guid, "felironbomb", 15000, function() onFelIronBomb(creature, guid) end)
    schedule(guid, "sheep", 10000, function() onHighExplosiveSheep(creature, guid) end)
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

RegisterCreatureEvent(ENTRY_ZELFAN, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_ZELFAN, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_ZELFAN, 4, onDied)
RegisterCreatureEvent(ENTRY_ZELFAN, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY_ZELFAN, 23, onCombatEnd)
