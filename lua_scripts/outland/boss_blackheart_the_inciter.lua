-- Blackheart the Inciter (Shadow Labyrinth, Auchindoun) — Lua port of
-- src/server/scripts/Outland/Auchindoun/ShadowLabyrinth/boss_blackheart_the_inciter.cpp
-- (1 CreatureScript — boss_blackheart_the_inciter (BossAI via
-- GetShadowLabyrinthAI, DATA_BLACKHEART_THE_INCITER = 1); registered
-- from inside AddSC_boss_blackheart_the_inciter(); loader decl 37 /
-- call 160 per outland_script_loader.cpp — the SECOND group of the
-- "// Auchindoun - Shadow Labyrinth" sub-block in AddOutlandScripts(),
-- immediately after AddSC_boss_ambassador_hellmaw() (call 159) —
-- verified from the loader this run; the checkpoint sequence
-- (ambassador_hellmaw -> blackheart_the_inciter) is followed).
-- Entry: 18667 NPC_BLACKHEART — from shadow_labyrinth.h's Creatures
-- enum (line 44); the .cpp file itself carries zero NPC_ constants
-- (the boss entry is verifiable from the C++ tree —
-- NPC_ANZU-in-sethekk_halls.h / ambassador_hellmaw precedents).
-- ScriptName bindings are DB-side as usual. Sole-source verified:
-- whole-server-tree grep for "AddSC_boss_blackheart_the_inciter"
-- hits boss_blackheart_the_inciter.cpp (+ the loader decl/call
-- lines) only. No blackheart_the_inciter lua existed.
-- Eluna creature events: 1 OnEnterCombat, 3 OnTargetDied, 4 OnDied.
-- Ported arms (C++-exact for all modeled arms):
-- boss_blackheart_the_inciter::JustEngagedWith — Talk(SAY_AGGRO 1)
-- (line 89; event 1 — the auriaya precedent) + schedules
-- EVENT_WAR_STOMP (15s) + EVENT_CHARGE_ATTACK (5s, one-shot).
-- EVENT_WAR_STOMP — DoCast(me, SPELL_WAR_STOMP 33707) 2-arg
-- (non-triggered), ungated, 15s init, randtime(18s,24s) repeat
-- (the Oct-12 "no-timer bridge" claim was wrong — CreateLuaEvent is a
-- real bridge, retracted this run). All timers cancelled on 2/4/23
-- (gargolmar precedent).
-- EVENT_CHARGE_ATTACK — DoCast(target, SPELL_CHARGE 33709) 2-arg
-- (non-triggered), ungated, SelectTarget(Random, 0) offset-0 pool,
-- 5s init, ONE-SHOT (the C++ 15-25s re-arm is a verified no-op —
-- EVENT_CHARGE=1003 unhandled by the UpdateAI switch) — ported on a
-- fire-once CreateLuaEvent from event 1.
-- boss_blackheart_the_inciter::KilledUnit — Talk(SAY_SLAY 2) (line
-- 95) gated on who->GetTypeId() == TYPEID_PLAYER — the razuvious
-- player-gated precedent (event 3).
-- boss_blackheart_the_inciter::JustDied — Talk(SAY_DEATH 4) (line
-- 101) — the sjonnir self-Talk precedent (event 4; the _JustDied()
-- instance leg rides the no-instance bridge — documented-only).
-- DOCUMENTED-ONLY (in this header; only entry 18667 registered):
-- boss_blackheart_the_inciter::Reset — me->SetReactState
-- (REACT_AGGRESSIVE) is PORTED below on event 23 (the Oct-18
-- "no-react-state bridge" note was stale — the bridge is live,
-- lua_creature_events.go); the _Reset() instance leg stays
-- documented-only (no instance bridge).
-- boss_blackheart_the_inciter::SetData — charm-count machine (++
-- on charmed, sanity-check EnterEvadeMode(EVADE_REASON_OTHER) when
-- -- on zero, REACT_PASSIVE while any charmed player) driven from
-- BlackheartCharmedPlayerAI::OnCharmed — no charmed-player-actor /
-- no-instance bridges — documented-only.
-- boss_blackheart_the_inciter::UpdateAI — the timer machine:
-- EVENT_INCITE_CHAOS (threat-list-size > 1 gate → ResetThreatList
-- + DoCast(me, SPELL_INCITE_CHAOS 33676), 40s cycle) stays OUT — the
-- whole effect is SpellScript-delivered (spell_blackheart_incite_
-- chaos HandleDummy cycles INCITE_SPELLS {33677, 33680, 33681, 33682,
-- 33683} on hit units); a bare cast would fabricate a no-op cast bar
-- (shazzrah precedent);
-- EVENT_CHARGE_ATTACK — PORTED above: SelectTarget(Random, 0)
-- → DoCast(target, SPELL_CHARGE 33709) (2-arg, non-triggered,
-- offset 0 — tank INCLUDED, threat-list pool via GetAITargets
-- (black-stalker idiom), 5s init,
-- ONE-SHOT. C++ BUG (verified): the arm re-arms EVENT_CHARGE (line
-- 147), but the enum defines only EVENT_CHARGE_ATTACK —
-- EVENT_CHARGE resolves to SharedDefines.h:3342 (id 1003,
-- MotionMaster generic; MotionMaster.h:173 default arg) which the
-- UpdateAI switch (cases 1/2/3 only) never handles — the re-arm is
-- a verified no-op, so charge fires ONCE at 5s and never re-arms
-- (the "15-25s cycle" does not exist at runtime);
-- EVENT_WAR_STOMP ported above; DoMeleeAttackIfReady —
-- documented-only.
-- BlackheartCharmedPlayerAI (SimpleCharmedPlayerAI) —
-- OnCharmed: instance GetGuidData(DATA_BLACKHEART_THE_INCITER) →
-- blackheart->AI()->SetData(0, charmed) + AddThreat — no
-- charmed-player / no-instance bridges — documented-only.
-- boss_blackheart_the_inciter_mc_dummy (NullCreatureAI;
-- NPC_BLACKHEART_DUMMY1..5 = 19300-19304, shadow_labyrinth.h lines
-- 45-49) — IsSummonedBy casts SPELL_INCITE_CHAOS_B 33684 on the
-- summoner + dummy-threat juggling across all dummies and their
-- m_Controlled; DespawnOrUnsummon when m_Controlled empties;
-- GetAIForCharmedPlayer returns BlackheartCharmedPlayerAI —
-- zero Talk arms anywhere in the dummy script; no-summon /
-- no-threat / no-cast bridges — documented-only (dummies NOT
-- registered).
-- spell_blackheart_incite_chaos SpellScript — HandleDummy cycles
-- INCITE_SPELLS {33677, 33680, 33681, 33682, 33683} on hit units
-- (EFFECT_0 SPELL_EFFECT_SCRIPT_EFFECT) — no SpellScript bridge
-- (instance_ulduar precedent) — documented-only.
-- All Talk() calls in the file accounted for (3 ported above;
-- SAY_INTRO 0 / SAY_HELP 3 / SAY2_* 5-9 are never called anywhere
-- in the file).

local ENTRY_BLACKHEART_THE_INCITER = 18667 -- NPC_BLACKHEART (shadow_labyrinth.h)

local SAY_AGGRO = 1
local SAY_SLAY  = 2
local SAY_DEATH = 4

local SPELL_WAR_STOMP = 33707 -- boss_blackheart_the_inciter.cpp:34
local SPELL_CHARGE = 33709 -- boss_blackheart_the_inciter.cpp:33

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

-- C++ SelectTarget(Random, 0) (UnitAI.h:170 defaults
-- playerOnly=false, withTank=true, dist=0, aura=0): random alive
-- unit from the boss's threat list, tank included, no distance
-- gate (black-stalker idiom; the old map-player pool could pick
-- never-engaged instance players and missed non-player threat
-- units).
local function pickThreatTarget(creature)
    local found = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if not t:IsDead() then
            found[#found + 1] = t
        end
    end
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
end

-- C++ EVENT_WAR_STOMP (:149-151): DoCast(me, SPELL_WAR_STOMP 33707)
-- 2-arg (non-triggered), ungated; 15s init, randtime(18s,24s) repeat
-- (millisecond granularity — the {min,max} table form, not
-- math.random(18,24)*1000).
local function warStompTick(creature, guid)
    creature:CastSpell(creature, SPELL_WAR_STOMP)
    schedule(guid, "war_stomp", {18000, 24000}, function()
        warStompTick(creature, guid)
    end)
end

-- C++ EVENT_CHARGE_ATTACK (:144-147): if (Unit* target =
-- SelectTarget(SelectTargetMethod::Random, 0)) DoCast(target,
-- SPELL_CHARGE 33709) (2-arg, non-triggered), 5s init, ONE-SHOT —
-- the arm's re-arm schedules EVENT_CHARGE (SharedDefines.h:3342,
-- id 1003) which the UpdateAI switch never handles (verified
-- no-op). Threat-list pool via GetAITargets (see above).
local function blackheartChargeTick(creature, guid)
    local target = pickThreatTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_CHARGE)
    end
