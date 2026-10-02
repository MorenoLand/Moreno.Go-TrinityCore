-- Lord Jaraxxus (Trial of the Crusader) — Lua port of
-- src/server/scripts/Northrend/CrusadersColiseum/TrialOfTheCrusader/boss_lord_jaraxxus.cpp
-- (boss_jaraxxus (BossAI); npc_legion_flame / npc_infernal_volcano /
-- npc_fel_infernal / npc_nether_portal / npc_mistress_of_pain
-- (ScriptedAI); spell_mistress_kiss (AuraScript via RegisterSpellScript),
-- spell_mistress_kiss_area / spell_fel_streak_visual (SpellScript via
-- RegisterSpellScript); AddSC_boss_jaraxxus at end registers all via
-- RegisterTrialOfTheCrusaderCreatureAI (=
-- RegisterCreatureAIWithFactory(ai_name, GetTrialOfTheCrusaderAI),
-- trial_of_the_crusader.h line 288) / RegisterSpellScript). The THIRD
-- Trial of the Crusader group in northrend_script_loader.cpp order
-- (decl 53 / call 248, immediately after AddSC_boss_faction_champions(),
-- under the "// Trial of the Crusader" marker).
-- Entry: 34780 Lord Jaraxxus (trial_of_the_crusader.h NPC_JARAXXUS —
-- kalecgos pass; instance_trial_of_the_crusader.cpp binds NPC_JARAXXUS
-- -> DATA_JARAXXUS with a CircleBoundary; the
-- RegisterTrialOfTheCrusaderCreatureAI ScriptName binding is
-- instance-shimmed, the creature_template binding DB-side as usual).
-- Add entries: 34784 Legion Flame / 34813 Infernal Volcano / 34815 Fel
-- Infernal / 34825 Nether Portal / 34826 Mistress of Pain (Summons enum
-- in the .cpp — kalecgos pass).
-- Sole-source verified: whole-server-tree grep for "boss_jaraxxus",
-- "npc_legion_flame", "npc_infernal_volcano", "npc_fel_infernal",
-- "npc_nether_portal", "npc_mistress_of_pain", "spell_mistress_kiss"
-- and "spell_fel_streak_visual" hits boss_lord_jaraxxus.cpp only
-- (loader carries only the AddSC_boss_jaraxxus decl/call lines); zero
-- sql/ hits. No jaraxxus lua existed (the overnight window did not
-- outrun this one).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent;
-- melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. C++ DoCast default is triggered=false
-- (vaelastrasz convention): creature:CastSpell(nil, spell) =
-- DoCastVictim, creature:CastSpell(target, spell) = DoCast,
-- creature:CastSpell(creature, spell) = DoCastSelf (phase_hunter /
-- apothecary_hanes precedent). C++ Repeat(urand) -> math.random
-- (amanitar / boss_black_knight precedent).
-- Ported arms (C++-exact for all modeled arms):
-- boss_jaraxxus: JustEngagedWith Talk(SAY_AGGRO 1) (event 1; the
-- BossAI::JustEngagedWith instance-bookkeeping leg has no bridge —
-- tharon_ja precedent); KilledUnit Talk(SAY_KILL_PLAYER 9)
-- player-gated (event 3, who->GetTypeId()==TYPEID_PLAYER — nalorakk
-- precedent); JustDied Talk(SAY_DEATH 10) (event 4; _JustDied instance
-- bookkeeping has no bridge — tharon_ja precedent); EVENT_FEL_FIREBALL
-- — DoCastVictim(Fel Fireball 66532), 6s init, urand(11s,13s) repeat
-- (moroes precedent; Jaraxxus has no submerge/phase machine gating it
-- — UpdateAI runs combat events whenever UpdateVictim() holds);
-- EVENT_NETHER_POWER — DoCastSelf(Nether Power 66228), 22s init, 42s
-- repeat (phase_hunter self-cast precedent; C++-side detail unmodeled:
-- C++ casts triggered with SPELLVALUE_AURA_STACK RAID_MODE(5,10) —
-- no spell-mod bridge); EVENT_ENRAGE — Talk(SAY_BERSERK 11) +
-- DoCastSelf(Berserk 64238), 10min, fires once (anubarak berserk
-- precedent; never rescheduled in C++). All timers cancelled on 2/4/23
-- (gargolmar precedent).
-- npc_mistress_of_pain (34826 — Summons enum kalecgos pass):
-- JustEngagedWith EVENT_SHIVAN_SLASH — DoCastVictim(Shivan Slash
-- 66378), 4s init, urand(3s,10s) repeat (moroes precedent). No
-- boss_ai.go entry — npc entries register in the lua file only
-- (terestian precedent: Kil'rek 17229 / fiendish imp 17267;
-- npc_swarm_scarab 34605).
-- Unmodeled (no bridges — documented, not wired):
-- boss_jaraxxus intro machine — Reset: instance->GetBossState(
-- DATA_JARAXXUS) NOT_STARTED -> DoAction(ACTION_JARAXXUS_INTRO)
-- (SetReactState(REACT_PASSIVE) + events.SetPhase(PHASE_INTRO) +
-- EVENT_INTRO 1s); FAIL -> DoCastSelf(SPELL_JARAXXUS_CHAINS 67924,
-- trial_of_the_crusader.h) + SetImmuneToPC(true) +
-- SetReactState(REACT_PASSIVE) (instance model absent, standing; no
-- react / immune bridges); EVENT_INTRO: DoCastSelf(Lord Jaraxxus'
-- Hittin' Ya 66327) + MoveAlongSplineChain(POINT_SUMMONED,
-- SPLINE_INITIAL_MOVEMENT) (no motion bridge — the self-cast is
-- port-pattern-ready in isolation (phase_hunter precedent) but rides
-- the unbridged intro machine); EVENT_TAUNT_GNOME: Talk(SAY_INTRO 0),
-- 9s (rides the intro machine); EVENT_KILL_GNOME: DoCastSelf(Fel
-- Lightning Intro 67888) (rides the intro machine); MovementInform
-- SPLINE_CHAIN_POINT_SUMMONED -> SetFacingToObject(wilfred via
-- instance->GetCreature(DATA_FIZZLEBANG)) (no MovementInform hook —
-- instance model absent); EVENT_CHANGE_ORIENTATION: SetFacingTo(
-- 4.729842f) (no facing bridge); EVENT_START_COMBAT:
-- SetImmuneToPC(false) + SetReactState(REACT_AGGRESSIVE) +
-- DoZoneInCombat (no immune / react / zone-in-combat bridges);
-- DoAction(ACTION_JARAXXUS_ENGAGE): RemoveAurasDueToSpell(
-- SPELL_JARAXXUS_CHAINS) + SetImmuneToPC(false) +
-- SetReactState(REACT_AGGRESSIVE) + DoZoneInCombat (no cross-AI
-- DoAction / aura-removal / immune / react / zone bridges);
-- EnterEvadeMode: _EnterEvadeMode + instance->SetBossState(
-- DATA_JARAXXUS, FAIL) + DespawnOrUnsummon (instance model +
-- no despawn bridge — terestian precedent).
-- boss_jaraxxus EVENT_FEL_LIGHTNING — SelectTarget(Random,0,0.0f,true)
-- -> DoCast(Fel Lightning 66528), 17s init, urand(10s,30s) repeat —
-- no random-target SelectTarget bridge (cairne/kazzak precedent) —
-- documented-only.
-- boss_jaraxxus EVENT_INCINERATE_FLESH — SelectTarget(Random,1,...)
-- -> Talk(EMOTE_INCINERATE 5, target) + Talk(SAY_INCINERATE 6) +
-- DoCast(Incinerate Flesh 66237), 14s init, 23s repeat — same
-- random-target bar — documented-only.
-- boss_jaraxxus EVENT_LEGION_FLAME — SelectTarget(Random,1,...) ->
-- Talk(EMOTE_LEGION_FLAME 2, target) + DoCast(Legion Flame 66197),
-- 20s init, 30s repeat — same random-target bar — documented-only.
-- boss_jaraxxus EVENT_SUMMON_NETHER_PORTAL — Talk(EMOTE_NETHER_PORTAL
-- 3) + Talk(SAY_MISTRESS_OF_PAIN 4) + DoCast(Nether Portal 66269),
-- 20s init, 2min repeat — summon STRAND absent (no Lua-surface
-- consumer without summoning) — documented-only.
-- boss_jaraxxus EVENT_SUMMON_INFERNAL_ERUPTION — Talk(
-- EMOTE_INFERNAL_ERUPTION 7) + Talk(SAY_INFERNAL_ERUPTION 8) +
-- DoCast(Infernal Eruption 66258), 1min20s init, 2min repeat —
-- summon STRAND absent — documented-only.
-- npc_legion_flame (34784): Reset — instance->GetBossState(
-- DATA_JARAXXUS) != IN_PROGRESS -> DespawnOrUnsummon (instance model
-- + no despawn bridge); else cross-AI jaraxxus->AI()->JustSummoned
-- (no cross-AI bridge) + SetReactState(REACT_PASSIVE) (no react
-- bridge) + DoCastSelf(Legion Flame Effect 66201) — self-cast
-- bridged in isolation but instance-gated in C++: ungated emulation
-- would fire outside the encounter window (jedoga over-cast bar) —
-- documented-only; joins the entry-verifiable-but-bridge-blocked
-- queue.
-- npc_infernal_volcano (34813): Reset — SetReactState(REACT_PASSIVE)
-- (no react bridge) + DoCastSelf(Infernal Eruption Effect 66252) +
-- heroic RemoveFlag(UNIT_FLAG_NOT_SELECTABLE) (no flag bridge; heroic
-- legs have no difficulty bridge — kelidan precedent) — rides the
-- unbridged summon gate (burrower precedent) — documented-only;
-- joins the entry-verifiable-but-bridge-blocked queue.
-- npc_fel_infernal (34815): Reset — instance-gated DespawnOrUnsummon
-- + cross-AI JustSummoned (instance / cross-AI bridges absent) +
-- scheduler SelectTarget(Random,0,0.0f,true) -> DoCast(Fel Streak
-- Visual 66493) every 15s (no random-target bridge) + DoCastSelf(
-- Lord Jaraxxus' Hittin' Ya 66327) (rides the instance gate — jedoga
-- bar) — documented-only; joins the
-- entry-verifiable-but-bridge-blocked queue. (C++ note carried: Fel
-- Infernals are immune to all CC on Heroic — stuns, banish,
-- interrupt, etc.)
-- npc_nether_portal (34825): Reset — SetReactState(REACT_PASSIVE) +
-- DoCastSelf(Nether Portal Effect 66263) + heroic RemoveFlag(
-- UNIT_FLAG_NOT_SELECTABLE) — react / flag / difficulty bridges
-- absent; rides the unbridged summon gate (burrower precedent) —
-- documented-only; joins the entry-verifiable-but-bridge-blocked
-- queue.
-- npc_mistress_of_pain — zero registration beyond the Shivan Slash
-- arm: Reset — instance-gated DespawnOrUnsummon + cross-AI
-- JustSummoned + DoCastSelf(66327) + SetData(
-- DATA_MISTRESS_OF_PAIN_COUNT, INCREASE) (instance / cross-AI
-- bridges absent); JustEngagedWith EVENT_SPINNING_SPIKE:
-- SelectTarget(Random,0) -> DoCast(Spinning Spike 66283), 9s init,
-- 20s repeat — no random-target bridge; EVENT_MISTRESS_KISS:
-- DoCastSelf(Mistress' Kiss 66336), 15s init, 30s repeat — self-cast
-- bridged in isolation but IsHeroic()-gated — no difficulty bridge
-- (kelidan precedent); JustDied: SetData(
-- DATA_MISTRESS_OF_PAIN_COUNT, DECREASE) — instance model absent.
-- spell_mistress_kiss (AuraScript 66334/67905/67906/67907) —
-- HandleDummyTick: casting target -> caster casts Mistress' Kiss
-- Damage+Silence 66359 + RemoveAurasDueToSpell — no AuraScript
-- binding bridge (razelikh precedent) — documented-only.
-- spell_mistress_kiss_area (SpellScript 66336/67076/67077/67078) —
-- area-target filter (mana players) + HandleScript effect-value cast
-- — no SpellScript binding bridge (razelikh precedent) —
-- documented-only.
-- spell_fel_streak_visual (SpellScript 66493) — HandleScript
-- effect-value cast — no SpellScript binding bridge — documented-only.
-- The three spell scripts join the SpellScript/AuraScript queue.

local ENTRY_JARAXXUS = 34780
local ENTRY_MISTRESS_OF_PAIN = 34826

local SAY_AGGRO = 1
local SAY_KILL_PLAYER = 9
local SAY_DEATH = 10
local SAY_BERSERK = 11

local SPELL_FEL_FIREBALL = 66532
local SPELL_NETHER_POWER = 66228
local SPELL_BERSERK = 64238

local SPELL_SHIVAN_SLASH = 66378

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

-- C++ EVENT_FEL_FIREBALL: DoCastVictim(Fel Fireball 66532), 6s init,
-- urand(11s,13s) repeat.
local function fireballTick(creature, guid)
    creature:CastSpell(nil, SPELL_FEL_FIREBALL)
    schedule(guid, "fireball", math.random(11000, 13000), function()
        fireballTick(creature, guid)
    end)
end

-- C++ EVENT_NETHER_POWER: me->CastSpell(me, SPELL_NETHER_POWER, args)
-- (triggered, SPELLVALUE_AURA_STACK RAID_MODE(5,10) — the stack/trigger
-- args have no bridge), 22s init, 42s repeat.
local function netherPowerTick(creature, guid)
    creature:CastSpell(creature, SPELL_NETHER_POWER)
    schedule(guid, "netherpower", 42000, function()
        netherPowerTick(creature, guid)
    end)
end

local function jaraxxusEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "fireball", 6000, function()
        fireballTick(creature, guid)
    end)
    schedule(guid, "netherpower", 22000, function()
        netherPowerTick(creature, guid)
    end)
    -- C++ EVENT_ENRAGE: Talk(SAY_BERSERK) + DoCastSelf(Berserk 64238),
    -- 10min, fires once (never rescheduled).
    schedule(guid, "berserk", 600000, function()
        creature:Talk(SAY_BERSERK)
        creature:CastSpell(creature, SPELL_BERSERK)
    end)
