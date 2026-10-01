-- Boss Mr. Smite (Deadmines) --
-- Lua port of src/server/scripts/EasternKingdoms/Deadmines/boss_mr_smite.cpp
-- (boss_mr_smiteAI — ScriptedAI via GetDeadminesAI; registered by
-- AddSC_boss_mr_smite in eastern_kingdoms_script_loader.cpp). Boss/zone-
-- script unit per the loader order; the Deadmines block is now OPEN.
-- Entry (verifiable from the C++ sources): NPC_MR_SMITE = 646 in the
-- DMCreaturesIds enum in Deadmines/deadmines.h (same-block header enum).
-- Whole-server-tree grep confirms boss_mr_smite.cpp as the only source
-- of "boss_mr_smite" (loader line in eastern_kingdoms_script_loader.cpp).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 5 OnSpawn, 9 OnDamageTaken, 23 OnReset. Driven by CreateLuaEvent
-- timers; melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
-- Verifiable numbers (file's own enums): SPELL_TRASH = 3391 /
-- SPELL_SMITE_STOMP = 6432 / SPELL_SMITE_SLAM = 6435; EQUIP_SWORD =
-- 5191 / EQUIP_AXE = 5196 / EQUIP_MACE = 7230; SAY_PHASE_1 = 2 /
-- SAY_PHASE_2 = 3; GO_MR_SMITE_CHEST = 144111 (deadmines.h).
-- Ported arms (C++ Initialize + UpdateAI combat machine):
-- - SPELL_TRASH: self-cast TRASH 3391 (DoCast(me)), {5,9}s init ->
--   {6,15.5}s re-arm, bCheckChances gate (urand(0,99) <= 15 skips).
-- - SPELL_SMITE_SLAM: victim-cast SMITE_SLAM 6435 (DoCastVictim via
--   GetVictim), 9s init -> 11s re-arm, bCheckChances gate.
-- - Phase transitions (the C++ per-tick !HealthAbovePct(66)/!HealthAbovePct
--   (33) checks, balinda DamageTaken-latch convention): the OnDamageTaken
--   handler fires with a fresh creature object built from live motion,
--   so (health - damage) is the genuine post-hit percent. 66% crossing
--   -> stomp (DoCastAOE 6432 ~ creature:CastSpell(creature, 6432)) +
--   Talk(SAY_PHASE_1 = 2); 33% crossing -> stomp + Talk(SAY_PHASE_2 = 3).
--   One-shot per stage via the per-guid phaseStage latch.
-- Deviations from C++: the !uiIsMoving ability-halt leg of the phase
-- transition (SetCombatMovement(false)/AttackStop/
-- InterruptNonMeleeSpells/REACT_PASSIVE) has no bridges, so Trash/Slam
-- keep their timers through the phase crossings; no UNIT_STATE_CASTING
-- model in Go (creature casts are packet-visual), so timers fire
-- unconditionally (maiden precedent).
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - Reset's SetEquipmentSlots(false, EQUIP_SWORD, EQUIP_UNEQUIP,
--   EQUIP_NO_CHANGE) (no equipment bridge — koltira precedent) +
--   SetStandState(UNIT_STAND_STATE_STAND) (no stand-state bridge —
--   koltira precedent) + SetReactState(REACT_AGGRESSIVE) (no react-state
--   bridge — eye_of_acherus precedent) + SetNoCallAssistance(true)
--   (no bridge).
-- - The phase/equip event machine: phase 1's instance chest lookup
--   (instance->GetGuidData(DATA_SMITE_CHEST), GO 144111) through
--   ObjectAccessor (instance-script model blocked; no ObjectAccessor
--   bridge — dark-rider precedent) + GetMotionMaster()->MovePoint(1)
--   (no MotionMaster bridge — selin_fireheart precedent);
--   MovementInform(POINT_MOTION_TYPE) -> SetFacingTo(5.47f) + kneel +
--   phase 2 (no MovementInform bridge — kalecgos precedent); phase 2's
--   SetEquipmentSlots(EQUIP_AXE x2 / EQUIP_MACE) (no equipment bridge);
--   phase 3's SetStandState(STAND) (no bridge); phase 4's
--   SetCombatMovement(true) + MoveChase (no bridges). JustEngagedWith
--   is empty in C++ — nothing to port.

local SPELL_TRASH = 3391
local SPELL_SMITE_STOMP = 6432
local SPELL_SMITE_SLAM = 6435

local SAY_PHASE_1 = 2
local SAY_PHASE_2 = 3

local ENTRY_MR_SMITE = 646

local timers = {}
local phaseStage = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

local function initState(guid)
    phaseStage[guid] = 0
end

local function schedule(guid, key, delay, fn)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    per[key] = CreateLuaEvent(fn, delay)
end

-- bCheckChances: urand(0, 99) <= 15 skips the cast.
local function checkChances()
    return math.random(0, 99) > 15
end

local function onTrash(creature, guid)
    if checkChances() then
        creature:CastSpell(creature, SPELL_TRASH)
    end
    schedule(guid, "trash", math.random(6000, 15500), function() onTrash(creature, guid) end)
end

local function onSlam(creature, guid)
    local victim = creature:GetVictim()
    if victim and checkChances() then
        creature:CastSpell(victim, SPELL_SMITE_SLAM)
    end
    schedule(guid, "slam", 11000, function() onSlam(creature, guid) end)
end

local function onDamageTaken(_, creature, _, damage)
    local guid = creature:GetGUID()
    local stage = phaseStage[guid]
    if stage == nil or stage >= 2 then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    local pct = (creature:GetHealth() - damage) * 100 / maxHealth
    if stage == 0 and pct <= 66 then
        phaseStage[guid] = 1
        creature:CastSpell(creature, SPELL_SMITE_STOMP)
        creature:Talk(SAY_PHASE_1)
    elseif stage == 1 and pct <= 33 then
        phaseStage[guid] = 2
        creature:CastSpell(creature, SPELL_SMITE_STOMP)
        creature:Talk(SAY_PHASE_2)
    end
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    initState(guid)
    schedule(guid, "trash", math.random(5000, 9000), function() onTrash(creature, guid) end)
    schedule(guid, "slam", 9000, function() onSlam(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

local function onSpawn(_, creature)
    initState(creature:GetGUID())
end

local function onReset(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    initState(guid)
end

RegisterCreatureEvent(ENTRY_MR_SMITE, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_MR_SMITE, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_MR_SMITE, 4, onCombatEnd)
RegisterCreatureEvent(ENTRY_MR_SMITE, 5, onSpawn)
RegisterCreatureEvent(ENTRY_MR_SMITE, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY_MR_SMITE, 23, onReset)
