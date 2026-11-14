-- Mature Netherwing Drake (Shadowmoon Valley) --
-- Lua port of src/server/scripts/Outland/zone_shadowmoon_valley.cpp
-- (npc_mature_netherwing_drakeAI). Entry 21648 TDB-verified
-- ('Mature Netherwing Drake', ScriptName npc_mature_netherwing_drake).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 5 OnSpawn, 7 OnAIUpdate, 14 OnHitBySpell, 23 OnReset, 28 OnMovementInform.
-- Ported arms (C++-exact for all modeled arms; line refs are
-- zone_shadowmoon_valley.cpp):
-- Reset (:248) — Initialize() field clears (bCanEat/bIsEating false,
-- player GUID clear, EatTimer/CastTimer 5000); lands on spawn (5),
-- reset (23) and enter-combat (1), the gargolmar precedent.
-- SpellHit (:253-262) — player caster + spell 38439 (PLACE_CARCASS)
-- + !HasAura(38502 JUST_EATEN) + not already eating -> latch the
-- player GUID and bCanEat. The early return when bCanEat/bIsEating
-- is the C++ gate.
-- MovementInform (:265-276) — POINT_MOTION_TYPE 8 + id 1 (POINT_ID)
-- -> bIsEating, EatTimer 7000, EMOTE_ONESHOT_ATTACK_UNARMED 35.
-- UpdateAI (:278-326) — the eat branch runs out of combat and rides
-- ev7 (100ms diff, C++-exact): bCanEat -> find the carcass GO 185155
-- within 10yd, MoveIdle + StopMoving + MovePoint to it, bCanEat false;
-- bIsEating -> DoCastSelf 38502 + Talk 0 + player KilledMonsterCredit
-- 22131 + carcass GO Delete + Reset + motion Clear. The combat branch
-- is a 1s CreateLuaEvent pump (illidari precedent): Nether Breath
-- 38467 DoCastVictim every 5s; melee engine-driven.
-- Deviations: the C++ eat branch resolves the PLAYER by GUID
-- (ObjectAccessor::GetPlayer) for the credit and the GO delete; Lua
-- has no GUID->player bridge, so the ev14 caster OBJECT is stored in
-- the per-GUID state (illidari-spawn closure idiom) and used for the
-- credit; the GO lookups are position-based, so the creature's own
-- FindNearestGameObject(185155, 10) stands in for the player's. If
-- the stored player object is gone at eat time the credit/delete legs
-- are skipped. C++ GetMotionMaster()->Clear() after eating twins to
-- StopMoving + state reset.

local ENTRY_MATURE_NETHERWING_DRAKE = 21648

local SAY_JUST_EATEN = 0

local SPELL_PLACE_CARCASS = 38439
local SPELL_JUST_EATEN = 38502
local SPELL_NETHER_BREATH = 38467
local POINT_ID = 1

local GO_CARCASS = 185155

local NPC_EVENT_PINGER = 22131

local POINT_MOTION_TYPE = 8
local EMOTE_ONESHOT_ATTACK_UNARMED = 35

local drakeState = {}
local drakePump = {}

local function cancelPump(guid)
    local id = drakePump[guid]
    if id then
        RemoveEventById(id)
        drakePump[guid] = nil
    end
end

local function fullReset(guid)
    cancelPump(guid)
    drakeState[guid] = nil
end

local function resetState(creature)
    local guid = creature:GetGUIDLow()
    drakeState[guid] = {
        playerGUID = nil,
        playerObj = nil,
        canEat = false,
        isEating = false,
        eatTimer = 5000,
        castTimer = 5000,
    }
end

local function combatTick(creature, guid)
    local st = drakeState[guid]
    if not st then
        return
    end
    if st.canEat or st.isEating then
        return
    end
    if not creature:IsInCombat() then
        return
    end
    st.castTimer = st.castTimer - 1000
    if st.castTimer <= 0 then
        creature:CastSpell(nil, SPELL_NETHER_BREATH)
        st.castTimer = 5000
    end
end

local function onSpawnOrReset(_, creature)
    resetState(creature)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUIDLow()
    resetState(creature)
    cancelPump(guid)
    drakePump[guid] = CreateLuaEvent(function()
        combatTick(creature, guid)
    end, 1000, 0)
end

local function onLeaveCombat(_, creature)
    -- Combat pump only: the eat state survives evade — C++ Reset()
    -- re-initializes it on event 23, which fires on the same evade.
    -- Dropping the state here would strand the drake if 23 did not
    -- re-fire before the next SpellHit.
    cancelPump(creature:GetGUIDLow())
end

local function onDied(_, creature)
    fullReset(creature:GetGUIDLow())
end

local function onSpellHit(_, creature, caster, spellId)
    local st = drakeState[creature:GetGUIDLow()]
    if not st then
        return
    end
    if st.canEat or st.isEating then
        return
    end
    if caster == nil or caster:GetObjectType() ~= "Player" then
        return
    end
    if spellId ~= SPELL_PLACE_CARCASS then
        return
    end
    if creature:HasAura(SPELL_JUST_EATEN) then
        return
    end
    st.playerGUID = caster:GetGUIDLow()
    st.playerObj = caster
    st.canEat = true
end

local function onMovementInform(_, creature, type, id)
    local st = drakeState[creature:GetGUIDLow()]
    if not st then
        return
    end
    if type ~= POINT_MOTION_TYPE then
        return
    end
    if id == POINT_ID then
        st.isEating = true
        st.eatTimer = 7000
        creature:PerformEmote(EMOTE_ONESHOT_ATTACK_UNARMED)
    end
end

local function onAIUpdate(_, creature, diff)
    local guid = creature:GetGUIDLow()
    local st = drakeState[guid]
    if not st then
        return
    end
    if not st.canEat and not st.isEating then
        return
    end
    st.eatTimer = st.eatTimer - diff
    if st.eatTimer > 0 then
        return
    end
    if st.canEat and not st.isEating then
        local go = creature:FindNearestGameObject(GO_CARCASS, 10)
        if go then
            creature:MoveIdle()
            creature:StopMoving()
            creature:MoveTo(POINT_ID, go:GetX(), go:GetY(), go:GetZ())
        end
        st.canEat = false
    elseif st.isEating then
        creature:CastSpell(creature, SPELL_JUST_EATEN)
        creature:Talk(SAY_JUST_EATEN)
        if st.playerObj then
            st.playerObj:KilledMonsterCredit(NPC_EVENT_PINGER)
        end
        local go = creature:FindNearestGameObject(GO_CARCASS, 10)
        if go then
            go:Delete()
        end
        resetState(creature)
        creature:StopMoving()
    end
end

RegisterCreatureEvent(ENTRY_MATURE_NETHERWING_DRAKE, 5, onSpawnOrReset)
RegisterCreatureEvent(ENTRY_MATURE_NETHERWING_DRAKE, 23, onSpawnOrReset)
RegisterCreatureEvent(ENTRY_MATURE_NETHERWING_DRAKE, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_MATURE_NETHERWING_DRAKE, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY_MATURE_NETHERWING_DRAKE, 4, onDied)
RegisterCreatureEvent(ENTRY_MATURE_NETHERWING_DRAKE, 14, onSpellHit)
RegisterCreatureEvent(ENTRY_MATURE_NETHERWING_DRAKE, 28, onMovementInform)
RegisterCreatureEvent(ENTRY_MATURE_NETHERWING_DRAKE, 7, onAIUpdate)
