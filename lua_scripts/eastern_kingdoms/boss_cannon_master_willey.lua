-- Cannon Master Willey (Stratholme) -- Lua port of
-- src/server/scripts/EasternKingdoms/Stratholme/boss_cannon_master_willey.cpp
-- (boss_cannon_master_willeyAI : public ScriptedAI via GetStratholmeAI;
-- registered by AddSC_boss_cannon_master_willey in
-- eastern_kingdoms_script_loader.cpp (declaration line 129, call line 307;
-- follows nerubenkan :128/306)).
-- The file holds 1 script: boss_cannon_master_willey (pure timer-driven
-- boss AI — no Talk lines, no gossip/quest/vehicle arms, no
-- SpellScript/AuraScript loaders; C++ JustEngagedWith is empty).
-- Entry: no NPC_ constant in stratholme.h (DB-side ScriptName binding) —
-- 10997 independently cited (classic.wowhead.com/npc=10997/
-- cannon-master-willey).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven
-- (DoMeleeAttackIfReady).
-- Ported arms (C++-exact where bridges exist):
-- - OnEnterCombat: C++ Initialize()/Reset() — SHOOT 1000ms, PUMMEL
--   7000ms, KNOCKAWAY 11000ms, SUMMONRIFLEMAN 15000ms.
-- - EVENT_PUMMEL: C++ `if (rand32() % 100 < 90)` (90% cast chance) then
--   DoCastVictim(SPELL_PUMMEL 15615); 12000ms re-arm is unconditional
--   either way.
-- - EVENT_KNOCKAWAY: C++ `if (rand32() % 100 < 80)` (80% cast chance)
--   then DoCastVictim(SPELL_KNOCKAWAY 10101); 14000ms re-arm is
--   unconditional either way.
-- - EVENT_SHOOT: DoCastVictim(SPELL_SHOOT 16496); 1000ms re-arm.
-- - EVENT_SUMMONRIFLEMAN is PORTED (onSummonRifleman): the timer shape
--   (15000ms then 30000ms re-arm), the C++ `switch (rand32() % 9)` case
--   selection, and the 3x SummonCreature(11054) legs at the C++-exact
--   position triples — 0:(1,2,4), 1:(2,3,5), 2:(3,4,6), 3:(4,5,7),
--   4:(5,6,8), 5:(6,7,9), 6:(7,8,1), 7:(8,9,2), 8:(9,1,3) —
--   TEMPSUMMON_TIMED_DESPAWN (Eluna type 2, 4min), entry 11054
--   (Crimson Rifleman). The stale "no summon bridge" claim is
--   retracted. The 9 positions (C++ ADD_1..ADD_9, all Z 125.001015 /
--   O 0.592007): 1 front-left (3553.851807, -2945.885986), 2
--   front-right (3559.206299, -2952.929932), 3 mid-left (3552.41748,
--   -2948.667236), 4 mid-right (3555.651855, -2953.519043), 5
--   back-left (3547.927246, -2950.977295), 6 back-mid (3553.094697,
--   -2952.123291), 7 back-right (3552.727539, -2957.776123), 8
--   behind-left (3547.15625, -2953.162354), 9 behind-right
--   (3550.202148, -2957.437744).
-- - OnDied: timer clear modeled + the C++ JustDied legs (7x
--   SummonCreature(11054) at positions 1,2,3,4,5,7,9 — note C++ skips
--   6 and 8 here, 4min timed despawn) are PORTED (same summon idiom).
-- Unmodeled (documented-only, no bridges):
-- - UpdateAI's `if (!UpdateVictim()) return` gate: engine-driven.
-- - GetStratholmeAI wrapper: plain AI factory template, no logic.
-- - SPELL_SUMMONCRIMSONRIFLEMAN = 17279: commented out in the C++
--   enum, never used — not modeled.
-- Verifiable numbers (file's own enums): SPELL_KNOCKAWAY = 10101,
-- SPELL_PUMMEL = 15615, SPELL_SHOOT = 16496.

local ENTRY_CANNON_MASTER_WILLEY = 10997

local SPELL_PUMMEL = 15615
local SPELL_KNOCKAWAY = 10101
local SPELL_SHOOT = 16496

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

-- C++ DoCastVictim(SPELL, false) -> GetVictim + CastSpell, non-triggered
-- (jeklik convention); nil victim keeps the schedule.
local function doCastVictim(creature, spell)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, spell)
    end
end

-- C++ `if (rand32() % 100 < 90)` — 90% cast chance; the 12000ms re-arm
-- is unconditional and runs either way.
local function pummelGate()
    return math.random(0, 99) < 90
end

-- C++ `if (rand32() % 100 < 80)` — 80% cast chance; the 14000ms re-arm
-- is unconditional and runs either way.
local function knockAwayGate()
    return math.random(0, 99) < 80
end

