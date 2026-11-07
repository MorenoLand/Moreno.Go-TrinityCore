-- Razorfen Downs: tomb creatures (Tomb Fiend / Tomb Reaver, used in the
-- Tuten Kash summon event) — Lua port of
-- src/server/scripts/Kalimdor/RazorfenDowns/razorfen_downs.cpp (class
-- npc_tomb_creature : CreatureScript("npc_tomb_creature") {
-- npc_tomb_creatureAI : public ScriptedAI }; GetAI via
-- GetRazorfenDownsAI<npc_tomb_creatureAI> (razorfen_downs.h —
-- GetInstanceAI<AI>(obj, "instance_razorfen_downs") gate,
-- DataHeader "RFD", EncounterCount = 5, map 47/233 gate via
-- instance_razorfen_downs.cpp); AddSC_razorfen_downs at end registers
-- the four zone scripts; kalimdor loader decl 73 / call 186 per
-- kalimdor_script_loader.cpp — the zone-script group of the
-- "// Razorfen Downs" loader block). Whole-server-tree grep for the
-- four script names ("belnistrasz", "idol_room_spawner",
-- "npc_tomb_creature", "\"go_gong\"") hits only razorfen_downs.cpp
-- (sole-source verified); zero sql/ hits for belnistrasz.
-- Entries: razorfen_downs.h RFDCreatureIds names NPC_TOMB_FIEND = 7349
-- and NPC_TOMB_REAVER = 7351 — kalecgos check PASSES for both;
-- creature_template ScriptName binding stays DB-side. Eluna creature
-- events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied, 23 OnReset.
-- Timers via CreateLuaEvent; melee is engine-driven in Go (creature
-- combat tick), like C++ DoMeleeAttackIfReady. The file has no
-- UNIT_STATE_CASTING gates.
-- Ported arms:
-- - Reset (C++ Reset, event 23): entry-conditional self-buff refresh —
--   Tomb Fiend (7349) DoCast(me, SPELL_POISON_PROC 3616) and Tomb
--   Reaver (7351) DoCast(me, SPELL_VIRULENT_POISON_PROC 12254), each
--   under its C++ !me->HasAura() guard (HasAura bridge exists —
--   hyjal_trash/kodo precedents; creature:GetEntry() is wired —
--   boss_midnight precedent).
-- - Engage (C++ JustEngagedWith): arm Web at init {5s,8s}.
-- - Web (EVENT_WEB): DoCastVictim(SPELL_WEB 745), reschedule {7s,16s}
--   unconditional (C++ Repeat sits outside any gate — faithful).
-- - JustDied (C++ JustDied): instance->SetData(DATA_WAVE,
--   me->GetEntry()) — the fiend/reaver kill counters + gong re-arm,
--   via the shared instanceData table (the Map:GetInstanceData twin
--   of the instance SetData switch; gong object from instanceData.gong,
--   stashed by the gong gossip handler, FindNearestGameObject
--   fallback). The gong-ring (IN_PROGRESS) half lives in
--   npc_belnistrasz.lua.
-- - LeaveCombat/Died cancel timers (hyjal.lua convention); Reset
--   cancels timers too.

local ENTRY_TOMB_FIEND  = 7349
local ENTRY_TOMB_REAVER = 7351

local SPELL_POISON_PROC          = 3616
local SPELL_VIRULENT_POISON_PROC = 12254
local SPELL_WEB                  = 745

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

-- C++ EVENT_WEB: DoCastVictim(745); reschedule {7s,16s} unconditional.
local function onWeb(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_WEB)
    end
    schedule(guid, "web", math.random(7000, 16000), function()
        onWeb(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "web", math.random(5000, 8000), function()
        onWeb(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ Reset: refresh the entry-conditional self-buff aura under its
-- HasAura guard (Poison Proc 3616 for Tomb Fiend, Virulent Poison
-- Proc 12254 for Tomb Reaver).
local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
    -- C++ Reset(): entry-gated DoCast(POISON_PROC/VIRULENT_POISON_PROC)
    -- is Reset-only (razorfen_downs.cpp). Alive-gated; the timer cancel
    -- rides the death leg deliberately.
    if creature:IsAlive() then
        local entry = creature:GetEntry()
        if entry == ENTRY_TOMB_FIEND then
            if not creature:HasAura(SPELL_POISON_PROC) then
                creature:CastSpell(creature, SPELL_POISON_PROC)
            end
        elseif entry == ENTRY_TOMB_REAVER then
            if not creature:HasAura(SPELL_VIRULENT_POISON_PROC) then
                creature:CastSpell(creature, SPELL_VIRULENT_POISON_PROC)
            end
        end
    end
end

local GO_GONG = 148917 -- razorfen_downs.h:50
local GAMEOBJECT_FLAGS = 9 -- UpdateFields.h: OBJECT_END 6 + 3 (arlokk precedent)
local GO_FLAG_NOT_SELECTABLE = 16 -- SharedDefines.h:1641

local function instanceDataOf(creature)
    local mapObj = creature:GetMap()
    if not mapObj then
        return nil
    end
    return mapObj:GetInstanceData()
end

-- C++ instance->SetData(DATA_WAVE, me->GetEntry()) (razorfen_downs.cpp:330)
-- -> instance_razorfen_downs.cpp:100-190: fiendsKilled/reaversKilled
-- counters; at 10 fiends / 4 reavers the gong loses NOT_SELECTABLE
-- (re-armed for the next wave). The counters live in the shared
-- instanceData table (no instance SetData hook in Go); the gong object
-- is instanceData.gong, stashed by the gong gossip handler (the C++
-- OnGameObjectCreate latch twin for the static gong, which never fires
-- the Go create hook), with a FindNearestGameObject fallback.
local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    local data = instanceDataOf(creature)
    if not data then
        return
    end
    local entry = creature:GetEntry()
    local key, target
    if entry == ENTRY_TOMB_FIEND then
        key, target = "fiendsKilled", 10
    elseif entry == ENTRY_TOMB_REAVER then
        key, target = "reaversKilled", 4
    else
        return
    end
    local killed = (data[key] or 0) + 1
    if killed == target then
        data[key] = 0
        local gong = data.gong
        if gong == nil then
            gong = creature:FindNearestGameObject(GO_GONG, 100.0)
        end
        if gong ~= nil then
            gong:RemoveFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE)
        end
    else
        data[key] = killed
    end
end

local ENTRIES = { ENTRY_TOMB_FIEND, ENTRY_TOMB_REAVER }
for _, entry in ipairs(ENTRIES) do
    RegisterCreatureEvent(entry, 1, onEnterCombat)
    RegisterCreatureEvent(entry, 2, onLeaveCombat)
    RegisterCreatureEvent(entry, 4, onDied)
    RegisterCreatureEvent(entry, 23, onReset)
end
