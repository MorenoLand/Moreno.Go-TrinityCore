-- Baroness Anastari (Stratholme) -- Lua port of
-- src/server/scripts/EasternKingdoms/Stratholme/boss_baroness_anastari.cpp
-- (boss_baroness_anastari : public BossAI via RegisterStratholmeCreatureAI,
-- TYPE_BARONESS = 2, registered by AddSC_boss_baroness_anastari in
-- eastern_kingdoms_script_loader.cpp (declaration line 130, call line
-- 308; follows cannon_master_willey :129/307)).
-- The file holds 1 script: boss_baroness_anastari (pure timer-driven
-- boss AI — no Talk lines, no gossip/quest/vehicle arms, no
-- SpellScript/AuraScript loaders).
-- Entry: no NPC_ constant in stratholme.h (DB-side ScriptName binding) —
-- 10436 independently cited (wowhead npc=10436/baroness-anastari).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven
-- (DoMeleeAttackIfReady).
-- Ported arms (C++-exact where bridges exist):
-- - OnEnterCombat (C++ JustEngagedWith): BANSHEEWAIL 1000ms /
--   BANSHEECURSE 11000ms / SILENCE 13000ms / POSSESS random 20000-30000ms
--   (C++ ScheduleEvent(EVENT_SPELL_POSSESS, 20s, 30s)).
-- - EVENT_BANSHEEWAIL: non-triggered DoCastVictim(SPELL_BANSHEEWAIL
--   16565); 4000ms re-arm.
-- - EVENT_BANSHEECURSE: non-triggered DoCastVictim(SPELL_BANSHEECURSE
--   16867); 18000ms re-arm.
-- - EVENT_SILENCE: non-triggered DoCastVictim(SPELL_SILENCE 18327);
--   13000ms re-arm.
-- - EVENT_POSSESS: C++ SelectTarget(Random, 1, 0, true, false) — random
--   alive player excluding the victim (position 1, fairbanks-fear
--   convention), any range (0 distance), LOS required, no aura filter.
--   Target found: triggered DoCast(possessTarget, SPELL_POSSESS 17244) +
--   triggered DoCast(possessTarget, SPELL_POSSESSED 17246) + triggered
--   DoCastSelf(SPELL_POSSESS_INV 17250, baroness invisible), latch the
--   target GUID, and schedule EVENT_CHECK_POSSESSED on the next tick.
--   No target: Repeat(20s, 30s) (random re-arm).
-- - EVENT_CHECK_POSSESSED: C++ ObjectAccessor::GetPlayer(*me,
--   _possessedTargetGuid) -> GetPlayerByGUID (nil when the player is
--   gone — the event is then consumed with no re-arm, C++-exact).
--   Player found and (!HasAura(SPELL_POSSESSED 17246) or
--   HealthBelowPct(50)): RemoveAura(17244) + RemoveAura(17246) on the
--   player, RemoveAura(17250) on the baroness, clear the GUID latch,
--   reschedule POSSESS 20s-30s random, cancel the check. Still
--   possessed: Repeat(1s).
-- - OnReset (C++ Reset()): GUID latch clear (via timer-state wipe) +
--   me->RemoveAurasDueToSpell(SPELL_POSSESS_INV) modeled as
--   creature:RemoveAura(17250) (kirtonos precedent).
-- Unmodeled (documented-only, no bridges):
-- - Reset's instance->DoRemoveAurasDueToSpellOnPlayers(SPELL_POSSESS)
--   and (SPELL_POSSESSED): no instance-aura bridge.
-- - JustDied instance->SetData(TYPE_BARONESS 2, IN_PROGRESS): no
--   instance-script bridge (nerubenkan precedent; IN_PROGRESS-not-DONE
--   is a C++ quirk — needed until crystals are implemented, see
--   instance_stratholme.cpp line 305).
-- - UpdateAI's `if (!UpdateVictim()) return` gate and the
--   UNIT_STATE_CASTING gates before/after the event loop: engine-driven
--   (barthilas precedent).
-- - BossAI ctor (boss_baroness_anastari(Creature*, TYPE_BARONESS)):
--   ctor wiring only, no logic.
-- Verifiable numbers (file's own enums): SPELL_BANSHEEWAIL = 16565,
-- SPELL_BANSHEECURSE = 16867, SPELL_SILENCE = 18327, SPELL_POSSESS =
-- 17244, SPELL_POSSESSED = 17246, SPELL_POSSESS_INV = 17250.

local ENTRY_BARONESS_ANASTARI = 10436

local SPELL_BANSHEEWAIL = 16565
local SPELL_BANSHEECURSE = 16867
local SPELL_SILENCE = 18327
local SPELL_POSSESS = 17244
local SPELL_POSSESSED = 17246
local SPELL_POSSESS_INV = 17250

-- per-GUID state: timers table plus the possessed-target GUID latch
-- (C++ _possessedTargetGuid)
local state = {}

local function cancelTimers(guid)
    local st = state[guid]
    if st then
        for _, id in pairs(st.timers) do
            RemoveEventById(id)
        end
        state[guid] = nil
    end
end

local function schedule(guid, key, delay, fn)
    local st = state[guid]
    if not st then
        st = { timers = {}, possessedGuid = nil }
        state[guid] = st
    end
    if st.timers[key] then
        RemoveEventById(st.timers[key])
    end
    st.timers[key] = CreateLuaEvent(fn, delay)
end

-- C++ DoCastVictim(SPELL, false) -> GetVictim + CastSpell, non-triggered
-- (jeklik convention); nil victim keeps the schedule.
local function doCastVictim(creature, spell)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, spell)
    end
