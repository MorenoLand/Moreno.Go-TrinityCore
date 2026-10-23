-- Lady Deathwhisper (Icecrown Citadel) — Lua port of
-- src/server/scripts/Northrend/IcecrownCitadel/boss_lady_deathwhisper.cpp
-- (1071 lines incl. license; 9 scripts — 5 CreatureScripts
-- (boss_lady_deathwhisper (BossAI, DATA_LADY_DEATHWHISPER = 1),
-- npc_cult_fanatic (ScriptedAI trash), npc_cult_adherent (ScriptedAI
-- trash), npc_vengeful_shade (ScriptedAI summon),
-- npc_darnavan (ScriptedAI ally)) + 4 SpellScripts
-- (spell_deathwhisper_mana_barrier (AuraScript),
-- spell_deathwhisper_dominated_mind (AuraScript),
-- spell_deathwhisper_summon_spirits (SpellScript),
-- spell_deathwhisper_vampiric_might (AuraScript)) + 1
-- AreaTriggerScript (at_lady_deathwhisper_entrance); all registered
-- from inside AddSC_boss_lady_deathwhisper(); loader decl 172 / call
-- 367 per northrend_script_loader.cpp — the SECOND group of the
-- "// Icecrown Citadel" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_lord_marrowgar() (call 366); the call after it is
-- verified from the loader this run; the checkpoint sequence
-- (boss_lord_marrowgar -> boss_lady_deathwhisper) is followed).
-- Entry: 36855 Lady Deathwhisper (icecrown_citadel.h
-- NPC_LADY_DEATHWHISPER, line 174; instance_icecrown_citadel.cpp
-- OnCreatureCreate binds case NPC_LADY_DEATHWHISPER, line 224) —
-- entry-verifiable, registration proceeds (the nexus_commanders
-- kalecgos precedent); the CreatureScript ScriptName binding is
-- DB-side as usual.
-- Entries: 38472 / 38485 Darnavan (boss_lady_deathwhisper.cpp
-- enum, lines 154-155: NPC_DARNAVAN_10 = 38472, NPC_DARNAVAN_25 =
-- 38485; NPC_DARNAVAN = RAID_MODE(38472, 38485, 38472, 38485)) —
-- entry-verifiable from the C++ enum, registration proceeds (the
-- raid-mode dual-entry pattern); ScriptName binding is DB-side.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_lady_deathwhisper" hits boss_lady_deathwhisper.cpp
-- only (+ the loader decl/call lines); this clone carries no sql/
-- tree, so ScriptName bindings are DB-side by construction.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnKill
-- (player-gated), 4 OnDied, 23 OnReset. Timers via CreateLuaEvent
-- (GUID-keyed, cancelled on events 2/23); melee is engine-driven in Go.
--
-- DOCTRINE RETRACTION (2026-10-13): the earlier port of this file
-- ported Talk arms only under the disproved "no-timer-bridge" doctrine.
-- CreateLuaEvent is a real registered bridge
-- (engine/scripting/runtime.go:798); timer-driven cast arms port on the
-- maiden_of_virtue / moroes / curator / nightbane convention (see the
-- marrowgar retraction this run). Phase/machine gating is the real
-- blocker (the gothik precedent), not timers.
--
-- PORTED ARMS (C++-exact for all modeled arms):
-- boss JustEngagedWith (:281-342): Talk(SAY_AGGRO 7) (:338),
-- RemoveAurasDueToSpell(SHADOW_CHANNELING) (:339), then
-- DoCastSelf(SPELL_MANA_BARRIER 70842, true) TRIGGERED (:340). The
-- CheckRequiredBosses evade branch,
-- instance->SetBossState, scheduler.CancelGroup(GROUP_INTRO) legs have
-- no bridge (the DoZoneInCombat leg :293 is PORTED in the event-1
-- handler — live bridge engine/world/lua_creature_events.go,
-- 2026-10-23). Phase-INDEPENDENT scheduler arms ported:
-- SPELL_BERSERK 26662 self-cast 10min one-shot + Talk(SAY_BERSERK 15)
-- (:296-300); SPELL_DEATH_AND_DECAY 71001 — no GROUP arg, survives
-- phase transition (:301-306): random-target cast init 17s / re-arm
-- {22s,30s} unconditional (:305-306). Phase-ONE GROUP_ONE arm ported:
-- SPELL_SHADOW_BOLT 71254 (:318-326) — init 2s / re-arm {2450ms,3600ms}
-- unconditional, cancelled at the phase-2 transition
-- (CancelGroup(GROUP_ONE) :419). The other phase-one scheduler seeds
-- stay out: SummonWaveP1 (summon machine); Dominate Mind
-- (multi-target SelectTargetList + -SPELL_DOMINATE_MIND exclusion +
-- the GetDifficulty() != 10MAN_NORMAL gate, which IsHeroic() does not
-- express exactly); DoImproveCultist (cross-creature machine) — see
-- documented-only.
-- npc_cult_fanatic (37890, icecrown_citadel.h:175) — C++ schedules in
--   Reset() (:601-615); re-keyed to the add's own OnEnterCombat
--   (summoned mid-combat with immediate AttackStart, boss JustSummoned
--   :484-488; the scheduler only runs with a victim :663-668):
--   SPELL_VAMPIRIC_MIGHT 70674 self-cast 17s/25s (:603);
--   SPELL_SHADOW_CLEAVE 70670 victim-cast 12s/14s (:608);
--   SPELL_NECROTIC_STRIKE 70659 victim-cast 10s/17s (:613) — all
--   non-triggered. CORRECTION (2026-10-16): the IsCasting bridge EXISTS
--   but always returns false for creatures (no cast state,
--   engine/world/lua_objects.go) — the SetValidator always passes, so
--   timers firing unconditionally is C++-net-equal, not a missing leg.
--   SpellHit arm PORTED 2026-10-16 (event 14,
--   CREATURE_EVENT_ON_HIT_BY_SPELL — the earlier "no SpellHit bridge"
--   claim was stale; engine/world/spells.go:11709-11730, boss_ai.go:2052-2096):
--   case SPELL_DARK_TRANSFORMATION_T (70895, :622-625) —
--   DoCastSelf(SPELL_DARK_TRANSFORMATION 70900) non-triggered; the
--   InterruptNonMeleeSpells(true) leg has no bridge on the
--   event-creature method set (net no-op in Go: packet-only casts).
--   RE-KEY 2026-10-22: the remaining fanatic SpellHit legs are PORTED —
--   the :626-629 FULL_HEAL self-cast + UpdateEntry(NPC_DEFORMED_FANATIC)
--   (Creature::UpdateEntry twin bridged 2026-10-22); the :630-634
--   martyrdom arm (SetReactState/AttackStop/CastSpell — AttackStop
--   bridged 2026-10-22); the :640-660 2s/6s reanimate scheduler
--   (UpdateEntry(REANIMATED_FANATIC) + feign + clear-debuffs + full-heal
--   triggered + SetFlag(STUNNED|UNK_29|NOT_SELECTABLE) at 2s;
--   RemoveAuras + RemoveFlag + SetReactState(AGGRESSIVE) + shroud at 6s).
--   Only the instance-legged ladyDeathwhisper->AI()->Talk(SAY_ANIMATE_DEAD
--   13) stays out (no instance bridge).
-- npc_cult_adherent (37949, icecrown_citadel.h:178) — same Reset→engage
--   re-key (:693-710): SPELL_DEATHCHILL 5s/2500ms victim-cast —
--   entry-conditional DoCastVictim(SPELL_DEATHCHILL_BLAST 70906) when
--   GetEntry() == 38136 else DoCastVictim(SPELL_DEATHCHILL_BOLT 70594)
--   (:696-698); SPELL_SHROUD_OF_THE_OCCULT 70768 self-cast 15s/10s
--   (:703) — all non-triggered. RE-KEY 2026-10-22: the empowered entry
--   change is PORTED — the SpellHit DARK_EMPOWERMENT_T arm (:714-718)
--   UpdateEntry-swaps to ENTRY_EMPOWERED_ADHERENT (Creature::UpdateEntry
--   twin bridged 2026-10-22; registered for both entries since the twin
--   re-dispatches hooks to the live entry), so the Blast branch is live
--   once empowered. CORRECTION (2026-10-16): the
--   IsCasting bridge EXISTS but always returns false for creatures (no
--   cast state, engine/world/lua_objects.go) — the adherent
--   SetValidator always passes, so timers firing unconditionally is
--   C++-net-equal, not a missing leg.
--   SpellHit arms PORTED 2026-10-22 (event 14 dispatch bridged — the
--   earlier "no SpellHit bridge" claim was stale): :719-723
--   DARK_MARTYRDOM_T (SetReactState/AttackStop/CastSpell); the :724-752
--   2s/6s reanimate scheduler (UpdateEntry(REANIMATED_ADHERENT) + feign +
--   clear-debuffs + full-heal triggered + SetFlag at 2s; RemoveAuras +
--   RemoveFlag + SetReactState(AGGRESSIVE) + shroud at 6s). Only the
--   instance-legged boss Talk(SAY_ANIMATE_DEAD) stays out (no bridges).
--   SPELL_CURSE_OF_TORPOR 71237 (:706-710, ported): DoCast(target,
--   non-triggered) on SelectTarget(Random, 1) (offset 1 skips the
--   MaxThreat head, UnitAI.h:172-174 — tank-skip unbridged, the
--   netherspite precedent); init 15s / re-arm 18s unconditional.
-- npc_darnavan (38472/38485) — C++ Reset() EventMap (:838-844) re-keyed
--   to OnEnterCombat: SPELL_BLADESTORM 65947 self-cast 10s/{90s,100s}
--   (:912); SPELL_INTIMIDATING_SHOUT 65930 self-cast {20s,25s}/{90s,120s}
--   (:919); SPELL_MORTAL_STRIKE 65926 victim-cast {25s,30s}/{15s,30s}
--   (:923); SPELL_SUNDER_ARMOR 65936 victim-cast {5s,8s}/{3s,7s} (:930)
--   — all non-triggered (DoCast 1-arg = self per UnitAI.cpp:113). The
--   CHARGE conditional leg (:899-904) is PORTED via a 1s
--   charge-poll (darnavanCanCharge latch, 20s re-arm :928); the
--   SHATTERING_THROW leg (:893-898) stays out — its
--   IsImmunedToDamage(SPELL_SCHOOL_MASK_NORMAL) gate has no bridge.
--
-- DOCUMENTED-ONLY (registrations: 36855 / 37890 / 37949 / 38472 /
-- 38485):
-- boss_lady_deathwhisper DoAction(ACTION_START_INTRO) —
-- Talk(SAY_INTRO_1 0) rides a DoAction leg with no bridge (the
-- krick ACTION_OUTRO precedent); Talk(SAY_INTRO_2 1 .. SAY_INTRO_7
-- 6) ride the intro scheduler machine (intro machine — no bridge).
-- boss_lady_deathwhisper JustEngagedWith scheduler legs —
-- Talk(SAY_BERSERK 15) is PORTED (10-min berserk timer, above);
-- Talk(SAY_DOMINATE_MIND 10) rides a scheduler task with
-- multi-target SelectTargetList (randomPlayer is the in-tree bridge —
-- the earlier "no random-target bridge" claim is STALE, retracted) and
-- a GetDifficulty() != RAID_DIFFICULTY_10MAN_NORMAL gate (:307; stays
-- out: IsHeroic() is bridged but false for 25N, which the gate passes,
-- so no exact difficulty bridge; the list also carries the
-- -SPELL_DOMINATE_MIND aura exclusion); SummonWaveP1 /
-- Shadow Bolt scheduler tasks with
-- Talk(SAY_DARK_EMPOWERMENT 11 / SAY_DARK_TRANSFORMATION 12) ride
-- summon / random-target machines (no bridge); the DoImproveCultist
-- task is PORTED 2026-10-23 (onImproveCultist: GetCreaturesInGrid pool,
-- cross-creature CastSpell + triggered flag, Talk — the "no
-- friendly/cross-creature bridge" claim was stale).
-- CheckRequiredBosses / EnterEvadeMode / instance->DoCastSpellOnPlayers
-- / instance->SetBossState legs have no instance bridge (the
-- DoZoneInCombat :293 leg is PORTED in the event-1 handler — live
-- bridge engine/world/lua_creature_events.go, 2026-10-23).
-- boss_lady_deathwhisper DamageTaken — Talk(SAY_PHASE_2 8) +
-- Talk(EMOTE_PHASE_2 9) ride the mana-power pct gate of the phase
-- transition; the old "(no health-pct bridge; the garfrost
-- precedent)" note was wrong — HealthBelowPct appears nowhere in
-- the C++ arm (cpp:406-419), the gate is damage > GetPower(MANA),
-- so the health-pct precedent does not apply. The DamageTaken
-- phase-2 transition (mana-power gate, event-9 latch, per-GUID
-- PHASE_ONE latch, damage -= mana via event-9 return) + Frostbolt /
-- Frostbolt Volley GROUP_TWO seeds are PORTED 2026-10-14
-- (deathwhisperDamageTaken; Touch of Insignificance stays out —
-- victim-targeted AddAura has no bridge, the Lua AddAura is
-- self-only; Summon Spirits stays out — creature-cast path has no
-- SPELL_EFFECT_SUMMON delivery leg, the eregos precedent); the
-- mana-barrier aura legs have no bridge.
-- boss_lady_deathwhisper JustDied — the Full House achievement
-- legs, the DaranavanMoveEvent, faction/ReactState legs, and
-- KilledMonsterCredit(NPC_DARNAVAN_CREDIT) group loop have no
-- bridge; darnavan->AI()->Talk(SAY_DARNAVAN_RESCUED 1) is Talk on
-- another creature riding instance legs (no instance bridge).
-- Talk(SAY_ANIMATE_DEAD 13) is called by
-- ladyDeathwhisper->AI()->Talk from inside the npc_cult_fanatic /
-- npc_cult_adherent summon-death scheduler legs (still out — the
-- instance-legged boss Talk has no bridge; the UpdateEntry/SetFlag legs
-- are PORTED 2026-10-22).
-- npc_vengeful_shade (38222) — zero own Talk lines; the
-- VengefulBlast passive / KillSelf / SpellHitTarget legs have no
-- bridge — NOT registered (the bronjahm npc_corrupted_soul_fragment
-- precedent). npc_cult_fanatic / npc_cult_adherent ARE registered
-- (timer arms ported above); the fanatic/adherent SpellHit transformation
-- and reanimate arms (:622-660 / :714-752) are PORTED 2026-10-22 — event 14
-- CREATURE_EVENT_ON_HIT_BY_SPELL IS bridged
-- (engine/world/spells.go:11709-11730, boss_ai.go:2052-2096) and the
-- Creature::UpdateEntry twin is bridged (lua_creature_events.go), so the
-- earlier "no SpellHit/UpdateEntry bridge" claims were stale. The only
-- SpellHit stay-out is the instance-legged
-- ladyDeathwhisper->AI()->Talk(SAY_ANIMATE_DEAD) (no instance bridge),
-- as is the darnavan SHATTERING_THROW conditional leg
-- (:893-898, immunity gate — no bridge); darnavan CHARGE leg PORTED
-- (charge-poll). CORRECTION (2026-10-16): the IsCasting bridge EXISTS
-- but always returns false for creatures (no cast state,
-- engine/world/lua_objects.go) — the validators always pass, so the
-- boss scheduler and adherent SetValidator "no bridge" claims above
-- are restated as net-no-ops, not missing legs. Boss scheduler SetValidator (:216-219:
-- !UNIT_STATE_CASTING unless intro) and the adherent validator
-- (:691-692) have no bridge — timers fire unconditionally (maiden
-- precedent).
-- spell_deathwhisper_mana_barrier /
-- spell_deathwhisper_dominated_mind /
-- spell_deathwhisper_vampiric_might join the no-AuraScript-bridge
-- queue (the boss_moragg optic-link precedent);
-- spell_deathwhisper_summon_spirits joins the
-- no-SpellScript-bridge queue.
-- at_lady_deathwhisper_entrance joins the unbridged-area-trigger
-- queue (the pit_of_saron cavern-triggers precedent).
-- All 19 Talk() calls in the file accounted for (19 = ported (9): lady
-- aggro 7 + berserk 15 + phase-2 8 + emote 9 + kill 14 + death 16 +
-- dark empowerment 11 + dark transformation 12 +
-- darnavan aggro 0; documented-only (10): intro 0-6 (7) + dominate 10
-- + animate-dead 13 (2) + darnavan rescued 1 (1)).

