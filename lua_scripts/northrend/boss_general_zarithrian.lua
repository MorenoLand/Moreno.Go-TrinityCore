-- General Zarithrian (Ruby Sanctum) — Lua port of
-- src/server/scripts/Northrend/ChamberOfAspects/RubySanctum/boss_general_zarithrian.cpp
-- (2 CreatureScripts: boss_general_zarithrian (BossAI,
-- DATA_GENERAL_ZARITHRIAN = 1) + npc_onyx_flamecaller (ScriptedAI,
-- NPC_ONYX_FLAMECALLER = 39814); both registered from inside
-- AddSC_boss_general_zarithrian() (line 276); loader decl 192 /
-- call 387 per northrend_script_loader.cpp — the FIFTH group of the
-- "// Ruby Sanctum" block in AddNorthrendScripts(), immediately after
-- AddSC_boss_saviana_ragefire() (call 386) — verified from the
-- loader this run; the checkpoint sequence (saviana -> zarithrian) is
-- followed).
-- Entry: 39746 General Zarithrian (ruby_sanctum.h
-- NPC_GENERAL_ZARITHRIAN, line 72; instance_ruby_sanctum.cpp
-- creatureData binds NPC_GENERAL_ZARITHRIAN ->
-- DATA_GENERAL_ZARITHRIAN (line 52); DATA_GENERAL_ZARITHRIAN = 1
-- (ruby_sanctum.h line 32)) — entry-verifiable, registration proceeds
-- (the nexus_commanders kalecgos precedent); the ScriptName bindings
-- are DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_general_zarithrian" hits boss_general_zarithrian.cpp
-- only (+ the loader decl/call lines); this clone carries no sql/ tree,
-- so ScriptName bindings are DB-side by construction.
-- Eluna creature events: 1 OnEnterCombat, 3 OnKill, 4 OnDied.
-- Ported arms (C++-exact for all modeled arms):
-- boss_general_zarithrian JustEngagedWith — Talk(SAY_AGGRO 0)
-- (event 1 — the auriaya precedent; the BossAI::JustEngagedWith /
-- events.ScheduleEvent(EVENT_CLEAVE / EVENT_INTIDMDATING_ROAR /
-- EVENT_SUMMON_ADDS [/EVENT_SUMMON_ADDS2]) legs ride the maiden
-- CreateLuaEvent convention now (ported 2026-10-13/14).
-- boss_general_zarithrian KilledUnit — Talk(SAY_KILL 1) gated on
-- victim->GetTypeId() == TYPEID_PLAYER (event 3 — the razuvious
-- player-gated variant precedent).
-- boss_general_zarithrian JustDied — _JustDied() SetBossState(DONE) then
-- Talk(SAY_DEATH 3) (event 4 — the sjonnir precedent; PORTED
-- 2026-11-06 via creature:SetBossState, live bridge
-- lua_creature_events.go:5917).
-- DOCUMENTED-ONLY (in this header; only entry 39746 registered):
-- boss_general_zarithrian Reset — PORTED 2026-11-06: _Reset()'s gated
-- SetBossState(NOT_STARTED) + the Saviana/Baltharus-DONE gate
-- (RemoveFlag NOT_SELECTABLE / SetImmuneToPC(false) as RemoveFlag(59,
-- 0x100)) ride creature:GetBossState / RemoveFlag (both live).
-- CanAIAttack stays out (no Lua hook for the CanAIAttack query).
-- boss_general_zarithrian JustSummoned — summons.Summon(summon)
-- add-registration leg (no summon-registry bridge).
-- boss_general_zarithrian EnterEvadeMode — the _DespawnAtEvade()
-- SetBossState(FAIL) leg is PORTED 2026-11-06 on event 2 (ev2 fires
-- only from triggerCreatureEvade, creaturemotion.go:4487); the
-- summons.DespawnAll() leg (no summon-registry bridge) and the
-- DespawnOrUnsummon(0s, 30s) respawn leg (creature:Despawn takes no
-- delay arg and deletes the motion mid-evade) stay out. No ev24
-- handler: BossAI::_JustReachedHome is setActive(false) only
-- (ScriptedCreature.cpp:520-523) and Zarithrian overrides nothing
-- there — no FAIL leg exists on reach-home; ev2 carries FAIL.
-- boss_general_zarithrian UpdateAI — CASTING gates (boss :133
-- pre-loop + :166 in-loop; flamecaller pre-loop-only :233) ported as
-- 500ms IsCasting defer on cleave/roar/adds + blast-nova/lava-gout.
-- 2026-10-14 retraction re-audit:
-- stale "no-timer-bridge" claim RETRACTED (CreateLuaEvent is a real
-- bridge); Talk(SAY_ADDS 2) rides EVENT_SUMMON_ADDS unconditionally
-- (:128-129, Talk then [[fallthrough]]) and is now PORTED on
-- 15s-init / 45s-repeat CreateLuaEvent (addsTick, below — the maiden
-- precedent). 2026-11-06: the stalker CastSpell(SPELL_SUMMON_FLAMECALLER
-- 74398, triggered) legs are PORTED nil-guarded via
-- creature:GetInstanceCreature(DATA_ZARITHRIAN_SPAWN_STALKER_*) (the
-- illidari-council precedent) in addsTick/adds2Tick; EVENT_SUMMON_ADDS2
-- (25-man only, 16s init, no Talk) is PORTED as adds2Tick on
-- creature:Is25ManRaid(). DORMANT in-engine: no Go Ruby Sanctum
-- instance script latches the stalker GUIDs (instance_ruby_sanctum.cpp
-- OnCreatureCreate :106-114 has no Go twin; setInstanceGuidData's only
-- Go caller is Lua SetGuidData), so GetInstanceCreature(8/9) returns
-- nil until a stalker latch lands — the casts then fire with no edit.
-- DoCastSelf(SPELL_INTIMIDATING_ROAR 74384) /
-- DoCastVictim(SPELL_CLEAVE_ARMOR 74367) now ported (roarTick /
-- cleaveTick, 2026-10-13); DoMeleeAttackIfReady has no melee-model
-- bridge.
-- npc_onyx_flamecaller (NPC_ONYX_FLAMECALLER = 39814) — REGISTERED
-- (entry-verifiable, ruby_sanctum.h:73 — the bronjahm
-- not-registered verdict is superseded: C++ AddSC registers the
-- script and a spawned 39814 gets this AI in Go too; dormant
-- in-engine until the stalker summon path lands): JustEngagedWith
-- seeds EVENT_BLAST_NOVA 17s + EVENT_LAVA_GOUT 3s (ported below);
-- BLAST_NOVA 74392 — DoCastAOE = self-cast (non-triggered),
-- Repeat(15s, 20s); LAVA_GOUT 74394 — DoCastVictim (non-triggered)
-- with the 3-cast latch (_lavaGoutCount >= 3 -> reset + 8s re-arm;
-- else cast, count++, 1s re-arm); zero Talk lines; Reset's
-- MoveAlongSplineChain (POINT_GENERAL_ROOM) (no-motion-master
-- bridge), MovementInform DoZoneInCombat (no-movement-inform
-- bridge), IsSummonedBy zarithrian->AI()->JustSummoned(me) (no
-- cross-AI-summon bridge) stay out.
-- All 4 Talk() calls in the file accounted for and ported
-- (SAY_AGGRO 0 + SAY_KILL 1 + SAY_ADDS 2 + SAY_DEATH 3).

-- 2026-11-06 U6 (tick 270) SetBossState port — DATA verified from
-- ruby_sanctum.h (DATA_GENERAL_ZARITHRIAN = 1, line 32; EncounterState
-- NOT_STARTED 0 / IN_PROGRESS 1 / FAIL 2 / DONE 3, InstanceScript.h:68-76):
-- ev1 SetBossState(IN_PROGRESS) base-first (ScriptedCreature.cpp:537);
-- ev4 _JustDied SetBossState(DONE) before Talk (ScriptedCreature.cpp:519);
-- ev23 _Reset gated SetBossState(NOT_STARTED) + SetCombatPulseDelay(0) +
-- the Saviana/Baltharus-DONE RemoveFlag gate; ev2 _DespawnAtEvade
-- SetBossState(FAIL) (ScriptedCreature.cpp:617). NO ev24 handler: no
-- reach-home state leg exists in C++ (see EnterEvadeMode note above).
-- Bridge re-checks this run: SetBossState/GetBossState live
-- (lua_creature_events.go:5917/5936); GetInstanceCreature live (:5986)
-- with :CastSpell(target, spell, triggered) (:883); Is25ManRaid live
-- (:3651); RemoveFlag(59, ...) live (:5445); SetImmuneToPC has NO
-- method — ported as RemoveFlag(59, 0x100) per Unit.cpp:8453.
-- Stale claims retracted: '_JustDied() instance leg has no bridge',
-- 'Reset legs ride no instance bridge', 'stalker casts have no instance
-- bridge' (bridges live; stalker casts dormant only for lack of a Go
-- stalker-GUID latch — see retraction note). Stay-outs re-verified:
-- CanAIAttack (no Lua hook); JustSummoned summons.Summon (no
-- summon-registry bridge); flamecaller MoveToGeneral / MovementInform /
-- IsSummonedBy (no motion-master / movement-inform / cross-AI bridges);
-- DoMeleeAttackIfReady (no melee-model bridge).
-- Combat-arm re-verification this run: cleave (DoCastVictim 74367,
-- non-triggered, 8s/15s fixed) ✓; roar (DoCastSelf 74384,
-- non-triggered, 14s/Repeat 35-40s) ✓; blast-nova (DoCastAOE 74392
-- self-cast, non-triggered, 17s/Repeat 15-20s) ✓; lava-gout
-- (DoCastVictim 74394, non-triggered, 3-cast latch, 3s/1s/8s) ✓.
-- One real deviation fixed: addsTick Talked SAY_ADDS before the
-- IsCasting check, double-firing the line on a mid-cast tick — C++
-- executes the event (Talk included) only when not casting, so the
-- check now precedes Talk (the saviana enrageTick ordering).

