-- Boss Kirtonos the Herald (Scholomance)
-- Lua port of src/server/scripts/EasternKingdoms/Scholomance/
-- boss_kirtonos_the_herald.cpp
-- (boss_kirtonos_the_heraldAI — BossAI(creature, DATA_KIRTONOS) via
-- GetScholomanceAI -> GetInstanceAI; registered by
-- AddSC_boss_kirtonos_the_herald in eastern_kingdoms_script_loader.cpp
-- (declaration line 121, call line 299)). The Scholomance block is OPEN.
-- Entry (verifiable from the C++ sources): NPC_KIRTONOS = 10506 in the
-- Brazier_Of_The_Herald enum in boss_kirtonos_the_herald.cpp itself
-- (go_brazier_of_the_herald::OnGossipHello:
-- player->SummonCreature(NPC_KIRTONOS, PosSummon[0], ...)). Whole-server-
-- tree grep confirms boss_kirtonos_the_herald.cpp as the only source of
-- "kirtonos"/"Kirtonos" for the script registration (loader lines and
-- instance_scholomance.cpp's GO_GATE_KIRTONOS guid arms only otherwise).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Driven by CreateLuaEvent timers; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Verifiable numbers (file's own enums): SPELL_SWOOP = 18144 /
-- SPELL_WING_FLAP = 12882 / SPELL_PIERCE_ARMOR = 6016 / SPELL_DISARM =
-- 8379 / SPELL_KIRTONOS_TRANSFORM = 16467 / SPELL_SHADOW_BOLT = 17228 /
-- SPELL_CURSE_OF_TONGUES = 12889 / SPELL_DOMINATE_MIND = 14515;
-- WEAPON_KIRTONOS_STAFF = 11365 (no equipment bridge — skipped);
-- GO_GATE_KIRTONOS = 175570 (scholomance.h:48, instance-model blocked).
-- Ported arms (C++ JustEngagedWith + UpdateAI combat machine):
-- - SPELL_SWOOP: self-cast 18144 (DoCast(me)), 8s init -> 15s re-arm.
-- - SPELL_WING_FLAP: self-cast 12882 (DoCast(me)), 15s init -> 13s re-arm.
-- - SPELL_PIERCE_ARMOR: victim-cast 6016 (DoCastVictim, triggered),
--   18s init -> 12s re-arm.
-- - SPELL_DISARM: victim-cast 8379 (DoCastVictim, triggered), 22s init ->
--   11s re-arm.
-- - SPELL_SHADOW_BOLT: victim-cast 17228 (DoCastVictim, triggered),
--   42s init -> 42s re-arm.
-- - SPELL_CURSE_OF_TONGUES: victim-cast 12889 (DoCastVictim, triggered),
--   53s init -> 35s re-arm.
-- - SPELL_DOMINATE_MIND: victim-cast 14515 (DoCastVictim, triggered),
--   {34s, 48s} init -> {44s, 48s} re-arm.
-- - SPELL_KIRTONOS_TRANSFORM: 20s init -> {16s, 18s} re-arm toggle. The
--   C++ arm reads me->HasAura(16467) and removes or applies the aura;
--   there is no HasAura bridge, so the toggle state rides a per-guid Lua
--   latch (arlokk convention): latch clear -> DoCast(me, 16467),
--   latch set; latch set -> creature:RemoveAura(16467), latch clear.
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (creature casts
-- are packet-visual), so timers fire unconditionally (maiden precedent);
-- the transform arm's SetUInt32Value(UNIT_VIRTUAL_ITEM_SLOT_ID) staff
-- equip/clear and SetCanFly(true/false) legs have no bridges (zuljin
-- precedent) — only the aura itself is applied/removed.
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - The BossAI(creature, DATA_KIRTONOS) constructor and
--   GetScholomanceAI -> GetInstanceAI legs (instance-script model
--   blocked, standing).
-- - Reset -> _Reset() (instance-model leg) and the _JustDied() leg.
-- - JustDied / EnterEvadeMode: the gate/brazier GameObject arms
--   (instance->GetGuidData(GO_GATE_KIRTONOS) /
--   GO_BRAZIER_OF_THE_HERALD through ObjectAccessor + SetGoState /
--   ResetDoorOrButton; instance-script model blocked) and the 5s
--   DespawnOrUnsummon arm (no despawn bridge).
-- - IsSummonedBy + MovementInform INTRO_1..6 choreography: MovePath(
--   KIRTONOS_PATH = 105061) and MovePoint (no MotionMaster bridge —
--   selin_fireheart precedent); the gate SetGoState(GO_STATE_READY) arm
--   (instance-model blocked); SetFacingTo / SetWalk / SetDisableGravity
--   (no bridges); Talk(EMOTE_SUMMONED = 0) and the EMOTE_ONESHOT_ROAR
--   (emote legs fire inside the unmodeled intro); the staff equip arm
--   (no equipment bridge); UNIT_FLAG_NON_ATTACKABLE/NOT_SELECTABLE and
--   REACT_PASSIVE/REACT_AGGRESSIVE flag legs (no flag/react-state
--   bridges — koltira / eye_of_acherus precedents).
-- - go_brazier_of_the_herald::OnGossipHello (GameObjectScript): the
--   UseDoorOrButton / PlayDirectSound(SOUND_SCREECH = 557) arms have no
--   bridges and the player->SummonCreature(NPC_KIRTONOS, PosSummon[0],
--   TEMPSUMMON_DEAD_DESPAWN, 15min) arm has no summon bridge — skipped.

