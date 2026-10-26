package world

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

type gossipMenuItem struct {
	Icon         uint8
	Coded        bool
	BoxMoney     uint32
	Message      string
	BoxMessage   string
	Sender       uint32
	Action       uint32
	ActionMenuID uint32
	ActionPoiID  uint32
}

type gossipMenuState struct {
	SenderGUID uint64
	MenuID     uint32
	TitleID    uint32
	Items      map[uint32]gossipMenuItem
	Quests     []gossipQuestItem
}

type gossipQuestItem struct {
	ID           uint32
	Icon         uint32
	Level        int32
	Flags        uint32
	AutoComplete bool
	Title        string
}

func (s *session) handleGossipHello(ctx context.Context, payload []byte) bool {
	return s.helloCreatureNPC(ctx, payload, npcFlagGossip, false)
}

// helloCreatureNPC is the shared creature-hello flow behind CMSG_GOSSIP_HELLO
// and CMSG_QUESTGIVER_HELLO. The two C++ handlers differ in the NPC-flag gate
// (UNIT_NPC_FLAG_GOSSIP vs UNIT_NPC_FLAG_QUESTGIVER) and in the arms the
// questgiver path skips: it has no spirit-guide arm and no trainer/vendor/
// flightmaster/banker/tabard/auctioneer/stable service fallthroughs — it goes
// straight from the script hooks to PrepareGossipMenu + SendPreparedGossip
// (QuestHandler.cpp:78 vs NPCHandler.cpp:147).
func (s *session) helloCreatureNPC(ctx context.Context, payload []byte, requiredFlag uint32, questPath bool) bool {
	reader := protocol.NewReader(payload)
	guid, err := reader.ReadU64()
	if err != nil {
		s.debug("gossip hello rejected", "account", s.accountName, "error", err)
		return true
	}
	creature := s.luaCreature(ctx, guid)
	if creature == nil {
		s.debug("gossip hello unknown", "account", s.accountName, "guid", guid)
		return true
	}
	if !s.canInteractWithCreature(ctx, creature) {
		s.debug("gossip hello out of interaction range", "account", s.accountName, "guid", guid)
		return true
	}
	// GetNPCIfCanInteractWith(guid, flag) requires the matching NPC flag before
	// any script hook runs: UNIT_NPC_FLAG_GOSSIP for
	// WorldSession::HandleGossipHelloOpcode (NPCHandler.cpp:153),
	// UNIT_NPC_FLAG_QUESTGIVER for WorldSession::HandleQuestgiverHelloOpcode
	// (QuestHandler.cpp:78).
	npcFlags := objectUint32OrZero(creature, "NPCFlags")
	if npcFlags&requiredFlag == 0 {
		s.debug("npc hello rejected: missing required npc flag", "account", s.accountName, "guid", guid, "flag", requiredFlag)
		return true
	}
	entry, ok := objectUint32Field(creature, "Entry")
	if !ok {
		return true
	}
	if questPath {
		// WorldSession::HandleQuestgiverHelloOpcode (QuestHandler.cpp:90-92):
		// a feign-deathing player talking to a questgiver strips the fake
		// death (RemoveAurasByType(SPELL_AURA_FEIGN_DEATH) while
		// UNIT_STATE_DIED is set). The quest path has no TALK-interrupt
		// strip — that arm exists only in HandleGossipHelloOpcode.
		if s.hasAuraType(spellAuraFeignDeath) {
			s.removeAurasByType(spellAuraFeignDeath)
		}
	} else {
		// WorldSession::HandleGossipHelloOpcode (NPCHandler.cpp:162-163):
		// talking to the NPC makes its faction template's faction visible
		// in the reputation list (ReputationMgr::SetVisible).
		s.setGossipHelloFactionVisible(ctx, entry)
		// WorldSession::HandleGossipHelloOpcode (NPCHandler.cpp:164):
		// RemoveAurasWithInterruptFlags(AURA_INTERRUPT_FLAG_TALK) runs
		// right after the SetVisible arm, before the spirit-guide arm.
		s.removeAurasWithInterruptFlags(auraInterruptFlagTalk)
	}
	// Unit::PauseMovement + Creature::SetHomePosition run on BOTH paths:
	// HandleGossipHelloOpcode (NPCHandler.cpp:168-169) and
	// HandleQuestgiverHelloOpcode (QuestHandler.cpp:96-97).
	s.pauseCreatureMotionForHello(guid)
	// The spirit-guide arm exists only on the gossip path;
	// WorldSession::HandleQuestgiverHelloOpcode (QuestHandler.cpp:78) has none.
	// WorldSession::HandleGossipHelloOpcode (NPCHandler.cpp:171-185) checks
	// only the battleground — no death gate: a living player clicking a
	// spirit guide in a BG is queued for resurrection the same way.
	if !questPath && isBattlegroundMap(s.player.Map) && npcFlags&npcFlagSpiritGuide != 0 {
		// WorldSession::HandleGossipHelloOpcode (NPCHandler.cpp:171-185): a
		// spirit guide queues the ghost (== AddPlayerToResurrectQueue) and
		// answers with the wave timer (== SendAreaSpiritHealerQueryOpcode)
		// instead of opening a gossip menu. The queue handler itself sends no
		// time packet, so it goes out here, exactly where C++ sends it.
		if !s.handleAreaSpiritHealerQueue(ctx, payload) {
			return false
		}
		if s.inBattlegroundWaveMap() {
			s.sendAreaSpiritHealerTime(guid, s.server.spiritWaveTimeLeftMs())
		}
		return true
	}
	s.gossip = nil
	s.gossipClosed = false
	if s.server.Features != nil && s.server.Features.Scripts != nil {
		hookKind := "creature_gossip:" + strconv.FormatUint(uint64(entry), 10)
		values, terr := s.server.Features.Scripts.Trigger(ctx, hookKind, 1, uint32(1), s.luaPlayer(), creature)
		if terr != nil {
			s.debug("gossip hello hook failed", "account", s.accountName, "entry", entry, "error", terr)
		}
		// Eluna::OnGossipHello (GossipHooks.cpp:107-114) returns
		// CallAllFunctionsBool(..., true): true — vetoing the default menu —
		// when at least one hook is registered and no hook returned boolean
		// false. Both HandleQuestgiverHelloOpcode (QuestHandler.cpp:78) and
		// HandleGossipHelloOpcode (NPCHandler.cpp:147) return early on that
		// true. A script that sent its own menu already set s.gossip, so
		// this arm only changes the hook-registered-but-menu-unsent case
		// (C++ shows nothing; previously Go fell through to the default
		// menu).
		if s.server.Features.Scripts.HasHook(hookKind, 1) && !luaCancelled(values) {
			s.debug("gossip hello vetoed by lua", "account", s.accountName, "entry", entry)
			return true
		}
	}
	if s.gossip == nil && !s.gossipClosed {
		defaultMenu, err := s.prepareCreatureGossip(ctx, guid, entry, npcFlags, objectUint32OrZero(creature, "GossipMenuID"), true, true)
		if err != nil {
			s.debug("default gossip load failed", "account", s.accountName, "entry", entry, "error", err)
			s.gossipClosed = true
			_ = s.write(uint16(protocol.OpcodeSMSG_GOSSIP_COMPLETE), nil, true)
			return true
		}
		// The service fallthroughs exist only on the gossip path; the questgiver
		// hello never opens trainer/vendor/etc. windows (QuestHandler.cpp:78).
		if !questPath && defaultMenu != nil && len(defaultMenu.Items) == 0 && len(defaultMenu.Quests) == 0 {
			if npcFlags&0x70 != 0 { // UNIT_NPC_FLAG_TRAINER (0x10, 0x20, 0x40)
				return s.sendTrainerList(ctx, guid)
			}
			if npcFlags&0x1F80 != 0 { // UNIT_NPC_FLAG_VENDOR (0x80..0x1000)
				return s.sendVendorList(ctx, guid)
			}
			if npcFlags&0x2000 != 0 { // UNIT_NPC_FLAG_FLIGHTMASTER (0x2000)
				return s.sendTaxiMenu(ctx, guid)
			}
			if npcFlags&0x20000 != 0 { // UNIT_NPC_FLAG_BANKER (0x20000)
				bank := protocol.NewBuffer(8)
				bank.WriteU64(guid)
				return s.write(uint16(protocol.OpcodeSMSG_SHOW_BANK), bank.Bytes(), true) == nil
			}
			if npcFlags&0x80000 != 0 { // UNIT_NPC_FLAG_TABARDDESIGNER (0x80000)
				tabard := protocol.NewBuffer(8)
				tabard.WriteU64(guid)
				return s.write(uint16(protocol.OpcodeMSG_TABARDVENDOR_ACTIVATE), tabard.Bytes(), true) == nil
			}
			if npcFlags&0x200000 != 0 { // UNIT_NPC_FLAG_AUCTIONEER (0x200000)
				// Routes through WorldSession::SendAuctionHello
				// (AuctionHouseHandler.cpp:59-75): the 13-byte layout
				// (guid + u32 house id + u8 enabled) and the auction level
				// gate, not the bare 9-byte write this fallthrough had.
				s.gossipClosed = true
				s.sendAuctionHelloPacket(guid)
				return true
			}
			if npcFlags&0x400000 != 0 { // UNIT_NPC_FLAG_STABLEMASTER (0x400000)
				stableBuf := protocol.NewBuffer(8)
				stableBuf.WriteU64(guid)
				return s.handleListStabledPets(ctx, stableBuf.Bytes())
			}
		}
		// Player::SendPreparedGossip (Player.cpp:14536-14548): on the
		// questgiver-hello path a flagless questgiver with a non-empty quest
		// menu goes through SendPreparedQuest (Player.cpp:14816) — a single
		// quest fast-opens, multiple quests go out in
		// SMSG_QUESTGIVER_QUEST_LIST. The quest menu takes precedence over
		// gossip items (!questMenu.Empty() is the whole gate); with an empty
		// quest menu it falls through to SendGossipMenu. The gossip-hello
		// path always carries the flag (gated above), so it always sends the
		// menu.
		if questPath && npcFlags&npcFlagGossip == 0 && defaultMenu != nil && len(defaultMenu.Quests) != 0 {
			return s.sendPreparedCreatureQuest(ctx, guid, entry, defaultMenu.Quests)
		}
		s.gossip = defaultMenu
		if err := s.sendGossipMenu(); err != nil {
			s.debug("gossip hello response failed", "account", s.accountName, "entry", entry, "error", err)
			return true
		}
	}
	s.debug("npc hello handled", "account", s.accountName, "entry", entry, "questPath", questPath)
	return true
}

// setGossipHelloFactionVisible mirrors the faction-visibility arm of
// WorldSession::HandleGossipHelloOpcode (NPCHandler.cpp:162-163): talking to
// a creature makes its faction template's faction visible in the reputation
// list. ReputationMgr::SetVisible resolves the faction template to the
// faction id and no-ops when the faction can't have reputation, so the
// template lookup here feeds the same setFactionVisible setter the quest
// accept path uses (ReputationMgr.cpp:445-460).
func (s *session) setGossipHelloFactionVisible(ctx context.Context, entry uint32) {
	if s == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil || s.server.Data == nil || entry == 0 {
		return
	}
	var factionTemplate int64
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(faction, 0) FROM creature_template WHERE entry = ?", entry).Scan(&factionTemplate); err != nil || factionTemplate == 0 {
		return
	}
	ft, found, err := s.server.Data.FactionTemplate(uint32(factionTemplate))
	if err != nil || !found || ft.Faction == 0 {
		return
	}
	s.setFactionVisible(ctx, ft.Faction)
}

