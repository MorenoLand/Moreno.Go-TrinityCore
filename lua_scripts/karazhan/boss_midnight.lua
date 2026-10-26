-- Attumen the Huntsman / Midnight (Karazhan) — Lua port of
-- src/server/scripts/EasternKingdoms/Karazhan/boss_midnight.cpp
-- Creature entries: 16151 (Midnight, TDB creature_template ScriptName
-- "boss_midnight"); 15550 (Attumen the Huntsman, unmounted) and 16152
-- (Attumen the Huntsman, mounted), both ScriptName "boss_attumen" per the
-- AddSC_boss_attumen registration. Instance data: DATA_ATTUMEN = 0
-- (karazhan.h:30); the encounter never reads/writes instance state beyond
-- the BossAI admission handled by the luaBossAI shim in
-- engine/world/boss_ai.go.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 9 OnDamageTaken (returns false, newDamage for the never-die
-- clamps), 14 OnHitBySpell (the SPELL_MOUNT merge arm), 23 OnReset.
-- Timers via CreateLuaEvent; melee is engine-driven in Go (creature combat
-- tick), like C++ DoMeleeAttackIfReady.
-- Deviations from C++: event 19 (JustSummoned) DOES fire on the summoner via the shared
-- summonTempCreature funnel, so the summon->AI()->Talk(SAY_APPEAR) greeting
-- is relayed in midnightJustSummoned below; event 22 (IsSummonedBy) fires
-- only from the pet funnel, so the summoner-entry phase assignments stay
-- approximated in OnEnterCombat keyed on entry. The SetGUID wiring stays
-- out (no bridge); the AttackStart(victim) leg IS ported in
-- midnightJustSummoned via summon:EngageWithTarget(victim). The 29714 cast's spell-effect
-- summon spawns Attumen when the DBC props pass; if it does not, no
-- Attumen exists to relay. Approximation: the
-- phase that C++ IsSummonedBy would set before combat (ATTUMEN_ENGAGES for
-- 15550, MOUNTED for 16152) is set in OnEnterCombat keyed on entry, which
-- is where C++ ScheduleTasks/JustEngagedWith also runs. The 25% mount arm
-- (Midnight's DamageTaken casting SPELL_MOUNT triggered) fires from the
-- Midnight damage hook; Attumen's SpellHit(SPELL_MOUNT) arm cancels its
-- timers and Talk(SAY_MOUNT), but the follow step stays out (no MoveFollow
-- bridge); the visibility + mounted-summon steps are ported via the
-- midnight-side 1s proximity poll below (Go self-casts hit only the caster
-- plus chain jumps, so SpellHit delivery of MOUNT to Attumen is unproven
-- and the poll is driven from midnight's 25% arm, not SpellHit). No
-- MECHANIC_DISARM info reaches the Lua spell-hit hook, so the
-- SAY_DISARMED arm is skipped. Midnight's directed Talk(SAY_MIDNIGHT_KILL,
-- attumenGUID) — Talk(textId[, target]) is bridged (lua_creature_events.go:102),
-- but there is no handle to the attumen creature from the midnight kill arm,
-- so it broadcasts untargeted. Attumen's JustDied Talk(SAY_DEATH) is kept; midnight->KillSelf
-- stays out on the SetGUID midnight-handle binding (creature:Kill() IS bridged,
-- lua_creature_events.go:1027 — the handle, not the kill, is missing). The EMOTE_MOUNT_UP mount-emote arm is PORTED in
-- attumenSpellHit below (2026-10-21): the stale "no GUID->creature
-- bridge" claim is retracted — event 14 passes the midnight creature
-- as the caster arg. EnterEvadeMode despawns now
-- has no bridge. EnterEvadeMode despawns now ported: midnight's
-- _DespawnAtEvade(Seconds(10)) as a 10s event-2 schedule (jedoga
-- precedent, re-engage cancels); attumen's me->DespawnOrUnsummon()
-- immediate. Stale "no despawn bridge" retracted; the
-- _DespawnAtEvade(10s, midnight) cross-GUID leg stays blocked (zero
-- GUID->creature lookup bridges on the Lua surface).
-- SPELL_CHARGE / SPELL_INTANGIBLE_PRESENCE with no candidate target are
-- skipped (the bridge would fall back to the victim where C++ DoCast(null)
-- is a no-op).

local ENTRY_MIDNIGHT = 16151
local ENTRY_ATTUMEN = 15550
local ENTRY_ATTUMEN_MOUNTED = 16152

-- UnitDefines.h react states (engine/world/lua_creature_events.go SetReactState bridge).
local REACT_DEFENSIVE = 1
local REACT_PASSIVE = 0

local SAY_KILL, SAY_RANDOM, SAY_DISARMED = 0, 1, 2
local SAY_MIDNIGHT_KILL, SAY_DEATH, SAY_APPEAR, SAY_MOUNT = 3, 3, 4, 5
local EMOTE_CALL_ATTUMEN, EMOTE_MOUNT_UP = 0, 1

local SPELL_SHADOWCLEAVE = 29832
local SPELL_INTANGIBLE_PRESENCE = 29833
local SPELL_SPAWN_SMOKE = 10389
local SPELL_CHARGE = 29847
local SPELL_KNOCKDOWN = 29711
local SPELL_SUMMON_ATTUMEN = 29714
local SPELL_SUMMON_ATTUMEN_MOUNTED = 29799
local SPELL_MOUNT = 29770

local PHASE_NONE, PHASE_ATTUMEN_ENGAGES, PHASE_MOUNTED = 0, 1, 2

local timers = {}
local midnightState = {}
local attumenState = {}

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

-- Threat-list target pools: boss_midnight.cpp:154-160,189-215. The charge
-- arm iterates GetThreatManager().GetUnsortedThreatList() raw (no offline
-- skip, dead victims in the 8-25 band stay candidates), so
-- randomTargetInBand keeps no dead filter. The intangible-presence arm is
-- SelectTarget(Random, 0) (:112): SelectTargetList skips offline refs
-- (UnitAI.h) and dead victims go offline via ShouldBeOffline /
-- _IsTargetAcceptable (Creature.cpp:2508, UNIT_STATE_DIED), so
-- randomTargetAnywhere filters dead entries. GetAITargets
-- (lua_creature_events.go:345) is the bridged threat list; GetDistance2d
-- resolves via the generic fallback (engine/scripting/object.go:158).
local function randomTargetInBand(creature, minDist, maxDist)
    -- C++ IsWithinDist(me, d, false) compares raw 2D dist against
    -- d + own reach + target reach (Object.cpp _IsWithinDist); Go's
    -- GetDistance2d is raw, so subtract the creature's own reach
    -- plus the target's (1.5 for players, else target:GetCombatReach()).
    local reach = creature:GetCombatReach()
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        local targetReach = 1.5
        if t:GetObjectType() ~= "Player" then
            targetReach = t:GetCombatReach()
        end
        local dist = creature:GetDistance2d(t) - reach - targetReach
        if dist >= minDist and dist < maxDist then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

local function randomTargetAnywhere(creature)
    local targets = creature:GetAITargets()
    local alive = {}
    for _, t in ipairs(targets) do
        if not t:IsDead() then
            alive[#alive + 1] = t
        end
    end
    if #alive == 0 then
        return nil
    end
    return alive[math.random(#alive)]
end

-- Midnight (16151) ------------------------------------------------------------

local function midnightInitState(guid)
    midnightState[guid] = { phase = PHASE_NONE }
end

local function onMidnightKnockdown(creature, guid)
    -- C++ boss_midnightAI::UpdateAI returns early (scheduler never runs) while
    -- _phase == PHASE_MOUNTED: the event stays scheduled but never fires.
    local state = midnightState[guid]
    if state ~= nil and state.phase ~= PHASE_MOUNTED then
        local victim = creature:GetVictim()
        if victim ~= nil then
            creature:CastSpell(victim, SPELL_KNOCKDOWN)
        end
    end
    schedule(guid, "knockdown", {15000, 25000}, function() onMidnightKnockdown(creature, guid) end)
end

local function midnightEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    midnightInitState(guid)
    schedule(guid, "knockdown", {15000, 25000}, function() onMidnightKnockdown(creature, guid) end)
end

-- C++ boss_attumenAI::SpellHit(SPELL_MOUNT) 1s proximity poll
-- (boss_midnight.cpp): if within 5yd 2d + Attumen's reach of midnight, DoCastAOE
-- (SPELL_SUMMON_ATTUMEN_MOUNTED) + both SetVisible(false); else re-follow
-- + repeat. Driven here from midnight's 25% arm because Go self-casts hit
-- only the caster (no AoE fan-out), so SpellHit delivery to Attumen is
-- unproven. The mounted summon is a direct SummonCreature (no 29799 cast,
-- so no double spawn); the health merge + DoZoneInCombat twin the attumen
-- JustSummoned arm (C++: midnight's health when higher, Attumen's otherwise). Attumen's own timers cannot be
-- cancelled cross-creature (documented deviation: the invisible unmounted
-- form's casts may still fire). Cancelled with the midnight timers on
-- LeaveCombat/Reset.
local function onMountPoll(creature, guid)
    local attumen = creature:FindNearestCreature(ENTRY_ATTUMEN, 200)
    if attumen ~= nil and creature:GetDistance2d(attumen) - attumen:GetCombatReach() < 5.0 then -- IsWithinDist2d adds Attumen's reach (Object.cpp:1228)
        local mounted = creature:SummonCreature(ENTRY_ATTUMEN_MOUNTED,
            attumen:GetX(), attumen:GetY(), attumen:GetZ(), 0)
        if mounted ~= nil then
            local mh, ah = creature:GetHealth(), attumen:GetHealth()
            if mh > ah then
                mounted:SetHealth(mh)
            else
                mounted:SetHealth(ah)
            end
            mounted:DoZoneInCombat()
            attumen:SetVisible(false)
            creature:SetVisible(false)
        end
    else
        schedule(guid, "mountpoll", 1000, function() onMountPoll(creature, guid) end)
    end
end

-- C++ boss_midnightAI::JustSummoned (boss_midnight.cpp:327-338): summon
-- entry 15550 -> store GUID, SetGUID(me->GetGUID(), NPC_MIDNIGHT),
-- AttackStart(me->GetVictim()), Talk(SAY_APPEAR). The AttackStart leg IS
-- ported via summon:EngageWithTarget(victim) (2026-10-20 bridge; the
-- funnel is player-target-only, so a non-player victim no-ops). The
-- GUID store stays dead — KilledUnit's Talk target needs a GUID->unit
-- handle, which has no bridge — and SetGUID stays out (no bridge).
local function midnightJustSummoned(event, creature, summon)
    if summon:GetEntry() == ENTRY_ATTUMEN then
        local victim = creature:GetVictim()
        if victim ~= nil then
            summon:EngageWithTarget(victim)
        end
        summon:Talk(SAY_APPEAR)
    end
end

local function midnightDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local state = midnightState[guid]
    if state == nil then
        state = { phase = PHASE_NONE }
        midnightState[guid] = state
    end
    local health, maxHealth = creature:GetHealth(), creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    local newDamage = damage
    -- Midnight never dies: C++ clamps unconditionally (damage >= health ->
    -- health - 1), pinning it at 1 HP forever. health > 0 guards the hook
    -- firing on an already-dead creature only.
    if health > 0 and damage >= health then
        newDamage = health - 1
    end
    local postHealth = health - newDamage
    if state.phase == PHASE_NONE and postHealth < math.floor(maxHealth * 95 / 100) then
        state.phase = PHASE_ATTUMEN_ENGAGES
        creature:Talk(EMOTE_CALL_ATTUMEN)
        creature:CastSpell(creature, SPELL_SUMMON_ATTUMEN)
    elseif state.phase == PHASE_ATTUMEN_ENGAGES and postHealth < math.floor(maxHealth * 25 / 100) then
        state.phase = PHASE_MOUNTED
        creature:CastSpell(creature, SPELL_MOUNT, true)
        schedule(guid, "mountpoll", 1000, function() onMountPoll(creature, guid) end)
    end
    return false, newDamage
end

local function midnightTargetDied(event, creature, victim)
    local state = midnightState[creature:GetGUID()]
    if state ~= nil and state.phase == PHASE_ATTUMEN_ENGAGES then
        creature:Talk(SAY_MIDNIGHT_KILL)
    end
end

local function midnightLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    midnightState[guid] = nil
    -- C++ boss_midnightAI::EnterEvadeMode: _DespawnAtEvade(Seconds(10))
    -- (boss_midnight.cpp:351-354); the 10s is the respawn delay, the
    -- despawn itself is immediate (ScriptedCreature.cpp:616).
    creature:Despawn()
end

local function midnightReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    midnightInitState(guid)
    -- C++ boss_midnightAI::Reset (boss_midnight.cpp:300-306): SetVisible(true)
    -- (creature:SetVisible bridge, Unit::SetVisible Unit.cpp:8656) +
    -- SetReactState(REACT_DEFENSIVE) (SetReactState bridge), in C++ order.
    creature:SetVisible(true)
    creature:SetReactState(REACT_DEFENSIVE)
end

RegisterCreatureEvent(ENTRY_MIDNIGHT, 1, midnightEnterCombat)
RegisterCreatureEvent(ENTRY_MIDNIGHT, 2, midnightLeaveCombat)
RegisterCreatureEvent(ENTRY_MIDNIGHT, 3, midnightTargetDied)
RegisterCreatureEvent(ENTRY_MIDNIGHT, 9, midnightDamageTaken)
RegisterCreatureEvent(ENTRY_MIDNIGHT, 23, midnightReset)
RegisterCreatureEvent(ENTRY_MIDNIGHT, 19, midnightJustSummoned)

-- Attumen (15550 unmounted / 16152 mounted) -----------------------------------

local function attumenInitState(guid)
    attumenState[guid] = { phase = PHASE_NONE }
end

local function onShadowCleave(creature, guid)
    local victim = creature:GetVictim()
    if victim ~= nil then
        creature:CastSpell(victim, SPELL_SHADOWCLEAVE)
    end
    schedule(guid, "shadowcleave", {15000, 25000}, function() onShadowCleave(creature, guid) end)
end

local function onIntangiblePresence(creature, guid)
    local target = randomTargetAnywhere(creature)
    if target ~= nil then
        creature:CastSpell(target, SPELL_INTANGIBLE_PRESENCE)
    end
    schedule(guid, "intangible", {25000, 45000}, function() onIntangiblePresence(creature, guid) end)
end

local function onRandomSay(creature, guid)
    creature:Talk(SAY_RANDOM)
    schedule(guid, "randomsay", {30000, 60000}, function() onRandomSay(creature, guid) end)
end

local function onCharge(creature, guid)
    local target = randomTargetInBand(creature, 8.0, 25.0)
    if target ~= nil then
        creature:CastSpell(target, SPELL_CHARGE)
    end
    schedule(guid, "charge", {10000, 25000}, function() onCharge(creature, guid) end)
end

local function onMountedKnockdown(creature, guid)
    local victim = creature:GetVictim()
    if victim ~= nil then
        creature:CastSpell(victim, SPELL_KNOCKDOWN)
    end
    schedule(guid, "knockdown", {25000, 35000}, function() onMountedKnockdown(creature, guid) end)
end

local function attumenEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    local state = { phase = PHASE_NONE }
    attumenState[guid] = state
    if creature:GetEntry() == ENTRY_ATTUMEN_MOUNTED then
        -- C++ IsSummonedBy(summoner 15550) sets PHASE_MOUNTED and casts
        -- the smoke; the event-22 hook never fires in the engine, so the
        -- phase and smoke are armed here on entry.
        state.phase = PHASE_MOUNTED
        creature:CastSpell(creature, SPELL_SPAWN_SMOKE)
        -- C++ BossAI::JustEngagedWith unconditionally runs ScheduleTasks(),
        -- so the mounted form keeps the shadowcleave / intangible-presence /
        -- SAY_RANDOM arms; the charge + knockdown legs come from
        -- IsSummonedBy(summoner 15550).
        schedule(guid, "shadowcleave", {15000, 25000}, function() onShadowCleave(creature, guid) end)
        schedule(guid, "intangible", {25000, 45000}, function() onIntangiblePresence(creature, guid) end)
        schedule(guid, "randomsay", {30000, 60000}, function() onRandomSay(creature, guid) end)
        schedule(guid, "charge", {10000, 25000}, function() onCharge(creature, guid) end)
        schedule(guid, "knockdown", {25000, 35000}, function() onMountedKnockdown(creature, guid) end)
    else
        -- C++ IsSummonedBy(summoner 16151) sets PHASE_ATTUMEN_ENGAGES
        -- before the unmounted Attumen first fights; approximated here.
        state.phase = PHASE_ATTUMEN_ENGAGES
        schedule(guid, "shadowcleave", {15000, 25000}, function() onShadowCleave(creature, guid) end)
        schedule(guid, "intangible", {25000, 45000}, function() onIntangiblePresence(creature, guid) end)
        schedule(guid, "randomsay", {30000, 60000}, function() onRandomSay(creature, guid) end)
    end
end

local function attumenDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local state = attumenState[guid]
    if state == nil then
        state = { phase = PHASE_NONE }
        attumenState[guid] = state
    end
    local health, maxHealth = creature:GetHealth(), creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    local newDamage = damage
    -- Attumen does not die until he mounts Midnight: C++ clamps unconditionally
    -- (damage >= health -> health - 1) whenever phase != MOUNTED, pinning him
    -- at 1 HP forever.
    if state.phase ~= PHASE_MOUNTED and health > 0 and damage >= health then
        newDamage = health - 1
    end
    local postHealth = health - newDamage
    -- C++ asks the midnight creature to cast SPELL_MOUNT (triggered) here;
    -- the cross-creature GUID wiring (SetGUID/NPC_MIDNIGHT) has no Lua
    -- bridge, so the mount cast is skipped — the mounted form's arms are
    -- covered by the 16152 entry when it exists in the world, and midnight's
    -- own 25% arm drives the mount poll below.
    if state.phase == PHASE_ATTUMEN_ENGAGES and postHealth < math.floor(maxHealth * 25 / 100) then
        state.phase = PHASE_NONE
    end
    return false, newDamage
end

local function attumenSpellHit(event, creature, caster, spellId)
    -- C++ SpellHit(SPELL_MOUNT): cancel the scheduler, both creatures
    -- stop + go PASSIVE, walk toward each other (MoveFollow, no bridge),
    -- then the 1s proximity summon (ported via midnight's mountpoll).
    -- The stop/passive legs are ported below (AttackStop /
    -- RemoveAllAttackers / SetReactState all bridged); MoveFollow stays
    -- out. The midnight->AI()->Talk(EMOTE_MOUNT_UP=1) arm
    -- (boss_midnight.cpp:241) rides the event-14 caster arg — the stale
    -- "no GUID->creature bridge" claim is retracted: the caster IS the
    -- midnight creature that cast SPELL_MOUNT.
    if spellId == SPELL_MOUNT then
        local state = attumenState[creature:GetGUID()]
        if state ~= nil then
            state.phase = PHASE_NONE
        end
        cancelTimers(creature:GetGUID())
        if caster ~= nil then
            caster:AttackStop()
            caster:RemoveAllAttackers()
            caster:SetReactState(REACT_PASSIVE)
            caster:Talk(EMOTE_MOUNT_UP)
        end
        creature:AttackStop()
        creature:RemoveAllAttackers()
        creature:SetReactState(REACT_PASSIVE)
        creature:Talk(SAY_MOUNT)
    end
end

local function attumenTargetDied(event, creature, victim)
    creature:Talk(SAY_KILL)
end

local function attumenDied(event, creature, killer)
    -- C++ JustDied -> _JustDied -> scheduler.CancelAll(): the timers must
    -- not keep firing on the corpse.
    cancelTimers(creature:GetGUID())
    creature:Talk(SAY_DEATH)
end

local function attumenLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    attumenState[guid] = nil
    -- C++ boss_attumenAI::EnterEvadeMode: me->DespawnOrUnsummon()
    -- immediate (the _DespawnAtEvade(10s, midnight) cross-GUID leg stays
    -- blocked — zero GUID->creature lookup bridges on the Lua surface).
    creature:Despawn()
end

local function attumenReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    attumenInitState(guid)
end

RegisterCreatureEvent(ENTRY_ATTUMEN, 1, attumenEnterCombat)
RegisterCreatureEvent(ENTRY_ATTUMEN, 2, attumenLeaveCombat)
RegisterCreatureEvent(ENTRY_ATTUMEN, 3, attumenTargetDied)
RegisterCreatureEvent(ENTRY_ATTUMEN, 4, attumenDied)
RegisterCreatureEvent(ENTRY_ATTUMEN, 9, attumenDamageTaken)
RegisterCreatureEvent(ENTRY_ATTUMEN, 14, attumenSpellHit)
RegisterCreatureEvent(ENTRY_ATTUMEN, 23, attumenReset)

RegisterCreatureEvent(ENTRY_ATTUMEN_MOUNTED, 1, attumenEnterCombat)
RegisterCreatureEvent(ENTRY_ATTUMEN_MOUNTED, 2, attumenLeaveCombat)
RegisterCreatureEvent(ENTRY_ATTUMEN_MOUNTED, 3, attumenTargetDied)
RegisterCreatureEvent(ENTRY_ATTUMEN_MOUNTED, 4, attumenDied)
RegisterCreatureEvent(ENTRY_ATTUMEN_MOUNTED, 9, attumenDamageTaken)
RegisterCreatureEvent(ENTRY_ATTUMEN_MOUNTED, 14, attumenSpellHit)
RegisterCreatureEvent(ENTRY_ATTUMEN_MOUNTED, 23, attumenReset)
