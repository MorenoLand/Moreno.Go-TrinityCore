-- Halls of Reflection intro dialogue — Lua port of
-- npc_jaina_or_sylvanas_intro_hor in
-- src/server/scripts/Northrend/FrozenHalls/HallsOfReflection/halls_of_reflection.cpp
-- (the first script of the "// Halls of Reflection" block in
-- AddSC_halls_of_reflection(), loader call 362).
-- Entries (halls_of_reflection.h:63-66): 37221 Jaina intro, 37223
-- Sylvanas intro, 37225 Uther, 37226 Lich King intro. The
-- CreatureScript ScriptName binding is DB-side as usual; the file is
-- walkdir-loaded, no boss_ai.go registration needed (escape-trash
-- precedent).
--
-- FACTION RULE: the C++ branches on instance DATA_TEAM_IN_INSTANCE
-- (no instance bridge), but the spawned creature's own entry encodes
-- its faction — 37221 IS the Alliance NPC, 37223 IS the Horde NPC —
-- so entry-based branching reproduces the C++ team branch for
-- whichever NPC spawned.
--
-- PORTED ARMS (C++-exact for the modeled legs; delays are ms):
-- Reset (ev23): events cancel, gossip|questgiver flags cleared,
--   SetStandState(STAND), WALK_INTRO1 at 3000ms.
-- WALK_INTRO1: Talk(0), then WALK_INTRO2 at 7000ms (Jaina) /
--   9000ms (Sylvanas); WALK_INTRO2: Talk(1) + gossip|questgiver
--   flags set. Movement legs stay out (no MovePoint bridge).
-- OnGossipSelect case 0 (see select note): close menu, flags
--   cleared, START_INTRO at 1000ms, then the faction chain at 0s.
-- Jaina chain (A2_1..A2_19): Talk(2) 7s, Talk(3) 10s, cast
--   65633 + triggered 70667 10s, summon Uther 2s, Uther Talk(0) 3s,
--   Talk(4) 7s, Uther Talk(1) 7s, Talk(5) 1200ms, Uther Talk(2) 11s,
--   Talk(6) 6s, Uther Talk(3) 12s, Talk(7) 6s, Uther Talk(4) 13s,
--   Talk(8) 12s, Uther Talk(5) 25s, Uther Talk(6) 6s, Talk(9) 5s,
--   Uther EMOTE_ONESHOT_NO + Talk(7) 12s, Talk(10) 3s, then LK.
-- Sylvanas chain (H2_1..H2_15): Talk(0) 8s, Talk(1) 6s, Talk(2) +
--   cast 65633 + triggered 70667 6s, summon Uther 2s, Uther Talk(9)
--   11s, Talk(3) 3s, Uther Talk(10) 6s, Talk(4) 5s, Uther Talk(11)
--   19s, Talk(5) 1500ms, Uther Talk(12) 19500ms, Talk(6) 2s, Uther
--   EMOTE_ONESHOT_NO + Talk(13) 12s, Uther Talk(14) 8s, Talk(7) 2s,
--   then LK.
-- LK chain (common): summon LK + Uther EmoteState(431) COWER +
--   Uther Talk(8 Jaina / 15 Sylvanas) 10s, LK Talk(0) 1s, Uther
--   triggered 70693 + 5s despawn 9s, LK triggered 72729 + triggered
--   73220 8s, LK Talk(1) 10s, Falric + Marwyn spawn-aura 72712 +
--   visible + LK Talk(2) 10s, Marwyn Talk(4) 1s, Falric Talk(5) 5s,
--   Falric Talk(6) 4s, Talk(11 Jaina / 8 Sylvanas) 5s, LK Talk(3
--   Jaina / 4 Sylvanas) 5s, INTRO_END: LK 5s despawn + intro NPC 10s
--   despawn. The Falric/Marwyn legs are PORTED 2026-11-05 via the
--   instanceData mirror (instance_halls_of_reflection.lua binds);
--   only the MovePoint legs in those steps stay out. Every cross-creature arm nil-guards
-- its summon (C++ null-checks the same fetches; a nil summon skips
-- the arm, it never nil-calls). Death (ev4) cancels the chain.
-- SPELL_CAST_VISUAL 65633 is marked "wrong" in the C++ enum and is
-- ported as-is.
--
-- DOCUMENTED-ONLY:
-- OnGossipHello's DATA_QUEL_DELAR_EVENT gate (IN_PROGRESS/SPECIAL
-- clears the menu): no instance bridge, so the menu shows
-- regardless; the gate stays out per the checkpoint head.
-- OnGossipSelect case 1 (SKIP_INTRO): PORTED — the creature select
-- hook now carries the trailing listID arg (gossip.go), so case 1
-- branches to the skip chain: LK summoned at 1s with REACT_PASSIVE
-- (movement/door legs stay out, no motion/GO bridges), then LK_4
-- (takeFrostmourne) at +15s, reusing the LK_STEPS tail from index 4.
-- All MovePoint legs (intro walks, LK walk-away, Falric/Marwyn
-- intro walks, Koreln/Loralen moves): no movement bridge.
-- Koreln/Loralen arms: the feign-death 29266 leg is PORTED 2026-11-08
--   (tick 492) via the instanceData mirror (korelnOrLoralen bind);
--   the MovePoint legs stay out (no movement bridge). Frostmourne /
--   impenetrable-door GO legs (HandleGameObject, phase mask): instance
--   GetGuidData, no bridge.
-- Falric/Marwyn aura (72712) + visible + Talk legs: PORTED 2026-11-05
--   via the instanceData mirror (instance_halls_of_reflection.lua
--   binds falric/marwyn; the old "instance GetGuidData, no bridge"
--   claim is retracted — the mirror IS the GetGuidData bridge, and
--   the fetch rule is the C++ GUID fetch, not a proximity lookup).
--   The MovePoint legs in the same steps stay out (no movement
--   bridge). ProcessEvent(EVENT_SPAWN_WAVES) in LK_9 is PORTED
--   2026-11-08 (tick 492) via the Go-only signal
--   instanceData:SetData(HOR_SETDATA_SPAWN_WAVES 1001, 0), run by the
--   instance event-8 handler (tick 491).
-- Instance SetData(DATA_INTRO_EVENT, DONE) is PORTED 2026-11-08
--   (tick 492) in introEnd via the same event-8 bridge. LK
--   SetWalk(true): no SetWalk bridge, moot without movement.

