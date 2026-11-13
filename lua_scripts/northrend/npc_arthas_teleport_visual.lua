-- Arthas teleport visual (Icecrown Citadel) — Lua port of
-- npc_arthas_teleport_visual
-- (src/server/scripts/Northrend/IcecrownCitadel/icecrown_citadel.cpp:560-609).
-- The AI binds (C++ GetAI :600-607) only to a creature within 100yd
-- (2D) of the spire center (4357.052, 2769.421) with home Z < 315:
-- the NPC_INVISIBLE_STALKER 30298 (icecrown_citadel.h:335) at the
-- Frozen Throne teleporter. Entry 30298 is the generic invisible
-- stalker (also the Gundrak altar trigger), so the gate below is the
-- GetAI twin — every other 30298 spawn fails it and no-ops.
-- Reset (:571-577): events reset; if Putricide + Blood-Queen +
-- Sindragosa are all DONE, schedule the soul-missile leg at {1,6}s.
-- UpdateAI (:580-594): DoCastAOE(SPELL_SOUL_MISSILE 72585), re-arm
-- {5,7}s; empty event map = no-op.
-- DORMANT: 72585 has no seed Spell.dbc row (tools/spelldbc_seed/
-- main.go) — the cast no-ops until the row ships (needs the same
-- wago.tools read queue as 66545's SpellRadius 28). The gate +
-- scheduling are live now.
-- DoCastAOE twin: creature:CastSpell(creature, id) (boss_ai.go
-- convention). At Reset the creature is at home (spawn / evade
-- return), so the home position carries both gate checks.

local NPC_INVISIBLE_STALKER = 30298 -- icecrown_citadel.h:335
local SPELL_SOUL_MISSILE = 72585 -- icecrown_citadel.cpp:91
local DATA_PROFESSOR_PUTRICIDE = 6 -- icecrown_citadel.h:80
local DATA_BLOOD_QUEEN_LANA_THEL = 8 -- icecrown_citadel.h:82
local DATA_SINDRAGOSA = 11 -- icecrown_citadel.h:85
local BOSS_STATE_DONE = 3 -- InstanceScript.h:68-76
local SPIRE_X = 4357.052
local SPIRE_Y = 2769.421

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

local function soulMissileTick(creature, guid)
    creature:CastSpell(creature, SPELL_SOUL_MISSILE)
    schedule(guid, "soul_missile", {5000, 7000}, function()
        soulMissileTick(creature, guid)
    end)
end

local function teleportVisualReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    local hx, hy, hz = creature:GetHomePosition()
    if hx == nil or hz == nil then
        return
    end
    local dx, dy = hx - SPIRE_X, hy - SPIRE_Y
    if math.sqrt(dx * dx + dy * dy) >= 100.0 or hz >= 315.0 then
        return
    end
    if creature:GetBossState(DATA_PROFESSOR_PUTRICIDE) == BOSS_STATE_DONE
        and creature:GetBossState(DATA_BLOOD_QUEEN_LANA_THEL) == BOSS_STATE_DONE
        and creature:GetBossState(DATA_SINDRAGOSA) == BOSS_STATE_DONE then
        schedule(guid, "soul_missile", {1000, 6000}, function()
            soulMissileTick(creature, guid)
        end)
    end
end

local function teleportVisualCancel(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

RegisterCreatureEvent(NPC_INVISIBLE_STALKER, 2, teleportVisualCancel)
RegisterCreatureEvent(NPC_INVISIBLE_STALKER, 4, teleportVisualCancel)
RegisterCreatureEvent(NPC_INVISIBLE_STALKER, 23, teleportVisualReset)
