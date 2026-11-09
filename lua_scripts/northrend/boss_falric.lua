-- Falric (Halls of Reflection) — Lua port of
-- src/server/scripts/Northrend/FrozenHalls/HallsOfReflection/boss_falric.cpp
-- (161 lines incl. license; 1 script — boss_falric (CreatureScript via
-- boss_horAI (custom HallsOfReflection base AI wrapping
-- GetHallsOfReflectionAI), DATA_FALRIC = 0); registered from inside
-- AddSC_boss_falric(); loader decl 168 / call 363 per
-- northrend_script_loader.cpp — the THIRD group of the
-- "// Halls of Reflection" block in AddNorthrendScripts(), immediately
-- after AddSC_halls_of_reflection() (call 362); the call after it is
-- AddSC_boss_marwyn() (call 364) — verified from the loader during the
-- Oct-13 Talk-only port; the checkpoint sequence (halls_of_reflection ->
-- boss_falric) is followed).
-- Entry: 38112 Falric (halls_of_reflection.h NPC_FALRIC, line 71;
-- instance_halls_of_reflection.cpp OnCreatureCreate binds case
-- NPC_FALRIC, line 132) — entry-verifiable, registration proceeds (the
-- nexus_commanders kalecgos precedent); the CreatureScript ScriptName
-- binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for "AddSC_boss_falric"
-- hits boss_falric.cpp only (+ the loader decl/call lines); this clone
-- carries no sql/ tree, so ScriptName bindings are DB-side by
-- construction.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 9 OnDamageTaken, 23 OnReset. Timers via CreateLuaEvent
-- ({min,max} table = randtime range); melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady (:145).
--
-- DOCTRINE RETRACTION (2026-10-13): the earlier port of this file
-- treated the whole EventMap machine as unbridgeable under the
-- "no-timer-bridge" doctrine and ported Talk arms only. That doctrine is
-- disproved and stale — CreateLuaEvent is a real registered bridge
-- (engine/scripting/runtime.go:798), and timer-driven cast arms port on
-- the maiden_of_virtue / moroes / curator / nightbane convention:
-- timers table keyed by creature GUID, cancelled on Eluna events 2
-- (leave combat), 4 (death), 23 (reset); CastSpell(nil, spellId) =
-- victim-cast (bridge falls back to TargetGUID); CastSpell(creature,
-- spellId) = self-cast (the DoCastAOE convention, boss_ai.go DoCastAOE);
-- CastSpell(target, spellId) = explicit-target cast; {min,max} delay =
-- randtime range; TRIGGERED casts pass true as the third CastSpell arg.
-- None of the ported arms here is a TRIGGERED cast (every C++ DoCast is
-- 1-2 arg, non-triggered). The hopelessness machine is a SIMPLE LATCH
-- (DamageTaken flips a per-GUID counter; no summon lists, MoveFollow, or
-- DoAction cross-creature infrastructure), so it ports — the garfrost
-- phase-latch precedent.
--
-- PORTED ARMS (C++ line refs; all C++-exact for the modeled arms):
-- JustEngagedWith (:78-84) — Talk(SAY_AGGRO 0) (:78) unconditional;
--   DoZoneInCombat (:79) PORTED 2026-10-23; instance->SetBossState
--   (DATA_FALRIC 0, IN_PROGRESS) (:80) PORTED 2026-11-05 via
--   creature:SetBossState (lua_creature_events.go:5917, the garfrost
--   precedent) — in C++ arm order, after Talk + DoZoneInCombat and
--   before the timer seeds; the three ScheduleEvent seeds:
--   EVENT_QUIVERING_STRIKE at 23000ms (:82),
--   EVENT_IMPENDING_DESPAIR at 9000ms (:83), EVENT_DEFILING_HORROR at
--   {21000,39000}ms randtime (:84). The old "no bridge" claim for :80
--   is retracted.
-- KilledUnit (:110) — Talk(SAY_SLAY 1) player-gated, the razuvious
--   player-gated variant precedent.
-- JustDied (:102-104) — Talk(SAY_DEATH 2) unconditional, the sjonnir
--   JustDied-Talk precedent; events.Reset (:103) = timer cancellation;
--   instance->SetBossState (DATA_FALRIC 0, DONE) (:104) PORTED
--   2026-11-05 via creature:SetBossState, in C++ order AFTER Talk
--   (unlike garfrost, whose _JustDied runs first). The old "no
--   bridge" claim for :104 is retracted. The SetBossState-override
--   leg (instance_halls_of_reflection.cpp:342-348: ++_waveCount +
--   EVENT_NEXT_WAVE 1min) is PORTED 2026-11-08 (tick 491) via the
--   Go-only signal instanceData:SetData(HOR_SETDATA_NEXT_WAVE = 1000,
--   0) right after the DONE write (creature:SetBossState fires no
--   instance event, so the override leg cannot live in the instance
--   file).
-- DoAction (boss_horAI.cpp:36-48) — ACTION_ENTER_COMBAT (-668001,
--   halls_of_reflection.h:130) PORTED 2026-11-08 (tick 491) via the
--   ev38 receiver: SetImmuneToAll(false), REACT_AGGRESSIVE,
--   DoZoneInCombat. The sender is the instance wave machine (ADD_WAVE
--   at wave 5/10); the apply half lives in falricReset below, so the
--   toggle pair is complete (the 2026-10-23 SetImmuneToAll sweep bar
--   is met — the old "unported instance wave machine" note is stale).
-- Reset (:60-64 + boss_horAI.cpp:26-34) — _Reset() first:
--   SetBossState(DATA_FALRIC 0, NOT_STARTED) only when the stored
--   state != DONE (the bronjahm / cyanigosa precedent: ev23 fires on
--   spawn too, and a DONE boss must stay DONE), PORTED 2026-11-05
--   via creature:SetBossState in a new ev23 leg split from ev2
--   (leave-combat does not call Reset in C++); then Initialize
--   (_hopelessnessCount = 0, :67). The SetVisible(false) /
--   SetImmuneToAll(true) / REACT_PASSIVE legs (:30-32) are PORTED
--   2026-11-08 (tick 491) — all three bridges are live
--   (lua_creature_events.go:5548/:5966/:4229) and the clear half
--   (DoAction ACTION_ENTER_COMBAT, see above) is ported, so the
--   toggle pair is complete. The GetData(DATA_WAVE_COUNT) /
--   ProcessEvent(EVENT_DO_WIPE) leg (:33-34) is PORTED 2026-11-08
--   (tick 491) via instanceData:SetData(DATA_WAVE_COUNT, 0) when the
--   stored wave count is non-zero (the instance event-8 handler
--   runs the wipe).
-- DamageTaken (:89-96) — the hopelessness machine, floored single-flip
--   latch: event 9 fires pre-application, so the post-damage health
--   decides ((GetHealth()-damage), strictly below 66 / 33 / 10 like C++
--   HealthBelowPctDamaged — the moroes convention); the OR chain selects
--   exactly one level per hit (:89-91) and the body fires once, so a
--   single massive hit flips only one level — replicated. At flip N>=1
--   (:93-94) the previous aura clears via the RemoveAura bridge (the
--   arlokk / bronjahm idiom) and DoCast(me, HopelessnessHelper[N])
--   (:95) is a non-triggered self-cast.
-- EVENT_QUIVERING_STRIKE (:125-127) — DoCastVictim(SPELL_QUIVERING_STRIKE
--   72422) = victim-cast; re-arm 10000ms unconditional.
-- EVENT_IMPENDING_DESPAIR (:129-135) — SelectTarget(Random, 0, 45.0f,
--   playerOnly): random alive threat-list player within 45.0f (the
--   45.0f distance filter is the IsWithinCombatRange twin); target found
--   -> Talk(SAY_IMPENDING_DESPAIR 3) (:132) + non-triggered cast 72426
--   on the target (:133); re-arm 13000ms unconditional (:135, outside
--   the target-if).
-- EVENT_DEFILING_HORROR (:137-139) — DoCastAOE(SPELL_DEFILING_HORROR
--   72435) = self-cast; re-arm {21000,39000}ms randtime unconditional.
--
-- DOCUMENTED-ONLY (no registration beyond entry 38112):
-- ctor / boss_horAI::Reset (boss_horAI.cpp:26-34) — the _Reset()
--   passthrough, SetVisible(false), SetImmuneToAll(true) (:30),
--   SetReactState(REACT_PASSIVE) and the GetData(DATA_WAVE_COUNT) /
--   ProcessEvent(EVENT_DO_WIPE) wave leg are all PORTED (see the
--   PORTED ARMS notes above, tick 491); the toggle pair is complete
--   because the clear (DoAction(ACTION_ENTER_COMBAT),
--   boss_horAI.cpp:41, driven by the ported instance wave machine,
--   instance_halls_of_reflection.cpp:605) is ported too. The old
--   "both halves stay out" note (2026-10-23 SetImmuneToAll sweep)
--   is retracted. Initialize (_hopelessnessCount =
--   0, :67) is the per-GUID state seed on enter combat.
-- JustEngagedWith :79 DoZoneInCombat — PORTED 2026-10-23
-- (creature:DoZoneInCombat, in arm order after Talk; bridge live at
-- engine/world/lua_creature_events.go; the old "no bridge" claim was
-- stale) / :80 instance->SetBossState IN_PROGRESS — PORTED 2026-11-05
-- (creature:SetBossState, see the PORTED ARMS note above).
-- UpdateAI :120 HasUnitState(UNIT_STATE_CASTING) gate — PORTED: the
--   gate sits before ExecuteEvent (:123), so all three timer ticks
--   defer 500ms via creature:IsCasting() (lua_creature_events.go:1950)
--   without consuming their re-arm (the mal_ganis idiom); the
--   DamageTaken hopelessness latch is a separate hook and stays
--   ungated per C++. The old "no unit-state bridge; timers fire
--   unconditionally" claim is retracted.
-- UpdateAI :145 DoMeleeAttackIfReady — engine-driven in Go.
-- SAY_DEFILING_HORROR = 4 (:30) is declared but never Talked in the .cpp
--   — upstream finding, nothing to port.
--
-- DEVIATIONS FROM C++ (documented; every other value C++-exact):
-- RemoveOwnedAura leg (:94) resolves the spell via
-- sSpellMgr->GetSpellIdForDifficulty, but the cast leg (:95) uses the
-- plain HopelessnessHelper id — so the aura on the creature is the
-- plain-id aura (72395/72396/72397); RemoveAura(plain id) clears the
-- aura the cast actually applied.
-- SelectTarget(Random, 0, 45.0f, true): random alive threat-list
-- player within 45.0f (GetAITargets is the threat-list bridge; the old
-- GetPlayersInWorld pool could pick instance players outside combat).
-- DamageTaken hook is pre-application: post-damage health decides,
-- strictly below 66 / 33 / 10 like C++ HealthBelowPctDamaged (the
-- moroes convention).
--
-- UPSTREAM C++ FINDINGS: SAY_DEFILING_HORROR = 4 is declared and never
-- used (see documented-only); the :94 removal resolves via
-- GetSpellIdForDifficulty while the :95 cast does not — if the ids
-- differ per difficulty, the removal targets a different id than the one
-- DoCast applied.

