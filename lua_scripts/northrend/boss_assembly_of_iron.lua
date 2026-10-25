-- Assembly of Iron (Ulduar) — Lua port of
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_assembly_of_iron.cpp
-- (boss_steelbreaker / boss_runemaster_molgeim /
-- boss_stormcaller_brundir (all CreatureScript via
-- GetUlduarAI<...AI> (BossAI), BOSS_ASSEMBLY_OF_IRON = 4 —
-- registered from inside AddSC_boss_assembly_of_iron(); loader
-- decl 116 / call 311 per northrend_script_loader.cpp — the
-- SEVENTH group of the "// Ulduar" block in
-- AddNorthrendScripts(), immediately after
-- AddSC_boss_general_vezax() (decl 117 / call 310); the call
-- after it is AddSC_boss_kologarn() (decl 115 / call 312) —
-- loader order confirmed this run; the checkpoint sequence
-- (boss_general_vezax -> boss_assembly_of_iron) is followed).
-- Entries: 32867 Steelbreaker (ulduar.h NPC_STEELBREAKER,
-- line 67); 32927 Runemaster Molgeim (ulduar.h NPC_MOLGEIM,
-- line 68); 32857 Stormcaller Brundir (ulduar.h NPC_BRUNDIR,
-- line 69) — all entry-verifiable, registration proceeds (the
-- nexus_commanders / malygos / sartharion kalecgos precedent);
-- the CreatureScript ScriptName bindings are DB-side as usual.
-- Sole-source verified: whole-server-tree grep for each of the
-- seven script names ("boss_steelbreaker",
-- "boss_runemaster_molgeim", "boss_stormcaller_brundir",
-- "spell_shield_of_runes", "spell_assembly_meltdown",
-- "spell_assembly_rune_of_summoning",
-- "achievement_assembly_i_choose_you") hits
-- boss_assembly_of_iron.cpp only; zero sql/ hits for all seven.
-- Audited 2026-10-05: lua port from the 10-02 run (a7021b3) re-verified
-- C++-exact — all 21 Talk arms accounted (14 ported / 7
-- documented-only); loader decl 116 / call 311 re-confirmed.
-- Re-audited 2026-10-13: timer/cast legs ported on the
-- CreateLuaEvent convention (berserk x3, fusion punch, shield
-- of runes, overload, steelbreaker engage high voltage) —
-- phase-gated / random-target / summon / instance / movement
-- legs stay documented-only.
-- Re-audited 2026-10-18: EVENT_CHAIN_LIGHTNING ported on the
-- GetAITargets random-target idiom (the old "random-target
-- bridge absent" claim was stale — the razuvious precedent);
-- RemoveAllAuras() added to the three event-23 Reset handlers
-- (bridge exists at engine/world/lua_creature_events.go:166);
-- the UNIT_STATE_CASTING gates stay unmodeled — IsCasting is
-- bridged-but-false in Go (engine/world/lua_objects.go:330),
-- so the gate is vacuous; every spell enum value, yell index,
-- init/re-arm delay and triggered flag re-verified C++-exact
-- name-by-name this run.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset.
-- Ported arms (C++-exact for all modeled arms):
-- boss_steelbreaker — JustEngagedWith: Talk(SAY_AGGRO 0)
-- (event 1 — the auriaya precedent); DoCast(me,
-- SPELL_HIGH_VOLTAGE 61890) (non-triggered self-cast) at
-- engage; EVENT_BERSERK (Talk(SAY_BERSERK 5) +
-- DoCast(SPELL_BERSERK 47008) non-triggered self-cast, 15min
-- init, one-shot); EVENT_FUSION_PUNCH
-- (DoCastVictim(SPELL_FUSION_PUNCH 61903) non-triggered,
-- melee-range gated, 15s init, {13s,22s} repeat) — the maiden
-- CreateLuaEvent convention (the BossAI::JustEngagedWith
-- passthrough and SetPhase legs have no bridges); KilledUnit:
-- player-gated
-- Talk(SAY_SLAY 1) (event 3; who->GetTypeId() ==
-- TYPEID_PLAYER — the razuvious player-gated variant
-- precedent — the twenty-sixth player-gated variant ported;
-- the phase == 3 DoCast SPELL_ELECTRICAL_CHARGE 61902 leg is
-- phase-gated — out per the gothik precedent); JustDied:
-- Talk(SAY_DEATH 3) (event 4; the
-- _JustDied() passthrough, the instance GetBossState DONE
-- check, the DONE branch (Talk SAY_ENCOUNTER_DEFEATED 4 +
-- DoCastAOE SPELL_KILL_CREDIT 65195), the SetLootRecipient
-- leg and the supercharge DoAction cascade to the surviving
-- bosses have no bridges — the sjonnir JustDied-Talk
-- precedent).
-- boss_runemaster_molgeim — JustEngagedWith: Talk(SAY_AGGRO
-- 0) (event 1 — the auriaya precedent); EVENT_BERSERK
-- (Talk(SAY_BERSERK 6) + DoCast(SPELL_BERSERK 47008)
-- non-triggered self-cast, 15min init, one-shot);
-- EVENT_SHIELD_OF_RUNES (DoCast(me, SPELL_SHIELD_OF_RUNES
-- 62274) non-triggered self-cast, 30s init, {27s,34s} repeat)
-- — the maiden CreateLuaEvent convention (the
-- BossAI::JustEngagedWith passthrough, SetPhase and
-- EVENT_RUNE_OF_POWER legs have no bridges — rune of power
-- rides a random-ally instance-GUID leg); KilledUnit:
-- player-gated Talk(SAY_SLAY 1) (event 3; the twenty-seventh
-- player-gated variant ported); JustDied: Talk(SAY_DEATH 4)
-- (event 4; the _JustDied() passthrough, the instance
-- GetBossState DONE check, the DONE branch (Talk
-- SAY_ENCOUNTER_DEFEATED 5 + DoCastAOE SPELL_KILL_CREDIT),
-- the SetLootRecipient leg and the supercharge DoAction
-- cascade have no bridges — the sjonnir JustDied-Talk
-- precedent).
-- boss_stormcaller_brundir — JustEngagedWith: Talk(SAY_AGGRO
-- 0) (event 1 — the auriaya precedent); EVENT_BERSERK
-- (Talk(SAY_BERSERK 6) + DoCast(SPELL_BERSERK 47008)
-- non-triggered self-cast, 15min init, one-shot);
-- EVENT_OVERLOAD (Talk(EMOTE_BRUNDIR_OVERLOAD 7) +
-- Talk(SAY_BRUNDIR_SPECIAL 2) + DoCast(SPELL_OVERLOAD 61869)
-- non-triggered self-cast, {60s,120s} init and repeat);
-- EVENT_CHAIN_LIGHTNING (DoCast(target, SPELL_CHAIN_LIGHTNING
-- 61879) non-triggered, SelectTarget(Random, 0) ->
-- randomThreatTarget via the handler GetAITargets twin — the
-- razuvious random-target idiom — 4s init, {7s,10s} repeat) —
-- the maiden CreateLuaEvent convention (the
-- BossAI::JustEngagedWith passthrough, SetPhase leg, the
-- EVENT_MOVE_POSITION motion-machine leg, and the
-- FindNearestCreature NPC_WORLD_TRIGGER 22515 trigger-GUID leg
-- have no bridges);
-- KilledUnit: player-gated Talk(SAY_SLAY 1) (event 3; the
-- twenty-eighth player-gated variant ported); JustDied:
-- Talk(SAY_DEATH 4) (event 4; the _JustDied() passthrough, the
-- instance GetBossState DONE check, the DONE branch (Talk
-- SAY_ENCOUNTER_DEFEATED 5 + DoCastAOE SPELL_KILL_CREDIT),
-- the SetLootRecipient leg and the supercharge DoAction
-- cascade have no bridges — the sjonnir JustDied-Talk
-- precedent).
-- DOCUMENTED-ONLY (in this header; no registration beyond
-- entries 32867 / 32927 / 32857):
-- Reset / Initialize (all three) — the _Reset() + phase = 0
-- legs have no bridges; RemoveAllAuras() IS bridged (the
-- handler twin, engine/world/lua_creature_events.go:166) and
-- rides the three event-23 handlers; brundir's SetHover(false)
-- is bridged (lua_creature_events.go) and rides brundirReset in
-- C++ arm order (the old "no bridge" claim was stale). The
-- interrupt/stun immunity-reset removes ride the
-- creature:ApplySpellImmune bridge but are no-op removes — nothing
-- ever adds those rows (the DoAction supercharge adds are unported),
-- so they stay out.
-- DoAction (all three) — ACTION_SUPERCHARGE 1 (SetFullHealth
-- + SPELL_SUPERCHARGE 61920 + phase++ + event reschedules)
-- and ACTION_ADD_CHARGE 2 (steelbreaker, SPELL_ELECTRICAL_
-- CHARGE trigger cast — the spell_assembly_meltdown producer)
-- — no DoAction bridge (joins the no-DoAction-bridge queue).
-- GetData(DATA_PHASE_3 1) — creature:GetData IS bridged (ScriptData
-- store, 2026-10-24) but the sole consumer is achievement-only; consumer:
-- achievement_assembly_i_choose_you below.
-- DOCUMENTED-ONLY UpdateAI legs (blockers per arm):
-- steelbreaker EVENT_STATIC_DISRUPTION
-- (SPELL_STATIC_DISRUPTION 44008, random target, phase >= 2)
-- and EVENT_OVERWHELMING_POWER (Talk(SAY_POWER 2) +
-- SPELL_OVERWHELMING_POWER 64637, phase >= 3) — phase-gated,
-- out per the gothik precedent (porting on combat-start
-- timers would fabricate phase-1 casts). molgeim
-- EVENT_RUNE_OF_POWER (SPELL_SUMMON_RUNE_OF_POWER 63513,
-- random alive ally via instance GUIDs — note the C++ case-2
-- copy-paste quirk fetching DATA_STEELBREAKER for the brundir
-- slot); EVENT_RUNE_OF_DEATH (Talk(SAY_RUNE_DEATH 2) +
-- SPELL_RUNE_OF_DEATH 62269, random target, phase >= 2);
-- EVENT_RUNE_OF_SUMMONING (Talk(SAY_SUMMON 3) +
-- SPELL_RUNE_OF_SUMMONING 62273, random target, phase >= 3).
-- brundir EVENT_LIGHTNING_WHIRL (SPELL_LIGHTNING_WHIRL
-- 61915, phase >= 2); EVENT_LIGHTNING_TENDRILS
-- (Talk(SAY_FLIGHT 3) + SPELL_LIGHTNING_TENDRILS 61887 /
-- VISUAL 61883 + hover / flight / landing state machine,
-- phase >= 3); EVENT_FLIGHT / EVENT_ENDFLIGHT / EVENT_GROUND
-- / EVENT_LAND / EVENT_MOVE_POSITION — phase-gated /
-- motion-master / hover / DoStartMovement / ResetThreatList
-- machine, no bridges. Deviation note: the
-- DoAction(ACTION_SUPERCHARGE) phase machine (phase++,
-- SetFullHealth, SPELL_SUPERCHARGE 61920, event reschedules:
-- fusion punch 15s / shield 27s / rune of power 25s /
-- chain lightning {7s,12s} / overload {40s,50s}, plus
-- phase >= 2 / 3 arm enables) has no DoAction bridge, so
-- timers keep their combat-start schedules after a
-- supercharge instead of the C++ reschedules.
-- spell_shield_of_runes (62274 — AuraScript
-- AfterEffectRemove: on non-expire removal the caster casts
-- SPELL_SHIELD_OF_RUNES_BUFF 62277 — no AuraScript bridge;
-- joins the no-AuraScript-bridge queue);
-- spell_assembly_meltdown (SPELL_EFFECT_INSTAKILL handler:
-- instance GetGuidData(DATA_STEELBREAKER) ->
-- DoAction(ACTION_ADD_CHARGE) — no SpellScript bridge; joins
-- the no-SpellScript-bridge queue);
-- spell_assembly_rune_of_summoning (62273 — periodic-dummy
-- AuraScript: casts SPELL_RUNE_OF_SUMMONING_SUMMON 62020 with
-- the summoner GUID + OnRemove despawn of the rune summon —
-- no AuraScript bridge; joins the no-AuraScript-bridge
-- queue).
-- achievement_assembly_i_choose_you (OnCheck: target GetAI()
-- -> GetData(DATA_PHASE_3)) — no GetData / achievement
-- bridges; joins the unmodeled-achievement queue.

local ENTRY_STEELBREAKER = 32867
local ENTRY_MOLGEIM = 32927
local ENTRY_BRUNDIR = 32857

local SAY_STEELBREAKER_AGGRO = 0
local SAY_STEELBREAKER_SLAY = 1
local SAY_STEELBREAKER_DEATH = 3
local SAY_STEELBREAKER_BERSERK = 5
local SAY_MOLGEIM_AGGRO = 0
local SAY_MOLGEIM_SLAY = 1
local SAY_MOLGEIM_DEATH = 4
local SAY_MOLGEIM_BERSERK = 6
local SAY_BRUNDIR_AGGRO = 0
local SAY_BRUNDIR_SLAY = 1
local SAY_BRUNDIR_SPECIAL = 2
local SAY_BRUNDIR_DEATH = 4
local SAY_BRUNDIR_BERSERK = 6
local EMOTE_BRUNDIR_OVERLOAD = 7

local SPELL_BERSERK = 47008
local SPELL_HIGH_VOLTAGE = 61890
local SPELL_FUSION_PUNCH = 61903
local SPELL_SHIELD_OF_RUNES = 62274
local SPELL_OVERLOAD = 61869
local SPELL_CHAIN_LIGHTNING = 61879

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

-- C++ EVENT_FUSION_PUNCH: if IsWithinMeleeRange(victim)
-- DoCastVictim(SPELL_FUSION_PUNCH 61903) (non-triggered),
-- 15s init, {13s,22s} repeat; the cast is skipped when the
-- victim is out of melee range but the timer still re-arms.
local function fusionPunchTick(creature, guid)
    local victim = creature:GetVictim()
    if victim and creature:IsWithinMeleeRange(victim) then
        creature:CastSpell(victim, SPELL_FUSION_PUNCH)
    end
    schedule(guid, "fusionpunch", {13000, 22000}, function()
        fusionPunchTick(creature, guid)
    end)
end

-- C++ EVENT_BERSERK: Talk(SAY_BERSERK 5) +
-- DoCast(SPELL_BERSERK 47008) (non-triggered self-cast),
-- 15min init; the event is cancelled after firing.
local function steelbreakerBerserkTick(creature, guid)
    creature:Talk(SAY_STEELBREAKER_BERSERK)
    creature:CastSpell(creature, SPELL_BERSERK)
end

-- C++ boss_steelbreaker::JustEngagedWith: BossAI passthrough
-- + DoCast(me, SPELL_HIGH_VOLTAGE 61890) (non-triggered
-- self-cast) + SetPhase + ScheduleEvent(EVENT_BERSERK, 15min)
-- + ScheduleEvent(EVENT_FUSION_PUNCH, 15s) + Talk(SAY_AGGRO).
local function steelbreakerEnterCombat(event, creature, target)
    creature:Talk(SAY_STEELBREAKER_AGGRO)
    creature:CastSpell(creature, SPELL_HIGH_VOLTAGE)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "berserk", 900000, function()
        steelbreakerBerserkTick(creature, guid)
    end)
    schedule(guid, "fusionpunch", 15000, function()
        fusionPunchTick(creature, guid)
    end)
end

local function steelbreakerLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ Reset: _Reset() + phase = 0 + me->RemoveAllAuras() — the
-- _Reset()/phase legs have no bridges; RemoveAllAuras rides the
-- handler twin (engine/world/lua_creature_events.go:166, the
-- evade-leg twin).
local function steelbreakerReset(event, creature)
    cancelTimers(creature:GetGUID())
    creature:RemoveAllAuras()
end

-- C++ KilledUnit: who->GetTypeId() == TYPEID_PLAYER, then Talk
-- (SAY_SLAY) — the razuvious player-gated variant precedent;
-- the phase == 3 SPELL_ELECTRICAL_CHARGE leg has no bridge.
local function steelbreakerTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_STEELBREAKER_SLAY)
    end
