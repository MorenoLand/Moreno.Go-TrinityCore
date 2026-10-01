-- Sironas (Bloodmyst Isle) --
-- Lua port of src/server/scripts/Kalimdor/zone_bloodmyst_isle.cpp
-- (npc_sironasAI). Zone-script unit per kalimdor_script_loader.cpp
-- order (ashenvale, azshara, azuremyst_isle done; bloodmyst_isle
-- now; darkshore, desolace, dustwallow_marsh, silithus, tanaris,
-- the_barrens, winterspring).
-- Entry (EndingTheirWorldMisc enum, verifiable from the C++
-- sources): 17678 (NPC_SIRONAS). The creature_template
-- ScriptName binding is DB-side (no TDB in this workspace).
-- npc_webbed_creature and npc_demolitionist_legoso are
-- documented-only: the webbed creature's whole JustDied arm
-- sits behind the no-summon / quest-credit bridges (and its
-- AI-owner entry is unverifiable in the C++ sources — only
-- NPC_EXPEDITION_RESEARCHER = 17681, the summoned/credit
-- target, appears); the legoso escort event machine sits
-- behind the no-escort / no-quest-accept / no-summon / no-
-- movement / no-gameobject bridges (omor / steamrigger /
-- hellfire precedents).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat,
-- 4 OnDied, 23 OnReset (the C++ overrides no KilledUnit/
-- DamageTaken/SpellHit — no events 3/9/14).
-- Timers via a 1s CreateLuaEvent pump (a port of UpdateAI in
-- C++ arm order — 1s granularity exact for all C++ timers;
-- the !UpdateVictim early return collapses into the pump).
-- C++ DoCast default is triggered=false (vaelastrasz
-- convention): creature:CastSpell(nil, spell) = DoCastVictim.
-- Fight shape (C++-exact for all modeled arms):
-- OnEnterCombat(1): per-GUID reset to the C++ JustEngagedWith
-- values (uppercut 15s, immolate 10s, curse of blood 5s) + 1s
-- scheduler pump; the C++ Reset() display-id arm (SetDisplayId
-- Modelid2) has no display bridge — unmodeled.
-- Pump (C++ UpdateAI arm order): uppercut arm — 15s init
-- then 10-12s: non-triggered DoCastVictim(SPELL_UPPERCUT
-- 10966) (C++-exact), re-arm 10s+rand(2s) regardless;
-- immolate arm — 10s init then 15-20s: non-triggered
-- DoCastVictim(SPELL_IMMOLATE 12742) (C++-exact), re-arm
-- 15s+rand(5s) regardless; curse-of-blood arm — 5s init then
-- 20-25s: non-triggered DoCastVictim(SPELL_CURSE_OF_BLOOD
-- 8282) (C++-exact), re-arm 20s+rand(5s) regardless.
-- Melee is engine-driven in Go (creature combat tick), like
-- C++ DoMeleeAttackIfReady.
-- OnDied(4): the C++ JustDied arms — the scale reset and the
-- cross-creature legoso DoAction(ACTION_LEGOSO_SIRONAS_KILLED)
-- cascade via FindNearestCreature(NPC_LEGOSO) — have no
-- scale / cross-creature bridges, so the hook only drops
-- per-GUID state (illidari_spawn precedent).
-- OnLeaveCombat(2)/OnReset(23): cancel the pump, drop
-- per-GUID state (the C++ Reset() observable remainder — the
-- event-map reset — is latched on OnEnterCombat, gargolmar
-- precedent; Eluna's On_Reset fires ahead of OnDied and
-- OnSpawn — millhouse note — so both hooks land the same
-- reset).
-- Deliberate deviations (all await engine bridges): no
-- cross-creature bridge — the JustDied FindNearestCreature
-- (NPC_LEGOSO 17982) DoAction cascade unmodeled; no DoAction
-- bridge — the Sironas channel arms (ACTION_SIRONAS_CHANNEL_
-- START/STOP: self-cast SPELL_SIRONAS_CHANNELING 31612 plus
-- the Tesla coil 17979 grid-cast of SPELL_BLOODMYST_TESLA
-- 31611 and its interrupt stop) are driven by legoso's escort
-- event machine and have no hook in this model (the engine
-- shim fires events 1/2/3/4/5/9/14/23/26 only — omor
-- precedent); no display / scale bridge — the Reset()
-- SetDisplayId(Modelid2) and JustDied SetObjectScale(1.0f)
-- arms unmodeled (nether_drake precedent).
-- Zone set status: zone_bloodmyst_isle.cpp closed —
-- npc_webbed_creature documented-only (no-summon /
-- quest-credit bridges, unverifiable entry),
-- npc_demolitionist_legoso documented-only (no-escort /
-- quest-accept / summon / movement / gameobject bridges),
-- npc_sironas ported for 17678.

