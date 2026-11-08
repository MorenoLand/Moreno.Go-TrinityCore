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
-- precedent); JustDied Talk(SAY_DEATH 10) (event 4) + _JustDied
-- SetBossState(DATA_JARAXXUS 1, DONE 3) PORTED tick 405
-- (creature:SetBossState bridge live; the _JustDied "no bridge" claim
-- is STALE — curator/archavon precedent); events.Reset() /
-- scheduler.CancelAll() ride cancelTimers; summons.DespawnAll() stays
-- out (no summon-registry bridge); EVENT_FEL_FIREBALL
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
-- CASTING gates: boss_jaraxxus own UpdateAI pre-loop
-- (boss_lord_jaraxxus.cpp:200) + in-loop (:282) gates and
-- npc_mistress_of_pain own pre-loop (:433) + in-loop (:457) gates
-- PORTED as a 500ms IsCasting defer per ported arm (mal_ganis idiom)
-- — defer precedes target pick / Talk / cast, no repeat re-arm
-- consumed; the berserk one-shot defer re-queues a fresh 500ms entry.
-- Talk / KilledUnit / JustDied arms ungated per C++ (separate hooks).
-- npc_mistress_of_pain (34826 — Summons enum kalecgos pass):
-- JustEngagedWith EVENT_SHIVAN_SLASH — DoCastVictim(Shivan Slash
-- 66378), 4s init, urand(3s,10s) repeat (moroes precedent). No
-- boss_ai.go entry — npc entries register in the lua file only
-- (terestian precedent: Kil'rek 17229 / fiendish imp 17267;
-- npc_swarm_scarab 34605).
-- Unmodeled (no bridges — documented, not wired):
-- boss_jaraxxus intro machine — PORTED tick 406 (the "no
-- MovementInform hook" / "instance model absent" bars are STALE):
-- Reset (:116-130): GetBossState==NOT_STARTED -> DoAction(INTRO)
-- (SetReactState PASSIVE + EVENT_INTRO 1s; events.SetPhase(PHASE_INTRO)
-- has no bridge — the intro timers are the phase); FAIL ->
-- DoCastSelf(SPELL_JARAXXUS_CHAINS 67924) + SetReactState PASSIVE
-- (SetImmuneToPC(true) stays out — no bridge, verified: no such method
-- in engine/); EVENT_INTRO (:253-255): DoCastSelf(Hittin' Ya 66327,
-- triggered) + MoveAlongSplineChain(POINT_SUMMONED 1,
-- SPLINE_INITIAL_MOVEMENT 1, walk=true); MovementInform (:171-178):
-- SPLINE_CHAIN_MOTION_TYPE 17 + POINT_SUMMONED -> SetFacingToObject(
-- wilfred via GetInstanceCreature(DATA_FIZZLEBANG 16)) +
-- EVENT_TAUNT_GNOME 9s (nil wilfred schedules nothing, C++ if-guard);
-- EVENT_TAUNT_GNOME (:257-259): Talk(SAY_INTRO 0) + KILL_GNOME 9s;
-- EVENT_KILL_GNOME (:261-263): DoCastSelf(Fel Lightning Intro 67888) +
-- CHANGE_ORIENTATION 3s; EVENT_CHANGE_ORIENTATION (:265-267):
-- SetFacingTo(4.729842) + START_COMBAT 7s; EVENT_START_COMBAT
-- (:269-272): SetReactState(REACT_AGGRESSIVE) + DoZoneInCombat
-- (SetImmuneToPC(false) stays out — no bridge). Intro timers ride the
-- per-GUID timers table, cancelled on 2/4/23 like the combat arms;
-- DoAction(INTRO 15) is also receivable via ev38.
-- DoAction(ACTION_JARAXXUS_ENGAGE 21, :162-168) — PORTED tick 406 via
-- ev38: RemoveAura(SPELL_JARAXXUS_CHAINS 67924) + SetReactState(
-- REACT_AGGRESSIVE) + DoZoneInCombat (SetImmuneToPC(false) stays out —
-- no bridge). The old "no cross-AI DoAction / aura-removal / immune /
-- react / zone bridges" claim is STALE — DoAction fires ev38,
-- RemoveAura / SetReactState / DoZoneInCombat are all bridged. This is
-- the Barrett Jaraxxus-fail branch receiver.
-- EnterEvadeMode (:147-151) — PORTED tick 406 via ev2 (the evade hook;
-- death never reaches ev2, tick-406 beasts precedent): cancelTimers +
-- SetBossState(DATA_JARAXXUS, FAIL) + fordring DoAction(
-- ACTION_JARAXXUS_WIPE 24) + heroic TrialCounter decrement + shared
-- mistressOfPainCount = 0 (instance :182-187) + DespawnOrUnsummon last.
-- The FAIL relay/counter fire once per wipe (guarded on state ~= FAIL
-- and ~= DONE). The old "instance model is the true bar" claim is
-- STALE — GetBossState / SetBossState / GetInstanceData are bridged.
-- boss_jaraxxus EVENT_FEL_LIGHTNING — PORTED: SelectTarget(
-- Random,0,0.0f,true) (any-range -> 100, curator precedent) ->
-- DoCast(Fel Lightning 66528) non-triggered, 17s init, urand(10s,30s)
-- repeat, via the maiden/beast random-player convention. The old "no
-- random-target SelectTarget bridge" claim (cairne/kazzak precedent)
-- is retracted.
-- boss_jaraxxus EVENT_INCINERATE_FLESH — PORTED: SelectTarget(
-- Random,1,0.0f,true) (index-1 = exclude-victim idiom, approximated
-- with a random pick — akilzon precedent) -> Talk(EMOTE_INCINERATE 5,
-- target) + Talk(SAY_INCINERATE 6) + DoCast(Incinerate Flesh 66237)
-- non-triggered, 14s init, 23s repeat. The "same random-target bar"
-- claim is retracted.
-- boss_jaraxxus EVENT_LEGION_FLAME — PORTED: SelectTarget(
-- Random,1,0.0f,true) (index-1 exclude-victim, akilzon precedent) ->
-- Talk(EMOTE_LEGION_FLAME 2, target) + DoCast(Legion Flame 66197)
-- non-triggered, 20s init, 30s repeat. The "same random-target bar"
-- claim is retracted. The summon strand (66197 -> 34784 at the target)
-- is PORTED tick 411 U1 (explicit SummonCreature in legionFlameTick —
-- see below).
-- boss_jaraxxus EVENT_SUMMON_NETHER_PORTAL — PORTED
-- (netherPortalTick; the summon strand is PORTED tick 410 U2 — see
-- below). EVENT_SUMMON_INFERNAL_ERUPTION — PORTED (infernalTick;
-- the volcano summon strand is PORTED tick 410 U2 — see below).
-- Tick-410 U2 DBC finding: the spell-effect summon path is dead in this
-- deployment — no Spell.dbc ships anywhere in the repo (no data/dbc dir
-- at all; `find` over ~/workspace returns zero *.dbc), so s.Data.Spell
-- (store.go:981, via File("Spell"), :423-443) fails, spellKnown stays
-- false (boss_ai.go:1706-1711), and creatureEffectSummonType
-- (boss_ai.go:4335) never runs for 66269/66258 — the 2min timer casts
-- summon nothing. The engine path itself is live and correct when data
-- exists (summonTempCreature funnel fires ev19 on the caster +
-- fireCreatureSpawned ev23 Reset on the summoned, so the tick-409
-- volcano/portal Reset arms would arm), but with no spell data it never
-- executes. The explicit Lua delivery legs below are therefore the only
-- delivery: netherPortalTick summons 34825 + infernalTick summons 34813
-- at the caster's exact position (the engine spell path's own no-dest
-- fallback, boss_ai.go:4356-4361; GetX/GetY/GetZ/GetO resolve via the
-- generic object fallback, engine/scripting/object.go:177), default
-- type 7 MANUAL_DESPAWN (the spell-driven lifetime rides the DBC
-- duration, unreadable here); the second-order legs ride the same
-- finding — infernalVolcanoReset summons fel infernal 34815 (66252's
-- effect, 1:1 per eruption) and netherPortalReset summons mistress
-- 34826 (66263's effect, 1:1 per portal), each at its caster's position.
-- The 1:1 counts and the 34815/34826 entries ride the DBC effect rows
-- (MiscValue), unverifiable in-tree — no Spell.dbc, no sql/ spell data.
-- Caveat: if Spell.dbc ever ships, the 66269/66258/66252/66263 casts
-- would ALSO summon, double-spawning — these legs must be revisited then.
-- The legion-flame strand (66197 -> 34784 at a player target) is PORTED
-- tick 411 U1 (explicit SummonCreature in legionFlameTick; same
-- double-spawn caveat applies to 66197).
-- npc_legion_flame (34784): Reset — PORTED tick 408
-- (legionFlameReset): instance->GetBossState(DATA_JARAXXUS) !=
-- IN_PROGRESS -> DespawnOrUnsummon (GetBossState IS bridged,
-- lua_creature_events.go:6174; DespawnOrUnsummon :6373 — the old
-- "instance gate is the true bar" claim is STALE); else ctor
-- SetCombatMovement(false) (:294; lua_creature_events.go:4202,
-- ayamiss Reset-arm precedent) + SetReactState(REACT_PASSIVE)
-- (:308; :4021) + DoCastSelf(Legion Flame Effect 66201, triggered)
-- (:309). The cross-AI jaraxxus->AI()->JustSummoned leg (:305-306)
-- re-examined tick 410 U2: BossAI::JustSummoned
-- (ScriptedCreature.cpp:559-564) is summons.Summon (no bridge — the
-- summon-registry leg, same bar as summons.DespawnAll) + DoZoneInCombat
-- (summon) when the boss is engaged. The zone leg is bridged
-- (lua_creature_events.go:1944) but Map.dbc-gated (no Map.dbc ships),
-- and for the passive flame (no UpdateAI, no attacks) it is
-- unobservable in any case — skipped deliberately; the fel-infernal /
-- mistress legs ARE ported (see below) because those summons fight.
-- npc_infernal_volcano (34813): Reset — PORTED tick 409
-- (infernalVolcanoReset): ctor SetCombatMovement(false) (:321;
-- no Lua ctor hook — Reset is the spawn hook, ayamiss precedent)
-- + SetReactState(REACT_PASSIVE) (:324; SetReactState IS bridged,
-- frozen_core precedent) + DoCastSelf(Infernal Eruption Effect
-- 66252, triggered) (:326) + heroic RemoveFlag(
-- UNIT_FLAG_NOT_SELECTABLE) (:327-328; flag bridge exists,
-- lua_creature_events.go:5637; IsHeroic() is bridged — the
-- "no difficulty bridge" leg of the kelidan precedent is STALE).
-- The old "rides the unbridged summon gate (burrower precedent)"
-- bar is retracted: the Reset arm itself uses only bridged
-- methods, like the tick-408 legion-flame / fel-infernal /
-- mistress Resets; the volcano summon strand is PORTED tick 410 U2
-- (infernalTick summons 34813 explicitly — see above).
-- npc_fel_infernal (34815): Reset — PORTED tick 408
-- (felInfernalReset / felStreakTick): instance->GetBossState(
-- DATA_JARAXXUS) != IN_PROGRESS -> DespawnOrUnsummon (GetBossState
-- IS bridged, lua_creature_events.go:6174 — the old "instance /
-- cross-AI bridges absent" claim is STALE for the gate); else
-- scheduler SelectTarget(Random,0,0.0f,true) -> DoCast(Fel Streak
-- Visual 66493) non-triggered, 2s init, 15s repeat (:352-357; the
-- scheduler validator !UNIT_STATE_CASTING (:347-350) rides the 500ms
-- IsCasting defer, mal_ganis idiom; repeat unconditional, outside
-- the target check; UpdateAI's UpdateVictim gate (:364) is implicit
-- — no threat targets means no cast) + DoCastSelf(Lord Jaraxxus'
-- Hittin' Ya 66327, triggered) (:359). The cross-AI
-- jaraxxus->AI()->JustSummoned leg (:344-345) re-examined tick 410 U2:
-- leg (a) summons.Summon stays out (no bridge); leg (b) DoZoneInCombat
-- (boss engaged) is PORTED in felInfernalReset below (IN_PROGRESS gate
-- is the engaged check). (C++ note carried: Fel Infernals are immune
-- to all CC on Heroic — stuns, banish, interrupt, etc.)
-- npc_nether_portal (34825): Reset — PORTED tick 409
-- (netherPortalReset): ctor SetCombatMovement(false) (:382;
-- no Lua ctor hook — Reset is the spawn hook, ayamiss precedent)
-- + SetReactState(REACT_PASSIVE) (:386; SetReactState IS bridged,
-- frozen_core precedent) + DoCastSelf(Nether Portal Effect 66263,
-- triggered) (:388) + heroic RemoveFlag(UNIT_FLAG_NOT_SELECTABLE)
-- (:389-390; flag bridge exists, lua_creature_events.go:5637;
-- IsHeroic() is bridged — the "no difficulty bridge" claim is
-- STALE). The old "rides the unbridged summon gate (burrower
-- precedent)" bar is retracted: the Reset arm itself uses only
-- bridged methods, like the tick-408 legion-flame / fel-infernal
-- / mistress Resets; the portal summon strand is PORTED tick 410 U2
-- (netherPortalTick summons 34825 explicitly — see above).
-- npc_mistress_of_pain — JustEngagedWith EVENT_SPINNING_SPIKE PORTED:
-- SelectTarget(Random,0,0.0f,true) -> DoCast(Spinning Spike 66283)
-- non-triggered, 9s init, 20s repeat (not heroic-gated — scheduled
-- unconditionally; the "no random-target bridge" claim is retracted);
-- Shivan Slash arm stays ported; Reset — PORTED tick 408
-- (mistressReset): instance-gated DespawnOrUnsummon (GetBossState IS
-- bridged, lua_creature_events.go:6174) + DoCastSelf(Hittin' Ya 66327,
-- triggered) + SetData(DATA_MISTRESS_OF_PAIN_COUNT 302, INCREASE 501)
-- via the shared instanceData table (same mirror as JustDied below);
-- the cross-AI JustSummoned leg (a) stays out (no summon-registry
-- bridge); leg (b) DoZoneInCombat is PORTED tick 410 U2 in
-- mistressReset below (IN_PROGRESS gate is the engaged check);
-- EVENT_MISTRESS_KISS:
-- DoCastSelf(Mistress' Kiss 66336) non-triggered, 15s init,
-- 30s repeat — PORTED (IsHeroic-gated at engage; the old "no difficulty
-- bridge (kelidan precedent)" claim is STALE — lavanthor heroic-flames
-- port); JustDied: SetData(DATA_MISTRESS_OF_PAIN_COUNT 302, DECREASE
-- 502) PORTED tick 405 via the shared instanceData table
-- (data.mistressOfPainCount, faction-champions precedent; the
-- "instance model absent" claim is STALE for the count leg —
-- creature:SetData is per-creature ScriptData, not the instance, so it
-- is NOT used); floored at 0 (paired INCREASE ported tick 408 in
-- mistressReset); the THREE_SIXTY_PAIN_SPIKE achievement read
-- (MistressOfPainCount >= 2) is PORTED tick 411 U3 via the Go
-- DATA_TYPE_INSTANCE_SCRIPT gate (checkTrialOfTheCrusaderCriteria
-- reads data.mistressOfPainCount) — the old "no Go bridge" claim is
-- STALE.
-- spell_mistress_kiss (AuraScript 66334/67905/67906/67907) —
-- HandleDummyTick: casting target -> caster casts Mistress' Kiss
-- Damage+Silence 66359 + RemoveAurasDueToSpell — PORTED tick 371
-- via RegisterAuraEvent (AuraEventOnPeriodic = 1), mistressKissAuraTick
-- below (aura:GetCaster / target:IsCasting / target:RemoveAura bridges;
-- the old "no AuraScript binding bridge (razelikh precedent)" claim is
-- STALE — the periodic-dummy hook fires for player auras, shadow-prison
-- precedent).
-- spell_mistress_kiss_area (SpellScript 66336/67076/67077/67078) —
-- area-target filter (mana players) + HandleScript effect-value cast
-- — PORTED Go-side (creatureMistressKissArea): the cast of 66336 above
-- now lands the kiss aura on one random non-mana player. The kiss
-- aura's own periodic tick (spell_mistress_kiss 66334, 66359 on a
-- casting target) is PORTED tick 371 via RegisterAuraEvent below.
-- spell_fel_streak_visual (SpellScript 66493) — HandleScript
-- effect-value cast — no SpellScript binding bridge — documented-only.
-- The three spell scripts join the SpellScript/AuraScript queue.

local ENTRY_JARAXXUS = 34780
local ENTRY_LEGION_FLAME = 34784 -- Summons enum
local ENTRY_INFERNAL_VOLCANO = 34813 -- Summons enum
local ENTRY_FEL_INFERNAL = 34815 -- Summons enum
local ENTRY_NETHER_PORTAL = 34825 -- Summons enum
local ENTRY_MISTRESS_OF_PAIN = 34826
local DATA_JARAXXUS = 1 -- trial_of_the_crusader.h:33
local DATA_ANUBARAK = 5 -- trial_of_the_crusader.h:37
local DATA_FORDRING = 12 -- trial_of_the_crusader.h:46
local DATA_FIZZLEBANG = 16 -- trial_of_the_crusader.h:50
local ENTRY_FORDRING = 34996 -- trial_of_the_crusader.h NPC_TIRION_FORDRING
local BOSS_STATE_NOT_STARTED = 0 -- EncounterState NOT_STARTED (InstanceScript.h:68-76)
local BOSS_STATE_IN_PROGRESS = 1 -- EncounterState IN_PROGRESS (InstanceScript.h:68-76)
local BOSS_STATE_FAIL = 2 -- EncounterState FAIL (InstanceScript.h:68-76)
local BOSS_STATE_DONE = 3 -- EncounterState DONE (InstanceScript.h:68-76)

local ACTION_JARAXXUS_DEFEATED = 9 -- trial_of_the_crusader.h TCRActions (1-based from ACTION_START_GORMOK = 1)
local ACTION_JARAXXUS_INTRO = 15
local ACTION_JARAXXUS_ENGAGE = 21
local ACTION_JARAXXUS_WIPE = 24

local REACT_PASSIVE = 0 -- UnitDefines.h
local REACT_AGGRESSIVE = 2

local UNIT_FIELD_FLAGS = 59 -- UpdateFields.h OBJECT_END + 0x35
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000 -- UnitDefines.h

local SPLINE_CHAIN_MOTION_TYPE = 17
local POINT_SUMMONED = 1 -- boss_lord_jaraxxus.cpp Misc enum
local SPLINE_INITIAL_MOVEMENT = 1

local SAY_INTRO = 0
local SAY_AGGRO = 1
local SAY_KILL_PLAYER = 9
local SAY_DEATH = 10
local SAY_BERSERK = 11

local SPELL_FEL_FIREBALL = 66532
local SPELL_NETHER_POWER = 66228
local SPELL_BERSERK = 64238
local SPELL_JARAXXUS_CHAINS = 67924 -- trial_of_the_crusader.h TCRSpellIds
local SPELL_HITTIN_YA = 66327 -- SPELL_LORD_JARAXXUS_HITTIN_YA
local SPELL_FEL_LIGHTNING_INTRO = 67888
local SPELL_FEL_LIGHTNING = 66528
local SPELL_INCINERATE_FLESH = 66237
local SPELL_LEGION_FLAME = 66197
local SPELL_LEGION_FLAME_EFFECT = 66201 -- BossSpells enum
local SPELL_INFERNAL_ERUPTION_EFFECT = 66252 -- BossSpells enum
local SPELL_NETHER_PORTAL_EFFECT = 66263 -- BossSpells enum
local SPELL_FEL_STREAK_VISUAL = 66493 -- BossSpells enum, Felflame Infernal
local SPELL_SPINNING_SPIKE = 66283
local SPELL_MISTRESS_KISS = 66336
local SPELL_NETHER_PORTAL = 66269
local SPELL_INFERNAL_ERUPTION = 66258

local EMOTE_INCINERATE = 5
local SAY_INCINERATE = 6
local EMOTE_LEGION_FLAME = 2
local EMOTE_NETHER_PORTAL = 3
local SAY_MISTRESS_OF_PAIN = 4
local EMOTE_INFERNAL_ERUPTION = 7
local SAY_INFERNAL_ERUPTION = 8

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

-- Fordring relay: the ToC instance script's SetBossState hook fires
-- fordring->AI()->DoAction(action) (instance_trial_of_the_crusader.cpp:
-- :185-186 FAIL, :191-192 DONE); there is no Go ToC instance script, so
-- the relay is Lua-side. GetInstanceCreature(DATA_FORDRING) is the
-- latched path; the FindNearestCreature grid fallback covers an
-- unlatched DATA_FORDRING. DoAction fires ev38 on the target.
local function sendToFordring(creature, action)
    local fordring = creature:GetInstanceCreature(DATA_FORDRING)
    if fordring == nil then fordring = creature:FindNearestCreature(ENTRY_FORDRING, 200) end
    if fordring ~= nil then fordring:DoAction(action) end
end

-- C++ SelectTarget(Random, idx, 0.0f, true) playerOnly=true arms ride the
-- handler-surface GetAITargets twin (lua_creature_events.go): threat
-- list, playerOnly + alive, 0.0f = any range (no distance filter) —
-- C++-exact since 2026-10-18, retargeted off the retired
-- GetPlayersInWorld 100yd approx.
local function randomThreatPlayer(creature)
    local candidates = {}
    for _, unit in ipairs(creature:GetAITargets()) do
        if unit:GetObjectType() == "Player" and not unit:IsDead() then
            candidates[#candidates + 1] = unit
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- SelectTarget(Random,1,...) twin (:217/:234): UnitAI.h pops the
-- current victim (non-distance list is victim-first), not the
-- threat-sorted head: exclude by victim GUID, then the player/alive
-- filter over the rest.
local function randomThreatPlayerSkipFirst(creature)
    local victim = creature:GetVictim()
    local victimGuid = nil
    if victim ~= nil then
        victimGuid = victim:GetGUID()
    end
    local targets = creature:GetAITargets()
    local candidates = {}
    local aliveSeen = 0
    for _, unit in ipairs(targets) do
        local skip = false
        if victimGuid ~= nil then
            skip = unit:GetGUID() == victimGuid
        elseif not unit:IsDead() then
            aliveSeen = aliveSeen + 1
            skip = aliveSeen == 1
        end
        if not skip
                and unit:GetObjectType() == "Player" and not unit:IsDead() then
            candidates[#candidates + 1] = unit
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EVENT_FEL_FIREBALL: DoCastVictim(Fel Fireball 66532), 6s init,
-- urand(11s,13s) repeat.
local function fireballTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "fireball", 500, function() fireballTick(creature, guid) end)
        return
    end
    creature:CastSpell(nil, SPELL_FEL_FIREBALL)
    schedule(guid, "fireball", math.random(11000, 13000), function()
        fireballTick(creature, guid)
    end)
end

-- C++ EVENT_NETHER_POWER: me->CastSpell(me, SPELL_NETHER_POWER, args)
-- (TRIGGERED_FULL_MASK + SPELLVALUE_AURA_STACK RAID_MODE(5,10) — the
-- stack mod has no bridge; the triggered flag is bridged), 22s init,
-- 42s repeat.
local function netherPowerTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "netherpower", 500, function() netherPowerTick(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_NETHER_POWER, true)
    schedule(guid, "netherpower", 42000, function()
        netherPowerTick(creature, guid)
    end)
end

-- C++ EVENT_FEL_LIGHTNING: SelectTarget(Random,0,0.0f,true) ->
-- DoCast(Fel Lightning 66528) non-triggered, 17s init,
-- Repeat(10s,30s) outside the target check (re-arm unconditional).
local function felLightningTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "fellightning", 500, function() felLightningTick(creature, guid) end)
        return
    end
    local target = randomThreatPlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_FEL_LIGHTNING)
    end
    schedule(guid, "fellightning", math.random(10000, 30000), function()
        felLightningTick(creature, guid)
    end)
end

-- C++ EVENT_INCINERATE_FLESH: SelectTarget(Random,1,0.0f,true) ->
-- Talk(EMOTE_INCINERATE 5, target) + Talk(SAY_INCINERATE 6) +
-- DoCast(Incinerate Flesh 66237) non-triggered, 14s init,
-- Repeat(23s) outside the target check (re-arm unconditional).
local function incinerateFleshTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "incinerateflesh", 500, function() incinerateFleshTick(creature, guid) end)
        return
    end
    local target = randomThreatPlayerSkipFirst(creature)
    if target then
        creature:Talk(EMOTE_INCINERATE, target)
        creature:Talk(SAY_INCINERATE)
        creature:CastSpell(target, SPELL_INCINERATE_FLESH)
    end
    schedule(guid, "incinerateflesh", 23000, function()
        incinerateFleshTick(creature, guid)
    end)
