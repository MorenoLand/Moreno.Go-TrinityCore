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
-- OnTargetDied, 4 OnDied, 23 OnReset. Driven by CreateLuaEvent
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
--   with SelectTarget(Random, 1, 30.0f, true). Random-target legs ride
--   the maiden_of_virtue/moroes playersInInstance + IsWithinDist
--   local pattern; the C++ position-1 (skip-top-threat) nuance is not
--   modeled — a uniform random in-range alive player is picked.
-- - EVENT_TELEPORT: 15s init -> 20s re-arm; self-cast one of the 3
--   teleport spells with the C++ no-repeat-twice-in-a-row rule: the
--   C++ arm swaps teleportSpells[0] with teleportSpells[urand(1, 2)],
--   which guarantees the newly cast spell differs from the last one
--   cast; the Lua per-guid latch picks uniformly from the two spells
--   that were not cast last, which is the same distribution.
-- - EVENT_VOID_BOLT: 1s init -> 5s re-arm; DoCastVictim(7588) ->
--   GetVictim + CastSpell (mr_smite convention).
-- - KilledUnit: OnTargetDied(3) with victim:IsPlayer() guard ->
--   Talk(SAY_SLAY) (vancleef/illidan convention).
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - EVENT_THUNDERSHOCK: DoCastAOE(7803) has no bridge (shazzrah /
--   supremus precedent) — skipped.
-- - SpellHitTarget: Talk(SAY_TRANSFORM) fires when SPELL_ARUGAL_CURSE
--   (7621) lands, but event 15 (OnSpellHitTarget) is never fired by
--   the Go engine (standing SpellHit/Summon blocker).
-- - AttackStart -> AttackStartCaster(who, 100.0f): no attack-start
--   bridge on the Lua surface.
-- - The BossAI(creature, BOSS_ARUGAL) constructor and
--   GetShadowfangKeepAI -> GetInstanceAI legs (instance-script model
--   blocked, standing).

local ENTRY_ARUGAL = 4275

local SAY_AGGRO = 1
local SAY_SLAY = 3

local SPELL_ARUGAL_CURSE = 7621
local SPELL_VOIDBOLT = 7588
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
    per[key] = CreateLuaEvent(fn, delay)
end

local function playersInInstance(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            found[#found + 1] = p
        end
    end
    return found
end

local function randomTargetInRange(creature, range)
    local candidates = {}
    for _, p in ipairs(playersInInstance(creature)) do
        if creature:IsWithinDist(p, range) then
            candidates[#candidates + 1] = p
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

local function onTeleport(creature, guid)
    local last = lastTeleport[guid]
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

local function onTargetDied(_, creature, victim)
    if victim:IsPlayer() then
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
end

local function onCombatEnd(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    lastTeleport[guid] = nil
end

local function onReset(_, creature)
    onCombatEnd(nil, creature)
end

RegisterCreatureEvent(ENTRY_ARUGAL, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_ARUGAL, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_ARUGAL, 3, onTargetDied)
RegisterCreatureEvent(ENTRY_ARUGAL, 4, onCombatEnd)
RegisterCreatureEvent(ENTRY_ARUGAL, 23, onReset)
