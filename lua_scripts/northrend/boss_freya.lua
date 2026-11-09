-- Freya (Ulduar) — Lua port of
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_freya.cpp
-- (boss_freya (CreatureScript) via GetUlduarAI<boss_freyaAI>
-- (BossAI), BOSS_FREYA = 9; boss_elder_brightleaf /
-- boss_elder_ironbranch / boss_elder_stonebark (CreatureScript
-- via GetUlduarAI<...AI> (BossAI), BOSS_BRIGHTLEAF = 14 /
-- BOSS_IRONBRANCH = 15 / BOSS_STONEBARK = 16 — all registered
-- from inside AddSC_boss_freya(); loader decl 120 / call 315
-- per northrend_script_loader.cpp — the ELEVENTH group of the
-- "// Ulduar" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_hodir() (decl 119 / call 314); the call
-- after it is AddSC_boss_thorim()).
-- Entry: 32906 Freya (ulduar.h NPC_FREYA, line 80),
-- 32913 Ironbranch (NPC_IRONBRANCH, line 136),
-- 32915 Brightleaf (NPC_BRIGHTLEAF, line 137),
-- 32914 Stonebark (NPC_STONEBARK, line 138) — all
-- entry-verifiable; the CreatureScript ScriptName bindings are
-- DB-side as usual.
-- TIMER-BRIDGE DOCTRINE (2026-10-13, supersedes the stale
-- "no timer-event / cast bridges" claims this header used to
-- carry): CreateLuaEvent is a real registered bridge
-- (engine/scripting/runtime.go); timer-driven cast/Talk arms
-- on the main AI port via the GUID-keyed timers table below
-- (cancel on events 2/4/23), following the maiden_of_virtue /
-- moroes in-tree precedent. CastSpell(nil, spell) =
-- DoCastVictim; CastSpell(creature, spell) = DoCastSelf /
-- DoCastAOE; the 3rd arg carries the C++ DoCast triggered
-- flag (2-arg overload = non-triggered). Phase-machine-gated,
-- summon/instance-GUID, adds/hard-mode, MotionMaster and
-- DoAction-gated arms stay out; random-target legs bridge via
-- the threat-list candidate-selection precedent
-- (thorim colossus RUNIC_CHARGE / giant
-- RUNE_DETONATION this run).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat,
-- 3 OnTargetDied, 4 OnDied, 9 OnDamageTaken, 23 OnReset.
--
-- PORTED ARMS (all C++-exact unless noted under DEVIATIONS):
-- Freya JustEngagedWith (:388-391) unconditional timers:
--   EVENT_ENRAGE (:421-424, init 10min :390, one-shot — no
--     re-arm in C++) — Talk(SAY_BERSERK 4) + DoCast(me,
--     SPELL_ENRAGE 47008), non-triggered self-cast.
--   EVENT_EONAR_GIFT (:446-449, init 25s :389, repeat
--     40s-50s) — Talk(EMOTE_LIFEBINDERS_GIFT 8) + DoCast(me,
--     SPELL_SUMMON_EONAR_GIFT 62572), non-triggered self-cast
--     (the NPC 33228 summon is spell-effect-side; the AI arm
--     itself is a plain cast).
--   EVENT_SUNBEAM (:425-429, init 5s-15s :391, repeat
--     10s-15s) — random player 100y (SelectTarget Random 0
--     100.0f playerOnly), DoCast(target, SPELL_SUNBEAM
--     62623), non-triggered; bridged via the
--     threat-list candidate-selection below.
--   EVENT_UNSTABLE_ENERGY (:434-438, init 10s-20s :364,
--     repeat 15s-20s) / EVENT_GROUND_TREMOR (:457-461, init
--     10s-20s :370, repeat 25s-28s) /
--     EVENT_STRENGTHENED_IRON_ROOTS (:451-456, init 10s-20s
--     :376, repeat 12s-20s) — elder-alive-gated at engage
--     (Brightleaf / Stonebark / Ironbranch; the grid-scan
--     gate in freyaEnterCombat), PORTED 2026-11-08 (the
--     header used to list them as staying out; the body
--     port predates this tick).
--   EVENT_WAVE (:439-445) + SpawnWave (:579-600) +
--     GetWaveId (:563-577) + EVENT_NATURE_BOMB (:430-433)
--     — PORTED 2026-11-08: 10s init, 60s waves while
--     waveCount <= 6 (7 waves), then nature bombs 10s-20s
--     init / 10s-12s repeat; GetWaveId is the C++-exact
--     non-repeating urand(0,2) with reset; lashers x10 /
--     trio / conservator summon casts + SAY + EMOTE.
-- Freya JustSummoned (ev19) + SummonedCreatureDies (ev21)
-- — PORTED 2026-11-08: trio GUID tracking (pendingTrioSlot),
-- AttackStart random player 250y + 250 threat + DoZoneInCombat
-- on every summon; lasher REMOVE_2STACK + DETONATE on killer
-- + 5s despawn, conservator REMOVE_25STACK + 5s despawn.
-- Elemental revive (UpdateAI :480-531) + LasherDead
-- deforestation (:533-560) — PORTED 2026-11-08: 12s window
-- from first trio death (all three dead defeats the trio now:
-- 5s despawn + 3x REMOVE_10STACK; otherwise heal-to-full /
-- Respawn); 6-death window, masks 1/2/4, <10s + sum 14 ->
-- credit 65015 on threat-list players.
-- Freya KilledUnit — player-gated Talk(SAY_SLAY 2) (event 3;
-- who->GetTypeId() == TYPEID_PLAYER — the razuvious
-- player-gated variant precedent).
-- Freya DamageTaken — lethal (damage >= health) ->
-- Talk(SAY_DEATH 3) + damage = 0 (event 9; in C++ the
-- lethal branch sets damage = 0 and manually calls
-- JustDied(who), whose only bridgeable arm is the
-- Talk(SAY_DEATH) — the numeric second return performs the
-- C++-exact damage=0 rewrite (the moroes damage-rewrite
-- precedent); the companion legs have no bridges — see
-- DOCUMENTED-ONLY; the hodir consequence applies: without
-- the despawn leg, subsequent hits keep satisfying
-- damage >= health, so the death Talk re-fires on each hit
-- — verbatim C++ behavior, bounded in C++ by the 7.5s
-- despawn).
-- Elder Brightleaf Reset timers (:714-716), UpdateAI machine
-- gated on !HasAura(SPELL_DRAINED_OF_POWER 62467) (:740):
--   EVENT_SOLAR_FLARE (:756-765, init 5s-7s, repeat 5s-10s)
--     — self-cast SPELL_SOLAR_FLARE 62240, non-triggered
--     (SPELLVALUE_MAX_TARGETS mod — see DEVIATIONS).
--   EVENT_UNSTABLE_SUN_BEAM (:752-755, init 7s-12s, repeat
--     10s-15s) — me->CastSpell(me,
--     SPELL_UNSTABLE_SUN_BEAM_SUMMON 62207, true), triggered
--     self-cast.
--   EVENT_FLUX (:767-772, init 5s, repeat 7500ms) —
--     RemoveAurasDueToSpell + AddAura(SPELL_FLUX_AURA 62239)
--     (SetStackAmount — see DEVIATIONS).
-- Elder Stonebark Reset timers (:817-819), machine gated
-- :858:
--   EVENT_BARK (:870-873, init 37500ms-40s, repeat 30s-50s)
--     — DoCast(me, SPELL_PETRIFIED_BARK 62337), non-triggered
--     self-cast.
--   EVENT_FISTS (:874-877, init 25s-35s, repeat 20s-30s) —
--     DoCastVictim(SPELL_FISTS_OF_STONE 62344), non-triggered.
--   EVENT_TREMOR (:878-882, init 10s-12s, repeat 10s-20s) —
--     DoCastVictim(SPELL_GROUND_TREMOR 62325), non-triggered,
--     gated on !me->HasAura(SPELL_FISTS_OF_STONE) — C++-exact
--     via HasAura.
-- Elder Ironbranch Reset timers (:926-928), machine gated
-- :952:
--   EVENT_IMPALE (:964-967, init 18s-22s, repeat 15s-25s) —
--     DoCastVictim(SPELL_IMPALE 62310), non-triggered.
--   EVENT_THORN_SWARM (:973-976, init 7500ms-12500ms,
--     repeat 8s-13s) — DoCastVictim(SPELL_THORN_SWARM 62285),
--     non-triggered.
--   EVENT_IRON_ROOTS (:968-972, init 12s-17s :927, repeat
--     10s-20s) — random player 100y (playerOnly, aura-filter
--     -SPELL_ROOTS_IRONBRANCH), target->CastSpell(target,
--     SPELL_ROOTS_IRONBRANCH 62438, true) triggered; the
--     -SPELL_ROOTS_IRONBRANCH "without aura" filter has no
--     bridge (documented deviation), bridged otherwise via
--     the baltharus precedent.
-- Elder JustEngagedWith (all three elders, identical) —
-- Talk(SAY_ELDER_AGGRO 0) gated by
-- !me->HasAura(SPELL_DRAINED_OF_POWER 62467) (event 1; the
-- BossAI::JustEngagedWith passthrough has no bridge — the
-- auriaya engage-port precedent); the same gate is applied
-- to every elder timer handler below, mirroring the C++
-- UpdateAI early-return — C++-exact (the Drained application
-- itself lives in Freya's unbridgeable JustEngagedWith, and
-- nothing in the current engine engages elders jointly, so
-- the timers only ever run in standalone grove fights where
-- C++ carries no Drained either).
-- Elder KilledUnit (all three elders, identical) —
-- player-gated Talk(SAY_ELDER_SLAY 1) (event 3; the razuvious
-- player-gated variant precedent).
-- Elder JustDied (all three elders, identical) —
-- Talk(SAY_ELDER_DEATH 2) (event 4; the _JustDied()
-- passthrough has no bridge — the sjonnir JustDied-Talk
-- precedent).
-- npc_ancient_conservator JustEngagedWith (:1293-1296) —
-- DoCast(who, SPELL_CONSERVATOR_GRIP 62532), triggered; who
-- is the engaging unit (the event 1 target arg), bridged as
-- creature:CastSpell(target, spell, true) per the file
-- doctrine. The class's UpdateAI/summon legs stay out (see
-- DOCUMENTED-ONLY).
-- npc_iron_roots (33088) JustDied (boss_freya.cpp:243-253) —
-- me->DespawnOrUnsummon() ports (event 4, creature:Despawn,
-- bridged); the summonerGUID aura-removal legs have no bridge
-- (stay out); the rest of the class stays out (see
-- DOCUMENTED-ONLY).
--
-- DEVIATIONS from C++ (all bridge-limited, documented):
-- - Cast-state gates PORTED 2026-11-04: Freya's 6 timer arms
--   (eonar-gift / sunbeam / unstable-energy / ground-tremor /
--   iron-roots / enrage one-shot) and all 9 elder ticks defer
--   500ms on creature:IsCasting() (live,
--   lua_creature_events.go:1950), no repeat re-arm consumed —
--   the mal_ganis idiom for the boss pre-loop (:414) +
--   in-loop (:464) and elder pre-loop (:745/:863/:957) +
--   in-loop (:776/:885/:979) CASTING gates. Elder ticks keep
--   the drained-aura check first (C++ UpdateAI order). The
--   stale "timers fire unconditionally" deviation is retracted
--   for these arms. The wave / nature-bomb arms ported
--   2026-11-08 defer the same way (they sit under the same
--   :414/:464 gates in C++).
-- - EVENT_SOLAR_FLARE: C++ passes SPELLVALUE_MAX_TARGETS =
--   the FLUX_AURA stack count (1-8) via CastSpellExtraArgs;
--   creature:GetStackAmount is live now (2026-10-23) but the chain
--   is broken — Lua AddAura is presence-only (stacks unreadable)
--   and creature:CastSpell has no SpellMod/extra-args path — so
--   the cast fires with the spell's default targeting.
-- - EVENT_FLUX: C++ sets the re-applied aura to
--   urand(1, 8) stacks via Flux->SetStackAmount; dropped —
--   the aura is re-applied presence-only.
-- - Freya lethal DamageTaken: cancelTimers added — C++ keeps
--   the event queue but UpdateAI stops ticking (no victim
--   after RemoveAllAttackers/AttackStop) and the creature
--   despawns 7.5s later; without the cancel the Lua timers
--   would fire through the 7.5s despawn window (the despawn
--   itself IS ported — CreateLuaEvent 7500 in freyaDamageTaken;
--   the stale "despawn leg has no bridge" wording is retracted).
--
-- npc_unstable_sun_beam (33050) — ctor casts 62209/62211 +
-- REACT_PASSIVE, 7-12s DoCastAOE 62217 + DisappearAndDie, and
-- the ev15 62217 strip of 62211/62243 — PORTED 2026-11-05
-- (unstableSunBeamSpawn / unstableSunBeamSpellHitTarget below;
-- the 62207 summon spell materializing 33050 is DBC-side and
-- unverified in-repo).
-- DOCUMENTED-ONLY (in this header; registrations cover entries
-- 32906 / 32915 / 32914 / 32913 / 33203 / 33088 / 33050):
-- Freya JustEngagedWith companion legs (:342-391) —
-- DoZoneInCombat (:345) PORTED 2026-10-23 via the live bridge
-- (C++ arm order — before the elder scan); the elder loop is PORTED
-- 2026-10-26 into freyaEnterCombat (grid scan + drained aura +
-- essence casts + RemoveLootMode + EngageWithTarget + 250 threat,
-- C++ arm order); the Talk choice (SAY_AGGRO 0 vs
-- SAY_AGGRO_WITH_ELDER 1) was already ported. Still out: the
-- CastSpell SPELL_ATTUNED_TO_NATURE 62519 with 150-stack spell-mod
-- (no spell-mod bridge).
-- Freya JustDied companion legs (:602-~640) — chest summon
-- (summonSpell[2][4]: 62950/62952/62953/62954 10-man,
-- 62955/62956/62957/62958 25-man, indexed by
-- difficulty/elderCount) BLOCKED on the difficulty index (no
-- 10/25 raid-size bridge; creature:IsHeroic is heroic-only),
-- InterruptNonMeleeSpells, RemoveAllAttackers — PORTED 2026-10-24 into
-- freyaDamageTaken below (C++ JustDied order);
-- the live-elder shutdown sweep (RemoveAllAuras / AttackStop /
-- CombatStop / EngagementOver / DoAction(ACTION_ELDER_FREYA_KILLED))
-- — PORTED 2026-10-24 into freyaDied via the GetCreaturesInGrid elder
-- scan (no instance GuidData needed); CombatStop is bridged
-- (lua_creature_events.go, 2026-10-24) and ported into freyaDied below;
-- the elder _JustDied SetBossState DONE legs PORTED tick 259 via the
-- elder DoAction (ACTION_ELDER_FREYA_KILLED) + elderDied. Freya's own
-- legs were PORTED 2026-10-21 into
-- freyaDamageTaken (the stale "no cast / react-state / faction
-- bridges" retraction): SetReactState REACT_PASSIVE, AttackStop,
-- SetFaction FRIENDLY, CastSpell SPELL_KNOCK_ON_WOOD_CREDIT 65074,
-- DespawnOrUnsummon(7.5s) (stale "no despawn bridge" already
-- retracted; creature:Despawn() is bridged, lua_objects.go).
-- Talk(SAY_DEATH) itself is ported via event 9.
-- Freya UpdateAI body legs — Touch-of-Eonar aura
-- maintenance (every-tick, not timer-driven); attuned-stack
-- achievement sampling PORTED tick 417 as the Lua counter in
-- freyaEnterCombat (150 at engage) + freyaHitBySpell (REMOVE_*STACK
-- decrements) + freyaReset (0); elemental 12s revive / trio-defeated
-- GUID logic; DoMeleeAttackIfReady (engine-driven in Go).
-- Freya GetData(DATA_GETTING_BACK_TO_NATURE /
-- DATA_KNOCK_ON_WOOD) — the Lua write legs are PORTED tick 417 (see
-- above); the Go read side is the coordinator's
-- checkAchievementCriteriaScript case. The elder DoAction
-- (ACTION_ELDER_FREYA_KILLED -> DespawnOrUnsummon(10s)) is PORTED
-- into freyaDied (see above).
-- Elder DamageTaken — stonebark petrified-bark reflect
-- (:841-855: who->CastSpell(who, SPELL_PETRIFIED_BARK_DMG
-- 62379) + damage = 0) PORTED as event 9 (attacker self-cast
-- via attacker:CastSpell, doomwalker precedent); only the
-- AddSpellBP0(damage) mod stays out (no extra-args bridge).
-- npc_snaplasher /
-- npc_storm_lasher / npc_ancient_water_spirit /
-- npc_detonating_lasher / npc_sun_beam / npc_nature_bomb /
-- npc_eonars_gift / npc_healthy_spore /
-- npc_iron_roots — wave/summon creatures: their own AIs stay
-- out (instance-coupled ENSURE_AI Freya-AI callbacks,
-- waveCount from Freya's AI, GetCreature(BOSS_FREYA)),
-- random-target legs (tidal wave, stormbolt, nature fury),
-- threat/AttackStart legs, summon/despawn/RemoveFromWorld/
-- DisappearAndDie legs, SetReactState / SetFlag /
-- SetImmuneToPC / SetCombatMovement legs,
-- FindNearestGameObject leg — no spawn / movement /
-- threat bridges; no registrations. The FREYA-side legs are
-- PORTED 2026-11-08 (ev19 JustSummoned, ev21
-- SummonedCreatureDies, elemental revive, LasherDead) —
-- see PORTED ARMS above. (npc_ancient_conservator
-- is registered for its engage arm only — see PORTED ARMS;
-- its Reset/UpdateAI/summon legs stay out for the same
-- reasons.)
-- spell_freya_attuned_to_nature_dose_reduction /
-- spell_freya_iron_roots — no SpellScript bridges; both join
-- the no-SpellScript-bridge queue.
-- achievement_getting_back_to_nature /
-- achievement_knock_on_wood / achievement_knock_knock_on_wood /
-- achievement_knock_knock_knock_on_wood (OnCheck: GetData
-- legs) — Lua write legs PORTED tick 417 (SetData slots 1/2 in
-- freyaEnterCombat / freyaReset / freyaHitBySpell above); Go cases
-- are the coordinator's (checkAchievementCriteriaScript).

local ENTRY_FREYA = 32906
local ENTRY_BRIGHTLEAF = 32915
local ENTRY_STONEBARK = 32914
local ENTRY_IRONBRANCH = 32913
local BOSS_FREYA = 9 -- ulduar.h:45
-- Elder boss DATA ids (ulduar.h:50-52): Brightleaf 14, Ironbranch 15,
-- Stonebark 16 — keyed by creature entry for the shared elder handlers.
local ELDER_BOSS_DATA = {
    [ENTRY_BRIGHTLEAF] = 14,
    [ENTRY_IRONBRANCH] = 15,
    [ENTRY_STONEBARK] = 16,
}
local ACTION_ELDER_FREYA_KILLED = 1 -- boss_freya.cpp:176
local DATA_GETTING_BACK_TO_NATURE = 1 -- boss_freya.cpp:210
local DATA_KNOCK_ON_WOOD = 2 -- boss_freya.cpp:211
local ENTRY_ANCIENT_CONSERVATOR = 33203
local ENTRY_IRON_ROOTS = 33088 -- boss_freya.cpp:168
local ENTRY_UNSTABLE_SUN_BEAM = 33050 -- boss_freya.cpp:167

local SPELL_DRAINED_OF_POWER = 62467
local SPELL_STONEBARK_ESSENCE = 62483 -- boss_freya.cpp:60
local SPELL_IRONBRANCH_ESSENCE = 62484 -- boss_freya.cpp:61
local SPELL_BRIGHTLEAF_ESSENCE = 62485 -- boss_freya.cpp:62

-- Freya
local SPELL_ENRAGE = 47008
local SPELL_SUMMON_EONAR_GIFT = 62572
local SPELL_SUNBEAM = 62623
local SPELL_FREYA_UNSTABLE_SUNBEAM = 62450
local SPELL_FREYA_GROUND_TREMOR = 62437
local SPELL_ROOTS_FREYA = 62283

-- Freya wave machine (SpawnWave / EVENT_WAVE / EVENT_NATURE_BOMB)
local SPELL_SUMMON_LASHERS = 62687 -- boss_freya.cpp:95
local SPELL_SUMMON_TRIO = 62686 -- boss_freya.cpp:96
local SPELL_SUMMON_ANCIENT_CONSERVATOR = 62685 -- boss_freya.cpp:97
local SPELL_SUMMON_NATURE_BOMB = 64604 -- boss_freya.cpp:137
local SPELL_DETONATE = 62598 -- boss_freya.cpp:100
local SPELL_DEFORESTATION_CREDIT = 65015 -- boss_freya.cpp:91
local ENTRY_SNAPLASHER = 32916 -- boss_freya.cpp:163
local ENTRY_STORM_LASHER = 32919 -- boss_freya.cpp:162
local ENTRY_ANCIENT_WATER_SPIRIT = 33202 -- boss_freya.cpp:161
local ENTRY_DETONATING_LASHER = 32918 -- boss_freya.cpp:159

-- Elder Brightleaf
local SPELL_UNSTABLE_SUN_BEAM_SUMMON = 62207
local SPELL_SOLAR_FLARE = 62240
local SPELL_FLUX_AURA = 62239

-- Elder Stonebark
local SPELL_PETRIFIED_BARK = 62337
local SPELL_PETRIFIED_BARK_DMG = 62379
local SPELL_FISTS_OF_STONE = 62344
local SPELL_GROUND_TREMOR = 62325

-- Elder Ironbranch
local SPELL_IMPALE = 62310
local SPELL_THORN_SWARM = 62285
local SPELL_ROOTS_IRONBRANCH = 62438

-- Ancient Conservator
local SPELL_CONSERVATOR_GRIP = 62532

-- Unstable Sun Beam (33050)
local SPELL_PHOTOSYNTHESIS = 62209 -- boss_freya.cpp:142
local SPELL_UNSTABLE_SUN_BEAM = 62211 -- boss_freya.cpp:140
local SPELL_UNSTABLE_ENERGY = 62217 -- boss_freya.cpp:141
local SPELL_UNSTABLE_SUN_BEAM_TRIGGERED = 62243 -- boss_freya.cpp:143

local SAY_SLAY = 2
local SAY_DEATH = 3
local SAY_BERSERK = 4
local SAY_SUMMON_CONSERVATOR = 5 -- boss_freya.cpp:37
local SAY_SUMMON_TRIO = 6 -- boss_freya.cpp:38
local SAY_SUMMON_LASHERS = 7 -- boss_freya.cpp:39
local EMOTE_ALLIES_OF_NATURE = 9 -- boss_freya.cpp:41
local SAY_AGGRO = 0 -- boss_freya.cpp:32
local SAY_AGGRO_WITH_ELDER = 1 -- boss_freya.cpp:33
local EMOTE_LIFEBINDERS_GIFT = 8
local EMOTE_GROUND_TREMOR = 10
local EMOTE_IRON_ROOTS = 11

local SAY_ELDER_AGGRO = 0
local SAY_ELDER_SLAY = 1
local SAY_ELDER_DEATH = 2

local SPELL_KNOCK_ON_WOOD_CREDIT = 65074 -- boss_freya.cpp:92
local SPELL_ATTUNED_TO_NATURE = 62519 -- boss_freya.cpp:60
local SPELL_REMOVE_25STACK = 62521 -- boss_freya.cpp:86
local SPELL_REMOVE_10STACK = 62525 -- boss_freya.cpp:87
local SPELL_REMOVE_2STACK = 62524 -- boss_freya.cpp:88
local FACTION_FRIENDLY = 35 -- SharedDefines.h:231
local REACT_PASSIVE = 0 -- UnitDefines.h:340

-- GUID-keyed timer harness (the maiden_of_virtue precedent):
-- CreateLuaEvent ids per creature GUID, cancelled on
-- OnLeaveCombat / OnDied / OnReset. A {min, max} delay is a
-- randtime range, matching C++ ScheduleEvent(min, max).
local timers = {}

local freyaFinished = {}

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

-- C++ elder UpdateAI: if (!UpdateVictim() ||
-- me->HasAura(SPELL_DRAINED_OF_POWER)) return; — the victim
-- half is modeled by timer lifetime (cancelled on
-- leave-combat/death/reset); the aura half is C++-exact via
-- HasAura (the same gate the aggro Talk already used).
local function elderTimersActive(creature)
    return not creature:HasAura(SPELL_DRAINED_OF_POWER)
end

-- C++ SelectTarget(SelectTargetMethod::Random, 0, dist, true)
-- legs (boss_freya.cpp:426 sunbeam, :435 unstable energy,
-- :453 strengthened roots with -SPELL_ROOTS_FREYA, :969 iron
-- roots with -SPELL_ROOTS_IRONBRANCH): threat-list players
-- alive + dist, offset 0 (tank eligible). Gate twin: IsWithinCombatRange
-- (Unit.cpp:583) — raw center minus own reach minus player reach 1.5,
-- reach-exact.
local function randomPlayerInRange(creature, minDist, maxDist, excludeAura)
    local candidates = {}
    for _, unit in ipairs(creature:GetAITargets()) do
        if unit:GetObjectType() == "Player" and not unit:IsDead()
                and (excludeAura == nil or not unit:HasAura(excludeAura)) then
            local dist = creature:GetDistance(unit) - creature:GetCombatReach() - 1.5
            if (minDist <= 0 or dist >= minDist) and dist < maxDist then
                candidates[#candidates + 1] = unit
            end
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EVENT_EONAR_GIFT (boss_freya.cpp:446-449):
-- Talk(EMOTE_LIFEBINDERS_GIFT) + DoCast(me,
-- SPELL_SUMMON_EONAR_GIFT), non-triggered self-cast;
-- init 25s (:389), repeat 40s-50s.
local function onEonarGift(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "eonar_gift", 500, function() onEonarGift(creature, guid) end)
        return
    end
    creature:Talk(EMOTE_LIFEBINDERS_GIFT)
    creature:CastSpell(creature, SPELL_SUMMON_EONAR_GIFT)
    schedule(guid, "eonar_gift", {40000, 50000}, function() onEonarGift(creature, guid) end)
end

-- C++ JustEngagedWith (:388-391) schedules the timers, and the aggro
-- Talk choice (SAY_AGGRO vs SAY_AGGRO_WITH_ELDER, :379-382) IS ported
-- below: elderCount (alive elders via instance GuidData — no bridge)
-- is reconstructed via GetCreaturesInGrid + IsAlive over the three
-- elder entries (the C++ counts Elder[n] && Elder[n]->IsAlive(),
-- :357). Only the unconditional timer arms register here otherwise.
-- C++ EVENT_SUNBEAM (boss_freya.cpp:425-429): random player
-- 100y, DoCast(target, SPELL_SUNBEAM), 2-arg = non-triggered;
-- init 5s-15s (:391), repeat 10s-15s.
local function onSunbeam(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "sunbeam", 500, function() onSunbeam(creature, guid) end)
        return
    end
    local target = randomPlayerInRange(creature, 0, 100)
    if target then
        creature:CastSpell(target, SPELL_SUNBEAM)
    end
    schedule(guid, "sunbeam", {10000, 15000}, function() onSunbeam(creature, guid) end)
end

-- C++ EVENT_UNSTABLE_ENERGY (boss_freya.cpp:434-437): random
-- player 100y, DoCast(target, SPELL_FREYA_UNSTABLE_SUNBEAM 62450,
-- true) triggered; init 10s-20s (:364), repeat 15s-20s. Gated on
-- Brightleaf alive at engage (see freyaEnterCombat).
local function onUnstableEnergy(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "unstable_energy", 500, function() onUnstableEnergy(creature, guid) end)
        return
    end
    local target = randomPlayerInRange(creature, 0, 100)
    if target then
        creature:CastSpell(target, SPELL_FREYA_UNSTABLE_SUNBEAM, true)
    end
    schedule(guid, "unstable_energy", {15000, 20000}, function() onUnstableEnergy(creature, guid) end)
end

-- C++ EVENT_GROUND_TREMOR (boss_freya.cpp:457-460): Talk
-- (EMOTE_GROUND_TREMOR 10) + DoCastAOE (self-cast)
-- SPELL_FREYA_GROUND_TREMOR 62437, non-triggered; init 10s-20s
-- (:370), repeat 25s-28s. Gated on Stonebark alive at engage.
local function onFreyaGroundTremor(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "freya_ground_tremor", 500, function() onFreyaGroundTremor(creature, guid) end)
        return
    end
    creature:Talk(EMOTE_GROUND_TREMOR)
    creature:CastSpell(creature, SPELL_FREYA_GROUND_TREMOR)
    schedule(guid, "freya_ground_tremor", {25000, 28000}, function() onFreyaGroundTremor(creature, guid) end)
end

-- C++ EVENT_STRENGTHENED_IRON_ROOTS (boss_freya.cpp:451-455):
-- Talk(EMOTE_IRON_ROOTS 11) + random player 100y without
-- SPELL_ROOTS_FREYA -> target self-cast 62283, triggered; init
-- 10s-20s (:376), repeat 12s-20s. Gated on Ironbranch alive at
-- engage.
local function onStrengthenedIronRoots(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "strengthened_iron_roots", 500, function() onStrengthenedIronRoots(creature, guid) end)
        return
    end
    creature:Talk(EMOTE_IRON_ROOTS)
    local target = randomPlayerInRange(creature, 0, 100, SPELL_ROOTS_FREYA)
    if target then
        target:CastSpell(target, SPELL_ROOTS_FREYA, true)
    end
    schedule(guid, "strengthened_iron_roots", {12000, 20000}, function() onStrengthenedIronRoots(creature, guid) end)
end

-- C++ EVENT_ENRAGE one-shot (:421-424): named tick so the 500ms
-- IsCasting defer does not consume the cast — the auriaya berserk
-- precedent; under the boss pre-loop (:414) + in-loop (:464) gates.
local function onFreyaEnrage(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "enrage", 500, function() onFreyaEnrage(creature, guid) end)
        return
    end
    creature:Talk(SAY_BERSERK)
    creature:CastSpell(creature, SPELL_ENRAGE)
end

-- Freya wave state, per Freya GUID (fresh each engage; the C++
-- members Initialize() zeroes: waveCount / trioWaveCount /
-- trioWaveController / random[3] / checkElementalAlive[2] /
-- trioDefeated[2] / deforestation[6][2] / diffTimer,
-- boss_freya.cpp:280-301). elementals[slot] is the Lua twin of
-- ElementalGUID[3][2] (:300): per-trio-slot list of
-- { guid, obj, entry, alive }; the trioWaveController index is
-- the list position. C++ writes ElementalGUID[][trioWaveCount]
-- with no bound check — a third trio wave would index [x][2]
-- out of bounds; Lua tracks only slots 1-2 and lets a third
-- trio wave's elementals run untracked (engage legs only).
local freyaWave = {}

local function newWaveState()
    return {
        waveCount = 0,
        trioWaveCount = 0,
        random = { false, false, false },
        pendingTrioSlot = nil,
        checkElementalAlive = { true, true },
        trioDefeated = { false, false },
        elementals = { {}, {} },
        revivePending = { false, false },
        deforestation = {},
        fightStart = 0,
    }
end

-- C++ GetWaveId (boss_freya.cpp:563-577): non-repeating
-- urand(0, 2) over { lashers, trio, conservator }, reset when
-- all three have fired. Lua table is 1-based; the returned id
-- stays 0-based to match the SpawnWave switch.
local function getWaveId(st)
    if st.random[1] and st.random[2] and st.random[3] then
        st.random = { false, false, false }
    end
    local id = math.random(0, 2)
    while st.random[id + 1] do
        id = math.random(0, 2)
    end
    st.random[id + 1] = true
    return id
end

-- C++ EVENT_NATURE_BOMB (boss_freya.cpp:430-433): DoCastAOE
-- (self-cast) SPELL_SUMMON_NATURE_BOMB 64604, triggered;
-- repeat 10s-12s. Scheduled from the EVENT_WAVE arm once
-- waveCount > 6 (:439-444).
local function onNatureBomb(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "nature_bomb", 500, function() onNatureBomb(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_SUMMON_NATURE_BOMB, true)
    schedule(guid, "nature_bomb", {10000, 12000}, function() onNatureBomb(creature, guid) end)
end

-- C++ EVENT_WAVE (boss_freya.cpp:439-445) + SpawnWave
-- (:579-600): Talk + summon casts per GetWaveId, then Talk
-- (EMOTE_ALLIES_OF_NATURE 9) + waveCount++; while waveCount
-- <= 6 the next wave comes in FREYA_WAVE_TIME 60s (:214),
-- else EVENT_NATURE_BOMB 10s-20s. 7 waves total. The trio
-- slot is captured BEFORE the summon cast (pendingTrioSlot):
-- C++ reads the pre-increment trioWaveCount in JustSummoned,
-- and the Go spell cast may resolve after the increment line.
local function onFreyaWave(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "wave", 500, function() onFreyaWave(creature, guid) end)
        return
    end
    local st = freyaWave[guid]
    if not st then
        return
    end
    local waveId = getWaveId(st)
    if waveId == 0 then
        creature:Talk(SAY_SUMMON_LASHERS)
        for _ = 1, 10 do
            creature:CastSpell(creature, SPELL_SUMMON_LASHERS)
        end
    elseif waveId == 1 then
        creature:Talk(SAY_SUMMON_TRIO)
        st.pendingTrioSlot = st.trioWaveCount
        creature:CastSpell(creature, SPELL_SUMMON_TRIO)
        st.trioWaveCount = st.trioWaveCount + 1
    else
        creature:Talk(SAY_SUMMON_CONSERVATOR)
        creature:CastSpell(creature, SPELL_SUMMON_ANCIENT_CONSERVATOR)
    end
    creature:Talk(EMOTE_ALLIES_OF_NATURE)
    st.waveCount = st.waveCount + 1
    if st.waveCount <= 6 then
        schedule(guid, "wave", 60000, function() onFreyaWave(creature, guid) end)
    else
        schedule(guid, "nature_bomb", {10000, 20000}, function() onNatureBomb(creature, guid) end)
    end
end

-- C++ UpdateAI elemental revive (boss_freya.cpp:495-524, the
-- >12000 branch): past 12s with the trio not defeated, living
-- elementals heal to full and dead ones respawn; the
-- checkElementalAlive flag re-arms for the next death. The
-- all-dead defeat check runs on every tick below 12s, so it
-- lives in freyaSummonedDied, not here.
local function onTrioRevive(creature, guid, slot)
    local st = freyaWave[guid]
    if not st then
        return
    end
    st.revivePending[slot] = false
    st.checkElementalAlive[slot] = true
    if st.trioDefeated[slot] then
        return
    end
    local trio = st.elementals[slot]
    if #trio ~= 3 then
        return
    end
    for _, el in ipairs(trio) do
        if el.alive then
            el.obj:SetHealth(el.obj:GetMaxHealth())
        else
            el.obj:Respawn()
            el.alive = true
        end
    end
end

-- C++ LasherDead (boss_freya.cpp:533-560): sliding window of
-- the last 6 trio-elemental deaths (time, binary mask); when
-- the window spans <10s and the masks sum to 14, the
-- deforestation credit 65015 is cast on the players. Masks:
-- water spirit 1, storm lasher 2, snaplasher 4 (the detonating
-- lasher never calls LasherDead). Time is os.time() seconds
-- from engage (the boss_ignis precedent); C++ diffTimer is
-- millisecond UpdateAI accumulation. Deviation: C++ casts on
-- every player in the instance; Lua casts on the threat-list
-- players (the raid in combat) — no instance-wide cast bridge.
local function lasherDead(creature, st, mask)
    local window = st.deforestation
    window[#window + 1] = { time = os.time() - st.fightStart, mask = mask }
    if #window > 6 then
        table.remove(window, 1)
    end
    if #window == 6 then
        local sum = 0
        for _, w in ipairs(window) do
            sum = sum + w.mask
        end
        if sum == 14 and (window[6].time - window[1].time) < 10 then
            for _, unit in ipairs(creature:GetAITargets()) do
                if unit:GetObjectType() == "Player" and not unit:IsDead() then
                    creature:CastSpell(unit, SPELL_DEFORESTATION_CREDIT, true)
                end
            end
        end
    end
end

-- C++ JustSummoned (boss_freya.cpp:643-671): trio elementals
-- are tracked in ElementalGUID (the pendingTrioSlot captured
-- at cast time); every summon then attacks a random player in
-- 250y with 250 threat + a zone engage. The target pick reads
-- FREYA's threat list (C++ SelectTarget runs on Freya's AI),
-- not the fresh summon's empty one. summons.Summon has no Lua
-- twin (ev21 dispatch rides the OwnerGUID stamp, not the
-- BossAI summons list).
local function freyaJustSummoned(event, creature, summoned)
    local guid = creature:GetGUIDLow()
    local st = freyaWave[guid]
    local entry = summoned:GetEntry()
    if st ~= nil and (entry == ENTRY_SNAPLASHER or entry == ENTRY_ANCIENT_WATER_SPIRIT or entry == ENTRY_STORM_LASHER) then
        local slot = st.pendingTrioSlot
        if slot ~= nil and slot <= 1 then
            local trio = st.elementals[slot + 1]
            trio[#trio + 1] = { guid = summoned:GetGUID(), obj = summoned, entry = entry, alive = true }
        end
    end
    local target = randomPlayerInRange(creature, 0, 250)
    if target then
        summoned:EngageWithTarget(target)
        summoned:AddThreat(target, 250)
        summoned:DoZoneInCombat()
    end
end

-- C++ SummonedCreatureDies (boss_freya.cpp:674-688): a dying
-- detonating lasher casts REMOVE_2STACK 62524 on Freya +
-- DETONATE 62598 on its killer and despawns in 5s; a dying
-- ancient conservator casts REMOVE_25STACK 62521 on Freya and
-- despawns in 5s. The REMOVE casts land via freyaHitBySpell
-- (ev14). The trio elementals' death legs are NOT here in C++
-- (their own JustDied :1119/:1193/:1242 sets
-- checkElementalAlive[slot] = false + LasherDead(mask)); the
-- elementals carry no Lua registration, so ev21 drives those
-- legs here, keyed by the ev19-tracked GUID. The defeat check
-- is the C++ UpdateAI :505-521 else-branch: all three dead
-- inside the 12s window defeats the trio now (5s despawn +
-- one REMOVE_10STACK 62525 cast per elemental, -30 via
-- freyaHitBySpell); otherwise a 12s revive timer starts on
-- the first death of the trio (revivePending — the C++
-- elementalTimer accumulates from the first death, it does
-- not reset on later deaths).
local function freyaSummonedDied(event, creature, summoned, killer)
    local guid = creature:GetGUIDLow()
    local st = freyaWave[guid]
    local entry = summoned:GetEntry()
    if entry == ENTRY_DETONATING_LASHER then
        summoned:CastSpell(creature, SPELL_REMOVE_2STACK, true)
        if killer then
            summoned:CastSpell(killer, SPELL_DETONATE, true)
        end
        summoned:DespawnOrUnsummon(5000)
    elseif entry == ENTRY_ANCIENT_CONSERVATOR then
        summoned:CastSpell(creature, SPELL_REMOVE_25STACK, true)
        summoned:DespawnOrUnsummon(5000)
    elseif st ~= nil and (entry == ENTRY_SNAPLASHER or entry == ENTRY_ANCIENT_WATER_SPIRIT or entry == ENTRY_STORM_LASHER) then
        local slot = nil
        for s = 1, 2 do
            for _, el in ipairs(st.elementals[s]) do
                if el.guid == summoned:GetGUID() then
                    slot = s
                    el.alive = false
                    break
                end
            end
            if slot then
                break
            end
        end
        if slot ~= nil and not st.trioDefeated[slot] then
            st.checkElementalAlive[slot] = false
            local mask = 1
            if entry == ENTRY_STORM_LASHER then
                mask = 2
            elseif entry == ENTRY_SNAPLASHER then
                mask = 4
            end
            lasherDead(creature, st, mask)
            local trio = st.elementals[slot]
            if #trio == 3 and not trio[1].alive and not trio[2].alive and not trio[3].alive then
                st.trioDefeated[slot] = true
                st.checkElementalAlive[slot] = true
                for _, el in ipairs(trio) do
                    el.obj:DespawnOrUnsummon(5000)
                    el.obj:CastSpell(creature, SPELL_REMOVE_10STACK, true)
                end
            elseif not st.revivePending[slot] then
                st.revivePending[slot] = true
                schedule(guid, "revive_" .. slot, 12000, function() onTrioRevive(creature, guid, slot) end)
            end
        end
    end
end

local function freyaEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    creature:SetBossState(BOSS_FREYA, 1) -- BossAI::JustEngagedWith SetBossState(DATA_FREYA 9, IN_PROGRESS), base-first (boss_freya.cpp:344; PORTED tick 259)
    creature:SetCombatPulseDelay(5) -- BossAI::JustEngagedWith arm 1 (ScriptedCreature.cpp:540) -- base-first (boss_freya.cpp:344; me-form DoZone already ported below as the :345 arm)
    -- C++ :345 — me-form DoZoneInCombat after BossAI::JustEngagedWith,
    -- before the elder scan (C++ arm order).
    creature:DoZoneInCombat()
    -- C++ JustEngagedWith (boss_freya.cpp:379-382): elderCount == 0 ->
    -- Talk(SAY_AGGRO), else Talk(SAY_AGGRO_WITH_ELDER); elderCount is
    -- the alive-elder count (C++: Elder[n] && Elder[n]->IsAlive(),
    -- :357) reconstructed via GetCreaturesInGrid + IsAlive.
    local eldersAlive = 0
    local elderAlive = {}
    local elderObj = {}
    for i, entry in ipairs({ ENTRY_BRIGHTLEAF, ENTRY_STONEBARK, ENTRY_IRONBRANCH }) do
        elderAlive[i] = false
        for _, elder in ipairs(creature:GetCreaturesInGrid(entry, 150)) do
            if elder:IsAlive() then
                eldersAlive = eldersAlive + 1
                elderAlive[i] = true
                if elderObj[i] == nil then
                    elderObj[i] = elder
                end
            end
        end
    end
    -- C++ GetData(DATA_KNOCK_ON_WOOD 2) returns elderCount
    -- (boss_freya.cpp:400-401), counted at engage (:357); the
    -- achievement_knock_*_on_wood Go gates read this slot off the
    -- killed victim's motion (achievement_criteria_scripts.go).
    creature:SetData(DATA_KNOCK_ON_WOOD, eldersAlive)
    -- C++ JustEngagedWith (:386-388) casts SPELL_ATTUNED_TO_NATURE
    -- 62519 with a 150-stack spell-mod; the mod has no bridge, so the
    -- Lua counter models the C++ attunedToNature member (GetData
    -- DATA_GETTING_BACK_TO_NATURE 1, :398-399), sampled from the aura
    -- stack every UpdateAI tick (:471-475). It starts at 150 here;
    -- only the REMOVE_*STACK hits in freyaHitBySpell below can lower
    -- it. The aura itself is never applied in Go, so polling
    -- creature:GetStackAmount(62519) would read 0 forever — the
    -- counter is the live leg, not the poll.
    creature:SetData(DATA_GETTING_BACK_TO_NATURE, 150)
    -- C++ JustEngagedWith elder loop (:347-358, Brightleaf/Stonebark/
    -- Ironbranch order; instance GuidData in C++, the grid scan above
    -- is the engage-time precedent): per alive elder — drained aura
    -- onto the elder, ironbranch-essence cast onto Freya, loot-mode
    -- strip, AttackStart onto the engager + 250 threat on the elder's
    -- own list (C++ AddThreat(who, 250, Elder) lowers to the elder's
    -- ThreatManager, ScriptedCreature.cpp:202-209). The elder's own
    -- enter handler fires via the engage funnel, matching the C++
    -- elder JustEngagedWith. The specific-essence casts (:360-377)
    -- follow below; the 150-stack attuned cast has no spell-mod
    -- bridge and stays out.
    local victim = target or creature:GetVictim()
    for i = 1, 3 do
        local elder = elderObj[i]
        if elder ~= nil then
            elder:AddAura(SPELL_DRAINED_OF_POWER)
            elder:CastSpell(creature, SPELL_IRONBRANCH_ESSENCE, true)
            elder:RemoveLootMode(1) -- LOOT_MODE_DEFAULT (SharedDefines.h:42)
            if victim ~= nil then
                elder:EngageWithTarget(victim)
                elder:AddThreat(victim, 250)
            end
        end
    end
    if elderAlive[1] then
        elderObj[1]:CastSpell(creature, SPELL_BRIGHTLEAF_ESSENCE, true)
    end
    if elderAlive[2] then
        elderObj[2]:CastSpell(creature, SPELL_STONEBARK_ESSENCE, true)
    end
    if elderAlive[3] then
        elderObj[3]:CastSpell(creature, SPELL_IRONBRANCH_ESSENCE, true)
    end
    if eldersAlive == 0 then
        creature:Talk(SAY_AGGRO)
    else
        creature:Talk(SAY_AGGRO_WITH_ELDER)
    end
    -- C++ EVENT_ENRAGE (:421-424): one-shot 10min (:390);
    -- Talk(SAY_BERSERK) + DoCast(me, SPELL_ENRAGE),
    -- non-triggered self-cast; no re-arm in C++.
    schedule(guid, "enrage", 600000, function() onFreyaEnrage(creature, guid) end)
    schedule(guid, "eonar_gift", 25000, function() onEonarGift(creature, guid) end)
    schedule(guid, "sunbeam", {5000, 15000}, function() onSunbeam(creature, guid) end)
    -- C++ JustEngagedWith elder-gated arms (:362-377): the gate is
    -- the same per-entry alive scan as the Talk choice above
    -- (C++ reads instance GuidData; the grid scan is this file's
    -- engage-time precedent). Inits are all 10s-20s.
    if elderAlive[1] then
        schedule(guid, "unstable_energy", {10000, 20000}, function() onUnstableEnergy(creature, guid) end)
    end
    if elderAlive[2] then
        schedule(guid, "freya_ground_tremor", {10000, 20000}, function() onFreyaGroundTremor(creature, guid) end)
    end
    if elderAlive[3] then
        schedule(guid, "strengthened_iron_roots", {10000, 20000}, function() onStrengthenedIronRoots(creature, guid) end)
    end
    -- C++ JustEngagedWith (:390): EVENT_WAVE init 10s; the wave
    -- machine (SpawnWave :579-600, GetWaveId :563-577,
    -- FREYA_WAVE_TIME 60s :214) is ported above — see
    -- onFreyaWave. Fresh wave state each engage (C++ Reset ->
    -- Initialize :280-301).
    freyaWave[guid] = newWaveState()
    freyaWave[guid].fightStart = os.time()
    schedule(guid, "wave", 10000, function() onFreyaWave(creature, guid) end)
end

local function freyaLeaveCombat(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    freyaWave[guid] = nil
end

-- C++ Freya JustDied elder shutdown sweep (boss_freya.cpp:630-641):
-- for each live elder (instance GuidData in C++, the GetCreaturesInGrid
-- scan here — the engage-time precedent above): RemoveAllAuras +
-- CombatStop(true) (AttackStop + RemoveAllAttackers + EndAllPvECombat
-- legs, bridged 2026-10-24) + the elder DoAction
-- (ACTION_ELDER_FREYA_KILLED, PORTED tick 259 via creature:DoAction,
-- ev38 dispatch bridged lua_creature_events.go:802 — the handler does
-- DespawnOrUnsummon(10s) + _JustDied() SetBossState DONE, C++ :783-789).
-- EngagementOver is freya-side housekeeping on the dying boss (its Go
-- analog, InCombat=false, rides CombatStop).
local function freyaElderShutdown(creature)
    for _, entry in ipairs({ ENTRY_BRIGHTLEAF, ENTRY_STONEBARK, ENTRY_IRONBRANCH }) do
        for _, elder in ipairs(creature:GetCreaturesInGrid(entry, 150)) do
            if elder:IsAlive() then
                elder:RemoveAllAuras()
                elder:CombatStop(true)
                cancelTimers(elder:GetGUIDLow())
                elder:DoAction(ACTION_ELDER_FREYA_KILLED)
            end
        end
    end
end

-- Fallback for true-kill paths: on the lethal DamageTaken path the
-- kill funnel never runs (damage is rewritten to 0), so the sweep
-- is driven from freyaDamageTaken below; this stays as the
-- event-4 fallback.
local function freyaDied(event, creature, killer)
    creature:SetBossState(BOSS_FREYA, 3) -- _JustDied() SetBossState(DATA_FREYA 9, DONE), PORTED tick 259 (C++ JustDied :628)
    cancelTimers(creature:GetGUIDLow())
    freyaWave[creature:GetGUIDLow()] = nil
    freyaElderShutdown(creature)
end

local function freyaReset(event, creature)
    if freyaFinished[creature:GetGUIDLow()] then
        return -- C++ Reset early-returns when _encounterFinished (:318-322): no _Reset(), no NOT_STARTED write
    end
    creature:SetBossState(BOSS_FREYA, 0) -- _Reset() SetBossState(DATA_FREYA 9, NOT_STARTED), PORTED tick 259 (the tick-258 auriaya convention)
    cancelTimers(creature:GetGUIDLow())
    freyaWave[creature:GetGUIDLow()] = nil
    -- C++ Reset -> Initialize (:285) zeroes elderCount; the attuned
    -- half mirrors the no-aura -> 0 sampling (:471-475). A fresh
    -- motion reads 0 anyway; this is the wipe twin. Alive-gated:
    -- ev23 fires on death too (fireCreatureDied runs Reset before
    -- the kill-credit criteria read, kill.go:1091/:1400), and a
    -- death-time zero would clobber the fight's latch before the
    -- achievement gate reads it; engage re-sets both.
    if creature:IsAlive() then
        creature:SetData(DATA_KNOCK_ON_WOOD, 0)
        creature:SetData(DATA_GETTING_BACK_TO_NATURE, 0)
    end
end

-- C++ KilledUnit: who->GetTypeId() == TYPEID_PLAYER, then
-- Talk(SAY_SLAY) — the razuvious player-gated variant
-- precedent.
local function freyaTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ DamageTaken: damage >= me->GetHealth() -> damage = 0
-- + manual JustDied(who) call; the manual JustDied's bridgeable
-- arms are Talk(SAY_DEATH) + DespawnOrUnsummon(7500ms)
-- (boss_freya.cpp:626) + the 2026-10-21 completion: SetReactState
-- REACT_PASSIVE, AttackStop, SetFaction(FACTION_FRIENDLY), and
-- CastSpell(me, SPELL_KNOCK_ON_WOOD_CREDIT 65074, true) as
-- creature:CastSpell(creature, spell, true) — the stale "no cast /
-- react-state / faction bridges" header claim retracted. The
-- numeric second return performs the C++-exact damage=0 rewrite
-- (the moroes damage-rewrite precedent; the hodir consequence
-- applies). cancelTimers models the C++ post-lethal stop (no
-- victim after RemoveAllAttackers/AttackStop; without the cancel
-- the Lua timers would fire forever on the unkillable body). The
-- 7.5s despawn is a fire-and-forget one-shot CreateLuaEvent —
-- C++-exact: C++ DespawnOrUnsummon rides the creature's own
-- despawn timer, not the EventMap, so it is deliberately NOT
-- tracked in the timers table (leave-combat/reset must not
-- cancel it, which C++ never does either). Still out: the chest
-- summon (summonSpell indexed by GetMap()->GetDifficulty() —
-- no 10/25 raid-size bridge; creature:IsHeroic is heroic-only).
-- InterruptNonMeleeSpells (no-op twin) and RemoveAllAttackers are ported
-- above in C++ JustDied order. The live-elder shutdown sweep runs
-- here, after the credit cast (C++ JustDied order: self legs, then
-- the elder sweep :630-641) — event 4 never fires on this path
-- because the damage rewrite keeps health above 0. The GUID-keyed
-- freyaFinished latch mirrors C++ JustDied's _encounterFinished
-- early-return (:602-606): later lethal hits only re-zero damage.
-- C++ Reset also early-returns when finished (:318-322), so the
-- latch is deliberately NOT cleared on Reset; a respawned Freya is
-- a new GUID.
local function freyaDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUIDLow()
    if damage >= creature:GetHealth() then
        if freyaFinished[guid] then
            return false, 0
        end
        freyaFinished[guid] = true
        creature:Talk(SAY_DEATH)
        creature:SetReactState(REACT_PASSIVE)
        creature:InterruptNonMeleeSpells()
        creature:RemoveAllAttackers()
        creature:AttackStop()
        creature:SetFaction(FACTION_FRIENDLY)
        cancelTimers(guid)
        CreateLuaEvent(function()
            creature:Despawn()
        end, 7500)
        creature:CastSpell(creature, SPELL_KNOCK_ON_WOOD_CREDIT, true)
        creature:SetBossState(BOSS_FREYA, 3) -- _JustDied() SetBossState(DATA_FREYA 9, DONE), PORTED tick 259 (C++ JustDied :628, after the credit cast, before the elder sweep)
        freyaElderShutdown(creature)
        return false, 0
    end
end

-- C++ Freya UpdateAI attuned sampling (:471-475) reads the
-- SPELL_ATTUNED_TO_NATURE 62519 aura stack; the stack only falls via
-- the REMOVE_*STACK casts onto Freya, each handled by the
-- dose-reduction SpellScript (:1547-1568, removes N stacks of 62519
-- when the aura is present): trio elementals cast
-- SPELL_REMOVE_10STACK 62525 on trio defeat (:520, one per elemental,
-- 30 per trio), a dying Detonating Lasher casts SPELL_REMOVE_2STACK
-- 62524 (SummonedCreatureDies :678), a dying Ancient Conservator casts
-- SPELL_REMOVE_25STACK 62521 (:684). Event 14 (OnHitBySpell) is the
-- target-side twin (faerlina ev14 precedent); the counter>0 guard is
-- the SpellScript's HasAura check. The casters are the ev19/ev21 wave
-- legs ported 2026-11-08, so the counter now decrements live; in a
-- zero-trio-defeat fight it stays 150 and the >= 25 gate reads true,
-- matching C++.
local function freyaHitBySpell(event, creature, caster, spellId)
    local remove = nil
    if spellId == SPELL_REMOVE_10STACK then
        remove = 10
    elseif spellId == SPELL_REMOVE_2STACK then
        remove = 2
    elseif spellId == SPELL_REMOVE_25STACK then
        remove = 25
    else
        return
    end
    local current = creature:GetData(DATA_GETTING_BACK_TO_NATURE)
    if current > 0 then
        creature:SetData(DATA_GETTING_BACK_TO_NATURE, math.max(0, current - remove))
    end
end

-- C++ JustEngagedWith (BossAI passthrough): Talk(SAY_ELDER_AGGRO)
-- unless the elder still carries SPELL_DRAINED_OF_POWER —
-- the auriaya engage-port precedent with the C++-exact aura
-- gate. C++ Reset() (:714-716 / :817-819 / :926-928)
-- schedules the elder timers; they start ticking on combat
-- here, matching the C++ UpdateAI (which early-returns
-- without a victim).
local function elderAggroTalk(creature)
    if not creature:HasAura(SPELL_DRAINED_OF_POWER) then
        creature:Talk(SAY_ELDER_AGGRO)
    end
end

-- Elder Brightleaf (:714-716, machine gated :740).
-- C++ EVENT_SOLAR_FLARE (:756-765): init 5s-7s, repeat
-- 5s-10s; me->CastSpell(me, SPELL_SOLAR_FLARE, args),
-- non-triggered self-cast — the SPELLVALUE_MAX_TARGETS =
-- flux-stack-count mod has no bridge (see header).
local function onSolarFlare(creature, guid)
    if elderTimersActive(creature) then
        if creature:IsCasting() then
            schedule(guid, "solar_flare", 500, function() onSolarFlare(creature, guid) end)
            return
        end
        creature:CastSpell(creature, SPELL_SOLAR_FLARE)
    end
    schedule(guid, "solar_flare", {5000, 10000}, function() onSolarFlare(creature, guid) end)
end

-- C++ EVENT_UNSTABLE_SUN_BEAM (:752-755): init 7s-12s,
-- repeat 10s-15s; me->CastSpell(me,
-- SPELL_UNSTABLE_SUN_BEAM_SUMMON, true) — triggered
-- self-cast.
local function onUnstableSunBeam(creature, guid)
    if elderTimersActive(creature) then
        if creature:IsCasting() then
            schedule(guid, "unstable_sun_beam", 500, function() onUnstableSunBeam(creature, guid) end)
            return
        end
        creature:CastSpell(creature, SPELL_UNSTABLE_SUN_BEAM_SUMMON, true)
    end
    schedule(guid, "unstable_sun_beam", {10000, 15000}, function() onUnstableSunBeam(creature, guid) end)
end

-- C++ EVENT_FLUX (:767-772): init 5s, repeat 7500ms;
-- RemoveAurasDueToSpell + AddAura(SPELL_FLUX_AURA) —
-- SetStackAmount(urand(1, 8)) has no bridge (only the read side,
-- creature:GetStackAmount, was built 2026-10-23; the Go aura
-- model is presence-only), so the aura is re-applied as-is.
local function onFlux(creature, guid)
    if elderTimersActive(creature) then
        if creature:IsCasting() then
            schedule(guid, "flux", 500, function() onFlux(creature, guid) end)
            return
        end
        creature:RemoveAura(SPELL_FLUX_AURA)
        creature:AddAura(SPELL_FLUX_AURA)
    end
    schedule(guid, "flux", 7500, function() onFlux(creature, guid) end)
end

local function brightleafEnterCombat(event, creature, target)
    creature:SetBossState(ELDER_BOSS_DATA[ENTRY_BRIGHTLEAF], 1) -- BossAI::JustEngagedWith SetBossState(BOSS_BRIGHTLEAF 14, IN_PROGRESS), base-first (boss_freya.cpp:733; PORTED tick 259)
    creature:SetCombatPulseDelay(5) -- BossAI::JustEngagedWith arm 1 (ScriptedCreature.cpp:540) -- base-first (boss_freya.cpp:733)
    creature:DoZoneInCombat() -- BossAI::JustEngagedWith me-form arm (ScriptedCreature.cpp:542)
    elderAggroTalk(creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "solar_flare", {5000, 7000}, function() onSolarFlare(creature, guid) end)
    schedule(guid, "unstable_sun_beam", {7000, 12000}, function() onUnstableSunBeam(creature, guid) end)
    schedule(guid, "flux", 5000, function() onFlux(creature, guid) end)
end

-- Elder Stonebark (:817-819, machine gated :858).
-- C++ EVENT_BARK (:870-873): init 37500ms-40s, repeat
-- 30s-50s; DoCast(me, SPELL_PETRIFIED_BARK), non-triggered
-- self-cast.
local function onBark(creature, guid)
    if elderTimersActive(creature) then
        if creature:IsCasting() then
            schedule(guid, "bark", 500, function() onBark(creature, guid) end)
            return
        end
        creature:CastSpell(creature, SPELL_PETRIFIED_BARK)
    end
    schedule(guid, "bark", {30000, 50000}, function() onBark(creature, guid) end)
end

-- C++ EVENT_FISTS (:874-877): init 25s-35s, repeat 20s-30s;
-- DoCastVictim(SPELL_FISTS_OF_STONE), non-triggered.
local function onFists(creature, guid)
    if elderTimersActive(creature) then
        if creature:IsCasting() then
            schedule(guid, "fists", 500, function() onFists(creature, guid) end)
            return
        end
        creature:CastSpell(nil, SPELL_FISTS_OF_STONE)
    end
    schedule(guid, "fists", {20000, 30000}, function() onFists(creature, guid) end)
end

-- C++ EVENT_TREMOR (:878-882): init 10s-12s, repeat
-- 10s-20s; if (!me->HasAura(SPELL_FISTS_OF_STONE))
-- DoCastVictim(SPELL_GROUND_TREMOR), non-triggered.
local function onTremor(creature, guid)
    if elderTimersActive(creature) and not creature:HasAura(SPELL_FISTS_OF_STONE) then
        if creature:IsCasting() then
            schedule(guid, "tremor", 500, function() onTremor(creature, guid) end)
            return
        end
        creature:CastSpell(nil, SPELL_GROUND_TREMOR)
    end
    schedule(guid, "tremor", {10000, 20000}, function() onTremor(creature, guid) end)
end

local function stonebarkEnterCombat(event, creature, target)
    creature:SetBossState(ELDER_BOSS_DATA[ENTRY_STONEBARK], 1) -- BossAI::JustEngagedWith SetBossState(BOSS_STONEBARK 16, IN_PROGRESS), base-first (boss_freya.cpp:836; PORTED tick 259)
    creature:SetCombatPulseDelay(5) -- BossAI::JustEngagedWith arm 1 (ScriptedCreature.cpp:540) -- base-first (boss_freya.cpp:836)
    creature:DoZoneInCombat() -- BossAI::JustEngagedWith me-form arm (ScriptedCreature.cpp:542)
    elderAggroTalk(creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "tremor", {10000, 12000}, function() onTremor(creature, guid) end)
    schedule(guid, "fists", {25000, 35000}, function() onFists(creature, guid) end)
    schedule(guid, "bark", {37500, 40000}, function() onBark(creature, guid) end)
end

-- Elder Ironbranch (:926-928, machine gated :952).
-- C++ EVENT_IMPALE (:964-967): init 18s-22s, repeat
-- 15s-25s; DoCastVictim(SPELL_IMPALE), non-triggered.
local function onImpale(creature, guid)
    if elderTimersActive(creature) then
        if creature:IsCasting() then
            schedule(guid, "impale", 500, function() onImpale(creature, guid) end)
            return
        end
        creature:CastSpell(nil, SPELL_IMPALE)
    end
    schedule(guid, "impale", {15000, 25000}, function() onImpale(creature, guid) end)
end

-- C++ EVENT_THORN_SWARM (:973-976): init 7500ms-12500ms,
-- repeat 8s-13s; DoCastVictim(SPELL_THORN_SWARM),
-- non-triggered.
local function onThornSwarm(creature, guid)
    if elderTimersActive(creature) then
        if creature:IsCasting() then
            schedule(guid, "thorn_swarm", 500, function() onThornSwarm(creature, guid) end)
            return
        end
        creature:CastSpell(nil, SPELL_THORN_SWARM)
    end
    schedule(guid, "thorn_swarm", {8000, 13000}, function() onThornSwarm(creature, guid) end)
end

-- C++ EVENT_IRON_ROOTS (boss_freya.cpp:968-972): random
-- player 100y (playerOnly, -SPELL_ROOTS_IRONBRANCH
-- aura-filter), target->CastSpell(target,
-- SPELL_ROOTS_IRONBRANCH 62438, true) — triggered;
-- init 12s-17s (:927), repeat 10s-20s. The aura filter rides
-- randomPlayerInRange's excludeAura (player:HasAura); the
-- target self-casts (player:CastSpell, vezax precedent).
local function onIronRoots(creature, guid)
    if elderTimersActive(creature) then
        if creature:IsCasting() then
            schedule(guid, "iron_roots", 500, function() onIronRoots(creature, guid) end)
            return
        end
        local target = randomPlayerInRange(creature, 0, 100, SPELL_ROOTS_IRONBRANCH)
        if target then
            target:CastSpell(target, SPELL_ROOTS_IRONBRANCH, true)
        end
    end
    schedule(guid, "iron_roots", {10000, 20000}, function() onIronRoots(creature, guid) end)
end

local function ironbranchEnterCombat(event, creature, target)
    creature:SetBossState(ELDER_BOSS_DATA[ENTRY_IRONBRANCH], 1) -- BossAI::JustEngagedWith SetBossState(BOSS_IRONBRANCH 15, IN_PROGRESS), base-first (boss_freya.cpp:945; PORTED tick 259)
    creature:SetCombatPulseDelay(5) -- BossAI::JustEngagedWith arm 1 (ScriptedCreature.cpp:540) -- base-first (boss_freya.cpp:945)
    creature:DoZoneInCombat() -- BossAI::JustEngagedWith me-form arm (ScriptedCreature.cpp:542)
    elderAggroTalk(creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "impale", {18000, 22000}, function() onImpale(creature, guid) end)
    schedule(guid, "thorn_swarm", {7500, 12500}, function() onThornSwarm(creature, guid) end)
    schedule(guid, "iron_roots", {12000, 17000}, function() onIronRoots(creature, guid) end)
end

local function elderLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ elder Reset (:709-713 / :812-816 / :921-925): if
-- (me->HasAura(SPELL_DRAINED_OF_POWER)) me->
-- RemoveAurasDueToSpell(SPELL_DRAINED_OF_POWER) — creature:HasAura
-- + creature:RemoveAura (moorabi precedent); shared by all three
-- elders (one handler, three registrations).
local function elderReset(event, creature)
    creature:SetBossState(ELDER_BOSS_DATA[creature:GetEntry()], 0) -- _Reset() SetBossState(elder DATA, NOT_STARTED), first per C++ :711/:814/:923 (PORTED tick 259)
    cancelTimers(creature:GetGUIDLow())
    if creature:HasAura(SPELL_DRAINED_OF_POWER) then
        creature:RemoveAura(SPELL_DRAINED_OF_POWER)
    end
end

-- C++ KilledUnit: who->GetTypeId() == TYPEID_PLAYER, then
-- Talk(SAY_ELDER_SLAY) — the razuvious player-gated variant
-- precedent.
local function elderTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_ELDER_SLAY)
    end
end

-- C++ JustDied: _JustDied() passthrough, then
-- Talk(SAY_ELDER_DEATH) — the sjonnir JustDied-Talk
-- precedent.
local function elderDied(event, creature, killer)
    creature:SetBossState(ELDER_BOSS_DATA[creature:GetEntry()], 3) -- _JustDied() SetBossState(elder DATA, DONE), first per C++ :725/:828/:937 (PORTED tick 259)
    cancelTimers(creature:GetGUIDLow())
    creature:Talk(SAY_ELDER_DEATH)
end

-- C++ elder DoAction ACTION_ELDER_FREYA_KILLED (boss_freya.cpp:783-790):
-- DespawnOrUnsummon(10s) + _JustDied() SetBossState(elder DATA, DONE).
-- Sent by freyaElderShutdown on Freya's death (C++ :639); the ev38
-- dispatch is bridged (lua_creature_events.go:802). PORTED tick 259.
local function elderDoAction(event, creature, action)
    if action ~= ACTION_ELDER_FREYA_KILLED then
        return
    end
    creature:DespawnOrUnsummon(10000)
    creature:SetBossState(ELDER_BOSS_DATA[creature:GetEntry()], 3)
end

-- C++ boss_stonebarkAI::DamageTaken (boss_freya.cpp:841-855): if
-- the attacker exists, isn't self (GUID compare, malchezaar
-- precedent), and Stonebark carries SPELL_PETRIFIED_BARK, the
-- attacker self-casts SPELL_PETRIFIED_BARK_DMG 62379, triggered,
-- and damage = 0. Only the AddSpellBP0(damage) mod stays out
-- (no extra-args bridge).
local function stonebarkDamageTaken(event, creature, attacker, damage)
    if attacker and attacker:GetGUID() ~= creature:GetGUID()
            and creature:HasAura(SPELL_PETRIFIED_BARK) then
        attacker:CastSpell(attacker, SPELL_PETRIFIED_BARK_DMG, true)
        return false, 0
    end
end

RegisterCreatureEvent(ENTRY_FREYA, 1, freyaEnterCombat)
RegisterCreatureEvent(ENTRY_FREYA, 2, freyaLeaveCombat)
RegisterCreatureEvent(ENTRY_FREYA, 3, freyaTargetDied)
RegisterCreatureEvent(ENTRY_FREYA, 4, freyaDied)
RegisterCreatureEvent(ENTRY_FREYA, 9, freyaDamageTaken)
RegisterCreatureEvent(ENTRY_FREYA, 14, freyaHitBySpell)
RegisterCreatureEvent(ENTRY_FREYA, 19, freyaJustSummoned)
RegisterCreatureEvent(ENTRY_FREYA, 21, freyaSummonedDied)
RegisterCreatureEvent(ENTRY_FREYA, 23, freyaReset)

RegisterCreatureEvent(ENTRY_BRIGHTLEAF, 1, brightleafEnterCombat)
RegisterCreatureEvent(ENTRY_BRIGHTLEAF, 2, elderLeaveCombat)
RegisterCreatureEvent(ENTRY_BRIGHTLEAF, 3, elderTargetDied)
RegisterCreatureEvent(ENTRY_BRIGHTLEAF, 4, elderDied)
RegisterCreatureEvent(ENTRY_BRIGHTLEAF, 23, elderReset)
RegisterCreatureEvent(ENTRY_BRIGHTLEAF, 38, elderDoAction)

RegisterCreatureEvent(ENTRY_STONEBARK, 1, stonebarkEnterCombat)
RegisterCreatureEvent(ENTRY_STONEBARK, 2, elderLeaveCombat)
RegisterCreatureEvent(ENTRY_STONEBARK, 3, elderTargetDied)
RegisterCreatureEvent(ENTRY_STONEBARK, 4, elderDied)
RegisterCreatureEvent(ENTRY_STONEBARK, 9, stonebarkDamageTaken)
RegisterCreatureEvent(ENTRY_STONEBARK, 23, elderReset)
RegisterCreatureEvent(ENTRY_STONEBARK, 38, elderDoAction)

RegisterCreatureEvent(ENTRY_IRONBRANCH, 1, ironbranchEnterCombat)
RegisterCreatureEvent(ENTRY_IRONBRANCH, 2, elderLeaveCombat)
RegisterCreatureEvent(ENTRY_IRONBRANCH, 3, elderTargetDied)
RegisterCreatureEvent(ENTRY_IRONBRANCH, 4, elderDied)
RegisterCreatureEvent(ENTRY_IRONBRANCH, 23, elderReset)
RegisterCreatureEvent(ENTRY_IRONBRANCH, 38, elderDoAction)

-- C++ npc_ancient_conservator::JustEngagedWith
-- (boss_freya.cpp:1293-1296): DoCast(who, SPELL_CONSERVATOR_GRIP
-- 62532), triggered — who is the engaging unit, bridged as the
-- event 1 target arg (creature:CastSpell(target, spell, true)
-- per the file doctrine).
local function conservatorEnterCombat(event, creature, target)
    creature:CastSpell(target, SPELL_CONSERVATOR_GRIP, true)
end

RegisterCreatureEvent(ENTRY_ANCIENT_CONSERVATOR, 1, conservatorEnterCombat)

-- C++ npc_iron_rootsAI::JustDied (boss_freya.cpp:243-253): the
-- summoner aura-removal legs ride the summonerGUID member (no
-- bridge, stay out); me->DespawnOrUnsummon() ports
-- (creature:Despawn, bridged).
local function ironRootsJustDied(event, creature, killer)
    creature:Despawn()
end

RegisterCreatureEvent(ENTRY_IRON_ROOTS, 4, ironRootsJustDied)

-- C++ npc_unstable_sun_beamAI ctor (boss_freya.cpp:1487-1496):
-- SetCombatMovement(false) + despawnTimer urand(7000, 12000) +
-- DoCast(me, SPELL_PHOTOSYNTHESIS 62209) + DoCast(me,
-- SPELL_UNSTABLE_SUN_BEAM 62211) + SetReactState(REACT_PASSIVE);
-- UpdateAI (:1498-1507): on timer expiry DoCastAOE(
-- SPELL_UNSTABLE_ENERGY 62217, true) + DisappearAndDie(). The
-- DoCastAOE self-cast convention: CastSpell(creature, spell,
-- true); Despawn() is the DisappearAndDie twin. The 7-12s timer
-- is a one-shot CreateLuaEvent, deliberately NOT tracked in the
-- timers table — the beam has no leave-combat/reset path and the
-- C++ despawn rides the creature's own timer too. Caveat: the
-- strip leg is inert until aura application exists (the Go
-- creature-cast path applies no aura effects); the lifecycle
-- half (the beam currently never despawns) is the observable
-- win. The 62207 summon spell materializing 33050 is DBC-side
-- and unverified in-repo (no Spell.dbc in the workspace).
local function unstableSunBeamSpawn(event, creature)
    creature:SetCombatMovement(false)
    creature:CastSpell(creature, SPELL_PHOTOSYNTHESIS)
    creature:CastSpell(creature, SPELL_UNSTABLE_SUN_BEAM)
    creature:SetReactState(REACT_PASSIVE)
    CreateLuaEvent(function()
        creature:CastSpell(creature, SPELL_UNSTABLE_ENERGY, true)
        creature:Despawn()
    end, {7000, 12000})
end

-- C++ npc_unstable_sun_beamAI::SpellHitTarget
-- (boss_freya.cpp:1509-1520): if spellId == SPELL_UNSTABLE_ENERGY
-- (62217) the target loses SPELL_UNSTABLE_SUN_BEAM (62211) and
-- SPELL_UNSTABLE_SUN_BEAM_TRIGGERED (62243); the ToUnit nil-guard
-- is the target nil-check (ev15 arg order verified against
-- boss_grobbulus.lua grobbulusSpellHitTarget).
local function unstableSunBeamSpellHitTarget(event, creature, target, spellId)
    if spellId == SPELL_UNSTABLE_ENERGY and target then
        target:RemoveAura(SPELL_UNSTABLE_SUN_BEAM)
        target:RemoveAura(SPELL_UNSTABLE_SUN_BEAM_TRIGGERED)
    end
end

RegisterCreatureEvent(ENTRY_UNSTABLE_SUN_BEAM, 5, unstableSunBeamSpawn)
RegisterCreatureEvent(ENTRY_UNSTABLE_SUN_BEAM, 15, unstableSunBeamSpellHitTarget)
