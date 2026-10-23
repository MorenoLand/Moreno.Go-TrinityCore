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
-- JustEngagedWith (:78-84) — Talk(SAY_AGGRO 0) (:78) unconditional; the
--   three ScheduleEvent seeds: EVENT_QUIVERING_STRIKE at 23000ms (:82),
--   EVENT_IMPENDING_DESPAIR at 9000ms (:83), EVENT_DEFILING_HORROR at
--   {21000,39000}ms randtime (:84). The DoZoneInCombat (:79) and
--   instance->SetBossState(DATA_FALRIC, IN_PROGRESS) (:80) legs have no
--   bridge.
-- KilledUnit (:110) — Talk(SAY_SLAY 1) player-gated, the razuvious
--   player-gated variant precedent.
-- JustDied (:102-104) — Talk(SAY_DEATH 2) unconditional, the sjonnir
--   JustDied-Talk precedent; the events.Reset and instance->SetBossState
--   DONE legs (:103-104) have no bridge.
-- DamageTaken (:89-96) — the hopelessness machine, floored single-flip
--   latch: event 9 fires pre-application, so the post-damage health
--   decides ((GetHealth()-damage), at/below 66 / 33 / 10 like C++
--   HealthBelowPctDamaged — the moroes convention); the OR chain selects
--   exactly one level per hit (:89-91) and the body fires once, so a
--   single massive hit flips only one level — replicated. At flip N>=1
--   (:93-94) the previous aura clears via the RemoveAura bridge (the
--   arlokk / bronjahm idiom) and DoCast(me, HopelessnessHelper[N])
--   (:95) is a non-triggered self-cast.
-- EVENT_QUIVERING_STRIKE (:125-127) — DoCastVictim(SPELL_QUIVERING_STRIKE
--   72422) = victim-cast; re-arm 10000ms unconditional.
-- EVENT_IMPENDING_DESPAIR (:129-135) — SelectTarget(Random, 0, 45.0f,
--   playerOnly): random alive player within 45.0f (the 45.0f distance
--   filter is modeled via IsWithinDist); target found -> Talk(
--   SAY_IMPENDING_DESPAIR 3) (:132) + non-triggered cast 72426 on the
--   target (:133); re-arm 13000ms unconditional (:135, outside the
--   target-if).
-- EVENT_DEFILING_HORROR (:137-139) — DoCastAOE(SPELL_DEFILING_HORROR
--   72435) = self-cast; re-arm {21000,39000}ms randtime unconditional.
--
-- DOCUMENTED-ONLY (no registration beyond entry 38112):
-- ctor / boss_horAI::Reset (boss_horAI.cpp:26-34) — _Reset() passthrough,
--   SetVisible(false), SetImmuneToAll(true) (:30),
--   SetReactState(REACT_PASSIVE), the GetData(DATA_WAVE_COUNT) /
--   ProcessEvent(EVENT_DO_WIPE) wave leg. The flag twin
--   (creature:SetFlag(59, 0x300)) IS bridged, but :30 is the apply half
--   of an instance-sequencer-driven toggle: the clear
--   (DoAction(ACTION_ENTER_COMBAT), boss_horAI.cpp:41, driven by the
--   unported instance wave machine) has no DoAction bridge, so porting
--   the apply alone fabricates a permanently-immune boss the C++ never
--   exhibits — both halves stay out (the 2026-10-23 SetImmuneToAll
--   sweep; same bar as the svalna gauntlet toggle). The old "no bridge
--   by construction" phrasing is retracted: the blocker is the unported
--   toggle pair, not the flag bridge. Initialize (_hopelessnessCount =
--   0, :67) is the per-GUID state seed on enter combat.
-- JustEngagedWith :79 DoZoneInCombat — PORTED 2026-10-23
-- (creature:DoZoneInCombat, in arm order after Talk; bridge live at
-- engine/world/lua_creature_events.go; the old "no bridge" claim was
-- stale) / :80 instance->SetBossState IN_PROGRESS — no bridge.
-- UpdateAI :120 HasUnitState(UNIT_STATE_CASTING) gate — no unit-state
--   bridge; timers fire unconditionally (the maiden precedent; creature
--   casts are packet-visual in Go).
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
-- SelectTarget(Random, 0, 45.0f, true): approximated with a random alive
-- player on the same map/instance within 45.0f (the keleseth / thorim
-- GetPlayersInWorld precedent with the 45.0f distance filter applied).
-- DamageTaken hook is pre-application: post-damage health decides,
-- strictly at/below 66 / 33 / 10 like C++ HealthBelowPctDamaged (the
-- moroes convention).
--
-- UPSTREAM C++ FINDINGS: SAY_DEFILING_HORROR = 4 is declared and never
-- used (see documented-only); the :94 removal resolves via
-- GetSpellIdForDifficulty while the :95 cast does not — if the ids
-- differ per difficulty, the removal targets a different id than the one
-- DoCast applied.

