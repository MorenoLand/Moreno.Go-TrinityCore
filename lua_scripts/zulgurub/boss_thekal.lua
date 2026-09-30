-- High Priest Thekal (Zul'Gurub) — Lua port of
-- src/server/scripts/EasternKingdoms/ZulGurub/boss_thekal.cpp
-- (boss_thekalAI + npc_zealot_lorkhanAI + npc_zealot_zathAI);
-- zulgurub.h:34 (DATA_THEKAL = 4, main boss), :40
-- (DATA_LORKHAN = 10), :41 (DATA_ZATH = 11), :52
-- (NPC_ZEALOT_LORKHAN = 11347), :53 (NPC_ZEALOT_ZATH = 11348), :55
-- (NPC_HIGH_PRIEST_THEKAL = 14509).
-- Creature entries: 14509 High Priest Thekal (C++ ScriptName
-- "boss_thekal" per AddSC_boss_thekal; zulgurub.h
-- NPC_HIGH_PRIEST_THEKAL); 11347 Zealot Lor'Khan ("npc_zealot_lorkhan");
-- 11348 Zealot Zath ("npc_zealot_zath").
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 9 OnDamageTaken, 23 OnReset. Timers via CreateLuaEvent; melee is
-- engine-driven in Go (creature combat tick), like C++.
-- Fight shape: enter combat -> arm mortal cleave 22859 4s then
-- {15s,20s} on the victim (non-triggered DoCastVictim, phase one only,
-- C++-exact) / silence 22666 9s then {20s,25s} on the victim
-- (non-triggered DoCastVictim, phase one only, C++-exact). No aggro
-- Talk in the C++ AI.
-- Fake death (phase one killing blow, thekal and both adds — the adds
-- have no phase gate in C++): Talk(TALK_FAKE_DEATH=2), triggered
-- self-cast 29266 (permanent feign death), the creature's timers are
-- pushed out 10s (C++ EventMap::DelayEvents), the kill is forwarded to
-- thekal's SetData(DATA_FAKE_DEATH, entry) and the damage is zeroed.
-- A feign-dead creature ignores further damage (C++-exact
-- approximation of the SetImmuneToPC/NPC flags, which have no bridge).
-- SetData: one or two of the three down -> thekal arms the 10s
-- resurrection timer (once); all three down -> thekal's timers are
-- canceled and the tiger-form phase change starts in 3s. Resurrect
-- timer: each flagged creature casts the resurrect visual 24171 on
-- itself (non-triggered DoCastSelf, C++-exact), drops the feign-death
-- aura and is healed to full (the SetImmuneToPC/NPC(false) arms have no
-- bridge). Phase change: root (no bridge), drop feign death, full
-- health, self-cast 24171 -> 1s -> Talk(TALK_TIGER_PHASE=0) -> 1s ->
-- self-cast 24169 (tiger form; the immune-flag removal, the +40% damage
-- stat modifier, ResetThreatList and the unroot arms have no bridge)
-- and arm force punch 24189 4s then {16s,21s} on the victim (triggered
-- DoCastVictim, C++-exact) / charge 24193 — C++ schedules
-- EVENT_SPELL_CHARGE (8) at 12s but the switch case reads EVENT_CHARGE
-- (SharedDefines.h:3342, value 1003), so the event never matches and
-- the cast never fires: modeled C++-exact as a single inert 12s firing
-- with no re-arm / summon tigers 24183 25s then {10s,14s} (triggered
-- DoCastVictim in C++; no summon model, so only the re-arm cycle is
-- kept, jeklik six-bat convention). Phase-two frenzy: dropping below
-- 10% health -> self-cast 8269 + Talk(TALK_FRENZY=3), once (the
-- HealthBelowPct check is pre-damage in C++ DamageTaken, C++-exact via
-- GetHealthPct with no adjustment). Phase-two death: Talk(TALK_DEATH=1).
-- Zealot Lor'Khan (11347): shield 20545 1s then 61s self-cast /
-- bloodlust 24185 16s then {20s,28s} self-cast / greater heal 24208 32s
-- then {15s,20s} on the most-damaged of thekal/lorkhan/zath that is
-- alive, in combat and has no feign-death aura (the 100-yd range check
-- is skipped — all three share the arena) / disarm 6713 6s then
-- {15s,25s} on the victim (all non-triggered, C++-exact); same
-- fake-death machine as thekal, forwarding SetData to thekal.
-- Zealot Zath (11348): sweeping strikes 18765 13s then {22s,26s} /
-- sinister strike 15581 8s then {8s,16s} / gouge 12540 25s then
-- {17s,27s} (the -100% threat cut has no bridge) / kick 15614 18s then
-- {15s,25s} / blind 21060 5s then {10s,20s}, all on the victim,
-- non-triggered (C++-exact); same fake-death machine as thekal. Zath's
-- C++ UpdateAI has no UNIT_STATE_CASTING gate.
-- The SetData AI-to-AI communication (adds -> thekal via
-- instance->GetCreature) is implemented as shared same-file encounter
-- state keyed by map+instance — no instance-script model is required
-- for it. Nil-victim ticks cast nothing but keep the schedule (jeklik
-- convention).
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING queue gate (thekal,
-- lorkhan) has no UNIT_STATE bridge — timers fire unconditionally (jeklik
-- convention); no instance-script model — boss admission via the luaBossAI
-- shim (_Reset/_JustDied/BossAI::JustEngagedWith, GetZulGurubAI
-- bookkeeping and the DATA_THEKAL/LORKHAN/ZATH encounter-state arms
-- skipped); the SetImmuneToPC/NPC flag arms and the RemoveFlag
-- un-immune arms have no flag bridge — feign death is aura-only and
-- further damage while feign-dead is zeroed to approximate immunity;
-- RemoveAllAuras has no bridge — only the feign-death aura is removed at
-- resurrect/phase change/reset; no stat-modifier bridge — the +40% tiger
-- damage hack and the Reset -damage hack are skipped (marli/arlokk
-- convention); no threat model — ResetThreatList and the gouge
-- ModifyThreatByPercent arms skipped; no movement model — the charge
-- AttackStart arm skipped (charge is inert anyway, see above); no
-- SetControlled bridge — the phase-change root/unroot arms skipped and
-- the _isChangingPhase melee skip has no bearer (melee is
-- engine-driven); no summon model — the SPELL_SUMMONTIGERS tigers never
-- spawn; thekal's JustDied KillSelf of the two adds has no kill bridge —
-- their timers are canceled instead; Lor'Khan's greater-heal 100-yd
-- range check is skipped.

local ENTRY_THEKAL = 14509
local ENTRY_LORKHAN = 11347
local ENTRY_ZATH = 11348

local SPELL_RESURRECT_VISUAL = 24171
local SPELL_PERMANENT_FEIGN_DEATH = 29266

local SPELL_MORTALCLEAVE = 22859
local SPELL_SILENCE = 22666
local SPELL_TIGER_FORM = 24169
local SPELL_FRENZY = 8269
local SPELL_FORCEPUNCH = 24189
local SPELL_CHARGE = 24193
local SPELL_SUMMONTIGERS = 24183

local SPELL_SHIELD = 20545
local SPELL_BLOODLUST = 24185
local SPELL_GREATERHEAL = 24208
local SPELL_DISARM = 6713

local SPELL_SWEEPINGSTRIKES = 18765
local SPELL_SINISTERSTRIKE = 15581
local SPELL_GOUGE = 12540
local SPELL_KICK = 15614
local SPELL_BLIND = 21060

local TALK_TIGER_PHASE = 0
local TALK_DEATH = 1
local TALK_FAKE_DEATH = 2
local TALK_FRENZY = 3

local PHASE_ONE = 1
local PHASE_TWO = 2

local timers = {}
local encounters = {}

local function nowMs()
    return math.floor(os.clock() * 1000)
end

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, t in pairs(per) do
            RemoveEventById(t.id)
        end
        timers[guid] = nil
    end
end

local function schedule(guid, key, delayMs, fn)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key].id)
    end
    per[key] = { id = CreateLuaEvent(fn, delayMs), due = nowMs() + delayMs, fn = fn }
