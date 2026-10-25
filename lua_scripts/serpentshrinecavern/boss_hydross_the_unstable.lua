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
-- is PORTED 2026-10-17; SummonBeams is PORTED 2026-10-24
-- (summonBeams, C++ order after ResetThreatList); the 4x
-- DoSpawnCreature(ENTRY_PURE_SPAWN 22035) arm is PORTED 2026-10-25
-- (spawnAdds, C++ order after SummonBeams) and SetMeleeDamageSchool
-- (frost) stays out (no bridge); the ApplySpellImmune
-- school arms are ported
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
-- switch) and ResetThreatList is PORTED 2026-10-17;
-- DeSummonBeams is PORTED 2026-10-24 (despawnBeams, C++ order
-- after ResetThreatList); the 4x DoSpawnCreature(ENTRY_TAINTED_
-- SPAWN 22036) arm is PORTED 2026-10-25 (spawnAdds, C++ order after
-- DeSummonBeams) and SetMeleeDamageSchool(nature) stays out
-- (no bridge);
-- the ApplySpellImmune school arms are PORTED at the switch
-- points (below). Enrage (both
-- forms): 27680 10min then 60s, non-triggered self-cast
-- (C++-exact — the spell carries "needs verification" in C++, kept
-- verbatim). OnTargetDied(3): Talk(corrupted ? SAY_CORRUPT_SLAY :
-- SAY_CLEAN_SLAY) — no TYPEID gate in C++ (C++-exact). OnDied(4):
-- Talk(corrupted ? SAY_CORRUPT_DEATH : SAY_CLEAN_DEATH) + cleanup
-- (the SetData(DONE) arm is instance-blocked). OnLeaveCombat(2)/
-- OnReset(23): cancel the pump, drop per-GUID state, despawn the
-- stored beam pair (C++ Reset :137 DeSummonBeams, PORTED
-- 2026-10-24; the adds despawn via despawnAdds, PORTED 2026-10-25
-- — beams are NOT in Summons);
-- the instance SetData(NOT_STARTED) and SetMeleeDamageSchool have
-- no bridges; the ApplySpellImmune Reset/tick arms are PORTED
-- 2026-10-20 (creature:ApplySpellImmune bridge,
-- lua_creature_events.go); the SetDisplayId arm is ported).
-- Deliberate deviations (all await engine bridges): the beam pair
-- is PORTED 2026-10-24 (summonBeams/despawnBeams: SummonCreature
-- type 4 + beamer CastSpell + SetDisplayId + SetFlag; clean-switch
-- re-summon overwrites the stored pair, orphaning the old beams
-- exactly as C++ does); the pair lands at pull instead of on the
-- pre-pull first UpdateAI tick via event 7 (PORTED 2026-10-24:
-- event 7 fires on the 100ms world tick for every alive motion,
-- creaturemotion.go, so the latch twin lands pre-pull as in C++;
-- gated 2026-10-25 to pre-pull/post-evade only, so the corrupt-switch
-- despawn sticks as in C++ where the beam flag is consumed once);
-- the 4x pure/tainted spawn DoSpawnCreature arms and
-- Summons.DespawnAll are PORTED 2026-10-25 (spawnAdds/despawnAdds);
-- the JustSummoned elemental-spawnin (25035) + ApplySpellImmune
-- arms are PORTED 2026-10-25 (hydrossJustSummoned, event 19); ResetThreatList on both form switches
-- PORTED 2026-10-17 (creature:ResetAllThreat, bridged
-- lua_objects.go:389, C++ arm order after Talk); the
-- MODEL_CORRUPT/MODEL_CLEAN SetDisplayId swaps are PORTED
-- 2026-10-20 (handler-surface creature:SetDisplayId bridge,
-- lua_creature_events.go — C++ arm order, first at both switches
-- and at Reset); no school bridge — SetMeleeDamageSchool
-- unmodeled; the Reset + both form-switch ApplySpellImmune
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

local ENTRY_PURE_SPAWN = 22035 -- boss_hydross_the_unstable.cpp:66
local ENTRY_TAINTED_SPAWN = 22036 -- boss_hydross_the_unstable.cpp:67
local SPELL_ELEMENTAL_SPAWNIN = 25035 -- boss_hydross_the_unstable.cpp:63

