-- Feugen (Naxxramas) — Lua port of the npc_feugen slice of
-- src/server/scripts/Northrend/Naxxramas/boss_thaddius.cpp
-- (npc_feugen via GetNaxxramasAI<npc_feugenAI> (ScriptedAI);
-- AddSC_boss_thaddius registers it alongside boss_thaddius,
-- npc_stalagg, npc_tesla, the three polarity/magnetic-pull
-- SpellScripts, at_thaddius_entrance and achievement_thaddius_
-- shocking; loader decl 73 / call 268 per
-- northrend_script_loader.cpp — the FIFTEENTH Naxxramas group in
-- AddNorthrendScripts(), immediately after AddSC_boss_gothik()).
-- Entry: 15930 Feugen (naxxramas.h NPC_FEUGEN line 99 —
-- kalecgos pass; instance_naxxramas.cpp binds NPC_FEUGEN ->
-- FeugenGUID (line 167) and DATA_FEUGEN -> FeugenGUID (line
-- 321); the CreatureScript ScriptName binding is
-- instance-shimmed, the creature_template binding DB-side as usual).
-- Sole-source verified: whole-server-tree grep for "npc_feugen"
-- hits boss_thaddius.cpp only (loader carries only the decl/call
-- lines); zero sql/ hits. No feugen lua existed.
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
-- npc_feugen: JustEngagedWith Talk(SAY_FEUGEN_AGGRO 0) (event 1;
-- the thaddius DoAction(ACTION_FEUGEN_AGGRO) cross-AI leg is PORTED
-- 2026-11-05 in feugenEnterCombat (ev38 dispatch); the stalagg
-- AddThreat(who, 0.0f) leg is PORTED 2026-10-24 in feugenEnterCombat); UpdateAI staticFieldTimer — DoCastSelf(SPELL_FEUGEN_
-- STATICFIELD 28135), 6s init, 6s repeat; KilledUnit Talk(
-- SAY_FEUGEN_SLAY 1) (event 3 — C++-GATED: victim->GetTypeId() ==
-- TYPEID_PLAYER — nalorakk/kelthuzad player-gate variant). The
-- magneticPullTimer DoCastSelf(SPELL_MAGNETIC_PULL 54517) leg is
-- documented-only: the cast itself is C++ (:20s/:20s arm), but the
-- pull's whole effect lives in spell_thaddius_magnetic_pull's OnCast
-- HandleCast (threat-table swap + SPELL_MAGNETIC_PULL_EFFECT 28337),
-- and there is no SpellScript binding bridge (razelikh precedent) —
-- so it is the effect, not the cast, that lacks a bridge (gluth's
-- DoCast(28239/28404)-into-SpellScript documented-only precedent).
-- All timers
-- cancelled on 2/4/23 (gargolmar precedent).
-- Unmodeled (no bridges — documented, not wired):
-- npc_feugen: InitializeAI — the tesla-coil GO SetGoState(GO_STATE_
-- ACTIVE) leg is PORTED in feugenSpawn (ev5 fires on spawn, so the
-- "no Lua event" claim was stale) + timer inits — GO SetGoState
-- exists and the creature-side find is bridged
-- (creature:FindNearestGameObject, 2026-10-23); the ctor
-- SetBoundary(instance->GetBossBoundary(BOSS_THADDIUS)) — no
-- boundary bridge; EnterEvadeMode — thaddius DoAction(
-- ACTION_FEUGEN_RESET) is PORTED 2026-11-05 in feugenLeaveCombat
-- (ev38 dispatch); BeginResetEncounter — coil GO GO_STATE_READY +
-- DespawnOrUnsummon(0s, 7_days) is PORTED 2026-11-05 in feugenDoAction
-- (ev38 ACTION_BEGIN_RESET_ENCOUNTER; the 7-day respawn leg has no
-- bridge — DespawnOrUnsummon takes the delay only; thaddius
-- ResetEncounter re-spawns the pets, also unbridged). ev23 keeps
-- modeling the full evade->despawn->respawn cycle whose C++ end state
-- is ACTIVE (fresh AI InitializeAI), so no bare READY rides ev23 —
-- the transient READY is unobservable; DoAction(
-- ACTION_FEUGEN_REVIVED) — the full revive machine (SetFullHealth
-- + STAND + REACT_AGGRESSIVE + NOT_SELECTABLE removal + root
-- release + Talk(EMOTE_FEIGN_REVIVE 4) + refreshBeam + the stalagg
-- AddThreat victim-steal leg + timer re-init) is PORTED 2026-11-05 in
-- feugenDoAction (ev38 dispatch; SetFullHealth rides
-- SetHealth(GetMaxHealth()); SetInCombatWith has no bridge —
-- kalecgos precedent — the AddThreat(0) leg is ported; the
-- staticField 6s re-seed is ported, the magneticPull 30s re-seed is
-- moot — the cast is documented-only); DoAction(
-- ACTION_TRANSITION) — KillSelf (creature:Kill(creature), black_morass
-- precedent) + the coil CastStop + coil->AI()->
-- Talk(EMOTE_TESLA_OVERLOAD 1) — PORTED 2026-11-05 in feugenDoAction
-- (ev38 dispatch; the thaddius sender chain EVENT_TRANSITION_1 is
-- ported in boss_thaddius.lua); DoAction(
-- ACTION_TRANSITION_2) — coil->CastSpell(thaddius, SPELL_SHOCK_
-- VISUAL 28159) via DATA_THADDIUS — PORTED 2026-11-05 in feugenDoAction
-- (ev38 dispatch); DoAction(
-- ACTION_TRANSITION_3) — coil GO GO_STATE_READY +
-- DespawnOrUnsummon(0s, 7_days) — PORTED 2026-11-05 in feugenDoAction
-- (ev38 dispatch; the 7-day respawn leg has no bridge); DamageTaken — the
-- feign-death machine (EMOTE_FEIGN_DEATH 3 Talk + thaddius->
-- AI()->DoAction(ACTION_FEUGEN_DIED) + NOT_SELECTABLE + auras
-- wipe + REACT_PASSIVE + root + STAND_DEAD + damage = health-1,
-- and the isFeignDeath damage = 0 arm): the Talk(3) + damage =
-- health-1 + damage = 0 legs ARE ported (see feugenDamageTaken
-- below — event 9 numeric-return rewrite, halazzi precedent); the
-- cross-AI DoAction(ACTION_FEUGEN_DIED) leg is PORTED 2026-11-05 in
-- feugenDamageTaken (ev38 dispatch); SetControlled(root) IS ported
-- (creature:SetRooted, lua_creature_events.go) and the
-- stand/react/flag/aura legs ARE ported since the 2026-10-20
-- standstate strip); the latch clears on death (event 4) and ev23
-- restores fresh-spawn state (Go-lifecycle analog of the C++
-- despawn/respawn cycle — see feugenReset); the
-- C++ ACTION_FEUGEN_REVIVED revive machine is PORTED 2026-11-05
-- (see above) — a feigned feugen revives on thaddius's 5s schedule
-- when its twin lives; SpellHit — the tesla-beam machine
-- (npc_feugenAI::SpellHit, boss_thaddius.cpp:853-886) IS ported
-- (see feugenSpellHit below — event 14 passes the caster as a full
-- creature object, boss_ai.go:2074; the stale "no caster bridge"
-- rationale is retracted). Upstream quirk preserved: the C++
-- overload leg removes SPELL_STALAGG_CHAIN_VISUAL (28096) from
-- feugen (boss_thaddius.cpp:870), not the feugen visual — ported
-- as-written. Trigger caveat: nothing in the C++ tree casts
-- SPELL_FEUGEN_TESLA_PERIODIC 28110 (whole-src grep finds only the
-- enum decl and the SpellHit check; the tesla coil's UpdateAI is
-- empty), so the arm is faithful but currently unfired;
-- myCoil() (boss_thaddius.cpp:924): first resolution sets the tesla
-- coil REACT_PASSIVE — PORTED 2026-11-05 in feugenDoAction (the coil
-- resolve in the TRANSITION/TRANSITION_2 legs sets it; idempotent on
-- repeat resolves);
-- UpdateAI's isFeignDeath early-return is modeled via the Lua latch
-- (see staticFieldTick).

