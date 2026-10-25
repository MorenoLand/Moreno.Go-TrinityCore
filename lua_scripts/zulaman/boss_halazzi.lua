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
-- earthshock 43305 when the target is casting (player IsCasting bridge),
-- else flameshock 43303, on a random alive player) and summon totem 43302 at
-- 12s (repeat 20s). The 1s check timer merges below 20% boss health:
-- Talk(SAY_MERGE), merge walk, then back to lynx (health reset to
-- 600000-150000*TransformCount, C++-exact fractions) or to the ENRAGE
-- phase after 3 merges — enrage keeps frenzy, saber lash, shock and totem
-- with no further splits. Berserk 45078 one-shot 10min with
-- Talk(SAY_BERSERK), repeat 60s (C++-exact). The damage hook zeroes lethal
-- damage outside the enrage phase (C++ DamageTaken), and the lynx zeroes
-- lethal damage unconditionally.
-- Deviations from C++: the spirit lynx IS summoned now (2026-10-24
-- summon bridge — DoSpawnCreature at boss +5,+5, type 4; the
-- commented-out SPELL_SUMMON_LYNX stays unmodeled), the merge-walk has no movement model
-- (MoveFollow/Clear have no bridge), so the merge completes on the C++
-- 1s proximity poll (6-yd IsWithinDistInMap check, boss_halazzi.cpp:292-304)
-- plus a 2s timer fallback for lynxes that never close distance; the lynx-health merge arm IS
-- ported now (2026-10-24, lynx summoned since 07:27), the lynx
-- DisappearAndDie on lynx/enrage entry IS ported (Despawn twin), and
-- the merge-entry NON_ATTACKABLE flag IS ported (SetFlag twin); the transform-completion signal (SPELL_TRANSFORM_SPLIT2
-- 43573 SpellHit) has no firing hook — the engine's event 14 fires only
-- for player casts (the boss's own split self-cast is visual-only on the
-- bridge), so an event-14 guard for 43573 is kept but the actual human
-- entry runs on a 2s timer approximating the transform; the per-cycle
-- health reset is ported as SetMaxHealth(600000) +
-- SetHealth(600000-150000*TransformCount) (boss_halazzi.cpp:164-165,
-- C++-exact; the old "no SetMaxHealth bridge" claim was stale — the
-- bridge is live and used at the lynx/human entries below); no instance-script
-- model — admission only via the luaBossAI shim; the shock target's
-- IsNonMeleeSpellCast check uses the live player IsCasting bridge
-- (Go activeCast is the cast-bar phase, like C++'s non-melee check);
-- the saber-lash defense
-- comment arm is dead C++ (not modeled); AttackStart's merge-phase
-- suppression and the lynx's NON_ATTACKABLE AttackStart gate have no
-- bridge (engine combat is continuous).

local ENTRY_HALAZZI = 23577
local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NON_ATTACKABLE = 0x2
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

-- C++ SelectTarget(Random, 0) with playerOnly=FALSE (UnitAI.h:170):
-- random over the threat list, pets/totems eligible — no range cap
-- (the old 100yd randomPlayerInRange cap was invented; C++ carries
-- the 0.0f dist default). Bridged via creature:GetAITargets()
-- (threat-ordered; lua_creature_events.go:345), the boss_midnight
-- randomTargetAnywhere precedent.
local function randomThreatTarget(creature)
    local targets = creature:GetAITargets()
    if #targets == 0 then
        return nil
    end
    return targets[math.random(#targets)]
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
            "splitwait", "mergewait", "mergepoll" }) do
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
    -- C++ UpdateAI returns on !UpdateVictim(): Reset arms the timers via
    -- EnterPhase(PHASE_LYNX) but they never fire out of combat — gate
    -- the cast; the re-arm below keeps the cadence.
    if creature:GetVictim() then
        creature:CastSpell(creature, SPELL_FRENZY)
    end
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
-- target is casting, flameshock otherwise. creature-side IsCasting is the
-- vacuous-false twin (no cast state on handler creatures); players carry
-- the live bridge (lua_player.go:619). The {10s,15s} re-arm lives inside the if(target) leg (nil target
-- retries in 1s — C++ leaves ShockTimer at 0, next tick retries).
local function onShock(creature, guid)
    local target = randomThreatTarget(creature)
    if target then
        if target:IsCasting() then
            creature:CastSpell(target, SPELL_EARTHSHOCK)
        else
            creature:CastSpell(target, SPELL_FLAMESHOCK)
        end
        schedule(timers, guid, "shock", math.random(10000, 15000), function()
            onShock(creature, guid)
        end)
    else
        -- C++ ShockTimer stays 0 on a nil target (retry next tick);
        -- the arm is not dropped.
        schedule(timers, guid, "shock", 1000, function()
            onShock(creature, guid)
        end)
    end
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
-- movement model, so the merge completes on the 1s proximity poll
-- (onMergePoll) or the 2s timer fallback. TransformCount increments
-- here — C++ increments it in EnterPhase(PHASE_MERGE) when the lynx
-- exists; the timer fallback moves it unconditionally so a missing
-- lynx cannot soft-lock the merge.
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

-- C++ PHASE_MERGE 1s check (boss_halazzi.cpp:292-304): each tick the
-- lynx and boss MoveFollow each other and the merge completes when
-- within 6 yd. MoveFollow has no bridge, so a 1s proximity poll twins
-- the check (IsWithinDistInMap bridged via the generic object
-- fallback: same-map gate, raw 3D distance with no combat-reach
-- subtraction — a small conservative deviation); the 2s mergewait
-- timer stays as the fallback for lynxes that never close distance.
local function onMergePoll(creature, guid)
    local st = halazziState(guid)
    if st.phase ~= PHASE_MERGE then
        return
    end
    local lynx = creature:FindNearestCreature(ENTRY_SPIRIT_LYNX, 100.0)
    if lynx and creature:IsWithinDistInMap(lynx, 6.0) then
        onMergeComplete(creature, guid)
        return
    end
    schedule(timers, guid, "mergepoll", 1000, function()
        onMergePoll(creature, guid)
    end)
end

-- C++ EnterPhase(PHASE_MERGE) (boss_halazzi.cpp:182-193): when the
-- lynx is found, Talk(SAY_MERGE), NON_ATTACKABLE flag, MoveFollow
-- both ways, ++TransformCount. The flag leg is ported (SetFlag twin,
-- zuljin precedent); the MoveFollow walk is approximated by the 1s
-- proximity poll plus the 2s timer fallback (no movement bridge),
-- and Talk stays unconditional so a missing
-- lynx (failed summon) cannot soft-lock the merge -- C++ would stall
-- in MERGE with no lynx, while the timer always completes here.
-- TransformCount still increments in onMergeComplete (see there).
local function startMerge(creature, guid)
    local st = halazziState(guid)
    st.phase = PHASE_MERGE
    cancelPhaseTimers(guid)
    creature:Talk(SAY_MERGE)
    local lynx = creature:FindNearestCreature(ENTRY_SPIRIT_LYNX, 100.0)
    if lynx then
        lynx:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
    end
    schedule(timers, guid, "mergewait", 2000, function()
        onMergeComplete(creature, guid)
    end)
    schedule(timers, guid, "mergepoll", 1000, function()
        onMergePoll(creature, guid)
    end)
end

-- C++ 1s check timer: lynx phase -> split below 25%*(3-TransformCount);
-- human phase -> merge below 20% boss health, or below 20% lynx health
-- when the lynx exists (boss_halazzi.cpp:281-282, second arm).
local function onCheck(creature, guid)
    local st = halazziState(guid)
    local health, maxHealth = creature:GetHealth(), creature:GetMaxHealth()
    if st.phase == PHASE_LYNX and maxHealth > 0 then
        if health < math.floor(maxHealth * (25 * (3 - st.transformCount)) / 100) then
            startSplit(creature, guid)
        end
    elseif st.phase == PHASE_HUMAN and maxHealth > 0 then
        if health <= math.floor(maxHealth * 20 / 100) then  -- C++: !HealthAbovePct(20), fires at <=20%
            startMerge(creature, guid)
        else
            -- C++ human check, second arm (boss_halazzi.cpp:281-282):
            -- merge when the lynx exists and is not above 20% health.
            local lynx = creature:FindNearestCreature(ENTRY_SPIRIT_LYNX, 100.0)
            if lynx and lynx:GetHealthPct() <= 20 then
                startMerge(creature, guid)
            end
        end
    end
    if halazziState(guid).phase ~= PHASE_NONE then
        schedule(timers, guid, "check", 1000, function()
            onCheck(creature, guid)
        end)
    end
end

-- C++ EnterPhase: arm the timers for the new phase and reset the per-phase
-- health on lynx/enrage entry (C++: SetMaxHealth(600000) +
-- SetHealth(600000-150000*count), boss_halazzi.cpp:164-165).
enterPhase = function(creature, guid, nextPhase)
    local st = halazziState(guid)
    cancelPhaseTimers(guid)
    if nextPhase == PHASE_LYNX or nextPhase == PHASE_ENRAGE then
        if st.phase == PHASE_MERGE then
            creature:CastSpell(creature, SPELL_TRANSFORM_MERGE, true)
            -- me->Attack(victim, true) + MoveChase have no bridge; engine
            -- melee is continuous.
        end
        -- C++ EnterPhase LYNX/ENRAGE: lynx DisappearAndDie via
        -- LynxGUID (boss_halazzi.cpp:162-163); FindNearestCreature
        -- is the GUID-lookup twin, Despawn the removal twin.
        local lynx = creature:FindNearestCreature(ENTRY_SPIRIT_LYNX, 100.0)
        if lynx then
            lynx:Despawn()
        end
        creature:SetMaxHealth(600000)
        creature:SetHealth(600000 - 150000 * st.transformCount)
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
        -- C++ DoSpawnCreature(SPIRIT_LYNX, 5, 5, 0, 0,
        -- TEMPSUMMON_CORPSE_DESPAWN, 0s) — DoSpawnCreature adds offsets
        -- to the boss position (ScriptedCreature.cpp:286-289, janalai
        -- precedent); type 4, despawnMs 0 (corpse family starts its
        -- timer at death, unmodeled). JustSummoned AttackStart(victim)
        -- has no bridge, so the lynx idles until the engine aggros it
        -- (its npc_halazzi_lynx AI below is registered).
        creature:SummonCreature(ENTRY_SPIRIT_LYNX, creature:GetX() + 5,
            creature:GetY() + 5, creature:GetZ(), creature:GetO(), 4, 0)
        -- C++ EnterPhase(PHASE_HUMAN): SetMaxHealth(400000) +
        -- SetHealth(400000) — a full heal, ported C++-exact
        -- (boss_halazzi.cpp:178-179).
        st.phase = PHASE_HUMAN
        creature:SetMaxHealth(400000)
        creature:SetHealth(400000)
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
    -- Event 23 also fires on death (engine reset-then-died order,
    -- lua_creature_events.go) but the C++ AI Reset() never runs on death;
    -- without the gate the corpse would take a phantom dual-wield cast
    -- and a SetHealth from enterPhase.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_DUAL_WIELD, true)
        enterPhase(creature, guid, PHASE_LYNX)
    end
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
-- on the damage hook; the NON_ATTACKABLE AttackStart gate and the
-- merge MoveFollow arms have no bridge (the lynx is summoned by the
-- human-phase arm above, and any spawned 24143 fights correctly once
-- engaged). The merge-entry NON_ATTACKABLE flag and the lynx/enrage
-- entry DisappearAndDie are ported boss-side (see enterPhase).
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
