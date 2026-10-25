-- go_scripts.cpp PART 1 (lines 61-682): 13 GameObjectScript ports.
-- Source: src/server/scripts/World/go_scripts.cpp (C++ line refs below).
--
-- VERDICT: all 13 are DOCUMENTED-ONLY — no Register calls in this file.
-- Bridge verification (grep against the CURRENT runtime, 2026-10-18):
--   * RegisterGameObjectGossipEvent(entry, 1, fn) hello arm fires as
--     (event, player, go) via fireGameObjectGossipHelloHook (gameobject_hooks.go:158);
--     a Lua TRUE/nil return suppresses native GO use (C++ `return true`
--     handled == Lua true — arlokk gong / belnistrasz polarity), a Lua
--     FALSE return lets the native arms proceed. Corrected 2026-10-24
--     (the 2026-10-18 polarity note above was inverted; see
--     gameobject_hooks.go:133-144 + gameobjects.go:754).
--   * RegisterGameObjectGossipEvent(entry, 2, fn) select arm fires as
--     (event, player, go, sender, action[, code]) via fireGameObjectGossipSelectHook
--     (gameobject_hooks.go:190, wired at gossip.go:705). (The stale
--     "ON_SELECT no-bridge" comment at gameobject_hooks.go:145 predates the wiring.)
--   * Bridged and usable: go:UseDoorOrButton (lua_objects.go:1113),
--     go:SetLootState (lua_objects.go:1095), player:SendNotification,
--     player:HasQuest (lua_player.go:303 — counts character_queststatus rows,
--     NOT quest status), player:HasItem (lua_player.go:188), player:GetItemCount,
--     player:GossipMenuAddItem / GossipSendMenu / GossipClearMenu / GossipComplete
--     (gossip.go:1372-1475), player:Teleport/TeleportNear, player:HasAura.
--   * MISSING bridges that block classes here: player:GetQuestRewardStatus,
--     player:DestroyItemCount, player:HasItemCount (use HasItem/GetItemCount),
--     player:IsInSameRaidWith, go:GetOwner, go:CastSpell, go:AddUse,
--     GO-side go:SummonCreature (player/creature surfaces bridged).
--     Corrected 2026-10-24: player:CastSpell + player:SummonCreature +
--     player:GetQuestStatus bridged; this tick: go:GetGoType (lua_objects.go)
--     + player:GetNativeGender (lua_player.go) bridged;
--     GetClosestCreatureWithEntry rides the player:FindNearestCreature
--     twin (entry-filtered nearest == C++ closest). All verified zero hits in
--     engine/world/lua_player.go, engine/world/lua_objects.go,
--     engine/scripting/object.go (2026-10-18). Corrected 2026-10-24:
--     player:GetQuestStatus left this list — it IS bridged since
--     2026-10-24 (lua_player.go: NONE 0 / COMPLETE 1 / INCOMPLETE 3 /
--     FAILED 5 / REWARDED 6); classes below stay blocked on their
--     other gates + registration.
--     (2026-10-24: player:SummonCreature landed in lua_player.go; the
--     classes below stay blocked on their other gates + registration.)
--   * GO entry IDs come from the C++ tree only; classes whose C++ names no GO
--     entry const are not registered (DB-side binding, unverifiable).
--
-- Class index (:line = class start in go_scripts.cpp):
-- 1. go_gilded_brazier          :61   DOCUMENTED-ONLY
-- 2. go_tablet_of_the_seven     :94   DOCUMENTED-ONLY
-- 3. go_ethereum_prison         :142  DOCUMENTED-ONLY
-- 4. go_ethereum_stasis         :202  DOCUMENTED-ONLY
-- 5. go_resonite_cask           :238  DOCUMENTED-ONLY
-- 6. go_southfury_moonstone     :273  DOCUMENTED-ONLY
-- 7. go_tele_to_dalaran_crystal :312  DOCUMENTED-ONLY
-- 8. go_tele_to_violet_stand    :341  DOCUMENTED-ONLY
-- 9. go_matrix_punchograph      :386  PORTED 2026-10-24
-- 10. go_blood_filled_orb       :450  DOCUMENTED-ONLY
-- 11. go_soulwell               :495  DOCUMENTED-ONLY
-- 12. go_amberpine_outhouse     :594  DOCUMENTED-ONLY
-- 13. go_massive_seaforium_charge :650 DOCUMENTED-ONLY

