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
-- The C++ UpdateAI IsWithinMeleeRange gate for the Earth Shock
-- reschedule is ported via creature:GetDistance (ported; the old
-- "no range bridge" claim was stale).
-- Ported arms (the self-contained in-combat legs):
-- - Arcane Explosion 26192 (DoCastAOE, triggered): self-cast,
--   init urand(6s,12s) -> urand(8s,18s). The SpellScript target
--   filter (players + pets only) is no-bridge (SpellScript has
--   zero Lua usage, viscidus precedent) — documented-only.
-- - True Fulfillment 785 (15s init -> 20-30s): random player via
--   GetPlayersInWorld filtered by map/instance/alive + 45.0f range
--   (C++ SelectTarget Random 1, 45.0f, true; the old unfiltered pick
--   could hit dead players in other zones); the SpellScript charm
--   effect legs (dismount 61286 + 2313 recast) are no-bridge,
--   documented-only.
-- - Blink (30-45s init -> 10-30s): self-cast one of {4801, 8195,
--   20449} (C++ BlinkSpells). ResetThreatList PORTED 2026-10-19
--   (creature:ResetAllThreat, bridged lua_objects.go); SetVisible(true)
--   PORTED via the creature:SetVisible bridge (Unit.cpp:8656).
-- - Earth Shock 26194 (DoCastVictim, 2s repeat): victim cast
--   (fankriss GetVictim convention).
-- - Split at 75/50/25% (latched, event 9 projected-health check,
--   sartura convention): Talk SAY_SPLIT + BLINK re-armed to 2s.
--   postDamage idiom 2026-10-21: the C++ UpdateAI GetHealthPct() < _hpct
--   arm reads post-damage health and never fires on the killing blow,
--   so the port projects (health - damage) clamped at 0 with a
--   postDamage > 0 guard (nefarian rule).
--   The DoCastAOE SPELL_SUMMON_IMAGES 747 leg is no-bridge
--   (SummonCreature zero Lua usage) — documented-only; SetVisible(false)
--   PORTED via the creature:SetVisible bridge (Unit.cpp:8656).
-- - Talk: SAY_AGGRO 0 on engage, SAY_SLAY 1 on kill,
--   SAY_SPLIT 2 at split thresholds, SAY_DEATH 3 on death.
-- Unmodeled (documented-only, no bridges):
-- - JustSummoned image-positioning leg: BlinkSpells shuffle over
--   the summoned images, health seeding (10/20/50% by boss HP),
--   AttackStart on random target, summons.Summon bookkeeping —
--   all inside the summon-image AI, unreachable without a
--   SummonCreature/JustSummoned bridge.
-- - JustDied !IsSummon gate + image DespawnOrUnsummon PORTED
--   below (creature:IsSummon bridge, engine/world/
--   lua_creature_events.go + lua_objects.go; the BossAI::JustDied
--   instance leg stays out — no instance bridge).
-- - EnterEvadeMode image self-despawn PORTED below on the event-2
--   evade proxy (creature:IsSummon gate; the boss itself never
--   despawns on evade, C++-exact).
-- - BossAI ctor leg DATA_SKERAM and _Reset() — instance bridge,
--   standing.
-- - SelectTarget/AttackStart/SetVisible/
--   InterruptNonMeleeSpells/GetThreatModifiers — no exact
--   threat-list SelectTarget bridge in the engine's Lua API;
--   random-target selection IS bridged via the documented
--   maiden_of_virtue idiom (used in this file, lines 123-144).
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
    creature:CastSpell(creature, SPELL_ARCANE_EXPLOSION, true)
    schedule(guid, "ae", math.random(8000, 18000), function()
        onArcaneExplosion(creature, guid)
    end)
end