end

-- C++ EventMap::DelayEvents: push every armed timer out by extraMs,
-- preserving each timer's remaining time.
local function delayTimers(guid, extraMs)
    local per = timers[guid]
    if not per then
        return
    end
    local now = nowMs()
    for key, t in pairs(per) do
        RemoveEventById(t.id)
        local remaining = t.due - now
        if remaining < 0 then
            remaining = 0
        end
        local fn = t.fn
        per[key] = { id = CreateLuaEvent(fn, remaining + extraMs), due = now + remaining + extraMs, fn = fn }
    end
end

-- Shared encounter state for thekal + both zealots: implements the C++
-- SetData(DATA_FAKE_DEATH / DATA_RESURRECTED) AI-to-AI communication
-- that C++ routes through instance->GetCreature.
local function encounterState(creature)
    local key = creature:GetMapId() .. ":" .. creature:GetInstanceId()
    local e = encounters[key]
    if not e then
        e = {
            phase = PHASE_ONE,
            thekalDead = false,
            lorkhanDead = false,
            zathDead = false,
            resurrectTimerActive = false,
            changingPhase = false,
            enraged = false,
            thekalGuid = nil,
            thekal = nil,
            lorkhanGuid = nil,
            lorkhan = nil,
            zathGuid = nil,
            zath = nil,
        }
        encounters[key] = e
    end
    return e
