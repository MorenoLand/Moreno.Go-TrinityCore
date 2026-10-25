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
-- target = random alive threat-list unit (C++
-- SelectTarget(Random, 0) defaults, black stalker precedent),
-- nil pick
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
-- blocked). OnLeaveCombat(2)/OnReset(23): cancel the pump
-- and apply the C++ Reset() = Initialize() re-latch
-- (fireball 4000 / fly 45000; the ctor-only flight /
-- bellowingRoar / coneOfFire fields keep their values —
-- the _Reset arm is instance-blocked).
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
-- no live unit-state read bridge (IsCasting is constant-false — Go casts are fire-and-forget) — the C++ flight phase never calls
-- DoMeleeAttackIfReady but engine melee is always on in
-- this model (kargath in-blade precedent); the heroic-only
-- bellowing roar 39427 arm is ported below (IsHeroic() is
-- bridged — blood_prince_council precedent); no summon
-- bridge — the SpellHitTarget liquid-fire arm and the JustSummoned
-- liquid-fire setup are PORTED 2026-10-23 (nazanSpellHitTarget /
-- liquidFireSummoned below; the event-19 binding was FIXED 2026-10-24
-- (was registered on the summoned entry, never fired); the stale "no summon bridge" claim was
-- retracted — creature:SummonCreature is bridged,
-- lua_creature_events.go:728); the JustSummoned SetLevel leg is PORTED
-- via the new SetLevel bridge; no SpellScript/AuraScript scripts in this file.

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

local function evadeBoss(guid)
    -- C++ Reset() = Initialize() only: re-latch the
    -- Initialize() timers (fireball 4000 / fly 45000); the
    -- ctor-only flight / bellowingRoar / coneOfFire fields
    -- keep their values (C++ boss_nazanAI ctor vs Reset()).
    cancelPump(guid)
    local st = states[guid]
    if st then
        st.fireball = 4000
        st.fly = 45000
    end
end

-- C++ ctor + Initialize() values (the BossAI ctor instance
-- arms are instance blocked, so the whole engagement
-- schedule lands on OnEnterCombat — broggok precedent):
-- flight true / bellowingRoar 0 / fireball 4000 / fly
-- 45000 / turn 0; coneOfFire is 0 at spawn in C++ but the
-- land arm re-latches 12000 before the land-phase block
-- can consume it, so 12000 here is behaviorally exact.
local function freshState()
    return {
        flight = true,
        bellowingRoar = 0,
        fireball = 4000,
        fly = 45000,
        coneOfFire = 12000,
    }
end

