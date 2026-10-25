-- General Angerforge (Blackrock Depths, Hall of Crafting) -- Lua port of
-- src/server/scripts/EasternKingdoms/BlackrockMountain/BlackrockDepths/
-- boss_general_angerforge.cpp (boss_general_angerforgeAI : public
-- ScriptedAI). Boss-script unit per eastern_kingdoms_script_loader.cpp
-- order (AddSC_boss_general_angerforge follows
-- AddSC_boss_ambassador_flamelash).
-- Entry (no NPC_ constant in the C++ tree — the creature_template
-- ScriptName binding is DB-side; the BRD instance creatures enum in
-- instance_blackrock_depths.cpp names the neighbors NPC_MOIRA 8929 /
-- NPC_EMPEROR 9019 but not Angerforge): 9033 "General Angerforge",
-- verified against the classic NPC ID list (inven.co.kr/board/wow/49/
-- 34580: "9033 General Angerforge"; neighbors 9034-9040 are the
-- C++-verified NPC_ANGERREL..NPC_DOOMREL Tomb of Seven, 9019 is the
-- C++-verified NPC_EMPEROR) — same wowhead-level verification as the
-- flamelash 9156 precedent.
-- Eluna creature events: 5 OnSpawn, 1 OnEnterCombat, 2 OnLeaveCombat,
-- 4 OnDied, 9 OnDamageTaken, 23 OnReset. Timers via CreateLuaEvent
-- per-GUID (cancel on leave combat / died / reset); melee is
-- engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
-- Ported arms (C++-exact):
-- JustEngagedWith: SetPhase(PHASE_ONE); EVENT_MIGHTYBLOW 8s->18s
-- DoCastVictim (14099), EVENT_HAMSTRING 12s->15s DoCastVictim (9080),
-- EVENT_CLEAVE 16s->9s DoCastVictim (20691). All three are scheduled
-- phase-agnostic in C++ (no phase group on the JustEngagedWith
-- schedule), so they keep firing after the phase transition.
-- DamageTaken: HealthBelowPctDamaged(20) one-shot phase latch
-- (int64(health)-damage < floor(maxHealth*20/100), magmus precedent;
-- C++ IsInPhase(PHASE_ONE) guard = one-shot) -> PHASE_TWO; schedules
-- EVENT_MEDIC at 0s (one-shot, no re-arm in C++) and EVENT_ADDS at 0s
-- (re-arms 25s in PHASE_TWO).
-- Deviations: no UNIT_STATE_CASTING gate in the C++ UpdateAI (maiden
-- precedent — timers fire unconditionally). EVENT_MEDIC / EVENT_ADDS
-- summon legs PORTED 2026-10-24 (SummonMedic: DoSpawnCreature(8894) x2;
-- SummonAdd: DoSpawnCreature(8901) x3, both
-- TEMPSUMMON_TIMED_OR_CORPSE_DESPAWN 120s + AttackStart(victim) via
-- creature:SummonCreature type 1 + EngageWithTarget; offsets are
-- boss-pos + irand(-14,14)/irand(-9,9) per ScriptedCreature.cpp:288);
-- both events keep their C++-exact 0s/25s schedule. The C++ 0s schedule fires the tick directly
-- (CreateLuaEvent rejects delay 0 — the C++ 0s means "next UpdateAI
-- tick", equivalent here). No Talk lines exist in the C++.
-- OnLeaveCombat(2)/OnDied(4)/OnReset(23): cancel the timers (the C++
-- Reset() _events.Reset() has no other observable arms).

local SPELL_MIGHTYBLOW = 14099
local SPELL_HAMSTRING = 9080
local SPELL_CLEAVE = 20691

local NPC_ANVILRAGE_MEDIC = 8894
local NPC_ANVILRAGE_ADD = 8901

local ENTRY = 9033

local angerforgeState = {}
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
    angerforgeState[guid] = { phaseTwo = false }
end

local function onMightyBlow(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_MIGHTYBLOW)
    end
    schedule(guid, "mightyblow", 18000, function()
        onMightyBlow(creature, guid)
    end)
end

local function onHamstring(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_HAMSTRING)
    end
    schedule(guid, "hamstring", 15000, function()
        onHamstring(creature, guid)
    end)
end

local function onCleave(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CLEAVE)
    end
    schedule(guid, "cleave", 9000, function()
        onCleave(creature, guid)
    end)
end

-- C++ SummonMedic x2 (DoSpawnCreature(8894) + AttackStart(victim)):
-- boss-pos + irand(-9,9) offsets, type 1, 120s; AttackStart via
-- EngageWithTarget (player-target funnel; non-player victim no-ops
-- in the bridge, spawn still happens — flamelash precedent).
-- One-shot 0s schedule, no re-arm in C++.
local function onMedic(creature, guid)
    local bx, by, bz = creature:GetX(), creature:GetY(), creature:GetZ()
    local victim = creature:GetVictim()
    for _ = 1, 2 do
        local medic = creature:SummonCreature(NPC_ANVILRAGE_MEDIC,
            bx + math.random(-9, 9), by + math.random(-9, 9), bz, 0, 1, 120000)
        if medic and victim then
            medic:EngageWithTarget(victim)
        end
    end
end

-- C++ SummonAdd x3 (DoSpawnCreature(8901) + AttackStart(victim)):
-- boss-pos + irand(-14,14) offsets, type 1, 120s; AttackStart via
-- EngageWithTarget (same funnel note as onMedic); re-arms 25s in
-- PHASE_TWO (C++-exact).
local function onAdds(creature, guid)
    local bx, by, bz = creature:GetX(), creature:GetY(), creature:GetZ()
    local victim = creature:GetVictim()
    for _ = 1, 3 do
        local add = creature:SummonCreature(NPC_ANVILRAGE_ADD,
            bx + math.random(-14, 14), by + math.random(-14, 14), bz, 0, 1, 120000)
        if add and victim then
            add:EngageWithTarget(victim)
        end
    end
    local state = angerforgeState[guid]
    if state and state.phaseTwo then
        schedule(guid, "adds", 25000, function()
            onAdds(creature, guid)
        end)
    end
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    initState(guid)
    -- boss_general_angerforge.cpp JustEngagedWith schedule
    -- (PHASE_ONE, phase-agnostic casts).
    schedule(guid, "mightyblow", 8000, function()
        onMightyBlow(creature, guid)
    end)
    schedule(guid, "hamstring", 12000, function()
        onHamstring(creature, guid)
    end)
    schedule(guid, "cleave", 16000, function()
        onCleave(creature, guid)
    end)
end

local function onDamageTaken(_, creature, _, damage)
    local guid = creature:GetGUID()
    local state = angerforgeState[guid]
    if state == nil or state.phaseTwo then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    -- C++ HealthBelowPctDamaged(20, damage) one-shot latch
    -- (magmus precedent).
    local postDamage = creature:GetHealth() - damage
    if postDamage < math.floor(maxHealth * 20 / 100) then
        state.phaseTwo = true
        -- C++ schedules EVENT_MEDIC and EVENT_ADDS at 0s on the phase
        -- transition: fire both ticks immediately (CreateLuaEvent
        -- rejects delay 0; 0s = next UpdateAI tick in C++). But a
        -- lethal hit must NOT fire them: C++ executes 0s events on
        -- the next UpdateAI tick, and a dead creature never runs
        -- UpdateAI again, so the summons never happen in C++ (the
        -- phase latch itself still sets, as in C++).
        if postDamage > 0 then
            onMedic(creature, guid)
            onAdds(creature, guid)
        end
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

RegisterCreatureEvent(ENTRY, 5, onSpawn)
RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY, 4, onCombatEnd)
RegisterCreatureEvent(ENTRY, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY, 23, onReset)