local ENTRY_FEUGEN = 15930
local ENTRY_STALAGG = 15929

local SAY_FEUGEN_AGGRO = 0
local SAY_FEUGEN_SLAY = 1

local SPELL_FEUGEN_STATICFIELD = 28135

local timers = {}

-- Feign-death latch, declared before every function that touches it
-- (staticFieldTick's isFeignDeath gate reads it).
local feugenFeign = {}

-- Tesla-beam SpellHit state (boss_thaddius.cpp:853-886,
-- npc_feugenAI::SpellHit): isOverloading / refreshBeam latches,
-- declared before every function that touches them.
local feugenOverload = {}
local feugenRefreshBeam = {}

local SPELL_FEUGEN_TESLA_PERIODIC = 28110
local SPELL_STALAGG_CHAIN_VISUAL = 28096
local SPELL_FEUGEN_CHAIN_VISUAL = 28111
local SPELL_TESLA_SHOCK = 28099
local EMOTE_TESLA_LINK_BREAKS = 0
local EMOTE_TESLA_OVERLOAD = 1
local OVERLOAD_DISTANCE = 28
local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_IMMUNE_TO_PC = 0x100
local GO_CONS_NOX_TESLA_FEUGEN = 181477

local DATA_THADDIUS = 9 -- naxxramas.h NAXData64
local DATA_FEUGEN = 11
local DATA_STALAGG = 12
local ENTRY_TESLA = 16218
local ACTION_BEGIN_RESET_ENCOUNTER = 0
local ACTION_FEUGEN_DIED = 1
local ACTION_FEUGEN_RESET = 3
local ACTION_FEUGEN_AGGRO = 5
local ACTION_FEUGEN_REVIVING_FX = 7
local ACTION_FEUGEN_REVIVED = 9
local ACTION_TRANSITION = 11
local ACTION_TRANSITION_2 = 12
local ACTION_TRANSITION_3 = 13
local SPELL_SHOCK_VISUAL = 28159
local EMOTE_FEIGN_REVIVE = 4

-- C++ SelectTarget(SelectTargetMethod::Random): random alive unit
-- from the threat list (playerOnly defaults false — the old
-- gerstahn player-narrowing idiom deviated here, corrected
-- 2026-10-25: the shock arm's only caller is this helper).
-- maxDist <= 0 = no distance limit.
local function randomThreatTarget(creature, maxDist)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:IsAlive() then
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

-- C++ UpdateAI staticFieldTimer: DoCastSelf(SPELL_FEUGEN_STATICFIELD
-- 28135), 6s init, 6s repeat. C++ UpdateAI returns early while feigned
-- (isFeignDeath, boss_thaddius.cpp:890) — modeled via the Lua latch:
-- no cast, re-arm on the 1s beat; the revive never fires in this port,
-- so the cast stays suppressed while feigned.
local function staticFieldTick(creature, guid)
    if feugenFeign[guid] then
        schedule(guid, "staticfield", 1000, function()
            staticFieldTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_FEUGEN_STATICFIELD)
    schedule(guid, "staticfield", 6000, function()
        staticFieldTick(creature, guid)
    end)
end

-- C++ JustEngagedWith: Talk(SAY_FEUGEN_AGGRO 0) + the static-field
-- schedule (6s, from InitializeAI). The magneticPullTimer (20s)
-- leg is documented-only (see header). The thaddius cross-AI
-- DoAction(ACTION_FEUGEN_AGGRO) + stalagg AddThreat legs have no
-- bridge — documented, not wired.
local function feugenEnterCombat(event, creature, target)
    creature:Talk(SAY_FEUGEN_AGGRO)
    -- C++ JustEngagedWith (boss_thaddius.cpp:811-821): thaddius
    -- DoAction(ACTION_FEUGEN_AGGRO) (:815-816), then the twin
    -- AddThreat(who, 0.0f, stalagg) (:818-820).
    local thaddius = creature:GetInstanceCreature(DATA_THADDIUS)
    if thaddius then
        thaddius:DoAction(ACTION_FEUGEN_AGGRO)
    end
    -- C++ JustEngagedWith :818-820: twin AddThreat(who, 0.0f, stalagg);
    -- the instance-GUID fetch rides the entry-search precedent
    -- (darnavan/krick); gate is C++-literal IsInCombat().
    local stalagg = creature:FindNearestCreature(ENTRY_STALAGG, 500.0)
    if stalagg and not stalagg:IsInCombat() then
        stalagg:AddThreat(target, 0)
    end
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "staticfield", 6000, function()
        staticFieldTick(creature, guid)
    end)
end

local function feugenLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
    -- C++ EnterEvadeMode (boss_thaddius.cpp:736-740): thaddius
    -- DoAction(ACTION_FEUGEN_RESET).
    local thaddius = creature:GetInstanceCreature(DATA_THADDIUS)
    if thaddius then
        thaddius:DoAction(ACTION_FEUGEN_RESET)
    end
end

-- C++ KilledUnit: Talk(SAY_FEUGEN_SLAY 1) only when victim->
-- GetTypeId() == TYPEID_PLAYER (nalorakk / kelthuzad player-gate
-- variant).
local function feugenTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_FEUGEN_SLAY)
    end
