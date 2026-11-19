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

-- Borean Tundra: Thassarian — Lua port of npc_thassarianAI (EscortAI)
-- in src/server/scripts/Northrend/zone_borean_tundra.cpp (lines
-- 401-749; AddSC at line 1886, zone-script block per
-- northrend_script_loader.cpp). Entry: 26170 (TDB 335.26091
-- creature_template 'Thassarian'; the spawn row guid 101303 carries
-- ScriptName 'npc_thassarian' — 101303 is the spawn GUID, not the
-- entry; the old header conflated them). Eluna creature
-- events: 4 OnDied, 6 OnReachWP, 23 OnReset; gossip events 1 OnHello
-- / 2 OnSelect.
-- Escort-path note: TDB 335.26091 has NO waypoint_data rows for
-- 26170 and no creature_addon path_id for spawn 101303, so the
-- escort has an empty path in both models (C++ GetPath(entry)
-- likewise finds nothing) — the wp3/wp4 legs are inert until DB
-- path data exists, but the handlers are the faithful port and the
-- gossip Start still arms the escort.
-- Shared scene: ThassarianScene[thassarianGUIDLow] is the global the
-- four summoned actors write through (one shared lua.State, runtime.go
-- Load; netherspite cross-file global precedent): phase, the four
-- GUID latches (arthas/talbot from wp3, arlos/leryssa from phase 6),
-- the four InPosition flags (MovementInform writers in the actor
-- files), pump (phase-machine timer id). Leryssa's phase-7 write
-- (scene.phase = 16) and her first-arrival leryssaInPosition write
-- land here.
-- Ported arms (C++ order, zone_borean_tundra.cpp):
-- - Gossip hello (:724-731): quest menu via GossipAddQuests
--   (PrepareQuestMenu twin; no-ops when not a questgiver) + the
--   GOSSIP_ITEM_T item when quest 12019 is INCOMPLETE and area is
--   4128; menu sent with GetGossipTextId.
-- - Gossip select (:733-740): GOSSIP_ACTION_INFO_DEF+1 ->
--   StartEscort(player) (EscortAI::Start run=true, repeatable=false
--   twin). SetMaxPlayerDistance(200.0f) has no bridge (documented).
-- - WaypointReached (:495-547), under the GetEscortPlayer()
--   nil-guard: wp3 SetEscortPaused(true); summon Arthas 26203 +
--   Talbot 25301 (CORPSE_TIMED_DESPAWN 2min) with the C++ flag/walk/
--   MoveTo legs, latch both GUIDs, arm the phase-machine pump; wp4
--   SetEscortPaused(true) + phase = 7.
-- - UpdateAI phase machine (:549-704) as a one-shot re-arm
--   CreateLuaEvent pump (Barnes idiom): phase 0 polls every 500ms
--   for the two InPosition gates (arthas+talbot -> phase 1, fired
--   inline the same tick, C++-exact; arlos+leryssa -> Talk
--   SAY_THASSARIAN_1 + SetEscortPaused(false)); phases 1-17 run
--   their legs and re-arm with the C++ phaseTimer (3000/5000/2000/
--   1500/20000; phase 13's unset timer fires next tick, 100ms twin).
--   Phase 2 UpdateEntry(28189) re-seats Talbot (dual-entry handlers
--   in npc_counselor_talbot.lua); phase 6 summons Arlos/Leryssa with
--   the C++ coords/walk/react/flag legs; phase 8 SetInFront has no
--   bridge (orientation cosmetic — the C++ comment notes the client
--   doesn't update it either); phase 13/17 RemoveFromWorld legs use
--   the Despawn twin; phase 17's RemoveStandFlags(SIT) rides the
--   GetStandState()==SIT / SetStandState(STAND) twin. The pump stops
--   at phase 17 (scene complete) and is cancelled on Died/Reset.
-- - JustDied (:706-718): Despawn on all four latched GUIDs + pump
--   cancel + scene clear.
-- - Reset (:487-493): Initialize() clears the scene (no despawn —
--   C++ Reset only forgets the GUIDs); RestoreFaction stays out (no
--   Lua bridge); RemoveStandFlags(SIT) rides the stand-state twin.
-- Deliberate omissions: none in the AI — the whole struct is ported.

local ENTRY_THASSARIAN = 26170 -- creature_template entry; the
-- 101303 in the old header was the spawn GUID (TDB creature row
-- guid 101303, id 26170, ScriptName 'npc_thassarian').

local QUEST_LAST_RITES = 12019 -- zone_borean_tundra.cpp:406
local QUEST_STATUS_INCOMPLETE = 3 -- lua_player.go:4902
local AREA_THASSARIAN_ESCORT = 4128 -- :729

local NPC_IMAGE_LICH_KING = 26203 -- zone_borean_tundra.cpp:412
local NPC_COUNSELOR_TALBOT = 25301 -- zone_borean_tundra.cpp:413
local NPC_PRINCE_VALANAR = 28189 -- zone_borean_tundra.cpp:414
local NPC_GENERAL_ARLOS = 25250 -- zone_borean_tundra.cpp:415
local NPC_LERYSSA = 25251 -- zone_borean_tundra.cpp:416

local SPELL_SHADOW_BOLT = 15537 -- zone_borean_tundra.cpp:410

local SAY_THASSARIAN_1 = 0 -- :418
local SAY_THASSARIAN_2 = 1 -- :419
local SAY_THASSARIAN_3 = 2 -- :420
local SAY_TALBOT_1 = 0 -- :426
local SAY_TALBOT_2 = 1 -- :427
local SAY_TALBOT_3 = 2 -- :428
local SAY_TALBOT_4 = 3 -- :429
local SAY_LICH_1 = 0 -- :431
local SAY_LICH_2 = 1 -- :432
local SAY_LICH_3 = 2 -- :433

local UNIT_FIELD_FLAGS = 59
local UNIT_NPC_FLAGS = 82 -- OBJECT_END(6) + 0x4C
local UNIT_FLAG_NON_ATTACKABLE = 0x2
local UNIT_NPC_FLAG_QUESTGIVER = 0x2 -- UnitDefines.h:187
local FACTION_MONSTER = 14 -- SharedDefines.h:226
local REACT_PASSIVE = 0
local REACT_AGGRESSIVE = 2 -- UnitDefines.h:340-342
local UNIT_STAND_STATE_STAND = 0 -- UnitDefines.h:33
local UNIT_STAND_STATE_SIT = 1 -- UnitDefines.h:34
local UNIT_STAND_STATE_KNEEL = 8 -- UnitDefines.h:41

local GOSSIP_ICON_CHAT = 0
local GOSSIP_SENDER_MAIN = 1
local GOSSIP_ACTION_INFO_DEF = 1000
local GOSSIP_ITEM_T = "Let's do this, Thassarian. It's now or never." -- :444

local SUMMON_CORPSE_TIMED_DESPAWN = 5
local SUMMON_DESPAWN_MS = 120000 -- 2min

local PHASE_POLL_MS = 500 -- phase-0 InPosition gate poll interval
local PHASE_NEXT_TICK_MS = 100 -- phase 13's unset-timer twin

-- Shared Thassarian scene table (see header). Created on Reset/wp3/wp4;
-- the four summoned actors write their InPosition flags into it.
ThassarianScene = {}

local function sceneOf(creature, create)
    local guid = creature:GetGUIDLow()
    local scene = ThassarianScene[guid]
    if not scene and create then
        scene = {
            phase = 0,
            arthasGUID = nil,
            talbotGUID = nil,
            leryssaGUID = nil,
            arlosGUID = nil,
            arthasInPosition = false,
            talbotInPosition = false,
            arlosInPosition = false,
            leryssaInPosition = false,
            pump = nil,
        }
        ThassarianScene[guid] = scene
    end
    return scene
end

local function cancelPump(scene)
    if scene and scene.pump then
        RemoveEventById(scene.pump)
        scene.pump = nil
    end
end

-- Forward declaration (tharon_ja/gal_darah idiom): the pump tick and
-- the armer reference each other, so the tick is assigned, never
-- re-declared as `local function` (malown shadowing time-bomb).
local thassarianPhaseTick

local function armPump(creature, guid, delay)
    local scene = ThassarianScene[guid]
    if not scene then
        return
    end
    cancelPump(scene)
    scene.pump = CreateLuaEvent(function() thassarianPhaseTick(creature, guid) end, delay)
end

-- C++ npc_thassarianAI::UpdateAI phase machine (:549-704).
thassarianPhaseTick = function(creature, guid)
    local scene = ThassarianScene[guid]
    if not scene then
        return
    end
    scene.pump = nil
    local function resolveCreature(storedGuid)
        if not storedGuid then
            return nil
        end
        return creature:GetCreatureByGUID(storedGuid)
    end
    -- C++ gates, checked every UpdateAI tick while phase == 0.
    if scene.phase == 0 then
        if scene.arthasInPosition and scene.talbotInPosition then
            scene.arthasInPosition = false
            scene.talbotInPosition = false
            scene.phase = 1
        end
        if scene.arlosInPosition and scene.leryssaInPosition then
            scene.arlosInPosition = false
            scene.leryssaInPosition = false
            creature:Talk(SAY_THASSARIAN_1)
            creature:SetEscortPaused(false)
        end
        if scene.phase == 0 then
            armPump(creature, guid, PHASE_POLL_MS)
            return
        end
    end
    local talbot = resolveCreature(scene.talbotGUID)
    local arthas = resolveCreature(scene.arthasGUID)
    local phase = scene.phase
    if phase == 1 then
        if talbot then
            talbot:SetStandState(UNIT_STAND_STATE_KNEEL)
        end
        scene.phase = 2
        armPump(creature, guid, 3000)
    elseif phase == 2 then
        if talbot then
            talbot:UpdateEntry(NPC_PRINCE_VALANAR)
            talbot:SetFaction(FACTION_MONSTER)
            talbot:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
            talbot:SetReactState(REACT_PASSIVE)
        end
        scene.phase = 3
        armPump(creature, guid, 5000)
    elseif phase == 3 then
        if talbot then
            talbot:Talk(SAY_TALBOT_1)
        end
        scene.phase = 4
        armPump(creature, guid, 5000)
    elseif phase == 4 then
        if arthas then
            arthas:Talk(SAY_LICH_1)
        end
        scene.phase = 5
        armPump(creature, guid, 5000)
    elseif phase == 5 then
        if talbot then
            talbot:Talk(SAY_TALBOT_2)
        end
        scene.phase = 6
        armPump(creature, guid, 5000)
    elseif phase == 6 then
        local arlos = creature:SummonCreature(NPC_GENERAL_ARLOS,
            3745.527100, 3615.655029, 473.321533, 4.447805,
            SUMMON_CORPSE_TIMED_DESPAWN, SUMMON_DESPAWN_MS)
        if arlos then
            scene.arlosGUID = arlos:GetGUIDLow()
            arlos:SetWalk(true)
            arlos:MoveTo(0, 3735.570068, 3572.419922, 477.441010)
        end
        local leryssa = creature:SummonCreature(NPC_LERYSSA,
            3749.654541, 3614.959717, 473.323486, 4.524959,
            SUMMON_CORPSE_TIMED_DESPAWN, SUMMON_DESPAWN_MS)
        if leryssa then
            scene.leryssaGUID = leryssa:GetGUIDLow()
            leryssa:SetWalk(false)
            leryssa:SetReactState(REACT_PASSIVE)
            leryssa:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
            leryssa:MoveTo(0, 3741.969971, 3571.439941, 477.441010)
        end
        scene.phase = 0
        armPump(creature, guid, PHASE_POLL_MS)
    elseif phase == 7 then
        creature:Talk(SAY_THASSARIAN_2)
        scene.phase = 8
        armPump(creature, guid, 5000)
    elseif phase == 8 then
        if arthas and talbot then
            -- arthas->SetInFront(me) has no bridge (orientation-only;
            -- the C++ comment notes the client doesn't update it).
            talbot:SetStandState(UNIT_STAND_STATE_STAND)
            arthas:Talk(SAY_LICH_2)
        end
        scene.phase = 9
        armPump(creature, guid, 5000)
    elseif phase == 9 then
        creature:Talk(SAY_THASSARIAN_3)
        scene.phase = 10
        armPump(creature, guid, 5000)
    elseif phase == 10 then
        if talbot then
            talbot:Talk(SAY_TALBOT_3)
        end
        scene.phase = 11
        armPump(creature, guid, 5000)
    elseif phase == 11 then
        if arthas then
            arthas:Talk(SAY_LICH_3)
        end
        scene.phase = 12
        armPump(creature, guid, 5000)
    elseif phase == 12 then
        if talbot then
            talbot:Talk(SAY_TALBOT_4)
        end
        scene.phase = 13
        armPump(creature, guid, 2000)
    elseif phase == 13 then
        if arthas then
            arthas:Despawn()
        end
        scene.phase = 14
        -- C++ leaves phaseTimer unset here, so case 14 fires on the
        -- next UpdateAI tick.
        armPump(creature, guid, PHASE_NEXT_TICK_MS)
    elseif phase == 14 then
        creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
        if talbot then
            talbot:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
            talbot:SetReactState(REACT_AGGRESSIVE)
            talbot:CastSpell(creature, SPELL_SHADOW_BOLT)
        end
        scene.phase = 15
        armPump(creature, guid, 1500)
    elseif phase == 15 then
        creature:SetReactState(REACT_AGGRESSIVE)
        if talbot then
            creature:AttackStart(talbot)
        end
        -- C++ phase = 0: the machine idles here until Leryssa's
        -- phase-7 write sets phase = 16 (kept polling).
        scene.phase = 0
        armPump(creature, guid, PHASE_POLL_MS)
    elseif phase == 16 then
        creature:SetFlag(UNIT_NPC_FLAGS, UNIT_NPC_FLAG_QUESTGIVER)
        scene.phase = 17
        armPump(creature, guid, 20000)
    elseif phase == 17 then
        local leryssa = resolveCreature(scene.leryssaGUID)
        if leryssa then
            leryssa:Despawn()
        end
        local arlos = resolveCreature(scene.arlosGUID)
        if arlos then
            arlos:Despawn()
        end
        if talbot then
            talbot:Despawn()
        end
        if creature:GetStandState() == UNIT_STAND_STATE_SIT then
            creature:SetStandState(UNIT_STAND_STATE_STAND)
        end
        creature:SetEscortPaused(false)
        scene.phase = 0
        -- Pump stops: the scene is complete.
    end
end

-- C++ WaypointReached (zone_borean_tundra.cpp:495-547).
local function thassarianReachWP(event, creature)
    local player = creature:GetEscortPlayer()
    if not player then
        return
    end
    local wp = creature:GetCurrentWaypointId()
    local scene = sceneOf(creature, true)
    if wp == 3 then
        creature:SetEscortPaused(true)
        local arthas = creature:SummonCreature(NPC_IMAGE_LICH_KING,
            3730.313, 3518.689, 473.324, 1.562,
            SUMMON_CORPSE_TIMED_DESPAWN, SUMMON_DESPAWN_MS)
        if arthas then
            -- C++ wp3 latches arthasGUID (:507); thassarianDied below
            -- resolves it.
            scene.arthasGUID = arthas:GetGUIDLow()
            arthas:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
            arthas:SetReactState(REACT_PASSIVE)
            arthas:SetWalk(true)
            arthas:MoveTo(0, 3737.374756, 3564.841309, 477.433014)
        end
        local talbot = creature:SummonCreature(NPC_COUNSELOR_TALBOT,
            3747.23, 3614.936, 473.321, 4.462012,
            SUMMON_CORPSE_TIMED_DESPAWN, SUMMON_DESPAWN_MS)
        if talbot then
            -- C++ wp3 latches talbotGUID (:515); thassarianDied
            -- below resolves it.
            scene.talbotGUID = talbot:GetGUIDLow()
            talbot:SetWalk(true)
            talbot:MoveTo(0, 3738.000977, 3568.882080, 477.433014)
        end
        creature:SetWalk(false)
        -- The actors are walking: start polling for the InPosition
        -- gates.
        armPump(creature, creature:GetGUIDLow(), PHASE_POLL_MS)
    elseif wp == 4 then
        creature:SetEscortPaused(true)
        scene.phase = 7
        armPump(creature, creature:GetGUIDLow(), PHASE_POLL_MS)
    end
end

-- C++ npc_thassarianAI::JustDied (zone_borean_tundra.cpp:706-718):
-- RemoveFromWorld on Talbot (talbotGUID), Leryssa (leryssaGUID),
-- Arlos (arlosGUID) and the Arthas image (arthasGUID), in C++ order.
-- Despawn() is the RemoveFromWorld twin; the GUIDs resolve via
-- GetCreatureByGUID (lua_creature_events.go:4706).
local function thassarianDied(event, creature)
    local guid = creature:GetGUIDLow()
    local scene = ThassarianScene[guid]
    if scene then
        cancelPump(scene)
        local function despawnLatched(storedGuid)
            if not storedGuid then
                return
            end
            local target = creature:GetCreatureByGUID(storedGuid)
            if target then
                target:Despawn()
            end
        end
        despawnLatched(scene.talbotGUID)
        despawnLatched(scene.leryssaGUID)
        despawnLatched(scene.arlosGUID)
        despawnLatched(scene.arthasGUID)
        ThassarianScene[guid] = nil
    end
end

-- C++ npc_thassarianAI::Reset (zone_borean_tundra.cpp:487-493).
local function thassarianReset(event, creature)
    local guid = creature:GetGUIDLow()
    local scene = ThassarianScene[guid]
    if scene then
        cancelPump(scene)
        ThassarianScene[guid] = nil
    end
    if creature:GetStandState() == UNIT_STAND_STATE_SIT then
        creature:SetStandState(UNIT_STAND_STATE_STAND)
    end
end

-- C++ npc_thassarian::OnGossipHello (zone_borean_tundra.cpp:724-731).
local function thassarianGossipHello(event, player, creature)
    player:GossipAddQuests(creature)
    if player:GetQuestStatus(QUEST_LAST_RITES) == QUEST_STATUS_INCOMPLETE
        and creature:GetAreaId() == AREA_THASSARIAN_ESCORT then
        player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_ITEM_T,
            GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 1)
    end
    player:GossipSendMenu(player:GetGossipTextId(creature), creature)
end

-- C++ npc_thassarian::OnGossipSelect (zone_borean_tundra.cpp:733-740).
local function thassarianGossipSelect(event, player, creature, sender, action)
    player:GossipClearMenu()
    player:GossipComplete()
    if sender == GOSSIP_SENDER_MAIN and action == GOSSIP_ACTION_INFO_DEF + 1 then
        -- EscortAI::Start(run=true, repeatable=false, playerGUID) twin.
        -- SetMaxPlayerDistance(200.0f) has no bridge (documented).
        creature:StartEscort(player)
    end
    return false
end

RegisterCreatureEvent(ENTRY_THASSARIAN, 6, thassarianReachWP)
RegisterCreatureEvent(ENTRY_THASSARIAN, 4, thassarianDied)
RegisterCreatureEvent(ENTRY_THASSARIAN, 23, thassarianReset)
RegisterCreatureGossipEvent(ENTRY_THASSARIAN, 1, thassarianGossipHello)
RegisterCreatureGossipEvent(ENTRY_THASSARIAN, 2, thassarianGossipSelect)
