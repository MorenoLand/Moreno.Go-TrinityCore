-- Felblood Kael'thas (Magister's Terrace) --
-- Lua port of src/server/scripts/EasternKingdoms/MagistersTerrace/
-- boss_felblood_kaelthas.cpp
-- (boss_felblood_kaelthas — BossAI phase-one scheduler + 50%-HP
-- phase-two latch + lethal-damage outro chain). Boss-script unit per
-- eastern_kingdoms_script_loader.cpp order (Karazhan block CLOSED —
-- attumen/midnight, curator, maiden, aran, malchezaar, terestian,
-- moroes, opera, netherspite, karazhan, nightbane all ported;
-- instance_karazhan blocked on the instance-script model; Gnomeregan
-- block yielded nothing bridgeable — blastmaster_emi_shortfuse is an
-- EscortAI/gossip/instance/gameobject/summon machine, grubbis arms
-- all gate on the summoner bridge, collecting-fallout is a
-- SpellScript; AddSC_boss_felblood_kaelthas next in the Magister's
-- Terrace block; instance_magisters_terrace blocked on the
-- instance-script model).
-- Entry (verifiable from the C++ sources): magisters_terrace.h:50
-- names BOSS_KAELTHAS_SUNSTRIDER = 24664 in the MT creatures enum.
-- Script name: no explicit CreatureScript string in the file —
-- AddSC registers via RegisterMagistersTerraceCreatureAI(
-- boss_felblood_kaelthas) = RegisterCreatureAIWithFactory, which
-- stringifies the type name (ScriptMgr.h:1234), so "boss_felblood_
-- kaelthas". The creature_template ScriptName binding is DB-side (no
-- TDB in this workspace), but the entry itself is C++-verifiable so
-- the port is registered (boss_vaelastrasz precedent).
-- Whole-server-tree grep confirms the .cpp file +
-- eastern_kingdoms_script_loader.cpp (fwd-decl, call) as the only
-- sources of the class name.
-- Verifiable numbers (file's own enums/header): SPELL_FIREBALL =
-- 44189 / SPELL_PHOENIX = 44194 / SPELL_FLAME_STRIKE = 46162
-- (SelectTarget-gated cast — no bridge) / SPELL_SHOCK_BARRIER = 46165
-- (heroic-only arm — no difficulty bridge) / SPELL_PYROBLAST = 36819
-- (SelectTarget-gated — no bridge) / SPELL_EMOTE_TALK_EXCLAMATION =
-- 48348 / SPELL_EMOTE_POINT = 48349 / SPELL_EMOTE_ROAR = 48350 /
-- SPELL_QUITE_SUICIDE = 3617 (serverside) / SAY_GRAVITY_LAPSE_1 = 2 /
-- SAY_GRAVITY_LAPSE_2 = 3 (repeat lapses never recur — no phase-two
-- machinery) / SAY_POWER_FEEDBACK = 4 (phase-two — no bridge) /
-- SAY_SUMMON_PHOENIX = 5 / SAY_ANNOUNCE_PYROBLAST = 6
-- (heroic-only arm — no bridge) / SAY_FLAME_STRIKE = 7 (cast
-- SelectTarget-gated — no bridge, no timer ported) / SAY_DEATH = 8 /
-- SAY_INTRO_1 = 0 / SAY_INTRO_2 = 1 (SetData(DATA_KAELTHAS_INTRO)-
-- gated — no instance SetData bridge) / EVENT_FIREBALL init 1ms
-- repeat 2s500ms / EVENT_PHOENIX init 12s repeat 45s /
-- EVENT_FLAME_STRIKE repeat 44s / EVENT_SHOCK_BARRIER repeat 1min /
-- EVENT_QUITE_SUICIDE 11s / DATA_KAELTHAS_SUNSTRIDER (boss id).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 9 OnDamageTaken, 23 OnReset. Driven by CreateLuaEvent timers; melee
-- is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
-- Ported arms:
-- Phase-one combat machine (maiden-of-virtue convention — keyed
-- timers with EventMap cancel-on-reschedule semantics, victim casts
-- GetVictim nil-guarded (incarcerator convention), self-casts via
-- creature:CastSpell(creature, spell); timers start on
-- OnEnterCombat(1), cancelled on OnLeaveCombat(2)/OnDied(4)/
-- OnReset(23)):
-- - EVENT_FIREBALL: victim-cast 44189, 1ms init -> 2s500ms loop.
-- - EVENT_PHOENIX: Talk(SAY_SUMMON_PHOENIX) + self-cast 44194,
--   12s init -> 45s loop (the phoenix add's own AI is unbridgeable
--   — see below; the summon spell itself is real).
-- 50%-HP phase-two latch via OnDamageTaken(9) with a per-guid
-- one-shot flag reset on OnEnterCombat(1) (gyth convention —
-- C++-exact HealthBelowPctDamaged(50): GetHealth() - damage <
-- 0.5*GetMaxHealth()): Talk(SAY_GRAVITY_LAPSE_1) + cancel of the
-- phase-one timers (C++-exact observable: phase-one events stay
-- scheduled but no longer execute once the phase flips to
-- PHASE_TWO; cancelling them is the faithful net effect).
-- Lethal-damage outro chain via OnDamageTaken(9) with a per-guid
-- one-shot flag (C++-exact: damage >= GetHealth() is checked before
-- the 50% latch, and the 50% latch is skipped once the outro phase
-- is set — same ordering here): Talk(SAY_DEATH) + cancel of all
-- timers + one-shot self-cast chain 48348 @1s, 48349 @3s800ms,
-- 48350 @7s400ms, 48350 @10s, 3617 @11s (C++ EVENT_EMOTE_* /
-- EVENT_QUITE_SUICIDE, PHASE_OUTRO-gated).
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (casts are
-- packet-visual), so timers fire unconditionally and the in-loop
-- casting gate is dropped (maiden precedent). The C++ scheduler is
-- driven from JustEngagedWith and drained by UpdateAI; the port
-- schedules from OnEnterCombat(1) and cancels on 2/4/23 (maiden
-- convention). The outro's damage cap (lethal non-self damage is
-- clamped to health-1 so the boss survives at 1 HP through the
-- death sequence), AttackStop / REACT_PASSIVE / InterruptNonMelee-
-- Spells / RemoveAuras / summons.DespawnAll / DoCastAOE(CLEAR_
-- FLIGHT) machinery has no bridges — the outro chain plays as the
-- observable remnant (documented). The intro is SetData(DATA_
-- KAELTHAS_INTRO)-gated (instance arm — no bridge); the intro talks
-- are unreached (vaelastrasz gossip precedent). JustDied's
-- instance->SetBossState(DONE) has no bridge (instance precedent).
-- EnterEvadeMode's ClearFlight + summons.DespawnAll + _DespawnAtEvade
-- has no evade bridge. JustSummoned's arcane-sphere MoveFollow and
-- flame-strike-trigger dummy-cast arms have no summon/MotionMaster
-- bridges (the_beast precedent).
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - EVENT_FLAME_STRIKE: Talk(SAY_FLAME_STRIKE) + DoCast(random
--   target, 46162) gates on SelectTarget(Random, 0, 40.0f, true) —
--   no SelectTarget bridge (the_beast / vaelastrasz precedent); no
--   timer ported.
-- - EVENT_SHOCK_BARRIER + EVENT_PYROBLAST: the whole arm is heroic-
--   gated (IsHeroic) and pyroblast gates on SelectTarget(Random, 0,
--   40.0f, true) — no difficulty bridge, no SelectTarget bridge; no
--   timers ported.
-- - Phase-two machinery: EVENT_PREPARE_GRAVITY_LAPSE /
--   EVENT_GRAVITY_LAPSE_CENTER_TELEPORT (self-cast 44218) /
--   EVENT_GRAVITY_LAPSE (DoCastAOE 44224) / beam visual 44251 /
--   arcane-sphere summons 44265 / EVENT_POWER_FEEDBACK (Talk +
--   DoCastAOE 44232 + self-cast 44233 + summons.DespawnEntry) — the
--   teleport targeting, SpellHitTarget counter logic, AOE auras and
--   summon entries have no bridges (no teleport / SpellHit /
--   DoCastAOE-target / summon bridges).
-- - npc_felblood_kaelthas_phoenix (second CreatureScript class in
--   the file): IsSummonedBy's DoZoneInCombat + self-casts 44197/
--   44196, the lethal DamageTaken egg sequence (DoSummon(NPC_
--   PHOENIX_EGG 24675) + instance->GetCreature(DATA_KAELTHAS_
--   SUNSTRIDER)->AI()->JustSummoned + ember blast 44199 + damage
--   clamped to health-1), EVENT_HATCH_FROM_EGG (ObjectAccessor egg
--   despawn), EVENT_REBIRTH (self-cast 44196), EVENT_PREPARE_
--   REENGAGE (self-cast 17683 + 44197) — every arm gates on the
--   summon / instance / creature-list / zone-in-combat bridges
--   absent from the Lua surface (razorgore / go_suppression_device
--   precedent); no file written.
-- - spell_felblood_kaelthas_flame_strike AuraScript (AfterEffect-
--   Remove -> dummy 44191): AuraScript not modeled (standing
--   blocker).

