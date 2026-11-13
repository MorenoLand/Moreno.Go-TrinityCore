-- Gluth (Naxxramas) — Lua port of
-- src/server/scripts/Northrend/Naxxramas/boss_gluth.cpp
-- (boss_gluth (BossAI, BOSS_GLUTH); spell_gluth_decimate +
-- spell_gluth_zombiechow_search (SpellScripts via SpellScriptLoader);
-- npc_zombie_chow (ScriptedAI); AddSC_boss_gluth registers all four;
-- loader decl 67 / call 262 per northrend_script_loader.cpp — the
-- NINTH Naxxramas group in AddNorthrendScripts(), immediately after
-- AddSC_boss_noth(), under the "// Naxxramas" marker; the call after
-- it is AddSC_boss_sapphiron() (decl 68 / call 263)).
-- Entry: 15932 Gluth (naxxramas.h NPC_GLUTH — kalecgos pass;
-- instance_naxxramas.cpp binds NPC_GLUTH -> GluthGUID and DATA_GLUTH
-- -> GluthGUID; the CreatureScript ScriptName binding is
-- instance-shimmed, the creature_template binding DB-side as usual).
-- Sole-source verified: whole-server-tree grep for each of the four
-- script names hits boss_gluth.cpp only (loader carries only the
-- decl/call lines); zero sql/ hits. No gluth lua existed.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in
-- Go (creature combat tick), like C++ DoMeleeAttackIfReady. C++
-- DoCast default is triggered=false (vaelastrasz convention):
-- creature:CastSpell(nil, spell) = DoCastVictim (moroes precedent),
-- creature:CastSpell(creature, spell) = DoCastSelf (phase_hunter /
-- apothecary_hanes precedent). C++ randtime -> math.random (amanitar
-- / boss_black_knight precedent).
-- Ported arms (C++-exact for all modeled arms):
-- boss_gluth: EVENT_WOUND — DoCastVictim(Mortal Wound 54378), 10s
-- init, 10s repeat (moroes precedent); the STATE_GLUTH_EATING
-- deferral arm (events.Repeat(3s)) has no state-machine bridge —
-- documented, not wired. EVENT_ENRAGE — Talk(EMOTE_ENRAGE 2) +
-- DoCastSelf(Enrage 28371), randtime(16s,22s) init, randtime(16s,22s)
-- repeat (phase_hunter self-cast precedent); the eating-state 5s
-- deferral arm has no state bridge — documented, not wired.
-- EVENT_DECIMATE — Talk(EMOTE_DECIMATE 1) + DoCastAOE(Decimate
-- 28374), randtime(110s,120s) init, re-scheduled randtime(110s,120s)
-- (C++ re-arms via ScheduleEvent, not Repeat — same interval);
-- DoCastAOE lowers to self-cast creature:CastSpell(creature, spell)
-- (boss_ai.go DoCastAOE self-cast convention — selin_fireheart /
-- mr_smite precedent; the old "no DoCastAOE bridge" claim was
-- wrong). The 20x EVENT_SEARCH_ZOMBIE_MULTI scheduling leg and the
-- eating-state 4s deferral have no summon/state bridges — documented,
-- not wired. The decimate damage rider lives in the unported
-- spell_gluth_decimate SpellScript (documented below).
-- EVENT_BERSERK — Talk(EMOTE_BERSERKER 4) + DoCastSelf(Berserk
-- 26662), 8min init, 5min repeat. JustEngagedWith schedules the four
-- ported events (BossAI::JustEngagedWith instance-bookkeeping leg has
-- no bridge — tharon_ja precedent); no aggro Talk in C++.
-- EMOTE_SPOTS_ONE (0), EMOTE_DEVOURS_ALL (3) ride unported legs
-- (DoCastAOE / summon / motion-master) — not ported (vortex
-- precedent). (EMOTE_DECIMATE (1) rides the ported decimate tick —
-- the earlier "orphaned" list was stale.) No KilledUnit Talk, no
-- JustDied Talk in C++ for gluth. All timers cancelled on 2/4/23
-- (gargolmar precedent).
-- Unmodeled (no bridges — documented, not wired):
-- Reset — _Reset + SetReactState(REACT_AGGRESSIVE) + SetSpeed(
-- MOVE_RUN, 12.0f) — the speed leg is ported via creature:SetSpeedRate
-- as the rate 12.0/7.0 (baseMoveSpeed[MOVE_RUN] = 7.0 yd/s, Unit.cpp);
-- SummonedCreatureDies
-- — summons.Despawn (single, of an already-dead zombie) is SummonList
-- housekeeping, vacuous with no Lua summon list to drain; EVENT_DECIMATE's 20x
-- EVENT_SEARCH_ZOMBIE_MULTI scheduling machine (DoCastAOE(
-- ZOMBIE_CHOW_SEARCH_MULTI 28404) per zombie) — no summon/zombie-list
-- bridge; the STATE_GLUTH_EATING 4s deferral — no state bridge;
-- EVENT_SUMMON — PORTED below (summonTick): Is25ManRaid() ->
-- SummonCreatureGroup(SUMMON_GROUP_CHOW_25MAN 2) else
-- SummonCreatureGroup(SUMMON_GROUP_CHOW_10MAN 1)
-- (boss_gluth.cpp:185-191), 15s init, 10s repeat; the :139 pre-loop
-- HasUnitState(CASTING) gate covers EVENT_SUMMON too (500ms IsCasting
-- defer ported, mal_ganis idiom). Group coords DB-side;
-- creature:SummonCreatureGroup bridged
-- 2026-11-05 (tick 238). (The old "Is25ManRaid difficulty gate"
-- wording EVENT_SEARCH_ZOMBIE_SINGLE — summons
-- list iteration + cross-AI SetData(TOBE_EATEN) + REACT_PASSIVE +
-- AttackStop + MoveCloserAndStop — no summon / cross-AI / react-state
-- / motion bridges; EVENT_KILL_ZOMBIE_SINGLE — DoCast(ZOMBIE_CHOW_
-- SEARCH_SINGLE 28239) + react-state / speed restore legs — no
-- bridges; EVENT_SEARCH_ZOMBIE_MULTI — the per-zombie DoCastAOE(
-- ZOMBIE_CHOW_SEARCH_MULTI 28404) fires only when the summons-list
-- scan finds an alive zombie within 18.0 yards (dist > 18.0 keeps
-- scanning; the AoE hits zombies within 10 yards) and is skipped while
-- STATE_GLUTH_EATING (the 20x one-shot 3s*i schedule rides the
-- EVENT_DECIMATE arm). DoCastAOE itself bridges to self-cast
-- (boss_ai.go convention) — the real blocker is the summon-list /
-- zombie-proximity conditional gating, not the cast; DoAction(
-- ACTION_DECIMATE_EVENT) — cross-AI SetData(DECIMATED) over summons
-- — no bridges; MovementInform — MoveIdle — no motion bridge;
-- the whole zombie state machine (STATE_GLUTH_EATING / zombie GUID
-- persistence) — no state bridge.
-- spell_gluth_decimate (28374/54426): HandleScriptEffect -> Decimate
-- damage 28375 via CastCustomSpell BP0 + HandleEvent -> DoAction(
-- ACTION_DECIMATE_EVENT) — PORTED below (ev5 effectIndex 0 +
-- ev4 effectIndex 2 + ev38 receiver, the "no SpellScript binding
-- bridge (razelikh precedent)" header claim is stale —
-- RegisterSpellEvent is live); the DoAction receiver body stays an
-- empty no-op twin until the zombie machine ports.
-- spell_gluth_zombiechow_search (28239/28404): AfterHit -> 5%-max-
-- health heal — PORTED below (ev3); DORMANT — both cast sites ride
-- the unported zombie-eat machine.
-- npc_zombie_chow (ScriptedAI — entries 16360/30303 appear only in a
-- C++ comment; no NPC_ constants in naxxramas.h, no instance
-- bindings — entry-unverifiable from C++ evidence — no registration):
-- ctor DoCastSelf(Infected Wound 29307) — port-pattern-ready in
-- isolation (phase_hunter self-cast precedent) but entry-blocked —
-- joins the bridgeable-but-entry-blocked queue; the DATA_GLUTH GUID
-- leg, decimated-state MovePoint/StopMoving walk-leg, to-be-eaten
-- mechanic-immune leg (IMMUNITY_MECHANIC/GRIP) — no instance / motion
-- / react-state / immunity bridges.

local ENTRY_GLUTH = 15932

local EMOTE_ENRAGE = 2
local EMOTE_DECIMATE = 1
local EMOTE_BERSERKER = 4

local SPELL_MORTAL_WOUND = 54378
local SPELL_ENRAGE = 28371
local SPELL_DECIMATE = 28374
local SPELL_DECIMATE_HEROIC = 54426 -- boss_gluth.cpp:40 (25-man twin)
local SPELL_DECIMATE_DMG = 28375
local SPELL_ZOMBIE_CHOW_SINGLE = 28239
local SPELL_ZOMBIE_CHOW_MULTI = 28404 -- boss_gluth.cpp:44
local ACTION_DECIMATE_EVENT = 2 -- boss_gluth.cpp:87
local SPELL_BERSERK = 26662

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

-- CASTING gates PORTED (500ms IsCasting defer, mal_ganis idiom):
-- boss_gluthAI::UpdateAI pre-loop :139 + in-loop :264 gates cover
-- wound/enrage/decimate/berserk; the defer re-queues without
-- consuming the repeat.
-- C++ EVENT_WOUND: DoCastVictim(Mortal Wound 54378), 10s init, 10s
-- repeat.
local function woundTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "wound", 500, function()
            woundTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_MORTAL_WOUND)
    schedule(guid, "wound", 10000, function()
        woundTick(creature, guid)
    end)
