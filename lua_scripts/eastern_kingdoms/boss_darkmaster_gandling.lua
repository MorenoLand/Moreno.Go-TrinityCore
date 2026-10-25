-- Darkmaster Gandling (1853), Scholomance final boss — Lua port of
-- src/server/scripts/EasternKingdoms/Scholomance/
-- boss_darkmaster_gandling.cpp (creature AI only); entry C++-named in
-- scholomance.h (NPC_DARKMASTER_GANDLING=1853 :42, DATA_DARKMASTERGANDLING=6
-- :36; RegisterScholomanceCreatureAI binds the creature_template ScriptName
-- DB-side). Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat,
-- 4 OnDied, 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
--
-- Modeled arms (C++-exact where bridges exist):
-- OnEnterCombat: arcane missiles 15790 at 4.5s->8s / shadow shield 12040
-- (non-triggered self) at 12s->{14s,28s} / curse 18702 at 2s->{15s,27s} /
-- shadow portal 17950 at 15s->{17s,27s}. Triggered DoCastVictim ->
-- GetVictim + CastSpell(victim, spell, true), nil-victim keeps schedule
-- (moroes/maiden triggered convention; the C++ DoCast's triggered flag
-- -> CastSpell's third argument). urand re-arms -> math.random
-- (sulfuron convention). Shadow portal: only while own current health is
-- strictly above 3% (HealthAbovePct(3) current-health read); target =
-- random alive player within 100yd (SelectTarget(Random, 0, 100, true),
-- janalai/kelris randomPlayerInRange convention), nil pick casts nothing
-- but the re-arm still happens — the {17s,27s} schedule lives INSIDE the
-- health gate in C++, so below 3% the event is never re-armed and dies
-- (modeled by skipping the reschedule entirely). IsSummonedBy's
-- Talk(YELL_SUMMONED 0) is document-only with its MoveRandom(5) (no
-- MotionMaster bridge, gizrul precedent) — Gandling is map-spawned, not
-- summoned, so the arm never fires in normal play.
--
-- Documented-unmodeled (no bridges):
-- Reset/JustDied/JustEngagedWith GO_GATE_GANDLING (177374) state legs
-- (SetGoState ACTIVE/ACTIVE/READY) PORTED below via
-- creature:FindNearestGameObject — C++ used instance GetGuidData;
-- the entry is unique in the instance, so the target is identical. spell_shadow_portal (17950: picks one of six
-- room portals 17863/17939/17943/17944/17946/17948 among rooms whose
-- gates are still open, up to 6 tries) + spell_shadow_portal_rooms
-- (17863..: summons 3x NPC_RISEN_GUARDIAN 11598 at the room's SummonPos,
-- MoveRandom, SetData phase, closes that room's gate) — no SpellScript
-- bridge (nightbane precedent), so the shadow-portal cast is a lone
-- triggered spell with no room effect. UpdateAI UNIT_STATE_CASTING queue +
-- post-event gates + BossAI::JustEngagedWith + DATA_DARKMASTERGANDLING
-- bookkeeping have no bridges (luaBossAI shim).

local ENTRY_GANDLING = 1853
local GO_GATE_GANDLING = 177374 -- scholomance.h:49

local SPELL_ARCANE_MISSILES = 15790
local SPELL_SHADOW_SHIELD = 12040
local SPELL_CURSE = 18702
local SPELL_SHADOW_PORTAL = 17950

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

local function castOnVictim(creature, spellId, triggered)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, spellId, triggered)
    end
end

local function healthAbovePct(creature, pct)
    local maxHealth = creature:GetMaxHealth()
    return maxHealth > 0 and creature:GetHealth() > math.floor(maxHealth * pct / 100)
end

-- C++ SelectTarget(Random, 0, 100, true): random alive
-- threat-list player within maxDist yards.
local function randomThreatPlayerInRange(creature, maxDist)
    local targets = creature:GetAITargets()
    local out = {}
    for _, unit in ipairs(targets) do
        if unit:GetObjectType() == "Player" and not unit:IsDead()
                and creature:GetDistance(unit) <= maxDist then
            out[#out + 1] = unit
        end
    end
    if #out == 0 then
        return nil
    end
    return out[math.random(#out)]
end

-- C++ EVENT_ARCANEMISSILES: triggered DoCastVictim(15790); Repeat(8s).
local function onArcaneMissiles(creature, guid)
    castOnVictim(creature, SPELL_ARCANE_MISSILES, true)
    schedule(guid, "arcaneMissiles", 8000, function()
        onArcaneMissiles(creature, guid)
    end)
end

-- C++ EVENT_SHADOWSHIELD: DoCast(me, 12040) non-triggered; Repeat(14s, 28s).
local function onShadowShield(creature, guid)
    creature:CastSpell(creature, SPELL_SHADOW_SHIELD)
    schedule(guid, "shadowShield", math.random(14000, 28000), function()
        onShadowShield(creature, guid)
    end)
end

-- C++ EVENT_CURSE: triggered DoCastVictim(18702); Repeat(15s, 27s).
local function onCurse(creature, guid)
    castOnVictim(creature, SPELL_CURSE, true)
    schedule(guid, "curse", math.random(15000, 27000), function()
        onCurse(creature, guid)
    end)
end

-- C++ EVENT_SHADOW_PORTAL: only if HealthAbovePct(3); triggered DoCast on
-- the random player target; Repeat(17s, 27s) lives inside the gate.
local function onShadowPortal(creature, guid)
    if not healthAbovePct(creature, 3) then
        return
    end
    local target = randomThreatPlayerInRange(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_SHADOW_PORTAL, true)
    end
    schedule(guid, "shadowPortal", math.random(17000, 27000), function()
        onShadowPortal(creature, guid)
    end)
end

-- C++ JustEngagedWith: BossAI::JustEngagedWith (shim) + the four events +
-- GO_GATE_GANDLING SetGoState(READY) PORTED below (FindNearestGameObject).
local function gandlingEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "arcaneMissiles", 4500, function()
        onArcaneMissiles(creature, guid)
    end)
    schedule(guid, "shadowShield", 12000, function()
        onShadowShield(creature, guid)
    end)
    schedule(guid, "curse", 2000, function()
        onCurse(creature, guid)
    end)
    schedule(guid, "shadowPortal", 15000, function()
        onShadowPortal(creature, guid)
    end)
    local gate = creature:FindNearestGameObject(GO_GATE_GANDLING, 100.0)
    if gate then gate:SetGoState(1) end -- GO_STATE_READY
end

local function gandlingLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function gandlingDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    local gate = creature:FindNearestGameObject(GO_GATE_GANDLING, 100.0)
    if gate then gate:SetGoState(0) end -- GO_STATE_ACTIVE
end

-- C++ Reset: _Reset + GO_GATE_GANDLING SetGoState(ACTIVE) PORTED below.
local function gandlingReset(event, creature)
    cancelTimers(creature:GetGUID())
    local gate = creature:FindNearestGameObject(GO_GATE_GANDLING, 100.0)
    if gate then gate:SetGoState(0) end -- GO_STATE_ACTIVE
end

RegisterCreatureEvent(ENTRY_GANDLING, 1, gandlingEnterCombat)
RegisterCreatureEvent(ENTRY_GANDLING, 2, gandlingLeaveCombat)
RegisterCreatureEvent(ENTRY_GANDLING, 4, gandlingDied)
RegisterCreatureEvent(ENTRY_GANDLING, 23, gandlingReset)