local ENTRY_FALRIC = 38112
local DATA_FALRIC = 0 -- halls_of_reflection.h:37
local DATA_WAVE_COUNT = 6 -- halls_of_reflection.h:45
local BOSS_STATE_DONE = 3 -- InstanceScript.h:68-76

local ACTION_ENTER_COMBAT = -668001 -- halls_of_reflection.h:130
local HOR_SETDATA_NEXT_WAVE = 1000 -- Go-only signal, instance_halls_of_reflection.lua

local REACT_PASSIVE = 0 -- UnitDefines.h:340
local REACT_AGGRESSIVE = 2 -- UnitDefines.h:342

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2
local SAY_IMPENDING_DESPAIR = 3

local SPELL_QUIVERING_STRIKE = 72422
local SPELL_IMPENDING_DESPAIR = 72426
local SPELL_DEFILING_HORROR = 72435
-- C++ :51 uint32 const HopelessnessHelper[3] (1-indexed here).
local SPELL_HOPELESSNESS = { 72395, 72396, 72397 }

local timers = {}
local falricState = {}

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

-- C++ :67 Initialize: _hopelessnessCount = 0 (called from the ctor and
-- from Reset).
local function initState(guid)
    cancelTimers(guid)
    falricState[guid] = { hopelessness = 0 }
