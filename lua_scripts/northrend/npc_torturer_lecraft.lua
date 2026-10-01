-- Torturer LeCraft (Dragonblight) --
-- Lua port of src/server/scripts/Northrend/zone_dragonblight.cpp
-- (npc_torturer_lecraftAI — JustEngagedWith schedule + UpdateAI
-- combat rotation arms only). Zone-script unit per
-- northrend_script_loader.cpp order (borean_tundra done;
-- dragonblight: npc_commander_eligor_dawnbringer, spell_q12096_
-- q12092_dummy, spell_q12096_q12092_bark, npc_wyrmrest_defender
-- documented-only, npc_torturer_lecraft ported).
-- Entry (TorturerLeCraft enum, verifiable from the C++ sources):
-- 27394 (NPC_TORTURER_LECRAFT). The creature_template ScriptName
-- binding is DB-side (no TDB in this workspace).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat,
-- 4 OnDied, 23 OnReset (the C++ overrides no DamageTaken /
-- MovementInform arms — no events 9/14; the SpellHit arm has no
-- SpellHit bridge — fizzule precedent — unmodeled).
-- Timers via a 1s CreateLuaEvent pump (a port of UpdateAI's combat
-- section in C++ arm order — 1s granularity exact for all C++
-- timers; the !UpdateVictim early return collapses into the pump,
-- which only runs in combat). The C++ HasUnitState(UNIT_STATE_
-- CASTING) pump-skip has no state bridge (twilight_corrupter
-- precedent) — unmodeled.
-- C++ DoCastVictim default is triggered=false (vaelastrasz
-- convention): creature:CastSpell(nil, spell) = DoCastVictim
-- (boss_twilight_corrupter.lua comment).
-- Fight shape (C++-exact for all modeled arms):
-- OnEnterCombat(1): per-GUID latch of the C++ JustEngagedWith
-- schedule — hemorrhage 5-8s, kidney shot 12-15s (ScheduleEvent
-- urand ranges, C++-exact), Talk(SAY_AGGRO 0) — the C++ Talk
-- targets the engaging player; the Go Talk bridge takes a textId
-- only (broadcast to nearby, C++-exact for the line played) —
-- documented deviation — + 1s scheduler pump (events.Reset in
-- the C++ latch — the pump is cancelled first, gargolmar
-- precedent).
-- Pump (C++ UpdateAI arm order): hemorrhage arm —
-- DoCastVictim(SPELL_HEMORRHAGE 30478), re-arm urand(12s, 168s)
-- (C++-exact range); kidney-shot arm —
-- DoCastVictim(SPELL_KIDNEY_SHOT 30621), re-arm urand(20s, 26s)
-- (C++-exact). Melee is engine-driven.
-- OnReset(23): per-GUID state drop (C++ Reset: _textCounter = 1
-- + _playerGUID.Clear() — the SpellHit half is unmodeled, so
-- the drop collapses to the pump cancel + state table clear,
-- C++-equivalent). OnLeaveCombat(2) and OnDied(4): cancel the
-- pump + drop per-GUID state (mushroom precedent; the C++
-- Reset path covers both).
-- npc_commander_eligor_dawnbringer — the whole Naxxramas-wings
-- talk machine (EVENT_START_RANDOM 20s Talk(talkWing) + 8s
-- MovePoint + MovementInform change-image Talk/SAY_ chain +
-- EVENT_GET_TARGETS 5s StoreTargets GetCreatureListWithEntryInGrid
-- / FindNearestCreature + ChangeImage SetEntry/SetDisplayId +
-- SPELL_HEROIC_IMAGE_CHANNEL cast + TurnAudience SetFacingToObject)
-- sits behind the no-movement / no-world-search / no-entry-update /
-- no-cross-creature-facing bridges — documented-only.
-- spell_q12096_q12092_dummy — Strengthen the Ancients On Interact
-- Dummy to Woodlands Walker (roll 1: CastSpell-on-player 47550
-- item bark + Talk SAY_WALKER_FRIENDLY + DespawnOrUnsummon 1s;
-- roll 0: Talk SAY_WALKER_ENEMY + SetFaction(FACTION_MONSTER) +
-- Attack) has no SpellScript bridge anywhere in the model
-- (blasted_lands / gordunni precedents) — documented-only.
-- spell_q12096_q12092_bark — Bark of the Walkers (Lothalor
-- entry-gated Talk SAY_LOTHALOR + RemoveAura(52405 confused) +
-- DespawnOrUnsummon 4s) has no SpellScript bridge — documented-only.
-- npc_wyrmrest_defender (VehicleAI) — UpdateAI low-hp arm
-- (GetHealthPct() <= 30% -> CastSpell(me, 52421), 20s renew
-- recovery re-arm gate) sits behind the unmodeled VehicleAI
-- base UpdateAI pass and there is no NPC_ entry constant in the
-- C++ sources for the defender itself (entry DB-side only,
-- unregistered file = dead code — minigob precedent); the
-- SpellHit arms (49256 mount / 52421 low-hp emote / 49263 renew
-- talk machines) have no SpellHit bridge, the OnGossipSelect
-- quest-12372 arm has no gossip bridge, the OnCharmed flag arm
-- has no flag bridge — documented-only.

