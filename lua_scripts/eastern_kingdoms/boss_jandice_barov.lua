-- Jandice Barov (Scholomance) — Lua port of
-- src/server/scripts/EasternKingdoms/Scholomance/boss_jandice_barov.cpp
-- (boss_jandicebarovAI only; CreatureScript name "boss_jandice_barov",
-- EK loader AddSC_boss_jandicebarov decl :114 / call :292). No NPC_
-- constant in scholomance.h — RegisterLuaBoss binds the
-- creature_template ScriptName "boss_jandice_barov" DB-side; entry
-- wotlk.wowhead-verified: 10503. Eluna creature events: 1
-- OnEnterCombat, 2 OnLeaveCombat, 4 OnDied, 19 OnJustSummoned, 23 OnReset. Timers via
-- CreateLuaEvent; melee is engine-driven in Go (creature combat
-- tick), like C++ DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnEnterCombat:
-- curse of blood 24673 (non-triggered DoCastVictim, 15s->30s) /
-- illusion 17773 (non-triggered DoCast self, 30s->25s, plus a 3s
-- one-shot EVENT_SET_VISIBILITY); nil-victim ticks cast nothing but
-- keep the schedule (jeklik convention). OnDied: triggered
-- DoCastSelf drop journal 26096. Zero Talk lines in C++ (no _SAY
-- enum).
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING queue gate and
-- the post-event casting check have no live UNIT_STATE_CASTING read bridge (IsCasting is constant-false — Go casts are fire-and-forget) — timers
-- fire unconditionally (jeklik convention). Documented-only (no
-- bridges): the Summons SummonList/DespawnAll tracking only —
-- JustSummoned AttackStart on a random target PORTED 2026-10-24
-- (see handler below). PORTED 2026-10-24: JustSummoned ApplySpellImmune(0,
-- IMMUNITY_DAMAGE, MAGIC, true) via event 19 — the 'no summoned
-- bearer' claim was STALE (ev19 passes the summoned creature as a
-- full method object, lua_creature_events.go fireCreatureSummoned;
-- spell-effect summons route via the summonTempCreature funnel,
-- boss_ai.go creatureEffectSummonType; bronjahm precedent). C++
-- JustSummoned is ungated, so the handler is ungated too (Jandice
-- summons only illusions).
-- EVENT_ILLUSION's SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
-- + SetDisplayId(11686 invisible model) legs PORTED 2026-10-20
-- (flag/display bridges live — handler-surface SetFlag/RemoveFlag
-- on UNIT_FIELD_FLAGS=59, creature:SetDisplayId); the
-- ModifyThreatByPercent(victim, -99) leg PORTED 2026-10-24
-- (threat bridge live). EVENT_SET_VISIBILITY's RemoveFlag +
-- SetDisplayId(11073 Jandice model) legs PORTED 2026-10-20.
-- EVENT_CLEAVE is declared in the C++ enum but never scheduled.
-- Spell 17773's summon effect is engine-driven.

local ENTRY_JANDICE_BAROV = 10503

local SPELL_CURSE_OF_BLOOD = 24673
local SPELL_ILLUSION = 17773
local SPELL_DROP_JOURNAL = 26096

local IMMUNITY_OP_DAMAGE = 3
local SPELL_SCHOOL_MASK_MAGIC = 126

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NOT_SELECTABLE = 0x2000000

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

-- C++ EVENT_CURSE_OF_BLOOD: non-triggered DoCastVictim(24673);
-- re-arm 30s.
local function onCurseOfBlood(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CURSE_OF_BLOOD)
    end
    schedule(guid, "curse_of_blood", 30000, function()
        onCurseOfBlood(creature, guid)
    end)
end

-- C++ EVENT_SET_VISIBILITY (3s one-shot from EVENT_ILLUSION):
-- RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE) +
-- SetDisplayId(11073 Jandice model) — both bridged, PORTED
-- 2026-10-20 (handler-surface RemoveFlag(59) + SetDisplayId).
local function onSetVisibility(creature, guid)
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    creature:SetDisplayId(11073)
end

-- C++ EVENT_ILLUSION: non-triggered DoCast(17773) on self; re-arm
-- 25s; schedule EVENT_SET_VISIBILITY 3s. The flag/display legs are
-- ported below; the threat leg is PORTED below (see header).
local function onIllusion(creature, guid)
    creature:CastSpell(creature, SPELL_ILLUSION)
    -- C++ arm order: cast, SetFlag(NOT_SELECTABLE),
    -- SetDisplayId(11686 invisible), ModifyThreatByPercent(victim, -99).
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    creature:SetDisplayId(11686)
    local victim = creature:GetVictim()
    if victim then
        creature:ModifyThreatByPercent(victim, -99)
    end
    schedule(guid, "set_visibility", 3000, function()
        onSetVisibility(creature, guid)
    end)
    schedule(guid, "illusion", 25000, function()
        onIllusion(creature, guid)
    end)
end

local function jandiceBarovResetState(guid)
    cancelTimers(guid)
end

local function jandiceBarovEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    jandiceBarovResetState(guid)
    schedule(guid, "curse_of_blood", 15000, function()
        onCurseOfBlood(creature, guid)
    end)
    schedule(guid, "illusion", 30000, function()
        onIllusion(creature, guid)
    end)
end

local function jandiceBarovLeaveCombat(event, creature)
    jandiceBarovResetState(creature:GetGUID())
end

-- C++ JustDied: Summons.DespawnAll() (no SummonList bridge) +
-- triggered DoCastSelf(drop journal 26096).
local function jandiceBarovDied(event, creature, killer)
    jandiceBarovResetState(creature:GetGUID())
    creature:CastSpell(creature, SPELL_DROP_JOURNAL, true)
end

local function jandiceBarovReset(event, creature)
    jandiceBarovResetState(creature:GetGUID())
end

-- C++ JustSummoned (boss_jandice_barov.cpp:52-58): illusion
-- AttackStart on a random target + summoned->ApplySpellImmune(0,
-- IMMUNITY_DAMAGE, MAGIC, true), in C++ order. SelectTarget(Random, 0)
-- is the threat-list random pick via GetAITargets (gerstahn/kagani
-- precedent; SelectTargetList skips offline refs, UnitAI.h, so the
-- pick is alive-filtered); AttackStart via EngageWithTarget (player-target
-- funnel; a non-player random pick no-ops in the bridge).
-- C++ JustSummoned carries no entry gate, so neither does this
-- handler: Jandice summons only illusions (spell 17773).
local function jandiceJustSummoned(event, creature, summon)
    local alive = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if not t:IsDead() then
            alive[#alive + 1] = t
        end
    end
    if #alive > 0 then
        summon:EngageWithTarget(alive[math.random(#alive)])
    end
    summon:ApplySpellImmune(0, IMMUNITY_OP_DAMAGE, SPELL_SCHOOL_MASK_MAGIC, true)
end

RegisterCreatureEvent(ENTRY_JANDICE_BAROV, 1, jandiceBarovEnterCombat)
RegisterCreatureEvent(ENTRY_JANDICE_BAROV, 2, jandiceBarovLeaveCombat)
RegisterCreatureEvent(ENTRY_JANDICE_BAROV, 4, jandiceBarovDied)
RegisterCreatureEvent(ENTRY_JANDICE_BAROV, 23, jandiceBarovReset)
RegisterCreatureEvent(ENTRY_JANDICE_BAROV, 19, jandiceJustSummoned)
