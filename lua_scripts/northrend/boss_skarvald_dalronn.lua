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
-- the script names DB-side and PORTED this tick (ghost handlers below):
-- ghost Reset DoZoneInCombat (:108-116), ghost JustEngagedWith schedule
-- legs (charge/stone-strike :192-201, shadow-bolt/debilitate/skeletons
-- :244-254 — only the Talk / delayed aggro say are !IsInGhostForm
-- gated), and the shared generic JustDied machine (:119-137, incl. the
-- triggered ghost re-summon). KilledUnit (:165-169) and the Skarvald
-- DamageTaken enrage (:218-227) stay ghost-gated in C++ — no ghost
-- registrations for events 3 / 9.
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
-- Cast-state gates (2026-11-04): BossAI::UpdateAI pre-loop + in-loop
-- CASTING returns (ScriptedCreature.cpp:578/:584; neither boss overrides
-- UpdateAI) ported as 500ms IsCasting defer (live,
-- lua_creature_events.go:1950) on all timer arms incl. the delayed
-- aggro Talk and the death-response relay; DamageTaken / KilledUnit /
-- JustDied hooks ungated per C++ (separate hooks).
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
-- generic_boss_controllerAI::JustDied (lines 119-137) — PORTED this tick
-- as genericBossDied, shared by the real bosses and their ghosts: the
-- branch is the C++ instance GetGuidData(OtherBossData) liveness check
-- (the per-pull death-counter model is retired — the ev5 SetGuidData
-- latches below feed it; nil other falls through to the both-dead
-- branch). First death: Talk(SAY_DIED_FIRST 2) + RemoveFlag
-- (UNIT_DYNFLAG_LOOTABLE) + ACTION_OTHER_JUST_DIED relay (2s death-
-- response Talk on the other boss) + TRIGGERED DoCast(me,
-- SPELL_SUMMON_SKARVALD_GHOST 48613 / SPELL_SUMMON_DALRONN_GHOST 48612,
-- true) (:128) as a direct SummonCreature at the dying boss (the
-- summon spell has no Lua arm; the keleseth tomb precedent). Both
-- dead: Talk(SAY_DEATH 1) + ACTION_DESPAWN_SUMMONS (despawn the other
-- boss's latched ghost) + _JustDied() SetBossState(DATA_SKARVALD_DALRONN
-- 1, DONE) (tick 260) — C++ order :131 Talk, :132 DoAction, :133
-- _JustDied, so DONE lands after the death Talk.
-- DoAction(ACTION_OTHER_JUST_DIED) (lines 144-157) — schedules
-- EVENT_DEATH_RESPONSE (2s) -> Talk(SAY_DEATH_RESPONSE 4) (lines
-- 159-163); ports via relayDeathResponse (ev38 dispatch IS bridged,
-- lua_creature_events.go:802, but no dispatch is needed — the sender
-- is the ported JustDied hook and the receiver is a bare Talk on the
-- instance-GUID-resolved other boss, no range limit).
-- ACTION_DESPAWN_SUMMONS (summons.DespawnAll()) — PORTED this tick via
-- the per-boss ghost latch (ghostSummons).
-- Dalronn EVENT_SUMMON_SKELETONS (lines 272-275): DoCast(me,
-- SPELL_SUMMON_SKELETONS 52611) self-cast summon, heroic-gated init
-- 10s, repeat {10s,30s} — PORTED 2026-10-25: creature:IsHeroic() is
-- bridged (lua_creature_events.go:1806); the stale "BLOCKED per
-- doctrine" claim is corrected.
-- Ghost-form Reset DoZoneInCombat (lines 108-116) — PORTED this tick
-- (ghostReset below: DoZoneInCombat + engage arms, alive-guarded since
-- ev23 pre-fires on death; the ghost JustEngagedWith schedule legs ride
-- ghostEnterCombat, Talk/State gated per C++). BossAI JustEngagedWith /
-- _Reset / _JustDied passthroughs
-- PORTED tick 260 via creature:SetBossState (lua_creature_events.go:5917)
-- in the engage / reset / both-dead-died handlers below; the stale
-- "no bridges" claim is retracted.

local ENTRY_SKARVALD = 24200
local ENTRY_DALRONN = 24201
local DATA_SKARVALD_DALRONN = 1 -- utgarde_keep.h:32

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
local SPELL_SUMMON_SKARVALD_GHOST = 48613 -- boss_skarvald_dalronn.cpp:46
local SPELL_SUMMON_DALRONN_GHOST = 48612 -- boss_skarvald_dalronn.cpp:55

