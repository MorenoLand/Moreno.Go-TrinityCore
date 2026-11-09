-- Nesingwary Trapper (Borean Tundra, quest 11865) --
-- Lua port of npc_nesingwary_trapperAI in
-- src/server/scripts/Northrend/zone_borean_tundra.cpp (lines 238-361;
-- AddSC at line 1884, zone-script block per northrend_script_loader.cpp).
-- Entry (C++-verifiable): NPC_NESINGWARY_TRAPPER = 25835 (:172).
-- Eluna creature events: 5 OnSpawn, 23 OnReset, 4 OnDied. The phase
-- machine rides a CreateLuaEvent chain (beryl/talbot convention): the
-- C++ UpdateAI phase timer is ungated (no UpdateVictim gate), so no
-- LeaveCombat cancel — death (event 4) and reset (event 23) cancel.
-- Ported arms (C++ order, :243-360):
-- - Initialize/Reset: SetVisible(false), phaseTimer 2500, phase 1,
--   caribou-GO latch cleared. Spawn and (live) reset both re-run it.
-- - Phase 1 (2500ms): SetVisible(true), 2000ms, phase 2.
-- - Phase 2 (2000ms): nearest High Quality Fur GO (187983) within
--   11.0yd -> MoveTo(0, fur pos) (drake MovePoint twin; C++ never
--   calls SetWalk, so the bridge's run spline is C++-exact), 1500ms,
--   phase 3. No fur found: C++ still advances (the MovePoint leg is
--   conditional, the timer is not).
-- - Phase 3 (1500ms): Talk(SAY_NESINGWARY_1 0), 2000ms, phase 4.
-- - Phase 4 (2000ms): PerformEmote(EMOTE_ONESHOT_LOOT 381,
--   SharedDefines.h:2019), 1000ms, phase 5.
-- - Phase 5 (1000ms): PerformEmote(EMOTE_ONESHOT_NONE 0), 500ms,
--   phase 6.
-- - Phase 6 (500ms): fur GO within 11.0yd -> Delete() leg STAYS OUT
--   (see below); 500ms, phase 7 regardless.
-- - Phase 7 (500ms): scan the 15 caribou-trap GOs in enum order,
--   first found within 5.0yd -> SetGoState(GO_STATE_ACTIVE 0) and
--   the latch stores that GO; 1000ms, phase 8 regardless.
-- - Phase 8 (1000ms): triggered self-cast SPELL_TRAPPED 46104
--   (DoCast(me, spell, true) -> CastSpell(creature, spell, true),
--   emalon precedent); phase 0, chain ends.
-- - JustDied: latched trap GO -> SetLootState(GO_JUST_DEACTIVATED
--   3), then the summoner-credit leg (GetSummonerUnit -> ToPlayer()
--   -> KilledMonsterCredit(25835), on its own guard — it fires even
--   with no latch), then SetGoState(GO_STATE_READY 1). C++ re-fetches
--   the GO for the second leg; same object, same order. No latch
--   (died before phase 7): both GO legs skip, matching the C++ nil-GO
--   guards; the credit leg still runs.
-- Unmodeled (documented-only, no bridges):
-- - The spell_q11865_place_fake_fur summoner (SpellScript class, no
--   spell-script bridge): the fur GO + trapper spawn never fire in
--   Go. The trapper AI is registered anyway (horseman-bearer
--   precedent: latent AI ports so DB/future-spell spawns behave).
-- - Phase 6 fur Delete(): the GO Despawn method
--   (engine/world/lua_objects.go:4515) only locks the GO object,
--   it does not remove it from the world — no GO-delete bridge.
-- - JustDied summoner credit: TempSummon GetSummonerUnit PORTS via
--   creature:GetSummonerUnit (bridged 2026-11-05), ToPlayer() gated
--   with GetObjectType() == "Player" (algalon/amanitar precedent);
--   the KilledMonsterCredit(25835) leg sits between the GO legs in
--   C++ order, below. The bridge returns nil when CreatedByGUID is
--   empty, the Go form of the C++ null-summoner guards.
-- - JustEngagedWith / MoveInLineOfSight are empty C++ overrides.

local ENTRY_NESINGWARY_TRAPPER = 25835

local GO_HIGH_QUALITY_FUR = 187983
local SPELL_TRAPPED = 46104

local EMOTE_ONESHOT_LOOT = 381
local EMOTE_ONESHOT_NONE = 0

local GO_STATE_ACTIVE = 0
local GO_STATE_READY = 1
local GO_LOOT_JUST_DEACTIVATED = 3

local CARIBOU_TRAPS = {
    187982, 187995, 187996, 187997, 187998, 187999, 188000, 188001,
    188002, 188003, 188004, 188005, 188006, 188007, 188008,
}

local timers = {}
local trapGOs = {}

local function cancelTimer(guid)
    local id = timers[guid]
    if id then
        RemoveEventById(id)
        timers[guid] = nil
    end
end

local function cancelAll(guid)
    cancelTimer(guid)
    trapGOs[guid] = nil
end

-- Forward declaration (tharon_ja / gal_darah idiom): the phase
-- chain's scheduler references advancePhase, which is assigned
-- below with a plain assignment, never a second local function.
local advancePhase

local function schedulePhase(guid, delay, phase, creature)
    cancelTimer(guid)
    timers[guid] = CreateLuaEvent(function() advancePhase(creature, guid, phase) end, delay)
end

advancePhase = function(creature, guid, phase)
    if phase == 1 then
        creature:SetVisible(true)
        schedulePhase(guid, 2000, 2, creature)
    elseif phase == 2 then
        local fur = creature:FindNearestGameObject(GO_HIGH_QUALITY_FUR, 11.0)
        if fur then
            creature:MoveTo(0, fur:GetX(), fur:GetY(), fur:GetZ())
        end
        schedulePhase(guid, 1500, 3, creature)
    elseif phase == 3 then
        creature:Talk(0)
        schedulePhase(guid, 2000, 4, creature)
    elseif phase == 4 then
        creature:PerformEmote(EMOTE_ONESHOT_LOOT)
        schedulePhase(guid, 1000, 5, creature)
    elseif phase == 5 then
        creature:PerformEmote(EMOTE_ONESHOT_NONE)
        schedulePhase(guid, 500, 6, creature)
    elseif phase == 6 then
        -- Fur Delete() leg stays out (no GO-delete bridge); the
        -- timer advances regardless, exactly like C++.
        schedulePhase(guid, 500, 7, creature)
    elseif phase == 7 then
        for _, trapEntry in ipairs(CARIBOU_TRAPS) do
            local trap = creature:FindNearestGameObject(trapEntry, 5.0)
            if trap then
                trap:SetGoState(GO_STATE_ACTIVE)
                trapGOs[guid] = trap
                break
            end
        end
        schedulePhase(guid, 1000, 8, creature)
    elseif phase == 8 then
        creature:CastSpell(creature, SPELL_TRAPPED, true)
        timers[guid] = nil
    end
end

-- C++ Initialize() (:247-253) + Reset() (:255-258): hidden, 2500ms
-- to phase 1, latch cleared. Spawn (event 5) and live reset
-- (event 23) both re-run it; the death-leg ev23 fire must not
-- restart the chain (beryl/calf IsDead guard).
local function trapperSpawnOrReset(event, creature)
    if event == 23 and creature:IsDead() then
        return
    end
    local guid = creature:GetGUIDLow()
    cancelAll(guid)
    creature:SetVisible(false)
    timers[guid] = CreateLuaEvent(function() advancePhase(creature, guid, 1) end, 2500)
end

-- C++ JustDied (:270-284): latched trap GO loot-state leg, then the
-- summoner-credit leg (GetSummonerUnit -> ToPlayer() ->
-- KilledMonsterCredit(entry)), then the GO-state leg; timers die with
-- the creature. The credit leg is NOT inside the GO guard — C++ runs
-- it on its own summoner chain, so it fires even with no latch.
local function trapperDied(event, creature)
    local guid = creature:GetGUIDLow()
    local trap = trapGOs[guid]
    if trap then
        trap:SetLootState(GO_LOOT_JUST_DEACTIVATED)
    end
    local summoner = creature:GetSummonerUnit()
    if summoner and summoner:GetObjectType() == "Player" then
        summoner:KilledMonsterCredit(ENTRY_NESINGWARY_TRAPPER)
    end
    if trap then
        trap:SetGoState(GO_STATE_READY)
    end
    cancelAll(guid)
end

RegisterCreatureEvent(ENTRY_NESINGWARY_TRAPPER, 5, trapperSpawnOrReset)
RegisterCreatureEvent(ENTRY_NESINGWARY_TRAPPER, 23, trapperSpawnOrReset)
RegisterCreatureEvent(ENTRY_NESINGWARY_TRAPPER, 4, trapperDied)