local ENTRY_GENERAL_ZARITHRIAN = 39746
local ENTRY_ONYX_FLAMECALLER = 39814

local SAY_AGGRO = 0
local SAY_KILL  = 1
local SAY_ADDS  = 2
local SAY_DEATH = 3

local SPELL_CLEAVE_ARMOR    = 74367
local SPELL_INTIMIDATING_ROAR = 74384
local SPELL_BLAST_NOVA      = 74392
local SPELL_LAVA_GOUT       = 74394
local SPELL_SUMMON_FLAMECALLER = 74398

-- ruby_sanctum.h RSDataTypes (EncounterState, InstanceScript.h:68-76:
-- NOT_STARTED 0, IN_PROGRESS 1, FAIL 2, DONE 3).
local DATA_BALTHARUS_THE_WARBORN      = 0
local DATA_GENERAL_ZARITHRIAN         = 1
local DATA_SAVIANA_RAGEFIRE           = 2
local DATA_ZARITHRIAN_SPAWN_STALKER_1 = 8
local DATA_ZARITHRIAN_SPAWN_STALKER_2 = 9

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

-- C++ EVENT_CLEAVE: DoCastVictim(SPELL_CLEAVE_ARMOR 74367)
-- (non-triggered), 8s init, re-ScheduleEvent(15s) (fixed, not Repeat).
local function cleaveTick(creature, guid)
    creature:CastSpell(nil, SPELL_CLEAVE_ARMOR)
    if creature:IsCasting() then
        schedule(guid, "cleave", 500, function()
            cleaveTick(creature, guid)
        end)
        return
    end
    schedule(guid, "cleave", 15000, function()
        cleaveTick(creature, guid)
    end)
