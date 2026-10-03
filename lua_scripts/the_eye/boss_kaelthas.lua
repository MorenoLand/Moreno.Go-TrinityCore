-- Kael'thas Sunstrider and advisors (The Eye, Tempest Keep) --
-- Lua port of src/server/scripts/Outland/TempestKeep/Eye/
-- boss_kaelthas.cpp (9 scripts registered from
-- AddSC_boss_kaelthas(): boss_kaelthas, boss_thaladred_the_
-- darkener, boss_lord_sanguinar, boss_grand_astromancer_
-- capernian, boss_master_engineer_telonicus, npc_kael_
-- flamestrike, npc_phoenix_tk, npc_phoenix_egg_tk,
-- spell_kael_gravity_lapse).
-- Second boss in The Eye set per outland_script_loader.cpp
-- order (boss_alar, boss_kaelthas, boss_void_reaver, boss_
-- high_astromancer_solarian).
-- Entries (the_eye.h, verifiable from the C++ sources):
-- NPC_KAELTHAS = 19622 (DATA_KAELTHAS = 0), NPC_THALADRED =
-- 20064, NPC_SANGUINAR = 20060, NPC_CAPERNIAN = 20062,
-- NPC_TELONICUS = 20063 (instance OnCreatureCreate maps
-- these to the advisor GUIDs used by the instance script,
-- verified in instance_the_eye.cpp). The creature_template
-- ScriptName bindings are DB-side (no TDB in this workspace).
-- Talk lines used: Kael'thas — SAY_SLAY = 9 (kill — the C++
-- KilledUnit talks unconditionally, no player gate,
-- gargolmar precedent, C++-exact), SAY_DEATH = 13 (death —
-- fired from the C++ JustDied, C++-exact); advisors —
-- SAY_THALADRED_AGGRO = 0 / SAY_SANGUINAR_AGGRO = 0 /
-- SAY_CAPERNIAN_AGGRO = 0 / SAY_TELONICUS_AGGRO = 0 (pull —
-- fired from the C++ JustEngagedWith, C++-exact). All other
-- Kael'thas talks (SAY_INTRO 0 / SAY_INTRO_CAPERNIAN 1 /
-- SAY_INTRO_TELONICUS 2 / SAY_INTRO_THALADRED 3 /
-- SAY_INTRO_SANGUINAR 4 / SAY_PHASE2_WEAPON 5 /
-- SAY_PHASE3_ADVANCE 6 / SAY_PHASE4_INTRO2 7 /
-- SAY_PHASE5_NUTS 8 / SAY_MIND_CONTROL 10 /
-- SAY_GRAVITY_LAPSE 11 / SAY_SUMMON_PHOENIX 12 /
-- EMOTE_PYROBLAST 14) and advisor death talks (SAY_THALADRED_
-- DEATH 1 / SAY_SANGUINAR_DEATH 1 / SAY_CAPERNIAN_DEATH 1 /
-- SAY_TELONICUS_DEATH 1 — all gated on _hasRessurrected, set
-- only by the SPELL_RESSURECTION SpellHit from Kael'thas's
-- instance-driven phase machine) never fire in this model —
-- solarian SAY_SUMMON1/2 precedent, documented only.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset (Kael'thas does not
-- override DamageTaken in a bridgeable way — no event 9 on
-- him; the advisors' DamageTaken fake-death latch has no
-- display/stand/flag/health bridges — unmodeled, see below).
-- Timers via a 1s CreateLuaEvent pump (a port of UpdateAI in
-- C++ arm order); melee is engine-driven in Go (creature
-- combat tick), like C++ DoMeleeAttackIfReady. C++ DoCast
-- default is triggered=false (vaelastrasz convention):
-- creature:CastSpell(target, spell) = non-triggered DoCast,
-- creature:CastSpell(target, spell, true) = TRIGGERED DoCast,
-- creature:CastSpell(nil, spell) = DoCastVictim.
-- Fight shape (C++-exact for all modeled arms): the
-- advisorbase_ai UpdateAI (the UpdateAI in each advisor runs
-- advisorbase_ai::UpdateAI first) is unmodeled — its only
-- event (EVENT_DELAYED_RESSURECTION) rides the unmodeled
-- resurrection machine; the advisorbase_ai Reset() UNIT_
-- STAND_STATE / UNIT_FLAG_NON_ATTACKABLE arms and the kael-
-- evade arm (kael->AI()->EnterEvadeMode when DATA_KAELTHAS
-- is IN_PROGRESS) have no stand/flag/instance bridges;
-- MoveInLineOfSight / AttackStart _inFakeDeath /
-- NON_ATTACKABLE guards collapse into the pump (no flag
-- bridge); JustDied kael->AI()->DoAction(ACTION_ACTIVE_
-- ADVISOR) is a cross-creature/instance bridge — unmodeled.
-- Thaladred (20064): OnEnterCombat(1): Talk(SAY_THALADRED_
-- AGGRO) + 1s pump (the C++ JustEngagedWith AddThreat(who,
-- 5000000.0f) has no threat bridge — unmodeled). Pump (C++
-- arm order): silence — 20000 init then 20000:
-- DoCastVictim(SPELL_SILENCE 30225); rend — 4000 init then
-- 4000: DoCastVictim(SPELL_REND 36965); psychic blow — 10000
-- init then 20000+rand()%5000: DoCastVictim(SPELL_PSYCHIC_
-- BLOW 10689). The gaze arm (100 init then 8500: random
-- target + ResetThreatList + AddThreat(5000000) + Talk(EMOTE_
-- THALADRED_GAZE 2, target)) is unmodeled — no threat
-- bridge and no targeted-Talk bridge (keleseth precedent).
-- Sanguinar (20060): OnEnterCombat(1): Talk(SAY_SANGUINAR_
-- AGGRO) + 1s pump: bellowing roar — 20000 init then
-- 25000+rand()%10000: DoCastVictim(SPELL_BELLOWING_ROAR
-- 40636).
-- Capernian (20062): OnEnterCombat(1): Talk(SAY_CAPERNIAN_
-- AGGRO) + 1s pump (C++-exact arm order): fireball — 2000
-- init then 4000: DoCastVictim(SPELL_CAPERNIAN_FIREBALL
-- 36971); conflagration — 20000 init then 10000+rand()%5000:
-- random alive player in the instance (thespia precedent);
-- if the target is within 30yd (creature:GetDistance,
-- C++-exact) DoCast(target, SPELL_CONFLAGRATION 37018),
-- else DoCastVictim. The arcane explosion arm (5000 init
-- then 4000+rand()%2000, fired only when a threat-list
-- victim is in melee range) is unmodeled — no threat-list /
-- melee-range bridge (thorngrin flame-buffet precedent). The
-- AttackStart MoveChase(CAPERNIAN_DISTANCE 20.0f) arm has no
-- movement bridge — unmodeled. Capernian deals no melee
-- damage (no DoMeleeAttackIfReady in C++ — C++-exact).
-- Telonicus (20063): OnEnterCombat(1): Talk(SAY_TELONICUS_
-- AGGRO) + 1s pump (C++-exact arm order): bomb — 10000 init
-- then 25000: DoCastVictim(SPELL_BOMB 37036); remote toy —
-- 5000 init then 10000+rand()%5000: DoCast on a random alive
-- player in the instance (SPELL_REMOTE_TOY 37027, thespia
-- precedent).
-- npc_kael_flamestrike — no Talk arms (its AI only casts
-- SPELL_FLAME_STRIKE_VIS 36730 then SPELL_FLAME_STRIKE_DMG
-- 36731 5s later and kills itself); it is spawned only by
-- the unmodeled EVENT_FLAMESTRIKE DoCast machine (no summon
-- bridge — steamrigger precedent) — documented only, no
-- registration. npc_phoenix_tk (21362) — no Talk arms (its
-- AI self-buffs SPELL_BURN 36720, drains 4500-5500 health
-- per 2s cycle, and summons NPC_PHOENIX_EGG 21364 on
-- death); spawned only by the unmodeled SPELL_PHOENIX_
-- ANIMATION 36723 machine (no summon bridge) — documented
-- only, no registration. npc_phoenix_egg_tk (21364) — no
-- Talk arms (15s rebirth timer summoning NPC_PHOENIX 21362
-- — no summon bridge); spawned only by the phoenix's
-- JustDied (summon chain, unreachable) — documented only, no
-- registration. spell_kael_gravity_lapse (25 teleport spells
-- 35966-35990 + SPELL_GRAVITY_LAPSE_PERIODIC 34480 /
-- SPELL_GRAVITY_LAPSE_FLIGHT_AURA 39432) — SpellScript, no
-- SpellScript bridge (standing blocker) — documented only.
-- Kael'thas himself (19622): the whole phase machine is
-- unmodeled — DoAction chains (ACTION_START_ENCOUNTER /
-- ACTION_PREPARE_ADVISORS / ACTION_ACTIVE_ADVISOR /
-- ACTION_SCHEDULE_COMBAT_EVENTS) are cross-creature /
-- instance-driven (instance SetBossState(DATA_KAELTHAS),
-- GetGuidData advisor GUIDs, GO statue/window arms — no
-- instance/GUID/gameobject bridges); the 7-weapon summon
-- machine (SPELL_SUMMON_WEAPONS 36976 + SPELL_SUMMON_
-- WEAPONA-G 36958-36964 — no summon bridge); the EVENT_
-- RESUME path after the phase-5 transition MovePoints (no
-- movement bridge — omor/nightbane precedent); gravity
-- lapse / nether beam / pyroblast / mind control / phoenix
-- summon / flamestrike pump arms all ride the instance-
-- phased event machine; the 50% DamageTaken transition
-- latch (MovePoint POINT_START_TRANSITION) has no movement
-- bridge; BossAI ctor/_Reset()/JustReachedHome()/
-- JustSummoned() bookkeeping (DATA_KAELTHAS = 0) has no
-- instance-script bridge (instance_the_eye.cpp stays in the
-- no-instance-script-bridge queue). Melee engine-driven.

