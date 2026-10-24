-- Prince Keleseth (Utgarde Keep) — Lua port of
-- src/server/scripts/Northrend/UtgardeKeep/UtgardeKeep/boss_keleseth.cpp
-- (390 lines; 5 scripts — boss_keleseth (CreatureScript via
-- GetUtgardeKeepAI (BossAI), DATA_PRINCE_KELESETH = 0); npc_frost_tomb
-- (23965) / npc_vrykul_skeleton (23970) (CreatureScript via
-- GetUtgardeKeepAI (ScriptedAI)); spell_frost_tomb (SpellScriptLoader,
-- AuraScript OnRemove); achievement_on_the_rocks
-- (AchievementCriteriaScript, OnCheck GetData DATA_ON_THE_ROCKS); all
-- registered from inside AddSC_boss_keleseth(); loader decl 126 / call
-- 321 per northrend_script_loader.cpp — the FIRST group of the
-- "// Utgarde Keep - Utgarde Keep" block in AddNorthrendScripts(),
-- immediately after AddSC_instance_ulduar() (call 319); the call after
-- it is AddSC_boss_skarvald_dalronn() — verified from the loader; the
-- checkpoint sequence (instance_ulduar -> boss_keleseth) is followed).
-- Entry: 23953 Prince Keleseth (utgarde_keep.h NPC_PRINCE_KELESETH, line
-- 46 — entry-verifiable, registration proceeds (the nexus_commanders /
-- malygos / sartharion kalecgos precedent); the CreatureScript
-- ScriptName binding is DB-side as usual). DATA_PRINCE_KELESETH = 0
-- (utgarde_keep.h line 31).
-- Sole-source verified: whole-server-tree grep for each of the five
-- script names hits boss_keleseth.cpp (+ the loader decl/call lines for
-- AddSC_boss_keleseth) only; this clone carries no sql/ tree, so
-- ScriptName bindings are DB-side by construction.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timer-driven casts ride the CreateLuaEvent convention
-- (engine/scripting/runtime.go:798; the maiden_of_virtue / moroes /
-- curator / nightbane / baltharus precedent): module-local GUID-keyed
-- timers table, cancelTimers on events 2/4/23, schedule(guid, key,
-- delayMs, fn). creature:CastSpell(nil, spell) = DoCastVictim (2-arg,
-- non-triggered); creature:CastSpell(creature, spell) = DoCastSelf /
-- DoCastAOE (the boss_ai.go DoCastAOE lowering; the selin_fireheart /
-- mr_smite precedent); the 3rd CastSpell arg is the C++ DoCast 3rd arg
-- (2-arg / 1-arg overloads = non-triggered); {minMs, maxMs} table delay
-- = randtime range. Melee is engine-driven in Go, like C++
-- DoMeleeAttackIfReady. No UNIT_STATE_CASTING model in Go, so timers
-- fire unconditionally (the maiden / hodir precedent). The 2026-10-02
-- "no-timer-bridge" header doctrine is RETRACTED this run:
-- CreateLuaEvent is a real registered bridge, so the ungated timer arms
-- below are ported rather than parked.
-- Talk ids (KelsethEncounter, boss_keleseth.cpp:52-56, verified this
-- run): SAY_START_COMBAT 1, SAY_SUMMON_SKELETONS 2, SAY_FROST_TOMB 3,
-- SAY_FROST_TOMB_EMOTE 4, SAY_DEATH 5.
-- Ported arms (C++-exact for all modeled arms; line refs are
-- boss_keleseth.cpp):
-- JustEngagedWith (:144-164) — Talk(SAY_START_COMBAT 1) (event 1);
-- the Talk fires unconditionally in C++ (:146-147, before the !who
-- early return at :149-150) and the guard AttackStart leg
-- (NPC_RUNEMAGE 23960 / NPC_STRATEGIST 23956 grid search 60.0f, LOS,
-- AttackStart, :153-160) has no bridges — the auriaya engage-port
-- precedent. The Reset init schedules (:137-139) are taken over by the
-- event-1 handler (the xt002 precedent: C++ schedules on Reset, Go
-- schedules on combat start, same inits).
-- EVENT_SHADOWBOLT (id 1; init 2-3s :137, arm :203-205):
-- DoCastVictim(SPELL_SHADOWBOLT 43667), 1-arg = non-triggered ->
-- CastSpell(nil, 43667); repeat {2s,3s}.
-- EVENT_FROST_TOMB (id 2; init 14-19s :138, arm :207-217):
-- SelectTarget(Random, 0, 100.0f, true, true, -SPELL_FROST_TOMB 48400)
-- -> randomFrostTombTarget (threat list via the handler GetAITargets
-- twin — C++-exact pool, retargeted 2026-10-18 off the retired
-- GetPlayersInWorld 100yd approx; same map/instance, alive, within
-- 100yd, minus aura 48400 — the baltharus ENERVATING_BRAND precedent); Talk(SAY_FROST_TOMB 3);
-- Talk(SAY_FROST_TOMB_EMOTE 4, target) — the targeted-emote whisper
-- target is bridgeable (the mandokir SAY_WATCH / razorscale
-- EMOTE_BERSERK precedent); DoCast(target, SPELL_FROST_TOMB_STUN
-- 42672, true) -> CastSpell(target, 42672, true); target->CastSpell
-- (target, SPELL_FROST_TOMB_SUMMON 42714, true) -> creature:CastSpell
-- (target, 42714, true) — the C++ comment says "checked from sniffs -
-- the player casts the spell", but player:CastSpell has no bridge
-- (CastSpell is creature-only, engine/world/lua_creature_events.go:60),
-- so the caster deviates from C++ (the moroes GARROTE target-self-cast
-- shape is unbridgeable); repeat {14s,19s} regardless of target found,
-- C++-exact.
-- JustDied (:165-169) — Talk(SAY_DEATH 5) (event 4 — the sjonnir
-- JustDied-Talk precedent); the BossAI::_JustDied() passthrough has no
-- bridge; the combat timers are wiped with it.
-- EVENT_SUMMON_SKELETONS (id 3; init 6s :139, arm :199-202):
-- Talk(SAY_SUMMON_SKELETONS 2) + SummonSkeletons() (4x SummonCreature
-- NPC_SKELETON 23970 at {156.2559, 259.2093}, Z 42.8668, :230-234 —
-- "I could not found any spell cast for this", :231) — summon bridge
-- live 2026-10-22 (5-arg map summon = TEMPSUMMON_MANUAL_DESPAWN =
-- Lua type 7 default); fires once in C++ (no re-arm) -> one-shot
-- schedule in kelesethEnterCombat (Reset-inits-on-combat xt002
-- precedent).
-- DOCUMENTED-ONLY (in this header; registrations are entry 23953 +
-- skeleton entry 23970 below):
-- npc_frost_tomb (NPC_FROSTTOMB 23965, :79-115) — NOT registered (no
-- genuinely bridgeable arms, the yogg_saron adds precedent):
-- IsSummonedBy DoCast(summoner, SPELL_FROST_TOMB 48400, true) (:91-97)
-- — no IsSummonedBy / summoner-carrying spawn bridge in the modeled
-- Eluna creature events; JustDied cross-creature SetData
-- (DATA_ON_THE_ROCKS false) via instance GetGuidData
-- (DATA_PRINCE_KELESETH) (:101-105) — instance-GUID leg, no bridge.
-- (The spell-summoned tomb from the FROST_TOMB arm therefore spawns
-- with no Lua AI — visual only.)
-- npc_vrykul_skeleton (NPC_SKELETON 23970, :247-320) — PORTED this
-- run (skeletonReset / skeletonDamageTaken / skeletonDecrepifyTick /
-- skeletonResurrectTick / skeletonShadowFissureTick; the old "no
-- flag / stand-state bridges" block is stale — SetFlag, SetStandState,
-- HasFlag, event-9 damage rewrite and CreateLuaEvent are all live):
-- Reset decrepify init {4s,6s}, decrepify random-target cast + {1s,5s}
-- re-arm, lethal-damage zero + fake-death (NOT_SELECTABLE + STAND_DEAD
-- + RESURRECT {18s,22s}), resurrect chain (FULL_HEAL + SHADOW_FISSURE
-- + BONE_ARMOR self-cast + flag clear + STAND + decrepify re-arm).
-- Deviations: MotionMaster Clear/MoveIdle (fake-death) and MoveChase
-- (resurrect) stay out — no motion bridge; the add keeps its motion
-- state through the fake-death window. The decrepify init is armed
-- from skeletonTick's returned summon object (temp summons bypass
-- the motion funnel, so event 23 may never fire for them); event 23
-- covers any respawn-path Reset.
-- spell_frost_tomb (AuraScript OnRemove: despawn-or-unsummon the tomb
-- caster when the stun aura is removed by non-death, :341-370) — no
-- AuraScript / SpellScript bridge; joins the no-SpellScript-bridge
-- queue.
-- achievement_on_the_rocks (OnCheck: GetAI()->GetData
-- (DATA_ON_THE_ROCKS), :372-380) — no GetData / achievement bridges;
-- joins the unmodeled-achievement queue.

