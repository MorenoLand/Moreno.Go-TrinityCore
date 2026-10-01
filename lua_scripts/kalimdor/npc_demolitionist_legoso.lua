-- Demolitionist Legoso (Bloodmyst Isle) --
-- Lua port of src/server/scripts/Kalimdor/zone_bloodmyst_isle.cpp
-- (npc_demolitionist_legosoAI — combat rotation only; the
-- 40-phase escort event machine is documented-only, see
-- deviations). Zone-script unit per kalimdor_script_loader.cpp
-- order (ashenvale, azshara, azuremyst_isle closed; bloodmyst_
-- isle: npc_sironas ported, webbed creature documented-only).
-- Entry (EndingTheirWorldMisc enum, verifiable from the C++
-- sources): 17982 (NPC_LEGOSO). The creature_template
-- ScriptName binding is DB-side (no TDB in this workspace).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat,
-- 4 OnDied, 23 OnReset (the C++ overrides no KilledUnit/
-- DamageTaken/SpellHit — no events 3/9/14).
-- Timers via a 1s CreateLuaEvent pump (a port of UpdateAI's
-- combat section in C++ arm order — 1s granularity exact
-- for all C++ timers; the !UpdateVictim early return
-- collapses into the pump, which only runs in combat).
-- C++ DoCast default is triggered=false (vaelastrasz
-- convention): creature:CastSpell(nil, spell) = DoCastVictim.
-- Fight shape (C++-exact for all modeled arms):
-- OnEnterCombat(1): per-GUID reset to the C++ Reset()
-- values (frost shock 1s, healing surge 5s, searing totem
-- 15s, strength of earth totem 20s) + 1s scheduler pump;
-- the C++ Reset() SetCanDualWield arm has no dual-wield
-- bridge — unmodeled. Each pump tick decrements every
-- timer by 1000 (the C++ EventMap::Update(diff) pass),
-- then fires expired events in C++ arm order; a fired
-- frost/to-tem event pushes the other armed timers out by
-- 1s before they are checked (C++ EventMap::DelayEvents).
-- Pump (C++ UpdateAI arm order): frost-shock arm — 1s init
-- then 10-15s: non-triggered DoCastVictim(SPELL_FROST_SHOCK
-- 8056) (C++-exact), DelayEvents(1s), re-arm 10s+rand(5s);
-- searing-totem arm — 15s init then 110-130s: non-triggered
-- DoCast(me, SPELL_SEARING_TOTEM 38116) (C++-exact),
-- DelayEvents(1s), re-arm 110s+rand(20s); strength-of-earth-
-- totem arm — 20s init then 110-130s: non-triggered
-- DoCast(me, SPELL_STRENGTH_OF_EARTH_TOTEM 31633)
-- (C++-exact), DelayEvents(1s), re-arm 110s+rand(20s);
-- healing-surge arm — 5s init: strict self-health below 85%
-- (GetHealthPct() < 85, C++-exact) -> non-triggered
-- DoCast(self, SPELL_HEALING_SURGE 8004) (C++-exact),
-- re-arm 10s; else re-arm 2s (C++-exact; no DelayEvents
-- on the healing arm). Melee is engine-driven.
-- OnLeaveCombat(2)/OnReset(23): cancel the pump, drop
-- per-GUID state (the C++ Reset() observable remainder —
-- the event-map reset — is latched on OnEnterCombat,
-- gargolmar precedent; Eluna's On_Reset fires ahead of
-- OnDied and OnSpawn — millhouse note — so both hooks
-- land the same reset).
-- Deliberate deviations (all await engine bridges): no
-- escort / quest-accept / movement / summon / gameobject
-- bridges — the whole 40-phase escort event machine
-- (OnQuestAccept quest 9759 Start, the WaypointReached
-- kneel/plant/detonate choreography with draenei-explosives
-- gameobjects, the sironas-meeting phases, the post-slay
-- quest-credit) is unmodeled (omor / steamrigger
-- precedents); the escort-player healing-surge arm
-- (GetPlayerForEscort() below 85%) is unmodeled — no
-- escort bridge, so no escort player exists in this
-- model (C++-exact when GetPlayerForEscort is nil);
-- no DoAction bridge — ACTION_LEGOSO_SIRONAS_KILLED
-- (the sironas-slain speech cascade) is unmodeled;
-- no dual-wield bridge — Reset() SetCanDualWield(true)
-- unmodeled; no talk bridge — no SAY_LEGOSO_* line fires
-- in this model (sironas precedent).
-- Zone set status: zone_bloodmyst_isle.cpp closed —
-- npc_webbed_creature documented-only (no-summon /
-- quest-credit bridges, unverifiable entry),
-- npc_sironas ported for 17678,
-- npc_demolitionist_legoso combat rotation ported for
-- 17982; escort event machine documented-only.

