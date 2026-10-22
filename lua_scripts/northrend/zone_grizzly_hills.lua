-- Grizzly Hills: npc_lake_frog — Lua port of
-- src/server/scripts/Northrend/zone_grizzly_hills.cpp
-- (npc_lake_frog : public CreatureScript; registered in AddSC_zone_grizzly_hills()).
-- Entries: NPC_LAKE_FROG = 33211 / NPC_LAKE_FROG_QUEST = 33224
-- (zone_grizzly_hills.cpp:655-667 LakeFrog enum). The C++ tree has a single
-- npc_lake_frog script (one `new` at :1130), so both entries share this AI —
-- both are registered.
-- Ported: Reset's RemoveFlag(UNIT_NPC_FLAGS, UNIT_NPC_FLAG_GOSSIP) twin on
-- spawn (event 5) — C++ Reset (:702-707) is entry-conditional: it removes
-- the flag ONLY for NPC_LAKE_FROG_QUEST (33224), so only that entry is
-- registered below. 33211 keeps its template flag exactly like C++.
-- The select arm below stays gated exactly like C++ —
-- plus the OnGossipSelect DoCast(player, SPELL_SUMMON_ASHWOOD_BRAND 62554)
-- arm (:774-778), non-triggered (default DoCast overload); return false lets
-- the AI/native select arms proceed (Eluna gossip polarity, gossip.go).
-- Ported: the ReceiveEmote kiss machine (zone_grizzly_hills.cpp:746-777,
-- npc_lake_frogAI::ReceiveEmote, ev8 — the trigger is the emote, not
-- MoveInLineOfSight): TEXT_EMOTE_KISS (58) within 30yd while the player
-- carries the Warts-B-Gone Lip Balm (44986). player:HasItem,
-- creature:IsWithinDist (object.go generic fallback) and every aura leg are
-- bridged. Without the lip-balm aura (62574) the player gets SPELL_WARTS
-- (62581); with it, the frog casts SPELL_FROG_KISS (62536, non-triggered
-- DoCast twin) and 33211 takes SPELL_FROG_LOVE (62537). The
-- _following/_runningScript re-entry latch is a per-GUID table, and the
-- UpdateAI followup (:699-701 — the follower despawns once FROG_LOVE fades;
-- the aura lasts 60s) rides a 61s one-shot CreateLuaEvent, rechecked until
-- the aura is gone (C++ polls it every UpdateAI tick).
-- Stays out (no bridges): MoveFollow / MoveIdle / SetFacingToObject, and
-- the 33224 EVENT_LAKEFROG_1..5 maiden-transform chain (DoCast
-- SPELL_MAIDEN_OF_ASHWOOD_LAKE_TRANSFORM 62550, SetEntry 33220, maiden
-- Talk SAY_MAIDEN_0/1, gossip-flag windows) — the gossip flag can never be
-- re-set without it.
-- npc_venture_co_straggler (zone_grizzly_hills.cpp:562-651) — NOT
-- registered: Reset (RemoveFlag NON_ATTACKABLE + SetImmuneToPC(false) +
-- SetReactState(REACT_AGGRESSIVE)) and SpellHit (SMOKE_BOMB ->
-- NON_ATTACKABLE + ImmuneToPC + SetReactState(REACT_PASSIVE) +
-- CombatStop + the EVENT_STRAGGLER_1..4 chain) ride the MovePoint
-- waypoint machine — no movement bridge; the react flips stay out
-- with the rest of the AI.
-- Documented-only (nearest-creature axis): npc_amberpine_woodsman
-- (zone_grizzly_hills.cpp:409-457): Reset's
-- FindNearestCreature(NPC_TALLHORN_STAG 26363, 0.2f) emote-state gate
-- (:420 — EMOTE_STATE_USE_STANDING vs. the EVENT_WOODSMAN_1/2
-- loot/attack emote loop) has a live bridge twin, but the AI is not
-- registered in boss_ai.go and carries no NPC_ entry constant anywhere
-- in the C++ tree (DB ScriptName binding only, unverifiable —
-- wounded-skirmisher precedent); stays out as unregistered AI.
local ENTRY_LAKE_FROG = 33211
local ENTRY_LAKE_FROG_QUEST = 33224
local UNIT_NPC_FLAGS_FIELD = 82
local UNIT_NPC_FLAG_GOSSIP = 1
local SPELL_SUMMON_ASHWOOD_BRAND = 62554

