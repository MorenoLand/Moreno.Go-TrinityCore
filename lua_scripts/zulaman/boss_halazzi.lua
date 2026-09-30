-- Halazzi (Zul'Aman) — Lua port of
-- src/server/scripts/EasternKingdoms/ZulAman/boss_halazzi.cpp
-- (boss_halazziAI + npc_halazzi_lynxAI); zulaman.h:31 (BOSS_HALAZZI = 3),
-- zulaman.h:48 (NPC_HALAZZI = 23577).
-- Creature entries: 23577 Halazzi (C++ ScriptName "boss_halazzi" per
-- AddSC_boss_halazzi); 24143 Spirit Lynx (C++ ScriptName
-- "npc_halazzi_lynx"). Entry 24224 (Totem) has no C++ script and is not
-- registered.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 9 OnDamageTaken (returns false, newDamage for the never-die
-- clamps), 14 OnHitBySpell (the SPELL_TRANSFORM_SPLIT2 merge arm),
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Fight shape: lynx phase opens with frenzy 43139 at 16s (repeat
-- {10s,15s}) and saber lash 43267 at 20s (repeat 30s, victim via
-- nil-target fallback). The 1s check timer drops to the SPLIT phase below
-- 25%*(3-TransformCount) health: Talk(SAY_SPLIT), triggered 43142
-- self-cast, then the human phase — shock at 10s (repeat {10s,15s},
-- flameshock 43303 on a random alive player) and summon totem 43302 at
-- 12s (repeat 20s). The 1s check timer merges below 20% boss health:
-- Talk(SAY_MERGE), merge walk, then back to lynx (health reset to
-- 600000-150000*TransformCount, C++-exact fractions) or to the ENRAGE
-- phase after 3 merges — enrage keeps frenzy, saber lash, shock and totem
-- with no further splits. Berserk 45078 one-shot 10min with
-- Talk(SAY_BERSERK), repeat 60s (C++-exact). The damage hook zeroes lethal
-- damage outside the enrage phase (C++ DamageTaken), and the lynx zeroes
-- lethal damage unconditionally.
-- Deviations from C++: no summon model — the spirit lynx is never
-- spawned (the commented-out SPELL_SUMMON_LYNX and the C++
-- DoSpawnCreature have no bearer), the merge-walk has no movement model
-- (MoveFollow/Clear have no bridge), so the merge completes on a 2s timer
-- instead of the 6-yd proximity check, and the lynx-health merge arm has
-- no bearer; the transform-completion signal (SPELL_TRANSFORM_SPLIT2
-- 43573 SpellHit) has no firing hook — the engine's event 14 fires only
-- for player casts (the boss's own split self-cast is visual-only on the
-- bridge), so an event-14 guard for 43573 is kept but the actual human
-- entry runs on a 2s timer approximating the transform; no SetMaxHealth
-- on the bridge, so the per-cycle health reset is mapped as
-- SetHealth(max - max/4*TransformCount) — the exact C++ fractions
-- (600000-150000*count) on the engine's fixed max; no instance-script
-- model — admission only via the luaBossAI shim; no cast-state model, so
-- the shock target's IsNonMeleeSpellCast check always takes the
-- flameshock branch (earthshock unreachable); the saber-lash defense
-- comment arm is dead C++ (not modeled); AttackStart's merge-phase
-- suppression and the lynx's NON_ATTACKABLE AttackStart gate have no
-- bridge (engine combat is continuous).

local ENTRY_HALAZZI = 23577
local ENTRY_SPIRIT_LYNX = 24143

local PHASE_NONE = 0
local PHASE_LYNX = 1
local PHASE_SPLIT = 2
local PHASE_HUMAN = 3
local PHASE_MERGE = 4
local PHASE_ENRAGE = 5

local SAY_AGGRO = 0
local SAY_SPLIT = 2
local SAY_MERGE = 3
local SAY_KILL = 4
local SAY_DEATH = 5
local SAY_BERSERK = 6

local SPELL_DUAL_WIELD = 29651
local SPELL_SABER_LASH = 43267
local SPELL_FRENZY = 43139
local SPELL_FLAMESHOCK = 43303
local SPELL_EARTHSHOCK = 43305
local SPELL_TRANSFORM_SPLIT = 43142
local SPELL_TRANSFORM_SPLIT2 = 43573
local SPELL_TRANSFORM_MERGE = 43271
local SPELL_SUMMON_TOTEM = 43302
local SPELL_BERSERK = 45078
local SPELL_LYNX_FRENZY = 43290
local SPELL_SHRED_ARMOR = 43243

local timers = {}
local lynxTimers = {}
local state = {}
local lynxState = {}

local function cancelTimers(store, guid)
    local per = store[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        store[guid] = nil
    end
end

local function schedule(store, guid, key, delay, fn)
    local per = store[guid]
    if not per then
        per = {}
        store[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

local function alivePlayersInRange(creature, maxDist)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() and creature:GetDistance(p) <= maxDist then
            found[#found + 1] = p
        end
    end
    return found
end

local function randomPlayerInRange(creature, maxDist)
    local candidates = alivePlayersInRange(creature, maxDist)
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

local function halazziState(guid)
    local st = state[guid]
    if not st then
        st = { phase = PHASE_NONE, transformCount = 0 }
        state[guid] = st
    end
    return st
end

local function cancelPhaseTimers(guid)
    local per = timers[guid]
    if not per then
        return
    end
    for _, key in ipairs({ "frenzy", "saberlash", "shock", "totem", "check",
            "splitwait", "mergewait" }) do
        if per[key] then
            RemoveEventById(per[key])
            per[key] = nil
        end
    end
end

local enterPhase

-- C++ EnterPhase(PHASE_SPLIT) (called from the 1s check timer when
-- HealthBelowPct(25*(3-TransformCount)) in the lynx phase): Talk, triggered
-- split self-cast; the SPELL_TRANSFORM_SPLIT2 SpellHit arm has no firing
-- hook (see onSplitFallback).
local function startSplit(creature, guid)
    local st = halazziState(guid)
    if st.phase ~= PHASE_LYNX then
        return
    end
    st.phase = PHASE_SPLIT
    cancelPhaseTimers(guid)
    creature:Talk(SAY_SPLIT)
    creature:CastSpell(creature, SPELL_TRANSFORM_SPLIT, true)
    -- The C++ SpellHit(43573) -> EnterPhase(PHASE_HUMAN) arm never fires
    -- through the bridge (the boss's self-cast is visual-only and event 14
    -- fires only for player casts), so the transform completes on this
    -- timer, approximating the split transform; the event-14 guard below
    -- stays as a no-op match for the C++ SpellHit arm.
    schedule(timers, guid, "splitwait", 2000, function()
        if halazziState(guid).phase == PHASE_SPLIT then
            enterPhase(creature, guid, PHASE_HUMAN)
        end
    end)
end

-- C++ EVENT frenzy (lynx/enrage phases): DoCast(me, FRENZY) non-triggered.
-- Repeat {10s,15s}.
local function onFrenzy(creature, guid)
    creature:CastSpell(creature, SPELL_FRENZY)
    schedule(timers, guid, "frenzy", math.random(10000, 15000), function()
        onFrenzy(creature, guid)
    end)
end

-- C++ EVENT saber lash (lynx/enrage phases): DoCastVictim(SABER_LASH,
-- true). Repeat 30s. nil-target falls back to the victim on the bridge.
local function onSaberLash(creature, guid)
    if creature:GetVictim() then
        creature:CastSpell(nil, SPELL_SABER_LASH, true)
    end
    schedule(timers, guid, "saberlash", 30000, function()
        onSaberLash(creature, guid)
    end)
end

-- C++ shock (human/enrage phases): random target; earthshock when the
-- target is casting, flameshock otherwise. No cast-state model — the
-- flameshock branch always runs. Repeat {10s,15s}.
local function onShock(creature, guid)
    local target = randomPlayerInRange(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_FLAMESHOCK)
    end
    schedule(timers, guid, "shock", math.random(10000, 15000), function()
        onShock(creature, guid)
    end)
end

-- C++ totem (human/enrage phases): DoCast(me, SUMMON_TOTEM) — the totem
-- never spawns without a summon model, so the cast is visual-only.
-- Repeat 20s.
local function onTotem(creature, guid)
    creature:CastSpell(creature, SPELL_SUMMON_TOTEM)
    schedule(timers, guid, "totem", 20000, function()
        onTotem(creature, guid)
    end)
end

-- C++ merge completion (MoveFollow both ways until within 6 yd): no
-- movement model, so the merge completes on a timer. TransformCount
-- increments here — C++ increments it when the lynx exists; the lynx
-- never exists here, so the counter moves with the (unavoidable) merge.
local function onMergeComplete(creature, guid)
    local st = halazziState(guid)
    if st.phase ~= PHASE_MERGE then
        return
    end
    st.transformCount = st.transformCount + 1
    if st.transformCount < 3 then
        enterPhase(creature, guid, PHASE_LYNX)
    else
        enterPhase(creature, guid, PHASE_ENRAGE)
    end
end

-- C++ 1s check timer: lynx phase -> split below 25%*(3-TransformCount);
-- human phase -> merge below 20% boss health (the lynx-health arm has no
-- bearer since the lynx never spawns).
local function onCheck(creature, guid)
    local st = halazziState(guid)
    local health, maxHealth = creature:GetHealth(), creature:GetMaxHealth()
    if st.phase == PHASE_LYNX and maxHealth > 0 then
        if health * 100 / maxHealth < 25 * (3 - st.transformCount) then
            startSplit(creature, guid)
        end
    elseif st.phase == PHASE_HUMAN and maxHealth > 0 then
        if health * 100 / maxHealth <= 20 then
            st.phase = PHASE_MERGE
            cancelPhaseTimers(guid)
            creature:Talk(SAY_MERGE)
            -- No MoveFollow bridge: the merge walk is approximated by the
            -- timer; C++ increments TransformCount when the lynx is found,
            -- done in onMergeComplete here.
            schedule(timers, guid, "mergewait", 2000, function()
                onMergeComplete(creature, guid)
            end)
        end
    end
    if halazziState(guid).phase ~= PHASE_NONE then
        schedule(timers, guid, "check", 1000, function()
            onCheck(creature, guid)
        end)
    end
end

-- C++ EnterPhase: arm the timers for the new phase and reset the per-phase
-- health on lynx/enrage entry (C++: max 600000, health 600000-150000*count
-- — mapped onto the engine's fixed max as the exact fractions, since
-- SetMaxHealth has no bridge).
enterPhase = function(creature, guid, nextPhase)
    local st = halazziState(guid)
    cancelPhaseTimers(guid)
    if nextPhase == PHASE_LYNX or nextPhase == PHASE_ENRAGE then
        if st.phase == PHASE_MERGE then
            creature:CastSpell(creature, SPELL_TRANSFORM_MERGE, true)
            -- me->Attack(victim, true) + MoveChase have no bridge; engine
            -- melee is continuous.
        end
        -- The lynx DisappearAndDie has no bearer (the lynx never spawns).
        local maxHealth = creature:GetMaxHealth()
        if maxHealth > 0 then
            creature:SetHealth(maxHealth - math.floor(maxHealth / 4)
                * st.transformCount)
        end
        st.phase = nextPhase
        schedule(timers, guid, "frenzy", 16000, function()
            onFrenzy(creature, guid)
        end)
        schedule(timers, guid, "saberlash", 20000, function()
            onSaberLash(creature, guid)
        end)
        if nextPhase == PHASE_ENRAGE then
            schedule(timers, guid, "shock", 10000, function()
                onShock(creature, guid)
            end)
            schedule(timers, guid, "totem", 12000, function()
                onTotem(creature, guid)
            end)
        end
    elseif nextPhase == PHASE_HUMAN then
        -- C++ DoSpawnCreature(SPIRIT_LYNX) skipped — no summon model.
        -- The max-health reset (400000/400000) is subsumed by the
        -- ratio-preserving heal on merge return.
        st.phase = PHASE_HUMAN
        schedule(timers, guid, "shock", 10000, function()
            onShock(creature, guid)
        end)
        schedule(timers, guid, "totem", 12000, function()
            onTotem(creature, guid)
        end)
    else
        st.phase = nextPhase
    end
    if nextPhase ~= PHASE_NONE then
        schedule(timers, guid, "check", 1000, function()
            onCheck(creature, guid)
        end)
    end
end

-- C++ berserk: one-shot 10min, Talk, triggered self-cast; repeat 60s
-- (C++-exact).
local function onBerserk(creature, guid)
    creature:Talk(SAY_BERSERK)
    creature:CastSpell(creature, SPELL_BERSERK, true)
    schedule(timers, guid, "berserk", 60000, function()
        onBerserk(creature, guid)
    end)
end

local function halazziResetState(guid)
    cancelTimers(timers, guid)
    state[guid] = { phase = PHASE_NONE, transformCount = 0 }
end

local function halazziEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    halazziResetState(guid)
    creature:Talk(SAY_AGGRO)
    -- C++ JustEngagedWith: Talk + EnterPhase(PHASE_LYNX).
    enterPhase(creature, guid, PHASE_LYNX)
    schedule(timers, guid, "berserk", 600000, function()
        onBerserk(creature, guid)
    end)
end

local function halazziLeaveCombat(event, creature)
    halazziResetState(creature:GetGUID())
end

local function halazziTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

local function halazziDied(event, creature, killer)
    halazziResetState(creature:GetGUID())
    creature:Talk(SAY_DEATH)
end

local function halazziReset(event, creature)
    local guid = creature:GetGUID()
    halazziResetState(guid)
    -- C++ Reset: triggered dual-wield self-cast, then EnterPhase(LYNX).
    creature:CastSpell(creature, SPELL_DUAL_WIELD, true)
    enterPhase(creature, guid, PHASE_LYNX)
end

-- C++ DamageTaken: lethal damage is zeroed outside the enrage phase.
local function halazziDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local st = state[guid]
    if not st then
        return
    end
    local health = creature:GetHealth()
    local newDamage = damage
    if damage >= health and st.phase ~= PHASE_ENRAGE then
        newDamage = 0
    end
    return false, newDamage
end

-- C++ SpellHit(TRANSFORM_SPLIT2) -> EnterPhase(PHASE_HUMAN). The engine's
-- event 14 fires only for player casts, so in practice the splitwait
-- timer in startSplit is the bearer; this guard keeps the C++ arm wired.
local function halazziSpellHit(event, creature, spellId)
    if spellId == SPELL_TRANSFORM_SPLIT2
            and halazziState(creature:GetGUID()).phase == PHASE_SPLIT then
        enterPhase(creature, creature:GetGUID(), PHASE_HUMAN)
    end
end

RegisterCreatureEvent(ENTRY_HALAZZI, 1, halazziEnterCombat)
RegisterCreatureEvent(ENTRY_HALAZZI, 2, halazziLeaveCombat)
RegisterCreatureEvent(ENTRY_HALAZZI, 3, halazziTargetDied)
RegisterCreatureEvent(ENTRY_HALAZZI, 4, halazziDied)
RegisterCreatureEvent(ENTRY_HALAZZI, 9, halazziDamageTaken)
RegisterCreatureEvent(ENTRY_HALAZZI, 14, halazziSpellHit)
RegisterCreatureEvent(ENTRY_HALAZZI, 23, halazziReset)

-- npc_halazzi_lynx: frenzy 43290 self-cast every {30s,50s}, shred armor
-- 43243 on the victim every 4s. The C++ DamageTaken never-die arm is kept
-- on the damage hook; the NON_ATTACKABLE AttackStart gate, the lynx
-- DisappearAndDie, and the merge MoveFollow arms have no bridge (the lynx
-- never spawns from this script, but any spawned 24143 fights correctly).
local function onLynxFrenzy(creature, guid)
    creature:CastSpell(creature, SPELL_LYNX_FRENZY)
    schedule(lynxTimers, guid, "frenzy", math.random(30000, 50000), function()
        onLynxFrenzy(creature, guid)
    end)
end

local function onShredArmor(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SHRED_ARMOR)
    end
    schedule(lynxTimers, guid, "shred", 4000, function()
        onShredArmor(creature, guid)
    end)
end

local function lynxResetState(guid)
    cancelTimers(lynxTimers, guid)
    lynxState[guid] = nil
end

local function lynxEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    lynxResetState(guid)
    schedule(lynxTimers, guid, "frenzy", math.random(30000, 50000), function()
        onLynxFrenzy(creature, guid)
    end)
    schedule(lynxTimers, guid, "shred", 4000, function()
        onShredArmor(creature, guid)
    end)
end

local function lynxLeaveCombat(event, creature)
    lynxResetState(creature:GetGUID())
end

local function lynxDied(event, creature, killer)
    lynxResetState(creature:GetGUID())
end

local function lynxReset(event, creature)
    lynxResetState(creature:GetGUID())
end

-- C++ lynx DamageTaken: lethal damage is zeroed unconditionally.
local function lynxDamageTaken(event, creature, attacker, damage)
    local health = creature:GetHealth()
    local newDamage = damage
    if damage >= health then
        newDamage = 0
    end
    return false, newDamage
end

RegisterCreatureEvent(ENTRY_SPIRIT_LYNX, 1, lynxEnterCombat)
RegisterCreatureEvent(ENTRY_SPIRIT_LYNX, 2, lynxLeaveCombat)
RegisterCreatureEvent(ENTRY_SPIRIT_LYNX, 4, lynxDied)
RegisterCreatureEvent(ENTRY_SPIRIT_LYNX, 9, lynxDamageTaken)
RegisterCreatureEvent(ENTRY_SPIRIT_LYNX, 23, lynxReset)
