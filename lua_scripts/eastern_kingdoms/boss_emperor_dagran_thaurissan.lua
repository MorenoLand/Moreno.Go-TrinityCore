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
-- JustEngagedWith Talk(SAY_AGGRO=0);
-- KilledUnit victim->GetTypeId() == TYPEID_PLAYER -> Talk(SAY_SLAY=1).
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (creature
-- casts are packet-visual), so timers fire unconditionally and the
-- in-loop casting-state gate is dropped (maiden precedent). The C++
-- scheduler is driven from JustEngagedWith and drained by UpdateAI;
-- the port schedules from OnEnterCombat(1) and cancels on
-- OnLeaveCombat(2)/OnDied(4)/OnReset(23) (maiden convention).
-- Unmodeled: EVENT_HANDOFTHAURISSAN (4s init -> 5s) -> SelectTarget
-- (Random) + DoCast(target, 17492) — no SelectTarget bridge on the Lua
-- surface (kazzak / doomwalker precedent); the timer chain is
-- documented-only, not a no-op dead loop.
-- Unmodeled: JustEngagedWith me->CallForHelp(VISIBLE_RANGE) — no
-- CallForHelp bridge on the luaMotionCreature surface.
-- Unmodeled: JustDied moira arm — ObjectAccessor::GetCreature via
-- _instance->GetGuidData(DATA_MOIRA) + AI()->EnterEvadeMode() +
-- SetFaction(FACTION_FRIENDLY) + Talk(EMOTE_SHAKEN=0) — no
-- instance-script / GetGuidData / SetFaction bridges on the Lua surface
-- (instance_blackrock_depths.cpp stays blocked on the instance-script
-- model).

local SPELL_AVATAROFFLAME = 15636

local SAY_AGGRO = 0
local SAY_SLAY = 1

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
    per[key] = CreateLuaEvent(fn, delay)
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
    schedule(guid, "avatarofflame", 25000, function() onAvatarOfFlame(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
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
RegisterCreatureEvent(ENTRY_EMPEROR_DAGRAN_THAURISSAN, 4, onCombatEnd)
RegisterCreatureEvent(ENTRY_EMPEROR_DAGRAN_THAURISSAN, 23, onReset)
