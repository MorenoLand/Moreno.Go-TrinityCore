-- Vexallus (Magister's Terrace) --
-- Lua port of src/server/scripts/EasternKingdoms/MagistersTerrace/
-- boss_vexallus.cpp
-- (boss_vexallus — BossAI combat entry + 15%-interval energy-discharge
-- latch + 10%-HP overload enrage + KilledUnit talk). Boss-script unit per
-- eastern_kingdoms_script_loader.cpp order (boss_selin_fireheart done:
-- boss class ported, entry 24723 registered; AddSC_boss_vexallus next in
-- the Magister's Terrace block; boss_priestess_delrissa after;
-- instance_magisters_terrace.cpp stays blocked on the instance-script
-- model).
-- Entry (verifiable from the C++ sources): magisters_terrace.h names
-- BOSS_VEXALLUS = 24744 in the MTCreatureIds enum (the AI runs under
-- BossAI(DATA_VEXALLUS = 2) via GetMagistersTerraceAI, a GetInstanceAI
-- retrieval wrapper only; the ported arms have no instance state). The
-- creature_template ScriptName binding is DB-side (no TDB in this
-- workspace), but the entry itself is C++-verifiable so the port is
-- registered (boss_felblood_kaelthas precedent). Script name is the
-- stringified class name (CreatureScript("boss_vexallus");
-- RegisterCreatureAIWithFactory convention, felblood precedent).
-- Verifiable numbers (file's own enums/header): SAY_AGGRO = 0 /
-- SAY_ENERGY = 1 / SAY_OVERLOAD = 2 (enum-only, never spoken) /
-- SAY_KILL = 3 / EMOTE_DISCHARGE_ENERGY = 4; SPELL_CHAIN_LIGHTNING =
-- 44318 / SPELL_OVERLOAD = 44353 / SPELL_ARCANE_SHOCK = 44319 /
-- SPELL_SUMMON_PURE_ENERGY = 44322 / H_SPELL_SUMMON_PURE_ENERGY1 = 46154
-- / H_SPELL_SUMMON_PURE_ENERGY2 = 46159 (heroic only); EVENT_ENERGY_BOLT
-- = 1 / EVENT_ENERGY_FEEDBACK / EVENT_CHAIN_LIGHTNING / EVENT_OVERLOAD /
-- EVENT_ARCANE_SHOCK; INTERVAL_MODIFIER = 15 / INTERVAL_SWITCH = 6.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied (C++ Eluna::KilledUnit — lua_creature_events.go fires it
-- with (event, creature, victim) from Unit::Kill), 4 OnDied, 9
-- OnDamageTaken (lua_creature_events.go fires it with (event, creature,
-- attacker, damage)), 23 OnReset. Driven by CreateLuaEvent timers;
-- melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
-- Ported arms (C++ JustEngagedWith + KilledUnit + DamageTaken + the
-- EVENT_OVERLOAD arm of UpdateAI):
-- Combat entry: Talk(SAY_AGGRO) (maiden-of-virtue convention). The C++
-- JustEngagedWith schedules EVENT_CHAIN_LIGHTNING (8s, recast 8s) and
-- EVENT_ARCANE_SHOCK (5s, recast 8s); both ported as random threat-list
-- player casts (GetAITargets + alive filter) — the old "no SelectTarget
-- bridge" claim is retracted. C++ SelectTarget(Random, 0, 0.0f, true)
-- for chain lightning is any-range (dist 0 = no distance restriction),
-- ported with no cap; arcane shock's 20.0f is used literally. Timers cancelled on OnLeaveCombat(2)/OnDied(4)/
-- OnReset(23), and on enrage via cancelTimers (C++ events.Reset()).
-- KilledUnit: Talk(SAY_KILL) via OnTargetDied(3) — the C++ arm has no
-- victim-type gate, so neither does the port.
-- DamageTaken 15%-interval discharge latch via OnDamageTaken(9).
-- C++ checks pre-damage health (Unit::DealDamage fires DamageTaken
-- before health is reduced; GetHealth() at event 9 IS pre-damage), so
-- no -damage subtraction: HealthAbovePct fires when health <=
-- floor(max*pct/100) (ironaya convention). _intervalHealthAmount
-- starts at 1;
-- at health below 85/70/55/40/25% (100 - 15*stage): stage += 1,
-- Talk(SAY_ENERGY), Talk(EMOTE_DISCHARGE_ENERGY), self-cast
-- SPELL_SUMMON_PURE_ENERGY 44322 (kaelthas phoenix-summon precedent —
-- the summon spell itself is real even though the add's class has no
-- C++-verifiable entry and is unported, queued below); the heroic
-- pair 46154/46159 (C++: if (IsHeroic()) DoCast(me, 46154) +
-- DoCast(me, 46159) else DoCast(me, 44322), cpp:125-131) is ported
-- above — stale "no difficulty bridge" retracted
-- (creature:IsHeroic() bridged, lua_creature_events.go:260).
-- At health below 10% (stage == INTERVAL_SWITCH = 6): _enraged latch —
-- Talk is skipped, all timers cancelled (C++ events.Reset()), then
-- EVENT_OVERLOAD 1200ms -> 2s loop: DoCastVictim(SPELL_OVERLOAD 44353)
-- via GetVictim nil-guarded (incarcerator convention). One-shot per
-- engagement; Reset on OnEnterCombat(1) mirrors the C++ Reset().
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (casts are
-- packet-visual), so the overload timer fires unconditionally and the
-- in-loop casting gate is dropped (maiden precedent). The C++
-- scheduler is driven from JustEngagedWith and drained by UpdateAI;
-- the port schedules from OnEnterCombat(1)/OnDamageTaken(9) and
-- cancels on 2/4/23 (maiden convention).
-- Reset() _Reset() / JustDied() _JustDied() are internal BossAI
-- machinery covered by the cancel/re-arm on combat events (halycon
-- precedent); JustDied has no C++ arms.
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - JustSummoned: SelectTarget(Random, 0) -> summoned->GetMotionMaster(
--   )->MoveFollow — the SelectTarget leg is bridgeable but MoveFollow
--   has no movement bridge (midnight/halazzi precedent); summons.Summon
--   is internal BossAI machinery. Old "no SelectTarget bridge" reason
--   corrected.
-- - npc_pure_energy (the file's second CreatureScript class): its entry
--   is NOT C++-verifiable (no pure-energy constant in
--   magisters_terrace.h; the class name appears only in
--   boss_vexallus.cpp + the loader fwd-decl/call) — no .lua written
--   (stub-free precedent, grubbis / boss_lord_valthalak queue
--   precedent); queued awaiting entry evidence from a TDB dump. Arms
--   mapped for the future port: constructor SetDisplayId(Modelid2);
--   JustDied: killer->CastSpell(killer, SPELL_ENERGY_FEEDBACK 44335,
--   true) + me->RemoveAurasDueToSpell(SPELL_PURE_ENERGY_PASSIVE 44326);
--   SPELL_ENERGY_BOLT 46156 + EVENT_ENERGY_BOLT/EVENT_ENERGY_FEEDBACK
--   are enum-only in C++ (never scheduled by the class).
local SAY_AGGRO = 0
local SAY_ENERGY = 1
local SAY_KILL = 3
local EMOTE_DISCHARGE_ENERGY = 4

local SPELL_SUMMON_PURE_ENERGY = 44322
local SPELL_SUMMON_PURE_ENERGY_H1 = 46154
local SPELL_SUMMON_PURE_ENERGY_H2 = 46159
local SPELL_OVERLOAD = 44353
local SPELL_CHAIN_LIGHTNING = 44318
local SPELL_ARCANE_SHOCK = 44319

local ENTRY_VEXALLUS = 24744

local INTERVAL_MODIFIER = 15
local INTERVAL_SWITCH = 6

local timers = {}
local dischargeStage = {}
local enraged = {}

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

-- C++ SelectTarget(Random, 0, 0.0f, true) chain lightning (:150) = no dist
-- cap; arcane shock (:155) is Random, 0, 20.0f, true (UnitAI.h:162-176).
local function randomThreatPlayer(creature)
    local candidates = {}
    for _, p in ipairs(creature:GetAITargets()) do
        if p:GetObjectType() == "Player" and not p:IsDead() then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

local function randomThreatPlayerInRange(creature, range)
    local candidates = {}
    for _, p in ipairs(creature:GetAITargets()) do
        if p:GetObjectType() == "Player" and not p:IsDead()
                and creature:IsWithinDist(p, range) then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

local function onChainLightning(creature, guid)
    local target = randomThreatPlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_CHAIN_LIGHTNING)
    end
    schedule(guid, "chain_lightning", 8000, function() onChainLightning(creature, guid) end)
end

local function onArcaneShock(creature, guid)
    local target = randomThreatPlayerInRange(creature, 20)
    if target then
        creature:CastSpell(target, SPELL_ARCANE_SHOCK)
    end
    schedule(guid, "arcane_shock", 8000, function() onArcaneShock(creature, guid) end)
end

local function onOverload(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_OVERLOAD)
    end
    schedule(guid, "overload", 2000, function() onOverload(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    dischargeStage[guid] = 1
    enraged[guid] = false
    creature:Talk(SAY_AGGRO)
    schedule(guid, "chain_lightning", 8000, function() onChainLightning(creature, guid) end)
    schedule(guid, "arcane_shock", 5000, function() onArcaneShock(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

local function onTargetDied(_, creature)
    creature:Talk(SAY_KILL)
end

local function onDamageTaken(_, creature, _, _damage)
    local guid = creature:GetGUID()
    if enraged[guid] then
        return
    end
    local stage = dischargeStage[guid] or 1
    if creature:GetHealth() <= math.floor(creature:GetMaxHealth() * (100 - INTERVAL_MODIFIER * stage) / 100) then
        if stage == INTERVAL_SWITCH then
            enraged[guid] = true
            cancelTimers(guid)
            schedule(guid, "overload", 1200, function() onOverload(creature, guid) end)
            return
        end
        dischargeStage[guid] = stage + 1
        creature:Talk(SAY_ENERGY)
        creature:Talk(EMOTE_DISCHARGE_ENERGY)
        -- C++: if (IsHeroic()) DoCast(me, 46154) + DoCast(me, 46159)
        -- else DoCast(me, 44322) (cpp:125-131); stale "no difficulty
        -- bridge" retracted, creature:IsHeroic() bridged.
        if creature:IsHeroic() then
            creature:CastSpell(creature, SPELL_SUMMON_PURE_ENERGY_H1)
            creature:CastSpell(creature, SPELL_SUMMON_PURE_ENERGY_H2)
        else
            creature:CastSpell(creature, SPELL_SUMMON_PURE_ENERGY)
        end
    end
end

RegisterCreatureEvent(ENTRY_VEXALLUS, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_VEXALLUS, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_VEXALLUS, 3, onTargetDied)
RegisterCreatureEvent(ENTRY_VEXALLUS, 4, onCombatEnd)
RegisterCreatureEvent(ENTRY_VEXALLUS, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY_VEXALLUS, 23, onCombatEnd)
