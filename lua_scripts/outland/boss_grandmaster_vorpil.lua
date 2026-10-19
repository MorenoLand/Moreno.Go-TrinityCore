-- Grandmaster Vorpil (Shadow Labyrinth, Auchindoun) — Lua port of
-- src/server/scripts/Outland/Auchindoun/ShadowLabyrinth/boss_grandmaster_vorpil.cpp
-- (2 CreatureScripts — boss_grandmaster_vorpil (BossAI via
-- GetShadowLabyrinthAI, DATA_GRANDMASTER_VORPIL = 2) + npc_voidtraveler
-- (ScriptedAI); both registered from inside
-- AddSC_boss_grandmaster_vorpil() (lines 296-300); loader decl 38 /
-- call 161 per outland_script_loader.cpp — the THIRD group of the
-- "// Auchindoun - Shadow Labyrinth" sub-block in AddOutlandScripts(),
-- immediately after AddSC_boss_blackheart_the_inciter() (call 160) —
-- verified from the loader this run; the checkpoint sequence
-- (blackheart_the_inciter -> grandmaster_vorpil) is followed).
-- Entry: 18732 NPC_GRANDMASTER_VORPIL — from shadow_labyrinth.h's
-- Creatures enum (line 50); the .cpp file's own enum carries only the
-- companion entries NPC_VOID_TRAVELER = 19226 / NPC_VOID_PORTAL =
-- 19224 — the boss entry is verifiable from the C++ tree
-- (NPC_ANZU-in-sethekk_halls.h / ambassador_hellmaw precedents).
-- ScriptName bindings are DB-side as usual. Sole-source verified:
-- whole-server-tree grep for "AddSC_boss_grandmaster_vorpil" hits
-- boss_grandmaster_vorpil.cpp (+ the loader decl/call lines) only.
-- No grandmaster_vorpil lua existed.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied. Timers ride the CreateLuaEvent convention
-- (timers table keyed by GUID, cancel on 2/4 — the gargolmar
-- precedent); melee is engine-driven in Go (creature combat tick).
-- Ported arms (C++-exact for all modeled arms):
-- boss_grandmaster_vorpilAI::JustEngagedWith — Talk(SAY_AGGRO 1)
-- (line 149; event 1 — the auriaya precedent) + SummonPortals() +
-- PORTED timer arm: EVENT_SHADOWBOLT_VOLLEY — DoCast(me,
-- SPELL_SHADOWBOLT_VOLLEY 33841) non-triggered self-cast,
-- randtime(7s,14s) init / randtime(15s,30s) re-arm (JustEngagedWith
-- :149 / switch :178-181); the arm is ungated (UpdateVictim +
-- UNIT_STATE_CASTING only) and 33841 has no SpellScript — the old
-- "no-timer" claim was wrong. (The 6s EVENT_SHADOWBOLT_VOLLEY
-- reschedule inside the unported EVENT_DRAW_SHADOWS leg :203 is
-- unreachable while draw_shadows stays out — documented below.)
-- boss_grandmaster_vorpilAI::KilledUnit — Talk(SAY_SLAY 3) (line
-- 131) gated on who->GetTypeId() == TYPEID_PLAYER — the razuvious
-- player-gated precedent (event 3).
-- boss_grandmaster_vorpilAI::JustDied — Talk(SAY_DEATH 4) (line
-- 137) — the sjonnir self-Talk precedent (event 4; the _JustDied()
-- instance leg rides the no-instance bridge — documented-only).
-- boss_grandmaster_vorpilAI::MoveInLineOfSight — the _intro
-- one-shot Talk(SAY_INTRO 0) (line 159) — PORTED as
-- vorpilMoveInLOS on event 27 (per-GUID _intro latch, latch
-- cleared on 4 (death = respawn = fresh ctor state) only; C++
-- Reset() calls Initialize() which re-latches _helpYell only —
-- _intro is ctor-set and never re-armed on evade);
-- returns nil so the default aggro engage proceeds, matching
-- the C++ BossAI::MoveInLineOfSight(who) call at :155.
-- DOCUMENTED-ONLY (in this header; only entry 18732 registered):
-- boss_grandmaster_vorpilAI::Reset — _Reset() + Initialize()
-- (_helpYell only; _intro is ctor-set and survives evade) —
-- no-instance bridge — documented-only.
-- boss_grandmaster_vorpilAI::spawnVoidTraveler — SummonCreature
-- (NPC_VOID_TRAVELER 19226, random VoidPortalCoords) + the one-shot
-- _helpYell Talk(SAY_HELP 2) (line 123) — rides the
-- EVENT_SUMMON_TRAVELER timer machine — no SummonCreature
-- binding — documented-only (the timer leg bridges; the summon
-- leg doesn't).
-- boss_grandmaster_vorpilAI::SummonPortals — 5x SummonCreature
-- (NPC_VOID_PORTAL 19224 at VoidPortalCoords) + SPELL_VOID_PORTAL_VISUAL
-- 33569 visual cast — no-summon / no-cast bridges —
-- documented-only.
-- boss_grandmaster_vorpilAI::UpdateAI — the remaining timer
-- machine (EVENT_SHADOWBOLT_VOLLEY + heroic-only EVENT_BANISH
-- ported above) stays out on evidence-based legs, not the old
-- blanket "no-timer" claim:
-- heroic-only EVENT_BANISH (SelectTarget random 30f → DoCast(
-- target, SPELL_BANISH 38791), 15s cycle) — PORTED as
-- banishTick, the scheduler seeded only when creature:IsHeroic()
-- (the earlier "no heroic bridge" claim was stale — IsHeroic is
-- bridged, engine/world/lua_creature_events.go:260; the lavanthor
-- precedent). Documented deviations: the C++ SelectTarget defaults
-- playerOnly=FALSE (EXPLICIT false here), so the pool can include
-- pets/totems — the threat-list pick via GetAITargets carries that
-- (no Player filter); the UNIT_STATE_CASTING pre-check has no bridge —
-- casts fire unconditionally on their timers (the hodir precedent);
-- EVENT_DRAW_SHADOWS (teleports every alive
-- non-banished player to VorpilPosition + me->UpdatePosition() —
-- no Teleport/UpdatePosition bridge in engine/scripting/object.go and
-- the teleport IS the mechanic; DoCast(me, SPELL_DRAW_SHADOWS 33563,
-- triggered) + DoCast(me, SPELL_RAIN_OF_FIRE 33617) —
-- H_SPELL_RAIN_OF_FIRE 39363 is a dead constant, no IsHeroic branch
-- exists in this arm — 45s init / 30s cycle; this leg also
-- reschedules EVENT_SHADOWBOLT_VOLLEY at 6s (:198), unreachable while
-- it stays out); EVENT_SUMMON_TRAVELER (spawnVoidTraveler →
-- SummonCreature(NPC_VOID_TRAVELER 19226, TEMPSUMMON_CORPSE_TIMED_DESPAWN,
-- 5s) — no Summon binding; effective first traveler ~5s
-- (SummonPortals() schedules EVENT_SUMMON_TRAVELER at 5s —
-- ScheduleEvent is a plain multimap insert, so the 5s and 90s events
-- both pend), 10s cycle with an ADDITIONAL 5s event scheduled below
-- 20% health (not a 5s replacement); the SAY_HELP (2) one-shot
-- _helpYell latch rides the same machine); DoMeleeAttackIfReady is
-- engine-driven in Go.
-- npc_voidtraveler (NPC_VOID_TRAVELER = 19226) — zero Talk calls;
-- the UpdateAI follow/sacrifice machine: Vorpil lookup via
-- _instance->GetGuidData(DATA_GRANDMASTER_VORPIL) (no-instance
-- bridge), MoveFollow (no motion bridge), DoCast(me,
-- SPELL_SACRIFICE 33587, false) at 3 yards, then on _sacrificed:
-- DoCastAOE(SPELL_EMPOWERING_SHADOWS 33783, true) + DoCast(me,
-- SPELL_SHADOW_NOVA 33846, true) + me->KillSelf() (no KillSelf
-- binding). The casts bridge (self-cast convention) but the follow /
-- instance-lookup / kill legs don't — documented-only (NOT
-- registered; shaffar-beacon precedent).
-- All Talk() calls in the file accounted for (4 ported above —
-- aggro/slay/death/intro; SAY_HELP 2 rides the traveler timer
-- machine — documented-only).

