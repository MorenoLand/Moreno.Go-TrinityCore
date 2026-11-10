-- Broggok (Blood Furnace, Hellfire Citadel) — Lua port of
-- src/server/scripts/Outland/HellfireCitadel/BloodFurnace/
-- boss_broggok.cpp (boss_broggok — the only CreatureScript AI
-- class AddSC_boss_broggok registers; npc_nascent_fel_orc
-- and npc_fel_orc_neophyte are the two prisoner AI structs
-- registered alongside in this file — ported in
-- npc_nascent_fel_orc.lua / npc_fel_orc_neophyte.lua; go_broggok_lever is a
-- GameObjectScript — gossip handler PORTED below (ev38 PREPARE
-- arm + lever flag/state legs); spell_broggok_poison_cloud is an AuraScript —
-- PeriodicTick PORTED below (ev1 + CastSpellWithRadiusMod).
-- First boss in the Blood Furnace set per
-- outland_script_loader.cpp order (instance_blood_furnace.
-- lua ports the prisoner machine — see below).
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
-- (17380): OnEnterCombat(1): Talk(SAY_AGGRO) only (C++
-- JustEngagedWith :71-75) — the ACTION_ACTIVATE_BROGGOK
-- schedule (slimeSpray 10000 / poisonBolt 7000 /
-- poisonCloud 5000) seeds from the ev38 ACTIVATE arm below
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
-- OnDied(4): cleanup + SetBossState(DATA_BROGGOK, DONE)
-- (the BossAI _JustDied arm).
-- OnLeaveCombat(2): cancel the pump, drop per-GUID state.
-- OnReset(23): the Reset() -> DoAction(ACTION_RESET_BROGGOK)
-- machine — react/flag/immune flips, SetBossState(
-- DATA_BROGGOK, NOT_STARTED), lever GO reset, ResetPrisons
-- (summons.DespawnAll stays out, no bridge — steamrigger
-- precedent).
-- Deliberate deviations (all await engine bridges): the
-- instance prisoner machine is PORTED in instance_blood_
-- furnace.lua (RegisterMapEvent(542, 9) OnUnitDeath counter
-- chain -> ActivateCell -> DoAction(ACTION_ACTIVATE_
-- BROGGOK); the C++ OnCreatureCreate cell-store delta is
-- documented there — counts are built by a lever-time grid
-- sweep); SetReactState IS bridged (lua_creature_events.go:1197 —
-- stale "no react bridge" retracted); the NON_ATTACKABLE flag
-- flips and the SetImmuneToAll(false/true) flag legs are
-- PORTED in the ACTIVATE/RESET arms (the SetFlag/RemoveFlag(59)
-- surface itself IS bridged, lua_creature_events.go — the stale
-- 'no flag/immune bridges' claim is retracted, 2026-10-23); the
-- ACTIVATE DoZoneInCombat arm IS PORTED (bridge live
-- 2026-10-23 — the stale "no bridge" claim is retracted);
-- the JustSummoned arms for NPC_BROGGOK_POISON_CLOUD
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
-- twins the instance boss-state check); the ACTIVATE arm is
-- PORTED via the ev38 receiver (sender: the instance
-- OnUnitDeath chain) and the RESET arm with it (the RESET
-- summons.DespawnAll arm is summon-bridgeless (steamrigger
-- precedent)); the lever SetFlag(GO_FLAG_NOT_
-- SELECTABLE|GO_FLAG_IN_USE)/SetGoState arms are PORTED in the
-- gossip handler below (go:SetFlag/go:SetGoState bridged —
-- arlokk/netherspite precedents); go_broggok_lever
-- (GO_BROGGOK_LEVER 181982) is REGISTERED below; the
-- prisoner JustReachedHome -> DoAction(ACTION_RESET_BROGGOK)
-- relay is PORTED in the prisoner files (ev2, gated on boss
-- IN_PROGRESS); spell_broggok_
-- poison_cloud (AuraScript on 30914/38462) PeriodicTick is PORTED
-- below (ev1 + CastSpellWithRadiusMod); no SpellScript scripts in this file.

local SPELL_SLIME_SPRAY = 30913
local SPELL_POISON_BOLT = 30917
local SPELL_POISON_CLOUD = 30916
local SPELL_POISON_CLOUD_PASSIVE = 30914
local SPELL_SUMMON_INCOMBAT_TRIGGER = 26837

local ACTION_ACTIVATE_BROGGOK = 1 -- blood_furnace.h:84
local ACTION_RESET_BROGGOK = 2 -- blood_furnace.h:85
local ACTION_PREPARE_BROGGOK = 3

