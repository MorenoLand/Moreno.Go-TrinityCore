-- Murmur (Shadow Labyrinth, Auchindoun) — Lua port of
-- src/server/scripts/Outland/Auchindoun/ShadowLabyrinth/boss_murmur.cpp
-- (1 CreatureScript — boss_murmur (BossAI via GetShadowLabyrinthAI,
-- DATA_MURMUR = 3); registered from inside AddSC_boss_murmur();
-- loader decl 39 / call 162 per outland_script_loader.cpp — the
-- "Auchindoun - Shadow Labyrinth" sub-block, immediately after
-- AddSC_boss_grandmaster_vorpil() — verified from the loader this run).
-- Entry: 18708 — DB-side (no NPC_MURMUR constant in
-- shadow_labyrinth.h; ScriptName bindings are DB-side as usual).
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_murmur" hits boss_murmur.cpp (+ the loader decl/call
-- lines) only. No boss_murmur lua existed.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat,
-- 4 OnDied, 23 OnReset.
-- Ported arms (C++-exact for all modeled arms):
-- boss_murmurAI::Reset — schedules EVENT_SONIC_BOOM (30s),
-- EVENT_MURMURS_TOUCH (8-20s rand), EVENT_RESONANCE (5s),
-- EVENT_MAGNETIC_PULL (15-30s rand), heroic-only
-- EVENT_THUNDERING_STORM (15s) + EVENT_SONIC_SHOCK (10s);
-- sets health to 40% max (creature:SetHealth bridged,
-- lua_creature_events.go:187) — ResetPlayerDamageReq rides the
-- no-player-damage-req bridge — documented-only (event 23).
-- EVENT_SONIC_BOOM — Talk(EMOTE_SONIC_BOOM 0) + DoCast(me,
-- SPELL_SONIC_BOOM_CAST 33923) 2-arg (non-triggered); 30s init,
-- 30s repeat; ALSO schedules EVENT_RESONANCE 1500ms (unconditional
-- — it re-arms resonance even when the melee gate killed it, per
-- C++ :131).
-- EVENT_MURMURS_TOUCH — SelectTarget(Random, 0, 80.0f, true) →
-- DoCast(target, SPELL_MURMURS_TOUCH 33711) 2-arg (non-triggered);
-- 8-20s init, 25-35s repeat. Threat-list pool via GetAITargets
-- (the bridged threat list; midnight precedent — C++ iterates the
-- threat list, never the instance map), playerOnly=true honored by
-- the "Player" filter (illidan victim:GetObjectType() convention),
-- 80.0f band via GetDistance2d.
-- EVENT_RESONANCE — if victim NOT within melee range: DoCast(me,
-- SPELL_RESONANCE 33657) 2-arg (non-triggered), 5s re-arm; if the
-- victim IS in melee range the event is CONSUMED with no re-arm
-- (C++ :136-141 — no ScheduleEvent on the fallthrough path).
-- gate is creature:IsWithinMeleeRange (bridged 2026-10-24).
-- EVENT_MAGNETIC_PULL — SelectTarget(Random, 0, 0.0f, true) →
-- DoCast(target, SPELL_MAGNETIC_PULL 33689) 2-arg (non-triggered);
-- 15-30s init, 15-30s repeat; NO target → 500ms fail-fast retry
-- (C++ :145 — re-arms 500ms, not the full cycle).
-- EVENT_THUNDERING_STORM (heroic) — DoCastAOE(SPELL_THUNDERING_
-- STORM 39365, true) → creature:CastSpell(creature, spell, true)
-- (triggered self-cast; ICC-strip DoCastAOE convention); 15s init,
-- 15s repeat; creature:IsHeroic() bridged
-- (lua_creature_events.go:203).
-- EVENT_SONIC_SHOCK (heroic) — SelectTarget(Random, 0, 20.0f,
-- false) → DoCast(target, SPELL_SONIC_SHOCK 38797) 2-arg
-- (non-triggered); playerOnly=false so NO player filter; 10s init,
-- 10-20s repeat.
-- All timers cancelled on 2/4/23 (gargolmar precedent).
-- DOCUMENTED-ONLY (in this header; only entry 18708 registered):
-- SetCombatMovement(false) — PORTED in murmurEnterCombat (boss_murmur.cpp:64
-- ctor arm; bridge live, lua_creature_events.go:1301).
-- The UpdateAI casting gate (HasUnitState(UNIT_STATE_CASTING)
-- early-out, and the per-event post-cast return) — creature
-- IsCasting is a stub returning false (lua_objects.go:330) — no
-- real bridge — documented-only.
-- The melee leg ("select nearest most aggro target if top aggro too
-- far" + DoMeleeAttackIfReady) — engine-driven melee is a genuine
-- gap (aran precedent) — documented-only.
-- spell_murmur_sonic_boom (33923 → CastSpell(33666) triggered),
-- spell_murmur_sonic_boom_effect (33666 damage = 80% of target max
-- health), spell_murmur_thundering_storm (25-100y target band) —
-- all SpellScript — no SpellScript bridge (instance_ulduar
-- precedent) — documented-only.

local ENTRY_MURMUR = 18708 -- DB-side; no NPC_MURMUR in shadow_labyrinth.h

local SPELL_RESONANCE = 33657 -- boss_murmur.cpp:32
local SPELL_MAGNETIC_PULL = 33689 -- boss_murmur.cpp:33
local SPELL_SONIC_SHOCK = 38797 -- boss_murmur.cpp:34
local SPELL_THUNDERING_STORM = 39365 -- boss_murmur.cpp:35
local SPELL_SONIC_BOOM_CAST = 33923 -- boss_murmur.cpp:36
local SPELL_MURMURS_TOUCH = 33711 -- boss_murmur.cpp:38

local EMOTE_SONIC_BOOM = 0 -- boss_murmur.cpp:40

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

-- Append-on-key (anzu scheduleOnce precedent): C++ EventMap::ScheduleEvent
-- is a plain multimap insert — sonic boom's 1500ms resonance schedule
-- ADDS a resonance chain instead of replacing the pending one.
local appendNonce = 0
local function scheduleAppend(guid, key, delay, fn)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    appendNonce = appendNonce + 1
    per[key .. "#" .. appendNonce] = CreateLuaEvent(fn, delay)
end

-- C++ SelectTarget(Random, ...) picks from the threat list.
-- GetAITargets (lua_creature_events.go:345) is the bridged threat
-- list (midnight precedent); GetDistance2d resolves via the generic
-- fallback (engine/scripting/object.go:158).
local function randomThreatTarget(creature, maxDist, playersOnly)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if (not playersOnly) or t:GetObjectType() == "Player" then
            if maxDist <= 0 or creature:GetDistance2d(t) <= maxDist then
                candidates[#candidates + 1] = t
            end
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- Forward declaration: the sonic-boom arm schedules resonance, and
-- resonance's re-arm references its own tick (cyclic chain —
-- tharon_ja precedent).
local resonanceTick

-- C++ EVENT_RESONANCE (:135-142): if the victim is NOT within melee
-- range, DoCast(me, SPELL_RESONANCE) 2-arg (non-triggered) and
-- re-arm 5s. If the victim IS in melee range the event dies with NO
-- re-arm (C++-exact fallthrough — not a reset to 5s).
resonanceTick = function(creature, guid)
    local victim = creature:GetVictim()
    if victim and not creature:IsWithinMeleeRange(victim) then
        creature:CastSpell(creature, SPELL_RESONANCE)
        scheduleAppend(guid, "resonance", 5000, function()
            resonanceTick(creature, guid)
        end)
    end
end

-- C++ EVENT_SONIC_BOOM (:126-132): Talk(EMOTE_SONIC_BOOM) +
-- DoCast(me, SPELL_SONIC_BOOM_CAST) 2-arg (non-triggered); re-arm
-- 30s; unconditionally schedules EVENT_RESONANCE at 1500ms.
local function sonicBoomTick(creature, guid)
    creature:Talk(EMOTE_SONIC_BOOM)
    creature:CastSpell(creature, SPELL_SONIC_BOOM_CAST)
    schedule(guid, "sonic_boom", 30000, function()
        sonicBoomTick(creature, guid)
    end)
    scheduleAppend(guid, "resonance", 1500, function()
        resonanceTick(creature, guid)
    end)
end

-- C++ EVENT_MURMURS_TOUCH (:133-135): SelectTarget(Random, 0, 80.0f,
-- true) → DoCast(target, SPELL_MURMURS_TOUCH) 2-arg (non-triggered);
-- re-arm randtime(25s,35s).
local function murmursTouchTick(creature, guid)
    local target = randomThreatTarget(creature, 80.0, true)
    if target then
        creature:CastSpell(target, SPELL_MURMURS_TOUCH)
    end
    schedule(guid, "murmurs_touch", {25000, 35000}, function()
        murmursTouchTick(creature, guid)
    end)
end

-- C++ EVENT_MAGNETIC_PULL (:143-150): SelectTarget(Random, 0, 0.0f,
-- true) → DoCast(target, SPELL_MAGNETIC_PULL) 2-arg (non-triggered),
-- re-arm randtime(15s,30s); NO target → 500ms fail-fast retry.
local function magneticPullTick(creature, guid)
    local target = randomThreatTarget(creature, 0, true)
    if target then
        creature:CastSpell(target, SPELL_MAGNETIC_PULL)
        schedule(guid, "magnetic_pull", {15000, 30000}, function()
            magneticPullTick(creature, guid)
        end)
    else
        schedule(guid, "magnetic_pull", 500, function()
            magneticPullTick(creature, guid)
        end)
    end
end

-- C++ EVENT_THUNDERING_STORM (:151-154, heroic): DoCastAOE(
-- SPELL_THUNDERING_STORM, true) — triggered self-cast; 15s cycle.
local function thunderingStormTick(creature, guid)
    creature:CastSpell(creature, SPELL_THUNDERING_STORM, true)
    schedule(guid, "thundering_storm", 15000, function()
        thunderingStormTick(creature, guid)
    end)
end

-- C++ EVENT_SONIC_SHOCK (:155-158, heroic): SelectTarget(Random, 0,
-- 20.0f, false) → DoCast(target, SPELL_SONIC_SHOCK) 2-arg
-- (non-triggered); playerOnly=false → no player filter; re-arm
-- randtime(10s,20s).
local function sonicShockTick(creature, guid)
    local target = randomThreatTarget(creature, 20.0, false)
    if target then
        creature:CastSpell(target, SPELL_SONIC_SHOCK)
    end
    schedule(guid, "sonic_shock", {10000, 20000}, function()
        sonicShockTick(creature, guid)
    end)
end

-- C++ boss_murmurAI::Reset (:98-117): _Reset() (instance leg —
-- documented-only) + schedules EVENT_SONIC_BOOM 30s,
-- EVENT_MURMURS_TOUCH 8-20s rand, EVENT_RESONANCE 5s,
-- EVENT_MAGNETIC_PULL 15-30s rand, heroic-only EVENT_THUNDERING_
-- STORM 15s + EVENT_SONIC_SHOCK 10s; sets health to 40% max
-- (CountPctFromMaxHealth(40) → SetHealth — bridged).
local function scheduleReset(creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:SetHealth(math.floor(creature:GetMaxHealth() * 40 / 100))
    schedule(guid, "sonic_boom", 30000, function()
        sonicBoomTick(creature, guid)
    end)
    schedule(guid, "murmurs_touch", {8000, 20000}, function()
        murmursTouchTick(creature, guid)
    end)
    schedule(guid, "resonance", 5000, function()
        resonanceTick(creature, guid)
    end)
    schedule(guid, "magnetic_pull", {15000, 30000}, function()
        magneticPullTick(creature, guid)
    end)
    if creature:IsHeroic() then
        schedule(guid, "thundering_storm", 15000, function()
            thunderingStormTick(creature, guid)
        end)
        schedule(guid, "sonic_shock", 10000, function()
            sonicShockTick(creature, guid)
        end)
    end
end

-- C++ schedules in Reset, not JustEngagedWith; event 23 (OnReset)
-- is the C++-exact init point. Event 1 re-runs the same idempotent
-- schedule so the first pull works even if the engine never fired
-- 23 for the initial spawn (schedule() replaces per-key timers).
local function murmurReset(event, creature)
    scheduleReset(creature)
end

local function murmurEnterCombat(event, creature, target)
    creature:SetCombatMovement(false) -- BossAI ctor arm, boss_murmur.cpp:64 (ranged AI stands and casts)
    scheduleReset(creature)
end

local function murmurLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function murmurDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_MURMUR, 1, murmurEnterCombat)
RegisterCreatureEvent(ENTRY_MURMUR, 2, murmurLeaveCombat)
RegisterCreatureEvent(ENTRY_MURMUR, 4, murmurDied)
RegisterCreatureEvent(ENTRY_MURMUR, 23, murmurReset)
