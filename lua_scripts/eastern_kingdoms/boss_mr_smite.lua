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
-- 5 OnSpawn, 9 OnDamageTaken, 23 OnReset, 28 OnMovementInform. Driven by CreateLuaEvent
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
-- - Phase machine (boss_mr_smite.cpp:190-220, PORTED below): 2500ms after
--   each crossing, phase 1 walks to the chest (MoveTo point 1, chest pos
--   -1.5/+1.4 via GetGuidData + GetGameObjectByGUID with a
--   GetGameObjectsInGrid fallback — the Go instance store is never latched
--   for DATA_SMITE_CHEST, see findSmiteChest); ev28 arrival kneels +
--   faces 5.47, then phase 2 equips (axe/axe stage 1, mace stage 2) @2000ms,
--   phase 3 stands @500ms, phase 4 re-aggresses + MoveChase @750ms.
-- Deviations from C++: of the phase-transition ability-halt leg
-- (boss_mr_smite.cpp:141-144), AttackStop IS ported (bridge
-- lua_creature_events.go:180); SetCombatMovement(false) has a bridge
-- (lua_creature_events.go:759) but the mid-combat arm is a C++ no-op
-- (only AttackStart reads the flag) and porting it would diverge via
-- the per-tick gate (10:19); InterruptNonMeleeSpells(false) is bridged
-- (no-op twin, lua_creature_events.go:514) and ported in both stage arms
-- below. SetReactState(
-- REACT_PASSIVE) is PORTED at the crossings (its phase-4
-- SetReactState(REACT_AGGRESSIVE) restore rides the ported chain below);
-- Trash/Slam timers keep running
-- through the phase crossings (no !uiIsMoving gate). No
-- UNIT_STATE_CASTING model in Go (creature casts are packet-visual), so
-- timers fire unconditionally (maiden precedent).
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - Reset's SetStandState(UNIT_STAND_STATE_STAND) PORTED via
--   creature:SetStandState(0) (bridge landed 2026-10-21,
--   lua_creature_events.go:568; koltira precedent) +
--   SetNoCallAssistance(true) PORTED via
--   creature:SetNoCallAssistance(true) (bridge in
--   lua_creature_events.go; the sticky NoCallAssistance latch holds
--   it across the evade clear). The Reset
--   SetEquipmentSlots arm IS ported (onSpawn / onReset via the
--   EQUIP_NO_CHANGE sentinel), as is SetReactState(REACT_AGGRESSIVE).
-- - The phase/equip event machine is PORTED (see Ported arms above):
--   phase 1 chest walk via MoveTo + ev28, kneel/facing arrival, phase 2
--   equip, phase 3 stand, phase 4 re-aggress + MoveChase. The old
--   "no ObjectAccessor / MotionMaster / MovementInform bridge" claims are
--   stale (GetGuidData :7593 + GetGameObjectByGUID :4671 + MoveTo :5769 +
--   ev28, all live). JustEngagedWith
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
local REACT_PASSIVE = 0
local REACT_AGGRESSIVE = 2

local EQUIP_AXE = 5196 -- boss_mr_smite.cpp Equips
local EQUIP_MACE = 7230 -- boss_mr_smite.cpp Equips
local DATA_SMITE_CHEST = 0 -- deadmines.h DMData64 (first/only entry)
local GO_MR_SMITE_CHEST = 144111 -- deadmines.h DMGameObjectsIds
local SMITE_CHEST_POINT = 1 -- boss_mr_smite.cpp phase-1 MovePoint id
local POINT_MOTION_TYPE = 8 -- MovementDefines.h:31

local timers = {}
local phaseStage = {}
local smitePhase = {}

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
    smitePhase[guid] = 0
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

local function cancelKey(guid, key)
    local per = timers[guid]
    if per and per[key] then
        RemoveEventById(per[key])
        per[key] = nil
    end
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

-- C++ phase 4 (boss_mr_smite.cpp:208-215): SetReactState(REACT_AGGRESSIVE)
-- + SetCombatMovement(true) + MoveChase(victim, m_CombatDistance) +
-- uiIsMoving = false, uiPhase = 0. Fires 750ms after phase 3.
local function smitePhase4(creature, guid)
    smitePhase[guid] = 0
    creature:SetReactState(REACT_AGGRESSIVE)
    creature:SetCombatMovement(true)
    local victim = creature:GetVictim()
    if victim then
        creature:MoveChase(victim)
    end
end

-- C++ phase 3 (:203-207): SetStandState(STAND), uiTimer = 750, uiPhase = 4.
local function smitePhase3(creature, guid)
    creature:SetStandState(0)
    schedule(guid, "smitePhase4", 750, function() smitePhase4(creature, guid) end)
end

