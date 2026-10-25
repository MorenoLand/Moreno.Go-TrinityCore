-- Temple of Ahn'Qiraj: Fankriss the Unyielding — Lua port of
-- src/server/scripts/Kalimdor/TempleOfAhnQiraj/boss_fankriss.cpp
-- (boss_fankrissAI : public BossAI(creature, DATA_FRANKRIS);
-- GetAI via GetAQ40AI<boss_fankrissAI>(creature) (AQ40ScriptName
-- "instance_temple_of_ahnqiraj" gate); AddSC_boss_fankriss at end
-- registers the boss script; kalimdor loader decl 89 / call 194 per
-- kalimdor_script_loader.cpp — third "// Temple of ahn'qiraj" loader
-- group, after cthun and viscidus, before huhuran).
-- Whole-server-tree grep confirms the cpp as the sole source of
-- "boss_fankriss" (loader decl/call lines only otherwise).
-- Entry: Fankriss the Unyielding = 15510 (armory npc=15510;
-- creature_template ScriptName binding stays DB-side). No
-- NPC_FANKRISS constant in temple_of_ahnqiraj.h.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- The C++ diff-timer + UNIT_STATE_CASTING UpdateAI loop has no
-- casting-state bridge in the Lua API (aeonus precedent — unmodeled);
-- GetThreat gating in the hatchling leg is instance-side and unwired.
-- Ported arms (the self-contained in-combat legs):
-- - Mortal Wound 28467 on the victim (C++ DoCastVictim, non-triggered),
--   init urand(10000,15000) -> urand(10000,20000).
-- - SpawnHatchlings_Timer (init urand(6000,12000) -> urand(45000,60000),
--   gated on HealthAbovePct(3), PORTED 2026-10-23 — position-leg axis):
--   random alive player (SelectTarget Random, 0, 0.0f, playerOnly TRUE,
--   no range cap) -> DoCast(target, SPELL_ROOT 28858) non-triggered ->
--   DoTeleportPlayer(target) to one of the three tunnel coords (player:
--   TeleportNear) -> 4x SummonCreature(15962 at the target's live
--   post-teleport position -3/-3, -3/+3, -5/-5, -5/+5 z,
--   TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT = bridge type 3, 15s, o=0 ->
--   summoner-orientation fallback), C++-verbatim offsets and
--   tunnel coords (boss_fankriss.cpp:139-206). The summoned hatchlings'
--   AttackStart(target) has no bridge (documented gap — they idle until
--   the engine engages them), and ModifyThreatByPercent(target, -100)
--   is PORTED 2026-10-24 (gate + cut after the ROOT cast). Re-arm {45000,60000} is
--   unconditional on fire (C++ sets it inside the <=diff branch even
--   with no target); the health gate drops the key when below 3%
--   (C++ outer HealthAbovePct(3) arm).
-- Unmodeled (documented-only, no bridges):
-- - SpawnSpawns_Timer (urand 15000-45000 init -> urand 30000-60000):
--   SummonSpawn(random target via SelectTarget Random) 1-3 times —
--   DoSpawnCreature(15630, offset coords, TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT)
--   has zero Lua usage (cthun precedent — no bridge); random player
--   selection goes through randomThreatPlayer above.
-- - SpawnHatchlings_Timer (urand 6000-12000 init -> urand 45000-60000,
--   gated by HealthAbovePct(3)): PORTED 2026-10-23 (onSpawnHatchlings
--   above; stale "SummonCreature zero Lua usage" claims retracted — the
--   summon bridge is live, jeklik/mandokir precedent).
-- - SOUND_SENTENCE_YOU/LAWS/TRESPASS/WILL_BE/SERVE_TO 8588-8592:
--   zero Talk() calls in C++ ("sound not implemented").
-- - BossAI ctor leg DATA_FRANKRIS (temple_of_ahnqiraj.h:32 = 2) and
--   _Reset() — instance bridge, standing.
local ENTRY = 15510
local ENTRY_HATCHLING = 15962

local SPELL_MORTAL_WOUND = 28467
local SPELL_ROOT = 28858

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

-- C++ MortalWound_Timer: DoCastVictim(28467), non-triggered,
-- init urand(10000,15000) -> urand(10000,20000).
local function onMortalWound(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_MORTAL_WOUND)
    end
    schedule(guid, "mortalwound", math.random(10000, 20000), function()
        onMortalWound(creature, guid)
    end)
end

-- C++ SpawnHatchlings_Timer (boss_fankriss.cpp:139-206): gated on
-- HealthAbovePct(3); on fire pick a random alive threat-list player (SelectTarget
-- Random, 0, 0.0f, playerOnly TRUE — no range cap), DoCast(target,
-- SPELL_ROOT 28858) non-triggered, ModifyThreatByPercent(target, -100)
-- (PORTED 2026-10-24), DoTeleportPlayer(target) to one
-- of three tunnel coords, then 4x SummonCreature(15962) at the target's
-- LIVE post-teleport position with the C++-verbatim offsets
-- (-3,-3 / -3,+3 / -5,-5 / -5,+5, z = target z), o = 0 (bridge falls
-- back to the summoner's orientation, Object.cpp:2061-2062 twin),
-- TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT (bridge type 3), 15s =
-- 15000ms. The hatchlings' AttackStart(target) has no bridge
-- (documented gap — they idle until the engine engages them).
-- Re-arm {45000,60000} unconditionally on fire; below-3% health drops
-- the key (C++ outer HealthAbovePct arm).
local TUNNEL_SPOTS = {
    { -8106.0142, 1289.2900, -74.419533, 5.112 },
    { -7990.135354, 1155.1907, -78.849319, 2.608 },
    { -8159.7753, 1127.9064, -76.868660, 0.675 },
}

local function randomThreatPlayer(creature)
    local targets = creature:GetAITargets()
    local out = {}
    for _, unit in ipairs(targets) do
        if unit:GetObjectType() == "Player" and not unit:IsDead() then
            out[#out + 1] = unit
        end
    end
    if #out == 0 then
        return nil
    end
    return out[math.random(#out)]
end

local function onSpawnHatchlings(creature, guid)
    if creature:GetHealthPct() > 3 then
        local target = randomThreatPlayer(creature)
        if target then
            creature:CastSpell(target, SPELL_ROOT)
            if creature:GetThreat(target) ~= 0 then
                creature:ModifyThreatByPercent(target, -100)
            end
            local spot = TUNNEL_SPOTS[math.random(#TUNNEL_SPOTS)]
            target:TeleportNear(spot[1], spot[2], spot[3], spot[4])
            local tx, ty, tz = target:GetX(), target:GetY(), target:GetZ()
            for _, off in ipairs({ { -3, -3 }, { -3, 3 }, { -5, -5 }, { -5, 5 } }) do
                creature:SummonCreature(ENTRY_HATCHLING, tx + off[1],
                    ty + off[2], tz, 0, 3, 15000)
            end
        end
        schedule(guid, "hatchlings", math.random(45000, 60000), function()
            onSpawnHatchlings(creature, guid)
        end)
    end
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "mortalwound", math.random(10000, 15000), function()
        onMortalWound(creature, guid)
    end)
    schedule(guid, "hatchlings", math.random(6000, 12000), function()
        onSpawnHatchlings(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ has no JustDied override for boss_fankrissAI — cancel only.
local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)
