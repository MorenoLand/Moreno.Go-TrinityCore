-- Darkshore: Kerlonian Evershade — Lua port of
-- src/server/scripts/Kalimdor/zone_darkshore.cpp
-- (class npc_kerlonian : CreatureScript("npc_kerlonian");
-- struct npc_kerlonianAI : public FollowerAI).
-- Entry: 11218 (creature_template ScriptName npc_kerlonian, TDB
-- 26091); Liladris Moonriver 11219. Quest 5321 "Sleeper Awakened".
-- Eluna creature events: 31 OnQuestAccept, 14 OnHitBySpell,
-- 4 OnDied, 23 OnReset.
-- Ported arms (C++ order):
-- - OnQuestAccept (zone_darkshore.cpp:160-167): quest 5321 ->
--   SetStandState(STAND) + Talk(SAY_KER_START 0, player) +
--   StartFollow(player, FACTION_ESCORTEE_N_NEUTRAL_PASSIVE 113,
--   5321) + 1s pump start.
-- - SpellHit (zone_darkshore.cpp:104-108): while
--   HasFollowState(INPROGRESS|PAUSED) and spell SPELL_AWAKEN
--   17536 -> ClearSleeping: RemoveAura(SPELL_SLEEP_VISUAL 25148)
--   + SetStandState(STAND) + Talk(EMOTE_KER_AWAKEN 5) +
--   SetFollowPaused(false).
-- - UpdateFollowerAI sleep machine (zone_darkshore.cpp:130-158):
--   1s pump; while INPROGRESS, not engaged and not PAUSED the
--   FallAsleepTimer counts down (init urand 10-45s, re-arm urand
--   25-90s) -> SetSleeping: SetFollowPaused(true) + Talk
--   (EMOTE_KER_SLEEP 1) + Talk(SAY_KER_SLEEP 2) +
--   SetStandState(SLEEP 3) + CastSpell(self, SPELL_SLEEP_VISUAL,
--   false). While engaged the timer freezes (C++ returns to
--   DoMeleeAttackIfReady; melee rides the engine).
-- - MoveInLineOfSight Liladris leg (zone_darkshore.cpp:88-102):
--   the 1s pump polls GetNearestCreature(25, NPC_LILADRIS) — the
--   ev27 twin (ev27 fires only from the aggro-engage branch, and
--   Liladris is not an enemy); no victim, not COMPLETE, within
--   INTERACTION_DISTANCE*5 = 25y -> leader with quest 5321
--   INCOMPLETE gets GroupEventHappens(5321, me) + Talk
--   (SAY_KER_END 4) + SetFollowComplete().
-- - FollowerAI::UpdateAI watchdog (ScriptedFollowerAI.cpp:105-150):
--   the 1s pump despawns on COMPLETE (kerlonian never sets
--   POSTEVENT), on leader unresolvable / beyond 100y, or when the
--   follow quest is neither INCOMPLETE nor COMPLETE (abandoned).
--   C++ re-checks every 1s after an initial 2.5s; the 1s pump is
--   the same cadence. All watchdog legs freeze while engaged
--   (C++ UpdateAI gates them on !IsEngaged).
-- - FollowerAI::JustDied (ScriptedFollowerAI.cpp:47-66): ev4 ->
--   while INPROGRESS the leader fails quest 5321.
-- - Reset (zone_darkshore.cpp:83-86): FallAsleepTimer re-init;
--   the pump restarts while INPROGRESS (C++ Reset touches only
--   the timer — follow state survives evade).
-- Unmodeled (documented-only, no bridges):
-- - FollowerAI::JustDied group fail leg: C++ fails the quest for
--   every group member in the leader's map; no group-iteration
--   bridge exists, so only the leader fails it.
-- - GetLeaderForFollower group migration: a dead leader hands off
--   to a live group member within 100y; the bridge returns nil
--   for a dead leader, so the watchdog despawns (same visible
--   outcome for a solo leader).
-- - FollowerAI::MoveInLineOfSight assist gate
--   (ShouldAssistPlayerInCombatAgainst): the CAN_ASSIST flag /
--   accessible-place / evade checks have no Lua twin; the port
--   never assists, matching the no-victim poll shape.
-- - FollowerAI::JustReachedHome re-follow: no evade-home event
--   carries the follow resume; the follow slot survives evade in
--   Go, so the NPC resumes following without it.