// pauseCreatureMotionForHello mirrors the stop arms shared by the two hello
// handlers: Unit::PauseMovement(CONFIG_CREATURE_STOP_FOR_PLAYER) +
// Creature::SetHomePosition(GetPosition()) in HandleGossipHelloOpcode
// (NPCHandler.cpp:168-169) and HandleQuestgiverHelloOpcode
// (QuestHandler.cpp:96-97). The default stop time is 3 minutes
// (Creature.MovingStopTimeForPlayer, World.cpp:1230). C++ stops the spline
// in place and pauses the current generator; Go cancels the current wander
// leg and holds WaitUntil so no new leg starts until the pause expires, and
// re-bases the home position at the current spot. Mid-chase NPCs keep their
// pursuit state (chase runs outside the wander WaitUntil gate).
func (s *session) pauseCreatureMotionForHello(guid uint64) {
	if s == nil || s.server == nil || s.player == nil {
		return
	}
	s.server.motionMu.Lock()
	defer s.server.motionMu.Unlock()
	motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, guid)
	if motion == nil {
		return
	}
	motion.Moving = false
	motion.WaitUntil = time.Now().Add(3 * time.Minute)
	motion.HomeX, motion.HomeY, motion.HomeZ = motion.X, motion.Y, motion.Z
}

