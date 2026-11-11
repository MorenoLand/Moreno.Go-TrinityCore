-- Yogg-Saron (Ulduar) — Lua port of
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_yogg_saron.cpp
-- (boss_voice_of_yogg_saron (CreatureScript via
-- GetUlduarAI<boss_voice_of_yogg_saronAI> (BossAI),
-- BOSS_YOGG_SARON = 12); boss_sara (CreatureScript via
-- GetUlduarAI<boss_saraAI> (ScriptedAI)); boss_yogg_saron
-- (CreatureScript via GetUlduarAI<boss_yogg_saronAI>
-- (PassiveAI)); boss_brain_of_yogg_saron (CreatureScript via
-- GetUlduarAI<boss_brain_of_yogg_saronAI> (PassiveAI));
-- fourteen npc_* scripts (npc_ominous_cloud /
-- npc_guardian_of_yogg_saron / npc_corruptor_tentacle /
-- npc_constrictor_tentacle / npc_crusher_tentacle /
-- npc_influence_tentacle / npc_descend_into_madness /
-- npc_immortal_guardian / npc_observation_ring_keeper /
-- npc_yogg_saron_keeper / npc_yogg_saron_illusions /
-- npc_garona / npc_turned_champion / npc_laughing_skull);
-- thirty-three spell_* scripts (all SpellScriptLoader, no
-- quoted names); all registered from inside
-- AddSC_boss_yogg_saron(); loader decl 122 / call 317 per
-- northrend_script_loader.cpp — the THIRTEENTH group of the
-- "// Ulduar" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_thorim() (decl 121 / call 316); the
-- checkpoint sequence (boss_thorim -> boss_yogg_saron) is
-- followed).
-- Entries: 33134 Sara (ulduar.h NPC_SARA, line 197);
-- 33288 Yogg-Saron (ulduar.h NPC_YOGG_SARON, line 82) —
-- both entry-verifiable, registrations proceed (the
-- nexus_commanders / malygos / sartharion kalecgos
-- precedent); the CreatureScript ScriptName bindings are
-- DB-side as usual.
-- Sole-source verified: whole-server-tree grep for each of
-- the eighteen quoted creature script names hits
-- boss_yogg_saron.cpp only (+ the loader decl/call lines
-- for AddSC_boss_yogg_saron); this clone carries no sql/
-- tree, so the ScriptName bindings are DB-side by
-- construction.
-- Audited artifact: this lua was ported 2026-10-02 09:40 in
-- e0f1acf and re-audited this run; the header below documents
-- that audit.
-- Eluna creature events: 1 OnEnterCombat, 4 OnDied.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Sara Talk(SAY_SARA_AGGRO 2) (event 1;
-- the three ScheduleEvent legs (EVENT_SARAS_FERVOR /
-- EVENT_SARAS_BLESSING / EVENT_SARAS_ANGER) have no
-- bridges — the auriaya engage-port precedent; Sara's AI is
-- ScriptedAI, but the engage port lives on the Eluna
-- event-1 hook, not on the AI base).
-- JustDied — Yogg-Saron Talk(SAY_YOGG_SARON_DEATH 6)
-- (event 4; the cross-creature legs — Unit::Kill(voice),
-- sara/brain DisappearAndDie, keeper EnterEvadeMode — and
-- the player SPELL_SANITY / SPELL_INSANE aura removal have
-- no bridges — the sjonnir JustDied-Talk precedent).
-- Reset — DoCast(me, SPELL_SHADOWY_BARRIER_YOGG 63894) +
-- DoCast(me, SPELL_KNOCK_AWAY 64022), both non-triggered and ungated
-- (:936-937), ported on event 23 (the urom evocate-reset precedent);
-- the _events phase/timer and loot-mode legs stay documented-only.
-- DOCUMENTED-ONLY (in this header; no registrations beyond
-- entries 33134 and 33288), except the Sara KilledUnit below —
-- player-gated AND !IsInEvadeMode-gated Talk(SAY_SARA_KILL 5),
-- PORTED on event 3 (it was documented-only until the IsInEvadeMode
-- bridge was re-verified; C++-exact):
-- Sara DamageTaken — PORTED (saraDamageTaken, event 9): lethal
-- (damage >= health) is NOT a death rewrite: damage = health - 1
-- (survive with 1 hp), unconditional in C++; the first-lethal
-- PHASE_ONE chain (Talk 6 + 4700/9500/14300/14500ms Talks, full heal,
-- faction, phase-2 transform, barrier) rides the same handler, with
-- only the cross-creature DoAction legs staying out (see the handler).
-- Sara SpellHitTarget — Talk(SAY_SARA_FERVOR_HIT /
-- SAY_SARA_BLESSING_HIT / SAY_SARA_PSYCHOSIS_HIT) gated on
-- roll_chance_i(30) and !PHASE_TRANSFORM — stays out on trigger-path
-- grounds: the EVENT_SARAS_FERVOR / BLESSING / ANGER casts are unported
-- and the phase gate has no bridge, so event 15 never fires for them in
-- Go (event 15 itself is live — boss_ai.go:1904).
-- Sara JustSummoned — Talk(SAY_SARA_DEATH_RAY) on NPC_DEATH_ORB
-- summon + self-cast SPELL_DEATH_RAY_ORIGIN_VISUAL are PORTED
-- (saraJustSummoned; the orb arrives via a spell summon leg, unported
-- — LATENT). The 4 NPC_DEATH_RAY summons in the same arm stay out
-- (map GetHeight terrain bridge absent — frost-sphere precedent);
-- the voice-of-Yogg forward stays out (cross-AI, no bridge).
-- Sara Reset — PORTED (saraReset, event 23): RemoveAllAuras +
-- SetReactState(REACT_PASSIVE) + SetFaction(FACTION_FRIENDLY)
-- (:783-790); only the _events Reset/SetPhase(PHASE_ONE) legs stay
-- unmodeled (phase machine, no bridge).
-- Voice JustSummoned — the triggered one-shot legs are PORTED
-- (voiceJustSummoned, event 19): constrictor Lunge, descend portal
-- visual, immortal simple teleport (all triggered), Yogg emerge emote.
-- The crusher/corruptor REACT_PASSIVE (:660-665) + StartAttackEvent
-- attack kick (:430-439, 1s/5s delayed BasicEvent) stay out: the
-- passive arm without the AttackStart kick would strand the summons,
-- and AttackStart has no bridge (DoZoneInCombat seeds whole-map threat,
-- not C++'s single-target AttackStart).
-- Sara timer Talks — 2026-10-14 retraction re-audit: none ported on
-- combat-start timers; all six are phase-gated with unbridged schedule
-- legs (stale "no-timer-bridge" claim RETRACTED — CreateLuaEvent is a
-- real bridge; gating is the blocker, gothik precedent).
-- EVENT_TRANSFORM_1/2/3 (:821-827, Talk 7/8 + full-heal/DoAction arm)
-- — PORTED in self-arm form (saraDamageTaken, event 9): the
-- damage=health-1 rewrite (:719-722), PHASE_ONE first-lethal latch,
-- and the 4700/9500/14300/14500ms Talk/heal/faction/transform/barrier
-- legs; the cross-creature voice->AI()->DoAction(
-- ACTION_PHASE_TRANSFORM) (:728-729) and TRANSFORM_3's
-- DoAction(ACTION_PHASE_TWO) legs (:831/:833) are PORTED below
-- (saraDamageTaken / saratransform3, via the shared instanceData
-- table; the voice/mimiron receivers are ported in this file).
-- Yogg-Saron timer Talks — EVENT_YELL_BOW_DOWN (Talk id 0, one-shot
-- 3s) fires from Reset (:933) with SetPhase(PHASE_TWO) (:932; yogg is
-- PassiveAI, never enters combat) — PORTED below (yoggSaronReset,
-- event 23; the summon-driven Reset fires on the temp-summon path).
-- EVENT_SHADOW_BEACON (DoCastAOE 64465 + Emote 7, 45s repeat) +
-- EVENT_LUNATIC_GAZE (DoCast 64163, 12s repeat) + EVENT_DEAFENING_ROAR
-- (DoCastAOE 64189 + Talk 4/Emote 5, {20s,25s} repeat) — scheduled
-- from DoAction(ACTION_PHASE_THREE) (:1020-1035, PHASE_THREE),
-- deafening roar additionally gated on Is25ManRaid + instance
-- DATA_KEEPERS_COUNT (:1026, read from the shared instanceData
-- keepersCount) — PORTED below (yoggDoAction, event 38) with the
-- brain DamageTaken cascade as sender. The lunatic-gaze SendSound
-- leg stays out (no SendSound bridge).
-- Yogg-Saron DoAction(ACTION_PHASE_THREE) — Talk(SAY_YOGG_SARON_PHASE_3)
-- + DoCast PHASE_3_TRANSFORM + barrier-aura removal +
-- ResetPlayerDamageReq — PORTED below (yoggDoAction, event 38).
-- Yogg-Saron SpellHit — Val'anyr loot-mode leg (no Talk) —
-- PORTED via event 14 (yoggSaronSpellHit below); the Reset
-- ResetLootMode leg (:939) rides the unbridged scheduler
-- (ResetLootMode itself IS bridged, lua_objects.go:1003; the
-- ladder is blocked on keepers instance-data, not the bridge).
-- Voice of Yogg-Saron (33280, ulduar.h line 203) —
-- PORTED: JustEngagedWith DoCast(me, SPELL_SANITY_PERIODIC 63786)
-- (2-arg self, non-triggered, ungated, :525) + the 15s
-- EVENT_LOCK_DOOR DoCast(me, SPELL_INSANE_PERIODIC 64554)
-- one-shot (:556-557, no phase arg on the schedule) — both ride
-- event 1 with the SetInCombatWith keeper leg left documented-only
-- (no instance bridge — the assembly_of_iron keeper-combat
-- precedent); the DoCastAOE(SPELL_SUMMON_GUARDIAN_2, spellvalue) /
-- EVENT_EXTINGUISH_ALL_LIFE / EVENT_SUMMON_GUARDIAN_OF_YOGG_SARON
-- schedule legs are PORTED below (voiceEnterCombat + guardianTick /
-- extinguishTick); JustDied has no
-- Talk arm (summons.Despawn + _JustDied() only); EnterEvadeMode
-- Talk(WHISPER_VOICE_PHASE_1_WIPE, player) rides a
-- player-loop wipe leg (RemoveAurasDueToSpell SPELL_SANITY
-- / SPELL_INSANE — PORTED via GetPlayersInWorld +
-- player:RemoveAura in yoggSaronReset/yoggSaronDied; the removals
-- are ungated in C++, only the whisper is PHASE_ONE-gated, and the
-- whisper keeps no phase bridge); timer Talk EMOTE_YOGG_SARON_
-- EXTINGUISH_ALL_LIFE on EVENT_EXTINGUISH_ALL_LIFE is PORTED below
-- (extinguishTick, event-1-seeded timer); EMOTE_YOGG_SARON_MADNESS + SAY_YOGG_SARON_MADNESS
-- on EVENT_ILLUSION are PORTED below (illusionTick, event-38-
-- seeded timer) with the tentacle summon arms and the brain
-- ACTION_INDUCE_MADNESS dispatch.
-- Brain of Yogg-Saron (33890, ulduar.h line 218) — Reset
-- DoCast(me, SPELL_MATCH_HEALTH 64066) PORTED below (event 23);
-- DamageTaken below-30% cascade (:1070-1090): DoCastAOE(
-- SPELL_SHATTERED_ILLUSION_REMOVE 65238, triggered) + DoCast(me,
-- SPELL_MATCH_HEALTH_2 64069, triggered) + DoCast(me,
-- SPELL_BRAIN_HURT_VISUAL 64361, triggered) + DoAction(
-- ACTION_PHASE_THREE) to voice/sara/yogg/thorim/mimiron — PORTED
-- below (brainDamageTaken, event 9); the SetImmuneToPC(true) leg
-- stays out (no bridge). DoAction ACTION_INDUCE_MADNESS /
-- ACTION_TENTACLE_KILLED are PORTED below (brainDoAction, event
-- 38; the GO-door legs stay out — no HandleGameObject bridge)
-- with the influence JustDied sender.
-- npc_observation_ring_keeper — OnGossipSelect PORTED below
-- (observationKeeperGossipSelect): menuId 10333 never reaches the
-- Lua select hook (sender/action only — gossip.go:351), so the gate
-- uses the sender == 0 DB-menu proxy (akama precedent); the keeper
-- class defines no hello/quest menu of its own. Reset one-shots
-- PORTED below (event 23); ring-keeper entries 33213 / 33241 /
-- 33242 / 33244, ulduar.h lines 199-202.
-- npc_yogg_saron_keeper (33410 / 33411 / 33412 / 33413,
-- ulduar.h lines 205-208) — no Talk arms — Reset + JustEnteredCombat
-- one-shots PORTED below (events 23 / 1).
-- npc_yogg_saron_illusions / npc_garona /
-- npc_turned_champion / npc_laughing_skull — roleplay
-- Talks (SAY_CHAMBER_ROLEPLAY_1-5 / SAY_ICECROWN_ROLEPLAY_1-6 /
-- SAY_STORMWIND_ROLEPLAY_1-7) fire from IsSummonedBy /
-- the illusion UpdateAI timer machine on instance
-- DATA_ILLUSION — no bridges; no registrations (no RegisterLuaBoss
-- for 33437 / 33441 / 33442 / 33495 / 33523 / 33535 / 33962;
-- creature:FindNearestCreature is live (lua_creature_events.go:1237)
-- but has no handler to attach to). The FindNearestCreature legs
-- stay out on those grounds: boss_yogg_saron.cpp:1783-1818
-- (immolated-champion -> Lich King DeathGrasp relay, Lich King /
-- Llane roleplay Talks) and :1877-1882 (garona FindNearestCreature(
-- NPC_KING_LLANE, 50.0f) + DoCast(SPELL_ASSASSINATE) +
-- llane->CastSpell(llane, SPELL_PERMANENT_FEIGN_DEATH)). (Turned
-- champion's Reset DoCast(SPELL_VERTEX_COLOR_BLACK 39662)
-- one-shot is PORTED below — event 23 — not roleplay-gated.)
-- Tentacle / cloud / guardian creatures (33136 / 33292 /
-- 33943 / 33966 / 33983 / 33985 / 33988 / 34072, ulduar.h
-- lines 198 / 204 / 219 / 221 / 222 / 223 / 224 / 226) —
-- guardian (33136) / corruptor (33985) / crusher (33966) /
-- immortal (33988) timer + Reset arms registered below;
-- constrictor (33983) Reset one-shots DoCast(me,
-- SPELL_TENTACLE_VOID_ZONE_2 64384) + DoCastAOE(SPELL_ERUPT
-- 64144), influence (33943) Reset entry-conditional DoCast(me,
-- 64013/64010), ominous cloud (33292) Reset DoCast(me,
-- SPELL_OMINOUS_CLOUD_VISUAL 63084) PORTED below (event 23).
-- The Talk arms inside SpellScripts / AuraScripts —
-- Talk(EMOTE_OMINOUS_CLOUD_PLAYER_TOUCH) in
-- spell_yogg_saron_boil_ominously (HandleDummy);
-- Talk(WHISPER_VOICE_INSANE) in spell_yogg_saron_insane
-- (OnApply) — no SpellScript bridge; the AuraScript side is
-- no longer blanket-blocked (tick 189): RegisterAuraEvent
-- (AuraEventOnPeriodic = 1, runtime.go) ports
-- spell_yogg_saron_empowered OnPeriodic (64161,
-- empoweredTick below) and spell_yogg_saron_sanity DummyTick
-- (63050, sanityTick below); spell_yogg_saron_brain_link (63802)
-- PORTED this tick below (yoggBrainLinkFilter / yoggBrainLinkTick /
-- yoggBrainLinkRemoved): the SaraAI _linkData map rides a PHASE3
-- module table (the 431 pursueTargetGuid precedent), the linked
-- player resolves via GetPlayerByGUID, and the distance threshold
-- reads aura:GetAmount(0) (lua_aura.go). No SaraAI bridge exists;
-- the C++ OnRemove SaraAI-cast gate rides aura:GetCaster() ~= nil.
-- Residuals
-- retired: empowered OnApply (stack-9 buff) ported tick 438
-- via ev2 (empoweredApply below); sanity OnRemove ported
-- tick 436 via ev3 (sanityRemove below).
-- spell_yogg_saron_target_selectors (63744/63745/63747/65206 DUMMY
-- cast-effect-value, ev5) + psychosis (63795/65301, ev7 filter + ev5
-- sanity -9/-12) + malady_of_the_mind (63830/63881, ev7 filter on 63830
-- only + ev5 sanity -3 + ev3 OnRemove jump cast) PORTED 2026-11-11
-- below (PHASE3.yoggTargetSelectorHandleScript /
-- PHASE3.yoggHighSanityFilter / PHASE3.yoggSanityReduction /
-- PHASE3.yoggMaladyRemoved — table fields, the file is at the
-- 200-local limit). Census rows 456-461 PORTED 2026-11-11 below:
-- brain_link_damage (63803, ev5 sanity -2 via yoggSanityReduction),
-- boil_ominously (63030, ev5 flash-freeze/guardian-aura gates + Talk
-- 0 + triggered self-cast 63031), shadow_beacon (64465, ev2/ev3
-- UpdateEntry 36064/33988), empowering_shadows range check (64466,
-- ev5 hit-target casts effect value on caster, triggered) + missile
-- (64467, ev5 hit-target self-casts 64468, triggered); brain_link
-- (63802, ev7 link filter + ev1 distance-gated 63803/63804 tick +
-- ev3 unlink/strip) PORTED this tick via a PHASE3 link table (the
-- sibling's stays-out call is overturned: every leg has a bridge or
-- a precedent). Census go_hit=true is a FALSE POSITIVE on all six
-- rows (Lua-comment hits only, zero Go).
-- Census rows 472-476 PORTED 2026-11-11 below: revealed_tentacle
-- (64012, ev5 triggered 64017 + 63305 self-casts + UpdateEntry
-- 33943), grim_reprisal (63305, ev6 60%-of-damage 64039 BP0 leg),
-- induce_madness (64059, ev5 63992 teleport + SANITY remove mode 3
-- + effect-value remove; AfterCast ClearShatteredIllusion stays out
-- — no spell AfterCast event), sanity (63050, VERIFIED-EXISTING:
-- ev1 tick + ev3 remove, BeforeCast stays out), insane (63120,
-- ev2 Yogg Talk(1, target) + 64464 visual + ev3 alive -> KillSelf).
-- Census hooks wrong on 473/474/475/476 (473 OnEffectHitTarget,
-- 474 AfterEffectApply, 475 both, 476 OnEffectHitTarget +
-- OnObjectAreaTargetSelect phantoms); 472 exact.
-- Census rows 477-481 PORTED/STAYED-OUT 2026-11-11 below:
-- insane_periodic (64555, ev5 triggered effect-value cast),
-- lunatic_gaze (64164/64168, ev7 front-arc filter + ev5 sanity -4/-2
-- via yoggSanityReduction), hate_to_zero (63984, ev5 creature-gated
-- ModifyThreatByPercent -100; bound by the ToC worm Emerge DoCastAOE).
-- keeper_aura (62650/62670/62671/62702) STAYS OUT — no DoCheckAreaTarget
-- aura event (aura events 1-7, runtime.go); in_the_maws_of_the_old_god
-- (64184) STAYS OUT — no OnCheckCast spell event (spell events 1-7);
-- its loot-mode leg is already ported via the Yogg SpellHit handler
-- above. Census go_hit=true is a FALSE POSITIVE on all five (zero Go
-- hits); lua_hit true only via consts/comments.
-- Census hooks wrong on 477/479/480 (OnCheckCast phantoms; 477 also
-- lists OnObjectAreaTargetSelect); 478's OnObjectAreaTargetSelect is
-- real; 481's OnCheckCast is real but unbridged.
-- Census rows 482-486 PORTED/STAYED-OUT 2026-11-11 below:
-- titanic_storm (64172, ev5 EFFECT_0 DUMMY -> Unit::Kill(caster,
-- hitTarget); Kill bridges split by receiver, so the handler branches
-- on hitTarget type — creature hitTarget:Kill(caster), player
-- hitTarget:Kill(hitTarget) self-kill; fixes 5da47a5f's
-- caster:Kill(hitTarget), which killed the caster); hodirs_protective_gaze
-- (64174) STAYS OUT — DoCheckAreaTarget unbridged AND no OnEffectAbsorb
-- aura event (aura events 1-7, runtime.go); the Hodir engage cast of
-- 64174 is already ported (keeperEnterCombat). ulduar_teleporter
-- STAYS OUT — OnCheckCast only, no spell OnCheckCast event (spell
-- events 1-7); no spell ID in the loader, DBC-bound (note in
-- instance_ulduar.lua). ingvar woe_strike (42730) VERIFIED-EXISTING
-- (woeStrikeProc, ev6; the CheckProc heal > 0 gate rides procs.go's
-- heal == 0 early return) and its heroic twin 59735 registered this
-- tick (same AuraScript class binds both IDs). Census go_hit/lua_hit
-- true on 482-485 are FALSE POSITIVES (64171/const/comment hits, zero
-- Go); 486 was correctly false on both. Census hooks: 482 exact,
-- 484 real but unbridged; 485/486 list none and miss the real
-- OnEffectProc.

local ENTRY_SARA = 33134
local ENTRY_YOGG_SARON = 33288
local BOSS_YOGG_SARON = 12 -- ulduar.h:48

local SAY_SARA_AGGRO = 2
local SAY_SARA_KILL = 5
local SAY_SARA_DEATH_RAY = 10 -- boss_yogg_saron.cpp:49
local ENTRY_DEATH_ORB = 33882 -- ulduar.h:217
local ENTRY_DEATH_RAY = 33881 -- ulduar.h:216
local SPELL_DEATH_RAY_ORIGIN_VISUAL = 63893 -- boss_yogg_saron.cpp:230
local SPELL_DEATH_RAY_WARNING_VISUAL = 63882 -- boss_yogg_saron.cpp:890
local SAY_YOGG_SARON_DEATH = 6

local SPELL_SHADOWY_BARRIER_YOGG = 63894
local SPELL_KNOCK_AWAY = 64022
local SPELL_IN_THE_MAWS_OF_THE_OLD_GOD = 64184
local SPELL_SANITY = 63050
local SPELL_INSANE = 63120
local SPELL_BRAIN_LINK = 63802 -- boss_yogg_saron.cpp:139

-- Phase-3 machine consts (boss_yogg_saron.cpp:365-367 actions,
-- :282-287 phases, :112-169/:222 spells, :36-60 yells) — grouped in
-- one table: the file sits at the 200-local limit, so new consts ride
-- this table (the LK-table precedent). The brain DamageTaken cascade (:1070-1090)
-- sends ACTION_PHASE_THREE to the voice, sara, yogg and the
-- thorim/mimiron keepers; the receivers are ported this run.
local ACTION_PHASE_THREE = 2
local PHASE_THREE = 4
local PHASE3 = {
    SPELL_SHATTERED_ILLUSION_REMOVE = 65238, -- :169
    SPELL_MATCH_HEALTH_2 = 64069, -- :165
    SPELL_BRAIN_HURT_VISUAL = 64361, -- :167
    SPELL_IMMORTAL_GUARDIAN = 64158, -- :124
    SPELL_EXTINGUISH_ALL_LIFE = 64166, -- :154
    SPELL_SHADOW_BEACON = 64465, -- :161
    SPELL_LUNATIC_GAZE = 64163, -- :159
    SPELL_DEAFENING_ROAR = 64189, -- :158
    SPELL_PHASE_3_TRANSFORM = 63895, -- :157
    SPELL_TITANIC_STORM = 64171, -- :222
    SAY_YOGG_SARON_SPAWN = 0, -- :52
    SAY_YOGG_SARON_PHASE_3 = 3, -- :56
    SAY_YOGG_SARON_DEAFENING_ROAR = 4, -- :57
    EMOTE_YOGG_SARON_DEAFENING_ROAR = 5, -- :58
    EMOTE_YOGG_SARON_EMPOWERING_SHADOWS = 7, -- :59
    EMOTE_YOGG_SARON_EXTINGUISH_ALL_LIFE = 8, -- :60
    -- Target-selector / psychosis / malady ids (boss_yogg_saron.cpp
    -- enum lines per key) + malady remove modes: grouped here because
    -- the file is at the 200-local limit; the handlers below are
    -- assigned as fields of this table for the same reason.
    SARAS_ANGER = 63744, -- :132
    SARAS_BLESSING = 63745, -- :130
    SARAS_FERVOR = 63747, -- :128
    PSYCHOSIS = 63795, -- :137
    PSYCHOSIS_HEROIC = 65301, -- class comment :2022 (no in-tree enum/cast site; paired-comment precedent, cf. 62604)
    MALADY = 63830, -- :138
    MALADY_JUMP = 63881, -- :269
    BRAIN_LINK_DAMAGE = 63803, -- :140
    BRAIN_LINK_NO_DAMAGE = 63804, -- :141
    brainLinks = {}, -- SaraAI _linkData twin: GUID -> partner GUID, both directions
    BOIL_OMINOUSLY = 63030, -- class comment :2247 (no in-tree enum/cast site; DBC-gated)
    FLASH_FREEZE = 64175, -- :276
    SUMMON_GUARDIAN_1 = 63031, -- :146
    EMOTE_OMINOUS_CLOUD_PLAYER_TOUCH = 0, -- :71
    EMPOWERING_SHADOWS_RANGE_CHECK = 64466, -- class comment :2319
    EMPOWERING_SHADOWS_MISSILE = 64467, -- class comment :2346
    EMPOWERING_SHADOWS = 64468, -- :193
    NPC_IMMORTAL_GUARDIAN = 33988, -- ulduar.h:224
    NPC_MARKED_IMMORTAL_GUARDIAN = 36064, -- ulduar.h:227
    ILLUSION_ROOM = 63988, -- :270
    AURA_REMOVE_BY_EXPIRE = 4, -- SpellAuraDefines.h:67
    AURA_REMOVE_BY_DEATH = 5, -- SpellAuraDefines.h:68
    SPELL_CONSTRICTOR_TENTACLE_SUMMON = 64133, -- :274
    SPELL_SQUEEZE = 64125, -- :275
    SPELL_SQUEEZE_HEROIC = 64126, -- loader comment :2445 (difficulty twin, no in-tree enum)
    SPELL_DEATH_RAY_PERIODIC = 63883, -- :234
    SPELL_DEATH_RAY_DAMAGE_VISUAL = 63886, -- :235
    SPELL_TELEPORT_BACK_TO_MAIN_ROOM = 63992, -- :272
    SPELL_CANCEL_ILLUSION_ROOM_AURA = 63993, -- loader comment :2645 (no in-tree enum/cast site)
    SPELL_REVEALED_TENTACLE = 64012, -- loader comment :2706 (no in-tree enum/cast site)
    SPELL_GRIM_REPRISAL = 63305, -- :247
    SPELL_GRIM_REPRISAL_DAMAGE = 64039, -- :248
    SPELL_INSANE_VISUAL = 64464, -- :273
    WHISPER_VOICE_INSANE = 1, -- :65
    NPC_INFLUENCE_TENTACLE = 33943, -- ulduar.h:219
    INSANE_PERIODIC = 64555, -- loader comment :2940 (64554 SPELL_INSANE_PERIODIC is the Yogg-cast aura, a different spell)
    LUNATIC_GAZE_DAMAGE = 64164, -- :160
    LUNATIC_GAZE_DAMAGE_2 = 64168, -- loader comment :2981 (no in-tree enum name; Load :2992 stacks 2)
    HATE_TO_ZERO = 63984, -- :271
    TITANIC_STORM_SCRIPT = 64172, -- loader comment :3111 (64171 SPELL_TITANIC_STORM is the Thorim-cast spell, a different spell)
}
local yoggPhase = {} -- GUID -> PHASE_THREE once yoggDoAction fires

local REACT_PASSIVE = 0 -- UnitDefines.h:340

local function yoggInstanceData(creature)
    local mapObj = creature:GetMap()
    if not mapObj then
        return nil
    end
    return mapObj:GetInstanceData()
end

-- C++ JustEngagedWith (boss_saraAI, ScriptedAI):
-- Talk(SAY_SARA_AGGRO) + three ScheduleEvent legs — only
-- the Talk arm is bridgeable; the auriaya engage-port
-- precedent.
local function saraEnterCombat(event, creature, target)
    creature:SetMeleeAttack(false) -- boss_yogg_saron.cpp:792: custom UpdateAI never calls DoMeleeAttackIfReady (cast-only in C++)
    creature:Talk(SAY_SARA_AGGRO)
end

-- C++ boss_saraAI::KilledUnit:
-- if (victim->GetTypeId() == TYPEID_PLAYER && !me->IsInEvadeMode())
--     Talk(SAY_SARA_KILL) — both gates bridgeable.
local function saraKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" and not creature:IsInEvadeMode() then
        creature:Talk(SAY_SARA_KILL)
    end
end

-- C++ boss_yogg_saronAI::EnterEvadeMode (:478-490) + JustDied (:973-981):
-- player-loop RemoveAurasDueToSpell(SPELL_SANITY) /
-- RemoveAurasDueToSpell(SPELL_INSANE), ungated in both (no alive /
-- distance gate in C++). Called from ev23 (EnterEvadeMode; ev23 also
-- pre-fires on the Go death path, covering JustDied) and ev4
-- (JustDied, belt-and-braces — the removal is idempotent). The
-- respawn-path ev23 over-fire is a no-op: players never carry these
-- auras outside the fight. The PHASE_ONE-gated
-- Talk(WHISPER_VOICE_PHASE_1_WIPE, player) has no phase bridge and
-- stays documented-only.
local function clearSanityAuras(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId then
            p:RemoveAura(SPELL_SANITY)
            p:RemoveAura(SPELL_INSANE)
        end
    end
end

-- C++ JustDied (boss_yogg_saronAI, PassiveAI):
-- Talk(SAY_YOGG_SARON_DEATH), then Unit::Kill on the voice
-- (boss_yogg_saron.cpp:962-963 — the voice's own JustDied then
-- writes _JustDied() DONE, PORTED tick 259 in voiceDied below),
-- DisappearAndDie(sara, brain) (:965-967 — no bridge, stays out),
-- keeper EnterEvadeMode loop (:968-972 — PORTED 2026-11-07 tick
-- 390 via the instanceData keeper latch below), and player
-- sanity-aura removal; the sjonnir JustDied-Talk precedent.
local function yoggSaronDied(event, creature, killer)
    creature:Talk(SAY_YOGG_SARON_DEATH)
    local data = yoggInstanceData(creature)
    if data and data.voice then
        data.voice:Kill(creature) -- C++ Unit::Kill(me, voice): Yogg is the killer (bridge takes the killer creature as arg 1)
    end
    -- C++ :968-972 keeper-evade loop, DATA_FREYA_YS..DATA_MIMIRON_YS
    -- order; the keepers ride the Lua-side instanceData latch (temp
    -- summons bypass the Go instance funnel — see the gossip
    -- handler). EnterEvadeMode no-ops on dead/already-evading
    -- keepers, matching the C++ void call on a found creature.
    if data then
        for _, keeper in ipairs({ data.freya, data.hodir, data.thorim, data.mimiron }) do
            if keeper then
                keeper:EnterEvadeMode()
            end
        end
    end
    clearSanityAuras(creature)
end

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

-- C++ boss_yogg_saronAI::Reset :931-945 — DoCast(me,
-- SPELL_SHADOWY_BARRIER_YOGG) + DoCast(me, SPELL_KNOCK_AWAY), both
-- non-triggered and ungated (the urom evocate-reset precedent) +
-- ScheduleEvent(EVENT_YELL_BOW_DOWN, 3s) — all PORTED below; the
-- keeper-count loot-mode legs stay out (the loot-mode bridge is live —
-- keepers instance-data is the blocker, not the bridge).
local function yoggSaronReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    -- C++ boss_yogg_saronAI::Reset(): _events.Reset + SetPhase(PHASE_TWO)
    -- (:931-932) — the Lua phase latch only tracks PHASE_THREE, so Reset
    -- clears it.
    yoggPhase[guid] = nil
    -- C++ Reset: DoCast(me, SHADOWY_BARRIER_YOGG);
    -- DoCast(me, KNOCK_AWAY) (boss_yogg_saron.cpp:931-937). Alive-gated.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_SHADOWY_BARRIER_YOGG)
        creature:CastSpell(creature, SPELL_KNOCK_AWAY)
    end
    -- C++ Reset: ScheduleEvent(EVENT_YELL_BOW_DOWN, 3s, PHASE_TWO)
    -- (:933) — Talk(SAY_YOGG_SARON_SPAWN 0) one-shot; the phase is
    -- PHASE_TWO by construction at Reset, so no gate is needed.
    schedule(guid, "bowdown", 3000, function()
        if creature:IsAlive() then
            creature:Talk(PHASE3.SAY_YOGG_SARON_SPAWN)
        end
    end)
    clearSanityAuras(creature)
end

RegisterCreatureEvent(ENTRY_SARA, 1, saraEnterCombat)
RegisterCreatureEvent(ENTRY_SARA, 3, saraKilledUnit)
-- C++ boss_saraAI::JustSummoned (:870-895): every summon
-- SetReactState(REACT_PASSIVE); NPC_DEATH_ORB -> Talk + self-cast
-- (non-triggered, C++ :877) in C++ order; NPC_DEATH_RAY -> self-cast
-- SPELL_DEATH_RAY_WARNING_VISUAL (non-triggered, C++ :890). LATENT: orbs spawn only
-- from the unported SPELL_DEATH_RAY cast (anubrekhan precedent).
local function saraJustSummoned(_, creature, summoned)
    summoned:SetReactState(0) -- REACT_PASSIVE
    if summoned:GetEntry() == ENTRY_DEATH_ORB then
        creature:Talk(SAY_SARA_DEATH_RAY)
        summoned:CastSpell(summoned, SPELL_DEATH_RAY_ORIGIN_VISUAL)
    elseif summoned:GetEntry() == ENTRY_DEATH_RAY then
        -- C++ boss_saraAI::JustSummoned (:889-890): non-triggered self-cast.
        summoned:CastSpell(summoned, SPELL_DEATH_RAY_WARNING_VISUAL)
    end
end
RegisterCreatureEvent(ENTRY_SARA, 19, saraJustSummoned)
-- C++ SpellHit :951-956 — Val'anyr: SPELL_IN_THE_MAWS_OF_THE_OLD_GOD
-- (64184) hit -> AddLootMode(32). Pure loot-mode leg, no Talk arm.
local function yoggSaronSpellHit(event, creature, caster, spellId)
    if spellId == SPELL_IN_THE_MAWS_OF_THE_OLD_GOD then
        creature:AddLootMode(32)
    end
end

RegisterCreatureEvent(ENTRY_YOGG_SARON, 4, yoggSaronDied)
RegisterCreatureEvent(ENTRY_YOGG_SARON, 14, yoggSaronSpellHit)
RegisterCreatureEvent(ENTRY_YOGG_SARON, 23, yoggSaronReset)

-- Timer-driven cast arms (the maiden CreateLuaEvent convention —
-- GUID-keyed timers table, cancel on events 2/4/23;
-- CastSpell(creature, spell) = DoCastAOE/DoCastSelf self-cast,
-- CastSpell(nil, spell) = DoCastVictim victim-cast; random targets via
-- GetPlayersInWorld — the baltharus/thorim precedents). Cast-state gates
-- PORTED 2026-11-04: all four arms defer 500ms on creature:IsCasting()
-- (live, lua_creature_events.go:1950), no repeat re-arm consumed — the
-- mal_ganis idiom for the own UpdateAI pre-loop CASTING returns
-- (guardian :1208, corruptor :1277, crusher :1378 combined
-- aura-or-casting return, immortal :1507). No phase args on their schedules; schedules are Reset/event-1-seeded, never
-- DoAction-driven — no DoAction arm exists on these tentacle AIs
-- (boss_yogg_saron.cpp:1181-1536); no instance legs.
-- The shattered-illusion aura legs (sara :798, corruptor :1280 pre-Update
-- returns, crusher :1378 aura arm) are out of family scope: Lua timers
-- are wall-clock, the C++ pre-Update timer freeze is unmodeled. The blocked arms — all of Sara's
-- EVENT_SARAS_* timer casts (phase-arg PHASE_ONE + spellvalue
-- target-selector legs on no-bridge SpellScripts) — stay
-- documented-only in the header above (the gothik precedent); the
-- Yogg-Saron body phase-three arms, the voice guardian /
-- extinguish / immortal-guardian schedules and the keeper
-- ACTION_PHASE_TWO destabilization + ACTION_PHASE_THREE arms are
-- PORTED below (yoggDoAction / voiceDoAction / keeperDoAction,
-- event 38, + brainDamageTaken, event 9).
-- voice EVENT_LOCK_DOOR + JustEngagedWith one-shots, constrictor /
-- influence / ominous-cloud / brain / turned-champion / observation-ring
-- and yogg-keeper Reset one-shots, and keeper JustEnteredCombat entry-gated one-shots are ungated (not
-- DoAction-driven) and ride event 1/23 below.
local ENTRY_GUARDIAN_OF_YOGG_SARON = 33136
local ENTRY_CORRUPTOR_TENTACLE = 33985
local ENTRY_CRUSHER_TENTACLE = 33966
local ENTRY_IMMORTAL_GUARDIAN = 33988

local SPELL_DARK_VOLLEY = 63038
local SPELL_SHADOW_NOVA = 62714
local SPELL_SHADOW_NOVA_2 = 65719
local SPELL_BLACK_PLAGUE = 64153
local SPELL_CURSE_OF_DOOM = 64157
local SPELL_APATHY = 64156
local SPELL_DRAINING_POISON = 64152
local SPELL_DIMINISH_POWER = 64145
local SPELL_DIMINSH_POWER = 64148
local SPELL_CRUSH = 64146
local SPELL_TENTACLE_VOID_ZONE = 64017
local SPELL_FOCUSED_ANGER = 57688
local SPELL_ERUPT = 64144
local SPELL_EMPOWERED = 64161
local SPELL_RECENTLY_SPAWNED = 64497
local SPELL_DRAIN_LIFE = 64159

-- EVENT_ILLUSION chain (boss_yogg_saron.cpp:587-641 voice
-- EVENT_ILLUSION + ACTION_PHASE_TWO receiver, :1096-1130 brain
-- ACTION_INDUCE_MADNESS / ACTION_TENTACLE_KILLED receivers,
-- :1425-1430 influence JustDied sender, :828-834 sara
-- TRANSFORM_3 sender, :1675-1690 keeper ACTION_PHASE_TWO
-- receiver) — PORTED 2026-11-05 (tick 241). Transport is the
-- shared instanceData table (the_black_morass precedent):
-- instance_ulduar.lua binds the map-603 instance hook, which
-- installs the Map:GetInstanceData gate
-- (engine/world/instance_hooks.go) and latches the voice/sara/
-- brain objects + the illusion slot (C++ member default 0 =
-- CHAMBER) at ON_CREATURE_CREATE; the mimiron keeper object is
-- latched by the gossip summon below (the C++ KeeperGUIDs[3]
-- leg, instance_ulduar.cpp:374-377); Yogg-Saron himself is
-- stored from the voice's SummonCreature return (temp summons
-- bypass the ON_CREATURE_CREATE funnel — instance_hooks.go).
-- The brain-room GO-door legs (HandleGameObject close/open,
-- :1106-1108/:1121) stay out — no HandleGameObject bridge.
local ACTION_PHASE_TRANSFORM = 0 -- boss_yogg_saron.cpp:368
local ACTION_PHASE_TWO = 1
local ACTION_INDUCE_MADNESS = 3
local ACTION_TENTACLE_KILLED = 6
local ACTION_TOGGLE_SHATTERED_ILLUSION = 9
local PHASE_TRANSFORM = 2 -- boss_yogg_saron.cpp:286
local PHASE_TWO = 3
local SPELL_CORRUPTOR_TENTACLE_SUMMON = 64143 -- boss_yogg_saron.cpp:123
local SPELL_CONSTRICTOR_TENTACLE = 64132 -- boss_yogg_saron.cpp:121
local SPELL_CRUSHER_TENTACLE_SUMMON = 64139 -- boss_yogg_saron.cpp:122
local SPELL_INDUCE_MADNESS = 64059 -- boss_yogg_saron.cpp:166
local SPELL_SHATTERED_ILLUSION = 64173 -- boss_yogg_saron.cpp:168
local SPELL_DESTABILIZATION_MATRIX = 65206 -- boss_yogg_saron.cpp:210
local SAY_YOGG_SARON_MADNESS = 1 -- boss_yogg_saron.cpp:54
local EMOTE_YOGG_SARON_MADNESS = 2 -- boss_yogg_saron.cpp:55
local EMOTE_BRAIN_ILLUSION_SHATTERED = 0 -- boss_yogg_saron.cpp:68
local CHAMBER_ILLUSION = 0 -- ulduar.h:508
local ICECROWN_ILLUSION = 1 -- ulduar.h:509
local CREATURE_GROUP_PORTALS_10 = 1 -- boss_yogg_saron.cpp:381
local CREATURE_GROUP_PORTALS_25 = 2 -- boss_yogg_saron.cpp:382
local YOGG_SARON_SPAWN_X = 1980.43 -- boss_yogg_saron.cpp:384
local YOGG_SARON_SPAWN_Y = -25.7708
local YOGG_SARON_SPAWN_Z = 324.9724
local YOGG_SARON_SPAWN_O = 3.141593

