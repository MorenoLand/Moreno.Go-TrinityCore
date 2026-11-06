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
-- UpdateAI CASTING gates PORTED (own UpdateAI pre-loop :98 + in-loop :128 returns, boss_kurinnaxx.cpp): each arm defers 500ms while creature:IsCasting() (live CastingUntil window, lua_creature_events.go:1950), no re-arm consumed; defer precedes victim pick (mortal wound), random pick (sand trap), and each self-cast; the trash one-shot still fires once, seeding wideslash2, after any defer.
-- Ported arms (the self-contained in-combat legs):
-- - Engage: arm each timer at its C++ ScheduleEvent cooldown (no Talk
--   in C++ JustEngagedWith — there is no SAY_AGGRO arm here).
-- - Mortal Wound 25646 on the victim (C++ DoCastVictim — jeklik
--   GetVictim + CastSpell convention, non-triggered), init 8s -> 8s.
-- - Sand Trap 25648: C++ SelectTarget(Random, 0, 100, true) casts
--   target->CastSpell(target, 25648, true); nil target falls back to
--   victim->CastSpell(victim, 25648, true). Ported as a random alive
--   threat-list player within 100 (Random, 0, 100, true), nil -> victim
--   fallback wired: victim casts
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
-- - JustDied's _JustDied() boss-state bookkeeping — PORTED in onDied
--   (SetBossState DATA_KURINNAXX DONE; bridge live,
--   lua_creature_events.go:5917).
-- - BossAI ctor leg DATA_KURINNAXX (ruins_of_ahnqiraj.h:28 = 0) and
--   _Reset() — PORTED: ev1 IN_PROGRESS + ev23 NOT_STARTED legs
--   (see onEnterCombat / onReset).
local ENTRY = 15348
local DATA_KURINNAXX = 0 -- ruins_of_ahnqiraj.h:28
local BOSS_STATE_NOT_STARTED = 0
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_DONE = 3

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

local function randomThreatPlayerInRange(creature, maxDist)
    local targets = creature:GetAITargets()
    local out = {}
    for _, unit in ipairs(targets) do
        if unit:GetObjectType() == "Player" and not unit:IsDead()
                and creature:GetDistance(unit) - creature:GetCombatReach() - 1.5 < maxDist then
            out[#out + 1] = unit
        end
    end
    if #out == 0 then
        return nil
    end
    return out[math.random(#out)]
end

-- C++ EVENT_MORTAL_WOUND: DoCastVictim(25646), non-triggered, init
-- 8s -> 8s.
local function onMortalWound(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "mortalwound", 500, function()
            onMortalWound(creature, guid)
        end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_MORTAL_WOUND)
    end
    schedule(guid, "mortalwound", 8000, function()
        onMortalWound(creature, guid)
    end)
end

-- C++ EVENT_SANDTRAP (boss_kurinnaxx.cpp:109-115): random alive player
-- within 100 self-casts SPELL_SANDTRAP (triggered); nil target -> victim
-- self-casts. Init {5s,15s} -> {5s,15s}. Deliberately NOT switched to
-- player:CastSpell: 25648 spawns the trap under the raid (summon-family
-- effect), and the session cast path executes no SPELL_EFFECT_SUMMON
-- (lua_player.go) while the creature cast path does (boss_ai.go
-- creatureEffectSummonType) — the switch would lose the spawn, so the
-- creature casts on the target/victim instead (caster deviates from C++).
local function onSandTrap(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "sandtrap", 500, function()
            onSandTrap(creature, guid)
        end)
        return
    end
    local target = randomThreatPlayerInRange(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_SAND_TRAP, true)
    else
        local victim = creature:GetVictim()
        if victim then
            creature:CastSpell(victim, SPELL_SAND_TRAP, true)
        end
    end
    schedule(guid, "sandtrap", math.random(5000, 15000), function()
        onSandTrap(creature, guid)
    end)
end

-- C++ EVENT_WIDE_SLASH: DoCast(me, 25814), init 11s -> 11s.
local function onWideSlash(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "wideslash", 500, function()
            onWideSlash(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_WIDE_SLASH)
    schedule(guid, "wideslash", 11000, function()
        onWideSlash(creature, guid)
    end)
end

-- C++ ScheduleEvent is a pure multimap insert (EventMap.cpp:44) — the
-- TRASH arm's 15s wide slash is a SECOND independent 11s line stacked on
-- the 11s init line, so it gets its own timer key (schedule() cancels the
-- key, i.e. RescheduleEvent semantics).
local function onWideSlash2(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "wideslash2", 500, function()
            onWideSlash2(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_WIDE_SLASH)
    schedule(guid, "wideslash2", 11000, function()
        onWideSlash2(creature, guid)
    end)
end

-- C++ EVENT_TRASH: DoCast(me, 3391), fired once at 1s; it seeds the second
-- wide-slash line at 15s (C++-verbatim, not a repeat of itself).
local function onTrash(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "trash", 500, function()
            onTrash(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_TRASH)
    schedule(guid, "wideslash2", 15000, function()
        onWideSlash2(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    -- C++ BossAI::_JustEngagedWith (ScriptedCreature.cpp:527-542; no
    -- JustEngagedWith override in boss_kurinnaxx.cpp, so the base
    -- runs): instance->SetBossState(DATA_KURINNAXX, IN_PROGRESS)
    -- ahead of the pulse / DoZoneInCombat arms below
    -- (creature:SetBossState, lua_creature_events.go:5917). The
    -- CheckRequiredBosses gate is vacuous (no instance override in
    -- instance_ruins_of_ahnqiraj.cpp).
    creature:SetBossState(DATA_KURINNAXX, BOSS_STATE_IN_PROGRESS)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542)
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
    -- C++ Reset _Reset() leg (boss_kurinnaxx.cpp:67;
    -- BossAI::_Reset ScriptedCreature.cpp:499-511: alive-gated and
    -- != DONE-gated; Go fires ev23 on the death path before ev4,
    -- moroes/algalon precedent, so both gates are live here).
    if creature:IsAlive() and creature:GetBossState(DATA_KURINNAXX) ~= BOSS_STATE_DONE then
        creature:SetBossState(DATA_KURINNAXX, BOSS_STATE_NOT_STARTED)
    end
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
    if creature:GetHealth() < math.floor(maxHealth * 30 / 100) then
        creature:CastSpell(creature, SPELL_ENRAGE)
        state.enraged = true
    end
end

-- C++ JustDied (boss_kurinnaxx.cpp:84-88): _JustDied() first, then the
-- Ossirian SAY_KURINNAXX_DEATH yell (instance GetGuidData leg, no
-- bridge — stays out). BossAI::_JustDied (ScriptedCreature.cpp:513-520)
-- writes SetBossState(DATA_KURINNAXX, DONE); the events.Reset /
-- summons / scheduler legs need no bridge.
local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:SetBossState(DATA_KURINNAXX, BOSS_STATE_DONE)
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY, 23, onReset)