end

-- C++ EVENT_LEGION_FLAME: SelectTarget(Random,1,0.0f,true) ->
-- Talk(EMOTE_LEGION_FLAME 2, target) + DoCast(Legion Flame 66197)
-- non-triggered, 20s init, Repeat(30s) outside the target check
-- (re-arm unconditional).
local function legionFlameTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "legionflame", 500, function() legionFlameTick(creature, guid) end)
        return
    end
    local target = randomThreatPlayerSkipFirst(creature)
    if target then
        creature:Talk(EMOTE_LEGION_FLAME, target)
        creature:CastSpell(target, SPELL_LEGION_FLAME)
        -- Spell-summon delivery — PORTED tick 411 U1 (same dead-DBC
        -- class as tick 410 U2): 66197's effect summons 34784 at the
        -- target, but no Spell.dbc ships so the engine path never runs;
        -- summon explicitly at the target's position (boss_ignis
        -- victim:GetX precedent). The flame's ev23 Reset arms on spawn
        -- (summonTempCreature funnel fires ev19 + ev23); if Spell.dbc
        -- ever ships, the 66197 cast would ALSO summon — revisit then.
        creature:SummonCreature(ENTRY_LEGION_FLAME, target:GetX(), target:GetY(), target:GetZ(), target:GetO())
    end
    schedule(guid, "legionflame", 30000, function()
        legionFlameTick(creature, guid)
    end)
