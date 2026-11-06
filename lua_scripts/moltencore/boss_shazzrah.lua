-- Shazzrah (Molten Core) — Lua port of
-- src/server/scripts/EasternKingdoms/BlackrockMountain/MoltenCore/
-- boss_shazzrah.cpp (boss_shazzrahAI only); molten_core.h:34
-- (BOSS_SHAZZRAH = 4, fifth boss), :58 (NPC_SHAZZRAH = 12264).
-- Creature entry: 12264 Shazzrah (C++ ScriptName "boss_shazzrah" per
-- AddSC_boss_shazzrah). No Talk lines in the C++ AI.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnEnterCombat arms
-- arcane explosion 19712 6s then {4s,7s}, non-triggered DoCastVictim /
-- Shazzrah's curse 19713 10s then {25s,30s}, non-triggered on a random
-- threat-list player alive without the curse aura (C++
-- SelectTarget(Random, 0, 0.0f, true, true, -SPELL_SHAZZRAH_CURSE) —
-- no-aura filter via the target HasAura bridge) /
-- magic
-- grounding 19714 24s then 35s, non-triggered self-cast / counterspell
-- 19715 15s then {16s,20s}, non-triggered DoCastVictim / Gate of
-- Shazzrah 45s then 45s (ResetThreatList PORTED 2026-10-19 via
-- creature:ResetAllThreat, bridged lua_objects.go; DoCastAOE(23138)
-- ports as non-triggered self-cast, selin_fireheart convention — the
-- SpellScript teleport/target-switch legs have no bridge): schedules the triggered arcane explosion
-- 2s later (non-triggered DoCastVictim 19712, one-shot) and
-- reschedules arcane explosion {3s,6s}. Nil-victim ticks cast nothing
-- but keep the schedule (jeklik convention). OnDied(4)/
-- OnLeaveCombat(2)/OnReset(23): cancel timers.
-- Cast gating PORTED (mal_ganis idiom): the C++ UpdateAI
-- UNIT_STATE_CASTING queue gate (boss_shazzrah.cpp:69) and post-event
-- check (:108) are modeled on every timer tick, including the one-shot
-- gate triggered explosion, via the live creature:IsCasting() bridge
-- (engine/world/lua_creature_events.go:1950, CastingUntil window) — a
-- casting tick defers 500ms under its own key, consuming no re-arm.
-- No instance-script model — boss admission via the luaBossAI shim;
-- the _Reset / _JustDied / _JustEngagedWith bookkeeping arms ARE
-- ported below (creature:SetBossState, bridge live
-- lua_creature_events.go:5917): ev1 IN_PROGRESS (JustEngagedWith
-- :52-54 calls the base first), ev23 NOT_STARTED (no Reset override —
-- base runs), ev4 DONE (no JustDied override — base runs by virtual
-- dispatch); the EVENT_SHAZZRAH_GATE
-- ResetThreatList arm IS ported (:154 — ResetAllThreat bridged,
-- lua_objects.go:389); the spell_shazzrah_gate_dummy SpellScript
-- (picks a random area target, that target casts 23139 teleporting
-- Shazzrah to it, then AttackStart on the target) has no SpellScript
-- bridge — the teleport never happens and the AttackStart has no
-- bearer (vaelastrasz convention) — the fight continues on the
-- existing victim instead of the gate target.

local ENTRY_SHAZZRAH = 12264

local SPELL_ARCANE_EXPLOSION = 19712
local SPELL_SHAZZRAH_CURSE = 19713
local SPELL_MAGIC_GROUNDING = 19714
local SPELL_COUNTERSPELL = 19715
local SPELL_SHAZZRAH_GATE_DUMMY = 23138

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

-- C++ EVENT_ARCANE_EXPLOSION: non-triggered DoCastVictim(19712);
-- re-arm {4s,7s}.
local function onArcaneExplosion(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "arcaneexplosion", 500, function()
            onArcaneExplosion(creature, guid)
        end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_ARCANE_EXPLOSION)
    end
    schedule(guid, "arcaneexplosion", math.random(4000, 7000), function()
        onArcaneExplosion(creature, guid)
    end)
end

