-- Temple of Ahn'Qiraj: Kri (Bug Trio) — Lua port of
-- src/server/scripts/Kalimdor/TempleOfAhnQiraj/boss_bug_trio.cpp
-- (boss_kriAI : public BossAI(creature, DATA_BUG_TRIO); GetAI via
-- GetAQ40AI<boss_kriAI>(creature); registered in AddSC_bug_trio();
-- kalimdor loader decl 91 — fifth "// Temple of ahn'qiraj" group).
-- Entry: 15511 = NPC_KRI (worker-verified).
-- Ported: Cleave 26350 on victim (4-8s -> 5-12s, jeklik
-- GetVictim + CastSpell convention); Toxic Volley 25812 on victim
-- (6-12s -> 10-15s); Poison Cloud 38718 on victim at <5% health
-- (event 9, latched — C++ Death flag).
-- Ported: Vem-dead Enrage 34624 (boss_bug_trio.cpp:113-130 — 2s
-- poll of instance DATA_VEMISDEAD, latched self-cast; the vemDead
-- slot is written by Vem's onDied in boss_vem.lua); JustDied
-- loot-flag + instance DATA_BUG_TRIO_DEATH legs PORTED below
-- (argent-soldier precedent).
local DATA_BUG_TRIO = 6 -- temple_of_ahnqiraj.h:36
local ENTRY = 15511

local SPELL_CLEAVE = 26350
local SPELL_TOXIC_VOLLEY = 25812
local SPELL_POISON_CLOUD = 38718
local SPELL_ENRAGE = 34624

-- UNIT_DYNAMIC_FLAGS (UpdateFields.h:136) / UNIT_DYNFLAG_LOOTABLE
-- (SharedDefines.h:3128) — eredar-twins precedent.
local UNIT_DYNAMIC_FLAGS = 79
local UNIT_DYNFLAG_LOOTABLE = 0x0001

local function getInstanceData(creature)
    local mapObj = creature:GetMap()
    return mapObj and mapObj:GetInstanceData() or nil
end

local timers = {}
local kriState = {}

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

local function onCleave(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CLEAVE)
    end
    schedule(guid, "cleave", math.random(5000, 12000), function()
        onCleave(creature, guid)
    end)
end

local function onVolley(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_TOXIC_VOLLEY)
    end
    schedule(guid, "volley", math.random(10000, 15000), function()
        onVolley(creature, guid)
    end)
end

local function onVemCheck(creature, guid)
    local state = kriState[guid]
    if state == nil or state.vemDead then
        return
    end
    local data = getInstanceData(creature)
    if data and data.vemDead then
        state.vemDead = true
        creature:CastSpell(creature, SPELL_ENRAGE)
        return
    end
    schedule(guid, "vemcheck", 2000, function()
        onVemCheck(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    -- C++ has no JustEngagedWith override (boss_bug_trio.cpp:56-94) —
    -- base BossAI::JustEngagedWith writes SetBossState(DATA_BUG_TRIO,
    -- IN_PROGRESS) by virtual dispatch.
    creature:SetBossState(DATA_BUG_TRIO, BOSS_STATE_IN_PROGRESS)
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542)
    kriState[guid] = { deathCloud = false, vemDead = false }
    schedule(guid, "cleave", math.random(4000, 8000), function()
        onCleave(creature, guid)
    end)
    schedule(guid, "volley", math.random(6000, 12000), function()
        onVolley(creature, guid)
    end)
    schedule(guid, "vemcheck", 2000, function()
        onVemCheck(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    kriState[guid] = nil
end

local function onReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    kriState[guid] = { deathCloud = false, vemDead = false }
    -- C++ Reset _Reset() leg (boss_bug_trio.cpp:80-84): alive-gated and
    -- != DONE-gated (ScriptedCreature.cpp:499-511; moroes/algalon precedent).
    if creature:IsAlive() and creature:GetBossState(DATA_BUG_TRIO) ~= BOSS_STATE_DONE then
        creature:SetBossState(DATA_BUG_TRIO, BOSS_STATE_NOT_STARTED)
    end
end

-- C++: !HealthAbovePct(5) && !Death -> Poison Cloud, latched.
local function onDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUIDLow()
    local state = kriState[guid]
    if state == nil or state.deathCloud then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    -- C++ !HealthAbovePct(5) is read post-damage in UpdateAI; event 9
    -- fires pre-damage (engine/world/combat.go:1274), so project the blow.
    -- The postDamage > 0 guard (nefarian rule): the C++ UpdateAI arm
    -- never fires on the killing blow.
    local postDamage = creature:GetHealth() - damage
    if postDamage > 0 and postDamage <= math.floor(maxHealth * 5 / 100) then
        state.deathCloud = true
        local victim = creature:GetVictim()
        if victim then
            creature:CastSpell(victim, SPELL_POISON_CLOUD)
        end
    end
end

-- C++ JustDied (boss_bug_trio.cpp:87-92) overrides WITHOUT calling
-- _JustDied() — lootable-flag + SetData(DATA_BUG_TRIO_DEATH) only —
-- so base DONE never runs: NO ev4 SetBossState leg (cthun precedent).
-- The instance legs are PORTED below: GetData < 2 clears the
-- lootable flag, then SetData(1) increments the shared
-- bugTrioDeathCount (instance temple_of_ahnqiraj.cpp:127-129) in
-- the instanceData table — no instance_temple_of_ahnqiraj.lua
-- exists, so the counter lives here (argent-soldier precedent).
local function onDied(event, creature, killer)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    kriState[guid] = nil
    local data = getInstanceData(creature)
    if data then
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
RegisterCreatureEvent(ENTRY, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY, 23, onReset)
