-- The Beast (Blackrock Spire; arena encounter) --
-- Lua port of src/server/scripts/EasternKingdoms/BlackrockMountain/
-- BlackrockSpire/boss_the_beast.cpp
-- (boss_the_beast — BossAI combat scheduler). Boss-script unit per
-- eastern_kingdoms_script_loader.cpp order (boss_shadow_hunter_voshgajin
-- done, registered for 9236; AddSC_boss_thebeast next in the
-- Blackrock Spire block).
-- Entry (verifiable from the C++ sources): blackrock_spire.h names
-- NPC_THE_BEAST = 10430 in the BRS creatures enum. The
-- creature_template ScriptName binding is DB-side (no TDB in this
-- workspace), but the entry itself is C++-verifiable so the port is
-- registered (boss_shadow_hunter_voshgajin / boss_overlord_wyrmthalak
-- precedent). GetBlackrockSpireAI is a GetInstanceAI retrieval wrapper
-- only (blackrock_spire.h:129-132). SetBossState legs (DATA_THE_BEAST
-- = 12, blackrock_spire.h:42; 12 < EncounterCount 23, live):
-- ev1 IN_PROGRESS via the base call (boss_the_beast.cpp:150,
-- BossAI::_JustEngagedWith, ScriptedCreature.cpp:537); ev23 NOT_STARTED
-- via Reset's _Reset() (:83, ScriptedCreature.cpp:499-507, alive-gated +
-- state != DONE) — Reset's MovePath arm stays out (no MotionMaster
-- bridge); ev4 DONE via the base BossAI::JustDied — there is NO
-- JustDied override in the file, so the base runs by virtual dispatch
-- (ScriptedCreature.h:330 -> _JustDied(), ScriptedCreature.cpp:513-518).
-- The remaining _Reset()/_JustDied() bookkeeping is covered by the
-- cancel/re-arm on combat events (halycon precedent).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Driven by CreateLuaEvent timers; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++ JustEngagedWith event schedule + UpdateAI event
-- machine):
-- EVENT_FLAME_BREAK: victim-cast FLAMEBREAK 16785, 12s init -> 10s
-- loop (maiden convention — DoCastVictim path, creature:GetVictim()).
-- EVENT_TERRIFYING_ROAR: victim-cast TERRIFYINGROAR 14100, 23s init
-- -> 20s loop (maiden convention).
-- EVENT_FIREBALL: victim-cast FIREBALL 16788, urand(8s, 21s) init ->
-- 8s..21s loop (C++ ScheduleEvent(8s, 21s) / Repeat(8s, 21s), C++-exact
-- — twilight_corrupter precedent).
-- EVENT_FIREBLAST: victim-cast FIREBLAST 16144, urand(5s, 8s) init ->
-- 5s..8s loop (C++ ScheduleEvent(5s, 8s) / Repeat(5s, 8s), C++-exact).
-- EVENT_IMMOLATE: random-player IMMOLATE 15570 — SelectTarget(Random,
-- 0, 100, true) is playerOnly, ported via the threat-list pool
-- (GetAITargets, alive, within 100yd); 3s init -> 8s
-- loop, re-arm unconditional (C++ Repeat sits outside the target
-- check), non-triggered DoCast(target).
-- EVENT_BERSERKER_CHARGE: random-player BERSERKER_CHARGE 16636 —
-- SelectTarget(Random, 0, 38, true) is playerOnly (threat-list pool,
-- within 38yd); 2s init -> urand(15s, 23s) loop, re-arm unconditional,
-- non-triggered DoCast(target).
-- SetData(DATA_BEAST_ROOM / DATA_BEAST_REACHED) — gates on
-- FindNearbyOrcs (GetCreatureListWithEntryInGrid NPC_BLACKHAND_ELITE
-- 10317), ObjectAccessor::GetCreature, MotionMaster::MovePath /
-- MovePoint, BasicEvent scheduling (OrcDeathEvent suicide 6s),
-- SetReactState, and both are only reachable via the area triggers
-- (at_trigger_the_beast_movement AT 2066, at_the_beast_room) and the
-- instance script; no area-trigger / instance / MotionMaster bridges
-- on the Lua surface (coren_direbrew / instance precedent).
-- Documented-only (STAY OUT, re-verified 2026-11-13). SpellHit skinning
-- arm (boss_the_beast.cpp:88-94): loops MAX_SPELL_EFFECTS (:90), fires
-- only if an effect IsEffect(SPELL_EFFECT_SKINNING = 95,
-- SharedDefines.h:906) (:91), then only if !me->IsAlive() (:92, nested
-- INSIDE the effect match — the alive gate does not run on non-skinning
-- hits), then DoCastAOE(SPELL_FINKLE_IS_EINHORN 16710, true) (:93,
-- enum :35, triggered). DoCastAOE is self-cast (UnitAI.h:322:
-- DoCast(nullptr, ...)), so the Lua twin would be
-- creature:CastSpell(creature, 16710, true).
-- Hook IS bridged (CreatureEventOnHitBySpell = 14, runtime.go:379) and
-- the producer IS live: dead motions stay findable (handleEffectSkinning,
-- spells.go:5621, resolves the corpse motion and opens skinning loot),
-- and ev14 fires per landed hit target in the same finishSpellCast
-- (spells.go:19914, 30186-30197) — skinning the dead Beast WOULD fire it.
-- The sole block is the effect gate: fireCreatureLuaEvent
-- (lua_creature_events.go:7597) passes (event, creature, caster,
-- spellId) only — every ev14 fire site passes caster + spellID, no
-- SpellInfo/effect list reaches Lua — and no source-grounded spell-ID
-- substitute exists: C++ gates on effect, not ID; no skinning spell ID
-- appears anywhere in either tree (grep 8613: zero hits), Spell.dbc is
-- absent from the workspace (engine/data/dbc/ holds only dbc.go), so
-- hard-coding an ID would be fabrication. Bridgeable legs, for the
-- record if the effect gate is ever exposed: creature:IsAlive()
-- (object.go:260, lua_objects.go:2192) and CastSpell(target, id[,
-- triggered]) (lua_creature_events.go:949-986); luaMotionCreature has no
-- alive gate (lua_creature_events.go:359). No ev14 registration.
-- CASTING gates ported: C++ UpdateAI pre-loop gate (:166) + in-loop
-- gate (:201) are a 500ms IsCasting defer under each arm's own key
-- (live, lua_creature_events.go:2480; mal_ganis idiom) — a gated arm
-- fires nothing and consumes no re-arm, staying due until the cast
-- ends. The in-loop gate's same-tick co-expiry suppression has no
-- timer-arm model (independent callbacks, AV precedent). The C++
-- scheduler is driven from JustEngagedWith and drained by UpdateAI;
-- the port schedules from OnEnterCombat(1) and cancels on 2/4/23
-- (maiden convention).

