-- Omor the Unscarred (Hellfire Ramparts, Hellfire Citadel) --
-- Lua port of src/server/scripts/Outland/HellfireCitadel/
-- HellfireRamparts/boss_omor_the_unscarred.cpp (boss_omor_the_
-- unscarred — the only CreatureScript AI class AddSC_boss_omor_
-- the_unscarred registers; no SpellScript/AuraScript scripts in
-- this file).
-- Second boss in the Hellfire Ramparts set per outland_script_
-- loader.cpp order (instance_hellfire_ramparts.cpp stays blocked
-- on the instance-script model).
-- Entry: hellfire_ramparts.h carries no NPC_ entry for Omor (it
-- lists only the sentry/vazruden/nazan/liquid-fire ids) and
-- instance_hellfire_ramparts.cpp has no OnCreatureCreate mapping
-- for the boss, so the entry is verified externally: wowhead
-- npc=17308/omor-the-unscarred (also in the wowhead Hellfire
-- Ramparts zone listing). The creature_template ScriptName
-- bindings are DB-side (no TDB in this workspace).
-- Talk: SAY_AGGRO = 0 (pull — fired from the C++ JustEngagedWith,
-- C++-exact), SAY_CURSE = 2 (treacherous aura — fired from the
-- Aura_Timer arm, C++-exact), SAY_KILL_1 = 3 (kill — the C++
-- gates with rand32()%2, C++-exact 50%, kelidan precedent),
-- SAY_DIE = 4 (death — fired from the C++ JustDied, C++-exact),
-- SAY_WIPE = 5 (wipe — fired from the C++ Reset, C++-exact);
-- SAY_SUMMON = 1 fires only from the JustSummoned arm — no
-- summon bridge, the fiendish hound never materializes, so it
-- is unreachable in this model.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via
-- CreateLuaEvent (per-GUID scheduler pump); melee is
-- engine-driven in Go (creature combat tick), like C++ DoMelee
-- AttackIfReady. C++ DoCast default is triggered=false
-- (vaelastrasz convention).
-- Fight shape (C++-exact for all modeled arms): Omor (17308):
-- OnEnterCombat(1): per-GUID reset to the C++ Initialize()
-- values + Talk(SAY_AGGRO) + 1s scheduler pump (a port of
-- UpdateAI in C++ arm order — 1s granularity exact for all C++
-- timers; the !UpdateVictim early return collapses into the
-- pump; the BossAI::JustEngagedWith/JustDied/_Reset arms are
-- instance-blocked, DATA_OMOR_THE_UNSCARRED = 1 is an instance-
-- side constant, unbridgeable).
-- Pump: summon 30707 — 10s init then {15s,30s}, gated on
-- summonedCount < 2 — non-triggered DoCastSelf (C++-exact),
-- re-arm {15s,30s} regardless (C++-exact 15000+rand32()%15000);
-- the InterruptNonMeleeSpells arm has no bridge; the
-- JustSummoned arm (Talk(SAY_SUMMON), the AttackStart relay,
-- ++SummonedCount) is unreachable — the hound never
-- materializes (steamrigger precedent), so summonedCount stays
-- 0 and the gate stays open; orbital strike 30637 — 25s init
-- then {14s,16s}, target = victim when within melee range else
-- a random alive player in the instance (the C++
-- IsWithinMeleeRange gate is approximated with GetDistance <=
-- 5, no IsWithinMeleeRange bridge, zuljin precedent; C++
-- SelectTarget(Random, 0), thespia precedent), nil pick casts
-- nothing, the TYPEID_PLAYER gate is C++-exact (kargath
-- GetObjectType convention — the timer is NOT re-armed when
-- the gate fails, so the arm retries on the next tick),
-- non-triggered DoCast on the target, re-arm {14s,16s} on fire
-- (C++ 14000+rand32()%2000), latch canPullBack and store the
-- player GUID; shadow whip 30638 — the whip arm's
-- HasUnitMovementFlag(MOVEMENTFLAG_FALLING_FAR) gate has no
-- bridge, so the cast never fires in this model; the GUID
-- clear + 2000 re-arm + canPullBack=false bookkeeping is
-- C++-exact (the ScriptData "temporary solution for orbital/
-- shadow whip-ability. Needs more core support before making
-- it more proper" caveat is upstream, documented, not
-- bridged); demonic shield 31901 — while health is strictly
-- below 20% (C++ HealthBelowPct(20), halazzi strict-fraction
-- precedent), 1s init then 15s — non-triggered DoCastSelf
-- (C++-exact), re-arm 15000 (C++-exact; the timer only ticks
-- while the health gate holds); treacherous aura 30695 (heroic
-- 37566 — no difficulty bridge, thespia precedent) — 10s init
-- then {8s,16s}, Talk(SAY_CURSE), target = random alive player
-- in the instance, nil pick casts nothing, non-triggered
-- DoCast on the target (C++-exact), re-arm {8s,16s} only when
-- a target was picked (C++-exact); shadow bolt 30686 (heroic
-- 39297 — no difficulty bridge) — 2s init then {4s,6.5s}: the
-- C++ picks SelectTarget(Random, 0) for the null check then
-- overrides the target to the victim, so the cast lands on the
-- victim (C++-exact), non-triggered DoCastVictim (felmyst
-- convention), re-arm {4s,6.5s} only when a target was picked
-- (C++-exact). Melee is engine-driven.
-- OnTargetDied(3): 50% Talk(SAY_KILL_1) (C++ `if (rand32() %
-- 2) return;`, kelidan precedent, C++-exact).
-- OnDied(4): Talk(SAY_DIE) + cleanup (the _JustDied arm is
-- instance-blocked).
-- OnLeaveCombat(2): Talk(SAY_WIPE) + cancel the pump + drop
-- per-GUID state (the C++ Reset() wipe arm — OnLeaveCombat is
-- the evade hook where the C++ Reset() fires; the Initialize()
-- re-latch lands on OnEnterCombat, gargolmar precedent; the
-- _Reset arm is instance-blocked). OnReset(23): cleanup only —
-- Eluna's On_Reset also fires ahead of OnDied and OnSpawn, so
-- keying the wipe yell to it would put SAY_WIPE ahead of SAY_
-- DIE on every kill; the wipe yell stays on the evade hook.
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI shim
-- (instance_hellfire_ramparts.cpp stays blocked on the
-- instance-script model; the BossAI ctor DATA_OMOR_THE_
-- UNSCARRED = 1 bookkeeping in Reset/JustEngagedWith/JustDied
-- skipped); no summon bridge — the SPELL_SUMMON_FIENDISH_HOUND
-- cast never produces a hound (steamrigger precedent), so the
-- JustSummoned arm (Talk(SAY_SUMMON), the AttackStart relay,
-- ++SummonedCount) is unreachable and SAY_SUMMON is
-- unreachable; no movement-flag bridge — the orbital-strike/
-- shadow-whip pullback machine's MOVEMENTFLAG_FALLING_FAR gate
-- is unbridgeable, so the shadow whip never fires (the
-- ScriptData temporary-solution caveat is upstream,
-- documented, not bridged); the IsWithinMeleeRange gate on
-- orbital strike is approximated with GetDistance <= 5 (no
-- IsWithinMeleeRange bridge, zuljin precedent); no movement
-- bridge — the SetCombatMovement(false) arm unmodeled; no
-- difficulty bridge — H_SPELL_BANE_OF_TREACHERY 37566 and H_
-- SPELL_SHADOW_BOLT 39297 unmodeled (thespia precedent); no
-- SpellScript/AuraScript scripts in this file.

