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
-- the UNIT_STATE_CASTING gates PORTED as a 500ms IsCasting
-- defer in all 7 ported ticks (steelbreaker pre-loop :241 +
-- in-loop :270, molgeim :386/:438, brundir :565/:653;
-- mal_ganis idiom, IsCasting live lua_creature_events.go:1950
-- — the stale "bridged-but-false" claim is retracted); every
-- spell enum value, yell index,
-- init/re-arm delay and triggered flag re-verified C++-exact
-- name-by-name this run.
-- Re-audited 2026-11-05 (tick 258): the SetBossState legs ported on
-- the maiden convention (DATA_ASSEMBLY_OF_IRON = 4, ulduar.h:40):
-- engage -> IN_PROGRESS (1) x3, reset -> NOT_STARTED (0) x3
-- (GetBossState-gated, four-horsemen precedent), died -> DONE (3)
-- gated on the last death (assemblyBossesAlive grid sweep
-- reproduces the InstanceScript::SetBossState DONE minion guard,
-- InstanceScript.cpp:362-366 — Go's setInstanceBossState has no
-- minion registry, so an unconditional DONE write would land DONE
-- on the first death). The DONE branch (DoCastAOE SPELL_KILL_CREDIT
-- 65195 triggered + Talk SAY_ENCOUNTER_DEFEATED 4/5/5) and the
-- else-branch SetLootRecipient(nullptr) are ported with it — the
-- old "no instance bridge" claims are retracted (SetBossState /
-- GetBossState live at lua_creature_events.go:5917, SetLootRecipient
-- at lua_creature_events.go:3932). The CheckRequiredBosses /
-- EnterEvadeMode gate in _JustEngagedWith stays out (no bridge).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset.
-- Ported arms (C++-exact for all modeled arms):
-- boss_steelbreaker — JustEngagedWith: SetBossState
-- (DATA_ASSEMBLY_OF_IRON, IN_PROGRESS) + Talk(SAY_AGGRO 0)
-- (event 1 — the auriaya precedent); DoCast(me,
-- SPELL_HIGH_VOLTAGE 61890) (non-triggered self-cast) at
-- engage; EVENT_BERSERK (Talk(SAY_BERSERK 5) +
-- DoCast(SPELL_BERSERK 47008) non-triggered self-cast, 15min
-- init, one-shot); EVENT_FUSION_PUNCH
-- (DoCastVictim(SPELL_FUSION_PUNCH 61903) non-triggered,
-- melee-range gated, 15s init, {13s,22s} repeat) — the maiden
-- CreateLuaEvent convention (the SetPhase leg has no bridge;
-- the CheckRequiredBosses / EnterEvadeMode gate in
-- BossAI::JustEngagedWith has no bridge); KilledUnit:
-- player-gated
-- Talk(SAY_SLAY 1) (event 3; who->GetTypeId() ==
-- TYPEID_PLAYER — the razuvious player-gated variant
-- precedent — the twenty-sixth player-gated variant ported;
-- the phase == 3 DoCast SPELL_ELECTRICAL_CHARGE 61902 leg is
-- phase-gated — out per the gothik precedent); JustDied:
-- _JustDied() SetBossState(DONE) leg (last-death gated via
-- assemblyBossesAlive) + GetBossState DONE branch (DoCastAOE
-- SPELL_KILL_CREDIT 65195 triggered + Talk
-- SAY_ENCOUNTER_DEFEATED 4) else SetLootRecipient(nullptr) +
-- Talk(SAY_DEATH 3) (event 4) + the supercharge DoAction
-- cascade to the surviving bosses (grid-sweep relay
-- superchargeSurvivors).
-- boss_runemaster_molgeim — JustEngagedWith: SetBossState
-- (DATA_ASSEMBLY_OF_IRON, IN_PROGRESS) + Talk(SAY_AGGRO
-- 0) (event 1 — the auriaya precedent); EVENT_BERSERK
-- (Talk(SAY_BERSERK 6) + DoCast(SPELL_BERSERK 47008)
-- non-triggered self-cast, 15min init, one-shot);
-- EVENT_SHIELD_OF_RUNES (DoCast(me, SPELL_SHIELD_OF_RUNES
-- 62274) non-triggered self-cast, 30s init, {27s,34s} repeat)
-- — the maiden CreateLuaEvent convention (the SetPhase and
-- EVENT_RUNE_OF_POWER legs have no bridges — rune of power
-- rides a random-ally instance-GUID leg; the
-- CheckRequiredBosses / EnterEvadeMode gate in
-- BossAI::JustEngagedWith has no bridge); KilledUnit:
-- player-gated Talk(SAY_SLAY 1) (event 3; the twenty-seventh
-- player-gated variant ported); JustDied: _JustDied()
-- SetBossState(DONE) leg (last-death gated via
-- assemblyBossesAlive) + GetBossState DONE branch (DoCastAOE
-- SPELL_KILL_CREDIT 65195 triggered + Talk
-- SAY_ENCOUNTER_DEFEATED 5) else SetLootRecipient(nullptr) +
-- Talk(SAY_DEATH 4) (event 4) + the supercharge DoAction
-- cascade (grid-sweep relay superchargeSurvivors).
-- boss_stormcaller_brundir — JustEngagedWith: SetBossState
-- (DATA_ASSEMBLY_OF_IRON, IN_PROGRESS) + Talk(SAY_AGGRO
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
-- the maiden CreateLuaEvent convention (the SetPhase leg, the
-- EVENT_MOVE_POSITION motion-machine leg, and the
-- FindNearestCreature NPC_WORLD_TRIGGER 22515 trigger-GUID leg
-- have no bridges; the CheckRequiredBosses / EnterEvadeMode
-- gate in BossAI::JustEngagedWith has no bridge);
-- KilledUnit: player-gated Talk(SAY_SLAY 1) (event 3; the
-- twenty-eighth player-gated variant ported); JustDied:
-- _JustDied() SetBossState(DONE) leg (last-death gated via
-- assemblyBossesAlive) + GetBossState DONE branch (DoCastAOE
-- SPELL_KILL_CREDIT 65195 triggered + Talk
-- SAY_ENCOUNTER_DEFEATED 5) else SetLootRecipient(nullptr) +
-- Talk(SAY_DEATH 4) (event 4) + the supercharge DoAction
-- cascade (grid-sweep relay superchargeSurvivors).
-- DOCUMENTED-ONLY (in this header; no registration beyond
-- entries 32867 / 32927 / 32857):
-- Reset / Initialize (all three) — the _Reset()
-- SetBossState(NOT_STARTED) leg is PORTED this run
-- (GetBossState-gated, four-horsemen precedent; see the ev23
-- handlers); the phase = 0 leg has no bridge; RemoveAllAuras()
-- IS bridged (the handler twin,
-- engine/world/lua_creature_events.go:166) and rides the three
-- event-23 handlers; brundir's SetHover(false) is bridged
-- (lua_creature_events.go) and rides brundirReset in C++ arm
-- order (the old "no bridge" claim was stale). The
-- interrupt/stun immunity-reset removes ride the
-- creature:ApplySpellImmune bridge but are no-op removes — nothing
-- ever adds those rows (the DoAction supercharge adds are unported),
-- so they stay out.
-- DoAction (all three) — ACTION_SUPERCHARGE 1: the JustDied
-- relay to survivors is ported via superchargeSurvivors
-- (SetFullHealth + SPELL_SUPERCHARGE 61920 + live-timer
-- reschedules; phase++ and phase >= 2/3 reschedules stay out
-- per the deviation note below). ACTION_ADD_CHARGE 2
-- (steelbreaker, SPELL_ELECTRICAL_CHARGE trigger cast — the
-- spell_assembly_meltdown producer) stays out: its sender is
-- a SpellScript, no bridge (joins the unported-DoAction-sender queue).
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
-- phase >= 2 / 3 arm enables) — dispatch IS bridged (ev38) and the
-- JustDied sender is already ported as superchargeSurvivors, so
-- the relay is ported as grid-sweep effects at the Died
-- handlers: live-timer reschedules ride superchargeSurvivors
-- while phase++ and the phase >= 2/3 arm enables stay out (the
-- phase-gated arms they would enable are themselves unported,
-- gothik precedent).
-- spell_shield_of_runes (62274 — AuraScript
-- AfterEffectRemove: on non-expire removal the caster casts
-- SPELL_SHIELD_OF_RUNES_BUFF 62277 — PORTED at the tail (tick 424,
-- RegisterAuraEvent OnRemove, mode gate via aura:GetRemoveMode));
-- spell_assembly_meltdown (SPELL_EFFECT_INSTAKILL handler:
-- instance GetGuidData(DATA_STEELBREAKER) ->
-- DoAction(ACTION_ADD_CHARGE) — no SpellScript bridge; joins
-- the no-SpellScript-bridge queue);
-- spell_assembly_rune_of_summoning (62273 — periodic-dummy
-- AuraScript: casts SPELL_RUNE_OF_SUMMONING_SUMMON 62020 with
-- the summoner GUID + OnRemove despawn of the rune summon —
-- the OnRemove leg is ported at the tail (tick 198,
-- RegisterAuraEvent OnRemove); the periodic leg stays out —
-- no GetSummonerGUID bridge).
-- achievement_assembly_i_choose_you (OnCheck: target GetAI()
-- -> GetData(DATA_PHASE_3)) — PORTED 2026-11-08 (tick 417): the
-- phase write leg rides the Lua twin above (ev1 phase 1, ev23
-- phase 0 + SetData 0, superchargeSurvivors SetData 1 at phase 3);
-- Go case in engine/world/achievement_criteria_scripts.go reads
-- the killed boss's ScriptData slot 1 (fail-closed: absent reads 0).

