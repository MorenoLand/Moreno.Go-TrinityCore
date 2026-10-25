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
-- (BossAI::JustEngagedWith instance-bookkeeping leg has no bridge —
-- tharon_ja precedent).
-- EVENT_CHECK_RESISTS — DoCastSelf(Check Resists 60539), 0s init
-- (fired on combat start), 30s repeat; the SpellHitTarget resist
-- check and GetData(DATA_THE_HUNDRED_CLUB) legs have no bridges —
-- documented, not wired.
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
-- handler surface; visual-only, no AI arms), liftoff/land emote
-- commands have no bridge.
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
-- Unmodeled (no bridges — documented, not wired):
-- InitializeAI — instance GetBossState(BOSS_SAPPHIRON) / GetData(
-- DATA_HAD_SAPPHIRON_BIRTH) legs + visibility/flag/react-state legs —
-- instance-blocked (no instance bridge; flag/react-state ARE bridged in
-- isolation).
-- Reset — PORTED 2026-10-25 for the flight-phase leg (player
-- icebolt-aura strip via the loatheb GetPlayersInWorld idiom +
-- SetReactState(REACT_AGGRESSIVE) + hover off, gated on the flight
-- latch; land emote has no bridge); _Reset bookkeeping stays out.
-- DamageTaken — PORTED 2026-10-25: air-phase death prevention
-- (damage = health-1) via the ev9 numeric-return bridge
-- (fireCreatureDamageTaken; tesla/halazzi precedent), gated on the
-- flight latch.
-- SpellHitTarget — SPELL_CHECK_RESISTS resist comparison — ev15 IS
-- bridged (caster-side, boss_ai.go:1817/1833; sapphiron self-casts
-- 60539) but the arm stays out on the resistance read alone:
-- no creature:GetResistance bridge (GetArmor exists, resistance
-- does not); the _canTheHundredClub latch feeds an achievement
-- check with no bridge either.
-- MovementInform (MovePoint 1 -> liftoff) — no motion bridge;
-- the flight chain is timer-driven instead (see the port note
-- above), so the travel leg is approximated away, not ported.
-- DoAction(ACTION_BIRTH) — cross-AI from go_sapphiron_birth — no
-- instance/cross-AI bridge.
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
-- Liftoff/land emote commands — no emote bridge.
-- Explosion / Reset includePets/includeControlled aura-strip legs
-- — no pet/controlled iteration bridge (player strip itself is
-- ported via the loatheb idiom).
-- EVENT_BIRTH — visible/flag/react legs (no bridges).
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
-- go_sapphiron_birth (GameObjectScript — no GO bridge): GO_ACTIVATED
-- -> cross-AI DoAction(ACTION_BIRTH) via instance GetGuidData(
-- DATA_SAPPHIRON) + SetData(DATA_HAD_SAPPHIRON_BIRTH, 1) — joins
-- the instance/GO queue.
-- spell_sapphiron_change_blizzard_target (AuraScript) /
-- spell_sapphiron_icebolt (AuraScript) /
-- spell_sapphiron_summon_blizzard (SpellScript) — no AuraScript /
-- SpellScript binding bridge (razelikh precedent) — join the
-- SpellScript/AuraScript queue.
-- achievement_the_hundred_club (DATA_THE_HUNDRED_CLUB 21462147 /
-- MAX_FROST_RESISTANCE 100) — no achievement-criteria bridge
-- (tharon_ja precedent) — joins that queue.
-- GO_ICEBLOCK 181247 rides the unported icebolt AuraScript.

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
local SPELL_FROST_MISSILE = 30101
local SPELL_FROST_BREATH = 28524
local SPELL_FROST_BREATH_ANTICHEAT = 29318
local SPELL_BERSERK = 26662
local SPELL_DIES = 29357
local SPELL_CHECK_RESISTS = 60539

local REACT_PASSIVE = 0
local REACT_AGGRESSIVE = 2

local timers = {}
local flying = {}
local delayedDrain = {}
local iceboltTargets = {}

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
-- despawn (never summoned — see liftoffTick), land emote (no
-- emote bridge — stays out), Talk(EMOTE_GROUND_PHASE), hover off,
-- phase GROUND, then the GROUND event at 3.5s.
landTick = function(creature, guid)
    if delayedDrain[guid] then
        delayedDrain[guid] = nil
        creature:CastSpell(creature, SPELL_LIFE_DRAIN)
        schedule(guid, "drain", {22000, 28000}, function()
            drainTick(creature, guid)
        end)
    end
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
-- liftoff emote + hover on + ICEBOLT at 7s + icebolt targets
-- (RAID_MODE 2/3 random players, 200yd, playerOnly). The buffet
-- summon stays out: DoSummon spawns at the boss's position and
-- the handler surface has no position getter — and the buffet is
-- visual-only (no AI arms in the file), so nothing gameplay rides
-- it. Liftoff/land emote commands have no bridge and stay out.
local function liftoffTick(creature, guid)
    creature:Talk(EMOTE_AIR_PHASE)
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
local function sapphironEnterCombat(event, creature, target)
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

-- C++ JustDied: _JustDied() bookkeeping (no bridge) + CastSpell(
-- me, SPELL_DIES 29357, triggered) self-cast death visual.
local function sapphironDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:CastSpell(creature, SPELL_DIES, true)
end

-- C++ Reset, flight-phase leg: strip SPELL_ICEBOLT on players
-- (loatheb idiom) + SetReactState(REACT_AGGRESSIVE) + hover off
-- (land emote has no bridge). Gated on the flight latch — the
-- non-flight Reset is _Reset bookkeeping only.
local function sapphironReset(event, creature)
    local guid = creature:GetGUID()
    local wasFlying = flying[guid]
    cancelTimers(guid)
    if wasFlying then
        creature:SetReactState(REACT_AGGRESSIVE)
        creature:SetHover(false)
        local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
        for _, plr in ipairs(GetPlayersInWorld()) do
            if plr:GetMapId() == mapId and plr:GetInstanceId() == instanceId then
                plr:RemoveAura(SPELL_ICEBOLT)
            end
        end
    end
end

-- C++ DamageTaken: while in PHASE_FLIGHT, damage = health - 1 (the
-- boss cannot die mid-air). Event 9's second return rewrites
-- damage (fireCreatureDamageTaken; tesla/halazzi precedent).
local function sapphironDamageTaken(event, creature, attacker, damage)
    if flying[creature:GetGUID()] then
        local hp = creature:GetHealth()
        if hp > 0 then
            return false, hp - 1
        end
    end
end

RegisterCreatureEvent(ENTRY_SAPPHIRON, 1, sapphironEnterCombat)
RegisterCreatureEvent(ENTRY_SAPPHIRON, 2, sapphironLeaveCombat)
RegisterCreatureEvent(ENTRY_SAPPHIRON, 4, sapphironDied)
RegisterCreatureEvent(ENTRY_SAPPHIRON, 9, sapphironDamageTaken)
RegisterCreatureEvent(ENTRY_SAPPHIRON, 23, sapphironReset)