-- C++ phase 2 (:196-202): equip per uiHealth — stage 1 (66% crossing)
-- axe/axe, stage 2 (33%) mace/unequipped; uiTimer = 500, uiPhase = 3.
-- Fires 2000ms after the ev28 arrival (MovementInform's uiTimer/uiPhase).
local function smitePhase2(creature, guid)
    if phaseStage[guid] == 1 then
        creature:SetEquipmentSlots(EQUIP_AXE, EQUIP_AXE, EQUIP_NO_CHANGE)
    else
        creature:SetEquipmentSlots(EQUIP_MACE, EQUIP_UNEQUIP, EQUIP_NO_CHANGE)
    end
    schedule(guid, "smitePhase3", 500, function() smitePhase3(creature, guid) end)
end

-- C++ phase 1 (:190-195, after the 2500ms uiTimer): instance chest lookup
-- (DATA_SMITE_CHEST) -> MovePoint(1, goX - 1.5, goY + 1.4, goZ),
-- uiIsMoving = true. GetGuidData is bridged but the Go instance store is
-- only populated by Lua SetGuidData (instance_guid_store.go) and nothing
-- latches the chest, so the grid scan is the live twin (arlokk
-- precedent, entry 144111 fight-unique); the GUID path stays first so a
-- future instance-script port takes over. A missed lookup re-arms at
-- 2500ms — the C++ per-tick phase-1 retry.
local function findSmiteChest(creature)
    local chestGuid = creature:GetGuidData(DATA_SMITE_CHEST)
    if chestGuid ~= nil and chestGuid ~= 0 then
        local go = creature:GetGameObjectByGUID(chestGuid)
        if go then
            return go
        end
    end
    for _, go in ipairs(creature:GetGameObjectsInGrid(GO_MR_SMITE_CHEST, 150)) do
        return go
    end
    return nil
end

local function smiteWalkToChest(creature, guid)
    if smitePhase[guid] ~= 1 then
        return
    end
    local chest = findSmiteChest(creature)
    if not chest then
        schedule(guid, "smiteWalk", 2500, function() smiteWalkToChest(creature, guid) end)
        return
    end
    creature:MoveClear()
    creature:MoveTo(SMITE_CHEST_POINT, chest:GetX() - 1.5, chest:GetY() + 1.4, chest:GetZ())
end

-- C++ MovementInform (boss_mr_smite.cpp:211-220): POINT_MOTION_TYPE
-- arrival -> SetFacingTo(5.47f) + SetStandState(KNEEL) + uiTimer = 2000,
-- uiPhase = 2. Gated on the walk leg (smitePhase 1) so a stray point
-- arrival cannot re-run the chain.
local function smiteMovementInform(_, creature, moveType, id)
    local guid = creature:GetGUIDLow()
    if moveType ~= POINT_MOTION_TYPE or id ~= SMITE_CHEST_POINT or smitePhase[guid] ~= 1 then
        return
    end
    smitePhase[guid] = 2
    creature:SetFacingTo(5.47)
    creature:SetStandState(8)
    schedule(guid, "smitePhase2", 2000, function() smitePhase2(creature, guid) end)
end

local function onDamageTaken(_, creature, _, damage)
    local guid = creature:GetGUIDLow()
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
        creature:InterruptNonMeleeSpells(false)
        creature:SetReactState(REACT_PASSIVE)
        -- C++ boss_mr_smite.cpp:147-148: uiTimer = 2500, uiPhase = 1.
        -- A second crossing while a chain is pending overwrites uiPhase,
        -- so the pending phase 2/3/4 legs are cancelled with it.
        smitePhase[guid] = 1
        cancelKey(guid, "smitePhase2")
        cancelKey(guid, "smitePhase3")
        cancelKey(guid, "smitePhase4")
        cancelKey(guid, "smiteWalk")
        schedule(guid, "smiteWalk", 2500, function() smiteWalkToChest(creature, guid) end)
        creature:Talk(SAY_PHASE_1)
    elseif stage == 1 and health <= math.floor(maxHealth * 33 / 100) then
        phaseStage[guid] = 2
        creature:CastSpell(creature, SPELL_SMITE_STOMP)
        creature:AttackStop()
        creature:InterruptNonMeleeSpells(false)
        creature:SetReactState(REACT_PASSIVE)
        -- C++ boss_mr_smite.cpp:147-148: uiTimer = 2500, uiPhase = 1 (see
        -- stage 1 above for the pending-chain cancel).
        smitePhase[guid] = 1
        cancelKey(guid, "smitePhase2")
        cancelKey(guid, "smitePhase3")
        cancelKey(guid, "smitePhase4")
        cancelKey(guid, "smiteWalk")
        schedule(guid, "smiteWalk", 2500, function() smiteWalkToChest(creature, guid) end)
        creature:Talk(SAY_PHASE_2)
    end
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    initState(guid)
    schedule(guid, "trash", math.random(5000, 9000), function() onTrash(creature, guid) end)
    schedule(guid, "slam", 9000, function() onSlam(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUIDLow())
end

local function onSpawn(_, creature)
    initState(creature:GetGUIDLow())
    creature:SetEquipmentSlots(EQUIP_SWORD, EQUIP_UNEQUIP, EQUIP_NO_CHANGE)
    -- C++ Reset (boss_mr_smite.cpp:101): Creature::Respawn calls
    -- AI()->Reset() (Creature.cpp:2198), so spawn gets the same arm.
    creature:SetReactState(REACT_AGGRESSIVE)
end

local function onReset(_, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    initState(guid)
    -- C++ Reset arms (boss_mr_smite.cpp:99-103): equipment (see
    -- onSpawn) + SetStandState(STAND) + SetReactState(REACT_AGGRESSIVE)
    -- + SetNoCallAssistance(true), C++ order.
    creature:SetEquipmentSlots(EQUIP_SWORD, EQUIP_UNEQUIP, EQUIP_NO_CHANGE)
    creature:SetStandState(0)
    creature:SetReactState(REACT_AGGRESSIVE)
    creature:SetNoCallAssistance(true)
end

RegisterCreatureEvent(ENTRY_MR_SMITE, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_MR_SMITE, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_MR_SMITE, 4, onCombatEnd)
RegisterCreatureEvent(ENTRY_MR_SMITE, 5, onSpawn)
RegisterCreatureEvent(ENTRY_MR_SMITE, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY_MR_SMITE, 23, onReset)
RegisterCreatureEvent(ENTRY_MR_SMITE, 28, smiteMovementInform)