end

local function clearState(guid)
    cancelTimers(guid)
    falricState[guid] = nil
end

-- C++ :129 SelectTarget(SelectTargetMethod::Random, 0, 45.0f, true) —
-- threat-list players alive within 45.0f (SelectTarget draws the
-- threat list, UnitAI.h; dead targets are offline refs, Go
-- SortedEntries returns them too — same stale-pool class as the
-- Halls of Stone / Forge of Souls strips).
local function randomPlayer(creature)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:GetObjectType() == "Player" and not t:IsDead()
                and creature:GetDistance(t) - creature:GetCombatReach() - 1.5 < 45 then -- IsWithinCombatRange twin (Unit.cpp:583; player reach 1.5, ObjectDefines.h:40)
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ :125-127 EVENT_QUIVERING_STRIKE: DoCastVictim(72422) = victim-cast;
-- re-arm 10000ms unconditional.
local function onQuiveringStrike(creature, guid)
    -- C++ UpdateAI :118/:120: events.Update, then the CASTING return
    -- sits BEFORE ExecuteEvent, so a due strike stays queued — 500ms
    -- IsCasting defer, no re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "quivering", 500, function() onQuiveringStrike(creature, guid) end)
        return
    end
    creature:CastSpell(nil, SPELL_QUIVERING_STRIKE)
    schedule(guid, "quivering", 10000, function() onQuiveringStrike(creature, guid) end)