local SPELL_FLAMEBREAK = 16785
local SPELL_TERRIFYINGROAR = 14100
local SPELL_FIREBALL = 16788
local SPELL_FIREBLAST = 16144
local SPELL_IMMOLATE = 15570
local SPELL_BERSERKER_CHARGE = 16636

local ENTRY_THE_BEAST = 10430

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

local function onFlameBreak(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "flamebreak", 500, function() onFlameBreak(creature, guid) end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FLAMEBREAK)
    end
    schedule(guid, "flamebreak", 10000, function() onFlameBreak(creature, guid) end)
end

local function onTerrifyingRoar(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "terrifyingroar", 500, function() onTerrifyingRoar(creature, guid) end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_TERRIFYINGROAR)
    end
    schedule(guid, "terrifyingroar", 20000, function() onTerrifyingRoar(creature, guid) end)
end

local function onFireball(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "fireball", 500, function() onFireball(creature, guid) end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FIREBALL)
    end
    schedule(guid, "fireball", 8000 + math.random(0, 13000), function() onFireball(creature, guid) end)
end

local function onFireblast(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "fireblast", 500, function() onFireblast(creature, guid) end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FIREBLAST)
    end
    schedule(guid, "fireblast", 5000 + math.random(0, 3000), function() onFireblast(creature, guid) end)
