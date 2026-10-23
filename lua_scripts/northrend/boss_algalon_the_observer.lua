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
-- — the event-3 slay (os.time() 1s rate-limit deadline), event-9 phase-two
-- threshold+latch, JustEngagedWith intro-Talk/_firstPull and the combat
-- timer arms are C++-exact.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 9 OnDamageTaken, 23 OnReset.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith intro Talks — the _firstPull flag (set true once in the
-- AI ctor, :291; never cleared by Reset()/Initialize(), :293-296) chooses
-- Talk(SAY_ALGALON_START_TIMER 3) + 26500ms intro (first pull, :377-387)
-- vs Talk(SAY_ALGALON_AGGRO 4) + 8s intro (later pulls, :370-376); mirrored
-- as a per-GUID Lua latch (script-lifetime, NOT wiped by event 23 —
-- C++-exact). First pull also schedules EVENT_START_COMBAT (16s, :554-559)
-- -> Talk(SAY_ALGALON_AGGRO 4) + PlayDirectMusic(ENGAGE_MUSIC_ID 15878)
-- (ported as creature:PlayMusic — the bridged SMSG_PLAY_MUSIC twin;
-- the instance->SetBossState(IN_PROGRESS) companion leg has no bridge).
-- The remaining JustEngagedWith legs (the later-pull SetFlag
-- NOT_SELECTABLE, SetImmuneToNPC, summons.DespawnEntry(NPC_AZEROTH),
-- instance->SetData, BossAI::JustEngagedWith passthrough)
-- have no bridges; InterruptNonMeleeSpells(:381) is dropped — StopSpellCast
-- is an explicit no-op for creatures (lua_objects.go:332).
-- (CORRECTION 2026-10-23: the :384 first-pull DoZoneInCombat leg IS
-- ported below — creature:DoZoneInCombat() is bridged
-- (engine/world/lua_creature_events.go); the stale "no bridge" claim
-- is retracted for that leg.)
-- (CORRECTION 2026-10-22: the old "PlayDirectMusic has no bridge" claims
-- were stale — creature:PlayMusic IS bridged (lua_creature_events.go:118);
-- both engage music arms (:373 later pulls, :557 EVENT_START_COMBAT) are
-- ported below.)
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
-- DOCUMENTED-ONLY (in this header; ENTRY_LIVING_CONSTELLATION 33052 is
-- registered too — entry-verifiable):
-- DamageTaken _fightWon branch — (health - damage) below 2.5% of max:
-- damage = 0 + the outro event machine (REACT_PASSIVE, AttackStop,
-- SPELL_SELF_STUN self-cast, summons.DespawnAll, EVENT_OUTRO_START ...)
-- — no Talk arm in this branch; a lone damage=0 return would strand the
-- boss unkillable without the outro, so it is not modeled.
-- Timer Talks — 2026-10-14 retraction re-audit (C++ line refs): algalon's
-- own quantum strike / phase punch / cosmic smash / ascend arms ARE ported
-- on combat-start timers (event 1, C++-exact inits :392-397, header
-- corrected 2026-10-16); every other arm rides an unbridged machine
-- (stale "no-timer-bridge" claim RETRACTED — CreateLuaEvent is a real
-- bridge; the blocker is machine gating, gothik precedent). The
-- constellation's Arcane Barrage is also ported on an enter-combat timer
-- (event 1): 2.5s init + {4s,5s} repeat C++-exact (:732) but the Lua anchor
-- is EnterCombat where C++ anchors at Reset — anchor deviation noted.
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
-- constellation summons) except the SetReactState(REACT_DEFENSIVE) (:566)
-- leg, which IS ported on the introDelay timer in algalonTimersStart;
-- Talk(AGGRO 4) fires in JustEngagedWith
-- non-first-pull (:372) and EVENT_START_COMBAT (:555, 16s, first-pull
-- only) — both now ported via the _firstPull latch above.
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
-- self-cast), but the summoned star AI has no registration yet
-- (the summon leg itself IS ported in collapsingStarTick — the stale
-- "no summon bridge" claim is retracted); the star arms stay out on
-- registration grounds (NPC_COLLAPSING_STAR 32955 entry-verifiable,
-- ulduar.h line 234).
-- npc_living_constellation / npc_black_hole — no Talk arms; no
-- registrations (NPC_LIVING_CONSTELLATION 33052 / NPC_BLACK_HOLE 32953
-- entry-verifiable, ulduar.h lines 232 / 235).
-- npc_living_constellation DoAction(ACTION_ACTIVATE_STAR) (:745-750):
-- SetReactState(REACT_AGGRESSIVE) + RemoveFlag(NOT_SELECTABLE) +
-- AttackStart + DoZoneInCombat — fired by the
-- spell_algalon_trigger_3_adds SpellScript (:1106-1112); no DoAction /
-- SpellScript bridges, so the constellation's react flip stays out
-- (its Arcane Barrage enter-combat timer above is the only modeled arm).
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

