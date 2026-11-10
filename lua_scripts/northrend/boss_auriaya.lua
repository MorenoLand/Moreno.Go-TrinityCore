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
-- BossAI::JustEngagedWith passthrough ports via SetBossState
-- (DATA_AURIAYA 6, IN_PROGRESS) + SetCombatPulseDelay
-- + DoZoneInCombat, and the SendEncounterUnit engage leg ports via
-- creature:SendEncounterUnit, bridge live 2026-11-05; the SetBossState
-- leg PORTED tick 258 via creature:SetBossState
-- (lua_creature_events.go:5917), base-first per C++ :162).
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
-- EVENT_SWARNING_GUARDIAN (:255-258) — SelectTarget(Random, 1,
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
-- (ACTION_CRAZY_CAT_LADY 0) — PORTED 2026-11-05 in sentryDied
-- (dispatch ev38 + lookup GetInstanceCreature bridged; boss GUID
-- latched by the ev5 SetGuidData in this file); sole consumer the
-- now-ported achievement_crazy_cat_lady (boss_auriaya.cpp:690).
-- DOCUMENTED-ONLY (no bridgeable arms):
-- Reset (:139-145) / HandleCats (:147-158) — _Reset() +
-- SetBossState(DATA_AURIAYA 6, NOT_STARTED) PORTED tick 258 via
-- creature:SetBossState (lua_creature_events.go:5917) + flag resets
-- + grid RESPAWN of entry 34014 within 500.0f — PORTED 2026-10-24
-- in auriayaReset (creature:Respawn bridge, lua_creature_events.go);
-- the HandleCats(false) despawn half is ported in auriayaDied;
-- stale "no grid bridge" retracted — creature:GetCreaturesInGrid is
-- bridged, lua_creature_events.go:1204.
-- DoAction (:178-193, ACTION_CRAZY_CAT_LADY 0 / ACTION_DEFENDER_DIED
-- 1) / GetData (:195-204, DATA_NINE_LIVES 30763077 /
-- DATA_CRAZY_CAT_LADY 30063007) — PORTED 2026-11-05 in auriayaDoAction
-- (ev38) + the SetData latches in auriayaReset / auriayaLeaveCombat /
-- auriayaSpawned; both relays ported (sentry :301, defender :452).
-- JustDied (:206-212) — _JustDied() SetBossState(DATA_AURIAYA 6,
-- DONE) PORTED tick 258 via creature:SetBossState
-- (lua_creature_events.go:5917), base-first per C++ :208;
-- DoPlaySoundToSet(15476) ported via
-- creature:PlayDirectSound (dedicated auriayaDied); HandleCats(false)
-- despawn leg ported there too (see above); the SendEncounterUnit
-- disengage leg ports there too (bridge live 2026-11-05); no Talk arms.
-- EnterEvadeMode (:214-218) — SendEncounterUnit disengage ports on
-- event 2 (dedicated auriayaLeaveCombat); the BossAI leg has no bridge.
-- The UNIT_STATE_CASTING gates (boss own UpdateAI pre-loop :227 +
-- in-loop :268; sentry pre-loop :311 + in-loop :335) are PORTED as a
-- 500ms IsCasting defer in every timer tick (mal_ganis idiom;
-- IsCasting bridged lua_creature_events.go:1950); the
-- DoMeleeAttackIfReady tail has no bridge.
-- npc_feral_defender — entry 34035 (no constant in C++; Wowhead-
-- verified 2026-11-05, npc=34035, Ulduar, WotLK) — the whole machine
-- is phase-gated (PHASE_NONE/PHASE_COMBAT :111-112) and its timer legs
-- (EVENT_RUSH :386-394, EVENT_START_COMBAT :379-384, respawn chain
-- :395-417) ride CatsTargetSelector random-target selection; the
-- DamageTaken (:427-446) feign-death machinery — health (GetHealth/
-- GetMaxHealth) and aura (creature:HasAura) bridges exist, and the
-- SetReactState/AttackStop/flag/threat/gravity bridges are live —
-- PORTED this tick: SetAuraStack (no bridge) is modeled Lua-side
-- (AddAura 64455 + per-GUID 8-stack counter, -1 per feign via the
-- 64456 script twin, aura removed at 0); the 61906 periodic retarget
-- (AuraScript, no bridge) rides a Lua 5s tick; JustDied (:448-453)
-- PORTED 2026-11-05 in feralDefenderDied (summon-essence self-cast
-- 64457 + DoAction(ACTION_DEFENDER_DIED) relay; entry 34035
-- registered in boss_ai.go).
-- npc_swarming_guardian — entry 34034 (no constant in C++; Wowhead-
-- verified this tick, npc=34034, Ulduar, WotLK) — Reset (:464-474):
-- REACT_PASSIVE + 1s scheduler -> REACT_AGGRESSIVE + DoCastSelf
-- SPELL_AGRO_CREATOR 63709 PORTED this tick in guardianReset (the
-- Auriaya JustSummoned relay is a no-op: boss_auriaya defines no
-- JustSummoned override; the 63709 pounce/threat leg lives in the
-- unported spell_auriaya_agro_creator SpellScript); registered in
-- boss_ai.go this tick.
-- npc_seeping_essence_stalker — entry 34098 (no constant in C++;
-- Wowhead-verified this tick, npc=34098, Ulduar, WotLK) — Reset
-- one-shot self-cast SPELL_SEEPING_ESSENCE 64458 (:494-499) PORTED
-- this tick in stalkerReset (JustSummoned relay no-op, as the
-- guardian; UpdateAI empty); registered in boss_ai.go this tick.
-- spell_auriaya_strenght_of_the_pack (64381),
-- spell_auriaya_sentinel_blast (64392 / 64679),
-- spell_auriaya_agro_creator (63709),
-- spell_auriaya_feral_essence_removal (64456),
-- spell_auriaya_feral_rush (64496 / 64674) — no SpellScript
-- bridge; spell_auriaya_random_agro_periodic (61906) — no
-- AuraScript bridge; achievement_nine_lives / achievement_crazy_cat_lady
-- — PORTED 2026-11-05: checkAchievementCriteriaScript reads the victim
-- motion's SetData latches (engine/world/achievement_criteria_scripts.go),
-- threaded from the kill-credit path (kill.go creditQuestKills).

local ENTRY_AURIAYA = 33515
local ENTRY_SANCTUM_SENTRY = 34014
local ENTRY_FERAL_DEFENDER = 34035
local ENTRY_SWARMING_GUARDIAN = 34034
local ENTRY_SEEPING_ESSENCE_STALKER = 34098

-- BOSS_AURIAYA = 6 (ulduar.h:42); DATA_NINE_LIVES 30763077 /
-- DATA_CRAZY_CAT_LADY 30063007 (boss_auriaya.cpp:108-109);
-- ACTION_CRAZY_CAT_LADY 0 / ACTION_DEFENDER_DIED 1
-- (boss_auriaya.cpp:101-104).
local BOSS_AURIAYA_DATA = 6
local DATA_NINE_LIVES = 30763077
local DATA_CRAZY_CAT_LADY = 30063007
local ACTION_CRAZY_CAT_LADY = 0
local ACTION_DEFENDER_DIED = 1

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
local SPELL_SUMMON_ESSENCE       = 64457
local SPELL_AGRO_CREATOR         = 63709
local SPELL_SEEPING_ESSENCE      = 64458
local SPELL_SHADOW_PAWS          = 64479
local SPELL_REDUCE_CRITCAL       = 64481
local SPELL_FERAL_ESSENCE        = 64455
local SPELL_RANDOM_AGRO_PERIODIC = 61906
local SPELL_FERAL_RUSH           = 64496
local SPELL_FERAL_POUNCE         = 64478
local SPELL_DROWNED_STATE        = 64462
local SPELL_PERMANENT_FEIGN_DEATH = 58951
local SPELL_FERAL_ESSENCE_APPLICATION_REMOVAL = 64456
local SPELL_CLEAR_ALL_DEBUFFS    = 34098
local SPELL_FULL_HEAL            = 64460

local REACT_PASSIVE = 0
local REACT_AGGRESSIVE = 2
local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000

local timers = {}

-- FERAL_ESSENCE stack counts per defender GUID (the SetAuraStack
-- twin — see feralDefenderDamageTaken); Reset sets 8, each feign
-- spends one, death/leave-combat clears via feralDefenderDied /
-- cancelCombatTimers + Reset re-init.
local defenderStacks = {}

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

-- C++ SelectTarget(Random, 1, 0.0f, true) leg (:256, UnitAI.h): offset 1
-- pops the current victim (non-distance list is victim-first), not
-- the threat-sorted head: exclude by victim GUID, then random over
-- the remaining threat-list players alive (offline refs are skipped
-- at build in C++, dead = offline here).
local function randomThreatPlayerSkipFirst(creature)
    local victim = creature:GetVictim()
    local victimGuid = nil
    if victim ~= nil then
        victimGuid = victim:GetGUID()
    end
    local targets = creature:GetAITargets()
    local candidates = {}
    local aliveSeen = 0
    for _, unit in ipairs(targets) do
        local skip = false
        if victimGuid ~= nil then
            skip = unit:GetGUID() == victimGuid
        elseif not unit:IsDead() then
            aliveSeen = aliveSeen + 1
            skip = aliveSeen == 1
        end
        if not skip
                and unit:GetObjectType() == "Player" and not unit:IsDead() then
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
    if creature:IsCasting() then
        schedule(guid, "sonic", 500, function()
            sonicScreechTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_SONIC_SCREECH)
    schedule(guid, "sonic", {22000, 30000}, function()
        sonicScreechTick(creature, guid)
    end)
end

-- C++ EVENT_BLAST (:244-245): DoCastAOE(SPELL_SENTINEL_BLAST 64389)
-- lowers to self-cast creature:CastSpell(creature, spell)
-- (non-triggered); fires 1ms after EVENT_TERRIFYING_SCREECH, no
-- re-arm.
local function blastTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "blast", 500, function()
            blastTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_SENTINEL_BLAST)
