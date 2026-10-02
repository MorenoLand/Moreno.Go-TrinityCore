-- Svala Sorrowgrave (Utgarde Pinnacle) — Lua port of
-- src/server/scripts/Northrend/UtgardeKeep/UtgardePinnacle/boss_svala.cpp
-- (621 lines; 6 scripts — boss_svala (CreatureScript via
-- GetUtgardePinnacleAI (BossAI), DATA_SVALA_SORROWGRAVE = 0);
-- npc_ritual_channeler (CreatureScript via GetUtgardePinnacleAI
-- (ScriptedAI)); npc_spectator (CreatureScript via GetUtgardePinnacleAI
-- (ScriptedAI)); spell_paralyze_pinnacle (SpellScriptLoader, SpellScript
-- OnObjectAreaTargetSelect); npc_scourge_hulk (CreatureScript via
-- GetUtgardePinnacleAI (ScriptedAI)); achievement_incredible_hulk
-- (AchievementCriteriaScript OnCheck); all registered from inside
-- AddSC_boss_svala(); loader decl 132 / call 327 per
-- northrend_script_loader.cpp — the FIRST group of the "// Utgarde Keep -
-- Utgarde Pinnacle" block in AddNorthrendScripts(), immediately after
-- AddSC_utgarde_keep() (call 325); the checkpoint sequence
-- (utgarde_keep -> boss_svala) is followed).
-- Entries: 26668 Svala Sorrowgrave (utgarde_pinnacle.h
-- NPC_SVALA_SORROWGRAVE, line 53) — entry-verifiable, registration
-- proceeds (the nexus_commanders / malygos / sartharion kalecgos
-- precedent); the CreatureScript ScriptName binding is DB-side as usual.
-- DATA_SVALA_SORROWGRAVE = 0 (utgarde_pinnacle.h line 31). The intro form
-- NPC_SVALA = 29281 (line 59) UpdateEntry-transforms into 26668; its Talk
-- arms ride the EVENT_INTRO_* timer machine (see DOCUMENTED-ONLY).
-- Sole-source verified: whole-server-tree grep for all six script names
-- hits boss_svala.cpp only (+ the loader decl/call lines for
-- AddSC_boss_svala); this clone carries no sql/ tree, so ScriptName
-- bindings are DB-side by construction. No svala lua existed.
-- Eluna creature events: 1 OnEnterCombat, 3 OnKill, 4 OnDied.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 2) (event 1; BossAI::JustEngagedWith
-- passthrough unmodeled — the auriaya engage-port precedent).
-- KilledUnit — Talk(SAY_SLAY 3) player-gated (event 3 — the razuvious
-- player-gated variant precedent).
-- JustDied — Talk(SAY_DEATH 4) unconditional (event 4 — the sjonnir
-- JustDied-Talk precedent; the SACRIFICING SetEquipmentSlots /
-- HandleEmoteCommand(EMOTE_ONESHOT_FLYDEATH) / _JustDied() legs have no
-- bridges).
-- DOCUMENTED-ONLY (in this header; no registration beyond entry 26668):
-- The whole intro Talk machine — Talk(SAY_SVALA_INTRO_0 0) on the
-- pre-transform entry (EVENT_INTRO_SVALA_TALK_0), Talk(SAY_SVALA_INTRO_1
-- 0) / Talk(SAY_SVALA_INTRO_2 1) on the boss entry
-- (EVENT_INTRO_SVALA_TALK_1 / EVENT_INTRO_SVALA_TALK_2), and the
-- cross-creature arthas->AI()->Talk(SAY_DIALOG_OF_ARTHAS_1 0 /
-- SAY_DIALOG_OF_ARTHAS_2 1) — the timer event machine has no bridge;
-- joins the no-timer-bridge queue (cross-creature Talk has no bridge).
-- Talk(SAY_SACRIFICE_PLAYER 5) rides EVENT_RITUAL_PREPARATION (timer) —
-- joins the no-timer-bridge queue.
-- The phase transitions (MoveInLineOfSight IDLE -> INTRO,
-- INTRO -> NORMAL, NORMAL -> SACRIFICING at HealthBelowPct(50),
-- SACRIFICING -> NORMAL on SPELL_RITUAL_STRIKE_EFF_1 SpellHitTarget),
-- the spectator MovePoint/DESPAWN flee machine (no Talk arms, no
-- MovementInform bridge), the ritual-channeler paralyze machine
-- (instance GetGuidData(DATA_SACRIFICED_PLAYER), no Talk arms), the
-- scourge-hulk mightyBlow / volatileInfection timers (no Talk arms),
-- spell_paralyze_pinnacle (SpellScript target filter) and
-- achievement_incredible_hulk (GetData(DATA_INCREDIBLE_HULK) check) —
-- no bridges; join the respective bridge queues.

local ENTRY_SVALA_SORROWGRAVE = 26668

local SAY_AGGRO = 2
local SAY_SLAY = 3
local SAY_DEATH = 4

-- C++ boss_svalaAI::JustEngagedWith: BossAI::JustEngagedWith(who)
-- (passthrough, no bridge); Talk(SAY_AGGRO) — unconditional (the auriaya
-- engage-port precedent).
local function svalaEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ KilledUnit: if (who->GetTypeId() == TYPEID_PLAYER) Talk(SAY_SLAY)
-- — the razuvious player-gated variant precedent.
local function svalaKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ JustDied: <unbridgeable equipment/emote/passthrough legs>;
-- Talk(SAY_DEATH) — unconditional — the sjonnir JustDied-Talk precedent.
local function svalaJustDied(event, creature)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_SVALA_SORROWGRAVE, 1, svalaEnterCombat)
RegisterCreatureEvent(ENTRY_SVALA_SORROWGRAVE, 3, svalaKilledUnit)
RegisterCreatureEvent(ENTRY_SVALA_SORROWGRAVE, 4, svalaJustDied)