local ENTRY_FALRIC = 38112

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
-- random alive player on the same map/instance within 45.0f (the
-- keleseth / thorim GetPlayersInWorld precedent, plus the 45.0f
-- distance filter).
local function randomPlayer(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local candidates = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() and creature:IsWithinDist(p, 45) then
            candidates[#candidates + 1] = p
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
    creature:CastSpell(nil, SPELL_QUIVERING_STRIKE)
    schedule(guid, "quivering", 10000, function() onQuiveringStrike(creature, guid) end)
end

-- C++ :129-135 EVENT_IMPENDING_DESPAIR: random player within 45.0f;
-- target found -> Talk(SAY_IMPENDING_DESPAIR) + DoCast(target, 72426)
-- non-triggered; re-arm 13000ms unconditional (outside the target-if).
local function onImpendingDespair(creature, guid)
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
    creature:CastSpell(creature, SPELL_DEFILING_HORROR)
    schedule(guid, "horror", {21000, 39000}, function() onDefilingHorror(creature, guid) end)
end

-- C++ :78-84 JustEngagedWith: Talk(SAY_AGGRO) unconditional; the three
-- ScheduleEvent seeds (23000 / 9000 / {21000,39000}ms). The
-- DoZoneInCombat leg (:79) is PORTED 2026-10-23 — creature:DoZoneInCombat
-- in arm order after Talk (bridge live engine/world/lua_creature_events.go);
-- instance->SetBossState IN_PROGRESS (:80) has no bridge.
local function falricEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    initState(guid)
    creature:Talk(SAY_AGGRO)
    creature:DoZoneInCombat() -- C++ :79, in arm order after Talk (2026-10-23)
    schedule(guid, "quivering", 23000, function() onQuiveringStrike(creature, guid) end)
    schedule(guid, "despair", 9000, function() onImpendingDespair(creature, guid) end)
    schedule(guid, "horror", {21000, 39000}, function() onDefilingHorror(creature, guid) end)
end

-- C++ :89-96 DamageTaken: the hopelessness machine — floored
-- single-flip latch (the garfrost phase-latch precedent). Event 9 fires
-- pre-application, so the post-damage health decides, at/below 66 / 33 /
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
-- JustDied-Talk precedent; the events.Reset and instance->SetBossState
-- DONE legs have no bridge.
local function falricJustDied(event, creature)
    clearState(creature:GetGUID())
    creature:Talk(SAY_DEATH)
end

local function falricLeaveCombat(event, creature)
    clearState(creature:GetGUID())
end

local function falricReset(event, creature)
    clearState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_FALRIC, 1, falricEnterCombat)
RegisterCreatureEvent(ENTRY_FALRIC, 2, falricLeaveCombat)
RegisterCreatureEvent(ENTRY_FALRIC, 3, falricKilledUnit)
RegisterCreatureEvent(ENTRY_FALRIC, 4, falricJustDied)
RegisterCreatureEvent(ENTRY_FALRIC, 9, falricDamageTaken)
RegisterCreatureEvent(ENTRY_FALRIC, 23, falricReset)
