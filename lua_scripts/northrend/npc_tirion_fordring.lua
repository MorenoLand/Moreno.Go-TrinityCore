-- Highlord Tirion Fordring + ToC announcer strand — Lua port of
-- src/server/scripts/Northrend/CrusadersColiseum/TrialOfTheCrusader/trial_of_the_crusader.cpp
-- (npc_tirion_toc / npc_barrett_toc / boss_lich_king_toc /
-- npc_fizzlebang_toc / npc_open_portal_target_toc / npc_garrosh_toc /
-- npc_varian_toc; AddSC_trial_of_the_crusader at end registers all via
-- RegisterTrialOfTheCrusaderCreatureAI, trial_of_the_crusader.h:288).
-- Entries: 34996 Tirion Fordring; Barrett 34816 / 35909 (heroic) /
-- 35035 / 35766 / 35770 / 35771; 34990 Varian; 34995 Garrosh; 35458
-- Wilfred Fizzlebang; 17965 Wilfred portal target; 35651 purple ground;
-- 35877 Lich King; 16980 Lich King voice; 22517 Arthas portal.
-- Summons: 34796 Gormok / 34799 Dreadscale / 34797 Icehowl / 34780
-- Jaraxxus / 34781 champions controller.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 14 OnHitBySpell, 19 OnJustSummoned, 23 OnReset, 28 OnMovementInform
-- (Go extension, args event/creature/type/id; SPLINE_CHAIN_MOTION_TYPE
-- 17), 38 OnDoAction (Go extension, args event/creature/action).
-- RegisterCreatureGossipEvent 2 = OnGossipSelect (args event/player/
-- creature/sender/action/code/listID; menuID is NOT forwarded —
-- gossip.go: menuId branches are recovered per-entry, each Barrett
-- entry carries exactly one menu). Timers via CreateLuaEvent; melee is
-- engine-driven. C++ DoCast default is triggered=false:
-- creature:CastSpell(nil, spell) = DoCastVictim,
-- creature:CastSpell(creature, spell) = DoCastSelf (phase_hunter /
-- apothecary_hanes precedent). DoUseDoorOrButton has no bridge — the
-- twin is FindNearestGameObject(195647) + SetGoState(0) (gothik
-- precedent: 0 = GO_STATE_ACTIVE / open).
-- Ported arms (C++-exact for all modeled arms):
-- npc_tirion_toc DoAction 21 arms (TCRActions, trial_of_the_crusader.h:
-- 92-118; actions 11/12/15/18/21 have no Fordring case in C++ —
-- 11 is an UpdateAI event name, 12 is unused in-tree, 15/21 are
-- Jaraxxus's own, 18 is Varian/Garrosh's — default-ignored) +
-- UpdateAI events + Reset (faction-leader derive + HandleBarrettSummon)
-- + HandleBarrettSummon (heroic Barrett entry, boss-state chain,
-- Jaraxxus-FAIL re-summon leg) + Barrett gossip (all 5 menus;
-- Jaraxxus-fail branch calls Jaraxxus DoAction(21) directly, no
-- gossip-flag removal, per C++) + Barrett spline-despawn (ev28) +
-- Varian/Garrosh ev38 (all 7 arms each, C++-exact delays) +
-- Fizzlebang chain (spline arrival via ev28, portal/purple-ground
-- summon, OPEN_PORTAL cast, Fordring DoAction(14)/(8) relays) +
-- portal-target SpellHit chain (67864 -> 2s -> 68424 + 9s despawn) +
-- LK chain (spline arrival via ev28, Talk, FOG weather, 3x self-cast
-- platform break + Fordring DoAction(20) + SetBossState(4, DONE)) +
-- LK-voice Talk(4)/(5) + Arthas-portal 3s cast/weather legs.
-- + Tirion/Fizzlebang/LK emote legs + EVENT_SUMMON_VALKYR (round-72
-- apply 2026-11-14 — see the Unmodeled note for bridges/cites).
-- The DONE/WIPE relays that feed this file's DoAction arms live in the
-- boss files (beasts 6/5, jaraxxus 9/24, champions 13/25, valkyr
-- 19/26) — this file is only the receiver.
-- Unmodeled (no bridges — documented, not wired):
-- npc_tirion_toc EnterEvadeMode _summons.DespawnAll (no
-- summons-enumeration bridge); ev19 JustSummoned (:381-384) is the
-- one-line _summons.Summon feed for that leg and stays out with it;
-- ev23 fires during evade too (creaturemotion.go:4699) but is not
-- evade-distinct (also spawn + death-path), so it cannot carry the
-- DespawnAll leg. DoAction arms' HandleEmoteCommand legs
-- (EVENT_GORMOK_EXCLAMATION / EVENT_EXCLAMATION / EVENT_EMOTE_SHEATHE)
-- and SetUInt32Value(UNIT_NPC_EMOTESTATE) legs (Fizzlebang EMOTE_TALK /
-- REMOVE_EMOTE_TALK, LK EMOTE_TALK/EXCLAMATION/KNEEL) PORTED 2026-11-14
-- (round-72 apply) via PerformEmote (lua_creature_events.go:4539) and
-- EmoteState (:4798); EVENT_SUMMON_VALKYR SummonCreatureGroup(1) PORTED
-- the same tick (bridge :4218; TDB 26091 ships 6 rows for 34996/group 1,
-- dormant until the repo world.db seeds creature_summon_groups —
-- illidan/council precedent); EVENT_SUMMON_CHAMPIONS
-- controller SetData(0, team) + EVENT_START_CHAMPIONS SetData(1, 0)
-- (boss_faction_champions.cpp:505-562: SummonChampions roster machine
-- + aggressive/flag legs — PORTED tick 410 in boss_faction_champions.lua
-- controllerReset; creature:SetData is a per-creature ScriptData store,
-- not a dispatch, so writing it would be a dead write);
-- ACTION_VALKYR_DEFEATED/WIPE _summons.DespawnEntry(essences)
-- rides the grid-sweep twin below (essences are valkyr-summoned, not
-- Fordring-summoned); boss_lich_king_toc EVENT_BREAK_PLATFORM floor
-- SetDestructibleState (no GO destructible-state bridge) and the
-- ArthasPortalEvent's SPELL_ARTHAS_PORTAL cast rides the summoned
-- portal object (ported) while its BasicEvent queue has no twin;
-- npc_fizzlebang_toc EVENT_SET_TARGET SetTarget twin is bridged
-- (lua_creature_events.go:5349).