func (s *session) handleGossipSelectOption(ctx context.Context, payload []byte) bool {
	reader := protocol.NewReader(payload)
	guid, err := reader.ReadU64()
	if err != nil {
		// HandleGossipSelectOptionOpcode (MiscHandler.cpp:94) never drops the
		// session for a malformed select — it silently ignores it. A false
		// return here ends the packet loop (server.go), i.e. a client
		// disconnect on a short packet; C++ has no such disconnect.
		s.debug("gossip selection malformed", "account", s.accountName, "error", err)
		return true
	}
	menuID, err := reader.ReadU32()
	if err != nil {
		s.debug("gossip selection rejected", "account", s.accountName, "reason", "missing menu", "error", err)
		return true
	}
	listID, err := reader.ReadU32()
	if err != nil {
		s.debug("gossip selection rejected", "account", s.accountName, "reason", "missing list", "error", err)
		return true
	}
	s.debug("gossip selection received", "account", s.accountName, "guid", guid, "menu", menuID, "list", listID)
	if uint16(guid>>48) == 0x4000 {
		return s.handleItemGossipSelectOption(ctx, reader, guid, listID)
	}
	// HandleGossipSelectOptionOpcode (MiscHandler.cpp:131-136): the sender
	// gate is the only cheat check shared by every arm; the menuId check
	// runs up front only for the player arm (MiscHandler.cpp:146-151) —
	// for creature/gameobject sources it lives inside
	// Player::OnGossipSelect (Player.cpp:14565-14569), after the Eluna/AI
	// hooks fire, so a stale-menu select still reaches the hooks.
	if s.gossip == nil || s.gossip.SenderGUID != guid {
		s.debug("gossip selection rejected", "account", s.accountName, "guid", guid, "menu", menuID, "list", listID)
		return true
	}
	item, ok := s.gossip.Items[listID]
	if !ok {
		return true
	}
	code := ""
	if item.Coded {
		code, err = reader.ReadCString()
		if err != nil {
			s.debug("gossip selection rejected", "account", s.accountName, "guid", guid, "menu", menuID, "list", listID, "reason", "malformed code", "error", err)
			return true
		}
	}
	// HandleGossipSelectOptionOpcode (MiscHandler.cpp:131-156): gameobject and
	// player GUIDs take their own arms — the Eluna gameobject select hook and
	// the ScriptMgr player select hook — instead of the creature path below.
	// Go exposes both via RegisterGameObjectGossipEvent and
	// RegisterPlayerGossipEvent, and Lua can open menus on either source via
	// GossipSendMenu, so selects on them must route here rather than drop at
	// the creature lookup.
	if uint16(guid>>48) == 0xF110 {
		return s.handleGameObjectGossipSelect(ctx, guid, menuID, listID, item, code)
	}
	if guid == s.playerGUID {
		return s.handlePlayerGossipSelect(ctx, menuID, item, code)
	}
	creature := s.luaCreature(ctx, guid)
	if creature == nil {
		return true
	}
	if !s.canInteractWithCreature(ctx, creature) {
		s.debug("gossip selection out of interaction range", "account", s.accountName, "guid", guid)
		return true
	}
	// HandleGossipSelectOptionOpcode (MiscHandler.cpp:119) uses
	// GetNPCIfCanInteractWith(guid, UNIT_NPC_FLAG_GOSSIP): the flag gate
	// applies to selections exactly as it does to hellos.
	npcFlags := objectUint32OrZero(creature, "NPCFlags")
	if npcFlags&npcFlagGossip == 0 {
		s.debug("gossip selection rejected: npc lacks gossip flag", "account", s.accountName, "guid", guid)
		return true
	}
	// MiscHandler.cpp:162-164 (HandleGossipSelectOptionOpcode) — the
	// feign-death strip runs after the creature validation, before dispatch.
	s.removeFakeDeath()
	entry, ok := objectUint32Field(creature, "Entry")
	if !ok {
		return true
	}
	// Player::OnGossipSelect's OUTDOORPVP arm (Player.cpp:14618-14620) sends no
	// close: the menu stays open. Capture the live menu so that arm can
	// restore it below.
	prevGossip := s.gossip
	s.gossip = nil
	s.gossipClosed = false
	// Creature select Lua args are (event, player, creature, sender, action,
	// code, listID). Code is always forwarded ("" when the option has no code
	// box) so listID sits at a stable trailing position, mirroring the
	// gameobject select hook (fireGameObjectGossipSelectHook); Eluna's
	// OnGossipSelectCode-vs-OnGossipSelect dispatch on !code.empty()
	// (MiscHandler.cpp:175) is not reproduced — no ported handler reads
	// code, and a varying arg count would make listID unreadable.
	// listID is the CMSG_GOSSIP_SELECT_OPTION menu index — the C++
	// gossipListId CreatureAI::OnGossipSelect receives — so arms that
	// branch on gossipListId directly (halls of reflection start/skip,
	// forge intro, grandmother/majordomo/akama listId-0 gates) port
	// directly; sender/action remain the GetGossipOptionSender/Action
	// twins. menuID is intentionally not forwarded: the post-hook
	// stale-menu gate below already enforces it.
	args := []any{uint32(2), s.luaPlayer(), creature, item.Sender, item.Action, code, listID}
	if s.server.Features != nil && s.server.Features.Scripts != nil {
		// HandleGossipSelectOptionOpcode (MiscHandler.cpp:175-204, 209):
		// the AI and native arms run only when the Eluna hook did NOT veto
		// (`if (!sEluna->OnGossipSelect(...))` then AI + native arms), and
		// the Eluna hook returns false iff a handler returned boolean
		// false (CallAllFunctionsBool(..., true), LuaEngine/HookHelpers.h:92).
		// So a Lua false return lets the AI and native select arms
		// PROCEED; true/nothing vetoes them — the uniform Eluna gossip
		// polarity, same as the creature hello hook above and the
		// gameobject hooks. The menu was already cleared (s.gossip = nil
		// above, matching Eluna's ClearMenus-before-hooks), so a veto
		// returns with it cleared and sends no close packet — ClearMenus
		// (GossipDef.cpp:185) is server-side only. (An older revision had
		// this inverted — Lua false vetoing; corrected 2026-10-19 against
		// the in-tree Eluna sources.)
		hookKind := "creature_gossip:" + strconv.FormatUint(uint64(entry), 10)
		values, terr := s.server.Features.Scripts.Trigger(ctx, hookKind, 2, args...)
		if terr != nil {
			s.debug("gossip selection hook failed", "account", s.accountName, "entry", entry, "error", terr)
		}
		if s.server.Features.Scripts.HasHook(hookKind, 2) && !luaCancelled(values) {
			s.debug("gossip selection vetoed by lua", "account", s.accountName, "entry", entry, "list", listID)
			return true
		}
	}
	if s.gossip == nil && !s.gossipClosed {
		// Player::OnGossipSelect (Player.cpp:14565-14569): the menuId gate
		// runs after the Eluna/AI hooks — a stale select fires the hooks
		// but never reaches the native arms, and the live menu stays open.
		if menuID != prevGossip.MenuID {
			s.debug("gossip selection rejected: stale menu", "account", s.accountName, "guid", guid, "menu", menuID)
			s.gossip = prevGossip
			return true
		}
		// Player::OnGossipSelect (Player.cpp:14595-14601): the option's
		// BoxMoney is checked for sufficiency up front — insufficient funds
		// send BUY_ERR_NOT_ENOUGHT_MONEY and close the menu — but the charge
		// itself (ModifyMoney(-cost), Player.cpp:14699) lands AFTER the
		// option arms, so arms that return early never charge.
		boxCost := item.BoxMoney
		if boxCost > 0 {
			if s.player == nil || s.player.Money < boxCost {
				_ = s.write(uint16(protocol.OpcodeSMSG_BUY_FAILED), buildBuyFailed(0, 0, buyErrNotEnoughMoney), true)
				s.sendGossipComplete()
				return true
			}
		}
		// TrinityCore Gossip_Option (GossipDef.h:34-54): 0 none, 1 gossip
		// submenu, 2 questgiver (quest menu), 3 vendor, 4 taxivendor,
		// 5 trainer, 6 spirithealer, 7 spiritguide, 8 innkeeper, 9 banker,
		// 10 petitioner, 11 tabarddesigner, 12 battlefield, 13 auctioneer,
		// 14 stablepet, 15 armorer, 16 unlearntalents, 17 unlearnpettalents,
		// 18 learndualspec, 19 outdoorpvp, 20 dualspec_info. Note 19 is
		// OUTDOORPVP, not DUALSPEC_INFO — an earlier revision mapped 19 to
		// the submenu arm.
		if item.Action == 3 || item.Action == 15 { // GOSSIP_OPTION_VENDOR / GOSSIP_OPTION_ARMORER
			// Player::OnGossipSelect (Player.cpp:14629-14631): SendListInventory
			// with no close — the tail restores the live menu.
			s.sendVendorList(ctx, guid)
		} else if item.Action == 6 { // GOSSIP_OPTION_SPIRITHEALER
			// Player::OnGossipSelect (Player.cpp:14621-14624): the spirit
			// healer casts 17251 (triggered, original caster = the player) on
			// itself; the spell's resurrect effect registers a resurrect
			// request (Spell::EffectResurrect, SpellEffects.cpp:4270-4293).
			// No confirm dialog: SMSG_SPIRIT_HEALER_CONFIRM is STATUS_NEVER in
			// Opcodes.cpp:677 — the confirm/activate flow nothing sends — and
			// no gossip close (Player.cpp:14621-14624), so the tail restores
			// the live menu. The cast visual
			// (SMSG_SPELL_START/GO from the creature) has no Go bridge; the
			// mechanic is what lands here.
			// Player::OnGossipSelect GOSSIP_OPTION_SPIRITHEALER arm
			// (Player.cpp:14621-14624): casts 17251 on isDead() only — no
			// npcflag check (the menu option is already gated by the
			// flag at menu build) and no pending-request check.
			if s.isDeadOrGhost() && s.player != nil {
				pct := uint32(50)
				ignoreReclaim := false
				if s.server != nil && s.server.Data != nil {
					if sp, found, derr := s.server.Data.Spell(17251); derr == nil && found && len(sp.Effects) > 0 {
						if v := sp.Effects[0].CalcValueForLevel(sp, uint32(s.player.Level)); v >= 1 && v <= 100 {
							pct = uint32(v)
						}
						ignoreReclaim = sp.AttributesEx3&spellAttr3IgnoreResurrectionTimer != 0
					}
				}
				health := uint32(int64(s.player.MaxHealth) * int64(pct) / 100)
				mana := uint32(int64(s.player.MaxPowers[0]) * int64(pct) / 100)
				name, _ := creature.Fields["Name"].(string)
				x, _ := objectFloat32Field(creature, "X")
				y, _ := objectFloat32Field(creature, "Y")
				z, _ := objectFloat32Field(creature, "Z")
				// Spell::ExecuteLogEffectResurrect (Spell.cpp:4617-4621) as
				// flushed by SendLogExecute: caster is the creature, and the
				// log broadcasts to nearby like C++ (was session-only).
				// SetResurrectRequestData WorldRelocates the caster: the
				// stored location is the spirit healer's position (the accept
				// path teleports the ghost there before resurrecting).
				s.setResurrectRequestData(guid, objectUint32OrZero(creature, "Map"), x, y, z, health, mana, 0)
				// Spell::SendResurrectRequest (Spell.cpp:4693-4709): creature
				// caster name, spirit-healer sickness flag set, reclaim-timer
				// override from the spell's attributes. The request goes first:
				// C++ SendDirectMessages it during EffectResurrect while the
				// log is only flushed in FinishTargetProcessing (Spell.cpp:3612).
				s.sendResurrectRequest(guid, name, true, !ignoreReclaim)
				// Spell::ExecuteLogEffectResurrect (Spell.cpp:4617-4621) as
				// flushed by SendLogExecute: caster is the creature, and the
				// log broadcasts to nearby like C++ (was session-only).
				s.sendResurrectLog(17251, guid, s.playerGUID)
				// No gossip close (Player.cpp:14621-14624) — the tail restores
				// the live menu.
			}
		} else if item.Action == 4 { // GOSSIP_OPTION_TAXIVENDOR
			// Player::OnGossipSelect (Player.cpp:14659-14661): SendTaxiMenu
			// with no close — the tail restores the live menu.
			s.sendTaxiMenu(ctx, guid)
		} else if item.Action == 5 { // GOSSIP_OPTION_TRAINER
			// Player::OnGossipSelect (Player.cpp:14636-14638): SendTrainerList
			// with no close — the tail restores the live menu.
			s.sendTrainerList(ctx, guid)
		} else if item.Action == 8 { // GOSSIP_OPTION_INNKEEPER
			// Player::OnGossipSelect (Player.cpp:14663-14666): SendCloseGossip
			// runs before Player::SetBindPoint, which itself only sends
			// SMSG_BINDER_CONFIRM (Player.cpp:9597-9600) — the bind lands on
			// CMSG_BINDER_ACTIVATE.
			if !s.sendGossipComplete() {
				return true
			}
			bind := protocol.NewBuffer(8)
			bind.WriteU64(guid)
			if err := s.write(uint16(protocol.OpcodeSMSG_BINDER_CONFIRM), bind.Bytes(), true); err != nil {
				return true
			}
		} else if item.Action == 9 { // GOSSIP_OPTION_BANKER
			// Player::OnGossipSelect (Player.cpp:14662-14664): SendShowBank
			// with no close — the tail restores the live menu.
			bank := protocol.NewBuffer(8)
			bank.WriteU64(guid)
			if err := s.write(uint16(protocol.OpcodeSMSG_SHOW_BANK), bank.Bytes(), true); err != nil {
				return true
			}
		} else if item.Action == 11 { // GOSSIP_OPTION_TABARDDESIGNER
			// Player::OnGossipSelect (Player.cpp:14669-14671): SendCloseGossip
			// before MSG_TABARDVENDOR_ACTIVATE.
			if !s.sendGossipComplete() {
				return true
			}
			tabard := protocol.NewBuffer(8)
			tabard.WriteU64(guid)
			if err := s.write(uint16(protocol.OpcodeMSG_TABARDVENDOR_ACTIVATE), tabard.Bytes(), true); err != nil {
				return true
			}
		} else if item.Action == 13 { // GOSSIP_OPTION_AUCTIONEER
			// Player::OnGossipSelect (Player.cpp:14672-14674) routes through
			// WorldSession::SendAuctionHello (AuctionHouseHandler.cpp:59-75):
			// the packet is guid + u32 house id + u8 enabled behind the
			// auction level gate — not the bare 9-byte write this arm had.
			// Player::OnGossipSelect (Player.cpp:14672-14674): no close — the
			// tail restores the live menu.
			s.sendAuctionHelloPacket(guid)
		} else if item.Action == 14 { // GOSSIP_OPTION_STABLEPET
			// Player::OnGossipSelect (Player.cpp:14632-14634): SendStablePet
			// with no close — the menu stays live for subsequent selects.
			s.gossip = prevGossip
			stableBuf := protocol.NewBuffer(8)
			stableBuf.WriteU64(guid)
			stableOK := s.handleListStabledPets(ctx, stableBuf.Bytes())
			// Player::OnGossipSelect (Player.cpp:14699): the stablepet
			// arm breaks, so the BoxMoney charge still lands — this
			// arm used to return before it.
			s.chargeGossipBoxMoney(ctx, boxCost)
			return stableOK
		} else if item.Action == 16 { // GOSSIP_OPTION_UNLEARNTALENTS
			// Player::OnGossipSelect (Player.cpp:14649-14651): SendCloseGossip
			// (SMSG_GOSSIP_COMPLETE) goes out BEFORE SendTalentWipeConfirm —
			// the menu must close before the wipe-confirm dialog opens.
			if !s.sendGossipComplete() {
				return true
			}
			wipeBuf := protocol.NewBuffer(12)
			wipeBuf.WriteU64(guid)
			// Player::SendTalentWipeConfirm (Player.cpp:9605): the prompt shows
			// the same escalating ResetTalentsCost the confirm handler charges,
			// zeroed by the NoResetTalentsCost custom switch (Player.cpp:9607).
			wipeCost := s.resetTalentsCost()
			if s.server.Config.NoResetTalentsCost {
				wipeCost = 0
			}
			wipeBuf.WriteU32(wipeCost)
			if err := s.write(uint16(protocol.OpcodeMSG_TALENT_WIPE_CONFIRM), wipeBuf.Bytes(), true); err != nil {
				return true
			}
		} else if item.Action == 1 || item.Action == 20 { // GOSSIP_OPTION_GOSSIP / GOSSIP_OPTION_DUALSPEC_INFO
			// Player::OnGossipSelect (Player.cpp:14604-14615): the POI fires
			// before the submenu, for both option types.
			if item.ActionPoiID != 0 {
				s.sendGossipPOI(ctx, item.ActionPoiID)
			}
			if item.ActionMenuID != 0 {
				defaultMenu, loadErr := s.prepareCreatureGossip(ctx, guid, entry, objectUint32OrZero(creature, "NPCFlags"), item.ActionMenuID, false, false)
				if loadErr != nil {
					s.debug("gossip submenu load failed", "account", s.accountName, "entry", entry, "menu", item.ActionMenuID, "error", loadErr)
					s.gossipClosed = true
					_ = s.write(uint16(protocol.OpcodeSMSG_GOSSIP_COMPLETE), nil, true)
					return true
				}
				s.gossip = defaultMenu
				if sendErr := s.sendGossipMenu(); sendErr != nil {
					s.debug("gossip submenu response failed", "account", s.accountName, "entry", entry, "menu", item.ActionMenuID, "error", sendErr)
					return true
				}
			}
		} else if item.Action == 2 { // GOSSIP_OPTION_QUESTGIVER
			// Player::OnGossipSelect (Player.cpp:14625-14628):
			// PrepareQuestMenu(guid) + SendPreparedQuest(guid). An empty menu
			// sends nothing, a single quest fast-opens, and multiple quests go
			// out in SMSG_QUESTGIVER_QUEST_LIST — not in a gossip message (the
			// gossip-message quest section is the hello path's shape).
			if s.player != nil {
				quests, qerr := s.loadCreatureQuestMenu(ctx, entry, s.player.Level)
				if qerr != nil {
					s.debug("gossip questgiver menu load failed", "account", s.accountName, "entry", entry, "error", qerr)
					s.gossipClosed = true
					_ = s.write(uint16(protocol.OpcodeSMSG_GOSSIP_COMPLETE), nil, true)
					return true
				}
				if !s.sendPreparedCreatureQuest(ctx, guid, entry, quests) {
					return false
				}
				// Player::OnGossipSelect (Player.cpp:14625-14628): no close —
				// the tail restores the live menu.
			}
		} else if item.Action == 7 { // GOSSIP_OPTION_SPIRITGUIDE
			// Player::OnGossipSelect (Player.cpp:14677-14680): re-prepare the
			// creature's default gossip menu without the quest list
			// (PrepareGossipMenu(source) passes showQuests=false).
			defaultMenu, loadErr := s.prepareCreatureGossip(ctx, guid, entry, npcFlags, 0, false, false)
			if loadErr != nil {
				s.debug("gossip spiritguide menu load failed", "account", s.accountName, "entry", entry, "error", loadErr)
				s.gossipClosed = true
				_ = s.write(uint16(protocol.OpcodeSMSG_GOSSIP_COMPLETE), nil, true)
				return true
			}
			defaultMenu.Quests = nil
			s.gossip = defaultMenu
			if sendErr := s.sendGossipMenu(); sendErr != nil {
				s.debug("gossip spiritguide response failed", "account", s.accountName, "entry", entry, "error", sendErr)
				return true
			}
		} else if item.Action == 10 { // GOSSIP_OPTION_PETITIONER
			// Player::OnGossipSelect (Player.cpp:14666-14668): SendCloseGossip
			// runs before SendPetitionShowList. Go has no petition-show model
			// (only arena-charter deletion), so only the close is bridged.
			if !s.sendGossipComplete() {
				return true
			}
		} else if item.Action == 12 { // GOSSIP_OPTION_BATTLEFIELD
			// Player::OnGossipSelect (Player.cpp:14684-14696): the battleground
			// type is derived from the creature entry via
			// BattlegroundMgr::GetBattleMasterBG; an unknown entry logs and
			// returns before the BoxMoney charge below.
			bgTypeID := s.battlemasterBGType(ctx, entry)
			if bgTypeID == battlegroundTypeNone {
				s.debug("gossip battlefield invalid creature", "account", s.accountName, "entry", entry)
				// C++ logs and returns before the BoxMoney charge with the
				// menu still live — restore it instead of dropping it.
				s.gossip = prevGossip
				return true
			}
			// Player::OnGossipSelect (Player.cpp:14684-14696): no close — the
			// tail restores the live menu.
			s.sendBattlefieldList(guid, 0, bgTypeID)
		} else if item.Action == 17 { // GOSSIP_OPTION_UNLEARNPETTALENTS
			// Player::OnGossipSelect (Player.cpp:14653-14656): SendCloseGossip
			// before Player::ResetPetTalents (Player.cpp:9612) — the active
			// hunter pet's learned talent spells are unlearned and the talent
			// frames resent. The "Yes, please do." confirmation is
			// data-driven (gossip menu text), not code.
			if !s.sendGossipComplete() {
				return true
			}
			s.resetActivePetTalents(ctx)
		} else if item.Action == 18 { // GOSSIP_OPTION_LEARNDUALSPEC
			// Player::OnGossipSelect (Player.cpp:14636-14646): gated on a
			// single spec and CONFIG_MIN_DUALSPEC_LEVEL (40, World.cpp
			// default); the two casts teach dual spec (the spells.go
			// 63624/63680 arm flips talentGroupsCount to 2 and refreshes the
			// talent frame) and the menu re-prepares to the option's action
			// menu.
			if s.player != nil && s.player.TalentGroupsCount == 1 && s.player.Level >= 40 {
				s.castSpellDirect(ctx, 63680, s.playerGUID)
				s.castSpellDirect(ctx, 63624, s.playerGUID)
				defaultMenu, loadErr := s.prepareCreatureGossip(ctx, guid, entry, npcFlags, item.ActionMenuID, false, false)
				if loadErr != nil {
					s.debug("gossip dualspec menu load failed", "account", s.accountName, "entry", entry, "menu", item.ActionMenuID, "error", loadErr)
					s.gossipClosed = true
					_ = s.write(uint16(protocol.OpcodeSMSG_GOSSIP_COMPLETE), nil, true)
					return true
				}
				s.gossip = defaultMenu
				if sendErr := s.sendGossipMenu(); sendErr != nil {
					s.debug("gossip dualspec menu response failed", "account", s.accountName, "entry", entry, "menu", item.ActionMenuID, "error", sendErr)
					return true
				}
			}
		} else if item.Action == 19 { // GOSSIP_OPTION_OUTDOORPVP
			// Player::OnGossipSelect (Player.cpp:14618-14620): routed to the
			// OutdoorPvP manager, which has no Go model — the gossip menu
			// stays open (C++ sends no close on this arm).
			s.gossip = prevGossip
		}
		// Player::OnGossipSelect (Player.cpp:14699): the BoxMoney charge lands
		// after the option arms — early-return arms (invalid battlemaster
		// above) never charge.
		s.chargeGossipBoxMoney(ctx, boxCost)
	}
	if s.gossip == nil && !s.gossipClosed {
		// Player::OnGossipSelect arms that send no packet leave the live
		// menu open (C++ never closes the menu for them) — restore it
		// instead of closing the client's window.
		s.gossip = prevGossip
	}
	s.debug("gossip selection handled", "account", s.accountName, "entry", entry, "list", listID)
	return true
}

