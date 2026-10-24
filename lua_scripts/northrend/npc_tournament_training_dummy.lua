-- Tournament Training Dummy (Icecrown, Argent Tournament) --
-- Lua port of src/server/scripts/Northrend/zone_icecrown.cpp
-- (npc_tournament_training_dummyAI, all three entries). Zone-script
-- unit per northrend_script_loader.cpp order.
-- MERGE NOTE (2026-10-18): the 02:3x tick added a second,
-- overlapping implementation of the SpellHit arms in
-- npc_blessed_banner.lua, double-registering event 14 on 33272
-- (double charge-credit casts) and event 23 (competing latch
-- clears). That block is removed; this file is the single home of
-- the dummy arms for all three entries. The melee/ranged legs it
-- documents as unmodelable were stale — C++ has an ungated
-- thrust-credit cast (33229) and a vulnerable-gated break-shield
-- credit cast (33243); only the 62709 vehicle-base counterattack
-- leg stays out (the aura-stack recast leg is now ported,
-- see arm 1 below).
-- Entries (TournamentDummy enum, zone_icecrown.cpp:169-191,
-- C++-verifiable): NPC_CHARGE_TARGET = 33272,
-- NPC_MELEE_TARGET = 33229, NPC_RANGED_TARGET = 33243.
-- The creature_template ScriptName binding is DB-side (no TDB in
-- this workspace).
-- Eluna creature events: 5 OnSpawn, 23 OnReset, 14 OnHitBySpell,
-- 9 OnDamageTaken, 2 OnLeaveCombat (evade re-arm, see below), 4 OnDied.
-- Spells (TournamentDummy enum): SPELL_CHARGE_CREDIT 62658,
-- SPELL_MELEE_CREDIT 62672, SPELL_RANGED_CREDIT 62673,
-- SPELL_PLAYER_THRUST 62544, SPELL_PLAYER_BREAK_SHIELD 62626,
-- SPELL_PLAYER_CHARGE 62874, SPELL_RANGED_DEFEND 62719,
-- SPELL_CHARGE_DEFEND 64100, SPELL_VULNERABLE 62665,
-- SPELL_COUNTERATTACK 62709.
-- Ported arm 1 (C++ Reset() :214-221 -> EVENT_DUMMY_RECAST_DEFEND
-- :278-294): Reset-seeded 5s repeating timer (first fire 5s after
-- reset — C++ schedules, not immediate); entry-switched: 33272 leg
-- if (!me->HasAura(SPELL_CHARGE_DEFEND)) DoCast(me, 64100, true) —
-- triggered self-cast, HasAura gate bridged (gunship precedent);
-- the 33243 leg (aura stack < 3 / duration <= 8000 gate) is
-- ported via creature:GetStackAmount(spell) and
-- creature:GetDuration(spell) (both bridged —
-- lua_creature_events.go:416; an absent aura reads 0, subsuming
-- the !defend leg), C++-exact. The 33229 entry
-- has no recast leg. All entries: isVulnerable=false (:293) and the 5s re-arm.
-- The SetControlled(STUNNED) Reset leg has no control-state
-- bridge (documented).
-- Ported arm 2 (C++ SpellHit :237-267, entry-switched):
-- 33272: 62874 SPELL_PLAYER_CHARGE, isVulnerable-gated ->
-- DoCast(unitCaster, 62658 SPELL_CHARGE_CREDIT, true);
-- 33229: 62544 SPELL_PLAYER_THRUST -> DoCast(unitCaster,
-- 62672 SPELL_MELEE_CREDIT, true), UNGATED (C++ has no
-- isVulnerable gate on the melee arm); the 62709 counterattack on
-- the caster's vehicle base stays out (no vehicle-base bridge);
-- 33243: 62626 SPELL_PLAYER_BREAK_SHIELD, isVulnerable-gated ->
-- DoCast(unitCaster, 62673 SPELL_RANGED_CREDIT, true).
-- creature:CastSpell(caster, spell, true) convention (zuljin
-- precedent). The 62626 latch leg (:266-269, outside the entry
-- switch — applies to all entries): if (!HasAura(64100) &&
-- !HasAura(62719)) isVulnerable = true — creature:HasAura bridged
-- (lua_creature_events.go:141). isVulnerable is per-GUID lua state
-- (C++ member :210, init false in Initialize :207/:215 — reset
-- here on spawn/reset; cleared on leave-combat/died via the
-- evade-Reset path, halycon precedent).
-- Ported arm 3 (C++ DamageTaken :226-230, shared AI — all three
-- entries): damage = 0 unconditionally -> event-9 damage-rewrite
-- returns (true, 0) (lich_king_escape precedent). The
-- RescheduleEvent(EVENT_DUMMY_RESET, 10s) leg rides the unbridged
-- evade machine (EVENT_DUMMY_RESET :296-302: UpdateVictim-gated
-- EnterEvadeMode, no evade bridge) — documented deviation; the
-- latch-clear side effect is moot since the 5s recast clears it,
-- and the evade->Reset defend re-arm IS modeled (ev23 then ev2
-- both re-arm — see the ev2 registration note below).
-- Unmodeled: the 62709 counterattack; the
-- SetControlled stunned leg; npc_argent_valiant's charge /
-- shield-breaker combat timers (no NPC_ARGENT_VALIANT entry
-- constant anywhere in-tree — entry-blocked, stinky precedent);
-- DamageTaken kill-credit machine (faction/despawn/evade legs);
-- npc_guardian_pavilion (MoveInLineOfSight area/team/aura gates —
-- no area/team bridges); npc_frostbrood_skytalon (VehicleAI);
-- blessed_banner summon/intro/wave machines (Reset arm ported in
-- npc_blessed_banner.lua).
-- No RegisterLuaBoss entry exists for the dummy entries
-- (boss_ai.go registers blessed_banner 30891 only); the file
-- is event-driven, and the no-op boss AI is not required for
-- the ev5/23/14/9/2/4/27 bindings.

