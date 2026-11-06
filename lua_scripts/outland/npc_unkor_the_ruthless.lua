-- npc_unkor_the_ruthless (Terokkar Forest zone script)
-- Lua port of src/server/scripts/Outland/zone_terokkar_forest.cpp
-- (npc_unkor_the_ruthlessAI — ScriptedAI, registered by
-- AddSC_terokkar_forest in outland_script_loader.cpp).
-- Entry: the C++ sources carry NO NPC_ constant for this script
-- (nothing in the file's own enums — the script binds by DB
-- ScriptName), so the entry is cited from the quest-9889 page
-- (pd.zroot.it/quests/9889: "Unkor the Ruthless NPC #18262"),
-- cross-checked by the file's own NPC_BOULDERFIST_INVADER =
-- 18260 on the same page. Quest 9889 is "Don't Kill the Fat
-- One": kill 10 Boulderfist Invaders (18260), then beat Unkor
-- below 30% health until he submits.
-- Eluna creature events: 1 OnEnterCombat, 4 OnDied, 5 OnSpawn,
-- 9 OnDamageTaken, 23 OnReset. Melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- JustEngagedWith is empty in C++; the pulverize timer starts on
-- combat entry (its 3000ms initial value comes from Reset /
-- Initialize, and it only counts while in combat — see below).
-- Verifiable numbers (file's own UnkorTheRuthless enum):
-- SAY_SUBMIT = 0, REQUIRED_KILL_COUNT = 10,
-- SPELL_PULVERIZE = 2676, QUEST_DONTKILLTHEFATONE = 9889,
-- NPC_BOULDERFIST_INVADER = 18260. Factions from
-- SharedDefines.h: FACTION_OGRE = 45 (:233),
-- FACTION_FRIENDLY = 35 (:231). UNIT_STAND_STATE_SIT = 1
-- (UnitDefines.h:34).
-- Ported arms (C++-exact):
-- - Reset (ev5/ev23): SetStandState(STAND), SetFaction(OGRE),
--   latch cleared, timers cancelled (C++ Initialize resets
--   CanDoQuest, UnkorUnfriendly_Timer and Pulverize_Timer).
-- - DamageTaken (zone_terokkar_forest.cpp:111-141): attacker
--   must be a player (ToPlayer) and post-damage health strictly
--   below 30% (anzu integer-floor form: postDamage <
--   floor(maxHealth * 30 / 100)). Group leg: every group member
--   on the same map+instance with quest 9889 INCOMPLETE (3) and
--   kill count of 18260 == 10 gets AreaExploredOrEventHappens
--   (9889); solo leg: the same check on the attacker. No damage
--   rewrite — C++ only grants credit, damage applies normally.
--   This is a DamageTaken arm, so it DOES fire on the killing
--   blow (nefarian rule: only UpdateAI arms skip lethal hits) —
--   no postDamage > 0 guard. The credit leg is NOT latch-guarded
--   (C++ re-grants on every qualifying hit below 30%); only the
--   submit scheduling fires once, mirroring C++ DoNice setting
--   UnkorUnfriendly_Timer = 60000 so later ticks take the
--   countdown branch instead.
-- - Submit (UpdateAI :147-161, first tick after the latch):
--   Talk(SAY_SUBMIT 0), SetFaction(FRIENDLY), SetStandState
--   (SIT), RemoveAllAuras, CombatStop(true). Scheduled 100ms
--   after the latch flips (C++ fires it on the next UpdateAI
--   tick). EngagementOver has no bridge; CombatStop(true) is
--   the observable twin (fires no Lua hooks, same as C++).
-- - Unfriendly timer (UpdateAI :155-160): 60s after the submit
--   C++ calls EnterEvadeMode, whose Reset restores faction
--   OGRE + STAND and clears the latch. EnterEvadeMode itself
--   has no bridge (standing), so the 60s timer applies the
--   Reset state changes directly; the walk-home leg is
--   unmodeled (same treatment as calvin's evade leg).
-- - Pulverize (UpdateAI :163-168): DoCast(me, 2676) every 9s
--   after a 3s initial, self-cast. The repeating timer is NOT
--   cancelled on leave-combat (C++ Pulverize_Timer survives
--   combat; only Reset re-arms it to 3000) — instead each tick
--   is gated on IsInCombat, matching the C++ UpdateVictim gate.
--   Cancelled on death/reset, re-armed to 3000 on combat entry.
-- Stay-out (documented-only, no bridge on the Lua surface):
-- - spell_skyguard_flare (40655, :170-184): SpellScript
--   (destination-height override) — no SpellScript bridge.

local ENTRY_NPC_UNKOR_THE_RUTHLESS = 18262

local QUEST_DONTKILLTHEFATONE = 9889
local NPC_BOULDERFIST_INVADER = 18260
local REQUIRED_KILL_COUNT = 10

local SPELL_PULVERIZE = 2676

local SAY_SUBMIT = 0

local FACTION_OGRE = 45
local FACTION_FRIENDLY = 35

local UNIT_STAND_STATE_STAND = 0
local UNIT_STAND_STATE_SIT = 1

local QUEST_STATUS_INCOMPLETE = 3

local HEALTH_PCT_SUBMIT = 30

local timers = {}
local canDoQuest = {}

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

-- C++ UpdateAI (:163-168): DoCast(me, SPELL_PULVERIZE) on a 3s
-- initial / 9s repeat while in combat (UpdateVictim gate).
local function pulverizeTick(creature, guid)
    if creature:IsInCombat() then
        creature:CastSpell(creature, SPELL_PULVERIZE)
    end
    schedule(guid, "pulverize", 9000, function()
        pulverizeTick(creature, guid)
    end)
end

-- C++ DoNice (:97-109): Talk, friendly faction, sit, clear
-- auras, stop combat; the 60s UnkorUnfriendly_Timer then fires
-- EnterEvadeMode -> Reset (see header).
local function doNice(creature, guid)
    creature:Talk(SAY_SUBMIT)
    creature:SetFaction(FACTION_FRIENDLY)
    creature:SetStandState(UNIT_STAND_STATE_SIT)
    creature:RemoveAllAuras()
    creature:CombatStop(true)
    schedule(guid, "unfriendly", 60000, function()
        canDoQuest[guid] = false
        cancelTimers(guid)
        creature:SetFaction(FACTION_OGRE)
        creature:SetStandState(UNIT_STAND_STATE_STAND)
    end)
end

-- C++ Reset (:86-91): stand + ogre faction, latch/timers reset.
local function onReset(_, creature)
    local guid = creature:GetGUID()
    canDoQuest[guid] = false
    cancelTimers(guid)
    creature:SetStandState(UNIT_STAND_STATE_STAND)
    creature:SetFaction(FACTION_OGRE)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    schedule(guid, "pulverize", 3000, function()
        pulverizeTick(creature, guid)
    end)
end

local function onDied(_, creature)
    local guid = creature:GetGUID()
    canDoQuest[guid] = false
    cancelTimers(guid)
end

-- C++ DamageTaken (:111-141): player attacker, post-damage
-- health below 30% -> quest credit for every qualifying group
-- member (same map+instance, quest INCOMPLETE, 10 invaders
-- slain) or the solo attacker; latch flips once for the submit.
local function onDamageTaken(_, creature, attacker, damage)
    local guid = creature:GetGUID()
    local player = attacker and attacker:ToPlayer() or nil
    if not player then
        return
    end
    local health = creature:GetHealth()
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    local postDamage = math.max(health - damage, 0)
    if postDamage >= math.floor(maxHealth * HEALTH_PCT_SUBMIT / 100) then
        return
    end
    local function tryCredit(member)
        if member:GetMapId() == player:GetMapId()
                and member:GetInstanceId() == player:GetInstanceId()
                and member:GetQuestStatus(QUEST_DONTKILLTHEFATONE) == QUEST_STATUS_INCOMPLETE
                and member:GetReqKillOrCastCurrentCount(QUEST_DONTKILLTHEFATONE, NPC_BOULDERFIST_INVADER) == REQUIRED_KILL_COUNT then
            member:AreaExploredOrEventHappens(QUEST_DONTKILLTHEFATONE)
            return true
        end
        return false
    end
    local credited = false
    local group = player:GetGroup()
    if group then
        for _, member in ipairs(group:GetMembers()) do
            if tryCredit(member) then
                credited = true
            end
        end
    else
        credited = tryCredit(player)
    end
    if credited and not canDoQuest[guid] then
        canDoQuest[guid] = true
        schedule(guid, "submit", 100, function()
            doNice(creature, guid)
        end)
    end
end

RegisterCreatureEvent(ENTRY_NPC_UNKOR_THE_RUTHLESS, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_NPC_UNKOR_THE_RUTHLESS, 4, onDied)
RegisterCreatureEvent(ENTRY_NPC_UNKOR_THE_RUTHLESS, 5, onReset)
RegisterCreatureEvent(ENTRY_NPC_UNKOR_THE_RUTHLESS, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY_NPC_UNKOR_THE_RUTHLESS, 23, onReset)
