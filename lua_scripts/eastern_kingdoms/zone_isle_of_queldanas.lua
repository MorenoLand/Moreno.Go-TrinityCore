-- Thalorien Dawnseeker (Isle of Quel'Danas) — Lua port of
-- src/server/scripts/EasternKingdoms/zone_isle_of_queldanas.cpp
-- (npc_thalorien_dawnseeker, the only AI AddSC_isle_of_queldanas()
-- registers). Entry: NPC_THALORIEN_DAWNSEEKER = 37205
-- (zone_isle_of_queldanas.cpp:90). The gossip NPC `me` is 37205 itself
-- (the fallen Thalorien whose remains the player examines); DoAction
-- summons a second 37205, the living Thalorien, plus the defenders.
-- No boss registration: this is a world-event NPC, not an instance boss
-- (no RegisterLuaBoss; Lua files load via runtime.go WalkDir).
-- Talk: SAY_THALORIEN_1..10 = 0..9 on the summoned Thalorien,
-- SAY_MORLEN_1..5 = 0..4 on Morlen Goldgrip (37542).
-- Event machine (zone_isle_of_queldanas.cpp:181-372, UpdateAI): a linear
-- one-shot chain, ported 1:1 below via CreateLuaEvent with a per-GUID
-- generation guard (nefarian pattern) — Reset cancels the chain.
-- C++ DoAction(ACTION_START_QUEST = 1) (:140-150): RemoveFlag gossip,
-- SummonCreature(37205, thalorienSummon, TEMPSUMMON_CORPSE_DESPAWN),
-- SummonCreatureGroup(SUMMON_SUNWELL_DEFENDER = 1), INTRO_0 3s.
-- INTRO_0..3 (:194-214): ThalorienTalk 0,1,2,3 at 3s/5s/6s/7s.
-- SALUTE (:215-221): every summon PerformEmote(EMOTE_ONESHOT_SALUTE 66);
-- DEFENDERS_RUN 3s. DEFENDERS_RUN (:222-244): RUN_2 3s + THALORIEN_GO 2s;
-- defenders 1-5 SetFacingTo(5.67232), defenders 6-10 MovePoint(0,
-- defendersRun[5..9]). DEFENDERS_RUN_2 (:245-261): defenders 1-9
-- MovePoint(0, defendersRun[1..9]); defender 10 stands (C++-exact).
-- THALORIEN_GO (:262-272): Thalorien SetWalk(true) + MovePoint(0,
-- thalorienFight) + EmoteState(EMOTE_STATE_READY2H 375) +
-- SetHomePosition(thalorienFight); INTRO_4 4s. INTRO_4 (:273-279):
-- Talk 4; SUMMON_MORLEN 6s + INTRO_5 9s. INTRO_5 (:280-285): Talk 5;
-- MORLEN_1 6s. SUMMON_MORLEN (:286-293): SummonCreature(37542,
-- morlenSummon, CORPSE_DESPAWN) + Morlen DoCastSelf(BLOOD_PRESENCE
-- 50689). MORLEN_1 (:294-299): MorlenTalk 0; SPAWN_WAVE_1 7s.
-- SPAWN_WAVE_1/2/3 (:300-320): MorlenTalk 1/2/3 +
-- SummonCreatureGroup(2/3/4: zombies/ghouls/crypt raiders) +
-- WAVE_ATTACK 3s. WAVE_ATTACK (:321-334): every summon except
-- Thalorien/Morlen gets RemoveFlag(UNIT_FIELD_FLAGS 59,
-- IMMUNE_TO_PC 0x100 | IMMUNE_TO_NPC 0x200) + AttackStart(thalorien)
-- — the defenders ride the same arm in C++, ported as-is.
-- MORLEN_2 (:335-340): MorlenTalk 4; MORLEN_ATTACK 1s. MORLEN_ATTACK
-- (:341-349): Morlen RemoveFlag(59, 0x300) + AttackStart(thalorien).
-- OUTRO_1 (:350-358): Thalorien DoCast(player, KILL_CREDIT 70265);
-- Talk 6; OUTRO_2 5s. OUTRO_2 (:359-363): Talk 7; OUTRO_3 7s.
-- OUTRO_3 (:364-371): Thalorien DoCastSelf(POLYMORPH_VISUAL 27123);
-- Talk 8; OUTRO_4 5s. OUTRO_4 (:372-377): Talk 9; KNEEL 6s.
-- KNEEL (:378-387): Thalorien SetStandState(UNIT_STAND_STATE_KNEEL 8)
-- + DespawnOrUnsummon(5000); `me` SetFlag gossip.
-- SummonedCreatureDespawn (:390-405): after removing the summon, size 2
-- + zombie/ghoul/crypt-raider despawn -> WAVE_2/WAVE_3/MORLEN_2 1s;
-- size 1 + Morlen despawn -> OUTRO_1 2s. (Size 2 means only Thalorien
-- + Morlen remain — the defenders are gone by then via their DB
-- summonTime; the check is ported verbatim, same DB rows.)
-- JustSummoned (:388-391): every summon joins the list, in summon
-- order — the list is an insertion-ordered array (not a pairs() map)
-- so the DEFENDERS_RUN 1-5/6-10 split matches C++ SummonList order.
-- Gossip: C++ OnGossipSelect (:128-137) gates on menuId ==
-- MENU_EXAMINE_REMAINS (10908). menuId is NOT forwarded to Lua
-- (gossip.go: "menuID is intentionally not forwarded: the post-hook
-- stale-menu gate below already enforces it") — the select handler
-- below starts the quest ungated, the engine gate enforcing the live
-- menu, the in-tree convention. No OnGossipHello is registered: C++
-- has none, the DB menu 10908 shows by default. C++ returns false
-- from OnGossipSelect; the handler returns false likewise (Eluna
-- gossip polarity, gossip.go). The gossip flag is removed in DoAction,
-- so the engine's flag gate rejects re-selects mid-event, exactly
-- like C++.
-- SummonType note: C++ passes TEMPSUMMON_CORPSE_DESPAWN; this fork's
-- ObjectDefines.h numbers it 5 (1-based), but Go's summonTempCreature
-- and every Lua port use the 0-based upstream numbering
-- (CORPSE_DESPAWN = 4 — mandokir precedent), so 4 is passed here.
-- Reset (ev23): C++ Reset (:152-159) sets the gossip flag, despawns
-- all summons, clears GUIDs/events. State is cleared BEFORE the
-- despawns so the ev20 arms find no state and cannot re-schedule a
-- wave mid-reset. ev23 fires on the respawn/evade paths
-- (fireCreatureSpawned fires ev23 then ev5); there is no Go fire site
-- for a fresh spawn's first grid activation, so a fresh spawn rides
-- the template NPCFlags (gossip flag) with empty state — same
-- observable behavior. The summoned Thalorien shares entry 37205, so
-- its own Reset/ev19/ev20 handlers fire too, on its own per-GUID
-- state, exactly like the shared C++ AI struct.
-- Ported 2026-11-10: full event machine (gossip select, all 23
-- UpdateAI events, JustSummoned/SummonedCreatureDespawn tracking).

local ENTRY_THALORIEN = 37205
local ENTRY_SUNWELL_DEFENDER = 37211
local ENTRY_MORLEN = 37542
local ENTRY_SCOURGE_ZOMBIE = 37538
local ENTRY_GHOUL_INVADER = 37539
local ENTRY_CRYPT_RAIDER = 37541

local MENU_EXAMINE_REMAINS = 10908

local SPELL_BLOOD_PRESENCE = 50689
local SPELL_KILL_CREDIT = 70265
local SPELL_POLYMORPH_VISUAL = 27123

local SUMMON_CORPSE_DESPAWN = 4 -- Go 0-based TempSummonType (see header)

local UNIT_NPC_FLAGS_FIELD = 82 -- grizzly_hills precedent
local UNIT_NPC_FLAG_GOSSIP = 1
local UNIT_FIELD_FLAGS = 59 -- arlokk precedent
local UNIT_FLAG_IMMUNE_TO_PC = 0x100
local UNIT_FLAG_IMMUNE_TO_NPC = 0x200

local UNIT_STAND_STATE_KNEEL = 8 -- UnitDefines.h:41
local EMOTE_ONESHOT_SALUTE = 66 -- SharedDefines.h:1985; taskmaster_fizzule precedent
local EMOTE_STATE_READY2H = 375 -- SharedDefines.h:2013; thrall_herald_of_war precedent

local DEFENDER_COUNT = 10
local DEFENDER_ORIENTATION = 5.67232

local THALORIEN_SUMMON = { 11795.32, -7070.476, 26.27511, 5.67232 }
local THALORIEN_FIGHT = { 11788.46, -7063.375, 25.79677, 3.054326 }
local MORLEN_SUMMON = { 11766.46, -7050.078, 26.19846, 5.637414 }

local DEFENDERS_RUN = {
    { 11934.78, -7064.424, 33.38046 },
    { 11927.9, -7061.303, 32.00986 },
    { 11927.81, -7067.289, 31.81058 },
    { 11927.89, -7058.299, 32.13486 },
    { 11927.9, -7061.303, 32.00986 },
    { 11934.78, -7064.424, 33.38046 },
    { 11920.69, -7061.134, 30.40684 },
    { 11920.56, -7067.133, 30.41605 },
    { 11920.76, -7058.135, 30.61656 },
    { 11920.5, -7070.132, 30.40836 },
}

local EVENT_INTRO_0 = 1
local EVENT_INTRO_1 = 2
local EVENT_INTRO_2 = 3
local EVENT_INTRO_3 = 4
local EVENT_SALUTE = 5
local EVENT_DEFENDERS_RUN = 6
local EVENT_DEFENDERS_RUN_2 = 7
local EVENT_THALORIEN_GO = 8
local EVENT_INTRO_4 = 9
local EVENT_SUMMON_MORLEN = 10
local EVENT_INTRO_5 = 11
local EVENT_MORLEN_1 = 12
local EVENT_SPAWN_WAVE_1 = 13
local EVENT_SPAWN_WAVE_2 = 14
local EVENT_SPAWN_WAVE_3 = 15
local EVENT_WAVE_ATTACK = 16
local EVENT_MORLEN_2 = 17
local EVENT_MORLEN_ATTACK = 18
local EVENT_OUTRO_1 = 19
local EVENT_OUTRO_2 = 20
local EVENT_OUTRO_3 = 21
local EVENT_OUTRO_4 = 22
local EVENT_KNEEL = 23

local GROUP_DEFENDERS = 1
local GROUP_ZOMBIES = 2
local GROUP_GHOULS = 3
local GROUP_CRYPT_RAIDERS = 4

local states = {}
-- GUID storage split: creature GUIDs are stored LOW (GetCreatureByGUID
-- masks the low 24 bits — lua_creature_events.go); the player GUID is
-- stored FULL because GetPlayerByGUID compares full equality
-- (globals.go; commander_dawnforge precedent).

local function freshState(gen)
    return { gen = gen, timers = {}, summons = {}, playerGuid = nil, thalorienGuid = nil, morlenGuid = nil }
end

local function getState(creature)
    local guid = creature:GetGUIDLow()
    local st = states[guid]
    if not st then
        st = freshState(0)
        states[guid] = st
    end
    return guid, st
end

local function cancelTimers(guid)
    local st = states[guid]
    if st then
        for _, id in ipairs(st.timers) do
            RemoveEventById(id)
        end
        st.timers = {}
    end
end

local runEvent -- forward declare (tharon_ja/gal_darah idiom): schedule's closure binds it

local function schedule(creature, guid, eventId, delayMs)
    local st = states[guid]
    if not st then
        return
    end
    local gen = st.gen
    local id = CreateLuaEvent(function()
        local cur = states[guid]
        if cur and cur.gen == gen then
            runEvent(creature, guid, eventId)
        end
    end, delayMs)
    st.timers[#st.timers + 1] = id
end

local function thalorienCreature(creature, st)
    if not st.thalorienGuid then
        return nil
    end
    return creature:GetCreatureByGUID(st.thalorienGuid)
end

local function morlenCreature(creature, st)
    if not st.morlenGuid then
        return nil
    end
    return creature:GetCreatureByGUID(st.morlenGuid)
end

local function thalorienTalk(creature, st, id)
    local thalorien = thalorienCreature(creature, st)
    if thalorien then
        thalorien:Talk(id)
    end
end

local function morlenTalk(creature, st, id)
    local morlen = morlenCreature(creature, st)
    if morlen then
        morlen:Talk(id)
    end
end

local function defendersInOrder(creature, st)
    local out = {}
    for _, entry in ipairs(st.summons) do
        if entry.guid then
            local summon = creature:GetCreatureByGUID(entry.guid)
            if summon and summon:GetEntry() == ENTRY_SUNWELL_DEFENDER then
                out[#out + 1] = summon
            end
        end
    end
    return out
end

runEvent = function(creature, guid, eventId)
    local st = states[guid]
    if not st then
        return
    end
    if eventId == EVENT_INTRO_0 then
        thalorienTalk(creature, st, 0)
        schedule(creature, guid, EVENT_INTRO_1, 5000)
    elseif eventId == EVENT_INTRO_1 then
        thalorienTalk(creature, st, 1)
        schedule(creature, guid, EVENT_INTRO_2, 6000)
    elseif eventId == EVENT_INTRO_2 then
        thalorienTalk(creature, st, 2)
        schedule(creature, guid, EVENT_INTRO_3, 7000)
    elseif eventId == EVENT_INTRO_3 then
        thalorienTalk(creature, st, 3)
        schedule(creature, guid, EVENT_SALUTE, 4000)
    elseif eventId == EVENT_SALUTE then
        for _, entry in ipairs(st.summons) do
            local summon = creature:GetCreatureByGUID(entry.guid)
            if summon then
                summon:PerformEmote(EMOTE_ONESHOT_SALUTE)
            end
        end
        schedule(creature, guid, EVENT_DEFENDERS_RUN, 3000)
    elseif eventId == EVENT_DEFENDERS_RUN then
        schedule(creature, guid, EVENT_DEFENDERS_RUN_2, 3000)
        schedule(creature, guid, EVENT_THALORIEN_GO, 2000)
        local defenders = defendersInOrder(creature, st)
        for i, defender in ipairs(defenders) do
            if i <= 5 then
                defender:SetFacingTo(DEFENDER_ORIENTATION)
            elseif i - 1 < DEFENDER_COUNT then
                local pos = DEFENDERS_RUN[i]
                defender:MoveTo(0, pos[1], pos[2], pos[3])
            end
        end
    elseif eventId == EVENT_DEFENDERS_RUN_2 then
        local defenders = defendersInOrder(creature, st)
        for i, defender in ipairs(defenders) do
            if i < DEFENDER_COUNT then
                local pos = DEFENDERS_RUN[i + 1]
                defender:MoveTo(0, pos[1], pos[2], pos[3])
            end
        end
    elseif eventId == EVENT_THALORIEN_GO then
        local thalorien = thalorienCreature(creature, st)
        if thalorien then
            thalorien:SetWalk(true)
            thalorien:MoveTo(0, THALORIEN_FIGHT[1], THALORIEN_FIGHT[2], THALORIEN_FIGHT[3])
            thalorien:EmoteState(EMOTE_STATE_READY2H)
            thalorien:SetHomePosition(THALORIEN_FIGHT[1], THALORIEN_FIGHT[2], THALORIEN_FIGHT[3], THALORIEN_FIGHT[4])
        end
        schedule(creature, guid, EVENT_INTRO_4, 4000)
    elseif eventId == EVENT_INTRO_4 then
        thalorienTalk(creature, st, 4)
        schedule(creature, guid, EVENT_SUMMON_MORLEN, 6000)
        schedule(creature, guid, EVENT_INTRO_5, 9000)
    elseif eventId == EVENT_INTRO_5 then
        thalorienTalk(creature, st, 5)
        schedule(creature, guid, EVENT_MORLEN_1, 6000)
    elseif eventId == EVENT_SUMMON_MORLEN then
        local morlen = creature:SummonCreature(ENTRY_MORLEN, MORLEN_SUMMON[1], MORLEN_SUMMON[2], MORLEN_SUMMON[3], MORLEN_SUMMON[4], SUMMON_CORPSE_DESPAWN)
        if morlen then
            st.morlenGuid = morlen:GetGUIDLow()
            morlen:CastSpell(morlen, SPELL_BLOOD_PRESENCE)
        end
    elseif eventId == EVENT_MORLEN_1 then
        morlenTalk(creature, st, 0)
        schedule(creature, guid, EVENT_SPAWN_WAVE_1, 7000)
    elseif eventId == EVENT_SPAWN_WAVE_1 then
        morlenTalk(creature, st, 1)
        creature:SummonCreatureGroup(GROUP_ZOMBIES)
        schedule(creature, guid, EVENT_WAVE_ATTACK, 3000)
    elseif eventId == EVENT_SPAWN_WAVE_2 then
        morlenTalk(creature, st, 2)
        creature:SummonCreatureGroup(GROUP_GHOULS)
        schedule(creature, guid, EVENT_WAVE_ATTACK, 3000)
    elseif eventId == EVENT_SPAWN_WAVE_3 then
        morlenTalk(creature, st, 3)
        creature:SummonCreatureGroup(GROUP_CRYPT_RAIDERS)
        schedule(creature, guid, EVENT_WAVE_ATTACK, 3000)
    elseif eventId == EVENT_WAVE_ATTACK then
        local thalorien = thalorienCreature(creature, st)
        if thalorien then
            for _, entry in ipairs(st.summons) do
                local summon = creature:GetCreatureByGUID(entry.guid)
                if summon then
                    local summonEntry = summon:GetEntry()
                    if summonEntry ~= ENTRY_THALORIEN and summonEntry ~= ENTRY_MORLEN then
                        summon:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC + UNIT_FLAG_IMMUNE_TO_NPC)
                        summon:AttackStart(thalorien)
                    end
                end
            end
        end
    elseif eventId == EVENT_MORLEN_2 then
        morlenTalk(creature, st, 4)
        schedule(creature, guid, EVENT_MORLEN_ATTACK, 1000)
    elseif eventId == EVENT_MORLEN_ATTACK then
        local morlen = morlenCreature(creature, st)
        local thalorien = thalorienCreature(creature, st)
        if morlen and thalorien then
            morlen:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC + UNIT_FLAG_IMMUNE_TO_NPC)
            morlen:AttackStart(thalorien)
        end
    elseif eventId == EVENT_OUTRO_1 then
        local thalorien = thalorienCreature(creature, st)
        if thalorien and st.playerGuid then
            local player = GetPlayerByGUID(st.playerGuid)
            if player then
                thalorien:CastSpell(player, SPELL_KILL_CREDIT)
            end
        end
        thalorienTalk(creature, st, 6)
        schedule(creature, guid, EVENT_OUTRO_2, 5000)
    elseif eventId == EVENT_OUTRO_2 then
        thalorienTalk(creature, st, 7)
        schedule(creature, guid, EVENT_OUTRO_3, 7000)
    elseif eventId == EVENT_OUTRO_3 then
        local thalorien = thalorienCreature(creature, st)
        if thalorien then
            thalorien:CastSpell(thalorien, SPELL_POLYMORPH_VISUAL)
        end
        thalorienTalk(creature, st, 8)
        schedule(creature, guid, EVENT_OUTRO_4, 5000)
    elseif eventId == EVENT_OUTRO_4 then
        thalorienTalk(creature, st, 9)
        schedule(creature, guid, EVENT_KNEEL, 6000)
    elseif eventId == EVENT_KNEEL then
        local thalorien = thalorienCreature(creature, st)
        if thalorien then
            thalorien:SetStandState(UNIT_STAND_STATE_KNEEL)
            thalorien:DespawnOrUnsummon(5000)
        end
        creature:SetFlag(UNIT_NPC_FLAGS_FIELD, UNIT_NPC_FLAG_GOSSIP)
    end
end

local function startQuest(creature, player)
    local guid, st = getState(creature)
    creature:RemoveFlag(UNIT_NPC_FLAGS_FIELD, UNIT_NPC_FLAG_GOSSIP)
    local thalorien = creature:SummonCreature(ENTRY_THALORIEN, THALORIEN_SUMMON[1], THALORIEN_SUMMON[2], THALORIEN_SUMMON[3], THALORIEN_SUMMON[4], SUMMON_CORPSE_DESPAWN)
    if thalorien then
        st.thalorienGuid = thalorien:GetGUIDLow()
    end
    creature:SummonCreatureGroup(GROUP_DEFENDERS)
    st.playerGuid = player:GetGUID()
    schedule(creature, guid, EVENT_INTRO_0, 3000)
end

-- C++ OnGossipSelect (zone_isle_of_queldanas.cpp:128-137): menuId ==
-- MENU_EXAMINE_REMAINS gate rides the engine stale-menu gate (see
-- header); CloseGossipMenuFor + DoAction(ACTION_START_QUEST).
local function onGossipSelect(_, player, creature)
    player:GossipClearMenu()
    player:GossipComplete()
    startQuest(creature, player)
    return false
end

-- C++ Reset (zone_isle_of_queldanas.cpp:152-159): gossip flag on,
-- despawn all summons, clear GUIDs/events. State is cleared BEFORE the
-- despawns so ev20 arms find no state (see header).
local function onReset(_, creature)
    local guid, st = getState(creature)
    local gen = st.gen + 1
    cancelTimers(guid)
    local summons = st.summons
    states[guid] = freshState(gen)
    creature:SetFlag(UNIT_NPC_FLAGS_FIELD, UNIT_NPC_FLAG_GOSSIP)
    for _, entry in ipairs(summons) do
        local summon = creature:GetCreatureByGUID(entry.guid)
        if summon then
            summon:DespawnOrUnsummon(0)
        end
    end
end

-- C++ JustSummoned (zone_isle_of_queldanas.cpp:388-391): track the
-- summon, insertion order (see header).
local function onJustSummoned(_, creature, summon)
    local _, st = getState(creature)
    st.summons[#st.summons + 1] = { guid = summon:GetGUIDLow(), entry = summon:GetEntry() }
end

-- C++ SummonedCreatureDespawn (zone_isle_of_queldanas.cpp:392-405):
-- remove, then size-2 wave advance by despawned entry, size-1 +
-- Morlen -> OUTRO_1 2s.
local function onSummonedCreatureDespawn(_, creature, summon)
    local guid = creature:GetGUIDLow()
    local st = states[guid]
    if not st then
        return
    end
    local summonGuid = summon:GetGUIDLow()
    local summonEntry = summon:GetEntry()
    for i, entry in ipairs(st.summons) do
        if entry.guid == summonGuid then
            table.remove(st.summons, i)
            break
        end
    end
    if #st.summons == 2 then
        if summonEntry == ENTRY_SCOURGE_ZOMBIE then
            schedule(creature, guid, EVENT_SPAWN_WAVE_2, 1000)
        elseif summonEntry == ENTRY_GHOUL_INVADER then
            schedule(creature, guid, EVENT_SPAWN_WAVE_3, 1000)
        elseif summonEntry == ENTRY_CRYPT_RAIDER then
            schedule(creature, guid, EVENT_MORLEN_2, 1000)
        end
    elseif #st.summons == 1 then
        if summonEntry == ENTRY_MORLEN then
            schedule(creature, guid, EVENT_OUTRO_1, 2000)
        end
    end
end

RegisterCreatureGossipEvent(ENTRY_THALORIEN, 2, onGossipSelect)
RegisterCreatureEvent(ENTRY_THALORIEN, 19, onJustSummoned)
RegisterCreatureEvent(ENTRY_THALORIEN, 20, onSummonedCreatureDespawn)
RegisterCreatureEvent(ENTRY_THALORIEN, 23, onReset)
