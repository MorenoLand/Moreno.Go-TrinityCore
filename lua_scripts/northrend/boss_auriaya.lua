-- Auriaya (Ulduar) — Lua port of
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_auriaya.cpp
-- (boss_auriaya (CreatureScript) via
-- RegisterUlduarCreatureAI<boss_auriaya> (BossAI), BOSS_AURIAYA = 6
-- (ulduar.h:42); registered from inside AddSC_boss_auriaya();
-- loader decl 110 / call 305 per northrend_script_loader.cpp —
-- first group of the "// Ulduar" block, after
-- AddSC_halls_of_stone() (decl 108 / call 303); the call after is
-- AddSC_boss_flame_leviathan() (decl 111 / call 306)).
-- Entry: 33515 Auriaya (ulduar.h NPC_AURIAYA, line 76 —
-- entry-verifiable; the CreatureScript ScriptName binding is
-- DB-side as usual).
-- Talk ids (AuriayaYells, boss_auriaya.cpp:93-97, verified this
-- run): SAY_AGGRO 0, SAY_SLAY 1, SAY_BERSERK 2, EMOTE_FEAR 3,
-- EMOTE_DEFENDER 4.
-- Sole-source verified: whole-server-tree grep for "boss_auriaya"
-- hits boss_auriaya.cpp (+ ulduar.h NPC_AURIAYA and the loader
-- decl/call for AddSC_boss_auriaya) only; zero sql/ hits.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timer-driven casts ride the
-- CreateLuaEvent convention (baltharus / maiden / sartharion
-- precedent): module-local GUID-keyed timers table, cancelTimers
-- on events 2/4/23, schedule(guid, key, delayMs, fn);
-- creature:CastSpell(nil, spell) = DoCastVictim (2-arg,
-- non-triggered), creature:CastSpell(creature, spell) =
-- DoCastSelf / DoCastAOE, third arg true = triggered,
-- {minMs, maxMs} = randtime re-arm. Melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms; line refs are
-- boss_auriaya.cpp):
-- JustEngagedWith (:160-170) — Talk(SAY_AGGRO 0) + the five
-- bridgeable timer legs below (event 1; the
-- BossAI::JustEngagedWith passthrough and the SendEncounterUnit
-- engage leg have no bridges — the boss_bookkeeping precedent).
-- EVENT_SONIC_SCREECH (:234-236) — DoCastVictim
-- (SPELL_SONIC_SCREECH 64422) 2-arg non-triggered, 48s init
-- (:165), re-arm {22s,30s}.
-- EVENT_TERRIFYING_SCREECH (:238-242) — Talk(EMOTE_FEAR 3) +
-- DoCastSelf(SPELL_TERRIFYING_SCREECH 64386) 2-arg non-triggered
-- + EVENT_BLAST in 1ms (:241), 38s init (:166), re-arm {36s,45s}.
-- EVENT_BLAST (:244-245) — DoCastAOE(SPELL_SENTINEL_BLAST 64389)
-- lowers to self-cast (non-triggered), fires 1ms after
-- TERRIFYING_SCREECH, no re-arm.
-- EVENT_SUMMON_DEFENDER (:247-250) — Talk(EMOTE_DEFENDER 4) +
-- DoCastSelf(SPELL_DEFENDER_TRIGGER 64448) 2-arg non-triggered +
-- EVENT_ACTIVATE_DEFENDER in 2s (:250), 60s init (:167), no re-arm.
-- EVENT_ACTIVATE_DEFENDER (:252-253) — DoCastSelf
-- (SPELL_ACTIVATE_DEFENDER 64449) 2-arg non-triggered, fires 2s
-- after SUMMON_DEFENDER, no re-arm.
-- EVENT_SWARMING_GUARDIAN (:255-258) — SelectTarget(Random, 1,
-- 0.0f, true) -> randomThreatPlayerSkipFirst (UnitAI.h: skip the
-- first MaxThreat-order entry BEFORE the player/alive filter, then
-- random over threat-list players alive, 0.0f unlimited),
-- nil-guarded pick, DoCast 2-arg non-triggered
-- (SPELL_SUMMON_SWARMING_GUARDIAN 64396), re-arm {25s,45s}
-- unconditional, 51s init (:168); the old instance-player pool
-- (randomNonVictimPlayer) misread the draw — C++ SelectTarget
-- draws the threat list, not the instance (baltharus precedent).
-- EVENT_BERSERK (:260-262) — DoCastSelf(SPELL_BERSERK 47008,
-- TRIGGERED) + Talk(SAY_BERSERK 2), 600s init (:169), no re-arm.
-- KilledUnit (:172-176) — TYPEID_PLAYER gate AND roll_chance_i(50),
-- then Talk(SAY_SLAY 1) (event 3; victim nil-guard,
-- victim:GetObjectType() == "Player", math.random(1, 100) <= 50).
-- npc_sanctum_sentry (CreatureScript via
-- RegisterUlduarCreatureAI<npc_sanctum_sentry> (ScriptedAI);
-- NPC_SANCTUM_SENTRY = 34014, local Misc enum boss_auriaya.cpp:115
-- — entry-verifiable from C++, REGISTERED this run):
-- EVENT_RIP (:318-320) — DoCastVictim(SPELL_RIP_FLESH 64375) 2-arg
-- non-triggered, 6s init (:293), re-arm {10s,12s}. The
-- JustEngagedWith SetWalk(false) / Reset SetWalk(true) legs have
-- no movement bridge; the Reset one-shot self-buff
-- SPELL_STRENGHT_OF_THE_PACK 64369 (triggered, :287) is ported
-- below on event 23 (reset arm, not a timer); EVENT_SAVAGE_POUNCE
-- (:322-330) PORTED 2026-10-23 via the bridged creature:IsWithinLOS
-- (WorldObject::IsWithinLOSInMap twin); the CatsTargetSelector
-- random-target leg rides creature:GetAITargets (baltharus
-- ENERVATING_BRAND precedent); JustDied (:298)
-- -> instance GetCreature(BOSS_AURIAYA) -> DoAction
-- (ACTION_CRAZY_CAT_LADY 0) — no instance / DoAction bridges.
-- DOCUMENTED-ONLY (no bridgeable arms):
-- Reset (:139-145) / HandleCats (:147-158) — _Reset() + flag resets
-- + grid RESPAWN of entry 34014 within 500.0f — PORTED 2026-10-24
-- in auriayaReset (creature:Respawn bridge, lua_creature_events.go);
-- the HandleCats(false) despawn half is ported in auriayaDied;
-- stale "no grid bridge" retracted — creature:GetCreaturesInGrid is
-- bridged, lua_creature_events.go:1204.
-- DoAction (:178-193, ACTION_CRAZY_CAT_LADY 0 / ACTION_DEFENDER_DIED
-- 1) / GetData (:195-204, DATA_NINE_LIVES 30763077 /
-- DATA_CRAZY_CAT_LADY 30063007) — no DoAction / GetData bridges.
-- JustDied (:206-212) — DoPlaySoundToSet(15476) ported via
-- creature:PlayDirectSound (dedicated auriayaDied); HandleCats(false)
-- despawn leg ported there too (see above); the SendEncounterUnit
-- disengage + HandleCats(true) respawn legs have no
-- instance/grid bridges; no Talk arms.
-- EnterEvadeMode (:214-218) — SendEncounterUnit disengage + BossAI
-- leg — no bridge.
-- The UNIT_STATE_CASTING gates and the DoMeleeAttackIfReady tail
-- have no bridges.
-- npc_feral_defender — the whole machine is phase-gated
-- (PHASE_NONE/PHASE_COMBAT :111-112) and its timer legs
-- (EVENT_RUSH :389-393, EVENT_START_COMBAT :384, respawn chain
-- :395-415) ride CatsTargetSelector random-target selection; the
-- DamageTaken (:427-446) feign-death machinery — health (GetHealth/
-- GetMaxHealth) and aura (creature:HasAura) bridges exist, but the
-- unit stays blocked on unbridged SetAuraStack + no creature entry
-- (SetReactState/AttackStop/flag/threat/gravity bridges are live —
-- the stale list above is retracted) + random-target selection — NO
-- registration; JustDied (:448-453) DoAction(ACTION_DEFENDER_DIED)
-- has no DoAction bridge; no creature entry in C++ evidence —
-- NO registration.
-- npc_swarming_guardian — the 1s scheduler self-cast
-- SPELL_AGRO_CREATOR 63709 (:464-474) is timer-driven but has no
-- creature entry in C++ evidence — NO registration; the 63709
-- pounce leg needs target search anyway.
-- npc_seeping_essence_stalker — Reset one-shot self-cast
-- SPELL_SEEPING_ESSENCE 64458 (:494-499), not timer-driven, no
-- entry in C++ evidence — NO registration.
-- spell_auriaya_strenght_of_the_pack (64381),
-- spell_auriaya_sentinel_blast (64392 / 64679),
-- spell_auriaya_agro_creator (63709),
-- spell_auriaya_feral_essence_removal (64456),
-- spell_auriaya_feral_rush (64496 / 64674) — no SpellScript
-- bridge; spell_auriaya_random_agro_periodic (61906) — no
-- AuraScript bridge; achievement_nine_lives / achievement_crazy_cat_lady
-- — no achievement / GetData bridges; all join the unmodeled
-- queues.

