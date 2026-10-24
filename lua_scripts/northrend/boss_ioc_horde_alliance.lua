-- Isle of Conquest faction bosses (Halford Wyrmbane / Overlord Agmar) --
-- Lua port of src/server/scripts/Northrend/IsleOfConquest/boss_ioc_horde_alliance.cpp
-- (boss_ioc_horde_allianceAI — engage-seeded timer arms only).
-- Zone-script unit per northrend_script_loader.cpp order (decl 196 /
-- call 391, second and last group of the "// Isle of Conquest" block;
-- AddSC_isle_of_conquest() already closed document-only).
-- Retraction re-audit of the 15:24 "VERIFIED CLOSED" entry: that audit
-- closed the file as document-only under the stale "lua port surface =
-- Talk() arms only" doctrine. The file carries zero Talk() calls
-- (grep-confirmed) but an engage-seeded EventMap with four cast arms —
-- under the retraction lens (CreateLuaEvent is a real bridge) the two
-- ungated victim-casts port.
-- Entries (BattlegroundIC.h:33-34): NPC_HIGH_COMMANDER_HALFORD_WYRMBANE
-- = 34924 (Alliance boss), NPC_OVERLORD_AGMAR = 34922 (Horde boss).
-- Both bosses share this one AI (the C++ Reset() entry-branches on
-- 34924 to pick the guard list). The C++ class derives ScriptedAI
-- (NOT BossAI), so no Go RegisterLuaBoss is needed — pure lua
-- creature-event registration (the lich_king adds precedent).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnSpawn (Reset re-arms the same seeds — C++ Reset() only resets
-- the EventMap, so the lua cancelTimers on 2/4/23 covers it).

local ENTRY_HALFORD_WYRMBANE = 34924
local ENTRY_OVERLORD_AGMAR = 34922
local ENTRY_KOR_KRON_GUARD = 34918 -- BattlegroundIC.h:35 (Horde guard)
local ENTRY_SEVENTH_LEGION_INFANTRY = 34919 -- BattlegroundIC.h:36 (Alliance guard)

local SPELL_BRUTAL_STRIKE = 58460 -- BossSpells :34
local SPELL_CRUSHING_LEAP = 68506 -- BossSpells :36
local SPELL_DAGGER_THROW  = 67280 -- BossSpells :25
local SPELL_RAGE          = 66776 -- BossSpells :27

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

