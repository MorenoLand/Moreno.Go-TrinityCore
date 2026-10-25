-- Terestian Illhoof (Karazhan) — Lua port of
-- src/server/scripts/EasternKingdoms/Karazhan/boss_terestian_illhoof.cpp
-- Creature entry: 15688 (wowhead wotlk npc=15688/terestian-illhoof; C++
-- ScriptName "boss_terestian_illhoof" per AddSC_boss_terestian_illhoof).
-- Instance data: DATA_TERESTIAN = 7 (karazhan.h:37); the encounter never
-- reads encounter state — the BossAI SetBossState arms have no Go model
-- (see deviations), admission is via the luaBossAI shim.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 14 OnHitBySpell (the BROKEN_PACT summon re-arm), 23 OnReset.
-- Timers via CreateLuaEvent; melee is engine-driven in Go (creature combat
-- tick), like C++ DoMeleeAttackIfReady.
-- Fight shape: shadow bolt 30055 on the victim every 4-10s (first 1s);
-- sacrifice 30115 on a random alive player within 100 yd every 42s (first
-- 30s) with Talk(SAY_SACRIFICE); the two fiendish portal casts one-shot at
-- 10s/11s (Talk(SAY_SUMMON_PORTAL) on the first); berserk 32965 one-shot
-- at 10min. Kil'rek is summoned once 3s in (removing any broken pact
-- debuff from the boss first); when broken pact 30065 hits the boss, the
-- summon timer is re-armed at 32s.
-- Deviations from C++: the two portal summons (SPELL_FIENDISH_PORTAL_1 30171,
-- SPELL_FIENDISH_PORTAL_2 30179) ARE ported: the spell effects spawn
-- portal 17265 (same engine gap as kil'rek below), the portal imp pump
-- (first imp 2.4s, then every 8s, DoCastAOE 30184 triggered) is armed by
-- the boss event-19 handler (the portal never engages, so no portal
-- combat event can arm it), the portal imps' DoZoneInCombat ports via
-- the portal event-19 handler, and Reset/JustDied/LeaveCombat despawn
-- portals + imps via a grid sweep (the ACTION_DESPAWN_IMPS DoAction
-- relay has no bridge). The sacrifice's demon-chains cast IS ported
-- (2026-10-24 player:CastSpell bridge): the sacrificed player self-casts
-- SPELL_SUMMON_DEMONCHAINS 30120 in C++ order, but the summon effect has
-- no session-path executor yet, so no chains creature spawns and the
-- sacrifice debuff still runs its full duration (see below). The kil'rek summon
-- (SPELL_SUMMON_IMP 30066) IS ported: the 2026-10-23 engine gap closure
-- (4212e3e7) executes SPELL_EFFECT_SUMMON on the creature-cast path, so the
-- DoCastAOE self-cast spawns 17229 whose npc_kilrek AI is registered below.
-- The kil'rek (17229, "npc_kilrek")
-- and fiendish imp (17267, "npc_fiendish_imp") AIs are registered below so
-- any that exist fight correctly. Demon chains (17248, "npc_demon_chain")
-- is not registered: its IsSummonedBy arm has no firing hook (events 19/22
-- never fire in the engine) and its JustDied arm needs the recorded
-- sacrifice GUID, which has no bridge — so the sacrifice debuff runs its
-- full duration instead of ending when the chains die. Fiendish portal
-- (17265, "npc_fiendish_portal") is registered for event 19 only
-- (JustSummoned imp engage); its imp pump lives in the boss file (see
-- above). No threat model — shadow bolt uses the victim via the
-- nil-target fallback (C++ MaxThreat) and sacrifice picks any random alive
-- player. No UNIT_STATE_CASTING model — timers fire unconditionally. The
-- fiendish imp's fire-school ApplySpellImmune and kil'rek's
-- DespawnOrUnsummon(15s) on death rides a 15s creature:Despawn event.

local ENTRY_TERESTIAN_ILLHOOF = 15688
local ENTRY_KILREK = 17229
local ENTRY_FIENDISH_IMP = 17267
local ENTRY_FIENDISH_PORTAL = 17265

local SAY_SLAY = 0
local SAY_DEATH = 1
local SAY_AGGRO = 2
local SAY_SACRIFICE = 3
local SAY_SUMMON_PORTAL = 4

local SPELL_SHADOW_BOLT = 30055
local SPELL_SUMMON_IMP = 30066
local SPELL_FIENDISH_PORTAL_1 = 30171
local SPELL_FIENDISH_PORTAL_2 = 30179
local SPELL_BERSERK = 32965
local SPELL_SUMMON_FIENDISH_IMP = 30184
local SPELL_BROKEN_PACT = 30065
local SPELL_AMPLIFY_FLAMES = 30053
local SPELL_FIREBOLT = 30050
local SPELL_SUMMON_DEMONCHAINS = 30120
local SPELL_SACRIFICE = 30115

local timers = {}
local kilrekTimers = {}
local impTimers = {}

local function cancelTimers(store, guid)
    local per = store[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        store[guid] = nil
    end
end

local function schedule(store, guid, key, delay, fn)
    local per = store[guid]
    if not per then
        per = {}
        store[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

local function cancelTimer(store, guid, key)
    local per = store[guid]
    if per and per[key] then
        RemoveEventById(per[key])
        per[key] = nil
    end
end

-- C++ SelectTarget(Random, 0, 100, true) (boss_terestian_illhoof.cpp:120):
-- the pool is the threat list, not the instance players.
local function randomPlayerInRange(creature, maxDist)
    local candidates = {}
    for _, unit in ipairs(creature:GetAITargets()) do
        if unit:GetObjectType() == "Player" and not unit:IsDead()
                and creature:GetDistance(unit) <= maxDist then
            candidates[#candidates + 1] = unit
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EVENT_SHADOWBOLT: SelectTarget(MaxThreat, 0) approximated by the
-- victim (nil-target fallback); Repeat(Seconds(4), Seconds(10)).
local function onShadowbolt(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SHADOW_BOLT)
    end
    schedule(timers, guid, "shadowbolt", math.random(4000, 10000), function()
        onShadowbolt(creature, guid)
    end)
end

-- C++ EVENT_SUMMON_KILREK: RemoveAurasDueToSpell(BROKEN_PACT), then
-- DoCastAOE(SUMMON_IMP, triggered) — self-cast, the summon effect spawns
-- kil'rek 17229 (npc_kilrek AI registered below; 4212e3e7 engine gap).
local function onSummonKilrek(creature, guid)
    creature:RemoveAura(SPELL_BROKEN_PACT)
    creature:CastSpell(creature, SPELL_SUMMON_IMP, true)
end

-- C++ EVENT_SACRIFICE: random alive player within 100 yd; DoCast(target,
-- SACRIFICE, triggered); target->CastSpell(target, SUMMON_DEMONCHAINS,
-- triggered) — ported via player:CastSpell; the summon effect itself has
-- no session-path executor, so no chains creature spawns and the
-- sacrifice debuff runs its full duration. Repeat 42s.
local function onSacrifice(creature, guid)
    local target = randomPlayerInRange(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_SACRIFICE, true)
        target:CastSpell(target, SPELL_SUMMON_DEMONCHAINS, true)
        creature:Talk(SAY_SACRIFICE)
    end
    schedule(timers, guid, "sacrifice", 42000, function()
        onSacrifice(creature, guid)
    end)
end

-- C++ EVENT_SUMMON_PORTAL_1: Talk(SAY_SUMMON_PORTAL) then
-- DoCastAOE(FIENDISH_PORTAL_1, non-triggered) — the spell effect
-- summons portal 17265 (4212e3e7 engine gap, kil'rek precedent); the
-- portal's imp pump is armed by the boss event-19 handler below.
local function onPortal1(creature, guid)
    creature:Talk(SAY_SUMMON_PORTAL)
    creature:CastSpell(creature, SPELL_FIENDISH_PORTAL_1)
end

local function onEnrage(creature, guid)
    creature:CastSpell(creature, SPELL_BERSERK, true)
end

local function terestianEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(timers, guid)
    -- C++ Reset arms the timers (also on evade); the luaBossAI shim binds
    -- the instance encounter. JustEngagedWith: Talk(SAY_AGGRO).
    creature:Talk(SAY_AGGRO)
    schedule(timers, guid, "shadowbolt", 1000, function()
        onShadowbolt(creature, guid)
    end)
    schedule(timers, guid, "kilrek", 3000, function()
        onSummonKilrek(creature, guid)
    end)
    schedule(timers, guid, "sacrifice", 30000, function()
        onSacrifice(creature, guid)
    end)
    schedule(timers, guid, "portal1", 10000, function()
        onPortal1(creature, guid)
    end)
    schedule(timers, guid, "portal2", 11000, function()
        -- C++ EVENT_SUMMON_PORTAL_2: DoCastAOE(SPELL_FIENDISH_PORTAL_2,
        -- triggered) — the spell effect summons portal 17265, same
        -- event-19 pump arming as portal 1.
        creature:CastSpell(creature, SPELL_FIENDISH_PORTAL_2, true)
    end)
    schedule(timers, guid, "enrage", 600000, function()
        onEnrage(creature, guid)
    end)
end

-- C++ npc_fiendish_portal Reset: first imp at 2.4s, then every 8s
-- (DoCastAOE triggered). The portal never engages (PassiveAI), so its
-- pump is armed by the boss event-19 handler when the portal spawns,
-- not by any portal combat event; the pump lives in the boss timers
-- store so LeaveCombat/Died/Reset cancel it with everything else.
local function portalImpPump(portal, bossGuid, key)
    portal:CastSpell(portal, SPELL_SUMMON_FIENDISH_IMP, true)
    schedule(timers, bossGuid, key, 8000, function()
        portalImpPump(portal, bossGuid, key)
    end)
end

local function terestianJustSummoned(event, creature, summon)
    if summon:GetEntry() ~= ENTRY_FIENDISH_PORTAL then
        return
    end
    local bossGuid = creature:GetGUID()
    local key = "portalImp:" .. summon:GetGUID()
    schedule(timers, bossGuid, key, 2400, function()
        portalImpPump(summon, bossGuid, key)
    end)
end

-- C++ npc_fiendish_portal JustSummoned: track + DoZoneInCombat.
local function portalJustSummoned(event, creature, summon)
    summon:DoZoneInCombat()
end

-- C++ Reset/JustDied: portals DoAction(ACTION_DESPAWN_IMPS) + BossAI
-- summon cleanup; the grid sweep is the Lua twin (no DoAction relay).
local function despawnPortalMachine(creature)
    for _, imp in ipairs(creature:GetCreaturesInGrid(ENTRY_FIENDISH_IMP, 100)) do
        imp:Despawn()
    end
    for _, portal in ipairs(creature:GetCreaturesInGrid(ENTRY_FIENDISH_PORTAL, 100)) do
        portal:Despawn()
    end
end

local function terestianLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(timers, guid)
    despawnPortalMachine(creature)
end

local function terestianDied(event, creature, killer)
    local guid = creature:GetGUID()
    cancelTimers(timers, guid)
    -- C++ JustDied: Talk(SAY_DEATH), then the portal-imp despawn arm
    -- (sweep above); SetBossState has no model.
    creature:Talk(SAY_DEATH)
    despawnPortalMachine(creature)
end

-- C++ SpellHit: re-arm EVENT_SUMMON_KILREK at 32s when hit by broken pact
-- (EventMap::ScheduleEvent overwrites the pending timer).
local function terestianSpellHit(event, creature, caster, spellId)
    if spellId == SPELL_BROKEN_PACT then
        local guid = creature:GetGUID()
        cancelTimer(timers, guid, "kilrek")
        schedule(timers, guid, "kilrek", 32000, function()
            onSummonKilrek(creature, guid)
        end)
    end
end

local function terestianReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(timers, guid)
    despawnPortalMachine(creature)
end

RegisterCreatureEvent(ENTRY_TERESTIAN_ILLHOOF, 19, terestianJustSummoned)
RegisterCreatureEvent(ENTRY_FIENDISH_PORTAL, 19, portalJustSummoned)

RegisterCreatureEvent(ENTRY_TERESTIAN_ILLHOOF, 1, terestianEnterCombat)
RegisterCreatureEvent(ENTRY_TERESTIAN_ILLHOOF, 2, terestianLeaveCombat)
RegisterCreatureEvent(ENTRY_TERESTIAN_ILLHOOF, 3, function(event, creature, victim)
    -- C++ KilledUnit: Talk(SAY_SLAY), TYPEID_PLAYER gate.
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end)
RegisterCreatureEvent(ENTRY_TERESTIAN_ILLHOOF, 4, terestianDied)
RegisterCreatureEvent(ENTRY_TERESTIAN_ILLHOOF, 14, terestianSpellHit)
RegisterCreatureEvent(ENTRY_TERESTIAN_ILLHOOF, 23, terestianReset)

-- Kil'rek (npc_kilrek, C++ ScriptName for 17229): amplify flames 30053 on
-- the victim 8s after Reset, every 9s after; melee is engine-driven. On
-- death casts broken pact 30065 triggered (the boss's SpellHit arm re-arms
-- the kilrek summon timer at 32s); DespawnOrUnsummon(15s) rides a 15s
-- creature:Despawn event.
-- Kil'rek never spawns in this build (no summon model), but the AI is
-- registered so any that exist fight correctly.
local function onAmplifyFlames(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_AMPLIFY_FLAMES)
    end
    schedule(kilrekTimers, guid, "amplify", 9000, function()
        onAmplifyFlames(creature, guid)
    end)
end

local function kilrekEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(kilrekTimers, guid)
    schedule(kilrekTimers, guid, "amplify", 8000, function()
        onAmplifyFlames(creature, guid)
    end)
end

local function kilrekLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(kilrekTimers, guid)
end

local function kilrekDied(event, creature, killer)
    local guid = creature:GetGUID()
    cancelTimers(kilrekTimers, guid)
    creature:CastSpell(creature, SPELL_BROKEN_PACT, true)
    -- C++ JustDied: DespawnOrUnsummon(Seconds(15)) after the broken-pact
    -- cast (boss_terestian_illhoof.cpp:180); modeled as a 15s Lua event.
    schedule(kilrekTimers, guid, "despawn", 15000, function()
        creature:Despawn()
    end)
end

RegisterCreatureEvent(ENTRY_KILREK, 1, kilrekEnterCombat)
RegisterCreatureEvent(ENTRY_KILREK, 2, kilrekLeaveCombat)
RegisterCreatureEvent(ENTRY_KILREK, 4, kilrekDied)
RegisterCreatureEvent(ENTRY_KILREK, 23, kilrekLeaveCombat)

-- Fiendish imp (npc_fiendish_imp, C++ ScriptName for 17267): firebolt 30050
-- on the victim 2s after Reset, every 2.4s after; the fire-school
-- ApplySpellImmune arm is PORTED in impLeaveCombat (Reset hook, :298).
-- The imps never spawn (no summon model), but the AI is registered so
-- any that exist fight correctly.
local function onFirebolt(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FIREBOLT)
    end
    schedule(impTimers, guid, "firebolt", 2400, function()
        onFirebolt(creature, guid)
    end)
end

local function impEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(impTimers, guid)
    schedule(impTimers, guid, "firebolt", 2000, function()
        onFirebolt(creature, guid)
    end)
end

local function impLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(impTimers, guid)
    -- C++ npc_fiendish_imp::Reset() :298: ApplySpellImmune(0,
    -- IMMUNITY_SCHOOL, SPELL_SCHOOL_MASK_FIRE=4, true) — fire-school
    -- immunity (runs on evade too: C++ EnterEvadeMode calls Reset).
    creature:ApplySpellImmune(0, 2, 4, true)
end

RegisterCreatureEvent(ENTRY_FIENDISH_IMP, 1, impEnterCombat)
RegisterCreatureEvent(ENTRY_FIENDISH_IMP, 2, impLeaveCombat)
RegisterCreatureEvent(ENTRY_FIENDISH_IMP, 23, impLeaveCombat)
