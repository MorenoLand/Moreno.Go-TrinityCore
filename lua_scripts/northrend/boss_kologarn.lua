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
-- vehicle/arm-gated arms port once the PassengerBoarded latches
-- have a bearer (the gothik precedent still bars ungated
-- timer ports of gated arms).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 5 OnSpawn, 23 OnReset.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 0) (cpp:122, event 1) +
-- _JustEngagedWith SetBossState(DATA_KOLOGARN 5, IN_PROGRESS)
-- PORTED 2026-11-05 via creature:SetBossState
-- (lua_creature_events.go:5917; the maiden_of_grief precedent) —
-- the stale "no instance bridge" claim is retracted; the
-- vehicle-kit DoZoneInCombat arms leg is PORTED this run (tick
-- 474, GetPassenger bridge, lua_vehicle.go:74) and the
-- CheckRequiredBosses gate (EnterEvadeMode has no bridge) stays
-- out. Base-last in C++ (cpp:136), so the state leg sits after
-- the schedules with SetCombatPulseDelay/DoZoneInCombat.
-- KilledUnit — player-gated Talk(SAY_SLAY 1) (cpp:159, event
-- 3; who->GetTypeId() == TYPEID_PLAYER — the razuvious
-- player-gated variant precedent).
-- JustDied — Talk(SAY_DEATH 6) + SetFlag(UNIT_FIELD_FLAGS,
-- UNIT_FLAG_NOT_SELECTABLE) (cpp:148, event 4) + _JustDied()
-- SetBossState(DATA_KOLOGARN 5, DONE) PORTED 2026-11-05 via
-- creature:SetBossState (lua_creature_events.go:5917; base-last in
-- C++ cpp:155, so the state leg sits after Talk/SetFlag); the
-- MoveTargetedHome leg is PORTED tick 386 via creature:MoveHome
-- (lua_creature_events.go:4680; the old "no bridge" claim is stale)
-- and the SetCorpseDelay(604800) leg has no bridge. The DoCast(SPELL_KOLOGARN_PACIFY 63726) leg has a
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
-- Boss UpdateAI CASTING gates (boss_kologarn.cpp pre-loop :251
-- + in-loop :315) PORTED as a 500ms IsCasting defer in all
-- seven cast ticks (focusedEyebeam/enrage/meleeCheck/smash/
-- sweep/stoneShout/stoneGrip) (mal_ganis idiom; IsCasting
-- live, lua_creature_events.go:1950) — the stale
-- "gate has no bridge" claim is retracted.
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
-- 64651) + SetCombatMovement(false) — PORTED (see kologarnSpawn
-- below; the root arm alone keeps him stationary via the
-- Go chase-pause gate, the documented turret deviation).
-- Reset — _Reset() SetBossState(DATA_KOLOGARN 5, NOT_STARTED)
-- PORTED 2026-11-05 via creature:SetBossState
-- (lua_creature_events.go:5917; the maiden_of_grief precedent) +
-- RemoveFlag(NOT_SELECTABLE) (ported, see kologarnReset);
-- eyebeamTarget.Clear() has no summon-list bridge.
-- PassengerBoarded — PORTED 2026-11-08 via RegisterVehicleEvent
-- (5 add / 6 remove; the warhead precedent) + the new
-- InstanceData:StartTimedAchievement bridge: Talk(SAY_LEFT_ARM_GONE
-- 2) (cpp:170) / Talk(SAY_RIGHT_ARM_GONE 3) (cpp:180) on arm detach
-- + 40s respawn schedule, arm-damage SPELL_ARM_DEAD_DAMAGE 63629 +
-- rubble-stalker SPELL_FALLING_RUBBLE 63821 / SPELL_SUMMON_RUBBLE
-- 63633 + arm despawn (inside the stalker-found leg, cpp:194-197),
-- EVENT_STONE_SHOUT scheduling (cpp:200) / CRITERIA_DISARMED 21687
-- timed-achievement start (type 1), and the attach branch
-- (EVENT_STONE_SHOUT cancel + DoZoneInCombat on the arm). The
-- stale "no PassengerBoarded bridge" claim is retracted: the
-- add/remove fire sites are live (vehicle.go install/remove
-- paths) and the left/right latches now have a bearer.
-- JustSummoned — PORTED as kologarnJustSummoned (event 19):
-- directional visuals 63676/63702 + PERIODIC 63347 + VISUAL 63369
-- (all triggered) + REACT_PASSIVE for eyebeam 33632/33802 and
-- rubble 33768; NPC_RUBBLE summon bookkeeping has no Lua summon
-- list; MoveChase(eyebeamTarget) has no motion bridge and stays out.
-- UpdateAI arms (gothik precedent: porting gated arms on
-- combat-start timers would fabricate casts — the latch-gated
-- arms below port ONLY because the PassengerBoarded latches
-- now have a bearer):
-- EVENT_MELEE_CHECK (cpp:258-261) — PORTED as meleeCheckTick:
-- DoCast(SPELL_PETRIFY_BREATH 62030) gated on
-- !me->IsWithinMeleeRange(me->GetVictim()); the gate rides
-- creature:IsWithinMeleeRange (bridged; Unit.cpp:599-618 twin incl.
-- the IsInMap leg), replacing the hand-rolled IsWithinDist twin.
-- DoCast resolves to a
-- victim cast via AISpellInfo (UnitAI.cpp FillAISpellInfo:
-- TARGET_UNIT_TARGET_ENEMY -> AITARGET_VICTIM), so the Lua
-- victim-cast (nil target) matches. 6s init, 1s re-arm.
-- EVENT_SWEEP (cpp:263-266) — PORTED as sweepTick 2026-11-08:
-- DoCast(sweep stalker, SPELL_ARM_SWEEP 63766, true) gated on the
-- left latch; stalker via FindNearestCreature(33661, 500)
-- (lua_creature_events.go:5959); null-stalker skip (the
-- focused-eyebeam null-selection precedent — C++ DoCast(null)
-- falls through to Unit::CastSpell target resolution); re-arm
-- 25s unconditional. 19s init.
-- EVENT_SMASH (cpp:268-273) — PORTED as smashTick 2026-11-08:
-- DoCastVictim(SPELL_TWO_ARM_SMASH 63356 / SPELL_ONE_ARM_SMASH
-- 63573) gated on the left/right latches; re-arm 15s. 5s init.
-- EVENT_STONE_SHOUT (cpp:275-277) — PORTED as stoneShoutTick
-- 2026-11-08: scheduled ONLY by PassengerBoarded (cpp:200, +5s,
-- re-arm 2s) when BOTH arms detach; DoCast(SPELL_STONE_SHOUT
-- 63716) self-cast non-triggered.
-- EVENT_STONE_GRIP (cpp:294-302) — PORTED as stoneGripTick
-- 2026-11-08: DoCast(SPELL_STONE_GRIP 62166) + Talk(SAY_GRAB_PLAYER
-- 5) + Talk(EMOTE_STONE_GRIP 8) gated on the right latch; re-arm
-- 25s unconditional. 25s init.
-- EVENT_RESPAWN_LEFT_ARM / EVENT_RESPAWN_RIGHT_ARM
-- (cpp:282-292) — PORTED as respawnArmTick 2026-11-08 via
-- SummonCreature (default summonType 7 = TEMPSUMMON_MANUAL_DESPAWN)
-- + GetVehicleKit():AddPassenger(guid, seat 0/1); the board fires
-- OnAddPassenger, re-arming the latch exactly like C++
-- InstallAccessory. Documented deviation: the summon spawns at
-- Kologarn's position and snaps to the seat offset on board;
-- C++ summons directly at the accessory offset.
-- (say-text inventory: all nine yells accounted for — SAY_
-- AGGRO/SLAY/DEATH ported, SAY_BERSERK ported this run,
-- SAY_LEFT_ARM_GONE/SAY_RIGHT_ARM_GONE + SAY_GRAB_PLAYER/
-- EMOTE_STONE_GRIP ride the unbridgeable PassengerBoarded /
-- STONE_GRIP legs; SAY_SHOCKWAVE 4 is unused in the C++ AI.)
-- spell_ulduar_rubble_summon (SCRIPT_EFFECT SpellScript: caster
-- casts GetEffectValue() with originalCaster = instance
-- GetGuidData(BOSS_KOLOGARN) — PORTED 2026-11-11 via
-- RegisterSpellEvent ev5 below: ev5 is the OnEffectHitTarget
-- bridge (runtime.go), GetEffectValue + GetGuidData + the
-- CastCustomSpell originalCaster arg are all live (the arg is
-- accepted-and-ignored, no Go model yet); binding 63633 =
-- SPELL_SUMMON_RUBBLE (boss_kologarn.cpp:47, stalker cast
-- :195, ported line 703). DORMANT until Spell.dbc ships
-- 63633's SCRIPT_EFFECT value (fail-closed 0 read).);
-- spell_ulduar_stone_grip_cast_target (SpellScript area-target
-- filters: EFFECT_0 removes the main tank (only when the threat
-- list holds > 1) + non-player targets and caps to 1 (62166) / 3
-- (63981) random targets, EFFECT_1/2 refill from the saved list —
-- PORTED 2026-11-08 (tick 464) via RegisterSpellEvent ev7 below:
-- the Go fire hands each effect its own per-effect list in effect
-- order (lua_spell.go fireSpellTargetSelects), so the saved list
-- rides a module table keyed by caster GUID (one Kologarn per
-- instance, one cast at a time). DORMANT until Spell.dbc ships:
-- the 62166/63981 casts are DBC-gated.);
-- spell_ulduar_stone_grip (AuraScript: stun AfterEffectRemove ->
-- owner player removes aurEff-amount spell — PORTED 2026-11-08
-- (tick 464) via RegisterAuraEvent ev3 below (aura:GetAmount(2)
-- reads the removed record's snapshot amounts; the > 0 guard is
-- the fail-closed DBC-absent read); vehicle OnEffectRemove ->
-- PreventDefaultAction + caster _ExitVehicle at the exit position
-- + caster RemoveAurasDueToSpell(GetId()) + Relocate/MoveFall/
-- Relocate — PORTED 2026-11-08 (tick 466) via the same ev3 handler:
-- the ev3 veto bridge (fireAuraRemove returns the PreventDefaultAction
-- verdict, Go) + caster RemoveAura(GetAuraId()) + the tick-466
-- creature ExitVehicle-with-position / Relocate / MoveFall bridges
-- (lua_creature_events.go); the exit runs before the aura strip,
-- the C++ order. RemovePendingEventsForPassenger has no Go model
-- (vehicle events are synchronous in Go) and stays out; the caster
-- boards only via the creature-caster apply leg (spells.go).
-- spell_ulduar_cancel_stone_grip (SCRIPT_EFFECT SpellScript:
-- target removes the CalcValue spell id for its difficulty —
-- PORTED 2026-11-08 (tick 464) via RegisterSpellEvent ev5 below:
-- target->GetVehicle() ports as target:IsOnVehicle() ~= nil (the
-- player/creature arms push the vehicle object or nil, the exact
-- GetVehicle() truthiness twin); the binding is the in-house enum
-- SPELL_STONE_GRIP_CANCEL 65594 (boss_kologarn.cpp:46, the rotface
-- in-house-enum precedent — AC's current spell_script_names
-- carries no row, the same deleted-upstream class as the absorb
-- 64224/64225 binding); difficulty reads the caster stalker
-- (Is25ManRaid is creature-only); values read dynamically via
-- spell:GetEffectValue(0/1), the CalcValue twin. DORMANT until
-- Spell.dbc ships: 65594 is cast only by the absorb leg's
-- stalker, itself DBC-gated.);
-- spell_ulduar_stone_grip_absorb (AuraScript AfterEffectRemove
-- school-absorb: ENEMY_SPELL removal only (boss_kologarn.cpp:537-544)
-- -> rubble stalker casts SPELL_STONE_GRIP_CANCEL 65594 — PORTED
-- 2026-11-08 (tick 463) via RegisterAuraEvent ev3 below: the binding
-- is 64224/64225, verified this tick against live Wowhead 3.3.5 DBC
-- renders ("Stone Grip Absorb", effect 1 absorb 80000/380000) which
-- corroborate the historical TrinityCore spell_script_names row;
-- AC's current DB carries no absorb row (script deleted upstream)
-- and no 64224 row at all, so there is no competing binding. The
-- ev3 remove hook fires with the removal mode (absorption.go
-- removes depleted absorbs with auraRemoveByEnemySpell, mode 3),
-- so the C++ ENEMY_SPELL gate ports as aura:GetRemoveMode() == 3.
-- spell_ulduar_squeezed_lifeless (SpellScript INSTAKILL
-- OnEffectHitTarget EFFECT_1 — STAYS OUT: the hook itself is
-- bridgeable (ev5, OnEffectHitTarget), but the binding spell
-- id is DB-side (no SQUEEZED enum in boss_kologarn.cpp,
-- census spell 0) and three legs have no bridge: player
-- ExitVehicle-with-position (session.exitVehicle takes no
-- position), DestroyForNearbyPlayers, UpdateObjectVisibility.);
-- spell_kologarn_stone_shout (SpellScript area-target filter
-- on EFFECT_0 — PORTED 2026-11-11 via RegisterSpellEvent ev7
-- below: keep players + pets, drop everything else; the
-- "no SpellScript bridge" note is stale, ev7 is live);
-- spell_kologarn_summon_focused_eyebeam (SpellScript
-- FORCE_CAST handler: caster casts the effect's TriggerSpell
-- — PORTED 2026-11-11 via RegisterSpellEvent ev5 below:
-- PreventHitDefaultEffect + GetTriggerSpell are both live;
-- the "no SpellScript bridge" note is stale).

local ENTRY_KOLOGARN = 32930

-- DATA_KOLOGARN = 5 (ulduar.h:41).
local DATA_KOLOGARN = 5
local BOSS_STATE_IN_PROGRESS = 1
local ACHIEVEMENT_TIMED_TYPE_EVENT = 1
local CRITERIA_DISARMED = 21687

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_LEFT_ARM_GONE = 2
local SAY_RIGHT_ARM_GONE = 3
local SAY_GRAB_PLAYER = 5
local SAY_DEATH = 6
local SAY_BERSERK = 7
local EMOTE_STONE_GRIP = 8

local SPELL_SUMMON_FOCUSED_EYEBEAM = 63342
local SPELL_BERSERK = 47008
local SPELL_KOLOGARN_REDUCE_PARRY = 64651
local SPELL_PETRIFY_BREATH = 62030
local SPELL_ARM_DEAD_DAMAGE = 63629
local SPELL_TWO_ARM_SMASH = 63356
local SPELL_ONE_ARM_SMASH = 63573
local SPELL_ARM_SWEEP = 63766
local SPELL_STONE_SHOUT = 63716
local SPELL_STONE_GRIP = 62166
local SPELL_STONE_GRIP_HEROIC = 63981
local SPELL_STONE_GRIP_ABSORB = 64224
local SPELL_STONE_GRIP_ABSORB_HEROIC = 64225
local SPELL_STONE_GRIP_CANCEL = 65594
local SPELL_FALLING_RUBBLE = 63821
local SPELL_SUMMON_RUBBLE = 63633

local ENTRY_FOCUSED_EYEBEAM = 33632
local ENTRY_FOCUSED_EYEBEAM_RIGHT = 33802
local ENTRY_RUBBLE = 33768
local ENTRY_LEFT_ARM = 32933
local ENTRY_RIGHT_ARM = 32934
local ENTRY_RUBBLE_STALKER = 33809
local ENTRY_RUBBLE_STALKER_HEROIC = 33942
local ENTRY_ARM_SWEEP_STALKER = 33661
local SPELL_FOCUSED_EYEBEAM_PERIODIC = 63347
local SPELL_FOCUSED_EYEBEAM_VISUAL = 63369
local SPELL_FOCUSED_EYEBEAM_VISUAL_LEFT = 63676
local SPELL_FOCUSED_EYEBEAM_VISUAL_RIGHT = 63702
local REACT_PASSIVE = 0

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000

local timers = {}

-- Per-cast shared target list for the stone-grip cast-target filter
-- (C++ spell_ulduar_stone_grip_cast_target::_unitList): effect 0
-- stores the filtered list, effects 1/2 refill from it. Keyed by
-- caster GUID; one Kologarn per instance and one cast at a time, so
-- no two casts share the slot; cleared by the effect-2 leg.
local stoneGripTargets = {}

-- PassengerBoarded left/right latches (C++ boss_kologarnAI ctor:
-- left(false), right(false); set only by the vehicle events).
-- Keyed by Kologarn GUID like the timers table; cleared on death
-- (the AI instance dies with the boss — a new spawn gets a fresh
-- GUID). Reset/leave-combat deliberately do NOT clear them:
-- C++ Reset() never touches left/right.
local arms = {}

local function armState(guid)
    local state = arms[guid]
    if not state then
        state = { left = false, right = false }
        arms[guid] = state
    end
    return state
end

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

local function cancelTimer(guid, key)
    local per = timers[guid]
    if per and per[key] then
        RemoveEventById(per[key])
        per[key] = nil
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
    if creature:IsCasting() then
        schedule(guid, "focusedeyebeam", 500, function()
            focusedEyebeamTick(creature, guid)
        end)
        return
    end
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
    if creature:IsCasting() then
        schedule(guid, "enrage", 500, function()
            enrageTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_BERSERK)
    creature:Talk(SAY_BERSERK)
end

-- C++ EVENT_MELEE_CHECK: 6s init from JustEngagedWith (cpp:124),
-- 1s re-arm (cpp:261); fires DoCast(SPELL_PETRIFY_BREATH) only
-- when the victim is outside melee range, gated on
-- creature:IsWithinMeleeRange (bridged); the C++ casting-state
-- early-return ports as the 500ms IsCasting defer above.
local function meleeCheckTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "meleecheck", 500, function()
            meleeCheckTick(creature, guid)
        end)
        return
    end
    local victim = creature:GetVictim()
    if victim and not creature:IsWithinMeleeRange(victim) then
        creature:CastSpell(nil, SPELL_PETRIFY_BREATH)
    end
    schedule(guid, "meleecheck", 1000, function()
        meleeCheckTick(creature, guid)
    end)
end

-- C++ EVENT_SMASH: DoCastVictim(TWO_ARM_SMASH 63356) when both
-- arms are boarded, else ONE_ARM_SMASH 63573 when either is
-- (cpp:268-273); 15s re-arm unconditional. 5s init.
local function smashTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "smash", 500, function()
            smashTick(creature, guid)
        end)
        return
    end
    local state = armState(guid)
    if state.left and state.right then
        creature:CastSpell(nil, SPELL_TWO_ARM_SMASH)
    elseif state.left or state.right then
        creature:CastSpell(nil, SPELL_ONE_ARM_SMASH)
    end
    schedule(guid, "smash", 15000, function()
        smashTick(creature, guid)
    end)
end

-- C++ EVENT_SWEEP: if (left) DoCast(sweep stalker,
-- SPELL_ARM_SWEEP 63766, true) (cpp:263-266); 25s re-arm
-- unconditional. 19s init.
local function sweepTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "sweep", 500, function()
            sweepTick(creature, guid)
        end)
        return
    end
    local state = armState(guid)
    if state.left then
        local stalker = creature:FindNearestCreature(ENTRY_ARM_SWEEP_STALKER, 500)
        if stalker then
            creature:CastSpell(stalker, SPELL_ARM_SWEEP, true)
        end
    end
    schedule(guid, "sweep", 25000, function()
        sweepTick(creature, guid)
    end)