-- ============================================================================
-- 1. go_gilded_brazier (go_scripts.cpp:61-92; OnGossipHello :76-86)
--    Enum: NPC_STILLBLADE = 17716, QUEST_THE_FIRST_TRIAL = 9678. No GO entry
--    const in C++ -> no registration (DB-side binding).
--    Arm: if go type == GAMEOBJECT_TYPE_GOOBER and
--    player->GetQuestStatus(9678) == QUEST_STATUS_INCOMPLETE (3),
--    player->SummonCreature(17716, 8106.11, -7542.06, 151.775, 3.02598,
--    TEMPSUMMON_DEAD_DESPAWN, 1min) then AttackStart(player). Returns true
--    (handled; native use suppressed).
--    Blockers: no registration; AttackStart(player) on the summoned
--    creature unbridged (go:GetGoType IS bridged 2026-10-24).
--    player:GetQuestStatus IS bridged since 2026-10-24 and
--    player:SummonCreature IS bridged since 2026-10-24. DOCUMENTED-ONLY.
-- ============================================================================

-- ============================================================================
-- 2. go_tablet_of_the_seven (go_scripts.cpp:94-125; OnGossipHello :110-120)
--    No GO entry const in C++ -> no registration (DB-side binding).
--    Arm: if go type != GAMEOBJECT_TYPE_QUESTGIVER return; if
--    player->GetQuestStatus(4296) == QUEST_STATUS_INCOMPLETE (3),
--    player->CastSpell(player, 15065, false). Returns true (handled).
--    C++ TODO notes a "Transcript the Tablet" gossip option pending
--    Trinity support. Blockers: no registration (no GO entry const) only —
--    player:CastSpell + go:GetGoType + player:GetQuestStatus all bridged
--    since 2026-10-24. DOCUMENTED-ONLY (bridge-complete, entry DB-side).
-- ============================================================================