end

-- C++ EVENT_ENRAGE: Talk(EMOTE_ENRAGE 2) + DoCastSelf(Enrage 28371),
-- randtime(16s,22s) init, randtime(16s,22s) repeat.
local function enrageTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "enrage", 500, function()
            enrageTick(creature, guid)
        end)
        return
    end
    creature:Talk(EMOTE_ENRAGE)
    creature:CastSpell(creature, SPELL_ENRAGE)
    schedule(guid, "enrage", {16000, 22000}, function()
        enrageTick(creature, guid)
    end)
end

-- C++ EVENT_DECIMATE: Talk(EMOTE_DECIMATE 1) + DoCastAOE(Decimate
-- 28374) as self-cast, randtime(110s,120s) init, re-scheduled
-- randtime(110s,120s) (C++ ScheduleEvent re-arm). The 25-man twin
-- 54426 rides the Is25ManRaid branch (C++ DoCastAOE maps via
-- GetSpellIdForDifficulty, which has no Go model — boss_ai.go:12751;
-- summonTick Is25ManRaid precedent below). The eating-state
-- 4s deferral and the 20x SEARCH_ZOMBIE_MULTI legs have no bridges
-- (documented in the header); the state is never EATING without the
-- zombie machine, so the timer fires unconditionally (maiden
-- precedent).
local function decimateTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "decimate", 500, function()
            decimateTick(creature, guid)
        end)
        return
    end
    creature:Talk(EMOTE_DECIMATE)
    if creature:Is25ManRaid() then
        creature:CastSpell(creature, SPELL_DECIMATE_HEROIC)
    else
        creature:CastSpell(creature, SPELL_DECIMATE)
    end
    schedule(guid, "decimate", {110000, 120000}, function()
        decimateTick(creature, guid)
    end)