end

-- C++ EVENT_TERRIFYING_SCREECH (:238-242): Talk(EMOTE_FEAR 3) +
-- DoCastSelf(SPELL_TERRIFYING_SCREECH 64386) (2-arg non-triggered)
-- + EVENT_BLAST in 1ms (:241); 38s init (:166), re-arm {36s,45s}.
local function terrifyingScreechTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "terrifying", 500, function()
            terrifyingScreechTick(creature, guid)
        end)
        return
    end
    creature:Talk(EMOTE_FEAR)
    creature:CastSpell(creature, SPELL_TERRIFYING_SCREECH)
    schedule(guid, "blast", 1, function()
        blastTick(creature, guid)
    end)
    schedule(guid, "terrifying", {36000, 45000}, function()
        terrifyingScreechTick(creature, guid)
    end)
end

-- C++ EVENT_ACTIVATE_DEFENDER (:252-253): DoCastSelf
-- (SPELL_ACTIVATE_DEFENDER 64449) (2-arg non-triggered); fires 2s
-- after EVENT_SUMMON_DEFENDER, no re-arm.
local function activateDefenderTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "activatedefender", 500, function()
            activateDefenderTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_ACTIVATE_DEFENDER)
end

-- C++ EVENT_SUMMON_DEFENDER (:247-250): Talk(EMOTE_DEFENDER 4) +
-- DoCastSelf(SPELL_DEFENDER_TRIGGER 64448) (2-arg non-triggered)
-- + EVENT_ACTIVATE_DEFENDER in 2s (:250); 60s init (:167), no
-- re-arm.
local function summonDefenderTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "summondefender", 500, function()
            summonDefenderTick(creature, guid)
        end)
        return
    end
    creature:Talk(EMOTE_DEFENDER)
    creature:CastSpell(creature, SPELL_DEFENDER_TRIGGER)
    schedule(guid, "activatedefender", 2000, function()
        activateDefenderTick(creature, guid)
    end)
