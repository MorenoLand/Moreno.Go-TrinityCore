-- Old Hillsbrad zone scripts — Lua port of
-- src/server/scripts/Kalimdor/CavernsOfTime/EscapeFromDurnholdeKeep/
-- old_hillsbrad.cpp (650 lines; class npc_erozion : public CreatureScript
-- { npc_erozionAI : public ScriptedAI } + class npc_thrall_old_hillsbrad :
-- public CreatureScript { npc_thrall_old_hillsbradAI : public EscortAI } +
-- class npc_taretha : public CreatureScript { npc_tarethaAI : public
-- EscortAI }; all GetAI via GetOldHillsbradAI (OHScriptName
-- "instance_old_hillsbrad" gate); AddSC_old_hillsbrad at end registers all
-- three; kalimdor loader decl 39 / call 152 per
-- kalimdor_script_loader.cpp, in the "// CoT Old Hillsbrad" group right
-- after AddSC_instance_old_hillsbrad()).
-- Whole-server-tree quoted-name grep confirms the cpp as the sole source
-- of "npc_erozion", "npc_thrall_old_hillsbrad", and "npc_taretha" (loader
-- decl/call lines only otherwise).
-- Entry: old_hillsbrad.h:35 names THRALL_ENTRY = 17876 AND
-- instance_old_hillsbrad.cpp:108 cases it in OnCreatureCreate (ThrallGUID
-- capture) with the DATA_THRALL GetGuidData leg — the name-to-entry tie
-- is C++-verified (ramstein strength); creature_template ScriptName
-- binding stays DB-side.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 23 OnReset. Melee is engine-driven in Go (creature combat
-- tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (npc_thrall_old_hillsbrad's self-contained in-combat legs,
-- legoso combat-rotation-only precedent; the escort machine is ported
-- below, tick 401, on the legoso ev6/StartEscort pattern):
-- - Engage (C++ JustEngagedWith): Talk SAY_TH_RANDOM_AGGRO (13) +
--   the dismount-if-mounted leg (creature:IsMounted/Dismount bridge;
--   the SetSpeedRate(MOVE_RUN, SPEED_RUN) and HadMount-latch legs have
--   no bearer — the escort/Reset machine stays unported, documented
--   below).
-- - LowHp latch (C++ UpdateAI, after EscortAI::UpdateAI + UpdateVictim):
--   !LowHp && HealthBelowPct(20) -> Talk SAY_TH_RANDOM_LOW_HP (11),
--   latched until Reset clears it (C++ Initialize() leg). Modeled as a 1s
--   CreateLuaEvent pump (sironas convention); the C++ checks every AI
--   tick while in combat, so the 1s granularity is an approximation.
-- - KilledUnit (event 3, wired — nalorakk DISCOVERY holds): Talk
--   SAY_TH_RANDOM_KILL (14); no TYPEID gate in C++ (shade_of_aran
--   precedent).
-- - Death (C++ JustDied): the instance->SetData(TYPE_THRALL_EVENT = 2,
--   FAIL) leg is instance bookkeeping (instance-script model blocked,
--   standing); Talk SAY_TH_RANDOM_DIE (12) unless killer == me (C++ skips
--   the yell when Thrall kills himself — player too far or escort end).
-- - Reset (event 23): clears the LowHp latch (C++ Initialize() leg) +
--   the HadMount machine (C++ Reset, old_hillsbrad.cpp:381-398): if
--   HadMount, DoMount (Mount 18223 + MOVE_RUN 1.6); if NOT escorting,
--   DoUnmount + HadMount=false + virtual-item slots 0 + display
--   unequipped 17292; if escorting, Talk SAY_TH_LEAVE_COMBAT (15).
--   PORTED tick 401; the HadMount latch is set in onEnterCombat below.
-- Escort machine (C++ EscortAI::Start / StartWP / WaypointReached,
-- old_hillsbrad.cpp:220-377, 399-404, 471-540) — PORTED tick 401 on the
-- legoso escort pattern (npc_demolitionist_legoso.lua): Start =
-- StartEscort(player) + SetWalk(!run) + SetNPCFlags(0) (C++ Start clears
-- UNIT_NPC_FLAGS) + MoveWaypoint(); StartWP = RemoveFlag(82, 1) (gossip
-- flag off) + SetEscortPaused(false); WaypointReached = ev6 handler
-- reading creature:GetCurrentWaypointId() (the DB point, legoso
-- precedent), guarded on IsEscorted() (C++ MovementInform gates on
-- STATE_ESCORT_ESCORTING; the per-case arms need no escort player
-- except case 106, which nil-guards it). SetRun(on) rides SetWalk(!on):
-- Go's per-node DB move_type still wins over motion.Walking
-- (creatureSplineVelocity ORs node walk with motion.Walking), so a DB
-- node marked WALK stays walk where C++ SetRun would force-run it —
-- a Go-model delta, DB-authored. SetMaxPlayerDistance(100) is the C++
-- default (DEFAULT_MAX_PLAYER_DISTANCE) and SetDespawnAtEnd(false) /
-- SetDespawnAtFar(false) are vacuous in Go (no despawn-at-end/far
-- model; the far check is disabled by the gossip leg itself), so all
-- three are documented, not wired. Combat: Go chase supersedes the
-- waypoint generator while engaged and evade returns home (the escort
-- leg of atEngageSetHomePosition is live); C++ returns to the
-- pre-combat point via POINT_LAST_POINT then resumes — the Go resume
-- continues from the next node instead, documented delta. Path rows are
-- deployment-DB waypoint_data (creature_addon.path_id), like legoso.
-- Waypoint arms (all C++-exact coords/entries/durations):
--   8 SetRun(false) + summon armorer 18764 (2181.87, 112.46, 89.45,
--   0.26, OUT_OF_COMBAT 5s); 9 Talk(1) + SetEquipmentSlots(927, 2129,
--   unchanged); 10 SetDisplayId(18165); 11/71/84 SetRun(); 15/21/25
--   summon rifle 17820 / warden 17833 / 2x veteran 17860 (OUT_OF_COMBAT
--   5s); 29 Talk(2) + summon Skarloc 17862 (2036.48, 271.22, 63.43,
--   5.27, OUT_OF_COMBAT 30s); 30 pause + gossip flag + SetRun(false);
--   31 Talk(5) + DoMount + SetRun(); 37 summon 2x watchman 17814 +
--   sentry 17815; 59 summon mount 18798 (TIMED_DESPAWN 10s) + DoUnmount
--   + HadMount=false + SetRun(false); 60 PerformEmote(5) + pause +
--   gossip flag + thrallPart2 DONE + SetRun(); 64/81 SetRun(false); 68
--   summon barn protector 18093 / lookout 18094 / 2x guardsman 18092;
--   83 summon church protector 23179 / lookout 23177 / 2x guardsman
--   23175 (TIMED_OR_DEAD_DESPAWN 5s); 84 Talk(6) + SetRun(); 91
--   SetWalk(true) + SetRun(false); 93 summon inn protector 23180 /
--   lookout 23178 / 2x guardsman 23176; 94 Taretha (instanceData bind)
--   Talk(1, Thrall); 95 Talk(7) + thrallPart3 DONE + pause; 96 Talk(8);
--   97 Talk(9) + SetRun(); 98 no-op (C++ comment only); 106 Taretha
--   escort Start (run=true, Thrall's escort player) + KilledMonsterCredit
--   (20156) for every map player + summon Erozion 18723 (2646.47,
--   680.416, 55.38, 4.16, TIMED_DESPAWN 2min); 108 SetVisible(false) +
--   StopEscort() (Go StopEscort now halts the waypoint generator —
--   C++ end-of-path is MoveIdle + drop ESCORTING; Go paths repeat by
--   default, so the halt is the twin). SAY_TH_SKARLOC_TAUNT (3) and
--   SAY_TH_EVENT_COMPLETE (10) are defined but never fired in the C++
--   files — dead enum entries, nothing to port.
-- - Thrall JustSummoned: barn guards/protectors/lookouts, SKARLOC_MOUNT,
--   EROZION_ENTRY are exempt; every other summoned creature gets
--   AttackStart(me) (summoned:Attack(creature, true), wilda precedent).
--   PORTED tick 401 (ev19).
-- - Thrall gossip: OnGossipHello shows GOSSIP_ITEM_WALKING
--   (menu GOSSIP_ID_START 9568) when TYPE_BARREL_DIVERSION 1 == DONE and
--   !TYPE_THRALL_EVENT 2; GOSSIP_ITEM_SKARLOC1 ("Taretha cannot see you,
--   Thrall.", menu 9614) when TYPE_THRALL_PART1 3 == DONE and
--   !TYPE_THRALL_PART2 4; GOSSIP_ITEM_TARREN ("We're ready, Thrall.", menu
--   9597) when TYPE_THRALL_PART2 4 == DONE and !TYPE_THRALL_PART3 5.
--   OnGossipSelect: action 1 -> SetData(TYPE_THRALL_EVENT 2, IN_PROGRESS) +
--   SetData(TYPE_THRALL_PART1 3, IN_PROGRESS) + Talk(SAY_TH_START_EVENT_PART1
--   0) + Start(true, true, playerGUID) + SetMaxPlayerDistance(100.0f) +
--   SetDespawnAtEnd(false) + SetDespawnAtFar(false); action 2 ->
--   GOSSIP_ITEM_SKARLOC2 chain (menus 9579 / 9580); action 20 ->
--   SummonCreature(SKARLOC_MOUNT 18798, 2038.81, 270.26, 63.20, 5.41,
--   TIMED_DESPAWN, 12s) + SetData(TYPE_THRALL_PART2 4, IN_PROGRESS) +
--   Talk(SAY_TH_START_EVENT_PART2 4) + StartWP() (escort unpause); action 3
--   -> SetData(TYPE_THRALL_PART3 5, IN_PROGRESS) + StartWP().
-- - npc_erozion (gossip-only — PORTED below, entry EROZION_ENTRY 18723):
--   OnGossipHello offers "I need a pack of Incendiary Bombs."
--   (GOSSIP_ACTION_INFO_DEF+1) when instance TYPE_BARREL_DIVERSION
--   1 != DONE and the player lacks ITEM_ENTRY_BOMBS 25853, and "[PH]
--   Teleport please, i'm tired." (action +2) when QUEST_ENTRY_RETURN 10285
--   is complete (menu 9778); action +1 grants one Incendiary Bombs 25853
--   via player:AddItem (menu 9515), action +2 just closes. The hello
--   bombs-item gates (instance GetData, player HasItemCount) have no
--   bridge — kalecgos precedent — so the item is offered unconditionally;
--   the quest-status gate is live (player:GetQuestStatus, COMPLETE 1).
-- - npc_taretha (EscortAI): WaypointReached case 6 Talk(SAY_TA_FREE 0),
--   case 7 PerformEmote(EMOTE_ONESHOT_CHEER 4) — PORTED tick 401 (ev6,
--   IsEscorted guard, below). OnGossipHello shows GOSSIP_ITEM_EPOCH1
--   ("Strange wizard?", menu 9610) when TYPE_THRALL_PART3 5 == DONE and
--   TYPE_THRALL_PART4 6 == NOT_STARTED; OnGossipSelect action 1001 ->
--   EPOCH2 item + menu 9613 (pure menu nav), action 1002 ->
--   CloseGossipMenuFor + the instance/summon machine (SetData
--   TYPE_THRALL_PART4 6 IN_PROGRESS, SummonCreature ENTRY_EPOCH 18096
--   at 2639.13/698.55/65.43/4.59 TIMED_OR_DEAD_DESPAWN 2min if
--   DATA_EPOCH 9 GUID empty, Thrall StartWP). PORTED below (entry
--   TARETHA_ENTRY 18887: instance_old_hillsbrad.cpp:36): the hello
--   instance gate is documented-only (kalecgos precedent); the
--   action-1002 machine's SetData/summon legs are ported, and the Thrall
--   StartWP leg landed tick 401 with the escort machine.
-- - The C++ "/// @todo add his abilities'n-crap here" comment (Thrall
--   UpdateAI) and the commented-out UNIT_VIRTUAL_ITEM_INFO sets are ported
--   as-is above; the escort Talk lines (SAY_TH_START_EVENT_PART1 0 through
--   SAY_TH_EVENT_COMPLETE 10) now fire inside the ported escort /
--   gossip machine (SAY_TH_SKARLOC_TAUNT 3 and SAY_TH_EVENT_COMPLETE 10
--   excepted — dead enum entries, never fired in C++).
-- SD%Complete: 40 — quest support 10283/10284; SD%Complete: 60 flags the
-- missing pre-event spawns + uncoordinated Thrall-escort speech — the
-- escort machine itself is now ported; the pre-event spawns stay out
-- (DB-side spawn data, not in the C++ tree).

-- PORTED 2026-11-07 (tick 397): the Thrall gossip writers (hello gates
-- + select actions 1/2/20/3 writing instanceData.thrallEvent /
-- thrallPart1-3 + Talk(0)/(4) + SKARLOC_MOUNT summon), the JustDied
-- TYPE_THRALL_EVENT FAIL leg (20-fail cap), the Taretha action-1002
-- machine (thrallPart4 + Epoch summon iff DATA_EPOCH empty), and the
-- Erozyon bombs-item gates (instance barrelDiversion + GetItemCount)
-- — the "no instance GetData/SetData bridge" claims in the paragraphs
-- above are retracted for the slots this file now writes/reads via the
-- shared instanceData table.
-- PORTED 2026-11-07 (tick 401): the Thrall escort machine (ev6
-- WaypointReached all cases + Start/StartWP in the gossip select +
-- Reset HadMount legs + ev19 JustSummoned), the Taretha escort
-- (ev6 cases 6/7 + Start at Thrall waypoint 106 + Thrall StartWP in
-- her action 1002), and the Go StopEscort waypoint-halt twin
-- (engine/world/lua_creature_events.go — ended escorts no longer loop
-- the path). The "no escort bridge" claims above are retracted; the
-- boss-file GetData readers (skarloc / epoch-hunter / drake) were
-- already ported tick 399/400. FIX tick 401: the tick-397 SKARLOC_MOUNT
-- summon used type 1 (TIMED_OR_CORPSE_DESPAWN) for C++ TIMED_DESPAWN
-- 12s — corrected to type 2 (TempSummonType 0/1/2/3 timed family,
-- summonTempCreature).
-- The barrel counter landed 2026-11-07 (go_barrel GO gossip handler,
-- boss_lieutenant_drake.lua, GO entry 182589) — barrelDiversion /
-- barrelCount are now live slots.

local ENTRY = 17876

local SAY_TH_START_EVENT_PART1 = 0
local SAY_TH_ARMORY = 1
local SAY_TH_SKARLOC_MEET = 2
local SAY_TH_START_EVENT_PART2 = 4
local SAY_TH_MOUNTS_UP = 5
local SAY_TH_CHURCH_END = 6
local SAY_TH_MEET_TARETHA = 7
local SAY_TH_EPOCH_WONDER = 8
local SAY_TH_EPOCH_KILL_TARETHA = 9
local SAY_TH_LEAVE_COMBAT = 15
local SAY_TA_FREE = 0
local SAY_TA_ESCAPED = 1
local SAY_TH_RANDOM_LOW_HP = 11
local SAY_TH_RANDOM_DIE    = 12
local SAY_TH_RANDOM_AGGRO  = 13
local SAY_TH_RANDOM_KILL   = 14

-- old_hillsbrad.cpp:131-180 (ThrallOldHillsbrad enum + speed defines).
local THRALL_MODEL_UNEQUIPPED = 17292
local THRALL_MODEL_EQUIPPED = 18165
local THRALL_WEAPON_ITEM = 927
local THRALL_SHIELD_ITEM = 2129
local SKARLOC_MOUNT_MODEL = 18223
local ENTRY_ARMORER = 18764
local ENTRY_SCARLOC = 17862
local NPC_RIFLE = 17820
local NPC_WARDEN = 17833
local NPC_VETERAN = 17860
local NPC_WATCHMAN = 17814
local NPC_SENTRY = 17815
local NPC_BARN_GUARDSMAN = 18092
local NPC_BARN_PROTECTOR = 18093
local NPC_BARN_LOOKOUT = 18094
local NPC_CHURCH_GUARDSMAN = 23175
local NPC_CHURCH_PROTECTOR = 23179
local NPC_CHURCH_LOOKOUT = 23177
local NPC_INN_GUARDSMAN = 23176
local NPC_INN_PROTECTOR = 23180
local NPC_INN_LOOKOUT = 23178
local ENTRY_EPOCH = 18096
local SKARLOC_MOUNT = 18798
local EROZION_ENTRY = 18723
local KILLED_MONSTER_CREDIT = 20156
local EMOTE_ONESHOT_CHEER = 4
local EMOTE_ONESHOT_EXCLAMATION = 5
local UNIT_NPC_FLAGS_FIELD = 82
local UNIT_NPC_FLAG_GOSSIP = 1
local SPEED_RUN = 1.0
local SPEED_MOUNT = 1.6
-- TempSummonType (TemporarySummon.h): 0 TIMED_OR_DEAD_DESPAWN,
-- 1 TIMED_OR_CORPSE_DESPAWN, 2 TIMED_DESPAWN,
-- 3 TIMED_DESPAWN_OUT_OF_COMBAT — the timed family (0-3) arms the
-- despawn timer at spawn (summonTempCreature).
local SUMMON_TIMED_OR_DEAD = 0
local SUMMON_TIMED_DESPAWN = 2
local SUMMON_TIMED_OUT_OF_COMBAT = 3

-- EncounterState (InstanceScript.h:68-76) for the instanceData
-- TYPE_* slots (old_hillsbrad.h:28-33): TYPE_BARREL_DIVERSION 1 /
-- TYPE_THRALL_EVENT 2 / TYPE_THRALL_PART1 3 / PART2 4 / PART3 5 /
-- PART4 6. The slots live in the shared instanceData table
-- (instance_old_hillsbrad.lua binds); nil reads as NOT_STARTED.
local STATE_NOT_STARTED = 0
local STATE_IN_PROGRESS = 1
local STATE_FAIL = 2
local STATE_DONE = 3

-- the_black_morass.lua instanceDataOf precedent.
local function instanceDataOf(creature)
    local mapObj = creature:GetMap()
    if not mapObj then
        return nil
    end
    return mapObj:GetInstanceData()
end

local lowHp = {}
local lowHpPump = {}
local hadMount = {}

local function cancelPump(guid)
    local id = lowHpPump[guid]
    if id then
        RemoveEventById(id)
        lowHpPump[guid] = nil
    end
end

-- C++ UpdateAI LowHp leg: !LowHp && HealthBelowPct(20) -> Talk(11),
-- latched until Reset (C++ Initialize() leg). 1s per-GUID pump started on
-- engage (sironas convention); the C++ checks every AI tick while in
-- combat, so 1s granularity is an approximation, documented above.
local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelPump(guid)
    lowHp[guid] = false
    creature:Talk(SAY_TH_RANDOM_AGGRO)
    if creature:IsMounted() then
        -- C++ DoUnmount: Dismount + SetSpeedRate(MOVE_RUN, SPEED_RUN);
        -- SPEED_RUN is 1.0f (old_hillsbrad.cpp:186). HadMount latches
        -- for Reset's re-mount leg (old_hillsbrad.cpp:383-385).
        hadMount[guid] = true
        creature:Dismount()
        creature:SetSpeedRate(1, SPEED_RUN)
    end
    lowHpPump[guid] = CreateLuaEvent(function()
        if not lowHp[guid] then
            local maxHealth = creature:GetMaxHealth()
            if maxHealth > 0
                    and creature:GetHealth() < math.floor(maxHealth * 20 / 100) then
                lowHp[guid] = true
                creature:Talk(SAY_TH_RANDOM_LOW_HP)
            end
        end
    end, 1000, 0)
end

local function onLeaveCombat(event, creature)
    local guid = creature:GetGUIDLow()
    cancelPump(guid)
    lowHp[guid] = false
end

local function onReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelPump(guid)
    lowHp[guid] = false
    -- C++ Reset (old_hillsbrad.cpp:381-398), in order: Initialize()
    -- (LowHp latch, above); if HadMount, DoMount (Mount + MOVE_RUN
    -- SPEED_MOUNT); if NOT escorting, DoUnmount + HadMount=false +
    -- virtual-item slots 0 + display unequipped; if escorting,
    -- Talk(SAY_TH_LEAVE_COMBAT). IsEscorted reads the armed escort
    -- player (HasEscortState twin, legoso precedent).
    if hadMount[guid] then
        creature:Mount(SKARLOC_MOUNT_MODEL)
        creature:SetSpeedRate(1, SPEED_MOUNT)
    end
    if not creature:IsEscorted() then
        creature:Dismount()
        creature:SetSpeedRate(1, SPEED_RUN)
        hadMount[guid] = false
        creature:SetEquipmentSlots(0, 0, 0xFFFFFFFF)
        creature:SetDisplayId(THRALL_MODEL_UNEQUIPPED)
    else
        creature:Talk(SAY_TH_LEAVE_COMBAT)
    end
end

local function onDied(event, creature, killer)
    local guid = creature:GetGUIDLow()
    cancelPump(guid)
    lowHp[guid] = false
    hadMount[guid] = false
    -- C++ JustDied skips the yell when Thrall kills himself (player too
    -- far or escort end): killer == me.
    if not killer or killer:GetGUIDLow() ~= guid then
        creature:Talk(SAY_TH_RANDOM_DIE)
    end
    -- C++ JustDied also sends instance->SetData(TYPE_THRALL_EVENT 2,
    -- FAIL) (old_hillsbrad.cpp:193-196): the FAIL leg lives in
    -- instance_old_hillsbrad.cpp:156-177 — fail-count-capped at 20, then
    -- FAIL. PORTED 2026-11-07 via the shared instanceData table
    -- (instanceDataOf below); the escort machine itself stays out.
    local data = instanceDataOf(creature)
    if data then
        local fails = (data.thrallFailCount or 0) + 1
        data.thrallFailCount = fails
        if fails <= 20 then
            data.thrallEvent = STATE_NOT_STARTED
            data.thrallPart1 = STATE_NOT_STARTED
            data.thrallPart2 = STATE_NOT_STARTED
            data.thrallPart3 = STATE_NOT_STARTED
            data.thrallPart4 = STATE_NOT_STARTED
        else
            data.thrallEvent = STATE_FAIL
            data.thrallPart1 = STATE_FAIL
            data.thrallPart2 = STATE_FAIL
            data.thrallPart3 = STATE_FAIL
            data.thrallPart4 = STATE_FAIL
        end
    end
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 3, function(event, creature, victim)
    creature:Talk(SAY_TH_RANDOM_KILL)
end)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)

-- C++ JustSummoned (old_hillsbrad.cpp:430-445): barn guards/protectors/
-- lookouts, SKARLOC_MOUNT and EROZION_ENTRY are exempt; every other
-- summoned creature gets AttackStart(me) (wilda ev19 precedent).
local JUST_SUMMONED_EXEMPT = {
    [NPC_BARN_GUARDSMAN] = true,
    [NPC_BARN_PROTECTOR] = true,
    [NPC_BARN_LOOKOUT] = true,
    [SKARLOC_MOUNT] = true,
    [EROZION_ENTRY] = true,
}

local function onJustSummoned(event, creature, summoned)
    if summoned and not JUST_SUMMONED_EXEMPT[summoned:GetEntry()] then
        summoned:Attack(creature, true)
    end
end

RegisterCreatureEvent(ENTRY, 19, onJustSummoned)

-- C++ WaypointReached (old_hillsbrad.cpp:220-377) — ev6 handler,
-- legoso pattern: the waypoint id is the DB point
-- (creature:GetCurrentWaypointId()), not the event's node index.
-- Guarded on IsEscorted() (C++ MovementInform gates on
-- STATE_ESCORT_ESCORTING). SetRun(on) rides SetWalk(!on) (header
-- delta: DB WALK nodes stay walk). Summon helper: each list row is
-- {entry, x, y, z, o}.
local function summonGroup(creature, list, stype, ms)
    for _, s in ipairs(list) do
        creature:SummonCreature(s[1], s[2], s[3], s[4], s[5], stype, ms)
    end
end

local DEFENDERS_15 = {
    { NPC_RIFLE, 2200.28, 137.37, 87.93, 5.07 },
    { NPC_WARDEN, 2197.44, 131.83, 87.93, 0.78 },
    { NPC_VETERAN, 2203.62, 135.40, 87.93, 3.70 },
    { NPC_VETERAN, 2200.75, 130.13, 87.93, 1.48 },
}
local DEFENDERS_21 = {
    { NPC_RIFLE, 2135.80, 154.01, 67.45, 4.98 },
    { NPC_WARDEN, 2144.36, 151.87, 67.74, 4.46 },
    { NPC_VETERAN, 2142.12, 154.41, 67.12, 4.56 },
    { NPC_VETERAN, 2138.08, 155.38, 67.24, 4.60 },
}
local DEFENDERS_25 = {
    { NPC_RIFLE, 2102.98, 192.17, 65.24, 6.02 },
    { NPC_WARDEN, 2108.48, 198.75, 65.18, 5.15 },
    { NPC_VETERAN, 2106.11, 197.29, 65.18, 5.63 },
    { NPC_VETERAN, 2104.18, 194.82, 65.18, 5.75 },
}
local BARN_SQUAD = {
    { NPC_BARN_PROTECTOR, 2500.22, 692.60, 55.50, 2.84 },
    { NPC_BARN_LOOKOUT, 2500.13, 696.55, 55.51, 3.38 },
    { NPC_BARN_GUARDSMAN, 2500.55, 693.64, 55.50, 3.14 },
    { NPC_BARN_GUARDSMAN, 2500.94, 695.81, 55.50, 3.14 },
}
local CHURCH_SQUAD = {
    { NPC_CHURCH_PROTECTOR, 2627.33, 646.82, 56.03, 4.28 },
    { NPC_CHURCH_LOOKOUT, 2624.14, 648.03, 56.03, 4.50 },
    { NPC_CHURCH_GUARDSMAN, 2625.32, 649.60, 56.03, 4.38 },
    { NPC_CHURCH_GUARDSMAN, 2627.22, 649.00, 56.03, 4.34 },
}
local INN_SQUAD = {
    { NPC_INN_PROTECTOR, 2652.71, 660.31, 61.93, 1.67 },
    { NPC_INN_LOOKOUT, 2648.96, 662.59, 61.93, 0.79 },
    { NPC_INN_GUARDSMAN, 2657.36, 662.34, 61.93, 2.68 },
    { NPC_INN_GUARDSMAN, 2656.39, 659.77, 2.61 },
}

local function thrallReachWP(_, creature)
    if not creature:IsEscorted() then
        return
    end
    local guid = creature:GetGUIDLow()
    local wp = creature:GetCurrentWaypointId()
    if wp == 8 then
        creature:SetWalk(true)
        creature:SummonCreature(ENTRY_ARMORER, 2181.87, 112.46, 89.45, 0.26,
            SUMMON_TIMED_OUT_OF_COMBAT, 5000)
    elseif wp == 9 then
        creature:Talk(SAY_TH_ARMORY)
        creature:SetEquipmentSlots(THRALL_WEAPON_ITEM, THRALL_SHIELD_ITEM, 0xFFFFFFFF)
    elseif wp == 10 then
        creature:SetDisplayId(THRALL_MODEL_EQUIPPED)
    elseif wp == 11 or wp == 71 or wp == 84 then
        if wp == 84 then
            creature:Talk(SAY_TH_CHURCH_END)
        end
        creature:SetWalk(false)
    elseif wp == 15 then
        summonGroup(creature, DEFENDERS_15, SUMMON_TIMED_OUT_OF_COMBAT, 5000)
    elseif wp == 21 then
        summonGroup(creature, DEFENDERS_21, SUMMON_TIMED_OUT_OF_COMBAT, 5000)
    elseif wp == 25 then
        summonGroup(creature, DEFENDERS_25, SUMMON_TIMED_OUT_OF_COMBAT, 5000)
    elseif wp == 29 then
        creature:Talk(SAY_TH_SKARLOC_MEET)
        creature:SummonCreature(ENTRY_SCARLOC, 2036.48, 271.22, 63.43, 5.27,
            SUMMON_TIMED_OUT_OF_COMBAT, 30000)
    elseif wp == 30 then
        creature:SetEscortPaused(true)
        creature:SetFlag(UNIT_NPC_FLAGS_FIELD, UNIT_NPC_FLAG_GOSSIP)
        creature:SetWalk(true)
    elseif wp == 31 then
        creature:Talk(SAY_TH_MOUNTS_UP)
        creature:Mount(SKARLOC_MOUNT_MODEL)
        creature:SetSpeedRate(1, SPEED_MOUNT)
        creature:SetWalk(false)
    elseif wp == 37 then
        creature:SummonCreature(NPC_WATCHMAN, 2124.26, 522.16, 56.87, 3.99,
            SUMMON_TIMED_OUT_OF_COMBAT, 5000)
        creature:SummonCreature(NPC_WATCHMAN, 2121.69, 525.37, 57.11, 4.01,
            SUMMON_TIMED_OUT_OF_COMBAT, 5000)
        creature:SummonCreature(NPC_SENTRY, 2124.65, 524.55, 56.63, 3.98,
            SUMMON_TIMED_OUT_OF_COMBAT, 5000)
    elseif wp == 59 then
        creature:SummonCreature(SKARLOC_MOUNT, 2488.64, 625.77, 58.26, 4.71,
            SUMMON_TIMED_DESPAWN, 10000)
        creature:Dismount()
        creature:SetSpeedRate(1, SPEED_RUN)
        hadMount[guid] = false
        creature:SetWalk(true)
    elseif wp == 60 then
        creature:PerformEmote(EMOTE_ONESHOT_EXCLAMATION)
        creature:SetEscortPaused(true)
        creature:SetFlag(UNIT_NPC_FLAGS_FIELD, UNIT_NPC_FLAG_GOSSIP)
        local data = instanceDataOf(creature)
        if data then
            data.thrallPart2 = STATE_DONE
        end
        creature:SetWalk(false)
    elseif wp == 64 or wp == 81 then
        creature:SetWalk(true)
    elseif wp == 68 then
        summonGroup(creature, BARN_SQUAD, SUMMON_TIMED_OUT_OF_COMBAT, 5000)
    elseif wp == 83 then
        summonGroup(creature, CHURCH_SQUAD, SUMMON_TIMED_OR_DEAD, 5000)
    elseif wp == 91 then
        creature:SetWalk(true)
    elseif wp == 93 then
        summonGroup(creature, INN_SQUAD, SUMMON_TIMED_OUT_OF_COMBAT, 5000)
    elseif wp == 94 then
        local data = instanceDataOf(creature)
        local taretha = data and data.taretha or nil
        if taretha then
            taretha:Talk(SAY_TA_ESCAPED, creature)
        end
    elseif wp == 95 then
        creature:Talk(SAY_TH_MEET_TARETHA)
        local data = instanceDataOf(creature)
        if data then
            data.thrallPart3 = STATE_DONE
        end
        creature:SetEscortPaused(true)
    elseif wp == 96 then
        creature:Talk(SAY_TH_EPOCH_WONDER)
    elseif wp == 97 then
        creature:Talk(SAY_TH_EPOCH_KILL_TARETHA)
        creature:SetWalk(false)
    elseif wp == 106 then
        local data = instanceDataOf(creature)
        local taretha = data and data.taretha or nil
        local player = creature:GetEscortPlayer()
        if taretha and player then
            taretha:StartEscort(player)
            taretha:SetWalk(false)
            taretha:SetNPCFlags(0)
            taretha:MoveWaypoint()
        end
        local mapObj = creature:GetMap()
        if mapObj then
            local players = mapObj:GetPlayers()
            if players then
                for _, p in ipairs(players) do
                    p:KilledMonsterCredit(KILLED_MONSTER_CREDIT)
                end
            end
        end
        creature:SummonCreature(EROZION_ENTRY, 2646.47, 680.416, 55.38, 4.16,
            SUMMON_TIMED_DESPAWN, 120000)
    elseif wp == 108 then
        creature:SetVisible(false)
        creature:StopEscort()
    end
end

RegisterCreatureEvent(ENTRY, 6, thrallReachWP)

-- npc_taretha gossip (old_hillsbrad.cpp:597-629) — entry TARETHA_ENTRY
-- 18887 (instance_old_hillsbrad.cpp:36).
local TARETHA_ENTRY = 18887
local GOSSIP_ICON_CHAT = 0
local GOSSIP_SENDER_MAIN = 1
local GOSSIP_ACTION_INFO_DEF = 1000
local GOSSIP_ITEM_EPOCH1 = "Strange wizard?"
local GOSSIP_ITEM_EPOCH2 = "We'll get you out. Taretha. Don't worry. I doubt the wizard would wander too far away."
local GOSSIP_MENU_EPOCH1 = 9610
local GOSSIP_MENU_EPOCH2 = 9613

-- C++ OnGossipHello (old_hillsbrad.cpp:622-629): EPOCH1 item + menu
-- 9610 only when instance TYPE_THRALL_PART3 == DONE and
-- TYPE_THRALL_PART4 == NOT_STARTED. Instance gates have no bridge —
-- kalecgos precedent — so the item is offered unconditionally and
-- the gate is documented.
local function tarethaGossipHello(event, player, creature)
    player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_ITEM_EPOCH1,
        GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 1)
    player:GossipSendMenu(GOSSIP_MENU_EPOCH1, creature)
