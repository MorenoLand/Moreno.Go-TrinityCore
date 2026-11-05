-- Broggok (Blood Furnace, Hellfire Citadel) — Lua port of
-- src/server/scripts/Outland/HellfireCitadel/BloodFurnace/
-- boss_broggok.cpp (boss_broggok — the only CreatureScript AI
-- class AddSC_boss_broggok registers; npc_nascent_fel_orc
-- and npc_fel_orc_neophyte are the two prisoner AI structs
-- registered alongside in this file; go_broggok_lever is a
-- GameObjectScript — gossip handler PORTED below (ev38 PREPARE
-- arm + lever flag/state legs); spell_broggok_poison_cloud is an AuraScript —
-- no AuraScript bridge, documented only, not registered).
-- First boss in the Blood Furnace set per
-- outland_script_loader.cpp order (instance_blood_furnace.
-- cpp stays blocked on the instance-script model).
-- Entry: NPC_BROGGOK = 17380 in blood_furnace.h (also mapped
-- in instance_blood_furnace.cpp creatureData { NPC_BROGGOK,
-- DATA_BROGGOK }); the creature_template ScriptName bindings
-- are DB-side (no TDB in this workspace).
-- Talk: SAY_AGGRO = 0 (pull). The file's KilledUnit override
-- does not exist — no event 3 registered.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4
-- OnDied, 23 OnReset. Timers via CreateLuaEvent (per-GUID
-- scheduler pump); melee is engine-driven in Go (creature
-- combat tick), like C++ DoMeleeAttackIfReady. C++ DoCast
-- default is triggered=false (vaelastrasz convention).
-- Fight shape (C++-exact for all modeled arms): Broggok
-- (17380): OnEnterCombat(1): per-GUID reset to the C++
-- ACTION_ACTIVATE_BROGGOK schedule values (slimeSpray 10000
-- / poisonBolt 7000 / poisonCloud 5000) + Talk(SAY_AGGRO)
-- + 1s scheduler pump (a port of ExecuteEvent — 1s
-- granularity exact for all C++ timers; the !UpdateVictim
-- early return collapses into the pump). BossAI base UpdateAI pre-loop (ScriptedCreature.cpp:578) + in-loop (:584) CASTING gates ported per arm below (no boss UpdateAI override; prisoner adds' plain scheduler UpdateAI has no gate). Pump: slime spray
-- 30913: 10s init then 4000+rand32()%8000 (C++ {4s,12s}) —
-- non-triggered DoCastVictim (felmyst convention), re-arm
-- {4s,12s} regardless (C++-exact); poison bolt 30917: 7s
-- init then {4s,12s} — non-triggered DoCastVictim, re-arm
-- {4s,12s} regardless (C++-exact); poison cloud 30916: 5s
-- init then 20s — non-triggered DoCastSelf (C++ DoCast(me),
-- C++-exact), re-arm 20s regardless (C++-exact).
-- OnDied(4): cleanup (the _JustDied arm is instance-blocked).
-- OnLeaveCombat(2)/OnReset(23): cancel the pump, drop
-- per-GUID state (the _Reset arm is instance-blocked; the
-- whole Reset() -> DoAction(ACTION_RESET_BROGGOK) machine —
-- react/flag/immune flips, summons.DespawnAll,
-- SetBossState(DATA_BROGGOK, NOT_STARTED), lever GO reset —
-- is instance/flag/summon/GO gated, no bridges).
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI
-- shim (instance_blood_furnace.cpp stays blocked on the
-- instance-script model; the lever gossip + DoAction(PREPARE)
-- legs are PORTED below — the SetBossState IN_PROGRESS leg,
-- prisoner-wave cell release, all-prisoners-dead counter,
-- ActivateCell(DATA_DOOR_4) and DoAction(ACTION_ACTIVATE_
-- BROGGOK) stay instance-gated); the C++ JustEngagedWith does NOT
-- schedule the events — the schedule fires only from the
-- unbridgeable ACTIVATE DoAction — so the activate schedule
-- is applied at the only engagement hook this model has,
-- OnEnterCombat(1) (Talk(SAY_AGGRO) stays on engage,
-- C++-exact); SetReactState IS bridged (lua_creature_events.go:1197 —
-- stale "no react bridge" retracted); the ACTIVATE arm's
-- SetReactState(REACT_AGGRESSIVE) and the RESET arm's mirror flip stay
-- out with the unbridged DoAction machine; the NON_ATTACKABLE flag
-- flips and the SetImmuneToAll(false/true) flag legs stay out
-- on the unbridgeable DoAction gate (the SetFlag/RemoveFlag(59)
-- surface itself IS bridged, lua_creature_events.go — the stale
-- 'no flag/immune bridges' claim is retracted, 2026-10-23); no
-- DoZoneInCombat bridge — the ACTIVATE DoZoneInCombat arm
-- unmodeled; the JustSummoned arms for NPC_BROGGOK_POISON_CLOUD
-- 17662 (react passive + CastSpell SPELL_POISON_CLOUD_PASSIVE 30914)
-- and NPC_INCOMBAT_TRIGGER 16006 (react passive) are PORTED in
-- onJustSummoned via the spell-summon EffectSummonType bridge
-- (event 19) — the C++ summons.Summon list has no bridge
-- (steamrigger precedent); the trigger's DoZoneInCombat arm IS PORTED
-- 2026-10-23 (summon:DoZoneInCombat() — the event-19 summon object is
-- the bearer);
-- ACTION_PREPARE_BROGGOK's DoCastSelf(SPELL_SUMMON_INCOMBAT_
-- TRIGGER 26837) is PORTED 2026-11-05 via the ev38 DoAction
-- bridge, called from the lever gossip handler below (grid twin
-- for the instance GetCreature lookup; the prepared-flag gate
-- twins the instance boss-state check); the ACTIVATE arm stays
-- out (instance OnUnitDeath caller, no bridge) and the RESET
-- arm with it (its immune legs would strand the boss with no
-- ACTIVATE twin); the RESET summons.DespawnAll arm is summon-
-- bridgeless (steamrigger precedent); the lever SetFlag(GO_FLAG_NOT_
-- SELECTABLE|GO_FLAG_IN_USE)/SetGoState arms are PORTED in the
-- gossip handler below (go:SetFlag/go:SetGoState bridged —
-- arlokk/netherspite precedents); go_broggok_lever
-- (GO_BROGGOK_LEVER 181982) is REGISTERED below; the
-- prisoner JustReachedHome -> DoAction(ACTION_RESET_BROGGOK)
-- relay is instance/cross-creature blocked; the instance
-- OnUnitDeath prisoner-counter -> ActivateCell chain and the
-- ResetPrisons/ResetPrisoners/ActivatePrisoners/
-- HandleGameObject arms are instance-gated; spell_broggok_
-- poison_cloud (AuraScript on 30914/38462) has no AuraScript
-- bridge (standing gap); no SpellScript scripts in this file.

