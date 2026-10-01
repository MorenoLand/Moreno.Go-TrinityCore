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
-- - Reset (23, C++-exact for the C++ Reset / JustReachedHome channeling
--   legs): self-cast SPELL_BLACKFATHOM_CHANNELING 8734 (non-triggered,
--   DoCastSelf).
-- - JustEngagedWith: Talk SAY_AGGRO(0); RemoveAurasDueToSpell(8734);
--   Mind Blast 15587 DoCastVictim, init {2s,5s} -> repeat {7s,9s}
--   (C++-exact; jeklik GetVictim + CastSpell convention, non-triggered).
-- - Sleep 8399: SelectTarget(Random,0,100,true) (janalai
--   randomPlayerInRange convention), Talk SAY_SLEEP(1) + DoCast on the
--   target (C++-exact — no triggered flag), init {9s,12s} -> repeat
--   {15s,20s}. If no target is found the arm no-ops but still re-arms,
--   matching C++'s unconditional re-schedule outside the target gate.
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

local function randomPlayerInRange(creature, maxDist)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() and creature:GetDistance(p) <= maxDist then
            found[#found + 1] = p
        end
    end
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
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

-- C++ Reset / JustReachedHome: DoCastSelf(SPELL_BLACKFATHOM_CHANNELING).
-- Eluna event 23 is the only Lua-modelable reset hook.
local function kelrisOnReset(event, creature)
    cancelTimers(creature:GetGUID())
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