local SPELL_SILENCE = 30225
local SPELL_REND = 36965
local SPELL_PSYCHIC_BLOW = 10689
local SPELL_BELLOWING_ROAR = 40636
local SPELL_CAPERNIAN_FIREBALL = 36971
local SPELL_CONFLAGRATION = 37018
local SPELL_BOMB = 37036
local SPELL_REMOTE_TOY = 37027

local KAEL_SAY_SLAY = 9
local KAEL_SAY_DEATH = 13

local ENTRY_KAELTHAS = 19622
local ENTRY_THALADRED = 20064
local ENTRY_SANGUINAR = 20060
local ENTRY_CAPERNIAN = 20062
local ENTRY_TELONICUS = 20063

local combatTimers = {}
local combatStates = {}

local function cancelCombatPump(guid)
    local id = combatTimers[guid]
    if id then
        RemoveEventById(id)
        combatTimers[guid] = nil
    end
end

local function fullReset(guid)
    cancelCombatPump(guid)
    combatStates[guid] = nil
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

-- C++ SelectTarget(Random, 0): a random alive player in the
-- instance (thespia precedent), player-only.
local function randomPlayer(creature)
    local candidates = playersInInstance(creature)
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- Kael'thas (19622): the phase machine is unmodeled (see
-- header); the only bridgeable arms are the two Talk arms.
-- C++ KilledUnit: Talk(SAY_SLAY) unconditionally (no player
-- gate, C++-exact).
RegisterCreatureEvent(ENTRY_KAELTHAS, 3, function(_, creature)
    creature:Talk(KAEL_SAY_SLAY)
end)

