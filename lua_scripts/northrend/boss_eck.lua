-- Eck the Ferocious (Gundrak) — Lua port of
-- src/server/scripts/Northrend/Gundrak/boss_eck.cpp (boss_eck).
-- Gundrak dungeon-script unit per northrend_script_loader.cpp order
-- (decl 24 / call 219, immediately after AddSC_boss_gal_darah();
-- next: Gundrak instance / next dungeon block).
-- Entry: 29932 Eck the Ferocious (gundrak.h NPC_ECK_THE_FEROCIOUS —
-- the RegisterCreatureAIWithFactory(GetGundrakAI) ScriptName binding
-- is instance-shimmed, the creature_template binding DB-side as
-- usual).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4
-- OnDied, 23 OnReset. Timers via CreateLuaEvent; melee is
-- engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. C++ DoCast default is triggered=false
-- (vaelastrasz convention): creature:CastSpell(nil, spell) =
-- DoCastVictim, creature:CastSpell(creature, spell) = DoCastSelf
-- (moroes / moorabi precedent). OnLeaveCombat(2)/OnDied(4)/
-- OnReset(23) cancel the scheduler (gargolmar precedent); BossAI
-- _Reset/_JustDied instance bookkeeping has no bridge.
-- Ported arms (C++-exact for all modeled arms):
-- boss_eck: JustEngagedWith-scheduled combat rotation —
-- EVENT_BITE DoCastVictim(Eck Bite 55813, 5s init, 8-12s repeat),
-- EVENT_SPIT DoCastVictim(Eck Spit 55814, 10s init, 6-14s repeat),
-- EVENT_BERSERK DoCastSelf(Eck Berserk 55816, 60-90s init,
-- single fire — C++ does not reschedule after it fires).
-- Unmodeled (no bridges — documented, not wired):
-- The BossAI ctor Talk(EMOTE_SPAWN 0) fires at AI construction
-- (first grid spawn); the only OnSpawn hook on the Go Lua surface
-- (event 5) fires on the respawn path only (kill.go
-- fireCreatureSpawned — phase_hunter precedent), so wiring it
-- there would miss fresh spawns — not C++-exact, documented-only.
-- DamageTaken: HealthBelowPctDamaged(20) -> RescheduleEvent
-- (EVENT_BERSERK, 1s) + _berserk latch — no health-pct bridge on
-- the Lua surface (doomwalker precedent), so the early-berserk leg
-- cannot be emulated; only the 60-90s timer rides the scheduler.
-- EVENT_SPRING — SelectTarget(Random, 1, 35.0f, true) ->
-- DoCast(target, RAND(SPELL_ECK_SPRING_1 55815,
-- SPELL_ECK_SPRING_2 55837)), 8s init, 5-10s repeat: the
-- random-target SelectTarget bridge is absent (cairne/kazzak
-- precedent), so the cast leg is unmodelable — documented-only.

local ENTRY_ECK_THE_FEROCIOUS = 29932

local SPELL_ECK_BITE    = 55813
local SPELL_ECK_SPIT    = 55814
local SPELL_ECK_BERSERK = 55816

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

-- C++ EVENT_BITE: DoCastVictim; 5s init, rescheduled 8-12s.
local function biteTick(creature, guid)
    creature:CastSpell(nil, SPELL_ECK_BITE)
    schedule(guid, "bite", math.random(8000, 12000), function()
        biteTick(creature, guid)
    end)
end

-- C++ EVENT_SPIT: DoCastVictim; 10s init, rescheduled 6-14s.
local function spitTick(creature, guid)
    creature:CastSpell(nil, SPELL_ECK_SPIT)
    schedule(guid, "spit", math.random(6000, 14000), function()
        spitTick(creature, guid)
    end)
end

-- C++ EVENT_BERSERK: DoCastSelf; 60-90s init, single fire.
local function berserkTick(creature, guid)
    creature:CastSpell(creature, SPELL_ECK_BERSERK)
    local per = timers[guid]
    if per then
        per["berserk"] = nil
    end
end

local function eckEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "bite", 5000, function()
        biteTick(creature, guid)
    end)
    schedule(guid, "spit", 10000, function()
        spitTick(creature, guid)
    end)
    schedule(guid, "berserk", math.random(60000, 90000), function()
        berserkTick(creature, guid)
    end)
end

local function eckLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function eckDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function eckReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_ECK_THE_FEROCIOUS, 1, eckEnterCombat)
RegisterCreatureEvent(ENTRY_ECK_THE_FEROCIOUS, 2, eckLeaveCombat)
RegisterCreatureEvent(ENTRY_ECK_THE_FEROCIOUS, 4, eckDied)
RegisterCreatureEvent(ENTRY_ECK_THE_FEROCIOUS, 23, eckReset)
