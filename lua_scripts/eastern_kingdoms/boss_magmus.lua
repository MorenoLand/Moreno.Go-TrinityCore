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
-- latch (int64(health)-damage < floor(maxHealth*50/100)) ->
-- schedules EVENT_WARSTOMP at 0s in PHASE_TWO (lethal-hit guard: the
-- 0s event executes on the next UpdateAI tick, which a dead creature
-- never runs, so the cast fires only when postDamage > 0; the latch
-- itself still sets, as in C++);
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
-- Repeat 16s/21s — the cast lowers to self-cast per the selin_fireheart
-- DoCastAOE convention, but the whole AI is gated on the unbridged
-- instance data (TYPE_IRON_HALL == NOT_STARTED), so no leg of the arm
-- can be reached. Documented-only.

local SPELL_FIERYBURST = 13900
local SPELL_WARSTOMP = 24375

local ENTRY_MAGMUS = 9938
local GO_THRONE_ROOM = 170575 -- instance_blackrock_depths.cpp:70, GO_THRONE_ROOM

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
    if per[key] then
        RemoveEventById(per[key])
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
    local postDamage = creature:GetHealth() - damage
    if postDamage < math.floor(maxHealth * 50 / 100) then
        state.phaseTwo = true
        -- C++ ScheduleEvent(EVENT_WARSTOMP, 0s) fires on the next
        -- UpdateAI tick, and a creature killed by this blow never runs
        -- UpdateAI again — so on a lethal hit the latch sets but the
        -- cast never fires (angerforge precedent). CreateLuaEvent
        -- rejects delay 0, so a surviving hit fires the tick directly
        -- (it re-arms at 8s itself).
        if postDamage > 0 then
            onWarstomp(creature, guid)
        end
    end
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ JustDied: throne-door open (HandleGameObject(DATA_THRONE_DOOR,
-- true) = GO_STATE_ACTIVE, InstanceScript.cpp:484) + SetData
-- TYPE_IRON_HALL DONE (no instance-data bridge). The door leg must NOT
-- ride onCombatEnd: evade (event 2) never opens it in C++.
local function onJustDied(_, creature)
    cancelTimers(creature:GetGUID())
    local door = creature:FindNearestGameObject(GO_THRONE_ROOM, 100.0)
    if door then door:SetGoState(0) end
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
RegisterCreatureEvent(ENTRY_MAGMUS, 4, onJustDied)
RegisterCreatureEvent(ENTRY_MAGMUS, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY_MAGMUS, 23, onReset)
