-- Scarlet Commander Mograine (3976) + High Inquisitor Whitemane (3977),
-- Scarlet Monastery Cathedral finale — Lua port of
-- src/server/scripts/EasternKingdoms/ScarletMonastery/
-- boss_mograine_and_whitemane.cpp (both boss AIs); entries C++-named in
-- scarlet_monastery.h (NPC_MOGRAINE=3976 :66, NPC_WHITEMANE=3977 :67;
-- the file's own comments name "Scarlet Commander Mograine - 3976" and
-- "High Inquisitor Whitemane - 3977" — RegisterScarletMonasteryCreatureAI
-- binds the creature_template ScriptName DB-side).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 9 OnDamageTaken, 14 OnHitBySpell, 15 OnHitBySpellTarget,
-- 23 OnReset, 28 OnMovementInform. Timers via CreateLuaEvent; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
--
-- Modeled arms (C++-exact where bridges exist):
-- Mograine: OnEnterCombat Talk(SAY_MO_AGGRO 0) + crusader strike 14518
-- at {10s,15s}->10s + hammer of justice 5589 at {10s,15s}->60s (both
-- non-triggered DoCastVictim -> GetVictim + CastSpell, nil-victim keeps
-- schedule, jeklik convention; urand(10s,15s) initial -> math.random,
-- sulfuron convention). KilledUnit: Talk(SAY_MO_KILL 1) only when the
-- victim is a player (TYPEID_PLAYER gate -> victim:GetObjectType() == "Player", herod
-- convention), but NOT a 5s cooldown — Mograine's UpdateAI never calls
-- _killYellTimer.Update (Timer.h TimeTracker advances only on Update),
-- so the yell fires once per combat until Reset clears the latch
-- (Whitemane's UpdateAI does update its timer, so her 5s cooldown is
-- real). OnReset cancels
-- timers + clears the yell flag + self-casts retribution aura 8990
-- (C++ DoCastSelf(..., true) triggered — Lua CastSpell(creature, spell, true)).
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
-- 8990 (C++ non-triggered) + SetReactState(REACT_AGGRESSIVE).
--
-- Documented-unmodeled (no bridges):
-- Resurrection machine PORTED 2026-11-13 (hook round 29, rows 26-27;
-- the "no bridge" claims below it are retracted — re-verified against
-- the current runtime): Mograine's fake-death DamageTaken (first lethal
-- hit: door open via FindNearestGameObject, Whitemane ordered to
-- WhitemaneIntroMovePos via FindNearestCreature + MoveTo + DoZoneInCombat,
-- InterruptNonMeleeSpells + RemoveAllAuras + flag NOT_SELECTABLE |
-- NON_ATTACKABLE + stand DEAD + REACT_PASSIVE + StopMoving, damage
-- negated via return false, 0; later lethal hits negated while !_canDie,
-- and a lethal hit after the resurrection kills him, C++-exact) +
-- Mograine SpellHit(9232) 3s/5s scheduler legs (Talk SAY_MO_RESURRECTED,
-- stand, re-arm crusader/hammer, REACT_AGGRESSIVE, _canDie=true,
-- retribution aura) + Whitemane's sub-50% DamageTaken latch
-- (HealthBelowPctDamaged(50), cancel heal/shield/smite, deep sleep 9256
-- self-cast, REACT_PASSIVE, 2yd MovePosition twin via atan2 + MoveTo(1))
-- + MovementInform(28) arrival -> face Mograine -> 3s -> cast Scarlet
-- Resurrection (9232) on him + SpellHitTarget(15) -> MograineResurrected
-- (Talk SAY_WH_RESURRECT, reschedule heal/shield/smite, _canDie=true,
-- REACT_AGGRESSIVE). ClearComboPointHolders / ClearAllReactives have no
-- bridge (skipped, no observable Lua effect); the MoveChase-victim leg
-- of MograineResurrected rides the engine combat tick. JustEngagedWith's
-- CallForHelp(VISIBLE_RANGE) + MoveIdle are PORTED in
-- mograineEnterCombat (bridged lua_creature_events.go:1704/:2794),
-- as are Mograine's BossAI::JustEngagedWith pulse/zone arms
-- (Whitemane's override never calls the base); DATA_MOGRAINE_AND_WHITE_EVENT
-- bookkeeping still has no bridge (luaBossAI shim).
-- Dominate mind 14515 is declared in the C++ spell enum but never cast
-- by either AI.
-- The UpdateAI UNIT_STATE_CASTING queue + post-event gates are PORTED
-- (each timer arm defers 500ms while creature:IsCasting(), live
-- CastingUntil window, lua_creature_events.go:1950, scorn/thalnos
-- precedent; no re-arm consumed; the stale "no UNIT_STATE bridge"
-- claim is retracted). Mograine's own UpdateAI
-- (boss_mograine_and_whitemane.cpp:214-232) and Whitemane's own
-- UpdateAI (:296-347) share the pre-loop + in-loop CASTING returns;
-- KilledUnit/DamageTaken/SpellHit scheduler legs stay ungated per C++.

local ENTRY_MOGRAINE = 3976
local ENTRY_WHITEMANE = 3977
local GO_HIGH_INQUISITORS_DOOR = 104600 -- scarlet_monastery.h:89

local SAY_MO_AGGRO = 0
local SAY_MO_KILL = 1
local SAY_MO_RESURRECTED = 2

local SAY_WH_INTRO = 0
local SAY_WH_KILL = 1
local SAY_WH_RESURRECT = 2

local SPELL_CRUSADER_STRIKE = 14518
local SPELL_HAMMER_OF_JUSTICE = 5589
local SPELL_RETRIBUTION_AURA = 8990
local SPELL_DEEP_SLEEP = 9256
local SPELL_SCARLET_RESURRECTION = 9232

local POINT_MOTION_TYPE = 8 -- MovementDefines.h:31
local POINT_WHITEMANE_MOVE_TO_MOGRAINE = 1
local UNIT_STAND_STATE_DEAD = 7
local REACT_PASSIVE = 0
local WHITEMANE_INTRO_POS = { 1163.113370, 1398.856812, 32.527786 } -- boss_mograine_and_whitemane.cpp:79

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NON_ATTACKABLE = 0x2
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000

local SPELL_HEAL = 12039
local SPELL_POWER_WORD_SHIELD = 22187
local SPELL_HOLY_SMITE = 9481

local timers = {}
local killYellOnCooldown = {}
local mograineKillYellDone = {}
local mograineDoorOpened = {}
local mograineFakeDead = {}
local mograineCanDie = {}
local whitemaneResurrection = {}
local whitemaneCanDie = {}

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

local function cancelTimerKeys(guid, keys)
    local per = timers[guid]
    if per then
        for _, key in ipairs(keys) do
            if per[key] then
                RemoveEventById(per[key])
                per[key] = nil
            end
        end
    end
end

local function castOnVictim(creature, spellId)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, spellId)
    end