end

-- C++ EVENT_SUMMON: Is25ManRaid() -> SummonCreatureGroup(
-- SUMMON_GROUP_CHOW_25MAN 2) else SummonCreatureGroup(
-- SUMMON_GROUP_CHOW_10MAN 1) (boss_gluth.cpp:185-191), 15s init,
-- 10s repeat (events.Repeat). The :139 pre-loop HasUnitState(CASTING)
-- gate covers EVENT_SUMMON too — 500ms IsCasting defer (mal_ganis idiom).
local function summonTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "summon", 500, function()
            summonTick(creature, guid)
        end)
        return
    end
    if creature:Is25ManRaid() then
        creature:SummonCreatureGroup(2)
    else
        creature:SummonCreatureGroup(1)
    end
    schedule(guid, "summon", 10000, function()
        summonTick(creature, guid)
    end)
end

-- C++ EVENT_BERSERK: Talk(EMOTE_BERSERKER 4) + DoCastSelf(Berserk
-- 26662), 8min init, 5min repeat (hard-enrage buff refresh).
local function berserkTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "berserk", 500, function()
            berserkTick(creature, guid)
        end)
        return
    end
    creature:Talk(EMOTE_BERSERKER)
    creature:CastSpell(creature, SPELL_BERSERK)
    schedule(guid, "berserk", 300000, function()
        berserkTick(creature, guid)
    end)
