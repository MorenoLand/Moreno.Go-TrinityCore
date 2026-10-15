-- Algalon the Observer (Ulduar) — Lua port of
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_algalon_the_observer.cpp
-- (1300 lines; 17 scripts — boss_algalon_the_observer (CreatureScript via
-- RegisterUlduarCreatureAI (BossAI), BOSS_ALGALON = 13);
-- npc_living_constellation / npc_black_hole / npc_collapsing_star /
-- npc_brann_bronzebeard_algalon (CreatureScript via
-- RegisterUlduarCreatureAI); go_celestial_planetarium_access
-- (GameObjectScript); ten spell_* scripts (spell_algalon_phase_punch /
-- spell_algalon_phase_constellation / spell_algalon_trigger_3_adds /
-- spell_algalon_collapse / spell_algalon_big_bang /
-- spell_algalon_remove_phase / spell_algalon_cosmic_smash /
-- spell_algalon_cosmic_smash_damage / spell_algalon_supermassive_fail /
-- spell_algalon_black_hole_phase_shifts (SpellScript / AuraScript));
-- achievement_he_feeds_on_your_tears (AchievementCriteriaScript); all
-- registered from inside AddSC_boss_algalon_the_observer(); loader decl
-- 123 / call 318 per northrend_script_loader.cpp — the FOURTEENTH group
-- of the "// Ulduar" block in AddNorthrendScripts(), immediately after
-- AddSC_boss_yogg_saron() (decl 122 / call 317); the call after it is
-- AddSC_instance_ulduar() — the Ulduar boss slice is closed by this run,
-- the Ulduar block's remaining unit is the instance script — loader order
-- confirmed this run; the checkpoint sequence (boss_yogg_saron ->
-- boss_algalon_the_observer) is followed).
-- Entry: 32871 Algalon (ulduar.h NPC_ALGALON, line 83 — entry-verifiable,
-- registration proceeds (the nexus_commanders / malygos / sartharion
-- kalecgos precedent); the CreatureScript ScriptName binding is DB-side
-- as usual). BOSS_ALGALON = 13 (ulduar.h line 49).
-- Sole-source verified: whole-server-tree grep for each of the seventeen
-- script names hits boss_algalon_the_observer.cpp (+ the loader decl/call
-- lines for AddSC_boss_algalon_the_observer) only; zero sql/ hits for all
-- seventeen. Lua body: ported Oct 2 09:46 in 8af8496; re-audited this run
-- — both ported arms C++-exact (player-gated event-3 slay with the
-- os.time() 1s rate-limit deadline; event-9 phase-two threshold+latch).
-- Eluna creature events: 3 OnTargetDied, 9 OnDamageTaken, 23 OnReset.
-- Ported arms (C++-exact for all modeled arms):
-- KilledUnit — player-gated Talk(SAY_ALGALON_KILL 20) (event 3;
-- who->GetTypeId() == TYPEID_PLAYER — the razuvious player-gated
-- variant precedent — at least the fortieth player-gated variant ported
-- (39 'player-gated' lua files git-born before it));
-- the C++ _hasYelled 1s rate limit (EVENT_UNLOCK_YELL) is modeled with
-- an os.time() per-guid deadline (the malchezaar os.time-deadline
-- precedent); 1s resolution caveat — the re-yell unlocks on the next
-- whole second rather than exactly 1000ms after the yell.
-- DamageTaken phase-two — Talk(SAY_ALGALON_PHASE_TWO 11) gated on
-- HealthBelowPctDamaged(20, damage) + the _phaseTwo latch (event 9; the
-- moroes threshold+latch precedent); the companion legs (summons
-- DespawnEntry NPC_LIVING_CONSTELLATION / NPC_COLLAPSING_STAR /
-- NPC_BLACK_HOLE / NPC_ALGALON_VOID_ZONE_VISUAL_STALKER, the stalker
-- KillAllEvents sweep, EVENT_SUMMON_COLLAPSING_STAR cancel, the four
-- worm-hole summons) have no bridges — documented only.
-- DOCUMENTED-ONLY (in this header; no registration beyond entry 32871):
-- JustEngagedWith — the _firstPull flag chooses Talk(SAY_ALGALON_START_TIMER
-- 3) (first pull, 26.5s intro) vs Talk(SAY_ALGALON_AGGRO 4) (later pulls)
-- — no bridge for the construction/instance flag; event 1 deliberately
-- NOT registered (no single C++-exact yell).
-- DamageTaken _fightWon branch — (health - damage) below 2.5% of max:
-- damage = 0 + the outro event machine (REACT_PASSIVE, AttackStop,
-- SPELL_SELF_STUN self-cast, summons.DespawnAll, EVENT_OUTRO_START ...)
-- — no Talk arm in this branch; a lone damage=0 return would strand the
-- boss unkillable without the outro, so it is not modeled.
-- Timer Talks — 2026-10-14 retraction re-audit (C++ line refs): none
-- ported on combat-start timers; every arm rides an unbridged machine
-- (stale "no-timer-bridge" claim RETRACTED — CreateLuaEvent is a real
-- bridge; the blocker is machine gating, gothik precedent).
-- COLLAPSING_STAR (18s+introDelay, :594-599) — Talk 5/6 + SummonCreature x4;
-- BIG_BANG (90s+introDelay, :601-616) — Talk 7/8 + SetPhase(PHASE_BIG_BANG)
-- + constellation DoAction(ACTION_BIG_BANG) + DoCastAOE(64443): unbridged
-- summon leg / phase machine; Talk-only ports would fire with no stars /
-- no phase context — stay out (their cast companions live on other arms).
-- Intro machine: Talk 0/1/2 (:529/:541/:546, EVENT_INTRO_1..3) chained under
-- PHASE_ROLE_PLAY from DoAction(ACTION_START_INTRO) (:328, Brann-summoned
-- pre-combat) — pre-combat roleplay, porting on combat-start timers would
-- fabricate them mid-fight. EVENT_INTRO_TIMER_DONE (:560-584) has NO Talk
-- arm (header correction this run) — pure machinery (SetPhase(NORMAL),
-- constellation summons); Talk(AGGRO 4) fires in JustEngagedWith
-- non-first-pull (:372) and EVENT_START_COMBAT (:555, 16s, first-pull only).
-- Outro machine: Talk 12..16 (:664-682, EVENT_OUTRO_6..10) chained from the
-- DamageTaken _fightWon branch (:483-495) via MovementInform spline arrival
-- (:413) and a Brann-summon DoAction(:657-661) — unbridged; a Talk-only
-- port would strand the boss without the outro.
-- Despawn machine: Talk 17..19 (:689-697, 5s/17s/26s) on
-- DoAction(EVENT_DESPAWN_ALGALON) — fired by the instance script on the
-- one-hour instance timer (instance_ulduar.cpp:774/1009, ulduar.h:499), not
-- combat — no bridge.
-- npc_brann_bronzebeard_algalon (34064 entry-verifiable, ulduar.h line
-- 230) — Talk(SAY_BRANN_ALGALON_INTRO_1 0) (EVENT_BRANN_SAY_INTRO_1),
-- Talk(SAY_BRANN_ALGALON_INTRO_2 1) (DoAction ACTION_INTRO_2),
-- Talk(SAY_BRANN_ALGALON_OUTRO 2) (EVENT_BRANN_OUTRO_1) — DoAction /
-- MovementInform / timer-gated; no bridges; no registration.
-- npc_collapsing_starAI DamageTaken — the _dying latch damage=0 rewrite +
-- SPELL_BLACK_HOLE_SPAWN_VISUAL / SPELL_SUMMON_BLACK_HOLE self-casts —
-- no Talk arm; the casts themselves are bridgeable (triggered
-- self-cast), but the star is never summoned in Go
-- (EVENT_SUMMON_COLLAPSING_STAR unported — no summon bridge), so the
-- arm stays out on trigger-path grounds; no registration
-- (NPC_COLLAPSING_STAR 32955 entry-verifiable, ulduar.h line 234).
-- npc_living_constellation / npc_black_hole — no Talk arms; no
-- registrations (NPC_LIVING_CONSTELLATION 33052 / NPC_BLACK_HOLE 32953
-- entry-verifiable, ulduar.h lines 232 / 235).
-- go_celestial_planetarium_access (GameObjectScript) — no GO bridge;
-- documented only.
-- The ten spell scripts (spell_algalon_phase_punch /
-- spell_algalon_phase_constellation / spell_algalon_trigger_3_adds /
-- spell_algalon_collapse / spell_algalon_big_bang /
-- spell_algalon_remove_phase / spell_algalon_cosmic_smash /
-- spell_algalon_cosmic_smash_damage / spell_algalon_supermassive_fail /
-- spell_algalon_black_hole_phase_shifts) — SpellScript / AuraScript — no
-- SpellScript / AuraScript bridges; all join the no-SpellScript /
-- no-AuraScript-bridge queues.
-- achievement_he_feeds_on_your_tears (OnCheck: GetData
-- DATA_HAS_FED_ON_TEARS) — no GetData / achievement bridges; joins the
-- unmodeled-achievement queue.

