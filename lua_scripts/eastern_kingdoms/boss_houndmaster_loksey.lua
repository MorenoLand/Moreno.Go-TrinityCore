-- Houndmaster Loksey (Scarlet Monastery, Library wing) — Lua port of
-- src/server/scripts/EasternKingdoms/ScarletMonastery/
-- boss_houndmaster_loksey.cpp (boss_houndmaster_loksey AI only);
-- scarlet_monastery.h names DATA_HOUNDMASTER_LOKSEY which the BossAI
-- constructor consumes.
-- Creature entry: 3974 Houndmaster Loksey (no NPC_ constant exists
-- in the C++ tree — RegisterScarletMonasteryCreatureAI binds the
-- creature_template ScriptName DB-side; entry db.moonwell-cited:
-- db.moonwell.su/?npc=3974 "Houndmaster Loksey" — the 3974 slot
-- that was "disproven" as an adjacent guess for Fairbanks is
-- Loksey himself).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in
-- Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnEnterCombat
-- Talk(SAY_AGGRO 0) + non-triggered DoCast(SPELL_SUMMON_SCARLET_
-- HOUND 17164) (DoCast(spellId) is a self-cast ->
-- creature:CastSpell(creature, ...)) + EVENT_BLOODLUST armed at 20s.
-- EVENT_BLOODLUST: if current health is strictly below 60%
-- (C++ HealthBelowPct(60) — current-health read, golemagg 704fb8e
-- bug class semantics) -> non-triggered DoCastSelf(bloodlust 6742)
-- and re-arm at 60s; otherwise re-arm at 1s (the C++
-- events.Repeat(1s) retry leg). The UpdateAI UNIT_STATE_CASTING
-- queue gate is PORTED — loksey overrides no UpdateAI, so the gate
-- is BossAI::UpdateAI's pre-loop + in-loop CASTING returns
-- (ScriptedCreature.cpp:571-586); the bloodlust arm defers 500ms
-- while creature:IsCasting() (live CastingUntil window,
-- lua_creature_events.go:1950; thalnos precedent), no re-arm
-- consumed, defer before the health read. BossAI::JustEngagedWith
-- pulse/zone arms are PORTED in lokseyEnterCombat; DATA_HOUNDMASTER_
-- LOKSEY encounter bookkeeping still has no instance-script bridge
-- (luaBossAI shim).

local ENTRY_HOUNDMASTER_LOKSEY = 3974

local SAY_AGGRO = 0

local SPELL_SUMMON_SCARLET_HOUND = 17164
local SPELL_BLOODLUST = 6742

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

-- C++ EVENT_BLOODLUST: HealthBelowPct(60) -> DoCastSelf(bloodlust
-- 6742), Repeat(60s); else Repeat(1s). Loksey overrides no UpdateAI,
-- so the gate is BossAI::UpdateAI's pre-loop + in-loop CASTING
-- returns (ScriptedCreature.cpp:571-586) — PORTED: the arm defers
-- 500ms while creature:IsCasting() (live CastingUntil window,
-- lua_creature_events.go:1950; thalnos precedent), no re-arm
-- consumed, defer before the health read.
local function onBloodlust(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "bloodlust", 500, function()
            onBloodlust(creature, guid)
        end)
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth > 0 and creature:GetHealth() < math.floor(maxHealth * 60 / 100) then
        creature:CastSpell(creature, SPELL_BLOODLUST)
        schedule(guid, "bloodlust", 60000, function()
            onBloodlust(creature, guid)
        end)
    else
        schedule(guid, "bloodlust", 1000, function()
            onBloodlust(creature, guid)
        end)
    end
end

local function lokseyEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    creature:SetCombatPulseDelay(5) -- BossAI::JustEngagedWith arm 1 (ScriptedCreature.cpp:540; boss_houndmaster_loksey.cpp:44 base call first)
    creature:DoZoneInCombat() -- BossAI::JustEngagedWith me-form arm (ScriptedCreature.cpp:542; boss_houndmaster_loksey.cpp:44 base call first)
    creature:Talk(SAY_AGGRO)
    creature:CastSpell(creature, SPELL_SUMMON_SCARLET_HOUND)
    schedule(guid, "bloodlust", 20000, function()
        onBloodlust(creature, guid)
    end)
end

local function lokseyLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

local function lokseyDied(event, creature, killer)
    cancelTimers(creature:GetGUIDLow())
end

local function lokseyReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

RegisterCreatureEvent(ENTRY_HOUNDMASTER_LOKSEY, 1, lokseyEnterCombat)
RegisterCreatureEvent(ENTRY_HOUNDMASTER_LOKSEY, 2, lokseyLeaveCombat)
RegisterCreatureEvent(ENTRY_HOUNDMASTER_LOKSEY, 4, lokseyDied)
RegisterCreatureEvent(ENTRY_HOUNDMASTER_LOKSEY, 23, lokseyReset)
