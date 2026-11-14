-- Shadowmoon Tuber Node (Shadowmoon Valley) --
-- Lua port of src/server/scripts/Outland/zone_shadowmoon_valley.cpp
-- (npc_shadowmoon_tuber_nodeAI). Entry 21347 TDB-verified
-- ('Shadowmoon Valley Tuber Node', ScriptName npc_shadowmoon_tuber_node).
-- Eluna creature events: 14 OnHitBySpell, 41 OnSetData.
-- Ported arms (C++-exact for all modeled arms; line refs are
-- zone_shadowmoon_valley.cpp):
-- SetData (:1554-1567) — id 1 (TYPE_BOAR) + data 1 (DATA_BOAR) ->
-- DoCastSelf 36462 (SHADOWMOON_TUBER) + nearest tuber mound GO 184701
-- within 5.0 -> SetLootState(GO_JUST_DEACTIVATED 3) + DespawnOrUnsummon.
-- The despawn sits INSIDE the GO-found branch in C++ (no GO, no
-- despawn) — the Lua arm keeps it there.
-- SpellHit (:1570-1580) — spell 36652 (WHISTLE) -> nearest boar 21195
-- within 30.0 -> SetWalk(false) + MovePoint(1, node position).
-- No Reset/JustEngagedWith defs in C++ (no events 5/23/1). The SetData
-- caller (boar side) lives outside this file; the ev41 handler only
-- needs to exist.

local ENTRY_SHADOWMOON_TUBER_NODE = 21347

local SPELL_WHISTLE = 36652
local SPELL_SHADOWMOON_TUBER = 36462

local NPC_BOAR_ENTRY = 21195
local GO_SHADOWMOON_TUBER_MOUND = 184701

local POINT_TUBER = 1
local TYPE_BOAR = 1
local DATA_BOAR = 1

local GO_JUST_DEACTIVATED = 3

local function onSetData(_, creature, type, data)
    if type ~= TYPE_BOAR or data ~= DATA_BOAR then
        return
    end
    creature:CastSpell(creature, SPELL_SHADOWMOON_TUBER)
    local tuber = creature:FindNearestGameObject(GO_SHADOWMOON_TUBER_MOUND, 5.0)
    if tuber then
        tuber:SetLootState(GO_JUST_DEACTIVATED)
        creature:Despawn()
    end
end

local function onSpellHit(_, creature, _, spellId)
    if spellId ~= SPELL_WHISTLE then
        return
    end
    local boar = creature:FindNearestCreature(NPC_BOAR_ENTRY, 30.0)
    if boar then
        boar:SetWalk(false)
        boar:MoveTo(POINT_TUBER, creature:GetX(), creature:GetY(), creature:GetZ())
    end
end

RegisterCreatureEvent(ENTRY_SHADOWMOON_TUBER_NODE, 41, onSetData)
RegisterCreatureEvent(ENTRY_SHADOWMOON_TUBER_NODE, 14, onSpellHit)