end

-- C++ EVENT_SWARNING_GUARDIAN (:255-258): SelectTarget(Random, 1,
-- 0.0f, true) -> randomThreatPlayerSkipFirst(creature), nil-guarded pick
-- (C++ `if` — skip cast on nil); DoCast 2-arg = non-triggered ->
-- CastSpell(target, SPELL_SUMMON_SWARMING_GUARDIAN 64396); re-arm
-- {25s,45s} unconditional (outside the target-if, C++-exact);
-- 51s init (:168). The summon is the spell's own effect (the
-- twilight-corrupter precedent); the old "needs target search, and
-- it is a summon leg; no bridges either way" claim was stale.
local function swarmingGuardianTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "swarming", 500, function()
            swarmingGuardianTick(creature, guid)
        end)
        return
    end
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
local function berserkTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "berserk", 500, function()
            berserkTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_BERSERK, true)
    creature:Talk(SAY_BERSERK)
end

-- C++ JustEngagedWith (:160-170): BossAI passthrough + Talk(AGGRO)
-- + encounter-frame engage + the five ScheduleEvent legs — the
-- Talk, the engage leg, and the four bridgeable timer legs are modeled here.
local function auriayaEnterCombat(event, creature, target)
    creature:SetBossState(BOSS_AURIAYA_DATA, 1) -- BossAI::JustEngagedWith _JustEngagedWith: SetBossState(DATA_AURIAYA 6, IN_PROGRESS), base-first (boss_auriaya.cpp:162)
    creature:SetCombatPulseDelay(5) -- BossAI::JustEngagedWith arm 1 (ScriptedCreature.cpp:540) -- base-first (boss_auriaya.cpp:162)
    creature:DoZoneInCombat() -- BossAI::JustEngagedWith me-form arm (ScriptedCreature.cpp:542)
    creature:Talk(SAY_AGGRO)
    creature:SendEncounterUnit(0, creature) -- ENCOUNTER_FRAME_ENGAGE (boss_auriaya.cpp:164)
    local guid = creature:GetGUIDLow()
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
        berserkTick(creature, guid)
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