end

local function healthBelowPct(creature, pct)
    local maxHealth = creature:GetMaxHealth()
    return maxHealth > 0 and creature:GetHealth() < math.floor(maxHealth * pct / 100)
end

-- C++ EVENT_CRUSADER_STRIKE: DoCastVictim(14518); Repeat(10s).
-- UpdateAI CASTING gate PORTED as above (defer before victim pick).
local function onCrusaderStrike(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "crusaderStrike", 500, function()
            onCrusaderStrike(creature, guid)
        end)
        return
    end
    castOnVictim(creature, SPELL_CRUSADER_STRIKE)
    schedule(guid, "crusaderStrike", 10000, function()
        onCrusaderStrike(creature, guid)
    end)
end

-- C++ EVENT_HAMMER_OF_JUSTICE: DoCastVictim(5589); Repeat(60s).
-- UpdateAI CASTING gate PORTED as above (defer before victim pick).
local function onHammerOfJustice(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "hammerOfJustice", 500, function()
            onHammerOfJustice(creature, guid)
        end)
        return
    end
    castOnVictim(creature, SPELL_HAMMER_OF_JUSTICE)
    schedule(guid, "hammerOfJustice", 60000, function()
        onHammerOfJustice(creature, guid)
    end)
end

-- C++ KilledUnit (both bosses): player-gated yell, 5s _killYellTimer.
local function onTargetDied(event, creature, victim, sayKill)
    if victim:GetObjectType() ~= "Player" then
        return
    end
    local guid = creature:GetGUIDLow()
    if killYellOnCooldown[guid] then
        return
    end
    killYellOnCooldown[guid] = true
    creature:Talk(sayKill)
    schedule(guid, "killYell", 5000, function()
        killYellOnCooldown[guid] = nil
    end)