local ENTRY_LADY_DEATHWHISPER = 36855
local ENTRY_CULT_FANATIC = 37890 -- icecrown_citadel.h:175
local ENTRY_CULT_ADHERENT = 37949 -- icecrown_citadel.h:178
local ENTRY_EMPOWERED_ADHERENT = 38136 -- icecrown_citadel.h:179
local ENTRY_DEFORMED_FANATIC = 38135 -- icecrown_citadel.h:176
local ENTRY_REANIMATED_FANATIC = 38009 -- icecrown_citadel.h:177
local ENTRY_REANIMATED_ADHERENT = 38010 -- icecrown_citadel.h:180
local ENTRY_DARNAVAN_10 = 38472
local ENTRY_DARNAVAN_25 = 38485

local SAY_AGGRO = 7
local SAY_KILL = 14
local SAY_DEATH = 16
local SAY_BERSERK = 15
local SAY_PHASE_2 = 8 -- boss_lady_deathwhisper.cpp:43
local EMOTE_PHASE_2 = 9 -- :44
local SAY_DARNAVAN_AGGRO = 0
local SAY_DARK_EMPOWERMENT = 11 -- boss_lady_deathwhisper.cpp:46
local SAY_DARK_TRANSFORMATION = 12 -- boss_lady_deathwhisper.cpp:47

