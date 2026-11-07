-- Tenebron / Shadron / Vesperon (Obsidian Sanctum drake mini-bosses) —
-- Lua port of
-- src/server/scripts/Northrend/ChamberOfAspects/ObsidianSanctum/obsidian_sanctum.cpp
-- (npc_tenebron / npc_shadron / npc_vesperon (CreatureScripts), each via
-- GetObsidianSanctumAI<...AI> (dummy_dragonAI : ScriptedAI);
-- registered from inside AddSC_obsidian_sanctum(); loader decl 95 /
-- call 290 per northrend_script_loader.cpp — the SECOND group of the
-- "// Obsidian Sanctum" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_sartharion() (decl 94 / call 289), under the
-- "// Obsidian Sanctum" marker; the call after it is
-- AddSC_instance_obsidian_sanctum() (decl 96 / call 291) — loader
-- order confirmed this run; the checkpoint sequence
-- (boss_sartharion -> obsidian_sanctum) is followed).
-- Entries: 30452 Tenebron (obsidian_sanctum.h NPC_TENEBRON, line 41),
-- 30451 Shadron (NPC_SHADRON, line 42), 30449 Vesperon
-- (NPC_VESPERON, line 43) — entry-verifiable (the nexus_commanders /
-- malygos / sartharion kalecgos precedent); the CreatureScript
-- ScriptName bindings are DB-side as usual.
-- Sole-source verified: whole-server-tree grep for each of the
-- twelve script names registered from AddSC_obsidian_sanctum()
-- hits obsidian_sanctum.cpp only; zero sql/ hits. No obsidian_sanctum
-- lua existed.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers ride the CreateLuaEvent
-- convention (baltharus / maiden precedent): GUID-keyed timers
-- table, cancel on events 2/4/23; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms; dummy_dragonAI base,
-- shared across the three drakes):
-- JustEngagedWith — Talk(SAY_AGGRO 0) + DoZoneInCombat (PORTED
-- 2026-10-23 — creature:DoZoneInCombat bridge live,
-- engine/world/lua_creature_events.go, C++ arm order) + the
-- shadow-breath and shadow-fissure timers (event 1; the
-- EVENT_SHADOW_FISSURE leg rides the fissureTick port, so the old
-- "no target-selection" leg claim is retracted).
-- EVENT_SHADOW_BREATH — Talk(SAY_BREATH 3) +
-- DoCastVictim(SPELL_SHADOW_BREATH 57570) 2-arg non-triggered,
-- 20s init / {20s,25s} re-arm (:193 / :392-:396).
-- EVENT_SHADOW_FISSURE — SelectTarget(Random, 0, 0.0f, true)
-- (playerOnly=true, threat-list pool) -> randomThreatPlayer(creature),
-- nil-guarded pick, DoCast 2-arg non-triggered
-- (SPELL_SHADOW_FISSURE 57579), {15s,20s} unconditional re-arm,
-- 5s init (:192 / :387-:391; the old "no target-selection / cast
-- bridges" claim was stale; the old GetPlayersInWorld pool was
-- retired 2026-10-25 for the threat-list pool).
-- KilledUnit — C++-gated on who->GetTypeId() == TYPEID_PLAYER,
-- then Talk(SAY_SLAY 1) — PORTED (event 3; the razuvious
-- player-gated variant precedent: victim nil-guard,
-- victim:GetObjectType() == "Player").
-- JustDied — PORTED 2026-11-05 (tick 255, dragonDied, C++ arm order):
-- _canLoot / SetLootRecipient (ev5 seed resolves the default-0 trap) +
-- entry-switched SetBossState (DATA_TENEBRON / DATA_SHADRON /
-- DATA_VESPERON, DONE, sartharion != IN_PROGRESS gate) + acolyte
-- KillSelf legs (FindNearestCreature NPC_ACOLYTE_OF_SHADRON 31218 /
-- NPC_ACOLYTE_OF_VESPERON 31219, 100.0f) + Talk(SAY_DEATH 2) + the
-- entry-switched SELF RemoveAurasDueToSpell leg (SPELL_POWER_OF_TENEBRON
-- 61248 / SPELL_POWER_OF_SHADRON 58105 / SPELL_POWER_OF_VESPERON 61251)
-- + DoRemoveAurasDueToSpellOnPlayers via GetPlayersInGrid(500)
-- (whole-instance twin; dead players skipped by the bridge) + Twilight
-- Revenge on Sartharion (SPELL_TWILIGHT_REVENGE 60639 triggered
-- DoCast via GetInstanceCreature, IN_PROGRESS gate).
-- DOCUMENTED-ONLY (in this header; no registration beyond entries
-- 30452 / 30451 / 30449 — no bridgeable arms):
-- dummy_dragonAI Reset — flag clear PORTED in dragonReset; Initialize
-- _canLoot=true seed PORTED in dragonSpawn (ev5); waypointId /
-- _canMoveFree PORTED 2026-11-07 (tick 394, dragonFlight latch, see
-- MovementInform below); portalRespawnTime stays out (SetRespawnTime
-- has no static-GO consumer, see OpenPortal below);
-- MovementInform — PORTED 2026-11-07 (tick 394, dragonMovementInform,
-- ev28 live tick 392): POINT_ID_INIT 100 / POINT_ID_LAND 200 machine
-- over dragonCommon[6] (waypointId wrap + EVENT_FREE_MOVEMENT 500ms)
-- + LAND DoZoneInCombat + random-threat-player AddThreat(1.0) +
-- Attack(true) + MoveChase; the != IN_PROGRESS -> EnterEvadeMode leg
-- is OMITTED per the checkpoint head NEXT (the drake returns without
-- moving when Sartharion is not IN_PROGRESS); the
-- GetMotionMaster()->Clear() leg is vacuous post-arrival (the spline
-- already landed).
-- OpenPortal PORTED 2026-11-05 (tick 255) in openPortal below (egg /
-- acolyte summons, Vesperon InterruptNonMeleeSpells + 32747 cast, both
-- Talks); the portal SetRespawnTime(30000) leg stays PARKED — Go GO
-- SetRespawnTime stores only the delay leg with no static-GO respawn
-- consumer (lua_objects.go:4318). The UpdateAI EVENT_FREE_MOVEMENT
-- machine is PORTED 2026-11-07 (tick 394, freeMovementTick below) —
-- the old "no motion bridge" claim is stale (ev28 + MoveTo live).
-- npc_tenebron — Reset (base); JustEngagedWith (base +
-- ScheduleEvent EVENT_HATCH_EGGS 30s); UpdateAI EVENT_HATCH_EGGS ->
-- OpenPortal PORTED 2026-11-05 (tick 255, hatchEggsTick 30s/30s).
-- npc_shadron — Reset (base + the self HasAura-gated aura strips
-- SPELL_TWILIGHT_TORMENT_VESP 57948 / SPELL_GIFT_OF_TWILIGTH_SHA
-- 57835 PORTED; SetBossState(DATA_PORTAL_OPEN, NOT_STARTED) PORTED
-- 2026-11-05 (tick 255, SetBossState bridge live — IsAlive-guarded:
-- the Go death funnel fires ev23 on death too, and C++ Reset never
-- runs on a corpse)); JustEngagedWith (base + ScheduleEvent
-- EVENT_ACOLYTE_SHADRON 1min); UpdateAI EVENT_ACOLYTE_SHADRON
-- PORTED 2026-11-05 (tick 255, shadronAcolyteTick: NOT_STARTED 10s
-- re-poll, HasAura gift leg consumes the event with no re-arm, else
-- OpenPortal + SetBossState(DATA_PORTAL_OPEN, IN_PROGRESS) +
-- {60s,65s} re-arm).
-- npc_vesperon — Reset (base); JustEngagedWith (base +
-- ScheduleEvent EVENT_ACOLYTE_VESPERON 1min); UpdateAI
-- EVENT_ACOLYTE_VESPERON PORTED 2026-11-05 (tick 255,
-- vesperonAcolyteTick: IN_PROGRESS 10s re-poll, else OpenPortal +
-- DoCastVictim(SPELL_TWILIGHT_TORMENT_VESP 57948) + {60s,70s}
-- re-arm; the old unconditional tormentTick is retired — the
-- DATA_PORTAL_OPEN gate is bridged now).
-- npc_acolyte_of_shadron (NPC_ACOLYTE_OF_SHADRON 31218,
-- entry-verifiable from the local enum) — Reset self
-- AddAura(SPELL_TWILIGHT_SHIFT_ENTER 57620) PORTED; DespawnOrUnsummon(28s)
-- PORTED below as a 28s one-shot creature:Despawn; the gift-aura legs
-- (58766 on Sartharion when IN_PROGRESS, else 57835 on Shadron) PORTED
-- 2026-11-05 (tick 255, GetInstanceCreature bridge live); JustDied
-- PORTED 2026-11-05 (tick 255, acolyteShadronDied: SetBossState
-- (DATA_PORTAL_OPEN, NOT_STARTED) + the player-loop de-phase casts
-- (player:CastSpell + player:GetVictim bridges live) + the gift-aura
-- strips on Sartharion/Shadron).
-- npc_acolyte_of_vesperon (NPC_ACOLYTE_OF_VESPERON 31219) —
-- Reset self AddAura(SPELL_TWILIGHT_SHIFT_ENTER 57620) +
-- DoCast(me, SPELL_TWILIGHT_TORMENT_VESP_ACO 58853) (2-arg
-- non-triggered self-cast) PORTED; JustDied PORTED 2026-11-05
-- (tick 255, acolyteVesperonDied: self RemoveAura + Vesperon
-- SetBossState(DATA_PORTAL_OPEN, NOT_STARTED) + torment strip +
-- the player-loop de-phase casts/removes + the three
-- DoRemoveAurasDueToSpellOnPlayers legs via GetPlayersInGrid(500));
-- UpdateAI melee.
-- npc_twilight_eggs (NPC_TWILIGHT_EGG 30882 /
-- NPC_SARTHARION_TWILIGHT_EGG 31204) — Reset self
-- AddAura(SPELL_TWILIGHT_SHIFT_ENTER 57620) PORTED; the
-- EVENT_TWILIGHT_EGGS 20s payload (SetBossState + SpawnWhelps whelp
-- summons NPC_TWILIGHT_WHELP 30890 / NPC_SARTHARION_TWILIGHT_WHELP
-- 31214 + KillSelf) PORTED 2026-11-05 (tick 255, twilightEggTick;
-- IsAlive guards cover the Go Kill() ev23-refire artifact and
-- player-killed eggs); JustSummoned DoZoneInCombat PORTED (ev19).
-- npc_flame_tsunami (NPC_FLAME_TSUNAMI 30616, boss_sartharion.cpp
-- enum) — EVENT_TSUNAMI_TIMER DoCast(me,
-- SPELL_FLAME_TSUNAMI_DMG_AURA 57491) (2-arg non-triggered
-- self-cast, 100ms init / 500ms re-arm) PORTED via CreateLuaEvent
-- (seeded on Reset, cancelled on event 37 — the wave is stationary;
-- the C++ leap motion has no bridge); Reset SetReactState(REACT_PASSIVE)
-- + SetFlag(NON_ATTACKABLE | NOT_SELECTABLE) PORTED (bridged);
-- EVENT_TSUNAMI_BUFF (nearest live NPC_LAVA_BLAZE 30643 within 10yd
-- self-casts SPELL_FLAME_TSUNAMI_BUFF 60430, triggered, 1s/1s)
-- PORTED via the FindNearestCreature bridge; the ctor legs
-- (SetDisplayId 11686 + aura SPELL_FLAME_TSUNAMI 57494) are PORTED
-- via event 5 (OnSpawn) — the stale "no spawn / display bridges"
-- claim retracted 2026-10-21.
-- npc_twilight_fissure — entry UNVERIFIABLE from C++ (no local
-- NPC enum; the fissure is spawned by SPELL_SHADOW_FISSURE
-- 57579, entry not named in the C++ tree) — joins the
-- entry-unverifiable queue (the npc_image_belgaristrasz precedent);
-- Reset / UpdateAI (DoCastAOE(SPELL_VOID_BLAST 57581) +
-- RemoveAllAuras + KillSelf): AddAura / RemoveAllAuras / CastSpell
-- are bridged (stale "no aura / cast bridges" claim retracted),
-- but the AI cannot register without a verifiable entry, so all
-- legs stay out on the entry gate.
-- npc_twilight_whelp — PORTED 2026-10-16: the EVENT_FADE_ARMOR
-- timer leg is bridgeable (CreateLuaEvent; see whelpEnterCombat /
-- fadeArmorTick below); Reset's RemoveAllAuras leg ported
-- 2026-10-19; the DoZoneInCombat leg stays out (re-evaluated
-- 2026-10-23 against the live bridge: arm lives in Reset, not a
-- JustEngagedWith — documented deviation).
-- achievement_twilight_assist / achievement_twilight_duo /
-- achievement_twilight_zone (target->GetAI()->GetData
-- (TWILIGHT_ACHIEVEMENTS) >= 1 / >= 2 / == 3 — Sartharion's
-- GetData arm, itself documented-only) — no achievement bridge;
-- all three join the unmodeled-achievement queue.

