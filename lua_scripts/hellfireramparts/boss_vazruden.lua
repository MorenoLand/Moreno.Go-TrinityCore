-- Vazruden (Hellfire Ramparts, Hellfire Citadel) --
-- Lua port of src/server/scripts/Outland/HellfireCitadel/
-- HellfireRamparts/boss_vazruden_the_herald.cpp (boss_
-- vazruden — one of the CreatureScript AI classes the same
-- file's AddSC_boss_vazruden_the_herald registers; no
-- SpellScript/AuraScript scripts in this file).
-- Final boss in the Hellfire Ramparts set per
-- outland_script_loader.cpp order (instance_hellfire_
-- ramparts.cpp stays blocked on the instance-script model).
-- Entry: NPC_VAZRUDEN = 17537 (hellfire_ramparts.h,
-- verifiable from the C++ sources); instance_hellfire_
-- ramparts.cpp has no OnCreatureCreate mapping for it —
-- only OnGameObjectCreate doors and a SetBossState
-- DATA_VAZRUDEN = 2 completion arm (instance-side
-- constant, unbridgeable). The creature_template
-- ScriptName bindings are DB-side (no TDB in this
-- workspace).
-- Talk: SAY_WIPE = 0 (wipe — fired from the C++
-- UnsummonCheck no-victim arm, C++-exact), SAY_AGGRO = 1
-- (pull — fired from the C++ JustEngagedWith, C++-exact),
-- SAY_KILL = 2 (kill — the C++ KilledUnit override gates on
-- victim->GetEntry() != NPC_VAZRUDEN, C++-exact via the
-- victim object the engine passes to event 3), SAY_DIE = 3
-- (death — fired from the C++ JustDied; the C++ killer !=
-- me gate excludes only the DisappearAndDie suicide path,
-- which has no bridge in this model — the wipe yell lives
-- on the evade hook, so Talk(SAY_DIE) on OnDied is
-- C++-equivalent for the reachable paths).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via
-- CreateLuaEvent (per-GUID scheduler pump); melee is
-- engine-driven in Go (creature combat tick), like C++ DoMelee
-- AttackIfReady. C++ DoCast default is triggered=false
-- (vaelastrasz convention).
-- Fight shape (C++-exact for all modeled arms): Vazruden
-- (17537): OnEnterCombat(1): per-GUID reset to the C++
-- Initialize() values (revenge 4000 / wipeSaid false — the
-- BossAI::JustEngagedWith arm is instance-blocked, DATA_
-- VAZRUDEN = 2 is unbridgeable) + Talk(SAY_AGGRO) + 1s
-- scheduler pump (a port of UpdateAI in C++ arm order — 1s
-- granularity exact for the C++ timer; the !UpdateVictim
-- early return collapses into the pump). Pump: revenge —
-- 4s init then 5s, target = victim (C++ GetVictim(), the
-- nil-fallback in this model, C++-exact), non-triggered
-- DoCast (DUNGEON_MODE normal 19130 — no difficulty bridge,
-- thespia precedent, the H_SPELL_REVENGE 40392 variant
-- unmodeled), re-arm 5000 regardless (C++-exact). Melee is
-- engine-driven.
-- OnTargetDied(3): Talk(SAY_KILL) gated on the killed unit's
-- entry not being NPC_VAZRUDEN (C++ `if (who && who->
-- GetEntry() != NPC_VAZRUDEN)`, C++-exact).
-- OnLeaveCombat(2): Talk(SAY_WIPE) + cleanup (the C++
-- UnsummonCheck 2s no-victim arm — the WipeSaid latch is
-- reset by C++ Reset() on the same evade, so the yell fires
-- on the evade hook; the DisappearAndDie arm has no
-- bridge, omor SAY_WIPE-on-evade precedent, C++-exact).
-- OnDied(4): Talk(SAY_DIE) + cleanup (the _JustDied arm is
-- instance-blocked). OnReset(23): cancel the pump, drop
-- per-GUID state (the Reset() observable remainder — the
-- Initialize() re-latch — lands on OnEnterCombat; the
-- _Reset arm is instance-blocked).
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI
-- shim (instance_hellfire_ramparts.cpp stays blocked on the
-- instance-script model; the BossAI ctor DATA_VAZRUDEN = 2
-- bookkeeping in Reset/JustEngagedWith/JustDied skipped);
-- no difficulty bridge — SPELL_REVENGE_H 40392 unmodeled
-- (thespia precedent); no despawn bridge — the
-- DisappearAndDie arm of the UnsummonCheck machine
-- unmodeled; no SpellScript/AuraScript scripts in this
-- file.

local SPELL_REVENGE = 19130
local SPELL_REVENGE_H = 40392

local SAY_WIPE = 0
local SAY_AGGRO = 1
local SAY_KILL = 2
local SAY_DIE = 3

local ENTRY_VAZRUDEN = 17537

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

-- C++ Initialize() values (the BossAI ctor instance arms are
-- instance blocked, so the whole engagement schedule lands
-- on OnEnterCombat — broggok precedent): revenge 4000 /
-- wipeSaid false.
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
    -- this model, C++-exact; no difficulty bridge so the
    -- normal spell is used, thespia precedent), re-arm 5000
    -- regardless (C++-exact).
    if st.revenge <= 1000 then
        creature:CastSpell(nil, SPELL_REVENGE)
        st.revenge = 5000
    else
        st.revenge = st.revenge - 1000
    end
end

RegisterCreatureEvent(ENTRY_VAZRUDEN, 1, function(_, creature)
    local guid = creature:GetGUID()
    resetBoss(guid)
    states[guid] = freshState()
    creature:Talk(SAY_AGGRO)
    timers[guid] = CreateLuaEvent(function()
        bossTick(creature, guid)
    end, 1000, 0)
end)

RegisterCreatureEvent(ENTRY_VAZRUDEN, 2, function(_, creature)
    -- C++ UnsummonCheck no-victim arm (2s): Talk(SAY_WIPE) +
    -- DisappearAndDie — the wipe yell lands on the evade
    -- hook in this model (omor precedent); the despawn arm
    -- has no bridge.
    creature:Talk(SAY_WIPE)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_VAZRUDEN, 3, function(_, creature, victim)
    -- C++ KilledUnit: Talk(SAY_KILL) when the killed unit's
    -- entry is not NPC_VAZRUDEN (C++-exact).
    if victim and victim:GetEntry() ~= ENTRY_VAZRUDEN then
        creature:Talk(SAY_KILL)
    end
end)

RegisterCreatureEvent(ENTRY_VAZRUDEN, 4, function(_, creature)
    creature:Talk(SAY_DIE)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_VAZRUDEN, 23, function(_, creature)
    resetBoss(creature:GetGUID())
end)
