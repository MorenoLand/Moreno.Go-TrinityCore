-- Overlord Wyrmthalak (Blackrock Spire; Hall of Blackhand) --
-- Lua port of src/server/scripts/EasternKingdoms/BlackrockMountain/
-- BlackrockSpire/boss_overlord_wyrmthalak.cpp (boss_overlordwyrmthalakAI
-- — BossAI combat scheduler). Boss-script unit per
-- eastern_kingdoms_script_loader.cpp order (boss_mother_smolderweb done,
-- registered for 10596; AddSC_boss_overlordwyrmthalak next in the
-- Blackrock Spire block).
-- Entry (verifiable from the C++ sources): blackrock_spire.h names
-- NPC_OVERLORD_WYRMTHALAK = 9568 in the BRS creatures enum. The
-- creature_template ScriptName binding is DB-side (no TDB in this
-- workspace), but the entry itself is C++-verifiable so the port is
-- registered (boss_mother_smolderweb / boss_highlord_omokk precedent).
-- GetBlackrockSpireAI is a GetInstanceAI retrieval wrapper only
-- (blackrock_spire.h:129-132); the AI has no instance arms — Reset()
-- _Reset() is internal machinery covered here by the cancel/re-arm on
-- combat events (halycon precedent). JustDied _JustDied() likewise
-- covered by the cancel on OnDied(4).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 9 OnDamageTaken (51% summon latch), 23 OnReset. Driven by CreateLuaEvent timers; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++ JustEngagedWith event schedule + UpdateAI event
-- machine + the HealthBelowPct(51) one-shot summon latch):
-- EVENT_BLAST_WAVE: victim-cast BLASTWAVE 11130, 20s init -> 20s loop
-- (maiden convention — DoCastVictim path, creature:GetVictim()).
-- EVENT_SHOUT: victim-cast SHOUT 23511, 2s init -> 10s loop
-- (maiden convention).
-- EVENT_CLEAVE: victim-cast CLEAVE 20691, 6s init -> 7s loop
-- (maiden convention).
-- EVENT_KNOCK_AWAY: victim-cast KNOCKAWAY 20686, 12s init -> 14s loop
-- (maiden convention).
-- HealthBelowPct(51) latch (boss_overlord_wyrmthalak.cpp:97): modeled on
-- OnDamageTaken(9) in nefarian form — post-damage health strictly below
-- the floored 51% threshold and not yet latched -> random alive player
-- within 100 yd (SelectTarget Random/0/100/playerOnly, anastari+doan
-- precedent) -> SummonCreature(SPIRESTONE_WARLORD 9216, SummonLocation1,
-- TEMPSUMMON_TIMED_DESPAWN, 5min) + SummonCreature(SMOLDERTHORN_BERSERKER
-- 9268, SummonLocation2, ...) (aran SummonCreature precedent), latch set
-- only when a target was found (C++-exact: Summoned = true sits inside
-- the target-found branch). The warlord/berserker AI()->AttackStart(target)
-- legs have no bridge on the Lua surface — the adds materialize but idle
-- until the engine aggros them (aran water-elemental precedent). The stale
-- "no SummonCreature bridge" claim is retracted (SummonCreature is bridged);
-- "no AttackStart bridge" claim is retracted 2026-10-24 (EngageWithTarget
-- twin, player-target-only) and the add engage legs are ported below.
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (creature
-- casts are packet-visual), so timers fire unconditionally and both
-- in-loop casting gates are dropped (maiden precedent). The C++
-- scheduler is driven from JustEngagedWith and drained by UpdateAI;
-- the port schedules from OnEnterCombat(1) and cancels on 2/4/23
-- (maiden convention).

local SPELL_BLASTWAVE = 11130
local SPELL_SHOUT = 23511
local SPELL_CLEAVE = 20691
local SPELL_KNOCKAWAY = 20686

-- HealthBelowPct(51) one-shot summon arm (boss_overlord_wyrmthalak.cpp:97):
local NPC_SPIRESTONE_WARLORD = 9216
local NPC_SMOLDERTHORN_BERSERKER = 9268
local SUMMON1_X, SUMMON1_Y, SUMMON1_Z, SUMMON1_O = -39.355, -513.456, 88.472, 4.679
local SUMMON2_X, SUMMON2_Y, SUMMON2_Z, SUMMON2_O = -49.875, -511.896, 88.195, 4.613
local TEMPSUMMON_TIMED_DESPAWN = 2