-- Shared timer wipe for events 4/23 (and the sentry's event 2;
-- auriaya's own event 2 runs auriayaLeaveCombat above, which
-- disengages first). Auriaya JustDied :206-212 carries no Talk arms,
-- so event 4 models only the wipe.
local function cancelCombatTimers(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- npc_sanctum_sentry — C++ EVENT_RIP (:318-320):
-- DoCastVictim(SPELL_RIP_FLESH 64375) (2-arg non-triggered), 6s
-- init (:293), re-arm {10s,12s}.
local function ripTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "rip", 500, function()
            ripTick(creature, guid)
        end)
        return
    end
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
-- Cats band twin: C++ GetDistance subtracts both reaches (Object.cpp:1169);
-- player pool, reach 1.5, reach-exact.
local function savagePounceTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "pounce", 500, function()
            savagePounceTick(creature, guid)
        end)
        return
    end
    local candidates = {}
    for _, unit in ipairs(creature:GetAITargets()) do
        if unit:GetObjectType() == "Player" and not unit:IsDead() then
            local d = creature:GetDistance(unit) - creature:GetCombatReach() - 1.5
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
    local guid = creature:GetGUIDLow()
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
-- _JustDied() SetBossState(DATA_AURIAYA 6, DONE) leg PORTED tick 258
-- via creature:SetBossState (lua_creature_events.go:5917); the
-- SendEncounterUnit disengage leg ports in auriayaDied below
-- (bridge live 2026-11-05).
-- C++ boss_auriaya Reset (:139-145): _Reset() — SetBossState
-- (DATA_AURIAYA 6, NOT_STARTED) PORTED tick 258 via
-- creature:SetBossState (lua_creature_events.go:5917), first per C++
-- :141 — + flags + HandleCats(true) (:147-158): every dead sanctum
-- sentry (34014) within 500.0f respawns. GetCreaturesInGrid enumerates
-- dead motions still in the map (no health filter); IsDead reads the
-- snapshot Health via the object.go fallback; creature:Respawn restores
-- full health at home and fires event 23 on the sentry, running its
-- Reset arm as C++ ai->Reset() does.
local function auriayaReset(event, creature)
    creature:SetBossState(BOSS_AURIAYA_DATA, 0)
    cancelCombatTimers(event, creature)
    -- C++ Reset (:139-145): _crazyCatLady = true, _nineLives = false.
    -- Alive-gated: ev23 fires on death too (fireCreatureDied runs
    -- Reset before the kill-credit criteria read, kill.go:1091/:1400),
    -- and a death-time re-arm would clobber the fight's latch before
    -- the achievement gate reads it; the spawn handler sets both.
    if creature:IsAlive() then
        creature:SetData(DATA_CRAZY_CAT_LADY, 1)
        creature:SetData(DATA_NINE_LIVES, 0)
    end
    for _, sentry in ipairs(creature:GetCreaturesInGrid(ENTRY_SANCTUM_SENTRY, 500)) do
        if sentry:IsDead() then
            sentry:Respawn()
        end
    end
end

local function auriayaDied(event, creature, killer)
    creature:SetBossState(BOSS_AURIAYA_DATA, 3) -- _JustDied() first per C++ :208
    creature:PlayDirectSound(SOUND_AURIAYA_DEATH)
    creature:SendEncounterUnit(1, creature) -- ENCOUNTER_FRAME_DISENGAGE (boss_auriaya.cpp:210)
    cancelCombatTimers(event, creature)
    for _, sentry in ipairs(creature:GetCreaturesInGrid(ENTRY_SANCTUM_SENTRY, 500)) do
        sentry:Despawn()
    end
