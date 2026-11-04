-- Area trigger scripts — src/server/scripts/World/areatrigger_scripts.cpp
-- (474 lines; 10 AreaTriggerScript classes, AddSC_areatrigger_scripts).
-- Lua fire site: engine/world/areatrigger.go:64 (TriggerServerEvent ctx,
-- scripting.ServerEventAreaTrigger (=24), luaPlayer, triggerID); handlers
-- registered via RegisterServerEvent(24, fn) receive (event, player, triggerID)
-- per runtime.go:640-642. WalkDir auto-loads this file; no loader edit needed.
-- PORTED (2026-10-24): at_area_52_entrance, at_brewfest (handler below).
-- PORTED (2026-11-02): at_coilfang_waterfall (handler below; the
-- entry-filtered player gameobject search that blocked it is now bridged
-- as player:GetNearestGameObject(range, entry, hostile)).
-- PORTED (2026-11-04): at_legion_teleporter, at_sholazar_waygate
-- (handler below; the header's "no player:CastSpell bridge" claim was
-- stale — player:CastSpell bridged 2026-10-24, lua_player.go:2549, and
-- GetQuestRewardStatus is bridged, lua_player.go:3787).
-- Remaining classes documented-only; bridges verified against the current
-- runtime this run (player method table at
-- engine/world/lua_player.go, creature table at
-- engine/world/lua_creature_events.go:55-102, server-event hooks at
-- engine/scripting/runtime.go:966).
-- Gap classes (spell IDs exact from the C++ enum blocks):
-- 1. at_stormwright_shelf (:159-177; SPELL_CREATE_TRUE_POWER_OF_THE_TEMPEST
--    53067 gated on GetQuestStatus(12741)==INCOMPLETE(3)) stays out on its
--    trigger ID only — it lives only in the DB areatrigger_scripts row
--    (gelihast precedent); the player:CastSpell leg is bridged
--    (lua_player.go:2549), so the old "no player:CastSpell bridge"
--    claim for this class is retracted. at_legion_teleporter and
--    at_sholazar_waygate PORTED below (their IDs are in-file: ContentData
--    4560 for legion, enum AT_SHOLAZAR 5046 / AT_UNGORO 5047).
-- 2. player:GetQuestStatus + player:FindNearestCreature bridged 2026-10-24
--    (lua_player.go). at_scent_larkorwi (:192-212; quest 4291 gate +
--    FindNearestCreature(9683, 15) dedup + SummonCreature(9683)) and
--    at_sentry_point (:298-326; quest 1265 gate + FindNearestCreature(4967,
--    100) dedup + SummonCreature(4967) + tervosh self-cast 799) are now
--    bridge-complete but UNPORTED: their trigger IDs live only in the DB
--    areatrigger_scripts rows (ObjectMgr.cpp:6653), not in the C++ file —
--    dispatching on a guessed ID would invent an identifier (gelihast
--    precedent). at_nats_landing (:261-293) stays out on its trigger ID
--    only; its AttackStart leg now has the EngageWithTarget twin
--    (2026-10-26, grandmother/sartura precedent);
--    at_area_52_entrance PORTED below (was in this class).
-- 3. no creature-motion bridges (SetWalk/MovePath/AddUnitState):
--    at_frostgrips_hollow (:449-512; quest 12877 + SummonCreature(29862/29861)
--    + MovePath(NPC*100)); quest-status gate bridged 2026-10-24, and its
--    trigger ID is DB-side only (not in the C++ file).
-- 4. at_brewfest PORTED below (:331-376): FindNearestCreature(24711/24710)
--    bridged 2026-10-24 (lua_player.go), Talk(SAY_WELCOME 4, player) via the
--    creature Talk whisper-target arm; 5s per-trigger cooldown on
--    AT_BREWFEST_DUROTAR 4829 / AT_BREWFEST_DUN_MOROGH 4820.
-- NOTE: RegisterServerEvent(24, areaTriggerHandler) below dispatches on the
-- triggerID argument (hooks are per-event, not per-trigger ID). Trigger IDs:
-- 4591 coilfang (PORTED), 4560 legion (PORTED), 5046/5047 sholazar (PORTED),
-- 4829/4820 brewfest (PORTED), 4472/4466/4471/4422 area52 (PORTED);
-- scent/sentry/nats/frostgrips/stormwright IDs live only in the DB
-- areatrigger_scripts rows (ObjectMgr.cpp:6653), not in the C++ file —
-- no guessed dispatch.