local voicePhase = {}
local voiceShattered = {}
local voiceYogg = {}
local brainTentaclesKilled = {}
local brainSummons = {}
local keeperPhase = {}
local voiceGuardians = {} -- GUID -> { count, timer }, voice guardian arm

-- C++ boss_yogg_saronAI::UpdateAI phase-3 arms, seeded by yoggDoAction
-- below: EVENT_SHADOW_BEACON (:996-1000, DoCastAOE 64465 + Talk EMOTE 7,
-- 45s repeat), EVENT_LUNATIC_GAZE (:1002-1005, DoCast 64163, 12s repeat;
-- the SendSound leg stays out — no SendSound bridge) and
-- EVENT_DEAFENING_ROAR (:1006-1011, DoCastAOE 64189 + Talk SAY 4 +
-- Talk EMOTE 5, {20s,25s} repeat). One generic tick: Yogg UpdateAI has
-- the CASTING pre-loop return (:984-985), so all three carry the 500ms
-- IsCasting defer (mal_ganis idiom); the phase gate is the Lua yoggPhase
-- latch; talk1/talk2 are optional Talk ids, delay the re-arm (number or
-- {min,max} table).
local function yoggPhase3Tick(creature, guid, key, spellId, talk1, talk2, delay)
    if not creature:IsAlive() or yoggPhase[guid] ~= PHASE_THREE then
        return
    end
    if creature:IsCasting() then
        schedule(guid, key, 500, function()
            yoggPhase3Tick(creature, guid, key, spellId, talk1, talk2, delay)
        end)
        return
    end
    creature:CastSpell(creature, spellId)
    if talk1 then
        creature:Talk(talk1)
    end
    if talk2 then
        creature:Talk(talk2)
    end
    schedule(guid, key, delay, function()
        yoggPhase3Tick(creature, guid, key, spellId, talk1, talk2, delay)
    end)