local ENTRY_GRANDMASTER_VORPIL = 18732 -- NPC_GRANDMASTER_VORPIL (shadow_labyrinth.h)

local SAY_INTRO = 0
local SAY_AGGRO = 1
local SAY_SLAY  = 3
local SAY_DEATH = 4

local SPELL_SHADOWBOLT_VOLLEY = 33841
local SPELL_BANISH = 38791

local timers = {}
local introSeen = {} -- per-GUID _intro latch (C++ _intro, :159)

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

-- C++ EVENT_SHADOWBOLT_VOLLEY: DoCast(me, SHADOWBOLT_VOLLEY 33841)
-- non-triggered self-cast, randtime(7s,14s) init / randtime(15s,30s)
-- re-arm.
local function shadowboltVolleyTick(creature, guid)
    creature:CastSpell(creature, SPELL_SHADOWBOLT_VOLLEY)
    schedule(guid, "volley", {15000, 30000}, function()
        shadowboltVolleyTick(creature, guid)
    end)
end

-- C++ SelectTarget(SelectTargetMethod::Random, 0, 30.0f, false) —
-- playerOnly=FALSE EXPLICIT — threat-list pick within 30 yd (3D, the
-- C++ DefaultTargetSelector dist semantics); no Player filter, so
-- pets/totems are eligible. The bridged, threat-ordered GetAITargets
-- list (lua_creature_events.go:345); the murmur
-- randomThreatTarget(creature, 30, false) precedent.
local function randomThreatTarget(creature, minDist, maxDist)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        local dist = creature:GetDistance(t)
        if dist >= minDist and dist <= maxDist then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EVENT_BANISH (:182-186): SelectTarget(Random, 0, 30.0f, false)
