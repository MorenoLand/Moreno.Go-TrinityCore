-- Slad'ran (Gundrak) — Lua port of
-- src/server/scripts/Northrend/Gundrak/boss_slad_ran.cpp
-- (boss_slad_ran + npc_slad_ran_constrictor + npc_slad_ran_viper).
-- Gundrak dungeon-script unit per northrend_script_loader.cpp order
-- (call 215, Gundrak block start; next: boss_drakkari_colossus).
-- Entries: 29304 Slad'ran (gundrak.h NPC_SLAD_RAN — the
-- RegisterCreatureAIWithFactory(GetGundrakAI) ScriptName bindings are
-- instance-shimmed, the creature_template bindings DB-side as usual);
-- 29713 Slad'ran Constrictor (C++ CREATURE_CONSTRICTORS, summoned);
-- 29680 Slad'ran Viper (C++ CREATURE_SNAKE, summoned).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent;
-- melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. C++ DoCast default is triggered=false
-- (vaelastrasz convention): creature:CastSpell(nil, spell) =
-- DoCastVictim, creature:CastSpell(target, spell) = DoCast.
-- Ported arms (C++-exact for all modeled arms):
-- boss_slad_ran: JustEngagedWith Talk(SAY_AGGRO 0); ScheduleTasks —
-- Poison Nova 55081 (victim, 10s init, 15s repeat) + Talk(EMOTE_NOVA
-- 5) on the same tick, Powerful Bite 48287 (victim, 3s init, 10s
-- repeat), Venom Bolt 54970 (victim, 15s init, 10s repeat); KilledUnit
-- Talk(SAY_SLAY 1) player-gated (event 3, victim:GetObjectType() ==
-- "Player" — nalorakk precedent); JustDied Talk(EMOTE_ACTIVATE_ALTAR
-- 6) + Talk(SAY_DEATH 2) (event 4).
-- npc_slad_ran_constrictor: JustEngagedWith — Grip of Slad'ran 55093
-- on the victim (2s init, 3-6s repeat). The 5-stack -> Snake Wrap
-- 55126 machine is NOT modeled (see below).
-- npc_slad_ran_viper: JustEngagedWith — Venomous Bite 54987 (victim,
-- 2s init, 10s repeat).
-- Unmodeled (no bridges — documented, not wired):
-- boss DamageTaken phase machine — HealthBelowPct(30/25) has no
-- bridge (zero hits on the Lua surface — doomwalker precedent) and
-- the SummonCreature snake/constrictor waves (3 normal / 5 heroic
-- SpawnLoc entries, TEMPSUMMON_CORPSE_TIMED_DESPAWN) sit behind the
-- summon STRAND (standing), so the GROUP_SNAKES cancel and the
-- PHASE_SNAKES -> PHASE_CONSTRICTORS transition never fire.
-- JustSummoned's MovePoint-to-boss choreography is movement-model
-- only and has no summons to move; summons.Summon bookkeeping has
-- no Go bearer. SetGUID(DATA_SNAKES_WHYD_IT_HAVE_TO_BE_SNAKES) /
-- WasWrapped have no cross-AI SetGUID bridge (zero hits —
-- netherstorm precedent). achievement_snakes_whyd_it_have_to_be_snakes
-- (AchievementCriteriaScript, CAST_AI cross-AI OnCheck) has no
-- achievement-criteria bridge on the Lua surface — documented-only.
-- Constrictor: the stack-count leg (GetAura(55093) stack check == 5),
-- RemoveAurasDueToSpell, the victim self-cast of SNAKE_WRAP 55126
-- (no victim CastSpell bridge), the TempSummon -> summoner SetGUID
-- cross-AI hop, and DespawnOrUnsummon all have no bridges
-- (terestian despawn precedent) — so only the Grip cast rides the
-- timer; the wrap/achievement-fail payload is documented-only.
-- Viper Reset cancels its scheduler; the boss Reset (BossAI _Reset +
-- _wrappedPlayers.clear) lands as per-GUID timer/state drop on
-- OnLeaveCombat(2)/OnReset(23), gargolmar precedent.

local ENTRY_SLAD_RAN = 29304
local ENTRY_CONSTRICTOR = 29713
local ENTRY_VIPER = 29680

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2
local SAY_SUMMON_SNAKES = 3
local SAY_SUMMON_CONSTRICTORS = 4
local EMOTE_NOVA = 5
local EMOTE_ACTIVATE_ALTAR = 6