local SPELL_BERSERK = 26662 -- icecrown_citadel.h:33
local SPELL_MANA_BARRIER = 70842
local SPELL_SHADOW_CHANNELING = 43897 -- :226 Reset self-cast
local POWER_MANA = 0 -- Powers enum (Unit.h): MANA 0
local SPELL_FROSTBOLT = 71420 -- :66
local SPELL_FROSTBOLT_VOLLEY = 72905 -- :67
local SPELL_DARK_TRANSFORMATION_T = 70895 -- :71 (boss trigger)
local SPELL_DARK_TRANSFORMATION = 70900 -- :86
local SPELL_FULL_HEAL = 17683 -- :82 (fanatic self-cast :627)
local SPELL_DARK_MARTYRDOM_T = 70897 -- :73 (fanatic SpellHit :630)
local SPELL_DARK_MARTYRDOM_FANATIC = 71236 -- :92 (fanatic self-cast :634)
local SPELL_DARK_EMPOWERMENT_T = 70896 -- :72 (adherent SpellHit :716)
local SPELL_DARK_MARTYRDOM_ADHERENT = 70903 -- :105 (adherent self-cast :723)
local SPELL_DARK_MARTYRDOM_ADHERENT_25N = 72498 -- :106
local SPELL_DARK_MARTYRDOM_ADHERENT_10H = 72499 -- :107
local SPELL_DARK_MARTYRDOM_ADHERENT_25H = 72500 -- :108
local SPELL_PERMANENT_FEIGN_DEATH = 70628 -- :83 (reanimate 2s :644)
local SPELL_CLEAR_ALL_DEBUFFS = 34098 -- :81 (reanimate 2s :645)
local UNIT_FIELD_FLAGS = 59 -- Unit.h UNIT_FIELD_FLAGS
local UNIT_FLAG_STUNNED = 0x40000 -- UnitDefines.h:142
local UNIT_FLAG_NOT_SELECTABLE = 0x2000000 -- UnitDefines.h:149
local UNIT_FLAG_UNK_29 = 0x20000000 -- UnitDefines.h:153 (feign death)
local REACT_AGGRESSIVE = 1 -- UnitDefines.h ReactStates
local REACT_PASSIVE = 0
local SPELL_VAMPIRIC_MIGHT = 70674
local SPELL_SHADOW_CLEAVE = 70670
local SPELL_NECROTIC_STRIKE = 70659
local SPELL_DEATHCHILL_BOLT = 70594
local SPELL_DEATHCHILL_BLAST = 70906
local SPELL_SHROUD_OF_THE_OCCULT = 70768
local SPELL_DEATH_AND_DECAY = 71001 -- :63
local SPELL_SHADOW_BOLT = 71254 -- :62
local SPELL_CURSE_OF_TORPOR = 71237 -- :102 (adherent)
local SPELL_BLADESTORM = 65947
local SPELL_INTIMIDATING_SHOUT = 65930
local SPELL_MORTAL_STRIKE = 65926
local SPELL_SUNDER_ARMOR = 65936
local SPELL_CHARGE = 65927 -- :901 DoCastVictim, non-triggered

