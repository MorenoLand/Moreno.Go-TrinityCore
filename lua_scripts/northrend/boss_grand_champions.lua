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
-- Combat rotations: all five class rotations are ported below — warrior
-- (boss_warrior_toc5AI UpdateAI :372-446, round-173), mage
-- (boss_mage_toc5AI :507-581, round-174), shaman (boss_shaman_toc5AI
-- :651-729, round-174), hunter (boss_hunter_toc5AI :830-908,
-- round-174), rogue (boss_rouge_toc5AI :986-1022, round-174). The
-- generic_vehicleAI_toc5 mount machine
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
-- Warrior spells 63784/67540/68783 and mage 66042-66045, shaman
-- 67528-67530/67534, hunter 65868/66081/66083, rogue 67701/67706/67709
-- are NOT in spelldbc_seed yet, so the casts are live-dead until a
-- seed lands (valiant precedent).

-- C++ SelectTarget(Random, 0) picks a random unit off the threat
-- list: GetAITargets is that list (lua_creature_events.go:4894), and
-- the maiden_of_virtue idiom picks uniformly from its alive players.
local function randomTarget(creature)
    local candidates = {}
    for _, unit in ipairs(creature:GetAITargets()) do
        if unit:GetObjectType() == "Player" and not unit:IsDead() then
            candidates[#candidates + 1] = unit
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end
-- The six Reset() arms are timer seeds for their rotations: all five
-- class Resets are ported (ev23 below); the vehicle AI's Reset stays
-- out with its machine (round-80 re-verification stands for that).
-- The waypoint legs stay out too: SetData cases 1/2/3 (AddWaypoint +
-- Start) and WaypointReached :220/:223 (instance SetData
-- DATA_MOVEMENT_DONE, GetData+1) have no waypoint bridge; the instance
-- consumer (instance_trial_of_the_champion.cpp:210: at 3 the announcer
-- gets SetData(DATA_IN_POSITION, 0)) stays out with them — the
-- announcer escort machine is documented in
-- instance_trial_of_the_champion.lua.

local BOSS_GRAND_CHAMPIONS = 0
local DONE = 3

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

-- Mage rotation (boss_mage_toc5AI, boss_grand_champions.cpp:507-581).
local SPELL_FIREBALL = 66042
local SPELL_POLYMORPH = 66043
local SPELL_BLAST_WAVE = 66044
local SPELL_HASTE = 66045

local MAGE_ENTRIES = { 34702, 35569 }

-- C++ fireball leg (:541-556): the timer is checked twice per tick —
-- once before the UpdateVictim/ONTRANSPORT gate (fires if a victim
-- exists) and once after it — but the first check resets the timer to
-- 5s, so the second never fires in the same tick. Net effect is one
-- 5s victim pump; a single pump is C++-exact here.
local function onFireball(creature, guid)
    if not mounted(creature) then
        local victim = creature:GetVictim()
        if victim then
            creature:CastSpell(victim, SPELL_FIREBALL)
        end
    end
    schedule(guid, "fireball", 5000, function() onFireball(creature, guid) end)
end

-- C++ polymorph leg (:558-562): random threat target, re-arm 8s.
local function onPolymorph(creature, guid)
    if not mounted(creature) then
        local target = randomTarget(creature)
        if target then
            creature:CastSpell(target, SPELL_POLYMORPH)
        end
    end
    schedule(guid, "polymorph", 8000, function() onPolymorph(creature, guid) end)
end

-- C++ blast wave leg (:564-567): DoCastAOE non-triggered = self-cast
-- (established DoCastAOE convention), re-arm 13s.
local function onBlastWave(creature, guid)
    if not mounted(creature) then
        creature:CastSpell(creature, SPELL_BLAST_WAVE)
    end
    schedule(guid, "blastwave", 13000, function() onBlastWave(creature, guid) end)
end

-- C++ haste leg (:570-575): InterruptNonMeleeSpells(true), self-cast,
-- re-arm 22s.
local function onHaste(creature, guid)
    if not mounted(creature) then
        creature:InterruptNonMeleeSpells(true)
        creature:CastSpell(creature, SPELL_HASTE)
    end
    schedule(guid, "haste", 22000, function() onHaste(creature, guid) end)
end

-- C++ Reset (:498-503) = Initialize(): fireball 5s, polymorph 8s,
-- blast wave 12s, haste 22s; also the engage seed.
local function seedMageTimers(creature, guid)
    cancelTimers(guid)
    schedule(guid, "fireball", 5000, function() onFireball(creature, guid) end)
    schedule(guid, "polymorph", 8000, function() onPolymorph(creature, guid) end)
    schedule(guid, "blastwave", 12000, function() onBlastWave(creature, guid) end)
    schedule(guid, "haste", 22000, function() onHaste(creature, guid) end)
end

local function onMageEnterCombat(event, creature)
    seedMageTimers(creature, creature:GetGUIDLow())
end

local function onMageReset(event, creature)
    seedMageTimers(creature, creature:GetGUIDLow())
end

local function onMageLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- Shaman rotation (boss_shaman_toc5AI, boss_grand_champions.cpp:651-729).
local SPELL_CHAIN_LIGHTNING = 67529
local SPELL_EARTH_SHIELD = 67530
local SPELL_HEALING_WAVE = 67528
local SPELL_HEX_OF_MENDING = 67534

local SHAMAN_ENTRIES = { 34701, 35571 }

-- C++ chain lightning leg (:696-701): random threat target, re-arm 16s.
local function onChainLightning(creature, guid)
    if not mounted(creature) then
        local target = randomTarget(creature)
        if target then
            creature:CastSpell(target, SPELL_CHAIN_LIGHTNING)
        end
    end
    schedule(guid, "chain", 16000, function() onChainLightning(creature, guid) end)
end

-- C++ healing wave leg (:703-714): roll_chance_i(50) true -> self-cast;
-- false -> lowest-HP friendly in 40 (DoSelectLowestHpFriendly, minHPDiff
-- 0: alive, in combat, missing > 0). GetFriendlyUnitsInRange is the
-- hexlord lowestHpFriendly scan (same filter, most-missing wins).
local function onHealingWave(creature, guid)
    if not mounted(creature) then
        if math.random(1, 100) <= 50 then
            creature:CastSpell(creature, SPELL_HEALING_WAVE)
        else
            local best, bestMissing = nil, 0
            for _, unit in ipairs(creature:GetFriendlyUnitsInRange(40.0)) do
                if not unit:IsDead() and unit:IsInCombat() then
                    local missing = unit:GetMaxHealth() - unit:GetHealth()
                    if missing > bestMissing then
                        best, bestMissing = unit, missing
                    end
                end
            end
            if best then
                creature:CastSpell(best, SPELL_HEALING_WAVE)
            end
        end
    end
    schedule(guid, "healing", 12000, function() onHealingWave(creature, guid) end)
end

-- C++ earth shield leg (:716-720): self-cast, re-arm urand(30s, 35s).
local function onEarthShield(creature, guid)
    if not mounted(creature) then
        creature:CastSpell(creature, SPELL_EARTH_SHIELD)
    end
    schedule(guid, "earthshield", math.random(30000, 35000), function() onEarthShield(creature, guid) end)
end

-- C++ hex of mending leg (:722-726): DoCastVictim TRIGGERED, re-arm
-- urand(20s, 25s). Third CastSpell arg is the triggered flag.
local function onHexOfMending(creature, guid)
    if not mounted(creature) then
        local victim = creature:GetVictim()
        if victim then
            creature:CastSpell(victim, SPELL_HEX_OF_MENDING, true)
        end
    end
    schedule(guid, "hex", math.random(20000, 25000), function() onHexOfMending(creature, guid) end)
end

-- C++ Reset (:625-630) = Initialize(): chain 16s, healing 12s, earth
-- shield urand(30s,35s), hex urand(20s,25s); also the engage seed.
local function seedShamanTimers(creature, guid)
    cancelTimers(guid)
    schedule(guid, "chain", 16000, function() onChainLightning(creature, guid) end)
    schedule(guid, "healing", 12000, function() onHealingWave(creature, guid) end)
    schedule(guid, "earthshield", math.random(30000, 35000), function() onEarthShield(creature, guid) end)
    schedule(guid, "hex", math.random(20000, 25000), function() onHexOfMending(creature, guid) end)
end

-- C++ JustEngagedWith (:640-644): earth shield on self, hex of mending
-- on the engager (ev1 passes no attacker; GetVictim() is the engager
-- at engage time).
local function onShamanEnterCombat(event, creature)
    seedShamanTimers(creature, creature:GetGUIDLow())
    if not mounted(creature) then
        creature:CastSpell(creature, SPELL_EARTH_SHIELD)
        local victim = creature:GetVictim()
        if victim then
            creature:CastSpell(victim, SPELL_HEX_OF_MENDING)
        end
    end
end

local function onShamanReset(event, creature)
    seedShamanTimers(creature, creature:GetGUIDLow())
end

local function onShamanLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- Hunter rotation (boss_hunter_toc5AI, boss_grand_champions.cpp:830-908).
local SPELL_SHOOT = 65868
local SPELL_MULTI_SHOT = 66081
local SPELL_LIGHTNING_ARROWS = 66083

local HUNTER_ENTRIES = { 34657, 35570 }

local hunterState = {}

local function cancelHunterState(guid)
    hunterState[guid] = nil
end

-- C++ lightning arrows leg (:844-848): DoCastAOE non-triggered =
-- self-cast, re-arm 7s.
local function onLightningArrows(creature, guid)
    if not mounted(creature) then
        creature:CastSpell(creature, SPELL_LIGHTNING_ARROWS)
    end
    schedule(guid, "arrows", 7000, function() onLightningArrows(creature, guid) end)
end

-- C++ multi-shot leg (:867-893): 3s after a shoot, InterruptNonMelee-
-- Spells(true), then multi-shot on the stored shoot target if it is
-- still in 5-30yd, else the first non-GM map player in 5-30yd.
-- The stored target is kept as the unit object (no GUID-resolve bridge
-- is needed: the object stays live in the map while in combat).
local function onMultiShot(creature, guid)
    local st = hunterState[guid]
    if st and st.shoot then
        st.shoot = false
        if not mounted(creature) then
            creature:InterruptNonMeleeSpells(true)
            local target = st.target
            if target and not target:IsDead()
                    and creature:GetDistance(target) >= 5.0
                    and creature:GetDistance(target) <= 30.0 then
                creature:CastSpell(target, SPELL_MULTI_SHOT)
            else
                local map = creature:GetMap()
                if map then
                    for _, player in ipairs(map:GetPlayers()) do
                        if not player:IsGM() then
                            local dist = creature:GetDistance(player)
                            if dist >= 5.0 and dist <= 30.0 then
                                creature:CastSpell(player, SPELL_MULTI_SHOT)
                                break
                            end
                        end
                    end
                end
            end
        end
    end
end

-- C++ shoot leg (:850-862): farthest threat target within 30yd
-- (SelectTarget MaxDistance, 0, 30.0f); re-arm 12s and arm the 3s
-- multi-shot follow-up. The timer reset and bShoot=true sit OUTSIDE
-- the target-found branch, so the multi-shot leg runs (map-scan
-- fallback) even when no shoot target exists.
local function onShoot(creature, guid)
    if not mounted(creature) then
        local best, bestDist = nil, -1
        for _, unit in ipairs(creature:GetAITargets()) do
            if unit:GetObjectType() == "Player" and not unit:IsDead() then
                local dist = creature:GetDistance(unit)
                if dist <= 30.0 and dist > bestDist then
                    best, bestDist = unit, dist
                end
            end
        end
        hunterState[guid] = { shoot = true, target = best }
        if best then
            creature:CastSpell(best, SPELL_SHOOT)
        end
        schedule(guid, "multishot", 3000, function() onMultiShot(creature, guid) end)
    end
    schedule(guid, "shoot", 12000, function() onShoot(creature, guid) end)
end

-- C++ Reset (:798-805) = Initialize(): shoot 12s, lightning arrows 7s,
-- multi-shot 0 (armed only by the shoot leg); also the engage seed.
local function seedHunterTimers(creature, guid)
    cancelTimers(guid)
    cancelHunterState(guid)
    schedule(guid, "shoot", 12000, function() onShoot(creature, guid) end)
    schedule(guid, "arrows", 7000, function() onLightningArrows(creature, guid) end)
end

local function onHunterEnterCombat(event, creature)
    seedHunterTimers(creature, creature:GetGUIDLow())
end

local function onHunterReset(event, creature)
    seedHunterTimers(creature, creature:GetGUIDLow())
end

local function onHunterLeaveCombat(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    cancelHunterState(guid)
end

-- Rogue rotation (boss_rouge_toc5AI, boss_grand_champions.cpp:986-1022).
local SPELL_EVISCERATE = 67709
local SPELL_FAN_OF_KNIVES = 67706
local SPELL_POISON_BOTTLE = 67701

local ROGUE_ENTRIES = { 34703, 35617 }

-- C++ eviscerate leg (:997-1001): DoCastVictim, re-arm 8s.
local function onEviscerate(creature, guid)
    if not mounted(creature) then
        local victim = creature:GetVictim()
        if victim then
            creature:CastSpell(victim, SPELL_EVISCERATE)
        end
    end
    schedule(guid, "eviscerate", 8000, function() onEviscerate(creature, guid) end)
end

-- C++ fan of knives leg (:1003-1007): DoCastAOE non-triggered =
-- self-cast, re-arm 14s.
local function onFanOfKnives(creature, guid)
    if not mounted(creature) then
        creature:CastSpell(creature, SPELL_FAN_OF_KNIVES)
    end
    schedule(guid, "fanknives", 14000, function() onFanOfKnives(creature, guid) end)
end

-- C++ poison bottle leg (:1009-1013): random threat target, re-arm 19s.
local function onPoisonBottle(creature, guid)
    if not mounted(creature) then
        local target = randomTarget(creature)
        if target then
            creature:CastSpell(target, SPELL_POISON_BOTTLE)
        end
    end
    schedule(guid, "poison", 19000, function() onPoisonBottle(creature, guid) end)
end

-- C++ Reset (:966-970) = Initialize(): eviscerate 8s, fan 14s, poison
-- 19s; also the engage seed.
local function seedRogueTimers(creature, guid)
    cancelTimers(guid)
    schedule(guid, "eviscerate", 8000, function() onEviscerate(creature, guid) end)
    schedule(guid, "fanknives", 14000, function() onFanOfKnives(creature, guid) end)
    schedule(guid, "poison", 19000, function() onPoisonBottle(creature, guid) end)
end

local function onRogueEnterCombat(event, creature)
    seedRogueTimers(creature, creature:GetGUIDLow())
end

local function onRogueReset(event, creature)
    seedRogueTimers(creature, creature:GetGUIDLow())
end

local function onRogueLeaveCombat(event, creature)
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

local function onMageDied(event, creature, killer)
    cancelTimers(creature:GetGUIDLow())
    grandChampionDied(event, creature, killer)
end

local function onShamanDied(event, creature, killer)
    cancelTimers(creature:GetGUIDLow())
    grandChampionDied(event, creature, killer)
end

local function onHunterDied(event, creature, killer)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    cancelHunterState(guid)
    grandChampionDied(event, creature, killer)
end

local function onRogueDied(event, creature, killer)
    cancelTimers(creature:GetGUIDLow())
    grandChampionDied(event, creature, killer)
end

for _, entry in ipairs(WARRIOR_ENTRIES) do
    RegisterCreatureEvent(entry, 1, onWarriorEnterCombat)
    RegisterCreatureEvent(entry, 2, onWarriorLeaveCombat)
    RegisterCreatureEvent(entry, 4, onWarriorDied)
    RegisterCreatureEvent(entry, 23, onWarriorReset)
end

for _, entry in ipairs(MAGE_ENTRIES) do
    RegisterCreatureEvent(entry, 1, onMageEnterCombat)
    RegisterCreatureEvent(entry, 2, onMageLeaveCombat)
    RegisterCreatureEvent(entry, 4, onMageDied)
    RegisterCreatureEvent(entry, 23, onMageReset)
end

for _, entry in ipairs(SHAMAN_ENTRIES) do
    RegisterCreatureEvent(entry, 1, onShamanEnterCombat)
    RegisterCreatureEvent(entry, 2, onShamanLeaveCombat)
    RegisterCreatureEvent(entry, 4, onShamanDied)
    RegisterCreatureEvent(entry, 23, onShamanReset)
end

for _, entry in ipairs(HUNTER_ENTRIES) do
    RegisterCreatureEvent(entry, 1, onHunterEnterCombat)
    RegisterCreatureEvent(entry, 2, onHunterLeaveCombat)
    RegisterCreatureEvent(entry, 4, onHunterDied)
    RegisterCreatureEvent(entry, 23, onHunterReset)
end

for _, entry in ipairs(ROGUE_ENTRIES) do
    RegisterCreatureEvent(entry, 1, onRogueEnterCombat)
    RegisterCreatureEvent(entry, 2, onRogueLeaveCombat)
    RegisterCreatureEvent(entry, 4, onRogueDied)
    RegisterCreatureEvent(entry, 23, onRogueReset)
end