// chargeGossipBoxMoney applies the Player::OnGossipSelect tail charge
// (Player.cpp:14699): ModifyMoney(-cost) after the option arms, with the
// money-change notification, characters-row write, and player update.
// Arms that return from inside the option switch (stablepet, GO questgiver)
// call it explicitly — the shared tail never runs for them.
func (s *session) chargeGossipBoxMoney(ctx context.Context, boxCost uint32) {
	if boxCost == 0 || s.player == nil {
		return
	}
	s.player.Money -= boxCost
	s.questMoneyChanged(ctx)
	if cdb := s.server.CharactersStore.DB; cdb != nil {
		_, _ = cdb.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", s.player.Money, s.playerGUID)
	}
	s.sendPlayerUpdate()
}

// handleGameObjectGossipSelect processes CMSG_GOSSIP_SELECT_OPTION for a
// gameobject GUID — the gameobject arm of
// WorldSession::HandleGossipSelectOptionOpcode (MiscHandler.cpp:131-136,
// 168-173, 198-204). listID is the packet's menu index (the C++ gossipListId);
// it is threaded to the select hook, which forwards it to Lua past the code
// argument, because table-indexing select arms need it.
func (s *session) handleGameObjectGossipSelect(ctx context.Context, guid uint64, menuID uint32, listID uint32, item gossipMenuItem, code string) bool {
	if !s.playerLoaded || s.player == nil || s.server == nil {
		return true
	}
	entry := uint32((guid >> 24) & 0x00FFFFFF)
	low := uint32(guid & 0x00FFFFFF)
	// GetGameObjectIfCanInteractWith (Player.cpp:2363-2398): the gameobject
	// must exist, not use the "Point" icon, and be within interaction range
	// on the player's map and instance — the handleGameObjectUse gates.
	goState, err := s.server.getOrLoadGameObjectState(ctx, guid, low, entry, s.player.Map, s.player.InstanceID)
	if err != nil || goState == nil || goState.IconName == "Point" {
		return true
	}
	maxUseDist := 10.0
	switch goState.Type {
	case GameObjectTypeFishingNode:
		maxUseDist = 100.0
	case GameObjectTypeFishingHole:
		maxUseDist = 20.5
	}
	if goState.Map != s.player.Map || goState.InstanceID != s.player.InstanceID ||
		distance3D(s.player.X, s.player.Y, s.player.Z, goState.X, goState.Y, goState.Z) > maxUseDist {
		return true
	}
	// MiscHandler.cpp:162-164 (HandleGossipSelectOptionOpcode) — the
	// feign-death strip runs after the gameobject validation, before the
	// Eluna select hook.
	s.removeFakeDeath()
	// The select hook nils the live menu before firing (like the creature
	// path), so capture it for the menuId gate and the no-close restore
	// below.
	prevGossip := s.gossip
	// Eluna::OnGossipSelect[Code] fires before the native arms; a true
	// hook return (no handler returned false) skips them
	// (MiscHandler.cpp:198-204, 225) — i.e. a Lua false return lets the
	// native arms proceed (Eluna polarity; see
	// fireGameObjectGossipSelectHook).
	if s.fireGameObjectGossipSelectHook(ctx, guid, item.Sender, item.Action, code, listID) {
		return true
	}
	// Player::OnGossipSelect (Player.cpp:14565-14569): for gameobject
	// sources the menuId gate runs after the hook — a stale select fires
	// the hook but never reaches the native arms, and the live menu stays
	// open.
	if menuID != prevGossip.MenuID {
		s.debug("gameobject gossip selection rejected: stale menu", "account", s.accountName, "entry", entry, "menu", menuID)
		s.gossip = prevGossip
		return true
	}
	// Player::OnGossipSelect (Player.cpp:14580-14600): gameobjects accept
	// only options up to GOSSIP_OPTION_QUESTGIVER; the BoxMoney sufficiency
	// check runs before the arms, the charge after them.
	if item.Action > 2 {
		s.debug("gameobject gossip selection rejected: invalid option", "account", s.accountName, "entry", entry, "action", item.Action)
		return true
	}
	boxCost := item.BoxMoney
	if boxCost > 0 {
		if s.player.Money < boxCost {
			_ = s.write(uint16(protocol.OpcodeSMSG_BUY_FAILED), buildBuyFailed(0, 0, buyErrNotEnoughMoney), true)
			s.sendGossipComplete()
			return true
		}
	}
	switch item.Action {
	case 1, 20: // GOSSIP_OPTION_GOSSIP / GOSSIP_OPTION_DUALSPEC_INFO
		// Player::OnGossipSelect (Player.cpp:14604-14616): the POI fires
		// before the submenu. The submenu re-prepares the gameobject's menu
		// with showQuests=false.
		if item.ActionPoiID != 0 {
			s.sendGossipPOI(ctx, item.ActionPoiID)
		}
		if item.ActionMenuID != 0 {
			submenu, loadErr := s.prepareGameObjectGossip(ctx, guid, entry, item.ActionMenuID, s.gameObjectDefaultGossipMenuID(ctx, entry, goState.Type), false)
			if loadErr != nil {
				s.debug("gameobject gossip submenu load failed", "account", s.accountName, "entry", entry, "menu", item.ActionMenuID, "error", loadErr)
				s.gossipClosed = true
				_ = s.write(uint16(protocol.OpcodeSMSG_GOSSIP_COMPLETE), nil, true)
				return true
			}
			s.gossip = submenu
			if sendErr := s.sendGossipMenu(); sendErr != nil {
				s.debug("gameobject gossip submenu response failed", "account", s.accountName, "entry", entry, "error", sendErr)
				return true
			}
		}
	case 2: // GOSSIP_OPTION_QUESTGIVER
		// Player::OnGossipSelect (Player.cpp:14625-14628):
		// PrepareQuestMenu(guid) + SendPreparedQuest(guid) — the gameobject
		// quest relations resolve through the GO entry.
		quests, qerr := s.loadGameObjectQuestMenu(ctx, entry, s.player.Level)
		if qerr != nil {
			s.debug("gameobject questgiver menu load failed", "account", s.accountName, "entry", entry, "error", qerr)
			s.gossipClosed = true
			_ = s.write(uint16(protocol.OpcodeSMSG_GOSSIP_COMPLETE), nil, true)
			return true
		}
		questOK := s.sendPreparedGameObjectQuest(ctx, guid, entry, quests)
		// Player::OnGossipSelect (Player.cpp:14699): the questgiver arm
		// breaks, so the BoxMoney charge still lands — this case used
		// to return before it.
		s.chargeGossipBoxMoney(ctx, boxCost)
		return questOK
	}
	// Player::OnGossipSelect (Player.cpp:14699): the BoxMoney charge lands
	// after the option arms.
	s.chargeGossipBoxMoney(ctx, boxCost)
	if s.gossip == nil && !s.gossipClosed {
		// Player::OnGossipSelect arms that send no packet leave the live
		// menu open (C++ never closes the menu for them) — restore it
		// instead of closing the client's window.
		s.gossip = prevGossip
	}
	s.debug("gameobject gossip selection handled", "account", s.accountName, "entry", entry)
	return true
}

