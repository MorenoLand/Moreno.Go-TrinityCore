-- npc_tournament_mount (Argent Tournament mounts) --
-- Lua port of src/server/scripts/World/npcs_special.cpp:1282-1427
-- (AddSC_npcs_special). PassengerBoarded applies the pennant for the
-- mount's faction and the rider's champion/valiant/aspirant standing;
-- unboarding strips it. RegisterVehicleEvent 5/6 (warhead precedent);
-- the handler filters on the 13 mount entries, like C++'s per-AI switch.
--
-- Fidelity note: C++ casts the pennant triggered
-- (player->CastSpell(nullptr, spell, true)); player:CastSpell takes the
-- triggered arg but the session cast path ignores it (castSpellDirect
-- hardcodes triggered=false, spells.go:23307), so the cast lands
-- non-triggered — same fire-and-forget model as the rest of Go's casts.
-- The event fires only when the mount template has a vehicle kit
-- (DB-side, unverifiable in-repo).

local CLASS_DEATH_KNIGHT = 6

local MOUNTS = {
    [33217] = "stormwind", [33316] = "ironforge", [33317] = "gnomeregan",
    [33318] = "exodar", [33319] = "darnassus", [33320] = "orgrimmar",
    [33321] = "senjin", [33322] = "thunderbluff", [33323] = "silvermoon",
    [33324] = "undercity", [33782] = "argent", [33845] = "stormwind",
    [33844] = "silvermoon",
}

-- Pennants per faction: {aspirant, valiant, champion} (npcs_special.cpp:1187-1224).
local PENNANTS = {
    stormwind = {62595, 62596, 62594},
    gnomeregan = {63394, 63395, 63396},
    senjin = {63397, 63398, 63399},
    silvermoon = {63401, 63402, 63403},
    darnassus = {63404, 63405, 63406},
    exodar = {63421, 63422, 63423},
    ironforge = {63425, 63426, 63427},
    undercity = {63428, 63429, 63430},
    orgrimmar = {63431, 63432, 63433},
    thunderbluff = {63434, 63435, 63436},
    argent = {63606, 63500, 63501},
    ebon = {63607, 63608, 63609},
}

-- Champion achievements + valiant quests per faction (npcs_special.cpp:1244-1279).
local CHAMPION = {
    stormwind = 2781, darnassus = 2777, ironforge = 2780, gnomeregan = 2779,
    exodar = 2778, orgrimmar = 2783, senjin = 2784, thunderbluff = 2786,
    undercity = 2787, silvermoon = 2785,
}
local VALIANT_QUESTS = {
    stormwind = {13593, 13684}, darnassus = {13706, 13689},
    ironforge = {13703, 13685}, gnomeregan = {13704, 13688},
    exodar = {13705, 13690}, orgrimmar = {13707, 13691},
    senjin = {13708, 13693}, thunderbluff = {13709, 13694},
    undercity = {13710, 13695}, silvermoon = {13711, 13696},
}
local ACHIEVEMENT_ARGENT_VALOR = 2758
local ACHIEVEMENT_CHAMPION_ALLIANCE = 2782
local ACHIEVEMENT_CHAMPION_HORDE = 2788

local pennantByVehicle = {}

local function getPennantSpellId(entry, player)
    if entry == 33782 then
        local set = "argent"
        if player:GetClass() == CLASS_DEATH_KNIGHT then
            set = "ebon"
        end
        if player:HasAchieved(ACHIEVEMENT_CHAMPION_ALLIANCE) or player:HasAchieved(ACHIEVEMENT_CHAMPION_HORDE) then
            return PENNANTS[set][3]
        elseif player:HasAchieved(ACHIEVEMENT_ARGENT_VALOR) then
            return PENNANTS[set][2]
        else
            return PENNANTS[set][1]
        end
    end
    local faction = MOUNTS[entry]
    if faction == nil then
        return 0
    end
    local set = PENNANTS[faction]
    if player:HasAchieved(CHAMPION[faction]) then
        return set[3]
    end
    local quests = VALIANT_QUESTS[faction]
    if player:GetQuestRewardStatus(quests[1]) or player:GetQuestRewardStatus(quests[2]) then
        return set[2]
    end
    return set[1]
end

local function onBoard(event, vehicle, passenger, seatId)
    if vehicle == nil or MOUNTS[vehicle:GetEntry()] == nil then
        return
    end
    if passenger == nil or passenger:GetObjectType() ~= "Player" then
        return
    end
    local pennant = getPennantSpellId(vehicle:GetEntry(), passenger)
    if pennant == 0 then
        return
    end
    pennantByVehicle[vehicle:GetGUID()] = pennant
    passenger:CastSpell(nil, pennant, true)
end

local function onUnboard(event, vehicle, passenger, seatId)
    if vehicle == nil or MOUNTS[vehicle:GetEntry()] == nil then
        return
    end
    if passenger == nil or passenger:GetObjectType() ~= "Player" then
        return
    end
    local guid = vehicle:GetGUID()
    local pennant = pennantByVehicle[guid]
    pennantByVehicle[guid] = nil
    if pennant == nil or pennant == 0 then
        return
    end
    passenger:RemoveAura(pennant)
end

RegisterVehicleEvent(5, onBoard)
RegisterVehicleEvent(6, onUnboard)