local ENTRY_ALGALON = 32871

local SAY_ALGALON_PHASE_TWO = 11
local SAY_ALGALON_KILL = 20

-- Per-creature latch mirroring the AI members: _phaseTwo (one-shot at the
-- 20% threshold) and _hasYelled (the 1s kill-yell rate limit). C++
-- Reset() -> Initialize() clears both; event 23 does the same here.
local algalonState = {}

local function getState(guid)
    local state = algalonState[guid]
    if state == nil then
        state = { phaseTwo = false, killYellUntil = 0 }
        algalonState[guid] = state
    end
    return state
end

-- C++ DamageTaken: if (!_phaseTwo && me->HealthBelowPctDamaged(20,
-- damage)) { _phaseTwo = true; Talk(SAY_ALGALON_PHASE_TWO); <unbridgeable
-- despawn/cancel/summon legs> } — the moroes threshold+latch precedent.
-- The _fightWon 2.5% branch has no Talk arm and its damage=0 rewrite
-- rides the unbridgeable outro machine — deliberately not modeled.
local function algalonDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local state = getState(guid)
    if state.phaseTwo then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if (creature:GetHealth() - damage) < math.floor(maxHealth * 20 / 100) then
        state.phaseTwo = true
        creature:Talk(SAY_ALGALON_PHASE_TWO)
    end