end

-- C++ EVENT_SUMMON_NETHER_PORTAL (boss_lord_jaraxxus.cpp:241-245):
-- Talk(EMOTE_NETHER_PORTAL) + Talk(SAY_MISTRESS_OF_PAIN) + DoCast(
-- SPELL_NETHER_PORTAL 66269), 20s init (:142), unconditional
-- events.Repeat(2min) (:245); the summon strand is PORTED tick 410 U2
-- (explicit SummonCreature below — see header).
local function netherPortalTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "netherportal", 500, function() netherPortalTick(creature, guid) end)
        return
    end
    creature:Talk(EMOTE_NETHER_PORTAL)
    creature:Talk(SAY_MISTRESS_OF_PAIN)
    creature:CastSpell(creature, SPELL_NETHER_PORTAL)
    -- Spell-summon delivery — PORTED tick 410 U2 (see header): the DBC
    -- path is dead (no Spell.dbc ships), so the portal is summoned
    -- explicitly at the caster's exact position (the engine spell
    -- path's own no-dest fallback, boss_ai.go:4356-4361).
    creature:SummonCreature(ENTRY_NETHER_PORTAL, creature:GetX(), creature:GetY(), creature:GetZ(), creature:GetO())
    schedule(guid, "netherportal", 120000, function()
        netherPortalTick(creature, guid)
    end)
