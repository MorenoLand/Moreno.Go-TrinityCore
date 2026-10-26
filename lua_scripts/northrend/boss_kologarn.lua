-- Kologarn (Ulduar) — Lua port of
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_kologarn.cpp
-- (boss_kologarn (CreatureScript) via
-- GetUlduarAI<boss_kologarnAI> (BossAI), BOSS_KOLOGARN = 5
-- — registered from inside AddSC_boss_kologarn(); loader decl
-- 115 / call 312 per northrend_script_loader.cpp — the EIGHTH
-- group of the "// Ulduar" block in AddNorthrendScripts(),
-- immediately after AddSC_boss_assembly_of_iron() (decl 116 /
-- call 311); the call after it is AddSC_boss_mimiron() —
-- loader order confirmed this run; the checkpoint sequence
-- (boss_assembly_of_iron -> boss_kologarn) is followed).
-- Entry: 32930 Kologarn (ulduar.h NPC_KOLOGARN, line 70 —
-- entry-verifiable, registration proceeds (the
-- nexus_commanders / malygos / sartharion kalecgos precedent);
-- the CreatureScript ScriptName binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for each of the
-- nine script names ("boss_kologarn",
-- "spell_ulduar_rubble_summon", "spell_ulduar_squeezed_lifeless",
-- "spell_ulduar_cancel_stone_grip",
-- "spell_ulduar_stone_grip_cast_target",
-- "spell_ulduar_stone_grip_absorb", "spell_ulduar_stone_grip",
-- "spell_kologarn_stone_shout",
-- "spell_kologarn_summon_focused_eyebeam") hits
-- boss_kologarn.cpp only; zero sql/ hits for all nine.
-- Re-audited this run name-by-name against the C++ text:
-- all Talk ids, spell ids, triggered flags, initial/re-arm
-- timers, and gating context re-checked (line refs below).
-- The stale "no timer-event / cast bridges" doctrine is
-- disproved: CreateLuaEvent + CastSpell are real bridges
-- (the baltharus convention). Timer-driven UNGATED arms port;
-- vehicle/arm-gated arms stay out (the gothik precedent).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 5 OnSpawn, 23 OnReset.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 0) (cpp:122, event 1; the
-- vehicle-kit DoZoneInCombat arms leg and the
-- BossAI::JustEngagedWith passthrough have no bridges — the
-- auriaya engage-port precedent).
-- KilledUnit — player-gated Talk(SAY_SLAY 1) (cpp:159, event
-- 3; who->GetTypeId() == TYPEID_PLAYER — the razuvious
-- player-gated variant precedent).
-- JustDied — Talk(SAY_DEATH 6) + SetFlag(UNIT_FIELD_FLAGS,
-- UNIT_FLAG_NOT_SELECTABLE) (cpp:148, event 4; the MoveTargetedHome
-- leg, the SetCorpseDelay(604800) leg and the _JustDied()
-- passthrough have no bridges — the sjonnir JustDied-Talk
-- precedent). The DoCast(SPELL_KOLOGARN_PACIFY 63726) leg has a
-- cast bridge (creature:CastSpell, lua_creature_events.go:62) but
-- stays out as a no-op: corpse-management-cluster self-pacify on
-- a dead caster — the Go combat tick already gates dead
-- creatures (stepCreatureMotion Health==0 early-return,
-- creaturemotion.go), so the aura changes nothing (shazzrah
-- no-op-cast precedent).
-- EVENT_ENRAGE — DoCast(SPELL_BERSERK 47008) (self-cast,
-- non-triggered) + Talk(SAY_BERSERK 7) (cpp:279-281); 10min
-- init from JustEngagedWith (cpp:129), one-shot (no re-arm).
-- Rides the baltharus CreateLuaEvent convention (timers table
-- keyed by GUID, cancelled on events 2/4/23); the C++
-- UNIT_STATE_CASTING gate has no bridge, so timers fire
-- unconditionally — the documented maiden deviation.
-- EVENT_FOCUSED_EYEBEAM — DoCast(me,
-- SPELL_SUMMON_FOCUSED_EYEBEAM 63342, true) (cpp:305-311) =
-- triggered self-cast (CastSpell 3rd arg); 21s init from
-- JustEngagedWith (cpp:128), re-arm
-- ScheduleEvent(EVENT_FOCUSED_EYEBEAM, 15s, 35s) (cpp:311)
-- via the {min,max} randtime table, unconditional even when the
-- cast is skipped. Documented deviation: the SelectTarget
-- (MaxDistance) gate (cpp:304) is emulated as a null-selection
-- skip via anyThreatPlayer (threat-list players alive); the stored
-- eyebeamTarget for the JustSummoned MoveChase leg has no
-- summon/motion bridge.
-- DOCUMENTED-ONLY (in this header; no registration beyond entry
-- 32930):
-- Constructor (cpp:107-117) — RemoveFlag(NOT_SELECTABLE) +
-- SetControlled(ROOT) + DoCast(SPELL_KOLOGARN_REDUCE_PARRY
-- 64651) — PORTED (see kologarnSpawn below; SetCombatMovement(false)
-- has no bridge — the root arm alone keeps him stationary via the
-- Go chase-pause gate, the documented turret deviation).
-- Reset — _Reset() + RemoveFlag(NOT_SELECTABLE) +
-- eyebeamTarget.Clear() — no bridges.
-- PassengerBoarded — Talk(SAY_LEFT_ARM_GONE 2) (cpp:170) /
-- Talk(SAY_RIGHT_ARM_GONE 3) (cpp:180) on arm detach, arm-damage
-- SPELL_ARM_DEAD_DAMAGE 63629 + rubble-stalker
-- SPELL_FALLING_RUBBLE 63821 / SPELL_SUMMON_RUBBLE 63633,
-- EVENT_STONE_SHOUT scheduling (cpp:200) /
-- CRITERIA_DISARMED timed-achievement start, and the attach
-- branch (EVENT_STONE_SHOUT cancel + DoZoneInCombat) — no
-- PassengerBoarded bridge.
-- JustSummoned — PORTED as kologarnJustSummoned (event 19):
-- directional visuals 63676/63702 + PERIODIC 63347 + VISUAL 63369
-- (all triggered) + REACT_PASSIVE for eyebeam 33632/33802 and
-- rubble 33768; NPC_RUBBLE summon bookkeeping has no Lua summon
-- list; MoveChase(eyebeamTarget) has no motion bridge and stays out.
-- UpdateAI arms that STAY OUT (gothik precedent: porting gated
-- arms on combat-start timers would fabricate casts):
-- EVENT_MELEE_CHECK (cpp:258-261) — PORTED as meleeCheckTick:
-- DoCast(SPELL_PETRIFY_BREATH 62030) gated on
-- !me->IsWithinMeleeRange(me->GetVictim()); the gate rides
-- creature:IsWithinMeleeRange (bridged; Unit.cpp:599-618 twin incl.
-- the IsInMap leg), replacing the hand-rolled IsWithinDist twin.
-- DoCast resolves to a
-- victim cast via AISpellInfo (UnitAI.cpp FillAISpellInfo:
-- TARGET_UNIT_TARGET_ENEMY -> AITARGET_VICTIM), so the Lua
-- victim-cast (nil target) matches. 6s init, 1s re-arm.
-- EVENT_SWEEP (cpp:263-266) — DoCast(sweep stalker,
-- SPELL_ARM_SWEEP 63766, true) gated on left-arm presence
-- (the PassengerBoarded `left` flag); no PassengerBoarded
-- bridge (boarded-state has no bearer — arm-creature presence is
-- not equivalent); FindNearestCreature IS bridged
-- (lua_creature_events.go:1154) — the stalker search + triggered
-- DoCast are expressible once the gate has a bearer.
-- EVENT_SMASH (cpp:268-273) —
-- DoCastVictim(SPELL_TWO_ARM_SMASH 63356 / SPELL_ONE_ARM_SMASH
-- 63573) gated on left/right arm presence; no bridge.
-- EVENT_STONE_SHOUT (cpp:275-277) — scheduled ONLY by
-- PassengerBoarded (cpp:200, +5s, re-arm 2s) when BOTH arms
-- detach; not a combat-start event.
-- EVENT_STONE_GRIP (cpp:294-302) — DoCast(SPELL_STONE_GRIP
-- 62166) + Talk(SAY_GRAB_PLAYER 5) + Talk(EMOTE_STONE_GRIP 8)
-- gated on right-arm presence (cpp:296); no bridge.
-- EVENT_RESPAWN_LEFT_ARM / EVENT_RESPAWN_RIGHT_ARM
-- (cpp:282-292) — VehicleKit InstallAccessory(NPC_LEFT_ARM /
-- NPC_RIGHT_ARM); no vehicle bridge.
-- (say-text inventory: all nine yells accounted for — SAY_
-- AGGRO/SLAY/DEATH ported, SAY_BERSERK ported this run,
-- SAY_LEFT_ARM_GONE/SAY_RIGHT_ARM_GONE + SAY_GRAB_PLAYER/
-- EMOTE_STONE_GRIP ride the unbridgeable PassengerBoarded /
-- STONE_GRIP legs; SAY_SHOCKWAVE 4 is unused in the C++ AI.)
-- spell_ulduar_rubble_summon (SCRIPT_EFFECT SpellScript: caster
-- casts GetEffectValue() with originalCaster = instance
-- GetGuidData(BOSS_KOLOGARN) — no SpellScript bridge; joins
-- the no-SpellScript-bridge queue);
-- spell_ulduar_stone_grip_cast_target (SpellScript area-target
-- filters: EFFECT_0 removes the main tank + non-player targets,
-- EFFECT_1/2 subsequential fill — no SpellScript bridge;
-- joins the no-SpellScript-bridge queue);
-- spell_ulduar_cancel_stone_grip (SCRIPT_EFFECT SpellScript:
-- target removes the two CalcValue spell ids — no SpellScript
-- bridge; joins the no-SpellScript-bridge queue);
-- spell_ulduar_squeezed_lifeless (SpellScript INSTAKILL
-- handler — no SpellScript bridge; joins the
-- no-SpellScript-bridge queue);
-- spell_ulduar_stone_grip_absorb (AuraScript AfterEffectRemove
-- school-absorb: non-enemy-spell removal -> rubble stalker
-- casts SPELL_STONE_GRIP_CANCEL 65594 — no AuraScript bridge;
-- joins the no-AuraScript-bridge queue);
-- spell_ulduar_stone_grip (AuraScript: vehicle OnRemove ->
-- owner removes aurEff-amount spell; stun AfterEffectRemove ->
-- caster RemoveAurasDueToSpell(GetId()) — no AuraScript
-- bridge; joins the no-AuraScript-bridge queue);
-- spell_kologarn_stone_shout (SpellScript area-target filter
-- on EFFECT_0 — no SpellScript bridge; joins the
-- no-SpellScript-bridge queue);
-- spell_kologarn_summon_focused_eyebeam (SpellScript
-- FORCE_CAST handler: caster casts the effect's TriggerSpell
-- — no SpellScript bridge; joins the no-SpellScript-bridge
-- queue).