end

local function clearEncounter(e)
    e.phase = PHASE_ONE
    e.thekalDead = false
    e.lorkhanDead = false
    e.zathDead = false
    e.resurrectTimerActive = false
    e.changingPhase = false
    e.enraged = false
    e.thekalGuid = nil
    e.thekal = nil
    e.lorkhanGuid = nil
    e.lorkhan = nil
    e.zathGuid = nil
    e.zath = nil
end

-- C++ boss_thekalAI::SetData(DATA_RESURRECTED, entry): drop the feign
-- death aura and heal to full (immune-flag arms have no bridge).
local function resurrectCreature(c)
    c:RemoveAura(SPELL_PERMANENT_FEIGN_DEATH)
    c:SetHealth(c:GetMaxHealth())
end

local function onResurrectTimer(e, thekalGuid)
    e.resurrectTimerActive = false
    if e.thekalDead and e.thekal then
        e.thekal:CastSpell(e.thekal, SPELL_RESURRECT_VISUAL)
        resurrectCreature(e.thekal)
        e.thekalDead = false
    end
    if e.lorkhanDead and e.lorkhan then
        e.lorkhan:CastSpell(e.lorkhan, SPELL_RESURRECT_VISUAL)
        resurrectCreature(e.lorkhan)
        e.lorkhanDead = false
    end
    if e.zathDead and e.zath then
        e.zath:CastSpell(e.zath, SPELL_RESURRECT_VISUAL)
        resurrectCreature(e.zath)
        e.zathDead = false
    end
end

-- C++ EVENT_FORCEPUNCH: triggered DoCastVictim(24189); re-arm
-- {16s,21s}, phase two.
local function onForcePunch(e, creature, guid)
    if not creature or e.phase ~= PHASE_TWO then
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FORCEPUNCH, true)
    end
    schedule(guid, "forcepunch", math.random(16000, 21000), function()
        onForcePunch(e, creature, guid)
    end)
end

-- C++ EVENT_SUMMONTIGERS: triggered DoCastVictim(24183) in C++; no
-- summon model here, so only the {10s,14s} re-arm cycle is kept
-- (jeklik six-bat convention).
local function onSummonTigers(e, creature, guid)
    if not creature or e.phase ~= PHASE_TWO then
        return
    end
    schedule(guid, "summontigers", math.random(10000, 14000), function()
        onSummonTigers(e, creature, guid)
    end)
end

local function onChangePhase3(e, thekalGuid)
    e.changingPhase = false
    e.phase = PHASE_TWO
    local c = e.thekal
    if c then
        c:CastSpell(c, SPELL_TIGER_FORM)
    end
    schedule(thekalGuid, "forcepunch", 4000, function()
        onForcePunch(e, c, thekalGuid)
    end)
    -- C++ schedules EVENT_SPELL_CHARGE (8) but the switch case reads
    -- EVENT_CHARGE (1003): the event never matches, so the cast never
    -- fires — a single inert 12s firing, C++-exact, no re-arm.
    schedule(thekalGuid, "charge", 12000, function() end)
    schedule(thekalGuid, "summontigers", 25000, function()
        onSummonTigers(e, c, thekalGuid)
    end)
