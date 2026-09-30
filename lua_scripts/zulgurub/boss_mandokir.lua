-- Bloodlord Mandokir (Zul'Gurub) — Lua port of
-- src/server/scripts/EasternKingdoms/ZulGurub/boss_mandokir.cpp
-- (boss_mandokirAI + npc_ohganAI + npc_vilebranch_speakerAI);
-- zulgurub.h:36 (DATA_MANDOKIR = 6, optional boss), :60
-- (NPC_MANDOKIR = 11382), :61 (NPC_OHGAN = 14988), :62
-- (NPC_VILEBRANCH_SPEAKER = 11391), :63 (NPC_CHAINED_SPIRT = 15117),
-- :37 (DATA_JINDO = 7), :44 (DATA_VILEBRANCH_SPEAKER = 13).
-- Creature entries: 11382 Bloodlord Mandokir (C++ ScriptName
-- "boss_mandokir" per AddSC_boss_mandokir; zulgurub.h NPC_MANDOKIR);
-- 14988 Ohgan ("npc_ohgan"); 11391 Vilebranch Speaker
-- ("npc_vilebranch_speaker"). The 20 chained spirits 15117 have no C++
-- CreatureScript in this file and are not registered.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent; melee
-- is engine-driven in Go (creature combat tick), like C++.
-- Fight shape: enter combat -> Talk(SAY_AGGRO=0), arm overpower 24407
-- {7s,9s} then {6s,12s} on the victim (triggered DoCastVictim,
-- C++-exact) / mortal strike 16856 {12s,18s} then {12s,18s} on the
-- victim, cast only when the victim is below 50% health (triggered
-- DoCastVictim with the HealthBelowPct(50) gate, C++-exact) /
-- whirlwind 13736 {24s,30s} then {22s,26s} self-cast (non-triggered
-- DoCast(me), C++-exact) / watch 24314 {13s,15s} then {12s,15s} on a
-- random alive player within 100 yd (Talk(SAY_WATCH=2) at the player,
-- C++-exact) / charge 24408 {33s,38s} then {22s,30s} on a random
-- alive player within 40 yd (triggered DoCast, C++-exact). Re-arms are
-- unconditional (nil-victim/nil-target ticks cast nothing but keep the
-- schedule, jeklik convention). Every third player kill: Talk
-- (SAY_DING_KILL=1), triggered self-cast 24312 (level up), kill count
-- reset. Ohgan (14988): sunder armor 24317 5s after Reset then
-- {10s,15s} on the victim (triggered DoCastVictim, C++-exact); never
-- spawns in this build (no summon model) but is registered so any that
-- exist fight correctly. Vilebranch Speaker (11391): demoralizing shout
-- 13730 {2s,4s} after Reset then {22s,30s} self-cast (non-triggered
-- DoCast(me), C++-exact) / cleave 15284 {5s,8s} after Reset then
-- {6s,9s} on the victim (triggered DoCastVictim, C++-exact).
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING queue gate has
-- no UNIT_STATE bridge — timers fire unconditionally (jeklik
-- convention); no instance-script model — boss admission via the
-- luaBossAI shim (_Reset/_JustDied/BossAI::JustEngagedWith, the
-- GetZulGurubAI bookkeeping, and the DATA_MANDOKIR encounter-state
-- arms skipped), so the pre-combat intro machine (Reset's
-- SetImmuneToAll(true) + EVENT_CHECK_START 1s poll of
-- DATA_MANDOKIR == SPECIAL from the speaker's death, MovePoint to
-- PosMandokir[1], EVENT_STARTED -> SetImmuneToAll(false) +
-- MovePath(PATH_MANDOKIR), MovementInform's SetHomePosition +
-- MoveTargetedHome + SetBossState(NOT_STARTED) at POINT_MANDOKIR_END,
-- the JustReachedHome SetImmuneToAll(false) arm) has no bearer —
-- Mandokir simply starts combat already at the arena; the Mount
-- (MODEL_OHGAN_MOUNT = 15271) / Dismount arms have no bridge; the
-- Ohgan and chained-spirit summon arms have no bearer (no summon
-- model) — neither ever spawns; SummonedCreatureDies' Ohgan frenzy
-- 24318 + Talk(SAY_OHGAN_DEAD=4) arm therefore never fires; the
-- KilledUnit jindo Talk(SAY_GRATS_JINDO) arm has no bearer (no
-- instance-script model); the mortal strike HealthBelowPct(50) check
-- uses post-damage GetHealthPct (no pre-damage health bridge);
-- spell_threatening_gaze (the 24314 -> 24315/24316 AuraScript OnRemove
-- chain) is an AuraScript — not modeled (standing gap); the speaker's
-- JustDied SetBossState(DATA_MANDOKIR, SPECIAL) arm has no bearer (no
-- instance-script model).

local ENTRY_MANDOKIR = 11382
local ENTRY_OHGAN = 14988
local ENTRY_SPEAKER = 11391

local SPELL_OVERPOWER = 24407
local SPELL_MORTAL_STRIKE = 16856
local SPELL_WHIRLWIND = 13736
local SPELL_WATCH = 24314
local SPELL_CHARGE = 24408
local SPELL_LEVEL_UP = 24312
local SPELL_SUNDERARMOR = 24317
local SPELL_DEMORALIZING_SHOUT = 13730
local SPELL_CLEAVE = 15284

local SAY_AGGRO = 0
local SAY_DING_KILL = 1
local SAY_WATCH = 2

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

local function alivePlayersInInstance(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            found[#found + 1] = p
        end
    end
    return found
end

-- C++ SelectTarget(Random, 0, range, true): random alive player in
-- range, nil when none qualify.
local function randomPlayerInRange(creature, range)
    local candidates = {}
    for _, p in ipairs(alivePlayersInInstance(creature)) do
        if creature:GetDistance(p) <= range then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

local function mandokirState(guid)
    local st = state[guid]
    if not st then
        st = { killCount = 0 }
        state[guid] = st
    end
    return st
end

-- C++ EVENT_OVERPOWER: triggered DoCastVictim(24407); re-arm {6s,12s}.
local function onOverpower(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_OVERPOWER, true)
    end
    schedule(guid, "overpower", math.random(6000, 12000), function()
        onOverpower(creature, guid)
    end)
end

-- C++ EVENT_MORTAL_STRIKE: victim below 50% health -> triggered
-- DoCastVictim(16856); re-arm {12s,18s} regardless (re-arm is outside
-- the health gate, C++-exact).
local function onMortalStrike(creature, guid)
    local victim = creature:GetVictim()
    if victim and victim:GetHealthPct() < 50 then
        creature:CastSpell(victim, SPELL_MORTAL_STRIKE, true)
    end
    schedule(guid, "mortalstrike", math.random(12000, 18000), function()
        onMortalStrike(creature, guid)
    end)
end

-- C++ EVENT_WHIRLWIND: non-triggered DoCast(me, 13736); re-arm
-- {22s,26s}.
local function onWhirlwind(creature, guid)
    creature:CastSpell(creature, SPELL_WHIRLWIND)
    schedule(guid, "whirlwind", math.random(22000, 26000), function()
        onWhirlwind(creature, guid)
    end)
end

-- C++ EVENT_WATCH_PLAYER: random 100-yd player -> DoCast(player,
-- SPELL_WATCH) + Talk(SAY_WATCH, player); re-arm {12s,15s}.
local function onWatchPlayer(creature, guid)
    local target = randomPlayerInRange(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_WATCH)
        creature:Talk(SAY_WATCH)
    end
    schedule(guid, "watch", math.random(12000, 15000), function()
        onWatchPlayer(creature, guid)
    end)
end

-- C++ EVENT_CHARGE_PLAYER: triggered DoCast(SelectTarget(Random, 0,
-- 40.0f, true), SPELL_CHARGE); re-arm {22s,30s}. Nil-target ticks cast
-- nothing but keep the schedule.
local function onChargePlayer(creature, guid)
    local target = randomPlayerInRange(creature, 40)
    if target then
        creature:CastSpell(target, SPELL_CHARGE, true)
    end
    schedule(guid, "charge", math.random(22000, 30000), function()
        onChargePlayer(creature, guid)
    end)
end

local function mandokirResetState(guid)
    cancelTimers(guid)
    state[guid] = nil
end

local function mandokirEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "overpower", math.random(7000, 9000), function()
        onOverpower(creature, guid)
    end)
    schedule(guid, "mortalstrike", math.random(12000, 18000), function()
        onMortalStrike(creature, guid)
    end)
    schedule(guid, "whirlwind", math.random(24000, 30000), function()
        onWhirlwind(creature, guid)
    end)
    schedule(guid, "watch", math.random(13000, 15000), function()
        onWatchPlayer(creature, guid)
    end)
    schedule(guid, "charge", math.random(33000, 38000), function()
        onChargePlayer(creature, guid)
    end)
end

-- C++ KilledUnit: player victims only; every third kill -> Talk
-- (SAY_DING_KILL), jindo congrats (no instance-script bridge, skipped),
-- triggered self-cast 24312, count reset.
local function mandokirTargetDied(event, creature, victim)
    if victim:GetObjectType() ~= "Player" then
        return
    end
    local st = mandokirState(creature:GetGUID())
    st.killCount = st.killCount + 1
    if st.killCount >= 3 then
        creature:Talk(SAY_DING_KILL)
        creature:CastSpell(creature, SPELL_LEVEL_UP, true)
        st.killCount = 0
    end
end

local function mandokirLeaveCombat(event, creature)
    mandokirResetState(creature:GetGUID())
end

local function mandokirDied(event, creature, killer)
    mandokirResetState(creature:GetGUID())
end

local function mandokirReset(event, creature)
    -- C++ Reset's Z>140 immunity/intro/summon arms have no bridge (no
    -- UNIT_STATE/immune flag, movement, instance-script, or summon
    -- model) — only the timer cancel is modeled.
    mandokirResetState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_MANDOKIR, 1, mandokirEnterCombat)
