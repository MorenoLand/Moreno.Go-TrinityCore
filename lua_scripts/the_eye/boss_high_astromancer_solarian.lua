-- High Astromancer Solarian (The Eye, Tempest Keep) --
-- Lua port of src/server/scripts/Outland/TempestKeep/Eye/
-- boss_astromancer.cpp (boss_high_astromancer_solarian — the
-- only boss CreatureScript AI class AddSC_boss_high_
-- astromancer_solarian registers; npc_solarium_priest (18806)
-- documented only — its heal arm sits behind the cross-
-- creature/instance bridge (instance->GetGuidData(DATA_
-- ASTROMANCER), warp_splinter-treant / skyriss-illusion
-- precedent) and it is only spawned by the unmodeled portal
-- machine (unreachable); the spell_astromancer_wrath_of_the_
-- astromancer AuraScript has no AuraScript bridge — AuraScript
-- check handlers are not modeled, standing blocker,
-- documented only).
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
-- C++-exact); SAY_SUMMON1 = 1 / SAY_SUMMON2 = 2 never fire
-- in this model — they sit in the unmodeled Phase 2/3 portal
-- arms (millhouse SAY_ICEBLOCK precedent).
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
--    5000: InterruptNonMeleeSpells(false) ported (no-op twin);
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
--    MeleeSpells unmodeled (no live cast-state read bridge (IsCasting is constant-false — Go casts are fire-and-forget)); target =
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
--    Phase 2/3 portal machine — the UpdatePosition teleports,
--    the NPC_ASTROMANCER_SOLARIAN_SPOTLIGHT 18928 /
--    NPC_SOLARIUM_AGENT 18925 / NPC_SOLARIUM_PRIEST 18806
--    summons, the SetVisible/SetFlag(NOT_SELECTABLE) arms and
--    the AppearDelay/StopMoving/AttackStop machine — unmodeled
--    (steamrigger / omor precedents); the summon/StopMoving/AttackStop/
--    visibility/flag bridges are all live now (2026-10-23), but the
--    machine still needs the motion-master Clear, cross-creature
--    AttackStart for the 12 agents + 2 priests (no bridge — they'd
--    spawn inert), and the npc_solarium_priest/agent minion AIs; Phase
--    stays 1 until the 20% latch. SAY_SUMMON1/SAY_SUMMON2 never
--    fire in this model.
-- Phase 4 latch (C++ arm order — after the phase branches,
-- every tick): if Phase ~= 4 and HealthBelowPct(20): Phase =
-- 4 + Talk(SAY_VOIDA) + Talk(SAY_VOIDB) (C++-exact); the
-- RemoveFlag(NOT_SELECTABLE) + SetVisible(true) guarantee arms ARE
-- ported (2026-10-23 — the stale "paired with the reveal" rationale is
-- retracted: the Lua model never goes invisible, so the reveal is
-- vacuous, exactly like the display/scale arms); the
-- SetArmor(31000) arm IS ported below via creature:SetArmor
-- (armor bridge, lua_creature_events.go); the
-- SetDisplayId(MODEL_VOIDWALKER 18988) arm IS ported (the stale
-- "paired with the reveal" rationale retracted 2026-10-21 — the Lua
-- model never goes invisible, so the phase-4 reveal is vacuous,
-- exactly like the already-ported scale arm);
-- the SetObjectScale(x2.5) arm is ported below via
-- creature:SetScale (scale bridge now exists).
-- Phase 4 arms (C++ arm order): fear — 20s: non-triggered
-- DoCast(me, SPELL_FEAR 34322) (C++-exact), re-arm 20000
-- (C++-exact); void bolt — 10s: non-triggered DoCastVictim
-- (SPELL_VOID_BOLT 39329) (C++-exact), re-arm 10000
-- (C++-exact). Melee is engine-driven.
-- OnTargetDied(3): Talk(SAY_KILL) unconditionally (no player
-- gate, C++-exact). OnDied(4): Talk(SAY_DEATH) + cleanup
-- (the _JustDied arm is instance-blocked); the SetObjectScale
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
-- SOLARIAN = 2 bookkeeping in Reset/JustEngagedWith/JustDied
-- skipped); 'no summon bridge' is stale (the bridge is live) —
-- per-arm: agent (18925)/priest (18806) stay out on the paired
-- Summoned->AI()->AttackStart(random target) legs (:184,
-- unbridged cross-creature relay; their role is combat);
-- spotlight (18928) stays out on the paired summon-side
-- self-cast SPELL_SPOTLIGHT 25824 (:322); no
-- movement bridge — the UpdatePosition teleports are teleport-
-- bridgeable and StopMoving/AttackStop bridges are live, but the
-- AppearDelay machine stays unmodeled (omor precedent) with the
-- unbridged motion-master Clear and the unported phase 2/3 machine;
-- the SetArmor arms ARE ported (phase-4 set + Reset restore,
-- armor bridge, lua_creature_events.go); the phase 2/3 SetVisible/SetFlag(NOT_SELECTABLE)
-- machine legs stay out with the machine (their bridges are live —
-- see the phase-2/3 note above); the phase-4/reset
-- RemoveFlag(NOT_SELECTABLE) + SetVisible(true) guarantee arms ARE
-- ported (2026-10-23); the SetDisplayId arms are ported,
-- SetObjectScale ported via the scale bridge; the HasInArc
-- victim fallback IS ported via the IsInBack twin; no cast-
-- state bridge — the remaining InterruptNonMeleeSpells arms
-- unmodeled, casts fire unconditionally (netherspite
-- precedent); the wrath arms now pick from the threat list
-- (GetAITargets), including the non-player 1s re-arm;
-- npc_solarium_priest
-- (18806) documented only, not registered; no AuraScript
-- bridge — spell_astromancer_wrath_of_the_astromancer
-- documented only (standing blocker).