end

-- C++ boss_blackheart_the_inciter::JustEngagedWith (:85-87):
-- events.ScheduleEvent(EVENT_INCITE_CHAOS, 20s);
-- events.ScheduleEvent(EVENT_CHARGE_ATTACK, 5s);
-- events.ScheduleEvent(EVENT_WAR_STOMP, 15s);
-- Talk(SAY_AGGRO); — the auriaya precedent (event 1). Only WAR_STOMP
-- and CHARGE_ATTACK port: INCITE_CHAOS is SpellScript-delivered (spell_
-- blackheart_incite_chaos HandleDummy cycles 33677/33680/33681/33682/
-- 33683 on hit units — a bare cast would be a no-op cast bar, shazzrah
-- precedent) and its threat-list-size>1 gate + ResetThreatList ride
-- the same unbridged machine; CHARGE_ATTACK is PORTED below as a
-- fire-once one-shot (its C++ 15-25s re-arm is a verified no-op).
local function blackheartEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "war_stomp", 15000, function()
        warStompTick(creature, guid)
    end)
    -- C++ JustEngagedWith: events.ScheduleEvent(EVENT_CHARGE_ATTACK,
    -- 5s) — one-shot (the re-arm is a verified C++ no-op, see above).
    schedule(guid, "charge", 5000, function()
        blackheartChargeTick(creature, guid)
    end)
