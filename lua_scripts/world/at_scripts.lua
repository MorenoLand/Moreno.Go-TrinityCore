-- Area trigger scripts — src/server/scripts/World/areatrigger_scripts.cpp
-- (474 lines; 10 AreaTriggerScript classes, AddSC_areatrigger_scripts).
-- Lua fire site: engine/world/areatrigger.go:64 (TriggerServerEvent ctx,
-- scripting.ServerEventAreaTrigger (=24), luaPlayer, triggerID); handlers
-- registered via RegisterServerEvent(24, fn) receive (event, player, triggerID)
-- per runtime.go:640-642. WalkDir auto-loads this file; no loader edit needed.
-- PORTED (2026-10-24): at_area_52_entrance, at_brewfest (handler below).
-- Remaining classes documented-only; bridges verified against the current
-- runtime this run (player method table at
-- engine/world/lua_player.go, creature table at
-- engine/world/lua_creature_events.go:55-102, server-event hooks at
-- engine/scripting/runtime.go:966).
-- Gap classes (spell IDs exact from the C++ enum blocks):
-- 1. no player:CastSpell bridge (creatures only: lua_creature_events.go:62;
--    in-tree f839950e nil-call-class precedent): at_legion_teleporter
--    (:117-141; SPELL_TELE_A_TO 37387 / SPELL_TELE_H_TO 37389 gated on
--    IsAlive/IsInCombat/GetTeam/GetQuestRewardStatus 10589/10604 — reward
--    status also unbridged: only HasQuest exists, lua_player.go:303), and
--    at_stormwright_shelf (:159-177; SPELL_CREATE_TRUE_POWER_OF_THE_TEMPEST
--    53067 gated on GetQuestStatus(12741)==INCOMPLETE(3) — gate bridged
--    2026-10-24, player:CastSpell leg still blocks), at_sholazar_waygate
--    (:214-256; SHOLAZAR_TO_UNGORO 52056 /
--    UNGORO_TO_SHOLAZAR 52057 on AT_SHOLAZAR 5046 / AT_UNGORO 5047, gated on
--    quest statuses 12613/12559/13956).
-- 2. player:GetQuestStatus + player:FindNearestCreature bridged 2026-10-24
--    (lua_player.go). at_scent_larkorwi (:192-212; quest 4291 gate +
--    FindNearestCreature(9683, 15) dedup + SummonCreature(9683)) and
--    at_sentry_point (:298-326; quest 1265 gate + FindNearestCreature(4967,
--    100) dedup + SummonCreature(4967) + tervosh self-cast 799) are now
--    bridge-complete but UNPORTED: their trigger IDs live only in the DB
--    areatrigger_scripts rows (ObjectMgr.cpp:6653), not in the C++ file —
--    dispatching on a guessed ID would invent an identifier (gelihast
--    precedent). at_nats_landing (:261-293) stays out on AttackStart;
--    at_area_52_entrance PORTED below (was in this class).
-- 3. no creature-motion bridges (SetWalk/MovePath/AddUnitState):
--    at_frostgrips_hollow (:449-512; quest 12877 + SummonCreature(29862/29861)
--    + MovePath(NPC*100)); quest-status gate bridged 2026-10-24, and its
--    trigger ID is DB-side only (not in the C++ file).
-- 4. no entry-filtered gameobject bridge on the player surface
--    (player:GetNearestGameObject(range) at lua_player.go:968 takes no
--    entry; creature:FindNearestGameObject can't reach the AT hook's
--    player-only context; go:GetLootState + go:UseDoorOrButton ARE bridged
--    — lua_objects.go:1093 — but the arm stays out behind the search
--    gate): at_coilfang_waterfall (:72-94; GO_COILFANG_WATERFALL
--    184212 within 35.0f, GO_READY check, UseDoorOrButton).
-- 5. at_brewfest PORTED below (:331-376): FindNearestCreature(24711/24710)
--    bridged 2026-10-24 (lua_player.go), Talk(SAY_WELCOME 4, player) via the
--    creature Talk whisper-target arm; 5s per-trigger cooldown on
--    AT_BREWFEST_DUROTAR 4829 / AT_BREWFEST_DUN_MOROGH 4820.
-- NOTE: RegisterServerEvent(24, areaTriggerHandler) below dispatches on the
-- triggerID argument (hooks are per-event, not per-trigger ID). Trigger IDs:
-- 4591 coilfang, 4560 legion, 5046/5047 sholazar, 4829/4820 brewfest
-- (PORTED), 4472/4466/4471/4422 area52 (PORTED); scent/sentry/nats/
-- frostgrips/stormwright IDs live only in the DB areatrigger_scripts rows
-- (ObjectMgr.cpp:6653), not in the C++ file — no guessed dispatch.

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

local function areaTriggerHandler(_, player, triggerID)
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
