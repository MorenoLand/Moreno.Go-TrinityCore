-- Hydromancer Thespia (The Steamvault) — Lua port of
-- src/server/scripts/Outland/CoilfangReservoir/SteamVault/
-- boss_hydromancer_thespia.cpp (boss_hydromancer_thespia,
-- npc_coilfang_waterelemental — the two AI classes AddSC_boss_
-- hydromancer_thespia registers; no other creature or game
-- object scripts in this file). First boss in the Steam Vault
-- set, the next untouched zone per outland_script_loader.cpp
-- order after Serpentshrine Cavern.
-- Entry 17797 verified from the C++ sources (steam_vault.h:
-- NPC_HYDROMANCER_THESPIA = 17797; instance_steam_vault.cpp
-- maps it to DATA_HYDROMANCER_THESPIA = 0); entry 17917 for
-- the Coilfang Water Elemental verified externally (wowhead
-- npc=17917 — Coilfang Water Elemental, gruul precedent). The
-- creature_template ScriptName bindings are DB-side (no TDB in
-- this workspace).
-- Talk lines used: SAY_AGGRO=1 (pull), SAY_SLAY=2 (kill — the
-- C++ has a TYPEID_PLAYER gate, modeled via victim:IsPlayer()),
-- SAY_DEAD=3 (death); SAY_SUMMON=0 is defined but never called
-- anywhere in the file (unused enum member, documented only,
-- gruul precedent).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent
-- (per-GUID scheduler pump); melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady. C++
-- DoCast default is triggered=false (vaelastrasz convention).
-- Fight shape (C++-exact for the modeled arms):
-- boss_hydromancer_thespia (17797): OnEnterCombat(1):
-- per-GUID scheduler reset (lightningCloud 15000 / lungBurst
-- 7000 / envelopingWinds 9000) + Talk(SAY_AGGRO) + 1s
-- scheduler pump (a port of boss_thespiaAI::ExecuteEvent — 1s
-- granularity is exact for all C++ timers here). Pump:
-- lightning cloud 25033 15s init then {15s,25s} — random alive
-- player in the instance within 30 yd (C++ SelectTarget(Random,
-- 0, 30.0f, true), netherspite distance-gate convention); nil
-- pick casts nothing, timer re-arms (C++-exact); the Heroic
-- second cast has no difficulty bridge — unmodeled (below).
-- Lung burst 31481 7s init then {7s,12s} — random alive player
-- within 40 yd, cast or skip, re-arm (C++-exact). Enveloping
-- winds 31718 9s init then {10s,15s} — random alive player
-- within 35 yd, cast or skip, re-arm (C++-exact); the Heroic
-- second cast unmodeled (below). OnTargetDied(3): victim:
-- IsPlayer() -> Talk(SAY_SLAY) (C++-exact). OnDied(4): Talk(
-- SAY_DEAD) + cleanup (the _JustDied arm is instance-blocked).
-- OnLeaveCombat(2)/OnReset(23): cancel the pump, drop per-GUID
-- state (the _Reset arm is instance-blocked). Melee is engine-
-- driven.
-- npc_coilfang_waterelemental (17917): OnEnterCombat(1):
-- per-GUID reset (waterBoltVolley {3s,6s}) + 1s pump (a port of
-- the UpdateAI event processing; the !UpdateVictim early return
-- collapses into the pump, which only runs in combat). Pump:
-- water bolt volley 34449 — creature:CastSpell(nil, 34449)
-- non-triggered self-cast (C++ DoCast(me)), re-arm {7s,12s}
-- (C++-exact; the UNIT_STATE_CASTING early return has no unit-
-- state bridge, so the event fires even on the pump tick a C++
-- cast would have skipped — the timer bookkeeping is C++-exact).
-- OnDied(4)/OnLeaveCombat(2)/OnReset(23): cleanup.
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI shim
-- (the DATA_HYDROMANCER_THESPIA NOT_STARTED/IN_PROGRESS/DONE
-- bookkeeping in _Reset/_JustDied/BossAI::JustEngagedWith
-- skipped; the DATA_ACCESS_PANEL_HYDRO / main-door sequencing
-- arms live in instance_steam_vault.cpp, which stays blocked on
-- the instance-script model); no difficulty bridge — the
-- IsHeroic() second casts of lightning cloud (25033) and
-- enveloping winds (31718) are unmodeled; no unit-state bridge —
-- the elemental's UNIT_STATE_CASTING skip arm unmodeled (timer
-- bookkeeping exact); no SpellScript/AuraScript scripts in this
-- file.

local SPELL_LIGHTNING_CLOUD = 25033
local SPELL_LUNG_BURST = 31481
local SPELL_ENVELOPING_WINDS = 31718
local SPELL_WATER_BOLT_VOLLEY = 34449