end

-- Mograine ---------------------------------------------------------------

-- C++ JustEngagedWith: Talk(SAY_MO_AGGRO) + EVENT_CRUSADER_STRIKE /
-- EVENT_HAMMER_OF_JUSTICE at urand(10s, 15s). CallForHelp (bridged
-- lua_creature_events.go:1704) + MoveIdle (bridged :2794, StopMoving
-- alias) ported below; BossAI::JustEngagedWith bookkeeping documented-only
-- (no bridge).
local function mograineEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    creature:SetCombatPulseDelay(5) -- BossAI::JustEngagedWith arm 1 (ScriptedCreature.cpp:540; boss_mograine_and_whitemane.cpp:108 base call first)
    creature:DoZoneInCombat() -- BossAI::JustEngagedWith me-form arm (ScriptedCreature.cpp:542; boss_mograine_and_whitemane.cpp:108 base call first)
    creature:Talk(SAY_MO_AGGRO)
    creature:CallForHelp(166.0) -- VISIBLE_RANGE (ScriptMgr.h:92)
    creature:MoveIdle()
    schedule(guid, "crusaderStrike", math.random(10000, 15000), function()
        onCrusaderStrike(creature, guid)
    end)
    schedule(guid, "hammerOfJustice", math.random(10000, 15000), function()
        onHammerOfJustice(creature, guid)
    end)
end

-- C++ Mograine KilledUnit: player-gated Talk(SAY_MO_KILL 1), but the
-- 5s _killYellTimer NEVER advances — Mograine's UpdateAI
-- (boss_mograine_and_whitemane.cpp:214-232) never calls
-- _killYellTimer.Update, and TimeTracker only advances on Update
-- (Timer.h:115-134), so after the first kill yell Passed() stays
-- false until Reset. Once per combat, modeled as a Reset-only latch
-- (Whitemane's UpdateAI does update its timer, so her 5s cooldown
-- via onTargetDied is real).
local function mograineTargetDied(event, creature, victim)
    if victim:GetObjectType() ~= "Player" then
        return
    end
    local guid = creature:GetGUIDLow()
    if mograineKillYellDone[guid] then
        return
    end
    mograineKillYellDone[guid] = true
    creature:Talk(SAY_MO_KILL)
end

local function mograineLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

local function mograineDied(event, creature, killer)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ Reset: Initialize + _Reset + _killYellTimer.Reset(0s) +
-- DoCastSelf(retribution aura, triggered) + flag/standstate/react legs
-- (PORTED below — all three surfaces bridged; the fake-death latches
-- are cleared here, so a post-wipe Mograine can fake-die again).
local function mograineReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    mograineKillYellDone[guid] = nil
    mograineDoorOpened[guid] = nil
    mograineFakeDead[guid] = nil
    mograineCanDie[guid] = nil
    -- C++ Reset: triggered self-cast (Reset-only arm); ev23 fires on death
    -- too (engine reset-then-died order) but C++ Reset() never runs on
    -- death, so the cast is alive-gated.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_RETRIBUTION_AURA, true)
        -- C++ Reset (:99-101): RemoveFlag(NOT_SELECTABLE | NON_ATTACKABLE)
        -- + SetStandState(STAND) + SetReactState(AGGRESSIVE) — clears the
        -- fake-death state on evade (the set side stays out, above).
        creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE + UNIT_FLAG_NOT_SELECTABLE)
        creature:SetStandState(0) -- UNIT_STAND_STATE_STAND
        creature:SetReactState(2) -- REACT_AGGRESSIVE
        -- C++ Reset (:103): instance->HandleGameObject(false, door) ->
        -- GO_STATE_READY (InstanceScript.cpp:479: open=false -> READY).
        local door = creature:FindNearestGameObject(GO_HIGH_INQUISITORS_DOOR, 100.0)
        if door then door:SetGoState(1) end -- GO_STATE_READY (close)
    end
end