-- C++ EVENT_FULLFILMENT: SelectTarget Random,1,45.0f,true ->
-- DoCast(target, SPELL_TRUE_FULFILLMENT), init 15000 ->
-- urand(20000,30000). Random-player pick via GetPlayersInWorld
-- filtered map/instance/alive + 45.0f (C++ SelectTarget Random 1,
-- 45.0f, true). The C++ position=1 arm skips the first entry in
-- MaxThreat order (the current victim/tank, UnitAI.h:162-163)
-- before the player/range predicate, so the tank is excluded
-- here via GetVictim (malchezaar precedent); threat-list-empty
-- no-cast falls out of the empty-candidates arm.
local function onTrueFulfillment(creature, guid)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local victim = creature:GetVictim()
    local victimGuid = victim ~= nil and victim:GetGUID() or nil
    local cands = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p and p:IsAlive() and p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and creature:IsWithinDist(p, 45.0) and p:GetGUID() ~= victimGuid then
            cands[#cands + 1] = p
        end
    end
    if #cands > 0 then
        creature:CastSpell(cands[math.random(#cands)], SPELL_TRUE_FULFILLMENT)
    end
    schedule(guid, "fulfillment", math.random(20000, 30000), function()
        onTrueFulfillment(creature, guid)
    end)
end

-- C++ EVENT_BLINK: DoCast(me, BlinkSpells[urand(0,2)]), init
-- urand(30000,45000) -> urand(10000,30000). ResetThreatList PORTED
-- 2026-10-19 (creature:ResetAllThreat); SetVisible(true) PORTED
-- 2026-10-23 on the live creature:SetVisible bridge (below, in
-- C++ order — the stale "SetVisible: no bridge" claim is retracted).
local function onBlink(creature, guid)
    creature:CastSpell(creature, BLINK_SPELLS[math.random(#BLINK_SPELLS)])
    -- C++ EVENT_BLINK: ResetThreatList() after the blink cast (bridged
    -- lua_objects.go:389; the old "no bridge" header claims were stale).
    creature:ResetAllThreat()
    -- C++ EVENT_BLINK: me->SetVisible(true) (boss_skeram.cpp:174,
    -- creature:SetVisible bridge, Unit::SetVisible Unit.cpp:8656),
    -- in C++ order (after ResetThreatList, before the re-arm).
    creature:SetVisible(true)
    schedule(guid, "blink", math.random(10000, 30000), function()
        onBlink(creature, guid)
    end)
end

-- C++ EVENT_EARTH_SHOCK: DoCastVictim(SPELL_EARTH_SHOCK), 2s
-- repeat. C++ UpdateAI (boss_skeram.cpp:193-196) re-arms
-- RescheduleEvent(EVENT_EARTH_SHOCK, 2s) every tick while
-- IsWithinMeleeRange(GetVictim()), so the cast only fires ~2s
-- after the victim leaves melee range. Emulated with
-- creature:GetDistance(victim) (the old "no range bridge" header
-- note was stale — used by moroes/midnight/netherspite/etc.):
-- in-melee ticks skip the cast and re-arm the 2s per-key timer,
-- exactly what the C++ re-arm does.
local function onEarthShock(creature, guid)
    local victim = creature:GetVictim()
    if victim and creature:GetDistance(victim) > 5.0 then
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
-- (sartura/huhuran convention). Summon cast has no bridge;
-- SetVisible(false) PORTED via the creature:SetVisible bridge;
-- ported as the Talk + visibility + blink re-arm.
local function onDamageTaken(event, creature, attacker, damage)
    -- C++ split check is !IsSummon-gated (boss_skeram.cpp:184);
    -- images run the same AI but never split.
    if creature:IsSummon() then
        return
    end
    local guid = creature:GetGUID()
    local s = st(guid)
    if s.splitHpct <= 0 then
        return
    end
    local health, maxHealth = creature:GetHealth(), creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    -- C++ boss_skeram.cpp:184 is the FLOAT form: me->GetHealthPct() < _hpct
    -- (100.f*health/maxHealth), NOT HealthBelowPct's floored CalculatePct
    -- (Unit.h:872). The 2690b455 floor port deviated up to 1 HP at
    -- non-divisible thresholds; postDamage*100 < maxHealth*_hpct is the
    -- integer-exact twin of the float compare. UpdateAI never runs dead,
    -- so the port guards the killing blow with postDamage > 0.
    local postDamage = math.max(health - damage, 0)
    if postDamage > 0 and postDamage * 100 < maxHealth * s.splitHpct then
        s.splitHpct = s.splitHpct - 25
        creature:Talk(SAY_SPLIT)
        -- C++ UpdateAI split (boss_skeram.cpp:184-190): _hpct -= 25 ->
        -- me->SetVisible(false) (creature:SetVisible bridge,
        -- Unit::SetVisible Unit.cpp:8656) -> RescheduleEvent(EVENT_BLINK,
        -- 2s), in C++ order.
        creature:SetVisible(false)
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
    -- C++ boss_skeram.cpp:81-85 EnterEvadeMode: ScriptedAI base
    -- then, only for images, me->DespawnOrUnsummon() — the event-2
    -- handler is the evade proxy (drakkari precedent); the boss
    -- (IsSummon false) keeps its existing cancel-only behavior.
    if creature:IsSummon() then
        creature:Despawn()
    end
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
    -- C++ boss_skeramAI::Reset (boss_skeram.cpp:70-74): Initialize()
    -- (split-hpct re-init, covered by st()) -> me->SetVisible(true)
    -- (creature:SetVisible bridge, Unit::SetVisible Unit.cpp:8656).
    creature:SetVisible(true)
end

local function onTargetDied(event, creature, victim)
    creature:Talk(SAY_SLAY)
end

local function onDied(event, creature, killer)
    -- C++ boss_skeram.cpp:129-136 JustDied: images DespawnOrUn-
    -- summon with no Talk; only the true boss Talks (the BossAI::
    -- JustDied instance leg stays out — no instance bridge).
    if creature:IsSummon() then
        creature:Despawn()
    else
        creature:Talk(SAY_DEATH)
    end
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 3, onTargetDied)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY, 23, onReset)
