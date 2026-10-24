-- Hydross the Unstable (Serpentshrine Cavern) — Lua port of
-- src/server/scripts/Outland/CoilfangReservoir/SerpentShrine/
-- boss_hydross_the_unstable.cpp (boss_hydross_the_unstable;
-- AddSC_boss_hydross_the_unstable registers that one C++ ScriptName).
-- serpent_shrine.h defines no NPC_HYDROSS entry constant (it carries
-- only DATA_HYDROSSTHEUNSTABLEEVENT = 3); the entry is verified
-- externally per the gruul precedent — Hydross the Unstable is npc
-- 21216 (tbc.cavernoftime.com/npc=21216, twinhead, tauri); the
-- creature_template ScriptName binding is DB-side (no TDB in this
-- workspace). Talk lines used: SAY_AGGRO=0 (pull), SAY_SWITCH_TO_
-- CLEAN=1 (clean-form switch), SAY_CLEAN_SLAY=2 (kill while clean),
-- SAY_CLEAN_DEATH=3 (death while clean), SAY_SWITCH_TO_CORRUPT=4
-- (corrupt-form switch), SAY_CORRUPT_SLAY=5 (kill while corrupted),
-- SAY_CORRUPT_DEATH=6 (death while corrupted). Eluna creature events:
-- 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied, 4 OnDied, 23
-- OnReset. Timers via CreateLuaEvent (per-GUID scheduler pump);
-- melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. C++ DoCast default is triggered=false
-- (vaelastrasz convention); DoCastVictim takes nil as the victim
-- arm (felmyst convention).
-- Fight shape (C++-exact for the modeled arms): OnEnterCombat(1):
-- per-GUID scheduler reset to the C++ Initialize() values (PosCheck
-- 2.5s / MarkOfHydross 15s / MarkOfCorruption 15s / WaterTomb 7s /
-- VileSludge 7s / MarkOfHydross_Count 0 / MarkOfCorruption_Count 0
-- / Enrage 10min / CorruptedForm false) + Talk(SAY_AGGRO) + start
-- of a 1s scheduler pump (a port of boss_hydross_the_unstableAI::
-- UpdateAI — 1s granularity is exact for the 7s/15s/10min/60s
-- timers; the 2.5s PosCheck uses fractional countdown arithmetic,
-- C++-exact). The instance SetData(IN_PROGRESS) arm is blocked.
-- Corrupted form: MarkOfCorruption 15s then 15s — DoCastVictim of
-- 38219/38220/38221/38222/38230/40583 by stack count; count
-- increments while < 5 (so the sixth spell repeats — C++-exact).
-- Vile sludge 38246 7s then 15s on a random alive player in the
-- instance (C++ SelectTarget(Random, 0), teron convention; nil pick
-- casts nothing but the timer re-arms, C++-exact). PosCheck 2.5s:
-- if within 18 yd (2D) of (HYDROSS_X, HYDROSS_Y) -> switch to clean
-- form: CorruptedForm=false, MarkOfHydross_Count=0, Talk(SAY_SWITCH_
-- TO_CLEAN); the SetDisplayId(MODEL_CLEAN 20162) arm is PORTED
-- 2026-10-20 (C++ arm order — first at the switch) and ResetThreatList
-- is PORTED 2026-10-17; the SummonBeams, 4x DoSpawnCreature(ENTRY_
-- PURE_SPAWN 22035) and SetMeleeDamageSchool(frost) arms have no
-- bridges (below); the ApplySpellImmune school arms are ported
-- (below). Clean form: MarkOfHydross 15s then 15s —
-- DoCastVictim of 38215/38216/38217/38218/38231/40584 by stack
-- count, same escalation rule. Water tomb 38235 7s then 7s — the
-- C++ SelectTarget(Random, 0, 100, true) player-only 100-yd pick is
-- a random alive player in the instance within 100 yd (creature:
-- GetDistance bridge); nil pick skips the cast, timer re-arms
-- (C++-exact). PosCheck 2.5s: if NOT within 18 yd -> switch to
-- corrupted form: CorruptedForm=true, MarkOfCorruption_Count=0,
-- Talk(SAY_SWITCH_TO_CORRUPT); the SetDisplayId(MODEL_CORRUPT
-- 20609) arm is PORTED 2026-10-20 (C++ arm order — first at the
-- switch) and ResetThreatList is PORTED 2026-10-17; the
-- DeSummonBeams, 4x DoSpawnCreature(ENTRY_TAINTED_SPAWN 22036)
-- and SetMeleeDamageSchool(nature) arms stay out (no bridges);
-- the ApplySpellImmune school arms are PORTED at the switch
-- points (below). Enrage (both
-- forms): 27680 10min then 60s, non-triggered self-cast
-- (C++-exact — the spell carries "needs verification" in C++, kept
-- verbatim). OnTargetDied(3): Talk(corrupted ? SAY_CORRUPT_SLAY :
-- SAY_CLEAN_SLAY) — no TYPEID gate in C++ (C++-exact). OnDied(4):
-- Talk(corrupted ? SAY_CORRUPT_DEATH : SAY_CLEAN_DEATH) + cleanup
-- (the SetData(DONE) arm is instance-blocked). OnLeaveCombat(2)/
-- OnReset(23): cancel the pump, drop per-GUID state (the Reset-side
-- DeSummonBeams + Summons.DespawnAll are summon-blocked; the
-- instance SetData(NOT_STARTED) and SetMeleeDamageSchool have no
-- bridges; the ApplySpellImmune Reset/tick arms are PORTED 2026-10-20
-- (creature:ApplySpellImmune bridge, lua_creature_events.go); the
-- SetDisplayId arm is ported).
-- Deliberate deviations (all await engine bridges): no summon
-- bridge — the first-UpdateAI-tick SummonBeams pair (ENTRY_BEAM_
-- DUMMY 21934 + triggered SPELL_BLUE_BEAM 38015), the 4x pure/
-- tainted spawn DoSpawnCreature arms and the DeSummonBeams/
-- Summons.DespawnAll arms unmodeled; the beams are summoned on
-- the very first UpdateAI tick even out of combat in C++, which
-- has no out-of-combat update hook in the Lua model — unmodeled;
-- the JustSummoned elemental-spawnin (25035) + ApplySpellImmune
-- arms are summon-blocked; ResetThreatList on both form switches
-- PORTED 2026-10-17 (creature:ResetAllThreat, bridged
-- lua_objects.go:389, C++ arm order after Talk); the
-- MODEL_CORRUPT/MODEL_CLEAN SetDisplayId swaps are PORTED
-- 2026-10-20 (handler-surface creature:SetDisplayId bridge,
-- lua_creature_events.go — C++ arm order, first at both switches
-- and at Reset); no school bridge — SetMeleeDamageSchool
-- unmodeled, and the JustSummoned elemental ApplySpellImmune arms
-- stay summon-blocked; the Reset + both form-switch ApplySpellImmune
-- frost/nature arms are PORTED 2026-10-20 (the karazhan mana-feeder
-- convention); no instance-script model —
-- boss admission via the luaBossAI shim (DATA_HYDROSSTHEUNSTABLE
-- EVENT NOT_STARTED/IN_PROGRESS/DONE bookkeeping skipped).