end

-- C++ SelectTarget(Random, 0, 100/38, true) (boss_the_beast.cpp:178/:187):
-- threat-list players alive within range (UnitAI.h:162-176).
local function randomThreatPlayerInRange(creature, range)
    local candidates = {}
    for _, plr in ipairs(creature:GetAITargets()) do
        if plr:GetObjectType() == "Player" and not plr:IsDead()
                and creature:GetDistance(plr) - creature:GetCombatReach() - 1.5 < range then -- IsWithinCombatRange twin (Unit.cpp:583; player reach 1.5)
            candidates[#candidates + 1] = plr
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EVENT_IMMOLATE: SelectTarget(Random, 0, 100, true) playerOnly,
-- DoCast(target, 15570) non-triggered, 3s init, Repeat(8s) outside the
-- target check (re-arm unconditional).
local function onImmolate(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "immolate", 500, function() onImmolate(creature, guid) end)
        return
    end
    local target = randomThreatPlayerInRange(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_IMMOLATE)
    end
    schedule(guid, "immolate", 8000, function() onImmolate(creature, guid) end)
end

-- C++ EVENT_BERSERKER_CHARGE: SelectTarget(Random, 0, 38, true)
-- playerOnly, DoCast(target, 16636) non-triggered, 2s init,
-- Repeat(15s, 23s) outside the target check (re-arm unconditional).
local function onBerserkerCharge(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "berserkercharge", 500, function() onBerserkerCharge(creature, guid) end)
        return
    end
    local target = randomThreatPlayerInRange(creature, 38)
    if target then
        creature:CastSpell(target, SPELL_BERSERKER_CHARGE)
    end
    schedule(guid, "berserkercharge", 15000 + math.random(0, 8000), function() onBerserkerCharge(creature, guid) end)
end

local function onEnterCombat(_, creature)
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540; boss_the_beast.cpp:150 base call first)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542; boss_the_beast.cpp:150 base call first)
    -- BossAI::_JustEngagedWith (ScriptedCreature.cpp:537): the base call
    -- sets the boss state IN_PROGRESS; DATA_THE_BEAST=12 (blackrock_spire.h:42).
    creature:SetBossState(12, 1)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "flamebreak", 12000, function() onFlameBreak(creature, guid) end)
    schedule(guid, "terrifyingroar", 23000, function() onTerrifyingRoar(creature, guid) end)
    schedule(guid, "fireball", 8000 + math.random(0, 13000), function() onFireball(creature, guid) end)
    schedule(guid, "fireblast", 5000 + math.random(0, 3000), function() onFireblast(creature, guid) end)
    schedule(guid, "immolate", 3000, function() onImmolate(creature, guid) end)
    schedule(guid, "berserkercharge", 2000, function() onBerserkerCharge(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ Reset (boss_the_beast.cpp:81-86): _Reset() bookkeeping (timer
-- cancel above) + the NOT_STARTED write (ScriptedCreature.cpp:499-507:
-- alive-gated, state != DONE). The _beastReached MovePath arm stays
-- out (no MotionMaster bridge). Alive gate is load-bearing: Go fires
-- ev23 on the death path too, and C++ Reset never runs on death.
local function onReset(_, creature)
    cancelTimers(creature:GetGUIDLow())
    if creature:IsAlive() and creature:GetBossState(12) ~= 3 then
        creature:SetBossState(12, 0)
    end
end

-- No JustDied override in boss_the_beast.cpp: the base
-- BossAI::JustDied runs by virtual dispatch (ScriptedCreature.h:330)
-- -> _JustDied() (ScriptedCreature.cpp:513-518) bookkeeping (timer
-- cancel) + SetBossState(DATA_THE_BEAST=12, DONE).
local function onDied(_, creature)
    cancelTimers(creature:GetGUIDLow())
    creature:SetBossState(12, 3)
end

RegisterCreatureEvent(ENTRY_THE_BEAST, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_THE_BEAST, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_THE_BEAST, 4, onDied)
RegisterCreatureEvent(ENTRY_THE_BEAST, 23, onReset)