local ENTRY_TENEBRON = 30452
local ENTRY_SHADRON = 30451
local ENTRY_VESPERON = 30449

local SAY_AGGRO  = 0
local SAY_SLAY   = 1
local SAY_DEATH  = 2
local SAY_BREATH = 3

local SPELL_SHADOW_BREATH = 57570
local SPELL_SHADOW_FISSURE = 57579
local SPELL_TWILIGHT_TORMENT_VESP = 57948
local SPELL_GIFT_OF_TWILIGTH_SHA = 57835
local SPELL_POWER_OF_TENEBRON = 61248
local SPELL_POWER_OF_SHADRON = 58105
local SPELL_POWER_OF_VESPERON = 61251
local SPELL_FADE_ARMOR = 60708

local ENTRY_TWILIGHT_WHELP = 30890
local ENTRY_SARTHARION_TWILIGHT_WHELP = 31214

-- Instance data (obsidian_sanctum.h OSDataTypes) + boss states.
local DATA_SARTHARION = 0
local DATA_TENEBRON = 1
local DATA_SHADRON = 2
local DATA_VESPERON = 3
local DATA_PORTAL_OPEN = 4
local DATA_CAN_LOOT = 0
local BOSS_STATE_NOT_STARTED = 0
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_DONE = 3

local ENTRY_ACOLYTE_SHADRON = 31218
local ENTRY_ACOLYTE_VESPERON = 31219
local ENTRY_TWILIGHT_EGG = 30882
local ENTRY_SARTHARION_TWILIGHT_EGG = 31204

