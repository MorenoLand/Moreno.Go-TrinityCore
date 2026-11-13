-- The Scarlet Enclave: Dark Rider of Acherus (chaser) — Lua port of
-- src/server/scripts/EasternKingdoms/ScarletEnclave/chapter1.cpp
-- (struct npc_dark_rider_of_acherus : public ScriptedAI;
-- AddSC_the_scarlet_enclave_c1 registers it with the other chapter1
-- scripts; EK loader decl :95 / call :273 per kalimdor_script_loader.cpp).
-- Entry: 28654, verified via spell 52266 SPELL_CALL_DARK_RIDER summon
-- target (round-44 browser check 2026-11-13; 7 locations, no abilities,
-- "The realm of shadows awaits..."). The fightable rider is 28768
-- (npc_ros_dark_rider.lua) — not this file.
-- Eluna creature events: 22 OnSummoned (JustAppeared twin, see below),
-- 4 OnDied, 15 OnSpellHitTarget, 23 OnReset. Driven by CreateLuaEvent
-- timers (maiden_of_virtue precedent); melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady (unused here —
-- the chaser never engages).
-- Ported arms (chapter1.cpp:709-781):
-- - JustAppeared (chapter1.cpp:714-720): TempSummon summoner GUID is
--   stored, EVENT_START_MOVING scheduled in 1s. Go fire site is ev22
--   (IsSummonedBy), NOT ev5: summonTempCreature fires ev23+ev5 inside
--   motionFor BEFORE stamping CreatedByGUID (lua_creature_events.go:
--   136-150), so an ev5 handler would see GetSummonerUnit() == nil;
--   ev22 fires after the stamp with the summoner as the extra argument
--   (:189-196) — the live JustAppeared twin for a TempSummon.
-- - EVENT_START_MOVING (chapter1.cpp:741-747): SetTarget(horse GUID),
--   MoveChase(horse) when the horse resolves, EVENT_DESPAWN_HORSE in
--   5s. The MoveChase bridge arms ScriptChaseTargetGUID and the chase
--   generator runs out of combat (creaturemotion.go:4496, before the
--   InCombat case), so the chase moves. The despawn leg is scheduled
--   unconditionally, matching C++ (it schedules even when the horse
--   lookup fails).
-- - EVENT_DESPAWN_HORSE (chapter1.cpp:748-754): Talk(SAY_DARK_RIDER 0),
--   DoCast(horse, 52267, triggered), EVENT_END_SCRIPT in 2s. C++
--   arm order: Talk, then cast.
-- - EVENT_END_SCRIPT (chapter1.cpp:755-757): DespawnOrUnsummon().
-- - SpellHitTarget (chapter1.cpp:763-768): spell 52267 hitting the
--   horse despawns the horse in 2s (GUID-gated). The same hit also
--   despawns the CASTER via spell_gen_despawn_self (spell_generic.lua:
--   25952/42268/52267) — C++-exact: the spell script despawns the
--   caster, the AI hook despawns the horse.
-- - Reset (chapter1.cpp:722-725): _events.Reset() — cancelTimers.
-- DBC: spell 52267 is seeded (tools/spelldbc_seed/main.go, wago.tools
-- 3.4.0.43659 verdict 2026-11-13: effect 77 SCRIPT_EFFECT, TargetA 25 /
-- TargetB 63, RangeIndex 13 = 0-50000 yd, school physical), so the
-- chaser's triggered cast resolves and ev15 fires.

local ENTRY_DARK_RIDER_CHASER = 28654
local SPELL_DESPAWN_HORSE = 52267
local SAY_DARK_RIDER = 0

local timers = {}
local horseGUIDs = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
    horseGUIDs[guid] = nil
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

local function onDespawnHorse(creature, guid)
    creature:Talk(SAY_DARK_RIDER)
    local horseGUID = horseGUIDs[guid]
    if horseGUID then
        local horse = creature:GetCreatureByGUID(horseGUID)
        if horse then
            creature:CastSpell(horse, SPELL_DESPAWN_HORSE, true)
        end
    end
    schedule(guid, "endScript", 2000, function()
        creature:DespawnOrUnsummon()
    end)
end

local function onStartMoving(creature, guid)
    local horseGUID = horseGUIDs[guid]
    if horseGUID then
        creature:SetTarget(horseGUID)
        local horse = creature:GetCreatureByGUID(horseGUID)
        if horse then
            creature:MoveChase(horse)
        end
    end
    schedule(guid, "despawnHorse", 5000, function()
        onDespawnHorse(creature, guid)
    end)
end

-- C++ npc_dark_rider_of_acherus::JustAppeared (chapter1.cpp:714-720).
local function onSummoned(_, creature, summoner)
    if not summoner then
        return
    end
    local guid = creature:GetGUIDLow()
    horseGUIDs[guid] = summoner:GetGUID()
    schedule(guid, "startMoving", 1000, function()
        onStartMoving(creature, guid)
    end)
end

-- C++ npc_dark_rider_of_acherus::SpellHitTarget (chapter1.cpp:763-768).
local function onSpellHitTarget(_, creature, target, spellId)
    if spellId ~= SPELL_DESPAWN_HORSE or target == nil then
        return
    end
    local horseGUID = horseGUIDs[creature:GetGUIDLow()]
    if horseGUID and target:GetGUID() == horseGUID then
        target:DespawnOrUnsummon(2000)
    end
end

local function onDied(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ npc_dark_rider_of_acherus::Reset (chapter1.cpp:722-725).
local function onReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

RegisterCreatureEvent(ENTRY_DARK_RIDER_CHASER, 22, onSummoned)
RegisterCreatureEvent(ENTRY_DARK_RIDER_CHASER, 15, onSpellHitTarget)
RegisterCreatureEvent(ENTRY_DARK_RIDER_CHASER, 4, onDied)
RegisterCreatureEvent(ENTRY_DARK_RIDER_CHASER, 23, onReset)