end

-- C++ OnGossipSelect (old_hillsbrad.cpp:597-620): action 1001 ->
-- AddGossipItemFor EPOCH2 + SendGossipMenuFor 9613 (pure menu nav);
-- action 1002 -> CloseGossipMenuFor + instance/summon machine
-- (SetData TYPE_THRALL_PART4 IN_PROGRESS, SummonCreature ENTRY_EPOCH,
-- Thrall StartWP) — PORTED below; the Thrall StartWP leg landed
-- tick 401 with the escort machine.
local function tarethaGossipSelect(event, player, creature, sender, action)
    if action == GOSSIP_ACTION_INFO_DEF + 1 then
        player:GossipClearMenu()
        player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_ITEM_EPOCH2,
            GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 2)
        player:GossipSendMenu(GOSSIP_MENU_EPOCH2, creature)
    elseif action == GOSSIP_ACTION_INFO_DEF + 2 then
        player:GossipComplete()
        -- C++ OnGossipSelect action 1002 (old_hillsbrad.cpp:605-620):
        -- CloseGossipMenuFor + SetData(TYPE_THRALL_PART4 6, IN_PROGRESS)
        -- + SummonCreature(ENTRY_EPOCH 18096, 2639.13, 698.55, 65.43,
        -- 4.59, TIMED_OR_DEAD_DESPAWN, 2min) iff DATA_EPOCH 9 GUID empty
        -- + Thrall StartWP (RemoveFlag gossip + SetEscortPaused(false)).
        -- The epoch gate reads the instance_old_hillsbrad.lua bind
        -- (nil = never summoned); Thrall resolves via the same bind.
        local data = instanceDataOf(creature)
        if data then
            data.thrallPart4 = STATE_IN_PROGRESS
            if not data.epoch then
                creature:SummonCreature(ENTRY_EPOCH, 2639.13, 698.55, 65.43, 4.59,
                    SUMMON_TIMED_OR_DEAD, 120000)
            end
            local thrall = data.thrall
            if thrall then
                thrall:RemoveFlag(UNIT_NPC_FLAGS_FIELD, UNIT_NPC_FLAG_GOSSIP)
                thrall:SetEscortPaused(false)
            end
        end
    end
