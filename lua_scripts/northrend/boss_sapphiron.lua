-- Sapphiron (Naxxramas) — Lua port of
-- src/server/scripts/Northrend/Naxxramas/boss_sapphiron.cpp
-- (boss_sapphiron (CreatureScript; boss_sapphironAI BossAI, :123);
-- npc_sapphiron_blizzard (ScriptedAI, :429 — registered via
-- RegisterNaxxramasCreatureAI); go_sapphiron_birth (GameObjectScript,
-- :464; go_sapphiron_birthAI GameObjectAI, :469);
-- spell_sapphiron_change_blizzard_target / spell_sapphiron_icebolt
-- (SpellScriptLoader + AuraScript, :501 / :541);
-- spell_sapphiron_summon_blizzard (SpellScriptLoader + SpellScript,
-- :591); achievement_the_hundred_club (AchievementCriteriaScript,
-- :637); AddSC_boss_sapphiron registers all seven — owner .cpp is
-- 657 lines; loader decl 68 / call 263 per
-- northrend_script_loader.cpp — the TENTH Naxxramas group in
-- AddNorthrendScripts(), immediately after AddSC_boss_gluth()
-- (decl 67 / call 262), under the "// Naxxramas" marker; the call
-- after it is AddSC_boss_four_horsemen()).
-- Entry: 15989 Sapphiron (naxxramas.h NPC_SAPPHIRON :102 — kalecgos
-- pass; BOSS_SAPPHIRON :43, DATA_SAPPHIRON :76;
-- instance_naxxramas.cpp binds NPC_SAPPHIRON -> SapphironGUID
-- (:172) and DATA_SAPPHIRON -> SapphironGUID (:326); the
-- CreatureScript ScriptName binding is instance-shimmed, the
-- creature_template binding DB-side as usual).
-- Sole-source verified: whole-server-tree grep for each of the
-- seven script names hits boss_sapphiron.cpp only (loader carries
-- only the decl/call lines); zero sql/ hits. No sapphiron lua
-- existed.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in
-- Go (creature combat tick), like C++ DoMeleeAttackIfReady. C++
-- DoCast default is triggered=false (vaelasz convention):
-- creature:CastSpell(nil, spell) = DoCastVictim (moroes precedent),
-- creature:CastSpell(creature, spell) = DoCastSelf (phase_hunter /
-- apothecary_hanes precedent); the third CastSpell arg is the
-- explicit CastSpell(..., true) triggered flag (moroes shadowform
-- precedent). C++ randtime -> math.random (amanitar /
-- boss_black_knight precedent).
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — CastSpell(me, SPELL_FROST_AURA 28531, triggered)
-- self-cast on combat start; the events.SetPhase(PHASE_GROUND) leg
-- has no phase bridge — documented, not wired
-- BossAI::JustEngagedWith base arms PORTED 2026-11-03: SetCombatPulseDelay(5)
-- + me-form DoZoneInCombat (ScriptedCreature.cpp:540/542; anubrekhan/gothik
-- precedent) — the old note read only the instance-bookkeeping leg and missed
-- the base call at boss_sapphiron.cpp:180; instance legs + setActive unbridged
-- (tharon_ja precedent).
-- EVENT_CHECK_RESISTS — DoCastSelf(Check Resists 60539), 0s init
-- (fired on combat start), 30s repeat; the SpellHitTarget resist
-- check and GetData(DATA_THE_HUNDRED_CLUB) legs are PORTED
-- 2026-11-08 (tick 419): ev15 clear leg below + player/creature
-- GetResistance bridges (Unit.h:863) + Go achievement case.
-- EVENT_CLEAVE — DoCastVictim(Cleave 19983), randtime(5s,15s) init,
-- randtime(5s,15s) repeat (moroes precedent); PHASE_GROUND-masked,
-- so the tick latch-skips without re-arm while flying and is
-- re-seeded at ground (heigan dancing-latch precedent).
-- EVENT_TAIL / EVENT_DRAIN / EVENT_FLIGHT + the whole air chain
-- (LIFTOFF / ICEBOLT / BREATH / EXPLOSION / LAND / GROUND) — PORTED
-- 2026-10-25 as a timer-driven flight latch (the flip is a fixed
-- engage+48.5s / land+60s timer, gothik phase-two precedent): tail
-- self-cast {7s,10s} ground-gated, drain self-cast {22s,28s}
-- unmasked with the flight-deferral flag, flight gated on
-- HealthAbovePct(10) (arm dies below 10%, C++-exact), liftoff
-- Talk + hover + RAID_MODE 2/3 icebolt targets at 200yd, icebolt
-- pops at 3s to BREATH at 2s, breath Talk + missile to EXPLOSION
-- at 8s, breath + anticheat + player icebolt-aura strip to LAND at
-- 3.5s, land Talk + hover off + delayed-drain + ground re-seed at
-- 3.5s. Approximations, all documented at the arms: MovePoint-home
-- travel is skipped (no motion bridge — liftoff starts at once),
-- the wing buffet is never summoned (no position getter on the
-- handler surface; visual-only, no AI arms), liftoff/land emotes
-- port via creature:PerformEmote (254/293, tick 224).
-- EVENT_BERSERK — Talk(EMOTE_ENRAGE 3) + DoCastSelf(Berserk 26662),
-- 15min init, fires ONCE (C++ schedules it once, never rescheduled
-- — anubarak one-shot precedent).
-- JustDied — _JustDied() instance bookkeeping has no bridge
-- (tharon_ja precedent); CastSpell(me, SPELL_DIES 29357, triggered)
-- self-cast death visual ported as the event-4 arm.
-- Talk() sites: 4 total in C++ (zero spaced `Talk (` variant) —
-- ALL FOUR PORTED: EVENT_BERSERK EMOTE_ENRAGE (3), EVENT_LIFTOFF
-- EMOTE_AIR_PHASE (0), EVENT_BREATH EMOTE_BREATH (2), EVENT_LAND
-- EMOTE_GROUND_PHASE (1).
-- All timers cancelled on 2/4/23 (gargolmar precedent).
-- BossAI base legs PORTED 2026-11-05 (SetBossState census):
-- IN_PROGRESS on ev1 (base-first), DONE on ev4, NOT_STARTED on
-- ev23 when != DONE (ScriptedCreature.cpp:537/:519/:510); the
-- tick-246 "boss-state legs PORTED" note was wrong — this file
-- had zero SetBossState calls before this port.
-- Unmodeled (no bridges — documented, not wired):
-- InitializeAI — PORTED 2026-11-05 (SetBossState census): DONE-gated
-- return + had-birth-gated invisibility via the shared instanceData
-- table (go_sapphiron_birth.lua sets hadSapphironBirth); the
-- BossAI::InitializeAI() tail is the Reset twin (ev23).
-- Reset — PORTED 2026-10-25 for the flight-phase leg (player
-- icebolt-aura strip via the loatheb GetPlayersInWorld idiom +
-- SetReactState(REACT_AGGRESSIVE) + hover off, gated on the flight
-- latch; land emote ported via creature:PerformEmote(293) in
-- landTick below (tick 224) and in Reset itself (tick 246, :162);
-- _Reset bookkeeping stays out.
-- DamageTaken — PORTED 2026-10-25, lethal-only guard fixed tick 246:
-- air-phase death prevention (damage = health-1 ONLY when the hit is
-- lethal, damage >= health, boss_sapphiron.cpp:171-176) via the ev9
-- numeric-return bridge (fireCreatureDamageTaken; tesla/halazzi
-- precedent), gated on the flight latch. The pre-fix port rewrote
-- EVERY flight-phase hit to health-1.
-- SpellHitTarget — SPELL_CHECK_RESISTS resist comparison — PORTED
-- 2026-11-08 (tick 419) as sapphironSpellHitTarget (ev15, boss_ai.go:
-- 4005-4022; caster-side, sapphiron self-casts 60539): a hit target
-- whose frost resistance exceeds 100 clears the _canTheHundredClub
-- latch via creature:SetData(DATA_THE_HUNDRED_CLUB 21462147, 0); the
-- latch is set in Reset (alive-gated) and InitializeAI (ev5) below.
-- The old "no creature:GetResistance bridge" bar is RETIRED:
-- player/creature GetResistance are bridged (Unit.h:863).
-- MovementInform (MovePoint 1 -> liftoff) — no motion bridge;
-- the flight chain is timer-driven instead (see the port note
-- above), so the travel leg is approximated away, not ported.
-- DoAction(ACTION_BIRTH) — PORTED 2026-11-05 (ev38 receiver +
-- 23s birth tick; creature:DoAction is bridged, thaddius tick-248
-- precedent; sender is go_sapphiron_birth.lua).
-- GetGUID(DATA_BLIZZARD_TARGET) — summons-list filtering +
-- BlizzardTargetSelector random target — no summon-list /
-- random-target bridges (cairne/kazzak precedent).
-- EnterPhaseGround — PORTED 2026-10-25 as the ground/flight
-- timer latch above (initial + land re-seeds). The old
-- phase-machine verdicts for TAIL / DRAIN / FLIGHT / LIFTOFF /
-- ICEBOLT / BREATH / EXPLOSION / LAND are superseded: the header's
-- DRAIN "same PHASE_GROUND bar" claim was wrong (drain is
-- scheduled WITHOUT a phase mask — the flight branch only defers
-- it), the "no raid-size selector" claim was stale
-- (creature:Is25ManRaid() is bridged — loatheb/noth), and the
-- "no summon / hover / random-target bridges" claims were stale
-- (SummonCreature, SetHover, and the GetAITargets idiom are all
-- live). What genuinely stays out:
-- EVENT_BLIZZARD — DoCastAOE(Summon Blizzard 28560) + RAID_MODE
-- (20s, 7s) repeat — the cast's ONLY effect is the SpellScript
-- summon (blizzard NPC + MoveFollow target machine), and Go has
-- no spell-effect-summon executor (jaraxxus precedent) — a
-- summon-less cast would be dead code, so the arm stays out
-- with its machine, not on a phase bar.
-- Wing-buffet summon — no position getter on the handler surface
-- (DoSummon spawns at the boss); visual-only, no AI arms.
-- Liftoff/land emote commands — PORTED tick 224 via
-- creature:PerformEmote (254 LIFTOFF / 293 LAND).
-- Explosion / Reset includePets/includeControlled aura-strip legs
-- — no pet/controlled iteration bridge (player strip itself is
-- ported via the loatheb idiom).
-- EVENT_BIRTH — PORTED 2026-11-05 as the 23s birth tick in
-- sapphironDoAction (SetPhase(PHASE_BIRTH) has no Lua consumer).
-- npc_sapphiron_blizzard (NPC_BLIZZARD 16474, C++-exact from the
-- owner enum; ScriptedAI — no RegisterLuaBoss surface, razuvious
-- precedent): Reset REACT_PASSIVE + TaskScheduler DoCastSelf(
-- m_spells[0]) — port-pattern-ready in isolation (moroes precedent)
-- but bridge-blocked — joins the bridgeable-but-entry-blocked
-- queue; SetGUID/GetGUID(DATA_BLIZZARD_TARGET) cross-AI target
-- machinery — no cross-AI bridge.
-- NPC_WING_BUFFET 17025 is C++-exact from the owner enum
-- (entry-verifiable but summon STRAND absent — bridge-blocked
-- queue).
-- go_sapphiron_birth — PORTED 2026-11-05 as
-- go_sapphiron_birth.lua (OnUse 14 twin of the GO_ACTIVATED leg;
-- instance SetData rides the shared instanceData table;
-- JUST_DEACTIVATED delete rides the immediate Despawn).
-- spell_sapphiron_icebolt (AuraScript) — PORTED tick 367 via
-- RegisterAuraEvent (the "no AuraScript bridge" claim is RETIRED:
-- the periodic hook fires per aura tick on the player path, the
-- tick-366 creature:IsMoving bridge gates the periodic arm, and the
-- new player SummonGameObject / ApplySpellImmune /
-- GetGameObjectByGUID bridges + GameObject:Delete carry the summon,
-- immunity, and remove arms). spell_sapphiron_change_blizzard_target
-- (AuraScript) / spell_sapphiron_summon_blizzard (SpellScript) still
-- join the SpellScript/AuraScript queue.
-- achievement_the_hundred_club (DATA_THE_HUNDRED_CLUB 21462147 /
-- MAX_FROST_RESISTANCE 100) — PORTED 2026-11-08 (tick 419): Go case
-- reads the boss motion's SetData slot (bossLatchData); the write
-- legs are the ev15 clear + ev23/ev5 sets above. The old "no
-- achievement-criteria bridge" note is RETIRED.

local ENTRY_SAPPHIRON = 15989

local EMOTE_AIR_PHASE = 0
local EMOTE_GROUND_PHASE = 1
local EMOTE_BREATH = 2
local EMOTE_ENRAGE = 3

local SPELL_FROST_AURA = 28531
local SPELL_CLEAVE = 19983
local SPELL_TAIL_SWEEP = 55697
local SPELL_LIFE_DRAIN = 28542
local SPELL_ICEBOLT = 28522
local GO_ICEBLOCK = 181247 -- boss_sapphiron.cpp:85
local SPELL_FROST_MISSILE = 30101
local SPELL_FROST_BREATH = 28524
local SPELL_FROST_BREATH_ANTICHEAT = 29318
local SPELL_BERSERK = 26662
local SPELL_DIES = 29357
local SPELL_CHECK_RESISTS = 60539
local DATA_THE_HUNDRED_CLUB = 21462147 -- boss_sapphiron.cpp:88
local SPELL_SCHOOL_FROST = 4 -- SharedDefines.h:315
local MAX_FROST_RESISTANCE = 100 -- boss_sapphiron.cpp:89

local REACT_PASSIVE = 0
local REACT_AGGRESSIVE = 2

local timers = {}
local flying = {}
local delayedDrain = {}
local iceboltTargets = {}
local iceboltBlocks = {} -- target GUID -> summoned GO_ICEBLOCK GUID (the C++ _block member)

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
    flying[guid] = nil
    delayedDrain[guid] = nil
    iceboltTargets[guid] = nil
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

-- C++ EVENT_CHECK_RESISTS: DoCastSelf(Check Resists 60539), 0s init,
-- 30s repeat.
local function resistsTick(creature, guid)
    creature:CastSpell(creature, SPELL_CHECK_RESISTS)
    schedule(guid, "resists", 30000, function()
        resistsTick(creature, guid)
    end)
end

-- C++ EVENT_CLEAVE: DoCastVictim(Cleave 19983), randtime(5s,15s)
-- init, randtime(5s,15s) repeat, PHASE_GROUND-masked — skipped
-- without re-arm while flying (re-seeded at ground, heigan
-- dancing-latch precedent).
local function cleaveTick(creature, guid)
    if flying[guid] then
        return
    end
    creature:CastSpell(nil, SPELL_CLEAVE)
    schedule(guid, "cleave", {5000, 15000}, function()
        cleaveTick(creature, guid)
    end)
end

-- C++ EVENT_TAIL: DoCastAOE(Tail Sweep 55697) self-cast,
-- randtime(7s,10s) init/repeat, PHASE_GROUND-masked — same
-- latch-skip as cleave.
local function tailTick(creature, guid)
    if flying[guid] then
        return
    end
    creature:CastSpell(creature, SPELL_TAIL_SWEEP)
    schedule(guid, "tail", {7000, 10000}, function()
        tailTick(creature, guid)
    end)
end

-- C++ EVENT_DRAIN / CastDrain: DoCastAOE(Life Drain 28542)
-- self-cast, randtime(22s,28s) — scheduled WITHOUT a phase mask, so
-- it fires in flight too, where the flight branch only sets
-- _delayedDrain (no re-arm; LAND's CastDrain re-seeds). Lua twins
-- it: flying -> set the flag, no re-arm; ground -> cast + re-arm.
local function drainTick(creature, guid)
    if flying[guid] then
        delayedDrain[guid] = true
        return
    end
    creature:CastSpell(creature, SPELL_LIFE_DRAIN)
    schedule(guid, "drain", {22000, 28000}, function()
        drainTick(creature, guid)
    end)
end

-- C++ EVENT_ICEBOLT: pop the stored target, cast Icebolt 28522 if
-- alive, 3s repeat until the list empties, then BREATH at 2s (an
-- empty list at entry also goes straight to BREATH at 2s).
-- The flight chain is cyclic (icebolt -> breath -> explosion ->
-- land -> ground -> flight -> liftoff -> icebolt), so the later
-- links are forward-declared here and plain-assigned below
-- (tharon_ja / gal_darah idiom — never re-`local function` a
-- forward-declared name, or closures bind the nil slot).
local breathTick, explosionTick, landTick, groundTick, flightTick

local function iceboltTick(creature, guid)
    local targets = iceboltTargets[guid]
    if targets == nil or #targets == 0 then
        schedule(guid, "breath", 2000, function()
            breathTick(creature, guid)
        end)
        return
    end
    local target = targets[#targets]
    targets[#targets] = nil
    if target ~= nil and not target:IsDead() then
        creature:CastSpell(target, SPELL_ICEBOLT)
    end
    if #targets == 0 then
        schedule(guid, "breath", 2000, function()
            breathTick(creature, guid)
        end)
    else
        schedule(guid, "icebolt", 3000, function()
            iceboltTick(creature, guid)
        end)
    end
end

-- C++ EVENT_BREATH: Talk(EMOTE_BREATH) + DoCastAOE(Frost Missile
-- 30101, visual) self-cast, then EXPLOSION at 8s.
breathTick = function(creature, guid)
    creature:Talk(EMOTE_BREATH)
    creature:CastSpell(creature, SPELL_FROST_MISSILE)
    schedule(guid, "explosion", 8000, function()
        explosionTick(creature, guid)
    end)
end

-- C++ EVENT_EXPLOSION: DoCastAOE(Frost Breath 28524 + Frost Breath
-- Anti-Cheat 29318) self-casts + instance icebolt-aura strip on
-- players (loatheb GetPlayersInWorld idiom — the only players who
-- can hold the aura are the icebolt targets) + LAND at 3.5s.
explosionTick = function(creature, guid)
    creature:CastSpell(creature, SPELL_FROST_BREATH)
    creature:CastSpell(creature, SPELL_FROST_BREATH_ANTICHEAT)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    for _, plr in ipairs(GetPlayersInWorld()) do
        if plr:GetMapId() == mapId and plr:GetInstanceId() == instanceId then
            plr:RemoveAura(SPELL_ICEBOLT)
        end
    end
    schedule(guid, "land", 3500, function()
        landTick(creature, guid)
    end)
end

-- C++ EVENT_LAND: delayed-drain CastDrain if flagged, buffet
-- despawn (never summoned — see liftoffTick), land emote
-- (PerformEmote(293), EMOTE_ONESHOT_LAND, SharedDefines.h:2007),
-- Talk(EMOTE_GROUND_PHASE), hover off, phase GROUND, then the
-- GROUND event at 3.5s.
landTick = function(creature, guid)
    if delayedDrain[guid] then
        delayedDrain[guid] = nil
        creature:CastSpell(creature, SPELL_LIFE_DRAIN)
        schedule(guid, "drain", {22000, 28000}, function()
            drainTick(creature, guid)
        end)
    end
    creature:PerformEmote(293) -- C++ EVENT_LAND (boss_sapphiron.cpp:397)
    creature:Talk(EMOTE_GROUND_PHASE)
    creature:SetHover(false)
    flying[guid] = nil
    schedule(guid, "ground", 3500, function()
        groundTick(creature, guid)
    end)
end

-- C++ EVENT_GROUND / EnterPhaseGround(false): re-seed cleave +
-- tail + FLIGHT at 60s. No drain re-seed (drain runs on its own
-- unmasked schedule) and no blizzard (summon-only cast, jaraxxus
-- precedent — Go has no spell-effect-summon executor).
groundTick = function(creature, guid)
    schedule(guid, "cleave", {5000, 15000}, function()
        cleaveTick(creature, guid)
    end)
    schedule(guid, "tail", {7000, 10000}, function()
        tailTick(creature, guid)
    end)
    schedule(guid, "flight", 60000, function()
        flightTick(creature, guid)
    end)
end

-- C++ EVENT_LIFTOFF: Talk(EMOTE_AIR_PHASE) + wing-buffet summon +
-- liftoff emote (PerformEmote(254), EMOTE_ONESHOT_LIFTOFF,
-- SharedDefines.h:2003) + hover on + ICEBOLT at 7s + icebolt targets
-- (RAID_MODE 2/3 random players, 200yd, playerOnly). The buffet
-- summon stays out: DoSummon spawns at the boss's position and
-- the handler surface has no position getter — and the buffet is
-- visual-only (no AI arms in the file), so nothing gameplay rides
-- it.
local function liftoffTick(creature, guid)
    creature:Talk(EMOTE_AIR_PHASE)
    creature:PerformEmote(254) -- C++ EVENT_LIFTOFF (boss_sapphiron.cpp:345)
    creature:SetHover(true)
    local candidates = {}
    -- C++ SelectTargetList dist gate is IsWithinCombatRange
    -- (Unit.cpp:583-598: 3D dist < nominal + both combat reaches;
    -- player reach DEFAULT_PLAYER_COMBAT_REACH 1.5,
    -- ObjectDefines.h:40).
    local bound = 200 + creature:GetCombatReach() + 1.5
    for _, unit in ipairs(creature:GetAITargets()) do
        if unit:GetObjectType() == "Player"
                and not unit:IsDead()
                and creature:GetDistance(unit) < bound then
            candidates[#candidates + 1] = unit
        end
    end
    for i = #candidates, 2, -1 do
        local j = math.random(i)
        candidates[i], candidates[j] = candidates[j], candidates[i]
    end
    local count = 2
    if creature:Is25ManRaid() then
        count = 3
    end
    local targets = {}
    for i = 1, math.min(count, #candidates) do
        targets[i] = candidates[i]
    end
    iceboltTargets[guid] = targets
    schedule(guid, "icebolt", 7000, function()
        iceboltTick(creature, guid)
    end)
end

-- C++ EVENT_FLIGHT (48.5s initial / 60s repeat, PHASE_GROUND):
-- if NOT HealthAbovePct(10) (health > floor(max*10/100),
-- Unit.h:874) the arm dies with no re-arm (C++ breaks without
-- Repeat). Otherwise REACT_PASSIVE + AttackStop + MovePoint home
-- -> MovementInform -> LIFTOFF. MovePoint has no bridge, so the
-- travel leg is skipped and liftoff starts at once — the air-chain
-- timing shifts by the (unknowable, position-dependent) travel
-- time; documented approximation.
flightTick = function(creature, guid)
    if creature:GetHealth() <= math.floor(creature:GetMaxHealth() * 10 / 100) then
        return
    end
    flying[guid] = true
    creature:SetReactState(REACT_PASSIVE)
    creature:AttackStop()
    liftoffTick(creature, guid)
end

-- C++ EVENT_BERSERK: Talk(EMOTE_ENRAGE 3) + DoCastSelf(Berserk
-- 26662), 15min init, fires ONCE (never rescheduled).
local function berserkTick(creature, guid)
    creature:Talk(EMOTE_ENRAGE)
    creature:CastSpell(creature, SPELL_BERSERK)
    local per = timers[guid]
    if per then
        per["berserk"] = nil
    end
end

-- C++ JustEngagedWith: CastSpell(me, SPELL_FROST_AURA 28531,
-- triggered) self-cast + SetPhase(PHASE_GROUND) + EnterPhaseGround
-- (true): EVENT_CHECK_RESISTS (0s, unmasked), EVENT_CLEAVE
-- (randtime 5s,15s), EVENT_TAIL (randtime 7s,10s), EVENT_BLIZZARD
-- (randtime 5s,10s — summon-only cast, stays out, jaraxxus
-- precedent), EVENT_DRAIN (randtime 22s,28s, unmasked),
-- EVENT_FLIGHT (48.5s), EVENT_BERSERK (15min, unmasked). No aggro
-- Talk in C++.
local BOSS_SAPPHIRON = 13 -- naxxramas.h:43 (NAXEncounter)
local BOSS_STATE_NOT_STARTED = 0
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_DONE = 3 -- EncounterState (InstanceScript.h:68-76)
local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NON_ATTACKABLE = 2
local ACTION_BIRTH = 1 -- boss_sapphiron.cpp:90

local function sapphironEnterCombat(event, creature, target)
    creature:SetBossState(BOSS_SAPPHIRON, BOSS_STATE_IN_PROGRESS) -- BossAI::_JustEngagedWith (ScriptedCreature.cpp:537), base-first (faerlina precedent)
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542; anubrekhan/gothik precedent)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:CastSpell(creature, SPELL_FROST_AURA, true)
    resistsTick(creature, guid)
    schedule(guid, "cleave", {5000, 15000}, function()
        cleaveTick(creature, guid)
    end)
    schedule(guid, "tail", {7000, 10000}, function()
        tailTick(creature, guid)
    end)
    schedule(guid, "drain", {22000, 28000}, function()
        drainTick(creature, guid)
    end)
    schedule(guid, "flight", 48500, function()
        flightTick(creature, guid)
    end)
    schedule(guid, "berserk", 900000, function()
        berserkTick(creature, guid)
    end)
end

local function sapphironLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ SpellHitTarget (boss_sapphiron.cpp:193-203): SPELL_CHECK_RESISTS
-- 60539 hitting a unit whose frost resistance exceeds 100 clears the
-- _canTheHundredClub latch (Initialize/Reset set it). ev15 fires
-- caster-side with the hit target (boss_ai.go:4005-4022); the target
-- resolves to a player object for raid members and a creature object
-- otherwise, and both surfaces now carry GetResistance (Unit.h:863).
-- PORTED 2026-11-08 (tick 419).
local function sapphironSpellHitTarget(event, creature, target, spellId)
    if spellId ~= SPELL_CHECK_RESISTS then
        return
    end
    if target == nil then
        return
    end
    if target:GetResistance(SPELL_SCHOOL_FROST) > MAX_FROST_RESISTANCE then
        creature:SetData(DATA_THE_HUNDRED_CLUB, 0)
    end
end

-- C++ JustDied: _JustDied() bookkeeping (no bridge) + CastSpell(
-- me, SPELL_DIES 29357, triggered) self-cast death visual.
local GAMEOBJECT_FLAGS = 9
local GO_FLAG_NOT_SELECTABLE = 16
local GO_ROOM_KELTHUZAD = 181228 -- naxxramas.h:141
local GO_STATE_READY = 1

-- C++ SetBossState(BOSS_SAPPHIRON, DONE) legs (instance_naxxramas.cpp):
-- kelthuzad door close (HandleGameObject(false) = READY; the
-- non-DONE transitions are unobserved in Go) + door object kept
-- for the instance pump's chain-end open + DONE flag for the chain.
local function sapphironDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:SetBossState(BOSS_SAPPHIRON, BOSS_STATE_DONE) -- BossAI::_JustDied (ScriptedCreature.cpp:519)
    creature:CastSpell(creature, SPELL_DIES, true)
    local door = creature:FindNearestGameObject(GO_ROOM_KELTHUZAD, 1000)
    if door then
        door:SetGoState(GO_STATE_READY)
    end
    local mapObj = creature:GetMap()
    if mapObj then
        local data = mapObj:GetInstanceData()
        if data then
            data.sapphironDone = true
            data.kelthuzadDoor = door
        end
    end
end

-- C++ Reset (:154-168), flight-phase leg: strip SPELL_ICEBOLT on
-- players (loatheb idiom) + SetReactState(REACT_AGGRESSIVE) + hover
-- off + land emote (PerformEmote(293), :162; flying latch implies
-- hovering). Gated on the flight latch — the non-flight Reset is
-- _Reset bookkeeping only.
local function sapphironReset(event, creature)
    local guid = creature:GetGUID()
    local wasFlying = flying[guid]
    cancelTimers(guid)
    if creature:GetBossState(BOSS_SAPPHIRON) ~= BOSS_STATE_DONE then
        creature:SetBossState(BOSS_SAPPHIRON, BOSS_STATE_NOT_STARTED) -- BossAI::_Reset (ScriptedCreature.cpp:510), != DONE-gated
    end
    -- C++ Reset -> Initialize (boss_sapphiron.cpp:154-168/:131-134):
    -- _canTheHundredClub = true. Alive-gated: ev23 fires on death too
    -- (fireCreatureDied runs Reset before the kill-credit criteria
    -- read, kill.go:1091/:1400), and a death-time re-arm would clobber
    -- the fight's latch before the achievement gate reads it
    -- (razorscale precedent). PORTED 2026-11-08 (tick 419).
    if creature:IsAlive() then
        creature:SetData(DATA_THE_HUNDRED_CLUB, 1)
    end
    if wasFlying then
        creature:SetReactState(REACT_AGGRESSIVE)
        creature:PerformEmote(293) -- C++ Reset :162 (EMOTE_ONESHOT_LAND)
        creature:SetHover(false)
        local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
        for _, plr in ipairs(GetPlayersInWorld()) do
            if plr:GetMapId() == mapId and plr:GetInstanceId() == instanceId then
                plr:RemoveAura(SPELL_ICEBOLT)
            end
        end
    end
end

-- C++ InitializeAI (boss_sapphiron.cpp:133-149): DONE-gated return +
-- had-birth-gated invisibility (SetVisible(false) +
-- UNIT_FLAG_NON_ATTACKABLE + REACT_PASSIVE); the BossAI::InitializeAI()
-- tail is the Reset twin (ev23). The had-birth latch rides the shared
-- instanceData table (go_sapphiron_birth.lua sets it on use; nil
-- before the first birth, so the pre-birth invisibility is the
-- default, C++-exact).
local function sapphironSpawn(event, creature)
    if creature:GetBossState(BOSS_SAPPHIRON) == BOSS_STATE_DONE then
        return
    end
    -- C++ InitializeAI (boss_sapphiron.cpp:133-149): _canTheHundredClub
    -- = true (unless DONE, returned above). PORTED 2026-11-08 (tick 419).
    creature:SetData(DATA_THE_HUNDRED_CLUB, 1)
    local mapObj = creature:GetMap()
    local data = mapObj and mapObj:GetInstanceData() or nil
    if not (data and data.hadSapphironBirth) then
        creature:SetVisible(false)
        creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
        creature:SetReactState(REACT_PASSIVE)
    end
end

-- C++ DoAction(ACTION_BIRTH 1) (boss_sapphiron.cpp:219-223): SetPhase(
-- PHASE_BIRTH) + EVENT_BIRTH in 23s. The phase has no Lua consumer
-- (the engage handler seeds the ground timers directly), so only the
-- 23s birth tick ports: visible + flag clear + REACT_AGGRESSIVE
-- (boss_sapphiron.cpp:403-407). Combat cannot start pre-birth
-- (invisible + non-attackable + passive), so cancelTimers cannot
-- eat this tick.
local function sapphironDoAction(event, creature, action)
    if action ~= ACTION_BIRTH then
        return
    end
    local guid = creature:GetGUID()
    schedule(guid, "birth", 23000, function()
        creature:SetVisible(true)
        creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
        creature:SetReactState(REACT_AGGRESSIVE)
    end)
end

-- C++ DamageTaken (:171-176): while in PHASE_FLIGHT, a LETHAL hit only
-- (damage >= health) is capped to health - 1 (the boss cannot die
-- mid-air); non-lethal damage passes through. Event 9's second
-- return rewrites damage (fireCreatureDamageTaken; tesla/halazzi
-- precedent).
local function sapphironDamageTaken(event, creature, attacker, damage)
    if flying[creature:GetGUID()] then
        local hp = creature:GetHealth()
        if damage >= hp and hp > 0 then
            return false, hp - 1
        end
    end
end

RegisterCreatureEvent(ENTRY_SAPPHIRON, 1, sapphironEnterCombat)
RegisterCreatureEvent(ENTRY_SAPPHIRON, 15, sapphironSpellHitTarget)
RegisterCreatureEvent(ENTRY_SAPPHIRON, 5, sapphironSpawn)
RegisterCreatureEvent(ENTRY_SAPPHIRON, 38, sapphironDoAction)
RegisterCreatureEvent(ENTRY_SAPPHIRON, 2, sapphironLeaveCombat)
RegisterCreatureEvent(ENTRY_SAPPHIRON, 4, sapphironDied)
RegisterCreatureEvent(ENTRY_SAPPHIRON, 9, sapphironDamageTaken)
RegisterCreatureEvent(ENTRY_SAPPHIRON, 23, sapphironReset)

-- C++ spell_sapphiron_icebolt_AuraScript (boss_sapphiron.cpp:546-585,
-- aura 28522): HandleApply grants the target immunity to 28522 frost
-- damage (IMMUNITY_DAMAGE 3, SPELL_SCHOOL_MASK_FROST 16) and
-- HandleRemove revokes it — the OnApply/OnRemove hooks fire once per
-- fresh application / actual removal on the player path, so both legs
-- are exact. HandlePeriodic (EFFECT_2 SPELL_AURA_PERIODIC_TRIGGER_SPELL)
-- summons GO_ICEBLOCK 181247 at the target's position for 25s the
-- first tick the target stands still, and no-ops while a block exists
-- or the target is moving. No PreventDefaultAction anywhere — the Go
-- default periodic-trigger cast still fires alongside the summon,
-- exactly like C++. PORTED tick 367 via RegisterAuraEvent + the new
-- player SummonGameObject / ApplySpellImmune / GetGameObjectByGUID
-- bridges and GameObject:Delete (the remove leg deletes the block;
-- Delete removes dynamic GOs, Despawn would only hide it).
local function iceboltApply(event, aura, target)
    target:ApplySpellImmune(SPELL_ICEBOLT, 3, 16, true)
end

local function iceboltRemove(event, aura, target)
    local guid = target:GetGUID()
    local blockGuid = iceboltBlocks[guid]
    if blockGuid then
        local block = target:GetGameObjectByGUID(blockGuid)
        if block then
            block:Delete()
        end
        iceboltBlocks[guid] = nil
    end
    target:ApplySpellImmune(SPELL_ICEBOLT, 3, 16, false)
end

local function iceboltPeriodic(event, aura, target)
    local guid = target:GetGUID()
    if iceboltBlocks[guid] then
        return
    end
    if target:IsMoving() then
        return
    end
    local blockGuid = target:SummonGameObject(GO_ICEBLOCK, target:GetX(), target:GetY(), target:GetZ(), 0, 25)
    if blockGuid then
        iceboltBlocks[guid] = blockGuid
    end
end

RegisterAuraEvent(SPELL_ICEBOLT, 1, iceboltPeriodic)
RegisterAuraEvent(SPELL_ICEBOLT, 2, iceboltApply)
RegisterAuraEvent(SPELL_ICEBOLT, 3, iceboltRemove)
