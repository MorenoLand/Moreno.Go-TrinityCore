-- Emalon the Storm Watcher (Vault of Archavon) — Lua port of
-- src/server/scripts/Northrend/VaultOfArchavon/boss_emalon.cpp
-- (290 lines; 2 scripts — boss_emalon and npc_tempest_minion, both
-- registered from inside AddSC_boss_emalon() — loader decl 140 /
-- call 335, the "// Vault of Archavon" block in
-- AddNorthrendScripts(), northrend_script_loader.cpp).
-- Entries: 33993 Emalon the Storm Watcher (vault_of_archavon.h
-- NPC_EMALON, line 39; DATA_EMALON boss slot); 33998 Tempest Minion
-- (boss_emalon.cpp enum Npcs :54-57 — header carries no minion id).
-- The stale "no-timer-bridge" doctrine is superseded by the
-- CreateLuaEvent bridge (engine/scripting/runtime.go:798 — the
-- archavon / maiden_of_virtue precedent): module-local GUID-keyed
-- timers table, wipe on events 2 (leave combat), 4 (death), 23
-- (evade).
-- creature:CastSpell(nil, spell) = DoCastVictim; creature:CastSpell
-- (creature, spell) = DoCastSelf / DoCast(me); creature:CastSpell
-- (target, spell[, triggered]) = DoCast(target, spell[, triggered])
-- (the 3rd CastSpell arg is the C++ DoCast 3rd arg; 2-arg / 1-arg
-- overloads = non-triggered). All delays below are fixed ms (C++ uses
-- plain 5s/25s/40s/6min literals), so plain ms delays carry the
-- C++-exact timing — no {minMs, maxMs} randtime table.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset.
-- Ported arms (C++-exact for all modeled arms; line refs are
-- boss_emalon.cpp):
-- boss_emalonAI::JustEngagedWith (:102-120) — the four ScheduleEvent
-- calls seed the combat timers below; the summons-list
-- minion-AttackStart leg and the BossAI::JustEngagedWith(who)
-- passthrough are unmodeled.
-- EVENT_CHAIN_LIGHTNING (:136-139) — SelectTarget(Random, 0) (:137,
-- no distance filter) -> DoCast(target, SPELL_CHAIN_LIGHTNING
-- 64213) (:138, 2-arg = non-triggered) -> CastSpell(target, 64213);
-- re-arm unconditional (:139), 25s. Random-player pool:
-- GetPlayersInWorld, same map/instance, alive (the keleseth / thorim
-- precedent); only threat-listed units qualify in practice.
-- EVENT_LIGHTNING_NOVA (:141-143) — DoCastAOE(SPELL_LIGHTNING_NOVA
-- 64216) (:142) = self-cast per the in-tree DoCastAOE convention
-- (boss_ai.go) -> CastSpell(creature, 64216); re-arm 40s (:143).
-- EVENT_BERSERK (:158-160) — DoCast(me, SPELL_BERSERK 26662) (:159,
-- 2-arg = non-triggered) -> CastSpell(creature, 26662); Talk
-- (EMOTE_BERSERK 2) (:160) -> creature:Talk(2); cast-before-Talk kept;
-- 6min one-shot, no re-arm (:116).
-- npc_tempest_minionAI::JustEngagedWith (:219-229) — schedules
-- EVENT_SHOCK at 20s (:222); the DoZoneInCombat() and the
-- instance->GetGuidData(DATA_EMALON) pEmalon AttackStart legs are
-- unmodeled.
-- EVENT_SHOCK (:265-268) — DoCastVictim(SPELL_SHOCK 64363) (:267,
-- 1-arg = non-triggered) -> CastSpell(nil, 64363); re-arm 20s
-- (:268).
-- DOCUMENTED DEVIATIONS: none on the ported arms — all four boss
-- timers and the minion shock timer carry C++-exact timing and
-- targeting. The UpdateAI HasUnitState(UNIT_STATE_CASTING) gates
-- (:129, :166) and DoMeleeAttackIfReady() (:170) have no bridge —
-- casts fire unconditionally on their timers (the hodir
-- UNIT_STATE_CASTING precedent).
-- DOCUMENTED-ONLY (in this header; entries 33993 / 33998):
-- EVENT_OVERCHARGE (:145-157) — random alive minion picked from the
-- boss's BossAI summons list (ObjectAccessor::GetCreature on
-- SelectRandomContainerElement — no summons-list bridge, even though
-- the minions themselves now materialize on Reset); the
-- minion->CastSpell(me, SPELL_OVERCHARGED, true) + SetFullHealth()
-- legs and the Talk (EMOTE_OVERCHARGE 0) riding it all stay out.
-- boss_emalonAI::Reset (:85-91) — four SummonCreature
-- (NPC_TEMPEST_MINION, TEMPSUMMON_CORPSE_DESPAWN) at TempestMinions[]
-- positions PORTED this run (emalonReset, event 23 — the old "no
-- SummonCreature bridge" claim is re-keyed): summonType 4 /
-- despawnMs 0; documented delta: the corpse-despawn leg has no bridge
-- — minion corpses stay on the normal corpse sweep.
-- boss_emalonAI::JustSummoned (:93-100) — minion AttackStart leg: no
-- bridge.
-- npc_tempest_minionAI::JustDied (:207-217) — instance->
-- GetGuidData(DATA_EMALON) respawn leg (Emalon re-summons the minion,
-- Talk EMOTE_MINION_RESPAWN 1): instance-script bridge missing.
-- npc_tempest_minionAI::UpdateAI overcharge self-stack loop
-- (:242-263) — GetAura stack count (creature:HasAura is
-- boolean-only), DoCast(me, SPELL_OVERCHARGED) every 2s,
-- SPELL_OVERCHARGED_BLAST 64219 at 10 stacks: no GetAura stack-count
-- bridge.
-- SetFullHealth (:152) and DespawnOrUnsummon (:259): the minions now
-- exist (Reset summons, above), but the 10-stack trigger needs
-- GetAura stack count (HasAura is boolean-only) — SetHealth/Despawn
-- bridges exist but the arms stay out behind the unbridged
-- stack-count gate.
-- DoZoneInCombat() (:221): no bridge.

local ENTRY_EMALON = 33993
local ENTRY_TEMPEST_MINION = 33998

local EMOTE_BERSERK = 2

local SPELL_CHAIN_LIGHTNING = 64213
local SPELL_LIGHTNING_NOVA = 64216
local SPELL_BERSERK = 26662

local SPELL_SHOCK = 64363

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

-- C++ SelectTarget(SelectTargetMethod::Random, 0) — the bridged,
-- threat-ordered GetAITargets list (lua_creature_events.go:345);
-- playerOnly defaults FALSE (UnitAI.h:170), so the tempest-minion adds
-- on the threat list are eligible — no Player filter (the midnight
-- randomTargetAnywhere precedent). No distance filter.
local function randomThreatTarget(creature)
    local targets = creature:GetAITargets()
    if #targets == 0 then
        return nil
    end
    return targets[math.random(#targets)]
end

-- C++ EVENT_CHAIN_LIGHTNING (:136-139): DoCast(target,
-- SPELL_CHAIN_LIGHTNING), 2-arg = non-triggered; re-arm
-- unconditional, 25s.
local function chainLightningTick(creature, guid)
    local target = randomThreatTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_CHAIN_LIGHTNING)
    end
    schedule(guid, "chainlightning", 25000, function()
        chainLightningTick(creature, guid)
    end)
end

-- C++ EVENT_LIGHTNING_NOVA (:141-143): DoCastAOE(SPELL_LIGHTNING_NOVA)
-- = self-cast per the in-tree DoCastAOE convention; re-arm 40s.
local function lightningNovaTick(creature, guid)
    creature:CastSpell(creature, SPELL_LIGHTNING_NOVA)
    schedule(guid, "lightningnova", 40000, function()
        lightningNovaTick(creature, guid)
    end)
end

-- C++ EVENT_BERSERK (:158-160): DoCast(me, SPELL_BERSERK), 2-arg =
-- non-triggered; Talk(EMOTE_BERSERK); cast-before-Talk kept; 6min
-- one-shot, no re-arm.
local function emalonBerserkTick(creature, guid)
    creature:CastSpell(creature, SPELL_BERSERK)
    creature:Talk(EMOTE_BERSERK)
end

-- C++ TempestMinions[] (boss_emalon.cpp:63-69): the four Reset summon
-- positions.
local TEMPEST_MINION_SPAWNS = {
    { -203.980103, -281.287720, 91.650223, 1.598807 },
    { -233.489410, -281.139282, 91.652412, 1.598807 },
    { -233.267578, -297.104645, 91.681915, 1.598807 },
    { -203.842529, -297.097015, 91.745163, 1.598807 },
}

-- C++ boss_emalonAI::Reset (boss_emalon.cpp:85-91): _Reset() (timer
-- clear) then four SummonCreature(NPC_TEMPEST_MINION 33998,
-- TempestMinions[i], TEMPSUMMON_CORPSE_DESPAWN (4)) — the C++ call has
-- no despawn-time arg, so despawnMs 0. Documented delta: the
-- corpse-despawn leg has no bridge — minion corpses stay on the normal
-- corpse sweep. The JustSummoned AttackStart leg has no bridge
-- (AttackStart is unbridged); at Reset the C++ AttackStart is a
-- nullptr-guarded no-op (no victim), so nothing is lost.
local function emalonReset(event, creature)
    cancelTimers(creature:GetGUID())
    for _, p in ipairs(TEMPEST_MINION_SPAWNS) do
        creature:SummonCreature(ENTRY_TEMPEST_MINION, p[1], p[2], p[3], p[4], 4, 0)
    end
end

-- C++ boss_emalonAI::JustEngagedWith (:102-120): the ScheduleEvent
-- calls seed the combat timers below (5s / 40s / 6min; EVENT_OVERCHARGE
-- 45s stays out — documented in the header).
local function emalonEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "chainlightning", 5000, function()
        chainLightningTick(creature, guid)
    end)
    schedule(guid, "lightningnova", 40000, function()
        lightningNovaTick(creature, guid)
    end)
    schedule(guid, "berserk", 360000, function()
        emalonBerserkTick(creature, guid)
    end)
end

-- Shared timer wipe for events 2/23/4 (the svala / yogg_saron
-- convention).
local function emalonCancelCombatTimers(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_EMALON, 1, emalonEnterCombat)
RegisterCreatureEvent(ENTRY_EMALON, 2, emalonCancelCombatTimers)
RegisterCreatureEvent(ENTRY_EMALON, 4, emalonCancelCombatTimers)
RegisterCreatureEvent(ENTRY_EMALON, 23, emalonReset)

-- C++ EVENT_SHOCK (:265-268): DoCastVictim(SPELL_SHOCK), 1-arg =
-- non-triggered; re-arm 20s.
local function shockTick(creature, guid)
    creature:CastSpell(nil, SPELL_SHOCK)
    schedule(guid, "shock", 20000, function()
        shockTick(creature, guid)
    end)
end

-- C++ npc_tempest_minionAI::JustEngagedWith (:219-229): EVENT_SHOCK
-- scheduled at 20s (:222). The DoZoneInCombat and instance legs are
-- unmodeled (documented in the header).
local function minionEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "shock", 20000, function()
        shockTick(creature, guid)
    end)
end

local function minionCancelCombatTimers(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_TEMPEST_MINION, 1, minionEnterCombat)
RegisterCreatureEvent(ENTRY_TEMPEST_MINION, 2, minionCancelCombatTimers)
RegisterCreatureEvent(ENTRY_TEMPEST_MINION, 4, minionCancelCombatTimers)
RegisterCreatureEvent(ENTRY_TEMPEST_MINION, 23, minionCancelCombatTimers)
