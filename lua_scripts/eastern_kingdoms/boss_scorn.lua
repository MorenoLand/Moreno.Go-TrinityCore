-- Scorn (Scarlet Monastery, Graveyard wing, Scourge Invasion) — Lua port of
-- src/server/scripts/EasternKingdoms/ScarletMonastery/boss_scorn.cpp
-- (boss_scorn AI only); scarlet_monastery.h names DATA_SCORN which the
-- BossAI constructor consumes.
-- Creature entry: 14693 Scorn (no NPC_ constant exists in the C++ tree —
-- RegisterScarletMonasteryCreatureAI binds the creature_template ScriptName
-- DB-side; entry db.moonwell-cited: db.moonwell.su/?npc=14693 "Scorn",
-- level 34 elite undead, "may be spawned via a script", matching the
-- Scourge Invasion graveyard spawn).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in
-- Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnEnterCombat arms
-- EVENT_LICH_SLAP at 45s, EVENT_FROSTBOLT_VOLLEY at 30s,
-- EVENT_MIND_FLAY at 30s, EVENT_FROST_NOVA at 30s (JustEngagedWith
-- legs only — the BossAI::JustEngagedWith pulse/zone arms are
-- PORTED in scornEnterCombat; DATA_SCORN encounter bookkeeping
-- still has no instance-script bridge, luaBossAI shim).
-- All four arms are non-triggered DoCastVictim -> creature:GetVictim()
-- + creature:CastSpell(victim, ...) (jeklik convention); a nil victim
-- tick keeps the schedule and re-arms. Lich Slap re-arms at 45s,
-- Frostbolt Volley and Mind Flay re-arm at 20s, Frost Nova re-arms at
-- 15s (events.Repeat legs — no rescheduling jitter in C++).
-- No Talk lines exist in the C++ (no SAY enum); the UpdateAI
-- UNIT_STATE_CASTING queue gate is PORTED — scorn overrides no
-- UpdateAI, so the gate is BossAI::UpdateAI's pre-loop + in-loop
-- CASTING returns (ScriptedCreature.cpp:571-586); all four timer arms
-- defer 500ms while creature:IsCasting() (live CastingUntil window,
-- lua_creature_events.go:1950; thalnos precedent), no re-arm consumed,
-- defer before the victim pick.

local ENTRY_SCORN = 14693

local SPELL_LICH_SLAP = 28873
local SPELL_FROSTBOLT_VOLLEY = 8398
local SPELL_MINDFLAY = 17313
local SPELL_FROSTNOVA = 15531

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

local function castOnVictim(creature, spellId)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, spellId)
    end
end

-- BossAI::UpdateAI CASTING gate PORTED as above (defer before victim pick).
local function onLichSlap(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "lichSlap", 500, function()
            onLichSlap(creature, guid)
        end)
        return
    end
    castOnVictim(creature, SPELL_LICH_SLAP)
    schedule(guid, "lichSlap", 45000, function()
        onLichSlap(creature, guid)
    end)
end

-- BossAI::UpdateAI CASTING gate PORTED as above (defer before victim pick).
local function onFrostboltVolley(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "frostboltVolley", 500, function()
            onFrostboltVolley(creature, guid)
        end)
        return
    end
    castOnVictim(creature, SPELL_FROSTBOLT_VOLLEY)
    schedule(guid, "frostboltVolley", 20000, function()
        onFrostboltVolley(creature, guid)
    end)
end

-- BossAI::UpdateAI CASTING gate PORTED as above (defer before victim pick).
local function onMindFlay(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "mindFlay", 500, function()
            onMindFlay(creature, guid)
        end)
        return
    end
    castOnVictim(creature, SPELL_MINDFLAY)
    schedule(guid, "mindFlay", 20000, function()
        onMindFlay(creature, guid)
    end)
end

-- BossAI::UpdateAI CASTING gate PORTED as above (defer before victim pick).
local function onFrostNova(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "frostNova", 500, function()
            onFrostNova(creature, guid)
        end)
        return
    end
    castOnVictim(creature, SPELL_FROSTNOVA)
    schedule(guid, "frostNova", 15000, function()
        onFrostNova(creature, guid)
    end)
end

local function scornEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:SetCombatPulseDelay(5) -- BossAI::JustEngagedWith arm 1 (ScriptedCreature.cpp:540; boss_scorn.cpp:44 base call first)
    creature:DoZoneInCombat() -- BossAI::JustEngagedWith me-form arm (ScriptedCreature.cpp:542; boss_scorn.cpp:44 base call first)
    schedule(guid, "lichSlap", 45000, function()
        onLichSlap(creature, guid)
    end)
    schedule(guid, "frostboltVolley", 30000, function()
        onFrostboltVolley(creature, guid)
    end)
    schedule(guid, "mindFlay", 30000, function()
        onMindFlay(creature, guid)
    end)
    schedule(guid, "frostNova", 30000, function()
        onFrostNova(creature, guid)
    end)
end

local function scornLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function scornDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function scornReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_SCORN, 1, scornEnterCombat)
RegisterCreatureEvent(ENTRY_SCORN, 2, scornLeaveCombat)
RegisterCreatureEvent(ENTRY_SCORN, 4, scornDied)
RegisterCreatureEvent(ENTRY_SCORN, 23, scornReset)