local ENTRY_OVERLORD_WYRMTHALAK = 9568

local timers = {}
local summoned = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
    summoned[guid] = nil
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

local function onBlastWave(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_BLASTWAVE)
    end
    schedule(guid, "blastwave", 20000, function() onBlastWave(creature, guid) end)
end

local function onShout(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SHOUT)
    end
    schedule(guid, "shout", 10000, function() onShout(creature, guid) end)
end

local function onCleave(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CLEAVE)
    end
    schedule(guid, "cleave", 7000, function() onCleave(creature, guid) end)
end

local function onKnockAway(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_KNOCKAWAY)
    end
    schedule(guid, "knockaway", 14000, function() onKnockAway(creature, guid) end)
end

-- C++ SelectTarget(SelectTargetMethod::Random, 0, 100, true): random
-- alive player within 100 yd, position 0 (victim included) — the
-- GetPlayersInWorld + mapId/instanceId filter (anastari precedent) with
-- a distance gate (doan precedent).
local function randomAlivePlayer(creature, maxDist)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local players = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() and creature:GetDistance(p) <= maxDist then
            players[#players + 1] = p
        end
    end
    if #players == 0 then
        return nil
    end
    return players[math.random(#players)]
end

-- C++ UpdateAI HealthBelowPct(51) one-shot (boss_overlord_wyrmthalak.cpp:
-- 97-107): !Summoned and post-damage health strictly below 51% ->
-- random alive player within 100 yd, SummonCreature warlord 9216 at
-- SummonLocation1 + berserker 9268 at SummonLocation2
-- (TEMPSUMMON_TIMED_DESPAWN, 5min), Summoned = true. The
-- warlord->AI()->AttackStart(target) / berserker->AI()->AttackStart(target)
-- legs are PORTED below: the SummonCreature return is the add object
-- (aran elemental precedent) and EngageWithTarget is the player-target
-- AttackStart twin, C++ order (warlord engage, then berserker engage).
-- The latch sets only when a target was found, matching C++ (Summoned = true
-- sits inside the target-found branch, so a target-less tick retries).
-- C++ reads post-damage health in UpdateAI, so the arm never fires on the
-- killing blow (nefarian postDamage > 0 rule).
local function wyrmthalakDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    if summoned[guid] then
        return false, damage
    end
    local health, maxHealth = creature:GetHealth(), creature:GetMaxHealth()
    if maxHealth == 0 then
        return false, damage
    end
    local postDamage = math.max(health - damage, 0)
    if postDamage > 0 and postDamage < math.floor(maxHealth * 51 / 100) then
        local target = randomAlivePlayer(creature, 100)
        if target then
            local warlord = creature:SummonCreature(NPC_SPIRESTONE_WARLORD,
                SUMMON1_X, SUMMON1_Y, SUMMON1_Z, SUMMON1_O,
                TEMPSUMMON_TIMED_DESPAWN, 300000)
            if warlord then
                warlord:EngageWithTarget(target)
            end
            local berserker = creature:SummonCreature(NPC_SMOLDERTHORN_BERSERKER,
                SUMMON2_X, SUMMON2_Y, SUMMON2_Z, SUMMON2_O,
                TEMPSUMMON_TIMED_DESPAWN, 300000)
            if berserker then
                berserker:EngageWithTarget(target)
            end
            summoned[guid] = true
        end
    end
    return false, damage
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    summoned[guid] = false
    schedule(guid, "blastwave", 20000, function() onBlastWave(creature, guid) end)
    schedule(guid, "shout", 2000, function() onShout(creature, guid) end)
    schedule(guid, "cleave", 6000, function() onCleave(creature, guid) end)
    schedule(guid, "knockaway", 12000, function() onKnockAway(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_OVERLORD_WYRMTHALAK, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_OVERLORD_WYRMTHALAK, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_OVERLORD_WYRMTHALAK, 4, onCombatEnd)
RegisterCreatureEvent(ENTRY_OVERLORD_WYRMTHALAK, 9, wyrmthalakDamageTaken)
RegisterCreatureEvent(ENTRY_OVERLORD_WYRMTHALAK, 23, onCombatEnd)
