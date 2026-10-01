-- Fathom-Lord Karathress (Serpentshrine Cavern) — Lua port of
-- src/server/scripts/Outland/CoilfangReservoir/SerpentShrine/
-- boss_fathomlord_karathress.cpp (boss_fathomlord_karathress,
-- boss_fathomguard_sharkkis, boss_fathomguard_tidalvess,
-- boss_fathomguard_caribdis; AddSC_boss_fathomlord_karathress
-- registers those four C++ ScriptNames). serpent_shrine.h
-- defines no NPC_ entry constants; the entries are verified
-- from instance_serpent_shrine.cpp's OnCreatureCreate entry->
-- GUID mapping: case 21214 -> Karathress (DATA_KARATHRESS), case
-- 21966 -> Sharkkis (DATA_SHARKKIS), case 21965 -> Tidalvess
-- (DATA_TIDALVESS), case 21964 -> Caribdis (DATA_CARIBDIS); the
-- creature_template ScriptName bindings are DB-side (no TDB in
-- this workspace). Entries registered: 21214 Karathress, 21966
-- Sharkkis, 21965 Tidalvess, 21964 Caribdis. Karathress Talk
-- lines used: SAY_AGGRO=0 (pull), SAY_SLAY=5 (kill — no TYPEID
-- gate in C++, C++-exact), SAY_DEATH=6 (death). SAY_GAIN_
-- BLESSING=1 is defined but never called in C++ (documented
-- only); SAY_GAIN_ABILITY1..3 (2/3/4) fire in the
-- EventSharkkis/Tidalvess/CaribdisDeath relays, which are
-- cross-creature blocked (below). The advisors have no Talk
-- calls in C++. Eluna creature events: 1 OnEnterCombat, 2
-- OnLeaveCombat, 3 OnTargetDied, 4 OnDied, 23 OnReset. Timers
-- via CreateLuaEvent (per-GUID named schedule helper); melee is
-- engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. C++ DoCast default is triggered=false
-- (vaelastrasz convention); DoCastVictim takes nil as the
-- victim arm (felmyst convention).
-- Fight shape (C++-exact for the modeled arms): Karathress
-- (21214). OnEnterCombat(1): per-GUID scheduler reset to the
-- C++ Initialize() values + Talk(SAY_AGGRO) + start of a 1s
-- scheduler pump (a port of boss_fathomlord_karathressAI::
-- UpdateAI — 1s granularity is exact for all C++ timers; the
-- StartEvent Talk(SAY_AGGRO) / DoZoneInCombat / SetData(IN_
-- PROGRESS) / SetGuidData(STARTER) arms are instance/z-combat
-- blocked). Pump per tick: cataclysmic bolt 38441 10s then 10s
-- — pick = random alive player in the instance excluding the
-- victim (C++ SelectTarget(Random, 1), "other than the main
-- tank" — curator/teron convention, no threat model); if no
-- such pick, DoCastVictim fallback (C++-exact: "if there aren't
-- other units, cast on the tank"). Sear nova 38445 20-60s then
-- 20-60s, DoCastVictim (C++-exact). Enrage 24318 600s then 90s
-- repeat, non-triggered self-cast (C++-exact). Blessing of the
-- Tides: once-guarded at <75% health (C++-exact) — non-
-- triggered self-cast 38449; the C++ "at least one advisor
-- alive" precondition has no cross-creature bridge (the advisor
-- DoAction EventSharkkis/Tidalvess/CaribdisDeath relays and the
-- advisors' GUID bookkeeping live in the instance script), and
-- the me->Yell(raw string)/DoPlaySoundToSet(11278) arms have no
-- yell/sound bridges (gurtogg convention) — all documented.
-- OnTargetDied(3): Talk(SAY_SLAY) (C++-exact — no TYPEID gate).
-- OnDied(4): Talk(SAY_DEATH) + cleanup (the SetData(FATHOMLORD_
-- KARATHRESSEVENT, DONE) arm is instance-blocked; the Seer Olum
-- 22820 quest-10944 summon arm is summon-blocked). OnLeaveCombat
-- (2)/OnReset(23): cancel the pump, drop per-GUID state (the
-- evade-side advisor respawn + MoveTargetedHome + SetData(NOT_
-- STARTED) arms are instance/movement blocked).
-- Sharkkis (21966). OnEnterCombat(1): per-GUID scheduler reset
-- to the C++ Initialize() values + 1s pump (the instance SetData
-- (IN_PROGRESS)/SetGuidData(STARTER) arms blocked). Pump:
-- leeching throw 29436 20s then 20s, DoCastVictim (C++-exact);
-- multishot 38366 15s then 20s, DoCastVictim (C++-exact); the
-- beast within 38373 30s then 30s, non-triggered self-cast
-- (the pet-enrage arm is summon-blocked — the pet object has no
-- bridge); the 10s one-shot pet arm (DoSpawnCreature lurker
-- 22119 / sporebat 22120, pet_id by urand(0,1)) has no summon
-- bridge — unmodeled (firesworn convention). OnDied(4):
-- cleanup — the JustDied EventSharkkisDeath relay into
-- Karathress is cross-creature blocked. OnLeaveCombat(2)/
-- OnReset(23): cancel the pump, drop per-GUID state (the evade-
-- side pet KillSelf arm is summon-blocked).
-- Tidalvess (21965). OnEnterCombat(1): per-GUID scheduler reset
-- + non-triggered self-cast windfury weapon 38184 (C++-exact
-- JustEngagedWith arm) + 1s pump. Pump: windfury upkeep — if
-- the creature lacks aura 38184, non-triggered self-cast
-- (C++-exact HasAura poll); frost shock 38234 25s then {25s,30s
-- }, DoCastVictim (C++-exact); spitfire totem 38236 60s then
-- 60s, non-triggered self-cast (the FindNearestCreature(
-- 22091, 100yd) AttackStart relay has no nearest-creature/
-- cross-creature bridge); poison cleansing totem 38306 30s
-- then 30s, non-triggered self-cast; earthbind totem 38304 45s
-- then 45s, non-triggered self-cast (cast despite the C++
-- "Spell obsolete" comment — C++-exact). OnDied(4): cleanup
-- (EventTidalvessDeath relay blocked). OnLeaveCombat(2)/
-- OnReset(23): cancel the pump, drop per-GUID state.
-- Caribdis (21964). OnEnterCombat(1): per-GUID scheduler reset
-- to the C++ Initialize() values + 1s pump. Pump: water bolt
-- volley 38335 35s then 30s, DoCastVictim (C++-exact); tidal
-- surge 38358 {15s,20s} then {15s,20s}, DoCastVictim (C++-
-- exact — the "hacky" victim self-cast triggered freeze 38357
-- arm has no bridge: the Go engine exposes CastSpell only on
-- the creature wrapper, so another unit cannot be made to cast
-- — unmodeled); heal 38330 55s then 60s — C++ picks a live
-- advisor from {karathress, sharkkis, tidalvess, self} via the
-- instance GUIDs, which have no bridge: only the caster itself
-- is resolvable, so the heal is cast on self (documented
-- deviation — the advisor heal support is cross-creature
-- blocked); the cyclone 30-40s arm is unmodeled (the C++
-- SPELL_SUMMON_CYCLONE cast is commented out as broken, and the
-- SummonCreature(22104) + flag/scale/faction/cast/AttackStart
-- chain has no summon/flag/cross-creature bridges). OnDied(4):
-- cleanup (EventCaribdisDeath relay blocked). OnLeaveCombat(2)/
-- OnReset(23): cancel the pump, drop per-GUID state.
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI shim
-- (the DATA_KARATHRESSEVENT / DATA_KARATHRESSEVENT_STARTER /
-- DATA_FATHOMLORDKARATHRESSEVENT SetData/GetData gates, the
-- DATA_KARATHRESS/DATA_SHARKKIS/DATA_TIDALVESS/DATA_CARIBDIS
-- advisor GUID bookkeeping, the starter-target re-engage arm,
-- the evade-mode event reset and the advisor respawn arms
-- skipped); no cross-creature bridge — the advisor-death DoAction
-- relays (EventSharkkis/Tidalvess/CaribdisDeath: Talk(SAY_GAIN_
-- ABILITY1..3) + self-cast POWER_OF_SHARKKIS 38455 / TIDALVESS
-- 38452 / CARIBDIS 38451) unmodeled; no summon bridge — the
-- sharkkis pet spawns, the caribdis cyclone summon chain and
-- the Seer Olum quest-10944 summon unmodeled; no movement/
-- threat/nearest-creature bridges — the spitfire-totem target
-- relay, the advisor MoveTargetedHome respawn arms and the
-- cataclysmic-bolt tank-pick threat arm approximated (random
-- alive non-victim player pick); no yell/sound bridges — the
-- blessing yell + sound arms unmodeled; no UNIT_STATE bridge —
-- timers fire unconditionally (jeklik convention).

local SPELL_CATACLYSMIC_BOLT = 38441
local SPELL_SEAR_NOVA = 38445
local SPELL_ENRAGE = 24318
local SPELL_BLESSING_OF_THE_TIDES = 38449

local SPELL_LEECHING_THROW = 29436
local SPELL_THE_BEAST_WITHIN = 38373
local SPELL_MULTISHOT = 38366

local SPELL_WINDFURY_WEAPON = 38184
local SPELL_FROST_SHOCK = 38234
local SPELL_SPITFIRE_TOTEM = 38236
local SPELL_POISON_CLEANSING_TOTEM = 38306
local SPELL_EARTHBIND_TOTEM = 38304

local SPELL_WATER_BOLT_VOLLEY = 38335
local SPELL_TIDAL_SURGE = 38358
local SPELL_HEAL = 38330

local SAY_AGGRO = 0
local SAY_SLAY = 5
local SAY_DEATH = 6

local ENTRY_KARATHRESS = 21214
local ENTRY_SHARKKIS = 21966
local ENTRY_TIDALVESS = 21965
local ENTRY_CARIBDIS = 21964

local pumpTimers = {}
local karathressState = {}
local sharkkisState = {}
local tidalvessState = {}
local caribdisState = {}

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

-- C++ SelectTarget(Random, 1): random alive player other than the
-- main tank (the victim). Returns nil when no such pick exists.
local function pickRandomNonTank(creature)
    local victim = creature:GetVictim()
    local found = {}
    for _, p in ipairs(playersInInstance(creature)) do
        if not victim or p:GetGUID() ~= victim:GetGUID() then
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

local function resetState(stateTable, guid)
    cancelPump(guid)
    stateTable[guid] = nil
end

-- C++ Initialize(): cataclysmic bolt 10000, enrage 600000, sear
-- nova 20000 + rand32() % 40000, BlessingOfTides false.
local function freshKarathressState()
    return {
        cataclysmicBolt = 10000,
        enrage = 600000,
        searNova = 20000 + math.random(0, 39999),
        blessingOfTides = false,
    }
end

local function karathressTick(creature, guid)
    local st = karathressState[guid]
    if not st then
        return
    end

    -- Cataclysmic Bolt: random non-tank player, else the victim.
    st.cataclysmicBolt = st.cataclysmicBolt - 1000
    if st.cataclysmicBolt <= 0 then
        local pick = pickRandomNonTank(creature)
        if pick then
            creature:CastSpell(pick, SPELL_CATACLYSMIC_BOLT)
        else
            creature:CastSpell(nil, SPELL_CATACLYSMIC_BOLT)
        end
        st.cataclysmicBolt = 10000
    end

    -- Sear Nova: DoCastVictim, re-arm 20-60s.
    st.searNova = st.searNova - 1000
    if st.searNova <= 0 then
        creature:CastSpell(nil, SPELL_SEAR_NOVA)
        st.searNova = 20000 + math.random(0, 39999)
    end

    -- Enrage: 10min first, then 90s repeats.
    st.enrage = st.enrage - 1000
    if st.enrage <= 0 then
        creature:CastSpell(creature, SPELL_ENRAGE)
        st.enrage = 90000
    end

    -- Blessing of the Tides: once-guarded at <75% health. The
    -- C++ at-least-one-advisor-alive precondition has no cross-
    -- creature bridge; the Yell + DoPlaySoundToSet arms have no
    -- yell/sound bridges.
    if not st.blessingOfTides and creature:GetHealthPct() < 75 then
        st.blessingOfTides = true
        creature:CastSpell(creature, SPELL_BLESSING_OF_THE_TIDES)
    end
end

local function karathressEnterCombat(event, creature)
    local guid = creature:GetGUID()
    resetState(karathressState, guid)
    karathressState[guid] = freshKarathressState()
    creature:Talk(SAY_AGGRO)
    pumpTimers[guid] = CreateLuaEvent(function()
        karathressTick(creature, guid)
    end, 1000)
end

local function karathressLeaveCombat(event, creature)
    resetState(karathressState, creature:GetGUID())
end

-- C++ KilledUnit: unconditional Talk(SAY_SLAY) — no TYPEID gate.
local function karathressTargetDied(event, creature, victim)
    creature:Talk(SAY_SLAY)
end

local function karathressDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    resetState(karathressState, creature:GetGUID())
end

local function karathressReset(event, creature)
    resetState(karathressState, creature:GetGUID())
end

-- C++ Initialize(): leeching throw 20000, beast within 30000,
-- multishot 15000, pet 10000 (pet arm summon-blocked).
local function freshSharkkisState()
    return {
        leechingThrow = 20000,
        beastWithin = 30000,
        multishot = 15000,
    }
end

local function sharkkisTick(creature, guid)
    local st = sharkkisState[guid]
    if not st then
        return
    end

    st.leechingThrow = st.leechingThrow - 1000
    if st.leechingThrow <= 0 then
        creature:CastSpell(nil, SPELL_LEECHING_THROW)
        st.leechingThrow = 20000
    end

    st.multishot = st.multishot - 1000
    if st.multishot <= 0 then
        creature:CastSpell(nil, SPELL_MULTISHOT)
        st.multishot = 20000
    end

    st.beastWithin = st.beastWithin - 1000
    if st.beastWithin <= 0 then
        creature:CastSpell(creature, SPELL_THE_BEAST_WITHIN)
        st.beastWithin = 30000
    end
end

local function sharkkisEnterCombat(event, creature)
    local guid = creature:GetGUID()
    resetState(sharkkisState, guid)
    sharkkisState[guid] = freshSharkkisState()
    pumpTimers[guid] = CreateLuaEvent(function()
        sharkkisTick(creature, guid)
    end, 1000)
end

local function sharkkisLeaveCombat(event, creature)
    resetState(sharkkisState, creature:GetGUID())
end

local function sharkkisDied(event, creature, killer)
    resetState(sharkkisState, creature:GetGUID())
end

local function sharkkisReset(event, creature)
    resetState(sharkkisState, creature:GetGUID())
end

-- C++ Initialize(): frost shock 25000, spitfire 60000, poison
-- cleansing 30000, earthbind 45000.
local function freshTidalvessState()
    return {
        frostShock = 25000,
        spitfire = 60000,
        poisonCleansing = 30000,
        earthbind = 45000,
    }
end

local function tidalvessTick(creature, guid)
    local st = tidalvessState[guid]
    if not st then
        return
    end

    -- Windfury Weapon upkeep: re-cast when the aura is missing.
    if not creature:HasAura(SPELL_WINDFURY_WEAPON) then
        creature:CastSpell(creature, SPELL_WINDFURY_WEAPON)
    end

    st.frostShock = st.frostShock - 1000
    if st.frostShock <= 0 then
        creature:CastSpell(nil, SPELL_FROST_SHOCK)
        st.frostShock = 25000 + math.random(0, 4999)
    end

    st.spitfire = st.spitfire - 1000
    if st.spitfire <= 0 then
        creature:CastSpell(creature, SPELL_SPITFIRE_TOTEM)
        st.spitfire = 60000
    end

    st.poisonCleansing = st.poisonCleansing - 1000
    if st.poisonCleansing <= 0 then
        creature:CastSpell(creature, SPELL_POISON_CLEANSING_TOTEM)
        st.poisonCleansing = 30000
    end

    st.earthbind = st.earthbind - 1000
    if st.earthbind <= 0 then
        creature:CastSpell(creature, SPELL_EARTHBIND_TOTEM)
        st.earthbind = 45000
    end
end

local function tidalvessEnterCombat(event, creature)
    local guid = creature:GetGUID()
    resetState(tidalvessState, guid)
    tidalvessState[guid] = freshTidalvessState()
    creature:CastSpell(creature, SPELL_WINDFURY_WEAPON)
    pumpTimers[guid] = CreateLuaEvent(function()
        tidalvessTick(creature, guid)
    end, 1000)
end

local function tidalvessLeaveCombat(event, creature)
    resetState(tidalvessState, creature:GetGUID())
end

local function tidalvessDied(event, creature, killer)
    resetState(tidalvessState, creature:GetGUID())
end

local function tidalvessReset(event, creature)
    resetState(tidalvessState, creature:GetGUID())
end

-- C++ Initialize(): water bolt volley 35000, tidal surge 15000
-- + rand32() % 5000, heal 55000, cyclone 30000 + rand32() %
-- 10000 (cyclone arm summon-blocked).
local function freshCaribdisState()
    return {
        waterBoltVolley = 35000,
        tidalSurge = 15000 + math.random(0, 4999),
        heal = 55000,
    }
end

local function caribdisTick(creature, guid)
    local st = caribdisState[guid]
    if not st then
        return
    end

    st.waterBoltVolley = st.waterBoltVolley - 1000
    if st.waterBoltVolley <= 0 then
        creature:CastSpell(nil, SPELL_WATER_BOLT_VOLLEY)
        st.waterBoltVolley = 30000
    end

    st.tidalSurge = st.tidalSurge - 1000
    if st.tidalSurge <= 0 then
        creature:CastSpell(nil, SPELL_TIDAL_SURGE)
        st.tidalSurge = 15000 + math.random(0, 4999)
    end

    -- Heal: the C++ advisor pick (karathress/sharkkis/tidalvess/
    -- self via instance GUIDs) has no cross-creature bridge —
    -- only self is resolvable.
    st.heal = st.heal - 1000
    if st.heal <= 0 then
        creature:CastSpell(creature, SPELL_HEAL)
        st.heal = 60000
    end
end

local function caribdisEnterCombat(event, creature)
    local guid = creature:GetGUID()
    resetState(caribdisState, guid)
    caribdisState[guid] = freshCaribdisState()
    pumpTimers[guid] = CreateLuaEvent(function()
        caribdisTick(creature, guid)
    end, 1000)
end

local function caribdisLeaveCombat(event, creature)
    resetState(caribdisState, creature:GetGUID())
end

local function caribdisDied(event, creature, killer)
    resetState(caribdisState, creature:GetGUID())
end

local function caribdisReset(event, creature)
    resetState(caribdisState, creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_KARATHRESS, 1, karathressEnterCombat)
RegisterCreatureEvent(ENTRY_KARATHRESS, 2, karathressLeaveCombat)
RegisterCreatureEvent(ENTRY_KARATHRESS, 3, karathressTargetDied)
RegisterCreatureEvent(ENTRY_KARATHRESS, 4, karathressDied)
RegisterCreatureEvent(ENTRY_KARATHRESS, 23, karathressReset)

RegisterCreatureEvent(ENTRY_SHARKKIS, 1, sharkkisEnterCombat)
RegisterCreatureEvent(ENTRY_SHARKKIS, 2, sharkkisLeaveCombat)
RegisterCreatureEvent(ENTRY_SHARKKIS, 4, sharkkisDied)
RegisterCreatureEvent(ENTRY_SHARKKIS, 23, sharkkisReset)

RegisterCreatureEvent(ENTRY_TIDALVESS, 1, tidalvessEnterCombat)
RegisterCreatureEvent(ENTRY_TIDALVESS, 2, tidalvessLeaveCombat)
RegisterCreatureEvent(ENTRY_TIDALVESS, 4, tidalvessDied)
RegisterCreatureEvent(ENTRY_TIDALVESS, 23, tidalvessReset)

RegisterCreatureEvent(ENTRY_CARIBDIS, 1, caribdisEnterCombat)
RegisterCreatureEvent(ENTRY_CARIBDIS, 2, caribdisLeaveCombat)
RegisterCreatureEvent(ENTRY_CARIBDIS, 4, caribdisDied)
RegisterCreatureEvent(ENTRY_CARIBDIS, 23, caribdisReset)
