-- The Halls of Reflection instance script — Lua port of
-- src/server/scripts/Northrend/FrozenHalls/HallsOfReflection/instance_halls_of_reflection.cpp
-- (class instance_halls_of_reflection : public InstanceMapScript, map 668).
-- RegisterMapEvent binds the map-level Eluna instance hooks (the
-- instance_pit_of_saron.lua / instance_forge_of_souls.lua precedent);
-- its presence opens the Map:GetInstanceData gate for map 668
-- (engine/world/instance_hooks.go) that the boss and zone files share
-- with this file.
-- Shared instanceData keys (one table per (map, instance)):
-- falric / marwyn / frostswornGeneral / jainaOrSylvanasIntro /
-- jainaOrSylvanasEscape / lichkingIntro / lichkingEscape / uther /
-- korelnOrLoralen / frostmourneAltarBunny / captain / icewallTarget —
-- the OnCreatureCreate binds (instance_halls_of_reflection.cpp:125-171);
-- gunshipCannons — the NPC_GUNSHIP_CANNON_HORDE 37593 list (:161-164)
-- (NPC_WORLD_TRIGGER 22515 joins it only when it has a transport, and
-- there is no GetTransport bridge — see below).
-- Ported arms (C++-exact unless noted):
-- - OnCreatureCreate: the binds above. Fires on materialization
--   (motionFor funnel, instance_hooks.go:311-320); a static template
--   spawn never materializes server-side and is never bound
--   (naxxramas LoadGrid delta).
-- - OnCreatureCreate conditional legs, via creature:GetBossState
--   (lua_creature_events.go:5936; the oculus precedent):
--   Koreln/Loralen (:128-132): RemoveFlag(UNIT_NPC_FLAGS,
--   UNIT_NPC_FLAG_QUESTGIVER) when GetBossState(DATA_MARWYN 1) !=
--   DONE — the flag is re-armed by the Marwyn DONE override leg
--   (boss_marwyn.lua, garfrost tyrannus-Talk precedent).
--   Frostsworn General (:145-148): SetPhaseMask(1, true) when
--   GetBossState(DATA_MARWYN 1) == DONE — same re-arm source.
-- - GetGuidData (instance_halls_of_reflection.cpp:660-700): the Lua
--   mirror is the instanceData binds above, read as instanceData.falric /
--   instanceData.marwyn / etc. (oculus precedent). Live consumers:
--   boss_marwyn.lua JustDied reads instanceData.frostmourneAltarBunny /
--   korelnOrLoralen / frostswornGeneral for the Marwyn DONE override
--   legs (the kelthuzad GetInstanceData precedent); the intro chain
--   (halls_of_reflection_intro.lua) reads instanceData.falric /
--   instanceData.marwyn for the LK_6 spawn-aura + visible legs and the
--   LK_7/8/9 intro Talks.
-- Unmodeled (documented-only, no bridges):
-- - OnPlayerEnter team latch (:111-123) + GetCreatureEntry team swap
--   (:176-201) + GetData(DATA_TEAM_IN_INSTANCE) (:639-641): no
--   instance GetData/SetData bridge from Lua, and the latch's only
--   consumers are the team swap and the DONE-spawn legs below.
-- - SetBossState override (:332-419): DATA_FALRIC DONE (:342-348)
--   ++_waveCount + EVENT_NEXT_WAVE 1min rides the unported wave
--   machine (ProcessEvent EVENT_SPAWN_WAVES / EVENT_ADD_WAVE, no
--   instance ProcessEvent bridge; wave GUID lists live only in the
--   C++ script); DATA_MARWYN DONE (:349-371) — the bunny cast 72900,
--   Koreln/Loralen questgiver flag, DoUpdateWorldState(4884, 0) and
--   general SetPhaseMask(1) legs are PORTED in boss_marwyn.lua
--   JustDied (they run inside the C++ SetBossState call, in order
--   after the DONE write); the HandleGameObject door legs ride the
--   door model (no door model in the Go boss-state store,
--   instance_boss_states.go:72); SpawnGunship() (:425-447) rides
--   CreateTransport (no bridge) + the team latch; SpawnEscapeEvent()
--   (:449-467) rides instance SummonCreature + the team latch.
--   DATA_THE_LICH_KING_ESCAPE IN_PROGRESS (:384-388) DoAction
--   (ACTION_START_PREFIGHT) rides the escape-leader AI, whose
--   gunship-arrival trigger (instance ProcessEvent
--   EVENT_GUNSHIP_ARRIVAL, transport arrival) has no Lua fire site;
--   DONE (:389-402) chest-flag / cave-in-door / LK 72830-cast legs
--   ride the same untriggerable chain (the chest GO flag removal
--   and DoUseDoorOrButton have no GO-state bridge either);
--   FAIL (:403-415) rides the LK escape EnterEvadeMode, whose FAIL
--   write IS ported in npc_the_lich_king_escape.lua (leave-combat
--   handler) — the instance-side despawn / icewall-delete / 30s
--   re-spawn legs stay out (no GO-delete / instance-summon bridge).
--   No ON_UPDATE poll: the only DONE legs with a live consumer run
--   at DONE time in the boss died handlers (palehoof precedent).
-- - SetData (:469-517): DATA_INTRO_EVENT / DATA_WAVE_COUNT /
--   DATA_FROSTSWORN_GENERAL / DATA_QUEL_DELAR_EVENT — no instance
--   GetData/SetData bridge; the frostsworn-general SetData legs are
--   documented in npc_frostsworn_general.lua, the intro DONE leg in
--   halls_of_reflection_intro.lua, the wave-count wipe leg in
--   npc_hor_gauntlet_trash.lua.
-- - SetGuidData(DATA_QUEL_DELAR_INVOKER) (:519-527): no instance
--   SetGuidData bridge from Lua (the invoker GUID is written only by
--   the Quel'Delar gossip path, unported).
-- - OnUnitDeath wave-clear check (:530-556): no instance OnUnitDeath
--   hook (map events are 1-7 only, engine/scripting/runtime.go:172-178);
--   the wave GUID lists live only in the C++ script.
-- - OnCreatureRemove wave-list erase (:157-168): the wave GUID lists
--   are C++-only; nothing in Lua reads them.
-- - FillInitialWorldStates (:322-326): WORLD_STATE_HOR_WAVES_ENABLED
--   4884 = (_introState == DONE && marwyn != DONE) and
--   WORLD_STATE_HOR_WAVE_COUNT 4882 = _waveCount — both inputs are
--   instance SetData state with no bridge; nothing to port.
-- - OnGameObjectCreate (:203-276): Frostmourne phase-2 leg is gated
--   on GetData(DATA_INTRO_EVENT) (no bridge); the door legs ride the
--   door model; the gunship-stairs SetRespawnTime(DAY) leg has no
--   GO SetRespawnTime bridge (pit_of_saron precedent); the captain's
--   chest latch has no consumer (its flag removal rides the LK DONE
--   override above). No GO binds: every GO leg is unbridgeable or
--   gated on unbridged state, so latching the GUIDs would be dead
--   binds (pit_of_saron precedent).
-- - NPC_WORLD_TRIGGER (:156-161): joins GunshipCannonGUIDs only when
--   it has a transport (GetTransport, no bridge) — the bind is
--   skipped; the Horde cannon entry 37593 binds unconditionally.
-- - TheLichkingIntroGUID is bound but never read in C++ (declared
--   "unused", :657) — mirrored as a bind anyway, like the C++.
-- - JainaOrSylvanasIntroGUID is likewise never read in C++ — mirrored
--   as a bind anyway.
-- - ProcessEvent gunship-arrival legs (:573-625: captain Talks
--   SAY_CAPTAIN_FIRE / SAY_CAPTAIN_FINAL, cannon 70017 casts,
--   transport EnableMovement, stairs SetRespawnTime): no transport /
--   ProcessEvent bridges; the escape-leader DoAction legs they drive
--   are documented in the zone audit (halls_of_reflection.cpp
--   npc_jaina_or_sylvanas_escape_hor stays out: MovementInform +
--   transport + instance GetData machine).
-- - SetHeaders(DataHeader "HOR") / SetBossNumber(EncounterCount 3)
--   / SaveToDB / WriteSaveDataMore / ReadSaveDataMore: no bridges;
--   the boss-state store is written via creature:SetBossState
--   (lua_creature_events.go:5917) and read via creature:GetBossState
--   (lua_creature_events.go:5936).
-- No ON_UPDATE poll: see the SetBossState note above.

local MAP_HALLS_OF_REFLECTION = 668
local INSTANCE_EVENT_ON_CREATURE_CREATE = 5

local DATA_MARWYN = 1 -- halls_of_reflection.h:38
local BOSS_STATE_DONE = 3 -- InstanceScript.h:68-76

local UNIT_NPC_FLAGS = 82 -- UpdateFields.h:139 (baltharus precedent)
local UNIT_NPC_FLAG_QUESTGIVER = 0x00000002 -- UnitDefines.h:187

local NPC_JAINA_INTRO = 37221 -- halls_of_reflection.h:63
local NPC_SYLVANAS_INTRO = 37223 -- halls_of_reflection.h:64
local NPC_UTHER = 37225 -- halls_of_reflection.h:65
local NPC_THE_LICH_KING_INTRO = 37226 -- halls_of_reflection.h:66
local NPC_KORELN = 37582 -- halls_of_reflection.h:67
local NPC_LORALEN = 37779 -- halls_of_reflection.h:68
local NPC_FROSTMOURNE_ALTAR_BUNNY = 37704 -- halls_of_reflection.h:69
local NPC_FALRIC = 38112 -- halls_of_reflection.h:71
local NPC_MARWYN = 38113 -- halls_of_reflection.h:72
local NPC_FROSTSWORN_GENERAL = 36723 -- halls_of_reflection.h:79
local NPC_JAINA_ESCAPE = 36955 -- halls_of_reflection.h:82
local NPC_SYLVANAS_ESCAPE = 37554 -- halls_of_reflection.h:83
local NPC_THE_LICH_KING_ESCAPE = 36954 -- halls_of_reflection.h:84
local NPC_ICE_WALL_TARGET = 37014 -- halls_of_reflection.h:85
local NPC_GUNSHIP_CANNON_HORDE = 37593 -- halls_of_reflection.h:91
local NPC_JUSTIN_BARTLETT = 30344 -- halls_of_reflection.h:92
local NPC_KORM_BLACKSCAR = 30824 -- halls_of_reflection.h:93
local NPC_WORLD_TRIGGER = 22515 -- halls_of_reflection.h:95

local bossBinds = {
    [NPC_JAINA_INTRO] = "jainaOrSylvanasIntro",
    [NPC_SYLVANAS_INTRO] = "jainaOrSylvanasIntro",
    [NPC_KORELN] = "korelnOrLoralen",
    [NPC_LORALEN] = "korelnOrLoralen",
    [NPC_THE_LICH_KING_INTRO] = "lichkingIntro",
    [NPC_FALRIC] = "falric",
    [NPC_MARWYN] = "marwyn",
    [NPC_FROSTMOURNE_ALTAR_BUNNY] = "frostmourneAltarBunny",
    [NPC_FROSTSWORN_GENERAL] = "frostswornGeneral",
    [NPC_JAINA_ESCAPE] = "jainaOrSylvanasEscape",
    [NPC_SYLVANAS_ESCAPE] = "jainaOrSylvanasEscape",
    [NPC_THE_LICH_KING_ESCAPE] = "lichkingEscape",
    [NPC_JUSTIN_BARTLETT] = "captain",
    [NPC_KORM_BLACKSCAR] = "captain",
    [NPC_ICE_WALL_TARGET] = "icewallTarget",
    [NPC_UTHER] = "uther",
}

RegisterMapEvent(MAP_HALLS_OF_REFLECTION, INSTANCE_EVENT_ON_CREATURE_CREATE, function(event, instanceData, mapObj, creature)
    local entry = creature:GetEntry()
    if entry == NPC_GUNSHIP_CANNON_HORDE then
        if instanceData.gunshipCannons == nil then
            instanceData.gunshipCannons = {}
        end
        instanceData.gunshipCannons[#instanceData.gunshipCannons + 1] = creature
        return
    end
    -- C++ :156-161: NPC_WORLD_TRIGGER joins the cannon list only when
    -- it has a transport; no GetTransport bridge, so the bind is
    -- skipped (see header).
    if entry == NPC_WORLD_TRIGGER then
        return
    end
    local key = bossBinds[entry]
    if not key then
        return
    end
    instanceData[key] = creature
    -- C++ :128-132: Koreln/Loralen loses the questgiver flag while
    -- Marwyn is not DONE (re-armed by the Marwyn DONE leg).
    if entry == NPC_KORELN or entry == NPC_LORALEN then
        if creature:GetBossState(DATA_MARWYN) ~= BOSS_STATE_DONE then
            creature:RemoveFlag(UNIT_NPC_FLAGS, UNIT_NPC_FLAG_QUESTGIVER)
        end
    -- C++ :145-148: the Frostsworn General phases in once Marwyn is DONE.
    elseif entry == NPC_FROSTSWORN_GENERAL then
        if creature:GetBossState(DATA_MARWYN) == BOSS_STATE_DONE then
            creature:SetPhaseMask(1, true)
        end
    end
end)