local ENTRY_FORDRING = 34996
local ENTRY_BARRETT_BEASTS = 34816
local ENTRY_BARRETT_BEASTS_HC = 35909
local ENTRY_BARRETT_JARAXXUS = 35035
local ENTRY_BARRETT_FACTION = 35766
local ENTRY_BARRETT_VALKYR = 35770
local ENTRY_BARRETT_LK = 35771
local ENTRY_VARIAN = 34990
local ENTRY_GARROSH = 34995
local ENTRY_FIZZLEBANG = 35458
local ENTRY_WILFRED_PORTAL = 17965
local ENTRY_PURPLE_GROUND = 35651
local ENTRY_LICH_KING = 35877
local ENTRY_LICH_KING_VOICE = 16980
local ENTRY_ARTHAS_PORTAL = 22517
local ENTRY_GORMOK = 34796
local ENTRY_DREADSCALE = 34799
local ENTRY_ICEHOWL = 34797
local ENTRY_JARAXXUS = 34780
local ENTRY_CHAMPIONS_CONTROLLER = 34781
local ENTRY_LIGHT_ESSENCE = 34568
local ENTRY_DARK_ESSENCE = 34567

local DATA_NORTHREND_BEASTS = 0
local DATA_JARAXXUS = 1
local DATA_FACTION_CRUSADERS = 2
local DATA_TWIN_VALKIRIES = 3
local DATA_LICH_KING = 4
local DATA_FORDRING = 12
local DATA_VARIAN = 14
local DATA_GARROSH = 15
local DATA_LICH_KING_VOICE = 28
local BOSS_STATE_FAIL = 2
local BOSS_STATE_DONE = 3

local ACTION_START_GORMOK = 1
local ACTION_START_GORMOK_FAIL = 2
local ACTION_START_JORMUNGARS = 3
local ACTION_START_ICEHOWL = 4
local ACTION_NORTHREND_BEASTS_WIPE = 5
local ACTION_NORTHREND_BEASTS_DEFEATED = 6
local ACTION_START_JARAXXUS_EVENT = 7
local ACTION_KILL_JARAXXUS = 8
local ACTION_JARAXXUS_DEFEATED = 9
local ACTION_START_CHAMPIONS = 10
local ACTION_CHAMPIONS_DEFEATED = 13
local ACTION_SUMMON_JARAXXUS = 14
local ACTION_START_VALKYR = 16
local ACTION_START_LK_EVENT = 17
local ACTION_VALKYR_DEFEATED = 19
local ACTION_LK_EVENT_FINISHED = 20
local ACTION_START_CHAMPIONS_ENGAGE = 22
local ACTION_START_VALKYR_ENGAGE = 23
local ACTION_JARAXXUS_WIPE = 24
local ACTION_FACTION_WIPE = 25
local ACTION_VALKYR_WIPE = 26

local GO_MAIN_GATE_DOOR = 195647
local AREA_TRIAL_OF_THE_CRUSADER = 4722
local WEATHER_FOG = 1
local WEATHER_LIGHT_SNOW = 6
local WEATHER_MEDIUM_SNOW = 7
local SPLINE_CHAIN_MOTION_TYPE = 17
local SPLINE_INITIAL_MOVEMENT = 1
local POINT_SUMMON = 1
local POINT_MIDDLE = 2
local POINT_BARRETT_DESPAWN = 1
local GOSSIPID_FAIL = 1
local REACT_PASSIVE = 0
local SUMMON_TIMED_DESPAWN = 0
local SUMMON_CORPSE_TIMED_DESPAWN = 5
local SUMMON_DEAD_DESPAWN = 6

local SPELL_OPEN_PORTAL = 67864
local SPELL_JARAXXUS_HITTIN_YA = 66327
local SPELL_FEL_LIGHTNING_INTRO = 67888
local SPELL_LK_FROST_NOVA = 68198
local SPELL_CORPSE_TELEPORT = 69016
local SPELL_DESTROY_FLOOR_KNOCKUP = 68193
local SPELL_ARTHAS_PORTAL = 51807