local ENTRY_STEELBREAKER = 32867
local ENTRY_MOLGEIM = 32927
local ENTRY_BRUNDIR = 32857

-- DATA_ASSEMBLY_OF_IRON = 4 (ulduar.h:40); all three AIs share this
-- bossId (BossAI(creature, BOSS_ASSEMBLY_OF_IRON)).
local DATA_ASSEMBLY_OF_IRON = 4

local SAY_STEELBREAKER_AGGRO = 0
local SAY_STEELBREAKER_SLAY = 1
local SAY_STEELBREAKER_DEATH = 3
local SAY_STEELBREAKER_ENCOUNTER_DEFEATED = 4
local SAY_STEELBREAKER_BERSERK = 5
local SAY_MOLGEIM_AGGRO = 0
local SAY_MOLGEIM_SLAY = 1
local SAY_MOLGEIM_DEATH = 4
local SAY_MOLGEIM_ENCOUNTER_DEFEATED = 5
local SAY_MOLGEIM_BERSERK = 6
local SAY_BRUNDIR_AGGRO = 0
local SAY_BRUNDIR_SLAY = 1
local SAY_BRUNDIR_SPECIAL = 2
local SAY_BRUNDIR_DEATH = 4
local SAY_BRUNDIR_ENCOUNTER_DEFEATED = 5
local SAY_BRUNDIR_BERSERK = 6
local EMOTE_BRUNDIR_OVERLOAD = 7