end

-- C++ KilledUnit: if (victim->GetTypeId() == TYPEID_PLAYER) {
-- _fedOnTears = true; if (!_hasYelled) { _hasYelled = true;
-- events.ScheduleEvent(EVENT_UNLOCK_YELL, 1s); Talk(SAY_ALGALON_KILL); } }
-- — the razuvious player-gated variant precedent; the 1s rate limit is
-- modeled with an os.time() deadline (the malchezaar os.time-deadline
-- precedent; re-yell unlocks on the next whole second).
local function algalonTargetDied(event, creature, victim)
    if not victim or victim:GetObjectType() ~= "Player" then
        return
    end
    local state = getState(creature:GetGUID())
    local now = os.time()
    if now >= state.killYellUntil then
        state.killYellUntil = now + 1
        creature:Talk(SAY_ALGALON_KILL)
    end
end

local function algalonReset(event, creature)
    algalonState[creature:GetGUID()] = nil
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_ALGALON, 3, algalonTargetDied)
RegisterCreatureEvent(ENTRY_ALGALON, 9, algalonDamageTaken)
RegisterCreatureEvent(ENTRY_ALGALON, 23, algalonReset)

-- Timer-driven cast arms (the maiden CreateLuaEvent convention —
-- GUID-keyed timers table, cancel on events 2/4/23;
-- CastSpell(nil, spell) = DoCastVictim victim-cast,
-- CastSpell(creature, spell) = DoCastAOE/DoCastSelf self-cast). All four
-- boss arms are scheduled in JustEngagedWith with NO phase/group args —
-- valid in all phases (phase-arg arms like the voice-of-Yogg-Saron kit are
-- the gothik-blocked kind; these are not). DOCUMENTED DEVIATIONS: (1)
-- introDelay is 26500ms on true first pull / 8s on later pulls
-- (:364-386); the _firstPull flag has no bridge, so the re-pull 8s path is
-- used — timers fire ~18.5s early on a genuine first pull. (2)
-- EVENT_ASCEND_TO_THE_HEAVENS schedules EVENT_EVADE 2.5s later
-- (EnterEvadeMode — boss-state FAIL, no bridge); only the Talk+cast arm is
-- ported, so the lua boss stays in combat ~2.5s past enrage. (3)
-- EVENT_SUMMON_COLLAPSING_STAR (Talk 5/6 + four SummonCreature legs) is
-- not ported: the summon leg has no bridge (the arlokk precedent) and a
-- Talk-only port would fire with no stars. (4) EVENT_BIG_BANG stays out:
-- the arm sets PHASE_BIG_BANG, orders constellation DoActions via a
-- grid search and reschedules the phase restore — phase-machine legs block
-- it (gothik precedent). (5) npc_living_constellation's
-- EVENT_ARCANE_BARRAGE carries SPELLVALUE_MAX_TARGETS=1 which the
-- three-arg CastSpell bridge cannot express (documented, same class as
-- brightleaf's flux-stacks deviation).
local SPELL_QUANTUM_STRIKE = 64395
local SPELL_PHASE_PUNCH = 64412
local SPELL_ASCEND_TO_THE_HEAVENS = 64487
local SPELL_COSMIC_SMASH = 62301
local SPELL_ARCANE_BARRAGE = 64599

local ENTRY_LIVING_CONSTELLATION = 33052

local SAY_ALGALON_ASCEND = 9
local EMOTE_ALGALON_COSMIC_SMASH = 10

-- Re-pull introDelay (:376); first-pull 26500ms unbridged — see (1) above.
local INTRO_DELAY = 8000

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

-- C++ boss_algalon_the_observerAI::EVENT_QUANTUM_STRIKE:
-- DoCastVictim(SPELL_QUANTUM_STRIKE 64395) (non-triggered),
-- 3500ms+introDelay init, Repeat(3s, 5s) (:392, :586-589 verified).
local function quantumStrikeTick(creature, guid)
    creature:CastSpell(nil, SPELL_QUANTUM_STRIKE)
    schedule(guid, "quantumstrike", {3000, 5000}, function()
        quantumStrikeTick(creature, guid)
    end)
end

-- C++ EVENT_PHASE_PUNCH: DoCastVictim(SPELL_PHASE_PUNCH 64412)
-- (non-triggered), 15500ms+introDelay init, Repeat(15500ms)
-- (:393, :590-593 verified).
local function phasePunchTick(creature, guid)
    creature:CastSpell(nil, SPELL_PHASE_PUNCH)
    schedule(guid, "phasepunch", 15500, function()
        phasePunchTick(creature, guid)
    end)
end

-- C++ EVENT_COSMIC_SMASH: Talk(EMOTE_ALGALON_COSMIC_SMASH 10) +
-- DoCastAOE(SPELL_COSMIC_SMASH 62301) (non-triggered), 25s+introDelay
-- init, Repeat(25500ms) (:397, :626-630 verified).
local function cosmicSmashTick(creature, guid)
    creature:Talk(EMOTE_ALGALON_COSMIC_SMASH)
    creature:CastSpell(creature, SPELL_COSMIC_SMASH)
    schedule(guid, "cosmicsmash", 25500, function()
        cosmicSmashTick(creature, guid)
    end)
end

-- C++ EVENT_ASCEND_TO_THE_HEAVENS: Talk(SAY_ALGALON_ASCEND 9) +
-- DoCastAOE(SPELL_ASCEND_TO_THE_HEAVENS 64487) (non-triggered),
-- 360s+introDelay init, one-shot (:396, :618-622 verified).
local function ascendTick(creature, guid)
    creature:Talk(SAY_ALGALON_ASCEND)
    creature:CastSpell(creature, SPELL_ASCEND_TO_THE_HEAVENS)
end

local function algalonTimersStart(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "quantumstrike", 3500 + INTRO_DELAY, function()
        quantumStrikeTick(creature, guid)
    end)
    schedule(guid, "phasepunch", 15500 + INTRO_DELAY, function()
        phasePunchTick(creature, guid)
    end)
    schedule(guid, "cosmicsmash", 25000 + INTRO_DELAY, function()
        cosmicSmashTick(creature, guid)
    end)
    schedule(guid, "ascend", 360000 + INTRO_DELAY, function()
        ascendTick(creature, guid)
    end)
end

local function algalonTimersStop(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ npc_living_constellationAI::EVENT_ARCANE_BARRAGE:
-- DoCastAOE(SPELL_ARCANE_BARRAGE 64599) with SPELLVALUE_MAX_TARGETS=1
-- (non-triggered; the extra-args form, not the triggered bool), 2.5s init,
-- Repeat(4s, 5s) — scheduled in Reset with no phase arg (:732, :794-800
-- verified). The max-targets leg is unbridgeable — documented above.
local function arcaneBarrageTick(creature, guid)
    creature:CastSpell(creature, SPELL_ARCANE_BARRAGE)
    schedule(guid, "arcanebarrage", {4000, 5000}, function()
        arcaneBarrageTick(creature, guid)
    end)
end

local function constellationEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "arcanebarrage", 2500, function()
        arcaneBarrageTick(creature, guid)
    end)
end

RegisterCreatureEvent(ENTRY_ALGALON, 1, algalonTimersStart)
RegisterCreatureEvent(ENTRY_ALGALON, 2, algalonTimersStop)
RegisterCreatureEvent(ENTRY_ALGALON, 4, algalonTimersStop)
RegisterCreatureEvent(ENTRY_LIVING_CONSTELLATION, 1, constellationEnterCombat)
RegisterCreatureEvent(ENTRY_LIVING_CONSTELLATION, 2, algalonTimersStop)
RegisterCreatureEvent(ENTRY_LIVING_CONSTELLATION, 4, algalonTimersStop)
RegisterCreatureEvent(ENTRY_LIVING_CONSTELLATION, 23, algalonTimersStop)
