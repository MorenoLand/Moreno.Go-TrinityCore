-- Undercity — Lua port of
-- src/server/scripts/EasternKingdoms/zone_undercity.cpp
-- (npc_lady_sylvanas_windrunner; npc_highborne_lamenter lives
-- in its own file, npc_highborne_lamenter.lua; npc_parqual_
-- fintallas is named in the file's ContentData header but has no
-- AI class in the C++ sources — unmodeled, velen precedent).
-- Entry: the C++ sources carry NO NPC_ constant for Sylvanas
-- (the script binds by DB ScriptName), so the entry is cited
-- from www.wowhead.com/classic/npc=10181/lady-sylvanas-
-- windrunner, cross-checked on the WotLK page (same ID 10181):
-- Lady Sylvanas Windrunner <Banshee Queen>, Undercity
-- (calvin 6784 precedent). Quest 9180 "Journey to Undercity"
-- is a WotLK quest whose End NPC is Sylvanas (Start: High
-- Executor Mavren) — the C++ OnQuestReward trigger matches.
-- Eluna creature events: 1 OnEnterCombat, 19 OnJustSummoned,
-- 2 OnLeaveCombat, 4 OnDied, 23 OnReset, 34 OnQuestReward.
-- Ported arms (C++-exact):
-- - JustEngagedWith: PlayDirectSound(SOUND_AGGRO 5886;
--   DoPlaySoundToSet = PlayDirectSound, gurtogg convention) +
--   FADE 30s / SUMMON_SKELETON 20s / BLACK_ARROW 15s / SHOOT 8s
--   / MULTI_SHOT 10s.
-- - UpdateAI combat rotation, IsCasting-gated (100ms pump retry,
--   no re-arm consumed, fireTimer convention): FADE 20672 +
--   FADE_BLINK 29211 (victim > 10yd -> MULTI_SHOT on victim),
--   re-arm urand(30000, 35000); SUMMON_SKELETON 59711 self,
--   urand(20000, 30000); BLACK_ARROW 59712 victim,
--   urand(15000, 20000); SHOOT 59710 victim, urand(8000, 10000);
--   MULTI_SHOT 59713 victim, urand(10000, 13000). Melee is
--   engine-driven (DoMeleeAttackIfReady).
-- - OnQuestReward(9180) inlines the SetGUID(GUID_EVENT_INVOKER
--   1) body (creature:SetGUID has no bridge; the SetGUID call is
--   C++ structure only — no other SetGUID(1) caller exists in
--   the file): Talk(EMOTE_LAMENT 2) + PlayDirectSound(
--   SOUND_CREDIT 10896) + non-triggered self-cast SYLVANAS_CAST
--   36568 + 4 lamenters (21628) at HighborneLoc + Y -61.00,
--   TEMPSUMMON_TIMED_DESPAWN (type 2) 160s + LAMENT 2s +
--   SUNSORROW_WHISPER 10s.
-- - LAMENT leg: if !HasAura(SYLVANAS_CAST) -> Talk(
--   SAY_LAMENT_END 1) + Talk(EMOTE_LAMENT_END 0) + PerformEmote(
--   EMOTE_ONESHOT_KNEEL 16) + Reset (all timers cancelled, state
--   cleared — C++ Reset clears the EventMap, combat pumps
--   included); else DoSummon bunny (21641) at a random point
--   within 10yd, 3s, type 2 + re-arm 2s.
-- - SUNSORROW leg (one-shot): FindNearestCreature(AMBASSADOR
--   16287, 20.0) -> ambassador Talk(SAY_SUNSORROW_WHISPER 0,
--   player via GetPlayerByGUID).
-- - JustSummoned (bunny 21641 only): the PREVIOUS bunny
--   (targetGUID) MoveJumps to (its X, its Y, Sylvanas Z + 15)
--   and gets RIBBON_OF_SOULS 37099 (non-triggered) from the new
--   bunny; targetGUID becomes the new bunny. MoveJump(x, y, z,
--   10.0, 15.0) is the arc twin (C++ passes orientation + speed
--   0; Go has no orientation/speed slots); the C++ UpdatePosition
--   snap after the jump has no bridge — the spline lands there.

local ENTRY_SYLVANAS = 10181

local QUEST_JOURNEY_TO_UNDERCITY = 9180

local EMOTE_LAMENT_END = 0
local SAY_LAMENT_END = 1
local EMOTE_LAMENT = 2
local SAY_SUNSORROW_WHISPER = 0

