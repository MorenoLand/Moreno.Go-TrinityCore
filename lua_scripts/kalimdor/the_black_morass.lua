-- The Black Morass: zone script — Lua port of
-- src/server/scripts/Kalimdor/CavernsOfTime/TheBlackMorass/
-- the_black_morass.cpp (359 lines; classes npc_medivh_bm :
-- public CreatureScript (npc_medivh_bmAI : public ScriptedAI) and
-- npc_time_rift : public CreatureScript (npc_time_riftAI :
-- public ScriptedAI); both GetAI via GetBlackMorassAI<T> (TBMScriptName
-- "instance_the_black_morass" gate); AddSC_the_black_morass at end
-- registers both; kalimdor loader decl 44 / call 157 per
-- kalimdor_script_loader.cpp — the fourth "// CoT The Black Morass"
-- loader group, right after AddSC_boss_temporus()).
-- Entries: the_black_morass.h:55 names NPC_MEDIVH = 15608 and
-- instance_the_black_morass.cpp:137 GUID-binds it in OnCreatureCreate
-- (DATA_MEDIVH); h:56 names NPC_TIME_RIFT = 17838 and
-- instance_the_black_morass.cpp:296 has medivh SummonCreature
-- (NPC_TIME_RIFT, ...) — summon-strength tie; the creature_template
-- ScriptName bindings stay DB-side.
-- Shared state lives in the instanceData table (see
-- instance_the_black_morass.lua): medivhState / riftState /
-- shield / portalCount / nextPortalMs / portalPending / pump_<guid>.
-- Eluna creature events: 4 OnDied, 7 OnAIUpdate, 14 OnHitBySpell,
-- 20 OnSummonedCreatureDespawn, 21 OnSummonedCreatureDied,
-- 23 OnReset.
-- Ported arms (C++-exact unless noted):
-- - npc_medivh_bm death (C++ JustDied): Talk SAY_DEATH (5) unless the
--   killer has Medivh's own entry (C++: killer &&
--   killer->GetEntry() == me->GetEntry() -> return — the self-kill
--   skip; thrall old_hillsbrad death-arm convention).
-- - npc_medivh_bm Reset (event 23): latches re-armed, then
--   TYPE_MEDIVH == IN_PROGRESS -> DoCast(me, SPELL_CHANNEL 31556,
--   triggered) else remove the aura if present.
-- - npc_medivh_bm MoveInLineOfSight player leg via the ev7 poll
--   (aeonus/deja/temporus precedent: ev7 fires pre-pull for every
--   alive motion; ev27 is player-only and aggro-scan-fired, so it
--   cannot carry a non-combat NPC's proximity leg): a player within
--   10.0f while TYPE_MEDIVH is neither IN_PROGRESS nor DONE ->
--   Talk SAY_INTRO (1) + channel cast (non-triggered) +
--   TYPE_MEDIVH/RIFT = IN_PROGRESS + InitWorldState broadcast
--   (BM 1 / SHIELD 100 / RIFT 0) + 15s first-portal schedule +
--   Check_Timer 5s start. The stored shield value is NOT reset here
--   (C++ InitWorldState only broadcasts 100).
-- - npc_medivh_bm MoveInLineOfSight infinite-mob leg via the same
--   ev7 poll: while TYPE_MEDIVH == IN_PROGRESS, every live mob of
--   the five infinite entries (17835/21818/17892/18994/18995) or
--   Aeonus (17881) within 15.0f -> StopMoving + CastSpell on Medivh
--   (SPELL_CORRUPT 31326, SPELL_CORRUPT_AEONUS 37853 for Aeonus,
--   non-triggered). GetCreaturesInGrid enumerates all of an entry
--   (C++ MIOS fires per mob); its metric is 2D + both combat reaches
--   where C++ IsWithinDistInMap is 3D — documented delta.
-- - npc_medivh_bm SpellHit via event 14 (the funnel fires 14 once per
--   hit creature, post-miss list): SPELL_CORRUPT_AEONUS hit ->
--   SpellCorrupt_Timer 1000, SPELL_CORRUPT hit -> 3000, ignored
--   while the timer runs (C++ early return). On expiry the
--   instance SetData(TYPE_MEDIVH, SPECIAL) leg runs inline (its
--   only caller is this timer): shield-- + SHIELD worldstate, and at
--   0 Medivh KillSelf (self as killer, so the JustDied self-kill
--   skip suppresses SAY_DEATH) + TYPE_MEDIVH FAIL + TYPE_RIFT
--   NOT_STARTED; otherwise re-arm 1000/3000 from HasAura
--   (AEONUS first, C++ order).
--   DELIBERATE DELTA (C++ instance_the_black_morass.cpp:172):
--   SetData(TYPE_MEDIVH, SPECIAL) decrements the shield ONLY while
--   TYPE_MEDIVH == IN_PROGRESS; otherwise the else branch sets the
--   encounter to SPECIAL (4), which would let a later player
--   proximity re-trigger SAY_INTRO and restart a finished event.
--   The expiry leg gates the decrement on IN_PROGRESS (a corrupt
--   tick landing in the <=3s window after a win no longer drains
--   the shield) but does NOT replicate the state=4 assignment,
--   and the re-arm still runs from HasAura (C++ re-arms outside
--   SetData).
-- - npc_medivh_bm UpdateAI Check_Timer 5s arm (self-rescheduling
--   one-shots, temporus timer idiom): shield latches -> Talk
--   SAY_WEAK25 (4) / SAY_WEAK50 (3) / SAY_WEAK75 (2) in the C++
--   else-if order (25 first, so one yell per tick); TYPE_MEDIVH ==
--   NOT_STARTED -> DespawnOrUnsummon + Respawn (no reschedule;
--   respawn's Reset re-arms); TYPE_RIFT == DONE -> Talk SAY_WIN (6)
--   + remove SPELL_CHANNEL + TYPE_MEDIVH DONE, check timer stopped.
-- - npc_time_rift boss-death legs (events 20/21, fired on the rift
--   as summoner): a died/despawned summon whose entry is a portal
--   boss (keeper 21104 / lord 17839 / deja 17879 / temporus 17880 /
--   aeonus 17881) means the rift's channel broke (C++ IsNonMelee-
--   SpellCast false leg, no bridge — boss death/despawn is the
--   channel-break cause): pump cancelled, rift KillSelf, then the
--   instance SetData(TYPE_RIFT, SPECIAL) leg inline (5s next portal,
--   only while TYPE_RIFT == IN_PROGRESS and portalCount < 7).
--   Infinite-mob deaths do not trigger this (entry filter).
-- - npc_time_rift OnDied (event 4): pump timer + wave state cleanup.
-- Unmodeled (documented-only, no bridges):
-- - npc_medivh_bm AttackStart: entirely commented out in C++ (no-op).
-- - npc_medivh_bm JustEngagedWith: empty in C++ (no-op).
-- - SetData TYPE_MEDIVH DONE quest credit (QUEST_OPENING_PORTAL
--   10297 + QUEST_MASTER_TOUCH 9836 AreaExploredOrEventHappens):
--   PORTED — inlined at both DONE writers (Aeonus JustDied in
--   boss_aeonus.lua, Medivh WIN leg in this file); the old
--   "no quest-event-happens bridge" note was stale
--   (player:AreaExploredOrEventHappens live, lua_player.go:4457).
-- - SAY_ENTER (0) "where does this belong?", SAY_ORCS_ENTER (7),
--   SAY_ORCS_ANSWER (8): declared in the MedivhBm enum but never
--   Talked anywhere in C++ (archimonde SAY_SOUL_CHARGE case).
-- - SPELL_PORTAL_RUNE 32570, SPELL_BLACK_CRYSTAL 32563,
--   SPELL_PORTAL_CRYSTAL 32564, SPELL_BANISH_PURPLE 32566,
--   SPELL_BANISH_GREEN 32567, C_COUNCIL_ENFORCER 17023: declared but
--   unused in this cpp.
-- - Time-rift Z water leg: summon Z is Map:GetHeight only (no
--   GetWaterLevel bridge; see instance file header).

local ENTRY_MEDIVH = 15608
local ENTRY_TIME_RIFT = 17838

local STATE_NOT_STARTED = 0
local STATE_IN_PROGRESS = 1
local STATE_FAIL = 2
local STATE_DONE = 3

local WORLD_STATE_BM = 2541
local WORLD_STATE_BM_SHIELD = 2540
local WORLD_STATE_BM_RIFT = 2784

local SPELL_CHANNEL = 31556
local SPELL_CORRUPT = 31326
local SPELL_CORRUPT_AEONUS = 37853

local QUEST_OPENING_PORTAL = 10297
local QUEST_MASTER_TOUCH = 9836
local QUEST_STATUS_INCOMPLETE = 3

local SAY_INTRO = 1
local SAY_WEAK75 = 2
local SAY_WEAK50 = 3
local SAY_WEAK25 = 4
local SAY_DEATH = 5
local SAY_WIN = 6

local INFINITE_ENTRIES = {17835, 21818, 17892, 18994, 18995}
local PORTAL_BOSS_ENTRIES = {
    [21104] = true,
    [17839] = true,
    [17879] = true,
    [17880] = true,
    [17881] = true,
}

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
    per[key] = CreateLuaEvent(fn, delay, 1)
end

local function instanceDataOf(creature)
    local mapObj = creature:GetMap()
    if not mapObj then
        return nil
    end
    return mapObj:GetInstanceData()
end

-- C++ SetData(TYPE_MEDIVH, DONE) quest-credit arm
-- (instance_the_black_morass.cpp:204-220): every player in the
-- instance with QUEST_OPENING_PORTAL 10297 / QUEST_MASTER_TOUCH 9836
-- incomplete gets AreaExploredOrEventHappens for it, 10297 first.
local function grantMedivhDoneCredit(mapObj)
    if not mapObj then
        return
    end
    local players = mapObj:GetPlayers()
    if not players then
        return
    end
    for _, player in ipairs(players) do
        if player:GetQuestStatus(QUEST_OPENING_PORTAL) == QUEST_STATUS_INCOMPLETE then
            player:AreaExploredOrEventHappens(QUEST_OPENING_PORTAL)
        end
        if player:GetQuestStatus(QUEST_MASTER_TOUCH) == QUEST_STATUS_INCOMPLETE then
            player:AreaExploredOrEventHappens(QUEST_MASTER_TOUCH)
        end
    end
end

local function medivhCheckTick(creature, guid)
    -- The firing one-shot's id must clear first: a later hit/tick
    -- tests key presence for "timer running" (C++ SpellCorrupt_Timer
    -- / Check_Timer nonzero), and a stale fired id would read as
    -- running forever (no aura -> no re-arm -> next hit ignored).
    local per = timers[guid]
    if per then
        per["check"] = nil
    end
    local data = instanceDataOf(creature)
    if not data then
        return
    end
    local shield = data.shield or 100
    if data.life25 ~= false and shield <= 25 then
        creature:Talk(SAY_WEAK25)
        data.life25 = false
    elseif data.life50 ~= false and shield <= 50 then
        creature:Talk(SAY_WEAK50)
        data.life50 = false
    elseif data.life75 ~= false and shield <= 75 then
        creature:Talk(SAY_WEAK75)
        data.life75 = false
    end
    if (data.medivhState or STATE_NOT_STARTED) == STATE_NOT_STARTED then
        creature:Despawn()
        creature:Respawn()
        return
    end
    if (data.riftState or STATE_NOT_STARTED) == STATE_DONE then
        creature:Talk(SAY_WIN)
        if creature:HasAura(SPELL_CHANNEL) then
            creature:RemoveAura(SPELL_CHANNEL)
        end
        data.medivhState = STATE_DONE
        -- C++ SetData(TYPE_MEDIVH, DONE) quest-credit arm
        -- (instance_the_black_morass.cpp:204-220) runs inline here:
        -- the second DONE writer (the first is Aeonus's JustDied).
        grantMedivhDoneCredit(creature:GetMap())
        return
    end
    schedule(guid, "check", 5000, function()
        medivhCheckTick(creature, guid)
    end)
end

local function medivhCorruptExpiry(creature, guid)
    local per = timers[guid]
    if per then
        per["corrupt"] = nil
    end
    local data = instanceDataOf(creature)
    if not data then
        return
    end
    -- C++ SetData(TYPE_MEDIVH, SPECIAL) (instance_the_black_morass.
    -- cpp:172) decrements the shield ONLY while TYPE_MEDIVH ==
    -- IN_PROGRESS; otherwise the else branch sets the encounter to
    -- SPECIAL (4) — deliberately not replicated (a later player
    -- proximity would re-trigger SAY_INTRO and restart a finished
    -- event). The re-arm below still runs: C++ re-arms outside
    -- SetData, from HasAura.
    if (data.medivhState or STATE_NOT_STARTED) == STATE_IN_PROGRESS then
        local shield = data.shield or 100
        if shield > 0 then
            shield = shield - 1
        end
        data.shield = shield
        creature:UpdateWorldState(WORLD_STATE_BM_SHIELD, shield)
        if shield == 0 then
            data.medivhState = STATE_FAIL
            data.riftState = STATE_NOT_STARTED
            creature:Kill(creature)
            return
        end
    end
    if creature:HasAura(SPELL_CORRUPT_AEONUS) then
        schedule(guid, "corrupt", 1000, function()
            medivhCorruptExpiry(creature, guid)
        end)
    elseif creature:HasAura(SPELL_CORRUPT) then
        schedule(guid, "corrupt", 3000, function()
            medivhCorruptExpiry(creature, guid)
        end)
    end
end

local function onMedivhAIUpdate(event, creature)
    local data = instanceDataOf(creature)
    if not data then
        return
    end
    local state = data.medivhState or STATE_NOT_STARTED
    if state ~= STATE_IN_PROGRESS and state ~= STATE_DONE then
        local players = creature:GetPlayersInRange(10)
        if players and #players > 0 then
            creature:Talk(SAY_INTRO)
            creature:CastSpell(creature, SPELL_CHANNEL, false)
            data.medivhState = STATE_IN_PROGRESS
            data.riftState = STATE_IN_PROGRESS
            data.shield = data.shield or 100
            creature:UpdateWorldState(WORLD_STATE_BM, 1)
            creature:UpdateWorldState(WORLD_STATE_BM_SHIELD, 100)
            creature:UpdateWorldState(WORLD_STATE_BM_RIFT, 0)
            data.nextPortalMs = 15000
            data.portalPending = true
            local guid = creature:GetGUID()
            schedule(guid, "check", 5000, function()
                medivhCheckTick(creature, guid)
            end)
            state = STATE_IN_PROGRESS
        end
    end
    if state == STATE_IN_PROGRESS then
        for _, entry in ipairs(INFINITE_ENTRIES) do
            local mobs = creature:GetCreaturesInGrid(entry, 15)
            if mobs then
                for _, mob in ipairs(mobs) do
                    if not mob:IsDead() then
                        mob:StopMoving()
                        mob:CastSpell(creature, SPELL_CORRUPT, false)
                    end
                end
            end
        end
        local aeonus = creature:GetCreaturesInGrid(17881, 15)
        if aeonus then
            for _, mob in ipairs(aeonus) do
                if not mob:IsDead() then
                    mob:StopMoving()
                    mob:CastSpell(creature, SPELL_CORRUPT_AEONUS, false)
                end
            end
        end
    end
end

local function onMedivhSpellHit(event, creature, caster, spellId)
    local delay = nil
    if spellId == SPELL_CORRUPT_AEONUS then
        delay = 1000
    elseif spellId == SPELL_CORRUPT then
        delay = 3000
    else
        return
    end
    local guid = creature:GetGUID()
    local per = timers[guid]
    if per and per["corrupt"] then
        return
    end
    schedule(guid, "corrupt", delay, function()
        medivhCorruptExpiry(creature, guid)
    end)
end

local function onMedivhReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    local data = instanceDataOf(creature)
    if not data then
        return
    end
    data.life25 = true
    data.life50 = true
    data.life75 = true
    if (data.medivhState or STATE_NOT_STARTED) == STATE_IN_PROGRESS then
        creature:CastSpell(creature, SPELL_CHANNEL, true)
    elseif creature:HasAura(SPELL_CHANNEL) then
        creature:RemoveAura(SPELL_CHANNEL)
    end
end

local function onMedivhDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    -- C++ JustDied skips the yell when the killer has Medivh's own
    -- entry (killer && killer->GetEntry() == me->GetEntry() -> return).
    if killer and killer:GetEntry() == ENTRY_MEDIVH then
        return
    end
    creature:Talk(SAY_DEATH)
end

local function onRiftSummonedGone(event, rift, summoned)
    if not PORTAL_BOSS_ENTRIES[summoned:GetEntry()] then
        return
    end
    local guid = rift:GetGUID()
    cancelTimers(guid)
    local data = instanceDataOf(rift)
    if data then
        data["pump_" .. guid] = nil
        data["riftwave_" .. guid] = nil
    end
    rift:Kill()
    if data and (data.riftState or STATE_NOT_STARTED) == STATE_IN_PROGRESS
        and (data.portalCount or 0) < 7 then
        data.nextPortalMs = 5000
        data.portalPending = true
    end
end

local function onRiftDied(event, rift, killer)
    local guid = rift:GetGUID()
    cancelTimers(guid)
    local data = instanceDataOf(rift)
    if data then
        data["pump_" .. guid] = nil
        data["riftwave_" .. guid] = nil
    end
end

RegisterCreatureEvent(ENTRY_MEDIVH, 7, onMedivhAIUpdate)
RegisterCreatureEvent(ENTRY_MEDIVH, 14, onMedivhSpellHit)
RegisterCreatureEvent(ENTRY_MEDIVH, 23, onMedivhReset)
RegisterCreatureEvent(ENTRY_MEDIVH, 4, onMedivhDied)
RegisterCreatureEvent(ENTRY_TIME_RIFT, 20, onRiftSummonedGone)
RegisterCreatureEvent(ENTRY_TIME_RIFT, 21, onRiftSummonedGone)
RegisterCreatureEvent(ENTRY_TIME_RIFT, 4, onRiftDied)