end

-- C++ EnterEvadeMode (:214-218): disengage first, then
-- BossAI::EnterEvadeMode (no bridge). Event 2 only — the shared
-- cancelCombatTimers stays on events 4/23, which carry no disengage.
local function auriayaLeaveCombat(event, creature)
    creature:SendEncounterUnit(1, creature) -- ENCOUNTER_FRAME_DISENGAGE
    cancelCombatTimers(event, creature)
    -- C++ EnterEvadeMode ends with Reset() (CreatureAI.cpp:237).
    creature:SetData(DATA_CRAZY_CAT_LADY, 1)
    creature:SetData(DATA_NINE_LIVES, 0)
end

-- C++ instance OnCreatureCreate latching (InstanceScript::AddObject):
-- the boss GUID goes in the instance store under BOSS_AURIAYA so the
-- sentry/defender relays can resolve it via GetInstanceCreature; the
-- latch init is the ctor twin (boss_auriaya.cpp:137).
local function auriayaSpawned(event, creature)
    creature:SetGuidData(BOSS_AURIAYA_DATA)
    creature:SetData(DATA_CRAZY_CAT_LADY, 1)
    creature:SetData(DATA_NINE_LIVES, 0)
end

-- C++ DoAction (:178-193): ACTION_CRAZY_CAT_LADY clears the lady latch,
-- ACTION_DEFENDER_DIED sets the nine-lives latch.
local function auriayaDoAction(event, creature, action)
    if action == ACTION_CRAZY_CAT_LADY then
        creature:SetData(DATA_CRAZY_CAT_LADY, 0)
    elseif action == ACTION_DEFENDER_DIED then
        creature:SetData(DATA_NINE_LIVES, 1)
    end
end

-- C++ npc_sanctum_sentry::JustDied (:298-302): instance GetCreature
-- (BOSS_AURIAYA) -> DoAction(ACTION_CRAZY_CAT_LADY); the timer cancel
-- is the file's ev4 hygiene arm (timers keyed by GUID).
local function sentryDied(event, creature)
    local auriaya = creature:GetInstanceCreature(BOSS_AURIAYA_DATA)
    if auriaya then
        auriaya:DoAction(ACTION_CRAZY_CAT_LADY)
    end
    cancelCombatTimers(event, creature)
end

-- C++ npc_feral_defender::JustDied (:448-453): DoCastSelf
-- (SPELL_SUMMON_ESSENCE 64457, triggered) then instance GetCreature
-- (BOSS_AURIAYA) -> DoAction(ACTION_DEFENDER_DIED). The feign-death
-- revives (DamageTaken :427-446) never reach JustDied, so this fires
-- only on a real death.
local function feralDefenderDied(event, creature)
    cancelCombatTimers(event, creature)
    defenderStacks[creature:GetGUIDLow()] = nil
    creature:CastSpell(creature, SPELL_SUMMON_ESSENCE, true)
    local auriaya = creature:GetInstanceCreature(BOSS_AURIAYA_DATA)
    if auriaya then
        auriaya:DoAction(ACTION_DEFENDER_DIED)
    end
end

