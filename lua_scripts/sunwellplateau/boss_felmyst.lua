-- Felmyst (Sunwell Plateau) — Lua port of
-- src/server/scripts/EasternKingdoms/SunwellPlateau/boss_felmyst.cpp
-- (boss_felmyst, npc_felmyst_vapor, npc_felmyst_trail); sunwell_
-- plateau.h:33 (DATA_FELMYST = 2, third boss), :67 (NPC_FELMYST =
-- 25038), :69 (NPC_DEAD = 25268), :73 (NPC_VAPOR = 25265), :74
-- (NPC_VAPOR_TRAIL = 25267). Entries: 25038 Felmyst (C++
-- ScriptName "boss_felmyst"), 25267 vapor trail ("npc_felmyst_
-- trail") — entry+ScriptName verifiable from the C++ sources;
-- the creature_template ScriptName bindings are DB-side — no TDB
-- in this workspace, so only the C++-side naming is verified.
-- Talk lines used: YELL_KILL=1 (kill), YELL_BERSERK=4 (berserk),
-- YELL_DEATH=5 (death); YELL_BIRTH=0 (JustAppeared), YELL_
-- BREATH=2 (the DoTextEmote is commented out in C++) and YELL_
-- TAKEOFF=3 belong to the unmodeled spawn/flight arms.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 14 OnHitBySpell, 19 OnJustSummoned, 22
-- OnSummoned (trail ctor arms), 23 OnReset. Timers via CreateLuaEvent;
-- melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnEnterCombat(1):
-- triggered self-cast sunwell radiance 45769 + noxious fumes
-- 47002 (C++ DoCast(me, AURA_*, true)), arm cleave 19983 {5s,10s}
-- then {5s,10s}, non-triggered DoCastVictim (nil ticks cast
-- nothing but keep the schedule, jeklik convention) / corrosion
-- 45866 {10s,20s} then {20s,30s}, non-triggered DoCastVictim /
-- gas nova 45855 {15s,20s} then {20s,25s}, non-triggered self-cast
-- (C++ DoCast default is triggered=false — vaelastrasz
-- convention) / encapsulate 45661 {20s,25s} then {25s,30s}, non-
-- triggered on a threat-list player alive within 150 yd
-- (C++ SelectTarget(Random, 0, 150, true), player-only;
-- nil pick casts nothing but keeps the schedule) /
-- berserk 45078 10min one-shot: Talk(YELL_BERSERK), triggered
-- self-cast (C++ DoCast(me, SPELL_BERSERK, true)), then re-armed
-- at 10s (C++-exact — the PHASE_GROUND arm re-schedules
-- EVENT_BERSERK at 10s; the PHASE_FLIGHT arm fires without
-- re-arming, and the ground-only Lua model matches that arm).
-- OnTargetDied(3): Talk(YELL_KILL), no
-- TYPEID gate (C++-exact — the C++ KilledUnit Talk has no victim
-- gate). OnDied(4): Talk(YELL_DEATH) + SetBossState(DATA_FELMYST=2,
-- DONE) via creature:SetBossState (bridge live 2026-11-05).
-- OnReset(23): the
-- Initialize/events.Reset arms (phase tracking lives only in the
-- unmodeled flight sequence — see below); the DisableGravity arm
-- ports via the creature:SetDisableGravity bridge (felmystResetState;
-- stale "no bridge" retracted); the bounding-radius (no creature
-- motion slot) and setActive(false) arms stay out, and the
-- SetBossState(NOT_STARTED) arm ports via creature:SetBossState
-- (bridge live 2026-11-05). OnLeaveCombat(2)/OnReset(23): cancel timers.
-- Trail (25267): OnSummoned(22): NOT_SELECTABLE SetFlag + triggered
-- self-cast trail trigger 45399 + SetTarget(self) (C++ constructor
-- arms, C++-exact — the constructor runs once at AI creation; C++
-- Reset() is empty, so OnReset(23) would wrongly re-fire on evade,
-- and ev5 never fires for temp summons, so the ctor arms ride ev22;
-- stale "no flag bridge" retracted — SetFlag(59, x) is bridged);
-- the 0.01 bounding-radius arm has no bridge (bounding radius has
-- no creature motion slot; the combat-reach bridge covers only
-- UNIT_FIELD_COMBATREACH).
-- Vapor (25265, "npc_felmyst_vapor") is not registered: its only
-- C++ arms are the NOT_SELECTABLE flag, the 0.8 run-speed arm and
-- the no-victim random-target AttackStart selection — all bridged
-- now (SetFlag, SetSpeedRate, DoZoneInCombat, AttackStart, ev7,
-- ev22) — but vapors never spawn without the flight machine, so
-- registration waits on it; entry verified for future registration.
-- NPC_DEAD (25268, Blazing Dead) has no AI class in this file — no
-- C++ ScriptName to verify a binding against, so it is not
-- registered (firesworn convention); its boss-side JustSummoned
-- (AttackStart random + DoZoneInCombat + triggered self-cast dead
-- passive 45415) and SpellHit arms port below (felmystSummoned /
-- felmystSpellHit).
-- Deliberate deviations (all await engine bridges): the flight
-- phase is unmodeled — the EVENT_FLIGHT 1min transition, the
-- entire 10-step HandleFlightSequence (liftoff emote + Talk(YELL_
-- TAKEOFF), MovePoint/contact-point/facing movement, the two
-- vapor summons + VAPOR_CHANNEL 45389 / VAPOR_TRIGGER 45411
-- cross-creature casts, the fog-breath 45495 self-cast + FOG_
-- TRIGGER 45582 / FOG_FORCE 45782 fog summon + breath-move loop x3,
-- the max-threat chase and the landing + EnterPhase(GROUND) re-
-- entry) has no movement/contact-point/facing/cross-creature
-- bridges — the summon bridge is live, but the whole flight phase is
-- phase-gated on the unported PHASE_FLIGHT/MovementInform machine;
-- the fight stays in ground-phase timers
-- (ragnaros-intro convention); the DamageTaken lethal-damage
-- rewrite arm has no bearer without the flight phase (phase is
-- always GROUND here). The SpellHit FOG_INFORM 45714 arm (summon
-- NPC_DEAD at the caster, copy health, triggered FOG_CHARM 45717
-- + FOG_CHARM2 45726 self-casts, lethal DealDamage on the caster)
-- ports below (felmystSpellHit, event 14; the summoned DEAD's
-- AttackStart/DoZoneInCombat/passive legs port in felmystSummoned,
-- event 19). The
-- DespawnSummons(NPC_VAPOR_TRAIL)->NPC_DEAD summon arm (fires only
-- when phase == PHASE_FLIGHT) is phase-gated; the SetVisible/
-- Despawn side ports via GetCreaturesInGrid + SetVisible + Despawn
-- (bridged) in the Reset leg below. The
-- JustAppeared Talk(YELL_BIRTH) arm ports on event 5 in the sibling
-- eastern_kingdoms/boss_felmyst.lua (ev5 fires on initial spawn and
-- respawn via fireCreatureSpawned — the old "no spawn-fire bridge"
-- note is retracted). The AttackStart/MoveInLineOfSight PHASE_
-- FLIGHT gates have no bridge (melee/aggro gating is engine-
-- side). The UpdateAI gate is C++ IsNonMeleeSpellCast(false)
-- (boss_felmyst.cpp:421, after events.Update, before both phase
-- switches): while a non-melee cast is in flight the due ground
-- event does not execute. Ported via the live creature:IsCasting()
-- bridge (lua_creature_events.go:1950, bjarngrim precedent): each
-- ground tick defers 500ms under its own key, consuming no re-arm;
-- the stale "IsCasting is constant-false / timers fire
-- unconditionally" claim is retracted. The UpdateVictim evade-side
-- arm has no port (evade-side cleanup is engine-side).

local NPC_FELMYST = 25038
local NPC_VAPOR_TRAIL = 25267
local NPC_DEAD = 25268 -- sunwell_plateau.h:69

local SPELL_CLEAVE = 19983
local SPELL_CORROSION = 45866
local SPELL_GAS_NOVA = 45855
local SPELL_ENCAPSULATE_CHANNEL = 45661
local SPELL_BERSERK = 45078
local AURA_SUNWELL_RADIANCE = 45769
local AURA_NOXIOUS_FUMES = 47002
local SPELL_TRAIL_TRIGGER = 45399
local SPELL_DEAD_PASSIVE = 45415 -- boss_felmyst.cpp:70
local SPELL_FOG_INFORM = 45714 -- boss_felmyst.cpp:74
local SPELL_FOG_CHARM = 45717 -- boss_felmyst.cpp:75
local SPELL_FOG_CHARM2 = 45726 -- boss_felmyst.cpp:76

local YELL_KILL = 1
local YELL_BERSERK = 4
local YELL_DEATH = 5

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

-- Threat-list players alive within range (C++ SelectTarget
-- Random pool via GetAITargets).
local function randomThreatPlayerInRange(creature, maxDist)
    local targets = creature:GetAITargets()
    local out = {}
    for _, unit in ipairs(targets) do
        if unit:GetObjectType() == "Player" and not unit:IsDead()
                and creature:GetDistance(unit) - creature:GetCombatReach() - 1.5 < maxDist then -- IsWithinCombatRange gate (Unit.cpp:583; player reach 1.5, ObjectDefines.h:40)
            out[#out + 1] = unit
        end
    end
    if #out == 0 then
        return nil
    end
    return out[math.random(#out)]
end

-- C++ EVENT_CLEAVE: DoCastVictim(19983); repeat {5s,10s}.
local function felmystCleave(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "cleave", 500, function()
            felmystCleave(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_CLEAVE)
    schedule(guid, "cleave", math.random(5000, 10000), function()
        felmystCleave(creature, guid)
    end)
end

-- C++ EVENT_CORROSION: DoCastVictim(45866); repeat {20s,30s}.
local function felmystCorrosion(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "corrosion", 500, function()
            felmystCorrosion(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_CORROSION)
    schedule(guid, "corrosion", math.random(20000, 30000), function()
        felmystCorrosion(creature, guid)
    end)
end

-- C++ EVENT_GAS_NOVA: DoCast(me, 45855); repeat {20s,25s}.
local function felmystGasNova(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "gasnova", 500, function()
            felmystGasNova(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_GAS_NOVA)
    schedule(guid, "gasnova", math.random(20000, 25000), function()
        felmystGasNova(creature, guid)
    end)
end

-- C++ EVENT_ENCAPSULATE: DoCast(threat-list player alive
-- within 150 yd, 45661); repeat {25s,30s}.
local function felmystEncapsulate(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "encapsulate", 500, function()
            felmystEncapsulate(creature, guid)
        end)
        return
    end
    local target = randomThreatPlayerInRange(creature, 150)
    if target then
        creature:CastSpell(target, SPELL_ENCAPSULATE_CHANNEL)
    end
    schedule(guid, "encapsulate", math.random(25000, 30000), function()
        felmystEncapsulate(creature, guid)
    end)
end

-- C++ EVENT_BERSERK: Talk(YELL_BERSERK) + triggered self-cast
-- 45078; C++ re-arms the event at 10s after every fire.
local function felmystBerserkTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "berserk", 500, function()
            felmystBerserkTick(creature, guid)
        end)
        return
    end
    creature:Talk(YELL_BERSERK)
    creature:CastSpell(creature, SPELL_BERSERK, true)
    schedule(guid, "berserk", 10000, function()
        felmystBerserkTick(creature, guid)
    end)
end

-- C++ JustEngagedWith: triggered self-cast of the two pull auras
-- + the ground-phase schedule. The setActive arm has no bridge;
-- SetBossState(IN_PROGRESS) ports via creature:SetBossState
-- (bridge live 2026-11-05); the DoZoneInCombat arm is ported below
-- (bridge live 2026-10-23).
local function felmystEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    -- C++ boss_felmyst.cpp:160-169: berserk schedule, setActive(true),
    -- DoZoneInCombat() (bridge live 2026-10-23), then the two aura self-casts.
    creature:DoZoneInCombat()
    creature:CastSpell(creature, AURA_SUNWELL_RADIANCE, true)
    creature:CastSpell(creature, AURA_NOXIOUS_FUMES, true)
    -- C++ EnterPhase(PHASE_GROUND) (boss_felmyst.cpp:258): SetSpeedRate(MOVE_RUN, 2.0f)
    -- ahead of the ground-phase schedule; the landing re-entry (:403) rides the
    -- unported flight machine and stays out.
    creature:SetSpeedRate(1, 2.0)
    creature:SetBossState(2, 1) -- DATA_FELMYST, IN_PROGRESS (boss_felmyst.cpp:169)
    schedule(guid, "cleave", math.random(5000, 10000), function()
        felmystCleave(creature, guid)
    end)
    schedule(guid, "corrosion", math.random(10000, 20000), function()
        felmystCorrosion(creature, guid)
    end)
    schedule(guid, "gasnova", math.random(15000, 20000), function()
        felmystGasNova(creature, guid)
    end)
    schedule(guid, "encapsulate", math.random(20000, 25000), function()
        felmystEncapsulate(creature, guid)
    end)
    schedule(guid, "berserk", 600000, function()
        felmystBerserkTick(creature, guid)
    end)
end

-- C++ KilledUnit: Talk(YELL_KILL) with no victim gate (C++-exact).
local function felmystTargetDied(event, creature, victim)
    creature:Talk(YELL_KILL)
end

-- C++ JustDied: Talk(YELL_DEATH) + SetBossState(DATA_FELMYST=2, DONE)
-- (boss_felmyst.cpp:194-199).
local function felmystDied(event, creature, killer)
    creature:Talk(YELL_DEATH)
    creature:SetBossState(2, 3)
    cancelTimers(creature:GetGUIDLow())
end

local function felmystResetState(event, creature)
    cancelTimers(creature:GetGUIDLow())
    -- C++ boss_felmyst::Reset (boss_felmyst.cpp:143-156): Initialize()
    -- -> events.Reset() (both internal; timer cancel above is the Lua
    -- twin) -> me->SetDisableGravity(true) (:149) ->
    -- SetFloatValue(COMBATREACH, 10) (:151) — the BOUNDINGRADIUS leg
    -- (:150) has no creature motion slot and stays out, as does the
    -- setActive(false) leg; the SetBossState(NOT_STARTED) leg ports
    -- below (bridge live 2026-11-05).
    creature:SetDisableGravity(true)
    creature:SetCombatReach(10)
    -- C++ DespawnSummons(NPC_VAPOR_TRAIL) (:153, :488-508): every
    -- creature of the entry within 100 yd (2D) -> SetVisible(false) +
    -- DespawnOrUnsummon; the NPC_DEAD-summon arm only fires when phase
    -- == PHASE_FLIGHT, which is never true at Reset (phase == PHASE_NONE).
    for _, trail in ipairs(creature:GetCreaturesInGrid(NPC_VAPOR_TRAIL, 100)) do
        trail:SetVisible(false)
        trail:Despawn()
    end
    -- C++ Reset tail (boss_felmyst.cpp:156): SetBossState(DATA_FELMYST, NOT_STARTED).
    -- Alive-gated: Go fires ev23 on the death path too, and C++ Reset
    -- never runs on death (JustDied owns the DONE set).
    if creature:IsAlive() then
        creature:SetBossState(2, 0)
    end
end

-- C++ npc_felmyst_trailAI constructor arms: triggered self-cast
-- trail trigger 45399 + SetTarget(self) (boss_felmyst.cpp:562-567).
-- OnSummoned(22): the constructor runs once at AI creation and C++
-- Reset() is empty, so ev23 would wrongly re-fire on evade; ev5 does
-- NOT fire for temp summons (summonTempCreature fires only ev19/ev22,
-- lua_creature_events.go:190-198 — trails spawn only as temp summons,
-- EVENT_SUMMON_FOG 10s), so the ctor arms ride ev22, which fires on
-- the summoned creature (fireCreaturePetSummoned).
local function trailSpawn(event, creature)
    -- C++ npc_felmyst_trailAI ctor arm order: SetFlag(UNIT_FIELD_FLAGS,
    -- UNIT_FLAG_NOT_SELECTABLE) -> DoCast(me, 45399, true) ->
    -- SetTarget(self) (boss_felmyst.cpp:563-567); stale "no flag
    -- bridge" retracted — creature:SetFlag(59, x) is bridged
    -- (lua_creature_events.go:968) with real targeting effect. The
    -- SetFloatValue(BOUNDINGRADIUS) leg stays unbridged.
    creature:SetFlag(59, 0x2000000) -- UNIT_FLAG_NOT_SELECTABLE
    creature:CastSpell(creature, SPELL_TRAIL_TRIGGER, true)
    creature:SetTarget(creature:GetGUID())
end

-- C++ SpellHit (boss_felmyst.cpp:201-221): hit by FOG_INFORM 45714 ->
-- summon NPC_DEAD (25268) at the caster, copy the caster's max health,
-- triggered self-casts FOG_CHARM 45717 + FOG_CHARM2 45726 on the summon,
-- then lethal DealDamage on the caster (full current health, school 0
-- = physical mask 1, C++ SPELL_SCHOOL_MASK_NORMAL). The summon type is
-- Go 3 = C++ TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT, 5s.
local function felmystSpellHit(event, creature, caster, spellId)
    if spellId ~= SPELL_FOG_INFORM or caster == nil then
        return
    end
    local summon = creature:SummonCreature(NPC_DEAD, caster:GetX(), caster:GetY(), caster:GetZ(), 0, 3, 5000)
    if summon ~= nil then
        local maxHealth = caster:GetMaxHealth()
        summon:SetMaxHealth(maxHealth)
        summon:SetHealth(maxHealth)
        summon:CastSpell(summon, SPELL_FOG_CHARM, true)
        summon:CastSpell(summon, SPELL_FOG_CHARM2, true)
    end
    creature:DealDamage(caster, caster:GetHealth(), false, 0)
end

-- C++ JustSummoned (boss_felmyst.cpp:228-236): summon entry 25268 ->
-- AttackStart(random threat-list pick — C++ SelectTarget(Random) has
-- no playerOnly flag and no distance), DoZoneInCombat(summon),
-- triggered self-cast DEAD_PASSIVE 45415.
local function felmystSummoned(event, creature, summoned)
    if summoned:GetEntry() ~= NPC_DEAD then
        return
    end
    local targets = creature:GetAITargets()
    if #targets > 0 then
        summoned:AttackStart(targets[math.random(#targets)])
    end
    summoned:DoZoneInCombat()
    summoned:CastSpell(summoned, SPELL_DEAD_PASSIVE, true)
end

RegisterCreatureEvent(NPC_FELMYST, 1, felmystEnterCombat)
RegisterCreatureEvent(NPC_FELMYST, 2, felmystResetState)
RegisterCreatureEvent(NPC_FELMYST, 3, felmystTargetDied)
RegisterCreatureEvent(NPC_FELMYST, 4, felmystDied)
RegisterCreatureEvent(NPC_FELMYST, 14, felmystSpellHit)
RegisterCreatureEvent(NPC_FELMYST, 19, felmystSummoned)
RegisterCreatureEvent(NPC_FELMYST, 23, felmystResetState)
RegisterCreatureEvent(NPC_VAPOR_TRAIL, 22, trailSpawn)
RegisterCreatureEvent(NPC_VAPOR_TRAIL, 1, function(_, creature)
    creature:SetMeleeAttack(false) -- boss_felmyst.cpp:576: empty UpdateAI in C++ (never melees)
end)

-- C++ npc_felmyst_trailAI::MoveInLineOfSight (boss_felmyst.cpp:574) is an
-- EMPTY override — it suppresses the default ScriptedAI::MoveInLineOfSight
-- aggro engage (the trail is a visual trigger, never attacks). Go's
-- fireCreatureMoveInLOS (lua_creature_events.go:531) fires ev27 on the
-- aggro-engage branch and a boolean true vetoes exactly that engage, so
-- return true models :561 C++-exact (the cyanigosa precedent).
local function trailMoveInLOS(event, creature, unit)
    return true
end

RegisterCreatureEvent(NPC_VAPOR_TRAIL, 27, trailMoveInLOS)