local SAY_AGGRO = 1
local SAY_SLAY = 2
local SAY_DEAD = 3

local ENTRY_THESPIA = 17797
local ENTRY_WATER_ELEMENTAL = 17917

local thespiaTimers = {}
local thespiaState = {}
local elementalTimers = {}

local function cancelPump(timers, guid)
    local id = timers[guid]
    if id then
        RemoveEventById(id)
        timers[guid] = nil
    end
end

local function resetThespia(guid)
    cancelPump(thespiaTimers, guid)
    thespiaState[guid] = nil
end

local function resetElemental(guid)
    cancelPump(elementalTimers, guid)
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

-- C++ SelectTarget(Random, 0, dist, true): random alive player
-- in the instance within dist yards. Nil when none is in range
-- (C++-exact — the caller skips the cast).
local function pickRandomPlayerInRange(creature, dist)
    local found = {}
    for _, p in ipairs(playersInInstance(creature)) do
        if creature:GetDistance(p) <= dist then
            found[#found + 1] = p
        end
    end
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
end

-- C++ JustEngagedWith arms: lightningCloud 15000, lungBurst
-- 7000, envelopingWinds 9000 (event scheduler init values).
local function freshThespiaState()
    return {
        lightningCloud = 15000,
        lungBurst = 7000,
        envelopingWinds = 9000,
    }
end

local function thespiaTick(creature, guid)
    local st = thespiaState[guid]
    if not st then
        return
    end

    -- Lightning cloud 25033: 15s init then {15s,25s}; nil pick
    -- casts nothing, timer re-arms (C++-exact). The Heroic
    -- second cast has no difficulty bridge — unmodeled (header).
    if st.lightningCloud <= 1000 then
        local target = pickRandomPlayerInRange(creature, 30)
        if target then
            creature:CastSpell(target, SPELL_LIGHTNING_CLOUD)
        end
        st.lightningCloud = 15000 + math.random(0, 10000)
    else
        st.lightningCloud = st.lightningCloud - 1000
    end

    -- Lung burst 31481: 7s init then {7s,12s} (C++-exact).
    if st.lungBurst <= 1000 then
        local target = pickRandomPlayerInRange(creature, 40)
        if target then
            creature:CastSpell(target, SPELL_LUNG_BURST)
        end
        st.lungBurst = 7000 + math.random(0, 5000)
    else
        st.lungBurst = st.lungBurst - 1000
    end

    -- Enveloping winds 31718: 9s init then {10s,15s}; the Heroic
    -- second cast unmodeled (header).
    if st.envelopingWinds <= 1000 then
        local target = pickRandomPlayerInRange(creature, 35)
        if target then
            creature:CastSpell(target, SPELL_ENVELOPING_WINDS)
        end
        st.envelopingWinds = 10000 + math.random(0, 5000)
    else
        st.envelopingWinds = st.envelopingWinds - 1000
    end
end

local function elementalTick(creature, guid)
    -- Water bolt volley 34449: non-triggered self-cast, re-arm
    -- {7s,12s} (C++-exact; header).
    creature:CastSpell(nil, SPELL_WATER_BOLT_VOLLEY)
    elementalTimers[guid] = CreateLuaEvent(function()
        elementalTick(creature, guid)
    end, 7000 + math.random(0, 5000), 1)
end

RegisterCreatureEvent(ENTRY_THESPIA, 1, function(_, creature)
    local guid = creature:GetGUID()
    resetThespia(guid)
    thespiaState[guid] = freshThespiaState()
    creature:Talk(SAY_AGGRO)
    thespiaTimers[guid] = CreateLuaEvent(function()
        thespiaTick(creature, guid)
    end, 1000, 0)
end)

RegisterCreatureEvent(ENTRY_THESPIA, 2, function(_, creature)
    resetThespia(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_THESPIA, 3, function(_, creature, victim)
    if victim and victim:IsPlayer() then
        creature:Talk(SAY_SLAY)
    end
end)

RegisterCreatureEvent(ENTRY_THESPIA, 4, function(_, creature)
    creature:Talk(SAY_DEAD)
    resetThespia(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_THESPIA, 23, function(_, creature)
    resetThespia(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_WATER_ELEMENTAL, 1, function(_, creature)
    local guid = creature:GetGUID()
    resetElemental(guid)
    elementalTimers[guid] = CreateLuaEvent(function()
        elementalTick(creature, guid)
    end, 3000 + math.random(0, 3000), 1)
end)

RegisterCreatureEvent(ENTRY_WATER_ELEMENTAL, 2, function(_, creature)
    resetElemental(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_WATER_ELEMENTAL, 4, function(_, creature)
    resetElemental(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_WATER_ELEMENTAL, 23, function(_, creature)
    resetElemental(creature:GetGUID())
end)
