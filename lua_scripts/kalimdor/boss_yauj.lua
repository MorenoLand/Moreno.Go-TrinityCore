-- Temple of Ahn'Qiraj: Princess Yauj (Bug Trio) — Lua port of
-- src/server/scripts/Kalimdor/TempleOfAhnQiraj/boss_bug_trio.cpp
-- (boss_yaujAI : public BossAI(creature, DATA_BUG_TRIO); GetAI via
-- GetAQ40AI<boss_yaujAI>(creature); registered in AddSC_bug_trio();
-- kalimdor loader decl 91 — fifth "// Temple of ahn'qiraj" group).
-- Entry: 15543 (no NPC_YAUJ constant exists in temple_of_ahnqiraj.h —
-- only NPC_VEM = 15544 and NPC_KRI = 15511; binding is DB-side via
-- RegisterTempleOfAhnQirajCreatureAI).
-- Ported: Fear 19408 on victim (12-24s -> 20s) + the ResetThreatList leg
-- (2026-10-21, creature:ResetAllThreat — fires ungated C++-exact).
-- Ported: JustDied 10x summon (boss_bug_trio.cpp:270-279 —
-- creature:SummonCreature + summoned:AttackStart, both bridged)
-- + the instance DATA_BUG_TRIO_DEATH + lootable-flag legs
-- (boss_bug_trio.cpp:264-268, ported below).
-- Ported: Vem-dead Enrage 34624 (boss_bug_trio.cpp:321-332 — 2s
-- poll of instance DATA_VEMISDEAD, latched self-cast; the vemDead
-- slot is written by Vem's onDied in boss_vem.lua).
-- Ported: Heal 25807 (boss_bug_trio.cpp:294-313 — uniform 1-of-3
-- Kri / Vem / self; the twins resolve via GetInstanceCreature
-- (DATA_KRI/DATA_VEM), latched by Kri/Vem's onSpawn SetGuidData
-- — illidari-council precedent).
local DATA_BUG_TRIO = 6 -- temple_of_ahnqiraj.h:36
local DATA_KRI = 9 -- temple_of_ahnqiraj.h:41
local DATA_VEM = 10 -- temple_of_ahnqiraj.h:42
local ENTRY = 15543

local SPELL_FEAR = 19408
local SPELL_ENRAGE = 34624
local SPELL_HEAL = 25807

-- boss_bug_trio.cpp:274 summon target (no name constant in the C++
-- tree — the entry comes from the summon call site itself).
local ENTRY_DEATH_SPAWN = 15621

-- UNIT_DYNAMIC_FLAGS (UpdateFields.h:136) / UNIT_DYNFLAG_LOOTABLE
-- (SharedDefines.h:3128) — eredar-twins precedent.
local UNIT_DYNAMIC_FLAGS = 79
local UNIT_DYNFLAG_LOOTABLE = 0x0001

local function getInstanceData(creature)
    local mapObj = creature:GetMap()
    return mapObj and mapObj:GetInstanceData() or nil
end

local timers = {}
local yaujState = {}

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

local function onFear(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FEAR)
    end
    -- C++ boss_bug_trio.cpp:290 — ResetThreatList() fires with the fear
    -- cast, ungated on victim (bridged lua_creature_events.go).
    creature:ResetAllThreat()
    schedule(guid, "fear", 20000, function()
        onFear(creature, guid)
    end)
end

-- C++ boss_bug_trio.cpp:294-313: Heal_Timer (25-40s, then
-- 15-30s) picks uniformly 1-of-3: Kri via instance
-- GetCreature(DATA_KRI), Vem via GetCreature(DATA_VEM), or self.
-- GetInstanceCreature is bridged (lua_creature_events.go) over
-- the guid store Kri/Vem latch on spawn; nil when unlatched or
-- off-map, like the C++ nullptr. A dead pick wastes the tick
-- like C++ (DoCast on a dead twin casts nothing); no re-roll,
-- preserving the uniform 1-of-3.
local function onHeal(creature, guid)
    local roll = math.random(0, 2)
    if roll == 0 then
        local kri = creature:GetInstanceCreature(DATA_KRI)
        if kri and kri:IsAlive() then
            creature:CastSpell(kri, SPELL_HEAL)
        end
    elseif roll == 1 then
        local vem = creature:GetInstanceCreature(DATA_VEM)
        if vem and vem:IsAlive() then
            creature:CastSpell(vem, SPELL_HEAL)
        end
    else
        creature:CastSpell(creature, SPELL_HEAL)
    end
    schedule(guid, "heal", math.random(15000, 30000), function()
        onHeal(creature, guid)
    end)
end

-- C++ boss_bug_trio.cpp:321-332: Check_Timer (2s) polls instance
-- GetData(DATA_VEMISDEAD); on true, self-cast Enrage 34624 and
-- latch VemDead. The outer poll keeps re-arming in C++ even once
-- latched — no observable effect, so the Lua stops polling after
-- the latch. The vemDead slot is written by Vem's onDied
-- (boss_vem.lua) — the same instanceData table.
local function onVemCheck(creature, guid)
    local state = yaujState[guid]
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
    -- C++ has no JustEngagedWith override (boss_bug_trio.cpp:237-259)
    -- — base BossAI::JustEngagedWith writes SetBossState(DATA_BUG_TRIO,
    -- IN_PROGRESS) by virtual dispatch.
    creature:SetBossState(DATA_BUG_TRIO, BOSS_STATE_IN_PROGRESS)
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542)
    yaujState[guid] = { vemDead = false }
    schedule(guid, "fear", math.random(12000, 24000), function()
        onFear(creature, guid)
    end)
    schedule(guid, "heal", math.random(25000, 40000), function()
        onHeal(creature, guid)
    end)
    schedule(guid, "vemcheck", 2000, function()
        onVemCheck(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    yaujState[guid] = nil
end

local function onReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    yaujState[guid] = { vemDead = false }
    -- No _Reset() in C++ Reset (boss_bug_trio.cpp:259-262, Initialize
    -- only) — this leg models base BossAI::JustReachedHome ->
    -- _JustReachedHome (NOT_STARTED, alive-gated and != DONE-gated,
    -- ScriptedCreature.cpp:499-511; skeram precedent).
    if creature:IsAlive() and creature:GetBossState(DATA_BUG_TRIO) ~= BOSS_STATE_DONE then
        creature:SetBossState(DATA_BUG_TRIO, BOSS_STATE_NOT_STARTED)
    end
end

local function onDied(event, creature, killer)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    yaujState[guid] = nil
    -- C++ JustDied (boss_bug_trio.cpp:264-268, before the summon
    -- loop): GetData(DATA_BUG_TRIO_DEATH) < 2 clears the lootable
    -- flag, then SetData(1) increments the shared bugTrioDeathCount
    -- (instance temple_of_ahnqiraj.cpp:127-129) in the instanceData
    -- table — no instance_temple_of_ahnqiraj.lua exists
    -- (argent-soldier precedent). NO ev4 DONE leg: JustDied
    -- overrides WITHOUT calling _JustDied() (cthun precedent).
    local data = getInstanceData(creature)
    if data then
        local count = data.bugTrioDeathCount or 0
        if count < 2 then
            creature:RemoveFlag(UNIT_DYNAMIC_FLAGS, UNIT_DYNFLAG_LOOTABLE)
        end
        data.bugTrioDeathCount = count + 1
    end
    -- C++ JustDied (boss_bug_trio.cpp:270-279): 10 iterations, each
    -- picking a random threat-list target (SelectTarget Random, no
    -- playerOnly/dist args) and summoning one 15621 at the boss's
    -- position (TEMPSUMMON_TIMED_OR_CORPSE_DESPAWN, 90s) with
    -- Summoned->AI()->AttackStart(target). A nil pick skips that
    -- iteration; an empty threat list therefore summons nothing,
    -- exactly like C++. The Go death path clears no threat state
    -- before event 4 fires (kill.go), so GetAITargets still reads
    -- the fight's threat list here.
    local targets = creature:GetAITargets()
    if #targets > 0 then
        local x, y, z = creature:GetX(), creature:GetY(), creature:GetZ()
        for _ = 1, 10 do
            local target = targets[math.random(#targets)]
            local summoned = creature:SummonCreature(ENTRY_DEATH_SPAWN, x, y, z, 0, 1, 90000)
            if summoned ~= nil then
                summoned:AttackStart(target)
            end
        end
    end
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)
