-- Commander Kolurg / Commander Stoutbeard (The Nexus) — Lua port of
-- src/server/scripts/Northrend/Nexus/Nexus/boss_nexus_commanders.cpp
-- (boss_nexus_commanders (CreatureScript) via GetNexusAI<
-- boss_nexus_commandersAI> (BossAI, DATA_COMMANDER); AddSC_boss_nexus_
-- commanders() registers the single script; loader decl 77 / call 272
-- per northrend_script_loader.cpp — the FIRST group of the
-- "// The Nexus: Nexus" block in AddNorthrendScripts(), immediately
-- after AddSC_instance_naxxramas() (decl 75 / call 270); the call after
-- it is AddSC_boss_magus_telestra() (decl 78 / call 273) — loader order
-- confirmed this run; the checkpoint sequence (instance_naxxramas ->
-- boss_nexus_commanders) is followed).
-- Entries: 26796 Commander Stoutbeard (nexus.h NPC_COMMANDER_
-- STOUTBEARD), 26798 Commander Kolurg (nexus.h NPC_COMMANDER_KOLURG)
-- — kalecgos pass; instance_nexus.cpp OnCreatureCreate binds both
-- NPC_COMMANDER_STOUTBEARD / NPC_ALLIANCE_COMMANDER to the
-- DATA_COMMANDER GUID (lines 59-60) and GetGuidData(DATA_COMMANDER)
-- resolves team-dependently (ALLIANCE team in instance -> KOLURG, else
-- STOUTBEARD, lines 90-91) — entry-verifiable, registration proceeds
-- for both entries (twin_valkyr shared-handler precedent); the
-- CreatureScript ScriptName binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "boss_nexus_commanders" hits boss_nexus_commanders.cpp only (loader
-- carries only the decl/call lines); zero sql/ hits. No nexus lua
-- existed.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent;
-- melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. Victim gating: C++ KilledUnit checks
-- TYPEID_PLAYER — Lua expresses it via victim:GetObjectType()
-- (nalorakk / kelthuzad player-gate variant).
-- creature:CastSpell(creature, spell) = DoCastSelf (phase_hunter
-- precedent).
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 0) (event 1; BossAI::JustEngagedWith
-- instance-bookkeeping leg has no bridge — tharon_ja precedent) +
-- DoCast(me, SPELL_BATTLE_SHOUT 31403) as creature:CastSpell(creature,
-- SPELL_BATTLE_SHOUT) + schedules EVENT_WHIRLWIND (6s, 8s); the
-- EVENT_CHARGE_COMMANDER / EVENT_FRIGHTENING_SHOUT scheduling legs are
-- documented below, not wired; the RemoveAurasDueToSpell(SPELL_FROZEN_
-- PRISON 47543) leg has no aura-removal bridge — documented, not wired.
-- EVENT_WHIRLWIND — DoCast(me, SPELL_WHIRLWIND 38618), 6s/8s init,
-- 19500ms/25s repeat.
-- KilledUnit — Talk(SAY_KILL 1) (event 3 — C++-GATED:
-- victim->GetTypeId() == TYPEID_PLAYER).
-- JustDied — Talk(SAY_DEATH 2) (event 4; the _JustDied bookkeeping has
-- no bridge — tharon_ja precedent). All timers cancelled on 2/4/23
-- (gargolmar precedent).
-- Unmodeled (no bridges — documented, not wired):
-- JustEngagedWith — RemoveAurasDueToSpell(SPELL_FROZEN_PRISON 47543)
-- (no aura-removal bridge — boss_amanitar / boss_anub_arak precedent).
-- EVENT_CHARGE_COMMANDER — SelectTarget(Random, 0, 100.0f, true) ->
-- DoCast(SPELL_CHARGE 60067), 3s/4s init, 11s/15s repeat — no
-- random-target SelectTarget bridge (cairne/kazzak via anubrekhan
-- precedent).
-- EVENT_FRIGHTENING_SHOUT — DoCastAOE(SPELL_FRIGHTENING_SHOUT 19134),
-- 13s/15s init, 45s/55s repeat — no DoCastAOE bridge
-- (terestian/shazzrah precedent).

local ENTRY_STOUTBEARD = 26796
local ENTRY_KOLURG = 26798

local SAY_AGGRO = 0
local SAY_KILL = 1
local SAY_DEATH = 2

local SPELL_BATTLE_SHOUT = 31403
local SPELL_WHIRLWIND = 38618

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

-- C++ EVENT_WHIRLWIND: DoCast(me, SPELL_WHIRLWIND 38618), 6s/8s init,
-- 19500ms/25s repeat.
local function whirlwindTick(creature, guid)
    creature:CastSpell(creature, SPELL_WHIRLWIND)
    schedule(guid, "whirlwind", math.random(19500, 25000), function()
        whirlwindTick(creature, guid)
    end)
end

-- C++ JustEngagedWith: Talk(SAY_AGGRO) + DoCast(me, SPELL_BATTLE_SHOUT
-- 31403) + schedules EVENT_WHIRLWIND (6s, 8s). The RemoveAurasDueToSpell
-- (SPELL_FROZEN_PRISON 47543) leg has no aura-removal bridge; the
-- EVENT_CHARGE_COMMANDER / EVENT_FRIGHTENING_SHOUT scheduling legs are
-- unbridged (no random-target SelectTarget / no DoCastAOE bridges) —
-- documented, not wired.
local function commandersEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    creature:CastSpell(creature, SPELL_BATTLE_SHOUT)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "whirlwind", math.random(6, 8) * 1000, function()
        whirlwindTick(creature, guid)
    end)
end

local function commandersLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ KilledUnit: Talk(SAY_KILL 1) only when victim->GetTypeId() ==
-- TYPEID_PLAYER (nalorakk / kelthuzad player-gate variant).
local function commandersTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

-- C++ JustDied: Talk(SAY_DEATH 2) (the _JustDied bookkeeping has no
-- bridge — tharon_ja precedent).
local function commandersDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    cancelTimers(creature:GetGUID())
end

local function commandersReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_STOUTBEARD, 1, commandersEnterCombat)
RegisterCreatureEvent(ENTRY_STOUTBEARD, 2, commandersLeaveCombat)
RegisterCreatureEvent(ENTRY_STOUTBEARD, 3, commandersTargetDied)
RegisterCreatureEvent(ENTRY_STOUTBEARD, 4, commandersDied)
RegisterCreatureEvent(ENTRY_STOUTBEARD, 23, commandersReset)

RegisterCreatureEvent(ENTRY_KOLURG, 1, commandersEnterCombat)
RegisterCreatureEvent(ENTRY_KOLURG, 2, commandersLeaveCombat)
RegisterCreatureEvent(ENTRY_KOLURG, 3, commandersTargetDied)
RegisterCreatureEvent(ENTRY_KOLURG, 4, commandersDied)
RegisterCreatureEvent(ENTRY_KOLURG, 23, commandersReset)