local POINT_ID_INIT = 100 -- obsidian_sanctum.cpp:86 / boss_sartharion.cpp:72
local POINT_ID_LAND = 200 -- obsidian_sanctum.cpp:87 / boss_sartharion.cpp:73
local MAX_WAYPOINT = 6 -- obsidian_sanctum.cpp:95

-- C++ dragonCommon (obsidian_sanctum.cpp:96-103): points around the
-- raid "isle", counter clockwise.
local DRAGON_COMMON = {
    { 3214.012, 468.932, 98.652 },
    { 3244.950, 468.427, 98.652 },
    { 3283.520, 496.869, 98.652 },
    { 3287.316, 555.875, 98.652 },
    { 3250.479, 585.827, 98.652 },
    { 3209.969, 566.523, 98.652 },
}

local WHISPER_OPEN_PORTAL = 6
local WHISPER_OPENED_PORTAL = 7

local SPELL_TWILIGHT_SHIFT = 57874
local SPELL_TWILIGHT_SHIFT_REMOVAL_ALL = 61190
local SPELL_TWILIGHT_RESIDUE = 61885
local SPELL_TWILIGHT_REVENGE = 60639
local SPELL_GIFT_OF_TWILIGTH_SAR = 58766
local SPELL_VESPERON_PORTAL_CHANNEL = 32747 -- unnamed literal, obsidian_sanctum.cpp:290

-- C++ TwilightEggs / TwilightEggsSarth (obsidian_sanctum.cpp:125-140).
local TWILIGHT_EGGS = {
    { 3219.28, 669.121, 88.5549 },
    { 3221.55, 682.852, 90.5361 },
    { 3239.77, 685.94, 90.3168 },
    { 3250.33, 669.749, 88.7637 },
    { 3246.6, 642.365, 84.8752 },
    { 3233.68, 653.117, 85.7051 },
}
local TWILIGHT_EGGS_SARTH = {
    { 3252.73, 515.762, 58.5501 },
    { 3256.56, 521.119, 58.6061 },
    { 3255.63, 527.513, 58.7568 },
    { 3264.90, 525.865, 58.6436 },
    { 3264.26, 516.364, 58.8011 },
    { 3257.54, 502.285, 58.2077 },
}
local ACOLYTE_SHADRON_POS = { 3363.92, 534.703, 97.2683 }
local ACOLYTE_SHADRON_POS2 = { 3246.57, 551.263, 58.6164 }
local ACOLYTE_VESPERON_POS = { 3145.68, 520.71, 89.7 }
local ACOLYTE_VESPERON_POS2 = { 3246.57, 551.263, 58.6164 }

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

-- C++ SelectTarget(Random, 0, 0.0f, true) leg: threat-list players
-- alive, no distance filter (UnitAI.h: SelectTarget draws the threat
-- list; dead = offline refs). The old GetPlayersInWorld instance pool
-- could pick players never engaged with the drake (2026-10-25 fix,
-- same class as the archavon/nexus/ahnkahet threat-pool fixes).
local function randomThreatPlayer(creature)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:GetObjectType() == "Player" and t:IsAlive() then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EVENT_SHADOW_FISSURE (dummy_dragonAI::ExecuteEvent,
-- obsidian_sanctum.cpp:387-391): SelectTarget(Random, 0, 0.0f, true)
-- (playerOnly=true, threat-list pool) -> randomThreatPlayer(creature),
-- nil-guarded pick (C++ `if` — skip cast on nil); DoCast 2-arg =
-- non-triggered -> CastSpell(target, spell); re-arm {15s,20s}
-- unconditional (outside the target-if, C++-exact); 5s init (:192).
local function shadowFissureTick(creature, guid)
    local target = randomThreatPlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_SHADOW_FISSURE)
    end
    schedule(guid, "shadowfissure", {15000, 20000}, function()
        shadowFissureTick(creature, guid)
    end)
end

-- C++ EVENT_SHADOW_BREATH (dummy_dragonAI::ExecuteEvent):
-- Talk(SAY_BREATH 3) + DoCastVictim(SPELL_SHADOW_BREATH 57570)
-- (non-triggered), 20s init, re-arm {20s,25s}.
local function shadowBreathTick(creature, guid)
    creature:Talk(SAY_BREATH)
    creature:CastSpell(nil, SPELL_SHADOW_BREATH)
    schedule(guid, "shadowbreath", {20000, 25000}, function()
        shadowBreathTick(creature, guid)
    end)
end

-- C++ dummy_dragonAI::Initialize (obsidian_sanctum.cpp:171-176):
-- waypointId = 0, _canMoveFree = false. Per-GUID latch; seeded in
-- dragonSpawn (the ctor twin) and re-zeroed in dragonReset (C++
-- Reset calls Initialize).
local dragonFlight = {}

-- C++ dummy_dragonAI::UpdateAI EVENT_FREE_MOVEMENT
-- (obsidian_sanctum.cpp:371-379): one-shot — if _canMoveFree and
-- waypointId < MAX_WAYPOINT, MovePoint(waypointId,
-- dragonCommon[waypointId]); the next schedule comes from the next
-- MovementInform arrival, never from here.
local function freeMovementTick(creature, guid)
    local flight = dragonFlight[guid]
    if flight and flight.canMoveFree and flight.waypointId < MAX_WAYPOINT then
        local pt = DRAGON_COMMON[flight.waypointId + 1]
        creature:MoveTo(flight.waypointId, pt[1], pt[2], pt[3])
    end
end

