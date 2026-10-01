-- Culling of Stratholme: Meathook — Lua port of
-- src/server/scripts/Kalimdor/CavernsOfTime/CullingOfStratholme/
-- boss_meathook.cpp (127 lines; class boss_meathook :
-- public CreatureScript { boss_meathookAI : public BossAI(creature,
-- DATA_MEATHOOK) }; GetAI via GetCullingOfStratholmeAI<boss_meathookAI>
-- (CoSScriptName "instance_culling_of_stratholme" gate);
-- AddSC_boss_meathook at end registers the one script;
-- kalimdor loader decl 52 / call 165 per kalimdor_script_loader.cpp —
-- the sixth "// CoT Culling Of Stratholme" loader group, right after
-- AddSC_boss_mal_ganis()). Whole-server-tree quoted-name grep confirms
-- the cpp as the sole source of "boss_meathook" (loader decl/call
-- lines only otherwise). Entry: instance_culling_of_stratholme.cpp:73
-- names NPC_MEATHOOK = 26529 and :553 has
-- instance->SummonCreature(NPC_MEATHOOK, spawnLocation.SpawnPoints[0])
-- in the WAVE_MEATHOOK wave-machine leg — summon-strength tie (not
-- GUID-bound in OnCreatureCreate, like salramm/aeonus); BossAI ctor
-- leg DATA_MEATHOOK is culling_of_stratholme.h:116;
-- creature_template ScriptName binding stays DB-side. Eluna creature
-- events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnKilledUnit, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (the self-contained in-combat legs; instance legs below):
-- - Engage (C++ JustEngagedWith): Talk SAY_AGGRO (0) + arm each timer
--   at its C++ ScheduleEvent cooldown ({7s,11s} / 2s / {13s,17s});
--   the BossAI::JustEngagedWith instance leg is blocked (standing).
-- - Constricting Chains (EVENT_CHAIN): SelectTarget(Random, 0, -20.0f,
--   true) -> random alive player at least 20yd away (C++'s negative
--   dist is a minimum-distance filter — UnitAI.h/UnitAI.cpp document
--   "dist: if 0: ignored, if > 0: maximum distance, if < 0: minimum
--   distance"; IsWithinCombatRange approximated by GetDistance here)
--   else SelectTarget(Random, 1, 100.0f, true) -> random alive player
--   within 100m excluding the victim (azgalor position-1 convention —
--   no threat-list bridge) else DoCastVictim; DoCast non-triggered
--   (52720-era salramm DoCast convention); init {7s,11s} ->
--   repeat {10s,15s} unconditional (C++ Repeat sits outside the
--   if/else, nil target -> victim cast).
-- - Disease Expulsion (EVENT_DISEASE): DoCastAOE(SPELL_DISEASE_EXPULSION
--   52666), non-triggered (DoCastAOE resolves to self-cast —
--   kazrogal/illidan precedent), init 2s -> repeat 3.5s.
-- - Frenzy (EVENT_FRENZY): DoCast(me, SPELL_FRENZY 58841),
--   non-triggered self-cast, init {13s,17s} -> repeat {13s,17s}.
-- - KilledUnit (event 3, wired — nalorakk DISCOVERY holds): C++ gates
--   on victim->GetTypeId() == TYPEID_PLAYER before Talk SAY_SLAY (1)
--   (terestian_illhoof victim:GetObjectType() == "Player"
--   convention).
-- - Death (C++ JustDied): Talk SAY_DEATH (3) + cancel; the _JustDied()
--   and instance->SetData(DATA_NOTIFY_DEATH, 1) legs are blocked
--   (standing).
-- Unmodeled (documented-only, no bridges):
-- - InitializeAI's Talk(SAY_SPAWN) (2) — no spawn/appear bridge in the
--   Lua API (arthas JustAppeared precedent).
-- - InitializeAI's GetBossState(DATA_MEATHOOK) == DONE ->
--   RemoveLootMode(LOOT_MODE_DEFAULT) — no instance-data bridge
--   (epoch InitializeAI precedent).
-- - BossAI::JustEngagedWith / BossAI::_JustDied instance legs —
--   blocked (standing).

local ENTRY = 26529

local SAY_AGGRO = 0
local SAY_SLAY  = 1
local SAY_DEATH = 3

local SPELL_CONSTRICTING_CHAINS = 52696
local SPELL_DISEASE_EXPULSION   = 52666
local SPELL_FRENZY              = 58841

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
    per[key] = CreateLuaEvent(fn, delay, 1)
end

-- nefarian convention: random alive player in the instance within
-- maxDist; minDist models C++'s negative SelectTarget dist ("at least
-- -dist yards away" per UnitAI.h — IsWithinCombatRange approximated by
-- GetDistance here); excludeVictim approximates C++ position 1
-- ("never on tank") where no threat-list bridge exists (azgalor Doom
-- precedent).
local function randomPlayer(creature, minDist, maxDist, excludeVictim)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local victim = creature:GetVictim()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        local d = creature:GetDistance(p)
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() and d >= minDist
                and (not maxDist or d <= maxDist)
                and not (excludeVictim and victim and p == victim) then
            found[#found + 1] = p
        end
    end
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
end

-- C++ EVENT_CHAIN: DoCast(SelectTarget(Random, 0, -20.0f, true),
-- 52696) else DoCast(SelectTarget(Random, 1, 100.0f, true), 52696)
-- else DoCastVictim(52696), non-triggered; init {7s,11s} ->
-- repeat {10s,15s} unconditional (C++ Repeat sits outside the
-- if/else; nil target -> victim cast).
local function onChain(creature, guid)
    local target = randomPlayer(creature, 20, nil, false)
    if not target then
        target = randomPlayer(creature, 0, 100, true)
    end
    if target then
        creature:CastSpell(target, SPELL_CONSTRICTING_CHAINS)
    else
        local victim = creature:GetVictim()
        if victim then
            creature:CastSpell(victim, SPELL_CONSTRICTING_CHAINS)
        end
    end
    schedule(guid, "chain", math.random(10000, 15000), function()
        onChain(creature, guid)
    end)
end

-- C++ EVENT_DISEASE: DoCastAOE(52666), non-triggered, self-cast
-- (kazrogal/illidan DoCastAOE precedent); init 2s -> repeat 3.5s.
local function onDisease(creature, guid)
    creature:CastSpell(creature, SPELL_DISEASE_EXPULSION)
    schedule(guid, "disease", 3500, function()
        onDisease(creature, guid)
    end)
end

-- C++ EVENT_FRENZY: DoCast(me, 58841), non-triggered self-cast;
-- init {13s,17s} -> repeat {13s,17s}.
local function onFrenzy(creature, guid)
    creature:CastSpell(creature, SPELL_FRENZY)
    schedule(guid, "frenzy", math.random(13000, 17000), function()
        onFrenzy(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "chain", math.random(7000, 11000), function()
        onChain(creature, guid)
    end)
    schedule(guid, "disease", 2000, function()
        onDisease(creature, guid)
    end)
    schedule(guid, "frenzy", math.random(13000, 17000), function()
        onFrenzy(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 3, function(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end)
RegisterCreatureEvent(ENTRY, 4, function(event, creature, killer)
    creature:Talk(SAY_DEATH)
    cancelTimers(creature:GetGUID())
end)
RegisterCreatureEvent(ENTRY, 23, onReset)
