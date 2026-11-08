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
-- Shutdown counter (SPELL_OVERLOAD_CIRCUIT 62399 vehicle spellclick leg;
-- threshold + aura-removal blocked, interrupt leg now bridged). NOTE: DelayEvents(20s, 0) at :431 is a C++ no-op
-- (EventMap.cpp:111 early-returns on group 0), so the shutdown arm
-- is fully C++-exact as ported.
-- The systems_shutdown AuraScript (stun/root + gathering-speed removal)
-- has no AddUnitState bridge — documented. The Shutout achievement
-- flag IS modeled (tick 417): spawn init (Initialize twin) + the
-- EVENT_SHUTDOWN clear leg in shutdownTick below. The
-- ClearUnitState(UNIT_STATE_STUNNED | UNIT_STATE_ROOT) line in EVENT_REPAIR
-- (:435) is likewise unmodeled (no ClearUnitState bridge); the
-- UpdateAI CASTING gates (own pre-loop :394 + in-loop :473) are
-- PORTED as a 500ms IsCasting defer in every ported timer tick
-- (mal_ganis idiom; IsCasting bridged lua_creature_events.go:1950).
-- JustDied — _JustDied() SetBossState(DATA_LEVIATHAN 0, DONE) PORTED
-- 2026-11-05 via creature:SetBossState (lua_creature_events.go:5917;
-- maiden precedent) + Talk(SAY_DEATH 2) (event 4; :325-331, state first,
-- then Talk); the flag/dynflag/npcflag cosmetics have no bridges.
-- Reset — _Reset() SetBossState(DATA_LEVIATHAN 0, NOT_STARTED) PORTED
-- 2026-11-05 on ev23 (maiden precedent, unconditional per that
-- convention; C++ gates on GetBossState != DONE, ScriptedCreature.cpp
-- :503-512, vacuous here — ev23 only fires on a live boss, and DONE is
-- only ever set by JustDied).
-- DOCUMENTED-ONLY (no registration beyond entry 33113):
-- JustEngagedWith / ActiveTower() — the BossAI::JustEngagedWith base leg
-- SetBossState(DATA_LEVIATHAN 0, IN_PROGRESS) is PORTED 2026-11-05 via
-- creature:SetBossState (lua_creature_events.go:5917; maiden precedent;
-- base-first, :275-277, before SetCombatPulseDelay/DoZoneInCombat). The
-- aggro talk is CONDITIONAL on tower state (SAY_AGGRO 0 when ActiveTowers
-- false; SAY_HARDMODE 4 when any tower up; SAY_TOWER_NONE 5 when all
-- destroyed). Tower state is driven by DoAction(1..4) tower-destruction
-- legs with no SetData/DoAction bridge; porting Talk(0) unconditionally
-- would mis-port the hardmode legs — documented, not registered.
-- EVENT_SUMMON (1s init / 2s re-arm, :284 / :417-:422) — DoSummonFlyer
-- NPC_MECHANOLIFT 33214 leg — SummonCreature (lua_creature_events.go:3399)
-- and MoveRandom are bridged, but the leg stays out: the summons.size() < 15
-- gate has no bridge (no summons-count method), DoSummonFlyer spawns at
-- flightZ 30 / radius 50 (CreatureAI.cpp:448) with no flyer-spawn twin,
-- and the mechanolift's own AI is unmodeled apart from its JustDied
-- (PORTED below, tick 386) — flagged candidate for a
-- dedicated tick, not ported here.
-- EVENT_THORIM_S_HAMMER / EVENT_MIMIRON_S_INFERNO / EVENT_HODIR_S_FURY /
-- EVENT_FREYA_S_WARD — hardmode tower-state-gated summon + Talk legs
-- (SAY_TOWER_STORM 9 / SAY_TOWER_FLAME 7 / SAY_TOWER_FROST 6 /
-- SAY_TOWER_NATURE 8); tower state has no bridge and the summon legs are
-- excluded.
-- SpellHitTarget (SPELL_PURSUED: _pursueTarget GUID + vehicle seat walk ->
-- Talk(EMOTE_PURSUE 12, passenger)) — no vehicle bridge.
-- SpellHit: stale "no SpellHit bridge" retracted (2026-10-21) — ev14
-- CreatureEventOnHitBySpell is bridged; the arms stay out on their own
-- machines: SPELL_START_THE_ENGINE 62472 vehicle-accessory install —
-- no vehicle bridge; SPELL_ELECTROSHOCK 62522
-- InterruptSpell(CURRENT_CHANNELED_SPELL) — stale "no InterruptSpell
-- Lua method" retracted 2026-11-05 (creature:InterruptSpell(2) is
-- bridged, lua_creature_events.go:987) — but the arm is vacuous in
-- both trees: it only cancels a live channel, and no leviathan arm
-- in the C++ file or the Lua port ever starts one (all casts
-- instant), so a handler would be dead code (snobold precedent);
-- SPELL_OVERLOAD_CIRCUIT 62399 ++Shutdown — the only consumer is the
-- early-4s shutdown (ScheduleEvent(EVENT_SHUTDOWN, 4s) +
-- InterruptNonMeleeSpells, now bridged) — still blocked on the
-- RAID_MODE 2/4-seat threshold (no raid-size bridge) and the
-- unbridged RemoveAurasDueToSpell leg, so the counter alone
-- stays dead state.
-- MovementInform (boss_flame_leviathan.cpp:570-576) — the
-- ACTION_MOVE_TO_CENTER_POSITION arrival leg (SetReactState(REACT_AGGRESSIVE)
-- + RemoveFlag(NON_ATTACKABLE | NOT_SELECTABLE | STUNNED)) rides the
-- unbridged MovementInform hook; the boss stays REACT_DEFENSIVE in Lua.
-- UpdateAI MoveTimer arm (:871-881) — GetVehicleKit()->HasEmptySeat(-1)
-- gated FindNearestCreature(NPC_CONTAINER, 50) + MovePoint(1) leg rides
-- the unported vehicle machine (GetVehicleKit/GetVehicle unbridged) and
-- the unbridged MovePoint — stays out.
-- GetData(DATA_UNBROKEN) / SetData(DATA_UNBROKEN) — the instance-script
-- Unbroken latch is cleared only by the spell_auto_repair SpellScript
-- (boss_flame_leviathan.cpp:1630-1645; no SpellScript bridge), so the
-- latch stays 1 in the Go-modeled fight; the instance write leg is
-- PORTED 2026-11-08 (tick 419, flameLeviathanSpawned below) and the Go
-- case reads the shared instanceData table. The DoAction
-- tower-destruction loot-mode ladder stays out (no loot-mode bridge);
-- the tower-destruction COUNT leg is modeled via the GO-destroyed
-- handlers below, and the hard-mode activation leg via the lorekeeper
-- gossip select + ev38 DoAction handler.
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
-- boss_flame_leviathan_safety_container (33218) / npc_mechanolift (33214,
-- JustDied PORTED below, tick 386 — dust explosion + liquid summon) /
-- npc_pool_of_tar (33189) / npc_colossus (33237, ulduar.h line 102) /
-- npc_thorims_hammer (33365/33364) / npc_mimirons_inferno (33370/33369) /
-- npc_hodirs_fury (33212/33108) / npc_freyas_ward (33367/33366) /
-- npc_freya_ward_summon — aura/timer/summon/move legs, no Talk arms anywhere
-- — no registrations EXCEPT the pool-of-tar DamageTaken immunity
-- (damage = 0, boss_flame_leviathan.cpp:909-912 — registered 2026-10-19,
-- event 9 numeric-return rewrite). npc_pool_of_tar SpellHit (:914-917):
-- FIRE-school hit && !HasAura(SPELL_BLAZE 62292) -> DoCastSelf(BLAZE,
-- true) — the ev14 handler receives only spellId (no spell-school lookup
-- bridge), so the SCHOOL_MASK_FIRE gate can't be reproduced —
-- documented-only.
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
-- achievement_three_car_garage_{demolisher,chopper,siege} — already
-- ported Go-side (vehicleCreatureEntry cases in
-- achievement_criteria_scripts.go); no Lua leg needed.
-- achievement_shutout, achievement_orbital_bombardment,
-- achievement_orbital_devastation, achievement_nuked_from_orbit,
-- achievement_orbit_uary — PORTED (tick 417): the FL SetData write
-- legs below (spawn init, EVENT_SHUTDOWN clear, lorekeeper
-- hard-mode activation, tower-destruction decrements); the Go cases
-- read them off the killed leviathan's SetData store
-- (bossLatchData). achievement_unbroken — PORTED 2026-11-08 (tick 419):
-- the instance latch write leg is flameLeviathanSpawned below (ev5,
-- instance ctor twin) and the Go case reads the shared instanceData
-- table; its only clear leg is the spell_auto_repair SpellScript
-- (boss_flame_leviathan.cpp:1630-1645; no SpellScript bridge), so the
-- latch stays 1 in the Go-modeled fight.
-- spell_overload_circuit (62399) / spell_tar_blaze (62292) /
-- spell_load_into_catapult / spell_auto_repair / spell_pursue /
-- spell_vehicle_throw_passenger — AuraScript/SpellScript bridges absent;
-- all join the no-SpellScript-bridge queue. spell_systems_shutdown
-- (62475) is PORTED below (RegisterAuraEvent apply/remove).

