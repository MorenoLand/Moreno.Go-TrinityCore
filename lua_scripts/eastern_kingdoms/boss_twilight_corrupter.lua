-- Twilight Corrupter (Duskwood) --
-- Lua port of src/server/scripts/EasternKingdoms/zone_duskwood.cpp
-- (boss_twilight_corrupterAI — JustEngagedWith schedule + UpdateAI
-- combat rotation + KilledUnit kill-counter arms only). Zone-script
-- unit per eastern_kingdoms_script_loader.cpp order (blasted_lands
-- done; duskwood: at_twilight_grove documented-only, boss_twilight_
-- corrupter ported).
-- Entry (TwilightCorrupter enum, verifiable from the C++ sources):
-- 15625 (NPC_TWILIGHT_CORRUPTER). The creature_template ScriptName
-- binding is DB-side (no TDB in this workspace).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat,
-- 3 OnTargetDied, 4 OnDied, 23 OnReset (the C++ overrides no
-- SpellHit / DamageTaken / MovementInform arms — no events 9/14).
-- Timers via a 1s CreateLuaEvent pump (a port of UpdateAI's combat
-- section in C++ arm order — 1s granularity exact for all C++
-- timers; the !UpdateVictim early return collapses into the pump,
-- which only runs in combat). The C++ HasUnitState(UNIT_STATE_
-- CASTING) pump-skip has no state bridge (azuregos precedent) —
-- unmodeled.
-- C++ DoCast default is triggered=false (vaelastrasz convention):
-- creature:CastSpell(nil, spell) = DoCastVictim; creature:
-- CastSpell(creature, spell) = self-cast.
-- Fight shape (C++-exact for all modeled arms):
-- OnEnterCombat(1): per-GUID latch of the C++ JustEngagedWith
-- schedule — Talk(YELL_TWILIGHT_CORRUPTOR_AGGRO 1), soul
-- corruption 15s, creature of nightmare 30s — + 1s scheduler pump
-- (events.Reset in the C++ latch — the pump is cancelled first,
-- gargolmar precedent). KillCount = 0 on every engage (the C++
-- Initialize call lands in the constructor and Reset, both of
-- which the per-GUID latch covers).
-- Pump (C++ UpdateAI arm order): soul-corruption arm — the C++
-- DoCastAOE(SPELL_SOUL_CORRUPTION) is CreatureAI::DoCastAOE =
-- DoCast(nullptr, spellId) (UnitAI.h:322), so creature:
-- CastSpell(creature, 25805) is C++-exact; re-arm 15-19s
-- (urand(15s, 19s), C++-exact); creature-of-nightmare arm — the
-- C++ SelectTarget(Random, 0, 0.0f, true) has no target-selection
-- bridge — the cast lands on the victim (nil target, omen
-- precedent; the playerOnly flag is a documented degradation),
-- non-triggered DoCast(target, SPELL_CREATURE_OF_NIGHTMARE
-- 25806), re-arm 45s (C++-exact). Melee is engine-driven.
-- OnTargetDied(3): the C++ KilledUnit victim->GetTypeId() ==
-- TYPEID_PLAYER gate is C++-exact via victim:GetTypeId() == 4
-- (TYPEID_PLAYER = 4; object.go:objectTypeID). On a player kill:
-- KillCount++, Talk(YELL_TWILIGHT_CORRUPTOR_KILL 2) — the C++
-- Talk(textId, victim) targets the victim; the Go Talk bridge
-- takes a textId only (broadcast to nearby, C++-exact for the
-- line played) — documented deviation; at 3 kills DoCast(me,
-- SPELL_LEVEL_UP 24312, true) — the triggered flag has no
-- distinct bridge (castCreatureSpell broadcasts the same path
-- either way, boss_ai.go:445) — documented deviation — then
-- KillCount = 0.
-- OnReset(23): full per-GUID state drop (C++ Reset: _events.
-- Reset() + KillCount = 0 — C++-exact). OnLeaveCombat(2) and
-- OnDied(4): cancel the pump + drop per-GUID state (gargolmar /
-- omen precedent; the C++ Reset path covers both).
-- at_twilight_grove — the quest-8735-INCOMPLETE area trigger
-- (FindNearestCreature(15625, 500yd) skip + SummonCreature of
-- the corrupter at (-10328.16, -489.57, 49.95) + Talk respawn
-- yell) has no AreaTriggerScript dispatch hook anywhere in the
-- model (felwood at_ancient_leaf precedent) and sits behind the
-- no-quest-status / no-world-search / no-summon bridges —
-- documented-only.
-- Zone set status: zone_duskwood.cpp closed (at_twilight_grove
-- — area-trigger summon machine documented-only; boss_twilight_
-- corrupter — JustEngagedWith / combat rotation / KilledUnit
-- arms ported for 15625, all C++-exact except: the
-- creature-of-nightmare random-target pick degrades to the
-- victim, the kill-yell loses its victim targeting, the level-
-- up cast's triggered flag collapses to the standard cast
-- path, and the UNIT_STATE_CASTING pump-skip is unmodeled).

