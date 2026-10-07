-- Magistrate Barthilas (Stratholme) -- Lua port of
-- src/server/scripts/EasternKingdoms/Stratholme/
-- boss_magistrate_barthilas.cpp (boss_magistrate_barthilasAI : public
-- ScriptedAI via GetStratholmeAI; registered by
-- AddSC_boss_magistrate_barthilas in eastern_kingdoms_script_loader.cpp
-- (declaration line 126, call line 304; follows hummel :125/:303)).
-- The file holds 1 script: boss_magistrate_barthilas (pure timer-driven
-- boss AI — no Talk lines, no instance binding, no SpellScript/
-- AuraScript loaders, no gossip/quest/vehicle arms).
-- Entry: no NPC_ constant in the C++ tree (DB-side ScriptName binding) —
-- 10435 independently cited (wowhead npc=10435/magistrate-barthilas).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven
-- (DoMeleeAttackIfReady).
-- Ported arms (C++-exact where bridges exist):
-- - OnEnterCombat: C++ Initialize() — DRAINING_BLOW 20s, CROWD_PUMMEL
--   15s, MIGHTY_BLOW 10s, FURIOUS_ANGER 5s, AngerCount = 0.
-- - EVENT_FURIOUS_ANGER: DoCast(me, SPELL_FURIOUS_ANGER 16791, false)
--   self-cast non-triggered -> creature:CastSpell(creature, 16791);
--   4s re-arm. C++ quirk preserved: the re-arm happens BEFORE the
--   `AngerCount > 25` check, ++AngerCount runs before the cast, so the
--   26th cast is the last (the check then starts failing every 4s).
-- - EVENT_DRAINING_BLOW: DoCastVictim(SPELL_DRAINING_BLOW 16793)
--   non-triggered -> GetVictim + CastSpell (jeklik convention,
--   nil-victim keeps schedule); 20s initial, 15s re-arm.
-- - EVENT_CROWD_PUMMEL: DoCastVictim(SPELL_CROWDPUMMEL 10887)
--   non-triggered -> GetVictim + CastSpell; 15s initial, 15s re-arm.
-- - EVENT_MIGHTY_BLOW: DoCastVictim(SPELL_MIGHTYBLOW 14099)
--   non-triggered -> GetVictim + CastSpell; 10s initial, 20s re-arm.
-- Unmodeled (documented-only, no bridges):
-- - Reset: SetDisplayId(MODEL_NORMAL 10433) when alive /
--   SetDisplayId(MODEL_HUMAN 3637) when dead — no display-ID bridge
--   (jandice precedent); the timer/state reset is modeled.
-- - JustDied: SetDisplayId(MODEL_HUMAN 3637) — same, documented-only;
--   the timer/state clear is modeled.
-- - MoveInLineOfSight / JustEngagedWith: empty in C++ (pass-through to
--   ScriptedAI) — no events registered.
-- - UpdateAI's early `return` when AngerCount > 25 also skips the
--   victim-cast processing of that single diff tick in C++; with the Lua
--   timers decoupled the other pumps keep their cadence (negligible —
--   one server-tick slip per anger tick).
-- Verifiable numbers (file's own enums): SPELL_DRAININGBLOW = 16793,
-- SPELL_CROWDPUMMEL = 10887, SPELL_MIGHTYBLOW = 14099,
-- SPELL_FURIOUS_ANGER = 16791; MODEL_NORMAL = 10433, MODEL_HUMAN = 3637.

local ENTRY_BARTHILAS = 10435

local SPELL_DRAININGBLOW = 16793
local SPELL_CROWDPUMMEL = 10887
local SPELL_MIGHTYBLOW = 14099
local SPELL_FURIOUS_ANGER = 16791

local ANGER_CAST_LIMIT = 25

local timers = {}
local state = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

local function clearState(guid)
    cancelTimers(guid)
    state[guid] = nil
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

local function getState(guid)
    local st = state[guid]
    if not st then
        st = { angerCount = 0 }
        state[guid] = st
    end
    return st
end

-- C++ DoCastVictim(SPELL, false) -> GetVictim + CastSpell, non-triggered
-- (jeklik convention); nil victim keeps the schedule.
local function doCastVictim(creature, spell)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, spell)
    end
end

-- C++ FuriousAnger block: re-arm 4000ms, then `if (AngerCount > 25)
-- return;` before ++AngerCount and the self-cast.
local function onFuriousAnger(creature, guid)
    local st = getState(guid)
    schedule(guid, "anger", 4000, function() onFuriousAnger(creature, guid) end)
    if st.angerCount > ANGER_CAST_LIMIT then
        return
    end
    st.angerCount = st.angerCount + 1
    creature:CastSpell(creature, SPELL_FURIOUS_ANGER)
end

-- C++ DrainingBlow: DoCastVictim(SPELL_DRAININGBLOW); 15000ms re-arm.
local function onDrainingBlow(creature, guid)
    doCastVictim(creature, SPELL_DRAININGBLOW)
    schedule(guid, "drain", 15000, function() onDrainingBlow(creature, guid) end)
end

-- C++ CrowdPummel: DoCastVictim(SPELL_CROWDPUMMEL); 15000ms re-arm.
local function onCrowdPummel(creature, guid)
    doCastVictim(creature, SPELL_CROWDPUMMEL)
    schedule(guid, "pummel", 15000, function() onCrowdPummel(creature, guid) end)
end

-- C++ MightyBlow: DoCastVictim(SPELL_MIGHTYBLOW); 20000ms re-arm.
local function onMightyBlow(creature, guid)
    doCastVictim(creature, SPELL_MIGHTYBLOW)
    schedule(guid, "mighty", 20000, function() onMightyBlow(creature, guid) end)
end

-- C++ Initialize(): DrainingBlow 20000, CrowdPummel 15000,
-- MightyBlow 10000, FuriousAnger 5000, AngerCount = 0.
local function onEnterCombat(event, creature)
    local guid = creature:GetGUID()
    local st = getState(guid)
    st.angerCount = 0
    schedule(guid, "anger", 5000, function() onFuriousAnger(creature, guid) end)
    schedule(guid, "drain", 20000, function() onDrainingBlow(creature, guid) end)
    schedule(guid, "pummel", 15000, function() onCrowdPummel(creature, guid) end)
    schedule(guid, "mighty", 10000, function() onMightyBlow(creature, guid) end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onDied(event, creature)
    clearState(creature:GetGUID())
end

local function onReset(event, creature)
    clearState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_BARTHILAS, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_BARTHILAS, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY_BARTHILAS, 4, onDied)
RegisterCreatureEvent(ENTRY_BARTHILAS, 23, onReset)