local DATA_BROGGOK = 1 -- blood_furnace.h:29
local BOSS_STATE_NOT_STARTED = 0
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_DONE = 3

local UNIT_FIELD_FLAGS = 59 -- arlokk precedent
local UNIT_FLAG_NON_ATTACKABLE = 0x2 -- bosses_opera.lua:103

local GO_BROGGOK_LEVER = 181982

local GAMEOBJECT_FLAGS = 9 -- arlokk precedent: OBJECT_END 6 + 3
local GO_FLAG_IN_USE = 1 -- SharedDefines.h:1637
local GO_FLAG_NOT_SELECTABLE = 16 -- SharedDefines.h:1641
local GO_STATE_ACTIVE = 0 -- SharedDefines.h:1661 (netherspite precedent)
local GO_STATE_READY = 1 -- GameObjectMethods.h:91

local NPC_POISON_CLOUD = 17662
local NPC_INCOMBAT_TRIGGER = 16006

local REACT_PASSIVE = 0
local REACT_AGGRESSIVE = 2 -- boss_mr_smite.lua:89

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
    -- C++ UpdateVictim gate (BossAI::UpdateAI): no victim, no
    -- casts — the ACTIVATE-seeded pump can tick before the
    -- DoZoneInCombat engage lands.
    if not creature:IsInCombat() then
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

-- C++ boss_broggokAI::DoAction (boss_broggok.cpp:113-143),
-- all three arms PORTED: PREPARE (3) DoCastSelf(
-- SPELL_SUMMON_INCOMBAT_TRIGGER 26837) — the trigger's
-- JustSummoned arm is ported above (event 19); ACTIVATE (1)
-- SetReactState(REACT_AGGRESSIVE) + RemoveFlag
-- NON_ATTACKABLE + SetImmuneToAll(false) + DoZoneInCombat
-- + the slime/bolt/cloud schedule (10s/7s/5s) — the
-- instance OnUnitDeath prisoner-counter chain that sends
-- it rides RegisterMapEvent(542, 9) in
-- instance_blood_furnace.lua (the old "no bridge" note is
-- stale: triggerInstanceUnitDeath, instance_hooks.go:525);
-- RESET (2) SetReactState(REACT_PASSIVE) + SetFlag
-- NON_ATTACKABLE + SetImmuneToAll(true) + summons.
-- DespawnAll (no bridge, steamrigger precedent) +
-- SetBossState(DATA_BROGGOK, NOT_STARTED) + lever GO reset
-- + ResetPrisons via the instance file (guarded: the
-- instance file loads after this one in WalkDir order).
-- The RESET legs run from Reset() too (C++ :66-70), so
-- the ev23 handler below shares resetBroggok.
local function resetBroggok(creature)
    local guid = creature:GetGUIDLow()
    resetBoss(guid)
    creature:SetReactState(REACT_PASSIVE)
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
    creature:SetImmuneToAll(true)
    creature:SetBossState(DATA_BROGGOK, BOSS_STATE_NOT_STARTED)
    local lever = creature:FindNearestGameObject(GO_BROGGOK_LEVER, 100)
    if lever then
        lever:RemoveFlag(GAMEOBJECT_FLAGS, GO_FLAG_IN_USE + GO_FLAG_NOT_SELECTABLE)
        lever:SetGoState(GO_STATE_READY)
    end
    if BloodFurnace then
        BloodFurnace.resetPrisons(creature)
    end
end

local function broggokDoAction(event, creature, action)
    if action == ACTION_PREPARE_BROGGOK then
        creature:CastSpell(creature, SPELL_SUMMON_INCOMBAT_TRIGGER)
    elseif action == ACTION_ACTIVATE_BROGGOK then
        creature:SetReactState(REACT_AGGRESSIVE)
        creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
        creature:SetImmuneToAll(false)
        local guid = creature:GetGUIDLow()
        states[guid] = freshState()
        timers[guid] = CreateLuaEvent(function()
            bossTick(creature, guid)
        end, 1000, 0)
        creature:DoZoneInCombat()
    elseif action == ACTION_RESET_BROGGOK then
        resetBroggok(creature)
    end
end

RegisterCreatureEvent(ENTRY_BROGGOK, 38, broggokDoAction)

