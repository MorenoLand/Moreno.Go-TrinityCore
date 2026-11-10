-- Zul'Farrak: Sergeant Bly — Lua port of
-- src/server/scripts/Kalimdor/ZulFarrak/zulfarrak.cpp (class
-- npc_sergeant_bly : CreatureScript("npc_sergeant_bly") {
-- npc_sergeant_blyAI : public ScriptedAI }; GetAI via
-- GetZulFarrakAI<npc_sergeant_blyAI> (zulfarrak.h — GetInstanceAI<AI>(obj,
-- "instance_zulfarrak") gate, ZFScriptName "instance_zulfarrak",
-- DataHeader "ZF"); AddSC_zulfarrak at end registers it alongside
-- npc_weegli_blastfuse, go_shallow_grave, at_zumrah, go_troll_cage;
-- kalimdor loader decl 103 / call 216 per kalimdor_script_loader.cpp —
-- the SECOND "// Zul'Farrak" loader group, right after
-- AddSC_boss_zum_rah()). Whole-server-tree "npc_sergeant_bly" grep hits
-- only the ZulFarrak dir files + loader (sole-source verified); zero
-- sql/ hits.
-- Entry: zulfarrak.h ZFEntries names ENTRY_BLY = 7604 (alongside the
-- pyramid ENTRY_RAVEN/ORO/WEEGLI/MURTA entries) — kalecgos check PASSES;
-- creature_template ScriptName binding stays DB-side. Eluna creature
-- events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied, 23 OnReset, 38
-- OnDoAction. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady. No C++ HasUnitState
-- (UNIT_STATE_CASTING) gates in this AI.
-- Ported arms (combat only):
-- - Shield Bash 11972: DoCastVictim non-triggered via creature:GetVictim()
--   (boss_onyxia precedent), init 5s, reschedule 15s unconditional
--   (C++ Repeat outside any gate — faithful).
-- - Revenge 12170: DoCastVictim non-triggered, init 8s, reschedule 10s
--   unconditional. (C++ carries a code comment that Revenge should only
--   fire after a dodge/parry/block and needs Trinity support — the C++
--   itself casts it unconditionally, and this port mirrors the code, not
--   the comment.)
-- - Reset: me->SetFaction(FACTION_FRIENDLY) (zulfarrak.cpp:82-86) — live
--   now that the gossip machine below can flip the faction.
-- Ported arms (pyramid machine, 2026-11-10):
-- - OnGossipHello (zulfarrak.cpp:174-188): phase-gated menus from the
--   shared instanceData table (instance_zulfarrak.lua) — menu 1517 with
--   the GOSSIP_BLY item at PYRAMID_KILLED_ALL_TROLLS, menu 1515 at
--   PYRAMID_NOT_STARTED, menu 1516 otherwise. GossipMenuAddItem /
--   GossipSendMenu are the zulaman.lua twins; the hello hook vetoes the
--   default menu (gossip.go), so the handler returns nothing.
-- - OnGossipSelect (zulfarrak.cpp:157-172): action GOSSIP_ACTION_INFO_DEF
--   + 1 (1001) closes the menu, stores the player GUID, and starts the
--   postGossipStep machine (the C++ DoAction(0) call). The select args
--   carry sender/action/code/listID (gossip.go:381) — the nat_pagle.lua
--   pattern.
-- - postGossipStep machine (zulfarrak.cpp:89-138 + DoAction :151-155):
--   the UpdateAI poll is a CreateLuaEvent chain keyed by GUID — step 1
--   at Text_Timer 0 (delay 1ms, the minimum — effectively the next tick,
--   as C++ fires it on the next UpdateAI tick), steps 2/3 at +5s each.
--   Step 1 talks SAY_1 (0), step 2 talks SAY_2 (1), step 3 flips Bly to
--   FACTION_MONSTER, AttackStarts the stored player, and flips Raven /
--   Oro / Murta to FACTION_MONSTER if alive (switchFactionIfAlive via the
--   instanceData crew binds). The chain runs out of combat like C++ (the
--   block precedes UpdateVictim) and is cancelled only on death — C++
--   Reset() does not touch postGossipStep, so evade/leave-combat leave it
--   running; a dead creature stops UpdateAI, so ev4 cancels.
-- Unmodeled (documented-only):
-- - Step 1's weegli->AI()->DoAction(0) (zulfarrak.cpp:106) is vacuous —
--   Weegli defines no DoAction override (zulfarrak.cpp:270-387), so the
--   C++ base no-ops; there is nothing to dispatch to (the ev38 dispatch
--   itself is bridged, npc_weegli_blastfuse.lua header).
-- - go_troll_cage OnGossipHello crew-arouse machine (zulfarrak.cpp:218-232):
--   ENTRY-BLOCKED (no cage GO entry in the C++ tree or workspace SQL —
--   see instance_zulfarrak.lua); until the entry is sourced the crew
--   stays passive and the pyramid machine never starts.

local ENTRY = 7604

local SPELL_SHIELD_BASH = 11972
local SPELL_REVENGE     = 12170

local FACTION_FRIENDLY = 35
local FACTION_MONSTER = 14

local PYRAMID_NOT_STARTED = 0
local PYRAMID_KILLED_ALL_TROLLS = 8