end

-- C++ JustDied: _JustDied() passthrough + instance
-- GetBossState DONE check + DONE branch (Talk
-- SAY_ENCOUNTER_DEFEATED + DoCastAOE SPELL_KILL_CREDIT) +
-- SetLootRecipient + supercharge DoAction cascade — only the
-- per-boss death Talk arm is bridgeable.
local function steelbreakerDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(SAY_STEELBREAKER_DEATH)
end

-- C++ EVENT_SHIELD_OF_RUNES: DoCast(me,
-- SPELL_SHIELD_OF_RUNES 62274) (non-triggered self-cast),
-- 30s init, {27s,34s} repeat.
local function shieldOfRunesTick(creature, guid)
    creature:CastSpell(creature, SPELL_SHIELD_OF_RUNES)
    schedule(guid, "shieldofrunes", {27000, 34000}, function()
        shieldOfRunesTick(creature, guid)
    end)
end

-- C++ EVENT_BERSERK: Talk(SAY_BERSERK 6) +
-- DoCast(SPELL_BERSERK 47008) (non-triggered self-cast),
-- 15min init; the event is cancelled after firing.
local function molgeimBerserkTick(creature, guid)
    creature:Talk(SAY_MOLGEIM_BERSERK)
    creature:CastSpell(creature, SPELL_BERSERK)
