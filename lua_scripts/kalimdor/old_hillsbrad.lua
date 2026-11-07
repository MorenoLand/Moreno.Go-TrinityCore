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
-- - Reset (event 23): clears the LowHp latch (C++ Initialize() leg); the
--   re-mount-if-HadMount, virtual-item-slot clear, and
--   SetDisplayId(THRALL_MODEL_UNEQUIPPED 17292) legs ride the
--   unported escort machine (display bridge live 2026-10-20; Mount bridge live
--   lua_creature_events.go — the legs ride the unported escort machine); the Talk
--   SAY_TH_LEAVE_COMBAT (15) leg is
--   conditioned
--   on HasEscortState(STATE_ESCORT_ESCORTING) — unmodeled escort state.
-- Unmodeled (documented-only — display bridge live 2026-10-20,
-- the legs ride the unported escort machine):
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
--   flag + instance->SetData(TYPE_THRALL_PART2 4, DONE); case 64/81
--   SetRun(false); case 68 summons barn protector 18093 / lookout 18094 /
--   2x guardsman 18092; case 83 summons church protector 23179 / lookout
--   23177 / 2x guardsman 23175; case 84 Talk(SAY_TH_CHURCH_END 6); case 91
--   SetWalk(true) + SetRun(false); case 93 summons inn protector 23180 /
--   lookout 23178 / 2x guardsman 23176; case 94 Taretha (instance
--   DATA_TARETHA 8 GUID) Talk(SAY_TA_ESCAPED 1, me); case 95
--   Talk(SAY_TH_MEET_TARETHA 7) + instance->SetData(TYPE_THRALL_PART3 5,
--   DONE) + escort-pause; case 96 Talk(SAY_TH_EPOCH_WONDER 8); case 97
--   Talk(SAY_TH_EPOCH_KILL_TARETHA 9) + SetRun(); case 106: starts Taretha's
--   escort (ENSURE_AI(EscortAI, Taretha->AI())->Start(false, true,
--   playerGUID)) + KilledMonsterCredit(20156) for every player in map +
--   SummonCreature(EROZION_ENTRY 18723, 2646.47, 680.416, 55.38, 4.16,
--   TIMED_DESPAWN, 2min); case 108 SetVisible(false). No movement /
--   summon-with-position / instance-script bridges exist — documented
--   here, not wired (EscortAI movement + instance-script model, both
--   blocked, standing).
-- - Thrall gossip (stale 'no gossip bridge' retracted —
--   RegisterCreatureGossipEvent bridged): OnGossipHello shows
--   GOSSIP_ITEM_WALKING
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
-- - Thrall JustSummoned: barn guards/protectors/lookouts, SKARLOC_MOUNT,
--   EROZION_ENTRY are exempt; every other summoned creature gets
--   AttackStart(me) (summon-ownership bridge blocked, standing).
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
--   case 7 HandleEmoteCommand(EMOTE_ONESHOT_CHEER) — escort-only,
--   documented. OnGossipHello shows GOSSIP_ITEM_EPOCH1 ("Strange
--   wizard?", menu 9610) when TYPE_THRALL_PART3 5 == DONE and
--   TYPE_THRALL_PART4 6 == NOT_STARTED; OnGossipSelect action 1001 ->
--   EPOCH2 item + menu 9613 (pure menu nav), action 1002 ->
--   CloseGossipMenuFor + the instance/summon machine (SetData
--   TYPE_THRALL_PART4 6 IN_PROGRESS, SummonCreature ENTRY_EPOCH 18096
--   at 2639.13/698.55/65.43/4.59 TIMED_OR_DEAD_DESPAWN 2min if
--   DATA_EPOCH 9 GUID empty, Thrall StartWP). PORTED below (entry
--   TARETHA_ENTRY 18887: instance_old_hillsbrad.cpp:36): the hello
--   instance gate is documented-only (kalecgos precedent); the
--   action-1002 machine's SetData/summon/escort legs are unbridged,
--   so only the menu close is ported there.
-- - The C++ "/// @todo add his abilities'n-crap here" comment (Thrall
--   UpdateAI) and the commented-out UNIT_VIRTUAL_ITEM_INFO sets are ported
--   as-is above; the escort Talk lines (SAY_TH_START_EVENT_PART1 0 through
--   SAY_TH_EVENT_COMPLETE 10) fire only inside the unbridgeable escort /
--   gossip machine — no bridgeable firing context exists for them.
-- SD%Complete: 40 — quest support 10283/10284; SD%Complete: 60 flags the
-- missing pre-event spawns + uncoordinated Thrall-escort speech, which the
-- no-summon / EscortAI bridges above still block.

-- PORTED 2026-11-07 (tick 397): the Thrall gossip writers (hello gates
-- + select actions 1/2/20/3 writing instanceData.thrallEvent /
-- thrallPart1-3 + Talk(0)/(4) + SKARLOC_MOUNT summon), the JustDied
-- TYPE_THRALL_EVENT FAIL leg (20-fail cap), the Taretha action-1002
-- machine (thrallPart4 + Epoch summon iff DATA_EPOCH empty), and the
-- Erozyon bombs-item gates (instance barrelDiversion + GetItemCount)
-- — the "no instance GetData/SetData bridge" claims in the paragraphs
-- above are retracted for the slots this file now writes/reads via the
-- shared instanceData table. Still out: Start/StartWP escort legs,
-- the WaypointReached machine, the barrel counter itself (go_barrel GO
-- entry is DB-side), and the boss-file GetData readers (skarloc /
-- epoch-hunter / drake), which still read the C++ encounter array.

local ENTRY = 17876

local SAY_TH_RANDOM_LOW_HP = 11
local SAY_TH_RANDOM_DIE    = 12
local SAY_TH_RANDOM_AGGRO  = 13
local SAY_TH_RANDOM_KILL   = 14

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
    if creature:IsMounted() then
        -- C++ DoUnmount: Dismount + SetSpeedRate(MOVE_RUN, SPEED_RUN);
        -- SPEED_RUN is 1.0f (old_hillsbrad.cpp:186).
        creature:Dismount()
        creature:SetSpeedRate(1, 1.0)
        -- C++ also latches HadMount = true for Reset's re-mount leg; that
        -- escort/Reset machine is unported, so the latch has no bearer.
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
-- Thrall StartWP) — instance/summon/escort unbridged, so only the
-- menu close is ported on that leg, the machine documented above.
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
        -- + Thrall StartWP. PORTED 2026-11-07 except Thrall StartWP
        -- (no escort bridge); the epoch gate reads the
        -- instance_old_hillsbrad.lua bind (nil = never summoned).
        local data = instanceDataOf(creature)
        if data then
            data.thrallPart4 = STATE_IN_PROGRESS
            if not data.epoch then
                creature:SummonCreature(18096, 2639.13, 698.55, 65.43, 4.59, 0, 120000)
            end
        end
    end
