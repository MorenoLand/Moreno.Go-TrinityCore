-- Doomwalker (Shadowmoon Valley world boss) — Lua port of
-- src/server/scripts/Outland/boss_doomwalker.cpp.
-- Creature entry 17711 (no NPC_ constant anywhere in the
-- C++ sources; verified via the 2010 TrinityCore changelog
-- paste listing "NPC: Doomwalker has now fixed respawn rate
-- (Entry ID: #17711)" — same paste grounds Doom Lord
-- Kazzak #18728; void_reaver precedent). The
-- creature_template ScriptName binding is DB-side, no TDB
-- in this workspace.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat,
-- 3 OnTargetDied, 4 OnDied, 23 OnReset.
-- Timers via CreateLuaEvent (per-GUID scheduler pump);
-- melee is engine-driven in Go (creature combat tick), like
-- C++ DoMeleeAttackIfReady.
-- Deviations from C++ (documented-only legs):
-- * KilledUnit's victim->CastSpell(victim, SPELL_MARK_DEATH
--   37128, true) (cpp:83) — no player-side CastSpell bridge
--   (boss_ai.go:4247/4389/5330/5395 precedent) — documented
--   only; the 20% SAY_SLAY talk IS ported (event 3).
-- * MoveInLineOfSight's mark->aura conversion (cpp:100-106:
--   who->CastSpell(who, SPELL_AURA_DEATH 37131, triggered)) —
--   same missing player-cast bridge; Go's event 27 is also
--   an aggro-veto hook fired only in the engage branch
--   (lua_creature_events.go:527-540), not a general
--   MoveInLineOfSight notifier — documented only.
-- * The UNIT_STATE_CASTING gate (cpp:115-116) —
--   creature:IsCasting is a stub returning false
--   (lua_objects.go:330) — timers fire unconditionally.
-- * Reset() schedules the events; Lua seeds them in
--   OnEnterCombat (hellmaw precedent) — timing-model
--   deviation.
-- * EVENT_ENRAGE 0s init (cpp:70) -> 1000ms (CreateLuaEvent
--   needs minDelay >= 1ms, runtime.go:1119-1121) — deviation
--   noted.
-- Fidelity notes (C++-exact):
-- * All casts are non-triggered DoCast/DoCastVictim
--   (:123, :130, :143, :148, :152) — plain CastSpell with no
--   triggered flag.
-- * EVENT_ENRAGE re-arms ONLY when health is at/below 20%
--   (cpp:121-128: !HealthAbovePct(20) gate) — otherwise the
--   event dies — modeled exactly, not a 6s health poll.
-- * EVENT_QUAKE's 50% urand(0,1) skip (cpp:134) kills the
--   event with no re-arm — modeled exactly; the firing arm
--   strips enrage via creature:RemoveAura(33653) (cpp:141),
--   C++-exact.
-- * EVENT_CHAIN SelectTarget(Random, 1, 0.0f, true): position
--   1 skips the top-threat entry (UnitAI.h:172-174), dist
--   0.0f = no range limit, playerOnly — modeled as a random
--   alive player on the same map/instance excluding the
--   victim (halion/netherspite victim-exclusion precedent).
-- * KilledUnit's urand(0,4) nonzero skip (cpp:82) — talk only
--   on roll 0 — modeled exactly.

local ENTRY = 17711

local SAY_AGGRO, SAY_EARTHQUAKE, SAY_OVERRUN, SAY_SLAY, SAY_DEATH = 0, 1, 2, 3, 4

local SPELL_EARTHQUAKE      = 32686
local SPELL_SUNDER_ARMOR    = 33661
local SPELL_CHAIN_LIGHTNING = 33665
local SPELL_OVERRUN         = 32636
local SPELL_ENRAGE          = 33653

local timers = {}
local enraged = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
    enraged[guid] = nil
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

local function randomPlayerNotVictim(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local victim = creature:GetVictim()
    local candidates = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() and p ~= victim then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

local function onEnrage(creature, guid)
    if creature:GetHealthPct() <= 20 then
        creature:CastSpell(creature, SPELL_ENRAGE)
        enraged[guid] = true
        schedule(guid, "enrage", 6000, function() onEnrage(creature, guid) end)
    end
    -- C++: no re-arm above 20% — the event dies (cpp:121-128).
end

local function onArmor(creature, guid)
    creature:CastSpell(nil, SPELL_SUNDER_ARMOR)
    schedule(guid, "armor", {10000, 25000}, function() onArmor(creature, guid) end)
end

local function onChain(creature, guid)
    local target = randomPlayerNotVictim(creature)
    if target then
        creature:CastSpell(target, SPELL_CHAIN_LIGHTNING)
    end
    schedule(guid, "chain", {7000, 27000}, function() onChain(creature, guid) end)
end

local function onQuake(creature, guid)
    if math.random(0, 1) == 1 then
        return -- C++: 50% skip kills the event, no re-arm (cpp:134).
    end
    creature:Talk(SAY_EARTHQUAKE)
    if enraged[guid] then
        creature:RemoveAura(SPELL_ENRAGE)
    end
    creature:CastSpell(creature, SPELL_EARTHQUAKE)
    schedule(guid, "quake", {30000, 55000}, function() onQuake(creature, guid) end)
end

local function onOverrun(creature, guid)
    creature:Talk(SAY_OVERRUN)
    creature:CastSpell(nil, SPELL_OVERRUN)
    schedule(guid, "overrun", {25000, 40000}, function() onOverrun(creature, guid) end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "enrage", 1000, function() onEnrage(creature, guid) end)
    schedule(guid, "armor", {5000, 13000}, function() onArmor(creature, guid) end)
    schedule(guid, "chain", {10000, 30000}, function() onChain(creature, guid) end)
    schedule(guid, "quake", {25000, 35000}, function() onQuake(creature, guid) end)
    schedule(guid, "overrun", {30000, 45000}, function() onOverrun(creature, guid) end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onTargetDied(event, creature, victim)
    -- C++: urand(0,4) nonzero -> skip; talk only on roll 0
    -- (cpp:82).
    if math.random(0, 4) == 0 then
        creature:Talk(SAY_SLAY)
    end
end

local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 3, onTargetDied)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)
