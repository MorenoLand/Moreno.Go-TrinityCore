-- Nexus-Prince Shaffar (Mana Tombs, Auchindoun) — Lua port of
-- src/server/scripts/Outland/Auchindoun/ManaTombs/boss_nexusprince_shaffar.cpp
-- (4 CreatureScripts: boss_nexusprince_shaffar (BossAI via
-- GetManaTombsAI, DATA_NEXUSPRINCE_SHAFFAR) + npc_ethereal_beacon
-- (ScriptedAI) + npc_ethereal_apprentice (ScriptedAI) + npc_yor
-- (ScriptedAI); all registered from inside
-- AddSC_boss_nexusprince_shaffar() (line 387); loader decl 26 / call
-- 149 per outland_script_loader.cpp — the SECOND group of the
-- "// Auchindoun - Mana Tombs" sub-block in AddOutlandScripts(),
-- immediately after AddSC_boss_pandemonius() (call 148) — verified
-- from the loader this run; the checkpoint sequence (pandemonius ->
-- nexusprince_shaffar) is followed).
-- Entries: 18344 NPC_SHAFFAR / 18431 NPC_BEACON — from the file's own
-- Creatures enum; the ethereal apprentice entry (18430) appears only
-- in the SPELL_ETHEREAL_APPRENTICE comment; the ScriptName bindings
-- are DB-side as usual. Sole-source verified: whole-server-tree grep
-- for "AddSC_boss_nexusprince_shaffar" hits
-- boss_nexusprince_shaffar.cpp (+ the loader decl/call lines) only.
-- No shaffar lua existed.
-- Eluna creature events: 1 OnEnterCombat, 3 OnTargetDied, 4 OnDied.
-- Ported arms (C++-exact for all modeled arms):
-- boss_nexusprince_shaffarAI::JustEngagedWith — Talk(SAY_AGGRO 1)
-- (line 137; event 1 — the auriaya precedent) + schedules
-- EVENT_FIREBALL (8s) / EVENT_FROSTBOLT (4s) / EVENT_FROST_NOVA (15s)
-- (the BossAI::JustEngagedWith legs have no bridge — tharon_ja
-- precedent; EVENT_BEACON ported below as a direct-summon
-- supplement, see below).
-- EVENT_FIREBALL — DoCastVictim(SPELL_FROSTBOLT 32364) 2-arg
-- (non-triggered), 8s init, randtime(4500ms, 6s) repeat (the C++
-- fires SPELL_FROSTBOLT under the EVENT_FIREBALL case — preserved
-- C++-exact); EVENT_FROSTBOLT — DoCastVictim(SPELL_FROSTBOLT 32364),
-- 4s init, randtime(4500ms, 6s) repeat; EVENT_FROST_NOVA —
-- DoCastSelf(SPELL_FROSTNOVA 32365) 2-arg (non-triggered), 15s init,
-- randtime(17500ms, 25s) repeat (the chained EVENT_BLINK 1500ms leg
-- has no motion/interrupt bridges — documented, not wired). All
-- timers cancelled on 2/4/23 (gargolmar precedent).
-- boss_nexusprince_shaffarAI::KilledUnit — Talk(SAY_SLAY 2) (line
-- 161) gated on victim->GetTypeId() == TYPEID_PLAYER — the
-- razuvious player-gated precedent (event 3).
-- boss_nexusprince_shaffarAI::JustDied — Talk(SAY_DEAD 4) (line
-- 166) — the sjonnir self-Talk precedent (event 4).
-- boss_nexusprince_shaffarAI::MoveInLineOfSight — the
-- _hasTaunted one-shot Talk(SAY_INTRO 0) (line 101) — PORTED as
-- shaffarMoveInLOS on event 27 (per-GUID never-cleared latch:
-- _hasTaunted is ctor-only in C++ — Reset() re-summons beacons but
-- never touches it, ossirian precedent); returns true to veto the
-- default aggro engage, matching the C++ body which never calls
-- the base (depth_charge precedent).
-- DOCUMENTED-ONLY (in this header; entries 18344/18431/18430 registered):
-- boss_nexusprince_shaffarAI::Reset — the three initial beacon
-- SummonCreature(NPC_BEACON 18431, TEMPSUMMON_CORPSE_TIMED_DESPAWN 2h)
-- legs are PORTED on ev23 (live summon bridge 2026-10-22); the beacon
-- AI (npc_ethereal_beaconAI) is PORTED below (18431 registrations).
-- boss_nexusprince_shaffarAI::MoveInLineOfSight — Talk(SAY_INTRO 0)
-- (line 101) one-shot _hasTaunted-gated on a player within 100.0f —
-- PORTED on event 27 (CREATURE_EVENT_ON_MOVE_IN_LOS): Go's fire
-- site is the aggro-engage branch ("about to aggro a player it has
-- sighted"), which is a SUBSET of the C++ trigger — at fire time
-- dist <= aggro range (<=45) < 100 and who is always a player, so
-- both C++ gates hold. The C++ body never calls
-- ScriptedAI::MoveInLineOfSight(who), so the handler returns true
-- to veto the default engage (depth_charge precedent), matching
-- C++ where the boss never aggroes by sight. The old
-- "fire-site mismatch / documented-only" note (the maladaar
-- precedent) was stale: event 27 fires live from the Go aggro
-- path (creaturemotion.go), and the gargolmar taunt arm is the one
-- genuinely unfirable here (its else-branch fires only when the
-- player is NOT engaged).
-- boss_nexusprince_shaffarAI::JustSummoned — beacon visual leg PORTED
-- below (shaffarJustSummoned: non-triggered ETHEREAL_BEACON_VISUAL
-- self-cast; ev19 fires for the Lua Reset summons). The
-- Random-SelectTarget AttackStart leg PORTED below (EngageWithTarget
-- twin, 2026-10-24); the beacon AI's own JustSummoned (:223) is PORTED
-- below (beaconJustSummoned: apprentice engages the beacon's victim).
-- boss_nexusprince_shaffarAI::ExecuteEvent:
-- EVENT_BEACON — urand(0,3)==0-gated Talk(SAY_SUMMON 3) + DoCast(me,
-- SPELL_ETHEREAL_BEACON 32371, triggered), 10s repeat — PORTED as a
-- direct-summon supplement: the spell summons NPC_BEACON 18431
-- (Creatures enum, same entry as the Reset beacons) at the caster,
-- so the port summons it at shaffar's position (type 5, 2h, as
-- the Reset beacons); the JustSummoned visual + random-engage
-- arms fire via event 19 exactly as for Reset beacons. The beacon
-- AI itself is PORTED below (18431 arcane-bolt / apprentice +
-- 18430 ping-pong registrations); yor entry still DB-side
-- unverified, not guessed;
-- EVENT_BLINK — interrupt + Clear(MOTION_PRIORITY_NORMAL) +
-- DoCast(SPELL_BLINK 34605) — stays OUT: the interrupt/motion legs
-- have no bridge (the blink teleport effect has no lua-bridge model);
-- EVENT_FIREBALL / EVENT_FROSTBOLT / EVENT_FROST_NOVA ported above.
-- npc_ethereal_beacon (18431) — zero Talk; PORTED below (registered
-- as npc_ethereal_beacon in boss_ai.go): JustEngagedWith cross-creature
-- leg via FindNearestCreature(18344, 100.0) + shaffar:EngageWithTarget
-- (who) when shaffar not in combat (FindNearestCreature /
-- IsInCombat / EngageWithTarget all bridged); EVENT_APPRENTICE
-- DUNGEON_MODE 20s/10s (IsHeroic bridge) as a direct summon of 18430
-- (type 7 manual-despawn — the spell's duration is not C++-visible,
-- maladaar D'ore precedent) + Despawn, replacing the triggered
-- spell-effect summon (shaffar EVENT_BEACON precedent); EVENT_ARCANE_BOLT
-- 1s init -> 2-4.5s loop, non-triggered victim cast. The C++ UpdateAI
-- casting-state early return has no Lua model (as in all timer ports).
-- The stale "no-timer / no-cast" claim is corrected — the legs bridge.
-- npc_ethereal_apprentice (18430) — zero Talk; PORTED below (registered
-- as npc_ethereal_apprentice in boss_ai.go): the alternating
-- EVENT_ETHEREAL_APPRENTICE_FIREBOLT (SPELL_FIREBOLT 32369, triggered)
-- / EVENT_ETHEREAL_APPRENTICE_FROSTBOLT (SPELL_FROSTBOLT 32370,
-- triggered) 3s-ping-pong victim-cast machine (forward-declared
-- locals, tharon_ja/gal_darah idiom — never re-local function).
-- The stale "no-timer / no-cast" claim is corrected.
-- npc_yor — zero Talk; NOT registered: the EVENT_DOUBLE_BREATH
-- 6-9s machine (IsWithinDist(victim, ATTACK_DISTANCE)-gated
-- non-triggered DoCastVictim(SPELL_DOUBLE_BREATH 38361) +
-- DoMeleeAttackIfReady) — timer + cast + IsWithinDist gate legs
-- would all bridge (the gate is IsWithinDist itself, generic
-- fallback in engine/scripting/object.go — the "no bridge" claim
-- is stale), but the entry is DB-side only: a whole-C++-tree grep
-- for npc_yor hits only boss_nexusprince_shaffar.cpp itself (no
-- Creatures entry constant), so no entry can be registered without
-- guessing — stays out on unverified entry alone.
-- All Talk() calls in the file accounted for (3 ported above +
-- SAY_INTRO 0 MoveInLineOfSight + SAY_SUMMON 3 EVENT_BEACON machine
-- documented above).

local ENTRY_SHAFFAR = 18344 -- NPC_SHAFFAR (Creatures enum)
local ENTRY_BEACON = 18431 -- NPC_BEACON (boss_nexusprince_shaffar.cpp:61)

local SAY_INTRO = 0
local SAY_AGGRO = 1
local SAY_SLAY  = 2
local SAY_SUMMON = 3
local SAY_DEAD  = 4

local SPELL_FROSTBOLT = 32364 -- boss_nexusprince_shaffar.cpp:42
local SPELL_FROSTNOVA = 32365 -- boss_nexusprince_shaffar.cpp:44
local SPELL_ETHEREAL_BEACON_VISUAL = 32368 -- boss_nexusprince_shaffar.cpp:47
local SPELL_ARCANE_BOLT = 15254 -- boss_nexusprince_shaffar.cpp:50
local SPELL_ETHEREAL_APPRENTICE = 32372 -- boss_nexusprince_shaffar.cpp:51 (summon spell; ported as a direct summon, see beaconApprenticeTick)
local SPELL_APPRENTICE_FIREBOLT = 32369 -- boss_nexusprince_shaffar.cpp:268
local SPELL_APPRENTICE_FROSTBOLT = 32370 -- boss_nexusprince_shaffar.cpp:269
local ENTRY_APPRENTICE = 18430 -- summoned by SPELL_ETHEREAL_APPRENTICE (comment at boss_nexusprince_shaffar.cpp:51)

local timers = {}
local taunted = {} -- per-GUID _hasTaunted latch (C++ _hasTaunted, :184)

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

-- C++ SelectTarget(Random, 0) in JustSummoned (:125): dist 0 = no range
-- gate, playerOnly unset = any attackable unit. The Lua pool is alive
-- players on the same map+instance (threat-list members the bridge can
-- address); a non-player C++ pick no-ops through EngageWithTarget's
-- player-session gate, so the engage fires exactly for player picks.
local function randomAttackablePlayer(creature)
    local players = {}
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            players[#players + 1] = p
        end
    end
    if #players == 0 then
        return nil
    end
    return players[math.random(#players)]
end

-- C++ EVENT_FIREBALL (:165-167): DoCastVictim(SPELL_FROSTBOLT 32364)
-- 2-arg (non-triggered); 8s init, randtime(4500ms, 6s) repeat.
-- (The C++ fires SPELL_FROSTBOLT under the EVENT_FIREBALL case —
-- preserved C++-exact.)
local function fireballTick(creature, guid)
    creature:CastSpell(nil, SPELL_FROSTBOLT)
    schedule(guid, "fireball", math.random(4500, 6000), function()
        fireballTick(creature, guid)
    end)
end

-- C++ EVENT_FROSTBOLT (:169-171): DoCastVictim(SPELL_FROSTBOLT 32364)
-- 2-arg (non-triggered); 4s init, randtime(4500ms, 6s) repeat.
local function frostboltTick(creature, guid)
    creature:CastSpell(nil, SPELL_FROSTBOLT)
    schedule(guid, "frostbolt", math.random(4500, 6000), function()
        frostboltTick(creature, guid)
    end)
end

-- C++ EVENT_FROST_NOVA (:173-176): DoCast(me, SPELL_FROSTNOVA 32365)
-- 2-arg (non-triggered) self-cast; 15s init, randtime(17500ms, 25s)
-- repeat. The chained EVENT_BLINK 1500ms leg (interrupt + motion
-- clear + DoCast(SPELL_BLINK 34605)) has no motion/interrupt bridges
-- — documented, not wired.
local function frostNovaTick(creature, guid)
    creature:CastSpell(creature, SPELL_FROSTNOVA)
    schedule(guid, "frost_nova", math.random(17500, 25000), function()
        frostNovaTick(creature, guid)
    end)
end

-- C++ EVENT_BEACON (:158-163): urand(0,3)==0 Talk(SAY_SUMMON 3)
-- + triggered DoCast(me, SPELL_ETHEREAL_BEACON 32371), 10s repeat.
-- Ported as a direct summon of NPC_BEACON 18431 at shaffar's
-- position (type 5, 2h — the Reset-beacon precedent); the spell's
-- summon target is the caster, and event 19 fires the JustSummoned
-- visual + engage arms for the new beacon.
local function beaconTick(creature, guid)
    if math.random(0, 3) == 0 then
        creature:Talk(SAY_SUMMON)
    end
    creature:SummonCreature(ENTRY_BEACON, creature:GetX(), creature:GetY(),
        creature:GetZ(), 0, 5, 7200000)
    schedule(guid, "beacon", 10000, function()
        beaconTick(creature, guid)
    end)
end

-- C++ boss_nexusprince_shaffarAI::JustEngagedWith (:108-117):
-- Talk(SAY_AGGRO); BossAI::JustEngagedWith(who) — the auriaya
-- precedent (the BossAI legs have no bridge) — then the four
-- ScheduleEvent legs: EVENT_BEACON 10s / EVENT_FIREBALL 8s /
-- EVENT_FROSTBOLT 4s / EVENT_FROST_NOVA 15s.
local function shaffarEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "beacon", 10000, function()
        beaconTick(creature, guid)
    end)
    schedule(guid, "fireball", 8000, function()
        fireballTick(creature, guid)
    end)
    schedule(guid, "frostbolt", 4000, function()
        frostboltTick(creature, guid)
    end)
    schedule(guid, "frost_nova", 15000, function()
        frostNovaTick(creature, guid)
    end)
end

local function shaffarLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
end

local function shaffarReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    -- C++ Reset (boss_nexusprince_shaffar.cpp:86-97): three initial
    -- beacon SummonCreature(NPC_BEACON, home±8, TEMPSUMMON_CORPSE_TIMED_
    -- DESPAWN 2h) legs — summon bridge live 2026-10-22; the beacon AI
    -- (npc_ethereal_beaconAI) is ported below, so the beacons fight.
    local hx, hy, hz, ho = creature:GetHomePosition()
    creature:SummonCreature(ENTRY_BEACON, hx - 8.0, hy - 8.0, hz, ho, 5, 7200000)
    creature:SummonCreature(ENTRY_BEACON, hx - 8.0, hy + 8.0, hz, ho, 5, 7200000)
    creature:SummonCreature(ENTRY_BEACON, hx + 8.0, hy, hz, ho, 5, 7200000)
end

-- C++ boss_nexusprince_shaffarAI::KilledUnit:
-- if (victim->GetTypeId() == TYPEID_PLAYER) Talk(SAY_SLAY) — the
-- razuvious player-gated precedent (event 3 — the illidan
-- victim:GetObjectType() == "Player" convention).
local function shaffarTargetDied(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ boss_nexusprince_shaffarAI::JustDied: Talk(SAY_DEAD) — the
-- sjonnir self-Talk precedent (event 4).
local function shaffarDied(event, creature, killer)
    creature:Talk(SAY_DEAD)
    local guid = creature:GetGUID()
    cancelTimers(guid)
end

-- C++ boss_nexusprince_shaffarAI::MoveInLineOfSight (:99-105):
-- the _hasTaunted one-shot Talk(SAY_INTRO 0) when a player is
-- within 100.0f; the body never calls the base AI, so the boss
-- never engages by sight in C++. _hasTaunted is ctor-only (set
-- false in the ctor, never touched by Reset), so the Lua latch is
-- never cleared (ossirian precedent). Go's event-27 fire site is a
-- subset of the C++ trigger (dist <= aggro range < 100, who always
-- a player), so both gates hold at fire time. Return true to veto
-- the default engage — the depth_charge precedent — matching C++.
local function shaffarMoveInLOS(event, creature, player)
    local guid = creature:GetGUID()
    if not taunted[guid]
            and creature:IsWithinDistInMap(player, 100.0) then
        taunted[guid] = true
        creature:Talk(SAY_INTRO)
    end
    return true
end

-- C++ boss_nexusprince_shaffarAI::JustSummoned (boss_nexusprince_shaffar.cpp:119-127),
-- beacon case: the visual self-cast is unconditional in C++ (it precedes
-- the target select), so it ports alone; the AttackStart leg is PORTED:
-- random attackable pick + EngageWithTarget (player-target AttackStart twin).
local function shaffarJustSummoned(event, creature, summoned)
    if summoned:GetEntry() ~= ENTRY_BEACON then
        return
    end
    summoned:CastSpell(summoned, SPELL_ETHEREAL_BEACON_VISUAL)
    local target = randomAttackablePlayer(creature)
    if target then
        summoned:EngageWithTarget(target)
    end
end


-- C++ npc_ethereal_beaconAI::JustEngagedWith (:213-221): FindNearestCreature
-- (NPC_SHAFFAR 18344, 100.0f) + shaffar AttackStart(who) when shaffar not
-- in combat, then EVENT_APPRENTICE DUNGEON_MODE(20s, 10s) +
-- EVENT_ARCANE_BOLT 1s. DUNGEON_MODE(normal, heroic) via IsHeroic.
-- C++ npc_ethereal_beaconAI::UpdateAI (:232-251): UpdateVictim gate +
-- casting-state early return (no Lua model, as in all timer ports).
local function beaconBoltTick(creature, guid)
    creature:CastSpell(nil, SPELL_ARCANE_BOLT)
    schedule(guid, "beacon_bolt", math.random(2000, 4500), function()
        beaconBoltTick(creature, guid)
    end)
end

-- C++ EVENT_APPRENTICE (:241-245): triggered DoCast(me,
-- SPELL_ETHEREAL_APPRENTICE 32372) + DespawnOrUnsummon. Ported as a
-- direct summon at the beacon (shaffar EVENT_BEACON precedent): the
-- spell's summon target is the caster, and no C++-visible duration
-- exists, so type 7 manual-despawn (maladaar D'ore precedent). The
-- beacon's ev19 arm engages the apprentice to the beacon's victim
-- (C++ beacon JustSummoned :223).
local function beaconApprenticeTick(creature, guid)
    creature:SummonCreature(ENTRY_APPRENTICE, creature:GetX(), creature:GetY(),
        creature:GetZ(), 0, 7)
    creature:Despawn()
end

local function beaconEnterCombat(event, creature, target)
    local shaffar = creature:FindNearestCreature(ENTRY_SHAFFAR, 100.0)
    if shaffar and not shaffar:IsInCombat() then
        shaffar:EngageWithTarget(target)
    end
    local guid = creature:GetGUID()
    cancelTimers(guid)
    local apprenticeDelay = 20000
    if creature:IsHeroic() then
        apprenticeDelay = 10000
    end
    schedule(guid, "beacon_apprentice", apprenticeDelay, function()
        beaconApprenticeTick(creature, guid)
    end)
    schedule(guid, "beacon_bolt", 1000, function()
        beaconBoltTick(creature, guid)
    end)
end

local function beaconLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function beaconDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function beaconReset(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ npc_ethereal_beaconAI::JustSummoned (:223-226):
-- summoned->AI()->AttackStart(me->GetVictim()).
local function beaconJustSummoned(event, creature, summoned)
    if summoned:GetEntry() ~= ENTRY_APPRENTICE then
        return
    end
    local victim = creature:GetVictim()
    if victim then
        summoned:EngageWithTarget(victim)
    end
end

-- C++ npc_ethereal_apprenticeAI::JustEngagedWith (:288-291) +
-- UpdateAI (:304-318): 3s ping-pong of triggered victim casts
-- (firebolt 32369 / frostbolt 32370). Cyclic pair, so the locals are
-- forward-declared and plain-assigned (tharon_ja/gal_darah idiom) —
-- a later `local function` would shadow the forward slot and every
-- pump stream would call nil (malown-class bug).
local apprenticeFireTick, apprenticeFrostTick
apprenticeFireTick = function(creature, guid)
    creature:CastSpell(nil, SPELL_APPRENTICE_FIREBOLT, true)
    schedule(guid, "appr_frost", 3000, function()
        apprenticeFrostTick(creature, guid)
    end)
end
apprenticeFrostTick = function(creature, guid)
    creature:CastSpell(nil, SPELL_APPRENTICE_FROSTBOLT, true)
    schedule(guid, "appr_fire", 3000, function()
        apprenticeFireTick(creature, guid)
    end)
end

local function apprenticeEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "appr_fire", 3000, function()
        apprenticeFireTick(creature, guid)
    end)
end

local function apprenticeLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function apprenticeDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function apprenticeReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_SHAFFAR, 19, shaffarJustSummoned)
RegisterCreatureEvent(ENTRY_SHAFFAR, 1, shaffarEnterCombat)
RegisterCreatureEvent(ENTRY_SHAFFAR, 2, shaffarLeaveCombat)
RegisterCreatureEvent(ENTRY_SHAFFAR, 3, shaffarTargetDied)
RegisterCreatureEvent(ENTRY_SHAFFAR, 4, shaffarDied)
RegisterCreatureEvent(ENTRY_SHAFFAR, 23, shaffarReset)
RegisterCreatureEvent(ENTRY_SHAFFAR, 27, shaffarMoveInLOS)
RegisterCreatureEvent(ENTRY_BEACON, 19, beaconJustSummoned)
RegisterCreatureEvent(ENTRY_BEACON, 1, beaconEnterCombat)
RegisterCreatureEvent(ENTRY_BEACON, 2, beaconLeaveCombat)
RegisterCreatureEvent(ENTRY_BEACON, 4, beaconDied)
RegisterCreatureEvent(ENTRY_BEACON, 23, beaconReset)
RegisterCreatureEvent(ENTRY_APPRENTICE, 1, apprenticeEnterCombat)
RegisterCreatureEvent(ENTRY_APPRENTICE, 2, apprenticeLeaveCombat)
RegisterCreatureEvent(ENTRY_APPRENTICE, 4, apprenticeDied)
RegisterCreatureEvent(ENTRY_APPRENTICE, 23, apprenticeReset)