end

RegisterCreatureGossipEvent(TARETHA_ENTRY, 1, tarethaGossipHello)
RegisterCreatureGossipEvent(TARETHA_ENTRY, 2, tarethaGossipSelect)

-- C++ WaypointReached (old_hillsbrad.cpp:576-585): case 6 Talk(0),
-- case 7 PerformEmote(EMOTE_ONESHOT_CHEER 4). Ev6, IsEscorted guard
-- (legoso pattern); her escort starts at Thrall waypoint 106 above.
local function tarethaReachWP(_, creature)
    if not creature:IsEscorted() then
        return
    end
    local wp = creature:GetCurrentWaypointId()
    if wp == 6 then
        creature:Talk(SAY_TA_FREE)
    elseif wp == 7 then
        creature:PerformEmote(EMOTE_ONESHOT_CHEER)
    end
end

RegisterCreatureEvent(TARETHA_ENTRY, 6, tarethaReachWP)

-- npc_erozion gossip (old_hillsbrad.cpp:55-98) — entry EROZION_ENTRY
-- 18723 (old_hillsbrad.cpp:173; constant at the top of this file).

local ITEM_ENTRY_BOMBS = 25853
local QUEST_ENTRY_RETURN = 10285
local QUEST_STATUS_COMPLETE = 1
local GOSSIP_MENU_EROZION = 9778
local GOSSIP_MENU_BOMBS = 9515
local GOSSIP_ITEM_BOMBS = "I need a pack of Incendiary Bombs."
local GOSSIP_ITEM_TELEPORT = "[PH] Teleport please, i'm tired."

