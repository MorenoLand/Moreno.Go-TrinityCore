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
-- (player-gated), 4 OnDied, 9 OnDamageTaken, 23 OnReset. Timers via
-- CreateLuaEvent (GUID-keyed, cancelled on events 2/4/23); melee is
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
--   (:308). The CheckRequiredBosses evade branch, DoCastSpellOnPlayers,
--   DoZoneInCombat, instance->SetBossState(IN_PROGRESS),
--   DoRemoveAurasDueToSpellOnPlayers legs have no instance bridge;
--   the !_introDone DoCastSelf(SPELL_GRIP_OF_AGONY 70572) exploit
--   leg rides the unbridged intro DoAction machine (see
--   documented-only).
-- EVENT_SUMMON_BLOOD_BEAST (:493-502) — DoCastSelf(
--   SPELL_SUMMON_BLOOD_BEAST+i10 72172/72173) = 2x non-triggered
--   self-cast, then Talk(SAY_BLOOD_BEASTS 9) (:498); re-arm 40s
--   unconditional (:499). The 25-man trio DoCastSelf(
--   SPELL_SUMMON_BLOOD_BEAST_25_MAN+i25 72356/72357/72358) (:495-497)
--   stays out: no difficulty bridge (the faerlina precedent).
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
--   one-shot seeded on engage (the marrowgar berserk precedent).
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
--   DoCastAOE(SPELL_REMOVE_MARKS_OF_THE_FALLEN_CHAMPION 72257) =
--   non-triggered self-cast (:370), DoCastSelf(SPELL_ACHIEVEMENT
--   72928, true) TRIGGERED (:371), Talk(SAY_DEATH 13) (:372),
--   DoCastSelf(SPELL_REPUTATION_BOSS_KILL 73843, true) TRIGGERED
--   (:373), DoCastSelf(SPELL_PERMANENT_FEIGN_DEATH 70628) (:374).
--   C++ order kept (frenzy check before fight-won; both may fire on
--   one hit). The _EnterEvadeMode / SetFlag(NOT_SELECTABLE) /
--   SetImmuneToPC / RemoveAurasOnEvade legs and the cross-AI
--   DoAction(ACTION_START_OUTRO) leg have no bridge.
-- Reset (:257-273) — C++ schedules no timers here, so event 23 only
--   cancels timers and replays the bridgeable aura-setup arms:
--   DoCastSelf(SPELL_ZERO_POWER 72242 / SPELL_BLOOD_LINK 72178 /
--   SPELL_BLOOD_POWER 72371 /
--   SPELL_MARK_OF_THE_FALLEN_CHAMPION_S 72256 /
--   SPELL_RUNE_OF_BLOOD_S 72408, true) all TRIGGERED (:264-268) +
--   RemoveAurasDueToSpell(SPELL_BERSERK / SPELL_FRENZY) via the
--   RemoveAura bridge (the arlokk / bronjahm idiom). The _Reset() /
--   SetPower(ENERGY, 0) / SetPhase bookkeeping legs have no bridge.
--   Mirrors the `if (_dead) return;` early-out (:259-260): after the
--   fight-won leg the reset casts are skipped.
-- JustDied (:315-317) is EMPTY in C++ — event 4 only cancels timers
--   and clears state (the sjonnir JustDied-Talk precedent does NOT
--   apply; SAY_DEATH 13 rides the DamageTaken fight-won leg above).
--
-- DOCUMENTED-ONLY (no registration beyond entry 37813):
-- boss_deathbringer_saurfang SpellHitTarget (:447-477) —
--   Talk(SAY_MARK_OF_THE_FALLEN_CHAMPION 8) rides the
--   SPELL_MARK_OF_THE_FALLEN_CHAMPION case of the SpellHitTarget
--   switch; the 72255/72444/72445/72446 triggered-mark cases cast
--   SPELL_BLOOD_LINK_DUMMY — stays out on trigger-path grounds: the
--   72293 mark cast rides the unported ACTION_MARK_OF_THE_FALLEN_CHAMPION
--   DoAction machine (no DoAction / random-target bridges; its trigger
--   is an unbridged AuraScript), so event 15 never fires for it in Go
--   (event 15 itself is live — boss_ai.go:1904).
-- boss_deathbringer_saurfang intro machine — Talk(
--   SAY_INTRO_ALLIANCE_2 0 / SAY_INTRO_ALLIANCE_3 1 /
--   SAY_INTRO_ALLIANCE_6 2 / SAY_INTRO_ALLIANCE_7 3 /
--   SAY_INTRO_HORDE_2 4 / SAY_INTRO_HORDE_4 5 /
--   SAY_INTRO_HORDE_9 6) ride the EVENT_INTRO_* scheduler machine
--   (:540-571, intro phase machine — the gothik precedent); the
--   DoAction(PHASE_INTRO_A/H / ACTION_CONTINUE_INTRO) legs (:582-603)
--   have no DoAction bridge (the krick ACTION_OUTRO precedent); the
--   ACTION_MARK_OF_THE_FALLEN_CHAMPION leg (:604-615) is a
--   random-target SelectTarget with an aura-exclusion filter +
--   DoCast — no DoAction bridge, no random-target bridge, and its
--   trigger (the blood-link aura periodic tick) is an AuraScript
--   (no bridge); the _fallenChampionCastCount /
--   GetData(DATA_MADE_A_MESS) achievement legs ride with it.
-- boss_deathbringer_saurfang JustSummoned (:395-411) — summon
--   machine: random-target AttackStart, SPELL_BLOOD_LINK_BEAST 72176
--   + SPELL_RESISTANT_SKIN 72723 triggered casts on the summon,
--   summons.Summon, DoZoneInCombat — no summon / cross-creature
--   bridge. SummonedCreatureDies (:413-416), MovementInform
--   (:418-424, instance GO leg), AttackStart/CanAIAttack transport
--   legs, EnterEvadeMode/JustReachedHome instance legs
--   (SetBossState FAIL / DoRemoveAurasDueToSpellOnPlayers) — no
--   instance / motion bridges.
-- EVENT_SCENT_OF_BLOOD (:520-526) — heroic-gated (no difficulty
--   bridge) AND gated on !summons.empty() (no summon-list bridge);
--   one-shot, never re-armed — out on both counts.
-- npc_high_overlord_saurfang_icc / npc_muradin_bronzebeard_icc —
--   zero bridgeable arms, NOT registered (the bronjahm
--   npc_corrupted_soul_fragment precedent): gossip -> DoAction(
--   ACTION_START_EVENT) (gossip is instance-model-blocked; no
--   DoAction bridge); Talk(SAY_INTRO_HORDE_1 0 /
--   SAY_INTRO_ALLIANCE_1 0) ride the DoAction legs; Talk(
--   SAY_INTRO_HORDE_3 1 / SAY_INTRO_ALLIANCE_4 1) ride
--   MovementInform POINT_FIRST_STEP (no-motion bridge); Talk(
--   SAY_INTRO_HORDE_5..8 2..5 / SAY_INTRO_ALLIANCE_5 2) and Talk(
--   SAY_OUTRO_HORDE_2..4 12..14) ride scheduler events (intro/outro
--   machines); Talk(SAY_OUTRO_HORDE_1 11 / SAY_OUTRO_ALLIANCE_1 3)
--   ride DoAction(ACTION_START_OUTRO); SpellHit(SPELL_GRIP_OF_AGONY)
--   movement legs — SpellHit never fires; the cross-AI
--   deathbringer->AI()->DoAction(PHASE_INTRO_*/ACTION_CONTINUE_INTRO)
--   legs have no bridge.
-- npc_saurfang_event (guards) — zero own Talk lines; SpellHit /
--   DoAction(ACTION_CHARGE / ACTION_DESPAWN) movement legs only —
--   NOT registered (the bronjahm npc_corrupted_soul_fragment
--   precedent).
-- spell_deathbringer_blood_link / spell_deathbringer_blood_power /
-- spell_deathbringer_rune_of_blood / spell_deathbringer_blood_nova /
-- spell_deathbringer_blood_nova_targeting /
-- spell_deathbringer_boiling_blood / spell_deathbringer_remove_marks
-- join the no-SpellScript-bridge queue (the boss_moragg
-- optic-link precedent); spell_deathbringer_blood_link_aura /
-- spell_deathbringer_blood_power_aura /
-- spell_deathbringer_blood_beast_blood_link join the
-- no-AuraScript-bridge queue.
-- achievement_ive_gone_and_made_a_mess joins the no-achievement
-- bridge queue (the gunship achievement_im_on_a_boat precedent).
-- All 29 Talk() calls in the file accounted for (29 = ported: aggro
-- 7 + kill 10 + frenzy 11 + death 13 + blood beasts 9 + berserk 12;
-- documented-only: boss intro alliance 0-3 (4) + boss intro horde
-- 4-6 (3) + mark 8 (1) + emote 14 (1) + horde saurfang 0/1/2..5/
-- 11..14 (10) + muradin 0/1/2/3 (4)).
--
-- DEVIATIONS FROM C++ (documented; every other value C++-exact):
-- EVENT_BERSERK seed: C++ IsHeroic() ? 360s : 480s (:305); no
--   difficulty bridge (the faerlina precedent) — ported at the
--   non-heroic 480000ms. Heroic fights enrage 2min early vs C++.
-- EVENT_SCENT_OF_BLOOD (heroic-only, :520-526) not ported at all:
--   no difficulty bridge + no summon-list bridge for the
--   !summons.empty() gate.
-- EVENT_SUMMON_BLOOD_BEAST 25-man trio (72356/72357/72358) not
--   ported: no difficulty bridge (the faerlina precedent).
-- The UNIT_STATE_CASTING gate in UpdateAI (:534-535) has no bridge
--   (the maiden precedent: timers fire unconditionally).
-- The intro exploit-engage DoCastSelf(SPELL_GRIP_OF_AGONY) leg
--   (:293-298) rides the unbridged intro DoAction machine (_introDone
--   latch has no bridge).
-- Lua OnReset (event 23) replays the Reset aura-setup casts; C++
--   Reset also runs at initial spawn — if the engine never fires 23
--   pre-combat, the setup auras apply on the first evade instead.