local ENTRY_KELESETH = 23953
local ENTRY_SKELETON = 23970 -- NPC_SKELETON (boss_keleseth.cpp:47)

local SAY_START_COMBAT = 1
local SAY_SUMMON_SKELETONS = 2 -- KelsethEncounter (boss_keleseth.cpp:53)
local SAY_FROST_TOMB = 3
local SAY_FROST_TOMB_EMOTE = 4
local SAY_DEATH = 5

local SPELL_SHADOWBOLT = 43667
local SPELL_FROST_TOMB = 48400
local SPELL_FROST_TOMB_STUN = 42672
local SPELL_FROST_TOMB_SUMMON = 42714

local SPELL_DECREPIFY = 42702
local SPELL_FULL_HEAL = 17683
local SPELL_SHADOW_FISSURE = 50657
local SPELL_BONE_ARMOR = 59386

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000

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

-- C++ EVENT_SHADOWBOLT (id 1; init :137, arm :203-205):
-- DoCastVictim(SPELL_SHADOWBOLT 43667), 1-arg = non-triggered;
-- re-arm {2s,3s}.
local function shadowboltTick(creature, guid)
    creature:CastSpell(nil, SPELL_SHADOWBOLT)
    schedule(guid, "shadowbolt", {2000, 3000}, function()
        shadowboltTick(creature, guid)
    end)