local BARRETT_POS = { 559.1528, 90.55729, 395.2734, 5.078908 }
local WILFRED_POS = { 563.6007, 208.5278, 395.2696, 4.729842 }
local JARAXXUS_POS = { 563.8264, 140.6563, 393.9861, 4.694936 }
local PORTAL_TARGET_POS = { 563.6597, 139.7569, 399.2507, 4.712389 }
local PURPLE_GROUND_POS = { 563.6858, 139.4323, 393.9862, 4.694936 }
local ARTHAS_PORTAL_POS = { 563.6996, 175.9826, 394.5042, 4.694936 }
local LICH_KING_POS = { 563.5712, 174.8351, 394.4954, 4.712389 }
local BEASTS_POS_GORMOK = { 563.9358, 229.8299, 394.8061, 4.694936 }
local BEASTS_POS_DREADSCALE = { 564.2802, 233.1322, 394.7897, 1.621917 }
local CHAMPIONS_CONTROLLER_POS = { 563.672974, 139.571, 393.837006, 0.0 }

local timers = {}
local ford = {}

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

-- C++ InstanceScript::DoUseDoorOrButton twin (no bridge): the main gate
-- is only ever opened in these arms, never closed — gothik precedent.
local function openMainGate(creature)
    local gate = creature:FindNearestGameObject(GO_MAIN_GATE_DOOR, 200)
    if gate then
        gate:SetGoState(0)
    end
end

local function getFordring(creature)
    local fordring = creature:GetInstanceCreature(DATA_FORDRING)
    if fordring == nil then
        fordring = creature:FindNearestCreature(ENTRY_FORDRING, 200)
    end
    return fordring
end

local function sendToFordring(creature, action)
    local fordring = getFordring(creature)
    if fordring ~= nil then
        fordring:DoAction(action)
    end
end

-- C++ npc_tirion_toc::Reset derives _factionLeaderData from the
-- instance team (DATA_TEAM, set on first player enter); Lua has no
-- instance GetData bridge, so derive it from the first player on the
-- map (player:GetTeam 0 Alliance / 1 Horde). No players yet mirrors
-- the C++ TEAM_OTHER default, which picks Garrosh.
local function deriveFactionLeader(creature)
    local mapObj = creature:GetMap()
    if mapObj ~= nil then
        local players = mapObj:GetPlayers()
        if players ~= nil then
            for _, player in ipairs(players) do
                local team = player:GetTeam()
                if team == 0 then
                    return DATA_VARIAN
                elseif team == 1 then
                    return DATA_GARROSH
                end
            end
        end
    end
    return DATA_GARROSH
end

local function fordringState(creature)
    local guid = creature:GetGUIDLow()
    local st = ford[guid]
    if not st then
        st = { leader = DATA_GARROSH, jormungarsSummoned = false, icehowlSummoned = false }
        ford[guid] = st
    end
    return st
end

-- C++ _summons.DespawnEntry twin: the essences are summoned by the
-- valkyrs, not Fordring, so no summon list holds them — entry-exact
-- grid sweep (snobold precedent); both essences live in-arena.
local function despawnEssences(creature)
    for _, essence in ipairs(creature:GetCreaturesInGrid(ENTRY_LIGHT_ESSENCE, 200)) do
        essence:DespawnOrUnsummon()
    end
    for _, essence in ipairs(creature:GetCreaturesInGrid(ENTRY_DARK_ESSENCE, 200)) do
        essence:DespawnOrUnsummon()
    end
end

-- C++ npc_tirion_toc::HandleBarrettSummon (trial_of_the_crusader.cpp:
-- 372-390): first not-DONE encounter in order, heroic beasts Barrett
-- when IsHeroic(); the Jaraxxus branch re-summons Jaraxxus itself when
-- the Jaraxxus state is FAIL.
local function handleBarrettSummon(creature)
    if creature:GetBossState(DATA_NORTHREND_BEASTS) ~= BOSS_STATE_DONE then
        if creature:IsHeroic() then
            creature:SummonCreature(ENTRY_BARRETT_BEASTS_HC, BARRETT_POS[1], BARRETT_POS[2], BARRETT_POS[3], BARRETT_POS[4])
        else
            creature:SummonCreature(ENTRY_BARRETT_BEASTS, BARRETT_POS[1], BARRETT_POS[2], BARRETT_POS[3], BARRETT_POS[4])
        end
    elseif creature:GetBossState(DATA_JARAXXUS) ~= BOSS_STATE_DONE then
        creature:SummonCreature(ENTRY_BARRETT_JARAXXUS, BARRETT_POS[1], BARRETT_POS[2], BARRETT_POS[3], BARRETT_POS[4])
        if creature:GetBossState(DATA_JARAXXUS) == BOSS_STATE_FAIL then
            sendToFordring(creature, ACTION_SUMMON_JARAXXUS)
        end
    elseif creature:GetBossState(DATA_FACTION_CRUSADERS) ~= BOSS_STATE_DONE then
        creature:SummonCreature(ENTRY_BARRETT_FACTION, BARRETT_POS[1], BARRETT_POS[2], BARRETT_POS[3], BARRETT_POS[4])
    elseif creature:GetBossState(DATA_TWIN_VALKIRIES) ~= BOSS_STATE_DONE then
        creature:SummonCreature(ENTRY_BARRETT_VALKYR, BARRETT_POS[1], BARRETT_POS[2], BARRETT_POS[3], BARRETT_POS[4])
    elseif creature:GetBossState(DATA_LICH_KING) ~= BOSS_STATE_DONE then
        creature:SummonCreature(ENTRY_BARRETT_LK, BARRETT_POS[1], BARRETT_POS[2], BARRETT_POS[3], BARRETT_POS[4])
    end
