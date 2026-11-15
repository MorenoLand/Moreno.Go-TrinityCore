-- Grand Champions (Trial of the Champion) — Lua port of the JustDied
-- arms of
-- src/server/scripts/Northrend/CrusadersColiseum/TrialOfTheChampion/boss_grand_champions.cpp
-- (boss_warrior_toc5, boss_mage_toc5, boss_shaman_toc5,
-- boss_hunter_toc5, boss_rouge_toc5; AddSC_boss_grand_champions at
-- end registers all five via GetTrialOfTheChampionAI).
-- JustDied census (2026-11-07 tick 378): a filename census of the C++
-- tree flagged this file as unported, but entry-ID verification showed
-- most "missing" names were ports under different basenames
-- (boss_anub_arak, boss_krik_thir, boss_epoch, boss_salramm); the
-- grand-champion entries below have zero hits anywhere in Lua or
-- boss_ai.go — genuinely unported.
-- Class -> entry mapping (trial_of_the_champion.h TCCreatureIds,
-- kalecgos pass; class comments in boss_grand_champions.cpp):
--   Warrior: Marshal Jacob Alerius 34705 / Mokra the Skullcrusher 35572
--   Mage: Ambrose Boltspark 34702 / Eressea Dawnsinger 35569
--   Shaman: Colosos 34701 / Runok Wildmane 35571
--   Hunter: Jaelyne Evensong 34657 / Zul'tore 35570
--   Rogue: Lana Stouthammer 34703 / Deathstalker Visceri 35617
-- All five JustDied arms are identical (boss_grand_champions.cpp:436,
-- :580, :732, :893, :1019): instance->SetBossState(BOSS_GRAND_CHAMPIONS,
-- DONE). BOSS_GRAND_CHAMPIONS = 0 (trial_of_the_champion.h TCData),
-- DONE = 3 (InstanceScript.h EncounterState). Bridged via
-- creature:SetBossState (engine/world/lua_creature_events.go:7099).
-- C++-exact note: the instance override
-- (instance_trial_of_the_champion.cpp SetBossState BOSS_GRAND_CHAMPIONS
-- DONE) increments uiGrandChampionsDeaths and spawns the loot chest at
-- 3 — but InstanceScript::SetBossState returns false when the state is
-- already DONE (InstanceScript.cpp:353-360), and Go's
-- setInstanceBossState mirrors that (instance_boss_states.go:183-190),
-- so the override runs only on the FIRST champion death and the count
-- stalls at 1: the loot leg never fires in C++ either. There is also no
-- boss-state-change instance event for Lua to observe the transition
-- (runtime.go InstanceEvents are 1-7 only), so the instance-side
-- counter stays out — faithfully, not as a gap.
-- Combat rotations: the WARRIOR rotation is ported below
-- (boss_warrior_toc5AI UpdateAI :372-446, round-173); the mage /
-- shaman / hunter / rogue rotations stay out (same timer-machine shape,
-- unported — next strips). The generic_vehicleAI_toc5 mount machine
-- (:180-309, EscortAI charge/shield-breaker) stays out with the
-- dismount machine (GrandChampionsOutVehicle :144-165 +
-- SetHomePosition + EnterEvadeMode + JustReachedHome + AggroAllPlayers
-- :375-400): the vehicle phase has no Go model, and the champions
-- spawn on foot here (0 static spawns in TDB; the instance summon leg
-- is unported), so the ONTRANSPORT gate below is a safety net, never a
-- live branch. The warrior ctor legs (REACT_PASSIVE + NON_ATTACKABLE +
-- ImmuneToPC, :325-329) stay out with that machine: nothing clears
-- them without AggroAllPlayers, so porting them would strand the
-- champion unattackable; the rotation runs from the post-dismount
-- state instead (documented deviation, dormant until the summon lands).
-- Warrior spells 63784/67540/68783 are NOT in spelldbc_seed yet, so
-- the casts are live-dead until a seed lands (valiant precedent).
-- The six Reset() arms are timer seeds for their rotations: the
-- warrior one is ported (ev23 below), the other five stay out with
-- their rotations (round-80 re-verification stands for those).
-- The waypoint legs stay out too: SetData cases 1/2/3 (AddWaypoint +
-- Start) and WaypointReached :220/:223 (instance SetData
-- DATA_MOVEMENT_DONE, GetData+1) have no waypoint bridge; the instance
-- consumer (instance_trial_of_the_champion.cpp:210: at 3 the announcer
-- gets SetData(DATA_IN_POSITION, 0)) stays out with them — the
-- announcer escort machine is documented in
-- instance_trial_of_the_champion.lua.

local BOSS_GRAND_CHAMPIONS = 0
local DONE = 3