local ENTRY_KOLOGARN = 32930

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 6
local SAY_BERSERK = 7

local SPELL_SUMMON_FOCUSED_EYEBEAM = 63342
local SPELL_BERSERK = 47008
local SPELL_KOLOGARN_REDUCE_PARRY = 64651
local SPELL_PETRIFY_BREATH = 62030

local ENTRY_FOCUSED_EYEBEAM = 33632
local ENTRY_FOCUSED_EYEBEAM_RIGHT = 33802
local ENTRY_RUBBLE = 33768
local SPELL_FOCUSED_EYEBEAM_PERIODIC = 63347
local SPELL_FOCUSED_EYEBEAM_VISUAL = 63369
local SPELL_FOCUSED_EYEBEAM_VISUAL_LEFT = 63676
local SPELL_FOCUSED_EYEBEAM_VISUAL_RIGHT = 63702
local REACT_PASSIVE = 0

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000

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

-- C++ EVENT_FOCUSED_EYEBEAM: the cast is gated on
-- if (Unit* u = SelectTarget(MaxDistance, 0, 0, true)) (cpp:306-309):
-- a null selection skips the cast but the re-arm (cpp:311) stays
-- unconditional. The selection only stores eyebeamTarget for the
-- unbridged JustSummoned MoveChase leg, so it is emulated as a
-- null-selection skip over threat-list players alive (C++
-- SelectTarget draws the threat list, UnitAI.h — the old
-- instance-player anyAlivePlayer pool misread the draw).
local function anyThreatPlayer(creature)
    for _, unit in ipairs(creature:GetAITargets()) do
        if unit:GetObjectType() == "Player" and not unit:IsDead() then
            return true
        end
    end
    return false
