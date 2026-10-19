-- Skarvald the Constructor and Dalronn the Controller (Utgarde Keep) —
-- Lua port of
-- src/server/scripts/Northrend/UtgardeKeep/UtgardeKeep/boss_skarvald_dalronn.cpp
-- (302 lines; 2 scripts — boss_skarvald_the_constructor /
-- boss_dalronn_the_controller (CreatureScript via GetUtgardeKeepAI
-- (BossAI), DATA_SKARVALD_DALRONN = 1; generic_boss_controllerAI shared
-- base, OtherBossData DATA_DALRONN = 4 / DATA_SKARVALD = 3); all
-- registered from inside AddSC_boss_skarvald_dalronn(); loader decl 127
-- / call 322 per northrend_script_loader.cpp — the SECOND group of the
-- "// Utgarde Keep - Utgarde Keep" block in AddNorthrendScripts(),
-- immediately after AddSC_boss_keleseth() (decl 126 / call 321); the
-- checkpoint sequence (boss_keleseth -> boss_skarvald_dalronn) is
-- followed).
-- Entries: 24200 Skarvald the Constructor / 24201 Dalronn the
-- Controller (utgarde_keep.h NPC_SKARVALD / NPC_DALRONN, lines 47-48 —
-- entry-verifiable, registration proceeds (the nexus_commanders /
-- malygos / sartharion kalecgos precedent); the CreatureScript
-- ScriptName binding is DB-side as usual). DATA_SKARVALD = 3 /
-- DATA_DALRONN = 4 (utgarde_keep.h lines 36-37). Ghosts 27390
-- (NPC_SKARVALD_GHOST) / 27389 (NPC_DALRONN_GHOST, lines 52-53) share
-- the script names DB-side but carry no bridgeable arms — no
-- registrations; the !IsInGhostForm gates below are satisfied
-- structurally (only non-ghost entries run this lua).
-- Sole-source verified: whole-server-tree grep for both script names
-- hits boss_skarvald_dalronn.cpp only (+ the loader decl/call lines for
-- AddSC_boss_skarvald_dalronn); this clone carries an (empty) sql/ tree
-- with zero skarvald hits, so ScriptName bindings are DB-side by
-- construction. The lua was ported Oct 2 10:00 in 980a824 and re-ported
-- 2026-10-13 on the CreateLuaEvent timer convention (this run).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 9 OnDamageTaken, 23 OnReset.
-- Timer bridge: CreateLuaEvent harness, GUID-keyed timers table
-- (single table; GUIDs unique across both bosses — the yogg_saron
-- multi-NPC precedent), cancel on events 2/4/23 (the maiden_of_virtue /
-- moroes / curator / nightbane precedent). CastSpell(nil, spell) =
-- DoCastVictim; CastSpell(creature, spell) = DoCastSelf; the 3rd
-- CastSpell arg is the C++ DoCast 3rd arg (2-arg DoCast overloads =
-- non-triggered). {min,max} table delay = randtime range. Random-target
-- arms via GetPlayersInWorld filtered by map/instance/alive/distance
-- (the baltharus EVENT_ENERVATING_BRAND precedent). The no-timer-bridge
-- doctrine from the Oct-2 header is STALE — CreateLuaEvent is a real
-- registered bridge (engine/scripting/runtime.go:798) and the
-- engage-schedule legs are ported below.
-- Ported arms (all C++-exact for the modeled arms):
-- Skarvald JustEngagedWith (lines 192-201): Talk(SAY_AGGRO 0) (event
-- 1; !IsInGhostForm gate structural — ghost entry 27390 not
-- registered); EVENT_SKARVALD_CHARGE init 5s (line 197) and
-- EVENT_STONE_STRIKE init 10s (line 198) become CreateLuaEvent timers.
-- EVENT_SKARVALD_CHARGE (arm lines 205-209): DoCast(target,
-- SPELL_CHARGE 43651), 2-arg = non-triggered, on SelectTarget(Random,
-- 0, SkarvaldChargePredicate) (random unit 5-30yd). FIRES ONCE in C++:
-- the re-arm (line 208) schedules EVENT_CHARGE — not
-- EVENT_SKARVALD_CHARGE. EVENT_CHARGE is the global Movement EventId =
-- 1003 (SharedDefines.h line 3342), which never matches the
-- ExecuteEvent switch case (EVENT_SKARVALD_CHARGE = 1), so the 5-10s
-- re-arm never fires the arm again — ported C++-exact as a 5s one-shot
-- with no re-arm. Target deviation: C++ selects any unit (not
-- playerOnly); the lua selects a random player 5-30yd (baltharus
-- precedent) — the closest available bridge.
-- EVENT_STONE_STRIKE (arm lines 210-213): DoCastVictim(SPELL_STONE_STRIKE
-- 48583), non-triggered -> CastSpell(nil, 48583); init 10s, repeat
-- {5s,10s} (line 213).
-- Skarvald DamageTaken (lines 218-227): Enrage latch + DoCast(me,
-- SPELL_ENRAGE 48193) (2-arg = non-triggered -> CastSpell(creature,
-- 48193)) when !Enraged && !IsInGhostForm (structural) &&
-- HealthBelowPctDamaged(15, damage) — floored integer threshold
-- < 15 via event 9 (the algalon threshold+latch precedent); latch
-- cleared on event 23 (C++ Reset() clears Enraged, line 188); damage
-- is NOT rewritten.
-- KilledUnit (both bosses; lines 165-169): Talk(SAY_KILL 3)
-- player-gated (event 3; the razuvious player-gated variant precedent;
-- !IsInGhostForm gate structural).
-- Dalronn JustEngagedWith (lines 244-254): the schedule legs become
-- CreateLuaEvent timers — EVENT_SHADOW_BOLT init 1s (line 248),
-- EVENT_DEBILITATE init 5s (line 249), EVENT_DELAYED_AGGRO_SAY init 5s
-- (line 251-252, !IsInGhostForm structural); event 1 deliberately does
-- NOT Talk — Dalronn's SAY_AGGRO rides the 5s delayed one-shot (C++
-- overlap avoidance), so an immediate yell would deviate. The
-- EVENT_SUMMON_SKELETONS heroic leg (lines 253-254) is blocked — see
-- DOCUMENTED-ONLY.
-- EVENT_SHADOW_BOLT (arm lines 262-266): DoCast(target,
-- SPELL_SHADOW_BOLT 43649), non-triggered, on SelectTarget(Random, 0,
-- 45.0f, true) (random PLAYER within 45yd) -> CastSpell(target, 43649);
-- repeat 2100ms (line 265, "give a 100ms pause to try cast other
-- spells").
-- EVENT_DEBILITATE (arm lines 267-271): DoCast(target,
-- SPELL_DEBILITATE 43650), non-triggered, on SelectTarget(Random, 0,
-- 50.0f, true) (random player within 50yd) -> CastSpell(target,
-- 43650); repeat {5s,10s} (line 270).
-- EVENT_DELAYED_AGGRO_SAY (arm lines 276-278): Talk(SAY_AGGRO 0)
-- one-shot 5s.
-- DOCUMENTED-ONLY (in this header; no registrations beyond entries
-- 24200 / 24201):
-- generic_boss_controllerAI::JustDied (lines 124-142) — when the other
-- boss is alive: Talk(SAY_DIED_FIRST 2) + RemoveFlag(UNIT_DYNFLAG_LOOTABLE)
-- + cross-creature DoAction(ACTION_OTHER_JUST_DIED) + TRIGGERED
-- DoCast(me, SPELL_SUMMON_SKARVALD_GHOST 48613 / SPELL_SUMMON_DALRONN_GHOST
-- 48612, true) (line 134); when both are dead: Talk(SAY_DEATH 1) +
-- cross-creature DoAction(ACTION_DESPAWN_SUMMONS) + _JustDied().
-- BLOCKED: the whole arm rides instance->GetGuidData(OtherBossData)
-- (no instance-GuidData bridge) and cross-creature DoAction
-- (no DoAction bridge); the triggered ghost summon is a summon leg.
-- Event 4 deliberately NOT registered (neither Talk is unconditional).
-- DoAction(ACTION_OTHER_JUST_DIED) (lines 144-157) — schedules
-- EVENT_DEATH_RESPONSE (2s) -> Talk(SAY_DEATH_RESPONSE 4) (lines
-- 159-163); DoAction-gated, no bridge. ACTION_DESPAWN_SUMMONS
-- (summons.DespawnAll()) — no bridge.
-- Dalronn EVENT_SUMMON_SKELETONS (lines 272-275): DoCast(me,
-- SPELL_SUMMON_SKELETONS 52611) self-cast summon, heroic-gated init
-- 10s, repeat {10s,30s} — summon leg, BLOCKED per doctrine.
-- Ghost-form Reset DoZoneInCombat (lines 108-116), BossAI
-- JustEngagedWith/_Reset/_JustDied passthroughs — no bridges.

local ENTRY_SKARVALD = 24200
local ENTRY_DALRONN = 24201

local SAY_AGGRO = 0
local SAY_KILL = 3

local SPELL_CHARGE = 43651
local SPELL_STONE_STRIKE = 48583
local SPELL_ENRAGE = 48193
local SPELL_SHADOW_BOLT = 43649
local SPELL_DEBILITATE = 43650

local timers = {}
local enraged = {}

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

-- Random living players in the same map/instance within [minDist,
-- maxDist] of the creature (the baltharus precedent).
local function randomPlayer(creature, minDist, maxDist)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local candidates = {}
    for _, plr in ipairs(GetPlayersInWorld()) do
        if plr:GetMapId() == mapId and plr:GetInstanceId() == instanceId
                and not plr:IsDead()
                and creature:GetDistance(plr) >= minDist
                and creature:GetDistance(plr) <= maxDist then
            candidates[#candidates + 1] = plr
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ SkarvaldChargePredicate (boss_skarvald_dalronn.cpp:80-91):
-- threat-list pick with GetDistance2d in [5, 30] — NOT player-only
-- (the template SelectTarget overload has no player gate). The
-- midnight randomTargetInBand precedent over the bridged,
-- threat-ordered GetAITargets (lua_creature_events.go:345); the C++
-- bounds are inclusive on both ends, kept here (midnight's upper
-- bound was exclusive — this one is C++-exact). C++ applies no dead
-- filter beyond threat-list offline-ness, so none is added.
local function randomTargetInBand(creature, minDist, maxDist)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        local dist = creature:GetDistance2d(t)
        if dist >= minDist and dist <= maxDist then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EVENT_STONE_STRIKE (lines 210-213): DoCastVictim(SPELL_STONE_STRIKE),
-- 2-arg = non-triggered; init 10s (line 198), repeat {5s,10s} (line 213).
local function stoneStrikeTick(creature, guid)
    creature:CastSpell(nil, SPELL_STONE_STRIKE)
    schedule(guid, "stonestrike", {5000, 10000}, function() stoneStrikeTick(creature, guid) end)
end

-- C++ EVENT_SKARVALD_CHARGE (lines 205-209): DoCast(target, SPELL_CHARGE),
-- 2-arg = non-triggered; SelectTarget(Random, 0,
-- SkarvaldChargePredicate(me)) (:206) — threat-list pick, GetDistance2d
-- in [5, 30], NOT player-only (template overload has no player gate);
-- one-shot 5s (line 197) — the line-208 re-arm targets the global
-- EVENT_CHARGE (1003), never the arm, so it does not re-fire in C++
-- either.
local function chargeTick(creature, guid)
    local target = randomTargetInBand(creature, 5, 30)
    if target then
        creature:CastSpell(target, SPELL_CHARGE)
    end
end

-- C++ boss_skarvald_the_constructorAI::JustEngagedWith (lines 192-201):
-- BossAI passthrough (no bridge); if (!IsInGhostForm) Talk(SAY_AGGRO) —
-- gate satisfied structurally (ghost entry 27390 not registered).
local function skarvaldEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "charge", 5000, function() chargeTick(creature, guid) end)
    schedule(guid, "stonestrike", 10000, function() stoneStrikeTick(creature, guid) end)
end

local function skarvaldLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function skarvaldDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function skarvaldReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    enraged[guid] = nil
end

-- C++ generic_boss_controllerAI::KilledUnit (lines 165-169):
-- if (!IsInGhostForm && who->GetTypeId() == TYPEID_PLAYER) Talk(SAY_KILL)
-- — ghost gate structural — the razuvious player-gated variant precedent.
local function skarvaldKilledUnit(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

-- C++ boss_skarvald_the_constructorAI::DamageTaken (lines 218-227):
-- !Enraged && !IsInGhostForm (structural) && HealthBelowPctDamaged(15,
-- damage) -> Enraged = true; DoCast(me, SPELL_ENRAGE) (2-arg =
-- non-triggered) — the algalon threshold+latch precedent. Damage is
-- not rewritten.
local function skarvaldDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    if not enraged[guid] then
        local maxHealth = creature:GetMaxHealth()
        if maxHealth > 0 and (creature:GetHealth() - damage) < math.floor(maxHealth * 15 / 100) then
            enraged[guid] = true
            creature:CastSpell(creature, SPELL_ENRAGE)
        end
    end
end

-- C++ EVENT_SHADOW_BOLT (lines 262-266): DoCast(target, SPELL_SHADOW_BOLT),
-- 2-arg = non-triggered, random player within 45yd; init 1s (line 248),
-- repeat 2100ms (line 265).
local function shadowBoltTick(creature, guid)
    local target = randomPlayer(creature, 0, 45)
    if target then
        creature:CastSpell(target, SPELL_SHADOW_BOLT)
    end
    schedule(guid, "shadowbolt", 2100, function() shadowBoltTick(creature, guid) end)
end

-- C++ EVENT_DEBILITATE (lines 267-271): DoCast(target, SPELL_DEBILITATE),
-- 2-arg = non-triggered, random player within 50yd; init 5s (line 249),
-- repeat {5s,10s} (line 270).
local function debilitateTick(creature, guid)
    local target = randomPlayer(creature, 0, 50)
    if target then
        creature:CastSpell(target, SPELL_DEBILITATE)
    end
    schedule(guid, "debilitate", {5000, 10000}, function() debilitateTick(creature, guid) end)
end

-- C++ boss_dalronn_the_controllerAI::JustEngagedWith (lines 244-254):
-- BossAI passthrough (no bridge); the schedule legs become CreateLuaEvent
-- timers. No immediate Talk — SAY_AGGRO rides EVENT_DELAYED_AGGRO_SAY.
local function dalronnEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "shadowbolt", 1000, function() shadowBoltTick(creature, guid) end)
    schedule(guid, "debilitate", 5000, function() debilitateTick(creature, guid) end)
    -- C++ EVENT_DELAYED_AGGRO_SAY (lines 276-278): Talk(SAY_AGGRO), init 5s
    -- if !IsInGhostForm (lines 251-252; structural); one-shot.
    schedule(guid, "aggrosay", 5000, function() creature:Talk(SAY_AGGRO) end)
end

local function dalronnLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function dalronnDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function dalronnReset(event, creature)
    cancelTimers(creature:GetGUID())
end

local function dalronnKilledUnit(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

RegisterCreatureEvent(ENTRY_SKARVALD, 1, skarvaldEnterCombat)
RegisterCreatureEvent(ENTRY_SKARVALD, 2, skarvaldLeaveCombat)
RegisterCreatureEvent(ENTRY_SKARVALD, 3, skarvaldKilledUnit)
RegisterCreatureEvent(ENTRY_SKARVALD, 4, skarvaldDied)
RegisterCreatureEvent(ENTRY_SKARVALD, 9, skarvaldDamageTaken)
RegisterCreatureEvent(ENTRY_SKARVALD, 23, skarvaldReset)
RegisterCreatureEvent(ENTRY_DALRONN, 1, dalronnEnterCombat)
RegisterCreatureEvent(ENTRY_DALRONN, 2, dalronnLeaveCombat)
RegisterCreatureEvent(ENTRY_DALRONN, 3, dalronnKilledUnit)
RegisterCreatureEvent(ENTRY_DALRONN, 4, dalronnDied)
RegisterCreatureEvent(ENTRY_DALRONN, 23, dalronnReset)
