-- High Astromancer Solarian (The Eye, Tempest Keep) --
-- Lua port of src/server/scripts/Outland/TempestKeep/Eye/
-- boss_astromancer.cpp (boss_high_astromancer_solarian — the
-- only boss CreatureScript AI class AddSC_boss_high_
-- astromancer_solarian registers; npc_solarium_priest (18806)
-- PORTED in npc_solarium_priest.lua (tick 292) — its heal arm
-- reads GetInstanceCreature(4), nil when unlatched (C++-exact
-- per the tick-292 spec correction); spawned by the ported
-- phase-3 machine; the spell_astromancer_wrath_of_the_
-- astromancer AuraScript is PORTED below (tick 424,
-- RegisterAuraEvent ev3, expire-only).
-- Fourth boss in The Eye set per outland_script_loader.cpp
-- order (boss_alar, boss_kaelthas, boss_void_reaver, boss_
-- high_astromancer_solarian).
-- Entry: 18805 (NPC_HIGH_ASTROMANCER_SOLARIAN — the_eye.h,
-- verifiable from the C++ sources; DATA_HIGH_ASTROMANCER_
-- SOLARIAN = 2 — instance-side constant, unbridgeable).
-- instance_the_eye.cpp OnCreatureCreate maps NPC_HIGH_
-- ASTROMANCER_SOLARIAN to Astromancer (verified). The
-- creature_template ScriptName bindings are DB-side (no TDB
-- in this workspace).
-- Talk lines used: SAY_AGGRO = 0 (pull — fired from the C++
-- JustEngagedWith, C++-exact), SAY_KILL = 3 (kill — the C++
-- KilledUnit talks unconditionally, no player gate,
-- gargolmar precedent, C++-exact), SAY_DEATH = 4 (death —
-- fired from the C++ JustDied, C++-exact), SAY_VOIDA = 5 and
-- SAY_VOIDB = 6 (void — fired from the 20% phase-4 latch,
-- C++-exact); SAY_SUMMON1 = 1 / SAY_SUMMON2 = 2 fire from the
-- ported Phase 2/3 portal machine below (C++-exact).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset (the C++ does not
-- override DamageTaken — no event 9; the 20% phase latch is a
-- per-tick HealthBelowPct check, ported into the pump,
-- C++-exact).
-- Timers via a 1s CreateLuaEvent pump (a port of UpdateAI in
-- C++ arm order); melee is engine-driven in Go (creature
-- combat tick), like C++ DoMeleeAttackIfReady. C++ DoCast
-- default is triggered=false (vaelastrasz convention):
-- creature:CastSpell(target, spell) = non-triggered DoCast,
-- creature:CastSpell(target, spell, true) = TRIGGERED DoCast,
-- creature:CastSpell(nil, spell) = DoCastVictim.
-- Fight shape (C++-exact for all modeled arms): Solarian
-- (18805): OnEnterCombat(1): per-GUID reset to the C++ ctor /
-- Initialize() values (arcane missiles 2000 / blinding light
-- 41000 / wrath 20000+rand()%5000 / wrath-of-the-astromancer
-- 15000 / fear 20000 / void bolt 10000 / phase = 1 — the
-- BossAI ctor DATA_HIGH_ASTROMANCER_SOLARIAN = 2 arm is
-- instance-blocked, unbridgeable; the ctor defaultarmor /
-- defaultsize arms: the armor arm is ported below via
-- creature:GetArmor (armor bridge, lua_creature_events.go) and the
-- scale arm is bridged (creature:SetScale) and ported
-- below) + Talk(SAY_AGGRO) + 1s scheduler pump.
-- Pump (C++ UpdateAI arm order — Phase == 1 branch):
-- 1. Blinding light arm — 41s init then 45s: BlindingLight =
--    true (C++-exact); the next arcane-missiles arm then
--    casts SPELL_BLINDING_LIGHT on the victim instead of
--    missiles.
-- 2. Wrath arm — 20000+rand()%5000 init then 20000+rand()%
--    5000: InterruptNonMeleeSpells(false) PORTED (live cancel bridge lua_creature_events.go:1995);
--    target = SelectTarget (Random, 1, 100, true) — random
--    threat-list player within 100 yd, first MaxThreat entry
--    skipped before filters (GetAITargets pick); TRIGGERED
--    DoCast(target, SPELL_
--    WRATH_OF_THE_ASTROMANCER 42783, true) (C++-exact),
--    re-arm 20000+rand()%5000 regardless of target pick
--    (C++-exact).
-- 3. Arcane missiles arm — 2000 init then 3000: if
--    BlindingLight: non-triggered DoCastVictim(SPELL_
--    BLINDING_LIGHT 33009) (C++-exact), BlindingLight =
--    false; else target = SelectTarget(Random, 0) — random
--    threat-list unit, victim eligible (position 0); the
--    !HasInArc(2.5f) victim-fallback arm is PORTED via the
--    inArc twin (arc 2.5);
--    non-triggered DoCast(target, SPELL_ARCANE_MISSILES
--    33031) (C++-exact), re-arm 3000 regardless (C++-exact).
-- 4. Wrath-of-the-astromancer arm — 15000 init: InterruptNon
--    MeleeSpells(false) PORTED (live cancel bridge, see :298/:343); target =
--    SelectTarget(Random, 1) — random threat-list unit,
--    first MaxThreat entry skipped before filters; the C++
--    re-arms
--    1000 for non-player (pet) picks (threat list, playerOnly
--    = false, so the pet leg is reachable);
--    non-triggered DoCast(target, SPELL_WRATH_OF_THE_
--    ASTROMANCER 42783) (C++-exact), re-arm 25000 on a player
--    pick; no pick leaves the timer expired for a next-tick
--    retry (C++-exact).
-- 5. Portal transition arm (Phase1_Timer 50000): the whole
--    Phase 2/3 portal machine IS ported below (tick 293) —
--    the UpdatePosition teleports (creature:Teleport,
--    alar precedent), the NPC_ASTROMANCER_SOLARIAN_SPOTLIGHT
--    18928 / NPC_SOLARIUM_AGENT 18925 / NPC_SOLARIUM_PRIEST
--    18806 summons (SummonCreature returns the summoned
--    creature; agents/priests AttackStart a random threat-list
--    unit via the EngageWithTarget funnel; the priest's own Lua
--    drives its combat once engaged; agents have no C++ AI class
--    — default AI, melee only), the SetVisible/SetFlag(NOT_SELECTABLE)
--    arms and the AppearDelay/StopMoving/AttackStop machine.
--    Motion-master Clear rides the StopMoving (MoveClear) twin.
--    SAY_SUMMON1/SAY_SUMMON2 fire from the machine (C++-exact).
-- Phase 4 latch (C++ arm order — after the phase branches,
-- every tick): if Phase ~= 4 and HealthBelowPct(20): Phase =
-- 4 + Talk(SAY_VOIDA) + Talk(SAY_VOIDB) (C++-exact); the
-- RemoveFlag(NOT_SELECTABLE) + SetVisible(true) guarantee arms ARE
-- ported (2026-10-23); non-vacuous now that the phase 2/3 machine
-- conceals her — the latch can fire mid-phase-2/3; the
-- SetArmor(31000) arm IS ported below via creature:SetArmor
-- (armor bridge, lua_creature_events.go); the
-- SetDisplayId(MODEL_VOIDWALKER 18988) arm IS ported (the stale
-- "paired with the reveal" rationale retracted 2026-10-21; the
-- phase-4 reveal is non-vacuous now that the phase 2/3 machine
-- conceals her);
-- the SetObjectScale(x2.5) arm is ported below via
-- creature:SetScale (scale bridge now exists).
-- Phase 4 arms (C++ arm order): fear — 20s: non-triggered
-- DoCast(me, SPELL_FEAR 34322) (C++-exact), re-arm 20000
-- (C++-exact); void bolt — 10s: non-triggered DoCastVictim
-- (SPELL_VOID_BOLT 39329) (C++-exact), re-arm 10000
-- (C++-exact). Melee is engine-driven.
-- OnTargetDied(3): Talk(SAY_KILL) unconditionally (no player
-- gate, C++-exact). OnDied(4): Talk(SAY_DEATH) + cleanup
-- (the _JustDied SetBossState(DONE) arm IS ported below —
-- stale "instance-blocked" retracted 2026-11-07,
-- creature:SetBossState is bridged); the SetObjectScale
-- restore arm is ported as a scale clear, and the
-- RemoveFlag(NOT_SELECTABLE) + SetVisible(true) guarantee arms are
-- ported too (2026-10-23 — visibility bridge is live; the stale
-- "no visibility bridge" claim is retracted).
-- OnLeaveCombat(2)/OnReset(23): cancel the pump, drop
-- per-GUID state (the C++ Reset() observable remainder —
-- the Initialize() re-latch — lands on OnEnterCombat,
-- gargolmar precedent; the SetArmor arm IS ported below (armor bridge);
-- the RemoveFlag(NOT_SELECTABLE) + SetVisible(true) guarantee arms and
-- the SetDisplayId restore ARE ported (2026-10-23 — visibility bridge
-- is live; the stale "no visibility bridge / paired with the reveal"
-- claims are retracted), plus the SetObjectScale restore as a scale
-- clear;
-- the _Reset arm is instance-
-- blocked; Eluna's On_Reset fires ahead of OnDied and
-- OnSpawn — millhouse note — so both hooks land the same
-- reset).
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI
-- shim (instance_the_eye.cpp stays blocked on the instance-
-- script model; the BossAI ctor DATA_HIGH_ASTROMANCER_
-- SOLARIAN = 2 bookkeeping in Reset/JustEngagedWith is
-- skipped, but the JustDied SetBossState(DONE) arm IS
-- ported below (creature:SetBossState, bridged)); the phase
-- 2/3 portal machine IS ported (tick 293):
-- UpdatePosition via creature:Teleport (alar precedent),
-- motion-master Clear via the StopMoving (MoveClear) twin,
-- spotlight/agent/priest summons via SummonCreature with the
-- returned creature's AttackStart/CastSpell/SetFlag legs
-- (EngageWithTarget funnel); the SetArmor arms ARE ported
-- (phase-4 set + Reset restore, armor bridge,
-- lua_creature_events.go); the SetDisplayId arms are ported,
-- SetObjectScale ported via the scale bridge; the HasInArc
-- victim fallback IS ported via the IsInBack twin; no cast-
-- state bridge — the remaining InterruptNonMeleeSpells arms
-- unmodeled, casts fire unconditionally (netherspite
-- precedent); the wrath arms now pick from the threat list
-- (GetAITargets), including the non-player 1s re-arm;
-- npc_solarium_priest
-- (18806) PORTED in npc_solarium_priest.lua (tick 292); the
-- spell_astromancer_wrath_of_the_astromancer AuraScript is
-- PORTED below (tick 424, RegisterAuraEvent ev3, expire-only).

local SPELL_ARCANE_MISSILES = 33031
local SPELL_WRATH_OF_THE_ASTROMANCER = 42783
local SPELL_BLINDING_LIGHT = 33009
local SPELL_FEAR = 34322
local SPELL_VOID_BOLT = 39329
local SPELL_SPOTLIGHT = 25824 -- boss_astromancer.cpp:53

-- Portal machine constants (boss_astromancer.cpp:81-88).
local CENTER_X = 432.909
local CENTER_Y = -373.424
local CENTER_Z = 17.9608
local CENTER_O = 1.06421
local SMALL_PORTAL_RADIUS = 12.6
local LARGE_PORTAL_RADIUS = 26.0
local PORTAL_Z = 17.005

local NPC_SPOTLIGHT = 18928 -- boss_astromancer.cpp:64
local NPC_SOLARIUM_AGENT = 18925 -- boss_astromancer.cpp:66
local NPC_SOLARIUM_PRIEST = 18806 -- boss_astromancer.cpp:67
local SUMMON_TIMED_DESPAWN = 2 -- Go 0-based TempSummonType (spells.go)
local SUMMON_TIMED_DESPAWN_OUT_OF_COMBAT = 3 -- Go 0-based TempSummonType

local ENTRY_SOLARIAN = 18805
-- the_eye.h:31 (TEDataTypes): DATA_HIGH_ASTROMANCER_SOLARIAN = 2.
local DATA_HIGH_ASTROMANCER_SOLARIAN = 2

local MODEL_HUMAN = 18239 -- boss_astromancer.cpp:72
local MODEL_VOIDWALKER = 18988 -- boss_astromancer.cpp:73
local WV_ARMOR = 31000 -- boss_astromancer.cpp:78
local UNIT_FIELD_FLAGS = 59 -- UpdateFields.h:117 (OBJECT_END+0x35)
local UNIT_FLAG_NOT_SELECTABLE = 33554432 -- 0x02000000, UnitDefines.h:149

local SAY_AGGRO = 0
local SAY_SUMMON1 = 1
local SAY_SUMMON2 = 2
local SAY_KILL = 3
local SAY_DEATH = 4
local SAY_VOIDA = 5
local SAY_VOIDB = 6

local solarianState = {}
local solarianPump = {}
-- C++ ctor capture (boss_astromancer.cpp:105): defaultarmor read once
-- per GUID at first combat entry; nothing changes armor pre-combat,
-- so the first-enterCombat read is the ctor value, and it survives
-- evade/death cycles (Reset restores it, JustDied does not touch it).
local solarianDefaultArmor = {}

local function cancelPump(guid)
    local id = solarianPump[guid]
    if id then
        RemoveEventById(id)
        solarianPump[guid] = nil
    end
end

-- C++ Reset(): the observable remainder — the Initialize()
-- re-latch — lands on OnEnterCombat (gargolmar precedent).
local function fullReset(guid)
    cancelPump(guid)
    solarianState[guid] = nil
end

-- C++ SelectTarget(Random, 0): any threat-list unit, victim
-- eligible (position 0 = no skip); dead filtered (the threat
-- manager drops them, same as the wrath helpers below).
local function randomThreatUnit(creature)
    local pool = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if not t:IsDead() then
            pool[#pool + 1] = t
        end
    end
    if #pool == 0 then
        return nil
    end
    return pool[math.random(#pool)]
end

-- C++ SelectTarget(Random, 1, 100, true) / SelectTarget(Random, 1):
-- UnitAI.h builds the list victim-first, so offset 1 removes the
-- victim, not the threat head; then the playerOnly/maxDist filters
-- (DefaultTargetSelector) and the dead-offline filter apply.
local function randomThreatTarget(creature, playerOnly, maxDist)
    local victim = creature:GetVictim()
    local victimGuid = nil
    if victim ~= nil then
        victimGuid = victim:GetGUID()
    end
    local targets = creature:GetAITargets()
    local pool = {}
    local aliveSeen = 0
    for _, t in ipairs(targets) do
        local targetReach = 1.5
        if t:GetObjectType() ~= "Player" then
            targetReach = t:GetCombatReach()
        end
        local skip = false
        if victimGuid ~= nil then
            skip = t:GetGUID() == victimGuid
        elseif not t:IsDead() then
            aliveSeen = aliveSeen + 1
            skip = aliveSeen == 1
        end
        if not skip
                and not t:IsDead()
                and (not playerOnly or t:GetObjectType() == "Player")
                and (not maxDist or creature:GetDistance(t) - creature:GetCombatReach() - targetReach < maxDist) then -- IsWithinCombatRange gate, reach-exact (Unit.cpp:583; mixed pool branches target reach)
            pool[#pool + 1] = t
        end
    end
    if #pool == 0 then
        return nil
    end
    return pool[math.random(#pool)]
end

-- C++ Portal_X (boss_astromancer.cpp:198-203): urand(0, 1)
-- flips the radius sign, then radius * (rand32() % 100) / 100
-- offset from CENTER_X.
local function portalX(radius)
    local r = radius
    if math.random(0, 1) == 1 then
        r = -r
    end
    return r * math.random(0, 99) / 100 + CENTER_X
end

-- C++ Portal_Y (boss_astromancer.cpp:205-210): RAND(1.0f,
-- -1.0f) times the circle residual, offset from CENTER_Y.
local function portalY(x, radius)
    local z = math.random() * 2 - 1
    local dx = x - CENTER_X
    return z * math.sqrt(radius * radius - dx * dx) + CENTER_Y
end

-- C++ SummonMinion (boss_astromancer.cpp:177-186):
-- SummonCreature(entry, x, y, z, 0, TEMPSUMMON_TIMED_DESPAWN_
-- OUT_OF_COMBAT, 5s); the summon then AttackStarts a random
-- threat-list unit (SelectTarget Random, 0). The o = 0 falls
-- back to the summoner orientation in the Go bridge.
local function summonMinion(creature, entry, x, y, z)
    local summoned = creature:SummonCreature(entry, x, y, z, 0,
        SUMMON_TIMED_DESPAWN_OUT_OF_COMBAT, 5000)
    if summoned then
        local target = randomThreatUnit(creature)
        if target then
            summoned:AttackStart(target)
        end
    end
end

-- C++ UpdateAI in arm order — 1s granularity exact for all
-- C++ timers; the !UpdateVictim early return collapses into
-- the pump (it only runs in combat).
local function combatTick(creature, guid)
    local st = solarianState[guid]
    if not st then
        return
    end

    -- C++ AppearDelay block (boss_astromancer.cpp:216-231) —
    -- runs before the phase branches every tick while latched:
    -- StopMoving + AttackStop, 2s timer; on expiry the Phase
    -- == 2 concealment lands (SetFlag NOT_SELECTABLE +
    -- SetVisible(false)), timer re-armed 2s.
    if st.appearDelay then
        creature:StopMoving()
        creature:AttackStop()
        if st.appearDelayTimer <= 1000 then
            st.appearDelay = false
            if st.phase == 2 then
                creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
                creature:SetVisible(false)
            end
            st.appearDelayTimer = 2000
        else
            st.appearDelayTimer = st.appearDelayTimer - 1000
        end
    end

    if st.phase == 1 then
        -- 1. Blinding light arm — 41s init then 45s: latch
        -- BlindingLight (C++-exact).
        if st.blindingLightTimer <= 1000 then
            st.blindingLight = true
            st.blindingLightTimer = 45000
        else
            st.blindingLightTimer = st.blindingLightTimer - 1000
        end
        -- 2. Wrath arm — 20000+rand()%5000: TRIGGERED cast on
        -- a random threat-list player within 100 yd, first
        -- MaxThreat entry skipped before filters
        -- (C++ SelectTarget(Random, 1, 100, true),
        -- boss_astromancer.cpp:236-241); re-arm regardless of
        -- target pick (C++-exact).
        if st.wrathTimer <= 1000 then
            creature:InterruptNonMeleeSpells(false)
            local target = randomThreatTarget(creature, true, 100)
            if target then
                creature:CastSpell(target, SPELL_WRATH_OF_THE_ASTROMANCER, true)
            end
            st.wrathTimer = 20000 + math.random(0, 4999)
        else
            st.wrathTimer = st.wrathTimer - 1000
        end
        -- 3. Arcane missiles arm — 2000 init then 3000:
        -- BlindingLight latched -> non-triggered Blinding Light
        -- on the victim; else non-triggered Arcane Missiles on
        -- a random threat-list unit, falling back to the victim when the pick
        -- is outside the 2.5-rad frontal arc (C++ HasInArc(2.5, target),
        -- boss_astromancer.cpp:257-258, via the inArc twin); re-arm
        -- regardless (C++-exact).
        if st.arcaneMissilesTimer <= 1000 then
            if st.blindingLight then
                creature:CastSpell(nil, SPELL_BLINDING_LIGHT)
                st.blindingLight = false
            else
                local target = randomThreatUnit(creature)
                -- C++ !me->HasInArc(2.5f, target) (boss_astromancer.cpp:257):
                -- IsInBack(t, arc) = !HasInArc(2*pi - arc), so
                -- IsInBack(target, 2*pi - 2.5) == !HasInArc(2.5).
                if target ~= nil and creature:IsInBack(target, 2 * math.pi - 2.5) then
                    target = creature:GetVictim()
                end
                if target then
                    creature:CastSpell(target, SPELL_ARCANE_MISSILES)
                end
            end
            st.arcaneMissilesTimer = 3000
        else
            st.arcaneMissilesTimer = st.arcaneMissilesTimer - 1000
        end
        -- 4. Wrath-of-the-astromancer arm — 15000 init:
        -- C++ SelectTarget(Random, 1) from the threat list
        -- (boss_astromancer.cpp:268-285): offset 1 pops the
        -- first MaxThreat entry before filters,
        -- playerOnly=false so non-players are
        -- eligible; player pick -> non-triggered cast + 25000,
        -- non-player pick -> 1000 re-arm, no pick -> timer
        -- stays expired for a next-tick retry (C++-exact).
        if st.wrathOfAstromancerTimer <= 1000 then
            creature:InterruptNonMeleeSpells(false)
            local target = randomThreatTarget(creature, false, nil)
            if target then
                if target:GetObjectType() == "Player" then
                    creature:CastSpell(target, SPELL_WRATH_OF_THE_ASTROMANCER)
                    st.wrathOfAstromancerTimer = 25000
                else
                    st.wrathOfAstromancerTimer = 1000
                end
            end
            -- No target: the C++ leaves the timer expired so
            -- the arm retries on the next tick (C++-exact).
        else
            st.wrathOfAstromancerTimer = st.wrathOfAstromancerTimer - 1000
        end
        -- 5. Portal transition arm (Phase1_Timer 50000,
        -- boss_astromancer.cpp:288-330): Phase = 2, re-arm
        -- 50000; motion-master Clear (StopMoving twin) +
        -- UpdatePosition teleport to the room center (alar
        -- precedent: creature:Teleport + StopMoving); portal 0
        -- on the small radius, portals 1-2 on the large radius
        -- at PORTAL_Z; the 7-yard separation fix for portals
        -- 1-2; 3 spotlight summons (TEMPSUMMON_TIMED_DESPAWN,
        -- Phase2 + Phase3 + AppearDelay + 1700ms = 28700ms),
        -- each flagged NOT_SELECTABLE and self-casting
        -- SPELL_SPOTLIGHT 25824; AppearDelay latched. The
        -- phase-2 branch does not run this tick (C++ else-if
        -- chain, boss_astromancer.cpp:331).
        if st.phase1Timer <= 1000 then
            st.phase = 2
            st.phase1Timer = 50000
            creature:StopMoving()
            creature:Teleport(CENTER_X, CENTER_Y, CENTER_Z, CENTER_O)
            st.portals = {}
            for i = 1, 3 do
                local radius = SMALL_PORTAL_RADIUS
                local z = CENTER_Z
                if i > 1 then
                    radius = LARGE_PORTAL_RADIUS
                    z = PORTAL_Z
                end
                local x = portalX(radius)
                st.portals[i] = { x, portalY(x, radius), z }
            end
            if math.abs(st.portals[3][1] - st.portals[2][1]) < 7
                    and math.abs(st.portals[3][2] - st.portals[2][2]) < 7 then
                local i = 1
                if math.abs(CENTER_X + 26.0 - st.portals[3][1]) < 7 then
                    i = -1
                end
                st.portals[3][1] = st.portals[3][1] + 7 * i
                st.portals[3][2] = portalY(st.portals[3][1], LARGE_PORTAL_RADIUS)
            end
            for i = 1, 3 do
                local spotlight = creature:SummonCreature(NPC_SPOTLIGHT,
                    st.portals[i][1], st.portals[i][2], st.portals[i][3],
                    CENTER_O, SUMMON_TIMED_DESPAWN, 28700)
                if spotlight then
                    spotlight:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
                    spotlight:CastSpell(spotlight, SPELL_SPOTLIGHT)
                end
            end
            st.appearDelay = true
        else
            st.phase1Timer = st.phase1Timer - 1000
        end
    elseif st.phase == 2 then
        -- Phase 2 arm (boss_astromancer.cpp:331-346): 10s
        -- after Solarian disappears, 12 agents spawn out of
        -- the three portals (4 each); Talk(SAY_SUMMON1);
        -- Phase = 3, timer re-armed 10s. AttackStop +
        -- StopMoving every tick (C++-exact).
        creature:AttackStop()
        creature:StopMoving()
        if st.phase2Timer <= 1000 then
            st.phase = 3
            for i = 1, 3 do
                for _ = 1, 4 do
                    summonMinion(creature, NPC_SOLARIUM_AGENT,
                        st.portals[i][1], st.portals[i][2], st.portals[i][3])
                end
            end
            creature:Talk(SAY_SUMMON1)
            st.phase2Timer = 10000
        else
            st.phase2Timer = st.phase2Timer - 1000
        end
    elseif st.phase == 3 then
        -- Phase 3 arm (boss_astromancer.cpp:347-371): 15s
        -- later Solarian reappears out of one of the 3 portals
        -- (random) while 2 priests appear in the other two;
        -- RemoveFlag(NOT_SELECTABLE) + SetVisible(true);
        -- Talk(SAY_SUMMON2); AppearDelay latched (the reveal
        -- concealment runs through the AppearDelay block, which
        -- no-ops here since Phase is now 1); Phase = 1, timer
        -- re-armed 15s. AttackStop + StopMoving every tick
        -- (C++-exact).
        creature:AttackStop()
        creature:StopMoving()
        if st.phase3Timer <= 1000 then
            st.phase = 1
            local i = math.random(1, 3)
            creature:StopMoving()
            creature:Teleport(st.portals[i][1], st.portals[i][2],
                st.portals[i][3], CENTER_O)
            for j = 1, 3 do
                if j ~= i then
                    summonMinion(creature, NPC_SOLARIUM_PRIEST,
                        st.portals[j][1], st.portals[j][2], st.portals[j][3])
                end
            end
            creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
            creature:SetVisible(true)
            creature:Talk(SAY_SUMMON2)
            st.appearDelay = true
            st.phase3Timer = 15000
        else
            st.phase3Timer = st.phase3Timer - 1000
        end
    elseif st.phase == 4 then
        -- Fear arm — 20s: non-triggered self cast
        -- (C++-exact), re-arm 20000 (C++-exact).
        if st.fearTimer <= 1000 then
            creature:CastSpell(creature, SPELL_FEAR)
            st.fearTimer = 20000
        else
            st.fearTimer = st.fearTimer - 1000
        end
        -- Void bolt arm — 10s: non-triggered DoCastVictim
        -- (C++-exact), re-arm 10000 (C++-exact).
        if st.voidBoltTimer <= 1000 then
            creature:CastSpell(nil, SPELL_VOID_BOLT)
            st.voidBoltTimer = 10000
        else
            st.voidBoltTimer = st.voidBoltTimer - 1000
        end
    end

    -- Phase 4 latch (C++ arm order — after the phase
    -- branches): HealthBelowPct(20) while not already phase
    -- 4 — the RemoveFlag(NOT_SELECTABLE) + SetVisible(true)
    -- guarantee arms port below (2026-10-23 — the stale "no
    -- visibility bridge / paired with the reveal" claims are
    -- retracted), then Talk(SAY_VOIDA) + Talk(SAY_VOIDB)
    -- (C++-exact); the SetArmor arm is ported below via
    -- creature:SetArmor (armor bridge); the display arm (MODEL_VOIDWALKER)
    -- is ported below; the scale arm is ported below via
    -- creature:SetScale.
    if st.phase ~= 4 then
        local maxHealth = creature:GetMaxHealth()
        if maxHealth > 0
                and creature:GetHealth() < math.floor(maxHealth * 20 / 100) then
            st.phase = 4
            -- C++ arm order (boss_astromancer.cpp:400-401): the
            -- "make sure she wont be invisible or not selectable"
            -- guarantee — RemoveFlag(NOT_SELECTABLE) + SetVisible(true)
            -- before the talks; non-vacuous now that the phase 2/3
            -- machine conceals her (the latch can fire mid-phase-2/3).
            creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
            creature:SetVisible(true)
            creature:Talk(SAY_VOIDA)
            creature:Talk(SAY_VOIDB)
            -- C++ arm order (boss_astromancer.cpp:404-406): SetArmor
            -- before SetDisplayId/SetObjectScale.
            creature:SetArmor(WV_ARMOR)
            -- C++ arm order: SetDisplayId(MODEL_VOIDWALKER) before
            -- SetObjectScale(defaultsize*2.5f).
            creature:SetDisplayId(MODEL_VOIDWALKER)
            -- C++: me->SetObjectScale(defaultsize*2.5f) — GetScale()
            -- reads the template base (the C++ defaultsize capture)
            -- while no script scale is known.
            creature:SetScale(creature:GetScale() * 2.5)
        end
    end
end

-- C++ ctor / Initialize() values land on OnEnterCombat:
-- arcane missiles 2000 / blinding light 41000 / wrath 20000+
-- rand()%5000 / wrath-of-the-astromancer 15000 / fear 20000 /
-- void bolt 10000 / phase = 1 (the BossAI ctor DATA_HIGH_
-- ASTROMANCER_SOLARIAN = 2 arm is instance-blocked,
-- unbridgeable; the ctor defaultarmor arm is captured below via
-- creature:GetArmor (armor bridge) and the defaultsize capture is
-- subsumed by GetScale's template-base read now that the scale
-- bridge exists).
RegisterCreatureEvent(ENTRY_SOLARIAN, 1, function(_, creature)
    local guid = creature:GetGUIDLow()
    cancelPump(guid)
    if solarianDefaultArmor[guid] == nil then
        solarianDefaultArmor[guid] = creature:GetArmor()
    end
    creature:Talk(SAY_AGGRO)
    solarianState[guid] = {
        phase = 1,
        arcaneMissilesTimer = 2000,
        blindingLightTimer = 41000,
        wrathTimer = 20000 + math.random(0, 4999),
        wrathOfAstromancerTimer = 15000,
        fearTimer = 20000,
        voidBoltTimer = 10000,
        blindingLight = false,
        portals = {},
        phase1Timer = 50000,
        phase2Timer = 10000,
        phase3Timer = 15000,
        appearDelayTimer = 2000,
        appearDelay = false,
    }
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542; sapphiron precedent)
    solarianPump[guid] = CreateLuaEvent(function()
        combatTick(creature, guid)
    end, 1000, 0)
end)

-- C++ Reset() (called on evade): fullReset — see above.
RegisterCreatureEvent(ENTRY_SOLARIAN, 2, function(_, creature)
    fullReset(creature:GetGUIDLow())
end)

-- C++ KilledUnit: Talk(SAY_KILL) unconditionally (no player
-- gate, C++-exact).
RegisterCreatureEvent(ENTRY_SOLARIAN, 3, function(_, creature)
    creature:Talk(SAY_KILL)
end)

-- C++ JustDied (boss_astromancer.cpp:152-157): SetObjectScale
-- (defaultsize) + SetDisplayId(MODEL_HUMAN) + Talk(SAY_DEATH) +
-- _JustDied() -> SetBossState(DATA_HIGH_ASTROMANCER_SOLARIAN,
-- DONE). The scale restore ports as a scale clear and the
-- SetDisplayId restore as SetDisplayId(MODEL_HUMAN) (the stale
-- "paired with the unbridged reveal legs" rationale retracted —
-- the reveal is non-vacuous now that the phase 2/3 machine
-- conceals her); the _JustDied arm IS ported below (stale
-- "instance-blocked" retracted 2026-11-07, creature:SetBossState
-- is bridged).
RegisterCreatureEvent(ENTRY_SOLARIAN, 4, function(_, creature)
    creature:SetScale(0)
    creature:SetDisplayId(MODEL_HUMAN)
    creature:Talk(SAY_DEATH)
    creature:SetBossState(DATA_HIGH_ASTROMANCER_SOLARIAN, 3) -- DONE
    fullReset(creature:GetGUIDLow())
end)

-- Eluna's On_Reset fires ahead of OnDied and OnSpawn, so
-- this lands the same Reset() reset as the evade hook; the C++
-- Reset SetObjectScale(defaultsize) restore arm ports as a scale
-- clear and the SetDisplayId(MODEL_HUMAN) restore as
-- SetDisplayId(MODEL_HUMAN); the RemoveFlag(NOT_SELECTABLE) +
-- SetVisible(true) guarantee arms are ported too (2026-10-23 —
-- the stale "no visibility bridge" claim is retracted); the
-- SetArmor(defaultarmor) arm IS ported below (armor bridge).
RegisterCreatureEvent(ENTRY_SOLARIAN, 23, function(_, creature)
    -- C++ Reset() arm order (boss_astromancer.cpp:151-155):
    -- SetArmor(defaultarmor) first, then RemoveFlag + SetVisible.
    local defaultArmor = solarianDefaultArmor[creature:GetGUIDLow()]
    if defaultArmor then
        creature:SetArmor(defaultArmor)
    end
    -- RemoveFlag(NOT_SELECTABLE) + SetVisible(true) are
    -- C++-exact — non-vacuous now that the phase 2/3 machine
    -- conceals her (an evade mid-phase-2 leaves her flagged
    -- and hidden until Reset); then the scale clear and
    -- SetDisplayId restore.
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    creature:SetVisible(true)
    creature:SetScale(0)
    creature:SetDisplayId(MODEL_HUMAN)
    fullReset(creature:GetGUIDLow())
end)

-- ================= Wrath of the Astromancer aura (42783) =================

local AURA_REMOVE_BY_EXPIRE = 4 -- SpellAuraDefines.h:67

-- C++ spell_astromancer_wrath_of_the_astromancer::AfterRemove
-- (boss_astromancer.cpp:522-530): EFFECT_0 SPELL_AURA_PERIODIC_
-- TRIGGER_SPELL. On expiry only, the target self-casts
-- Effects[EFFECT_1].CalcValue(), non-triggered. The DBC-only
-- spell id is guarded: with no Spell.dbc loaded the read is 0
-- and the handler no-ops instead of casting spell 0
-- (GetSpellEffectCalcValue convention, spell_generic.lua).
local function wrathOfTheAstromancerRemove(event, aura, target)
    if aura:GetRemoveMode() ~= AURA_REMOVE_BY_EXPIRE then
        return
    end
    if target == nil then
        return
    end
    local finalSpell = aura:GetSpellEffectCalcValue(SPELL_WRATH_OF_THE_ASTROMANCER, 1)
    if finalSpell ~= 0 then
        target:CastSpell(target, finalSpell)
    end
end

RegisterAuraEvent(SPELL_WRATH_OF_THE_ASTROMANCER, 3, wrathOfTheAstromancerRemove)
