-- Enslaved Netherwing Drake (Shadowmoon Valley) --
-- Lua port of src/server/scripts/Outland/zone_shadowmoon_valley.cpp
-- (npc_enslaved_netherwing_drakeAI). Entry 21722 TDB-verified
-- ('Enslaved Netherwing Drake', ScriptName npc_enslaved_netherwing_drake).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 5 OnSpawn, 7 OnAIUpdate, 14 OnHitBySpell, 23 OnReset, 28 OnMovementInform.
-- Ported arms (C++-exact for all modeled arms; line refs are
-- zone_shadowmoon_valley.cpp):
-- Reset (:371-376) — if not Tapped SetFaction(FACTION_ORC_DRAGONMAW 62)
-- + SetDisableGravity(false). The ctor values (Tapped false, FlyTimer
-- 10000) land on spawn (5); reset (23) runs the C++ body exactly and
-- does NOT clear Tapped — the tapped state survives evade so the
-- out-of-combat fly-away still runs after the subjugator fight ends.
-- SpellHit (:379-401) — player caster + spell 38762
-- (HIT_FORCE_OF_NELTHARAKU) + !Tapped -> Tapped latch, player GUID
-- latch, SetFaction(FACTION_FLAYER_HUNTER 1840), nearest Dragonmaw
-- Subjugator 21718 within 50 -> AddThreat(100000) + AttackStart,
-- FlyTimer 10000, ClearThreat(player).
-- MovementInform (:404-414) — POINT_MOTION_TYPE 8 + id 1
-- (POINT_MOVE_UP) -> player GUID clear + DespawnOrUnsummon(1ms).
-- UpdateAI (:418-458) — the fly-away runs when !UpdateVictim, so it
-- rides ev7 (100ms diff, C++-exact) gated on Tapped and not in
-- combat: FlyTimer expiry -> Tapped false; if the player still has
-- quest 10854 INCOMPLETE -> DoCast(player, 38775 FORCE_OF_NELTHARAKU,
-- triggered) + escape-dummy position (nearest 22317 within 30) or the
-- fallback leg + SetCanFly(true) + SetDisableGravity(true) +
-- MoveTakeoff(1, pos). In combat the drake only melees (engine-driven).
-- Deviations: per-unit threat clear has no bridge — ClearThreatList
-- clears the whole list, so the Lua arm clears FIRST and then adds the
-- subjugator threat + AttackStart (C++ order is AddThreat, AttackStart,
-- then ClearThreat(player); the end state is identical: the list holds
-- only the subjugator at 100000). GetRandomNearPosition has no bridge —
-- the fallback leg uses the drake's own position +25z (the escape
-- dummy is the common path; the C++ comment marks it the expected
-- case). The player is resolved by GUID in C++ (ObjectAccessor::
-- GetPlayer); Lua has no GUID->player bridge, so the ev14 caster
-- OBJECT is stored in the per-GUID state (illidari-spawn closure
-- idiom). DespawnOrUnsummon(1ms) twins to immediate Despawn (the 1ms
-- delay is below the 1s pump granularity).

local ENTRY_ENSLAVED_NETHERWING_DRAKE = 21722

local SPELL_HIT_FORCE_OF_NELTHARAKU = 38762
local SPELL_FORCE_OF_NELTHARAKU = 38775

local NPC_DRAGONMAW_SUBJUGATOR = 21718
local NPC_ESCAPE_DUMMY = 22317

local QUEST_THE_FORCE_OF_NELTHARAKU = 10854
local QUEST_STATUS_INCOMPLETE = 3

local POINT_MOVE_UP = 1
local POINT_MOTION_TYPE = 8

local FACTION_ORC_DRAGONMAW = 62
local FACTION_FLAYER_HUNTER = 1840

local drakeState = {}

-- C++ Reset() (:371-376) does NOT clear Tapped or FlyTimer — it only
-- re-applies the faction (when untapped) and gravity. A spawn (5)
-- initializes the ctor values; reset (23) runs the C++ body exactly.
local function onSpawn(_, creature)
    drakeState[creature:GetGUIDLow()] = {
        tapped = false,
        flyTimer = 10000,
        playerGUID = nil,
        playerObj = nil,
    }
    creature:SetFaction(FACTION_ORC_DRAGONMAW)
    creature:SetDisableGravity(false)
end

local function onReset(_, creature)
    local st = drakeState[creature:GetGUIDLow()]
    if st == nil or not st.tapped then
        creature:SetFaction(FACTION_ORC_DRAGONMAW)
    end
    creature:SetDisableGravity(false)
end

local function onDied(_, creature)
    drakeState[creature:GetGUIDLow()] = nil
end

local function onSpellHit(_, creature, caster, spellId)
    local st = drakeState[creature:GetGUIDLow()]
    if not st then
        return
    end
    if caster == nil or caster:GetObjectType() ~= "Player" then
        return
    end
    if spellId ~= SPELL_HIT_FORCE_OF_NELTHARAKU or st.tapped then
        return
    end
    st.tapped = true
    st.playerGUID = caster:GetGUIDLow()
    st.playerObj = caster
    creature:SetFaction(FACTION_FLAYER_HUNTER)
    creature:ClearThreatList()
    local subjugator = creature:FindNearestCreature(NPC_DRAGONMAW_SUBJUGATOR, 50)
    if subjugator then
        creature:AddThreat(subjugator, 100000)
        creature:AttackStart(subjugator)
    end
    st.flyTimer = 10000
end

local function onMovementInform(_, creature, type, id)
    local st = drakeState[creature:GetGUIDLow()]
    if not st then
        return
    end
    if type ~= POINT_MOTION_TYPE then
        return
    end
    if id == POINT_MOVE_UP then
        st.playerGUID = nil
        st.playerObj = nil
        creature:Despawn()
    end
end

local function onAIUpdate(_, creature, diff)
    local st = drakeState[creature:GetGUIDLow()]
    if not st then
        return
    end
    if not st.tapped or creature:IsInCombat() then
        return
    end
    st.flyTimer = st.flyTimer - diff
    if st.flyTimer > 0 then
        return
    end
    st.tapped = false
    local player = st.playerObj
    if player and player:GetQuestStatus(QUEST_THE_FORCE_OF_NELTHARAKU) == QUEST_STATUS_INCOMPLETE then
        creature:CastSpell(player, SPELL_FORCE_OF_NELTHARAKU, true)
        local x, y, z
        local dummy = creature:FindNearestCreature(NPC_ESCAPE_DUMMY, 30)
        if dummy then
            x = dummy:GetX()
            y = dummy:GetY()
            z = dummy:GetZ()
        else
            x = creature:GetX()
            y = creature:GetY()
            z = creature:GetZ() + 25
        end
        creature:SetCanFly(true)
        creature:SetDisableGravity(true)
        creature:MoveTakeoff(POINT_MOVE_UP, x, y, z)
    end
end

RegisterCreatureEvent(ENTRY_ENSLAVED_NETHERWING_DRAKE, 5, onSpawn)
RegisterCreatureEvent(ENTRY_ENSLAVED_NETHERWING_DRAKE, 23, onReset)
RegisterCreatureEvent(ENTRY_ENSLAVED_NETHERWING_DRAKE, 4, onDied)
RegisterCreatureEvent(ENTRY_ENSLAVED_NETHERWING_DRAKE, 14, onSpellHit)
RegisterCreatureEvent(ENTRY_ENSLAVED_NETHERWING_DRAKE, 28, onMovementInform)
RegisterCreatureEvent(ENTRY_ENSLAVED_NETHERWING_DRAKE, 7, onAIUpdate)