-- ============================================================================
-- 3. go_ethereum_prison (go_scripts.cpp:142-200; OnGossipHello :155-198)
--    No GO entry const in C++ -> no registration (DB-side binding).
--    Arm: me->UseDoorOrButton(); summon random entry from NpcPrisonEntry[]
--    {22810,22811,22812,22813,22814,22815, 20783,20784,20785,20786,20788,
--     20789,20790} at GO position (facing player), TEMPSUMMON_TIMED_DESPAWN_
--     OUT_OF_COMBAT 30s. If summoned creature not hostile: faction -> rep
--     spell on player: 1011->39456, 935->39457, 942->39460, 933->39474,
--     989->39475, 970->39476. Returns false (native use continues).
--    Blockers: no registration (no GO entry const); player:SummonCreature
--    IS bridged since 2026-10-24 (go:UseDoorOrButton IS bridged,
--    but the summon is the script's observable effect). DOCUMENTED-ONLY.
-- ============================================================================

-- ============================================================================
-- 4. go_ethereum_stasis (go_scripts.cpp:202-236; OnGossipHello :218-230)
--    No GO entry const in C++ -> no registration (DB-side binding).
--    Arm: me->UseDoorOrButton(); player->SummonCreature(random of
--    NpcStasisEntry[] {22825,20888,22827,22826,22828} at GO position,
--    TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT 30s). Returns false.
--    Blocker: no registration (no GO entry const); player:SummonCreature
--    IS bridged since 2026-10-24. DOCUMENTED-ONLY.
-- ============================================================================

-- ============================================================================
-- 5. go_resonite_cask (go_scripts.cpp:238-271; OnGossipHello :251-260)
--    Enum: NPC_GOGGEROC = 11920. No GO entry const -> no registration.
--    Arm: if go type == GAMEOBJECT_TYPE_GOOBER,
--    me->SummonCreature(11920, 0,0,0,0, TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT,
--    5min). Returns false. Blockers: GO-side SummonCreature unbridged
--    (player:SummonCreature with 0,0,0,0 spawns at the player close point,
--    not the GO position; go:GetGoType IS bridged 2026-10-24). DOCUMENTED-ONLY.
-- ============================================================================

-- ============================================================================
-- 6. go_southfury_moonstone (go_scripts.cpp:273-310; OnGossipHello :283-294)
--    Enum: NPC_RIZZLE = 23002, SPELL_BLACKJACK = 39865 (stuns player),
--    SPELL_SUMMON_RIZZLE = 39866. No GO entry const -> no registration.
--    Arm: player->SummonCreature(23002, 0,0,0,0, TEMPSUMMON_DEAD_DESPAWN),
--    then creature->CastSpell(player, 39865, false). Returns false.
--    (C++ note: implicitTarget=48 spell summon path not implemented, manual
--    summon used.) Blocker: no registration (no GO entry const);
--    player:SummonCreature IS bridged since 2026-10-24. DOCUMENTED-ONLY.
-- ============================================================================

-- ============================================================================
-- 7. go_tele_to_dalaran_crystal (go_scripts.cpp:312-339; OnGossipHello :318-328)
--    Enum: QUEST_LEARN_LEAVE_RETURN = 12790, QUEST_TELE_CRYSTAL_FLAG = 12845.
--    No GO entry const -> no registration.
--    Arm: if player->GetQuestRewardStatus(12845) return false (native use
--    proceeds). Else player->GetSession()->SendNotification(
--    "This teleport crystal cannot be used until the teleport crystal in
--    Dalaran has been used at least once.") and return true (handled).
--    Blocker: player:GetQuestRewardStatus unbridged (player:SendNotification
--    IS bridged but the gate is not; C++ gate is IsQuestRewarded/seasonal,
--    not the GetQuestStatus value — no twin yet). DOCUMENTED-ONLY.
-- ============================================================================

-- ============================================================================
-- 8. go_tele_to_violet_stand (go_scripts.cpp:341-365; OnGossipHello :347-357)
--    Reuses enum above: QUEST_LEARN_LEAVE_RETURN = 12790. No GO entry const
--    -> no registration.
--    Arm: if player->GetQuestRewardStatus(12790) OR
--    player->GetQuestStatus(12790) == QUEST_STATUS_INCOMPLETE return false
--    (native use proceeds); else return true. Blocker: GetQuestRewardStatus
--    unbridged (GetQuestStatus IS bridged since 2026-10-24).
--    DOCUMENTED-ONLY.
-- ============================================================================

-- ============================================================================
-- 9. go_matrix_punchograph (go_scripts.cpp:386-448; OnGossipHello :390-421)
--    GO entries (C++-named, verified): 142345, 142475, 142476, 142696.
--    Items: white 9279, yellow 9280, red 9281, blue 9282, prismatic 9316.
--    Spells: yellow 11512, blue 11525, red 11528, prismatic 11545.
--    Arm per GO entry: if player has the matching punch card, DestroyItemCount
--    (card,1,true) then player->CastSpell(player, upgrade spell, true).
--    142345: white->yellow; 142475: yellow->blue; 142476: blue->red;
--    142696: red->prismatic. Returns false.
--    PORTED 2026-10-24 (see bottom of file): player:DestroyItemCount
--    IS bridged (lua_player.go, Player.cpp:12727 twin) and
--    player:CastSpell IS bridged since 2026-10-24; the HasItemCount gate
--    rides player:HasItem (GetItemCount >= 1). C++ returns false
--    (native use continues) == Lua false (gong polarity).
-- ============================================================================

-- ============================================================================
-- 10. go_blood_filled_orb (go_scripts.cpp:450-474; OnGossipHello :441-449)
--    Enum: NPC_ZELEMAR = 17830. No GO entry const -> no registration.
--    Arm: if go type == GAMEOBJECT_TYPE_GOOBER,
--    player->SummonCreature(17830, -369.746, 166.759, -21.50, 5.235,
--    TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT, 30s). Returns true (handled).
--    Blockers: no registration (no GO entry const) only;
--    go:GetGoType IS bridged 2026-10-24 and player:SummonCreature IS
--    bridged since 2026-10-24. DOCUMENTED-ONLY.
-- ============================================================================

-- ============================================================================
-- 11. go_soulwell (go_scripts.cpp:495-592; ctor :503-536, OnGossipHello :538-564)
--    GO entries (C++-named, verified): 181621 (R1), 193169 (R2).
--    Stone spells: master R0 34130 / R1 34149 / R2 34150; fel R0 58890 /
--    R1 58896 / R2 58898. Improved healthstone auras: 18692, 18693.
--    Ctor: latches _stoneSpell/_stoneId per GO entry and owner aura state
--    (18692 -> R1 variant; 34150-aura -> R2 variant), then reads the stone
--    ItemType from spell EFFECT_0.
--    OnGossipHello: needs GO owner that is a player in the same raid as the
--    user; if player already has the stone item, SendCastResult
--    SPELL_FAILED_TOO_MANY_OF_ITEM; else owner->CastSpell(player, _stoneSpell,
--    true), and me->AddUse() only if the item was actually created. Returns
--    false (gossip consumed on failure paths via `return true`).
--    Blockers: go:GetOwner unbridged (DB GO state carries no owner;
--    dynamic OwnerGUID not exposed on the Lua GO surface);
--    player:IsInSameRaidWith unbridged; owner->CastSpell unbridged;
--    go:AddUse unbridged (m_usetimes not modeled in Go);
--    Spell::SendCastResult unbridged. DOCUMENTED-ONLY despite C++-named entries.
-- ============================================================================

-- ============================================================================
-- 12. go_amberpine_outhouse (go_scripts.cpp:594-648;
--    OnGossipHello :599-613, OnGossipSelect :615-641)
--    Enum: ITEM_ANDERHOLS_SLIDER_CIDER = 37247, NPC_OUTHOUSE_BUNNY = 27326,
--    QUEST_DOING_YOUR_DUTY = 12227, SPELL_INDISPOSED = 53017,
--    SPELL_INDISPOSED_III = 48341, SPELL_CREATE_AMBERSEEDS = 48330,
--    menus 12775/12779. No GO entry const -> no registration.
--    Hello arm: quest 12227 status INCOMPLETE(3)/COMPLETE(1)/REWARDED(6) ->
--    AddGossipItemFor(player, GOSSIP_ICON_CHAT(0), "Use the outhouse.",
--    GOSSIP_SENDER_MAIN(1), GOSSIP_ACTION_INFO_DEF+1(100001)) +
--    SendGossipMenuFor(player, 12779, go); else SendGossipMenuFor(player,
--    12775, go). Returns true.
--    Select arm (action 100001): ClearGossipMenu, CloseGossipMenu,
--    GetClosestCreatureWithEntry(player, 27326, 3.0f) -> AI SetData(1,
--    player native gender) + go:CastSpell(bunny, 48341);
--    go:CastSpell(player, 53017); if player has item 37247 ->
--    go:CastSpell(player, 48330). Else branch sends notification
--    "Quest item Anderhol's Slider Cider not found." and returns false.
--    Bridged pieces: player:GossipMenuAddItem/GossipSendMenu/SendNotification
--    and the GO select hook signature (event, player, go, sender, action)
--    are bridged. Blockers: the hello gate needs player:GetQuestStatus
--    (bridged 2026-10-24 — the gate is portable, but the arm stays unregistered:
--    no GO entry const);
--    select needs creature AI SetData + go:CastSpell (unbridged);
--    GetClosestCreatureWithEntry rides player:FindNearestCreature and
--    player:GetNativeGender IS bridged 2026-10-24. DOCUMENTED-ONLY.
-- ============================================================================

-- ============================================================================
-- 13. go_massive_seaforium_charge (go_scripts.cpp:650-682;
--    OnGossipHello :658-664)
--    No GO entry const in C++ -> no registration (DB-side binding).
--    Arm: me->SetLootState(GO_JUST_DEACTIVATED); return true (handled).
--    GO_JUST_DEACTIVATED = 3 (GameObject.h:70-73: 0 GO_NOT_READY, 1 GO_READY,
--    2 GO_ACTIVATED, 3 GO_JUST_DEACTIVATED); go:SetLootState IS bridged
--    (lua_objects.go:1095, accepts 0-3), so this arm is FULLY bridgeable —
--    the Lua port would be:
--      -- RegisterGameObjectGossipEvent(<entry>, 1, function(_, _, go)
--      --     go:SetLootState(3) return false end)
--    held back only because the C++ names no GO entry and unverifiable
--    entries are not registered. DOCUMENTED-ONLY (registration-ready once a
--    C++-tree entry source is identified).
-- ============================================================================

-- 9. go_matrix_punchograph (go_scripts.cpp:386-448; OnGossipHello :390-421)
--    PORTED 2026-10-24: per GO entry, if the player carries the matching
--    punch card, DestroyItemCount(card, 1, true) then player self-casts
--    the upgrade spell triggered, in C++ order. C++ returns false
--    (native use continues) == Lua false (gong polarity).
local MATRIX_PUNCHOGRAPH_CARDS = {
    [142345] = { 9279, 11512 }, -- white -> yellow
    [142475] = { 9280, 11525 }, -- yellow -> blue
    [142476] = { 9282, 11528 }, -- blue -> red
    [142696] = { 9281, 11545 }, -- red -> prismatic
}

local function onMatrixPunchographGossip(_, player, go)
    local pair = MATRIX_PUNCHOGRAPH_CARDS[go:GetEntry()]
    if pair ~= nil and player:HasItem(pair[1]) then
        player:DestroyItemCount(pair[1], 1, true)
        player:CastSpell(player, pair[2], true)
    end
    return false
end

RegisterGameObjectGossipEvent(142345, 1, onMatrixPunchographGossip)
RegisterGameObjectGossipEvent(142475, 1, onMatrixPunchographGossip)
RegisterGameObjectGossipEvent(142476, 1, onMatrixPunchographGossip)
RegisterGameObjectGossipEvent(142696, 1, onMatrixPunchographGossip)
