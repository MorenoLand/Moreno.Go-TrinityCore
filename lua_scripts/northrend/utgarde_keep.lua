-- Utgarde Keep zone spell aura scripts -- Lua ports of the AuraScript arms
-- in src/server/scripts/Northrend/UtgardeKeep/UtgardeKeep/utgarde_keep.cpp.
--
-- PORTED (tick 205, RegisterAuraEvent OnRemove = 3):
--   spell_ticking_time_bomb (59686, utgarde_keep.cpp:125-154):
--   EFFECT_0 PERIODIC_DUMMY. OnEffectRemove, if the caster is the
--   target, the target casts Ticking Time Bomb Explode (59687) on
--   itself, triggered. No mode gate in C++.
--   Aura binding is DB-side only: TDB spell_script_names row
--   (59686,'spell_ticking_time_bomb')
--   (sql/old/3.3.5a/TDB49_to_TDB50_updates/world/
--   2012_11_16_00_world_utgarde.sql), Wowhead "Ticking Time Bomb"
--   Effect 0 is Persistent Area Aura: Periodic Dummy, 1 sec interval.
-- DELTA: C++ compares Unit pointers (GetCaster() == GetTarget()); the
-- port compares GUIDs (aura:GetCasterGUID() vs target:GetGUID()).
--
-- DOCUMENTED-ONLY (tick 260; no registrations beyond the aura above):
--   npc_dragonflayer_forge_master (utgarde_keep.cpp:34-117) — Reset /
--   JustDied / JustEngagedWith call instance->SetData(DATA_FORGE_1 +
--   _forgeId - 1, NOT_STARTED / DONE / IN_PROGRESS), where _forgeId
--   (1-3) is the nearest GO_GLOWING_ANVIL_* within 30yd
--   (GetForgeMasterType, :91-108). STAYS OUT on two independent
--   grounds: (1) the entry is DB-side only — no C++ NPC_ constant, no
--   sql/ tree in this clone, world.db carries no creature_template
--   table — so no entry can be registered (the tick-1514 triage);
--   (2) the only consumer of the SetData writes is
--   instance_utgarde_keep::SetData -> HandleGameObject on the forge
--   anvil/bellow/fire GOs (instance_utgarde_keep.cpp:141-160), and
--   instance_utgarde_keep.lua registers no OnSetData handler while the
--   OnGameObjectCreate GO-GUID latches are vacuous (static template GOs
--   never pass the Go GO-create funnel) — the writes would fire an
--   event nothing consumes. (Round-26 correction: the SetData WRITE
--   bridge itself exists — instance_hooks.go:51, razorgore precedent —
--   and GO:SetGoState exists; the block is the missing consumer +
--   vacuous latches, not the write.) Re-activates when the forge GO
--   machine lands.
--   npc_enslaved_proto_drake (24083) is ported in
--   npc_enslaved_proto_drake.lua; spell_fixate is ported below
--   (40414 ev5) and spell_uk_second_wind (ported in procs.go) are
--   the remaining AddSC_utgarde_keep() registrations
--   (utgarde_keep.cpp:345-352).

local SPELL_TICKING_TIME_BOMB = 59686 -- TDB spell_script_names
local SPELL_TICKING_TIME_BOMB_EXPLODE = 59687 -- utgarde_keep.cpp:122
local SPELL_FIXATE = 40414 -- Wowhead WotLK Fixate: effect 2 Script Effect, value 40415; TDB spell_script_names binding DB-side
local SPELL_FIXATE_TRIGGER = 40415 -- utgarde_keep.cpp:159

-- C++ spell_ticking_time_bomb_AuraScript::HandleOnEffectRemove
-- (utgarde_keep.cpp:139-144).
local function tickingTimeBombRemove(event, aura, target)
    if aura:GetCasterGUID() ~= 0 and aura:GetCasterGUID() == target:GetGUID() then
        target:CastSpell(target, SPELL_TICKING_TIME_BOMB_EXPLODE, true)
    end
end

RegisterAuraEvent(SPELL_TICKING_TIME_BOMB, 3, tickingTimeBombRemove)

-- C++ spell_fixate::HandleScriptEffect (utgarde_keep.cpp:177-180):
-- OnEffectHitTarget, EFFECT_2 SPELL_EFFECT_SCRIPT_EFFECT -> the hit
-- unit casts Fixate Trigger (40415) on the caster, triggered. The
-- "no bridge" header claim was stale: ev5 is OnEffectHitTarget
-- (runtime.go:600-608) and the palehoof crazed_effect arm
-- (boss_palehoof.lua:824-831) ports the identical shape. No in-tree
-- cast site; DBC-bound. ev5 fires on the creature path and the
-- triggered funnel only — a player caster would stay blocked.
local function fixateHit(event, spell, hitTarget, effectIndex)
    if effectIndex == 2 and hitTarget ~= nil then
        local caster = spell:GetCaster()
        if caster ~= nil then
            hitTarget:CastSpell(caster, SPELL_FIXATE_TRIGGER, true)
        end
    end
end

RegisterSpellEvent(SPELL_FIXATE, 5, fixateHit)
