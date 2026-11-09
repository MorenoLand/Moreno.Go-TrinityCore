-- Koralon the Flame Watcher (Vault of Archavon) — Lua port of
-- src/server/scripts/Northrend/VaultOfArchavon/boss_koralon.cpp
-- (230 lines; 4 scripts — boss_koralon, spell_koralon_meteor_fists,
-- spell_koralon_meteor_fists_damage, spell_flame_warder_meteor_fists,
-- all registered from inside AddSC_boss_koralon() — loader decl 141
-- / call 336, the "// Vault of Archavon" block in
-- AddNorthrendScripts(), northrend_script_loader.cpp).
-- Entry: 35013 Koralon the Flame Watcher (vault_of_archavon.h
-- NPC_KORALON, line 40; DATA_KORALON boss slot). Zero Talk lines in
-- the C++ — none ported.
-- The stale "no-timer-bridge" doctrine is superseded by the
-- CreateLuaEvent bridge (engine/scripting/runtime.go:798 — the
-- archavon / maiden_of_virtue precedent): module-local GUID-keyed
-- timers table, wipe on events 2 (leave combat), 4 (death), 23
-- (evade).
-- creature:CastSpell(creature, spell) = DoCastSelf / DoCast(me)
-- (2-arg = non-triggered; no 3rd-arg casts exist in this AI). All
-- delays below are fixed ms (C++ uses plain 15s/20s/30s/45s/75s
-- literals), so plain ms delays carry the C++-exact timing — no
-- {minMs, maxMs} randtime table.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset.
-- Ported arms (C++-exact for all modeled arms; line refs are
-- boss_koralon.cpp):
-- boss_koralonAI::JustEngagedWith (:54-68) — the immediate DoCast(me,
-- SPELL_BURNING_FURY 66721) (:59, non-triggered) fires once before
-- the timers are scheduled, then seeds the four combat timers;
-- BossAI::JustEngagedWith(who) is LAST in C++: _JustEngagedWith
-- (ScriptedCreature.cpp:527-542) — SetBossState(DATA_KORALON 2,
-- IN_PROGRESS) PORTED tick 262 via creature:SetBossState
-- (lua_creature_events.go:5917), then pulse + me-zone at the handler
-- tail; the CheckRequiredBosses gate has no bridge and stays out.
-- BossAI::JustDied (no override in this file): _JustDied()
-- (ScriptedCreature.cpp:513-520) — SetBossState(DATA_KORALON 2, DONE)
-- PORTED tick 262 in koralonJustDied, with the instance
-- SetBossState-override DONE legs (instance_vault_of_archavon.cpp:
-- 57-83: KoralonDeath record + DoCastSpellOnPlayers(68308)) in the
-- same handler, C++ order (state first, then the override legs).
-- BossAI::Reset (no override in this file): _Reset()
-- (ScriptedCreature.cpp:499-511) — SetBossState(DATA_KORALON 2,
-- NOT_STARTED), != DONE-gated, PORTED tick 262 in koralonReset
-- (new ev23, split from ev2 — leave-combat does not call Reset).
-- EVENT_BURNING_FURY (:83-86) — DoCast(me, 66721) (:84,
-- non-triggered) -> CastSpell(creature, 66721); re-arm 20s (:85).
-- EVENT_BURNING_BREATH (:87-90) — DoCast(me, SPELL_BURNING_BREATH
-- 66665) (:88, non-triggered) -> CastSpell(creature, 66665); 15s
-- init (:62) / 45s re-arm (:89).
-- EVENT_METEOR_FISTS (:91-94) — DoCast(me, SPELL_METEOR_FISTS
-- 66725) (:92, non-triggered) -> CastSpell(creature, 66725); 75s
-- init (:63) / 45s re-arm (:93).
-- EVENT_FLAME_CINDER (:95-98) — DoCast(me, SPELL_FLAME_CINDER_A
-- 66684) (:96, non-triggered) -> CastSpell(creature, 66684); re-arm
-- 30s (:97). SPELL_FLAME_CINDER_B 66681 sits in the enum but is never
-- cast anywhere in the file.
-- spell_koralon_meteor_fists AuraScript (:133-159; OnEffectProc
-- SPELL_AURA_DUMMY hook firing SPELL_METEOR_FISTS_DAMAGE 66765) —
-- PORTED (meteorFistsProc, RegisterAuraEvent(66725, 6)): the
-- beast-mark precedent (utgarde_pinnacle.lua) — fireAuraProc
-- (lua_aura.go) passes (event, aura, target, actor, procTarget,
-- damage); target is the aura holder (Koralon), procTarget is
-- eventInfo.GetProcTarget(); PreventDefaultAction = return true; the
-- aurEff trigger arg = triggered cast. No CheckProc gates in C++ —
-- the engine proc gates (DBC proc flags on 66725) decide when it
-- fires. Binding rides the DB spell_script_names mapping of the
-- loader name to the 66725 aura spell. DORMANT until Spell.dbc ships
-- the 66725 aura + proc records.
-- DOCUMENTED DEVIATIONS:
-- Cast-state gates (2026-11-04): the UpdateAI CASTING gates
-- (pre-check :77 + per-event :102 return) are PORTED as the 500ms
-- IsCasting defer on all four timer arms; the stale hodir 'no
-- bridge' claim is retracted (IsCasting live,
-- lua_creature_events.go:1950).
-- DOCUMENTED-ONLY (in this header; entry 35013 is the sole
-- registration):
-- spell_koralon_meteor_fists_damage (SpellScript :161-196; split
-- damage over chain targets) — no SpellScript bridge; the 66765 casts
-- from meteorFistsProc therefore do not split.
-- spell_flame_warder_meteor_fists (AuraScript :198-225; firing
-- SPELL_FW_METEOR_FISTS_DAMAGE 66809) — no SpellScript/AuraScript
-- bridge, and the warder aura spell id is not named in the C++ file
-- (no spell_script_names data in-tree to resolve the loader binding),
-- so no RegisterAuraEvent binding can be written without guessing.
-- DoMeleeAttackIfReady() (:103) — no bridge.

