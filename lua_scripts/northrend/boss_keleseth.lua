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
-- DOCUMENTED-ONLY (in this header; no registration beyond entry 23953):
-- npc_frost_tomb (NPC_FROSTTOMB 23965, :79-115) — NOT registered (no
-- genuinely bridgeable arms, the yogg_saron adds precedent):
-- IsSummonedBy DoCast(summoner, SPELL_FROST_TOMB 48400, true) (:91-97)
-- — no IsSummonedBy / summoner-carrying spawn bridge in the modeled
-- Eluna creature events; JustDied cross-creature SetData
-- (DATA_ON_THE_ROCKS false) via instance GetGuidData
-- (DATA_PRINCE_KELESETH) (:101-105) — instance-GUID leg, no bridge.
-- (The spell-summoned tomb from the FROST_TOMB arm therefore spawns
-- with no Lua AI — visual only.)
-- npc_vrykul_skeleton (NPC_SKELETON 23970, :247-320) — NOT registered:
-- the add only enters combat through the blocked boss summon leg, and
-- its whole state machine (DamageTaken lethal rewrite damage = 0 +
-- UNIT_FLAG_NOT_SELECTABLE + stand-state dead + events Reset +
-- EVENT_RESURRECT 18-22s + MotionMaster Clear/MoveIdle, :261-285)
-- rides flag / stand-state / MotionMaster legs with no bridges — a lone
-- damage-rewrite port without the machine would leave an unkillable
-- add; EVENT_DECREPIFY (:299-302: random-target DoCast(target,
-- SPELL_DECREPIFY 42702), 2-arg non-triggered, init 4-6s :259, re-arm
-- 1-5s) is individually bridgeable on the baltharus pattern but moot
-- without the summon machine; EVENT_RESURRECT (:304-306) /
-- EVENT_FULL_HEAL (:308-309: DoCast(me, SPELL_FULL_HEAL 17683, true))
-- / EVENT_SHADOW_FISSURE (:311-317: DoCast(me, SPELL_SHADOW_FISSURE
-- 50657, true) + DoCastAOE(SPELL_BONE_ARMOR 59386, true) -> self-cast
-- + flag clear + stand + MoveChase + re-arm DECREPIFY 4-6s) are all
-- rooted in the unbridgeable fake-death machine — the gothik
-- machine-gating precedent.
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
-- The adds get default melee AI (npc_vrykul_skeleton unregistered,
-- header).
local function skeletonTick(creature, guid)
    creature:Talk(SAY_SUMMON_SKELETONS)
    for _ = 1, 4 do
        creature:SummonCreature(ENTRY_SKELETON, 156.2559, 259.2093, 42.8668, 0)
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