local SPELL_FROST_SHOCK = 8056
local SPELL_HEALING_SURGE = 8004
local SPELL_SEARING_TOTEM = 38116
local SPELL_STRENGTH_OF_EARTH_TOTEM = 31633

local LEGOSO_ENTRY = 17982

local legosoState = {}
local legosoPump = {}

local TIMER_KEYS = { "frostTimer", "searingTimer", "strengthTimer", "healingTimer" }

local function cancelPump(guid)
    local id = legosoPump[guid]
    if id then
        RemoveEventById(id)
        legosoPump[guid] = nil
    end
end

-- C++ Reset() (called on evade): fullReset — the observable
-- remainder (event-map reset) is re-latched on OnEnterCombat
-- (gargolmar precedent).
local function fullReset(guid)
    cancelPump(guid)
    legosoState[guid] = nil
end

-- C++ EventMap::DelayEvents(1s): push every other armed
-- timer out by 1000ms, preserving remaining time.
local function delayOthers(st, except)
    for _, key in ipairs(TIMER_KEYS) do
        if key ~= except then
            st[key] = st[key] + 1000
        end
    end
end

-- C++ UpdateAI combat section in arm order — 1s granularity
-- exact for all C++ timers; the !UpdateVictim early return
-- collapses into the pump (it only runs in combat). Each
-- tick decrements every timer (the EventMap::Update(diff)
-- pass), then expired events fire in arm order with their
-- DelayEvents applied before the next event is checked.
local function combatTick(creature, guid)
    local st = legosoState[guid]
    if not st then
        return
    end

    for _, key in ipairs(TIMER_KEYS) do
        st[key] = st[key] - 1000
    end

    -- Frost shock arm — 1s init then 10-15s: non-triggered
    -- DoCastVictim, DelayEvents(1s), re-arm 10s+rand(5s)
    -- regardless (C++-exact).
    if st.frostTimer <= 0 then
        creature:CastSpell(nil, SPELL_FROST_SHOCK)
        delayOthers(st, "frostTimer")
        st.frostTimer = 10000 + math.random(0, 5000)
    end

    -- Searing totem arm — 15s init then 110-130s: non-
    -- triggered DoCast(self), DelayEvents(1s), re-arm
    -- 110s+rand(20s) regardless (C++-exact).
    if st.searingTimer <= 0 then
        creature:CastSpell(creature, SPELL_SEARING_TOTEM)
        delayOthers(st, "searingTimer")
        st.searingTimer = 110000 + math.random(0, 20000)
    end

    -- Strength of earth totem arm — 20s init then 110-130s:
    -- non-triggered DoCast(self), DelayEvents(1s), re-arm
    -- 110s+rand(20s) regardless (C++-exact).
    if st.strengthTimer <= 0 then
        creature:CastSpell(creature, SPELL_STRENGTH_OF_EARTH_TOTEM)
        delayOthers(st, "strengthTimer")
        st.strengthTimer = 110000 + math.random(0, 20000)
    end

    -- Healing surge arm — 5s init, no DelayEvents: strict
    -- self-health below 85% -> non-triggered DoCast(self),
    -- re-arm 10s; else re-arm 2s (C++-exact). The escort-
    -- player arm has no escort bridge — unmodeled.
    if st.healingTimer <= 0 then
        if creature:GetHealthPct() < 85 then
            creature:CastSpell(creature, SPELL_HEALING_SURGE)
            st.healingTimer = 10000
        else
            st.healingTimer = 2000
        end
    end
end

-- C++ Reset() values land on OnEnterCombat: frost shock 1s,
-- healing surge 5s, searing totem 15s, strength of earth
-- totem 20s.
local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelPump(guid)
    legosoState[guid] = {
        frostTimer = 1000,
        healingTimer = 5000,
        searingTimer = 15000,
        strengthTimer = 20000,
    }
    legosoPump[guid] = CreateLuaEvent(function()
        combatTick(creature, guid)
    end, 1000, 0)
end

-- The C++ JustDied override is empty; the hook drops
-- per-GUID state like the other cleanup hooks.
local function onDied(_, creature)
    fullReset(creature:GetGUID())
end

-- C++ Reset() (called on evade): fullReset — see above.
local function onLeaveCombat(_, creature)
    fullReset(creature:GetGUID())
end

RegisterCreatureEvent(LEGOSO_ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(LEGOSO_ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(LEGOSO_ENTRY, 4, onDied)
-- Eluna's On_Reset fires ahead of OnDied and OnSpawn, so
-- this lands the same Reset() reset as the evade hook.
RegisterCreatureEvent(LEGOSO_ENTRY, 23, onLeaveCombat)
