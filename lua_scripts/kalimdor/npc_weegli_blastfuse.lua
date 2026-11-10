-- Zul'Farrak: Weegli Blastfuse — Lua port of
-- src/server/scripts/Kalimdor/ZulFarrak/zulfarrak.cpp (class
-- npc_weegli_blastfuse : CreatureScript("npc_weegli_blastfuse") {
-- npc_weegli_blastfuseAI : public ScriptedAI }; GetAI via
-- GetZulFarrakAI<npc_weegli_blastfuseAI> (zulfarrak.h — GetInstanceAI<AI>(obj,
-- "instance_zulfarrak") gate, ZFScriptName "instance_zulfarrak",
-- DataHeader "ZF"); AddSC_zulfarrak at end registers it alongside
-- npc_sergeant_bly, go_shallow_grave, at_zumrah, go_troll_cage;
-- kalimdor loader decl 103 / call 216 per kalimdor_script_loader.cpp —
-- the SECOND "// Zul'Farrak" loader group, right after
-- AddSC_boss_zum_rah()). Whole-server-tree "npc_weegli_blastfuse" grep
-- hits only the ZulFarrak dir files + loader (sole-source verified);
-- zero sql/ hits.
-- Entry: zulfarrak.h ZFEntries names ENTRY_WEEGLI = 7607 (alongside the
-- pyramid ENTRY_BLY/RAVEN/ORO/MURTA entries) — kalecgos check PASSES;
-- creature_template ScriptName binding stays DB-side. Eluna creature
-- events: 1 OnEnterCombat, 2 OnLeaveCombat, 23 OnReset, 28
-- OnMovementInform. Timers via CreateLuaEvent; melee is engine-driven in
-- Go (creature combat tick), like C++ DoMeleeAttackIfReady. No C++
-- HasUnitState(UNIT_STATE_CASTING) gates in this AI.
-- Ported arms (combat only):
-- - Bomb 8858: DoCastVictim non-triggered via creature:GetVictim()
--   (boss_onyxia precedent), init 10s, reschedule 10s unconditional
--   (C++ Repeat outside any gate — faithful). Death cancels the Lua-side
--   timer (the C++ AI object dies with its EventMap — a fresh 10s on the
--   next engage); evade preserves it (C++ Reset leaves Bomb_Timer
--   untouched).
-- Unmodeled (documented-only, no bridges):
-- - AttackStart: AttackStartCaster(victim, 10) (keep back & toss
--   bombs/shoot) — stays out: EngageWithTarget (2026-10-24
--   bridge) is the melee aggro funnel, not a substitute for
--   the ranged keep-back override, and the engine's combat
--   entry has no Lua AttackStart call site for Weegli.
-- - Shoot 6660 + SetSheath(SHEATH_STATE_RANGED/MELEE): PORTED 2026-10-20
--   (zulfarrak.cpp:311-322) via the new creature:SetSheath bridge
--   (Unit::SetSheath, Unit.h:903). The isAttackReady swing gate has no
--   bridge, so the Lua check re-arms at SWING_MS=2000 (engine default
--   base attack time); melee-range gate is creature:IsWithinMeleeRange
--   (C++-exact; the old 5.0 assumption is retired).
--   Melee stays engine-driven (DoMeleeAttackIfReady twin); the shoot
--   fires at most once per swing, only when the victim is out of range.
-- Ported arms (pyramid machine, 2026-11-10):
-- - OnGossipHello (zulfarrak.cpp:361-378): phase-gated menus from the
--   shared instanceData table — menu 1514 with the GOSSIP_WEEGLI item at
--   PYRAMID_KILLED_ALL_TROLLS, menu 1511 at PYRAMID_NOT_STARTED, menu
--   1513 otherwise.
-- - OnGossipSelect (zulfarrak.cpp:348-359): action GOSSIP_ACTION_INFO_DEF
--   + 1 (1001) closes the menu and runs DestroyDoor.
-- - DestroyDoor (zulfarrak.cpp:332-345): SetFaction(FACTION_FRIENDLY) +
--   MoveTo(0, 1858.57, 1146.35, 14.745) + SetHomePosition(same, 3.85) +
--   Talk(SAY_WEEGLI_OK_I_GO=1) + destroyingDoor latch. MoveTo is the
--   MovePoint bridge (lua_creature_events.go:4974); the arrival fires
--   ev28 below.
-- - MovementInform (zulfarrak.cpp:324-330, ev28 — the handler receives
--   (event, creature, moveType, id), the broodkeeper precedent; C++
--   ignores type/id and branches on state): PYRAMID_CAGES_OPEN ->
--   instanceData:SetData(EVENT_PYRAMID, PYRAMID_ARRIVED_AT_STAIR) +
--   Talk(SAY_WEEGLI_OHNO=0) + SetHomePosition(1882.69, 1272.28, 41.87,
--   0); destroyingDoor -> open the end door + DespawnOrUnsummon. The
--   door leg is the InstanceScript::DoUseDoorOrButton(GO_END_DOOR 146084)
--   twin via FindNearestGameObject + SetGoState(GO_STATE_ACTIVE 0), the
--   broggok precedent (DoUseDoorOrButton with restore 0 leaves the door
--   open, exactly what SetGoState(ACTIVE) does). C++ checks the phase
--   branch first, else-if the door branch — replicated.
--   The CAGES_OPEN leg is ported and reachable now: go_troll_cage is
--   ported in instance_zulfarrak.lua (entries 141070-141074), the only
--   CAGES_OPEN writer.
--   Cage-herd coords (zulfarrak.cpp:220-228): Bly 1884.99/1263/41.52,
--   Raven 1882.5/1263/41.52, Oro 1886.47/1270.68/41.68, Weegli 1890/
--   1263/41.52, Murta 1891.19/1272.03/41.60, facing 0.
-- Unmodeled (documented-only, no bridges):
-- - LandMine_Timer (30s init, never fires in the C++ UpdateAI — dead
--   leg); SPELL_GOBLIN_LAND_MINE 21688 and SPELL_WEEGLIS_BARREL 10772
--   are dead enum entries (never cast). Reset and JustDied are
--   commented-out in C++ — nothing to port.
-- - go_troll_cage OnGossipHello crew-arouse machine (zulfarrak.cpp:218-232):
--   PORTED in instance_zulfarrak.lua (entries 141070-141074).
-- - Bly->Weegli DoAction(0) (zulfarrak.cpp:106) is vacuous — Weegli defines
--   no DoAction override (zulfarrak.cpp:270-387), so the C++ base no-ops;
--   no ev38 handler is registered for Weegli.

