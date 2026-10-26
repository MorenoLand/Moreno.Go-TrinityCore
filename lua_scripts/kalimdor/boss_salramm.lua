-- Culling of Stratholme: Salramm the Fleshcrafter — Lua port of
-- src/server/scripts/Kalimdor/CavernsOfTime/CullingOfStratholme/
-- boss_salramm_the_fleshcrafter.cpp (166 lines; class boss_salramm :
-- public CreatureScript { boss_salrammAI : public BossAI(creature,
-- DATA_SALRAMM) } + class spell_salramm_steal_flesh : public AuraScript;
-- GetAI via GetCullingOfStratholmeAI<boss_salrammAI> (CoSScriptName
-- "instance_culling_of_stratholme" gate); AddSC_boss_salramm at end
-- registers both; kalimdor loader decl 50 / call 163 per
-- kalimdor_script_loader.cpp — the fourth "// CoT Culling Of
-- Stratholme" loader group, right after AddSC_boss_infinite_corruptor()).
-- Whole-server-tree quoted-name grep confirms the cpp as the sole source
-- of "boss_salramm" and "spell_salramm_steal_flesh" (loader decl/call
-- lines only otherwise). Entry: instance_culling_of_stratholme.cpp:74
-- names NPC_SALRAMM = 26530 and :557 has
-- instance->SummonCreature(NPC_SALRAMM, spawnLocation.SpawnPoints[0]) in
-- the WAVE_SALRAMM wave-machine leg — the name-to-entry tie is
-- C++-verified at summon strength (not GUID-bound in OnCreatureCreate,
-- like aeonus/deja/temporus). BossAI's ctor leg DATA_SALRAMM is
-- culling_of_stratholme.h:117. Eluna creature events: 1 OnEnterCombat,
-- 2 OnLeaveCombat, 3 OnKilledUnit, 4 OnDied, 23 OnReset. Timers via
-- CreateLuaEvent; melee is engine-driven in Go (creature combat tick),
-- like C++ DoMeleeAttackIfReady.
-- Ported arms (the self-contained in-combat legs; instance legs below):
-- - Engage (C++ JustEngagedWith): Talk SAY_AGGRO (0) + arm each timer at
--   its C++ ScheduleEvent cooldown ({19s,24s} / 2s / {25s,35s} /
--   40s heroic-only); the BossAI::JustEngagedWith instance leg is
--   blocked (standing).
-- - Curse of Twisted Flesh (EVENT_CURSE_FLESH, heroic-only):
--   DoCastVictim(SPELL_CURSE_OF_TWISTED_FLESH 58845), non-triggered,
--   init 40s -> repeat 37s (C++ Repeat unconditional, nil victim ->
--   no cast), scheduled only under creature:IsHeroic() (C++-exact;
--   stale "no difficulty/heroic bridge" claim retracted — temporus
--   SPELL_REFLECTION / deja ATTRACTION precedent).
-- - Summon Ghouls (EVENT_SUMMON_GHOULS): Talk SAY_SUMMON_GHOULS (6) +
--   DoCastAOE(SPELL_SUMMON_GHOULS 52451), non-triggered (DoCastAOE
--   resolves to self-cast — kazrogal/illidan precedent), init
--   {19s,24s}; C++ does not re-arm here — the re-arm arrives from the
--   EVENT_EXPLODE_GHOUL2 fallthrough (+4s) below.
-- - Explode Ghoul (EVENT_EXPLODE_GHOUL1): Talk SAY_EXPLODE_GHOUL (4) +
--   DoCastAOE(SPELL_EXPLODE_GHOUL 52480, triggered) — aku_mai
--   CastSpell(self, spell, true) convention; scheduled {20s,24s} after
--   each summon wave (no re-arm of itself).
-- - EVENT_EXPLODE_GHOUL2 (C++ [[fallthrough]] into EXPLODE_GHOUL1):
--   schedule EVENT_SUMMON_GHOULS +4s, then Talk SAY_EXPLODE_GHOUL (4) +
--   triggered self-cast SPELL_EXPLODE_GHOUL — scheduled {25s,29s} after
--   each summon wave.
-- - Shadow Bolt 57725 on SelectTarget(Random, 0, 40.0f, true) ->
--   random alive threat-list player within 40m, non-triggered
--   (C++ DoCast), init 2s -> repeat 3s; C++ Repeat is unconditional
--   even on an empty target (kazrogal precedent), so the Lua re-arm is
--   too (nil target -> no cast).
-- - Steal Flesh 52708 on SelectTarget(Random, 1, 50.0f, true) ->
--   random threat-list player within 50m with the current victim
--   skipped BEFORE the player/dist filters (offset-before-filter,
--   UnitAI.h), non-triggered (C++ DoCast), Talk SAY_STEAL_FLESH (5) fires
--   regardless of target (C++ Talks before the SelectTarget), init
--   {25s,35s} -> repeat {15s,20s} unconditional (nil target -> no
--   cast).
-- - KilledUnit (event 3, wired — nalorakk DISCOVERY holds): C++ gates on
--   victim->GetTypeId() == TYPEID_PLAYER before Talk SAY_SLAY (2)
--   (terestian_illhoof victim:GetObjectType() == "Player" convention).
-- - Death (C++ JustDied): Talk SAY_DEATH (3) + cancel; the _JustDied()
--   and instance->SetData(DATA_NOTIFY_DEATH, 1) legs are blocked
--   (standing).
-- Unmodeled (documented-only, no bridges):
-- - InitializeAI: Talk(SAY_SPAWN) (1) — no spawn/appear bridge in the
--   Lua API (arthas JustAppeared precedent); instance->GetBossState
--   (DATA_SALRAMM) == DONE -> RemoveLootMode(LOOT_MODE_DEFAULT) — no
--   instance-data bridge (epoch InitializeAI precedent).
-- - BossAI::JustEngagedWith / BossAI::_Reset / BossAI::_JustDied
--   instance legs — blocked (standing).
-- - spell_salramm_steal_flesh (AuraScript): HandlePeriodic on
--   SPELL_AURA_PERIODIC_DUMMY EFFECT_0 — GetCaster()->CastSpell
--   (caster, SPELL_STEAL_FLESH_BUFF 52712, triggered) + GetCaster()->
--   CastSpell(target, SPELL_STEAL_FLESH_DEBUFF 52711, triggered) —
--   AuraScript check/periodic handlers not modeled (standing queue;
--   the arthas crusader_strike SpellScript precedent); documented, not
--   registered. SPELL_STEAL_FLESH 52708's spell data drives it.
-- - SAY_AGGRO (0) / SAY_SPAWN (1) / SAY_SLAY (2) / SAY_DEATH (3) /
--   SAY_EXPLODE_GHOUL (4) / SAY_STEAL_FLESH (5) /
--   SAY_SUMMON_GHOULS (6) are all the Talked lines; SAY_SPAWN fires
--   in InitializeAI only (unmodeled).

local ENTRY = 26530

local SAY_AGGRO        = 0
local SAY_SLAY         = 2
local SAY_DEATH        = 3
local SAY_EXPLODE_GHOUL = 4
local SAY_STEAL_FLESH  = 5
local SAY_SUMMON_GHOULS = 6

local SPELL_SUMMON_GHOULS = 52451
local SPELL_EXPLODE_GHOUL = 52480
local SPELL_SHADOW_BOLT   = 57725
local SPELL_STEAL_FLESH   = 52708
local SPELL_CURSE_OF_TWISTED_FLESH = 58845

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
-- player/dist filters (offset-before-filter: C++ builds the list
-- victim-first, so the offset pop removes the victim; Go's
-- threat-sorted list needs the victim-GUID exclusion). Dead
-- members are offline refs in C++ (skipped at list build) so they are
-- filtered here. The old instance-player pool is retired
-- (threat-list precedent, Oct-25 strips).
local function randomThreatPlayerInRange(creature, maxDist, skipFirst)
    local targets = creature:GetAITargets()
    local victimGuid = nil
    if skipFirst then
        local victim = creature:GetVictim()
        if victim ~= nil then
            victimGuid = victim:GetGUID()
        end
    end
    local pool = {}
    local aliveSeen = 0
    for _, t in ipairs(targets) do
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
                and creature:GetDistance(t) - creature:GetCombatReach() - 1.5 < maxDist then
            pool[#pool + 1] = t
        end
    end
    if #pool == 0 then
        return nil
    end
    return pool[math.random(#pool)]
end

-- C++ EVENT_EXPLODE_GHOUL1: Talk SAY_EXPLODE_GHOUL + DoCastAOE
-- (SPELL_EXPLODE_GHOUL, triggered) — aku_mai CastSpell(self, spell,
-- true) convention; fired from both ghoul-event arms (ghoul2 falls
-- through to it in C++).
local function onExplodeGhoul(creature)
    creature:Talk(SAY_EXPLODE_GHOUL)
    creature:CastSpell(creature, SPELL_EXPLODE_GHOUL, true)
end

-- C++ EVENT_SUMMON_GHOULS: Talk SAY_SUMMON_GHOULS + DoCastAOE
-- (SPELL_SUMMON_GHOULS), non-triggered, self-cast (DoCastAOE precedent);
-- then schedules the two explode arms. No self re-arm in C++ — the
-- re-arm is the ghoul2 fallthrough's +4s schedule.
local function onSummonGhouls(creature, guid)
    creature:Talk(SAY_SUMMON_GHOULS)
    creature:CastSpell(creature, SPELL_SUMMON_GHOULS)
    schedule(guid, "explodeghoul1", math.random(20000, 24000), function()
        onExplodeGhoul(creature)
    end)
    schedule(guid, "explodeghoul2", math.random(25000, 29000), function()
        -- C++ EVENT_EXPLODE_GHOUL2: schedules the next summon wave +4s
        -- then [[fallthrough]] into the ghoul1 Talk + triggered cast.
        schedule(guid, "summonghouls", 4000, function()
            onSummonGhouls(creature, guid)
        end)
        onExplodeGhoul(creature)
    end)
end

-- C++ EVENT_SHADOW_BOLT: DoCast(SelectTarget(Random, 0, 40.0f, true),
-- 57725), non-triggered, init 2s -> repeat 3s (re-arm unconditional in
-- C++, even on empty target — kazrogal precedent).
local function onShadowBolt(creature, guid)
    local target = randomThreatPlayerInRange(creature, 40, false)
    if target then
        creature:CastSpell(target, SPELL_SHADOW_BOLT)
    end
    schedule(guid, "shadowbolt", 3000, function()
        onShadowBolt(creature, guid)
    end)
end

-- C++ EVENT_STEAL_FLESH: Talk SAY_STEAL_FLESH (fires regardless of
-- target — C++ Talks before SelectTarget) + DoCast(SelectTarget(Random,
-- 1, 50.0f, true), 52708), non-triggered, init {25s,35s} ->
-- repeat {15s,20s} unconditional (nil target -> no cast).
local function onStealFlesh(creature, guid)
    creature:Talk(SAY_STEAL_FLESH)
    local target = randomThreatPlayerInRange(creature, 50, true)
    if target then
        creature:CastSpell(target, SPELL_STEAL_FLESH)
    end
    schedule(guid, "stealflesh", math.random(15000, 20000), function()
        onStealFlesh(creature, guid)
    end)
end

-- C++ EVENT_CURSE_FLESH: DoCastVictim(SPELL_CURSE_OF_TWISTED_FLESH),
-- non-triggered; Repeat(37s) unconditional.
local function onCurseFlesh(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CURSE_OF_TWISTED_FLESH)
    end
    schedule(guid, "curseflesh", 37000, function()
        onCurseFlesh(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "summonghouls", math.random(19000, 24000), function()
        onSummonGhouls(creature, guid)
    end)
    schedule(guid, "shadowbolt", 2000, function()
        onShadowBolt(creature, guid)
    end)
    schedule(guid, "stealflesh", math.random(25000, 35000), function()
        onStealFlesh(creature, guid)
    end)
    -- C++: if (IsHeroic()) ScheduleEvent(EVENT_CURSE_FLESH, 40s).
    if creature:IsHeroic() then
        schedule(guid, "curseflesh", 40000, function()
            onCurseFlesh(creature, guid)
        end)
    end
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 3, function(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)
