-- npc_garments_of_quests (wounded quest NPCs: Shaya/Roberts/Dolf/Korja/DG Kel) --
-- Lua port of src/server/scripts/World/npcs_special.cpp:973-1121
-- (AddSC_npcs_special). Reset kneels the NPC at 70% health; a Lesser
-- Heal (2052) stands and heals them, then Power Word: Fortitude (1243)
-- sends them running and grants quest credit via TalkedToCreature.
-- UpdateAI runaway (:1068) PORTED (round 57): CanRun + !IsInCombat +
-- 5s -> Talk(SAY_GOODBYE 1, caster) + StartEscort(caster) (nil caster
-- -> EnterEvadeMode), re-armed every 30s with a started flag. The
-- caster object is stored from the ev14 handler (illidari-spawn
-- closure idiom — no GUID->player bridge exists).
-- Deviations: StartEscort takes only the player (no run flag — walk,
-- not run); the waypoints are DB escort data, not faked with
-- MoveTo+Despawn.

local SPELL_LESSER_HEAL_R2 = 2052
local SPELL_FORTITUDE_R1 = 1243
local QUEST_STATUS_INCOMPLETE = 3
local STAND_KNEEL = 8
local STAND_STAND = 0

-- entry -> quest (npcs_special.cpp:947-960).
local QUESTS = {
    [12429] = 5621,
    [12423] = 5624,
    [12427] = 5625,
    [12430] = 5648,
    [12428] = 5650,
}

local stateByGuid = {}

local function resetState(creature)
    stateByGuid[creature:GetGUIDLow()] = {casterGuid = nil, casterObj = nil, isHealed = false, canRun = false, runAwayTimer = 5000, started = false}
    creature:SetStandState(STAND_KNEEL)
    creature:SetHealth(math.floor(creature:GetMaxHealth() * 70 / 100))
end

local function onReset(event, creature)
    resetState(creature)
end

local function onSpellHit(event, creature, caster, spellId)
    if spellId ~= SPELL_LESSER_HEAL_R2 and spellId ~= SPELL_FORTITUDE_R1 then
        return
    end
    if creature:IsInCombat() then
        return
    end
    local guid = creature:GetGUIDLow()
    local st = stateByGuid[guid]
    if st == nil then
        st = {casterGuid = nil, casterObj = nil, isHealed = false, canRun = false, runAwayTimer = 5000, started = false}
        stateByGuid[guid] = st
    end
    if st.isHealed and st.canRun then
        return
    end
    if caster == nil or caster:GetObjectType() ~= "Player" then
        return
    end
    local quest = QUESTS[creature:GetEntry()]
    if quest ~= nil and caster:GetQuestStatus(quest) == QUEST_STATUS_INCOMPLETE then
        if st.isHealed and not st.canRun and spellId == SPELL_FORTITUDE_R1 then
            creature:Talk(0, caster)
            st.canRun = true
        elseif not st.isHealed and spellId == SPELL_LESSER_HEAL_R2 then
            st.casterGuid = caster:GetGUID()
            st.casterObj = caster
            creature:SetStandState(STAND_STAND)
            creature:Talk(2, caster)
            st.isHealed = true
        end
    end
    if st.canRun then
        caster:TalkedToCreature(creature:GetEntry(), creature:GetGUID())
    end
end

-- C++ UpdateAI :1068 runaway branch (ev7 ticks with the 100ms diff).
local function onUpdate(_, creature, diff)
    local guid = creature:GetGUIDLow()
    local st = stateByGuid[guid]
    if not st or not st.canRun or creature:IsInCombat() then
        return
    end
    if st.runAwayTimer > diff then
        st.runAwayTimer = st.runAwayTimer - diff
        return
    end
    if st.casterObj then
        creature:Talk(1, st.casterObj)
        if not st.started then
            creature:StartEscort(st.casterObj)
            st.started = true
        end
    else
        creature:EnterEvadeMode()
    end
    st.runAwayTimer = 30000
end

for entry, _ in pairs(QUESTS) do
    RegisterCreatureEvent(entry, 5, onReset)
    RegisterCreatureEvent(entry, 23, onReset)
    RegisterCreatureEvent(entry, 14, onSpellHit)
    RegisterCreatureEvent(entry, 7, onUpdate)
end
