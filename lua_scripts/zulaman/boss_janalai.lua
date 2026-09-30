-- Jan'alai (Zul'Aman) — Lua port of
-- src/server/scripts/EasternKingdoms/ZulAman/boss_janalai.cpp
-- (boss_janalaiAI + npc_janalai_firebombAI + npc_janalai_hatcherAI +
-- npc_janalai_hatchlingAI + npc_janalai_eggAI); zulaman.h:30
-- (BOSS_JANALAI = 2), zulaman.h:47 (NPC_JANALAI = 23578).
-- Creature entries: 23578 Jan'alai (C++ ScriptName "boss_janalai" per
-- AddSC_boss_janalai); 23920 fire bomb ("npc_janalai_firebomb");
-- 23818 Amani hatcher ("npc_janalai_hatcher"); 23598 dragonhawk hatchling
-- ("npc_janalai_hatchling"); 23817 egg ("npc_janalai_egg").
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 9 OnDamageTaken, 14 OnSpellHit, 23 OnReset. Timers via
-- CreateLuaEvent; melee is engine-driven in Go (creature combat tick),
-- like C++ DoMeleeAttackIfReady.
-- Fight shape: flame breath 43140 on a random player at 8s, repeat 8s
-- (C++ AttackStop + motion-master clear + StopMoving have no bridge — the
-- cast is not interrupted here, and the DamageDealt 30-degree arc gate
-- has no bridge, so the isFlameBreathing arm never runs); enrage arm —
-- a 5min one-shot (EnrageTimer = 5*60*1000) whose first fire triggered-
-- casts enrage 44779, sets enraged, and re-arms 300s, and whose later
-- fires Talk(SAY_BERSERK=4) + triggered berserk 45078 + re-arm 300s
-- (C++-exact); the sub-25% early trigger (C++: EnrageTimer = 0 when
-- HealthBelowPct(25)) runs in the event-9 hook on pre-damage health
-- (malchezaar convention), canceling the pending timer and firing the
-- same handler. Kills: Talk SAY_SLAY (C++ KilledUnit has no player gate);
-- death: Talk SAY_DEATH.
-- Deviations from C++: the entire bomb phase has no bridge — no summon
-- model (the 40 fire bombs + fire wall never exist), no creature teleport
-- (DoTeleportTo), no player teleport (DoTeleportPlayer), no AttackStop/
-- motion-master clear, so Talk(SAY_FIRE_BOMBS=1), the fire-wall arms, the
-- 100ms-per-bomb sequence, Boom, and the enrage-timer 10s shave never run;
-- the hatcher arm has no bridge — no egg enumeration (HatchAllEggs), no
-- summon model (the two 23818 hatchers never exist), so Talk(SAY_SUMMON_
-- HATCHER=2) and HatcherTimer never run; the all-eggs 35% arm has no
-- bridge — no teleport, SPELL_HATCH_ALL 43144 is never cast, eggs never
-- hatch, Talk(SAY_ALL_EGGS=3) never runs, noeggs stays false; the Reset
-- egg display-id restore (HatchAllEggs(1), display 10056) has no bearer;
-- no instance-script model — admission only via the luaBossAI shim
-- (GetZulAmanAI/BossAI SetBossState arms skipped, the hatchling's
-- IN_PROGRESS DisappearAndDie arm skipped); the hatcher AI is not
-- registered at all — its whole body is MovementInform waypoints, MoveIn-
-- LineOfSight/AttackStart gates, and instance-state checks (fiendish-
-- portal precedent: no Lua-modelable hooks fire for it); the fire bomb
-- 23920 SpellHit arm (42628 -> triggered self-cast 42629) is registered
-- so any spawned fire bomb reacts correctly, though none ever spawn; the
-- hatchling 23598 flame-buffet AI and the egg 23817 SpellHit arm
-- (42471 -> self-cast 42493) are registered for the same reason; the
-- pre-fight trash gauntlet's SAY_EVENT_STRANGERS/FRIENDS lines belong to
-- zulaman.cpp, not this file's AI.

local ENTRY_JANALAI = 23578
local ENTRY_FIRE_BOMB = 23920
local ENTRY_HATCHLING = 23598
local ENTRY_EGG = 23817

local SAY_AGGRO = 0
local SAY_BERSERK = 4
local SAY_SLAY = 5
local SAY_DEATH = 6

local SPELL_FLAME_BREATH = 43140
local SPELL_ENRAGE = 44779
local SPELL_BERSERK = 45078

local SPELL_FIRE_BOMB_THROW = 42628
local SPELL_FIRE_BOMB_DUMMY = 42629

local SPELL_FLAMEBUFFET = 43299

local SPELL_HATCH_EGG = 42471
local SPELL_SUMMON_HATCHLING = 42493

local timers = {}
local state = {}

