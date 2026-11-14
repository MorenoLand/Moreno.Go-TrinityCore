-- Wyrmrest Temple: Alexstrasza the Lifebinder — Lua port of
-- src/server/scripts/Events/childrens_week.cpp
-- (class npc_alexstraza_the_lifebinder :
-- CreatureScript("npc_alexstraza_the_lifebinder")).
-- Entry: 26917 (creature_template ScriptName verified in TDB 26091;
-- spawn guid 130984, map 571, Wyrmrest Temple). The head entry's
-- "no ScriptName row" claim is WRONG — the row exists (the earlier
-- search used the "alexstrasza" spelling; the C++ class and the DB
-- row spell it "alexstraza").
-- Eluna creature events: 23 OnReset, 4 OnDied, 41 OnSetData
-- (engine/scripting/runtime.go:440, the Go-side extension fired by
-- creature:SetData).
-- Ported arms:
-- - MoveInLineOfSight + UpdateAI orphan phase machine
--   (childrens_week.cpp:795-963): the sighting leg is a 1s
--   GetPlayersInRange poll — the ev27 twin (ev27 fires only from
--   the aggro-engage branch; the orphan trigger needs a proximity
--   sighting, not an engage). Poll: any player within 10y 2D with
--   QUEST_THE_DRAGON_QUEEN_ORACLE (13954) INCOMPLETE whose
--   SPELL_ORPHAN_OUT (58818) aura was cast by the oracle orphan
--   (33533) captures playerGUID/orphanGUID and starts phase 1; a
--   player WITHOUT the oracle quest incomplete falls through to the
--   wolvar leg (QUEST_THE_DRAGON_QUEEN_WOLVAR 13955 + orphan 33532
--   -> phase 7) — the C++ if/else-if chain, verbatim: a player with
--   the oracle quest incomplete but no oracle orphan out does NOT
--   get the wolvar leg tried. Phase machine (5s steps, 1s pump
--   granularity): oracle p1 orphan MovePoint 5y in front + Talk 11;
--   p2 orphan faces me + Talk 12; p3 Talk 13; p4 Talk 2 (me) +
--   SetStandState KNEEL + face orphan; p5 orphan Talk 14; p6
--   SetStandState STAND + SetOrientation(home) +
--   GroupEventHappens(13954) + orphan MoveFollow(player) + Reset.
--   Wolvar p7 orphan MovePoint 5y in front + Talk 11; p8
--   FindNearestCreature(KRASUS 27990, 10y) -> orphan faces krasus +
--   krasus Talk 8 (nil-safe: no krasus nearby, no Talk, timer still
--   re-arms); p9 orphan Talk 12; p10 orphan faces me + Talk 2 (me);
--   p11 orphan Talk 13; p12 GroupEventHappens(13955) + orphan
--   MoveFollow(player) + Reset. Player/orphan unresolvable on a
--   tick -> Reset (C++ UpdateAI null-guard). Timer-0 first step
--   fires on the sighting tick (C++-exact); later steps land on 1s
--   boundaries. After a terminal phase (6/12) the state returns to
--   phase 0 and the pump re-arms: C++ Reset() leaves
--   MoveInLineOfSight live, so the next player can trigger the
--   event (deliberate improvement over the etymidian idiom, whose
--   pump stops after completion).
-- - SetData (childrens_week.cpp:795-811): type 0, data 1 ->
--   SetOrientation(1.6049); data 2 -> SetOrientation(home
--   orientation). No in-tree caller exists (no smart_scripts row
--   for 26917, no other C++ reference to alexstraza) — the legs are
--   dormant but portable via ev41, so they are ported, not dropped.
--   SetOrientation is the new raw-slot bridge
--   (lua_creature_events.go: Position::SetOrientation twin, no
--   packet — C++ WorldObject::SetOrientation is a pure slot write).
-- creature_text holds NO 26917/27990/33533/33532 rows in TDB 26091
-- (the table has exactly 2 rows, entries 6/23383 — the captive
-- precedent); all Talk ids are ported verbatim and are silent
-- data-side.

local ENTRY_ALEXSTRASZA = 26917

local NPC_KRASUS = 27990
local ORPHAN_ORACLE = 33533
local ORPHAN_WOLVAR = 33532
local SPELL_ORPHAN_OUT = 58818

local QUEST_THE_DRAGON_QUEEN_ORACLE = 13954
local QUEST_THE_DRAGON_QUEEN_WOLVAR = 13955

local TEXT_ALEXSTRASZA_2 = 2
local TEXT_KRASUS_8 = 8

