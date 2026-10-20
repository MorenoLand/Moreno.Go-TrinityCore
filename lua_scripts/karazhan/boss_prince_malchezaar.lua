-- Prince Malchezaar (Karazhan) — Lua port of
-- src/server/scripts/EasternKingdoms/Karazhan/boss_prince_malchezaar.cpp
-- (604 lines — boss_malchezaarAI + netherspite_infernalAI).
-- Creature entries: 15690 Prince Malchezaar (wowhead npc=15690/prince-
-- malchezaar; AddSC registers "boss_malchezaar"), 17646 Netherspite
-- Infernal, 17650 Malchezaar's Axes. Instance data: DATA_GO_NETHER_DOOR =
-- 23 (karazhan.h:51) — door toggles only; no Go instance-script model, so
-- those arms are skipped and the encounter is admission-only via the
-- luaBossAI shim in engine/world/boss_ai.go.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 9 OnDamageTaken (60% / 30% phase transitions, pre-damage
-- health like C++ HealthBelowPct), 23 OnReset. Timers via CreateLuaEvent;
-- melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady / DoMeleeAttacksIfReady.
-- Fight shape: phase 1 — Enfeeble 30s (up to 5 random non-tank players
-- dropped to 1 hp, restored 9s later), Shadow Word: Pain 20s on the
-- victim, Shadow Nova 35.5s on the victim, Infernals 40s; phase 2 (below
-- 60%) — Sunder Armor {10s,18s} and Cleave {6s,12s} on the victim, Shadow
-- Word: Pain stops; phase 3 (below 30%) — axes fly out, Amplify Damage
-- {20s,30s} on a random target, Shadow Nova 31s, Infernals every 14.5s.
-- Deviations from C++: no Go summon model, so the infernal summons
-- (18 fixed INFERNAL_Z points on map 532), the two axe summons (17650),
-- the SPELL_INFERNAL_RELAY handshake, and the whole netherspite_infernal
-- AI (Hellfire 4s / Cleanup 170s / DamageTaken non-Malchezaar immunity /
-- Cleanup point recycling) have no bearer — Infernals stay virtual and the
-- InfernalTimer arm keeps the C++ cadence as Talk(SAY_SUMMON) only; the
-- axes are never spawned, so the phase-3 AxesTargetSwitchTimer threat
-- rewiring (no threat model) is skipped entirely. No instance-script
-- model, so the DATA_GO_NETHER_DOOR toggles on Reset/JustEngagedWith/
-- JustDied are skipped. The phase-2 equipment swap is ported via
-- creature:SetEquipmentSlots (33542,33542,0xFFFFFFFF at the phase-2
-- transition; 0,0,0xFFFFFFFF at phase 3 = C++ ClearWeapons) alongside the
-- visual SPELL_EQUIP_AXES (30857); only SetCanDualWield and the OFF_ATTACK
-- 1.5x attack-time change have no bridge. The phase-2 thrash aura (12787)
-- is removed at the phase-3 transition via creature:RemoveAura (bridged).
-- The UpdateAI stun-gate (UNIT_STATE_STUNNED during the phase-2 shift) and
-- InterruptNonMeleeSpells have no UNIT_STATE model — timers fire
-- unconditionally, the standing convention for these ports. The
-- InfernalCleanupTimer (47s) is initialized in C++ Initialize but has no
-- arm in UpdateAI — no Lua timer, C++-exact. Enfeeble's tank exclusion
-- uses GetVictim (no threat list); Enfeeble itself is threat-list-based
-- with no range cap, while Amplify Damage and phase-3 SW:Pain use the
-- 100 yd SelectTarget cap (creature:GetDistance). The phase-3
-- Shadow Nova clamp (re-arm to EnfeebleTimer+5s when >35s out) is modeled
-- with second-resolution deadline stamps recorded at arm time plus a
-- shadowArmed latch mirroring C++'s uint32(-1) disabled state: a nova that
-- already fired in phase 1/2 (not armed) always takes the clamp, as in
-- C++; an armed timer is clamped only when > 35s remain. The phase-2
-- SWPain freeze (C++ neither fires nor decrements the timer in phase 2,
-- preserving the leftover into phase 3) is modeled exactly via the same
-- deadline stamps: the in-flight timer fires once as a no-op in phase 2
-- and the preserved leftover is re-armed at phase-3 entry.