local SOUND_AGGRO = 5886
local SOUND_CREDIT = 10896

local NPC_HIGHBORNE_LAMENTER = 21628
local NPC_HIGHBORNE_BUNNY = 21641
local NPC_AMBASSADOR_SUNSORROW = 16287

local SPELL_SYLVANAS_CAST = 36568
local SPELL_RIBBON_OF_SOULS = 37099
local SPELL_BLACK_ARROW = 59712
local SPELL_FADE = 20672
local SPELL_FADE_BLINK = 29211
local SPELL_MULTI_SHOT = 59713
local SPELL_SHOT = 59710
local SPELL_SUMMON_SKELETON = 59711

local EMOTE_ONESHOT_KNEEL = 16

local HIGHBORNE_LOC = {
    { 1285.41, 312.47, 0.51 },
    { 1286.96, 310.40, 1.00 },
    { 1289.66, 309.66, 1.52 },
    { 1292.51, 310.50, 1.99 },
}
local HIGHBORNE_LOC_Y = -61.00

local timers = {}
local state = {}

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

local function getState(guid)
    local st = state[guid]
    if not st then
        st = { lament = false, playerGuid = 0, targetGuid = 0 }
        state[guid] = st
    end
    return st
end

-- C++ UpdateAI IsCasting gate (zone_undercity.cpp:178-180):
-- the event stays pending while casting; retry in 100ms with no
-- re-arm consumed (fireTimer convention).
local function combatArm(creature, guid, key, fn)
    if creature:IsDead() then
        return
    end
    if creature:IsCasting() then
        schedule(guid, key, 100, function()
            combatArm(creature, guid, key, fn)
        end)
        return
    end
    fn()
end

local function fadeArm(creature, guid)
    combatArm(creature, guid, "fade", function()
        creature:CastSpell(creature, SPELL_FADE)
        creature:CastSpell(creature, SPELL_FADE_BLINK)
        local victim = creature:GetVictim()
        if victim and creature:GetDistance(victim) > 10.0 then
            creature:CastSpell(victim, SPELL_MULTI_SHOT)
        end
        schedule(guid, "fade", math.random(30000, 35000), function()
            fadeArm(creature, guid)
        end)
    end)
end

local function skeletonArm(creature, guid)
    combatArm(creature, guid, "skeleton", function()
        creature:CastSpell(creature, SPELL_SUMMON_SKELETON)
        schedule(guid, "skeleton", math.random(20000, 30000), function()
            skeletonArm(creature, guid)
        end)
    end)
end

local function blackArrowArm(creature, guid)
    combatArm(creature, guid, "blackarrow", function()
        local victim = creature:GetVictim()
        if victim then
            creature:CastSpell(victim, SPELL_BLACK_ARROW)
        end
        schedule(guid, "blackarrow", math.random(15000, 20000), function()
            blackArrowArm(creature, guid)
        end)
    end)
end

local function shootArm(creature, guid)
    combatArm(creature, guid, "shoot", function()
        local victim = creature:GetVictim()
        if victim then
            creature:CastSpell(victim, SPELL_SHOT)
        end
        schedule(guid, "shoot", math.random(8000, 10000), function()
            shootArm(creature, guid)
        end)
    end)
end

local function multiShotArm(creature, guid)
    combatArm(creature, guid, "multishot", function()
        local victim = creature:GetVictim()
        if victim then
            creature:CastSpell(victim, SPELL_MULTI_SHOT)
        end
        schedule(guid, "multishot", math.random(10000, 13000), function()
            multiShotArm(creature, guid)
        end)
    end)
end

-- C++ EVENT_LAMENT_OF_THE_HIGHBORN (zone_undercity.cpp:222-235).
local function lamentArm(creature, guid)
    local st = state[guid]
    if not st or not st.lament or creature:IsDead() then
        return
    end
    if creature:IsCasting() then
        schedule(guid, "lament", 100, function()
            lamentArm(creature, guid)
        end)
        return
    end
    if not creature:HasAura(SPELL_SYLVANAS_CAST) then
        creature:Talk(SAY_LAMENT_END)
        creature:Talk(EMOTE_LAMENT_END)
        st.lament = false
        creature:PerformEmote(EMOTE_ONESHOT_KNEEL)
        -- C++ Reset(): Initialize + _events.Reset().
        cancelTimers(guid)
        st.playerGuid = 0
        st.targetGuid = 0
        return
    end
    local angle = math.random() * 2 * math.pi
    local dist = math.random() * 10.0
    creature:SummonCreature(NPC_HIGHBORNE_BUNNY, creature:GetX() + math.cos(angle) * dist, creature:GetY() + math.sin(angle) * dist, creature:GetZ(), 0, 2, 3000)
    schedule(guid, "lament", 2000, function()
        lamentArm(creature, guid)
    end)