RegisterCreatureEvent(ENTRY_MANDOKIR, 2, mandokirLeaveCombat)
RegisterCreatureEvent(ENTRY_MANDOKIR, 3, mandokirTargetDied)
RegisterCreatureEvent(ENTRY_MANDOKIR, 4, mandokirDied)
RegisterCreatureEvent(ENTRY_MANDOKIR, 23, mandokirReset)

-- Ohgan (npc_ohgan, C++ ScriptName for 14988): sunder armor 24317 5s
-- after Reset then {10s,15s} on the victim (triggered DoCastVictim,
-- C++-exact); melee is engine-driven. Never spawns in this build (no
-- summon model), but the AI is registered so any that exist fight
-- correctly.
local function onSunderArmor(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SUNDERARMOR, true)
    end
    schedule(guid, "sunder", math.random(10000, 15000), function()
        onSunderArmor(creature, guid)
    end)
end

local function ohganEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "sunder", 5000, function()
        onSunderArmor(creature, guid)
    end)
end

RegisterCreatureEvent(ENTRY_OHGAN, 1, ohganEnterCombat)
RegisterCreatureEvent(ENTRY_OHGAN, 2, function(event, creature)
    cancelTimers(creature:GetGUID())
end)
RegisterCreatureEvent(ENTRY_OHGAN, 4, function(event, creature, killer)
    cancelTimers(creature:GetGUID())
end)
RegisterCreatureEvent(ENTRY_OHGAN, 23, function(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "sunder", 5000, function()
        onSunderArmor(creature, guid)
    end)
end)