end

-- C++ EVENT_SUMMON_INFERNAL_ERUPTION (:247-251): Talk(
-- EMOTE_INFERNAL_ERUPTION) + Talk(SAY_INFERNAL_ERUPTION) + DoCast(
-- SPELL_INFERNAL_ERUPTION 66258), 1min20s init (:143), unconditional
-- events.Repeat(2min) (:251); the volcano summon strand is PORTED tick
-- 410 U2 (explicit SummonCreature below — see header).
local function infernalTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "infernal", 500, function() infernalTick(creature, guid) end)
        return
    end
    creature:Talk(EMOTE_INFERNAL_ERUPTION)
    creature:Talk(SAY_INFERNAL_ERUPTION)
    creature:CastSpell(creature, SPELL_INFERNAL_ERUPTION)
    -- Spell-summon delivery — PORTED tick 410 U2 (see header): the DBC
    -- path is dead (no Spell.dbc ships), so the volcano is summoned
    -- explicitly at the caster's exact position (the engine spell
    -- path's own no-dest fallback, boss_ai.go:4356-4361).
    creature:SummonCreature(ENTRY_INFERNAL_VOLCANO, creature:GetX(), creature:GetY(), creature:GetZ(), creature:GetO())
    schedule(guid, "infernal", 120000, function()
        infernalTick(creature, guid)
    end)
