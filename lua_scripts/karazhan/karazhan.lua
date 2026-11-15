-- Karazhan zone file — Lua port of
-- src/server/scripts/EasternKingdoms/Karazhan/karazhan.cpp and
-- src/server/scripts/EasternKingdoms/Karazhan/instance_karazhan.cpp
-- (karazhan.h). Entries: npc_barnes 16812 (wowhead npc=16812/barnes),
-- npc_image_of_medivh 17651 (wowhead cata npc=17651/image-of-medivh);
-- the TDB creature_template ScriptName bindings are DB-side (no TDB in
-- this workspace). Deliberately unported (documented only — neither
-- script has a bridgeable core):
-- * npc_barnes (16812, EscortAI): the ev6 + Reset arms are PORTED
--   2026-11-15 in lua_scripts/karazhan/npc_barnes.lua (wp0 tuxedo +
--   stage door, wp4 spotlight summon + pause, wp8 performance-ready,
--   wp9 PrepareEncounter summons + curtains; Reset rolls the opera
--   event 1-3 per instance). Still unported here: the
--   TalkCount/TalkTimer dialogue machine (Oz/Hood/RAJ textids 0-11),
--   the gossip driver (OnGossipHello/Select StartEvent) and the
--   raid-wipe evade poll — all UpdateAI/gossip scope, so the ev6
--   handler fires once the driver lands.
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
--   15-step dialogue machine PORTED 2026-11-14 (ev28 arrival ->
--   StartEvent :494-510, step chain via CreateLuaEvent, fire-timer
--   cross-fire for steps 7..12): summon Arcanagos (17652, type 5
--   CORPSE_TIMED_DESPAWN 20s) + SetDisableGravity / MoveTo /
--   SetOrientation cross-creature arms, raw-text SendUnitYell
--   (LANG_UNIVERSAL 0) / SendUnitEmote, mana shield self-cast,
--   conflagration blast 30977 + fire ball 30967 cross-casts,
--   step-14 quest 9645 completion pass over GetMap():GetPlayers(),
--   step-15 arca:Kill() (KillSelf twin). Stay-outs, both inside step
--   12 only: setActive(true) (no bridge) + SetSpeedRate(MOVE_FLIGHT,
--   2.0) (MOVE_RUN-only bridge); the flee MovePoint + SetFarVisible +
--   InterruptNonMeleeSpells arms of that step are ported.
-- * Berthold (SPELL_TELEPORT 39567) and npc_arcane_protector /
--   npc_mana_feeder / bosses_opera content are untouched by this
--   file (arcane protector and mana feeder live in their own files).
-- Eluna creature events: 4 OnDied (cancel timers), 23 OnReset
-- (npc_image_of_medivh Reset, below), 28 MovementInform (arrival
-- starts the dialogue machine).

local ENTRY_IMAGE_OF_MEDIVH = 17651 -- karazhan.lua header; karazhan.h:55
local DATA_IMAGE_OF_MEDIVH = 26 -- karazhan.h:55
local ENTRY_ARCANAGOS = 17652 -- karazhan.cpp:60
local QUEST_MEDIVH = 9645 -- karazhan.cpp:596
local QUEST_STATUS_INCOMPLETE = 3
local LANG_UNIVERSAL = 0
local POINT_MOTION_TYPE = 8

local SPELL_FIRE_BALL = 30967 -- karazhan.cpp:52
local SPELL_CONFLAGRATION_BLAST = 30977 -- karazhan.cpp:54
local SPELL_MANA_SHIELD = 31635 -- karazhan.cpp:55

-- MedivPos (karazhan.cpp:426); orientation 1.94 is not consumed by
-- creature:MoveTo(id, x, y, z).
local MEDIVH_X, MEDIVH_Y, MEDIVH_Z, MEDIVH_O = -11161.49, -1902.24, 91.48, 1.94
local ARC_X, ARC_Y, ARC_Z, ARC_O = -11169.75, -1881.48, 95.39, 4.83
local ARC_FLEE_X, ARC_FLEE_Y, ARC_FLEE_Z = -11010.82, -1761.18, 156.47

