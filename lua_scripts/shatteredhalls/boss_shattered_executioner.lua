-- Shattered Executioner (Shattered Halls, Hellfire Citadel) —
-- Lua port of src/server/scripts/Outland/HellfireCitadel/
-- ShatteredHalls/shattered_halls.cpp (boss_shattered_
-- executioner — the only CreatureScript AI class
-- AddSC_shattered_halls registers whose combat timer is
-- bridgeable; at_nethekurse_exit is an AreaTriggerScript —
-- no Lua area-trigger bridge, not registered (hakkar/
-- zulgurub precedent); its whole OnTrigger body is
-- instance-gated anyway (GetInstanceScript, IsHeroic,
-- GetGuidData(NPC_KARGATH_BLADEFIST)/GetGuidData(NPC_
-- SHATTERED_EXECUTIONER), SummonCreature, DoAction(ACTION_
-- EXECUTIONER_TAUNT = 1)); spell_kargath_executioner is an
-- AuraScript and spell_remove_kargath_executioner is a
-- SpellScript — no AuraScript/SpellScript bridges, both
-- documented only, not registered).
-- Zone .cpp in the Shattered Halls set per outland_script_
-- loader.cpp order (boss roster 3/3 closed with kargath;
-- instance_shattered_halls.cpp stays blocked on the
-- instance-script model).
-- Entry: NPC_SHATTERED_EXECUTIONER = 17301 from shattered_
-- halls.h (SHCreatureIds); the creature_template ScriptName
-- bindings are DB-side (no TDB in this workspace). The
-- executioner defines no Say enum — it never Talk()s.
-- SPELL_CLEAVE = 15284 is the .cpp's own enum; SPELL_
-- KARGATH_EXECUTIONER_1/2/3 (39288/39289/39290) and SPELL_
-- REMOVE_KARGATH_EXECUTIONER (39291) from shattered_halls.h
-- are data only here (their AuraScript/SpellScript scripts
-- have no bridge).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat,
-- 4 OnDied, 23 OnReset. No event 3: the C++ has no
-- KilledUnit override.
-- Fight shape (C++-exact for all modeled arms): Shattered
-- Executioner (17301): OnEnterCombat(1) — per-GUID reset to
-- the C++ Initialize() schedule (cleaveTimer = 500) + 1s
-- scheduler pump (a port of UpdateAI — 1s granularity exact
-- for the C++ timer; the !UpdateVictim early return
-- collapses into the pump). Pump: cleave 15284 500ms init
-- then {5s,7s} — non-triggered DoCastVictim (felmyst
-- convention: C++ `DoCast(SPELL_CLEAVE)` single-arg casts
-- on the victim), re-arm {5s,7s} regardless (C++-exact).
-- OnDied(4): cleanup (the _JustDied arm is instance-
-- blocked; the quest-completion arms have no quest bridge —
-- see below). OnLeaveCombat(2)/OnReset(23): cancel the pump,
-- drop per-GUID state (the Reset() observable remainder —
-- cleaveTimer = 500 — lands on OnEnterCombat per the
-- schedule precedent; the _Reset arm is instance-blocked).
-- Melee is engine-driven.
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI
-- shim (the BossAI ctor DATA_SHATTERED_EXECUTIONER = 3
-- bookkeeping in Reset/JustEngagedWith/JustDied skipped —
-- DATA_SHATTERED_EXECUTIONER is an instance-side constant,
-- unbridgeable; GetShatteredHallsAI is an AI-factory helper,
-- not a script); no loot-mode bridge — the Reset() arms
-- (prisonersExecuted == 0 → AddLootMode(LOOT_MODE_HARD_
-- MODE_3), <= 1 → HARD_MODE_2, <= 2 → HARD_MODE_1, all gated
-- on instance GetData(DATA_PRISONERS_EXECUTED)) unmodeled,
-- and the SetData(DATA_PRISONERS_EXECUTED) RemoveLootMode
-- fall-through machine (case 3 → remove HARD_MODE_1, case 2
-- → HARD_MODE_2, case 1 → HARD_MODE_3) unmodeled; no immune
-- bridge — the Reset() SetImmuneToPC arm (false only when
-- GetBossState(DATA_KARGATH) == DONE — instance gated)
-- unmodeled (kalithresh precedent); no quest/player-list
-- bridge — the JustDied arms (if GetData(DATA_PRISONERS_
-- EXECUTED) > 0 return, else for each instance player:
-- GetTeam == ALLIANCE ? CompleteQuest(QUEST_IMPRISONED_A =
-- 9524) : CompleteQuest(QUEST_IMPRISONED_H = 9525) when the
-- quest is incomplete) unmodeled, and the SetData(data ==
-- 1) FailQuest player loop unmodeled; no cross-creature/
-- ObjectAccessor bridge — the SetData(DATA_PRISONERS_
-- EXECUTED, data <= 3) prisoner-kill arm (GetGuidData(DATA_
-- FIRST_PRISONER + data - 1) → Unit::Kill) unmodeled;
-- DATA_PRISONERS_EXECUTED itself is an instance-side
-- constant, so the shim never delivers SetData to this
-- script — the whole SetData machine is unreachable in this
-- model; the empty JustSummoned override (anti-despawn of
-- prisoners on death/reset) is skipped (ahune bunny /
-- nethekurse fissure empty-skeleton precedent).

local SPELL_CLEAVE = 15284

local ENTRY_SHATTERED_EXECUTIONER = 17301

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

-- C++ Initialize() values: cleaveTimer = 500.
local function freshState()
    return {
        cleave = 500,
    }
end

local function bossTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end

    -- Cleave 15284: 500ms init then {5s,7s} — non-triggered
    -- DoCastVictim (felmyst convention; C++-exact), re-arm
    -- {5s,7s} regardless (C++ `urand(5000, 7000)`, inclusive).
    if st.cleave <= 1000 then
        creature:CastSpell(nil, SPELL_CLEAVE)
        st.cleave = math.random(5000, 7000)
    else
        st.cleave = st.cleave - 1000
    end
end

RegisterCreatureEvent(ENTRY_SHATTERED_EXECUTIONER, 1, function(_, creature)
    local guid = creature:GetGUID()
    resetBoss(guid)
    states[guid] = freshState()
    timers[guid] = CreateLuaEvent(function()
        bossTick(creature, guid)
    end, 1000, 0)
end)

RegisterCreatureEvent(ENTRY_SHATTERED_EXECUTIONER, 2, function(_, creature)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_SHATTERED_EXECUTIONER, 4, function(_, creature)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_SHATTERED_EXECUTIONER, 23, function(_, creature)
    resetBoss(creature:GetGUID())
end)
