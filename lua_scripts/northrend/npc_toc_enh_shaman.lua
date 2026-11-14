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
-- JustSummoned ev19 stays out (round-62): the arm only feeds the
-- _totemCount counter and ev19 IS live (fireCreatureSummoned) — the
-- stay-out stands on vacuity (the AI summons nothing), not on a
-- missing bridge; the AI itself IS ported (this file) and registered
-- (boss_ai.go:19783-19784).
-- Entries: 34463 Alliance enhancement shaman / 34455 Horde enhancement
-- shaman (trial_of_the_crusader.h:190/205 NPC_ALLIANCE_SHAMAN_ENHANCEMENT
-- / NPC_HORDE_SHAMAN_ENHANCEMENT).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 5 OnSpawn, 23 OnReset. Melee is engine-driven in Go.
-- Ported arms (C++-exact for all modeled arms):
-- Engage (base boss_faction_championsAI::JustEngagedWith) — DoCast(me,
-- SPELL_ANTI_AOE 68595, true) triggered, SetCombatPulseDelay(5),
-- DoZoneInCombat() (boss_faction_champions.cpp:645-649; setActive has
-- no bridge; controller SetData(2, IN_PROGRESS) PORTED 2026-11-07 via
-- the shared instanceData counter).
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

-- Faction-champions controller SetData(2, ...) machine — compact twin
-- of the canonical helpers in boss_faction_champions.lua (each Lua
-- file loads in its own state): shared instanceData table from
-- instance_trial_of_the_crusader.lua (map 649).
local DATA_FACTION_CRUSADERS = 2 -- trial_of_the_crusader.h:34
local DATA_ANUBARAK = 5 -- trial_of_the_crusader.h:37
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_FAIL = 2
local BOSS_STATE_DONE = 3

-- C++ controller SetData(2, IN_PROGRESS) (:537-545): reset counters +
-- SetBossState IN_PROGRESS once per pull (boss-state guard = C++
-- !_inProgress gate); total 6 (10-man) / 10 (25-man) per
-- SelectChampions (:395-461); Is25ManRaid live (lce.go:3783).
local DATA_FORDRING = 12 -- trial_of_the_crusader.h:46
local ENTRY_FORDRING = 34996 -- trial_of_the_crusader.h:156 NPC_TIRION_FORDRING
local ACTION_CHAMPIONS_DEFEATED = 13 -- trial_of_the_crusader.h:110
local ACTION_FACTION_WIPE = 25 -- trial_of_the_crusader.h:122

-- Fordring relay (tick 406 U2): GetInstanceCreature + creature:DoAction(n)
-- are bridged; DoAction fires ev38 on the target.
local function sendToFordring(creature, action)
    local fordring = creature:GetInstanceCreature(DATA_FORDRING)
    if fordring == nil then fordring = creature:FindNearestCreature(ENTRY_FORDRING, 200) end
    if fordring ~= nil then fordring:DoAction(action) end
end

local function championEncounterStart(creature)
    if creature:GetBossState(DATA_FACTION_CRUSADERS) == BOSS_STATE_IN_PROGRESS then
        return
    end
    local mapObj = creature:GetMap()
    if mapObj == nil then
        return
    end
    local data = mapObj:GetInstanceData()
    if data == nil then
        return
    end
    data.championsKilled = 0
    data.championsFailed = 0
    data.championsTotal = creature:Is25ManRaid() and 10 or 6
    data.championsInProgress = true
    data.resilienceTimer = 0
    creature:SetBossState(DATA_FACTION_CRUSADERS, BOSS_STATE_IN_PROGRESS)
end

-- C++ controller SetData(2, DONE) (:546-556): 1st kill sets the 60s
-- Resilience timer + special flag (instance :437-440); at total ->
-- SetBossState DONE (instance DONE legs: criteria/chest stay out — no
-- achievement/GO bridges — fordring DoAction(13) is PORTED below,
-- tick 406 U2; resilienceInMinute records the timer read).
local function championCountDeath(creature)
    local mapObj = creature:GetMap()
    if mapObj == nil then
        return
    end
    local data = mapObj:GetInstanceData()
    if data == nil then
        return
    end
    local killed = (data.championsKilled or 0) + 1
    data.championsKilled = killed
    if killed == 1 then
        data.resilienceTimer = 60000
        data.crusadersSpecial = true
    elseif killed >= (data.championsTotal or 6) then
        if (data.resilienceTimer or 0) > 0 then
            data.resilienceInMinute = true
        end
        data.championsInProgress = false
        creature:SetBossState(DATA_FACTION_CRUSADERS, BOSS_STATE_DONE)
        sendToFordring(creature, ACTION_CHAMPIONS_DEFEATED) -- instance :214-215
    end
end

-- C++ controller SetData(2, FAIL) (:526-534) via JustReachedHome
-- (:596-600): FAIL when failed+killed >= total. ev2 (evade start)
-- is the closest twin — it fires only on the evade path, never on
-- death (creaturemotion.go triggerCreatureEvade), so no double-count;
-- fires earlier than C++ if a champion re-engages mid-evade.
local function championCountFail(creature)
    local mapObj = creature:GetMap()
    if mapObj == nil then
        return
    end
    local data = mapObj:GetInstanceData()
    if data == nil then
        return
    end
    local failed = (data.championsFailed or 0) + 1
    data.championsFailed = failed
    if failed + (data.championsKilled or 0) >= (data.championsTotal or 6) then
        data.crusadersSpecial = false
        data.championsInProgress = false
        -- Guard (beasts precedent): C++ cannot re-fire FAIL (summons +
        -- controller despawned), but Lua has no despawn, so repeated
        -- evades would re-fire the relay + decrement — fire once only.
        local state = creature:GetBossState(DATA_FACTION_CRUSADERS)
        if state ~= BOSS_STATE_FAIL and state ~= BOSS_STATE_DONE then
            creature:SetBossState(DATA_FACTION_CRUSADERS, BOSS_STATE_FAIL)
            sendToFordring(creature, ACTION_FACTION_WIPE) -- instance :204-205
            if creature:IsHeroic() then
                local c = (data.trialCounter or 50) - 1
                if c < 0 then c = 0 end
                data.trialCounter = c
                -- TrialCounter == 0 -> despawn Anub'arak (instance :313-317).
                if c == 0 then
                    local anubarak = creature:GetInstanceCreature(DATA_ANUBARAK)
                    if anubarak then
                        anubarak:DespawnOrUnsummon()
                    end
                end
            end
        end
    end
