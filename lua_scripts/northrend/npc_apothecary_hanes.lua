-- Apothecary Hanes (Howling Fjord) --
-- Lua port of src/server/scripts/Northrend/zone_howling_fjord.cpp
-- (npc_apothecary_hanesAI — UpdateAI healing-potion health arm
-- only). Zone-script unit per northrend_script_loader.cpp order
-- (grizzly_hills done; howling_fjord: npc_daegarn, npc_mindless_
-- abomination, spell_mindless_abomination_explosion_fx_master,
-- npc_riven_widow_cocoon documented-only, npc_apothecary_hanes
-- ported).
-- Entry (file's own Entries enum, verifiable from the C++
-- sources): 23784 (NPC_APOTHECARY_HANES). The creature_template
-- ScriptName binding is DB-side (no TDB in this workspace).
-- Eluna creature events: 5 OnSpawn, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset.
-- Ported arm (C++ UpdateAI head): HealthBelowPct(75) -> 10s
-- PotTimer -> triggered DoCast(me, SPELL_HEALING_POTION 17534).
-- The arm is not combat-gated in C++ (it runs in UpdateAI before
-- the UpdateVictim check), so it rides an always-on 1s pump armed
-- on OnSpawn(5) — the underbog_mushroom precedent — rather than
-- the omen combat-only pump. 1s granularity is C++-exact for the
-- 10s timer (the torturer_lecraft convention); the timer holds
-- (does not decrement or reset) while at/above 75% — C++-exact.
-- C++ DoCast with triggered=true goes through the same server
-- creature-cast path as the non-triggered casts (the Lua binding
-- accepts the third arg but does not thread the flag — gruul
-- reverberation precedent) — documented deviation, same as every
-- other triggered port. creature:CastSpell(creature, spell, true)
-- is the vaelastrasz self-cast convention.
-- Unmodeled: the whole escort cinematic (OnQuestAccept quest-
-- 11241 launch — no quest-accept hook — EscortAI Start, waypoint
-- 1-40 Talk/emote chain, DoCastAOE 42684 burn-crate arms,
-- SetFaction/SetReactState, GroupEventHappens quest credit)
-- behind the quest-accept / escort / movement / emote / faction
-- / quest bridges; the EVENT_EMOTE_BEG 25s kneel emote behind
-- the no-emote / no-stand-state bridges; the melee arm is
-- engine-driven.
-- npc_daegarn — no NPC_ entry constant anywhere in the C++
-- sources (the creature_template ScriptName binding is DB-side,
-- no TDB in this workspace — minigob precedent): no Lua file
-- written. The gladiator event (OnQuestAccept quest-11300 chain:
-- 24213 -> 24215 -> 24214 -> 23931 summon waves + JustSummoned
-- MovePoint home-position machine + 20s/5s stuck-reset scheduler)
-- sits behind the quest-accept / summon / movement / world-search
-- bridges; the 40s idle Talk(SAY_TEXT 0) scheduler arm is
-- bridgeable in principle but unregistered without an entry.
-- npc_mindless_abomination — the EVENT_CHECK_CHARMED 1s arm
-- (IsCharmedOwnedByPlayerOrPlayer gate -> DespawnOrUnsummon)
-- has no charm-check / despawn bridges in the model —
-- documented-only.
-- spell_mindless_abomination_explosion_fx_master — SpellScript
-- (creature-gated 43401 blood-explosion self-cast + 10x 42266
-- poison circumference casts) has no SpellScript bridge anywhere
-- in the model (blasted_lands / gordunni precedents) —
-- documented-only.
-- npc_riven_widow_cocoon — the JustDied player-killer machine
-- (20% roll: player->CastSpell(me, 43289) + KilledMonsterCredit
-- 24211; else player->CastSpell(me, one of the 11 cocoon summon
-- spells 43275-43285)) has no killer-player / player-cast-on-me
-- / kill-credit path (borean nerubar_victim precedent) —
-- documented-only. No NPC_ entry constant for the cocoon in the
-- C++ sources either.

local SPELL_HEALING_POTION = 17534

local APOTHECARY_HANES_ENTRY = 23784

local hanesState = {}
local hanesPump = {}

local function cancelPump(guid)
    local id = hanesPump[guid]
    if id then
        RemoveEventById(id)
        hanesPump[guid] = nil
    end
end

-- C++ UpdateAI health arm in arm order: below 75% the 10s timer
-- ticks; on expiry the triggered self-cast fires and the timer
-- re-arms to 10s (C++-exact at 1s granularity).
local function potionTick(creature, guid)
    local st = hanesState[guid]
    if not st then
        return
    end
    if creature:GetHealthPct() < 75 then
        st.potTimer = st.potTimer - 1000
        if st.potTimer <= 0 then
            creature:CastSpell(creature, SPELL_HEALING_POTION, true)
            st.potTimer = 10000
        end
    end
end

local function armPump(creature)
    local guid = creature:GetGUID()
    cancelPump(guid)
    hanesState[guid] = { potTimer = 10000 }
    hanesPump[guid] = CreateLuaEvent(function()
        potionTick(creature, guid)
    end, 1000, 0)
end

-- C++ ctor/Reset Initialize(): PotTimer = 10000 (kneel / emote /
-- despawn-at-far arms unmodeled — no stand-state / emote bridges).
-- The arm ticks outside combat too, so the pump survives evade:
-- OnLeaveCombat(2)/OnReset(23) only re-arm the timer (C++ Reset
-- does exactly that).
local function onSpawn(_, creature)
    armPump(creature)
end

local function onReset(_, creature)
    local guid = creature:GetGUID()
    local st = hanesState[guid]
    if st then
        st.potTimer = 10000
    else
        armPump(creature)
    end
end

-- C++ JustDied: FailQuest(QUEST_TRAIL_OF_FIRE 11241) on the
-- escorted player — no quest bridge (documented-only). Death
-- ends the machine: cancel the pump + drop per-GUID state
-- (mushroom precedent).
local function onDied(_, creature)
    local guid = creature:GetGUID()
    cancelPump(guid)
    hanesState[guid] = nil
end

RegisterCreatureEvent(APOTHECARY_HANES_ENTRY, 5, onSpawn)
RegisterCreatureEvent(APOTHECARY_HANES_ENTRY, 2, onReset)
RegisterCreatureEvent(APOTHECARY_HANES_ENTRY, 4, onDied)
RegisterCreatureEvent(APOTHECARY_HANES_ENTRY, 23, onReset)