end

local function feugenDied(event, creature, killer)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    feugenFeign[guid] = nil
    feugenOverload[guid] = nil
    feugenRefreshBeam[guid] = nil
end

-- C++ npc_feugenAI ctor (boss_thaddius.cpp:718-733): refreshBeam =
-- true ("force tesla coil state refresh") alongside the latch inits.
-- Go ev5 fires once per spawn (felmyst trailSpawn precedent).
local function feugenSpawn(event, creature)
    local guid = creature:GetGUIDLow()
    creature:SetGuidData(DATA_FEUGEN) -- C++ instance OnCreatureCreate latch, faerlina precedent
    feugenOverload[guid] = nil
    feugenRefreshBeam[guid] = true
    -- C++ InitializeAI (boss_thaddius.cpp:724-728): myCoilGO() =
    -- FindNearestGameObject(GO_CONS_NOX_TESLA_FEUGEN, 1000.0f) ->
    -- SetGoState(GO_STATE_ACTIVE); ev5 is the spawn/InitializeAI
    -- analog (this handler already models the ctor latch inits).
    local coil = creature:FindNearestGameObject(GO_CONS_NOX_TESLA_FEUGEN, 1000.0)
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

local function feugenReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    -- C++ has no Reset override: on evade feugen runs EnterEvadeMode ->
    -- thaddius DoAction(ACTION_FEUGEN_RESET) -> BeginResetEncounter ->
    -- DespawnOrUnsummon (boss_thaddius.cpp:736-748) and is force-respawned
    -- by thaddius with a fresh AI (isFeignDeath=false, template stand /
    -- flags / react state). Go has no despawn model, so ev23 restores the
    -- fresh-spawn equivalent and clears the Lua latch; without this a
    -- feigned feugen would stay damage-immune across evades.
    feugenFeign[guid] = nil
    -- Fresh-AI analog of the C++ ctor (see feugenSpawn): the evade ->
    -- despawn -> force-respawn cycle rebuilds the AI with
    -- isOverloading=false, refreshBeam=true.
    feugenOverload[guid] = nil
    feugenRefreshBeam[guid] = true
    creature:SetStandState(UNIT_STAND_STATE_STAND)
    creature:RemoveFlag(59, UNIT_FLAG_NOT_SELECTABLE)
    creature:SetReactState(REACT_AGGRESSIVE)
    creature:SetRooted(false)
