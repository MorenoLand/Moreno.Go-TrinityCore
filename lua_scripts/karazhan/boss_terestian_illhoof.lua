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
-- Deviations from C++: no summon model — the kil'rek summon
-- (SPELL_SUMMON_IMP 30066), the two portal summons (SPELL_FIENDISH_PORTAL_1
-- 30171, SPELL_FIENDISH_PORTAL_2 30179), and the sacrifice's demon-chains
-- summon (SPELL_SUMMON_DEMONCHAINS 30120) never spawn their creatures, so
-- those casts are skipped while the non-summon arms (RemoveAura, Talk)
-- stay; the boss's Reset/JustDied ACTION_DESPAWN_IMPS arms and the portal
-- imps' DoZoneInCombat have no bearer. The kil'rek (17229, "npc_kilrek")
-- and fiendish imp (17267, "npc_fiendish_imp") AIs are registered below so
-- any that exist fight correctly. Demon chains (17248, "npc_demon_chain")
-- is not registered: its IsSummonedBy arm has no firing hook (events 19/22
-- never fire in the engine) and its JustDied arm needs the recorded
-- sacrifice GUID, which has no bridge — so the sacrifice debuff runs its
-- full duration instead of ending when the chains die. Fiendish portal
-- (17265, "npc_fiendish_portal") is not registered: PassiveAI with its
-- scheduler on UpdateAI, no combat events fire for it and its imp summons
-- have no bearer. No threat model — shadow bolt uses the victim via the
-- nil-target fallback (C++ MaxThreat) and sacrifice picks any random alive
-- player. No UNIT_STATE_CASTING model — timers fire unconditionally. The
-- fiendish imp's fire-school ApplySpellImmune and kil'rek's
-- DespawnOrUnsummon(15s) on death have no bridge.

local ENTRY_TERESTIAN_ILLHOOF = 15688
local ENTRY_KILREK = 17229
local ENTRY_FIENDISH_IMP = 17267

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
    per[key] = CreateLuaEvent(fn, delay)
end

local function cancelTimer(store, guid, key)
    local per = store[guid]
    if per and per[key] then
        RemoveEventById(per[key])
        per[key] = nil
    end
end

local function alivePlayersInRange(creature, maxDist)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() and creature:GetDistance(p) <= maxDist then
            found[#found + 1] = p
        end
    end
    return found
end

local function randomPlayerInRange(creature, maxDist)
    local candidates = alivePlayersInRange(creature, maxDist)
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
-- DoCastAOE(SUMMON_IMP, triggered) — the summon has no bearer, skipped.
local function onSummonKilrek(creature, guid)
    creature:RemoveAura(SPELL_BROKEN_PACT)
end

-- C++ EVENT_SACRIFICE: random alive player within 100 yd; DoCast(target,
-- SACRIFICE, triggered); target->CastSpell(target, SUMMON_DEMONCHAINS,
-- triggered) — the chains summon has no bearer, skipped (the sacrifice
-- debuff then runs its full duration). Repeat 42s.
local function onSacrifice(creature, guid)
    local target = randomPlayerInRange(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_SACRIFICE, true)
        creature:Talk(SAY_SACRIFICE)
    end
    schedule(timers, guid, "sacrifice", 42000, function()
        onSacrifice(creature, guid)
    end)
end

-- C++ EVENT_SUMMON_PORTAL_1 / _2: DoCastAOE(FIENDISH_PORTAL_1/_2) — the
-- portal summons have no bearer, skipped; only the emote stays.
local function onPortal1(creature, guid)
    creature:Talk(SAY_SUMMON_PORTAL)
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
        -- triggered) only — the portal summon has no bearer, so the event
        -- is a no-op in this build.
    end)
    schedule(timers, guid, "enrage", 600000, function()
        onEnrage(creature, guid)
    end)
end

local function terestianLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(timers, guid)
end

local function terestianDied(event, creature, killer)
    local guid = creature:GetGUID()
    cancelTimers(timers, guid)
    -- C++ JustDied: Talk(SAY_DEATH); the portal-imp despawn arm and
    -- SetBossState have no model.
    creature:Talk(SAY_DEATH)
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
end

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
-- the kilrek summon timer at 32s); DespawnOrUnsummon has no bridge.
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
end

RegisterCreatureEvent(ENTRY_KILREK, 1, kilrekEnterCombat)
RegisterCreatureEvent(ENTRY_KILREK, 2, kilrekLeaveCombat)
RegisterCreatureEvent(ENTRY_KILREK, 4, kilrekDied)
RegisterCreatureEvent(ENTRY_KILREK, 23, kilrekLeaveCombat)

-- Fiendish imp (npc_fiendish_imp, C++ ScriptName for 17267): firebolt 30050
-- on the victim 2s after Reset, every 2.4s after; the fire-school
-- ApplySpellImmune has no bridge. The imps never spawn (no summon model),
-- but the AI is registered so any that exist fight correctly.
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
end

RegisterCreatureEvent(ENTRY_FIENDISH_IMP, 1, impEnterCombat)
RegisterCreatureEvent(ENTRY_FIENDISH_IMP, 2, impLeaveCombat)
RegisterCreatureEvent(ENTRY_FIENDISH_IMP, 23, impLeaveCombat)