-- C++ JustDied: Talk(SAY_DEATH) (the _JustDied arm is
-- instance-blocked).
RegisterCreatureEvent(ENTRY_KAELTHAS, 4, function(_, creature)
    creature:Talk(KAEL_SAY_DEATH)
end)

-- Thaladred the Darkener (20064): C++ UpdateAI in arm order
-- (gaze arm unmodeled — see header; the !UpdateVictim /
-- _inFakeDeath early return collapses into the pump) — 1s
-- granularity exact for all C++ timers.
local function thaladredTick(creature, guid)
    local st = combatStates[guid]
    if not st then
        return
    end

    -- 1. Silence arm: DoCastVictim(SPELL_SILENCE), 20000 init
    -- then 20000 re-arm (C++-exact).
    if st.silence <= 1000 then
        creature:CastSpell(nil, SPELL_SILENCE)
        st.silence = 20000
    else
        st.silence = st.silence - 1000
    end

    -- 2. Rend arm: DoCastVictim(SPELL_REND), 4000 init then
    -- 4000 re-arm (C++-exact).
    if st.rend <= 1000 then
        creature:CastSpell(nil, SPELL_REND)
        st.rend = 4000
    else
        st.rend = st.rend - 1000
    end

    -- 3. Psychic blow arm: DoCastVictim(SPELL_PSYCHIC_BLOW),
    -- 10000 init then 20000+rand()%5000 re-arm (C++-exact).
    if st.psychicBlow <= 1000 then
        creature:CastSpell(nil, SPELL_PSYCHIC_BLOW)
        st.psychicBlow = 20000 + math.random(0, 4999)
    else
        st.psychicBlow = st.psychicBlow - 1000
    end
end

