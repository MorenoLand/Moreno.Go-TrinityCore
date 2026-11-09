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
-- (bridge landed), events.SetPhase(PHASE_ONE) (no phase bridge on the
-- moorabi precedent), and the 25 NPC_TWILIGHT_VOLUNTEER 30385
-- summons with MovePoint(POINT_INITIAL_POSITION) choreography
-- (summon STRAND + motion-master bridges absent) — are
-- documented-only below);
-- KilledUnit Talk(SAY_SLAY 3) player-gated (event 3,
-- victim:GetObjectType()=="Player" — nalorakk precedent);
-- JustDied Talk(SAY_DEATH 4) (event 4; _JustDied instance
-- bookkeeping ports in jedogaDied below — SetBossState + flag).
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
-- / PHASE_THREE) and the phase flips are driven by unbridged arms
-- (intro MovementInform(POINT_GROUND) -> REACT_AGGRESSIVE +
-- DoZoneInCombat; DamageTaken HealthBelowPct(55) -> PHASE_TWO, no
-- health-pct bridge — doomwalker precedent; EVENT_CHOOSE_
-- VOLUNTEER / volunteer-DoAction sacrifice machine -> PHASE_THREE;
-- EVENT_END_PHASE_TWO -> MoveLand(POINT_GROUND) re-arm) — no phase
-- bridge on the Lua surface (gal_darah / moorabi precedent), so
-- nothing phase-gated is emulated; ungated timers would over-cast
-- versus C++ (e.g. Cyclone Strike firing during the unbridged
-- phase-two flying sacrifice sequence — the intro gating alone
-- makes the engage-timer reading unfaithful).
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
-- DamageTaken 55% health crossing (events.Reset + SetPhase(
-- PHASE_TWO) + EVENT_START_PHASE_TWO 1s) — the old "no health-pct
-- bridge (doomwalker precedent)" claim was stale (event 9 carries
-- pre-damage damage; creature:GetHealthPct() is bridged), but the
-- arm stays documented-only: the payload is the whole unbridged
-- phase-two machine — SetReactState/InterruptNonMeleeSpells/
-- SetFlag(NOT_SELECTABLE) + the flying-sacrifice MovePoint/motion
-- + summon + cross-AI DoAction sequence (no phase / motion /
-- summon bridges; the cross-AI DoAction sender chain is unported
-- — ev38 dispatch IS bridged, lua_creature_events.go:802;
-- flag/react-state bridges exist since 2026-10-20 but the payload
-- is the unported phase machine).
-- Phase-two machine — EVENT_START_PHASE_TWO (SetReactState(
-- REACT_PASSIVE), AttackStop, InterruptNonMeleeSpells(true),
-- SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE), MovePoint(
-- POINT_PHASE_TWO)), EVENT_FLY_DELAY (SetDisableGravity(true) +
-- MoveTakeoff(POINT_PHASE_TWO_FLY)), EVENT_CHOOSE_VOLUNTEER
-- (controller summon + SetFacingToObject — the me->
-- SetFacingToObject(controller) leg (C++ :337) is bridgeable via the
-- atan2 twin (devourer_of_souls precedent), but the arm stays
-- documented-only: the controller is created by SummonCreature (no
-- summon bridge) inside the unported phase-two machine; +
-- SPELL_SACRIFICE_VISUAL 56133 self-cast + Talk(SAY_CHOOSE 1) +
-- random volunteer pick via _volunteerGUIDS + ObjectAccessor::
-- GetCreature + cross-AI DoAction(ACTION_CHOSEN)),
-- EVENT_SUMMON_VOLUNTEER re-summon
-- machine, EVENT_END_PHASE_TWO (summons.DespawnEntry(controller) +
-- DoCastSelf(SPELL_HOVER_FALL_2 56157) + MoveLand(POINT_GROUND)):
-- summon STRAND + motion-master (MovePoint / MoveLand / MoveTakeoff
-- / SetDisableGravity) + flag / interrupt / stand-state + cross-AI
-- DoAction sender chain + ObjectAccessor lookup absent (ev38
-- dispatch IS bridged, lua_creature_events.go:802) —
-- documented-only.
-- DoAction(ACTION_SACRIFICE) (Talk(SAY_SACRIFICE 2) + DoCastAOE(
-- Sacrifice Beam 56150) — the self-cast lowering is port-pattern-
-- ready (arugal precedent), but the arm fires only through the
-- unbridged cross-AI DoAction + phase-two sacrifice machine
-- (gothik precedent) — + EVENT_END_PHASE_TWO re-schedule +
-- EVENT_SUMMON_VOLUNTEER reschedule): ev38 dispatch IS bridged
-- (creature:DoAction, lua_creature_events.go:802); the sender is
-- the volunteer MovementInform POINT_SACRIFICE
-- (boss_jedoga_shadowseeker.cpp:438) on an unregistered bearer —
-- documented-only.
-- JustSummoned — worshipper kneel leg stays out (SummonCreatureGroup
-- arrivals, unbridged), but the summons latch + DespawnAll legs are
-- PORTED via ev19 (jedogaJustSummoned below; controllers are direct
-- Lua summons), SummonedCreatureDies (initiate _initiatesKilled == 15
-- latch -> SPELL_HOVER_FALL_1 56100 + EVENT_START_FIGHT_1 1s
-- (RemoveAurasDueToSpell(SPELL_BEAM_VISUAL_JEDOGA 56312) then
-- summons.DespawnEntry(controller) + SetDisableGravity(false) +
-- MoveLand(POINT_GROUND)) and the volunteer _volunteerWork=false
-- latch + PHASE_THREE flip; payload rides the unbridged summon /
-- movement / instance arms), EnterEvadeMode (summons.DespawnAll PORTED
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
-- npc_twilight_volunteer — ENTRY-VERIFIABLE BUT BRIDGE-BLOCKED
-- (30385 — ahnkahet.h NPC_TWILIGHT_VOLUNTEER, kalecgos pass): zero
-- registration. The entire AI rides the unported phase-two
-- machine — DoAction(ACTION_CHOSEN) initiation (ev38 dispatch IS
-- bridged, lua_creature_events.go:802; the sender is the boss's
-- own EVENT_CHOOSE_VOLUNTEER, boss_jedoga_shadowseeker.cpp:348),
-- SPELL_PILLAR_OF_
-- LIGHTNING 56868 self-cast (port-pattern-ready: creature:CastSpell(
-- creature, spell)), RemoveAurasDueToSpell(SPELL_SPHERE_VISUAL_
-- VOLUNTEER 56102) (the old "no aura-removal bridge" claim was
-- stale — creature:RemoveAura is bridged, lua_creature_events.go:
-- 153 — but the arm fires only through the unported volunteer
-- MovementInform sender (boss_jedoga_shadowseeker.cpp:438) +
-- sacrifice machine, so it stays documented-only), Talk(SAY_CHOSEN
-- 0) /
-- Talk(SAY_SACRIFICED 1), stand-state machine, MovePoint(
-- POINT_SACRIFICE) choreography (no motion bridges), the sacrifice
-- payload (instance->GetCreature(DATA_JEDOGA_SHADOWSEEKER) +
-- cross-AI DoAction(ACTION_SACRIFICE) — instance-script model
-- absent and the sender is the volunteer MovementInform
-- (boss_jedoga_shadowseeker.cpp:438); ev38 dispatch IS bridged,
-- lua_creature_events.go:802), and DespawnOrUnsummon(5s) (stale "no
-- despawn bridge" wording: creature:Despawn() IS bridged, but the
-- arm rides the unbridged MovementInform/sacrifice choreography
-- machine on a zero-registration bearer — terestian precedent).
-- Joins the entry-verifiable-but-
-- bridge-blocked queue (amanitar mushrooms precedent).
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
end

local function jedogaTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

local function jedogaDied(event, creature, killer)
    -- C++ JustDied (:195-198): _JustDied() (DespawnAll) before Talk.
    despawnJedogaSummons(creature:GetGUIDLow())
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
    end
end

RegisterCreatureEvent(ENTRY_JEDOGA_SHADOWSEEKER, 1, jedogaEnterCombat)
RegisterCreatureEvent(ENTRY_JEDOGA_SHADOWSEEKER, 2, jedogaLeaveCombat)
RegisterCreatureEvent(ENTRY_JEDOGA_SHADOWSEEKER, 23, jedogaReset)
RegisterCreatureEvent(ENTRY_JEDOGA_SHADOWSEEKER, 19, jedogaJustSummoned)
RegisterCreatureEvent(ENTRY_JEDOGA_SHADOWSEEKER, 3, jedogaTargetDied)
RegisterCreatureEvent(ENTRY_JEDOGA_SHADOWSEEKER, 4, jedogaDied)
RegisterCreatureEvent(ENTRY_JEDOGA_SHADOWSEEKER, 5, jedogaSpawned)
RegisterCreatureEvent(ENTRY_JEDOGA_SHADOWSEEKER, 21, jedogaSummonedDied)
