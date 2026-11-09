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
-- BossAI::UpdateAI CASTING gates PORTED (ScriptedCreature.cpp:578/:584, thalnos precedent): each timer arm defers 500ms while creature:IsCasting(), no re-arm consumed.
-- Ported arms (the self-contained in-combat legs; instance legs below):
-- - Engage (C++ JustEngagedWith): Talk SAY_AGGRO (0) + arm each timer
--   at its C++ ScheduleEvent cooldown ({7s,11s} / 2s / {13s,17s});
--   BossAI::JustEngagedWith pulse + me-DoZone PORTED, and
--   SetBossState(DATA_MEATHOOK, IN_PROGRESS) PORTED 2026-11-06 on ev1
--   (Talk-then-base in boss_meathook.cpp:61-64,
--   ScriptedCreature.cpp:527-539; CheckRequiredBosses has no CoS
--   instance override, so the write is unconditional).
-- - Reset (BossAI::Reset -> _Reset, ScriptedCreature.h:328 /
--   ScriptedCreature.cpp:499-510): SetCombatPulseDelay(0) +
--   SetBossState(NOT_STARTED) unless already DONE, PORTED 2026-11-06
--   on ev23 (zarithrian idiom, incl. the !IsAlive early return).
-- - Death (C++ JustDied): Talk SAY_DEATH (3) + _JustDied() ->
--   SetBossState(DONE) PORTED 2026-11-06 on ev4 (C++ order: Talk,
--   _JustDied, SetData); the instance->SetData(DATA_NOTIFY_DEATH, 1)
--   leg stays blocked (no instance SetData bridge, standing).
-- - InitializeAI (C++ :55-60): Talk SAY_SPAWN (2) + GetBossState ==
--   DONE -> RemoveLootMode(LOOT_MODE_DEFAULT 1, SharedDefines.h:42),
--   both PORTED 2026-11-06 on ev5 (ev5 fires at the C++ JustAppeared
--   moments; the old "no spawn/appear bridge" note is retracted —
--   ev5 is live, lua_creature_events.go:6748).
-- - Constricting Chains (EVENT_CHAIN): SelectTarget(Random, 0, -20.0f,
--   true) -> random alive threat-list player at least 20yd away (C++'s
--   negative dist is a minimum-distance filter — UnitAI.h/UnitAI.cpp
--   document "dist: if 0: ignored, if > 0: maximum distance, if < 0:
--   minimum distance"; IsWithinCombatRange approximated by GetDistance
--   here) else SelectTarget(Random, 1, 100.0f, true) -> random alive
--   threat-list player within 100m with the MaxThreat head skipped
--   BEFORE the player/dist filters (offset-before-filter, UnitAI.h)
--   else DoCastVictim; DoCast non-triggered
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
-- Unmodeled (documented-only, no bridges):

local ENTRY = 26529

-- culling_of_stratholme.h:116-120 (EncounterState, InstanceScript.h:68-76:
-- NOT_STARTED 0, IN_PROGRESS 1, FAIL 2, DONE 3).
local DATA_MEATHOOK = 0
local LOOT_MODE_DEFAULT = 1 -- SharedDefines.h:42

local SAY_AGGRO = 0
local SAY_SLAY  = 1
local SAY_SPAWN = 2
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