end

-- C++ EVENT_STONE_SHOUT: DoCast(SPELL_STONE_SHOUT 63716)
-- (cpp:275-277); 2s re-arm. Scheduled only by PassengerBoarded.
local function stoneShoutTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "stoneshout", 500, function()
            stoneShoutTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_STONE_SHOUT)
    schedule(guid, "stoneshout", 2000, function()
        stoneShoutTick(creature, guid)
    end)
end

-- C++ EVENT_STONE_GRIP: if (right) { DoCast(SPELL_STONE_GRIP
-- 62166); Talk(SAY_GRAB_PLAYER 5); Talk(EMOTE_STONE_GRIP 8) }
-- (cpp:294-302); 25s re-arm unconditional. 25s init.
local function stoneGripTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "stonegrip", 500, function()
            stoneGripTick(creature, guid)
        end)
        return
    end
    local state = armState(guid)
    if state.right then
        creature:CastSpell(creature, SPELL_STONE_GRIP)
        creature:Talk(SAY_GRAB_PLAYER)
        creature:Talk(EMOTE_STONE_GRIP)
    end
    schedule(guid, "stonegrip", 25000, function()
        stoneGripTick(creature, guid)
    end)
end

-- C++ EVENT_RESPAWN_LEFT_ARM / EVENT_RESPAWN_RIGHT_ARM
-- (cpp:282-292): VehicleKit InstallAccessory(entry, seat, true,
-- TEMPSUMMON_MANUAL_DESPAWN, 0) — seat 0 left, 1 right.
-- SummonCreature's default summonType 7 is the manual-despawn
-- twin; the board re-fires OnAddPassenger, re-arming the latch.
local function respawnArmTick(creature, guid, entry, seat)
    local kit = creature:GetVehicleKit()
    if kit == nil then
        return
    end
    local arm = creature:SummonCreature(entry, creature:GetX(), creature:GetY(), creature:GetZ(), creature:GetO())
    if arm ~= nil then
        kit:AddPassenger(arm, seat) -- unit object, not GUID: luaVehicleUnitArg errors on a number (lua_vehicle.go:142)
    end
