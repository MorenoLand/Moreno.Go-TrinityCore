-- Hellfire Sentry (Hellfire Ramparts, Hellfire Citadel) --
-- Lua port of src/server/scripts/Outland/HellfireCitadel/
-- HellfireRamparts/boss_vazruden_the_herald.cpp (npc_
-- hellfire_sentry — one of the CreatureScript AI classes
-- the same file's AddSC_boss_vazruden_the_herald registers;
-- no SpellScript/AuraScript scripts in this file).
-- Final boss in the Hellfire Ramparts set per
-- outland_script_loader.cpp order (instance_hellfire_
-- ramparts.cpp stays blocked on the instance-script model).
-- Entry: NPC_HELLFIRE_SENTRY = 17517 (hellfire_ramparts.h,
-- verifiable from the C++ sources); instance_hellfire_
-- ramparts.cpp has no OnCreatureCreate mapping for it. The
-- creature_template ScriptName bindings are DB-side (no TDB
-- in this workspace).
-- Talk: none — the C++ AI never Talk()s. The sentry's
-- JustDied override relays to the herald (see deviations).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4
-- OnDied, 23 OnReset. Timers via CreateLuaEvent (per-GUID
-- scheduler pump); melee is engine-driven in Go (creature
-- combat tick), like C++ DoMeleeAttackIfReady. C++ DoCast
-- default is triggered=false (vaelastrasz convention).
-- Fight shape (C++-exact for the modeled arm): Sentry
-- (17517): OnEnterCombat(1): per-GUID reset to the C++
-- Initialize() value (kidneyShot {3s,7s}) + 1s scheduler
-- pump (a port of UpdateAI — 1s granularity exact for the
-- C++ timer; the !UpdateVictim early return collapses into
-- the pump). Pump: kidney shot 30621 — {3s,7s} init then
-- 20s, target = victim (C++ GetVictim(), the nil-fallback
-- in this model, C++-exact), non-triggered DoCast
-- (C++-exact), re-arm 20000 regardless (C++-exact). Melee
-- is engine-driven.
-- OnDied(4): cleanup (the JustDied -> herald SentryDownBy
-- relay is cross-creature blocked, documented below).
-- OnLeaveCombat(2)/OnReset(23): cancel the pump, drop
-- per-GUID state (the Reset() observable remainder — the
-- Initialize() re-latch — lands on OnEnterCombat).
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI
-- shim (instance_hellfire_ramparts.cpp stays blocked on the
-- instance-script model); no cross-creature bridge — the
-- JustDied arm (FindNearestCreature NPC_VAZRUDEN_HERALD
-- 17307 within 150 -> ENSURE_AI SentryDownBy(killer),
-- toggling the herald's sentryDown latch toward the
-- AttackStartNoMove arm) unmodeled, so the herald's
-- phase-0 pull trigger via sentry death is unreachable in
-- this model (see boss_vazruden_the_herald.lua); no
-- SpellScript/AuraScript scripts in this file.

local SPELL_KIDNEY_SHOT = 30621

local NPC_VAZRUDEN_HERALD = 17307

local ENTRY_HELLFIRE_SENTRY = 17517

local timers = {}
local states = {}

local function cancelPump(guid)
    local id = timers[guid]
    if id then
        RemoveEventById(id)
        timers[guid] = nil
    end
end

local function resetSentry(guid)
    cancelPump(guid)
    states[guid] = nil
end

-- C++ Initialize(): kidneyShot urand(3000, 7000).
local function freshState()
    return {
        kidneyShot = math.random(3000, 7000),
    }
end

local function sentryTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end

    -- Kidney shot 30621: {3s,7s} init then 20s — non-
    -- triggered DoCast on the victim (C++ GetVictim(), the
    -- nil-fallback in this model, C++-exact), re-arm 20000
    -- regardless (C++-exact).
    if st.kidneyShot <= 1000 then
        creature:CastSpell(nil, SPELL_KIDNEY_SHOT)
        st.kidneyShot = 20000
    else
        st.kidneyShot = st.kidneyShot - 1000
    end
end

RegisterCreatureEvent(ENTRY_HELLFIRE_SENTRY, 1,
    function(_, creature)
        local guid = creature:GetGUID()
        resetSentry(guid)
        states[guid] = freshState()
        timers[guid] = CreateLuaEvent(function()
            sentryTick(creature, guid)
        end, 1000, 0)
    end)

-- No event 3: the C++ has no KilledUnit override.

RegisterCreatureEvent(ENTRY_HELLFIRE_SENTRY, 2,
    function(_, creature)
        resetSentry(creature:GetGUID())
    end)

RegisterCreatureEvent(ENTRY_HELLFIRE_SENTRY, 4,
    function(_, creature)
        -- The JustDied -> herald SentryDownBy relay has no
        -- cross-creature bridge (documented above).
        resetSentry(creature:GetGUID())
    end)

RegisterCreatureEvent(ENTRY_HELLFIRE_SENTRY, 23,
    function(_, creature)
        resetSentry(creature:GetGUID())
    end)