local timers = {}

-- C++ _cultistQueue removal latch (DoImproveCultist :564-565): GUIDs
-- already improved are never re-picked; per-boss-GUID.
local improvedCultists = {}

-- C++ _phase (PHASE_ONE=2 / PHASE_TWO=3, :141-142); Lua-side latch for
-- the event-9 phase-2 transition (loken event-9 precedent).
local deathwhisperPhase = {}

-- C++ npc_darnavan::_canCharge (Initialize :831, re-armed by
-- EVENT_DARNAVAN_CHARGE 20s :928); per-GUID latch.
local darnavanCanCharge = {}

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

-- lavanthor / the_beast precedent: map-wide same-instance alive-player
-- pool standing in for SelectTarget(Random) threat-list selection.
-- FIDELITY CAVEAT: C++ SelectTarget defaults playerOnly=FALSE
-- (UnitAI.h:170), so the pool can include pets/totems — Lua's
-- player-only enumeration is the documented approximation (the
-- tuten_kash precedent); the unit-enumeration bridge IS GetAITargets
-- (lua_creature_events.go:345).
local function randomPlayer(creature)
    local mapId = creature:GetMapId()
    local instanceId = creature:GetInstanceId()
    local candidates = {}
    for _, plr in ipairs(GetPlayersInWorld()) do
        if plr:GetMapId() == mapId and plr:GetInstanceId() == instanceId
                and not plr:IsDead() then
            candidates[#candidates + 1] = plr
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ :296-300 phase-independent scheduler: DoCastSelf(SPELL_BERSERK)
-- non-triggered + Talk(SAY_BERSERK 15) — 10min one-shot.
local function onBerserk(creature, guid)
    creature:CastSpell(creature, SPELL_BERSERK)
    creature:Talk(SAY_BERSERK)
end

-- C++ :301-306 phase-INDEPENDENT (no GROUP arg — survives the
-- CancelGroup(GROUP_ONE) at phase transition): DoCast(target,
-- SPELL_DEATH_AND_DECAY 71001) non-triggered on SelectTarget(Random);
-- init 17s, re-arm {22s,30s} unconditional (outside the target if).
local function onDeathAndDecay(creature, guid)
    local target = randomPlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_DEATH_AND_DECAY)
    end
    schedule(guid, "deathanddecay", {22000, 30000}, function()
        onDeathAndDecay(creature, guid)
    end)
end