end

-- Constructor (boss_kologarn.cpp:107-117) — RemoveFlag(UNIT_FIELD_FLAGS,
-- UNIT_FLAG_NOT_SELECTABLE) + SetControlled(true, UNIT_STATE_ROOT)
-- (creature:SetRooted, lua_creature_events.go) + DoCast(self,
-- SPELL_KOLOGARN_REDUCE_PARRY 64651) non-triggered, in C++ arm order;
-- SetCombatMovement(false) is bridged (lua_creature_events.go:759) and
-- rides this handler too (moved here from enterCombat, tick 474 —
-- the ctor arm runs at spawn); the root arm (SetRooted,
-- bridged) would alone keep him stationary (Go's chase-pause NOT_MOVE
-- gate, creaturemotion.go), the documented turret deviation. Go event 5
-- fires once per spawn (grid/respawn funnel, creaturemotion.go:713) —
-- the ctor analog (felmyst trail-spawn precedent).
local function kologarnSpawn(event, creature)
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    creature:SetRooted(true)
    creature:CastSpell(creature, SPELL_KOLOGARN_REDUCE_PARRY)
    -- C++ ctor :114: SetCombatMovement(false) — moved here from
    -- kologarnEnterCombat this run (tick 474): the ctor arm runs
    -- at spawn, and ev5 is the ctor analog; the root arm alone
    -- already kept him stationary, this is the C++-exact seat.
    creature:SetCombatMovement(false)
