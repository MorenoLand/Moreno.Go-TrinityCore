-- Vanndar Stormpike (Alterac Valley) --
-- Lua port of src/server/scripts/EasternKingdoms/AlteracValley/
-- boss_vanndar.cpp (boss_vanndar — TaskScheduler combat pump +
-- CheckInRoom home leash). Boss-script unit per
-- eastern_kingdoms_script_loader.cpp order (boss_vanndar follows
-- AddSC_boss_galvangar; closes the Alterac Valley boss set).
-- Entry (verifiable from the C++ sources): 11948 "Vanndar Stormpike"
-- in the BattlegroundAV.h AV creature lists (line 1184 spawn data,
-- lines 1268/1299; neighbors 11946/11947/11949 are Drek'Thar,
-- Galvangar, Balinda — the other AV boss-set members). The
-- creature_template ScriptName binding is DB-side (no TDB in this
-- workspace), but the entry itself is C++-verifiable so the port is
-- registered (storm_cloud / av_marshal / balinda / drekthar /
-- galvangar precedent).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Driven by CreateLuaEvent timers; melee is
-- engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
-- Ported arms (C++ JustEngagedWith schedule + UpdateAI event machine):
-- SPELL_AVATAR: victim-cast AVATAR 19135, 3s init -> {15,20}s;
-- SPELL_THUNDERCLAP: victim-cast THUNDERCLAP 15588, 4s init ->
--   {5,15}s;
-- SPELL_STORMBOLT: victim-cast STORMBOLT 20685, 6s init -> {10,25}s;
-- random yell Talk(YELL_RANDOM=2), {20,30}s init -> {20,30}s;
-- JustEngagedWith Talk(YELL_AGGRO=0).
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (creature
-- casts are packet-visual), so timers fire unconditionally and the
-- in-loop casting-state gate is dropped (maiden precedent). The C++
-- scheduler is driven from JustEngagedWith and drained by UpdateAI;
-- the port schedules from OnEnterCombat(1) and cancels on
-- OnLeaveCombat(2)/OnDied(4)/OnReset(23) (maiden convention).
-- Unmodeled: the 5s home leash check (home-position 2D distance >
-- 50yd -> EnterEvadeMode + Talk(YELL_EVADE=1)) — no HomePosition
-- bridge on the luaMotionCreature surface (X/Y/Z reads only;
-- GetDistance2d needs two objects — av_marshal / balinda / drekthar
-- / galvangar precedent); evade still re-arms through OnReset(23)
-- when the engine leashes the creature.
-- Unmodeled: YELL_SPELL=3 (declared but unused by this AI) and the
-- commented-out YELL_RESPAWN1/YELL_RESPAWN2 (-1810010/-1810011,
-- "Missing in database" in the C++ comments).

local SPELL_AVATAR = 19135
local SPELL_THUNDERCLAP = 15588
local SPELL_STORMBOLT = 20685

local YELL_AGGRO = 0
local YELL_RANDOM = 2

local ENTRY_VANNDAR = 11948

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
    per[key] = CreateLuaEvent(fn, delay)
end

local function onAvatar(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_AVATAR)
    end
    schedule(guid, "avatar", {15000, 20000}, function() onAvatar(creature, guid) end)
end

local function onThunderclap(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_THUNDERCLAP)
    end
    schedule(guid, "thunderclap", {5000, 15000}, function() onThunderclap(creature, guid) end)
end

local function onStormbolt(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_STORMBOLT)
    end
    schedule(guid, "stormbolt", {10000, 25000}, function() onStormbolt(creature, guid) end)
end

local function onRandomYell(creature, guid)
    creature:Talk(YELL_RANDOM)
    schedule(guid, "random_yell", {20000, 30000}, function() onRandomYell(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(YELL_AGGRO)
    schedule(guid, "avatar", 3000, function() onAvatar(creature, guid) end)
    schedule(guid, "thunderclap", 4000, function() onThunderclap(creature, guid) end)
    schedule(guid, "stormbolt", 6000, function() onStormbolt(creature, guid) end)
    schedule(guid, "random_yell", {20000, 30000}, function() onRandomYell(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(_, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_VANNDAR, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_VANNDAR, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_VANNDAR, 4, onCombatEnd)
RegisterCreatureEvent(ENTRY_VANNDAR, 23, onReset)