local ENTRY_CHARGE_TARGET = 33272
local ENTRY_MELEE_TARGET = 33229
local ENTRY_RANGED_TARGET = 33243

local SPELL_CHARGE_CREDIT = 62658
local SPELL_MELEE_CREDIT = 62672
local SPELL_RANGED_CREDIT = 62673
local SPELL_PLAYER_THRUST = 62544
local SPELL_PLAYER_BREAK_SHIELD = 62626
local SPELL_PLAYER_CHARGE = 62874
local SPELL_RANGED_DEFEND = 62719
local SPELL_CHARGE_DEFEND = 64100

local RECAST_INTERVAL = 5000

local timers = {}
local vulnerable = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
    vulnerable[guid] = nil
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

-- C++ :278-294 EVENT_DUMMY_RECAST_DEFEND: 33272 leg
-- (!HasAura(64100) -> triggered self-cast 64100) + 33243 leg
-- (!HasAura(62719) or stack < 3 or duration <= 8000 ->
-- triggered self-cast 62719, C++-exact via GetStackAmount /
-- GetDuration; a presence-only aura reads stack 0, so the
-- recast may repeat at 5s rather than only below 3 stacks);
-- all entries get
-- isVulnerable=false + the 5s re-arm.
local function onRecastDefend(creature, guid)
    if creature:GetEntry() == ENTRY_CHARGE_TARGET
            and not creature:HasAura(SPELL_CHARGE_DEFEND) then
        creature:CastSpell(creature, SPELL_CHARGE_DEFEND, true)
    end
    if creature:GetEntry() == ENTRY_RANGED_TARGET
            and (not creature:HasAura(SPELL_RANGED_DEFEND)
                or creature:GetStackAmount(SPELL_RANGED_DEFEND) < 3
                or creature:GetDuration(SPELL_RANGED_DEFEND) <= 8000) then
        creature:CastSpell(creature, SPELL_RANGED_DEFEND, true)
    end
    vulnerable[guid] = false
    schedule(guid, "recast_defend", RECAST_INTERVAL, function() onRecastDefend(creature, guid) end)
end

-- C++ Reset() :214-221: Initialize() (isVulnerable=false) +
-- schedule EVENT_DUMMY_RECAST_DEFEND 5s (first fire delayed).
-- C++ ctor (:203) SetCombatMovement(false) rides here too
-- (re-applied on reset/evade — value-idempotent).
local function dummySpawnOrReset(event, creature)
    local guid = creature:GetGUID()
    creature:SetCombatMovement(false)
    cancelTimers(guid)
    vulnerable[guid] = false
    schedule(guid, "recast_defend", RECAST_INTERVAL, function() onRecastDefend(creature, guid) end)
end