local ENTRY_JAINA_INTRO = 37221
local ENTRY_SYLVANAS_INTRO = 37223
local ENTRY_UTHER = 37225
local ENTRY_LK_INTRO = 37226

local UNIT_NPC_FLAG_GOSSIP = 0x00000001
local UNIT_NPC_FLAG_QUESTGIVER = 0x00000002
local UNIT_NPC_FLAGS_MASK = 0x00000003
local UNIT_STAND_STATE_STAND = 0
local EMOTE_ONESHOT_NO = 274
local EMOTE_STATE_COWER = 431
local REACT_PASSIVE = 0

local UTHER_SPAWN = { 5307.814, 2003.168, 709.4244, 4.537856 }
local LK_SPAWN = { 5362.463, 2062.693, 707.7781, 3.944444 }

local SPELL_CAST_VISUAL = 65633
local SPELL_FROSTMOURNE_SOUNDS = 70667
local SPELL_UTHER_DESPAWN = 70693
local SPELL_TAKE_FROSTMOURNE = 72729
local SPELL_FROSTMOURNE_VISUAL = 73220
local SPELL_BOSS_SPAWN_AURA = 72712 -- halls_of_reflection.cpp:181
local SPELL_FEIGN_DEATH = 29266 -- halls_of_reflection.cpp:254

local DATA_INTRO_EVENT = 3 -- halls_of_reflection.h:41
local BOSS_STATE_DONE = 3 -- InstanceScript.h:68-76
local HOR_SETDATA_SPAWN_WAVES = 1001 -- Go-only signal, instance_halls_of_reflection.lua

local SAY_MARWYN_INTRO_1 = 4 -- halls_of_reflection.cpp:112
local SAY_FALRIC_INTRO_1 = 5 -- halls_of_reflection.cpp:110
local SAY_FALRIC_INTRO_2 = 6 -- halls_of_reflection.cpp:111

local chainTimers = {}
local chainState = {}

local function cancelChain(guid)
    local id = chainTimers[guid]
    if id then
        RemoveEventById(id)
        chainTimers[guid] = nil
    end
    chainState[guid] = nil
