-- Grand Crusader Dathrohan / Balnazzar (Stratholme) -- Lua port of
-- src/server/scripts/EasternKingdoms/Stratholme/
-- boss_dathrohan_balnazzar.cpp (boss_dathrohan_balnazzarAI : public
-- ScriptedAI via GetStratholmeAI; registered by
-- AddSC_boss_dathrohan_balnazzar in eastern_kingdoms_script_loader.cpp
-- (declaration line 135, call line 313; follows baron_rivendare
-- :134/:312)). The file holds 1 script: boss_dathrohan_balnazzar
-- (pure timer-driven boss AI — no Talk lines, no instance binding, no
-- gossip/quest/vehicle arms, no SpellScript/AuraScript loaders).
-- Entry: NPC_DATHROHAN = 10812 (file's own enum; DB-side ScriptName
-- binding — no NPC_ constant in stratholme.h); transforms into
-- NPC_BALNAZZAR = 10813 mid-fight.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven
-- (DoMeleeAttackIfReady).
-- Ported arms (C++-exact where bridges exist):
-- - OnEnterCombat: C++ Initialize() — MINDBLAST 6000, CRUSADERSHAMMER
--   8000, CRUSADERSTRIKE 12000, HOLYSTRIKE 18000, SHADOWSHOCK 4000,
--   PSYCHICSCREAM 16000, DEEPSLEEP 20000, MINDCONTROL 10000,
--   transformed = false.
-- - EVENT_MINDBLAST (both phases, SHARED timer — C++ uses the same
--   m_uiMindBlast_Timer member in both UpdateAI branches, so the
--   pending Lua timer is deliberately NOT cancelled on transform):
--   DoCastVictim(SPELL_MINDBLAST 17287) non-triggered -> GetVictim +
--   CastSpell (jeklik convention, nil-victim keeps schedule);
--   re-arm urand(15000, 20000).
-- - Dathrohan arms (cancelled on transform): CRUSADERSHAMMER 17286
--   8s->12s / CRUSADERSTRIKE 17281 12s->15s / HOLYSTRIKE 17284
--   18s->15s (all DoCastVictim non-triggered -> doCastVictim).
-- - Transform check: C++ evaluates HealthBelowPct(40) every UpdateAI
--   tick in the not-transformed branch. No OnAIUpdate bridge (event 7
--   is unfired in this engine), so a 1s repeating poll approximates it
--   (documented granularity trade-off): on crossing, the Dathrohan
--   arms + poll are cancelled, DoCast(me, SPELL_BALNAZZARTRANSFORM
--   17288, false) self-cast fires (barthilas furious-anger convention),
--   transformed latch set, and the Balnazzar arms start at their
--   C++-frozen initial values (C++ never decrements them pre-transform):
--   SHADOWSHOCK 17399 4s->11s (DoCastVictim) / PSYCHICSCREAM 13704
--   16s->20s (SelectTarget(Random, 0) -> random alive player on
--   map+instance, no distance bound — baron_rivendare precedent;
--   nil pick casts nothing but the 20s re-arm is unconditional, as in
--   C++) / DEEPSLEEP 12098 20s->15s (same random-pick shape) /
--   MINDCONTROL 15690 10s->15s (DoCastVictim).
-- - Reset/LeaveCombat/Died clear timers + transform latch (C++ Reset
--   calls Initialize; the UpdateEntry(NPC_DATHROHAN) revert leg is
--   documented-only).
-- Unmodeled (documented-only, no bridges):
-- - Transform's `if (IsNonMeleeSpellCast(false)) InterruptNonMeleeSpells
--   (false)` gate and me->UpdateEntry(NPC_BALNAZZAR) — no casting-state
--   or entry-update bridges (jandice/barthilas display-ID precedent).
-- - JustDied: 8x SummonCreature(NPC_ZOMBIE = 10698 "probably incorrect",
--   TEMPSUMMON_TIMED_DESPAWN, 1h) at the file's m_aSummonPoint coords
--   — no summon bridge (willey/herod precedent); positions C++-exact:
--   (3444.156, -3090.626, 135.002, 2.240) / (3449.123, -3087.009,
--   135.002, 2.240) / (3446.246, -3093.466, 135.002, 2.240) /
--   (3451.160, -3089.904, 135.002, 2.240) / (3457.995, -3080.916,
--   135.002, 3.784) / (3454.302, -3076.330, 135.002, 3.784) /
--   (3460.975, -3078.901, 135.002, 3.784) / (3457.338, -3073.979,
--   135.002, 3.784). The SDComment ("Possibly need to fix/improve
--   summons after death") stays with C++.
-- - JustEngagedWith: empty in C++ (pass-through to ScriptedAI) — no
--   event registered. GetStratholmeAI factory (luaBossAI shim); melee
--   engine-driven.
-- Verifiable numbers (file's own enums): SPELL_CRUSADERSHAMMER = 17286,
-- SPELL_CRUSADERSTRIKE = 17281, SPELL_HOLYSTRIKE = 17284,
-- SPELL_BALNAZZARTRANSFORM = 17288, SPELL_SHADOWSHOCK = 17399,
-- SPELL_MINDBLAST = 17287, SPELL_PSYCHICSCREAM = 13704,
-- SPELL_SLEEP = 12098, SPELL_MINDCONTROL = 15690;
-- NPC_DATHROHAN = 10812, NPC_BALNAZZAR = 10813, NPC_ZOMBIE = 10698.

local ENTRY_DATHROHAN = 10812

local SPELL_CRUSADERSHAMMER = 17286
local SPELL_CRUSADERSTRIKE = 17281
local SPELL_HOLYSTRIKE = 17284
local SPELL_BALNAZZARTRANSFORM = 17288
local SPELL_SHADOWSHOCK = 17399
local SPELL_MINDBLAST = 17287
local SPELL_PSYCHICSCREAM = 13704
local SPELL_SLEEP = 12098
local SPELL_MINDCONTROL = 15690

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

local function cancelTimer(guid, key)
    local per = timers[guid]
    if per and per[key] then
        RemoveEventById(per[key])
        per[key] = nil
    end
end

local function getState(guid)
    local st = state[guid]
    if not st then
        st = { transformed = false }
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

-- C++ SelectTarget(SelectTargetMethod::Random, 0): dist=0 means
-- "ignored" (UnitAI.h) = unlimited — random alive player on the
-- map+instance (baron_rivendare precedent).
local function randomPlayerOnMap(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local candidates = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EVENT_MINDBLAST (both phases, shared timer):
-- DoCastVictim(SPELL_MINDBLAST); re-arm urand(15000, 20000).
local function onMindBlast(creature, guid)
    doCastVictim(creature, SPELL_MINDBLAST)
    schedule(guid, "mindblast", math.random(15000, 20000),
        function() onMindBlast(creature, guid) end)
end

-- C++ EVENT_CRUSADERSHAMMER: DoCastVictim(17286); 12000ms re-arm.
local function onCrusadersHammer(creature, guid)
    doCastVictim(creature, SPELL_CRUSADERSHAMMER)
    schedule(guid, "hammer", 12000,
        function() onCrusadersHammer(creature, guid) end)
end

-- C++ EVENT_CRUSADERSTRIKE: DoCastVictim(17281); 15000ms re-arm.
local function onCrusaderStrike(creature, guid)
    doCastVictim(creature, SPELL_CRUSADERSTRIKE)
    schedule(guid, "strike", 15000,
        function() onCrusaderStrike(creature, guid) end)
end

-- C++ EVENT_HOLYSTRIKE: DoCastVictim(17284); 15000ms re-arm.
local function onHolyStrike(creature, guid)
    doCastVictim(creature, SPELL_HOLYSTRIKE)
    schedule(guid, "holy", 15000,
        function() onHolyStrike(creature, guid) end)
end

-- C++ EVENT_SHADOWSHOCK (Balnazzar phase): DoCastVictim(17399);
-- 11000ms re-arm.
local function onShadowShock(creature, guid)
    doCastVictim(creature, SPELL_SHADOWSHOCK)
    schedule(guid, "shadowshock", 11000,
        function() onShadowShock(creature, guid) end)
end

-- C++ EVENT_PSYCHICSCREAM: SelectTarget(Random, 0) -> DoCast(target,
-- 13704); 20000ms re-arm unconditionally (nil pick casts nothing).
local function onPsychicScream(creature, guid)
    local target = randomPlayerOnMap(creature)
    if target then
        creature:CastSpell(target, SPELL_PSYCHICSCREAM)
    end
    schedule(guid, "psychicscream", 20000,
        function() onPsychicScream(creature, guid) end)
end

-- C++ EVENT_DEEPSLEEP: SelectTarget(Random, 0) -> DoCast(target,
-- SPELL_SLEEP 12098); 15000ms re-arm unconditionally.
local function onDeepSleep(creature, guid)
    local target = randomPlayerOnMap(creature)
    if target then
        creature:CastSpell(target, SPELL_SLEEP)
    end
    schedule(guid, "deepsleep", 15000,
        function() onDeepSleep(creature, guid) end)
end

-- C++ EVENT_MINDCONTROL: DoCastVictim(15690); 15000ms re-arm.
local function onMindControl(creature, guid)
    doCastVictim(creature, SPELL_MINDCONTROL)
    schedule(guid, "mindcontrol", 15000,
        function() onMindControl(creature, guid) end)
end

-- C++ transform leg: HealthBelowPct(40) -> InterruptNonMeleeSpells (no
-- bridge) -> DoCast(me, SPELL_BALNAZZARTRANSFORM) -> UpdateEntry
-- (NPC_BALNAZZAR) (no bridge) -> transformed = true. The shared
-- mindblast timer is kept pending exactly like the C++ member; the
-- Dathrohan-only arms and the transform poll stop.
local function doTransform(creature, guid)
    local st = getState(guid)
    if st.transformed then
        return
    end
    st.transformed = true
    cancelTimer(guid, "hammer")
    cancelTimer(guid, "strike")
    cancelTimer(guid, "holy")
    cancelTimer(guid, "transformcheck")
    creature:CastSpell(creature, SPELL_BALNAZZARTRANSFORM)
    schedule(guid, "shadowshock", 4000,
        function() onShadowShock(creature, guid) end)
    schedule(guid, "psychicscream", 16000,
        function() onPsychicScream(creature, guid) end)
    schedule(guid, "deepsleep", 20000,
        function() onDeepSleep(creature, guid) end)
    schedule(guid, "mindcontrol", 10000,
        function() onMindControl(creature, guid) end)
end

-- C++ checks HealthBelowPct(40) every UpdateAI tick while not
-- transformed. No OnAIUpdate bridge (event 7 unfired) — 1s poll.
local function onTransformCheck(creature, guid)
    local st = getState(guid)
    if st.transformed then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth > 0 and creature:GetHealth() * 100 / maxHealth < 40 then
        doTransform(creature, guid)
        return
    end
    schedule(guid, "transformcheck", 1000,
        function() onTransformCheck(creature, guid) end)
end

-- C++ Initialize(): hammer 8000, strike 12000, mindblast 6000,
-- holy 18000, shadowshock 4000, psychicscream 16000, deepsleep 20000,
-- mindcontrol 10000, transformed = false. The Balnazzar-only timers
-- stay frozen until doTransform schedules them at those initials.
local function onEnterCombat(event, creature)
    local guid = creature:GetGUID()
    local st = getState(guid)
    st.transformed = false
    schedule(guid, "mindblast", 6000,
        function() onMindBlast(creature, guid) end)
    schedule(guid, "hammer", 8000,
        function() onCrusadersHammer(creature, guid) end)
    schedule(guid, "strike", 12000,
        function() onCrusaderStrike(creature, guid) end)
    schedule(guid, "holy", 18000,
        function() onHolyStrike(creature, guid) end)
    schedule(guid, "transformcheck", 1000,
        function() onTransformCheck(creature, guid) end)
end

local function onLeaveCombat(event, creature)
    clearState(creature:GetGUID())
end

local function onDied(event, creature)
    clearState(creature:GetGUID())
end

local function onReset(event, creature)
    clearState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_DATHROHAN, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_DATHROHAN, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY_DATHROHAN, 4, onDied)
RegisterCreatureEvent(ENTRY_DATHROHAN, 23, onReset)
