-- King Dred (Drak'Tharon Keep) — Lua port of
-- src/server/scripts/Northrend/DraktharonKeep/boss_king_dred.cpp
-- (boss_king_dred (BossAI), npc_drakkari_gutripper (ScriptedAI),
-- npc_drakkari_scytheclaw (ScriptedAI),
-- achievement_king_dred (AchievementCriteriaScript);
-- AddSC_boss_king_dred at end registers all via GetDrakTharonKeepAI /
-- new). The third Drak'Tharon Keep group in northrend_script_loader.cpp
-- order (decl 41 / call 236, immediately after AddSC_boss_novos();
-- next: AddSC_boss_tharon_ja()).
-- Entry: 27483 King Dred (drak_tharon_keep.h NPC_KING_DRED — kalecgos
-- pass; the GetDrakTharonKeepAI ScriptName binding is
-- instance-shimmed, the creature_template binding DB-side as usual).
-- NOTE: unlike the other Drak'Tharon Keep bosses, King Dred has no
-- SAY/yell enum at all — no Talk arms exist in C++ to port.
-- Sole-source verified: whole-server-tree grep for "boss_king_dred",
-- "npc_drakkari_gutripper", "npc_drakkari_scytheclaw" and
-- "achievement_king_dred" hits boss_king_dred.cpp only (loader carries
-- only the decl/call lines); zero sql/ hits.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady. C++ DoCast
-- default is triggered=false (vaelastrasz convention):
-- creature:CastSpell(nil, spell) = DoCastVictim (moroes precedent).
-- OnLeaveCombat(2)/OnDied(4)/OnReset(23) cancel the scheduler
-- (gargolmar precedent); BossAI _Reset/_JustDied instance bookkeeping
-- has no bridge.
-- Ported arms (C++-exact for the modeled arms — the three
-- JustEngagedWith schedules are independent, none of the unmodeled
-- events gate them):
-- boss_king_dred: EVENT_GRIEVOUS_BITE DoCastVictim(Grievous Bite
-- 48920), 20s init, 20s repeat (event 1 scheduler);
-- EVENT_MANGLING_SLASH DoCastVictim(Mangling Slash 48873), 18500ms
-- init, 18500ms repeat; EVENT_PIERCING_SLASH DoCastVictim(Piercing
-- Slash 48878), 15s init, 15s repeat. No Talk, no JustDied legs —
-- C++ JustDied is only _JustDied() (instance bookkeeping, no bridge).
-- Unmodeled (no bridges — documented, not wired):
-- EVENT_BELLOWING_ROAR — DoCastAOE(Bellowing Roar 22686), 33s init,
-- 33s repeat — no DoCastAOE bridge (terestian/shazzrah precedent).
-- EVENT_FEARSOME_ROAR — DoCastAOE(Fearsome Roar 48849), randtime(
-- 10s,20s) init, 10-20s repeat — no DoCastAOE bridge.
-- EVENT_RAPTOR_CALL — DoCastVictim(SPELL_RAPTOR_CALL 59416, dummy) +
-- me->SummonCreature(RAND(NPC_DRAKKARI_GUTRIPPER 26641 /
-- NPC_DRAKKARI_SCYTHECLAW 26628), GetClosePoint(combatReach/3, 10.0f),
-- TEMPSUMMON_DEAD_DESPAWN 1s), randtime(20s,25s) init, 20-25s repeat
-- — the meaningful leg is the summon: GetClosePoint + SummonCreature
-- ride the absent summon STRAND bridge (standing). The only
-- bridgeable leg is the no-op dummy cast, whose payload is the
-- unbridged summon — arming a timer for the dummy alone would fire a
-- no-effect cast without its C++ payload, so the timer is not armed
-- (genuinely-bridgeable bar).
-- UpdateAI HasUnitState(UNIT_STATE_CASTING) skip — no unit-state
-- bridge.
-- DoAction(ACTION_RAPTOR_KILLED 1) ++raptorsKilled + GetData(
-- DATA_RAPTORS_KILLED 2) — no DoAction/GetData bridges
-- (drakkari_colossus precedent); its only caller is
-- achievement_king_dred — see below.
-- npc_drakkari_gutripper / npc_drakkari_scytheclaw — ENTRY-VERIFIABLE
-- BUT BRIDGE-BLOCKED (26641 / 26628 — drak_tharon_keep.h
-- NPC_DRAKKARI_GUTRIPPER / NPC_DRAKKARI_SCYTHECLAW, kalecgos pass):
-- zero registration. Both exist in C++ only as the unbridged
-- EVENT_RAPTOR_CALL SummonCreature targets, so wiring them would
-- attach to never-spawned creatures — C++-exact bar fails (amanitar
-- mushrooms precedent). Port-pattern-ready rotation arms: gutripper
-- GutRipTimer urand(10s,15s) -> DoCastVictim(SPELL_GUT_RIP 49710);
-- scytheclaw uiRendTimer urand(10s,15s) -> DoCastVictim(SPELL_REND
-- 13738). JustDied of both is ObjectAccessor::GetCreature(*me,
-- instance->GetGuidData(DATA_KING_DRED))->AI()->DoAction(
-- ACTION_RAPTOR_KILLED) — instance GetGuidData + ObjectAccessor +
-- DoAction bridges absent. Joins the entry-verifiable-but-
-- bridge-blocked queue.
-- achievement_king_dred — AchievementCriteriaScript (target
-- ToCreature -> AI()->GetData(DATA_RAPTORS_KILLED) >= 6) — no
-- achievement-criteria bridge (snakes precedent) + cross-AI GetData
-- bridge absent — documented-only.

local ENTRY_KING_DRED = 27483

local SPELL_GRIEVOUS_BITE = 48920
local SPELL_MANGLING_SLASH = 48873
local SPELL_PIERCING_SLASH = 48878

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

-- C++ EVENT_GRIEVOUS_BITE: DoCastVictim; 20s init, 20s repeat.
local function grievousBiteTick(creature, guid)
    creature:CastSpell(nil, SPELL_GRIEVOUS_BITE)
    schedule(guid, "grievousbite", 20000, function()
        grievousBiteTick(creature, guid)
    end)
end

-- C++ EVENT_MANGLING_SLASH: DoCastVictim; 18500ms init, 18500ms
-- repeat.
local function manglingSlashTick(creature, guid)
    creature:CastSpell(nil, SPELL_MANGLING_SLASH)
    schedule(guid, "manglingslash", 18500, function()
        manglingSlashTick(creature, guid)
    end)
end

-- C++ EVENT_PIERCING_SLASH: DoCastVictim; 15s init, 15s repeat.
local function piercingSlashTick(creature, guid)
    creature:CastSpell(nil, SPELL_PIERCING_SLASH)
    schedule(guid, "piercingslash", 15000, function()
        piercingSlashTick(creature, guid)
    end)
end

local function kingDredEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "grievousbite", 20000, function()
        grievousBiteTick(creature, guid)
    end)
    schedule(guid, "manglingslash", 18500, function()
        manglingSlashTick(creature, guid)
    end)
    schedule(guid, "piercingslash", 15000, function()
        piercingSlashTick(creature, guid)
    end)
end

local function kingDredLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function kingDredDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function kingDredReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_KING_DRED, 1, kingDredEnterCombat)
RegisterCreatureEvent(ENTRY_KING_DRED, 2, kingDredLeaveCombat)
RegisterCreatureEvent(ENTRY_KING_DRED, 4, kingDredDied)
RegisterCreatureEvent(ENTRY_KING_DRED, 23, kingDredReset)