local ENTRY_AURIAYA = 33515
local ENTRY_SANCTUM_SENTRY = 34014

local SAY_AGGRO      = 0
local SAY_SLAY       = 1
local SAY_BERSERK    = 2
local EMOTE_FEAR     = 3
local EMOTE_DEFENDER = 4

local SPELL_SONIC_SCREECH      = 64422
local SPELL_SENTINEL_BLAST     = 64389
local SPELL_TERRIFYING_SCREECH = 64386
local SPELL_DEFENDER_TRIGGER   = 64448
local SPELL_ACTIVATE_DEFENDER  = 64449
local SPELL_SUMMON_SWARMING_GUARDIAN = 64396
local SPELL_BERSERK            = 47008

local SOUND_AURIAYA_DEATH = 15476
local SPELL_RIP_FLESH          = 64375
local SPELL_SAVAGE_POUNCE        = 64666
local SPELL_STRENGHT_OF_THE_PACK = 64369

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

-- C++ SelectTarget(Random, 1, 0.0f, true) leg (UnitAI.h): skip the
-- first entry in MaxThreat order (the current victim heads the
-- list) BEFORE the player/alive filter, then random over the
-- remaining threat-list players alive (baltharus
-- randomThreatPlayerSkipFirst precedent; offline refs are skipped
-- at build in C++, dead = offline here).
local function randomThreatPlayerSkipFirst(creature)
    local targets = creature:GetAITargets()
    local candidates = {}
    for i = 2, #targets do
        local unit = targets[i]
        if unit:GetObjectType() == "Player" and not unit:IsDead() then
            candidates[#candidates + 1] = unit
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EVENT_SONIC_SCREECH (:234-236): DoCastVictim
-- (SPELL_SONIC_SCREECH 64422) (2-arg non-triggered), 48s init
-- (:165), re-arm {22s,30s}.
local function sonicScreechTick(creature, guid)
    creature:CastSpell(nil, SPELL_SONIC_SCREECH)
    schedule(guid, "sonic", {22000, 30000}, function()
        sonicScreechTick(creature, guid)
    end)
end

-- C++ EVENT_BLAST (:244-245): DoCastAOE(SPELL_SENTINEL_BLAST 64389)
-- lowers to self-cast creature:CastSpell(creature, spell)
-- (non-triggered); fires 1ms after EVENT_TERRIFYING_SCREECH, no
-- re-arm.
local function blastTick(creature)
    creature:CastSpell(creature, SPELL_SENTINEL_BLAST)
end

-- C++ EVENT_TERRIFYING_SCREECH (:238-242): Talk(EMOTE_FEAR 3) +
-- DoCastSelf(SPELL_TERRIFYING_SCREECH 64386) (2-arg non-triggered)
-- + EVENT_BLAST in 1ms (:241); 38s init (:166), re-arm {36s,45s}.
local function terrifyingScreechTick(creature, guid)
    creature:Talk(EMOTE_FEAR)
    creature:CastSpell(creature, SPELL_TERRIFYING_SCREECH)
    schedule(guid, "blast", 1, function()
        blastTick(creature)
    end)
    schedule(guid, "terrifying", {36000, 45000}, function()
        terrifyingScreechTick(creature, guid)
    end)
end

-- C++ EVENT_ACTIVATE_DEFENDER (:252-253): DoCastSelf
-- (SPELL_ACTIVATE_DEFENDER 64449) (2-arg non-triggered); fires 2s
-- after EVENT_SUMMON_DEFENDER, no re-arm.
local function activateDefenderTick(creature)
    creature:CastSpell(creature, SPELL_ACTIVATE_DEFENDER)
end

-- C++ EVENT_SUMMON_DEFENDER (:247-250): Talk(EMOTE_DEFENDER 4) +
-- DoCastSelf(SPELL_DEFENDER_TRIGGER 64448) (2-arg non-triggered)
-- + EVENT_ACTIVATE_DEFENDER in 2s (:250); 60s init (:167), no
-- re-arm.
local function summonDefenderTick(creature, guid)
    creature:Talk(EMOTE_DEFENDER)
    creature:CastSpell(creature, SPELL_DEFENDER_TRIGGER)
    schedule(guid, "activatedefender", 2000, function()
        activateDefenderTick(creature)
    end)
end

-- C++ EVENT_SWARMING_GUARDIAN (:255-258): SelectTarget(Random, 1,
-- 0.0f, true) -> randomThreatPlayerSkipFirst(creature), nil-guarded pick
-- (C++ `if` — skip cast on nil); DoCast 2-arg = non-triggered ->
-- CastSpell(target, SPELL_SUMMON_SWARMING_GUARDIAN 64396); re-arm
-- {25s,45s} unconditional (outside the target-if, C++-exact);
-- 51s init (:168). The summon is the spell's own effect (the
-- twilight-corrupter precedent); the old "needs target search, and
-- it is a summon leg; no bridges either way" claim was stale.
local function swarmingGuardianTick(creature, guid)
    local target = randomThreatPlayerSkipFirst(creature)
    if target then
        creature:CastSpell(target, SPELL_SUMMON_SWARMING_GUARDIAN)
    end
    schedule(guid, "swarming", {25000, 45000}, function()
        swarmingGuardianTick(creature, guid)
    end)