end

-- C++ boss_runemaster_molgeim::JustEngagedWith: BossAI
-- passthrough + SetPhase + ScheduleEvent(EVENT_BERSERK,
-- 15min) + ScheduleEvent(EVENT_SHIELD_OF_RUNES, 30s) +
-- ScheduleEvent(EVENT_RUNE_OF_POWER, 20s) + Talk(SAY_AGGRO).
local function molgeimEnterCombat(event, creature, target)
    creature:Talk(SAY_MOLGEIM_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "berserk", 900000, function()
        molgeimBerserkTick(creature, guid)
    end)
    schedule(guid, "shieldofrunes", 30000, function()
        shieldOfRunesTick(creature, guid)
    end)
end

local function molgeimLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ Reset: _Reset() + phase = 0 + me->RemoveAllAuras() — the
-- _Reset()/phase legs have no bridges; RemoveAllAuras rides the
-- handler twin (engine/world/lua_creature_events.go:166, the
-- evade-leg twin).
local function molgeimReset(event, creature)
    cancelTimers(creature:GetGUID())
    creature:RemoveAllAuras()
end

-- C++ KilledUnit: who->GetTypeId() == TYPEID_PLAYER, then Talk
-- (SAY_SLAY) — the razuvious player-gated variant precedent.
local function molgeimTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_MOLGEIM_SLAY)
    end