end

local function runSteps(creature, guid, steps, index)
    local step = steps[index]
    if not step then
        chainTimers[guid] = nil
        return
    end
    step.fn(chainState[guid])
    if step.delay then
        chainTimers[guid] = CreateLuaEvent(function()
            chainTimers[guid] = nil
            runSteps(creature, guid, steps, index + 1)
        end, step.delay)
    else
        chainTimers[guid] = nil
    end
end

local function scheduleChain(creature, steps, delay)
    local guid = creature:GetGUIDLow()
    cancelChain(guid)
    chainState[guid] = {
        creature = creature,
        jaina = creature:GetEntry() == ENTRY_JAINA_INTRO,
    }
    chainTimers[guid] = CreateLuaEvent(function()
        chainTimers[guid] = nil
        runSteps(creature, guid, steps, 1)
    end, delay)
end

local function talkSelf(id)
    return function(st)
        st.creature:Talk(id)
    end
end

local function talkUther(id)
    return function(st)
        if st.uther then
            st.uther:Talk(id)
        end
    end
end

local function talkLK(id)
    return function(st)
        if st.lk then
            st.lk:Talk(id)
        end
    end
end

local function summonUther(st)
    local uther = st.creature:SummonCreature(ENTRY_UTHER,
        UTHER_SPAWN[1], UTHER_SPAWN[2], UTHER_SPAWN[3], UTHER_SPAWN[4])
    if uther then
        st.uther = uther
    end
end

local function castFrostmourne(st)
    st.creature:CastSpell(st.creature, SPELL_CAST_VISUAL)
    st.creature:CastSpell(st.creature, SPELL_FROSTMOURNE_SOUNDS, true)
end

local function summonLK(st)
    local lk = st.creature:SummonCreature(ENTRY_LK_INTRO,
        LK_SPAWN[1], LK_SPAWN[2], LK_SPAWN[3], LK_SPAWN[4])
    if lk then
        st.lk = lk
    end
    if st.uther then
        st.uther:EmoteState(EMOTE_STATE_COWER)
        st.uther:Talk(st.jaina and 8 or 15)
    end
end

local function banishUther(st)
    local uther = st.uther
    if uther then
        uther:CastSpell(uther, SPELL_UTHER_DESPAWN, true)
        CreateLuaEvent(function()
            uther:Despawn()
        end, 5000)
    end
    st.uther = nil
end

local function takeFrostmourne(st)
    if st.lk then
        st.lk:CastSpell(st.lk, SPELL_TAKE_FROSTMOURNE, true)
        st.lk:CastSpell(st.lk, SPELL_FROSTMOURNE_VISUAL, true)
    end
end

local function utherNoAndTalk(id)
    return function(st)
        if st.uther then
            st.uther:PerformEmote(EMOTE_ONESHOT_NO)
            st.uther:Talk(id)
        end
    end
end

-- C++ EVENT_INTRO_LK_6..9 Falric/Marwyn legs, via the instanceData
-- mirror (instance_halls_of_reflection.lua binds falric/marwyn; the
-- kelthuzad GetInstanceData precedent). Every fetch is nil-guarded
-- like the C++ GetCreature checks.
local function instanceDataTable(st)
    local mapObj = st.creature:GetMap()
    if not mapObj then
        return nil
    end
    return mapObj:GetInstanceData()
end

local function instanceBoss(st, key)
    local data = instanceDataTable(st)
    if not data then
        return nil
    end
    return data[key]
end

-- C++ EVENT_INTRO_LK_6 (:668-682): Falric + Marwyn CastSpell(self,
-- SPELL_BOSS_SPAWN_AURA 72712, true) + SetVisible(true), in C++ order,
-- then LK Talk(SAY_LK_INTRO_3 2). The LK MovePoint walk-away and the
-- 5s impenetrable-door open stay out (no movement / GO bridges).
local function spawnBossesAndTalkLK(st)
    local falric = instanceBoss(st, "falric")
    if falric then
        falric:CastSpell(falric, SPELL_BOSS_SPAWN_AURA, true)
        falric:SetVisible(true)
    end
    local marwyn = instanceBoss(st, "marwyn")
    if marwyn then
        marwyn:CastSpell(marwyn, SPELL_BOSS_SPAWN_AURA, true)
        marwyn:SetVisible(true)
    end
    talkLK(2)(st)