-- C++ CatsTargetSelector ring pick (boss_auriaya.cpp:120-134):
-- threat-list player alive in [minDist, maxDist) with LOS; the
-- distance is C++ GetDistance (both reaches subtracted,
-- Object.cpp:1169) — the sentry savage-pounce twin (reach 1.5,
-- reach-exact). skipVictim mirrors the SelectTarget offset-1 legs
-- (victim first, popped before the draw).
local function randomRingTarget(creature, minDist, maxDist, skipVictim)
    local victim = creature:GetVictim()
    local victimGuid = nil
    if victim ~= nil then
        victimGuid = victim:GetGUID()
    end
    local candidates = {}
    for _, unit in ipairs(creature:GetAITargets()) do
        if unit:GetObjectType() == "Player" and not unit:IsDead()
                and (not skipVictim or victimGuid == nil
                    or unit:GetGUID() ~= victimGuid) then
            local d = creature:GetDistance(unit) - creature:GetCombatReach() - 1.5
            if d >= minDist and d < maxDist and creature:IsWithinLOS(unit) then
                candidates[#candidates + 1] = unit
            end
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ SelectTarget(Random, 0, 0.0f, true): random threat-list player
-- alive, no distance filter, no skip.
local function randomAliveThreatPlayer(creature)
    local candidates = {}
    for _, unit in ipairs(creature:GetAITargets()) do
        if unit:GetObjectType() == "Player" and not unit:IsDead() then
            candidates[#candidates + 1] = unit
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EVENT_RUSH (:386-394): SelectTarget(Random, 1,
-- CatsTargetSelector(me, 10.0f, 11.0f)) -> DoCast(target,
-- SPELL_FERAL_RUSH 64496, triggered); found: 5s re-arm; none: 1s.
local function defenderRushTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "rush", 500, function()
            defenderRushTick(creature, guid)
        end)
        return
    end
    local target = randomRingTarget(creature, 10.0, 11.0, true)
    if target then
        creature:CastSpell(target, SPELL_FERAL_RUSH, true)
        schedule(guid, "rush", 5000, function()
            defenderRushTick(creature, guid)
        end)
    else
        schedule(guid, "rush", 1000, function()
            defenderRushTick(creature, guid)
        end)
    end
end

-- C++ spell_auriaya_random_agro_periodic (61906, :580-616): 5s
-- periodic — no AuraScript bridge, so the leg rides this Lua 5s tick
-- (the aura itself is still cast triggered in Reset; its engine
-- periodic is a no-op without the script). PASSIVE-gated exactly
-- like the C++ handler; far target 15-25yd ring + LOS -> AddThreat
-- 3e6 + CastSpell FERAL_POUNCE 64478 triggered + AttackStart; else
-- any random threat player -> AddThreat 3e6 + AttackStart; none:
-- nothing. No IsCasting defer — aura periodics tick outside UpdateAI.
local function defenderRetargetTick(creature, guid)
    if creature:GetReactState() ~= REACT_PASSIVE then
        local target = randomRingTarget(creature, 15.0, 25.0, false)
        local farTarget = target ~= nil
        if not target then
            target = randomAliveThreatPlayer(creature)
        end
        if target then
            creature:AddThreat(target, 3000000)
            if farTarget then
                creature:CastSpell(target, SPELL_FERAL_POUNCE, true)
            end
            creature:AttackStart(target)
        end
    end
    schedule(guid, "retarget", 5000, function()
        defenderRetargetTick(creature, guid)
    end)
end

-- C++ EVENT_START_COMBAT (:379-384): phase COMBAT +
-- SetReactState(AGGRESSIVE) + random target AttackStart +
-- EVENT_RUSH in 1s. The phase needs no Lua variable — the schedule
-- encodes it (RUSH only exists after this arm).
local function defenderStartCombatTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "startcombat", 500, function()
            defenderStartCombatTick(creature, guid)
        end)
        return
    end
    creature:SetReactState(REACT_AGGRESSIVE)
    local target = randomAliveThreatPlayer(creature)
    if target then
        creature:AttackStart(target)
    end
    schedule(guid, "rush", 1000, function()
        defenderRushTick(creature, guid)
    end)
end

-- C++ EVENT_RESPAWN_DEFENDER_2 (:402-404): RemoveAurasDueToSpell
-- (SPELL_DROWNED_STATE 64462) — creature:RemoveAura twin.
local function defenderRespawnTick2(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "respawn2", 500, function()
            defenderRespawnTick2(creature, guid)
        end)
        return
    end
    creature:RemoveAura(SPELL_DROWNED_STATE)
end

-- C++ EVENT_RESPAWN_DEFENDER_3 (:405-417): RemoveAurasDueToSpell
-- (FEIGN_DEATH) + DoCastSelf(FULL_HEAL 64460, triggered) +
-- RemoveFlag(NOT_SELECTABLE) + SetReactState(AGGRESSIVE) +
-- SetDisableGravity(false) + SetHover(false) + random target
-- AttackStart + EVENT_RUSH in 1s.
local function defenderRespawnTick3(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "respawn3", 500, function()
            defenderRespawnTick3(creature, guid)
        end)
        return
    end
    creature:RemoveAura(SPELL_PERMANENT_FEIGN_DEATH)
    creature:CastSpell(creature, SPELL_FULL_HEAL, true)
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    creature:SetReactState(REACT_AGGRESSIVE)
    creature:SetDisableGravity(false)
    creature:SetHover(false)
    local target = randomAliveThreatPlayer(creature)
    if target then
        creature:AttackStart(target)
    end
    schedule(guid, "rush", 1000, function()
        defenderRushTick(creature, guid)
    end)
