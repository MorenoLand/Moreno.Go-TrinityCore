-- Razorfen Downs: Tuten'kash — Lua port of
-- src/server/scripts/Kalimdor/RazorfenDowns/boss_tuten_kash.cpp (class
-- boss_tuten_kash : CreatureScript("boss_tuten_kash") {
-- boss_tuten_kashAI : public BossAI(creature, DATA_TUTEN_KASH) };
-- GetAI via GetRazorfenDownsAI<boss_tuten_kashAI> (razorfen_downs.h —
-- GetInstanceAI<AI>(obj, "instance_razorfen_downs") gate, DATA_TUTEN_KASH
-- = 0 of RFDDataTypes; RFDScriptName "instance_razorfen_downs",
-- DataHeader "RFD", EncounterCount = 5, map 47/233 gate via
-- instance_razorfen_downs.cpp); AddSC_boss_tuten_kash at end
-- registers the one script; kalimdor loader decl 69 / call 182 per
-- kalimdor_script_loader.cpp — the FIRST "// Razorfen Downs" loader
-- group, right after AddSC_instance_onyxias_lair() closes the
-- Onyxia's Lair block). Whole-server-tree "tuten" grep hits only
-- the RazorfenDowns dir files + loader (sole-source verified).
-- Entry: razorfen_downs.h RFDCreatureIds names NPC_TUTEN_KASH = 7355
-- (used in the Tuten Kash summon event block alongside NPC_TOMB_FIEND
-- 7349 / NPC_TOMB_REAVER 7351; NPC_PLAGUEMAW_THE_ROTTING 7356 is the
-- neighbouring enum entry) — kalecgos check PASSES; creature_template
-- ScriptName binding stays DB-side. lua_scripts/kalimdor/ holds no
-- tuten_kash/razorfen lua (overnight window did not outrun this
-- one). Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4
-- OnDied, 23 OnReset. Timers via CreateLuaEvent; melee is
-- engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. The C++ HasUnitState(UNIT_STATE_CASTING)
-- early-return gates have no live casting-state read bridge (IsCasting is constant-false — Go casts are fire-and-forget) (mal_ganis
-- precedent).
-- Ported arms:
-- - Reset (C++ Reset, event 23): refresh the two self-buff auras —
--   DoCast(me, SPELL_THRASH 8876) and DoCast(me, SPELL_VIRULENT_POISON
--   12254) — each under its C++ !me->HasAura() guard (HasAura bridge
--   exists on the unit object — hyjal_trash/kodo precedents).
-- - Engage (C++ JustEngagedWith): arm Web Spray at init {3s,5s} and
--   Curse of Tuten'kash at init {9s,14s} (the BossAI::JustEngagedWith
--   instance leg is blocked — standing, no instance bridge).
-- - Web Spray (EVENT_WEB_SPRAY): SelectTarget(Random, 0, 100, false)
--   -> randomThreatTargetInRange(creature, 100) — threat-list pick, no
--   Player filter (playerOnly=FALSE explicit in C++); position 0 means
--   no victim exclusion — and only
--   DoCast(target, SPELL_WEB_SPRAY 12252) when the target lacks the
--   aura (C++ !target->HasAura() guard, HasAura bridge exists);
--   reschedules unconditionally at {6s,8s} (C++ Repeat sits outside
--   the if — faithful).
-- - Curse of Tuten'kash (EVENT_CURSE_OF_TUTENKASH): DoCast(me,
--   SPELL_CURSE_OF_TUTENKASH 12255) self-cast, non-triggered; repeat
--   {15s,25s} unconditional.
-- Unmodeled (documented-only, no bridges):
-- - The BossAI ctor legs (DATA_TUTEN_KASH = 0 boss-data registration,
--   _Reset / _JustDied instance-state bookkeeping) — no instance /
--   BossAI-Reset bridges (standing).
-- - The C++ UpdateAI me->HasUnitState(UNIT_STATE_CASTING) gates —
--   mal_ganis precedent, no live casting-state read bridge (IsCasting is constant-false — Go casts are fire-and-forget).

local ENTRY = 7355

local SPELL_THRASH              = 8876
local SPELL_WEB_SPRAY           = 12252
local SPELL_VIRULENT_POISON     = 12254
local SPELL_CURSE_OF_TUTENKASH  = 12255

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

-- Threat-list pick within maxDist (3D, the C++ IsWithinDist semantics
-- for the SelectTarget dist param); playerOnly=FALSE (EXPLICIT in the
-- C++: SelectTarget(Random, 0, 100, false)), so pets/totems are
-- eligible — no Player filter. The bridged, threat-ordered
-- GetAITargets list (lua_creature_events.go:345); offset 0 means no
-- tank exclusion.
local function randomThreatTargetInRange(creature, maxDist)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if not t:IsDead() and creature:GetDistance(t) <= maxDist then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EVENT_WEB_SPRAY (boss_tuten_kash.cpp:81): SelectTarget(Random, 0,
-- 100, false) -> DoCast(target, 12252) under !target->HasAura(); reschedule
-- {6s,8s} unconditional. playerOnly=FALSE explicit — pets/totems are
-- eligible; threat-list pick via GetAITargets (creature HasAura is
-- bridged, lua_objects.go:228).
local function onWebSpray(creature, guid)
    local target = randomThreatTargetInRange(creature, 100)
    if target and not target:HasAura(SPELL_WEB_SPRAY) then
        creature:CastSpell(target, SPELL_WEB_SPRAY)
    end
    schedule(guid, "webspray", math.random(6000, 8000), function()
        onWebSpray(creature, guid)
    end)
end

-- C++ EVENT_CURSE_OF_TUTENKASH: DoCast(me, 12255) self-cast,
-- non-triggered; repeat {15s,25s} unconditional.
local function onCurseOfTutenkash(creature, guid)
    creature:CastSpell(creature, SPELL_CURSE_OF_TUTENKASH)
    schedule(guid, "curse", math.random(15000, 25000), function()
        onCurseOfTutenkash(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "webspray", math.random(3000, 5000), function()
        onWebSpray(creature, guid)
    end)
    schedule(guid, "curse", math.random(9000, 14000), function()
        onCurseOfTutenkash(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ Reset: refresh the two self-buff auras under their HasAura
-- guards (Thrash 8876, Virulent Poison 12254).
local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
    -- C++ Reset: non-triggered self-casts gated on !HasAura
    -- (Reset-only arms, boss_tuten_kash.cpp). Event 23 fires on
    -- death too (engine reset-then-died order) but C++ Reset()
    -- never runs on death, so both are alive-gated.
    if creature:IsAlive() and not creature:HasAura(SPELL_THRASH) then
        creature:CastSpell(creature, SPELL_THRASH)
    end
    if creature:IsAlive() and not creature:HasAura(SPELL_VIRULENT_POISON) then
        creature:CastSpell(creature, SPELL_VIRULENT_POISON)
    end
end

local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)
