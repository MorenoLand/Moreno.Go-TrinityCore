-- Magtheridon (Magtheridon's Lair) — Lua port of
-- src/server/scripts/Outland/HellfireCitadel/MagtheridonsLair/
-- boss_magtheridon.cpp (boss_magtheridon, npc_hellfire_channeler,
-- npc_magtheridon_room; go_manticron_cube and the three spell_*
-- scripts documented below, not registered); magtheridons_lair.h
-- (NPC_MAGTHERIDON = 17257, NPC_HELLFIRE_CHANNELLER = 17256,
-- NPC_MAGTHERIDON_ROOM = 17516, GO_MANTICRON_CUBE = 181713 —
-- all verifiable from the C++ sources; the C++ ScriptNames are
-- "boss_magtheridon", "npc_hellfire_channeler" and
-- "npc_magtheridon_room" per AddSC_boss_magtheridon; the
-- creature_template ScriptName bindings are DB-side, no TDB in
-- this workspace). Entries registered: 17257 Magtheridon,
-- 17256 Hellfire Channeler, 17516 Magtheridon Room. Talk lines
-- used: SAY_TAUNT=0 (banished taunt), SAY_FREE=1 (break free),
-- SAY_SLAY=2 (kill — player-gated in C++, C++-exact via
-- victim:GetObjectType() == "Player"), SAY_BANISHED=3 (shadow cage), SAY_COLLAPSE
-- =4 (30% collapse), SAY_DEATH=5 (death), EMOTE_WEAKEN=6
-- (unmodeled — the DoAction arm that fires it is cross-creature
-- blocked, below), EMOTE_NEARLY_FREE=7 (unmodeled — the
-- EVENT_NEARLY_EMOTE arm is scheduled by the same blocked
-- DoAction arm, below), EMOTE_BREAKS_FREE=8 (break free),
-- EMOTE_BLAST_NOVA=9 (blast nova). Eluna creature events: 1
-- OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied, 4 OnDied, 9
-- OnDamageTaken, 14 OnSpellHit, 23 OnReset. Timers via
-- CreateLuaEvent (per-GUID named schedule helper); melee is
-- engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. C++ DoCast default is triggered=false
-- (vaelastrasz convention); DoCastVictim/DoCastAOE take nil as
-- the victim arm (felmyst convention).
-- Fight shape (C++-exact for the modeled arms): Magtheridon
-- (17257). OnReset(23): per-GUID state cleared (the _Reset
-- instance bookkeeping is blocked on the instance-script
-- model), non-triggered self-cast 30205 (C++ DoCastSelf(SPELL_
-- SHADOW_CAGE_C)), channelers=5 (C++-exact init — the
-- SummonedCreatureDies decrement relay is cross-creature
-- blocked), phase="banish", taunt timer armed {4min,5min}
-- repeat -> Talk(SAY_TAUNT) (C++-exact schedule; the
-- SummonCreatureGroup(1) arm is summon-blocked). OnEnterCombat
-- (1): see the deviation note — the fight starts on pull in
-- this model (ragnaros-intro convention): cancel the taunt
-- timer, Talk(EMOTE_BREAKS_FREE) + Talk(SAY_FREE),
-- RemoveAura(30205), phase="released", then the C++ RELEASED
-- timer set: cleave 30619 10s then 10s non-triggered
-- DoCastVictim (nil ticks cast nothing but keep the schedule,
-- jeklik convention); blast nova 30616 60s then 55s,
-- Talk(EMOTE_BLAST_NOVA) + non-triggered DoCastAOE self-cast
-- (kalecgos convention); blaze 30541 20s then 20s, non-
-- triggered DoCastAOE self-cast (the SPELLVALUE_MAX_TARGETS=1
-- cast arg has no bridge — kalecgos convention); quake 30657
-- 35s then 60s, non-triggered DoCastAOE self-cast (the
-- SPELLVALUE_MAX_TARGETS=5 cast arg has no bridge — same);
-- berserk 27680 20min one-shot, non-triggered self-cast. The
-- UNIT_STATE_CASTING queue gate has no bridge — timers fire
-- unconditionally (jeklik convention). OnDamageTaken(9, pre-
-- damage hook): incoming damage would take health to <=30% of
-- max and phase ~= "collapsed" -> phase="collapsed",
-- Talk(SAY_COLLAPSE), non-triggered DoCastAOE self-cast camera
-- shake 36455 (kalecgos convention; the react bridge exists —
-- creature:SetReactState — but the collapse PASSIVE arm stays out:
-- its AGGRESSIVE restore rides the unported world-trigger machine,
-- so the flip alone would strand the boss passive; the AttackStop
-- arm has no bridge), 6s -> 4s -> start the 20s
-- (the instance SetData COLLAPSE / COLLAPSE_2 arms and the
-- world-trigger 36449 cast via instance->GetCreature(DATA_
-- WORLD_TRIGGER) have no instance/cross-creature bridges; the
-- react-state flip back to aggressive stays out — the react bridge
-- exists (creature:SetReactState) but the world-trigger machine is
-- unported).
-- Damage is NOT rewritten (C++-exact — the arm only gates on
-- the 30% line). OnSpellHit(14): hit by 30168 (shadow cage) ->
-- Talk(SAY_BANISHED) (muru-portal event-14 precedent). OnTarget-
-- Died(3): victim:GetObjectType() == "Player" -> Talk(SAY_SLAY) (C++-exact
-- TYPEID gate, gruul convention). OnDied(4): Talk(SAY_DEATH) +
-- cleanup (the _JustDied arm and the instance MANTICRON_CUBE
-- disable are blocked). OnLeaveCombat(2)/OnReset(23): cancel
-- timers, drop per-GUID state (the evade-side summons.
-- DespawnAll + instance-disable arms are summon/instance
-- blocked). Hellfire Channeler (17256). OnReset(23): per-GUID
-- state cleared, non-triggered self-cast 30207 (the REACT_DEFENSIVE
-- arm is PORTED 2026-10-20 — creature:SetReactState).
-- OnEnterCombat(1): the
-- magtheridon->DoAction(ACTION_START_CHANNELERS_EVENT) relay
-- is cross-creature blocked (documented below); the self-
-- contained timers arm normally: shadow bolt volley 30510 20s
-- then {15s,20s}, non-triggered DoCastAOE self-cast
-- (kalecgos convention); fear 30530 {15s,20s} then {25s,40s}
-- on a random alive player in the instance (C++ SelectTarget(
-- Random, 1) — teron convention; nil pick keeps the schedule,
-- burn convention); abyssal 30511 30s then 60s, non-triggered
-- DoCastVictim (jeklik convention). The InterruptNonMeleeSpells
-- arm is PORTED (InterruptNonMeleeSpells(false) first in
-- channelerEnterCombat, boss_magtheridon.cpp:335). OnDied(4):
-- non-triggered DoCastAOE self-cast soul transfer 30531
-- (kalecgos convention) + cleanup (the "Hit Kill" DoAction
-- relay is cross-creature blocked). OnLeaveCombat(2)/
-- OnReset(23): cancel timers, drop per-GUID state. Magtheridon
-- Room (17516, PassiveAI). OnReset(23): non-triggered self-cast
-- 30632, then 5s -> non-triggered DoCastAOE self-cast 30631
-- (kalecgos convention). The Go gossip object (181713) and the
-- three spell scripts are unmodeled (standing gaps — below).
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI shim
-- (the SetBossState/SetData/GetCreature instance arms and the
-- _Reset/_JustDied bookkeeping skipped); no cross-creature
-- bridge — the channeler->magtheridon ACTION_START_CHANNELERS_
-- EVENT relay, the "Hit Kill" relay, the magtheridon break-free
-- phase machine (PHASE_BANISH -> PHASE_1 -> CombatStart ->
-- RELEASED, including the 2min auto-start and the 6s release
-- delay) and the world-trigger debris-knockdown cast
-- unmodeled — so the port treats magtheridon's own pull as the
-- release (EMOTE_BREAKS_FREE + SAY_FREE + the RELEASED timer
-- set, C++-exact values); no summon bridge — the channeler
-- group summon skipped; the magtheridon passive/aggressive arms stay
-- out on the unported collapse machine; the EVENT_RELEASED
-- RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE) leg is
-- PORTED (boss_magtheridon.cpp:253, magtheridonEnterCombat below —
-- the flag bridge is live since e9387671; the old "no flag bridge"
-- claim is retracted for this leg); the paired SetImmuneToPC(false)
-- leg IS bridged (RemoveFlag(59, 0x100)) and ported in C++ order at
-- magtheridonEnterCombat (boss_magtheridon.cpp:254); the channeler REACT_DEFENSIVE arm
-- is PORTED 2026-10-20 (creature:SetReactState);
-- no distance bridge — the
-- quake/blaze SPELLVALUE target-count args stay unmodeled; the
-- DoSelectBelowHpPctFriendly dark-mending pick is PORTED below
-- (channelerCheckFriend: 1s pump, 17256/30yd/HealthBelowPct(51)
-- grid pick, non-triggered 30528, {10s,20s} re-enable latch;
-- boss_magtheridon.cpp:395-407, GridNotifiers.h:855-867)
-- (the channeler InterruptNonMeleeSpells leg is PORTED above);
-- the "no SpellHitTarget bridge / event 15 never fires" claim is
-- STALE — event 15 (CREATURE_EVENT_ON_SPELL_HIT_TARGET) is bridged
-- (boss_ai.go:2097-2115; the arugal precedent). The boss's
-- SpellHit(SPELL_SHADOW_CAGE 30168) -> Talk(SAY_BANISHED) arm
-- (:187-191) is event-14-side and is ALREADY ported
-- (magtheridonSpellHit below, registered :364; the muru-portal
-- precedent). Corrected 2026-10-17.
-- No SpellScript/AuraScript bridge — spell_magtheridon_
-- blaze_target (30541-hit -> 30542 blaze on the hit unit),
-- spell_magtheridon_shadow_grasp (30410-remove -> interrupt +
-- triggered 44032 mind exhaustion) and spell_magtheridon_
-- shadow_grasp_visual (30166-apply x5 -> triggered 30168
-- shadow cage; -remove -> remove 30168) unmodeled (standing
-- gap); no live UNIT_STATE_CASTING read bridge (IsCasting is constant-false — Go casts are fire-and-forget) (jeklik convention, above). go_
-- manticron_cube (181713) not registered: the GO gossip bridge
-- exists (RegisterGameObjectGossipEvent) but the cube's only
-- functional arms — the player->FindNearestCreature(17376, 10yd)
-- trigger cast of 30166 and the player's triggered self-cast of
-- 30410 (boss_magtheridon.cpp:469-491, go_manticron_cubeAI::
-- OnGossipHello) — stay unbridgeable: the nearest-creature bridge
-- exists only on the creature handler surface (lua_creature_events.
-- go:1237; player->FindNearestCreature has no bridge), player->
-- CastSpell has no bridge, and the GO AI itself has no registered
-- handler; registering the gossip for the aura gates alone would
-- be a stub (firesworn convention, cf. the karazhan barnes note).
-- Corrected 2026-10-22 (stale "no nearest-creature bridge" claim
-- retracted — the bridge exists, this arm just can't reach it).

local ENTRY_MAGTHERIDON = 17257
local ENTRY_CHANNELER = 17256
local ENTRY_MAGTHERIDON_ROOM = 17516

local SPELL_SHADOW_CAGE_C = 30205
local SPELL_SHADOW_GRASP_C = 30207
local SPELL_CLEAVE = 30619
local SPELL_BLAST_NOVA = 30616
local SPELL_BLAZE_TARGET = 30541
local SPELL_QUAKE = 30657
local SPELL_BERSERK = 27680
local SPELL_CAMERA_SHAKE = 36455
local SPELL_DEBRIS = 30630
local SPELL_DEBRIS_VISUAL = 30632
local SPELL_DEBRIS_DAMAGE = 30631
local SPELL_SHADOW_BOLT_VOLLEY = 30510
local SPELL_FEAR = 30530
local SPELL_ABYSSAL = 30511
local SPELL_SOUL_TRANSFER = 30531
local SPELL_DARK_MENDING = 30528
local SPELL_SHADOW_CAGE = 30168

local SAY_TAUNT = 0
local SAY_FREE = 1
local SAY_SLAY = 2
local SAY_BANISHED = 3
local SAY_COLLAPSE = 4
local SAY_DEATH = 5
local EMOTE_BREAKS_FREE = 8
local EMOTE_BLAST_NOVA = 9

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000 -- boss_magtheridon.cpp:253

local timers = {}
local magState = {}
local chanState = {}
local chanCanMend = {}
local roomTimers = {}

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

local function pickRandomPlayer(creature)
    local players = playersInInstance(creature)
    if #players == 0 then
        return nil
    end
    return players[math.random(#players)]
end

-- ================= Magtheridon (17257) =================

local function magtheridonTaunt(creature, guid)
    creature:Talk(SAY_TAUNT)
    schedule(guid, "taunt", math.random(240000, 300000), function()
        magtheridonTaunt(creature, guid)
    end)
end

-- C++ Reset: non-triggered self-cast 30205, phase BANISH,
-- channelers 5, taunt {4min,5min} repeat.
local function magtheridonReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    magState[guid] = nil
    -- The cast and the taunt scheduling are Reset-only arms
    -- (boss_magtheridon.cpp; C++ BossAI::_JustDied cancels the
    -- scheduler, so they never fire after death). Event 23 fires on
    -- death too (engine reset-then-died order), so both are
    -- alive-gated; the timer cancel and state legs ride the death
    -- leg deliberately.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_SHADOW_CAGE_C)
        magState[guid] = { phase = "banish", channelers = 5 }
        schedule(guid, "taunt", math.random(240000, 300000), function()
            magtheridonTaunt(creature, guid)
        end)
    end
end

local function magtheridonCleave(creature, guid)
    creature:CastSpell(nil, SPELL_CLEAVE)
    schedule(guid, "cleave", 10000, function()
        magtheridonCleave(creature, guid)
    end)
end

local function magtheridonBlastNova(creature, guid)
    creature:Talk(EMOTE_BLAST_NOVA)
    creature:CastSpell(creature, SPELL_BLAST_NOVA)
    schedule(guid, "blastnova", 55000, function()
        magtheridonBlastNova(creature, guid)
    end)
end

local function magtheridonBlaze(creature, guid)
    creature:CastSpell(creature, SPELL_BLAZE_TARGET)
    schedule(guid, "blaze", 20000, function()
        magtheridonBlaze(creature, guid)
    end)
end

local function magtheridonQuake(creature, guid)
    creature:CastSpell(creature, SPELL_QUAKE)
    schedule(guid, "quake", 60000, function()
        magtheridonQuake(creature, guid)
    end)
end

local function magtheridonDebris(creature, guid)
    creature:CastSpell(creature, SPELL_DEBRIS)
    schedule(guid, "debris", 20000, function()
        magtheridonDebris(creature, guid)
    end)
end

-- C++ CombatStart + EVENT_RELEASED collapsed onto the pull (see
-- the header deviation): EMOTE_BREAKS_FREE + SAY_FREE,
-- RemoveAura(30205), RemoveFlag(UNIT_FIELD_FLAGS,
-- UNIT_FLAG_NOT_SELECTABLE) (EVENT_RELEASED, boss_magtheridon.cpp:253;
-- the paired SetImmuneToPC(false) leg has no bridge, stays out),
-- phase released, the C++ timer set.
local function magtheridonEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    magState[guid] = { phase = "released", channelers = 5 }
    creature:Talk(EMOTE_BREAKS_FREE)
    creature:Talk(SAY_FREE)
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    -- C++ EVENT_RELEASED (boss_magtheridon.cpp:253-254), in C++ order
    -- right after the NOT_SELECTABLE clear: SetImmuneToPC(false) ≡
    -- RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC 0x100).
    creature:RemoveFlag(59, 0x100)
    creature:RemoveAura(SPELL_SHADOW_CAGE_C)
    schedule(guid, "cleave", 10000, function()
        magtheridonCleave(creature, guid)
    end)
    schedule(guid, "blastnova", 60000, function()
        magtheridonBlastNova(creature, guid)
    end)
    schedule(guid, "blaze", 20000, function()
        magtheridonBlaze(creature, guid)
    end)
    schedule(guid, "quake", 35000, function()
        magtheridonQuake(creature, guid)
    end)
    schedule(guid, "berserk", 1200000, function()
        creature:CastSpell(creature, SPELL_BERSERK)
    end)
end

-- C++ DamageTaken: HealthBelowPctDamaged(30, damage) (Unit.h:873 --
-- int64(health) - int64(damage) < CountPctFromMaxHealth(30), so killing
-- and overkill blows fire the arms too) -> collapse arms, once.
local function magtheridonDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local st = magState[guid]
    if not st or st.phase == "collapsed" then
        return
    end
    local health = creature:GetHealth()
    local maxHealth = creature:GetMaxHealth()
    if maxHealth > 0 and health - damage < math.floor(maxHealth * 30 / 100) then
        st.phase = "collapsed"
        -- C++ boss_magtheridon.cpp:199: me->AttackStop() (arm order
        -- REACT_PASSIVE flip -> AttackStop -> Talk; the stop is a
        -- one-shot, melee re-arms on the post-collapse flip via the
        -- Go combat tick).
        creature:AttackStop()
        creature:Talk(SAY_COLLAPSE)
        creature:CastSpell(creature, SPELL_CAMERA_SHAKE)
        schedule(guid, "collapse", 6000, function()
            schedule(guid, "debrisknockdown", 4000, function()
                magtheridonDebris(creature, guid)
            end)
        end)
    end
end

-- C++ SpellHit: hit by 30168 -> SAY_BANISHED.
local function magtheridonSpellHit(event, creature, caster, spellId)
    if spellId == SPELL_SHADOW_CAGE then
        creature:Talk(SAY_BANISHED)
    end
end

-- C++ KilledUnit: TYPEID_PLAYER gate (C++-exact via GetObjectType() == "Player").
local function magtheridonTargetDied(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ JustDied: Talk(SAY_DEATH); the _JustDied arm is blocked.
local function magtheridonDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    cancelTimers(creature:GetGUID())
    magState[creature:GetGUID()] = nil
end

local function magtheridonLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    magState[guid] = nil
end

RegisterCreatureEvent(ENTRY_MAGTHERIDON, 1, magtheridonEnterCombat)
RegisterCreatureEvent(ENTRY_MAGTHERIDON, 2, magtheridonLeaveCombat)
RegisterCreatureEvent(ENTRY_MAGTHERIDON, 3, magtheridonTargetDied)
RegisterCreatureEvent(ENTRY_MAGTHERIDON, 4, magtheridonDied)
RegisterCreatureEvent(ENTRY_MAGTHERIDON, 9, magtheridonDamageTaken)
RegisterCreatureEvent(ENTRY_MAGTHERIDON, 14, magtheridonSpellHit)
RegisterCreatureEvent(ENTRY_MAGTHERIDON, 23, magtheridonReset)

-- ================= Hellfire Channeler (17256) =================

local function channelerShadowBolt(creature, guid)
    creature:CastSpell(creature, SPELL_SHADOW_BOLT_VOLLEY)
    schedule(guid, "shadowbolt", math.random(15000, 20000), function()
        channelerShadowBolt(creature, guid)
    end)
end

local function channelerFear(creature, guid)
    local pick = pickRandomPlayer(creature)
    if pick then
        creature:CastSpell(pick, SPELL_FEAR)
    end
    schedule(guid, "fear", math.random(25000, 40000), function()
        channelerFear(creature, guid)
    end)
end

local function channelerAbyssal(creature, guid)
    creature:CastSpell(nil, SPELL_ABYSSAL)
    schedule(guid, "abyssal", 60000, function()
        channelerAbyssal(creature, guid)
    end)
end

-- C++ Reset: non-triggered self-cast 30207 (the REACT_DEFENSIVE
-- arm is PORTED 2026-10-20 — creature:SetReactState).
local function channelerReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    chanState[guid] = nil
    chanCanMend[guid] = nil
    -- C++ Reset: non-triggered self-cast (Reset-only arm,
    -- boss_magtheridon.cpp). Event 23 fires on death too (engine
    -- reset-then-died order) but C++ Reset() never runs on death,
    -- so the cast is alive-gated.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_SHADOW_GRASP_C)
        -- C++ boss_magtheridon.cpp:330: me->SetReactState(REACT_DEFENSIVE)
        -- (channeler Reset order: cast -> flip).
        creature:SetReactState(1) -- REACT_DEFENSIVE
    end
end

-- C++ npc_hellfire_channelerAI::EVENT_CHECK_FRIEND (boss_magtheridon.cpp:395-403,
-- init 1s, Repeat 1s): if _canCastDarkMending, target =
-- DoSelectBelowHpPctFriendlyWithEntry(17256, 30.0f, 51, false) -> friendly
-- 17256 within 30yd, alive, in combat, HealthBelowPct(51)
-- (GridNotifiers.h:855-867); non-triggered DoCast(30528), latch false,
-- EVENT_DARK_MENDING {10s,20s} re-enables. C++ pick is grid-visit-last
-- (unspecified order); Lua takes the first GUID-sorted grid match.
-- Hostility gate vacuous: entry-filtered 17256 peers share faction
-- (IsHostileTo has no creature-surface bridge). UpdateAI casting gate
-- unmodeled (jeklik convention).
local function channelerCheckFriend(creature, guid)
    if chanCanMend[guid] then
        local target = nil
        for _, other in ipairs(creature:GetCreaturesInGrid(ENTRY_CHANNELER, 30.0)) do
            if not other:IsDead() and other:IsInCombat() and other:GetHealthPct() < 51 then
                target = other
                break
            end
        end
        if target then
            creature:CastSpell(target, SPELL_DARK_MENDING)
            chanCanMend[guid] = false
            schedule(guid, "darkmending", math.random(10000, 20000), function()
                chanCanMend[guid] = true
            end)
        end
    end
    schedule(guid, "checkfriend", 1000, function()
        channelerCheckFriend(creature, guid)
    end)
end

-- C++ JustEngagedWith: the magtheridon DoAction relay is
-- cross-creature blocked (header); the interrupt arm is PORTED
-- (boss_magtheridon.cpp:335, first in arm order).
local function channelerEnterCombat(event, creature)
    creature:InterruptNonMeleeSpells(false)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    chanState[guid] = true
    chanCanMend[guid] = true
    schedule(guid, "checkfriend", 1000, function()
        channelerCheckFriend(creature, guid)
    end)
    schedule(guid, "shadowbolt", 20000, function()
        channelerShadowBolt(creature, guid)
    end)
    schedule(guid, "fear", math.random(15000, 20000), function()
        channelerFear(creature, guid)
    end)
    schedule(guid, "abyssal", 30000, function()
        channelerAbyssal(creature, guid)
    end)
end

-- C++ JustDied: non-triggered DoCastAOE soul transfer (kalecgos
-- convention); the "Hit Kill" DoAction relay is cross-creature
-- blocked.
local function channelerDied(event, creature, killer)
    creature:CastSpell(creature, SPELL_SOUL_TRANSFER)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    chanState[guid] = nil
    chanCanMend[guid] = nil
end

local function channelerLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    chanState[guid] = nil
    chanCanMend[guid] = nil
end

-- C++ npc_hellfire_channelerAI::JustSummoned (boss_magtheridon.cpp:356-362):
-- the magtheridon->AI()->JustSummoned(summon) relay is cross-creature
-- blocked (header); the C++ DoZoneInCombat(summon) arm ports via the
-- summon-object receiver (mandokir precedent — DoZoneInCombat twins
-- CreatureAI.cpp:73-103 on any creature object; broggok
-- onJustSummoned precedent for the event-19 shape).
local function channelerJustSummoned(_, creature, summon)
    summon:DoZoneInCombat()
end

RegisterCreatureEvent(ENTRY_CHANNELER, 1, channelerEnterCombat)
RegisterCreatureEvent(ENTRY_CHANNELER, 2, channelerLeaveCombat)
RegisterCreatureEvent(ENTRY_CHANNELER, 4, channelerDied)
RegisterCreatureEvent(ENTRY_CHANNELER, 23, channelerReset)
RegisterCreatureEvent(ENTRY_CHANNELER, 19, channelerJustSummoned)

-- ================= Magtheridon Room (17516) =================

-- C++ Reset: non-triggered self-cast 30632, then 5s ->
-- non-triggered DoCastAOE self-cast 30631 (kalecgos
-- convention).
local function roomReset(event, creature)
    local guid = creature:GetGUID()
    local per = roomTimers[guid]
    if per then
        RemoveEventById(per)
        roomTimers[guid] = nil
    end
    -- C++ Reset: non-triggered self-cast 30632, then 5s ->
    -- non-triggered DoCastAOE self-cast 30631 (Reset-only arms;
    -- the C++ scheduler never updates on a corpse). Event 23
    -- fires on death too (engine reset-then-died order), so the
    -- cast and the 5s timer that delivers it are alive-gated;
    -- the timer cancel rides the death leg deliberately.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_DEBRIS_VISUAL)
        roomTimers[guid] = CreateLuaEvent(function()
            creature:CastSpell(creature, SPELL_DEBRIS_DAMAGE)
            roomTimers[guid] = nil
        end, 5000)
    end
end

RegisterCreatureEvent(ENTRY_MAGTHERIDON_ROOM, 23, roomReset)
RegisterCreatureEvent(ENTRY_MAGTHERIDON_ROOM, 1, function(_, creature)
    creature:SetMeleeAttack(false) -- boss_magtheridon.cpp:454: custom UpdateAI never calls DoMeleeAttackIfReady (scheduler-only UpdateAI in C++, never melees)
end)
