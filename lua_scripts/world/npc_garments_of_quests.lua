-- npc_garments_of_quests (wounded quest NPCs: Shaya/Roberts/Dolf/Korja/DG Kel) --
-- Lua port of src/server/scripts/World/npcs_special.cpp:940-1076
-- (AddSC_npcs_special). Reset kneels the NPC at 70% health; a Lesser
-- Heal (2052) stands and heals them, then Power Word: Fortitude (1243)
-- sends them running and grants quest credit via TalkedToCreature.
--
-- NOT ported: UpdateAI runaway (:1037-1068) — no EscortAI Start bridge
-- and no EnterEvadeMode; the waypoints are DB escort data, not faked
-- with MoveTo+Despawn. The NPC stays standing, healed, and credited.

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
    stateByGuid[creature:GetGUID()] = {casterGuid = nil, isHealed = false, canRun = false}
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
    local guid = creature:GetGUID()
    local st = stateByGuid[guid]
    if st == nil then
        st = {casterGuid = nil, isHealed = false, canRun = false}
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
            creature:SetStandState(STAND_STAND)
            creature:Talk(2, caster)
            st.isHealed = true
        end
    end
    if st.canRun then
        caster:TalkedToCreature(creature:GetEntry(), creature:GetGUID())
    end
end

for entry, _ in pairs(QUESTS) do
    RegisterCreatureEvent(entry, 5, onReset)
    RegisterCreatureEvent(entry, 23, onReset)
    RegisterCreatureEvent(entry, 14, onSpellHit)
end