end

-- C++ npc_feugen::DamageTaken (boss_thaddius.cpp:823-852), portable
-- legs: lethal damage -> Talk(EMOTE_FEIGN_DEATH 3) +
-- SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE) +
-- RemoveAllAuras() + SetReactState(REACT_PASSIVE) + AttackStop() +
-- SetStandState(UNIT_STAND_STATE_DEAD) + SetControlled(UNIT_STATE_ROOT)
-- (creature:SetRooted, lua_creature_events.go) + damage = health-1; once latched,
-- all further damage = 0. The cross-AI DoAction(ACTION_FEUGEN_DIED)
-- leg is PORTED 2026-11-05 in feugenDamageTaken (ev38 dispatch);
-- refreshBeam = true
-- IS modeled via the Lua latch (see DamageTaken below). Event 9's
-- second return rewrites damage
-- (fireCreatureDamageTaken; halazzi precedent). The C++ latch resets
-- in the AI ctor; the Lua mirror wipes the GUID entry on death
-- (event 4) and restores fresh-spawn state on reset (event 23, see
-- above). The ACTION_FEUGEN_REVIVED machine is PORTED 2026-11-05
-- (feugenDoAction) — a feigned feugen revives on thaddius's 5s
-- schedule when its twin lives.
local EMOTE_FEIGN_DEATH = 3