local ITEM_WARTS_B_GONE_LIP_BALM = 44986
local SPELL_WARTSBGONE_LIP_BALM = 62574
local SPELL_FROG_LOVE = 62537
local SPELL_WARTS = 62581
local SPELL_FROG_KISS = 62536
local TEXT_EMOTE_KISS = 58

local timers = {}
local frogKissLatched = {}

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
    per[key] = CreateLuaEvent(fn, delay)
end

local function lakeFrogSpawn(_, creature)
    creature:RemoveFlag(UNIT_NPC_FLAGS_FIELD, UNIT_NPC_FLAG_GOSSIP)
end

local function lakeFrogGossipSelect(_, player, creature)
    creature:CastSpell(player, SPELL_SUMMON_ASHWOOD_BRAND)
    return false
end

-- C++ UpdateAI :699-701: the kissed follower despawns once SPELL_FROG_LOVE
-- fades. Rechecked until the aura is gone (C++ polls every UpdateAI tick).
local function frogLoveWatch(creature, guid)
    if creature:HasAura(SPELL_FROG_LOVE) then
        schedule(guid, "fadinglove", 61000, function()
            frogLoveWatch(creature, guid)
        end)
    else
        timers[guid] = nil
        frogKissLatched[guid] = nil
        creature:Despawn()
    end
end

local function lakeFrogReceiveEmote(_, creature, player, textEmote)
    local guid = creature:GetGUIDLow()
    if frogKissLatched[guid] then
        return
    end
    if textEmote == TEXT_EMOTE_KISS and creature:IsWithinDist(player, 30)
        and player:HasItem(ITEM_WARTS_B_GONE_LIP_BALM, 1) then
        if not player:HasAura(SPELL_WARTSBGONE_LIP_BALM) then
            player:AddAura(SPELL_WARTS)
        else
            creature:CastSpell(player, SPELL_FROG_KISS)
            if creature:GetEntry() == ENTRY_LAKE_FROG then
                creature:AddAura(SPELL_FROG_LOVE)
                schedule(guid, "fadinglove", 61000, function()
                    frogLoveWatch(creature, guid)
                end)
            end
            -- 33224's MoveIdle/SetFacingToObject + EVENT_LAKEFROG chain:
            -- motion/transform legs unbridged (see header).
            frogKissLatched[guid] = true
        end
    end
end

-- Pure timer/latch hygiene (no C++ JustDied/Reset counterpart for these
-- legs): cancel the fading-love watch and clear the kiss latch.
local function lakeFrogDied(_, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    frogKissLatched[guid] = nil
end

local function lakeFrogReset(_, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    frogKissLatched[guid] = nil
end

RegisterCreatureEvent(ENTRY_LAKE_FROG_QUEST, 5, lakeFrogSpawn)
RegisterCreatureGossipEvent(ENTRY_LAKE_FROG, 2, lakeFrogGossipSelect)
RegisterCreatureGossipEvent(ENTRY_LAKE_FROG_QUEST, 2, lakeFrogGossipSelect)
RegisterCreatureEvent(ENTRY_LAKE_FROG, 8, lakeFrogReceiveEmote)
RegisterCreatureEvent(ENTRY_LAKE_FROG_QUEST, 8, lakeFrogReceiveEmote)
RegisterCreatureEvent(ENTRY_LAKE_FROG, 4, lakeFrogDied)
RegisterCreatureEvent(ENTRY_LAKE_FROG_QUEST, 4, lakeFrogDied)
RegisterCreatureEvent(ENTRY_LAKE_FROG, 23, lakeFrogReset)
RegisterCreatureEvent(ENTRY_LAKE_FROG_QUEST, 23, lakeFrogReset)