-- C++ dummy_dragonAI::MovementInform (obsidian_sanctum.cpp:202-245):
-- POINT_MOTION_TYPE is moveType 8 (krick precedent); the instance-nil
-- leg is vacuous (drakes always have an instance script). The
-- GetBossState(DATA_SARTHARION) != IN_PROGRESS -> EnterEvadeMode leg
-- is OMITTED per the checkpoint head NEXT (the drake returns without
-- moving when Sartharion is not IN_PROGRESS). POINT_ID_LAND (200):
-- DoZoneInCombat + random-threat-player AddThreat(1.0) + Attack(true)
-- + MoveChase + _canMoveFree = false (the GetMotionMaster()->Clear()
-- leg is vacuous post-arrival — the spline already landed). Any other
-- point: waypointId = pointId + 1, wrap at MAX_WAYPOINT (6) with
-- _canMoveFree = true, then EVENT_FREE_MOVEMENT in 500ms.
local function dragonMovementInform(_, creature, moveType, id)
    if moveType ~= 8 then
        return
    end
    if creature:GetBossState(DATA_SARTHARION) ~= BOSS_STATE_IN_PROGRESS then
        return
    end
    local guid = creature:GetGUID()
    local flight = dragonFlight[guid]
    if not flight then
        flight = { waypointId = 0, canMoveFree = false }
        dragonFlight[guid] = flight
    end
    if id == POINT_ID_LAND then
        creature:DoZoneInCombat()
        local target = randomThreatPlayer(creature)
        if target then
            creature:AddThreat(target, 1.0)
            creature:Attack(target, true)
            creature:MoveChase(target)
        end
        flight.canMoveFree = false
        return
    end
    flight.waypointId = id + 1
    if flight.waypointId >= MAX_WAYPOINT then
        flight.canMoveFree = true
        flight.waypointId = 0
    end
    schedule(guid, "free_movement", 500, function()
        freeMovementTick(creature, guid)
    end)
end

-- C++ dummy_dragonAI::OpenPortal (obsidian_sanctum.cpp:257-315),
-- PORTED 2026-11-05 (tick 255): entry-switched egg / acolyte summons
-- (type 5 CORPSE_TIMED_DESPAWN, 20s eggs / 28s shadron acolyte /
-- 20s vesperon acolyte) + the Vesperon InterruptNonMeleeSpells +
-- 32747 self-cast leg + Talk(WHISPER_OPEN_PORTAL 6) +
-- Talk(WHISPER_OPENED_PORTAL 7). The portal SetRespawnTime(30000)
-- leg stays PARKED (Go GO SetRespawnTime has no static-GO respawn
-- consumer, lua_objects.go:4318), so no FindNearestGameObject
-- search runs — the search result feeds only that leg.
local function openPortal(creature)
    local entry = creature:GetEntry()
    local sartharionInProgress = creature:GetBossState(DATA_SARTHARION) == BOSS_STATE_IN_PROGRESS
    if entry == ENTRY_TENEBRON then
        local eggs = TWILIGHT_EGGS
        local eggEntry = ENTRY_TWILIGHT_EGG
        if sartharionInProgress then
            eggs = TWILIGHT_EGGS_SARTH
            eggEntry = ENTRY_SARTHARION_TWILIGHT_EGG
        end
        for _, pos in ipairs(eggs) do
            creature:SummonCreature(eggEntry, pos[1], pos[2], pos[3], 0.0, 5, 20000)
        end
    elseif entry == ENTRY_SHADRON then
        local pos = ACOLYTE_SHADRON_POS
        if sartharionInProgress then
            pos = ACOLYTE_SHADRON_POS2
        end
        creature:SummonCreature(ENTRY_ACOLYTE_SHADRON, pos[1], pos[2], pos[3], 0.0, 5, 28000)
    elseif entry == ENTRY_VESPERON then
        local pos = ACOLYTE_VESPERON_POS
        if sartharionInProgress then
            pos = ACOLYTE_VESPERON_POS2
        end
        local acolyte = creature:SummonCreature(ENTRY_ACOLYTE_VESPERON, pos[1], pos[2], pos[3], 0.0, 5, 20000)
        if acolyte then
            creature:InterruptNonMeleeSpells(true)
            acolyte:InterruptNonMeleeSpells(true)
            creature:CastSpell(creature, SPELL_VESPERON_PORTAL_CHANNEL, false)
        end
    end
    creature:Talk(WHISPER_OPEN_PORTAL)
    creature:Talk(WHISPER_OPENED_PORTAL)
end

-- C++ npc_tenebron::UpdateAI EVENT_HATCH_EGGS
-- (obsidian_sanctum.cpp:451-453): OpenPortal, unconditional 30s
-- re-arm, 30s init.
local function hatchEggsTick(creature, guid)
    openPortal(creature)
    schedule(guid, "hatcheggs", 30000, function()
        hatchEggsTick(creature, guid)
    end)
end

-- C++ npc_shadron::UpdateAI EVENT_ACOLYTE_SHADRON
-- (obsidian_sanctum.cpp:519-531): DATA_PORTAL_OPEN NOT_STARTED ->
-- 10s re-poll; else HasAura(GIFT_SHA) consumes the event with no
-- re-arm (C++ `return` from UpdateAI); else OpenPortal +
-- SetBossState(DATA_PORTAL_OPEN, IN_PROGRESS) + {60s,65s} re-arm.
local function shadronAcolyteTick(creature, guid)
    if creature:GetBossState(DATA_PORTAL_OPEN) == BOSS_STATE_NOT_STARTED then
        schedule(guid, "acolyte", 10000, function()
            shadronAcolyteTick(creature, guid)
        end)
        return
    end
    if creature:HasAura(SPELL_GIFT_OF_TWILIGTH_SHA) then
        return
    end
    openPortal(creature)
    creature:SetBossState(DATA_PORTAL_OPEN, BOSS_STATE_IN_PROGRESS)
    schedule(guid, "acolyte", {60000, 65000}, function()
        shadronAcolyteTick(creature, guid)
    end)
end

-- C++ npc_vesperon::UpdateAI EVENT_ACOLYTE_VESPERON
-- (obsidian_sanctum.cpp:590-597): DATA_PORTAL_OPEN IN_PROGRESS ->
-- 10s re-poll; else OpenPortal + DoCastVictim
-- (SPELL_TWILIGHT_TORMENT_VESP 57948, non-triggered) + {60s,70s}
-- re-arm.
local function vesperonAcolyteTick(creature, guid)
    if creature:GetBossState(DATA_PORTAL_OPEN) == BOSS_STATE_IN_PROGRESS then
        schedule(guid, "acolyte", 10000, function()
            vesperonAcolyteTick(creature, guid)
        end)
        return
    end
    openPortal(creature)
    creature:CastSpell(nil, SPELL_TWILIGHT_TORMENT_VESP)
    schedule(guid, "acolyte", {60000, 70000}, function()
        vesperonAcolyteTick(creature, guid)
    end)
end

