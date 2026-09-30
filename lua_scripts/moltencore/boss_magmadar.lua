-- Magmadar (Molten Core) — Lua port of
-- src/server/scripts/EasternKingdoms/BlackrockMountain/MoltenCore/
-- boss_magmadar.cpp (boss_magmadarAI only); molten_core.h:31
-- (BOSS_MAGMADAR = 1, second boss), :55 (NPC_MAGMADAR = 11982).
-- Creature entry: 11982 Magmadar (C++ ScriptName "boss_magmadar" per
-- AddSC_boss_magmadar). One Talk line: EMOTE_FRENZY = 0, spoken on
-- the frenzy arm. SDComment upstream: "Conflag on ground nyi".
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnReset: triggered
-- self-cast 19449 (magma spit; C++ DoCast(me, SPELL_MAGMA_SPIT,
-- true)). OnEnterCombat: arm frenzy 19451 30s then 15s — Talk(0) +
-- non-triggered self-cast (C++ DoCast default is triggered=false —
-- vaelastrasz convention) / panic 19408 20s then 35s, non-triggered
-- DoCastVictim / lava bomb 19428 12s then 12s, non-triggered on a
-- random alive player in the instance (the C++ -SPELL_LAVA_BOMB
-- no-aura filter has no HasAura bridge — the pick may land on a
-- target already carrying it). Nil-target ticks cast nothing but keep
-- the schedule (jeklik convention). OnDied/OnLeaveCombat(2)/
-- OnReset(23): cancel timers.
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING queue gate and
-- the post-event casting check have no UNIT_STATE bridge — timers fire
-- unconditionally (jeklik convention); no instance-script model —
-- boss admission via the luaBossAI shim (BossAI::JustEngagedWith and
-- the BOSS_MAGMADAR bookkeeping arms skipped).

local ENTRY_MAGMADAR = 11982

local SPELL_FRENZY = 19451
local SPELL_MAGMA_SPIT = 19449
local SPELL_PANIC = 19408
local SPELL_LAVA_BOMB = 19428

local EMOTE_FRENZY = 0

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
    per[key] = CreateLuaEvent(fn, delay)
end

-- C++ EVENT_FRENZY: Talk(EMOTE_FRENZY); non-triggered self-cast
-- 19451; re-arm 15s.
local function onFrenzy(creature, guid)
    creature:Talk(EMOTE_FRENZY)
    creature:CastSpell(creature, SPELL_FRENZY)
    schedule(guid, "frenzy", 15000, function()
        onFrenzy(creature, guid)
    end)
end

-- C++ EVENT_PANIC: non-triggered DoCastVictim(19408); re-arm 35s.
local function onPanic(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_PANIC)
    end
    schedule(guid, "panic", 35000, function()
        onPanic(creature, guid)
    end)
end

-- C++ EVENT_LAVA_BOMB: non-triggered cast 19428 on a random alive
-- player in the instance (the C++ -SPELL_LAVA_BOMB no-aura filter has
-- no HasAura bridge — the pick may land on a target already carrying
-- it); re-arm 12s.
local function randomAlivePlayer(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local players = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            players[#players + 1] = p
        end
    end
    if #players == 0 then
        return nil
    end
    return players[math.random(#players)]
end

local function onLavaBomb(creature, guid)
    local pick = randomAlivePlayer(creature)
    if pick then
        creature:CastSpell(pick, SPELL_LAVA_BOMB)
    end
    schedule(guid, "lavabomb", 12000, function()
        onLavaBomb(creature, guid)
    end)
end

local function magmadarEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "frenzy", 30000, function()
        onFrenzy(creature, guid)
    end)
    schedule(guid, "panic", 20000, function()
        onPanic(creature, guid)
    end)
    schedule(guid, "lavabomb", 12000, function()
        onLavaBomb(creature, guid)
    end)
end

local function magmadarResetState(guid)
    cancelTimers(guid)
end

local function magmadarLeaveCombat(event, creature)
    magmadarResetState(creature:GetGUID())
end

local function magmadarDied(event, creature, killer)
    magmadarResetState(creature:GetGUID())
end

-- C++ Reset: triggered self-cast of magma spit 19449 (C++-exact);
-- the BossAI::Reset instance bookkeeping has no bridge.
local function magmadarReset(event, creature)
    magmadarResetState(creature:GetGUID())
    creature:CastSpell(creature, SPELL_MAGMA_SPIT, true)
end

RegisterCreatureEvent(ENTRY_MAGMADAR, 1, magmadarEnterCombat)
RegisterCreatureEvent(ENTRY_MAGMADAR, 2, magmadarLeaveCombat)
RegisterCreatureEvent(ENTRY_MAGMADAR, 4, magmadarDied)
RegisterCreatureEvent(ENTRY_MAGMADAR, 23, magmadarReset)