local ENTRY_SKARVALD_GHOST = 27390 -- utgarde_keep.h NPC_SKARVALD_GHOST
local ENTRY_DALRONN_GHOST = 27389 -- utgarde_keep.h NPC_DALRONN_GHOST
local DATA_SKARVALD = 3 -- utgarde_keep.h:36
local DATA_DALRONN = 4 -- utgarde_keep.h:37

local timers = {}
local enraged = {}

-- UNIT_DYNAMIC_FLAGS (UpdateFields.h:136: OBJECT_END+0x49 = 79);
-- UNIT_DYNFLAG_LOOTABLE (SharedDefines.h:3128).
local UNIT_DYNAMIC_FLAGS = 79
local UNIT_DYNFLAG_LOOTABLE = 0x0001

-- Ghost latch: the live ghost summoned by each boss, keyed by that
-- boss's DATA id (DATA_SKARVALD 3 / DATA_DALRONN 4) — the Lua mirror of
-- the BossAI summons list the C++ ACTION_DESPAWN_SUMMONS leg clears.
-- The old per-pull death-counter model is retired this tick: the branch
-- is the C++ instance-GUID liveness check (genericBossDied below), which
-- has no range limit (the old FindNearestCreature 100yd relay could miss
-- a survivor across the room).
local ghostSummons = {}

-- Forward declarations (tharon_ja / gal_darah idiom): genericBossDied
-- below references the ghost engage arms, defined after it — a later
-- `local function` re-declare would shadow this slot and nil-call, so
-- the definitions below are plain assignments, never re-local.
local ghostSkarvaldEngage, ghostDalronnEngage

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
            local dist = creature:GetDistance(plr) - creature:GetCombatReach() - 1.5
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
-- (Object.cpp:1187); unfiltered pool branches target reach. SelectTargetList
-- skips offline refs (UnitAI.h), so the dead entries Go
-- GetAITargets still carries are filtered here.
local function randomTargetInBand(creature, minDist, maxDist)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        local targetReach = 1.5
        if t:GetObjectType() ~= "Player" then
            targetReach = t:GetCombatReach()
        end
        local dist = creature:GetDistance2d(t) - creature:GetCombatReach() - targetReach
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
    if creature:IsCasting() then
        schedule(guid, "stonestrike", 500, function()
            stoneStrikeTick(creature, guid)
        end)
        return
    end
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
    if creature:IsCasting() then
        schedule(guid, "charge", 500, function()
            chargeTick(creature, guid)
        end)
        return
    end
    local target = randomTargetInBand(creature, 5, 30)
    if target then
        creature:CastSpell(target, SPELL_CHARGE)
    end
end

-- C++ boss_skarvald_the_constructorAI::JustEngagedWith (lines 192-201):
-- BossAI passthrough (SetCombatPulseDelay(5) + me-form DoZoneInCombat
-- ported into skarvaldEnterCombat, base-first); if (!IsInGhostForm) Talk(SAY_AGGRO) —
-- gate satisfied structurally (ghost entry 27390 not registered).
local function skarvaldEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    -- generic_boss_controllerAI::JustEngagedWith (:192, base-first;
    -- generic base gates on !IsInGhostForm, structural: ghost entries
    -- unregistered) — SetBossState(DATA_SKARVALD_DALRONN 1, IN_PROGRESS)
    -- PORTED tick 260 via creature:SetBossState (lua_creature_events.go:5917),
    -- before SetCombatPulseDelay(5) + me-form DoZoneInCombat (C++
    -- BossAI::_JustEngagedWith order, ScriptedCreature.cpp:527-544).
    creature:SetBossState(DATA_SKARVALD_DALRONN, 1)
    creature:SetCombatPulseDelay(5)
    creature:DoZoneInCombat()
    creature:Talk(SAY_AGGRO)
    schedule(guid, "charge", 5000, function() chargeTick(creature, guid) end)
    schedule(guid, "stonestrike", 10000, function() stoneStrikeTick(creature, guid) end)
end

