-- Sulfuron Harbinger (Molten Core) — Lua port of
-- src/server/scripts/EasternKingdoms/BlackrockMountain/MoltenCore/
-- boss_sulfuron_harbinger.cpp (boss_sulfuronAI only); molten_core.h:36
-- (BOSS_SULFURON_HARBINGER = 6, seventh boss), :60
-- (NPC_SULFURON_HARBINGER = 12098).
-- Creature entry: 12098 Sulfuron Harbinger (C++ ScriptName
-- "boss_sulfuron" per AddSC_boss_sulfuron). No Talk lines in the C++
-- AI.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnEnterCombat arms
-- dark strike 19777 10s then {15s,18s}, non-triggered self-cast /
-- demoralizing shout 19778 15s then {15s,20s}, non-triggered
-- DoCastVictim / inspire 19779 13s then {20s,26s}, non-triggered
-- self-cast (the random-friendly-missing-buff arm has no bridge —
-- see deviations) / knockdown 19780 6s then {12s,15s}, non-triggered
-- DoCastVictim / flamespear 19781 2s then {12s,16s}, non-triggered
-- on a random alive player in the instance (wushoolay
-- randomAlivePlayer helper; the C++ SelectTarget(Random) arm takes
-- any alive target — C++-exact here). Nil-victim ticks cast nothing
-- but keep the schedule (jeklik convention). OnDied(4)/
-- OnLeaveCombat(2)/OnReset(23): cancel timers.
-- The npc_flamewaker_priest AI in the same C++ file (heal 19775 on
-- the lowest-HP friendly in 60 yd {15s,30s} then {15s,20s} / shadow
-- word: pain 19776 2s then {18s,26s}, non-triggered on a random alive
-- target without the aura / immolate 20294 8s then {15s,25s},
-- non-triggered on a random alive target without the aura) is not
-- registered — no NPC_FLAMEWAKER_PRIEST constant exists in the C++
-- tree and the creature_template ScriptName binding is DB-side, so
-- the add entry cannot be verified from the C++ sources alone (garr
-- firesworn convention); the lowest-HP-friendly pick and the
-- no-aura target filters would additionally need bridges.
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING queue gate
-- and the post-event casting check have no UNIT_STATE bridge —
-- timers fire unconditionally (jeklik convention); no instance-
-- script model — boss admission via the luaBossAI shim
-- (BossAI::JustEngagedWith and the BOSS_SULFURON_HARBINGER
-- bookkeeping arms skipped); the DoFindFriendlyMissingBuff(45.0f,
-- SPELL_INSPIRE) arm has no friendly-creature bridge — only the
-- DoCast(me, SPELL_INSPIRE) arm is kept.

local ENTRY_SULFURON_HARBINGER = 12098

local SPELL_DARK_STRIKE = 19777
local SPELL_DEMORALIZING_SHOUT = 19778
local SPELL_INSPIRE = 19779
local SPELL_KNOCKDOWN = 19780
local SPELL_FLAMESPEAR = 19781

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

-- C++ EVENT_DARK_STRIKE: non-triggered DoCast(me, 19777);
-- re-arm {15s,18s}.
local function onDarkStrike(creature, guid)
    creature:CastSpell(creature, SPELL_DARK_STRIKE)
    schedule(guid, "darkstrike", math.random(15000, 18000), function()
        onDarkStrike(creature, guid)
    end)
end

-- C++ EVENT_DEMORALIZING_SHOUT: non-triggered DoCastVictim(19778);
-- re-arm {15s,20s}.
local function onDemoralizingShout(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_DEMORALIZING_SHOUT)
    end
    schedule(guid, "demoralizingshout", math.random(15000, 20000), function()
        onDemoralizingShout(creature, guid)
    end)
end

-- C++ EVENT_INSPIRE: DoCast(me, 19779) — the
-- DoFindFriendlyMissingBuff arm has no bridge, so only the self-cast
-- is kept; re-arm {20s,26s}.
local function onInspire(creature, guid)
    creature:CastSpell(creature, SPELL_INSPIRE)
    schedule(guid, "inspire", math.random(20000, 26000), function()
        onInspire(creature, guid)
    end)
end

-- C++ EVENT_KNOCKDOWN: non-triggered DoCastVictim(19780);
-- re-arm {12s,15s}.
local function onKnockdown(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_KNOCKDOWN)
    end
    schedule(guid, "knockdown", math.random(12000, 15000), function()
        onKnockdown(creature, guid)
    end)
end

-- C++ EVENT_FLAMESPEAR: non-triggered DoCast on a random alive
-- target (SelectTarget(Random) — any alive target); re-arm {12s,16s}.
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

local function onFlamespear(creature, guid)
    local pick = randomAlivePlayer(creature)
    if pick then
        creature:CastSpell(pick, SPELL_FLAMESPEAR)
    end
    schedule(guid, "flamespear", math.random(12000, 16000), function()
        onFlamespear(creature, guid)
    end)
end

local function sulfuronEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "darkstrike", 10000, function()
        onDarkStrike(creature, guid)
    end)
    schedule(guid, "demoralizingshout", 15000, function()
        onDemoralizingShout(creature, guid)
    end)
    schedule(guid, "inspire", 13000, function()
        onInspire(creature, guid)
    end)
    schedule(guid, "knockdown", 6000, function()
        onKnockdown(creature, guid)
    end)
    schedule(guid, "flamespear", 2000, function()
        onFlamespear(creature, guid)
    end)
end

local function sulfuronResetState(guid)
    cancelTimers(guid)
end

local function sulfuronLeaveCombat(event, creature)
    sulfuronResetState(creature:GetGUID())
end

local function sulfuronDied(event, creature, killer)
    sulfuronResetState(creature:GetGUID())
end

local function sulfuronReset(event, creature)
    sulfuronResetState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_SULFURON_HARBINGER, 1, sulfuronEnterCombat)
RegisterCreatureEvent(ENTRY_SULFURON_HARBINGER, 2, sulfuronLeaveCombat)
RegisterCreatureEvent(ENTRY_SULFURON_HARBINGER, 4, sulfuronDied)
RegisterCreatureEvent(ENTRY_SULFURON_HARBINGER, 23, sulfuronReset)