local ENTRY_FLAME_LEVIATHAN = 33113
local ENTRY_POOL_OF_TAR = 33189
local ENTRY_MECHANOLIFT = 33214
local ENTRY_LIQUID = 33189 -- NPC_LIQUID (boss_flame_leviathan.cpp:90), same entry as the pool of tar
local SPELL_DUSTY_EXPLOSION = 63360
local SPELL_LIQUID_PYRITE = 62494
local SPELL_DUST_CLOUD_IMPACT = 54740

-- DATA_LEVIATHAN = 0 (ulduar.h:36; BOSS_LEVIATHAN bossId).
local DATA_LEVIATHAN = 0

-- boss_flame_leviathan.cpp:143-144 Misc enum: the boss-AI
-- GetData slots the achievement OnChecks read.
local DATA_SHUTOUT = 29112912
local DATA_ORBIT_ACHIEVEMENTS = 1

-- boss_flame_leviathan.cpp:172 MiscellanousData enum.
local ACTION_START_HARD_MODE = 5

-- ulduar.h:108 NPC_LORE_KEEPER_OF_NORGANNON; boss_flame_leviathan.cpp
-- :105-108 GameObject entries for the four destructible towers.
local ENTRY_LORE_KEEPER_OF_NORGANNON = 33686
local GO_TOWER_OF_STORMS = 194377
local GO_TOWER_OF_FLAMES = 194371
local GO_TOWER_OF_FROST = 194370
local GO_TOWER_OF_LIFE = 194375