end

-- C++ EVENT_FOCUSED_EYEBEAM: DoCast(me,
-- SPELL_SUMMON_FOCUSED_EYEBEAM 63342, true) — triggered
-- self-cast — 21s init, re-arm ScheduleEvent(15s, 35s) (cpp:311),
-- unconditional even when the SelectTarget gate skips the cast.
local function focusedEyebeamTick(creature, guid)
    if anyThreatPlayer(creature) then
        creature:CastSpell(creature, SPELL_SUMMON_FOCUSED_EYEBEAM, true)
    end
    schedule(guid, "focusedeyebeam", {15000, 35000}, function()
        focusedEyebeamTick(creature, guid)
    end)
end

-- C++ EVENT_ENRAGE: DoCast(SPELL_BERSERK 47008) (self-cast) +
-- Talk(SAY_BERSERK 7) — 10min init, one-shot (no re-arm).
local function enrageTick(creature, guid)
    creature:CastSpell(creature, SPELL_BERSERK)
    creature:Talk(SAY_BERSERK)
end

-- C++ EVENT_MELEE_CHECK: 6s init from JustEngagedWith (cpp:124),
-- 1s re-arm (cpp:261); fires DoCast(SPELL_PETRIFY_BREATH) only
-- when the victim is outside melee range, gated on
-- creature:IsWithinMeleeRange (bridged). The C++ casting-state
-- early-return has no bridge (maiden deviation, as with the other
-- timers in this file).
local function meleeCheckTick(creature, guid)
    local victim = creature:GetVictim()
    if victim and not creature:IsWithinMeleeRange(victim) then
        creature:CastSpell(nil, SPELL_PETRIFY_BREATH)
    end
    schedule(guid, "meleecheck", 1000, function()
        meleeCheckTick(creature, guid)
    end)