end

-- C++ boss_yogg_saronAI::DoAction(ACTION_PHASE_THREE) (:1020-1035),
-- the ev38 receiver. C++ order: SetPhase, seed the three timer arms
-- (deafening roar only on 25-man with <4 keepers, :1026), Talk
-- SAY_PHASE_3, DoCast PHASE_3_TRANSFORM, RemoveAurasDueToSpell
-- SHADOWY_BARRIER_YOGG (RemoveAura twin — the spell id is the whole
-- leg), ResetPlayerDamageReq. The sender is the brain DamageTaken
-- cascade (brainDamageTaken below).
local function yoggDoAction(event, creature, action)
    if action ~= ACTION_PHASE_THREE then
        return
    end
    local guid = creature:GetGUIDLow()
    yoggPhase[guid] = PHASE_THREE
    schedule(guid, "shadowbeacon", 45000, function()
        yoggPhase3Tick(creature, guid, "shadowbeacon", PHASE3.SPELL_SHADOW_BEACON, PHASE3.EMOTE_YOGG_SARON_EMPOWERING_SHADOWS, nil, 45000)
    end)
    schedule(guid, "lunaticgaze", 12000, function()
        yoggPhase3Tick(creature, guid, "lunaticgaze", PHASE3.SPELL_LUNATIC_GAZE, nil, nil, 12000)
    end)
    local data = yoggInstanceData(creature)
    if creature:Is25ManRaid() and (data and data.keepersCount or 0) < 4 then
        schedule(guid, "deafeningroar", {20000, 25000}, function()
            yoggPhase3Tick(creature, guid, "deafeningroar", PHASE3.SPELL_DEAFENING_ROAR, PHASE3.SAY_YOGG_SARON_DEAFENING_ROAR, PHASE3.EMOTE_YOGG_SARON_DEAFENING_ROAR, {20000, 25000})
        end)
    end
    creature:Talk(PHASE3.SAY_YOGG_SARON_PHASE_3)
    creature:CastSpell(creature, PHASE3.SPELL_PHASE_3_TRANSFORM)
    creature:RemoveAura(SPELL_SHADOWY_BARRIER_YOGG)
    creature:ResetPlayerDamageReq()
end

local function yoggCancelAll(event, creature)
    cancelTimers(creature:GetGUIDLow())
    yoggPhase[creature:GetGUIDLow()] = nil
end

RegisterCreatureEvent(ENTRY_YOGG_SARON, 2, yoggCancelAll)
RegisterCreatureEvent(ENTRY_YOGG_SARON, 38, yoggDoAction)

-- C++ SelectTarget(Random) draws the threat list (UnitAI.h), never the
-- whole instance player pool — GetAITargets is the bridged threat list
-- (midnight precedent); SelectTargetList skips offline refs
-- (UnitAI.h), so the dead entries Go GetAITargets still carries
-- are filtered here.
local function randomThreatTarget(creature)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if not t:IsDead() then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ npc_guardian_of_yogg_saronAI::EVENT_DARK_VOLLEY:
-- DoCastAOE(SPELL_DARK_VOLLEY 63038) (non-triggered), {10s,15s} init,
-- {10s,15s} repeat — scheduled in Reset with no phase arg (:1198,
-- :1215-1218 verified).
local function darkVolleyTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "darkvolley", 500, function()
            darkVolleyTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_DARK_VOLLEY)
    schedule(guid, "darkvolley", {10000, 15000}, function()
        darkVolleyTick(creature, guid)
    end)
end

local function guardianEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "darkvolley", {10000, 15000}, function()
        darkVolleyTick(creature, guid)
    end)
end

-- C++ npc_corruptor_tentacleAI::EVENT_CAST_RANDOM_SPELL: random-player
-- DoCast(target, RAND(SPELL_BLACK_PLAGUE 64153, SPELL_CURSE_OF_DOOM
-- 64157, SPELL_APATHY 64156, SPELL_DRAINING_POISON 64152))
-- (non-triggered), 1ms init, 3s repeat — scheduled in Reset with no phase
-- arg; SelectTarget(Random) draws the threat list (GetAITargets bridge),
-- not the map-wide player pool (:1264, :1284-1288 verified).
-- The Reset-time DoCast(me, SPELL_TENTACLE_VOID_ZONE) + DoCastAOE(SPELL_ERUPT)
-- legs are ported on event 23 (corruptorReset).
local function corruptorTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "randomspell", 500, function()
            corruptorTick(creature, guid)
        end)
        return
    end
    local target = randomThreatTarget(creature)
    if target then
        local spells = { SPELL_BLACK_PLAGUE, SPELL_CURSE_OF_DOOM, SPELL_APATHY, SPELL_DRAINING_POISON }
        creature:CastSpell(target, spells[math.random(#spells)])
    end
    schedule(guid, "randomspell", 3000, function()
        corruptorTick(creature, guid)
    end)
end

local function corruptorEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "randomspell", 1, function()
        corruptorTick(creature, guid)
    end)
end

-- C++ npc_corruptor_tentacleAI::Reset (:1260-1265): non-triggered
-- one-shots DoCast(me, SPELL_TENTACLE_VOID_ZONE 64017) +
-- DoCastAOE(SPELL_ERUPT 64144) ride event 23.
local function corruptorReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    -- C++ npc_corruptor_tentacleAI ctor (:1257): SetCombatMovement(false).
    creature:SetCombatMovement(false)
    -- C++ npc_corruptor_tentacleAI::Reset(): DoCast(me, TENTACLE_VOID_ZONE);
    -- DoCastAOE(ERUPT) (boss_yogg_saron.cpp). Alive-gated; cancelTimers
    -- rides the death leg deliberately.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_TENTACLE_VOID_ZONE)
        creature:CastSpell(creature, SPELL_ERUPT)
    end
end

-- C++ npc_crusher_tentacleAI::EVENT_DIMINISH_POWER:
-- DoCast(SPELL_DIMINISH_POWER 64145) (non-triggered self-cast), {6s,8s}
-- init, {20s,30s} repeat — scheduled in Reset with no phase arg (:1370,
-- :1389-1392 verified). (SPELL_DIMINSH_POWER 64148 is a different enum
-- entry — the typo'd name — cast once in Reset, see crusherReset.)
local function diminishPowerTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "diminishpower", 500, function()
            diminishPowerTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_DIMINISH_POWER)
    schedule(guid, "diminishpower", {20000, 30000}, function()
        diminishPowerTick(creature, guid)
    end)
end

local function crusherEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "diminishpower", {6000, 8000}, function()
        diminishPowerTick(creature, guid)
    end)
end

-- C++ npc_crusher_tentacleAI::Reset (:1362-1368): five non-triggered
-- one-shots — DoCast(me, SPELL_CRUSH 64146), DoCast(me,
-- SPELL_TENTACLE_VOID_ZONE 64017), DoCast(me, SPELL_DIMINSH_POWER 64148),
-- DoCast(me, SPELL_FOCUSED_ANGER 57688), DoCastAOE(SPELL_ERUPT 64144) —
-- ride event 23 (halion Reset one-shot precedent); the SetCombatMovement
-- (false) ctor arm (:1359) rides the same handler below.
local function crusherReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    -- C++ npc_crusher_tentacleAI ctor (:1359): SetCombatMovement(false).
    creature:SetCombatMovement(false)
    -- C++ npc_crusher_tentacleAI::Reset(): DoCast(me, CRUSH/TENTACLE_VOID_ZONE/
    -- DIMINSH_POWER/FOCUSED_ANGER); DoCastAOE(ERUPT) (boss_yogg_saron.cpp).
    -- Alive-gated; cancelTimers rides the death leg deliberately.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_CRUSH)
        creature:CastSpell(creature, SPELL_TENTACLE_VOID_ZONE)
        creature:CastSpell(creature, SPELL_DIMINSH_POWER)
        creature:CastSpell(creature, SPELL_FOCUSED_ANGER)
        creature:CastSpell(creature, SPELL_ERUPT)
    end
end

-- C++ npc_immortal_guardianAI::EVENT_DRAIN_LIFE:
-- DoCast(SPELL_DRAIN_LIFE 64159) (non-triggered self-cast), {3s,13s} init,
-- {20s,30s} repeat — scheduled in Reset with no phase arg (:1491,
-- :1514-1517 verified).
local function drainLifeTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "drainlife", 500, function()
            drainLifeTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_DRAIN_LIFE)
    schedule(guid, "drainlife", {20000, 30000}, function()
        drainLifeTick(creature, guid)
    end)
end

local function immortalEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "drainlife", {3000, 13000}, function()
        drainLifeTick(creature, guid)
    end)
end

-- C++ npc_immortal_guardianAI::Reset (:1489-1493): non-triggered
-- one-shots DoCast(me, SPELL_EMPOWERED 64161) + DoCast(me,
-- SPELL_RECENTLY_SPAWNED 64497) ride event 23; the DamageTaken 1%
-- floor is ported on event 9 below (the old "no health bridge"
-- claim was stale — creature:GetHealth/GetMaxHealth are bridged,
-- object.go fallback, sindragosa precedent).
local function immortalReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    -- C++ npc_immortal_guardianAI::Reset(): DoCast(me, EMPOWERED);
    -- DoCast(me, RECENTLY_SPAWNED) (boss_yogg_saron.cpp). Alive-gated;
    -- cancelTimers rides the death leg deliberately.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_EMPOWERED)
        creature:CastSpell(creature, SPELL_RECENTLY_SPAWNED)
    end
end

-- C++ npc_immortal_guardianAI::DamageTaken (:1494-1499): the 1% floor —
-- if (HealthBelowPctDamaged(1, damage)) damage = GetHealth() -
-- CountPctFromMaxHealth(1). PORTED 2026-10-17 via event 9's second-return
-- damage rewrite (volazj event-9 precedent; midnight/curator/nightbane
-- "return false, newDamage" convention): HealthBelowPctDamaged is the
-- int64 compare (int64(health)-int64(damage) < int64(1% of max),
-- Unit.h:873); CountPctFromMaxHealth(1) = math.floor(maxHealth/100)
-- per the nightbane CalculatePct convention. The C++ uint32 wrap when
-- health < 1% is unreachable — the floor pins health at exactly 1%.
local function immortalDamageTaken(event, creature, attacker, damage)
    local health = creature:GetHealth()
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return false, damage
    end
    local newDamage = damage
    local onePct = math.floor(maxHealth / 100)
    if health - damage < onePct then
        newDamage = health - onePct
    end
    return false, newDamage
end

RegisterCreatureEvent(ENTRY_IMMORTAL_GUARDIAN, 9, immortalDamageTaken)

-- C++ spell_yogg_saron_empowered::OnPeriodic
-- (boss_yogg_saron.cpp:2524-2540): the immortal guardian's
-- empowered aura (64161, self-cast in immortalReset above)
-- re-stacks SPELL_EMPOWERED_BUFF (65294) each tick from current
-- health: stack = ceil(healthPct / 10) - 1; the old buff is
-- stripped, then re-cast triggered with SPELLVALUE_AURA_STACK =
-- stack (and SPELL_WEAKENED 64162 stripped first). CastCustomSpell
-- sets base points, not aura stacks (lua_creature_events.go:887),
-- so the stack lands via a second step: cast, then
-- target:GetAura(65294):SetStackAmount(stack) (aura SetStackAmount
-- is bridged, lua_aura.go; the nil guard covers a non-instant
-- buff cast applying off-tick). stack > 0 matches C++ `if (stack)`
-- for every live target (stack is -1 only at 0 health, where a
-- negative stack count has no Lua expression). No
-- PreventDefaultAction in C++, so the handler returns nothing.
-- PORTED tick 189 via RegisterAuraEvent (64161, AuraEventOnPeriodic
-- = 1). The sibling OnApply (buff with stack 9, :2516-2521) is
-- PORTED tick 438 via ev2 (empoweredApply below): same cast-then-
-- SetStackAmount idiom as the tick leg.
local SPELL_EMPOWERED_BUFF = 65294 -- boss_yogg_saron.cpp:195
local SPELL_WEAKENED = 64162 -- boss_yogg_saron.cpp:196

local function empoweredTick(event, aura, target)
    local stack = math.ceil((target:GetHealthPct() / 10) - 1)
    target:RemoveAura(SPELL_EMPOWERED_BUFF)
    if stack > 0 then
        target:RemoveAura(SPELL_WEAKENED)
        target:CastSpell(target, SPELL_EMPOWERED_BUFF, true)
        local buff = target:GetAura(SPELL_EMPOWERED_BUFF)
        if buff then
            buff:SetStackAmount(stack)
        end
    elseif not target:HealthAbovePct(1) and not target:HasAura(SPELL_WEAKENED) then
        target:CastSpell(target, SPELL_WEAKENED, true)
    end
end

RegisterAuraEvent(SPELL_EMPOWERED, 1, empoweredTick)

-- C++ spell_yogg_saron_empowered::OnApply (boss_yogg_saron.cpp:2516-2521):
-- AfterEffectApply, EFFECT_0 PERIODIC_DUMMY, REAL: the target casts
-- SPELL_EMPOWERED_BUFF (65294) on itself, triggered, with
-- SPELLVALUE_AURA_STACK = 9. Same cast-then-SetStackAmount idiom as
-- the tick leg above (CastCustomSpell sets base points, not stacks).
local function empoweredApply(event, aura, target)
    target:CastSpell(target, SPELL_EMPOWERED_BUFF, true)
    local buff = target:GetAura(SPELL_EMPOWERED_BUFF)
    if buff then
        buff:SetStackAmount(9)
    end
end

RegisterAuraEvent(SPELL_EMPOWERED, 2, empoweredApply)