end

-- C++ EVENT_ENRAGE: own UpdateAI pre-loop (:200) + in-loop (:282)
-- CASTING gates: 500ms IsCasting defer, no repeat re-arm (mal_ganis
-- idiom) — the one-shot defer re-queues a fresh 500ms entry.
local function berserkTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "berserk", 500, function()
            berserkTick(creature, guid)
        end)
        return
    end
    creature:Talk(SAY_BERSERK)
    creature:CastSpell(creature, SPELL_BERSERK, true)
end

-- Intro machine (boss_lord_jaraxxus.cpp UpdateAI intro events,
-- :253-272). Forward declarations with plain assignment (never re-
-- `local function` after a forward declare — malown shadowing bug):
-- the taunt/kill/orientation ticks reference the next leg in the chain.
local introTick, tauntTick, killGnomeTick, orientationTick, startCombatTick

-- C++ EVENT_INTRO (:253-255): DoCastSelf(Hittin' Ya 66327, triggered) +
-- MoveAlongSplineChain(POINT_SUMMONED 1, SPLINE_INITIAL_MOVEMENT 1,
-- walk=true). The 500ms IsCasting defer is the UpdateAI pre-loop /
-- in-loop casting gate (:200, :282) per ported arm (mal_ganis idiom).
introTick = function(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "intro", 500, function() introTick(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_HITTIN_YA, true)
    creature:MoveAlongSplineChain(POINT_SUMMONED, SPLINE_INITIAL_MOVEMENT, true)
end

-- C++ EVENT_TAUNT_GNOME (:257-259): Talk(SAY_INTRO 0) + KILL_GNOME 9s.
tauntTick = function(creature, guid)
    creature:Talk(SAY_INTRO)
    schedule(guid, "killgnome", 9000, function() killGnomeTick(creature, guid) end)
end

-- C++ EVENT_KILL_GNOME (:261-263): DoCastSelf(Fel Lightning Intro
-- 67888) + CHANGE_ORIENTATION 3s.
killGnomeTick = function(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "killgnome", 500, function() killGnomeTick(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_FEL_LIGHTNING_INTRO)
    schedule(guid, "orientation", 3000, function() orientationTick(creature, guid) end)
end

-- C++ EVENT_CHANGE_ORIENTATION (:265-267): SetFacingTo(4.729842) +
-- START_COMBAT 7s.
orientationTick = function(creature, guid)
    creature:SetFacingTo(4.729842)
    schedule(guid, "startcombat", 7000, function() startCombatTick(creature, guid) end)
end

-- C++ EVENT_START_COMBAT (:269-272): SetImmuneToPC(false) [no bridge,
-- stays out] + SetReactState(REACT_AGGRESSIVE) + DoZoneInCombat.
startCombatTick = function(creature, guid)
    creature:SetReactState(REACT_AGGRESSIVE)
    creature:DoZoneInCombat()
end

-- C++ DoAction(ACTION_JARAXXUS_INTRO 15, :155-161): SetReactState(
-- REACT_PASSIVE) + events.SetPhase(PHASE_INTRO) (no phase bridge — the
-- intro timers above are the phase) + EVENT_INTRO 1s. Called from
-- jaraxxusReset (C++ Reset calls DoAction(INTRO)) and from the ev38
-- handler below.
local function startIntro(creature, guid)
    creature:SetReactState(REACT_PASSIVE)
    schedule(guid, "intro", 1000, function() introTick(creature, guid) end)
end

local function jaraxxusEnterCombat(event, creature, target)
    creature:SetCombatPulseDelay(5) -- boss_lord_jaraxxus.cpp:135 (BossAI base arm; setActive/instance legs have no bridge)
    creature:DoZoneInCombat() -- boss_lord_jaraxxus.cpp:135 (BossAI base arm)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "fireball", 6000, function()
        fireballTick(creature, guid)
    end)
    schedule(guid, "netherpower", 22000, function()
        netherPowerTick(creature, guid)
    end)
    schedule(guid, "fellightning", 17000, function()
        felLightningTick(creature, guid)
    end)
    schedule(guid, "incinerateflesh", 14000, function()
        incinerateFleshTick(creature, guid)
    end)
    schedule(guid, "legionflame", 20000, function()
        legionFlameTick(creature, guid)
    end)
    schedule(guid, "netherportal", 20000, function()
        netherPortalTick(creature, guid)
    end)
    schedule(guid, "infernal", 80000, function()
        infernalTick(creature, guid)
    end)
    -- C++ EVENT_ENRAGE: Talk(SAY_BERSERK) + DoCastSelf(Berserk 64238, triggered),
    -- 10min, fires once (never rescheduled).
    schedule(guid, "berserk", 600000, function()
        berserkTick(creature, guid)
    end)
end

local function jaraxxusLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    -- C++ boss_jaraxxus::EnterEvadeMode (:147-151): _EnterEvadeMode
    -- (reset legs ride jaraxxusReset ev23, which Go fires first) +
    -- instance->SetBossState(DATA_JARAXXUS, FAIL) +
    -- me->DespawnOrUnsummon() (:151 — the despawn IS in C++, so it
    -- ports, last). The FAIL relay is the instance script's SetBossState
    -- hook (instance_trial_of_the_crusader.cpp:182-187): fordring
    -- DoAction(ACTION_JARAXXUS_WIPE 24) (:185-186) +
    -- MistressOfPainCount = 0 (:187). The TrialCounter decrement is the
    -- generic heroic-wipe leg (:303-305). Guard: C++ sets FAIL
    -- unconditionally, but ev2 can fire twice on a re-evade — the
    -- relay/counter must fire once per wipe, so skip when already FAIL
    -- or DONE.
    if creature:GetBossState(DATA_JARAXXUS) ~= BOSS_STATE_FAIL
            and creature:GetBossState(DATA_JARAXXUS) ~= BOSS_STATE_DONE then
        creature:SetBossState(DATA_JARAXXUS, BOSS_STATE_FAIL)
        sendToFordring(creature, ACTION_JARAXXUS_WIPE)
        local mapObj = creature:GetMap()
        if mapObj then
            local data = mapObj:GetInstanceData()
            if data then
                if creature:IsHeroic() then
                    local c = (data.trialCounter or 50) - 1
                    if c < 0 then c = 0 end
                    data.trialCounter = c
                    -- TrialCounter == 0 -> despawn Anub'arak (instance :313-317).
                    if c == 0 then
                        local anubarak = creature:GetInstanceCreature(DATA_ANUBARAK)
                        if anubarak then
                            anubarak:DespawnOrUnsummon()
                        end
                    end
                end
                data.mistressOfPainCount = 0
            end
        end
    end
    creature:DespawnOrUnsummon()
end

local function jaraxxusTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL_PLAYER)
    end
end

local function jaraxxusDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(SAY_DEATH)
    -- C++ _JustDied() (ScriptedCreature.cpp:513-519): events.Reset() +
    -- scheduler.CancelAll() ride cancelTimers above; summons.DespawnAll()
    -- stays out (no summon-registry/DespawnAll bridge — the nether-portal
    -- / infernal summons are spell-spawned, not Lua-tracked);
    -- instance->SetBossState(DATA_JARAXXUS 1, DONE 3) ports via
    -- creature:SetBossState (lua_creature_events.go:6155; curator/archavon
    -- precedent). The instance-script DONE legs
    -- (instance_trial_of_the_crusader.cpp:189-194) port tick 406:
    -- fordring ACTION_JARAXXUS_DEFEATED (9) (:191-192) via sendToFordring;
    -- EventStage = 2000 (:193) stays out — no instance-stage bridge.
    creature:SetBossState(DATA_JARAXXUS, BOSS_STATE_DONE)
    sendToFordring(creature, ACTION_JARAXXUS_DEFEATED)
end

local function jaraxxusReset(event, creature)
    creature:SetCombatPulseDelay(0) -- boss_lord_jaraxxus.cpp:118 (Reset arm 1; pulse disable, ResetLootMode leg has no bridge)
    cancelTimers(creature:GetGUID())
    -- C++ boss_jaraxxus::Reset (:116-130): events.Reset() /
    -- summons.DespawnAll() ride cancelTimers above (DespawnAll itself
    -- stays out — no summon-registry bridge); NOT_STARTED ->
    -- DoAction(ACTION_JARAXXUS_INTRO 15); FAIL -> DoCastSelf(
    -- SPELL_JARAXXUS_CHAINS 67924) + SetImmuneToPC(true) [no bridge,
    -- stays out — verified: no such method in engine/] +
    -- SetReactState(REACT_PASSIVE). GetBossState IS bridged
    -- (lua_creature_events.go:6174) — the old "FAIL gate has no bridge"
    -- claim is STALE. On evade ev23 fires before ev2 sets FAIL, so the
    -- state here is still IN_PROGRESS and neither branch fires.
    local guid = creature:GetGUID()
    local state = creature:GetBossState(DATA_JARAXXUS)
    if state == BOSS_STATE_NOT_STARTED then
        startIntro(creature, guid)
    elseif state == BOSS_STATE_FAIL then
        creature:CastSpell(creature, SPELL_JARAXXUS_CHAINS)
        creature:SetReactState(REACT_PASSIVE)
    end
end

-- C++ boss_jaraxxus::MovementInform (:171-178): type ==
-- SPLINE_CHAIN_MOTION_TYPE (17) and pointId == POINT_SUMMONED (1) ->
-- SetFacingToObject(wilfred via instance->GetCreature(DATA_FIZZLEBANG
-- 16)) + EVENT_TAUNT_GNOME 9s. Nil wilfred schedules nothing (the C++
-- if-guard). ev28 args are (event, creature, type, id)
-- (npc_nether_drake.lua precedent); SetFacingToObject is bridged
-- (lua_creature_events.go:4534) — the old atan2-twin note is superseded.
local function jaraxxusMovementInform(event, creature, moveType, pointId)
    if moveType ~= SPLINE_CHAIN_MOTION_TYPE or pointId ~= POINT_SUMMONED then
        return
    end
    local wilfred = creature:GetInstanceCreature(DATA_FIZZLEBANG)
    if wilfred ~= nil then
        creature:SetFacingToObject(wilfred)
        local guid = creature:GetGUID()
        schedule(guid, "taunt", 9000, function() tauntTick(creature, guid) end)
    end
end

-- C++ boss_jaraxxus::DoAction (:153-168), ev38. INTRO (15) re-enters
-- startIntro (same arm Reset calls); ENGAGE (21) is the Barrett
-- Jaraxxus-fail branch receiver: RemoveAurasDueToSpell(
-- SPELL_JARAXXUS_CHAINS 67924) + SetImmuneToPC(false) [no bridge,
-- stays out] + SetReactState(REACT_AGGRESSIVE) + DoZoneInCombat.
-- DoAction fires ev38 on the target (lua_creature_events.go:827).
local function jaraxxusDoAction(event, creature, action)
    if action == ACTION_JARAXXUS_INTRO then
        startIntro(creature, creature:GetGUID())
    elseif action == ACTION_JARAXXUS_ENGAGE then
        creature:RemoveAura(SPELL_JARAXXUS_CHAINS)
        creature:SetReactState(REACT_AGGRESSIVE)
        creature:DoZoneInCombat()
    end
end

RegisterCreatureEvent(ENTRY_JARAXXUS, 1, jaraxxusEnterCombat)
RegisterCreatureEvent(ENTRY_JARAXXUS, 2, jaraxxusLeaveCombat)
RegisterCreatureEvent(ENTRY_JARAXXUS, 3, jaraxxusTargetDied)
RegisterCreatureEvent(ENTRY_JARAXXUS, 4, jaraxxusDied)
RegisterCreatureEvent(ENTRY_JARAXXUS, 23, jaraxxusReset)
RegisterCreatureEvent(ENTRY_JARAXXUS, 28, jaraxxusMovementInform)
RegisterCreatureEvent(ENTRY_JARAXXUS, 38, jaraxxusDoAction)

-- C++ npc_mistress_of_pain JustEngagedWith EVENT_SHIVAN_SLASH:
-- DoCastVictim(Shivan Slash 66378), 4s init, urand(3s,10s) repeat.
local function shivanSlashTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "shivanslash", 500, function() shivanSlashTick(creature, guid) end)
        return
    end
    creature:CastSpell(nil, SPELL_SHIVAN_SLASH)
    schedule(guid, "shivanslash", math.random(3000, 10000), function()
        shivanSlashTick(creature, guid)
    end)