local SPELL_BERSERK = 47008
local SPELL_KILL_CREDIT = 65195
local SPELL_HIGH_VOLTAGE = 61890
local SPELL_FUSION_PUNCH = 61903
local SPELL_SHIELD_OF_RUNES = 62274
local SPELL_OVERLOAD = 61869
local SPELL_CHAIN_LIGHTNING = 61879
local SPELL_SUPERCHARGE = 61920

-- DATA_PHASE_3 = 1 (boss_assembly_of_iron.cpp:133, Misc enum).
-- assemblyPhase is the GUID-keyed twin of the C++ member `phase`
-- (one per boss AI): Initialize/Reset 0, JustEngagedWith ++ to 1
-- (events.SetPhase(++phase), :167/:314/:493), each supercharge ++
-- (:187/:336/:511). GetData(DATA_PHASE_3) returns 1 iff phase >= 3
-- (:172-177/:320-326/:481-486) — reached only by the last survivor
-- (engage 1 + two supercharges). The store write lands in
-- superchargeSurvivors below; the Go gate reads it off the killed
-- boss (achievement_assembly_i_choose_you).
local DATA_PHASE_3 = 1
local assemblyPhase = {}

local timers = {}

local fusionPunchTick, shieldOfRunesTick, chainLightningTick, overloadTick

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