local SPELL_ORBITAL_STRIKE = 30637
local SPELL_SHADOW_WHIP = 30638
local SPELL_TREACHEROUS_AURA = 30695
local SPELL_DEMONIC_SHIELD = 31901
local SPELL_SHADOW_BOLT = 30686
local SPELL_SUMMON_FIENDISH_HOUND = 30707

local SAY_AGGRO = 0
local SAY_SUMMON = 1
local SAY_CURSE = 2
local SAY_KILL_1 = 3
local SAY_DIE = 4
local SAY_WIPE = 5

local ENTRY_OMOR = 17308

local MELEE_RANGE = 5

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
-- on OnEnterCombat — broggok precedent): orbitalStrike 25000
-- / shadowWhip 2000 / aura 10000 / demonicShield 1000 /
-- shadowbolt 2000 / summon 10000 / summonedCount 0 /
-- pullGuid cleared / canPullBack false.
local function freshState()
    return {
        orbitalStrike = 25000,
        shadowWhip = 2000,
        aura = 10000,
        demonicShield = 1000,
        shadowbolt = 2000,
        summon = 10000,
        summonedCount = 0,
        pullGuid = nil,
        canPullBack = false,
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

local function randomPlayer(creature)
    local players = playersInInstance(creature)
    if #players == 0 then
        return nil
    end
    return players[math.random(#players)]
end

local function healthBelowPct(creature, pct)
    local maxHealth = creature:GetMaxHealth()
    return maxHealth > 0
        and creature:GetHealth() * 100 / maxHealth < pct
end

local function bossTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end

    -- Summon 30707: 10s init then {15s,30s}, gated on
    -- summonedCount < 2 — non-triggered DoCastSelf (C++-exact),
    -- re-arm {15s,30s} regardless (C++-exact). The
    -- InterruptNonMeleeSpells arm has no bridge. The
    -- JustSummoned arm (Talk(SAY_SUMMON), AttackStart relay,
    -- ++SummonedCount) is unreachable — the hound never
    -- materializes (steamrigger precedent), so summonedCount
    -- stays 0 and the gate stays open.
    if st.summonedCount < 2 then
        if st.summon <= 1000 then
            creature:CastSpell(creature, SPELL_SUMMON_FIENDISH_HOUND)
            st.summon = 15000 + math.random(0, 14999)
        else
            st.summon = st.summon - 1000
        end
    end

    if st.canPullBack then
        -- Shadow whip 30638: 2s after the orbital strike, the
        -- C++ arm looks the struck player up by GUID and casts
        -- only when the player HasUnitMovementFlag(
        -- MOVEMENTFLAG_FALLING_FAR) — no movement-flag bridge,
        -- so the cast never fires in this model; the GUID clear
        -- + 2000 re-arm + canPullBack=false bookkeeping is
        -- C++-exact.
        if st.shadowWhip <= 1000 then
            st.pullGuid = nil
            st.shadowWhip = 2000
            st.canPullBack = false
        else
            st.shadowWhip = st.shadowWhip - 1000
        end
    else
        -- Orbital strike 30637: 25s init then {14s,16s} —
        -- target = victim when within melee range else a
        -- random alive player in the instance (GetDistance <=
        -- 5 stands in for the C++ IsWithinMeleeRange gate,
        -- zuljin precedent; C++ SelectTarget(Random, 0),
        -- thespia precedent). The TYPEID_PLAYER gate is C++-
        -- exact (kargath GetObjectType convention) — the timer
        -- is NOT re-armed when the gate fails, so the arm
        -- retries on the next tick.
        if st.orbitalStrike <= 1000 then
            local target
            local victim = creature:GetVictim()
            if victim and creature:GetDistance(victim) <= MELEE_RANGE then
                target = victim
            else
                target = randomPlayer(creature)
            end
            if target and target:GetObjectType() == "Player" then
                creature:CastSpell(target, SPELL_ORBITAL_STRIKE)
                st.orbitalStrike = 14000 + math.random(0, 1999)
                st.pullGuid = target:GetGUID()
                st.canPullBack = true
            end
        else
            st.orbitalStrike = st.orbitalStrike - 1000
        end
    end

    -- Demonic shield 31901: while health is strictly below
    -- 20% (C++-exact), 1s init then 15s — non-triggered
    -- DoCastSelf (C++-exact), re-arm 15000 (C++-exact; the
    -- timer only ticks while the health gate holds).
    if healthBelowPct(creature, 20) then
        if st.demonicShield <= 1000 then
            creature:CastSpell(creature, SPELL_DEMONIC_SHIELD)
            st.demonicShield = 15000
        else
            st.demonicShield = st.demonicShield - 1000
        end
    end

    -- Treacherous aura 30695 (heroic 37566 — no difficulty
    -- bridge, thespia precedent): 10s init then {8s,16s} —
    -- Talk(SAY_CURSE), target = random alive player in the
    -- instance, nil pick casts nothing, non-triggered DoCast
    -- on the target (C++-exact), re-arm {8s,16s} only when a
    -- target was picked (C++-exact).
    if st.aura <= 1000 then
        creature:Talk(SAY_CURSE)
        local target = randomPlayer(creature)
        if target then
            creature:CastSpell(target, SPELL_TREACHEROUS_AURA)
            st.aura = 8000 + math.random(0, 7999)
        end
    else
        st.aura = st.aura - 1000
    end

    -- Shadow bolt 30686 (heroic 39297 — no difficulty bridge):
    -- 2s init then {4s,6.5s} — the C++ picks SelectTarget
    -- (Random, 0) for the null check then overrides the target
    -- to the victim, so the cast lands on the victim (C++-
    -- exact), non-triggered DoCastVictim (felmyst convention),
    -- re-arm {4s,6.5s} only when a target was picked (C++-
    -- exact).
    if st.shadowbolt <= 1000 then
        if randomPlayer(creature) then
            creature:CastSpell(nil, SPELL_SHADOW_BOLT)
            st.shadowbolt = 4000 + math.random(0, 2499)
        end
    else
        st.shadowbolt = st.shadowbolt - 1000
    end
end

RegisterCreatureEvent(ENTRY_OMOR, 1, function(_, creature)
    local guid = creature:GetGUID()
    resetBoss(guid)
    states[guid] = freshState()
    creature:Talk(SAY_AGGRO)
    timers[guid] = CreateLuaEvent(function()
        bossTick(creature, guid)
    end, 1000, 0)
end)

-- The C++ Reset() wipe arm (Talk(SAY_WIPE)) is keyed to the
-- evade hook: Eluna's On_Reset (23) also fires ahead of
-- OnDied and OnSpawn, so keying the yell to it would put
-- SAY_WIPE ahead of SAY_DIE on every kill.
RegisterCreatureEvent(ENTRY_OMOR, 2, function(_, creature)
    creature:Talk(SAY_WIPE)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_OMOR, 3, function(_, creature)
    -- C++ `if (rand32() % 2) return;` — 50% Talk(SAY_KILL_1)
    -- (kelidan precedent, C++-exact).
    if math.random(0, 1) ~= 0 then
        return
    end
    creature:Talk(SAY_KILL_1)
end)

RegisterCreatureEvent(ENTRY_OMOR, 4, function(_, creature)
    creature:Talk(SAY_DIE)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_OMOR, 23, function(_, creature)
    resetBoss(creature:GetGUID())
end)
