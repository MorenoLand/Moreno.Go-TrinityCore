-- Temple of Ahn'Qiraj: Prophet Skeram — Lua port of
-- src/server/scripts/Kalimdor/TempleOfAhnQiraj/boss_skeram.cpp
-- (boss_skeramAI : public BossAI(creature, DATA_SKERAM);
-- GetAI via GetAQ40AI<boss_skeramAI>(creature) (AQ40ScriptName
-- "instance_temple_of_ahnqiraj" gate); AddSC_boss_skeram at end
-- registers boss_skeram + spell_skeram_arcane_explosion +
-- spell_skeram_true_fulfillment; kalimdor loader decl 93 / call
-- per kalimdor_script_loader.cpp — seventh "// Temple of
-- ahn'qiraj" loader group, after sartura, before twinemperors).
-- Whole-server-tree grep confirms the cpp as the sole source of
-- "boss_skeram" (loader decl/call lines only otherwise).
-- Entry: Prophet Skeram = 15263 (temple_of_ahnqiraj.h:71
-- NPC_SKERAM = 15263; creature_template ScriptName binding stays
-- DB-side). DATA_SKERAM = 0 (temple_of_ahnqiraj.h:30).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat,
-- 3 OnTargetDied, 4 OnDied, 9 OnDamageTaken, 23 OnReset.
-- Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- The C++ UpdateAI gating on !me->IsWithinMeleeRange for the
-- Earth Shock reschedule has no range bridge; ported as the
-- fankriss victim-cast convention.
-- Ported arms (the self-contained in-combat legs):
-- - Arcane Explosion 26192 (DoCastAOE, triggered): self-cast,
--   init urand(6s,12s) -> urand(8s,18s). The SpellScript target
--   filter (players + pets only) is no-bridge (SpellScript has
--   zero Lua usage, viscidus precedent) — documented-only.
-- - True Fulfillment 785 (15s init -> 20-30s): random player via
--   GetPlayersInWorld (huhuran convention — C++ SelectTarget
--   Random 1, 45.0f, true has no bridge); the SpellScript charm
--   effect legs (dismount 61286 + 2313 recast) are no-bridge,
--   documented-only.
-- - Blink (30-45s init -> 10-30s): self-cast one of {4801, 8195,
--   20449} (C++ BlinkSpells). The ResetThreatList + SetVisible
--   legs are no-bridge — documented-only.
-- - Earth Shock 26194 (DoCastVictim, 2s repeat): victim cast
--   (fankriss GetVictim convention).
-- - Split at 75/50/25% (latched, event 9 projected-health check,
--   sartura convention): Talk SAY_SPLIT + BLINK re-armed to 2s.
--   The DoCastAOE SPELL_SUMMON_IMAGES 747 leg is no-bridge
--   (SummonCreature zero Lua usage) and SetVisible(false) has no
--   bridge — documented-only.
-- - Talk: SAY_AGGRO 0 on engage, SAY_SLAY 1 on kill,
--   SAY_SPLIT 2 at split thresholds, SAY_DEATH 3 on death.
-- Unmodeled (documented-only, no bridges):
-- - JustSummoned image-positioning leg: BlinkSpells shuffle over
--   the summoned images, health seeding (10/20/50% by boss HP),
--   AttackStart on random target, summons.Summon bookkeeping —
--   all inside the summon-image AI, unreachable without a
--   SummonCreature/JustSummoned bridge.
-- - JustDied !IsSummon gate + image DespawnOrUnsummon +
--   BossAI::JustDied leg: IsSummon has no Lua bridge, so the
--   DEATH Talk is ungated here (C++-verbatim would suppress it
--   for images).
-- - EnterEvadeMode: image self-despawn (IsSummon leg) — no bridge.
-- - BossAI ctor leg DATA_SKERAM and _Reset() — instance bridge,
--   standing.
-- - SelectTarget/AttackStart/ResetThreatList/SetVisible/
--   InterruptNonMeleeSpells/GetThreatModifiers — no bridges
--   anywhere in the engine's Lua API.
local ENTRY = 15263

local SPELL_ARCANE_EXPLOSION = 26192
local SPELL_TRUE_FULFILLMENT = 785
local SPELL_EARTH_SHOCK = 26194

local BLINK_SPELLS = { 4801, 8195, 20449 }

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_SPLIT = 2
local SAY_DEATH = 3

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

local function st(guid)
    local s = flags[guid]
    if not s then
        s = { splitHpct = 75 }
        flags[guid] = s
    end
    return s
end

-- C++ EVENT_ARCANE_EXPLOSION: DoCastAOE(SPELL_ARCANE_EXPLOSION,
-- true), init urand(6000,12000) -> urand(8000,18000).
local function onArcaneExplosion(creature, guid)
    creature:CastSpell(creature, SPELL_ARCANE_EXPLOSION)
    schedule(guid, "ae", math.random(8000, 18000), function()
        onArcaneExplosion(creature, guid)
    end)
end

-- C++ EVENT_FULLFILMENT: SelectTarget Random,1,45.0f,true ->
-- DoCast(target, SPELL_TRUE_FULFILLMENT), init 15000 ->
-- urand(20000,30000). Random-player pick via GetPlayersInWorld
-- (huhuran convention).
local function onTrueFulfillment(creature, guid)
    local players = GetPlayersInWorld()
    if #players > 0 then
        local target = players[math.random(#players)]
        creature:CastSpell(target, SPELL_TRUE_FULFILLMENT)
    end
    schedule(guid, "fulfillment", math.random(20000, 30000), function()
        onTrueFulfillment(creature, guid)
    end)
end

-- C++ EVENT_BLINK: DoCast(me, BlinkSpells[urand(0,2)]), init
-- urand(30000,45000) -> urand(10000,30000). ResetThreatList /
-- SetVisible: no bridge.
local function onBlink(creature, guid)
    creature:CastSpell(creature, BLINK_SPELLS[math.random(#BLINK_SPELLS)])
    schedule(guid, "blink", math.random(10000, 30000), function()
        onBlink(creature, guid)
    end)
end

-- C++ EVENT_EARTH_SHOCK: DoCastVictim(SPELL_EARTH_SHOCK), 2s
-- repeat (UpdateAI reschedules 2s while in melee range; no
-- range bridge, so fixed 2s — fankriss convention).
local function onEarthShock(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_EARTH_SHOCK)
    end
    schedule(guid, "earthshock", 2000, function()
        onEarthShock(creature, guid)
    end)
end

-- C++ UpdateAI per-tick: !me->IsSummon() && HealthBelowPct(_hpct)
-- -> DoCastAOE(SPELL_SUMMON_IMAGES, true) + Talk(SAY_SPLIT),
-- _hpct -= 25, SetVisible(false), RescheduleEvent(EVENT_BLINK,
-- 2s). Eluna event 9 is pre-damage, so projected-health check
-- (sartura/huhuran convention). Summon cast + SetVisible: no
-- bridge; ported as the Talk + blink re-arm.
local function onDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local s = st(guid)
    if s.splitHpct <= 0 then
        return
    end
    local health, maxHealth = creature:GetHealth(), creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if (health - damage) * 100 < s.splitHpct * maxHealth then
        s.splitHpct = s.splitHpct - 25
        creature:Talk(SAY_SPLIT)
        schedule(guid, "blink", 2000, function()
            onBlink(creature, guid)
        end)
    end
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    st(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "ae", math.random(6000, 12000), function()
        onArcaneExplosion(creature, guid)
    end)
    schedule(guid, "fulfillment", 15000, function()
        onTrueFulfillment(creature, guid)
    end)
    schedule(guid, "blink", math.random(30000, 45000), function()
        onBlink(creature, guid)
    end)
    schedule(guid, "earthshock", 2000, function()
        onEarthShock(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onTargetDied(event, creature, victim)
    creature:Talk(SAY_SLAY)
end

local function onDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 3, onTargetDied)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY, 23, onReset)