-- C++ InstanceScript::SetBossState(DONE) guard
-- (InstanceScript.cpp:362-366): a DONE write is BLOCKED while any
-- world-boss minion of the boss is still alive — all three assembly
-- bosses are minions of BOSS_ASSEMBLY_OF_IRON
-- (instance_ulduar.cpp:79-81). BossAI::_JustDied()
-- (ScriptedCreature.cpp:513-521) therefore leaves the state at
-- IN_PROGRESS on the first two deaths and only lands DONE on the
-- third. Go's setInstanceBossState has no minion registry and would
-- land DONE on the first death, so the gate is reproduced here with
-- the in-file grid sweep: no alive assembly boss other than the
-- dying one (self-GUID excluded) means this is the last death.
-- Range 200 covers the room (CircleBoundary radius 90,
-- instance_ulduar.cpp:39 — max separation under 180).
local function assemblyBossesAlive(creature)
    local selfGuid = creature:GetGUID()
    for _, entry in ipairs({ENTRY_STEELBREAKER, ENTRY_MOLGEIM, ENTRY_BRUNDIR}) do
        for _, other in ipairs(creature:GetCreaturesInGrid(entry, 200)) do
            if other:GetGUID() ~= selfGuid and other:IsAlive() then
                return true
            end
        end
    end
    return false
end