end

-- C++ JustEngagedWith: schedules EVENT_WOUND (10s), EVENT_ENRAGE
-- (randtime 16s,22s), EVENT_DECIMATE (randtime 110s,120s),
-- EVENT_BERSERK (8min). No aggro Talk in C++.
local BOSS_GLUTH = 8 -- naxxramas.h:38 (NAXEncounter)
local BOSS_STATE_NOT_STARTED = 0
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_DONE = 3 -- EncounterState (InstanceScript.h:68-76)

local function gluthEnterCombat(event, creature, target)
    creature:SetBossState(BOSS_GLUTH, BOSS_STATE_IN_PROGRESS) -- BossAI::_JustEngagedWith (ScriptedCreature.cpp:537), base-first (faerlina precedent)
    creature:SetCombatPulseDelay(5) -- BossAI::JustEngagedWith arm 1 (ScriptedCreature.cpp:540)
    creature:DoZoneInCombat() -- BossAI::JustEngagedWith me-form arm (ScriptedCreature.cpp:542)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "decimate", {110000, 120000}, function()
        decimateTick(creature, guid)
    end)
    schedule(guid, "wound", 10000, function()
        woundTick(creature, guid)
    end)
    schedule(guid, "enrage", {16000, 22000}, function()
        enrageTick(creature, guid)
    end)
    schedule(guid, "berserk", 480000, function()
        berserkTick(creature, guid)
    end)
    schedule(guid, "summon", 15000, function()
        summonTick(creature, guid)
    end)
end

local function gluthLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

local function gluthDied(event, creature, killer)
    cancelTimers(creature:GetGUIDLow())
    creature:SetBossState(BOSS_GLUTH, BOSS_STATE_DONE) -- BossAI::_JustDied (ScriptedCreature.cpp:519)
end

local function gluthReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    if creature:GetBossState(BOSS_GLUTH) ~= BOSS_STATE_DONE then
        creature:SetBossState(BOSS_GLUTH, BOSS_STATE_NOT_STARTED) -- BossAI::_Reset (ScriptedCreature.cpp:510), != DONE-gated
    end
    -- C++ boss_gluthAI::Reset (boss_gluth.cpp:107-114): _Reset() +
    -- zombie GUID clear + state NORMAL + SetReactState(REACT_AGGRESSIVE)
    -- (:112) + SetSpeed 12.0f below. The react leg is a C++-exact
    -- restore: the Lua model never sets PASSIVE (the decimate
    -- zombie-eat machine is unported), so it is vacuous until that
    -- machine ports.
    creature:SetReactState(2) -- REACT_AGGRESSIVE
    -- C++ Reset() :113: SetSpeed(MOVE_RUN, 12.0f) — absolute form; the
    -- rate is 12.0 / baseMoveSpeed[MOVE_RUN] (7.0 yd/s, Unit.cpp:8816-8819).
    -- The decimate 36.0f leg stays out: it fires only inside the eating
    -- state behind the unported zombie machine (documented above).
    creature:SetSpeedRate(1, 12.0 / 7.0)
end