-- Vilebranch Speaker (npc_vilebranch_speaker, C++ ScriptName for
-- 11391): demoralizing shout 13730 {2s,4s} after Reset then {22s,30s}
-- self-cast (non-triggered DoCast(me), C++-exact) / cleave 15284 {5s,8s}
-- after Reset then {6s,9s} on the victim (triggered DoCastVictim,
-- C++-exact); melee is engine-driven. The JustDied
-- SetBossState(DATA_MANDOKIR, SPECIAL) arm has no bearer (no
-- instance-script model) — not modeled.
local function onDemoralizingShout(creature, guid)
    creature:CastSpell(creature, SPELL_DEMORALIZING_SHOUT)
    schedule(guid, "shout", math.random(22000, 30000), function()
        onDemoralizingShout(creature, guid)
    end)
end

local function onSpeakerCleave(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CLEAVE, true)
    end
    schedule(guid, "cleave", math.random(6000, 9000), function()
        onSpeakerCleave(creature, guid)
    end)
end

local function speakerReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "shout", math.random(2000, 4000), function()
        onDemoralizingShout(creature, guid)
    end)
    schedule(guid, "cleave", math.random(5000, 8000), function()
        onSpeakerCleave(creature, guid)
    end)
end

RegisterCreatureEvent(ENTRY_SPEAKER, 1, function(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "shout", math.random(22000, 30000), function()
        onDemoralizingShout(creature, guid)
    end)
    schedule(guid, "cleave", math.random(6000, 9000), function()
        onSpeakerCleave(creature, guid)
    end)
end)
RegisterCreatureEvent(ENTRY_SPEAKER, 2, function(event, creature)
    cancelTimers(creature:GetGUID())
end)
RegisterCreatureEvent(ENTRY_SPEAKER, 4, function(event, creature, killer)
    cancelTimers(creature:GetGUID())
end)
RegisterCreatureEvent(ENTRY_SPEAKER, 23, speakerReset)