end

-- C++ EVENT_INTIDMDATING_ROAR: DoCastSelf(SPELL_INTIMIDATING_ROAR 74384)
-- (non-triggered), 14s init, Repeat(35s, 40s).
local function intimidatingRoarTick(creature, guid)
    creature:CastSpell(creature, SPELL_INTIMIDATING_ROAR)
    if creature:IsCasting() then
        schedule(guid, "roar", 500, function()
            intimidatingRoarTick(creature, guid)
        end)
        return
    end
    schedule(guid, "roar", {35000, 40000}, function()
        intimidatingRoarTick(creature, guid)
    end)
end

-- C++ EVENT_SUMMON_ADDS (:141-151): Talk(SAY_ADDS 2) then [[fallthrough]]
-- into the stalker summon legs; 15s init (JustEngagedWith :97),
-- Repeat(45s) (:151). The stalker CastSpell(SPELL_SUMMON_FLAMECALLER
-- 74398, triggered) legs are PORTED nil-guarded via GetInstanceCreature
-- (the illidari-council precedent); dormant until the stalker GUIDs are
-- latched in Go (see header). Talk moved AFTER the IsCasting check
-- 2026-11-06: C++ executes the event (Talk included) only when not
-- casting, so Talk-before-check double-fired SAY_ADDS on a mid-cast tick.
local function castStalkerFlamecallers(creature)
    local stalker1 = creature:GetInstanceCreature(DATA_ZARITHRIAN_SPAWN_STALKER_1)
    if stalker1 ~= nil then
        stalker1:CastSpell(stalker1, SPELL_SUMMON_FLAMECALLER, true)
    end
    local stalker2 = creature:GetInstanceCreature(DATA_ZARITHRIAN_SPAWN_STALKER_2)
    if stalker2 ~= nil then
        stalker2:CastSpell(stalker2, SPELL_SUMMON_FLAMECALLER, true)
    end