-- C++ DamageTaken (boss_mograine_and_whitemane.cpp:152-183): first
-- lethal hit fake-dies (door open, order Whitemane to the intro pos +
-- zone her in, interrupt, RemoveAllAuras, flag NOT_SELECTABLE |
-- NON_ATTACKABLE, stand DEAD, REACT_PASSIVE, motion clear) and negates
-- the damage; while !_canDie later lethal hits are also negated. After
-- the resurrection _canDie is true again, so a further lethal hit kills
-- him for real (C++ _fakeDeath stays latched, so no second fake-death).
-- ClearComboPointHolders / ClearAllReactives have no bridge and are
-- skipped (no observable Lua effect). Damage rewrite is the reliquary
-- convention (return false, newDamage); combat timers keep firing
-- while fake-dead, as in C++ (DamageTaken cancels no events).
local function mograineDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUIDLow()
    local health = creature:GetHealth()
    if damage < health then
        return
    end
    if not mograineFakeDead[guid] then
        mograineFakeDead[guid] = true
        mograineCanDie[guid] = false
        if not mograineDoorOpened[guid] then
            mograineDoorOpened[guid] = true
            local door = creature:FindNearestGameObject(GO_HIGH_INQUISITORS_DOOR, 100.0)
            if door then door:SetGoState(0) end -- GO_STATE_ACTIVE (open)
        end
        local whitemane = creature:FindNearestCreature(ENTRY_WHITEMANE, 100.0)
        if whitemane then
            whitemane:MoveTo(0, WHITEMANE_INTRO_POS[1], WHITEMANE_INTRO_POS[2], WHITEMANE_INTRO_POS[3])
            whitemane:DoZoneInCombat()
        end
        creature:InterruptNonMeleeSpells(true)
        creature:RemoveAllAuras()
        creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE + UNIT_FLAG_NON_ATTACKABLE)
        creature:SetStandState(UNIT_STAND_STATE_DEAD)
        creature:SetReactState(REACT_PASSIVE)
        creature:StopMoving()
        return false, 0
    end
    if not mograineCanDie[guid] then
        return false, 0
    end
end

-- C++ SpellHit (boss_mograine_and_whitemane.cpp:185-212): hit by
-- Scarlet Resurrection (9232, cast by Whitemane) -> 3s scheduler arm
-- (Talk SAY_MO_RESURRECTED, clear NOT_SELECTABLE, stand) then 5s arm
-- (re-arm crusader strike / hammer of justice at urand(10s, 15s),
-- clear NON_ATTACKABLE, REACT_AGGRESSIVE, _canDie=true, triggered
-- retribution-aura self-cast), C++ arm order in each leg.
local function mograineSpellHit(event, creature, caster, spellId)
    if spellId ~= SPELL_SCARLET_RESURRECTION then
        return
    end
    local guid = creature:GetGUIDLow()
    schedule(guid, "resurrectTalk", 3000, function()
        creature:Talk(SAY_MO_RESURRECTED)
        creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
        creature:SetStandState(0) -- UNIT_STAND_STATE_STAND
    end)
    schedule(guid, "resurrectRearm", 5000, function()
        schedule(guid, "crusaderStrike", math.random(10000, 15000), function()
            onCrusaderStrike(creature, guid)
        end)
        schedule(guid, "hammerOfJustice", math.random(10000, 15000), function()
            onHammerOfJustice(creature, guid)
        end)
        creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
        creature:SetReactState(2) -- REACT_AGGRESSIVE
        mograineCanDie[guid] = true
        creature:CastSpell(creature, SPELL_RETRIBUTION_AURA, true)
    end)
end

RegisterCreatureEvent(ENTRY_MOGRAINE, 1, mograineEnterCombat)
RegisterCreatureEvent(ENTRY_MOGRAINE, 2, mograineLeaveCombat)
RegisterCreatureEvent(ENTRY_MOGRAINE, 3, mograineTargetDied)
RegisterCreatureEvent(ENTRY_MOGRAINE, 4, mograineDied)
RegisterCreatureEvent(ENTRY_MOGRAINE, 9, mograineDamageTaken)
RegisterCreatureEvent(ENTRY_MOGRAINE, 14, mograineSpellHit)
RegisterCreatureEvent(ENTRY_MOGRAINE, 23, mograineReset)