end

-- C++ EVENT_BERSERK (:260-262): DoCastSelf(SPELL_BERSERK 47008,
-- TRIGGERED) + Talk(SAY_BERSERK 2); 600s init (:169), no re-arm.
local function berserkTick(creature)
    creature:CastSpell(creature, SPELL_BERSERK, true)
    creature:Talk(SAY_BERSERK)
end

-- C++ JustEngagedWith (:160-170): BossAI passthrough + Talk(AGGRO)
-- + encounter-frame engage + the five ScheduleEvent legs — the
-- Talk and the four bridgeable timer legs are modeled here.
local function auriayaEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "sonic", 48000, function()
        sonicScreechTick(creature, guid)
    end)
    schedule(guid, "terrifying", 38000, function()
        terrifyingScreechTick(creature, guid)
    end)
    schedule(guid, "summondefender", 60000, function()
        summonDefenderTick(creature, guid)
    end)
    schedule(guid, "swarming", 51000, function()
        swarmingGuardianTick(creature, guid)
    end)
    schedule(guid, "berserk", 600000, function()
        berserkTick(creature)
    end)
end

-- C++ KilledUnit (:172-176): who->GetTypeId() == TYPEID_PLAYER AND
-- roll_chance_i(50), then Talk(SAY_SLAY) — victim nil-guard,
-- victim:GetObjectType() == "Player", math.random(1, 100) <= 50.
local function auriayaTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" and math.random(1, 100) <= 50 then
        creature:Talk(SAY_SLAY)
    end