end

-- C++ npc_tirion_toc::UpdateAI events, as CreateLuaEvent timer chains
-- keyed by the Fordring GUID. EVENT_EXCLAMATION /
-- EVENT_EMOTE_SHEATHE are HandleEmoteCommand legs, ported via PerformEmote
-- (lua_creature_events.go:4539) — their downstream schedules are kept.
local function fordringGormokIntro(creature, guid)
    creature:Talk(1)
    local leader = creature:GetInstanceCreature(fordringState(creature).leader)
    if leader ~= nil then
        leader:DoAction(ACTION_START_GORMOK)
    end
    schedule(guid, "gormokExclamation", 6000, function()
        -- EVENT_GORMOK_EXCLAMATION: emote, then schedules SPAWN_GORMOK.
        creature:PerformEmote(5) -- C++ EMOTE_ONESHOT_EXCLAMATION
        schedule(guid, "spawnGormok", 6000, function()
            openMainGate(creature)
            creature:SummonCreature(ENTRY_GORMOK, BEASTS_POS_GORMOK[1], BEASTS_POS_GORMOK[2], BEASTS_POS_GORMOK[3], BEASTS_POS_GORMOK[4], SUMMON_CORPSE_TIMED_DESPAWN, 12000)
        end)
    end)
end

local function fordringTirionLament(creature, guid)
    creature:Talk(8)
    local varian = creature:GetInstanceCreature(DATA_VARIAN)
    if varian ~= nil then
        varian:DoAction(ACTION_JARAXXUS_DEFEATED)
    end
    local garrosh = creature:GetInstanceCreature(DATA_GARROSH)
    if garrosh ~= nil then
        garrosh:DoAction(ACTION_JARAXXUS_DEFEATED)
    end
    schedule(guid, "tirionCalmDown", 33000, function()
        creature:Talk(9)
        schedule(guid, "summonBarrett", 20000, function()
            handleBarrettSummon(creature)
        end)
    end)
end

local function fordringLichKingChallenge(creature, guid)
    local voice = creature:GetInstanceCreature(DATA_LICH_KING_VOICE)
    if voice ~= nil then
        voice:Talk(4)
    end
    local portal = creature:SummonCreature(ENTRY_ARTHAS_PORTAL, ARTHAS_PORTAL_POS[1], ARTHAS_PORTAL_POS[2], ARTHAS_PORTAL_POS[3], ARTHAS_PORTAL_POS[4], SUMMON_TIMED_DESPAWN, 34000)
    if portal ~= nil then
        -- ArthasPortalEvent twin (3s): cast + MEDIUM_SNOW weather.
        schedule(guid, "arthasPortal", 3000, function()
            portal:CastSpell(portal, SPELL_ARTHAS_PORTAL, true)
            local mapObj = creature:GetMap()
            if mapObj ~= nil then
                mapObj:SetZoneWeather(AREA_TRIAL_OF_THE_CRUSADER, WEATHER_MEDIUM_SNOW, 0.75)
            end
        end)
    end
    schedule(guid, "summonLichKing", 5000, function()
        creature:SummonCreature(ENTRY_LICH_KING, LICH_KING_POS[1], LICH_KING_POS[2], LICH_KING_POS[3], LICH_KING_POS[4], SUMMON_TIMED_DESPAWN, 30000)
    end)
end

