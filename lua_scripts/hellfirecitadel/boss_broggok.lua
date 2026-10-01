-- Broggok (Blood Furnace, Hellfire Citadel) — Lua port of
-- src/server/scripts/Outland/HellfireCitadel/BloodFurnace/
-- boss_broggok.cpp (boss_broggok — the only CreatureScript AI
-- class AddSC_boss_broggok registers; npc_nascent_fel_orc
-- and npc_fel_orc_neophyte are the two prisoner AI structs
-- registered alongside in this file; go_broggok_lever is a
-- GameObjectScript — no GO bridge, documented only, not
-- registered; spell_broggok_poison_cloud is an AuraScript —
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
-- early return collapses into the pump). Pump: slime spray
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
-- instance-script model; the whole lever-gossip ->
-- SetBossState IN_PROGRESS -> DoAction(ACTION_PREPARE_
-- BROGGOK) -> prisoner-wave cell release -> all-prisoners-
-- dead -> ActivateCell(DATA_DOOR_4) -> DoAction(ACTION_
-- ACTIVATE_BROGGOK) choreography is instance/GO/
-- cross-creature gated); the C++ JustEngagedWith does NOT
-- schedule the events — the schedule fires only from the
-- unbridgeable ACTIVATE DoAction — so the activate schedule
-- is applied at the only engagement hook this model has,
-- OnEnterCombat(1) (Talk(SAY_AGGRO) stays on engage,
-- C++-exact); no flag/react/immune bridges — the ACTIVATE
-- arm's SetReactState(REACT_AGGRESSIVE), RemoveFlag(NON_
-- ATTACKABLE), SetImmuneToAll(false) and the RESET arm's
-- mirror flips unmodeled (thespia/kalithresh precedent); no
-- DoZoneInCombat bridge — the ACTIVATE DoZoneInCombat arm
-- unmodeled; no summon bridge — the JustSummoned arms for
-- NPC_BROGGOK_POISON_CLOUD 17662 (react passive + CastSpell
-- SPELL_POISON_CLOUD_PASSIVE 30914 + summons.Summon) and
-- NPC_INCOMBAT_TRIGGER 16006 (react passive + DoZoneInCombat
-- + summons.Summon) unmodeled (steamrigger precedent);
-- ACTION_PREPARE_BROGGOK's DoCastSelf(SPELL_SUMMON_INCOMBAT_
-- TRIGGER 26837) fires only cross-creature from the lever
-- gossip — unreachable in this model, documented only; the
-- RESET summons.DespawnAll arm is summon-bridgeless
-- (steamrigger precedent); the lever SetFlag(GO_FLAG_NOT_
-- SELECTABLE|GO_FLAG_IN_USE)/SetGoState/RemoveFlag arms have
-- no GO bridge — go_broggok_lever (GO_BROGGOK_LEVER 181982)
-- documented only, not registered (ahune precedent); the
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

local SAY_AGGRO = 0

local ENTRY_BROGGOK = 17380

local timers = {}
local states = {}

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

local function bossTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end

    -- Slime spray 30913: 10s init then 4000+rand32()%8000
    -- (C++ {4s,12s}) — non-triggered DoCastVictim (felmyst
    -- convention), re-arm {4s,12s} regardless (C++-exact).
    if st.slimeSpray <= 1000 then
        creature:CastSpell(nil, SPELL_SLIME_SPRAY)
        st.slimeSpray = math.random(4000, 12000)
    else
        st.slimeSpray = st.slimeSpray - 1000
    end

    -- Poison bolt 30917: 7s init then {4s,12s} — non-triggered
    -- DoCastVictim, re-arm {4s,12s} regardless (C++-exact).
    if st.poisonBolt <= 1000 then
        creature:CastSpell(nil, SPELL_POISON_BOLT)
        st.poisonBolt = math.random(4000, 12000)
    else
        st.poisonBolt = st.poisonBolt - 1000
    end

    -- Poison cloud 30916: 5s init then 20s — non-triggered
    -- DoCastSelf (C++ DoCast(me), C++-exact), re-arm 20s
    -- regardless (C++-exact). The summoned NPC_BROGGOK_POISON_
    -- CLOUD (17662) JustSummoned arms are summon-bridgeless.
    if st.poisonCloud <= 1000 then
        creature:CastSpell(creature, SPELL_POISON_CLOUD)
        st.poisonCloud = 20000
    else
        st.poisonCloud = st.poisonCloud - 1000
    end
end

RegisterCreatureEvent(ENTRY_BROGGOK, 1, function(_, creature)
    local guid = creature:GetGUID()
    resetBoss(guid)
    states[guid] = freshState()
    creature:Talk(SAY_AGGRO)
    timers[guid] = CreateLuaEvent(function()
        bossTick(creature, guid)
    end, 1000, 0)
end)

RegisterCreatureEvent(ENTRY_BROGGOK, 2, function(_, creature)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_BROGGOK, 4, function(_, creature)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_BROGGOK, 23, function(_, creature)
    resetBoss(creature:GetGUID())
end)
