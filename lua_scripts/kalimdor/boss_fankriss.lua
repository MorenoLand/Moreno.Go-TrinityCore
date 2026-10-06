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
-- Unmodeled (documented-only, no bridges):
-- - SpawnSpawns_Timer (urand 6000-12000 init -> urand 30000-60000):
--   SummonSpawn(random target via SelectTarget Random) 1-3 times —
--   DoSpawnCreature(15630, offset coords, TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT)
--   has zero Lua usage (cthun precedent — no bridge), and SelectTarget
--   Random has none either; random-player-via-GetPlayersInWorld is
--   unused without a working summon.
-- - SpawnHatchlings_Timer (urand 6000-12000 init -> urand 45000-60000,
--   gated by HealthAbovePct(3)): DoCast(target, SPELL_ROOT 28858) +
--   DoTeleportPlayer to one of three tunnel coords (no DoTeleportPlayer
--   bridge) + ModifyThreatByPercent(target, -100) (no threat bridge) +
--   me->SummonCreature(15962 x4, TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT
--   15s) with AttackStart(target) (SummonCreature zero Lua usage,
--   AttackStart cross-AI unbridgeable). Casting Root alone without the
--   teleport/summon legs would misrepresent the mechanic.
-- - SOUND_SENTENCE_YOU/LAWS/TRESPASS/WILL_BE/SERVE_TO 8588-8592:
--   zero Talk() calls in C++ ("sound not implemented").
-- - BossAI ctor leg DATA_FRANKRIS (temple_of_ahnqiraj.h:32 = 2) and
--   _Reset() — instance bridge, standing.
local ENTRY = 15510

local SPELL_MORTAL_WOUND = 28467

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

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "mortalwound", math.random(10000, 15000), function()
        onMortalWound(creature, guid)
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
