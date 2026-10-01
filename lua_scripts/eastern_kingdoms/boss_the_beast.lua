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
-- EVENT_FIREBALL: victim-cast FIREBALL 16788, 8s init -> 8s..21s loop
-- (C++ urand(8s, 21s), C++-exact — twilight_corrupter precedent).
-- EVENT_FIREBLAST: victim-cast FIREBLAST 16144, 5s init -> 5s..8s loop
-- (C++ urand(5s, 8s), C++-exact — twilight_corrupter precedent).
-- Unmodeled: EVENT_IMMOLATE — gates on
-- SelectTarget(SelectTargetMethod::Random, 0, 100, true) then
-- DoCast(target, SPELL_IMMOLATE=15570); no SelectTarget bridge on the
-- Lua surface (doomwalker / kazzak / doomrel precedent). EVENT_BERSERKER_
-- CHARGE — gates on SelectTarget(Random, 0, 38, true) then
-- DoCast(target, SPELL_BERSERKER_CHARGE=16636); same bridge gap. Both
-- are random-target mid-fight casts, not the combat trigger, so porting
-- the four victim-cast timers without them does not strand the fight.
-- SetData(DATA_BEAST_ROOM / DATA_BEAST_REACHED) — gates on
-- FindNearbyOrcs (GetCreatureListWithEntryInGrid NPC_BLACKHAND_ELITE
-- 10317), ObjectAccessor::GetCreature, MotionMaster::MovePath /
-- MovePoint, BasicEvent scheduling (OrcDeathEvent suicide 6s),
-- SetReactState, and both are only reachable via the area triggers
-- (at_trigger_the_beast_movement AT 2066, at_the_beast_room) and the
-- instance script; no area-trigger / instance / MotionMaster bridges
-- on the Lua surface (coren_direbrew / instance precedent).
-- Documented-only. SpellHit skinning arm — no SpellHit bridge on the
-- Lua surface (draenei_survivor precedent); gates on !IsAlive
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

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "flamebreak", 12000, function() onFlameBreak(creature, guid) end)
    schedule(guid, "terrifyingroar", 23000, function() onTerrifyingRoar(creature, guid) end)
    schedule(guid, "fireball", 8000, function() onFireball(creature, guid) end)
    schedule(guid, "fireblast", 5000, function() onFireblast(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_THE_BEAST, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_THE_BEAST, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_THE_BEAST, 4, onCombatEnd)
RegisterCreatureEvent(ENTRY_THE_BEAST, 23, onCombatEnd)