-- -> DoCast(target, SPELL_BANISH 38791), fixed 15s re-arm; the
-- scheduler seeds only when IsHeroic() (:144-145). DoCast 1-arg
-- form is non-triggered, so creature:CastSpell(target, spell).
local function banishTick(creature, guid)
    local target = randomThreatTarget(creature, 0, 30)
    if target then
        creature:CastSpell(target, SPELL_BANISH)
    end
    schedule(guid, "banish", 15000, function()
        banishTick(creature, guid)
    end)
end

-- C++ boss_grandmaster_vorpilAI::JustEngagedWith:
-- events.ScheduleEvent(EVENT_SHADOWBOLT_VOLLEY, 7s, 14s);
-- if (IsHeroic()) events.ScheduleEvent(EVENT_BANISH, 15s);
-- events.ScheduleEvent(EVENT_DRAW_SHADOWS, 45s);
-- events.ScheduleEvent(EVENT_SUMMON_TRAVELER, 90s);
-- Talk(SAY_AGGRO); SummonPortals(); — the auriaya precedent
-- (event 1; the EVENT_SHADOWBOLT_VOLLEY leg is ported above and the
-- heroic-only EVENT_BANISH leg is ported as banishTick seeded under
-- creature:IsHeroic(); the remaining ScheduleEvent legs
-- (draw_shadows / summon_traveler) + SummonPortals() ride the
-- documented-only bridges — see the header).
local function vorpilEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "volley", {7000, 14000}, function()
        shadowboltVolleyTick(creature, guid)
    end)
    -- C++ :144-145: the EVENT_BANISH scheduler seeds only when
    -- IsHeroic() (the earlier "no heroic bridge" claim was stale —
    -- creature:IsHeroic() is bridged,
    -- engine/world/lua_creature_events.go:260; the lavanthor
    -- precedent).
    if creature:IsHeroic() then
        schedule(guid, "banish", 15000, function()
            banishTick(creature, guid)
        end)
    end
end

local function vorpilLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
end

-- C++ boss_grandmaster_vorpilAI::KilledUnit:
-- if (who->GetTypeId() == TYPEID_PLAYER) Talk(SAY_SLAY) — the
-- razuvious player-gated precedent (event 3 — the illidan
-- victim:GetObjectType() == "Player" convention).
local function vorpilTargetDied(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ boss_grandmaster_vorpilAI::JustDied:
-- _JustDied(); Talk(SAY_DEATH); — the sjonnir self-Talk precedent
-- (event 4; the _JustDied() instance leg rides the no-instance
-- bridge — documented-only).
local function vorpilDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    introSeen[guid] = nil
end

-- C++ boss_grandmaster_vorpilAI::Reset -> Initialize() (:102-105) re-arms
-- _intro = false: after an evade the intro Talk replays. Go fires event 23
-- on evade (before event 2) and on the death path (before event 4), so the
-- wipe lives here with the IsDead guard (overgrind precedent).
local function vorpilReset(event, creature)
    if creature:IsDead() then
        return
    end
    local guid = creature:GetGUID()
    cancelTimers(guid)
    introSeen[guid] = nil
end

-- C++ boss_grandmaster_vorpilAI::MoveInLineOfSight (:155-161):
-- BossAI::MoveInLineOfSight(who) first, then the _intro one-shot
-- Talk(SAY_INTRO 0) when the player is within 100.0f + LOS +
-- valid-attack-target. Go's event-27 fire site (aggro-engage
-- branch) is a subset of the C++ trigger: dist <= aggro range at
-- fire time makes the 100.0f gate vacuous (kept for shape
-- fidelity), LOS and the attack-target gates already hold. Return
-- nil so the engage proceeds — C++ calls the base, the
-- depth_charge return-true precedent does not apply.
local function vorpilMoveInLOS(event, creature, player)
    local guid = creature:GetGUID()
    if not introSeen[guid]
            and creature:IsWithinDistInMap(player, 100.0) then
        introSeen[guid] = true
        creature:Talk(SAY_INTRO)
    end
end

RegisterCreatureEvent(ENTRY_GRANDMASTER_VORPIL, 1, vorpilEnterCombat)
RegisterCreatureEvent(ENTRY_GRANDMASTER_VORPIL, 2, vorpilLeaveCombat)
RegisterCreatureEvent(ENTRY_GRANDMASTER_VORPIL, 3, vorpilTargetDied)
RegisterCreatureEvent(ENTRY_GRANDMASTER_VORPIL, 4, vorpilDied)
RegisterCreatureEvent(ENTRY_GRANDMASTER_VORPIL, 23, vorpilReset)
RegisterCreatureEvent(ENTRY_GRANDMASTER_VORPIL, 27, vorpilMoveInLOS)
