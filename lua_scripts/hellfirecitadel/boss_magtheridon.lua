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
-- victim:IsPlayer()), SAY_BANISHED=3 (shadow cage), SAY_COLLAPSE
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
-- shake 36455 (kalecgos convention; the react-passive/
-- AttackStop arms have no bridges), 6s -> 4s -> start the 20s
-- debris loop, non-triggered DoCastAOE self-cast debris 30630
-- (the instance SetData COLLAPSE / COLLAPSE_2 arms and the
-- world-trigger 36449 cast via instance->GetCreature(DATA_
-- WORLD_TRIGGER) have no instance/cross-creature bridges; the
-- react-state flip back to aggressive has no react bridge).
-- Damage is NOT rewritten (C++-exact — the arm only gates on
-- the 30% line). OnSpellHit(14): hit by 30168 (shadow cage) ->
-- Talk(SAY_BANISHED) (muru-portal event-14 precedent). OnTarget-
-- Died(3): victim:IsPlayer() -> Talk(SAY_SLAY) (C++-exact
-- TYPEID gate, gruul convention). OnDied(4): Talk(SAY_DEATH) +
-- cleanup (the _JustDied arm and the instance MANTICRON_CUBE
-- disable are blocked). OnLeaveCombat(2)/OnReset(23): cancel
-- timers, drop per-GUID state (the evade-side summons.
-- DespawnAll + instance-disable arms are summon/instance
-- blocked). Hellfire Channeler (17256). OnReset(23): per-GUID
-- state cleared, non-triggered self-cast 30207 (the REACT_
-- DEFENSIVE arm has no react bridge). OnEnterCombat(1): the
-- magtheridon->DoAction(ACTION_START_CHANNELERS_EVENT) relay
-- is cross-creature blocked (documented below); the self-
-- contained timers arm normally: shadow bolt volley 30510 20s
-- then {15s,20s}, non-triggered DoCastAOE self-cast
-- (kalecgos convention); fear 30530 {15s,20s} then {25s,40s}
-- on a random alive player in the instance (C++ SelectTarget(
-- Random, 1) — teron convention; nil pick keeps the schedule,
-- burn convention); abyssal 30511 30s then 60s, non-triggered
-- DoCastVictim (jeklik convention). The InterruptNonMeleeSpells
-- arm has no interrupt bridge (krosh convention). OnDied(4):
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
-- group summon skipped; no react/flag bridge — the passive/
-- aggressive/react-defensive, NOT_SELECTABLE and immune-to-PC
-- arms unmodeled; no movement/interrupt/distance bridges — the
-- quake/blaze SPELLVALUE target-count args, the channeler
-- InterruptNonMeleeSpells and the DoSelectBelowHpPctFriendly
-- dark-mending pick (no creature-enumeration bridge) unmodeled;
-- no SpellHitTarget bridge — event 15 never fires in the Go
-- engine (eredar-twins standing gap; nothing in this file uses
-- it); no SpellScript/AuraScript bridge — spell_magtheridon_
-- blaze_target (30541-hit -> 30542 blaze on the hit unit),
-- spell_magtheridon_shadow_grasp (30410-remove -> interrupt +
-- triggered 44032 mind exhaustion) and spell_magtheridon_
-- shadow_grasp_visual (30166-apply x5 -> triggered 30168
-- shadow cage; -remove -> remove 30168) unmodeled (standing
-- gap); no UNIT_STATE bridge (jeklik convention, above). go_
-- manticron_cube (181713) not registered: the GO gossip bridge
-- exists (RegisterGameObjectGossipEvent) but the cube's only
-- functional arms — the FindNearestCreature(17376, 10yd)
-- trigger cast of 30166 (no nearest-creature bridge) and the
-- player's triggered self-cast of 30410 (no player CastSpell
-- bridge) — are unbridgeable; registering the gossip for the
-- aura gates alone would be a stub (firesworn convention, cf.
-- the karazhan barnes note).

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
local SPELL_SHADOW_CAGE = 30168

local SAY_TAUNT = 0
local SAY_FREE = 1
local SAY_SLAY = 2
local SAY_BANISHED = 3
local SAY_COLLAPSE = 4
local SAY_DEATH = 5
local EMOTE_BREAKS_FREE = 8
local EMOTE_BLAST_NOVA = 9

local timers = {}
local magState = {}
local chanState = {}
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
    creature:CastSpell(creature, SPELL_SHADOW_CAGE_C)
    magState[guid] = { phase = "banish", channelers = 5 }
    schedule(guid, "taunt", math.random(240000, 300000), function()
        magtheridonTaunt(creature, guid)
    end)
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
-- RemoveAura(30205), phase released, the C++ timer set.
local function magtheridonEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    magState[guid] = { phase = "released", channelers = 5 }
    creature:Talk(EMOTE_BREAKS_FREE)
    creature:Talk(SAY_FREE)
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

-- C++ DamageTaken: below 30% (once) -> collapse arms.
local function magtheridonDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local st = magState[guid]
    if not st or st.phase == "collapsed" then
        return
    end
    local health = creature:GetHealth()
    local maxHealth = creature:GetMaxHealth()
    if maxHealth > 0 and health > damage
            and (health - damage) <= maxHealth * 0.3 then
        st.phase = "collapsed"
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

-- C++ KilledUnit: TYPEID_PLAYER gate (C++-exact via IsPlayer()).
local function magtheridonTargetDied(event, creature, victim)
    if victim:IsPlayer() then
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
-- arm has no react bridge).
local function channelerReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    chanState[guid] = nil
    creature:CastSpell(creature, SPELL_SHADOW_GRASP_C)
end

-- C++ JustEngagedWith: the magtheridon DoAction relay is
-- cross-creature blocked (header); the interrupt arm has no
-- bridge; the self-contained timers arm normally.
local function channelerEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    chanState[guid] = true
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
end

local function channelerLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    chanState[guid] = nil
end

RegisterCreatureEvent(ENTRY_CHANNELER, 1, channelerEnterCombat)
RegisterCreatureEvent(ENTRY_CHANNELER, 2, channelerLeaveCombat)
RegisterCreatureEvent(ENTRY_CHANNELER, 4, channelerDied)
RegisterCreatureEvent(ENTRY_CHANNELER, 23, channelerReset)

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
    creature:CastSpell(creature, SPELL_DEBRIS_VISUAL)
    roomTimers[guid] = CreateLuaEvent(function()
        creature:CastSpell(creature, SPELL_DEBRIS_DAMAGE)
        roomTimers[guid] = nil
    end, 5000)
end

RegisterCreatureEvent(ENTRY_MAGTHERIDON_ROOM, 23, roomReset)