-- C++ :331-334 phase-ONE only (GROUP_ONE, cancelled at the phase
-- transition :419): DoImproveCultist (:559-577); init 15s, unconditional
-- 25s re-arm. Bridges: creature:GetCreaturesInGrid
-- (lua_creature_events.go:2164) for the pool, creature:CastSpell with a
-- target + triggered flag (:226), creature:Talk. The C++ _cultistQueue is
-- summon-membership-exact; the Lua twin pools alive non-improved cult
-- adds in 80 yd (all 7 fixed summon spots are within ~75 yd of the
-- boss), the closest live-bridge stand-in.
local function onImproveCultist(creature, guid)
    local improved = improvedCultists[guid] or {}
    local pool = {}
    for _, entry in ipairs({ ENTRY_CULT_FANATIC, ENTRY_CULT_ADHERENT }) do
        for _, add in ipairs(creature:GetCreaturesInGrid(entry, 80.0)) do
            if not add:IsDead() and not improved[add:GetGUID()] then
                pool[#pool + 1] = add
            end
        end
    end
    if #pool > 0 then
        -- C++ :564-565 uniform pick + removal from the queue; Lua latch.
        local cultist = pool[math.random(#pool)]
        improved[cultist:GetGUID()] = true
        improvedCultists[guid] = improved
        -- C++ :571-575: RAND(0,1) martyrdom vs triggered
        -- transformation (fanatic) / empowerment (adherent) + Talk.
        if math.random(2) == 1 then
            creature:CastSpell(cultist, SPELL_DARK_MARTYRDOM_T)
        else
            local fanatic = cultist:GetEntry() == ENTRY_CULT_FANATIC
            creature:CastSpell(cultist, fanatic and SPELL_DARK_TRANSFORMATION_T or SPELL_DARK_EMPOWERMENT_T, true)
            creature:Talk(fanatic and SAY_DARK_TRANSFORMATION or SAY_DARK_EMPOWERMENT)
        end
    end
    schedule(guid, "improve", 25000, function()
        onImproveCultist(creature, guid)
    end)
end

-- C++ :318-326 phase-ONE only (GROUP_ONE, cancelled at the phase
-- transition :419): DoCast(target, SPELL_SHADOW_BOLT 71254)
-- non-triggered on SelectTarget(Random); init 2s, re-arm
-- {2450ms,3600ms} unconditional.
local function onShadowBolt(creature, guid)
    local target = randomPlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_SHADOW_BOLT)
    end
    schedule(guid, "shadowbolt", {2450, 3600}, function()
        onShadowBolt(creature, guid)
    end)
end

-- C++ DamageTaken phase-2 transition (:407-417): gate
-- _phase == PHASE_ONE && damage > me->GetPower(POWER_MANA); then
-- Talk(SAY_PHASE_2 8) + Talk(EMOTE_PHASE_2 9), damage -= mana,
-- SetPower(MANA, 0), RemoveAurasDueToSpell(MANA_BARRIER),
-- CancelGroup(GROUP_ONE), seed GROUP_TWO. Ported leg: DoStartMovement
-- (:414) issues MoveChase directly (ScriptedCreature.cpp:159-163, no flag
-- leg); the Lua twin flips the per-tick pursuit gate back on at the
-- transition (it was flipped off at engage for the DoStartNoMovement leg)
-- — chase resumes in phase 2, net-identical to C++ (razuvious 7s
-- EVENT_ATTACK precedent; the 10:19 "mid-combat true-leg diverges"
-- doctrine misread the flag no-op as the whole arm).
-- SetPower(MANA, 0) rides the creature:SetPower
-- bridge (lua_creature_events.go), C++ order (drain, zero, strip aura).
-- FIDELITY CAVEAT: the mana_barrier AuraScript is unbridged, so in Go
-- mana never drains and damage > mana fires only on a single hit
-- exceeding her full mana pool — the transition condition is C++-exact,
-- but it will rarely fire live until the engine models the barrier.
local function frostboltTick(creature, guid)
    -- C++ :423-427 GROUP_TWO: DoCastVictim(SPELL_FROSTBOLT), 12s init,
    -- Repeat() (12s loop).
    creature:CastSpell(nil, SPELL_FROSTBOLT)
    schedule(guid, "frostbolt", 12000, function()
        frostboltTick(creature, guid)
    end)
end

local function frostboltVolleyTick(creature, guid)
    -- C++ :428-432 GROUP_TWO: DoCastAOE(SPELL_FROSTBOLT_VOLLEY),
    -- self-cast (boss_ai.go:4783 convention), 20s init, Repeat()
    -- (20s loop).
    creature:CastSpell(creature, SPELL_FROSTBOLT_VOLLEY)
    schedule(guid, "frostboltvolley", 20000, function()
        frostboltVolleyTick(creature, guid)
    end)
end

local function deathwhisperDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    if deathwhisperPhase[guid] == 3 then
        return
    end
    local mana = creature:GetPower()
    if damage > mana then
        deathwhisperPhase[guid] = 3
        creature:Talk(SAY_PHASE_2)
        creature:Talk(EMOTE_PHASE_2)
        -- C++ :414 DoStartMovement(me->GetVictim()) — direct MoveChase
        -- (ScriptedCreature.cpp:159-163); the Lua twin re-enables the
        -- per-tick pursuit gate (C++-exact position: before
        -- ResetThreatList).
        creature:SetCombatMovement(true)
        -- C++ :416-417: damage -= me->GetPower(POWER_MANA);
        -- me->SetPower(POWER_MANA, 0) — drain then zero, before the aura
        -- strip.
        creature:SetPower(POWER_MANA, 0)
        creature:RemoveAura(SPELL_MANA_BARRIER)
        -- C++ DamageTaken phase-2: ResetThreatList() (bridged
        -- lua_objects.go:389; the old "no threat bridge" header claim was
        -- stale).
        creature:ClearThreatList()
        -- C++ CancelGroup(GROUP_ONE) (:419): the shadowbolt arm dies at
        -- the phase transition; deathanddecay is phase-independent and
        -- rides the normal cancelTimers on 2/4/23.
        local per = timers[guid]
        if per and per["shadowbolt"] then
            RemoveEventById(per["shadowbolt"])
            per["shadowbolt"] = nil
        end
        -- C++ CancelGroup(GROUP_ONE) also kills the DoImproveCultist arm.
        if per and per["improve"] then
            RemoveEventById(per["improve"])
            per["improve"] = nil
        end
        schedule(guid, "frostbolt", 12000, function()
            frostboltTick(creature, guid)
        end)
        schedule(guid, "frostboltvolley", 20000, function()
            frostboltVolleyTick(creature, guid)
        end)
        -- C++ DamageTaken :449-450: on heroic mode the phase-2
        -- transition makes her immune to taunt effects
        -- (IMMUNITY_STATE/SPELL_AURA_MOD_TAUNT=11,
        -- IMMUNITY_EFFECT/SPELL_EFFECT_ATTACK_ME=114; IsHeroic() gate,
        -- bridged lua_creature_events.go:203). The SummonWaveP2
        -- scheduler leg stays out (summon machine).
        if creature:IsHeroic() then
            creature:ApplySpellImmune(0, 1, 11, true)
            creature:ApplySpellImmune(0, 0, 114, true)
        end
        local newDamage = damage - mana
        if newDamage ~= damage then
            return true, newDamage
        end
    end
end

-- C++ boss_lady_deathwhisper::JustEngagedWith (:281-342): the
-- CheckRequiredBosses evade branch has no bridge; the phase-one
-- scheduler seeds that stay out are SummonWaveP1 (summon machine),
-- Dominate Mind (multi-target SelectTargetList + the GetDifficulty()
-- != 10MAN_NORMAL gate) and DoImproveCultist (cross-creature machine).
-- Shadow Bolt (GROUP_ONE) and Death and Decay (phase-INDEPENDENT) ARE
-- ported below — the C++ SelectTarget(Random) playerOnly=FALSE default
-- is documented on randomPlayer (the tuten_kash pets/totems caveat).
-- Talk(SAY_AGGRO)
-- (:338) then DoCastSelf(SPELL_MANA_BARRIER, true) TRIGGERED (:340).
local function deathwhisperJustEngagedWith(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    improvedCultists[guid] = {}
    deathwhisperPhase[guid] = 2 -- PHASE_ONE (:290)
    creature:SetCombatPulseDelay(5) -- boss_lady_deathwhisper.cpp:291 (JustEngagedWith arm 2; the setActive leg still has no bridge)
    creature:DoZoneInCombat() -- boss_lady_deathwhisper.cpp:293 (JustEngagedWith arm 3; bridge live engine/world/lua_creature_events.go, 2026-10-23)
    creature:Talk(SAY_AGGRO)
    -- C++ :338 DoStartNoMovement(who) — engage-stationary; phase-1 exact.
    -- The phase-2 DoStartMovement leg is ported in the DamageTaken
    -- transition arm above (gate flip back on, C++ :414 position).
    creature:SetCombatMovement(false)
    -- C++ :339 — the channeling aura is stripped on engage (re-cast on
    -- Reset, :226; the RemoveAura bridge).
    creature:RemoveAura(SPELL_SHADOW_CHANNELING)
    creature:CastSpell(creature, SPELL_MANA_BARRIER, true)
    schedule(guid, "berserk", 600000, function() onBerserk(creature, guid) end)
    schedule(guid, "deathanddecay", 17000, function() onDeathAndDecay(creature, guid) end)
    schedule(guid, "shadowbolt", 2000, function() onShadowBolt(creature, guid) end)
    schedule(guid, "improve", 15000, function() onImproveCultist(creature, guid) end)
end

local function deathwhisperLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    improvedCultists[guid] = nil
    deathwhisperPhase[guid] = nil
end

local function deathwhisperReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    improvedCultists[guid] = nil
    deathwhisperPhase[guid] = nil
    -- C++ Reset() (:226): unconditional non-triggered self-cast of the
    -- phase-1 channeling aura (removed on engage via
    -- RemoveAurasDueToSpell, :340). Reset-only; alive-gated.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_SHADOW_CHANNELING)
    end
    -- C++ :227: me->SetPower(POWER_MANA, me->GetMaxPower(POWER_MANA)) —
    -- unconditional, after the channeling cast; the mana pool refills on
    -- reset via the creature:SetPower bridge.
    creature:SetPower(POWER_MANA, creature:GetMaxPower())
    -- C++ :228-229: remove the heroic phase-2 taunt immunities on reset
    -- (IMMUNITY_STATE/SPELL_AURA_MOD_TAUNT=11,
    -- IMMUNITY_EFFECT/SPELL_EFFECT_ATTACK_ME=114, both apply=false).
    creature:ApplySpellImmune(0, 1, 11, false)
    creature:ApplySpellImmune(0, 0, 114, false)
end

-- C++ boss_lady_deathwhisper::KilledUnit:
-- if (victim->GetTypeId() == TYPEID_PLAYER) Talk(SAY_KILL) — the
-- razuvious player-gated variant precedent.
local function deathwhisperKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

-- C++ boss_lady_deathwhisper::JustDied: Talk(SAY_DEATH) — the sjonnir
-- JustDied-Talk precedent. cancelTimers stops the 10-min berserk +
-- deathanddecay + (phase-1) shadowbolt arms from firing on the corpse.
local function deathwhisperJustDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    improvedCultists[creature:GetGUID()] = nil
    creature:Talk(SAY_DEATH)
end

-- C++ npc_cult_fanatic Reset() scheduler (:601-615), re-keyed to the
-- add's own OnEnterCombat (summoned mid-combat with immediate AttackStart,
-- boss JustSummoned :484-488; the scheduler only runs with a victim
-- :663-668). All three arms non-triggered; the SetValidator
-- !UNIT_STATE_CASTING gate has no bridge (maiden precedent).
local function onVampiricMight(creature, guid)
    creature:CastSpell(creature, SPELL_VAMPIRIC_MIGHT)
    schedule(guid, "vampiric", 25000, function() onVampiricMight(creature, guid) end)
end

local function onShadowCleave(creature, guid)
    creature:CastSpell(nil, SPELL_SHADOW_CLEAVE)
    schedule(guid, "cleave", 14000, function() onShadowCleave(creature, guid) end)
end

local function onNecroticStrike(creature, guid)
    creature:CastSpell(nil, SPELL_NECROTIC_STRIKE)
    schedule(guid, "strike", 17000, function() onNecroticStrike(creature, guid) end)
end

local function fanaticEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "vampiric", 17000, function() onVampiricMight(creature, guid) end)
    schedule(guid, "cleave", 12000, function() onShadowCleave(creature, guid) end)
    schedule(guid, "strike", 10000, function() onNecroticStrike(creature, guid) end)
