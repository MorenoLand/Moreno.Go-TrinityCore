-- Grizzly Hills: npc_rocket_propelled_warhead + spell_z_check (61678) +
-- spell_warhead_fuse (49181) — Lua port of
-- src/server/scripts/Northrend/zone_grizzly_hills.cpp (quest "Riding the
-- Red Rocket", daily 12432 Alliance / 12434 Horde).
-- Entry: NPC_ROCKET_PROPELLED_WARHEAD = 27593 (Wowhead npc=27593, reached
-- from the wowwiki Rocket Propelled Warhead page; no entry constant in the
-- C++ tree — DB ScriptName binding, verified externally like the tick-205
-- TDB rows). Lumberboat entries ARE C++ constants
-- (zone_grizzly_hills.cpp:880-881).
-- The machine: boarding the warhead casts 49107 (whose SpellScript casts
-- 61678 + 49331 + 49181 on the warhead); 61678 fires DoAction(0) when the
-- warhead's Z moves; 49181 fires DoAction(0) on remove via the passenger's
-- vehicle base; DoAction runs FinishQuest(false); a lumberboat spell hit
-- runs FinishQuest(true) with the kill-credit cast.

local ENTRY_WARHEAD = 27593 -- Wowhead npc=27593 (see header)
local NPC_HORDE_LUMBERBOAT = 27702 -- zone_grizzly_hills.cpp:880
local NPC_ALLIANCE_LUMBERBOAT = 27688 -- zone_grizzly_hills.cpp:881
local NPC_LUMBERBOAT_EXPLOSIONS = 27689 -- spell_warhead_detonate (:1001)
local SPELL_VEHICLE_WARHEAD_FUSE = 49107 -- zone_grizzly_hills.cpp:877
local SPELL_ALLIANCE_KILL_CREDIT = 49510 -- zone_grizzly_hills.cpp:878
local SPELL_HORDE_KILL_CREDIT = 49340 -- zone_grizzly_hills.cpp:879
local SPELL_WARHEAD_Z_CHECK = 61678 -- zone_grizzly_hills.cpp:954
local SPELL_WARHEAD_SEEKING_LUMBERSHIP = 49331 -- zone_grizzly_hills.cpp:955
local SPELL_WARHEAD_FUSE = 49181 -- zone_grizzly_hills.cpp:956

local SPELL_PARACHUTE = 66154 -- spell_warhead_detonate (:997)
local SPELL_TORPEDO_EXPLOSION = 49290 -- spell_warhead_detonate (:998)

local TEAM_ALLIANCE = 0 -- TeamId (characters.go teamForRace: Alliance 0)

-- Per-warhead-GUID state: the C++ AI members _finished / _faction plus
-- the passenger the detonate script ejects.
local warheadState = {}
-- Z recorded by the 61678 apply arm, per target GUID (C++ _posZ member;
-- nitro-boosts 54621 idiom).
local zCheckZ = {}

local function finishQuest(warhead, success)
    local guid = warhead:GetGUID()
    local st = warheadState[guid]
    if st == nil then
        st = { finished = false, faction = TEAM_ALLIANCE, passengerGuid = nil }
    end
    if st.finished then
        return
    end
    st.finished = true
    warheadState[guid] = st
    if success then
        local credit = SPELL_HORDE_KILL_CREDIT
        if st.faction == TEAM_ALLIANCE then
            credit = SPELL_ALLIANCE_KILL_CREDIT
        end
        warhead:CastSpell(warhead, credit, true)
    end
    -- 49250 detonate has no SpellScript hook, so its script legs run
    -- inline here and the cast itself is skipped (its DBC carries no
    -- other effect): eject the passenger, parachute them, and set off
    -- the lumberboat explosion bunnies in 90yd.
    local passenger = nil
    local kit = warhead:GetVehicleKit()
    if kit ~= nil then
        passenger = kit:GetPassenger(0)
    end
    if passenger == nil and st.passengerGuid ~= nil then
        passenger = GetPlayerByGUID(st.passengerGuid)
    end
    if passenger ~= nil then
        passenger:ExitVehicle()
        warhead:CastSpell(passenger, SPELL_PARACHUTE, true)
    end
    local bunnies = warhead:GetCreaturesInRange(90, NPC_LUMBERBOAT_EXPLOSIONS)
    if bunnies ~= nil then
        for _, bunny in ipairs(bunnies) do
            bunny:CastSpell(bunny, SPELL_TORPEDO_EXPLOSION, true)
        end
    end
    warhead:RemoveAllAuras()
    warhead:SetVisible(false)
    warhead:MoveHome()
end

-- VehicleAI::PassengerBoarded (:898-905): boarding player -> DoCast the
-- 49107 fuse spell on the warhead + record the faction. 49107 is cast
-- non-triggered like the C++ DoCast (its DBC swim-speed leg rides the
-- engine); the SpellScript's three inner casts are triggered, so they
-- are cast directly — the script has no Go hook.
local function warheadPassengerBoarded(event, vehicle, passenger, seatId)
    if vehicle:GetEntry() ~= ENTRY_WARHEAD then
        return
    end
    if passenger == nil or passenger:GetObjectType() ~= "Player" then
        return
    end
    local guid = vehicle:GetGUID()
    warheadState[guid] = {
        finished = false,
        faction = passenger:GetTeam(),
        passengerGuid = passenger:GetGUID(),
    }
    vehicle:CastSpell(vehicle, SPELL_VEHICLE_WARHEAD_FUSE)
    vehicle:CastSpell(vehicle, SPELL_WARHEAD_Z_CHECK, true)
    vehicle:CastSpell(vehicle, SPELL_WARHEAD_SEEKING_LUMBERSHIP, true)
    vehicle:CastSpell(vehicle, SPELL_WARHEAD_FUSE, true)
end

-- VehicleAI::DoAction (:914-917) via the creature:DoAction bridge (event
-- 38): the only action the AI defines runs FinishQuest(false).
local function warheadDoAction(event, creature, action)
    if action == 0 then
        finishQuest(creature, false)
    end
end

-- VehicleAI::SpellHit (:919-923): a lumberboat spell hit finishes the
-- quest with the kill credit.
local function warheadSpellHit(event, creature, caster, spellId)
    if caster == nil then
        return
    end
    local entry = caster:GetEntry()
    if entry == NPC_HORDE_LUMBERBOAT or entry == NPC_ALLIANCE_LUMBERBOAT then
        finishQuest(creature, true)
    end
end

-- VehicleAI::JustReachedHome (:907-912): the home spline landing resets
-- the finished latch and re-shows the warhead (motion Clear is vacuous
-- once home).
local function warheadReachHome(event, creature)
    warheadState[creature:GetGUID()] = nil
    creature:SetVisible(true)
end

-- spell_z_check (:1046-1090): apply records the target Z; the periodic
-- arm PreventDefaults (return true) and fires DoAction(0) on the target
-- creature whenever the Z has moved since apply — _posZ is deliberately
-- NOT updated on fire, so later ticks keep firing and the FinishQuest
-- finished-latch absorbs the repeats, exactly like C++.
local function zCheckApply(event, aura, target)
    zCheckZ[target:GetGUID()] = target:GetZ()
end

local function zCheckPeriodic(event, aura, target)
    local guid = target:GetGUID()
    local lastZ = zCheckZ[guid]
    local curZ = target:GetZ()
    if lastZ == nil then
        zCheckZ[guid] = curZ
        return true
    end
    if lastZ ~= curZ and target:GetObjectType() == "Creature" then
        target:DoAction(0)
    end
    return true
end

local function zCheckRemove(event, aura, target)
    zCheckZ[target:GetGUID()] = nil
end

-- spell_warhead_fuse (:1092-1126): on remove, the target's vehicle base
-- (nil when the target rides nothing — the C++ GetVehicleBase() null arm)
-- gets DoAction(0). The base is the warhead when the fuse sits on the
-- passenger, and the no-op when it sits on the warhead itself, exactly
-- like C++.
local function warheadFuseRemove(event, aura, target)
    local vehicle = target:IsOnVehicle()
    if vehicle == nil then
        return
    end
    local base = vehicle:GetOwner()
    if base == nil or base:GetObjectType() ~= "Creature" then
        return
    end
    base:DoAction(0)
end

RegisterVehicleEvent(5, warheadPassengerBoarded)
RegisterCreatureEvent(ENTRY_WARHEAD, 38, warheadDoAction)
RegisterCreatureEvent(ENTRY_WARHEAD, 14, warheadSpellHit)
RegisterCreatureEvent(ENTRY_WARHEAD, 24, warheadReachHome)
RegisterAuraEvent(SPELL_WARHEAD_Z_CHECK, 2, zCheckApply)
RegisterAuraEvent(SPELL_WARHEAD_Z_CHECK, 1, zCheckPeriodic)
RegisterAuraEvent(SPELL_WARHEAD_Z_CHECK, 3, zCheckRemove)
RegisterAuraEvent(SPELL_WARHEAD_FUSE, 3, warheadFuseRemove)