local ENTRY_DEATHBRINGER_SAURFANG = 37813

local SAY_AGGRO = 7
local SAY_BLOOD_BEASTS = 9
local SAY_KILL = 10
local SAY_FRENZY = 11
local SAY_BERSERK = 12
local SAY_DEATH = 13

local SPELL_BERSERK = 26662 -- icecrown_citadel.h:33
local SPELL_SUMMON_BLOOD_BEAST = 72172
local SPELL_FRENZY = 72737
local SPELL_BLOOD_NOVA_TRIGGER = 72378
local SPELL_BOILING_BLOOD = 72385
local SPELL_RUNE_OF_BLOOD = 72410
local SPELL_REMOVE_MARKS_OF_THE_FALLEN_CHAMPION = 72257
local SPELL_ACHIEVEMENT = 72928
local SPELL_REPUTATION_BOSS_KILL = 73843 -- icecrown_citadel.h:35
local SPELL_PERMANENT_FEIGN_DEATH = 70628
local SPELL_ZERO_POWER = 72242
local SPELL_BLOOD_LINK = 72178
local SPELL_BLOOD_POWER = 72371
local SPELL_MARK_OF_THE_FALLEN_CHAMPION_S = 72256
local SPELL_RUNE_OF_BLOOD_S = 72408

