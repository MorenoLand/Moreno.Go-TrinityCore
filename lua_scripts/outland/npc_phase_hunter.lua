-- Phase Hunter (Netherstorm) --
-- Lua port of src/server/scripts/Outland/zone_netherstorm.cpp
-- (npc_phase_hunterAI). Zone-script unit after zone_nagrand.cpp
-- per outland_script_loader.cpp order (netherstorm,
-- shadowmoon_valley, terokkar_forest).
-- Entries (PhaseHunterData enum, verifiable from the C++
-- sources): 18879 (NPC_PHASE_HUNTER_ENTRY), 19595
-- (NPC_DRAINED_PHASE_HUNTER_ENTRY). The C++ CreatureScript
-- GetAI returns the same AI class for both entries (no
-- per-entry switch), so both register the same handlers
-- (C++-exact). The creature_template ScriptName bindings are
-- DB-side (no TDB in this workspace).
-- npc_commander_dawnforge and at_commander_dawnforge are
-- documented-only: the whole event machine sits behind the
-- no-bridge arms — cross-creature GUID coordination with
-- Ardonis (19830) via FindNearestCreature, Pathaleon's image
-- (21504) via SummonCreature + the JustSummoned GUID latch,
-- SetFacingToObject / SetStandState kneel choreography, the
-- AreaTrigger start condition (player HasAura(SPELL_SUNFURY_
-- DISGUISE 34603) + quest 10198 incomplete within 30 yd) and
-- the quest credit via AreaExploredOrEventHappens (no cross-
-- creature / summon / movement / stand-state / AreaTrigger /
-- quest-credit bridges; omor / steamrigger / hellfire
-- precedents). Entries 19830 / 19831 / 21504 come from the
-- C++ CreatureEntry array but nothing is registered for
-- them — an unregistered file would be dead code (the
-- luaBossAI shim admits only registered entries).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat,
-- 23 OnReset (the C++ overrides no KilledUnit/JustDied/
-- DamageTaken — the Weak/Drained quest arms are per-tick
-- health checks, C++-exact — so no events 3/4/9; no event
-- 14 — no SpellHit override).
-- Timers via a 1s CreateLuaEvent pump (a port of UpdateAI in
-- C++ arm order — 1s granularity exact for all C++ timers;
-- the !UpdateVictim early return collapses into the pump).
-- C++ DoCast default is triggered=false (vaelastrasz
-- convention): creature:CastSpell(nil, spell) = DoCastVictim,
-- creature:CastSpell(creature, spell) = DoCastSelf.
-- Fight shape (C++-exact for all modeled arms):
-- OnEnterCombat(1): per-GUID reset to the C++ ctor /
-- Reset() values (Weak=false, Materialize=false,
-- Drained=false, ManaBurnTimer 5000+rand32()%3*1000 —
-- 5-8s; the WeakPercent = 25+rand32()%16 init belongs to the
-- unmodeled quest arm, see deviations; the C++ Reset()
-- entry-restore arm (UpdateEntry(18879) when drained) has no
-- entry-update bridge — unmodeled) + 1s scheduler pump; the
-- C++ JustEngagedWith override stores the player GUID, which
-- only feeds the unmodeled quest arms — lands nowhere.
-- Pump (C++ UpdateAI arm order): materialize arm — one-shot
-- on the first tick: non-triggered DoCastSelf(SPELL_
-- MATERIALIZE 34804) (C++-exact; in C++ it fires on the
-- first UpdateAI regardless of combat state — the Lua pump
-- is combat-only, so it fires at the first combat tick, see
-- deviations); mana-burn arm — 5-8s init then 8-18s: if a
-- threat target has mana: non-triggered DoCast(random mana
-- target, SPELL_MANA_BURN 13321) (C++-exact — the C++ picks
-- a random mana-holder from the threat list; the Lua model
-- picks a random alive player in the instance whose power
-- type is mana, thespia + moroes precedents; nil pick casts
-- nothing, C++-exact), re-arm 8000+rand()%10000 regardless
-- (C++-exact); empty list: re-arm 3500 (C++-exact).
-- Melee is engine-driven in Go (creature combat tick), like
-- C++ DoMeleeAttackIfReady.
-- OnLeaveCombat(2)/OnReset(23): cancel the pump, drop
-- per-GUID state (the C++ Reset() observable remainder — the
-- timer re-latch — lands on OnEnterCombat, gargolmar
-- precedent; Eluna's On_Reset fires ahead of OnDied and OnSpawn
-- — millhouse note — so both hooks land the same reset).
-- Deliberate deviations (all await engine bridges): no
-- entry-update bridge — the C++ Reset() drained->full
-- restore arm (me->UpdateEntry(NPC_PHASE_HUNTER_ENTRY)) and
-- the Drained arm's UpdateEntry(NPC_DRAINED_PHASE_HUNTER_
-- ENTRY) + SetHealth pct + LowerPlayerDamageReq +
-- SetInCombatWith unmodeled (nether_drake precedent); no
-- aura-state / unit-state bridge — the phase-slip arm
-- (HasAuraType(SPELL_AURA_MOD_DECREASE_SPEED) or UNIT_STATE_
-- ROOT -> DoCast(me, SPELL_PHASE_SLIP 36574)) unmodeled
-- (netherspite cast-state precedent); no quest bridge — the
-- Weak latch (Talk(EMOTE_WEAK 0) at HealthBelowPct(WeakPct
-- 25-40) while the engaging player's quest 10190 is
-- incomplete) and the whole drained arm sit behind quest-
-- status and HasAura(34219) gates (hellfire quest
-- precedent); Talk(EMOTE_WEAK) fires nowhere in this model
-- (millhouse SAY_ICEBLOCK precedent).
-- Zone set status: zone_netherstorm.cpp closed — commander_
-- dawnforge + at_commander_dawnforge documented-only,
-- npc_phase_hunter ported for both entries.