end

-- Constructor (boss_kologarn.cpp:107-117) — RemoveFlag(UNIT_FIELD_FLAGS,
-- UNIT_FLAG_NOT_SELECTABLE) + SetControlled(true, UNIT_STATE_ROOT)
-- (creature:SetRooted, lua_creature_events.go) + DoCast(self,
-- SPELL_KOLOGARN_REDUCE_PARRY 64651) non-triggered, in C++ arm order;
-- SetCombatMovement(false) is bridged (lua_creature_events.go:759) and
-- PORTED in kologarnEnterCombat below (event 1); the root arm (SetRooted,
-- bridged) would alone keep him stationary (Go's chase-pause NOT_MOVE
-- gate, creaturemotion.go), the documented turret deviation. Go event 5
-- fires once per spawn (grid/respawn funnel, creaturemotion.go:713) —
-- the ctor analog (felmyst trail-spawn precedent).
local function kologarnSpawn(event, creature)
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    creature:SetRooted(true)
    creature:CastSpell(creature, SPELL_KOLOGARN_REDUCE_PARRY)
end

-- C++ JustEngagedWith: Talk(SAY_AGGRO) + the two bridgeable
-- event-scheduling legs (EVENT_FOCUSED_EYEBEAM 21s,
-- EVENT_ENRAGE 10min); the vehicle-kit DoZoneInCombat arms leg
-- and the BossAI::JustEngagedWith passthrough have no
-- bridges.
local function kologarnEnterCombat(event, creature, target)
    creature:SetCombatMovement(false) -- ScriptedAI ctor arm, boss_kologarn.cpp:114 (ranged AI stands and casts)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "focusedeyebeam", 21000, function()
        focusedEyebeamTick(creature, guid)
    end)
    schedule(guid, "enrage", 600000, function()
        enrageTick(creature, guid)
    end)
    schedule(guid, "meleecheck", 6000, function()
        meleeCheckTick(creature, guid)
    end)