// handlePlayerGossipSelect processes CMSG_GOSSIP_SELECT_OPTION for the
// player's own GUID — the player arm of
// WorldSession::HandleGossipSelectOptionOpcode (MiscHandler.cpp:146-151).
// C++ accepts only the player's own GUID with the current menu (the shared
// sender/menu gates above already enforce both), then fires
// ScriptMgr::OnGossipSelect[Code] — Eluna's HandleGossipSelectOption(Player*,
// menuId, ...) (LuaEngine/GossipHooks.cpp:64): ClearMenus, then (player,
// player, sender, action, code-or-nil) with no cancel semantics
// (CallAllFunctions). There are no native arms on this path.
func (s *session) handlePlayerGossipSelect(ctx context.Context, menuID uint32, item gossipMenuItem, code string) bool {
	// MiscHandler.cpp:146-151: the player arm rejects when the guid isn't
	// the player's own or the menuId no longer matches, before the
	// feign-death strip and the hooks.
	if s.gossip == nil || s.gossip.MenuID != menuID {
		s.debug("player gossip selection rejected: stale menu", "account", s.accountName, "menu", menuID)
		return true
	}
	// MiscHandler.cpp:162-164 (HandleGossipSelectOptionOpcode) — the
	// feign-death strip runs after the player-arm validation, before dispatch.
	s.removeFakeDeath()
	s.gossip = nil
	codeArg := any(nil)
	if code != "" {
		codeArg = code
	}
	if s.server != nil && s.server.Features != nil && s.server.Features.Scripts != nil {
		kind := scripting.PlayerGossipKind(menuID)
		if s.server.Features.Scripts.HasHook(kind, scripting.GossipEventOnSelect) {
			if _, err := s.server.Features.Scripts.TriggerPlayerGossipEvent(ctx, menuID, scripting.GossipEventOnSelect, s.luaPlayer(), s.luaPlayer(), item.Sender, item.Action, codeArg); err != nil {
				s.debug("lua player gossip select failed", "account", s.accountName, "menu", menuID, "error", err)
			}
		}
	}
	s.gossipClosed = true
	if err := s.write(uint16(protocol.OpcodeSMSG_GOSSIP_COMPLETE), nil, true); err != nil {
		return true
	}
	return true
}

// handleItemGossipSelectOption processes CMSG_GOSSIP_SELECT_OPTION for an
// item GUID — the item arm of WorldSession::HandleGossipSelectOptionOpcode
// (MiscHandler.cpp:138-145). C++ resolves the item by GUID only
// (Player::GetItemByGuid): no interaction-range or npc-flag gates apply,
// and only the gossip sender-GUID cheat check runs (no menuId check,
// unlike the player arm). It dispatches Eluna::HandleGossipSelectOption's
// item arm (GossipHooks.cpp:90) via ScriptMgr::OnGossipSelect[Code]
// (ScriptMgr.cpp:1655-1677), which fires GOSSIP_EVENT_ON_SELECT (2) with
// (event, player, item, sender, action[, code]) and no cancel semantics
// (CallAllFunctions). Go has no C++ ItemScript system, so the native
// OnGossipSelect[Code] tail has no counterpart. The feign-death strip
// (MiscHandler.cpp:162-164) runs for every gossip select arm.
func (s *session) handleItemGossipSelectOption(ctx context.Context, reader *protocol.Buffer, guid uint64, listID uint32) bool {
	if s.gossip == nil || s.gossip.SenderGUID != guid {
		s.debug("item gossip selection rejected", "account", s.accountName, "guid", guid, "list", listID)
		return true
	}
	menuItem, ok := s.gossip.Items[listID]
	if !ok {
		return true
	}
	code := ""
	if menuItem.Coded {
		var err error
		code, err = reader.ReadCString()
		if err != nil {
			s.debug("item gossip selection rejected", "account", s.accountName, "guid", guid, "list", listID, "reason", "malformed code", "error", err)
			return true
		}
	}
	item := s.sessionLuaItem(ctx, guid)
	if item == nil {
		s.debug("item gossip selection unknown item", "account", s.accountName, "guid", guid)
		return true
	}
	// MiscHandler.cpp:162-164 (HandleGossipSelectOptionOpcode) — the
	// feign-death strip runs after the item validation, before dispatch.
	s.removeFakeDeath()
	s.fireItemGossipSelectHook(ctx, objectUint32OrZero(item, "Entry"), menuItem, code, item)
	s.debug("item gossip selection handled", "account", s.accountName, "guid", guid, "list", listID)
	return true
}

func (s *session) prepareCreatureGossip(ctx context.Context, guid uint64, entry, npcFlags, menuID uint32, isDefaultMenu, showQuests bool) (*gossipMenuState, error) {
	menu := &gossipMenuState{SenderGUID: guid, MenuID: menuID, TitleID: 0x00FFFFFF, Items: make(map[uint32]gossipMenuItem)}
	titleID, err := s.loadGossipMenuTitleID(ctx, menuID, entry, guid)
	if err != nil {
		return nil, err
	}
	menu.TitleID = titleID
	options, err := s.loadCreatureGossipOptions(ctx, menuID, npcFlags, entry, guid)
	if err != nil {
		return nil, err
	}
	// Player::PrepareGossipMenu (Player.cpp:14371): the menu-0 fallback applies
	// only when the requested menu IS the source's default menu — an explicit
	// submenu (ActionMenuID) with no rows shows empty in C++.
	if len(options) == 0 && isDefaultMenu {
		options, err = s.loadCreatureGossipOptions(ctx, 0, npcFlags, entry, guid)
		if err != nil {
			return nil, err
		}
	}
	// Player::PrepareGossipMenu (Player.cpp:14465-14468): a TAXIVENDOR option
	// that survives the npcflag/condition gates learns an unknown flight node
	// and aborts the menu build; the flight map opens through the taxi option
	// afterwards. The arm is per-option, not per-creature — a flightmaster
	// hello whose menu holds no taxivendor option still shows its menu.
	for _, option := range options {
		if option.Item.Action == 4 && s.learnNewTaxiNode(ctx, guid) {
			return nil, nil
		}
	}
	for _, option := range options {
		if option.Item.Action == 6 && !s.isDeadOrGhost() {
			continue
		}
		// Player.cpp:14425 — the UNLEARNTALENTS option is hidden when
		// !creature->CanResetTalents(this, false); see session.canResetTalents
		// (trainers.go) for the Creature::CanResetTalents bridge.
		if option.Item.Action == 16 && !s.canResetTalents(ctx, guid) {
			continue
		}
		menu.Items[option.ID] = option.Item
	}
	// Player::PrepareGossipMenu (Player.cpp:14380-14384): the quest menu is
	// prepared only when showQuests is set — gossip submenus opened from
	// OnGossipSelect (showQuests=false) carry no quest list.
	if showQuests && npcFlags&0x00000002 != 0 && s.player != nil {
		quests, err := s.loadCreatureQuestMenu(ctx, entry, s.player.Level)
		if err != nil {
			return nil, err
		}
		menu.Quests = quests
	}
	return menu, nil
}

// loadGossipMenuTitleID mirrors Player::GetGossipTextId (Player.cpp:14711-14727):
// every gossip_menu row for the menu is visited and the LAST TextID whose
// attached conditions (SourceType 14, SourceGroup=MenuID, SourceEntry=TextID)
// pass is kept. A menuId of 0 falls back to DEFAULT_GOSSIP_MESSAGE
// (0x00FFFFFF) without a lookup.
func (s *session) loadGossipMenuTitleID(ctx context.Context, menuID, creatureEntry uint32, creatureGUID uint64) (uint32, error) {
	titleID := uint32(0x00FFFFFF)
	if menuID == 0 || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return titleID, nil
	}
	rows, err := s.server.WorldStore.DB.QueryContext(ctx, "SELECT TextID FROM gossip_menu WHERE MenuID = ? ORDER BY TextID", menuID)
	if err != nil {
		if missingTable(err) {
			return titleID, nil
		}
		return titleID, err
	}
	defer rows.Close()
	for rows.Next() {
		var textID int64
		if err := rows.Scan(&textID); err != nil {
			return titleID, err
		}
		meets, err := s.meetGossipMenuConditions(ctx, menuID, uint32(textID), creatureEntry, creatureGUID)
		if err != nil {
			return titleID, err
		}
		if meets {
			titleID = uint32(textID)
		}
	}
	return titleID, rows.Err()
}

// sendGossipPOI mirrors PlayerMenu::SendPointOfInterest (GossipDef.cpp:250-274):
// SMSG_GOSSIP_POI carries the points_of_interest row for a gossip option's
// ActionPoiID (flags, X, Y, icon, importance, name).
func (s *session) sendGossipPOI(ctx context.Context, poiID uint32) {
	if s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return
	}
	var flags, icon, importance int64
	var x, y float64
	var name string
	err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT Flags, PositionX, PositionY, Icon, Importance, Name FROM points_of_interest WHERE ID = ?", poiID).Scan(&flags, &x, &y, &icon, &importance, &name)
	if err != nil {
		if !errorsIsNoRows(err) {
			s.debug("gossip poi lookup failed", "account", s.accountName, "poi", poiID, "error", err)
		}
		return
	}
	buf := protocol.NewBuffer(32)
	buf.WriteU32(uint32(flags))
	buf.WriteF32(float32(x))
	buf.WriteF32(float32(y))
	buf.WriteU32(uint32(icon))
	buf.WriteU32(uint32(importance))
	buf.WriteCString(name)
	_ = s.write(uint16(protocol.OpcodeSMSG_GOSSIP_POI), buf.Bytes(), true)
}

type loadedGossipOption struct {
	ID   uint32
	Item gossipMenuItem
}

// trainerValidForCreature mirrors the GOSSIP_OPTION_TRAINER visibility gate in
// Player::PrepareGossipMenu (Player.cpp:14449-14455): the option is hidden (and
// C++ logs a sql error) when the creature has no trainer row or the trainer is
// not valid for the player. Same lookup as sendTrainerList.
func (s *session) trainerValidForCreature(ctx context.Context, creatureEntry uint32) bool {
	if s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return false
	}
	var tType, tReq int64
	err := s.server.WorldStore.DB.QueryRowContext(ctx, `SELECT COALESCE(t.Type, 0), COALESCE(t.Requirement, 0)
		FROM trainer AS t
		WHERE t.Id IN (SELECT TrainerId FROM creature_default_trainer WHERE CreatureId = ?)
		LIMIT 1`, creatureEntry).Scan(&tType, &tReq)
	if err != nil {
		return false
	}
	return s.isTrainerValidForPlayer(uint32(tType), uint32(tReq))
}

// gossipOptionRow is one scanned gossip_menu_option row; the creature and
// gameobject loaders share the query and apply their own visibility gates.
type gossipOptionRow struct {
	id, icon, optionType, requiredFlags, actionMenuID, actionPoiID, coded, boxMoney int64
	message, boxMessage                                                             string
}

