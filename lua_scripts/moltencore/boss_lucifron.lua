-- Lucifron (Molten Core) — Lua port of
-- src/server/scripts/EasternKingdoms/BlackrockMountain/MoltenCore/
-- boss_lucifron.cpp (boss_lucifronAI only); molten_core.h:30
-- (BOSS_LUCIFRON = 0, first boss), :54 (NPC_LUCIFRON = 12118).
-- Creature entry: 12118 Lucifron (C++ ScriptName "boss_lucifron" per
-- AddSC_boss_lucifron). No Talk lines in the C++ AI.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnEnterCombat: arm
-- impending doom 19702 10s then 20s / Lucifron's curse 19703 20s then
-- 15s / shadow shock 20603 6s then 6s, all non-triggered DoCastVictim
-- (C++ DoCast default is triggered=false — vaelastrasz convention).
-- Nil-victim ticks cast nothing but keep the schedule (jeklik
-- convention). OnDied/OnLeaveCombat(2)/OnReset(23): cancel timers.
-- Cast gating PORTED (mal_ganis idiom): the C++ UpdateAI
-- UNIT_STATE_CASTING queue gate (boss_lucifron.cpp:70) and post-event
-- check (:93) are modeled on every timer tick via the live
-- creature:IsCasting() bridge (engine/world/lua_creature_events.go:1950,
-- CastingUntil window) — a casting tick defers 500ms under its own
-- key, consuming no re-arm. No instance-script model — boss admission
-- via the luaBossAI shim; the _Reset / _JustDied / _JustEngagedWith
-- bookkeeping arms ARE ported below (creature:SetBossState, bridge live
-- lua_creature_events.go:5917): ev1 IN_PROGRESS (JustEngagedWith
-- :55-57 calls the base first), ev23 NOT_STARTED (no Reset override —
-- base runs), ev4 DONE (no JustDied override — base runs by virtual
-- dispatch).

local ENTRY_LUCIFRON = 12118

local SPELL_IMPENDING_DOOM = 19702
local SPELL_LUCIFRON_CURSE = 19703
local SPELL_SHADOW_SHOCK = 20603

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

-- C++ EVENT_IMPENDING_DOOM: non-triggered DoCastVictim(19702);
-- re-arm 20s.
local function onImpendingDoom(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "impendingdoom", 500, function()
            onImpendingDoom(creature, guid)
        end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_IMPENDING_DOOM)
    end
    schedule(guid, "impendingdoom", 20000, function()
        onImpendingDoom(creature, guid)
    end)
end

-- C++ EVENT_LUCIFRON_CURSE: non-triggered DoCastVictim(19703);
-- re-arm 15s.
local function onLucifronCurse(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "lucifroncurse", 500, function()
            onLucifronCurse(creature, guid)
        end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_LUCIFRON_CURSE)
    end
    schedule(guid, "lucifroncurse", 15000, function()
        onLucifronCurse(creature, guid)
    end)
end

-- C++ EVENT_SHADOW_SHOCK: non-triggered DoCastVictim(20603);
-- re-arm 6s.
local function onShadowShock(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "shadowshock", 500, function()
            onShadowShock(creature, guid)
        end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SHADOW_SHOCK)
    end
    schedule(guid, "shadowshock", 6000, function()
        onShadowShock(creature, guid)
    end)
end

local function lucifronEnterCombat(event, creature, target)
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542; sapphiron precedent)
    -- C++ JustEngagedWith (boss_lucifron.cpp:55-57) calls
    -- BossAI::JustEngagedWith first; _JustEngagedWith writes
    -- SetBossState(BOSS_LUCIFRON 0, IN_PROGRESS) (ScriptedCreature.cpp:537).
    creature:SetBossState(0, 1)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "impendingdoom", 10000, function()
        onImpendingDoom(creature, guid)
    end)
    schedule(guid, "lucifroncurse", 20000, function()
        onLucifronCurse(creature, guid)
    end)
    schedule(guid, "shadowshock", 6000, function()
        onShadowShock(creature, guid)
    end)
end

local function lucifronResetState(guid)
    cancelTimers(guid)
end

local function lucifronLeaveCombat(event, creature)
    lucifronResetState(creature:GetGUID())
end

local function lucifronDied(event, creature, killer)
    -- No JustDied override in C++ — base BossAI::JustDied runs by
    -- virtual dispatch (ScriptedCreature.h:330); _JustDied() writes
    -- SetBossState(BOSS_LUCIFRON 0, DONE).
    creature:SetBossState(0, 3)
    lucifronResetState(creature:GetGUID())
end

local function lucifronReset(event, creature)
    -- No Reset override in C++ — base BossAI::Reset runs; _Reset()
    -- writes SetBossState(BOSS_LUCIFRON 0, NOT_STARTED), alive-gated
    -- and skipped when already DONE (ScriptedCreature.cpp:499-511);
    -- Go fires ev23 on the death path too, hence the gate.
    if creature:IsAlive() and creature:GetBossState(0) ~= 3 then
        creature:SetBossState(0, 0)
    end
    lucifronResetState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_LUCIFRON, 1, lucifronEnterCombat)
RegisterCreatureEvent(ENTRY_LUCIFRON, 2, lucifronLeaveCombat)
RegisterCreatureEvent(ENTRY_LUCIFRON, 4, lucifronDied)
RegisterCreatureEvent(ENTRY_LUCIFRON, 23, lucifronReset)