-- C++ boss_gluthAI::SummonedCreatureDies (boss_gluth.cpp:127-130):
-- summons.Despawn(summoned) — SummonList housekeeping, vacuous with
-- no Lua summon list to drain (summonTick fires SummonCreatureGroup
-- and forgets); dead zombies despawn via the engine death path.
local function gluthSummonedCreatureDied(event, creature, summon, killer)
end

RegisterCreatureEvent(ENTRY_GLUTH, 1, gluthEnterCombat)
RegisterCreatureEvent(ENTRY_GLUTH, 2, gluthLeaveCombat)
RegisterCreatureEvent(ENTRY_GLUTH, 4, gluthDied)
RegisterCreatureEvent(ENTRY_GLUTH, 21, gluthSummonedCreatureDied)
RegisterCreatureEvent(ENTRY_GLUTH, 23, gluthReset)

-- C++ spell_gluth_decimate::HandleScriptEffect (boss_gluth.cpp:313-326):
-- Decimate (28374 / 25-man 54426) hitting a unit deals (health - 5%
-- max health) as 28375 triggered with BP0, when positive. ev5 fires
-- once per (effect, hit target) from the creature cast path
-- (boss_ai.go:3435), effectIndex == 0 is the EFFECT_0 bind; the
-- conditions-table rows on effect 0 (boss_gluth.cpp:40) are DB-side
-- and unevaluated in Go — flagged, not a verdict-changer.
local function decimateDamageHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetEntry() ~= ENTRY_GLUTH then
        return
    end
    local damage = hitTarget:GetHealth() - hitTarget:CountPctFromMaxHealth(5)
    if damage > 0 then
        caster:CastCustomSpell(hitTarget, SPELL_DECIMATE_DMG, true, damage)
    end
end

-- C++ spell_gluth_decimate::HandleEvent (boss_gluth.cpp:328-331): the
-- SEND_EVENT effect fires DoAction(ACTION_DECIMATE_EVENT) on the boss
-- AI once per cast (ev4, boss_ai.go:3373 — once per non-zero effect,
-- not per target). The receiver (boss_gluth.cpp:267-276) sets every
-- summoned zombie DECIMATED — no summon list / cross-AI bridge yet,
-- so the receiver stays an empty no-op twin until the zombie machine
-- ports.
local function decimateEventHit(event, spell, effectIndex)
    if effectIndex ~= 2 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetEntry() ~= ENTRY_GLUTH then
        return
    end
    caster:DoAction(ACTION_DECIMATE_EVENT)
end

local function gluthDoAction(event, creature, action)
    if action ~= ACTION_DECIMATE_EVENT then
        return
    end
end

-- C++ spell_gluth_zombiechow_search::HealForEachTargetHit
-- (boss_gluth.cpp:368-371): AfterHit heals Gluth 5% max health per
-- unit hit (ev3 fires once per hit target, boss_ai.go:4215-4228).
-- 28239 single-target / 28404 multi (boss_gluth.cpp:44; census row
-- 386 is the 28404 entry). DORMANT: both cast sites ride the unported
-- zombie-eat machine (EVENT_KILL_ZOMBIE_SINGLE :231,
-- EVENT_SEARCH_ZOMBIE_MULTI :258).
local function zombieChowHeal(event, spell, hitTarget)
    local caster = spell:GetCaster()
    if caster == nil or caster:GetEntry() ~= ENTRY_GLUTH then
        return
    end
    caster:SetHealth(caster:GetHealth() + caster:CountPctFromMaxHealth(5))
end

RegisterSpellEvent(SPELL_DECIMATE, 5, decimateDamageHit)
RegisterSpellEvent(SPELL_DECIMATE_HEROIC, 5, decimateDamageHit)
RegisterSpellEvent(SPELL_DECIMATE, 4, decimateEventHit)
RegisterSpellEvent(SPELL_DECIMATE_HEROIC, 4, decimateEventHit)
RegisterSpellEvent(SPELL_ZOMBIE_CHOW_SINGLE, 3, zombieChowHeal)
RegisterSpellEvent(SPELL_ZOMBIE_CHOW_MULTI, 3, zombieChowHeal)
RegisterCreatureEvent(ENTRY_GLUTH, 38, gluthDoAction)
