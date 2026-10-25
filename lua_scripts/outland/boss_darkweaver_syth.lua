-- Darkweaver Syth (Sethekk Halls, Auchindoun) — Lua port of
-- src/server/scripts/Outland/Auchindoun/SethekkHalls/boss_darkweaver_syth.cpp
-- (1 CreatureScript — boss_darkweaver_syth (BossAI via GetSethekkHallsAI,
-- DATA_DARKWEAVER_SYTH = 0); registered from inside
-- AddSC_boss_darkweaver_syth(); loader decl/call per
-- outland_script_loader.cpp — the "Auchindoun - Sethekk Halls"
-- sub-block — verified from the loader this run).
-- Entry: 18968 — DB-side (no NPC_DARKWEAVER_SYTH constant in
-- sethekk_halls.h; ScriptName bindings are DB-side as usual).
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_darkweaver_syth" hits boss_darkweaver_syth.cpp (+ the
-- loader decl/call lines) only. No boss_darkweaver_syth lua existed.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat,
-- 3 OnTargetDied, 4 OnDied, 9 OnDamageTaken, 23 OnReset.
-- Ported arms (C++-exact for all modeled arms):
-- boss_darkweaver_sythAI::JustEngagedWith — schedules
-- EVENT_FLAME_SHOCK (2s), EVENT_ARCANE_SHOCK (4s),
-- EVENT_FROST_SHOCK (6s), EVENT_SHADOW_SHOCK (8s),
-- EVENT_CHAIN_LIGHTNING (15s); Talk(SAY_AGGRO 1) (event 1).
-- EVENT_FLAME_SHOCK — SelectTarget(Random, 0) (playerOnly defaults
-- FALSE per UnitAI.h:170 — pets/totems eligible) →
-- DoCast(target, SPELL_FLAME_SHOCK 34354) 2-arg (non-triggered);
-- re-arm randtime(10s,15s). Threat-list pool via GetAITargets (the
-- bridged threat list; midnight precedent); the Player filter was
-- dropped 2026-10-19.
-- EVENT_ARCANE_SHOCK — same shape, SPELL_ARCANE_SHOCK 37132, 4s init.
-- EVENT_FROST_SHOCK — same shape, SPELL_FROST_SHOCK 21401, 6s init.
-- EVENT_SHADOW_SHOCK — same shape, SPELL_SHADOW_SHOCK 30138, 8s init.
-- EVENT_CHAIN_LIGHTNING — SelectTarget(Random, 0) →
-- DoCast(target, SPELL_CHAIN_LIGHTNING 15659) 2-arg
-- (non-triggered); 15s init, 25s flat repeat.
-- boss_darkweaver_sythAI::DamageTaken — HealthBelowPctDamaged(90 /
-- 50 / 10, damage) one-shot latches (event 9 fires pre-damage —
-- moroes precedent) → SythSummoning(): Talk(SAY_SUMMON 0) +
-- DoCast(me, SPELL_SUMMON_SYTH_{ARCANE 33538, FIRE 33537, FROST
-- 33539, SHADOW 33540}, true) — 3-arg TRIGGERED self-casts
-- (creature:CastSpell(creature, spell, true); the C++ fires front/
-- back/left/right in ARCANE, FIRE, FROST, SHADOW order — preserved).
-- The IsNonMeleeSpellCast → InterruptNonMeleeSpells(false) leg rides
-- the no-cast-state bridge (IsCasting is a stub returning false,
-- lua_objects.go:330) — documented-only. The projection uses the
-- moroes-verified integer-floor form.
-- boss_darkweaver_sythAI::KilledUnit — TYPEID_PLAYER gate →
-- Talk(SAY_SLAY 2) (event 3; razuvious player-gated precedent).
-- boss_darkweaver_sythAI::JustDied — Talk(SAY_DEATH 3) (event 4;
-- sjonnir self-Talk precedent); the Lakka leg
-- (FindNearestCreature(NPC_LAKKA 18956, 500.0f) → Talk(1)) is PORTED
-- in sythDied (the old "no FindNearestCreature bridge" claim was
-- stale — lua_creature_events.go:2332, live-only per Health==0 skip).
-- All timers cancelled on 2/4/23 (gargolmar precedent); the 90/50/10
-- latches reset on 23 (C++ Reset → Initialize).
-- boss_darkweaver_sythAI::JustSummoned — PORTED below
-- (sythJustSummoned, event 19): SelectTarget(Random, 0) over the
-- threat list + summoned:EngageWithTarget (AttackStart twin,
-- shaffar beacon precedent). No entry gate in C++ (:171-175), so
-- none here either. The arm fires whenever a summon materializes:
-- the Go cast path runs EffectSummonType through the shared
-- summonTempCreature funnel, which fires event 19 on the summoner
-- (lua_creature_events.go:166) — on the same spell-data basis as
-- the summon casts themselves. The summons.Summon bookkeeping leg
-- (despawn on evade/death) stays documented-only: no
-- summon-tracking bridge.
-- DOCUMENTED-ONLY (in this header; only entry 18968 registered):
-- boss_darkweaver_sythAI::JustSummoned — summons.Summon
-- bookkeeping leg only (see above).
-- The four elemental adds (19203-19206, SPELL_{FLAME,ARCANE,FROST,
-- SHADOW}_BUFFET 33526-33529) have NO CreatureScript anywhere in
-- the C++ tree — nothing to port.

