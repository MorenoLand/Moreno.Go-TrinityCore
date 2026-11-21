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
-- 6 OnReachWP, 22 OnSummoned, 23 OnReset. Driven by CreateLuaEvent timers; melee is engine-driven
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
-- Deviations from C++: UpdateAI UNIT_STATE_CASTING queue gate PORTED:
-- each combat arm below defers 500ms while creature:IsCasting() (live
-- CastingUntil window, lua_creature_events.go:2480; broodkeeper
-- precedent) — no re-arm consumed; the non-victim intro machine branch
-- is unmodeled, so only the 8 combat timer callbacks are gated;
-- the transform arm's SetCanFly(true/false) legs are PORTED 2026-10-22
-- (the old "no bridge" note was stale);
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - The BossAI(creature, DATA_KIRTONOS) constructor and
--   GetScholomanceAI -> GetInstanceAI legs (instance-script model
--   blocked, standing).
-- - Reset -> _Reset() (instance-model leg) and the _JustDied() leg.
-- - JustDied / EnterEvadeMode: the gate/brazier SetGoState arms
--   PORTED below via creature:FindNearestGameObject (both entries
--   unique in the instance; C++ used instance GetGuidData).
--   ResetDoorOrButton stays out (no bridge); _JustDied() leg stays out;
--   the old "no despawn bridge" on the 5s DespawnOrUnsummon arm was
--   stale (creature:Despawn() is bridged, lua_creature_events.go:3318).
-- - IsSummonedBy + MovementInform INTRO_1..6 choreography PORTED
--   below (ev22 + ev6 + MovePath/MoveTo bridges): INTRO_1 MovePath(
--   KIRTONOS_PATH = 840488, false) — TDB 26091 id (C++ enum 105061 x8,
--   same 14 nodes); waypoint arrival at node index 13
--   (POINT_KIRTONOS_LAND, the C++ _currentNode index — ev6's id arg,
--   not the DB point id) -> INTRO_2 after 1500ms; INTRO_2 MoveTo
--   PosMove[0] + INTRO_3 after 1s; INTRO_3 gate SetGoState(READY) +
--   SetFacingTo(0.01745329) + INTRO_4 after 3s; INTRO_4 brazier
--   SetGoState(READY) + SetWalk(true) + SetDisableGravity(false) +
--   DoCast(KIRTONOS_TRANSFORM) + SetCanFly(false) + INTRO_5 after 1s;
--   INTRO_5 PerformEmote(ROAR) + staff equip + RemoveFlag(
--   NON_ATTACKABLE|NOT_SELECTABLE) + SetReactState(AGGRESSIVE) +
--   INTRO_6 after 5s; INTRO_6 MoveTo PosMove[1]. The gate/brazier
--   SetGoState legs use the same FindNearestGameObject pattern as
--   JustDied/EnterEvadeMode (entries unique in the instance).
-- - go_brazier_of_the_herald::OnGossipHello (GameObjectScript, registered
--   by new go_brazier_of_the_herald): me->UseDoorOrButton() is ported via
--   go:UseDoorOrButton (bridged, engine/world/lua_objects.go:1113) on the
--   brazier GO gossip hook (onBrazierGossip); player->SummonCreature(
--   NPC_KIRTONOS, PosSummon[0], TEMPSUMMON_DEAD_DESPAWN, 15min) PORTED
--   via player:SummonCreature on the same hook; me->PlayDirectSound(SOUND_SCREECH = 557)
--   is ported via go:PlayDirectSound on the same hook. (The 2026-10-18
--   "no door bridge" claim was stale; corrected 2026-10-19.)
-- - ev19 JustSummoned (2026-11-13 triage): ALREADY PORTED / no-op —
--   no handler, no SummonList twin. C++
--   boss_kirtonos_the_herald.cpp:136-139 is bare BossAI::JustSummoned
--   (ScriptedCreature.cpp:559-564): summons.Summon(summon) plus
--   DoZoneInCombat(summon) if engaged. The boss AI summons nothing —
--   the file's only SummonCreature is the brazier's
--   player->SummonCreature(NPC_KIRTONOS, ...) at :293, which summons
--   Kirtonos himself — so the SummonList stays empty and the
--   _Reset/_JustDied DespawnAll legs have nothing to despawn. ev19 IS
--   bridged (fireCreatureSummoned, lua_creature_events.go:8002), but it
--   can never fire for entry 10506 as summoner; player summoners pass
--   nil SummonerMotion, so no ev19 fires for the brazier summon either
--   (lua_creature_events.go:67-72). No terestian-style twin needed.