end

-- C++ JustDied: _JustDied() passthrough + instance
-- GetBossState DONE check + DONE branch (Talk
-- SAY_ENCOUNTER_DEFEATED + DoCastAOE SPELL_KILL_CREDIT) +
-- SetLootRecipient + supercharge DoAction cascade — only the
-- per-boss death Talk arm is bridgeable.
local function molgeimDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(SAY_MOLGEIM_DEATH)
end

-- C++ SelectTarget(Random, 0) leg: alive units on the threat
-- list (C++ playerOnly default = false), no distance filter —
-- the GetAITargets pool (the razuvious random-target idiom;
-- the old "no random-target SelectTarget bridge" claim was
-- stale).
local function randomThreatTarget(creature)
    local candidates = {}
    for _, unit in ipairs(creature:GetAITargets()) do
        if not unit:IsDead() then
            candidates[#candidates + 1] = unit
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EVENT_CHAIN_LIGHTNING: if (Unit* target =
-- SelectTarget(Random, 0)) DoCast(target, SPELL_CHAIN_LIGHTNING
-- 61879) (non-triggered), 4s init, {7s,10s} repeat; the re-arm
-- sits outside the target check — C++-exact.
local function chainLightningTick(creature, guid)
    local target = randomThreatTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_CHAIN_LIGHTNING)
    end
    schedule(guid, "chainlightning", {7000, 10000}, function()
        chainLightningTick(creature, guid)
    end)