-- Whitemane --------------------------------------------------------------

-- C++ EVENT_HEAL: self if HealthBelowPct(75), else Mograine if alive
-- and below 75% (instance leg unmodeled — no bridge); nil target casts
-- nothing. Repeat(13s) regardless.
-- UpdateAI CASTING gate PORTED as above (defer before the 75% read).
local function onHeal(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "heal", 500, function()
            onHeal(creature, guid)
        end)
        return
    end
    if healthBelowPct(creature, 75) then
        creature:CastSpell(creature, SPELL_HEAL)
    end
    schedule(guid, "heal", 13000, function()
        onHeal(creature, guid)
    end)
end

-- C++ EVENT_POWER_WORD_SHIELD: DoCastSelf(22187); Repeat(15s).
-- UpdateAI CASTING gate PORTED as above (defer before self-cast).
local function onPowerWordShield(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "powerWordShield", 500, function()
            onPowerWordShield(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_POWER_WORD_SHIELD)
    schedule(guid, "powerWordShield", 15000, function()
        onPowerWordShield(creature, guid)
    end)
end

-- C++ EVENT_HOLY_SMITE: DoCastVictim(9481); Repeat(6s).
-- UpdateAI CASTING gate PORTED as above (defer before victim pick).
local function onHolySmite(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "holySmite", 500, function()
            onHolySmite(creature, guid)
        end)
        return
    end
    castOnVictim(creature, SPELL_HOLY_SMITE)
    schedule(guid, "holySmite", 6000, function()
        onHolySmite(creature, guid)
    end)
end

-- C++ JustEngagedWith: Talk(SAY_WH_INTRO) + MoveIdle + a 5s scheduler
-- gate, then EVENT_HEAL at 10s, EVENT_POWER_WORD_SHIELD at 15s,
-- EVENT_HOLY_SMITE at 6s.
local function whitemaneEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
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
    cancelTimers(creature:GetGUIDLow())
end

local function whitemaneDied(event, creature, killer)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ Reset: _events.Reset + _scheduler.CancelAll +
-- _killYellTimer.Reset(0s) + DoCastSelf(retribution aura) +
-- SetReactState(REACT_AGGRESSIVE) — bridged (same surface as
-- mograineReset above).
local function whitemaneReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    killYellOnCooldown[guid] = nil
    whitemaneResurrection[guid] = nil
    whitemaneCanDie[guid] = nil
    -- C++ Reset: non-triggered self-cast (Reset-only arm); ev23 fires on
    -- death too (engine reset-then-died order) but C++ Reset() never runs
    -- on death, so the cast is alive-gated.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_RETRIBUTION_AURA)
        creature:SetReactState(2) -- REACT_AGGRESSIVE, C++ Reset leg
    end
end

-- C++ Whitemane DamageTaken (boss_mograine_and_whitemane.cpp:351-379,
-- HealthBelowPctDamaged class — post-damage projection, yazzai/nefarian
-- convention): dropping below 50% after the hit, once per combat,
-- cancels heal/shield/smite, interrupts, DoCastAOE deep sleep (self-cast
-- lowering, selin_fireheart convention), goes REACT_PASSIVE and walks to
-- 2yd from Mograine (MovePosition twin via atan2 + MoveTo; C++ facing
-- is applied on MovementInform arrival). Lethal hits while !_canDie are
-- negated (return false, 0, reliquary convention); a lethal hit above
-- 50% with _canDie true kills her, as in C++.
local function whitemaneDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUIDLow()
    local health = creature:GetHealth()
    local maxHealth = creature:GetMaxHealth()
    if not whitemaneResurrection[guid] and maxHealth > 0
            and math.max(0, health - damage) < math.floor(maxHealth * 50 / 100) then
        whitemaneResurrection[guid] = true
        whitemaneCanDie[guid] = false
        cancelTimerKeys(guid, { "heal", "powerWordShield", "holySmite" })
        creature:InterruptNonMeleeSpells(true)
        creature:CastSpell(creature, SPELL_DEEP_SLEEP)
        local mograine = creature:FindNearestCreature(ENTRY_MOGRAINE, 100.0)
        if mograine then
            creature:SetReactState(REACT_PASSIVE)
            creature:StopMoving()
            local dx = mograine:GetX() - creature:GetX()
            local dy = mograine:GetY() - creature:GetY()
            local angle = math.atan2(dy, dx)
            local tx = mograine:GetX() - 2.0 * math.cos(angle)
            local ty = mograine:GetY() - 2.0 * math.sin(angle)
            creature:MoveTo(POINT_WHITEMANE_MOVE_TO_MOGRAINE, tx, ty, mograine:GetZ())
        end
    end
    if damage >= health and whitemaneCanDie[guid] == false then
        return false, 0
    end
