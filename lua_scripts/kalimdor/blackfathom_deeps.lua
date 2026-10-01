-- Blackfathom Deeps: dungeon scripts (Aku'mai event trash + Altar of the Deeps)
-- Lua port of src/server/scripts/Kalimdor/BlackfathomDeeps/blackfathom_deeps.cpp
-- (go_blackfathom_altar / go_blackfathom_fire GameObjectScripts;
-- npc_blackfathom_deeps_event : public ScriptedAI (per-entry switch);
-- npc_morridune : public EscortAI; AddSC_blackfathom_deeps at end;
-- kalimdor loader decl/call per kalimdor_script_loader.cpp).
-- Whole-server-tree quoted-name grep confirms blackfathom_deeps.cpp as the
-- sole source of all four script names (loader lines only otherwise).
-- Entries (blackfathom_deeps.h BFDCreatureIds/BFDGameObjectIds enums):
-- NPC_AKU_MAI_SNAPJAW = 4825 / NPC_AKU_MAI_SERVANT = 4978 — corroborated
-- by a second C++ source: instance_blackfathom_deeps.cpp's SetData(DATA_FIRE)
-- arms summon these named constants (case 1 -> Snapjaw x4, case 3 -> Servant
-- x2), so the name-to-entry tie is C++-verified (kalecgos entry-
-- verifiability check, ramstein strength); the creature_template ScriptName
-- binding stays DB-side (no TDB in this workspace).
-- GO_ALTAR_OF_THE_DEEPS = 103016 for go_blackfathom_altar: header enum plus
-- instance OnGameObjectCreate/SetBossState arms (NOT_SELECTABLE gate until
-- DATA_AKU_MAI DONE); the ScriptName->entry tie is name-inferential
-- (altar = ALTAR_OF_THE_DEEPS), documented per the hexlord-adds precedent.
-- go_blackfathom_fire's four braziers (GO_FIRE_OF_AKU_MAI_1..4 =
-- 21118-21121) stay unregistered (see below).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Driven by CreateLuaEvent timers; melee is engine-driven in
-- Go (creature combat tick), like C++ DoMeleeAttackIfReady. There is no
-- UNIT_STATE_CASTING gate in Go, so timers fire unconditionally (maiden
-- precedent). The C++ EnterEvadeMode _events.Reset() leg collapses into
-- the 2/4/23 cancels.
-- Verifiable numbers (file's own enums): SPELL_BLESSING_OF_BLACKFATHOM =
-- 8733 / SPELL_RAVAGE = 8391 / SPELL_FROST_NOVA = 865 / SPELL_FROST_BOLT_
-- VOLLEY = 8398 / SPELL_TELEPORT_DARNASSUS = 9268.
-- Ported arms:
-- - npc_blackfathom_deeps_event, entry 4825 (JustEngagedWith switch arm):
--   Ravage 8391 DoCastVictim, init {5s,8s} -> repeat {9s,14s} (C++-exact;
--   jeklik GetVictim + CastSpell convention, non-triggered).
-- - npc_blackfathom_deeps_event, entry 4978 (JustEngagedWith switch arm):
--   Frostbolt Volley 8398 on a random alive player (SelectTarget Random,0
--   -> janalai randomPlayerInRange convention), init {2s,4s} -> repeat
--   {5s,8s} (C++-exact, non-triggered). The Frost Nova 865 DoCastAOE arm
--   has no bridge (arugal precedent) — skipped.
-- - go_blackfathom_altar (GO 103016), GossipHello(1): if the player lacks
--   aura 8733, player:AddAura(8733) (kalecgos spectral-rift GO gossip +
--   netherspite player:HasAura/AddAura precedent); returns true.
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - JustDied's me->IsSummon() -> _instance->SetData(DATA_EVENT, +1) leg:
--   instance-script model blocked (standing).
-- - IsSummonedBy -> DoZoneInCombat(): no zone-in-combat bridge.
-- - DamageTaken's 15%-hp flee arm (entries 4977 Murkshallow Softshell /
--   4823 Barbed Crustacean, not registered): DoFleeToGetAssistance has no
--   movement bridge (av_marshal precedent); their only other C++ arm is
--   engine-driven melee.
-- - GetBlackfathomDeepsAI -> GetInstanceAI leg: instance-script model
--   blocked (standing).
-- - go_blackfathom_fire (GO 21118-21121): OnGossipHello's core arm is
--   instance->SetData(DATA_FIRE, GetData(DATA_FIRE) + 1), which drives the
--   instance's summon waves — instance model blocked; registering a gossip
--   hook whose SetGoState/NOT_SELECTABLE arms are purely cosmetic would be
--   a firesworn stub (go_keystone_chamber / go_altar_of_archaedas
--   precedent) — DOCUMENTED, not registered.
-- - npc_morridune (entry 6729): the whole choreography is EscortAI
--   waypoint work (Reset Talk(0) + Start, WaypointReached(4) -> pause +
--   SetFacingTo + UNIT_NPC_FLAG_GOSSIP + Talk(1)) — no movement/waypoint
--   bridge; the gossip flag is only set at escort waypoint 4, so a
--   spawn-time gossip hook would fire at the wrong time (harrison
--   precedent) — DOCUMENTED, not registered. The teleport DoCast(player,
--   9268) arm is unreachable without the escort.