end

local function onChangePhase2(e, thekalGuid)
    local c = e.thekal
    if c then
        c:Talk(TALK_TIGER_PHASE)
    end
    schedule(thekalGuid, "change3", 1000, function()
        onChangePhase3(e, thekalGuid)
    end)
end

local function onChangePhase1(e, thekalGuid)
    local c = e.thekal
    e.changingPhase = true
    if c then
        c:RemoveAura(SPELL_PERMANENT_FEIGN_DEATH)
        c:SetHealth(c:GetMaxHealth())
        c:CastSpell(c, SPELL_RESURRECT_VISUAL)
    end
    schedule(thekalGuid, "change2", 1000, function()
        onChangePhase2(e, thekalGuid)
    end)
end

-- C++ boss_thekalAI::SetData(DATA_FAKE_DEATH, entry). The adds call this
-- through instance->GetCreature(DATA_THEKAL); here it is shared state.
-- In phase two the adds stay feign-dead (C++ never clears the flags),
-- so the whole machine is phase-one only.
local function thekalFakeDeath(e, entry)
    if e.phase ~= PHASE_ONE then
        return
    end
    if entry == ENTRY_THEKAL then
        e.thekalDead = true
    elseif entry == ENTRY_LORKHAN then
        e.lorkhanDead = true
    elseif entry == ENTRY_ZATH then
        e.zathDead = true
    else
        return
    end
    local thekalGuid = e.thekalGuid
    if not thekalGuid then
        return
    end
    if e.thekalDead and e.lorkhanDead and e.zathDead then
        e.resurrectTimerActive = false
        cancelTimers(thekalGuid)
        schedule(thekalGuid, "change1", 3000, function()
            onChangePhase1(e, thekalGuid)
        end)
    elseif not e.resurrectTimerActive then
        e.resurrectTimerActive = true
        schedule(thekalGuid, "resurrect", 10000, function()
            onResurrectTimer(e, thekalGuid)
        end)
    end
end

-- Shared fake-death DamageTaken arm (thekal phase-one, both adds in any
-- phase — C++ has no phase gate for the adds). Returns true when the hit
-- was converted into a fake death (damage must be zeroed by the caller).
local function fakeDeath(e, creature, entry, guid)
    creature:Talk(TALK_FAKE_DEATH)
    -- RemoveAllAuras / SetImmuneToPC / SetImmuneToNPC have no bridge —
    -- only the feign-death aura is applied.
    creature:CastSpell(creature, SPELL_PERMANENT_FEIGN_DEATH, true)
    delayTimers(guid, 10000)
    thekalFakeDeath(e, entry)
    return true
end

-- C++ EVENT_MORTALCLEAVE: non-triggered DoCastVictim(22859); re-arm
-- {15s,20s}, phase one.
local function onMortalCleave(e, creature, guid)
    if e.phase ~= PHASE_ONE then
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_MORTALCLEAVE)
    end
    schedule(guid, "mortalcleave", math.random(15000, 20000), function()
        onMortalCleave(e, creature, guid)
    end)
end

-- C++ EVENT_SILENCE: non-triggered DoCastVictim(22666); re-arm
-- {20s,25s}, phase one.
local function onSilence(e, creature, guid)
    if e.phase ~= PHASE_ONE then
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SILENCE)
    end
    schedule(guid, "silence", math.random(20000, 25000), function()
        onSilence(e, creature, guid)
    end)
end

local function thekalResetEncounter(e, creature)
    local guids = { e.thekalGuid, e.lorkhanGuid, e.zathGuid }
    for _, g in ipairs(guids) do
        if g then
            cancelTimers(g)
        end
    end
    clearEncounter(e)
    creature:RemoveAura(SPELL_PERMANENT_FEIGN_DEATH)
end

local function thekalEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    local e = encounterState(creature)
    cancelTimers(guid)
    clearEncounter(e)
    e.thekalGuid = guid
    e.thekal = creature
    schedule(guid, "mortalcleave", 4000, function()
        onMortalCleave(e, creature, guid)
    end)
    schedule(guid, "silence", 9000, function()
        onSilence(e, creature, guid)
    end)
end