end

-- Events 2 (OnLeaveCombat): cancelTimers only.
local function blackheartLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ boss_blackheart_the_inciter::Reset (lines 76-80):
-- me->SetReactState(REACT_AGGRESSIVE) (bridged — lua_creature_events.go;
-- REACT_AGGRESSIVE = 2, UnitDefines.h) + _Reset() (no instance bridge —
-- documented-only). cancelTimers mirrors _Reset()'s events.Reset().
local function blackheartReset(event, creature)
    cancelTimers(creature:GetGUID())
    creature:SetReactState(2)
end

-- C++ boss_blackheart_the_inciter::KilledUnit:
-- if (who->GetTypeId() == TYPEID_PLAYER) Talk(SAY_SLAY) — the
-- razuvious player-gated precedent (event 3 — the illidan
-- victim:GetObjectType() == "Player" convention).
local function blackheartTargetDied(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ boss_blackheart_the_inciter::JustDied:
-- _JustDied(); Talk(SAY_DEATH); — the sjonnir self-Talk precedent
-- (event 4; the _JustDied() instance leg rides the no-instance
-- bridge — documented-only).
local function blackheartDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_BLACKHEART_THE_INCITER, 1, blackheartEnterCombat)
RegisterCreatureEvent(ENTRY_BLACKHEART_THE_INCITER, 2, blackheartLeaveCombat)
RegisterCreatureEvent(ENTRY_BLACKHEART_THE_INCITER, 3, blackheartTargetDied)
RegisterCreatureEvent(ENTRY_BLACKHEART_THE_INCITER, 4, blackheartDied)
RegisterCreatureEvent(ENTRY_BLACKHEART_THE_INCITER, 23, blackheartReset)
