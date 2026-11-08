-- Skadi the Ruthless (Utgarde Pinnacle) — Lua port of
-- src/server/scripts/Northrend/UtgardeKeep/UtgardePinnacle/boss_skadi.cpp
-- (993 lines; 15 scripts — boss_skadi (CreatureScript via
-- GetUtgardePinnacleAI (BossAI), DATA_SKADI_THE_RUTHLESS = 2);
-- npc_grauf (CreatureScript via GetUtgardePinnacleAI (ScriptedAI),
-- DATA_GRAUF = 13); npc_ymirjar_warrior / npc_ymirjar_witch_doctor /
-- npc_ymirjar_harpooner (CreatureScript via GetUtgardePinnacleAI
-- (npc_skadi_trashAI : ScriptedAI)); spell_freezing_cloud_area_left /
-- spell_freezing_cloud_area_right / spell_freezing_cloud_damage /
-- spell_skadi_reset_check / spell_skadi_launch_harpoon /
-- spell_skadi_poisoned_spear (SpellScriptLoader); spell_skadi_ride_vehicle
-- (bare AuraScript via RegisterSpellScript);
-- spell_summon_gauntlet_mobs_periodic (SpellScriptLoader, AuraScript);
-- achievement_girl_love_to_skadi (AchievementCriteriaScript, OnCheck
-- GetData DATA_LOVE_TO_SKADI); at_skadi_gaunlet (AreaTriggerScript,
-- OnTrigger -> DoAction(ACTION_START_ENCOUNTER)); all registered from
-- inside AddSC_boss_skadi(); loader decl 134 / call 329 per
-- northrend_script_loader.cpp — the THIRD group of the "// Utgarde Keep -
-- Utgarde Pinnacle" block in AddNorthrendScripts(), immediately after
-- AddSC_boss_palehoof() (call 328); the checkpoint sequence
-- (boss_palehoof -> boss_skadi) is followed).
-- Entries: 26693 Skadi the Ruthless (utgarde_pinnacle.h
-- NPC_SKADI_THE_RUTHLESS, line 55); 26690 Ymirjar Warrior, 26691 Ymirjar
-- Witch Doctor, 26692 Ymirjar Harpooner (utgarde_pinnacle.h lines 71-73 —
-- registered this run for their ungated timer arms) — entry-verifiable,
-- registration proceeds (the nexus_commanders / malygos / sartharion
-- kalecgos precedent); the CreatureScript ScriptName binding is DB-side
-- as usual. DATA_SKADI_THE_RUTHLESS = 2 (utgarde_pinnacle.h line 33).
-- Sole-source verified: whole-server-tree grep for "boss_skadi" /
-- "npc_grauf" / "at_skadi_gaunlet" hits boss_skadi.cpp only (+ the loader
-- decl/call lines); this clone carries no sql/ tree, so ScriptName
-- bindings are DB-side by construction. The lua was ported Oct 2
-- (Talk-only, stale "no-timer-bridge" doctrine) and UPGRADED this run
-- under the CreateLuaEvent doctrine (engine/scripting/runtime.go:798 —
-- the maiden_of_virtue / keleseth / yogg_saron precedent): module-local
-- GUID-keyed timers table, wipe on events 2 (leave combat), 4 (death),
-- 23 (evade). creature:CastSpell(nil, spell) = DoCastVictim;
-- creature:CastSpell(creature, spell) = DoCastSelf / DoCastAOE (the
-- selin_fireheart / mr_smite / boss_ai.go DoCastAOE lowering precedent);
-- the 3rd CastSpell arg is the C++ DoCast 3rd arg (2-arg / 1-arg
-- overloads = non-triggered); {minMs, maxMs} table delay = randtime
-- range; random-target arms use GetPlayersInWorld (the baltharus /
-- thorim colossus precedent). The 2026-10-02 "no-timer-bridge" header
-- doctrine is RETRACTED for this file: CreateLuaEvent is a real
-- registered bridge, so the ungated timer arms below are ported rather
-- than parked.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnKill,
-- 4 OnDied, 23 OnReset.
-- Ported arms (C++-exact for all modeled arms; line refs are
-- boss_skadi.cpp):
-- boss_skadi KilledUnit — Talk(SAY_KILL 1) player-gated (event 3 — the
-- razuvious player-gated variant precedent).
-- boss_skadi JustDied — _JustDied() SetBossState(DATA_SKADI 2, DONE)
-- PORTED tick 261 via creature:SetBossState (lua_creature_events.go:5917),
-- first per C++ :217; Talk(SAY_DEATH 3) unconditional (event 4 — the
-- sjonnir JustDied-Talk precedent; the events.Reset / scheduler.CancelAll
-- legs are internal).
-- boss_skadi inherited BossAI::JustEngagedWith (boss_skadiAI has NO
-- override — kelthuzad precedent) — SetBossState(DATA_SKADI 2,
-- IN_PROGRESS) PORTED tick 261 via creature:SetBossState
-- (lua_creature_events.go:5917) + SetCombatPulseDelay(5) + me-form
-- DoZoneInCombat (ScriptedCreature.cpp:537/540/542) ported into
-- skadiEnterCombat (instance/setActive/ScheduleTasks legs out; the
-- DoAction encounter-start legs ride the collapsed ev1 model).
-- boss_skadi gauntlet reset-check — DoAction(ACTION_START_ENCOUNTER,
-- :241-246) 6s scheduler: FindNearestCreature(NPC_TRIGGER_RESET 23472,
-- 200.0f) -> trigger self-cast SPELL_UTGARDE_PINNACLE_GAUNTLET_RESET_
-- CHECK 49308, triggered, Repeat 6s. PORTED on event 1 (the C++ start
-- leg is the area trigger's DoAction call (:968, unported sender —
-- dispatch IS bridged, ev38), so the Lua pump starts at first combat
-- instead of encounter-start: gauntlet-window checks are missed, the
-- 6s cadence once running is C++-exact); cancelled on 2/4/23. The
-- SetBossState(IN_PROGRESS) :232 leg rides the same event-1 hook (see
-- the engage leg above). The 2s DoCast(grauf, SPELL_RIDE_GRAUF 61791)
-- leg (:247-251) stays out — the cast's effect is the vehicle board
-- (PassengerBoarded machine), no vehicle bridge; re-verified tick 261.
-- The ymirjar trash spell-timer kits run under the npc_skadi_trashAI
-- TaskScheduler validator (!CASTING, boss_skadi.cpp ctor :476-479),
-- so all six carry the 500ms IsCasting defer, ported with per-entry
-- event-1 scheduling carrying the C++ inits (their JustEngagedWith
-- calls CreatureAI::JustEngagedWith passthrough + ScheduleTasks):
--   npc_ymirjar_warrior (26690), ScheduleTasks :545-555:
--   DoCastVictim(SPELL_HAMSTRING 48639), 1-arg = non-triggered ->
--   CastSpell(nil, 48639), init 2s, Repeat({11s,18s}); DoCastVictim
--   (SPELL_STRIKE 48640), 1-arg = non-triggered -> CastSpell(nil,
--   48640), init 9s, Repeat({10s,13s}).
--   npc_ymirjar_witch_doctor (26691), ScheduleTasks :576-588:
--   DoCastVictim(SPELL_SHADOW_BOLT 49084), 1-arg = non-triggered ->
--   CastSpell(nil, 49084), init 2s, Repeat() = 2s; DoCastVictim
--   (SPELL_SHRINK 49089), 1-arg = non-triggered -> CastSpell(nil,
--   49089), init {20s,34s}, Repeat() = {20s,34s}.
--   npc_ymirjar_harpooner (26692), ScheduleTasks :607-619:
--   SelectTarget(SelectTargetMethod::MaxDistance, 0, 30, true) ->
--   DoCast(target, SPELL_NET 49092), 2-arg = non-triggered ->
--   CastSpell(target, 49092), init 13s, Repeat() = 13s (C++-exact via
--   farthestPlayerTarget — the stale "no distance-ordering bridge"
--   claim is retracted: GetDistance is the bridged object.go fallback); DoCastVictim
--   (SPELL_THROW 49091), 1-arg = non-triggered -> CastSpell(nil,
--   49091), init 2s, Repeat() = 2s.
-- DOCUMENTED DEVIATION: the TaskScheduler validator (!me->
-- HasUnitState(UNIT_STATE_CASTING), npc_skadi_trashAI ctor :476-479
-- + boss_skadiAI scheduler validator :160-163) is PORTED 2026-11-04
-- as the 500ms IsCasting defer on all six trash ticks, the boss
-- resetCheck tick and the three ground-phase ticks (TaskScheduler
-- Dispatch aborts while casting, tasks stay queued); the trash's
-- spawn/MovementInform machine (SummonCreature
-- legs, MovePoint POINT_0/1/2 legs, DoZoneInCombat on arrival, the
-- skadi->AI()->JustSummoned handoff) is unbridged — timers schedule on
-- combat start (event 1) rather than after the gauntlet walk.
-- DOCUMENTED-ONLY (in this header; no registration beyond the four
-- entries above):
-- boss_skadi's ground-phase timer kits — .Schedule(8s crush, 11s
-- poisonedSpear, 23s whirlwind) inside DoAction(ACTION_GAUNTLET_END,
-- :274-290): DoCastVictim(SPELL_CRUSH 50234), SelectTarget(Random)
-- DoCast(target, SPELL_POISONED_SPEAR 50255), DoCast(SPELL_WHIRLWIND
-- 50228) — fire ONLY inside the ACTION_GAUNTLET_END leg, which rides
-- the vehicle dismount (PassengerBoarded !apply on npc_grauf) — an
-- unbridged vehicle machine (the gothik precedent; DoAction dispatch
-- itself IS bridged, ev38). PORTED
-- 2026-10-24 as a proxy from graufJustDied: Grauf death ->
-- skadi->ExitVehicle() -> PassengerBoarded(!apply) -> GAUNTLET_END
-- (:350-356, :362), so the timer kit starts there (plus, since
-- 2026-10-25, the Talk/teleport/flag/react arms of the same leg);
-- Skadi rides Grauf, so FindNearestCreature resolves her.
-- The SetReactState(REACT_AGGRESSIVE) (:270) PORTED 2026-10-25 via
-- the graufJustDied proxy; boss_skadi Reset's SetReactState(REACT_
-- PASSIVE) (:170) + Grauf summon PORTED 2026-10-25 on event 23
-- (skadiReset; the stale "event 23 not registered for Skadi" claim
-- is corrected — event 23 was registered, as cancelCombatTimers).
-- Boss-state leg (tick 261, corrected in-house): ev23 is state-aware
-- — IN_PROGRESS (the evade-path fire, EnterEvadeMode :177-179 ->
-- _DespawnAtEvade, ScriptedCreature.cpp:617) sets FAIL; any other
-- stored state (!= DONE, the spawn/respawn-path fire where C++ Reset
-- runs _Reset) sets NOT_STARTED. The death pre-fire is IsDead-guarded
-- in skadiReset.
-- Talk(SAY_AGGRO 0) rides DoAction(ACTION_START_ENCOUNTER) (area-trigger
-- DoAction machine) — event 1 deliberately NOT registered (registering
-- it would deviate from C++: ground-phase combat entry yells nothing
-- there); Talk(SAY_DRAKE_BREATH 6) (ACTION_DRAKE_BREATH) rides the
-- grauf spline-chain machine (:416/:435, unported vehicle/motion —
-- dispatch IS bridged, ev38). Stay out
-- (the harpoon-hit counter itself is PORTED 2026-11-04 via the ev14
-- graufSpellHit handler; only the Talk trigger stays out);
-- Talk(SAY_DRAKE_DEATH
-- 5) (ACTION_GAUNTLET_END) PORTED 2026-10-25 via the graufJustDied
-- proxy.
-- npc_grauf (26893, utgarde_pinnacle.h line 70) — all timer legs are
-- spline-chain gated (PassengerBoarded / MovementInform SPLINE_CHAIN_
-- MOTION_TYPE: EMOTE_ON_RANGE / EMOTE_BREATH Talks, FREEZING_CLOUD
-- periodic casts, ACTION_DRAKE_BREATH handoff) — no MovementInform /
-- spline / vehicle bridges. Stay out; the JustDied
-- DespawnOrUnsummon(Seconds(6)) leg ports (event 4, below — the freya
-- delayed-despawn idiom), the paired skadi->ExitVehicle() leg having
-- no cross-creature bridge, and the Reset trio (REACT_PASSIVE +
-- no-regen + run speed 2.5) ports on event 23 below.
-- npc_ymirjar_harpooner JustDied — DoCast(SPELL_SUMMON_HARPOON 56789)
-- is PORTED below (harpoonerDied, ev4): non-triggered self-cast, the
-- viscidus-suicide-cast idiom (the old "no bridge" claim was stale).
-- spell_freezing_cloud_area_left / right, spell_freezing_cloud_damage,
-- spell_skadi_reset_check, spell_skadi_launch_harpoon,
-- spell_skadi_poisoned_spear — SpellScript / AuraScript hooks only —
-- join the no-SpellScript-bridge queue (freezing_cloud_damage's
-- DoCheckAreaTarget has no aura-event twin: the RegisterAuraEvent set
-- is Periodic 1 / Apply 2 / Remove 3 / ChangeAmount 4 / Dispel 5 /
-- Proc 6 / BeforeDispel 7, engine/scripting/runtime.go:497-503).
-- spell_skadi_ride_vehicle + spell_summon_gauntlet_mobs_periodic are
-- PORTED below via RegisterAuraEvent (this run) — the old
-- "no-AuraScript-bridge" claim for them is retired.
-- achievement_girl_love_to_skadi (OnCheck: GetData(DATA_LOVE_TO_SKADI)
-- == 1) — PORTED tick 417: the love latch is mirrored into Skadi's
-- SetData store (Reset 0 / first harpoon hit 1, graufSpellHit below)
-- and read by the Go gate via bossLatchData. Delta: the drake-breath
-- 1->2 leg stays out (spline-chain MovementInform, no bridge) — see
-- below. The love counter itself is kept in harpoonState (see below).
-- at_skadi_gaunlet — AreaTriggerScript, no bridge; documented only.