-- Per-tower destroyed flags (C++ towerOfStorms/towerOfLife/
-- towerOfFlames/towerOfFrost members, :256-261): a tower counts
-- down the orbit total at most once per hard-mode activation.
-- Recorded only when the decrement actually fires, so a tower
-- destroyed before activation (no vehicles exist yet, so
-- unreachable in practice) does not swallow a later decrement.
local towerDestroyed = {}

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
    if creature:IsCasting() then
        schedule(guid, "pursue", 500, function()
            pursueTick(creature, guid)
        end)
        return
    end
    creature:Talk(SAY_TARGET)
    creature:CastSpell(creature, SPELL_PURSUED)
    schedule(guid, "pursue", 35000, function()
        pursueTick(creature, guid)
    end)
end

-- C++ EVENT_MISSILE: DoCast(me, SPELL_MISSILE_BARRAGE 62400, true)
-- (self-cast TRIGGERED), {1.5s,4s} randtime init, 2s re-arm.
local function missileTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "missile", 500, function()
            missileTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_MISSILE_BARRAGE, true)
    schedule(guid, "missile", 2000, function()
        missileTick(creature, guid)
    end)
end

-- C++ EVENT_VENT: DoCastAOE(SPELL_FLAME_VENTS 62396) (non-triggered),
-- 20s init, 20s re-arm.
local function ventTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "vent", 500, function()
            ventTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_FLAME_VENTS)
    schedule(guid, "vent", 20000, function()
        ventTick(creature, guid)
    end)
