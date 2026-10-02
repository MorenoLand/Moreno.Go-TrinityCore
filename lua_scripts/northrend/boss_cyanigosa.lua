-- Cyanigosa (Violet Hold) — Lua port of
-- src/server/scripts/Northrend/VioletHold/boss_cyanigosa.cpp
-- (172 lines; 3 scripts — boss_cyanigosa (CreatureScript via
-- GetVioletHoldAI (BossAI), DATA_CYANIGOSA = 2); achievement_defenseless
-- (AchievementCriteriaScript, OnCheck instance->GetData(DATA_DEFENSELESS));
-- spell_cyanigosa_arcane_vacuum (SpellScriptLoader, OnEffectHitTarget
-- casts SPELL_SUMMON_PLAYER 21150); all registered from inside
-- AddSC_boss_cyanigosa(); loader decl 145 / call 340 per
-- northrend_script_loader.cpp — the FIRST group of the "// Violet Hold"
-- block in AddNorthrendScripts(), immediately after
-- AddSC_instance_vault_of_archavon() (call 338); the call after it is
-- AddSC_boss_erekem() (call 341) — the checkpoint sequence
-- (instance_vault_of_archavon -> boss_cyanigosa) is followed).
-- Entries: 31134 Cyanigosa (violet_hold.h NPC_CYANIGOSA, line 116) —
-- entry-verifiable, registration proceeds (the nexus_commanders /
-- malygos / sartharion kalecgos precedent); the CreatureScript
-- ScriptName binding is DB-side as usual. DATA_CYANIGOSA = 2
-- (violet_hold.h line 51).
-- Sole-source verified: whole-server-tree grep for "AddSC_boss_cyanigosa"
-- hits boss_cyanigosa.cpp only (+ the loader decl/call lines); this
-- clone carries no sql/ tree, so ScriptName bindings are DB-side by
-- construction. No cyanigosa lua existed.
-- Eluna creature events: 1 OnEnterCombat, 3 OnKill, 4 OnDeath.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 0) — unconditional (the auriaya
-- engage-port precedent; the BossAI::JustEngagedWith passthrough has
-- no bridge).
-- KilledUnit — Talk(SAY_SLAY 1) player-gated (event 3 — the razuvious
-- player-gated variant precedent).
-- JustDied — Talk(SAY_DEATH 2) — unconditional (the sjonnir
-- JustDied-Talk precedent; the _JustDied() passthrough leg has no
-- bridge).
-- DOCUMENTED-ONLY (in this header; no registration beyond entry 31134):
-- SAY_SPAWN (3) / SAY_DISRUPTION (4) / SAY_BREATH_ATTACK (5) /
-- SAY_SPECIAL_ATTACK (6) are never Talk()ed anywhere in the file
-- (grep -c "Talk(" = 3 total, all ported above) — dead enum text.
-- The ScheduleTasks scheduler machine (SPELL_ARCANE_VACUUM 10s /
-- SPELL_BLIZZARD 15s / SPELL_TAIL_SWEEP 20s / SPELL_UNCONTROLLABLE_ENERGY
-- 25s, plus heroic-only SPELL_MANA_DESTRUCTION 30s), the empty
-- MoveInLineOfSight override, the spell_cyanigosa_arcane_vacuum
-- SpellScript OnEffectHitTarget hook and the achievement_defenseless
-- GetData(DATA_DEFENSELESS) check — no timer / SpellScript /
-- achievement bridges; join the respective bridge queues.

local ENTRY_CYANIGOSA = 31134

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2

-- C++ boss_cyanigosaAI::JustEngagedWith: BossAI::JustEngagedWith(who)
-- (passthrough, no bridge); Talk(SAY_AGGRO) — unconditional (the auriaya
-- engage-port precedent).
local function cyanigosaEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ KilledUnit: if (victim->GetTypeId() == TYPEID_PLAYER)
-- Talk(SAY_SLAY) — the razuvious player-gated variant precedent.
local function cyanigosaKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ JustDied: _JustDied() (passthrough, no bridge);
-- Talk(SAY_DEATH) — unconditional — the sjonnir JustDied-Talk precedent.
local function cyanigosaJustDied(event, creature)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_CYANIGOSA, 1, cyanigosaEnterCombat)
RegisterCreatureEvent(ENTRY_CYANIGOSA, 3, cyanigosaKilledUnit)
RegisterCreatureEvent(ENTRY_CYANIGOSA, 4, cyanigosaJustDied)
