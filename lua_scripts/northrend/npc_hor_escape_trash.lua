-- Halls of Reflection escape-event trash — Lua port of the three
-- npc_escape_event_trash CreatureScripts in
-- src/server/scripts/Northrend/FrozenHalls/HallsOfReflection/halls_of_reflection.cpp
-- (2916 lines; part of the 25 scripts registered from inside
-- AddSC_halls_of_reflection(); loader decl 167 / call 362 per
-- northrend_script_loader.cpp — the SECOND group of the "// Halls of
-- Reflection" block in AddNorthrendScripts(), immediately after
-- AddSC_instance_halls_of_reflection() (call 361), before
-- AddSC_boss_falric() (call 363)).
-- Entries (halls_of_reflection.h lines 87-89; the CreatureScript
-- ScriptName bindings are DB-side as usual):
--   36940 raging ghoul (npc_raging_ghoul, C++ :2200-2263)
--   36941 risen witch doctor (npc_risen_witch_doctor, C++ :2264-2333)
--   37069 lumbering abomination (npc_lumbering_abomination, C++ :2334-2457)
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 5 OnSpawn (the IsSummonedBy funnel — fireCreatureSpawned fires
-- event 23 then event 5, engine/world/lua_creature_events.go:3719),
-- 23 OnReset. Timers via CreateLuaEvent ({min,max} table = randtime
-- range), GUID-keyed, cancelled on events 2/4/23 (the bronjahm /
-- devourer_of_souls convention); creature:CastSpell(nil, spellId) =
-- victim-cast, creature:CastSpell(creature, spellId) = self-cast (the
-- DoCastAOE convention); all combat casts here are non-triggered.
-- Threat-list pools (the 2026-10-25 strip convention, UnitAI.h):
-- C++ SelectTarget draws the threat list, not the instance player
-- list, so random/max-threat picks use creature:GetAITargets()
-- filtered to Player + alive + IsWithinDist, never GetPlayersInWorld.
--
-- PORTED ARMS (C++ line refs; C++-exact for the modeled arms):
-- npc_raging_ghoul Reset (:2210-2213) — EVENT_RAGING_GHOUL_JUMP seed
--   5000ms, seeded on ev1 (first combat) instead of C++ Reset/spawn;
--   the C++ spawn seed is unreachable in Go without a Reset-on-spawn
--   arm that also fires on respawn, and the 5s passive window below
--   means no victim exists before combat anyway.
-- EVENT_RAGING_GHOUL_JUMP (:2239-2249) — one-shot-on-success pump:
--   victim = GetVictim(); if within [5,30] yd (C++ IsInRange, via
--   plain GetDistance — the reach-adjusted nuance is approximated)
--   DoCast(victim, SPELL_GHOUL_JUMP 70150) and STOP (C++ returns
--   without re-arming); otherwise re-arm 500ms. No re-arm after a
--   successful jump — the C++ case returns, it does not reschedule.
-- npc_risen_witch_doctor Reset (:2274-2279) — SHADOW_BOLT 6000ms,
--   SHADOW_BOLT_VOLLEY 15000ms, CURSE 7000ms seeds on ev1.
-- EVENT_RISEN_WITCH_DOCTOR_SHADOW_BOLT (:2310-2314) — MaxThreat
--   pick = first threat-ordered Player + alive within 20 yd,
--   cast 70080, re-arm {2000,3000}ms.
-- EVENT_RISEN_WITCH_DOCTOR_CURSE (:2305-2309) — random threat-list
--   player within 30 yd, cast 70144, re-arm {10000,15000}ms.
-- EVENT_RISEN_WITCH_DOCTOR_SHADOW_BOLT_VOLLEY (:2315-2319) — cast
--   only if a random threat-list player within 30 yd exists (the
--   C++ SelectTarget existence gate), then DoCastAOE self-cast
--   70145, re-arm {15000,22000}ms (re-arm unconditional, like C++).
-- npc_lumbering_abomination Reset (:2343-2347) — VOMIT_SPRAY
--   15000ms, CLEAVE 6000ms seeds on ev1.
-- EVENT_LUMBERING_ABOMINATION_VOMIT_SPRAY (:2352-2356) —
--   DoCastVictim 70176, re-arm {15000,20000}ms.
-- EVENT_LUMBERING_ABOMINATION_CLEAVE (:2357-2361) — DoCastVictim
--   40505, re-arm {7000,9000}ms.
-- IsSummonedBy (:2215-2224 ghoul, :2281-2290 witch doctor) —
--   triggered self-cast SPELL_RAGING_GHOUL_SPAWN 69636 /
--   SPELL_RISEN_WITCH_DOCTOR_SPAWN 69639, SetReactState(REACT_PASSIVE
--   = 0, bridged lua_creature_events.go:1934),
--   HandleEmoteCommand(EMOTE_ONESHOT_EMERGE 449, SharedDefines.h:2086),
--   then the base-class arm (below), in C++ arm order.
-- HoRStartMovementEvent (:2151-2167) — the 5s delayed
--   SetReactState(REACT_AGGRESSIVE) flip IS ported (wakes table, NOT
--   cancelled on ev2/ev23 — the C++ event is a BasicEvent, not an
--   EventMap event, so it survives evade/reset); the AttackStart
--   leg stays out (no AttackStart bridge on the creature event
--   surface — the :2091 comment is about SetCombatMovement, and no
--   Lua file calls :AttackStart(). The flip must ride with the
--   passive arm: without it the creature would stay passive forever.
-- npc_escape_event_trash::IsSummonedBy (:2183-2193) — DoZoneInCombat
--   (bridged lua_creature_events.go:734) ported on ev5 for all three,
--   after the derived arms, in C++ order (derived IsSummonedBy calls
--   the base at its end; abomination has no override, base only).
--
-- DOCUMENTED-ONLY (no registration beyond the three entries above):
-- npc_escape_event_trash::IsSummonedBy leader link (:2186-2192) —
--   instance GetGuidData(DATA_ESCAPE_LEADER) + SetInCombatWith +
--   AddThreat(leader, 0): no instance bridge, and SetInCombatWith
--   has no Lua bridge (engine-internal CombatManager only, the
--   2026-10-22 SetInCombatWith sweep).
-- npc_escape_event_trash::UpdateAI despawn (:2176-2180) — despawns
--   when instance DATA_THE_LICH_KING_ESCAPE is FAIL/NOT_STARTED:
--   no instance boss-state bridge.
-- npc_escape_event_trash::Reset (:2172-2174) — EventMap reset only;
--   covered by the event-2/4/23 timer cancel.
--
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (creature
-- casts are packet-visual), so timers fire unconditionally (the
-- maiden precedent); ev5 fires on every spawn including respawn,
-- while C++ IsSummonedBy fires only on summon — for escape trash
-- the two coincide in practice (LK-summoned during the escape
-- event; the base UpdateAI despawns them outside it).

local ENTRY_RAGING_GHOUL = 36940
local ENTRY_RISEN_WITCH_DOCTOR = 36941
local ENTRY_LUMBERING_ABOMINATION = 37069

local SPELL_GHOUL_JUMP = 70150
local SPELL_RAGING_GHOUL_SPAWN = 69636
local SPELL_CURSE_OF_DOOM = 70144
local SPELL_SHADOW_BOLT_VOLLEY = 70145
local SPELL_SHADOW_BOLT = 70080
local SPELL_RISEN_WITCH_DOCTOR_SPAWN = 69639
local SPELL_CLEAVE = 40505
local SPELL_VOMIT_SPRAY = 70176

local EMOTE_ONESHOT_EMERGE = 449

local timers = {}
local wakes = {}

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

local function alive(guid)
    return timers[guid] ~= nil
end

-- C++ SelectTarget(SelectTargetMethod::Random, 0, dist, true): a random
-- alive threat-list player within dist yards.
local function randomPlayerInRange(creature, dist)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:GetObjectType() == "Player" and not t:IsDead()
                and creature:GetDistance(t) - creature:GetCombatReach() - 1.5 < dist then -- IsWithinCombatRange twin (Unit.cpp:583; player reach 1.5, ObjectDefines.h:40)
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ SelectTarget(SelectTargetMethod::MaxThreat, 0, dist, true): the
-- first threat-ordered entry that is a Player + alive + within dist
-- (GetAITargets is threat-ordered, the eck skip-first precedent).
local function maxThreatPlayerInRange(creature, dist)
    for _, t in ipairs(creature:GetAITargets()) do
        if t:GetObjectType() == "Player" and not t:IsDead()
                and creature:GetDistance(t) - creature:GetCombatReach() - 1.5 < dist then -- IsWithinCombatRange twin (Unit.cpp:583; player reach 1.5, ObjectDefines.h:40)
            return t
        end
    end
    return nil
end

-- Shared spawn tail: 5s aggressive flip (HoRStartMovementEvent) plus
-- the base-class DoZoneInCombat arm, in C++ order.
local function spawnTail(creature, guid)
    wakes[guid] = CreateLuaEvent(function()
        wakes[guid] = nil
        creature:SetReactState(REACT_AGGRESSIVE)
    end, 5000)
    creature:DoZoneInCombat()
end

-- Raging ghoul --------------------------------------------------------
-- C++ :2239-2249: one-shot-on-success jump pump, 500ms retry.
local function ghoulJump(creature, guid)
    if not alive(guid) then return end
    local victim = creature:GetVictim()
    if victim then
        local dist = creature:GetDistance(victim) - creature:GetCombatReach()
        if dist >= 5 and dist < 30 then
            creature:CastSpell(victim, SPELL_GHOUL_JUMP)
            return
        end
    end
    schedule(guid, "jump", 500, function() ghoulJump(creature, guid) end)
end

local function ghoulEngaged(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "jump", 5000, function() ghoulJump(creature, guid) end)
end

-- C++ :2215-2224: spawn self-cast, passive, emerge emote, then base.
local function ghoulSpawn(event, creature)
    creature:CastSpell(creature, SPELL_RAGING_GHOUL_SPAWN, true)
    creature:SetReactState(REACT_PASSIVE)
    creature:HandleEmoteCommand(EMOTE_ONESHOT_EMERGE)
    spawnTail(creature, creature:GetGUID())
end

-- Risen witch doctor ---------------------------------------------------
-- C++ :2310-2314: MaxThreat 20 yd, cast 70080, re-arm {2000,3000}.
local function witchDoctorBolt(creature, guid)
    if not alive(guid) then return end
    local target = maxThreatPlayerInRange(creature, 20)
    if target then
        creature:CastSpell(target, SPELL_SHADOW_BOLT)
    end
    schedule(guid, "bolt", {2000, 3000}, function() witchDoctorBolt(creature, guid) end)
end

-- C++ :2315-2319: existence-gated self-cast 70145, re-arm {15000,22000}.
local function witchDoctorVolley(creature, guid)
    if not alive(guid) then return end
    if randomPlayerInRange(creature, 30) then
        creature:CastSpell(creature, SPELL_SHADOW_BOLT_VOLLEY)
    end
    schedule(guid, "volley", {15000, 22000}, function() witchDoctorVolley(creature, guid) end)
end

-- C++ :2305-2309: random 30 yd, cast 70144, re-arm {10000,15000}.
local function witchDoctorCurse(creature, guid)
    if not alive(guid) then return end
    local target = randomPlayerInRange(creature, 30)
    if target then
        creature:CastSpell(target, SPELL_CURSE_OF_DOOM)
    end
    schedule(guid, "curse", {10000, 15000}, function() witchDoctorCurse(creature, guid) end)
end

-- C++ JustEngagedWith via Reset seeds :2274-2279.
local function witchDoctorEngaged(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "bolt", 6000, function() witchDoctorBolt(creature, guid) end)
    schedule(guid, "volley", 15000, function() witchDoctorVolley(creature, guid) end)
    schedule(guid, "curse", 7000, function() witchDoctorCurse(creature, guid) end)
end

-- C++ :2281-2290: spawn self-cast, passive, emerge emote, then base.
local function witchDoctorSpawn(event, creature)
    creature:CastSpell(creature, SPELL_RISEN_WITCH_DOCTOR_SPAWN, true)
    creature:SetReactState(REACT_PASSIVE)
    creature:HandleEmoteCommand(EMOTE_ONESHOT_EMERGE)
    spawnTail(creature, creature:GetGUID())
end

-- Lumbering abomination ------------------------------------------------
-- C++ :2352-2356: DoCastVictim 70176, re-arm {15000,20000}.
local function abominationVomit(creature, guid)
    if not alive(guid) then return end
    creature:CastSpell(nil, SPELL_VOMIT_SPRAY)
    schedule(guid, "vomit", {15000, 20000}, function() abominationVomit(creature, guid) end)
end

-- C++ :2357-2361: DoCastVictim 40505, re-arm {7000,9000}.
local function abominationCleave(creature, guid)
    if not alive(guid) then return end
    creature:CastSpell(nil, SPELL_CLEAVE)
    schedule(guid, "cleave", {7000, 9000}, function() abominationCleave(creature, guid) end)
end

-- C++ JustEngagedWith via Reset seeds :2343-2347.
local function abominationEngaged(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "vomit", 15000, function() abominationVomit(creature, guid) end)
    schedule(guid, "cleave", 6000, function() abominationCleave(creature, guid) end)
end

-- C++ base IsSummonedBy only (no derived override): DoZoneInCombat.
local function abominationSpawn(event, creature)
    creature:DoZoneInCombat()
end

-- Shared teardown: combat timers cancel on 2/4/23; the wake flip is
-- a BasicEvent twin and survives evade/reset, cleared only on death.
local function onLeaveOrDiedOrReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    if event == 4 and wakes[guid] then
        RemoveEventById(wakes[guid])
        wakes[guid] = nil
    end
end

RegisterCreatureEvent(ENTRY_RAGING_GHOUL, 1, ghoulEngaged)
RegisterCreatureEvent(ENTRY_RAGING_GHOUL, 2, onLeaveOrDiedOrReset)
RegisterCreatureEvent(ENTRY_RAGING_GHOUL, 4, onLeaveOrDiedOrReset)
RegisterCreatureEvent(ENTRY_RAGING_GHOUL, 5, ghoulSpawn)
RegisterCreatureEvent(ENTRY_RAGING_GHOUL, 23, onLeaveOrDiedOrReset)

RegisterCreatureEvent(ENTRY_RISEN_WITCH_DOCTOR, 1, witchDoctorEngaged)
RegisterCreatureEvent(ENTRY_RISEN_WITCH_DOCTOR, 2, onLeaveOrDiedOrReset)
RegisterCreatureEvent(ENTRY_RISEN_WITCH_DOCTOR, 4, onLeaveOrDiedOrReset)
RegisterCreatureEvent(ENTRY_RISEN_WITCH_DOCTOR, 5, witchDoctorSpawn)
RegisterCreatureEvent(ENTRY_RISEN_WITCH_DOCTOR, 23, onLeaveOrDiedOrReset)

RegisterCreatureEvent(ENTRY_LUMBERING_ABOMINATION, 1, abominationEngaged)
RegisterCreatureEvent(ENTRY_LUMBERING_ABOMINATION, 2, onLeaveOrDiedOrReset)
RegisterCreatureEvent(ENTRY_LUMBERING_ABOMINATION, 4, onLeaveOrDiedOrReset)
RegisterCreatureEvent(ENTRY_LUMBERING_ABOMINATION, 5, abominationSpawn)
RegisterCreatureEvent(ENTRY_LUMBERING_ABOMINATION, 23, onLeaveOrDiedOrReset)