local function skarvaldLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ generic_boss_controllerAI::JustDied (lines 119-137) + DoAction
-- (ACTION_OTHER_JUST_DIED) (lines 144-157): the first death of the pull
-- schedules EVENT_DEATH_RESPONSE (2s) on the surviving boss, which then
-- Talks SAY_DEATH_RESPONSE. Ev38 DoAction dispatch IS bridged
-- (creature:DoAction, lua_creature_events.go:802), but the relay does
-- not use it: the sender is the ported JustDied hook (ev4) and the
-- receiver lookup is the bridged FindNearestCreature (maulgar
-- precedent) and the receiver leg is a bare Talk, so the relay ports
-- directly: the 2s timer is keyed
-- by the survivor's GUID, so the survivor's own ev2/ev4/ev23 cancelTimers
-- drops it if the survivor dies or leaves combat first (C++ _JustDied
-- resets the survivor's events on its own death). Nil survivor (dead or
-- absent — the bridge skips Health==0 entries) means no relay, matching
-- the C++ otherBoss->IsAlive() gate.
-- C++ EVENT_DEATH_RESPONSE (lines 159-163): Talk(SAY_DEATH_RESPONSE)
-- — an event-loop arm under the BossAI UpdateAI gates, so it defers
-- while the survivor casts like the other timer arms.
local function deathResponseTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "deathresponse", 500, function() deathResponseTick(creature, guid) end)
        return
    end
    creature:Talk(SAY_DEATH_RESPONSE)
end

-- C++ DoAction(ACTION_OTHER_JUST_DIED) (:144-157) schedules
-- EVENT_DEATH_RESPONSE (2s) on the OTHER boss's EventMap — resolved via
-- the instance GUID store, no range limit (the old FindNearestCreature
-- 100yd relay could miss a survivor across the room).
local function relayDeathResponse(creature, otherData)
    local other = creature:GetInstanceCreature(otherData)
    if other ~= nil and other:IsAlive() then
        local guid = other:GetGUIDLow()
        schedule(guid, "deathresponse", 2000, function() deathResponseTick(other, other:GetGUIDLow()) end)
    end
end

-- C++ generic_boss_controllerAI::JustDied (:119-137), shared by the real
-- bosses and their ghosts (same AI class, same script name): otherBoss =
-- instance GetGuidData(OtherBossData); alive -> Talk SAY_DIED_FIRST +
-- RemoveFlag lootable + DoAction(OTHER_JUST_DIED) on the other boss +
-- triggered ghost summon of SELF; dead -> Talk SAY_DEATH +
-- DoAction(DESPAWN_SUMMONS) on the other boss + _JustDied()
-- (SetBossState DONE). Nil other (unlatched GUID — cannot happen in UK,
-- both bosses spawn at instance load) falls through to the both-dead
-- branch so the encounter stays completable. A ghost dying while the
-- other real boss lives re-summons its own ghost — C++-exact per :128.
local function genericBossDied(creature, selfData, otherData, ghostEntry)
    cancelTimers(creature:GetGUIDLow())
    local other = creature:GetInstanceCreature(otherData)
    if other ~= nil and other:IsAlive() then
        creature:Talk(SAY_DIED_FIRST)
        creature:RemoveFlag(UNIT_DYNAMIC_FLAGS, UNIT_DYNFLAG_LOOTABLE)
        relayDeathResponse(creature, otherData)
        -- C++ :128 triggered DoCast(me, SPELL_SUMMON_*_GHOST, true): the
        -- summon spell has no Lua arm, so the ghost is summoned directly
        -- at the dying boss (the keleseth tomb precedent); 5-arg
        -- SummonCreature = MANUAL_DESPAWN, the ghost lives until killed
        -- or despawned by the both-dead leg below.
        local ghost = creature:SummonCreature(ghostEntry, creature:GetX(), creature:GetY(), creature:GetZ(), creature:GetO())
        if ghost ~= nil then
            ghostSummons[selfData] = ghost
            -- C++ ghost Reset (:108-116) DoZoneInCombat + schedule legs:
            -- armed off the returned object (temp summons may never fire
            -- ev23; the keleseth decrepify precedent); the ghost ev1/ev23
            -- handlers re-arm idempotently via schedule() key replace.
            ghost:DoZoneInCombat()
            if ghostEntry == ENTRY_SKARVALD_GHOST then
                ghostSkarvaldEngage(ghost)
            else
                ghostDalronnEngage(ghost)
            end
        end
    else
        creature:Talk(SAY_DEATH)
        -- C++ :132 DoAction(ACTION_DESPAWN_SUMMONS) on the other boss:
        -- its summons.DespawnAll() drops the other boss's ghost.
        local otherGhost = ghostSummons[otherData]
        if otherGhost ~= nil then
            otherGhost:Despawn()
            ghostSummons[otherData] = nil
        end
        -- C++ :133 _JustDied() — SetBossState(DATA_SKARVALD_DALRONN 1,
        -- DONE) PORTED tick 260; after the Talk per C++ order.
        creature:SetBossState(DATA_SKARVALD_DALRONN, 3)
    end
