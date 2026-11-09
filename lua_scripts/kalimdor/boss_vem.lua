-- Temple of Ahn'Qiraj: Vem (Bug Trio) — Lua port of
-- src/server/scripts/Kalimdor/TempleOfAhnQiraj/boss_bug_trio.cpp
-- (boss_vemAI : public BossAI(creature, DATA_BUG_TRIO); GetAI via
-- GetAQ40AI<boss_vemAI>(creature); registered in AddSC_bug_trio();
-- kalimdor loader decl 91 — fifth "// Temple of ahn'qiraj" group).
-- Entry: 15544 = NPC_VEM (worker-verified).
-- Ported: Charge 26561 on a random threat-list any-unit alive
-- (15-27s -> 8-16s; C++ SelectTarget Random,0 defaults, playerOnly
-- false — the old instance-player pool wrongly excluded pets/totems
-- and included unengaged bystanders; the AttackStart leg has no
-- bridge); Knockback 26027 on victim
-- (8-20s -> 15-25s; the -80% threat modify is PORTED 2026-10-24);
-- Enrage 34624 is NOT ported: boss_bug_trio.cpp:219's enrage arm is
-- dead code in this tree — the else-branch decrements Charge_Timer
-- instead of Enrage_Timer, so Enrage_Timer never ticks down and the
-- cast never fires (side effect: Charge_Timer is double-decremented
-- per tick). If the upstream/local C++ is fixed, re-add a latched
-- 120s self-cast 34624 (akilzon precedent).
-- Documented-only: JustDied instance DATA_VEM_DEATH +
-- DATA_BUG_TRIO_DEATH + loot flag legs PORTED below (argent-soldier
-- precedent); the vemDead slot also backs the Kri/Yauj enrage-arm
-- read of DATA_VEMISDEAD (11) — ported in boss_kri.lua /
-- boss_yauj.lua (2s poll, latched self-cast 34624).
local DATA_BUG_TRIO = 6 -- temple_of_ahnqiraj.h:36
local DATA_VEM = 10 -- temple_of_ahnqiraj.h:42
local ENTRY = 15544

local SPELL_CHARGE = 26561
local SPELL_KNOCKBACK = 26027

-- UNIT_DYNAMIC_FLAGS (UpdateFields.h:136) / UNIT_DYNFLAG_LOOTABLE
-- (SharedDefines.h:3128) — eredar-twins precedent.
local UNIT_DYNAMIC_FLAGS = 79
local UNIT_DYNFLAG_LOOTABLE = 0x0001

local function getInstanceData(creature)
    local mapObj = creature:GetMap()
    return mapObj and mapObj:GetInstanceData() or nil
end

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

-- C++ Charge_Timer (boss_bug_trio.cpp:195): SelectTarget Random,0
-- defaults — threat-list any-unit alive, no dist, playerOnly=false;
-- pets/totems are eligible, unlike the old instance-player pool.
-- Instance guid store (engine/world/instance_guid_store.go): Vem
-- latches its own GUID under DATA_VEM on spawn (CREATURE_EVENT_ON_SPAWN
-- 5, fired on first spawn + respawn) — the Lua mirror of the C++
-- instance script's OnCreatureCreate latching (instance_temple_of_
-- ahnqiraj.cpp:32-33); Yauj resolves it via GetInstanceCreature for
-- the Heal 25807 random target (illidari-council precedent).
local function onSpawn(event, creature)
    creature:SetGuidData(DATA_VEM)
end

local function onCharge(creature, guid)
    local candidates = {}
    for _, p in ipairs(creature:GetAITargets()) do
        if not p:IsDead() then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates > 0 then
        creature:CastSpell(candidates[math.random(#candidates)], SPELL_CHARGE)
    end
    schedule(guid, "charge", math.random(8000, 16000), function()
        onCharge(creature, guid)
    end)
end

local function onKnockback(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_KNOCKBACK)
        if creature:GetThreat(victim) ~= 0 then
            creature:ModifyThreatByPercent(victim, -80)
        end
    end
    schedule(guid, "knockback", math.random(15000, 25000), function()
        onKnockback(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    -- C++ has no JustEngagedWith override (boss_bug_trio.cpp:150-186)
    -- — base BossAI::JustEngagedWith writes SetBossState(DATA_BUG_TRIO,
    -- IN_PROGRESS) by virtual dispatch.
    creature:SetBossState(DATA_BUG_TRIO, BOSS_STATE_IN_PROGRESS)
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542)
    schedule(guid, "charge", math.random(15000, 27000), function()
        onCharge(creature, guid)
    end)
    schedule(guid, "knockback", math.random(8000, 20000), function()
        onKnockback(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
end

local function onReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    -- C++ Reset _Reset() leg (boss_bug_trio.cpp:172-176): alive-gated and
    -- != DONE-gated (ScriptedCreature.cpp:499-511; moroes/algalon precedent).
    if creature:IsAlive() and creature:GetBossState(DATA_BUG_TRIO) ~= BOSS_STATE_DONE then
        creature:SetBossState(DATA_BUG_TRIO, BOSS_STATE_NOT_STARTED)
    end
end

-- C++ JustDied (boss_bug_trio.cpp:178-184) overrides WITHOUT calling
-- _JustDied() — SetData(DATA_VEM_DEATH) + lootable-flag + SetData
-- (DATA_BUG_TRIO_DEATH) only — so base DONE never runs: NO ev4
-- SetBossState leg (cthun precedent). The instance legs are PORTED
-- below in C++ order: SetData(DATA_VEM_DEATH, 0) sets
-- IsBossDied[0] (instance temple_of_ahnqiraj.cpp:122-125) — the
-- same slot backs GetData(DATA_VEMISDEAD, 11); then the < 2
-- lootable-flag clear, then the bugTrioDeathCount increment.
local function onDied(event, creature, killer)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    local data = getInstanceData(creature)
    if data then
        data.vemDead = true
        local count = data.bugTrioDeathCount or 0
        if count < 2 then
            creature:RemoveFlag(UNIT_DYNAMIC_FLAGS, UNIT_DYNFLAG_LOOTABLE)
        end
        data.bugTrioDeathCount = count + 1
    end
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 5, onSpawn)
RegisterCreatureEvent(ENTRY, 23, onReset)
