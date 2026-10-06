-- Temple of Ahn'Qiraj: Emperor Vek'nilash — Lua port of
-- src/server/scripts/Kalimdor/TempleOfAhnQiraj/boss_twinemperors.cpp
-- (boss_veknilashAI : public boss_twinemperorsAI; GetAI via
-- GetAQ40AI<boss_veknilashAI>(creature); registered in
-- AddSC_boss_twinemperors(); kalimdor loader decl 94 — eighth
-- "// Temple of ahn'qiraj" group, after skeram, before ouro).
-- Entry: 15275 = NPC_VEKNILASH (temple_of_ahnqiraj.h:75; the
-- creature_template ScriptName binding stays DB-side).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (the self-contained in-combat legs):
-- - Unbalancing Strike 26613 (DoCastVictim): init urand(8s,18s)
--   -> urand(8s,20s). Victim cast (jeklik convention).
-- - Uppercut 26007: C++ DoCast on a random MELEE-range target
--   (SelectTarget Random, 0, NOMINAL_MELEE_RANGE, true), init
--   urand(14s,29s) -> urand(15s,30s). No range/SelectTarget
--   bridge; ported as victim-cast (kri convention) — the victim
--   of the melee emperor is by definition within melee range.
-- - Berserk 26662 (CheckEnrage): DoCast self, init 15min ->
--   repeat 60min. The IsNonMeleeSpellCast gate has no bridge —
--   fixed timer.
-- Unmodeled (documented-only, no bridges):
-- - TwinReset / BossAI ctor leg DATA_TWIN_EMPERORS: instance
--   bridge, standing.
-- - Shared health (DamageTaken %mirroring via GetOtherBoss +
--   SpellHit SPELL_HEAL_BROTHER 7393 rebalance): cross-boss via
--   instance->GetCreature(DATA_VEKLOR) — no instance bridge.
-- - TryHealBrother: VN casts 7393 on VEKLOR when within 60y —
--   cross-boss, no bridge.
-- - TeleportToMyBrother + SetAfterTeleport + TryActivateAfterTTelep:
--   position swap + stun/interrupt/ResetThreatList + twin visual
--   cast 26638 + post-teleport threat arm — no position/SelectTarget/
--   InterruptNonMeleeSpells/ResetThreatList/AttackStart bridges.
-- - HandleBugs / CastSpellOnBug (SPELL_MUTATE_BUG 802): creature
--   grid enumeration + SetFaction + threat-victim AttackStart +
--   AddAura — no bridges.
-- - KilledUnit/JustEngagedWith/JustDied DoPlaySoundToSet
--   (SOUND_VN_AGGRO 8661 / KILL 8662 / DEATH 8660): no sound
--   bridge; zero Talk() in C++.
local ENTRY = 15275

local SPELL_UNBALANCING_STRIKE = 26613
local SPELL_UPPERCUT = 26007
local SPELL_BERSERK = 26662

local timers = {}
local flags = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
    flags[guid] = nil
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

-- C++ EVENT_UNBALANCING_STRIKE: DoCastVictim(SPELL_UNBALANCING_STRIKE),
-- init urand(8000,18000) -> urand(8000,20000).
local function onUnbalancingStrike(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_UNBALANCING_STRIKE)
    end
    schedule(guid, "us", math.random(8000, 20000), function()
        onUnbalancingStrike(creature, guid)
    end)
end

-- C++ EVENT_UPPERCUT: DoCast on random melee-range target, init
-- urand(14000,29000) -> urand(15000,30000). No range/SelectTarget
-- bridge; ported as victim-cast (kri convention).
local function onUppercut(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_UPPERCUT)
    end
    schedule(guid, "uppercut", math.random(15000, 30000), function()
        onUppercut(creature, guid)
    end)
end

-- C++ CheckEnrage: DoCast(me, SPELL_BERSERK) at 15min, re-armed
-- to 60min. The IsNonMeleeSpellCast gate has no bridge — fixed
-- timer.
local function onBerserk(creature, guid)
    creature:CastSpell(creature, SPELL_BERSERK)
    schedule(guid, "berserk", 3600000, function()
        onBerserk(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    flags[guid] = {}
    schedule(guid, "us", math.random(8000, 18000), function()
        onUnbalancingStrike(creature, guid)
    end)
    schedule(guid, "uppercut", math.random(14000, 29000), function()
        onUppercut(creature, guid)
    end)
    schedule(guid, "berserk", 900000, function()
        onBerserk(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 23, onReset)