-- C++ dummy_dragonAI::JustEngagedWith: Talk(SAY_AGGRO) +
-- DoZoneInCombat + ScheduleEvent(EVENT_SHADOW_FISSURE,
-- EVENT_SHADOW_BREATH) — the Talk and both timer legs are bridgeable
-- (the old "no target-selection bridge" claim for the fissure leg was
-- stale — randomThreatPlayer); the DoZoneInCombat leg is PORTED
-- 2026-10-23 (bridge live).
-- Plus the per-drake engage timers, PORTED 2026-11-05 (tick 255):
-- EVENT_HATCH_EGGS (tenebron, hatchEggsTick 30s), EVENT_ACOLYTE_SHADRON
-- (shadron, shadronAcolyteTick 1min), EVENT_ACOLYTE_VESPERON (vesperon,
-- vesperonAcolyteTick 1min).
local function dragonEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    creature:DoZoneInCombat() -- obsidian_sanctum.cpp:189 (dummy_dragonAI::JustEngagedWith arm 2, C++ order; bridge live 2026-10-23)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "shadowbreath", 20000, function()
        shadowBreathTick(creature, guid)
    end)
    schedule(guid, "shadowfissure", 5000, function()
        shadowFissureTick(creature, guid)
    end)
    if creature:GetEntry() == ENTRY_TENEBRON then
        schedule(guid, "hatcheggs", 30000, function()
            hatchEggsTick(creature, guid)
        end)
    elseif creature:GetEntry() == ENTRY_SHADRON then
        schedule(guid, "acolyte", 60000, function()
            shadronAcolyteTick(creature, guid)
        end)
    elseif creature:GetEntry() == ENTRY_VESPERON then
        schedule(guid, "acolyte", 60000, function()
            vesperonAcolyteTick(creature, guid)
        end)
    end
end

local function dragonLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ dummy_dragonAI::KilledUnit: victim->GetTypeId() ==
-- TYPEID_PLAYER gate, then Talk(SAY_SLAY) — the razuvious
-- player-gated variant precedent.
local function dragonTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

local powerAuraByEntry = {
    [ENTRY_TENEBRON] = SPELL_POWER_OF_TENEBRON,
    [ENTRY_SHADRON] = SPELL_POWER_OF_SHADRON,
    [ENTRY_VESPERON] = SPELL_POWER_OF_VESPERON,
}

-- C++ dummy_dragonAI::JustDied (obsidian_sanctum.cpp:318-377), PORTED
-- 2026-11-05 (tick 255) in C++ arm order: _canLoot/SetLootRecipient
-- (the ev5 SetData seed resolves the default-0 trap — a drake that
-- never heard SetData reads 0, the C++ member default is true, so the
-- read is only meaningful with the seed) + entry-switched
-- SetBossState(DATA_TENEBRON/SHADRON/VESPERON, DONE, sartharion !=
-- IN_PROGRESS gate) + Shadron/Vesperon acolyte KillSelf (nearest of
-- own kind within 100yd) + Talk(SAY_DEATH) + entry-switched SELF
-- RemoveAurasDueToSpell(spellId) + DoRemoveAurasDueToSpellOnPlayers
-- via GetPlayersInGrid(500) (whole-instance twin — the instance spans
-- ~220 units; dead players are skipped by the bridge, a documented
-- deviation from the C++ all-players removal) + Twilight Revenge on
-- Sartharion (IN_PROGRESS gate: RemoveAurasDueToSpell + triggered
-- DoCast 60639 via GetInstanceCreature).
local drakeDataByEntry = {
    [ENTRY_TENEBRON] = DATA_TENEBRON,
    [ENTRY_SHADRON] = DATA_SHADRON,
    [ENTRY_VESPERON] = DATA_VESPERON,
}

local function dragonDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    if creature:GetData(DATA_CAN_LOOT) == 0 then
        creature:SetLootRecipient()
    end
    local entry = creature:GetEntry()
    local spellId = powerAuraByEntry[entry]
    local dataId = drakeDataByEntry[entry]
    if dataId and creature:GetBossState(DATA_SARTHARION) ~= BOSS_STATE_IN_PROGRESS then
        creature:SetBossState(dataId, BOSS_STATE_DONE)
    end
    local acolyteEntry = nil
    if entry == ENTRY_SHADRON then
        acolyteEntry = ENTRY_ACOLYTE_SHADRON
    elseif entry == ENTRY_VESPERON then
        acolyteEntry = ENTRY_ACOLYTE_VESPERON
    end
    if acolyteEntry ~= nil then
        local acolyte = creature:FindNearestCreature(acolyteEntry, 100.0)
        if acolyte ~= nil then
            acolyte:Kill()
        end
    end
    creature:Talk(SAY_DEATH)
    if spellId then
        creature:RemoveAura(spellId)
        for _, player in ipairs(creature:GetPlayersInGrid(500)) do
            player:RemoveAura(spellId)
        end
    end
    if creature:GetBossState(DATA_SARTHARION) == BOSS_STATE_IN_PROGRESS and spellId then
        local sartharion = creature:GetInstanceCreature(DATA_SARTHARION)
        if sartharion and sartharion:IsAlive() then
            sartharion:RemoveAura(spellId)
            creature:CastSpell(sartharion, SPELL_TWILIGHT_REVENGE, true)
        end
    end
end

local function dragonReset(event, creature)
    -- C++ dummy_dragonAI::Reset (obsidian_sanctum.cpp:178-181): the
    -- HasFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)-gated
    -- RemoveFlag leg, in C++ arm order before events.Reset; the flag
    -- is live in this port (intro mask SetFlag, line 544), and
    -- creature:HasFlag is bridged (Unit::HasFlag twin).
    if creature:HasFlag(59, 0x2) then
        creature:RemoveFlag(59, 0x2)
    end
    cancelTimers(creature:GetGUID())
    -- C++ Reset calls Initialize (:184): waypointId = 0,
    -- _canMoveFree = false — the free-movement machine restarts from
    -- the next INIT arrival.
    dragonFlight[creature:GetGUID()] = { waypointId = 0, canMoveFree = false }
end

-- C++ instance OnCreatureCreate latch (instance_obsidian_sanctum.cpp:60-78)
-- mirrored via the ev5 SetGuidData population leg (the Go guid-store
-- route; sartharionSpawn precedent — without this latch
-- GetInstanceCreature(DATA_TENEBRON/SHADRON/VESPERON) reads nil and
-- the tick-254 Sartharion FetchDragons / DrakeRespawn / CallDragon
-- legs find no drakes) + the ctor Initialize _canLoot=true seed
-- (dummy_dragonAI::Initialize, obsidian_sanctum.cpp:171-176). The
-- ev5 fire is synchronous inside Respawn, so Sartharion's DrakeRespawn
-- SetData(0) write still lands after the seed (C++ order: Respawn
-- -> Initialize -> SetData).
local function dragonSpawn(event, creature)
    local dataId = drakeDataByEntry[creature:GetEntry()]
    if dataId then
        creature:SetGuidData(dataId)
    end
    creature:SetData(DATA_CAN_LOOT, 1)
    dragonFlight[creature:GetGUID()] = { waypointId = 0, canMoveFree = false }
