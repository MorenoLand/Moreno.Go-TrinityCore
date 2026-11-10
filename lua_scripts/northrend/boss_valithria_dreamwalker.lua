-- Valithria Dreamwalker (Icecrown Citadel) — Lua port of
-- src/server/scripts/Northrend/IcecrownCitadel/boss_valithria_dreamwalker.cpp
-- (1269 lines incl. license; 10 CreatureScripts
-- (boss_valithria_dreamwalker (ScriptedAI, DATA_VALITHRIA_DREAMWALKER = 10),
-- npc_green_dragon_combat_trigger (BossAI),
-- npc_the_lich_king_controller (ScriptedAI),
-- npc_risen_archmage / npc_blazing_skeleton / npc_suppresser /
-- npc_blistering_zombie / npc_gluttonous_abomination (ScriptedAI),
-- npc_dream_portal (CreatureAI), npc_dream_cloud (ScriptedAI)) +
-- 9 SpellScripts (spell_dreamwalker_mana_void,
-- spell_dreamwalker_decay_periodic_timer, spell_dreamwalker_summoner,
-- spell_dreamwalker_summon_suppresser,
-- spell_dreamwalker_summon_suppresser_effect,
-- spell_dreamwalker_summon_dream_portal,
-- spell_dreamwalker_summon_nightmare_portal,
-- spell_dreamwalker_nightmare_cloud,
-- spell_dreamwalker_twisted_nightmares) + 1 AchievementCriteriaScript
-- (achievement_portal_jockey); all registered from inside
-- AddSC_boss_valithria_dreamwalker(); loader decl 181 / call 376 per
-- northrend_script_loader.cpp — the ELEVENTH group of the
-- "// Icecrown Citadel" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_sister_svalna() (call 375) — verified from the
-- loader this run; the checkpoint sequence (sister_svalna ->
-- valithria_dreamwalker) is followed).
-- Entries: 36789 Valithria Dreamwalker (icecrown_citadel.h
-- NPC_VALITHRIA_DREAMWALKER, line 287; instance_icecrown_citadel.cpp
-- OnCreatureCreate binds case NPC_VALITHRIA_DREAMWALKER, line 291),
-- 16980 the lich king controller (icecrown_citadel.h
-- NPC_THE_LICH_KING_VALITHRIA, line 297; OnCreatureCreate binds case
-- NPC_THE_LICH_KING_VALITHRIA, line 294) — entry-verifiable,
-- registration proceeds (the nexus_commanders kalecgos precedent); the
-- RegisterIcecrownCitadelCreatureAI ScriptName bindings are DB-side as
-- usual.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_valithria_dreamwalker" hits boss_valithria_dreamwalker.cpp
-- only (+ the loader decl/call lines); this clone carries no sql/ tree,
-- so ScriptName bindings are DB-side by construction. No valithria lua
-- existed.
-- Eluna creature events: 1 OnEnterCombat.
-- Ported arms (C++-exact for all modeled arms):
-- npc_the_lich_king_controller JustEngagedWith — Talk(SAY_LICH_KING_INTRO 0)
-- (event 1 — the auriaya engage-port precedent; the me->setActive(true)
-- leg has no bridge).
-- DOCUMENTED-ONLY (in this header; only entry 16980 registered):
-- boss_valithria_dreamwalker exposes zero bridgeable arms — NOT registered
-- (the bronjahm npc_corrupted_soul_fragment precedent), 8 Talk() calls
-- all on unbridged legs:
-- PORTED (2026-10-24): Talk(SAY_VALITHRIA_SUCCESS 7) + Talk(
-- SAY_VALITHRIA_75_PERCENT 2) ride the HealReceived legs via Go-side
-- event 11 (fireCreatureHealReceived): the 100% leg ports Talk +
-- DoCastSelf(SPELL_ACHIEVEMENT_CHECK 72706) + DoCastAOE(
-- SPELL_DREAMWALKERS_RAGE 71189, self-cast twin) + 3500ms
-- EVENT_DREAM_SLIP DoCastSelf(SPELL_DREAM_SLIP 71196) + its SpellHit
-- consequence (CLEAR_ALL + REPUTATION + invisible + NOT_SELECTABLE +
-- 4s despawn, event 14 valithriaSpellHit); SendEncounterUnit DISENGAGE
-- (:328) PORTED in valithriaHealReceived (tick 222); still out:
-- LowerPlayerDamageReq,
-- Initialize, lichKing EnterEvadeMode
-- (instance GUID), and the NOT_STARTED first-heal combat-start leg
-- (FindNearestCreature archmage + archmage:DoZoneInCombat() — PORTED
-- 2026-10-24 with a once-latch; the handler Talks nothing on that
-- branch, matching C++);
-- PORTED (2026-10-13): Talk(SAY_VALITHRIA_25_PERCENT 3) — the
-- DamageTaken HealthBelowPctDamaged(25, damage) leg with the
-- _under25PercentTalkDone latch (event 9; ironaya floored-integer
-- convention — algalon/moroes precedent). The death-negation leg
-- (damage = 0, _justDied flag) has no bridge — a lone damage=0 would
-- strand the encounter (algalon precedent);
-- Talk(SAY_VALITHRIA_DEATH 4) rides the DamageTaken death-negated leg
-- (damage = 0, _justDied flag — she never actually dies; the trigger
-- DoAction(ACTION_DEATH) cross-AI leg is PORTED 2026-11-06 below via
-- the instance GUID + the trigger ev38 handler); SendEncounterUnit
-- DISENGAGE (:365) PORTED in valithriaDamageTaken (tick 222); the Reset
-- DISENGAGE (:295) is PORTED in valithriaReset below (tick 222);
-- PORTED (2026-11-06): Talk(SAY_VALITHRIA_ENTER_COMBAT 0) rides
-- UpdateAI EVENT_INTRO_TALK + Talk(SAY_VALITHRIA_BERSERK 6) rides
-- EVENT_BERSERK (heroic-only) + Talk(SAY_VALITHRIA_DREAM_PORTAL 1)
-- rides EVENT_DREAM_PORTAL — all three schedules fire from DoAction
-- ACTION_ENTER_COMBAT (:305-313), now received via valithriaDoAction
-- (ev38) from the trigger engage leg; the SUMMON_PORTAL DoCast legs
-- (72224/72480 x3/8) are PORTED 2026-11-09 below, dormant until
-- Spell.dbc ships (the old "no-cast bridge" claim was stale);
-- SAY_VALITHRIA_PLAYER_DEATH 5 is declared in the text enum but never
-- referenced by a Talk() call — dead text evidence (no KilledUnit arm
-- exists in the file).
-- npc_the_lich_king_controller Reset scheduler legs (5 summon-timer
-- schedules), JustSummoned phase-mask / DoZoneInCombat legs, UpdateAI
-- summoner machine and the 5 DoCastSelf SPELL_TIMER_* legs
-- (70915/70912/70914/70916/70913) — stay out (2026-11-09 re-audit):
-- the timer-spell DBC trigger wiring is unknown in-tree, the ev5
-- force-cast leg needs the LK-GUID originalCaster (CastCustomSpell
-- accepts and ignores it — no Go model), and the ev7 filter binding
-- is a 5-spell DB inference; casting the timer spells raw would run
-- Go's default force-cast (boss_ai.go:6902, C++-exact except the
-- originalCaster) unfiltered. The JustSummoned legs are dormant
-- behind the same machine (bridges exist: SetPhaseMask/GetPhaseMask,
-- AttackStart = EngageWithTarget).
-- JustReachedHome me->setActive(false) (no bridge).
-- npc_green_dragon_combat_trigger (38752) — no Talk; the Reset
-- SummonCreatureGroup legs + ACTION_SETUP_ARCHMAGES relay are PORTED
-- below (greenDragonCombatTriggerReset, event 23 — the trigger was
-- registered 2026-11-05). PORTED (2026-11-06): the SetBossState legs
-- — Reset NOT_STARTED (ev23, != DONE-gated), engage IN_PROGRESS
-- (ev1, + DoZoneInCombat + valithria/archmage DoAction relays),
-- alive-leave-combat ACTION_DEATH NOT_STARTED (ev2), death DONE
-- (ev4, BossAI base _JustDied), ev38 ACTION_DEATH receiver, ev5
-- SetGuidData latch; the ValithriaDespawner grid sweep (:215-250, 5s
-- BasicEvent after ACTION_DEATH) has no BasicEvent bridge — stays out
-- (tick 222 triage); the CheckRequiredBosses gate + EngagementStart +
-- setActive legs have no bridge — stay out.
-- PORTED (2026-10-13 retraction re-audit): npc_risen_archmage
-- (37868) — FROSTBOLT_VOLLEY 70759 self-cast {5s,15s}/{8s,15s};
-- MANA_VOID 71179 random-player {20s,25s}/{20s,25s} (POWER_MANA
-- filter bridged via player:GetPowerType() — moroes.lua:294 precedent,
-- dist>15.0f kept, threat head excluded via victim GUID);
-- COLUMN_OF_FROST 70704 random-player {10s,20s}/{15s,25s},
-- dist>10.0f (:646-648/:705-721); the pre-combat SPELL_CORRUPTION
-- 70602 self-channel (:695) is PORTED below (corruptionTick, 1s poll
-- seeded by the ACTION_SETUP_ARCHMAGES DoAction relay); JustSummoned
-- column-of-frost / mana-void legs PORTED 2026-11-09 below (ev19,
-- risenArchmageJustSummoned); npc_blazing_skeleton (36791) —
-- FIREBALL 70754 victim-cast {2s,4s}/{2s,4s}, !IsWithinMeleeRange
-- gated (5-yd check, the venoxis precedent); LEY_WASTE 69325
-- self-cast {15s,20s}/{15s,20s} (:745-749/:760-773).
-- npc_suppresser (37863) — zero Talk lines; IsSummonedBy PORTED
-- 2026-11-05 (ev22 suppresserIsSummonedBy + ev5 valithriaSpawn latch
-- below); SUPPRESSION 70588 is MovementInform CHASE_MOTION_TYPE-driven
-- and stays out — Go's chase re-paths instead of finalizing, so ev28
-- never fires for chase (creaturemotion.go:5715-5718); no in-tree
-- consumer needs the chase inform.
-- PORTED (2026-10-13): npc_blistering_zombie (37934) JustDied —
-- DoCastSelf(SPELL_ACID_BURST 70744, triggered); npc_gluttonous_abomination
-- (37886) JustDied — DoCastSelf(SPELL_ROT_WORM_SPAWNER 70675, triggered)
-- unless killed by Valithria (event 4; the_black_morass killer-gate precedent).
-- PORTED (2026-10-13 retraction re-audit): npc_gluttonous_abomination —
-- GUT_SPRAY 70633 self-cast {10s,13s} init/re-arm (:871-872/:899).
-- CAST-STATE GATES PORTED (2026-11-04): boss UpdateAI :420/:447 gates
-- all four ported boss events (intro-talk, dream-portal, dream-slip,
-- berserk — all four defer); archmage :703/:728 gates all 3
-- ticks, skeleton :759/:779 gates both ticks, abomination :890
-- (pre-loop only) gates gut-spray; controller :590/:616 gates cover
-- unported summoner casts only (ported engage Talk ungated, separate
-- hook); suppresser :823 pre-loop gate covers the unported SUPPRESSION
-- arm only (NOT registered); blistering/dream_portal/dream_cloud
-- UpdateAIs have zero CASTING hits; JustDied/JustSummoned/SpellHit legs
-- ungated per C++ (separate hooks / BasicEvents, not the scheduler).
-- npc_dream_portal (37945) / npc_dream_cloud (entry DB-side, no header
-- constant) — zero Talk lines. npc_dream_cloud stays out (2026-11-09
-- re-audit): no cloud entry constant exists anywhere in the C++ tree,
-- so the AI cannot be registered, and no spawn path exists in Go
-- (clouds come only from the unported portal-spell chain); the body
-- bridges exist (GetPlayersInRange, MoveIdle = StopMoving,
-- DespawnOrUnsummon(ms), GetBossState) except SetCorpseDelay(0) and
-- the original-caster arg (accepted-and-ignored by CastCustomSpell).
-- PORTED 2026-11-08: npc_dream_portal
-- OnSpellClick (ev39 bridge) + _used/MISSED_PORTALS GetData leg (ev22
-- init, ev39 clear) + the pre-effect delayed-cast summon leg in
-- valithriaJustSummoned + the boss ev20 missed-counter;
-- achievement_portal_jockey PORTED the same tick (Go case reads the
-- portal slot; dormant until a pre-effect is summoned — the boss
-- EVENT_DREAM_PORTAL pre-summon targeting is DBC-side).
-- The 9 spell scripts (2026-11-09 re-audit; the blanket
-- "no-SpellScript-bridge" claim is STALE — RegisterSpellEvent ev5/ev7
-- + RegisterAuraEvent exist, runtime.go:422-560): mana_void PORTED
-- (RegisterAuraEvent 71085 kind 1, below); summon_dream_portal /
-- summon_nightmare_portal (ev5 SCRIPT_EFFECT on 71305/71987) stay out
-- — no live cast path in Go (the pre-effect cast is twinned as an
-- explicit SummonCreature in valithriaJustSummoned), so a handler
-- would be dead code; summoner (ev7 filter + ev5 force-cast on the
-- 70912-70916 timer spells), summon_suppresser_effect (ev5 on 70936)
-- and twisted_nightmares (ev5 EFFECT_2) stay out — DBC trigger-spell
-- ids unknown in-tree + the LK/Valithria-GUID originalCaster has no
-- Go model; summon_suppresser (aura 70912 OnApply/periodic) stays out
-- — dormant behind the unported controller machine (no live aura
-- source); decay_periodic_timer stays out — no update-periodic aura
-- event exists (runtime.go enum is 1-7); nightmare_cloud (aura 71970
-- periodic, boss-state gate) stays out — no live aura source (cloud
-- entries are DB-side, clouds never spawn).