local ENTRY_AKU_MAI_SNAPJAW = 4825
local ENTRY_AKU_MAI_SERVANT = 4978
local ENTRY_ALTAR_OF_THE_DEEPS = 103016

local SPELL_RAVAGE = 8391
local SPELL_FROST_BOLT_VOLLEY = 8398
local SPELL_BLESSING_OF_BLACKFATHOM = 8733

local timers = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

local function schedule(guid, key, delay, fn)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    per[key] = CreateLuaEvent(fn, delay)
end

local function randomPlayerInRange(creature, maxDist)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() and creature:GetDistance(p) <= maxDist then
            found[#found + 1] = p
        end
    end
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
end

-- npc_blackfathom_deeps_event: 4825 Aku'mai Snapjaw — Ravage (C++-exact
-- timers, non-triggered DoCastVictim -> GetVictim + CastSpell).
local function onRavage(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_RAVAGE)
    end
    schedule(guid, "ravage", math.random(9000, 14000), function()
        onRavage(creature, guid)
    end)
end

-- npc_blackfathom_deeps_event: 4978 Aku'mai Servant — Frostbolt Volley on
-- a random alive player (C++-exact timers, non-triggered).
local function onFrostboltVolley(creature, guid)
    local target = randomPlayerInRange(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_FROST_BOLT_VOLLEY)
    end
    schedule(guid, "frostboltvolley", math.random(5000, 8000), function()
        onFrostboltVolley(creature, guid)
    end)
end

local function onEventEnterCombat(_, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    local entry = creature:GetEntry()
    if entry == ENTRY_AKU_MAI_SNAPJAW then
        schedule(guid, "ravage", math.random(5000, 8000), function()
            onRavage(creature, guid)
        end)
    elseif entry == ENTRY_AKU_MAI_SERVANT then
        schedule(guid, "frostboltvolley", math.random(2000, 4000), function()
            onFrostboltVolley(creature, guid)
        end)
    end
end

local function onEventCombatEnd(_, creature)
    cancelTimers(creature:GetGUIDLow())
end

RegisterCreatureEvent(ENTRY_AKU_MAI_SNAPJAW, 1, onEventEnterCombat)
RegisterCreatureEvent(ENTRY_AKU_MAI_SNAPJAW, 2, onEventCombatEnd)
RegisterCreatureEvent(ENTRY_AKU_MAI_SNAPJAW, 4, onEventCombatEnd)
RegisterCreatureEvent(ENTRY_AKU_MAI_SNAPJAW, 23, onEventCombatEnd)
RegisterCreatureEvent(ENTRY_AKU_MAI_SERVANT, 1, onEventEnterCombat)
RegisterCreatureEvent(ENTRY_AKU_MAI_SERVANT, 2, onEventCombatEnd)
RegisterCreatureEvent(ENTRY_AKU_MAI_SERVANT, 4, onEventCombatEnd)
RegisterCreatureEvent(ENTRY_AKU_MAI_SERVANT, 23, onEventCombatEnd)

-- go_blackfathom_altar (GO 103016): GossipHello grants Blessing of
-- Blackfathom 8733 to players lacking the aura (C++-exact gate).
local function onAltarGossip(_, player)
    if not player:HasAura(SPELL_BLESSING_OF_BLACKFATHOM) then
        player:AddAura(SPELL_BLESSING_OF_BLACKFATHOM)
    end
    return true
end

RegisterGameObjectGossipEvent(ENTRY_ALTAR_OF_THE_DEEPS, 1, onAltarGossip)
