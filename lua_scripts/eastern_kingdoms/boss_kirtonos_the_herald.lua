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
-- WEAPON_KIRTONOS_STAFF = 11365 (staff equip/clear legs ported via
-- creature:SetEquipmentSlots with the 0xFFFFFFFF NO_CHANGE sentinel —
-- see onTransform);
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
--   creature:HasAura is bridged, so the toggle reads the real aura
--   state C++-exact. The old per-guid latch's evade caveat is gone:
--   C++ EnterEvadeMode despawns the boss without touching the aura,
--   so a fresh re-summon starts aura-clean exactly as C++ does.
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (creature casts
-- are packet-visual), so timers fire unconditionally (maiden precedent);
-- the transform arm's SetCanFly(true/false) legs are PORTED 2026-10-22
-- (the old "no bridge" note was stale);
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - The BossAI(creature, DATA_KIRTONOS) constructor and
--   GetScholomanceAI -> GetInstanceAI legs (instance-script model
--   blocked, standing).
-- - Reset -> _Reset() (instance-model leg) and the _JustDied() leg.
-- - JustDied / EnterEvadeMode: the gate/brazier GameObject arms
--   (instance->GetGuidData(GO_GATE_KIRTONOS) /
--   GO_BRAZIER_OF_THE_HERALD through ObjectAccessor + SetGoState /
--   ResetDoorOrButton; instance-script model blocked — the true bar;
--   the old "no despawn bridge" on the 5s DespawnOrUnsummon arm was
--   stale (creature:Despawn() is bridged, lua_creature_events.go:214).
-- - IsSummonedBy + MovementInform INTRO_1..6 choreography: MovePath(
--   KIRTONOS_PATH = 105061) and MovePoint (no MotionMaster bridge —
--   selin_fireheart precedent); the gate SetGoState(GO_STATE_READY) arm
--   (instance-model blocked); SetFacingTo / SetWalk / SetDisableGravity
--   (no bridges); Talk(EMOTE_SUMMONED = 0) and the EMOTE_ONESHOT_ROAR
--   (emote legs fire inside the unmodeled intro); the INTRO_5 staff equip
--   arm fires inside the same unmodeled intro choreography (the combat
--   transform-toggle staff legs are ported in onTransform); UNIT_FLAG_NON_ATTACKABLE/NOT_SELECTABLE and
--   REACT_PASSIVE/REACT_AGGRESSIVE flag legs (the SetFlag(59)/
--   SetReactState bridges exist since 2026-10-20, but these legs fire
--   inside the unported MovementInform intro choreography, so they
--   stay out — koltira / eye_of_acherus precedents for the machine).
-- - go_brazier_of_the_herald::OnGossipHello (GameObjectScript, registered
--   by new go_brazier_of_the_herald): me->UseDoorOrButton() is ported via
--   go:UseDoorOrButton (bridged, engine/world/lua_objects.go:1113) on the
--   brazier GO gossip hook (onBrazierGossip); player->SummonCreature(
--   NPC_KIRTONOS, PosSummon[0], TEMPSUMMON_DEAD_DESPAWN, 15min) has no
--   summon bridge — skipped; me->PlayDirectSound(SOUND_SCREECH = 557)
--   is ported via go:PlayDirectSound on the same hook. (The 2026-10-18
--   "no door bridge" claim was stale; corrected 2026-10-19.)

local ENTRY_KIRTONOS = 10506

local EQUIP_NO_CHANGE = 0xFFFFFFFF

local WEAPON_KIRTONOS_STAFF = 11365

local GO_BRAZIER_OF_THE_HERALD = 175564
local SOUND_SCREECH = 557

local SPELL_SWOOP = 18144
local SPELL_WING_FLAP = 12882
local SPELL_PIERCE_ARMOR = 6016
local SPELL_DISARM = 8379
local SPELL_KIRTONOS_TRANSFORM = 16467
local SPELL_SHADOW_BOLT = 17228
local SPELL_CURSE_OF_TONGUES = 12889
local SPELL_DOMINATE_MIND = 14515

local timers = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
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

-- C++ EVENT_KIRTONOS_TRANSFORM: HasAura(16467) -> RemoveAura + staff
-- clear + ground legs; else DoCast(me, 16467) + staff equip + fly legs.
-- creature:HasAura is bridged, so the toggle reads the real aura state
-- C++-exact. The staff legs are ported via creature:SetEquipmentSlots
-- with EQUIP_NO_CHANGE (0xFFFFFFFF) on the untouched slots, mirroring
-- C++ SetUInt32Value(UNIT_VIRTUAL_ITEM_SLOT_ID + 0, 11365/0) which
-- writes slot 0 only. The SetCanFly(true/false) legs ARE bridged
-- (lua_creature_events.go, PORTED 2026-10-22; the old "no bridge"
-- note was stale).
local function onTransform(creature, guid)
    if creature:HasAura(SPELL_KIRTONOS_TRANSFORM) then
        creature:RemoveAura(SPELL_KIRTONOS_TRANSFORM)
        creature:SetEquipmentSlots(0, EQUIP_NO_CHANGE, EQUIP_NO_CHANGE)
        creature:SetCanFly(false) -- C++ :237, after the staff clear
    else
        creature:CastSpell(creature, SPELL_KIRTONOS_TRANSFORM)
        creature:SetEquipmentSlots(WEAPON_KIRTONOS_STAFF, EQUIP_NO_CHANGE, EQUIP_NO_CHANGE)
        creature:SetCanFly(true) -- C++ :243, after the staff equip
    end
    schedule(guid, "transform", math.random(16000, 18000), function() onTransform(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
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

local function onEvade(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    -- C++ EnterEvadeMode (boss_kirtonos_the_herald.cpp:115-124):
    -- me->DespawnOrUnsummon(5s) — the gate/brazier GO legs ride the
    -- unbridged instance-GO machine; the 5s self-despawn is a genuine
    -- delayed despawn (terestian precedent).
    CreateLuaEvent(function()
        creature:Despawn()
    end, 5000)
end

local function onReset(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
end

-- go_brazier_of_the_herald::OnGossipHello
-- (boss_kirtonos_the_herald.cpp:290-294): me->UseDoorOrButton() (bridged,
-- ported below) + me->PlayDirectSound(SOUND_SCREECH, 0) +
-- player->SummonCreature(NPC_KIRTONOS, PosSummon[0],
-- TEMPSUMMON_DEAD_DESPAWN, 15min) (no summon bridge — skipped).
-- C++ returns true (handled: native GO use suppressed) == Lua true
-- (Eluna GOSSIP_EVENT_ON_HELLO: only a Lua false lets native use
-- proceed; see engine/world/gameobject_hooks.go).
local function onBrazierGossip(_, player, go)
    go:UseDoorOrButton()
    go:PlayDirectSound(SOUND_SCREECH)
    -- C++ go_brazier_of_the_herald::OnGossipHello returns true (handled:
    -- native GO use suppressed). Eluna GOSSIP_EVENT_ON_HELLO polarity
    -- (in-tree GossipHooks.cpp:27-34, GameObject.cpp:1500-1503): a Lua
    -- handler returning false lets native use PROCEED, so handled ==
    -- return true here. (A 2026-10-19 edit briefly had this inverted.)
    return true
end

RegisterCreatureEvent(ENTRY_KIRTONOS, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_KIRTONOS, 2, onEvade)
RegisterCreatureEvent(ENTRY_KIRTONOS, 4, onCombatEnd)
RegisterCreatureEvent(ENTRY_KIRTONOS, 23, onReset)

RegisterGameObjectGossipEvent(GO_BRAZIER_OF_THE_HERALD, 1, onBrazierGossip)
