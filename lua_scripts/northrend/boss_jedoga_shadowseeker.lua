-- Jedoga Shadowseeker (Ahn'kahet) — Lua port of
-- src/server/scripts/Northrend/AzjolNerub/Ahnkahet/boss_jedoga_shadowseeker.cpp
-- (four scripts — boss_jedoga_shadowseeker (BossAI),
-- npc_twilight_volunteer (ScriptedAI),
-- spell_random_lightning_visual_effect (SpellScript),
-- achievement_volunteer_work (AchievementCriteriaScript)).
-- Ahn'kahet dungeon-script unit per northrend_script_loader.cpp order
-- (decl 35 / call 225, next: AddSC_boss_volazj();
-- the Ahn'kahet dungeon boss roster closes with that unit, then
-- instance_ahnkahet (decl 37 / call 227):
-- elder_nadox 29309, taldaram 29308, amanitar 30258,
-- volazj 29311, jedoga 29310).
-- Entry: 29310 Jedoga Shadowseeker (ahnkahet.h
-- NPC_JEDOGA_SHADOWSEEKER — kalecgos pass; the
-- RegisterAhnKahetCreatureAI ScriptName binding is instance-shimmed,
-- the creature_template binding DB-side as usual). Sole-source
-- verified: whole-server-tree grep for "boss_jedoga_shadowseeker",
-- "npc_twilight_volunteer", "spell_random_lightning_visual_effect"
-- and "achievement_volunteer_work" hits boss_jedoga_shadowseeker.cpp
-- only (loader carries only the decl/call lines); zero sql/ hits.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms):
-- boss_jedoga_shadowseeker: Reset controller summons (event 23,
-- IsAlive-guarded — Go 23 also fires on death) + beam-visual casts
-- (jedogaReset; SummonCreatureGroup(INITIATES) PORTED 2026-11-05;
-- the phase / react legs stay below);
-- JustEngagedWith Talk(SAY_AGGRO 0)
-- (event 1 — the hook itself is bridged); the
-- RemoveAurasDueToSpell(SPELL_SPHERE_VISUAL 56075) /
-- RemoveAurasDueToSpell(SPELL_RANDOM_LIGHTNING_VISUAL 56327) legs are
-- PORTED (stale "no aura-removal bridge" claim retracted —
-- creature:RemoveAura bridged); SummonCreatureGroup(
-- SUMMON_GROUP_WORSHIPPERS) PORTED 2026-11-05 in jedogaEnterCombat
-- (bridge landed), events.SetPhase(PHASE_ONE) (no EventMap phase
-- bridge — the Lua-side phase state below stands in for it), and
-- the 25 NPC_TWILIGHT_VOLUNTEER 30385 summons with
-- MoveTo(POINT_INITIAL_POSITION) choreography are PORTED 2026-11-13
-- in jedogaEnterCombat (summon + motion bridges live);
-- KilledUnit Talk(SAY_SLAY 3) player-gated (event 3,
-- victim:GetObjectType()=="Player" — nalorakk precedent);
-- JustDied Talk(SAY_DEATH 4) (event 4; _JustDied instance
-- bookkeeping ports in jedogaDied below — SetBossState + flag).
-- DamageTaken 55% flip + the whole phase-two flying-sacrifice
-- machine + the volunteer AI — PORTED 2026-11-13 (ev9 / ev28 /
-- ev38; see the phase-two notes below).
-- Cast-state VERIFIED gate-less for ported arms: the boss's own
-- UpdateAI pre-loop (:302) + in-loop (:385) UNIT_STATE_CASTING
-- gates exist in C++, but they guard only the unported
-- phase-machine events (intro/cyclone/lightning/thundershock/
-- sacrifice sequence) — no ported arm fires through UpdateAI, so
-- no gate is ported; Talk arms (events 1/3/4) and Reset stay
-- ungated per C++.
-- Unmodeled (no bridges — documented, not wired):
-- Reset intro machine: _Reset() (instance-script model absent —
-- standing blocker), events.SetPhase(PHASE_INTRO) (no phase
-- bridge), SetReactState(REACT_PASSIVE) (react-state bridge exists
-- since 2026-10-20 — the arm stays unported because the intro Reset is
-- instance/phase-gated, drakkari_colossus precedent), SummonCreatureGroup(
-- SUMMON_GROUP_INITIATES) PORTED 2026-11-05 in jedogaReset (bridge
-- landed), and
-- EVENT_INTRO_SAY (Talk(SAY_PREACHING 5), Minutes(2) repeat —
-- instance->GetBossState(DATA_PRINCE_TALDARAM) == DONE gate needs
-- the absent instance-script model).
-- The whole combat rotation is phase-gated through
-- events.SetPhase / IsInPhase (PHASE_INTRO / PHASE_ONE / PHASE_TWO
-- / PHASE_THREE); the phase flips are driven by the Lua-side phase
-- state below (no EventMap phase bridge on the Lua surface —
-- gal_darah / moorabi precedent). The DamageTaken 55% flip and the
-- phase-two flying-sacrifice machine are PORTED 2026-11-13 (ev9 /
-- ev28 / ev38); the intro machine and the phase-one rotation
-- (Cyclone Strike / Lightning Bolt / Thundershock) stay
-- documented-only — ungated engage timers would over-cast versus
-- C++ (e.g. Cyclone Strike firing during the intro, before the
-- initiates are killed — the intro gating alone makes the
-- engage-timer reading unfaithful).
-- EVENT_CYCLONE_STRIKE — DoCastSelf(Cyclone Strike 56855),
-- MovementInform(POINT_GROUND)-scheduled 3s init, 15-30s repeat;
-- bridged in isolation (creature:CastSpell(creature, spell) —
-- moorabi precedent) but gated as above — documented-only.
-- EVENT_LIGHTNING_BOLT — SelectTarget(SelectTargetMethod::Random,
-- 0, 100.0f, true) -> DoCast(target, Lightning Bolt 56891), 7s
-- init, 15-30s repeat; EVENT_THUNDERSHOCK — same selector ->
-- DoCast(target, Thundershock 56926), 12s init, 15-30s repeat: the
-- "random-target SelectTarget bridge absent (cairne/kazzak)" claim is
-- STALE (GetAITargets is the bridge, tharon_ja precedent) — the
-- blocker is gating, not bridge: both are POINT_GROUND-scheduled
-- phase-one rotation arms, and ungated engage timers would over-cast
-- during the unbridged intro/phase-two machines (gothik rule) —
-- documented-only.
-- DamageTaken 55% health crossing — PORTED 2026-11-13 (event 9,
-- jedogaDamageTaken): pre-damage HealthBelowPct(55) via the ionar
-- GetHealth()/GetMaxHealth() floor idiom, phase-one gated on the
-- Lua-side phase state, one-shot; events.Reset() cancels no Lua
-- timers (the phase-one rotation is unported). The old "no
-- health-pct bridge (doomwalker precedent)" claim was stale
-- (event 9 carries pre-damage damage; creature:GetHealthPct() is
-- bridged, scripting/object.go:209) and the "unbridged phase-two
-- machine" claim is retracted — the machine is ported below.
-- Phase-two machine — PORTED 2026-11-13 (jedogaStartPhaseTwo /
-- jedogaFlyDelay / jedogaChooseVolunteer / jedogaEndPhaseTwo /
-- jedogaSummonVolunteer + boss ev28/ev38 + volunteer ev28/ev38):
-- EVENT_START_PHASE_TWO (SetReactState(REACT_PASSIVE), AttackStop,
-- InterruptNonMeleeSpells(true), SetFlag(NOT_SELECTABLE), MoveTo(
-- POINT_PHASE_TWO)), EVENT_FLY_DELAY (SetDisableGravity(true) +
-- MoveTakeoff(POINT_PHASE_TWO_FLY)), EVENT_CHOOSE_VOLUNTEER
-- (third-controller summon + SetFacingToObject +
-- SPELL_SACRIFICE_VISUAL 56133 self-cast + Talk(SAY_CHOOSE 1) +
-- random alive-volunteer pick + cross-AI DoAction(ACTION_CHOSEN)),
-- EVENT_SUMMON_VOLUNTEER re-summon at the sacrificed spot,
-- EVENT_END_PHASE_TWO (DespawnEntry(controller) + DoCastSelf(
-- SPELL_HOVER_FALL_2 56157) + SetDisableGravity(false) +
-- MoveLand(POINT_GROUND)). Bridges verified this run:
-- MoveTo/MoveLand/MoveTakeoff (lua_creature_events.go:5298/5333/
-- 5361, ev28 arrival POINT 8 / EFFECT 16), SetDisableGravity
-- (:3776), SetReactState (:4651), InterruptNonMeleeSpells (:2512),
-- SetFacingToObject (:5209), DoAction dispatch (:845),
-- GetInstanceCreature (:7081), SetWalk (:3956), SetStandState
-- (:4760). Deltas: (a) the phase model is Lua-side state, not
-- EventMap; (b) the CHOOSE pick filters to alive volunteers (a
-- dead C++ pick fizzles the sacrifice and strands the boss
-- flying); (c) re-summoned volunteers are not sacrificeable
-- (C++ never pushes them into _volunteerGUIDS); (d) the
-- POINT_GROUND rotation re-arm stays out (header above).
-- DoAction(ACTION_SACRIFICE) — PORTED 2026-11-13 (event 38,
-- jedogaDoAction): Talk(SAY_SACRIFICE 2) + DoCastAOE(Sacrifice
-- Beam 56150, self-cast lowering, arugal precedent) +
-- EVENT_END_PHASE_TWO 3s + EVENT_SUMMON_VOLUNTEER 15s; the sender
-- is the volunteer MovementInform POINT_SACRIFICE
-- (boss_jedoga_shadowseeker.cpp:438), now registered (ev28,
-- volunteerMovementInform).
-- JustSummoned — worshipper kneel leg stays out (SummonCreatureGroup
-- arrivals, unbridged), but the summons latch + DespawnAll legs are
-- PORTED via ev19 (jedogaJustSummoned below; controllers are direct
-- Lua summons), SummonedCreatureDies (initiate _initiatesKilled == 15
-- latch -> SPELL_HOVER_FALL_1 56100 + EVENT_START_FIGHT_1 1s
-- (RemoveAurasDueToSpell(SPELL_BEAM_VISUAL_JEDOGA 56312) then
-- summons.DespawnEntry(controller) + SetDisableGravity(false) +
-- MoveLand(POINT_GROUND)) and the volunteer _volunteerWork=false
-- latch + PHASE_THREE flip — the volunteer half is PORTED
-- 2026-11-13 in jedogaSummonedDied (phase-three flip +
-- END_PHASE_TWO 1s + SUMMON_VOLUNTEER 10s); the initiate half
-- stays out (payload rides the unported intro machine),
-- EnterEvadeMode (summons.DespawnAll PORTED
-- via the ev19 latch in jedogaLeaveCombat; the _DespawnAtEvade(15s) leg
-- is ported as immediate creature:Despawn (drakkari
-- precedent; C++ despawns immediately, the 15s is respawn-side —
-- ScriptedCreature.cpp:596-616; stale "no despawn bridge"
-- retracted),
-- GetData(DATA_VOLUNTEER_WORK) — PORTED tick 417 via event 21
-- (jedogaSummonedDied below): a Twilight Volunteer actually dying
-- writes SetData(DATA_VOLUNTEER_WORK, 0); the spawn-time write of 1
-- rides event 5 (jedogaSpawned below). A sacrificed volunteer does
-- NOT die (it kneels and despawns, boss_jedoga_shadowseeker.cpp:428-444),
-- so the sacrifice path never clears the latch — C++-exact.
-- achievement_volunteer_work — PORTED tick 417 via the SetData write
-- leg + Go gate (bossLatchData, achievement_criteria_scripts.go).
-- npc_twilight_volunteer (30385 — ahnkahet.h NPC_TWILIGHT_
-- VOLUNTEER, kalecgos pass) — PORTED 2026-11-13 (entry 30385,
-- ev28 volunteerMovementInform + ev38 volunteerDoAction):
-- DoAction(ACTION_CHOSEN) (SPELL_PILLAR_OF_LIGHTNING 56868
-- self-cast + RemoveAura(SPHERE_VISUAL_VOLUNTEER 56102) +
-- Talk(SAY_CHOSEN 0) + stand + RemoveFlag(NOT_SELECTABLE) +
-- SetWalk(true) + MoveTo(POINT_SACRIFICE)), MovementInform
-- POINT_INITIAL_POSITION (face Jedoga via GetInstanceCreature +
-- SPELL_SPHERE_VISUAL_VOLUNTEER self-cast + kneel) and
-- POINT_SACRIFICE (kneel + boss DoAction(ACTION_SACRIFICE) +
-- Talk(SAY_SACRIFICED 1) + 3s later stand-dead +
-- DespawnOrUnsummon(5s)). The old "entry-verifiable but
-- bridge-blocked" verdict is retracted — the "no motion /
-- summon / instance bridges" claims were stale (see the
-- phase-two machine note above). Stand states are
-- UnitDefines.h values: KNEEL 8, DEAD 7 (the zuljin "7 is
-- kneel" comment is wrong — 7 is DEAD).
-- spell_random_lightning_visual_effect (SpellScript 56328 — On
-- DestinationTargetSelect EFFECT_0 relocate -19z): no SpellScript
-- binding bridge (razelikh precedent) — documented-only.
-- achievement_volunteer_work (AchievementCriteriaScript — GetAI()->
-- GetData(DATA_VOLUNTEER_WORK)) — PORTED tick 417 via the SetData
-- write leg + Go gate (bossLatchData, achievement_criteria_scripts.go);
-- the "no bridge" claim is stale (ev21 + the SetData store are live).

local ENTRY_JEDOGA_SHADOWSEEKER = 29310
local DATA_JEDOGA_SHADOWSEEKER = 2 -- ahnkahet.h:33
local BOSS_STATE_DONE = 3 -- EncounterState DONE (InstanceScript.h:68-76)
local DATA_VOLUNTEER_WORK = 1 -- boss_jedoga_shadowseeker.cpp:137
local NPC_TWILIGHT_VOLUNTEER = 30385 -- ahnkahet.h:57

local SAY_AGGRO = 0
local SAY_SLAY = 3
local SAY_DEATH = 4

local SPELL_SPHERE_VISUAL = 56075            -- boss_jedoga_shadowseeker.cpp:46
local ENTRY_JEDOGA_CONTROLLER = 30181 -- ahnkahet.h:59
local SPELL_BEAM_VISUAL_JEDOGA = 56312 -- boss_jedoga_shadowseeker.cpp:53
local SPELL_RANDOM_LIGHTNING_VISUAL = 56327  -- boss_jedoga_shadowseeker.cpp:43

local SPELL_SACRIFICE_VISUAL = 56133 -- boss_jedoga_shadowseeker.cpp:54
local SPELL_SACRIFICE_BEAM = 56150 -- boss_jedoga_shadowseeker.cpp:50
local SPELL_HOVER_FALL_2 = 56157 -- boss_jedoga_shadowseeker.cpp:45
local SPELL_SPHERE_VISUAL_VOLUNTEER = 56102 -- boss_jedoga_shadowseeker.cpp:57
local SPELL_PILLAR_OF_LIGHTNING = 56868 -- boss_jedoga_shadowseeker.cpp:58

local SAY_CHOOSE = 1 -- boss_jedoga_shadowseeker.cpp:31
local SAY_SACRIFICE = 2 -- boss_jedoga_shadowseeker.cpp:32
local VOLUNTEER_SAY_CHOSEN = 0 -- boss_jedoga_shadowseeker.cpp:37
local VOLUNTEER_SAY_SACRIFICED = 1 -- boss_jedoga_shadowseeker.cpp:38

local PHASE_ONE = 1 -- JedogaPhases, boss_jedoga_shadowseeker.cpp:76
local PHASE_TWO = 2
local PHASE_THREE = 3

local ACTION_CHOSEN = 1 -- JedogaMisc, boss_jedoga_shadowseeker.cpp:133
local ACTION_SACRIFICE = 2

local POINT_INITIAL_POSITION = 0 -- JedogaPoints, boss_jedoga_shadowseeker.cpp:84
local POINT_SACRIFICE = 1
local POINT_GROUND = 2
local POINT_PHASE_TWO = 3
local POINT_PHASE_TWO_FLY = 4

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000

local REACT_PASSIVE = 0 -- UnitDefines.h
local REACT_AGGRESSIVE = 2

local POINT_MOTION_TYPE = 8 -- MotionMaster.h (MoveTo/MovePoint arrival)
local EFFECT_MOTION_TYPE = 16 -- MotionMaster.h (MoveLand/MoveTakeoff arrival)

local STAND_STAND = 0 -- UnitStandStateType, UnitDefines.h:30
local STAND_DEAD = 7
local STAND_KNEEL = 8

-- C++ boss_jedoga_shadowseeker::JustSummoned (:230-238): twilight
-- worshippers kneel + latch, every other summon latches via
-- BossAI::JustSummoned; the latch feeds EnterEvadeMode DespawnAll
-- (:182-186), _JustDied DespawnAll and the DespawnEntry(controller)
-- legs. Lua summons only the two controllers (Reset, below), so the
-- ev19 latch holds controllers; worshippers arrive via the unbridged
-- SummonCreatureGroup leg. Table defined above its first caller.
local jedogaSummons = {}

local function despawnJedogaSummons(guid)
    local latched = jedogaSummons[guid]
    if latched then
        for _, summon in ipairs(latched) do
            if summon then
                summon:Despawn()
            end
        end
        jedogaSummons[guid] = nil
    end
end

-- C++ EVENT_END_PHASE_TWO (:371-376): summons.DespawnEntry(
-- NPC_JEDOGA_CONTROLLER) removes ALL controllers, the two Reset
-- ones included — not just the sacrifice controller.
local function despawnJedogaControllers(guid)
    local latched = jedogaSummons[guid]
    if not latched then
        return
    end
    local kept = {}
    for _, summon in ipairs(latched) do
        if summon and summon:GetEntry() == ENTRY_JEDOGA_CONTROLLER then
            summon:Despawn()
        else
            kept[#kept + 1] = summon
        end
    end
    jedogaSummons[guid] = kept
end

-- Phase-two machine state, keyed by GetGUIDLow() (ionar idiom):
-- phase is the Lua-side twin of the C++ EventMap phase (PHASE_ONE
-- at engage, PHASE_TWO at the 55% flip, PHASE_THREE on a volunteer
-- death in phase two); volunteers holds the 25 engage summons by
-- spot index; selectedSpot is the sacrificed volunteer's spot.
local jedogaState = {}

-- CreateLuaEvent timers, keyed by GUID then key (ionar idiom).
-- Boss timers key on the boss GUID; the volunteer sacrifice-end
-- timer keys on the volunteer GUID; cancelAllTimers clears both.
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

local function cancelAllTimers()
    for guid, _ in pairs(timers) do
        cancelTimers(guid)
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

-- VolunteerSpotPositions (boss_jedoga_shadowseeker.cpp:104-131):
-- { spawnX, spawnY, spawnZ, destX, destY, destZ } per pair.
local VOLUNTEER_SPOTS = {
    { 400.7701, -784.8928, -31.60143, 365.9514, -719.1235, -16.17974 },
    { 397.3595, -788.5157, -31.59679, 359.7433, -715.017, -16.17974 },
    { 399.3177, -787.2599, -31.59631, 362.0263, -719.1036, -16.17974 },
    { 460.4623, -719.2227, -31.58718, 389.266, -679.3693, -16.17973 },
    { 456.0909, -724.3412, -31.58718, 400.5992, -691.7954, -16.17973 },
    { 452.6613, -726.9518, -31.58718, 400.3423, -701.5115, -16.17974 },
    { 447.8852, -732.3298, -31.58718, 389.861, -710.6993, -16.17974 },
    { 457.562, -721.1855, -31.58718, 395.4494, -684.5345, -16.17973 },
    { 451.7243, -730.2181, -31.58718, 397.0945, -708.4188, -15.99747 },
    { 413.9582, -777.132, -31.58716, 388.1394, -723.124, -15.9938 },
    { 411.5661, -781.2356, -31.58716, 381.7102, -730.0745, -15.99554 },
    { 407.395, -786.793, -31.58716, 366.9791, -737.3303, -16.17974 },
    { 404.9166, -788.3472, -31.58716, 358.6124, -735.9944, -15.9855 },
    { 401.5697, -791.2033, -31.58717, 351.9383, -729.6436, -16.17974 },
    { 410.1105, -785.4691, -31.58716, 373.1659, -736.2893, -16.17974 },
    { 442.5644, -730.2499, -31.59826, 390.5955, -714.6851, -16.17974 },
    { 445.5233, -725.9542, -31.60173, 393.9694, -708.1727, -16.17974 },
    { 448.5531, -722.5888, -31.60066, 395.2702, -702.556, -16.17974 },
    { 449.8521, -719.7265, -31.58849, 394.5757, -695.1004, -16.17974 },
    { 453.5134, -717.7018, -31.59883, 387.6152, -690.1782, -16.17974 },
    { 457.8564, -711.7424, -31.59773, 378.6874, -687.1343, -16.17973 },
    { 410.0583, -774.4119, -31.60115, 383.8151, -723.4276, -16.17974 },
    { 408.7458, -777.955, -31.59873, 376.9857, -725.0735, -16.17974 },
    { 405.2404, -779.6614, -31.60512, 373.3736, -722.7498, -16.17974 },
    { 404.0797, -783.829, -31.59497, 367.8631, -722.5212, -16.17974 },
}

-- Forward declarations (tharon_ja / gal_darah idiom: plain
-- assignment below, never re-`local function`): the phase-machine
-- functions call each other across definition order, and
-- jedogaEnterCombat (defined below) summons volunteers.
local summonVolunteer, jedogaStartPhaseTwo, jedogaFlyDelay, jedogaChooseVolunteer, jedogaEndPhaseTwo, jedogaSummonVolunteer

-- C++ JustEngagedWith (:173-180) / EVENT_SUMMON_VOLUNTEER
-- (:358-369): TEMPSUMMON_CORPSE_TIMED_DESPAWN, 3s (type 5,
-- 3000ms); the spawn Position carries no orientation (C++
-- default 0). track=true latches the volunteer into
-- state.volunteers (engage summons); the EVENT_SUMMON_VOLUNTEER
-- replacement is NOT pushed into _volunteerGUIDS in C++, so it
-- passes track=false and can never be chosen for a sacrifice.
summonVolunteer = function(creature, guid, spotIndex, track)
    local spot = VOLUNTEER_SPOTS[spotIndex]
    if not spot then
        return nil
    end
    local volunteer = creature:SummonCreature(NPC_TWILIGHT_VOLUNTEER, spot[1], spot[2], spot[3], 0, 5, 3000)
    if not volunteer then
        return nil
    end
    volunteer:SetReactState(REACT_PASSIVE) -- volunteer ctor :402
    -- NOT_SELECTABLE: the C++ DoAction(ACTION_CHOSEN) leg removes
    -- it, so the template carries it; set explicitly so the leg has
    -- something to remove even if the template row lacks the flag.
    volunteer:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    volunteer:MoveTo(POINT_INITIAL_POSITION, spot[4], spot[5], spot[6])
    if track then
        local state = jedogaState[guid]
        if state then
            state.volunteers[spotIndex] = volunteer
        end
    end
    return volunteer
end

-- C++ JustSummoned, above: latch every direct summon (halazzi idiom).
local function jedogaJustSummoned(event, creature, summon)
    if not summon then
        return
    end
    local guid = creature:GetGUIDLow()
    local latched = jedogaSummons[guid]
    if not latched then
        latched = {}
        jedogaSummons[guid] = latched
    end
    latched[#latched + 1] = summon
end

local function jedogaEnterCombat(event, creature, target)
    -- cpp:162-165 — JustEngagedWith strips the two intro visual auras before
    -- anything else (arm order C++-exact); the stale "no aura-removal
    -- bridge" claim is retracted — creature:RemoveAura is bridged
    -- (engine/world/lua_creature_events.go:155). The SummonCreatureGroup
    -- leg of the same handler is PORTED below (2026-11-05); the phase leg
    -- stays documented-only.
    creature:RemoveAura(SPELL_SPHERE_VISUAL)
    creature:RemoveAura(SPELL_RANDOM_LIGHTNING_VISUAL)
    -- C++ JustEngagedWith :166 SummonCreatureGroup(SUMMON_GROUP_WORSHIPPERS
    -- = 2), PORTED 2026-11-05 (bridge landed) — C++ order, after the aura
    -- strips, before the BossAI base arms.
    creature:SummonCreatureGroup(2)
    -- BossAI::JustEngagedWith base arms, after the aura strips and the
    -- (unported) SummonCreatureGroup leg, before Talk (C++ arm order).
    creature:SetCombatPulseDelay(5)
    creature:DoZoneInCombat()
    creature:Talk(SAY_AGGRO)
    -- C++ JustEngagedWith :170 events.SetPhase(PHASE_ONE) — the
    -- Lua-side phase state (no EventMap phase bridge); the 25
    -- volunteer summons (:173-180) follow Talk in C++ order.
    local guid = creature:GetGUIDLow()
    jedogaState[guid] = { phase = PHASE_ONE, volunteers = {}, selectedSpot = nil }
    for spotIndex = 1, #VOLUNTEER_SPOTS do
        summonVolunteer(creature, guid, spotIndex, true)
    end
end

local function jedogaTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

local function jedogaDied(event, creature, killer)
    -- C++ JustDied (:195-198): _JustDied() (DespawnAll) before Talk.
    despawnJedogaSummons(creature:GetGUIDLow())
    cancelAllTimers()
    jedogaState[creature:GetGUIDLow()] = nil
    -- C++ BossAI::_JustDied (ScriptedCreature.cpp:513-520):
    -- instance->SetBossState(DATA_JEDOGA_SHADOWSEEKER, DONE); the
    -- flag rides the same instance-consumer pattern as
    -- instance_ahnkahet.lua (no boss-state-change event exists).
    creature:SetBossState(DATA_JEDOGA_SHADOWSEEKER, BOSS_STATE_DONE)
    local mapObj = creature:GetMap()
    local data = mapObj and mapObj:GetInstanceData() or nil
    if data then
        data.jedogaShadowseekerDone = true
    end
    creature:Talk(SAY_DEATH)
end

-- C++ boss_jedoga_shadowseeker::Reset (:148-160): the two
-- NPC_JEDOGA_CONTROLLER summons + SPELL_BEAM_VISUAL_JEDOGA casts on
-- Jedoga are PORTED below, IsAlive-guarded (Go event 23 also fires on
-- death; C++ Reset never runs on death). SummonCreatureGroup(INITIATES)
-- is PORTED 2026-11-05; the phase, react-state and EVENT_INTRO_SAY legs
-- of the same Reset stay documented-only above.
local function jedogaReset(event, creature)
    if creature:IsAlive() then
        -- C++ Reset (:148-160): _Reset() (DespawnAll) before re-summoning.
        despawnJedogaSummons(creature:GetGUIDLow())
        cancelAllTimers()
        jedogaState[creature:GetGUIDLow()] = nil
        -- C++ :152 SummonCreatureGroup(SUMMON_GROUP_INITIATES = 1),
        -- PORTED 2026-11-05 (bridge landed) — C++ order, before the
        -- controller summons.
        creature:SummonCreatureGroup(1)
        local controller = creature:SummonCreature(ENTRY_JEDOGA_CONTROLLER, 402.7893, -748.5251, 29.39399, 2.024582)
        if controller then
            controller:CastSpell(creature, SPELL_BEAM_VISUAL_JEDOGA)
        end
        controller = creature:SummonCreature(ENTRY_JEDOGA_CONTROLLER, 420.1999, -727.0132, 28.88036, 2.042035)
        if controller then
            controller:CastSpell(creature, SPELL_BEAM_VISUAL_JEDOGA)
        end
    end
end

local function jedogaLeaveCombat(event, creature)
    -- C++ EnterEvadeMode: _DespawnAtEvade(Seconds(15)) (cpp:182-186;
    -- summons.DespawnAll is moot — nothing is modeled) — DespawnOrUnsummon
    -- is immediate at evade, the 15s is respawn-side
    -- (ScriptedCreature.cpp:596-616); drakkari precedent.
    -- C++ EnterEvadeMode (:182-186): summons.DespawnAll() before the
    -- evade despawn.
    despawnJedogaSummons(creature:GetGUIDLow())
    cancelAllTimers()
    jedogaState[creature:GetGUIDLow()] = nil
    creature:Despawn()
end

-- C++ ctor _volunteerWork = true (boss_jedoga_shadowseeker.cpp:145):
-- the init write rides event 5 (spawn/respawn = fresh motion, the
-- SetGuidData precedent), NOT event 23 — C++ Reset (:148-160) never
-- touches _volunteerWork, so a wipe after a volunteer died must keep
-- the latch cleared.
local function jedogaSpawned(event, creature)
    creature:SetData(DATA_VOLUNTEER_WORK, 1)
    -- C++ LoadObjectData (instance_ahnkahet.cpp:30-37): latch the
    -- boss GUID under DATA_JEDOGA_SHADOWSEEKER
    -- (instance_guid_store.go; the instance-side OnCreatureCreate
    -- route is dead for static spawns — instance_ahnkahet.lua
    -- header).
    creature:SetGuidData(DATA_JEDOGA_SHADOWSEEKER)
end

-- C++ SummonedCreatureDies (boss_jedoga_shadowseeker.cpp:253-260): a
-- summoned Twilight Volunteer dying sets _volunteerWork = false.
-- Event 21 fires on the summoner with (event, creature, summon, killer)
-- (lua_creature_events.go fireCreatureDied; amanitar precedent).
local function jedogaSummonedDied(event, creature, summon, killer)
    if summon:GetEntry() == NPC_TWILIGHT_VOLUNTEER then
        creature:SetData(DATA_VOLUNTEER_WORK, 0)
        -- C++ SummonedCreatureDies (:253-260): a volunteer dying in
        -- PHASE_TWO flips to PHASE_THREE + EVENT_END_PHASE_TWO in 1s,
        -- and EVENT_SUMMON_VOLUNTEER re-arms in 10s. The re-arm is
        -- unconditional in C++; the handler no-ops without a selected
        -- volunteer (jedogaSummonVolunteer below).
        local guid = creature:GetGUIDLow()
        local state = jedogaState[guid]
        if state then
            if state.phase == PHASE_TWO then
                state.phase = PHASE_THREE
                schedule(guid, "endPhaseTwo", 1000, function()
                    jedogaEndPhaseTwo(creature, guid)
                end)
            end
            schedule(guid, "summonVolunteer", 10000, function()
                jedogaSummonVolunteer(creature, guid)
            end)
        end
    end
end

-- C++ boss_jedoga_shadowseeker::DamageTaken (:209-216): if
-- HealthBelowPct(55) and in PHASE_ONE, events.Reset() +
-- SetPhase(PHASE_TWO) + EVENT_START_PHASE_TWO in 1s. HealthBelowPct
-- is pre-damage health here (DamageTaken runs before the hit lands
-- — the nefarian postDamage>0 guard does NOT apply to DamageTaken
-- arms); the ionar GetHealth()/GetMaxHealth() floor idiom matches
-- C++ HealthBelowPct exactly. The phase-one gate makes it one-shot.
-- events.Reset() cancels no Lua timers (the phase-one rotation is
-- unported); the payload is the phase-two machine below. No damage
-- rewrite: return nothing.
local function jedogaDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUIDLow()
    local state = jedogaState[guid]
    if not state or state.phase ~= PHASE_ONE then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth > 0 and creature:GetHealth() < math.floor(maxHealth * 55 / 100) then
        state.phase = PHASE_TWO
        schedule(guid, "startPhaseTwo", 1000, function()
            jedogaStartPhaseTwo(creature, guid)
        end)
    end
end

-- C++ EVENT_START_PHASE_TWO (:331-337): REACT_PASSIVE + AttackStop
-- + InterruptNonMeleeSpells(true) + NOT_SELECTABLE + MovePoint(
-- POINT_PHASE_TWO, JedogaGroundPosition :94).
jedogaStartPhaseTwo = function(creature, guid)
    local state = jedogaState[guid]
    if not state then
        return
    end
    creature:SetReactState(REACT_PASSIVE)
    creature:AttackStop()
    creature:InterruptNonMeleeSpells(true)
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    creature:MoveTo(POINT_PHASE_TWO, 371.6281, -704.4836, -16.17967)
end

-- C++ EVENT_FLY_DELAY (:338-341): SetDisableGravity(true) +
-- MoveTakeoff(POINT_PHASE_TWO_FLY, JedogaFlyPosition :95).
jedogaFlyDelay = function(creature, guid)
    local state = jedogaState[guid]
    if not state then
        return
    end
    creature:SetDisableGravity(true)
    creature:MoveTakeoff(POINT_PHASE_TWO_FLY, 371.627, -704.4217, -6.707521)
end

-- C++ EVENT_CHOOSE_VOLUNTEER (:342-356): summon the third
-- controller (JedogaControllerPositions[2] :99), face it, its
-- SPELL_SACRIFICE_VISUAL self-cast, Talk(SAY_CHOOSE), then a
-- random volunteer pick + cross-AI DoAction(ACTION_CHOSEN).
jedogaChooseVolunteer = function(creature, guid)
    local state = jedogaState[guid]
    if not state then
        return
    end
    local controller = creature:SummonCreature(ENTRY_JEDOGA_CONTROLLER, 375.4977, -707.3635, -16.0964, 2.426008)
    if controller then
        creature:SetFacingToObject(controller)
        controller:CastSpell(controller, SPELL_SACRIFICE_VISUAL)
    end
    creature:Talk(SAY_CHOOSE)
    -- Random pick over the latched volunteers. C++ picks any GUID in
    -- _volunteerGUIDS (dead ones included — a dead pick fizzles the
    -- sacrifice and strands the boss flying); Lua picks among the
    -- alive ones, avoiding the C++ soft-lock edge.
    local alive = {}
    for spotIndex, volunteer in pairs(state.volunteers) do
        if volunteer and volunteer:IsAlive() then
            alive[#alive + 1] = { obj = volunteer, spot = spotIndex }
        end
    end
    if #alive == 0 then
        return
    end
    local pick = alive[math.random(#alive)]
    state.selectedSpot = pick.spot
    pick.obj:DoAction(ACTION_CHOSEN)
end

-- C++ EVENT_END_PHASE_TWO (:371-376): DespawnEntry(controller) +
-- DoCastSelf(SPELL_HOVER_FALL_2) + SetDisableGravity(false) +
-- MoveLand(POINT_GROUND, JedogaGroundPosition :94).
jedogaEndPhaseTwo = function(creature, guid)
    local state = jedogaState[guid]
    if not state then
        return
    end
    despawnJedogaControllers(guid)
    creature:CastSpell(creature, SPELL_HOVER_FALL_2)
    creature:SetDisableGravity(false)
    creature:MoveLand(POINT_GROUND, 371.6281, -704.4836, -16.17967)
end

-- C++ EVENT_SUMMON_VOLUNTEER (:358-369): re-summon a volunteer at
-- the sacrificed volunteer's spot. No-ops without a selected
-- volunteer (C++: index == size). The replacement is NOT pushed
-- into _volunteerGUIDS (track=false above).
jedogaSummonVolunteer = function(creature, guid)
    local state = jedogaState[guid]
    if not state or not state.selectedSpot then
        return
    end
    summonVolunteer(creature, guid, state.selectedSpot, false)
end

-- C++ boss_jedoga_shadowseeker::MovementInform (:270-290):
-- POINT_MOTION_TYPE (8) and EFFECT_MOTION_TYPE (16) both route.
-- POINT_PHASE_TWO -> EVENT_FLY_DELAY 2s; POINT_PHASE_TWO_FLY ->
-- EVENT_CHOOSE_VOLUNTEER 2s; POINT_GROUND -> RemoveFlag +
-- REACT_AGGRESSIVE + DoZoneInCombat + rotation re-arm (the rotation
-- itself stays documented-only — header).
local function jedogaMovementInform(event, creature, moveType, id)
    if moveType ~= POINT_MOTION_TYPE and moveType ~= EFFECT_MOTION_TYPE then
        return
    end
    local guid = creature:GetGUIDLow()
    local state = jedogaState[guid]
    if not state then
        return
    end
    if id == POINT_PHASE_TWO then
        schedule(guid, "flyDelay", 2000, function()
            jedogaFlyDelay(creature, guid)
        end)
    elseif id == POINT_PHASE_TWO_FLY then
        schedule(guid, "chooseVolunteer", 2000, function()
            jedogaChooseVolunteer(creature, guid)
        end)
    elseif id == POINT_GROUND then
        creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
        creature:SetReactState(REACT_AGGRESSIVE)
        creature:DoZoneInCombat()
    end
end

-- C++ boss_jedoga_shadowseeker::DoAction(ACTION_SACRIFICE)
-- (:218-226): Talk(SAY_SACRIFICE) + DoCastAOE(SPELL_SACRIFICE_BEAM)
-- (self-cast lowering, arugal precedent) + EVENT_END_PHASE_TWO 3s
-- + EVENT_SUMMON_VOLUNTEER 15s. No phase check in C++ — the
-- volunteer arrival fires it regardless of phase.
local function jedogaDoAction(event, creature, action)
    if action ~= ACTION_SACRIFICE then
        return
    end
    local guid = creature:GetGUIDLow()
    local state = jedogaState[guid]
    if not state then
        return
    end
    creature:Talk(SAY_SACRIFICE)
    creature:CastSpell(creature, SPELL_SACRIFICE_BEAM)
    schedule(guid, "endPhaseTwo", 3000, function()
        jedogaEndPhaseTwo(creature, guid)
    end)
    schedule(guid, "summonVolunteer", 15000, function()
        jedogaSummonVolunteer(creature, guid)
    end)
end

-- C++ npc_twilight_volunteer::MovementInform (:425-454):
-- POINT_MOTION_TYPE only. POINT_INITIAL_POSITION: face Jedoga,
-- SPELL_SPHERE_VISUAL_VOLUNTEER self-cast, kneel.
-- POINT_SACRIFICE: kneel, boss DoAction(ACTION_SACRIFICE),
-- Talk(SAY_SACRIFICED), then 3s later stand-dead +
-- DespawnOrUnsummon(5s).
local function volunteerMovementInform(event, creature, moveType, id)
    if moveType ~= POINT_MOTION_TYPE then
        return
    end
    if id == POINT_INITIAL_POSITION then
        local jedoga = creature:GetInstanceCreature(DATA_JEDOGA_SHADOWSEEKER)
        if jedoga then
            creature:SetFacingToObject(jedoga)
        end
        creature:CastSpell(creature, SPELL_SPHERE_VISUAL_VOLUNTEER)
        creature:SetStandState(STAND_KNEEL)
    elseif id == POINT_SACRIFICE then
        local jedoga = creature:GetInstanceCreature(DATA_JEDOGA_SHADOWSEEKER)
        if jedoga then
            creature:SetStandState(STAND_KNEEL)
            jedoga:DoAction(ACTION_SACRIFICE)
            creature:Talk(VOLUNTEER_SAY_SACRIFICED)
            -- The 3s scheduler leg (:449-453) runs on the volunteer AI;
            -- a dead volunteer never ticks it in C++, so the IsAlive
            -- guard mirrors that. Boss evade/death cancels the timer via
            -- cancelAllTimers (the closure then never runs).
            local vguid = creature:GetGUIDLow()
            schedule(vguid, "sacrificeEnd", 3000, function()
                if not creature:IsAlive() then
                    return
                end
                creature:SetStandState(STAND_DEAD)
                creature:DespawnOrUnsummon(5000)
            end)
        end
    end
end

-- C++ npc_twilight_volunteer::DoAction(ACTION_CHOSEN) (:405-417):
-- SPELL_PILLAR_OF_LIGHTNING self-cast + RemoveAurasDueToSpell(
-- SPHERE_VISUAL_VOLUNTEER) + Talk(SAY_CHOSEN) + stand +
-- RemoveFlag(NOT_SELECTABLE) + SetWalk(true) + MovePoint(
-- POINT_SACRIFICE, SacrificePosition :93).
local function volunteerDoAction(event, creature, action)
    if action ~= ACTION_CHOSEN then
        return
    end
    creature:CastSpell(creature, SPELL_PILLAR_OF_LIGHTNING)
    creature:RemoveAura(SPELL_SPHERE_VISUAL_VOLUNTEER)
    creature:Talk(VOLUNTEER_SAY_CHOSEN)
    creature:SetStandState(STAND_STAND)
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    creature:SetWalk(true)
    creature:MoveTo(POINT_SACRIFICE, 376.5385, -707.3567, -16.14124)
end

RegisterCreatureEvent(ENTRY_JEDOGA_SHADOWSEEKER, 1, jedogaEnterCombat)
RegisterCreatureEvent(ENTRY_JEDOGA_SHADOWSEEKER, 2, jedogaLeaveCombat)
RegisterCreatureEvent(ENTRY_JEDOGA_SHADOWSEEKER, 23, jedogaReset)
RegisterCreatureEvent(ENTRY_JEDOGA_SHADOWSEEKER, 19, jedogaJustSummoned)
RegisterCreatureEvent(ENTRY_JEDOGA_SHADOWSEEKER, 3, jedogaTargetDied)
RegisterCreatureEvent(ENTRY_JEDOGA_SHADOWSEEKER, 4, jedogaDied)
RegisterCreatureEvent(ENTRY_JEDOGA_SHADOWSEEKER, 5, jedogaSpawned)
RegisterCreatureEvent(ENTRY_JEDOGA_SHADOWSEEKER, 21, jedogaSummonedDied)
RegisterCreatureEvent(ENTRY_JEDOGA_SHADOWSEEKER, 9, jedogaDamageTaken)
RegisterCreatureEvent(ENTRY_JEDOGA_SHADOWSEEKER, 28, jedogaMovementInform)
RegisterCreatureEvent(ENTRY_JEDOGA_SHADOWSEEKER, 38, jedogaDoAction)
RegisterCreatureEvent(NPC_TWILIGHT_VOLUNTEER, 28, volunteerMovementInform)
RegisterCreatureEvent(NPC_TWILIGHT_VOLUNTEER, 38, volunteerDoAction)