local FIGHT_WON_VALUE = 100000 -- boss_deathbringer_saurfang.cpp:614

local timers = {}
local saurfangState = {}

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
    saurfangState[guid] = { frenzied = false, dead = false }
end

local function clearState(guid)
    cancelTimers(guid)
    saurfangState[guid] = nil
end

-- C++ :493-502 EVENT_SUMMON_BLOOD_BEAST: 2x DoCastSelf(
-- SPELL_SUMMON_BLOOD_BEAST+i10 72172/72173) non-triggered, then
-- Talk(SAY_BLOOD_BEASTS 9); re-arm 40s unconditional. The 25-man
-- trio (72356+i25, :495-497) stays out: no difficulty bridge. The
-- heroic-only EVENT_SCENT_OF_BLOOD schedule (:500-501) stays out:
-- no difficulty bridge + no summon-list bridge for !summons.empty().
local function onSummonBloodBeast(creature, guid)
    creature:CastSpell(creature, SPELL_SUMMON_BLOOD_BEAST)
    creature:CastSpell(creature, SPELL_SUMMON_BLOOD_BEAST + 1)
    creature:Talk(SAY_BLOOD_BEASTS)
    schedule(guid, "beasts", 40000, function() onSummonBloodBeast(creature, guid) end)
end

-- C++ :516-519 EVENT_BERSERK: DoCastSelf(SPELL_BERSERK 26662)
-- non-triggered + Talk(SAY_BERSERK 12), cast-before-Talk kept;
-- one-shot. Seed is 480000ms: C++ IsHeroic() ? 360s : 480s (:305)
-- has no difficulty bridge — ported at the non-heroic value
-- (documented deviation).
local function onBerserk(creature, guid)
    creature:CastSpell(creature, SPELL_BERSERK)
    creature:Talk(SAY_BERSERK)
end

-- C++ :512-515 EVENT_BOILING_BLOOD: DoCastSelf(SPELL_BOILING_BLOOD
-- 72385) non-triggered; re-arm {15s,20s} randtime.
local function onBoilingBlood(creature, guid)
    creature:CastSpell(creature, SPELL_BOILING_BLOOD)
    schedule(guid, "boiling", {15000, 20000}, function() onBoilingBlood(creature, guid) end)
end