end

local function kologarnLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function kologarnReset(event, creature)
    cancelTimers(creature:GetGUID())
    -- C++ boss_kologarn.cpp:142: me->RemoveFlag(UNIT_FIELD_FLAGS,
    -- UNIT_FLAG_NOT_SELECTABLE) (Reset: flip follows _Reset); JustDied
    -- sets the flag live (boss_kologarn.cpp:151), so the reset clear is
    -- a real re-arm, not a template no-op.
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
end

-- C++ KilledUnit: who->GetTypeId() == TYPEID_PLAYER, then Talk
-- (SAY_SLAY) — the razuvious player-gated variant precedent.
local function kologarnTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ JustDied: DoCast PACIFY + MoveTargetedHome + SetFlag +
-- SetCorpseDelay + _JustDied() + Talk(SAY_DEATH) — the Talk and
-- SetFlag arms are ported (the PACIFY cast is bridgeable but a
-- no-op on a dead caster — see header).
local function kologarnDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(SAY_DEATH)
    -- C++ boss_kologarnAI::JustDied (boss_kologarn.cpp:151): the corpse
    -- stays unselectable (SetCorpseDelay 604800 keeps the corpse a week).
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
end

RegisterCreatureEvent(ENTRY_KOLOGARN, 1, kologarnEnterCombat)
RegisterCreatureEvent(ENTRY_KOLOGARN, 2, kologarnLeaveCombat)
RegisterCreatureEvent(ENTRY_KOLOGARN, 3, kologarnTargetDied)
RegisterCreatureEvent(ENTRY_KOLOGARN, 4, kologarnDied)
RegisterCreatureEvent(ENTRY_KOLOGARN, 5, kologarnSpawn)
RegisterCreatureEvent(ENTRY_KOLOGARN, 23, kologarnReset)

-- C++ boss_kologarnAI::JustSummoned (boss_kologarn.cpp:214-235):
-- eyebeam 33632/33802 cast their directional visual on the boss
-- (triggered); rubble 33768 joins the summon list (no Lua summon
-- list); all three then cast PERIODIC 63347 + VISUAL 63369 on
-- themselves (triggered) and go REACT_PASSIVE, in C++ order. The
-- eyebeamTarget Attack/MoveChase legs (:237-242) have no motion
-- bridge and stay out. Event 19 fires on the summoner for the
-- ported SUMMON_FOCUSED_EYEBEAM spell-effect summon through the
-- shared summonTempCreature funnel (boss_ai.go:1734).
local function kologarnJustSummoned(event, creature, summon)
    local entry = summon:GetEntry()
    if entry == ENTRY_FOCUSED_EYEBEAM then
        summon:CastSpell(creature, SPELL_FOCUSED_EYEBEAM_VISUAL_LEFT, true)
    elseif entry == ENTRY_FOCUSED_EYEBEAM_RIGHT then
        summon:CastSpell(creature, SPELL_FOCUSED_EYEBEAM_VISUAL_RIGHT, true)
    else
        -- C++ boss_kologarn.cpp:214-235: NPC_RUBBLE summons.Summon
        -- then falls through to default: return — rubble never
        -- reaches the PERIODIC/VISUAL/PASSIVE legs below.
        return
    end
    summon:CastSpell(summon, SPELL_FOCUSED_EYEBEAM_PERIODIC, true)
    summon:CastSpell(summon, SPELL_FOCUSED_EYEBEAM_VISUAL, true)
    summon:SetReactState(REACT_PASSIVE)
end
RegisterCreatureEvent(ENTRY_KOLOGARN, 19, kologarnJustSummoned)