-- C++ SelectTarget(Random, 0) (UnitAI.h:170 defaults
-- playerOnly=false, withTank=true, dist=0, aura=0; UnitAI.h
-- SelectTargetList skips offline refs and Creature.cpp
-- _IsTargetAcceptable rejects dead targets): random alive
-- unit from the boss's threat list, tank included, no
-- distance gate (black stalker precedent). Nil when the
-- threat list holds no alive unit — the caller skips the
-- cast (C++-exact).
local function pickThreatTarget(creature)
    local found = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if not t:IsDead() then
            found[#found + 1] = t
        end
    end
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
end

local function bossTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end

    -- Fireball 34653: 4s init then {4s,7s} — target = random
    -- alive threat-list unit (C++ SelectTarget(Random, 0)
    -- defaults, black stalker precedent), nil pick casts nothing,
    -- TRIGGERED DoCast on the target (C++-exact), re-arm
    -- {4s,7s} regardless (C++-exact) — fires in both phases.
    -- The SpellHitTarget liquid-fire summon arm is PORTED
    -- (nazanSpellHitTarget, ev15 — both bridges live).
    if st.fireball <= 1000 then
        local target = pickThreatTarget(creature)
        if target then
            creature:CastSpell(target, SPELL_FIREBALL, true)
        end
        st.fireball = math.random(4000, 7000)
    else
        st.fireball = st.fireball - 1000
    end

    if st.flight then
        -- Flight phase: the Turn_Timer waypoint hops have
        -- no movement bridge (documented above); the only
        -- bridgeable arm is the Fly_Timer 45s timeout —
        -- Talk(EMOTE), land, ConeOfFire 12000 (C++-exact;
        -- the expiry test is strict `<`, leotheras
        -- precedent).
        -- The Vazruden-alive/<20% early-land condition has
        -- no cross-creature bridge.
        if st.fly < 1000 then
            st.flight = false
            st.bellowingRoar = 6000
            st.coneOfFire = 12000
            creature:Talk(EMOTE)
        else
            st.fly = st.fly - 1000
        end
        return
    end

    -- Land phase: cone of fire 30926 — 12s init then 12s,
    -- non-triggered DoCast on self (C++-exact), re-arm
    -- 12000, Fireball_Timer reset to 4000 (C++-exact); the
    -- heroic-only bellowing roar 39427 arm is ported below
    -- (IsHeroic() is bridged — blood_prince_council
    -- precedent).
    if st.coneOfFire <= 1000 then
        creature:CastSpell(creature, SPELL_CONE_OF_FIRE)
        st.coneOfFire = 12000
        st.fireball = 4000
    else
        st.coneOfFire = st.coneOfFire - 1000
    end

    -- Bellowing roar 39427 (heroic only, C++-exact: armed 6s
    -- at land, then 45s, non-triggered self-cast —
    -- boss_vazruden_the_herald.cpp:175-183); the timer only
    -- ticks under IsHeroic(), like the C++.
    if creature:IsHeroic() then
        if st.bellowingRoar <= 1000 then
            creature:CastSpell(creature, SPELL_BELLOWING_ROAR)
            st.bellowingRoar = 45000
        else
            st.bellowingRoar = st.bellowingRoar - 1000
        end
    end
end

RegisterCreatureEvent(ENTRY_NAZAN, 1, function(_, creature)
    local guid = creature:GetGUID()
    cancelPump(guid)
    if not states[guid] then
        -- Fresh spawn or post-death re-engage: C++ ctor +
        -- Initialize() state.
        states[guid] = freshState()
    end
    -- Re-engage after evade resumes the Reset()-retained
    -- state (the Initialize() re-latch landed on the evade
    -- hook; C++ Reset() does not re-latch the ctor-only
    -- flight field, so a landed Nazan stays landed).
    timers[guid] = CreateLuaEvent(function()
        bossTick(creature, guid)
    end, 1000, 0)
end)

-- No event 3: the C++ has no KilledUnit override.

RegisterCreatureEvent(ENTRY_NAZAN, 2, function(_, creature)
    evadeBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_NAZAN, 4, function(_, creature)
    -- Death: full state drop — respawn re-engages with
    -- fresh ctor state (C++-exact).
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_NAZAN, 23, function(_, creature)
    evadeBoss(creature:GetGUID())
end)

-- C++ boss_nazanAI::SpellHitTarget (boss_vazruden_the_herald.cpp:113-117):
-- on SPELL_FIREBALL 34653 hit, SummonCreature(NPC_LIQUID_FIRE 22515) at
-- the target's position, TEMPSUMMON_TIMED_DESPAWN 30s. The spell-id gate
-- is bridged on the ev15 args (lethon precedent).
local function nazanSpellHitTarget(event, creature, target, spellId)
    if spellId == SPELL_FIREBALL and target then
        creature:SummonCreature(NPC_LIQUID_FIRE, target:GetX(), target:GetY(),
            target:GetZ(), target:GetO(), 2, 30000)
    end
end

-- C++ boss_nazanAI::JustSummoned (boss_vazruden_the_herald.cpp:103-112),
-- entry-gated on the SUMMON: event 19 fires on the SUMMONER's entry
-- bindings with (event, summoner, summon) (lua_creature_events.go:
-- fireCreatureSummoned) — the 2026-10-23 port registered it on the
-- summoned entry with a 2-arg handler, so it never fired; fixed
-- 2026-10-24 to the summoner entry + 3-arg handler. Arms (C++ order):
-- SetFaction(summoner faction) + the DUNGEON_MODE(23971, 30928)
-- triggered self-cast + FIRE_NOVA_VISUAL 19823 triggered self-cast;
-- SetLevel leg PORTED via the new SetLevel bridge (C++ order: level/faction/casts).
local function liquidFireSummoned(event, creature, summon)
    if summon and summon:GetEntry() == NPC_LIQUID_FIRE then
        summon:SetLevel(creature:GetLevel())
        summon:SetFaction(creature:GetFaction())
        if summon:IsHeroic() then
            summon:CastSpell(summon, SPELL_SUMMON_LIQUID_FIRE_H, true)
        else
            summon:CastSpell(summon, SPELL_SUMMON_LIQUID_FIRE, true)
        end
        summon:CastSpell(summon, SPELL_FIRE_NOVA_VISUAL, true)
    end
end

RegisterCreatureEvent(ENTRY_NAZAN, 15, nazanSpellHitTarget)
-- NPC_LIQUID_FIRE 22515 exists only via the SpellHitTarget summon above
-- (the yor precedent); the event-19 binding lives on the summoner.
RegisterCreatureEvent(ENTRY_NAZAN, 19, liquidFireSummoned)
