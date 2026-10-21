-- Razorscale (Ulduar) — Lua port of
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_razorscale.cpp
-- (boss_razorscale (CreatureScript) via
-- RegisterUlduarCreatureAI<boss_razorscale> (BossAI),
-- BOSS_RAZORSCALE = 2 (ulduar.h:38); registered from inside
-- AddSC_boss_razorscale() — the FOURTH group of the "// Ulduar"
-- block (loader decl :113 / call :308 in
-- northrend_script_loader.cpp, verified this run; preceded by
-- AddSC_boss_ignis() decl :112 / call :307; followed by
-- AddSC_boss_xt002() decl :114 / call :309).
-- Entry: 33186 Razorscale (ulduar.h NPC_RAZORSCALE, line 63 —
-- entry-verifiable; the CreatureScript ScriptName binding is
-- DB-side as usual).
-- Talk ids (Says, boss_razorscale.cpp:36-55, verified this run):
-- Razorscale EMOTE_PERMA_GROUND 0, EMOTE_BREATH 1, EMOTE_BERSERK 2
-- (the Razorscale Talk arm uses the Razorscale ids, not the
-- expedition commander/engineer Say ids).
-- Sole-source verified: whole-server-tree grep for each of the
-- twenty script names ("boss_razorscale",
-- "npc_expedition_commander", "npc_expedition_defender",
-- "npc_expedition_trapper", "npc_expedition_engineer",
-- "npc_razorscale_spawner", "npc_darkrune_watcher",
-- "npc_darkrune_guardian", "npc_darkrune_sentinel",
-- "npc_razorscale_harpoon_fire_state",
-- "npc_razorscale_devouring_flame", "go_razorscale_harpoon",
-- "go_razorscale_mole_machine", "spell_razorscale_flame_breath",
-- "spell_razorscale_summon_iron_dwarves",
-- "spell_razorscale_fuse_armor", "spell_razorscale_firebolt",
-- "achievement_iron_dwarf_medium_rare", "achievement_quick_shave")
-- hits boss_razorscale.cpp (+ the loader decl/call lines for
-- AddSC_boss_razorscale) only; zero sql/ hits for all twenty —
-- the audited artifact this run. No second razorscale lua exists.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4
-- OnDied, 23 OnReset. Timer-driven casts ride the CreateLuaEvent
-- convention (baltharus / maiden / auriaya precedent):
-- module-local GUID-keyed timers table, cancelTimers on events
-- 2/4/23, schedule(guid, key, delayMs, fn);
-- creature:CastSpell(nil, spell) = DoCastVictim (2-arg,
-- non-triggered), creature:CastSpell(creature, spell) =
-- DoCastSelf / DoCastAOE, third arg true = triggered,
-- {minMs, maxMs} = randtime re-arm. Melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms; line refs are
-- boss_razorscale.cpp):
-- JustEngagedWith (:356-365) — the BossAI::JustEngagedWith
-- passthrough, the SendEncounterUnit engage leg, the
-- ScheduleAirPhaseEvents leg (phase machine), the
-- summons.DoAction(ACTION_START_FIGHT) leg and the HandleMusic
-- leg have no bridges — event 1 models only the EVENT_BERSERK
-- timer leg (the auriaya engage-port precedent).
-- EVENT_BERSERK (:572-575) — DoCastSelf(SPELL_BERSERK 47008,
-- TRIGGERED) + Talk(EMOTE_BERSERK 2, me), 900s init (:362), no
-- re-arm, no phase gate (scheduled bare in JustEngagedWith, never
-- canceled — the auriaya/ignis triggered-berserk precedent;
-- cast-before-Talk order kept).
-- JustDied (:526-531) — _JustDied() + SendEncounterUnit disengage
-- + HandleMusic(false); no Talk arms; event 4 models only the
-- timer wipe (the auriaya JustDied-wipe precedent).
-- DOCUMENTED-ONLY (in this header; no registration beyond entry
-- 33186):
-- The whole air/ground/perma-ground phase machine — DoAction
-- (ACTION_START_FIGHT / ACTION_GROUND_PHASE /
-- ACTION_START_PERMA_GROUND / ACTION_RETURN_TO_BASE — no DoAction
-- bridge), MovementInform (POINT_RAZORSCALE_FLIGHT /
-- POINT_RAZORSCALE_GROUND / POINT_RAZORSCALE_TAKEOFF /
-- POINT_RAZORSCALE_LAND — no MovementInform bridge),
-- MoveSplineInit / MovePoint / MoveLand / MoveTakeoff legs — no
-- MotionMaster / spline bridges; SetPhase(PHASE_AIR /
-- PHASE_GROUND / PHASE_PERMA_GROUND) gates every other timer arm.
-- EVENT_FIREBALL (:576-580) — SelectTarget(Random) + DoCast
-- (SPELL_FIREBALL 63815), re-arm {2s,3s}, PHASE_AIR-gated —
-- random-target selection + air-phase machine; no bridges.
-- EVENT_DEVOURING_FLAME (:581-588) — SelectTarget(Random) +
-- DoCast(SPELL_DEVOURING_FLAME 63236), re-arm {6s,12s} /
-- {10s,12s} — random-target selection leg; no bridge.
-- EVENT_SUMMON_MINIONS / EVENT_SUMMON_MINIONS_2 (:589-603) —
-- SummonCreature(NPC_RAZORSCALE_SPAWNER) spawner leg — no bridge.
-- EVENT_FLAME_BREATH (:604-609) — Talk(EMOTE_BREATH 1, me) +
-- DoCastVictim(FLAME_BREATH 63317/64021) + EVENT_WING_BUFFET in
-- 2s, PHASE_GROUND-gated 30s one-shot — phase machine.
-- EVENT_FLAME_BREATH_GROUND (:610-614) — Talk(EMOTE_BREATH 1, me)
-- + DoCastVictim(FLAME_BREATH), re-arm {15s,18s},
-- PHASE_PERMA_GROUND-gated — phase machine.
-- EVENT_WING_BUFFET (:615-623) — DoCastSelf(SPELL_WING_BUFFET
-- 62666) + EVENT_FIREBOLT in 2s + EVENT_RESUME_AIR_PHASE in 4s —
-- phase-machine chain.
-- EVENT_RESUME_AIR_PHASE (:624-634) — SetDisableGravity /
-- SetPhase(PHASE_AIR) / MoveTakeoff / engineer
-- ACTION_FIX_HARPOONS DoAction — phase machine + motion +
-- DoAction; no bridges.
-- EVENT_FIREBOLT (:636-638) — DoCastSelf(SPELL_FIREBOLT 62669)
-- one-shot rides the unbridgeable WING_BUFFET phase leg.
-- EVENT_FUSE_ARMOR (:639-643) — GetVictim + !HasAura
-- (SPELL_FUSED_ARMOR 64774) gate + DoCast(SPELL_FUSE_ARMOR 64821),
-- re-arm {10s,15s}, PHASE_PERMA_GROUND-gated — aura gate + phase
-- machine.
-- EVENT_RESUME_MOVE_CHASE (:645-649) — MotionMaster leg.
-- DamageTaken (:547-555) — HealthBelowPctDamaged(50) +
-- PHASE_GROUND-gated perma-ground latch + DoAction
-- (ACTION_START_PERMA_GROUND) — no damage-taken / phase bridges.
-- SpellHit (:497-506) — stale "no SpellHit bridge" retracted
-- (2026-10-21): ev14 IS bridged (fires from the Go spell path) —
-- but the arm stays out: SPELL_HARPOON_TRIGGER 62505 drives a
-- counter whose sole effect is DoAction(ACTION_GROUND_PHASE) — no
-- DoAction bridge, and it feeds the unbridged air/ground phase
-- machine; a ported counter with no downstream would be an inert
-- no-op.
-- CanAIAttack / JustSummoned / SummonedCreatureDies /
-- EnterEvadeMode / HandleMusic / SummonMinions — no bridges.
-- npc_expedition_commander (33210 ulduar.h:90 — entry-verifiable
-- but bridge-blocked): OnGossipSelect -> DoAction
-- (ACTION_START_FIGHT) — gossip bridged (stale 'no gossip bridge' retracted — RegisterCreatureGossipEvent, engine/scripting/runtime.go:902), no DoAction bridge;
-- DoAction Talk(SAY_COMMANDER_AGGRO 0) / Talk
-- (SAY_COMMANDER_GROUND_PHASE 1) / Talk
-- (SAY_COMMANDER_ENGINEERS_DEAD 2) ride the DoAction machine —
-- Talk riding DoAction stays out; GO_RAZOR_HARPOON / broken-
-- harpoon summon legs — no GO bridge; no registration.
-- npc_expedition_defender (33816 ulduar.h:92): Reset one-shot
-- DoCastSelf(SPELL_THREAT 65146), not timer-driven; DoAction /
-- MovementInform machine — no bridges; no registration.
-- npc_expedition_trapper (33259 ulduar.h:93): DoAction machine +
-- MovementInform -> DoCastSelf(SPELL_SHACKLE 62646) — no bridges;
-- no registration.
-- npc_expedition_engineer (33287 ulduar.h:91): DoAction machine —
-- Talk(SAY_AGGRO 0) (engineer-east-gated) and Talk
-- (SAY_START_REPAIR 1) ride DoAction legs; spline movement
-- legs — no bridges; no registration.
-- npc_razorscale_spawner (33245 ulduar.h:89): Reset one-shot
-- DoCastSelf(SPELL_SUMMON_MOLE_MACHINE 62899) + DoCastSelf
-- (SummonMinionsSpells[urand(0,3)]) — spawner machine — no
-- bridge; no registration.
-- npc_darkrune_watcher (33453 ulduar.h:88 — entry-verifiable but
-- spawner-machine-blocked): EVENT_LIGHTNING_BOLT — DoCastVictim
-- (LIGHTNING_BOLT 63809 10-man) 5s init, 3s repeat; EVENT_CHAIN_
-- LIGHTNING — DoCastVictim(CHAIN_LIGHTNING 64758 10-man) 34s
-- init, re-arm {9s,15s}; both timer-driven and phase-free, but
-- the add exists only via the unbridged spawner chain (boss
-- EVENT_SUMMON_MINIONS -> NPC_RAZORSCALE_SPAWNER -> mole-machine
-- GO -> SPELL_TRIGGER_SUMMON_IRON_DWARVES 63968/63970/63969 ->
-- SPELL_SUMMON_IRON_DWARF_WATCHER 63135) — task exclusion:
-- summon/add legs out; no registration.
-- npc_darkrune_guardian (33388 ulduar.h:86 — entry-verifiable but
-- spawner-machine-blocked): EVENT_STORMSTRIKE — DoCastVictim
-- (SPELL_STORMSTRIKE 64757) 23s init, re-arm {13s,25s} —
-- same blocker; no registration.
-- npc_darkrune_sentinel (33846 ulduar.h:87 — entry-verifiable but
-- spawner-machine-blocked): EVENT_HEROIC_STRIKE — DoCastVictim
-- (SPELL_HEROIC_STRIKE 45026) 9s init, re-arm {5s,9s};
-- EVENT_BATTLE_SHOUT — DoCastSelf(SPELL_BATTLE_SHOUT 46763)
-- 15s init, 25s repeat; EVENT_WHIRLWIND — DoCastSelf
-- (SPELL_WHIRLWIND 63808) 17s init, re-arm {10s,13s} —
-- same blocker; no registration.
-- npc_razorscale_harpoon_fire_state (33282 ulduar.h:95):
-- SpellHit(SPELL_FIREBOLT 62669) machine — stale "no bridge"
-- retracted (2026-10-21): ev14 IS bridged (fires from the Go spell
-- path) — but the arm stays out: the DoCastSelf(HARPOON_FIRE_STATE
-- 62696) rides an instance-GetCreature(DATA_EXPEDITION_COMMANDER)
-- -> DoAction(ACTION_DESTROY_HARPOONS) cross-creature relay (no
-- instance-GetCreature / DoAction bridges), and 33282 itself has
-- no registration.
-- npc_razorscale_devouring_flame: Reset one-shot DoCastSelf
-- (DEVOURING_FLAME_GROUND 64709/64734, triggered), not
-- timer-driven; no registration.
-- go_razorscale_harpoon / go_razorscale_mole_machine —
-- GameObjectScripts — no GO bridge; no registrations.
-- spell_razorscale_flame_breath (63317/64021),
-- spell_razorscale_summon_iron_dwarves (63968/63970/63969),
-- spell_razorscale_fuse_armor (64821), spell_razorscale_firebolt
-- (62669) — no SpellScript / AuraScript bridges; all four join
-- the no-SpellScript / no-AuraScript-bridge queues;
-- achievement_iron_dwarf_medium_rare /
-- achievement_quick_shave (GetData-based OnCheck — no GetData /
-- achievement bridges; both join the unmodeled-achievement queue).