end

-- C++ EVENT_SPINNING_SPIKE: SelectTarget(Random,0,0.0f,true) ->
-- DoCast(Spinning Spike 66283) non-triggered, 9s init, Repeat(20s)
-- outside the target check (re-arm unconditional).
local function spinningSpikeTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "spinningspike", 500, function() spinningSpikeTick(creature, guid) end)
        return
    end
    local target = randomThreatPlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_SPINNING_SPIKE)
    end
    schedule(guid, "spinningspike", 20000, function()
        spinningSpikeTick(creature, guid)
    end)
end

local function mistressKissTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "mistresskiss", 500, function() mistressKissTick(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_MISTRESS_KISS)
    schedule(guid, "mistresskiss", 30000, function()
        mistressKissTick(creature, guid)
    end)
end

local function mistressEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "shivanslash", 4000, function()
        shivanSlashTick(creature, guid)
    end)
    schedule(guid, "spinningspike", 9000, function()
        spinningSpikeTick(creature, guid)
    end)
    -- C++ npc_mistress_of_pain JustEngagedWith: EVENT_MISTRESS_KISS
    -- DoCastSelf(Mistress' Kiss 66336) non-triggered, 15s init,
    -- Repeat(30s) outside the cast (re-arm unconditional), seeded only
    -- when IsHeroic() (the "no difficulty bridge (kelidan precedent)"
    -- header claim is STALE — creature:IsHeroic() is bridged
    -- engine/world/lua_creature_events.go:260, lavanthor precedent).
    if creature:IsHeroic() then
        schedule(guid, "mistresskiss", 15000, function()
            mistressKissTick(creature, guid)
        end)
    end
end

local function mistressLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function mistressDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    -- C++ npc_mistress_of_pain::JustDied (boss_lord_jaraxxus.cpp:421-424):
    -- _instance->SetData(DATA_MISTRESS_OF_PAIN_COUNT 302, DECREASE 502)
    -- (trial_of_the_crusader.h:73, :76); instance SetData decrements
    -- MistressOfPainCount (instance_trial_of_the_crusader.cpp:431-436),
    -- read only by the THREE_SIXTY_PAIN_SPIKE achievement check (:636,
    -- MistressOfPainCount >= 2 — no Go achievement-criteria bridge, stays
    -- out). Mirror: bank the count in the shared instanceData table
    -- (boss_faction_champions.lua championCountDeath precedent).
    -- Floored at 0: the paired INCREASE leg is ported tick 408 in
    -- mistressReset below, so the field only decrements after a real
    -- increment — C++ MistressOfPainCount is uint8 and never goes below 0.
    -- creature:SetData is NOT used: it is per-creature ScriptData
    -- (lua_creature_events.go:792), not the instance script's SetData.
    local mapObj = creature:GetMap()
    if mapObj then
        local data = mapObj:GetInstanceData()
        if data and (data.mistressOfPainCount or 0) > 0 then
            data.mistressOfPainCount = data.mistressOfPainCount - 1
        end
    end
end

local function mistressReset(event, creature)
    cancelTimers(creature:GetGUID())
    -- C++ npc_mistress_of_pain::Reset (boss_lord_jaraxxus.cpp:398-411):
    -- instance->GetBossState(DATA_JARAXXUS) != IN_PROGRESS ->
    -- DespawnOrUnsummon (:400-404; GetBossState IS bridged,
    -- lua_creature_events.go:6174 — the old "no instance boss-state
    -- read bridge" claim is STALE); else DoCastSelf(Hittin' Ya 66327,
    -- triggered) (:409) + _instance->SetData(
    -- DATA_MISTRESS_OF_PAIN_COUNT 302, INCREASE 501) (:410, mirrored
    -- in the shared instanceData table like JustDied — creature:SetData
    -- is per-creature ScriptData, lua_creature_events.go:792, not the
    -- instance). The cross-AI jaraxxus->AI()->JustSummoned leg
    -- (:406-407) re-examined tick 410 U2: leg (a) summons.Summon
    -- stays out (no bridge); leg (b) DoZoneInCombat is PORTED below
    -- (IN_PROGRESS gate is the engaged check — see felInfernalReset).
    if creature:GetBossState(DATA_JARAXXUS) ~= BOSS_STATE_IN_PROGRESS then
        creature:DespawnOrUnsummon()
        return
    end
    -- BossAI::JustSummoned leg (b) (ScriptedCreature.cpp:559-564) — see
    -- felInfernalReset above: IN_PROGRESS gate is the engaged check,
    -- the Reset carries the zone leg (Go fires ev19 after Reset).
    creature:DoZoneInCombat()
    creature:CastSpell(creature, SPELL_HITTIN_YA, true)
    local mapObj = creature:GetMap()
    if mapObj then
        local data = mapObj:GetInstanceData()
        if data then
            data.mistressOfPainCount = (data.mistressOfPainCount or 0) + 1
        end
    end
end

RegisterCreatureEvent(ENTRY_MISTRESS_OF_PAIN, 1, mistressEnterCombat)
RegisterCreatureEvent(ENTRY_MISTRESS_OF_PAIN, 2, mistressLeaveCombat)
RegisterCreatureEvent(ENTRY_MISTRESS_OF_PAIN, 4, mistressDied)
RegisterCreatureEvent(ENTRY_MISTRESS_OF_PAIN, 23, mistressReset)

-- C++ npc_legion_flame::Reset (boss_lord_jaraxxus.cpp:297-310):
-- instance->GetBossState(DATA_JARAXXUS) != IN_PROGRESS ->
-- DespawnOrUnsummon (:299-303); else ctor SetCombatMovement(false)
-- (:294; no Lua ctor hook — Reset is the spawn hook, ayamiss
-- precedent) + SetReactState(REACT_PASSIVE) (:308) + DoCastSelf(
-- Legion Flame Effect 66201, triggered) (:309). The cross-AI
-- jaraxxus->AI()->JustSummoned leg (:305-306) stays out (no cross-AI
-- bridge). No UpdateAI in C++ — the flame has no combat ticks.
local function legionFlameReset(event, creature)
    cancelTimers(creature:GetGUID())
    if creature:GetBossState(DATA_JARAXXUS) ~= BOSS_STATE_IN_PROGRESS then
        creature:DespawnOrUnsummon()
        return
    end
    creature:SetCombatMovement(false)
    creature:SetReactState(REACT_PASSIVE)
    creature:CastSpell(creature, SPELL_LEGION_FLAME_EFFECT, true)
end

RegisterCreatureEvent(ENTRY_LEGION_FLAME, 23, legionFlameReset)

-- C++ npc_fel_infernal Reset scheduler (boss_lord_jaraxxus.cpp:
-- :352-357) + UpdateAI (:362-371): _scheduler.Schedule(2s,
-- SelectTarget(Random,0,0.0f,true) -> DoCast(Fel Streak Visual 66493)
-- non-triggered, Repeat(15s) outside the target check (re-arm
-- unconditional); the scheduler validator !UNIT_STATE_CASTING
-- (:347-350) rides the 500ms IsCasting defer (mal_ganis idiom).
-- UpdateAI's UpdateVictim gate (:364) is implicit: with no threat
-- targets the cast is skipped and the re-arm still fires. Melee is
-- engine-driven in Go, like C++ DoMeleeAttackIfReady (:369).
local function felStreakTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "felstreak", 500, function() felStreakTick(creature, guid) end)
        return
    end
    local target = randomThreatPlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_FEL_STREAK_VISUAL)
    end
    schedule(guid, "felstreak", 15000, function()
        felStreakTick(creature, guid)
    end)