// loadGossipOptionRows runs the gossip_menu_option lookup shared by the
// creature and gameobject menu loaders (broadcast_text fallback included).
func (s *session) loadGossipOptionRows(ctx context.Context, menuID uint32) ([]gossipOptionRow, error) {
	rows, err := s.server.WorldStore.DB.QueryContext(ctx, `SELECT gmo.OptionID, gmo.OptionIcon,
		COALESCE(NULLIF(gmo.OptionText, ''), bt.Text, ''),
		gmo.OptionType, gmo.OptionNpcFlag, gmo.ActionMenuID, gmo.ActionPoiID, gmo.BoxCoded, gmo.BoxMoney,
		COALESCE(NULLIF(gmo.BoxText, ''), btb.Text, '')
		FROM gossip_menu_option AS gmo
		LEFT JOIN broadcast_text AS bt ON bt.ID = gmo.OptionBroadcastTextID
		LEFT JOIN broadcast_text AS btb ON btb.ID = gmo.BoxBroadcastTextID
		WHERE gmo.MenuID = ? ORDER BY gmo.OptionID`, menuID)
	if err != nil {
		rows, err = s.server.WorldStore.DB.QueryContext(ctx, "SELECT OptionID, OptionIcon, COALESCE(OptionText, ''), OptionType, OptionNpcFlag, ActionMenuID, ActionPoiID, BoxCoded, BoxMoney, COALESCE(BoxText, '') FROM gossip_menu_option WHERE MenuID = ? ORDER BY OptionID", menuID)
		if err != nil {
			if missingTable(err) {
				return nil, nil
			}
			return nil, err
		}
	}
	defer rows.Close()
	result := make([]gossipOptionRow, 0, 32)
	for rows.Next() {
		var row gossipOptionRow
		if err := rows.Scan(&row.id, &row.icon, &row.message, &row.optionType, &row.requiredFlags, &row.actionMenuID, &row.actionPoiID, &row.coded, &row.boxMoney, &row.boxMessage); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (s *session) loadCreatureGossipOptions(ctx context.Context, menuID, npcFlags, creatureEntry uint32, creatureGUID uint64) ([]loadedGossipOption, error) {
	optionRows, err := s.loadGossipOptionRows(ctx, menuID)
	if err != nil {
		return nil, err
	}
	options := make([]loadedGossipOption, 0, 32)
	for _, row := range optionRows {
		id, icon, optionType, requiredFlags := row.id, row.icon, row.optionType, row.requiredFlags
		actionMenuID, actionPoiID, coded, boxMoney := row.actionMenuID, row.actionPoiID, row.coded, row.boxMoney
		message, boxMessage := row.message, row.boxMessage
		// Player::PrepareGossipMenu (Player.cpp:14392): the npcflag gate
		// has no zero-flag exemption — an option with OptionNpcFlag 0 never
		// shows for a creature source.
		if uint32(requiredFlags)&npcFlags == 0 {
			continue
		}
		if optionType == 2 { // GOSSIP_OPTION_QUESTGIVER is handled via QuestMenu, not as a gossip text item
			continue
		}
		// Player::PrepareGossipMenu (Player.cpp:14404-14491) hides options
		// whose per-type conditions fail:
		if optionType == 15 { // GOSSIP_OPTION_ARMORER is "added in special mode" — never menu-listed
			continue
		}
		if optionType == 16 && !s.canResetTalents(ctx, creatureGUID) { // GOSSIP_OPTION_UNLEARNTALENTS: hidden when the creature can't reset talents (Player.cpp:14459)
			continue
		}
		if optionType == 17 { // GOSSIP_OPTION_UNLEARNPETTALENTS (Player.cpp:14462)
			// Hidden without an active pet, when the active pet isn't a
			// hunter pet (getPetType()!=HUNTER_PET), when it knows one or
			// fewer spells (m_spells.size()<=1), or when the creature isn't
			// a valid pet-talent trainer (Creature::CanResetTalents(pet),
			// Creature.cpp:1303). C++ has no learned-talent gate — the
			// option shows with zero talents and the reset is a no-op there.
			if s.activePetNumber() == 0 || !s.activePetIsHunterType(ctx) || s.activePetSpellCount(ctx) <= 1 || !s.canResetPetTalents(ctx, creatureGUID) {
				continue
			}
		}
		if optionType == 19 { // GOSSIP_OPTION_OUTDOORPVP: no OutdoorPvP manager in Go
			continue
		}
		if optionType == 12 { // GOSSIP_OPTION_BATTLEFIELD
			// Creature::isCanInteractWithBattleMaster (Creature.cpp:1271): the
			// entry must map to a battleground template and the player must
			// pass the level gate.
			if bgTypeID := s.battlemasterBGType(ctx, creatureEntry); bgTypeID == battlegroundTypeNone || !s.bgAccessByLevel(ctx, bgTypeID) {
				continue
			}
		}
		if optionType == 14 && (s.player == nil || s.player.Class != 3) { // GOSSIP_OPTION_STABLEPET: hunters only (ChrClasses.dbc 3)
			continue
		}
		if optionType == 3 && len(s.expandedVendorRows(ctx, creatureEntry)) == 0 { // GOSSIP_OPTION_VENDOR: hidden with an empty list (Player.cpp:14440)
			s.debug("gossip vendor option hidden: empty vendor list", "account", s.accountName, "entry", creatureEntry, "menu", menuID)
			continue
		}
		if optionType == 5 && !s.trainerValidForCreature(ctx, creatureEntry) { // GOSSIP_OPTION_TRAINER: hidden for invalid trainers (Player.cpp:14478)
			continue
		}
		if (optionType == 18 || optionType == 20) && // GOSSIP_OPTION_LEARNDUALSPEC / DUALSPEC_INFO (Player.cpp:14456)
			(s.player == nil || s.player.TalentGroupsCount != 1 || s.player.Level < 40 || !s.canResetTalents(ctx, creatureGUID)) {
			continue
		}
		// ConditionMgr gate (SourceType 14): seasonal/event/class/race/
		// quest-chain options stay hidden until their conditions pass.
		meets, err := s.meetGossipOptionConditions(ctx, menuID, uint32(id), creatureEntry, creatureGUID)
		if err != nil {
			return nil, err
		}
		if !meets {
			continue
		}
		if len(options) >= 32 {
			break
		}
		options = append(options, loadedGossipOption{ID: uint32(id), Item: gossipMenuItem{Icon: uint8(icon), Coded: coded != 0, BoxMoney: uint32(boxMoney), Message: message, BoxMessage: boxMessage, Action: uint32(optionType), ActionMenuID: uint32(actionMenuID), ActionPoiID: uint32(actionPoiID)}})
	}
	return options, nil
}

// loadGameObjectGossipOptions mirrors the gameobject arm of
// Player::PrepareGossipMenu (Player.cpp:14363-14403): a gameobject source is
// not a creature, so the OptionNpcFlag filter and the per-type visibility
// switch never apply — the ConditionMgr gate is the only filter.
func (s *session) loadGameObjectGossipOptions(ctx context.Context, menuID, entry uint32, guid uint64) ([]loadedGossipOption, error) {
	optionRows, err := s.loadGossipOptionRows(ctx, menuID)
	if err != nil {
		return nil, err
	}
	// Player::PrepareGossipMenu GO arm (Player.cpp:14474-14484): a gameobject
	// source lists only GOSSIP_OPTION_GOSSIP (1), and only for the
	// questgiver/goober types — every other option type is hidden.
	goType := uint8(0)
	if s.server != nil {
		goType = s.server.gameObjectTemplateType(entry)
	}
	options := make([]loadedGossipOption, 0, 32)
	for _, row := range optionRows {
		if uint32(row.optionType) != 1 || (goType != GameObjectTypeQuestGiver && goType != GameObjectTypeGoober) {
			continue
		}
		meets, err := s.meetGossipOptionConditions(ctx, menuID, uint32(row.id), entry, guid)
		if err != nil {
			return nil, err
		}
		if !meets {
			continue
		}
		if len(options) >= 32 {
			break
		}
		options = append(options, loadedGossipOption{ID: uint32(row.id), Item: gossipMenuItem{Icon: uint8(row.icon), Coded: row.coded != 0, BoxMoney: uint32(row.boxMoney), Message: row.message, BoxMessage: row.boxMessage, Action: uint32(row.optionType), ActionMenuID: uint32(row.actionMenuID), ActionPoiID: uint32(row.actionPoiID)}})
	}
	return options, nil
}

// gameObjectDefaultGossipMenuID mirrors the gameobject arm of
// Player::GetDefaultGossipMenuForSource (Player.cpp:14729) via
// GameObjectTemplate::GetGossipMenuId (GameObjectData.h:560): only the
// questgiver and goober types carry a gossip menu ID.
func (s *session) gameObjectDefaultGossipMenuID(ctx context.Context, entry uint32, goType uint8) uint32 {
	switch goType {
	case GameObjectTypeQuestGiver:
		return s.loadGameObjectTemplateData(ctx, entry)[3] // questgiver.gossipID (GameObjectData.h:75)
	case GameObjectTypeGoober:
		return s.loadGooberTemplate(ctx, entry).gossipID
	default:
		return 0
	}
}

// prepareGameObjectGossip mirrors Player::PrepareGossipMenu
// (Player.cpp:14363) for a gameobject source: the gossip menu for menuID,
// the menu-0 fallback when menuID is the gameobject's default menu, and the
// quest menu from the gameobject quest relations when showQuests is set.
func (s *session) prepareGameObjectGossip(ctx context.Context, guid uint64, entry, menuID, defaultMenuID uint32, showQuests bool) (*gossipMenuState, error) {
	menu := &gossipMenuState{SenderGUID: guid, MenuID: menuID, TitleID: 0x00FFFFFF, Items: make(map[uint32]gossipMenuItem)}
	titleID, err := s.loadGossipMenuTitleID(ctx, menuID, entry, guid)
	if err != nil {
		return nil, err
	}
	menu.TitleID = titleID
	options, err := s.loadGameObjectGossipOptions(ctx, menuID, entry, guid)
	if err != nil {
		return nil, err
	}
	// Player::PrepareGossipMenu (Player.cpp:14373): the menu-0 fallback
	// applies only when the requested menu is the source's default menu.
	if len(options) == 0 && menuID == defaultMenuID {
		options, err = s.loadGameObjectGossipOptions(ctx, 0, entry, guid)
		if err != nil {
			return nil, err
		}
	}
	for _, option := range options {
		menu.Items[option.ID] = option.Item
	}
	if showQuests && s.player != nil {
		quests, err := s.loadGameObjectQuestMenu(ctx, entry, s.player.Level)
		if err != nil {
			return nil, err
		}
		menu.Quests = quests
	}
	return menu, nil
}

// buildQuestGiverQuestList mirrors PlayerMenu::SendQuestGiverQuestList
// (GossipDef.cpp:319-370): guid, the quest_greeting row keyed by entry and the
// source's type (or an empty title with zero emotes), then the quest rows
// with the 3.3.3 autocomplete byte.
func (s *session) buildQuestGiverQuestList(ctx context.Context, guid uint64, entry uint32, greetType uint32, quests []gossipQuestItem) []byte {
	title := ""
	var emoteDelay, emoteType uint32
	greetingFound := false
	if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		var greetEmoteType, greetEmoteDelay int64
		var greeting string
		// LoadQuestGreetings (ObjectMgr.cpp:6447): ID, Type, GreetEmoteType, GreetEmoteDelay, Greeting.
		if err := s.server.WorldStore.DB.QueryRowContext(ctx,
			`SELECT GreetEmoteType, GreetEmoteDelay, Greeting FROM quest_greeting WHERE ID = ? AND Type = ? LIMIT 1`,
			entry, greetType).Scan(&greetEmoteType, &greetEmoteDelay, &greeting); err == nil {
			title = greeting
			// GossipDef.cpp:340-341: the packet carries delay then type.
			emoteDelay, emoteType = uint32(greetEmoteDelay), uint32(greetEmoteType)
			greetingFound = true
		}
	}
	// Player::SendPreparedQuest (Player.cpp:14855-14880): with no
	// quest_greeting row, a creature source falls back to its NPC gossip
	// text title (Text_0, else Text_1) and the first emote from
	// GetGossipTextId(creature); a gameobject source keeps the empty title
	// and zero emotes. npc_text locale overrides are unmodeled with the
	// rest of the no-locale divergence.
	if !greetingFound && greetType == typeIDUnit && s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		var gossipMenuID int64
		if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT gossip_menu_id FROM creature_template WHERE entry = ?", entry).Scan(&gossipMenuID); err == nil && gossipMenuID != 0 {
			if textID, terr := s.loadGossipMenuTitleID(ctx, uint32(gossipMenuID), entry, guid); terr == nil {
				if options, oerr := s.loadNPCText(ctx, textID); oerr == nil && len(options) > 0 {
					emoteDelay, emoteType = options[0].Emotes[0].Delay, options[0].Emotes[0].ID
					title = options[0].Text0
					if title == "" {
						title = options[0].Text1
					}
				}
			}
		}
	}
	entries := protocol.NewBuffer(64)
	var count uint8
	for _, q := range quests {
		entries.WriteU32(q.ID)
		entries.WriteU32(q.Icon)
		entries.WriteI32(q.Level)
		entries.WriteU32(q.Flags)
		if q.AutoComplete {
			entries.WriteU8(1)
		} else {
			entries.WriteU8(0)
		}
		entries.WriteCString(q.Title)
		count++
	}
	packet := protocol.NewBuffer(128)
	packet.WriteU64(guid)
	packet.WriteCString(title)
	packet.WriteU32(emoteDelay)
	packet.WriteU32(emoteType)
	packet.WriteU8(count)
	packet.Write(entries.Bytes())
	return packet.Bytes()
}