end

-- C++ JustEngagedWith: Talk(SAY_AGGRO) + the bridgeable
-- event-scheduling legs (EVENT_MELEE_CHECK 6s, EVENT_FOCUSED_EYEBEAM
-- 21s, EVENT_ENRAGE 10min) + the vehicle-kit DoZoneInCombat arms
-- leg (:132-135, seats 0/1, before the base call); the
-- CheckRequiredBosses gate (EnterEvadeMode has no bridge) stays
-- out; BossAI::JustEngagedWith is LAST (cpp:136) and its
-- SetBossState/SetCombatPulseDelay/DoZoneInCombat arms are ported
-- below in base order (ScriptedCreature.cpp:527-544).
local function kologarnEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUIDLow()
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
    schedule(guid, "smash", 5000, function()
        smashTick(creature, guid)
    end)
    schedule(guid, "sweep", 19000, function()
        sweepTick(creature, guid)
    end)
    schedule(guid, "stonegrip", 25000, function()
        stoneGripTick(creature, guid)
    end)
    -- C++ JustEngagedWith :132-135: the vehicle-kit arms enter
    -- combat with the zone (seats 0/1) before the BossAI base
    -- call — GetPassenger is bridged (lua_vehicle.go:74,
    -- warhead precedent) and the arm-side DoZoneInCombat is
    -- the attach-branch precedent in this file.
    local kit = creature:GetVehicleKit()
    if kit ~= nil then
        for seat = 0, 1 do
            local arm = kit:GetPassenger(seat)
            if arm ~= nil then
                arm:DoZoneInCombat()
            end
        end
    end
    -- BossAI::JustEngagedWith(who) is LAST in C++ (boss_kologarn.cpp:136, after the schedules + vehicle-kit DoZone legs):
    -- _JustEngagedWith base order (ScriptedCreature.cpp:527-544):
    -- SetBossState(IN_PROGRESS) first, then SetCombatPulseDelay(5)
    -- + me-form DoZoneInCombat.
    creature:SetBossState(DATA_KOLOGARN, 1)
    creature:SetCombatPulseDelay(5) -- BossAI::JustEngagedWith arm 2 (ScriptedCreature.cpp:540)
    creature:DoZoneInCombat() -- BossAI::JustEngagedWith me-form arm (ScriptedCreature.cpp:542)
