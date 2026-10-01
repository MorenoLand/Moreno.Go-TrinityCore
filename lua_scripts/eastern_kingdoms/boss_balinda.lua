-- Balinda Stonehearth (Alterac Valley) --
-- Lua port of src/server/scripts/EasternKingdoms/AlteracValley/
-- boss_balinda.cpp (boss_balindaAI — combat scheduler + DamageTaken
-- iceblock latch). Boss-script unit per eastern_kingdoms_script_loader.cpp
-- order (boss_balinda follows AddSC_alterac_valley; the Alterac Valley
-- boss set continues with drekthar, galvangar, vanndar).
-- Entry (verifiable from the C++ sources): 11949 "Captain Balinda
-- Stonehearth" in the BattlegroundAV.h AV creature lists (lines 1269 and
-- 1301, both commented "Captain Balinda Stonehearth"; the neighboring
-- 11946/11947/11948 entries are Drek'Thar, Galvangar, Vanndar — the other
-- AV boss set members). The creature_template ScriptName binding is
-- DB-side (no TDB in this workspace), but the entry itself is
-- C++-verifiable so the port is registered (storm_cloud /
-- av_marshal precedent).
-- Eluna creature events: 5 OnSpawn, 1 OnEnterCombat, 2 OnLeaveCombat,
-- 4 OnDied, 9 OnDamageTaken, 23 OnReset. Driven by CreateLuaEvent
-- timers; melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
-- Ported arms (C++ JustEngagedWith event schedule + UpdateAI event
-- machine + DamageTaken latch):
-- EVENT_ARCANE_EXPLOSION: victim-cast ARCANE_EXPLOSION 46608,
--   {5,15}s init -> {5,15}s;
-- EVENT_CONE_OF_COLD: victim-cast CONE_OF_COLD 38384, 8s init ->
--   {10,20}s;
-- EVENT_FIREBOLT (casts SPELL_FIREBALL 46988): victim-cast, 1s init ->
--   {5,9}s;
-- EVENT_FROSTBOLT: victim-cast FROSTBOLT 46987, 4s init -> {4,12}s;
-- JustEngagedWith Talk(SAY_AGGRO=0);
-- DamageTaken: HealthBelowPctDamaged(40, damage) one-shot latch
-- (HasCastIceblock) -> self-cast ICEBLOCK 46604 (moroes damage-taken
-- convention: (GetHealth() - damage) * 100 / maxHealth < 40).
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (creature casts
-- are packet-visual), so timers fire unconditionally and the in-loop
-- casting-state re-check is dropped (maiden precedent).
-- Unmodeled: EVENT_SUMMON_WATER_ELEMENTAL (3s init -> 50s; summons the
-- water elemental when summons.empty()) + JustSummoned
-- (AttackStart(SelectTarget(Random)) + SetFaction + GUID store) +
-- SummonedCreatureDespawn + JustDied summons.DespawnAll — no summon
-- model on the Lua surface (no SummonCreature/AttackStart/SetFaction
-- bridges — phoenix precedent); the elemental never enters the world,
-- so its timer chain is documented-only rather than a no-op dead loop.
-- Unmodeled: EVENT_CHECK_RESET (home-position 2D distance > 50yd ->
-- EnterEvadeMode + Talk(SAY_EVADE=1), plus the elemental's leash arm) —
-- no HomePosition bridge on the luaMotionCreature surface (X/Y/Z reads
-- only; GetDistance2d needs two objects — av_marshal precedent); evade
-- still re-arms through OnReset(23) when the engine leashes the
-- creature. Unmodeled: DoAction(ACTION_BUFF_YELL=-30001) -> Talk(0) —
-- driven by the BattlegroundAV script (BattlegroundAV.cpp), and there is
-- no Battleground-to-Lua DoAction dispatch bridge. SAY_SALVATION=2 is
-- declared in the C++ Texts enum but never used by this AI.

local SPELL_ARCANE_EXPLOSION = 46608
local SPELL_CONE_OF_COLD = 38384
local SPELL_FIREBALL = 46988
local SPELL_FROSTBOLT = 46987
local SPELL_ICEBLOCK = 46604

local SAY_AGGRO = 0

local ENTRY_BALINDA = 11949

local balindaState = {}
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
    per[key] = CreateLuaEvent(fn, delay)
end

local function initState(guid)
    balindaState[guid] = { iceblock = false }
end

local function onArcaneExplosion(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_ARCANE_EXPLOSION)
    end
    schedule(guid, "arcane_explosion", {5000, 15000}, function() onArcaneExplosion(creature, guid) end)
end

local function onConeOfCold(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CONE_OF_COLD)
    end
    schedule(guid, "cone_of_cold", {10000, 20000}, function() onConeOfCold(creature, guid) end)
end

local function onFireball(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FIREBALL)
    end
    schedule(guid, "fireball", {5000, 9000}, function() onFireball(creature, guid) end)
end

local function onFrostbolt(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FROSTBOLT)
    end
    schedule(guid, "frostbolt", {4000, 12000}, function() onFrostbolt(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    initState(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "arcane_explosion", {5000, 15000}, function() onArcaneExplosion(creature, guid) end)
    schedule(guid, "cone_of_cold", 8000, function() onConeOfCold(creature, guid) end)
    schedule(guid, "fireball", 1000, function() onFireball(creature, guid) end)
    schedule(guid, "frostbolt", 4000, function() onFrostbolt(creature, guid) end)
end

local function onDamageTaken(_, creature, _, damage)
    local guid = creature:GetGUID()
    local state = balindaState[guid]
    if state == nil or state.iceblock then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if (creature:GetHealth() - damage) * 100 / maxHealth < 40 then
        creature:CastSpell(creature, SPELL_ICEBLOCK)
        state.iceblock = true
    end
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

local function onSpawn(_, creature)
    initState(creature:GetGUID())
end

local function onReset(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    initState(guid)
end

RegisterCreatureEvent(ENTRY_BALINDA, 5, onSpawn)
RegisterCreatureEvent(ENTRY_BALINDA, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_BALINDA, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_BALINDA, 4, onCombatEnd)
RegisterCreatureEvent(ENTRY_BALINDA, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY_BALINDA, 23, onReset)
