-- Shazzrah (Molten Core) — Lua port of
-- src/server/scripts/EasternKingdoms/BlackrockMountain/MoltenCore/
-- boss_shazzrah.cpp (boss_shazzrahAI only); molten_core.h:34
-- (BOSS_SHAZZRAH = 4, fifth boss), :58 (NPC_SHAZZRAH = 12264).
-- Creature entry: 12264 Shazzrah (C++ ScriptName "boss_shazzrah" per
-- AddSC_boss_shazzrah). No Talk lines in the C++ AI.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnEnterCombat arms
-- arcane explosion 19712 6s then {4s,7s}, non-triggered DoCastVictim /
-- Shazzrah's curse 19713 10s then {25s,30s}, non-triggered on a random
-- alive player in the instance (wushoolay randomAlivePlayer helper;
-- the C++ -SPELL_SHAZZRAH_CURSE no-aura filter has no HasAura bridge
-- — the pick may land on a target already carrying it) / magic
-- grounding 19714 24s then 35s, non-triggered self-cast / counterspell
-- 19715 15s then {16s,20s}, non-triggered DoCastVictim / Gate of
-- Shazzrah 45s then 45s (threat-reset and DoCastAOE(23138) arms have no
-- bridges — see deviations): schedules the triggered arcane explosion
-- 2s later (non-triggered DoCastVictim 19712, one-shot) and
-- reschedules arcane explosion {3s,6s}. Nil-victim ticks cast nothing
-- but keep the schedule (jeklik convention). OnDied(4)/
-- OnLeaveCombat(2)/OnReset(23): cancel timers.
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING queue gate and
-- the post-event casting check have no UNIT_STATE bridge — timers fire
-- unconditionally (jeklik convention); no instance-script model —
-- boss admission via the luaBossAI shim (BossAI::JustEngagedWith and
-- the BOSS_SHAZZRAH bookkeeping arms skipped); ResetThreatList has no
-- bridge (no threat model); the spell_shazzrah_gate_dummy SpellScript
-- (picks a random area target, that target casts 23139 teleporting
-- Shazzrah to it, then AttackStart on the target) has no SpellScript
-- bridge — the teleport never happens and the AttackStart has no
-- bearer (vaelastrasz convention) — the fight continues on the
-- existing victim instead of the gate target.

local ENTRY_SHAZZRAH = 12264

local SPELL_ARCANE_EXPLOSION = 19712
local SPELL_SHAZZRAH_CURSE = 19713
local SPELL_MAGIC_GROUNDING = 19714
local SPELL_COUNTERSPELL = 19715

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

-- C++ EVENT_ARCANE_EXPLOSION: non-triggered DoCastVictim(19712);
-- re-arm {4s,7s}.
local function onArcaneExplosion(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_ARCANE_EXPLOSION)
    end
    schedule(guid, "arcaneexplosion", math.random(4000, 7000), function()
        onArcaneExplosion(creature, guid)
    end)
end

-- C++ EVENT_SHAZZRAH_CURSE: non-triggered cast 19713 on a random alive
-- player in the instance; re-arm {25s,30s}.
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

local function onShazzrahCurse(creature, guid)
    local pick = randomAlivePlayer(creature)
    if pick then
        creature:CastSpell(pick, SPELL_SHAZZRAH_CURSE)
    end
    schedule(guid, "curse", math.random(25000, 30000), function()
        onShazzrahCurse(creature, guid)
    end)
end

-- C++ EVENT_MAGIC_GROUNDING: non-triggered DoCast(me, 19714);
-- re-arm 35s.
local function onMagicGrounding(creature, guid)
    creature:CastSpell(creature, SPELL_MAGIC_GROUNDING)
    schedule(guid, "magicgrounding", 35000, function()
        onMagicGrounding(creature, guid)
    end)
end

-- C++ EVENT_COUNTERSPELL: non-triggered DoCastVictim(19715);
-- re-arm {16s,20s}.
local function onCounterspell(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_COUNTERSPELL)
    end
    schedule(guid, "counterspell", math.random(16000, 20000), function()
        onCounterspell(creature, guid)
    end)
end

-- C++ EVENT_SHAZZRAH_GATE: 45s then 45s. The ResetThreatList and
-- DoCastAOE(SPELL_SHAZZRAH_GATE_DUMMY) arms have no bridges — the
-- timer keeps the explosion interplay: one-shot triggered arcane
-- explosion 2s later and arcane explosion rescheduled {3s,6s}.
local function onGateTriggeredExplosion(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_ARCANE_EXPLOSION)
    end
    timers[guid]["gateexplosion"] = nil
end

local function onShazzrahGate(creature, guid)
    schedule(guid, "gateexplosion", 2000, function()
        onGateTriggeredExplosion(creature, guid)
    end)
    schedule(guid, "arcaneexplosion", math.random(3000, 6000), function()
        onArcaneExplosion(creature, guid)
    end)
    schedule(guid, "gate", 45000, function()
        onShazzrahGate(creature, guid)
    end)
end

local function shazzrahEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "arcaneexplosion", 6000, function()
        onArcaneExplosion(creature, guid)
    end)
    schedule(guid, "curse", 10000, function()
        onShazzrahCurse(creature, guid)
    end)
    schedule(guid, "magicgrounding", 24000, function()
        onMagicGrounding(creature, guid)
    end)
    schedule(guid, "counterspell", 15000, function()
        onCounterspell(creature, guid)
    end)
    schedule(guid, "gate", 45000, function()
        onShazzrahGate(creature, guid)
    end)
end

local function shazzrahResetState(guid)
    cancelTimers(guid)
end

local function shazzrahLeaveCombat(event, creature)
    shazzrahResetState(creature:GetGUID())
end

local function shazzrahDied(event, creature, killer)
    shazzrahResetState(creature:GetGUID())
end

local function shazzrahReset(event, creature)
    shazzrahResetState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_SHAZZRAH, 1, shazzrahEnterCombat)
RegisterCreatureEvent(ENTRY_SHAZZRAH, 2, shazzrahLeaveCombat)
RegisterCreatureEvent(ENTRY_SHAZZRAH, 4, shazzrahDied)
RegisterCreatureEvent(ENTRY_SHAZZRAH, 23, shazzrahReset)
