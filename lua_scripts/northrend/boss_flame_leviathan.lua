-- Flame Leviathan (Ulduar) — Lua port of
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_flame_leviathan.cpp
-- (boss_flame_leviathan (CreatureScript) via
-- GetUlduarAI<boss_flame_leviathanAI>(creature) (BossAI, BOSS_LEVIATHAN = 0,
-- ulduar.h line 36)).
-- Entry: 33113 Flame Leviathan (ulduar.h NPC_FLAME_LEVIATHAN, line 111).
-- Sole-source verified: whole-server-tree grep for "boss_flame_leviathan"
-- hits boss_flame_leviathan.cpp (+ the loader decl/call lines for
-- AddSC_boss_flame_leviathan) only; zero sql/ hits.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timer casts ride the CreateLuaEvent convention (sartharion /
-- maiden precedent): GUID-keyed timers table, cancel on events 2/4/23.
-- Ported arms (C++-exact for all modeled arms):
-- EVENT_PURSUE — Talk(SAY_TARGET 3) + DoCast(SPELL_PURSUED 62374)
-- (non-triggered), 1ms init / 35s re-arm (:279 / :401-:404). The single-arg
-- DoCast resolves its target via AISpellInfo and the spell script
-- re-selects it (TARGET_UNIT_SRC_AREA_ENEMY +
-- FlameLeviathanPursuedTargetSelector — salvaged demolisher/siege vehicles
-- in the Formation Grounds), so the Lua cast is self-targeted:
-- creature:CastSpell(creature, spell) (the 03:44 tick corrected an earlier
-- CastSpell(nil, ...) victim-cast misread).
-- EVENT_MISSILE — DoCast(me, SPELL_MISSILE_BARRAGE 62400, true) (self-cast
-- TRIGGERED), {1.5s,4s} randtime init / 2s re-arm (:280 / :406-:408); the
-- 3-arg ScheduleEvent is the minTime/maxTime overload (EventMap.h:135).
-- EVENT_VENT — DoCastAOE(SPELL_FLAME_VENTS 62396) (non-triggered), 20s init /
-- 20s re-arm (:281 / :410-:412); DoCastAOE lowers to self-cast
-- creature:CastSpell(creature, spell).
-- EVENT_SPEED — DoCastAOE(SPELL_GATHERING_SPEED 62375) (non-triggered), 15s
-- init / 15s re-arm (:283 / :414-:416).
-- EVENT_SHUTDOWN — 150s init (:282), arm at :424-:432: Talk(SAY_OVERLOAD 11)
-- + Talk(EMOTE_OVERLOAD 13) + me->CastSpell(me, SPELL_SYSTEMS_SHUTDOWN 62475,
-- true) triggered self-cast; EVENT_REPAIR 4s later (:433-:437):
-- Talk(EMOTE_REPAIR 14) + reschedules EVENT_SHUTDOWN 150s. Only the 150s
-- timer path is modeled — the early 4s path is gated on the SpellHit-driven
-- Shutdown counter (SPELL_OVERLOAD_CIRCUIT 62399 vehicle spellclick leg, no
-- bridge). NOTE: DelayEvents(20s, 0) at :431 is a C++ no-op
-- (EventMap.cpp:111 early-returns on group 0), so the shutdown arm
-- is fully C++-exact as ported.
-- The systems_shutdown AuraScript (stun/root + gathering-speed removal) and the
-- Shutout achievement flag have no bridges — documented. The
-- ClearUnitState(UNIT_STATE_STUNNED | UNIT_STATE_ROOT) line in EVENT_REPAIR
-- (:435) is likewise unmodeled — no live unit-state read bridge (IsCasting is constant-false — Go casts are fire-and-forget).
-- JustDied — Talk(SAY_DEATH 2) (event 4; :331; the _JustDied() passthrough
-- and the flag/dynflag/npcflag cosmetics have no bridges).
-- DOCUMENTED-ONLY (no registration beyond entry 33113):
-- JustEngagedWith / ActiveTower() — the aggro talk is CONDITIONAL on tower
-- state (SAY_AGGRO 0 when ActiveTowers false; SAY_HARDMODE 4 when any tower
-- up; SAY_TOWER_NONE 5 when all destroyed). Tower state is driven by
-- DoAction(1..4) tower-destruction legs with no SetData/DoAction bridge;
-- porting Talk(0) unconditionally would mis-port the hardmode legs —
-- documented, not registered.
-- EVENT_SUMMON (1s init / 2s re-arm, :284 / :417-:422) — DoSummonFlyer
-- NPC_MECHANOLIFT 33214 leg — no summon bridge.
-- EVENT_THORIM_S_HAMMER / EVENT_MIMIRON_S_INFERNO / EVENT_HODIR_S_FURY /
-- EVENT_FREYA_S_WARD — hardmode tower-state-gated summon + Talk legs
-- (SAY_TOWER_STORM 9 / SAY_TOWER_FLAME 7 / SAY_TOWER_FROST 6 /
-- SAY_TOWER_NATURE 8); tower state has no bridge and the summon legs are
-- excluded.
-- SpellHitTarget (SPELL_PURSUED: _pursueTarget GUID + vehicle seat walk ->
-- Talk(EMOTE_PURSUE 12, passenger)) — no vehicle bridge.
-- SpellHit (SPELL_START_THE_ENGINE 62472 vehicle-accessory install —
-- no vehicle bridge; SPELL_ELECTROSHOCK 62522
-- InterruptSpell(CURRENT_CHANNELED_SPELL) — no interrupt bridge
-- (CastSpell itself is bridged; the arm interrupts, it doesn't cast);
-- SPELL_OVERLOAD_CIRCUIT 62399 ++Shutdown — the early-4s shutdown
-- consumer calls InterruptNonMeleeSpells, also unbridged).
-- GetData(DATA_SHUTOUT / DATA_UNBROKEN / DATA_ORBIT_ACHIEVEMENTS) /
-- SetData(DATA_UNBROKEN) / DoAction tower-destruction loot-mode ladder —
-- no GetData/SetData/DoAction bridges.
-- DoBatteringRamIfReady — _pursueTarget + attack-timer legs — no bridges.
-- SAY_SLAY 1 is defined but never used in C++ (no KilledUnit override) —
-- no event 3 arm.
-- boss_flame_leviathan_seat (33114) — PassengerBoarded vehicle legs ->
-- Talk(SAY_PLAYER_RIDING 10) — no vehicle / cross-AI Talk bridge.
-- boss_flame_leviathan_defense_cannon — SPELL_NAPALM 63666 timer leg is
-- vehicle-gated random-target (CanAIAttack) — no vehicle/target bridges;
-- Reset DoCast(me, AURA_STEALTH_DETECTION 18950) rides the same
-- unregistered-cannon stay-out.
-- boss_flame_leviathan_defense_turret (33142, ulduar.h line 114) /
-- boss_flame_leviathan_overload_device (33143, ulduar.h line 115) /
-- boss_flame_leviathan_safety_container (33218) / npc_mechanolift (33214) /
-- npc_pool_of_tar (33189) / npc_colossus (33237, ulduar.h line 102) /
-- npc_thorims_hammer (33365/33364) / npc_mimirons_inferno (33370/33369) /
-- npc_hodirs_fury (33212/33108) / npc_freyas_ward (33367/33366) /
-- npc_freya_ward_summon — aura/timer/summon/move legs, no Talk arms anywhere
-- — no registrations EXCEPT the pool-of-tar DamageTaken immunity
-- (damage = 0, boss_flame_leviathan.cpp:909-912 — registered 2026-10-19,
-- event 9 numeric-return rewrite).
-- npc_brann_bronzebeard_ulduar_intro / npc_lorekeeper — gossip legs:
-- the gossip bridge IS live (RegisterCreatureGossipEvent,
-- engine/world/gossip.go:129/369) — the "no gossip bridge" claim was
-- stale; the arms stay documented-only because C++ gates them on
-- OnGossipSelect menuId (10355 / 10477), which the Lua select hook
-- never receives (sender/action only — gossip.go:351); the lorekeeper
-- Talk call is commented out in C++ (dead code).
-- go_ulduar_tower — GameObjectScript — RegisterGameObjectGossipEvent
-- exists (runtime.go:802); the arm's legs (tower destruction state)
-- stay documented-only on the instance/GO-state machine.
-- achievement_three_car_garage_{demolisher,chopper,siege}, achievement_shutout,
-- achievement_unbroken, achievement_orbital_bombardment,
-- achievement_orbital_devastation, achievement_nuked_from_orbit,
-- achievement_orbit_uary — GetData/vehicle-based OnCheck legs — no
-- achievement/GetData bridges; all nine join the unmodeled-achievement queue.
-- spell_overload_circuit (62399) / spell_tar_blaze (62292) /
-- spell_load_into_catapult / spell_systems_shutdown (62475) / spell_auto_repair /
-- spell_pursue / spell_vehicle_throw_passenger — AuraScript/SpellScript
-- bridges absent; all join the no-SpellScript-bridge queue.

local ENTRY_FLAME_LEVIATHAN = 33113
local ENTRY_POOL_OF_TAR = 33189

local SAY_TARGET = 3
local SAY_DEATH  = 2
local SAY_OVERLOAD = 11
local EMOTE_OVERLOAD = 13
local EMOTE_REPAIR = 14

local SPELL_PURSUED         = 62374
local SPELL_GATHERING_SPEED = 62375
local SPELL_FLAME_VENTS     = 62396
local SPELL_MISSILE_BARRAGE = 62400
local SPELL_SYSTEMS_SHUTDOWN = 62475

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

-- C++ EVENT_PURSUE: Talk(SAY_TARGET 3) + DoCast(SPELL_PURSUED 62374)
-- (single-arg DoCast resolves SELF via AISpellInfo default; the
-- spell script re-selects the target), 1ms init, 35s re-arm.
local function pursueTick(creature, guid)
    creature:Talk(SAY_TARGET)
    creature:CastSpell(creature, SPELL_PURSUED)
    schedule(guid, "pursue", 35000, function()
        pursueTick(creature, guid)
    end)
end

-- C++ EVENT_MISSILE: DoCast(me, SPELL_MISSILE_BARRAGE 62400, true)
-- (self-cast TRIGGERED), {1.5s,4s} randtime init, 2s re-arm.
local function missileTick(creature, guid)
    creature:CastSpell(creature, SPELL_MISSILE_BARRAGE, true)
    schedule(guid, "missile", 2000, function()
        missileTick(creature, guid)
    end)
end

-- C++ EVENT_VENT: DoCastAOE(SPELL_FLAME_VENTS 62396) (non-triggered),
-- 20s init, 20s re-arm.
local function ventTick(creature, guid)
    creature:CastSpell(creature, SPELL_FLAME_VENTS)
    schedule(guid, "vent", 20000, function()
        ventTick(creature, guid)
    end)
end

-- C++ EVENT_SPEED: DoCastAOE(SPELL_GATHERING_SPEED 62375) (non-triggered),
-- 15s init, 15s re-arm.
local function speedTick(creature, guid)
    creature:CastSpell(creature, SPELL_GATHERING_SPEED)
    schedule(guid, "speed", 15000, function()
        speedTick(creature, guid)
    end)
end

-- C++ EVENT_SHUTDOWN (150s init :282, arm :424-:432): Talk(SAY_OVERLOAD) +
-- Talk(EMOTE_OVERLOAD) + me->CastSpell(me, SPELL_SYSTEMS_SHUTDOWN, true)
-- triggered self-cast; then EVENT_REPAIR at 4s (:433-:437): Talk(EMOTE_REPAIR)
-- + reschedules EVENT_SHUTDOWN 150s. Only the timer path is modeled; the
-- early 4s path rides the SpellHit-driven Shutdown counter (no bridge).
-- DelayEvents(20s, 0) is a C++ no-op (EventMap.cpp:111 early-returns on
-- group 0), so nothing needs emulation — C++-exact.
local shutdownTick

local function repairTick(creature, guid)
    creature:Talk(EMOTE_REPAIR)
    schedule(guid, "shutdown", 150000, function()
        shutdownTick(creature, guid)
    end)
end

shutdownTick = function(creature, guid)
    creature:Talk(SAY_OVERLOAD)
    creature:Talk(EMOTE_OVERLOAD)
    creature:CastSpell(creature, SPELL_SYSTEMS_SHUTDOWN, true)
    schedule(guid, "repair", 4000, function()
        repairTick(creature, guid)
    end)
end

-- C++ JustEngagedWith: BossAI::JustEngagedWith + react state + ScheduleEvent
-- x6 + ActiveTower() — the four unconditional timer casts, the
-- REACT_PASSIVE arm (PORTED 2026-10-20 — creature:SetReactState) and
-- the EVENT_SHUTDOWN 150s timer path are bridgeable; the
-- tower-condition aggro talk, the shutdown early path and EVENT_SUMMON
-- are documented-only.
local function flameLeviathanEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    -- C++ boss_flame_leviathan.cpp:278: me->SetReactState(REACT_PASSIVE)
    -- (JustEngagedWith: flip before EVENT_PURSUE).
    creature:SetReactState(0) -- REACT_PASSIVE
    schedule(guid, "pursue", 1, function()
        pursueTick(creature, guid)
    end)
    schedule(guid, "missile", {1500, 4000}, function()
        missileTick(creature, guid)
    end)
    schedule(guid, "vent", 20000, function()
        ventTick(creature, guid)
    end)
    schedule(guid, "speed", 15000, function()
        speedTick(creature, guid)
    end)
    schedule(guid, "shutdown", 150000, function()
        shutdownTick(creature, guid)
    end)
end

local function flameLeviathanLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ JustDied: _JustDied() + Talk(SAY_DEATH 2) — only the Talk arm
-- is bridgeable.
local function flameLeviathanDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(SAY_DEATH)
end

local function flameLeviathanReset(event, creature)
    cancelTimers(creature:GetGUID())
    -- C++ boss_flame_leviathan.cpp:272: me->SetReactState(REACT_DEFENSIVE)
    -- (Reset last leg).
    creature:SetReactState(1) -- REACT_DEFENSIVE
end

RegisterCreatureEvent(ENTRY_FLAME_LEVIATHAN, 1, flameLeviathanEnterCombat)
RegisterCreatureEvent(ENTRY_FLAME_LEVIATHAN, 2, flameLeviathanLeaveCombat)
RegisterCreatureEvent(ENTRY_FLAME_LEVIATHAN, 4, flameLeviathanDied)
RegisterCreatureEvent(ENTRY_FLAME_LEVIATHAN, 23, flameLeviathanReset)

-- C++ npc_pool_of_tar::DamageTaken (boss_flame_leviathan.cpp:909-912):
-- damage = 0 unconditionally (NPC_LIQUID 33189 local enum). Event 9's
-- second return rewrites damage (fireCreatureDamageTaken; halazzi
-- precedent).
local function poolOfTarDamageTaken(event, creature, attacker, damage)
    return false, 0
end

RegisterCreatureEvent(ENTRY_POOL_OF_TAR, 9, poolOfTarDamageTaken)
