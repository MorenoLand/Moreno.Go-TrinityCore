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
-- legoso combat-rotation-only precedent; the entire escort machine below):
-- - Engage (C++ JustEngagedWith): Talk SAY_TH_RANDOM_AGGRO (13); the
--   dismount-if-mounted leg has no mount bridge — documented below.
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
-- - Reset (event 23): clears the LowHp latch (C++ Initialize() leg); the
--   re-mount-if-HadMount, virtual-item-slot clear, and
--   SetDisplayId(THRALL_MODEL_UNEQUIPPED 17292) legs have no mount /
--   display bridges; the Talk SAY_TH_LEAVE_COMBAT (15) leg is conditioned
--   on HasEscortState(STATE_ESCORT_ESCORTING) — unmodeled escort state.
-- Unmodeled (documented-only, no bridges):
-- - The entire EscortAI waypoint machine (npc_thrall_old_hillsbradAI
--   WaypointReached): case 8 SetRun(false) + SummonCreature(ENTRY_ARMORER
--   18764, 2181.87, 112.46, 89.45, 0.26, TIMED_DESPAWN_OUT_OF_COMBAT, 5s);
--   case 9 Talk(SAY_TH_ARMORY 1) + SetUInt32Value(UNIT_VIRTUAL_ITEM_SLOT_ID,
--   THRALL_WEAPON_ITEM 927) + (SLOT_ID+1, THRALL_SHIELD_ITEM 2129); case 10
--   SetDisplayId(THRALL_MODEL_EQUIPPED 18165); cases 11/71/84 SetRun();
--   case 15 summons rifle 17820 / warden 17833 / 2x veteran 17860; case 21
--   summons 17820 / 17833 / 2x 17860; case 25 summons 17820 / 17833 /
--   2x 17860; case 29 Talk(SAY_TH_SKARLOC_MEET 2) +
--   SummonCreature(ENTRY_SCARLOC 17862, 2036.48, 271.22, 63.43, 5.27,
--   TIMED_DESPAWN_OUT_OF_COMBAT, 30s); case 30 SetEscortPaused(true) +
--   UNIT_NPC_FLAG_GOSSIP flag; case 31 Talk(SAY_TH_MOUNTS_UP 5) + DoMount()
--   (Mount(SKARLOC_MOUNT_MODEL 18223) + MOVE_RUN SPEED_MOUNT 1.6f);
--   case 37 summons 2x watchman 17814 + sentry 17815; case 59
--   SummonCreature(SKARLOC_MOUNT 18798, 2488.64, 625.77, 58.26, 4.71,
--   TIMED_DESPAWN, 10s) + DoUnmount() + SetRun(false); case 60
--   HandleEmoteCommand(EMOTE_ONESHOT_EXCLAMATION) + escort-pause + gossip
--   flag + instance->SetData(TYPE_THRALL_PART2 5, DONE); case 64/81
--   SetRun(false); case 68 summons barn protector 18093 / lookout 18094 /
--   2x guardsman 18092; case 83 summons church protector 23179 / lookout
--   23177 / 2x guardsman 23175; case 84 Talk(SAY_TH_CHURCH_END 6); case 91
--   SetWalk(true) + SetRun(false); case 93 summons inn protector 23180 /
--   lookout 23178 / 2x guardsman 23176; case 94 Taretha (instance
--   DATA_TARETHA 8 GUID) Talk(SAY_TA_ESCAPED 1, me); case 95
--   Talk(SAY_TH_MEET_TARETHA 7) + instance->SetData(TYPE_THRALL_PART3 6,
--   DONE) + escort-pause; case 96 Talk(SAY_TH_EPOCH_WONDER 8); case 97
--   Talk(SAY_TH_EPOCH_KILL_TARETHA 9) + SetRun(); case 106: starts Taretha's
--   escort (ENSURE_AI(EscortAI, Taretha->AI())->Start(false, true,
--   playerGUID)) + KilledMonsterCredit(20156) for every player in map +
--   SummonCreature(EROZION_ENTRY 18723, 2646.47, 680.416, 55.38, 4.16,
--   TIMED_DESPAWN, 2min); case 108 SetVisible(false). No movement /
--   summon-with-position / instance-script bridges exist — documented
--   here, not wired (EscortAI movement + instance-script model, both
--   blocked, standing).
-- - Thrall gossip (no gossip bridge): OnGossipHello shows GOSSIP_ITEM_WALKING
--   (menu GOSSIP_ID_START 9568) when TYPE_BARREL_DIVERSION 1 == DONE and
--   !TYPE_THRALL_EVENT 2; GOSSIP_ITEM_SKARLOC1 ("Taretha cannot see you,
--   Thrall.", menu 9614) when TYPE_THRALL_PART1 3 == DONE and
--   !TYPE_THRALL_PART2 5; GOSSIP_ITEM_TARREN ("We're ready, Thrall.", menu
--   9597) when TYPE_THRALL_PART2 5 == DONE and !TYPE_THRALL_PART3 6.
--   OnGossipSelect: action 1 -> SetData(TYPE_THRALL_EVENT 2, IN_PROGRESS) +
--   SetData(TYPE_THRALL_PART1 3, IN_PROGRESS) + Talk(SAY_TH_START_EVENT_PART1
--   0) + Start(true, true, playerGUID) + SetMaxPlayerDistance(100.0f) +
--   SetDespawnAtEnd(false) + SetDespawnAtFar(false); action 2 ->
--   GOSSIP_ITEM_SKARLOC2 chain (menus 9579 / 9580); action 20 ->
--   SummonCreature(SKARLOC_MOUNT 18798, 2038.81, 270.26, 63.20, 5.41,
--   TIMED_DESPAWN, 12s) + SetData(TYPE_THRALL_PART2 5, IN_PROGRESS) +
--   Talk(SAY_TH_START_EVENT_PART2 4) + StartWP() (escort unpause); action 3
--   -> SetData(TYPE_THRALL_PART3 6, IN_PROGRESS) + StartWP().
-- - Thrall JustSummoned: barn guards/protectors/lookouts, SKARLOC_MOUNT,
--   EROZION_ENTRY are exempt; every other summoned creature gets
--   AttackStart(me) (summon-ownership bridge blocked, standing).
-- - npc_erozion (gossip-only, no gossip / inventory bridges — documented,
--   not registered): OnGossipHello offers "I need a pack of Incendiary
--   Bombs." (GOSSIP_ACTION_INFO_DEF+1) when instance TYPE_BARREL_DIVERSION
--   1 != DONE and the player lacks ITEM_ENTRY_BOMBS 25853, and "[PH]
--   Teleport please, i'm tired." (action +2) when QUEST_ENTRY_RETURN 10285
--   is complete (menu 9778); action +1 grants one Incendiary Bombs 25853
--   via CanStoreNewItem/StoreNewItem (menu 9515), action +2 just closes.
-- - npc_taretha (EscortAI, zero bridgeable arms — documented, not
--   registered): WaypointReached case 6 Talk(SAY_TA_FREE 0), case 7
--   HandleEmoteCommand(EMOTE_ONESHOT_CHEER); OnGossipHello shows
--   GOSSIP_ITEM_EPOCH1 ("Strange wizard?", menu 9610) when
--   TYPE_THRALL_PART3 6 == DONE and TYPE_THRALL_PART4 7 == NOT_STARTED;
--   OnGossipSelect action 2 -> SetData(TYPE_THRALL_PART4 7, IN_PROGRESS) +
--   SummonCreature(ENTRY_EPOCH 18096, 2639.13, 698.55, 65.43, 4.59,
--   TIMED_OR_DEAD_DESPAWN, 2min) if DATA_EPOCH 9 GUID empty + Thrall's
--   StartWP(). Taretha's entry (TARETHA_ENTRY 18887: old_hillsbrad.h:36 +
--   instance_old_hillsbrad.cpp:111 OnCreatureCreate case) and Erozion's
--   (EROZION_ENTRY 18723: named in this cpp's ThrallOldHillsbrad enum and
--   summoned at waypoint 106 — the skarloc summon-strength pattern) are
--   C++-verifiable, but neither script has a bridgeable arm to register.
-- - The C++ "/// @todo add his abilities'n-crap here" comment (Thrall
--   UpdateAI) and the commented-out UNIT_VIRTUAL_ITEM_INFO sets are ported
--   as-is above; the escort Talk lines (SAY_TH_START_EVENT_PART1 0 through
--   SAY_TH_EVENT_COMPLETE 10) fire only inside the unbridgeable escort /
--   gossip machine — no bridgeable firing context exists for them.
-- SD%Complete: 40 — quest support 10283/10284; SD%Complete: 60 flags the
-- missing pre-event spawns + uncoordinated Thrall-escort speech, which the
-- no-summon / EscortAI bridges above still block.

