-- Borean Tundra zone spell aura scripts -- Lua ports of the AuraScript arms
-- in src/server/scripts/Northrend/zone_borean_tundra.cpp.
--
-- PORTED (tick 205, RegisterAuraEvent OnRemove = 3):
--   spell_windsoul_totem_aura (46374, zone_borean_tundra.cpp:1669-1694):
--   EFFECT_0 DUMMY. OnEffectRemove, if the target is dead, the caster
--   casts Windsoul credit (46378) with a nil target, non-triggered.
--   Quest "The Power of the Elements": the Windsoul Totem gathers the
--   energy of slain Steam Ragers.
--   Aura binding is DB-side only: TDB spell_script_names row
--   (46374,'spell_windsoul_totem_aura')
--   (sql/old/3.3.5a/TDB52_to_TDB53_updates/world/
--   2014_03_26_00_world_sai.sql), Wowhead "Windsoul Totem Aura"
--   Effect 0 is Apply Aura: Dummy, 3 sec duration.
-- DELTA: C++ casts with a nullptr target (the credit spell's implicit
-- targeting); Go creature:CastSpell(nil, ...) falls back to the
-- caster's current victim and no-ops without one, so the port casts
-- explicitly on the caster.

local SPELL_WINDSOUL_TOTEM_AURA = 46374 -- TDB spell_script_names
local SPELL_WINDSOUL_CREDIT = 46378 -- zone_borean_tundra.cpp:1666

-- C++ spell_windsoul_totem_aura_AuraScript::OnRemove
-- (zone_borean_tundra.cpp:1678-1684).
local function windsoulTotemAuraRemove(event, aura, target)
    if target:IsDead() then
        local caster = aura:GetCaster()
        if caster ~= nil then
            caster:CastSpell(caster, SPELL_WINDSOUL_CREDIT)
        end
    end
end

RegisterAuraEvent(SPELL_WINDSOUL_TOTEM_AURA, 3, windsoulTotemAuraRemove)

local SPELL_BLOODSPORE_RUINATION = 45997 -- zone_borean_tundra.cpp:1705 (class spell_q11719_bloodspore_ruination_45997)
local NPC_BLOODMAGE_LAURITH = 25381 -- zone_borean_tundra.cpp:1699

-- C++ spell_q11719_bloodspore_ruination_45997::HandleEffect
-- (zone_borean_tundra.cpp:1714-1719): OnEffectHit, EFFECT_1
-- SPELL_EFFECT_SEND_EVENT — the caster finds Bloodmage Laurith
-- within 100yd and hands the AI the caster's GUID via SetGUID.
-- ev4 = OnEffectHit (lua_spell.go:588-594; sonicBoomHit precedent).
-- PORTED (round 26): npc_bloodmage_laurith AI lives in
-- npc_bloodmage_laurith.lua (ev40 SetGUID + 1s Talk + 5s orientation
-- reset); this leg now fires into the registered AI.
local function bloodsporeRuinationHit(event, spell, effectIndex)
    if effectIndex ~= 1 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local laurith = caster:FindNearestCreature(NPC_BLOODMAGE_LAURITH, 100.0)
    if laurith ~= nil then
        laurith:SetGUID(caster:GetGUIDLow(), 0)
    end
end

RegisterSpellEvent(SPELL_BLOODSPORE_RUINATION, 4, bloodsporeRuinationHit)

-- Borean Tundra: Thassarian — Lua port of the WaypointReached arms
-- of src/server/scripts/Northrend/zone_borean_tundra.cpp
-- (class npc_thassarian : CreatureScript("npc_thassarian");
-- struct npc_thassarianAI : public EscortAI).
-- Entry: 101303 (TDB 335.26091 creature_template ScriptName
-- npc_thassarian). Eluna creature events: 6 OnReachWP.
-- Ported arms (C++ order, zone_borean_tundra.cpp:495-547), under
-- the GetPlayerForEscort() nil-guard (Lua twin: GetEscortPlayer()):
-- - wp3 SetEscortPaused(true); summon NPC_IMAGE_LICH_KING 26203
--   (3730.313, 3518.689, 473.324, 1.562, CORPSE_TIMED_DESPAWN
--   2min) + SetFlag(UNIT_FIELD_FLAGS 59, UNIT_FLAG_NON_ATTACKABLE
--   0x2) + SetReactState(REACT_PASSIVE 0, UnitDefines.h:340) +
--   SetWalk(true) + MoveTo(0, 3737.374756, 3564.841309,
--   477.433014); summon NPC_COUNSELOR_TALBOT 25301 (3747.23,
--   3614.936, 473.321, 4.462012, same despawn) + SetWalk(true) +
--   MoveTo(0, 3738.000977, 3568.882080, 477.433014); me:SetWalk(false).
-- - wp4 SetEscortPaused(true) + phase = 7 latch (per-GUID; the
--   phase machine reads it, see below).
-- Deliberate omissions / unported scope: the UpdateAI phase machine
-- (zone_borean_tundra.cpp:549-704: phases 1-17, Talbot/Arthas/Arlos/
-- Leryssa dialogue chain, Valanar transform, combat start) — the
-- wp4 phase=7 arm only arms it, and the wp3 summon GUID latches
-- (arthasGUID/talbotGUID) live in it. The gossip driver
-- (OnGossipSelect Start, :733-740) is with it, so this handler
-- fires once that lands.

local ENTRY_THASSARIAN = 101303

local NPC_IMAGE_LICH_KING = 26203 -- zone_borean_tundra.cpp:412
local NPC_COUNSELOR_TALBOT = 25301 -- zone_borean_tundra.cpp:413

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NON_ATTACKABLE = 0x2
local REACT_PASSIVE = 0

local SUMMON_CORPSE_TIMED_DESPAWN = 5
local SUMMON_DESPAWN_MS = 120000 -- 2min

local thassarianPhase = {}

-- C++ WaypointReached (zone_borean_tundra.cpp:495-547).
local function thassarianReachWP(event, creature)
    local player = creature:GetEscortPlayer()
    if not player then
        return
    end
    local wp = creature:GetCurrentWaypointId()
    if wp == 3 then
        creature:SetEscortPaused(true)
        local arthas = creature:SummonCreature(NPC_IMAGE_LICH_KING,
            3730.313, 3518.689, 473.324, 1.562,
            SUMMON_CORPSE_TIMED_DESPAWN, SUMMON_DESPAWN_MS)
        if arthas then
            arthas:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
            arthas:SetReactState(REACT_PASSIVE)
            arthas:SetWalk(true)
            arthas:MoveTo(0, 3737.374756, 3564.841309, 477.433014)
        end
        local talbot = creature:SummonCreature(NPC_COUNSELOR_TALBOT,
            3747.23, 3614.936, 473.321, 4.462012,
            SUMMON_CORPSE_TIMED_DESPAWN, SUMMON_DESPAWN_MS)
        if talbot then
            talbot:SetWalk(true)
            talbot:MoveTo(0, 3738.000977, 3568.882080, 477.433014)
        end
        creature:SetWalk(false)
    elseif wp == 4 then
        creature:SetEscortPaused(true)
        thassarianPhase[creature:GetGUIDLow()] = 7
    end
end

RegisterCreatureEvent(ENTRY_THASSARIAN, 6, thassarianReachWP)
