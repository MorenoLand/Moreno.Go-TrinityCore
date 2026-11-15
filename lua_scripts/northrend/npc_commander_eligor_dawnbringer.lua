-- Commander Eligor Dawnbringer (Dragonblight, Wyrmrest Temple) --
-- Lua port of src/server/scripts/Northrend/zone_dragonblight.cpp
-- (npc_commander_eligor_dawnbringerAI — the Naxxramas-wings talk
-- cinematic). Zone-script unit per northrend_script_loader.cpp
-- order (borean_tundra done; dragonblight: lecraft + wyrmrest
-- defender ported, eligor this file).
-- Entry 27784 'Commander Eligor Dawnbringer' (TDB-verified:
-- creature_template ScriptName 'npc_commander_eligor_dawnbringer';
-- spawn guid 133420, map 571; the 16115/29177 same-name rows carry
-- an empty ScriptName). Talk groups 0-26 exist in creature_text
-- for 27784. Audience entries 27160/27162/27164 ('7th Legion
-- Infantryman/Sentinel/Battle Mage') and image entries 27766-27782
-- TDB-verified.
-- Eluna creature events: 5 OnSpawn, 23 OnReset, 28 OnMovementInform,
-- 2 OnLeaveCombat, 4 OnDied. C++ Reset runs at spawn in TC (AI init)
-- and on evade, so the same init handler rides ev5 + ev23
-- (frozen-core precedent); ev2/ev4 cancel the timers + drop state
-- (C++ Reset re-latches on the next evade/spawn).
-- Bridges verified live this tick (the old "documented-only, awaits
-- the movement / world-search / entry-update / cross-creature-facing
-- bridges" notes in npc_torturer_lecraft.lua + boss_ai.go are stale):
-- creature:MoveTo + ev28 (POINT_MOTION_TYPE 8, nightbane precedent),
-- GetCreaturesInGrid(entry, dist) = GetCreatureListWithEntryInGrid,
-- FindNearestCreature(entry, dist, alive), SetEntry, SetDisplayId,
-- SetFacingTo, SetFacingToObject, SetWalk, Talk. C++ DoCastVictim
-- default is triggered=false (vaelastrasz convention):
-- creature:CastSpell(target, spell) = DoCast.
-- Fight shape (C++-exact for all modeled arms):
-- Reset: talkWing = 0, audience/image GUID stores cleared,
-- EVENT_GET_TARGETS 5s + EVENT_START_RANDOM 20s (one-shot).
-- EVENT_GET_TARGETS: StoreTargets — GetCreaturesInGrid for the 3
-- audience entries (15.0f) + FindNearestCreature for the 5 image
-- anchors (20.0f, alive). EVENT_START_RANDOM: talkWing = urand(0,4),
-- Talk(talkWing), EVENT_MOVE_TO_POINT 8s. EVENT_MOVE_TO_POINT:
-- SetWalk(true) + MovePoint(1, PosTalkLocations[talkWing]).
-- MovementInform(POINT, 1): SetFacingTo(talk location orientation)
-- + TurnAudience (every stored audience creature faces Eligor) +
-- wing switch: 0 Pinnacle (urand 0-1: Kel'Thuzad + KELTHUZAD_2 8s
-- / Sapphiron), 1 Death knight wing (urand 0-2: Razuvious / Gothik /
-- Thane + DEATH_KNIGHTS_2 10s), 2 Abomination wing (urand 0-3:
-- Patchwerk / Grobbulus / Thaddius / Gluth), 3 Spider wing (urand
-- 0-2: Anub'rekhan / Faerlina / Maexxna), 4 Plague wing (urand 0-2:
-- Noth / Heigan + HEIGAN_2 8s / Loatheb), 5 Home (START_RANDOM 30s).
-- ChangeImage: Talk(text) + image SetEntry + SetDisplayId +
-- CastSpell(self, SPELL_HEROIC_IMAGE_CHANNEL) + TALK_COMPLETE 40s.
-- EVENT_TALK_COMPLETE: talkWing = 5, Talk(5), MOVE_TO_POINT 5s.
-- KELTHUZAD_2: Talk(8) + KELTHUZAD_3 8s; KELTHUZAD_3: Talk(9).
-- DEATH_KNIGHTS_2: Talk(13) + image SetEntry/SetDisplayId Blaumeux
-- + DEATH_KNIGHTS_3 10s; DEATH_KNIGHTS_3: Talk(14) + Zeliek +
-- DEATH_KNIGHTS_4 10s; DEATH_KNIGHTS_4: Talk(15). HEIGAN_2: Talk(25).
-- Deliberate deviations: SPELL_HEROIC_IMAGE_CHANNEL 49519 is NOT
-- seeded in Spell.dbc — the cast call is ported as-is and stays
-- dormant until the seed lands (lecraft 47575 precedent).
-- GetMotionMaster()->Clear() before MovePoint has no Lua twin;
-- the MoveTo launch replaces the motion (nightbane precedent).
-- DoMeleeAttackIfReady is engine-driven melee, no port (lecraft
-- precedent). Audience store caps at 10 (C++ audienceList[10],
-- no bounds check; the temple spawns are fixed). urand(a, b) =
-- math.random(a, b) (netherspite precedent).

local ENTRY_ELIGOR = 27784

local NPC_INFANTRYMAN = 27160
local NPC_SENTINAL = 27162
local NPC_BATTLE_MAGE = 27164

local NPC_IMAGE_OF_KELTHUZAD = 27766
local NPC_IMAGE_OF_RAZUVIOUS = 27768
local NPC_IMAGE_OF_PATCHWERK = 27773
local NPC_IMAGE_OF_ANUBREKHAN = 27776
local NPC_IMAGE_OF_NOTH = 27779
local NPC_IMAGE_OF_SAPPHIRON = 27767
local NPC_IMAGE_OF_GOTHIK = 27769
local NPC_IMAGE_OF_THANE = 27770
local NPC_IMAGE_OF_BLAUMEUX = 27771
local NPC_IMAGE_OF_ZELIEK = 27772
local NPC_IMAGE_OF_GROBBULUS = 27774
local NPC_IMAGE_OF_THADDIUS = 27775
local NPC_IMAGE_OF_GLUTH = 27782
local NPC_IMAGE_OF_FAERLINA = 27777
local NPC_IMAGE_OF_MAEXXNA = 27778
local NPC_IMAGE_OF_HEIGAN = 27780
local NPC_IMAGE_OF_LOATHEB = 27781

local MODEL_IMAGE_OF_KELTHUZAD = 24787
local MODEL_IMAGE_OF_SAPPHIRON = 24788
local MODEL_IMAGE_OF_RAZUVIOUS = 24799
local MODEL_IMAGE_OF_GOTHIK = 24804
local MODEL_IMAGE_OF_THANE = 24802
local MODEL_IMAGE_OF_BLAUMEUX = 24794
local MODEL_IMAGE_OF_ZELIEK = 24800
local MODEL_IMAGE_OF_PATCHWERK = 24798
local MODEL_IMAGE_OF_GROBBULUS = 24792
local MODEL_IMAGE_OF_THADDIUS = 24801
local MODEL_IMAGE_OF_GLUTH = 24803
local MODEL_IMAGE_OF_ANUBREKHAN = 24789
local MODEL_IMAGE_OF_FAERLINA = 24790
local MODEL_IMAGE_OF_MAEXXNA = 24796
local MODEL_IMAGE_OF_NOTH = 24797
local MODEL_IMAGE_OF_HEIGAN = 24793
local MODEL_IMAGE_OF_LOATHEB = 24795

local SAY_PINNACLE = 0
local SAY_DEATH_KNIGHT_WING = 1
local SAY_ABOMINATION_WING = 2
local SAY_SPIDER_WING = 3
local SAY_PLAGUE_WING = 4
local SAY_TALK_COMPLETE = 5
local SAY_SAPPHIRON = 6
local SAY_KELTHUZAD_1 = 7
local SAY_KELTHUZAD_2 = 8
local SAY_KELTHUZAD_3 = 9
local SAY_RAZUVIOUS = 10
local SAY_GOTHIK = 11
local SAY_DEATH_KNIGHTS_1 = 12
local SAY_DEATH_KNIGHTS_2 = 13
local SAY_DEATH_KNIGHTS_3 = 14
local SAY_DEATH_KNIGHTS_4 = 15
local SAY_PATCHWERK = 16
local SAY_GROBBULUS = 17
local SAY_GLUTH = 18
local SAY_THADDIUS = 19
local SAY_ANUBREKHAN = 20
local SAY_FAERLINA = 21
local SAY_MAEXXNA = 22
local SAY_NOTH = 23
local SAY_HEIGAN_1 = 24
local SAY_HEIGAN_2 = 25
local SAY_LOATHEB = 26

local SPELL_HEROIC_IMAGE_CHANNEL = 49519

local POINT_MOTION_TYPE = 8

local AUDIENCE_MOBS = { NPC_INFANTRYMAN, NPC_SENTINAL, NPC_BATTLE_MAGE }
local IMAGE_ANCHORS = {
    NPC_IMAGE_OF_KELTHUZAD,
    NPC_IMAGE_OF_RAZUVIOUS,
    NPC_IMAGE_OF_PATCHWERK,
    NPC_IMAGE_OF_ANUBREKHAN,
    NPC_IMAGE_OF_NOTH,
}

-- PosTalkLocations (zone_dragonblight.cpp:130-137): {x, y, z, o}.
local POS_TALK_LOCATIONS = {
    { 3805.453, -682.9075, 222.2917, 2.793398 },
    { 3807.508, -691.0882, 221.9688, 2.094395 },
    { 3797.228, -690.3555, 222.5019, 1.134464 },
    { 3804.038, -672.3098, 222.5019, 4.578917 },
    { 3815.097, -680.2596, 221.9777, 2.86234 },
    { 3798.05, -680.611, 222.9825, 6.038839 },
}

local timers = {}
local eligorState = {}

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

local function storeTargets(creature, guid)
    local st = eligorState[guid]
    if st == nil then
        return
    end
    st.audience = {}
    for _, entry in ipairs(AUDIENCE_MOBS) do
        for _, mob in ipairs(creature:GetCreaturesInGrid(entry, 15.0)) do
            if #st.audience >= 10 then
                break
            end
            st.audience[#st.audience + 1] = mob
        end
        if #st.audience >= 10 then
            break
        end
    end
    st.images = {}
    for i, entry in ipairs(IMAGE_ANCHORS) do
        st.images[i] = creature:FindNearestCreature(entry, 20.0, true)
    end
end

local function onGetTargets(creature, guid)
    storeTargets(creature, guid)
end

local function onMoveToPoint(creature, guid)
    local st = eligorState[guid]
    if st == nil then
        return
    end
    local pos = POS_TALK_LOCATIONS[st.talkWing + 1]
    creature:SetWalk(true)
    creature:MoveTo(1, pos[1], pos[2], pos[3])
end

local function onTalkComplete(creature, guid)
    local st = eligorState[guid]
    if st == nil then
        return
    end
    st.talkWing = 5
    creature:Talk(SAY_TALK_COMPLETE)
    schedule(guid, "move_to_point", 5000, function() onMoveToPoint(creature, guid) end)
end

local function changeImage(creature, guid, entry, model, text)
    local st = eligorState[guid]
    if st == nil then
        return
    end
    local image = st.images[st.talkWing + 1]
    if image ~= nil then
        creature:Talk(text)
        image:SetEntry(entry)
        image:SetDisplayId(model)
        image:CastSpell(image, SPELL_HEROIC_IMAGE_CHANNEL)
        schedule(guid, "talk_complete", 40000, function() onTalkComplete(creature, guid) end)
    end
end

local function onKelthuzad3(creature, guid)
    if eligorState[guid] == nil then
        return
    end
    creature:Talk(SAY_KELTHUZAD_3)
end

local function onKelthuzad2(creature, guid)
    if eligorState[guid] == nil then
        return
    end
    creature:Talk(SAY_KELTHUZAD_2)
    schedule(guid, "kelthuzad_3", 8000, function() onKelthuzad3(creature, guid) end)
end

local function onDeathKnights4(creature, guid)
    if eligorState[guid] == nil then
        return
    end
    creature:Talk(SAY_DEATH_KNIGHTS_4)
end

local function onDeathKnights3(creature, guid)
    local st = eligorState[guid]
    if st == nil then
        return
    end
    creature:Talk(SAY_DEATH_KNIGHTS_3)
    local image = st.images[st.talkWing + 1]
    if image ~= nil then
        image:SetEntry(NPC_IMAGE_OF_ZELIEK)
        image:SetDisplayId(MODEL_IMAGE_OF_ZELIEK)
    end
    schedule(guid, "death_knights_4", 10000, function() onDeathKnights4(creature, guid) end)
end

local function onDeathKnights2(creature, guid)
    local st = eligorState[guid]
    if st == nil then
        return
    end
    creature:Talk(SAY_DEATH_KNIGHTS_2)
    local image = st.images[st.talkWing + 1]
    if image ~= nil then
        image:SetEntry(NPC_IMAGE_OF_BLAUMEUX)
        image:SetDisplayId(MODEL_IMAGE_OF_BLAUMEUX)
    end
    schedule(guid, "death_knights_3", 10000, function() onDeathKnights3(creature, guid) end)
end

local function onHeigan2(creature, guid)
    if eligorState[guid] == nil then
        return
    end
    creature:Talk(SAY_HEIGAN_2)
end

local function onStartRandom(creature, guid)
    local st = eligorState[guid]
    if st == nil then
        return
    end
    st.talkWing = math.random(0, 4)
    creature:Talk(st.talkWing)
    schedule(guid, "move_to_point", 8000, function() onMoveToPoint(creature, guid) end)
end

local function turnAudience(creature, guid)
    local st = eligorState[guid]
    if st == nil then
        return
    end
    for _, mob in ipairs(st.audience) do
        if mob ~= nil then
            mob:SetFacingToObject(creature)
        end
    end
end

local function onMovementInform(event, creature, moveType, pointId)
    local guid = creature:GetGUIDLow()
    local st = eligorState[guid]
    if st == nil then
        return
    end
    if moveType ~= POINT_MOTION_TYPE or pointId ~= 1 then
        return
    end
    local pos = POS_TALK_LOCATIONS[st.talkWing + 1]
    creature:SetFacingTo(pos[4])
    turnAudience(creature, guid)
    if st.talkWing == SAY_PINNACLE then
        if math.random(0, 1) == 0 then
            changeImage(creature, guid, NPC_IMAGE_OF_KELTHUZAD, MODEL_IMAGE_OF_KELTHUZAD, SAY_KELTHUZAD_1)
            schedule(guid, "kelthuzad_2", 8000, function() onKelthuzad2(creature, guid) end)
        else
            changeImage(creature, guid, NPC_IMAGE_OF_SAPPHIRON, MODEL_IMAGE_OF_SAPPHIRON, SAY_SAPPHIRON)
        end
    elseif st.talkWing == SAY_DEATH_KNIGHT_WING then
        local roll = math.random(0, 2)
        if roll == 0 then
            changeImage(creature, guid, NPC_IMAGE_OF_RAZUVIOUS, MODEL_IMAGE_OF_RAZUVIOUS, SAY_RAZUVIOUS)
        elseif roll == 1 then
            changeImage(creature, guid, NPC_IMAGE_OF_GOTHIK, MODEL_IMAGE_OF_GOTHIK, SAY_GOTHIK)
        else
            changeImage(creature, guid, NPC_IMAGE_OF_THANE, MODEL_IMAGE_OF_THANE, SAY_DEATH_KNIGHTS_1)
            schedule(guid, "death_knights_2", 10000, function() onDeathKnights2(creature, guid) end)
        end
    elseif st.talkWing == SAY_ABOMINATION_WING then
        local roll = math.random(0, 3)
        if roll == 0 then
            changeImage(creature, guid, NPC_IMAGE_OF_PATCHWERK, MODEL_IMAGE_OF_PATCHWERK, SAY_PATCHWERK)
        elseif roll == 1 then
            changeImage(creature, guid, NPC_IMAGE_OF_GROBBULUS, MODEL_IMAGE_OF_GROBBULUS, SAY_GROBBULUS)
        elseif roll == 2 then
            changeImage(creature, guid, NPC_IMAGE_OF_THADDIUS, MODEL_IMAGE_OF_THADDIUS, SAY_THADDIUS)
        else
            changeImage(creature, guid, NPC_IMAGE_OF_GLUTH, MODEL_IMAGE_OF_GLUTH, SAY_GLUTH)
        end
    elseif st.talkWing == SAY_SPIDER_WING then
        local roll = math.random(0, 2)
        if roll == 0 then
            changeImage(creature, guid, NPC_IMAGE_OF_ANUBREKHAN, MODEL_IMAGE_OF_ANUBREKHAN, SAY_ANUBREKHAN)
        elseif roll == 1 then
            changeImage(creature, guid, NPC_IMAGE_OF_FAERLINA, MODEL_IMAGE_OF_FAERLINA, SAY_FAERLINA)
        else
            changeImage(creature, guid, NPC_IMAGE_OF_MAEXXNA, MODEL_IMAGE_OF_MAEXXNA, SAY_MAEXXNA)
        end
    elseif st.talkWing == SAY_PLAGUE_WING then
        local roll = math.random(0, 2)
        if roll == 0 then
            changeImage(creature, guid, NPC_IMAGE_OF_NOTH, MODEL_IMAGE_OF_NOTH, SAY_NOTH)
        elseif roll == 1 then
            changeImage(creature, guid, NPC_IMAGE_OF_HEIGAN, MODEL_IMAGE_OF_HEIGAN, SAY_HEIGAN_1)
            schedule(guid, "heigan_2", 8000, function() onHeigan2(creature, guid) end)
        else
            changeImage(creature, guid, NPC_IMAGE_OF_LOATHEB, MODEL_IMAGE_OF_LOATHEB, SAY_LOATHEB)
        end
    elseif st.talkWing == SAY_TALK_COMPLETE then
        schedule(guid, "start_random", 30000, function() onStartRandom(creature, guid) end)
    end
end

local function onReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    eligorState[guid] = { talkWing = 0, audience = {}, images = {} }
    schedule(guid, "get_targets", 5000, function() onGetTargets(creature, guid) end)
    schedule(guid, "start_random", 20000, function() onStartRandom(creature, guid) end)
end

local function onLeaveCombat(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    eligorState[guid] = nil
end

local function onDied(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    eligorState[guid] = nil
end

RegisterCreatureEvent(ENTRY_ELIGOR, 5, onReset)
RegisterCreatureEvent(ENTRY_ELIGOR, 23, onReset)
RegisterCreatureEvent(ENTRY_ELIGOR, 28, onMovementInform)
RegisterCreatureEvent(ENTRY_ELIGOR, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY_ELIGOR, 4, onDied)