-- Raw dialogue text (karazhan.cpp:414-424); C++ uses me->Yell /
-- arca->Yell (LANG_UNIVERSAL) and me->TextEmote for step 9.
local SAY_MEDIVH_1 = "You've got my attention, dragon. You'll find I'm not as easily scared as the villagers below."
local SAY_ARCANAGOS_2 = "Your dabbling in the arcane has gone too far, Medivh. You've attracted the attention of powers beyond your understanding. You must leave Karazhan at once!"
local SAY_MEDIVH_3 = "You dare challenge me at my own dwelling? Your arrogance is astounding, even for a dragon!"
local SAY_ARCANAGOS_4 = "A dark power seeks to use you, Medivh! If you stay, dire days will follow. You must hurry, we don't have much time!"
local SAY_MEDIVH_5 = "I do not know what you speak of, dragon... but I will not be bullied by this display of insolence. I'll leave Karazhan when it suits me!"
local SAY_ARCANAGOS_6 = "You leave me no alternative. I will stop you by force if you won't listen to reason!"
local EMOTE_MEDIVH_7 = "begins to cast a spell of great power, weaving his own essence into the magic."
local SAY_ARCANAGOS_8 = "What have you done, wizard? This cannot be! I'm burning from... within!"
local SAY_MEDIVH_9 = "He should not have angered me. I must go... recover my strength now..."

-- Per-GUID timer tables (boss_maiden_of_virtue.lua idiom) + dialogue
-- state (step = next step to execute, mirroring C++ Step after the
-- post-NextStep increment; arca = summoned Arcanagos object, nil on
-- summon failure — C++ guards every Arcanagos arm the same way).
local timers = {}
local medivhState = {}

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

-- Forward declarations (tharon_ja/gal_darah idiom): the step machine
-- and the fire chains reference each other; plain assignments below,
-- never re-`local function` (that would shadow the forward slot).
local doStep, nextStep, onArcaFire, onMedivhFire

-- C++ UpdateAI fire legs (karazhan.cpp:603-619): while Step is 7..12
-- both fire timers tick; Arcanagos casts fire ball at Medivh every
-- 6s, Medivh casts fire ball at Arcanagos every 5s. The gate reads
-- st.step (the post-increment value), so a fire scheduled past step
-- 12 simply stops re-arming.
onArcaFire = function(creature, guid)
    local st = medivhState[guid]
    if not st or not st.started or st.step < 7 or st.step > 12 then
        return
    end
    if st.arca then
        st.arca:CastSpell(creature, SPELL_FIRE_BALL)
    end
    schedule(guid, "arcaFire", 6000, function() onArcaFire(creature, guid) end)
end

onMedivhFire = function(creature, guid)
    local st = medivhState[guid]
    if not st or not st.started or st.step < 7 or st.step > 12 then
        return
    end
    if st.arca then
        creature:CastSpell(st.arca, SPELL_FIRE_BALL)
    end
    schedule(guid, "medivhFire", 5000, function() onMedivhFire(creature, guid) end)
end

-- C++ npc_image_of_medivhAI::NextStep (karazhan.cpp:518-600). Returns
-- the delay to the next step; the caller advances st.step, mirroring
-- the C++ Step++ after NextStep returns.
nextStep = function(creature, guid, step)
    local st = medivhState[guid]
    if not st then
        return 9999999
    end
    local arca = st.arca
    if step == 1 then
        creature:SendUnitYell(SAY_MEDIVH_1, LANG_UNIVERSAL)
        return 10000
    elseif step == 2 then
        if arca then arca:SendUnitYell(SAY_ARCANAGOS_2, LANG_UNIVERSAL) end
        return 20000
    elseif step == 3 then
        creature:SendUnitYell(SAY_MEDIVH_3, LANG_UNIVERSAL)
        return 10000
    elseif step == 4 then
        if arca then arca:SendUnitYell(SAY_ARCANAGOS_4, LANG_UNIVERSAL) end
        return 20000
    elseif step == 5 then
        creature:SendUnitYell(SAY_MEDIVH_5, LANG_UNIVERSAL)
        return 20000
    elseif step == 6 then
        if arca then arca:SendUnitYell(SAY_ARCANAGOS_6, LANG_UNIVERSAL) end
        -- The fire gate opens once Step becomes 7 (right after this
        -- step executes): both 0-initialised fire timers fire on the
        -- next UpdateAI tick, re-arming 6000/5000. The 100ms first
        -- delay is that next-tick fire.
        schedule(guid, "arcaFire", 100, function() onArcaFire(creature, guid) end)
        schedule(guid, "medivhFire", 100, function() onMedivhFire(creature, guid) end)
        return 10000
    elseif step == 7 then
        -- C++ overwrites the running Arcanagos timer with 500ms.
        schedule(guid, "arcaFire", 500, function() onArcaFire(creature, guid) end)
        return 5000
    elseif step == 8 then
        -- C++ overwrites the running Medivh timer with 500ms + the
        -- mana-shield self-cast (DoCast(me, SPELL_MANA_SHIELD)).
        schedule(guid, "medivhFire", 500, function() onMedivhFire(creature, guid) end)
        creature:CastSpell(creature, SPELL_MANA_SHIELD)
        return 10000
    elseif step == 9 then
        creature:SendUnitEmote(EMOTE_MEDIVH_7)
        return 10000
    elseif step == 10 then
        if arca then creature:CastSpell(arca, SPELL_CONFLAGRATION_BLAST) end
        return 1000
    elseif step == 11 then
        if arca then arca:SendUnitYell(SAY_ARCANAGOS_8, LANG_UNIVERSAL) end
        return 5000
    elseif step == 12 then
        if arca then
            arca:MoveTo(0, ARC_FLEE_X, ARC_FLEE_Y, ARC_FLEE_Z)
            arca:SetFarVisible(true)
            arca:InterruptNonMeleeSpells(true)
            -- setActive(true) + SetSpeedRate(MOVE_FLIGHT, 2.0) stay
            -- out: no setActive bridge; SetSpeedRate is MOVE_RUN-only.
        end
        return 10000
    elseif step == 13 then
        creature:SendUnitYell(SAY_MEDIVH_9, LANG_UNIVERSAL)
        return 10000
    elseif step == 14 then
        creature:SetVisible(false)
        creature:ClearInCombat()
        local map = creature:GetMap()
        if map then
            local players = map:GetPlayers()
            if players then
                for _, player in ipairs(players) do
                    if player:IsAlive()
                        and player:GetQuestStatus(QUEST_MEDIVH) == QUEST_STATUS_INCOMPLETE then
                        player:CompleteQuest(QUEST_MEDIVH)
                    end
                end
            end
        end
        return 50000
    elseif step == 15 then
        if arca then arca:Kill() end
        return 5000
    else
        return 9999999
    end