end

local function addsTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "adds", 500, function()
            addsTick(creature, guid)
        end)
        return
    end
    creature:Talk(SAY_ADDS)
    castStalkerFlamecallers(creature)
    schedule(guid, "adds", 45000, function()
        addsTick(creature, guid)
    end)
end

-- C++ EVENT_SUMMON_ADDS2 (:147-151): 25-man only (16s init, JustEngagedWith
-- :99-100), no Talk — same stalker casts, own Repeat(45s).
local function adds2Tick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "adds2", 500, function()
            adds2Tick(creature, guid)
        end)
        return
    end
    castStalkerFlamecallers(creature)
    schedule(guid, "adds2", 45000, function()
        adds2Tick(creature, guid)
    end)
end

-- C++ boss_general_zarithrian::JustEngagedWith:
-- BossAI::JustEngagedWith(who); Talk(SAY_AGGRO);
-- events.ScheduleEvent(EVENT_CLEAVE, 8s);
-- events.ScheduleEvent(EVENT_INTIDMDATING_ROAR, 14s);
-- events.ScheduleEvent(EVENT_SUMMON_ADDS, 15s) (:97); [+ 16s 25-man
-- EVENT_SUMMON_ADDS2] — the auriaya precedent; the cleave/roar
-- timer legs ride the maiden CreateLuaEvent convention now (see
-- below). The EVENT_SUMMON_ADDS stalker-triggered 74398 casts are
-- ported nil-guarded via GetInstanceCreature (addsTick, below);
-- the Talk(SAY_ADDS 2) arm is now ported (addsTick, below).
local function zarithrianEnterCombat(event, creature, target)
    -- C++ JustEngagedWith base call first (boss_general_zarithrian.cpp:93):
    -- BossAI::JustEngagedWith arms (ScriptedCreature.cpp:527-539) —
    -- SetBossState(IN_PROGRESS) + SetCombatPulseDelay(5) + me-form
    -- DoZoneInCombat, base-first (C++ arm order); CheckRequiredBosses /
    -- setActive / ScheduleTasks legs out (no bridges).
    creature:SetBossState(DATA_GENERAL_ZARITHRIAN, 1) -- IN_PROGRESS
    creature:SetCombatPulseDelay(5)
    creature:DoZoneInCombat()
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "cleave", 8000, function()
        cleaveTick(creature, guid)
    end)
    schedule(guid, "roar", 14000, function()
        intimidatingRoarTick(creature, guid)
    end)
    schedule(guid, "adds", 15000, function()
        addsTick(creature, guid)
    end)
    -- C++ JustEngagedWith :99-100: EVENT_SUMMON_ADDS2, 16s, 25-man only.
    if creature:Is25ManRaid() then
        schedule(guid, "adds2", 16000, function()
            adds2Tick(creature, guid)
        end)
    end
