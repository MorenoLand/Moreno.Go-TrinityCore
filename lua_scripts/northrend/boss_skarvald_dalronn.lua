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
-- arms via the bridged threat-ordered GetAITargets (dalronn
-- shadowbolt/debilitate player-only; skarvald charge any-unit). The no-timer-bridge
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
-- playerOnly); the lua selects a random threat-list unit 5-30yd
-- (randomTargetInBand) — C++-exact.
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
-- generic_boss_controllerAI::JustDied (lines 119-137) — when the other
-- boss is alive: Talk(SAY_DIED_FIRST 2) + RemoveFlag(UNIT_DYNFLAG_LOOTABLE)
-- + cross-creature DoAction(ACTION_OTHER_JUST_DIED) + TRIGGERED
-- DoCast(me, SPELL_SUMMON_SKARVALD_GHOST 48613 / SPELL_SUMMON_DALRONN_GHOST
-- 48612, true) (line 128); when both are dead: Talk(SAY_DEATH 1) +
-- cross-creature DoAction(ACTION_DESPAWN_SUMMONS) + _JustDied().
-- PORTED 2026-10-21: the Talk + RemoveFlag legs are C++-exact under the
-- twin_valkyr per-pull death-counter model (first ev4 of the pull takes
-- the other-alive branch, second the both-dead branch; counter reset on
-- engage since ev23 fires ahead of ev4 on the death path). PORTED
-- 2026-10-24: the ACTION_OTHER_JUST_DIED leg — first-death handler relays
-- via relayDeathResponse (FindNearestCreature survivor + 2s timer keyed by
-- the survivor's GUID, maulgar precedent). STILL OUT: the triggered
-- ghost summon (summon leg), the summons-despawn leg, and _JustDied().
-- DoAction(ACTION_OTHER_JUST_DIED) (lines 144-157) — schedules
-- EVENT_DEATH_RESPONSE (2s) -> Talk(SAY_DEATH_RESPONSE 4) (lines
-- 159-163); ports via relayDeathResponse (no DoAction bridge needed —
-- bare-Talk receiver + bridged FindNearestCreature lookup).
-- ACTION_DESPAWN_SUMMONS (summons.DespawnAll()) — no bridge.
-- Dalronn EVENT_SUMMON_SKELETONS (lines 272-275): DoCast(me,
-- SPELL_SUMMON_SKELETONS 52611) self-cast summon, heroic-gated init
-- 10s, repeat {10s,30s} — PORTED 2026-10-25: creature:IsHeroic() is
-- bridged (lua_creature_events.go:1806); the stale "BLOCKED per
-- doctrine" claim is corrected.
-- Ghost-form Reset DoZoneInCombat (lines 108-116), BossAI
-- JustEngagedWith/_Reset/_JustDied passthroughs — no bridges.

local ENTRY_SKARVALD = 24200
local ENTRY_DALRONN = 24201

local SAY_AGGRO = 0
local SAY_DEATH = 1
local SAY_DIED_FIRST = 2
local SAY_KILL = 3
local SAY_DEATH_RESPONSE = 4

local SPELL_CHARGE = 43651
local SPELL_STONE_STRIKE = 48583
local SPELL_ENRAGE = 48193
local SPELL_SHADOW_BOLT = 43649
local SPELL_DEBILITATE = 43650
local SPELL_SUMMON_SKELETONS = 52611

local timers = {}
local enraged = {}

-- UNIT_DYNAMIC_FLAGS (UpdateFields.h:136: OBJECT_END+0x49 = 79);
-- UNIT_DYNFLAG_LOOTABLE (SharedDefines.h:3128).
local UNIT_DYNAMIC_FLAGS = 79
local UNIT_DYNFLAG_LOOTABLE = 0x0001

-- Both bosses share this module scope, so the C++ otherBoss->IsAlive()
-- liveness check is modeled as a per-pull death count: the first death of
-- a pull takes the other-alive branch, the second the both-dead branch
-- (twin_valkyr precedent). Reset on engage — ev23 also fires on the death
-- path (ahead of ev4), so the counter cannot live in reset().
local bossDeaths = 0

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

-- Random living threat-list players within [minDist, maxDist] of the
-- creature (C++ SelectTarget(Random, 0, dist, true) = player-only
-- threat pick; the keleseth randomFrostTombTarget precedent).
local function randomPlayer(creature, minDist, maxDist)
    local candidates = {}
    for _, plr in ipairs(creature:GetAITargets()) do
        if plr:GetObjectType() == "Player" and not plr:IsDead() then
            local dist = creature:GetDistance(plr) - creature:GetCombatReach()
            if (minDist <= 0 or dist >= minDist) and dist < maxDist then
                candidates[#candidates + 1] = plr
            end
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
-- bound was exclusive). C++ GetDistance2d subtracts both reaches
-- (Object.cpp:1187); Lua subtracts own reach (skadi idiom). SelectTargetList
-- skips offline refs (UnitAI.h), so the dead entries Go
-- GetAITargets still carries are filtered here.
local function randomTargetInBand(creature, minDist, maxDist)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        local dist = creature:GetDistance2d(t) - creature:GetCombatReach()
        if not t:IsDead() and dist >= minDist and dist <= maxDist then
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
    bossDeaths = 0
    creature:Talk(SAY_AGGRO)
    schedule(guid, "charge", 5000, function() chargeTick(creature, guid) end)
    schedule(guid, "stonestrike", 10000, function() stoneStrikeTick(creature, guid) end)
end

local function skarvaldLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ generic_boss_controllerAI::JustDied (lines 119-137) + DoAction
-- (ACTION_OTHER_JUST_DIED) (lines 144-157): the first death of the pull
-- schedules EVENT_DEATH_RESPONSE (2s) on the surviving boss, which then
-- Talks SAY_DEATH_RESPONSE. No DoAction bridge exists, but the receiver
-- lookup is the bridged FindNearestCreature (maulgar precedent) and the
-- receiver leg is a bare Talk, so the relay ports: the 2s timer is keyed
-- by the survivor's GUID, so the survivor's own ev2/ev4/ev23 cancelTimers
-- drops it if the survivor dies or leaves combat first (C++ _JustDied
-- resets the survivor's events on its own death). Nil survivor (dead or
-- absent — the bridge skips Health==0 entries) means no relay, matching
-- the C++ otherBoss->IsAlive() gate.
local function relayDeathResponse(creature, otherEntry)
    local survivor = creature:FindNearestCreature(otherEntry, 100.0)
    if survivor ~= nil then
        schedule(survivor:GetGUID(), "deathresponse", 2000, function() survivor:Talk(SAY_DEATH_RESPONSE) end)
    end
end

local function skarvaldDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    -- C++ generic_boss_controllerAI::JustDied (lines 119-137): first death
    -- of the pull takes the other-alive branch — Talk(SAY_DIED_FIRST),
    -- then RemoveFlag(UNIT_DYNAMIC_FLAGS, UNIT_DYNFLAG_LOOTABLE); second
    -- death takes the both-dead branch — Talk(SAY_DEATH). The ghost-summon
    -- DoCast and the summons-despawn leg stay out (no summon bridges);
    -- the OTHER_JUST_DIED leg ports via relayDeathResponse.
    bossDeaths = bossDeaths + 1
    if bossDeaths >= 2 then
        creature:Talk(SAY_DEATH)
    else
        creature:Talk(SAY_DIED_FIRST)
        creature:RemoveFlag(UNIT_DYNAMIC_FLAGS, UNIT_DYNFLAG_LOOTABLE)
        relayDeathResponse(creature, ENTRY_DALRONN)
    end
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
local function summonSkeletonsTick(creature, guid)
    creature:CastSpell(creature, SPELL_SUMMON_SKELETONS)
    schedule(guid, "summonskeletons", {10000, 30000}, function() summonSkeletonsTick(creature, guid) end)
end

local function dalronnEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    bossDeaths = 0
    schedule(guid, "shadowbolt", 1000, function() shadowBoltTick(creature, guid) end)
    schedule(guid, "debilitate", 5000, function() debilitateTick(creature, guid) end)
    -- C++ EVENT_SUMMON_SKELETONS (lines 272-275): DoCast(me,
    -- SPELL_SUMMON_SKELETONS), 2-arg = non-triggered, self-cast;
    -- heroic-gated init 10s (line 257), repeat {10s,30s} (line 274).
    if creature:IsHeroic() then
        schedule(guid, "summonskeletons", 10000, function() summonSkeletonsTick(creature, guid) end)
    end
    -- C++ EVENT_DELAYED_AGGRO_SAY (lines 276-278): Talk(SAY_AGGRO), init 5s
    -- if !IsInGhostForm (lines 251-252; structural); one-shot.
    schedule(guid, "aggrosay", 5000, function() creature:Talk(SAY_AGGRO) end)
end

local function dalronnLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function dalronnDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    -- Same C++ generic_boss_controllerAI::JustDied model as skarvaldDied:
    -- first death of the pull takes the other-alive branch, second the
    -- both-dead branch; summon legs stay out, OTHER_JUST_DIED ports via
    -- relayDeathResponse.
    bossDeaths = bossDeaths + 1
    if bossDeaths >= 2 then
        creature:Talk(SAY_DEATH)
    else
        creature:Talk(SAY_DIED_FIRST)
        creature:RemoveFlag(UNIT_DYNAMIC_FLAGS, UNIT_DYNFLAG_LOOTABLE)
        relayDeathResponse(creature, ENTRY_SKARVALD)
    end
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
