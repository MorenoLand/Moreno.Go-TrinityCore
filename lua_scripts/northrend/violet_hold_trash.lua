-- Violet Hold trash (Northrend) — Lua port of the 8 violet_hold_trashAI
-- subclasses in src/server/scripts/Northrend/VioletHold/violet_hold.cpp
-- (AddSC_violet_hold — loader decl 153 / call 348, the NINTH and LAST
-- group of the "// Violet Hold" block).
-- Entries (violet_hold.cpp enum PortalCreatureIds :37-58):
--   30661 Azure Invader (_1) / 30961 (_2) / 31008 (_3, no arms)
--   30662 Azure Spellbreaker (_1) / 30962 (_2) / 31009 (_3, no arms)
--   30663 Azure Binder (_1) / 30918 (_2) / 31007 (_3, no arms)
--   30664 Azure Mage Slayer (_1) / 30963 (_2) / 31010 (_3, no arms) /
--   30665 Veteran Mage Hunter (no arms)
--   30666 Azure Captain / 30667 Azure Sorceror /
--   30668 Azure Raider (_1) / 31118 (_2) / 32191 Azure Stalker
-- Ported: only the combat spell arms each class's ScheduledTasks() seeds
-- from JustEngagedWith (violet_hold_trashAI::JustEngagedWith). The _3
-- variants and 30665 fall through the C++ entry if/else-ifs with no
-- arms — mirrored here (melee-only).
-- Convention (boss_emalon.lua / boss_maiden_of_virtue.lua): module-local
-- GUID-keyed timers table, wipe on events 2 (leave combat), 4 (died),
-- 23 (reset); CreateLuaEvent takes plain ms or a {minMs, maxMs} table
-- for randtime re-arms (maiden precedent);
-- creature:CastSpell(nil, id) = DoCastVictim;
-- creature:CastSpell(creature, id) = DoCast(me)/DoCastAOE self-cast;
-- 2-arg casts = non-triggered (every arm below is non-triggered — no
-- triggered=true in the C++ timer arms).
-- creature:IsWithinDist(plr, range) — the moroes / maiden precedent.
-- DOCUMENTED DEVIATIONS: the TaskScheduler UNIT_STATE_CASTING validator
-- has no bridge — timers fire unconditionally (hodir precedent);
-- UpdateEscortAI's DATA_MAIN_EVENT_STATE != IN_PROGRESS CastStop leg is
-- unmodeled (it would need a per-tick instance-state poll; the door-seal
-- cast is a one-shot at arrival, so the leg only matters if the event
-- ends mid-path); random/nearest target picks are threat-list picks via
-- GetAITargets (corrected 2026-10-25 — the C++ SelectTarget calls pass
-- no playerOnly flag, so the pool is threat-list any-unit, not instance
-- players; the moragg / lavanthor precedent); DoMeleeAttackIfReady is
-- engine-driven.
-- WAYPOINT MACHINE (ported tick 489, below): SetData(DATA_PORTAL_LOCATION
-- 12) paths ride ev22 (IsSummonedBy) — the portal's ev19 writes
-- SetData(12, location) BEFORE the summon's ev22 fires
-- (summonTempCreature: fireCreatureSummoned then fireCreaturePetSummoned,
-- lua_creature_events.go:190/:198), so GetData(12) is already set; the
-- commented-out final waypoint ({1825.7, 807.3, 44.36}) is commented out
-- in C++ too and stays out. Each leg is a MoveTo(id, x, y, z) whose
-- arrival fires ev28 (type 8 POINT_MOTION_TYPE, id) — MoveTo
-- (lua_creature_events.go:4838) + the ymiron precedent; the last
-- arrival runs CreatureStartAttackDoor (SetReactState REACT_DEFENSIVE 1
-- + DoCastAOE 58040 as self-cast). SetHomePosition (bridged,
-- lua_creature_events.go:6368) takes the raw last point + M_PI, exactly
-- as C++ (the AddWaypoint jitter applies to the walked points only).
-- COMBAT DEVIATION: Go script splines are not paused by combat (chase
-- is a derived state, not a spline supersede — creaturemotion.go:5715),
-- so unlike C++ EscortAI the trash keeps walking while fighting and the
-- door-attack can fire mid-combat; ev2 re-issues the next leg as the
-- evade-resume approximation (C++ resumes from the paused waypoint after
-- JustReachedHome; Go has no home-arrival resume leg).
-- DOCUMENTED-ONLY (this file's entries): npc_sinclari_vh — gossip
-- select arms gate on menuId (GOSSIP_MENU_START_ENCOUNTER 9998 /
-- GOSSIP_MENU_SEND_ME_IN 10275), which the Lua select hook never
-- receives (sender/action only — gossip.go:351), and the SEND_ME_IN
-- option's action value is DB-side; the intro/outro scheduler
-- machine (instance wave-count gates, sinclariTrigger talks) is
-- unbridged — documented-only.
-- The 3 teleportation portal scripts are ported in violet_hold.lua
-- (tick 488: wave-summon machines, instance wave-count gates);
-- npc_azure_saboteur (MoveSmoothPath choreography,
-- MovementInform-gated SHIELD_DISRUPTION 58291, instance
-- DATA_START_BOSS_ENCOUNTER), npc_violet_hold_defense_system
-- (Reset-scheduled, not combat-engaged: 4s DoCastAOE 57912+57930, 1s
-- re-arms, 58152 instakill on repeat counter 2, 7s despawn),
-- go_activation_crystal (gossip), and the teleport-player spell
-- script (62138, no SpellScript bridge).

local ENTRY_INVADER_1, ENTRY_INVADER_2 = 30661, 30961
local ENTRY_SPELLBREAKER_1, ENTRY_SPELLBREAKER_2 = 30662, 30962
local ENTRY_BINDER_1, ENTRY_BINDER_2 = 30663, 30918
local ENTRY_MAGE_SLAYER_1, ENTRY_MAGE_SLAYER_2 = 30664, 30963
local ENTRY_CAPTAIN_1 = 30666
local ENTRY_SORCEROR_1 = 30667
local ENTRY_RAIDER_1, ENTRY_RAIDER_2 = 30668, 31118
local ENTRY_STALKER_1 = 32191

local SPELL_CLEAVE = 15496
local SPELL_IMPALE = 58459
local SPELL_BRUTAL_STRIKE = 58460
local SPELL_SUNDER_ARMOR = 58461

local SPELL_ARCANE_BLAST = 58462
local SPELL_SLOW = 25603
local SPELL_CHAINS_OF_ICE = 58464
local SPELL_CONE_OF_COLD = 58463

local SPELL_ARCANE_BARRAGE = 58456
local SPELL_ARCANE_EXPLOSION = 58455
local SPELL_FROST_NOVA = 58458
local SPELL_FROSTBOLT = 58457

local SPELL_ARCANE_EMPOWERMENT = 58469
local SPELL_SPELL_LOCK = 30849

local SPELL_MORTAL_STRIKE = 32736
local SPELL_WHIRLWIND_OF_STEEL = 41057

local SPELL_ARCANE_STREAM = 60181
local SPELL_MANA_DETONATION = 60182

local SPELL_CONCUSSION_BLOW = 52719
local SPELL_MAGIC_REFLECTION = 60158

local SPELL_BACKSTAB = 58471
local SPELL_TACTICAL_BLINK = 58470

local DATA_PORTAL_LOCATION = 12 -- violet_hold.h:64 (VHData: MAIN_EVENT_STATE 9, WAVE_COUNT 10, DOOR_INTEGRITY 11, PORTAL_LOCATION 12)
local SPELL_DESTROY_DOOR_SEAL = 58040
local POINT_MOTION_TYPE = 8 -- movementGeneratorPoint (creaturemotion.go:4020)
local REACT_DEFENSIVE = 1 -- UnitDefines.h: REACT_PASSIVE 0, REACT_DEFENSIVE 1, REACT_AGGRESSIVE 2

local NPC_TELEPORTATION_PORTAL = 30679
local NPC_TELEPORTATION_PORTAL_ELITE = 32174
local NPC_TELEPORTATION_PORTAL_INTRO = 31011

-- violet_hold_trashAI::SetData (violet_hold.cpp:1006-1053): the path
-- array per portal location. Location 7 picks one of the two
-- SecondPortal arrays at random (urand(0, 1)); any other location
-- falls through to DefaultPortalWPs. Coords are C++-exact; the
-- commented-out final waypoint in every array is commented out in C++
-- too and stays out.
local FIRST_PORTAL_WPS = {
    {1877.670288, 842.280273, 43.333591},
    {1877.338867, 834.615356, 38.762287},
    {1872.161011, 823.854309, 38.645401},
    {1864.860474, 815.787170, 38.784843},
    {1858.953735, 810.048950, 44.008759},
    {1843.707153, 805.807739, 44.135197},
}
local SECOND_PORTAL_FIRST_WPS = {
    {1902.561401, 853.334656, 47.106117},
    {1895.486084, 855.376404, 44.334591},
    {1882.805176, 854.993286, 43.333591},
    {1877.670288, 842.280273, 43.333591},
    {1877.338867, 834.615356, 38.762287},
    {1872.161011, 823.854309, 38.645401},
    {1864.860474, 815.787170, 38.784843},
    {1858.953735, 810.048950, 44.008759},
    {1843.707153, 805.807739, 44.135197},
}
local SECOND_PORTAL_SECOND_WPS = {
    {1929.392212, 837.614990, 47.136166},
    {1928.290649, 824.750427, 45.474411},
    {1915.544922, 826.919373, 38.642811},
    {1900.933960, 818.855652, 38.801647},
    {1886.810547, 813.536621, 38.490490},
    {1869.079712, 808.701538, 38.689003},
    {1860.843384, 806.645020, 44.008789},
    {1843.707153, 805.807739, 44.135197},
}
local THIRD_PORTAL_WPS = {
    {1934.049438, 815.778503, 52.408699},
    {1928.290649, 824.750427, 45.474411},
    {1915.544922, 826.919373, 38.642811},
    {1900.933960, 818.855652, 38.801647},
    {1886.810547, 813.536621, 38.490490},
    {1869.079712, 808.701538, 38.689003},
    {1860.843384, 806.645020, 44.008789},
    {1843.707153, 805.807739, 44.135197},
}
local FOURTH_PORTAL_WPS = {
    {1921.658447, 761.657043, 50.866741},
    {1910.559814, 755.780457, 47.701447},
    {1896.664673, 752.920898, 47.667004},
    {1887.398804, 763.633240, 47.666851},
    {1879.020386, 775.396973, 38.705990},
    {1872.439087, 782.568604, 38.808292},
    {1863.573364, 791.173584, 38.743660},
    {1857.811890, 796.765564, 43.950329},
    {1845.577759, 800.681152, 44.104248},
}
local FIFTH_PORTAL_WPS = {
    {1887.398804, 763.633240, 47.666851},
    {1879.020386, 775.396973, 38.705990},
    {1872.439087, 782.568604, 38.808292},
    {1863.573364, 791.173584, 38.743660},
    {1857.811890, 796.765564, 43.950329},
    {1845.577759, 800.681152, 44.104248},
}
local SIXTH_PORTAL_WPS = {
    {1888.861084, 805.074768, 38.375790},
    {1869.793823, 804.135804, 38.647018},
    {1861.541504, 804.149780, 43.968292},
    {1843.567017, 804.288208, 44.139091},
}
local DEFAULT_PORTAL_WPS = {
    {1843.567017, 804.288208, 44.139091},
}

-- Per-GUID escort state: pts = jittered waypoints (C++ AddWaypoint
-- adds irand(-1, 1) to X/Y once, at path selection), next = id of the
-- MoveTo currently in flight (0-based, the C++ waypoint id), last =
-- id of the final waypoint.
local paths = {}

local function selectPath(location)
    if location == 0 then
        return FIRST_PORTAL_WPS
    elseif location == 7 then
        if math.random(0, 1) == 0 then
            return SECOND_PORTAL_FIRST_WPS
        end
        return SECOND_PORTAL_SECOND_WPS
    elseif location == 2 then
        return THIRD_PORTAL_WPS
    elseif location == 6 then
        return FOURTH_PORTAL_WPS
    elseif location == 1 then
        return FIFTH_PORTAL_WPS
    elseif location == 5 then
        return SIXTH_PORTAL_WPS
    end
    return DEFAULT_PORTAL_WPS
end

-- violet_hold_trashAI::SetData(DATA_PORTAL_LOCATION) + Start(true, true)
-- (violet_hold.cpp:1006-1053): rides ev22, which fires on the summoned
-- creature after the portal's ev19 wrote SetData(12, location). The
-- summoner-entry gate keeps GM-spawned or otherwise unsummoned trash
-- (which C++ never starts an escort for) from walking a default path.
-- MoveTo runs at run speed (walk=false in the bridge), matching
-- Start(true, true)'s run=true.
local function trashSummoned(event, creature, summoner)
    if not summoner then
        return
    end
    local summonerEntry = summoner:GetEntry()
    if summonerEntry ~= NPC_TELEPORTATION_PORTAL and summonerEntry ~= NPC_TELEPORTATION_PORTAL_ELITE and summonerEntry ~= NPC_TELEPORTATION_PORTAL_INTRO then
        return
    end
    local guid = creature:GetGUID()
    local raw = selectPath(creature:GetData(DATA_PORTAL_LOCATION))
    local pts = {}
    for i, pt in ipairs(raw) do
        pts[i] = {pt[1] + math.random(-1, 1), pt[2] + math.random(-1, 1), pt[3]}
    end
    local lastPt = raw[#raw]
    creature:SetHomePosition(lastPt[1], lastPt[2], lastPt[3], math.pi)
    paths[guid] = {pts = pts, next = 0, last = #raw - 1}
    creature:MoveTo(0, pts[1][1], pts[1][2], pts[1][3])
end

-- violet_hold_trashAI::WaypointReached (violet_hold.cpp:1055-1059) via
-- the ev28 MovementInform arrival: stale arrivals (an id that is not
-- the leg in flight — a superseded spline never informs, and the gate
-- drops anything else) are ignored; the last waypoint runs
-- CreatureStartAttackDoor, every other arrival chains the next leg.
local function trashMovementInform(event, creature, moveType, id)
    if moveType ~= POINT_MOTION_TYPE then
        return
    end
    local guid = creature:GetGUID()
    local st = paths[guid]
    if not st or id ~= st.next then
        return
    end
    if id == st.last then
        paths[guid] = nil
        creature:SetReactState(REACT_DEFENSIVE)
        creature:CastSpell(creature, SPELL_DESTROY_DOOR_SEAL)
        return
    end
    st.next = id + 1
    local pt = st.pts[st.next + 1]
    creature:MoveTo(st.next, pt[1], pt[2], pt[3])
end

local function clearPath(guid)
    paths[guid] = nil
end

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

-- C++ SelectTarget(Random, 0, dist) with no playerOnly flag draws the
-- threat list (UnitAI.h: "from the threat list"), any unit, alive,
-- within dist (the lavanthor any-unit pool precedent).
local function randomPlayerInRange(creature, range)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        local targetReach = 1.5
        if t:GetObjectType() ~= "Player" then
            targetReach = t:GetCombatReach()
        end
        if t:IsAlive() and creature:GetDistance(t) - creature:GetCombatReach() - targetReach < range then -- IsWithinCombatRange gate, reach-exact (Unit.cpp:583; unfiltered pool branches target reach)
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ SelectTarget(MinDistance, 0, dist) with no playerOnly flag — the
-- nearest alive threat-list unit within dist (any unit, not just
-- players).
local function nearestPlayerInRange(creature, range)
    local best, bestDist = nil, nil
    local reach = creature:GetCombatReach()
    for _, t in ipairs(creature:GetAITargets()) do
        if t:IsAlive() then
            local dist = creature:GetDistance(t)
            local targetReach = 1.5
            if t:GetObjectType() ~= "Player" then
                targetReach = t:GetCombatReach()
            end
            if dist - reach - targetReach < range and (bestDist == nil or dist < bestDist) then -- IsWithinCombatRange gate, reach-exact (Unit.cpp:583; unfiltered pool branches target reach)
                best, bestDist = t, dist
            end
        end
    end
    return best
end

-- npc_azure_invader, entry 30661: DoCastVictim(SPELL_CLEAVE), 5s/5s;
-- DoCastVictim(SPELL_IMPALE), 4s/4s.
local function cleaveTick(creature, guid)
    creature:CastSpell(nil, SPELL_CLEAVE)
    schedule(guid, "cleave", 5000, function() cleaveTick(creature, guid) end)
end

local function impaleTick(creature, guid)
    creature:CastSpell(nil, SPELL_IMPALE)
    schedule(guid, "impale", 4000, function() impaleTick(creature, guid) end)
end

-- npc_azure_invader, entry 30961: DoCastVictim(SPELL_BRUTAL_STRIKE),
-- 5s/5s; DoCastVictim(SPELL_SUNDER_ARMOR), 4s then {8s,10s}.
local function brutalStrikeTick(creature, guid)
    creature:CastSpell(nil, SPELL_BRUTAL_STRIKE)
    schedule(guid, "brutalstrike", 5000, function() brutalStrikeTick(creature, guid) end)
end

local function sunderArmorTick(creature, guid)
    creature:CastSpell(nil, SPELL_SUNDER_ARMOR)
    schedule(guid, "sunderarmor", {8000, 10000}, function() sunderArmorTick(creature, guid) end)
end

-- npc_azure_spellbreaker, entry 30662: random <=30y DoCast
-- SPELL_ARCANE_BLAST, 5s/6s; random <=30y DoCast SPELL_SLOW, 4s/5s.
local function arcaneBlastTick(creature, guid)
    local target = randomPlayerInRange(creature, 30)
    if target then
        creature:CastSpell(target, SPELL_ARCANE_BLAST)
    end
    schedule(guid, "arcaneblast", 6000, function() arcaneBlastTick(creature, guid) end)
end

local function slowTick(creature, guid)
    local target = randomPlayerInRange(creature, 30)
    if target then
        creature:CastSpell(target, SPELL_SLOW)
    end
    schedule(guid, "slow", 5000, function() slowTick(creature, guid) end)
end

-- npc_azure_spellbreaker, entry 30962: random <=30y DoCast
-- SPELL_CHAINS_OF_ICE, 5s/7s; DoCast(me) SPELL_CONE_OF_COLD, 4s/5s.
local function chainsOfIceTick(creature, guid)
    local target = randomPlayerInRange(creature, 30)
    if target then
        creature:CastSpell(target, SPELL_CHAINS_OF_ICE)
    end
    schedule(guid, "chainsofice", 7000, function() chainsOfIceTick(creature, guid) end)
end

local function coneOfColdTick(creature, guid)
    creature:CastSpell(creature, SPELL_CONE_OF_COLD)
    schedule(guid, "coneofcold", 5000, function() coneOfColdTick(creature, guid) end)
end

-- npc_azure_binder, entry 30663: DoCastAOE(SPELL_ARCANE_EXPLOSION),
-- 5s/5s; random <=30y DoCast SPELL_ARCANE_BARRAGE, 4s/6s.
local function arcaneExplosionTick(creature, guid)
    creature:CastSpell(creature, SPELL_ARCANE_EXPLOSION)
    schedule(guid, "arcaneexplosion", 5000, function() arcaneExplosionTick(creature, guid) end)
end

local function arcaneBarrageTick(creature, guid)
    local target = randomPlayerInRange(creature, 30)
    if target then
        creature:CastSpell(target, SPELL_ARCANE_BARRAGE)
    end
    schedule(guid, "arcanebarrage", 6000, function() arcaneBarrageTick(creature, guid) end)
end

-- npc_azure_binder, entry 30918: DoCastAOE(SPELL_FROST_NOVA), 5s/5s;
-- random <=40y DoCast SPELL_FROSTBOLT, 4s/6s.
local function frostNovaTick(creature, guid)
    creature:CastSpell(creature, SPELL_FROST_NOVA)
    schedule(guid, "frostnova", 5000, function() frostNovaTick(creature, guid) end)
end

local function frostboltTick(creature, guid)
    local target = randomPlayerInRange(creature, 40)
    if target then
        creature:CastSpell(target, SPELL_FROSTBOLT)
    end
    schedule(guid, "frostbolt", 6000, function() frostboltTick(creature, guid) end)
end

-- npc_azure_mage_slayer, entry 30664: DoCast(me)
-- SPELL_ARCANE_EMPOWERMENT, 5s/14s.
local function arcaneEmpowermentTick(creature, guid)
    creature:CastSpell(creature, SPELL_ARCANE_EMPOWERMENT)
    schedule(guid, "arcaneempowerment", 14000, function() arcaneEmpowermentTick(creature, guid) end)
end

-- npc_azure_mage_slayer, entry 30963: random <=30y DoCast
-- SPELL_SPELL_LOCK, 5s/9s.
local function spellLockTick(creature, guid)
    local target = randomPlayerInRange(creature, 30)
    if target then
        creature:CastSpell(target, SPELL_SPELL_LOCK)
    end
    schedule(guid, "spelllock", 9000, function() spellLockTick(creature, guid) end)
end

-- npc_azure_raider (30668, 31118): DoCastVictim
-- SPELL_CONCUSSION_BLOW, 5s/5s; DoCast(me) SPELL_MAGIC_REFLECTION,
-- 8s then {10s,15s}.
local function concussionBlowTick(creature, guid)
    creature:CastSpell(nil, SPELL_CONCUSSION_BLOW)
    schedule(guid, "concussionblow", 5000, function() concussionBlowTick(creature, guid) end)
end

local function magicReflectionTick(creature, guid)
    creature:CastSpell(creature, SPELL_MAGIC_REFLECTION)
    schedule(guid, "magicreflection", {10000, 15000}, function() magicReflectionTick(creature, guid) end)
end

-- npc_azure_stalker (32191): 8s -> random <=40y DoCast
-- SPELL_TACTICAL_BLINK, then a nested 1300ms one-shot -> nearest
-- <=5y DoCast SPELL_BACKSTAB; re-arm 8s. C++ nests the backstab
-- schedule and the Repeat() outside the target check — mirrored here.
local function backstabTick(creature)
    local target = nearestPlayerInRange(creature, 5)
    if target then
        creature:CastSpell(target, SPELL_BACKSTAB)
    end
end

local function tacticalBlinkTick(creature, guid)
    local target = randomPlayerInRange(creature, 40)
    if target then
        creature:CastSpell(target, SPELL_TACTICAL_BLINK)
    end
    schedule(guid, "backstab", 1300, function() backstabTick(creature) end)
    schedule(guid, "tacticalblink", 8000, function() tacticalBlinkTick(creature, guid) end)
end

-- npc_azure_captain (30666): DoCastVictim SPELL_MORTAL_STRIKE, 5s/5s;
-- DoCast(me) SPELL_WHIRLWIND_OF_STEEL, 8s/8s.
local function mortalStrikeTick(creature, guid)
    creature:CastSpell(nil, SPELL_MORTAL_STRIKE)
    schedule(guid, "mortalstrike", 5000, function() mortalStrikeTick(creature, guid) end)
end

local function whirlwindOfSteelTick(creature, guid)
    creature:CastSpell(creature, SPELL_WHIRLWIND_OF_STEEL)
    schedule(guid, "whirlwindofsteel", 8000, function() whirlwindOfSteelTick(creature, guid) end)
end

-- npc_azure_sorceror (30667): random <=35y DoCast SPELL_ARCANE_STREAM,
-- 4s then {5s,10s}; DoCastAOE(SPELL_MANA_DETONATION) first at
-- randtime(0s,0s) then {2s,6s} (C++ Schedule(Seconds(), Seconds(), ...)
-- is the randtime overload).
local function arcaneStreamTick(creature, guid)
    local target = randomPlayerInRange(creature, 35)
    if target then
        creature:CastSpell(target, SPELL_ARCANE_STREAM)
    end
    schedule(guid, "arcanestream", {5000, 10000}, function() arcaneStreamTick(creature, guid) end)
end

local function manaDetonationTick(creature, guid)
    creature:CastSpell(creature, SPELL_MANA_DETONATION)
    schedule(guid, "manadetonation", {2000, 6000}, function() manaDetonationTick(creature, guid) end)
end

local function trashEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    local entry = creature:GetEntry()
    if entry == ENTRY_INVADER_1 then
        schedule(guid, "cleave", 5000, function() cleaveTick(creature, guid) end)
        schedule(guid, "impale", 4000, function() impaleTick(creature, guid) end)
    elseif entry == ENTRY_INVADER_2 then
        schedule(guid, "brutalstrike", 5000, function() brutalStrikeTick(creature, guid) end)
        schedule(guid, "sunderarmor", 4000, function() sunderArmorTick(creature, guid) end)
    elseif entry == ENTRY_SPELLBREAKER_1 then
        schedule(guid, "arcaneblast", 5000, function() arcaneBlastTick(creature, guid) end)
        schedule(guid, "slow", 4000, function() slowTick(creature, guid) end)
    elseif entry == ENTRY_SPELLBREAKER_2 then
        schedule(guid, "chainsofice", 5000, function() chainsOfIceTick(creature, guid) end)
        schedule(guid, "coneofcold", 4000, function() coneOfColdTick(creature, guid) end)
    elseif entry == ENTRY_BINDER_1 then
        schedule(guid, "arcaneexplosion", 5000, function() arcaneExplosionTick(creature, guid) end)
        schedule(guid, "arcanebarrage", 4000, function() arcaneBarrageTick(creature, guid) end)
    elseif entry == ENTRY_BINDER_2 then
        schedule(guid, "frostnova", 5000, function() frostNovaTick(creature, guid) end)
        schedule(guid, "frostbolt", 4000, function() frostboltTick(creature, guid) end)
    elseif entry == ENTRY_MAGE_SLAYER_1 then
        schedule(guid, "arcaneempowerment", 5000, function() arcaneEmpowermentTick(creature, guid) end)
    elseif entry == ENTRY_MAGE_SLAYER_2 then
        schedule(guid, "spelllock", 5000, function() spellLockTick(creature, guid) end)
    elseif entry == ENTRY_RAIDER_1 or entry == ENTRY_RAIDER_2 then
        schedule(guid, "concussionblow", 5000, function() concussionBlowTick(creature, guid) end)
        schedule(guid, "magicreflection", 8000, function() magicReflectionTick(creature, guid) end)
    elseif entry == ENTRY_STALKER_1 then
        schedule(guid, "tacticalblink", 8000, function() tacticalBlinkTick(creature, guid) end)
    elseif entry == ENTRY_CAPTAIN_1 then
        schedule(guid, "mortalstrike", 5000, function() mortalStrikeTick(creature, guid) end)
        schedule(guid, "whirlwindofsteel", 8000, function() whirlwindOfSteelTick(creature, guid) end)
    elseif entry == ENTRY_SORCEROR_1 then
        schedule(guid, "arcanestream", 4000, function() arcaneStreamTick(creature, guid) end)
        -- C++ ScheduledTasks: _scheduler.Schedule(0s, ...) = next-tick
        -- fire; direct fire (CreateLuaEvent rejects delay 0,
        -- magmus/abomination precedent); re-arms {2s,6s} itself.
        manaDetonationTick(creature, guid)
    end
    -- _3 variants (31007/31008/31009/31010) and 30665: C++-exact
    -- fall-through — no timer arms, melee only.
end

local function trashCancelCombatTimers(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    clearPath(guid)
end

-- EscortAI evade-resume approximation (see header): leaving combat
-- re-issues the leg in flight. When combat merely interrupted a
-- still-running leg this restarts that leg from the current position;
-- after an evade it resumes the path instead of stranding the trash
-- at its home point (the door, per SetHomePosition above).
local function trashLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    local st = paths[guid]
    if st and not creature:IsDead() then
        local pt = st.pts[st.next + 1]
        creature:MoveTo(st.next, pt[1], pt[2], pt[3])
    end
end

local TRASH_ENTRIES = {
    30661, 30961, 31008,
    30662, 30962, 31009,
    30663, 30918, 31007,
    30664, 30963, 31010, 30665,
    30666,
    30667,
    30668, 31118,
    32191,
}

for _, entry in ipairs(TRASH_ENTRIES) do
    RegisterCreatureEvent(entry, 1, trashEnterCombat)
    RegisterCreatureEvent(entry, 2, trashLeaveCombat)
    RegisterCreatureEvent(entry, 4, trashCancelCombatTimers)
    RegisterCreatureEvent(entry, 22, trashSummoned)
    RegisterCreatureEvent(entry, 23, trashCancelCombatTimers)
    RegisterCreatureEvent(entry, 28, trashMovementInform)
end