-- Evade order (engine triggerCreatureEvade, creaturemotion.go):
-- ev23 fires BEFORE ev2. The old ev2 handler cancelled the
-- timers, undoing the ev23 Reset re-arm — after the first
-- evade the defend timer never ran again. ev2 now re-arms via
-- dummySpawnOrReset (C++ evade -> Reset semantics); ev4 keeps
-- the cancel (a dead dummy runs no timers; spawn re-arms).
local function dummyDied(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ SpellHit :237-269: entry-switched credit casts + the global
-- 62626 vulnerable latch (dual HasAura gate).
local function dummySpellHit(event, creature, caster, spellId)
    local guid = creature:GetGUID()
    local entry = creature:GetEntry()
    if entry == ENTRY_CHARGE_TARGET then
        if spellId == SPELL_PLAYER_CHARGE and vulnerable[guid] and caster then
            creature:CastSpell(caster, SPELL_CHARGE_CREDIT, true)
        end
    elseif entry == ENTRY_MELEE_TARGET then
        if spellId == SPELL_PLAYER_THRUST and caster then
            creature:CastSpell(caster, SPELL_MELEE_CREDIT, true)
        end
    elseif entry == ENTRY_RANGED_TARGET then
        if spellId == SPELL_PLAYER_BREAK_SHIELD and vulnerable[guid] and caster then
            creature:CastSpell(caster, SPELL_RANGED_CREDIT, true)
        end
    end
    if spellId == SPELL_PLAYER_BREAK_SHIELD then
        if not creature:HasAura(SPELL_CHARGE_DEFEND)
                and not creature:HasAura(SPELL_RANGED_DEFEND) then
            vulnerable[guid] = true
        end
    end
end

-- C++ DamageTaken :226-230: damage = 0 unconditionally (the
-- EVENT_DUMMY_RESET reschedule leg rides the unbridged evade
-- machine — documented above).
local function dummyDamageTaken(event, creature, attacker, damage)
    return true, 0
end

RegisterCreatureEvent(ENTRY_CHARGE_TARGET, 5, dummySpawnOrReset)
RegisterCreatureEvent(ENTRY_CHARGE_TARGET, 23, dummySpawnOrReset)
RegisterCreatureEvent(ENTRY_CHARGE_TARGET, 14, dummySpellHit)
RegisterCreatureEvent(ENTRY_CHARGE_TARGET, 9, dummyDamageTaken)
RegisterCreatureEvent(ENTRY_CHARGE_TARGET, 2, dummySpawnOrReset)
RegisterCreatureEvent(ENTRY_CHARGE_TARGET, 4, dummyDied)

RegisterCreatureEvent(ENTRY_MELEE_TARGET, 5, dummySpawnOrReset)
RegisterCreatureEvent(ENTRY_MELEE_TARGET, 23, dummySpawnOrReset)
RegisterCreatureEvent(ENTRY_MELEE_TARGET, 14, dummySpellHit)
RegisterCreatureEvent(ENTRY_MELEE_TARGET, 9, dummyDamageTaken)
RegisterCreatureEvent(ENTRY_MELEE_TARGET, 2, dummySpawnOrReset)
RegisterCreatureEvent(ENTRY_MELEE_TARGET, 4, dummyDied)

RegisterCreatureEvent(ENTRY_RANGED_TARGET, 5, dummySpawnOrReset)
RegisterCreatureEvent(ENTRY_RANGED_TARGET, 23, dummySpawnOrReset)
RegisterCreatureEvent(ENTRY_RANGED_TARGET, 14, dummySpellHit)
RegisterCreatureEvent(ENTRY_RANGED_TARGET, 9, dummyDamageTaken)
RegisterCreatureEvent(ENTRY_RANGED_TARGET, 2, dummySpawnOrReset)
RegisterCreatureEvent(ENTRY_RANGED_TARGET, 4, dummyDied)
for _, entry in ipairs({ ENTRY_CHARGE_TARGET, ENTRY_MELEE_TARGET, ENTRY_RANGED_TARGET }) do
    RegisterCreatureEvent(entry, 1, function(_, creature)
        creature:SetMeleeAttack(false) -- zone_icecrown.cpp:271: custom UpdateAI never calls DoMeleeAttackIfReady (defend/stun machine in C++, never melees)
    end)
end

-- C++ npc_tournament_training_dummyAI::MoveInLineOfSight
-- (zone_icecrown.cpp:313) is an EMPTY override — it suppresses the default
-- ScriptedAI::MoveInLineOfSight aggro engage (the dummies never attack).
-- Go's fireCreatureMoveInLOS (lua_creature_events.go:531) fires ev27 on the
-- aggro-engage branch and a boolean true vetoes exactly that engage, so
-- return true models :313 C++-exact (the cyanigosa precedent); registered
-- for all three dummy entries sharing the AI.
local function dummyMoveInLOS(event, creature, unit)
    return true
end

RegisterCreatureEvent(ENTRY_CHARGE_TARGET, 27, dummyMoveInLOS)
RegisterCreatureEvent(ENTRY_MELEE_TARGET, 27, dummyMoveInLOS)
RegisterCreatureEvent(ENTRY_RANGED_TARGET, 27, dummyMoveInLOS)