local function cancelAll(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ boss_saraAI::DamageTaken (:719-740): lethal damage is NOT a death
-- rewrite — damage = health - 1 (survive with 1 hp), unconditionally (the
-- rewrite sits outside the PHASE_ONE gate); in PHASE_ONE only, the
-- lethal hit also fires Talk(SAY_SARA_TRANSFORM_1 6) and schedules the
-- one-shot transform chain (:732-735, 4700/9500/14300/14500ms). The
-- chain's cross-creature legs — voice DoAction(ACTION_PHASE_TRANSFORM)
-- on the lethal hit (:728-729) and voice/mimiron DoAction(
-- ACTION_PHASE_TWO) at TRANSFORM_3 (:831-833) — are PORTED below via
-- the shared instanceData table (dispatch IS bridged — ev38 — and
-- both receivers are ported in this file); the
-- SPELL_RIDE_YOGG_SARON_VEHICLE cast on Yogg at TRANSFORM_4 stays
-- out (no Yogg object in sara's scope — the voice-stored twin does
-- not apply here); the phase-2 schedule legs (EVENT_DEATH_RAY/
-- MALADY/PSYCHOSIS/BRAIN_LINK, :842-845) ride the unported phase
-- machine and stay out. DamageTaken arms fire on lethal hits, so no
-- postDamage>0 guard (immortal precedent). The latch keeps the
-- one-shot chain first-lethal-only: C++ re-arms nothing once the
-- phase has left PHASE_ONE.
local SAY_SARA_TRANSFORM_1 = 6
local SAY_SARA_TRANSFORM_2 = 7
local SAY_SARA_TRANSFORM_3 = 8
local SAY_SARA_TRANSFORM_4 = 9
local SPELL_FULL_HEAL = 43978
local SPELL_PHASE_2_TRANSFORM = 65157
local SPELL_SHADOWY_BARRIER_SARA = 64775
local FACTION_FRIENDLY = 35
local FACTION_MONSTER_2 = 16
local saraTransformed = {}

local function saraDamageTaken(event, creature, attacker, damage)
    local health = creature:GetHealth()
    local newDamage = damage
    if health > 0 and damage >= health then
        newDamage = health - 1
        local guid = creature:GetGUIDLow()
        if not saraTransformed[guid] then
            saraTransformed[guid] = true
            -- C++ DamageTaken (:728-729): voice DoAction(
            -- ACTION_PHASE_TRANSFORM) BEFORE the Talk, in C++ order.
            local data = yoggInstanceData(creature)
            local voice = data and data.voice
            if voice then
                voice:DoAction(ACTION_PHASE_TRANSFORM)
            end
            creature:Talk(SAY_SARA_TRANSFORM_1)
            schedule(guid, "saratransform1", 4700, function()
                if creature:IsAlive() then
                    creature:Talk(SAY_SARA_TRANSFORM_2)
                end
            end)
            schedule(guid, "saratransform2", 9500, function()
                if creature:IsAlive() then
                    creature:Talk(SAY_SARA_TRANSFORM_3)
                end
            end)
            schedule(guid, "saratransform3", 14300, function()
                if creature:IsAlive() then
                    creature:Talk(SAY_SARA_TRANSFORM_4)
                    creature:CastSpell(creature, SPELL_FULL_HEAL)
                    creature:SetFaction(FACTION_MONSTER_2)
                    -- C++ EVENT_TRANSFORM_3 (:831-833): voice then
                    -- mimiron DoAction(ACTION_PHASE_TWO), in C++
                    -- order, after the heal/faction legs.
                    local data = yoggInstanceData(creature)
                    if data then
                        if data.voice then
                            data.voice:DoAction(ACTION_PHASE_TWO)
                        end
                        if data.mimiron then
                            data.mimiron:DoAction(ACTION_PHASE_TWO)
                        end
                    end
                end
            end)
            schedule(guid, "saratransform4", 14500, function()
                if creature:IsAlive() then
                    creature:CastSpell(creature, SPELL_PHASE_2_TRANSFORM)
                    creature:CastSpell(creature, SPELL_SHADOWY_BARRIER_SARA)
                end
            end)
        end
    end
    return false, newDamage
end

local function saraCancelTransform(event, creature)
    cancelTimers(creature:GetGUIDLow())
    saraTransformed[creature:GetGUIDLow()] = nil
end

-- C++ boss_saraAI::Reset (:783-790): RemoveAllAuras + SetReactState
-- (REACT_PASSIVE) + SetFaction(FACTION_FRIENDLY), all bridged; the
-- _events Reset/SetPhase(PHASE_ONE) legs ride the unported phase
-- machine. Reset also cancels the transform chain (C++ _events.Reset).
local function saraReset(event, creature)
    saraCancelTransform(event, creature)
    creature:RemoveAllAuras()
    creature:SetReactState(0) -- REACT_PASSIVE
    creature:SetFaction(FACTION_FRIENDLY)
end

RegisterCreatureEvent(ENTRY_SARA, 9, saraDamageTaken)
RegisterCreatureEvent(ENTRY_SARA, 2, saraCancelTransform)
RegisterCreatureEvent(ENTRY_SARA, 4, saraCancelTransform)
RegisterCreatureEvent(ENTRY_SARA, 23, saraReset)

-- C++ npc_guardian_of_yogg_saronAI::JustDied (:1190-1194): non-triggered
-- DoCastAOE(SPELL_SHADOW_NOVA 62714) + DoCastAOE(SPELL_SHADOW_NOVA_2
-- 65719) — self-cast form per the darkVolleyTick DoCastAOE convention.
local function guardianDied(event, creature, killer)
    creature:CastSpell(creature, SPELL_SHADOW_NOVA)
    creature:CastSpell(creature, SPELL_SHADOW_NOVA_2)
    cancelTimers(creature:GetGUIDLow())
end

RegisterCreatureEvent(ENTRY_GUARDIAN_OF_YOGG_SARON, 1, guardianEnterCombat)
RegisterCreatureEvent(ENTRY_GUARDIAN_OF_YOGG_SARON, 2, cancelAll)
RegisterCreatureEvent(ENTRY_GUARDIAN_OF_YOGG_SARON, 4, guardianDied)
RegisterCreatureEvent(ENTRY_GUARDIAN_OF_YOGG_SARON, 23, cancelAll)
RegisterCreatureEvent(ENTRY_CORRUPTOR_TENTACLE, 1, corruptorEnterCombat)
RegisterCreatureEvent(ENTRY_CORRUPTOR_TENTACLE, 2, cancelAll)
RegisterCreatureEvent(ENTRY_CORRUPTOR_TENTACLE, 4, cancelAll)
RegisterCreatureEvent(ENTRY_CORRUPTOR_TENTACLE, 23, corruptorReset)
RegisterCreatureEvent(ENTRY_CRUSHER_TENTACLE, 1, crusherEnterCombat)
RegisterCreatureEvent(ENTRY_CRUSHER_TENTACLE, 2, cancelAll)
RegisterCreatureEvent(ENTRY_CRUSHER_TENTACLE, 4, cancelAll)
RegisterCreatureEvent(ENTRY_CRUSHER_TENTACLE, 23, crusherReset)
RegisterCreatureEvent(ENTRY_IMMORTAL_GUARDIAN, 1, immortalEnterCombat)
RegisterCreatureEvent(ENTRY_IMMORTAL_GUARDIAN, 2, cancelAll)
RegisterCreatureEvent(ENTRY_IMMORTAL_GUARDIAN, 4, cancelAll)
RegisterCreatureEvent(ENTRY_IMMORTAL_GUARDIAN, 23, immortalReset)

-- ===== Slice-E ungated one-shot ports (2026-10-15) =====
-- All arms below are ungated in C++: no phase args on their schedules; the
-- ported one-shots are not DoAction-driven (sibling DoAction arms stay out on
-- unported senders — influence JustDied :1428, cloud voice-Reset :506, champion
-- illusions :1799); no instance legs — the stale "no-timer bridge" / "no random-target
-- bridge" / DoCastAOE doctrines retracted under the
-- CreateLuaEvent / DoCastAOE≡self-cast conventions (the gothik
-- phase-gating precedent).

local ENTRY_VOICE_OF_YOGG_SARON = 33280
local ENTRY_CONSTRICTOR_TENTACLE = 33983
local ENTRY_INFLUENCE_TENTACLE = 33943
local ENTRY_SUIT_OF_ARMOR = 33433
local ENTRY_OMINOUS_CLOUD = 33292
local ENTRY_BRAIN_OF_YOGG_SARON = 33890
local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000
local ENTRY_TURNED_CHAMPION = 33962
local ENTRY_OBSERVATION_RING_KEEPER_HODIR = 33213
local ENTRY_OBSERVATION_RING_KEEPER_FREYA = 33241
local ENTRY_OBSERVATION_RING_KEEPER_THORIM = 33242
local ENTRY_OBSERVATION_RING_KEEPER_MIMIRON = 33244
local ENTRY_KEEPER_FREYA = 33410
local ENTRY_SANITY_WELL = 33991 -- NPC_SANITY_WELL (ulduar.h:225)
local SPELL_SANITY_WELL = 64169 -- boss_yogg_saron.cpp:218
local SPELL_SANITY_WELL_VISUAL = 63288 -- boss_yogg_saron.cpp:217
local ENTRY_KEEPER_HODIR = 33411
local ENTRY_KEEPER_MIMIRON = 33412
local ENTRY_KEEPER_THORIM = 33413
local SPELL_KEEPER_TELEPORT = 62940
local SAY_KEEPER_CHOSEN_1 = 0
local SAY_KEEPER_CHOSEN_2 = 1
local UNIT_NPC_FLAGS_FIELD = 82
local UNIT_NPC_FLAG_GOSSIP = 1

local SPELL_SANITY_PERIODIC = 63786
local SPELL_SUMMON_GUARDIAN_2 = 62978
local SPELL_INSANE_PERIODIC = 64554
local SPELL_TENTACLE_VOID_ZONE_2 = 64384
local SPELL_NONDESCRIPT_1 = 64013
local SPELL_NONDESCRIPT_2 = 64010
local SPELL_OMINOUS_CLOUD_VISUAL = 63084
local SPELL_MATCH_HEALTH = 64066
local SPELL_VERTEX_COLOR_BLACK = 39662
local SPELL_SIMPLE_TELEPORT_KEEPERS = 12980
local SPELL_KEEPER_ACTIVE = 62647
local SPELL_RESILIENCE_OF_NATURE = 62670
local SPELL_SANITY_WELL_SUMMON = 64170
local SPELL_FORTITUDE_OF_FROST = 62650
local SPELL_HODIRS_PROTECTIVE_GAZE = 64174
local SPELL_FURY_OF_THE_STORM = 62702
local SPELL_SPEED_OF_INVENTION = 62671

-- C++ spell_yogg_saron_sanity::DummyTick
-- (boss_yogg_saron.cpp:2863-2871): on each tick of the sanity aura
-- (63050) on a player, standing in a sanity well (64169) restores
-- 20 stacks, and at 40 stacks or below the low-sanity screen
-- effect (63752) is (re-)cast triggered if absent. C++
-- ModStackAmount(20) is aura:SetStackAmount(aura:GetStackAmount()
-- + 20) (aura GetStackAmount/SetStackAmount bridged, lua_aura.go),
-- clamped at 100 — the cast-time stack the sanity SpellScript sets
-- via SPELLVALUE_AURA_STACK (C++ ModStackAmount clamps to the
-- aura's max stack). No PreventDefaultAction in C++, so the handler
-- returns nothing. PORTED tick 189 via RegisterAuraEvent (63050,
-- AuraEventOnPeriodic = 1). The sibling OnRemove (:2873-2880)
-- PORTED 2026-11-08 (tick 436) via RegisterAuraEvent (63050,
-- AuraEventOnRemove = 3): ENEMY_SPELL-gated (aura:GetRemoveMode,
-- SpellAuraDefines.h:66) instance DATA_DRIVE_ME_CRAZY false +
-- RemoveAurasDueToSpell BRAIN_LINK + caster casts INSANE 63120
-- triggered, C++ order. The stale "no OnRemove / no SetData
-- bridge" claim is retired: ev3 + GetRemoveMode + the shared
-- instanceData table (driveMeCrazyEligible slot) are all live. The sibling
-- SpellScript ModSanityStacks
-- (BeforeCast, SPELLVALUE_AURA_STACK 100) stays out: no spell
-- BeforeCast bridge.
local SPELL_LOW_SANITY_SCREEN_EFFECT = 63752 -- boss_yogg_saron.cpp:277

local AURA_REMOVE_BY_ENEMY_SPELL = 3 -- SpellAuraDefines.h:66

local function sanityTick(event, aura, target)
    if target:HasAura(SPELL_SANITY_WELL) then
        aura:SetStackAmount(math.min(aura:GetStackAmount() + 20, 100))
    end
    if aura:GetStackAmount() <= 40 and not target:HasAura(SPELL_LOW_SANITY_SCREEN_EFFECT) then
        target:CastSpell(target, SPELL_LOW_SANITY_SCREEN_EFFECT, true)
    end
end

RegisterAuraEvent(SPELL_SANITY, 1, sanityTick)

local function sanityRemove(event, aura, target)
    if aura:GetRemoveMode() ~= AURA_REMOVE_BY_ENEMY_SPELL then
        return
    end
    local data = yoggInstanceData(target)
    if data then
        data.driveMeCrazyEligible = 0
    end
    target:RemoveAura(SPELL_BRAIN_LINK)
    local caster = aura:GetCaster()
    if caster then
        caster:CastSpell(target, SPELL_INSANE, true)
    end
end

RegisterAuraEvent(SPELL_SANITY, 3, sanityRemove)

-- C++ spell_yogg_saron_target_selectors::HandleScript
-- (boss_yogg_saron.cpp:1972-1977): OnEffectHitTarget EFFECT_0
-- SPELL_EFFECT_DUMMY -> the caster casts the effect's value on the hit
-- unit, non-triggered (C++ passes no triggered arg); the DUMMY effect
-- has no Go default arm beyond the Eluna hook (boss_ai.go dummy arm),
-- so no PreventHitDefaultEffect, matching C++. Bound 63744/63745/63747
-- (Sara DoCastAOE :810-818) + 65206 (keeper DoCastAOE :1663-1667).
-- DORMANT until Spell.dbc ships: the selector casts are DBC-gated.
PHASE3.yoggTargetSelectorHandleScript = function(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local value = spell:GetEffectValue(0)
    if value ~= 0 then
        caster:CastSpell(hitTarget, value, false)
    end
end
RegisterSpellEvent(PHASE3.SARAS_ANGER, 5, PHASE3.yoggTargetSelectorHandleScript)
RegisterSpellEvent(PHASE3.SARAS_BLESSING, 5, PHASE3.yoggTargetSelectorHandleScript)
RegisterSpellEvent(PHASE3.SARAS_FERVOR, 5, PHASE3.yoggTargetSelectorHandleScript)
RegisterSpellEvent(SPELL_DESTABILIZATION_MATRIX, 5, PHASE3.yoggTargetSelectorHandleScript)

-- C++ spell_yogg_saron_psychosis::FilterTargets
-- (boss_yogg_saron.cpp:2038-2042) + spell_yogg_saron_malady_of_the_mind::FilterTargets
-- (:2070-2074): OnObjectAreaTargetSelect -> drop targets with no sanity
-- aura or sanity stacks <= 40 (HighSanityTargetSelector :2011-2020),
-- then drop targets carrying the illusion-room aura (UnitAuraCheck
-- true, SPELL_ILLUSION_ROOM). GetAura/HasAura are live on both the
-- creature and player builders. Psychosis registers eff0+eff1, malady
-- eff0/1/2 on 63830 only (C++ :2077 gate); the effectIndex gate below
-- keeps the Go ev7 fire (selecting legs only) from over-filtering
-- unregistered effects.
PHASE3.yoggHighSanityFilter = function(event, spell, effectIndex, targets)
    local id = spell:GetEntry()
    if id == PHASE3.PSYCHOSIS or id == PHASE3.PSYCHOSIS_HEROIC then
        if effectIndex ~= 0 and effectIndex ~= 1 then
            return
        end
    elseif id == PHASE3.MALADY then
        if effectIndex ~= 0 and effectIndex ~= 1 and effectIndex ~= 2 then
            return
        end
    else
        return
    end
    for _, target in ipairs(targets:GetTargets()) do
        if target == nil then
            -- nothing to keep; the list holds resolved objects only
        else
            local sanity = target:GetAura(SPELL_SANITY)
            if sanity == nil or sanity:GetStackAmount() <= 40 then
                targets:Remove(target)
            elseif target:HasAura(PHASE3.ILLUSION_ROOM) then
                targets:Remove(target)
            end
        end
    end
end
RegisterSpellEvent(PHASE3.PSYCHOSIS, 7, PHASE3.yoggHighSanityFilter)
RegisterSpellEvent(PHASE3.PSYCHOSIS_HEROIC, 7, PHASE3.yoggHighSanityFilter)
RegisterSpellEvent(PHASE3.MALADY, 7, PHASE3.yoggHighSanityFilter)

-- C++ SanityReduction::RemoveSanity (boss_yogg_saron.cpp:1999-2004):
-- the hit unit's sanity aura loses _stacks stacks, removed with
-- AURA_REMOVE_BY_ENEMY_SPELL when it hits zero; psychosis sets _stacks
-- 9 (63795) / 12 (65301) in Load (:2031-2034), malady fixes 3 in its
-- ctor (:2065), lunatic gaze sets 4 (64164) / 2 (64168) in Load
-- (:2990-2993). The Go twin is SetStackAmount for a partial reduction
-- and aura:Remove(3) at zero — the mode matters, the sanityRemove ev3
-- leg gates on it. No PreventHitDefaultEffect: SCRIPT_EFFECT has no Go
-- default arm (the 429/441 precedent).
PHASE3.yoggSanityReduction = function(event, spell, hitTarget, effectIndex)
    local id = spell:GetEntry()
    local stacks = 0
    if id == PHASE3.PSYCHOSIS then
        if effectIndex ~= 1 then
            return
        end
        stacks = 9
    elseif id == PHASE3.PSYCHOSIS_HEROIC then
        if effectIndex ~= 1 then
            return
        end
        stacks = 12
    elseif id == PHASE3.MALADY or id == PHASE3.MALADY_JUMP then
        if effectIndex ~= 2 then
            return
        end
        stacks = 3
    elseif id == PHASE3.BRAIN_LINK_DAMAGE then
        if effectIndex ~= 1 then
            return
        end
        stacks = 2
    elseif id == PHASE3.LUNATIC_GAZE_DAMAGE then
        if effectIndex ~= 1 then
            return
        end
        stacks = 4
    elseif id == PHASE3.LUNATIC_GAZE_DAMAGE_2 then
        if effectIndex ~= 1 then
            return
        end
        stacks = 2
    else
        return
    end
    if hitTarget == nil then
        return
    end
    local sanity = hitTarget:GetAura(SPELL_SANITY)
    if sanity == nil then
        return
    end
    local count = sanity:GetStackAmount()
    if count <= stacks then
        sanity:Remove(AURA_REMOVE_BY_ENEMY_SPELL)
    else
        sanity:SetStackAmount(count - stacks)
    end
end
RegisterSpellEvent(PHASE3.PSYCHOSIS, 5, PHASE3.yoggSanityReduction)
RegisterSpellEvent(PHASE3.PSYCHOSIS_HEROIC, 5, PHASE3.yoggSanityReduction)
RegisterSpellEvent(PHASE3.MALADY, 5, PHASE3.yoggSanityReduction)
RegisterSpellEvent(PHASE3.MALADY_JUMP, 5, PHASE3.yoggSanityReduction)
RegisterSpellEvent(PHASE3.BRAIN_LINK_DAMAGE, 5, PHASE3.yoggSanityReduction)

-- C++ spell_yogg_saron_lunatic_gaze::FilterTargets
-- (boss_yogg_saron.cpp:2995-2998): OnObjectAreaTargetSelect EFFECT_0 +
-- EFFECT_1 TARGET_UNIT_SRC_AREA_ENEMY -> drop targets where the caster
-- is not in the target's front PI arc (LunaticGazeTargetSelector
-- :2968-2980, HasInArc(M_PI, caster)); IsInBack(caster, pi) is the
-- exact twin (Object.cpp:1417-1420), the eadric radiance precedent.
-- The sanity leg rides yoggSanityReduction above (Load :2990-2993
-- stacks 4 for 64164 / 2 for 64168, EFFECT_1 SCRIPT_EFFECT).
-- DORMANT until Spell.dbc ships 64164/64168.
PHASE3.yoggLunaticGazeFilter = function(event, spell, effectIndex, targets)
    if effectIndex ~= 0 and effectIndex ~= 1 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    for _, target in ipairs(targets:GetTargets()) do
        if target ~= nil and target:IsInBack(caster, math.pi) then
            targets:Remove(target)
        end
    end
end
RegisterSpellEvent(PHASE3.LUNATIC_GAZE_DAMAGE, 7, PHASE3.yoggLunaticGazeFilter)
RegisterSpellEvent(PHASE3.LUNATIC_GAZE_DAMAGE_2, 7, PHASE3.yoggLunaticGazeFilter)
RegisterSpellEvent(PHASE3.LUNATIC_GAZE_DAMAGE, 5, PHASE3.yoggSanityReduction)
RegisterSpellEvent(PHASE3.LUNATIC_GAZE_DAMAGE_2, 5, PHASE3.yoggSanityReduction)

-- C++ spell_yogg_saron_insane_periodic::HandleScript
-- (boss_yogg_saron.cpp:2949-2955): OnEffectHitTarget EFFECT_0
-- SPELL_EFFECT_DUMMY -> the caster casts the effect's value on the hit
-- unit, triggered; the 453/460 shape with triggered=true (C++ passes
-- true here). No in-tree cast site; DBC-bound (loader comment :2940).
-- DORMANT until Spell.dbc ships 64555. Not 64554 SPELL_INSANE_PERIODIC,
-- the Yogg-cast aura handled above.
PHASE3.yoggInsanePeriodicHit = function(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local value = spell:GetEffectValue(0)
    if value ~= 0 then
        caster:CastSpell(hitTarget, value, true)
    end
end
RegisterSpellEvent(PHASE3.INSANE_PERIODIC, 5, PHASE3.yoggInsanePeriodicHit)

-- C++ spell_yogg_saron_hate_to_zero::HandleScript
-- (boss_yogg_saron.cpp:3055-3061): OnEffectHitTarget EFFECT_0
-- SPELL_EFFECT_DUMMY -> a hit unit that CanHaveThreatList zeroes the
-- caster's threat on its list (ModifyThreatByPercent -100). The Go twin
-- is hitTarget:ModifyThreatByPercent(caster, -100) on the motion
-- creature (lua_creature_events.go:2165, ScaleThreat x0.00). The gate
-- is the CanHaveThreatList twin (ThreatManager.cpp:156-180): creature
-- only, not a trigger (IsTrigger, lua_creature_events.go:2631), and
-- not summoned by a player (GetPlayerByGUID(GetOwnerGUID()) ~= nil
-- covers pets/totems/player-summoned minions; IsPet/IsTotem
-- unbridged and the minion/guardian-mask leg unexposed, so a
-- player-summoned non-minion TempSummon over-excludes, unreachable
-- in the bound ToC worm Emerge cast). Charm carve-out: C++ charm
-- never changes SUMMONEDBY (Unit.h:1147), but Go charm OVERWRITES
-- motion.OwnerGUID with the charmer (creaturemotion.go:3436), so a
-- creature-summoned unit MC'd by a player would read as
-- player-summoned; skip the exclusion while CharmerGUID ==
-- OwnerGUID (the overwritten state; uncharmed CharmerGUID is 0).
-- Residual: a player-summoned unit charmed by a creature is
-- included by both forms (C++ excludes it), unreachable here.
-- Bound 63984 (boss enum :271; Dreadscale/Acidmaw Emerge
-- DoCastAOE :770 beasts.cpp).
-- DORMANT until Spell.dbc ships 63984.
PHASE3.yoggHateToZeroHit = function(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    if hitTarget:GetObjectType() ~= "Creature" then
        return
    end
    if hitTarget:IsTrigger() then
        return
    end
    local ownerGUID = hitTarget:GetOwnerGUID()
    if ownerGUID ~= 0 and GetPlayerByGUID(ownerGUID) ~= nil
        and hitTarget:GetCharmerGUID() ~= ownerGUID then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    hitTarget:ModifyThreatByPercent(caster, -100)
end
RegisterSpellEvent(PHASE3.HATE_TO_ZERO, 5, PHASE3.yoggHateToZeroHit)

-- C++ spell_yogg_saron_titanic_storm::HandleScript
-- (boss_yogg_saron.cpp:3120-3125): OnEffectHitTarget EFFECT_0
-- SPELL_EFFECT_DUMMY -> Unit::Kill(GetCaster(), target). The Kill
-- bridges split by receiver: creature Kill kills the RECEIVER
-- (lua_creature_events.go:3228, the :413 voice precedent), player
-- Kill kills the ARG (lua_player.go:3521) — so one bare form is wrong
-- for one of the two victim types (5da47a5f's caster:Kill(hitTarget)
-- killed the caster whenever the caster was a creature). Branch on
-- hitTarget type: creature victims die via hitTarget:Kill(caster),
-- player victims via the insane self-kill idiom hitTarget:Kill
-- (hitTarget) (creditHonorableKill no-ops for killer == victim,
-- achievements.go:1802). No in-tree cast site; DBC-bound (loader
-- comment :3111). Not 64171 SPELL_TITANIC_STORM, the Thorim-cast
-- spell. DORMANT until Spell.dbc ships 64172.
PHASE3.yoggTitanicStormHit = function(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    if hitTarget:GetObjectType() == "Creature" then
        hitTarget:Kill(caster)
    else
        hitTarget:Kill(hitTarget)
    end
end
RegisterSpellEvent(PHASE3.TITANIC_STORM_SCRIPT, 5, PHASE3.yoggTitanicStormHit)

-- C++ spell_yogg_saron_brain_link::FilterTargets
-- (boss_yogg_saron.cpp:2139-2151): OnObjectAreaTargetSelect EFFECT_0
-- TARGET_UNIT_SRC_AREA_ENEMY -> drop illusion-room carriers, then
-- clear unless exactly 2 remain, then SaraAI::SetLinkBetween the two
-- survivors (:707-711, both directions). The link store rides
-- PHASE3.brainLinks (the 431 pursueTargetGuid precedent); the filter
-- idiom mirrors yoggHighSanityFilter above. Bound 63802 (boss enum
-- :139, Sara DoCastAOE :860, EVENT_BRAIN_LINK 23s, MAX_TARGETS 2).
PHASE3.yoggBrainLinkFilter = function(event, spell, effectIndex, targets)
    if effectIndex ~= 0 then
        return
    end
    for _, target in ipairs(targets:GetTargets()) do
        if target ~= nil and target:HasAura(PHASE3.ILLUSION_ROOM) then
            targets:Remove(target)
        end
    end
    if targets:GetCount() ~= 2 then
        targets:Clear()
        return
    end
    local list = targets:GetTargets()
    local a, b = list[1], list[2]
    if a == nil or b == nil then
        return
    end
    local ga, gb = a:GetGUID(), b:GetGUID()
    PHASE3.brainLinks[ga] = gb
    PHASE3.brainLinks[gb] = ga
end
RegisterSpellEvent(SPELL_BRAIN_LINK, 7, PHASE3.yoggBrainLinkFilter)

-- C++ spell_yogg_saron_brain_link_AuraScript::DummyTick
-- (boss_yogg_saron.cpp:2190-2203): OnEffectPeriodic EFFECT_0
-- SPELL_AURA_PERIODIC_DUMMY -> the linked player (SaraAI
-- GetLinkedPlayerGUID :698-705, via PHASE3.brainLinks here) takes
-- the damage beam 63803 when farther than the aura amount, else the
-- no-damage beam 63804, both triggered (player CastSpell takes no
-- triggered arg; the cast rides the ev5 funnel either way). The
-- caster gate mirrors the C++ caster/SaraAI nil return; the distance
-- is C++ WorldObject::GetDistance (Object.cpp:1169-1173): exact dist
-- minus both players' 1.5 combat reach, clamped at 0 (generic
-- GetDistance carries no reach subtraction, so subtract 3.0 here).
-- Bound 63802.
PHASE3.yoggBrainLinkTick = function(event, aura, target)
    if target == nil then
        return
    end
    if aura:GetCaster() == nil then
        return
    end
    local partnerGUID = PHASE3.brainLinks[target:GetGUID()]
    if partnerGUID == nil then
        return
    end
    local linked = GetPlayerByGUID(partnerGUID)
    if linked == nil then
        return
    end
    if math.max(0, target:GetDistance(linked) - 3.0) > aura:GetAmount(0) then
        target:CastSpell(linked, PHASE3.BRAIN_LINK_DAMAGE)
    else
        target:CastSpell(linked, PHASE3.BRAIN_LINK_NO_DAMAGE)
    end
end
RegisterAuraEvent(SPELL_BRAIN_LINK, 1, PHASE3.yoggBrainLinkTick)

-- C++ spell_yogg_saron_brain_link_AuraScript::OnRemove
-- (boss_yogg_saron.cpp:2166-2186): OnEffectRemove EFFECT_0
-- SPELL_AURA_PERIODIC_DUMMY, REAL handle -> EXPIRE unlinks only;
-- any other mode unlinks and strips BRAIN_LINK from the linked
-- player, but only when that player resolves (C++ gates both on the
-- GetPlayer hit). RemoveLinkFrom erases one direction only (:715);
-- the partner's own OnRemove erases theirs when its aura strips.
-- RemoveAurasDueToSpell has no Lua binding; player:RemoveAura is
-- the twin. Bound 63802.
PHASE3.yoggBrainLinkRemoved = function(event, aura, target)
    if target == nil then
        return
    end
    if aura:GetCaster() == nil then
        return
    end
    local guid = target:GetGUID()
    if aura:GetRemoveMode() == PHASE3.AURA_REMOVE_BY_EXPIRE then
        PHASE3.brainLinks[guid] = nil
        return
    end
    local partnerGUID = PHASE3.brainLinks[guid]
    if partnerGUID == nil then
        return
    end
    local linked = GetPlayerByGUID(partnerGUID)
    if linked == nil then
        return
    end
    PHASE3.brainLinks[guid] = nil
    linked:RemoveAura(SPELL_BRAIN_LINK)
end
RegisterAuraEvent(SPELL_BRAIN_LINK, 3, PHASE3.yoggBrainLinkRemoved)

-- C++ spell_yogg_saron_boil_ominously::HandleDummy
-- (boss_yogg_saron.cpp:2263-2271): OnEffectHitTarget EFFECT_0
-- SPELL_EFFECT_DUMMY -> if the hit unit carries no flash-freeze aura
-- and the caster carries neither summon-guardian aura, the caster
-- Talks the touch emote to the hit unit and self-casts the guardian
-- summon, triggered. No PreventHitDefaultEffect: DUMMY has no Go
-- default arm beyond the Eluna hook (the 453 precedent). Bound 63030
-- (ominous cloud, DBC-gated cast; no C++ cast site).
PHASE3.yoggBoilOminously = function(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    if hitTarget:HasAura(PHASE3.FLASH_FREEZE) or caster:HasAura(PHASE3.SUMMON_GUARDIAN_1) or caster:HasAura(SPELL_SUMMON_GUARDIAN_2) then
        return
    end
    if caster:GetObjectType() == "Creature" then
        caster:Talk(PHASE3.EMOTE_OMINOUS_CLOUD_PLAYER_TOUCH, hitTarget)
    end
    caster:CastSpell(caster, PHASE3.SUMMON_GUARDIAN_1, true)
end
RegisterSpellEvent(PHASE3.BOIL_OMINOUSLY, 5, PHASE3.yoggBoilOminously)

-- C++ spell_yogg_saron_shadow_beacon::OnApply/OnRemove
-- (boss_yogg_saron.cpp:2295-2305): AfterEffectApply/Remove EFFECT_0
-- SPELL_AURA_PERIODIC_TRIGGER_SPELL, REAL handle -> the aura target
-- (an immortal guardian creature) swaps entry to the marked guardian
-- on apply and back on remove, ungated in C++. C++ calls
-- Object::SetEntry (Object.h:85), a bare OBJECT_FIELD_ENTRY flip —
-- NOT Creature::UpdateEntry, whose full re-seat (template reload,
-- random level roll) the 668cd3fe landing wrongly used; SetEntry is
-- bridged this tick (lua_creature_events.go). The ev2/ev3 aura
-- target is the motion creature for creature owners (lua_aura.go
-- ownerObject). Bound 64465 (Yogg DoCastAOE, already ported as a
-- cast in yoggPhase3Tick).
PHASE3.yoggShadowBeaconApplied = function(event, aura, target)
    if target == nil or target:GetObjectType() ~= "Creature" then
        return
    end
    target:SetEntry(PHASE3.NPC_MARKED_IMMORTAL_GUARDIAN)
end
RegisterAuraEvent(PHASE3.SPELL_SHADOW_BEACON, 2, PHASE3.yoggShadowBeaconApplied)
PHASE3.yoggShadowBeaconRemoved = function(event, aura, target)
    if target == nil or target:GetObjectType() ~= "Creature" then
        return
    end
    target:SetEntry(PHASE3.NPC_IMMORTAL_GUARDIAN)
end
RegisterAuraEvent(PHASE3.SPELL_SHADOW_BEACON, 3, PHASE3.yoggShadowBeaconRemoved)

-- C++ spell_yogg_saron_empowering_shadows_range_check::HandleScript
-- (boss_yogg_saron.cpp:2331-2335): OnEffectHitTarget EFFECT_0
-- SPELL_EFFECT_SCRIPT_EFFECT -> the hit unit casts the effect's value
-- on the caster, triggered (the 453 target-selector shape, target as
-- caster). Bound 64466 (DBC-gated; eff0 value is the 64467 missile).
PHASE3.yoggEmpoweringShadowsRangeCheck = function(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local value = spell:GetEffectValue(0)
    if value ~= 0 then
        hitTarget:CastSpell(caster, value, true)
    end
end
RegisterSpellEvent(PHASE3.EMPOWERING_SHADOWS_RANGE_CHECK, 5, PHASE3.yoggEmpoweringShadowsRangeCheck)

-- C++ spell_yogg_saron_empowering_shadows_missile::HandleScript
-- (boss_yogg_saron.cpp:2361-2365): OnEffectHitTarget EFFECT_0
-- SPELL_EFFECT_SCRIPT_EFFECT -> the hit unit (the marked guardian)
-- casts empowering shadows, triggered, on a nil target. Go
-- CastSpell(nil) falls back to victim, so the C++ nil-target
-- self-cast is written as an explicit self-cast. Bound 64467.
PHASE3.yoggEmpoweringShadowsMissile = function(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    hitTarget:CastSpell(hitTarget, PHASE3.EMPOWERING_SHADOWS, true)
end
RegisterSpellEvent(PHASE3.EMPOWERING_SHADOWS_MISSILE, 5, PHASE3.yoggEmpoweringShadowsMissile)

-- C++ spell_yogg_saron_malady_of_the_mind_AuraScript::OnRemove
-- (boss_yogg_saron.cpp:2099-2111): AfterEffectRemove EFFECT_1
-- SPELL_AURA_MOD_FEAR -> when the fear aura leaves by enemy spell,
-- expiry, or death, the target casts the jump spell on itself,
-- non-triggered. Bound 63830 (the fear aura; 63881 is the cast, not a
-- second binding).
PHASE3.yoggMaladyRemoved = function(event, aura, target)
    local mode = aura:GetRemoveMode()
    if mode ~= AURA_REMOVE_BY_ENEMY_SPELL and mode ~= PHASE3.AURA_REMOVE_BY_EXPIRE and mode ~= PHASE3.AURA_REMOVE_BY_DEATH then
        return
    end
    if target == nil then
        return
    end
    target:CastSpell(target, PHASE3.MALADY_JUMP, false)
end
RegisterAuraEvent(PHASE3.MALADY, 3, PHASE3.yoggMaladyRemoved)

-- C++ boss_voice_of_yogg_saronAI::UpdateAI EVENT_SUMMON_GUARDIAN_OF_
-- YOGG_SARON (:567-573): DoCastAOE(SPELL_SUMMON_GUARDIAN_2,
-- {SPELLVALUE_MAX_TARGETS, 1}) — self-cast, max-targets clamp dropped
-- (voiceEnterCombat precedent) — PHASE_ONE-gated, _guardianTimer
-- 20s init; ++_guardiansCount, and while count <= 6 every 3rd
-- guardian shortens the timer by 5s. Voice UpdateAI has no CASTING
-- gate, so no IsCasting defer (unlike the tentacle ticks).
local function guardianTick(creature, guid)
    if not creature:IsAlive() or voicePhase[guid] ~= PHASE_ONE then
        return
    end
    local st = voiceGuardians[guid]
    if not st then
        st = { count = 0, timer = 20000 }
        voiceGuardians[guid] = st
    end
    creature:CastSpell(creature, SPELL_SUMMON_GUARDIAN_2)
    st.count = st.count + 1
    if st.count <= 6 and st.count % 3 == 0 then
        st.timer = st.timer - 5000
    end
    schedule(guid, "guardian", st.timer, function()
        guardianTick(creature, guid)
    end)
end

-- C++ boss_voice_of_yogg_saronAI::UpdateAI EVENT_EXTINGUISH_ALL_LIFE
-- (:559-565): yogg Talk(EMOTE_YOGG_SARON_EXTINGUISH_ALL_LIFE) +
-- yogg CastSpell(nullptr, SPELL_EXTINGUISH_ALL_LIFE, triggered),
-- 15min first fire, 10s repeat, no phase arg. The C++ nullptr target
-- has no Go form (a nil CastSpell target falls back to the victim,
-- and PassiveAI yogg has none — a no-op); the wipe spell rides the
-- self-cast DoCastAOE convention. The C++ Talk target is the voice
-- itself, so the Talk rides without a whisper target.
local function extinguishTick(creature, guid)
    local yogg = voiceYogg[guid]
    if yogg and yogg:IsAlive() then
        yogg:Talk(PHASE3.EMOTE_YOGG_SARON_EXTINGUISH_ALL_LIFE)
        yogg:CastSpell(yogg, PHASE3.SPELL_EXTINGUISH_ALL_LIFE, true)
    end
    schedule(guid, "extinguish", 10000, function()
        extinguishTick(creature, guid)
    end)
end

-- C++ boss_voice_of_yogg_saronAI::UpdateAI EVENT_SUMMON_IMMORTAL_
-- GUARDIAN (:605-608): DoCastAOE(SPELL_IMMORTAL_GUARDIAN),
-- PHASE_THREE-gated, 15s repeat — seeded 1s by voiceDoAction
-- ACTION_PHASE_THREE below. No CASTING gate on the voice UpdateAI.
local function immortalGuardianTick(creature, guid)
    if not creature:IsAlive() or voicePhase[guid] ~= PHASE_THREE then
        return
    end
    creature:CastSpell(creature, PHASE3.SPELL_IMMORTAL_GUARDIAN)
    schedule(guid, "immortal", 15000, function()
        immortalGuardianTick(creature, guid)
    end)
end

-- C++ boss_voice_of_yogg_saronAI::JustEngagedWith (:518-529):
-- DoCast(me, SPELL_SANITY_PERIODIC 63786) (2-arg self, non-triggered,
-- ungated, :523) + DoCastAOE(SPELL_SUMMON_GUARDIAN_2 62978,
-- {SPELLVALUE_MAX_TARGETS, 1}, :522) + events.ScheduleEvent(
-- EVENT_LOCK_DOOR, 15s) (no phase arg, :525). The SetInCombatWith
-- keeper machine (:518-519) rides unbridged instance legs
-- (assembly_of_iron precedent) — documented-only; the
-- EVENT_EXTINGUISH_ALL_LIFE / EVENT_SUMMON_GUARDIAN_OF_YOGG_SARON
-- schedules are PORTED below (guardianTick / extinguishTick, seeded
-- in voiceEnterCombat). DoCastAOE(62978) is a plain
-- straight-line arm (no machine state consumed), so the cast ports
-- as self-cast (DoCastAOE convention); the max-targets clamp has
-- no bridge and is dropped. Non-triggered: the SpellValueMod-only
-- CastSpellExtraArgs ctor leaves TriggerFlags at TRIGGERED_NONE
-- (SpellDefines.h:178).
local function voiceEnterCombat(event, creature, target)
    creature:SetMeleeAttack(false) -- boss_yogg_saron.cpp:539: custom UpdateAI never calls DoMeleeAttackIfReady (cast-only in C++)
    creature:SetCombatMovement(false) -- ScriptedAI ctor arm, boss_yogg_saron.cpp:452 (ranged AI stands and casts)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    -- C++ JustEngagedWith: DoCastAOE(SPELL_SUMMON_GUARDIAN_2 62978,
    -- {SPELLVALUE_MAX_TARGETS, 1}) (boss_yogg_saron.cpp:522) — plain
    -- self-cast arm (DoCastAOE convention), non-triggered; the
    -- max-targets clamp has no bridge and is dropped.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_SUMMON_GUARDIAN_2)
    end
    -- C++ JustEngagedWith: DoCast(me, SPELL_SANITY_PERIODIC)
    -- (boss_yogg_saron.cpp:523). Alive-gated.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_SANITY_PERIODIC)
    end
    -- C++ EVENT_LOCK_DOOR: one-shot 15s -> DoCast(me,
    -- SPELL_INSANE_PERIODIC) (:556-557) + instance->SetBossState(
    -- BOSS_YOGG_SARON, IN_PROGRESS) (:558, PORTED tick 259 via
    -- creature:SetBossState, lua_creature_events.go:5917).
    schedule(guid, "lockdoor", 15000, function()
        if creature:IsAlive() then
            creature:CastSpell(creature, SPELL_INSANE_PERIODIC)
            creature:SetBossState(BOSS_YOGG_SARON, 1)
        end
    end)
    -- C++ JustEngagedWith: ScheduleEvent(EVENT_SUMMON_GUARDIAN_OF_
    -- YOGG_SARON, _guardianTimer 20s, PHASE_ONE) (:526) — seeded
    -- here; the count/timer decrement lives in guardianTick below.
    schedule(guid, "guardian", 20000, function()
        guardianTick(creature, guid)
    end)
    -- C++ JustEngagedWith: ScheduleEvent(EVENT_EXTINGUISH_ALL_LIFE,
    -- 15min) (:527) — no phase arg; the yogg Talk + cast legs are
    -- nil-guarded inside extinguishTick (yogg does not exist until
    -- phase two), matching the C++ GetCreature null check.
    schedule(guid, "extinguish", 900000, function()
        extinguishTick(creature, guid)
    end)
end

-- C++ boss_voice_of_yogg_saronAI::JustSummoned (:647-675): only the
-- plain one-shot legs port — constrictor Lunge (triggered, :658),
-- descend portal visual (triggered, :667), immortal simple teleport
-- (triggered, :670), Yogg emerge emote (:655), crusher/corruptor
-- emerge emote (:660-665, passive + attack-kick legs withheld).
-- The guardian 1s and
-- crusher/corruptor 5s StartAttackEvent kicks + the crusher/corruptor
-- REACT_PASSIVE arm (:651-652, :660-665) stay out (AttackStart has no
-- bridge; passive without the kick would strand the summons).
local ENTRY_DESCEND_INTO_MADNESS = 34072 -- ulduar.h:226
local SPELL_LUNGE = 64131
local SPELL_TELEPORT_PORTAL_VISUAL = 64416
local SPELL_SIMPLE_TELEPORT = 64195
local EMOTE_ONESHOT_EMERGE = 449
local function voiceJustSummoned(event, creature, summoned)
    local entry = summoned:GetEntry()
    if entry == ENTRY_CONSTRICTOR_TENTACLE then
        summoned:CastSpell(summoned, SPELL_LUNGE, true)
    elseif entry == ENTRY_DESCEND_INTO_MADNESS then
        summoned:CastSpell(summoned, SPELL_TELEPORT_PORTAL_VISUAL, true)
    elseif entry == ENTRY_IMMORTAL_GUARDIAN then
        summoned:CastSpell(summoned, SPELL_SIMPLE_TELEPORT, true)
    elseif entry == ENTRY_CRUSHER_TENTACLE or entry == ENTRY_CORRUPTOR_TENTACLE then
        -- C++ voice JustSummoned (:660-665): emerge emote only — the
        -- REACT_PASSIVE + StartAttackEvent kick legs stay out (no
        -- AttackStart bridge; passive without the kick would strand).
        summoned:PerformEmote(EMOTE_ONESHOT_EMERGE)
    elseif entry == ENTRY_YOGG_SARON then
        summoned:PerformEmote(EMOTE_ONESHOT_EMERGE)
    end
end

-- C++ spell_yogg_saron_constrictor_tentacle::OnApply
-- (boss_yogg_saron.cpp:2393-2397): AfterEffectApply EFFECT_0
-- SPELL_AURA_DUMMY, REAL -> the aura target self-casts
-- SPELL_CONSTRICTOR_TENTACLE_SUMMON (64133), non-triggered.
-- Bound 64132 (voice DoCastAOE :580, max-targets 1). The census
-- OnEffectHitTarget hook is a phantom — the class registers only
-- AfterEffectApply. DORMANT until Spell.dbc ships.
PHASE3.yoggConstrictorTentacleApplied = function(event, aura, target)
    target:CastSpell(target, PHASE3.SPELL_CONSTRICTOR_TENTACLE_SUMMON, false)
end
RegisterAuraEvent(SPELL_CONSTRICTOR_TENTACLE, 2, PHASE3.yoggConstrictorTentacleApplied)

-- C++ spell_yogg_saron_lunge::HandleScript
-- (boss_yogg_saron.cpp:2424-2432): OnEffectHitTarget EFFECT_0
-- SPELL_EFFECT_SCRIPT_EFFECT -> the hit unit self-casts SPELL_SQUEEZE
-- (64125) triggered, then casts the effect's value on the caster,
-- triggered. No PreventHitDefaultEffect: SCRIPT_EFFECT has no Go
-- default arm beyond the Eluna hook (the 453 precedent). Bound 64131
-- (constrictor JustSummoned :658, triggered self-cast). The census
-- AfterEffectApply hook is a phantom. DORMANT until Spell.dbc ships.
PHASE3.yoggLungeHit = function(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    hitTarget:CastSpell(hitTarget, PHASE3.SPELL_SQUEEZE, true)
    local value = spell:GetEffectValue(0)
    if value ~= 0 then
        hitTarget:CastSpell(caster, value, true)
    end
end
RegisterSpellEvent(SPELL_LUNGE, 5, PHASE3.yoggLungeHit)

-- C++ spell_yogg_saron_squeeze::OnRemove
-- (boss_yogg_saron.cpp:2455-2464): AfterEffectRemove EFFECT_0
-- SPELL_AURA_PERIODIC_DAMAGE, REAL -> if the aura target rides a
-- vehicle (the constrictor tentacle) and it is alive, the tentacle
-- kills itself. GetVehicleBase has no Lua bridge; the twin is
-- IsOnVehicle + Vehicle:GetOwner (the mimiron/weldTick idiom) and
-- Kill is the KillSelf twin (lua_creature_events.go:3237). No
-- remove-mode gate in C++. Bound 64125 (lunge :2428) + 64126
-- (heroic difficulty twin, loader comment :2445). The census hooks
-- (AfterEffectApply + OnEffectHitTarget) are both phantoms — the
-- class registers only AfterEffectRemove. DORMANT until Spell.dbc ships.
PHASE3.yoggSqueezeRemoved = function(event, aura, target)
    if target == nil then
        return
    end
    local vehicle = target:IsOnVehicle()
    if vehicle == nil then
        return
    end
    local base = vehicle:GetOwner()
    if base ~= nil and base:IsAlive() then
        base:Kill()
    end
end
RegisterAuraEvent(PHASE3.SPELL_SQUEEZE, 3, PHASE3.yoggSqueezeRemoved)
RegisterAuraEvent(PHASE3.SPELL_SQUEEZE_HEROIC, 3, PHASE3.yoggSqueezeRemoved)

-- C++ spell_yogg_saron_diminsh_power::HandleProc
-- (boss_yogg_saron.cpp:2483-2492): OnEffectProc EFFECT_0
-- SPELL_AURA_PROC_TRIGGER_SPELL -> PreventDefaultAction, then if
-- the target's current channeled spell is casting, interrupt it.
-- The ev6 true-return is the PreventDefaultAction twin (it skips
-- the default trigger leg, lua_aura.go:836; oculus precedent);
-- GetCurrentSpell(2) is the live-channel nil test (Go has no
-- SPELL_STATE_DELAYED / PREPARING channel state, so a live channel
-- is a casting one) and InterruptSpell(2) is bridged on the
-- creature + player builders. Bound 64148 (crusher tentacle Reset
-- self-cast :1366). The census hooks (AfterEffectApply +
-- OnEffectHitTarget) are both phantoms — the class registers only
-- OnEffectProc. DORMANT until Spell.dbc ships.
PHASE3.yoggDiminshPowerProc = function(event, aura, target, actor, procTarget, damage)
    if target ~= nil and target:GetCurrentSpell(2) ~= nil then
        target:InterruptSpell(2)
    end
    return true
end
RegisterAuraEvent(SPELL_DIMINSH_POWER, 6, PHASE3.yoggDiminshPowerProc)

-- C++ spell_yogg_saron_match_health::HandleScript
-- (boss_yogg_saron.cpp:2563-2569): OnEffectHitTarget EFFECT_0
-- SPELL_EFFECT_SCRIPT_EFFECT -> the hit unit's health is set to
-- the caster's health pct of its own max. CountPctFromMaxHealth
-- truncates the float pct to int32 (lua_values.go), matching the
-- C++ (int32) cast. Bound 64069 (brain DamageTaken :1070-1090,
-- triggered) + 64066 (brain Reset :1065, same ID-agnostic arm;
-- 64066's DB binding to this loader is unconfirmed — no separate
-- loader exists in-tree). The census OnEffectHitTarget hook is
-- exact; no AfterEffectApply leg exists. DORMANT until Spell.dbc ships.
PHASE3.yoggMatchHealthHit = function(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    hitTarget:SetHealth(hitTarget:CountPctFromMaxHealth(caster:GetHealthPct()))
end
RegisterSpellEvent(PHASE3.SPELL_MATCH_HEALTH_2, 5, PHASE3.yoggMatchHealthHit)
RegisterSpellEvent(SPELL_MATCH_HEALTH, 5, PHASE3.yoggMatchHealthHit)

-- C++ spell_yogg_saron_shattered_illusion::HandleScript
-- (boss_yogg_saron.cpp:2592-2598): OnEffectHitTarget EFFECT_0
-- SPELL_EFFECT_SCRIPT_EFFECT -> the hit unit removes the aura
-- named by the effect value (DBC-side, presumably 64173
-- SPELL_SHATTERED_ILLUSION). Bound 65238 (brain DamageTaken
-- DoCastAOE :1070-1090, triggered). No PreventHitDefaultEffect:
-- SCRIPT_EFFECT has no Go default arm beyond the Eluna hook (the
-- 453 precedent). DORMANT until Spell.dbc ships.
PHASE3.yoggShatteredIllusionHit = function(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    local value = spell:GetEffectValue(0)
    if value ~= 0 then
        hitTarget:RemoveAura(value)
    end
end
RegisterSpellEvent(PHASE3.SPELL_SHATTERED_ILLUSION_REMOVE, 5, PHASE3.yoggShatteredIllusionHit)

-- C++ spell_yogg_saron_death_ray_warning_visual::OnRemove
-- (boss_yogg_saron.cpp:2622-2632): AfterEffectRemove EFFECT_0
-- SPELL_AURA_DUMMY, REAL -> the caster self-casts
-- SPELL_DEATH_RAY_PERIODIC (63883) triggered, casts
-- SPELL_DEATH_RAY_DAMAGE_VISUAL (63886) on a nil target
-- (explicit self-cast twin, the 461 precedent), triggered, then
-- MoveConfused (lua_creature_events.go:5237). Bound 63882 (death-ray
-- summon JustSummoned :890, non-triggered). The census
-- OnEffectHitTarget hook is a phantom — the class registers only
-- AfterEffectRemove. DORMANT until Spell.dbc ships.
PHASE3.yoggDeathRayWarningRemoved = function(event, aura, target)
    local caster = aura:GetCaster()
    if caster == nil then
        return
    end
    caster:CastSpell(caster, PHASE3.SPELL_DEATH_RAY_PERIODIC, true)
    caster:CastSpell(caster, PHASE3.SPELL_DEATH_RAY_DAMAGE_VISUAL, true)
    caster:MoveConfused()
end
RegisterAuraEvent(SPELL_DEATH_RAY_WARNING_VISUAL, 3, PHASE3.yoggDeathRayWarningRemoved)

-- C++ spell_yogg_saron_cancel_illusion_room_aura::HandleScript
-- (boss_yogg_saron.cpp:2660-2667): OnEffectHitTarget EFFECT_0
-- SPELL_EFFECT_SCRIPT_EFFECT -> the hit unit self-casts
-- SPELL_TELEPORT_BACK_TO_MAIN_ROOM (63992), non-triggered, then
-- removes the aura named by the effect value. Bound 63993 (loader
-- comment :2645, registered AddSC :3221; no in-tree cast site or
-- boss enum — the trigger lives in DBC, the 429 precedent).
-- DORMANT until Spell.dbc ships.
PHASE3.yoggCancelIllusionRoomHit = function(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    hitTarget:CastSpell(hitTarget, PHASE3.SPELL_TELEPORT_BACK_TO_MAIN_ROOM, false)
    local value = spell:GetEffectValue(0)
    if value ~= 0 then
        hitTarget:RemoveAura(value)
    end
end
RegisterSpellEvent(PHASE3.SPELL_CANCEL_ILLUSION_ROOM_AURA, 5, PHASE3.yoggCancelIllusionRoomHit)

-- C++ spell_yogg_saron_nondescript::OnRemove
-- (boss_yogg_saron.cpp:2691-2696): AfterEffectRemove EFFECT_0
-- SPELL_AURA_MOD_STUN, REAL -> the target self-casts the aura
-- effect's amount as a spell id, triggered. aura:GetAmount()
-- defaults to effect 0 (lua_aura.go:267) and the ev3 snapshot
-- carries the stored amounts (lua_aura.go:32). Bound 64010 +
-- 64013 (influence tentacle Reset :1422, entry-conditional
-- non-triggered cast; both ids served by the one loader). The
-- census OnEffectHitTarget hook is a phantom — the class registers
-- only AfterEffectRemove. DORMANT until Spell.dbc ships.
PHASE3.yoggNondescriptRemoved = function(event, aura, target)
    if target == nil then
        return
    end
    local value = aura:GetAmount()
    if value ~= 0 then
        target:CastSpell(target, value, true)
    end
end
RegisterAuraEvent(SPELL_NONDESCRIPT_1, 3, PHASE3.yoggNondescriptRemoved)
RegisterAuraEvent(SPELL_NONDESCRIPT_2, 3, PHASE3.yoggNondescriptRemoved)

-- C++ spell_yogg_saron_revealed_tentacle::HandleScript
-- (boss_yogg_saron.cpp:2721-2729): OnEffectHitTarget EFFECT_0
-- SPELL_EFFECT_DUMMY -> the caster, if a creature, self-casts
-- SPELL_TENTACLE_VOID_ZONE (64017) + SPELL_GRIM_REPRISAL (63305),
-- both triggered, then UpdateEntry(NPC_INFLUENCE_TENTACLE 33943,
-- GetCreatureData()) — the bridged seatCreatureEntry twin
-- (lua_creature_events.go:4824; the data/updateLevel args ride
-- their C++ defaults per the bridge comment). The ToCreature gate
-- is explicit: ev5 also fires on the player triggered funnel
-- (spells.go:28240), and player objects have no UpdateEntry, so a
-- player-cast 64012 would nil-call after the casts. No PreventHitDefaultEffect: DUMMY has no Go
-- default arm beyond the Eluna hook (the 453 precedent). Bound
-- 64012 (loader comment :2706; no in-tree cast site or boss enum
-- — the trigger lives in DBC, the 429 precedent). DORMANT until
-- Spell.dbc ships.
PHASE3.yoggRevealedTentacleHit = function(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetObjectType() ~= "Creature" then
        return
    end
    caster:CastSpell(caster, SPELL_TENTACLE_VOID_ZONE, true)
    caster:CastSpell(caster, PHASE3.SPELL_GRIM_REPRISAL, true)
    caster:UpdateEntry(PHASE3.NPC_INFLUENCE_TENTACLE)
end
RegisterSpellEvent(PHASE3.SPELL_REVEALED_TENTACLE, 5, PHASE3.yoggRevealedTentacleHit)

-- C++ spell_yogg_saron_grim_reprisal::HandleProc
-- (boss_yogg_saron.cpp:2757-2767): OnEffectProc EFFECT_0
-- SPELL_AURA_DUMMY -> PreventDefaultAction, then if the proc
-- damage is nonzero the target casts SPELL_GRIM_REPRISAL_DAMAGE
-- (64039) on the attacker with BP0 = CalculatePct(damage, 60)
-- (Util.h:66-70, float mult truncated). The ev6 true-return is
-- the PreventDefaultAction twin (lua_aura.go:836; the diminsh
-- precedent) and it vetoes on every path, matching the C++ call
-- order (PreventDefaultAction before the damage check); the
-- damage arg rides fireAuraProc (lua_aura.go:832) and actor is
-- the event actor, i.e. the damage dealer (procs.go:1497-1499);
-- CastSpellExtraArgs(aurEff) sets TRIGGERED_FULL_MASK
-- (SpellDefines.h:166), so the CastCustomSpell BP0 leg is
-- triggered. Bound 63305 (revealed tentacle :2725, triggered).
-- The census OnEffectHitTarget hook is a phantom — the class
-- registers only OnEffectProc. DORMANT until Spell.dbc ships.
PHASE3.yoggGrimReprisalProc = function(event, aura, target, actor, procTarget, damage)
    if target == nil or actor == nil or damage == nil or damage == 0 then
        return true
    end
    target:CastCustomSpell(actor, PHASE3.SPELL_GRIM_REPRISAL_DAMAGE, true, math.floor(damage * 60 / 100))
    return true
end
RegisterAuraEvent(PHASE3.SPELL_GRIM_REPRISAL, 6, PHASE3.yoggGrimReprisalProc)

-- C++ spell_yogg_saron_induce_madness::HandleScript
-- (boss_yogg_saron.cpp:2801-2809): OnEffectHitTarget EFFECT_0
-- SPELL_EFFECT_SCRIPT_EFFECT -> the hit unit self-casts
-- SPELL_TELEPORT_BACK_TO_MAIN_ROOM (63992), non-triggered, then
-- removes SANITY (63050) with AURA_REMOVE_BY_ENEMY_SPELL (mode 3
-- — which fires the sanityRemove ev3 leg, the drive-me-crazy /
-- INSANE chain) and the aura named by the effect value. The
-- AfterCast ClearShatteredIllusion leg (:2811-2820: caster casts
-- 65238 nil-target + voice DoAction(9)) stays out — no spell
-- AfterCast event in the Go runtime (runtime.go:601-607, events
-- 1-7 only). Bound 64059 (Sara DoCastAOE :1110,
-- ACTION_INDUCE_MADNESS). The census AfterEffectApply hook is a
-- phantom — the class registers OnEffectHitTarget + AfterCast.
-- DORMANT until Spell.dbc ships.
PHASE3.yoggInduceMadnessHit = function(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    hitTarget:CastSpell(hitTarget, PHASE3.SPELL_TELEPORT_BACK_TO_MAIN_ROOM, false)
    hitTarget:RemoveAura(SPELL_SANITY, AURA_REMOVE_BY_ENEMY_SPELL)
    local value = spell:GetEffectValue(0)
    if value ~= 0 then
        hitTarget:RemoveAura(value)
    end
end
RegisterSpellEvent(SPELL_INDUCE_MADNESS, 5, PHASE3.yoggInduceMadnessHit)

-- C++ spell_yogg_saron_insane::OnApply / OnRemove
-- (boss_yogg_saron.cpp:2915-2926): AfterEffectApply EFFECT_0
-- SPELL_AURA_AOE_CHARM, REAL -> the caster, if a creature (Yogg),
-- whispers WHISPER_VOICE_INSANE (1) to the target, then the target
-- self-casts SPELL_INSANE_VISUAL (64464), triggered.
-- AfterEffectRemove -> if the target is alive it kills itself.
-- The Talk target arg rides the bridged creature:Talk(id, target)
-- (lua_creature_events.go:1196); the GetCaster()->ToCreature()
-- gate is the GetObjectType() == "Creature" check (aura:GetCaster()
-- resolves via luaUnitObjectForGUID, lua_aura.go:207); the
-- KillSelf leg is target:Kill(target) — player Kill takes the
-- victim arg and creditHonorableKill no-ops for killer == victim
-- (achievements.go:1802). Bound 63120 (sanity OnRemove :2879,
-- triggered). The census OnEffectHitTarget +
-- OnObjectAreaTargetSelect hooks are phantoms — the class registers
-- only AfterEffectApply + AfterEffectRemove. DORMANT until
-- Spell.dbc ships.
PHASE3.yoggInsaneApplied = function(event, aura, target)
    if target == nil then
        return
    end
    local caster = aura:GetCaster()
    if caster ~= nil and caster:GetObjectType() == "Creature" then
        caster:Talk(PHASE3.WHISPER_VOICE_INSANE, target)
    end
    target:CastSpell(target, PHASE3.SPELL_INSANE_VISUAL, true)
end
RegisterAuraEvent(SPELL_INSANE, 2, PHASE3.yoggInsaneApplied)

PHASE3.yoggInsaneRemoved = function(event, aura, target)
    if target ~= nil and target:IsAlive() then
        target:Kill(target)
    end
end
RegisterAuraEvent(SPELL_INSANE, 3, PHASE3.yoggInsaneRemoved)

-- C++ boss_voice_of_yogg_saronAI::Reset (boss_yogg_saron.cpp:490-509):
-- _Reset + SetPhase(PHASE_ONE) + instance SetData(DATA_DRIVE_ME_CRAZY) +
-- DoStopTimedAchievement + Initialize (counters only) +
-- SummonCreatureGroup(CREATURE_GROUP_CLOUDS 0) + distance-sort +
-- alternating DoAction(0/1) (clockwise) on each cloud. The
-- SetData(DATA_DRIVE_ME_CRAZY, true) leg is PORTED 2026-11-08
-- (tick 436) via the shared instanceData table (driveMeCrazyEligible slot);
-- DoStopTimedAchievement stays out (no bridge). The sort +
-- DoAction loop stays out — the cloud DoAction
-- leg is MoveCirclePath (no bridge). The group spawn itself is the
-- list-consuming site (creature:SummonCreatureGroup bridged 2026-11-05,
-- tick 238); the clouds' own Reset visual cast rides ominousCloudReset
-- (event 23) below. Reset also clears the phase-two latch, the
-- shattered flag and the stored Yogg object (voiceDoAction state).
local function voiceReset(event, creature)
    creature:SetBossState(BOSS_YOGG_SARON, 0) -- _Reset() SetBossState(BOSS_YOGG_SARON 12, NOT_STARTED), first per C++ :490-492 (PORTED tick 259)
    cancelTimers(creature:GetGUIDLow())
    voicePhase[creature:GetGUIDLow()] = 1 -- C++ Reset: SetPhase(PHASE_ONE) (:493)
    voiceShattered[creature:GetGUIDLow()] = nil
    voiceYogg[creature:GetGUIDLow()] = nil
    voiceGuardians[creature:GetGUIDLow()] = nil -- C++ Reset: Initialize() _guardianTimer = 20s (:467)
    local data = yoggInstanceData(creature)
    if data then
        data.driveMeCrazyEligible = 1 -- C++ Reset: instance->SetData(DATA_DRIVE_ME_CRAZY, true) (:495)
    end
    creature:SummonCreatureGroup(0) -- CREATURE_GROUP_CLOUDS
end

-- C++ boss_voice_of_yogg_saronAI::UpdateAI tentacle arms
-- (:575-585): DoCastAOE self-casts (DoCastAOE convention),
-- non-triggered, PHASE_TWO-gated; repeats 30s / 25s / 60s.
-- The _illusionShattered DelayEvents leg (:550-552) is twinned
-- by a 1s poll re-arm with no cast while shattered (Lua timers
-- are wall-clock — the C++ pre-Update freeze is unmodeled).
local function corruptorTick(creature, guid)
    if not creature:IsAlive() or voicePhase[guid] ~= PHASE_TWO then
        return
    end
    if voiceShattered[guid] then
        schedule(guid, "corruptor", 1000, function()
            corruptorTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_CORRUPTOR_TENTACLE_SUMMON)
    schedule(guid, "corruptor", 30000, function()
        corruptorTick(creature, guid)
    end)
end

local function constrictorTick(creature, guid)
    if not creature:IsAlive() or voicePhase[guid] ~= PHASE_TWO then
        return
    end
    if voiceShattered[guid] then
        schedule(guid, "constrictor", 1000, function()
            constrictorTick(creature, guid)
        end)
        return
    end
    -- C++ :581-582: DoCastAOE(SPELL_CONSTRICTOR_TENTACLE,
    -- {SPELLVALUE_MAX_TARGETS, 1}) — the max-targets clamp has no
    -- bridge and is dropped (voiceEnterCombat precedent).
    creature:CastSpell(creature, SPELL_CONSTRICTOR_TENTACLE)
    schedule(guid, "constrictor", 25000, function()
        constrictorTick(creature, guid)
    end)
end

local function crusherTick(creature, guid)
    if not creature:IsAlive() or voicePhase[guid] ~= PHASE_TWO then
        return
    end
    if voiceShattered[guid] then
        schedule(guid, "crusher", 1000, function()
            crusherTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_CRUSHER_TENTACLE_SUMMON)
    schedule(guid, "crusher", 60000, function()
        crusherTick(creature, guid)
    end)
end

-- C++ boss_voice_of_yogg_saronAI::UpdateAI EVENT_ILLUSION
-- (:587-604): yogg Talk EMOTE/SAY_MADNESS, portal groups
-- (10-man always, 25-man extra), urand illusion 0-2 stored
-- to instance DATA_ILLUSION, brain DoAction(
-- ACTION_INDUCE_MADNESS), 80s repeat, PHASE_TWO-gated.
local function illusionTick(creature, guid)
    if not creature:IsAlive() or voicePhase[guid] ~= PHASE_TWO then
        return
    end
    local yogg = voiceYogg[guid]
    if yogg then
        yogg:Talk(EMOTE_YOGG_SARON_MADNESS)
        yogg:Talk(SAY_YOGG_SARON_MADNESS)
    end
    creature:SummonCreatureGroup(CREATURE_GROUP_PORTALS_10)
    if creature:Is25ManRaid() then
        creature:SummonCreatureGroup(CREATURE_GROUP_PORTALS_25)
    end
    local illusion = math.random(0, 2) -- C++ urand(CHAMBER, STORMWIND) (:599)
    local data = yoggInstanceData(creature)
    if data then
        data.illusion = illusion
        if data.brain then
            data.brain:DoAction(ACTION_INDUCE_MADNESS)
        end
    end
    schedule(guid, "illusion", 80000, function()
        illusionTick(creature, guid)
    end)
end

-- C++ boss_voice_of_yogg_saronAI::DoAction (:617-641): the
-- ev38 receiver. ACTION_PHASE_TRANSFORM sets the phase latch
-- and despawns the ominous clouds by entry sweep (the moroes
-- grid-sweep twin of summons.DespawnEntry — the voice's clouds
-- are the only 33292s in the arena). ACTION_PHASE_TWO sets
-- the phase latch, summons Yogg-Saron (stored for the
-- EVENT_ILLUSION Talk legs), pulls the brain into combat and
-- seeds the tentacle + illusion timers (initials 5s / 7s /
-- 5s / 60s, :630-633). ACTION_TOGGLE_SHATTERED_ILLUSION flips
-- the shattered flag the tentacle ticks poll on. ACTION_PHASE_THREE
-- sets the phase latch and seeds the immortal-guardian timer (1s,
-- :637-640); the phase-two summon also latches data.yogg for the
-- brain DamageTaken cascade below.
local function voiceDoAction(event, creature, action)
    local guid = creature:GetGUIDLow()
    if action == ACTION_PHASE_TRANSFORM then
        voicePhase[guid] = PHASE_TRANSFORM
        for _, cloud in ipairs(creature:GetCreaturesInGrid(ENTRY_OMINOUS_CLOUD, 200)) do
            cloud:Despawn()
        end
    elseif action == ACTION_PHASE_TWO then
        voicePhase[guid] = PHASE_TWO
        voiceYogg[guid] = creature:SummonCreature(ENTRY_YOGG_SARON,
            YOGG_SARON_SPAWN_X, YOGG_SARON_SPAWN_Y, YOGG_SARON_SPAWN_Z, YOGG_SARON_SPAWN_O)
        local data = yoggInstanceData(creature)
        if data then
            -- Yogg-Saron himself is stored from the voice's SummonCreature
            -- return (temp summons bypass the ON_CREATURE_CREATE funnel —
            -- instance_hooks.go); the brain cascade reads him from here.
            data.yogg = voiceYogg[guid]
        end
        if data and data.brain then
            -- C++ :629: DoZoneInCombat(brain) — the bridge takes no
            -- argument and applies to the receiver object.
            data.brain:DoZoneInCombat()
        end
        schedule(guid, "corruptor", 5000, function()
            corruptorTick(creature, guid)
        end)
        schedule(guid, "constrictor", 7000, function()
            constrictorTick(creature, guid)
        end)
        schedule(guid, "crusher", 5000, function()
            crusherTick(creature, guid)
        end)
        schedule(guid, "illusion", 60000, function()
            illusionTick(creature, guid)
        end)
    elseif action == ACTION_TOGGLE_SHATTERED_ILLUSION then
        voiceShattered[guid] = not voiceShattered[guid]
    elseif action == ACTION_PHASE_THREE then
        -- C++ DoAction (:637-640): SetPhase(PHASE_THREE) +
        -- ScheduleEvent(EVENT_SUMMON_IMMORTAL_GUARDIAN, 1s, PHASE_THREE).
        voicePhase[guid] = PHASE_THREE
        schedule(guid, "immortal", 1000, function()
            immortalGuardianTick(creature, guid)
        end)
    end
end

-- C++ npc_constrictor_tentacleAI::Reset (:1317-1320): non-triggered
-- one-shots DoCast(me, SPELL_TENTACLE_VOID_ZONE_2 64384) +
-- DoCastAOE(SPELL_ERUPT 64144) — self-cast form per the
-- DoCastAOE≡self-cast convention (boss_ai.go lowering). Event 23
-- (the halion/corruptor/crusher Reset precedent). The JustSummoned
-- LUNGE leg rides voiceJustSummoned above (triggered, voice-side :658).
local function constrictorReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    -- C++ npc_constrictor_tentacleAI ctor (:1314): SetCombatMovement(false).
    creature:SetCombatMovement(false)
    -- C++ Reset: DoCast(me, TENTACLE_VOID_ZONE_2); DoCastAOE(ERUPT)
    -- (boss_yogg_saron.cpp:1318-1319). Alive-gated.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_TENTACLE_VOID_ZONE_2)
        creature:CastSpell(creature, SPELL_ERUPT)
    end
end

-- C++ npc_influence_tentacleAI::Reset (:1419-1423): DoCast(me,
-- me->GetEntry() == NPC_SUIT_OF_ARMOR (33433, ulduar.h:209) ?
-- SPELL_NONDESCRIPT_1 64013 : SPELL_NONDESCRIPT_2 64010) (2-arg,
-- non-triggered, ungated) — event 23; both entries registered (the
-- conditional proves the script serves both; bindings are DB-side).
-- JustDied DoAction(ACTION_TENTACLE_KILLED) is PORTED below
-- (influenceJustDied, event 4).
local function influenceReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    -- C++ Reset: DoCast(me, GetEntry()==NPC_SUIT_OF_ARMOR ?
    -- NONDESCRIPT_1 : NONDESCRIPT_2) (boss_yogg_saron.cpp:1422).
    -- Alive-gated.
    if creature:IsAlive() then
        if creature:GetEntry() == ENTRY_SUIT_OF_ARMOR then
            creature:CastSpell(creature, SPELL_NONDESCRIPT_1)
        else
            creature:CastSpell(creature, SPELL_NONDESCRIPT_2)
        end
    end
end

-- C++ npc_influence_tentacleAI::JustDied (:1425-1430): brain
-- DoAction(ACTION_TENTACLE_KILLED), ungated, via the shared
-- instanceData table — PORTED (event 4); the brain receiver
-- counts toward the illusion-shatter threshold.
local function influenceJustDied(event, creature, killer)
    local data = yoggInstanceData(creature)
    if data and data.brain then
        data.brain:DoAction(ACTION_TENTACLE_KILLED)
    end
    cancelTimers(creature:GetGUIDLow())
end

-- C++ npc_ominous_cloudAI::Reset (:1158-1162): DoCast(me,
-- SPELL_OMINOUS_CLOUD_VISUAL 63084) (2-arg, non-triggered, ungated)
-- — event 23. The DoAction MoveCirclePath leg stays out.
local function ominousCloudReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    -- C++ Reset: DoCast(me, OMINOUS_CLOUD_VISUAL)
    -- (boss_yogg_saron.cpp:1160). Alive-gated.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_OMINOUS_CLOUD_VISUAL)
    end
end

-- C++ boss_brain_of_yogg_saronAI::Reset (:1061-1066): RemoveFlag
-- NOT_SELECTABLE + DoCast(me, SPELL_MATCH_HEALTH 64066)
-- (2-arg, non-triggered, ungated) — event 23. The SetImmuneToPC
-- leg (:1064) stays out (no bridge); the _summons.DespawnAll
-- leg is PORTED below (stored-summon twin, herald precedent);
-- the DamageTaken legs (:1070-1080, triggered casts + DoAction
-- machine) stay out.
local function brainDespawnSummons(creature)
    local list = brainSummons[creature:GetGUIDLow()]
    if list then
        for _, summon in ipairs(list) do
            summon:DespawnOrUnsummon()
        end
        brainSummons[creature:GetGUIDLow()] = nil
    end
end

-- Evade/died cancel (C++ EnterEvadeMode runs Reset, which
-- despawns the summons — the same three legs as brainReset
-- minus the flag/cast arms).
local function brainCancelAll(event, creature)
    cancelTimers(creature:GetGUIDLow())
    brainTentaclesKilled[creature:GetGUIDLow()] = nil
    brainDespawnSummons(creature)
end

local function brainReset(event, creature)
    brainCancelAll(event, creature)
    -- C++ Reset: RemoveFlag NOT_SELECTABLE (:1063), ungated,
    -- before the MATCH_HEALTH cast (:1065).
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    -- C++ Reset: DoCast(me, MATCH_HEALTH)
    -- (boss_yogg_saron.cpp:1065). Alive-gated.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_MATCH_HEALTH)
    end
end

-- C++ boss_brain_of_yogg_saronAI::JustSummoned (:1132-1135):
-- _summons.Summon(summon) — the brain's summon list, tracked
-- here as stored objects (herald guards precedent) for the
-- DespawnAll legs in Reset and ACTION_TENTACLE_KILLED.
local function brainJustSummoned(event, creature, summoned)
    local guid = creature:GetGUIDLow()
    local list = brainSummons[guid]
    if not list then
        list = {}
        brainSummons[guid] = list
    end
    list[#list + 1] = summoned
end

-- C++ boss_brain_of_yogg_saronAI::DamageTaken (:1070-1090): the
-- below-30% cascade, the phase-3 sender. Gate: HealthBelowPctDamaged(
-- 30, damage) (Unit.h:873 — health after the hit strictly below 30%
-- of max) && !HasAura(SPELL_BRAIN_HURT_VISUAL 64361). DamageTaken
-- arms fire on lethal hits, so no postDamage>0 guard (nefarian
-- form). C++ order: RemoveAllAuras; InterruptNonMeleeSpells(true);
-- DoCastAOE(SHATTERED_ILLUSION_REMOVE 65238, triggered); DoCast(
-- MATCH_HEALTH_2 64069, triggered); DoCast(BRAIN_HURT_VISUAL 64361,
-- triggered); SetFlag(NOT_SELECTABLE); SetImmuneToPC(true) — the
-- last leg stays out (no SetImmuneToPC bridge); the flag + the
-- hurt-visual aura gate carry the one-shot behavior. Then DoAction(
-- ACTION_PHASE_THREE) to the voice, sara, yogg and the thorim /
-- mimiron keepers (C++ loop DATA_THORIM_YS..DATA_MIMIRON_YS, :1087)
-- via the shared instanceData table (sara is bound by
-- instance_ulduar.lua ON_CREATURE_CREATE; yogg is latched by the
-- voice's phase-two summon above; keepers by the gossip summon).
-- The damage is not rewritten (C++ touches no damage here).
local function brainDamageTaken(event, creature, attacker, damage)
    if creature:GetHealth() - damage < creature:GetMaxHealth() * 30 / 100
        and not creature:HasAura(PHASE3.SPELL_BRAIN_HURT_VISUAL) then
        creature:RemoveAllAuras()
        creature:InterruptNonMeleeSpells(true)
        creature:CastSpell(creature, PHASE3.SPELL_SHATTERED_ILLUSION_REMOVE, true)
        creature:CastSpell(creature, PHASE3.SPELL_MATCH_HEALTH_2, true)
        creature:CastSpell(creature, PHASE3.SPELL_BRAIN_HURT_VISUAL, true)
        creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
        local data = yoggInstanceData(creature)
        if data then
            if data.voice then
                data.voice:DoAction(ACTION_PHASE_THREE)
            end
            if data.sara then
                data.sara:DoAction(ACTION_PHASE_THREE)
            end
            if data.yogg then
                data.yogg:DoAction(ACTION_PHASE_THREE)
            end
            if data.thorim then
                data.thorim:DoAction(ACTION_PHASE_THREE)
            end
            if data.mimiron then
                data.mimiron:DoAction(ACTION_PHASE_THREE)
            end
        end
    end
    return false, damage
end

-- C++ boss_brain_of_yogg_saronAI::DoAction (:1096-1130): the
-- ev38 receiver. ACTION_INDUCE_MADNESS resets the kill
-- counter, summons the illusion group (the instance
-- DATA_ILLUSION value, 0-2, read from the shared instanceData
-- table) and casts INDUCE_MADNESS; the GO-door close legs
-- (:1106-1108) stay out (no HandleGameObject bridge).
-- ACTION_TENTACLE_KILLED increments the counter and, at the
-- threshold (9 for ICECROWN, 8 otherwise, :1116), fires the
-- shatter Talk, despawns the brain's summons, casts
-- SHATTERED_ILLUSION (triggered) and toggles the voice's
-- shattered flag; the GO-door open leg (:1121) stays out.
local function brainDoAction(event, creature, action)
    local guid = creature:GetGUIDLow()
    if action == ACTION_INDUCE_MADNESS then
        brainTentaclesKilled[guid] = 0
        local data = yoggInstanceData(creature)
        creature:SummonCreatureGroup(data and data.illusion or 0)
        -- C++ :1110: DoCastAOE(SPELL_INDUCE_MADNESS) —
        -- self-cast, non-triggered (DoCastAOE convention).
        creature:CastSpell(creature, SPELL_INDUCE_MADNESS)
    elseif action == ACTION_TENTACLE_KILLED then
        local killed = (brainTentaclesKilled[guid] or 0) + 1
        brainTentaclesKilled[guid] = killed
        local data = yoggInstanceData(creature)
        local illusion = data and data.illusion or 0
        if killed >= (illusion == ICECROWN_ILLUSION and 9 or 8) then
            creature:Talk(EMOTE_BRAIN_ILLUSION_SHATTERED)
            brainDespawnSummons(creature)
            -- C++ :1120: DoCastAOE(SPELL_SHATTERED_ILLUSION,
            -- triggered) — self-cast, triggered.
            creature:CastSpell(creature, SPELL_SHATTERED_ILLUSION, true)
            if data and data.voice then
                data.voice:DoAction(ACTION_TOGGLE_SHATTERED_ILLUSION)
            end
        end
    end
end

-- C++ npc_turned_championAI::Reset (:1908-1912): DoCast(
-- SPELL_VERTEX_COLOR_BLACK 39662) (1-arg self, non-triggered,
-- ungated) — event 23. MovementInform emote + DoAction(
-- ACTION_START_ROLEPLAY) stay out.
local function turnedChampionReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    -- C++ Reset: DoCast(VERTEX_COLOR_BLACK)
    -- (boss_yogg_saron.cpp:1911). Alive-gated.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_VERTEX_COLOR_BLACK)
    end
end

-- C++ npc_observation_ring_keeperAI::Reset (:1543-1548): DoCast(
-- SPELL_SIMPLE_TELEPORT_KEEPERS 12980) + DoCast(
-- SPELL_KEEPER_ACTIVE 62647) (1-arg self, non-triggered, ungated)
-- — event 23 for all four entries (33213/33241/33242/33244,
-- ulduar.h:199-202). OnGossipSelect summon + Talk legs stay out.
local function observationKeeperReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    -- C++ Reset: DoCast(SIMPLE_TELEPORT_KEEPERS);
    -- DoCast(KEEPER_ACTIVE) (boss_yogg_saron.cpp:1546-1547).
    -- Alive-gated.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_SIMPLE_TELEPORT_KEEPERS)
        creature:CastSpell(creature, SPELL_KEEPER_ACTIVE)
    end
end

-- C++ npc_yogg_saron_keeperAI::Reset (:1605-1612): DoCast(
-- SPELL_KEEPER_ACTIVE 62647) (1-arg self, non-triggered, ungated)
-- — event 23. The _events Reset/SetPhase legs stay out; the Freya
-- sanity-well GetCreatureList cleanup PORTED in yoggKeeperReset below; the
-- SetReactState(REACT_PASSIVE) leg (ungated, :1607) IS ported below
-- (react-state bridge retraction; RemoveAllAuras ported 2026-10-19,
-- passive ported this run).
local function yoggKeeperReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    keeperPhase[creature:GetGUIDLow()] = nil -- C++ Reset: SetPhase(PHASE_ONE) (:1606)
    -- C++ npc_yogg_saron_keeperAI::Reset (boss_yogg_saron.cpp:1607):
    -- me->SetReactState(REACT_PASSIVE), ungated, before RemoveAllAuras.
    creature:SetReactState(REACT_PASSIVE)
    creature:RemoveAllAuras()
    -- C++ Reset: DoCast(KEEPER_ACTIVE)
    -- (boss_yogg_saron.cpp:1610). Alive-gated.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_KEEPER_ACTIVE)
    end
    -- C++ Reset (:1615-1620), Freya only, ungated: strip both sanity
    -- well auras from every sanity well within 200.0f.
    if creature:GetEntry() == ENTRY_KEEPER_FREYA then
        for _, well in ipairs(creature:GetCreaturesInGrid(ENTRY_SANITY_WELL, 200)) do
            well:RemoveAura(SPELL_SANITY_WELL)
            well:RemoveAura(SPELL_SANITY_WELL_VISUAL)
        end
    end
end

-- C++ npc_yogg_saron_keeperAI::JustEnteredCombat (:1614-1639):
-- entry-gated 1-arg non-triggered one-shots: Freya (33410)
-- SPELL_RESILIENCE_OF_NATURE 62670 + SPELL_SANITY_WELL_SUMMON 64170;
-- Hodir (33411) SPELL_FORTITUDE_OF_FROST 62650 +
-- SPELL_HODIRS_PROTECTIVE_GAZE 64174; Thorim (33413)
-- SPELL_FURY_OF_THE_STORM 62702; Mimiron (33412)
-- SPELL_SPEED_OF_INVENTION 62671 — the auriaya engage-port
-- precedent. The voice-driven SetInCombatWith combat-entry leg
-- (:518-519) has no bridge — documented deviation (assembly_of_iron
-- precedent); keeper IsSummonedBy SIMPLE_TELEPORT_KEEPERS (:1600)
-- stays out.
local function keeperEnterCombat(event, creature, target)
    -- C++ JustEnteredCombat: entry-gated one-shots
    -- (boss_yogg_saron.cpp:1618-1637). Alive-gated.
    if creature:IsAlive() then
        local entry = creature:GetEntry()
        if entry == ENTRY_KEEPER_FREYA then
            creature:CastSpell(creature, SPELL_RESILIENCE_OF_NATURE)
            creature:CastSpell(creature, SPELL_SANITY_WELL_SUMMON)
        elseif entry == ENTRY_KEEPER_HODIR then
            creature:CastSpell(creature, SPELL_FORTITUDE_OF_FROST)
            creature:CastSpell(creature, SPELL_HODIRS_PROTECTIVE_GAZE)
        elseif entry == ENTRY_KEEPER_THORIM then
            creature:CastSpell(creature, SPELL_FURY_OF_THE_STORM)
        elseif entry == ENTRY_KEEPER_MIMIRON then
            creature:CastSpell(creature, SPELL_SPEED_OF_INVENTION)
        end
    end
end

-- C++ npc_yogg_saron_keeperAI::UpdateAI EVENT_DESTABILIZATION_MATRIX
-- (:1663-1667): DoCastAOE(SPELL_DESTABILIZATION_MATRIX 65206,
-- {SPELLVALUE_MAX_TARGETS, 1}) — self-cast, max-targets clamp
-- dropped (voiceEnterCombat precedent) — {15s,25s} repeat,
-- PHASE_TWO-gated.
local function destabilizationMatrixTick(creature, guid)
    if not creature:IsAlive() or keeperPhase[guid] ~= PHASE_TWO then
        return
    end
    if creature:IsCasting() then
        -- C++ keeper UpdateAI pre-loop CASTING return (:1657) —
        -- the mal_ganis 500ms defer idiom (darkVolleyTick above).
        schedule(guid, "destabilization", 500, function()
            destabilizationMatrixTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_DESTABILIZATION_MATRIX)
    schedule(guid, "destabilization", {15000, 25000}, function()
        destabilizationMatrixTick(creature, guid)
    end)
end

-- C++ npc_yogg_saron_keeperAI::DoAction (:1675-1690): the ev38
-- receiver. ACTION_PHASE_TWO sets the phase latch and seeds
-- EVENT_DESTABILIZATION_MATRIX at {5s,15s} — the C++ comment
-- notes the phase set is only meaningful for Thorim and
-- Mimiron, but the class serves all four keepers, so the
-- handler is registered on all four entries; the sender
-- (sara TRANSFORM_3) targets mimiron only. ACTION_PHASE_THREE is
-- PORTED below (phase latch + Thorim-only TITANIC_STORM); its sender
-- is the brain DamageTaken cascade (brainDamageTaken, :1082-1090).
-- ACTION_SANITY_WELLS / ACTION_FLASH_FREEZE PORTED tick 558 below
-- (receiver only; senders are the unported instance events
-- EVENT_ACTIVATE_SANITY_WELL / EVENT_HODIRS_PROTECTIVE_GAZE_PROC,
-- instance_ulduar.cpp:602/606).
-- Grouped into one table local: the file sits at the 200-local
-- main-chunk limit, so new keeper DoAction ids share a table
-- (LK-table precedent).
local KEEPER_DO_ACTION = {
    SANITY_WELLS = 4, -- boss_yogg_saron.cpp:370
    FLASH_FREEZE = 5, -- :371
    FLASH_FREEZE_VISUAL = 64176, -- boss_yogg_saron.cpp:227
}

local function keeperDoAction(event, creature, action)
    if action == ACTION_PHASE_TWO then
        local guid = creature:GetGUIDLow()
        keeperPhase[guid] = PHASE_TWO
        schedule(guid, "destabilization", {5000, 15000}, function()
            destabilizationMatrixTick(creature, guid)
        end)
    elseif action == ACTION_PHASE_THREE then
        -- C++ DoAction (:1681-1684): SetPhase(PHASE_THREE) — the latch
        -- flip stops the phase-two destabilization ticks (they gate on
        -- keeperPhase == PHASE_TWO) — plus a Thorim-only DoCast(
        -- SPELL_TITANIC_STORM 64171).
        keeperPhase[creature:GetGUIDLow()] = PHASE_THREE
        if creature:GetEntry() == ENTRY_KEEPER_THORIM then
            creature:CastSpell(creature, PHASE3.SPELL_TITANIC_STORM)
        end
    elseif action == KEEPER_DO_ACTION.SANITY_WELLS then
        -- C++ DoAction (:1689-1699): grid-cast SANITY_WELL +
        -- SANITY_WELL_VISUAL on every sanity well in 200 yd, self-cast
        -- (no triggered flag). GetCreaturesInGrid is the
        -- GetCreatureListWithEntryInGrid twin
        -- (lua_creature_events.go:6250).
        local wells = creature:GetCreaturesInGrid(ENTRY_SANITY_WELL, 200)
        if wells then
            for _, well in ipairs(wells) do
                well:CastSpell(well, SPELL_SANITY_WELL)
                well:CastSpell(well, SPELL_SANITY_WELL_VISUAL)
            end
        end
    elseif action == KEEPER_DO_ACTION.FLASH_FREEZE then
        -- C++ DoAction (:1700-1704): DoCast(FLASH_FREEZE_VISUAL) +
        -- EVENT_HODIRS_PROTECTIVE_GAZE in {25s,30s}, whose UpdateAI
        -- arm (:1668-1670) is a plain DoCast(HODIRS_PROTECTIVE_GAZE).
        creature:CastSpell(creature, KEEPER_DO_ACTION.FLASH_FREEZE_VISUAL)
        local guid = creature:GetGUIDLow()
        schedule(guid, "hodirsprotectivegaze", {25000, 30000}, function()
            creature:CastSpell(creature, SPELL_HODIRS_PROTECTIVE_GAZE)
        end)
    end
end

RegisterCreatureEvent(ENTRY_VOICE_OF_YOGG_SARON, 1, voiceEnterCombat)
RegisterCreatureEvent(ENTRY_VOICE_OF_YOGG_SARON, 2, cancelAll)
-- C++ boss_voice_of_yogg_saronAI::JustDied (:530-536): summons.Despawn(
-- yogg) (no bridge — removes Yogg from the voice's summon list without
-- despawning his corpse) + _JustDied() SetBossState(BOSS_YOGG_SARON 12,
-- DONE). PORTED tick 259. Fires when Yogg's own JustDied kills the voice
-- (boss_yogg_saron.cpp:962-963) and on any direct voice kill.
local function voiceDied(event, creature, killer)
    cancelTimers(creature:GetGUIDLow())
    creature:SetBossState(BOSS_YOGG_SARON, 3)
end

RegisterCreatureEvent(ENTRY_VOICE_OF_YOGG_SARON, 4, voiceDied)
RegisterCreatureEvent(ENTRY_VOICE_OF_YOGG_SARON, 19, voiceJustSummoned)
RegisterCreatureEvent(ENTRY_VOICE_OF_YOGG_SARON, 23, voiceReset)
RegisterCreatureEvent(ENTRY_VOICE_OF_YOGG_SARON, 38, voiceDoAction)
RegisterCreatureEvent(ENTRY_CONSTRICTOR_TENTACLE, 2, cancelAll)
RegisterCreatureEvent(ENTRY_CONSTRICTOR_TENTACLE, 4, cancelAll)
RegisterCreatureEvent(ENTRY_CONSTRICTOR_TENTACLE, 23, constrictorReset)
RegisterCreatureEvent(ENTRY_INFLUENCE_TENTACLE, 2, cancelAll)
RegisterCreatureEvent(ENTRY_INFLUENCE_TENTACLE, 4, influenceJustDied)
RegisterCreatureEvent(ENTRY_INFLUENCE_TENTACLE, 23, influenceReset)
RegisterCreatureEvent(ENTRY_SUIT_OF_ARMOR, 2, cancelAll)
RegisterCreatureEvent(ENTRY_SUIT_OF_ARMOR, 4, influenceJustDied)
RegisterCreatureEvent(ENTRY_SUIT_OF_ARMOR, 23, influenceReset)
RegisterCreatureEvent(ENTRY_OMINOUS_CLOUD, 2, cancelAll)
RegisterCreatureEvent(ENTRY_OMINOUS_CLOUD, 4, cancelAll)
RegisterCreatureEvent(ENTRY_OMINOUS_CLOUD, 23, ominousCloudReset)
RegisterCreatureEvent(ENTRY_BRAIN_OF_YOGG_SARON, 2, brainCancelAll)
RegisterCreatureEvent(ENTRY_BRAIN_OF_YOGG_SARON, 4, brainCancelAll)
RegisterCreatureEvent(ENTRY_BRAIN_OF_YOGG_SARON, 9, brainDamageTaken)
RegisterCreatureEvent(ENTRY_BRAIN_OF_YOGG_SARON, 19, brainJustSummoned)
RegisterCreatureEvent(ENTRY_BRAIN_OF_YOGG_SARON, 23, brainReset)
RegisterCreatureEvent(ENTRY_BRAIN_OF_YOGG_SARON, 38, brainDoAction)
RegisterCreatureEvent(ENTRY_TURNED_CHAMPION, 2, cancelAll)
RegisterCreatureEvent(ENTRY_TURNED_CHAMPION, 4, cancelAll)
RegisterCreatureEvent(ENTRY_TURNED_CHAMPION, 23, turnedChampionReset)
RegisterCreatureEvent(ENTRY_OBSERVATION_RING_KEEPER_HODIR, 2, cancelAll)
RegisterCreatureEvent(ENTRY_OBSERVATION_RING_KEEPER_HODIR, 4, cancelAll)
RegisterCreatureEvent(ENTRY_OBSERVATION_RING_KEEPER_HODIR, 23, observationKeeperReset)
RegisterCreatureEvent(ENTRY_OBSERVATION_RING_KEEPER_FREYA, 2, cancelAll)
RegisterCreatureEvent(ENTRY_OBSERVATION_RING_KEEPER_FREYA, 4, cancelAll)
RegisterCreatureEvent(ENTRY_OBSERVATION_RING_KEEPER_FREYA, 23, observationKeeperReset)
RegisterCreatureEvent(ENTRY_OBSERVATION_RING_KEEPER_THORIM, 2, cancelAll)
RegisterCreatureEvent(ENTRY_OBSERVATION_RING_KEEPER_THORIM, 4, cancelAll)
RegisterCreatureEvent(ENTRY_OBSERVATION_RING_KEEPER_THORIM, 23, observationKeeperReset)
RegisterCreatureEvent(ENTRY_OBSERVATION_RING_KEEPER_MIMIRON, 2, cancelAll)
RegisterCreatureEvent(ENTRY_OBSERVATION_RING_KEEPER_MIMIRON, 4, cancelAll)
RegisterCreatureEvent(ENTRY_OBSERVATION_RING_KEEPER_MIMIRON, 23, observationKeeperReset)

-- C++ npc_observation_ring_keeperAI::OnGossipSelect (:1551-1576):
-- menuId 10333 gate (sender == 0 DB-menu proxy — the Lua select
-- hook passes sender/action, never menuId) -> RemoveFlag gossip +
-- 2s despawn + DoCast TELEPORT + Talk 0/1 to player + entry-switched
-- keeper summon at YSKeepersPos, C++ order. C++ returns false
-- (native arms proceed); Lua mirrors it.
local function observationKeeperGossipSelect(event, player, creature, sender, action)
    if sender ~= 0 then
        return false
    end
    creature:RemoveFlag(UNIT_NPC_FLAGS_FIELD, UNIT_NPC_FLAG_GOSSIP)
    local guid = creature:GetGUIDLow()
    schedule(guid, "keeper_despawn", 2000, function()
        creature:Despawn()
    end)
    creature:CastSpell(creature, SPELL_KEEPER_TELEPORT)
    creature:Talk(SAY_KEEPER_CHOSEN_1, player)
    creature:Talk(SAY_KEEPER_CHOSEN_2, player)
    local entry = creature:GetEntry()
    local summoned = nil
    if entry == ENTRY_OBSERVATION_RING_KEEPER_FREYA then
        summoned = creature:SummonCreature(ENTRY_KEEPER_FREYA, 2036.873, 25.42513, 338.4984, 3.909538)
    elseif entry == ENTRY_OBSERVATION_RING_KEEPER_HODIR then
        summoned = creature:SummonCreature(ENTRY_KEEPER_HODIR, 1939.045, -90.87457, 338.5426, 0.994837)
    elseif entry == ENTRY_OBSERVATION_RING_KEEPER_THORIM then
        summoned = creature:SummonCreature(ENTRY_KEEPER_THORIM, 1939.148, 42.49035, 338.5427, 5.235988)
    elseif entry == ENTRY_OBSERVATION_RING_KEEPER_MIMIRON then
        summoned = creature:SummonCreature(ENTRY_KEEPER_MIMIRON, 2036.658, -73.58822, 338.4985, 2.460914)
    end
    -- The keepers are the C++ KeeperGUIDs legs
    -- (instance_ulduar.cpp:357-377, DATA_FREYA_YS..DATA_MIMIRON_YS);
    -- latch all four in the shared instanceData table — the
    -- instance ON_CREATURE_CREATE handler cannot see them (temp
    -- summons bypass the funnel — instance_hooks.go). Mimiron is
    -- also the sara TRANSFORM_3 DoAction target (DATA_MIMIRON_YS).
    if summoned then
        local data = yoggInstanceData(creature)
        if data then
            -- C++ keepersCount (instance_ulduar.cpp:357-377,
            -- OnCreatureCreate per YS keeper, DATA_KEEPERS_COUNT):
            -- the gossip summon is the only spawn path in Go, so
            -- count each slot's first summon; the nil guard keeps
            -- a re-gossip from double-counting.
            if entry == ENTRY_OBSERVATION_RING_KEEPER_FREYA then
                if data.freya == nil then
                    data.keepersCount = (data.keepersCount or 0) + 1
                end
                data.freya = summoned
            elseif entry == ENTRY_OBSERVATION_RING_KEEPER_HODIR then
                if data.hodir == nil then
                    data.keepersCount = (data.keepersCount or 0) + 1
                end
                data.hodir = summoned
            elseif entry == ENTRY_OBSERVATION_RING_KEEPER_THORIM then
                if data.thorim == nil then
                    data.keepersCount = (data.keepersCount or 0) + 1
                end
                data.thorim = summoned
            elseif entry == ENTRY_OBSERVATION_RING_KEEPER_MIMIRON then
                if data.mimiron == nil then
                    data.keepersCount = (data.keepersCount or 0) + 1
                end
                data.mimiron = summoned
            end
        end
    end
    return false
end
RegisterCreatureGossipEvent(ENTRY_OBSERVATION_RING_KEEPER_FREYA, 2, observationKeeperGossipSelect)
RegisterCreatureGossipEvent(ENTRY_OBSERVATION_RING_KEEPER_HODIR, 2, observationKeeperGossipSelect)
RegisterCreatureGossipEvent(ENTRY_OBSERVATION_RING_KEEPER_THORIM, 2, observationKeeperGossipSelect)
RegisterCreatureGossipEvent(ENTRY_OBSERVATION_RING_KEEPER_MIMIRON, 2, observationKeeperGossipSelect)
RegisterCreatureEvent(ENTRY_KEEPER_FREYA, 1, keeperEnterCombat)
RegisterCreatureEvent(ENTRY_KEEPER_FREYA, 2, cancelAll)
RegisterCreatureEvent(ENTRY_KEEPER_FREYA, 4, cancelAll)
RegisterCreatureEvent(ENTRY_KEEPER_FREYA, 23, yoggKeeperReset)
RegisterCreatureEvent(ENTRY_KEEPER_FREYA, 38, keeperDoAction)
RegisterCreatureEvent(ENTRY_KEEPER_HODIR, 1, keeperEnterCombat)
RegisterCreatureEvent(ENTRY_KEEPER_HODIR, 2, cancelAll)
RegisterCreatureEvent(ENTRY_KEEPER_HODIR, 4, cancelAll)
RegisterCreatureEvent(ENTRY_KEEPER_HODIR, 23, yoggKeeperReset)
RegisterCreatureEvent(ENTRY_KEEPER_HODIR, 38, keeperDoAction)
RegisterCreatureEvent(ENTRY_KEEPER_MIMIRON, 1, keeperEnterCombat)
RegisterCreatureEvent(ENTRY_KEEPER_MIMIRON, 2, cancelAll)
RegisterCreatureEvent(ENTRY_KEEPER_MIMIRON, 4, cancelAll)
RegisterCreatureEvent(ENTRY_KEEPER_MIMIRON, 23, yoggKeeperReset)
RegisterCreatureEvent(ENTRY_KEEPER_MIMIRON, 38, keeperDoAction)
RegisterCreatureEvent(ENTRY_KEEPER_THORIM, 1, keeperEnterCombat)
RegisterCreatureEvent(ENTRY_KEEPER_THORIM, 2, cancelAll)
RegisterCreatureEvent(ENTRY_KEEPER_THORIM, 4, cancelAll)
RegisterCreatureEvent(ENTRY_KEEPER_THORIM, 23, yoggKeeperReset)
RegisterCreatureEvent(ENTRY_KEEPER_THORIM, 38, keeperDoAction)