end

-- C++ SelectTarget(Random, 1, 0, true, false) — random alive player on
-- the same map+instance excluding the given GUID (position 1 excludes
-- the victim, fairbanks-fear convention); 0 distance = any range, LOS
-- leg not modeled (no bridge).
local function randomAlivePlayerExcluding(creature, excludeGUID)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local players = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() and p:GetGUID() ~= excludeGUID then
            players[#players + 1] = p
        end
    end
    if #players == 0 then
        return nil
    end
    return players[math.random(#players)]
end

-- C++ ScheduleEvent(EVENT_SPELL_POSSESS, 20s, 30s) / Repeat(20s, 30s):
-- random re-arm between 20 and 30 seconds.
local function possessDelay()
    return 20000 + math.random(0, 10000)
end

-- C++ EVENT_SPELL_BANSHEEWAIL: DoCastVictim(16565); Repeat(4s).
local function onBansheeWail(creature, guid)
    doCastVictim(creature, SPELL_BANSHEEWAIL)
    schedule(guid, "banshee_wail", 4000, function() onBansheeWail(creature, guid) end)
end

-- C++ EVENT_SPELL_BANSHEECURSE: DoCastVictim(16867); Repeat(18s).
local function onBansheeCurse(creature, guid)
    doCastVictim(creature, SPELL_BANSHEECURSE)
    schedule(guid, "banshee_curse", 18000, function() onBansheeCurse(creature, guid) end)
end

-- C++ EVENT_SPELL_SILENCE: DoCastVictim(18327); Repeat(13s).
local function onSilence(creature, guid)
    doCastVictim(creature, SPELL_SILENCE)
    schedule(guid, "silence", 13000, function() onSilence(creature, guid) end)
end

-- C++ EVENT_CHECK_POSSESSED: 1s polling leg. Player gone (nil GUID
-- lookup, ObjectAccessor::GetPlayer parity): the event is consumed with
-- no re-arm (C++-exact) — the possess chain dies until Reset.
onCheckPossessed = function(creature, guid)
    local st = state[guid]
    if not st or not st.possessedGuid then
        return
    end
    local target = GetPlayerByGUID(st.possessedGuid)
    if not target then
        return
    end
    if not target:HasAura(SPELL_POSSESSED)
            or target:GetHealth() * 100 < target:GetMaxHealth() * 50 then
        target:RemoveAura(SPELL_POSSESS)
        target:RemoveAura(SPELL_POSSESSED)
        creature:RemoveAura(SPELL_POSSESS_INV)
        st.possessedGuid = nil
        schedule(guid, "possess", possessDelay(), function() onPossess(creature, guid) end)
    else
        schedule(guid, "check_possessed", 1000, function() onCheckPossessed(creature, guid) end)
    end
end

-- Forward declaration: onPossess and onCheckPossessed recurse into each
-- other (doan precedent — Lua locals are only visible after declaration).
local onCheckPossessed

-- C++ EVENT_SPELL_POSSESS: random non-victim player pick; triggered
-- casts on pick + invisibility self-cast + GUID latch + immediate check
-- scheduling; no pick -> Repeat(20s, 30s).
local function onPossess(creature, guid)
    local victim = creature:GetVictim()
    local target = randomAlivePlayerExcluding(creature, victim and victim:GetGUID() or nil)
    if target then
        creature:CastSpell(target, SPELL_POSSESS, true)
        creature:CastSpell(target, SPELL_POSSESSED, true)
        creature:CastSpell(creature, SPELL_POSSESS_INV, true)
        local st = state[guid]
        if st then
            st.possessedGuid = target:GetGUID()
        end
        schedule(guid, "check_possessed", 100, function() onCheckPossessed(creature, guid) end)
    else
        schedule(guid, "possess", possessDelay(), function() onPossess(creature, guid) end)
    end
end

-- C++ JustEngagedWith: 1s / 11s / 13s / 20s-30s random.
local function onEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "banshee_wail", 1000, function() onBansheeWail(creature, guid) end)
    schedule(guid, "banshee_curse", 11000, function() onBansheeCurse(creature, guid) end)
    schedule(guid, "silence", 13000, function() onSilence(creature, guid) end)
    schedule(guid, "possess", possessDelay(), function() onPossess(creature, guid) end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onDied(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ Reset(): latch clear (timer-state wipe) +
-- me->RemoveAurasDueToSpell(SPELL_POSSESS_INV); the instance-wide
-- DoRemoveAurasDueToSpellOnPlayers legs have no bridge (documented in
-- the header).
local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
    creature:RemoveAura(SPELL_POSSESS_INV)
end

RegisterCreatureEvent(ENTRY_BARONESS_ANASTARI, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_BARONESS_ANASTARI, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY_BARONESS_ANASTARI, 4, onDied)
RegisterCreatureEvent(ENTRY_BARONESS_ANASTARI, 23, onReset)
