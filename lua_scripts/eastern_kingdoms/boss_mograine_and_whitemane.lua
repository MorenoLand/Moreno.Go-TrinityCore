-- Scarlet Commander Mograine (3976) + High Inquisitor Whitemane (3977),
-- Scarlet Monastery Cathedral finale — Lua port of
-- src/server/scripts/EasternKingdoms/ScarletMonastery/
-- boss_mograine_and_whitemane.cpp (both boss AIs); entries C++-named in
-- scarlet_monastery.h (NPC_MOGRAINE=3976 :66, NPC_WHITEMANE=3977 :67;
-- the file's own comments name "Scarlet Commander Mograine - 3976" and
-- "High Inquisitor Whitemane - 3977" — RegisterScarletMonasteryCreatureAI
-- binds the creature_template ScriptName DB-side).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
--
-- Modeled arms (C++-exact where bridges exist):
-- Mograine: OnEnterCombat Talk(SAY_MO_AGGRO 0) + crusader strike 14518
-- at {10s,15s}->10s + hammer of justice 5589 at {10s,15s}->60s (both
-- non-triggered DoCastVictim -> GetVictim + CastSpell, nil-victim keeps
-- schedule, jeklik convention; urand(10s,15s) initial -> math.random,
-- sulfuron convention). KilledUnit: Talk(SAY_MO_KILL 1) only when the
-- victim is a player (TYPEID_PLAYER gate -> victim:IsPlayer(), herod
-- convention), gated by the 5s _killYellTimer (Reset(0s) in Reset means
-- available immediately — modeled with a per-GUID one-shot clear,
-- fairbanks heal-cooldown pattern; C++ _Reset() does NOT touch the
-- timer, so the flag is cleared only on event 23). OnReset cancels
-- timers + clears the yell flag + self-casts retribution aura 8990
-- (C++ DoCastSelf(..., true) triggered — the Lua CastSpell has no
-- triggered leg, documented).
-- Whitemane: OnEnterCombat Talk(SAY_WH_INTRO 0) + 5s scheduler gate ->
-- heal 12039 at 10s->13s + power word: shield 22187 at 15s->15s + holy
-- smite 9481 at 6s->6s. Heal: self-cast when own current health is
-- strictly below 75% (HealthBelowPct(75) current-health read); the
-- C++ Mograine-target leg (alive && HealthBelowPct(75)) needs the
-- instance GetCreature(DATA_MOGRAINE) bridge — documented, nil target
-- casts nothing, 13s re-arm unconditional. Shield/smite non-triggered
-- self/victim casts (smite nil-victim keeps schedule). KilledUnit:
-- player-gated Talk(SAY_WH_KILL 1), same 5s cooldown shape. OnReset
-- cancels timers + clears the yell flag + self-cast retribution aura
-- 8990 (C++ non-triggered).
--
-- Documented-unmodeled (no bridges):
-- Mograine's fake-death DamageTaken machine: first lethal hit sets
-- _fakeDeath/_canDie=false, opens DATA_HIGH_INQUISITORS_DOOR (no GO/
-- instance bridge), orders whitemane via instance->GetCreature(
-- DATA_WHITEMANE) + MovePoint(WhitemaneIntroMovePos) + DoZoneInCombat
-- (no instance creature-lookup, MotionMaster, or zone-combat bridges),
-- InterruptNonMeleeSpells + UNIT_FLAG_NOT_SELECTABLE/NON_ATTACKABLE +
-- SetStandState(DEAD) + REACT_PASSIVE + MotionMaster Clear (no flag/
-- stand-state/react bridges, npc_barnes precedent), and negates the
-- killing damage while !_canDie. Without the negation the boss dies on
-- the first lethal hit (documented divergence — the headless-horseman
-- cheat-death precedent: porting negation alone would strand the boss
-- unkillable, since the SpellHit(SCARLET_RESURRECTION 9232) 3s/5s
-- scheduler legs that restore stand state, reschedule crusader/hammer,
-- remove NON_ATTACKABLE, set REACT_AGGRESSIVE and _canDie=true have no
-- SpellHit bridge). JustEngagedWith's CallForHelp(VISIBLE_RANGE) +
-- MoveIdle and BossAI::JustEngagedWith + DATA_MOGRAINE_AND_WHITE_EVENT
-- bookkeeping have no bridges (luaBossAI shim).
-- Whitemane's sub-50% DamageTaken latch (HealthBelowPctDamaged(50,
-- damage) — damaged class): cancel combat events, InterruptNonMeleeSpells,
-- DoCastAOE(deep sleep 9256) (no AoE-cast bridge), REACT_PASSIVE,
-- MotionMaster Clear, MovePosition(2yd, facing) + MovePoint(
-- POINT_WHITEMANE_MOVE_TO_MOGRAINE 1) toward Mograine via
-- instance->GetCreature(DATA_MOGRAINE) (no instance lookup or
-- MotionMaster/MovePosition bridges), MovementInform -> 3s scheduler ->
-- DoCast(mograine, SCARLET_RESURRECTION 9232) (no MovementInform/
-- scheduler-cross-creature bridge), SpellHitTarget -> MograineResurrected:
-- Talk(SAY_WH_RESURRECT 2), reschedule heal/shield/smite, _canDie=true,
-- REACT_AGGRESSIVE, MoveChase victim (no SpellHitTarget bridge). Damage
-- negation while !_canDie unmodeled — the Lua leaves her killable below
-- 50% (documented divergence). Dominate mind 14515 is declared in the
-- C++ spell enum but never cast by either AI.
-- The UpdateAI UNIT_STATE_CASTING queue + post-event gates have no
-- UNIT_STATE bridge (timers fire unconditionally, jeklik convention).

