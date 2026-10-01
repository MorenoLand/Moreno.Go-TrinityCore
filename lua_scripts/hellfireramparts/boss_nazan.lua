-- Nazan (Hellfire Ramparts, Hellfire Citadel) --
-- Lua port of src/server/scripts/Outland/HellfireCitadel/
-- HellfireRamparts/boss_vazruden_the_herald.cpp (boss_nazan
-- — one of the CreatureScript AI classes the same file's
-- AddSC_boss_vazruden_the_herald registers; no
-- SpellScript/AuraScript scripts in this file).
-- Final boss in the Hellfire Ramparts set per
-- outland_script_loader.cpp order (instance_hellfire_
-- ramparts.cpp stays blocked on the instance-script model).
-- Entry: NPC_NAZAN = 17536 (hellfire_ramparts.h, verifiable
-- from the C++ sources); instance_hellfire_ramparts.cpp has
-- no OnCreatureCreate mapping for it — only OnGameObjectCreate
-- doors and a SetBossState DATA_NAZAN = 3 completion arm
-- (instance-side constant, unbridgeable). The
-- creature_template ScriptName bindings are DB-side (no TDB
-- in this workspace).
-- Talk: EMOTE = 0 (land — fired from the C++ flight -> land
-- transition arm, C++-exact). Nazan has no engage/kill/
-- death talk lines in the C++ (its JustEngagedWith override
-- is empty).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4
-- OnDied, 23 OnReset. Timers via CreateLuaEvent (per-GUID
-- scheduler pump); melee is engine-driven in Go (creature
-- combat tick), like C++ DoMeleeAttackIfReady.
-- Fight shape (C++-exact for all modeled arms): Nazan
-- (17536): OnEnterCombat(1): per-GUID reset to the C++ ctor
-- + Initialize() values (flight true / bellowingRoar 0 /
-- coneOfFire 0 / fireball 4000 / fly 45000 / turn 0 — the
-- BossAI::JustEngagedWith arm is instance-blocked, DATA_
-- NAZAN = 3 is unbridgeable) + 1s scheduler pump (a port of
-- UpdateAI in C++ arm order — 1s granularity exact for the
-- C++ timers; the !UpdateVictim early return collapses into
-- the pump). Pump: fireball 34653 — 4s init then {4s,7s},
-- target = random alive player in the instance (C++
-- SelectTarget(Random, 0), thespia precedent), nil pick
-- casts nothing, TRIGGERED DoCast on the target (C++
-- `DoCast(victim, SPELL_FIREBALL, true)`, C++-exact),
-- re-arm {4s,7s} regardless (C++-exact) — fires in both
-- flight and land phases (C++-exact). Flight phase: fly
-- timer 45000 decrements; on expiry — Talk(EMOTE), land
-- (flight = false, C++-exact latch) + ConeOfFire 12000
-- (C++-exact). The VazrudenRing waypoint hops (Turn_Timer
-- 10s, MovePoint to (-1430.0, 1705.0, 112.0) /
-- (-1377.0, 1760.0, 112.0)) are movement-blocked, so no
-- movement arms run (documented below). Land phase: cone
-- of fire 30926 — 12s init then 12s, non-triggered DoCast
-- on self (C++ DoCast(me), C++-exact), re-arm 12000,
-- Fireball_Timer reset to 4000 (C++-exact). Melee is
-- engine-driven.
-- OnDied(4): cleanup (the _JustDied arm is instance-
-- blocked). OnLeaveCombat(2)/OnReset(23): cancel the pump,
-- drop per-GUID state (the Reset() observable remainder —
-- the Initialize() re-latch — lands on OnEnterCombat; the
-- _Reset arm is instance-blocked).
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI
-- shim (instance_hellfire_ramparts.cpp stays blocked on the
-- instance-script model; the BossAI ctor DATA_NAZAN = 3
-- bookkeeping in Reset/JustEngagedWith/JustDied skipped);
-- no movement bridge — the whole flight waypoint machine
-- unmodeled (Turn_Timer MovePoint hops, the flight-phase
-- DoStartMovement/AttackStart re-acquire arms, the land
-- transition's SetDisableGravity(false)/SetWalk(true)/
-- MotionMaster Clear/AttackStart(nearest)/DoStartMovement
-- arms); no cross-creature bridge — the early-land
-- condition (VazrudenGUID via IsSummonedBy NPC_VAZRUDEN_
-- HERALD, Vazruden alive and above 20% health) unmodeled,
-- so only the 45s Fly_Timer timeout lands in this model
-- (the GUID latch from IsSummonedBy has no bridge either);
-- no unit-state bridge — the C++ flight phase never calls
-- DoMeleeAttackIfReady but engine melee is always on in
-- this model (kargath in-blade precedent); no difficulty
-- bridge — the heroic-only bellowing roar 39427 arm
-- unmodeled (thespia precedent); no summon bridge — the
-- SpellHitTarget arm (SummonCreature NPC_LIQUID_FIRE 22515
-- at the fireball target, TEMPSUMMON_TIMED_DESPAWN 30s) and
-- the JustSummoned liquid-fire setup (SetLevel/SetFaction/
-- triggered 23971/30928 + FIRE_NOVA_VISUAL 19823) unmodeled
-- (steamrigger precedent); no SpellScript/AuraScript
-- scripts in this file.

