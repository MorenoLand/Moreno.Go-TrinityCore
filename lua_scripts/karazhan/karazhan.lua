-- Karazhan zone file — Lua port of
-- src/server/scripts/EasternKingdoms/Karazhan/karazhan.cpp and
-- src/server/scripts/EasternKingdoms/Karazhan/instance_karazhan.cpp
-- (karazhan.h). Entries: npc_barnes 16812 (wowhead npc=16812/barnes),
-- npc_image_of_medivh 17651 (wowhead cata npc=17651/image-of-medivh);
-- the TDB creature_template ScriptName bindings are DB-side (no TDB in
-- this workspace). Deliberately unported (documented only — neither
-- script has a bridgeable core):
-- * npc_barnes (16812, EscortAI): the whole script is escort-machine
--   driven — waypoint 0 tuxedo + stage door, waypoint 4 spotlight
--   summon + SetEscortPaused dialogue machine (TalkCount 0..3 per
--   Oz/Hood/RAJ tables), waypoint 8 performance-ready, waypoint 9
--   PrepareEncounter summons + curtains, plus a 15s raid-wipe evade
--   poll. Escort bridges now EXIST (re-verified 2026-11-14, round116):
--   creature:StartEscort (lua_creature_events.go:2668), MoveWaypoint
--   (:4719, DB-driven via creaturemotion.go:4893), OnReachWP event 6
--   (runtime.go:371), SetEscortPaused (:2721), gameobject:UseDoorOrButton
--   (lua_objects.go:4674), GetGuidData (:7166) + GetGameObjectByGUID
--   (:4385), EnterEvadeMode (:4823), SetFlag (:6466), GetPlayersInGrid
--   (:6948) / Map:GetPlayers (globals.go:1201); live escort precedents
--   (old_hillsbrad Thrall, npc_earthmender_wilda). The Reset arm is
--   portable with the port (m_uiEventId = instance GetData
--   (DATA_OPERA_PERFORMANCE), rolled once per instance 1-3 in the
--   instance ctor — math.random(1, 3) cached per (map, instance) via
--   GetMapId/GetInstanceId) but lands WITH the Barnes port, not solo
--   (unconsumed state otherwise). Remaining gates: instance uint32
--   GetData (DATA_MOROES DONE, DATA_OPERA_OZ_DEATHCOUNT,
--   DATA_GO_STAGEDOORLEFT / DATA_GO_CURTAINS) has no instance bridge;
--   the dialogue timers, spotlight (19525) and encounter summons
--   (17535/17546/17547/17543/17603/17534) ride the escort machine.
--   The gossip arms (OnGossipHello menu 8970/8975/8978, OnGossipSelect
--   start event / GM event override) are registered gossip-event
--   content in the engine, but the hello gate and every select arm are
--   instance/escort-driven, so registering them would be a stub —
--   not registered (firesworn convention). Awaits the escort-port
--   assembly, not a missing bridge.
-- * npc_image_of_medivh (17651, ScriptedAI dialogue machine):
--   Reset PORTED 2026-11-14 (ev23 below — GetGuidData(26) latch /
--   MoveTo(1, MedivPos) / Despawn duplicate; all four bridges live:
--   GetGuidData :7166, SetGuidData :7151, MoveTo :5424, Despawn
--   :3318; the old "no movement bridge" claim is retracted). The
--   15-step dialogue machine still stays out: it starts from the
--   MovementInform (ev28) arrival -> StartEvent (:494-510), driven by
--   cross-creature relays to Arcanagos (17652) — summon (no summon
--   bridge), cross-creature Yell / DoCast(conflagration blast 30977)
--   / CastSpell(fire ball 30967) / KillSelf / MovePoint (no
--   cross-creature bridge), fire-timer cross-fire for steps 7..12,
--   SetDisableGravity / setActive / SetFarVisible / SetSpeedRate arms
--   (no bridges; InterruptNonMeleeSpells is bridged but rides the same
--   blocked machine), and the step-14 quest
--   9645 completion pass over instance players. No summon,
--   cross-creature or instance-script bridge — the dialogue machine
--   cannot be started — its arms are not registered (firesworn convention).
-- * Berthold (SPELL_TELEPORT 39567) and npc_arcane_protector /
--   npc_mana_feeder / bosses_opera content are untouched by this
--   file (arcane protector and mana feeder live in their own files).
-- Eluna creature events: 23 OnReset (npc_image_of_medivh Reset, below).

local ENTRY_IMAGE_OF_MEDIVH = 17651 -- karazhan.lua header; karazhan.h:55
local DATA_IMAGE_OF_MEDIVH = 26 -- karazhan.h:55

-- MedivPos (karazhan.cpp:426); orientation 1.94 is not consumed by
-- creature:MoveTo(id, x, y, z).
local MEDIVH_X, MEDIVH_Y, MEDIVH_Z = -11161.49, -1902.24, 91.48

-- C++ npc_image_of_medivhAI::Reset (karazhan.cpp:468-480): Initialize()
-- is pure locals (Step 0 etc., consumed only by the unported dialogue
-- machine); the observable arms are the instance guid latch + walk to
-- MedivPos, else despawn the duplicate. GetGuidData returns 0 for an
-- unlatched type (the Go form of ObjectGuid::Empty); SetGuidData(26)
-- latches the caller's GUID; MoveTo records id 1 so the arrival fires
-- MovementInform (event 28) when the dialogue machine lands.
local function onMedivhReset(event, creature)
    if creature:GetGuidData(DATA_IMAGE_OF_MEDIVH) == 0 then
        creature:SetGuidData(DATA_IMAGE_OF_MEDIVH)
        creature:MoveTo(1, MEDIVH_X, MEDIVH_Y, MEDIVH_Z)
    else
        creature:Despawn()
    end
end

RegisterCreatureEvent(ENTRY_IMAGE_OF_MEDIVH, 23, onMedivhReset)
