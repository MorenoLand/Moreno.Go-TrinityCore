-- Ambassador Flamelash (Blackrock Depths, Chamber of Enchantment) — Lua
-- port of src/server/scripts/EasternKingdoms/BlackrockMountain/
-- BlackrockDepths/boss_ambassador_flamelash.cpp
-- (boss_ambassador_flamelashAI : public ScriptedAI).
-- Entry 9156 — "Ambassador Flamelash" (wowhead-verified:
-- wowhead.com/mop-classic/npc=9156/ambassador-flamelash; no NPC_
-- constant in the C++ tree — the creature_template ScriptName binding
-- is DB-side, no TDB in this workspace).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset (the C++ overrides no KilledUnit/DamageTaken/SpellHit).
-- Timers via CreateLuaEvent per-GUID timers (cancel on leave combat /
-- died / reset); melee is engine-driven in Go (creature combat tick),
-- like C++ DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms):
-- OnEnterCombat(1): EVENT_FIREBLAST 2s, EVENT_SUMMON_SPIRITS 24s.
-- Pump: fireblast arm — non-triggered DoCastVictim(SPELL_FIREBLAST
-- 15573), re-arm 7s regardless (C++-exact: no UNIT_STATE_CASTING
-- early return in this UpdateAI — no casting gate modeled).
-- summon-spirits arm — C++ SummonSpirit x4 (DoSpawnCreature(9178,
-- frand(-9,9), frand(-9,9)) + AttackStart(victim),
-- TEMPSUMMON_TIMED_OR_CORPSE_DESPAWN 60s) then re-arm 30s.
-- PORTED below: live-position offsets + EngageWithTarget(victim)
-- (the 24s/30s schedule stays C++-exact).
-- OnLeaveCombat(2)/OnDied(4)/OnReset(23): cancel the timers (the
-- C++ Reset() _events.Reset() has no other observable arms).
-- Deliberate deviations (all await engine bridges): no
-- the Burning Spirit 9178 summon leg of EVENT_SUMMON_SPIRITS is
-- PORTED below (EngageWithTarget bridge, 2026-10-24); no Talk lines
-- exist in the C++.

local SPELL_FIREBLAST = 15573
local NPC_BURNING_SPIRIT = 9178

local ENTRY = 9156

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

local function castFireblast(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FIREBLAST)
    end
    schedule(guid, "fireblast", 7000, function()
        castFireblast(creature, guid)
    end)
end

-- C++ SummonSpirit x4 (DoSpawnCreature(9178, frand(-9,9),
-- frand(-9,9), 0, 0, TEMPSUMMON_TIMED_OR_CORPSE_DESPAWN, 60s)
-- + AttackStart(victim)): DoSpawnCreature spawns at the boss's
-- live position plus the offsets — creature:GetX/GetY/GetZ
-- resolve via the object-Fields fallback — and the AttackStart
-- leg ports via EngageWithTarget (2026-10-24 bridge; summoned
-- objects need no registration — aran precedent). A nil victim
-- still spawns (C++ spawns before the AttackStart) but engages
-- nothing (aran precedent). Summon type 1 is the bridge's
-- 0-based TIMED_OR_CORPSE_DESPAWN; o 0 rides the bridge's
-- summoner-orientation fallback.
local function summonSpiritsTick(creature, guid)
    local bx, by, bz = creature:GetX(), creature:GetY(), creature:GetZ()
    local victim = creature:GetVictim()
    for _ = 1, 4 do
        local spirit = creature:SummonCreature(NPC_BURNING_SPIRIT,
            bx + math.random() * 18 - 9,
            by + math.random() * 18 - 9,
            bz, 0, 1, 60000)
        if spirit and victim then
            spirit:EngageWithTarget(victim)
        end
    end
    schedule(guid, "spirits", 30000, function()
        summonSpiritsTick(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    -- boss_ambassador_flamelash.cpp JustEngagedWith schedule.
    schedule(guid, "fireblast", 2000, function()
        castFireblast(creature, guid)
    end)
    schedule(guid, "spirits", 24000, function()
        summonSpiritsTick(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)