-- C++ SelectTarget(SelectTargetMethod::Random, 1) — the bridged,
-- threat-ordered GetAITargets list (lua_creature_events.go:345);
-- offset 1 skips the first (MaxThreat-ordered) entry, i.e. the current
-- tank; the pick runs over the REST of the threat list — players,
-- pets, totems (playerOnly defaults FALSE, UnitAI.h:170).
local function randomNonTankThreatTarget(creature)
    local targets = creature:GetAITargets()
    if #targets <= 1 then
        return nil
    end
    return targets[1 + math.random(#targets - 1)]
end

-- C++ :90-99 EVENT_BRUTAL_STRIKE: DoCastVictim(SPELL_BRUTAL_STRIKE)
-- 2-arg (non-triggered) — CastSpell(nil, spell) is the victim-cast
-- convention; init 5000ms, re-arm 5000ms. No phase/machine/
-- random-target gates.
local function onBrutalStrike(creature, guid)
    creature:CastSpell(nil, SPELL_BRUTAL_STRIKE)
    schedule(guid, "brutal_strike", 5000, function() onBrutalStrike(creature, guid) end)
end

-- C++ :104-107 EVENT_CRUSHING_LEAP: DoCastVictim(SPELL_CRUSHING_LEAP)
-- 2-arg (non-triggered); init 15000ms, re-arm 25000ms (asymmetric —
-- kept C++-exact). No gates.
local function onCrushingLeap(creature, guid)
    creature:CastSpell(nil, SPELL_CRUSHING_LEAP)
    schedule(guid, "crushing_leap", 25000, function() onCrushingLeap(creature, guid) end)
end

-- C++ :100-103 EVENT_DAGGER_THROW: if (Unit* target =
-- SelectTarget(Random, 1)) DoCast(target, SPELL_DAGGER_THROW), 2-arg
-- (non-triggered); init 7s / unconditional re-arm 7s (re-arms even on
-- nil target — the rage_winterchill precedent).
local function onDaggerThrow(creature, guid)
    local target = randomNonTankThreatTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_DAGGER_THROW)
    end
    schedule(guid, "dagger_throw", 7000, function() onDaggerThrow(creature, guid) end)
end

-- C++ :108-113 EVENT_CHECK_RANGE: init 1s / unconditional re-arm 1s;
-- home-distance > 25.0f -> DoCast(me, SPELL_RAGE 66776) else
-- RemoveAurasDueToSpell(66776), 2-arg (non-triggered). The distance gate
-- is C++ me->GetDistance(me->GetHomePosition()) — WorldObject::GetDistance
-- (Object.cpp:1175-1179) subtracts the boss's own combat reach from the
-- exact 3D distance (clamped >= 0), so the Lua compares
-- exactDist - creature:GetCombatReach() > 25.0 (the clamp is unobservable:
-- 25.0 > 0, so dist > 25.0 is false exactly when clamped-dist > 25.0f is).
-- The 2026-10-21 header note claiming "no home-position accessor" was stale:
-- creature:GetHomePosition is bridged at lua_creature_events.go:1241;
-- creature:GetCombatReach at lua_creature_events.go (Object.cpp:1175 twin).
local function onCheckRange(creature, guid)
    local hx, hy, hz = creature:GetHomePosition()
    local dx = creature:GetX() - hx
    local dy = creature:GetY() - hy
    local dz = creature:GetZ() - hz
    local dist = math.sqrt(dx * dx + dy * dy + dz * dz) - creature:GetCombatReach()
    if dist > 25.0 then
        creature:CastSpell(creature, SPELL_RAGE)
    else
        creature:RemoveAura(SPELL_RAGE)
    end
    schedule(guid, "check_range", 1000, function() onCheckRange(creature, guid) end)
end

-- C++ boss_ioc_horde_alliance::JustEngagedWith (:72-78): the EventMap
-- seeds — BRUTAL_STRIKE / CRUSHING_LEAP / DAGGER_THROW / CHECK_RANGE
-- port (below); SpellHit + Reset guard-respawn legs stay out.
local function iocBossJustEngagedWith(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "brutal_strike", 5000, function() onBrutalStrike(creature, guid) end)
    schedule(guid, "crushing_leap", 15000, function() onCrushingLeap(creature, guid) end)
    schedule(guid, "dagger_throw", 7000, function() onDaggerThrow(creature, guid) end)
    schedule(guid, "check_range", 1000, function() onCheckRange(creature, guid) end)
end

-- C++ Reset() (:52-63): _events.Reset() + entry-gated guard respawn
-- (GetCreatureListWithEntryInGrid + Respawn, 100.0f) - PORTED 2026-10-24
-- in iocBossReset (creature:Respawn bridge, lua_creature_events.go;
-- the stale "only Lua Respawn is the GameObject hide/unhide" note is
-- retracted - that Respawn lives on the GO surface only). C++ calls
-- Respawn unconditionally; the bridge no-ops on live guards, matching
-- the C++ DEAD-gated restore block. LeaveCombat/JustDied carry only
-- the timer wipe (maiden convention).
local function iocBossLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function iocBossReset(event, creature)
    cancelTimers(creature:GetGUID())
    local guardEntry = ENTRY_SEVENTH_LEGION_INFANTRY
    if creature:GetEntry() ~= ENTRY_HALFORD_WYRMBANE then
        guardEntry = ENTRY_KOR_KRON_GUARD
    end
    for _, guard in ipairs(creature:GetCreaturesInGrid(guardEntry, 100)) do
        guard:Respawn()
    end
end

local function iocBossDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

-- Documented-only arms (no bridge):
-- EVENT_DAGGER_THROW (:100-103): PORTED above via the threat-list
-- pool (randomNonTankThreatTarget — offset 1 skips the MaxThreat
-- head, the netherspite/curator offset-1 precedent generalized to
-- the whole threat list since playerOnly defaults FALSE).
-- EVENT_CHECK_RANGE (:108-113): PORTED above (onCheckRange) via the
-- handler-surface GetHomePosition bridge + GetCombatReach bridge — the
-- "no home-position accessor" rationale recorded here 2026-10-21 is stale
-- (lua_creature_events.go:1241), and the C++ compare is on the
-- reach-adjusted distance (Object.cpp:1175-1179).
-- SpellHit (:80-88): unitCaster->IsVehicle() -> Unit::Kill(me, unitCaster)
-- — stale "no-SpellHit bridge" retracted (2026-10-21): ev14
-- CREATURE_EVENT_ON_HIT_BY_SPELL IS bridged
-- (engine/scripting/runtime.go:227; fired boss_ai.go:2125/2134,
-- spells.go:11765); arm stays out: caster->IsVehicle() has no Lua
-- bridge (no IsVehicle on the unit method set) and Unit::Kill(me,
-- unitCaster) has no Lua bridge — the vehicle-caster gate can't be
-- reproduced, so no port.
-- UpdateAI UNIT_STATE_CASTING early-out — unmodeled (maiden
-- precedent). DoMeleeAttackIfReady — engine-side.

RegisterCreatureEvent(ENTRY_HALFORD_WYRMBANE, 1, iocBossJustEngagedWith)
RegisterCreatureEvent(ENTRY_HALFORD_WYRMBANE, 2, iocBossLeaveCombat)
RegisterCreatureEvent(ENTRY_HALFORD_WYRMBANE, 4, iocBossDied)
RegisterCreatureEvent(ENTRY_HALFORD_WYRMBANE, 23, iocBossReset)
RegisterCreatureEvent(ENTRY_OVERLORD_AGMAR, 1, iocBossJustEngagedWith)
RegisterCreatureEvent(ENTRY_OVERLORD_AGMAR, 2, iocBossLeaveCombat)
RegisterCreatureEvent(ENTRY_OVERLORD_AGMAR, 4, iocBossDied)
RegisterCreatureEvent(ENTRY_OVERLORD_AGMAR, 23, iocBossReset)