end

for _, entry in ipairs({ ENTRY_TENEBRON, ENTRY_SHADRON, ENTRY_VESPERON }) do
    RegisterCreatureEvent(entry, 1, dragonEnterCombat)
    RegisterCreatureEvent(entry, 2, dragonLeaveCombat)
    RegisterCreatureEvent(entry, 3, dragonTargetDied)
    RegisterCreatureEvent(entry, 4, dragonDied)
    RegisterCreatureEvent(entry, 5, dragonSpawn)
    RegisterCreatureEvent(entry, 23, dragonReset)
    RegisterCreatureEvent(entry, 28, dragonMovementInform)
end

-- C++ npc_shadron::Reset: the self HasAura-gated strips of
-- SPELL_TWILIGHT_TORMENT_VESP / SPELL_GIFT_OF_TWILIGTH_SHA are PORTED
-- (stale "no aura bridge" claim retracted — HasAura/RemoveAura are
-- bridged); the SetBossState(DATA_PORTAL_OPEN, NOT_STARTED) leg has no
-- instance bridge (documented). Registered after dragonReset so the
-- timer cancel lands first, mirroring the C++ base-then-override order.
local function shadronReset(event, creature)
    if creature:HasAura(SPELL_TWILIGHT_TORMENT_VESP) then
        creature:RemoveAura(SPELL_TWILIGHT_TORMENT_VESP)
    end
    if creature:HasAura(SPELL_GIFT_OF_TWILIGTH_SHA) then
        creature:RemoveAura(SPELL_GIFT_OF_TWILIGTH_SHA)
    end
    -- C++ npc_shadron::Reset (obsidian_sanctum.cpp:511-513): the
    -- SetBossState(DATA_PORTAL_OPEN, NOT_STARTED) leg, PORTED
    -- 2026-11-05 (tick 255). IsAlive-guarded: the Go death funnel
    -- fires ev23 on death too, and C++ Reset never runs on a corpse
    -- (an unguarded write would flip the shared portal key while
    -- Vesperon's acolyte cycle still reads it).
    if creature:IsAlive() then
        creature:SetBossState(DATA_PORTAL_OPEN, BOSS_STATE_NOT_STARTED)
    end
end
RegisterCreatureEvent(ENTRY_SHADRON, 23, shadronReset)

-- C++ npc_twilight_whelp::Reset (obsidian_sanctum.cpp:983-988):
-- RemoveAllAuras ported 2026-10-19; the DoZoneInCombat leg stays out
-- (re-evaluated 2026-10-23 against the live bridge: arm lives in
-- Reset, not a JustEngagedWith — documented deviation); the
-- bridgeable leg is
-- ScheduleEvent(EVENT_FADE_ARMOR,
-- 1s), seeded here on event 1 per the svalna precedent (Reset-seeded
-- timers re-keyed to combat start). UpdateAI ExecuteEvent:
-- DoCastVictim(SPELL_FADE_ARMOR 60708) (non-triggered), re-arm
-- {5s,10s}, UpdateVictim()-gated.
local function fadeArmorTick(creature, guid)
    creature:CastSpell(nil, SPELL_FADE_ARMOR)
    schedule(guid, "fadearmor", {5000, 10000}, function()
        fadeArmorTick(creature, guid)
    end)
end

local function whelpEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "fadearmor", 1000, function()
        fadeArmorTick(creature, guid)
    end)
end

local function whelpLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function whelpDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function whelpReset(event, creature)
    creature:RemoveAllAuras()
    cancelTimers(creature:GetGUID())
end

for _, entry in ipairs({ ENTRY_TWILIGHT_WHELP, ENTRY_SARTHARION_TWILIGHT_WHELP }) do
    RegisterCreatureEvent(entry, 1, whelpEnterCombat)
    RegisterCreatureEvent(entry, 2, whelpLeaveCombat)
    RegisterCreatureEvent(entry, 4, whelpDied)
    RegisterCreatureEvent(entry, 23, whelpReset)
end

-- New arms (ported 2026-10-18, entries C++-verifiable):
local SPELL_TWILIGHT_SHIFT_ENTER = 57620
local SPELL_TWILIGHT_TORMENT_VESP_ACO = 58853
local SPELL_FLAME_TSUNAMI_DMG_AURA = 57491
local SPELL_FLAME_TSUNAMI = 57494 -- obsidian_sanctum.cpp:78 (visual dummy)
local SPELL_FLAME_TSUNAMI_BUFF = 60430
local NPC_LAVA_BLAZE = 30643
local REACT_PASSIVE = 0
local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NON_ATTACKABLE = 0x2
local UNIT_FLAG_NOT_SELECTABLE = 0x2000000
local UNIT_FLAG_INTRO_MASK = UNIT_FLAG_NON_ATTACKABLE + UNIT_FLAG_NOT_SELECTABLE
local ENTRY_FLAME_TSUNAMI = 30616

-- C++ npc_acolyte_of_shadron::Reset (obsidian_sanctum.cpp:632-650):
-- me->AddAura(SPELL_TWILIGHT_SHIFT_ENTER, me) — self-aura leg
-- ported; DespawnOrUnsummon(28s) ports as a 28s one-shot
-- self-despawn (arm-if-absent: the C++ despawn countdown keeps
-- running through evade, so the ev23-on-evade refire must not
-- re-arm it; ev4 cancels it); the instance-gated gift-aura legs
-- stay out.
local function acolyteShadronReset(event, creature)
    -- The Go Kill() funnel fires ev23 on death before ev4; a dead
    -- acolyte must not re-arm the despawn timer or re-cast auras
    -- (C++ Reset never runs on a corpse).
    if not creature:IsAlive() then
        return
    end
    creature:AddAura(SPELL_TWILIGHT_SHIFT_ENTER)
    -- C++ npc_acolyte_of_shadron::Reset gift legs
    -- (obsidian_sanctum.cpp:639-648): Sartharion IN_PROGRESS -> the
    -- boss gains GIFT_OF_TWILIGTH_SAR 58766, else Shadron gains
    -- GIFT_OF_TWILIGTH_SHA 57835. PORTED 2026-11-05 (tick 255).
    if creature:GetBossState(DATA_SARTHARION) == BOSS_STATE_IN_PROGRESS then
        local sartharion = creature:GetInstanceCreature(DATA_SARTHARION)
        if sartharion then
            sartharion:AddAura(SPELL_GIFT_OF_TWILIGTH_SAR)
        end
    else
        local shadron = creature:GetInstanceCreature(DATA_SHADRON)
        if shadron then
            shadron:AddAura(SPELL_GIFT_OF_TWILIGTH_SHA)
        end
    end
    local guid = creature:GetGUID()
    local per = timers[guid]
    if not (per and per["despawn"]) then
        schedule(guid, "despawn", 28000, function()
            cancelTimers(guid)
            creature:Despawn()
        end)
    end
