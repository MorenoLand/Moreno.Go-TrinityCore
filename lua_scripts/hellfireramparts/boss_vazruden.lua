-- Vazruden (Hellfire Ramparts, Hellfire Citadel) --
-- Lua port of src/server/scripts/Outland/HellfireCitadel/
-- HellfireRamparts/boss_vazruden_the_herald.cpp (boss_
-- vazruden — one of the CreatureScript AI classes the same
-- file's AddSC_boss_vazruden_the_herald registers; no
-- SpellScript/AuraScript scripts in this file).
-- Final boss in the Hellfire Ramparts set per
-- outland_script_loader.cpp order (instance_hellfire_
-- ramparts.cpp PORTED in instance_hellfire_ramparts.lua).
-- Entry: NPC_VAZRUDEN = 17537 (hellfire_ramparts.h,
-- verifiable from the C++ sources); instance_hellfire_
-- ramparts.cpp has no OnCreatureCreate mapping for it —
-- only OnGameObjectCreate doors and a SetBossState
-- DATA_VAZRUDEN = 2 completion arm (PORTED below in
-- OnDied: SetBossState DONE + instanceData flag). The creature_template
-- ScriptName bindings are DB-side (no TDB in this
-- workspace).
-- Talk: SAY_WIPE = 0 (wipe — fired from the C++
-- UnsummonCheck no-victim arm, C++-exact), SAY_AGGRO = 1
-- (pull — fired from the C++ JustEngagedWith, C++-exact),
-- SAY_KILL = 2 (kill — the C++ KilledUnit override gates on
-- victim->GetEntry() != NPC_VAZRUDEN, C++-exact via the
-- victim object the engine passes to event 3), SAY_DIE = 3
-- (death — fired from the C++ JustDied; JustDied is only
-- reached from Unit::Kill, never from DisappearAndDie /
-- ForcedDespawn, so the C++ `killer && killer != me` gate
-- excludes Kill(nullptr) / Kill(self) deaths; the killer
-- rides event 4's 3rd arg in this engine).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via
-- CreateLuaEvent (per-GUID scheduler pump); melee is
-- engine-driven in Go (creature combat tick), like C++ DoMelee
-- AttackIfReady. C++ DoCast default is triggered=false
-- (vaelastrasz convention).
-- Fight shape (C++-exact for all modeled arms): Vazruden
-- (17537): OnEnterCombat(1): per-GUID reset to the C++
-- Initialize() values (revenge 4000 / wipeSaid false — the
-- BossAI::JustEngagedWith IN_PROGRESS write stays out —
-- no consumer; Talk + SetCombatPulseDelay +
-- DoZoneInCombat are ported) + Talk(SAY_AGGRO) + 1s
-- scheduler pump (a port of UpdateAI in C++ arm order — 1s
-- granularity exact for the C++ timer; the !UpdateVictim
-- early return collapses into the pump). Pump: revenge —
-- 4s init then 5s, target = victim (C++ GetVictim(), the
-- nil-fallback in this model, C++-exact), non-triggered
-- DoCast (DUNGEON_MODE normal 19130 / heroic 40392 via
-- creature:IsHeroic()), re-arm 5000 regardless (C++-exact). Melee is
-- engine-driven.
-- OnTargetDied(3): Talk(SAY_KILL) gated on the killed unit's
-- entry not being NPC_VAZRUDEN (C++ `if (who && who->
-- GetEntry() != NPC_VAZRUDEN)`, C++-exact).
-- OnLeaveCombat(2): cleanup + 2000ms one-shot (the C++
-- UnsummonCheck no-victim arm: Talk(SAY_WIPE) once +
-- DisappearAndDie — self, no delay, me->IsAlive() gate,
-- creature:Despawn(), cancelled on re-engage, C++-exact).
-- OnDied(4): Talk(SAY_DIE) gated on killer ~= me
-- (C++-exact) + cleanup + the _JustDied arm (SetBossState
-- DATA_VAZRUDEN DONE + instanceData.vazrudenDone flag,
-- PORTED). OnReset(23): cancel the pump, drop
-- per-GUID state (the Reset() observable remainder — the
-- Initialize() re-latch — lands on OnEnterCombat; the
-- _Reset NOT_STARTED write stays out — no consumer).
-- Deliberate deviations (all await engine bridges): the
-- instance script is PORTED (instance_hellfire_ramparts.lua
-- — RegisterMapEvent model; the _JustDied DONE arm rides
-- ev4 below); the BossAI ctor DATA_VAZRUDEN = 2
-- IN_PROGRESS / NOT_STARTED bookkeeping in Reset /
-- JustEngagedWith stays out (no consumer);
-- difficulty is bridged — creature:IsHeroic() (per-instance
-- difficulty, lua_creature_events.go) drives the DUNGEON_MODE
-- spell variants; the UnsummonCheck DisappearAndDie arm is PORTED
-- (2026-10-17): the old "no despawn bridge" was stale —
-- creature:Despawn() is bridged (lua_creature_events.go:214) —
-- 2000ms evade one-shot -> Talk(SAY_WIPE) + Despawn(), cancelled
-- on re-engage (the WipeSaid latch is the one-shot; Reset()
-- re-seeds the 2000ms timer in C++, C++-exact); no
-- SpellScript/AuraScript scripts in this file.

local SPELL_REVENGE = 19130
local SPELL_REVENGE_H = 40392

local SAY_WIPE = 0
local SAY_AGGRO = 1
local SAY_KILL = 2
local SAY_DIE = 3

local ENTRY_VAZRUDEN = 17537

local DATA_VAZRUDEN = 2 -- hellfire_ramparts.h DATA_VAZRUDEN
local BOSS_STATE_DONE = 3 -- EncounterState DONE (InstanceScript.h:68-76)

local timers = {}
local states = {}

local function cancelPump(guid)
    local id = timers[guid]
    if id then
        RemoveEventById(id)
        timers[guid] = nil
    end
end

local function resetBoss(guid)
    cancelPump(guid)
    states[guid] = nil
end

local despawnTimers = {}

local function cancelDespawn(guid)
    local id = despawnTimers[guid]
    if id then
        RemoveEventById(id)
        despawnTimers[guid] = nil
    end
end

-- C++ Initialize() values (the BossAI ctor instance arms are
-- instance blocked, so the whole engagement schedule lands
-- on OnEnterCombat — broggok precedent): revenge 4000
-- (the WipeSaid latch lives on the evade hook, where the
-- C++ UnsummonCheck no-victim arm's yell was remodeled).
local function freshState()
    return {
        revenge = 4000,
    }
end

local function bossTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end

    -- Revenge 19130: 4s init then 5s — non-triggered DoCast
    -- on the victim (C++ GetVictim(), the nil-fallback in
    -- this model, C++-exact; DUNGEON_MODE heroic 40392 via
    -- creature:IsHeroic()), re-arm 5000
    -- regardless (C++-exact).
    if st.revenge <= 1000 then
        creature:CastSpell(nil, creature:IsHeroic() and SPELL_REVENGE_H or SPELL_REVENGE)
        st.revenge = 5000
    else
        st.revenge = st.revenge - 1000
    end
end

RegisterCreatureEvent(ENTRY_VAZRUDEN, 1, function(_, creature)
    local guid = creature:GetGUIDLow()
    resetBoss(guid)
    cancelDespawn(guid)
    states[guid] = freshState()
    creature:Talk(SAY_AGGRO)
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542; sapphiron precedent)
    timers[guid] = CreateLuaEvent(function()
        bossTick(creature, guid)
    end, 1000, 0)
end)

RegisterCreatureEvent(ENTRY_VAZRUDEN, 2, function(_, creature)
    -- C++ UnsummonCheck no-victim arm (:253-266): 2000ms after losing
    -- the victim, Talk(SAY_WIPE) once (the WipeSaid latch — the
    -- one-shot fires once) + DisappearAndDie — self, no delay,
    -- me->IsAlive() gate; creature:Despawn() is bridged
    -- (lua_creature_events.go:214). Re-engage (event 1) cancels the
    -- pending despawn — the C++ decrements UnsummonCheck only in the
    -- !UpdateVictim() branch, and Reset() re-seeds it to 2000.
    local guid = creature:GetGUIDLow()
    resetBoss(guid)
    cancelDespawn(guid)
    despawnTimers[guid] = CreateLuaEvent(function()
        despawnTimers[guid] = nil
        if not creature:IsDead() then
            creature:Talk(SAY_WIPE)
            creature:Despawn()
        end
    end, 2000, 1)
end)

RegisterCreatureEvent(ENTRY_VAZRUDEN, 3, function(_, creature, victim)
    -- C++ KilledUnit: Talk(SAY_KILL) when the killed unit's
    -- entry is not NPC_VAZRUDEN (C++-exact).
    if victim and victim:GetEntry() ~= ENTRY_VAZRUDEN then
        creature:Talk(SAY_KILL)
    end
end)

RegisterCreatureEvent(ENTRY_VAZRUDEN, 4, function(_, creature, killer)
    -- C++ JustDied: `if (killer && killer != me)
    -- Talk(SAY_DIE)` — the killer rides event 4's 3rd arg
    -- in this engine (lua_creature_events.go).
    if killer and killer:GetGUID() ~= creature:GetGUID() then
        creature:Talk(SAY_DIE)
    end
    -- C++ BossAI::_JustDied (ScriptedCreature.cpp:513-520):
    -- instance->SetBossState(DATA_VAZRUDEN, DONE). The
    -- instance script's chest-clear leg reads the flag below
    -- (no boss-state-change event exists —
    -- instance_hellfire_ramparts.lua header).
    creature:SetBossState(DATA_VAZRUDEN, BOSS_STATE_DONE)
    local mapObj = creature:GetMap()
    local data = mapObj and mapObj:GetInstanceData() or nil
    if data then
        data.vazrudenDone = true
    end
    resetBoss(creature:GetGUIDLow())
    cancelDespawn(creature:GetGUIDLow())
end)

RegisterCreatureEvent(ENTRY_VAZRUDEN, 23, function(_, creature)
    resetBoss(creature:GetGUIDLow())
    cancelDespawn(creature:GetGUIDLow())
end)
