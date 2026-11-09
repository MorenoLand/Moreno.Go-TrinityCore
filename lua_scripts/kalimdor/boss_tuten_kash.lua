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
-- early-return gates PORTED (own pre-loop + in-loop CASTING returns,
-- boss_tuten_kash.cpp; each timer arm defers 500ms while
-- creature:IsCasting(), no re-arm consumed — meathook precedent; the
-- stale constant-false claim is retracted, IsCasting is live).
-- Ported arms:
-- - Reset (C++ Reset, event 23): refresh the two self-buff auras —
--   DoCast(me, SPELL_THRASH 8876) and DoCast(me, SPELL_VIRULENT_POISON
--   12254) — each under its C++ !me->HasAura() guard (HasAura bridge
--   exists on the unit object — hyjal_trash/kodo precedents).
-- - Engage (C++ JustEngagedWith): arm Web Spray at init {3s,5s} and
--   Curse of Tuten'kash at init {9s,14s} (BossAI::JustEngagedWith
--   pulse + me-DoZone + SetBossState(DATA_TUTEN_KASH, IN_PROGRESS)
--   PORTED, bridge live lua_creature_events.go:5917; the
--   CheckRequiredBosses SEQUENCE_BREAK leg stays blocked).
-- - Death (C++ JustDied, _JustDied): SetBossState(DATA_TUTEN_KASH,
--   DONE) PORTED in onDied.
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
-- - The BossAI ctor DATA_TUTEN_KASH = 0 registration is the boss id
--   above; _Reset / _JustDied instance bookkeeping beyond the
--   SetBossState legs (events.Reset / summons.DespawnAll /
--   scheduler.CancelAll) needs no bridge (tick-driven AIs; the
--   per-file cancelTimers covers the timer half).
-- - The C++ UpdateAI CASTING gates are PORTED in onWebSpray /
--   onCurseOfTutenkash below (500ms IsCasting defer).

local ENTRY = 7355

local DATA_TUTEN_KASH = 0 -- razorfen_downs.h:31
local BOSS_STATE_NOT_STARTED = 0
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_DONE = 3
-- EncounterState (InstanceScript.h:68-76).
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
        if not t:IsDead() and creature:GetDistance(t) - creature:GetCombatReach() - (t:GetObjectType() == "Player" and 1.5 or t:GetCombatReach()) < maxDist then
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
    if creature:IsCasting() then
        schedule(guid, "webspray", 500, function()
            onWebSpray(creature, guid)
        end)
        return
    end
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
    if creature:IsCasting() then
        schedule(guid, "curse", 500, function()
            onCurseOfTutenkash(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_CURSE_OF_TUTENKASH)
    schedule(guid, "curse", math.random(15000, 25000), function()
        onCurseOfTutenkash(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    -- C++ BossAI::_JustEngagedWith (ScriptedCreature.cpp:527-542,
    -- base-first in boss_tuten_kash.cpp:54-56): instance->SetBossState
    -- (DATA_TUTEN_KASH, IN_PROGRESS) ahead of the pulse / DoZoneInCombat
    -- arms below (creature:SetBossState, lua_creature_events.go:5917).
    creature:SetBossState(DATA_TUTEN_KASH, BOSS_STATE_IN_PROGRESS)
    creature:SetCombatPulseDelay(5) -- BossAI::JustEngagedWith arm (ScriptedCreature.cpp:540; base-first in boss_tuten_kash.cpp:54-56)
    creature:DoZoneInCombat() -- BossAI::JustEngagedWith me-form arm (ScriptedCreature.cpp:542; instance/setActive/ScheduleTasks legs out)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "webspray", math.random(3000, 5000), function()
        onWebSpray(creature, guid)
    end)
    schedule(guid, "curse", math.random(9000, 14000), function()
        onCurseOfTutenkash(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ Reset: _Reset() first (boss_tuten_kash.cpp:45-47), then refresh
-- the two self-buff auras under their HasAura guards (Thrash 8876,
-- Virulent Poison 12254). The _Reset() SetBossState(NOT_STARTED) leg
-- (BossAI::_Reset ScriptedCreature.cpp:499-511) is alive-gated and
-- != DONE-gated (Go fires ev23 on the death path before ev4,
-- moroes/algalon precedent, so both gates are live here).
local function onReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    if creature:IsAlive() and creature:GetBossState(DATA_TUTEN_KASH) ~= BOSS_STATE_DONE then
        creature:SetBossState(DATA_TUTEN_KASH, BOSS_STATE_NOT_STARTED)
    end
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
    cancelTimers(creature:GetGUIDLow())
    -- C++ JustDied (boss_tuten_kash.cpp:61-63): _JustDied() only —
    -- BossAI::_JustDied (ScriptedCreature.cpp:513-520) writes
    -- SetBossState(DATA_TUTEN_KASH, DONE); the events.Reset /
    -- summons / scheduler legs need no bridge.
    creature:SetBossState(DATA_TUTEN_KASH, BOSS_STATE_DONE)
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)