local SAY_ALGALON_START_TIMER = 3
local SAY_ALGALON_AGGRO = 4
local SAY_ALGALON_PHASE_TWO = 11
local SAY_ALGALON_KILL = 20
-- C++ ENGAGE_MUSIC_ID (boss_algalon_the_observer.cpp:64) — engage music,
-- played via PlayDirectMusic on every engage (:373 later pulls, :557
-- first-pull EVENT_START_COMBAT); creature:PlayMusic is the bridged twin
-- (SMSG_PLAY_MUSIC, lua_creature_events.go:118).
local ENGAGE_MUSIC_ID = 15878

-- Per-creature latch mirroring the AI members: _phaseTwo (one-shot at the
-- 20% threshold) and _hasYelled (the 1s kill-yell rate limit). C++
-- Reset() -> Initialize() clears both; event 23 does the same here.
local algalonState = {}

-- C++ _firstPull (boss_algalon_the_observer.cpp:291): set true once in the
-- AI ctor, never cleared by Reset()/Initialize() (:293-296) — mirrored as
-- a per-GUID latch that event 23 deliberately does NOT wipe (C++-exact);
-- event 4 (death -> fresh AI on respawn) clears it back to first-pull.
-- Absent key = true (never pulled since script load, like a fresh AI).
local algalonFirstPull = {}

local function getState(guid)
    local state = algalonState[guid]
    if state == nil then
        state = { phaseTwo = false, killYellUntil = 0 }
        algalonState[guid] = state
    end
    return state
end

local timers = {}

-- C++ DamageTaken: if (!_phaseTwo && me->HealthBelowPctDamaged(20,
-- damage)) { _phaseTwo = true; Talk(SAY_ALGALON_PHASE_TWO); <unbridgeable
-- despawn/summon legs> } — the moroes threshold+latch precedent.
-- The EVENT_SUMMON_COLLAPSING_STAR cancel (boss_algalon_the_
-- observer.cpp:473) is ported: the "collapsingstar" schedule key is
-- cancelled one-shot (the summons.DespawnEntry constellation/star/
-- black-hole/stalker legs + worm-hole DoSummon machine stay out —
-- documented only).
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
        local per = timers[guid]
        if per and per["collapsingstar"] then
            RemoveEventById(per["collapsingstar"])
            per["collapsingstar"] = nil
        end
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