end

local function skarvaldDied(event, creature, killer)
    genericBossDied(creature, DATA_SKARVALD, DATA_DALRONN, ENTRY_SKARVALD_GHOST)
end

local function skarvaldReset(event, creature)
    -- C++ generic_boss_controllerAI::Reset (:102-112): non-ghost
    -- _Reset() — SetBossState(DATA_SKARVALD_DALRONN 1, NOT_STARTED)
    -- PORTED tick 260 via creature:SetBossState (lua_creature_events.go:5917),
    -- first per the auriaya/ignis precedent; the ghost-form
    -- DoZoneInCombat leg is structural (ghosts unregistered).
    creature:SetBossState(DATA_SKARVALD_DALRONN, 0)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    enraged[guid] = nil
    -- C++ _Reset() summons.DespawnAll(): the latched ghost goes with it.
    local ghost = ghostSummons[DATA_SKARVALD]
    if ghost ~= nil then
        ghost:Despawn()
        ghostSummons[DATA_SKARVALD] = nil
    end
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
    local guid = creature:GetGUIDLow()
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
    if creature:IsCasting() then
        schedule(guid, "shadowbolt", 500, function()
            shadowBoltTick(creature, guid)
        end)
        return
    end
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
    if creature:IsCasting() then
        schedule(guid, "debilitate", 500, function()
            debilitateTick(creature, guid)
        end)
        return
    end
    local target = randomPlayer(creature, 0, 50)
    if target then
        creature:CastSpell(target, SPELL_DEBILITATE)
    end
    schedule(guid, "debilitate", {5000, 10000}, function() debilitateTick(creature, guid) end)
end

-- C++ EVENT_DELAYED_AGGRO_SAY (lines 276-278): Talk(SAY_AGGRO) — an
-- event-loop arm under the BossAI UpdateAI gates, so it defers while
-- casting like the other timer arms.
local function aggroSayTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "aggrosay", 500, function() aggroSayTick(creature, guid) end)
        return
    end
    creature:Talk(SAY_AGGRO)
end

-- C++ boss_dalronn_the_controllerAI::JustEngagedWith (lines 244-254):
-- BossAI passthrough (SetCombatPulseDelay(5) + me-form DoZoneInCombat
-- ported into dalronnEnterCombat, base-first); the schedule legs become CreateLuaEvent
-- timers. No immediate Talk — SAY_AGGRO rides EVENT_DELAYED_AGGRO_SAY.
local function summonSkeletonsTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "summonskeletons", 500, function()
            summonSkeletonsTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_SUMMON_SKELETONS)
    schedule(guid, "summonskeletons", {10000, 30000}, function() summonSkeletonsTick(creature, guid) end)
end

local function dalronnEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    -- generic_boss_controllerAI::JustEngagedWith (:252, base-first;
    -- generic base gates on !IsInGhostForm, structural: ghost entries
    -- unregistered) — SetBossState(DATA_SKARVALD_DALRONN 1, IN_PROGRESS)
    -- PORTED tick 260 via creature:SetBossState (lua_creature_events.go:5917),
    -- before SetCombatPulseDelay(5) + me-form DoZoneInCombat (C++
    -- BossAI::_JustEngagedWith order, ScriptedCreature.cpp:527-544).
    creature:SetBossState(DATA_SKARVALD_DALRONN, 1)
    creature:SetCombatPulseDelay(5)
    creature:DoZoneInCombat()
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
    schedule(guid, "aggrosay", 5000, function() aggroSayTick(creature, guid) end)
end

local function dalronnLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

local function dalronnDied(event, creature, killer)
    genericBossDied(creature, DATA_DALRONN, DATA_SKARVALD, ENTRY_DALRONN_GHOST)
end

local function dalronnReset(event, creature)
    -- C++ generic_boss_controllerAI::Reset (:102-112): non-ghost
    -- _Reset() — SetBossState(DATA_SKARVALD_DALRONN 1, NOT_STARTED)
    -- PORTED tick 260 via creature:SetBossState (lua_creature_events.go:5917),
    -- first per the auriaya/ignis precedent; the ghost-form
    -- DoZoneInCombat leg is structural (ghosts unregistered).
    creature:SetBossState(DATA_SKARVALD_DALRONN, 0)
    cancelTimers(creature:GetGUIDLow())
    -- C++ _Reset() summons.DespawnAll(): the latched ghost goes with it.
    local ghost = ghostSummons[DATA_DALRONN]
    if ghost ~= nil then
        ghost:Despawn()
        ghostSummons[DATA_DALRONN] = nil
    end
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