-- C++ JustDied relays (boss_assembly_of_iron.cpp:217/221,
-- 365/369, 544/548): the dying member's AI calls
-- ACTION_SUPERCHARGE on the two survivors via instance GUIDs.
-- Lua has no instance-GUID lookup, so survivors are found by
-- entry sweep in grid range (the moroes despawnAdds precedent).
-- Ported per-survivor arm: SetFullHealth + AddAura(SPELL_
-- SUPERCHARGE) + the reschedules of timers Lua actually runs
-- (fusion punch 15s / shield of runes 27s / chain lightning
-- {7s,12s} / overload {40s,50s}). The phase++ and the
-- phase >= 2/3 reschedules (static disruption, overwhelming
-- power, rune of power/death/summoning, lightning whirl/
-- tendrils, stormshield, stun immunity) stay out — those
-- events are unported phase-gated legs. The instance DONE
-- gate is subsumed by the gates below: on the final death
-- every same-entry creature in range is the dying member
-- (GUID-excluded) or already dead (IsAlive-excluded), so the
-- sweep no-ops, matching the DONE branch's lack of cascade.
local function superchargeSurvivors(creature)
    local rearms = {
        [ENTRY_STEELBREAKER] = function(add, guid)
            schedule(guid, "fusionpunch", 15000, function()
                fusionPunchTick(add, guid)
            end)
        end,
        [ENTRY_MOLGEIM] = function(add, guid)
            schedule(guid, "shieldofrunes", 27000, function()
                shieldOfRunesTick(add, guid)
            end)
        end,
        [ENTRY_BRUNDIR] = function(add, guid)
            schedule(guid, "chainlightning", {7000, 12000}, function()
                chainLightningTick(add, guid)
            end)
            schedule(guid, "overload", {40000, 50000}, function()
                overloadTick(add, guid)
            end)
        end,
    }
    local selfGuid = creature:GetGUID()
    for _, entry in ipairs({ENTRY_STEELBREAKER, ENTRY_MOLGEIM, ENTRY_BRUNDIR}) do
        for _, add in ipairs(creature:GetCreaturesInGrid(entry, 100)) do
            if add:GetGUID() ~= selfGuid and add:IsAlive() then
                add:SetHealth(add:GetMaxHealth())
                add:AddAura(SPELL_SUPERCHARGE)
                -- C++ DoAction(ACTION_SUPERCHARGE): events.SetPhase
                -- (++phase) (boss_assembly_of_iron.cpp:187/:336/:511).
                -- The `or 1` fallback is the post-engage phase (each
                -- boss's ev1 sets 1 below); at phase 3 the survivor is
                -- the achievement target (GetData(DATA_PHASE_3) == 1).
                local phase = (assemblyPhase[add:GetGUID()] or 1) + 1
                assemblyPhase[add:GetGUID()] = phase
                if phase >= 3 then
                    add:SetData(DATA_PHASE_3, 1)
                end
                local rearm = rearms[entry]
                if rearm then
                    rearm(add, add:GetGUID())
                end
            end
        end
    end
end

-- C++ EVENT_FUSION_PUNCH: if IsWithinMeleeRange(victim)
-- DoCastVictim(SPELL_FUSION_PUNCH 61903) (non-triggered),
-- 15s init, {13s,22s} repeat; the cast is skipped when the
-- victim is out of melee range but the timer still re-arms.
fusionPunchTick = function(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "fusionpunch", 500, function()
            fusionPunchTick(creature, guid)
        end)
        return
    end
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
    if creature:IsCasting() then
        schedule(guid, "berserk", 500, function()
            steelbreakerBerserkTick(creature, guid)
        end)
        return
    end
    creature:Talk(SAY_STEELBREAKER_BERSERK)
    creature:CastSpell(creature, SPELL_BERSERK)
end

-- C++ boss_steelbreaker::JustEngagedWith: BossAI passthrough
-- (SetBossState(DATA_ASSEMBLY_OF_IRON, IN_PROGRESS) first —
-- ScriptedCreature.cpp:527-538; the CheckRequiredBosses /
-- EnterEvadeMode gate has no bridge — stay-out) + Talk(SAY_AGGRO)
-- + DoCast(me, SPELL_HIGH_VOLTAGE 61890) (non-triggered
-- self-cast) + SetPhase + ScheduleEvent(EVENT_BERSERK, 15min)
-- + ScheduleEvent(EVENT_FUSION_PUNCH, 15s).
local function steelbreakerEnterCombat(event, creature, target)
    creature:SetBossState(DATA_ASSEMBLY_OF_IRON, 1) -- BossAI::JustEngagedWith arm 1 (ScriptedCreature.cpp:537) -- base-first (boss_assembly_of_iron.cpp:162)
    creature:SetCombatPulseDelay(5) -- BossAI::JustEngagedWith arm 1 (ScriptedCreature.cpp:540) -- base-first (boss_assembly_of_iron.cpp:164)
    creature:DoZoneInCombat() -- BossAI::JustEngagedWith me-form arm (ScriptedCreature.cpp:542)
    creature:Talk(SAY_STEELBREAKER_AGGRO)
    creature:CastSpell(creature, SPELL_HIGH_VOLTAGE)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    assemblyPhase[guid] = 1 -- C++ JustEngagedWith: events.SetPhase(++phase), phase 0 -> 1 (boss_assembly_of_iron.cpp:167)
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

-- C++ Reset (:155-160): _Reset() + phase = 0 + me->RemoveAllAuras()
-- — the _Reset() SetBossState(NOT_STARTED) leg is PORTED
-- (GetBossState-gated: _Reset writes NOT_STARTED only if the state
-- != DONE, ScriptedCreature.cpp:509-510 — the four-horsemen
-- precedent; Go's setInstanceBossState also blocks DONE moving
-- backwards); the phase leg has no bridge; RemoveAllAuras rides
-- the handler twin (engine/world/lua_creature_events.go:166, the
-- evade-leg twin).
local function steelbreakerReset(event, creature)
    cancelTimers(creature:GetGUID())
    if creature:GetBossState(DATA_ASSEMBLY_OF_IRON) ~= 3 then
        creature:SetBossState(DATA_ASSEMBLY_OF_IRON, 0)
    end
    creature:RemoveAllAuras()
    -- C++ Reset -> Initialize: phase = 0 (boss_assembly_of_iron.cpp:155-160).
    assemblyPhase[creature:GetGUID()] = 0
    -- Alive-gated: ev23 fires on death too (fireCreatureDied runs
    -- Reset before the kill-credit criteria read, kill.go:1091/:1400),
    -- and a death-time zero would clobber the fight's phase before
    -- the achievement gate reads it; engage re-sets it.
    if creature:IsAlive() then
        creature:SetData(DATA_PHASE_3, 0)
    end
end

-- C++ KilledUnit: who->GetTypeId() == TYPEID_PLAYER, then Talk
-- (SAY_SLAY) — the razuvious player-gated variant precedent;
-- the phase == 3 SPELL_ELECTRICAL_CHARGE leg has no bridge.
local function steelbreakerTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_STEELBREAKER_SLAY)
    end
end

