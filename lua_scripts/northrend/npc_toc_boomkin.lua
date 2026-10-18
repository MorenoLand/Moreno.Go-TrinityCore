-- npc_toc_boomkin (Trial of the Crusader, Faction Champions) — Lua port of
-- src/server/scripts/Northrend/CrusadersColiseum/TrialOfTheCrusader/boss_faction_champions.cpp
-- (class npc_toc_boomkin (AI_RANGED); C++ CreatureScript("npc_toc_boomkin"),
-- GetTrialOfTheCrusaderAI; AddSC_boss_faction_champions registers the
-- controller + 15 champion/pet scripts). The champion-summon side stays
-- unbridged (DoSummonGrandChampion / NPC_CHAMPIONS_CONTROLLER 34781
-- encounter machine — 06:5x audit, faction_champions documented-only),
-- but the boomkin rotation itself is ~100% bridgeable (06:5x verdict).
-- Entries: 34460 Alliance balance druid / 34451 Horde balance druid
-- (trial_of_the_crusader.h:183/197 NPC_ALLIANCE_DRUID_BALANCE /
-- NPC_HORDE_DRUID_BALANCE).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 5 OnSpawn, 23 OnReset. Timers via CreateLuaEvent; melee is
-- engine-driven in Go (creature combat tick). C++ DoCast default is
-- triggered=false (vaelastrasz convention): creature:CastSpell(nil,
-- spell) = DoCastVictim (moroes precedent), creature:CastSpell(target,
-- spell) = DoCast, creature:CastSpell(creature, spell) = DoCastSelf
-- (phase_hunter precedent; trailing true = triggered, black_knight
-- precedent). C++ events.ScheduleEvent(x, min, max) -> {min, max}ms
-- table delay (amanitar precedent).
-- Ported arms (C++-exact for all modeled arms):
-- npc_toc_boomkin Reset: schedules 8 combat events (event 23, also on
-- spawn event 5 — C++ Creature::Respawn calls AI()->Reset(), mr_smite
-- precedent) + SetEquipmentSlots(50966, NO_CHANGE, NO_CHANGE)
-- (creature twin lua_creature_events.go:294; 0xFFFFFFFF NO_CHANGE
-- sentinel, mr_smite precedent). The base Reset legs ride unbridged:
-- _teamInstance = instance->GetData(DATA_TEAM) (no instance bridge),
-- SetBossBoundary (no boundary bridge), EVENT_THREAT 5s
-- (cross-AI threat-table machine), heroic EVENT_REMOVE_CC 5s.
-- base JustEngagedWith (event 1): DoCast(me, SPELL_ANTI_AOE 68595,
-- true) triggered self-cast — the SetCombatPulseDelay / setActive /
-- DoZoneInCombat / controller SetData(2, IN_PROGRESS) legs have no
-- bridges (tharon_ja precedent).
-- EVENT_CYCLONE — SelectTarget(Random, 0, NonTankTargetSelector) ->
-- DoCast(Cyclone 65859), 10-20s init, 10-20s re-arm: randomNonTankPlayer
-- (netherspite / blood_prince_council precedent — alive same-map
-- players minus the victim; C++ uses the threat list, the
-- GetPlayersInWorld pool is the documented in-tree approx).
-- EVENT_ENTANGLING_ROOTS — SelectTarget(Random, 0, 30.0f, true) ->
-- DoCast(Entangling Roots 65857), 10-20s init, 10-20s re-arm:
-- randomPlayerInRange 30.0f (cyanigosa precedent; tank included,
-- C++-exact).
-- EVENT_FAERIE_FIRE — DoCastVictim(Faerie Fire 65863), 2-5s init,
-- 30-40s re-arm.
-- EVENT_FORCE_OF_NATURE — DoCastVictim(Force of Nature 65861),
-- 20-30s init, 2min re-arm (summoned treants' AIs ride the unbridged
-- summon strand; the cast itself is C++-exact).
-- EVENT_INSECT_SWARM — DoCastVictim(Insect Swarm 65855), 5-10s init,
-- 15-25s re-arm.
-- EVENT_MOONFIRE — DoCastVictim(Moonfire 65856), 10-20s init,
-- 15-30s re-arm.
-- EVENT_STARFIRE — DoCastVictim(Starfire 65854), 10-20s init,
-- 15-30s re-arm.
-- EVENT_DPS_BARKSKIN — HealthBelowPct(30) (creature:GetHealthPct(),
-- sindragosa precedent) -> DoCastSelf(Barkskin 65860) + 60s re-arm,
-- else 5s re-arm; 20-30s init.
-- Unmodeled (no bridges — documented, not wired):
-- boss_faction_championsAI UpdateAI: UpdateThreat() (threat-manager
-- ScaleThreat/AddThreat), UpdatePower() (mana pump), EVENT_REMOVE_CC
-- RemoveAurasByType legs, EVENT_THREAT 5s scheduler — no
-- threat/power/aura-type bridges.
-- npc_toc_boomkin UpdateAI DoSpellAttackIfReady(SPELL_WRATH 65862) —
-- ranged auto-attack leg; Go melee is engine-driven (deviation: the
-- champion melees instead of wrath-spamming between casts).
-- base JustDied: controller SetData(2, DONE) (no cross-AI bridge).
-- base KilledUnit: varian/garrosh DoAction(ACTION_SAY_KILLED_PLAYER)
-- (no cross-AI bridge).
-- base JustReachedHome: controller SetData(2, FAIL) + DespawnOrUnsummon
-- (no bridges).
-- base AttackStart / AI_RANGED movement legs (no motion-master bridge);
-- faction targeting is DB-side (the _teamInstance GetData(DATA_TEAM)
-- leg has no bridge — the creature attacks per its template faction).
-- UNIT_STATE_CASTING gate: Go creature IsCasting() is hard-coded
-- false (lua_objects.go:326-330, ormorok precedent) — timers do not
-- gate on it, C++-suppressed overlap documented.