-- C++ instance_utgarde_keep.cpp:57/:140 OnCreatureCreate latches the
-- boss GUIDs; the Lua mirror is the ev5 SetGuidData (sjonnir precedent)
-- so the other-boss lookups (genericBossDied / relayDeathResponse)
-- resolve via the instance GUID store.
local function skarvaldSpawned(event, creature)
    creature:SetGuidData(DATA_SKARVALD)
end

local function dalronnSpawned(event, creature)
    creature:SetGuidData(DATA_DALRONN)
end

-- C++ boss_skarvald_the_constructorAI::JustEngagedWith (:192-201) for
-- the GHOST (entry 27390): the Talk is !IsInGhostForm-gated, but the
-- charge 5s + stone strike 10s schedules run unconditionally.
ghostSkarvaldEngage = function(creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "charge", 5000, function() chargeTick(creature, guid) end)
    schedule(guid, "stonestrike", 10000, function() stoneStrikeTick(creature, guid) end)
end

-- C++ boss_dalronn_the_controllerAI::JustEngagedWith (:244-254) for the
-- GHOST (entry 27389): shadow bolt 1s + debilitate 5s run
-- unconditionally; summon skeletons 10s if heroic; the delayed aggro
-- say is !IsInGhostForm-gated and never schedules.
ghostDalronnEngage = function(creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "shadowbolt", 1000, function() shadowBoltTick(creature, guid) end)
    schedule(guid, "debilitate", 5000, function() debilitateTick(creature, guid) end)
    if creature:IsHeroic() then
        schedule(guid, "summonskeletons", 10000, function() summonSkeletonsTick(creature, guid) end)
    end
end

-- C++ ghost JustEngagedWith: the BossAI passthrough (SetBossState /
-- SetCombatPulseDelay / DoZoneInCombat) and the SAY_AGGRO Talk are
-- !IsInGhostForm-gated — only the schedule legs run. No Talk, no state.
local function ghostEnterCombat(event, creature, target)
    if creature:GetEntry() == ENTRY_SKARVALD_GHOST then
        ghostSkarvaldEngage(creature)
    else
        ghostDalronnEngage(creature)
    end
end

local function ghostLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ generic_boss_controllerAI::Reset (:108-116): ghost form calls
-- DoZoneInCombat(me) — ghosts aren't set in combat as they spawn.
-- Alive-guarded: ev23 pre-fires on death (fireCreatureDied runs Reset
-- before the ev4 JustDied leg), and a dead ghost must not re-engage.
local function ghostReset(event, creature)
    if not creature:IsAlive() then
        return
    end
    creature:DoZoneInCombat()
    if creature:GetEntry() == ENTRY_SKARVALD_GHOST then
        ghostSkarvaldEngage(creature)
    else
        ghostDalronnEngage(creature)
    end
end

-- C++ generic_boss_controllerAI::JustDied (:119-137) runs for ghosts
-- too (same AI class): a ghost dying while the other real boss lives
-- re-summons its own ghost (:128); both real bosses dead -> SAY_DEATH
-- + despawn the other ghost + DONE.
local function ghostDied(event, creature, killer)
    if creature:GetEntry() == ENTRY_SKARVALD_GHOST then
        genericBossDied(creature, DATA_SKARVALD, DATA_DALRONN, ENTRY_SKARVALD_GHOST)
    else
        genericBossDied(creature, DATA_DALRONN, DATA_SKARVALD, ENTRY_DALRONN_GHOST)
    end
end

RegisterCreatureEvent(ENTRY_SKARVALD, 5, skarvaldSpawned)
RegisterCreatureEvent(ENTRY_DALRONN, 5, dalronnSpawned)
RegisterCreatureEvent(ENTRY_SKARVALD_GHOST, 1, ghostEnterCombat)
RegisterCreatureEvent(ENTRY_SKARVALD_GHOST, 2, ghostLeaveCombat)
RegisterCreatureEvent(ENTRY_SKARVALD_GHOST, 4, ghostDied)
RegisterCreatureEvent(ENTRY_SKARVALD_GHOST, 23, ghostReset)
RegisterCreatureEvent(ENTRY_DALRONN_GHOST, 1, ghostEnterCombat)
RegisterCreatureEvent(ENTRY_DALRONN_GHOST, 2, ghostLeaveCombat)
RegisterCreatureEvent(ENTRY_DALRONN_GHOST, 4, ghostDied)
RegisterCreatureEvent(ENTRY_DALRONN_GHOST, 23, ghostReset)