local SPELL_HEMORRHAGE = 30478
local SPELL_KIDNEY_SHOT = 30621

local TORTURER_LECRAFT_ENTRY = 27394

local SAY_AGGRO = 0

local lecraftState = {}
local lecraftPump = {}

local TIMER_KEYS = { "hemorrhageTimer", "kidneyShotTimer" }

local function cancelPump(guid)
    local id = lecraftPump[guid]
    if id then
        RemoveEventById(id)
        lecraftPump[guid] = nil
    end
end

local function fullReset(guid)
    cancelPump(guid)
    lecraftState[guid] = nil
end

-- C++ UpdateAI combat section in arm order — 1s granularity
-- exact for all C++ timers; the !UpdateVictim early return
-- collapses into the pump (it only runs in combat). Each
-- tick decrements every timer (the EventMap::Update(diff)
-- pass), then expired events fire in C++ arm order.
local function combatTick(creature, guid)
    local st = lecraftState[guid]
    if not st then
        return
    end

    for _, key in ipairs(TIMER_KEYS) do
        st[key] = st[key] - 1000
    end

    -- Hemorrhage arm — non-triggered DoCastVictim(SPELL_
    -- HEMORRHAGE 30478), re-arm urand(12s, 168s) (C++-exact).
    if st.hemorrhageTimer <= 0 then
        creature:CastSpell(nil, SPELL_HEMORRHAGE)
        st.hemorrhageTimer = math.random(12000, 168000)
    end

    -- Kidney-shot arm — non-triggered DoCastVictim(SPELL_
    -- KIDNEY_SHOT 30621), re-arm urand(20s, 26s) (C++-exact).
    if st.kidneyShotTimer <= 0 then
        creature:CastSpell(nil, SPELL_KIDNEY_SHOT)
        st.kidneyShotTimer = math.random(20000, 26000)
    end
end

-- C++ JustEngagedWith schedule values land on OnEnterCombat:
-- hemorrhage 5-8s, kidney shot 12-15s (C++ urand ranges,
-- C++-exact), Talk(SAY_AGGRO 0) — the Go Talk bridge takes a
-- textId only, so the victim targeting collapses to the nearby
-- broadcast (line still plays, C++-exact) — + 1s scheduler
-- pump (events.Reset in the C++ latch — the pump is cancelled
-- first, gargolmar precedent).
local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelPump(guid)
    creature:Talk(SAY_AGGRO)
    lecraftState[guid] = {
        hemorrhageTimer = math.random(5000, 8000),
        kidneyShotTimer = math.random(12000, 15000),
    }
    lecraftPump[guid] = CreateLuaEvent(function()
        combatTick(creature, guid)
    end, 1000, 0)
end

-- C++ Reset(): per-GUID state drop is C++-equivalent (the
-- _textCounter / _playerGUID half served the unmodeled SpellHit
-- arm). The C++ death path runs Reset too, so OnDied lands
-- here as well (mushroom precedent).
local function onReset(_, creature)
    fullReset(creature:GetGUID())
end

RegisterCreatureEvent(TORTURER_LECRAFT_ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(TORTURER_LECRAFT_ENTRY, 2, onReset)
RegisterCreatureEvent(TORTURER_LECRAFT_ENTRY, 4, onReset)
RegisterCreatureEvent(TORTURER_LECRAFT_ENTRY, 23, onReset)