local QUEST_STATUS_INCOMPLETE = 3
local SIGHT_RANGE = 10.0
local POLL_RANGE = 25.0
local PET_FOLLOW_DIST = 1.0
local PET_FOLLOW_ANGLE = math.pi / 2

local UNIT_STAND_STATE_STAND = 0
local UNIT_STAND_STATE_KNEEL = 8

local ORIENTATION_FACE_ORPHAN = 1.6049

local timers = {}
local states = {}

-- Forward declaration: tick's closure calls alexTick, assigned below
-- (tharon_ja/gal_darah idiom; a later `local function` would shadow
-- the slot and nil-call at runtime).
local alexTick

local function cancelPump(guid)
    local id = timers[guid]
    if id then
        RemoveEventById(id)
        timers[guid] = nil
    end
end

local function armPump(guid, creature)
    timers[guid] = CreateLuaEvent(function()
        timers[guid] = nil
        alexTick(creature, guid)
    end, 1000)
end

-- C++ getOrphanGUID (childrens_week.cpp:136-144): the SPELL_ORPHAN_OUT
-- aura on the player, when its caster is the expected orphan entry,
-- yields the orphan's GUID; otherwise empty.
local function orphanGUIDFor(player, orphanEntry)
    local aura = player:GetAura(SPELL_ORPHAN_OUT)
    if aura == nil then
        return nil
    end
    local caster = aura:GetCaster()
    if caster == nil or caster:GetEntry() ~= orphanEntry then
        return nil
    end
    return caster:GetGUIDLow()
end

-- C++ orphan->GetMotionMaster()->MovePoint(0, me + front * 5):
-- creature:MoveTo is the MovePoint twin (grizzly precedent).
local function moveOrphanInFront(creature, orphan)
    local o = creature:GetO()
    orphan:MoveTo(creature:GetX() + math.cos(o) * 5,
        creature:GetY() + math.sin(o) * 5, creature:GetZ())
end

