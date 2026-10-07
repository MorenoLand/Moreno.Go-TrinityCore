-- Order of the Silver Hand (17910-17914: Gregor, Cathela, Nemas, Aelmar,
-- Vicar) — Lua port of
-- src/server/scripts/EasternKingdoms/Stratholme/boss_order_of_silver_hand.cpp
-- (one script "boss_silver_hand_bosses", ScriptedAI diff-timer AI; ScriptName
-- binds all five creature_template rows DB-side). Quest-9737 horde paladin
-- epic mount support: all five members run this AI (SD%Complete 40).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady. No Talk lines.
--
-- Modeled arms (C++-exact where bridges exist):
-- Initialize() runs in the AI ctor AND Reset() — both timers reset to 20s
-- on Reset (modeled by cancelling + re-arming on events 2/4/23, since the
-- C++ timers only tick while UpdateVictim() returns true, i.e. in combat;
-- arming on EnterCombat matches the first post-pull tick).
-- UpdateAI UpdateVictim gate: timers fire only in combat (barthilas
-- precedent).
-- HolyLight 25263: 20s->20s; on fire, self-cast ONLY when own health is
-- strictly below 20% (HealthBelowPct — current-health read). C++ re-arms
-- 20s only inside the health gate; when the gate fails the timer stays
-- elapsed and re-checks every UpdateAI tick — modeled as a 1s poll
-- (dathrohan transform granularity precedent, documented).
-- DivineShield 13874: 20s->40s; same gate shape, fires strictly below 5%.
-- Both arms are non-triggered DoCast(me) -> CastSpell(creature, spell),
-- nil-victim n/a (self-cast).
--
-- Documented-unmodeled (no bridges):
-- Reset/JustDied instance->SetData(TYPE_SH_AELMAR=25/TYPE_SH_CATHELA=21/
-- TYPE_SH_GREGOR=22/TYPE_SH_NEMAS=23/TYPE_SH_VICAR=24, 0/2) legs (no
-- instance-script bridge, shadowfang instance precedent); JustDied quest
-- credit leg: instance->GetData(TYPE_SH_QUEST=20) && killer->ToPlayer()
-- -> KilledMonsterCredit(SH_QUEST_CREDIT=17915) (no instance data bridge);
-- GetStratholmeAI factory template (no logic, dathrohan precedent);
-- UpdateAI UNIT_STATE_CASTING queue gate (timers fire unconditionally,
-- jeklik convention); the C++ header's TODO (Aurius/eternal-flame event
-- 11206) is C++ commentary, not scripted arms.

local SPELL_HOLY_LIGHT = 25263
local SPELL_DIVINE_SHIELD = 13874

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

local function healthBelowPct(creature, pct)
    local maxHealth = creature:GetMaxHealth()
    return maxHealth > 0 and creature:GetHealth() * 100 / maxHealth < pct
end

-- C++ HolyLight_Timer: cast + re-arm 20s only when HealthBelowPct(20);
-- gate failure leaves the timer elapsed and re-checks every AI tick,
-- approximated here by a 1s poll (documented granularity trade-off).
local function onHolyLight(creature, guid)
    if healthBelowPct(creature, 20) then
        creature:CastSpell(creature, SPELL_HOLY_LIGHT)
        schedule(guid, "holyLight", 20000, function()
            onHolyLight(creature, guid)
        end)
    else
        schedule(guid, "holyLight", 1000, function()
            onHolyLight(creature, guid)
        end)
    end
end

-- C++ DivineShield_Timer: cast + re-arm 40s only when HealthBelowPct(5);
-- gate failure polls like HolyLight.
local function onDivineShield(creature, guid)
    if healthBelowPct(creature, 5) then
        creature:CastSpell(creature, SPELL_DIVINE_SHIELD)
        schedule(guid, "divineShield", 40000, function()
            onDivineShield(creature, guid)
        end)
    else
        schedule(guid, "divineShield", 1000, function()
            onDivineShield(creature, guid)
        end)
    end
end

local function armTimers(creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "holyLight", 20000, function()
        onHolyLight(creature, guid)
    end)
    schedule(guid, "divineShield", 20000, function()
        onDivineShield(creature, guid)
    end)
end

-- C++ JustEngagedWith is empty; the ctor-initialized timers only tick
-- while UpdateVictim() is true, so arming on EnterCombat matches the
-- first post-pull AI tick C++-exactly.
local function onEnterCombat(event, creature, target)
    armTimers(creature)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

-- C++ Reset calls Initialize(): both timers back to 20s.
local function onReset(event, creature)
    armTimers(creature)
end

local SH_ENTRIES = { 17910, 17911, 17912, 17913, 17914 }

for _, entry in ipairs(SH_ENTRIES) do
    RegisterCreatureEvent(entry, 1, onEnterCombat)
    RegisterCreatureEvent(entry, 2, onLeaveCombat)
    RegisterCreatureEvent(entry, 4, onDied)
    RegisterCreatureEvent(entry, 23, onReset)
end
