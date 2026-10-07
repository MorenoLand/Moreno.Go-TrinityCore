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
-- Deviations from C++: the engine has no summon model, so
-- SPELL_SUMMON_ATTUMEN (29714) and SPELL_SUMMON_ATTUMEN_MOUNTED (29799)
-- are visual-only casts and the Eluna events 19/22 (JustSummoned/
-- IsSummonedBy) never fire — the SetGUID/_midnightGUID/_attumenGUID wiring,
-- the summon health-max merge, the DoZoneInCombat call, and the
-- summoner-entry phase assignments have no bridge. Approximation: the
-- phase that C++ IsSummonedBy would set before combat (ATTUMEN_ENGAGES for
-- 15550, MOUNTED for 16152) is set in OnEnterCombat keyed on entry, which
-- is where C++ ScheduleTasks/JustEngagedWith also runs. The 25% mount arm
-- (Midnight's DamageTaken casting SPELL_MOUNT triggered) fires from the
-- Midnight damage hook; Attumen's SpellHit(SPELL_MOUNT) arm cancels its
-- timers and Talk(SAY_MOUNT), but the follow/visibility/summon-mounted
-- steps have no bridge (no MoveFollow/SetVisible/react-state model). No
-- MECHANIC_DISARM info reaches the Lua spell-hit hook, so the
-- SAY_DISARMED arm is skipped. Midnight's directed Talk(SAY_MIDNIGHT_KILL,
-- attumenGUID) broadcasts without a target (Talk bridge takes only the
-- text id). Attumen's JustDied Talk(SAY_DEATH) is kept; midnight->KillSelf
-- has no bridge. EnterEvadeMode despawns (10s) have no bridge.
-- SPELL_CHARGE / SPELL_INTANGIBLE_PRESENCE with no candidate target are
-- skipped (the bridge would fall back to the victim where C++ DoCast(null)
-- is a no-op).

local ENTRY_MIDNIGHT = 16151
local ENTRY_ATTUMEN = 15550
local ENTRY_ATTUMEN_MOUNTED = 16152

local SAY_KILL, SAY_RANDOM, SAY_DISARMED = 0, 1, 2
local SAY_MIDNIGHT_KILL, SAY_DEATH, SAY_APPEAR, SAY_MOUNT = 3, 3, 4, 5
local EMOTE_CALL_ATTUMEN, EMOTE_MOUNT_UP = 0, 1

local SPELL_SHADOWCLEAVE = 29832
local SPELL_INTANGIBLE_PRESENCE = 29833
local SPELL_SPAWN_SMOKE = 10389
local SPELL_CHARGE = 29847
local SPELL_KNOCKDOWN = 29711
local SPELL_SUMMON_ATTUMEN = 29714
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
    per[key] = CreateLuaEvent(fn, delay)
end

local function playersInInstance(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            found[#found + 1] = p
        end
    end
    return found
end

local function randomTargetInBand(creature, minDist, maxDist)
    local candidates = {}
    for _, p in ipairs(playersInInstance(creature)) do
        local dist = creature:GetDistance(p)
        if dist >= minDist and dist < maxDist then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

local function randomTargetAnywhere(creature)
    local candidates = playersInInstance(creature)
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
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
    if state.phase == PHASE_NONE and postHealth * 100 / maxHealth < 95 then
        state.phase = PHASE_ATTUMEN_ENGAGES
        creature:Talk(EMOTE_CALL_ATTUMEN)
        creature:CastSpell(creature, SPELL_SUMMON_ATTUMEN)
    elseif state.phase == PHASE_ATTUMEN_ENGAGES and postHealth * 100 / maxHealth < 25 then
        state.phase = PHASE_MOUNTED
        creature:CastSpell(creature, SPELL_MOUNT, true)
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
end

local function midnightReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    midnightInitState(guid)
end

RegisterCreatureEvent(ENTRY_MIDNIGHT, 1, midnightEnterCombat)
RegisterCreatureEvent(ENTRY_MIDNIGHT, 2, midnightLeaveCombat)
RegisterCreatureEvent(ENTRY_MIDNIGHT, 3, midnightTargetDied)
RegisterCreatureEvent(ENTRY_MIDNIGHT, 9, midnightDamageTaken)
RegisterCreatureEvent(ENTRY_MIDNIGHT, 23, midnightReset)

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
    -- covered by the 16152 entry when it exists in the world.
    if state.phase == PHASE_ATTUMEN_ENGAGES and postHealth * 100 / maxHealth < 25 then
        state.phase = PHASE_NONE
    end
    return false, newDamage
end

local function attumenSpellHit(event, creature, caster, spellId)
    -- C++ SpellHit(SPELL_MOUNT): cancel the scheduler, both creatures walk
    -- toward each other, then DoCastAOE(SPELL_SUMMON_ATTUMEN_MOUNTED).
    -- Movement/visibility/summon have no Lua bridge; keep the timer cancel
    -- and the mount talk.
    if spellId == SPELL_MOUNT then
        local state = attumenState[creature:GetGUID()]
        if state ~= nil then
            state.phase = PHASE_NONE
        end
        cancelTimers(creature:GetGUID())
        creature:Talk(SAY_MOUNT)
    end
end

local function attumenTargetDied(event, creature, victim)
    creature:Talk(SAY_KILL)
end

local function attumenDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

local function attumenLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    attumenState[guid] = nil
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
