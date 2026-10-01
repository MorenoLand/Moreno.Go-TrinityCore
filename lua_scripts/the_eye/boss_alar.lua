-- Al'ar (The Eye, Tempest Keep) --
-- Lua port of src/server/scripts/Outland/TempestKeep/Eye/
-- boss_alar.cpp (boss_alar — the only CreatureScript AI class
-- AddSC_boss_alar registers; npc_ember_of_alar (19551) and
-- npc_flame_patch_alar (20602) documented only — their AIs sit
-- behind summon / cross-creature bridges, see below; the
-- spell_alar_flame_quills AuraScript has no AuraScript bridge
-- — AuraScript check handlers are not modeled, standing
-- blocker, documented only).
-- First boss in The Eye set per outland_script_loader.cpp
-- order (boss_alar, boss_kaelthas, boss_void_reaver, boss_
-- high_astromancer_solarian).
-- Entry: 19514 (NPC_ALAR — the_eye.h, verifiable from the C++
-- sources; DATA_ALAR = 1 — instance-side constant,
-- unbridgeable). instance_the_eye.cpp OnCreatureCreate maps
-- NPC_ALAR to Alar (verified). The creature_template
-- ScriptName bindings are DB-side (no TDB in this workspace).
-- Talk: boss_alar.cpp defines no SAY_ enums — no talks fire
-- anywhere in the fight (C++-exact).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4
-- OnDied, 9 OnDamageTaken, 23 OnReset (the C++ does not
-- override KilledUnit — no event 3, laj precedent).
-- Timers via a 1s CreateLuaEvent pump (a port of UpdateAI in
-- C++ arm order); melee is engine-driven in Go (creature
-- combat tick), like C++ DoMeleeAttackIfReady. C++ DoCast
-- default is triggered=false (vaelastrasz convention):
-- creature:CastSpell(target, spell) = non-triggered DoCast,
-- creature:CastSpell(target, spell, true) = TRIGGERED DoCast,
-- creature:CastSpell(nil, spell) = DoCastVictim.
-- Fight shape (C++-exact for all modeled arms): Al'ar (19514):
-- OnEnterCombat(1): per-GUID reset to the C++ ctor /
-- Initialize() values (berserk 1200000 / platforms-move 0 /
-- phase = 1 — the BossAI ctor DATA_ALAR = 1 arm is instance-
-- blocked, unbridgeable; the JustEngagedWith SetDisableGravity
-- / setActive arms have no gravity/activation bridge) + 1s
-- scheduler pump.
-- Pump (C++ UpdateAI arm order):
-- 1. Berserk arm — ticks in every state (C++ decrements it
--    before the WaitEvent machine): 1200000 init then 60000:
--    TRIGGERED DoCast(me, SPELL_BERSERK 45078, true)
--    (C++-exact), re-arm 60000 (C++-exact).
-- 2. WE_DIE -> WE_REVIVE sequence (the DamageTaken latch, see
--    OnDamageTaken): 5s (the C++ WE_DIE arm) then the WE_
--    REVIVE arm — TRIGGERED DoCast(me, SPELL_REBIRTH 34342,
--    true) (C++-exact) + phase = 2 + melt armor 60000 /
--    charge 7000 / dive bomb 40000+rand()%5000 (the flame-
--    patch summon arm has no summon bridge — steamrigger
--    precedent — so its timer is not tracked; the WE_DIE /
--    WE_REVIVE SetHealth/SetStandState/flag/speed/MovePoint
--    arms have no health/stand/flag/speed/movement bridges —
--    unmodeled; the DoZoneInCombat arm is engine-driven).
--    While the sequence runs, all damage is zeroed — the C++
--    UNIT_FLAG_NON_ATTACKABLE arm (the flag itself has no
--    bridge, documented below).
-- 3. Dive-bomb chain (WE_METEOR -> WE_DIVE -> WE_LAND ->
--    WE_SUMMON, C++ order): meteor — non-triggered DoCast(me,
--    SPELL_DIVE_BOMB_VISUAL 35367) (C++-exact; the SpellHit
--    SetDisplayId/ApplySpellImmune arms have no display/
--    immune bridge — unmodeled; the ascent MovePoint has no
--    movement bridge — the 5s meteor wait approximates the
--    flight, nightbane precedent); dive (+4s, C++-exact) —
--    TRIGGERED DoCast(target, SPELL_DIVE_BOMB 35181, true) on
--    a random alive player in the instance (C++ SelectTarget
--    (Random, 0), thespia precedent; the RemoveAurasDueToSpell
--    arm has no aura bridge and the UpdatePosition teleport
--    has no movement bridge — unmodeled), no target -> evade
--    (C++ EnterEvadeMode — the evade hooks land the reset);
--    land (+1s — C++ WaitTimer = 1000 + floor(dist/80*1000),
--    dist in {3.0, 5.0} -> {1037, 1062}ms, 1s granularity
--    exact); summon (+2s, C++-exact) — the two CREATURE_EMBER_
--    OF_ALAR 19551 summons have no summon bridge (steamrigger
--    precedent), only the TRIGGERED DoCast(me, SPELL_REBIRTH_
--    2 35369, true) is bridged (C++-exact; the bounding-
--    radius/display/flag arms have no bridges — unmodeled).
-- 4. Phase 1 arms: platforms-move cycle — 0 init then 30000+
--    rand()%5000: the platform MovePoints, the ember
--    DoSpawnCreature and the cur_wp bookkeeping have no
--    movement/summon bridge (steamrigger / omor precedents);
--    the only bridgeable output is the 20% branch — urand(0,
--    4) == 0 (C++-exact): TRIGGERED DoCast(me, SPELL_FLAME_
--    QUILLS 34229, true) (C++-exact, the WE_QUILL arm). The
--    phase-1 !IsThreatened() evade arm has no threat model —
--    unmodeled (nightbane precedent).
-- 5. Phase 2 arms (C++ arm order): charge — 30s: non-
--    triggered DoCast(target, SPELL_CHARGE 35412) on a random
--    alive player in the instance within 100 yd excluding the
--    current victim (C++ SelectTarget(Random, 1, 100, true),
--    najentus precedent; nil pick casts nothing — C++-exact),
--    re-arm 30000 regardless (C++-exact); melt armor — 60s:
--    non-triggered DoCastVictim(SPELL_MELT_ARMOR 35410)
--    (C++-exact), re-arm 60000 (C++-exact); dive bomb —
--    40000+rand()%5000: starts the dive chain above, re-arm
--    40000+rand()%5000 (C++-exact); flame patch — 30s: the
--    whole arm is a CREATURE_FLAME_PATCH_ALAR 20602 summon +
--    summon-side setup/cast (no summon bridge — steamrigger
--    precedent), unmodeled; the DoMeleeAttackIfReady flame-
--    buffet arm (SPELL_FLAME_BUFFET 34121 — no victim in melee
--    range) has no attack-timer/melee-range bridge — melee is
--    engine-driven, unmodeled.
-- OnDamageTaken(9): the C++ DamageTaken latch — damage >=
-- current health while phase 1 and no WaitEvent pending:
-- the rewrite zeroes the hit (C++ damage = 0 — the shim's
-- second-return damage rewrite, combat.go:508), the latch
-- fires once (!WaitEvent — C++-exact), the phase-1 schedule
-- is dropped and the WE_DIE -> WE_REVIVE sequence starts.
-- OnDied(4): cleanup (the _JustDied arm is instance-blocked).
-- OnLeaveCombat(2)/OnReset(23): cancel the pump, drop per-GUID
-- state (the C++ Reset() observable remainder — the
-- Initialize() re-latch — lands on OnEnterCombat, gargolmar
-- precedent; the _Reset arm is instance-blocked; Eluna's On_
-- Reset fires ahead of OnDied and OnSpawn — millhouse note —
-- so both hooks land the same reset).
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI
-- shim (instance_the_eye.cpp stays blocked on the instance-
-- script model; the BossAI ctor DATA_ALAR = 1 bookkeeping in
-- Reset/JustEngagedWith/JustDied skipped); no movement bridge
-- — the phase-1 platform MovePoints, the ForceMove re-issue,
-- the WE_DIE waypoint[5] MovePoint, the dive-bomb waypoint[4]
-- MovePoint and the WE_DIVE UpdatePosition teleport unmodeled
-- (omor / nightbane precedents); no summon bridge — the
-- phase-1 ember DoSpawnCreature, the dive-bomb two-ember
-- DoSpawnCreature and the whole flame-patch arm unmodeled
-- (steamrigger precedent); npc_ember_of_alar (19551)
-- documented only, not registered — its DamageTaken latch
-- (TRIGGERED SPELL_EMBER_BLAST 34133 + the 3% Al'ar health
-- cut via instance->GetGuidData(DATA_ALAR)) sits behind the
-- summon bridge (unreachable — nothing spawns it) and the
-- cross-creature/instance bridges (skyriss-illusion /
-- warp_splinter-treant precedent); npc_flame_patch_alar
-- (20602) documented only, not registered — its C++ AI is a
-- no-op (skyriss-illusion precedent); no SpellScript/
-- AuraScript bridge — spell_alar_flame_quills (the 24 flame-
-- quill sub-spells 34269-34289 / 34314-34316 PeriodicTick)
-- documented only, not bridged (standing blocker); no
-- display/immune/flag/speed/stand-state/attack-state bridges —
-- the Reset() and phase arms touching SetDisplayId/
-- SetSpeedRate/ApplySpellImmune/SetDisableGravity/UNIT_FLAG_*
-- /SetStandState/IsNonMeleeSpellCast unmodeled (netherspite
-- precedent).