local ENTRY_KIRTONOS = 10506

local EQUIP_NO_CHANGE = 0xFFFFFFFF

local WEAPON_KIRTONOS_STAFF = 11365

local GO_BRAZIER_OF_THE_HERALD = 175564
local GO_GATE_KIRTONOS = 175570 -- scholomance.h:48
local POS_SUMMON = {315.028, 70.53845, 102.1496, 0.3859715} -- C++ PosSummon[0]
local POS_MOVE = { -- C++ PosMove[2] (boss_kirtonos_the_herald.cpp:71-75)
    {299.4884, 92.76137, 105.6335},
    {314.8673, 90.30210, 101.6459},
}
local KIRTONOS_PATH = 840488 -- TDB 26091 id; C++ enum KIRTONOS_PATH = 105061
local POINT_KIRTONOS_LAND = 13 -- C++ _currentNode index, not the DB point id
local EMOTE_SUMMONED = 0
local EMOTE_ONESHOT_ROAR = 15 -- SharedDefines.h:1950
local REACT_PASSIVE = 0
local REACT_AGGRESSIVE = 2
local UNIT_FLAG_NON_ATTACKABLE = 0x00000002
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000
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
    if creature:IsCasting() then
        schedule(guid, "swoop", 500, function() onSwoop(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_SWOOP)
    schedule(guid, "swoop", 15000, function() onSwoop(creature, guid) end)
end

local function onWingFlap(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "wingflap", 500, function() onWingFlap(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_WING_FLAP)
    schedule(guid, "wingflap", 13000, function() onWingFlap(creature, guid) end)
end

local function onPierceArmor(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "piercearmor", 500, function() onPierceArmor(creature, guid) end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_PIERCE_ARMOR, true)
    end
    schedule(guid, "piercearmor", 12000, function() onPierceArmor(creature, guid) end)
end

local function onDisarm(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "disarm", 500, function() onDisarm(creature, guid) end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_DISARM, true)
    end
    schedule(guid, "disarm", 11000, function() onDisarm(creature, guid) end)
end

local function onShadowBolt(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "shadowbolt", 500, function() onShadowBolt(creature, guid) end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SHADOW_BOLT, true)
    end
    schedule(guid, "shadowbolt", 42000, function() onShadowBolt(creature, guid) end)
end

local function onCurseOfTongues(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "curseoftongues", 500, function() onCurseOfTongues(creature, guid) end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CURSE_OF_TONGUES, true)
    end
    schedule(guid, "curseoftongues", 35000, function() onCurseOfTongues(creature, guid) end)
end

local function onDominateMind(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "dominatemind", 500, function() onDominateMind(creature, guid) end)
        return
    end
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
    if creature:IsCasting() then
        schedule(guid, "transform", 500, function() onTransform(creature, guid) end)
        return
    end
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

-- C++ INTRO_6 (boss_kirtonos_the_herald.cpp:186-187): MovePoint(0,
-- PosMove[1]) via creature:MoveTo (MovePoint bridge, thrall-guard
-- precedent); the ev28 arrival has no C++ arm.
local function onIntro6(creature, guid)
    creature:MoveTo(0, POS_MOVE[2][1], POS_MOVE[2][2], POS_MOVE[2][3])
end

-- C++ INTRO_5 (:179-185): roar emote, staff equip (slot 0 only),
-- clear NON_ATTACKABLE|NOT_SELECTABLE, REACT_AGGRESSIVE, INTRO_6 in 5s.
local function onIntro5(creature, guid)
    creature:PerformEmote(EMOTE_ONESHOT_ROAR)
    creature:SetEquipmentSlots(WEAPON_KIRTONOS_STAFF, EQUIP_NO_CHANGE, EQUIP_NO_CHANGE)
    creature:RemoveFlag(59, UNIT_FLAG_NON_ATTACKABLE + UNIT_FLAG_NOT_SELECTABLE)
    creature:SetReactState(REACT_AGGRESSIVE)
    schedule(guid, "intro6", 5000, function() onIntro6(creature, guid) end)
end

-- C++ INTRO_4 (:170-177): brazier READY, walk, gravity off, transform
-- cast, can-fly off, INTRO_5 in 1s.
local function onIntro4(creature, guid)
    local brazier = creature:FindNearestGameObject(GO_BRAZIER_OF_THE_HERALD, 100.0)
    if brazier then brazier:SetGoState(1) end -- GO_STATE_READY
    creature:SetWalk(true)
    creature:SetDisableGravity(false)
    creature:CastSpell(creature, SPELL_KIRTONOS_TRANSFORM)
    creature:SetCanFly(false)
    schedule(guid, "intro5", 1000, function() onIntro5(creature, guid) end)
end

-- C++ INTRO_3 (:164-168): gate READY, face 0.01745329, INTRO_4 in 3s.
local function onIntro3(creature, guid)
    local gate = creature:FindNearestGameObject(GO_GATE_KIRTONOS, 100.0)
    if gate then gate:SetGoState(1) end -- GO_STATE_READY
    creature:SetFacingTo(0.01745329)
    schedule(guid, "intro4", 3000, function() onIntro4(creature, guid) end)
end

-- C++ INTRO_2 (:160-162): MovePoint(0, PosMove[0]) via creature:MoveTo,
-- INTRO_3 in 1s.
local function onIntro2(creature, guid)
    creature:MoveTo(0, POS_MOVE[1][1], POS_MOVE[1][2], POS_MOVE[1][3])
    schedule(guid, "intro3", 1000, function() onIntro3(creature, guid) end)
end

-- C++ INTRO_1 (:157-159): MovePath(KIRTONOS_PATH, false).
local function onIntro1(creature, guid)
    creature:MovePath(KIRTONOS_PATH, false)
end

-- C++ MovementInform (:141-145): WAYPOINT_MOTION_TYPE arrival at node
-- index POINT_KIRTONOS_LAND (13) schedules INTRO_2 after 1500ms. The
-- ev6 id arg is the node index (C++ _currentNode), matching the C++
-- check exactly; other waypoint arrivals are ignored.
local function onReachWP(_, creature, moveType, pointId)
    if moveType ~= 2 then -- WAYPOINT_MOTION_TYPE
        return
    end
    if pointId == POINT_KIRTONOS_LAND then
        local guid = creature:GetGUIDLow()
        schedule(guid, "intro2", 1500, function() onIntro2(creature, guid) end)
    end
end

-- C++ IsSummonedBy (:127-134): INTRO_1 in 500ms, gravity off, passive,
-- NON_ATTACKABLE|NOT_SELECTABLE, Talk(EMOTE_SUMMONED).
local function onSummoned(_, creature)
    local guid = creature:GetGUIDLow()
    creature:SetDisableGravity(true)
    creature:SetReactState(REACT_PASSIVE)
    creature:SetFlag(59, UNIT_FLAG_NON_ATTACKABLE + UNIT_FLAG_NOT_SELECTABLE)
    creature:Talk(EMOTE_SUMMONED)
    schedule(guid, "intro1", 500, function() onIntro1(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "swoop", 8000, function() onSwoop(creature, guid) end)
    schedule(guid, "wingflap", 15000, function() onWingFlap(creature, guid) end)
    schedule(guid, "piercearmor", 18000, function() onPierceArmor(creature, guid) end)
    schedule(guid, "disarm", 22000, function() onDisarm(creature, guid) end)
    schedule(guid, "shadowbolt", 42000, function() onShadowBolt(creature, guid) end)
    schedule(guid, "curseoftongues", 53000, function() onCurseOfTongues(creature, guid) end)
    schedule(guid, "dominatemind", math.random(34000, 48000), function() onDominateMind(creature, guid) end)
    schedule(guid, "transform", 20000, function() onTransform(creature, guid) end)
    creature:SetCombatPulseDelay(5) -- BossAI::JustEngagedWith arm 1 (ScriptedCreature.cpp:540; boss_kirtonos_the_herald.cpp:100 base call last)
    creature:DoZoneInCombat() -- BossAI::JustEngagedWith me-form arm (ScriptedCreature.cpp:542; boss_kirtonos_the_herald.cpp:100 base call last)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUIDLow())
    -- C++ JustDied (boss_kirtonos_the_herald.cpp:106-113): gate ACTIVE,
    -- brazier READY (ResetDoorOrButton leg unbridged, skipped).
    local gate = creature:FindNearestGameObject(GO_GATE_KIRTONOS, 100.0)
    if gate then gate:SetGoState(0) end -- GO_STATE_ACTIVE
    local brazier = creature:FindNearestGameObject(GO_BRAZIER_OF_THE_HERALD, 100.0)
    if brazier then brazier:SetGoState(1) end -- GO_STATE_READY
end

local function onEvade(_, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    -- C++ EnterEvadeMode (:115-124): gate ACTIVE, brazier READY, then
    -- the 5s self-despawn below (ResetDoorOrButton leg unbridged).
    local gate = creature:FindNearestGameObject(GO_GATE_KIRTONOS, 100.0)
    if gate then gate:SetGoState(0) end -- GO_STATE_ACTIVE
    local brazier = creature:FindNearestGameObject(GO_BRAZIER_OF_THE_HERALD, 100.0)
    if brazier then brazier:SetGoState(1) end -- GO_STATE_READY
    -- C++ EnterEvadeMode (boss_kirtonos_the_herald.cpp:115-124):
    -- me->DespawnOrUnsummon(5s) — the gate/brazier SetGoState legs are
    -- PORTED above (FindNearestGameObject); the 5s self-despawn is a genuine
    -- delayed despawn (terestian precedent).
    CreateLuaEvent(function()
        creature:Despawn()
    end, 5000)
end

local function onReset(_, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
end

-- go_brazier_of_the_herald::OnGossipHello
-- (boss_kirtonos_the_herald.cpp:290-294): me->UseDoorOrButton() (bridged,
-- ported below) + me->PlayDirectSound(SOUND_SCREECH, 0) +
-- player->SummonCreature(NPC_KIRTONOS, PosSummon[0],
-- TEMPSUMMON_DEAD_DESPAWN, 15min) — PORTED via player:SummonCreature
-- (lua_player.go; shares the creature-side summonTempCreature funnel,
-- no event-19 fire for player summoners).
-- C++ returns true (handled: native GO use suppressed) == Lua true
-- (Eluna GOSSIP_EVENT_ON_HELLO: only a Lua false lets native use
-- proceed; see engine/world/gameobject_hooks.go).
local function onBrazierGossip(_, player, go)
    go:UseDoorOrButton()
    go:PlayDirectSound(SOUND_SCREECH)
    player:SummonCreature(ENTRY_KIRTONOS, POS_SUMMON[1], POS_SUMMON[2], POS_SUMMON[3], POS_SUMMON[4], 6, 900000)
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
RegisterCreatureEvent(ENTRY_KIRTONOS, 6, onReachWP)
RegisterCreatureEvent(ENTRY_KIRTONOS, 22, onSummoned)
RegisterCreatureEvent(ENTRY_KIRTONOS, 23, onReset)

RegisterGameObjectGossipEvent(GO_BRAZIER_OF_THE_HERALD, 1, onBrazierGossip)