local ENTRY = 7607

local SPELL_BOMB = 8858
local SPELL_SHOOT = 6660

local FACTION_FRIENDLY = 35

local EVENT_PYRAMID = 1
local PYRAMID_NOT_STARTED = 0
local PYRAMID_CAGES_OPEN = 1
local PYRAMID_ARRIVED_AT_STAIR = 2
local PYRAMID_KILLED_ALL_TROLLS = 8

local GO_END_DOOR = 146084
local GO_STATE_ACTIVE = 0

local GOSSIP_ICON_CHAT = 0
local GOSSIP_SENDER_MAIN = 1
local GOSSIP_ACTION_INFO_DEF = 1000
local GOSSIP_WEEGLI = "Will you blow up that door now?"

local MENU_WEEGLI_DOOR = 1514
local MENU_WEEGLI_NOT_STARTED = 1511
local MENU_WEEGLI_IN_PROGRESS = 1513

-- C++ melee/ranged decision (zulfarrak.cpp:311-322): when the swing timer is
-- ready and the victim is beyond melee range, Weegli shoots 6660 and shows
-- the ranged weapon (SHEATH_STATE_RANGED); otherwise he fights melee and
-- shows the melee weapon (SHEATH_STATE_MELEE). Gate is now the
-- creature:IsWithinMeleeRange bridge (C++-exact).
-- isAttackReady pacing: the Lua side has no swing-timer bridge, so the
-- check re-arms at the engine's default base attack time (2000ms —
-- NULLIF(t.BaseAttackTime, 0) fallback in creaturemotion.go / combat.go);
-- the shoot then fires at most once per swing, never faster than C++.
local SWING_MS = 2000

local timers = {}
local doorState = {}

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

-- C++ DestroyDoor (zulfarrak.cpp:332-345).
local function destroyDoor(creature, guid)
    if creature:IsDead() then
        return
    end
    creature:SetFaction(FACTION_FRIENDLY)
    creature:MoveTo(0, 1858.57, 1146.35, 14.745)
    creature:SetHomePosition(1858.57, 1146.35, 14.745, 3.85)
    creature:Talk(1) -- SAY_WEEGLI_OK_I_GO
    doorState[guid] = true
end

-- C++ EVENT_BOMB (10s init): DoCastVictim(SPELL_BOMB), non-triggered;
-- reschedule 10s unconditional.
local function onBomb(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_BOMB)
    end
    schedule(guid, "bomb", 10000, function()
        onBomb(creature, guid)
    end)
end

