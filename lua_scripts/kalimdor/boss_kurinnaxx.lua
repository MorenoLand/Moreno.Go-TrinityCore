-- Ruins of Ahn'Qiraj: Kurinnaxx — Lua port of
-- src/server/scripts/Kalimdor/RuinsOfAhnQiraj/boss_kurinnaxx.cpp
-- (boss_kurinnaxxAI : public BossAI(creature, DATA_KURINNAXX);
-- GetAI via GetAQ20AI<boss_kurinnaxxAI>(creature) (AQ20ScriptName
-- "instance_ruins_of_ahnqiraj" gate); AddSC_boss_kurinnaxx at end
-- registers the one script; kalimdor loader decl 79 / call 192 per
-- kalimdor_script_loader.cpp — first "// Ruins of ahn'qiraj" loader
-- group, followed by rajaxx, moam, buru, ayamiss, ossirian, instance).
-- Whole-server-tree quoted-name grep confirms the cpp as the sole
-- source of "boss_kurinnaxx" (loader decl/call lines only otherwise).
-- Entry: ruins_of_ahnqiraj.h:41 names NPC_KURINNAXX = 15348; the
-- creature_template ScriptName binding stays DB-side. Not GUID-bound in
-- OnCreatureCreate, so no ramstein-strength GUID leg.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 9 OnDamageTaken, 23 OnReset. Timers via CreateLuaEvent; melee is
-- engine-driven in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- The C++ while(ExecuteEvent()) + HasUnitState(UNIT_STATE_CASTING) gate
-- has no casting-state bridge in the Lua API (aeonus precedent —
-- unmodeled).
-- Ported arms (the self-contained in-combat legs):
-- - Engage: arm each timer at its C++ ScheduleEvent cooldown (no Talk
--   in C++ JustEngagedWith — there is no SAY_AGGRO arm here).
-- - Mortal Wound 25646 on the victim (C++ DoCastVictim — jeklik
--   GetVictim + CastSpell convention, non-triggered), init 8s -> 8s.
-- - Sand Trap 25648: C++ SelectTarget(Random, 0, 100, true) casts
--   target->CastSpell(target, 25648, true); nil target falls back to
--   victim->CastSpell(victim, 25648, true). Ported as a bounded random
--   alive player within 100 (anetheron randomPlayerInRange convention,
--   nil -> no cast) with the C++ victim-fallback wired: victim casts
--   on itself when no random player exists. Init {5s,15s} -> {5s,15s}.
-- - Wide Slash 25814 self-cast (C++ DoCast(me)), init 11s -> 11s.
-- - Trash 3391 self-cast (C++ DoCast(me); the enum comment notes it
--   "Should perhaps be triggered by an aura"), fired ONCE at 1s — the
--   C++ EVENT_TRASH arm reschedules EVENT_WIDE_SLASH at 15s instead of
--   itself (faithful C++-verbatim: wide slash's next tick is pushed
--   out to 15s when trash fires).
-- - DamageTaken enrage (event 9): PRE-damage health below 30% ->
--   self-cast 26527, latched by a per-GUID flag (C++ _enraged,
--   reset in Reset's Initialize()).
-- Unmodeled (documented-only, no bridges):
-- - JustDied's Ossirian yell: sCreatureTextMgr->SendChat(Ossirian,
--   SAY_KURINNAXX_DEATH 5, ...) where Ossirian comes from
--   ObjectAccessor::GetCreature(*me, instance->GetGuidData(DATA_OSSIRIAN)).
--   No instance-data bridge (standing), so the Talk arm is not wired.
-- - JustDied's _JustDied() boss-state bookkeeping — instance bridge,
--   standing.
-- - BossAI ctor leg DATA_KURINNAXX (ruins_of_ahnqiraj.h:28 = 0) and
--   _Reset() — instance bridge, standing.
local ENTRY = 15348

local SPELL_MORTAL_WOUND = 25646
local SPELL_SAND_TRAP = 25648
local SPELL_ENRAGE = 26527
local SPELL_TRASH = 3391
local SPELL_WIDE_SLASH = 25814

local timers = {}
local kurinnaxxState = {}

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

local function randomPlayerInRange(creature, maxDist)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() and creature:GetDistance(p) <= maxDist then
            found[#found + 1] = p
        end
    end
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
end

-- C++ EVENT_MORTAL_WOUND: DoCastVictim(25646), non-triggered, init
-- 8s -> 8s.
local function onMortalWound(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_MORTAL_WOUND)
    end
    schedule(guid, "mortalwound", 8000, function()
        onMortalWound(creature, guid)
    end)
end

-- C++ EVENT_SANDTRAP: random alive player within 100 casts on itself
-- (triggered); nil target -> victim casts on itself. Init {5s,15s} ->
-- {5s,15s}.
local function onSandTrap(creature, guid)
    local target = randomPlayerInRange(creature, 100)
    if target then
        target:CastSpell(target, SPELL_SAND_TRAP, true)
    else
        local victim = creature:GetVictim()
        if victim then
            victim:CastSpell(victim, SPELL_SAND_TRAP, true)
        end
    end
    schedule(guid, "sandtrap", math.random(5000, 15000), function()
        onSandTrap(creature, guid)
    end)
end

-- C++ EVENT_WIDE_SLASH: DoCast(me, 25814), init 11s -> 11s.
local function onWideSlash(creature, guid)
    creature:CastSpell(creature, SPELL_WIDE_SLASH)
    schedule(guid, "wideslash", 11000, function()
        onWideSlash(creature, guid)
    end)
end

-- C++ EVENT_TRASH: DoCast(me, 3391), fired once at 1s; it reschedules
-- EVENT_WIDE_SLASH at 15s (C++-verbatim, not a repeat of itself).
local function onTrash(creature, guid)
    creature:CastSpell(creature, SPELL_TRASH)
    schedule(guid, "wideslash", 15000, function()
        onWideSlash(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    kurinnaxxState[guid] = { enraged = false }
    schedule(guid, "mortalwound", 8000, function()
        onMortalWound(creature, guid)
    end)
    schedule(guid, "sandtrap", math.random(5000, 15000), function()
        onSandTrap(creature, guid)
    end)
    schedule(guid, "trash", 1000, function()
        onTrash(creature, guid)
    end)
    schedule(guid, "wideslash", 11000, function()
        onWideSlash(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    kurinnaxxState[guid] = { enraged = false }
end

-- C++ DamageTaken: !_enraged && HealthBelowPct(30) -> DoCast(me,
-- SPELL_ENRAGE 26527), latched. HealthBelowPct reads the PRE-damage
-- health (the damage parameter is unnamed/unused in C++; event 9 fires
-- before damage is applied per engine lua_creature_events.go), so no
-- damage subtraction — unlike moroes, whose C++ uses
-- HealthBelowPctDamaged and correctly subtracts.
local function onDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local state = kurinnaxxState[guid]
    if state == nil or state.enraged then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if creature:GetHealth() * 100 / maxHealth < 30 then
        creature:CastSpell(creature, SPELL_ENRAGE)
        state.enraged = true
    end
end

-- C++ JustDied: _JustDied() + the Ossirian SAY_KURINNAXX_DEATH yell —
-- both instance-bridge legs, unwired; cancel only.
local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY, 23, onReset)