local function feugenDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUIDLow()
    if damage < creature:GetHealth() then
        return false, damage
    end
    if feugenFeign[guid] then
        return false, 0
    end
    feugenFeign[guid] = true
    -- C++ DamageTaken feign arm (boss_thaddius.cpp:835,851):
    -- isOverloading = false + refreshBeam = true — "force beam
    -- refresh as we just removed auras".
    feugenOverload[guid] = false
    feugenRefreshBeam[guid] = true
    creature:Talk(EMOTE_FEIGN_DEATH)
    -- C++ DamageTaken (boss_thaddius.cpp:836-838): thaddius
    -- DoAction(ACTION_FEUGEN_DIED).
    local thaddius = creature:GetInstanceCreature(DATA_THADDIUS)
    if thaddius then
        thaddius:DoAction(ACTION_FEUGEN_DIED)
    end
    creature:SetFlag(59, UNIT_FLAG_NOT_SELECTABLE)
    creature:RemoveAllAuras()
    creature:SetReactState(REACT_PASSIVE)
    creature:AttackStop()
    creature:SetRooted(true)
    creature:SetStandState(UNIT_STAND_STATE_DEAD)
    return false, creature:GetHealth() - 1
end

-- C++ npc_feugenAI::SpellHit (boss_thaddius.cpp:853-886), ported
-- C++-exact for every leg, including the upstream quirk at :870
-- (the overload leg removes SPELL_STALAGG_CHAIN_VISUAL 28096, not
-- the feugen visual — as-written). Leg mapping is the stalagg
-- mirror: SetImmuneToPC(false/true) = RemoveFlag/SetFlag(
-- UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC) (thekal / lurker_below
-- precedent); caster:Talk; CastStop is the bridged
-- creature:StopSpellCast (lua_creature_events.go:1010) — the
-- :874/:882 legs stay out because 28111 is channeled with NO finite
-- DBC duration (Wowhead 2026-11-05: Channeled flag, duration n/a) —
-- Go stamps a channel only when dur > 0 (boss_ai.go:2994), so no
-- ChannelSpellID can ever be live for the coil to interrupt; the
-- beam ends via the RemoveAura leg above, C++-exact. No-op-safe
-- forms recorded in caststop_tick225_verdicts.md (kept out: dead
-- code in Go's model); home-distance via the handler-surface
-- GetHomePosition bridge (3D, OVERLOAD_DISTANCE 28).
local function feugenSpellHit(event, creature, caster, spellId)
    if spellId ~= SPELL_FEUGEN_TESLA_PERIODIC then
        return
    end
    if not caster or caster:GetObjectType() ~= "Creature" then
        return
    end
    local guid = creature:GetGUIDLow()
    local hx, hy, hz = creature:GetHomePosition()
    local dx = creature:GetX() - hx
    local dy = creature:GetY() - hy
    local dz = creature:GetZ() - hz
    local farFromHome = dx * dx + dy * dy + dz * dz >= OVERLOAD_DISTANCE * OVERLOAD_DISTANCE
    if not feugenFeign[guid] and creature:IsInCombat() and farFromHome then
        if not feugenOverload[guid] then
            feugenOverload[guid] = true
            caster:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC)
            caster:Talk(EMOTE_TESLA_LINK_BREAKS)
            creature:RemoveAura(SPELL_STALAGG_CHAIN_VISUAL)
        end
        local target = randomThreatTarget(creature, 0)
        if target then
            caster:CastSpell(target, SPELL_TESLA_SHOCK, true)
        end
    elseif feugenOverload[guid] or feugenRefreshBeam[guid] then
        feugenOverload[guid] = false
        feugenRefreshBeam[guid] = false
        caster:CastSpell(creature, SPELL_FEUGEN_CHAIN_VISUAL, true)
        caster:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC)
    end
end

