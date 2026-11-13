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
-- (HasCastIceblock) -> self-cast ICEBLOCK 46604 (int64(health)-damage
-- < floor(maxHealth*40/100)).
-- Cast-state gates PORTED: the C++ UpdateAI pre-loop gate
-- (boss_balinda.cpp:131) + in-loop gate (:174) ride the fire-time 500ms
-- IsCasting defer (mal_ganis idiom adapted to timer arms; IsCasting is
-- live, lua_creature_events.go:1950): a firing arm that finds the
-- creature casting re-schedules itself after 500ms and consumes no
-- re-arm, so the arm stays due; the check-reset arm defers the same way
-- (it sits inside the gated C++ loop). The DamageTaken iceblock latch is
-- a separate hook and stays ungated per C++. The in-loop gate's
-- same-tick co-expiry suppression has no timer-arm model (independent
-- timers fire on separate callbacks) and is the only unmodeled sliver.
-- PORTED 2026-11-13 (strip 12 rowsB): EVENT_SUMMON_WATER_ELEMENTAL
-- (3s init -> 50s; summons the water elemental when summons.empty())
-- + JustSummoned (AttackStart(SelectTarget(Random)) + SetFaction +
-- GUID store) + SummonedCreatureDespawn + JustDied summons.DespawnAll.
-- The 2026-10-01 stay-out ("no engine spell-summon data footprint")
-- is stale: creatureEffectSummonType (boss_ai.go:4866, 2026-10-23)
-- runs effect-28 summons on the creature-cast path and fires ev19
-- via summonTempCreature (broggok precedent). The summons.empty()
-- gate rides a per-GUID elementalGUID flag, cleared on ev20 (despawn)
-- and ev21 (killed; Balinda has no C++ SummonedCreatureDies override,
-- the base removes the dead summon) and on combat-end/reset; the
-- DespawnAll twin despawns the elemental on ev2/ev4/ev23 (BossAI
-- _Reset/_JustDied, ScriptedCreature.cpp:499-516).
-- EVENT_CHECK_RESET gate + Talk(SAY_EVADE=1) PORTED 2026-10-24
-- (GetHomePosition + GetX/Y math minus GetCombatReach twins
-- GetDistance2d; Talk once-latched per engagement); the self
-- EnterEvadeMode leg and the elemental leash leg are PORTED
-- 2026-11-13 (strip 12 rowsB) — EnterEvadeMode is bridged
-- (lua_creature_events.go:5519); the elemental leg measures the
-- elemental's plain 2D distance from Balinda's home, no reach
-- subtraction, C++-exact (boss_balinda.cpp:165-168).
-- Unmodeled: DoAction(ACTION_BUFF_YELL=-30001) -> Talk(0) —
-- driven by the BattlegroundAV script (BattlegroundAV.cpp), and there is
-- no Battleground-to-Lua DoAction dispatch bridge. SAY_SALVATION=2 is
-- declared in the C++ Texts enum but never used by this AI.

local SPELL_ARCANE_EXPLOSION = 46608
local SPELL_CONE_OF_COLD = 38384
local SPELL_FIREBALL = 46988
local SPELL_FROSTBOLT = 46987
local SPELL_SUMMON_WATER_ELEMENTAL = 45067
local SPELL_ICEBLOCK = 46604

local SAY_AGGRO = 0
local SAY_EVADE = 1

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
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

local function initState(guid)
    balindaState[guid] = { iceblock = false, evadeTalked = false, elementalGUID = nil }
end

-- C++ BossAI::_Reset/_JustDied summons.DespawnAll twin
-- (ScriptedCreature.cpp:499-516): despawn the tracked elemental, if
-- any, and clear the summons.empty() gate flag. Safe to call when no
-- elemental is tracked or it already despawned (GetCreatureByGUID
-- returns nil).
local function despawnElemental(creature, guid)
    local state = balindaState[guid]
    if state == nil or state.elementalGUID == nil then
        return
    end
    local elemental = creature:GetCreatureByGUID(state.elementalGUID)
    if elemental then
        elemental:Despawn()
    end
    state.elementalGUID = nil