local SPELL_MARK_OF_HYDROSS = { 38215, 38216, 38217, 38218, 38231, 40584 }
local SPELL_MARK_OF_CORRUPTION = { 38219, 38220, 38221, 38222, 38230, 40583 }

local SPELL_WATER_TOMB = 38235
local SPELL_VILE_SLUDGE = 38246
local SPELL_ENRAGE = 27680

local SAY_AGGRO = 0
local SAY_SWITCH_TO_CLEAN = 1
local SAY_CLEAN_SLAY = 2
local SAY_CLEAN_DEATH = 3
local SAY_SWITCH_TO_CORRUPT = 4
local SAY_CORRUPT_SLAY = 5
local SAY_CORRUPT_DEATH = 6

local ENTRY_HYDROSS = 21216

local HYDROSS_X = -239.439
local HYDROSS_Y = -363.481
local SWITCH_RADIUS = 18

local pumpTimers = {}
local hydrossState = {}

-- Alive players sharing the creature's map+instance (teron
-- convention).
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

-- C++ SelectTarget(Random, 0): random alive player in the instance.
local function pickRandomAlivePlayer(creature)
    local found = playersInInstance(creature)
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
end

-- C++ SelectTarget(Random, 0, 100, true): random alive player in the
-- instance within 100 yd (netherspite convention — the GetDistance
-- bridge stands in for the C++ distance gate).
local function pickRandomAlivePlayerInRange(creature, maxDist)
    local found = {}
    for _, p in ipairs(playersInInstance(creature)) do
        if creature:GetDistance(p) <= maxDist then
            found[#found + 1] = p
        end
    end
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
end

local function cancelPump(guid)
    local id = pumpTimers[guid]
    if id then
        RemoveEventById(id)
        pumpTimers[guid] = nil
    end
end

local function resetState(guid)
    cancelPump(guid)
    hydrossState[guid] = nil
end

-- C++ Initialize(): posCheck 2500, marks 15000, waterTomb 7000,
-- vileSludge 7000, counts 0, enrage 600000, clean form.
local function freshHydrossState()
    return {
        posCheck = 2500,
        markHydross = 15000,
        markCorruption = 15000,
        waterTomb = 7000,
        vileSludge = 7000,
        hydrossCount = 0,
        corruptCount = 0,
        enrage = 600000,
        corrupted = false,
    }
end

local function hydrossTick(creature, guid)
    local st = hydrossState[guid]
    if not st then
        return
    end

    if st.corrupted then
        -- Mark of Corruption: escalating stack, repeats the sixth
        -- spell once the count is maxed (C++-exact).
        st.markCorruption = st.markCorruption - 1000
        if st.markCorruption <= 0 then
            creature:CastSpell(nil, SPELL_MARK_OF_CORRUPTION[st.corruptCount + 1])
            if st.corruptCount < 5 then
                st.corruptCount = st.corruptCount + 1
            end
            st.markCorruption = 15000
        end

        -- Vile Sludge: random pick, nil keeps the schedule (C++-exact).
        st.vileSludge = st.vileSludge - 1000
        if st.vileSludge <= 0 then
            local pick = pickRandomAlivePlayer(creature)
            if pick then
                creature:CastSpell(pick, SPELL_VILE_SLUDGE)
            end
            st.vileSludge = 15000
        end

        -- PosCheck: switch back to clean form inside the radius.
        -- The beam-summon / add-spawn / melee-school arms stay out
        -- (no bridges); the SetDisplayId(MODEL_CLEAN 20162) arm is
        -- PORTED 2026-10-20 (C++ arm order — first); the school
        -- immunity arms ride
        -- the creature:ApplySpellImmune bridge (C++ arm order after
        -- SetMeleeDamageSchool). ResetThreatList PORTED 2026-10-17
        -- (creature:ResetAllThreat, bridged lua_objects.go:389, C++
        -- arm order after Talk).
        st.posCheck = st.posCheck - 1000
        if st.posCheck <= 0 then
            local dx = creature:GetX() - HYDROSS_X
            local dy = creature:GetY() - HYDROSS_Y
            if dx * dx + dy * dy <= SWITCH_RADIUS * SWITCH_RADIUS then
                -- C++ :289: SetDisplayId(MODEL_CLEAN) — first arm.
                creature:SetDisplayId(20162)
                st.corrupted = false
                st.hydrossCount = 0
                creature:Talk(SAY_SWITCH_TO_CLEAN)
                creature:ResetAllThreat()
                -- C++ :304-305: clean form is frost-school immune,
                -- nature explicitly not (IMMUNITY_SCHOOL).
                creature:ApplySpellImmune(0, 2, 16, true)
                creature:ApplySpellImmune(0, 2, 8, false)
            end
            st.posCheck = 2500
        end
    else
        -- Mark of Hydross: same escalation rule as the corruption
        -- stack (C++-exact).
        st.markHydross = st.markHydross - 1000
        if st.markHydross <= 0 then
            creature:CastSpell(nil, SPELL_MARK_OF_HYDROSS[st.hydrossCount + 1])
            if st.hydrossCount < 5 then
                st.hydrossCount = st.hydrossCount + 1
            end
            st.markHydross = 15000
        end

        -- Water Tomb: random alive player within 100 yd; nil pick
        -- skips the cast, timer re-arms (C++-exact).
        st.waterTomb = st.waterTomb - 1000
        if st.waterTomb <= 0 then
            local pick = pickRandomAlivePlayerInRange(creature, 100)
            if pick then
                creature:CastSpell(pick, SPELL_WATER_TOMB)
            end
            st.waterTomb = 7000
        end

        -- PosCheck: switch to corrupted form outside the radius.
        -- The beam-despawn / add-spawn / melee-school arms stay out
        -- (no bridges); the SetDisplayId(MODEL_CORRUPT 20609) arm is
        -- PORTED 2026-10-20 (C++ arm order — first); the school
        -- immunity arms ride
        -- the creature:ApplySpellImmune bridge (C++ arm order after
        -- SetMeleeDamageSchool). ResetThreatList PORTED 2026-10-17
        -- (creature:ResetAllThreat, bridged lua_objects.go:389, C++
        -- arm order after Talk).
        st.posCheck = st.posCheck - 1000
        if st.posCheck <= 0 then
            local dx = creature:GetX() - HYDROSS_X
            local dy = creature:GetY() - HYDROSS_Y
            if dx * dx + dy * dy > SWITCH_RADIUS * SWITCH_RADIUS then
                -- C++ :373: SetDisplayId(MODEL_CORRUPT) — first arm.
                creature:SetDisplayId(20609)
                st.corrupted = true
                st.corruptCount = 0
                creature:Talk(SAY_SWITCH_TO_CORRUPT)
                creature:ResetAllThreat()
                -- C++ :388-389: corrupted form is nature-school immune,
                -- frost explicitly not (IMMUNITY_SCHOOL).
                creature:ApplySpellImmune(0, 2, 8, true)
                creature:ApplySpellImmune(0, 2, 16, false)
            end
            st.posCheck = 2500
        end
    end

    -- Enrage: 10min first, then 60s repeats (both forms, C++-exact).
    st.enrage = st.enrage - 1000
    if st.enrage <= 0 then
        creature:CastSpell(creature, SPELL_ENRAGE)
        st.enrage = 60000
    end
end

local function hydrossEnterCombat(event, creature)
    local guid = creature:GetGUID()
    resetState(guid)
    hydrossState[guid] = freshHydrossState()
    creature:Talk(SAY_AGGRO)
    pumpTimers[guid] = CreateLuaEvent(function()
        hydrossTick(creature, guid)
    end, 1000, 0)
end

local function hydrossLeaveCombat(event, creature)
    resetState(creature:GetGUID())
    -- C++ Reset() :143 runs on evade: restore the clean model.
    creature:SetDisplayId(20162)
end

-- C++ KilledUnit: per-form slay line, no TYPEID gate.
local function hydrossTargetDied(event, creature, victim)
    local st = hydrossState[creature:GetGUID()]
    local corrupted = st and st.corrupted or false
    if corrupted then
        creature:Talk(SAY_CORRUPT_SLAY)
    else
        creature:Talk(SAY_CLEAN_SLAY)
    end
end

local function hydrossDied(event, creature, killer)
    local st = hydrossState[creature:GetGUID()]
    local corrupted = st and st.corrupted or false
    if corrupted then
        creature:Talk(SAY_CORRUPT_DEATH)
    else
        creature:Talk(SAY_CLEAN_DEATH)
    end
    resetState(creature:GetGUID())
end

local function hydrossReset(event, creature)
    resetState(creature:GetGUID())
    -- C++ Reset() :140-141: clean-form school immunities (frost-school
    -- immune, nature explicitly not) — the creature:ApplySpellImmune
    -- bridge (lua_creature_events.go).
    creature:ApplySpellImmune(0, 2, 16, true)
    creature:ApplySpellImmune(0, 2, 8, false)
    -- C++ Reset() :143: SetDisplayId(MODEL_CLEAN) — C++ arm order
    -- after the school-immunity arms.
    creature:SetDisplayId(20162)
end

RegisterCreatureEvent(ENTRY_HYDROSS, 1, hydrossEnterCombat)
RegisterCreatureEvent(ENTRY_HYDROSS, 2, hydrossLeaveCombat)
RegisterCreatureEvent(ENTRY_HYDROSS, 3, hydrossTargetDied)
RegisterCreatureEvent(ENTRY_HYDROSS, 4, hydrossDied)
RegisterCreatureEvent(ENTRY_HYDROSS, 23, hydrossReset)