local SPELL_SLIME_SPRAY = 30913
local SPELL_POISON_BOLT = 30917
local SPELL_POISON_CLOUD = 30916
local SPELL_POISON_CLOUD_PASSIVE = 30914
local SPELL_SUMMON_INCOMBAT_TRIGGER = 26837

local ACTION_PREPARE_BROGGOK = 3

local GO_BROGGOK_LEVER = 181982

local GAMEOBJECT_FLAGS = 9 -- arlokk precedent: OBJECT_END 6 + 3
local GO_FLAG_IN_USE = 1 -- SharedDefines.h:1637
local GO_FLAG_NOT_SELECTABLE = 16 -- SharedDefines.h:1641
local GO_STATE_ACTIVE = 0 -- SharedDefines.h:1661 (netherspite precedent)

local NPC_POISON_CLOUD = 17662
local NPC_INCOMBAT_TRIGGER = 16006

local REACT_PASSIVE = 0

local SAY_AGGRO = 0

local ENTRY_BROGGOK = 17380

local timers = {}
local states = {}
local prepared = {} -- per-broggok-GUID lever gate (see the lever handler below)

local function cancelPump(guid)
    local id = timers[guid]
    if id then
        RemoveEventById(id)
        timers[guid] = nil
    end
end

local function resetBoss(guid)
    cancelPump(guid)
    states[guid] = nil
    prepared[guid] = nil -- boss Reset (evade/spawn) re-arms the lever (C++ RESET lever reset)