end

-- C++ EVENT_SPEED: DoCastAOE(SPELL_GATHERING_SPEED 62375) (non-triggered),
-- 15s init, 15s re-arm.
local function speedTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "speed", 500, function()
            speedTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_GATHERING_SPEED)
    schedule(guid, "speed", 15000, function()
        speedTick(creature, guid)
    end)
end

-- C++ EVENT_SHUTDOWN (150s init :282, arm :424-:432): Talk(SAY_OVERLOAD) +
-- Talk(EMOTE_OVERLOAD) + me->CastSpell(me, SPELL_SYSTEMS_SHUTDOWN, true)
-- triggered self-cast; then EVENT_REPAIR at 4s (:433-:437): Talk(EMOTE_REPAIR)
-- + reschedules EVENT_SHUTDOWN 150s. Only the timer path is modeled; the
-- early 4s path rides the SpellHit-driven Shutdown counter
-- (threshold + aura-removal blocked; interrupt leg bridged).
-- DelayEvents(20s, 0) is a C++ no-op (EventMap.cpp:111 early-returns on
-- group 0), so nothing needs emulation — C++-exact.
local shutdownTick

local function repairTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "repair", 500, function()
            repairTick(creature, guid)
        end)
        return
    end
    creature:Talk(EMOTE_REPAIR)
    schedule(guid, "shutdown", 150000, function()
        shutdownTick(creature, guid)
    end)
end

shutdownTick = function(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "shutdown", 500, function()
            shutdownTick(creature, guid)
        end)
        return
    end
    creature:Talk(SAY_OVERLOAD)
    creature:Talk(EMOTE_OVERLOAD)
    creature:CastSpell(creature, SPELL_SYSTEMS_SHUTDOWN, true)
    -- C++ UpdateAI EVENT_SHUTDOWN arm (:428-429): the shutdown
    -- clears the Shutout latch (achievement_shutout gate) — the
    -- first shutdown ends the shutout, later ones are no-ops.
    creature:SetData(DATA_SHUTOUT, 0)
    schedule(guid, "repair", 4000, function()
        repairTick(creature, guid)
    end)
end

-- C++ JustEngagedWith (:275-287): BossAI::JustEngagedWith + react state +
-- ScheduleEvent x6 + ActiveTower() — the BossAI base SetBossState
-- (DATA_LEVIATHAN, IN_PROGRESS) leg is PORTED (base-first, :277); the four
-- unconditional timer casts, the REACT_PASSIVE arm (PORTED 2026-10-20 —
-- creature:SetReactState) and the EVENT_SHUTDOWN 150s timer path are
-- bridgeable; the tower-condition aggro talk, the shutdown early path
-- and EVENT_SUMMON are documented-only.
local function flameLeviathanEnterCombat(event, creature, target)
    -- C++ JustEngagedWith base-first (boss_flame_leviathan.cpp:277;
    -- ScriptedCreature.cpp:527-543): SetBossState(IN_PROGRESS), then
    -- SetCombatPulseDelay(5) + me-form DoZoneInCombat, before the
    -- tower-conditional Talk (ActiveTower, :287 — documented-only).
    -- The CheckRequiredBosses gate (:532) has no bridge (EnterEvadeMode
    -- unbridged) — stay-out, port only the state leg.
    creature:SetBossState(DATA_LEVIATHAN, 1)
    creature:SetCombatPulseDelay(5) -- BossAI::JustEngagedWith (ScriptedCreature.cpp:540)
    creature:DoZoneInCombat() -- BossAI::JustEngagedWith me-form arm (ScriptedCreature.cpp:542)
    creature:SetMeleeAttack(false) -- boss_flame_leviathan.cpp:372: custom UpdateAI never calls DoMeleeAttackIfReady (cast-only in C++)
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