local ENTRY_KORALON = 35013
local DATA_KORALON = 2

local SPELL_BURNING_BREATH = 66665
local SPELL_BURNING_FURY = 66721
local SPELL_FLAME_CINDER_A = 66684
local SPELL_METEOR_FISTS = 66725
local SPELL_METEOR_FISTS_DAMAGE = 66765

local SPELL_EARTH_WIND_FIRE_ACHIEVEMENT_CHECK = 68308

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

-- C++ EVENT_BURNING_FURY (:83-86): DoCast(me, SPELL_BURNING_FURY),
-- 2-arg = non-triggered; re-arm 20s.
local function burningFuryTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "burningfury", 500, function()
            burningFuryTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_BURNING_FURY)
    schedule(guid, "burningfury", 20000, function()
        burningFuryTick(creature, guid)
    end)
end

-- C++ EVENT_BURNING_BREATH (:87-90): DoCast(me,
-- SPELL_BURNING_BREATH), 2-arg = non-triggered; re-arm 45s.
local function burningBreathTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "burningbreath", 500, function()
            burningBreathTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_BURNING_BREATH)
    schedule(guid, "burningbreath", 45000, function()
        burningBreathTick(creature, guid)
    end)
end

-- C++ EVENT_METEOR_FISTS (:91-94): DoCast(me, SPELL_METEOR_FISTS),
-- 2-arg = non-triggered; re-arm 45s.
local function meteorFistsTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "meteorfists", 500, function()
            meteorFistsTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_METEOR_FISTS)
    schedule(guid, "meteorfists", 45000, function()
        meteorFistsTick(creature, guid)
    end)
end

-- C++ EVENT_FLAME_CINDER (:95-98): DoCast(me, SPELL_FLAME_CINDER_A),
-- 2-arg = non-triggered; re-arm 30s.
local function flameCinderTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "flamecinder", 500, function()
            flameCinderTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_FLAME_CINDER_A)
    schedule(guid, "flamecinder", 30000, function()
        flameCinderTick(creature, guid)
    end)
end