end

-- C++ :129-135 EVENT_IMPENDING_DESPAIR: random player within 45.0f;
-- target found -> Talk(SAY_IMPENDING_DESPAIR) + DoCast(target, 72426)
-- non-triggered; re-arm 13000ms unconditional (outside the target-if).
local function onImpendingDespair(creature, guid)
    -- C++ UpdateAI :118/:120: pre-ExecuteEvent CASTING return — 500ms
    -- IsCasting defer precedes the target pick and Talk, no re-arm
    -- consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "despair", 500, function() onImpendingDespair(creature, guid) end)
        return
    end
    local target = randomPlayer(creature)
    if target then
        creature:Talk(SAY_IMPENDING_DESPAIR)
        creature:CastSpell(target, SPELL_IMPENDING_DESPAIR)
    end
    schedule(guid, "despair", 13000, function() onImpendingDespair(creature, guid) end)
end

-- C++ :137-139 EVENT_DEFILING_HORROR: DoCastAOE(72435) = self-cast;
-- re-arm {21000,39000}ms randtime unconditional.
local function onDefilingHorror(creature, guid)
    -- C++ UpdateAI :118/:120: pre-ExecuteEvent CASTING return — 500ms
    -- IsCasting defer, no re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "horror", 500, function() onDefilingHorror(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_DEFILING_HORROR)
    schedule(guid, "horror", {21000, 39000}, function() onDefilingHorror(creature, guid) end)
end

-- C++ :78-84 JustEngagedWith: Talk(SAY_AGGRO) unconditional; the three
-- ScheduleEvent seeds (23000 / 9000 / {21000,39000}ms). The
-- DoZoneInCombat leg (:79) is PORTED 2026-10-23 — creature:DoZoneInCombat
-- in arm order after Talk (bridge live engine/world/lua_creature_events.go);
-- instance->SetBossState IN_PROGRESS (:80) is PORTED 2026-11-05 via
-- creature:SetBossState, in C++ arm order after DoZoneInCombat and
-- before the seeds (garfrost precedent).
local function falricEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    initState(guid)
    creature:Talk(SAY_AGGRO)
    creature:DoZoneInCombat() -- C++ :79, in arm order after Talk (2026-10-23)
    creature:SetBossState(DATA_FALRIC, 1) -- C++ :80 IN_PROGRESS
    schedule(guid, "quivering", 23000, function() onQuiveringStrike(creature, guid) end)
    schedule(guid, "despair", 9000, function() onImpendingDespair(creature, guid) end)
    schedule(guid, "horror", {21000, 39000}, function() onDefilingHorror(creature, guid) end)
end

-- C++ :89-96 DamageTaken: the hopelessness machine — floored
-- single-flip latch (the garfrost phase-latch precedent). Event 9 fires
-- pre-application, so the post-damage health decides, strictly below 66 / 33 /
-- 10 like C++ HealthBelowPctDamaged (the moroes convention); the OR
-- chain selects one level per hit. At flip N>=1 the previous aura is
-- removed via the RemoveAura bridge and HopelessnessHelper[N] is cast
-- non-triggered on self.
local function falricDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local st = falricState[guid]
    if not st then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    local postHealth = creature:GetHealth() - damage
    local count = st.hopelessness
    if not ((count < 1 and postHealth < math.floor(maxHealth * 66 / 100))
            or (count < 2 and postHealth < math.floor(maxHealth * 33 / 100))
            or (count < 3 and postHealth < math.floor(maxHealth * 10 / 100))) then
        return
    end
    if count >= 1 then
        creature:RemoveAura(SPELL_HOPELESSNESS[count])
    end
    creature:CastSpell(creature, SPELL_HOPELESSNESS[count + 1])
    st.hopelessness = count + 1