-- C++ UpdateAI ranged/melee arm (zulfarrak.cpp:311-322): swing-ready and
-- victim beyond melee range -> DoCastVictim(SPELL_SHOOT) non-triggered +
-- SetSheath(SHEATH_STATE_RANGED); otherwise SetSheath(SHEATH_STATE_MELEE).
-- Melee itself is engine-driven in Go (DoMeleeAttackIfReady twin, a no-op
-- out of range), so only the shoot+sheath decision is ported; the 2s
-- re-arm paces the isAttackReady swing gate (SWING_MS above).
local function onSwingCheck(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        if creature:IsWithinMeleeRange(victim) then
            creature:SetSheath(1) -- SHEATH_STATE_MELEE
        else
            creature:CastSpell(victim, SPELL_SHOOT) -- DoCastVictim, non-triggered
            creature:SetSheath(2) -- SHEATH_STATE_RANGED
        end
    end
    schedule(guid, "swing", SWING_MS, function()
        onSwingCheck(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    -- C++ Reset() is effectively empty (body commented out): Bomb_Timer
    -- is NOT re-armed on evade, it freezes and resumes. Preserve a
    -- pending bomb timer across re-engages instead of restarting 10s.
    local per = timers[guid]
    if not (per and per["bomb"]) then
        schedule(guid, "bomb", 10000, function()
            onBomb(creature, guid)
        end)
    end
    -- The C++ UpdateAI evaluates the shoot/sheath decision every tick while
    -- a victim exists: run one immediate check, then re-arm on the swing
    -- cadence. The poll self-cancels at nil victim only by timer removal;
    -- it is cancelled on leave-combat and death below.
    if not (per and per["swing"]) then
        onSwingCheck(creature, guid)
    end
end

-- C++ Reset() leaves Bomb_Timer untouched, so leave-combat and reset
-- must not cancel the pending bomb; the timer resumes on re-engage.
-- The swing poll has no C++ timer state — it is evaluated only while a
-- victim exists — so leave-combat cancels it and re-engage restarts it.
local function onLeaveCombat(event, creature)
    local per = timers[creature:GetGUIDLow()]
    if per and per["swing"] then
        RemoveEventById(per["swing"])
        per["swing"] = nil
    end
end

local function onReset(event, creature)
end

-- The C++ AI object dies with its EventMap (JustDied commented out, but
-- the creature object is destroyed), so a death must cancel the Lua-side
-- bomb timer: otherwise it re-arms forever post-death and the stale
-- per["bomb"] blocks the fresh 10s ctor re-arm on the next engage.
-- Death also clears the destroyingDoor latch (the AI object dies with it).
local function onDied(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    doorState[guid] = nil
end

-- C++ MovementInform (zulfarrak.cpp:324-330): phase branch first,
-- destroyingDoor branch else-if; type/id are ignored.
local function onMovementInform(event, creature, moveType, id)
    local data = creature:GetMap():GetInstanceData()
    local phase = PYRAMID_NOT_STARTED
    if data and data.pyramidPhase then
        phase = data.pyramidPhase
    end
    if phase == PYRAMID_CAGES_OPEN then
        if data then
            data:SetData(EVENT_PYRAMID, PYRAMID_ARRIVED_AT_STAIR)
        end
        creature:Talk(0) -- SAY_WEEGLI_OHNO
        creature:SetHomePosition(1882.69, 1272.28, 41.87, 0)
    elseif doorState[creature:GetGUIDLow()] then
        local door = creature:FindNearestGameObject(GO_END_DOOR, 500)
        if door then
            door:SetGoState(GO_STATE_ACTIVE)
        end
        creature:DespawnOrUnsummon()
    end
end

-- C++ OnGossipHello (zulfarrak.cpp:361-378): phase-gated menus.
local function onGossipHello(event, player, creature)
    local data = creature:GetMap():GetInstanceData()
    local phase = PYRAMID_NOT_STARTED
    if data and data.pyramidPhase then
        phase = data.pyramidPhase
    end
    if phase == PYRAMID_KILLED_ALL_TROLLS then
        player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_WEEGLI, GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 1)
        player:GossipSendMenu(MENU_WEEGLI_DOOR, creature)
    elseif phase == PYRAMID_NOT_STARTED then
        player:GossipSendMenu(MENU_WEEGLI_NOT_STARTED, creature)
    else
        player:GossipSendMenu(MENU_WEEGLI_IN_PROGRESS, creature)
    end
end

-- C++ OnGossipSelect (zulfarrak.cpp:348-359): action INFO_DEF + 1 closes
-- the menu and runs DestroyDoor.
local function onGossipSelect(event, player, creature, sender, action)
    player:GossipClearMenu()
    if action == GOSSIP_ACTION_INFO_DEF + 1 then
        player:GossipComplete()
        destroyDoor(creature, creature:GetGUIDLow())
    end
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)
RegisterCreatureEvent(ENTRY, 28, onMovementInform)
RegisterCreatureGossipEvent(ENTRY, 1, onGossipHello)
RegisterCreatureGossipEvent(ENTRY, 2, onGossipSelect)