local SPELL_UPPERCUT = 10966
local SPELL_IMMOLATE = 12742
local SPELL_CURSE_OF_BLOOD = 8282

local SIRONAS_ENTRY = 17678

local sironasState = {}
local sironasPump = {}

local function cancelPump(guid)
    local id = sironasPump[guid]
    if id then
        RemoveEventById(id)
        sironasPump[guid] = nil
    end
end

-- C++ Reset() (called on evade): fullReset — the observable
-- remainder (event-map reset) is re-latched on OnEnterCombat
-- (gargolmar precedent).
local function fullReset(guid)
    cancelPump(guid)
    sironasState[guid] = nil
end

-- C++ UpdateAI in arm order — 1s granularity exact for all
-- C++ timers; the !UpdateVictim early return collapses into
-- the pump (it only runs in combat).
local function combatTick(creature, guid)
    local st = sironasState[guid]
    if not st then
        return
    end

    -- Uppercut arm — 15s init then 10-12s: non-triggered
    -- DoCastVictim, re-arm 10s+rand(2s) regardless (C++-exact).
    if st.uppercutTimer <= 1000 then
        creature:CastSpell(nil, SPELL_UPPERCUT)
        st.uppercutTimer = 10000 + math.random(0, 2000)
    else
        st.uppercutTimer = st.uppercutTimer - 1000
    end

    -- Immolate arm — 10s init then 15-20s: non-triggered
    -- DoCastVictim, re-arm 15s+rand(5s) regardless (C++-exact).
    if st.immolateTimer <= 1000 then
        creature:CastSpell(nil, SPELL_IMMOLATE)
        st.immolateTimer = 15000 + math.random(0, 5000)
    else
        st.immolateTimer = st.immolateTimer - 1000
    end

    -- Curse of blood arm — 5s init then 20-25s: non-triggered
    -- DoCastVictim, re-arm 20s+rand(5s) regardless (C++-exact).
    if st.curseOfBloodTimer <= 1000 then
        creature:CastSpell(nil, SPELL_CURSE_OF_BLOOD)
        st.curseOfBloodTimer = 20000 + math.random(0, 5000)
    else
        st.curseOfBloodTimer = st.curseOfBloodTimer - 1000
    end
end

-- C++ JustEngagedWith values land on OnEnterCombat: uppercut
-- 15s, immolate 10s, curse of blood 5s.
local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelPump(guid)
    sironasState[guid] = {
        uppercutTimer = 15000,
        immolateTimer = 10000,
        curseOfBloodTimer = 5000,
    }
    sironasPump[guid] = CreateLuaEvent(function()
        combatTick(creature, guid)
    end, 1000, 0)
end

-- The C++ JustDied arms (scale reset, cross-creature legoso
-- DoAction cascade) have no scale / cross-creature bridges
-- — unmodeled; the hook drops per-GUID state (illidari_spawn
-- precedent).
local function onDied(_, creature)
    fullReset(creature:GetGUID())
end

-- C++ Reset() (called on evade): fullReset — see above.
local function onLeaveCombat(_, creature)
    fullReset(creature:GetGUID())
end

RegisterCreatureEvent(SIRONAS_ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(SIRONAS_ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(SIRONAS_ENTRY, 4, onDied)
-- Eluna's On_Reset fires ahead of OnDied and OnSpawn, so
-- this lands the same Reset() reset as the evade hook.
RegisterCreatureEvent(SIRONAS_ENTRY, 23, onLeaveCombat)