// fastOpenQuestGiverQuest mirrors the single-quest arm of
// Player::SendPreparedQuest (Player.cpp:14812-14843). A questIcon==4 quest
// takes the turn-in arm (Player.cpp:14820), routed through the existing
// complete-quest packet path, which models the RequestItems open
// (QuestHandler.cpp:520-565) behind its interact gates. Otherwise the
// giver object must start or involve the quest (Player.cpp:14826-14831)
// or the menu closes; auto-accept quests are added on view behind
// CanAddQuest(msg=true) and CanTakeQuest(msg=true) (Player.cpp:14833-14834);
// autocomplete (+repeatable, non-daily/weekly) or AUTOCOMPLETE-flagged
// quests open the reward window (Player.cpp:14836-14838); the rest open
// the quest details unconditionally (Player.cpp:14840).
func (s *session) fastOpenQuestGiverQuest(ctx context.Context, guid uint64, q gossipQuestItem) bool {
	questID := q.ID
	if q.Icon == 4 {
		// Player::SendPreparedQuest (Player.cpp:14818-14820): icon 4 routes to
		// the request-items window unconditionally — this covers
		// autocomplete-repeatable quests the player has not taken yet, which
		// the old status gate sent to the details window instead.
		queryPayload := protocol.NewBuffer(12)
		queryPayload.WriteU64(guid)
		queryPayload.WriteU32(questID)
		return s.handleQuestgiverCompleteQuest(ctx, queryPayload.Bytes())
	}
	view, err := s.loadQuestRewardView(ctx, questID)
	if err != nil {
		s.debug("quest fast-open failed", "account", s.accountName, "quest", questID, "error", err)
		return false
	}
	if !s.questGiverObjectHasQuest(ctx, guid, questID) {
		return s.sendGossipComplete()
	}
	// Quest::IsAutoAccept folds the quest_template_addon SpecialFlags leg
	// into quest_template.Flags at load (QuestDef.cpp:193), so Go ORs both
	// legs like the query opcode does.
	flags := view.Detail.Flags
	var specialFlags int64
	if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(SpecialFlags, 0) FROM quest_template_addon WHERE ID = ?", questID).Scan(&specialFlags)
	}
	if flags&questFlagsAutoAccept != 0 || specialFlags&4 != 0 {
		if canTake, terr := s.canTakeQuest(ctx, questID); terr == nil && canTake && s.canAddQuest(ctx, questID, true) {
			s.addQuestToPlayer(ctx, questID, guid)
			s.fireCreatureQuestHook(ctx, guid, scripting.CreatureEventOnQuestAccept, s.luaQuest(ctx, questID))
			s.fireGameObjectQuestHook(ctx, guid, scripting.GameObjectEventOnQuestAccept, s.luaQuest(ctx, questID))
			s.fireItemQuestHook(ctx, guid, questID)
		}
	}
	if (s.questIsAutoComplete(ctx, questID, flags) && s.questIsRepeatable(ctx, questID) && flags&(questFlagsDaily|questFlagsWeekly) == 0) || flags&questAutoCompleteFlags != 0 {
		return s.sendQuestRequestItems(view, guid, s.canCompleteRepeatableQuest(ctx, questID, view, true), true)
	}
	return s.sendQuestGiverQuestDetails(ctx, guid, view.Detail)
}

// sendPreparedCreatureQuest mirrors Player::SendPreparedQuest
// (Player.cpp:14808) for a creature source: an empty quest menu sends
// nothing, a single quest fast-opens, and multiple quests go out in
// SMSG_QUESTGIVER_QUEST_LIST keyed by the creature entry's quest greeting
// (TYPEID_UNIT).
func (s *session) sendPreparedCreatureQuest(ctx context.Context, guid uint64, entry uint32, quests []gossipQuestItem) bool {
	switch len(quests) {
	case 0:
		return true
	case 1:
		return s.fastOpenQuestGiverQuest(ctx, guid, quests[0])
	default:
		return s.write(uint16(protocol.OpcodeSMSG_QUESTGIVER_QUEST_LIST), s.buildQuestGiverQuestList(ctx, guid, entry, typeIDUnit, quests), true) == nil
	}
}

// sendPreparedGameObjectQuest mirrors Player::SendPreparedQuest
// (Player.cpp:14808) for a gameobject source: an empty quest menu sends
// nothing, a single quest fast-opens, and multiple quests go out in
// SMSG_QUESTGIVER_QUEST_LIST.
func (s *session) sendPreparedGameObjectQuest(ctx context.Context, guid uint64, entry uint32, quests []gossipQuestItem) bool {
	switch len(quests) {
	case 0:
		return true
	case 1:
		return s.fastOpenQuestGiverQuest(ctx, guid, quests[0])
	default:
		return s.write(uint16(protocol.OpcodeSMSG_QUESTGIVER_QUEST_LIST), s.buildQuestGiverQuestList(ctx, guid, entry, typeIDGameObject, quests), true) == nil
	}
}

func (s *session) luaGossipComplete(_ context.Context, _ []any) ([]any, error) {
	s.gossip = nil
	s.gossipClosed = true
	return nil, s.write(uint16(protocol.OpcodeSMSG_GOSSIP_COMPLETE), nil, true)
}

func (s *session) luaGossipClearMenu(_ context.Context, _ []any) ([]any, error) {
	if s.gossip != nil {
		s.gossip.Items = make(map[uint32]gossipMenuItem)
		s.gossip.Quests = nil
	}
	return nil, nil
}

func (s *session) luaGossipMenuAddItem(_ context.Context, args []any) ([]any, error) {
	if len(args) < 4 {
		return nil, fmt.Errorf("GossipMenuAddItem requires icon, message, sender, and action")
	}
	icon, err := luaUint32Arg(args, 0)
	if err != nil {
		return nil, err
	}
	message, ok := args[1].(string)
	if !ok {
		return nil, fmt.Errorf("gossip message must be a string")
	}
	sender, err := luaUint32Arg(args, 2)
	if err != nil {
		return nil, err
	}
	action, err := luaUint32Arg(args, 3)
	if err != nil {
		return nil, err
	}
	coded := false
	if len(args) > 4 && args[4] != nil {
		var ok bool
		coded, ok = args[4].(bool)
		if !ok {
			return nil, fmt.Errorf("gossip coded flag must be a boolean")
		}
	}
	boxMessage := ""
	if len(args) > 5 && args[5] != nil {
		var ok bool
		boxMessage, ok = args[5].(string)
		if !ok {
			return nil, fmt.Errorf("gossip box message must be a string")
		}
	}
	boxMoney := uint32(0)
	if len(args) > 6 && args[6] != nil {
		boxMoney, err = luaUint32Arg(args, 6)
		if err != nil {
			return nil, err
		}
	}
	if s.gossip == nil {
		s.gossip = &gossipMenuState{Items: make(map[uint32]gossipMenuItem)}
	}
	if len(s.gossip.Items) >= 32 {
		return nil, fmt.Errorf("gossip menu item limit reached")
	}
	itemID := uint32(0)
	for {
		if _, exists := s.gossip.Items[itemID]; !exists {
			break
		}
		itemID++
	}
	s.gossip.Items[itemID] = gossipMenuItem{Icon: uint8(icon), Coded: coded, BoxMoney: boxMoney, Message: message, BoxMessage: boxMessage, Sender: sender, Action: action}
	return nil, nil
}

func (s *session) luaGossipSendMenu(_ context.Context, args []any) ([]any, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("GossipSendMenu requires a title and source object")
	}
	title, err := luaUint32Arg(args, 0)
	if err != nil {
		return nil, err
	}
	object, ok := args[1].(*scripting.Object)
	if !ok {
		return nil, fmt.Errorf("gossip source must be an object")
	}
	guid, ok := objectUint64Field(object, "GUID")
	if !ok {
		return nil, fmt.Errorf("gossip source has no guid")
	}
	menuID := uint32(0)
	if len(args) > 2 && args[2] != nil {
		menuID, err = luaUint32Arg(args, 2)
		if err != nil {
			return nil, err
		}
	}
	if s.gossip == nil {
		s.gossip = &gossipMenuState{Items: make(map[uint32]gossipMenuItem)}
	}
	s.gossip.SenderGUID, s.gossip.MenuID, s.gossip.TitleID = guid, menuID, title
	s.gossipClosed = false
	return nil, s.sendGossipMenu()
}

func (s *session) sendGossipMenu() error {
	if s.gossip == nil {
		return nil
	}
	s.debug("gossip menu response", "account", s.accountName, "guid", s.gossip.SenderGUID, "menu", s.gossip.MenuID, "title", s.gossip.TitleID, "options", len(s.gossip.Items), "quests", len(s.gossip.Quests))
	return s.write(uint16(protocol.OpcodeSMSG_GOSSIP_MESSAGE), buildGossipMessage(*s.gossip), true)
}