end

local function kologarnLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

local function kologarnReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    -- C++ Reset (boss_kologarn.cpp:140-144): _Reset() first —
    -- SetBossState(DATA_KOLOGARN, NOT_STARTED)
    -- (ScriptedCreature.cpp:499-511; the maiden_of_grief precedent
    -- sets it unconditionally on ev23).
    creature:SetBossState(DATA_KOLOGARN, 0)
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

-- C++ JustDied: Talk(SAY_DEATH) first, then DoCast PACIFY +
-- MoveTargetedHome + SetFlag + SetCorpseDelay, _JustDied() LAST —
-- the Talk, MoveHome, SetFlag and SetBossState(DONE) arms are
-- ported (the PACIFY cast is bridgeable but a no-op on a dead
-- caster — see header).
local function kologarnDied(event, creature, killer)
    cancelTimers(creature:GetGUIDLow())
    arms[creature:GetGUIDLow()] = nil
    creature:Talk(SAY_DEATH)
    -- C++ boss_kologarn.cpp:151-152: MoveTargetedHome after Talk.
    creature:MoveHome()
    -- C++ boss_kologarnAI::JustDied (boss_kologarn.cpp:151): the corpse
    -- stays unselectable (SetCorpseDelay 604800 keeps the corpse a week).
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    -- C++ JustDied (boss_kologarn.cpp:155): _JustDied() is LAST —
    -- SetBossState(DATA_KOLOGARN, DONE) (ScriptedCreature.cpp:513-520).
    creature:SetBossState(DATA_KOLOGARN, 3)
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

