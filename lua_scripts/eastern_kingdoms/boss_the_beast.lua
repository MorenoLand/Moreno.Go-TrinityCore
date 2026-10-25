-- The Beast (Blackrock Spire; arena encounter) --
-- Lua port of src/server/scripts/EasternKingdoms/BlackrockMountain/
-- BlackrockSpire/boss_the_beast.cpp
-- (boss_the_beast — BossAI combat scheduler). Boss-script unit per
-- eastern_kingdoms_script_loader.cpp order (boss_shadow_hunter_voshgajin
-- done, registered for 9236; AddSC_boss_thebeast next in the
-- Blackrock Spire block).
-- Entry (verifiable from the C++ sources): blackrock_spire.h names
-- NPC_THE_BEAST = 10430 in the BRS creatures enum. The
-- creature_template ScriptName binding is DB-side (no TDB in this
-- workspace), but the entry itself is C++-verifiable so the port is
-- registered (boss_shadow_hunter_voshgajin / boss_overlord_wyrmthalak
-- precedent). GetBlackrockSpireAI is a GetInstanceAI retrieval wrapper
-- only (blackrock_spire.h:129-132); the AI has no instance arms —
-- Reset() _Reset() / JustDied _JustDied() are internal machinery
-- covered here by the cancel/re-arm on combat events (halycon
-- precedent).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Driven by CreateLuaEvent timers; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++ JustEngagedWith event schedule + UpdateAI event
-- machine):
-- EVENT_FLAME_BREAK: victim-cast FLAMEBREAK 16785, 12s init -> 10s
-- loop (maiden convention — DoCastVictim path, creature:GetVictim()).
-- EVENT_TERRIFYING_ROAR: victim-cast TERRIFYINGROAR 14100, 23s init
-- -> 20s loop (maiden convention).
-- EVENT_FIREBALL: victim-cast FIREBALL 16788, urand(8s, 21s) init ->
-- 8s..21s loop (C++ ScheduleEvent(8s, 21s) / Repeat(8s, 21s), C++-exact
-- — twilight_corrupter precedent).
-- EVENT_FIREBLAST: victim-cast FIREBLAST 16144, urand(5s, 8s) init ->
-- 5s..8s loop (C++ ScheduleEvent(5s, 8s) / Repeat(5s, 8s), C++-exact).
-- EVENT_IMMOLATE: random-player IMMOLATE 15570 — SelectTarget(Random,
-- 0, 100, true) is playerOnly, ported via the threat-list pool
-- (GetAITargets, alive, within 100yd); 3s init -> 8s
-- loop, re-arm unconditional (C++ Repeat sits outside the target
-- check), non-triggered DoCast(target).
-- EVENT_BERSERKER_CHARGE: random-player BERSERKER_CHARGE 16636 —
-- SelectTarget(Random, 0, 38, true) is playerOnly (threat-list pool,
-- within 38yd); 2s init -> urand(15s, 23s) loop, re-arm unconditional,
-- non-triggered DoCast(target).
-- SetData(DATA_BEAST_ROOM / DATA_BEAST_REACHED) — gates on
-- FindNearbyOrcs (GetCreatureListWithEntryInGrid NPC_BLACKHAND_ELITE
-- 10317), ObjectAccessor::GetCreature, MotionMaster::MovePath /
-- MovePoint, BasicEvent scheduling (OrcDeathEvent suicide 6s),
-- SetReactState, and both are only reachable via the area triggers
-- (at_trigger_the_beast_movement AT 2066, at_the_beast_room) and the
-- instance script; no area-trigger / instance / MotionMaster bridges
-- on the Lua surface (coren_direbrew / instance precedent).
-- Documented-only. SpellHit skinning arm — CreatureEventOnHitBySpell(14)
-- is bridged on the Lua surface, but the C++ arm gates on
-- spellInfo effect SPELL_EFFECT_SKINNING, which the bridge does not
-- expose (handler receives (event, creature, caster, spellId) only);
-- hard-coding spell IDs would be fabrication. Gates on !IsAlive
-- anyway. Documented-only.
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (creature
-- casts are packet-visual), so timers fire unconditionally and both
-- in-loop casting gates are dropped (maiden precedent). The C++
-- scheduler is driven from JustEngagedWith and drained by UpdateAI;
-- the port schedules from OnEnterCombat(1) and cancels on 2/4/23
-- (maiden convention).

local SPELL_FLAMEBREAK = 16785
local SPELL_TERRIFYINGROAR = 14100
local SPELL_FIREBALL = 16788
local SPELL_FIREBLAST = 16144
local SPELL_IMMOLATE = 15570
local SPELL_BERSERKER_CHARGE = 16636

local ENTRY_THE_BEAST = 10430

local timers = {}

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

local function onFlameBreak(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FLAMEBREAK)
    end
    schedule(guid, "flamebreak", 10000, function() onFlameBreak(creature, guid) end)
end

local function onTerrifyingRoar(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_TERRIFYINGROAR)
    end
    schedule(guid, "terrifyingroar", 20000, function() onTerrifyingRoar(creature, guid) end)
end

local function onFireball(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FIREBALL)
    end
    schedule(guid, "fireball", 8000 + math.random(0, 13000), function() onFireball(creature, guid) end)
end

local function onFireblast(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FIREBLAST)
    end
    schedule(guid, "fireblast", 5000 + math.random(0, 3000), function() onFireblast(creature, guid) end)
end

-- C++ SelectTarget(Random, 0, 100/38, true) (boss_the_beast.cpp:178/:187):
-- threat-list players alive within range (UnitAI.h:162-176).
local function randomThreatPlayerInRange(creature, range)
    local candidates = {}
    for _, plr in ipairs(creature:GetAITargets()) do
        if plr:GetObjectType() == "Player" and not plr:IsDead()
                and creature:IsWithinDist(plr, range) then
            candidates[#candidates + 1] = plr
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EVENT_IMMOLATE: SelectTarget(Random, 0, 100, true) playerOnly,
-- DoCast(target, 15570) non-triggered, 3s init, Repeat(8s) outside the
-- target check (re-arm unconditional).
local function onImmolate(creature, guid)
    local target = randomThreatPlayerInRange(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_IMMOLATE)
    end
    schedule(guid, "immolate", 8000, function() onImmolate(creature, guid) end)
end

-- C++ EVENT_BERSERKER_CHARGE: SelectTarget(Random, 0, 38, true)
-- playerOnly, DoCast(target, 16636) non-triggered, 2s init,
-- Repeat(15s, 23s) outside the target check (re-arm unconditional).
local function onBerserkerCharge(creature, guid)
    local target = randomThreatPlayerInRange(creature, 38)
    if target then
        creature:CastSpell(target, SPELL_BERSERKER_CHARGE)
    end
    schedule(guid, "berserkercharge", 15000 + math.random(0, 8000), function() onBerserkerCharge(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "flamebreak", 12000, function() onFlameBreak(creature, guid) end)
    schedule(guid, "terrifyingroar", 23000, function() onTerrifyingRoar(creature, guid) end)
    schedule(guid, "fireball", 8000 + math.random(0, 13000), function() onFireball(creature, guid) end)
    schedule(guid, "fireblast", 5000 + math.random(0, 3000), function() onFireblast(creature, guid) end)
    schedule(guid, "immolate", 3000, function() onImmolate(creature, guid) end)
    schedule(guid, "berserkercharge", 2000, function() onBerserkerCharge(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_THE_BEAST, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_THE_BEAST, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_THE_BEAST, 4, onCombatEnd)
RegisterCreatureEvent(ENTRY_THE_BEAST, 23, onCombatEnd)
