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
-- 4 OnDied, 23 OnReset, 14 OnHitBySpell (the C++ overrides no
-- DamageTaken / MovementInform arms — no event 9; event 14
-- fires with Eluna arg order (attumen precedent)).
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
-- urand ranges, C++-exact); Talk(SAY_AGGRO 0, player) gated on
-- the engaging unit being a player (who->ToPlayer()) with Talk's
-- optional whisper-target arg (whisperTarget arm, C++-exact) —
-- + 1s scheduler pump (events.Reset in the C++ latch — the pump
-- is cancelled first, gargolmar precedent).
-- Pump (C++ UpdateAI arm order): hemorrhage arm —
-- DoCastVictim(SPELL_HEMORRHAGE 30478), re-arm urand(12s, 168s)
-- (C++-exact range); kidney-shot arm —
-- DoCastVictim(SPELL_KIDNEY_SHOT 30621), re-arm urand(20s, 26s)
-- (C++-exact). Melee is engine-driven.
-- OnReset(23): per-GUID state drop (C++ Reset: _textCounter = 1
-- + _playerGUID.Clear() — a fresh state re-latches on the next
-- hit, C++-equivalent) + pump cancel. OnLeaveCombat(2) and
-- OnDied(4): cancel the pump + drop per-GUID state (mushroom
-- precedent; the C++ Reset path covers both).
-- npc_commander_eligor_dawnbringer — the whole Naxxramas-wings
-- talk machine (EVENT_START_RANDOM 20s Talk(talkWing) + 8s
-- MovePoint + MovementInform change-image Talk/SAY_ chain +
-- EVENT_GET_TARGETS 5s StoreTargets GetCreatureListWithEntryInGrid
-- / FindNearestCreature + ChangeImage SetEntry/SetDisplayId +
-- SPELL_HEROIC_IMAGE_CHANNEL cast + TurnAudience SetFacingToObject —
-- the TurnAudience facing leg (creature->SetFacingToObject(me),
-- C++ :290) is bridgeable via the atan2 twin (devourer_of_souls
-- precedent), retracting the old "no-cross-creature-facing"
-- parenthetical; the machine stays documented-only — the payload
-- legs ride the no-movement / no-world-search /
-- no-entry-update / no-summon bridges.
-- spell_q12096_q12092_dummy — Strengthen the Ancients On Interact
-- Dummy to Woodlands Walker (roll 1: CastSpell-on-player 47550
-- item bark + Talk SAY_WALKER_FRIENDLY + DespawnOrUnsummon 1s;
-- roll 0: Talk SAY_WALKER_ENEMY + SetFaction(FACTION_MONSTER) +
-- Attack) has no SpellScript bridge anywhere in the model
-- (blasted_lands / gordunni precedents) — documented-only.
-- spell_q12096_q12092_bark — Bark of the Walkers (Lothalor
-- entry-gated Talk SAY_LOTHALOR + RemoveAura(SPELL_CONFUSED 47044) +
-- DespawnOrUnsummon 4s) has no SpellScript bridge — documented-only.
-- npc_wyrmrest_defender (VehicleAI) — UpdateAI low-hp arm
-- (GetHealthPct() <= 30% -> CastSpell(me, 52421), 20s renew
-- recovery re-arm gate) sits behind the unmodeled VehicleAI
-- base UpdateAI pass; the defender has no NPC_ entry constant in
-- the C++ sources (entry DB-side only) but IS registered —
-- AddSC_dragonblight() (zone_dragonblight.cpp:690-696, loader call
-- 395) — so the old "unregistered file = dead code" parenthetical
-- is retracted (2026-10-23 SetImmuneToAll sweep); the
-- SpellHit arms (49256 mount / 52421 low-hp emote / 49263 renew
-- talk machines): the 'no SpellHit bridge' claim is stale (ev14
-- fires — this file registers it below); the arms stay out on the
-- VehicleAI / quest-vehicle-machine grounds (the whole defender
-- script is documented-only, no Lua handler is registered for the
-- defender's entry); the OnGossipSelect
-- quest-12372 arm: stale 'no gossip bridge' retracted (bridged —
-- RegisterCreatureGossipEvent); arm stays out on the no
-- player:CastSpell bridge (kalecgos pass).

local SPELL_HEMORRHAGE = 30478
local SPELL_KIDNEY_SHOT = 30621
local SPELL_HIGH_EXECUTORS_BRANDING_IRON = 48603

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
-- C++-exact); Talk(SAY_AGGRO 0) only when the engaging unit is
-- a player (who->ToPlayer()), with Talk's optional whisper-
-- target arg (C++-exact) — + 1s scheduler pump (events.Reset
-- in the C++ latch — the pump is cancelled first, gargolmar
-- precedent).
local function onEnterCombat(_, creature, target)
    local guid = creature:GetGUID()
    cancelPump(guid)
    if target and target:GetObjectType() == "Player" then
        creature:Talk(SAY_AGGRO, target)
    end
    -- C++ JustEngagedWith (zone_dragonblight.cpp:617-624) re-schedules
    -- the two timers and Talks but does NOT touch _textCounter /
    -- _playerGUID (only Reset() clears them) — preserve any pre-combat
    -- SpellHit latch state across combat entry.
    local st = lecraftState[guid] or {}
    st.hemorrhageTimer = math.random(5000, 8000)
    st.kidneyShotTimer = math.random(12000, 15000)
    st.textCounter = st.textCounter or 1 -- C++ ctor value; the SpellHit ladder latches here
    lecraftState[guid] = st
    lecraftPump[guid] = CreateLuaEvent(function()
        combatTick(creature, guid)
    end, 1000, 0)
end

-- C++ Reset(): per-GUID state drop is C++-equivalent (Reset
-- sets _textCounter = 1 and clears _playerGUID; a fresh state
-- re-latches on the next hit). The C++ death path runs Reset
-- too, so OnDied lands here as well (mushroom precedent).
local function onReset(_, creature)
    fullReset(creature:GetGUID())
end

-- C++ SpellHit arm (:628-650) — the High Executor's Branding
-- Iron (48603) talk ladder. The first qualifying hit latches
-- the caster's GUID (textCounter starts 1, the C++ ctor value);
-- only the latched caster's subsequent hits advance the ladder.
-- Talk(textCounter, player) passes the latched player as Talk's
-- whisper-target arg (C++-exact); KilledMonsterCredit(
-- NPC_TORTURER_LECRAFT 27394) at counter 5 has no lua bridge —
-- unmodeled; counter wraps 13 -> 6.
local function onSpellHit(event, creature, caster, spellId)
    if spellId ~= SPELL_HIGH_EXECUTORS_BRANDING_IRON then
        return
    end
    if caster:GetTypeId() ~= 4 then -- TYPEID_PLAYER
        return
    end
    local guid = creature:GetGUID()
    local st = lecraftState[guid]
    if not st then
        st = { textCounter = 1 }
        lecraftState[guid] = st
    end
    if st.textCounter == 1 then
        st.playerGUID = caster:GetGUID()
    end
    if st.playerGUID ~= caster:GetGUID() then
        return
    end
    creature:Talk(st.textCounter, caster)
    st.textCounter = st.textCounter + 1
    if st.textCounter == 13 then
        st.textCounter = 6
    end
end

RegisterCreatureEvent(TORTURER_LECRAFT_ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(TORTURER_LECRAFT_ENTRY, 2, onReset)
RegisterCreatureEvent(TORTURER_LECRAFT_ENTRY, 4, onReset)
RegisterCreatureEvent(TORTURER_LECRAFT_ENTRY, 14, onSpellHit)
RegisterCreatureEvent(TORTURER_LECRAFT_ENTRY, 23, onReset)