local ENTRY_DARKWEAVER_SYTH = 18968 -- DB-side; no NPC_ constant in sethekk_halls.h

local SAY_SUMMON = 0 -- boss_darkweaver_syth.cpp:38
local SAY_AGGRO = 1 -- boss_darkweaver_syth.cpp:39
local SAY_SLAY = 2 -- boss_darkweaver_syth.cpp:40
local SAY_DEATH = 3 -- boss_darkweaver_syth.cpp:41
local NPC_LAKKA = 18956 -- boss_darkweaver_syth.cpp:68
local SAY_LAKKA_FREE = 1 -- boss_darkweaver_syth.cpp:69

local SPELL_FROST_SHOCK = 21401 -- boss_darkweaver_syth.cpp:44
local SPELL_FLAME_SHOCK = 34354 -- boss_darkweaver_syth.cpp:45
local SPELL_SHADOW_SHOCK = 30138 -- boss_darkweaver_syth.cpp:46
local SPELL_ARCANE_SHOCK = 37132 -- boss_darkweaver_syth.cpp:47
local SPELL_CHAIN_LIGHTNING = 15659 -- boss_darkweaver_syth.cpp:49
local SPELL_SUMMON_SYTH_FIRE = 33537 -- boss_darkweaver_syth.cpp:51
local SPELL_SUMMON_SYTH_ARCANE = 33538 -- boss_darkweaver_syth.cpp:52
local SPELL_SUMMON_SYTH_FROST = 33539 -- boss_darkweaver_syth.cpp:53
local SPELL_SUMMON_SYTH_SHADOW = 33540 -- boss_darkweaver_syth.cpp:54

local timers = {}
local sythState = {}

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