end

-- C++ :106-111 KilledUnit: if (who->GetTypeId() == TYPEID_PLAYER)
-- Talk(SAY_SLAY) — the razuvious player-gated variant precedent.
local function falricKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ :99-104 JustDied: Talk(SAY_DEATH) unconditional — the sjonnir
-- JustDied-Talk precedent; events.Reset (:103) = timer cancellation;
-- instance->SetBossState DONE (:104) PORTED 2026-11-05 via
-- creature:SetBossState, in C++ order AFTER Talk (unlike garfrost,
-- whose _JustDied runs first).
local function falricJustDied(event, creature)
    creature:Talk(SAY_DEATH)
    clearState(creature:GetGUID())
    creature:SetBossState(DATA_FALRIC, BOSS_STATE_DONE)
    -- C++ instance_halls_of_reflection.cpp:342-348 SetBossState-override
    -- leg: ++_waveCount + EVENT_NEXT_WAVE in 1min. creature:SetBossState
    -- fires no instance event, so the leg rides the Go-only
    -- HOR_SETDATA_NEXT_WAVE signal (instance event 8).
    local mapObj = creature:GetMap()
    if mapObj then
        local data = mapObj:GetInstanceData()
        if data then
            data:SetData(HOR_SETDATA_NEXT_WAVE, 0)
        end
    end
end

local function falricLeaveCombat(event, creature)
    clearState(creature:GetGUID())
end

-- C++ boss_horAI::DoAction (boss_horAI.cpp:36-48): ACTION_ENTER_COMBAT
-- — SetImmuneToAll(false), REACT_AGGRESSIVE, DoZoneInCombat. The
-- sender is the instance wave machine (ADD_WAVE at wave 5/10); the
-- apply half lives in falricReset below, so the toggle pair is
-- complete (tick 491).
local function falricDoAction(event, creature, action)
    if action ~= ACTION_ENTER_COMBAT then
        return
    end
    creature:SetImmuneToAll(false)
    creature:SetReactState(REACT_AGGRESSIVE)
    creature:DoZoneInCombat()
end

-- C++ Reset (:60-64 + boss_horAI.cpp:26-34): _Reset() first —
-- SetBossState(DATA_FALRIC 0, NOT_STARTED) only when the stored state
-- != DONE (the bronjahm / cyanigosa precedent: ev23 fires on spawn
-- too, and a DONE boss must stay DONE) — then Initialize
-- (_hopelessnessCount = 0, via clearState). The SetVisible(false) /
-- SetImmuneToAll(true) / REACT_PASSIVE + wave-wipe legs stay out (see
-- header); ev23 is split from ev2 (leave-combat does not call Reset).
local function falricReset(event, creature)
    if creature:GetBossState(DATA_FALRIC) ~= BOSS_STATE_DONE then
        creature:SetBossState(DATA_FALRIC, 0)
    end
    -- C++ boss_horAI.cpp:30-32: SetVisible(false), SetImmuneToAll(true),
    -- REACT_PASSIVE (tick 491; the clear half is falricDoAction above).
    creature:SetVisible(false)
    creature:SetImmuneToAll(true)
    creature:SetReactState(REACT_PASSIVE)
    -- C++ boss_horAI.cpp:33-34: GetData(DATA_WAVE_COUNT) != NOT_STARTED
    -- -> ProcessEvent(EVENT_DO_WIPE); the instance event-8 handler runs
    -- the wipe when it receives DATA_WAVE_COUNT NOT_STARTED with a
    -- non-zero count (instance_halls_of_reflection.lua).
    local mapObj = creature:GetMap()
    if mapObj then
        local data = mapObj:GetInstanceData()
        if data ~= nil and (data.waveCount or 0) ~= 0 then
            data:SetData(DATA_WAVE_COUNT, 0)
        end
    end
    clearState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_FALRIC, 1, falricEnterCombat)
RegisterCreatureEvent(ENTRY_FALRIC, 2, falricLeaveCombat)
RegisterCreatureEvent(ENTRY_FALRIC, 3, falricKilledUnit)
RegisterCreatureEvent(ENTRY_FALRIC, 4, falricJustDied)
RegisterCreatureEvent(ENTRY_FALRIC, 9, falricDamageTaken)
RegisterCreatureEvent(ENTRY_FALRIC, 23, falricReset)
RegisterCreatureEvent(ENTRY_FALRIC, 38, falricDoAction)
