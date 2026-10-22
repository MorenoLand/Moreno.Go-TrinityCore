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
-- Deviations from C++: of the phase-transition ability-halt leg
-- (boss_mr_smite.cpp:141-144), AttackStop IS ported (bridge
-- lua_creature_events.go:180); SetCombatMovement(false) has a bridge
-- (lua_creature_events.go:670) but the mid-combat arm is a C++ no-op
-- (only AttackStart reads the flag) and porting it would diverge via
-- the per-tick gate (10:19); InterruptNonMeleeSpells(false) has no
-- bridge. SetReactState(
-- REACT_PASSIVE) is deliberately OMITTED, not unbridgeable: its restore
-- (phase 4's SetReactState(REACT_AGGRESSIVE)) rides the unbridged
-- MovePoint/kneel/equip/stand chain, so a one-sided passive port would
-- freeze the boss passive forever; Trash/Slam timers keep running
-- through the phase crossings (no !uiIsMoving gate). No
-- UNIT_STATE_CASTING model in Go (creature casts are packet-visual), so
-- timers fire unconditionally (maiden precedent).
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - Reset's SetStandState(UNIT_STAND_STATE_STAND) PORTED via
--   creature:SetStandState(0) (bridge landed 2026-10-21,
--   lua_creature_events.go:568; koltira precedent) +
--   SetNoCallAssistance(true) (no bridge). The Reset
--   SetEquipmentSlots arm IS ported (onSpawn / onReset via the
--   EQUIP_NO_CHANGE sentinel), as is SetReactState(REACT_AGGRESSIVE).
-- - The SetReactState(REACT_AGGRESSIVE) restore (phase 4 of the transition
--   machine; see Deviations above) + the phase/equip event machine: phase 1's
--   instance chest lookup
--   (instance->GetGuidData(DATA_SMITE_CHEST), GO 144111) through
--   ObjectAccessor (instance-script model blocked; no ObjectAccessor
--   bridge — dark-rider precedent) + GetMotionMaster()->MovePoint(1)
--   (no MotionMaster bridge — selin_fireheart precedent);
--   MovementInform(POINT_MOTION_TYPE) -> SetFacingTo(5.47f) + kneel +
--   phase 2 (no MovementInform bridge — kalecgos precedent); phase 2's
--   SetEquipmentSlots(EQUIP_AXE x2 / EQUIP_MACE) arms fire inside the
--   same unmodeled phase machine (the bridge exists — see onReset);
--   phase 3's SetStandState(STAND) (bridge live — phase machine keeps it out);
--   phase 4's SetCombatMovement(true) (bridged 06:45 — phase machine keeps it out)
--   + MoveChase (no bridge). JustEngagedWith
--   is empty in C++ — nothing to port.

local SPELL_TRASH = 3391
local SPELL_SMITE_STOMP = 6432
local SPELL_SMITE_SLAM = 6435

local SAY_PHASE_1 = 2
local SAY_PHASE_2 = 3

local ENTRY_MR_SMITE = 646

-- C++ Reset: SetEquipmentSlots(false, EQUIP_SWORD, EQUIP_UNEQUIP,
-- EQUIP_NO_CHANGE) (boss_mr_smite.cpp:41/99): main hand 5191, off hand
-- unequipped, ranged untouched (0xFFFFFFFF NO_CHANGE sentinel).
local EQUIP_SWORD = 5191
local EQUIP_UNEQUIP = 0
local EQUIP_NO_CHANGE = 0xFFFFFFFF

-- UnitDefines.h react states (engine/world/lua_creature_events.go:524):
-- REACT_PASSIVE = 0 / REACT_DEFENSIVE = 1 / REACT_AGGRESSIVE = 2.
local REACT_AGGRESSIVE = 2

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
    if per[key] then
        RemoveEventById(per[key])
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
    -- C++ UpdateAI !HealthAbovePct(66/33) on current (post-damage)
    -- health (jeklik convention): health <= floor(maxHealth*pct/100).
    -- Eluna event 9 fires pre-application, so the port subtracts the
    -- incoming damage to get the genuine post-hit health.
    -- nefarian rule: the C++ UpdateAI !HealthAbovePct arms never fire on
    -- the killing blow, which no UpdateAI tick follows.
    local health = creature:GetHealth() - damage
    if health <= 0 then
        return
    end
    if stage == 0 and health <= math.floor(maxHealth * 66 / 100) then
        phaseStage[guid] = 1
        creature:CastSpell(creature, SPELL_SMITE_STOMP)
        -- C++ boss_mr_smite.cpp:142: me->AttackStop() (bridge:
        -- lua_creature_events.go:180; drops the current swing, threat
        -- untouched, re-engagement rides the assist/aggro paths).
        creature:AttackStop()
        creature:Talk(SAY_PHASE_1)
    elseif stage == 1 and health <= math.floor(maxHealth * 33 / 100) then
        phaseStage[guid] = 2
        creature:CastSpell(creature, SPELL_SMITE_STOMP)
        creature:AttackStop()
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
    creature:SetEquipmentSlots(EQUIP_SWORD, EQUIP_UNEQUIP, EQUIP_NO_CHANGE)
    -- C++ Reset (boss_mr_smite.cpp:101): Creature::Respawn calls
    -- AI()->Reset() (Creature.cpp:2198), so spawn gets the same arm.
    creature:SetReactState(REACT_AGGRESSIVE)
end

local function onReset(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    initState(guid)
    -- C++ Reset arms (boss_mr_smite.cpp:99-103): equipment (see
    -- onSpawn) + SetStandState(STAND) + SetReactState(REACT_AGGRESSIVE),
    -- C++ order.
    creature:SetEquipmentSlots(EQUIP_SWORD, EQUIP_UNEQUIP, EQUIP_NO_CHANGE)
    creature:SetStandState(0)
    creature:SetReactState(REACT_AGGRESSIVE)
end

RegisterCreatureEvent(ENTRY_MR_SMITE, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_MR_SMITE, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_MR_SMITE, 4, onCombatEnd)
RegisterCreatureEvent(ENTRY_MR_SMITE, 5, onSpawn)
RegisterCreatureEvent(ENTRY_MR_SMITE, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY_MR_SMITE, 23, onReset)