end

local function onArcaneExplosion(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "arcane_explosion", 500, function() onArcaneExplosion(creature, guid) end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_ARCANE_EXPLOSION)
    end
    schedule(guid, "arcane_explosion", {5000, 15000}, function() onArcaneExplosion(creature, guid) end)
end

local function onConeOfCold(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "cone_of_cold", 500, function() onConeOfCold(creature, guid) end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CONE_OF_COLD)
    end
    schedule(guid, "cone_of_cold", {10000, 20000}, function() onConeOfCold(creature, guid) end)
end

local function onFireball(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "fireball", 500, function() onFireball(creature, guid) end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FIREBALL)
    end
    schedule(guid, "fireball", {5000, 9000}, function() onFireball(creature, guid) end)
end

local function onFrostbolt(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "frostbolt", 500, function() onFrostbolt(creature, guid) end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FROSTBOLT)
    end
    schedule(guid, "frostbolt", {4000, 12000}, function() onFrostbolt(creature, guid) end)
end

-- C++ EVENT_SUMMON_WATER_ELEMENTAL (boss_balinda.cpp:154-157): 3s
-- init -> 50s; DoCast(SPELL_SUMMON_WATER_ELEMENTAL 45067) only when
-- summons.empty() (the elementalGUID flag below), re-arm 50s
-- unconditional. The cast's SPELL_EFFECT_SUMMON leg spawns the
-- elemental through the creature cast path's EffectSummonType twin,
-- which fires event 19 — the JustSummoned arm is ported in
-- onJustSummoned below (broggok precedent).
local function onSummonWaterElemental(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "summon_water_elemental", 500, function() onSummonWaterElemental(creature, guid) end)
        return
    end
    local state = balindaState[guid]
    if state == nil or state.elementalGUID == nil then
        creature:CastSpell(creature, SPELL_SUMMON_WATER_ELEMENTAL)
    end
    schedule(guid, "summon_water_elemental", 50000, function() onSummonWaterElemental(creature, guid) end)
end