end
-- C++ npc_acolyte_of_shadron::JustDied (obsidian_sanctum.cpp:653-684),
-- PORTED 2026-11-05 (tick 255): SetBossState(DATA_PORTAL_OPEN,
-- NOT_STARTED) when the Shadron GUID is latched + the player-loop
-- de-phase legs (alive + HasAura(TWILIGHT_SHIFT 57874) + no victim:
-- self-cast TWILIGHT_SHIFT_REMOVAL_ALL 61190 + TWILIGHT_RESIDUE
-- 61885, triggered, then remove SHIFT + SHIFT_ENTER) + the gift-aura
-- strips (Sartharion 58766 / Shadron 57835, alive + HasAura-gated).
-- The stale "no player:CastSpell / no !GetVictim() bridge" claims are
-- retracted (lua_player.go:2752 / :2957). Timer cancel folds in here
-- (replaces the old acolyteDespawnCancel registration).
local function acolyteShadronDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    if creature:GetInstanceCreature(DATA_SHADRON) then
        creature:SetBossState(DATA_PORTAL_OPEN, BOSS_STATE_NOT_STARTED)
    end
    for _, player in ipairs(creature:GetPlayersInGrid(500)) do
        if player:IsAlive() and player:HasAura(SPELL_TWILIGHT_SHIFT) and not player:GetVictim() then
            player:CastSpell(player, SPELL_TWILIGHT_SHIFT_REMOVAL_ALL, true)
            player:CastSpell(player, SPELL_TWILIGHT_RESIDUE, true)
            player:RemoveAura(SPELL_TWILIGHT_SHIFT)
            player:RemoveAura(SPELL_TWILIGHT_SHIFT_ENTER)
        end
    end
    local sartharion = creature:GetInstanceCreature(DATA_SARTHARION)
    if sartharion and sartharion:IsAlive() and sartharion:HasAura(SPELL_GIFT_OF_TWILIGTH_SAR) then
        sartharion:RemoveAura(SPELL_GIFT_OF_TWILIGTH_SAR)
    end
    local shadron = creature:GetInstanceCreature(DATA_SHADRON)
    if shadron and shadron:IsAlive() and shadron:HasAura(SPELL_GIFT_OF_TWILIGTH_SHA) then
        shadron:RemoveAura(SPELL_GIFT_OF_TWILIGTH_SHA)
    end
end
RegisterCreatureEvent(ENTRY_ACOLYTE_SHADRON, 23, acolyteShadronReset)
RegisterCreatureEvent(ENTRY_ACOLYTE_SHADRON, 4, acolyteShadronDied)

-- C++ npc_acolyte_of_vesperon::Reset (obsidian_sanctum.cpp:718-726):
-- me->AddAura(SPELL_TWILIGHT_SHIFT_ENTER, me) + DoCast(me,
-- SPELL_TWILIGHT_TORMENT_VESP_ACO) (2-arg non-triggered self-cast)
-- ported; DespawnOrUnsummon(28s) ports as a 28s one-shot
-- self-despawn (same arm-if-absent/ev4-cancel terms as shadron).
local function acolyteVesperonReset(event, creature)
    -- Same Kill()-ev23 artifact guard as the Shadron acolyte.
    if not creature:IsAlive() then
        return
    end
    creature:AddAura(SPELL_TWILIGHT_SHIFT_ENTER)
    creature:CastSpell(creature, SPELL_TWILIGHT_TORMENT_VESP_ACO)
    local guid = creature:GetGUID()
    local per = timers[guid]
    if not (per and per["despawn"]) then
        schedule(guid, "despawn", 28000, function()
            cancelTimers(guid)
            creature:Despawn()
        end)
    end
end
RegisterCreatureEvent(ENTRY_ACOLYTE_VESPERON, 23, acolyteVesperonReset)

-- C++ npc_acolyte_of_vesperon::JustDied (obsidian_sanctum.cpp:728):
-- me->RemoveAurasDueToSpell(SPELL_TWILIGHT_TORMENT_VESP_ACO) —
-- self leg ported; the instance/vesperon/player legs stay out.
local function acolyteVesperonDied(event, creature, killer)
    creature:RemoveAura(SPELL_TWILIGHT_TORMENT_VESP_ACO)
    cancelTimers(creature:GetGUID())
    -- C++ npc_acolyte_of_vesperon::JustDied (obsidian_sanctum.cpp:728-766),
    -- PORTED 2026-11-05 (tick 255): Vesperon SetBossState(DATA_PORTAL_OPEN,
    -- NOT_STARTED) + torment strip (alive + HasAura-gated) + the
    -- player-loop de-phase legs (same gates as the Shadron acolyte) +
    -- the per-player torment removal + the three
    -- DoRemoveAurasDueToSpellOnPlayers legs (58853 / 57935 / 58835,
    -- unnamed literals in C++) via GetPlayersInGrid(500).
    local vesperon = creature:GetInstanceCreature(DATA_VESPERON)
    if vesperon then
        creature:SetBossState(DATA_PORTAL_OPEN, BOSS_STATE_NOT_STARTED)
        if vesperon:IsAlive() and vesperon:HasAura(SPELL_TWILIGHT_TORMENT_VESP) then
            vesperon:RemoveAura(SPELL_TWILIGHT_TORMENT_VESP)
        end
    end
    local players = creature:GetPlayersInGrid(500)
    for _, player in ipairs(players) do
        if player:IsAlive() and player:HasAura(SPELL_TWILIGHT_SHIFT) and not player:GetVictim() then
            player:CastSpell(player, SPELL_TWILIGHT_SHIFT_REMOVAL_ALL, true)
            player:CastSpell(player, SPELL_TWILIGHT_RESIDUE, true)
            player:RemoveAura(SPELL_TWILIGHT_SHIFT)
            player:RemoveAura(SPELL_TWILIGHT_SHIFT_ENTER)
        end
        if player:IsAlive() and player:HasAura(SPELL_TWILIGHT_TORMENT_VESP) and not player:GetVictim() then
            player:RemoveAura(SPELL_TWILIGHT_TORMENT_VESP)
        end
    end
    for _, player in ipairs(players) do
        player:RemoveAura(SPELL_TWILIGHT_TORMENT_VESP_ACO)
        player:RemoveAura(57935) -- TWILIGHT_TORMENT trigger component, obsidian_sanctum.cpp:764
        player:RemoveAura(58835) -- TWILIGHT_TORMENT trigger component, obsidian_sanctum.cpp:765
    end
end
RegisterCreatureEvent(ENTRY_ACOLYTE_VESPERON, 4, acolyteVesperonDied)