local ENTRY = 17876

local SAY_TH_RANDOM_LOW_HP = 11
local SAY_TH_RANDOM_DIE    = 12
local SAY_TH_RANDOM_AGGRO  = 13
local SAY_TH_RANDOM_KILL   = 14

local lowHp = {}
local lowHpPump = {}

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
    local guid = creature:GetGUID()
    cancelPump(guid)
    lowHp[guid] = false
    creature:Talk(SAY_TH_RANDOM_AGGRO)
    lowHpPump[guid] = CreateLuaEvent(function()
        if not lowHp[guid] and creature:GetHealthPct() < 20 then
            lowHp[guid] = true
            creature:Talk(SAY_TH_RANDOM_LOW_HP)
        end
    end, 1000, 0)
end

local function onLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelPump(guid)
    lowHp[guid] = false
end

local function onReset(event, creature)
    local guid = creature:GetGUID()
    cancelPump(guid)
    lowHp[guid] = false
end

local function onDied(event, creature, killer)
    local guid = creature:GetGUID()
    cancelPump(guid)
    lowHp[guid] = false
    -- C++ JustDied skips the yell when Thrall kills himself (player too
    -- far or escort end): killer == me.
    if not killer or killer:GetGUID() ~= guid then
        creature:Talk(SAY_TH_RANDOM_DIE)
    end
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 3, function(event, creature, victim)
    creature:Talk(SAY_TH_RANDOM_KILL)
end)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)