-- C++ npc_tirion_toc::DoAction (trial_of_the_crusader.cpp:392-510).
local function fordringDoAction(event, creature, action)
    local guid = creature:GetGUIDLow()
    local st = fordringState(creature)
    if action == ACTION_START_GORMOK then
        creature:Talk(0)
        schedule(guid, "gormokIntro", 24000, function()
            fordringGormokIntro(creature, guid)
        end)
    elseif action == ACTION_START_GORMOK_FAIL then
        schedule(guid, "gormokIntro", 1, function()
            fordringGormokIntro(creature, guid)
        end)
    elseif action == ACTION_START_JORMUNGARS then
        if st.jormungarsSummoned then
            return
        end
        st.jormungarsSummoned = true
        creature:Talk(2)
        openMainGate(creature)
        creature:SummonCreature(ENTRY_DREADSCALE, BEASTS_POS_DREADSCALE[1], BEASTS_POS_DREADSCALE[2], BEASTS_POS_DREADSCALE[3], BEASTS_POS_DREADSCALE[4])
        schedule(guid, "exclamationJormungars", 7000, function()
            creature:PerformEmote(5) -- C++ EVENT_EXCLAMATION: EMOTE_ONESHOT_EXCLAMATION
        end)
    elseif action == ACTION_START_ICEHOWL then
        if st.icehowlSummoned then
            return
        end
        st.icehowlSummoned = true
        creature:Talk(3)
        openMainGate(creature)
        creature:SummonCreature(ENTRY_ICEHOWL, BEASTS_POS_GORMOK[1], BEASTS_POS_GORMOK[2], BEASTS_POS_GORMOK[3], BEASTS_POS_GORMOK[4], SUMMON_DEAD_DESPAWN)
        schedule(guid, "exclamationIcehowl", 6000, function()
            creature:PerformEmote(5) -- C++ EVENT_EXCLAMATION: EMOTE_ONESHOT_EXCLAMATION
        end)
    elseif action == ACTION_NORTHREND_BEASTS_WIPE then
        st.jormungarsSummoned = false
        st.icehowlSummoned = false
        creature:Talk(5)
        schedule(guid, "summonBarrett", 13000, function()
            handleBarrettSummon(creature)
        end)
    elseif action == ACTION_NORTHREND_BEASTS_DEFEATED then
        creature:Talk(4)
        schedule(guid, "exclamationBeastsDefeated", 2000, function()
            creature:PerformEmote(5) -- C++ EVENT_EXCLAMATION: EMOTE_ONESHOT_EXCLAMATION
        end)
        schedule(guid, "summonBarrett", 6000, function()
            handleBarrettSummon(creature)
        end)
    elseif action == ACTION_START_JARAXXUS_EVENT then
        schedule(guid, "startCallWilfred", 1000, function()
            creature:Talk(6)
            schedule(guid, "summonWilfred", 7000, function()
                openMainGate(creature)
                creature:SummonCreature(ENTRY_FIZZLEBANG, WILFRED_POS[1], WILFRED_POS[2], WILFRED_POS[3], WILFRED_POS[4], SUMMON_CORPSE_TIMED_DESPAWN, 16000)
            end)
        end)
    elseif action == ACTION_KILL_JARAXXUS then
        schedule(guid, "killJaraxxus", 6000, function()
            creature:Talk(7)
            schedule(guid, "emoteSheathe", 2000, function()
                creature:PerformEmote(396) -- C++ EVENT_EMOTE_SHEATHE: EMOTE_ONESHOT_TALK_NO_SHEATHE
            end)
        end)
    elseif action == ACTION_JARAXXUS_DEFEATED then
        schedule(guid, "tirionLament", 7000, function()
            fordringTirionLament(creature, guid)
        end)
    elseif action == ACTION_JARAXXUS_WIPE then
        schedule(guid, "summonBarrett", 30000, function()
            handleBarrettSummon(creature)
        end)
    elseif action == ACTION_START_CHAMPIONS then
        creature:Talk(10)
        local team = deriveFactionLeader(creature)
        local otherData = team == DATA_VARIAN and DATA_GARROSH or DATA_VARIAN
        local otherLeader = creature:GetInstanceCreature(otherData)
        if otherLeader ~= nil then
            otherLeader:DoAction(ACTION_START_CHAMPIONS)
        end
        schedule(guid, "allowCombat", 26000, function()
            creature:Talk(11)
            schedule(guid, "summonChampions", 7000, function()
                creature:SummonCreature(ENTRY_CHAMPIONS_CONTROLLER, CHAMPIONS_CONTROLLER_POS[1], CHAMPIONS_CONTROLLER_POS[2], CHAMPIONS_CONTROLLER_POS[3], CHAMPIONS_CONTROLLER_POS[4])
                -- controller SetData(0, team) + EVENT_START_CHAMPIONS 3s
                -- SetData(1, 0) are PORTED tick 410 in
                -- boss_faction_champions.lua (controllerReset: the
                -- controller's own ev23 Reset is the SetData(0) twin,
                -- a 3s timer the SetData(1) twin); creature:SetData
                -- stays a dead ScriptData write and is not used.
            end)
        end)
    elseif action == ACTION_START_CHAMPIONS_ENGAGE then
        local team = deriveFactionLeader(creature)
        local otherData = team == DATA_VARIAN and DATA_GARROSH or DATA_VARIAN
        local otherLeader = creature:GetInstanceCreature(otherData)
        if otherLeader ~= nil then
            otherLeader:DoAction(ACTION_START_CHAMPIONS_ENGAGE)
        end
        schedule(guid, "summonChampions", 3000, function()
            creature:SummonCreature(ENTRY_CHAMPIONS_CONTROLLER, CHAMPIONS_CONTROLLER_POS[1], CHAMPIONS_CONTROLLER_POS[2], CHAMPIONS_CONTROLLER_POS[3], CHAMPIONS_CONTROLLER_POS[4])
        end)
    elseif action == ACTION_CHAMPIONS_DEFEATED then
        schedule(guid, "tragicVictory", 7000, function()
            creature:Talk(12)
            schedule(guid, "summonBarrett", 24000, function()
                handleBarrettSummon(creature)
            end)
        end)
    elseif action == ACTION_FACTION_WIPE then
        schedule(guid, "summonBarrett", 4000, function()
            handleBarrettSummon(creature)
        end)
    elseif action == ACTION_SUMMON_JARAXXUS then
        creature:SummonCreature(ENTRY_JARAXXUS, JARAXXUS_POS[1], JARAXXUS_POS[2], JARAXXUS_POS[3], JARAXXUS_POS[4])
    elseif action == ACTION_START_VALKYR then
        creature:Talk(13)
        schedule(guid, "summonValkyr", 17000, function()
            creature:SummonCreatureGroup(1) -- C++ EVENT_SUMMON_VALKYR: GROUP_VALKYR
            schedule(guid, "openGate", 3000, function()
                openMainGate(creature)
            end)
        end)
    elseif action == ACTION_START_VALKYR_ENGAGE then
        creature:Talk(14)
        schedule(guid, "summonValkyr", 5000, function()
            creature:SummonCreatureGroup(1) -- C++ EVENT_SUMMON_VALKYR: GROUP_VALKYR
            schedule(guid, "openGate", 3000, function()
                openMainGate(creature)
            end)
        end)
    elseif action == ACTION_VALKYR_WIPE then
        schedule(guid, "summonBarrett", 6000, function()
            handleBarrettSummon(creature)
        end)
        despawnEssences(creature)
    elseif action == ACTION_VALKYR_DEFEATED then
        schedule(guid, "summonBarrett", 4000, function()
            handleBarrettSummon(creature)
        end)
        local leader = creature:GetInstanceCreature(st.leader)
        if leader ~= nil then
            leader:DoAction(ACTION_VALKYR_DEFEATED)
        end
        despawnEssences(creature)
    elseif action == ACTION_START_LK_EVENT then
        creature:Talk(15)
        local mapObj = creature:GetMap()
        if mapObj ~= nil then
            mapObj:SetZoneWeather(AREA_TRIAL_OF_THE_CRUSADER, WEATHER_LIGHT_SNOW, 0.5)
        end
        schedule(guid, "lichKingChallenge", 19000, function()
            fordringLichKingChallenge(creature, guid)
        end)
        schedule(guid, "sayArthas", 26000, function()
            creature:Talk(16)
        end)
    elseif action == ACTION_LK_EVENT_FINISHED then
        schedule(guid, "lichKingSouls", 2000, function()
            local voice = creature:GetInstanceCreature(DATA_LICH_KING_VOICE)
            if voice ~= nil then
                voice:Talk(5)
            end
        end)
    end