end

local function fanaticLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function fanaticReset(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ npc_cult_fanatic::SpellHit (:618-646) — bridged as event 14
-- CREATURE_EVENT_ON_HIT_BY_SPELL (engine/world/spells.go,
-- boss_ai.go), args (event, creature, caster, spellid). The engine fires
-- ev14 once per hit target — the true C++ DoDamageAndTriggers twin
-- (Spell.cpp:2629 — NOT per effect). Both legs below are fire-safe: the
-- FULL_HEAL re-cast is net-idempotent and the DARK_TRANSFORMATION re-cast
-- is visually harmless (C++ gates the repeat behind the UpdateEntry
-- one-way transition, unmodeled here) — no first-fire gate needed,
-- unlike the moorabi Talk arm.
-- PORTED arm — case SPELL_DARK_TRANSFORMATION_T (70895, :622-625):
-- DoCastSelf(SPELL_DARK_TRANSFORMATION 70900) non-triggered.
-- InterruptNonMeleeSpells(true) has no bridge on the event-creature
-- method set (StopSpellCast lives only on the session-scoped gossip
-- creature; Go creature casts are packet-only with no cast state, so
-- the interrupt is a net no-op anyway — engine/world/lua_objects.go).
-- BLOCKED arms in the same switch: the :626-629 UpdateEntry
-- (NPC_DEFORMED_FANATIC) leg stays out (no UpdateEntry bridge) while
-- the FULL_HEAL self-cast is PORTED (above); :631-635
-- DARK_MARTYRDOM rides SetReactState/AttackStop — AttackStop bridged
-- 2026-10-22 (PORTED below); the
-- :640-660 2s/6s reanimate scheduler legs ride UpdateEntry +
-- SetFlag(UNIT_FIELD_FLAGS) + the instance-legged
-- ladyDeathwhisper->AI()->Talk(SAY_ANIMATE_DEAD) (no instance bridge).
local function fanaticSpellHit(event, creature, caster, spellId)
    if spellId == SPELL_DARK_TRANSFORMATION_T then
        creature:CastSpell(creature, SPELL_DARK_TRANSFORMATION)
    elseif spellId == SPELL_DARK_TRANSFORMATION then
        -- C++ :626-629 — DoCastSelf(SPELL_FULL_HEAL 17683) non-triggered
        -- fires on the self-cast hit (event 14 delivers self-cast hits,
        -- engine/world/boss_ai.go:2052-2096 — evaded targets and bounced
        -- reflects never fire; a landed reflect fires on the caster),
        -- then me->UpdateEntry(NPC_DEFORMED_FANATIC) (Creature::UpdateEntry
        -- twin, bridged 2026-10-22).
        creature:CastSpell(creature, SPELL_FULL_HEAL)
        creature:UpdateEntry(ENTRY_DEFORMED_FANATIC)
    elseif spellId == SPELL_DARK_MARTYRDOM_T then
        -- C++ :630-634 — me->SetReactState(REACT_PASSIVE) ->
        -- InterruptNonMeleeSpells(true) (no bridge; Go creature casts are
        -- packet-only, the interrupt is a net no-op) -> me->AttackStop()
        -- -> DoCastSelf(SPELL_DARK_MARTYRDOM_FANATIC) non-triggered.
        creature:SetReactState(REACT_PASSIVE)
        creature:AttackStop()
        creature:CastSpell(creature, SPELL_DARK_MARTYRDOM_FANATIC)
    elseif spellId == SPELL_DARK_MARTYRDOM_FANATIC then
        -- C++ :640-660 — the 2s/6s reanimate scheduler fires on the
        -- DARK_MARTYRDOM_FANATIC self-cast hit: 2s -> UpdateEntry(
        -- NPC_REANIMATED_FANATIC) + DoCastSelf(PERMANENT_FEIGN_DEATH) +
        -- DoCastSelf(CLEAR_ALL_DEBUFFS) + DoCastSelf(FULL_HEAL, triggered)
        -- + SetFlag(STUNNED|UNK_29|NOT_SELECTABLE); 6s ->
        -- RemoveAurasDueToSpell(PERMANENT_FEIGN_DEATH) + RemoveFlag +
        -- SetReactState(AGGRESSIVE) + DoCastSelf(SHROUD_OF_THE_OCCULT).
        -- The ladyDeathwhisper->AI()->Talk(SAY_ANIMATE_DEAD 13) leg has no
        -- instance bridge (documented).
        local guid = creature:GetGUID()
        schedule(guid, "reanimate2", 2000, function()
            creature:UpdateEntry(ENTRY_REANIMATED_FANATIC)
            creature:CastSpell(creature, SPELL_PERMANENT_FEIGN_DEATH)
            creature:CastSpell(creature, SPELL_CLEAR_ALL_DEBUFFS)
            creature:CastSpell(creature, SPELL_FULL_HEAL, true)
            creature:SetFlag(UNIT_FIELD_FLAGS,
                UNIT_FLAG_STUNNED + UNIT_FLAG_NOT_SELECTABLE + UNIT_FLAG_UNK_29)
        end)
        schedule(guid, "reanimate6", 6000, function()
            creature:RemoveAura(SPELL_PERMANENT_FEIGN_DEATH)
            creature:RemoveFlag(UNIT_FIELD_FLAGS,
                UNIT_FLAG_STUNNED + UNIT_FLAG_NOT_SELECTABLE + UNIT_FLAG_UNK_29)
            creature:SetReactState(REACT_AGGRESSIVE)
            creature:CastSpell(creature, SPELL_SHROUD_OF_THE_OCCULT)
        end)
    end
end

-- C++ SelectTarget(Random, 1) (UnitAI.h:170-174): random over the threat
-- list — playerOnly=FALSE so pets/totems are eligible — with offset 1
-- skipping the first MaxThreat-ordered entry (the tank). Bridged via
-- creature:GetAITargets() (threat-ordered; lua_creature_events.go:345),
-- the netherspite/auriaya tank-skip idiom on the bridged threat list.
local function randomNonHeadThreatTarget(creature)
    local targets = creature:GetAITargets()
    if #targets <= 1 then
        return nil
    end
    return targets[math.random(2, #targets)]
end

-- C++ npc_cult_adherent Reset() scheduler (:693-710), same re-key.
local function onDeathchill(creature, guid)
    -- C++ :696-698 — entry-conditional: the empowered adherent casts the
    -- Blast variant, otherwise the Bolt. The entry change rides the
    -- SpellHit DARK_EMPOWERMENT_T arm (UpdateEntry twin, bridged
    -- 2026-10-22), so the Blast branch is live once empowered.
    if creature:GetEntry() == ENTRY_EMPOWERED_ADHERENT then
        creature:CastSpell(nil, SPELL_DEATHCHILL_BLAST)
    else
        creature:CastSpell(nil, SPELL_DEATHCHILL_BOLT)
    end
    schedule(guid, "deathchill", 2500, function() onDeathchill(creature, guid) end)
end

local function onShroudOfTheOccult(creature, guid)
    creature:CastSpell(creature, SPELL_SHROUD_OF_THE_OCCULT)
    schedule(guid, "shroud", 10000, function() onShroudOfTheOccult(creature, guid) end)
end

-- C++ :706-710: DoCast(target, SPELL_CURSE_OF_TORPOR) non-triggered on
-- SelectTarget(Random, 1) — offset-1 skips the MaxThreat head
-- (UnitAI.h:173-174), playerOnly=FALSE (UnitAI.h:170) so pets/totems
-- are eligible; lowered over the threat-ordered GetAITargets list
-- (lua_creature_events.go:345 — the netherspite/auriaya tank-skip
-- idiom on the bridged threat list); init 15s, re-arm 18s
-- unconditional.
local function onCurseOfTorpor(creature, guid)
    local target = randomNonHeadThreatTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_CURSE_OF_TORPOR)
    end
    schedule(guid, "torpor", 18000, function() onCurseOfTorpor(creature, guid) end)
end

local function adherentEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "deathchill", 5000, function() onDeathchill(creature, guid) end)
    schedule(guid, "shroud", 15000, function() onShroudOfTheOccult(creature, guid) end)
    schedule(guid, "torpor", 15000, function() onCurseOfTorpor(creature, guid) end)
end

local function adherentLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function adherentReset(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ npc_darnavan::JustEngagedWith: Talk(SAY_DARNAVAN_AGGRO) — the
-- auriaya engage-port precedent. The Reset() EventMap (:838-844) is
-- re-keyed to OnEnterCombat: all four arms non-triggered (DoCast 1-arg
-- = self per UnitAI.cpp:113).
-- C++ npc_darnavan::UpdateAI :899-904 — CHARGE conditional leg ported:
-- when _canCharge and the victim is outside melee range,
-- DoCastVictim(SPELL_CHARGE 65927) fires and the flag re-arms 20s
-- later (EVENT_DARNAVAN_CHARGE :928). IsWithinMeleeRange is modeled
-- via creature:GetDistance (the skeram precedent). The _canShatter /
-- SHATTERING_THROW leg (:893-898) stays out — the
-- IsImmunedToDamage(SPELL_SCHOOL_MASK_NORMAL) gate has no bridge.
local function onDarnavanChargePoll(creature, guid)
    if darnavanCanCharge[guid] then
        local victim = creature:GetVictim()
        if victim and creature:GetDistance(victim) > 5.0 then
            creature:CastSpell(nil, SPELL_CHARGE)
            darnavanCanCharge[guid] = false
            schedule(guid, "chargerearm", 20000, function()
                darnavanCanCharge[guid] = true
            end)
        end
    end
    schedule(guid, "chargepoll", 1000, function() onDarnavanChargePoll(creature, guid) end)
end

local function onDarnavanBladestorm(creature, guid)
    creature:CastSpell(creature, SPELL_BLADESTORM)
    schedule(guid, "bladestorm", {90000, 100000}, function() onDarnavanBladestorm(creature, guid) end)
end

local function onDarnavanShout(creature, guid)
    creature:CastSpell(creature, SPELL_INTIMIDATING_SHOUT)
    schedule(guid, "shout", {90000, 120000}, function() onDarnavanShout(creature, guid) end)
end

local function onDarnavanMortalStrike(creature, guid)
    creature:CastSpell(nil, SPELL_MORTAL_STRIKE)
    schedule(guid, "mortal", {15000, 30000}, function() onDarnavanMortalStrike(creature, guid) end)
end

local function onDarnavanSunderArmor(creature, guid)
    creature:CastSpell(nil, SPELL_SUNDER_ARMOR)
    schedule(guid, "sunder", {3000, 7000}, function() onDarnavanSunderArmor(creature, guid) end)
end

local function darnavanJustEngagedWith(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_DARNAVAN_AGGRO)
    schedule(guid, "bladestorm", 10000, function() onDarnavanBladestorm(creature, guid) end)
    schedule(guid, "shout", {20000, 25000}, function() onDarnavanShout(creature, guid) end)
    schedule(guid, "mortal", {25000, 30000}, function() onDarnavanMortalStrike(creature, guid) end)
    schedule(guid, "sunder", {5000, 8000}, function() onDarnavanSunderArmor(creature, guid) end)
    darnavanCanCharge[guid] = true -- C++ Initialize :831
    schedule(guid, "chargepoll", 1000, function() onDarnavanChargePoll(creature, guid) end)
end

local function darnavanLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    darnavanCanCharge[guid] = nil
end

local function darnavanReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    darnavanCanCharge[guid] = nil
end

RegisterCreatureEvent(ENTRY_LADY_DEATHWHISPER, 1, deathwhisperJustEngagedWith)
RegisterCreatureEvent(ENTRY_LADY_DEATHWHISPER, 2, deathwhisperLeaveCombat)
RegisterCreatureEvent(ENTRY_LADY_DEATHWHISPER, 3, deathwhisperKilledUnit)
RegisterCreatureEvent(ENTRY_LADY_DEATHWHISPER, 4, deathwhisperJustDied)
RegisterCreatureEvent(ENTRY_LADY_DEATHWHISPER, 9, deathwhisperDamageTaken)
RegisterCreatureEvent(ENTRY_LADY_DEATHWHISPER, 23, deathwhisperReset)
RegisterCreatureEvent(ENTRY_CULT_FANATIC, 1, fanaticEnterCombat)
RegisterCreatureEvent(ENTRY_CULT_FANATIC, 2, fanaticLeaveCombat)
RegisterCreatureEvent(ENTRY_CULT_FANATIC, 14, fanaticSpellHit)
RegisterCreatureEvent(ENTRY_CULT_FANATIC, 23, fanaticReset)
RegisterCreatureEvent(ENTRY_CULT_ADHERENT, 1, adherentEnterCombat)
RegisterCreatureEvent(ENTRY_CULT_ADHERENT, 2, adherentLeaveCombat)
RegisterCreatureEvent(ENTRY_CULT_ADHERENT, 23, adherentReset)

-- C++ npc_cult_adherent::SpellHit (:714-752) — bridged as event 14
-- (the fanatic SpellHit precedent above). Registered for both the base
-- and empowered entries: the DARK_EMPOWERMENT_T arm UpdateEntry-swaps
-- the creature to ENTRY_EMPOWERED_ADHERENT, and the UpdateEntry twin
-- re-dispatches hooks to the live entry (mirroring the C++ AI object
-- surviving the transform); the onDeathchill entry-conditional Blast
-- branch goes live with it.
local function adherentSpellHit(event, creature, caster, spellId)
    if spellId == SPELL_DARK_EMPOWERMENT_T then
        -- C++ :716-718 — me->UpdateEntry(NPC_EMPOWERED_ADHERENT)
        -- (Creature::UpdateEntry twin, bridged 2026-10-22).
        creature:UpdateEntry(ENTRY_EMPOWERED_ADHERENT)
    elseif spellId == SPELL_DARK_MARTYRDOM_T then
        -- C++ :719-723 — me->SetReactState(REACT_PASSIVE) ->
        -- InterruptNonMeleeSpells(true) (no bridge; Go creature casts are
        -- packet-only, the interrupt is a net no-op) -> me->AttackStop()
        -- -> DoCastSelf(SPELL_DARK_MARTYRDOM_ADHERENT) non-triggered.
        creature:SetReactState(REACT_PASSIVE)
        creature:AttackStop()
        creature:CastSpell(creature, SPELL_DARK_MARTYRDOM_ADHERENT)
    elseif spellId == SPELL_DARK_MARTYRDOM_ADHERENT
        or spellId == SPELL_DARK_MARTYRDOM_ADHERENT_25N
        or spellId == SPELL_DARK_MARTYRDOM_ADHERENT_10H
        or spellId == SPELL_DARK_MARTYRDOM_ADHERENT_25H then
        -- C++ :724-752 — the 2s/6s reanimate scheduler (same shape as the
        -- fanatic's :640-660): 2s -> UpdateEntry(NPC_REANIMATED_ADHERENT)
        -- + feign + clear debuffs + full heal (triggered) + SetFlag(
        -- STUNNED|UNK_29|NOT_SELECTABLE); 6s -> RemoveAurasDueToSpell(
        -- PERMANENT_FEIGN_DEATH) + RemoveFlag + SetReactState(AGGRESSIVE)
        -- + DoCastSelf(SHROUD_OF_THE_OCCULT). The boss Talk(SAY_ANIMATE_DEAD)
        -- leg has no instance bridge (documented).
        local guid = creature:GetGUID()
        schedule(guid, "reanimate2", 2000, function()
            creature:UpdateEntry(ENTRY_REANIMATED_ADHERENT)
            creature:CastSpell(creature, SPELL_PERMANENT_FEIGN_DEATH)
            creature:CastSpell(creature, SPELL_CLEAR_ALL_DEBUFFS)
            creature:CastSpell(creature, SPELL_FULL_HEAL, true)
            creature:SetFlag(UNIT_FIELD_FLAGS,
                UNIT_FLAG_STUNNED + UNIT_FLAG_NOT_SELECTABLE + UNIT_FLAG_UNK_29)
        end)
        schedule(guid, "reanimate6", 6000, function()
            creature:RemoveAura(SPELL_PERMANENT_FEIGN_DEATH)
            creature:RemoveFlag(UNIT_FIELD_FLAGS,
                UNIT_FLAG_STUNNED + UNIT_FLAG_NOT_SELECTABLE + UNIT_FLAG_UNK_29)
            creature:SetReactState(REACT_AGGRESSIVE)
            creature:CastSpell(creature, SPELL_SHROUD_OF_THE_OCCULT)
        end)
    end
end

RegisterCreatureEvent(ENTRY_CULT_ADHERENT, 14, adherentSpellHit)
RegisterCreatureEvent(ENTRY_EMPOWERED_ADHERENT, 14, adherentSpellHit)
RegisterCreatureEvent(ENTRY_DARNAVAN_10, 1, darnavanJustEngagedWith)
RegisterCreatureEvent(ENTRY_DARNAVAN_10, 2, darnavanLeaveCombat)
RegisterCreatureEvent(ENTRY_DARNAVAN_10, 23, darnavanReset)
RegisterCreatureEvent(ENTRY_DARNAVAN_25, 1, darnavanJustEngagedWith)
RegisterCreatureEvent(ENTRY_DARNAVAN_25, 2, darnavanLeaveCombat)
RegisterCreatureEvent(ENTRY_DARNAVAN_25, 23, darnavanReset)
