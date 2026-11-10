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
-- C++ has a TYPEID_PLAYER gate, modeled via victim:GetObjectType() == "Player"),
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
-- granularity is exact for all C++ timers here). The BossAI::
-- UpdateAI pre-loop + in-loop CASTING gates (ScriptedCreature.
-- cpp:571-589; no boss UpdateAI override) are ported per arm:
-- an expired arm held while casting keeps its timer expired
-- (else-branch countdown only) and fires on the first
-- non-casting tick; unexpired arms keep counting down, since
-- events.Update runs ahead of the gate in C++. Pump:
-- lightning cloud 25033 15s init then {15s,25s} — threat-list
-- player alive within 30 yd (C++ SelectTarget(Random, 0,
-- 30.0f, true), salramm idiom); nil pick casts nothing, timer
-- re-arms (C++-exact); the Heroic second cast is modeled via
-- creature:IsHeroic() (independent random pick, below).
-- Lung burst 31481 7s init then {7s,12s} — threat-list player
-- alive within 40 yd, cast or skip, re-arm (C++-exact).
-- Enveloping winds 31718 9s init then {10s,15s} — threat-list
-- player alive within 35 yd, cast or skip, re-arm (C++-exact);
-- the Heroic second cast is modeled via creature:IsHeroic()
-- (above). OnTargetDied(3): victim:
-- GetObjectType() == "Player" -> Talk(SAY_SLAY) (C++-exact). OnDied(4): Talk(
-- SAY_DEAD) + cleanup (the _JustDied DONE arm is PORTED below:
-- SetBossState + instanceData.thespiaDone flag + the hydro
-- panel clear).
-- OnLeaveCombat(2)/OnReset(23): cancel the pump, drop per-GUID
-- state (the _Reset arm is instance-blocked). Melee is engine-
-- driven.
-- npc_coilfang_waterelemental (17917): OnEnterCombat(1):
-- per-GUID reset (waterBoltVolley {3s,6s}) + 1s pump (a port of
-- the UpdateAI event processing; the !UpdateVictim early return
-- collapses into the pump, which only runs in combat). Pump:
-- water bolt volley 34449 — creature:CastSpell(creature, 34449)
-- non-triggered self-cast (C++ DoCast(me)), re-arm {7s,12s}
-- (C++-exact; the UpdateAI in-loop UNIT_STATE_CASTING gate
-- (:153, after _events.Update, before the ExecuteEvent loop) is
-- ported as a 500ms IsCasting defer, no re-arm consumed).
-- OnDied(4)/OnLeaveCombat(2)/OnReset(23): cleanup.
-- Deliberate deviations (all await engine bridges): the
-- instance-script model is PORTED (instance_steam_vault.lua) —
-- the DATA_HYDROMANCER_THESPIA DONE arm rides ev4 below
-- (SetBossState + instanceData flag + panel clear); the
-- NOT_STARTED/IN_PROGRESS bookkeeping in _Reset/BossAI::
-- JustEngagedWith stays out (no consumer); the DATA_ACCESS_
-- PANEL_HYDRO / main-door sequencing arms are PORTED in
-- instance_steam_vault.lua + go_main_chambers_access_panel.lua;
-- boss admission via the luaBossAI shim for the rest; difficulty is bridged —
-- creature:IsHeroic() (per-instance difficulty, lua_creature_events.go)
-- drives the heroic second casts of lightning cloud (25033) and
-- enveloping winds (31718); cast-state gating PORTED (boss
-- BossAI gates + elemental in-loop gate, live creature:
-- IsCasting bridge, lua_creature_events.go:1950); no
-- SpellScript/AuraScript scripts in this file.

local SPELL_LIGHTNING_CLOUD = 25033
local SPELL_LUNG_BURST = 31481
local SPELL_ENVELOPING_WINDS = 31718
local SPELL_WATER_BOLT_VOLLEY = 34449

local SAY_AGGRO = 1
local SAY_SLAY = 2
local SAY_DEAD = 3

local ENTRY_THESPIA = 17797
local ENTRY_WATER_ELEMENTAL = 17917

-- steam_vault.h SVGameObjectIds; UnitDefines/GameObject field ids.
local GO_ACCESS_PANEL_HYDRO = 184125
local GAMEOBJECT_FLAGS = 9
local GO_FLAG_NOT_SELECTABLE = 16

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

