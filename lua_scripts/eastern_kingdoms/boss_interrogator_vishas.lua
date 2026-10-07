-- Interrogator Vishas (Scarlet Monastery, Graveyard wing) — Lua port of
-- src/server/scripts/EasternKingdoms/ScarletMonastery/
-- boss_interrogator_vishas.cpp (boss_interrogator_vishas AI only);
-- scarlet_monastery.h names DATA_INTERROGATOR_VISHAS = 0 which the
-- BossAI constructor consumes, and DATA_VORREL with NPC_VORREL 3981.
-- Creature entry: 3983 Interrogator Vishas (no NPC_ constant exists
-- in the C++ tree — RegisterScarletMonasteryCreatureAI binds the
-- creature_template ScriptName DB-side; entries classicdb-cited:
-- classicdb.ch/?npc=3983 and www.wowhead.com/classic/npc=3983).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat,
-- 3 OnTargetDied, 4 OnDied, 9 OnDamageTaken (pre-damage hook),
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in
-- Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnEnterCombat
-- Talk(SAY_AGGRO 0) + EVENT_SHADOW_WORD_PAIN armed at 5s.
-- EVENT_SHADOW_WORD_PAIN: non-triggered DoCastVictim(shadow word:
-- pain 2767) -> GetVictim + CastSpell, nil-victim ticks cast nothing
-- but keep the schedule (jeklik convention), re-arm {5s,15s}.
-- KilledUnit: Talk(SAY_KILL 3) only when the victim is a player
-- (C++ TYPEID_PLAYER gate -> victim:IsPlayer(), illidan convention).
-- The DamageTaken latches use the _yellCount staircase: when
-- post-damage health drops strictly below 60%
-- (HealthBelowPctDamaged(60, damage) — the thalnos/azshir damaged
-- class) and the count is < 1 -> Talk(SAY_HEALTH1 1) and increment;
-- then when post-damage health drops strictly below 30% and the
-- count is < 2 -> Talk(SAY_HEALTH2 2) and increment. Both latches
-- can fire on a single large damage packet (C++ checks them
-- sequentially), so the port uses a per-GUID integer counter, not
-- booleans; modeled on the pre-damage hook (event 9), cleared on
-- 2/4/23.
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING queue gate has
-- no UNIT_STATE bridge — timers fire unconditionally (jeklik
-- convention); the DamageTaken latch fires on the damage hook rather
-- than the script's UpdateAI tick (timing documented); JustDied's
-- Vorrel Sengutz Talk(SAY_TRIGGER_VORREL 0) via
-- instance->GetCreature(DATA_VORREL) has no instance-creature
-- bridge (documented-only); BossAI::JustEngagedWith/_Reset/_JustDied
-- and the DATA_INTERROGATOR_VISHAS encounter bookkeeping have no
-- instance-script bridge — boss admission via the luaBossAI shim.

local ENTRY_INTERROGATOR_VISHAS = 3983

local SAY_AGGRO = 0
local SAY_HEALTH1 = 1
local SAY_HEALTH2 = 2
local SAY_KILL = 3

local SPELL_SHADOW_WORD_PAIN = 2767

local timers = {}
local yellCounts = {}

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

local function onShadowWordPain(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SHADOW_WORD_PAIN)
    end
    schedule(guid, "pain", {5000, 15000}, function()
        onShadowWordPain(creature, guid)
    end)
end

local function vishasEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "pain", 5000, function()
        onShadowWordPain(creature, guid)
    end)
end

local function vishasCombatEnd(event, creature)
    cancelTimers(creature:GetGUID())
end

local function vishasTargetDied(event, creature, victim)
    if victim and victim:IsPlayer() then
        creature:Talk(SAY_KILL)
    end
end

-- C++ DamageTaken arm: the _yellCount staircase on
-- HealthBelowPctDamaged semantics (post-damage, strictly below).
local function vishasDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local count = yellCounts[guid] or 0
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    local healthPct = (creature:GetHealth() - damage) * 100 / maxHealth
    if healthPct < 60 and count < 1 then
        creature:Talk(SAY_HEALTH1)
        count = count + 1
    end
    if healthPct < 30 and count < 2 then
        creature:Talk(SAY_HEALTH2)
        count = count + 1
    end
    yellCounts[guid] = count
end

local function vishasReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    yellCounts[guid] = nil
end

RegisterCreatureEvent(ENTRY_INTERROGATOR_VISHAS, 1, vishasEnterCombat)
RegisterCreatureEvent(ENTRY_INTERROGATOR_VISHAS, 2, vishasCombatEnd)
RegisterCreatureEvent(ENTRY_INTERROGATOR_VISHAS, 3, vishasTargetDied)
RegisterCreatureEvent(ENTRY_INTERROGATOR_VISHAS, 4, vishasCombatEnd)
RegisterCreatureEvent(ENTRY_INTERROGATOR_VISHAS, 9, vishasDamageTaken)
RegisterCreatureEvent(ENTRY_INTERROGATOR_VISHAS, 23, vishasReset)