local SPELL_FIREBALL = 44189
local SPELL_PHOENIX = 44194
local SPELL_EMOTE_TALK_EXCLAMATION = 48348
local SPELL_EMOTE_POINT = 48349
local SPELL_EMOTE_ROAR = 48350
local SPELL_QUITE_SUICIDE = 3617

local SAY_GRAVITY_LAPSE_1 = 2
local SAY_SUMMON_PHOENIX = 5
local SAY_DEATH = 8

local ENTRY_KAELTHAS = 24664

local timers = {}
local phaseTwo = {}
local outroFired = {}

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

local function onFireball(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FIREBALL)
    end
    schedule(guid, "fireball", 2500, function() onFireball(creature, guid) end)
end

local function onPhoenix(creature, guid)
    creature:Talk(SAY_SUMMON_PHOENIX)
    creature:CastSpell(creature, SPELL_PHOENIX)
    schedule(guid, "phoenix", 45000, function() onPhoenix(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    phaseTwo[guid] = nil
    outroFired[guid] = nil
    schedule(guid, "fireball", 1, function() onFireball(creature, guid) end)
    schedule(guid, "phoenix", 12000, function() onPhoenix(creature, guid) end)
end

local function onCombatEnd(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    phaseTwo[guid] = nil
    outroFired[guid] = nil
end

local function onDamageTaken(_, creature, _, damage)
    local guid = creature:GetGUID()
    if outroFired[guid] then
        return
    end
    local health = creature:GetHealth()
    -- C++ checks lethal damage first; the 50% latch is skipped once
    -- the outro phase is set.
    if damage >= health then
        outroFired[guid] = true
        cancelTimers(guid)
        creature:Talk(SAY_DEATH)
        schedule(guid, "outro_1", 1000, function()
            creature:CastSpell(creature, SPELL_EMOTE_TALK_EXCLAMATION)
        end)
        schedule(guid, "outro_2", 3800, function()
            creature:CastSpell(creature, SPELL_EMOTE_POINT)
        end)
        schedule(guid, "outro_3", 7400, function()
            creature:CastSpell(creature, SPELL_EMOTE_ROAR)
        end)
        schedule(guid, "outro_4", 10000, function()
            creature:CastSpell(creature, SPELL_EMOTE_ROAR)
        end)
        schedule(guid, "outro_5", 11000, function()
            creature:CastSpell(creature, SPELL_QUITE_SUICIDE)
        end)
        return
    end
    if not phaseTwo[guid]
        and health - damage < creature:GetMaxHealth() * 0.5 then
        phaseTwo[guid] = true
        cancelTimers(guid)
        creature:Talk(SAY_GRAVITY_LAPSE_1)
    end
end

RegisterCreatureEvent(ENTRY_KAELTHAS, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_KAELTHAS, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_KAELTHAS, 4, onCombatEnd)
RegisterCreatureEvent(ENTRY_KAELTHAS, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY_KAELTHAS, 23, onCombatEnd)