-- C++ OnGossipHello (old_hillsbrad.cpp:85-97): quest menu first when
-- the creature is a quest giver (player:GossipAddQuests twin,
-- npc_thrall_warchief.lua precedent); the bombs item only when
-- instance TYPE_BARREL_DIVERSION 1 != DONE and the player lacks
-- ITEM_ENTRY_BOMBS 25853, and the teleport item only when
-- QUEST_ENTRY_RETURN 10285 is COMPLETE. Both bombs-item gates have no
-- bridge (no instance GetData, no player HasItemCount) — kalecgos
-- precedent — so the bombs item is offered unconditionally and the
-- gate is documented in the header; the quest-status gate is live
-- (player:GetQuestStatus, COMPLETE 1 per lua_player.go:3980-3983).
local function erozionGossipHello(event, player, creature)
    if creature:IsQuestGiver() then
        player:GossipAddQuests(creature)
    end
    -- C++ OnGossipHello (old_hillsbrad.cpp:85-97): the bombs item only
    -- when instance TYPE_BARREL_DIVERSION 1 != DONE and the player lacks
    -- ITEM_ENTRY_BOMBS 25853. The instance gate is now live
    -- (instanceData.barrelDiversion, nil reads as NOT_STARTED) and the
    -- item gate reads player:GetItemCount (lua_player.go:2146) — the
    -- header's "no bridge" note is retracted 2026-11-07. The barrel
    -- counter now has its writer (go_barrel GO gossip handler,
    -- boss_lieutenant_drake.lua, GO entry 182589) — the gate reads the
    -- live barrelDiversion slot.
    local data = instanceDataOf(creature)
    if (not data or (data.barrelDiversion or STATE_NOT_STARTED) ~= STATE_DONE)
        and player:GetItemCount(ITEM_ENTRY_BOMBS) == 0 then
        player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_ITEM_BOMBS,
            GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 1)
    end
    if player:GetQuestStatus(QUEST_ENTRY_RETURN) == QUEST_STATUS_COMPLETE then
        player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_ITEM_TELEPORT,
            GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 2)
    end
    player:GossipSendMenu(GOSSIP_MENU_EROZION, creature)