local ENTRY_MOGRAINE = 3976
local ENTRY_WHITEMANE = 3977

local SAY_MO_AGGRO = 0
local SAY_MO_KILL = 1
-- SAY_MO_RESURRECTED (2) belongs to the unmodeled SpellHit leg.

local SAY_WH_INTRO = 0
local SAY_WH_KILL = 1
-- SAY_WH_RESURRECT (2) belongs to the unmodeled resurrection leg.

local SPELL_CRUSADER_STRIKE = 14518
local SPELL_HAMMER_OF_JUSTICE = 5589
local SPELL_RETRIBUTION_AURA = 8990

local SPELL_HEAL = 12039
local SPELL_POWER_WORD_SHIELD = 22187
local SPELL_HOLY_SMITE = 9481

local timers = {}
local killYellOnCooldown = {}

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

local function castOnVictim(creature, spellId)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, spellId)
    end
end

local function healthBelowPct(creature, pct)
    local maxHealth = creature:GetMaxHealth()
    return maxHealth > 0 and creature:GetHealth() * 100 / maxHealth < pct
end

-- C++ EVENT_CRUSADER_STRIKE: DoCastVictim(14518); Repeat(10s).
local function onCrusaderStrike(creature, guid)
    castOnVictim(creature, SPELL_CRUSADER_STRIKE)
    schedule(guid, "crusaderStrike", 10000, function()
        onCrusaderStrike(creature, guid)
    end)
end

-- C++ EVENT_HAMMER_OF_JUSTICE: DoCastVictim(5589); Repeat(60s).
local function onHammerOfJustice(creature, guid)
    castOnVictim(creature, SPELL_HAMMER_OF_JUSTICE)
    schedule(guid, "hammerOfJustice", 60000, function()
        onHammerOfJustice(creature, guid)
    end)
end

-- C++ KilledUnit (both bosses): player-gated yell, 5s _killYellTimer.
local function onTargetDied(event, creature, victim, sayKill)
    if not victim:IsPlayer() then
        return
    end
    local guid = creature:GetGUID()
    if killYellOnCooldown[guid] then
        return
    end
    killYellOnCooldown[guid] = true
    creature:Talk(sayKill)
    CreateLuaEvent(function()
        killYellOnCooldown[guid] = nil
    end, 5000)
end

-- Mograine ---------------------------------------------------------------

-- C++ JustEngagedWith: Talk(SAY_MO_AGGRO) + EVENT_CRUSADER_STRIKE /
-- EVENT_HAMMER_OF_JUSTICE at urand(10s, 15s). CallForHelp + MoveIdle +
-- BossAI::JustEngagedWith bookkeeping documented-only (no bridges).
local function mograineEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_MO_AGGRO)
    schedule(guid, "crusaderStrike", math.random(10000, 15000), function()
        onCrusaderStrike(creature, guid)
    end)
    schedule(guid, "hammerOfJustice", math.random(10000, 15000), function()
        onHammerOfJustice(creature, guid)
    end)