end

local function zarithrianLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
    -- C++ EnterEvadeMode (boss_general_zarithrian.cpp:120-124):
    -- summons.DespawnAll() (no summon-registry bridge) +
    -- _DespawnAtEvade() -> instance->SetBossState(FAIL)
    -- (ScriptedCreature.cpp:596-617). Event 2 fires only from
    -- triggerCreatureEvade (creaturemotion.go:4487), after event 23,
    -- so the ev23 NOT_STARTED arm lands first and FAIL is final.
    creature:SetBossState(DATA_GENERAL_ZARITHRIAN, 2) -- FAIL
end

-- C++ boss_general_zarithrian::Reset (:80-88): _Reset() first, then the
-- Saviana/Baltharus-DONE gate. Go fires event 23 on evade, on the death
-- path (before event 4) and on spawn/respawn; the death path returns
-- early like the moroes precedent (C++ _Reset returns early when the
-- boss is dead, ScriptedCreature.cpp:499-501).
local function zarithrianReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    if creature:IsDead() then
        return
    end
    -- C++ _Reset (ScriptedCreature.cpp:499-510): SetCombatPulseDelay(0),
    -- events.Reset / summons.DespawnAll / scheduler.CancelAll (timers
    -- above; the summon-registry leg has no bridge), then SetBossState
    -- NOT_STARTED unless already DONE.
    creature:SetCombatPulseDelay(0)
    if creature:GetBossState(DATA_GENERAL_ZARITHRIAN) ~= 3 then
        creature:SetBossState(DATA_GENERAL_ZARITHRIAN, 0) -- NOT_STARTED
    end
    -- C++ Reset gate (:84-87): once Saviana Ragefire and Baltharus are
    -- both DONE, the boss becomes selectable and attackable.
    if creature:GetBossState(DATA_SAVIANA_RAGEFIRE) == 3
        and creature:GetBossState(DATA_BALTHARUS_THE_WARBORN) == 3 then
        creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
        -- SetImmuneToPC(false) = RemoveFlag(UNIT_FIELD_FLAGS, 0x100)
        -- (Unit.cpp:8453-8457; UNIT_FLAG_IMMUNE_TO_PC, UnitDefines.h:132).
        creature:RemoveFlag(UNIT_FIELD_FLAGS, 0x100)
    end
end

-- C++ boss_general_zarithrian::KilledUnit:
-- if (victim->GetTypeId() == TYPEID_PLAYER)
--     Talk(SAY_KILL) — the razuvious player-gated variant precedent.
local function zarithrianKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

-- C++ boss_general_zarithrian::JustDied (:115-118):
-- _JustDied(); Talk(SAY_DEATH) — the sjonnir precedent. _JustDied()
-- (ScriptedCreature.cpp:513-519) sets SetBossState(DONE) first;
-- PORTED 2026-11-06 via creature:SetBossState (live bridge).
local function zarithrianDied(event, creature, killer)
    cancelTimers(creature:GetGUIDLow())
    creature:SetBossState(DATA_GENERAL_ZARITHRIAN, 3) -- DONE
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_GENERAL_ZARITHRIAN, 1, zarithrianEnterCombat)
RegisterCreatureEvent(ENTRY_GENERAL_ZARITHRIAN, 2, zarithrianLeaveCombat)
RegisterCreatureEvent(ENTRY_GENERAL_ZARITHRIAN, 3, zarithrianKilledUnit)
RegisterCreatureEvent(ENTRY_GENERAL_ZARITHRIAN, 4, zarithrianDied)
RegisterCreatureEvent(ENTRY_GENERAL_ZARITHRIAN, 23, zarithrianReset)

