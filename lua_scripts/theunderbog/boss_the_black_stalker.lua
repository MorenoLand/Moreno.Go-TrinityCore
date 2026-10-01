-- The Black Stalker (The Underbog) — Lua port of
-- src/server/scripts/Outland/CoilfangReservoir/TheUnderbog/
-- boss_the_black_stalker.cpp (boss_the_black_stalker — the
-- only AI class AddSC_boss_the_black_stalker registers; no
-- other creature/GO scripts in this file). Second and final
-- boss in The Underbog set per outland_script_loader.cpp
-- order (hungarfen 1/2 closed; instance_the_underbog.cpp is a
-- placeholder and stays blocked on the instance-script
-- model).
-- Entry 17882 verified externally (wotlk.ezhead npc=17882,
-- wow-petopia npc=17882, db.moonwell npc=17882, tauri
-- npc=17882); the_underbog.h defines NO NPC_ entries and
-- instance_the_underbog.cpp has NO OnCreatureCreate mappings
-- at all — the entry is not verifiable from the C++ sources,
-- documented here. The creature_template ScriptName bindings
-- are DB-side (no TDB in this workspace).
-- The file has NO Talk calls (no Say enum at all) and its
-- KilledUnit override does not exist — no event 3 registered.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4
-- OnDied, 23 OnReset. Timers via CreateLuaEvent (per-GUID
-- scheduler pump); melee is engine-driven in Go (creature
-- combat tick), like C++ DoMeleeAttackIfReady. C++ DoCast
-- default is triggered=false (vaelastrasz convention).
-- Fight shape (C++-exact for all modeled arms): The Black
-- Stalker (17882): OnEnterCombat(1): per-GUID reset to the
-- C++ Initialize() values (levitate 12000 / chainLightning
-- 6000 / staticCharge 10000 / sporeStriders 10000+rand32()%
-- 5000 / check 5000 / InAir false — the C++ JustEngagedWith
-- override is empty) + 1s scheduler pump (a port of UpdateAI
-- — 1s granularity exact for all C++ timers; the
-- !UpdateVictim early return collapses into the pump). Pump:
-- levitate 31704: 12s init then 12000+rand32()%3000 — target
-- = random alive player excluding the victim (C++
-- SelectTarget(Random, 1), quagmirran convention), nil pick
-- casts nothing, non-triggered DoCast on the target, re-arm
-- regardless (C++-exact — the re-arm is outside the if
-- (target) block). Chain lightning 31717: 6s init then 7000
-- — random alive player in the instance (C++ SelectTarget
-- (Random, 0), thespia precedent), nil pick casts nothing,
-- re-arm 7000 regardless (C++-exact). Static charge 31715:
-- 10s init then 10000 — random alive player within 30 yd (C++
-- SelectTarget(Random, 0, 30, true), thespia distance-gate
-- convention), nil pick casts nothing, re-arm 10000
-- regardless (C++-exact). Spore striders 38755:
-- 10000+rand32()%5000 init then 10000+rand32()%5000 — timer
-- bookkeeping only: the whole fire arm is IsHeroic()-gated
-- and no difficulty bridge exists (thespia precedent);
-- re-arm regardless (C++-exact). OnDied(4): cleanup (the
-- Striders.DespawnAll arm is summon-bridgeless, steamrigger
-- precedent). OnLeaveCombat(2)/OnReset(23): cancel the pump,
-- drop per-GUID state (the _Reset arm is instance-blocked;
-- the Reset() Striders.DespawnAll arm is summon-bridgeless).
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI shim
-- (instance_the_underbog.cpp stays blocked on the
-- instance-script model); no difficulty bridge — the heroic-
-- only spore-strider summon fire arm unmodeled (timer
-- bookkeeping exact); no ObjectAccessor/cross-creature
-- bridge — the whole LevitatedTarget/LevitatedTarget_Timer/
-- InAir followup machine unmodeled (GetUnit + HasAura +
-- AddAura(SPELL_SUSPENSION 31719) + triggered CastSpell
-- (SPELL_MAGNETIC_PULL 31705) arms all need it; timer
-- bookkeeping for the levitate re-arm itself is exact); no
-- home-position bridge — the check_Timer 60-yd evade arm
-- unmodeled (timer bookkeeping exact; the re-arm from 5000
-- to 1000 is C++-exact); no summon/attack-start bridge —
-- the JustSummoned AttackStart relay and the SummonList
-- Striders tracking/DespawnAll arms unmodeled (steamrigger
-- precedent); SPELL_LEVITATION_PULSE 31701 is an unused enum
-- member — never referenced in the file (gruul unused-enum
-- precedent); the ScriptData "Timers may be incorrect"
-- (SD%Complete: 95) is an upstream caveat, documented, not
-- bridged; no SpellScript/AuraScript scripts in this file.

local SPELL_LEVITATE = 31704
local SPELL_CHAIN_LIGHTNING = 31717
local SPELL_STATIC_CHARGE = 31715

local ENTRY_BLACK_STALKER = 17882

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

