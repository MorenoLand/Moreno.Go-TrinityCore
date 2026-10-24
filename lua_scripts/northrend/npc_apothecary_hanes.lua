-- Apothecary Hanes (Howling Fjord) --
-- Lua port of src/server/scripts/Northrend/zone_howling_fjord.cpp
-- (npc_Apothecary_HanesAI — UpdateAI healing-potion health arm
-- only). Zone-script unit per northrend_script_loader.cpp order
-- (grizzly_hills done; howling_fjord: npc_daegarn, npc_mindless_
-- abomination, spell_mindless_abomination_explosion_fx_master,
-- npc_riven_widow_cocoon documented-only, npc_apothecary_hanes
-- ported).
-- Entry (file's own Entries enum, verifiable from the C++
-- sources): 23784 (NPC_APOTHECARY_HANES). The creature_template
-- ScriptName binding is DB-side (no TDB in this workspace).
-- Eluna creature events: 5 OnSpawn, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset, 31 OnQuestAccept.
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
-- 11241 launch — EscortAI Start, waypoint
-- 1-40 Talk/emote chain, DoCastAOE 42684 burn-crate arms,
-- SetFaction/SetReactState, GroupEventHappens quest credit)
-- behind the escort / movement / emote / faction
-- / quest bridges; the EVENT_EMOTE_BEG 25s beg-emote arm is PORTED
-- below (timer-driven HandleEmoteCommand(EMOTE_ONESHOT_BEG 20) twin
-- via CreateLuaEvent + creature:PerformEmote — the stale "no emote
-- bridge" claim is retracted); the SetDespawnAtFar(false) Reset leg
-- stays out (no SetDespawnAtFar bridge in the runtime — zero hits
-- in engine/); the melee arm is engine-driven.
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
-- now has its bridges (creature:IsCharmed + creature:Despawn)
-- but no NPC_ entry constant exists in the C++ sources, so it
-- stays unregistered; the charm-semantics gap also stands
-- (Go IsCharmed = any charm, C++ requires player-owned charm).
-- spell_mindless_abomination_explosion_fx_master — SpellScript
-- (creature-gated 43401 blood-explosion self-cast + 10x 42266
-- poison circumference casts) has no SpellScript bridge anywhere
-- in the model (blasted_lands / gordunni precedents) —
-- documented-only.
-- npc_riven_widow_cocoon — the JustDied player-killer machine
-- (20% roll: player->CastSpell(me, 43289) + KilledMonsterCredit
-- 24211; else player->CastSpell(me, one of the 11 cocoon summon
-- spells 43275-43285)) KilledMonsterCredit is bridged, but the summon casts are
-- player->CastSpell(me, ...) with no player-cast bridge (moroes
-- precedent) — documented-only. No NPC_ entry constant for the cocoon in the
-- C++ sources either.

local SPELL_HEALING_POTION = 17534
local EMOTE_ONESHOT_BEG = 20 -- SharedDefines.h:1955

local APOTHECARY_HANES_ENTRY = 23784

local QUEST_TRAIL_OF_FIRE = 11241

local hanesState = {}
local hanesPump = {}

local hanesEscortPlayer = {}

-- C++ HealthBelowPct(pct) = GetHealth() < floor(maxHealth*pct/100)
-- (ironaya convention) — GetHealthPct() fires up to 1 HP early on the
-- float edge.
local function healthBelowPct(creature, pct)
    local max = creature:GetMaxHealth()
    if max == 0 then
        return false
    end
    return creature:GetHealth() < math.floor(max * pct / 100)
end

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
    if healthBelowPct(creature, 75) then
        st.potTimer = st.potTimer - 1000
        if st.potTimer <= 0 then
            creature:CastSpell(creature, SPELL_HEALING_POTION, true)
            st.potTimer = 10000
        end
    end
end

local hanesEmote = {}

local function cancelEmote(guid)
    local id = hanesEmote[guid]
    if id then
        RemoveEventById(id)
        hanesEmote[guid] = nil
    end
end

-- zone_howling_fjord.cpp:138-141 — EVENT_EMOTE_BEG: HandleEmoteCommand
-- (EMOTE_ONESHOT_BEG) then re-schedule at 25s. creature:PerformEmote
-- is the exact HandleEmoteCommand twin (lua_creature_events.go:1053;
-- spectral_ghostly_citizen precedent). C++ skips the EventMap update
-- while in combat (:132-133), so the handler skips in-combat fires
-- (documented deviation: C++ pauses the 25s countdown in combat rather
-- than skipping the occurrence). First fire 2s after Initialize (:92),
-- then 25s repeats.
local function begTick(creature, guid)
    if creature:IsInCombat() then
        return
    end
    creature:PerformEmote(EMOTE_ONESHOT_BEG)
end

local function armEmote(creature)
    local guid = creature:GetGUID()
    cancelEmote(guid)
    hanesEmote[guid] = CreateLuaEvent(function()
        begTick(creature, guid)
        hanesEmote[guid] = CreateLuaEvent(function()
            begTick(creature, guid)
        end, 25000, 0)
    end, 2000)
end

local function armPump(creature)
    local guid = creature:GetGUID()
    cancelPump(guid)
    hanesState[guid] = { potTimer = 10000 }
    hanesPump[guid] = CreateLuaEvent(function()
        potionTick(creature, guid)
    end, 1000, 0)
    -- zone_howling_fjord.cpp:93 — Initialize() stands the apothecary in
    -- KNEEL (SetStandState bridged 2026-10-21, lua_creature_events.go:568)
    -- and re-arms EVENT_EMOTE_BEG at 2s (ported above); only the
    -- SetDespawnAtFar(false) Reset leg stays unmodeled (no bridge).
    creature:SetStandState(8)
    armEmote(creature)
end

-- C++ ctor/Reset Initialize(): PotTimer = 10000, EVENT_EMOTE_BEG
-- re-armed at 2s (ported above); SetDespawnAtFar(false) unmodeled
-- (no bridge).
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
    -- C++ Reset -> Initialize() re-schedules EVENT_EMOTE_BEG at 2s.
    armEmote(creature)
end

-- C++ OnQuestAccept (zone_howling_fjord.cpp:240-243): quest 11241
-- starts the escort (StartEscort captures the player GUID); the escort
-- itself stays unported, but the GUID latch lets JustDied FailQuest the
-- escort player C++-exactly.
local function onQuestAccept(_, player, creature, quest)
    if not quest or quest:GetId() ~= QUEST_TRAIL_OF_FIRE then
        return
    end
    if player then
        hanesEscortPlayer[creature:GetGUID()] = player:GetGUID()
    end
end

-- C++ JustDied: FailQuest(QUEST_TRAIL_OF_FIRE 11241) on the
-- escorted player — PORTED below (quest-11241 accept latch +
-- player:FailQuest bridge, lua_player.go:410). Death
-- ends the machine: cancel the pump + emote event and drop per-GUID
-- state (mushroom precedent).
local function onDied(_, creature)
    local guid = creature:GetGUID()
    local playerGuid = hanesEscortPlayer[guid]
    hanesEscortPlayer[guid] = nil
    if playerGuid then
        for _, plr in ipairs(GetPlayersInWorld()) do
            if plr:GetGUID() == playerGuid then
                plr:FailQuest(QUEST_TRAIL_OF_FIRE)
                break
            end
        end
    end
    cancelPump(guid)
    cancelEmote(guid)
    hanesState[guid] = nil
end

RegisterCreatureEvent(APOTHECARY_HANES_ENTRY, 5, onSpawn)
RegisterCreatureEvent(APOTHECARY_HANES_ENTRY, 2, onReset)
RegisterCreatureEvent(APOTHECARY_HANES_ENTRY, 4, onDied)
RegisterCreatureEvent(APOTHECARY_HANES_ENTRY, 23, onReset)
RegisterCreatureEvent(APOTHECARY_HANES_ENTRY, 31, onQuestAccept)