-- C++ DamageTaken fires pre-damage (Unit::DealDamage calls it before
-- ModifyHealth), so both checks below are C++-exact with no adjustment.
local function thekalDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local e = encounterState(creature)
    e.thekalGuid = guid
    e.thekal = creature
    if e.phase == PHASE_ONE then
        if e.thekalDead then
            return false, 0
        end
        if damage >= creature:GetHealth() then
            fakeDeath(e, creature, ENTRY_THEKAL, guid)
            return false, 0
        end
        return
    end
    if not e.enraged and creature:GetHealthPct() < 10 then
        creature:CastSpell(creature, SPELL_FRENZY)
        creature:Talk(TALK_FRENZY)
        e.enraged = true
    end
end

local function thekalLeaveCombat(event, creature)
    thekalResetEncounter(encounterState(creature), creature)
end

local function thekalDied(event, creature, killer)
    local e = encounterState(creature)
    creature:Talk(TALK_DEATH)
    -- C++ KillSelf's the two adds here; no kill bridge — their timers
    -- are canceled instead and the encounter state is cleared.
    thekalResetEncounter(e, creature)
end

local function thekalReset(event, creature)
    -- C++ Reset's phase-two damage-decrease modifier, SetControlled
    -- root clear and immune-flag removal have no bridge — only the event
    -- reset, feign-death aura removal and encounter-state clear are
    -- modeled.
    thekalResetEncounter(encounterState(creature), creature)
end

RegisterCreatureEvent(ENTRY_THEKAL, 1, thekalEnterCombat)
RegisterCreatureEvent(ENTRY_THEKAL, 2, thekalLeaveCombat)
RegisterCreatureEvent(ENTRY_THEKAL, 4, thekalDied)
RegisterCreatureEvent(ENTRY_THEKAL, 9, thekalDamageTaken)
RegisterCreatureEvent(ENTRY_THEKAL, 23, thekalReset)

-- Zealot Lor'Khan (npc_zealot_lorkhan, C++ ScriptName for 11347).
local function onShield(e, creature, guid)
    creature:CastSpell(creature, SPELL_SHIELD)
    schedule(guid, "shield", 61000, function()
        onShield(e, creature, guid)
    end)
end

local function onBloodlust(e, creature, guid)
    creature:CastSpell(creature, SPELL_BLOODLUST)
    schedule(guid, "bloodlust", math.random(20000, 28000), function()
        onBloodlust(e, creature, guid)
    end)
end

-- C++ EVENT_GREATER_HEAL: most-damaged of thekal/lorkhan/zath that is
-- alive, in combat and has no feign-death aura -> non-triggered
-- DoCast(24208); re-arm {15s,20s} regardless.
local function onGreaterHeal(e, creature, guid)
    local best = nil
    local bestMissing = 0
    for _, c in ipairs({ e.thekal, e.lorkhan, e.zath }) do
        if c and not c:IsDead() and c:IsInCombat()
                and not c:HasAura(SPELL_PERMANENT_FEIGN_DEATH) then
            local missing = c:GetMaxHealth() - c:GetHealth()
            if missing > bestMissing then
                bestMissing = missing
                best = c
            end
        end
    end
    if best then
        creature:CastSpell(best, SPELL_GREATERHEAL)
    end
    schedule(guid, "greaterheal", math.random(15000, 20000), function()
        onGreaterHeal(e, creature, guid)
    end)
end

local function onDisarm(e, creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_DISARM)
    end
    schedule(guid, "disarm", math.random(15000, 25000), function()
        onDisarm(e, creature, guid)
    end)
end

local function lorkhanReset(e, creature, guid)
    cancelTimers(guid)
    e.lorkhanDead = false
    creature:RemoveAura(SPELL_PERMANENT_FEIGN_DEATH)
end

local function lorkhanEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    local e = encounterState(creature)
    cancelTimers(guid)
    e.lorkhanDead = false
    e.lorkhanGuid = guid
    e.lorkhan = creature
    schedule(guid, "shield", 1000, function()
        onShield(e, creature, guid)
    end)
    schedule(guid, "bloodlust", 16000, function()
        onBloodlust(e, creature, guid)
    end)
    schedule(guid, "greaterheal", 32000, function()
        onGreaterHeal(e, creature, guid)
    end)
    schedule(guid, "disarm", 6000, function()
        onDisarm(e, creature, guid)
    end)
