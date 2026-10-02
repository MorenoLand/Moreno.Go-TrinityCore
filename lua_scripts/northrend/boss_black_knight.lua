-- The Black Knight (Trial of the Champion) — Lua port of
-- src/server/scripts/Northrend/CrusadersColiseum/TrialOfTheChampion/boss_black_knight.cpp
-- (boss_black_knight (ScriptedAI), npc_risen_ghoul (ScriptedAI),
-- npc_black_knight_skeletal_gryphon (EscortAI); AddSC_boss_black_knight
-- at end registers all three via GetTrialOfTheChampionAI).
-- The second Trial of the Champion group in northrend_script_loader.cpp
-- order (decl 46 / call 241, immediately after
-- AddSC_boss_argent_challenge(); Trial of the Champion block stays
-- OPEN).
-- Entry: 35451 The Black Knight (trial_of_the_champion.h
-- NPC_BLACK_KNIGHT — kalecgos pass; the GetTrialOfTheChampionAI
-- ScriptName binding is instance-shimmed, the creature_template binding
-- DB-side as usual).
-- Sole-source verified: whole-server-tree grep for "boss_black_knight",
-- "npc_risen_ghoul" and "npc_black_knight_skeletal_gryphon" hits
-- boss_black_knight.cpp only (loader carries only the AddSC decl/call
-- lines); zero sql/ hits.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady. C++ DoCast
-- default is triggered=false (vaelastrasz convention):
-- creature:CastSpell(nil, spell) = DoCastVictim (moroes precedent);
-- creature:CastSpell(creature, spell) = DoCastSelf (phase_hunter /
-- apothecary_hanes precedent — the trailing true is the triggered
-- port).
-- C++ has no Say enum at all ("missing yells" in its SD%Complete
-- 80 % note) — no Talk arms to port.
-- C++ note: the uiPlagueStrikeTimer arm calls
-- DoCastVictim(SPELL_ICY_TOUCH), not Plague Strike — replicated
-- C++-exact here (apparent C++ bug; the timers' init/repeat windows
-- are verbatim from C++ Initialize()).
-- Ported arms (C++-exact for all modeled arms):
-- boss_black_knight: uiIcyTouchTimer — DoCastVictim(Icy Touch 67718),
-- urand(5000,9000) init, urand(5000,7000) repeat; uiPlagueStrikeTimer —
-- DoCastVictim(Icy Touch 67718) (see note above), urand(10000,13000)
-- init, urand(12000,15000) repeat; uiObliterateTimer —
-- DoCastVictim(Obliterate 67725), urand(17000,19000) init, repeat —
-- all scheduled from combat start and independent of the unbridged
-- DamageTaken resurrect machine (the three arms fire identically in
-- PHASE_UNDEAD and PHASE_SKELETON; the phase machine never advances
-- here, so the fight plays its phase-1 rotation until death —
-- boss_eadric precedent for timer arms independent of an unbridged
-- DamageTaken leg). JustDied: DoCastSelf(Kill Credit 68663) on event
-- 4 (its instance SetBossState(BOSS_BLACK_KNIGHT, DONE) leg has no
-- bridge — instance model absent, standing). OnLeaveCombat(2) /
-- OnDied(4) / OnReset(23) cancel the scheduler (gargolmar precedent).
-- Unmodeled (no bridges — documented, not wired):
-- boss_black_knight DamageTaken — damage > health && uiPhase <=
-- PHASE_SKELETON -> damage = 0 + SetHealth(0) + AddUnitState(ROOT |
-- STUNNED) + summons.DespawnAll() + SetDisplayId(MODEL_SKELETON
-- 29846 / MODEL_GHOST 21300) + bEventInProgress — no DamageTaken hook
-- (npc_unkor_the_ruthless precedent) + no unit-state / display /
-- summon bridges.
-- boss_black_knight bEventInProgress resurrect machine —
-- SetFullHealth + DoCastSelf(Black Knight Res 67693, true) + uiPhase++
-- + ClearUnitState, gated solely on the unbridged DamageTaken leg
-- (jedoga over-cast bar — never fires here).
-- boss_black_knight uiDeathRespiteTimer (PHASE_UNDEAD) —
-- SelectTarget(Random, 0, 100, true) -> DoCast(Death's Respite 67745),
-- urand(15000,16000) — random-target SelectTarget bridge absent
-- (cairne/kazzak precedent).
-- boss_black_knight PHASE_SKELETON legs — phase never arrives (the
-- DamageTaken-driven machine is unbridged): bSummonArmy one-shot
-- AddUnitState(ROOT | STUNNED) + DoCastSelf(Army of the Dead 67761)
-- (no unit-state bridge) + uiDeathArmyCheckTimer's ClearUnitState;
-- uiDesecration — SelectTarget(Random, 0, 100, true) ->
-- DoCast(Desecration 67778), urand(15000,16000) (random-target bridge
-- absent); uiGhoulExplodeTimer — DoCastSelf(Ghoul Explode 67751),
-- 8000 repeat (self-cast bridged in isolation, but phase-gated —
-- jedoga bar).
-- boss_black_knight PHASE_GHOST legs — phase never arrives:
-- uiDeathBiteTimer — DoCastAOE(Death's Bite 67808),
-- urand(2000,4000) — no DoCastAOE bridge (terestian/shazzrah
-- precedent); uiMarkedDeathTimer — SelectTarget(Random, 0, 100, true)
-- -> DoCast(Marked for Death 67882), urand(5000,7000) — random-target
-- bridge absent.
-- boss_black_knight Reset — summons.DespawnAll() + SetDisplayId(native)
-- + ClearUnitState(ROOT | STUNNED) — no summon / display / unit-state
-- bridges; JustSummoned — summons.Summon + cross-AI AttackStart(me->
-- GetVictim()) (summon STRAND absent, standing). Melee gated on
-- !HasUnitState(ROOT) && !HealthBelowPct(1) — no unit-state bridge +
-- no health-pct bridge (doomwalker precedent); melee plays
-- engine-driven unconditionally in this port.
-- npc_risen_ghoul — zero registration: ENTRY UNVERIFIABLE (no NPC
-- constant for the ghoul in trial_of_the_champion.h or anywhere in the
-- C++ sources; the ghouls materialize via the DB-side SPELL_ARMY_DEAD
-- 67761 summon effect — belnistrasz/willix precedent). Joins the
-- bridgeable-but-entry-blocked queue with a port-pattern-ready rotation
-- arm: uiAttackTimer — SelectTarget(Random, 1, 100, true) ->
-- DoCast(Leap 67749), 3500 repeat — random-target SelectTarget bridge
-- absent (cairne/kazzak precedent).
-- npc_black_knight_skeletal_gryphon — zero registration: ENTRY
-- UNVERIFIABLE (no NPC constant ties the script name to an entry in
-- C++ — the creature_template binding is DB-side; the header's
-- VEHICLE_BLACK_KNIGHT 35491 names the mount, not the script binding).
-- Whole AI is the EscortAI machine (constructor Start(false, true) +
-- EscortAI::UpdateAI) — escort / motion-master bridges absent (escort
-- queue precedent). Joins the bridgeable-but-entry-blocked queue.

local ENTRY_BLACK_KNIGHT = 35451

local SPELL_ICY_TOUCH = 67718
local SPELL_OBLITERATE = 67725
local SPELL_KILL_CREDIT = 68663

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

-- C++ uiIcyTouchTimer: DoCastVictim(Icy Touch), urand(5000,9000) init,
-- urand(5000,7000) repeat; independent of the unbridged DamageTaken /
-- phase machine (fires identically in PHASE_UNDEAD and PHASE_SKELETON).
local function icyTouchTick(creature, guid)
    creature:CastSpell(nil, SPELL_ICY_TOUCH)
    local next = math.random(5000, 7000)
    schedule(guid, "icy_touch", next, function()
        icyTouchTick(creature, guid)
    end)
end

-- C++ uiPlagueStrikeTimer: DoCastVictim(SPELL_ICY_TOUCH) — the C++
-- names the timer Plague Strike but casts Icy Touch (apparent C++
-- bug); replicated C++-exact. urand(10000,13000) init,
-- urand(12000,15000) repeat.
local function plagueStrikeTick(creature, guid)
    creature:CastSpell(nil, SPELL_ICY_TOUCH)
    local next = math.random(12000, 15000)
    schedule(guid, "plague_strike", next, function()
        plagueStrikeTick(creature, guid)
    end)
end

-- C++ uiObliterateTimer: DoCastVictim(Obliterate), urand(17000,19000)
-- init, urand(17000,19000) repeat.
local function obliterateTick(creature, guid)
    creature:CastSpell(nil, SPELL_OBLITERATE)
    local next = math.random(17000, 19000)
    schedule(guid, "obliterate", next, function()
        obliterateTick(creature, guid)
    end)
end

local function blackKnightEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "icy_touch", math.random(5000, 9000), function()
        icyTouchTick(creature, guid)
    end)
    schedule(guid, "plague_strike", math.random(10000, 13000), function()
        plagueStrikeTick(creature, guid)
    end)
    schedule(guid, "obliterate", math.random(17000, 19000), function()
        obliterateTick(creature, guid)
    end)
end

local function blackKnightLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ JustDied: DoCastSelf(Kill Credit 68663) (the
-- instance->SetBossState leg has no bridge).
local function blackKnightDied(event, creature, killer)
    creature:CastSpell(creature, SPELL_KILL_CREDIT)
    cancelTimers(creature:GetGUID())
end

local function blackKnightReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_BLACK_KNIGHT, 1, blackKnightEnterCombat)
RegisterCreatureEvent(ENTRY_BLACK_KNIGHT, 2, blackKnightLeaveCombat)
RegisterCreatureEvent(ENTRY_BLACK_KNIGHT, 4, blackKnightDied)
RegisterCreatureEvent(ENTRY_BLACK_KNIGHT, 23, blackKnightReset)