end

local function mograineTargetDied(event, creature, victim)
    onTargetDied(event, creature, victim, SAY_MO_KILL)
end

local function mograineLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function mograineDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

-- C++ Reset: Initialize + _Reset + _killYellTimer.Reset(0s) +
-- DoCastSelf(retribution aura, triggered) + flag/standstate/react legs
-- (bridgeless legs documented-only).
local function mograineReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    killYellOnCooldown[guid] = nil
    creature:CastSpell(creature, SPELL_RETRIBUTION_AURA)
end

RegisterCreatureEvent(ENTRY_MOGRAINE, 1, mograineEnterCombat)
RegisterCreatureEvent(ENTRY_MOGRAINE, 2, mograineLeaveCombat)
RegisterCreatureEvent(ENTRY_MOGRAINE, 3, mograineTargetDied)
RegisterCreatureEvent(ENTRY_MOGRAINE, 4, mograineDied)
RegisterCreatureEvent(ENTRY_MOGRAINE, 23, mograineReset)

-- Whitemane --------------------------------------------------------------

-- C++ EVENT_HEAL: self if HealthBelowPct(75), else Mograine if alive
-- and below 75% (instance leg unmodeled — no bridge); nil target casts
-- nothing. Repeat(13s) regardless.
local function onHeal(creature, guid)
    if healthBelowPct(creature, 75) then
        creature:CastSpell(creature, SPELL_HEAL)
    end
    schedule(guid, "heal", 13000, function()
        onHeal(creature, guid)
    end)
end

-- C++ EVENT_POWER_WORD_SHIELD: DoCastSelf(22187); Repeat(15s).
local function onPowerWordShield(creature, guid)
    creature:CastSpell(creature, SPELL_POWER_WORD_SHIELD)
    schedule(guid, "powerWordShield", 15000, function()
        onPowerWordShield(creature, guid)
    end)
end

-- C++ EVENT_HOLY_SMITE: DoCastVictim(9481); Repeat(6s).
local function onHolySmite(creature, guid)
    castOnVictim(creature, SPELL_HOLY_SMITE)
    schedule(guid, "holySmite", 6000, function()
        onHolySmite(creature, guid)
    end)
end

-- C++ JustEngagedWith: Talk(SAY_WH_INTRO) + MoveIdle + a 5s scheduler
-- gate, then EVENT_HEAL at 10s, EVENT_POWER_WORD_SHIELD at 15s,
-- EVENT_HOLY_SMITE at 6s.
local function whitemaneEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_WH_INTRO)
    schedule(guid, "startup", 5000, function()
        schedule(guid, "heal", 10000, function()
            onHeal(creature, guid)
        end)
        schedule(guid, "powerWordShield", 15000, function()
            onPowerWordShield(creature, guid)
        end)
        schedule(guid, "holySmite", 6000, function()
            onHolySmite(creature, guid)
        end)
    end)
end

local function whitemaneTargetDied(event, creature, victim)
    onTargetDied(event, creature, victim, SAY_WH_KILL)
end

local function whitemaneLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function whitemaneDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

-- C++ Reset: _events.Reset + _scheduler.CancelAll +
-- _killYellTimer.Reset(0s) + DoCastSelf(retribution aura) +
-- REACT_AGGRESSIVE (bridgeless leg documented-only).
local function whitemaneReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    killYellOnCooldown[guid] = nil
    creature:CastSpell(creature, SPELL_RETRIBUTION_AURA)
end

RegisterCreatureEvent(ENTRY_WHITEMANE, 1, whitemaneEnterCombat)
RegisterCreatureEvent(ENTRY_WHITEMANE, 2, whitemaneLeaveCombat)
RegisterCreatureEvent(ENTRY_WHITEMANE, 3, whitemaneTargetDied)
RegisterCreatureEvent(ENTRY_WHITEMANE, 4, whitemaneDied)
RegisterCreatureEvent(ENTRY_WHITEMANE, 23, whitemaneReset)