-- C++ JustDied (boss_flame_leviathan.cpp:325-331): _JustDied() then
-- Talk(SAY_DEATH 2) — SetBossState(DATA_LEVIATHAN, DONE) first, then Talk
-- (maiden order); the flag/dynflag/npcflag cosmetics have no bridges.
local function flameLeviathanDied(event, creature, killer)
    creature:SetBossState(DATA_LEVIATHAN, 3)
    creature:Talk(SAY_DEATH)
    cancelTimers(creature:GetGUID())
end

-- C++ Reset (boss_flame_leviathan.cpp:265-273): _Reset() —
-- SetBossState(DATA_LEVIATHAN, NOT_STARTED) (ScriptedCreature.cpp:503-512;
-- maiden precedent sets it unconditionally on ev23) — then Shutdown = 0 /
-- _pursueTarget.Clear() (unmodeled Lua state, consumed only by the blocked
-- shutdown-threshold / SpellHitTarget legs) and SetReactState
-- (REACT_DEFENSIVE) last.
local function flameLeviathanReset(event, creature)
    cancelTimers(creature:GetGUID())
    creature:SetBossState(DATA_LEVIATHAN, 0)
    -- C++ boss_flame_leviathan.cpp:272: me->SetReactState(REACT_DEFENSIVE)
    -- (Reset last leg).
    creature:SetReactState(1) -- REACT_DEFENSIVE
end

-- C++ Initialize() (boss_flame_leviathan.cpp:228-241, ctor +
-- InitializeAI twin): ActiveTowersCount = 4, ActiveTowers =
-- false, Shutout = true, Unbroken = true. The Lua slot holds the
-- GetData RESULT, so the orbit slot starts at 0 (ActiveTowers
-- false reads 0, :355-358) and the shutout slot at 1. Reset does
-- NOT re-run Initialize in C++ (:265-273), so these legs live
-- only on spawn — a wipe leaves both latches as they were. The
-- GUID latch is the instance OnCreatureCreate twin
-- (instance_ulduar.cpp:87), so the lorekeeper gossip relay can
-- resolve the leviathan via GetInstanceCreature.
local function flameLeviathanSpawned(event, creature)
    creature:SetGuidData(DATA_LEVIATHAN)
    creature:SetData(DATA_SHUTOUT, 1)
    creature:SetData(DATA_ORBIT_ACHIEVEMENTS, 0)
    -- C++ instance ctor (instance_ulduar.cpp:183): Unbroken = true;
    -- the achievement_unbroken OnCheck reads the INSTANCE latch
    -- (instance->GetData(DATA_UNBROKEN 29052906) != 0,
    -- boss_flame_leviathan.cpp:1416-1423), so the write rides the
    -- shared instanceData table (blood-queen precedent). The only
    -- clear is spell_auto_repair (:1630-1645, no SpellScript
    -- bridge), so the latch stays 1 in the Go-modeled fight.
    -- PORTED 2026-11-08 (tick 419).
    local mapObj = creature:GetMap()
    local data = mapObj and mapObj:GetInstanceData() or nil
    if data then
        data["unbroken"] = 1
    end
end