end

-- C++ EVENT_OVERLOAD: Talk(EMOTE_BRUNDIR_OVERLOAD 7) +
-- Talk(SAY_BRUNDIR_SPECIAL 2) + DoCast(SPELL_OVERLOAD 61869)
-- (non-triggered self-cast), {60s,120s} init and repeat.
local function overloadTick(creature, guid)
    creature:Talk(EMOTE_BRUNDIR_OVERLOAD)
    creature:Talk(SAY_BRUNDIR_SPECIAL)
    creature:CastSpell(creature, SPELL_OVERLOAD)
    schedule(guid, "overload", {60000, 120000}, function()
        overloadTick(creature, guid)
    end)
end

-- C++ EVENT_BERSERK: Talk(SAY_BERSERK 6) +
-- DoCast(SPELL_BERSERK 47008) (non-triggered self-cast),
-- 15min init; the event is cancelled after firing.
local function brundirBerserkTick(creature, guid)
    creature:Talk(SAY_BRUNDIR_BERSERK)
    creature:CastSpell(creature, SPELL_BERSERK)
end

-- C++ boss_stormcaller_brundir::JustEngagedWith: BossAI
-- passthrough + SetPhase + ScheduleEvent(EVENT_MOVE_POSITION,
-- 1s) + ScheduleEvent(EVENT_BERSERK, 15min) +
-- ScheduleEvent(EVENT_CHAIN_LIGHTNING, 4s) +
-- ScheduleEvent(EVENT_OVERLOAD, 60s, 120s) +
-- FindNearestCreature(NPC_WORLD_TRIGGER 22515) leg +
-- Talk(SAY_AGGRO).
local function brundirEnterCombat(event, creature, target)
    creature:Talk(SAY_BRUNDIR_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "berserk", 900000, function()
        brundirBerserkTick(creature, guid)
    end)
    schedule(guid, "overload", {60000, 120000}, function()
        overloadTick(creature, guid)
    end)
    schedule(guid, "chainlightning", 4000, function()
        chainLightningTick(creature, guid)
    end)