local ENTRY_SKADI_THE_RUTHLESS = 26693
local ENTRY_YMIRJAR_WARRIOR = 26690
local ENTRY_YMIRJAR_WITCH_DOCTOR = 26691
local ENTRY_YMIRJAR_HARPOONER = 26692
local ENTRY_GRAUF = 26893 -- utgarde_pinnacle.h:70
local DATA_SKADI_THE_RUTHLESS = 2 -- utgarde_pinnacle.h:33
local DATA_LOVE_TO_SKADI = 0 -- boss_skadi.cpp:112 (MiscData)
local NPC_TRIGGER_RESET = 23472 -- utgarde_pinnacle.h:74
local SPELL_UTGARDE_PINNACLE_GAUNTLET_RESET_CHECK = 49308 -- boss_skadi.cpp:54

local SAY_KILL = 1
local SAY_DEATH = 3
local SAY_DRAKE_DEATH = 5

local SPELL_HAMSTRING = 48639
local SPELL_STRIKE = 48640
local SPELL_SHADOW_BOLT = 49084
local SPELL_CRUSH = 50234
local SPELL_POISONED_SPEAR = 50255
local SPELL_WHIRLWIND = 50228
local SPELL_SKADI_TELEPORT = 61790
local SPELL_SHRINK = 49089
local SPELL_NET = 49092
local SPELL_THROW = 49091
local SPELL_LAUNCH_HARPOON = 48642 -- boss_skadi.cpp:39
local SPELL_SUMMON_HARPOON = 56789 -- boss_skadi.cpp:62
local SPELL_RIDE_GRAUF = 61791 -- boss_skadi.cpp:41
local SPELL_SUMMON_GAUNLET_MOBS_PERIODIC = 59275 -- boss_skadi.cpp:40
local SPELL_SUMMON_YMIRJAR_WARRIOR_W = 48631 -- boss_skadi.cpp:47
local SPELL_SUMMON_YMIRJAR_WARRIOR_E = 48632 -- boss_skadi.cpp:48
local SPELL_SUMMON_YMIRJAR_HARPOONER_W = 48633 -- boss_skadi.cpp:49
local SPELL_SUMMON_YMIRJAR_HARPOONER_E = 48634 -- boss_skadi.cpp:50
local SPELL_SUMMON_YMIRJAR_WITCH_DOCTOR_W = 48635 -- boss_skadi.cpp:51
local SPELL_SUMMON_YMIRJAR_WITCH_DOCTOR_E = 48636 -- boss_skadi.cpp:52

