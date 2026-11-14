-- Argent Valiant (Icecrown) --
-- Lua port of src/server/scripts/Northrend/zone_icecrown.cpp
-- (npc_argent_valiantAI — ctor spawn arm, UpdateAI combat rotation,
-- MovementInform faction flip, DamageTaken duel-end machine).
-- Entry: 33448 'Argent Valiant' (TDB-verified, ScriptName
-- npc_argent_valiant; summon-only via the Aspirant's Challenge,
-- zero static spawns).
-- C++ ctor (:47-51): Initialize() (charge 7s / shield-breaker 10s)
-- + MovePoint(0, 8599.258, 963.951, 547.553) + SetFaction(FRIENDLY 35).
-- C++ UpdateAI (:88-111): charge timer 7s -> DoCastVictim(63010),
-- shield-breaker timer 10s -> DoCastVictim(65147); melee is engine.
-- C++ Reset (:62-65): Initialize() only — re-seeds both timers.
-- C++ MovementInform (:67-73): POINT_MOTION_TYPE arrival ->
-- SetFaction(FACTION_MONSTER 14).
-- C++ DamageTaken (:75-86): lethal damage (uiDamage > GetHealth)
-- from a player -> damage = 0, the player self-casts
-- SPELL_KILL_CREDIT 63049 (triggered), SetFaction(FACTION_FRIENDLY
-- 35), DespawnOrUnsummon(5s), SetHomePosition(current X/Y/Z/O),
-- EnterEvadeMode().
-- 63010/65147 are NOT in the seed Spell.dbc yet, so the rotation
-- casts are live-dead until a seed lands (hyjal precedent: port
-- the shape, seeding still open); the timers + faction machine run.
-- Eluna creature events: 5 OnSpawn, 1 OnEnterCombat, 23 OnReset,
-- 28 OnMovementInform, 2 OnLeaveCombat, 4 OnDied, 9 OnDamageTaken
-- — fireCreatureDamageTaken threads the handler's returns[1] back
-- as the new damage (dummy.lua `return true, 0` precedent). C++
-- DamageTaken fires pre-death on the killing blow, so the ev9
-- rewrite (damage = 0) reproduces it — no postDamage > 0 guard
-- (that guard is for UpdateAI HealthBelowPct arms, not DamageTaken).
-- DespawnOrUnsummon(5s) ports as a one-shot CreateLuaEvent 5000ms
-- -> Despawn() (creature:Despawn takes no delay arg).
-- Evade order (engine triggerCreatureEvade, creaturemotion.go):
-- ev23 fires BEFORE ev2, so the Reset re-arm lands first and the
-- leave-combat drain follows; a re-engage re-seeds via ev1, matching
-- C++ (timers never fire without a victim: UpdateAI returns early
-- on !UpdateVictim). Death also fires ev23 before ev4, drained the
-- same way.
-- Bridges: player:CastSpell lua_player.go:3501 (cocoon precedent);
-- SetFaction lua_creature_events.go:6698; SetHomePosition :7048;
-- GetX/Y/Z/GetO via the object.go generic fallback (:196, GetO =
-- Orientation field); creature:GetHealth object.go:203;
-- EnterEvadeMode :5624; attacker:GetTypeId() == 4 TYPEID_PLAYER
-- (lecraft precedent); creature:MoveTo is the MovePoint twin
-- (:5769, fires ev28 POINT_MOTION_TYPE on arrival); GetVictim
-- (ayamiss precedent).

local ARGENT_VALIANT_ENTRY = 33448
local SPELL_CHARGE = 63010
local SPELL_SHIELD_BREAKER = 65147
local SPELL_KILL_CREDIT = 63049
local FACTION_FRIENDLY = 35 -- SharedDefines.h:231
local FACTION_MONSTER = 14 -- SharedDefines.h:226
local POINT_MOTION_TYPE = 8 -- MovementDefines.h:31 (kalecgos precedent)
local CHARGE_INTERVAL = 7000
local SHIELD_BREAKER_INTERVAL = 10000

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

-- C++ UpdateAI charge leg (:91-96): DoCastVictim(63010), re-arm 7s.
local function onCharge(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CHARGE)
    end
    schedule(guid, "charge", CHARGE_INTERVAL, function() onCharge(creature, guid) end)
end

-- C++ UpdateAI shield-breaker leg (:98-103): DoCastVictim(65147),
-- re-arm 10s.
local function onShieldBreaker(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SHIELD_BREAKER)
    end
    schedule(guid, "shield_breaker", SHIELD_BREAKER_INTERVAL, function() onShieldBreaker(creature, guid) end)
end

-- C++ Reset (:62-65) = Initialize(): re-seed both timers at the
-- ctor values; also the engage seed (C++ UpdateAI runs the timers
-- from the ctor-initialized values).
local function seedTimers(creature, guid)
    cancelTimers(guid)
    schedule(guid, "charge", CHARGE_INTERVAL, function() onCharge(creature, guid) end)
    schedule(guid, "shield_breaker", SHIELD_BREAKER_INTERVAL, function() onShieldBreaker(creature, guid) end)
end

-- C++ ctor (:47-51): MovePoint + SetFaction(FRIENDLY).
local function onSpawn(event, creature)
    creature:MoveTo(0, 8599.258, 963.951, 547.553)
    creature:SetFaction(FACTION_FRIENDLY)
end

local function onEnterCombat(event, creature)
    seedTimers(creature, creature:GetGUIDLow())
end

local function onReset(event, creature)
    seedTimers(creature, creature:GetGUIDLow())
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

local function onDied(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ MovementInform (:67-73): POINT_MOTION_TYPE arrival ->
-- SetFaction(FACTION_MONSTER).
local function onMovementInform(event, creature, moveType, pointId)
    if moveType == POINT_MOTION_TYPE then
        creature:SetFaction(FACTION_MONSTER)
    end
end

local function onDamageTaken(event, creature, attacker, damage)
    if damage > creature:GetHealth() and attacker and attacker:GetTypeId() == 4 then
        attacker:CastSpell(attacker, SPELL_KILL_CREDIT, true)
        creature:SetFaction(FACTION_FRIENDLY)
        CreateLuaEvent(function()
            creature:Despawn()
        end, 5000)
        creature:SetHomePosition(creature:GetX(), creature:GetY(), creature:GetZ(), creature:GetO())
        creature:EnterEvadeMode()
        return true, 0
    end
end

RegisterCreatureEvent(ARGENT_VALIANT_ENTRY, 5, onSpawn)
RegisterCreatureEvent(ARGENT_VALIANT_ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ARGENT_VALIANT_ENTRY, 23, onReset)
RegisterCreatureEvent(ARGENT_VALIANT_ENTRY, 28, onMovementInform)
RegisterCreatureEvent(ARGENT_VALIANT_ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ARGENT_VALIANT_ENTRY, 4, onDied)
RegisterCreatureEvent(ARGENT_VALIANT_ENTRY, 9, onDamageTaken)
