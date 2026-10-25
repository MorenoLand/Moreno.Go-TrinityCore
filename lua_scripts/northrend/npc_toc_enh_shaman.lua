-- npc_toc_enh_shaman (Trial of the Crusader, Faction Champions) — Lua port of
-- src/server/scripts/Northrend/CrusadersColiseum/TrialOfTheCrusader/boss_faction_champions.cpp
-- (class npc_toc_enh_shamanAI (AI_MELEE); C++ CreatureScript("npc_toc_enh_shaman")).
-- Base engage arms, the KilledUnit relay, and the Reset equipment leg
-- are ported, plus the bridgeable melee rotation arms below: earth
-- shock / lava lash / stormstrike / bloodlust-heroism / windfury
-- (boss_faction_champions.cpp:1933-1938 schedules, armed on enter
-- combat per the boss_faction_champions.lua precedent). The totem
-- machine stays out: DeployTotem (:1955) is bookkeeping-only in
-- C++ (sets _totemCount = 4 + center, no casts/summons — the totem
-- effects are comments), so the EVENT_DEPLOY_TOTEM 1s pump has no
-- observable arm; JustSummoned/SummonedCreatureDespawn only feed
-- that counter, and Reset/JustDied DespawnAll stays vacuous
-- in-model.
-- Entries: 34463 Alliance enhancement shaman / 34455 Horde enhancement
-- shaman (trial_of_the_crusader.h:190/205 NPC_ALLIANCE_SHAMAN_ENHANCEMENT
-- / NPC_HORDE_SHAMAN_ENHANCEMENT).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 5 OnSpawn, 23 OnReset. Melee is engine-driven in Go.
-- Ported arms (C++-exact for all modeled arms):
-- Engage (base boss_faction_championsAI::JustEngagedWith) — DoCast(me,
-- SPELL_ANTI_AOE 68595, true) triggered, SetCombatPulseDelay(5),
-- DoZoneInCombat() (boss_faction_champions.cpp:645-649; setActive has
-- no bridge, controller SetData is instance-gated).
-- KilledUnit relay (base :655-666) — player victim -> faction-leader
-- bare Talk(SAY_KILLED_PLAYER 6); entry-faction inference per the
-- npc_toc_druid.lua precedent (champions spawn opposite-faction to the
-- raid, so an Alliance-entry champion means a Horde raid -> Garrosh).
-- Reset — SetEquipmentSlots(51803, 48013, NO_CHANGE)
-- (boss_faction_champions.cpp:1941); base Reset threat schedule is
-- PORTED below (threatCheck: UpdatePower + melee UpdateThreat, on
-- the new player:GetArmor bridge), and the heroic CC schedule is
-- PORTED below (removeCCCheck); the Reset
-- summons.DespawnAll() (:1942) is vacuous in-model (no totem summons
-- are tracked until the totem machine ports).

local ENTRY_ENH_SHAMAN_ALLIANCE = 34463 -- trial_of_the_crusader.h:190
local ENTRY_ENH_SHAMAN_HORDE = 34455 -- trial_of_the_crusader.h:205

local SPELL_ANTI_AOE = 68595 -- boss_faction_champions.cpp:42

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

local SPELL_HEROISM = 65983 -- boss_faction_champions.cpp:59
local SPELL_BLOODLUST = 65980 -- boss_faction_champions.cpp:60
local SPELL_EARTH_SHOCK = 65973 -- boss_faction_champions.cpp:63
local AURA_EXHAUSTION = 57723 -- boss_faction_champions.cpp:64
local AURA_SATED = 57724 -- boss_faction_champions.cpp:65
local SPELL_LAVA_LASH = 65974 -- boss_faction_champions.cpp:174
local SPELL_STORMSTRIKE = 65970 -- boss_faction_champions.cpp:175
local SPELL_WINDFURY = 65976 -- boss_faction_champions.cpp:176
local POWER_MANA = 0 -- moroes.lua:81 precedent

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

-- C++ boss_faction_championsAI::SelectEnemyCaster (:678, bool ignored):
-- first PvE combat ref whose player has POWER_MANA; GetAITargets is
-- the bridged threat list (threat-sorted, same set).
local function firstManaPlayer(creature)
    for _, target in ipairs(creature:GetAITargets()) do
        if target:GetObjectType() == "Player" and target:GetPowerType() == POWER_MANA then
            return target
        end
    end
    return nil
end

