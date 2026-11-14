-- Sironas (Bloodmyst Isle) --
-- Lua port of src/server/scripts/Kalimdor/zone_bloodmyst_isle.cpp
-- (npc_sironasAI). Zone-script unit per kalimdor_script_loader.cpp
-- order (ashenvale, azshara, azuremyst_isle done; bloodmyst_isle
-- now; darkshore, desolace, dustwallow_marsh, silithus, tanaris,
-- the_barrens, winterspring).
-- Entry (EndingTheirWorldMisc enum, verifiable from the C++
-- sources): 17678 (NPC_SIRONAS). The creature_template
-- ScriptName binding is DB-side (no TDB in this workspace).
-- npc_webbed_creature is documented-only: the webbed
-- creature's whole JustDied arm sits behind the no-summon /
-- quest-credit bridges (and its AI-owner entry is
-- unverifiable in the C++ sources — only
-- NPC_EXPEDITION_RESEARCHER = 17681, the summoned/credit
-- target, appears). The legoso escort event machine is
-- PORTED (npc_demolitionist_legoso.lua) — the no-escort /
-- no-quest-accept / no-summon / no-movement / no-gameobject
-- bridge claims from the 2026-11-05 port are stale.
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
-- scheduler pump; the C++ Reset() display-id arm is PORTED in
-- onLeaveCombat below (SetDisplayId(Modelid2) via
-- creature:GetTemplateModelId(2), bridge live 2026-10-22).
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
-- OnDied(4): the C++ JustDied arms — the scale reset
-- (SetScale(1.0), live now the legoso 3.0f flip is ported)
-- and the cross-creature legoso DoAction
-- (ACTION_LEGOSO_SIRONAS_KILLED) cascade via
-- FindNearestCreature(NPC_LEGOSO) — the cascade is PORTED
-- (ev38 bridge; killer gates: legoso himself, a
-- loot-recipient-group member, or the event-starter GUID
-- counter via legoso GetData(0) / killer GetGUIDLow()).
-- OnLeaveCombat(2)/OnReset(23): cancel the pump, drop
-- per-GUID state (the C++ Reset() observable remainder — the
-- event-map reset — is latched on OnEnterCombat, gargolmar
-- precedent; Eluna's On_Reset fires ahead of OnDied and
-- OnSpawn — millhouse note — so both hooks land the same
-- reset).
-- Deliberate deviations: the Sironas channel arms
-- (ACTION_SIRONAS_CHANNEL_START/STOP) are PORTED via the
-- ev38 DoAction handler below (called from legoso's escort
-- machine); Go InterruptNonMeleeSpells takes no spell arg —
-- the stop arm drops all non-melee casts, the only casts in
-- play. The Modelid2-bearer bridge (GetTemplateModelId, live
-- 2026-10-22) lands the Reset() SetDisplayId(Modelid2) arm in
-- onLeaveCombat below (nether_drake precedent updated).
-- Zone set status: zone_bloodmyst_isle.cpp closed —
-- npc_webbed_creature documented-only (no-summon /
-- quest-credit bridges, unverifiable entry),
-- npc_demolitionist_legoso ported for 17982 (combat rotation
-- + escort event machine), npc_sironas ported for 17678.

local SPELL_UPPERCUT = 10966
local SPELL_IMMOLATE = 12742
local SPELL_CURSE_OF_BLOOD = 8282
local SPELL_SIRONAS_CHANNELING = 31612
local SPELL_BLOODMYST_TESLA = 31611

local SIRONAS_ENTRY = 17678
local LEGOSO_ENTRY = 17982
local NPC_BLOODMYST_TESLA_COIL = 17979

local ACTION_SIRONAS_CHANNEL_START = 1
local ACTION_SIRONAS_CHANNEL_STOP = 2
local ACTION_LEGOSO_SIRONAS_KILLED = 1

local DATA_EVENT_STARTER_GUID = 0

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
    local guid = creature:GetGUIDLow()
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