end

RegisterCreatureGossipEvent(TARETHA_ENTRY, 1, tarethaGossipHello)
RegisterCreatureGossipEvent(TARETHA_ENTRY, 2, tarethaGossipSelect)

-- npc_erozion gossip (old_hillsbrad.cpp:55-98) — entry EROZION_ENTRY
-- 18723 (old_hillsbrad.cpp:173).
local EROZION_ENTRY = 18723

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
    -- counter itself still has no writer (go_barrel GO entry is DB-side,
    -- not in the C++ tree), so the gate reads NOT_STARTED until that port
    -- lands.
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
-- select writers land the SetData legs. The escort legs
-- (Start/StartWP/SetMaxPlayerDistance/SetDespawnAtEnd/SetDespawnAtFar)
-- have no bridge and stay out — Talk + the state write are the live
-- halves. At most one hello branch matches (gates are mutually
-- exclusive: gate 1 needs !thrallEvent, gate 2 needs part1 DONE +
-- !part2, gate 3 needs part2 DONE + !part3).
local SAY_TH_START_EVENT_PART1 = 0
local SAY_TH_START_EVENT_PART2 = 4
local GOSSIP_ID_START = 9568
local GOSSIP_ID_SKARLOC1 = 9614
local GOSSIP_ID_SKARLOC2 = 9579
local GOSSIP_ID_SKARLOC3 = 9580
local GOSSIP_ID_TARREN = 9597
local GOSSIP_ITEM_WALKING = "We are ready to get you out of here, Thrall. Let's go!"
local GOSSIP_ITEM_SKARLOC1 = "Taretha cannot see you, Thrall."
local GOSSIP_ITEM_SKARLOC2 = "The situation is rather complicated, Thrall. It would be best for you to head into the mountains now, before more of Blackmoore's men show up. We'll make sure Taretha is safe."
local GOSSIP_ITEM_TARREN = "We're ready, Thrall."
local SKARLOC_MOUNT = 18798

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
    elseif action == GOSSIP_ACTION_INFO_DEF + 2 then
        player:GossipClearMenu()
        player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_ITEM_SKARLOC2,
            GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 20)
        player:GossipSendMenu(GOSSIP_ID_SKARLOC2, creature)
    elseif action == GOSSIP_ACTION_INFO_DEF + 20 then
        player:GossipSendMenu(GOSSIP_ID_SKARLOC3, creature)
        creature:SummonCreature(SKARLOC_MOUNT, 2038.81, 270.26, 63.20, 5.41, 1, 12000)
        if data then
            data.thrallPart2 = STATE_IN_PROGRESS
        end
        creature:Talk(SAY_TH_START_EVENT_PART2)
    elseif action == GOSSIP_ACTION_INFO_DEF + 3 then
        player:GossipComplete()
        if data then
            data.thrallPart3 = STATE_IN_PROGRESS
        end
    end
end

RegisterCreatureGossipEvent(ENTRY, 1, thrallGossipHello)
RegisterCreatureGossipEvent(ENTRY, 2, thrallGossipSelect)