-- C++ boss_kologarnAI::PassengerBoarded (boss_kologarn.cpp:162-212)
-- via RegisterVehicleEvent 5 (add) / 6 (remove): the warhead
-- handler shape (event, vehicle, passenger, seatId); the remove
-- event carries no seat, but apply rides the event id. The
-- left/right flags set even out of combat (cpp:164-185); the Talk
-- + 40s respawn schedule only when detaching in progress; the
-- damage/rubble/despawn/shout/timed-achievement legs only when in
-- progress; the attach branch cancels the shout and zones the arm
-- in (DoZoneInCombat is the me-form on the passenger).
local function kologarnPassengerBoarded(event, vehicle, passenger, seatId)
    if vehicle:GetEntry() ~= ENTRY_KOLOGARN then
        return
    end
    if passenger == nil then
        return
    end
    local entry = passenger:GetEntry()
    if entry ~= ENTRY_LEFT_ARM and entry ~= ENTRY_RIGHT_ARM then
        return
    end
    local guid = vehicle:GetGUIDLow()
    local state = armState(guid)
    local isLeft = entry == ENTRY_LEFT_ARM
    local apply = event == 5
    if isLeft then
        state.left = apply
    else
        state.right = apply
    end
    local inProgress = vehicle:GetBossState(DATA_KOLOGARN) == BOSS_STATE_IN_PROGRESS
    if not apply and inProgress then
        if isLeft then
            vehicle:Talk(SAY_LEFT_ARM_GONE)
            schedule(guid, "respawnleftarm", 40000, function()
                respawnArmTick(vehicle, guid, ENTRY_LEFT_ARM, 0)
            end)
        else
            vehicle:Talk(SAY_RIGHT_ARM_GONE)
            schedule(guid, "respawnrightarm", 40000, function()
                respawnArmTick(vehicle, guid, ENTRY_RIGHT_ARM, 1)
            end)
        end
    end
    if not inProgress then
        return
    end
    if not apply then
        passenger:CastSpell(vehicle, SPELL_ARM_DEAD_DAMAGE, true)
        local stalker = passenger:FindNearestCreature(ENTRY_RUBBLE_STALKER, 70)
        if stalker then
            stalker:CastSpell(stalker, SPELL_FALLING_RUBBLE, true)
            stalker:CastSpell(stalker, SPELL_SUMMON_RUBBLE, true)
            passenger:DespawnOrUnsummon()
        end
        if not state.right and not state.left then
            schedule(guid, "stoneshout", 5000, function()
                stoneShoutTick(vehicle, guid)
            end)
        end
        local mapObj = vehicle:GetMap()
        if mapObj then
            local data = mapObj:GetInstanceData()
            if data then
                data:StartTimedAchievement(ACHIEVEMENT_TIMED_TYPE_EVENT, CRITERIA_DISARMED)
            end
        end
    else
        cancelTimer(guid, "stoneshout")
        passenger:DoZoneInCombat()
    end
end
RegisterVehicleEvent(5, kologarnPassengerBoarded)
RegisterVehicleEvent(6, kologarnPassengerBoarded)

-- C++ spell_ulduar_stone_grip_absorb::OnRemove
-- (boss_kologarn.cpp:526-549): AfterEffectRemove EFFECT_0
-- SPELL_AURA_SCHOOL_ABSORB, AURA_EFFECT_HANDLE_REAL — fires when
-- the Right Arm's absorb aura is removed by an enemy spell (the
-- raid breaking through it); the nearest rubble stalker (33809
-- normal / 33942 heroic, 200yd) casts SPELL_STONE_GRIP_CANCEL
-- 65594 on itself, triggered. Bound 64224/64225 (see header).
-- DORMANT until Spell.dbc ships: 64224/64225 are applied by the
-- grip DBC chain, never cast in-house.
local function stoneGripAbsorbRemoved(event, aura, target)
    if target == nil or target:GetObjectType() ~= "Creature" then
        return
    end
    if aura:GetRemoveMode() ~= 3 then
        return
    end
    -- C++ picks the heroic stalker for every difficulty but
    -- 10-man normal (DUNGEON_DIFFICULTY_NORMAL 0, :541); 10-man
    -- heroic is not 25-man, so the 25-man test alone misses it.
    local entry = ENTRY_RUBBLE_STALKER
    if target:Is25ManRaid() or target:IsHeroic() then
        entry = ENTRY_RUBBLE_STALKER_HEROIC
    end
    local stalker = target:FindNearestCreature(entry, 200)
    if stalker then
        stalker:CastSpell(stalker, SPELL_STONE_GRIP_CANCEL, true)
    end
end
RegisterAuraEvent(SPELL_STONE_GRIP_ABSORB, 3, stoneGripAbsorbRemoved)
RegisterAuraEvent(SPELL_STONE_GRIP_ABSORB_HEROIC, 3, stoneGripAbsorbRemoved)

-- C++ spell_ulduar_cancel_stone_grip::HandleScript
-- (boss_kologarn.cpp:448-476): OnEffectHitTarget EFFECT_0
-- SPELL_EFFECT_SCRIPT_EFFECT -> the hit unit, if it rides a
-- vehicle, removes the aura whose id is the effect's CalcValue:
-- EFFECT_0 on 10-man, EFFECT_1 on 25-man. Bound 65594 (in-house
-- enum SPELL_STONE_GRIP_CANCEL, boss_kologarn.cpp:46; the
-- spell_script_names row is absent upstream, see header).
-- DORMANT until Spell.dbc ships: 65594 is cast only by the
-- absorb leg's rubble stalker, itself DBC-gated.
local function cancelStoneGrip(event, spell, target, effectIndex)
    if effectIndex ~= 0 or target == nil then
        return
    end
    if target:IsOnVehicle() == nil then
        return
    end
    local caster = spell:GetCaster()
    -- C++ switches on the target's map difficulty
    -- (boss_kologarn.cpp:462-472): 10-man normal removes the EFFECT_0 value,
    -- 25-man normal the EFFECT_1 value, heroic difficulties hit
    -- the default leg and remove nothing (10H=2, 25H=3,
    -- DBCEnums.h:286-289).
    if caster ~= nil and caster:IsHeroic() then
        return
    end
    local valueIndex = 0
    if caster ~= nil and caster:Is25ManRaid() then
        valueIndex = 1
    end
    local value = spell:GetEffectValue(valueIndex)
    if value > 0 then
        target:RemoveAura(value)
    end