end

-- CASTING gate: derived UpdateAI pre-loop return
-- (boss_faction_champions.cpp:1987) PORTED as a 500ms
-- IsCasting defer per rotation arm (mal_ganis idiom), no repeat
-- re-arm consumed; base EVENT_THREAT / EVENT_REMOVE_CC arms stay
-- ungated (base UpdateAI has no gate). IsCasting is live on the
-- handler surface (lua_creature_events.go:1950).

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
        if target:GetObjectType() == "Player" and not target:IsDead() and target:GetPowerType() == POWER_MANA then
            return target
        end
    end
    return nil
end

-- C++ EVENT_DPS_EARTH_SHOCK (:1994): enemy caster, init
-- {5s,10s}, re-arm {10s,15s}.
local function onEarthShock(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "earthshock", 500, function() onEarthShock(creature, guid) end)
        return
    end
    local target = firstManaPlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_EARTH_SHOCK)
    end
    schedule(guid, "earthshock", {10000, 15000}, function() onEarthShock(creature, guid) end)
end

-- C++ EVENT_LAVA_LASH (:1999-2003): DoCastVictim, init {3s,5s},
-- re-arm {5s,15s}.
local function onLavaLash(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "lava", 500, function() onLavaLash(creature, guid) end)
        return
    end
    creature:CastSpell(nil, SPELL_LAVA_LASH)
    schedule(guid, "lava", {5000, 15000}, function() onLavaLash(creature, guid) end)
end

-- C++ EVENT_STORMSTRIKE (:2003): DoCastVictim, init {2s,5s},
-- re-arm {8s,10s}.
local function onStormstrike(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "storm", 500, function() onStormstrike(creature, guid) end)
        return
    end
    creature:CastSpell(nil, SPELL_STORMSTRIKE)
    schedule(guid, "storm", {8000, 10000}, function() onStormstrike(creature, guid) end)
end

-- C++ EVENT_DPS_BLOODLUST_HEROISM (:2007): same entry-gated
-- GetFaction arm as the resto shaman (:900-909) — Alliance entry
-- casts Heroism unless Exhaustion, Horde entry casts Bloodlust
-- unless Sated; DoCastAOE = self-cast. Init 20s, re-arm 5min.
local function onBloodlustHeroism(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "bloodlust", 500, function() onBloodlustHeroism(creature, guid) end)
        return
    end
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
    if creature:IsCasting() then
        schedule(guid, "windfury", 500, function() onWindfury(creature, guid) end)
        return
    end
    creature:CastSpell(nil, SPELL_WINDFURY)
    schedule(guid, "windfury", {20000, 60000}, function() onWindfury(creature, guid) end)
end

-- C++ Reset schedules (:1933-1938, minus the bookkeeping-only
-- EVENT_DEPLOY_TOTEM) armed on enter combat
-- (boss_faction_champions.lua precedent: the EventMap ticks only in
-- combat; canceled on events 2/4/23).
local function scheduleRotation(creature)
    local guid = creature:GetGUIDLow()
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
-- DoZoneInCombat() + controller SetData(2, IN_PROGRESS) via the shared
-- instanceData counter (PORTED 2026-11-07) — applies to every champion
-- subclass including
-- this enhancement shaman AI (npc_toc_enh_shamanAI : public
-- boss_faction_championsAI, :1916); the npc_toc_druid.lua precedent
-- ports the bridged arms.
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
            local dist2d = creature:GetDistance2d(victim) - creature:GetCombatReach() - 1.5
            if dist2d < 0 then dist2d = 0 end -- C++ GetDistance2d clamps at 0 (Object.cpp:1187); victim ToPlayer-gated, player reach 1.5
            local distMod = 15.0 / (15.0 + dist2d)
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
    cancelTimers(creature:GetGUIDLow())
    creature:CastSpell(creature, SPELL_ANTI_AOE, true)
    creature:SetCombatPulseDelay(5) -- boss_faction_champions.cpp:647
    creature:DoZoneInCombat() -- boss_faction_champions.cpp:649, in arm order after pulse delay
    championEncounterStart(creature) -- C++ :650-651 SetData(2, IN_PROGRESS)
    scheduleRotation(creature)
    local guid = creature:GetGUIDLow()
    schedule(guid, "threat", 5000, function() threatCheck(creature, guid) end)
    if creature:IsHeroic() then
        schedule(guid, "removecc", 5000, function() removeCCCheck(creature, guid) end)
    end
end

local function enhShamanLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
    championCountFail(creature) -- C++ JustReachedHome :596-600 SetData(2, FAIL), ev2 twin
end

local function enhShamanDied(event, creature, killer)
    cancelTimers(creature:GetGUIDLow())
    championCountDeath(creature) -- C++ JustDied :637-641 SetData(2, DONE)
end

local function enhShamanSpawn(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

local function enhShamanReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
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