-- C++ EVENT_DPS_EARTH_SHOCK (:1994): enemy caster, init
-- {5s,10s}, re-arm {10s,15s}.
local function onEarthShock(creature, guid)
    local target = firstManaPlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_EARTH_SHOCK)
    end
    schedule(guid, "earthshock", {10000, 15000}, function() onEarthShock(creature, guid) end)
end

-- C++ EVENT_LAVA_LASH (:1999-2003): DoCastVictim, init {3s,5s},
-- re-arm {5s,15s}.
local function onLavaLash(creature, guid)
    creature:CastSpell(nil, SPELL_LAVA_LASH)
    schedule(guid, "lava", {5000, 15000}, function() onLavaLash(creature, guid) end)
end

-- C++ EVENT_STORMSTRIKE (:2003): DoCastVictim, init {2s,5s},
-- re-arm {8s,10s}.
local function onStormstrike(creature, guid)
    creature:CastSpell(nil, SPELL_STORMSTRIKE)
    schedule(guid, "storm", {8000, 10000}, function() onStormstrike(creature, guid) end)
end

-- C++ EVENT_DPS_BLOODLUST_HEROISM (:2007): same entry-gated
-- GetFaction arm as the resto shaman (:900-909) — Alliance entry
-- casts Heroism unless Exhaustion, Horde entry casts Bloodlust
-- unless Sated; DoCastAOE = self-cast. Init 20s, re-arm 5min.
local function onBloodlustHeroism(creature, guid)
    if creature:GetEntry() == ENTRY_ENH_SHAMAN_ALLIANCE then
        if not creature:HasAura(AURA_EXHAUSTION) then
            creature:CastSpell(creature, SPELL_HEROISM)
        end
    else
        if not creature:HasAura(AURA_SATED) then
            creature:CastSpell(creature, SPELL_BLOODLUST)
        end
    end
    schedule(guid, "bloodlust", 300000, function() onBloodlustHeroism(creature, guid) end)
end

-- C++ EVENT_WINDFURY (:2025): DoCastVictim, init {20s,50s},
-- re-arm {20s,60s}.
local function onWindfury(creature, guid)
    creature:CastSpell(nil, SPELL_WINDFURY)
    schedule(guid, "windfury", {20000, 60000}, function() onWindfury(creature, guid) end)
end

-- C++ Reset schedules (:1933-1938, minus the bookkeeping-only
-- EVENT_DEPLOY_TOTEM) armed on enter combat
-- (boss_faction_champions.lua precedent: the EventMap ticks only in
-- combat; canceled on events 2/4/23).
local function scheduleRotation(creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "earthshock", {5000, 10000}, function() onEarthShock(creature, guid) end)
    schedule(guid, "lava", {3000, 5000}, function() onLavaLash(creature, guid) end)
    schedule(guid, "storm", {2000, 5000}, function() onStormstrike(creature, guid) end)
    schedule(guid, "bloodlust", 20000, function() onBloodlustHeroism(creature, guid) end)
    schedule(guid, "windfury", {20000, 50000}, function() onWindfury(creature, guid) end)
end

-- C++ base boss_faction_championsAI::JustEngagedWith
-- (boss_faction_champions.cpp:645-649): DoCast(me, SPELL_ANTI_AOE, true)
-- triggered, SetCombatPulseDelay(5), setActive (no bridge),
-- DoZoneInCombat() — applies to every champion subclass including
-- this enhancement shaman AI (npc_toc_enh_shamanAI : public
-- boss_faction_championsAI, :1916); the npc_toc_druid.lua precedent
-- ports all three bridged arms.
-- C++ boss_faction_championsAI::UpdateThreat (:611-619) on EVENT_THREAT
-- (:721-724, Reset init 5s at :591, then 4s repeats): per threat-list
-- ref, player victims only get ScaleThreat(0) then AddThreat(1000000 *
-- CalculateThreat). For AI_MELEE, CalculateThreat (:603-609) is
-- full-form: dist_mod = 15 / (15 + GetDistance2d), armor_mod =
-- victim armor / 16635, eh = (health + 1) * (1 + armor_mod);
-- victim armor reads player:GetArmor() (lua_player.go). Lowered via
-- GetAITargets + per-victim ModifyThreatByPercent(-100) (ScaleThreat
-- x0) + AddThreat, in C++ per-victim order; creature entries are left
-- untouched, as in C++ (no IsDead filter in C++ either).
local function threatCheck(creature, guid)
    -- C++ UpdatePower (:621-624), first in the EVENT_THREAT case
    -- (:721-724): mana only, ModifyPower(MANA, GetMaxPower(MANA) / 3)
    -- (C++ integer division; boss_faction_champions.lua main-file idiom).
    if creature:GetPowerType() == 0 then
        creature:ModifyPower(0, math.floor(creature:GetMaxPower(0) / 3))
    end
    for _, victim in ipairs(creature:GetAITargets()) do
        if victim:GetObjectType() == "Player" then
            local distMod = 15.0 / (15.0 + creature:GetDistance2d(victim))
            local armorMod = victim:GetArmor() / 16635.0
            local eh = (victim:GetHealth() + 1) * (1.0 + armorMod)
            creature:ModifyThreatByPercent(victim, -100)
            creature:AddThreat(victim, 1000000.0 * distMod * 30000.0 / eh)
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