end

-- C++ UpdateAI YellTimer leg (karazhan.cpp:605-611): when the timer
-- expires and the event started, YellTimer = NextStep(Step++).
doStep = function(creature, guid)
    local st = medivhState[guid]
    if not st or not st.started then
        return
    end
    local step = st.step
    local delay = nextStep(creature, guid, step)
    st.step = step + 1
    if delay < 9999999 then
        schedule(guid, "step", delay, function() doStep(creature, guid) end)
    end
end

-- C++ npc_image_of_medivhAI::Reset (karazhan.cpp:468-480): Initialize()
-- is pure locals (Step 0 etc., consumed only by the dialogue machine);
-- the observable arms are the instance guid latch + walk to MedivPos,
-- else despawn the duplicate. GetGuidData returns 0 for an unlatched
-- type (the Go form of ObjectGuid::Empty); SetGuidData(26) latches the
-- caller's GUID; MoveTo records id 1 so the arrival fires
-- MovementInform (event 28) when the dialogue machine lands. Any
-- in-flight dialogue timers are cancelled (C++ Reset re-initialises
-- the machine).
local function onMedivhReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    medivhState[guid] = nil
    if creature:GetGuidData(DATA_IMAGE_OF_MEDIVH) == 0 then
        creature:SetGuidData(DATA_IMAGE_OF_MEDIVH)
        creature:MoveTo(1, MEDIVH_X, MEDIVH_Y, MEDIVH_Z)
    else
        creature:Despawn()
    end
end

local function onMedivhDied(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    medivhState[guid] = nil
end

-- C++ npc_image_of_medivhAI::MovementInform (karazhan.cpp:484-492):
-- point-1 arrival starts the event, then SetOrientation(MedivPos[3])
-- twice (duplicate in C++; once here, after StartEvent like C++).
-- C++ StartEvent (:494-510): Step 1, summon Arcanagos at ArcanagosPos
-- (orientation 0 in the summon, then SetDisableGravity + a zero-length
-- MovePoint + SetOrientation(4.83)), SetOrientation(1.94),
-- YellTimer 10000. On summon failure C++ returns with YellTimer 0, so
-- step 1 fires on the next UpdateAI tick — the 100ms delay below is
-- that tick.
local function onMedivhMoveInform(event, creature, moveType, pointId)
    if moveType ~= POINT_MOTION_TYPE or pointId ~= 1 then
        return
    end
    local guid = creature:GetGUIDLow()
    local arca = creature:SummonCreature(ENTRY_ARCANAGOS, ARC_X, ARC_Y, ARC_Z, 0, 5, 20000)
    medivhState[guid] = { step = 1, started = true, arca = arca }
    if arca then
        arca:SetDisableGravity(true)
        arca:MoveTo(0, ARC_X, ARC_Y, ARC_Z)
        arca:SetOrientation(ARC_O)
    end
    creature:SetOrientation(MEDIVH_O)
    schedule(guid, "step", arca and 10000 or 100, function() doStep(creature, guid) end)
end

RegisterCreatureEvent(ENTRY_IMAGE_OF_MEDIVH, 4, onMedivhDied)
RegisterCreatureEvent(ENTRY_IMAGE_OF_MEDIVH, 23, onMedivhReset)
RegisterCreatureEvent(ENTRY_IMAGE_OF_MEDIVH, 28, onMedivhMoveInform)