local SPELL_FLAME_QUILLS = 34229
local SPELL_REBIRTH = 34342
local SPELL_REBIRTH_2 = 35369
local SPELL_MELT_ARMOR = 35410
local SPELL_CHARGE = 35412
local SPELL_DIVE_BOMB_VISUAL = 35367
local SPELL_DIVE_BOMB = 35181
local SPELL_BERSERK = 45078

local ENTRY_ALAR = 19514

local alarState = {}
local alarPump = {}

local function cancelPump(guid)
    local id = alarPump[guid]
    if id then
        RemoveEventById(id)
        alarPump[guid] = nil
    end
end

-- C++ Reset() (called on evade): the observable remainder —
-- the Initialize() re-latch — lands nowhere; the re-latch
-- lands on OnEnterCombat (gargolmar precedent).
local function fullReset(guid)
    cancelPump(guid)
    alarState[guid] = nil
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

-- C++ SelectTarget(Random, 0): any alive player in the instance
-- (thespia precedent).
local function randomAlivePlayer(creature)
    local players = playersInInstance(creature)
    if #players == 0 then
        return nil
    end
    return players[math.random(#players)]
end

-- C++ SelectTarget(Random, 1, 100, true): any alive player in the
-- instance within 100 yd, excluding the current victim (position
-- 0) — najentus precedent.
local function randomChargeTarget(creature)
    local victim = creature:GetVictim()
    local victimGUID = victim and victim:GetGUID() or 0
    local candidates = {}
    for _, p in ipairs(playersInInstance(creature)) do
        if p:GetGUID() ~= victimGUID and creature:GetDistance(p) <= 100 then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- The dive-bomb chain (C++ WE_METEOR -> WE_DIVE -> WE_LAND ->
-- WE_SUMMON, in order); called when the current step's wait
-- expires. The ascent MovePoint has no movement bridge — the
-- 5s meteor wait approximates the flight (nightbane precedent).
local function advanceDive(creature, guid)
    local st = alarState[guid]
    if not st or not st.dive then
        return
    end
    local step = st.dive.step
    if step == "meteor" then
        -- WE_METEOR: non-triggered self cast (C++-exact).
        creature:CastSpell(creature, SPELL_DIVE_BOMB_VISUAL)
        st.dive = { step = "dive", wait = 4000 }
    elseif step == "dive" then
        -- WE_DIVE: TRIGGERED dive bomb on a random target
        -- (C++-exact); no target -> EnterEvadeMode (C++-exact —
        -- the evade hooks land the reset).
        local target = randomAlivePlayer(creature)
        if not target then
            fullReset(guid)
            return
        end
        creature:CastSpell(target, SPELL_DIVE_BOMB, true)
        st.dive = { step = "land", wait = 1000 }
    elseif step == "land" then
        -- WE_LAND -> WE_SUMMON after 2s (C++-exact).
        st.dive = { step = "summon", wait = 2000 }
    elseif step == "summon" then
        -- WE_SUMMON: the two ember summons have no summon
        -- bridge (steamrigger precedent) — only the TRIGGERED
        -- rebirth cast is bridged (C++-exact).
        creature:CastSpell(creature, SPELL_REBIRTH_2, true)
        st.dive = nil
    end
end

-- C++ UpdateAI in event arm order — 1s granularity exact for
-- all C++ timers; the !IsEngaged early return collapses into
-- the pump (it only runs in combat); the berserk arm is
-- decremented before the WaitEvent machine (C++-exact).
local function combatTick(creature, guid)
    local st = alarState[guid]
    if not st then
        return
    end

    -- 1. Berserk arm: 1200000 init then 60000 — TRIGGERED
    -- DoCast(me, SPELL_BERSERK, true) (C++-exact).
    if st.berserk <= 1000 then
        creature:CastSpell(creature, SPELL_BERSERK, true)
        st.berserk = 60000
    else
        st.berserk = st.berserk - 1000
    end

    -- 2. WE_DIE -> WE_REVIVE sequence (the DamageTaken latch):
    -- 5s then the WE_REVIVE arm — TRIGGERED DoCast(me, SPELL_
    -- REBIRTH, true) + phase 2 schedule (C++-exact).
    if st.dying then
        if st.revive <= 1000 then
            st.dying = false
            st.phase = 2
            creature:CastSpell(creature, SPELL_REBIRTH, true)
            st.meltArmor = 60000
            st.charge = 7000
            st.diveBomb = 40000 + math.random(0, 4999)
        else
            st.revive = st.revive - 1000
        end
        return
    end

    -- 3. Dive-bomb chain: while it runs, the other arms pause
    -- (C++ WaitEvent -> return).
    if st.dive then
        if st.dive.wait <= 1000 then
            advanceDive(creature, guid)
        else
            st.dive.wait = st.dive.wait - 1000
        end
        return
    end

    if st.phase == 1 then
        -- 4. Platforms-move cycle — 0 init then 30000+rand()%
        -- 5000: the only bridgeable output is the 20% flame-
        -- quills branch (C++-exact); the movement/summon arms
        -- have no bridge.
        if st.platform <= 1000 then
            if math.random(0, 4) == 0 then
                creature:CastSpell(creature, SPELL_FLAME_QUILLS, true)
            end
            st.platform = 30000 + math.random(0, 4999)
        else
            st.platform = st.platform - 1000
        end
    else
        -- 5. Phase 2 arms in C++ arm order.
        -- Charge arm — 30s: non-triggered DoCast on the random
        -- charge target; nil pick casts nothing; re-arm 30000
        -- regardless (C++-exact).
        if st.charge <= 1000 then
            local target = randomChargeTarget(creature)
            if target then
                creature:CastSpell(target, SPELL_CHARGE)
            end
            st.charge = 30000
        else
            st.charge = st.charge - 1000
        end
        -- Melt armor arm — 60s: non-triggered DoCastVictim
        -- (C++-exact), re-arm 60000 (C++-exact).
        if st.meltArmor <= 1000 then
            creature:CastSpell(nil, SPELL_MELT_ARMOR)
            st.meltArmor = 60000
        else
            st.meltArmor = st.meltArmor - 1000
        end
        -- Dive bomb arm — 40000+rand()%5000: starts the dive
        -- chain, re-arm 40000+rand()%5000 (C++-exact).
        if st.diveBomb <= 1000 then
            st.dive = { step = "meteor", wait = 5000 }
            st.diveBomb = 40000 + math.random(0, 4999)
        else
            st.diveBomb = st.diveBomb - 1000
        end
        -- Flame patch arm: summon-only, no summon bridge —
        -- unmodeled (steamrigger precedent).
    end
end

-- C++ ctor / Initialize() values land on OnEnterCombat:
-- berserk 1200000 / platforms-move 0 / phase = 1 (the BossAI
-- ctor DATA_ALAR = 1 arm is instance-blocked, unbridgeable).
RegisterCreatureEvent(ENTRY_ALAR, 1, function(_, creature)
    local guid = creature:GetGUID()
    cancelPump(guid)
    alarState[guid] = {
        phase = 1,
        berserk = 1200000,
        platform = 0,
        dying = false,
        dive = nil,
    }
    alarPump[guid] = CreateLuaEvent(function()
        combatTick(creature, guid)
    end, 1000, 0)
end)

-- C++ Reset() (called on evade): fullReset — see above.
RegisterCreatureEvent(ENTRY_ALAR, 2, function(_, creature)
    fullReset(creature:GetGUID())
end)

-- C++ JustDied: cleanup (the _JustDied arm is instance-
-- blocked; the C++ does not override KilledUnit, so no
-- event 3 — laj precedent).
RegisterCreatureEvent(ENTRY_ALAR, 4, function(_, creature)
    fullReset(creature:GetGUID())
end)

-- C++ DamageTaken: the phase-1 lethal latch — damage >=
-- current health while phase 1 and no WaitEvent pending:
-- zero the hit (C++ damage = 0 — the shim's second-return
-- damage rewrite, combat.go:508), latch once (!WaitEvent —
-- C++-exact) and start the WE_DIE -> WE_REVIVE sequence.
-- While the sequence runs, all damage is zeroed — the C++
-- UNIT_FLAG_NON_ATTACKABLE arm (no flag bridge, documented
-- above).
RegisterCreatureEvent(ENTRY_ALAR, 9, function(_, creature, attacker, damage)
    local guid = creature:GetGUID()
    local st = alarState[guid]
    if not st then
        return
    end
    if st.dying then
        return 0, 0
    end
    if st.phase == 1 and damage >= creature:GetHealth() then
        st.dying = true
        st.revive = 5000
        return 0, 0
    end
end)

-- Eluna's On_Reset fires ahead of OnDied and OnSpawn, so
-- this lands the same Reset() reset as the evade hook.
RegisterCreatureEvent(ENTRY_ALAR, 23, function(_, creature)
    fullReset(creature:GetGUID())
end)
