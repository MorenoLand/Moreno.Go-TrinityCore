-- npc_toc_druid (Trial of the Crusader, Faction Champions) — Lua port of
-- src/server/scripts/Northrend/CrusadersColiseum/TrialOfTheCrusader/boss_faction_champions.cpp
-- (class npc_toc_druidAI (AI_HEALER); C++ CreatureScript("npc_toc_druid")).
-- Currently only the EVENT_HEAL_BARKSKIN arm is ported: the rest of the
-- healer rotation (rejuvenation/tranquility/thorns/nature grasp) rides
-- the faction-champions encounter machine documented in
-- npc_toc_boomkin.lua's header.
-- Entries: 34469 Alliance restoration druid / 34459 Horde restoration
-- druid (trial_of_the_crusader.h:183/197 NPC_ALLIANCE_DRUID_RESTORATION
-- / NPC_HORDE_DRUID_RESTORATION).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 5 OnSpawn, 23 OnReset. Timers via CreateLuaEvent; melee is
-- engine-driven in Go (creature combat tick).
-- Ported arms (C++-exact for all modeled arms):
-- Engage (base boss_faction_championsAI::JustEngagedWith) — DoCast(me,
-- SPELL_ANTI_AOE 68595, true) triggered (boss_faction_champions.cpp:643).
-- EVENT_HEAL_BARKSKIN — HealthBelowPct(30) -> DoCastSelf(Barkskin
-- 65860) + 60s re-arm, else 3s re-arm; 15-25s init
-- (boss_faction_champions.cpp:771/816-824).
-- Reset — SetEquipmentSlots(51799, NO_CHANGE, NO_CHANGE)
-- (boss_faction_champions.cpp:774).
-- EVENT_THREAT UpdatePower + UpdateThreat — per player-victim ScaleThreat(0) +
-- AddThreat(1000000 * 30000 / (health + 1)) (AI_HEALER, no armor
-- read); 5s init, 4s repeats (boss_faction_champions.cpp:591,
-- :611-624, :721-724).

local ENTRY_DRUID_ALLIANCE = 34469 -- trial_of_the_crusader.h:183
local ENTRY_DRUID_HORDE = 34459 -- trial_of_the_crusader.h:197

local SPELL_BARKSKIN = 65860 -- boss_faction_champions.cpp:51
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
-- (boss_faction_champions.cpp:785) PORTED as a 500ms
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

-- C++ npc_toc_druidAI::EVENT_HEAL_BARKSKIN
-- (boss_faction_champions.cpp:816-824): HealthBelowPct(30) ->
-- DoCast(me, SPELL_BARKSKIN) + 60s re-arm, else 3s re-arm. Uses the
-- integer strict-< form (ironaya convention).
local function onBarkskin(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "barkskin", 500, function() onBarkskin(creature, guid) end)
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth > 0
            and creature:GetHealth() < math.floor(maxHealth * 30 / 100) then
        creature:CastSpell(creature, SPELL_BARKSKIN)
        schedule(guid, "barkskin", 60000, function() onBarkskin(creature, guid) end)
    else
        schedule(guid, "barkskin", 3000, function() onBarkskin(creature, guid) end)
    end
end

-- C++ base boss_faction_championsAI::JustEngagedWith
-- (boss_faction_champions.cpp:646-649): DoCast(me, SPELL_ANTI_AOE, true)
-- triggered, SetCombatPulseDelay(5), setActive (no bridge),
-- DoZoneInCombat() + controller SetData(2, IN_PROGRESS) via the shared
-- instanceData counter (PORTED 2026-11-07) — applies to every champion
-- subclass including
-- this druid AI (npc_toc_druidAI : public boss_faction_championsAI,
-- :759); the npc_toc_boomkin.lua precedent ports the bridged arms.
-- C++ boss_faction_championsAI::UpdateThreat (:611-619) on EVENT_THREAT
-- (:721-724, Reset init 5s at :591, then 4s repeats): per threat-list
-- ref, player victims only get ScaleThreat(0) then AddThreat(1000000 *
-- CalculateThreat). For AI_HEALER, dist_mod = 1 and armor_mod = 0, so the
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