local ENTRY_RAZORSCALE = 33186

local EMOTE_BERSERK = 2

local SPELL_BERSERK = 47008

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

-- C++ EVENT_BERSERK (:572-575): DoCastSelf(SPELL_BERSERK 47008,
-- TRIGGERED) + Talk(EMOTE_BERSERK 2, me); 900s init (:362), no
-- re-arm, no phase gate — cast-before-Talk order kept.
local function berserkTick(creature)
    creature:CastSpell(creature, SPELL_BERSERK, true)
    creature:Talk(EMOTE_BERSERK, creature)
end

-- C++ JustEngagedWith (:356-365): BossAI passthrough +
-- SendEncounterUnit engage + ScheduleAirPhaseEvents (phase
-- machine) + summons DoAction + HandleMusic — only the
-- EVENT_BERSERK timer leg is bridgeable.
local function razorscaleEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "berserk", 900000, function()
        berserkTick(creature)
    end)
end

-- Shared timer wipe for events 2/4/23 (razorscale JustDied
-- :526-531 carries no Talk arms, so event 4 models only the
-- wipe).
local function cancelCombatTimers(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_RAZORSCALE, 1, razorscaleEnterCombat)
RegisterCreatureEvent(ENTRY_RAZORSCALE, 2, cancelCombatTimers)
RegisterCreatureEvent(ENTRY_RAZORSCALE, 4, cancelCombatTimers)
RegisterCreatureEvent(ENTRY_RAZORSCALE, 23, cancelCombatTimers)