-- C++ ctor / Initialize() values (the whole schedule lands
-- on OnEnterCombat): gaze 100 (unmodeled) / silence 20000 /
-- rend 4000 / psychic blow 10000. The C++ JustEngagedWith
-- talks SAY_THALADRED_AGGRO (the AddThreat(who, 5000000.0f)
-- arm has no threat bridge — unmodeled).
RegisterCreatureEvent(ENTRY_THALADRED, 1, function(_, creature)
    local guid = creature:GetGUID()
    cancelCombatPump(guid)
    combatStates[guid] = {
        silence = 20000,
        rend = 4000,
        psychicBlow = 10000,
    }
    creature:Talk(0)
    combatTimers[guid] = CreateLuaEvent(function()
        thaladredTick(creature, guid)
    end, 1000, 0)
end)

-- C++ Reset() (called on evade): fullReset — see above.
RegisterCreatureEvent(ENTRY_THALADRED, 2, function(_, creature)
    fullReset(creature:GetGUID())
end)

-- C++ JustDied: SAY_THALADRED_DEATH fires only after the
-- unmodeled resurrection (_hasRessurrected gate) — never
-- fires in this model (solarian precedent); cleanup only
-- (the kael->AI()->DoAction(ACTION_ACTIVE_ADVISOR) arm is
-- a cross-creature/instance bridge — unmodeled).
RegisterCreatureEvent(ENTRY_THALADRED, 4, function(_, creature)
    fullReset(creature:GetGUID())
end)

-- Eluna's On_Reset fires ahead of OnDied and OnSpawn, so
-- this lands the same Reset() reset as the evade hook.
RegisterCreatureEvent(ENTRY_THALADRED, 23, function(_, creature)
    fullReset(creature:GetGUID())
end)

-- Lord Sanguinar (20060): C++ UpdateAI arm — the
-- !UpdateVictim / _inFakeDeath early return collapses into
-- the pump — 1s granularity exact.
local function sanguinarTick(creature, guid)
    local st = combatStates[guid]
    if not st then
        return
    end

    -- Fear arm: DoCastVictim(SPELL_BELLOWING_ROAR), 20000
    -- init then 25000+rand()%10000 re-arm (C++-exact).
    if st.fear <= 1000 then
        creature:CastSpell(nil, SPELL_BELLOWING_ROAR)
        st.fear = 25000 + math.random(0, 9999)
    else
        st.fear = st.fear - 1000
    end
end

-- C++ ctor / Initialize() values: fear 20000. The C++
-- JustEngagedWith talks SAY_SANGUINAR_AGGRO.
RegisterCreatureEvent(ENTRY_SANGUINAR, 1, function(_, creature)
    local guid = creature:GetGUID()
    cancelCombatPump(guid)
    combatStates[guid] = {
        fear = 20000,
    }
    creature:Talk(0)
    combatTimers[guid] = CreateLuaEvent(function()
        sanguinarTick(creature, guid)
    end, 1000, 0)
end)

-- C++ Reset() (called on evade): fullReset — see above.
RegisterCreatureEvent(ENTRY_SANGUINAR, 2, function(_, creature)
    fullReset(creature:GetGUID())
end)

-- C++ JustDied: SAY_SANGUINAR_DEATH fires only after the
-- unmodeled resurrection (_hasRessurrected gate) — never
-- fires in this model (solarian precedent); cleanup only.
RegisterCreatureEvent(ENTRY_SANGUINAR, 4, function(_, creature)
    fullReset(creature:GetGUID())
end)

-- Eluna's On_Reset fires ahead of OnDied and OnSpawn, so
-- this lands the same Reset() reset as the evade hook.
RegisterCreatureEvent(ENTRY_SANGUINAR, 23, function(_, creature)
    fullReset(creature:GetGUID())
end)

-- Grand Astromancer Capernian (20062): C++ UpdateAI in arm
-- order (arcane explosion arm unmodeled — see header; the
-- !UpdateVictim / _inFakeDeath early return collapses into
-- the pump) — 1s granularity exact for all C++ timers.
local function capernianTick(creature, guid)
    local st = combatStates[guid]
    if not st then
        return
    end

    -- 1. Fireball arm: DoCastVictim(SPELL_CAPERNIAN_FIREBALL),
    -- 2000 init then 4000 re-arm (C++-exact).
    if st.fireball <= 1000 then
        creature:CastSpell(nil, SPELL_CAPERNIAN_FIREBALL)
        st.fireball = 4000
    else
        st.fireball = st.fireball - 1000
    end

    -- 2. Conflagration arm: 20000 init then 10000+rand()%5000
    -- re-arm (C++-exact); random alive player in the
    -- instance — within 30yd DoCast on the target, else
    -- DoCastVictim (C++ IsWithinDistInMap branch, C++-exact).
    if st.conflagration <= 1000 then
        local target = randomPlayer(creature)
        if target and creature:GetDistance(target) <= 30 then
            creature:CastSpell(target, SPELL_CONFLAGRATION)
        else
            creature:CastSpell(nil, SPELL_CONFLAGRATION)
        end
        st.conflagration = 10000 + math.random(0, 4999)
    else
        st.conflagration = st.conflagration - 1000
    end