-- C++ npc_feugenAI::BeginResetEncounter (boss_thaddius.cpp:742-747):
-- coil GO GO_STATE_READY + DespawnOrUnsummon(0s, 7_days). The 7-day
-- respawn leg has no bridge (DespawnOrUnsummon takes the delay only);
-- thaddius ResetEncounter re-spawns the pets, also unbridged. The Lua
-- latches clear to the fresh-AI ctor state (the C++ despawn/respawn
-- rebuilds the AI).
local function feugenBeginResetEncounter(creature, guid)
    local coilGO = creature:FindNearestGameObject(GO_CONS_NOX_TESLA_FEUGEN, 1000.0)
    if coilGO then
        coilGO:SetGoState(1) -- GO_STATE_READY
    end
    cancelTimers(guid)
    feugenFeign[guid] = nil
    feugenOverload[guid] = nil
    feugenRefreshBeam[guid] = true
    creature:DespawnOrUnsummon(0)
end

-- C++ npc_feugenAI::DoAction (boss_thaddius.cpp:749-809), event 38.
local function feugenDoAction(event, creature, action)
    local guid = creature:GetGUIDLow()
    if action == ACTION_BEGIN_RESET_ENCOUNTER then
        feugenBeginResetEncounter(creature, guid)
    elseif action == ACTION_FEUGEN_REVIVING_FX then
        -- C++: empty case (break) — no-op.
    elseif action == ACTION_FEUGEN_REVIVED then
        if not feugenFeign[guid] then
            return
        end
        creature:SetHealth(creature:GetMaxHealth()) -- SetFullHealth twin
        creature:SetStandState(UNIT_STAND_STATE_STAND)
        creature:SetReactState(REACT_AGGRESSIVE)
        creature:RemoveFlag(59, UNIT_FLAG_NOT_SELECTABLE)
        creature:SetRooted(false)
        creature:Talk(EMOTE_FEIGN_REVIVE)
        feugenFeign[guid] = false
        feugenRefreshBeam[guid] = true
        local stalagg = creature:GetInstanceCreature(DATA_STALAGG)
        if stalagg then
            local victim = stalagg:GetVictim()
            if victim then
                creature:AddThreat(victim, 0)
                -- me->SetInCombatWith(victim) has no bridge (kalecgos precedent).
            end
        end
        -- C++ revive timer re-init (:778-779): staticFieldTimer = 6s
        -- (magneticPullTimer = 30s is moot — the cast is documented-only).
        -- The beat loop is replaced, not left to fire within 1s.
        schedule(guid, "staticfield", 6000, function()
            staticFieldTick(creature, guid)
        end)
    elseif action == ACTION_TRANSITION then
        creature:Kill(creature) -- KillSelf twin (black_morass precedent)
        local coil = creature:FindNearestCreature(ENTRY_TESLA, 1000.0)
        if coil then
            coil:SetReactState(REACT_PASSIVE) -- myCoil() first-resolution leg
            coil:StopSpellCast() -- no-op in Go's channel model (the 28111 channel has no finite DBC duration), ported as-written
            coil:Talk(EMOTE_TESLA_OVERLOAD)
        end
    elseif action == ACTION_TRANSITION_2 then
        local coil = creature:FindNearestCreature(ENTRY_TESLA, 1000.0)
        local thaddius = creature:GetInstanceCreature(DATA_THADDIUS)
        if coil and thaddius then
            coil:SetReactState(REACT_PASSIVE) -- myCoil() first-resolution leg
            coil:CastSpell(thaddius, SPELL_SHOCK_VISUAL)
        end
    elseif action == ACTION_TRANSITION_3 then
        feugenBeginResetEncounter(creature, guid)
    end
end

RegisterCreatureEvent(ENTRY_FEUGEN, 1, feugenEnterCombat)
RegisterCreatureEvent(ENTRY_FEUGEN, 2, feugenLeaveCombat)
RegisterCreatureEvent(ENTRY_FEUGEN, 3, feugenTargetDied)
RegisterCreatureEvent(ENTRY_FEUGEN, 4, feugenDied)
RegisterCreatureEvent(ENTRY_FEUGEN, 5, feugenSpawn)
RegisterCreatureEvent(ENTRY_FEUGEN, 9, feugenDamageTaken)
RegisterCreatureEvent(ENTRY_FEUGEN, 14, feugenSpellHit)
RegisterCreatureEvent(ENTRY_FEUGEN, 23, feugenReset)
RegisterCreatureEvent(ENTRY_FEUGEN, 38, feugenDoAction)