local SPELL_POISON_NOVA = 55081
local SPELL_POWERFULL_BITE = 48287
local SPELL_VENOM_BOLT = 54970
local SPELL_GRIP_OF_SLAD_RAN = 55093
local SPELL_SNAKE_WRAP = 55126
local SPELL_VENOMOUS_BITE = 54987

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

-- C++ ScheduleTasks: Poison Nova 55081 10s -> 15s (+ Talk EMOTE_NOVA),
-- Powerful Bite 48287 3s -> 10s, Venom Bolt 54970 15s -> 10s — all
-- DoCastVictim. The DamageTaken phase summon machine is unmodeled
-- (see header), so the pump arms only the three combat timers.
local function novaTick(creature, guid)
    creature:CastSpell(nil, SPELL_POISON_NOVA)
    creature:Talk(EMOTE_NOVA)
    schedule(guid, "nova", 15000, function()
        novaTick(creature, guid)
    end)
end

local function biteTick(creature, guid)
    creature:CastSpell(nil, SPELL_POWERFULL_BITE)
    schedule(guid, "bite", 10000, function()
        biteTick(creature, guid)
    end)
end

local function venomBoltTick(creature, guid)
    creature:CastSpell(nil, SPELL_VENOM_BOLT)
    schedule(guid, "venombolt", 10000, function()
        venomBoltTick(creature, guid)
    end)
end

local function sladranEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "nova", 10000, function()
        novaTick(creature, guid)
    end)
    schedule(guid, "bite", 3000, function()
        biteTick(creature, guid)
    end)
    schedule(guid, "venombolt", 15000, function()
        venomBoltTick(creature, guid)
    end)
end

local function sladranLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function sladranTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

local function sladranDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(EMOTE_ACTIVATE_ALTAR)
    creature:Talk(SAY_DEATH)
end

local function sladranReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_SLAD_RAN, 1, sladranEnterCombat)
RegisterCreatureEvent(ENTRY_SLAD_RAN, 2, sladranLeaveCombat)
RegisterCreatureEvent(ENTRY_SLAD_RAN, 3, sladranTargetDied)
RegisterCreatureEvent(ENTRY_SLAD_RAN, 4, sladranDied)
RegisterCreatureEvent(ENTRY_SLAD_RAN, 23, sladranReset)

-- npc_slad_ran_constrictor (29713): Grip of Slad'ran 55093 on the
-- victim, 2s init, 3-6s repeat. The stack-5 -> RemoveAurasDueToSpell
-- -> victim self-cast SNAKE_WRAP 55126 -> boss SetGUID wrap-latch ->
-- DespawnOrUnsummon machine has no bridges (see header) — the cast
-- is the only wireable arm.
local function gripTick(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_GRIP_OF_SLAD_RAN)
    end
    schedule(guid, "grip", math.random(3000, 6000), function()
        gripTick(creature, guid)
    end)
end

local function constrictorEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "grip", 2000, function()
        gripTick(creature, guid)
    end)
end

local function constrictorLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function constrictorDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function constrictorReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_CONSTRICTOR, 1, constrictorEnterCombat)
RegisterCreatureEvent(ENTRY_CONSTRICTOR, 2, constrictorLeaveCombat)
RegisterCreatureEvent(ENTRY_CONSTRICTOR, 4, constrictorDied)
RegisterCreatureEvent(ENTRY_CONSTRICTOR, 23, constrictorReset)

-- npc_slad_ran_viper (29680): Venomous Bite 54987 on the victim, 2s
-- init, 10s repeat. Reset cancels the scheduler (C++-exact).
local function viperBiteTick(creature, guid)
    creature:CastSpell(nil, SPELL_VENOMOUS_BITE)
    schedule(guid, "bite", 10000, function()
        viperBiteTick(creature, guid)
    end)
end

local function viperEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "bite", 2000, function()
        viperBiteTick(creature, guid)
    end)
end

local function viperLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function viperDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function viperReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_VIPER, 1, viperEnterCombat)
RegisterCreatureEvent(ENTRY_VIPER, 2, viperLeaveCombat)
RegisterCreatureEvent(ENTRY_VIPER, 4, viperDied)
RegisterCreatureEvent(ENTRY_VIPER, 23, viperReset)