local ENTRY_KERLONIAN = 11218
local NPC_LILADRIS = 11219

local QUEST_SLEEPER_AWAKENED = 5321
local SPELL_SLEEP_VISUAL = 25148
local SPELL_AWAKEN = 17536
local FACTION_ESCORTEE_N_NEUTRAL_PASSIVE = 113

local SAY_KER_START = 0
local EMOTE_KER_SLEEP = 1
local SAY_KER_SLEEP = 2
local SAY_KER_END = 4
local EMOTE_KER_AWAKEN = 5

local STATE_FOLLOW_INPROGRESS = 0x001
local STATE_FOLLOW_PAUSED = 0x002
local STATE_FOLLOW_COMPLETE = 0x004

local QUEST_STATUS_COMPLETE = 1
local QUEST_STATUS_INCOMPLETE = 3

local UNIT_STAND_STATE_STAND = 0
local UNIT_STAND_STATE_SLEEP = 3

local MAX_PLAYER_DISTANCE = 100.0
local LILADRIS_RANGE = 25.0

local timers = {}
local states = {}

-- Forward declaration: armPump's closure calls kerlonianTick, which
-- is assigned below. The declaration MUST precede armPump: a
-- local's scope starts at its declaration, so a closure defined
-- before it binds the global and nil-calls at runtime
-- (runtime-proved on the repo's go-lua: the draft order errors
-- "attempt to call a nil value"). A later `local function` here
-- would shadow this slot — plain assignment only
-- (tharon_ja/gal_darah idiom).
local kerlonianTick

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
        kerlonianTick(creature, guid)
    end, 1000)
end

-- C++ FollowerAI::UpdateAI watchdog (ScriptedFollowerAI.cpp:105-150)
-- + npc_kerlonianAI::UpdateFollowerAI (zone_darkshore.cpp:130-158),
-- 1s granularity exact for all C++ timers.
kerlonianTick = function(creature, guid)
    local st = states[guid]
    if not st then
        return
    end
    if creature:IsDead() then
        states[guid] = nil
        return
    end
    -- Engaged freeze (ScriptedFollowerAI.cpp UpdateAI): the whole
    -- watchdog (COMPLETE/range/quest) and the sleep machine run
    -- only while !IsEngaged; in combat the pump just re-arms.
    if creature:GetVictim() ~= nil then
        armPump(guid, creature)
        return
    end
    -- Watchdog: COMPLETE && !POSTEVENT despawns (kerlonian never
    -- sets POSTEVENT, so the tick after SetFollowComplete despawns).
    if creature:HasFollowState(STATE_FOLLOW_COMPLETE) then
        states[guid] = nil
        creature:Despawn()
        return
    end
    if not creature:HasFollowState(STATE_FOLLOW_INPROGRESS) then
        states[guid] = nil
        return
    end
    -- Watchdog: leader unresolvable / beyond 100y / quest abandoned
    -- despawns. C++ checks the leader's group members too; the port
    -- checks the leader only (no group-iteration bridge).
    local leader = creature:GetLeaderForFollower()
    if leader == nil then
        states[guid] = nil
        creature:Despawn()
        return
    end
    local dx = creature:GetX() - leader:GetX()
    local dy = creature:GetY() - leader:GetY()
    local dz = creature:GetZ() - leader:GetZ()
    if dx * dx + dy * dy + dz * dz > MAX_PLAYER_DISTANCE * MAX_PLAYER_DISTANCE then
        states[guid] = nil
        creature:Despawn()
        return
    end
    local questStatus = leader:GetQuestStatus(QUEST_SLEEPER_AWAKENED)
    if questStatus ~= QUEST_STATUS_INCOMPLETE
            and questStatus ~= QUEST_STATUS_COMPLETE then
        states[guid] = nil
        creature:Despawn()
        return
    end
    -- MoveInLineOfSight Liladris leg (zone_darkshore.cpp:88-102).
    local liladris = creature:GetNearestCreature(LILADRIS_RANGE,
        NPC_LILADRIS, 0, 1)
    if liladris ~= nil then
        if questStatus == QUEST_STATUS_INCOMPLETE then
            leader:GroupEventHappens(QUEST_SLEEPER_AWAKENED, creature)
        end
        creature:Talk(SAY_KER_END)
        creature:SetFollowComplete()
    end
    -- Sleep machine (zone_darkshore.cpp:130-149): frozen while
    -- engaged (freeze above) or PAUSED; skipped on the
    -- tick that just completed the follow (the watchdog
    -- despawns next tick, C++-verbatim).
    if not creature:HasFollowState(STATE_FOLLOW_COMPLETE)
            and not creature:HasFollowState(STATE_FOLLOW_PAUSED) then
        if st.fallAsleep <= 1000 then
            -- SetSleeping (zone_darkshore.cpp:110-119).
            creature:SetFollowPaused(true)
            creature:Talk(EMOTE_KER_SLEEP)
            creature:Talk(SAY_KER_SLEEP)
            creature:SetStandState(UNIT_STAND_STATE_SLEEP)
            creature:CastSpell(creature, SPELL_SLEEP_VISUAL, false)
            st.fallAsleep = 25000 + math.random(0, 65000)
        else
            st.fallAsleep = st.fallAsleep - 1000
        end
    end
    armPump(guid, creature)
