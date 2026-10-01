-- Quartermaster Zigris (Blackrock Spire; Hall of Blackhand) --
-- Lua port of src/server/scripts/EasternKingdoms/BlackrockMountain/
-- BlackrockSpire/boss_quartermaster_zigris.cpp
-- (quartermaster_zigris — BossAI combat scheduler). Boss-script unit
-- per eastern_kingdoms_script_loader.cpp order (boss_warmaster_voone
-- done, registered for 9237; AddSC_boss_quatermasterzigris next in the
-- Blackrock Spire block).
-- Entry (verifiable from the C++ sources): blackrock_spire.h names
-- NPC_QUARTERMASTER_ZIGRIS = 9736 in the BRS creatures enum. The
-- creature_template ScriptName binding is DB-side (no TDB in this
-- workspace), but the entry itself is C++-verifiable so the port is
-- registered (boss_warmaster_voone / boss_the_beast precedent).
-- GetBlackrockSpireAI is a GetInstanceAI retrieval wrapper only
-- (blackrock_spire.h:129-132); the AI has no instance arms —
-- Reset() _Reset() / JustDied _JustDied() are internal machinery
-- covered here by the cancel/re-arm on combat events (halycon
-- precedent). SPELL_HEALING_POTION (15504) and SPELL_HOOKEDNET
-- (15609) are defined in the file's spell enum but never cast by the
-- AI — omitted by design (shadowvosh ice-armor precedent).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Driven by CreateLuaEvent timers; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++ JustEngagedWith event schedule + UpdateAI event
-- machine):
-- EVENT_SHOOT: victim-cast SHOOT 16496, 1s init -> 500ms loop
-- (maiden convention — DoCastVictim path, creature:GetVictim()).
-- EVENT_STUN_BOMB: victim-cast STUNBOMB 16497, 16s init -> 14s loop
-- (maiden convention).
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (creature
-- casts are packet-visual), so timers fire unconditionally and both
-- in-loop casting gates are dropped (maiden precedent). The C++
-- scheduler is driven from JustEngagedWith and drained by UpdateAI;
-- the port schedules from OnEnterCombat(1) and cancels on 2/4/23
-- (maiden convention).

local SPELL_SHOOT = 16496
local SPELL_STUNBOMB = 16497

local ENTRY_QUARTERMASTER_ZIGRIS = 9736

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

local function onShoot(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SHOOT)
    end
    schedule(guid, "shoot", 500, function() onShoot(creature, guid) end)
end

local function onStunBomb(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_STUNBOMB)
    end
    schedule(guid, "stunbomb", 14000, function() onStunBomb(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "shoot", 1000, function() onShoot(creature, guid) end)
    schedule(guid, "stunbomb", 16000, function() onStunBomb(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_QUARTERMASTER_ZIGRIS, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_QUARTERMASTER_ZIGRIS, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_QUARTERMASTER_ZIGRIS, 4, onCombatEnd)
RegisterCreatureEvent(ENTRY_QUARTERMASTER_ZIGRIS, 23, onCombatEnd)