-- C++ MoveInLineOfSight (childrens_week.cpp:813-838) + UpdateAI
-- (childrens_week.cpp:840-963), 1s granularity.
alexTick = function(creature, guid)
    local st = states[guid]
    if not st then
        return
    end
    if creature:IsDead() then
        states[guid] = nil
        return
    end
    if st.phase == 0 then
        -- Sighting poll: first player in range with an incomplete
        -- dragon-queen quest and the matching orphan-out aura.
        -- C++ fires on the first unit to move in; the poll picks the
        -- first list match instead (1s granularity twin).
        local sighted = false
        local players = creature:GetPlayersInRange(POLL_RANGE)
        for _, player in ipairs(players) do
            if player:GetQuestStatus(QUEST_THE_DRAGON_QUEEN_ORACLE)
                    == QUEST_STATUS_INCOMPLETE then
                local dx = player:GetX() - creature:GetX()
                local dy = player:GetY() - creature:GetY()
                if dx * dx + dy * dy < SIGHT_RANGE * SIGHT_RANGE then
                    local og = orphanGUIDFor(player, ORPHAN_ORACLE)
                    if og ~= nil then
                        st.phase = 1
                        st.playerGUID = player:GetGUID()
                        st.orphanGUID = og
                        st.delay = 0
                        sighted = true
                        break
                    end
                end
                -- Oracle quest incomplete but no oracle orphan out:
                -- the C++ else-if does NOT try the wolvar leg for
                -- this player; move on to the next player.
            elseif player:GetQuestStatus(QUEST_THE_DRAGON_QUEEN_WOLVAR)
                    == QUEST_STATUS_INCOMPLETE then
                local dx = player:GetX() - creature:GetX()
                local dy = player:GetY() - creature:GetY()
                if dx * dx + dy * dy < SIGHT_RANGE * SIGHT_RANGE then
                    local og = orphanGUIDFor(player, ORPHAN_WOLVAR)
                    if og ~= nil then
                        st.phase = 7
                        st.playerGUID = player:GetGUID()
                        st.orphanGUID = og
                        st.delay = 0
                        sighted = true
                        break
                    end
                end
            end
        end
        if not sighted or st.delay > 0 then
            armPump(guid, creature)
            return
        end
        -- delay 0: fall through and run the first phase step on the
        -- sighting tick.
    end
    local player = creature:GetPlayerByGUID(st.playerGUID)
    local orphan = creature:GetCreatureByGUID(st.orphanGUID)
    if player == nil or orphan == nil then
        -- C++ UpdateAI null-guard -> Reset(): back to phase 0 with
        -- the pump re-armed so sighting resumes.
        states[guid] = { phase = 0, delay = 0, playerGUID = 0, orphanGUID = 0 }
        armPump(guid, creature)
        return
    end
    if st.delay > 1000 then
        st.delay = st.delay - 1000
        armPump(guid, creature)
        return
    end
    if st.phase == 1 then
        moveOrphanInFront(creature, orphan)
        orphan:Talk(11)
        st.delay = 5000
    elseif st.phase == 2 then
        orphan:SetFacingToObject(creature)
        orphan:Talk(12)
        st.delay = 5000
    elseif st.phase == 3 then
        orphan:Talk(13)
        st.delay = 5000
    elseif st.phase == 4 then
        creature:Talk(TEXT_ALEXSTRASZA_2)
        creature:SetStandState(UNIT_STAND_STATE_KNEEL)
        creature:SetFacingToObject(orphan)
        st.delay = 5000
    elseif st.phase == 5 then
        orphan:Talk(14)
        st.delay = 5000
    elseif st.phase == 6 then
        creature:SetStandState(UNIT_STAND_STATE_STAND)
        local _, _, _, homeO = creature:GetHomePosition()
        creature:SetOrientation(homeO)
        player:GroupEventHappens(QUEST_THE_DRAGON_QUEEN_ORACLE, creature)
        orphan:MoveFollow(player, PET_FOLLOW_DIST, PET_FOLLOW_ANGLE)
        states[guid] = { phase = 0, delay = 0, playerGUID = 0, orphanGUID = 0 }
        armPump(guid, creature)
        return
    elseif st.phase == 7 then
        moveOrphanInFront(creature, orphan)
        orphan:Talk(11)
        st.delay = 5000
    elseif st.phase == 8 then
        local krasus = creature:FindNearestCreature(NPC_KRASUS, 10.0)
        if krasus ~= nil then
            orphan:SetFacingToObject(krasus)
            krasus:Talk(TEXT_KRASUS_8)
        end
        st.delay = 5000
    elseif st.phase == 9 then
        orphan:Talk(12)
        st.delay = 5000
    elseif st.phase == 10 then
        orphan:SetFacingToObject(creature)
        creature:Talk(TEXT_ALEXSTRASZA_2)
        st.delay = 5000
    elseif st.phase == 11 then
        orphan:Talk(13)
        st.delay = 5000
    elseif st.phase == 12 then
        player:GroupEventHappens(QUEST_THE_DRAGON_QUEEN_WOLVAR, creature)
        orphan:MoveFollow(player, PET_FOLLOW_DIST, PET_FOLLOW_ANGLE)
        states[guid] = { phase = 0, delay = 0, playerGUID = 0, orphanGUID = 0 }
        armPump(guid, creature)
        return
    else
        states[guid] = nil
        return
    end
    st.phase = st.phase + 1
    armPump(guid, creature)
end

-- C++ npc_alexstraza_the_lifebinderAI::Reset (childrens_week.cpp:787-790)
-- + Initialize (childrens_week.cpp:778-785): timer/phase/GUIDs cleared.
-- Go fires ev23 on spawn, evade and death; the dead arm drops the
-- state (kerlonian idiom) and the pump restarts in phase 0.
local function onReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelPump(guid)
    if creature:IsDead() then
        states[guid] = nil
        return
    end
    states[guid] = { phase = 0, delay = 0, playerGUID = 0, orphanGUID = 0 }
    armPump(guid, creature)
end

local function onDied(event, creature)
    local guid = creature:GetGUIDLow()
    cancelPump(guid)
    states[guid] = nil
end

-- C++ SetData (childrens_week.cpp:793-811): type 0 only; data 1 faces
-- the orphan side (1.6049), data 2 restores the home orientation.
-- CreatureEventOnSetData (41) carries (event, creature, type, data).
local function onSetData(event, creature, typ, data)
    if typ ~= 0 then
        return
    end
    if data == 1 then
        creature:SetOrientation(ORIENTATION_FACE_ORPHAN)
    elseif data == 2 then
        local _, _, _, homeO = creature:GetHomePosition()
        creature:SetOrientation(homeO)
    end
end

RegisterCreatureEvent(ENTRY_ALEXSTRASZA, 23, onReset)
RegisterCreatureEvent(ENTRY_ALEXSTRASZA, 4, onDied)
RegisterCreatureEvent(ENTRY_ALEXSTRASZA, 41, onSetData)