local ENTRY_MALCHEZAAR = 15690

local SAY_AGGRO, SAY_AXE_TOSS1, SAY_AXE_TOSS2 = 0, 1, 2
local SAY_SLAY, SAY_SUMMON, SAY_DEATH = 6, 7, 8

local SPELL_ENFEEBLE = 30843
local SPELL_SHADOWNOVA = 30852
local SPELL_SW_PAIN = 30854
local SPELL_THRASH_AURA = 12787
local SPELL_SUNDER_ARMOR = 30901
local SPELL_EQUIP_AXES = 30857
local SPELL_AMPLIFY_DAMAGE = 39095
local SPELL_CLEAVE = 30131

local PHASE_ONE, PHASE_TWO, PHASE_THREE = 1, 2, 3
local ENFEEBLE_MAX_TARGETS = 5

local timers = {}
local malchezaarState = {}

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

local function cancelOne(guid, key)
    local per = timers[guid]
    if per and per[key] then
        RemoveEventById(per[key])
        per[key] = nil
    end
end

local function alivePlayers(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local out = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            out[#out + 1] = p
        end
    end
    return out
end

local function randomAlivePlayer(creature, excludeVictim, maxDist)
    local victimGuid = nil
    if excludeVictim then
        local victim = creature:GetVictim()
        victimGuid = victim ~= nil and victim:GetGUID() or nil
    end
    local candidates = alivePlayers(creature)
    local out = {}
    for _, p in ipairs(candidates) do
        if p:GetGUID() ~= victimGuid
                and (maxDist == nil or creature:GetDistance(p) <= maxDist) then
            out[#out + 1] = p
        end
    end
    if #out == 0 then
        return nil
    end
    return out[math.random(#out)]
end

local function malchezaarInitState(guid)
    -- C++ ctor / Initialize: phase = 1, enfeeble slots cleared.
    malchezaarState[guid] = {
        phase = PHASE_ONE,
        enfeebleTargets = {},
        enfeebleDeadlineSec = 0,
        shadowNovaDeadlineSec = 0,
        swpainDeadlineSec = 0,
        shadowArmed = false,
    }
end

-- Forward declarations: onShadowNova / onSWPain reference the arming
-- helpers that are assigned below (cyclic chain; gal_darah idiom).
local armShadowNova, armSWPain
local onSWPain

local function onShadowNova(creature, guid)
    local st = malchezaarState[guid]
    if st == nil then
        return
    end
    -- C++ DoCastVictim — nil falls back to the current victim.
    creature:CastSpell(nil, SPELL_SHADOWNOVA)
    -- C++: ShadowNovaTimer = phase == 3 ? 31000 : uint32(-1) (disabled).
    st.shadowArmed = false
    if st.phase == PHASE_THREE then
        armShadowNova(creature, guid, 31000)
    end
end

armShadowNova = function(creature, guid, delay)
    local st = malchezaarState[guid]
    if st == nil then
        return
    end
    cancelOne(guid, "shadow")
    -- Deadline in fractional seconds: the C++ 35000 ms clamp threshold
    -- needs sub-second fidelity (35500 ms must compare > 35).
    st.shadowNovaDeadlineSec = os.time() + delay / 1000
    st.shadowArmed = true
    schedule(guid, "shadow", delay, function() onShadowNova(creature, guid) end)
end

-- C++ UpdateAI every-tick arm (boss_prince_malchezaar.cpp:404-405): after
-- the EnfeebleReset branch and the UNIT_STATE_STUNNED early-return,
-- before the phase arms —
--   if (me->GetVictim() && me->GetTarget() != me->EnsureVictim()->GetGUID())
--       me->SetTarget(me->EnsureVictim()->GetGUID());
-- The visual target re-syncs to the victim whenever they differ. Modeled
-- as a 1s self-re-arming timer (the file's UpdateAI-timer idiom); the
-- stun-gate is not modeled — timers fire unconditionally, the standing
-- convention for these ports (documented above).
local function onTargetSync(creature, guid)
    local victim = creature:GetVictim()
    if victim ~= nil then
        local victimGuid = victim:GetGUID()
        if creature:GetTarget() ~= victimGuid then
            creature:SetTarget(victimGuid)
        end
    end
    schedule(guid, "targetsync", 1000, function() onTargetSync(creature, guid) end)
end

local function onEnfeebleReset(creature, guid)
    local st = malchezaarState[guid]
    if st == nil then
        return
    end
    -- C++ EnfeebleResetHealth: restore recorded health on alive targets,
    -- then clear the slots (Initialize runs on the next Reset anyway).
    for _, slot in ipairs(st.enfeebleTargets) do
        local target = slot.player
        if target ~= nil and not target:IsDead() then
            target:SetHealth(slot.health)
        end
    end
    st.enfeebleTargets = {}
end

local function onEnfeeble(creature, guid)
    local st = malchezaarState[guid]
    if st == nil or st.phase == PHASE_THREE then
        return
    end
    -- C++ EnfeebleHealthEffect: threat-list players except the tank; the
    -- tank is identified via GetVictim since there is no threat model.
    local victim = creature:GetVictim()
    local victimGuid = victim ~= nil and victim:GetGUID() or nil
    local candidates = {}
    for _, p in ipairs(alivePlayers(creature)) do
        if p:GetGUID() ~= victimGuid then
            candidates[#candidates + 1] = p
        end
    end
    -- C++ trims to 5 by erasing random entries.
    while #candidates > ENFEEBLE_MAX_TARGETS do
        table.remove(candidates, math.random(#candidates))
    end
    st.enfeebleTargets = {}
    for _, target in ipairs(candidates) do
        st.enfeebleTargets[#st.enfeebleTargets + 1] = {
            player = target,
            health = target:GetHealth(),
        }
        -- C++ TRIGGERED_FULL_MASK with OriginalCaster = me.
        creature:CastSpell(target, SPELL_ENFEEBLE, true)
        target:SetHealth(1)
    end
    st.enfeebleDeadlineSec = os.time() + 30
    schedule(guid, "enfeeble", 30000, function() onEnfeeble(creature, guid) end)
    -- C++: ShadowNovaTimer = 5000, EnfeebleResetTimer = 9000.
    armShadowNova(creature, guid, 5000)
    schedule(guid, "enfeeblereset", 9000, function() onEnfeebleReset(creature, guid) end)
end

armSWPain = function(creature, guid, delay)
    local st = malchezaarState[guid]
    if st == nil then
        return
    end
    cancelOne(guid, "swpain")
    st.swpainDeadlineSec = os.time() + delay / 1000
    schedule(guid, "swpain", delay, function() onSWPain(creature, guid) end)
end

onSWPain = function(creature, guid)
    local st = malchezaarState[guid]
    if st == nil or st.phase == PHASE_TWO then
        return
    end
    if st.phase == PHASE_ONE then
        -- C++ DoCastVictim — the tank, nil falls back to current victim.
        creature:CastSpell(nil, SPELL_SW_PAIN)
    else
        -- C++ SelectTarget(Random, 1, 100, true): random non-tank within
        -- 100 yd. C++ guards if (target) and skips the cast with no second
        -- target, so a nil pick must NOT fall back to the victim (the tank).
        local target = randomAlivePlayer(creature, true, 100)
        if target then
            creature:CastSpell(target, SPELL_SW_PAIN)
        end
    end
    armSWPain(creature, guid, 20000)
end

local function onSunderArmor(creature, guid)
    local st = malchezaarState[guid]
    if st == nil or st.phase ~= PHASE_TWO then
        return
    end
    creature:CastSpell(nil, SPELL_SUNDER_ARMOR)
    schedule(guid, "sunder", {10000, 18000}, function() onSunderArmor(creature, guid) end)
end

local function onCleave(creature, guid)
    local st = malchezaarState[guid]
    if st == nil or st.phase ~= PHASE_TWO then
        return
    end
    creature:CastSpell(nil, SPELL_CLEAVE)
    schedule(guid, "cleave", {6000, 12000}, function() onCleave(creature, guid) end)
end

local function onAmplifyDamage(creature, guid)
    local st = malchezaarState[guid]
    if st == nil or st.phase ~= PHASE_THREE then
        return
    end
    -- C++ SelectTarget(Random, 0, 100, true): tank included, 100 yd cap.
    local target = randomAlivePlayer(creature, false, 100)
    if target ~= nil then
        creature:CastSpell(target, SPELL_AMPLIFY_DAMAGE)
    end
    schedule(guid, "amplify", {20000, 30000}, function() onAmplifyDamage(creature, guid) end)
end

local function onInfernal(creature, guid)
    local st = malchezaarState[guid]
    if st == nil then
        return
    end
    -- C++ SummonInfernal: no Go summon model, so the virtual infernal is
    -- just the SAY_SUMMON announcement; hellfire/relay/display arms have
    -- no bearer. C++ repeats 14.5s in phase 3, 44.5s otherwise.
    creature:Talk(SAY_SUMMON)
    local delay = 44500
    if st.phase == PHASE_THREE then
        delay = 14500
    end
    schedule(guid, "infernal", delay, function() onInfernal(creature, guid) end)
end

local function startPhaseTwo(creature, guid)
    local st = malchezaarState[guid]
    if st == nil or st.phase ~= PHASE_ONE then
        return
    end
    -- C++ UpdateAI phase-1 arm: InterruptNonMeleeSpells (no bridge),
    -- DoCast SPELL_EQUIP_AXES, Talk(SAY_AXE_TOSS1), thrash aura triggered,
    -- SetEquipmentSlots(false, EQUIP_ID_AXE, EQUIP_ID_AXE, EQUIP_NO_CHANGE)
    -- (bridged, ported below); SetCanDualWield and the OFF_ATTACK 1.5x
    -- attack-time change have no bridge.
    st.phase = PHASE_TWO
    -- C++ arm order: DoCast(EQUIP_AXES) -> Talk(SAY_AXE_TOSS1) ->
    -- DoCast(THRASH_AURA, triggered) (mandokir cast-then-yell precedent).
    creature:CastSpell(creature, SPELL_EQUIP_AXES)
    creature:Talk(SAY_AXE_TOSS1)
    creature:CastSpell(creature, SPELL_THRASH_AURA, true)
    -- C++ SetEquipmentSlots(false, EQUIP_ID_AXE, EQUIP_ID_AXE,
    -- EQUIP_NO_CHANGE) = (33542, 33542, 0xFFFFFFFF).
    creature:SetEquipmentSlots(33542, 33542, 0xFFFFFFFF)
    schedule(guid, "sunder", {5000, 10000}, function() onSunderArmor(creature, guid) end)
    -- C++ Cleave_Timer = 8000 in Initialize, decremented only in the
    -- phase-2 arm, so the first cleave lands 8s after the transition.
    schedule(guid, "cleave", 8000, function() onCleave(creature, guid) end)
end

local function startPhaseThree(creature, guid)
    local st = malchezaarState[guid]
    if st == nil or st.phase ~= PHASE_TWO then
        return
    end
    -- C++ UpdateAI phase-2 arm: InfernalTimer = 15000, ClearWeapons /
    -- RemoveAurasDueToSpell(thrash) (both bridged, ported below),
    -- Talk(SAY_AXE_TOSS2), two axe summons with 10M threat (no summon or
    -- threat model — the AxesTargetSwitchTimer re-threating is skipped
    -- entirely).
    st.phase = PHASE_THREE
    -- C++ ClearWeapons: SetEquipmentSlots(false, EQUIP_UNEQUIP,
    -- EQUIP_UNEQUIP, EQUIP_NO_CHANGE) = (0, 0, 0xFFFFFFFF); the
    -- SetCanDualWield(false) leg has no bridge.
    creature:SetEquipmentSlots(0, 0, 0xFFFFFFFF)
    creature:RemoveAura(SPELL_THRASH_AURA)
    creature:Talk(SAY_AXE_TOSS2)
    cancelOne(guid, "sunder")
    cancelOne(guid, "cleave")
    cancelOne(guid, "swpain")
    cancelOne(guid, "infernal")
    schedule(guid, "infernal", 15000, function() onInfernal(creature, guid) end)
    -- C++: if (ShadowNovaTimer > 35000) ShadowNovaTimer = EnfeebleTimer +
    -- 5000. ShadowNovaTimer is uint32(-1) (unreachable) once the nova fired
    -- in phase 1/2 without re-arm, so the clamp always applies when no
    -- timer is armed; an armed timer is clamped only with > 35s left.
    local enfeebleRemaining = st.enfeebleDeadlineSec - os.time()
    if enfeebleRemaining < 0 then
        enfeebleRemaining = 0
    end
    if not st.shadowArmed or st.shadowNovaDeadlineSec - os.time() > 35 then
        armShadowNova(creature, guid, (enfeebleRemaining + 5) * 1000)
    end
    -- AmplifyDamage starts at 5000 on phase-3 entry (C++ Initialize).
    schedule(guid, "amplify", 5000, function() onAmplifyDamage(creature, guid) end)
    -- Phase-3 SWPain targets a random player, not the victim; C++ neither
    -- fires nor decrements SWPainTimer in phase 2, so the phase-1 leftover
    -- is preserved here.
    local swpainRemaining = st.swpainDeadlineSec - os.time()
    if swpainRemaining <= 0 then
        swpainRemaining = 1
    end
    armSWPain(creature, guid, swpainRemaining * 1000)
end

local function malchezaarEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    malchezaarInitState(guid)
    -- C++ JustEngagedWith: Talk(SAY_AGGRO) + door close (no instance model).
    creature:Talk(SAY_AGGRO)
    local st = malchezaarState[guid]
    st.enfeebleDeadlineSec = os.time() + 30
    schedule(guid, "enfeeble", 30000, function() onEnfeeble(creature, guid) end)
    armShadowNova(creature, guid, 35500)
    armSWPain(creature, guid, 20000)
    schedule(guid, "infernal", 40000, function() onInfernal(creature, guid) end)
    schedule(guid, "targetsync", 1000, function() onTargetSync(creature, guid) end)
end

local function malchezaarLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    malchezaarInitState(guid)
end

local function malchezaarTargetDied(event, creature, victim)
    creature:Talk(SAY_SLAY)
end

local function malchezaarDied(event, creature, killer)
    -- C++ JustDied: Talk(SAY_DEATH), AxesCleanup / ClearWeapons /
    -- InfernalCleanup / positions reset (nothing to clean with no summon
    -- model), door open (no instance model).
    creature:Talk(SAY_DEATH)
    cancelTimers(creature:GetGUID())
    malchezaarInitState(creature:GetGUID())
end

local function malchezaarReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    malchezaarInitState(guid)
end

local function malchezaarDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local st = malchezaarState[guid]
    if st == nil then
        malchezaarInitState(guid)
        st = malchezaarState[guid]
    end
    local health, maxHealth = creature:GetHealth(), creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    -- C++ UpdateAI checks HealthBelowPct on the post-damage health; the
    -- event-9 hook is pre-damage, so project (health - damage) (zum_rah
    -- precedent). HealthBelowPct(pct) is strict < trunc(max*pct/100)
    -- (Unit.h:872-875); the integer floor form matches CalculatePct.
    -- C++ is UpdateAI post-damage, so phases never flip on the killing
    -- blow; the postDamage > 0 guard (nefarian rule) keeps the port from
    -- phasing while dying.
    local postDamage = math.max(health - damage, 0)
    if postDamage > 0 and st.phase == PHASE_ONE and postDamage < math.floor(maxHealth * 60 / 100) then
        startPhaseTwo(creature, guid)
    elseif postDamage > 0 and st.phase == PHASE_TWO and postDamage < math.floor(maxHealth * 30 / 100) then
        startPhaseThree(creature, guid)
    end
    return false, damage
end

RegisterCreatureEvent(ENTRY_MALCHEZAAR, 1, malchezaarEnterCombat)
RegisterCreatureEvent(ENTRY_MALCHEZAAR, 2, malchezaarLeaveCombat)
RegisterCreatureEvent(ENTRY_MALCHEZAAR, 3, malchezaarTargetDied)
RegisterCreatureEvent(ENTRY_MALCHEZAAR, 4, malchezaarDied)
RegisterCreatureEvent(ENTRY_MALCHEZAAR, 9, malchezaarDamageTaken)
RegisterCreatureEvent(ENTRY_MALCHEZAAR, 23, malchezaarReset)
