-- Golemagg the Incinerator (Molten Core) — Lua port of
-- src/server/scripts/EasternKingdoms/BlackrockMountain/MoltenCore/
-- boss_golemagg.cpp (boss_golemagg AI only); molten_core.h:37
-- (BOSS_GOLEMAGG_THE_INCINERATOR = 7, eighth boss), :61
-- (NPC_GOLEMAGG_THE_INCINERATOR = 11988).
-- Creature entry: 11988 Golemagg the Incinerator (C++ ScriptName
-- "boss_golemagg" per AddSC_boss_golemagg). No Talk lines in the boss
-- AI (the EMOTE_LOWHP = 0 Talk belongs to the unregistered
-- npc_core_rager add). SDComment upstream: "Timers need to be
-- confirmed, Golemagg's Trust need to be checked".
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 9 OnDamageTaken (pre-damage hook), 23 OnReset. Timers via
-- CreateLuaEvent; melee is engine-driven in Go (creature combat
-- tick), like C++ DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnReset: triggered
-- self-cast magmasplash 13879 (C++ DoCast(me, SPELL_MAGMASPLASH,
-- true)). OnEnterCombat: arm pyroblast 20228 7s then 7s,
-- non-triggered on a threat-list target alive (C++ DoCast default is
-- triggered=false — vaelastrasz convention; C++ SelectTarget(Random,
-- 0) defaults playerOnly=false, so pets/totems are eligible — the old
-- "any alive player" wording was wrong). Nil-target ticks cast nothing
-- but keep
-- the schedule (jeklik convention). OnDamageTaken(9, pre-damage
-- hook): current (pre-damage) health strictly below 10%
-- (C++ HealthBelowPct(10) — the engine's event 9 fires before damage is
-- applied, matching C++ Unit::DealDamage) and not HasAura(19953)
-- -> triggered self-cast enrage 19953, arm earthquake 19798 3s then
-- 3s, non-triggered DoCastVictim (C++-exact once-guard via the
-- creature HasAura bridge, engine/world/lua_objects.go).
-- OnDied/OnLeaveCombat/OnReset: cancel timers.
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING queue gate and
-- the post-event casting check have no live UNIT_STATE_CASTING read bridge (IsCasting is constant-false — Go casts are fire-and-forget) — timers fire
-- unconditionally (jeklik convention); no instance-script model —
-- boss admission via the luaBossAI shim (BossAI::JustEngagedWith and
-- the BOSS_GOLEMAGG_THE_INCINERATOR bookkeeping arms skipped); the
-- npc_core_rager AI in the same C++ file (mangle 19820 7s then 10s
-- DoCastVictim; below 50% health adds Golemagg's trust 20553 to
-- itself, Talk(EMOTE_LOWHP=0) and self full-heal while Golemagg is
-- alive) is not registered — no NPC_CORE_RAGER constant exists in the
-- C++ tree and the creature_template ScriptName binding is DB-side,
-- so the add entry cannot be verified from the C++ sources alone
-- (garr firesworn convention); its below-50% trust arm would
-- additionally need an instance/creature-lookup bridge and a
-- SetFullHealth bridge.

local ENTRY_GOLEMAGG = 11988

local SPELL_MAGMASPLASH = 13879
local SPELL_PYROBLAST = 20228
local SPELL_EARTHQUAKE = 19798
local SPELL_ENRAGE = 19953

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

-- C++ SelectTarget(Random, 0): threat-list target alive, any unit
-- (playerOnly defaults false).
local function randomThreatTarget(creature)
    local targets = creature:GetAITargets()
    local out = {}
    for _, unit in ipairs(targets) do
        if not unit:IsDead() then
            out[#out + 1] = unit
        end
    end
    if #out == 0 then
        return nil
    end
    return out[math.random(#out)]
end

-- C++ EVENT_PYROBLAST: non-triggered cast 20228 on a threat-list
-- target alive; re-arm 7s.
local function onPyroblast(creature, guid)
    local pick = randomThreatTarget(creature)
    if pick then
        creature:CastSpell(pick, SPELL_PYROBLAST)
    end
    schedule(guid, "pyroblast", 7000, function()
        onPyroblast(creature, guid)
    end)
end

-- C++ EVENT_EARTHQUAKE: non-triggered DoCastVictim(19798); re-arm 3s.
local function onEarthquake(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_EARTHQUAKE)
    end
    schedule(guid, "earthquake", 3000, function()
        onEarthquake(creature, guid)
    end)
end

local function golemaggEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "pyroblast", 7000, function()
        onPyroblast(creature, guid)
    end)
end

local function golemaggResetState(guid)
    cancelTimers(guid)
end

local function golemaggLeaveCombat(event, creature)
    golemaggResetState(creature:GetGUID())
end

local function golemaggDied(event, creature, killer)
    golemaggResetState(creature:GetGUID())
end

-- C++ Reset: triggered self-cast of magmasplash 13879 (C++-exact);
-- the BossAI::Reset instance bookkeeping has no bridge.
local function golemaggReset(event, creature)
    golemaggResetState(creature:GetGUID())
    -- Reset-only arm (boss_golemagg.cpp); alive-gated.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_MAGMASPLASH, true)
    end
end

-- C++ DamageTaken: current (pre-damage) health strictly below 10%
-- (HealthBelowPct(10) = GetHealth() < floor(max*10/100) — the engine's
-- event 9 fires before damage is applied, matching C++ Unit::DealDamage)
-- and not HasAura(SPELL_ENRAGE) -> triggered self-cast enrage 19953
-- and arm the earthquake cycle (C++-exact; the creature HasAura bridge
-- means a dispelled enrage re-casts, matching C++).
local function golemaggDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    if creature:HasAura(SPELL_ENRAGE) then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if creature:GetHealth() < math.floor(maxHealth * 10 / 100) then
        creature:CastSpell(creature, SPELL_ENRAGE, true)
        schedule(guid, "earthquake", 3000, function()
            onEarthquake(creature, guid)
        end)
    end
end

RegisterCreatureEvent(ENTRY_GOLEMAGG, 1, golemaggEnterCombat)
RegisterCreatureEvent(ENTRY_GOLEMAGG, 2, golemaggLeaveCombat)
RegisterCreatureEvent(ENTRY_GOLEMAGG, 4, golemaggDied)
RegisterCreatureEvent(ENTRY_GOLEMAGG, 9, golemaggDamageTaken)
RegisterCreatureEvent(ENTRY_GOLEMAGG, 23, golemaggReset)
