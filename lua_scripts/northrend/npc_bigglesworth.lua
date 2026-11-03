-- Bigglesworth (Naxxramas) — Lua port of the OnUnitDeath leg in
-- src/server/scripts/Northrend/Naxxramas/instance_naxxramas.cpp
-- (instance_naxxramas_InstanceMapScript::OnUnitDeath): when the
-- creature with entry NPC_BIGGLESWORTH 16998
-- (naxxramas.h:108) dies, Kel'Thuzad talks SAY_KELTHUZAD_CAT_DIED
-- 5 (instance_naxxramas.cpp: C++ loads Kel'Thuzad's grid, then
-- instance->GetCreature(KelthuzadGUID)->AI()->Talk).
-- The C++ leg rides the instance OnUnitDeath hook, which has no
-- Go map-event twin, so it ports as this creature's own OnDied
-- (event 4) handler: the grid-load leg has no bridge, and the
-- instance-GUID lookup is the room-scale FindNearestCreature
-- idiom (skadi/eregos precedent) — Bigglesworth prowls
-- Kel'Thuzad's chamber, so 500yd covers the room; a missing
-- Kel'Thuzad (not yet materialized) skips the line silently.
-- Entry 16998 — "Bigglesworth" (naxxramas.h NPC_BIGGLESWORTH;
-- no boss_ai.go registration: event firing is entry-based, and
-- a critter takes no BossAI slot).

local ENTRY_BIGGLESWORTH = 16998
local ENTRY_KELTHUZAD = 15990

local SAY_KELTHUZAD_CAT_DIED = 5

local function bigglesworthDied(event, creature, killer)
    local kelthuzad = creature:FindNearestCreature(ENTRY_KELTHUZAD, 500)
    if kelthuzad then
        kelthuzad:Talk(SAY_KELTHUZAD_CAT_DIED)
    end
end

RegisterCreatureEvent(ENTRY_BIGGLESWORTH, 4, bigglesworthDied)
