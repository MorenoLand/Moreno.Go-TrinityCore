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
-- seventeen. No algalon lua existed.
-- Eluna creature events: 3 OnTargetDied, 9 OnDamageTaken, 23 OnReset.
-- Ported arms (C++-exact for all modeled arms):
-- KilledUnit — player-gated Talk(SAY_ALGALON_KILL 20) (event 3;
-- who->GetTypeId() == TYPEID_PLAYER — the razuvious player-gated
-- variant precedent — the thirty-fifth player-gated variant ported);
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
-- Timer Talks — Talk(SAY_ALGALON_INTRO_1 0) (EVENT_INTRO_1),
-- Talk(SAY_ALGALON_INTRO_2 1) (EVENT_INTRO_2),
-- Talk(SAY_ALGALON_INTRO_3 2) (EVENT_INTRO_3),
-- Talk(SAY_ALGALON_AGGRO 4) (EVENT_INTRO_TIMER_DONE, first pull),
-- Talk(SAY_ALGALON_COLLAPSING_STAR 5) +
-- Talk(EMOTE_ALGALON_COLLAPSING_STAR 6) (EVENT_SUMMON_COLLAPSING_STAR),
-- Talk(SAY_ALGALON_BIG_BANG 7) + Talk(EMOTE_ALGALON_BIG_BANG 8)
-- (EVENT_BIG_BANG), Talk(SAY_ALGALON_ASCEND 9)
-- (EVENT_ASCEND_TO_THE_HEAVENS),
-- Talk(EMOTE_ALGALON_COSMIC_SMASH 10) (EVENT_COSMIC_SMASH), the outro
-- Talks Talk(SAY_ALGALON_OUTRO_1..5 12..16) (EVENT_OUTRO_6..10) and the
-- despawn Talks Talk(SAY_ALGALON_DESPAWN_1..3 17..19)
-- (EVENT_DESPAWN_ALGALON_1..3) — the timer event machine has no bridge;
-- all join the no-timer-bridge queue.
-- npc_brann_bronzebeard_algalon (34064 entry-verifiable, ulduar.h line
-- 230) — Talk(SAY_BRANN_ALGALON_INTRO_1 0) (EVENT_BRANN_SAY_INTRO_1),
-- Talk(SAY_BRANN_ALGALON_INTRO_2 1) (DoAction ACTION_INTRO_2),
-- Talk(SAY_BRANN_ALGALON_OUTRO 2) (EVENT_BRANN_OUTRO_1) — DoAction /
-- MovementInform / timer-gated; no bridges; no registration.
-- npc_collapsing_starAI DamageTaken — the _dying latch damage=0 rewrite +
-- SPELL_BLACK_HOLE_SPAWN_VISUAL / SPELL_SUMMON_BLACK_HOLE self-casts —
-- no Talk arm; no bridges; no registration (NPC_COLLAPSING_STAR 32955
-- entry-verifiable, ulduar.h line 234).
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
    if (creature:GetHealth() - damage) * 100 / maxHealth < 20 then
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
end

RegisterCreatureEvent(ENTRY_ALGALON, 3, algalonTargetDied)
RegisterCreatureEvent(ENTRY_ALGALON, 9, algalonDamageTaken)
RegisterCreatureEvent(ENTRY_ALGALON, 23, algalonReset)