local function algalonReset(event, creature)
    algalonState[creature:GetGUID()] = nil
    cancelTimers(creature:GetGUID())
    -- EnterEvadeMode (boss_algalon_the_observer.cpp:449-455): instance
    -- SetBossState(FAIL) has no bridge, but the SetSheath(UNARMED) leg is
    -- ported — Algalon drew his weapon at the intro timer (:563 MELEE).
    creature:SetSheath(0) -- SHEATH_STATE_UNARMED
    -- C++ boss_algalon_the_observer.cpp:305: me->SetReactState(REACT_PASSIVE)
    -- (Reset arm order: _Reset -> flip -> Initialize).
    creature:SetReactState(0) -- REACT_PASSIVE
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
-- introDelay is selected by the _firstPull latch (26500ms first pull /
-- 8s later pulls, :364-386) — the 2026-10-16 "re-pull 8s path always"
-- deviation is superseded by the latch port above. (2)
-- EVENT_ASCEND_TO_THE_HEAVENS schedules EVENT_EVADE 2.5s later
-- (EnterEvadeMode — boss-state FAIL, no bridge); only the Talk+cast arm is
-- ported, so the lua boss stays in combat ~2.5s past enrage. The evade
-- SetSheath(SHEATH_STATE_UNARMED) leg IS ported (algalonReset, bridge
-- landed 2026-10-20). (3)
-- (3) EVENT_SUMMON_COLLAPSING_STAR (Talk 5/6 + four SummonCreature legs)
-- is PORTED (2026-10-23, creature:SummonCreature bridge — 'no summon
-- bridge' retracted) — collapsingstar key cancel on phase-2. (4)
-- EVENT_BIG_BANG stays out:
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
local SAY_ALGALON_COLLAPSING_STAR = 5
local EMOTE_ALGALON_COLLAPSING_STAR = 6

-- boss_algalon_the_observer.cpp:221-226 (NPC_COLLAPSING_STAR 32955,
-- ulduar.h line 234).
local NPC_COLLAPSING_STAR = 32955
local COLLAPSING_STAR_POSITIONS = {
    {1649.438, -319.8127, 418.3941, 1.082104},
    {1647.005, -288.6790, 417.3955, 3.490659},
    {1622.451, -321.1563, 417.6188, 4.677482},
    {1615.060, -291.6816, 417.7796, 3.490659},
}

-- Later-pull introDelay (:376); the first-pull 26500ms path is selected
-- by the _firstPull latch in algalonTimersStart above.
local INTRO_DELAY = 8000

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

-- C++ EVENT_SUMMON_COLLAPSING_STAR: Talk(SAY_ALGALON_COLLAPSING_
-- STAR 5) + Talk(EMOTE_ALGALON_COLLAPSING_STAR 6) + 4x SummonCreature(
-- NPC_COLLAPSING_STAR 32955, CollapsingStarPos literals,
-- TEMPSUMMON_CORPSE_DESPAWN Eluna type 4), 18s+introDelay init,
-- Repeat(1min) (:394, :594-600 verified). The stale "no summon
-- bridge" claim is retracted for this leg. Delta: summoned star AIs
-- are unported — the stars are inert.
local function collapsingStarTick(creature, guid)
    creature:Talk(SAY_ALGALON_COLLAPSING_STAR)
    creature:Talk(EMOTE_ALGALON_COLLAPSING_STAR)
    for _, pos in ipairs(COLLAPSING_STAR_POSITIONS) do
        creature:SummonCreature(NPC_COLLAPSING_STAR, pos[1], pos[2], pos[3], pos[4], 4, 0)
    end
    schedule(guid, "collapsingstar", 60000, function()
        collapsingStarTick(creature, guid)
    end)
end

-- C++ JustEngagedWith (boss_algalon_the_observer.cpp:362-386): the
-- _firstPull latch chooses the arm. First pull (:377-387):
-- Talk(SAY_ALGALON_START_TIMER 3), introDelay = 26500ms,
-- EVENT_START_COMBAT scheduled in 16s. Later pulls (:370-376):
-- Talk(SAY_ALGALON_AGGRO 4), introDelay = 8s. The combat timer arms
-- below ride the same introDelay (the maiden CreateLuaEvent convention).
local function algalonTimersStart(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    local introDelay
    if algalonFirstPull[guid] == nil then
        algalonFirstPull[guid] = false
        creature:Talk(SAY_ALGALON_START_TIMER)
        -- C++ JustEngagedWith first-pull (boss_algalon_the_observer.cpp:383-384):
        -- me->SetFarVisible(true), then DoZoneInCombat() (the
        -- InterruptNonMeleeSpells/setActive legs have no bridge).
        creature:SetFarVisible(true)
        creature:DoZoneInCombat() -- C++ :384, in arm order after SetFarVisible (bridge live 2026-10-23)
        introDelay = 26500
        -- C++ EVENT_START_COMBAT (:554-559, first-pull only, 16s):
        -- Talk(SAY_ALGALON_AGGRO 4) + PlayDirectMusic(ENGAGE_MUSIC_ID)
        -- (the instance->SetBossState(IN_PROGRESS) leg has no bridge).
        -- cancelTimers above mirrors the C++ events.Reset() (:366),
        -- which also wipes a pending START_COMBAT on re-pull.
        schedule(guid, "startcombat", 16000, function()
            creature:Talk(SAY_ALGALON_AGGRO)
            creature:PlayMusic(ENGAGE_MUSIC_ID)
        end)
    else
        -- C++ JustEngagedWith later-pull leg (:370-376):
        -- Talk(SAY_ALGALON_AGGRO 4) + PlayDirectMusic(ENGAGE_MUSIC_ID).
        creature:Talk(SAY_ALGALON_AGGRO)
        creature:PlayMusic(ENGAGE_MUSIC_ID)
        introDelay = INTRO_DELAY
    end
    -- C++ EVENT_INTRO_TIMER_DONE (:560-584): SetPhase(PHASE_NORMAL),
    -- SetSheath(SHEATH_STATE_MELEE), RemoveFlag(NOT_SELECTABLE),
    -- SetImmuneToNPC(false), SetReactState(REACT_DEFENSIVE) (:566),
    -- DoCastAOE(SUPERMASSIVE_FAIL) + the AttackStart workaround +
    -- the REACT_PASSIVE constellation summons. Only the react flip is
    -- modeled — the rest rides the unported intro machinery. Without
    -- it the boss keeps the algalonReset REACT_PASSIVE while engaged
    -- and the engine's isCreaturePassive gate never lets it attack.
    schedule(guid, "introdefensive", introDelay, function()
        creature:SetReactState(1) -- REACT_DEFENSIVE
    end)
    schedule(guid, "quantumstrike", 3500 + introDelay, function()
        quantumStrikeTick(creature, guid)
    end)
    schedule(guid, "phasepunch", 15500 + introDelay, function()
        phasePunchTick(creature, guid)
    end)
    schedule(guid, "cosmicsmash", 25000 + introDelay, function()
        cosmicSmashTick(creature, guid)
    end)
    schedule(guid, "ascend", 360000 + introDelay, function()
        ascendTick(creature, guid)
    end)
    schedule(guid, "collapsingstar", 18000 + introDelay, function()
        collapsingStarTick(creature, guid)
    end)
end

local function algalonTimersStop(event, creature)
    cancelTimers(creature:GetGUID())
    -- C++ _firstPull lives on the AI object: death -> despawn -> respawn
    -- constructs a fresh AI with _firstPull = true (boss_algalon_the_
    -- observer.cpp:291). Wipes (event 2) keep the latch; only death
    -- re-arms the first-pull intro. Gated on entry — the shared handler
    -- also serves the living constellation.
    if event == 4 and creature:GetEntry() == ENTRY_ALGALON then
        algalonFirstPull[creature:GetGUID()] = nil
    end
end

-- C++ npc_living_constellationAI::EVENT_ARCANE_BARRAGE:
-- DoCastAOE(SPELL_ARCANE_BARRAGE 64599) with SPELLVALUE_MAX_TARGETS=1
-- (non-triggered; the extra-args form, not the triggered bool), 2.5s init,
-- Repeat(4s, 5s) — scheduled in Reset with no phase arg (:732, :794-800
-- verified). Lua anchors the first schedule at EnterCombat (event 1),
-- C++ at Reset — value C++-exact, anchor deviation documented. The
-- max-targets leg is unbridgeable — documented above.
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