end

-- C++ boss_high_inquisitor_whitemane::MograineResurrected (:411-423):
-- Talk SAY_WH_RESURRECT, reschedule heal (10s) / power word shield
-- (15s) / holy smite (6s), _canDie=true, REACT_AGGRESSIVE. The
-- _ressurectionInProgress latch is NOT cleared in C++, so no second
-- resurrection — the Lua latch stays set too.
local function mograineResurrected(creature, guid)
    creature:Talk(SAY_WH_RESURRECT)
    schedule(guid, "heal", 10000, function()
        onHeal(creature, guid)
    end)
    schedule(guid, "powerWordShield", 15000, function()
        onPowerWordShield(creature, guid)
    end)
    schedule(guid, "holySmite", 6000, function()
        onHolySmite(creature, guid)
    end)
    whitemaneCanDie[guid] = true
    creature:SetReactState(2) -- REACT_AGGRESSIVE
end

-- C++ Whitemane MovementInform (boss_mograine_and_whitemane.cpp:383-402):
-- on POINT_MOTION_TYPE arrival at POINT_WHITEMANE_MOVE_TO_MOGRAINE,
-- face Mograine (SetFacingToObject twin via atan2, nightbane precedent),
-- then 3s later cast Scarlet Resurrection on him; if he is gone, call
-- MograineResurrected directly. The id-0 intro-pos arrival from
-- Mograine's fake-death is ignored, as in C++.
local function whitemaneMovementInform(event, creature, type, id)
    if type ~= POINT_MOTION_TYPE or id ~= POINT_WHITEMANE_MOVE_TO_MOGRAINE then
        return
    end
    local guid = creature:GetGUIDLow()
    local mograine = creature:FindNearestCreature(ENTRY_MOGRAINE, 100.0)
    if mograine then
        local dx = mograine:GetX() - creature:GetX()
        local dy = mograine:GetY() - creature:GetY()
        local face = math.atan2(dy, dx)
        if face < 0 then face = face + 2 * math.pi end
        creature:SetFacingTo(face)
    end
    schedule(guid, "resurrectCast", 3000, function()
        local target = creature:FindNearestCreature(ENTRY_MOGRAINE, 100.0)
        if target then
            creature:CastSpell(target, SPELL_SCARLET_RESURRECTION)
        else
            mograineResurrected(creature, guid)
        end
    end)
end

-- C++ Whitemane SpellHitTarget (boss_mograine_and_whitemane.cpp:404-408):
-- Scarlet Resurrection landing on Mograine -> MograineResurrected.
local function whitemaneSpellHitTarget(event, creature, target, spellId)
    if target == nil or spellId ~= SPELL_SCARLET_RESURRECTION then
        return
    end
    if target:GetEntry() == ENTRY_MOGRAINE then
        mograineResurrected(creature, creature:GetGUIDLow())
    end
end

RegisterCreatureEvent(ENTRY_WHITEMANE, 1, whitemaneEnterCombat)
RegisterCreatureEvent(ENTRY_WHITEMANE, 2, whitemaneLeaveCombat)
RegisterCreatureEvent(ENTRY_WHITEMANE, 3, whitemaneTargetDied)
RegisterCreatureEvent(ENTRY_WHITEMANE, 4, whitemaneDied)
RegisterCreatureEvent(ENTRY_WHITEMANE, 9, whitemaneDamageTaken)
RegisterCreatureEvent(ENTRY_WHITEMANE, 15, whitemaneSpellHitTarget)
RegisterCreatureEvent(ENTRY_WHITEMANE, 23, whitemaneReset)
RegisterCreatureEvent(ENTRY_WHITEMANE, 28, whitemaneMovementInform)