-- C++ npc_twilight_eggs::UpdateAI EVENT_TWILIGHT_EGGS + SpawnWhelps
-- (obsidian_sanctum.cpp:803-828), PORTED 2026-11-05 (tick 255): 20s
-- from Reset; if the tenebron GUID is latched, SetBossState
-- (DATA_PORTAL_OPEN, NOT_STARTED); RemoveAllAuras; summon the whelp
-- at the egg's position (type 1 TIMED_OR_CORPSE_DESPAWN, 60s) —
-- Sartharion variant when Sartharion is IN_PROGRESS; KillSelf. The
-- ev19 JustSummoned handler below arms the whelp's zone combat.
local function twilightEggTick(creature, guid)
    if not creature:IsAlive() then
        return
    end
    if creature:GetInstanceCreature(DATA_TENEBRON) then
        creature:SetBossState(DATA_PORTAL_OPEN, BOSS_STATE_NOT_STARTED)
    end
    creature:RemoveAllAuras()
    local whelpEntry = ENTRY_TWILIGHT_WHELP
    if creature:GetBossState(DATA_SARTHARION) == BOSS_STATE_IN_PROGRESS then
        whelpEntry = ENTRY_SARTHARION_TWILIGHT_WHELP
    end
    creature:SummonCreature(whelpEntry, creature:GetX(), creature:GetY(), creature:GetZ(), 0.0, 1, 60000)
    creature:Kill()
end

local function twilightEggDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

-- C++ npc_twilight_eggs::Reset (obsidian_sanctum.cpp:804-809):
-- me->AddAura(SPELL_TWILIGHT_SHIFT_ENTER, me) ported; the
-- EVENT_TWILIGHT_EGGS 20s payload is seeded here.
local function twilightEggReset(event, creature)
    -- The Go Kill() funnel fires ev23 on death before ev4; a dead egg
    -- must not re-arm the hatch timer (C++ Reset never runs on a corpse).
    if not creature:IsAlive() then
        return
    end
    creature:AddAura(SPELL_TWILIGHT_SHIFT_ENTER)
    local guid = creature:GetGUID()
    schedule(guid, "eggtick", 20000, function()
        twilightEggTick(creature, guid)
    end)
end
for _, entry in ipairs({ ENTRY_TWILIGHT_EGG, ENTRY_SARTHARION_TWILIGHT_EGG }) do
    RegisterCreatureEvent(entry, 23, twilightEggReset)
    RegisterCreatureEvent(entry, 4, twilightEggDied)
end
for _, entry in ipairs({ ENTRY_TWILIGHT_EGG, ENTRY_SARTHARION_TWILIGHT_EGG }) do
    RegisterCreatureEvent(entry, 1, function(_, creature)
        creature:SetMeleeAttack(false) -- obsidian_sanctum.cpp:827: custom UpdateAI never calls DoMeleeAttackIfReady (whelp-spawn event machine in C++, never melees)
    end)
end

-- C++ npc_twilight_eggs::JustSummoned (obsidian_sanctum.cpp:822-825):
-- DoZoneInCombat(who) engages the spawned whelp in zone combat.
local function twilightEggJustSummoned(_, creature, summoned)
    summoned:DoZoneInCombat()
end
for _, entry in ipairs({ ENTRY_TWILIGHT_EGG, ENTRY_SARTHARION_TWILIGHT_EGG }) do
    RegisterCreatureEvent(entry, 19, twilightEggJustSummoned)
end

-- C++ npc_flame_tsunami (obsidian_sanctum.cpp:866-916): Reset —
-- SetReactState(REACT_PASSIVE) + SetFlag(UNIT_FIELD_FLAGS,
-- NON_ATTACKABLE | NOT_SELECTABLE) ported (both bridged);
-- EVENT_TSUNAMI_TIMER DoCast(me, SPELL_FLAME_TSUNAMI_DMG_AURA)
-- (2-arg non-triggered self-cast), 100ms init / 500ms re-arm —
-- ported via CreateLuaEvent, seeded on Reset, cancelled on
-- OnRemove (event 37); EVENT_TSUNAMI_BUFF (1s init / 1s re-arm) —
-- the nearest live lava blaze within 10yd self-casts
-- SPELL_FLAME_TSUNAMI_BUFF (triggered) via the FindNearestCreature
-- bridge — ported. The ctor legs (SetDisplayId 11686 + AddAura
-- SPELL_FLAME_TSUNAMI 57494) are ported via tsunamiSpawn (event 5)
-- below — the stale "no spawn hook / display bridge" claim
-- retracted.
local function tsunamiTick(creature, guid)
    creature:CastSpell(creature, SPELL_FLAME_TSUNAMI_DMG_AURA)
    schedule(guid, "tsunami", 500, function()
        tsunamiTick(creature, guid)
    end)
end

-- C++ EVENT_TSUNAMI_BUFF (obsidian_sanctum.cpp:894-897).
local function tsunamiBuffTick(creature, guid)
    local blaze = creature:FindNearestCreature(NPC_LAVA_BLAZE, 10)
    if blaze then
        blaze:CastSpell(blaze, SPELL_FLAME_TSUNAMI_BUFF, true)
    end
    schedule(guid, "tsunamiBuff", 1000, function()
        tsunamiBuffTick(creature, guid)
    end)
end

local function tsunamiReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:SetReactState(REACT_PASSIVE)
    schedule(guid, "tsunami", 100, function()
        tsunamiTick(creature, guid)
    end)
    schedule(guid, "tsunamiBuff", 1000, function()
        tsunamiBuffTick(creature, guid)
    end)
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_INTRO_MASK)
end

local function tsunamiRemove(event, creature)
    cancelTimers(creature:GetGUID())
end
RegisterCreatureEvent(ENTRY_FLAME_TSUNAMI, 23, tsunamiReset)
RegisterCreatureEvent(ENTRY_FLAME_TSUNAMI, 37, tsunamiRemove)

-- C++ npc_flame_tsunamiAI ctor (obsidian_sanctum.cpp:868-872):
-- SetDisplayId(11686) + AddAura(SPELL_FLAME_TSUNAMI 57494, me).
-- The stale "no spawn / display bridges" claim is retracted —
-- creature:SetDisplayId (lua_creature_events.go:643) and
-- creature:AddAura (self-only, lua_creature_events.go:249) are
-- bridged; event 5 (OnSpawn) is the ctor-equivalent hook
-- (the felmyst YELL_BIRTH precedent).
local function tsunamiSpawn(event, creature)
    creature:SetDisplayId(11686)
    creature:AddAura(SPELL_FLAME_TSUNAMI)
end
RegisterCreatureEvent(ENTRY_FLAME_TSUNAMI, 5, tsunamiSpawn)
RegisterCreatureEvent(ENTRY_FLAME_TSUNAMI, 1, function(_, creature)
    creature:SetMeleeAttack(false) -- obsidian_sanctum.cpp:882: custom UpdateAI never calls DoMeleeAttackIfReady (aura-upkeep event machine in C++, never melees)
end)