end

local function brundirLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ Reset: _Reset() + phase = 0 + me->RemoveAllAuras() +
-- me->SetHover(false) (:476) — the _Reset()/phase legs have no
-- bridges; RemoveAllAuras rides the handler twin
-- (engine/world/lua_creature_events.go:166, the evade-leg twin),
-- and SetHover(false) follows in C++ arm order.
local function brundirReset(event, creature)
    cancelTimers(creature:GetGUID())
    creature:RemoveAllAuras()
    creature:SetHover(false)
end

-- C++ KilledUnit: who->GetTypeId() == TYPEID_PLAYER, then Talk
-- (SAY_SLAY) — the razuvious player-gated variant precedent.
local function brundirTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_BRUNDIR_SLAY)
    end
end

-- C++ JustDied: _JustDied() passthrough + instance
-- GetBossState DONE check + DONE branch (Talk
-- SAY_ENCOUNTER_DEFEATED + DoCastAOE SPELL_KILL_CREDIT) +
-- SetLootRecipient + supercharge DoAction cascade — only the
-- per-boss death Talk arm is bridgeable.
local function brundirDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(SAY_BRUNDIR_DEATH)
end

RegisterCreatureEvent(ENTRY_STEELBREAKER, 1, steelbreakerEnterCombat)
RegisterCreatureEvent(ENTRY_STEELBREAKER, 2, steelbreakerLeaveCombat)
RegisterCreatureEvent(ENTRY_STEELBREAKER, 3, steelbreakerTargetDied)
RegisterCreatureEvent(ENTRY_STEELBREAKER, 4, steelbreakerDied)
RegisterCreatureEvent(ENTRY_STEELBREAKER, 23, steelbreakerReset)
RegisterCreatureEvent(ENTRY_MOLGEIM, 1, molgeimEnterCombat)
RegisterCreatureEvent(ENTRY_MOLGEIM, 2, molgeimLeaveCombat)
RegisterCreatureEvent(ENTRY_MOLGEIM, 3, molgeimTargetDied)
RegisterCreatureEvent(ENTRY_MOLGEIM, 4, molgeimDied)
RegisterCreatureEvent(ENTRY_MOLGEIM, 23, molgeimReset)
RegisterCreatureEvent(ENTRY_BRUNDIR, 1, brundirEnterCombat)
RegisterCreatureEvent(ENTRY_BRUNDIR, 2, brundirLeaveCombat)
RegisterCreatureEvent(ENTRY_BRUNDIR, 3, brundirTargetDied)
RegisterCreatureEvent(ENTRY_BRUNDIR, 4, brundirDied)
RegisterCreatureEvent(ENTRY_BRUNDIR, 23, brundirReset)