-- C++ SelectTarget(Random, 0): random alive player in the
-- instance, no distance gate (thespia precedent). Nil when the
-- instance is empty — the caller skips the cast (C++-exact).
local function pickRandomPlayer(creature)
    local found = playersInInstance(creature)
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
end

-- C++ SelectTarget(Random, 1): random alive player excluding
-- the victim (quagmirran convention). Nil when no other
-- player is present — the caller skips the cast (C++-exact).
local function pickRandomPlayerExcludingVictim(creature)
    local victim = creature:GetVictim()
    local victimGuid = victim and victim:GetGUID() or nil
    local found = {}
    for _, p in ipairs(playersInInstance(creature)) do
        if not victimGuid or p:GetGUID() ~= victimGuid then
            found[#found + 1] = p
        end
    end
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
end

-- C++ SelectTarget(Random, 0, 30, true): random alive player
-- within 30 yd (thespia distance-gate convention). Nil when
-- none is in range — the caller skips the cast (C++-exact).
local function pickRandomPlayerIn30Yd(creature)
    local found = {}
    for _, p in ipairs(playersInInstance(creature)) do
        if creature:GetDistance(p) <= 30.0 then
            found[#found + 1] = p
        end
    end
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
end

-- C++ Initialize() values: Levitate_Timer=12000,
-- ChainLightning_Timer=6000, StaticCharge_Timer=10000,
-- SporeStriders_Timer=10000+rand32()%5000, check_Timer=5000,
-- InAir=false (the C++ JustEngagedWith is empty).
local function freshState()
    return {
        levitate = 12000,
        chainLightning = 6000,
        staticCharge = 10000,
        sporeStriders = 10000 + math.random(0, 4999),
        check = 5000,
    }
end

local function bossTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end

    -- Evade check: 5s init then 1s — timer bookkeeping only:
    -- the fire arm (60-yd home-position distance check ->
    -- EnterEvadeMode) has no GetHomePosition bridge; re-arm
    -- 1000 regardless (C++-exact).
    if st.check <= 1000 then
        st.check = 1000
    else
        st.check = st.check - 1000
    end

    -- Spore striders 38755 (ENTRY_SPORE_STRIDER 22299):
    -- 10000+rand32()%5000 init then 10000+rand32()%5000 —
    -- timer bookkeeping only: the whole fire arm is
    -- IsHeroic()-gated and no difficulty bridge exists
    -- (thespia precedent); re-arm regardless (C++-exact).
    if st.sporeStriders <= 1000 then
        st.sporeStriders = 10000 + math.random(0, 4999)
    else
        st.sporeStriders = st.sporeStriders - 1000
    end

    -- Levitate 31704: 12s init then 12000+rand32()%3000 —
    -- random alive player excluding the victim (quagmirran
    -- convention), nil pick casts nothing, non-triggered cast
    -- on the target (C++-exact), re-arm regardless (the re-arm
    -- is outside the if (target) block, C++-exact). The whole
    -- LevitatedTarget/LevitatedTarget_Timer/InAir followup
    -- machine (SUSPENSION 31719 / MAGNETIC_PULL 31705 arms)
    -- needs an ObjectAccessor/cross-creature bridge — timer
    -- bookkeeping only.
    if st.levitate <= 1000 then
        local target = pickRandomPlayerExcludingVictim(creature)
        if target then
            creature:CastSpell(target, SPELL_LEVITATE)
        end
        st.levitate = 12000 + math.random(0, 2999)
    else
        st.levitate = st.levitate - 1000
    end

    -- Chain lightning 31717: 6s init then 7000 — random alive
    -- player in the instance (thespia precedent), nil pick
    -- casts nothing, re-arm 7000 regardless (C++-exact).
    if st.chainLightning <= 1000 then
        local target = pickRandomPlayer(creature)
        if target then
            creature:CastSpell(target, SPELL_CHAIN_LIGHTNING)
        end
        st.chainLightning = 7000
    else
        st.chainLightning = st.chainLightning - 1000
    end

    -- Static charge 31715: 10s init then 10000 — random alive
    -- player within 30 yd (thespia distance-gate convention),
    -- nil pick casts nothing, re-arm 10000 regardless
    -- (C++-exact).
    if st.staticCharge <= 1000 then
        local target = pickRandomPlayerIn30Yd(creature)
        if target then
            creature:CastSpell(target, SPELL_STATIC_CHARGE)
        end
        st.staticCharge = 10000
    else
        st.staticCharge = st.staticCharge - 1000
    end
end

RegisterCreatureEvent(ENTRY_BLACK_STALKER, 1, function(_, creature)
    local guid = creature:GetGUID()
    resetBoss(guid)
    states[guid] = freshState()
    timers[guid] = CreateLuaEvent(function()
        bossTick(creature, guid)
    end, 1000, 0)
end)

RegisterCreatureEvent(ENTRY_BLACK_STALKER, 2, function(_, creature)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_BLACK_STALKER, 4, function(_, creature)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_BLACK_STALKER, 23, function(_, creature)
    resetBoss(creature:GetGUID())
end)
