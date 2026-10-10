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
	if !s.canInteractWithCreature(creature) {
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
	// WorldSession::HandleGossipHelloOpcode (NPCHandler.cpp:164):
	// GetPlayer()->RemoveAurasWithInterruptFlags(AURA_INTERRUPT_FLAG_TALK)
	// runs right after the gossip-flag gate, before the spirit-guide arm.
	// (The faction SetVisible call above it has no Go reputation-visible
	// setter — documented open gap.)
	s.removeAurasWithInterruptFlags(auraInterruptFlagTalk)
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
	entry, ok := objectUint32Field(creature, "Entry")
	if !ok {
		return true
	}
	s.gossip = nil
	s.gossipClosed = false
	if s.server.Features != nil && s.server.Features.Scripts != nil {
		if _, err := s.server.Features.Scripts.Trigger(ctx, "creature_gossip:"+strconv.FormatUint(uint64(entry), 10), 1, uint32(1), s.luaPlayer(), creature); err != nil {
			s.debug("gossip hello hook failed", "account", s.accountName, "entry", entry, "error", err)
		}
	}
	if s.gossip == nil && !s.gossipClosed {
		defaultMenu, err := s.prepareCreatureGossip(ctx, guid, entry, npcFlags, objectUint32OrZero(creature, "GossipMenuID"), true)
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
		// Reference Player::SendPreparedQuest (single-quest fast-open):
		// If the NPC has no other gossip options and exactly 1 quest, auto-open it directly.
		if defaultMenu != nil && len(defaultMenu.Items) == 0 && len(defaultMenu.Quests) == 1 {
			q := defaultMenu.Quests[0]
			queryPayload := protocol.NewBuffer(12)
			queryPayload.WriteU64(guid)
			queryPayload.WriteU32(q.ID)
			status, _ := s.characterQuestStatus(ctx, q.ID)
			if status == questStatusComplete || status == questStatusIncomplete {
				return s.handleQuestgiverCompleteQuest(ctx, queryPayload.Bytes())
			}
			return s.handleQuestgiverQueryQuest(ctx, queryPayload.Bytes())
		}
		s.gossip = defaultMenu
		if err := s.sendGossipMenu(); err != nil {
			s.debug("gossip hello response failed", "account", s.accountName, "entry", entry, "error", err)
			return false
		}
	}
	s.debug("npc hello handled", "account", s.accountName, "entry", entry, "questPath", questPath)
	return true
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
	if s.gossip == nil || s.gossip.SenderGUID != guid || s.gossip.MenuID != menuID {
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
		return s.handleGameObjectGossipSelect(ctx, guid, item, code)
	}
	if guid == s.playerGUID {
		return s.handlePlayerGossipSelect(ctx, menuID, item, code)
	}
	creature := s.luaCreature(ctx, guid)
	if creature == nil {
		return true
	}
	if !s.canInteractWithCreature(creature) {
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
	entry, ok := objectUint32Field(creature, "Entry")
	if !ok {
		return true
	}
	s.gossip = nil
	s.gossipClosed = false
	args := []any{uint32(2), s.luaPlayer(), creature, item.Sender, item.Action}
	// HandleGossipSelectOptionOpcode (MiscHandler.cpp:175) dispatches on
	// !code.empty() — Eluna::OnGossipSelectCode (5 args) vs OnGossipSelect
	// (4 args) — not on the coded flag: a coded option answered with an
	// empty code string takes the non-code path.
	if code != "" {
		args = append(args, code)
	}
	if s.server.Features != nil && s.server.Features.Scripts != nil {
		if _, err := s.server.Features.Scripts.Trigger(ctx, "creature_gossip:"+strconv.FormatUint(uint64(entry), 10), 2, args...); err != nil {
			s.debug("gossip selection hook failed", "account", s.accountName, "entry", entry, "error", err)
		}
	}
	if s.gossip == nil && !s.gossipClosed {
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
			s.sendVendorList(ctx, guid)
			s.gossipClosed = true
		} else if item.Action == 6 { // GOSSIP_OPTION_SPIRITHEALER
			// Player::OnGossipSelect (Player.cpp:14621-14624): the spirit
			// healer casts 17251 (triggered, original caster = the player) on
			// itself; the spell's resurrect effect registers a resurrect
			// request (Spell::EffectResurrect, SpellEffects.cpp:4270-4293).
			// No confirm dialog: SMSG_SPIRIT_HEALER_CONFIRM is STATUS_NEVER in
			// Opcodes.cpp:677 — the confirm/activate flow nothing sends — and
			// no gossip close, so gossipClosed only suppresses the tail's
			// SMSG_GOSSIP_COMPLETE to stay wire-faithful. The cast visual
			// (SMSG_SPELL_START/GO from the creature) has no Go bridge; the
			// mechanic is what lands here.
			if s.isDeadOrGhost() && s.resurrection == nil && s.player != nil && npcFlags&npcFlagSpiritHealer != 0 {
				pct := uint32(50)
				ignoreReclaim := false
				if s.server != nil && s.server.Data != nil {
					if sp, found, derr := s.server.Data.Spell(17251); derr == nil && found && len(sp.Effects) > 0 {
						if v := sp.Effects[0].BasePoints + 1; v >= 1 && v <= 100 {
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
				// flushed by SendLogExecute: caster is the creature here, not
				// the player sendResurrectLog assumes.
				log := protocol.NewBuffer(32)
				log.WritePackedGUID(guid)
				log.WriteU32(17251)
				log.WriteU32(1)
				log.WriteU32(spellEffectResurrect)
				log.WriteU32(1)
				log.WritePackedGUID(s.playerGUID)
				_ = s.write(uint16(protocol.OpcodeSMSG_SPELLLOGEXECUTE), log.Bytes(), true)
				// SetResurrectRequestData WorldRelocates the caster: the
				// stored location is the spirit healer's position (the accept
				// path teleports the ghost there before resurrecting).
				s.setResurrectRequestData(guid, objectUint32OrZero(creature, "Map"), x, y, z, health, mana, 0)
				// Spell::SendResurrectRequest (Spell.cpp:4693-4709): creature
				// caster name, spirit-healer sickness flag set, reclaim-timer
				// override from the spell's attributes.
				s.sendResurrectRequest(guid, name, true, !ignoreReclaim)
				s.gossipClosed = true
			}
		} else if item.Action == 4 { // GOSSIP_OPTION_TAXIVENDOR
			s.sendTaxiMenu(ctx, guid)
			s.gossipClosed = true
		} else if item.Action == 5 { // GOSSIP_OPTION_TRAINER
			s.sendTrainerList(ctx, guid)
			s.gossipClosed = true
		} else if item.Action == 8 { // GOSSIP_OPTION_INNKEEPER
			// Player::OnGossipSelect (Player.cpp:14663-14666): SendCloseGossip
			// runs before Player::SetBindPoint, which itself only sends
			// SMSG_BINDER_CONFIRM (Player.cpp:9597-9600) — the bind lands on
			// CMSG_BINDER_ACTIVATE.
			if !s.sendGossipComplete() {
				return false
			}
			bind := protocol.NewBuffer(8)
			bind.WriteU64(guid)
			if err := s.write(uint16(protocol.OpcodeSMSG_BINDER_CONFIRM), bind.Bytes(), true); err != nil {
				return false
			}
		} else if item.Action == 9 { // GOSSIP_OPTION_BANKER
			s.gossipClosed = true
			bank := protocol.NewBuffer(8)
			bank.WriteU64(guid)
			if err := s.write(uint16(protocol.OpcodeSMSG_SHOW_BANK), bank.Bytes(), true); err != nil {
				return false
			}
		} else if item.Action == 11 { // GOSSIP_OPTION_TABARDDESIGNER
			// Player::OnGossipSelect (Player.cpp:14669-14671): SendCloseGossip
			// before MSG_TABARDVENDOR_ACTIVATE.
			if !s.sendGossipComplete() {
				return false
			}
			tabard := protocol.NewBuffer(8)
			tabard.WriteU64(guid)
			if err := s.write(uint16(protocol.OpcodeMSG_TABARDVENDOR_ACTIVATE), tabard.Bytes(), true); err != nil {
				return false
			}
		} else if item.Action == 13 { // GOSSIP_OPTION_AUCTIONEER
			// Player::OnGossipSelect (Player.cpp:14672-14674) routes through
			// WorldSession::SendAuctionHello (AuctionHouseHandler.cpp:59-75):
			// the packet is guid + u32 house id + u8 enabled behind the
			// auction level gate — not the bare 9-byte write this arm had.
			s.gossipClosed = true
			s.sendAuctionHelloPacket(guid)
		} else if item.Action == 14 { // GOSSIP_OPTION_STABLEPET
			s.gossipClosed = true
			stableBuf := protocol.NewBuffer(8)
			stableBuf.WriteU64(guid)
			return s.handleListStabledPets(ctx, stableBuf.Bytes())
		} else if item.Action == 16 { // GOSSIP_OPTION_UNLEARNTALENTS
			// Player::OnGossipSelect (Player.cpp:14649-14651): SendCloseGossip
			// (SMSG_GOSSIP_COMPLETE) goes out BEFORE SendTalentWipeConfirm —
			// the menu must close before the wipe-confirm dialog opens.
			if !s.sendGossipComplete() {
				return false
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
				return false
			}
		} else if item.Action == 1 || item.Action == 20 { // GOSSIP_OPTION_GOSSIP / GOSSIP_OPTION_DUALSPEC_INFO
			// Player::OnGossipSelect (Player.cpp:14604-14615): the POI fires
			// before the submenu, for both option types.
			if item.ActionPoiID != 0 {
				s.sendGossipPOI(ctx, item.ActionPoiID)
			}
			if item.ActionMenuID != 0 {
				defaultMenu, loadErr := s.prepareCreatureGossip(ctx, guid, entry, objectUint32OrZero(creature, "NPCFlags"), item.ActionMenuID, false)
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
			// PrepareQuestMenu(guid) + SendPreparedQuest(guid) — the quest-menu
			// open. The single-quest fast-open mirrors the hello path's
			// existing SendPreparedQuest approximation; multiple quests go out
			// in the gossip message's quest section, same as the questgiver
			// hello.
			if s.player != nil {
				quests, qerr := s.loadCreatureQuestMenu(ctx, entry, s.player.Level)
				if qerr != nil {
					s.debug("gossip questgiver menu load failed", "account", s.accountName, "entry", entry, "error", qerr)
					s.gossipClosed = true
					_ = s.write(uint16(protocol.OpcodeSMSG_GOSSIP_COMPLETE), nil, true)
					return true
				}
				if len(quests) == 1 {
					q := quests[0]
					queryPayload := protocol.NewBuffer(12)
					queryPayload.WriteU64(guid)
					queryPayload.WriteU32(q.ID)
					status, _ := s.characterQuestStatus(ctx, q.ID)
					if status == questStatusComplete || status == questStatusIncomplete {
						return s.handleQuestgiverCompleteQuest(ctx, queryPayload.Bytes())
					}
					return s.handleQuestgiverQueryQuest(ctx, queryPayload.Bytes())
				}
				s.gossip = &gossipMenuState{SenderGUID: guid, MenuID: menuID, TitleID: 0x00FFFFFF, Items: make(map[uint32]gossipMenuItem), Quests: quests}
				if sendErr := s.sendGossipMenu(); sendErr != nil {
					s.debug("gossip questgiver menu response failed", "account", s.accountName, "entry", entry, "error", sendErr)
					return true
				}
			}
		} else if item.Action == 7 { // GOSSIP_OPTION_SPIRITGUIDE
			// Player::OnGossipSelect (Player.cpp:14677-14680): re-prepare the
			// creature's default gossip menu without the quest list
			// (PrepareGossipMenu(source) passes showQuests=false).
			defaultMenu, loadErr := s.prepareCreatureGossip(ctx, guid, entry, npcFlags, 0, false)
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
				return false
			}
		} else if item.Action == 12 { // GOSSIP_OPTION_BATTLEFIELD
			// Player::OnGossipSelect (Player.cpp:14684-14696): the battleground
			// type is derived from the creature entry via
			// BattlegroundMgr::GetBattleMasterBG; an unknown entry logs and
			// returns before the BoxMoney charge below.
			bgTypeID := s.battlemasterBGType(ctx, entry)
			if bgTypeID == battlegroundTypeNone {
				s.debug("gossip battlefield invalid creature", "account", s.accountName, "entry", entry)
				return true
			}
			s.gossipClosed = true
			s.sendBattlefieldList(guid, 0, bgTypeID)
		} else if item.Action == 17 { // GOSSIP_OPTION_UNLEARNPETTALENTS
			// Player::OnGossipSelect (Player.cpp:14653-14656): SendCloseGossip
			// before ResetPetTalents. Go's pet talents reset only at login —
			// no interactive model — so only the close is bridged.
			if !s.sendGossipComplete() {
				return false
			}
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
				defaultMenu, loadErr := s.prepareCreatureGossip(ctx, guid, entry, npcFlags, item.ActionMenuID, false)
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
			// OutdoorPvP manager, which has no Go model — the menu closes.
			s.gossipClosed = true
		}
		// Player::OnGossipSelect (Player.cpp:14699): the BoxMoney charge lands
		// after the option arms — early-return arms (invalid battlemaster
		// above) never charge.
		if boxCost > 0 && s.player != nil {
			s.player.Money -= boxCost
			if cdb := s.server.CharactersStore.DB; cdb != nil {
				_, _ = cdb.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", s.player.Money, s.playerGUID)
			}
			s.sendPlayerUpdate()
		}
	}
	if s.gossip == nil && !s.gossipClosed {
		s.gossipClosed = true
		if err := s.write(uint16(protocol.OpcodeSMSG_GOSSIP_COMPLETE), nil, true); err != nil {
			return false
		}
	}
	s.debug("gossip selection handled", "account", s.accountName, "entry", entry, "list", listID)
	return true
}

// handleGameObjectGossipSelect processes CMSG_GOSSIP_SELECT_OPTION for a
// gameobject GUID — the gameobject arm of
// WorldSession::HandleGossipSelectOptionOpcode (MiscHandler.cpp:131-136,
// 168-173, 198-204).
func (s *session) handleGameObjectGossipSelect(ctx context.Context, guid uint64, item gossipMenuItem, code string) bool {
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
	// Eluna::OnGossipSelect[Code] fires before the native arms; a Lua false
	// return skips them (MiscHandler.cpp:198-204).
	if s.fireGameObjectGossipSelectHook(ctx, guid, item.Sender, item.Action, code) {
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
		// before the submenu. The submenu re-prepare for a gameobject source
		// has no Go loader (Go builds gossip menus for creatures only), so
		// only the POI arm bridges; the menu otherwise stays open.
		if item.ActionPoiID != 0 {
			s.sendGossipPOI(ctx, item.ActionPoiID)
		}
	case 2: // GOSSIP_OPTION_QUESTGIVER
		// PrepareQuestMenu+SendPreparedQuest for a gameobject source has no
		// Go bridge — Go never opens gameobject quest menus natively — so a
		// scripted select hook is the only consumer of this arm.
	}
	// Player::OnGossipSelect (Player.cpp:14699): the BoxMoney charge lands
	// after the option arms.
	if boxCost > 0 {
		s.player.Money -= boxCost
		if cdb := s.server.CharactersStore.DB; cdb != nil {
			_, _ = cdb.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", s.player.Money, s.playerGUID)
		}
		s.sendPlayerUpdate()
	}
	if s.gossip == nil && !s.gossipClosed {
		s.gossipClosed = true
		if err := s.write(uint16(protocol.OpcodeSMSG_GOSSIP_COMPLETE), nil, true); err != nil {
			return false
		}
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
		return false
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
// OnGossipSelect[Code] tail has no counterpart. The feign-death strip C++
// runs for every arm is a standing Go-wide delta — the creature select arm
// in this file never had it either.
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
	s.fireItemGossipSelectHook(ctx, objectUint32OrZero(item, "Entry"), menuItem, code, item)
	s.debug("item gossip selection handled", "account", s.accountName, "guid", guid, "list", listID)
	return true
}

func (s *session) prepareCreatureGossip(ctx context.Context, guid uint64, entry, npcFlags, menuID uint32, isDefaultMenu bool) (*gossipMenuState, error) {
	// TrinityCore PrepareGossipMenu learns an unknown flight node and aborts
	// the menu build; the map itself opens through the taxi option afterwards.
	if npcFlags&unitNPCFlagFlightmaster != 0 && s.learnNewTaxiNode(ctx, guid) {
		return nil, nil
	}
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
	if npcFlags&0x00000002 != 0 && s.player != nil {
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
// Player::PrepareGossipMenu (Player.cpp:14478-14489): the option is hidden (and
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
		   OR t.Id = ?
		LIMIT 1`, creatureEntry, creatureEntry).Scan(&tType, &tReq)
	if err != nil {
		return false
	}
	return s.isTrainerValidForPlayer(uint32(tType), uint32(tReq))
}

func (s *session) loadCreatureGossipOptions(ctx context.Context, menuID, npcFlags, creatureEntry uint32, creatureGUID uint64) ([]loadedGossipOption, error) {
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
	options := make([]loadedGossipOption, 0, 32)
	for rows.Next() {
		var id, icon, optionType, requiredFlags, actionMenuID, actionPoiID, coded, boxMoney int64
		var message, boxMessage string
		if err := rows.Scan(&id, &icon, &message, &optionType, &requiredFlags, &actionMenuID, &actionPoiID, &coded, &boxMoney, &boxMessage); err != nil {
			return nil, err
		}
		if requiredFlags != 0 && uint32(requiredFlags)&npcFlags == 0 {
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
		if optionType == 17 { // GOSSIP_OPTION_UNLEARNPETTALENTS needs a hunter pet with talents; no interactive pet-talent model in Go
			continue
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
	return options, rows.Err()
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

func (s *session) canInteractWithCreature(creature *scripting.Object) bool {
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
		return false
	}
	r := protocol.NewReader(payload)
	npcGUID, err := r.ReadU64()
	if err != nil {
		return false
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
	s.player.HomebindZone = s.player.Zone
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
