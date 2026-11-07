-- Jaina / Sylvanas escape leader (Halls of Reflection) — Lua port of
-- npc_jaina_or_sylvanas_escape_hor::JustDied in
-- src/server/scripts/Northrend/FrozenHalls/HallsOfReflection/halls_of_reflection.cpp
-- (:843-847): if the Lich King is found via instance GetGuidData
-- (DATA_THE_LICH_KING_ESCAPE), lichking->AI()->EnterEvadeMode()
-- (event failed). The relayed EnterEvadeMode legs are ALL bridged and
-- already ported as lichKingEscapeLeaveCombat
-- (npc_the_lich_king_escape.lua:108-121, ev2) — this file replays the
-- same legs on the instance-mirror lichkingEscape creature.
-- Entry: 36955 NPC_JAINA_ESCAPE / 37554 NPC_SYLVANAS_ESCAPE
-- (halls_of_reflection.h:82-83). No RegisterLuaBoss line needed —
-- entry-keyed dispatch (non-boss NPC precedent).
local ENTRY_JAINA_ESCAPE = 36955 -- halls_of_reflection.h:82
local ENTRY_SYLVANAS_ESCAPE = 37554 -- halls_of_reflection.h:83
local DATA_THE_LICH_KING_ESCAPE = 2 -- halls_of_reflection.h:39
local BOSS_STATE_FAIL = 2 -- InstanceScript.h:68-76 (NOT_STARTED 0, IN_PROGRESS 1, FAIL 2, DONE 3)
local SOUND_LK_FURY_OF_FROSTMOURNE = 17224 -- halls_of_reflection.cpp:215
local SPELL_FURY_OF_FROSTMOURNE = 70063 -- halls_of_reflection.cpp:243

-- C++ JustDied (halls_of_reflection.cpp:843-847) + the relayed
-- EnterEvadeMode legs (halls_of_reflection.cpp:1305-1316, C++ order:
-- instance SetBossState(FAIL) -> StopMoving -> DoPlaySoundToSet ->
-- DoCastAOE (self-cast idiom) -> DespawnOrUnsummon(12s)).
-- Do NOT call lichking:EnterEvadeMode — no such creature method exists.
local function escapeLeaderDied(event, creature)
    local mapObj = creature:GetMap()
    if not mapObj then return end
    local data = mapObj:GetInstanceData()
    if not data or not data.lichkingEscape then return end
    local lichking = data.lichkingEscape
    if lichking:GetBossState(DATA_THE_LICH_KING_ESCAPE) == BOSS_STATE_FAIL then
        return -- LK already evaded; mirrors the C++ _despawn once-latch
    end
    lichking:SetBossState(DATA_THE_LICH_KING_ESCAPE, BOSS_STATE_FAIL)
    lichking:StopMoving()
    lichking:PlayDirectSound(SOUND_LK_FURY_OF_FROSTMOURNE)
    lichking:CastSpell(lichking, SPELL_FURY_OF_FROSTMOURNE)
    CreateLuaEvent(function() lichking:Despawn() end, 12000)
end

RegisterCreatureEvent(ENTRY_JAINA_ESCAPE, 4, escapeLeaderDied)
RegisterCreatureEvent(ENTRY_SYLVANAS_ESCAPE, 4, escapeLeaderDied)