-- C++ SelectTarget draws the threat list (UnitAI.h:217-218): position 0
-- takes any entry, then the player/dist filters apply (salramm idiom).
-- Dead members are offline refs in C++ (skipped at list build) so they
-- are filtered here. The old instance-player pool is retired
-- (threat-list precedent, Oct-25 strips). Nil when none is in range
-- (C++-exact — the caller skips the cast).
local function pickRandomPlayerInRange(creature, dist)
    local found = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:GetObjectType() == "Player" and not t:IsDead()
                and creature:GetDistance(t) - creature:GetCombatReach() - 1.5 < dist then -- IsWithinCombatRange gate (Unit.cpp:583; player reach 1.5, ObjectDefines.h:40)
            found[#found + 1] = t
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

-- Pump CASTING gate: each expired arm carries `and not
-- creature:IsCasting()` — while casting it fires nothing and
-- consumes no re-arm (else-branch countdown only, timer stays
-- expired); unexpired arms keep counting down, since C++
-- events.Update runs ahead of the gate.
local function thespiaTick(creature, guid)
    local st = thespiaState[guid]
    if not st then
        return
    end

    -- Lightning cloud 25033: 15s init then {15s,25s}; nil pick
    -- casts nothing, timer re-arms (C++-exact). Heroic second
    -- cast: independent random pick (C++-exact; IsHeroic is the
    -- per-instance difficulty bridge, lua_creature_events.go).
    if st.lightningCloud <= 1000 and not creature:IsCasting() then
        local target = pickRandomPlayerInRange(creature, 30)
        if target then
            creature:CastSpell(target, SPELL_LIGHTNING_CLOUD)
        end
        if creature:IsHeroic() then
            local heroicTarget = pickRandomPlayerInRange(creature, 30)
            if heroicTarget then
                creature:CastSpell(heroicTarget, SPELL_LIGHTNING_CLOUD)
            end
        end
        st.lightningCloud = 15000 + math.random(0, 10000)
    else
        st.lightningCloud = st.lightningCloud - 1000
    end

    -- Lung burst 31481: 7s init then {7s,12s} (C++-exact).
    if st.lungBurst <= 1000 and not creature:IsCasting() then
        local target = pickRandomPlayerInRange(creature, 40)
        if target then
            creature:CastSpell(target, SPELL_LUNG_BURST)
        end
        st.lungBurst = 7000 + math.random(0, 5000)
    else
        st.lungBurst = st.lungBurst - 1000
    end

    -- Enveloping winds 31718: 9s init then {10s,15s}; the Heroic
    -- second cast uses an independent random pick (C++-exact).
    if st.envelopingWinds <= 1000 and not creature:IsCasting() then
        local target = pickRandomPlayerInRange(creature, 35)
        if target then
            creature:CastSpell(target, SPELL_ENVELOPING_WINDS)
        end
        if creature:IsHeroic() then
            local heroicTarget = pickRandomPlayerInRange(creature, 35)
            if heroicTarget then
                creature:CastSpell(heroicTarget, SPELL_ENVELOPING_WINDS)
            end
        end
        st.envelopingWinds = 10000 + math.random(0, 5000)
    else
        st.envelopingWinds = st.envelopingWinds - 1000
    end
end

local function elementalTick(creature, guid)
    -- UpdateAI in-loop CASTING gate (:153): while casting the
    -- arm defers 500ms, consuming no re-arm (mal_ganis idiom).
    if creature:IsCasting() then
        elementalTimers[guid] = CreateLuaEvent(function()
            elementalTick(creature, guid)
        end, 500, 1)
        return
    end
    -- Water bolt volley 34449: non-triggered self-cast, re-arm
    -- {7s,12s} (C++-exact; header).
    creature:CastSpell(creature, SPELL_WATER_BOLT_VOLLEY)
    elementalTimers[guid] = CreateLuaEvent(function()
        elementalTick(creature, guid)
    end, 7000 + math.random(0, 5000), 1)
end

RegisterCreatureEvent(ENTRY_THESPIA, 1, function(_, creature)
    local guid = creature:GetGUIDLow()
    resetThespia(guid)
    thespiaState[guid] = freshThespiaState()
    creature:Talk(SAY_AGGRO)
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542; sapphiron precedent)
    thespiaTimers[guid] = CreateLuaEvent(function()
        thespiaTick(creature, guid)
    end, 1000, 0)
end)

RegisterCreatureEvent(ENTRY_THESPIA, 2, function(_, creature)
    resetThespia(creature:GetGUIDLow())
end)

RegisterCreatureEvent(ENTRY_THESPIA, 3, function(_, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end)

RegisterCreatureEvent(ENTRY_THESPIA, 4, function(_, creature)
    creature:Talk(SAY_DEAD)
    creature:SetBossState(0, 3) -- DATA_HYDROMANCER_THESPIA, DONE (boss_hydromancer_thespia.cpp: Talk precedes _JustDied)
    -- C++ instance_steam_vault.cpp SetBossState override (:134-141):
    -- DONE -> the hydro access panel loses GO_FLAG_NOT_SELECTABLE.
    -- Go fires no event on boss-state writes, so the panel clears
    -- here in ev4 (C++-exact timing); the main-door gate reads the
    -- instanceData flag below (instance_steam_vault.lua).
    for _, go in ipairs(creature:GetGameObjectsInGrid(GO_ACCESS_PANEL_HYDRO, 1000)) do
        go:RemoveFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE)
    end
    local mapObj = creature:GetMap()
    local data = mapObj and mapObj:GetInstanceData() or nil
    if data then
        data.thespiaDone = true
    end
    resetThespia(creature:GetGUIDLow())
end)

RegisterCreatureEvent(ENTRY_THESPIA, 23, function(_, creature)
    resetThespia(creature:GetGUIDLow())
end)

RegisterCreatureEvent(ENTRY_WATER_ELEMENTAL, 1, function(_, creature)
    local guid = creature:GetGUIDLow()
    resetElemental(guid)
    elementalTimers[guid] = CreateLuaEvent(function()
        elementalTick(creature, guid)
    end, 3000 + math.random(0, 3000), 1)
end)

RegisterCreatureEvent(ENTRY_WATER_ELEMENTAL, 2, function(_, creature)
    resetElemental(creature:GetGUIDLow())
end)

RegisterCreatureEvent(ENTRY_WATER_ELEMENTAL, 4, function(_, creature)
    resetElemental(creature:GetGUIDLow())
end)

RegisterCreatureEvent(ENTRY_WATER_ELEMENTAL, 23, function(_, creature)
    resetElemental(creature:GetGUIDLow())
end)
