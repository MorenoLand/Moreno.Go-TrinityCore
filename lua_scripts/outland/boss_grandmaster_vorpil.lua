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
-- reschedule inside EVENT_DRAW_SHADOWS :198 is a multimap add —
-- see drawShadowsTick.)
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
-- boss_grandmaster_vorpilAI::spawnVoidTraveler — PORTED as travelerTick
-- on the live SummonCreature bridge (lua_creature_events.go:877):
-- SummonCreature(NPC_VOID_TRAVELER 19226, random VoidPortalCoords)
-- (TEMPSUMMON_CORPSE_TIMED_DESPAWN, 5s — Lua type 5) + the one-shot
-- _helpYell Talk(SAY_HELP 2) latch; 10s cycle with an ADDITIONAL 5s
-- event below 20% health (ScheduleEvent is a plain multimap insert —
-- the 5s and 90s seeds both pend).
-- boss_grandmaster_vorpilAI::SummonPortals — PORTED on the live
-- SummonCreature bridge: 5x SummonCreature(NPC_VOID_PORTAL 19224 at
-- VoidPortalCoords) (TEMPSUMMON_CORPSE_DESPAWN — Lua type 4) +
-- triggered SPELL_VOID_PORTAL_VISUAL 33569 self-cast each (DoCastAOE
-- self-cast convention).
-- boss_grandmaster_vorpilAI::UpdateAI — the remaining timer
-- machine (EVENT_SHADOWBOLT_VOLLEY + heroic-only EVENT_BANISH
-- ported above) stays out on evidence-based legs, not the old
-- blanket "no-timer" claim:
-- heroic-only EVENT_BANISH (SelectTarget random 30f → DoCast(
-- target, SPELL_BANISH 38791), 15s cycle) — PORTED as
-- banishTick, the scheduler seeded only when creature:IsHeroic()
-- (the earlier "no heroic bridge" claim was stale — IsHeroic is
-- bridged, engine/world/lua_creature_events.go:409-417; the lavanthor
-- precedent). Documented deviations: the C++ SelectTarget defaults
-- playerOnly=FALSE (EXPLICIT false here), so the pool can include
-- pets/totems — the threat-list pick via GetAITargets carries that
-- (no Player filter); the UNIT_STATE_CASTING pre-check has no bridge —
-- casts fire unconditionally on their timers (the hodir precedent);
-- EVENT_DRAW_SHADOWS — PORTED as drawShadowsTick (45s init / 30s
-- cycle; player teleports via player:TeleportNear, boss reposition via
-- creature:Teleport, triggered draw-shadows + rain-of-fire casts, 6s
-- volley reschedule — see the port note above JustEngagedWith);
-- EVENT_SUMMON_TRAVELER — PORTED as travelerTick (see the port note
-- above JustEngagedWith): SummonCreature(NPC_VOID_TRAVELER 19226,
-- TEMPSUMMON_CORPSE_TIMED_DESPAWN, 5s); effective first traveler ~5s
-- (SummonPortals() schedules EVENT_SUMMON_TRAVELER at 5s —
-- ScheduleEvent is a plain multimap insert, so the 5s and 90s events
-- both pend), 10s cycle with an ADDITIONAL 5s event scheduled below
-- 20% health (not a 5s replacement); the SAY_HELP (2) one-shot
-- _helpYell latch rides the same machine (re-armed on Reset via
-- Initialize, the ctor/Reset note above); DoMeleeAttackIfReady is
-- engine-driven in Go.
-- npc_voidtraveler (NPC_VOID_TRAVELER = 19226) — zero Talk calls;
-- the UpdateAI follow/sacrifice machine: Vorpil lookup via
-- _instance->GetGuidData(DATA_GRANDMASTER_VORPIL) (no-instance
-- bridge), MoveFollow (no motion bridge), DoCast(me,
-- SPELL_SACRIFICE 33587, false) at 3 yards, then on _sacrificed:
-- DoCastAOE(SPELL_EMPOWERING_SHADOWS 33783, true) + DoCast(me,
-- SPELL_SHADOW_NOVA 33846, true) + me->KillSelf() (creature:Kill()
-- is the self-kill bridge, kj fiend precedent — but the kill leg
-- stays out with the rest: the casts bridge (self-cast convention)
-- but the follow / instance-lookup legs don't, and the file is
-- documented-only (NOT registered; shaffar-beacon precedent).
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
local SPELL_DRAW_SHADOWS = 33563
local SPELL_RAIN_OF_FIRE = 33617
local SPELL_VOID_PORTAL_VISUAL = 33569

local ENTRY_VOID_PORTAL = 19224
local ENTRY_VOID_TRAVELER = 19226
local SAY_HELP = 2

-- C++ VoidPortalCoords[5] (boss_grandmaster_vorpil.cpp:44-51).
local VOID_PORTAL_COORDS = {
    {-283.5894, -239.5718, 12.7},
    {-306.5853, -258.4539, 12.7},
    {-295.8789, -269.0899, 12.7},
    {-209.3401, -262.7564, 17.1},
    {-261.4533, -297.3298, 17.1},
}

-- VorpilPosition (boss_grandmaster_vorpil.cpp:65).
local VORPIL_X, VORPIL_Y, VORPIL_Z, VORPIL_O = -252.8820, -264.3030, 17.1, 0.0

local timers = {}
local introSeen = {} -- per-GUID _intro latch (C++ _intro, :159)
local helpYellSeen = {} -- per-GUID _helpYell latch (C++; re-armed on Reset)

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

-- Append-on-key (anzu scheduleOnce precedent): C++ EventMap::ScheduleEvent
-- is a plain multimap insert (EventMap.cpp) — scheduling a second same-ID
-- event does NOT cancel the pending one. Arms that C++ schedules twice
-- (volley 6s from Draw Shadows, traveler 5s below 20%, boom resonance)
-- must append, or the pending event is lost.
local appendNonce = 0
local function scheduleAppend(guid, key, delay, fn)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    appendNonce = appendNonce + 1
    per[key .. "#" .. appendNonce] = CreateLuaEvent(fn, delay)
end

-- C++ EVENT_SHADOWBOLT_VOLLEY: DoCast(me, SHADOWBOLT_VOLLEY 33841)
-- non-triggered self-cast, randtime(7s,14s) init / randtime(15s,30s)
-- re-arm.
local function shadowboltVolleyTick(creature, guid)
    creature:CastSpell(creature, SPELL_SHADOWBOLT_VOLLEY)
    scheduleAppend(guid, "volley", {15000, 30000}, function()
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

-- C++ EVENT_DRAW_SHADOWS (:187-200): every alive non-banished player
-- on the boss's map teleports to VorpilPosition (player:TeleportNear —
-- same-map, combat kept, the TELE_TO_NOT_LEAVE_COMBAT twin; C++
-- iterates me->GetMap()->GetPlayers(), so the pool is map-filtered —
-- GetPlayersInWorld() unfiltered would teleport players in other
-- maps/instances to these coordinates in their own map); the boss
-- repositions via creature:Teleport (C++ me->UpdatePosition sends no
-- packet, but Go clients only learn the reposition from the
-- MSG_MOVE_TELEPORT broadcast — the draw-shadows visual covers the
-- pop); triggered self-cast SPELL_DRAW_SHADOWS 33563 +
-- non-triggered SPELL_RAIN_OF_FIRE 33617 (H_SPELL_RAIN_OF_FIRE 39363
-- is a dead constant, no IsHeroic branch exists in this arm);
-- EVENT_SHADOWBOLT_VOLLEY scheduled at 6s (:198 — a multimap ADD,
-- not an overwrite: the pending volley still fires); 45s init / 30s
-- cycle.
local function drawShadowsTick(creature, guid)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    for _, plr in ipairs(GetPlayersInWorld()) do
        if plr:GetMapId() == mapId and plr:GetInstanceId() == instanceId
                and plr:IsAlive() and not plr:HasAura(SPELL_BANISH) then
            plr:TeleportNear(VORPIL_X, VORPIL_Y, VORPIL_Z, VORPIL_O)
        end
    end
    creature:Teleport(VORPIL_X, VORPIL_Y, VORPIL_Z, VORPIL_O)
    creature:CastSpell(creature, SPELL_DRAW_SHADOWS, true)
    creature:CastSpell(creature, SPELL_RAIN_OF_FIRE)
    scheduleAppend(guid, "volley", 6000, function()
        shadowboltVolleyTick(creature, guid)
    end)
    schedule(guid, "shadows", 30000, function()
        drawShadowsTick(creature, guid)
    end)
end

-- C++ boss_grandmaster_vorpilAI::spawnVoidTraveler (:118-126): summon
-- NPC_VOID_TRAVELER at a random VoidPortalCoords entry
-- (TEMPSUMMON_CORPSE_TIMED_DESPAWN, 5s — Lua type 5) plus the one-shot
-- _helpYell Talk(SAY_HELP 2) latch, then the EVENT_SUMMON_TRAVELER
-- re-arm: 10s always, plus an ADDITIONAL 5s event below 20% health
-- (ScheduleEvent is a plain multimap insert — the 5s is not a 5s
-- replacement, the vorpil 10:56 precedent). The traveler's own
-- follow/sacrifice AI (npc_voidtraveler) has no bridge — the summoned
-- travelers stand (documented-only in the header).
local function travelerTick(creature, guid)
    local c = VOID_PORTAL_COORDS[math.random(5)]
    creature:SummonCreature(ENTRY_VOID_TRAVELER, c[1], c[2], c[3], 0, 5, 5000)
    if not helpYellSeen[guid] then
        helpYellSeen[guid] = true
        creature:Talk(SAY_HELP)
    end
    -- Both re-arms append (multimap): the 10s chain and the extra 5s
    -- below 20% are independent pending events in C++; replace-on-key
    -- here would cancel the pending 10s chain whenever a 5s event
    -- fired, under-spawning travelers below 20%.
    scheduleAppend(guid, "traveler", 10000, function()
        travelerTick(creature, guid)
    end)
    if creature:GetHealthPct() < 20 then
        scheduleAppend(guid, "traveler", 5000, function()
            travelerTick(creature, guid)
        end)
    end
end

-- C++ boss_grandmaster_vorpilAI::JustEngagedWith:
-- events.ScheduleEvent(EVENT_SHADOWBOLT_VOLLEY, 7s, 14s);
-- if (IsHeroic()) events.ScheduleEvent(EVENT_BANISH, 15s);
-- events.ScheduleEvent(EVENT_DRAW_SHADOWS, 45s);
-- events.ScheduleEvent(EVENT_SUMMON_TRAVELER, 90s);
-- Talk(SAY_AGGRO); SummonPortals(); — the auriaya precedent
-- (event 1; the EVENT_SHADOWBOLT_VOLLEY leg is ported above and the
-- heroic-only EVENT_BANISH leg is ported as banishTick seeded under
-- creature:IsHeroic(). EVENT_SUMMON_TRAVELER is scheduled at 90s AND
-- SummonPortals() schedules it at 5s — both pend, the EventMap multimap
-- precedent, so both seeds are planted below). SummonPortals() (:108-115):
-- 5 NPC_VOID_PORTAL summons at VoidPortalCoords
-- (TEMPSUMMON_CORPSE_DESPAWN — Lua type 4; the 50min C++ timer is moot
-- for corpse-despawn) each with a triggered SPELL_VOID_PORTAL_VISUAL
-- self-cast (DoCastAOE self-cast convention).
local function vorpilEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    for i = 1, 5 do
        local c = VOID_PORTAL_COORDS[i]
        local portal = creature:SummonCreature(ENTRY_VOID_PORTAL, c[1], c[2], c[3], 0, 4)
        if portal then
            portal:CastSpell(portal, SPELL_VOID_PORTAL_VISUAL, true)
        end
    end
    schedule(guid, "traveler", 5000, function()
        travelerTick(creature, guid)
    end)
    schedule(guid, "traveler90", 90000, function()
        travelerTick(creature, guid)
    end)
    schedule(guid, "volley", {7000, 14000}, function()
        shadowboltVolleyTick(creature, guid)
    end)
    -- C++ :144-145: the EVENT_BANISH scheduler seeds only when
    -- IsHeroic() (the earlier "no heroic bridge" claim was stale —
    -- creature:IsHeroic() is bridged,
    -- engine/world/lua_creature_events.go:409-417; the lavanthor
    -- precedent).
    if creature:IsHeroic() then
        schedule(guid, "banish", 15000, function()
            banishTick(creature, guid)
        end)
    end
    -- C++ :143: events.ScheduleEvent(EVENT_DRAW_SHADOWS, 45s).
    schedule(guid, "shadows", 45000, function()
        drawShadowsTick(creature, guid)
    end)
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
-- _helpYell ONLY: _intro is set false in the constructor (:93-94) and
-- never touched by Reset, so after an evade the intro Talk does NOT
-- replay. Go fires event 23 on evade (before event 2) and on the death
-- path (before event 4), so the IsDead guard (overgrind precedent)
-- keeps the death path out; the intro latch is cleared on event 4
-- only (respawn = fresh ctor state).
local function vorpilReset(event, creature)
    if creature:IsDead() then
        return
    end
    local guid = creature:GetGUID()
    cancelTimers(guid)
    helpYellSeen[guid] = nil
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