end

local function jaraxxusLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function jaraxxusTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL_PLAYER)
    end
end

local function jaraxxusDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(SAY_DEATH)
end

local function jaraxxusReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_JARAXXUS, 1, jaraxxusEnterCombat)
RegisterCreatureEvent(ENTRY_JARAXXUS, 2, jaraxxusLeaveCombat)
RegisterCreatureEvent(ENTRY_JARAXXUS, 3, jaraxxusTargetDied)
RegisterCreatureEvent(ENTRY_JARAXXUS, 4, jaraxxusDied)
RegisterCreatureEvent(ENTRY_JARAXXUS, 23, jaraxxusReset)

-- C++ npc_mistress_of_pain JustEngagedWith EVENT_SHIVAN_SLASH:
-- DoCastVictim(Shivan Slash 66378), 4s init, urand(3s,10s) repeat.
local function shivanSlashTick(creature, guid)
    creature:CastSpell(nil, SPELL_SHIVAN_SLASH)
    schedule(guid, "shivanslash", math.random(3000, 10000), function()
        shivanSlashTick(creature, guid)
    end)
end

local function mistressEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "shivanslash", 4000, function()
        shivanSlashTick(creature, guid)
    end)
end

local function mistressLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function mistressDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function mistressReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_MISTRESS_OF_PAIN, 1, mistressEnterCombat)
RegisterCreatureEvent(ENTRY_MISTRESS_OF_PAIN, 2, mistressLeaveCombat)
RegisterCreatureEvent(ENTRY_MISTRESS_OF_PAIN, 4, mistressDied)
RegisterCreatureEvent(ENTRY_MISTRESS_OF_PAIN, 23, mistressReset)