end

-- C++ EVENT_INTRO_LK_7 (:683-690): Marwyn Talk(SAY_MARWYN_INTRO_1 4);
-- the MovePoint leg stays out.
local function talkMarwynIntro(st)
    local marwyn = instanceBoss(st, "marwyn")
    if marwyn then
        marwyn:Talk(SAY_MARWYN_INTRO_1)
    end
end

-- C++ EVENT_INTRO_LK_8 (:691-698): Falric Talk(SAY_FALRIC_INTRO_1 5);
-- the MovePoint leg stays out.
local function talkFalricIntro1(st)
    local falric = instanceBoss(st, "falric")
    if falric then
        falric:Talk(SAY_FALRIC_INTRO_1)
    end
end

-- C++ EVENT_INTRO_LK_9 (:699-704): Falric Talk(SAY_FALRIC_INTRO_2 6),
-- then ProcessEvent(EVENT_SPAWN_WAVES) — the Go-only signal
-- instanceData:SetData(HOR_SETDATA_SPAWN_WAVES 1001, 0), run by the
-- instance event-8 handler (tick 491).
local function talkFalricIntro2(st)
    local falric = instanceBoss(st, "falric")
    if falric then
        falric:Talk(SAY_FALRIC_INTRO_2)
    end
    local data = instanceDataTable(st)
    if data then
        data:SetData(HOR_SETDATA_SPAWN_WAVES, 0)
    end
end

-- C++ EVENT_INTRO_END (:736-747): SetData(DATA_INTRO_EVENT, DONE),
-- KORELN_LORALEN_DEATH in 8s, LK despawn 5s, me despawn 10s; the
-- CLOSE_IMPENETRABLE_DOOR leg stays out (door model).
local function introEnd(st)
    local data = instanceDataTable(st)
    if data then
        data:SetData(DATA_INTRO_EVENT, BOSS_STATE_DONE)
    end
    -- C++ :741/:776-778: Koreln/Loralen self-cast SPELL_FEIGN_DEATH
    -- 29266 (non-triggered); fetched at fire time like the C++
    -- GetGuidData check.
    CreateLuaEvent(function()
        local korelnOrLoralen = instanceBoss(st, "korelnOrLoralen")
        if korelnOrLoralen then
            korelnOrLoralen:CastSpell(korelnOrLoralen, SPELL_FEIGN_DEATH)
        end
    end, 8000)
    local lk = st.lk
    if lk then
        CreateLuaEvent(function()
            lk:Despawn()
        end, 5000)
    end
    st.lk = nil
    local me = st.creature
    CreateLuaEvent(function()
        me:Despawn()
    end, 10000)
end

-- LK chain, shared by both factions; faction-variant Talks read st.jaina.
local LK_STEPS = {
    { fn = summonLK, delay = 10000 },
    { fn = talkLK(0), delay = 1000 },
    { fn = banishUther, delay = 9000 },
    { fn = takeFrostmourne, delay = 8000 },
    { fn = talkLK(1), delay = 10000 },
    { fn = spawnBossesAndTalkLK, delay = 10000 },
    { fn = talkMarwynIntro, delay = 1000 },
    { fn = talkFalricIntro1, delay = 5000 },
    { fn = talkFalricIntro2, delay = 4000 },
    { fn = function(st) st.creature:Talk(st.jaina and 11 or 8) end, delay = 5000 },
    { fn = function(st) if st.lk then st.lk:Talk(st.jaina and 3 or 4) end end, delay = 5000 },
    { fn = introEnd },
}

local JAINA_STEPS = {
    { fn = talkSelf(2), delay = 7000 },
    { fn = talkSelf(3), delay = 10000 },
    { fn = castFrostmourne, delay = 10000 },
    { fn = summonUther, delay = 2000 },
    { fn = talkUther(0), delay = 3000 },
    { fn = talkSelf(4), delay = 7000 },
    { fn = talkUther(1), delay = 7000 },
    { fn = talkSelf(5), delay = 1200 },
    { fn = talkUther(2), delay = 11000 },
    { fn = talkSelf(6), delay = 6000 },
    { fn = talkUther(3), delay = 12000 },
    { fn = talkSelf(7), delay = 6000 },
    { fn = talkUther(4), delay = 13000 },
    { fn = talkSelf(8), delay = 12000 },
    { fn = talkUther(5), delay = 25000 },
    { fn = talkUther(6), delay = 6000 },
    { fn = talkSelf(9), delay = 5000 },
    { fn = utherNoAndTalk(7), delay = 12000 },
    { fn = talkSelf(10), delay = 3000 },
}

