-- Razorfen Downs: Belnistrasz (quest 3525 "Extinguishing the Idol") — Lua
-- port of src/server/scripts/Kalimdor/RazorfenDowns/razorfen_downs.cpp
-- (class npc_belnistrasz : CreatureScript("npc_belnistrasz") {
-- npc_belnistraszAI : public ScriptedAI }; GetAI via
-- GetRazorfenDownsAI<npc_belnistraszAI> (razorfen_downs.h —
-- GetInstanceAI<AI>(obj, "instance_razorfen_downs") gate — the
-- instance-side legs have no bridge, standing). AddSC_razorfen_downs at
-- end registers four zone scripts (kalimdor_script_loader.cpp decl :73 /
-- call :186, the "// Razorfen Downs" loader block); of the four,
-- npc_tomb_creature was already ported (lua_scripts/kalimdor/
-- npc_tomb_creature.lua), npc_idol_room_spawner and go_gong have zero
-- bridgeable arms (documented-only below) — this file covers
-- npc_belnistrasz only.
-- Entry: 8516 wowhead-verified (classic.wowhead.com/npc=8516/belnistrasz;
-- no NPC_ constant in the C++ tree — landslide/noxxion/ptheradras
-- precedent); creature_text rows back Talk ids 0-7.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset, 31 OnQuestAccept (engine/world/quest_details.go fires it
-- as (event, player, creature, quest); quest:GetId() wired —
-- luaQuest surface, lua_creature_events.go). Timers via CreateLuaEvent;
-- melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
-- Ported arms:
-- - Reset (C++ Reset, event 23): only the !eventInProgress branch is
--   reachable in this port (eventInProgress latches true on quest
--   accept; the C++ gossip-questgiver flag re-set has no bridge) —
--   DoCastSelf(SPELL_ARCANE_INTELLECT 13326) under the C++ HasAura
--   guard (non-triggered self-cast); local event/channeling state
--   re-armed, timers cleared.
-- - OnQuestAccept (C++ OnQuestAccept, event 31): quest id 3525
--   (QUEST_EXTINGUISHING_THE_IDOL) -> eventInProgress latch,
--   Talk(SAY_QUEST_ACCEPTED 0). The rest (RemoveFlag questgiver,
--   SetFaction escorted, MovePath(PATH_ESCORT 871710) escort run) has
--   no bridges — documented-only.
-- - Engage (C++ JustEngagedWith): C++-faithful: channeling (never set
--   here — MovementInform has no bridge) -> Talk(SAY_WATCH_OUT 7);
--   else arm Fireball at 1s and Frost Nova at {8s,12s}, and with
--   urand(0,100) > 40 -> Talk(SAY_AGGRO 6). C++ Talk takes the
--   target player; the bridge broadcasts (deviation, documented).
-- - Fireball (EVENT_FIREBALL): DoCastVictim(SPELL_FIREBALL 9053),
--   non-triggered (2-arg DoCastVictim); init 1s -> repeat 8s
--   unconditional (nil-victim keeps schedule — jeklik convention;
--   C++ returns WITHOUT re-arming when UNIT_STATE_CASTING or no
--   victim, mal_ganis precedent, documented deviation).
-- - Frost Nova (EVENT_FROST_NOVA): DoCastAOE(SPELL_FROST_NOVA 11831)
--   resolves to self-cast (kazrogal/illidan precedent), non-triggered;
--   init {8s,12s} -> repeat 15s; same victimless/casting-gate
--   deviation as Fireball.
-- - LeaveCombat (event 2): cancel timers (C++ UpdateAI early-return
--   without victim never clears scheduled events, but the hyjal.lua
--   convention clears timers on leave-combat; no timer re-arm
--   semantics change).
-- - Died (event 4): clear timers + state (C++ instance
--   SetBossState(DATA_EXTINGUISHING_THE_IDOL 6, DONE) + 5s despawn
--   have no bridges — documented-only).
-- Unmodeled (documented-only, no bridges):
-- - MovementInform WAYPOINT id 17 (channeling start), the escort
--   MovePath machine, EVENT_CHANNEL/IDOL_ROOM_SPAWNER/PROGRESS/
--   COMPLETE chain (SummonCreature NPC_IDOL_ROOM_SPAWNER 8611 with
--   SetData spawner-count, Talk progress lines 1-5, idol-shutdown
--   visual 12774, camera shake 12816, brazier GO summon 152097,
--   GroupEventHappens credit, idol-fire GO deletes 151951/151952/
--   151973, InterruptSpell channeled) — the whole idol-ritual machine
--   needs MovementInform + summon + quest-credit + GO-summon bridges.
-- - npc_idol_room_spawner ("npc_idol_room_spawner"): its only arm is
--   SetData summoning 4 trash entries (Withered Battle Boar 7333,
--   Death's Head Geomancer 7335, Withered Quilguard 7329,
--   Plaguemaw the Rotting 7356) — zero bridges, no registration.
-- - go_gong ("go_gong", GO 148917): OnGossipHello -> SendCustomAnim(0)
--   + instance SetData(DATA_WAVE 5, IN_PROGRESS) — no instance-data
--   bridge, zero bridges, no registration.

local ENTRY_BELNISTRASZ = 8516

local SAY_QUEST_ACCEPTED = 0
local SAY_AGGRO          = 6
local SAY_WATCH_OUT      = 7

local QUEST_EXTINGUISHING_THE_IDOL = 3525

local SPELL_ARCANE_INTELLECT = 13326
local SPELL_FIREBALL         = 9053
local SPELL_FROST_NOVA       = 11831

local timers = {}
local state  = {}

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

local function getState(guid)
    local st = state[guid]
    if not st then
        st = { eventInProgress = false, channeling = false }
        state[guid] = st
    end
    return st
end

local function clearState(guid)
    cancelTimers(guid)
    state[guid] = nil
end

-- C++ EVENT_FIREBALL: DoCastVictim(9053); re-arm 8s unconditional
-- (jeklik convention; C++ returns without re-arming when casting or
-- victimless — mal_ganis precedent, documented deviation).
local function onFireball(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FIREBALL)
    end
    schedule(guid, "fireball", 8000, function()
        onFireball(creature, guid)
    end)
end

-- C++ EVENT_FROST_NOVA: DoCastAOE(11831) -> self-cast (kazrogal/illidan
-- precedent); re-arm 15s unconditional, same blocked-gate deviation.
local function onFrostNova(creature, guid)
    creature:CastSpell(creature, SPELL_FROST_NOVA)
    schedule(guid, "frostnova", 15000, function()
        onFrostNova(creature, guid)
    end)
end

-- C++ JustEngagedWith: channeling -> Talk(WATCH_OUT); else arm the two
-- combat timers + 60% aggro talk (Talk broadcasts; the C++ targeted-
-- player variant has no bridge).
local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    clearState(guid)
    local st = getState(guid)
    if st.channeling then
        creature:Talk(SAY_WATCH_OUT)
        return
    end
    schedule(guid, "fireball", 1000, function()
        onFireball(creature, guid)
    end)
    schedule(guid, "frostnova", math.random(8000, 12000), function()
        onFrostNova(creature, guid)
    end)
    if math.random(0, 100) > 40 then
        creature:Talk(SAY_AGGRO)
    end
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ Reset: only the !eventInProgress branch is reachable here —
-- refresh Arcane Intellect under the C++ HasAura guard; re-arm local
-- event/channeling state (the gossip-questgiver flag re-set has no
-- bridge).
local function onReset(event, creature)
    local guid = creature:GetGUID()
    local st = getState(guid)
    cancelTimers(guid)
    if not st.eventInProgress then
        if not creature:HasAura(SPELL_ARCANE_INTELLECT) then
            creature:CastSpell(creature, SPELL_ARCANE_INTELLECT)
        end
        st.channeling = false
    end
end

-- C++ OnQuestAccept for quest 3525: latch the event + Talk(0); the
-- escort start (gossip-flag removal, faction swap, MovePath 871710)
-- has no bridges.
local function onQuestAccept(event, player, creature, quest)
    if not quest or quest:GetId() ~= QUEST_EXTINGUISHING_THE_IDOL then
        return
    end
    local st = getState(creature:GetGUID())
    st.eventInProgress = true
    creature:Talk(SAY_QUEST_ACCEPTED)
end

local function onDied(event, creature, killer)
    clearState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_BELNISTRASZ, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_BELNISTRASZ, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY_BELNISTRASZ, 4, onDied)
RegisterCreatureEvent(ENTRY_BELNISTRASZ, 23, onReset)
RegisterCreatureEvent(ENTRY_BELNISTRASZ, 31, onQuestAccept)