-- C++ SelectTarget draws the threat list (UnitAI.h:217-218): position 0
-- takes any entry, position 1 skips the current victim BEFORE the
-- player/dist filters (offset-before-filter, svalna idiom); dead entries
-- are offline refs in C++ (skipped at list build) so they are filtered
-- here. minDist models C++'s negative SelectTarget dist ("at least
-- -dist yards away" per UnitAI.h — reach-exact IsWithinCombatRange twin
-- (Unit.cpp:583; player reach 1.5). The old instance-player pool + victim-GUID
-- exclusion are retired (threat-list precedent, Oct-25 strips).
local function randomThreatPlayer(creature, minDist, maxDist, skipFirst)
    local targets = creature:GetAITargets()
    local victimGuid = nil
    if skipFirst then
        local victim = creature:GetVictim()
        if victim ~= nil then
            victimGuid = victim:GetGUID()
        end
    end
    local found = {}
    local aliveSeen = 0
    for _, t in ipairs(targets) do
        local d = creature:GetDistance(t) - creature:GetCombatReach() - 1.5
        local skip = false
        if skipFirst then
            if victimGuid ~= nil then
                skip = t:GetGUID() == victimGuid
            elseif not t:IsDead() then
                aliveSeen = aliveSeen + 1
                skip = aliveSeen == 1
            end
        end
        if not skip
                and t:GetObjectType() == "Player" and not t:IsDead()
                and (minDist <= 0 or d >= minDist)
                and (not maxDist or d < maxDist) then
            found[#found + 1] = t
        end
    end
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
end

-- C++ EVENT_CHAIN: DoCast(SelectTarget(Random, 0, -20.0f, true),
-- 52696) else DoCast(SelectTarget(Random, 1, 100.0f, true), 52696)
-- else DoCastVictim(52696), non-triggered; both picks draw the threat
-- list (UnitAI.h), the position-1 pick skipping the MaxThreat head
-- before the filters; init {7s,11s} -> repeat {10s,15s} unconditional
-- (C++ Repeat sits outside the if/else; nil target -> victim cast).
local function onChain(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "chain", 500, function()
            onChain(creature, guid)
        end)
        return
    end
    local target = randomThreatPlayer(creature, 20, nil, false)
    if not target then
        target = randomThreatPlayer(creature, 0, 100, true)
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
    if creature:IsCasting() then
        schedule(guid, "disease", 500, function()
            onDisease(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_DISEASE_EXPULSION)
    schedule(guid, "disease", 3500, function()
        onDisease(creature, guid)
    end)
end

-- C++ EVENT_FRENZY: DoCast(me, 58841), non-triggered self-cast;
-- init {13s,17s} -> repeat {13s,17s}.
local function onFrenzy(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "frenzy", 500, function()
            onFrenzy(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_FRENZY)
    schedule(guid, "frenzy", math.random(13000, 17000), function()
        onFrenzy(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    -- C++ JustEngagedWith (boss_meathook.cpp:61-64): Talk, then
    -- BossAI::JustEngagedWith -> SetBossState(IN_PROGRESS) +
    -- SetCombatPulseDelay(5) + me-form DoZoneInCombat
    -- (ScriptedCreature.cpp:527-539).
    creature:SetBossState(DATA_MEATHOOK, 1) -- IN_PROGRESS
    creature:SetCombatPulseDelay(5) -- BossAI::JustEngagedWith arm (ScriptedCreature.cpp:540; Talk-then-base in boss_meathook.cpp:61-64)
    creature:DoZoneInCombat() -- BossAI::JustEngagedWith me-form arm (ScriptedCreature.cpp:542; instance/setActive/ScheduleTasks legs out)
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
    cancelTimers(creature:GetGUIDLow())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    if creature:IsDead() then
        return
    end
    -- C++ BossAI::Reset -> _Reset (ScriptedCreature.h:328 /
    -- ScriptedCreature.cpp:499-510): SetCombatPulseDelay(0), then
    -- SetBossState NOT_STARTED unless already DONE.
    creature:SetCombatPulseDelay(0)
    if creature:GetBossState(DATA_MEATHOOK) ~= 3 then
        creature:SetBossState(DATA_MEATHOOK, 0) -- NOT_STARTED
    end
end

-- C++ JustDied (boss_meathook.cpp:100-105): Talk SAY_DEATH, then
-- _JustDied() -> SetBossState(DONE), then the unbridged instance
-- SetData(DATA_NOTIFY_DEATH, 1).
local function onDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    creature:SetBossState(DATA_MEATHOOK, 3) -- DONE
    cancelTimers(creature:GetGUIDLow())
end

-- C++ InitializeAI (boss_meathook.cpp:55-60): Talk SAY_SPAWN +
-- RemoveLootMode when already DONE.
local function onSpawn(event, creature)
    creature:Talk(SAY_SPAWN)
    if creature:GetBossState(DATA_MEATHOOK) == 3 then
        creature:RemoveLootMode(LOOT_MODE_DEFAULT)
    end
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 3, function(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 5, onSpawn)
RegisterCreatureEvent(ENTRY, 23, onReset)