func (s *session) canInteractWithCreature(ctx context.Context, creature *scripting.Object) bool {
	if s == nil || s.player == nil || creature == nil {
		return false
	}
	// Player::GetNPCIfCanInteractWith (Player.cpp:2323): no NPC interaction
	// while the player is in flight — the same gate canInteractWithNPC
	// carries; gossip hello/select both resolve through it in C++
	// (HandleGossipHelloOpcode, HandleGossipSelectOptionOpcode).
	if s.isInFlight() {
		return false
	}
	// Player::GetNPCIfCanInteractWith (Player.cpp:2343-2345): a creature under
	// control (UNIT_FIELD_CHARMEDBY set) cannot be interacted with. C++ sets
	// CHARMEDBY only in Unit::SetCharm (Unit.cpp:6229), so player pets — owned,
	// never charmed — keep passing this gate; Go's charm twin lives on the
	// motion record (charmCreature, creaturemotion.go:1752-1766).
	//
	// Player::GetNPCIfCanInteractWith (Player.cpp:2352-2354): an unfriendly or
	// worse creature (GetReactionTo <= REP_UNFRIENDLY) rejects the interaction;
	// creatureToPlayerRank ports the creature->player GetReactionTo tail
	// (Object.cpp:2795-2842) including the at-war neutral clamp.
	if guid, ok := objectUint64Field(creature, "GUID"); ok {
		if motion := s.findCreatureMotion(guid); motion != nil {
			if motion.Charmed {
				return false
			}
			player := playerPos{Race: s.player.Race, Class: s.player.Class, Reputations: playerReputationMap(s.player.Reputations), Sess: s}
			if s.server != nil {
				player.FactionTemplate = s.effectiveFactionTemplate(s.player.Race)
			}
			if s.creatureToPlayerRank(motion.Faction, player) <= 2 {
				return false
			}
		}
	}
	mapID, mapOK := objectUint32Field(creature, "Map")
	x, xOK := objectFloat32Field(creature, "X")
	y, yOK := objectFloat32Field(creature, "Y")
	z, zOK := objectFloat32Field(creature, "Z")
	if mapOK && mapID != s.player.Map {
		return false
	}
	if xOK && yOK && zOK && distance3D(s.player.X, s.player.Y, s.player.Z, x, y, z) > 5.0 {
		return false
	}
	// Player::GetNPCIfCanInteractWith (Player.cpp:2335-2337): a dead creature
	// cannot be interacted with unless the template allows it
	// (CREATURE_TYPE_FLAG_CAN_INTERACT_WHILE_DEAD, SharedDefines.h:2736).
	if health, healthOK := objectUint32Field(creature, "Health"); healthOK && health == 0 {
		entry, ok := objectUint32Field(creature, "Entry")
		if !ok {
			return false
		}
		if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
			var typeFlags int64
			if err := s.server.WorldStore.DB.QueryRowContext(ctx, `SELECT COALESCE(type_flags, 0) FROM creature_template WHERE entry = ?`, entry).Scan(&typeFlags); err == nil {
				if uint32(typeFlags)&creatureTypeFlagInteractDead == 0 {
					return false
				}
			}
		}
	}
	// Player::GetNPCIfCanInteractWith (Player.cpp:2332): a dead player
	// cannot interact unless the template is ghost-visible
	// (CREATURE_TYPE_FLAG_GHOST_VISIBLE, SharedDefines.h:2730).
	if s.player.Health == 0 || s.player.PlayerFlags&playerFlagGhost != 0 {
		entry, ok := objectUint32Field(creature, "Entry")
		if !ok {
			return false
		}
		if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
			var typeFlags int64
			if err := s.server.WorldStore.DB.QueryRowContext(ctx, `SELECT COALESCE(type_flags, 0) FROM creature_template WHERE entry = ?`, entry).Scan(&typeFlags); err == nil {
				if uint32(typeFlags)&creatureTypeFlagGhostVisible == 0 {
					return false
				}
			}
		}
	}
	return true
}

func buildGossipMessage(menu gossipMenuState) []byte {
	packet := protocol.NewBuffer(128)
	packet.WriteU64(menu.SenderGUID)
	packet.WriteU32(menu.MenuID)
	packet.WriteU32(menu.TitleID)
	ids := make([]uint32, 0, len(menu.Items))
	for id := range menu.Items {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	packet.WriteU32(uint32(len(ids)))
	for _, id := range ids {
		item := menu.Items[id]
		packet.WriteU32(id)
		packet.WriteU8(item.Icon)
		if item.Coded {
			packet.WriteU8(1)
		} else {
			packet.WriteU8(0)
		}
		packet.WriteU32(item.BoxMoney)
		packet.WriteCString(item.Message)
		packet.WriteCString(item.BoxMessage)
	}
	packet.WriteU32(uint32(len(menu.Quests)))
	for _, quest := range menu.Quests {
		packet.WriteU32(quest.ID)
		packet.WriteU32(quest.Icon)
		packet.WriteI32(quest.Level)
		packet.WriteU32(quest.Flags)
		if quest.AutoComplete {
			packet.WriteU8(1)
		} else {
			packet.WriteU8(0)
		}
		packet.WriteCString(quest.Title)
	}
	return packet.Bytes()
}

func objectUint32Field(object *scripting.Object, name string) (uint32, bool) {
	value, ok := object.Fields[name]
	if !ok {
		return 0, false
	}
	switch value := value.(type) {
	case uint8:
		return uint32(value), true
	case uint16:
		return uint32(value), true
	case uint32:
		return value, true
	case uint64:
		return uint32(value), true
	case int:
		return uint32(value), true
	case int64:
		return uint32(value), true
	case float64:
		return uint32(value), true
	default:
		return 0, false
	}
}

func objectUint32OrZero(object *scripting.Object, name string) uint32 {
	value, _ := objectUint32Field(object, name)
	return value
}

func objectUint64Field(object *scripting.Object, name string) (uint64, bool) {
	value, ok := object.Fields[name]
	if !ok {
		return 0, false
	}
	switch value := value.(type) {
	case uint64:
		return value, true
	case uint32:
		return uint64(value), true
	case int64:
		return uint64(value), true
	case float64:
		return uint64(value), true
	default:
		return 0, false
	}
}

// handleBinderActivate processes CMSG_BINDER_ACTIVATE (0x1B5).
// Reference: WorldSession::HandleBinderActivateOpcode (NPCHandler.cpp:247-286),
// Spell::EffectBind (SpellEffects.cpp:5710-5725), Player::SendBindPointUpdate (Player.cpp:17359-17366).
func (s *session) handleBinderActivate(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	r := protocol.NewReader(payload)
	npcGUID, err := r.ReadU64()
	if err != nil {
		// Truncated packet: WorldSession::Update catches ByteBufferException and
		// skips the packet (WorldSession.cpp:425-432) — drop, conn open.
		return true
	}
	// NPCHandler.cpp:247-256 (HandleBinderActivateOpcode): dead or
	// not-in-world players are silently dropped before the interact check.
	// Go has no separate in-world flag; playerLoaded sessions are in-world.
	if s.isDeadOrGhost() {
		return true
	}
	if !s.canInteractWithNPC(ctx, npcGUID, uint64(unitNPCFlagInnkeeper)) {
		return true
	}
	// NPCHandler.cpp:262-264 (HandleBinderActivateOpcode) — setting the
	// homebind strips a faked death.
	s.removeFakeDeath()
	// NPCHandler.cpp:265-268 (SendBindPoint): homebind can never be set
	// inside an instance (C++ Map::Instanceable, i.e. DBC Map InstanceType
	// != 0); the handler returns before the bind spell cast.
	if s.server != nil && s.server.Data != nil {
		if mapEntry, found, err := s.server.Data.Map(s.player.Map); err == nil && found && mapEntry.InstanceType != 0 {
			return true
		}
	}

	// Update player homebind location
	s.player.HomebindMap = s.player.Map
	// SpellEffects.cpp:5704-5710 (Spell::EffectBind): SetHomebind takes
	// player->GetAreaId() — the subzone area id, not the root zone.
	s.player.HomebindZone = s.areaID
	s.player.HomebindX = s.player.X
	s.player.HomebindY = s.player.Y
	s.player.HomebindZ = s.player.Z

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE characters SET homebind_map = ?, homebind_zone = ?, homebind_x = ?, homebind_y = ?, homebind_z = ? WHERE guid = ?",
			s.player.HomebindMap, s.player.HomebindZone, s.player.HomebindX, s.player.HomebindY, s.player.HomebindZ, s.playerGUID)
	}

	// 1. Send visual cast of homebind spell 3286 from NPC to player (TC: npc->CastSpell(_player, 3286, true))
	castTimeStamp := uint32(time.Now().UnixMilli())
	target := protocol.SpellTargetData{Flags: protocol.SpellTargetFlagUnit, UnitGUID: s.playerGUID}
	if s.server != nil && s.server.Data != nil {
		if spellInfo, found, err := s.server.Data.Spell(3286); err == nil && found {
			target = spellGoPacketTarget(spellInfo, target)
		}
	}
	goPkt := protocol.BuildSpellGo(npcGUID, s.playerGUID, 1, 3286, spellCastFlagGo, castTimeStamp, []uint64{s.playerGUID}, nil, target)
	_ = s.write(uint16(protocol.OpcodeSMSG_SPELL_GO), goPkt, true)
	if s.server != nil {
		s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELL_GO), goPkt, s)
	}

	// 2. SMSG_BIND_POINT_UPDATE (0x155, 20 bytes: X, Y, Z, Map, Area)
	_ = s.write(uint16(protocol.OpcodeSMSG_BIND_POINT_UPDATE), buildBindPointUpdate(s.player), true)

	// 3. SMSG_PLAYER_BOUND (0x158, 12 bytes: BinderGUID, AreaID) -> triggers client yellow message "You are now bound to..."
	_ = s.write(uint16(protocol.OpcodeSMSG_PLAYER_BOUND), buildPlayerBound(npcGUID, s.player.HomebindZone), true)

	// 4. Send trainer buy succeeded with homebind spell 3286
	res := protocol.NewBuffer(12)
	res.WriteU64(npcGUID)
	res.WriteU32(3286)
	_ = s.write(uint16(protocol.OpcodeSMSG_TRAINER_BUY_SUCCEEDED), res.Bytes(), true)
	s.sendGossipComplete()
	s.sendPlayerUpdate()
	s.debug("homebind set", "account", s.accountName, "npc", npcGUID, "map", s.player.HomebindMap, "zone", s.player.HomebindZone)
	return true
}

// buildBindPointUpdate constructs SMSG_BIND_POINT_UPDATE (0x155, 20 bytes).
// Reference: WorldPackets::Misc::BindPointUpdate (MiscPackets.cpp:20-27).
func buildBindPointUpdate(state *playerState) []byte {
	buf := protocol.NewBuffer(20)
	if state != nil {
		buf.WriteF32(state.HomebindX)
		buf.WriteF32(state.HomebindY)
		buf.WriteF32(state.HomebindZ)
		buf.WriteU32(state.HomebindMap)
		buf.WriteU32(state.HomebindZone)
	} else {
		buf.WriteF32(0)
		buf.WriteF32(0)
		buf.WriteF32(0)
		buf.WriteU32(0)
		buf.WriteU32(0)
	}
	return buf.Bytes()
}

// buildPlayerBound constructs SMSG_PLAYER_BOUND (0x158, 12 bytes).
// Reference: WorldPackets::Misc::PlayerBound (MiscPackets.cpp:29-35).
func buildPlayerBound(binderGUID uint64, areaID uint32) []byte {
	buf := protocol.NewBuffer(12)
	buf.WriteU64(binderGUID)
	buf.WriteU32(areaID)
	return buf.Bytes()
}
