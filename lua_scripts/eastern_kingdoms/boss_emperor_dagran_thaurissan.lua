-- Emperor Dagran Thaurissan (Blackrock Depths) --
-- Lua port of src/server/scripts/EasternKingdoms/BlackrockMountain/
-- BlackrockDepths/boss_emperor_dagran_thaurissan.cpp
-- (boss_emperor_dagran_thaurissan — ScriptedAI combat scheduler, derived
-- via GetBlackrockDepthsAI, an instance-AI retrieval wrapper). Boss-script
-- unit per eastern_kingdoms_script_loader.cpp order (boss_draganthaurissan
-- follows AddSC_boss_ambassador_flamelash in the BRD block).
-- Entry (verifiable from the C++ sources): NPC_EMPEROR = 9019 in
-- instance_blackrock_depths.cpp line 35 (the BRD instance creatures enum).
-- The creature_template ScriptName binding is DB-side (no TDB in this
-- workspace), but the entry itself is C++-verifiable so the port is
-- registered (npc_phalanx precedent).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 23 OnReset. Driven by CreateLuaEvent timers; melee is
-- engine-driven in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++ JustEngagedWith schedule + UpdateAI event machine):
-- SPELL_AVATAROFFLAME: victim-cast AVATAR 15636, 25s init -> 18s;
-- EVENT_HANDOFTHAURISSAN: SelectTarget(Random, 0) -> DoCast(target,
-- SPELL_HANDOFTHAURISSAN 17492) non-triggered, 4s init -> 5s
-- unconditional re-arm (C++ re-arms outside the target check),
-- ported via the maiden_of_virtue / thorim random-player pool
-- (GetPlayersInWorld, same map/instance, alive; no distance limit —
-- C++ distance 0.0f is unlimited); stale "no SelectTarget bridge"
-- (kazzak / doomwalker) retracted.
-- JustEngagedWith Talk(SAY_AGGRO=0);
-- KilledUnit victim->GetTypeId() == TYPEID_PLAYER -> Talk(SAY_SLAY=1).
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (creature
-- casts are packet-visual), so timers fire unconditionally and the
-- in-loop casting-state gate is dropped (maiden precedent). The C++
-- scheduler is driven from JustEngagedWith and drained by UpdateAI;
-- the port schedules from OnEnterCombat(1) and cancels on
-- OnLeaveCombat(2)/OnDied(4)/OnReset(23) (maiden convention).
-- - EVENT_HANDOFTHAURISSAN now ported (above); the old "no
--   SelectTarget bridge" claim (kazzak / doomwalker) was stale and
--   is retracted.
-- Unmodeled: JustEngagedWith me->CallForHelp(VISIBLE_RANGE) — no
-- CallForHelp bridge on the luaMotionCreature surface.
-- JustDied moira arm PORTED below (onDied): the instance GUID
-- lookup is a FindNearestCreature(8929, 100.0) search (moira
-- stands in the throne room; instance_blackrock_depths.cpp:45
-- NPC_MOIRA = 8929) + SetFaction(FACTION_FRIENDLY 35) +
-- Talk(EMOTE_SHAKEN=0), all bridged. Only the EnterEvadeMode
-- leg stays out (no bridge, 2026-10-21) — moira takes no part
-- in the fight, so evade is cleanup-only here.

local SPELL_AVATAROFFLAME = 15636
local SPELL_HANDOFTHAURISSAN = 17492

local SAY_AGGRO = 0
local SAY_SLAY = 1
local EMOTE_SHAKEN = 0
local FACTION_FRIENDLY = 35
local ENTRY_MOIRA = 8929

local TYPEID_PLAYER = 4

local ENTRY_EMPEROR_DAGRAN_THAURISSAN = 9019

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
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

-- C++ SelectTarget(Random, 0) legs (maiden_of_virtue / thorim precedent):
-- GetPlayersInWorld filtered by map/instance/alive; no distance limit
-- (C++ distance 0.0f = unlimited).
local function randomPlayer(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local candidates = {}
    for _, plr in ipairs(GetPlayersInWorld()) do
        if plr:GetMapId() == mapId and plr:GetInstanceId() == instanceId
                and not plr:IsDead() then
            candidates[#candidates + 1] = plr
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

local function onHandOfThaurissan(creature, guid)
    local target = randomPlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_HANDOFTHAURISSAN)
    end
    schedule(guid, "handofthaurissan", 5000, function() onHandOfThaurissan(creature, guid) end)
end

local function onAvatarOfFlame(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_AVATAROFFLAME)
    end
    schedule(guid, "avatarofflame", 18000, function() onAvatarOfFlame(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "handofthaurissan", 4000, function() onHandOfThaurissan(creature, guid) end)
    schedule(guid, "avatarofflame", 25000, function() onAvatarOfFlame(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ JustDied (boss_emperor_dagran_thaurissan.cpp:78-85):
-- moira->AI()->EnterEvadeMode() (no bridge, stays out) +
-- moira->SetFaction(FACTION_FRIENDLY) + moira->AI()->
-- Talk(EMOTE_SHAKEN); nil moira skips all three arms.
local function onDied(_, creature)
    cancelTimers(creature:GetGUID())
    local moira = creature:FindNearestCreature(ENTRY_MOIRA, 100.0)
    if moira then
        moira:SetFaction(FACTION_FRIENDLY)
        moira:Talk(EMOTE_SHAKEN)
    end
end

local function onReset(_, creature)
    cancelTimers(creature:GetGUID())
end

local function onTargetDied(_, creature, victim)
    if victim and victim:GetTypeId() == TYPEID_PLAYER then
        creature:Talk(SAY_SLAY)
    end
end

RegisterCreatureEvent(ENTRY_EMPEROR_DAGRAN_THAURISSAN, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_EMPEROR_DAGRAN_THAURISSAN, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_EMPEROR_DAGRAN_THAURISSAN, 3, onTargetDied)
RegisterCreatureEvent(ENTRY_EMPEROR_DAGRAN_THAURISSAN, 4, onDied)
RegisterCreatureEvent(ENTRY_EMPEROR_DAGRAN_THAURISSAN, 23, onReset)
