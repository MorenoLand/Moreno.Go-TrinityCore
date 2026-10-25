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
-- Seed-of-corruption machine: DoCast(SelectTarget(Random, 0), SPELL_
-- SEED_OF_CORRUPTION 44141) 2000ms init -> 10000ms loop,
-- unconditional re-arm (boss_priestess_delrissa.cpp:648-653).
-- Curse-of-agony machine: DoCast(SelectTarget(Random, 0), SPELL_
-- CURSE_OF_AGONY 14875) 1000ms init -> 13000ms loop, unconditional
-- re-arm (:655-660).
-- Fear machine: DoCast(SelectTarget(Random, 0), SPELL_FEAR 38595)
-- 10000ms init -> 10000ms loop, unconditional re-arm (:662-667). All
-- three target via the threat-list pool (GetAITargets, alive,
-- any-unit: pets/totems eligible per Random, 0 defaults).
-- Healing-potion latch (common-AI UpdateAI arm: !UsedPotion &&
-- HealthBelowPct(25) -> DoCast(me, SPELL_HEALING_POTION 15503),
-- UsedPotion = true): via OnDamageTaken(9), per-guid one-shot,
-- post-damage health check (gyth convention), reset on
-- OnEnterCombat(1) (C++ Initialize runs in the constructor and
-- Reset — per-engagement reset is the gyth/vaelastrasz convention).
-- postDamage idiom 2026-10-21: projected (health - damage) clamped at
-- 0 plus a postDamage > 0 killing-blow guard (nefarian rule), since
-- the C++ UpdateAI HealthBelowPct(25) arm never fires on the killing
-- blow.
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
-- - JustEngagedWith's DoCast(me, SPELL_SUMMON_IMP 44163) — the summon
--   rides the spell effect (SPELL_EFFECT_SUMMON, effect 28); the Go
--   spell dispatcher has no dispatch case for effect 28 (spells.go
--   only gate-checks it for players), so the imp leg is unmodeled —
--   spell-effect summon dispatch is the standing blocker (stalesummon
--   precedent).
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
local SPELL_SEED_OF_CORRUPTION = 44141
local SPELL_CURSE_OF_AGONY = 14875
local SPELL_FEAR = 38595

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

-- C++ SelectTarget(Random, 0) defaults (boss_priestess_delrissa.cpp:654/:662/:670): threat-list any-unit, no
-- dist cap (UnitAI.h:170); alive filter per strip convention (dead picks
-- are no-op casts in both engines). The old map-player pool could pick
-- players who never engaged, plus dead players.
local function threatTargets(creature)
    local found = {}
    for _, unit in ipairs(creature:GetAITargets()) do
        if not unit:IsDead() then
            found[#found + 1] = unit
        end
    end
    return found
end

local function onSeedOfCorruption(creature, guid)
    local candidates = threatTargets(creature)
    if #candidates > 0 then
        creature:CastSpell(candidates[math.random(#candidates)], SPELL_SEED_OF_CORRUPTION)
    end
    schedule(guid, "seedofcorruption", 10000, function() onSeedOfCorruption(creature, guid) end)
end

local function onCurseOfAgony(creature, guid)
    local candidates = threatTargets(creature)
    if #candidates > 0 then
        creature:CastSpell(candidates[math.random(#candidates)], SPELL_CURSE_OF_AGONY)
    end
    schedule(guid, "curseofagony", 13000, function() onCurseOfAgony(creature, guid) end)
end

local function onFear(creature, guid)
    local candidates = threatTargets(creature)
    if #candidates > 0 then
        creature:CastSpell(candidates[math.random(#candidates)], SPELL_FEAR)
    end
    schedule(guid, "fear", 10000, function() onFear(creature, guid) end)
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
    schedule(guid, "seedofcorruption", 2000, function() onSeedOfCorruption(creature, guid) end)
    schedule(guid, "curseofagony", 1000, function() onCurseOfAgony(creature, guid) end)
    schedule(guid, "fear", 10000, function() onFear(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

local function onDamageTaken(_, creature, _, damage)
    local guid = creature:GetGUID()
    if usedPotion[guid] then
        return
    end
    local health, maxHealth = creature:GetHealth(), creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    -- nefarian rule: the C++ common-AI UpdateAI HealthBelowPct(25) arm
    -- reads post-damage health every tick and never fires on the
    -- killing blow, so the event-9 port projects post-damage health
    -- (clamped) and guards the killing blow.
    local postDamage = math.max(health - damage, 0)
    if postDamage > 0 and postDamage < math.floor(maxHealth * 25 / 100) then
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