local SPELL_ARCANE_MISSILES = 33031
local SPELL_WRATH_OF_THE_ASTROMANCER = 42783
local SPELL_BLINDING_LIGHT = 33009
local SPELL_FEAR = 34322
local SPELL_VOID_BOLT = 39329

local ENTRY_SOLARIAN = 18805

local MODEL_HUMAN = 18239 -- boss_astromancer.cpp:72
local MODEL_VOIDWALKER = 18988 -- boss_astromancer.cpp:73
local WV_ARMOR = 31000 -- boss_astromancer.cpp:78
local UNIT_FIELD_FLAGS = 59 -- UpdateFields.h:117 (OBJECT_END+0x35)
local UNIT_FLAG_NOT_SELECTABLE = 33554432 -- 0x02000000, UnitDefines.h:149

local SAY_AGGRO = 0
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

-- C++ SelectTarget(Random, 1, 100, true) / SelectTarget(Random,
-- 1): the first MaxThreat entry is skipped BEFORE the filters
-- (UnitAI.h:163-174, offset-before-filter — not a victim-GUID
-- exclusion, which differs when the victim is not the threat
-- head); playerOnly and maxDist filter like
-- DefaultTargetSelector; dead members are offline in C++
-- (skipped at list build) so they are filtered here.
-- playerOnly=false leaves pets/totems eligible.
local function randomThreatTarget(creature, playerOnly, maxDist)
    local targets = creature:GetAITargets()
    local pool = {}
    for i = 2, #targets do
        local t = targets[i]
        if not t:IsDead()
                and (not playerOnly or t:GetObjectType() == "Player")
                and (not maxDist or creature:GetDistance(t) <= maxDist) then
            pool[#pool + 1] = t
        end
    end
    if #pool == 0 then
        return nil
    end
    return pool[math.random(#pool)]
end

-- C++ UpdateAI in arm order — 1s granularity exact for all
-- C++ timers; the !UpdateVictim early return collapses into
-- the pump (it only runs in combat). Phase 2/3 arms are
-- unreachable behind the portal bridges (see header).
local function combatTick(creature, guid)
    local st = solarianState[guid]
    if not st then
        return
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
        -- 5. Portal transition arm (Phase1_Timer 50000):
        -- summon / teleport / visibility driven — unmodeled,
        -- see header. Phase stays 1 until the 20% latch.
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
            -- before the talks; vacuous in this model (phases 2/3 never
            -- conceal her) but C++-exact; the stale "paired with the
            -- reveal" rationale is retracted 2026-10-23, same as the
            -- display/scale arms.
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
    local guid = creature:GetGUID()
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
    }
    solarianPump[guid] = CreateLuaEvent(function()
        combatTick(creature, guid)
    end, 1000, 0)
end)

-- C++ Reset() (called on evade): fullReset — see above.
RegisterCreatureEvent(ENTRY_SOLARIAN, 2, function(_, creature)
    fullReset(creature:GetGUID())
end)

-- C++ KilledUnit: Talk(SAY_KILL) unconditionally (no player
-- gate, C++-exact).
RegisterCreatureEvent(ENTRY_SOLARIAN, 3, function(_, creature)
    creature:Talk(SAY_KILL)
end)

-- C++ JustDied: Talk(SAY_DEATH) + cleanup (the _JustDied arm
-- is instance-blocked); the SetObjectScale(defaultsize) restore
-- arm is ported as a scale clear and the SetDisplayId(MODEL_HUMAN)
-- restore as SetDisplayId(MODEL_HUMAN) (the stale "paired with the
-- unbridged reveal legs" rationale retracted — the Lua model never
-- goes invisible, so the reveal is vacuous).
RegisterCreatureEvent(ENTRY_SOLARIAN, 4, function(_, creature)
    creature:SetScale(0)
    creature:SetDisplayId(MODEL_HUMAN)
    creature:Talk(SAY_DEATH)
    fullReset(creature:GetGUID())
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
    local defaultArmor = solarianDefaultArmor[creature:GetGUID()]
    if defaultArmor then
        creature:SetArmor(defaultArmor)
    end
    -- RemoveFlag(NOT_SELECTABLE) + SetVisible(true) are
    -- vacuous here (flag never set, never hidden) but C++-exact — the
    -- stale "no visibility bridge / paired with the reveal" rationale
    -- retracted 2026-10-23; then the scale clear and SetDisplayId restore.
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    creature:SetVisible(true)
    creature:SetScale(0)
    creature:SetDisplayId(MODEL_HUMAN)
    fullReset(creature:GetGUID())
end)
