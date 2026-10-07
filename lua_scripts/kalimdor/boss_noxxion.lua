-- Maraudon: Noxxion — Lua port of
-- src/server/scripts/Kalimdor/Maraudon/boss_noxxion.cpp
-- (SD%Complete: 100; class boss_noxxion : public CreatureScript {
-- boss_noxxionAI : public ScriptedAI }; GetAI via
-- GetMaraudonAI<boss_noxxionAI>; AddSC_boss_noxxion at end registers
-- the one script; kalimdor loader decl 62 / call 175 per
-- kalimdor_script_loader.cpp — the third "//Maraudon" loader group,
-- right after AddSC_boss_landslide()).
-- Entry: maraudon.h carries no NPC_ constants and the instance file
-- never names the boss — 13282 is wowhead-cited for "Noxxion"
-- (https://www.wowhead.com/classic/npc=13282/noxxion; no NPC_ constant
-- in the C++ tree; celebras / vishas / gelihast precedent); the
-- creature_template ScriptName binding stays DB-side.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (the self-contained in-combat legs):
-- - Engage (C++ JustEngagedWith is empty — no Talk): arm each timer at
--   its C++ Initialize() cooldown (hyjal.lua convention; Reset's
--   Initialize() timer-reset half collapses into the engage re-arm).
-- - Toxic Volley 21687 on the victim (C++ DoCastVictim — jeklik
--   GetVictim + CastSpell convention), non-triggered (C++ 2-arg cast),
--   init 7s -> 9s.
-- - Uppercut 22916 on the victim, non-triggered, init 16s -> 12s.
-- Unmodeled (documented-only, no bridges):
-- - The invisible/adds phase: C++ every 19s init -> 40s re-arm
--   (AddsTimer only ticks while visible; the invisible 15s window
--   freezes ALL timers including toxic volley / uppercut) —
--   InterruptNonMeleeSpells, SetFaction(FRIENDLY), UNIT_FLAG_NOT_SELECTABLE,
--   SetDisplayId(11686) / restore SetDisplayId(11172) + FACTION_MONSTER,
--   and the 5x DoSpawnCreature(13456, irand(-7,7) offsets,
--   TEMPSUMMON_TIMED_OR_CORPSE_DESPAWN 90s) with AttackStart(victim)
--   all have no faction / flag / display / summon / threat bridges
--   (standing) — so the lua timers keep their 9s/12s cadence straight
--   through the C++ invisible windows. The C++ file declares zero Talk
--   lines and no KilledUnit override — no event-3 registration.

local ENTRY = 13282

local SPELL_TOXIC_VOLLEY = 21687
local SPELL_UPPERCUT     = 22916

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

-- C++ ToxicVolley: DoCastVictim(21687), non-triggered, init 7s -> 9s.
local function onToxicVolley(creature, guid)
    local victim = creature:GetVictim()
    if victim and not victim:IsDead() then
        creature:CastSpell(victim, SPELL_TOXIC_VOLLEY)
    end
    schedule(guid, "toxicvolley", 9000, function()
        onToxicVolley(creature, guid)
    end)
end

-- C++ Uppercut: DoCastVictim(22916), non-triggered, init 16s -> 12s.
local function onUppercut(creature, guid)
    local victim = creature:GetVictim()
    if victim and not victim:IsDead() then
        creature:CastSpell(victim, SPELL_UPPERCUT)
    end
    schedule(guid, "uppercut", 12000, function()
        onUppercut(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "toxicvolley", 7000, function()
        onToxicVolley(creature, guid)
    end)
    schedule(guid, "uppercut", 16000, function()
        onUppercut(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)