local function cancelTimers(store, guid)
    local per = store[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        store[guid] = nil
    end
end

local function schedule(store, guid, key, delay, fn)
    local per = store[guid]
    if not per then
        per = {}
        store[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

local function janalaiState(guid)
    local st = state[guid]
    if not st then
        st = { enraged = false }
        state[guid] = st
    end
    return st
end

local function randomPlayerInRange(creature, maxDist)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() and creature:GetDistance(p) <= maxDist then
            found[#found + 1] = p
        end
    end
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
end

-- C++ flame breath: random target (Random,0 includes the tank), 8s
-- repeat; no triggered flag on the cast.
local function onFlameBreath(creature, guid)
    local target = randomPlayerInRange(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_FLAME_BREATH)
    end
    schedule(timers, guid, "flamebreath", 8000, function()
        onFlameBreath(creature, guid)
    end)
end

-- C++ EnrageTimer arm: first fire -> triggered self-cast 44779, enraged,
-- re-arm 300s; later fires -> Talk SAY_BERSERK, triggered 45078, re-arm
-- 300s (C++-exact).
local function onEnrage(creature, guid)
    local st = janalaiState(guid)
    if not st.enraged then
        creature:CastSpell(creature, SPELL_ENRAGE, true)
        st.enraged = true
    else
        creature:Talk(SAY_BERSERK)
        creature:CastSpell(creature, SPELL_BERSERK, true)
    end
    schedule(timers, guid, "enrage", 300000, function()
        onEnrage(creature, guid)
    end)
end

local function janalaiResetState(guid)
    cancelTimers(timers, guid)
    state[guid] = { enraged = false }
end

local function janalaiEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    janalaiResetState(guid)
    -- C++ JustEngagedWith: Talk + the Initialize timer set
    -- (FireBreathTimer 8s, EnrageTimer 5min); Bomb/Hatcher timers never
    -- run (no summon/teleport bridge).
    creature:Talk(SAY_AGGRO)
    schedule(timers, guid, "flamebreath", 8000, function()
        onFlameBreath(creature, guid)
    end)
    schedule(timers, guid, "enrage", 300000, function()
        onEnrage(creature, guid)
    end)
end

-- C++ UpdateAI: if !enraged and HealthBelowPct(25) -> EnrageTimer = 0
-- (fires immediately). Modeled on pre-damage health via the event-9 hook
-- (malchezaar convention), canceling the pending 5min timer.
local function janalaiDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local st = janalaiState(guid)
    if st.enraged then
        return
    end
    local health, maxHealth = creature:GetHealth(), creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if health * 100 / maxHealth < 25 then
        local per = timers[guid]
        if per and per["enrage"] then
            RemoveEventById(per["enrage"])
            per["enrage"] = nil
        end
        onEnrage(creature, guid)
    end
end

local function janalaiLeaveCombat(event, creature)
    janalaiResetState(creature:GetGUID())
end

local function janalaiTargetDied(event, creature, victim)
    creature:Talk(SAY_SLAY)
end

local function janalaiDied(event, creature, killer)
    janalaiResetState(creature:GetGUID())
    creature:Talk(SAY_DEATH)
end

local function janalaiReset(event, creature)
    janalaiResetState(creature:GetGUID())
    -- C++ Reset's HatchAllEggs(1) display-id restore has no bearer (no
    -- egg enumeration / SetDisplayId bridge).
end

RegisterCreatureEvent(ENTRY_JANALAI, 1, janalaiEnterCombat)
RegisterCreatureEvent(ENTRY_JANALAI, 2, janalaiLeaveCombat)
RegisterCreatureEvent(ENTRY_JANALAI, 3, janalaiTargetDied)
RegisterCreatureEvent(ENTRY_JANALAI, 4, janalaiDied)
RegisterCreatureEvent(ENTRY_JANALAI, 9, janalaiDamageTaken)
RegisterCreatureEvent(ENTRY_JANALAI, 23, janalaiReset)

-- npc_janalai_firebomb (23920, C++ ScriptName for the entry): SpellHit on
-- FIRE_BOMB_THROW 42628 -> triggered self-cast FIRE_BOMB_DUMMY 42629.
-- Fire bombs never spawn in this build (no summon model), but the AI is
-- registered so any that exist react correctly.
local function firebombSpellHit(event, creature, caster, spellId)
    if spellId == SPELL_FIRE_BOMB_THROW then
        creature:CastSpell(creature, SPELL_FIRE_BOMB_DUMMY, true)
    end
end

RegisterCreatureEvent(ENTRY_FIRE_BOMB, 14, firebombSpellHit)

-- npc_janalai_hatchling (23598, C++ ScriptName "npc_janalai_hatchling"):
-- flame buffet 43299 on the victim, first at 7s then every 10s; melee is
-- engine-driven. The MovePoint spawn walk, SetDisableGravity, and the
-- instance-state DisappearAndDie arm have no bridge. Hatchlings never
-- spawn in this build (no summon model), but the AI is registered so any
-- that exist fight correctly.
local function onBuffet(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FLAMEBUFFET)
    end
    schedule(timers, guid, "buffet", 10000, function()
        onBuffet(creature, guid)
    end)
end

local function hatchlingEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(timers, guid)
    schedule(timers, guid, "buffet", 7000, function()
        onBuffet(creature, guid)
    end)
end

local function hatchlingLeaveCombat(event, creature)
    cancelTimers(timers, creature:GetGUID())
end

local function hatchlingReset(event, creature)
    cancelTimers(timers, creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_HATCHLING, 1, hatchlingEnterCombat)
RegisterCreatureEvent(ENTRY_HATCHLING, 2, hatchlingLeaveCombat)
RegisterCreatureEvent(ENTRY_HATCHLING, 23, hatchlingReset)

-- npc_janalai_egg (23817, C++ ScriptName "npc_janalai_egg"): SpellHit on
-- HATCH_EGG 42471 -> self-cast SUMMON_HATCHLING 42493. Eggs never spawn in
-- this build (no summon model), but the AI is registered so any that
-- exist react correctly.
local function eggSpellHit(event, creature, caster, spellId)
    if spellId == SPELL_HATCH_EGG then
        creature:CastSpell(creature, SPELL_SUMMON_HATCHLING)
    end
end

RegisterCreatureEvent(ENTRY_EGG, 14, eggSpellHit)