end

-- C++ EVENT_SUNSORROW_WHISPER (zone_undercity.cpp:236-241),
-- one-shot.
local function sunsorrowArm(creature, guid)
    local st = state[guid]
    if not st or not st.lament or creature:IsDead() then
        return
    end
    if creature:IsCasting() then
        schedule(guid, "sunsorrow", 100, function()
            sunsorrowArm(creature, guid)
        end)
        return
    end
    local ambassador = creature:FindNearestCreature(NPC_AMBASSADOR_SUNSORROW, 20.0)
    if ambassador and st.playerGuid ~= 0 then
        local player = GetPlayerByGUID(st.playerGuid)
        if player then
            ambassador:Talk(SAY_SUNSORROW_WHISPER, player)
        end
    end
end

local function onEnterCombat(event, creature)
    creature:PlayDirectSound(SOUND_AGGRO)
    local guid = creature:GetGUIDLow()
    schedule(guid, "fade", 30000, function()
        fadeArm(creature, guid)
    end)
    schedule(guid, "skeleton", 20000, function()
        skeletonArm(creature, guid)
    end)
    schedule(guid, "blackarrow", 15000, function()
        blackArrowArm(creature, guid)
    end)
    schedule(guid, "shoot", 8000, function()
        shootArm(creature, guid)
    end)
    schedule(guid, "multishot", 10000, function()
        multiShotArm(creature, guid)
    end)
end

-- C++ OnQuestReward (zone_undercity.cpp:253-258) -> SetGUID(1)
-- body (zone_undercity.cpp:130-146), inlined: creature:SetGUID
-- has no bridge and no other SetGUID(1) caller exists in the file.
local function onQuestReward(event, player, creature, quest)
    if not quest or quest:GetId() ~= QUEST_JOURNEY_TO_UNDERCITY then
        return
    end
    if not player then
        return
    end
    local guid = creature:GetGUIDLow()
    local st = getState(guid)
    creature:Talk(EMOTE_LAMENT)
    creature:PlayDirectSound(SOUND_CREDIT)
    creature:CastSpell(creature, SPELL_SYLVANAS_CAST)
    st.playerGuid = player:GetGUID()
    st.lament = true
    for _, loc in ipairs(HIGHBORNE_LOC) do
        creature:SummonCreature(NPC_HIGHBORNE_LAMENTER, loc[1], loc[2], HIGHBORNE_LOC_Y, loc[3], 2, 160000)
    end
    schedule(guid, "lament", 2000, function()
        lamentArm(creature, guid)
    end)
    schedule(guid, "sunsorrow", 10000, function()
        sunsorrowArm(creature, guid)
    end)
end

-- C++ JustSummoned (zone_undercity.cpp:148-165), bunny leg only.
local function onJustSummoned(event, creature, summoned)
    if not summoned or summoned:GetEntry() ~= NPC_HIGHBORNE_BUNNY then
        return
    end
    local st = getState(creature:GetGUIDLow())
    if st.targetGuid ~= 0 then
        local target = creature:GetCreatureByGUID(st.targetGuid)
        if target then
            target:MoveJump(target:GetX(), target:GetY(), creature:GetZ() + 15.0, 10.0, 15.0)
            summoned:CastSpell(target, SPELL_RIBBON_OF_SOULS)
        end
    end
    summoned:SetDisableGravity(true)
    st.targetGuid = summoned:GetGUID()
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

local function onDied(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    state[guid] = nil
end

-- C++ Reset (zone_undercity.cpp:119-124): Initialize +
-- _events.Reset().
local function onReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    state[guid] = { lament = false, playerGuid = 0, targetGuid = 0 }
end

RegisterCreatureEvent(ENTRY_SYLVANAS, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_SYLVANAS, 19, onJustSummoned)
RegisterCreatureEvent(ENTRY_SYLVANAS, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY_SYLVANAS, 4, onDied)
RegisterCreatureEvent(ENTRY_SYLVANAS, 23, onReset)
RegisterCreatureEvent(ENTRY_SYLVANAS, 34, onQuestReward)
