-- Razorfen Downs: Mordresh Fire Eye — Lua port of
-- src/server/scripts/Kalimdor/RazorfenDowns/boss_mordresh_fire_eye.cpp
-- (class boss_mordresh_fire_eye : public CreatureScript {
-- boss_mordresh_fire_eyeAI : public BossAI(creature,
-- DATA_MORDRESH_FIRE_EYE) }; GetAI via GetRazorfenDownsAI
-- (DATA_MORDRESH_FIRE_EYE = 1, razorfen_downs.h:32); AddSC at end
-- registers the one script; kalimdor loader decl / call follows
-- AddSC_boss_tuten_kash() (call :183). Entry 7357 wowhead-cited
-- (cata/npc=7357 + tauri npc=7357; no NPC_ constant in the C++
-- tree — vishas/gelihast/landslide precedent). Eluna creature
-- events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied, 23 OnReset.
-- Timers via CreateLuaEvent; melee engine-driven in Go
-- (DoMeleeAttackIfReady).
-- Ported arms:
-- - Reset (C++ Reset, event 23): _Reset instance leg blocked;
--   arms EVENT_OOC_1 at 10s (out-of-combat talk loop).
-- - OOC loop (C++ UpdateAI !UpdateVictim() branch — fires only out
--   of combat; JustEngagedWith's events.Reset() clears it):
--   OOC_1 -> Talk(SAY_OOC_1 0), 8s -> OOC_2 -> Talk(SAY_OOC_2 1),
--   3s -> OOC_3 -> HandleEmoteCommand(EMOTE_ONESHOT_EXCLAMATION)
--   documented-only (no emote bridge), 6s -> OOC_4 ->
--   Talk(SAY_OOC_3 2), 14s -> OOC_1.
-- - Engage (C++ JustEngagedWith): events.Reset() (kills the OOC
--   chain) + Talk(SAY_AGGRO 3) + arm Fireball at 100ms and Fire
--   Nova at {8s,12s} (BossAI::JustEngagedWith instance leg blocked).
-- - Fireball (EVENT_FIREBALL): DoCastVictim(SPELL_FIREBALL 12466),
--   non-triggered (2-arg DoCastVictim); init 100ms -> repeat
--   {2400ms,3800ms} unconditional (nil-victim keeps schedule —
--   jeklik convention).
-- - Fire Nova (EVENT_FIRE_NOVA): DoCast(me, SPELL_FIRE_NOVA 12470),
--   self-cast non-triggered; init {8s,12s} -> repeat {11s,16s}.
-- Unmodeled (documented-only, no bridges):
-- - BossAI ctor / _Reset / _JustDied instance bookkeeping — no
--   instance bridge (standing).
-- - The C++ UNIT_STATE_CASTING early-return gates in UpdateAI —
--   mal_ganis precedent, no casting-state bridge.

local ENTRY = 7357

local SAY_OOC_1  = 0
local SAY_OOC_2  = 1
local SAY_OOC_3  = 2
local SAY_AGGRO  = 3

local SPELL_FIREBALL = 12466
local SPELL_FIRE_NOVA = 12470

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
    per[key] = CreateLuaEvent(fn, delay, 1)
end

local function castVictim(creature, spell)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, spell)
    end
end

-- C++ EVENT_OOC_1..4: out-of-combat-only talk loop; emote leg has
-- no bridge and is documented-only (timing preserved).
local function oocLoop(creature, guid)
    creature:Talk(SAY_OOC_1)
    schedule(guid, "ooc", 8000, function()
        creature:Talk(SAY_OOC_2)
        schedule(guid, "ooc", 3000, function()
            -- HandleEmoteCommand(EMOTE_ONESHOT_EXCLAMATION): no
            -- emote bridge (documented-only).
            schedule(guid, "ooc", 6000, function()
                creature:Talk(SAY_OOC_3)
                schedule(guid, "ooc", 14000, function()
                    oocLoop(creature, guid)
                end)
            end)
        end)
    end)
end

-- C++ EVENT_FIREBALL: DoCastVictim(12466), non-triggered; init
-- 100ms -> repeat {2400ms,3800ms} unconditional.
local function onFireball(creature, guid)
    castVictim(creature, SPELL_FIREBALL)
    schedule(guid, "fireball", math.random(2400, 3800), function()
        onFireball(creature, guid)
    end)
end

-- C++ EVENT_FIRE_NOVA: DoCast(me, 12470) self-cast, non-triggered;
-- init {8s,12s} -> repeat {11s,16s} unconditional.
local function onFireNova(creature, guid)
    creature:CastSpell(creature, SPELL_FIRE_NOVA)
    schedule(guid, "firenova", math.random(11000, 16000), function()
        onFireNova(creature, guid)
    end)
end

-- C++ JustEngagedWith: events.Reset() clears the OOC chain;
-- Talk(SAY_AGGRO); arm combat timers.
local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "fireball", 100, function()
        onFireball(creature, guid)
    end)
    schedule(guid, "firenova", math.random(8000, 12000), function()
        onFireNova(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ Reset: arm the OOC talk loop at 10s (fires out-of-combat
-- only; evade re-arms via Reset like C++).
local function onReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "ooc", 10000, function()
        oocLoop(creature, guid)
    end)
end

local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)