end

-- C++ EVENT_FROST_TOMB (id 2; arm :207-217): SelectTarget(Random, 0,
-- 100.0f, true, true, -SPELL_FROST_TOMB) — the baltharus
-- ENERVATING_BRAND precedent.
local function randomFrostTombTarget(creature)
    local candidates = {}
    for _, unit in ipairs(creature:GetAITargets()) do
        if unit:GetObjectType() == "Player" and not unit:IsDead()
                and creature:IsWithinDist(unit, 100)
                and not unit:HasAura(SPELL_FROST_TOMB) then
            candidates[#candidates + 1] = unit
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EVENT_FROST_TOMB arm :207-217: Talk(SAY_FROST_TOMB 3) +
-- Talk(SAY_FROST_TOMB_EMOTE 4, target) + DoCast(target,
-- SPELL_FROST_TOMB_STUN 42672, true) + target->CastSpell(target,
-- SPELL_FROST_TOMB_SUMMON 42714, true); re-arm {14s,19s} sits outside
-- the target check — C++-exact.
local function frostTombTick(creature, guid)
    local target = randomFrostTombTarget(creature)
    if target then
        creature:Talk(SAY_FROST_TOMB)
        creature:Talk(SAY_FROST_TOMB_EMOTE, target)
        creature:CastSpell(target, SPELL_FROST_TOMB_STUN, true)
        -- C++ comment: "checked from sniffs - the player casts the
        -- spell" — but player:CastSpell has no bridge (CastSpell is
        -- registered for creature objects only,
        -- engine/world/lua_creature_events.go:60); the target-self-cast
        -- would nil-call and kill this timer chain, so the creature
        -- casts on the target instead (caster deviates from C++).
        creature:CastSpell(target, SPELL_FROST_TOMB_SUMMON, true)
    end
    schedule(guid, "frosttomb", {14000, 19000}, function()
        frostTombTick(creature, guid)
    end)