end

-- C++ ACTION_ACTIVATE_BROGGOK schedule values: EVENT_SLIME_
-- SPRAY 10s, EVENT_POISON_BOLT 7s, EVENT_POISON_CLOUD 5s
-- (the C++ JustEngagedWith only talks — the activate DoAction
-- is unbridgeable, so the schedule lands on OnEnterCombat,
-- documented above).
local function freshState()
    return {
        slimeSpray = 10000,
        poisonBolt = 7000,
        poisonCloud = 5000,
    }
end

-- Pump CASTING gate: each expired arm carries `and not
-- creature:IsCasting()` — while casting it fires nothing and
-- consumes no re-arm (else-branch countdown only, timer stays
-- expired); unexpired arms keep counting down, since C++
-- events.Update runs ahead of the gate (BossAI::UpdateAI
-- ScriptedCreature.cpp:571-589, no boss override).
local function bossTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end

    -- Slime spray 30913: 10s init then 4000+rand32()%8000
    -- (C++ {4s,12s}) — non-triggered DoCastVictim (felmyst
    -- convention), re-arm {4s,12s} regardless (C++-exact).
    if st.slimeSpray <= 1000 and not creature:IsCasting() then
        creature:CastSpell(nil, SPELL_SLIME_SPRAY)
        st.slimeSpray = math.random(4000, 12000)
    else
        st.slimeSpray = st.slimeSpray - 1000
    end

    -- Poison bolt 30917: 7s init then {4s,12s} — non-triggered
    -- DoCastVictim, re-arm {4s,12s} regardless (C++-exact).
    if st.poisonBolt <= 1000 and not creature:IsCasting() then
        creature:CastSpell(nil, SPELL_POISON_BOLT)
        st.poisonBolt = math.random(4000, 12000)
    else
        st.poisonBolt = st.poisonBolt - 1000
    end

    -- Poison cloud 30916: 5s init then 20s — non-triggered
    -- DoCastSelf (C++ DoCast(me), C++-exact), re-arm 20s
    -- regardless (C++-exact). The spell's SPELL_EFFECT_SUMMON leg
    -- spawns NPC_BROGGOK_POISON_CLOUD (17662) through the creature
    -- cast path's EffectSummonType twin, which fires event 19 —
    -- the cloud's JustSummoned arm is ported in onJustSummoned below.
    if st.poisonCloud <= 1000 and not creature:IsCasting() then
        creature:CastSpell(creature, SPELL_POISON_CLOUD)
        st.poisonCloud = 20000
    else
        st.poisonCloud = st.poisonCloud - 1000
    end
end

-- C++ boss_broggokAI::JustSummoned (boss_broggok.cpp:76-90):
-- the poison cloud's SPELL_EFFECT_SUMMON leg spawns 17662 through
-- the creature cast path's EffectSummonType twin, which fires
-- event 19 on the summoner (Eluna::JustSummoned, C++-exact).
-- Cloud arm: SetReactState(REACT_PASSIVE) + triggered self-cast of
-- SPELL_POISON_CLOUD_PASSIVE 30914 (C++-exact; the C++ summons.Summon
-- list has no bridge — steamrigger precedent). In-combat trigger arm
-- (entry 16006, from SPELL_SUMMON_INCOMBAT_TRIGGER 26837 — fires only
-- via the instance-gated ACTION_PREPARE_BROGGOK cast, documented
-- above): SetReactState(REACT_PASSIVE) ported; the C++ DoZoneInCombat
-- arm is PORTED below (bridge live 2026-10-23 — the stale "no bridge"
-- claim is retracted) and the summons.Summon list has no bridge.
local function onJustSummoned(_, creature, summon)
    local entry = summon:GetEntry()
    if entry == NPC_POISON_CLOUD then
        summon:SetReactState(REACT_PASSIVE)
        summon:CastSpell(summon, SPELL_POISON_CLOUD_PASSIVE, true)
    elseif entry == NPC_INCOMBAT_TRIGGER then
        summon:SetReactState(REACT_PASSIVE)
        summon:DoZoneInCombat() -- boss_broggok.cpp:87 (JustSummoned arm 2, C++ order after SetReactState; the summons.Summon list has no bridge)
    end