-- C++ JustDied (zone_bloodmyst_isle.cpp:263-280): the scale
-- reset is PORTED (creature:SetScale(1.0) — the legoso 3.0f
-- flip rides the ported escort machine now); the legoso
-- DoAction (ACTION_LEGOSO_SIRONAS_KILLED) cascade is PORTED
-- 2026-11-05 via the ev38 DoAction bridge: killer nil-guard
-- first, then FindNearestCreature(NPC_LEGOSO) (the C++ SIZE_OF_GRIDS grid
-- twin), then the killer gates — killer is legoso himself, or a
-- member of the loot-recipient group (GetLootRecipientGroup /
-- group:IsMember, both bridged), or the killer GUID counter
-- equals legoso GetData(DATA_EVENT_STARTER_GUID) (the starter
-- GUID is set by legoso's ported OnQuestAccept; GetGUIDLow is
-- the C++ GetCounter() twin). The hook drops per-GUID state
-- (illidari_spawn precedent) either way.
local function onDied(_, creature, killer)
    creature:SetScale(1.0)
    fullReset(creature:GetGUIDLow())
    if not killer then
        return
    end
    local legoso = creature:FindNearestCreature(LEGOSO_ENTRY, 533.33)
    if not legoso then
        return
    end
    local killerGuid = killer:GetGUID()
    local relay = killerGuid == legoso:GetGUID()
    if not relay then
        local group = creature:GetLootRecipientGroup()
        if group then
            relay = group:IsMember(killerGuid)
        end
    end
    if not relay then
        relay = killer:GetGUIDLow() == legoso:GetData(DATA_EVENT_STARTER_GUID)
    end
    if relay then
        legoso:DoAction(ACTION_LEGOSO_SIRONAS_KILLED)
    end
end

-- C++ Reset() (called on evade): fullReset — see above.
local function onLeaveCombat(_, creature)
    fullReset(creature:GetGUIDLow())
    -- C++ npc_sironasAI::Reset() (zone_bloodmyst_isle.cpp:250-254):
    -- _events.Reset() then SetDisplayId(Modelid2) — ported
    -- 2026-10-22 via creature:GetTemplateModelId(2).
    creature:SetDisplayId(creature:GetTemplateModelId(2))
end

-- C++ DoAction (zone_bloodmyst_isle.cpp:313-344): CHANNEL_START
-- self-casts SPELL_SIRONAS_CHANNELING (31612) and every tesla
-- coil (17979) in the grid casts SPELL_BLOODMYST_TESLA (31611)
-- on itself (GetCreaturesInGrid twin of
-- GetCreatureListWithEntryInGrid); CHANNEL_STOP interrupts the
-- channel on sironas and the tesla cast on every coil in range
-- (Go InterruptNonMeleeSpells takes no spell arg — it drops all
-- non-melee casts, the only casts in play here). Called from
-- legoso's escort machine (WaypointReached 33 / detonate phase).
local function onDoAction(event, creature, action)
    if action == ACTION_SIRONAS_CHANNEL_START then
        creature:CastSpell(creature, SPELL_SIRONAS_CHANNELING)
        for _, coil in ipairs(creature:GetCreaturesInGrid(NPC_BLOODMYST_TESLA_COIL, 533.33)) do
            coil:CastSpell(coil, SPELL_BLOODMYST_TESLA)
        end
    elseif action == ACTION_SIRONAS_CHANNEL_STOP then
        creature:InterruptNonMeleeSpells()
        for _, coil in ipairs(creature:GetCreaturesInGrid(NPC_BLOODMYST_TESLA_COIL, 533.33)) do
            coil:InterruptNonMeleeSpells()
        end
    end
end

RegisterCreatureEvent(SIRONAS_ENTRY, 38, onDoAction)
RegisterCreatureEvent(SIRONAS_ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(SIRONAS_ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(SIRONAS_ENTRY, 4, onDied)
-- Eluna's On_Reset fires ahead of OnDied and OnSpawn, so
-- this lands the same Reset() reset as the evade hook.
RegisterCreatureEvent(SIRONAS_ENTRY, 23, onLeaveCombat)
