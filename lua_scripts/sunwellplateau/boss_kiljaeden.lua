-- Kil'jaeden (Sunwell Plateau) — Lua port of
-- src/server/scripts/EasternKingdoms/SunwellPlateau/boss_kiljaeden.cpp
-- (boss_kiljaeden, npc_hand_of_the_deceiver, npc_volatile_felfire_
-- fiend, npc_armageddon, npc_sinster_reflection — note the C++
-- ScriptName typo "sinster", preserved 1:1 in the registration
-- below; boss_kalecgos_kj, npc_kiljaeden_controller, npc_felfire_
-- portal, npc_shield_orb and go_orb_of_the_blue_flight documented
-- below, not registered); sunwell_plateau.h:36 (DATA_KILJAEDEN =
-- 5, sixth/final boss), :80 (NPC_ANVEENA = 26046), :81 (NPC_
-- KALECGOS_KJ = 25319), :83 (NPC_KILJAEDEN = 25315), :84 (NPC_
-- KILJAEDEN_CONTROLLER = 25608), :85 (NPC_HAND_OF_THE_DECEIVER =
-- 25588), :86 (NPC_FELFIRE_PORTAL = 25603), :87 (NPC_VOLATILE_
-- FELFIRE_FIEND = 25598), :88 (NPC_ARMAGEDDON_TARGET = 25735),
-- :89 (NPC_SHIELD_ORB = 25502), :94 (NPC_SINISTER_REFLECTION =
-- 25708). Entries: 25315 Kil'jaeden (C++ ScriptName "boss_
-- kiljaeden"), 25588 Hand of the Deceiver ("npc_hand_of_the_
-- deceiver"), 25598 Volatile Felfire Fiend ("npc_volatile_felfire_
-- fiend"), 25735 Armageddon Target ("npc_armageddon"), 25708
-- Sinister Reflection ("npc_sinster_reflection") — all entry+
-- ScriptName verifiable from the C++ sources per
-- RegisterSunwellPlateauCreatureAI in AddSC_boss_kiljaeden; the
-- creature_template ScriptName bindings are DB-side — no TDB in
-- this workspace, so only the C++-side naming is verified. Talk
-- lines used: Kil'jaeden SAY_KJ_DEATH=0 (death), SAY_KJ_SLAY=1
-- (kill), SAY_KJ_REFLECTION=2 (shadow spike), SAY_KJ_EMERGE=3
-- (pull), SAY_KJ_DARKNESS=4 (darkness damage), SAY_KJ_PHASE3=5,
-- SAY_KJ_PHASE4=6, SAY_KJ_PHASE5=7 (phase lines), EMOTE_KJ_
-- DARKNESS=8 (darkness channel); the kalecgos/anveena speech rows
-- belong to the unmodeled cross-creature speech chain — below.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 9 OnDamageTaken, 23 OnReset. Timers via
-- CreateLuaEvent. Melee is engine-driven in Go (creature combat
-- tick), like C++ DoMeleeAttackIfReady.
--
-- Fight shape (C++-exact for the modeled arms). Kil'jaeden
-- (25315). OnEnterCombat(1): per-GUID scheduler state reset to
-- the C++ Initialize() values (phase=PHASE_NORMAL, ActiveTimers=
-- 5, soul flay 11s / legion lightning 30s / fire bloom 20s /
-- shield orb 35s, shadow spike 4s / flame dart 3s / darkness 45s
-- / orbs empower 35s armed but unpumped, armageddon 2s) +
-- Talk(SAY_KJ_EMERGE) (the C++ speech timer fires EMERGE on the
-- first combat tick — C++-exact) + start the 100ms scheduler pump
-- (see below). OnTargetDied(3): Talk(SAY_KJ_SLAY), no TYPEID gate
-- (C++-exact — the C++ KilledUnit Talk has no victim gate).
-- OnDied(4): Talk(SAY_KJ_DEATH); timer/state cleanup only (the
-- summons.DespawnAll arm has no summon bridge; the SetBossState(
-- DONE) arm is blocked on the instance-script model).
-- OnReset(23)/OnLeaveCombat(2): timer/state cleanup (the evade-
-- side controller Reset relay and the RemoveDynObject ring arm
-- have no cross-creature/GO bridges; the SetBossState arms are
-- instance-blocked). The SetCombatMovement(false) constructor arm
-- has no movement bridge — KJ never moves in this model (C++-
-- exact, since it never schedules movement).
-- Scheduler pump (C++ UpdateAI 1:1): every 100ms, if waiting,
-- count the wait down and on expiry reactivate the timers
-- (ChangeTimers(false, 0) semantics); then fire each pumped timer
-- whose remaining time is below the tick and which is not
-- deactivated; then decrement the active timers; then run the
-- phase checks (one transition per tick, blocked while in
-- darkness — C++-exact). Fires:
-- * soul flay: non-triggered DoCastVictim(47106 slow + 45442)
--   (the IsNonMeleeSpellCast queue gate has no UNIT_STATE bridge
--   — timers fire unconditionally, jeklik convention); repeat 3.5s.
-- * legion lightning: RemoveAura(45442) + non-triggered cast
--   45664 on a random alive player in the instance within 100 yd
--   (up to 6 picks; the first pick without 45839 vengeance wins,
--   otherwise the last pick stands — C++-exact); repeat 30s (18s
--   in sacrifice); soul flay re-armed 2.5s.
-- * fire bloom: RemoveAura(45442) + non-triggered DoCastAOE(45641)
--   (self-cast, kalecgos convention); repeat 40s (25s in
--   sacrifice); soul flay re-armed 1s.
-- * shield orb: the (Phase-1) shield-orb summons have no summon
--   bridge — skipped; the soul-flay 2s re-arm is a real side
--   effect and is kept; repeat {30s,60s}.
-- * shadow spike (phase 3+): Talk(SAY_KJ_REFLECTION); the 4
--   sinister-reflection summons have no summon bridge — skipped;
--   non-triggered DoCastAOE(46680) (self-cast); ChangeTimers(true,
--   30000) — all timers deactivated, 30s wait; on resume the
--   shadow-spike timer stays deactivated (C++-exact — its timer
--   is 0, re-armed only by EnterNextPhase).
-- * flame dart (phase 3+): non-triggered DoCastAOE(45737)
--   (self-cast); repeat 3s.
-- * darkness (phase 3+): two-step. Not in darkness: Talk(EMOTE_KJ_
--   DARKNESS), non-triggered DoCastAOE(46605), ChangeTimers(true,
--   9000), darkness timer 8750, self re-activated, IsInDarkness=
--   true. In darkness: darkness timer 15s (sacrifice) else {40s,
--   70s}, IsInDarkness=false, non-triggered DoCastAOE(45657),
--   Talk(SAY_KJ_DARKNESS); soul flay re-armed 9s.
-- * orbs empower (phase 3+): the EmpowerOrb call targets boss_
--   kalecgos_kj via the instance script — cross-creature/instance
--   blocked; the C++ OrbActivated=true deactivation is mirrored
--   (timer deactivates after firing).
-- * armageddon (phase 4+): the target pick + NPC_ARMAGEDDON_TARGET
--   summon have no summon bridge — the arm is summon-blocked and
--   excluded from the pump (documented, no bridgeable side
--   effects).
-- Phase transitions (C++-exact): <85% -> PHASE_DARKNESS
-- (ActiveTimers=9), <55% -> PHASE_ARMAGEDDON (ActiveTimers=10),
-- <25% -> PHASE_SACRIFICE; each runs EnterNextPhase: all timers
-- deactivated, shadow spike re-armed at 100ms, darkness 15s
-- (sacrifice) else {10s,40s}, orbs empower 10s (sacrifice) else 5s;
-- KJ's own speech rows fire at their C++-table offsets from the
-- shadow-spike chain start (~20s SAY_KJ_PHASE3, ~25s SAY_KJ_
-- PHASE4, ~28.5s SAY_KJ_PHASE5) — the interleaved kalecgos/
-- anveena rows are cross-creature blocked.
-- Hand of the Deceiver (25588). OnReset(23): re-arm the 1s upkeep
-- tick (C++-exact Initialize values for the combat timers).
-- Upkeep tick: out of combat -> non-triggered self-cast shadow
-- channeling 46757 (C++-exact); in combat and <20% without the
-- 45772 aura -> triggered self-cast shadow infusion 45772 (C++-
-- exact). OnEnterCombat(1): arm shadow bolt volley {8s,14s} then
-- 12s, non-triggered DoCastVictim(45770). The felfire-portal arm
-- (NPC_FELFIRE_PORTAL summon + threat relay) has no summon/threat
-- bridges — unmodeled. OnDied(4): timer cleanup (the controller
-- deceiverDeathCount++ relay is cross-creature blocked; the
-- SetBossState arms are instance-blocked). OnLeaveCombat(2):
-- cancel the volley schedule (the InterruptNonMeleeSpells and
-- threat-relay arms have no bridges).
-- Volatile Felfire Fiend (25598). OnDamageTaken(9): lethal damage
-- -> triggered self-cast felfire fission 45779 (C++-exact; the
-- damage itself is NOT rewritten — the C++ lets the death
-- proceed). OnEnterCombat(1): 2s prime, then a 500ms proximity
-- check: armed and within 3 yd of the victim -> non-triggered
-- DoCastVictim(45779); the C++ KillSelf has no kill bridge — the
-- fiend is modeled as consumed by its explosion (check stops
-- after firing). The AddThreat lock-on arm has no threat bridge.
-- OnReset(23)/OnLeaveCombat(2)/OnDied(4): timer/state cleanup.
-- Armageddon Target (25735). OnReset(23): triggered self-cast
-- armageddon visual 45911, 9s later triggered 45914, 5s later
-- triggered 45909 (C++-exact chain); the DespawnOrUnsummon arm has
-- no despawn bridge — documented. (The KJ-side armageddon-target
-- summon arm is summon-blocked — above.)
-- Sinister Reflection (25708). OnEnterCombat(1): read the
-- victim's class (player-only; unclassed victims get no spells —
-- C++-exact, the C++ switch matches nothing for class 0) and arm
-- the C++-exact per-class rotation (C++ timers start at 0, so the
-- first cast fires immediately): druid 47072 moonfire {2s,4s};
-- hunter 48098 multi-shot {8s,10s} + 16496 shoot {4s,6s} + in
-- melee range 48098 multi-shot {6s,8s} (C++-exact quirk — the
-- melee arm re-casts multi-shot, never the 40652 wing clip the
-- enum defines); mage 47074 fireball {2s,4s}; warlock 47076
-- shadow bolt {3s,5s} + triggered 46190 curse of agony on a
-- random alive player in the instance within 100 yd {2s,4s};
-- warrior 17207 whirlwind {9s,11s}; paladin 37369 hammer of
-- justice {6s,8s} + 38921 holy shock {2s,4s}; priest 47077 holy
-- smite {4s,6s} + non-triggered self-cast 47079 renew {6s,8s};
-- shaman 47071 earth shock {4s,6s}; rogue triggered 45897
-- hemorrhage {4s,6s}. Melee is engine-driven (C++ calls
-- DoMeleeAttackIfReady in every class arm except the hunter's
-- ranged arm). The SetCanDualWield arms (warrior/shaman/rogue)
-- have no bridge; the SetDisplayId/AttackStart summon-side setup
-- is summon-blocked. OnReset(23)/OnLeaveCombat(2)/OnDied(4):
-- timer/state cleanup.
--
-- Deliberate deviations (standing gaps): no instance-script model
-- — boss admission via the luaBossAI shim (the SetBossState(
-- DATA_KILJAEDEN, NOT_STARTED/IN_PROGRESS/DONE) arms, the
-- JustEngagedWith DoZoneInCombat arm, the deceiver/controller
-- SetBossState arms and the DATA_MURU offcombat gate are
-- skipped); no cross-creature bridge — the kalecgos/anveena
-- speech rows, the controller<->deceiver death-count/threat/
-- Reset relays, the KJ->kalecgos EmpowerOrb/ResetOrbs/
-- SetRingOfBlueFlames calls and the reflection summon-side
-- AttackStart/JustSummoned setup are unmodeled; no summon bridge
-- — the controller's deceiver/anveena/KJ spawns, the shield-orb
-- summons, the armageddon-target summons, the sinister-reflection
-- summons, the felfire-portal/fiend spawns and the KJ/JustDied
-- summons.DespawnAll arms are unmodeled; no GameObjectAI bridge
-- — go_orb_of_the_blue_flight (gossip summon + vengeance cast)
-- is not registered; no movement/teleport bridge — the shield-
-- orb circle-movement/MovementInform/DoTeleportTo arms and the
-- SetCombatMovement arms are unmodeled; no threat bridge — the
-- deceiver/portal AddThreat relays are unmodeled; no kill/
-- despawn/flag/react/gravity bridges — the fiend KillSelf, the
-- armageddon DespawnOrUnsummon, the kalecgos-kj/controller flag/
-- gravity/faction/GO-type arms and the reflection dual-wield
-- arms are unmodeled; no UNIT_STATE bridge — the soul-flay/
-- lightning/fire-bloom/shadow-spike/darkness IsNonMeleeSpellCast
-- queue gates are unmodeled (timers fire unconditionally, jeklik
-- convention); the SPELLVALUE/aura-filter targeting nuances have
-- no bearer. boss_kalecgos_kj (25319) is not registered — every
-- arm is instance/GO-bound (orb GO lookups, GO faction/cast/
-- type arms, cross-creature EmpowerOrb entry points). npc_
-- kiljaeden_controller (25608) is not registered — its UpdateAI
-- is summon/instance driven (deceiver/anveena spawns, death-
-- count-driven KJ spawn) and its only self-contained arm (the
-- 30s offcombat Talk) is gated on instance boss states. npc_
-- felfire_portal (25603) is not registered — its only UpdateAI
-- arm is the summon+threat fiend spawner. npc_shield_orb (25502)
-- is not registered — movement-bound circle path plus an
-- instance-GUID-targeted shadow bolt, neither bridgeable.