end

-- C++ JustEngagedWith (:144-164): Talk(SAY_START_COMBAT 1) fires
-- unconditionally (:146-147); the guard AttackStart leg has no bridges
-- (the auriaya engage-port precedent). C++ Reset inits (:137-139) are
-- scheduled here on the xt002 precedent.
-- C++ EVENT_SUMMON_SKELETONS (id 3; init 6s :139, arm :199-202):
-- Talk(SAY_SUMMON_SKELETONS 2) + SummonSkeletons() — 4x
-- me->SummonCreature(NPC_SKELETON, {156.2559, 259.2093}, 42.8668, o=0)
-- (:230-234); one-shot in C++ (no re-arm) -> one-shot schedule here.
-- Forward declaration (tharon_ja / gal_darah idiom): skeletonTick below
-- references skeletonDecrepifyTick, defined after it — a later
-- `local function` re-declare would shadow this slot and nil-call, so
-- the definition below is a plain assignment, never re-local.
local skeletonDecrepifyTick

-- The adds run npc_vrykul_skeleton (handlers below, entry 23970).
local function skeletonTick(creature, guid)
    creature:Talk(SAY_SUMMON_SKELETONS)
    for _ = 1, 4 do
        local skeleton = creature:SummonCreature(ENTRY_SKELETON, 156.2559, 259.2093, 42.8668, 0)
        -- C++ Reset init (:259) rides the summon here: temp summons
        -- bypass the motion funnel, so event 23 may never fire for
        -- them (the curator-flare event-19 arming precedent) — arm
        -- the decrepify init off the returned object instead; the
        -- event-23 handler below covers any respawn-path Reset, and
        -- schedule() replaces the same key, so a double-arm is a
        -- harmless re-init, not a double timer.
        if skeleton then
            local skelGuid = skeleton:GetGUID()
            schedule(skelGuid, "decrepify", {4000, 6000}, function()
                skeletonDecrepifyTick(skeleton, skelGuid)
            end)
        end
    end
end