-- C++ DoAction ACTION_START_HARD_MODE
-- (boss_flame_leviathan.cpp:549-556): ActiveTowers = true, all
-- four tower flags true, ActiveTowersCount stays 4 — so GetData
-- (DATA_ORBIT_ACHIEVEMENTS) starts returning 4. The loot-mode
-- ladder in the same arm stays out (no loot-mode bridge); the
-- tower-buff aura/event legs ride the unported ActiveTower()
-- machine and stay documented-only. Only the achievement latch
-- ports here.
local function flameLeviathanDoAction(event, creature, action)
    if action == ACTION_START_HARD_MODE then
        creature:SetData(DATA_ORBIT_ACHIEVEMENTS, 4)
    end
end

RegisterCreatureEvent(ENTRY_FLAME_LEVIATHAN, 1, flameLeviathanEnterCombat)
RegisterCreatureEvent(ENTRY_FLAME_LEVIATHAN, 2, flameLeviathanLeaveCombat)
RegisterCreatureEvent(ENTRY_FLAME_LEVIATHAN, 4, flameLeviathanDied)
RegisterCreatureEvent(ENTRY_FLAME_LEVIATHAN, 5, flameLeviathanSpawned)
RegisterCreatureEvent(ENTRY_FLAME_LEVIATHAN, 23, flameLeviathanReset)
RegisterCreatureEvent(ENTRY_FLAME_LEVIATHAN, 38, flameLeviathanDoAction)

-- C++ npc_lorekeeperAI::OnGossipSelect
-- (boss_flame_leviathan.cpp:1266-1293): menu 10477, option 0 ->
-- leviathan DoAction(ACTION_START_HARD_MODE) + vehicle spawns +
-- SetVisible(false). The lorekeeper has a single gossip option,
-- so listID == 0 is the C++ gate (razorscale commander
-- precedent); the menuId itself is not forwarded by the select
-- hook (gossip.go) and needs no check. The vehicle spawns stay
-- out: RAID_MODE(2, 5) counts have no raid-size bridge
-- (creature:IsHeroic() is difficulty, not 10/25). The
-- Delorah/Brann legs stay out (instance GUID latches + MovePoint
-- unbridged). Returns true when handled to veto the native arms
-- after doing the work (uniform Eluna gossip polarity, razorscale
-- precedent); the C++ `return false` is the AI-hook return, not
-- the Eluna veto polarity.
local function loreKeeperGossipSelect(event, player, creature, sender, action, code, listID)
    if listID ~= 0 then
        return false
    end
    player:GossipClearMenu()
    player:GossipComplete()
    creature:RemoveFlag(82, 1) -- UNIT_NPC_FLAGS, UNIT_NPC_FLAG_GOSSIP (razorscale precedent)
    local leviathan = creature:GetInstanceCreature(DATA_LEVIATHAN)
    if leviathan then
        leviathan:DoAction(ACTION_START_HARD_MODE)
    end
    creature:SetVisible(false)
    return true
end

RegisterCreatureGossipEvent(ENTRY_LORE_KEEPER_OF_NORGANNON, 2, loreKeeperGossipSelect)

-- C++ go_ulduar_tower::Destroyed (boss_flame_leviathan.cpp:1315-1332)
-- -> instance ProcessEvent (instance_ulduar.cpp:581-597) ->
-- leviathan DoAction(ACTION_TOWER_OF_*_DESTROYED 1..4), which
-- decrements ActiveTowersCount when that tower's flag is true
-- (:522-547). No instance-ProcessEvent bridge exists, so the leg
-- rides the GO destroyed hook instead (event 7,
-- gameobject_destructible.go:334, fired from the
-- destructible-building state transition): the tower GO resolves
-- the leviathan by entry in the same instance
-- (FindNearestCreature, 500 yd — the razorscale 500-yd arena
-- precedent; the entry filter admits no wrong match) and the
-- orbit slot drops by one. The slot read (GetData) is the
-- ActiveTowers gate: 0 while hard mode is off, so a pre-activation
-- destruction neither decrements nor records.
local function towerDestroyed(event, go, attacker)
    local entry = go:GetEntry()
    if towerDestroyed[entry] then
        return
    end
    local leviathan = go:FindNearestCreature(ENTRY_FLAME_LEVIATHAN, 500)
    if leviathan == nil then
        return
    end
    local count = leviathan:GetData(DATA_ORBIT_ACHIEVEMENTS)
    if count > 0 then
        towerDestroyed[entry] = true
        leviathan:SetData(DATA_ORBIT_ACHIEVEMENTS, count - 1)
    end