local SPELL_MATERIALIZE = 34804
local SPELL_MANA_BURN = 13321

local POWER_MANA = 0

local PHASE_HUNTER_ENTRIES = {
    18879,  -- NPC_PHASE_HUNTER_ENTRY
    19595,  -- NPC_DRAINED_PHASE_HUNTER_ENTRY
}

local hunterState = {}
local hunterPump = {}

local function cancelPump(guid)
    local id = hunterPump[guid]
    if id then
        RemoveEventById(id)
        hunterPump[guid] = nil
    end
end

-- C++ Reset(): the observable remainder — the timer re-latch
-- — lands on OnEnterCombat (gargolmar precedent); the
-- entry-restore arm (UpdateEntry) has no bridge.
local function fullReset(guid)
    cancelPump(guid)
    hunterState[guid] = nil
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

-- C++: random mana-holder from the threat list. Lua model: a
-- random alive player in the instance whose power type is
-- mana (thespia select-target + moroes mana-gate
-- precedents).
local function randomManaPlayer(creature)
    local candidates = {}
    for _, p in ipairs(playersInInstance(creature)) do
        if p:GetPowerType() == POWER_MANA then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ UpdateAI in arm order — 1s granularity exact for all
-- C++ timers; the !UpdateVictim early return collapses into
-- the pump (it only runs in combat).
local function combatTick(creature, guid)
    local st = hunterState[guid]
    if not st then
        return
    end

    -- Materialize arm — one-shot on the first tick:
    -- non-triggered DoCastSelf, C++-exact. (In C++ it fires
    -- on the first UpdateAI even out of combat; the Lua pump
    -- is combat-only — deviation documented in the header.)
    if not st.materialized then
        creature:CastSpell(creature, SPELL_MATERIALIZE)
        st.materialized = true
    end

    -- Mana-burn arm — 5-8s init then 8-18s: cast on a random
    -- mana-holder, re-arm regardless (C++-exact); empty
    -- list: re-arm 3500 (C++-exact).
    if st.manaBurnTimer <= 1000 then
        local target = randomManaPlayer(creature)
        if target then
            creature:CastSpell(target, SPELL_MANA_BURN)
            st.manaBurnTimer = 8000 + math.random(0, 9999)
        else
            st.manaBurnTimer = 3500
        end
    else
        st.manaBurnTimer = st.manaBurnTimer - 1000
    end
end

-- C++ ctor / Reset() values land on OnEnterCombat: the C++
-- JustEngagedWith stores the player GUID, which only feeds
-- the unmodeled quest arms — lands nowhere (C++-exact).
local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelPump(guid)
    hunterState[guid] = {
        materialized = false,
        manaBurnTimer = 5000 + math.random(0, 2) * 1000,
    }
    hunterPump[guid] = CreateLuaEvent(function()
        combatTick(creature, guid)
    end, 1000, 0)
end

-- C++ Reset() (called on evade): fullReset — see above.
local function onLeaveCombat(_, creature)
    fullReset(creature:GetGUID())
end

-- The C++ CreatureScript binds one AI class to both entries
-- (no per-entry switch), so both register the same
-- handlers — C++-exact.
for _, entry in ipairs(PHASE_HUNTER_ENTRIES) do
    RegisterCreatureEvent(entry, 1, onEnterCombat)
    RegisterCreatureEvent(entry, 2, onLeaveCombat)
    -- Eluna's On_Reset fires ahead of OnDied and OnSpawn, so
    -- this lands the same Reset() reset as the evade hook.
    RegisterCreatureEvent(entry, 23, onLeaveCombat)
end