local timers = {}
local harpoonState = {} -- per-Skadi-GUID { hits, love } for ACTION_HARPOON_HIT

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

-- Shared timer wipe for events 2/23/4 on timer-carrying entries.
local function cancelCombatTimers(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ boss_skadi::JustSummoned (:183-197): ymirjar entries MovePoint
-- to the secondary wave point once the first wave exists, then the
-- unconditional summons.Summon latch (covers Grauf too). Lua summons
-- only Grauf (Reset, below); the ymirjar waves ride the unported
-- SpawnFirstWave/DoAction machine, so their MovePoint legs are
-- vacuous here. The latch feeds Reset _Reset DespawnAll (:166),
-- JustDied _JustDied DespawnAll (:217) and EnterEvadeMode DespawnAll
-- (:177-179). Table defined above its first caller (halazzi idiom).
local skadiSummons = {}

local function despawnSkadiSummons(guid)
    local latched = skadiSummons[guid]
    if latched then
        for _, summon in ipairs(latched) do
            if summon then
                summon:Despawn()
            end
        end
        skadiSummons[guid] = nil
    end
end

-- C++ JustSummoned, above: latch every direct summon.
local function skadiJustSummoned(event, creature, summon)
    if not summon then
        return
    end
    local guid = creature:GetGUID()
    local latched = skadiSummons[guid]
    if not latched then
        latched = {}
        skadiSummons[guid] = latched
    end
    latched[#latched + 1] = summon
end

-- C++ EnterEvadeMode (:177-179): summons.DespawnAll() before the
-- evade despawn; timers wipe rides along (cancelCombatTimers idiom).
local function skadiLeaveCombat(event, creature)
    despawnSkadiSummons(creature:GetGUID())
    cancelTimers(creature:GetGUID())
end

-- C++ boss_skadi::Reset (:166-172): REACT_PASSIVE + SummonCreature
-- (NPC_GRAUF, GraufLoc) when the instance lacks Grauf. The ev23
-- death-path pre-fire (23 then 4) is guarded: C++ Reset never runs
-- on death, so a dead Skadi only cancels timers here.
-- Boss-state leg (tick 261, corrected in-house): C++ Skadi OVERRIDES
-- EnterEvadeMode (:177-179) with no Reset() call — evade runs
-- summons.DespawnAll() + _DespawnAtEvade(), which sets FAIL
-- (ScriptedCreature.cpp:617). But ev23 also fires on spawn/respawn
-- (fireCreatureSpawned pre-fire, lua_creature_events.go:6762), where
-- C++ Reset runs _Reset() -> NOT_STARTED (:509-510). The two paths
-- are told apart by the stored state: IN_PROGRESS means this fire is
-- the evade path -> FAIL; anything else (!= DONE) is the spawn path
-- -> NOT_STARTED. The death pre-fire is guarded by the IsDead return
-- above. PORTED tick 261 via creature:SetBossState
-- (lua_creature_events.go:5917).
local function skadiReset(event, creature)
    cancelTimers(creature:GetGUID())
    if creature:IsDead() then
        return
    end
    if creature:GetBossState(DATA_SKADI_THE_RUTHLESS) == 1 then
        creature:SetBossState(DATA_SKADI_THE_RUTHLESS, 2) -- FAIL: evade path
    elseif creature:GetBossState(DATA_SKADI_THE_RUTHLESS) ~= 3 then
        creature:SetBossState(DATA_SKADI_THE_RUTHLESS, 0) -- NOT_STARTED: spawn path
    end
    -- C++ Initialize (:157-158) via Reset (:166-172): counters re-zeroed.
    harpoonState[creature:GetGUID()] = nil
    creature:SetData(DATA_LOVE_TO_SKADI, 0)
    -- C++ Reset (:166-172): _Reset() (DespawnAll) before re-summoning.
    despawnSkadiSummons(creature:GetGUID())
    creature:SetReactState(0) -- REACT_PASSIVE
    if not creature:FindNearestCreature(ENTRY_GRAUF, 500) then
        creature:SummonCreature(ENTRY_GRAUF, 341.7411, -516.9545, 104.6695, 3.124139)
    end
end

-- Farthest-target helper — the SelectTargetMethod::MaxDistance counterpart
-- of the baltharus / ENERVATING_BRAND / thorim colossus random-target
-- precedent (alive, same map and instance, within maxDist yards of the
-- creature; picks the max-distance candidate).
-- C++ SelectTarget(MaxDistance, 0, 30, true): farthest alive
-- threat-list player within 30yd.
local function farthestPlayerTarget(creature, maxDist)
    local best, bestDist = nil, nil
    local reach = creature:GetCombatReach()
    for _, plr in ipairs(creature:GetAITargets()) do
        if plr:GetObjectType() == "Player" and not plr:IsDead() then
            local dist = creature:GetDistance(plr)
            if dist - reach - 1.5 < maxDist and (bestDist == nil or dist > bestDist) then -- IsWithinCombatRange gate, reach-exact (Unit.cpp:583; player reach 1.5)
                best, bestDist = plr, dist
            end
        end
    end
    return best
end

-- C++ SelectTarget(Random) with defaults (offset 0, dist 0,
-- playerOnly false): any alive threat-list unit.
local function randomPlayerTarget(creature)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if not t:IsDead() then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ KilledUnit: if (who->GetTypeId() == TYPEID_PLAYER) Talk(SAY_KILL)
-- — the razuvious player-gated variant precedent.
local function skadiKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

-- C++ JustDied (:217-220): _JustDied() (DespawnAll + SetBossState DONE,
-- ScriptedCreature.cpp:519) before Talk (:218) — the DONE leg PORTED
-- tick 261 via creature:SetBossState (lua_creature_events.go:5917).
local function skadiJustDied(event, creature)
    creature:SetBossState(DATA_SKADI_THE_RUTHLESS, 3) -- DONE
    despawnSkadiSummons(creature:GetGUID())
    creature:Talk(SAY_DEATH)
    cancelTimers(creature:GetGUID())
end

-- C++ DoAction(ACTION_START_ENCOUNTER) reset-check scheduler
-- (boss_skadi.cpp:241-246): 6s first tick, Repeat 6s; the trigger
-- self-cast is the mandokir/fathomlord found-creature CastSpell
-- precedent. Starts at first combat (area-trigger sender unported —
-- see header).
-- Inherited BossAI::JustEngagedWith arms (no override — kelthuzad
-- precedent) ported at the handler tail (see header).
local function skadiEnterCombat(event, creature)
    -- C++ DoAction(ACTION_START_ENCOUNTER) :232 + BossAI::_JustEngagedWith
    -- (ScriptedCreature.cpp:537): SetBossState(DATA_SKADI_THE_RUTHLESS,
    -- IN_PROGRESS) — PORTED tick 261 via creature:SetBossState
    -- (lua_creature_events.go:5917), first per the _JustEngagedWith order.
    -- The area-trigger DoAction sender is unported, so the event-1 hook is
    -- the only available one (same proxy as the reset-check pump below).
    creature:SetBossState(DATA_SKADI_THE_RUTHLESS, 1)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    local function resetCheckTick()
    if creature:IsCasting() then
        schedule(guid, "resetCheck", 500, resetCheckTick)
        return
    end
        local trigger = creature:FindNearestCreature(NPC_TRIGGER_RESET, 200.0)
        if trigger then
            trigger:CastSpell(trigger, SPELL_UTGARDE_PINNACLE_GAUNTLET_RESET_CHECK, true)
        end
        schedule(guid, "resetCheck", 6000, resetCheckTick)
    end
    schedule(guid, "resetCheck", 6000, resetCheckTick)
    -- Inherited BossAI::JustEngagedWith (no override in boss_skadiAI —
    -- kelthuzad precedent): SetCombatPulseDelay(5) + me-form
    -- DoZoneInCombat (ScriptedCreature.cpp:540/542); instance/setActive/
    -- ScheduleTasks legs out. C++ order puts the base engage after the
    -- DoAction encounter-start legs (the summoned combat trigger's
    -- DoZone drives the engage), so these sit after the reset-check
    -- seed above.
    creature:SetCombatPulseDelay(5)
    creature:DoZoneInCombat()
end

-- C++ npc_ymirjar_warriorAI::ScheduleTasks (:545-555): DoCastVictim
-- (SPELL_HAMSTRING 48639) non-triggered, init 2s, Repeat({11s,18s});
-- DoCastVictim(SPELL_STRIKE 48640) non-triggered, init 9s,
-- Repeat({10s,13s}).
local function warriorEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    local function hamstringTick()
    if creature:IsCasting() then
        schedule(guid, "hamstring", 500, hamstringTick)
        return
    end
        creature:CastSpell(nil, SPELL_HAMSTRING)
        schedule(guid, "hamstring", {11000, 18000}, hamstringTick)
    end
    local function strikeTick()
    if creature:IsCasting() then
        schedule(guid, "strike", 500, strikeTick)
        return
    end
        creature:CastSpell(nil, SPELL_STRIKE)
        schedule(guid, "strike", {10000, 13000}, strikeTick)
    end
    schedule(guid, "hamstring", 2000, hamstringTick)
    schedule(guid, "strike", 9000, strikeTick)
end

-- C++ npc_ymirjar_witch_doctorAI::ScheduleTasks (:576-588):
-- DoCastVictim(SPELL_SHADOW_BOLT 49084) non-triggered, init 2s,
-- Repeat() = 2s; DoCastVictim(SPELL_SHRINK 49089) non-triggered, init
-- {20s,34s} — the randtime pick is made ONCE at Schedule; arg-less
-- Repeat() reuses that fixed duration (TaskScheduler.h:483-486).
local function witchDoctorEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    local function shadowBoltTick()
    if creature:IsCasting() then
        schedule(guid, "shadowbolt", 500, shadowBoltTick)
        return
    end
        creature:CastSpell(nil, SPELL_SHADOW_BOLT)
        schedule(guid, "shadowbolt", 2000, shadowBoltTick)
    end
    local shrinkDelay = math.random(20000, 34000)
    local function shrinkTick()
    if creature:IsCasting() then
        schedule(guid, "shrink", 500, shrinkTick)
        return
    end
        creature:CastSpell(nil, SPELL_SHRINK)
        schedule(guid, "shrink", shrinkDelay, shrinkTick)
    end
    schedule(guid, "shadowbolt", 2000, shadowBoltTick)
    schedule(guid, "shrink", shrinkDelay, shrinkTick)
end

-- C++ npc_ymirjar_harpoonerAI::ScheduleTasks (:607-619):
-- SelectTarget(MaxDistance, 0, 30, true) -> DoCast(target, SPELL_NET
-- 49092) non-triggered, init 13s, Repeat() = 13s; DoCastVictim
-- (SPELL_THROW 49091) non-triggered, init 2s, Repeat() = 2s.
local function harpoonerEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    local function netTick()
    if creature:IsCasting() then
        schedule(guid, "net", 500, netTick)
        return
    end
        local target = farthestPlayerTarget(creature, 30)
        if target then
            creature:CastSpell(target, SPELL_NET)
        end
        schedule(guid, "net", 13000, netTick)
    end
    local function throwTick()
    if creature:IsCasting() then
        schedule(guid, "throw", 500, throwTick)
        return
    end
        creature:CastSpell(nil, SPELL_THROW)
        schedule(guid, "throw", 2000, throwTick)
    end
    schedule(guid, "net", 13000, netTick)
    schedule(guid, "throw", 2000, throwTick)
end

-- C++ npc_ymirjar_harpoonerAI::JustDied (boss_skadi.cpp:622-625):
-- 1-arg DoCast(SPELL_SUMMON_HARPOON 56789) on self, non-triggered
-- (the viscidus-suicide-cast idiom — no `true` flag; the tick-384
-- audit's triggered prescription is corrected here). Timer wipe rides
-- along (cancelCombatTimers idiom).
local function harpoonerDied(event, creature, killer)
    cancelCombatTimers(event, creature)
    creature:CastSpell(creature, SPELL_SUMMON_HARPOON)
end

RegisterCreatureEvent(ENTRY_SKADI_THE_RUTHLESS, 1, skadiEnterCombat)
RegisterCreatureEvent(ENTRY_SKADI_THE_RUTHLESS, 2, skadiLeaveCombat)
RegisterCreatureEvent(ENTRY_SKADI_THE_RUTHLESS, 3, skadiKilledUnit)
RegisterCreatureEvent(ENTRY_SKADI_THE_RUTHLESS, 4, skadiJustDied)
RegisterCreatureEvent(ENTRY_SKADI_THE_RUTHLESS, 23, skadiReset)
RegisterCreatureEvent(ENTRY_SKADI_THE_RUTHLESS, 19, skadiJustSummoned)
RegisterCreatureEvent(ENTRY_YMIRJAR_WARRIOR, 1, warriorEnterCombat)
RegisterCreatureEvent(ENTRY_YMIRJAR_WARRIOR, 2, cancelCombatTimers)
RegisterCreatureEvent(ENTRY_YMIRJAR_WARRIOR, 4, cancelCombatTimers)
RegisterCreatureEvent(ENTRY_YMIRJAR_WARRIOR, 23, cancelCombatTimers)
RegisterCreatureEvent(ENTRY_YMIRJAR_WITCH_DOCTOR, 1, witchDoctorEnterCombat)
RegisterCreatureEvent(ENTRY_YMIRJAR_WITCH_DOCTOR, 2, cancelCombatTimers)
RegisterCreatureEvent(ENTRY_YMIRJAR_WITCH_DOCTOR, 4, cancelCombatTimers)
RegisterCreatureEvent(ENTRY_YMIRJAR_WITCH_DOCTOR, 23, cancelCombatTimers)
RegisterCreatureEvent(ENTRY_YMIRJAR_HARPOONER, 1, harpoonerEnterCombat)
RegisterCreatureEvent(ENTRY_YMIRJAR_HARPOONER, 2, cancelCombatTimers)
RegisterCreatureEvent(ENTRY_YMIRJAR_HARPOONER, 4, harpoonerDied)
RegisterCreatureEvent(ENTRY_YMIRJAR_HARPOONER, 23, cancelCombatTimers)

-- C++ npc_graufAI::Reset (boss_skadi.cpp:343-348): REACT_PASSIVE +
-- no-regen + run speed 2.5, in C++ order; the spline-chain timer
-- legs stay out (see header), but Reset itself is not machine-gated.
local function graufReset(event, creature)
    creature:SetReactState(0) -- REACT_PASSIVE
    creature:SetRegenerateHealth(false)
    creature:SetSpeedRate(1, 2.5)
end

RegisterCreatureEvent(ENTRY_GRAUF, 23, graufReset)

-- C++ npc_graufAI::JustDied (boss_skadi.cpp:350-356): the
-- skadi->ExitVehicle() leg is cross-creature (no bridge, stays out);
-- me->DespawnOrUnsummon(Seconds(6)) ports via the freya
-- delayed-despawn idiom (CreateLuaEvent one-shot).
local function graufJustDied(event, creature, killer)
    CreateLuaEvent(function()
        creature:Despawn()
    end, 6000)
    -- C++ GAUNTLET_END timer kit (:274-290) via the ExitVehicle chain
    -- (see header): crush 8s victim, poisoned spear 11s random,
    -- whirlwind 23s self — all Repeat(). Skadi's timers cancel with
    -- her events 2/4/23.
    local skadi = creature:FindNearestCreature(ENTRY_SKADI_THE_RUTHLESS, 200.0)
    if skadi then
        -- C++ ACTION_GAUNTLET_END (:264-272, in order; the
        -- summons.DespawnEntry(NPC_WORLD_TRIGGER) leg is a no-op here —
        -- Lua never summons the trigger): Talk + self-teleport +
        -- NOT_SELECTABLE clear + SetImmuneToPC(false) + aggressive.
        skadi:Talk(SAY_DRAKE_DEATH)
        skadi:CastSpell(skadi, SPELL_SKADI_TELEPORT)
        skadi:RemoveFlag(59, 0x02000000) -- UNIT_FIELD_FLAGS NOT_SELECTABLE
        skadi:RemoveFlag(59, 0x100) -- SetImmuneToPC(false)
        skadi:SetReactState(2) -- REACT_AGGRESSIVE
        local skadiGuid = skadi:GetGUID()
        local function crushTick()
            if skadi:IsCasting() then
                schedule(skadiGuid, "crush", 500, crushTick)
                return
            end
            skadi:CastSpell(nil, SPELL_CRUSH)
            schedule(skadiGuid, "crush", 8000, crushTick)
        end
        local function spearTick()
            if skadi:IsCasting() then
                schedule(skadiGuid, "poisonedSpear", 500, spearTick)
                return
            end
            local target = randomPlayerTarget(skadi)
            if target then
                skadi:CastSpell(target, SPELL_POISONED_SPEAR)
            end
            schedule(skadiGuid, "poisonedSpear", 11000, spearTick)
        end
        local function whirlwindTick()
            if skadi:IsCasting() then
                schedule(skadiGuid, "whirlwind", 500, whirlwindTick)
                return
            end
            skadi:CastSpell(skadi, SPELL_WHIRLWIND)
            schedule(skadiGuid, "whirlwind", 23000, whirlwindTick)
        end
        schedule(skadiGuid, "crush", 8000, crushTick)
        schedule(skadiGuid, "poisonedSpear", 11000, spearTick)
        schedule(skadiGuid, "whirlwind", 23000, whirlwindTick)
    end
end

RegisterCreatureEvent(ENTRY_GRAUF, 4, graufJustDied)

-- C++ npc_graufAI::SpellHit (boss_skadi.cpp:447-452): SPELL_LAUNCH_HARPOON
-- hit on Grauf -> skadi DoAction(ACTION_HARPOON_HIT) (:291-295), counter
-- only: _harpoonHit++, _loveSkadi = 1 on the first hit. Event 14 fires on
-- the hit target (boss_ai.go:3813, args event/creature/caster/spellid);
-- Skadi resolves via the graufJustDied FindNearestCreature precedent.
-- Consumers (DRAKE_BREATH love 1->2, GetData achievement) stay out.
local function graufSpellHit(event, creature, caster, spellId)
    if spellId ~= SPELL_LAUNCH_HARPOON then
        return
    end
    local skadi = creature:FindNearestCreature(ENTRY_SKADI_THE_RUTHLESS, 200.0)
    if not skadi then
        return
    end
    local guid = skadi:GetGUID()
    local st = harpoonState[guid]
    if not st then
        st = { hits = 0, love = 0 }
        harpoonState[guid] = st
    end
    st.hits = st.hits + 1
    if st.hits == 1 then
        st.love = 1
    end
    -- Achievement write leg (tick 417): mirror the love latch into
    -- Skadi's SetData store for achievement_girl_love_to_skadi's
    -- GetData read (Go gate via bossLatchData). Delta: the
    -- ACTION_DRAKE_BREATH 1->2 leg (boss_skadi.cpp:259-262, grauf
    -- spline-chain MovementInform) is unported, so love never returns
    -- to 2 in Lua — the gate awards on any kill after the first
    -- harpoon hit instead of only inside the first-hit-to-next-breath
    -- window.
    skadi:SetData(DATA_LOVE_TO_SKADI, st.love)
end

RegisterCreatureEvent(ENTRY_GRAUF, 14, graufSpellHit)
RegisterCreatureEvent(ENTRY_GRAUF, 1, function(_, creature)
    creature:SetMeleeAttack(false) -- boss_skadi.cpp:454: custom UpdateAI never calls DoMeleeAttackIfReady (scheduler-only UpdateAI in C++, never melees)
end)

-- C++ spell_summon_gauntlet_mobs_periodic_AuraScript
-- (boss_skadi.cpp:871-937): OnPeriodic, EFFECT_0 PERIODIC_DUMMY.
-- Each tick casts the next two summon spells off the script's
-- rotating 8-spell deque and rotates it by two more, so the front
-- advances by four per tick; the RAND branch only swaps which pair
-- fires first (cast-then-rotate casts positions 1-2, rotate-then-cast
-- casts positions 3-4). The deque is per-AuraScript-instance in C++;
-- the Lua twin is per-target-GUID state (the timers-table precedent),
-- reset by the OnApply handler below — Go ev2 fires only on fresh
-- applications (engine/world/spells.go:39194), so the reset matches
-- the fresh-script-instance construction. C++ casts with a nullptr
-- target (the summon spells resolve at the trigger); Go nil would
-- fall back to the trigger's (nonexistent) victim and no-op
-- (lua_creature_events.go:896), so the self-cast is the live twin.
-- DORMANT until the ACTION_START_ENCOUNTER summon-trigger cast leg
-- (:254, unported sender) lands and Spell.dbc ships the 59275 aura
-- record.
local gauntletSpells = {
    SPELL_SUMMON_YMIRJAR_WARRIOR_E,
    SPELL_SUMMON_YMIRJAR_HARPOONER_W,
    SPELL_SUMMON_YMIRJAR_WARRIOR_W,
    SPELL_SUMMON_YMIRJAR_HARPOONER_E,
    SPELL_SUMMON_YMIRJAR_WARRIOR_W,
    SPELL_SUMMON_YMIRJAR_WITCH_DOCTOR_E,
    SPELL_SUMMON_YMIRJAR_WARRIOR_E,
    SPELL_SUMMON_YMIRJAR_WITCH_DOCTOR_W,
}
local gauntletIndexByGuid = {}

local function gauntletMobsApply(event, aura, target)
    if target == nil then
        return
    end
    gauntletIndexByGuid[target:GetGUID()] = 1
end

local function gauntletMobsPeriodic(event, aura, target)
    if target == nil then
        return
    end
    local guid = target:GetGUID()
    local index = gauntletIndexByGuid[guid] or 1
    local offset = 0
    if math.random(0, 1) == 0 then
        offset = 2
    end
    local first = gauntletSpells[(index + offset - 1) % 8 + 1]
    local second = gauntletSpells[(index + offset) % 8 + 1]
    target:CastSpell(target, first, true)
    target:CastSpell(target, second, true)
    gauntletIndexByGuid[guid] = (index + 3) % 8 + 1
end

RegisterAuraEvent(SPELL_SUMMON_GAUNLET_MOBS_PERIODIC, 2, gauntletMobsApply)
RegisterAuraEvent(SPELL_SUMMON_GAUNLET_MOBS_PERIODIC, 1, gauntletMobsPeriodic)

-- C++ spell_skadi_ride_vehicle::OnRemoveVehicle (boss_skadi.cpp:848-867):
-- OnEffectRemove, EFFECT_0 SPELL_AURA_CONTROL_VEHICLE,
-- AURA_EFFECT_HANDLE_REAL. PreventDefaultAction, then the target's
-- vehicle kit drops the caster as a passenger and the caster is
-- un-rooted. The Go control-vehicle remove leg fires the ev3 hook
-- before its default exit leg (engine/world/spells.go:44661), so
-- returning true is the PreventDefaultAction twin (the default leg
-- is a no-op for the creature caster in any case). The kit is
-- nil-guarded — Go never installs one for the creature-cast ride,
-- so the leg runs when a kit exists and the un-root still lands.
-- DORMANT until the 2s ride cast (:247-251, stays out on the vehicle
-- board) lands and Spell.dbc ships the 61791 aura record.
local function rideVehicleRemove(event, aura, target)
    local caster = aura:GetCaster()
    if caster == nil then
        return true
    end
    if target ~= nil then
        local kit = target:GetVehicleKit()
        if kit ~= nil then
            kit:RemovePassenger(caster)
        end
    end
    caster:SetRooted(false)
    return true
end

RegisterAuraEvent(SPELL_RIDE_GRAUF, 3, rideVehicleRemove)
