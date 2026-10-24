-- Stalagg (Naxxramas) — Lua port of the npc_stalagg slice of
-- src/server/scripts/Northrend/Naxxramas/boss_thaddius.cpp
-- (npc_stalagg via GetNaxxramasAI<npc_stalaggAI> (ScriptedAI);
-- AddSC_boss_thaddius registers it alongside boss_thaddius,
-- npc_feugen, npc_tesla, the three polarity/magnetic-pull
-- SpellScripts, at_thaddius_entrance and achievement_thaddius_
-- shocking; loader decl 73 / call 268 per
-- northrend_script_loader.cpp — the FIFTEENTH Naxxramas group in
-- AddNorthrendScripts(), immediately after AddSC_boss_gothik()).
-- Entry: 15929 Stalagg (naxxramas.h NPC_STALAGG line 100 —
-- kalecgos pass; instance_naxxramas.cpp binds NPC_STALAGG ->
-- StalaggGUID (line 170) and DATA_STALAGG -> StalaggGUID
-- (line 323); the CreatureScript ScriptName binding is
-- instance-shimmed, the creature_template binding DB-side as usual).
-- Sole-source verified: whole-server-tree grep for "npc_stalagg"
-- hits boss_thaddius.cpp only (loader carries only the decl/call
-- lines); zero sql/ hits. No stalagg lua existed.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 5 OnSpawn, 14 OnHitBySpell, 23 OnReset.
-- Timers via CreateLuaEvent;
-- melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. Victim gating: C++ KilledUnit checks
-- TYPEID_PLAYER — Lua expresses it via victim:GetObjectType()
-- (nalorakk / kelthuzad / gothik / thaddius player-gate precedent).
-- DoCast default is triggered=false: creature:CastSpell(creature,
-- spell) = DoCastSelf (phase_hunter / gluth precedent).
-- Ported arms (C++-exact for all modeled arms):
-- npc_stalagg: JustEngagedWith Talk(SAY_STALAGG_AGGRO 0) (event 1;
-- the thaddius->AI()->DoAction(ACTION_STALAGG_AGGRO) cross-AI leg +
-- the feugen AddThreat(who, 0.0f) leg have no bridge — documented,
-- not wired); UpdateAI powerSurgeTimer — DoCastSelf(SPELL_STALAGG_
-- POWERSURGE 28134), 10s init, urandms(25,30) repeat (math.random
-- 25-30 per amanitar/black_knight/gluth precedent); KilledUnit
-- Talk(SAY_STALAGG_SLAY 1) (event 3 — C++-GATED: victim->
-- GetTypeId() == TYPEID_PLAYER — nalorakk/kelthuzad player-gate
-- variant). All timers cancelled on 2/4/23 (gargolmar precedent).
-- Unmodeled (no bridges — documented, not wired):
-- npc_stalagg: InitializeAI — the tesla-coil GO SetGoState(GO_STATE_
-- ACTIVE) leg is PORTED in stalaggSpawn (ev5 fires on spawn, so the
-- "no Lua event" claim was stale) + powerSurgeTimer init — GO
-- SetGoState exists and the creature-side find is bridged
-- (creature:FindNearestGameObject, 2026-10-23); the ctor
-- SetBoundary(instance->GetBossBoundary(BOSS_THADDIUS)) — no
-- boundary bridge; EnterEvadeMode — thaddius->AI()->DoAction(
-- ACTION_STALAGG_RESET) — no cross-AI bridge; BeginResetEncounter —
-- coil GO GO_STATE_READY + DespawnOrUnsummon(0s, 7_days) — GO-state /
-- despawn legs bridged, but BeginResetEncounter rides the unbridged
-- cross-AI DoAction machine, and ev23 already models the full
-- evade->despawn->respawn cycle whose C++ end state is ACTIVE (fresh
-- AI InitializeAI), so a bare READY on ev23 would strand the coil
-- READY — the transient READY is unobservable; DoAction(ACTION_BEGIN_RESET_ENCOUNTER) —
-- routes to BeginResetEncounter — no bridge; DoAction(
-- ACTION_STALAGG_REVIVED) — the full revive machine (SetFullHealth
-- + STAND + REACT_AGGRESSIVE + NOT_SELECTABLE removal + root
-- release + Talk(EMOTE_FEIGN_REVIVE 4) + refreshBeam + DoZoneInCombat
-- + the !IsEngaged() -> BeginResetEncounter leg) — no
-- zone-combat bridge (stand/react/flag bridges exist since the
-- 2026-10-20 standstate strip; the revive TRIGGER is the unbridged
-- thaddius DoAction machine);
-- DoAction(ACTION_TRANSITION) — KillSelf + the coil CastStop +
-- coil->AI()->Talk(EMOTE_TESLA_OVERLOAD 1) — no cross-AI bridge;
-- DoAction(ACTION_TRANSITION_2) — coil->CastSpell(thaddius,
-- SPELL_SHOCK_VISUAL 28159) via DATA_THADDIUS — no cross-AI bridge;
-- DoAction(ACTION_TRANSITION_3) — coil GO GO_STATE_READY +
-- DespawnOrUnsummon(0s, 7_days) — no bridges; DamageTaken — the
-- feign-death machine (EMOTE_FEIGN_DEATH 3 Talk + thaddius->
-- AI()->DoAction(ACTION_STALAGG_DIED) + NOT_SELECTABLE + auras
-- wipe + REACT_PASSIVE + root + STAND_DEAD + damage = health-1,
-- and the isFeignDeath damage = 0 arm): the Talk(3) + damage =
-- health-1 + damage = 0 legs ARE ported (see stalaggDamageTaken
-- below — event 9 numeric-return rewrite, halazzi precedent); the
-- cross-AI DoAction leg stays out; SetControlled(root) IS ported
-- (creature:SetRooted, lua_creature_events.go) and the
-- stand/react/flag/aura legs ARE ported since the 2026-10-20
-- standstate strip); the latch clears on death (event 4) and ev23
-- restores fresh-spawn state (Go-lifecycle analog of the C++
-- despawn/respawn cycle — see stalaggReset); the
-- C++ ACTION_STALAGG_REVIVED revive machine is unbridged, so a
-- feigned stalagg stays feigned until killed/despawned — partial
-- port, documented; SpellHit — the tesla-beam machine
-- (npc_stalaggAI::SpellHit, boss_thaddius.cpp:602-627) IS ported
-- (see stalaggSpellHit below — event 14 passes the caster as a full
-- creature object, boss_ai.go:2074; the stale "no caster bridge"
-- rationale is retracted). Trigger caveat: nothing in the C++ tree
-- casts SPELL_STALAGG_TESLA_PERIODIC 28098 (whole-src grep finds
-- only the enum decl and the SpellHit check; the tesla coil's
-- UpdateAI is empty), so the arm is faithful but currently unfired;
-- myCoil() (boss_thaddius.cpp:671): first resolution sets the tesla
-- coil REACT_PASSIVE — cross-creature on NPC_TESLA, helper-gated behind
-- the unported DoAction coil legs — documented-only;
-- UpdateAI's isFeignDeath powerSurge hold is modeled via the Lua
-- latch (see powerSurgeTick).