end

RegisterCreatureEvent(ENTRY_BROGGOK, 19, onJustSummoned)

-- C++ boss_broggokAI::DoAction (boss_broggok.cpp:113-143), PREPARE
-- arm only (3): DoCastSelf(SPELL_SUMMON_INCOMBAT_TRIGGER 26837)
-- — the trigger's JustSummoned arm is ported above (event 19).
-- ACTIVATE (1) has no Lua caller (the instance OnUnitDeath
-- prisoner-counter chain, instance_blood_furnace.cpp:306, has no
-- bridge) and RESET (2) stays out with it: its react/flag/immune
-- legs (SetFlag NON_ATTACKABLE + SetImmuneToAll(true), mandokir
-- SetFlag(59, 0x300) twin) would strand the boss unpullable with
-- no ACTIVATE twin to clear them, and the prisoner
-- JustReachedHome caller is instance-gated on boss IN_PROGRESS.
local function broggokDoAction(event, creature, action)
    if action == ACTION_PREPARE_BROGGOK then
        creature:CastSpell(creature, SPELL_SUMMON_INCOMBAT_TRIGGER)
    end
end

RegisterCreatureEvent(ENTRY_BROGGOK, 38, broggokDoAction)

-- C++ go_broggok_leverAI::OnGossipHello (boss_broggok.cpp:251-268):
-- when the boss state is not DONE/IN_PROGRESS, SetBossState
-- IN_PROGRESS + broggok DoAction(PREPARE); the lever always takes
-- GO_FLAG_NOT_SELECTABLE|GO_FLAG_IN_USE + GO_STATE_ACTIVE. The
-- instance state gate has no bridge — the per-boss prepared flag
-- is its twin (set when PREPARE fires, cleared on boss Reset,
-- re-blocked on boss death = DONE). The lever is adjacent to the
-- boss cell, so the grid twin (unique 17380) finds him.
local function onBroggokLeverGossip(event, player, go)
    local broggok = player:FindNearestCreature(ENTRY_BROGGOK, 100.0)
    if broggok and not prepared[broggok:GetGUID()] then
        prepared[broggok:GetGUID()] = true
        broggok:DoAction(ACTION_PREPARE_BROGGOK)
    end
    go:SetFlag(GAMEOBJECT_FLAGS, GO_FLAG_IN_USE + GO_FLAG_NOT_SELECTABLE)
    go:SetGoState(GO_STATE_ACTIVE)
    return true
end

RegisterGameObjectGossipEvent(GO_BROGGOK_LEVER, 1, onBroggokLeverGossip)

RegisterCreatureEvent(ENTRY_BROGGOK, 1, function(_, creature)
    local guid = creature:GetGUID()
    resetBoss(guid)
    states[guid] = freshState()
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542; sapphiron precedent)
    creature:Talk(SAY_AGGRO)
    timers[guid] = CreateLuaEvent(function()
        bossTick(creature, guid)
    end, 1000, 0)
end)

RegisterCreatureEvent(ENTRY_BROGGOK, 2, function(_, creature)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_BROGGOK, 4, function(_, creature)
    local guid = creature:GetGUID()
    resetBoss(guid)
    prepared[guid] = true -- boss death = DONE: the lever stays blocked (C++ OnGossipHello gate)
end)

RegisterCreatureEvent(ENTRY_BROGGOK, 23, function(_, creature)
    resetBoss(creature:GetGUID())
end)