end

-- C++ npc_fel_infernal::Reset (boss_lord_jaraxxus.cpp:336-360):
-- instance->GetBossState(DATA_JARAXXUS) != IN_PROGRESS ->
-- DespawnOrUnsummon (:338-342); else the Fel Streak scheduler above
-- (2s init) + DoCastSelf(Lord Jaraxxus' Hittin' Ya 66327, triggered)
-- (:359). The cross-AI jaraxxus->AI()->JustSummoned leg (:344-345)
-- stays out (no cross-AI bridge). No JustEngagedWith in C++ — the
-- scheduler starts in Reset, not on engage.
local function felInfernalReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    if creature:GetBossState(DATA_JARAXXUS) ~= BOSS_STATE_IN_PROGRESS then
        creature:DespawnOrUnsummon()
        return
    end
    -- BossAI::JustSummoned leg (b) (ScriptedCreature.cpp:559-564): the
    -- boss zones the summon in combat when engaged — the IN_PROGRESS
    -- gate above IS the engaged check, and Go fires ev19 after the
    -- summon's Reset, so the Reset carries the leg. DoZoneInCombat is
    -- bridged (lua_creature_events.go:1944) but Map.dbc-gated (no
    -- Map.dbc ships), so it is a no-op today, live if DBCs land.
    creature:DoZoneInCombat()
    schedule(guid, "felstreak", 2000, function()
        felStreakTick(creature, guid)
    end)
    creature:CastSpell(creature, SPELL_HITTIN_YA, true)