local ENTRY_STALAGG = 15929

local SAY_STALAGG_AGGRO = 0
local SAY_STALAGG_SLAY = 1

local SPELL_STALAGG_POWERSURGE = 28134

local timers = {}

-- Feign-death latch, declared before every function that touches it
-- (powerSurgeTick's isFeignDeath gate reads it).
local stalaggFeign = {}

-- Tesla-beam SpellHit state (boss_thaddius.cpp:602-627,
-- npc_stalaggAI::SpellHit): isOverloading / refreshBeam latches,
-- declared before every function that touches them.
local stalaggOverload = {}
local stalaggRefreshBeam = {}

local SPELL_STALAGG_TESLA_PERIODIC = 28098
local SPELL_STALAGG_CHAIN_VISUAL = 28096
local SPELL_TESLA_SHOCK = 28099
local EMOTE_TESLA_LINK_BREAKS = 0
local OVERLOAD_DISTANCE = 28
local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_IMMUNE_TO_PC = 0x100
local GO_CONS_NOX_TESLA_STALAGG = 181478

-- C++ SelectTarget(SelectTargetMethod::Random): random alive player
-- from the threat list (gerstahn randomPlayerTarget precedent —
-- GetAITargets is the bridged threat list; C++ takes any unit, the
-- in-tree idiom narrows to players, documented). maxDist <= 0 =
-- no distance limit.
local function randomPlayerTarget(creature, maxDist)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:GetObjectType() == "Player" and t:IsAlive() then
            if maxDist <= 0 or creature:GetDistance2d(t) <= maxDist then
                candidates[#candidates + 1] = t
            end
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
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

-- C++ UpdateAI powerSurgeTimer: DoCastSelf(SPELL_STALAGG_POWERSURGE
-- 28134), 10s init, urandms(25,30) repeat. While feigned the C++
-- holds the timer at 0 (powerSurgeTimer = 0u, "delay until potential
-- revive", boss_thaddius.cpp:644) — modeled via the Lua latch: no
-- cast, re-arm on the 1s beat; the revive never fires in this port,
-- so the cast stays suppressed while feigned.
local function powerSurgeTick(creature, guid)
    if stalaggFeign[guid] then
        schedule(guid, "powersurge", 1000, function()
            powerSurgeTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_STALAGG_POWERSURGE)
    schedule(guid, "powersurge", {25000, 30000}, function()
        powerSurgeTick(creature, guid)
    end)
end

-- C++ JustEngagedWith: Talk(SAY_STALAGG_AGGRO 0) + the powerSurge
-- schedule (10s, from InitializeAI). The thaddius cross-AI
-- DoAction(ACTION_STALAGG_AGGRO) + feugen AddThreat legs have no
-- bridge — documented, not wired.
local function stalaggEnterCombat(event, creature, target)
    creature:Talk(SAY_STALAGG_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "powersurge", 10000, function()
        powerSurgeTick(creature, guid)
    end)
end

local function stalaggLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ KilledUnit: Talk(SAY_STALAGG_SLAY 1) only when victim->
-- GetTypeId() == TYPEID_PLAYER (nalorakk / kelthuzad player-gate
-- variant).
local function stalaggTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_STALAGG_SLAY)
    end
end

local function stalaggDied(event, creature, killer)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    stalaggFeign[guid] = nil
    stalaggOverload[guid] = nil
    stalaggRefreshBeam[guid] = nil
end

-- C++ npc_stalaggAI ctor (boss_thaddius.cpp:473-489): refreshBeam =
-- true ("force tesla coil state refresh") alongside the latch inits.
-- Go ev5 fires once per spawn (felmyst trailSpawn precedent).
local function stalaggSpawn(event, creature)
    local guid = creature:GetGUID()
    stalaggOverload[guid] = nil
    stalaggRefreshBeam[guid] = true
    -- C++ InitializeAI (boss_thaddius.cpp:479-481): myCoilGO() =
    -- FindNearestGameObject(GO_CONS_NOX_TESLA_STALAGG, 1000.0f) ->
    -- SetGoState(GO_STATE_ACTIVE); ev5 is the spawn/InitializeAI
    -- analog (this handler already models the ctor latch inits).
    local coil = creature:FindNearestGameObject(GO_CONS_NOX_TESLA_STALAGG, 1000.0)
    if coil then coil:SetGoState(0) end -- GO_STATE_ACTIVE
end

-- UnitDefines.h react states / SharedDefines.h stand states and unit flags
-- (engine/world/lua_creature_events.go SetReactState / SetStandState /
-- SetFlag bridges).
local REACT_PASSIVE = 0
local REACT_AGGRESSIVE = 2
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000
local UNIT_STAND_STATE_STAND = 0
local UNIT_STAND_STATE_DEAD = 7

local function stalaggReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    -- C++ has no Reset override: on evade stalagg runs EnterEvadeMode ->
    -- thaddius DoAction(ACTION_STALAGG_RESET) -> BeginResetEncounter ->
    -- DespawnOrUnsummon (boss_thaddius.cpp:490-501) and is force-respawned
    -- by thaddius with a fresh AI (isFeignDeath=false, template stand /
    -- flags / react state). Go has no despawn model, so ev23 restores the
    -- fresh-spawn equivalent and clears the Lua latch; without this a
    -- feigned stalagg would stay damage-immune across evades.
    stalaggFeign[guid] = nil
    -- Fresh-AI analog of the C++ ctor (see stalaggSpawn): the evade ->
    -- despawn -> force-respawn cycle rebuilds the AI with
    -- isOverloading=false, refreshBeam=true.
    stalaggOverload[guid] = nil
    stalaggRefreshBeam[guid] = true
    creature:SetStandState(UNIT_STAND_STATE_STAND)
    creature:RemoveFlag(59, UNIT_FLAG_NOT_SELECTABLE)
    creature:SetReactState(REACT_AGGRESSIVE)
    creature:SetRooted(false)
end

-- C++ npc_stalagg::DamageTaken (boss_thaddius.cpp:572-602), portable
-- legs: lethal damage -> Talk(EMOTE_FEIGN_DEATH 3) +
-- SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE) +
-- RemoveAllAuras() + SetReactState(REACT_PASSIVE) + AttackStop() +
-- SetStandState(UNIT_STAND_STATE_DEAD) + SetControlled(UNIT_STATE_ROOT)
-- (creature:SetRooted, lua_creature_events.go) + damage = health-1; once latched,
-- all further damage = 0. The cross-AI DoAction(ACTION_STALAGG_DIED)
-- leg stays out; refreshBeam = true
-- IS modeled via the Lua latch (see DamageTaken below). Event 9's
-- second return rewrites damage
-- (fireCreatureDamageTaken; halazzi precedent). The C++ latch resets
-- in the AI ctor; the Lua mirror wipes the GUID entry on death
-- (event 4) and restores fresh-spawn state on reset (event 23, see
-- above). The unbridged ACTION_STALAGG_REVIVED machine means a
-- feigned stalagg never revives mid-fight in this port.
local EMOTE_FEIGN_DEATH = 3

local function stalaggDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    if damage < creature:GetHealth() then
        return false, damage
    end
    if stalaggFeign[guid] then
        return false, 0
    end
    stalaggFeign[guid] = true
    -- C++ DamageTaken feign arm (boss_thaddius.cpp:600): refreshBeam =
    -- true — "force beam refresh as we just removed auras".
    stalaggRefreshBeam[guid] = true
    creature:Talk(EMOTE_FEIGN_DEATH)
    creature:SetFlag(59, UNIT_FLAG_NOT_SELECTABLE)
    creature:RemoveAllAuras()
    creature:SetReactState(REACT_PASSIVE)
    creature:AttackStop()
    creature:SetRooted(true)
    creature:SetStandState(UNIT_STAND_STATE_DEAD)
    return false, creature:GetHealth() - 1
end

-- C++ npc_stalaggAI::SpellHit (boss_thaddius.cpp:602-627), ported
-- C++-exact for every leg. The C++ WorldObject* caster ->
-- creatureCaster = caster->ToCreature() guard is the GetObjectType()
-- gate below. Leg mapping: creatureCaster->SetImmuneToPC(false/true)
-- = caster:RemoveFlag/caster:SetFlag(UNIT_FIELD_FLAGS,
-- UNIT_FLAG_IMMUNE_TO_PC) (thekal / lurker_below precedent — the Go
-- engine honors the 0x100 bit in melee/spell targeting,
-- creaturemotion.go:3952); creatureCaster->AI()->Talk =
-- caster:Talk (handler-surface bridge); creatureCaster->CastStop =
-- no-op, omitted (no cast state in Go, lua_objects.go:344);
-- home-distance via the handler-surface GetHomePosition bridge (3D,
-- OVERLOAD_DISTANCE 28).
local function stalaggSpellHit(event, creature, caster, spellId)
    if spellId ~= SPELL_STALAGG_TESLA_PERIODIC then
        return
    end
    if not caster or caster:GetObjectType() ~= "Creature" then
        return
    end
    local guid = creature:GetGUID()
    local hx, hy, hz = creature:GetHomePosition()
    local dx = creature:GetX() - hx
    local dy = creature:GetY() - hy
    local dz = creature:GetZ() - hz
    local farFromHome = dx * dx + dy * dy + dz * dz >= OVERLOAD_DISTANCE * OVERLOAD_DISTANCE
    if not stalaggFeign[guid] and creature:IsInCombat() and farFromHome then
        if not stalaggOverload[guid] then
            stalaggOverload[guid] = true
            caster:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC)
            caster:Talk(EMOTE_TESLA_LINK_BREAKS)
            creature:RemoveAura(SPELL_STALAGG_CHAIN_VISUAL)
        end
        local target = randomPlayerTarget(creature, 0)
        if target then
            caster:CastSpell(target, SPELL_TESLA_SHOCK, true)
        end
    elseif stalaggOverload[guid] or stalaggRefreshBeam[guid] then
        stalaggOverload[guid] = false
        stalaggRefreshBeam[guid] = false
        caster:CastSpell(creature, SPELL_STALAGG_CHAIN_VISUAL, true)
        caster:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC)
    end
end

RegisterCreatureEvent(ENTRY_STALAGG, 1, stalaggEnterCombat)
RegisterCreatureEvent(ENTRY_STALAGG, 2, stalaggLeaveCombat)
RegisterCreatureEvent(ENTRY_STALAGG, 3, stalaggTargetDied)
RegisterCreatureEvent(ENTRY_STALAGG, 4, stalaggDied)
RegisterCreatureEvent(ENTRY_STALAGG, 5, stalaggSpawn)
RegisterCreatureEvent(ENTRY_STALAGG, 9, stalaggDamageTaken)
RegisterCreatureEvent(ENTRY_STALAGG, 14, stalaggSpellHit)
RegisterCreatureEvent(ENTRY_STALAGG, 23, stalaggReset)