local function enhShamanEnterCombat(event, creature, target)
    cancelTimers(creature:GetGUID())
    creature:CastSpell(creature, SPELL_ANTI_AOE, true)
    creature:SetCombatPulseDelay(5) -- boss_faction_champions.cpp:647
    creature:DoZoneInCombat() -- boss_faction_champions.cpp:649, in arm order after pulse delay
    scheduleRotation(creature)
    local guid = creature:GetGUID()
    schedule(guid, "threat", 5000, function() threatCheck(creature, guid) end)
    if creature:IsHeroic() then
        schedule(guid, "removecc", 5000, function() removeCCCheck(creature, guid) end)
    end
end

local function enhShamanLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function enhShamanDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function enhShamanSpawn(event, creature)
    cancelTimers(creature:GetGUID())
end

local function enhShamanReset(event, creature)
    cancelTimers(creature:GetGUID())
    -- C++ npc_toc_enh_shaman::Reset (boss_faction_champions.cpp:1941):
    -- SetEquipmentSlots(false, 51803, 48013, EQUIP_NO_CHANGE).
    creature:SetEquipmentSlots(51803, 48013, 0xFFFFFFFF)
end

-- C++ boss_faction_championsAI::KilledUnit relay (see
-- boss_faction_champions.lua): player victim -> faction-leader bare
-- Talk(SAY_KILLED_PLAYER 6); Alliance-entry enhancement shaman means
-- Horde players -> Garrosh (34995), Horde-entry enhancement shaman
-- means Alliance players -> Varian (34990). Trigger event 3, lookup
-- FindNearestCreature 150.0, nil-guarded.
local ENTRY_VARIAN = 34990
local ENTRY_GARROSH = 34995
local SAY_KILLED_PLAYER = 6

local function enhShamanTargetDied(event, creature, victim)
    if victim:GetObjectType() ~= "Player" then
        return
    end
    local leaderEntry = ENTRY_GARROSH
    if creature:GetEntry() == ENTRY_ENH_SHAMAN_HORDE then
        leaderEntry = ENTRY_VARIAN
    end
    local leader = creature:FindNearestCreature(leaderEntry, 150.0)
    if leader ~= nil then
        leader:Talk(SAY_KILLED_PLAYER)
    end
end

RegisterCreatureEvent(ENTRY_ENH_SHAMAN_ALLIANCE, 1, enhShamanEnterCombat)
RegisterCreatureEvent(ENTRY_ENH_SHAMAN_ALLIANCE, 2, enhShamanLeaveCombat)
RegisterCreatureEvent(ENTRY_ENH_SHAMAN_ALLIANCE, 3, enhShamanTargetDied)
RegisterCreatureEvent(ENTRY_ENH_SHAMAN_ALLIANCE, 4, enhShamanDied)
RegisterCreatureEvent(ENTRY_ENH_SHAMAN_ALLIANCE, 5, enhShamanSpawn)
RegisterCreatureEvent(ENTRY_ENH_SHAMAN_ALLIANCE, 23, enhShamanReset)

RegisterCreatureEvent(ENTRY_ENH_SHAMAN_HORDE, 1, enhShamanEnterCombat)
RegisterCreatureEvent(ENTRY_ENH_SHAMAN_HORDE, 2, enhShamanLeaveCombat)
RegisterCreatureEvent(ENTRY_ENH_SHAMAN_HORDE, 3, enhShamanTargetDied)
RegisterCreatureEvent(ENTRY_ENH_SHAMAN_HORDE, 4, enhShamanDied)
RegisterCreatureEvent(ENTRY_ENH_SHAMAN_HORDE, 5, enhShamanSpawn)
RegisterCreatureEvent(ENTRY_ENH_SHAMAN_HORDE, 23, enhShamanReset)