local ENTRY_LICH_KING_VALITHRIA = 16980
local ENTRY_VALITHRIA_DREAMWALKER = 36789
local ENTRY_BLISTERING_ZOMBIE = 37934
local ENTRY_GLUTTONOUS_ABOMINATION = 37886
local ENTRY_SUPPRESSER = 37863 -- icecrown_citadel.h:291 (NPC_SUPPRESSER)
local DATA_VALITHRIA_DREAMWALKER = 10 -- icecrown_citadel.h:84

local SAY_LICH_KING_INTRO = 0
local SAY_VALITHRIA_ENTER_COMBAT = 0
local SAY_VALITHRIA_DREAM_PORTAL = 1
local SAY_VALITHRIA_75_PERCENT = 2
local SAY_VALITHRIA_25_PERCENT = 3
local SAY_VALITHRIA_DEATH = 4
local SAY_VALITHRIA_SUCCESS = 7
local SAY_VALITHRIA_BERSERK = 6
local SPELL_COPY_DAMAGE = 71948 -- boss_valithria_dreamwalker.cpp:49

local SPELL_DREAMWALKERS_RAGE = 71189
local SPELL_DREAM_SLIP = 71196
local SPELL_CLEAR_ALL = 71721
local SPELL_REPUTATION_BOSS_KILL = 73843

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000
local MODEL_INVISIBLE = 11686
local SPELL_ACHIEVEMENT_CHECK = 72706
local SPELL_CORRUPTION_VALITHRIA = 70904
local SPELL_DREAM_PORTAL_VISUAL_PRE = 71304
local SPELL_NIGHTMARE_PORTAL_VISUAL_PRE = 71986
local ENTRY_DREAM_PORTAL_PRE_EFFECT = 38186
local ENTRY_NIGHTMARE_PORTAL_PRE_EFFECT = 38429
local ENTRY_DREAM_PORTAL = 37945 -- icecrown_citadel.h:300 (NPC_DREAM_PORTAL)
local ENTRY_NIGHTMARE_PORTAL = 38430 -- icecrown_citadel.h:301 (NPC_NIGHTMARE_PORTAL)
local MISSED_PORTALS = 2 -- boss_valithria_dreamwalker.cpp:149