-- C++ JustDied (:200-223): _JustDied() first — the SetBossState
-- (DONE) leg lands only on the last death (InstanceScript minion
-- guard, reproduced by assemblyBossesAlive) — then the
-- GetBossState DONE check branches: DONE -> DoCastAOE
-- (SPELL_KILL_CREDIT 65195, triggered) + Talk
-- (SAY_ENCOUNTER_DEFEATED 4); else SetLootRecipient(nullptr) +
-- Talk(SAY_DEATH 3) + supercharge DoAction cascade to survivors
-- (grid-sweep relay superchargeSurvivors). SetLootRecipient bridge
-- live (lua_creature_events.go:3932); GetBossState bridge live
-- (lua_creature_events.go).
local function steelbreakerDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    if not assemblyBossesAlive(creature) then
        creature:SetBossState(DATA_ASSEMBLY_OF_IRON, 3)
    end
    if creature:GetBossState(DATA_ASSEMBLY_OF_IRON) == 3 then
        creature:CastSpell(creature, SPELL_KILL_CREDIT, true)
        creature:Talk(SAY_STEELBREAKER_ENCOUNTER_DEFEATED)
    else
        creature:SetLootRecipient(nil)
        creature:Talk(SAY_STEELBREAKER_DEATH)
        superchargeSurvivors(creature)
    end
end

-- C++ EVENT_SHIELD_OF_RUNES: DoCast(me,
-- SPELL_SHIELD_OF_RUNES 62274) (non-triggered self-cast),
-- 30s init, {27s,34s} repeat.
shieldOfRunesTick = function(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "shieldofrunes", 500, function()
            shieldOfRunesTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_SHIELD_OF_RUNES)
    schedule(guid, "shieldofrunes", {27000, 34000}, function()
        shieldOfRunesTick(creature, guid)
    end)
end

-- C++ EVENT_BERSERK: Talk(SAY_BERSERK 6) +
-- DoCast(SPELL_BERSERK 47008) (non-triggered self-cast),
-- 15min init; the event is cancelled after firing.
local function molgeimBerserkTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "berserk", 500, function()
            molgeimBerserkTick(creature, guid)
        end)
        return
    end
    creature:Talk(SAY_MOLGEIM_BERSERK)
    creature:CastSpell(creature, SPELL_BERSERK)
end

-- C++ boss_runemaster_molgeim::JustEngagedWith: BossAI
-- passthrough (SetBossState(DATA_ASSEMBLY_OF_IRON, IN_PROGRESS)
-- first — ScriptedCreature.cpp:527-538; the CheckRequiredBosses /
-- EnterEvadeMode gate has no bridge — stay-out) + Talk(SAY_AGGRO)
-- + SetPhase + ScheduleEvent(EVENT_BERSERK,
-- 15min) + ScheduleEvent(EVENT_SHIELD_OF_RUNES, 30s) +
-- ScheduleEvent(EVENT_RUNE_OF_POWER, 20s).
local function molgeimEnterCombat(event, creature, target)
    creature:SetBossState(DATA_ASSEMBLY_OF_IRON, 1) -- BossAI::JustEngagedWith arm 1 (ScriptedCreature.cpp:537) -- base-first (boss_assembly_of_iron.cpp:310)
    creature:SetCombatPulseDelay(5) -- BossAI::JustEngagedWith arm 1 (ScriptedCreature.cpp:540) -- base-first (boss_assembly_of_iron.cpp:312)
    creature:DoZoneInCombat() -- BossAI::JustEngagedWith me-form arm (ScriptedCreature.cpp:542)
    creature:Talk(SAY_MOLGEIM_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    assemblyPhase[guid] = 1 -- C++ JustEngagedWith: events.SetPhase(++phase), phase 0 -> 1 (boss_assembly_of_iron.cpp:314)
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

-- C++ Reset (:303-308): _Reset() + phase = 0 + me->RemoveAllAuras()
-- — the _Reset() SetBossState(NOT_STARTED) leg is PORTED
-- (GetBossState-gated: _Reset writes NOT_STARTED only if the state
-- != DONE, ScriptedCreature.cpp:509-510 — the four-horsemen
-- precedent); the phase leg has no bridge; RemoveAllAuras rides
-- the handler twin (engine/world/lua_creature_events.go:166, the
-- evade-leg twin).
local function molgeimReset(event, creature)
    cancelTimers(creature:GetGUID())
    if creature:GetBossState(DATA_ASSEMBLY_OF_IRON) ~= 3 then
        creature:SetBossState(DATA_ASSEMBLY_OF_IRON, 0)
    end
    creature:RemoveAllAuras()
    -- C++ Reset -> Initialize: phase = 0 (boss_assembly_of_iron.cpp:303-308).
    assemblyPhase[creature:GetGUID()] = 0
    -- Alive-gated: ev23 fires on death too (fireCreatureDied runs
    -- Reset before the kill-credit criteria read, kill.go:1091/:1400),
    -- and a death-time zero would clobber the fight's phase before
    -- the achievement gate reads it; engage re-sets it.
    if creature:IsAlive() then
        creature:SetData(DATA_PHASE_3, 0)
    end
end

-- C++ KilledUnit: who->GetTypeId() == TYPEID_PLAYER, then Talk
-- (SAY_SLAY) — the razuvious player-gated variant precedent.
local function molgeimTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_MOLGEIM_SLAY)
    end