local ENTRIES = {
    34705, -- Jacob Alerius (warrior)
    35572, -- Mokra the Skullcrusher (warrior)
    34702, -- Ambrose Boltspark (mage)
    35569, -- Eressea Dawnsinger (mage)
    34701, -- Colosos (shaman)
    35571, -- Runok Wildmane (shaman)
    34657, -- Jaelyne Evensong (hunter)
    35570, -- Zul'tore (hunter)
    34703, -- Lana Stouthammer (rogue)
    35617, -- Deathstalker Visceri (rogue)
}

-- Warrior rotation (boss_warrior_toc5AI, boss_grand_champions.cpp:310-448).
local SPELL_INTERCEPT = 67540
local SPELL_BLADESTORM = 63784
local SPELL_MORTAL_STRIKE = 68783

local WARRIOR_ENTRIES = { 34705, 35572 }

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

-- C++ UpdateAI :402 early return: no victim or mounted (ONTRANSPORT)
-- skips the whole rotation. Timers only run in combat here (ev1 seed,
-- ev2/ev4 drain), so the victim gate is inherent; the mount gate is
-- creature:IsOnVehicle() (vehicle object or nil,
-- lua_creature_events.go:3792) — casts are skipped while mounted but
-- the pumps keep re-arming, where C++ freezes the timers instead.
local function mounted(creature)
    return creature:IsOnVehicle() ~= nil
end

-- C++ intercept leg (:404-419): every 7s, first map player (map order)
-- who is not a GM and in 8-25yd (2D) -> ResetThreatList + AddThreat(5)
-- + DoCast(player, INTERCEPT). C++ checks no alive state here.
local function onIntercept(creature, guid)
    if not mounted(creature) then
        local map = creature:GetMap()
        if map then
            local cx, cy = creature:GetX(), creature:GetY()
            for _, player in ipairs(map:GetPlayers()) do
                if not player:IsGM() then
                    local dx, dy = player:GetX() - cx, player:GetY() - cy
                    local dist = math.sqrt(dx * dx + dy * dy)
                    if dist >= 8.0 and dist <= 25.0 then
                        creature:ResetAllThreat()
                        creature:AddThreat(player, 5.0)
                        creature:CastSpell(player, SPELL_INTERCEPT)
                        break
                    end
                end
            end
        end
    end
    schedule(guid, "intercept", 7000, function() onIntercept(creature, guid) end)
end

-- C++ bladestorm leg (:421-425): DoCastVictim(63784), re-arm 15-20s.
local function onBladestorm(creature, guid)
    if not mounted(creature) then
        local victim = creature:GetVictim()
        if victim then
            creature:CastSpell(victim, SPELL_BLADESTORM)
        end
    end
    schedule(guid, "bladestorm", math.random(15000, 20000), function() onBladestorm(creature, guid) end)
end

-- C++ mortal strike leg (:427-431): DoCastVictim(68783), re-arm 8-12s.
local function onMortalStrike(creature, guid)
    if not mounted(creature) then
        local victim = creature:GetVictim()
        if victim then
            creature:CastSpell(victim, SPELL_MORTAL_STRIKE)
        end
    end
    schedule(guid, "mortal", math.random(8000, 12000), function() onMortalStrike(creature, guid) end)
end

-- C++ Reset (:354-357) = Initialize(): bladestorm urand(15s,20s),
-- intercept 7s, mortal urand(8s,12s); also the engage seed.
local function seedWarriorTimers(creature, guid)
    cancelTimers(guid)
    schedule(guid, "intercept", 7000, function() onIntercept(creature, guid) end)
    schedule(guid, "bladestorm", math.random(15000, 20000), function() onBladestorm(creature, guid) end)
    schedule(guid, "mortal", math.random(8000, 12000), function() onMortalStrike(creature, guid) end)
end

local function onWarriorEnterCombat(event, creature)
    seedWarriorTimers(creature, creature:GetGUIDLow())
end

local function onWarriorReset(event, creature)
    seedWarriorTimers(creature, creature:GetGUIDLow())
end

local function onWarriorLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ JustDied (boss_grand_champions.cpp, all five classes):
-- instance->SetBossState(BOSS_GRAND_CHAMPIONS, DONE).
local function grandChampionDied(event, creature, killer)
    creature:SetBossState(BOSS_GRAND_CHAMPIONS, DONE)
end

local function onWarriorDied(event, creature, killer)
    cancelTimers(creature:GetGUIDLow())
    grandChampionDied(event, creature, killer)
end

local warriorSet = {}
for _, entry in ipairs(WARRIOR_ENTRIES) do
    warriorSet[entry] = true
end

for _, entry in ipairs(ENTRIES) do
    if not warriorSet[entry] then
        RegisterCreatureEvent(entry, 4, grandChampionDied)
    end
end

for _, entry in ipairs(WARRIOR_ENTRIES) do
    RegisterCreatureEvent(entry, 1, onWarriorEnterCombat)
    RegisterCreatureEvent(entry, 2, onWarriorLeaveCombat)
    RegisterCreatureEvent(entry, 4, onWarriorDied)
    RegisterCreatureEvent(entry, 23, onWarriorReset)
end