end

-- C++ OnGossipSelect (old_hillsbrad.cpp:66-83): action 1001 grants one
-- Incendiary Bombs 25853 via CanStoreNewItem/StoreNewItem
-- (player:AddItem twin, lua_player.go:2196-2206 — returns nil on a full
-- inventory or unknown template, and the C++ sends menu 9515 either
-- way, so the result is ignored) and sends menu 9515; action 1002
-- just closes the menu.
local function erozionGossipSelect(event, player, creature, sender, action)
    player:GossipClearMenu()
    if action == GOSSIP_ACTION_INFO_DEF + 1 then
        player:AddItem(ITEM_ENTRY_BOMBS, 1)
        player:GossipSendMenu(GOSSIP_MENU_BOMBS, creature)
    elseif action == GOSSIP_ACTION_INFO_DEF + 2 then
        player:GossipComplete()
    end
end

RegisterCreatureGossipEvent(EROZION_ENTRY, 1, erozionGossipHello)
RegisterCreatureGossipEvent(EROZION_ENTRY, 2, erozionGossipSelect)

-- npc_thrall_old_hillsbrad gossip (old_hillsbrad.cpp:176-192,
-- 471-540) — PORTED 2026-11-07: the hello gates read the shared
-- instanceData TYPE_* slots (the header's "no instance GetData
-- bridge" is retracted for the slots this file now writes), and the
-- select writers land the SetData legs. The escort legs (Start/StartWP)
-- landed tick 401 (StartEscort + MoveWaypoint / RemoveFlag + 
-- SetEscortPaused(false)); SetMaxPlayerDistance/SetDespawnAtEnd/
-- SetDespawnAtFar are vacuous in Go (header). At most one hello branch matches (gates are mutually
-- exclusive: gate 1 needs !thrallEvent, gate 2 needs part1 DONE +
-- !part2, gate 3 needs part2 DONE + !part3).
local GOSSIP_ID_START = 9568
local GOSSIP_ID_SKARLOC1 = 9614
local GOSSIP_ID_SKARLOC2 = 9579
local GOSSIP_ID_SKARLOC3 = 9580
local GOSSIP_ID_TARREN = 9597
local GOSSIP_ITEM_WALKING = "We are ready to get you out of here, Thrall. Let's go!"
local GOSSIP_ITEM_SKARLOC1 = "Taretha cannot see you, Thrall."
local GOSSIP_ITEM_SKARLOC2 = "The situation is rather complicated, Thrall. It would be best for you to head into the mountains now, before more of Blackmoore's men show up. We'll make sure Taretha is safe."
local GOSSIP_ITEM_TARREN = "We're ready, Thrall."

local function thrallGossipHello(event, player, creature)
    if creature:IsQuestGiver() then
        player:GossipAddQuests(creature)
    end
    local data = instanceDataOf(creature)
    local barrel = data and (data.barrelDiversion or STATE_NOT_STARTED) or STATE_NOT_STARTED
    local thrallEvent = data and (data.thrallEvent or STATE_NOT_STARTED) or STATE_NOT_STARTED
    local part1 = data and (data.thrallPart1 or STATE_NOT_STARTED) or STATE_NOT_STARTED
    local part2 = data and (data.thrallPart2 or STATE_NOT_STARTED) or STATE_NOT_STARTED
    local part3 = data and (data.thrallPart3 or STATE_NOT_STARTED) or STATE_NOT_STARTED
    if barrel == STATE_DONE and thrallEvent == STATE_NOT_STARTED then
        player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_ITEM_WALKING,
            GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 1)
        player:GossipSendMenu(GOSSIP_ID_START, creature)
    elseif part1 == STATE_DONE and part2 == STATE_NOT_STARTED then
        player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_ITEM_SKARLOC1,
            GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 2)
        player:GossipSendMenu(GOSSIP_ID_SKARLOC1, creature)
    elseif part2 == STATE_DONE and part3 == STATE_NOT_STARTED then
        player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_ITEM_TARREN,
            GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 3)
        player:GossipSendMenu(GOSSIP_ID_TARREN, creature)
    end