end

-- C++ npc_tirion_toc::Reset (trial_of_the_crusader.cpp:355-362).
local function fordringReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    local st = fordringState(creature)
    st.leader = deriveFactionLeader(creature)
    st.jormungarsSummoned = false
    st.icehowlSummoned = false
    handleBarrettSummon(creature)
end

RegisterCreatureEvent(ENTRY_FORDRING, 23, fordringReset)
RegisterCreatureEvent(ENTRY_FORDRING, 38, fordringDoAction)

-- C++ npc_barrett_toc::SendActionToTirion (trial_of_the_crusader.cpp:
-- 241-247): Fordring DoAction + gossip-flag removal + spline-chain
-- walk to the despawn point (ev28 below despawns on arrival).
local function sendActionToTirion(creature, player, action)
    sendToFordring(creature, action)
    creature:RemoveFlag(82, 1)
    creature:MoveAlongSplineChain(POINT_BARRETT_DESPAWN, SPLINE_INITIAL_MOVEMENT, false)
    player:GossipComplete()
end

-- C++ npc_barrett_toc::OnGossipSelect (trial_of_the_crusader.cpp:
-- 249-290). The menuId branch is recovered per-entry (each Barrett
-- entry carries exactly one menu — the hook never sees menuId); the
-- menu ids (10600 / 10610 / 10687 / 10688 / 10693) are noted per
-- branch below.
local function barrettGossipSelect(event, player, creature, sender, action, code, listID)
    local entry = creature:GetEntry()
    if entry == ENTRY_BARRETT_BEASTS or entry == ENTRY_BARRETT_BEASTS_HC then
        -- MENUID_NORTHREND_BEASTS 10600.
        if listID == GOSSIPID_FAIL then
            sendActionToTirion(creature, player, ACTION_START_GORMOK_FAIL)
        else
            sendActionToTirion(creature, player, ACTION_START_GORMOK)
        end
    elseif entry == ENTRY_BARRETT_JARAXXUS then
        -- MENUID_JARAXXUS 10610. The fail branch does NOT call
        -- SendActionToTirion: Jaraxxus DoAction(21) directly, no
        -- gossip-flag removal, then the same spline walk + close.
        if listID == GOSSIPID_FAIL then
            local jaraxxus = creature:GetInstanceCreature(DATA_JARAXXUS)
            if jaraxxus ~= nil then
                jaraxxus:DoAction(21)
            end
            creature:MoveAlongSplineChain(POINT_BARRETT_DESPAWN, SPLINE_INITIAL_MOVEMENT, false)
            player:GossipComplete()
        else
            sendActionToTirion(creature, player, ACTION_START_JARAXXUS_EVENT)
        end
    elseif entry == ENTRY_BARRETT_FACTION then
        -- MENUID_FACTION_CHAMPIONS 10687.
        if listID == GOSSIPID_FAIL then
            sendActionToTirion(creature, player, ACTION_START_CHAMPIONS_ENGAGE)
        else
            sendActionToTirion(creature, player, ACTION_START_CHAMPIONS)
        end
    elseif entry == ENTRY_BARRETT_VALKYR then
        -- MENUID_VALKYR 10688.
        if listID == GOSSIPID_FAIL then
            sendActionToTirion(creature, player, ACTION_START_VALKYR_ENGAGE)
        else
            sendActionToTirion(creature, player, ACTION_START_VALKYR)
        end
    elseif entry == ENTRY_BARRETT_LK then
        -- MENUID_LK 10693: no fail option in C++.
        sendActionToTirion(creature, player, ACTION_START_LK_EVENT)
    end