-- npc_onyx_flamecaller (C++-exact for all bridgeable arms): the
-- entry-verifiable (39814, ruby_sanctum.h:73) add AI is REGISTERED —
-- the "not registered because it never spawns (bronjahm precedent)"
-- verdict is superseded by the strip rule that hunts
-- unregistered-but-ported scripts; C++ AddSC registers it and a
-- spawned 39814 gets this AI in Go too. Dormant in-engine until the
-- stalker summon path lands (unchanged). Zero Talk lines in C++.
local flamecallerState = {}

local function flamecallerCancel(guid)
    cancelTimers(guid)
    flamecallerState[guid] = nil
end

-- C++ EVENT_BLAST_NOVA (:226-229): DoCastAOE(SPELL_BLAST_NOVA 74392)
-- non-triggered self-cast; Repeat(15s, 20s).
local function flamecallerBlastNovaTick(creature, guid)
    creature:CastSpell(creature, SPELL_BLAST_NOVA)
    if creature:IsCasting() then
        schedule(guid, "blast_nova", 500, function()
            flamecallerBlastNovaTick(creature, guid)
        end)
        return
    end
    schedule(guid, "blast_nova", {15000, 20000}, function()
        flamecallerBlastNovaTick(creature, guid)
    end)
end

-- C++ EVENT_LAVA_GOUT (:246-256): _lavaGoutCount >= 3 -> count = 0 +
-- Repeat(8s); else DoCastVictim(SPELL_LAVA_GOUT 74394) non-triggered,
-- count++, Repeat(1s).
local function flamecallerLavaGoutTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "lava_gout", 500, function()
            flamecallerLavaGoutTick(creature, guid)
        end)
        return
    end
    local st = flamecallerState[guid]
    if st and st.lavaGout >= 3 then
        st.lavaGout = 0
        schedule(guid, "lava_gout", 8000, function()
            flamecallerLavaGoutTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_LAVA_GOUT)
    if st then
        st.lavaGout = st.lavaGout + 1
    end
    schedule(guid, "lava_gout", 1000, function()
        flamecallerLavaGoutTick(creature, guid)
    end)
end

-- C++ npc_onyx_flamecaller::JustEngagedWith (:200-204):
-- ScheduleEvent(EVENT_BLAST_NOVA, 17s) + ScheduleEvent(EVENT_LAVA_GOUT,
-- 3s). Reset (:195-199): _events.Reset() + _lavaGoutCount = 0 (the
-- motion leg has no bridge; the REACT_DEFENSIVE arm is PORTED
-- 2026-10-20 — creature:SetReactState).
local function flamecallerEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    flamecallerState[guid] = { lavaGout = 0 }
    schedule(guid, "blast_nova", 17000, function()
        flamecallerBlastNovaTick(creature, guid)
    end)
    schedule(guid, "lava_gout", 3000, function()
        flamecallerLavaGoutTick(creature, guid)
    end)
end

local function flamecallerLeaveCombat(event, creature)
    flamecallerCancel(creature:GetGUIDLow())
end

local function flamecallerDied(event, creature, killer)
    flamecallerCancel(creature:GetGUIDLow())
end

local function flamecallerReset(event, creature)
    flamecallerCancel(creature:GetGUIDLow())
    -- C++ boss_general_zarithrian.cpp:193: me->SetReactState(REACT_DEFENSIVE)
    -- (npc_onyx_flamecaller Reset: DEFENSIVE is the permanent baseline).
    creature:SetReactState(1) -- REACT_DEFENSIVE
end

RegisterCreatureEvent(ENTRY_ONYX_FLAMECALLER, 1, flamecallerEnterCombat)
RegisterCreatureEvent(ENTRY_ONYX_FLAMECALLER, 2, flamecallerLeaveCombat)
RegisterCreatureEvent(ENTRY_ONYX_FLAMECALLER, 4, flamecallerDied)
RegisterCreatureEvent(ENTRY_ONYX_FLAMECALLER, 23, flamecallerReset)