-- C++ SelectTarget(Random, 0) — playerOnly defaults false (UnitAI.h:170),
-- so the pool is the whole threat list (pets/totems included). Dead
-- victims go offline via ShouldBeOffline / _IsTargetAcceptable
-- (Creature.cpp:2508, UNIT_STATE_DIED) and SelectTargetList skips
-- offline refs (UnitAI.h), so the pick filters dead entries; Go's
-- GetAITargets carries them until the async motion prune.
local function randomThreatTarget(creature)
    local targets = creature:GetAITargets()
    local alive = {}
    for _, t in ipairs(targets) do
        if not t:IsDead() then
            alive[#alive + 1] = t
        end
    end
    if #alive == 0 then
        return nil
    end
    return alive[math.random(#alive)]
end

-- C++ shock arms (:212-231): SelectTarget(Random, 0) →
-- DoCast(target, SPELL) 2-arg (non-triggered); re-arm
-- randtime(10s,15s) ({min,max} table — millisecond granularity).
local function shockTick(creature, guid, key, spell)
    local target = randomThreatTarget(creature)
    if target then
        creature:CastSpell(target, spell)
    end
    schedule(guid, key, {10000, 15000}, function()
        shockTick(creature, guid, key, spell)
    end)
end

-- C++ EVENT_CHAIN_LIGHTNING (:232-236): SelectTarget(Random, 0) →
-- DoCast(target, SPELL_CHAIN_LIGHTNING) 2-arg (non-triggered);
-- 25s flat repeat.
local function chainLightningTick(creature, guid)
    local target = randomThreatTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_CHAIN_LIGHTNING)
    end
    schedule(guid, "chain_lightning", 25000, function()
        chainLightningTick(creature, guid)
    end)
end

-- C++ SythSummoning (:198-208): Talk(SAY_SUMMON) + 4x DoCast(me,
-- SPELL_SUMMON_SYTH_*, true) — 3-arg TRIGGERED self-casts, in the
-- C++ order ARCANE, FIRE, FROST, SHADOW.
local function sythSummoning(creature)
    creature:Talk(SAY_SUMMON)
    creature:CastSpell(creature, SPELL_SUMMON_SYTH_ARCANE, true)
    creature:CastSpell(creature, SPELL_SUMMON_SYTH_FIRE, true)
    creature:CastSpell(creature, SPELL_SUMMON_SYTH_FROST, true)
    creature:CastSpell(creature, SPELL_SUMMON_SYTH_SHADOW, true)
end

-- C++ boss_darkweaver_sythAI::JustEngagedWith (:143-153): schedules
-- EVENT_FLAME_SHOCK 2s, EVENT_ARCANE_SHOCK 4s, EVENT_FROST_SHOCK
-- 6s, EVENT_SHADOW_SHOCK 8s, EVENT_CHAIN_LIGHTNING 15s;
-- Talk(SAY_AGGRO) — the auriaya precedent (event 1).
local function sythEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    -- C++ JustEngagedWith does NOT touch the summon latches (they
    -- live in Reset/Initialize); state is created lazily on first
    -- damage (constructor-equivalent all-false) and reset on 23.
    creature:Talk(SAY_AGGRO)
    schedule(guid, "flame_shock", 2000, function()
        shockTick(creature, guid, "flame_shock", SPELL_FLAME_SHOCK)
    end)
    schedule(guid, "arcane_shock", 4000, function()
        shockTick(creature, guid, "arcane_shock", SPELL_ARCANE_SHOCK)
    end)
    schedule(guid, "frost_shock", 6000, function()
        shockTick(creature, guid, "frost_shock", SPELL_FROST_SHOCK)
    end)
    schedule(guid, "shadow_shock", 8000, function()
        shockTick(creature, guid, "shadow_shock", SPELL_SHADOW_SHOCK)
    end)
    schedule(guid, "chain_lightning", 15000, function()
        chainLightningTick(creature, guid)
    end)
end

local function sythLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function sythReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    sythState[guid] = {summon90 = false, summon50 = false, summon10 = false}
end

-- C++ boss_darkweaver_sythAI::JustSummoned (:171-175):
-- if (Unit* target = SelectTarget(Random, 0))
-- summoned->AI()->AttackStart(target) — no entry gate, no
-- player-only restriction (playerOnly defaults FALSE, UnitAI.h:170);
-- the threat-list pool is randomThreatTarget above (shock precedent).
-- A nil pick no-ops through the safe-harbor skip (C++ passes the
-- raw SelectTarget result into AttackStart).
local function sythJustSummoned(event, creature, summoned)
    local target = randomThreatTarget(creature)
    if target then
        summoned:EngageWithTarget(target)
    end
end

-- C++ boss_darkweaver_sythAI::KilledUnit (:165-169):
-- if (who->GetTypeId() == TYPEID_PLAYER) Talk(SAY_SLAY) — the
-- razuvious player-gated precedent (event 3).
local function sythTargetDied(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ boss_darkweaver_sythAI::JustDied (:156-163): _JustDied()
-- (instance leg — documented-only) + Talk(SAY_DEATH) — the sjonnir
-- self-Talk precedent (event 4). The Lakka leg
-- (FindNearestCreature(NPC_LAKKA 18956, 500.0f, true) → Talk(1))
-- is PORTED below (bridge live, lua_creature_events.go:2332).
local function sythDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    local lakka = creature:FindNearestCreature(NPC_LAKKA, 500.0)
    if lakka then
        lakka:Talk(SAY_LAKKA_FREE)
    end
    cancelTimers(creature:GetGUID())
    sythState[creature:GetGUID()] = nil
end

-- C++ boss_darkweaver_sythAI::DamageTaken (:179-196):
-- HealthBelowPctDamaged(90/50/10, damage) one-shot latches →
-- SythSummoning(). Event 9 fires pre-damage (moroes precedent), so
-- the moroes-verified integer-floor projection form is used.
local function sythDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    -- Lazy init: C++ Initialize() runs in the constructor too, so a
    -- fresh spawn has all-false latches even if the engine never
    -- fired 23 (OnReset) for it.
    local state = sythState[guid]
    if state == nil then
        state = {summon90 = false, summon50 = false, summon10 = false}
        sythState[guid] = state
    end
    local health, maxHealth = creature:GetHealth(), creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    local postDamage = math.max(health - damage, 0)
    if (not state.summon90) and postDamage < math.floor(maxHealth * 90 / 100) then
        state.summon90 = true
        sythSummoning(creature)
    end
    if (not state.summon50) and postDamage < math.floor(maxHealth * 50 / 100) then
        state.summon50 = true
        sythSummoning(creature)
    end
    if (not state.summon10) and postDamage < math.floor(maxHealth * 10 / 100) then
        state.summon10 = true
        sythSummoning(creature)
    end
end

RegisterCreatureEvent(ENTRY_DARKWEAVER_SYTH, 19, sythJustSummoned)
RegisterCreatureEvent(ENTRY_DARKWEAVER_SYTH, 1, sythEnterCombat)
RegisterCreatureEvent(ENTRY_DARKWEAVER_SYTH, 2, sythLeaveCombat)
RegisterCreatureEvent(ENTRY_DARKWEAVER_SYTH, 3, sythTargetDied)
RegisterCreatureEvent(ENTRY_DARKWEAVER_SYTH, 4, sythDied)
RegisterCreatureEvent(ENTRY_DARKWEAVER_SYTH, 9, sythDamageTaken)
RegisterCreatureEvent(ENTRY_DARKWEAVER_SYTH, 23, sythReset)