local ENTRY_KILJAEDEN = 25315
local ENTRY_DECEIVER = 25588
local ENTRY_FELFIRE_FIEND = 25598
local ENTRY_ARMAGEDDON_TARGET = 25735
local ENTRY_SINISTER_REFLECTION = 25708

-- Yells (boss_kiljaeden.cpp Yells enum, KJ block)
local SAY_KJ_DEATH = 0
local SAY_KJ_SLAY = 1
local SAY_KJ_REFLECTION = 2
local SAY_KJ_EMERGE = 3
local SAY_KJ_DARKNESS = 4
local SAY_KJ_PHASE3 = 5
local SAY_KJ_PHASE4 = 6
local SAY_KJ_PHASE5 = 7
local EMOTE_KJ_DARKNESS = 8

-- Spells (boss_kiljaeden.cpp Spells enum; only modeled ones listed)
local SPELL_SOUL_FLAY = 45442
local SPELL_SOUL_FLAY_SLOW = 47106
local SPELL_LEGION_LIGHTNING = 45664
local SPELL_FIRE_BLOOM = 45641
local SPELL_SHADOW_SPIKE = 46680
local SPELL_FLAME_DART = 45737
local SPELL_DARKNESS_OF_A_THOUSAND_SOULS = 46605
local SPELL_DARKNESS_OF_A_THOUSAND_SOULS_DAMAGE = 45657
local SPELL_VENGEANCE_OF_THE_BLUE_FLIGHT = 45839
local SPELL_SHADOW_BOLT_VOLLEY = 45770
local SPELL_SHADOW_INFUSION = 45772
local SPELL_SHADOW_CHANNELING = 46757
local SPELL_FELFIRE_FISSION = 45779
local SPELL_ARMAGEDDON_VISUAL = 45911
local SPELL_ARMAGEDDON_VISUAL2 = 45914
local SPELL_ARMAGEDDON_TRIGGER = 45909
local SPELL_SR_CURSE_OF_AGONY = 46190
local SPELL_SR_SHADOW_BOLT = 47076
local SPELL_SR_EARTH_SHOCK = 47071
local SPELL_SR_FIREBALL = 47074
local SPELL_SR_HEMORRHAGE = 45897
local SPELL_SR_HOLY_SHOCK = 38921
local SPELL_SR_HAMMER_OF_JUSTICE = 37369
local SPELL_SR_HOLY_SMITE = 47077
local SPELL_SR_RENEW = 47079
local SPELL_SR_SHOOT = 16496
local SPELL_SR_MULTI_SHOT = 48098
local SPELL_SR_WHIRLWIND = 17207
local SPELL_SR_MOONFIRE = 47072