-- C++ EVENT_SHAZZRAH_CURSE: non-triggered cast 19713 on a threat-list
-- player alive without aura 19713 (C++ SelectTarget(Random, 0, 0.0f,
-- true, true, -SPELL_SHAZZRAH_CURSE)); nil pick casts nothing but
-- keeps the schedule; re-arm {25s,30s}.
local function randomThreatPlayer(creature)
    local targets = creature:GetAITargets()
    local players = {}
    for _, unit in ipairs(targets) do
        if unit:GetObjectType() == "Player" and not unit:IsDead()
                and not unit:HasAura(SPELL_SHAZZRAH_CURSE) then
            players[#players + 1] = unit
        end
    end
    if #players == 0 then
        return nil
    end
    return players[math.random(#players)]
end

local function onShazzrahCurse(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "curse", 500, function()
            onShazzrahCurse(creature, guid)
        end)
        return
    end
    local pick = randomThreatPlayer(creature)
    if pick then
        creature:CastSpell(pick, SPELL_SHAZZRAH_CURSE)
    end
    schedule(guid, "curse", math.random(25000, 30000), function()
        onShazzrahCurse(creature, guid)
    end)
end

-- C++ EVENT_MAGIC_GROUNDING: non-triggered DoCast(me, 19714);
-- re-arm 35s.
local function onMagicGrounding(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "magicgrounding", 500, function()
            onMagicGrounding(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_MAGIC_GROUNDING)
    schedule(guid, "magicgrounding", 35000, function()
        onMagicGrounding(creature, guid)
    end)
end

-- C++ EVENT_COUNTERSPELL: non-triggered DoCastVictim(19715);
-- re-arm {16s,20s}.
local function onCounterspell(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "counterspell", 500, function()
            onCounterspell(creature, guid)
        end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_COUNTERSPELL)
    end
    schedule(guid, "counterspell", math.random(16000, 20000), function()
        onCounterspell(creature, guid)
    end)
end

-- C++ EVENT_SHAZZRAH_GATE: 45s then 45s. ResetThreatList PORTED
-- 2026-10-19 (creature:ResetAllThreat, bridged lua_objects.go);
-- the DoCastAOE(SPELL_SHAZZRAH_GATE_DUMMY
-- 23138) leg lowers to non-triggered self-cast (selin_fireheart
-- DoCastAOE convention — the old "no bridge" claim was wrong). The
-- spell_shazzrah_gate_dummy SpellScript (random area target, that
-- target casts 23139 teleporting Shazzrah to it, then AttackStart) has
-- no SpellScript bridge, so the cast fires visibly but the teleport +
-- target switch never happen (vaelastrasz convention).
local function onGateTriggeredExplosion(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "gateexplosion", 500, function()
            onGateTriggeredExplosion(creature, guid)
        end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_ARCANE_EXPLOSION)
    end
    timers[guid]["gateexplosion"] = nil
end

local function onShazzrahGate(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "gate", 500, function()
            onShazzrahGate(creature, guid)
        end)
        return
    end
    -- C++ EVENT_SHAZZRAH_GATE: ResetThreatList() before the gate dummy
    -- cast (bridged lua_objects.go:389; the old "no bridge" header claim
    -- was stale).
    creature:ResetAllThreat()
    creature:CastSpell(creature, SPELL_SHAZZRAH_GATE_DUMMY)
    schedule(guid, "gateexplosion", 2000, function()
        onGateTriggeredExplosion(creature, guid)
    end)
    schedule(guid, "arcaneexplosion", math.random(3000, 6000), function()
        onArcaneExplosion(creature, guid)
    end)
    schedule(guid, "gate", 45000, function()
        onShazzrahGate(creature, guid)
    end)
end

local function shazzrahEnterCombat(event, creature, target)
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542; sapphiron precedent)
    -- C++ JustEngagedWith (boss_shazzrah.cpp:52-54) calls
    -- BossAI::JustEngagedWith first; _JustEngagedWith writes
    -- SetBossState(BOSS_SHAZZRAH 4, IN_PROGRESS) (ScriptedCreature.cpp:537).
    creature:SetBossState(4, 1)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "arcaneexplosion", 6000, function()
        onArcaneExplosion(creature, guid)
    end)
    schedule(guid, "curse", 10000, function()
        onShazzrahCurse(creature, guid)
    end)
    schedule(guid, "magicgrounding", 24000, function()
        onMagicGrounding(creature, guid)
    end)
    schedule(guid, "counterspell", 15000, function()
        onCounterspell(creature, guid)
    end)
    schedule(guid, "gate", 45000, function()
        onShazzrahGate(creature, guid)
    end)
end

local function shazzrahResetState(guid)
    cancelTimers(guid)
end

local function shazzrahLeaveCombat(event, creature)
    shazzrahResetState(creature:GetGUID())
end

local function shazzrahDied(event, creature, killer)
    -- No JustDied override in C++ — base BossAI::JustDied runs by
    -- virtual dispatch (ScriptedCreature.h:330); _JustDied() writes
    -- SetBossState(BOSS_SHAZZRAH 4, DONE).
    creature:SetBossState(4, 3)
    shazzrahResetState(creature:GetGUID())
end

local function shazzrahReset(event, creature)
    -- No Reset override in C++ — base BossAI::Reset runs; _Reset()
    -- writes SetBossState(BOSS_SHAZZRAH 4, NOT_STARTED), alive-gated
    -- and skipped when already DONE (ScriptedCreature.cpp:499-511);
    -- Go fires ev23 on the death path too, hence the gate.
    if creature:IsAlive() and creature:GetBossState(4) ~= 3 then
        creature:SetBossState(4, 0)
    end
    shazzrahResetState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_SHAZZRAH, 1, shazzrahEnterCombat)
RegisterCreatureEvent(ENTRY_SHAZZRAH, 2, shazzrahLeaveCombat)
RegisterCreatureEvent(ENTRY_SHAZZRAH, 4, shazzrahDied)
RegisterCreatureEvent(ENTRY_SHAZZRAH, 23, shazzrahReset)
