-- Cyanigosa (Violet Hold) — Lua port of
-- src/server/scripts/Northrend/VioletHold/boss_cyanigosa.cpp
-- (3 scripts — boss_cyanigosa (CreatureScript via GetVioletHoldAI
-- (BossAI), DATA_CYANIGOSA = 2); achievement_defenseless
-- (AchievementCriteriaScript, OnCheck (:134) instance->GetData
-- (DATA_DEFENSELESS)); spell_cyanigosa_arcane_vacuum
-- (SpellScriptLoader, OnEffectHitTarget (:161) casts
-- SPELL_SUMMON_PLAYER 21150 triggered); all registered from inside
-- AddSC_boss_cyanigosa() (:178) — loader decl 145 / call 340 per
-- northrend_script_loader.cpp — the FIRST group of the "// Violet Hold"
-- block in AddNorthrendScripts(), immediately after
-- AddSC_instance_vault_of_archavon() (call 338); the call after it is
-- AddSC_boss_erekem() (call 341).
-- Entries: 31134 Cyanigosa (violet_hold.h NPC_CYANIGOSA, line 116) —
-- entry-verifiable (the nexus_commanders / malygos / sartharion
-- kalecgos precedent); the CreatureScript ScriptName binding is
-- DB-side as usual. DATA_CYANIGOSA = 2 (violet_hold.h line 51).
-- The stale "no-timer-bridge" doctrine is superseded by the
-- CreateLuaEvent bridge (engine/scripting/runtime.go:798 — the
-- archavon / maiden_of_virtue precedent): module-local GUID-keyed
-- timers table, wipe on events 2 (leave combat), 4 (death), 23
-- (evade). BossAI::_JustEngagedWith calls ScheduleTasks()
-- (ScriptedCreature.cpp:529-544), so the four timers below are seeded
-- at event 1.
-- creature:CastSpell(nil, spell) = DoCastVictim; creature:CastSpell
-- (creature, spell) = DoCastSelf / DoCastAOE (2-arg = non-triggered);
-- creature:CastSpell(target, spell) = DoCast(target, spell) (2-arg =
-- non-triggered). All delays below are fixed ms (C++ uses plain
-- Seconds literals), so plain ms delays carry the C++-exact timing —
-- no {minMs, maxMs} randtime table.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnKill,
-- 4 OnDied, 23 OnReset.
-- Ported arms (C++-exact for all modeled arms; line refs are
-- boss_cyanigosa.cpp):
-- boss_cyanigosaAI::JustEngagedWith (:55-59) — Talk(SAY_AGGRO 0) (:58)
-- at event 1 (the auriaya precedent); the BossAI::JustEngagedWith(who)
-- passthrough is unmodeled; the ScheduleTasks seeding is carried by
-- the event-1 CreateLuaEvent scheduling below.
-- ARCANE_VACUUM (:86-90) — DoCastAOE(SPELL_ARCANE_VACUUM 58694) (:88,
-- 1-arg = non-triggered) = self-cast per the in-tree DoCastAOE
-- convention (boss_ai.go) -> CastSpell(creature, 58694); 10s init /
-- Repeat() 10s. (The arcane-vacuum SpellScript's
-- SPELL_SUMMON_PLAYER 21150 leg stays out — documented below.)
-- BLIZZARD (:92-97) — SelectTarget(Random, 0, 45.0f, true) (:94,
-- random alive player within 45y, playerOnly, tank included) ->
-- DoCast(target, SPELL_BLIZZARD 58693) (:95, 2-arg = non-triggered)
-- -> CastSpell(target, 58693); 15s init / Repeat() 15s. Random-player
-- pool: GetPlayersInWorld, same map/instance, alive, within 45y (the
-- keleseth / thorim / freya precedent).
-- TAIL_SWEEP (:99-103) — DoCastVictim(SPELL_TAIL_SWEEP 58690) (:101,
-- 1-arg = non-triggered) -> CastSpell(nil, 58690); 20s init /
-- Repeat() 20s.
-- UNCONTROLLABLE_ENERGY (:105-109) — DoCastVictim
-- (SPELL_UNCONTROLLABLE_ENERGY 58688) (:107, 1-arg = non-triggered)
-- -> CastSpell(nil, 58688); 25s init / Repeat() 25s.
-- MANA_DESTRUCTION (:113-119) — seeded ONLY when IsHeroic() at
-- engage (:111): SelectTarget(Random, 0, 50.0f, true) (:115, random
-- alive player within 50y, playerOnly, tank included) ->
-- DoCast(target, SPELL_MANA_DESTRUCTION 59374) (:116, 2-arg =
-- non-triggered) -> CastSpell(target, 59374); 30s init / Repeat() 30s
-- (:117, unconditional — outside the target-pick if). The IsHeroic()
-- gate rides creature:IsHeroic()
-- (engine/world/lua_creature_events.go:203 — per-instance difficulty
-- read); the old "heroic-mode branches are unbridged" claim for this
-- arm is stale (ported this tick).
-- KilledUnit (:61-65) — if (victim->GetTypeId() == TYPEID_PLAYER)
-- Talk(SAY_SLAY 1) — player-gated, event 3 (the razuvious variant
-- precedent).
-- JustDied (:67-71) — Talk(SAY_DEATH 2) (:69); the _JustDied()
-- passthrough is unmodeled (the sjonnir JustDied-Talk precedent).
-- DOCUMENTED-ONLY (in this header; no registration beyond entry 31134):
-- spell_cyanigosa_arcane_vacuum (:147-176) — SpellScript
-- OnEffectHitTarget (:161): GetCaster()->CastSpell(GetHitUnit(),
-- SPELL_SUMMON_PLAYER 21150, true): no SpellScript bridge.
-- achievement_defenseless (:129-145) — AchievementCriteriaScript
-- OnCheck (:134): instance->GetData(DATA_DEFENSELESS): no
-- achievement-criteria bridge.
-- MoveInLineOfSight (:73) — empty override: nothing to port.
-- DoMeleeAttackIfReady (:80) — melee is engine-driven in Go (the
-- hodir / vezax precedent); the scheduler.Update passthrough is
-- unmodeled.
-- SAY_SPAWN (3) / SAY_DISRUPTION (4) / SAY_BREATH_ATTACK (5) /
-- SAY_SPECIAL_ATTACK (6) — never Talk()ed anywhere in the file
-- (grep -c "Talk(" = 3, all ported above): dead enum text.

local ENTRY_CYANIGOSA = 31134

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2

local SPELL_ARCANE_VACUUM = 58694
local SPELL_BLIZZARD = 58693
local SPELL_TAIL_SWEEP = 58690
local SPELL_UNCONTROLLABLE_ENERGY = 58688
local SPELL_MANA_DESTRUCTION = 59374

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

-- C++ SelectTarget(SelectTargetMethod::Random, 0, 45.0f, true) —
-- alive players on the same map/instance within 45y, tank included
-- (the freya randomPlayerInRange precedent).
local function randomPlayerInRange(creature, maxDist)
    local mapId = creature:GetMapId()
    local instanceId = creature:GetInstanceId()
    local candidates = {}
    for _, plr in ipairs(GetPlayersInWorld()) do
        if plr:GetMapId() == mapId and plr:GetInstanceId() == instanceId
                and not plr:IsDead() then
            if creature:GetDistance(plr) <= maxDist then
                candidates[#candidates + 1] = plr
            end
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ ARCANE_VACUUM (:86-90): DoCastAOE(SPELL_ARCANE_VACUUM) =
-- self-cast (in-tree DoCastAOE convention), non-triggered; 10s init,
-- Repeat() 10s.
local function arcaneVacuumTick(creature, guid)
    creature:CastSpell(creature, SPELL_ARCANE_VACUUM)
    schedule(guid, "arcanevacuum", 10000, function()
        arcaneVacuumTick(creature, guid)
    end)
end

-- C++ BLIZZARD (:92-97): random player within 45y -> DoCast(target),
-- non-triggered; 15s init, Repeat() 15s.
local function blizzardTick(creature, guid)
    local target = randomPlayerInRange(creature, 45)
    if target then
        creature:CastSpell(target, SPELL_BLIZZARD)
    end
    schedule(guid, "blizzard", 15000, function()
        blizzardTick(creature, guid)
    end)
end

-- C++ TAIL_SWEEP (:99-103): DoCastVictim, non-triggered; 20s init,
-- Repeat() 20s.
local function tailSweepTick(creature, guid)
    creature:CastSpell(nil, SPELL_TAIL_SWEEP)
    schedule(guid, "tailsweep", 20000, function()
        tailSweepTick(creature, guid)
    end)
end

-- C++ UNCONTROLLABLE_ENERGY (:105-109): DoCastVictim,
-- non-triggered; 25s init, Repeat() 25s.
local function uncontrollableEnergyTick(creature, guid)
    creature:CastSpell(nil, SPELL_UNCONTROLLABLE_ENERGY)
    schedule(guid, "uncontrollableenergy", 25000, function()
        uncontrollableEnergyTick(creature, guid)
    end)
end

-- C++ MANA_DESTRUCTION (:113-119, seeded only when IsHeroic() at
-- engage): SelectTarget(Random, 0, 50.0f, true) ->
-- DoCast(target, SPELL_MANA_DESTRUCTION 59374) (2-arg =
-- non-triggered); 30s init, Repeat() 30s unconditional (the Repeat
-- sits outside the target-pick if). The IsHeroic() gate rides
-- creature:IsHeroic() (engine/world/lua_creature_events.go:203, the
-- per-instance difficulty read); the old "heroic-mode branches are
-- unbridged" header claim is stale.
local function manaDestructionTick(creature, guid)
    local target = randomPlayerInRange(creature, 50)
    if target then
        creature:CastSpell(target, SPELL_MANA_DESTRUCTION)
    end
    schedule(guid, "manadestruction", 30000, function()
        manaDestructionTick(creature, guid)
    end)
end

-- C++ JustEngagedWith (:55-59): Talk(SAY_AGGRO) + the ScheduleTasks
-- seeding (10s / 15s / 20s / 25s; 30s MANA_DESTRUCTION only when
-- IsHeroic() — C++-exact: the timer is seeded at engage, not gated
-- per-tick); the BossAI::JustEngagedWith(who) passthrough is
-- unmodeled.
local function cyanigosaEnterCombat(event, creature)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "arcanevacuum", 10000, function()
        arcaneVacuumTick(creature, guid)
    end)
    schedule(guid, "blizzard", 15000, function()
        blizzardTick(creature, guid)
    end)
    schedule(guid, "tailsweep", 20000, function()
        tailSweepTick(creature, guid)
    end)
    schedule(guid, "uncontrollableenergy", 25000, function()
        uncontrollableEnergyTick(creature, guid)
    end)
    if creature:IsHeroic() then
        schedule(guid, "manadestruction", 30000, function()
            manaDestructionTick(creature, guid)
        end)
    end
end

-- C++ KilledUnit (:61-65): player-gated Talk(SAY_SLAY) (the
-- razuvious precedent).
local function cyanigosaKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ JustDied (:67-71): Talk(SAY_DEATH) (the sjonnir precedent);
-- the _JustDied() passthrough is unmodeled.
local function cyanigosaJustDied(event, creature)
    creature:Talk(SAY_DEATH)
end

-- Shared timer wipe for events 2/23/4 (the svala / yogg_saron
-- convention).
local function cyanigosaCancelCombatTimers(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_CYANIGOSA, 1, cyanigosaEnterCombat)
RegisterCreatureEvent(ENTRY_CYANIGOSA, 2, cyanigosaCancelCombatTimers)
RegisterCreatureEvent(ENTRY_CYANIGOSA, 3, cyanigosaKilledUnit)
RegisterCreatureEvent(ENTRY_CYANIGOSA, 4, cyanigosaCancelCombatTimers)
RegisterCreatureEvent(ENTRY_CYANIGOSA, 4, cyanigosaJustDied)
RegisterCreatureEvent(ENTRY_CYANIGOSA, 23, cyanigosaCancelCombatTimers)
