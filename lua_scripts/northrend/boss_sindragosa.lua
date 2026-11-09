-- Sindragosa (Icecrown Citadel) — Lua port of
-- src/server/scripts/Northrend/IcecrownCitadel/boss_sindragosa.cpp
-- (1556 lines incl. license; 5 CreatureScripts
-- (boss_sindragosa (BossAI, DATA_SINDRAGOSA = 11), npc_ice_tomb
-- (ScriptedAI), npc_spinestalker (ScriptedAI), npc_rimefang_icc
-- (ScriptedAI), npc_sindragosa_trash (ScriptedAI, shared AI for
-- NPC_FROSTWARDEN_HANDLER / NPC_FROSTWING_WHELP)) +
-- 13 Sindragosa-specific SpellScript/AuraScript legs
-- (spell_sindragosa_s_fury, spell_sindragosa_unchained_magic,
-- spell_sindragosa_frost_breath, spell_sindragosa_instability,
-- spell_sindragosa_frost_beacon, spell_sindragosa_ice_tomb_trap,
-- spell_sindragosa_icy_grip, spell_sindragosa_mystic_buffet,
-- spell_rimefang_icy_blast, spell_frostwarden_handler_order_whelp,
-- spell_frostwarden_handler_focus_fire,
-- spell_frostwarden_handler_focus_fire_aura (AuraScript),
-- spell_sindragosa_ice_tomb_target) + 2 generic
-- RegisterSpellScriptWithArgs spell_trigger_spell_from_caster legs
-- + 1 AreaTriggerScript
-- (at_sindragosa_lair) + 1 AchievementCriteriaScript
-- (achievement_all_you_can_eat); all registered from inside
-- AddSC_boss_sindragosa(); loader decl 182 / call 377 per
-- northrend_script_loader.cpp — the TWELFTH group of the
-- "// Icecrown Citadel" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_valithria_dreamwalker() (call 376) — verified
-- from the loader this run; the checkpoint sequence
-- (valithria_dreamwalker -> sindragosa) is followed).
-- Entry: 36853 Sindragosa (icecrown_citadel.h NPC_SINDRAGOSA, line
-- 304; instance_icecrown_citadel.cpp OnCreatureCreate binds case
-- NPC_SINDRAGOSA, line 300; also summoned via instance
-- SummonCreature(NPC_SINDRAGOSA, SindragosaSpawnPos), line 492) —
-- entry-verifiable, registration proceeds (the nexus_commanders
-- kalecgos precedent); the RegisterIcecrownCitadelCreatureAI
-- ScriptName bindings are DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_sindragosa" hits boss_sindragosa.cpp only (+ the
-- loader decl/call lines); this clone carries no sql/ tree, so
-- ScriptName bindings are DB-side by construction.
-- Eluna creature events: 1 OnEnterCombat, 3 OnKill, 4 OnDied.
-- Ported arms (C++-exact for all modeled arms):
-- boss_sindragosa JustEngagedWith — Talk(SAY_AGGRO 0) (event 1 —
-- the auriaya engage-port precedent; the instance
-- CheckRequiredBosses gate / EVADE_REASON_SEQUENCE_BREAK leg /
-- DoCastSpellOnPlayers LIGHT_S_HAMMER_TELEPORT leg / setActive leg
-- have no bridge and stay out; DoCastSelf FROST_AURA+PERMAEATING_CHILL
-- (:277-278) + SetBossState(IN_PROGRESS) (:280) PORTED 2026-11-06,
-- in C++ arm order; the SetCombatPulseDelay(5) +
-- SetFarVisible(true) arms are PORTED
-- (creature:SetCombatPulseDelay bridge, lua_creature_events.go; event 1,
-- C++ arm order); the DoZoneInCombat tail (:284) is PORTED
-- (creature:DoZoneInCombat, live bridge
-- engine/world/lua_creature_events.go, 2026-10-23)).
-- boss_sindragosa KilledUnit — Talk(SAY_KILL 8) gated on
-- victim->GetTypeId() == TYPEID_PLAYER (event 3 — the razuvious
-- player-gated variant precedent).
-- boss_sindragosa JustDied — Talk(SAY_DEATH 10) (event 4 — the
-- sjonnir JustDied-Talk precedent; the DoCastAOE
-- FROST_INFUSION_CREDIT leg PORTED (Is25ManRaid() + HasAura(SPELL_SHADOWS_FATE)
-- (:262-264); creature:Is25ManRaid() bridged lua_creature_events.go,
-- the old "no 25-man difficulty bridge" claim retracted).
-- PORTED (tick 190, RegisterAuraEvent): spell_sindragosa_frost_beacon
-- PeriodicTick — PreventDefaultAction + caster casts SPELL_ICE_TOMB_DAMAGE
-- 70157 triggered on the aura target (:1223-1232).
-- DOCUMENTED-ONLY (in this header; only entry 36853 registered):
-- PORTED (2026-10-13): JustEngagedWith — DoCastSelf(
-- SPELL_FROST_AURA 70084) + DoCastSelf(SPELL_PERMAEATING_CHILL 70109)
-- non-triggered (:277-278, maiden engage-cast convention);
-- EVENT_BERSERK — single-fire 10min — Talk(EMOTE_BERSERK_RAID 11) +
-- Talk(SAY_BERSERK 9) + DoCastSelf(SPELL_BERSERK 26662) non-triggered
-- (:242/:458-460), bare timer across all phases.
-- PORTED (2026-10-13 retraction re-audit): Reset() —
-- DoCastSelf(SPELL_TANK_MARKER 71039, triggered) (:241);
-- DamageTaken 35% latch — Talk(SAY_PHASE_2 6) +
-- DoCastSelf(SPELL_MYSTIC_BUFFET 70128, triggered) via a 1s
-- one-shot timer (the EVENT_THIRD_PHASE_CHECK leg :403-410 /
-- :548-553; CancelEvent(EVENT_AIR_PHASE) is moot — no air-phase
-- timers exist in this port; RescheduleEvent(EVENT_ICY_GRIP) rides
-- the unbridged icy-grip leg); EVENT_ICE_TOMB — CastSpell(nil,
-- SPELL_ICE_TOMB_TARGET 69712, triggered) init {7s,10s} / re-arm
-- {16s,23s} (:515-518/:551 — the air-phase retry loop is
-- vacuously satisfied with no air-phase machine; the SPELLVALUE_
-- MAX_TARGETS(1) spell-mod leg has no bridge, default cast
-- faithful).
-- boss_sindragosa UpdateAI — the land/air phase machine is PORTED
-- 2026-11-09: land timers (cleave / tail-smash / frost-breath /
-- unchained-magic / icy-grip -> blistering-cold chain) + EVENT_AIR_PHASE
-- (MoveTakeoff / MovePoint / MoveLand / ev28 MovementInform, all bridged
-- lua_creature_events.go) with the land-group cancel on air entry and the
-- LAND_GROUND re-seed; the EVENT_FROST_BOMB timer stays out (no
-- positional-cast bridge for the 69846 trigger) and the LAND_GROUND
-- asphyxiation summons.DoAction leg stays out (no summon-set bridge).
-- The EnterEvadeMode air-phase + BOUNDARY suppression stays out (ev2
-- carries no EvadeReason); the Reset intro-gated fly-set stays out (no
-- instance GetData bridge).
-- SAY_RESPITE_FOR_A_TORMENTED_SOUL 4 is declared in the text enum
-- but never referenced by a Talk() call — dead text evidence.
-- boss_sindragosa DoAction — ACTION_START_FROSTWYRM intro machine (ev38
-- dispatch bridged; senders at_sindragosa_lair OnTrigger
-- (boss_sindragosa.cpp:1505) + instance OnCreatureRemove
-- (instance_icecrown_citadel.cpp:493) unported, receiver arm is the unported
-- flight machine); JustReachedHome SetBossState FAIL leg PORTED
-- 2026-11-06 (sindragosaReachHome, ev24).
-- spell_sindragosa_ice_tomb_target::HandleSindragosaTalk —
-- creatureCaster->AI()->Talk(EMOTE_WARN_FROZEN_ORB 7, GetHitUnit())
-- is cross-AI Talk (no cross-AI-Talk bridge).
-- npc_ice_tomb (36980) — zero Talk lines — NOT registered (the
-- bronjahm npc_corrupted_soul_fragment precedent); its SetGUID /
-- DoAction ACTION_TRIGGER_ASPHYXIATION / GUID-bookkeeping legs ride
-- bridges that do not exist. npc_spinestalker (37534) /
-- npc_rimefang_icc (37533) / npc_sindragosa_trash handler (37531)
-- ARE registered — their pure timer arms are PORTED below (the
-- maiden CreateLuaEvent precedent); their intro DoAction flight machines and the trash JustAppeared / SetData
-- instance legs stay out (ev38 dispatch bridged; senders at_sindragosa_lair
-- (boss_sindragosa.cpp:1492,1496) + instance OnCreatureRemove
-- (instance_icecrown_citadel.cpp:469,476) unported / no-motion / no-instance
-- bridges). The
-- frostwing whelp (37532) carries no timer arms — NOT registered.
-- at_sindragosa_lair joins the no-AreaTrigger bridge queue (the
-- boss_anubrekhan at_anubrekhan_entrance precedent);
-- achievement_all_you_can_eat — PORTED 2026-11-08 (tick 416): the
-- SpellHitTarget peak latch rides sindragosaSpellHitTarget below; Go
-- case in engine/world/achievement_criteria_scripts.go. SpellScript /
-- AuraScript legs PORTED 2026-11-09 (RegisterSpellEvent ev5/ev7, dormant
-- until Spell.dbc ships): spell_sindragosa_icy_grip (ev5 effect 0),
-- spell_sindragosa_mystic_buffet (ev7 LOS filter),
-- spell_rimefang_icy_blast (ev5 effect 1, GetExplTargetDest),
-- spell_frostwarden_handler_order_whelp (ev7 filter + ev5 force-cast).
-- The remaining 7 Sindragosa-specific legs + the 2 generic
-- spell_trigger_spell_from_caster with-args legs stay out, each for a
-- named missing bridge, not a class queue: s_fury (BeforeCast dest
-- select + resistance damage split, no GetResistance / DealSpellDamage
-- bridge), unchained_magic (healer/caster-class/spec filter, no
-- IsPlayerHealer / specialization bridge), frost_breath (Shadowmourne
-- quest infusion, no GetQuestStatus bridge), instability (OnRemove
-- expire -> BACKLASH with AddSpellBP0, no cast-mod bridge),
-- ice_tomb_trap (trap-aura summon + SetGUID, binding not C++-verifiable),
-- focus_fire + focus_fire_aura (AddThreat legs, no AddThreat bridge),
-- ice_tomb_target filter (FrostBeaconSelector tank exclusion, no tank
-- bridge — the untargetable-aura leg already rides randomPlayerTarget),
-- spell_trigger_spell_from_caster x2 (DBC trigger-spell wiring).
-- All 6 Talk() calls in the file accounted for (6 =
-- SAY_AGGRO 0 (1) + SAY_DEATH 10 (1) + SAY_KILL 8 (1) +
-- EMOTE_BERSERK_RAID 11 (1, scheduler) + SAY_BERSERK 9 (1,
-- scheduler) + SAY_PHASE_2 6 (1, scheduler)).
-- The phase-machine Talk legs (SAY_UNCHAINED_MAGIC 1,
-- EMOTE_WARN_BLISTERING_COLD 2, SAY_BLISTERING_COLD 3,
-- SAY_AIR_PHASE 5) are PORTED 2026-11-09 with the air-phase machine above.

-- CAST-STATE GATES PORTED (2026-11-04): boss :450/:563 gates berserk /
-- ice-tomb / third-phase-check; spinestalker :730 (pre-loop only) gates
-- all 3 ticks; rimefang :862 (pre-loop only) gates frost-breath;
-- trash :986 (pre-loop only) gates both handler ticks; ice-tomb :618
-- UpdateAI has no gate (existence check only, NOT registered); engage /
-- DamageTaken / JustDied / JustSummoned legs ungated per C++ (separate
-- hooks / BasicEvent, not the scheduler).
local ENTRY_SINDRAGOSA = 36853
local ENTRY_SPINESTALKER = 37534 -- icecrown_citadel.h:305
local ENTRY_RIMEFANG = 37533 -- icecrown_citadel.h:306
local ENTRY_FROSTWARDEN_HANDLER = 37531 -- icecrown_citadel.h:307
local ENTRY_FROST_BOMB = 37186 -- icecrown_citadel.h:310

local SAY_AGGRO = 0
local SAY_KILL = 8
local SAY_DEATH = 10
local SAY_BERSERK = 9
local EMOTE_BERSERK_RAID = 11
local SAY_PHASE_2 = 6

local DATA_SINDRAGOSA = 11 -- icecrown_citadel.h:85
local BOSS_STATE_NOT_STARTED = 0 -- InstanceScript.h:68-76
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_FAIL = 2
local BOSS_STATE_DONE = 3

local SPELL_TANK_MARKER = 71039
local SPELL_MYSTIC_BUFFET = 70128
local SPELL_MYSTIC_BUFFET_HIT = 70127 -- boss_sindragosa.cpp:437, base id;
-- the 25-man/heroic difficulty variants live in DBC (spell_difficulty),
-- not in the C++ tree, so only the base id is filterable here.
local DATA_MYSTIC_BUFFET_STACK = 0 -- boss_sindragosa.cpp:141
local SPELL_FROST_BOMB_VISUAL = 70022 -- boss_sindragosa.cpp:74
local SPELL_BIRTH_NO_VISUAL = 40031 -- boss_sindragosa.cpp:75
local SPELL_ICE_TOMB_TARGET = 69712
local SPELL_ICE_TOMB_UNTARGETABLE = 69700
local SPELL_FROST_BEACON = 70126 -- boss_sindragosa.cpp:67
local SPELL_ICE_TOMB_DAMAGE = 70157 -- boss_sindragosa.cpp:71
local EMOTE_WARN_FROZEN_ORB = 7

local sindragosaThirdPhase = {}
local sindragosaAirPhase = {}
local rimefangBlastCounter = {}

local SPELL_FROST_AURA = 70084
local SPELL_PERMAEATING_CHILL = 70109
local SPELL_SHADOWS_FATE = 71169
local SPELL_FROST_INFUSION_CREDIT = 72289
local SPELL_BERSERK = 26662
local SPELL_BELLOWING_ROAR = 36922 -- :80
local SPELL_CLEAVE_SPINESTALKER = 40505 -- :81
local SPELL_TAIL_SWEEP = 71370 -- :82
local SPELL_FROST_BREATH_RIMEFANG = 71386 -- :85
local SPELL_FROST_AURA_RIMEFANG = 71387 -- :86
local SPELL_ORDER_WHELP = 71357 -- :92
local SPELL_CONCUSSIVE_SHOCK = 71337 -- :93
local SPELL_ICY_BLAST = 71376 -- :87
local SPELL_ICY_BLAST_AREA = 71380 -- :88
local SPELL_FOCUS_FIRE = 71350 -- :91

local SAY_UNCHAINED_MAGIC = 1
local EMOTE_WARN_BLISTERING_COLD = 2
local SAY_BLISTERING_COLD = 3
local SAY_AIR_PHASE = 5

local SPELL_CLEAVE = 19983 -- :58
local SPELL_TAIL_SMASH = 71077 -- :59
local SPELL_FROST_BREATH_P1 = 69649 -- :60
local SPELL_FROST_BREATH_P2 = 73061 -- :61
local SPELL_UNCHAINED_MAGIC = 69762 -- :62
local SPELL_ICY_GRIP = 70117 -- :64
local SPELL_ICY_GRIP_JUMP = 70122 -- :65
local SPELL_BLISTERING_COLD = 70123 -- :66

local ENTRY_ICY_BLAST = 38223 -- icecrown_citadel.h:309
local ENTRY_FROSTWING_WHELP = 37532 -- icecrown_citadel.h:308
local DATA_FROSTWYRM_OWNER = 1 -- boss_sindragosa.cpp:142
local DATA_WHELP_MARKER = 2 -- boss_sindragosa.cpp:143

local POINT_MOTION_TYPE = 8 -- MovementGeneratorType POINT (darnavan precedent)
local EFFECT_MOTION_TYPE = 16 -- MovementGeneratorType EFFECT (MoveTakeoff/MoveLand)
local POINT_FROSTWYRM_FLY_IN = 1 -- :151
local POINT_AIR_PHASE = 3 -- :153
local POINT_TAKEOFF = 4 -- :154
local POINT_LAND = 5 -- :155
local POINT_AIR_PHASE_FAR = 6 -- :156
local POINT_LAND_GROUND = 7 -- :157
local SINDRAGOSA_AIR_POS = {4475.99, 2484.43, 247.934}
local SINDRAGOSA_AIR_POS_FAR = {4525.6, 2485.15, 245.082}
local SINDRAGOSA_FLY_IN_POS = {4419.19, 2484.36, 232.515}
local SINDRAGOSA_LAND_POS = {4419.19, 2484.57, 203.3848}
local RIMEFANG_FLY_POS = {4413.309, 2456.421, 233.3795}

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

local function cancelKey(guid, key)
    local per = timers[guid]
    if per and per[key] then
        RemoveEventById(per[key])
        per[key] = nil
    end
end

-- C++ EVENT_CLEAVE: DoCastVictim(SPELL_CLEAVE) non-triggered, init 10s
-- (:243), re-arm {15s,20s} (:471-473), EVENT_GROUP_LAND_PHASE — the
-- air-phase entry below cancels the key; no re-arm while airborne.
local function onCleave(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "cleave", 500, function() onCleave(creature, guid) end)
        return
    end
    if sindragosaAirPhase[guid] then
        return
    end
    creature:CastSpell(nil, SPELL_CLEAVE)
    schedule(guid, "cleave", {15000, 20000}, function() onCleave(creature, guid) end)
end

-- C++ EVENT_TAIL_SMASH: DoCastSelf(SPELL_TAIL_SMASH) non-triggered,
-- init 20s (:244), re-arm {27s,32s} (:475-477), land-phase group.
local function onTailSmash(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "tail_smash", 500, function() onTailSmash(creature, guid) end)
        return
    end
    if sindragosaAirPhase[guid] then
        return
    end
    creature:CastSpell(creature, SPELL_TAIL_SMASH)
    schedule(guid, "tail_smash", {27000, 32000}, function() onTailSmash(creature, guid) end)
end

-- C++ EVENT_FROST_BREATH: DoCastVictim(P1 69649 / P2 73061 by third
-- phase) non-triggered, init {8s,12s} (:245), re-arm {20s,25s}
-- (:479-482), land-phase group.
local function onFrostBreath(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "frost_breath", 500, function() onFrostBreath(creature, guid) end)
        return
    end
    if sindragosaAirPhase[guid] then
        return
    end
    if sindragosaThirdPhase[guid] then
        creature:CastSpell(nil, SPELL_FROST_BREATH_P2)
    else
        creature:CastSpell(nil, SPELL_FROST_BREATH_P1)
    end
    schedule(guid, "frost_breath", {20000, 25000}, function() onFrostBreath(creature, guid) end)
end

-- C++ EVENT_UNCHAINED_MAGIC: Talk(SAY_UNCHAINED_MAGIC) +
-- DoCastSelf(SPELL_UNCHAINED_MAGIC) non-triggered, init {9s,14s}
-- (:246), re-arm {30s,35s} (:484-487), land-phase group. The
-- spell_sindragosa_unchained_magic healer/caster target filter stays
-- out (no IsPlayerHealer / specialization bridge).
local function onUnchainedMagic(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "unchained_magic", 500, function() onUnchainedMagic(creature, guid) end)
        return
    end
    if sindragosaAirPhase[guid] then
        return
    end
    creature:Talk(SAY_UNCHAINED_MAGIC)
    creature:CastSpell(creature, SPELL_UNCHAINED_MAGIC)
    schedule(guid, "unchained_magic", {30000, 35000}, function() onUnchainedMagic(creature, guid) end)
end

-- C++ EVENT_BLISTERING_COLD_YELL: Talk(SAY_BLISTERING_COLD) (:502-504).
local function onBlisteringColdYell(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "blistering_cold_yell", 500, function() onBlisteringColdYell(creature, guid) end)
        return
    end
    if sindragosaAirPhase[guid] then
        return
    end
    creature:Talk(SAY_BLISTERING_COLD)
end

-- C++ EVENT_BLISTERING_COLD: Talk(EMOTE_WARN_BLISTERING_COLD) +
-- DoCastSelf(SPELL_BLISTERING_COLD) non-triggered, 1s after icy grip
-- (:497-500), then the 5s yell above; land-phase group.
local function onBlisteringCold(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "blistering_cold", 500, function() onBlisteringCold(creature, guid) end)
        return
    end
    if sindragosaAirPhase[guid] then
        return
    end
    creature:Talk(EMOTE_WARN_BLISTERING_COLD)
    creature:CastSpell(creature, SPELL_BLISTERING_COLD)
    schedule(guid, "blistering_cold_yell", 5000, function() onBlisteringColdYell(creature, guid) end)
end

-- C++ EVENT_ICY_GRIP: DoCastSelf(SPELL_ICY_GRIP) non-triggered, init
-- 33.5s (:247), then EVENT_BLISTERING_COLD 1s (:493-495). No self
-- re-arm: the only re-seeds are EVENT_LAND_GROUND (:548-552) and
-- EVENT_THIRD_PHASE_CHECK RescheduleEvent (:551, {35s,40s}).
-- Land-phase group. The icy-grip jump-back leg rides the ev5
-- spell_sindragosa_icy_grip handler below.
local function onIcyGrip(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "icy_grip", 500, function() onIcyGrip(creature, guid) end)
        return
    end
    if sindragosaAirPhase[guid] then
        return
    end
    creature:CastSpell(creature, SPELL_ICY_GRIP)
    schedule(guid, "blistering_cold", 1000, function() onBlisteringCold(creature, guid) end)
end

-- C++ EVENT_LAND_GROUND (:545-553): re-seed the land-phase timers with
-- the post-landing init delays, then MoveLand(POINT_LAND_GROUND,
-- SindragosaLandPos) — the landing itself fires ev28 below.
local function onLandGround(creature, guid)
    schedule(guid, "cleave", {13000, 15000}, function() onCleave(creature, guid) end)
    schedule(guid, "tail_smash", {19000, 23000}, function() onTailSmash(creature, guid) end)
    schedule(guid, "frost_breath", {10000, 15000}, function() onFrostBreath(creature, guid) end)
    schedule(guid, "unchained_magic", {12000, 17000}, function() onUnchainedMagic(creature, guid) end)
    schedule(guid, "icy_grip", {35000, 40000}, function() onIcyGrip(creature, guid) end)
    creature:MoveLand(POINT_LAND_GROUND, SINDRAGOSA_LAND_POS[1], SINDRAGOSA_LAND_POS[2], SINDRAGOSA_LAND_POS[3])
end

-- C++ EVENT_LAND (:537-541): CancelEvent(EVENT_FROST_BOMB) +
-- MovePoint(POINT_LAND, SindragosaFlyInPos). The frost-bomb timer is
-- never seeded in this port (no positional-cast bridge for the
-- 69846 trigger, see below), so the cancel is a no-op by construction.
local function onLand(creature, guid)
    creature:MoveTo(POINT_LAND, SINDRAGOSA_FLY_IN_POS[1], SINDRAGOSA_FLY_IN_POS[2], SINDRAGOSA_FLY_IN_POS[3])
end

-- C++ EVENT_AIR_MOVEMENT_FAR (:525-528): MovePoint(POINT_AIR_PHASE_FAR,
-- SindragosaAirPosFar). The facing + EVENT_LAND 30s ride the FAR
-- arrival in MovementInform below.
local function onAirMovementFar(creature, guid)
    creature:MoveTo(POINT_AIR_PHASE_FAR, SINDRAGOSA_AIR_POS_FAR[1], SINDRAGOSA_AIR_POS_FAR[2], SINDRAGOSA_AIR_POS_FAR[3])
end

-- C++ EVENT_AIR_MOVEMENT (:521-523): MovePoint(POINT_AIR_PHASE,
-- SindragosaAirPos).
local function onAirMovement(creature, guid)
    creature:MoveTo(POINT_AIR_PHASE, SINDRAGOSA_AIR_POS[1], SINDRAGOSA_AIR_POS[2], SINDRAGOSA_AIR_POS[3])
end

-- C++ EVENT_AIR_PHASE (:505-519): _isInAirPhase = true, Talk(SAY_AIR_PHASE),
-- SetCanFly(true), SetDisableGravity(true), SetReactState(REACT_PASSIVE),
-- AttackStop, MoveTakeoff(POINT_TAKEOFF, pos + 17z),
-- CancelEventGroup(EVENT_GROUP_LAND_PHASE), re-arm 110s. Init 50s (:248).
local function onAirPhase(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "air_phase", 500, function() onAirPhase(creature, guid) end)
        return
    end
    sindragosaAirPhase[guid] = true
    creature:Talk(SAY_AIR_PHASE)
    creature:SetCanFly(true)
    creature:SetDisableGravity(true)
    creature:SetReactState(0) -- REACT_PASSIVE
    creature:AttackStop()
    creature:MoveTakeoff(POINT_TAKEOFF, creature:GetX(), creature:GetY(), creature:GetZ() + 17.0)
    cancelKey(guid, "cleave")
    cancelKey(guid, "tail_smash")
    cancelKey(guid, "frost_breath")
    cancelKey(guid, "unchained_magic")
    cancelKey(guid, "icy_grip")
    cancelKey(guid, "blistering_cold")
    cancelKey(guid, "blistering_cold_yell")
    schedule(guid, "air_phase", 110000, function() onAirPhase(creature, guid) end)
end

-- C++ EVENT_BERSERK: 10min single-fire — Talk(EMOTE_BERSERK_RAID) +
-- Talk(SAY_BERSERK) + DoCastSelf(SPELL_BERSERK) non-triggered
-- (:242/:458-460), bare timer across all phases.
local function onBerserk(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :450 (+ in-loop :563): 500ms IsCasting defer, cast not consumed (mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "berserk", 500, function() onBerserk(creature, guid) end)
        return
    end
    creature:Talk(EMOTE_BERSERK_RAID)
    creature:Talk(SAY_BERSERK)
    creature:CastSpell(creature, SPELL_BERSERK)
end

-- C++ EVENT_ICE_TOMB: CastSpell(nullptr, SPELL_ICE_TOMB_TARGET,
-- TRIGGERED_FULL_MASK) with SPELLVALUE_MAX_TARGETS=1 (:515-518) — the
-- nullptr runs the spell's implicit random-enemy selection, so the tomb
-- lands on a random raid member, not the victim. The Go cast path binds an
-- explicit target GUID only, so nil would always tomb the tank;
-- randomPlayerTarget restores the C++ pick (tank stays in the pool, per the
-- implicit targeting). Re-arm {16s,23s}.
local function randomPlayerTarget(creature, exclude)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local candidates = {}
    for _, plr in ipairs(GetPlayersInWorld()) do
        if plr:GetMapId() == mapId and plr:GetInstanceId() == instanceId
                and not plr:IsDead() and not plr:HasAura(SPELL_ICE_TOMB_UNTARGETABLE) then
            local excluded = false
            if exclude then
                for _, other in ipairs(exclude) do
                    if other == plr then
                        excluded = true
                        break
                    end
                end
            end
            if not excluded then
                candidates[#candidates + 1] = plr
            end
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ MovementInform POINT_AIR_PHASE (:405-413): triggered ice-tomb
-- cast with SPELLVALUE_MAX_TARGETS RAID_MODE(2, 5, 2, 6) — N random
-- players (no tank exclusion: the ev7
-- spell_sindragosa_ice_tomb_target filter stays out, no tank bridge;
-- the untargetable-aura leg rides randomPlayerTarget). The third-phase
-- Talk rides the same gate as the ground tomb cast. Then
-- SetFacingTo(pi), EVENT_AIR_MOVEMENT_FAR 1ms, EVENT_FROST_BOMB 9s
-- (the frost-bomb timer stays unseeded — no positional-cast bridge).
local function onAirPhaseIceTomb(creature, guid)
    local count = 2
    if creature:Is25ManRaid() then
        count = 5
        if creature:IsHeroic() then
            count = 6
        end
    end
    local picked = {}
    for _ = 1, count do
        local target = randomPlayerTarget(creature, picked)
        if target == nil then
            break
        end
        picked[#picked + 1] = target
        creature:CastSpell(target, SPELL_ICE_TOMB_TARGET, true)
        if sindragosaThirdPhase[guid] then
            creature:Talk(EMOTE_WARN_FROZEN_ORB, target)
        end
    end
end

-- C++ boss_sindragosa::MovementInform (:378-447): POINT_TAKEOFF (EFFECT)
-- -> EVENT_AIR_MOVEMENT 1ms; POINT_AIR_PHASE (POINT) -> ice-tomb cast +
-- facing + EVENT_AIR_MOVEMENT_FAR 1ms (+ EVENT_FROST_BOMB 9s, unseeded);
-- POINT_AIR_PHASE_FAR (POINT) -> facing + EVENT_LAND 30s; POINT_LAND
-- (POINT) -> EVENT_LAND_GROUND 1ms; POINT_LAND_GROUND (EFFECT) ->
-- fly/gravity off + REACT_DEFENSIVE + _isInAirPhase = false. The
-- asphyxiation summons.DoAction leg stays out (no summon-set bridge).
local function sindragosaMovementInform(event, creature, moveType, id)
    local guid = creature:GetGUID()
    if moveType == EFFECT_MOTION_TYPE and id == POINT_TAKEOFF then
        schedule(guid, "air_movement", 1, function() onAirMovement(creature, guid) end)
    elseif moveType == POINT_MOTION_TYPE and id == POINT_AIR_PHASE then
        onAirPhaseIceTomb(creature, guid)
        creature:SetFacingTo(math.pi)
        schedule(guid, "air_movement_far", 1, function() onAirMovementFar(creature, guid) end)
    elseif moveType == POINT_MOTION_TYPE and id == POINT_AIR_PHASE_FAR then
        creature:SetFacingTo(math.pi)
        schedule(guid, "land", 30000, function() onLand(creature, guid) end)
    elseif moveType == POINT_MOTION_TYPE and id == POINT_LAND then
        schedule(guid, "land_ground", 1, function() onLandGround(creature, guid) end)
    elseif moveType == EFFECT_MOTION_TYPE and id == POINT_LAND_GROUND then
        creature:SetCanFly(false)
        creature:SetDisableGravity(false)
        creature:SetReactState(1) -- REACT_DEFENSIVE
        sindragosaAirPhase[guid] = false
    end
end

local function onIceTomb(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :450 (+ in-loop :563); defer precedes the target pick: 500ms IsCasting defer, no re-arm consumed (mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "ice_tomb", 500, function() onIceTomb(creature, guid) end)
        return
    end
    local target = randomPlayerTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_ICE_TOMB_TARGET, true)
        -- spell_sindragosa_ice_tomb_target HandleSindragosaTalk: third-phase Talk to the hit unit.
        if sindragosaThirdPhase[guid] then
            creature:Talk(EMOTE_WARN_FROZEN_ORB, target)
        end
    end
    schedule(guid, "ice_tomb", {16000, 23000}, function() onIceTomb(creature, guid) end)
end

-- C++ EVENT_THIRD_PHASE_CHECK: Talk(SAY_PHASE_2) +
-- DoCastSelf(SPELL_MYSTIC_BUFFET, true); seeds EVENT_ICE_TOMB
-- {7s,10s} — the _isInAirPhase leg is vacuously true (no air-phase
-- machine in this port).
local function onThirdPhaseCheck(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :450 (+ in-loop :563): 500ms IsCasting defer, Talk/cast/seed not consumed (mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "third_phase_check", 500, function() onThirdPhaseCheck(creature, guid) end)
        return
    end
    -- C++ EVENT_THIRD_PHASE_CHECK (:548-553): if _isInAirPhase, re-check
    -- in 5s; else Talk + EVENT_ICE_TOMB {7s,10s} + RescheduleEvent
    -- (EVENT_ICY_GRIP, {35s,40s}) + DoCastSelf(MYSTIC_BUFFET).
    if sindragosaAirPhase[guid] then
        schedule(guid, "third_phase_check", 5000, function() onThirdPhaseCheck(creature, guid) end)
        return
    end
    creature:Talk(SAY_PHASE_2)
    creature:CastSpell(creature, SPELL_MYSTIC_BUFFET, true)
    schedule(guid, "ice_tomb", {7000, 10000}, function() onIceTomb(creature, guid) end)
    schedule(guid, "icy_grip", {35000, 40000}, function() onIcyGrip(creature, guid) end)
end

-- C++ boss_sindragosa::JustEngagedWith:
-- Talk(SAY_AGGRO) — the auriaya engage-port precedent; the
-- CheckRequiredBosses evade branch (:269-275) has no bridge.
local function sindragosaJustEngagedWith(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    -- C++ :277-278 — DoCastSelf(SPELL_FROST_AURA) +
    -- DoCastSelf(SPELL_PERMAEATING_CHILL), non-triggered.
    creature:CastSpell(creature, SPELL_FROST_AURA)
    creature:CastSpell(creature, SPELL_PERMAEATING_CHILL)
    creature:Talk(SAY_AGGRO)
    -- C++ :280 — instance->SetBossState(DATA_SINDRAGOSA, IN_PROGRESS),
    -- after Talk, before SetCombatPulseDelay (C++ arm order). PORTED
    -- 2026-11-06 via creature:SetBossState (the stale "no bridge"
    -- header claim is retracted — lua_creature_events.go:5917).
    creature:SetBossState(DATA_SINDRAGOSA, BOSS_STATE_IN_PROGRESS)
    creature:SetCombatPulseDelay(5) -- boss_sindragosa.cpp:281 (JustEngagedWith arm)
    creature:SetFarVisible(true) -- boss_sindragosa.cpp:283 (the setActive leg still has no bridge)
    creature:DoZoneInCombat() -- boss_sindragosa.cpp:284 (JustEngagedWith tail; bridge live engine/world/lua_creature_events.go, 2026-10-23)
    schedule(guid, "berserk", 600000, function() onBerserk(creature, guid) end)
    -- C++ Reset seeds (:242-248), re-keyed to engage (maiden convention):
    -- cleave 10s, tail-smash 20s, frost-breath {8s,12s}, unchained-magic
    -- {9s,14s}, icy-grip 33.5s, air-phase 50s.
    schedule(guid, "cleave", 10000, function() onCleave(creature, guid) end)
    schedule(guid, "tail_smash", 20000, function() onTailSmash(creature, guid) end)
    schedule(guid, "frost_breath", {8000, 12000}, function() onFrostBreath(creature, guid) end)
    schedule(guid, "unchained_magic", {9000, 14000}, function() onUnchainedMagic(creature, guid) end)
    schedule(guid, "icy_grip", 33500, function() onIcyGrip(creature, guid) end)
    schedule(guid, "air_phase", 50000, function() onAirPhase(creature, guid) end)
end

-- C++ Reset() _Reset() timer machinery — cancel/re-arm on combat
-- events (the maiden convention). C++ EnterEvadeMode override
-- (:287-292) suppresses the evade when _isInAirPhase +
-- EVADE_REASON_BOUNDARY — no air-phase flag exists in this port (the
-- phase machine is unported), so the suppression stays out; the base
-- evade path rides ev24 FAIL (ported above).
local function sindragosaLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function sindragosaReset(event, creature)
    -- C++ Reset() — BossAI::Reset() runs _Reset() first: SetBossState
    -- NOT_STARTED only when the stored state != DONE (the rotface
    -- precedent — ev23 fires on spawn/death/evade too, and a DONE boss
    -- must stay DONE). PORTED 2026-11-06 via creature:SetBossState.
    if creature:GetBossState(DATA_SINDRAGOSA) ~= BOSS_STATE_DONE then
        creature:SetBossState(DATA_SINDRAGOSA, BOSS_STATE_NOT_STARTED)
    end
    cancelTimers(creature:GetGUID())
    sindragosaThirdPhase[creature:GetGUID()] = nil
    sindragosaAirPhase[creature:GetGUID()] = nil
    -- Alive-gated like the tank-marker cast below: ev23 fires on
    -- death too (fireCreatureDied runs Reset before the kill-credit
    -- criteria read, kill.go:1091/:1400), and a death-time zero
    -- would clobber the fight's peak before the gate reads it.
    if creature:IsAlive() then
        creature:SetData(DATA_MYSTIC_BUFFET_STACK, 0) -- C++ Reset -> Initialize :233
    end
    -- C++ Reset(): DoCastSelf(SPELL_TANK_MARKER, true) (:241),
    -- unconditional one-shot. The engine fires event 23 on death too
    -- (Eluna reset-then-died order, lua_creature_events.go) but the C++ AI
    -- Reset() never runs on death, so the cast is alive-gated.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_TANK_MARKER, true)
        -- Fresh-spawn analog (stalagg/feugen precedent): C++ respawns a new
        -- creature at the template REACT_AGGRESSIVE, but the Go motion keeps
        -- the ev24 DEFENSIVE write below across respawns. C++ Reset() itself
        -- leaves react state untouched; on the evade path event 24 re-applies
        -- DEFENSIVE after this, so the transient here is harmless.
        creature:SetReactState(2)
    end
end

-- C++ boss_sindragosa::JustReachedHome (:294-300): BossAI::JustReachedHome()
-- is instance bookkeeping (no bridge), instance->SetBossState(FAIL) PORTED
-- 2026-11-06 via creature:SetBossState (ev24, marrowgar precedent);
-- SetCanFly(false) (:298) / SetDisableGravity(false) (:299) /
-- SetReactState(REACT_DEFENSIVE 1) (:300) ported in C++ arm order
-- (bridges live, lua_creature_events.go). The old "no bridges" note was
-- stale. Event 24 fires at the home-move completion (C++
-- HomeMovementGenerator::DoFinalize twin).
local function sindragosaReachHome(event, creature)
    -- C++ :297 — instance->SetBossState(DATA_SINDRAGOSA, FAIL), after
    -- BossAI::JustReachedHome() (no state) and before the fly/gravity/
    -- react arms (C++ order). PORTED 2026-11-06 via creature:SetBossState
    -- on ev24 (the marrowgar ev24 precedent; the stale "no event-24
    -- port precedent" header claim is retracted).
    creature:SetBossState(DATA_SINDRAGOSA, BOSS_STATE_FAIL)
    creature:SetCanFly(false)
    creature:SetDisableGravity(false)
    creature:SetReactState(1)
end

-- C++ boss_sindragosa::DamageTaken: !_isThirdPhase && !HealthAbovePct(35)
-- — single-flip latch (gothik precedent); schedules
-- EVENT_THIRD_PHASE_CHECK 1s (:403-410).
local function sindragosaDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    if not sindragosaThirdPhase[guid] and creature:GetHealthPct() <= 35 then
        sindragosaThirdPhase[guid] = true
        -- C++ DamageTaken (:403-410): CancelEvent(EVENT_AIR_PHASE) on the
        -- third-phase flip, then EVENT_THIRD_PHASE_CHECK 1s.
        cancelKey(guid, "air_phase")
        schedule(guid, "third_phase_check", 1000, function() onThirdPhaseCheck(creature, guid) end)
    end
    return true, damage
end

-- C++ boss_sindragosa::KilledUnit:
-- if (victim->GetTypeId() == TYPEID_PLAYER) Talk(SAY_KILL) — the
-- razuvious player-gated variant precedent.
local function sindragosaKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

-- C++ boss_sindragosa::JustDied:
-- Talk(SAY_DEATH) — the sjonnir JustDied-Talk precedent.
-- (the me->RemoveAllGameObjects() leg has no bridge — the garfrost
-- JustDied precedent; the _trappedPlayerGUID asphyxiation/ice-tomb
-- aura cleanup is cross-player GUID blocked — no bridge).
local function sindragosaJustDied(event, creature, killer)
    -- C++ _JustDied() resets the EventMap (boss_sindragosa.cpp:260):
    -- cancel the CreateLuaEvent timers — they have no death
    -- auto-cancel, so the self-re-arming ice-tomb / third-phase-check
    -- legs would keep firing on the corpse (festergut/rotface
    -- death-cancel precedent); drop the per-GUID third-phase latch
    -- like Reset does.
    local guid = creature:GetGUID()
    cancelTimers(guid)
    sindragosaThirdPhase[guid] = nil
    sindragosaAirPhase[guid] = nil
    -- C++ :260 — _JustDied() first: SetBossState DONE, before Talk
    -- (:261, C++ order). PORTED 2026-11-06 via creature:SetBossState.
    creature:SetBossState(DATA_SINDRAGOSA, BOSS_STATE_DONE)
    creature:Talk(SAY_DEATH)
    -- C++ :262-264 — Is25ManRaid() + HasAura(SPELL_SHADOWS_FATE)-gated
    -- DoCastAOE(SPELL_FROST_INFUSION_CREDIT, true) via self-cast
    -- (DoCastAOE = self-cast convention; C++ order kept).
    if creature:Is25ManRaid() and creature:HasAura(SPELL_SHADOWS_FATE) then
        creature:CastSpell(creature, SPELL_FROST_INFUSION_CREDIT, true)
    end
end

-- PORTED (2026-10-14): npc_spinestalker (37534) — Reset timer seeds
-- re-keyed to OnEnterCombat (the deathwhisper fanatic precedent):
-- EVENT_BELLOWING_ROAR DoCastSelf(SPELL_BELLOWING_ROAR) non-triggered
-- {20s,25s} -> {25s,30s} (:661/:737-739); EVENT_CLEAVE_SPINESTALKER
-- DoCastVictim(SPELL_CLEAVE_SPINESTALKER) non-triggered {10s,15s} ->
-- {10s,15s} (:662/:741-743); EVENT_TAIL_SWEEP
-- DoCastSelf(SPELL_TAIL_SWEEP) non-triggered {8s,12s} -> {22s,25s}
-- (:663/:745-747). Pure timer arms — no phase, random-target, or
-- summon gating (the maiden CreateLuaEvent precedent); the UpdateAI
-- CASTING gates are PORTED (500ms IsCasting defer, mal_ganis idiom
-- — IsCasting live, lua_creature_events.go:1950; the old "no bridge /
-- timers fire unconditionally" claim is retracted). The intro ACTION_START_FROSTWYRM flight machine stays out (ev38 dispatch
-- bridged; senders at_sindragosa_lair (boss_sindragosa.cpp:1492) + instance
-- OnCreatureRemove (instance_icecrown_citadel.cpp:469) unported;
-- MovePoint/MovementInform are bridged — the sindragosa air-phase machine
-- ports on them — but with no sender the receiver would be dead code).
local function onSpinestalkerRoar(creature, guid)
    -- C++ spinestalker UpdateAI pre-loop gate :730 (no in-loop return): 500ms IsCasting defer, no re-arm consumed (mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "roar", 500, function() onSpinestalkerRoar(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_BELLOWING_ROAR)
    schedule(guid, "roar", {25000, 30000}, function() onSpinestalkerRoar(creature, guid) end)
end

local function onSpinestalkerCleave(creature, guid)
    -- C++ spinestalker UpdateAI pre-loop gate :730 (no in-loop return): 500ms IsCasting defer, no re-arm consumed (mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "cleave", 500, function() onSpinestalkerCleave(creature, guid) end)
        return
    end
    creature:CastSpell(nil, SPELL_CLEAVE_SPINESTALKER)
    schedule(guid, "cleave", {10000, 15000}, function() onSpinestalkerCleave(creature, guid) end)
end

local function onSpinestalkerTailSweep(creature, guid)
    -- C++ spinestalker UpdateAI pre-loop gate :730 (no in-loop return): 500ms IsCasting defer, no re-arm consumed (mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "tailsweep", 500, function() onSpinestalkerTailSweep(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_TAIL_SWEEP)
    schedule(guid, "tailsweep", {22000, 25000}, function() onSpinestalkerTailSweep(creature, guid) end)
end

local function spinestalkerEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "roar", {20000, 25000}, function() onSpinestalkerRoar(creature, guid) end)
    schedule(guid, "cleave", {10000, 15000}, function() onSpinestalkerCleave(creature, guid) end)
    schedule(guid, "tailsweep", {8000, 12000}, function() onSpinestalkerTailSweep(creature, guid) end)
end

-- PORTED (2026-10-14): npc_rimefang_icc (37533) — JustEngagedWith
-- DoCastSelf(SPELL_FROST_AURA_RIMEFANG, true) (:850-853) +
-- EVENT_FROST_BREATH_RIMEFANG DoCastSelf(SPELL_FROST_BREATH)
-- non-triggered {12s,15s} -> {35s,40s} (:791/:868-871). EVENT_ICY_BLAST
-- PORTED 2026-11-09 (counter + MovePoint + ev28 FLY_IN arrival twin for
-- the moveTime re-arms + blast-cast chain). The
-- intro flight machine stays out (ev38 dispatch bridged; senders
-- at_sindragosa_lair (boss_sindragosa.cpp:1496) + instance OnCreatureRemove
-- (instance_icecrown_citadel.cpp:476) unported).
local function onRimefangFrostBreath(creature, guid)
    -- C++ rimefang UpdateAI pre-loop gate :862 (no in-loop return): 500ms IsCasting defer, no re-arm consumed (mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "frostbreath", 500, function() onRimefangFrostBreath(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_FROST_BREATH_RIMEFANG)
    schedule(guid, "frostbreath", {35000, 40000}, function() onRimefangFrostBreath(creature, guid) end)
end

-- C++ EVENT_ICY_BLAST_CAST (:905-921): counter-- ; while counter > 0,
-- random player target (SelectTarget Random, playerOnly),
-- SetFacingToObject + DoCast(SPELL_ICY_BLAST), re-arm 3s; at 0, victim =
-- SelectVictim -> REACT_DEFENSIVE + AttackStart + SetCanFly(false).
local function onRimefangIcyBlastCast(creature, guid)
    local counter = (rimefangBlastCounter[guid] or 1) - 1
    rimefangBlastCounter[guid] = counter
    if counter > 0 then
        local players = {}
        for _, target in ipairs(creature:GetAITargets()) do
            if target:GetObjectType() == "Player" and not target:IsDead() then
                players[#players + 1] = target
            end
        end
        if #players > 0 then
            local target = players[math.random(#players)]
            creature:SetFacingToObject(target)
            creature:CastSpell(target, SPELL_ICY_BLAST)
        end
        schedule(guid, "icy_blast_cast", 3000, function() onRimefangIcyBlastCast(creature, guid) end)
        return
    end
    local victim = creature:GetVictim()
    if victim ~= nil then
        creature:SetReactState(1) -- REACT_DEFENSIVE
        creature:AttackStart(victim)
        creature:SetCanFly(false)
    end
end

-- C++ EVENT_ICY_BLAST (:895-904): counter = RAID_MODE(5, 7, 6, 8),
-- SetReactState(REACT_PASSIVE), AttackStop, SetCanFly(true),
-- MovePoint(POINT_FROSTWYRM_FLY_IN, RimefangFlyPos); the moveTime +
-- {60s,70s} / moveTime + 250ms re-arms ride the FLY_IN arrival below.
local function onRimefangIcyBlast(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "icy_blast", 500, function() onRimefangIcyBlast(creature, guid) end)
        return
    end
    local counter = 5
    if creature:Is25ManRaid() then
        counter = 7
        if creature:IsHeroic() then
            counter = 8
        end
    elseif creature:IsHeroic() then
        counter = 6
    end
    rimefangBlastCounter[guid] = counter
    creature:SetReactState(0) -- REACT_PASSIVE
    creature:AttackStop()
    creature:SetCanFly(true)
    creature:MoveTo(POINT_FROSTWYRM_FLY_IN, RIMEFANG_FLY_POS[1], RIMEFANG_FLY_POS[2], RIMEFANG_FLY_POS[3])
end

-- C++ rimefang has no MovementInform leg for POINT_FROSTWYRM_FLY_IN —
-- the ICY_BLAST_CAST 250ms / ICY_BLAST {60s,70s} re-arms are scheduled
-- from moveTime, i.e. from the FLY_IN arrival; the ev28 arrival is
-- that twin (the darnavan precedent).
local function rimefangMovementInform(event, creature, moveType, id)
    if moveType ~= POINT_MOTION_TYPE or id ~= POINT_FROSTWYRM_FLY_IN then
        return
    end
    local guid = creature:GetGUID()
    schedule(guid, "icy_blast_cast", 250, function() onRimefangIcyBlastCast(creature, guid) end)
    schedule(guid, "icy_blast", {60000, 70000}, function() onRimefangIcyBlast(creature, guid) end)
end

local function rimefangEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    rimefangBlastCounter[guid] = nil
    creature:CastSpell(creature, SPELL_FROST_AURA_RIMEFANG, true)
    schedule(guid, "frostbreath", {12000, 15000}, function() onRimefangFrostBreath(creature, guid) end)
    -- C++ rimefang Reset (:798-800) seeds EVENT_ICY_BLAST {30s,35s},
    -- re-keyed to engage (maiden convention).
    schedule(guid, "icy_blast", {30000, 35000}, function() onRimefangIcyBlast(creature, guid) end)
end

-- PORTED (2026-10-14): npc_sindragosa_trash (37531 handler only) —
-- Reset timer seeds re-keyed to OnEnterCombat:
-- EVENT_FROSTWARDEN_ORDER_WHELP DoCastSelf(SPELL_ORDER_WHELP)
-- non-triggered 3s -> 3s (:943/:994-996);
-- EVENT_CONCUSSIVE_SHOCK DoCastSelf(SPELL_CONCUSSIVE_SHOCK)
-- non-triggered {8s,10s} -> {10s,13s} (:944/:998-1000). The whelp
-- (37532) carries no timer arms — NOT registered. The
-- JustAppeared / SetData instance legs stay out (no instance
-- bridge).
local function onHandlerOrderWhelp(creature, guid)
    -- C++ trash UpdateAI pre-loop gate :986 (no in-loop return): 500ms IsCasting defer, no re-arm consumed (mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "orderwhelp", 500, function() onHandlerOrderWhelp(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_ORDER_WHELP)
    schedule(guid, "orderwhelp", 3000, function() onHandlerOrderWhelp(creature, guid) end)
end

local function onHandlerConcussiveShock(creature, guid)
    -- C++ trash UpdateAI pre-loop gate :986 (no in-loop return): 500ms IsCasting defer, no re-arm consumed (mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "concussive", 500, function() onHandlerConcussiveShock(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_CONCUSSIVE_SHOCK)
    schedule(guid, "concussive", {10000, 13000}, function() onHandlerConcussiveShock(creature, guid) end)
end

local function handlerEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "orderwhelp", 3000, function() onHandlerOrderWhelp(creature, guid) end)
    schedule(guid, "concussive", {8000, 10000}, function() onHandlerConcussiveShock(creature, guid) end)
end

local function addLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function addReset(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ spinestalker Reset (:657-670) / rimefang Reset (:795-808):
-- SetCanFly(true) + SetDisableGravity(true) while !_summoned. The intro
-- DoAction that sets _summoned is unported (senders unported), so the
-- adds are perpetually pre-intro in this port and Reset always sets fly.
local function frostwyrmReset(event, creature)
    cancelTimers(creature:GetGUID())
    creature:SetCanFly(true)
    creature:SetDisableGravity(true)
end

-- C++ JustSummoned NPC_FROST_BOMB (boss_sindragosa.cpp:413-421):
-- CastSpell(summon, SPELL_FROST_BOMB_VISUAL 70022, triggered) +
-- CastSpell(summon, SPELL_BIRTH_NO_VISUAL 40031, triggered) in C++ order;
-- the 5500ms FrostBombExplosion BasicEvent's SPELL_FROST_BOMB 69845 cast
-- stays out (no exact Lua form); its visual-removal leg ports below via
-- a 5500ms CreateLuaEvent. LATENT: frost bombs spawn only from the
-- unported air-phase summon machine (anubrekhan latent precedent).
local function sindragosaJustSummoned(_, creature, summoned)
    if summoned:GetEntry() == ENTRY_FROST_BOMB then
        summoned:CastSpell(summoned, SPELL_FROST_BOMB_VISUAL, true)
        summoned:CastSpell(summoned, SPELL_BIRTH_NO_VISUAL, true)
        -- C++ FrostBombExplosion BasicEvent (boss_sindragosa.cpp:192-202,
        -- added at 5500ms): visual strip. Raw per-summon CreateLuaEvent, not
        -- the file's schedule() helper — its single per-GUID key would
        -- collapse concurrent bombs into one timer. The paired
        -- SPELL_FROST_BOMB 69845 cast stays out (no exact Lua form).
        CreateLuaEvent(function()
            summoned:RemoveAura(SPELL_FROST_BOMB_VISUAL)
        end, 5500)
    end
end

RegisterCreatureEvent(ENTRY_SINDRAGOSA, 1, sindragosaJustEngagedWith)
RegisterCreatureEvent(ENTRY_SINDRAGOSA, 2, sindragosaLeaveCombat)
RegisterCreatureEvent(ENTRY_SINDRAGOSA, 3, sindragosaKilledUnit)
RegisterCreatureEvent(ENTRY_SINDRAGOSA, 4, sindragosaJustDied)
-- C++ SpellHitTarget (boss_sindragosa.cpp:432-441): when the
-- difficulty-resolved 70127 hit lands on a unit, _mysticBuffetStack =
-- max(stack of that aura on the target) — the
-- achievement_all_you_can_eat latch (Go case in
-- engine/world/achievement_criteria_scripts.go). Dormant until the DBC
-- aura trigger (70128 -> 70127) runs: without Spell.dbc the helper hits
-- never occur and the slot stays 0.
local function sindragosaSpellHitTarget(event, creature, target, spellId)
    if spellId ~= SPELL_MYSTIC_BUFFET_HIT or not target then
        return
    end
    local aura = target:GetAura(spellId)
    if not aura then
        return
    end
    local stack = aura:GetStackAmount()
    if stack > creature:GetData(DATA_MYSTIC_BUFFET_STACK) then
        creature:SetData(DATA_MYSTIC_BUFFET_STACK, stack)
    end
end

RegisterCreatureEvent(ENTRY_SINDRAGOSA, 15, sindragosaSpellHitTarget)
RegisterCreatureEvent(ENTRY_SINDRAGOSA, 9, sindragosaDamageTaken)
RegisterCreatureEvent(ENTRY_SINDRAGOSA, 23, sindragosaReset)
RegisterCreatureEvent(ENTRY_SINDRAGOSA, 24, sindragosaReachHome)
RegisterCreatureEvent(ENTRY_SINDRAGOSA, 19, sindragosaJustSummoned)
RegisterCreatureEvent(ENTRY_SINDRAGOSA, 28, sindragosaMovementInform)
RegisterCreatureEvent(ENTRY_SPINESTALKER, 1, spinestalkerEnterCombat)
RegisterCreatureEvent(ENTRY_SPINESTALKER, 2, addLeaveCombat)
RegisterCreatureEvent(ENTRY_SPINESTALKER, 4, addLeaveCombat) -- C++ boss_sindragosa.cpp:678-681: JustDied _events.Reset() only
RegisterCreatureEvent(ENTRY_SPINESTALKER, 23, frostwyrmReset)
RegisterCreatureEvent(ENTRY_RIMEFANG, 1, rimefangEnterCombat)
RegisterCreatureEvent(ENTRY_RIMEFANG, 2, addLeaveCombat)
RegisterCreatureEvent(ENTRY_RIMEFANG, 4, addLeaveCombat) -- C++ boss_sindragosa.cpp:805-808: JustDied _events.Reset() only
RegisterCreatureEvent(ENTRY_RIMEFANG, 23, frostwyrmReset)
RegisterCreatureEvent(ENTRY_RIMEFANG, 28, rimefangMovementInform)
RegisterCreatureEvent(ENTRY_FROSTWARDEN_HANDLER, 1, handlerEnterCombat)
RegisterCreatureEvent(ENTRY_FROSTWARDEN_HANDLER, 2, addLeaveCombat)
RegisterCreatureEvent(ENTRY_FROSTWARDEN_HANDLER, 23, addReset)

-- C++ spell_sindragosa_frost_beacon::PeriodicTick
-- (boss_sindragosa.cpp:1228-1232): PreventDefaultAction; the aura
-- caster casts SPELL_ICE_TOMB_DAMAGE (70157) triggered on the aura
-- target each tick. Returns true, the PreventDefaultAction twin, so
-- the Go default trigger cast does not also fire.
local function frostBeaconTick(event, aura, target)
    local caster = aura:GetCaster()
    if caster then
        caster:CastSpell(target, SPELL_ICE_TOMB_DAMAGE, true)
    end
    return true
end

RegisterAuraEvent(SPELL_FROST_BEACON, 1, frostBeaconTick)

-- C++ spell_sindragosa_icy_grip::HandleScript (boss_sindragosa.cpp:1257-1271):
-- OnEffectHitTarget EFFECT_0 SPELL_EFFECT_SCRIPT_EFFECT — the hit unit
-- casts SPELL_ICY_GRIP_JUMP (70122) on the caster, triggered. ev5 fires
-- once per non-zero effect (boss_ai.go:3417/3420), so the effectIndex == 0
-- gate is load-bearing; no 70117 case in creatureEffectScriptEffect, so
-- no double-fire. Dormant until Spell.dbc ships.
local function icyGripScript(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    hitTarget:CastSpell(caster, SPELL_ICY_GRIP_JUMP, true)
end
RegisterSpellEvent(SPELL_ICY_GRIP, 5, icyGripScript)

-- C++ spell_sindragosa_mystic_buffet::FilterTargets (boss_sindragosa.cpp:1279-1290):
-- OnObjectAreaTargetSelect EFFECT_0 — remove targets not within LOS of
-- the caster. ev7 fires per selecting effect (lua_spell.go:738);
-- dormant until Spell.dbc ships. Binding 70128 is C++-verifiable (the
-- boss casts it, :551).
local function mysticBuffetFilter(event, spell, effectIndex, targets)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    for _, target in ipairs(targets:GetTargets()) do
        if not caster:IsWithinLOS(target) then
            targets:Remove(target)
        end
    end
end
RegisterSpellEvent(SPELL_MYSTIC_BUFFET, 7, mysticBuffetFilter)

-- C++ spell_rimefang_icy_blast::HandleTriggerMissile (boss_sindragosa.cpp:1298-1312):
-- OnEffectHit EFFECT_1 SPELL_EFFECT_TRIGGER_MISSILE — summon NPC_ICY_BLAST
-- (38223) at the explosion-target dest, TEMPSUMMON_TIMED_DESPAWN 40s, and
-- have it cast SPELL_ICY_BLAST_AREA (71380) triggered on itself. ev5
-- effectIndex == 1 gate; no 71376 case in creatureEffectScriptEffect, so
-- no double-fire. Dormant until Spell.dbc ships (no dest without a DBC row).
local function rimefangIcyBlastScript(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 1 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local x, y, z = spell:GetExplTargetDest()
    if x == nil or y == nil or z == nil then
        return
    end
    local summon = caster:SummonCreature(ENTRY_ICY_BLAST, x, y, z, 0, 2, 40000)
    if summon ~= nil then
        summon:CastSpell(summon, SPELL_ICY_BLAST_AREA, true)
    end
end
RegisterSpellEvent(SPELL_ICY_BLAST, 5, rimefangIcyBlastScript)

-- C++ spell_frostwarden_handler_order_whelp::FilterTargets (boss_sindragosa.cpp:1347-1362):
-- OnObjectAreaTargetSelect EFFECT_0 — drop non-players, then keep one
-- random player. ev7 fires per selecting effect (lua_spell.go:738);
-- dormant until Spell.dbc ships. Binding 71357 is C++-verifiable (the
-- handler casts it, :994-996).
local function orderWhelpFilter(event, spell, effectIndex, targets)
    if effectIndex ~= 0 then
        return
    end
    local players = {}
    for _, target in ipairs(targets:GetTargets()) do
        if target:GetObjectType() == "Player" then
            players[#players + 1] = target
        else
            targets:Remove(target)
        end
    end
    if #players <= 1 then
        return
    end
    local keep = players[math.random(#players)]
    for _, target in ipairs(players) do
        if target ~= keep then
            targets:Remove(target)
        end
    end
end
RegisterSpellEvent(SPELL_ORDER_WHELP, 7, orderWhelpFilter)

-- C++ spell_frostwarden_handler_order_whelp::HandleForcedCast (boss_sindragosa.cpp:1364-1381):
-- OnEffectHitTarget EFFECT_0 SPELL_EFFECT_FORCE_CAST — a random Frostwing
-- Whelp within 150y that is unmarked and shares the handler's frostwyrm
-- owner casts the effect-0 value (SPELL_FOCUS_FIRE 71350) triggered on
-- the hit unit. The marker/owner legs ride creature:GetData
-- (DATA_WHELP_MARKER / DATA_FROSTWYRM_OWNER); the marker is set only by
-- the unported focus_fire legs (no AddThreat bridge), and the owner
-- defaults 0 == 0 vacuously until the trash InitializeAI leg lands, so
-- in this port the selector keeps every whelp in range. ev5
-- effectIndex == 0 gate; dormant until Spell.dbc ships (value 0 ->
-- guarded no-op).
local function orderWhelpForcedCast(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetObjectType() ~= "Creature" then
        return
    end
    local owner = caster:GetData(DATA_FROSTWYRM_OWNER)
    local whelps = caster:GetCreaturesInGrid(ENTRY_FROSTWING_WHELP, 150)
    local pool = {}
    for _, whelp in ipairs(whelps) do
        if whelp:GetData(DATA_WHELP_MARKER) == 0 and whelp:GetData(DATA_FROSTWYRM_OWNER) == owner then
            pool[#pool + 1] = whelp
        end
    end
    if #pool == 0 then
        return
    end
    local value = spell:GetEffectValue(0)
    if value <= 0 then
        return
    end
    pool[math.random(#pool)]:CastSpell(hitTarget, value, true)
end
RegisterSpellEvent(SPELL_ORDER_WHELP, 5, orderWhelpForcedCast)