local ENTRY_KIRTONOS = 10506

local SPELL_SWOOP = 18144
local SPELL_WING_FLAP = 12882
local SPELL_PIERCE_ARMOR = 6016
local SPELL_DISARM = 8379
local SPELL_KIRTONOS_TRANSFORM = 16467
local SPELL_SHADOW_BOLT = 17228
local SPELL_CURSE_OF_TONGUES = 12889
local SPELL_DOMINATE_MIND = 14515

local timers = {}
local transformed = {}

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
    transformed[guid] = false
end

local function schedule(guid, key, delay, fn)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    per[key] = CreateLuaEvent(fn, delay)
end

local function onSwoop(creature, guid)
    creature:CastSpell(creature, SPELL_SWOOP)
    schedule(guid, "swoop", 15000, function() onSwoop(creature, guid) end)
end

local function onWingFlap(creature, guid)
    creature:CastSpell(creature, SPELL_WING_FLAP)
    schedule(guid, "wingflap", 13000, function() onWingFlap(creature, guid) end)
end

local function onPierceArmor(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_PIERCE_ARMOR, true)
    end
    schedule(guid, "piercearmor", 12000, function() onPierceArmor(creature, guid) end)
end

local function onDisarm(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_DISARM, true)
    end
    schedule(guid, "disarm", 11000, function() onDisarm(creature, guid) end)
end

local function onShadowBolt(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SHADOW_BOLT, true)
    end
    schedule(guid, "shadowbolt", 42000, function() onShadowBolt(creature, guid) end)
end

local function onCurseOfTongues(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CURSE_OF_TONGUES, true)
    end
    schedule(guid, "curseoftongues", 35000, function() onCurseOfTongues(creature, guid) end)
end

local function onDominateMind(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_DOMINATE_MIND, true)
    end
    schedule(guid, "dominatemind", math.random(44000, 48000), function() onDominateMind(creature, guid) end)
end

-- C++ EVENT_KIRTONOS_TRANSFORM: HasAura(16467) -> RemoveAura + ground
-- legs; else DoCast(me, 16467) + staff + fly legs. No HasAura / fly /
-- equipment bridges, so the toggle rides a Lua latch (arlokk
-- convention) and only the aura is applied/removed.
local function onTransform(creature, guid)
    if transformed[guid] then
        creature:RemoveAura(SPELL_KIRTONOS_TRANSFORM)
        transformed[guid] = false
    else
        creature:CastSpell(creature, SPELL_KIRTONOS_TRANSFORM)
        transformed[guid] = true
    end
    schedule(guid, "transform", math.random(16000, 18000), function() onTransform(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    initState(guid)
    schedule(guid, "swoop", 8000, function() onSwoop(creature, guid) end)
    schedule(guid, "wingflap", 15000, function() onWingFlap(creature, guid) end)
    schedule(guid, "piercearmor", 18000, function() onPierceArmor(creature, guid) end)
    schedule(guid, "disarm", 22000, function() onDisarm(creature, guid) end)
    schedule(guid, "shadowbolt", 42000, function() onShadowBolt(creature, guid) end)
    schedule(guid, "curseoftongues", 53000, function() onCurseOfTongues(creature, guid) end)
    schedule(guid, "dominatemind", math.random(34000, 48000), function() onDominateMind(creature, guid) end)
    schedule(guid, "transform", 20000, function() onTransform(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    initState(guid)
end

RegisterCreatureEvent(ENTRY_KIRTONOS, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_KIRTONOS, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_KIRTONOS, 4, onCombatEnd)
RegisterCreatureEvent(ENTRY_KIRTONOS, 23, onReset)