local SPELL_ACID_BURST = 70744
local SPELL_ROT_WORM_SPAWNER = 70675

local ENTRY_RISEN_ARCHMAGE = 37868
local ENTRY_BLAZING_SKELETON = 36791
local ENTRY_GREEN_DRAGON_COMBAT_TRIGGER = 38752 -- icecrown_citadel.h:288 (NPC_GREEN_DRAGON_COMBAT_TRIGGER)
local SUMMON_GROUP_ALL = 1 -- boss_valithria_dreamwalker.cpp:154
local SUMMON_GROUP_10 = 2 -- boss_valithria_dreamwalker.cpp:155
local SUMMON_GROUP_25 = 3 -- boss_valithria_dreamwalker.cpp:156
local ACTION_ENTER_COMBAT = 1 -- boss_valithria_dreamwalker.cpp:148
local ACTION_DEATH = 3 -- boss_valithria_dreamwalker.cpp:150
local ACTION_SETUP_ARCHMAGES = 4 -- boss_valithria_dreamwalker.cpp:151
local BOSS_STATE_NOT_STARTED = 0 -- InstanceScript.h:68-76
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_FAIL = 2
local BOSS_STATE_DONE = 3
local SPELL_CORRUPTION_ARCHMAGE = 70602 -- boss_valithria_dreamwalker.cpp:695
local DATA_VALITHRIA_TRIGGER = 35 -- icecrown_citadel.h:111
local DATA_VALITHRIA_LICH_KING = 36 -- icecrown_citadel.h:112

local SPELL_GUT_SPRAY = 70633
local SPELL_FROSTBOLT_VOLLEY = 70759
local SPELL_MANA_VOID = 71179
local SPELL_MANA_VOID_AURA = 71085
local SPELL_COLUMN_OF_FROST = 70704
local SPELL_COLUMN_OF_FROST_AURA = 70715 -- boss_valithria_dreamwalker.cpp:64
local SPELL_COLUMN_OF_FROST_DAMAGE = 70702 -- boss_valithria_dreamwalker.cpp:81
local ENTRY_COLUMN_OF_FROST = 37918 -- icecrown_citadel.h:295 (NPC_COLUMN_OF_FROST)
local ENTRY_MANA_VOID = 38068 -- icecrown_citadel.h:294 (NPC_MANA_VOID)
local SPELL_SPAWN_CHEST = 71207 -- boss_valithria_dreamwalker.cpp:74
local SPELL_PRE_SUMMON_DREAM_PORTAL = 72224 -- boss_valithria_dreamwalker.cpp:54
local SPELL_PRE_SUMMON_NIGHTMARE_PORTAL = 72480 -- boss_valithria_dreamwalker.cpp:55
local SPELL_FIREBALL = 70754
local SPELL_LEY_WASTE = 69325

local POWER_MANA = 0

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

