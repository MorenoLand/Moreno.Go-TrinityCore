-- Twilight Lord Kelris (Blackfathom Deeps) — Lua port of
-- src/server/scripts/Kalimdor/BlackfathomDeeps/boss_kelris.cpp
-- (boss_kelrisAI : public BossAI(creature, DATA_KELRIS); AddSC_boss_kelris
-- registers the one script; kalimdor loader decl/call per
-- kalimdor_script_loader.cpp).
-- Whole-server-tree quoted-name grep confirms boss_kelris.cpp as the sole
-- source of the "boss_kelris" script name (loader lines only otherwise).
-- Entry (blackfathom_deeps.h BFDCreatureIds): NPC_TWILIGHT_LORD_KELRIS =
-- 4832, corroborated by a second C++ source — instance_blackfathom_deeps.
-- cpp's OnCreatureCreate cases it (twilightLordKelrisGUID) — the name-to-
-- entry tie is C++-verified (kalecgos entry-verifiability check, ramstein
-- strength); the creature_template ScriptName binding stays DB-side.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady. There is no
-- UNIT_STATE_CASTING gate in Go, so timers fire unconditionally (maiden
-- precedent).
-- Ported arms:
-- - Reset (23): self-cast SPELL_BLACKFATHOM_CHANNELING 8734 (non-triggered,
--   DoCastSelf) — C++ casts it again at JustReachedHome, ported below on
--   event 24 (this tick; the old "one cast covers both" note was
--   C++-inexact — C++ auras are cleared by RemoveAurasOnEvade and the
--   cast is re-applied at the home-move completion).
-- - JustEngagedWith: Talk SAY_AGGRO(0); RemoveAurasDueToSpell(8734);
--   Mind Blast 15587 DoCastVictim, init {2s,5s} -> repeat {7s,9s}
--   (C++-exact; jeklik GetVictim + CastSpell convention, non-triggered).
-- - Sleep 8399: SelectTarget(Random,0,100,true) — random alive player
--   on the threat list within 100yd (UnitAI.h:162-170: SelectTarget
--   draws from the threat list; gerstahn randomPlayerTarget precedent),
--   Talk SAY_SLEEP(1) + DoCast on the target (C++-exact — no triggered
--   flag), init {9s,12s} -> repeat {15s,20s}. If no target is found the
--   arm no-ops but still re-arms, matching C++'s unconditional
--   re-schedule outside the target gate.
-- - Death: Talk SAY_DEATH(2).
-- Unmodeled (documented-only, no bridges): the BossAI _Reset() /
-- _JustReachedHome() / _JustDied() instance bookkeeping legs — no
-- instance-script model (admission only via the luaBossAI shim); the
-- GetBlackfathomDeepsAI -> GetInstanceAI leg (instance-script model
-- blocked, standing). C++ has no KilledUnit arms; its Reset leg is only the
-- channeling cast above.

local ENTRY_KELRIS = 4832

local SAY_AGGRO = 0
local SAY_SLEEP = 1
local SAY_DEATH = 2

local SPELL_MIND_BLAST = 15587
local SPELL_SLEEP = 8399
local SPELL_BLACKFATHOM_CHANNELING = 8734

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

-- C++ SelectTarget(Random, 0, 100, true): random alive player on the
-- threat list within 100yd (UnitAI.h:162-170 — SelectTarget draws from
-- the threat list; gerstahn randomPlayerTarget precedent). The old
-- GetPlayersInWorld pool wrongly included players never engaged with
-- the boss (bystanders in the instance).
local function randomPlayerInRange(creature, maxDist)
    local candidates = {}
    local reach = creature:GetCombatReach()
    for _, t in ipairs(creature:GetAITargets()) do
        if t:GetObjectType() == "Player" and t:IsAlive() then
            if maxDist <= 0 or creature:GetDistance(t) - reach - 1.5 < maxDist then -- IsWithinCombatRange gate, reach-exact (Unit.cpp:583; player reach 1.5)
                candidates[#candidates + 1] = t
            end
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EVENT_MIND_BLAST: DoCastVictim, non-triggered (jeklik convention).
local function onMindBlast(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_MIND_BLAST)
    end
    schedule(guid, "mindblast", math.random(7000, 9000), function()
        onMindBlast(creature, guid)
    end)
end

-- C++ EVENT_SLEEP: random player within 100 -> Talk SAY_SLEEP + DoCast,
-- non-triggered; re-arm is unconditional (C++ schedules outside the
-- target gate).
local function onSleep(creature, guid)
    local target = randomPlayerInRange(creature, 100)
    if target then
        creature:Talk(SAY_SLEEP)
        creature:CastSpell(target, SPELL_SLEEP)
    end
    schedule(guid, "sleep", math.random(15000, 20000), function()
        onSleep(creature, guid)
    end)
end

local function kelrisLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function kelrisDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(SAY_DEATH)
end

-- C++ Reset: DoCastSelf(SPELL_BLACKFATHOM_CHANNELING).
local function kelrisOnReset(event, creature)
    cancelTimers(creature:GetGUID())
    -- C++ Reset: non-triggered DoCastSelf (Reset-only arm,
    -- boss_kelris.cpp; the JustReachedHome leg rides the separate
    -- ev24 handler). Event 23 fires on death too (engine
    -- reset-then-died order) but C++ Reset() never runs on death,
    -- so the cast is alive-gated.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_BLACKFATHOM_CHANNELING)
    end
end

-- C++ JustReachedHome: the same ungated DoCastSelf(SPELL_BLACKFATHOM_CHANNELING)
-- at the home-move completion (event 24), non-triggered like the Reset leg.
local function kelrisReachHome(event, creature)
    creature:CastSpell(creature, SPELL_BLACKFATHOM_CHANNELING)
end

local function kelrisEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    creature:RemoveAura(SPELL_BLACKFATHOM_CHANNELING)
    schedule(guid, "mindblast", math.random(2000, 5000), function()
        onMindBlast(creature, guid)
    end)
    schedule(guid, "sleep", math.random(9000, 12000), function()
        onSleep(creature, guid)
    end)
end

RegisterCreatureEvent(ENTRY_KELRIS, 1, kelrisEnterCombat)
RegisterCreatureEvent(ENTRY_KELRIS, 2, kelrisLeaveCombat)
RegisterCreatureEvent(ENTRY_KELRIS, 4, kelrisDied)
RegisterCreatureEvent(ENTRY_KELRIS, 23, kelrisOnReset)
RegisterCreatureEvent(ENTRY_KELRIS, 24, kelrisReachHome)