local SYLVANAS_STEPS = {
    { fn = talkSelf(0), delay = 8000 },
    { fn = talkSelf(1), delay = 6000 },
    { fn = function(st)
        st.creature:Talk(2)
        castFrostmourne(st)
    end, delay = 6000 },
    { fn = summonUther, delay = 2000 },
    { fn = talkUther(9), delay = 11000 },
    { fn = talkSelf(3), delay = 3000 },
    { fn = talkUther(10), delay = 6000 },
    { fn = talkSelf(4), delay = 5000 },
    { fn = talkUther(11), delay = 19000 },
    { fn = talkSelf(5), delay = 1500 },
    { fn = talkUther(12), delay = 19500 },
    { fn = talkSelf(6), delay = 2000 },
    { fn = utherNoAndTalk(13), delay = 12000 },
    { fn = talkUther(14), delay = 8000 },
    { fn = talkSelf(7), delay = 2000 },
}

for _, step in ipairs(LK_STEPS) do
    table.insert(JAINA_STEPS, step)
    table.insert(SYLVANAS_STEPS, step)
end

local function onReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelChain(guid)
    creature:SetNPCFlags(bit32.band(creature:GetNPCFlags(),
        bit32.bnot(UNIT_NPC_FLAGS_MASK)))
    creature:SetStandState(UNIT_STAND_STATE_STAND)
    local walkSteps = {
        { fn = talkSelf(0), delay = nil },
        { fn = function(st)
            st.creature:Talk(1)
            st.creature:SetNPCFlags(bit32.bor(st.creature:GetNPCFlags(),
                UNIT_NPC_FLAGS_MASK))
        end },
    }
    walkSteps[1].delay = creature:GetEntry() == ENTRY_JAINA_INTRO and 7000 or 9000
    scheduleChain(creature, walkSteps, 3000)
end

local function onDeath(event, creature)
    cancelChain(creature:GetGUIDLow())
end

local function summonLKPassive(st)
    local lk = st.creature:SummonCreature(ENTRY_LK_INTRO,
        LK_SPAWN[1], LK_SPAWN[2], LK_SPAWN[3], LK_SPAWN[4])
    if lk then
        lk:SetReactState(REACT_PASSIVE)
        st.lk = lk
    end
end

local function onGossipSelect(event, player, creature, sender, action, code, listID)
    player:GossipClearMenu()
    if listID == 1 then
        player:GossipComplete()
        creature:SetNPCFlags(bit32.band(creature:GetNPCFlags(),
            bit32.bnot(UNIT_NPC_FLAGS_MASK)))
        local skipSteps = { { fn = summonLKPassive, delay = 15000 } }
        for i = 4, #LK_STEPS do
            table.insert(skipSteps, LK_STEPS[i])
        end
        scheduleChain(creature, skipSteps, 1000)
    elseif listID == 0 then
        player:GossipComplete()
        creature:SetNPCFlags(bit32.band(creature:GetNPCFlags(),
            bit32.bnot(UNIT_NPC_FLAGS_MASK)))
        if creature:GetEntry() == ENTRY_JAINA_INTRO then
            scheduleChain(creature, JAINA_STEPS, 1000)
        else
            scheduleChain(creature, SYLVANAS_STEPS, 1000)
        end
    end
end

RegisterCreatureEvent(ENTRY_JAINA_INTRO, 23, onReset)
RegisterCreatureEvent(ENTRY_SYLVANAS_INTRO, 23, onReset)
RegisterCreatureEvent(ENTRY_JAINA_INTRO, 4, onDeath)
RegisterCreatureEvent(ENTRY_SYLVANAS_INTRO, 4, onDeath)
RegisterCreatureGossipEvent(ENTRY_JAINA_INTRO, 2, onGossipSelect)
RegisterCreatureGossipEvent(ENTRY_SYLVANAS_INTRO, 2, onGossipSelect)