-- Player classes (nefarian convention)
local CLASS_WARRIOR = 1
local CLASS_PALADIN = 2
local CLASS_HUNTER = 3
local CLASS_ROGUE = 4
local CLASS_PRIEST = 5
local CLASS_SHAMAN = 7
local CLASS_MAGE = 8
local CLASS_WARLOCK = 9
local CLASS_DRUID = 11

-- Phases (boss_kiljaeden.cpp Phase enum)
local PHASE_NORMAL = 2
local PHASE_DARKNESS = 3
local PHASE_ARMAGEDDON = 4
local PHASE_SACRIFICE = 5

local TICK = 100

-- C++ Timer indices 1..8 (TIMER_SOUL_FLAY..TIMER_ORBS_EMPOWER);
-- TIMER_SPEECH is replaced by the KJ self-line one-shots and
-- TIMER_ARMAGEDDON is summon-blocked (header).
local TIMER_ORDER = {
    "soulflay", "lightning", "firebloom", "shieldorb",
    "shadowspike", "flamedart", "darkness", "orbsempower",
}

local timers = {}
local kjState = {}
local fiendState = {}
local reflectionState = {}

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

-- Alive players sharing the creature's map+instance (teron
-- convention).
local function playersInInstance(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            found[#found + 1] = p
        end
    end
    return found
end

local function belowPct(creature, pct)
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return false
    end
    return creature:GetHealth() * 100 / maxHealth < pct
end

-- C++ Initialize() values, 1:1.
local function newKjState()
    return {
        phase = PHASE_NORMAL,
        active = 5,
        t = {
            soulflay = 11000,
            lightning = 30000,
            firebloom = 20000,
            shieldorb = 35000,
            shadowspike = 4000,
            flamedart = 3000,
            darkness = 45000,
            orbsempower = 35000,
        },
        off = {
            soulflay = false,
            lightning = false,
            firebloom = false,
            shieldorb = false,
            shadowspike = false,
            flamedart = false,
            darkness = false,
            orbsempower = false,
        },
        isInDarkness = false,
        isWaiting = false,
        waitTimer = 0,
    }
end

-- C++ ChangeTimers(status, WTimer), 1:1. OrbActivated is always
-- false in this model (the orbs-empower cross-creature arm is
-- unmodeled), so that rule is a no-op here.
local function changeTimers(st, status, wTimer)
    for _, name in ipairs(TIMER_ORDER) do
        st.off[name] = status
    end
    if wTimer > 0 then
        st.isWaiting = true
        st.waitTimer = wTimer
    end
    if st.t.shadowspike == 0 then
        st.off.shadowspike = true
    end
    if st.phase == PHASE_SACRIFICE then
        st.off.shieldorb = true
    end
end

-- C++ EnterNextPhase(), 1:1, plus KJ's own speech rows at their
-- C++-table offsets (the kalecgos/anveena rows are cross-creature
-- blocked — header).
local function enterNextPhase(creature, guid, st, phase)
    st.phase = phase
    changeTimers(st, true, 0)
    st.off.shadowspike = false
    st.t.shadowspike = 100
    st.t.darkness = (phase == PHASE_SACRIFICE) and 15000 or math.random(10000, 40000)
    st.t.orbsempower = (phase == PHASE_SACRIFICE) and 10000 or 5000
    if phase == PHASE_DARKNESS then
        schedule(guid, "phase3say", 20000, function()
            creature:Talk(SAY_KJ_PHASE3)
        end)
    elseif phase == PHASE_ARMAGEDDON then
        schedule(guid, "phase4say", 25000, function()
            creature:Talk(SAY_KJ_PHASE4)
        end)
    elseif phase == PHASE_SACRIFICE then
        schedule(guid, "phase5say", 28500, function()
            creature:Talk(SAY_KJ_PHASE5)
        end)
    end
end

-- Random alive player in the instance within 100 yd; up to 6
-- picks, first pick without the vengeance aura wins, otherwise
-- the last pick stands (C++-exact SelectTarget loop).
local function pickRandomPlayer(creature)
    local candidates = {}
    for _, p in ipairs(playersInInstance(creature)) do
        if creature:IsWithinDist(p, 100) then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates == 0 then
        return nil
    end
    local pick = nil
    for _ = 1, 6 do
        pick = candidates[math.random(1, #candidates)]
        if not pick:HasAura(SPELL_VENGEANCE_OF_THE_BLUE_FLIGHT) then
            break
        end
    end
    return pick
end

local function fireTimer(creature, st, name)
    local phase = st.phase
    if name == "soulflay" then
        creature:CastSpell(nil, SPELL_SOUL_FLAY_SLOW)
        creature:CastSpell(nil, SPELL_SOUL_FLAY)
        st.t.soulflay = 3500
    elseif name == "lightning" then
        creature:RemoveAura(SPELL_SOUL_FLAY)
        local target = pickRandomPlayer(creature)
        if target then
            creature:CastSpell(target, SPELL_LEGION_LIGHTNING)
        end
        st.t.lightning = (phase == PHASE_SACRIFICE) and 18000 or 30000
        st.t.soulflay = 2500
    elseif name == "firebloom" then
        creature:RemoveAura(SPELL_SOUL_FLAY)
        creature:CastSpell(creature, SPELL_FIRE_BLOOM)
        st.t.firebloom = (phase == PHASE_SACRIFICE) and 25000 or 40000
        st.t.soulflay = 1000
    elseif name == "shieldorb" then
        -- The (Phase-1) shield-orb summons have no summon bridge —
        -- skipped; the soul-flay re-arm is a real side effect.
        st.t.shieldorb = math.random(30000, 60000)
        st.t.soulflay = 2000
    elseif name == "shadowspike" then
        creature:Talk(SAY_KJ_REFLECTION)
        -- The 4 sinister-reflection summons have no summon bridge.
        creature:CastSpell(creature, SPELL_SHADOW_SPIKE)
        changeTimers(st, true, 30000)
        st.t.shadowspike = 0
    elseif name == "flamedart" then
        creature:CastSpell(creature, SPELL_FLAME_DART)
        st.t.flamedart = 3000
    elseif name == "darkness" then
        if not st.isInDarkness then
            creature:Talk(EMOTE_KJ_DARKNESS)
            creature:CastSpell(creature, SPELL_DARKNESS_OF_A_THOUSAND_SOULS)
            changeTimers(st, true, 9000)
            st.t.darkness = 8750
            st.off.darkness = false
            st.isInDarkness = true
        else
            st.t.darkness = (phase == PHASE_SACRIFICE) and 15000 or math.random(40000, 70000)
            st.isInDarkness = false
            creature:CastSpell(creature, SPELL_DARKNESS_OF_A_THOUSAND_SOULS_DAMAGE)
            creature:Talk(SAY_KJ_DARKNESS)
        end
        st.t.soulflay = 9000
    elseif name == "orbsempower" then
        -- The EmpowerOrb call targets boss_kalecgos_kj via the
        -- instance script — cross-creature blocked; mirror the
        -- C++ OrbActivated=true deactivation.
        st.off.orbsempower = true
    end
end

-- C++ boss_kiljaedenAI::UpdateAI, 1:1 (melee is engine-driven).
local function kjPump(creature, guid)
    local st = kjState[guid]
    if not st then
        return
    end
    if st.isWaiting then
        if st.waitTimer <= TICK then
            st.isWaiting = false
            changeTimers(st, false, 0)
        else
            st.waitTimer = st.waitTimer - TICK
        end
    end
    local pumped = {}
    for i = 1, st.active - 1 do
        pumped[#pumped + 1] = TIMER_ORDER[i]
    end
    for _, name in ipairs(pumped) do
        if st.t[name] < TICK and not st.off[name] then
            fireTimer(creature, st, name)
        end
    end
    for _, name in ipairs(pumped) do
        if not st.off[name] then
            st.t[name] = st.t[name] - TICK
            if st.t[name] < 0 then
                st.t[name] = 0
            end
        end
    end
    -- Phase transitions: one per tick, blocked while in darkness.
    if st.phase == PHASE_NORMAL and not st.isInDarkness and belowPct(creature, 85) then
        st.active = 9
        enterNextPhase(creature, guid, st, PHASE_DARKNESS)
    elseif st.phase == PHASE_DARKNESS and not st.isInDarkness and belowPct(creature, 55) then
        st.active = 10
        enterNextPhase(creature, guid, st, PHASE_ARMAGEDDON)
    elseif st.phase == PHASE_ARMAGEDDON and not st.isInDarkness and belowPct(creature, 25) then
        enterNextPhase(creature, guid, st, PHASE_SACRIFICE)
    end
    schedule(guid, "pump", TICK, function()
        kjPump(creature, guid)
    end)
end

-- ================= Kil'jaeden (25315) =================

local function kjEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    kjState[guid] = newKjState()
    creature:Talk(SAY_KJ_EMERGE)
    schedule(guid, "pump", TICK, function()
        kjPump(creature, guid)
    end)
end

local function kjCleanup(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    kjState[guid] = nil
end

local function kjDied(event, creature, killer)
    creature:Talk(SAY_KJ_DEATH)
    kjCleanup(event, creature)
end

local function kjTargetDied(event, creature, victim)
    creature:Talk(SAY_KJ_SLAY)
end

RegisterCreatureEvent(ENTRY_KILJAEDEN, 1, kjEnterCombat)
RegisterCreatureEvent(ENTRY_KILJAEDEN, 2, kjCleanup)
RegisterCreatureEvent(ENTRY_KILJAEDEN, 3, kjTargetDied)
RegisterCreatureEvent(ENTRY_KILJAEDEN, 4, kjDied)
RegisterCreatureEvent(ENTRY_KILJAEDEN, 23, kjCleanup)

-- ================= Hand of the Deceiver (25588) =================

-- C++ UpdateAI upkeep: out of combat -> shadow channeling; in
-- combat and <20% without the infusion aura -> shadow infusion.
local function deceiverUpkeep(creature, guid)
    if creature:IsInCombat() then
        if belowPct(creature, 20) and not creature:HasAura(SPELL_SHADOW_INFUSION) then
            creature:CastSpell(creature, SPELL_SHADOW_INFUSION, true)
        end
    else
        creature:CastSpell(creature, SPELL_SHADOW_CHANNELING)
    end
    schedule(guid, "upkeep", 1000, function()
        deceiverUpkeep(creature, guid)
    end)
end

-- C++ Shadow Bolt Volley arm: DoCastVictim(45770); repeat 12s.
local function deceiverVolley(creature, guid)
    creature:CastSpell(nil, SPELL_SHADOW_BOLT_VOLLEY)
    schedule(guid, "volley", 12000, function()
        deceiverVolley(creature, guid)
    end)
end

local function deceiverReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "upkeep", 1000, function()
        deceiverUpkeep(creature, guid)
    end)
end

local function deceiverEnterCombat(event, creature)
    local guid = creature:GetGUID()
    schedule(guid, "volley", math.random(8000, 14000), function()
        deceiverVolley(creature, guid)
    end)
end

local function deceiverLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    local per = timers[guid]
    if per and per.volley then
        RemoveEventById(per.volley)
        per.volley = nil
    end
end

local function deceiverDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_DECEIVER, 1, deceiverEnterCombat)
RegisterCreatureEvent(ENTRY_DECEIVER, 2, deceiverLeaveCombat)
RegisterCreatureEvent(ENTRY_DECEIVER, 4, deceiverDied)
RegisterCreatureEvent(ENTRY_DECEIVER, 23, deceiverReset)

-- ================= Volatile Felfire Fiend (25598) =================

-- C++ DamageTaken: lethal damage -> triggered felfire fission;
-- the damage itself is NOT rewritten (the death proceeds).
local function fiendDamageTaken(event, creature, attacker, damage)
    if damage >= creature:GetHealth() then
        creature:CastSpell(creature, SPELL_FELFIRE_FISSION, true)
    end
end

-- C++ proximity detonation: 2s prime, then explode within 3 yd of
-- the victim. The C++ KillSelf has no kill bridge — the fiend is
-- modeled as consumed by its explosion.
local function fiendProximity(creature, guid)
    local st = fiendState[guid]
    if not st then
        return
    end
    if st.armed then
        local victim = creature:GetVictim()
        if victim and creature:IsWithinDist(victim, 3) then
            creature:CastSpell(nil, SPELL_FELFIRE_FISSION)
            st.armed = false
            return
        end
    end
    schedule(guid, "proximity", 500, function()
        fiendProximity(creature, guid)
    end)
end

local function fiendEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    fiendState[guid] = { armed = false }
    schedule(guid, "prime", 2000, function()
        local st = fiendState[guid]
        if st then
            st.armed = true
        end
        schedule(guid, "proximity", 500, function()
            fiendProximity(creature, guid)
        end)
    end)
end

local function fiendCleanup(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    fiendState[guid] = nil
end

RegisterCreatureEvent(ENTRY_FELFIRE_FIEND, 1, fiendEnterCombat)
RegisterCreatureEvent(ENTRY_FELFIRE_FIEND, 2, fiendCleanup)
RegisterCreatureEvent(ENTRY_FELFIRE_FIEND, 4, fiendCleanup)
RegisterCreatureEvent(ENTRY_FELFIRE_FIEND, 9, fiendDamageTaken)
RegisterCreatureEvent(ENTRY_FELFIRE_FIEND, 23, fiendCleanup)

-- ================= Armageddon Target (25735) =================

-- C++ npc_armageddonAI::UpdateAI visual chain, 1:1. The final
-- DespawnOrUnsummon arm has no despawn bridge — documented.
local function armageddonReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:CastSpell(creature, SPELL_ARMAGEDDON_VISUAL, true)
    schedule(guid, "visual2", 9000, function()
        creature:CastSpell(creature, SPELL_ARMAGEDDON_VISUAL2, true)
        schedule(guid, "trigger", 5000, function()
            creature:CastSpell(creature, SPELL_ARMAGEDDON_TRIGGER, true)
        end)
    end)
end

local function armageddonCleanup(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_ARMAGEDDON_TARGET, 2, armageddonCleanup)
RegisterCreatureEvent(ENTRY_ARMAGEDDON_TARGET, 4, armageddonCleanup)
RegisterCreatureEvent(ENTRY_ARMAGEDDON_TARGET, 23, armageddonReset)

-- ================= Sinister Reflection (25708) =================

local function reflectionDruid(creature, guid)
    creature:CastSpell(nil, SPELL_SR_MOONFIRE)
    schedule(guid, "sr", math.random(2000, 4000), function()
        reflectionDruid(creature, guid)
    end)
end

local function reflectionHunterMulti(creature, guid)
    creature:CastSpell(nil, SPELL_SR_MULTI_SHOT)
    schedule(guid, "sr1", math.random(8000, 10000), function()
        reflectionHunterMulti(creature, guid)
    end)
end

local function reflectionHunterShoot(creature, guid)
    creature:CastSpell(nil, SPELL_SR_SHOOT)
    schedule(guid, "sr2", math.random(4000, 6000), function()
        reflectionHunterShoot(creature, guid)
    end)
end

-- C++-exact quirk: the melee-range arm re-casts MULTI_SHOT, never
-- wing clip (40652). IsWithinMeleeRange has no bridge — 5 yd
-- approximates it.
local function reflectionHunterMelee(creature, guid)
    local victim = creature:GetVictim()
    if victim and creature:IsWithinDist(victim, 5) then
        creature:CastSpell(nil, SPELL_SR_MULTI_SHOT)
        schedule(guid, "sr0", math.random(6000, 8000), function()
            reflectionHunterMelee(creature, guid)
        end)
    else
        schedule(guid, "sr0", 1000, function()
            reflectionHunterMelee(creature, guid)
        end)
    end
end

local function reflectionHunter(creature, guid)
    creature:CastSpell(nil, SPELL_SR_MULTI_SHOT)
    schedule(guid, "sr1", math.random(8000, 10000), function()
        reflectionHunterMulti(creature, guid)
    end)
    schedule(guid, "sr2", math.random(4000, 6000), function()
        reflectionHunterShoot(creature, guid)
    end)
    schedule(guid, "sr0", 1000, function()
        reflectionHunterMelee(creature, guid)
    end)
end

local function reflectionMage(creature, guid)
    creature:CastSpell(nil, SPELL_SR_FIREBALL)
    schedule(guid, "sr", math.random(2000, 4000), function()
        reflectionMage(creature, guid)
    end)
end

local function reflectionWarlockBolt(creature, guid)
    creature:CastSpell(nil, SPELL_SR_SHADOW_BOLT)
    schedule(guid, "sr1", math.random(3000, 5000), function()
        reflectionWarlockBolt(creature, guid)
    end)
end

local function reflectionWarlockCurse(creature, guid)
    local candidates = {}
    for _, p in ipairs(playersInInstance(creature)) do
        if creature:IsWithinDist(p, 100) then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates > 0 then
        creature:CastSpell(candidates[math.random(1, #candidates)], SPELL_SR_CURSE_OF_AGONY, true)
    end
    schedule(guid, "sr2", math.random(2000, 4000), function()
        reflectionWarlockCurse(creature, guid)
    end)
end

local function reflectionWarrior(creature, guid)
    creature:CastSpell(nil, SPELL_SR_WHIRLWIND)
    schedule(guid, "sr", math.random(9000, 11000), function()
        reflectionWarrior(creature, guid)
    end)
end

local function reflectionPaladinJustice(creature, guid)
    creature:CastSpell(nil, SPELL_SR_HAMMER_OF_JUSTICE)
    schedule(guid, "sr1", math.random(6000, 8000), function()
        reflectionPaladinJustice(creature, guid)
    end)
end

local function reflectionPaladinShock(creature, guid)
    creature:CastSpell(nil, SPELL_SR_HOLY_SHOCK)
    schedule(guid, "sr2", math.random(2000, 4000), function()
        reflectionPaladinShock(creature, guid)
    end)
end

local function reflectionPriestSmite(creature, guid)
    creature:CastSpell(nil, SPELL_SR_HOLY_SMITE)
    schedule(guid, "sr1", math.random(4000, 6000), function()
        reflectionPriestSmite(creature, guid)
    end)
end

local function reflectionPriestRenew(creature, guid)
    creature:CastSpell(creature, SPELL_SR_RENEW)
    schedule(guid, "sr2", math.random(6000, 8000), function()
        reflectionPriestRenew(creature, guid)
    end)
end

local function reflectionShaman(creature, guid)
    creature:CastSpell(nil, SPELL_SR_EARTH_SHOCK)
    schedule(guid, "sr", math.random(4000, 6000), function()
        reflectionShaman(creature, guid)
    end)
end

local function reflectionRogue(creature, guid)
    creature:CastSpell(nil, SPELL_SR_HEMORRHAGE, true)
    schedule(guid, "sr", math.random(4000, 6000), function()
        reflectionRogue(creature, guid)
    end)
end

local function reflectionEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    local class = 0
    local victim = creature:GetVictim()
    if victim and victim:IsPlayer() then
        class = victim:GetClass()
    end
    reflectionState[guid] = { class = class }
    -- C++ timers start at 0: the first cast fires immediately.
    if class == CLASS_DRUID then
        reflectionDruid(creature, guid)
    elseif class == CLASS_HUNTER then
        reflectionHunter(creature, guid)
    elseif class == CLASS_MAGE then
        reflectionMage(creature, guid)
    elseif class == CLASS_WARLOCK then
        reflectionWarlockBolt(creature, guid)
        reflectionWarlockCurse(creature, guid)
    elseif class == CLASS_WARRIOR then
        reflectionWarrior(creature, guid)
    elseif class == CLASS_PALADIN then
        reflectionPaladinJustice(creature, guid)
        reflectionPaladinShock(creature, guid)
    elseif class == CLASS_PRIEST then
        reflectionPriestSmite(creature, guid)
        reflectionPriestRenew(creature, guid)
    elseif class == CLASS_SHAMAN then
        reflectionShaman(creature, guid)
    elseif class == CLASS_ROGUE then
        reflectionRogue(creature, guid)
    end
end

local function reflectionCleanup(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    reflectionState[guid] = nil
end

RegisterCreatureEvent(ENTRY_SINISTER_REFLECTION, 1, reflectionEnterCombat)
RegisterCreatureEvent(ENTRY_SINISTER_REFLECTION, 2, reflectionCleanup)
RegisterCreatureEvent(ENTRY_SINISTER_REFLECTION, 4, reflectionCleanup)
RegisterCreatureEvent(ENTRY_SINISTER_REFLECTION, 23, reflectionCleanup)