-- C++ :715 SelectTarget(Random, 1, ValithriaManaVoidSelector(me)) —
-- POWER_MANA + dist>15.0f, no player restriction (the selector
-- carries no TYPEID check; the old "player" wording was wrong);
-- C++ :720 SelectTarget(Random, 1, -10.0f, true) — min 10 yd,
-- playerOnly. Position=1 in both pops the victim BEFORE filters
-- (UnitAI.h victim-first list, akilzon idiom); GetPowerType
-- is bridged (moroes.lua:294 precedent, phase-hunter any-unit gate).
-- The IsDead gate follows the settled strip convention (C++
-- skips only offline refs; dead picks are no-op casts).
local function randomPlayer(creature, minDist, manaOnly)
    local victim = creature:GetVictim()
    local victimGuid = nil
    if victim ~= nil then
        victimGuid = victim:GetGUID()
    end
    local candidates = {}
    local reach = creature:GetCombatReach()
    local aliveSeen = 0
    for _, unit in ipairs(creature:GetAITargets()) do
        local dist = creature:GetDistance(unit)
        local targetReach = 1.5
        if manaOnly and unit:GetObjectType() ~= "Player" then
            targetReach = unit:GetCombatReach()
        end
        local pastMin
        if manaOnly then
            pastMin = dist - reach - targetReach > minDist -- Mana-void selector: C++ GetDistance > 15 (both reaches, Object.cpp:1169)
        else
            pastMin = dist - reach - targetReach >= minDist -- :720 min-10 arm, playerOnly: player reach 1.5
        end
        local skip = false
        if victimGuid ~= nil then
            skip = unit:GetGUID() == victimGuid
        elseif not unit:IsDead() then
            aliveSeen = aliveSeen + 1
            skip = aliveSeen == 1
        end
        if not skip
                and not unit:IsDead()
                and pastMin
                and (not manaOnly or unit:GetPowerType() == POWER_MANA)
                and (manaOnly or unit:GetObjectType() == "Player") then
            candidates[#candidates + 1] = unit
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ npc_gluttonous_abomination::JustEngagedWith (Reset seeds the
-- same timers; the maiden engage-hook convention): EVENT_GUT_SPRAY —
-- DoCastSelf(SPELL_GUT_SPRAY) non-triggered, {10s,13s}/{10s,13s}
-- (:871-872/:899).
local function onGutSpray(creature, guid)
    -- C++ gluttonous UpdateAI pre-loop gate :890 (no in-loop return): 500ms IsCasting defer, no re-arm consumed (mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "gut_spray", 500, function() onGutSpray(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_GUT_SPRAY)
    schedule(guid, "gut_spray", {10000, 13000}, function() onGutSpray(creature, guid) end)
end

local function gluttonousAbominationJustEngagedWith(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "gut_spray", {10000, 13000}, function() onGutSpray(creature, guid) end)
end

local function gluttonousAbominationLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ npc_risen_archmage::Reset — EVENT_FROSTBOLT_VOLLEY {5s,15s},
-- EVENT_MANA_VOID {20s,25s}, EVENT_COLUMN_OF_FROST {10s,20s}
-- (:646-648).
local function onFrostboltVolley(creature, guid)
    -- C++ archmage UpdateAI pre-loop gate :703 (+ in-loop :728): 500ms IsCasting defer, no re-arm consumed (mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "frostbolt_volley", 500, function() onFrostboltVolley(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_FROSTBOLT_VOLLEY)
    schedule(guid, "frostbolt_volley", {8000, 15000}, function() onFrostboltVolley(creature, guid) end)
end

-- C++ :715 EVENT_MANA_VOID — ValithriaManaVoidSelector: POWER_MANA
-- threat unit, dist>15.0f (:161-170), position=1 threat-head exclusion.
local function onManaVoid(creature, guid)
    -- C++ archmage UpdateAI pre-loop gate :703 (+ in-loop :728); defer precedes the target pick: 500ms IsCasting defer, no re-arm consumed (mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "mana_void", 500, function() onManaVoid(creature, guid) end)
        return
    end
    local target = randomPlayer(creature, 15.0, true)
    if target then
        creature:CastSpell(target, SPELL_MANA_VOID)
    end
    schedule(guid, "mana_void", {20000, 25000}, function() onManaVoid(creature, guid) end)
end

-- C++ :720 EVENT_COLUMN_OF_FROST — SelectTarget(Random, 1, -10.0f, true):
-- negative dist rejects IsWithinCombatRange(10) (UnitAI.cpp:276-277), so
-- the pool is raw >= 10 + reaches; twinned d - ownReach >= 10 (the mana
-- arm is the reach-subtracted GetDistance > 15 twin, :169).
local function onColumnOfFrost(creature, guid)
    -- C++ archmage UpdateAI pre-loop gate :703 (+ in-loop :728); defer precedes the target pick: 500ms IsCasting defer, no re-arm consumed (mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "column_of_frost", 500, function() onColumnOfFrost(creature, guid) end)
        return
    end
    local target = randomPlayer(creature, 10.0)
    if target then
        creature:CastSpell(target, SPELL_COLUMN_OF_FROST)
    end
    schedule(guid, "column_of_frost", {15000, 25000}, function() onColumnOfFrost(creature, guid) end)
end

-- Forward declarations (tharon_ja/gal_darah idiom):
-- risenArchmageJustEngagedWith / risenArchmageLeaveCombat reference these
-- before their definitions below; assigned there by plain assignment,
-- never re-`local function` (that would shadow the forward slot and leave
-- the closures calling nil).
local initialArchmage = {}
local corruptionTick

-- C++ npc_risen_archmage engage (:657): InterruptNonMeleeSpells(false)
-- PORTED first below (gal_darah precedent). The old "dropped — StopSpellCast
-- is an explicit no-op" note conflated two methods and a stale line cite:
-- InterruptNonMeleeSpells is a live bridge (lua_creature_events.go:1995,
-- cancels pending/channel casts) and StopSpellCast interrupts too (:985).
local function risenArchmageJustEngagedWith(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    creature:InterruptNonMeleeSpells(false)
    -- C++ JustEnteredCombat (:659-668): if _isInitialArchmage,
    -- DoZoneInCombat(lichKing) + DoZoneInCombat(trigger) via instance GUIDs.
    if initialArchmage[guid] then
        local lichKing = creature:GetInstanceCreature(DATA_VALITHRIA_LICH_KING)
        if lichKing then
            lichKing:DoZoneInCombat()
        end
        local trigger = creature:GetInstanceCreature(DATA_VALITHRIA_TRIGGER)
        if trigger then
            trigger:DoZoneInCombat()
        end
    end
    schedule(guid, "frostbolt_volley", {5000, 15000}, function() onFrostboltVolley(creature, guid) end)
    schedule(guid, "mana_void", {20000, 25000}, function() onManaVoid(creature, guid) end)
    schedule(guid, "column_of_frost", {10000, 20000}, function() onColumnOfFrost(creature, guid) end)
end

local function risenArchmageLeaveCombat(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    -- C++ UpdateAI :695 keeps polling out of combat after an evade while
    -- _isInitialArchmage stands (the flag survives Reset — only DoAction
    -- sets it, never clears it); re-arm the corruption poll on leave/reset.
    if initialArchmage[guid] then
        schedule(guid, "corruption", 1000, function() corruptionTick(creature, guid) end)
    end
end

-- C++ npc_risen_archmage::UpdateAI out-of-combat leg (:695): alive + not
-- in combat + _isInitialArchmage + no channeled spell -> DoCastSelf(
-- SPELL_CORRUPTION 70602). Twinned as a 1s poll (the maiden CreateLuaEvent
-- convention); the IsCasting guard is the
-- GetCurrentSpell(CURRENT_CHANNELED_SPELL) twin. The poll starts on
-- ACTION_SETUP_ARCHMAGES and stops on engage/leave/reset via cancelTimers.
corruptionTick = function(creature, guid)
    if creature:IsInCombat() or not creature:IsAlive() then
        return
    end
    if not creature:IsCasting() then
        creature:CastSpell(creature, SPELL_CORRUPTION_ARCHMAGE)
    end
    schedule(guid, "corruption", 1000, function() corruptionTick(creature, guid) end)
end

-- C++ npc_risen_archmage::DoAction (:671-677): ACTION_ENTER_COMBAT (1) ->
-- DoZoneInCombat(); ACTION_SETUP_ARCHMAGES (4) -> _isInitialArchmage = true.
-- The sender is the trigger's Reset/JustEnteredCombat summons.DoAction relay
-- (ev38 dispatch bridged; the Reset relay ports below via the returned
-- SummonCreatureGroup tables).
local function risenArchmageDoAction(event, creature, action)
    if action == ACTION_ENTER_COMBAT then
        creature:DoZoneInCombat()
    elseif action == ACTION_SETUP_ARCHMAGES then
        local guid = creature:GetGUIDLow()
        initialArchmage[guid] = true
        schedule(guid, "corruption", 1000, function() corruptionTick(creature, guid) end)
    end
end

-- C++ npc_risen_archmage::JustSummoned (boss_valithria_dreamwalker.cpp:678-690):
-- COLUMN_OF_FROST summon self-casts SPELL_COLUMN_OF_FROST_AURA 70715
-- (triggered) + a 2s ValithriaDelayedCastEvent for
-- SPELL_COLUMN_OF_FROST_DAMAGE 70702 (plain self-cast, originalCaster
-- empty) that despawns the summon 8s after the cast; MANA_VOID summon
-- self-casts SPELL_MANA_VOID_AURA 71085 (triggered) +
-- DespawnOrUnsummon(36s). PORTED 2026-11-09: the archmage's own 70704 /
-- 71179 casts were already ported, so both summons land in Go today and
-- were inert until now (no 36s mana-void / 10s column despawn legs).
local function risenArchmageJustSummoned(event, creature, summoned)
    local entry = summoned:GetEntry()
    if entry == ENTRY_COLUMN_OF_FROST then
        summoned:CastSpell(summoned, SPELL_COLUMN_OF_FROST_AURA, true)
        CreateLuaEvent(function()
            summoned:CastSpell(summoned, SPELL_COLUMN_OF_FROST_DAMAGE)
            CreateLuaEvent(function()
                summoned:DespawnOrUnsummon()
            end, 8000)
        end, 2000)
    elseif entry == ENTRY_MANA_VOID then
        summoned:CastSpell(summoned, SPELL_MANA_VOID_AURA, true)
        summoned:DespawnOrUnsummon(36000)
    end
end

-- C++ npc_green_dragon_combat_trigger::Reset (boss_valithria_dreamwalker.cpp:476-489):
-- _Reset + SetReactState(REACT_PASSIVE) + summons.DespawnAll() +
-- SummonCreatureGroup(SUMMON_GROUP_ALL 1) + 25-man SummonCreatureGroup(
-- SUMMON_GROUP_25 3) / 10-man SummonCreatureGroup(SUMMON_GROUP_10 2) +
-- summons.DoAction(ACTION_SETUP_ARCHMAGES, archmage-pred). The DespawnAll
-- has no summon-list bridge — twinned as a grid sweep (freya-elder-sweep
-- precedent: archmages spawn only from these groups, so the room holds no
-- pre-placed 37868). The DoAction relay rides the returned
-- SummonCreatureGroup tables, entry-filtered (group coords DB-side;
-- creature:SummonCreatureGroup bridged 2026-11-05, tick 238).
local function greenDragonCombatTriggerReset(event, creature)
    -- C++ Reset() :478 — _Reset() first: SetBossState NOT_STARTED
    -- only when the stored state != DONE (BossAI::_Reset,
    -- ScriptedCreature.cpp:499-509; the rotface precedent — ev23
    -- fires on spawn/death/evade too, and a DONE boss must stay
    -- DONE). PORTED 2026-11-06 via creature:SetBossState.
    if creature:GetBossState(DATA_VALITHRIA_DREAMWALKER) ~= BOSS_STATE_DONE then
        creature:SetBossState(DATA_VALITHRIA_DREAMWALKER, BOSS_STATE_NOT_STARTED)
    end
    creature:SetReactState(0) -- REACT_PASSIVE
    for _, archmage in ipairs(creature:GetCreaturesInGrid(ENTRY_RISEN_ARCHMAGE, 100)) do
        archmage:Despawn()
    end
    local summoned = creature:SummonCreatureGroup(SUMMON_GROUP_ALL)
    local extra
    if creature:Is25ManRaid() then
        extra = creature:SummonCreatureGroup(SUMMON_GROUP_25)
    else
        extra = creature:SummonCreatureGroup(SUMMON_GROUP_10)
    end
    for _, summon in ipairs(extra) do
        table.insert(summoned, summon)
    end
    for _, summon in ipairs(summoned) do
        if summon:GetEntry() == ENTRY_RISEN_ARCHMAGE then
            summon:DoAction(ACTION_SETUP_ARCHMAGES)
        end
    end
end

-- C++ npc_green_dragon_combat_trigger::DoAction ACTION_DEATH
-- (boss_valithria_dreamwalker.cpp:529-537): SetBossState NOT_STARTED
-- + 5s ValithriaDespawner + lichKing EnterEvadeMode. The despawner
-- (BasicEvent grid sweep, :215-250) has no bridge and stays out; the
-- EnterEvadeMode leg is PORTED 2026-11-09 below (bridged,
-- lua_creature_events.go:5004). The state leg is the portable arm,
-- shared by the ev2 (alive JustExitedCombat) and ev38 (cross-AI
-- DoAction) paths below.
local function triggerActionDeath(creature)
    creature:SetBossState(DATA_VALITHRIA_DREAMWALKER, BOSS_STATE_NOT_STARTED)
    local lichKing = creature:GetInstanceCreature(DATA_VALITHRIA_LICH_KING)
    if lichKing then
        lichKing:EnterEvadeMode()
    end
end

local function greenDragonCombatTriggerDoAction(event, creature, action)
    if action == ACTION_DEATH then
        triggerActionDeath(creature)
    end
end

-- C++ npc_green_dragon_combat_trigger::JustEnteredCombat (:496-513):
-- CheckRequiredBosses gate (no bridge, stays out) then EngagementStart
-- + setActive (no bridges) + DoZoneInCombat + SetBossState IN_PROGRESS
-- + valithria DoAction(ACTION_ENTER_COMBAT) via the instance GUID +
-- archmage summons DoAction(ACTION_ENTER_COMBAT). The archmage relay
-- rides a grid sweep (the trigger Reset sweep precedent — exactly the
-- summoned archmages are in the room). PORTED 2026-11-06.
local function greenDragonCombatTriggerJustEngagedWith(event, creature, target)
    creature:DoZoneInCombat()
    creature:SetBossState(DATA_VALITHRIA_DREAMWALKER, BOSS_STATE_IN_PROGRESS)
    local valithria = creature:GetInstanceCreature(DATA_VALITHRIA_DREAMWALKER)
    if valithria then
        valithria:DoAction(ACTION_ENTER_COMBAT)
    end
    for _, archmage in ipairs(creature:GetCreaturesInGrid(ENTRY_RISEN_ARCHMAGE, 100)) do
        archmage:DoAction(ACTION_ENTER_COMBAT)
    end
end

-- C++ npc_green_dragon_combat_trigger::JustExitedCombat (:515-525):
-- EngagementOver + setActive(false) (no bridges); if the trigger is
-- dead, return (the death path rides ev4 DONE below); alive, it
-- DoActions ACTION_DEATH. The ev2 alive-gate is the C++ early return.
local function greenDragonCombatTriggerLeaveCombat(event, creature)
    if creature:IsAlive() then
        triggerActionDeath(creature)
    end
end

-- C++ BossAI base _JustDied: the trigger IS the BossAI for
-- DATA_VALITHRIA_DREAMWALKER, so its death writes DONE (the encounter
-- completes when Valithria's SpellHit kills the trigger, :388-390).
-- PORTED 2026-11-06 via creature:SetBossState on ev4.
local function greenDragonCombatTriggerJustDied(event, creature, killer)
    creature:SetBossState(DATA_VALITHRIA_DREAMWALKER, BOSS_STATE_DONE)
end

-- C++ instance_icecrown_citadel.cpp OnCreatureCreate (:302-304) latches
-- ValithriaTriggerGUID when NPC_GREEN_DRAGON_COMBAT_TRIGGER spawns;
-- the Lua mirror is the ev5 SetGuidData latch (the valithriaSpawn
-- precedent below) so GetInstanceCreature(DATA_VALITHRIA_TRIGGER)
-- resolves for the Valithria lethal-damage leg. PORTED 2026-11-06.
local function greenDragonCombatTriggerSpawn(event, creature)
    creature:SetGuidData(DATA_VALITHRIA_TRIGGER)
end

-- C++ instance_icecrown_citadel.cpp OnCreatureCreate (:299-301) latches
-- ValithriaLichKingGUID; same ev5 mirror (the lichKing controller
-- engage Talk rides ev1 above). PORTED 2026-11-06.
local function lichKingValithriaSpawn(event, creature)
    creature:SetGuidData(DATA_VALITHRIA_LICH_KING)
end

-- C++ npc_blazing_skeleton::Reset — EVENT_FIREBALL {2s,4s},
-- EVENT_LEY_WASTE {15s,20s} (:745-749); fireball gated on
-- !IsWithinMeleeRange (C++ :767), bridged (lua_creature_events.go:3144).
local function onFireball(creature, guid)
    -- C++ skeleton UpdateAI pre-loop gate :759 (+ in-loop :779); defer precedes the melee-range check: 500ms IsCasting defer, no re-arm consumed (mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "fireball", 500, function() onFireball(creature, guid) end)
        return
    end
    local victim = creature:GetVictim()
    if victim and not creature:IsWithinMeleeRange(victim) then
        creature:CastSpell(nil, SPELL_FIREBALL)
    end
    schedule(guid, "fireball", {2000, 4000}, function() onFireball(creature, guid) end)
end

local function onLeyWaste(creature, guid)
    -- C++ skeleton UpdateAI pre-loop gate :759 (+ in-loop :779): 500ms IsCasting defer, no re-arm consumed (mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "ley_waste", 500, function() onLeyWaste(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_LEY_WASTE)
    schedule(guid, "ley_waste", {15000, 20000}, function() onLeyWaste(creature, guid) end)
end

local function blazingSkeletonJustEngagedWith(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "fireball", {2000, 4000}, function() onFireball(creature, guid) end)
    schedule(guid, "ley_waste", {15000, 20000}, function() onLeyWaste(creature, guid) end)
end

local function blazingSkeletonLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ npc_the_lich_king_controller::JustEngagedWith:
-- Talk(SAY_LICH_KING_INTRO) — the auriaya engage-port precedent.
local function lichKingJustEngagedWith(event, creature, target)
    creature:Talk(SAY_LICH_KING_INTRO)
end

local valithriaState = {}

-- C++ boss_valithria_dreamwalker::DamageTaken — HealthBelowPctDamaged(
-- 25, damage) = (health - damage) < floor(max*25/100) (Unit.h:873;
-- ironaya floored-integer convention) + the _under25PercentTalkDone
-- latch — Talk(SAY_VALITHRIA_25_PERCENT) (:348-356); the lethal leg
-- (damage >= health) negates damage to 0 + the _justDied latch —
-- Talk(SAY_VALITHRIA_DEATH) (:357-366). In C++ she never dies; the
-- encounter completion rides the cross-AI ACTION_DEATH leg (no
-- bridge), but the no-death + Talk behavior is exactly bridgeable
-- (return false, 0 — julianne/curator damage-negation precedent).
local function valithriaDamageTaken(_, creature, _, damage)
    local guid = creature:GetGUIDLow()
    local st = valithriaState[guid] or {}
    valithriaState[guid] = st
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return false, damage
    end
    if (creature:GetHealth() - damage) < math.floor(maxHealth * 25 / 100) then
        if not st.talk25Done then
            st.talk25Done = true
            creature:Talk(SAY_VALITHRIA_25_PERCENT)
        end
        if damage >= creature:GetHealth() then
            if not st.justDied then
                st.justDied = true
                creature:Talk(SAY_VALITHRIA_DEATH)
                -- C++ :365 — SendEncounterUnit(ENCOUNTER_FRAME_DISENGAGE,
                -- me), after Talk, before the trigger DoAction(ACTION_DEATH)
                -- leg (:366-367) — PORTED 2026-11-06 via the instance GUID
                -- (the trigger ev38 handler ports the NOT_STARTED leg;
                -- the ev5 latch above resolves the GUID).
                creature:SendEncounterUnit(1, creature)
                local trigger = creature:GetInstanceCreature(DATA_VALITHRIA_TRIGGER)
                if trigger then
                    trigger:DoAction(ACTION_DEATH)
                end
            end
            return false, 0
        end
    end
    return false, damage
end

-- C++ boss_valithria_dreamwalker UpdateAI pre-loop gate :420 (+ in-loop
-- :447): 500ms IsCasting defer, cast not consumed (mal_ganis idiom).
local function onDreamSlip(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "dream_slip", 500, function() onDreamSlip(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_DREAM_SLIP)
end

-- C++ boss_valithria_dreamwalker::HealReceived (:316-341) — strict
-- HealthAbovePctHealed (Unit.h:875: health + heal > floor(max*pct/100),
-- pre-clamp addhealth, ahead of the health gain) chained exactly as C++:
-- 100% + !_done -> Talk(SAY_VALITHRIA_SUCCESS) + DoCastSelf(
-- SPELL_ACHIEVEMENT_CHECK) + DoCastAOE(SPELL_DREAMWALKERS_RAGE) +
-- 3500ms EVENT_DREAM_SLIP DoCastSelf(SPELL_DREAM_SLIP); else 75% +
-- !_over75PercentTalkDone -> Talk(SAY_VALITHRIA_75_PERCENT); else the
-- NOT_STARTED combat-start leg (archmage DoZoneInCombat, PORTED
-- 2026-10-24 below with a once-latch). Talks nothing, so the handler
-- does nothing else on that branch. The
-- _done/_over75PercentTalkDone latches live in valithriaState, so the
-- event-23 reset clears them (C++ Initialize()).
local function valithriaHealReceived(_, creature, healer, heal)
    local guid = creature:GetGUIDLow()
    local st = valithriaState[guid] or {}
    valithriaState[guid] = st
    -- C++ :317-318 — first heal taps the healer as loot recipient,
    -- guarded by hasLootRecipient (the healer arg is the player
    -- object for player heals; a creature healer is a no-op in
    -- SetLootRecipient, matching C++'s non-player return).
    if healer and not creature:HasLootRecipient() then
        creature:SetLootRecipient(healer)
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    local healed = creature:GetHealth() + heal
    if healed > math.floor(maxHealth * 100 / 100) and not st.done then
        st.done = true
        creature:Talk(SAY_VALITHRIA_SUCCESS)
        -- C++ :328 — SendEncounterUnit(ENCOUNTER_FRAME_DISENGAGE, me),
        -- after Talk, before the corruption-strip (tick 222).
        creature:SendEncounterUnit(1, creature)
        creature:RemoveAura(SPELL_CORRUPTION_VALITHRIA)
        creature:CastSpell(creature, SPELL_ACHIEVEMENT_CHECK)
        creature:CastSpell(creature, SPELL_DREAMWALKERS_RAGE)
        schedule(guid, "dream_slip", 3500, function() onDreamSlip(creature, guid) end)
        -- C++ :338-339 — the Lich King controller evades on the
        -- successful heal (EnterEvadeMode bridged,
        -- lua_creature_events.go:5004; the 2026-10-21 "no bridge"
        -- note is stale — AGENTS.md 2026-11-08). PORTED 2026-11-09.
        local lichKing = creature:GetInstanceCreature(DATA_VALITHRIA_LICH_KING)
        if lichKing then
            lichKing:EnterEvadeMode()
        end
    elseif healed > math.floor(maxHealth * 75 / 100) and not st.over75Done then
        st.over75Done = true
        creature:Talk(SAY_VALITHRIA_75_PERCENT)
    elseif not st.done and not st.encounterStarted then
        st.encounterStarted = true
        local archmage = creature:FindNearestCreature(ENTRY_RISEN_ARCHMAGE, 30.0)
        if archmage then
            archmage:DoZoneInCombat()
        end
    end
end

-- C++ boss_valithria_dreamwalker::SpellHit (:376-393) — fires on the
-- DREAM_SLIP self-cast hit via event 14 (moorabi precedent: ev14 fires
-- once per hit target, self-cast included; handler filters on spellId).
-- C++ order: DoCastSelf(CLEAR_ALL) + DoCastSelf(REPUTATION, triggered)
-- + SetDisplayId(11686) + SetFlag(NOT_SELECTABLE) +
-- DespawnOrUnsummon(4s, lich_king_escape one-shot CreateLuaEvent
-- precedent). Trigger Kill PORTED 2026-11-06 below; the lichKing chest
-- cast (SPELL_SPAWN_CHEST 71207, :387) is PORTED 2026-11-09 below. If ev14 never fires for this cast
-- (spell-data gap), the arm is inert — no regression vs the old
-- cast-only timer.
local function valithriaSpellHit(event, creature, caster, spellId)
    if spellId ~= SPELL_DREAM_SLIP then
        return
    end
    creature:CastSpell(creature, SPELL_CLEAR_ALL)
    creature:CastSpell(creature, SPELL_REPUTATION_BOSS_KILL, true)
    creature:SetDisplayId(MODEL_INVISIBLE)
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    -- C++ :388-390 — Unit::Kill(me, trigger): Valithria kills the
    -- combat trigger; the trigger's ev4 BossAI base _JustDied then
    -- writes DONE (the encounter completion leg). The lichKing chest
    -- cast stays out (cross-AI spell, no bridge). PORTED 2026-11-06
    -- via creature:Kill(killer) (lua_creature_events.go:2971).
    local trigger = creature:GetInstanceCreature(DATA_VALITHRIA_TRIGGER)
    if trigger then
        trigger:Kill(creature)
    end
    -- C++ :387 — the Lich King controller casts SPELL_SPAWN_CHEST
    -- 71207 on himself (cross-creature CastSpell via the instance
    -- GUID, the trigger:Kill precedent above; the "no bridge" header
    -- claim is stale — CastSpell is generic,
    -- lua_creature_events.go:896). PORTED 2026-11-09.
    local lichKing = creature:GetInstanceCreature(DATA_VALITHRIA_LICH_KING)
    if lichKing then
        lichKing:CastSpell(lichKing, SPELL_SPAWN_CHEST)
    end
    CreateLuaEvent(function()
        creature:Despawn()
    end, 4000)
end

-- C++ boss_valithria_dreamwalker UpdateAI EVENT_INTRO_TALK (:424-426):
-- Talk(SAY_VALITHRIA_ENTER_COMBAT 0), one-shot 15s from DoAction
-- ACTION_ENTER_COMBAT. The UpdateAI gate (:420: state must be
-- IN_PROGRESS) is the re-check below.
local function onValithriaIntroTalk(creature, guid)
    if creature:GetBossState(DATA_VALITHRIA_DREAMWALKER) ~= BOSS_STATE_IN_PROGRESS then
        return
    end
    -- C++ UpdateAI pre-loop gate :420 (+ in-loop :447): 500ms IsCasting defer, no re-arm consumed (mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "intro_talk", 500, function() onValithriaIntroTalk(creature, guid) end)
        return
    end
    creature:Talk(SAY_VALITHRIA_ENTER_COMBAT)
end

-- C++ UpdateAI EVENT_DREAM_PORTAL (:431-440): non-heroic Talk(
-- SAY_VALITHRIA_DREAM_PORTAL 1) + _portalCount DoCastSelf(SUMMON_PORTAL)
-- + {45s,48s} re-arm. PORTED 2026-11-09: the casts ride the
-- CreateLuaEvent scheduler (the "no-cast bridge" header claim is
-- stale); the pre-summon spell's scatter positions are DBC-side, so
-- the casts are dormant until Spell.dbc ships, and the pre-effect
-- legs already ride valithriaJustSummoned. The UpdateAI gate
-- (:420) is the re-check below.
local function onValithriaDreamPortal(creature, guid)
    if creature:GetBossState(DATA_VALITHRIA_DREAMWALKER) ~= BOSS_STATE_IN_PROGRESS then
        return
    end
    -- C++ UpdateAI pre-loop gate :420 (+ in-loop :447): 500ms IsCasting defer, no re-arm consumed (mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "dream_portal", 500, function() onValithriaDreamPortal(creature, guid) end)
        return
    end
    if not creature:IsHeroic() then
        creature:Talk(SAY_VALITHRIA_DREAM_PORTAL)
    end
    -- C++ :436-438 + :104-105 — SUMMON_PORTAL is RAID_MODE 72224
    -- (10-man) / 72480 (25-man); _portalCount is RAID_MODE 3 / 8.
    local portalSpell = SPELL_PRE_SUMMON_DREAM_PORTAL
    local portalCount = 3
    if creature:Is25ManRaid() then
        portalSpell = SPELL_PRE_SUMMON_NIGHTMARE_PORTAL
        portalCount = 8
    end
    for _ = 1, portalCount do
        creature:CastSpell(creature, portalSpell)
    end
    schedule(guid, "dream_portal", {45000, 48000}, function() onValithriaDreamPortal(creature, guid) end)
end

-- C++ UpdateAI EVENT_BERSERK (:427-430, heroic-only): Talk(
-- SAY_VALITHRIA_BERSERK 6), one-shot 420s from DoAction
-- ACTION_ENTER_COMBAT, gated on IN_PROGRESS (:420).
local function onValithriaBerserk(creature, guid)
    if creature:GetBossState(DATA_VALITHRIA_DREAMWALKER) ~= BOSS_STATE_IN_PROGRESS then
        return
    end
    -- C++ UpdateAI pre-loop gate :420 (+ in-loop :447): 500ms IsCasting defer, no re-arm consumed (mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "berserk", 500, function() onValithriaBerserk(creature, guid) end)
        return
    end
    creature:Talk(SAY_VALITHRIA_BERSERK)
end

-- C++ boss_valithria_dreamwalker::DoAction ACTION_ENTER_COMBAT (:305-313):
-- DoCastSelf(SPELL_COPY_DAMAGE) + SendEncounterUnit ENGAGE +
-- EVENT_INTRO_TALK 15s + EVENT_DREAM_PORTAL {45s,48s} + heroic
-- EVENT_BERSERK 420s. Fired by the trigger engage leg (ev1 above);
-- the UpdateAI IN_PROGRESS gate (:420) is re-checked in each timer
-- callback. PORTED 2026-11-06 (the stale "sender never fires it"
-- header claim is retracted).
local function valithriaDoAction(event, creature, action)
    if action ~= ACTION_ENTER_COMBAT then
        return
    end
    local guid = creature:GetGUIDLow()
    creature:CastSpell(creature, SPELL_COPY_DAMAGE)
    creature:SendEncounterUnit(0, creature)
    schedule(guid, "intro_talk", 15000, function() onValithriaIntroTalk(creature, guid) end)
    schedule(guid, "dream_portal", {45000, 48000}, function() onValithriaDreamPortal(creature, guid) end)
    if creature:IsHeroic() then
        schedule(guid, "berserk", 420000, function() onValithriaBerserk(creature, guid) end)
    end
end

local function valithriaReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    valithriaState[creature:GetGUIDLow()] = nil
    -- C++ boss_valithria_dreamwalker.cpp:288: me->SetReactState(REACT_PASSIVE)
    -- (Reset init arm; no flip anywhere in C++).
    creature:SetReactState(0) -- REACT_PASSIVE
    -- C++ :291-294 — percent-heal immunity arms (bridged
    -- lua_creature_events.go; ops 1 STATE / 0 EFFECT / 6 ID).
    creature:ApplySpellImmune(0, 1, 20, true)
    creature:ApplySpellImmune(0, 0, 136, true)
    creature:ApplySpellImmune(0, 6, 56131, true)
    -- C++ :295 — SendEncounterUnit(ENCOUNTER_FRAME_DISENGAGE, me), after
    -- the immunities, before Initialize() (tick 222).
    creature:SendEncounterUnit(1, creature)
end

-- C++ instance_icecrown_citadel.cpp OnCreatureCreate (:291-292) latches
-- ValithriaDreamwalkerGUID when NPC_VALITHRIA_DREAMWALKER spawns; the
-- Lua mirror is the ev5 SetGuidData latch (instance_guid_store.go —
-- the exact Lua mirror of the C++ OnCreatureCreate entry-switch
-- latching; the illidari memberSpawn precedent). PORTED 2026-11-05:
-- the latch is verifiable from the instance script, not invented.
local function valithriaSpawn(event, creature)
    creature:SetGuidData(DATA_VALITHRIA_DREAMWALKER)
end

-- C++ npc_suppresser::IsSummonedBy (boss_valithria_dreamwalker.cpp:803):
-- resolve valithria via the instance GUID store, engage + fixate on
-- her, else despawn. ev22 fires on the summoned creature with (event,
-- creature, summoner) from the summonTempCreature fire site.
-- PORTED 2026-11-05; the SUPPRESSION arm stays out (MovementInform
-- CHASE_MOTION_TYPE-driven, no bridge).
local function suppresserIsSummonedBy(event, creature, summoner)
    local valithria = creature:GetInstanceCreature(DATA_VALITHRIA_DREAMWALKER)
    if valithria then
        creature:EngageWithTarget(valithria)
        creature:FixateTarget(valithria)
    else
        creature:DespawnOrUnsummon()
    end
end

-- C++ npc_blistering_zombie::JustDied — unconditional
-- DoCastSelf(SPELL_ACID_BURST, true) (triggered, :851-853).
local function blisteringZombieJustDied(event, creature, killer)
    creature:CastSpell(creature, SPELL_ACID_BURST, true)
end

-- C++ npc_gluttonous_abomination::JustDied —
-- DoCastSelf(SPELL_ROT_WORM_SPAWNER, true) (triggered) unless killed
-- by Valithria herself (:873-878).
local function gluttonousAbominationJustDied(event, creature, killer)
    if killer and killer:GetEntry() == ENTRY_VALITHRIA_DREAMWALKER then
        return
    end
    creature:CastSpell(creature, SPELL_ROT_WORM_SPAWNER, true)
end

RegisterCreatureEvent(ENTRY_LICH_KING_VALITHRIA, 1, lichKingJustEngagedWith)
RegisterCreatureEvent(ENTRY_VALITHRIA_DREAMWALKER, 5, valithriaSpawn)
RegisterCreatureEvent(ENTRY_VALITHRIA_DREAMWALKER, 9, valithriaDamageTaken)
RegisterCreatureEvent(ENTRY_VALITHRIA_DREAMWALKER, 11, valithriaHealReceived)
RegisterCreatureEvent(ENTRY_VALITHRIA_DREAMWALKER, 14, valithriaSpellHit)
RegisterCreatureEvent(ENTRY_VALITHRIA_DREAMWALKER, 23, valithriaReset)
RegisterCreatureEvent(ENTRY_VALITHRIA_DREAMWALKER, 38, valithriaDoAction)
RegisterCreatureEvent(ENTRY_SUPPRESSER, 22, suppresserIsSummonedBy)

-- C++ boss_valithria::JustSummoned (boss_valithria_dreamwalker.cpp:390-403):
-- each portal pre-effect summon gets a 15s ValithriaDelayedCastEvent
-- (SPELL_SUMMON_DREAM_PORTAL 71305 / NIGHTMARE 71987, despawn 6s) + a 15s
-- ValithriaAuraRemoveEvent for its pre visual. PORTED 2026-11-08: the
-- aura strip + the delayed summon as an explicit Valithria-side
-- SummonCreature (the 71305/71987 cast itself stays out — no Spell.dbc).
local function valithriaJustSummoned(_, creature, summoned)
    local visual = nil
    local portalEntry = nil
    if summoned:GetEntry() == ENTRY_DREAM_PORTAL_PRE_EFFECT then
        visual = SPELL_DREAM_PORTAL_VISUAL_PRE
        portalEntry = ENTRY_DREAM_PORTAL
    elseif summoned:GetEntry() == ENTRY_NIGHTMARE_PORTAL_PRE_EFFECT then
        visual = SPELL_NIGHTMARE_PORTAL_VISUAL_PRE
        portalEntry = ENTRY_NIGHTMARE_PORTAL
    end
    if visual then
        CreateLuaEvent(function()
            summoned:RemoveAura(visual)
        end, 15000)
    end
    if portalEntry then
        -- C++ :393-396 + :204-207 — the delayed cast summons the real
        -- portal at the pre-effect's position, then despawns the
        -- pre-effect 6s later. Twinned as an explicit summon from
        -- Valithria (the legion-flame precedent) so the portal's
        -- summoner is Valithria and ev20 fires on her; the 71305/71987
        -- cast itself stays out (no Spell.dbc, same class as the
        -- EVENT_DREAM_PORTAL pre-summon cast).
        CreateLuaEvent(function()
            creature:SummonCreature(portalEntry, summoned:GetX(), summoned:GetY(), summoned:GetZ(), summoned:GetO())
        end, 15000)
        CreateLuaEvent(function()
            summoned:DespawnOrUnsummon()
        end, 21000)
    end
end

-- C++ npc_dream_portal::OnSpellClick (boss_valithria_dreamwalker.cpp:913-930):
-- if !spellClickHandled return; _used = true; DespawnOrUnsummon(). The
-- _used latch is the portal's GetData(MISSED_PORTALS) (:924-930): 0 once
-- used, 1 otherwise — so the slot is initialized to 1 at summon (ev22,
-- the temp-summon IsSummonedBy hook; ev5 never fires for temp summons,
-- fireCreatureSpawned is respawn-only) and cleared to 0 here. Fired via
-- the Go-side event-39 OnSpellClick bridge (handleSpellClick fires after
-- the click-spell rows, Unit.cpp:13069-13070). Both portal entries share
-- this AI (boss SummonedCreatureDespawn :406-409 reads GetData(
-- MISSED_PORTALS) on both), so both register the same legs.
local function dreamPortalSummoned(event, creature, summoner)
    creature:SetData(MISSED_PORTALS, 1)
end

local function dreamPortalSpellClick(event, creature, clicker, handled)
    if not handled then
        return
    end
    creature:SetData(MISSED_PORTALS, 0)
    creature:DespawnOrUnsummon()
end

-- C++ boss_valithria_dreamwalker::SummonedCreatureDespawn
-- (:405-409): a dream/nightmare portal despawning with GetData(
-- MISSED_PORTALS) still 1 (never clicked) increments _missedPortals;
-- a used portal reads 0 and is not missed. The counter lives in
-- Valithria's own SetData slot (the boss GetData :453-455 mirror);
-- write-only in C++ too (no reader outside the boss's own GetData).
-- Fires via ev20 on the summoner — the portal's OwnerGUID is Valithria
-- (the delayed cast passes her as original caster, SpellEffects.cpp
-- :2210-2212, so the summon attributes to her).
local function valithriaSummonedCreatureDespawn(event, creature, summoned)
    local entry = summoned:GetEntry()
    if entry ~= ENTRY_DREAM_PORTAL and entry ~= ENTRY_NIGHTMARE_PORTAL then
        return
    end
    if summoned:GetData(MISSED_PORTALS) ~= 0 then
        creature:SetData(MISSED_PORTALS, creature:GetData(MISSED_PORTALS) + 1)
    end
end

RegisterCreatureEvent(ENTRY_VALITHRIA_DREAMWALKER, 19, valithriaJustSummoned)
RegisterCreatureEvent(ENTRY_VALITHRIA_DREAMWALKER, 20, valithriaSummonedCreatureDespawn)
RegisterCreatureEvent(ENTRY_DREAM_PORTAL, 22, dreamPortalSummoned)
RegisterCreatureEvent(ENTRY_DREAM_PORTAL, 39, dreamPortalSpellClick)
RegisterCreatureEvent(ENTRY_NIGHTMARE_PORTAL, 22, dreamPortalSummoned)
RegisterCreatureEvent(ENTRY_NIGHTMARE_PORTAL, 39, dreamPortalSpellClick)
RegisterCreatureEvent(ENTRY_BLISTERING_ZOMBIE, 4, blisteringZombieJustDied)
RegisterCreatureEvent(ENTRY_GLUTTONOUS_ABOMINATION, 4, gluttonousAbominationJustDied)
RegisterCreatureEvent(ENTRY_GLUTTONOUS_ABOMINATION, 1, gluttonousAbominationJustEngagedWith)
RegisterCreatureEvent(ENTRY_GLUTTONOUS_ABOMINATION, 2, gluttonousAbominationLeaveCombat)
RegisterCreatureEvent(ENTRY_GLUTTONOUS_ABOMINATION, 23, gluttonousAbominationLeaveCombat)
RegisterCreatureEvent(ENTRY_RISEN_ARCHMAGE, 1, risenArchmageJustEngagedWith)
RegisterCreatureEvent(ENTRY_RISEN_ARCHMAGE, 2, risenArchmageLeaveCombat)
RegisterCreatureEvent(ENTRY_RISEN_ARCHMAGE, 19, risenArchmageJustSummoned)
RegisterCreatureEvent(ENTRY_RISEN_ARCHMAGE, 23, risenArchmageLeaveCombat)
RegisterCreatureEvent(ENTRY_RISEN_ARCHMAGE, 38, risenArchmageDoAction)
RegisterCreatureEvent(ENTRY_GREEN_DRAGON_COMBAT_TRIGGER, 1, greenDragonCombatTriggerJustEngagedWith)
RegisterCreatureEvent(ENTRY_GREEN_DRAGON_COMBAT_TRIGGER, 2, greenDragonCombatTriggerLeaveCombat)
RegisterCreatureEvent(ENTRY_GREEN_DRAGON_COMBAT_TRIGGER, 4, greenDragonCombatTriggerJustDied)
RegisterCreatureEvent(ENTRY_GREEN_DRAGON_COMBAT_TRIGGER, 5, greenDragonCombatTriggerSpawn)
RegisterCreatureEvent(ENTRY_GREEN_DRAGON_COMBAT_TRIGGER, 23, greenDragonCombatTriggerReset)
RegisterCreatureEvent(ENTRY_GREEN_DRAGON_COMBAT_TRIGGER, 38, greenDragonCombatTriggerDoAction)
RegisterCreatureEvent(ENTRY_LICH_KING_VALITHRIA, 5, lichKingValithriaSpawn)
RegisterCreatureEvent(ENTRY_BLAZING_SKELETON, 1, blazingSkeletonJustEngagedWith)
RegisterCreatureEvent(ENTRY_BLAZING_SKELETON, 2, blazingSkeletonLeaveCombat)
RegisterCreatureEvent(ENTRY_BLAZING_SKELETON, 23, blazingSkeletonLeaveCombat)

-- C++ spell_dreamwalker_mana_void::PeriodicTick
-- (boss_valithria_dreamwalker.cpp:994-1001): the first 3 ticks have
-- amplitude 1s and the remaining ticks 500ms; ticks 2 and 4 (even
-- ticks at or below 5) PreventDefaultAction, the rest run the Go
-- default trigger cast. Returns true only on the suppressed ticks.
local function manaVoidTick(event, aura, target)
    local tick = aura:GetTickNumber()
    if tick <= 5 and tick % 2 == 0 then
        return true
    end
end

RegisterAuraEvent(SPELL_MANA_VOID_AURA, 1, manaVoidTick)