end

-- C++ EVENT_RESPAWN_DEFENDER (:395-401): SetDisableGravity(true) +
-- SetHover(true) + DoCastSelf(DROWNED_STATE 64462, triggered) +
-- _2 in 3s + _3 in 5s (both scheduled from here, C++-exact).
local function defenderRespawnTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "respawn", 500, function()
            defenderRespawnTick(creature, guid)
        end)
        return
    end
    creature:SetDisableGravity(true)
    creature:SetHover(true)
    creature:CastSpell(creature, SPELL_DROWNED_STATE, true)
    schedule(guid, "respawn2", 3000, function()
        defenderRespawnTick2(creature, guid)
    end)
    schedule(guid, "respawn3", 5000, function()
        defenderRespawnTick3(creature, guid)
    end)
end

-- C++ npc_feral_defender::DamageTaken (:427-446): lethal-or-equal
-- damage AND HasAura(FERAL_ESSENCE) zeroes the hit (DamageTaken
-- form — fires on lethal hits, no postDamage>0 guard, nefarian
-- precedent); unless already feigning (HasAura FEIGN_DEATH):
-- PASSIVE + AttackStop + NOT_SELECTABLE + self-cast FEIGN_DEATH
-- 58951 triggered + self-cast 64456 triggered (the essence-removal
-- SpellScript: ModStackAmount(-1) — modeled Lua-side below) +
-- self-cast SUMMON_ESSENCE 64457 triggered + self-cast CLEAR_ALL_
-- DEBUFFS 34098 triggered + ResetThreatList + RESPAWN_DEFENDER in
-- 30s + CancelEvent(RUSH). The 8-stack FERAL_ESSENCE latch is the
-- SetAuraStack twin (no bridge): Reset applies the aura via
-- creature:AddAura (HasAura reads the presence set) and the count
-- lives in defenderStacks; at 0 the aura is removed, so the next
-- lethal hit kills — 8 feigns + the death = nine lives.
local function feralDefenderDamageTaken(event, creature, attacker, damage)
    local newDamage = damage
    if damage >= creature:GetHealth() and creature:HasAura(SPELL_FERAL_ESSENCE) then
        newDamage = 0
        if not creature:HasAura(SPELL_PERMANENT_FEIGN_DEATH) then
            local guid = creature:GetGUIDLow()
            creature:SetReactState(REACT_PASSIVE)
            creature:AttackStop()
            creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
            creature:CastSpell(creature, SPELL_PERMANENT_FEIGN_DEATH, true)
            creature:CastSpell(creature, SPELL_FERAL_ESSENCE_APPLICATION_REMOVAL, true)
            local stacks = defenderStacks[guid] or 0
            if stacks > 0 then
                stacks = stacks - 1
                defenderStacks[guid] = stacks
                if stacks == 0 then
                    creature:RemoveAura(SPELL_FERAL_ESSENCE)
                end
            end
            creature:CastSpell(creature, SPELL_SUMMON_ESSENCE, true)
            creature:CastSpell(creature, SPELL_CLEAR_ALL_DEBUFFS, true)
            creature:ResetAllThreat()
            schedule(guid, "respawn", 30000, function()
                defenderRespawnTick(creature, guid)
            end)
            local per = timers[guid]
            if per and per["rush"] then
                RemoveEventById(per["rush"])
                per["rush"] = nil
            end
        end
    end
    return false, newDamage
end