local SPELL_SOUL_CORRUPTION = 25805
local SPELL_CREATURE_OF_NIGHTMARE = 25806
local SPELL_LEVEL_UP = 24312

local TWILIGHT_CORRUPTER_ENTRY = 15625

local YELL_RESPAWN = 0
local YELL_AGGRO = 1
local YELL_KILL = 2

local TYPEID_PLAYER = 4

local corrupterState = {}
local corrupterPump = {}

local TIMER_KEYS = { "soulCorruptionTimer", "nightmareTimer" }

local function cancelPump(guid)
    local id = corrupterPump[guid]
    if id then
        RemoveEventById(id)
        corrupterPump[guid] = nil
    end
end

local function fullReset(guid)
    cancelPump(guid)
    corrupterState[guid] = nil
end

-- C++ UpdateAI combat section in arm order — 1s granularity
-- exact for all C++ timers; the !UpdateVictim early return
-- collapses into the pump (it only runs in combat). Each
-- tick decrements every timer (the EventMap::Update(diff)
-- pass), then expired events fire in C++ arm order.
local function combatTick(creature, guid)
    local st = corrupterState[guid]
    if not st then
        return
    end

    for _, key in ipairs(TIMER_KEYS) do
        st[key] = st[key] - 1000
    end

    -- Soul corruption arm — C++ DoCastAOE = DoCast(nullptr,
    -- spellId) (UnitAI.h:322): self-cast is C++-exact. Re-arm
    -- 15-19s (C++ urand, C++-exact).
    if st.soulCorruptionTimer <= 0 then
        creature:CastSpell(creature, SPELL_SOUL_CORRUPTION)
        st.soulCorruptionTimer = 15000 + math.random(0, 4000)
    end

    -- Creature of nightmare arm — non-triggered DoCast(target,
    -- SPELL_CREATURE_OF_NIGHTMARE), re-arm 45s (C++-exact).
    -- The C++ SelectTarget(Random, 0, 0.0f, true) has no
    -- target-selection bridge — the cast lands on the victim
    -- (nil target, omen precedent).
    if st.nightmareTimer <= 0 then
        creature:CastSpell(nil, SPELL_CREATURE_OF_NIGHTMARE)
        st.nightmareTimer = 45000
    end
end

-- C++ JustEngagedWith schedule values land on OnEnterCombat:
-- Talk(YELL_AGGRO), soul corruption 15s, creature of nightmare
-- 30s (events.Reset in the C++ latch — the pump is cancelled
-- first, gargolmar precedent); KillCount = 0 per engage (the
-- C++ Initialize arm).
local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelPump(guid)
    creature:Talk(YELL_AGGRO)
    corrupterState[guid] = {
        killCount = 0,
        soulCorruptionTimer = 15000,
        nightmareTimer = 30000,
    }
    corrupterPump[guid] = CreateLuaEvent(function()
        combatTick(creature, guid)
    end, 1000, 0)
end

-- C++ KilledUnit: victim->GetTypeId() == TYPEID_PLAYER gates
-- the whole arm — victim:GetTypeId() == 4 is C++-exact
-- (object.go:objectTypeID). KillCount++ + Talk(YELL_KILL);
-- the C++ Talk(textId, victim) victim targeting has no bridge
-- (Talk takes a textId only) — the line still plays nearby.
-- At 3 kills: DoCast(me, SPELL_LEVEL_UP, true) — the triggered
-- flag collapses to the standard cast path (boss_ai.go:445) —
-- then KillCount = 0. Out-of-combat kills find no pump state
-- and are ignored, C++-equivalent: the next OnEnterCombat
-- re-latches the JustEngagedWith schedule anyway.
local function onTargetDied(_, creature, victim)
    if not victim or victim:GetTypeId() ~= TYPEID_PLAYER then
        return
    end
    local guid = creature:GetGUID()
    local st = corrupterState[guid]
    if not st then
        return
    end
    st.killCount = st.killCount + 1
    creature:Talk(YELL_KILL)
    if st.killCount == 3 then
        creature:CastSpell(creature, SPELL_LEVEL_UP)
        st.killCount = 0
    end
end

-- C++ Reset(): _events.Reset() + KillCount = 0 — a per-GUID
-- state drop is C++-exact. The C++ death path runs Reset too,
-- so OnDied lands here as well (gargolmar / omen precedent).
local function onReset(_, creature)
    fullReset(creature:GetGUID())
end

RegisterCreatureEvent(TWILIGHT_CORRUPTER_ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(TWILIGHT_CORRUPTER_ENTRY, 2, onReset)
RegisterCreatureEvent(TWILIGHT_CORRUPTER_ENTRY, 3, onTargetDied)
RegisterCreatureEvent(TWILIGHT_CORRUPTER_ENTRY, 4, onReset)
RegisterCreatureEvent(TWILIGHT_CORRUPTER_ENTRY, 23, onReset)
