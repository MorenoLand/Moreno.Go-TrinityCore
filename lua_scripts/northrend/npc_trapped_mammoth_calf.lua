-- Northrend: Trapped Mammoth Calf — Lua port of
-- src/server/scripts/Northrend/zone_borean_tundra.cpp
-- (class npc_trapped_mammoth_calf : CreatureScript("npc_trapped_mammoth_calf");
-- struct npc_trapped_mammoth_calfAI : public ScriptedAI).
-- Entry: 25850 (NPC_TRAPPED_MAMMOTH_CALF, World/item_scripts.cpp:167,
-- quest-11876 item script — the GO_MAMMOTH_TRAP_1..22 lists there match
-- this file's enum entry-for-entry, cross-confirming the binding).
-- Eluna creature events: 23 OnReset.
-- Ported arms (C++ order, zone_borean_tundra.cpp:1330-1343):
-- - Reset: scan the 22 mammoth-trap GOs in enum order; the first found
--   within 11.0 yards gets SetGoState(0) (GO_STATE_ACTIVE), then return.
--   The IsDead guard is the marli ev23 idiom: Go fires ev23 on spawn,
--   evade and death, while C++ Reset runs on spawn + evade only.
-- Unmodeled (documented-only, no bridges):
-- - UpdateAI MovePoint leg (no Move* bridge) and the DoAction(1)
--   bStarted latch (no DoAction bridge).
-- - MovementInform arrival leg: DisappearAndDie + trap SetLootState
--   (GO_JUST_DEACTIVATED) — no MovementInform bridge (SetLootState
--   itself is bridged on the GO object, but unreachable without it).

local ENTRY_CALF = 25850

local MAMMOTH_TRAPS = {
    188022, 188024, 188025, 188026, 188027, 188028, 188029, 188030,
    188031, 188032, 188033, 188034, 188035, 188036, 188037, 188038,
    188039, 188040, 188041, 188042, 188043, 188044,
}

-- C++ npc_trapped_mammoth_calfAI::Reset (zone_borean_tundra.cpp:1330-1343):
-- Initialize() (timer/latch state, feeds the unported UpdateAI only) ->
-- trap scan in enum order, first hit activated, then return.
local function calfReset(event, creature)
    if creature:IsDead() then
        return
    end
    for _, trapEntry in ipairs(MAMMOTH_TRAPS) do
        local trap = creature:FindNearestGameObject(trapEntry, 11.0)
        if trap then
            trap:SetGoState(0) -- GO_STATE_ACTIVE
            return
        end
    end
end

RegisterCreatureEvent(ENTRY_CALF, 23, calfReset)
