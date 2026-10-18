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
-- precedent; EVENT_BEACON stays out — triggered summon cast, see
-- below).
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
-- shaffarMoveInLOS on event 27 (per-GUID latch, latch cleared on
-- 2/4/23 — C++ Reset() :83 re-latches); returns true to veto the
-- default aggro engage, matching the C++ body which never calls
-- the base (depth_charge precedent).
-- DOCUMENTED-ONLY (in this header; only entry 18344 registered):
-- boss_nexusprince_shaffarAI::Reset — the three initial beacon
-- SummonCreature(NPC_BEACON 18431, TEMPSUMMON_CORPSE_TIMED_DESPAWN
-- 2h) legs — no-summon bridge (shirrak precedent).
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
-- boss_nexusprince_shaffarAI::JustSummoned — beacon
-- CastSpell(SPELL_ETHEREAL_BEACON_VISUAL 32368) + Random-SelectTarget
-- AttackStart legs — no-cast / no-random-target / no-cross-creature
-- bridges (the JustSummoned summon-coordination leg is the maladaar
-- precedent) — documented-only.
-- boss_nexusprince_shaffarAI::ExecuteEvent:
-- EVENT_BEACON — urand(0,3)==0-gated Talk(SAY_SUMMON 3) + DoCast(me,
-- SPELL_ETHEREAL_BEACON 32371, triggered), 10s repeat — stays OUT:
-- the cast is a triggered summon whose whole purpose is the beacon
-- summon machine (no-summon bridge — shirrak precedent);
-- EVENT_BLINK — interrupt + Clear(MOTION_PRIORITY_NORMAL) +
-- DoCast(SPELL_BLINK 34605) — stays OUT: the interrupt/motion legs
-- have no bridge (the blink teleport effect has no lua-bridge model);
-- EVENT_FIREBALL / EVENT_FROSTBOLT / EVENT_FROST_NOVA ported above.
-- npc_ethereal_beacon (18431) — zero Talk; NOT registered (no Lua
-- file for 18431/18430/yor): re-keyed 2026-10-15 (timer doctrine) —
-- the timer and cast legs bridge (CreateLuaEvent / CastSpell are
-- live), but the machines stay out on the real legs: the
-- JustEngagedWith cross-creature leg (FindNearestCreature(NPC_SHAFFAR
-- 18344, 100f) + AttackStart(who) when shaffar not in combat) has no
-- cross-creature bridge; EVENT_APPRENTICE (DUNGEON_MODE 20s/10s —
-- DoCast(SPELL_ETHEREAL_APPRENTICE 32372, triggered, summons 18430))
-- rides the missing summon bridge (the old "no despawn bridge" was
-- stale — creature:Despawn() is bridged, lua_creature_events.go:214;
-- the summon bridge is the true blocker); EVENT_ARCANE_BOLT (1s init
-- -> 2-4.5s loop,
-- non-triggered victim cast) would bridge but has no registered
-- creature to host it. The stale "no-timer / no-cast" claim is
-- corrected — the absence of registrations is the true blocker.
-- npc_ethereal_apprentice (18430) — zero Talk; NOT registered:
-- the alternating EVENT_ETHEREAL_APPRENTICE_FIREBOLT (SPELL_FIREBOLT
-- 32369, triggered) / EVENT_ETHEREAL_APPRENTICE_FROSTBOLT (SPELL_
-- FROSTBOLT 32370, triggered) 3s-ping-pong victim-cast machine —
-- timer + triggered-cast legs would bridge (CastSpell(target, spell,
-- true)), but with no registered Lua file the whole class stays out.
-- The stale "no-timer / no-cast" claim is corrected.
-- npc_yor — zero Talk; NOT registered: the EVENT_DOUBLE_BREATH
-- 6-9s machine (IsWithinDist(victim, ATTACK_DISTANCE)-gated
-- non-triggered DoCastVictim(SPELL_DOUBLE_BREATH 38361) +
-- DoMeleeAttackIfReady) — timer + cast legs would bridge, but the
-- melee-range distance gate has no bridge (alar precedent) and no
-- registered Lua file hosts the class. The stale "no-timer /
-- no-cast" claim is corrected.
-- All Talk() calls in the file accounted for (3 ported above +
-- SAY_INTRO 0 MoveInLineOfSight + SAY_SUMMON 3 EVENT_BEACON machine
-- documented above).

local ENTRY_SHAFFAR = 18344 -- NPC_SHAFFAR (Creatures enum)

local SAY_INTRO = 0
local SAY_AGGRO = 1
local SAY_SLAY  = 2
local SAY_DEAD  = 4

local SPELL_FROSTBOLT = 32364 -- boss_nexusprince_shaffar.cpp:42
local SPELL_FROSTNOVA = 32365 -- boss_nexusprince_shaffar.cpp:44

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

-- C++ boss_nexusprince_shaffarAI::JustEngagedWith (:108-117):
-- Talk(SAY_AGGRO); BossAI::JustEngagedWith(who) — the auriaya
-- precedent (the BossAI legs have no bridge) — then the four
-- ScheduleEvent legs: EVENT_BEACON 10s / EVENT_FIREBALL 8s /
-- EVENT_FROSTBOLT 4s / EVENT_FROST_NOVA 15s. BEACON stays out: the
-- arm is a triggered summon cast (DoCast(me, SPELL_ETHEREAL_BEACON
-- 32371, true)) whose whole purpose is the beacon summon machine
-- (no-summon bridge — shirrak precedent).
local function shaffarEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
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
    taunted[guid] = nil
end

local function shaffarReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    taunted[guid] = nil
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
    taunted[guid] = nil
end

-- C++ boss_nexusprince_shaffarAI::MoveInLineOfSight (:99-105):
-- the _hasTaunted one-shot Talk(SAY_INTRO 0) when a player is
-- within 100.0f; the body never calls the base AI, so the boss
-- never engages by sight in C++. Go's event-27 fire site is a
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

RegisterCreatureEvent(ENTRY_SHAFFAR, 1, shaffarEnterCombat)
RegisterCreatureEvent(ENTRY_SHAFFAR, 2, shaffarLeaveCombat)
RegisterCreatureEvent(ENTRY_SHAFFAR, 3, shaffarTargetDied)
RegisterCreatureEvent(ENTRY_SHAFFAR, 4, shaffarDied)
RegisterCreatureEvent(ENTRY_SHAFFAR, 23, shaffarReset)
RegisterCreatureEvent(ENTRY_SHAFFAR, 27, shaffarMoveInLOS)
