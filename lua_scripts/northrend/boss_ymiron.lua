-- King Ymiron (Utgarde Pinnacle) — Lua port of
-- src/server/scripts/Northrend/UtgardeKeep/UtgardePinnacle/boss_ymiron.cpp
-- (355 lines; 3 scripts — boss_ymiron (CreatureScript via
-- GetUtgardePinnacleAI (BossAI), DATA_KING_YMIRON = 3);
-- spell_dark_slash (bare SpellScript via RegisterSpellScript, SpellScript
-- OnHit -> CalculateDamage half-target-health); achievement_kings_bane
-- (AchievementCriteriaScript, OnCheck GetData(DATA_KINGS_BANE));
-- all registered from inside AddSC_boss_ymiron(); loader decl 135 /
-- call 330 per northrend_script_loader.cpp — the FOURTH group of the "//
-- Utgarde Keep - Utgarde Pinnacle" block in AddNorthrendScripts(),
-- immediately after AddSC_boss_skadi() (call 329); the checkpoint
-- sequence (boss_skadi -> boss_ymiron) is followed).
-- Entries: 26861 King Ymiron (utgarde_pinnacle.h NPC_KING_YMIRON,
-- line 56) — entry-verifiable, registration proceeds (the nexus_commanders
-- / malygos / sartharion kalecgos precedent); the CreatureScript
-- ScriptName binding is DB-side as usual. DATA_KING_YMIRON = 3
-- (utgarde_pinnacle.h line 34).
-- Sole-source verified: whole-server-tree grep for "AddSC_boss_ymiron"
-- hits boss_ymiron.cpp only (+ the loader decl/call lines); this clone
-- carries no sql/ tree, so ScriptName bindings are DB-side by
-- construction. No ymiron lua existed.
-- Eluna creature events: 1 OnEnterCombat, 3 OnKill, 4 OnDeath.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 0) — unconditional (the auriaya
-- engage-port precedent; BossAI::JustEngagedWith passthrough and the
-- EVENT_BANE / EVENT_FETID_ROT / EVENT_DARK_SLASH /
-- EVENT_ANCESTORS_VENGEANCE schedule legs have no bridges).
-- JustDied — Talk(SAY_DEATH 2) — unconditional (the sjonnir
-- JustDied-Talk precedent; the _JustDied() passthrough leg has no
-- bridge).
-- KilledUnit — Talk(SAY_SLAY 1) player-gated (event 3 — the razuvious
-- player-gated variant precedent).
-- DOCUMENTED-ONLY (in this header; no registration beyond entry 26861):
-- Talk(SAY_SUMMON_BJORN 3 / SAY_SUMMON_HALDOR 4 / SAY_SUMMON_RANULF 5 /
-- SAY_SUMMON_TORGYN 6) rides the MovementInform POINT_BOAT leg (the
-- DamageTaken health-threshold -> MovePoint(POINT_BOAT) ->
-- ActiveOrder-shuffled ancestor summon / channel / REACT_PASSIVE /
-- EVENT_RESUME_COMBAT machine) — no MovementInform / timer-event
-- bridges; joins the respective bridge queues.
-- spell_dark_slash — SpellScript OnHit damage hook only; joins the
-- no-SpellScript-bridge queue.
-- achievement_kings_bane (OnCheck: GetData(DATA_KINGS_BANE 2157) == 1) —
-- no GetData / achievement bridges; joins the unmodeled-achievement
-- queue.

local ENTRY_KING_YMIRON = 26861

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2

-- C++ boss_ymironAI::JustEngagedWith: BossAI::JustEngagedWith(who)
-- (passthrough, no bridge); Talk(SAY_AGGRO) — unconditional (the auriaya
-- engage-port precedent).
local function ymironEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ KilledUnit: if (who->GetTypeId() == TYPEID_PLAYER) Talk(SAY_SLAY)
-- — the razuvious player-gated variant precedent.
local function ymironKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ JustDied: _JustDied() (passthrough, no bridge);
-- Talk(SAY_DEATH) — unconditional — the sjonnir JustDied-Talk precedent.
local function ymironJustDied(event, creature)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_KING_YMIRON, 1, ymironEnterCombat)
RegisterCreatureEvent(ENTRY_KING_YMIRON, 3, ymironKilledUnit)
RegisterCreatureEvent(ENTRY_KING_YMIRON, 4, ymironJustDied)
