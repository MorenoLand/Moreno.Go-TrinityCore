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
-- creature:SummonCreature + summoned:AttackStart, both bridged;
-- the instance DATA_BUG_TRIO_DEATH + lootable-flag legs stay out).
-- Documented-only: Heal 25807 (target selection via instance
-- GetCreature(DATA_KRI/DATA_VEM) — no bridge; self-cast alone
-- would misrepresent the random 1-of-3); Vem-dead Enrage (instance
-- DATA_VEMISDEAD — no bridge).
local DATA_BUG_TRIO = 6 -- temple_of_ahnqiraj.h:36
local ENTRY = 15543

local SPELL_FEAR = 19408

-- boss_bug_trio.cpp:274 summon target (no name constant in the C++
-- tree — the entry comes from the summon call site itself).
local ENTRY_DEATH_SPAWN = 15621

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

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    -- C++ has no JustEngagedWith override (boss_bug_trio.cpp:237-259)
    -- — base BossAI::JustEngagedWith writes SetBossState(DATA_BUG_TRIO,
    -- IN_PROGRESS) by virtual dispatch.
    creature:SetBossState(DATA_BUG_TRIO, BOSS_STATE_IN_PROGRESS)
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542)
    schedule(guid, "fear", math.random(12000, 24000), function()
        onFear(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    -- No _Reset() in C++ Reset (boss_bug_trio.cpp:259-262, Initialize
    -- only) — this leg models base BossAI::JustReachedHome ->
    -- _JustReachedHome (NOT_STARTED, alive-gated and != DONE-gated,
    -- ScriptedCreature.cpp:499-511; skeram precedent).
    if creature:IsAlive() and creature:GetBossState(DATA_BUG_TRIO) ~= BOSS_STATE_DONE then
        creature:SetBossState(DATA_BUG_TRIO, BOSS_STATE_NOT_STARTED)
    end
end

local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUIDLow())
    -- C++ JustDied (boss_bug_trio.cpp:270-279): 10 iterations, each
    -- picking a random threat-list target (SelectTarget Random, no
    -- playerOnly/dist args) and summoning one 15621 at the boss's
    -- position (TEMPSUMMON_TIMED_OR_CORPSE_DESPAWN, 90s) with
    -- Summoned->AI()->AttackStart(target). A nil pick skips that
    -- iteration; an empty threat list therefore summons nothing,
    -- exactly like C++. The Go death path clears no threat state
    -- before event 4 fires (kill.go), so GetAITargets still reads
    -- the fight's threat list here. The instance legs (RemoveFlag
    -- lootable + SetData DATA_BUG_TRIO_DEATH) stay out — no
    -- instance bridge. NO ev4 DONE leg either: JustDied (:264-268)
    -- overrides WITHOUT calling _JustDied() (cthun precedent).
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
