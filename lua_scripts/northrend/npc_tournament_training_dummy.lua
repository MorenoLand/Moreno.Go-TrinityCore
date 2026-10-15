-- Tournament Training Dummy (Icecrown, Argent Tournament) --
-- Lua port of src/server/scripts/Northrend/zone_icecrown.cpp
-- (npc_tournament_training_dummyAI — charge-target arms only).
-- Zone-script unit per northrend_script_loader.cpp order (decl 203 /
-- call 398; howling_fjord done, icecrown: 5 registrations).
-- Retraction re-audit of the 11:28 "VERIFIED already ported (1fa3069)"
-- entry: that audit verified the blessed_banner Reset arm but the
-- dummy's timer/SpellHit/DamageTaken arms were never ported — the
-- "zero additional bridgeable arms" verdict predates the retraction
-- lens (CreateLuaEvent + event-14 + event-9-damage-rewrite are all
-- real bridges).
-- Entry: NPC_CHARGE_TARGET = 33272 (TournamentDummy enum :171,
-- C++-verifiable). Only the charge-target leg ports — the ranged leg
-- needs GetAura() stack/duration gates (no bridge) and the melee
-- target has no cast leg.
-- Eluna creature events: 5 OnSpawn, 23 OnReset, 14 OnHitBySpell,
-- 9 OnDamageTaken, 2 OnLeaveCombat, 4 OnDied.
-- Spells (TournamentDummy enum :169-191): SPELL_CHARGE_DEFEND 64100,
-- SPELL_RANGED_DEFEND 62719, SPELL_PLAYER_BREAK_SHIELD 62626,
-- SPELL_PLAYER_CHARGE 62874, SPELL_CHARGE_CREDIT 62658.
-- Ported arm 1 (C++ Reset() :214-221 -> EVENT_DUMMY_RECAST_DEFEND
-- :278-294): Reset-seeded 5s repeating timer (first fire 5s after
-- reset — C++ schedules, not immediate); the 33272 leg:
-- if (!me->HasAura(SPELL_CHARGE_DEFEND)) DoCast(me, 64100, true) —
-- triggered self-cast, HasAura gate bridged (gunship precedent);
-- then isVulnerable=false (:293) and the 5s re-arm. The
-- SetControlled(STUNNED) Reset leg has no control-state bridge
-- (documented).
-- Ported arm 2 (C++ SpellHit :237-269): receive-side event 14 is
-- engine-fired (boss_ai.go / spells.go) — the "no-SpellHit bridge"
-- doctrine is stale. if (spellId == 62626 SPELL_PLAYER_BREAK_SHIELD)
-- if (!HasAura(64100) && !HasAura(62719)) isVulnerable = true
-- (:267-269, outside the entry switch — applies to all dummies);
-- if (spellId == 62874 SPELL_PLAYER_CHARGE) if (isVulnerable)
-- DoCast(unitCaster, 62658 SPELL_CHARGE_CREDIT, true) — the
-- creature:CastSpell(caster, spell, true) convention (zuljin
-- precedent). isVulnerable is per-GUID lua state (C++ member :210,
-- init false :207/:215).
-- Ported arm 3 (C++ DamageTaken :226-230): damage = 0 unconditionally
-- -> event-9 damage-rewrite returns (true, 0) (lich_king_escape
-- precedent). The RescheduleEvent(EVENT_DUMMY_RESET, 10s) leg rides
-- the unbridged evade machine (EVENT_DUMMY_RESET :296-302:
-- UpdateVictim-gated EnterEvadeMode, no evade bridge) — documented
-- deviation.
-- Unmodeled: EVENT_DUMMY_RECAST_DEFEND ranged leg (aura stack <
-- 3 / duration <= 8000 gates — no aura-detail bridge); melee-target
-- entry (no cast leg); the SpellHit melee/ranged credit legs
-- (vehicle-base / counterattack gates); argent_valiant's charge /
-- shield-breaker combat timers (no NPC_ARGENT_VALIANT entry constant
-- anywhere in-tree — entry-blocked, stinky precedent); DamageTaken
-- kill-credit machine (faction/despawn/evade legs); guardian_pavilion
-- (MoveInLineOfSight area/team/aura gates — no area/team bridges);
-- frostbrood_skytalon (IsSummonedBy/MovementInform/vehicle legs);
-- blessed_banner summon/intro/wave machines (Reset arm ported Oct 1,
-- 1fa3069).
-- Go: RegisterLuaBoss("npc_tournament_training_dummy", 33272) — the
-- no-op AI neutralizes default combat behavior for this stunned
-- non-combatant (storm_cloud / blessed_banner zone-script precedent).

local ENTRY_CHARGE_TARGET = 33272

local SPELL_CHARGE_DEFEND = 64100
local SPELL_RANGED_DEFEND = 62719
local SPELL_PLAYER_BREAK_SHIELD = 62626
local SPELL_PLAYER_CHARGE = 62874
local SPELL_CHARGE_CREDIT = 62658

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

-- C++ :278-294 EVENT_DUMMY_RECAST_DEFEND (33272 leg): !HasAura(64100)
-- -> triggered self-cast 64100; isVulnerable=false; re-arm 5s.
local function onRecastDefend(creature, guid)
    if not creature:HasAura(SPELL_CHARGE_DEFEND) then
        creature:CastSpell(creature, SPELL_CHARGE_DEFEND, true)
    end
    vulnerable[guid] = false
    schedule(guid, "recast_defend", RECAST_INTERVAL, function() onRecastDefend(creature, guid) end)
end

-- C++ Reset() :214-221: Initialize() (isVulnerable=false) + schedule
-- EVENT_DUMMY_RECAST_DEFEND 5s (first fire delayed, not immediate).
local function dummySpawnOrReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    vulnerable[guid] = false
    schedule(guid, "recast_defend", RECAST_INTERVAL, function() onRecastDefend(creature, guid) end)
end

local function dummyLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ SpellHit :237-269 (33272): 62626 break-shield -> vulnerable
-- latch (dual HasAura gate :267-269); 62874 charge -> credit cast on
-- the caster while vulnerable.
local function dummySpellHit(event, creature, caster, spellId)
    local guid = creature:GetGUID()
    if spellId == SPELL_PLAYER_BREAK_SHIELD then
        if not creature:HasAura(SPELL_CHARGE_DEFEND) and not creature:HasAura(SPELL_RANGED_DEFEND) then
            vulnerable[guid] = true
        end
    elseif spellId == SPELL_PLAYER_CHARGE then
        if vulnerable[guid] and caster then
            creature:CastSpell(caster, SPELL_CHARGE_CREDIT, true)
        end
    end
end

-- C++ DamageTaken :226-230: damage = 0 unconditionally (the
-- EVENT_DUMMY_RESET reschedule leg rides the unbridged evade machine).
local function dummyDamageTaken(event, creature, attacker, damage)
    return true, 0
end

RegisterCreatureEvent(ENTRY_CHARGE_TARGET, 5, dummySpawnOrReset)
RegisterCreatureEvent(ENTRY_CHARGE_TARGET, 23, dummySpawnOrReset)
RegisterCreatureEvent(ENTRY_CHARGE_TARGET, 14, dummySpellHit)
RegisterCreatureEvent(ENTRY_CHARGE_TARGET, 9, dummyDamageTaken)
RegisterCreatureEvent(ENTRY_CHARGE_TARGET, 2, dummyLeaveCombat)
RegisterCreatureEvent(ENTRY_CHARGE_TARGET, 4, dummyLeaveCombat)