end

-- C++ JustDied (:348-371): _JustDied() first — the SetBossState
-- (DONE) leg lands only on the last death (InstanceScript minion
-- guard, reproduced by assemblyBossesAlive) — then the
-- GetBossState DONE check branches: DONE -> DoCastAOE
-- (SPELL_KILL_CREDIT 65195, triggered) + Talk
-- (SAY_ENCOUNTER_DEFEATED 5); else SetLootRecipient(nullptr) +
-- Talk(SAY_DEATH 4) + supercharge DoAction cascade to survivors
-- (grid-sweep relay superchargeSurvivors).
local function molgeimDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    if not assemblyBossesAlive(creature) then
        creature:SetBossState(DATA_ASSEMBLY_OF_IRON, 3)
    end
    if creature:GetBossState(DATA_ASSEMBLY_OF_IRON) == 3 then
        creature:CastSpell(creature, SPELL_KILL_CREDIT, true)
        creature:Talk(SAY_MOLGEIM_ENCOUNTER_DEFEATED)
    else
        creature:SetLootRecipient(nil)
        creature:Talk(SAY_MOLGEIM_DEATH)
        superchargeSurvivors(creature)
    end
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
chainLightningTick = function(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "chainlightning", 500, function()
            chainLightningTick(creature, guid)
        end)
        return
    end
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
overloadTick = function(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "overload", 500, function()
            overloadTick(creature, guid)
        end)
        return
    end
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
    if creature:IsCasting() then
        schedule(guid, "berserk", 500, function()
            brundirBerserkTick(creature, guid)
        end)
        return
    end
    creature:Talk(SAY_BRUNDIR_BERSERK)
    creature:CastSpell(creature, SPELL_BERSERK)
end

-- C++ boss_stormcaller_brundir::JustEngagedWith: BossAI
-- passthrough (SetBossState(DATA_ASSEMBLY_OF_IRON, IN_PROGRESS)
-- first — ScriptedCreature.cpp:527-538; the CheckRequiredBosses /
-- EnterEvadeMode gate has no bridge — stay-out) + Talk(SAY_AGGRO)
-- + SetPhase + ScheduleEvent(EVENT_MOVE_POSITION,
-- 1s) + ScheduleEvent(EVENT_BERSERK, 15min) +
-- ScheduleEvent(EVENT_CHAIN_LIGHTNING, 4s) +
-- ScheduleEvent(EVENT_OVERLOAD, 60s, 120s) +
-- FindNearestCreature(NPC_WORLD_TRIGGER 22515) leg.
local function brundirEnterCombat(event, creature, target)
    creature:SetBossState(DATA_ASSEMBLY_OF_IRON, 1) -- BossAI::JustEngagedWith arm 1 (ScriptedCreature.cpp:537) -- base-first (boss_assembly_of_iron.cpp:489)
    creature:SetCombatPulseDelay(5) -- BossAI::JustEngagedWith arm 1 (ScriptedCreature.cpp:540) -- base-first (boss_assembly_of_iron.cpp:491)
    creature:DoZoneInCombat() -- BossAI::JustEngagedWith me-form arm (ScriptedCreature.cpp:542)
    creature:Talk(SAY_BRUNDIR_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    assemblyPhase[guid] = 1 -- C++ JustEngagedWith: events.SetPhase(++phase), phase 0 -> 1 (boss_assembly_of_iron.cpp:493)
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

-- C++ Reset (:471-483): _Reset() + phase = 0 + me->RemoveAllAuras() +
-- me->SetHover(false) (:476) — the _Reset() SetBossState(NOT_STARTED)
-- leg is PORTED (GetBossState-gated: _Reset writes NOT_STARTED only
-- if the state != DONE, ScriptedCreature.cpp:509-510 — the
-- four-horsemen precedent); the phase leg has no bridge;
-- RemoveAllAuras rides the handler twin
-- (engine/world/lua_creature_events.go:166, the evade-leg twin),
-- and SetHover(false) follows in C++ arm order.
local function brundirReset(event, creature)
    cancelTimers(creature:GetGUID())
    if creature:GetBossState(DATA_ASSEMBLY_OF_IRON) ~= 3 then
        creature:SetBossState(DATA_ASSEMBLY_OF_IRON, 0)
    end
    creature:RemoveAllAuras()
    creature:SetHover(false)
    -- C++ Reset -> Initialize: phase = 0 (boss_assembly_of_iron.cpp:471-483).
    assemblyPhase[creature:GetGUID()] = 0
    -- Alive-gated: ev23 fires on death too (fireCreatureDied runs
    -- Reset before the kill-credit criteria read, kill.go:1091/:1400),
    -- and a death-time zero would clobber the fight's phase before
    -- the achievement gate reads it; engage re-sets it.
    if creature:IsAlive() then
        creature:SetData(DATA_PHASE_3, 0)
    end
end

-- C++ KilledUnit: who->GetTypeId() == TYPEID_PLAYER, then Talk
-- (SAY_SLAY) — the razuvious player-gated variant precedent.
local function brundirTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_BRUNDIR_SLAY)
    end