end

local function felInfernalLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_FEL_INFERNAL, 2, felInfernalLeaveCombat)
RegisterCreatureEvent(ENTRY_FEL_INFERNAL, 23, felInfernalReset)

-- C++ npc_infernal_volcano::Reset (boss_lord_jaraxxus.cpp:323-329):
-- ctor SetCombatMovement(false) (:321) + SetReactState(REACT_PASSIVE)
-- (:324) + DoCastSelf(Infernal Eruption Effect 66252, triggered)
-- (:326) + heroic RemoveFlag(UNIT_FLAG_NOT_SELECTABLE) (:327-328).
-- No instance gate in C++ (unlike legion-flame / fel-infernal /
-- mistress); no UpdateAI, no timers.
local function infernalVolcanoReset(event, creature)
    creature:SetCombatMovement(false)
    creature:SetReactState(REACT_PASSIVE)
    creature:CastSpell(creature, SPELL_INFERNAL_ERUPTION_EFFECT, true)
    -- Second-order delivery — PORTED tick 410 U2: 66252's summon effect
    -- (fel infernal 34815, 1:1 per eruption) rides the same dead DBC
    -- path, so the infernal is summoned explicitly at the volcano's
    -- position. The summon runs through summonTempCreature, so the
    -- infernal's ev23 Reset arms synchronously (felInfernalReset).
    creature:SummonCreature(ENTRY_FEL_INFERNAL, creature:GetX(), creature:GetY(), creature:GetZ(), creature:GetO())
    if creature:IsHeroic() then
        creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    end
end

RegisterCreatureEvent(ENTRY_INFERNAL_VOLCANO, 23, infernalVolcanoReset)

-- C++ npc_nether_portal::Reset (boss_lord_jaraxxus.cpp:378-390):
-- ctor SetCombatMovement(false) (:380) + SetReactState(REACT_PASSIVE)
-- (:386) + DoCastSelf(Nether Portal Effect 66263, triggered) (:388)
-- + heroic RemoveFlag(UNIT_FLAG_NOT_SELECTABLE) (:389-390).
-- No instance gate in C++; no UpdateAI, no timers.
local function netherPortalReset(event, creature)
    creature:SetCombatMovement(false)
    creature:SetReactState(REACT_PASSIVE)
    creature:CastSpell(creature, SPELL_NETHER_PORTAL_EFFECT, true)
    -- Second-order delivery — PORTED tick 410 U2: 66263's summon effect
    -- (mistress of pain 34826, 1:1 per portal) rides the same dead DBC
    -- path, so the mistress is summoned explicitly at the portal's
    -- position. The summon runs through summonTempCreature, so the
    -- mistress's ev23 Reset arms synchronously (mistressReset).
    creature:SummonCreature(ENTRY_MISTRESS_OF_PAIN, creature:GetX(), creature:GetY(), creature:GetZ(), creature:GetO())
    if creature:IsHeroic() then
        creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    end
end

RegisterCreatureEvent(ENTRY_NETHER_PORTAL, 23, netherPortalReset)

-- C++ spell_mistress_kiss::HandleDummyTick
-- (boss_lord_jaraxxus.cpp:480-489): on each periodic tick of the
-- Mistress' Kiss aura (66334/67905/67906/67907, EFFECT_0
-- SPELL_AURA_PERIODIC_DUMMY), if the aura target is casting, the
-- caster casts Mistress' Kiss Damage+Silence (66359) on the target,
-- triggered, and the target's kiss aura is removed. Both actions sit
-- inside the caster-exists + target-casting gates — a nil caster
-- skips the removal too. No PreventDefaultAction in C++, so the
-- handler returns nothing (the default PERIODIC_DUMMY tick is a
-- no-op in both engines). PORTED tick 371 via RegisterAuraEvent
-- (AuraEventOnPeriodic = 1); aura:GetAuraId reads the ticking rank
-- for the removal, so one handler serves all four ranks.
local SPELL_MISTRESS_KISS_DAMAGE_SILENCE = 66359 -- boss_lord_jaraxxus.cpp:74

local function mistressKissAuraTick(event, aura, target)
    local caster = aura:GetCaster()
    if caster and target:IsCasting() then
        caster:CastSpell(target, SPELL_MISTRESS_KISS_DAMAGE_SILENCE, true)
        target:RemoveAura(aura:GetAuraId())
    end
end

RegisterAuraEvent(66334, 1, mistressKissAuraTick)
RegisterAuraEvent(67905, 1, mistressKissAuraTick)
RegisterAuraEvent(67906, 1, mistressKissAuraTick)
RegisterAuraEvent(67907, 1, mistressKissAuraTick)
