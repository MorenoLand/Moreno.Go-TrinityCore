package world

import (
	"context"
	"database/sql"
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
	if s.isDeadOrGhost() && isBattlegroundMap(s.player.Map) && objectUint32OrZero(creature, "NPCFlags")&npcFlagSpiritGuide != 0 {
		return s.handleAreaSpiritHealerQueue(ctx, payload)
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
		npcFlags := objectUint32OrZero(creature, "NPCFlags")
		defaultMenu, err := s.prepareCreatureGossip(ctx, guid, entry, npcFlags, objectUint32OrZero(creature, "GossipMenuID"))
		if err != nil {
			s.debug("default gossip load failed", "account", s.accountName, "entry", entry, "error", err)
			s.gossipClosed = true
			_ = s.write(uint16(protocol.OpcodeSMSG_GOSSIP_COMPLETE), nil, true)
			return true
		}
		if defaultMenu != nil && len(defaultMenu.Items) == 0 && len(defaultMenu.Quests) == 0 {
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
				auction := protocol.NewBuffer(9)
				auction.WriteU64(guid)
				auction.WriteU8(1)
				return s.write(uint16(protocol.OpcodeMSG_AUCTION_HELLO), auction.Bytes(), true) == nil
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
	s.debug("gossip hello handled", "account", s.accountName, "entry", entry)
	return true
}

func (s *session) handleGossipSelectOption(ctx context.Context, payload []byte) bool {
	reader := protocol.NewReader(payload)
	guid, err := reader.ReadU64()
	if err != nil {
		return false
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
	creature := s.luaCreature(ctx, guid)
	if creature == nil {
		return true
	}
	if !s.canInteractWithCreature(creature) {
		s.debug("gossip selection out of interaction range", "account", s.accountName, "guid", guid)
		return true
	}
	entry, ok := objectUint32Field(creature, "Entry")
	if !ok {
		return true
	}
	s.gossip = nil
	s.gossipClosed = false
	args := []any{uint32(2), s.luaPlayer(), creature, item.Sender, item.Action}
	if item.Coded {
		args = append(args, code)
	}
	if s.server.Features != nil && s.server.Features.Scripts != nil {
		if _, err := s.server.Features.Scripts.Trigger(ctx, "creature_gossip:"+strconv.FormatUint(uint64(entry), 10), 2, args...); err != nil {
			s.debug("gossip selection hook failed", "account", s.accountName, "entry", entry, "error", err)
		}
	}
	if s.gossip == nil && !s.gossipClosed {
		// TrinityCore Gossip_Option: 1 gossip submenu, 2 questgiver (quest
		// menu), 3 vendor, 4 taxivendor, 5 trainer, 8 innkeeper, 9 banker,
		// 13 auctioneer. The old code treated 2/3/4 as vendor/taxi/trainer,
		// so selecting a vendor (3) opened the flight map instead.
		if item.Action == 3 || item.Action == 15 { // GOSSIP_OPTION_VENDOR / GOSSIP_OPTION_ARMORER
			s.sendVendorList(ctx, guid)
			s.gossipClosed = true
		} else if item.Action == 6 { // GOSSIP_OPTION_SPIRITHEALER
			if s.isDeadOrGhost() && objectUint32OrZero(creature, "NPCFlags")&npcFlagSpiritHealer != 0 {
				confirm := protocol.NewBuffer(8)
				confirm.WriteU64(guid)
				if err := s.write(uint16(protocol.OpcodeSMSG_SPIRIT_HEALER_CONFIRM), confirm.Bytes(), true); err != nil {
					return false
				}
				s.gossipClosed = true
			}
		} else if item.Action == 4 { // GOSSIP_OPTION_TAXIVENDOR
			s.sendTaxiMenu(ctx, guid)
			s.gossipClosed = true
		} else if item.Action == 5 { // GOSSIP_OPTION_TRAINER
			s.sendTrainerList(ctx, guid)
			s.gossipClosed = true
		} else if item.Action == 8 { // GOSSIP_OPTION_INNKEEPER
			s.gossipClosed = true
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
			s.gossipClosed = true
			tabard := protocol.NewBuffer(8)
			tabard.WriteU64(guid)
			if err := s.write(uint16(protocol.OpcodeMSG_TABARDVENDOR_ACTIVATE), tabard.Bytes(), true); err != nil {
				return false
			}
		} else if item.Action == 13 { // GOSSIP_OPTION_AUCTIONEER
			s.gossipClosed = true
			auction := protocol.NewBuffer(9)
			auction.WriteU64(guid)
			auction.WriteU8(1)
			if err := s.write(uint16(protocol.OpcodeMSG_AUCTION_HELLO), auction.Bytes(), true); err != nil {
				return false
			}
		} else if item.Action == 14 { // GOSSIP_OPTION_STABLEPET
			s.gossipClosed = true
			stableBuf := protocol.NewBuffer(8)
			stableBuf.WriteU64(guid)
			return s.handleListStabledPets(ctx, stableBuf.Bytes())
		} else if item.Action == 16 { // GOSSIP_OPTION_UNLEARNTALENTS
			s.gossipClosed = true
			wipeBuf := protocol.NewBuffer(12)
			wipeBuf.WriteU64(guid)
			cost := uint32(100000) // 10 gold base reset cost
			wipeBuf.WriteU32(cost)
			if err := s.write(uint16(protocol.OpcodeMSG_TALENT_WIPE_CONFIRM), wipeBuf.Bytes(), true); err != nil {
				return false
			}
		} else if item.Action == 1 && item.ActionMenuID != 0 {
			defaultMenu, loadErr := s.prepareCreatureGossip(ctx, guid, entry, objectUint32OrZero(creature, "NPCFlags"), item.ActionMenuID)
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

func (s *session) prepareCreatureGossip(ctx context.Context, guid uint64, entry, npcFlags, menuID uint32) (*gossipMenuState, error) {
	// TrinityCore PrepareGossipMenu learns an unknown flight node and aborts
	// the menu build; the map itself opens through the taxi option afterwards.
	if npcFlags&unitNPCFlagFlightmaster != 0 && s.learnNewTaxiNode(ctx, guid) {
		return nil, nil
	}
	menu := &gossipMenuState{SenderGUID: guid, MenuID: menuID, TitleID: 0x00FFFFFF, Items: make(map[uint32]gossipMenuItem)}
	var titleID int64
	err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT TextID FROM gossip_menu WHERE MenuID = ? ORDER BY TextID LIMIT 1", menuID).Scan(&titleID)
	if err == nil {
		menu.TitleID = uint32(titleID)
	} else if err != sql.ErrNoRows && !missingTable(err) {
		return nil, err
	}
	options, err := s.loadCreatureGossipOptions(ctx, menuID, npcFlags, entry)
	if err != nil {
		return nil, err
	}
	if len(options) == 0 && menuID != 0 {
		options, err = s.loadCreatureGossipOptions(ctx, 0, npcFlags, entry)
		if err != nil {
			return nil, err
		}
	}
	for _, option := range options {
		if option.Item.Action == 6 && !s.isDeadOrGhost() {
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

type loadedGossipOption struct {
	ID   uint32
	Item gossipMenuItem
}

func (s *session) loadCreatureGossipOptions(ctx context.Context, menuID, npcFlags, creatureEntry uint32) ([]loadedGossipOption, error) {
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
		// ConditionMgr gate (SourceType 14): seasonal/event/class/race/
		// quest-chain options stay hidden until their conditions pass.
		meets, err := s.meetGossipOptionConditions(ctx, menuID, uint32(id), creatureEntry)
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
	if !s.canInteractWithNPC(ctx, npcGUID, 0x00000004) {
		return true
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