end

-- Shared timer wipe for events 2/4/23 (auriaya JustDied :206-212
-- carries no Talk arms, so event 4 models only the wipe).
local function cancelCombatTimers(event, creature)
    cancelTimers(creature:GetGUID())
end

-- npc_sanctum_sentry — C++ EVENT_RIP (:318-320):
-- DoCastVictim(SPELL_RIP_FLESH 64375) (2-arg non-triggered), 6s
-- init (:293), re-arm {10s,12s}.
local function ripTick(creature, guid)
    creature:CastSpell(nil, SPELL_RIP_FLESH)
    schedule(guid, "rip", {10000, 12000}, function()
        ripTick(creature, guid)
    end)
end

-- npc_sanctum_sentry — C++ EVENT_SAVAGE_POUNCE (:322-330):
-- SelectTarget(Random, 0, CatsTargetSelector(me, 10.0f, 15.0f)) —
-- random threat-list player 10-15yd with LOS ->
-- DoCast(target, SPELL_SAVAGE_POUNCE 64666) (2-arg = non-triggered,
-- vaelastrasz convention); target found: 10s re-arm; none: 1s.
-- The LOS leg ports via the bridged creature:IsWithinLOS
-- (WorldObject::IsWithinLOSInMap twin).
local function savagePounceTick(creature, guid)
    local candidates = {}
    for _, unit in ipairs(creature:GetAITargets()) do
        if unit:GetObjectType() == "Player" and not unit:IsDead() then
            local d = creature:GetDistance(unit)
            if d >= 10.0 and d < 15.0 and creature:IsWithinLOS(unit) then
                candidates[#candidates + 1] = unit
            end
        end
    end
    if #candidates > 0 then
        creature:CastSpell(candidates[math.random(#candidates)], SPELL_SAVAGE_POUNCE)
        schedule(guid, "pounce", 10000, function()
            savagePounceTick(creature, guid)
        end)
    else
        schedule(guid, "pounce", 1000, function()
            savagePounceTick(creature, guid)
        end)
    end
end

-- C++ JustEngagedWith (:291-296): EVENT_RIP 6s +
-- EVENT_SAVAGE_POUNCE 1ms + SetWalk(false) — the SetWalk leg has
-- no movement bridge (separable).
local function sentryEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "rip", 6000, function()
        ripTick(creature, guid)
    end)
    schedule(guid, "pounce", 1, function()
        savagePounceTick(creature, guid)
    end)
end

-- npc_sanctum_sentry::Reset (:284-288): _events.Reset() (the
-- cancelCombatTimers arm) + DoCastSelf(SPELL_STRENGHT_OF_THE_PACK
-- 64369, triggered, 3rd arg true = C++-exact) — the
-- me->SetWalk(true) leg has no movement bridge (separable).
local function sentryReset(event, creature)
    cancelCombatTimers(event, creature)
    creature:CastSpell(creature, SPELL_STRENGHT_OF_THE_PACK, true)
end

-- C++ boss_auriayaAI::JustDied (:206-212): DoPlaySoundToSet(me,
-- AURIAYA_DEATH_SOUNDID 15476), ungated; HandleCats(false) despawn leg
-- ported below (stale "no grid bridge" retracted:
-- creature:GetCreaturesInGrid is bridged, lua_creature_events.go:1204;
-- entry 34014 registered, event 4 already wired). The HandleCats(true)
-- Respawn leg on Reset PORTED 2026-10-24 in auriayaReset; the
-- _JustDied() and SendEncounterUnit disengage legs have no instance bridges.
-- C++ boss_auriaya Reset (:139-145): _Reset() (instance state, no
-- bridge) + flags + HandleCats(true) (:147-158): every dead sanctum
-- sentry (34014) within 500.0f respawns. GetCreaturesInGrid enumerates
-- dead motions still in the map (no health filter); IsDead reads the
-- snapshot Health via the object.go fallback; creature:Respawn restores
-- full health at home and fires event 23 on the sentry, running its
-- Reset arm as C++ ai->Reset() does.
local function auriayaReset(event, creature)
    cancelCombatTimers(event, creature)
    for _, sentry in ipairs(creature:GetCreaturesInGrid(ENTRY_SANCTUM_SENTRY, 500)) do
        if sentry:IsDead() then
            sentry:Respawn()
        end
    end
end

local function auriayaDied(event, creature, killer)
    creature:PlayDirectSound(SOUND_AURIAYA_DEATH)
    cancelCombatTimers(event, creature)
    for _, sentry in ipairs(creature:GetCreaturesInGrid(ENTRY_SANCTUM_SENTRY, 500)) do
        sentry:Despawn()
    end
end

RegisterCreatureEvent(ENTRY_AURIAYA, 1, auriayaEnterCombat)
RegisterCreatureEvent(ENTRY_AURIAYA, 2, cancelCombatTimers)
RegisterCreatureEvent(ENTRY_AURIAYA, 3, auriayaTargetDied)
RegisterCreatureEvent(ENTRY_AURIAYA, 4, auriayaDied)
RegisterCreatureEvent(ENTRY_AURIAYA, 23, auriayaReset)
RegisterCreatureEvent(ENTRY_SANCTUM_SENTRY, 1, sentryEnterCombat)
RegisterCreatureEvent(ENTRY_SANCTUM_SENTRY, 2, cancelCombatTimers)
RegisterCreatureEvent(ENTRY_SANCTUM_SENTRY, 4, cancelCombatTimers)
RegisterCreatureEvent(ENTRY_SANCTUM_SENTRY, 23, sentryReset)