-- C++ DoSpawnCreature offsets (boss_hydross_the_unstable.cpp:83-90):
-- DoSpawnCreature spawns at the summoner position + offsets
-- (ScriptedCreature.cpp:286-289), z +3, angle 0, type 4.
local SPAWN_OFFSETS = {
    {6.934003, -11.255012},
    {-6.934003, 11.255012},
    {-12.577011, -4.72702},
    {12.577011, 4.72702},
}

-- UNIT_FIELD_FLAGS (UpdateFields.h:117: OBJECT_END+0x35 = 59);
-- UNIT_FLAG_NOT_SELECTABLE (SharedDefines.h); SetFlag bridge live.
local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000
local ENTRY_BEAM_DUMMY = 21934 -- boss_hydross_the_unstable.cpp:68
local SPELL_BLUE_BEAM = 38015 -- boss_hydross_the_unstable.cpp:64
local BEAM_DISPLAY_ID = 11686 -- boss_hydross_the_unstable.cpp:155 (invisible)

local HYDROSS_X = -239.439
local HYDROSS_Y = -363.481
local SWITCH_RADIUS = 18

local pumpTimers = {}
local hydrossState = {}
local hydrossBeams = {}
local hydrossAdds = {}

-- C++ SummonBeams (boss_hydross_the_unstable.cpp:149-167): per beam
-- SummonCreature(BEAM_DUMMY 21934, type 4 CORPSE_DESPAWN) + beamer
-- CastSpell(hydross, BLUE_BEAM 38015, triggered) + SetDisplayId
-- (11686, invisible) + SetFlag(UNIT_FIELD_FLAGS, NOT_SELECTABLE).
-- Stored as objects (leotherasBinders precedent): C++ keeps GUIDs
-- and resolves them via ObjectAccessor, which has no Lua bridge.
-- C++ re-summons on every clean switch WITHOUT despawning the old
-- pair — the stored pair is overwritten, orphaning the old beams,
-- replicated here.
local function summonBeams(creature, guid)
    local beams = {}
    local cords = {
        {-258.333, -356.34, 22.0499, 5.90835},
        {-219.918, -371.308, 22.0042, 2.73072},
    }
    for _, cord in ipairs(cords) do
        local beamer = creature:SummonCreature(ENTRY_BEAM_DUMMY,
            cord[1], cord[2], cord[3], cord[4], 4, 0)
        if beamer then
            beamer:CastSpell(creature, SPELL_BLUE_BEAM, true)
            beamer:SetDisplayId(BEAM_DISPLAY_ID)
            beamer:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
            beams[#beams + 1] = beamer
        end
    end
    hydrossBeams[guid] = beams
end

-- C++ DeSummonBeams (:168-175): DespawnOrUnsummon each stored beam.
local function despawnBeams(guid)
    local beams = hydrossBeams[guid]
    if beams then
        for _, beamer in ipairs(beams) do
            beamer:Despawn()
        end
    end
    hydrossBeams[guid] = nil
end

-- C++ form-switch spawn arms (boss_hydross_the_unstable.cpp:303-306
-- clean, :384-387 corrupt): 4x DoSpawnCreature at the boss position
-- + SPAWN_OFFSETS, z +3, angle 0, TEMPSUMMON_CORPSE_DESPAWN (4),
-- despawn 0. Stored per boss GUID (beams precedent) so the
-- Summons.DespawnAll arms on Reset/JustDied have a bearer; the
-- JustSummoned immunity + spawnin legs ride the ev19 handler below
-- (the Lua SummonCreature funnel fires event 19 on the summoner,
-- lua_creature_events.go SummonCreature comment). No DoZoneInCombat:
-- C++ spawns the adds with no engage call (their AI is the default
-- creature AI — no script exists for 22035/22036 anywhere in the
-- C++ tree — and Go melee/aggro is engine-driven).
local function spawnAdds(creature, guid, entry)
    local adds = {}
    local hx, hy, hz = creature:GetX(), creature:GetY(), creature:GetZ()
    for _, off in ipairs(SPAWN_OFFSETS) do
        local add = creature:SummonCreature(entry,
            hx + off[1], hy + off[2], hz + 3, 0, 4, 0)
        if add then
            adds[#adds + 1] = add
        end
    end
    hydrossAdds[guid] = adds
end

-- C++ Summons.DespawnAll (Reset :144, JustDied :210): the adds only
-- (beams are not in Summons — they have their own despawnBeams).
local function despawnAdds(guid)
    local adds = hydrossAdds[guid]
    if adds then
        for _, add in ipairs(adds) do
            add:Despawn()
        end
    end
    hydrossAdds[guid] = nil
end

-- C++ SelectTarget(Random, 0): random alive unit from the threat
-- list (any unit, pets included) — GetAITargets is the bridged
-- threat list; dead entries skipped (C++ SelectTargetList skip).
local function randomThreatTarget(creature)
    local targets = creature:GetAITargets()
    local out = {}
    for _, unit in ipairs(targets) do
        if not unit:IsDead() then
            out[#out + 1] = unit
        end
    end
    if #out == 0 then
        return nil
    end
    return out[math.random(#out)]
end

-- C++ SelectTarget(Random, 0, <dist>, true): random alive
-- threat-list player within <dist> yd.
local function randomThreatPlayerInRange(creature, maxDist)
    local targets = creature:GetAITargets()
    local out = {}
    for _, unit in ipairs(targets) do
        if unit:GetObjectType() == "Player" and not unit:IsDead()
                and creature:GetDistance(unit) <= maxDist then
            out[#out + 1] = unit
        end
    end
    if #out == 0 then
        return nil
    end
    return out[math.random(#out)]
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
            local pick = randomThreatTarget(creature)
            if pick then
                creature:CastSpell(pick, SPELL_VILE_SLUDGE)
            end
            st.vileSludge = 15000
        end

        -- PosCheck: switch back to clean form inside the radius.
        -- The melee-school arm stays out (no bridge); the
        -- SetDisplayId(MODEL_CLEAN 20162) arm is
        -- PORTED 2026-10-20 (C++ arm order — first); the school
        -- immunity arms ride
        -- the creature:ApplySpellImmune bridge (C++ arm order after
        -- SetMeleeDamageSchool). ResetThreatList PORTED 2026-10-17
        -- (creature:ResetAllThreat, bridged lua_objects.go:389, C++
        -- arm order after Talk). The 4x pure-spawn arm is PORTED
        -- 2026-10-25 (spawnAdds, C++ order after SummonBeams).
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
                summonBeams(creature, guid)
                spawnAdds(creature, guid, ENTRY_PURE_SPAWN)
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

        -- Water Tomb: random threat-list player within 100 yd; nil pick
        -- skips the cast, timer re-arms (C++-exact).
        st.waterTomb = st.waterTomb - 1000
        if st.waterTomb <= 0 then
            local pick = randomThreatPlayerInRange(creature, 100)
            if pick then
                creature:CastSpell(pick, SPELL_WATER_TOMB)
            end
            st.waterTomb = 7000
        end

        -- PosCheck: switch to corrupted form outside the radius.
        -- The melee-school arm stays out (no bridge); the
        -- SetDisplayId(MODEL_CORRUPT 20609) arm is
        -- PORTED 2026-10-20 (C++ arm order — first); the school
        -- immunity arms ride
        -- the creature:ApplySpellImmune bridge (C++ arm order after
        -- SetMeleeDamageSchool). ResetThreatList PORTED 2026-10-17
        -- (creature:ResetAllThreat, bridged lua_objects.go:389, C++
        -- arm order after Talk). The 4x tainted-spawn arm is PORTED
        -- 2026-10-25 (spawnAdds, C++ order after DeSummonBeams).
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
                despawnBeams(guid)
                spawnAdds(creature, guid, ENTRY_TAINTED_SPAWN)
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

-- C++ UpdateAI first-tick beam latch (boss_hydross_the_unstable.cpp:213-217)
-- PORTED: event 7 fires on the 100ms world tick for every alive motion
-- (creaturemotion.go), including pre-pull, so the !beam latch twin is
-- hydrossBeams[guid] == nil. After evade, LeaveCombat/Reset despawn the
-- pair and nil the slot (C++ Reset :137 DeSummonBeams + Initialize
-- beam=false), so the next tick re-summons pre-pull exactly as C++.
-- After death the motion stops ticking, so the standing pair is left
-- alone (C++ JustDied does not despawn beams). Returns nothing, so no
-- veto of the engine combat step (dathrohan precedent).
-- 2026-10-25 GATE: event 7 also fires every tick DURING combat, so an
-- ungated latch re-summoned the pair right after the corrupted-form
-- switch despawned it — C++ never re-summons there (the beam flag is
-- consumed on the first UpdateAI tick and only Reset clears it). The
-- latch therefore fires only while hydrossState[guid] is nil, i.e.
-- pre-pull / post-evade; in-combat switches summon/despawn directly.
local function hydrossAIUpdate(event, creature)
    local guid = creature:GetGUID()
    if hydrossState[guid] then
        return
    end
    if not hydrossBeams[guid] or #hydrossBeams[guid] == 0 then
        summonBeams(creature, guid)
    end
end

local function hydrossEnterCombat(event, creature)
    local guid = creature:GetGUID()
    resetState(guid)
    hydrossState[guid] = freshHydrossState()
    creature:Talk(SAY_AGGRO)
    -- C++ first-UpdateAI-tick SummonBeams latch (boss_hydross_the_
    -- unstable.cpp:217-221): the beam flag starts false, so the pair
    -- exists pre-pull in C++. The event-7 handler above arms the same
    -- latch pre-pull; this check is a fallback no-op when a pair
    -- already stands (no re-summon if a pair already stands).
    if not hydrossBeams[guid] or #hydrossBeams[guid] == 0 then
        summonBeams(creature, guid)
    end
    pumpTimers[guid] = CreateLuaEvent(function()
        hydrossTick(creature, guid)
    end, 1000, 0)
end

local function hydrossLeaveCombat(event, creature)
    despawnBeams(creature:GetGUID())
    despawnAdds(creature:GetGUID())
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
    despawnAdds(creature:GetGUID())
    resetState(creature:GetGUID())
end

local function hydrossReset(event, creature)
    despawnBeams(creature:GetGUID())
    despawnAdds(creature:GetGUID())
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

-- C++ JustSummoned (boss_hydross_the_unstable.cpp:196-206): the
-- elemental spawn gets its school immunity + the triggered
-- ELEMENTAL_SPAWNIN cast, then joins Summons (the table above
-- already tracks it via spawnAdds). Entry-gated: the beam dummies
-- summoned through the same funnel (21934) fall through untouched.
-- PORTED 2026-10-25 (the Lua SummonCreature funnel fires event 19
-- on the summoner, lua_creature_events.go SummonCreature comment).
local function hydrossJustSummoned(event, creature, summoned)
    local entry = summoned:GetEntry()
    if entry == ENTRY_PURE_SPAWN then
        summoned:ApplySpellImmune(0, 2, 16, true)
        summoned:CastSpell(summoned, SPELL_ELEMENTAL_SPAWNIN, true)
    elseif entry == ENTRY_TAINTED_SPAWN then
        summoned:ApplySpellImmune(0, 2, 8, true)
        summoned:CastSpell(summoned, SPELL_ELEMENTAL_SPAWNIN, true)
    end
end

RegisterCreatureEvent(ENTRY_HYDROSS, 1, hydrossEnterCombat)
RegisterCreatureEvent(ENTRY_HYDROSS, 19, hydrossJustSummoned)
RegisterCreatureEvent(ENTRY_HYDROSS, 7, hydrossAIUpdate)
RegisterCreatureEvent(ENTRY_HYDROSS, 2, hydrossLeaveCombat)
RegisterCreatureEvent(ENTRY_HYDROSS, 3, hydrossTargetDied)
RegisterCreatureEvent(ENTRY_HYDROSS, 4, hydrossDied)
RegisterCreatureEvent(ENTRY_HYDROSS, 23, hydrossReset)