end

-- C++ JustDied (:527-550): _JustDied() first — the SetBossState
-- (DONE) leg lands only on the last death (InstanceScript minion
-- guard, reproduced by assemblyBossesAlive) — then the
-- GetBossState DONE check branches: DONE -> DoCastAOE
-- (SPELL_KILL_CREDIT 65195, triggered) + Talk
-- (SAY_ENCOUNTER_DEFEATED 5); else SetLootRecipient(nullptr) +
-- Talk(SAY_DEATH 4) + supercharge DoAction cascade to survivors
-- (grid-sweep relay superchargeSurvivors).
local function brundirDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    if not assemblyBossesAlive(creature) then
        creature:SetBossState(DATA_ASSEMBLY_OF_IRON, 3)
    end
    if creature:GetBossState(DATA_ASSEMBLY_OF_IRON) == 3 then
        creature:CastSpell(creature, SPELL_KILL_CREDIT, true)
        creature:Talk(SAY_BRUNDIR_ENCOUNTER_DEFEATED)
    else
        creature:SetLootRecipient(nil)
        creature:Talk(SAY_BRUNDIR_DEATH)
        superchargeSurvivors(creature)
    end
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

-- C++ spell_assembly_rune_of_summoning::OnRemove
-- (boss_assembly_of_iron.cpp:746-750): EFFECT_0 PERIODIC_DUMMY. On
-- removal, a TempSummon target despawns (1ms). The periodic leg
-- (HandlePeriodic: cast SPELL_RUNE_OF_SUMMONING_SUMMON 62020 with
-- the summoner GUID) stays out — no GetSummonerGUID bridge.
local SPELL_RUNE_OF_SUMMONING = 62273 -- boss_assembly_of_iron.cpp:55

local function runeOfSummoningRemove(event, aura, target)
    if target:GetObjectType() ~= "Creature" then
        return
    end
    if target:IsSummon() then
        target:DespawnOrUnsummon(1)
    end
end

RegisterAuraEvent(SPELL_RUNE_OF_SUMMONING, 3, runeOfSummoningRemove)

-- C++ spell_shield_of_runes::OnRemove
-- (boss_assembly_of_iron.cpp:678-683): EFFECT_0 SPELL_AURA_SCHOOL_
-- ABSORB. On removal, unless the mode is EXPIRE, the caster casts
-- SPELL_SHIELD_OF_RUNES_BUFF 62277 on itself, non-triggered.
local SPELL_SHIELD_OF_RUNES_BUFF = 62277 -- boss_assembly_of_iron.cpp:52
local AURA_REMOVE_BY_EXPIRE = 4 -- SpellAuraDefines.h:67

local function shieldOfRunesRemove(event, aura, target)
    if aura:GetRemoveMode() == AURA_REMOVE_BY_EXPIRE then
        return
    end
    local caster = aura:GetCaster()
    if caster ~= nil then
        caster:CastSpell(caster, SPELL_SHIELD_OF_RUNES_BUFF)
    end
end

RegisterAuraEvent(SPELL_SHIELD_OF_RUNES, 3, shieldOfRunesRemove)
