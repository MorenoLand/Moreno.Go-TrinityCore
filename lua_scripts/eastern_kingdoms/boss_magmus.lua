-- Magmus (Blackrock Depths; Iron Hall) --
-- Lua port of src/server/scripts/EasternKingdoms/BlackrockMountain/
-- BlackrockDepths/boss_magmus.cpp (boss_magmusAI — ScriptedAI combat
-- scheduler). Boss-script unit per eastern_kingdoms_script_loader.cpp
-- order (boss_magmus follows boss_high_interrogator_gerstahn; the BRD
-- block continues with boss_moira_bronzebeard, boss_tomb_of_seven,
-- coren_direbrew).
-- Entry (verifiable from the C++ sources): instance_blackrock_depths.cpp
-- names NPC_MAGMUS = 9938 in the BRD instance creatures enum (line 44).
-- The creature_template ScriptName binding is DB-side (no TDB in this
-- workspace), but the entry itself is C++-verifiable so the port is
-- registered (npc_phalanx / boss_draganthaurissan precedent).
-- Eluna creature events: 5 OnSpawn, 1 OnEnterCombat, 2 OnLeaveCombat,
-- 4 OnDied, 9 OnDamageTaken, 23 OnReset. Driven by CreateLuaEvent
-- timers; melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
-- Ported arms (C++ JustEngagedWith event schedule + UpdateAI event
-- machine + DamageTaken phase latch):
-- EVENT_FIERY_BURST: victim-cast FIERYBURST 13900, 5s init -> 6s;
-- DamageTaken: HealthBelowPctDamaged(50, damage) one-shot phase-two
-- latch (moroes damage-taken convention: (GetHealth() - damage) * 100
-- / maxHealth < 50) -> schedules EVENT_WARSTOMP at 0s in PHASE_TWO;
-- EVENT_WARSTOMP: victim-cast WARSTOMP 24375, 0s init -> 8s.
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (creature casts
-- are packet-visual), so timers fire unconditionally (maiden
-- precedent). The C++ phase groups are collapsed into the one-shot
-- phase-two latch state — EVENT_FIERY_BURST is phase-agnostic in C++
-- too (scheduled without a phase group).
-- Unmodeled: JustEngagedWith instance->SetData(TYPE_IRON_HALL,
-- IN_PROGRESS) — no instance-script bridge on the Lua surface
-- (instance_blackrock_depths.cpp stays blocked on the instance-script
-- model). Unmodeled: JustDied instance->HandleGameObject(DATA_THRONE_DOOR)
-- + instance->SetData(TYPE_IRON_HALL, DONE) — same instance bridge gap.
-- Not ported (same file): npc_ironhand_guardian — its UpdateAI is gated
-- entirely on _instance->GetData(TYPE_IRON_HALL) == NOT_STARTED
-- (instance arm, no bridge) and fires DoCastAOE(SPELL_GOUTOFFLAME=15529)
-- Repeat 16s/21s — no AOE-cast bridge on the Lua surface. Documented-only.

local SPELL_FIERYBURST = 13900
local SPELL_WARSTOMP = 24375

local ENTRY_MAGMUS = 9938

local magmusState = {}
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
    per[key] = CreateLuaEvent(fn, delay)
end

local function initState(guid)
    magmusState[guid] = { phaseTwo = false }
end

local function onFieryBurst(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FIERYBURST)
    end
    schedule(guid, "fieryburst", 6000, function() onFieryBurst(creature, guid) end)
end

local function onWarstomp(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_WARSTOMP)
    end
    schedule(guid, "warstomp", 8000, function() onWarstomp(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    initState(guid)
    schedule(guid, "fieryburst", 5000, function() onFieryBurst(creature, guid) end)
end

local function onDamageTaken(_, creature, _, damage)
    local guid = creature:GetGUID()
    local state = magmusState[guid]
    if state == nil or state.phaseTwo then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if (creature:GetHealth() - damage) * 100 / maxHealth < 50 then
        state.phaseTwo = true
        schedule(guid, "warstomp", 0, function() onWarstomp(creature, guid) end)
    end
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

local function onSpawn(_, creature)
    initState(creature:GetGUID())
end

local function onReset(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    initState(guid)
end

RegisterCreatureEvent(ENTRY_MAGMUS, 5, onSpawn)
RegisterCreatureEvent(ENTRY_MAGMUS, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_MAGMUS, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_MAGMUS, 4, onCombatEnd)
RegisterCreatureEvent(ENTRY_MAGMUS, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY_MAGMUS, 23, onReset)