-- C++ boss_koralonAI::JustEngagedWith (:54-68): the immediate
-- DoCast(me, SPELL_BURNING_FURY) (:59) fires once before the timers
-- are scheduled (20s / 15s / 75s / 30s init); the BossAI base arms
-- (pulse + me-zone) ride the handler tail below (base call last
-- in C++); instance legs stay out.
local function koralonEnterCombat(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    creature:CastSpell(creature, SPELL_BURNING_FURY)
    schedule(guid, "burningfury", 20000, function()
        burningFuryTick(creature, guid)
    end)
    schedule(guid, "burningbreath", 15000, function()
        burningBreathTick(creature, guid)
    end)
    schedule(guid, "meteorfists", 75000, function()
        meteorFistsTick(creature, guid)
    end)
    schedule(guid, "flamecinder", 30000, function()
        flameCinderTick(creature, guid)
    end)
    -- BossAI::JustEngagedWith(who) is LAST in C++ (after the ScheduleEvents):
    -- _JustEngagedWith (ScriptedCreature.cpp:527-542) — SetBossState
    -- (DATA_KORALON 2, IN_PROGRESS) first, then SetCombatPulseDelay(5)
    -- + me-form DoZoneInCombat; the CheckRequiredBosses gate stays out.
    creature:SetBossState(DATA_KORALON, 1)
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542)
end

-- Shared timer wipe for events 2/23/4 (the svala / yogg_saron
-- convention).
local function koralonCancelCombatTimers(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ BossAI::JustDied (no override in boss_koralon.cpp): _JustDied()
-- (ScriptedCreature.cpp:513-520) — events.Reset() + SetBossState
-- (DATA_KORALON 2, DONE); then the instance SetBossState override
-- (instance_vault_of_archavon.cpp:57-83) records KoralonDeath
-- (GameTime::GetGameTime, seconds — os.time() here) and casts
-- SPELL_EARTH_WIND_FIRE_ACHIEVEMENT_CHECK 68308 on every instance
-- player (player->CastSpell(player, spell, true), no alive gate —
-- the loken GetPlayersInWorld idiom; the death time is banked in
-- instanceData.koralonDeath for the criteria check).
local function koralonJustDied(event, creature)
    cancelTimers(creature:GetGUIDLow())
    creature:SetBossState(DATA_KORALON, 3)
    local mapObj = creature:GetMap()
    if mapObj then
        local data = mapObj:GetInstanceData()
        if data then
            data.koralonDeath = os.time()
        end
    end
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId then
            p:CastSpell(p, SPELL_EARTH_WIND_FIRE_ACHIEVEMENT_CHECK, true)
        end
    end
end

-- C++ BossAI::Reset (no override in boss_koralon.cpp): _Reset()
-- (ScriptedCreature.cpp:499-511) — events.Reset() + SetBossState
-- (DATA_KORALON 2, NOT_STARTED) only when the stored state != DONE.
-- Split from ev2 (the keleseth/svala precedent): ev2 leave-combat
-- does not call Reset in C++, so it stays wipe-only.
local function koralonReset(event, creature)
    if creature:GetBossState(DATA_KORALON) ~= 3 then
        creature:SetBossState(DATA_KORALON, 0)
    end
    cancelTimers(creature:GetGUIDLow())
end

RegisterCreatureEvent(ENTRY_KORALON, 1, koralonEnterCombat)
RegisterCreatureEvent(ENTRY_KORALON, 2, koralonCancelCombatTimers)
RegisterCreatureEvent(ENTRY_KORALON, 4, koralonJustDied)
RegisterCreatureEvent(ENTRY_KORALON, 23, koralonReset)

-- C++ spell_koralon_meteor_fists_AuraScript::TriggerFists
-- (boss_koralon.cpp:144-148): PreventDefaultAction() +
-- GetTarget()->CastSpell(eventInfo.GetProcTarget(),
-- SPELL_METEOR_FISTS_DAMAGE 66765, aurEff). Hook args (event, aura,
-- target, actor, procTarget, damage) per fireAuraProc (lua_aura.go);
-- target is the aura holder, procTarget the proc target.
local function meteorFistsProc(event, aura, target, actor, procTarget, damage)
    if target == nil or procTarget == nil then
        return false
    end
    target:CastSpell(procTarget, SPELL_METEOR_FISTS_DAMAGE, true)
    return true
end

RegisterAuraEvent(SPELL_METEOR_FISTS, 6, meteorFistsProc)
