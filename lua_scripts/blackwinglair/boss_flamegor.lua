-- Flamegor (Blackwing Lair) — Lua port of
-- src/server/scripts/EasternKingdoms/BlackrockMountain/BlackwingLair/
-- boss_flamegor.cpp (boss_flamegorAI only); blackwing_lair.h:36
-- (DATA_FLAMEGOR = 5, sixth boss), :58 (NPC_FLAMEGOR = 11981).
-- Creature entry: 11981 Flamegor (C++ ScriptName "boss_flamegor" per
-- AddSC_boss_flamegor). One Talk line: EMOTE_FRENZY = 0, spoken on the
-- frenzy arm.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnEnterCombat: arm
-- shadowflame 22539 {10s,20s} then {10s,20s} / wingbuffet 23339 30s
-- then 30s / frenzy 23342 first 10s then {8s,10s} — Talk(EMOTE_FRENZY)
-- + non-triggered self-cast 23342 (C++ DoCast default is
-- triggered=false — vaelastrasz convention, not the broodlord one).
-- The 23342 spell itself periodically triggers fire nova. Nil-victim
-- ticks cast nothing but keep the schedule (jeklik convention).
-- OnDied/OnLeaveCombat(2)/OnReset(23): cancel timers.
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING queue gates
-- (boss_flamegor.cpp pre-loop :66 + in-loop :90) are ported as a
-- 500ms IsCasting defer on every ported timer tick (mal_ganis idiom;
-- IsCasting live, lua_creature_events.go:1950; defer precedes picks,
-- Talks, casts and re-arms, no re-arm consumed); no instance-script model —
-- the instance SetData arms have no bearer; the boss's own
-- BossAI::JustEngagedWith/_Reset/_JustDied bookkeeping writes ARE ported
-- (creature:SetBossState, bridge live lua_creature_events.go:5917): ev1
-- IN_PROGRESS (JustEngagedWith :50-52 calls the base first), ev23
-- NOT_STARTED (no Reset override — base runs, ScriptedCreature.h:328;
-- alive + != DONE gate, ScriptedCreature.cpp:499-511), ev4 DONE (no
-- JustDied override — base runs, ScriptedCreature.h:330); the wingbuffet-arm
-- GetThreat + ModifyThreatByPercent(victim, -75) PORTED 2026-10-24
-- (threat bridge live) — the cast lands and the 30s re-arm is kept.

local ENTRY_FLAMEGOR = 11981

local SPELL_SHADOWFLAME = 22539
local SPELL_WINGBUFFET = 23339
local SPELL_FRENZY = 23342

local EMOTE_FRENZY = 0

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

-- C++ EVENT_SHADOWFLAME: non-triggered DoCastVictim(22539);
-- re-arm {10s,20s}.
local function onShadowflame(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "shadowflame", 500, function()
            onShadowflame(creature, guid)
        end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SHADOWFLAME)
    end
    schedule(guid, "shadowflame", math.random(10000, 20000), function()
        onShadowflame(creature, guid)
    end)
end

-- C++ EVENT_WINGBUFFET: non-triggered DoCastVictim(23339) + -75%
-- threat on the victim (threat arm PORTED 2026-10-24); re-arm 30s.
local function onWingBuffet(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "wingbuffet", 500, function()
            onWingBuffet(creature, guid)
        end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_WINGBUFFET)
        -- C++ arm order: cast, then GetThreat gate + -75%.
        if creature:GetThreat(victim) ~= 0 then
            creature:ModifyThreatByPercent(victim, -75)
        end
    end
    schedule(guid, "wingbuffet", 30000, function()
        onWingBuffet(creature, guid)
    end)
end

-- C++ EVENT_FRENZY: Talk(EMOTE_FRENZY=0), non-triggered self-cast
-- 23342; re-arm {8s,10s}.
local function onFrenzy(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "frenzy", 500, function()
            onFrenzy(creature, guid)
        end)
        return
    end
    creature:Talk(EMOTE_FRENZY)
    creature:CastSpell(creature, SPELL_FRENZY)
    schedule(guid, "frenzy", math.random(8000, 10000), function()
        onFrenzy(creature, guid)
    end)
end

local function flamegorEnterCombat(event, creature, target)
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542; sapphiron precedent)
    -- BossAI::_JustEngagedWith (ScriptedCreature.cpp:537): the base call
    -- sets the boss state IN_PROGRESS; DATA_FLAMEGOR=5 (blackwing_lair.h:36).
    -- C++ calls the base first (:52).
    creature:SetBossState(5, 1)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "shadowflame", math.random(10000, 20000), function()
        onShadowflame(creature, guid)
    end)
    schedule(guid, "wingbuffet", 30000, function()
        onWingBuffet(creature, guid)
    end)
    schedule(guid, "frenzy", 10000, function()
        onFrenzy(creature, guid)
    end)
end

local function flamegorResetState(guid)
    cancelTimers(guid)
end

local function flamegorLeaveCombat(event, creature)
    flamegorResetState(creature:GetGUIDLow())
end

local function flamegorDied(event, creature, killer)
    -- C++ JustDied: no override — base BossAI::JustDied
    -- (ScriptedCreature.h:330) runs _JustDied(), writing
    -- SetBossState(DATA_FLAMEGOR 5, DONE).
    creature:SetBossState(5, 3)
    flamegorResetState(creature:GetGUIDLow())
end

local function flamegorReset(event, creature)
    -- C++ Reset: no override — base BossAI::Reset
    -- (ScriptedCreature.h:328) runs _Reset(), writing
    -- SetBossState(DATA_FLAMEGOR 5, NOT_STARTED), alive-gated and skipped
    -- when already DONE (ScriptedCreature.cpp:499-511); Go fires ev23 on
    -- the death path too, hence the gate.
    if creature:IsAlive() and creature:GetBossState(5) ~= 3 then
        creature:SetBossState(5, 0)
    end
    flamegorResetState(creature:GetGUIDLow())
end

RegisterCreatureEvent(ENTRY_FLAMEGOR, 1, flamegorEnterCombat)
RegisterCreatureEvent(ENTRY_FLAMEGOR, 2, flamegorLeaveCombat)
RegisterCreatureEvent(ENTRY_FLAMEGOR, 4, flamegorDied)
RegisterCreatureEvent(ENTRY_FLAMEGOR, 23, flamegorReset)