-- C++ EVENT_CHECK_RESET (boss_balinda.cpp:159-168): 2D distance from
-- home > 50 yd -> EnterEvadeMode + Talk(SAY_EVADE); the elemental's own
-- 2D-from-Balinda-home check -> elemental evade; 5s re-arm unconditional.
-- Distance twin: GetHomePosition + GetX/Y math, minus GetCombatReach
-- (WorldObject::GetDistance2d(x, y), Object.cpp:1193-1197), clamped >= 0.
-- Talk is once-latched per engagement (C++ evades at once, so the yell
-- fires once); both EnterEvadeMode legs are PORTED 2026-11-13 (strip 12
-- rowsB) — the bridge is live (lua_creature_events.go:5519); the
-- elemental leg below measures plain 2D, no reach subtraction.
local function onCheckReset(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "check_reset", 500, function() onCheckReset(creature, guid) end)
        return
    end
    local state = balindaState[guid]
    if state ~= nil and not state.evadeTalked then
        local hx, hy = creature:GetHomePosition()
        local dx = creature:GetX() - hx
        local dy = creature:GetY() - hy
        local dist = math.sqrt(dx * dx + dy * dy) - creature:GetCombatReach()
        if dist < 0 then dist = 0 end
        if dist > 50.0 then
            -- C++ EVENT_CHECK_RESET (boss_balinda.cpp:159-163):
            -- EnterEvadeMode + Talk(SAY_EVADE), C++ order. The evade
            -- fires the ev2/ev23 handlers (timers cancelled, state
            -- re-initialized), so return without re-arming — C++
            -- _Reset clears the event map and the arm re-arms only on
            -- the next engage.
            creature:EnterEvadeMode()
            creature:Talk(SAY_EVADE)
            state.evadeTalked = true
            return
        end
    end
    -- C++ EVENT_CHECK_RESET elemental leg (boss_balinda.cpp:165-168):
    -- the elemental evades when ITS plain 2D distance from Balinda's
    -- home exceeds 50 (no combat-reach subtraction — C++ GetDistance2d).
    -- Checked every tick, independent of the self latch above.
    if state ~= nil and state.elementalGUID ~= nil then
        local elemental = creature:GetCreatureByGUID(state.elementalGUID)
        if elemental then
            local hx, hy = creature:GetHomePosition()
            local dx = elemental:GetX() - hx
            local dy = elemental:GetY() - hy
            if math.sqrt(dx * dx + dy * dy) > 50.0 then
                elemental:EnterEvadeMode()
            end
        end
    end
    schedule(guid, "check_reset", 5000, function() onCheckReset(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    initState(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "arcane_explosion", {5000, 15000}, function() onArcaneExplosion(creature, guid) end)
    schedule(guid, "cone_of_cold", 8000, function() onConeOfCold(creature, guid) end)
    schedule(guid, "fireball", 1000, function() onFireball(creature, guid) end)
    schedule(guid, "frostbolt", 4000, function() onFrostbolt(creature, guid) end)
    schedule(guid, "summon_water_elemental", 3000, function() onSummonWaterElemental(creature, guid) end)
    schedule(guid, "check_reset", 5000, function() onCheckReset(creature, guid) end)
end

-- C++ boss_balindaAI::JustSummoned (boss_balinda.cpp:91-96), C++
-- order: AttackStart(SelectTarget(Random, 0, 50, true)) — a random
-- hostile alive player within 50yd (GetPlayersInRange(50, 1) +
-- the kiljaeden random-pick idiom; no target -> no engage, like the
-- C++ null-SelectTarget leg) — then SetFaction(me->GetFaction()),
-- then the GUID store (the summons.Summon list has no bridge; the
-- elementalGUID flag is its twin). Balinda summons nothing else, so
-- no entry check is needed.
local function onJustSummoned(_, creature, summon)
    local guid = creature:GetGUIDLow()
    local state = balindaState[guid]
    if state == nil then
        initState(guid)
        state = balindaState[guid]
    end
    local players = summon:GetPlayersInRange(50, 1)
    if #players > 0 then
        summon:AttackStart(players[math.random(1, #players)])
    end
    summon:SetFaction(creature:GetFaction())
    state.elementalGUID = summon:GetGUIDLow()
end

-- C++ SummonedCreatureDespawn (boss_balinda.cpp:98-101) and the
-- SummonedCreatureDies base leg share one Lua twin: the summon left
-- the list (timed despawn, ev20, or killed, ev21), so the
-- summons.empty() gate re-opens. Balinda summons nothing else, so
-- the GUID match is the whole gate — no entry check needed.
local function clearElemental(_, creature, summon)
    local state = balindaState[creature:GetGUIDLow()]
    if state ~= nil and state.elementalGUID == summon:GetGUIDLow() then
        state.elementalGUID = nil
    end
end

local function onDamageTaken(_, creature, _, damage)
    local guid = creature:GetGUIDLow()
    local state = balindaState[guid]
    if state == nil or state.iceblock then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if creature:GetHealth() - damage < math.floor(maxHealth * 40 / 100) then
        creature:CastSpell(creature, SPELL_ICEBLOCK)
        state.iceblock = true
    end
end

local function onCombatEnd(_, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    despawnElemental(creature, guid)
end

local function onSpawn(_, creature)
    initState(creature:GetGUIDLow())
end

local function onReset(_, creature)
    local guid = creature:GetGUIDLow()
    despawnElemental(creature, guid)
    cancelTimers(guid)
    initState(guid)
end

RegisterCreatureEvent(ENTRY_BALINDA, 5, onSpawn)
RegisterCreatureEvent(ENTRY_BALINDA, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_BALINDA, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_BALINDA, 4, onCombatEnd)
RegisterCreatureEvent(ENTRY_BALINDA, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY_BALINDA, 19, onJustSummoned)
RegisterCreatureEvent(ENTRY_BALINDA, 20, clearElemental)
RegisterCreatureEvent(ENTRY_BALINDA, 21, clearElemental)
RegisterCreatureEvent(ENTRY_BALINDA, 23, onReset)