-- C++ npc_feral_defender::Reset (:351-363): SetReactState(PASSIVE)
-- + DoCastSelf(SHADOW_PAWS 64479, triggered) + DoCastSelf
-- (REDUCE_CRITCAL 64481, triggered) + SetAuraStack(FERAL_ESSENCE
-- 64455, me, 8) — no bridge; AddAura + defenderStacks twin, see
-- DamageTaken — + DoCastSelf(RANDOM_AGRO_PERIODIC 61906, triggered)
-- + phase NONE + EVENT_START_COMBAT in 1s; the JustSummoned relay
-- (:361-362) is a no-op (no boss override, as the guardian).
-- The 61906 periodic retarget rides defenderRetargetTick (5s, from
-- Reset, re-arming — the AuraScript itself has no bridge).
local function feralDefenderReset(event, creature)
    cancelCombatTimers(event, creature)
    local guid = creature:GetGUIDLow()
    defenderStacks[guid] = 8
    creature:SetReactState(REACT_PASSIVE)
    creature:CastSpell(creature, SPELL_SHADOW_PAWS, true)
    creature:CastSpell(creature, SPELL_REDUCE_CRITCAL, true)
    creature:AddAura(SPELL_FERAL_ESSENCE)
    creature:CastSpell(creature, SPELL_RANDOM_AGRO_PERIODIC, true)
    schedule(guid, "startcombat", 1000, function()
        defenderStartCombatTick(creature, guid)
    end)
    schedule(guid, "retarget", 5000, function()
        defenderRetargetTick(creature, guid)
    end)
end

-- npc_swarming_guardian — C++ Reset (:464-474): SetReactState(PASSIVE)
-- + 1s TaskScheduler arm -> SetReactState(AGGRESSIVE) +
-- DoCastSelf(SPELL_AGRO_CREATOR 63709) (2-arg non-triggered); the
-- Auriaya JustSummoned relay (:472-473) is a no-op — boss_auriaya
-- defines no JustSummoned override. The 63709 dummy-effect pounce /
-- threat leg lives in spell_auriaya_agro_creator (SpellScript, no
-- bridge); the cast itself ports. The scheduler arm runs outside
-- UpdateVictim / the CASTING gate in C++, so no 500ms defer.
local function guardianAggroTick(creature, guid)
    creature:SetReactState(REACT_AGGRESSIVE)
    creature:CastSpell(creature, SPELL_AGRO_CREATOR)
end

local function guardianReset(event, creature)
    cancelCombatTimers(event, creature)
    creature:SetReactState(REACT_PASSIVE)
    local guid = creature:GetGUIDLow()
    schedule(guid, "guardianAggro", 1000, function()
        guardianAggroTick(creature, guid)
    end)
end

-- npc_seeping_essence_stalker — C++ Reset (:493-499): one-shot
-- DoCastSelf(SPELL_SEEPING_ESSENCE 64458) (2-arg non-triggered); the
-- Auriaya JustSummoned relay (:498) is a no-op (no override, as the
-- guardian); UpdateAI is empty.
local function stalkerReset(event, creature)
    creature:CastSpell(creature, SPELL_SEEPING_ESSENCE)
end

RegisterCreatureEvent(ENTRY_AURIAYA, 1, auriayaEnterCombat)
RegisterCreatureEvent(ENTRY_AURIAYA, 2, auriayaLeaveCombat)
RegisterCreatureEvent(ENTRY_AURIAYA, 3, auriayaTargetDied)
RegisterCreatureEvent(ENTRY_AURIAYA, 4, auriayaDied)
RegisterCreatureEvent(ENTRY_AURIAYA, 5, auriayaSpawned)
RegisterCreatureEvent(ENTRY_AURIAYA, 23, auriayaReset)
RegisterCreatureEvent(ENTRY_AURIAYA, 38, auriayaDoAction)
RegisterCreatureEvent(ENTRY_SANCTUM_SENTRY, 1, sentryEnterCombat)
RegisterCreatureEvent(ENTRY_SANCTUM_SENTRY, 2, cancelCombatTimers)
RegisterCreatureEvent(ENTRY_SANCTUM_SENTRY, 4, sentryDied)
RegisterCreatureEvent(ENTRY_SANCTUM_SENTRY, 23, sentryReset)
RegisterCreatureEvent(ENTRY_FERAL_DEFENDER, 2, cancelCombatTimers)
RegisterCreatureEvent(ENTRY_FERAL_DEFENDER, 4, feralDefenderDied)
RegisterCreatureEvent(ENTRY_FERAL_DEFENDER, 9, feralDefenderDamageTaken)
RegisterCreatureEvent(ENTRY_FERAL_DEFENDER, 23, feralDefenderReset)
RegisterCreatureEvent(ENTRY_SWARMING_GUARDIAN, 2, cancelCombatTimers)
RegisterCreatureEvent(ENTRY_SWARMING_GUARDIAN, 4, cancelCombatTimers)
RegisterCreatureEvent(ENTRY_SWARMING_GUARDIAN, 23, guardianReset)
RegisterCreatureEvent(ENTRY_SEEPING_ESSENCE_STALKER, 23, stalkerReset)