end
RegisterSpellEvent(SPELL_STONE_GRIP_CANCEL, 5, cancelStoneGrip)

-- C++ spell_ulduar_rubble_summon::HandleScript
-- (boss_kologarn.cpp:334-356): OnEffectHitTarget EFFECT_0
-- SPELL_EFFECT_SCRIPT_EFFECT -> the caster (rubble stalker)
-- casts GetEffectValue() on itself 5 times with originalCaster
-- = instance GetGuidData(BOSS_KOLOGARN). Bound 63633
-- (SPELL_SUMMON_RUBBLE, boss_kologarn.cpp:47; the stalker
-- casts it at :195, ported above). DORMANT until Spell.dbc
-- ships 63633's effect-0 value (fail-closed 0 read); the
-- originalCaster arg is accepted-and-ignored in Go
-- (lua_creature_events.go), so the 5 casts fire with the boss
-- GUID passed through for a future model.
local function rubbleSummonHandleScript(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local spellId = spell:GetEffectValue(0)
    if spellId == 0 then
        return
    end
    local kologarnGuid = 0
    if caster:GetObjectType() == "Creature" then
        kologarnGuid = caster:GetGuidData(DATA_KOLOGARN)
    end
    for _ = 1, 5 do
        -- TRIGGERED_FULL_MASK: the ObjectGuid CastSpellExtraArgs
        -- ctor sets it (SpellDefines.h:167) — the C++ 3-arg call
        -- resolves through that ctor, so the 5 casts are triggered.
        caster:CastCustomSpell(caster, spellId, true, nil, nil, nil, nil, kologarnGuid)
    end
end
RegisterSpellEvent(SPELL_SUMMON_RUBBLE, 5, rubbleSummonHandleScript)

-- C++ spell_ulduar_stone_grip::OnRemoveStun
-- (boss_kologarn.cpp:568-573): AfterEffectRemove EFFECT_2
-- SPELL_AURA_MOD_STUN, AURA_EFFECT_HANDLE_REAL — the gripped
-- player removes the auras due to the stun effect's amount when
-- the grip aura is removed, whatever the remove mode (no mode
-- gate in C++). The Go ev3 fires once per aura removal, the
-- same granularity; aura:GetAmount(2) reads the removed
-- record's snapshot amounts. Bound 62166/63981 (the two grip
-- variants; the cast-target script branches on 63981).
-- DORMANT until Spell.dbc ships: the amount read is 0 without
-- the DBC row (the fuse-armor fail-closed precedent).
local function stoneGripRemoved(event, aura, target)
    if target == nil or target:GetObjectType() ~= "Player" then
        return
    end
    -- C++ spell_ulduar_stone_grip::OnRemoveVehicle
    -- (boss_kologarn.cpp:575-599): OnEffectRemove EFFECT_0
    -- SPELL_AURA_CONTROL_VEHICLE, AURA_EFFECT_HANDLE_REAL --
    -- PreventDefaultAction() vetoes the default caster-exit +
    -- dummy-aura drop, then the caster exits the target's vehicle
    -- at (1750.0, -7.5 +/- 3.0, 457.9322) and drops its same-spell
    -- aura. The veto is the `return true` below (ev3 veto bridge:
    -- the Go pre-leg fire runs the handler before the default leg
    -- and skips it on a true return). C++ order is exit first,
    -- then the aura strip (RemoveAurasDueToSpell(GetId())), so the
    -- exit below runs before caster:RemoveAura — the exit-then-strip
    -- order the tick-465 handler could not yet express. The exit
    -- position rides the U1 bridges (creature ExitVehicle with
    -- position + Relocate + MoveFall, tick 466); the orientation is
    -- the C++ default Position orientation (0 — the C++ exit
    -- position sets X/Y/Z only). oldPos is read AFTER the exit, so
    -- it is the exit position, C++-exact (:591-595); the
    -- Relocate(exit) / MoveFall / Relocate(oldPos) triple is the
    -- "proper fall movement" leg (:592-595). RemovePendingEventsForPassenger
    -- has no Go model (vehicle events are synchronous) — stays out.
    -- Returning true also vetoes the default leg for the stun leg
    -- below — C++-exact, the default leg only ever exits the caster.
    local caster = aura:GetCaster()
    if caster ~= nil then
        if caster:GetObjectType() == "Creature" then
            local exitY = -7.5 + (math.random() * 6.0 - 3.0)
            caster:ExitVehicle(1750.0, exitY, 457.9322, 0)
            local oldX, oldY, oldZ = caster:GetX(), caster:GetY(), caster:GetZ()
            caster:Relocate(1750.0, exitY, 457.9322, 0)
            caster:MoveFall()
            caster:Relocate(oldX, oldY, oldZ)
        end
        -- A player caster has no ExitVehicle-with-position bridge
        -- (session.exitVehicle takes no position) — the exit leg
        -- stays out for that case, which never occurs in this fight
        -- (the caster is always Kologarn); the aura strip below runs
        -- for either caster kind, the RemoveAurasDueToSpell twin.
        caster:RemoveAura(aura:GetAuraId())
    end
    local amount = aura:GetAmount(2)
    if amount > 0 then
        target:RemoveAura(amount)
    end
    return true