end

local function thrallGossipSelect(event, player, creature, sender, action)
    local data = instanceDataOf(creature)
    if action == GOSSIP_ACTION_INFO_DEF + 1 then
        player:GossipComplete()
        if data then
            data.thrallEvent = STATE_IN_PROGRESS
            data.thrallPart1 = STATE_IN_PROGRESS
        end
        creature:Talk(SAY_TH_START_EVENT_PART1)
        -- C++ Start(true, true, playerGUID) (old_hillsbrad.cpp:490-496):
        -- run=true; SetMaxPlayerDistance(100) is the C++ default and
        -- SetDespawnAtEnd(false)/SetDespawnAtFar(false) are vacuous in
        -- Go (header). Start clears UNIT_NPC_FLAGS (SetNPCFlags(0)).
        creature:StartEscort(player)
        creature:SetWalk(false)
        creature:SetNPCFlags(0)
        creature:MoveWaypoint()
    elseif action == GOSSIP_ACTION_INFO_DEF + 2 then
        player:GossipClearMenu()
        player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_ITEM_SKARLOC2,
            GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 20)
        player:GossipSendMenu(GOSSIP_ID_SKARLOC2, creature)
    elseif action == GOSSIP_ACTION_INFO_DEF + 20 then
        player:GossipSendMenu(GOSSIP_ID_SKARLOC3, creature)
        creature:SummonCreature(SKARLOC_MOUNT, 2038.81, 270.26, 63.20, 5.41,
            SUMMON_TIMED_DESPAWN, 12000)
        if data then
            data.thrallPart2 = STATE_IN_PROGRESS
        end
        creature:Talk(SAY_TH_START_EVENT_PART2)
        -- C++ StartWP() (old_hillsbrad.cpp:399-404): RemoveFlag gossip
        -- + SetEscortPaused(false).
        creature:RemoveFlag(UNIT_NPC_FLAGS_FIELD, UNIT_NPC_FLAG_GOSSIP)
        creature:SetEscortPaused(false)
    elseif action == GOSSIP_ACTION_INFO_DEF + 3 then
        player:GossipComplete()
        if data then
            data.thrallPart3 = STATE_IN_PROGRESS
        end
        creature:RemoveFlag(UNIT_NPC_FLAGS_FIELD, UNIT_NPC_FLAG_GOSSIP)
        creature:SetEscortPaused(false)
    end
end

RegisterCreatureGossipEvent(ENTRY, 1, thrallGossipHello)
RegisterCreatureGossipEvent(ENTRY, 2, thrallGossipSelect)
