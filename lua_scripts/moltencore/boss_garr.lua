-- Garr (Molten Core) — Lua port of
-- src/server/scripts/EasternKingdoms/BlackrockMountain/MoltenCore/
-- boss_garr.cpp (boss_garr AI only); molten_core.h:33
-- (BOSS_GARR = 3, fourth boss), :57 (NPC_GARR = 12057).
-- Creature entry: 12057 Garr (C++ ScriptName "boss_garr" per
-- AddSC_boss_garr). No Talk lines in the C++ AI.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnEnterCombat arms
-- antimagic pulse 19492 25s then {10s,15s}, non-triggered self-cast
-- (C++ DoCast default is triggered=false — vaelastrasz convention) /
-- magma shackles 19496 15s then {8s,12s}, non-triggered self-cast.
-- OnDied(4)/OnLeaveCombat(2)/OnReset(23): cancel timers.
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING queue gate and
-- the post-event casting check have no UNIT_STATE bridge — timers fire
-- unconditionally (jeklik convention); no instance-script model —
-- boss admission via the luaBossAI shim (BossAI::JustEngagedWith and
-- the BOSS_GARR bookkeeping arms skipped); the npc_firesworn AI in the
-- same file (immolate schedule 4s then {5s,10s} on a random target,
-- separation-anxiety check vs NPC_GARR 20 yd, eruption+despawn below
-- 10% health) is not registered — no NPC_FIRESWORN constant exists in
-- the C++ tree and the creature_template ScriptName binding is DB-side,
-- so the add entry cannot be verified from the C++ sources alone; the
-- eruption damage-zeroing arm would need a pre-damage hook plus a
-- despawn bridge (no despawn bridge, razorgore convention).

local ENTRY_GARR = 12057

local SPELL_ANTIMAGIC_PULSE = 19492
local SPELL_MAGMA_SHACKLES = 19496

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

-- C++ EVENT_ANTIMAGIC_PULSE: non-triggered DoCast(me, 19492);
-- re-arm {10s,15s}.
local function onAntimagicPulse(creature, guid)
    creature:CastSpell(creature, SPELL_ANTIMAGIC_PULSE)
    schedule(guid, "antimagicpulse", math.random(10000, 15000), function()
        onAntimagicPulse(creature, guid)
    end)
end

-- C++ EVENT_MAGMA_SHACKLES: non-triggered DoCast(me, 19496);
-- re-arm {8s,12s}.
local function onMagmaShackles(creature, guid)
    creature:CastSpell(creature, SPELL_MAGMA_SHACKLES)
    schedule(guid, "magmashackles", math.random(8000, 12000), function()
        onMagmaShackles(creature, guid)
    end)
end

local function garrEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "antimagicpulse", 25000, function()
        onAntimagicPulse(creature, guid)
    end)
    schedule(guid, "magmashackles", 15000, function()
        onMagmaShackles(creature, guid)
    end)
end

local function garrResetState(guid)
    cancelTimers(guid)
end

local function garrLeaveCombat(event, creature)
    garrResetState(creature:GetGUID())
end

local function garrDied(event, creature, killer)
    garrResetState(creature:GetGUID())
end

local function garrReset(event, creature)
    garrResetState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_GARR, 1, garrEnterCombat)
RegisterCreatureEvent(ENTRY_GARR, 2, garrLeaveCombat)
RegisterCreatureEvent(ENTRY_GARR, 4, garrDied)
RegisterCreatureEvent(ENTRY_GARR, 23, garrReset)