-- PORTED ARMS (areatrigger_scripts.cpp; RegisterServerEvent 24 handler).
-- Go/Eluna summonType is 0-based (lua_creature_events.go:1241): TIMED_DESPAWN
-- = 2 here, matching the ramstein/leotheras ports; the C++ enum is 1-based
-- (ObjectDefines.h:57-67).

local AREA52_POS = {
    [4472] = { 2950.63, 3719.905, 143.33 }, -- SOUTH
    [4466] = { 3114.87, 3687.619, 143.62 }, -- NORTH
    [4471] = { 3017.79, 3746.806, 144.27 }, -- WEST
    [4422] = { 3044.176, 3610.692, 143.61 }, -- EAST
}

local BREWFEST_TAPPER = {
    [4829] = 24711, -- DUROTAR: Tapper Swindlekeg
    [4820] = 24710, -- DUN MOROGH: Ipfelkofer Ironkeg
}

local lastTrigger = {}

local SHOLAZAR_TELEPORT = {
    [5046] = 52056, -- AT_SHOLAZAR: Sholazar -> Un'goro
    [5047] = 52057, -- AT_UNGORO: Un'goro -> Sholazar
}

local function areaTriggerHandler(_, player, triggerID)
    if triggerID == 4560 then
        -- at_legion_teleporter (:117-141): alive + out of combat, then
        -- faction + quest-reward gates: Alliance + rewarded 10589 ->
        -- non-triggered self-cast 37387; Horde + rewarded 10604 ->
        -- non-triggered self-cast 37389. C++ returns true on cast,
        -- false otherwise; the handler returns nothing per the
        -- PORTED-arm convention.
        if player:IsAlive() and not player:IsInCombat() then
            if player:GetTeam() == 0 and player:GetQuestRewardStatus(10589) then
                player:CastSpell(player, 37387, false)
            elseif player:GetTeam() == 1 and player:GetQuestRewardStatus(10604) then
                player:CastSpell(player, 37389, false)
            end
        end
        return
    end
    local sholazarSpell = SHOLAZAR_TELEPORT[triggerID]
    if sholazarSpell then
        -- at_sholazar_waygate (:214-256): not dead + (quest 13956 not
        -- NONE, or quests 12613 + 12559 both REWARDED) -> triggered
        -- self-cast of the direction spell. Quest statuses: NONE 0,
        -- REWARDED 6 (quests.go).
        if not player:IsDead() and (player:GetQuestStatus(13956) ~= 0
            or (player:GetQuestStatus(12613) == 6
                and player:GetQuestStatus(12559) == 6)) then
            player:CastSpell(player, sholazarSpell, true)
        end
        return
    end
    if triggerID == 4591 then
        -- at_coilfang_waterfall (:72-94): nearest GO_COILFANG_WATERFALL
        -- 184212 within 35.0f, GO_READY (loot state 1) check, then
        -- UseDoorOrButton. C++ returns false; the Go handler returns
        -- nothing, the existing PORTED-arm convention. The GO_READY
        -- read is the snapshot LootState (1 for DB/runtime states),
        -- so the check passes and the button use is the real effect.
        local go = player:GetNearestGameObject(35.0, 184212)
        if go and go:GetLootState() == 1 then
            go:UseDoorOrButton()
        end
        return
    end
    local area52 = AREA52_POS[triggerID]
    if area52 then
        -- at_area_52_entrance (:381-442): IsAlive + 5s per-trigger cooldown,
        -- then spotlight summon + neuralyzer aura, C++ order.
        if not player:IsAlive() then
            return
        end
        local now = os.time()
        if lastTrigger[triggerID] and now - lastTrigger[triggerID] < 5 then
            return
        end
        player:SummonCreature(19913, area52[1], area52[2], area52[3], 0.0, 2, 5000)
        player:AddAura(34400)
        lastTrigger[triggerID] = now
        return
    end
    local tapperEntry = BREWFEST_TAPPER[triggerID]
    if tapperEntry then
        -- at_brewfest (:331-376): 5s per-trigger cooldown, then the tapper's
        -- welcome Talk directed at the triggering player; the timestamp
        -- updates even when no tapper is found, as in C++.
        local now = os.time()
        if lastTrigger[triggerID] and now - lastTrigger[triggerID] < 5 then
            return
        end
        local tapper = player:FindNearestCreature(tapperEntry, 20.0)
        if tapper then
            tapper:Talk(4, player)
        end
        lastTrigger[triggerID] = now
        return
    end
end

RegisterServerEvent(24, areaTriggerHandler)