end

-- C++ ctor / Initialize() values: fireball 2000 /
-- conflagration 20000 / arcane explosion 5000 (unmodeled) /
-- Yell 2000 (no yell arm — dead timer). The C++
-- JustEngagedWith talks SAY_CAPERNIAN_AGGRO (the AttackStart
-- MoveChase(CAPERNIAN_DISTANCE) arm has no movement bridge
-- — unmodeled). No melee in C++ (no DoMeleeAttackIfReady).
RegisterCreatureEvent(ENTRY_CAPERNIAN, 1, function(_, creature)
    local guid = creature:GetGUID()
    cancelCombatPump(guid)
    combatStates[guid] = {
        fireball = 2000,
        conflagration = 20000,
    }
    creature:Talk(0)
    combatTimers[guid] = CreateLuaEvent(function()
        capernianTick(creature, guid)
    end, 1000, 0)
end)

-- C++ Reset() (called on evade): fullReset — see above.
RegisterCreatureEvent(ENTRY_CAPERNIAN, 2, function(_, creature)
    fullReset(creature:GetGUID())
end)

-- C++ JustDied: SAY_CAPERNIAN_DEATH fires only after the
-- unmodeled resurrection (_hasRessurrected gate) — never
-- fires in this model (solarian precedent); cleanup only.
RegisterCreatureEvent(ENTRY_CAPERNIAN, 4, function(_, creature)
    fullReset(creature:GetGUID())
end)

-- Eluna's On_Reset fires ahead of OnDied and OnSpawn, so
-- this lands the same Reset() reset as the evade hook.
RegisterCreatureEvent(ENTRY_CAPERNIAN, 23, function(_, creature)
    fullReset(creature:GetGUID())
end)

-- Master Engineer Telonicus (20063): C++ UpdateAI in arm
-- order (the !UpdateVictim / _inFakeDeath early return
-- collapses into the pump) — 1s granularity exact.
local function telonicusTick(creature, guid)
    local st = combatStates[guid]
    if not st then
        return
    end

    -- 1. Bomb arm: DoCastVictim(SPELL_BOMB), 10000 init then
    -- 25000 re-arm (C++-exact).
    if st.bomb <= 1000 then
        creature:CastSpell(nil, SPELL_BOMB)
        st.bomb = 25000
    else
        st.bomb = st.bomb - 1000
    end

    -- 2. Remote toy arm: 5000 init then 10000+rand()%5000
    -- re-arm (C++-exact); DoCast on a random alive player in
    -- the instance (thespia precedent).
    if st.remoteToy <= 1000 then
        local target = randomPlayer(creature)
        if target then
            creature:CastSpell(target, SPELL_REMOTE_TOY)
        end
        st.remoteToy = 10000 + math.random(0, 4999)
    else
        st.remoteToy = st.remoteToy - 1000
    end
end

-- C++ ctor / Initialize() values: bomb 10000 / remote toy
-- 5000. The C++ JustEngagedWith talks SAY_TELONICUS_AGGRO.
RegisterCreatureEvent(ENTRY_TELONICUS, 1, function(_, creature)
    local guid = creature:GetGUID()
    cancelCombatPump(guid)
    combatStates[guid] = {
        bomb = 10000,
        remoteToy = 5000,
    }
    creature:Talk(0)
    combatTimers[guid] = CreateLuaEvent(function()
        telonicusTick(creature, guid)
    end, 1000, 0)
end)

-- C++ Reset() (called on evade): fullReset — see above.
RegisterCreatureEvent(ENTRY_TELONICUS, 2, function(_, creature)
    fullReset(creature:GetGUID())
end)

-- C++ JustDied: SAY_TELONICUS_DEATH fires only after the
-- unmodeled resurrection (_hasRessurrected gate) — never
-- fires in this model (solarian precedent); cleanup only.
RegisterCreatureEvent(ENTRY_TELONICUS, 4, function(_, creature)
    fullReset(creature:GetGUID())
end)

-- Eluna's On_Reset fires ahead of OnDied and OnSpawn, so
-- this lands the same Reset() reset as the evade hook.
RegisterCreatureEvent(ENTRY_TELONICUS, 23, function(_, creature)
    fullReset(creature:GetGUID())
end)