-- C++ Pummel: gate, then DoCastVictim(SPELL_PUMMEL); 12000ms re-arm
-- unconditional.
local function onPummel(creature, guid)
    if pummelGate() then
        doCastVictim(creature, SPELL_PUMMEL)
    end
    schedule(guid, "pummel", 12000, function() onPummel(creature, guid) end)
end

-- C++ KnockAway: gate, then DoCastVictim(SPELL_KNOCKAWAY); 14000ms
-- re-arm unconditional.
local function onKnockAway(creature, guid)
    if knockAwayGate() then
        doCastVictim(creature, SPELL_KNOCKAWAY)
    end
    schedule(guid, "knock_away", 14000, function() onKnockAway(creature, guid) end)
end

-- C++ Shoot: DoCastVictim(SPELL_SHOOT); 1000ms re-arm.
local function onShoot(creature, guid)
    doCastVictim(creature, SPELL_SHOOT)
    schedule(guid, "shoot", 1000, function() onShoot(creature, guid) end)
end

-- C++ SummonRifleman timer (15000ms then 30000ms) — PORTED: the
-- rand32() % 9 case selection is preserved as the random leg and the
-- 3x SummonCreature(11054, entry 11054, TEMPSUMMON_TIMED_DESPAWN
-- Eluna type 2, 4min) legs fire at the C++-exact position triples
-- (C++ boss_cannon_master_willey.cpp ADD_1..ADD_9, all Z
-- 125.001015 / O 0.592007): 0:(1,2,4), 1:(2,3,5), 2:(3,4,6),
-- 3:(4,5,7), 4:(5,6,8), 5:(6,7,9), 6:(7,8,1), 7:(8,9,2), 8:(9,1,3).
local ADD_POSITIONS = {
    {3553.851807, -2945.885986}, -- 1 front-left
    {3559.206299, -2952.929932}, -- 2 front-right
    {3552.417480, -2948.667236}, -- 3 mid-left
    {3555.651855, -2953.519043}, -- 4 mid-right
    {3547.927246, -2950.977295}, -- 5 back-left
    {3553.094697, -2952.123291}, -- 6 back-mid
    {3552.727539, -2957.776123}, -- 7 back-right
    {3547.156250, -2953.162354}, -- 8 behind-left
    {3550.202148, -2957.437744}, -- 9 behind-right
}
local RIFLEMAN_Z = 125.001015
local RIFLEMAN_O = 0.592007
local SUMMON_TRIPLES = {
    {1, 2, 4}, {2, 3, 5}, {3, 4, 6}, {4, 5, 7}, {5, 6, 8},
    {6, 7, 9}, {7, 8, 1}, {8, 9, 2}, {9, 1, 3},
}
local function onSummonRifleman(creature, guid)
    local triple = SUMMON_TRIPLES[math.random(0, 8) + 1]
    for _, idx in ipairs(triple) do
        local x, y = ADD_POSITIONS[idx][1], ADD_POSITIONS[idx][2]
        creature:SummonCreature(11054, x, y, RIFLEMAN_Z, RIFLEMAN_O, 2, 240000)
    end
    schedule(guid, "summon_rifleman", 30000, function() onSummonRifleman(creature, guid) end)
end

-- C++ Initialize()/Reset(): SHOOT 1000ms, PUMMEL 7000ms, KNOCKAWAY
-- 11000ms, SUMMONRIFLEMAN 15000ms. JustEngagedWith is empty in C++ —
-- no Talk, no immediate casts.
local function onEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "shoot", 1000, function() onShoot(creature, guid) end)
    schedule(guid, "pummel", 7000, function() onPummel(creature, guid) end)
    schedule(guid, "knock_away", 11000, function() onKnockAway(creature, guid) end)
    schedule(guid, "summon_rifleman", 15000, function() onSummonRifleman(creature, guid) end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onDied(event, creature)
    cancelTimers(creature:GetGUID())
    -- C++ JustDied (boss_cannon_master_willey.cpp:119-126): 7x
    -- SummonCreature(11054, positions 1,2,3,4,5,7,9 — C++ skips 6 and 8
    -- here, all Z 125.001015 / O 0.592007, TEMPSUMMON_TIMED_DESPAWN
    -- Eluna type 2, 4min) — PORTED (the stale "no summon bridge" claim
    -- is retracted).
    for _, idx in ipairs({1, 2, 3, 4, 5, 7, 9}) do
        local x, y = ADD_POSITIONS[idx][1], ADD_POSITIONS[idx][2]
        creature:SummonCreature(11054, x, y, RIFLEMAN_Z, RIFLEMAN_O, 2, 240000)
    end
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_CANNON_MASTER_WILLEY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_CANNON_MASTER_WILLEY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY_CANNON_MASTER_WILLEY, 4, onDied)
RegisterCreatureEvent(ENTRY_CANNON_MASTER_WILLEY, 23, onReset)
