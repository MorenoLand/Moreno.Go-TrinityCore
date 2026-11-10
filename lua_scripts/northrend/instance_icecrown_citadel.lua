-- The Icecrown Citadel instance script — Lua port of
-- src/server/scripts/Northrend/IcecrownCitadel/instance_icecrown_citadel.cpp
-- (class instance_icecrown_citadel : public InstanceMapScript, map 631).
-- RegisterMapEvent binds the map-level Eluna instance hooks (the
-- instance_halls_of_reflection.lua / instance_vault_of_archavon.lua
-- precedent); its presence opens the Map:GetInstanceData gate for map 631
-- (engine/world/instance_hooks.go) that the boss files share with this file.
-- Shared instanceData keys (one table per (map, instance)):
-- lordMarrowgar / ladyDeathwhisper / deathbringerSaurfang /
-- deathbringerSaurfangEvent / festergut / rotface / professorPutricide /
-- princeKeleseth / princeTaldaram / princeValanar / bloodPrincesControl /
-- bloodQueenCouncil / bloodQueenLanaThel / crokScourgebane /
-- crokCaptains[1-4] / sisterSvalna / valithriaDreamwalker /
-- valithriaLichKing / valithriaTrigger / sindragosa / spinestalker /
-- rimefang / theLichKing / highlordTirionFordring / terenas — the
-- OnCreatureCreate GUID latches (instance_icecrown_citadel.cpp:205-330).
-- Ported arms (C++-exact unless noted):
-- - OnCreatureCreate: the binds above. Fires on materialization
--   (motionFor funnel, instance_hooks.go:311-320); a static template
--   spawn never materializes server-side and is never bound (naxxramas
--   LoadGrid delta).
-- - OnUnitDeath (instance_icecrown_citadel.cpp:444-500, PORTED 2026-11-10):
--   NPC_FROSTWING_WHELP (:460-477): the trash-set erase/count is
--   modeled as a live sweep — the dead whelp's home Y (< 2484.35f =
--   RIMEFANG, else SPINESTALKER, trash InitializeAI) picks the owner,
--   and GetCreaturesInGrid(37532, 500) filtered to live whelps of that
--   owner stands in for the SpinestalkerTrash/RimefangTrash sets (the
--   whelp's own InitializeAI/JustAppeared SetData legs never fire in
--   Go — the whelp is unregistered and static spawns never fire
--   OnCreatureCreate); when none remain, the frostwyrm found by grid
--   sweep gets DoAction(ACTION_START_FROSTWYRM) (the C++
--   FrostwyrmGUIDs-empty early return is the no-live-frostwyrm
--   sweep miss). DELTA: whelps in never-loaded grids are invisible
--   to the sweep, so the fire can come early if one hall's whelps
--   never materialized. NPC_RIMEFANG/NPC_SPINESTALKER (:478-496):
--   the Sindragosa-DONE gate rides creature:GetBossState; the other
--   frostwyrm's liveness is a grid sweep in place of the
--   FrostwyrmGUIDs erase/count; both dead -> SummonCreature(36853,
--   SindragosaSpawnPos) + DoAction(ACTION_START_FROSTWYRM) (the
--   LoadGrid leg has no bridge; the heroic-attempts gate has no
--   bridge — the attempts machine is unmodeled, see SetBossState
--   note). The Ymirjar relay (:451-458, SetGUID to Crok) stays out
--   (no SetGUID bridge).
-- - OnCreatureCreate conditional legs:
--   NPC_INFILTRATOR_MINCHAR_BQ (:277-281): SetUInt32Value(
--   UNIT_NPC_EMOTESTATE, 0) + SetDisableGravity(true) — both bridged
--   (lua_values.go SetUInt32Value; lua_creature_events.go:3116).
--   NPC_WICKED_SPIRIT (:325-329): SetReactState(REACT_PASSIVE) —
--   SetCorpseDelay(0) has no bridge, stays out.
-- - GetGuidData (instance_icecrown_citadel.cpp:763-850): the Lua mirror
--   is the instanceData binds above, read as instanceData.<boss> (the
--   oculus precedent). NOTE: the live ICC boss files read GUIDs via
--   creature:GetInstanceCreature(DATA_X) instead, which rides the
--   instance GUID store populated by the ev5 SetGuidData latches in
--   the boss files (valithriaSpawn / BPC princes precedent) — the two
--   stores are independent, and this file's binds mirror the C++ latches
--   for future consumers (the halls_of_reflection "mirrored as a bind
--   anyway" precedent).
-- Unmodeled (documented-only, no bridges):
-- - SetBossState override (:856-1040): every leg rides the door/GO model
--   (SetTeleporterState, HandleGameObject sigils/gates/pipes, elevator
--   GO state, gunship-armory / deathbringers-cache flag removal,
--   precipice/platform SetFarVisible, bolvar/pillars SetRespawnTime),
--   the heroic-attempts machine (HandleHeroicAttempts, worldstate +
--   boss despawns), the weekly-quest summon (Valithria DONE, quest-pool
--   gated), or the LK DONE quest-NPC summons — no GO-state / instance-
--   summon / quest-pool bridges from Lua. The gunship FAIL leg
--   (EVENT_RESPAWN_GUNSHIP 30s) rides the unported ProcessEvent
--   machine; SpawnGunship() (:1041-1062) rides CreateTransport.
--   No ON_UPDATE poll: the override legs have no live Lua consumer —
--   the DONE legs already run at DONE time in the boss died handlers
--   (the palehoof precedent).
-- - OnCreatureCreate skipped binds: NPC_NERUBAR_BROODKEEPER (:216-220)
--   splits into two GUID lists by GetPositionX() > -230.0f — no
--   position bridge; NPC_ALLIANCE/HORDE_GUNSHIP_CANNON (:230-233)
--   SetControlled(true, UNIT_STATE_ROOT) — no bridge;
--   NPC_VOLATILE_OOZE / NPC_GAS_CLOUD (:242-251) relay into the
--   professor's JustSummoned — no cross-AI JustSummoned bridge;
--   NPC_INVISIBLE_STALKER (:313-318) casts SPELL_ARTHAS_TELEPORTER_CEREMONY
--   only within 10yd of (4357.052, 2769.421) — no position bridge;
--   the guardian faction-buff cast (:207-211) rides the team latch
--   (no instance GetData bridge — the halls_of_reflection precedent).
-- - OnCreatureRemove (:335-339): clears SindragosaGUID only — no
--   OnCreatureRemove map event exists (map events are 1-7,
--   engine/scripting/runtime.go:172-178); the bind goes stale on
--   remove instead (naxxramas delta).
-- - OnGameObjectCreate (:502-760): all door-model legs (AddDoor for
--   ~25 entrances), teleporter/elevator/armory/cache GUID latches and
--   their DONE-gated GO-state legs — no GO bridge; the Lady Deathwhisper
--   elevator GO-state leg is gated on GetBossState, readable, but the
--   GO write itself has no bridge.
-- - GetCreatureEntry weekly-quest prevention (:340-360): quest-pool
--   gated (sQuestPoolMgr) — no bridge.
-- - CheckLichKingAvailability (:1315-1337): GO-state + stalker cast +
--   Reset relay, gated on Putricide + Lana'thel + Sindragosa DONE —
--   no GO bridge.
-- - Update (:1365+): BloodQuickening / coldflame-jets machines ride
--   instance SetData state (no bridge).
-- - SetHeaders(DataHeader "ICC") / SetBossNumber(EncounterCount 13) /
--   LoadObjectData / WriteSaveDataMore / ReadSaveDataMore / SaveToDB:
--   no bridges; the boss-state store is written via
--   creature:SetBossState (lua_creature_events.go:5917) and read via
--   creature:GetBossState (lua_creature_events.go:5932).
-- No ON_UPDATE poll: see the SetBossState note above.

local MAP_ICECROWN_CITADEL = 631
local INSTANCE_EVENT_ON_CREATURE_CREATE = 5
local INSTANCE_EVENT_ON_UNIT_DEATH = 9

local NPC_FROSTWING_WHELP = 37532 -- icecrown_citadel.h:308
local NPC_NERUBAR_BROODKEEPER = 36725 -- icecrown_citadel.h:152
local NPC_INFILTRATOR_MINCHAR_BQ = 38558 -- icecrown_citadel.h:162
local NPC_LORD_MARROWGAR = 36612 -- icecrown_citadel.h:169
local NPC_LADY_DEATHWHISPER = 36855 -- icecrown_citadel.h:174
local NPC_ALLIANCE_GUNSHIP_CANNON = 36838 -- icecrown_citadel.h:185
local NPC_HORDE_GUNSHIP_CANNON = 36839 -- icecrown_citadel.h:186
local NPC_DEATHBRINGER_SAURFANG = 37813 -- icecrown_citadel.h:211
local NPC_SE_MURADIN_BRONZEBEARD = 37200 -- icecrown_citadel.h:214
local NPC_SE_HIGH_OVERLORD_SAURFANG = 37187 -- icecrown_citadel.h:216
local NPC_FESTERGUT = 36626 -- icecrown_citadel.h:222
local NPC_ROTFACE = 36627 -- icecrown_citadel.h:227
local NPC_PROFESSOR_PUTRICIDE = 36678 -- icecrown_citadel.h:234
local NPC_GAS_CLOUD = 37562 -- icecrown_citadel.h:237
local NPC_VOLATILE_OOZE = 37697 -- icecrown_citadel.h:238
local NPC_PRINCE_KELESETH = 37972 -- icecrown_citadel.h:245
local NPC_PRINCE_TALDARAM = 37973 -- icecrown_citadel.h:246
local NPC_PRINCE_VALANAR = 37970 -- icecrown_citadel.h:247
local NPC_BLOOD_ORB_CONTROLLER = 38008 -- icecrown_citadel.h:248
local NPC_BLOOD_QUEEN_LANA_THEL_COUNCIL = 38004 -- icecrown_citadel.h:256
local NPC_BLOOD_QUEEN_LANA_THEL = 37955 -- icecrown_citadel.h:266
local NPC_CROK_SCOURGEBANE = 37129 -- icecrown_citadel.h:269
local NPC_CAPTAIN_ARNATH = 37122 -- icecrown_citadel.h:270
local NPC_CAPTAIN_BRANDON = 37123 -- icecrown_citadel.h:271
local NPC_CAPTAIN_GRONDEL = 37124 -- icecrown_citadel.h:272
local NPC_CAPTAIN_RUPERT = 37125 -- icecrown_citadel.h:273
local NPC_SISTER_SVALNA = 37126 -- icecrown_citadel.h:283
local NPC_VALITHRIA_DREAMWALKER = 36789 -- icecrown_citadel.h:287
local NPC_GREEN_DRAGON_COMBAT_TRIGGER = 38752 -- icecrown_citadel.h:288
local NPC_THE_LICH_KING_VALITHRIA = 16980 -- icecrown_citadel.h:297
local NPC_SINDRAGOSA = 36853 -- icecrown_citadel.h:304
local NPC_SPINESTALKER = 37534 -- icecrown_citadel.h:305
local NPC_RIMEFANG = 37533 -- icecrown_citadel.h:306
local NPC_THE_LICH_KING = 36597 -- icecrown_citadel.h:314
local NPC_HIGHLORD_TIRION_FORDRING_LK = 38995 -- icecrown_citadel.h:315
local NPC_TERENAS_MENETHIL_FROSTMOURNE = 36823 -- icecrown_citadel.h:316
local NPC_TERENAS_MENETHIL_FROSTMOURNE_H = 39217 -- icecrown_citadel.h:318
local NPC_WICKED_SPIRIT = 39190 -- icecrown_citadel.h:326
local NPC_INVISIBLE_STALKER = 30298 -- icecrown_citadel.h:335

local UNIT_NPC_EMOTESTATE = 83 -- UpdateFields.h:140 (OBJECT_END 0x0006 + 0x4D)

local DATA_SINDRAGOSA = 11 -- icecrown_citadel.h:85
local BOSS_STATE_DONE = 3 -- InstanceScript.h:68-76
local ACTION_START_FROSTWYRM = -368530 -- icecrown_citadel.h:507
local SINDRAGOSA_SPAWN_POS = {4818.7, 2483.71, 287.065, 3.089233} -- boss_sindragosa.cpp:169

local bossBinds = {
    [NPC_LORD_MARROWGAR] = "lordMarrowgar",
    [NPC_LADY_DEATHWHISPER] = "ladyDeathwhisper",
    [NPC_DEATHBRINGER_SAURFANG] = "deathbringerSaurfang",
    [NPC_SE_HIGH_OVERLORD_SAURFANG] = "deathbringerSaurfangEvent",
    [NPC_SE_MURADIN_BRONZEBEARD] = "deathbringerSaurfangEvent",
    [NPC_FESTERGUT] = "festergut",
    [NPC_ROTFACE] = "rotface",
    [NPC_PROFESSOR_PUTRICIDE] = "professorPutricide",
    [NPC_PRINCE_KELESETH] = "princeKeleseth",
    [NPC_PRINCE_TALDARAM] = "princeTaldaram",
    [NPC_PRINCE_VALANAR] = "princeValanar",
    [NPC_BLOOD_ORB_CONTROLLER] = "bloodPrincesControl",
    [NPC_BLOOD_QUEEN_LANA_THEL_COUNCIL] = "bloodQueenCouncil",
    [NPC_BLOOD_QUEEN_LANA_THEL] = "bloodQueenLanaThel",
    [NPC_CROK_SCOURGEBANE] = "crokScourgebane",
    [NPC_SISTER_SVALNA] = "sisterSvalna",
    [NPC_VALITHRIA_DREAMWALKER] = "valithriaDreamwalker",
    [NPC_THE_LICH_KING_VALITHRIA] = "valithriaLichKing",
    [NPC_GREEN_DRAGON_COMBAT_TRIGGER] = "valithriaTrigger",
    [NPC_SINDRAGOSA] = "sindragosa",
    [NPC_SPINESTALKER] = "spinestalker",
    [NPC_RIMEFANG] = "rimefang",
    [NPC_THE_LICH_KING] = "theLichKing",
    [NPC_HIGHLORD_TIRION_FORDRING_LK] = "highlordTirionFordring",
    [NPC_TERENAS_MENETHIL_FROSTMOURNE] = "terenas",
    [NPC_TERENAS_MENETHIL_FROSTMOURNE_H] = "terenas",
}

RegisterMapEvent(MAP_ICECROWN_CITADEL, INSTANCE_EVENT_ON_CREATURE_CREATE, function(event, instanceData, mapObj, creature)
    local entry = creature:GetEntry()
    -- C++ :277-281: the infiltrator minchar is kept in air.
    if entry == NPC_INFILTRATOR_MINCHAR_BQ then
        creature:SetUInt32Value(UNIT_NPC_EMOTESTATE, 0)
        creature:SetDisableGravity(true)
        return
    end
    -- C++ :325-329: wicked spirits die without a corpse delay and
    -- stay passive; the corpse-delay leg has no bridge.
    if entry == NPC_WICKED_SPIRIT then
        creature:SetReactState(0)
        return
    end
    -- C++ :216-220 / :230-233 / :242-251 / :313-318: the broodkeeper
    -- group split, gunship-cannon root, ooze/gas-cloud professor relay
    -- and stalker teleporter cast are skipped — no position /
    -- SetControlled / cross-AI-JustSummoned bridges (see header).
    if entry == NPC_NERUBAR_BROODKEEPER or entry == NPC_ALLIANCE_GUNSHIP_CANNON or
            entry == NPC_HORDE_GUNSHIP_CANNON or entry == NPC_VOLATILE_OOZE or
            entry == NPC_GAS_CLOUD or entry == NPC_INVISIBLE_STALKER then
        return
    end
    -- C++ :269-276: the four Crok captains index by entry -
    -- NPC_CAPTAIN_ARNATH (entries are consecutive, :270-273).
    if entry >= NPC_CAPTAIN_ARNATH and entry <= NPC_CAPTAIN_RUPERT then
        if instanceData.crokCaptains == nil then
            instanceData.crokCaptains = {}
        end
        instanceData.crokCaptains[entry - NPC_CAPTAIN_ARNATH + 1] = creature
        return
    end
    local key = bossBinds[entry]
    if not key then
        return
    end
    instanceData[key] = creature
end)

-- C++ OnUnitDeath (instance_icecrown_citadel.cpp:444-500), PORTED
-- 2026-11-10 — the instance-side sender for the frostwyrm intro
-- machines (the 2026-11-04 stay-out is lifted:
-- INSTANCE_EVENT_ON_UNIT_DEATH fires from setDeathState,
-- instance_hooks.go, the Blood Furnace precedent).
local function onUnitDeath(event, instanceData, mapObj, unit)
    local entry = unit:GetEntry()
    if entry == NPC_FROSTWING_WHELP then
        -- C++ :464: the owner is the whelp AI's DATA_FROSTWYRM_OWNER,
        -- home Y < 2484.35f = RIMEFANG (trash InitializeAI).
        local _, homeY = unit:GetHomePosition()
        local ownerEntry = NPC_SPINESTALKER
        if homeY < 2484.35 then
            ownerEntry = NPC_RIMEFANG
        end
        -- C++ :466-476: erase from the owner's trash set, fire when
        -- empty — the live sweep below is the set twin.
        for _, whelp in ipairs(unit:GetCreaturesInGrid(NPC_FROSTWING_WHELP, 500)) do
            if not whelp:IsDead() then
                local _, whelpHomeY = whelp:GetHomePosition()
                if (whelpHomeY < 2484.35) == (homeY < 2484.35) then
                    return
                end
            end
        end
        for _, frostwyrm in ipairs(unit:GetCreaturesInGrid(ownerEntry, 500)) do
            if not frostwyrm:IsDead() then
                frostwyrm:DoAction(ACTION_START_FROSTWYRM)
                return
            end
        end
        return
    end
    if entry == NPC_SPINESTALKER or entry == NPC_RIMEFANG then
        -- C++ :481-483: the heroic-attempts gate has no bridge (the
        -- attempts machine is unmodeled); the DONE gate rides
        -- creature:GetBossState.
        if unit:GetBossState(DATA_SINDRAGOSA) == BOSS_STATE_DONE then
            return
        end
        local otherEntry = NPC_RIMEFANG
        if entry == NPC_RIMEFANG then
            otherEntry = NPC_SPINESTALKER
        end
        -- C++ :488-489: erase from FrostwyrmGUIDs, fire when empty —
        -- the live sweep below is the set twin.
        for _, frostwyrm in ipairs(unit:GetCreaturesInGrid(otherEntry, 500)) do
            if not frostwyrm:IsDead() then
                return
            end
        end
        -- C++ :491-493: LoadGrid has no bridge; the summon +
        -- DoAction legs port.
        local boss = unit:SummonCreature(NPC_SINDRAGOSA, SINDRAGOSA_SPAWN_POS[1], SINDRAGOSA_SPAWN_POS[2], SINDRAGOSA_SPAWN_POS[3], SINDRAGOSA_SPAWN_POS[4])
        if boss then
            boss:DoAction(ACTION_START_FROSTWYRM)
        end
        return
    end
end

RegisterMapEvent(MAP_ICECROWN_CITADEL, INSTANCE_EVENT_ON_UNIT_DEATH, onUnitDeath)