local ENTRY_BOOMKIN_ALLIANCE = 34460
local ENTRY_BOOMKIN_HORDE = 34451

local SPELL_ANTI_AOE = 68595
local SPELL_CYCLONE = 65859
local SPELL_ENTANGLING_ROOTS = 65857
local SPELL_FAERIE_FIRE = 65863
local SPELL_FORCE_OF_NATURE = 65861
local SPELL_INSECT_SWARM = 65855
local SPELL_MOONFIRE = 65856
local SPELL_STARFIRE = 65854
local SPELL_BARKSKIN = 65860

local EQUIP_STAFF = 50966
local EQUIP_NO_CHANGE = 0xFFFFFFFF

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

-- Random alive player on the same map/instance, excluding the tank
-- (victim) — netherspite / blood_prince_council NonTankTargetSelector
-- precedent.
local function randomNonTankPlayer(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local victim = creature:GetVictim()
    local victimGuid = victim ~= nil and victim:GetGUID() or nil
    local candidates = {}
    for _, plr in ipairs(GetPlayersInWorld()) do
        if plr:GetMapId() == mapId and plr:GetInstanceId() == instanceId
                and not plr:IsDead() and plr:GetGUID() ~= victimGuid then
            candidates[#candidates + 1] = plr
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- Random alive player within maxDist yards (tank included) —
-- cyanigosa precedent; C++ SelectTarget(Random, 0, 30.0f, true).
local function randomPlayerInRange(creature, maxDist)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local candidates = {}
    for _, plr in ipairs(GetPlayersInWorld()) do
        if plr:GetMapId() == mapId and plr:GetInstanceId() == instanceId
                and not plr:IsDead() then
            if creature:GetDistance(plr) <= maxDist then
                candidates[#candidates + 1] = plr
            end
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

local function onCyclone(creature, guid)
    local target = randomNonTankPlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_CYCLONE)
    end
    schedule(guid, "cyclone", {10000, 20000}, function() onCyclone(creature, guid) end)
end

local function onRoots(creature, guid)
    local target = randomPlayerInRange(creature, 30.0)
    if target then
        creature:CastSpell(target, SPELL_ENTANGLING_ROOTS)
    end
    schedule(guid, "roots", {10000, 20000}, function() onRoots(creature, guid) end)
end

local function onFaerieFire(creature, guid)
    creature:CastSpell(nil, SPELL_FAERIE_FIRE)
    schedule(guid, "faerie", {30000, 40000}, function() onFaerieFire(creature, guid) end)
end

local function onForceOfNature(creature, guid)
    creature:CastSpell(nil, SPELL_FORCE_OF_NATURE)
    schedule(guid, "fon", 120000, function() onForceOfNature(creature, guid) end)
end

local function onInsectSwarm(creature, guid)
    creature:CastSpell(nil, SPELL_INSECT_SWARM)
    schedule(guid, "swarm", {15000, 25000}, function() onInsectSwarm(creature, guid) end)
end

local function onMoonfire(creature, guid)
    creature:CastSpell(nil, SPELL_MOONFIRE)
    schedule(guid, "moonfire", {15000, 30000}, function() onMoonfire(creature, guid) end)
end

local function onStarfire(creature, guid)
    creature:CastSpell(nil, SPELL_STARFIRE)
    schedule(guid, "starfire", {15000, 30000}, function() onStarfire(creature, guid) end)
end

local function onBarkskin(creature, guid)
    if creature:GetHealthPct() < 30 then
        creature:CastSpell(creature, SPELL_BARKSKIN)
        schedule(guid, "barkskin", 60000, function() onBarkskin(creature, guid) end)
    else
        schedule(guid, "barkskin", 5000, function() onBarkskin(creature, guid) end)
    end
end

-- C++ Reset: base legs (unbridged, see header) + 8 event schedules +
-- SetEquipmentSlots(false, 50966, NO_CHANGE, NO_CHANGE).
local function scheduleRotation(creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "cyclone", {10000, 20000}, function() onCyclone(creature, guid) end)
    schedule(guid, "roots", {10000, 20000}, function() onRoots(creature, guid) end)
    schedule(guid, "faerie", {2000, 5000}, function() onFaerieFire(creature, guid) end)
    schedule(guid, "fon", {20000, 30000}, function() onForceOfNature(creature, guid) end)
    schedule(guid, "swarm", {5000, 10000}, function() onInsectSwarm(creature, guid) end)
    schedule(guid, "moonfire", {10000, 20000}, function() onMoonfire(creature, guid) end)
    schedule(guid, "starfire", {10000, 20000}, function() onStarfire(creature, guid) end)
    schedule(guid, "barkskin", {20000, 30000}, function() onBarkskin(creature, guid) end)
    creature:SetEquipmentSlots(EQUIP_STAFF, EQUIP_NO_CHANGE, EQUIP_NO_CHANGE)
end

-- C++ base JustEngagedWith: DoCast(me, SPELL_ANTI_AOE, true) triggered.
local function boomkinEnterCombat(event, creature, target)
    creature:CastSpell(creature, SPELL_ANTI_AOE, true)
end

local function boomkinLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function boomkinDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function boomkinSpawn(event, creature)
    scheduleRotation(creature)
end

local function boomkinReset(event, creature)
    scheduleRotation(creature)
end

RegisterCreatureEvent(ENTRY_BOOMKIN_ALLIANCE, 1, boomkinEnterCombat)
RegisterCreatureEvent(ENTRY_BOOMKIN_ALLIANCE, 2, boomkinLeaveCombat)
RegisterCreatureEvent(ENTRY_BOOMKIN_ALLIANCE, 4, boomkinDied)
RegisterCreatureEvent(ENTRY_BOOMKIN_ALLIANCE, 5, boomkinSpawn)
RegisterCreatureEvent(ENTRY_BOOMKIN_ALLIANCE, 23, boomkinReset)

RegisterCreatureEvent(ENTRY_BOOMKIN_HORDE, 1, boomkinEnterCombat)
RegisterCreatureEvent(ENTRY_BOOMKIN_HORDE, 2, boomkinLeaveCombat)
RegisterCreatureEvent(ENTRY_BOOMKIN_HORDE, 4, boomkinDied)
RegisterCreatureEvent(ENTRY_BOOMKIN_HORDE, 5, boomkinSpawn)
RegisterCreatureEvent(ENTRY_BOOMKIN_HORDE, 23, boomkinReset)
