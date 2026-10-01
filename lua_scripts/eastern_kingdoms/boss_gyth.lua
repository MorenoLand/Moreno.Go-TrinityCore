-- Gyth (Blackrock Spire; arena of Blackhand — Rend Blackhand's mount) --
-- Lua port of src/server/scripts/EasternKingdoms/BlackrockMountain/
-- BlackrockSpire/boss_gyth.cpp
-- (boss_gyth — BossAI combat scheduler, the only CreatureScript class in
-- the file; via GetBlackrockSpireAI). Boss-script unit per
-- eastern_kingdoms_script_loader.cpp order (AddSC_boss_gyth follows
-- AddSC_boss_pyroguardemberseer in the Blackrock Spire block).
-- Entry (verifiable from the C++ sources): blackrock_spire.h names
-- NPC_GYTH = 10339 in the BRS creatures enum (BRSDataTypes
-- DATA_GYTH = 11). The creature_template ScriptName binding is DB-side
-- (no TDB in this workspace), but the entry itself is C++-verifiable
-- so the port is registered (pyroguard_emberseer / quartermaster_zigris
-- precedent). GetBlackrockSpireAI is a GetInstanceAI retrieval wrapper
-- only (blackrock_spire.h:129-132).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 9 OnDamageTaken, 23 OnReset. Driven by CreateLuaEvent timers; melee
-- is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
-- Ported arms (C++ JustEngagedWith event schedule + UpdateAI combat
-- event machine):
-- EVENT_CORROSIVE_ACID: self-cast CORROSIVE_ACID 16359, 8s init -> 10s
-- loop (DoCast(me) path — creature:CastSpell(creature, spell),
-- pyroguard_emberseer convention; C++ urand(8s,16s) init / urand(10s,
-- 16s) loop, recurring-timer convention uses the lower bound).
-- EVENT_FREEZE: self-cast FREEZE 16350, 8s init -> 10s loop.
-- EVENT_FLAME_BREATH: self-cast FLAMEBREATH 16390, 8s init -> 10s loop.
-- EVENT_KNOCK_AWAY: victim-cast KNOCK_AWAY 10101, 12s init -> 14s loop
-- (C++ urand(12s,18s) init / urand(14s,20s) loop).
-- DamageTaken: HealthBelowPct(5) one-shot -> self-cast SPELL_SUMMON_REND
-- 16328 (DoCast(me, 16328)); C++-exact condition, carried as the cast
-- arm only — me->RemoveAura(SPELL_REND_MOUNTS 16167) has no bridge on
-- the Lua surface. Ported via OnDamageTaken(9) with a per-guid
-- one-shot latch reset on OnEnterCombat(1) (mother_smolderweb /
-- magmus latch convention); C++ checks the 5% latch in UpdateAI even
-- out of combat, but the only state transition into sub-5% is damage,
-- so the damage-event latch is faithful.
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (creature
-- casts are packet-visual), so timers fire unconditionally and both
-- in-loop casting gates are dropped (maiden precedent). The C++
-- scheduler is driven from JustEngagedWith and drained by UpdateAI;
-- the port schedules from OnEnterCombat(1) and cancels on 2/4/23
-- (maiden convention).
-- Unmodeled (documented-only): Reset's instance arm
-- (instance->GetBossState(DATA_GYTH) == IN_PROGRESS ->
-- SetBossState(DATA_GYTH, DONE) + me->DespawnOrUnsummon()) — no
-- instance bridge on the Lua surface (instance precedent);
-- Reset's _Reset() and JustDied's _JustDied() are internal BossAI
-- machinery covered by the cancel/re-arm on combat events (halycon
-- precedent); JustDied's instance->SetBossState(DATA_GYTH, DONE) —
-- instance precedent, unmodeled. SetData(type, 1) -> EVENT_SUMMONED_1
-- 1s (self-cast aura SPELL_REND_MOUNTS 16167 (AddAura), FindNearestGame
-- Object(GO_DR_PORTCULLIS 175186-ish, 40.0f)->UseDoorOrButton,
-- FindNearestCreature(NPC_LORD_VICTOR_NEFARIUS, 75.0f)->AI()->SetData(1,
-- 1), then EVENT_SUMMONED_2 2s -> MovePath(GYTH_PATH_1 1379681)) — the
-- whole pre-fight event gates on gameobject / creature-list /
-- MotionMaster bridges absent from the Lua surface (the_beast
-- SetData precedent). SPELL_REND_MOUNTS 16167, NEFARIUS_PATH_2/3
-- 1379671/1379672 and GYTH_PATH_1 1379681 are enum-only here
-- (no bridge consumes them).

local ENTRY_GYTH = 10339

local SPELL_CORROSIVE_ACID = 16359
local SPELL_FLAMEBREATH = 16390
local SPELL_FREEZE = 16350
local SPELL_KNOCK_AWAY = 10101
local SPELL_SUMMON_REND = 16328

local timers = {}
local rendFired = {}

local function schedule(guid, key, ms, fn)
    if timers[guid] then
        if timers[guid][key] then
            timers[guid][key]:Cancel()
        end
    else
        timers[guid] = {}
    end
    timers[guid][key] = CreateLuaEvent(fn, ms)
end

local function cancelTimers(guid)
    if timers[guid] then
        for _, ev in pairs(timers[guid]) do
            ev:Cancel()
        end
        timers[guid] = nil
    end
end

local function onCorrosiveAcid(creature, guid)
    creature:CastSpell(creature, SPELL_CORROSIVE_ACID)
    schedule(guid, "acid", 10000, function() onCorrosiveAcid(creature, guid) end)
end

local function onFreeze(creature, guid)
    creature:CastSpell(creature, SPELL_FREEZE)
    schedule(guid, "freeze", 10000, function() onFreeze(creature, guid) end)
end

local function onFlameBreath(creature, guid)
    creature:CastSpell(creature, SPELL_FLAMEBREATH)
    schedule(guid, "flame", 10000, function() onFlameBreath(creature, guid) end)
end

local function onKnockAway(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_KNOCK_AWAY)
    end
    schedule(guid, "knock", 14000, function() onKnockAway(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    rendFired[guid] = nil
    schedule(guid, "acid", 8000, function() onCorrosiveAcid(creature, guid) end)
    schedule(guid, "freeze", 8000, function() onFreeze(creature, guid) end)
    schedule(guid, "flame", 8000, function() onFlameBreath(creature, guid) end)
    schedule(guid, "knock", 12000, function() onKnockAway(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

local function onDamageTaken(_, creature, _, damage)
    local guid = creature:GetGUID()
    if rendFired[guid] then
        return
    end
    if creature:GetHealth() - damage < creature:GetMaxHealth() * 0.05 then
        rendFired[guid] = true
        creature:CastSpell(creature, SPELL_SUMMON_REND)
    end
end

RegisterCreatureEvent(ENTRY_GYTH, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_GYTH, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_GYTH, 4, onCombatEnd)
RegisterCreatureEvent(ENTRY_GYTH, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY_GYTH, 23, onCombatEnd)