end

-- C++ npc_kerlonianAI::Reset (zone_darkshore.cpp:83-86) +
-- Initialize (zone_darkshore.cpp:72-75): FallAsleepTimer re-init.
-- Go fires ev23 on spawn, evade and death; the dead arm drops the
-- state (survivor idiom) and the pump restarts only while a follow
-- runs, since C++ Reset touches only the timer.
local function onReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelPump(guid)
    if creature:IsDead() then
        states[guid] = nil
        return
    end
    if creature:HasFollowState(STATE_FOLLOW_INPROGRESS) then
        states[guid] = { fallAsleep = 10000 + math.random(0, 35000) }
        armPump(guid, creature)
    else
        states[guid] = nil
    end
end

-- C++ npc_kerlonianAI::OnQuestAccept (zone_darkshore.cpp:160-167).
local function onQuestAccept(event, player, creature, quest)
    if not quest or quest:GetId() ~= QUEST_SLEEPER_AWAKENED then
        return
    end
    creature:SetStandState(UNIT_STAND_STATE_STAND)
    creature:Talk(SAY_KER_START, player)
    creature:StartFollow(player, FACTION_ESCORTEE_N_NEUTRAL_PASSIVE,
        QUEST_SLEEPER_AWAKENED)
    local guid = creature:GetGUIDLow()
    states[guid] = { fallAsleep = 10000 + math.random(0, 35000) }
    armPump(guid, creature)
end

-- C++ npc_kerlonianAI::SpellHit (zone_darkshore.cpp:104-108) +
-- ClearSleeping (zone_darkshore.cpp:121-128).
local function onSpellHit(event, creature, caster, spellId)
    if spellId ~= SPELL_AWAKEN then
        return
    end
    if not creature:HasFollowState(
            STATE_FOLLOW_INPROGRESS + STATE_FOLLOW_PAUSED) then
        return
    end
    creature:RemoveAura(SPELL_SLEEP_VISUAL)
    creature:SetStandState(UNIT_STAND_STATE_STAND)
    creature:Talk(EMOTE_KER_AWAKEN)
    creature:SetFollowPaused(false)
end

-- C++ FollowerAI::JustDied (ScriptedFollowerAI.cpp:47-66): the
-- leader fails the follow quest (group leg stays out, see header).
local function onDied(event, creature)
    local guid = creature:GetGUIDLow()
    cancelPump(guid)
    states[guid] = nil
    if creature:HasFollowState(STATE_FOLLOW_INPROGRESS) then
        local leader = creature:GetLeaderForFollower()
        if leader ~= nil then
            leader:FailQuest(QUEST_SLEEPER_AWAKENED)
        end
    end
end

RegisterCreatureEvent(ENTRY_KERLONIAN, 31, onQuestAccept)
RegisterCreatureEvent(ENTRY_KERLONIAN, 14, onSpellHit)
RegisterCreatureEvent(ENTRY_KERLONIAN, 4, onDied)
RegisterCreatureEvent(ENTRY_KERLONIAN, 23, onReset)