end

local function lorkhanDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local e = encounterState(creature)
    e.lorkhanGuid = guid
    e.lorkhan = creature
    if e.lorkhanDead then
        return false, 0
    end
    if damage >= creature:GetHealth() then
        fakeDeath(e, creature, ENTRY_LORKHAN, guid)
        return false, 0
    end
end

RegisterCreatureEvent(ENTRY_LORKHAN, 1, lorkhanEnterCombat)
RegisterCreatureEvent(ENTRY_LORKHAN, 2, function(event, creature)
    local e = encounterState(creature)
    lorkhanReset(e, creature, creature:GetGUID())
end)
RegisterCreatureEvent(ENTRY_LORKHAN, 4, function(event, creature, killer)
    local e = encounterState(creature)
    lorkhanReset(e, creature, creature:GetGUID())
end)
RegisterCreatureEvent(ENTRY_LORKHAN, 9, lorkhanDamageTaken)
RegisterCreatureEvent(ENTRY_LORKHAN, 23, function(event, creature)
    local e = encounterState(creature)
    lorkhanReset(e, creature, creature:GetGUID())
end)

-- Zealot Zath (npc_zealot_zath, C++ ScriptName for 11348).
local function onSweepingStrikes(e, creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SWEEPINGSTRIKES)
    end
    schedule(guid, "sweeping", math.random(22000, 26000), function()
        onSweepingStrikes(e, creature, guid)
    end)
end

local function onSinisterStrike(e, creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SINISTERSTRIKE)
    end
    schedule(guid, "sinister", math.random(8000, 16000), function()
        onSinisterStrike(e, creature, guid)
    end)
end

local function onGouge(e, creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_GOUGE)
        -- the -100% ModifyThreatByPercent arm has no threat-model bridge
    end
    schedule(guid, "gouge", math.random(17000, 27000), function()
        onGouge(e, creature, guid)
    end)
end

local function onKick(e, creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_KICK)
    end
    schedule(guid, "kick", math.random(15000, 25000), function()
        onKick(e, creature, guid)
    end)
end

local function onBlind(e, creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_BLIND)
    end
    schedule(guid, "blind", math.random(10000, 20000), function()
        onBlind(e, creature, guid)
    end)
end

local function zathReset(e, creature, guid)
    cancelTimers(guid)
    e.zathDead = false
    creature:RemoveAura(SPELL_PERMANENT_FEIGN_DEATH)
end

local function zathEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    local e = encounterState(creature)
    cancelTimers(guid)
    e.zathDead = false
    e.zathGuid = guid
    e.zath = creature
    schedule(guid, "sweeping", 13000, function()
        onSweepingStrikes(e, creature, guid)
    end)
    schedule(guid, "sinister", 8000, function()
        onSinisterStrike(e, creature, guid)
    end)
    schedule(guid, "gouge", 25000, function()
        onGouge(e, creature, guid)
    end)
    schedule(guid, "kick", 18000, function()
        onKick(e, creature, guid)
    end)
    schedule(guid, "blind", 5000, function()
        onBlind(e, creature, guid)
    end)
end

local function zathDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local e = encounterState(creature)
    e.zathGuid = guid
    e.zath = creature
    if e.zathDead then
        return false, 0
    end
    if damage >= creature:GetHealth() then
        fakeDeath(e, creature, ENTRY_ZATH, guid)
        return false, 0
    end
end

RegisterCreatureEvent(ENTRY_ZATH, 1, zathEnterCombat)
RegisterCreatureEvent(ENTRY_ZATH, 2, function(event, creature)
    local e = encounterState(creature)
    zathReset(e, creature, creature:GetGUID())
end)
RegisterCreatureEvent(ENTRY_ZATH, 4, function(event, creature, killer)
    local e = encounterState(creature)
    zathReset(e, creature, creature:GetGUID())
end)
RegisterCreatureEvent(ENTRY_ZATH, 9, zathDamageTaken)
RegisterCreatureEvent(ENTRY_ZATH, 23, function(event, creature)
    local e = encounterState(creature)
    zathReset(e, creature, creature:GetGUID())
end)