end

-- C++ npc_barrett_toc::MovementInform (trial_of_the_crusader.cpp:
-- 292-299): spline-chain arrival at the despawn point.
local function barrettMovementInform(event, creature, type, id)
    if type == SPLINE_CHAIN_MOTION_TYPE and id == POINT_BARRETT_DESPAWN then
        creature:DespawnOrUnsummon()
    end
end

local BARRETT_ENTRIES = {
    ENTRY_BARRETT_BEASTS, ENTRY_BARRETT_BEASTS_HC, ENTRY_BARRETT_JARAXXUS,
    ENTRY_BARRETT_FACTION, ENTRY_BARRETT_VALKYR, ENTRY_BARRETT_LK,
}
for _, entry in ipairs(BARRETT_ENTRIES) do
    RegisterCreatureGossipEvent(entry, 2, barrettGossipSelect)
    RegisterCreatureEvent(entry, 28, barrettMovementInform)
end

-- C++ npc_garrosh_toc / npc_varian_toc DoAction + UpdateAI
-- (trial_of_the_crusader.cpp:684-824). Talk ids are shared numbering
-- (0 beasts / 1 pigs-or-dogs / 2 demand-justice / 3 no-mercy-or-glory /
-- 4 faction-dead / 5 valkyr-dead / 6 killed); delays differ per leader.
local function leaderDoAction(event, creature, action)
    local guid = creature:GetGUIDLow()
    local isVarian = creature:GetEntry() == ENTRY_VARIAN
    if action == ACTION_START_GORMOK then
        schedule(guid, "gormokIntro", isVarian and 11000 or 12000, function()
            creature:Talk(0)
        end)
    elseif action == ACTION_JARAXXUS_DEFEATED then
        schedule(guid, "jaraxxusDefeated", isVarian and 24000 or 14000, function()
            creature:Talk(1)
        end)
    elseif action == ACTION_START_CHAMPIONS then
        schedule(guid, "demandJustice", isVarian and 9000 or 8000, function()
            creature:Talk(2)
            schedule(guid, "noMercy", isVarian and 20000 or 21000, function()
                creature:Talk(3)
            end)
        end)
    elseif action == 18 then
        -- ACTION_SAY_KILLED_PLAYER (no Fordring case; leader's own).
        creature:Talk(6)
    elseif action == ACTION_CHAMPIONS_DEFEATED then
        creature:Talk(4)
    elseif action == ACTION_VALKYR_DEFEATED then
        schedule(guid, "valkyrDead", isVarian and 6000 or 5000, function()
            creature:Talk(5)
        end)
    elseif action == ACTION_START_CHAMPIONS_ENGAGE then
        schedule(guid, "noMercy", 6000, function()
            creature:Talk(3)
        end)
    end
end

local function leaderReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

RegisterCreatureEvent(ENTRY_VARIAN, 23, leaderReset)
RegisterCreatureEvent(ENTRY_VARIAN, 38, leaderDoAction)
RegisterCreatureEvent(ENTRY_GARROSH, 23, leaderReset)
RegisterCreatureEvent(ENTRY_GARROSH, 38, leaderDoAction)

-- C++ npc_fizzlebang_toc (trial_of_the_crusader.cpp:602-682). The
-- emote-state legs (EVENT_EMOTE_TALK / EVENT_REMOVE_EMOTE_TALK) PORTED
-- 2026-11-14 via creature:EmoteState (lua_creature_events.go:4798).
local function fizzlebangLastTalk(creature, guid)
    creature:Talk(3)
    sendToFordring(creature, ACTION_KILL_JARAXXUS)
end

local function fizzlebangSetTarget(creature, guid)
    local jaraxxus = creature:GetInstanceCreature(DATA_JARAXXUS)
    if jaraxxus ~= nil then
        creature:SetTarget(jaraxxus:GetGUID())
    end
    -- EVENT_EMOTE_SHEATHE 6s: emote, then schedules LAST_TALK 9s.
    schedule(guid, "emoteSheathe", 6000, function()
        creature:PerformEmote(396) -- C++ EVENT_EMOTE_SHEATHE: EMOTE_ONESHOT_TALK_NO_SHEATHE
        schedule(guid, "lastTalk", 9000, function()
            fizzlebangLastTalk(creature, guid)
        end)
    end)
end

local function fizzlebangOblivion(creature, guid)
    creature:SummonCreature(ENTRY_WILFRED_PORTAL, PORTAL_TARGET_POS[1], PORTAL_TARGET_POS[2], PORTAL_TARGET_POS[3], PORTAL_TARGET_POS[4])
    creature:SummonCreature(ENTRY_PURPLE_GROUND, PURPLE_GROUND_POS[1], PURPLE_GROUND_POS[2], PURPLE_GROUND_POS[3], PURPLE_GROUND_POS[4], SUMMON_TIMED_DESPAWN, 16000)
    creature:Talk(1)
    creature:CastSpell(creature, SPELL_OPEN_PORTAL)
    schedule(guid, "summonJaraxxus", 11000, function()
        sendToFordring(creature, ACTION_SUMMON_JARAXXUS)
        creature:Talk(2)
        schedule(guid, "emoteTalk", 2000, function()
            creature:EmoteState(378) -- C++ EVENT_EMOTE_TALK: EMOTE_STATE_TALK
        end)
        schedule(guid, "removeEmoteTalk", 7000, function()
            creature:EmoteState(0) -- C++ EVENT_REMOVE_EMOTE_TALK: EMOTE_ONESHOT_NONE
        end)
        schedule(guid, "setTarget", 4000, function()
            fizzlebangSetTarget(creature, guid)
        end)
    end)
end

local function fizzlebangReset(event, creature)
    creature:SetReactState(REACT_PASSIVE)
    cancelTimers(creature:GetGUIDLow())
    schedule(creature:GetGUIDLow(), "startMove", 1000, function()
        creature:MoveAlongSplineChain(POINT_SUMMON, SPLINE_INITIAL_MOVEMENT, true)
    end)
end

local function fizzlebangMovementInform(event, creature, type, id)
    if type == SPLINE_CHAIN_MOTION_TYPE and id == POINT_SUMMON then
        openMainGate(creature)
        creature:Talk(0)
        schedule(creature:GetGUIDLow(), "emoteTalk", 2000, function()
            creature:EmoteState(378) -- C++ EVENT_EMOTE_TALK: EMOTE_STATE_TALK
        end)
        schedule(creature:GetGUIDLow(), "removeEmoteTalk", 9000, function()
            creature:EmoteState(0) -- C++ EVENT_REMOVE_EMOTE_TALK: EMOTE_ONESHOT_NONE
        end)
        schedule(creature:GetGUIDLow(), "oblivion", 11000, function()
            fizzlebangOblivion(creature, creature:GetGUIDLow())
        end)
    end
end

RegisterCreatureEvent(ENTRY_FIZZLEBANG, 23, fizzlebangReset)
RegisterCreatureEvent(ENTRY_FIZZLEBANG, 28, fizzlebangMovementInform)

-- Wilfred portal-target arms removed 2026-11-09: entry 17965 ev23/ev14 are
-- registered by the canonical port lua_scripts/northrend/npc_open_portal_target_toc.lua;
-- two registrations double-fired every reset/spell-hit.

-- C++ boss_lich_king_toc (trial_of_the_crusader.cpp:303-353). The
-- emote-state legs (EVENT_EMOTE_TALK's SetUInt32Value + EXCLAMATION /
-- KNEEL) PORTED 2026-11-14 via EmoteState/PerformEmote — and EMOTE_TALK's
-- SetZoneWeather(FOG) leg IS ported, and BREAK_PLATFORM's floor
-- SetDestructibleState leg stays out (no GO destructible-state bridge).
local function lichKingReset(event, creature)
    creature:SetReactState(REACT_PASSIVE)
    creature:SetBossState(DATA_LICH_KING, 1)
    cancelTimers(creature:GetGUIDLow())
    schedule(creature:GetGUIDLow(), "startMove", 1000, function()
        creature:MoveAlongSplineChain(POINT_MIDDLE, SPLINE_INITIAL_MOVEMENT, true)
    end)
end

local function lichKingMovementInform(event, creature, type, id)
    if type == SPLINE_CHAIN_MOTION_TYPE and id == POINT_MIDDLE then
        schedule(creature:GetGUIDLow(), "startTalk", 4000, function()
            creature:Talk(0)
            schedule(creature:GetGUIDLow(), "emoteTalk", 4000, function()
                creature:EmoteState(378) -- C++ EVENT_EMOTE_TALK: EMOTE_STATE_TALK
                local mapObj = creature:GetMap()
                if mapObj ~= nil then
                    mapObj:SetZoneWeather(AREA_TRIAL_OF_THE_CRUSADER, WEATHER_FOG, 0.0)
                end
                schedule(creature:GetGUIDLow(), "emoteExclamation", 10000, function()
                    creature:PerformEmote(5) -- C++ EVENT_EMOTE_EXCLAMATION: EMOTE_ONESHOT_EXCLAMATION
                    schedule(creature:GetGUIDLow(), "emoteKneel", 3000, function()
                        creature:PerformEmote(16) -- C++ EVENT_EMOTE_KNEEL: EMOTE_ONESHOT_KNEEL
                    end)
                end)
            end)
            schedule(creature:GetGUIDLow(), "breakPlatform", 18000, function()
                creature:CastSpell(creature, SPELL_LK_FROST_NOVA, true)
                creature:CastSpell(creature, SPELL_CORPSE_TELEPORT, true)
                creature:CastSpell(creature, SPELL_DESTROY_FLOOR_KNOCKUP, true)
                sendToFordring(creature, ACTION_LK_EVENT_FINISHED)
                -- Floor SetDestructibleState stays out (header note).
                creature:SetBossState(DATA_LICH_KING, BOSS_STATE_DONE)
            end)
        end)
    end
end

RegisterCreatureEvent(ENTRY_LICH_KING, 23, lichKingReset)
RegisterCreatureEvent(ENTRY_LICH_KING, 28, lichKingMovementInform)
