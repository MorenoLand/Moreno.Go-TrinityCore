-- Deathbringer Saurfang (Icecrown Citadel) — Lua port of
-- src/server/scripts/Northrend/IcecrownCitadel/boss_deathbringer_saurfang.cpp
-- (1275 lines incl. license; 4 CreatureScripts
-- (boss_deathbringer_saurfang (BossAI, DATA_DEATHBRINGER_SAURFANG = 3),
-- npc_high_overlord_saurfang_icc (ScriptedAI intro/outro event NPC),
-- npc_muradin_bronzebeard_icc (ScriptedAI intro/outro event NPC),
-- npc_saurfang_event (ScriptedAI guard)) + 7 SpellScripts
-- (spell_deathbringer_blood_link, spell_deathbringer_blood_power,
-- spell_deathbringer_rune_of_blood, spell_deathbringer_blood_nova,
-- spell_deathbringer_blood_nova_targeting,
-- spell_deathbringer_boiling_blood, spell_deathbringer_remove_marks)
-- + 3 AuraScripts (spell_deathbringer_blood_link_aura,
-- spell_deathbringer_blood_power_aura,
-- spell_deathbringer_blood_beast_blood_link) + 1
-- AchievementCriteriaScript
-- (achievement_ive_gone_and_made_a_mess); all registered from
-- inside AddSC_boss_deathbringer_saurfang(); loader decl 174 /
-- call 369 per northrend_script_loader.cpp — the FOURTH group of
-- the "// Icecrown Citadel" block in AddNorthrendScripts(),
-- immediately after AddSC_boss_icecrown_gunship_battle() (call
-- 368); the call after it is AddSC_boss_festergut() — verified
-- from the loader this run; the checkpoint sequence
-- (boss_icecrown_gunship_battle -> boss_deathbringer_saurfang) is
-- followed).
-- Entry: 37813 Deathbringer Saurfang (icecrown_citadel.h
-- NPC_DEATHBRINGER_SAURFANG, line 211; instance_icecrown_citadel.cpp
-- OnCreatureCreate binds case NPC_DEATHBRINGER_SAURFANG, line 227)
-- — entry-verifiable, registration proceeds (the nexus_commanders
-- kalecgos precedent); the CreatureScript ScriptName binding is
-- DB-side as usual.
-- DOCUMENTED-ENTRY EVIDENCE (for later reference, not registered):
-- icecrown_citadel.h — NPC_BLOOD_BEAST = 38508 (line 212, summoned
-- by the SPELL_SUMMON_BLOOD_BEAST* casts; the beast has no AI
-- script in this file — generic creature); NPC_SE_MURADIN_BRONZEBEARD
-- = 37200 (line 214, spawned when TeamInInstance == ALLIANCE);
-- NPC_SE_HIGH_OVERLORD_SAURFANG = 37187 (line 216, spawned when
-- TeamInInstance == HORDE); NPC_SE_KOR_KRON_REAVER = 37920 /
-- NPC_SE_SKYBREAKER_MARINE = 37830 (guard entries); instance
-- OnCreatureCreate binds the saurfang/muradin event NPCs (lines
-- 234-235) and GetData(DATA_SAURFANG_EVENT_NPC) selects the
-- faction one (line 392-393).
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_deathbringer_saurfang" hits
-- boss_deathbringer_saurfang.cpp (+ the loader decl/call lines)
-- only; this clone carries no sql/ tree, so ScriptName bindings are
-- DB-side by construction.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnKill
-- (player-gated), 4 OnDied, 9 OnDamageTaken, 15 OnSpellHitTarget,
-- 23 OnReset, 24 OnReachHome, 38 OnDoAction.
-- Timers via CreateLuaEvent (GUID-keyed, cancelled on events 2/4/23); melee is
-- engine-driven in Go.
--
-- DOCTRINE RETRACTION (2026-10-13): the earlier port of this file
-- ported Talk arms only under the disproved "no-timer-bridge"
-- doctrine. CreateLuaEvent is a real registered bridge
-- (engine/scripting/runtime.go:798); timer-driven cast arms port on
-- the maiden_of_virtue / moroes / curator / nightbane convention:
-- timers table keyed by creature GUID, cancelled on Eluna events 2
-- (leave combat), 4 (death), 23 (reset); CastSpell(nil, spellId) =
-- victim-cast; CastSpell(creature, spellId) = self-cast (the
-- DoCastAOE convention); {min,max} delay = randtime range;
-- TRIGGERED casts pass true as the third CastSpell arg; event 9
-- fires pre-application and `return true, newDamage` rewrites the
-- damage (the gunship fireCreatureDamageTaken precedent). Phase /
-- intro-outro / summon / cross-creature machines are the real
-- blockers (the gothik precedent), not timers.
--
-- PORTED ARMS (C++-exact for all modeled arms):
-- JustEngagedWith (:275-313) — Talk(SAY_AGGRO 7) (:302); the five
--   combat scheduler seeds: EVENT_SUMMON_BLOOD_BEAST 30s (:304),
--   EVENT_BERSERK (:305, see deviation), EVENT_BOILING_BLOOD 15500ms
--   (:306), EVENT_BLOOD_NOVA 17s (:307), EVENT_RUNE_OF_BLOOD 20s
--   (:308). The _fallenChampionCastCount = 0 latch (:310) is PORTED
--   (initState zeroes it; the DATA_MADE_A_MESS SetData latch rides
--   it — see saurfangDoAction below); instance->DoRemoveAurasDueToSpellOnPlayers(
--   SPELL_MARK_OF_THE_FALLEN_CHAMPION 72293) (:311) is PORTED via
--   GetPlayersInWorld + mapId/instanceId filter + player:RemoveAura
--   (the moroes precedent — the stale "no instance bridge" claim
--   for this leg is retracted). instance->SetBossState(IN_PROGRESS)
--   (:312) PORTED 2026-11-05 via creature:SetBossState, last in the
--   handler. The CheckRequiredBosses evade branch and
--   DoCastSpellOnPlayers legs have no instance bridge
--   (DoZoneInCombat :289 is PORTED 2026-10-23 via the live
--   creature:DoZoneInCombat bridge — C++ arm order, after
--   me->setActive(true)); the !_introDone branch (:293-298) PORTS via
--   relayInterruptIntro below: DoCastSelf(SPELL_GRIP_OF_AGONY 70572) +
--   the ACTION_INTERRUPT_INTRO guard-despawn + NPC-timer-cancel relay
--   (the event NPC's _events.Reset leg ports as cancelTimers on the NPC
--   GUID + phase-mirror clear); the latch lives outside
--   saurfangState because C++ Reset never clears _introDone.
-- EVENT_SUMMON_BLOOD_BEAST (:493-502) — DoCastSelf(
--   SPELL_SUMMON_BLOOD_BEAST+i10 72172/72173) = 2x non-triggered
--   self-cast, then Talk(SAY_BLOOD_BEASTS 9) (:498); re-arm 40s
--   unconditional (:499). The 25-man trio DoCastSelf(
--   SPELL_SUMMON_BLOOD_BEAST_25_MAN+i25 72356/72357/72358) (:495-497)
--   ported via creature:Is25ManRaid() (C++ :495-497 gate, cast
--   order kept: pair, then trio, then Talk).
-- EVENT_BLOOD_NOVA (:504-507) — DoCastAOE(SPELL_BLOOD_NOVA_TRIGGER
--   72378) = non-triggered self-cast; re-arm {20s,25s} randtime.
-- EVENT_RUNE_OF_BLOOD (:508-511) — DoCastVictim(SPELL_RUNE_OF_BLOOD
--   72410) = non-triggered victim-cast; re-arm {20s,25s} randtime.
-- EVENT_BOILING_BLOOD (:512-515) — DoCastSelf(SPELL_BOILING_BLOOD
--   72385) = non-triggered self-cast; re-arm {15s,20s} randtime. The
--   cast leg uses the plain 72385 id; the RAID_MODE aura-id helper
--   (72385/72441/72442/72443) lives in the SpellScript (no bridge).
-- EVENT_BERSERK (:516-519) — DoCastSelf(SPELL_BERSERK 26662)
--   non-triggered + Talk(SAY_BERSERK 12), cast-before-Talk kept;
--   one-shot seeded on engage with the IsHeroic() ? 360s : 480s seed
--   (:305) via creature:IsHeroic() (the marrowgar berserk precedent).
-- KilledUnit (:344-348) — Talk(SAY_KILL 10) player-gated, the
--   razuvious player-gated variant precedent.
-- DamageTaken (:350-378) — the never-die damage cap (damage =
--   health-1 when damage >= health, :351-352) rewrites via event 9
--   `return true, newDamage` (the gunship precedent); the frenzy
--   latch (:355-360): !_frenzied && HealthBelowPct(31) — HealthBelowPct
--   is STRICTLY current-health < 31% of max (Unit.h:872), not
--   post-damage — DoCastSelf(SPELL_FRENZY 72737, true) TRIGGERED +
--   Talk(SAY_FRENZY 11); the fight-won leg (:362-377): !_dead &&
--   health-damage < FightWonValue (100000, :614, computed with the
--   CAPPED damage) — cancel timers (the _JustDied events.Reset),
--   SetBossState DONE (:364, _JustDied — PORTED 2026-11-05 via
--   creature:SetBossState, first in the block),
--   DoCastAOE(SPELL_REMOVE_MARKS_OF_THE_FALLEN_CHAMPION 72257) =
--   non-triggered self-cast (:370), DoCastSelf(SPELL_ACHIEVEMENT
--   72928, true) TRIGGERED (:371), Talk(SAY_DEATH 13) (:372),
--   DoCastSelf(SPELL_REPUTATION_BOSS_KILL 73843, true) TRIGGERED
--   (:373), DoCastSelf(SPELL_PERMANENT_FEIGN_DEATH 70628) (:374).
--   C++ order kept (frenzy check before fight-won; both may fire on
--   one hit). The _EnterEvadeMode / RemoveAurasOnEvade legs have no
--   bridge; the cross-AI DoAction(ACTION_START_OUTRO) leg (:375-376)
--   PORTS via relayStartOutro below; the SetImmuneToPC(true) (:368)
--   leg IS bridged —
--   SetFlag(59, 0x100), the nefarian/opera twin — ported flag-only in
--   C++ order.
-- Reset (:257-273) — C++ schedules no timers here, so event 23 only
--   cancels timers and replays the bridgeable aura-setup arms:
--   DoCastSelf(SPELL_ZERO_POWER 72242 / SPELL_BLOOD_LINK 72178 /
--   SPELL_BLOOD_POWER 72371 /
--   SPELL_MARK_OF_THE_FALLEN_CHAMPION_S 72256 /
--   SPELL_RUNE_OF_BLOOD_S 72408, true) all TRIGGERED (:264-268) +
--   RemoveAurasDueToSpell(SPELL_BERSERK / SPELL_FRENZY) via the
--   RemoveAura bridge (the arlokk / bronjahm idiom) + SetPower(ENERGY, 0)
--   (:265) via the creature:SetPower bridge (lua_creature_events.go).
--   The _Reset() SetBossState NOT_STARTED leg (:261, != DONE-gated)
--   PORTED 2026-11-05 via creature:SetBossState (the falric
--   precedent); the SetPhase bookkeeping leg has no bridge.
--   Mirrors the `if (_dead) return;` early-out (:277-278): after the
--   fight-won leg the reset casts are skipped.
-- JustDied (:315-317) is EMPTY in C++ — event 4 only cancels timers
--   and clears state (the sjonnir JustDied-Talk precedent does NOT
--   apply; SAY_DEATH 13 rides the DamageTaken fight-won leg above).
--
-- DOCUMENTED-ONLY (no registration beyond entry 37813):
-- boss_deathbringer_saurfang SpellHitTarget (:413-435) — PORTED
--   via ev15 (saurfangSpellHitTarget below): Talk(SAY_MARK_OF_THE_
--   FALLEN_CHAMPION 8) on the 72293 case; the 72255/72444/72445/
--   72446 triggered-mark cases cast SPELL_BLOOD_LINK_DUMMY 72202
--   (BP0=1, triggered) when power != max. The transport evade leg
--   (:415-419) stays out — EnterEvadeMode has no bridge.
--   (event 15 itself is live — boss_ai.go:1904; the old "rides the
--   unported ACTION machine" note is retired — the machine ports
--   below via ev38 + the blood-link ev1 tick.)
-- boss_deathbringer_saurfang SpellHit (:443-446) — STAY OUT (hook-coverage
--   strip round 6, row 12): if spellInfo->Id == SPELL_BLOOD_LINK_POWER
--   72195 and me->GetAura(SPELL_BLOOD_POWER 72371) exists, the aura's
--   RecalculateAmountOfEffects() runs. ev14 itself is a live event
--   (other files register it), but Aura:RecalculateAmountOfEffects has
--   NO Lua bridge — engine/world/lua_aura.go exposes GetAmount/SetAmount
--   and no Recalculate method, and no RecalculateAmountOfEffects symbol
--   exists anywhere under engine/. A SetAmount emulation would fabricate
--   the DBC-driven amount formula, so no ev14 registration. Same blocker
--   as the :591 DoAction recalc leg (stays out, see bloodLinkTick note).
-- boss_deathbringer_saurfang intro machine — PORTED 2026-11-09 (this tick):
--   the tick-556 "all three DoAction arms STAY OUT" verdict is retired —
--   all three blockers fell: RegisterCreatureGossipEvent is a live bridge
--   (runtime.go:1316, barada precedent), MoveTo/MoveCharge are bridged
--   (lua_creature_events.go:4947/:5037, ev28 POINT_MOTION_TYPE arrival),
--   and events.SetPhase is mirrored by the saurfangIntroPhase /
--   eventNpcPhase tables (the assembly_of_iron phase-mirror precedent).
--   PHASE_INTRO_A (1) / PHASE_INTRO_H (2) (:548-565) port via ev38
--   (saurfangDoAction below): SetHomePosition(deathbringerPos, live
--   orientation via GetO) + MoveTo(POINT_SAURFANG 3781300) + faction
--   intro schedules (ALLIANCE_2 2.5s / _3 20s; HORDE_2 5s).
--   ACTION_CONTINUE_INTRO (-3781301, :567-579) ports via ev38:
--   _introDone-gated (saurfangIntroDone, now also set by EVENT_INTRO_
--   FINISH so a post-intro engage skips the GRIP/interrupt leg exactly
--   as C++ :293-298); faction schedules (ALLIANCE_6 7s / FINISH 8s;
--   HORDE_4 6.5s / HORDE_9 48.2s / FINISH 55.7s). EVENT_INTRO_* bodies
--   (:463-492) port as the schedule closures: ALLIANCE_2/HORDE_2
--   RemoveFlag(NOT_SELECTABLE) + SetFaction(974) + Talk; ALLIANCE_6
--   Talk x2 + GRIP self-cast; HORDE_9 GRIP + Talk; FINISH _introDone +
--   SetImmuneToPC(false) (RemoveFlag(59, 0x100) twin).
--   The event NPCs (37187/37200) register gossip select (event 2) +
--   ev23/ev28: OnGossipSelect -> DoAction(ACTION_START_EVENT) (:629/
--   :829; menuId 10952/10934 is NOT forwarded by the Go hook and C++
--   ignores gossipListId, so any select on the entry's menu starts the
--   event); START_EVENT (:651-670/:851-870): distance-sorted guard
--   SetData(0, x) 1..5 (guardSortIndex table — guards unregistered),
--   gossip-flag clear (SetNPCFlags twin), Talk INTRO_1, phase set,
--   EVENT_INTRO_HORDE_3 18.5s / ALLIANCE_4 29.5s (SetWalk + MoveTo
--   POINT_FIRST_STEP), deathbringer DoAction(PHASE_INTRO_*).
--   MovementInform POINT_FIRST_STEP (ev28) ports: SetWalk(false) +
--   Talk INTRO_3/4 + HORDE_5/6/7/8 / ALLIANCE_5 schedules +
--   deathbringer DoAction(ACTION_CONTINUE_INTRO); HORDE_8/ALLIANCE_5
--   also broadcast ACTION_CHARGE (guard MoveCharge to chargePos[index]
--   at 13.0, self MoveCharge to chargePos[0] at 8.5).
--   Stay-outs: GO_SAURFANG_S_DOOR HandleGameObject (:669/:868, no
--   instance-GO bridge); the WAYPOINT outro chain (POINT_EXIT guards,
--   :763-771/:933-940). SpellHit GRIP legs (event NPC + guards MovePoint
--   CHOKE, :700-706/:908-914/:970-976) are PORTED via ev14
--   (eventNpcSpellHit below).
--   The ACTION_INTERRUPT_INTRO relay (relayInterruptIntro below) now
--   also cancels the event NPC's pending intro timers (C++ _events.
--   Reset, :689/:898) — the pre-existing relay only despawned guards.
--   (DoAction dispatch itself IS bridged — creature:DoAction fires
--   ev38, lua_creature_events.go:802.) The
--   ACTION_MARK_OF_THE_FALLEN_CHAMPION leg (:580-592) PORTED via
--   ev38 (saurfangDoAction below) + the blood-link ev1 tick
--   (bloodLinkTick below): random threat-list player, alive, without
--   the 72293 mark (moragg GetAITargets idiom) -> count++ -> DoCast
--   72293 non-triggered -> SetPower(ENERGY, 0) -> latch clear at
--   3/5. The blood-power RecalculateAmountOfEffects leg (:591)
--   stays out (no bridge); the trigger was the
--   spell_deathbringer_blood_link_aura periodic tick (:1028), now
--   PORTED via RegisterAuraEvent ev1 on 72178 — the old "no
--   AuraScript bridge" claim is retired for this script. The
--   _fallenChampionCastCount / GetData(DATA_MADE_A_MESS)
--   achievement legs ride with it (Go gate added this tick).
-- boss_deathbringer_saurfang JustSummoned (:380-399) — PORTED
--   2026-11-08 (tick 493, below): random-target AttackStart,
--   SPELL_BLOOD_LINK_BEAST 72176 + SPELL_RESISTANT_SKIN 72723 triggered
--   casts on the summon, summons.Summon (the live-beast set), DoZoneInCombat,
--   and the transport evade leg. ev19 fires from the summonTempCreature
--   funnel, which the spell-cast summons reach via creatureEffectSummonType
--   (boss_ai.go:3456/:4587) — the old "no summon / cross-creature bridge"
--   claim is retracted. SummonedCreatureDies (:400-404) ports via the
--   ev20/ev21 set legs below. MovementInform (:406-411, instance GO leg),
--   AttackStart/CanAIAttack transport legs stay out. EnterEvadeMode
--   (:327-331) — the SetImmuneToPC(false) leg PORTS in ev2 (RemoveFlag
--   0x100 twin, intro-done + not-dead gated, 2026-11-05); the ScriptedAI
--   base leg rides the full-evade bridge (lua_creature_events.go:5014).
--   JustReachedHome (:334-342) — SetBossState FAIL + Reset() +
--   DoRemoveAurasDueToSpellOnPlayers all PORT in the ev24 handler
--   (the old "no instance / motion bridges" claim is retracted);
--   MovementInform (:406-411, instance GO leg) stays out.
-- EVENT_SCENT_OF_BLOOD (:520-526) — PORTED 2026-11-08 (tick 493, below):
--   heroic-gated via creature:IsHeroic() AND gated on the live-beast set
--   (ev19/ev20/ev21); one-shot, never re-armed. The old "no summon-list
--   bridge" stay-out is retired by ev19+ev20+ev21.
-- npc_high_overlord_saurfang_icc / npc_muradin_bronzebeard_icc —
--   REGISTERED this tick (gossip select event 2 + ev23 + ev28; the
--   bronjahm npc_corrupted_soul_fragment precedent no longer applies —
--   the gossip bridge is live): gossip -> DoAction(ACTION_START_EVENT)
--   (eventNpcStartEvent below); Talk(SAY_INTRO_HORDE_1 0 /
--   SAY_INTRO_ALLIANCE_1 0) ride the START_EVENT legs; Talk(
--   SAY_INTRO_HORDE_3 1 / SAY_INTRO_ALLIANCE_4 1) ride the
--   MovementInform POINT_FIRST_STEP arrival; Talk(SAY_INTRO_HORDE_5..8
--   2..5 / SAY_INTRO_ALLIANCE_5 2) ride the intro scheduler machine.
--   The outro arms still port via the deathbringer-side relay below
--   (relayStartOutro); Talk(SAY_OUTRO_HORDE_4 14) rides the OUTRO_6
--   move-arrival chain (PORTED this tick via
--   deathbringer:GetRelativePoint(0, 0) = GetClosePoint twin);
--   the muradin outro chain rides waypoint/movement events (no bridge).
--   SpellHit(SPELL_GRIP_OF_AGONY) movement legs PORTED via ev14
--   (eventNpcSpellHit below). The cross-AI deathbringer->AI()->DoAction(
--   PHASE_INTRO_*/ACTION_CONTINUE_INTRO) legs are live receivers in
--   saurfangDoAction below (dispatch IS bridged — creature:DoAction
--   fires ev38).
-- npc_saurfang_event (guards) — zero own Talk lines; NOT registered
--   (the bronjahm npc_corrupted_soul_fragment precedent):
--   DoAction(ACTION_DESPAWN) ports via the relay guard sweeps below;
--   DoAction(ACTION_CHARGE) ports via the intro charge broadcast
--   (chargeEventGuards below — the _index sort lives in guardSortIndex,
--   guards otherwise unregistered); SpellHit GRIP legs PORTED via ev14
--   (eventNpcSpellHit below — guards carry only the ev14 handler).
-- spell_deathbringer_blood_link / spell_deathbringer_blood_power /
-- spell_deathbringer_rune_of_blood / spell_deathbringer_blood_nova /
-- spell_deathbringer_blood_nova_targeting /
-- spell_deathbringer_boiling_blood / spell_deathbringer_remove_marks
-- join the no-SpellScript-bridge queue (the boss_moragg
-- optic-link precedent); spell_deathbringer_blood_power_aura /
-- spell_deathbringer_blood_beast_blood_link join the
-- no-AuraScript-bridge queue (spell_deathbringer_blood_link_aura is
-- PORTED this tick via RegisterAuraEvent ev1 on 72178).
-- achievement_ive_gone_and_made_a_mess PORTED this tick (Lua latch +
-- Go gate in achievement_criteria_scripts.go).
-- Talk accounting (C++ call sites in this file): newly ported this
--   tick — boss intro 0/1/2/3/4/5/6 (8 calls: ALLIANCE_6 Talks twice)
--   + horde event-NPC 0/1/2/3/4/5 (6) + muradin 0/1/2 (3);
--   previously ported — aggro 7 + kill 10 + frenzy 11 + death 13 +
--   blood beasts 9 + berserk 12 + horde outro 11/12/13/14 + alliance
--   outro 3 (relayStartOutro below); still documented-only — mark 8
--   (1) + emote 14 (1) + muradin outro 4..12 (waypoint chain).
--
-- DEVIATIONS FROM C++ (documented; every other value C++-exact):
-- EVENT_SCENT_OF_BLOOD (:520-526) PORTED 2026-11-08 (tick 493): the
--   !summons.empty() gate reads the ev19/ev20/ev21 live-beast set, so the
--   emote + AoE fire only while a beast is alive.
-- EVENT_SUMMON_BLOOD_BEAST 25-man trio (72356/72357/72358) PORTED
--   via creature:Is25ManRaid() (C++ :495-497, cast order kept).
-- The UNIT_STATE_CASTING gate in UpdateAI (pre-loop :456 + in-loop
--   :531; the old note's line numbers drifted) is PORTED 2026-11-04:
--   500ms IsCasting defer in all six ported combat ticks + the two
--   intro GRIP legs (2026-11-09).
-- Lua OnReset (event 23) replays the Reset aura-setup casts; C++
--   Reset also runs at initial spawn — if the engine never fires 23
--   pre-combat, the setup auras apply on the first evade instead.

local ENTRY_DEATHBRINGER_SAURFANG = 37813
local DATA_DEATHBRINGER_SAURFANG = 3 -- icecrown_citadel.h:77
local ACTION_MARK_OF_THE_FALLEN_CHAMPION = -72293 -- boss_deathbringer_saurfang.cpp:202
local DATA_MADE_A_MESS = 45374613 -- boss_deathbringer_saurfang.cpp:207 (4537, 4613 achievement IDs)
local BOSS_STATE_NOT_STARTED = 0 -- InstanceScript.h:68-76
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_FAIL = 2
local BOSS_STATE_DONE = 3
local ENTRY_SE_HIGH_OVERLORD_SAURFANG = 37187 -- icecrown_citadel.h:216
local ENTRY_SE_MURADIN_BRONZEBEARD = 37200 -- icecrown_citadel.h:214
local ENTRY_SE_KOR_KRON_REAVER = 37920 -- icecrown_citadel.h:217
local ENTRY_SE_SKYBREAKER_MARINE = 37830 -- icecrown_citadel.h:218

local PHASE_INTRO_A = 1 -- boss_deathbringer_saurfang.cpp:187-192
local PHASE_INTRO_H = 2
local ACTION_START_EVENT = -3781300 -- boss_deathbringer_saurfang.cpp:194-203
local ACTION_CONTINUE_INTRO = -3781301
local POINT_SAURFANG = 3781300 -- boss_deathbringer_saurfang.cpp:213-222
local POINT_FIRST_STEP = 3781301
local POINT_CHARGE = 3781302
local POINT_CHOKE = 3781303
local POINT_CORPSE = 3781304
local POINT_FINAL = 3781305
local FACTION_UNDEAD_SCOURGE = 974 -- SharedDefines.h:253
local UNIT_FIELD_FLAGS = 59 -- UpdateFields.h (OBJECT_END + 0x35; the FL precedent)
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000 -- UnitDefines.h:149
local UNIT_NPC_FLAG_GOSSIP = 0x00000001 -- UnitDefines.h:186
local POINT_MOTION_TYPE = 8 -- MovementDefines.h:36 (the barada precedent)
-- Boss Talk ids, boss_deathbringer_saurfang.cpp:30-46.
local SAY_INTRO_ALLIANCE_2 = 0
local SAY_INTRO_ALLIANCE_3 = 1
local SAY_INTRO_ALLIANCE_6 = 2
local SAY_INTRO_ALLIANCE_7 = 3
local SAY_INTRO_HORDE_2 = 4
local SAY_INTRO_HORDE_4 = 5
local SAY_INTRO_HORDE_9 = 6
-- High Overlord Saurfang Talk ids, boss_deathbringer_saurfang.cpp:48-64.
local SAY_H_INTRO_1 = 0
local SAY_H_INTRO_3 = 1
local SAY_H_INTRO_5 = 2
local SAY_H_INTRO_6 = 3
local SAY_H_INTRO_7 = 4
local SAY_H_INTRO_8 = 5
-- Muradin Talk ids, boss_deathbringer_saurfang.cpp:65-80.
local SAY_A_INTRO_1 = 0
local SAY_A_INTRO_4 = 1
local SAY_A_INTRO_5 = 2
local DEATHBRINGER_X = -496.3542 -- deathbringerPos, boss_deathbringer_saurfang.cpp:223
local DEATHBRINGER_Y = 2211.33
local DEATHBRINGER_Z = 541.1138
local FIRST_STEP_X = -541.3177 -- firstStepPos, boss_deathbringer_saurfang.cpp:224
local FIRST_STEP_Y = 2211.365
local FIRST_STEP_Z = 539.2921
local FINAL_X = -563.7552 -- finalPos, boss_deathbringer_saurfang.cpp:247
local FINAL_Y = 2211.328
local FINAL_Z = 538.7848
-- chargePos[1] is the event NPC's own slot; [2..6] are the guards'
-- (C++ chargePos[0..5], boss_deathbringer_saurfang.cpp:226-234;
-- guard _index 1..5 -> Lua slots 2..6).
local CHARGE_POS = {
    { -509.6505, 2211.377, 539.2872 },
    { -508.7480, 2211.897, 539.2870 },
    { -509.2929, 2211.411, 539.2870 },
    { -506.6607, 2211.367, 539.2870 },
    { -506.1137, 2213.306, 539.2870 },
    { -509.0040, 2211.743, 539.2870 },
}
-- chokePos[0] is the event NPC's own slot; [1..5] are the guards'
-- (C++ chokePos[0..5], boss_deathbringer_saurfang.cpp:236-244;
-- guard _index 1..5 -> Lua slots 2..6).
local CHOKE_POS = {
    { -514.4834, 2211.334, 549.2887 },
    { -510.1081, 2211.592, 546.3773 },
    { -513.3210, 2211.396, 551.2882 },
    { -507.3684, 2210.353, 545.7497 },
    { -507.0486, 2212.999, 545.5512 },
    { -510.7041, 2211.069, 546.5298 },
}

local SAY_AGGRO = 7
local SAY_MARK_OF_THE_FALLEN_CHAMPION = 8
local SAY_BLOOD_BEASTS = 9
local SAY_KILL = 10
local SAY_FRENZY = 11
local SAY_BERSERK = 12
local SAY_DEATH = 13
local SAY_OUTRO_HORDE_1 = 11
local SAY_OUTRO_HORDE_2 = 12
local SAY_OUTRO_HORDE_3 = 13
local SAY_OUTRO_HORDE_4 = 14
local SAY_OUTRO_ALLIANCE_1 = 3

local SPELL_BERSERK = 26662 -- icecrown_citadel.h:33
local SPELL_SUMMON_BLOOD_BEAST = 72172
local SPELL_SUMMON_BLOOD_BEAST_25_MAN = 72356
local SPELL_BLOOD_LINK_BEAST = 72176 -- boss_deathbringer_saurfang.cpp:113
local SPELL_RESISTANT_SKIN = 72723 -- boss_deathbringer_saurfang.cpp:114
local SPELL_SCENT_OF_BLOOD = 72769 -- boss_deathbringer_saurfang.cpp:115 (heroic only)
local EMOTE_SCENT_OF_BLOOD = 14 -- boss_deathbringer_saurfang.cpp:46
local ENTRY_BLOOD_BEAST = 38508 -- boss_deathbringer_saurfang.cpp:212
local SPELL_FRENZY = 72737
local SPELL_BLOOD_NOVA_TRIGGER = 72378
local SPELL_BOILING_BLOOD = 72385
local SPELL_RUNE_OF_BLOOD = 72410
local SPELL_REMOVE_MARKS_OF_THE_FALLEN_CHAMPION = 72257
local SPELL_ACHIEVEMENT = 72928
local SPELL_REPUTATION_BOSS_KILL = 73843 -- icecrown_citadel.h:35
local SPELL_PERMANENT_FEIGN_DEATH = 70628
local SPELL_GRIP_OF_AGONY = 70572 -- boss_deathbringer_saurfang.cpp:95
local SPELL_RIDE_VEHICLE = 70640 -- boss_deathbringer_saurfang.cpp:117 (outro)
local EMOTE_STATE_DROWNED = 383 -- SharedDefines.h:2021
local SPELL_ZERO_POWER = 72242
local SPELL_BLOOD_LINK = 72178
local SPELL_BLOOD_LINK_DUMMY = 72202 -- boss_deathbringer_saurfang.cpp:107
local SPELL_BLOOD_POWER = 72371
local SPELL_MARK_OF_THE_FALLEN_CHAMPION_S = 72256
local SPELL_MARK_OF_THE_FALLEN_CHAMPION = 72293 -- boss_deathbringer_saurfang.cpp:108
local SPELL_RUNE_OF_BLOOD_S = 72408

local POWER_ENERGY = 3 -- Powers enum (Unit.h): MANA 0, RAGE 1, FOCUS 2, ENERGY 3

local FIGHT_WON_VALUE = 100000 -- boss_deathbringer_saurfang.cpp:614

local timers = {}
local saurfangState = {}
local saurfangLiveBeasts = {} -- boss GUID -> set of live blood-beast GUIDs (ev19/ev20/ev21)
local saurfangIntroDone = {} -- C++ _introDone: set on first engage, never cleared by Reset
local saurfangIntroPhase = {} -- boss GUID -> "A"/"H": C++ events phase mirror for the intro (Reset sets COMBAT; cleared in doSaurfangReset)
local eventNpcPhase = {} -- event-NPC GUID -> "A"/"H": C++ _events phase mirror (START_EVENT guard + scheduler gating); cleared on ev23
local guardSortIndex = {} -- guard GUID -> 1..5: C++ npc_saurfang_event _index (SetData(0, x), :963); guards unregistered, so the index lives here

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

-- bronjahm-style: re-scheduling a key replaces the pending timer, so each
-- named arm has exactly one live timer at a time.
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

local function initState(guid)
    cancelTimers(guid)
    saurfangState[guid] = { frenzied = false, dead = false, fallenChampionCastCount = 0 }
end

local function clearState(guid)
    cancelTimers(guid)
    saurfangState[guid] = nil
    saurfangLiveBeasts[guid] = nil
end

-- C++ SelectTarget(Random, 1, 0.0f, true) (:381): random living player
-- (the file's randomMarkTarget shape, without the mark filter).
local function randomBeastTarget(creature)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:GetObjectType() == "Player" and t:IsAlive() then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ boss_deathbringer_saurfang::JustSummoned (:380-399): random living
-- player target; a target on a transport despawns the beast (1ms) and
-- evades the boss (EnterEvadeMode — the virtual override's SetImmuneToPC
-- leg rides the ev2 handler and JustReachedHome rides ev24); otherwise
-- AttackStart on the target; then triggered self-casts BLOOD_LINK_BEAST
-- 72176 + RESISTANT_SKIN 72723, summons.Summon (the live-beast set
-- below) and DoZoneInCombat. PORTED 2026-11-08 (tick 493). Dormant
-- until the 72172/72173/72356-8 summon spells execute in Go.
local function saurfangJustSummoned(event, creature, summon)
    if summon == nil or summon:GetEntry() ~= ENTRY_BLOOD_BEAST then
        return
    end
    local target = randomBeastTarget(creature)
    if target ~= nil and target:GetTransport() then
        summon:DespawnOrUnsummon(1)
        creature:EnterEvadeMode()
        return
    end
    if target ~= nil then
        summon:AttackStart(target)
    end
    summon:CastSpell(summon, SPELL_BLOOD_LINK_BEAST, true)
    summon:CastSpell(summon, SPELL_RESISTANT_SKIN, true)
    local beasts = saurfangLiveBeasts[creature:GetGUIDLow()]
    if beasts == nil then
        beasts = {}
        saurfangLiveBeasts[creature:GetGUIDLow()] = beasts
    end
    beasts[summon:GetGUIDLow()] = true
    summon:DoZoneInCombat()
end

-- C++ SummonedCreatureDies (:400-404): summons.Despawn — the live-beast
-- set twin, kept in step by the despawn (ev20) and death (ev21) legs
-- (ichoron / bronjahm precedents). PORTED 2026-11-08 (tick 493).
local function saurfangSummonDespawned(_, creature, summon)
    local beasts = saurfangLiveBeasts[creature:GetGUIDLow()]
    if beasts then
        beasts[summon:GetGUIDLow()] = nil
    end
end

local function saurfangSummonDied(_, creature, summon)
    local beasts = saurfangLiveBeasts[creature:GetGUIDLow()]
    if beasts then
        beasts[summon:GetGUIDLow()] = nil
    end
end

-- C++ EVENT_SCENT_OF_BLOOD (:520-526): one-shot, never re-armed; gated
-- on !summons.empty() — the live-beast set above — then Talk
-- (EMOTE_SCENT_OF_BLOOD 14) + DoCastAOE(SPELL_SCENT_OF_BLOOD 72769)
-- self-cast. PORTED 2026-11-08 (tick 493).
local function onScentOfBlood(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :456 (+ in-loop :531, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "scent", 500, function() onScentOfBlood(creature, guid) end)
        return
    end
    local beasts = saurfangLiveBeasts[guid]
    if beasts and next(beasts) ~= nil then
        creature:Talk(EMOTE_SCENT_OF_BLOOD)
        creature:CastSpell(creature, SPELL_SCENT_OF_BLOOD)
    end
end

-- C++ :493-502 EVENT_SUMMON_BLOOD_BEAST: 2x DoCastSelf(
-- SPELL_SUMMON_BLOOD_BEAST+i10 72172/72173) non-triggered, then
-- Talk(SAY_BLOOD_BEASTS 9); re-arm 40s unconditional. The 25-man
-- trio (72356+i25, :495-497) PORTED via creature:Is25ManRaid().
-- The heroic-only EVENT_SCENT_OF_BLOOD schedule (:500-501) PORTS
-- 2026-11-08 (tick 493): scheduled 10s after each beast wave on heroic,
-- gated on the live-beast set in onScentOfBlood below.
local function onSummonBloodBeast(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :456 (+ in-loop :531, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "beasts", 500, function() onSummonBloodBeast(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_SUMMON_BLOOD_BEAST)
    creature:CastSpell(creature, SPELL_SUMMON_BLOOD_BEAST + 1)
    if creature:Is25ManRaid() then
        creature:CastSpell(creature, SPELL_SUMMON_BLOOD_BEAST_25_MAN)
        creature:CastSpell(creature, SPELL_SUMMON_BLOOD_BEAST_25_MAN + 1)
        creature:CastSpell(creature, SPELL_SUMMON_BLOOD_BEAST_25_MAN + 2)
    end
    creature:Talk(SAY_BLOOD_BEASTS)
    schedule(guid, "beasts", 40000, function() onSummonBloodBeast(creature, guid) end)
    if creature:IsHeroic() then
        schedule(guid, "scent", 10000, function() onScentOfBlood(creature, guid) end)
    end
end

-- C++ :516-519 EVENT_BERSERK: DoCastSelf(SPELL_BERSERK 26662)
-- non-triggered + Talk(SAY_BERSERK 12), cast-before-Talk kept;
-- one-shot. Seed IsHeroic() ? 360000 : 480000 (:305) rides
-- creature:IsHeroic() (engine/world/lua_creature_events.go:260, the
-- cyanigosa mana-destruction precedent).
local function onBerserk(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :456 (+ in-loop :531, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "berserk", 500, function() onBerserk(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_BERSERK)
    creature:Talk(SAY_BERSERK)
end

-- C++ :512-515 EVENT_BOILING_BLOOD: DoCastSelf(SPELL_BOILING_BLOOD
-- 72385) non-triggered; re-arm {15s,20s} randtime.
local function onBoilingBlood(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :456 (+ in-loop :531, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "boiling", 500, function() onBoilingBlood(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_BOILING_BLOOD)
    schedule(guid, "boiling", {15000, 20000}, function() onBoilingBlood(creature, guid) end)
end

-- C++ :504-507 EVENT_BLOOD_NOVA: DoCastAOE(SPELL_BLOOD_NOVA_TRIGGER
-- 72378) = non-triggered self-cast; re-arm {20s,25s} randtime.
local function onBloodNova(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :456 (+ in-loop :531, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "nova", 500, function() onBloodNova(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_BLOOD_NOVA_TRIGGER)
    schedule(guid, "nova", {20000, 25000}, function() onBloodNova(creature, guid) end)
end

-- C++ :508-511 EVENT_RUNE_OF_BLOOD: DoCastVictim(SPELL_RUNE_OF_BLOOD
-- 72410) = non-triggered victim-cast; re-arm {20s,25s} randtime.
local function onRuneOfBlood(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :456 (+ in-loop :531, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "rune", 500, function() onRuneOfBlood(creature, guid) end)
        return
    end
    creature:CastSpell(nil, SPELL_RUNE_OF_BLOOD)
    schedule(guid, "rune", {20000, 25000}, function() onRuneOfBlood(creature, guid) end)
end

-- C++ :275-313 JustEngagedWith: Talk(SAY_AGGRO 7) (:302), then the
-- five combat scheduler seeds (:304-308). The CheckRequiredBosses
-- evade branch and DoCastSpellOnPlayers legs have no instance bridge
-- (DoZoneInCombat :289 is PORTED 2026-10-23, C++ arm order);
-- instance->SetBossState(IN_PROGRESS) (:312) PORTED 2026-11-05 via
-- creature:SetBossState, last in the handler.
-- the !_introDone DoCastSelf(SPELL_GRIP_OF_AGONY) exploit leg
-- (:293-298) rides the unbridged intro DoAction machine.
-- C++ JustEngagedWith starts with `if (_dead) return;` (:277-278): after
-- the fight-won leg the whole engage sequence (Talk, seeds, reset
-- casts) is skipped. The check must come BEFORE initState, which
-- would otherwise reset dead=false.
-- C++ npc_high_overlord_saurfang_icc/npc_muradin_bronzebeard_icc DoAction(ACTION_START_EVENT) :662/:863
-- (*itr)->AI()->SetData(0, x): sort-index write into the unported npc_saurfang_event guard AIs (whose
-- SetData override is :963). No cross-AI SetData bridge, targets unported — dropped, documented.
-- Cross-AI DoAction relay arms (cross_doaction_relay census 2026-11-03):
-- the event NPC is the faction spawn (horde 37187 / alliance 37200);
-- only one exists per instance, so trying both entries is C++-exact
-- for the instance-GUID lookup. The event NPC has no Lua handler, so
-- its own _events.Reset leg is a no-op and its timer legs port only
-- as the Talk/despawn arms below.
local function findEventNpc(creature)
    local npc = creature:FindNearestCreature(ENTRY_SE_HIGH_OVERLORD_SAURFANG, 100.0)
    if npc then
        return npc, true
    end
    npc = creature:FindNearestCreature(ENTRY_SE_MURADIN_BRONZEBEARD, 100.0)
    if npc then
        return npc, false
    end
    return nil, false
end

-- C++ GuardBroadcast (event-NPC-centered, 100.0f): both factions'
-- guard entries swept — only the live faction's guards exist.
local function despawnEventGuards(eventNpc)
    for _, guard in ipairs(eventNpc:GetCreaturesInGrid(ENTRY_SE_KOR_KRON_REAVER, 100)) do
        guard:Despawn()
    end
    for _, guard in ipairs(eventNpc:GetCreaturesInGrid(ENTRY_SE_SKYBREAKER_MARINE, 100)) do
        guard:Despawn()
    end
end

-- C++ JustEngagedWith :293-298: first engage only. The latch lives
-- outside saurfangState (which resets on ev2/ev23) because C++ Reset
-- never clears _introDone.
local function relayInterruptIntro(creature, guid)
    if saurfangIntroDone[guid] then
        return
    end
    saurfangIntroDone[guid] = true
    creature:CastSpell(creature, SPELL_GRIP_OF_AGONY)
    local eventNpc = findEventNpc(creature)
    if eventNpc then
        -- C++ ACTION_INTERRUPT_INTRO (:688-695/:897-902): _events.Reset()
        -- cancels the NPC's pending intro timers before the guard despawn.
        local npcGuid = eventNpc:GetGUIDLow()
        cancelTimers(npcGuid)
        eventNpcPhase[npcGuid] = nil
        despawnEventGuards(eventNpc)
    end
end

-- C++ DamageTaken :375-376 -> event-NPC DoAction(ACTION_START_OUTRO).
-- Horde (:673-685): RemoveAura(GRIP) + Talk(OUTRO_1 11) + Talk(OUTRO_2
-- 12) at 10s + Talk(OUTRO_3 13) at 18s + SetDisableGravity(false) +
-- MoveFall + guard despawn; OUTRO_4 (24s corpse move, GetClosePoint via
-- deathbringer:GetRelativePoint(0, 0)) + OUTRO_5 (30s final move) +
-- OUTRO_6 (Talk OUTRO_4 14, 4s after the POINT_CORPSE arrival) PORTED
-- this tick; the POINT_CORPSE arrival (ride-vehicle cast, NOT_SELECTABLE
-- flag, DROWNED emote state) + POINT_FINAL arrival (both despawn) ride
-- eventNpcMovementInform below.
-- Alliance (muradin :873): RemoveAura(GRIP) + Talk(OUTRO_1 3) +
-- SetDisableGravity(false) + MoveFall + guard despawn + deathbringer
-- DespawnOrUnsummon(5s); muradin schedules no
-- outro Talks (its chain rides waypoint/movement events, no bridge).
-- Delayed legs ride schedule() keyed on the event NPC's GUID (C++
-- ownership: the timers live on the event NPC's EventMap).
local function relayStartOutro(creature)
    local eventNpc, isHorde = findEventNpc(creature)
    if not eventNpc then
        return
    end
    eventNpc:RemoveAura(SPELL_GRIP_OF_AGONY)
    local npcGuid = eventNpc:GetGUIDLow()
    if isHorde then
        eventNpc:Talk(SAY_OUTRO_HORDE_1)
        schedule(npcGuid, "outro2", 10000, function()
            eventNpc:Talk(SAY_OUTRO_HORDE_2)
        end)
        schedule(npcGuid, "outro3", 18000, function()
            eventNpc:Talk(SAY_OUTRO_HORDE_3)
        end)
        -- C++ :679-680: OUTRO_4 at 24s (corpse move) + OUTRO_5 at 30s
        -- (final move); the POINT_CORPSE arrival re-schedules outro5 at
        -- 1s (schedule() key-cancel = C++ RescheduleEvent, :735) and
        -- outro6 at 4s (:736).
        schedule(npcGuid, "outro4", 24000, function()
            local deathbringer = eventNpc:FindNearestCreature(ENTRY_DEATHBRINGER_SAURFANG, 100)
            if deathbringer then
                local x, y, z = deathbringer:GetRelativePoint(0, 0)
                eventNpc:SetWalk(true)
                eventNpc:MoveTo(POINT_CORPSE, x, y, z)
            end
        end)
        schedule(npcGuid, "outro5", 30000, function()
            eventNpc:MoveTo(POINT_FINAL, FINAL_X, FINAL_Y, FINAL_Z)
        end)
        -- C++ :681-682: SetDisableGravity(false) + MoveFall, after the
        -- schedules (both bridged now — the "no MoveFall bridge" note is stale).
        eventNpc:SetDisableGravity(false)
        eventNpc:MoveFall()
    else
        eventNpc:Talk(SAY_OUTRO_ALLIANCE_1)
        -- C++ muradin :876-877: SetDisableGravity(false) + MoveFall.
        eventNpc:SetDisableGravity(false)
        eventNpc:MoveFall()
        schedule(npcGuid, "outroDespawn", 5000, function()
            creature:Despawn()
        end)
    end
    despawnEventGuards(eventNpc)
end

local function saurfangJustEngagedWith(event, creature, target)
    local guid = creature:GetGUIDLow()
    local st = saurfangState[guid]
    if st and st.dead then
        return
    end
    initState(guid)
    -- C++ :289 — me-form DoZoneInCombat after me->setActive(true), before
    -- events.Reset/SetPhase and SetImmuneToPC(false) (C++ arm order).
    creature:DoZoneInCombat()
    creature:RemoveFlag(59, 0x100) -- C++ :293 (JustEngagedWith SetImmuneToPC(false) = clear UNIT_FLAG_IMMUNE_TO_PC, after events.Reset/SetPhase)
    -- C++ :293-298: first-engage branch in C++ arm order (GRIP cast,
    -- then ACTION_INTERRUPT_INTRO relay) before Talk(SAY_AGGRO).
    relayInterruptIntro(creature, guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "beasts", 30000, function() onSummonBloodBeast(creature, guid) end)
    -- C++ :305 seed: IsHeroic() ? 360s : 480s rides creature:IsHeroic()
    schedule(guid, "berserk", creature:IsHeroic() and 360000 or 480000, function() onBerserk(creature, guid) end)
    schedule(guid, "boiling", 15500, function() onBoilingBlood(creature, guid) end)
    schedule(guid, "nova", 17000, function() onBloodNova(creature, guid) end)
    schedule(guid, "rune", 20000, function() onRuneOfBlood(creature, guid) end)
    -- C++ JustEngagedWith (:310-313): _fallenChampionCastCount = 0
    -- (initState above zeroes the Lua latch) + instance->
    -- DoRemoveAurasDueToSpellOnPlayers(SPELL_MARK_OF_THE_FALLEN_
    -- CHAMPION 72293) via GetPlayersInWorld + mapId/instanceId filter
    -- + player:RemoveAura (the moroes precedent; the stale "no
    -- instance bridge" claim is retracted); the
    -- instance->SetBossState(DATA_DEATHBRINGER_SAURFANG, IN_PROGRESS)
    -- leg has no instance bridge.
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId then
            p:RemoveAura(SPELL_MARK_OF_THE_FALLEN_CHAMPION)
        end
    end
    -- C++ :312 — SetBossState IN_PROGRESS, last in JustEngagedWith,
    -- PORTED 2026-11-05 via creature:SetBossState
    -- (lua_creature_events.go:5917).
    -- C++ GetData(DATA_MADE_A_MESS) (:540-546) returns 1 while
    -- _fallenChampionCastCount < 3/5 (10/25-man); the count is 0 here,
    -- so the latch starts set — the Go achievement gate reads this
    -- slot (bossLatchData, achievement_criteria_scripts.go).
    creature:SetData(DATA_MADE_A_MESS, 1)
    creature:SetBossState(DATA_DEATHBRINGER_SAURFANG, BOSS_STATE_IN_PROGRESS)
end

local function saurfangLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
    -- C++ boss_deathbringer_saurfang::EnterEvadeMode (:327-331):
    -- ScriptedAI base + SetImmuneToPC(false) when _introDone — the
    -- UNIT_FLAG_IMMUNE_TO_PC (0x100) clear twin, already used in the
    -- engage handler (:293). Gated on the intro latch AND not-dead:
    -- the fight-won path calls _EnterEvadeMode() (non-virtual), which
    -- bypasses this virtual override, so a won fight stays immune.
    local guid = creature:GetGUIDLow()
    local st = saurfangState[guid]
    if saurfangIntroDone[guid] and not (st and st.dead) then
        creature:RemoveFlag(59, 0x100)
    end
end

-- C++ :257-273 Reset: no timer seeds in C++, so event 23 only
-- cancels timers and replays the bridgeable aura-setup arms — the
-- five TRIGGERED self-casts (:264-268) and the
-- RemoveAurasDueToSpell(SPELL_BERSERK / SPELL_FRENZY) removals via
-- the RemoveAura bridge (the arlokk / bronjahm idiom). Mirrors the
-- `if (_dead) return;` early-out (:277-278): after the fight-won
-- leg the reset casts are skipped.
-- C++ boss_deathbringer_saurfang::Reset() legs (:257-273), shared by the
-- event-23 Reset arm and the JustReachedHome arm (which calls Reset() in
-- C++ arm order).
local function doSaurfangReset(creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    saurfangIntroPhase[guid] = nil
    local st = saurfangState[guid]
    if st and st.dead then
        return
    end
    -- C++ Reset() :261 — _Reset() first (after the _dead guard):
    -- SetBossState NOT_STARTED only when the stored state != DONE
    -- (BossAI::_Reset, ScriptedCreature.cpp:499-509; the falric
    -- precedent — ev23 fires on spawn/death/evade too, and a DONE
    -- boss must stay DONE). PORTED 2026-11-05 via creature:SetBossState.
    -- Shared with the reach-home arm, where C++ calls Reset() before
    -- writing FAIL — the NOT_STARTED-then-FAIL order is C++-exact.
    if creature:GetBossState(DATA_DEATHBRINGER_SAURFANG) ~= BOSS_STATE_DONE then
        creature:SetBossState(DATA_DEATHBRINGER_SAURFANG, BOSS_STATE_NOT_STARTED)
    end
    -- C++ Reset() never clears _fallenChampionCastCount (only
    -- JustEngagedWith :310 zeroes it), so the count survives a reset
    -- exactly as the C++ member does.
    local oldCount = st and st.fallenChampionCastCount or 0
    saurfangState[guid] = { frenzied = false, dead = false, fallenChampionCastCount = oldCount }
    -- C++ Reset() :261 — _Reset() first: summons.DespawnAll()
    -- (BossAI::_Reset, ScriptedCreature.cpp:499-509) empties the summon
    -- list, so the live-beast set clears here too; without it a stale
    -- GUID from the wiped fight would let a later scent check fire
    -- after the new wave's beasts all died.
    saurfangLiveBeasts[guid] = nil
    -- C++ Reset() :265: me->SetPower(POWER_ENERGY, 0) — the blood-power
    -- meter zeroes on reset (SPELL_ZERO_POWER's aura scripts are
    -- unbridged; the bridge writes the live power field).
    creature:SetPower(POWER_ENERGY, 0)
    -- C++ Reset(): DoCastSelf(ZERO_POWER/BLOOD_LINK/BLOOD_POWER/
    -- MARK_OF_THE_FALLEN_CHAMPION_S/RUNE_OF_BLOOD_S) are Reset-only
    -- (boss_deathbringer_saurfang.cpp). Alive-gated; the timer cancel and
    -- state reset ride the death leg deliberately.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_ZERO_POWER, true)
        creature:CastSpell(creature, SPELL_BLOOD_LINK, true)
        creature:CastSpell(creature, SPELL_BLOOD_POWER, true)
        creature:CastSpell(creature, SPELL_MARK_OF_THE_FALLEN_CHAMPION_S, true)
        creature:CastSpell(creature, SPELL_RUNE_OF_BLOOD_S, true)
    end
    creature:RemoveAura(SPELL_BERSERK)
    creature:RemoveAura(SPELL_FRENZY)
end

local function saurfangReset(event, creature)
    doSaurfangReset(creature)
end

-- C++ :344-348 KilledUnit: if (victim->GetTypeId() == TYPEID_PLAYER)
-- Talk(SAY_KILL) — the razuvious player-gated variant precedent.
local function saurfangKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

-- C++ :315-317 JustDied is EMPTY — event 4 only cancels timers and
-- clears state (the sjonnir JustDied-Talk precedent does NOT apply;
-- SAY_DEATH 13 rides the DamageTaken fight-won leg below).
local function saurfangJustDied(event, creature)
    clearState(creature:GetGUIDLow())
end

-- C++ JustReachedHome (:334-342): the _dead guard; _JustReachedHome()
-- (events.Reset) is the cancelTimers inside doSaurfangReset; Reset()
-- ports via doSaurfangReset in C++ arm order; then SetBossState
-- (DATA_DEATHBRINGER_SAURFANG 3, FAIL 2 — bridged, moroes precedent);
-- then DoRemoveAurasDueToSpellOnPlayers(
-- SPELL_MARK_OF_THE_FALLEN_CHAMPION 72293) via GetPlayersInWorld +
-- mapId/instanceId filter + player:RemoveAura (the moroes precedent).
local function saurfangReachHome(event, creature)
    local guid = creature:GetGUIDLow()
    local st = saurfangState[guid]
    if st and st.dead then
        return
    end
    doSaurfangReset(creature)
    creature:SetBossState(DATA_DEATHBRINGER_SAURFANG, BOSS_STATE_FAIL) -- DATA_DEATHBRINGER_SAURFANG, FAIL (tick 236)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    for _, plr in ipairs(GetPlayersInWorld()) do
        if plr:GetMapId() == mapId and plr:GetInstanceId() == instanceId then
            plr:RemoveAura(SPELL_MARK_OF_THE_FALLEN_CHAMPION)
        end
    end
end

-- C++ :350-378 DamageTaken, fires pre-application (event 9). The
-- never-die cap (:351-352, damage = health-1 when damage >= health)
-- rewrites via `return true, newDamage` (the gunship
-- fireCreatureDamageTaken precedent). The frenzy latch (:355-360):
-- !_frenzied && HealthBelowPct(31) — STRICTLY current-health < 31%
-- of max (Unit.h:872), not post-damage — DoCastSelf(SPELL_FRENZY
-- 72737, true) TRIGGERED + Talk(SAY_FRENZY 11). The fight-won leg
-- (:362-377): !_dead && health-damage < FightWonValue (100000,
-- computed with the CAPPED damage) — cancel timers (the _JustDied
-- events.Reset), DoCastAOE(SPELL_REMOVE_MARKS_OF_THE_FALLEN_CHAMPION
-- 72257) = non-triggered self-cast, DoCastSelf(SPELL_ACHIEVEMENT
-- 72928, true) TRIGGERED, Talk(SAY_DEATH 13), DoCastSelf(
-- SPELL_REPUTATION_BOSS_KILL 73843, true) TRIGGERED, DoCastSelf(
-- SPELL_PERMANENT_FEIGN_DEATH 70628). C++ order kept (frenzy check
-- before fight-won; both may fire on one hit). The _EnterEvadeMode /
-- SetFlag(NOT_SELECTABLE) / RemoveAurasOnEvade legs and the cross-AI
-- DoAction(ACTION_START_OUTRO) leg ports via relayStartOutro below
-- (dispatch IS bridged — creature:DoAction fires ev38); the SetImmuneToPC(true)
-- (:368) leg is ported flag-only above via the SetFlag(59, 0x100) twin.
local function saurfangDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUIDLow()
    local st = saurfangState[guid]
    if not st then
        -- C++ _frenzied/_dead default false; a DamageTaken before any
        -- engage/reset handler must still apply the never-die cap.
        st = { frenzied = false, dead = false }
        saurfangState[guid] = st
    end
    local health = creature:GetHealth()
    local maxHealth = creature:GetMaxHealth()
    local newDamage = damage
    if damage >= health then
        newDamage = health - 1
    end
    if not st.frenzied and maxHealth ~= 0
            and health < math.floor(maxHealth * 31 / 100) then
        st.frenzied = true
        creature:CastSpell(creature, SPELL_FRENZY, true)
        creature:Talk(SAY_FRENZY)
    end
    if not st.dead and health - newDamage < FIGHT_WON_VALUE then
        st.dead = true
        cancelTimers(guid)
        -- C++ :364 — _JustDied() first in the fight-won block:
        -- SetBossState DONE PORTED 2026-11-05 via creature:SetBossState,
        -- before the flags/casts/Talk below (C++ order).
        creature:SetBossState(DATA_DEATHBRINGER_SAURFANG, BOSS_STATE_DONE)
        -- C++ :368-369 arm order: SetFlag(UNIT_FLAG_NOT_SELECTABLE) then
        -- SetImmuneToPC(true) (= set UNIT_FLAG_IMMUNE_TO_PC); the NOT_SELECTABLE
        -- leg ports 2026-10-23 on the live creature:SetFlag bridge (the stale
        -- "no bridge" claim is retracted); the _EnterEvadeMode /
        -- RemoveAurasOnEvade legs have no bridge; the cross-AI
        -- DoAction(ACTION_START_OUTRO) leg ports via relayStartOutro
        -- below (FindNearestCreature inline, maulgar/illidan precedent).
        creature:SetFlag(59, 0x2000000)
        creature:SetFlag(59, 0x100)
        creature:CastSpell(creature, SPELL_REMOVE_MARKS_OF_THE_FALLEN_CHAMPION)
        creature:CastSpell(creature, SPELL_ACHIEVEMENT, true)
        creature:Talk(SAY_DEATH)
        creature:CastSpell(creature, SPELL_REPUTATION_BOSS_KILL, true)
        creature:CastSpell(creature, SPELL_PERMANENT_FEIGN_DEATH)
        -- C++ :375-376: ACTION_START_OUTRO relay to the faction event
        -- NPC, last in the fight-won arm order, after feign death.
        relayStartOutro(creature)
    end
    if newDamage ~= damage then
        return true, newDamage
    end
end

-- C++ spell_deathbringer_blood_link_aura::HandlePeriodicTick
-- (boss_deathbringer_saurfang.cpp:1024-1032): OnEffectPeriodic,
-- EFFECT_1 SPELL_AURA_PERIODIC_DUMMY, PreventDefaultAction first,
-- then the power gate — owner power type ENERGY and power == max
-- power — calls the owner's DoAction(ACTION_MARK_OF_THE_FALLEN_
-- CHAMPION). The aura is SPELL_BLOOD_LINK 72178, self-cast by
-- Saurfang at Reset (:267); the AuraScript loader name binds to
-- that spell (the SpellScript twin hooks EFFECT_0 DUMMY of the
-- same 72178 cast), the fetch-ingredient convention. ev1 args
-- (event, aura, target); target is the aura owner, built fresh per
-- tick (luaUnitObjectForGUID), so GetPower/GetMaxPower/GetPowerType
-- read live motion fields via the object.go fallback. Returns true,
-- the PreventDefaultAction twin. DORMANT until Spell.dbc ships (the
-- 72178 cast at Reset is ported; the aura application is DBC-side).
local function bloodLinkTick(event, aura, target)
    if target:GetPowerType() == POWER_ENERGY
            and target:GetPower(POWER_ENERGY) == target:GetMaxPower(POWER_ENERGY) then
        target:DoAction(ACTION_MARK_OF_THE_FALLEN_CHAMPION)
    end
    return true
end

-- C++ SelectTarget(Random, 1, 0.0f, true, true, -SPELL_MARK_OF_THE_
-- FALLEN_CHAMPION) (:582): random threat-list player, alive, not
-- carrying the mark; dist 0.0f = no range filter. The moragg
-- randomPlayerInRange idiom (GetAITargets + IsAlive), plus the
-- aura-exclusion filter (player:HasAura, lua_player.go).
local function randomMarkTarget(creature)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:GetObjectType() == "Player" and t:IsAlive()
                and not t:HasAura(SPELL_MARK_OF_THE_FALLEN_CHAMPION) then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ boss_deathbringer_saurfang::DoAction ACTION_MARK_OF_THE_FALLEN_
-- CHAMPION (:580-592): random mark target (nil = whole arm skipped,
-- no count increment) -> ++_fallenChampionCastCount -> DoCast(target,
-- SPELL_MARK_OF_THE_FALLEN_CHAMPION 72293) non-triggered -> SetPower(
-- ENERGY, 0). The blood-power RecalculateAmountOfEffects leg (:591)
-- stays out: no RecalculateAmountOfEffects bridge (the recalc only
-- rescales the 72371 aura amount; the mark cast + power zero are the
-- functional legs). The DATA_MADE_A_MESS latch (:540-546, threshold
-- RAID_MODE 3/5) clears here via creature:Is25ManRaid() once the
-- count reaches it; the Go achievement gate reads this slot.
-- C++ boss_deathbringer_saurfang::DoAction PHASE_INTRO_A (1) /
-- PHASE_INTRO_H (2) (:548-565): events.SetPhase (mirrored by
-- saurfangIntroPhase) + SetHomePosition(deathbringerPos, live
-- orientation via GetO — the object.go fallback) + MovePoint(
-- POINT_SAURFANG 3781300, via the bridged MoveTo twin) + faction
-- intro schedules (ALLIANCE_2 2.5s / _3 20s; HORDE_2 5s). The
-- EVENT_INTRO_* bodies (:463-492) port as the schedule closures:
-- ALLIANCE_2/HORDE_2 RemoveFlag(NOT_SELECTABLE) + SetFaction(974) +
-- Talk; ALLIANCE_3 Talk; HORDE_2 Talk.
local function saurfangIntroPhaseAction(creature, guid, action)
    saurfangIntroPhase[guid] = action == PHASE_INTRO_A and "A" or "H"
    creature:SetHomePosition(DEATHBRINGER_X, DEATHBRINGER_Y, DEATHBRINGER_Z, creature:GetO())
    creature:MoveTo(POINT_SAURFANG, DEATHBRINGER_X, DEATHBRINGER_Y, DEATHBRINGER_Z)
    if action == PHASE_INTRO_A then
        schedule(guid, "introA2", 2500, function()
            creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
            creature:SetFaction(FACTION_UNDEAD_SCOURGE)
            creature:Talk(SAY_INTRO_ALLIANCE_2)
        end)
        schedule(guid, "introA3", 20000, function()
            creature:Talk(SAY_INTRO_ALLIANCE_3)
        end)
    else
        schedule(guid, "introH2", 5000, function()
            creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
            creature:SetFaction(FACTION_UNDEAD_SCOURGE)
            creature:Talk(SAY_INTRO_HORDE_2)
        end)
    end
end

-- C++ ACTION_CONTINUE_INTRO (-3781301, :567-579): _introDone-gated
-- schedules (ALLIANCE_6 7s / FINISH 8s; HORDE_4 6.5s / HORDE_9 48.2s
-- / FINISH 55.7s). Only the live faction's events schedule — the C++
-- phase gates on the other faction's are vacuous. FINISH (:488-492):
-- SetPhase(PHASE_COMBAT) + _introDone (saurfangIntroDone — set here
-- so a post-intro engage skips the GRIP/interrupt leg exactly as C++
-- :293-298) + SetImmuneToPC(false) (RemoveFlag(59, 0x100) twin).
-- C++ EVENT_INTRO_ALLIANCE_6 (:471-475): Talk x2 + DoCastSelf(GRIP_OF_AGONY);
-- EVENT_INTRO_HORDE_9 (:484-487): DoCastSelf(GRIP) + Talk. Both ride the
-- boss UpdateAI pre-loop gate :456 — 500ms IsCasting defer (mal_ganis idiom).
local function onIntroA6(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "introA6", 500, function() onIntroA6(creature, guid) end)
        return
    end
    creature:Talk(SAY_INTRO_ALLIANCE_6)
    creature:Talk(SAY_INTRO_ALLIANCE_7)
    creature:CastSpell(creature, SPELL_GRIP_OF_AGONY)
end

local function onIntroH9(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "introH9", 500, function() onIntroH9(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_GRIP_OF_AGONY)
    creature:Talk(SAY_INTRO_HORDE_9)
end

-- ALLIANCE_6: Talk x2 + GRIP self-cast; HORDE_9: GRIP + Talk.
local function saurfangContinueIntroAction(creature, guid)
    if saurfangIntroDone[guid] then
        return
    end
    local function onIntroFinish()
        saurfangIntroDone[guid] = true
        creature:RemoveFlag(UNIT_FIELD_FLAGS, 0x100)
    end
    if saurfangIntroPhase[guid] == "A" then
        schedule(guid, "introA6", 7000, function() onIntroA6(creature, guid) end)
        schedule(guid, "introFinish", 8000, onIntroFinish)
    elseif saurfangIntroPhase[guid] == "H" then
        schedule(guid, "introH4", 6500, function()
            creature:Talk(SAY_INTRO_HORDE_4)
        end)
        schedule(guid, "introH9", 48200, function() onIntroH9(creature, guid) end)
        schedule(guid, "introFinish", 55700, onIntroFinish)
    end
end

local function saurfangDoAction(event, creature, action)
    local guid = creature:GetGUIDLow()
    if action == PHASE_INTRO_A or action == PHASE_INTRO_H then
        saurfangIntroPhaseAction(creature, guid, action)
        return
    end
    if action == ACTION_CONTINUE_INTRO then
        saurfangContinueIntroAction(creature, guid)
        return
    end
    if action ~= ACTION_MARK_OF_THE_FALLEN_CHAMPION then
        return
    end
    local target = randomMarkTarget(creature)
    if not target then
        return
    end
    local st = saurfangState[guid]
    if not st then
        st = { frenzied = false, dead = false, fallenChampionCastCount = 0 }
        saurfangState[guid] = st
    end
    st.fallenChampionCastCount = (st.fallenChampionCastCount or 0) + 1
    creature:CastSpell(target, SPELL_MARK_OF_THE_FALLEN_CHAMPION)
    creature:SetPower(POWER_ENERGY, 0)
    local threshold = creature:Is25ManRaid() and 5 or 3
    if st.fallenChampionCastCount >= threshold then
        creature:SetData(DATA_MADE_A_MESS, 0)
    end
end

-- C++ boss_deathbringer_saurfang::SpellHitTarget (:413-435): case
-- SPELL_MARK_OF_THE_FALLEN_CHAMPION 72293 -> Talk(SAY_MARK_OF_THE_
-- FALLEN_CHAMPION 8); cases 72255/72444/72445/72446 (triggered-mark
-- ids) -> power != max -> target casts SPELL_BLOOD_LINK_DUMMY 72202
-- on itself, TRIGGERED, BP0=1 (the geddon CastCustomSpell(nil, ...)
-- precedent). The transport gate (:415-419, EnterEvadeMode) PORTS
-- 2026-11-08 (tick 493, below); the old "EnterEvadeMode has no bridge"
-- note is stale (bridged Nov 7, lua_creature_events.go:5014). The 72255
-- leg is DORMANT
-- until Spell.dbc ships (the triggered-mark ids are DBC-side).
local function saurfangSpellHitTarget(event, creature, target, spellId)
    -- C++ SpellHitTarget transport gate (:415-419): a target on a
    -- transport evades the boss (EnterEvadeMode — the virtual override's
    -- legs ride the ev2/ev24 handlers). PORTED 2026-11-08 (tick 493).
    -- Player-typed: GetTransport lives on the player surface only
    -- (lua_player.go:2825); C++ GetTransport() is nil for non-players,
    -- and the mark spells only hit players.
    if target ~= nil and target:GetObjectType() == "Player" and target:GetTransport() then
        creature:EnterEvadeMode()
        return
    end
    if spellId == SPELL_MARK_OF_THE_FALLEN_CHAMPION then
        creature:Talk(SAY_MARK_OF_THE_FALLEN_CHAMPION)
    elseif spellId == 72255 or spellId == 72444 or spellId == 72445 or spellId == 72446 then
        if target and creature:GetPower(POWER_ENERGY) ~= creature:GetMaxPower(POWER_ENERGY) then
            target:CastCustomSpell(nil, SPELL_BLOOD_LINK_DUMMY, true, 1)
        end
    end
end

-- C++ GuardBroadcast (event-NPC-centered, 100.0f, :644-649/:842-847):
-- every guard gets DoAction(ACTION_CHARGE); the guard AI gates on
-- _index (npc_saurfang_event :978-980) and charges to chargePos[_index]
-- at 13.0. Guards are unregistered, so the _index lives in
-- guardSortIndex and the broadcast calls MoveCharge directly.
local function chargeEventGuards(eventNpc, isHorde)
    local guardEntry = isHorde and ENTRY_SE_KOR_KRON_REAVER or ENTRY_SE_SKYBREAKER_MARINE
    for _, guard in ipairs(eventNpc:GetCreaturesInGrid(guardEntry, 100)) do
        local idx = guardSortIndex[guard:GetGUIDLow()]
        if idx then
            local p = CHARGE_POS[idx + 1]
            guard:MoveCharge(POINT_CHARGE, p[1], p[2], p[3], 13.0)
        end
    end
end

-- C++ npc_high_overlord_saurfang_icc / npc_muradin_bronzebeard_icc
-- DoAction(ACTION_START_EVENT) (:651-670/:851-870). Horde order:
-- phase guard -> guard sort + SetData -> gossip-flag clear -> Talk
-- INTRO_1 -> SetPhase -> schedule INTRO_HORDE_3 (18.5s) -> door ->
-- deathbringer DoAction(PHASE_INTRO_H). Alliance order: phase guard
-- -> SetPhase -> guard sort + SetData -> gossip-flag clear -> Talk
-- INTRO_1 -> schedule INTRO_ALLIANCE_4 (29.5s) -> door -> deathbringer
-- DoAction(PHASE_INTRO_A). The door leg (_instance->HandleGameObject(
-- GO_SAURFANG_S_DOOR 201825, true)) stays out: no instance-GO bridge.
-- The deathbringer lookup is the FindNearestCreature twin of the
-- instance GUID ref (mimiron precedent, 100yd like findEventNpc).
local function eventNpcStartEvent(creature)
    local guid = creature:GetGUIDLow()
    local isHorde = creature:GetEntry() == ENTRY_SE_HIGH_OVERLORD_SAURFANG
    if eventNpcPhase[guid] == "A" or eventNpcPhase[guid] == "H" then
        return
    end
    local guardEntry = isHorde and ENTRY_SE_KOR_KRON_REAVER or ENTRY_SE_SKYBREAKER_MARINE
    local function sortGuards()
        local guards = creature:GetCreaturesInGrid(guardEntry, 20)
        table.sort(guards, function(a, b) return creature:GetDistance(a) < creature:GetDistance(b) end)
        for x, guard in ipairs(guards) do
            if x < 6 then -- C++ SetData gate: type 0, data 1..5 (:963-968)
                guardSortIndex[guard:GetGUIDLow()] = x
            end
        end
    end
    local function clearGossipFlag()
        -- C++ RemoveFlag(UNIT_NPC_FLAGS, UNIT_NPC_FLAG_GOSSIP) — the
        -- SetNPCFlags twin (zulaman/barada precedent).
        creature:SetNPCFlags(bit32.band(creature:GetNPCFlags(), bit32.bnot(UNIT_NPC_FLAG_GOSSIP)))
    end
    local function walkToFirstStep(key, delay)
        schedule(guid, key, delay, function()
            creature:SetWalk(true)
            creature:MoveTo(POINT_FIRST_STEP, FIRST_STEP_X, FIRST_STEP_Y, FIRST_STEP_Z)
        end)
    end
    local function sendPhaseIntro()
        local deathbringer = creature:FindNearestCreature(ENTRY_DEATHBRINGER_SAURFANG, 100)
        if deathbringer then
            deathbringer:DoAction(isHorde and PHASE_INTRO_H or PHASE_INTRO_A)
        end
    end
    if isHorde then
        sortGuards()
        clearGossipFlag()
        creature:Talk(SAY_H_INTRO_1)
        eventNpcPhase[guid] = "H"
        walkToFirstStep("introH3", 18500)
    else
        eventNpcPhase[guid] = "A"
        sortGuards()
        clearGossipFlag()
        creature:Talk(SAY_A_INTRO_1)
        walkToFirstStep("introA4", 29500)
    end
    sendPhaseIntro()
end

-- C++ OnGossipSelect (:629-636/:829-836): menuId gate (10952 horde /
-- 10934 alliance) -> CloseGossipMenuFor + DoAction(ACTION_START_EVENT);
-- returns false. The Go hook forwards listID (gossipListId), NOT
-- menuId, and C++ ignores gossipListId — so any select on this entry's
-- menu starts the event (the menu is DB-side, single-option). Close +
-- return-false order follows the barada precedent.
local function eventNpcGossipSelect(event, player, creature, sender, action, code, listID)
    player:GossipClearMenu()
    player:GossipComplete()
    eventNpcStartEvent(creature)
    return false
end

-- C++ npc_high_overlord_saurfang_icc / npc_muradin_bronzebeard_icc
-- MovementInform POINT_FIRST_STEP (:708-726/:906-916): SetWalk(false)
-- + Talk INTRO_3/4 + HORDE_5/6/7/8 (15.5s/29.5s/43.8s/47s) or
-- ALLIANCE_5 (5s) schedules + deathbringer DoAction(
-- ACTION_CONTINUE_INTRO). HORDE_8 / ALLIANCE_5 also broadcast
-- ACTION_CHARGE (guards + self MoveCharge to chargePos[0] at 8.5).
-- MovementInform POINT_CORPSE (:728-737) + POINT_FINAL (:738-741):
-- corpse arrival (ride-vehicle cast, NOT_SELECTABLE flag, DROWNED
-- emote state, OUTRO_5 at 1s + OUTRO_6 at 4s) and final arrival
-- (deathbringer + self despawn).
local function eventNpcMovementInform(event, creature, motionType, id)
    if motionType ~= POINT_MOTION_TYPE then
        return
    end
    local guid = creature:GetGUIDLow()
    if id == POINT_CORPSE then
        local deathbringer = creature:FindNearestCreature(ENTRY_DEATHBRINGER_SAURFANG, 100)
        if deathbringer then
            deathbringer:CastSpell(creature, SPELL_RIDE_VEHICLE, true)
            deathbringer:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
            deathbringer:EmoteState(EMOTE_STATE_DROWNED)
        end
        schedule(guid, "outro5", 1000, function()
            creature:MoveTo(POINT_FINAL, FINAL_X, FINAL_Y, FINAL_Z)
        end)
        schedule(guid, "outro6", 4000, function()
            creature:Talk(SAY_OUTRO_HORDE_4)
        end)
        return
    end
    if id == POINT_FINAL then
        local deathbringer = creature:FindNearestCreature(ENTRY_DEATHBRINGER_SAURFANG, 100)
        if deathbringer then
            deathbringer:Despawn()
        end
        creature:Despawn()
        return
    end
    if id ~= POINT_FIRST_STEP then
        return
    end
    local isHorde = creature:GetEntry() == ENTRY_SE_HIGH_OVERLORD_SAURFANG
    creature:SetWalk(false)
    if isHorde then
        creature:Talk(SAY_H_INTRO_3)
        schedule(guid, "introH5", 15500, function() creature:Talk(SAY_H_INTRO_5) end)
        schedule(guid, "introH6", 29500, function() creature:Talk(SAY_H_INTRO_6) end)
        schedule(guid, "introH7", 43800, function() creature:Talk(SAY_H_INTRO_7) end)
        schedule(guid, "introH8", 47000, function()
            creature:Talk(SAY_H_INTRO_8)
            chargeEventGuards(creature, isHorde)
            local p = CHARGE_POS[1]
            creature:MoveCharge(POINT_CHARGE, p[1], p[2], p[3], 8.5)
        end)
    else
        creature:Talk(SAY_A_INTRO_4)
        schedule(guid, "introA5", 5000, function()
            creature:Talk(SAY_A_INTRO_5)
            chargeEventGuards(creature, isHorde)
            local p = CHARGE_POS[1]
            creature:MoveCharge(POINT_CHARGE, p[1], p[2], p[3], 8.5)
        end)
    end
    local deathbringer = creature:FindNearestCreature(ENTRY_DEATHBRINGER_SAURFANG, 100)
    if deathbringer then
        deathbringer:DoAction(ACTION_CONTINUE_INTRO)
    end
end

-- C++ SpellHit(SPELL_GRIP_OF_AGONY) (:700-706/:908-914 event NPCs;
-- :970-976 guards): SetDisableGravity(true) + MovePoint(POINT_CHOKE,
-- chokePos[0] / chokePos[_index]). PORTED via ev14 (yogg-saron
-- signature precedent): the event is live per hit target of a creature
-- cast (boss_ai.go:4282), SetDisableGravity / MoveTo are bridged
-- (lua_creature_events.go:3776/:5305), and the handler is purely
-- reactive — it runs only when the engine delivers the 70572 hit from
-- the boss's GRIP self-casts (EVENT_INTRO_ALLIANCE_6 / HORDE_9 /
-- first engage). Guards read guardSortIndex (C++ _index, SetData
-- 1..5); unsorted guards fall back to slot 1, the C++ _index=0 twin.
-- POINT_CHOKE arrival fires ev28 on the event NPCs, which the
-- MovementInform handler ignores (no C++ POINT_CHOKE case there).
local function eventNpcSpellHit(event, creature, caster, spellId)
    if spellId ~= SPELL_GRIP_OF_AGONY then
        return
    end
    local p = CHOKE_POS[1]
    local idx = guardSortIndex[creature:GetGUIDLow()]
    if idx then
        p = CHOKE_POS[idx + 1]
    end
    creature:SetDisableGravity(true)
    creature:MoveTo(POINT_CHOKE, p[1], p[2], p[3])
end

-- C++ Reset (:624-627/:823-826): _events.Reset() only.
local function eventNpcReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    eventNpcPhase[guid] = nil
end

RegisterCreatureEvent(ENTRY_SE_HIGH_OVERLORD_SAURFANG, 23, eventNpcReset)
RegisterCreatureEvent(ENTRY_SE_HIGH_OVERLORD_SAURFANG, 28, eventNpcMovementInform)
RegisterCreatureEvent(ENTRY_SE_MURADIN_BRONZEBEARD, 23, eventNpcReset)
RegisterCreatureEvent(ENTRY_SE_MURADIN_BRONZEBEARD, 28, eventNpcMovementInform)
RegisterCreatureGossipEvent(ENTRY_SE_HIGH_OVERLORD_SAURFANG, 2, eventNpcGossipSelect)
RegisterCreatureGossipEvent(ENTRY_SE_MURADIN_BRONZEBEARD, 2, eventNpcGossipSelect)
RegisterCreatureEvent(ENTRY_SE_HIGH_OVERLORD_SAURFANG, 14, eventNpcSpellHit)
RegisterCreatureEvent(ENTRY_SE_MURADIN_BRONZEBEARD, 14, eventNpcSpellHit)
RegisterCreatureEvent(ENTRY_SE_KOR_KRON_REAVER, 14, eventNpcSpellHit)
RegisterCreatureEvent(ENTRY_SE_SKYBREAKER_MARINE, 14, eventNpcSpellHit)

RegisterCreatureEvent(ENTRY_DEATHBRINGER_SAURFANG, 1, saurfangJustEngagedWith)
RegisterCreatureEvent(ENTRY_DEATHBRINGER_SAURFANG, 2, saurfangLeaveCombat)
RegisterCreatureEvent(ENTRY_DEATHBRINGER_SAURFANG, 3, saurfangKilledUnit)
RegisterCreatureEvent(ENTRY_DEATHBRINGER_SAURFANG, 4, saurfangJustDied)
RegisterCreatureEvent(ENTRY_DEATHBRINGER_SAURFANG, 9, saurfangDamageTaken)
RegisterCreatureEvent(ENTRY_DEATHBRINGER_SAURFANG, 15, saurfangSpellHitTarget)
RegisterCreatureEvent(ENTRY_DEATHBRINGER_SAURFANG, 19, saurfangJustSummoned)
RegisterCreatureEvent(ENTRY_DEATHBRINGER_SAURFANG, 20, saurfangSummonDespawned)
RegisterCreatureEvent(ENTRY_DEATHBRINGER_SAURFANG, 21, saurfangSummonDied)
RegisterCreatureEvent(ENTRY_DEATHBRINGER_SAURFANG, 23, saurfangReset)
RegisterCreatureEvent(ENTRY_DEATHBRINGER_SAURFANG, 24, saurfangReachHome)
RegisterCreatureEvent(ENTRY_DEATHBRINGER_SAURFANG, 38, saurfangDoAction)
RegisterAuraEvent(SPELL_BLOOD_LINK, 1, bloodLinkTick)