local SPELL_FIREBALL = 34653
local SPELL_CONE_OF_FIRE = 30926
local SPELL_BELLOWING_ROAR = 39427
local SPELL_SUMMON_LIQUID_FIRE = 23971
local SPELL_SUMMON_LIQUID_FIRE_H = 30928
local SPELL_FIRE_NOVA_VISUAL = 19823

local NPC_LIQUID_FIRE = 22515

local EMOTE = 0

local ENTRY_NAZAN = 17536

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

-- C++ ctor + Initialize() values (the BossAI ctor instance
-- arms are instance blocked, so the whole engagement
-- schedule lands on OnEnterCombat — broggok precedent):
-- flight true / bellowingRoar 0 / coneOfFire 0 /
-- fireball 4000 / fly 45000 / turn 0.
local function freshState()
    return {
        flight = true,
        fireball = 4000,
        fly = 45000,
        coneOfFire = 12000,
    }
end

local function playersInInstance(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            found[#found + 1] = p
        end
    end
    return found
end

local function bossTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end

    -- Fireball 34653: 4s init then {4s,7s} — target = random
    -- alive player in the instance (C++ SelectTarget(Random,
    -- 0), thespia precedent), nil pick casts nothing,
    -- TRIGGERED DoCast on the target (C++-exact), re-arm
    -- {4s,7s} regardless (C++-exact) — fires in both phases.
    -- The SpellHitTarget liquid-fire summon arm has no
    -- summon bridge (documented above).
    if st.fireball <= 1000 then
        local players = playersInInstance(creature)
        if #players > 0 then
            creature:CastSpell(players[math.random(#players)],
                SPELL_FIREBALL, true)
        end
        st.fireball = math.random(4000, 7000)
    else
        st.fireball = st.fireball - 1000
    end

    if st.flight then
        -- Flight phase: the Turn_Timer waypoint hops have
        -- no movement bridge (documented above); the only
        -- bridgeable arm is the Fly_Timer 45s timeout —
        -- Talk(EMOTE), land, ConeOfFire 12000 (C++-exact).
        -- The Vazruden-alive/<20% early-land condition has
        -- no cross-creature bridge.
        if st.fly <= 1000 then
            st.flight = false
            st.coneOfFire = 12000
            creature:Talk(EMOTE)
        else
            st.fly = st.fly - 1000
        end
        return
    end

    -- Land phase: cone of fire 30926 — 12s init then 12s,
    -- non-triggered DoCast on self (C++-exact), re-arm
    -- 12000, Fireball_Timer reset to 4000 (C++-exact). The
    -- heroic-only bellowing roar arm has no difficulty
    -- bridge (thespia precedent).
    if st.coneOfFire <= 1000 then
        creature:CastSpell(creature, SPELL_CONE_OF_FIRE)
        st.coneOfFire = 12000
        st.fireball = 4000
    else
        st.coneOfFire = st.coneOfFire - 1000
    end
end

RegisterCreatureEvent(ENTRY_NAZAN, 1, function(_, creature)
    local guid = creature:GetGUID()
    resetBoss(guid)
    states[guid] = freshState()
    timers[guid] = CreateLuaEvent(function()
        bossTick(creature, guid)
    end, 1000, 0)
end)

-- No event 3: the C++ has no KilledUnit override.

RegisterCreatureEvent(ENTRY_NAZAN, 2, function(_, creature)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_NAZAN, 4, function(_, creature)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_NAZAN, 23, function(_, creature)
    resetBoss(creature:GetGUID())
end)