-- C++ :504-507 EVENT_BLOOD_NOVA: DoCastAOE(SPELL_BLOOD_NOVA_TRIGGER
-- 72378) = non-triggered self-cast; re-arm {20s,25s} randtime.
local function onBloodNova(creature, guid)
    creature:CastSpell(creature, SPELL_BLOOD_NOVA_TRIGGER)
    schedule(guid, "nova", {20000, 25000}, function() onBloodNova(creature, guid) end)
end

-- C++ :508-511 EVENT_RUNE_OF_BLOOD: DoCastVictim(SPELL_RUNE_OF_BLOOD
-- 72410) = non-triggered victim-cast; re-arm {20s,25s} randtime.
local function onRuneOfBlood(creature, guid)
    creature:CastSpell(nil, SPELL_RUNE_OF_BLOOD)
    schedule(guid, "rune", {20000, 25000}, function() onRuneOfBlood(creature, guid) end)
end

-- C++ :275-313 JustEngagedWith: Talk(SAY_AGGRO 7) (:302), then the
-- five combat scheduler seeds (:304-308). The CheckRequiredBosses
-- evade branch, DoCastSpellOnPlayers, DoZoneInCombat,
-- instance->SetBossState(IN_PROGRESS) and
-- DoRemoveAurasDueToSpellOnPlayers legs have no instance bridge;
-- the !_introDone DoCastSelf(SPELL_GRIP_OF_AGONY) exploit leg
-- (:293-298) rides the unbridged intro DoAction machine.
local function saurfangJustEngagedWith(event, creature, target)
    local guid = creature:GetGUID()
    initState(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "beasts", 30000, function() onSummonBloodBeast(creature, guid) end)
    schedule(guid, "berserk", 480000, function() onBerserk(creature, guid) end)
    schedule(guid, "boiling", 15500, function() onBoilingBlood(creature, guid) end)
    schedule(guid, "nova", 17000, function() onBloodNova(creature, guid) end)
    schedule(guid, "rune", 20000, function() onRuneOfBlood(creature, guid) end)
end

local function saurfangLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ :257-273 Reset: no timer seeds in C++, so event 23 only
-- cancels timers and replays the bridgeable aura-setup arms — the
-- five TRIGGERED self-casts (:264-268) and the
-- RemoveAurasDueToSpell(SPELL_BERSERK / SPELL_FRENZY) removals via
-- the RemoveAura bridge (the arlokk / bronjahm idiom). Mirrors the
-- `if (_dead) return;` early-out (:259-260): after the fight-won
-- leg the reset casts are skipped.
local function saurfangReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    local st = saurfangState[guid]
    if st and st.dead then
        return
    end
    saurfangState[guid] = { frenzied = false, dead = false }
    creature:CastSpell(creature, SPELL_ZERO_POWER, true)
    creature:CastSpell(creature, SPELL_BLOOD_LINK, true)
    creature:CastSpell(creature, SPELL_BLOOD_POWER, true)
    creature:CastSpell(creature, SPELL_MARK_OF_THE_FALLEN_CHAMPION_S, true)
    creature:CastSpell(creature, SPELL_RUNE_OF_BLOOD_S, true)
    creature:RemoveAura(SPELL_BERSERK)
    creature:RemoveAura(SPELL_FRENZY)
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
    clearState(creature:GetGUID())
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
-- SetFlag(NOT_SELECTABLE) / SetImmuneToPC / RemoveAurasOnEvade legs
-- and the cross-AI DoAction(ACTION_START_OUTRO) leg have no bridge.
local function saurfangDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local st = saurfangState[guid]
    if not st then
        return
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
        creature:CastSpell(creature, SPELL_REMOVE_MARKS_OF_THE_FALLEN_CHAMPION)
        creature:CastSpell(creature, SPELL_ACHIEVEMENT, true)
        creature:Talk(SAY_DEATH)
        creature:CastSpell(creature, SPELL_REPUTATION_BOSS_KILL, true)
        creature:CastSpell(creature, SPELL_PERMANENT_FEIGN_DEATH)
    end
    if newDamage ~= damage then
        return true, newDamage
    end
end

RegisterCreatureEvent(ENTRY_DEATHBRINGER_SAURFANG, 1, saurfangJustEngagedWith)
RegisterCreatureEvent(ENTRY_DEATHBRINGER_SAURFANG, 2, saurfangLeaveCombat)
RegisterCreatureEvent(ENTRY_DEATHBRINGER_SAURFANG, 3, saurfangKilledUnit)
RegisterCreatureEvent(ENTRY_DEATHBRINGER_SAURFANG, 4, saurfangJustDied)
RegisterCreatureEvent(ENTRY_DEATHBRINGER_SAURFANG, 9, saurfangDamageTaken)
RegisterCreatureEvent(ENTRY_DEATHBRINGER_SAURFANG, 23, saurfangReset)
