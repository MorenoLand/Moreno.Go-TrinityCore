-- Boss Archmage Arugal (Shadowfang Keep)
-- Lua port of src/server/scripts/EasternKingdoms/ShadowfangKeep/
-- shadowfang_keep.cpp
-- (boss_archmage_arugalAI — BossAI(creature, BOSS_ARUGAL) via
-- GetShadowfangKeepAI -> GetInstanceAI; registered by
-- AddSC_shadowfang_keep in eastern_kingdoms_script_loader.cpp
-- (declaration line 123, call line 301)). The Shadowfang Keep block
-- is OPEN.
-- Entry (verifiable from the C++ sources): NPC_ARCHMAGE_ARUGAL =
-- 4275 in the SKCreatures enum in ShadowfangKeep/shadowfang_keep.h
-- (kalecgos precedent). Whole-server-tree grep confirms
-- shadowfang_keep.cpp as the only source of "archmage_arugal" for
-- the script (loader lines only otherwise).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 15 OnSpellHitTarget, 23 OnReset. Driven by CreateLuaEvent
-- timers; melee is engine-driven in Go (creature combat tick), like
-- C++ DoMeleeAttackIfReady. The C++ UNIT_STATE_CASTING gate has no
-- bridge (maiden precedent), so timers fire unconditionally. The C++
-- events.Repeat arms re-arm from execution; the Lua one-shot timer
-- chains match that cadence.
-- Verifiable numbers (file's own enums): SPELL_ARUGAL_CURSE = 7621 /
-- SPELL_THUNDERSHOCK = 7803 / SPELL_VOIDBOLT = 7588 / SPELL_TELE_
-- SPAWN = 7586 / SPELL_TELE_UPPER = 7587 / SPELL_TELE_STAIRS = 7136 /
-- NUM_TELEPORT_SPELLS = 3; SAY_AGGRO = 1 / SAY_TRANSFORM = 2 /
-- SAY_SLAY = 3.
-- Ported arms (C++ JustEngagedWith + UpdateAI + KilledUnit):
-- - SAY_AGGRO: Talk(1) on OnEnterCombat.
-- - EVENT_CURSE: 7s init -> 15s re-arm; DoCast(random target, 7621)
--   with SelectTarget(Random, 1, 30.0f, true). The C++ offset-1 leg is
--   ported: the threat list's first (top-threat) entry is skipped
--   before filtering (UnitAI.h:162-170, 176-184), and a threat list
--   of size <= 1 yields no cast at all (the C++ shortcut) — the pick
--   is a uniform random over the remaining in-range alive players
--   from the bridged threat list (GetAITargets, gerstahn/kelris
--   precedent), not an instance-wide player pool.
-- - EVENT_TELEPORT: 15s init -> 20s re-arm; self-cast one of the 3
--   teleport spells with the C++ no-repeat-twice-in-a-row rule: the
--   C++ arm swaps teleportSpells[0] with teleportSpells[urand(1, 2)],
--   which guarantees the newly cast spell differs from the last one
--   cast; the Lua per-guid latch picks uniformly from the two spells
--   that were not cast last, which is the same distribution. The
--   initial array is [SPAWN, UPPER, STAIRS], so the FIRST cast can
--   never be SPAWN — the Lua latch starts at SPAWN for the same
--   reason, and the latch survives evade (the C++ array is a member,
--   not reset by Reset) but clears on death (new AI object).
-- - EVENT_VOID_BOLT: 1s init -> 5s re-arm; DoCastVictim(7588) ->
--   GetVictim + CastSpell (mr_smite convention).
-- - EVENT_THUNDERSHOCK: 10s init -> 30s re-arm; non-triggered
--   DoCastAOE(7803) lowers to self-cast CastSpell(creature, spell)
--   (selin_fireheart DoCastAOE convention; 7803 has no SpellScript
--   and the arm is not phase-gated).
-- - KilledUnit: OnTargetDied(3) with victim:GetObjectType() == "Player" guard ->
--   Talk(SAY_SLAY) (vancleef/illidan convention).
-- - SpellHitTarget: Talk(SAY_TRANSFORM 2) fires when SPELL_ARUGAL_CURSE
--   (7621) lands (event 15; the curse cast is ported in onCurse, so the
--   trigger is live in Go — the old "event 15 never fires" note was
--   stale; event 15 fires from the Go spell path, boss_ai.go:1904).
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - AttackStart -> AttackStartCaster(who, 100.0f): no attack-start
--   bridge on the Lua surface.
-- - The BossAI(creature, BOSS_ARUGAL) constructor and
--   GetShadowfangKeepAI -> GetInstanceAI legs (instance-script model
--   blocked, standing).

local ENTRY_ARUGAL = 4275

local SAY_AGGRO = 1
local SAY_TRANSFORM = 2
local SAY_SLAY = 3

local SPELL_ARUGAL_CURSE = 7621
local SPELL_VOIDBOLT = 7588
local SPELL_THUNDERSHOCK = 7803
local SPELL_TELE_SPAWN = 7586
local SPELL_TELE_UPPER = 7587
local SPELL_TELE_STAIRS = 7136

local teleportSpells = { SPELL_TELE_SPAWN, SPELL_TELE_UPPER, SPELL_TELE_STAIRS }

local timers = {}
local lastTeleport = {}

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

-- C++ SelectTarget(Random, 1, 30.0f, true) (shadowfang_keep.cpp):
-- the threat list's first entry (top threat, UnitAI.h:176-184) is
-- skipped BEFORE the playerOnly/range filter, and a list of size
-- <= 1 yields nil outright (the C++ shortcut); GetAITargets is the
-- bridged threat list in descending-threat order (threat.go:889),
-- so index 1 is the skipped entry. Distance via GetDistance2d
-- (gerstahn/kelris precedent; C++ IsWithinCombatRange is the same
-- band up to combat reach).
local function randomTargetInRange(creature, range)
    local targets = creature:GetAITargets()
    if #targets <= 1 then
        return nil
    end
    local candidates = {}
    for i = 2, #targets do
        local t = targets[i]
        if t:GetObjectType() == "Player" and t:IsAlive()
                and creature:GetDistance2d(t) <= range then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

local function onCurse(creature, guid)
    local target = randomTargetInRange(creature, 30)
    if target then
        creature:CastSpell(target, SPELL_ARUGAL_CURSE)
    end
    schedule(guid, "curse", 15000, function() onCurse(creature, guid) end)
end

-- C++ teleport arm: teleportSpells starts [SPAWN, UPPER, STAIRS] and
-- each cast swaps [0] with urand(1, 2), so the cast is uniform over
-- the two spells that are not the last cast, and the first cast is
-- never SPAWN. The Lua latch starts at SPAWN (nil -> SPAWN) for the
-- same first-cast distribution; it survives evade (C++ member array,
-- untouched by Reset) and clears only on death (new AI object).
local function onTeleport(creature, guid)
    local last = lastTeleport[guid] or SPELL_TELE_SPAWN
    local pool = {}
    for i, spell in ipairs(teleportSpells) do
        if spell ~= last then
            pool[#pool + 1] = spell
        end
    end
    local spell = pool[math.random(#pool)]
    lastTeleport[guid] = spell
    creature:CastSpell(creature, spell)
    schedule(guid, "teleport", 20000, function() onTeleport(creature, guid) end)
end

local function onVoidBolt(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_VOIDBOLT)
    end
    schedule(guid, "voidbolt", 5000, function() onVoidBolt(creature, guid) end)
end

-- C++ EVENT_THUNDERSHOCK: non-triggered DoCastAOE(7803), 10s init ->
-- 30s re-arm. DoCastAOE lowers to non-triggered self-cast
-- (selin_fireheart convention); 7803 carries no SpellScript and the
-- arm is not phase-gated, so the stale "no DoCastAOE bridge" header
-- claim was wrong.
local function onThunderShock(creature, guid)
    creature:CastSpell(creature, SPELL_THUNDERSHOCK)
    schedule(guid, "thundershock", 30000, function() onThunderShock(creature, guid) end)
end

local function onTargetDied(_, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "curse", 7000, function() onCurse(creature, guid) end)
    schedule(guid, "teleport", 15000, function() onTeleport(creature, guid) end)
    schedule(guid, "voidbolt", 1000, function() onVoidBolt(creature, guid) end)
    schedule(guid, "thundershock", 10000, function() onThunderShock(creature, guid) end)
end

-- Evade (2) and reset (23) cancel timers only: the C++ teleport
-- array is a member, so the last-cast latch survives evade. Death
-- (4) clears it: a respawned Arugal gets a fresh AI object, whose
-- array starts [SPAWN, UPPER, STAIRS] again.
local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

local function onDied(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    lastTeleport[guid] = nil
end

local function onReset(_, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ SpellHitTarget: SPELL_ARUGAL_CURSE (7621) lands on a target ->
-- Talk(SAY_TRANSFORM 2), ungated. Event 15
-- (CREATURE_EVENT_ON_SPELL_HIT_TARGET) fires on the caster's bindings
-- from the Go spell path (boss_ai.go:1904); the curse cast is ported
-- (onCurse), so the trigger is live.
local function onSpellHitTarget(event, creature, target, spellId)
    if spellId == SPELL_ARUGAL_CURSE then
        creature:Talk(SAY_TRANSFORM)
    end
end

RegisterCreatureEvent(ENTRY_ARUGAL, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_ARUGAL, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_ARUGAL, 3, onTargetDied)
RegisterCreatureEvent(ENTRY_ARUGAL, 4, onDied)
RegisterCreatureEvent(ENTRY_ARUGAL, 23, onReset)
RegisterCreatureEvent(ENTRY_ARUGAL, 15, onSpellHitTarget)