local GOSSIP_ICON_CHAT = 0
local GOSSIP_SENDER_MAIN = 1
local GOSSIP_ACTION_INFO_DEF = 1000
local GOSSIP_BLY = "That's it!  I'm tired of helping you out.  It's time we settled things on the battlefield!"

local MENU_BLY_FIGHT = 1517
local MENU_BLY_NOT_STARTED = 1515
local MENU_BLY_IN_PROGRESS = 1516

local timers = {}
local machines = {}

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
    per[key] = CreateLuaEvent(fn, delay, 1)
end

local function cancelMachine(guid)
    local m = machines[guid]
    if m and m.timerId then
        RemoveEventById(m.timerId)
    end
    machines[guid] = nil
end

-- C++ UpdateAI postGossipStep block (zulfarrak.cpp:89-138): step 1 talks
-- SAY_1, step 2 talks SAY_2, step 3 flips factions + AttackStarts the
-- stored player; postGossipStep++ after each step, the machine ends at 4.
local function runPostGossipStep(creature, guid)
    local m = machines[guid]
    if not m or creature:IsDead() then
        machines[guid] = nil
        return
    end
    local step = m.step
    if step == 1 then
        creature:Talk(0) -- SAY_1
        m.step = 2
        m.timerId = CreateLuaEvent(function()
            runPostGossipStep(creature, guid)
        end, 5000, 1)
    elseif step == 2 then
        creature:Talk(1) -- SAY_2
        m.step = 3
        m.timerId = CreateLuaEvent(function()
            runPostGossipStep(creature, guid)
        end, 5000, 1)
    elseif step == 3 then
        creature:SetFaction(FACTION_MONSTER)
        if m.playerGuid then
            local player = GetPlayerByGUID(m.playerGuid)
            if player then
                creature:AttackStart(player)
            end
        end
        local data = creature:GetMap():GetInstanceData()
        if data then
            for _, key in ipairs({"raven", "oro", "murta"}) do
                local crew = data[key]
                if crew and crew:IsAlive() then
                    crew:SetFaction(FACTION_MONSTER)
                end
            end
        end
        machines[guid] = nil
    else
        machines[guid] = nil
    end
end

-- C++ DoAction (zulfarrak.cpp:151-155): postGossipStep = 1, Text_Timer = 0.
local function startPostGossip(creature, guid, playerGuid)
    cancelMachine(guid)
    machines[guid] = {step = 1, playerGuid = playerGuid}
    machines[guid].timerId = CreateLuaEvent(function()
        runPostGossipStep(creature, guid)
    end, 1, 1)
end

-- C++ EVENT_SHIELD_BASH (5s init): DoCastVictim(SPELL_SHIELD_BASH),
-- non-triggered; reschedule 15s unconditional.
local function onShieldBash(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SHIELD_BASH)
    end
    schedule(guid, "shieldbash", 15000, function()
        onShieldBash(creature, guid)
    end)
end

-- C++ EVENT_REVENGE (8s init): DoCastVictim(SPELL_REVENGE),
-- non-triggered; reschedule 10s unconditional.
local function onRevenge(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_REVENGE)
    end
    schedule(guid, "revenge", 10000, function()
        onRevenge(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "shieldbash", 5000, function()
        onShieldBash(creature, guid)
    end)
    schedule(guid, "revenge", 8000, function()
        onRevenge(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ has no JustDied arm, but a dead creature stops UpdateAI, so the
-- re-arming shield-bash / revenge timers must be cancelled on death or
-- they keep casting from the corpse forever. Death also stops the
-- postGossipStep machine (the AI object dies with it).
local function onDied(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    cancelMachine(guid)
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    creature:SetFaction(FACTION_FRIENDLY)
end

-- C++ DoAction (zulfarrak.cpp:151-155): starts the postGossipStep machine.
local function onDoAction(event, creature, action)
    startPostGossip(creature, creature:GetGUIDLow(), nil)
end

-- C++ OnGossipHello (zulfarrak.cpp:174-188): phase-gated menus.
local function onGossipHello(event, player, creature)
    local data = creature:GetMap():GetInstanceData()
    local phase = PYRAMID_NOT_STARTED
    if data and data.pyramidPhase then
        phase = data.pyramidPhase
    end
    if phase == PYRAMID_KILLED_ALL_TROLLS then
        player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_BLY, GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 1)
        player:GossipSendMenu(MENU_BLY_FIGHT, creature)
    elseif phase == PYRAMID_NOT_STARTED then
        player:GossipSendMenu(MENU_BLY_NOT_STARTED, creature)
    else
        player:GossipSendMenu(MENU_BLY_IN_PROGRESS, creature)
    end
end

-- C++ OnGossipSelect (zulfarrak.cpp:157-172): action INFO_DEF + 1 closes
-- the menu, stores the player, and starts the postGossipStep machine.
local function onGossipSelect(event, player, creature, sender, action)
    player:GossipClearMenu()
    if action == GOSSIP_ACTION_INFO_DEF + 1 then
        player:GossipComplete()
        startPostGossip(creature, creature:GetGUIDLow(), player:GetGUID())
    end
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)
RegisterCreatureEvent(ENTRY, 38, onDoAction)
RegisterCreatureGossipEvent(ENTRY, 1, onGossipHello)
RegisterCreatureGossipEvent(ENTRY, 2, onGossipSelect)