local function druidEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:CastSpell(creature, SPELL_ANTI_AOE, true)
    creature:SetCombatPulseDelay(5) -- boss_faction_champions.cpp:647
    creature:DoZoneInCombat() -- boss_faction_champions.cpp:649, in arm order after pulse delay
    championEncounterStart(creature) -- C++ :650-651 SetData(2, IN_PROGRESS)
    schedule(guid, "barkskin", {15000, 25000}, function() onBarkskin(creature, guid) end)
    schedule(guid, "threat", 5000, function() threatCheck(creature, guid) end)
    if creature:IsHeroic() then
        schedule(guid, "removecc", 5000, function() removeCCCheck(creature, guid) end)
    end
end

local function druidLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
    championCountFail(creature) -- C++ JustReachedHome :596-600 SetData(2, FAIL), ev2 twin
end

local function druidDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    championCountDeath(creature) -- C++ JustDied :637-641 SetData(2, DONE)
end

local function druidSpawn(event, creature)
    cancelTimers(creature:GetGUID())
end

local function druidReset(event, creature)
    cancelTimers(creature:GetGUID())
    -- C++ npc_toc_druid::Reset (boss_faction_champions.cpp:774):
    -- SetEquipmentSlots(false, 51799, EQUIP_NO_CHANGE, EQUIP_NO_CHANGE).
    creature:SetEquipmentSlots(51799, 0xFFFFFFFF, 0xFFFFFFFF)
end

-- C++ boss_faction_championsAI::KilledUnit relay (see
-- boss_faction_champions.lua): player victim -> faction-leader bare
-- Talk(SAY_KILLED_PLAYER 6); Alliance-entry druid means Horde players
-- -> Garrosh (34995), Horde-entry druid means Alliance players ->
-- Varian (34990). Trigger event 3, lookup FindNearestCreature 150.0,
-- nil-guarded.
local ENTRY_VARIAN = 34990
local ENTRY_GARROSH = 34995
local SAY_KILLED_PLAYER = 6

local function druidTargetDied(event, creature, victim)
    if victim:GetObjectType() ~= "Player" then
        return
    end
    local leaderEntry = ENTRY_GARROSH
    if creature:GetEntry() == ENTRY_DRUID_HORDE then
        leaderEntry = ENTRY_VARIAN
    end
    local leader = creature:FindNearestCreature(leaderEntry, 150.0)
    if leader ~= nil then
        leader:Talk(SAY_KILLED_PLAYER)
    end
end

RegisterCreatureEvent(ENTRY_DRUID_ALLIANCE, 1, druidEnterCombat)
RegisterCreatureEvent(ENTRY_DRUID_ALLIANCE, 2, druidLeaveCombat)
RegisterCreatureEvent(ENTRY_DRUID_ALLIANCE, 3, druidTargetDied)
RegisterCreatureEvent(ENTRY_DRUID_ALLIANCE, 4, druidDied)
RegisterCreatureEvent(ENTRY_DRUID_ALLIANCE, 5, druidSpawn)
RegisterCreatureEvent(ENTRY_DRUID_ALLIANCE, 23, druidReset)

RegisterCreatureEvent(ENTRY_DRUID_HORDE, 1, druidEnterCombat)
RegisterCreatureEvent(ENTRY_DRUID_HORDE, 2, druidLeaveCombat)
RegisterCreatureEvent(ENTRY_DRUID_HORDE, 3, druidTargetDied)
RegisterCreatureEvent(ENTRY_DRUID_HORDE, 4, druidDied)
RegisterCreatureEvent(ENTRY_DRUID_HORDE, 5, druidSpawn)
RegisterCreatureEvent(ENTRY_DRUID_HORDE, 23, druidReset)