-- C++ go_broggok_leverAI::OnGossipHello (boss_broggok.cpp:251-268):
-- when the boss state is not DONE/IN_PROGRESS, SetBossState
-- IN_PROGRESS + broggok DoAction(PREPARE); the lever always takes
-- GO_FLAG_NOT_SELECTABLE|GO_FLAG_IN_USE + GO_STATE_ACTIVE. The
-- SetBossState leg is the instance override's ActivateCell(5)
-- trigger (instance_blood_furnace.cpp:156-161), invoked
-- explicitly here via the instance file (no Lua
-- instance-override hook exists); the per-boss prepared flag
-- twins the C++ boss-state gate. The lever is adjacent to the
-- boss cell, so the grid twin (unique 17380) finds him.
local function onBroggokLeverGossip(event, player, go)
    local broggok = player:FindNearestCreature(ENTRY_BROGGOK, 100.0)
    if broggok and not prepared[broggok:GetGUIDLow()] then
        prepared[broggok:GetGUIDLow()] = true
        broggok:SetBossState(DATA_BROGGOK, BOSS_STATE_IN_PROGRESS)
        if BloodFurnace then
            BloodFurnace.beginEncounter(broggok)
            BloodFurnace.activateCell(broggok, 5)
        end
        broggok:DoAction(ACTION_PREPARE_BROGGOK)
    end
    go:SetFlag(GAMEOBJECT_FLAGS, GO_FLAG_IN_USE + GO_FLAG_NOT_SELECTABLE)
    go:SetGoState(GO_STATE_ACTIVE)
    return true
end

RegisterGameObjectGossipEvent(GO_BROGGOK_LEVER, 1, onBroggokLeverGossip)

-- C++ boss_broggokAI::JustEngagedWith (boss_broggok.cpp:71-75)
-- only Talks (SAY_AGGRO) — the slime/bolt/cloud schedule
-- fires from DoAction(ACTION_ACTIVATE_BROGGOK) (:120-128),
-- seeded there; ev1 seeds only if no state exists (a
-- direct pull while the ACTIVATE arm never ran — impossible
-- once the spawn flags land, kept as the fallback).
RegisterCreatureEvent(ENTRY_BROGGOK, 1, function(_, creature)
    local guid = creature:GetGUIDLow()
    if not states[guid] then
        states[guid] = freshState()
        timers[guid] = CreateLuaEvent(function()
            bossTick(creature, guid)
        end, 1000, 0)
    end
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542; sapphiron precedent)
    creature:Talk(SAY_AGGRO)
end)

RegisterCreatureEvent(ENTRY_BROGGOK, 2, function(_, creature)
    resetBoss(creature:GetGUIDLow())
end)

RegisterCreatureEvent(ENTRY_BROGGOK, 4, function(_, creature)
    local guid = creature:GetGUIDLow()
    resetBoss(guid)
    prepared[guid] = true -- boss death = DONE: the lever stays blocked (C++ OnGossipHello gate)
    creature:SetBossState(DATA_BROGGOK, BOSS_STATE_DONE) -- BossAI::_JustDied arm (ScriptedCreature.cpp)
end)

RegisterCreatureEvent(ENTRY_BROGGOK, 23, function(_, creature)
    resetBroggok(creature)
end)

-- C++ spell_broggok_poison_cloud::PeriodicTick (boss_broggok.cpp:293-304):
-- EFFECT_0 SPELL_AURA_PERIODIC_TRIGGER_SPELL on 30914 (normal) /
-- 38462 (heroic). PreventDefaultAction, so the Go default
-- trigger-spell cast is skipped (return true); a zero total-tick
-- count returns early (C++ GetTotalTicks() guard). The trigger spell
-- id comes from the aura's own Effects[0].TriggerSpell (DBC-side,
-- read via aura:GetTriggerSpell(0); 0 without Spell.dbc means no
-- cast). The radius mod grows with the tick: ((tick/total) * 0.9 +
-- 0.1) * 10000 * 2/3 (C++ int32 truncation). The cloud trigger
-- (NPC_POISON_CLOUD 17662, which self-casts 30914 in JustSummoned)
-- casts the trigger spell on itself (C++ CastSpell(nullptr, ...) —
-- self-centered area, DoCastAOE convention). DORMANT until Spell.dbc
-- ships: the trigger spell id and the 30914 aura itself are DBC-side.
local SPELL_POISON_CLOUD_PASSIVE_HEROIC = 38462

local function poisonCloudTick(event, aura, target)
    local totalTicks = aura:GetTotalTicks()
    if totalTicks == 0 then
        return true
    end
    local triggerSpell = aura:GetTriggerSpell(0)
    if triggerSpell == 0 then
        return true
    end
    local mod = math.floor(((aura:GetTickNumber() / totalTicks) * 0.9 + 0.1) * 10000 * 2 / 3)
    target:CastSpellWithRadiusMod(target, triggerSpell, true, mod)
    return true
end

RegisterAuraEvent(SPELL_POISON_CLOUD_PASSIVE, 1, poisonCloudTick)
RegisterAuraEvent(SPELL_POISON_CLOUD_PASSIVE_HEROIC, 1, poisonCloudTick)
