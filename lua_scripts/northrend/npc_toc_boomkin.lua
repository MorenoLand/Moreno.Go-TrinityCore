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
-- (UpdatePower + UpdateThreat legs PORTED below; heroic EVENT_REMOVE_CC
-- + RemoveCC legs PORTED below on the new HasBreakable /
-- RemoveAurasByType bridges).
-- base JustEngagedWith (event 1): DoCast(me, SPELL_ANTI_AOE 68595,
-- true) triggered self-cast + SetCombatPulseDelay(5) PORTED
-- (creature:SetCombatPulseDelay bridge, lua_creature_events.go);
-- the DoZoneInCombat leg is PORTED 2026-10-23 (bridge live,
-- engine/world/lua_creature_events.go); the setActive / controller
-- SetData(2, IN_PROGRESS) legs have no bridges (tharon_ja precedent).
-- EVENT_THREAT UpdatePower + UpdateThreat — per player-victim ScaleThreat(0) +
-- AddThreat(1000000 * 30000 / (health + 1)) (AI_RANGED, no armor
-- read); 5s init, 4s repeats (boss_faction_champions.cpp:591,
-- :611-624, :721-724).
-- EVENT_CYCLONE — SelectTarget(Random, 0, NonTankTargetSelector) ->
-- DoCast(Cyclone 65859), 10-20s init, 10-20s re-arm: randomNonTankPlayer
-- (netherspite / blood_prince_council precedent — threat-list
-- players alive minus the victim; C++ NonTankTargetSelector
-- (UnitAI.h:94, playerOnly=true default) excludes the current
-- victim, lowered via GetAITargets).
-- EVENT_ENTANGLING_ROOTS — SelectTarget(Random, 0, 30.0f, true) ->
-- DoCast(Entangling Roots 65857), 10-20s init, 10-20s re-arm:
-- randomPlayerInRange 30.0f (cyanigosa precedent; tank included,
-- threat-list players alive; C++-exact).
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
-- EVENT_DPS_BARKSKIN — HealthBelowPct(30) (integer strict-<,
-- ironaya convention) -> DoCastSelf(Barkskin 65860) + 60s re-arm,
-- else 5s re-arm; 20-30s init.
-- Unmodeled (no bridges — documented, not wired):
-- boss_faction_championsAI UpdateAI: UpdatePower() + UpdateThreat()
-- PORTED above (AI_RANGED, no armor read); EVENT_REMOVE_CC
-- RemoveAurasByType legs — PORTED below (see
-- boss_faction_champions.lua removeCCCheck for the C++ arm shape).
-- npc_toc_boomkin UpdateAI DoSpellAttackIfReady(SPELL_WRATH 65862) —
-- ranged auto-attack leg; Go melee is engine-driven (deviation: the
-- champion melees instead of wrath-spamming between casts).
-- base JustDied: controller SetData(2, DONE) (no cross-AI bridge).
-- base KilledUnit: varian/garrosh DoAction(ACTION_SAY_KILLED_PLAYER)
-- (no cross-AI bridge).
-- base JustReachedHome: controller SetData(2, FAIL) + DespawnOrUnsummon
-- (no bridges).
-- Reset-scan note (2026-10-19): the 16592 SPELL_SHADOWFORM DoCast(me)
-- flagged against this file belongs to npc_toc_shadow_priestAI::Reset
-- (boss_faction_champions.cpp:1141-1156), a separate unported champion
-- AI with no Lua file — not to npc_toc_boomkinAI::Reset, which casts
-- nothing. The shadow priest's whole rotation is unported (scope:
-- champion AIs beyond the boomkin stay out).
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
    local victim = creature:GetVictim()
    local victimGuid = victim ~= nil and victim:GetGUID() or nil
    local candidates = {}
    for _, plr in ipairs(creature:GetAITargets()) do
        if plr:GetObjectType() == "Player"
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
    local candidates = {}
    for _, plr in ipairs(creature:GetAITargets()) do
        if plr:GetObjectType() == "Player" and not plr:IsDead() then
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
    local maxHealth = creature:GetMaxHealth()
    if maxHealth > 0
            and creature:GetHealth() < math.floor(maxHealth * 30 / 100) then
        creature:CastSpell(creature, SPELL_BARKSKIN)
        schedule(guid, "barkskin", 60000, function() onBarkskin(creature, guid) end)
    else
        -- C++ EVENT_DPS_BARKSKIN else-leg: 5s
        -- (boss_faction_champions.cpp:1590; the 3s else-leg belongs to
        -- the resto druid's EVENT_HEAL_BARKSKIN, :823).
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

-- C++ base JustEngagedWith (boss_faction_champions.cpp:644-651): DoCast(me,
-- SPELL_ANTI_AOE, true) triggered, SetCombatPulseDelay(5), setActive,
-- DoZoneInCombat() (PORTED 2026-10-23 — bridge live,
-- engine/world/lua_creature_events.go), controller SetData(2, IN_PROGRESS)
-- (no instance/SetData bridge).
-- C++ boss_faction_championsAI::UpdateThreat (:611-619) on EVENT_THREAT
-- (:721-724, Reset init 5s at :591, then 4s repeats): per threat-list
-- ref, player victims only get ScaleThreat(0) then AddThreat(1000000 *
-- CalculateThreat). For AI_RANGED, dist_mod = 1 and armor_mod = 0, so the
-- amount is 1000000 * 30000 / (health + 1) with no distance or armor
-- read. Lowered via GetAITargets (the bridged threat list) +
-- per-victim ModifyThreatByPercent(-100) (ScaleThreat x0) + AddThreat,
-- in C++ per-victim order; creature entries are left untouched, as in
-- C++ (no IsDead filter in C++ either).
local function threatCheck(creature, guid)
    -- C++ UpdatePower (:621-624), first in the EVENT_THREAT case
    -- (:721-724): mana only, ModifyPower(MANA, GetMaxPower(MANA) / 3)
    -- (C++ integer division; boss_faction_champions.lua main-file idiom).
    if creature:GetPowerType() == 0 then
        creature:ModifyPower(0, math.floor(creature:GetMaxPower(0) / 3))
    end
    for _, victim in ipairs(creature:GetAITargets()) do
        if victim:GetObjectType() == "Player" then
            creature:ModifyThreatByPercent(victim, -100)
            creature:AddThreat(victim, 1000000.0 * (30000.0 / (victim:GetHealth() + 1)))
        end
    end
    schedule(guid, "threat", 4000, function() threatCheck(creature, guid) end)
end

-- C++ boss_faction_championsAI::RemoveCC (:628-635) on heroic
-- EVENT_REMOVE_CC (:726-734): breakable-CC gate, then
-- RemoveAurasByType STUN 12 / FEAR 7 / ROOT 26 / PACIFY 25 / CONFUSE 5
-- in C++ order; 2min re-arm after a strip, 3s re-check otherwise.
-- Full arm note in boss_faction_champions.lua (removeCCCheck).
local function removeCCCheck(creature, guid)
    if creature:HasBreakableByDamageCrowdControlAura() then
        creature:RemoveAurasByType(12)
        creature:RemoveAurasByType(7)
        creature:RemoveAurasByType(26)
        creature:RemoveAurasByType(25)
        creature:RemoveAurasByType(5)
        schedule(guid, "removecc", 120000, function() removeCCCheck(creature, guid) end)
    else
        schedule(guid, "removecc", 3000, function() removeCCCheck(creature, guid) end)
    end
end

local function boomkinEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    creature:CastSpell(creature, SPELL_ANTI_AOE, true)
    creature:SetCombatPulseDelay(5) -- boss_faction_champions.cpp:646
    creature:DoZoneInCombat() -- C++ :649, in arm order after pulse delay (bridge live 2026-10-23)
    schedule(guid, "threat", 5000, function() threatCheck(creature, guid) end)
    if creature:IsHeroic() then
        schedule(guid, "removecc", 5000, function() removeCCCheck(creature, guid) end)
    end
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

-- C++ boss_faction_championsAI::KilledUnit relay (see
-- boss_faction_champions.lua): player victim -> faction-leader bare
-- Talk(SAY_KILLED_PLAYER 6); Alliance-entry boomkin means Horde
-- players -> Garrosh (34995), Horde-entry boomkin means Alliance
-- players -> Varian (34990). Trigger event 3, lookup
-- FindNearestCreature 150.0, nil-guarded.
local ENTRY_VARIAN = 34990
local ENTRY_GARROSH = 34995
local SAY_KILLED_PLAYER = 6

local function boomkinTargetDied(event, creature, victim)
    if victim:GetObjectType() ~= "Player" then
        return
    end
    local leaderEntry = ENTRY_GARROSH
    if creature:GetEntry() == ENTRY_BOOMKIN_HORDE then
        leaderEntry = ENTRY_VARIAN
    end
    local leader = creature:FindNearestCreature(leaderEntry, 150.0)
    if leader ~= nil then
        leader:Talk(SAY_KILLED_PLAYER)
    end
end

RegisterCreatureEvent(ENTRY_BOOMKIN_ALLIANCE, 1, boomkinEnterCombat)
RegisterCreatureEvent(ENTRY_BOOMKIN_ALLIANCE, 2, boomkinLeaveCombat)
RegisterCreatureEvent(ENTRY_BOOMKIN_ALLIANCE, 3, boomkinTargetDied)
RegisterCreatureEvent(ENTRY_BOOMKIN_ALLIANCE, 4, boomkinDied)
RegisterCreatureEvent(ENTRY_BOOMKIN_ALLIANCE, 5, boomkinSpawn)
RegisterCreatureEvent(ENTRY_BOOMKIN_ALLIANCE, 23, boomkinReset)

RegisterCreatureEvent(ENTRY_BOOMKIN_HORDE, 1, boomkinEnterCombat)
RegisterCreatureEvent(ENTRY_BOOMKIN_HORDE, 2, boomkinLeaveCombat)
RegisterCreatureEvent(ENTRY_BOOMKIN_HORDE, 3, boomkinTargetDied)
RegisterCreatureEvent(ENTRY_BOOMKIN_HORDE, 4, boomkinDied)
RegisterCreatureEvent(ENTRY_BOOMKIN_HORDE, 5, boomkinSpawn)
RegisterCreatureEvent(ENTRY_BOOMKIN_HORDE, 23, boomkinReset)