end
RegisterAuraEvent(SPELL_STONE_GRIP, 3, stoneGripRemoved)
RegisterAuraEvent(SPELL_STONE_GRIP_HEROIC, 3, stoneGripRemoved)

-- C++ spell_ulduar_stone_grip_cast_target (boss_kologarn.cpp:
-- 363-440): OnObjectAreaTargetSelect EFFECT_0
-- TARGET_UNIT_SRC_AREA_ENEMY -> drop the main tank (only when
-- the threat list holds more than 1) and every non-player,
-- then trim to a random maxTargets (1 for 62166, 3 for 63981);
-- EFFECT_1/2 refill their lists from the saved one. The Go ev7
-- fire hands each effect its own list in effect order
-- (lua_spell.go fireSpellTargetSelects), so the saved list rides
-- the stoneGripTargets module table keyed by caster GUID. The
-- Load() caster-is-creature gate is structurally satisfied: the
-- Go fire runs only on the creature cast path. DORMANT until
-- Spell.dbc ships: the 62166/63981 casts are DBC-gated.
local function stoneGripTargetSelect(event, spell, effectIndex, targets)
    local caster = spell:GetCaster()
    if caster == nil or caster:GetObjectType() ~= "Creature" then
        return
    end
    local guid = caster:GetGUIDLow()
    if effectIndex == 0 then
        local victim = caster:GetVictim()
        local victimGuid = 0
        if victim ~= nil then
            victimGuid = victim:GetGUID()
        end
        local threatSize = #caster:GetAITargets()
        for _, target in ipairs(targets:GetTargets()) do
            if target:GetObjectType() ~= "Player" then
                targets:Remove(target)
            elseif victimGuid ~= 0 and target:GetGUID() == victimGuid and threatSize > 1 then
                targets:Remove(target)
            end
        end
        local maxTargets = 1
        if spell:GetEntry() == SPELL_STONE_GRIP_HEROIC then
            maxTargets = 3
        end
        while targets:GetCount() > maxTargets do
            local list = targets:GetTargets()
            if #list == 0 then
                break
            end
            targets:Remove(list[math.random(#list)])
        end
        stoneGripTargets[guid] = targets:GetTargets()
    elseif effectIndex == 1 or effectIndex == 2 then
        local saved = stoneGripTargets[guid]
        if saved ~= nil then
            targets:Clear()
            for _, target in ipairs(saved) do
                targets:Add(target)
            end
            if effectIndex == 2 then
                stoneGripTargets[guid] = nil
            end
        end
    end
end
RegisterSpellEvent(SPELL_STONE_GRIP, 7, stoneGripTargetSelect)
RegisterSpellEvent(SPELL_STONE_GRIP_HEROIC, 7, stoneGripTargetSelect)

-- C++ spell_kologarn_stone_shout::FilterTargets
-- (boss_kologarn.cpp:611-640): OnObjectAreaTargetSelect EFFECT_0
-- TARGET_UNIT_SRC_AREA_ENEMY -> keep players and pets, drop every
-- other target (creatures return !IsPet(), anything else is dropped).
-- No Lua IsPet method exists; the in-tree approximation is a nonzero
-- OwnerGUID (hyjal_trash / spell_hunter precedent, lua_creature_events.go:4253)
-- — it over-counts totems/guardians, which C++ drops. Bound 63716
-- (boss enum :43; boss casts it at :275-277, DoCast SPELL_STONE_SHOUT).
-- DORMANT until Spell.dbc ships: the 63716 cast is DBC-gated.
local function stoneShoutFilter(event, spell, effectIndex, targets)
    if effectIndex ~= 0 then
        return
    end
    for _, target in ipairs(targets:GetTargets()) do
        if target == nil then
            -- nothing to keep; the list holds resolved objects only
        elseif target:GetObjectType() == "Player" then
            -- keep
        elseif target:GetObjectType() == "Creature" and target:GetOwnerGUID() ~= 0 then
            -- pet: keep
        else
            targets:Remove(target)
        end
    end
end
RegisterSpellEvent(SPELL_STONE_SHOUT, 7, stoneShoutFilter)

-- C++ spell_kologarn_summon_focused_eyebeam::HandleForceCast
-- (boss_kologarn.cpp:646-668): OnEffectHitTarget EFFECT_0/1
-- SPELL_EFFECT_FORCE_CAST -> PreventHitDefaultEffect, then the caster
-- casts the effect's TriggerSpell on itself, triggered. Bound 63342
-- (boss enum :52; boss casts it at :305-311, DoCast(me, 63342, true)).
-- GetTriggerSpell(effectIndex) is live (lua_spell.go:424); the veto
-- record is threaded through the ev5 fire (lua_spell.go:461), so
-- PreventHitDefaultEffect skips the Go default force-cast arm.
-- DORMANT until Spell.dbc ships 63342's trigger values (fail-closed
-- 0 read: no bogus 0-id cast).
local function summonFocusedEyebeamForceCast(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 and effectIndex ~= 1 then
        return
    end
    spell:PreventHitDefaultEffect(effectIndex)
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local triggerSpell = spell:GetTriggerSpell(effectIndex)
    if triggerSpell ~= 0 then
        caster:CastSpell(caster, triggerSpell, true)
    end
end
RegisterSpellEvent(SPELL_SUMMON_FOCUSED_EYEBEAM, 5, summonFocusedEyebeamForceCast)