end

RegisterGameObjectEvent(GO_TOWER_OF_STORMS, 7, towerDestroyed)
RegisterGameObjectEvent(GO_TOWER_OF_FLAMES, 7, towerDestroyed)
RegisterGameObjectEvent(GO_TOWER_OF_FROST, 7, towerDestroyed)
RegisterGameObjectEvent(GO_TOWER_OF_LIFE, 7, towerDestroyed)

-- C++ npc_mechanoliftAI::JustDied (boss_flame_leviathan.cpp:852-862):
-- MoveTargetedHome + self-cast DUSTY_EXPLOSION + DoSummon NPC_LIQUID
-- (type 5 CORPSE_TIMED_DESPAWN, 0ms — the C++ DoSummon(entry, me, 0)
-- twin) + liquid self-casts LIQUID_PYRITE and DUST_CLOUD_IMPACT,
-- both triggered. Every leg bridged (MoveHome, CastSpell,
-- SummonCreature returning the summoned creature).
local function mechanoliftDied(event, creature, killer)
    creature:MoveHome()
    creature:CastSpell(creature, SPELL_DUSTY_EXPLOSION)
    local liquid = creature:SummonCreature(ENTRY_LIQUID, creature:GetX(), creature:GetY(), creature:GetZ(), creature:GetO(), 5, 0)
    if liquid then
        liquid:CastSpell(liquid, SPELL_LIQUID_PYRITE, true)
        liquid:CastSpell(liquid, SPELL_DUST_CLOUD_IMPACT, true)
    end
end
RegisterCreatureEvent(ENTRY_MECHANOLIFT, 4, mechanoliftDied)

-- C++ npc_pool_of_tar::DamageTaken (boss_flame_leviathan.cpp:909-912):
-- damage = 0 unconditionally (NPC_LIQUID 33189 local enum). Event 9's
-- second return rewrites damage (fireCreatureDamageTaken; halazzi
-- precedent).
local function poolOfTarDamageTaken(event, creature, attacker, damage)
    return false, 0
end

RegisterCreatureEvent(ENTRY_POOL_OF_TAR, 9, poolOfTarDamageTaken)

-- C++ spell_systems_shutdown_AuraScript (boss_flame_leviathan.cpp:1667-1695):
-- EFFECT_0 SPELL_AURA_MOD_DAMAGE_PERCENT_TAKEN, AURA_EFFECT_HANDLE_REAL.
-- OnApply the owner creature gains UNIT_STATE_STUNNED|ROOT, the
-- UNIT_FLAG_STUNNED flag, and loses its Gathering Speed aura (62375);
-- OnRemove the flag is cleared. The AddUnitState leg has no bridge and
-- stays out (the flag is the client-visible half); the flag and
-- RemoveAura legs ride the bridged SetFlag/RemoveFlag and RemoveAura.
local UNIT_FIELD_FLAGS = 59 -- UpdateFields.h:117 (OBJECT_END + 0x35)
local UNIT_FLAG_STUNNED = 0x00040000 -- UnitDefines.h:142

local function systemsShutdownApply(event, aura, target)
    target:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_STUNNED)
    target:RemoveAura(SPELL_GATHERING_SPEED)
end

local function systemsShutdownRemove(event, aura, target)
    target:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_STUNNED)
end

RegisterAuraEvent(SPELL_SYSTEMS_SHUTDOWN, 2, systemsShutdownApply)
RegisterAuraEvent(SPELL_SYSTEMS_SHUTDOWN, 3, systemsShutdownRemove)