-- C++ EVENT_DECREPIFY (skeleton arm :299-302): SelectTarget(Random, 0,
-- 0.0f, playerOnly, withTank, -SPELL_DECREPIFY) -> DoCast(target,
-- SPELL_DECREPIFY), 2-arg = non-triggered; re-arm {1s,5s} sits outside
-- the target check — C++-exact.
skeletonDecrepifyTick = function(creature, guid)
    local candidates = {}
    for _, unit in ipairs(creature:GetAITargets()) do
        if unit:GetObjectType() == "Player" and not unit:IsDead()
                and not unit:HasAura(SPELL_DECREPIFY) then
            candidates[#candidates + 1] = unit
        end
    end
    if #candidates > 0 then
        creature:CastSpell(candidates[math.random(#candidates)], SPELL_DECREPIFY)
    end
    schedule(guid, "decrepify", {1000, 5000}, function()
        skeletonDecrepifyTick(creature, guid)
    end)
end

-- C++ EVENT_SHADOW_FISSURE (skeleton arm :311-317): DoCast(me,
-- SPELL_SHADOW_FISSURE, true) + DoCastAOE(SPELL_BONE_ARMOR, true) ->
-- self-cast + RemoveFlag(NOT_SELECTABLE) + SetStandState(STAND) +
-- re-arm DECREPIFY {4s,6s}, in C++ order. MoveChase(victim) stays out
-- (no motion bridge); MoveIdle at fake-death likewise — the add keeps
-- its motion state through the fake-death window.
local function skeletonShadowFissureTick(creature, guid)
    creature:CastSpell(creature, SPELL_SHADOW_FISSURE, true)
    creature:CastSpell(creature, SPELL_BONE_ARMOR, true)
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    creature:SetStandState(0) -- UNIT_STAND_STATE_STAND
    schedule(guid, "decrepify", {4000, 6000}, function()
        skeletonDecrepifyTick(creature, guid)
    end)
end

-- C++ EVENT_RESURRECT (skeleton arm :304-306): EVENT_FULL_HEAL +
-- EVENT_SHADOW_FISSURE, both 1s (full heal scheduled first, C++ event
-- order).
local function skeletonResurrectTick(creature, guid)
    schedule(guid, "fullheal", 1000, function()
        creature:CastSpell(creature, SPELL_FULL_HEAL, true)
    end)
    schedule(guid, "shadowfissure", 1000, function()
        skeletonShadowFissureTick(creature, guid)
    end)
end

-- C++ npc_vrykul_skeleton::Reset (:256-260): events.Reset() +
-- EVENT_DECREPIFY init {4s,6s}.
local function skeletonReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "decrepify", {4000, 6000}, function()
        skeletonDecrepifyTick(creature, guid)
    end)
end

-- C++ npc_vrykul_skeleton::DamageTaken (:261-285): lethal damage is
-- zeroed unconditionally; the fake-death machine (NOT_SELECTABLE +
-- STAND_DEAD + events.Reset + RESURRECT {18s,22s}) fires only once
-- per fake-death — the HasFlag gate (cleared at SHADOW_FISSURE)
-- is C++-exact. The add is unkillable by damage by C++ design: it
-- fake-dies and resurrects at full health instead.
local function skeletonDamageTaken(event, creature, attacker, damage)
    local health = creature:GetHealth()
    if health > 0 and damage >= health then
        if not creature:HasFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE) then
            creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
            creature:SetStandState(7) -- UNIT_STAND_STATE_DEAD
            local guid = creature:GetGUID()
            cancelTimers(guid)
            schedule(guid, "resurrect", {18000, 22000}, function()
                skeletonResurrectTick(creature, guid)
            end)
        end
        return false, 0
    end
end

local function kelesethEnterCombat(event, creature, target)
    creature:Talk(SAY_START_COMBAT)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "shadowbolt", {2000, 3000}, function()
        shadowboltTick(creature, guid)
    end)
    schedule(guid, "frosttomb", {14000, 19000}, function()
        frostTombTick(creature, guid)
    end)
    schedule(guid, "skeletons", 6000, function()
        skeletonTick(creature, guid)
    end)
end

-- C++ JustDied (:165-169): Talk(SAY_DEATH 5) — the sjonnir
-- JustDied-Talk precedent; the _JustDied() passthrough has no bridge.
local function kelesethDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    cancelTimers(creature:GetGUID())
end

-- Shared timer wipe for events 2/23.
local function cancelCombatTimers(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_KELESETH, 1, kelesethEnterCombat)
RegisterCreatureEvent(ENTRY_KELESETH, 2, cancelCombatTimers)
RegisterCreatureEvent(ENTRY_KELESETH, 4, kelesethDied)
RegisterCreatureEvent(ENTRY_KELESETH, 23, cancelCombatTimers)
RegisterCreatureEvent(ENTRY_SKELETON, 9, skeletonDamageTaken)
RegisterCreatureEvent(ENTRY_SKELETON, 23, skeletonReset)
