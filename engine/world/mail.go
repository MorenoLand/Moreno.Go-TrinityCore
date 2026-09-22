package world

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// Reference: SharedDefines.h:3533 (enum MailResponseType)
const (
	mailSend             uint32 = 0
	mailMoneyTaken       uint32 = 1
	mailItemTaken        uint32 = 2
	mailReturnedToSender uint32 = 3
	mailDeleted          uint32 = 4
	mailMadePermanent    uint32 = 5
)

// Reference: SharedDefines.h:3543 (enum MailResponseResult)
const (
	mailOk                       uint32 = 0
	mailErrEquipError            uint32 = 1
	mailErrCannotSendToSelf      uint32 = 2
	mailErrNotEnoughMoney        uint32 = 3
	mailErrRecipientNotFound     uint32 = 4
	mailErrNotYourTeam           uint32 = 5
	mailErrInternalError         uint32 = 6
	mailErrDisabledForTrialAcc   uint32 = 14
	mailErrRecipientCapReached   uint32 = 15
	mailErrCantSendWrappedCOD    uint32 = 16
	mailErrMailAndChatSuspended  uint32 = 17
	mailErrTooManyAttachments    uint32 = 18
	mailErrMailAttachmentInvalid uint32 = 19
	mailErrItemHasExpired        uint32 = 21

	equipErrMailBoundItem uint32 = 72
)

type mailEntryRecord struct {
	ID           uint32
	MessageType  uint8
	Stationery   uint32
	Sender       uint64
	Receiver     uint64
	Subject      string
	Body         string
	Money        uint32
	COD          uint32
	Checked      uint32
	ExpireTime   int64
	DeliverTime  int64
	MailTemplate uint32
	Items        []mailItemRecord
}

type mailItemRecord struct {
	AttachID           uint32
	ItemEntry          uint32
	Count              uint32
	MaxDurability      uint32
	Durability         uint32
	Enchantments       [21]uint32
	RandomPropertyID   uint32
	RandomPropertySeed uint32
	Charges            uint32
}

func (s *session) sendNewMailNotification(ctx context.Context) {
	if s == nil || s.playerGUID == 0 || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	var unread int64
	err := s.server.CharactersStore.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM mail
		WHERE receiver = ? AND deliver_time <= ? AND expire_time > ? AND (COALESCE(checked, 0) & 1) = 0`, s.playerGUID, time.Now().Unix(), time.Now().Unix()).Scan(&unread)
	if err != nil || unread == 0 {
		if err == nil {
			s.unreadMails = 0
		}
		return
	}
	s.unreadMails = uint32(unread)
	packet := protocol.NewBuffer(4)
	packet.WriteF32(0)
	_ = s.write(uint16(protocol.OpcodeSMSG_RECEIVED_MAIL), packet.Bytes(), true)
}

func (s *session) loadMailState(ctx context.Context) {
	if s == nil || s.playerGUID == 0 || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	now := time.Now().Unix()
	var unread, next int64
	err := s.server.CharactersStore.DB.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(CASE WHEN deliver_time <= ? AND expire_time > ? AND (COALESCE(checked, 0) & 1) = 0 THEN 1 ELSE 0 END), 0),
		COALESCE(MIN(CASE WHEN deliver_time > ? AND expire_time > ? THEN deliver_time END), 0)
		FROM mail WHERE receiver = ?`, now, now, now, now, s.playerGUID).Scan(&unread, &next)
	if err != nil {
		return
	}
	if unread > 0 {
		s.unreadMails = uint32(unread)
	} else {
		s.unreadMails = 0
	}
	if next > 0 {
		s.nextMailDelivery = next
	} else {
		s.nextMailDelivery = 0
	}
}

func (s *session) updateMailDeliveries(ctx context.Context, now int64) {
	if s == nil || !s.playerLoaded || s.nextMailDelivery == 0 || now < s.nextMailDelivery {
		return
	}
	s.loadMailState(ctx)
	if s.unreadMails > 0 {
		s.sendNewMailNotification(ctx)
	}
}

func (s *Server) updateMailDeliveries(ctx context.Context, now int64) {
	if s == nil {
		return
	}
	s.sessionsMu.RLock()
	sessions := make([]*session, 0, len(s.sessions))
	for sess := range s.sessions {
		sessions = append(sessions, sess)
	}
	s.sessionsMu.RUnlock()
	for _, sess := range sessions {
		sess.updateMailDeliveries(ctx, now)
	}
}

func (s *session) handleGetMailList(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return true
	}
	s.expireOldMails(ctx)
	db := s.server.CharactersStore.DB
	now := time.Now().Unix()
	rows, err := db.QueryContext(ctx, `SELECT id, messageType, stationery, mailTemplateId, sender, receiver, subject, body, expire_time, deliver_time, money, cod, checked
		FROM mail WHERE receiver = ? AND deliver_time <= ? AND expire_time > ? ORDER BY id DESC LIMIT 50`, s.playerGUID, now, now)
	if err != nil {
		return true
	}
	// Drain the mail rows before touching mail_items: a nested query while the
	// outer cursor is open requires a second pooled connection and deadlocks
	// pools capped at one connection (see TestMailSendingAndReceiving).
	var mails []mailEntryRecord
	for rows.Next() {
		var id, msgType, stat, tmpl, sender, receiver, exp, del, money, cod, checked int64
		var subject, body string
		if err := rows.Scan(&id, &msgType, &stat, &tmpl, &sender, &receiver, &subject, &body, &exp, &del, &money, &cod, &checked); err != nil {
			continue
		}
		m := mailEntryRecord{
			ID:           uint32(id),
			MessageType:  uint8(msgType),
			Stationery:   uint32(stat),
			Sender:       uint64(sender),
			Receiver:     uint64(receiver),
			Subject:      subject,
			Body:         body,
			ExpireTime:   exp,
			DeliverTime:  del,
			Money:        uint32(money),
			COD:          uint32(cod),
			Checked:      uint32(checked),
			MailTemplate: uint32(tmpl),
		}
		if m.Stationery == 0 {
			m.Stationery = 41 // Standard default letter stationery
		}
		mails = append(mails, m)
	}
	rows.Close()
	itemMailProperties := func(itemEntry int64) (uint32, uint32) {
		if s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
			return 0, 0
		}
		var itemLevel, quality, inventoryType, randomSuffix, maxDurability int64
		if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(ItemLevel, 0), COALESCE(Quality, 0), COALESCE(InventoryType, 0), COALESCE(RandomSuffix, 0), COALESCE(MaxDurability, 0) FROM item_template WHERE entry = ?", itemEntry).Scan(&itemLevel, &quality, &inventoryType, &randomSuffix, &maxDurability); err != nil {
			return 0, 0
		}
		maxD := uint32(maxDurability)
		if s.server.Data == nil || randomSuffix == 0 {
			return 0, maxD
		}
		points, found, err := s.server.Data.RandPropPoints(uint32(itemLevel))
		if err != nil || !found {
			return 0, maxD
		}
		index := -1
		switch inventoryType {
		case 1, 4, 5, 7, 17, 20:
			index = 0
		case 3, 6, 8, 10, 12:
			index = 1
		case 2, 9, 11, 14, 16, 23:
			index = 2
		case 13, 21, 22:
			index = 3
		case 15, 25, 26:
			index = 4
		}
		if index < 0 {
			return 0, maxD
		}
		switch quality {
		case 2:
			return points.Good[index], maxD
		case 3:
			return points.Superior[index], maxD
		case 4:
			return points.Epic[index], maxD
		}
		return 0, maxD
	}
	// Load attached items per drained mail.
	for i := range mails {
		iRows, iErr := db.QueryContext(ctx, `SELECT mi.item_guid, mi.item_template, COALESCE(ii.count, 1), COALESCE(ii.durability, 0), COALESCE(ii.enchantments, ''), COALESCE(ii.randomPropertyId, 0), COALESCE(ii.charges, '')
			FROM mail_items AS mi
			LEFT JOIN item_instance AS ii ON ii.guid = mi.item_guid
			WHERE mi.mail_id = ?`, mails[i].ID)
		if iErr == nil {
			for iRows.Next() {
				var iGuid, iTmpl, iCount, iDur, randomPropertyID int64
				var enchantments, charges string
				if iRows.Scan(&iGuid, &iTmpl, &iCount, &iDur, &enchantments, &randomPropertyID, &charges) == nil {
					item := mailItemRecord{AttachID: uint32(iGuid), ItemEntry: uint32(iTmpl), Count: uint32(iCount), Durability: uint32(iDur), RandomPropertyID: uint32(int32(randomPropertyID))}
					item.RandomPropertySeed, item.MaxDurability = 0, 0
					seed, maxDurability := itemMailProperties(iTmpl)
					item.MaxDurability = maxDurability
					if randomPropertyID < 0 {
						item.RandomPropertySeed = seed
					}
					for index, token := range strings.Fields(enchantments) {
						if index >= len(item.Enchantments) {
							break
						}
						if value, err := strconv.ParseInt(token, 10, 64); err == nil {
							item.Enchantments[index] = uint32(value)
						}
					}
					chargeFields := strings.Fields(charges)
					if len(chargeFields) == 5 {
						if value, err := strconv.ParseInt(chargeFields[0], 10, 32); err == nil {
							item.Charges = uint32(int32(value))
						}
					}
					mails[i].Items = append(mails[i].Items, item)
				}
			}
			iRows.Close()
		}
	}
	// Build SMSG_MAIL_LIST_RESULT (0x23B)
	packet := protocol.NewBuffer(512)
	packet.WriteU32(uint32(len(mails))) // TotalNumRecords
	packet.WriteU8(uint8(len(mails)))   // Mails.size()
	for _, m := range mails {
		daysLeft := float32(m.ExpireTime-now) / 86400.0
		if daysLeft < 0 {
			daysLeft = 0
		}
		msgBuf := protocol.NewBuffer(128)
		msgBuf.WriteU32(m.ID)
		msgBuf.WriteU8(m.MessageType)
		if m.MessageType == 0 {
			msgBuf.WriteU64(m.Sender)
		} else {
			msgBuf.WriteU32(uint32(m.Sender))
		}
		msgBuf.WriteU32(m.COD)
		msgBuf.WriteU32(0) // PackageID
		msgBuf.WriteU32(m.Stationery)
		msgBuf.WriteU32(m.Money)
		msgBuf.WriteU32(m.Checked)
		msgBuf.WriteF32(daysLeft)
		msgBuf.WriteU32(m.MailTemplate)
		msgBuf.WriteString(m.Subject)
		msgBuf.WriteString(m.Body)
		msgBuf.WriteU8(uint8(len(m.Items)))
		for pos, it := range m.Items {
			msgBuf.WriteU8(uint8(pos))
			msgBuf.WriteU32(it.AttachID)
			msgBuf.WriteU32(it.ItemEntry)
			for j := 0; j < len(it.Enchantments); j += 3 {
				msgBuf.WriteU32(it.Enchantments[j])
				msgBuf.WriteU32(it.Enchantments[j+1])
				msgBuf.WriteU32(it.Enchantments[j+2])
			}
			msgBuf.WriteU32(it.RandomPropertyID)
			msgBuf.WriteU32(it.RandomPropertySeed)
			msgBuf.WriteU32(it.Count)
			msgBuf.WriteU32(it.Charges)
			msgBuf.WriteU32(it.MaxDurability)
			msgBuf.WriteU32(it.Durability)
			msgBuf.WriteU8(1) // Unlocked
		}
		entryBytes := msgBuf.Bytes()
		packet.WriteU16(uint16(len(entryBytes)))
		packet.Write(entryBytes)
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_MAIL_LIST_RESULT), packet.Bytes(), true)
	s.debug("mail list sent", "account", s.accountName, "mails", len(mails))
	return true
}

func (s *session) handleSendMail(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 20 {
		return true
	}
	reader := protocol.NewReader(payload)
	_, _ = reader.ReadU64() // mailbox GUID
	targetName, err := reader.ReadCString()
	if err != nil || targetName == "" {
		return false
	}
	subject, err := reader.ReadCString()
	if err != nil {
		return false
	}
	body, err := reader.ReadCString()
	if err != nil {
		return false
	}
	stationery, err := reader.ReadU32()
	if err != nil {
		stationery = 41
	}
	_, _ = reader.ReadU32() // packageID
	attachCount, err := reader.ReadU8()
	if err != nil {
		attachCount = 0
	}
	type itemAttachment struct {
		Slot     uint8
		ItemGUID uint64
	}
	var attachments []itemAttachment
	for i := uint8(0); i < attachCount; i++ {
		slot, _ := reader.ReadU8()
		itemGUID, _ := reader.ReadU64()
		if itemGUID != 0 {
			attachments = append(attachments, itemAttachment{Slot: slot, ItemGUID: itemGUID})
		}
	}
	money, _ := reader.ReadU32()
	cod, _ := reader.ReadU32()

	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}
	// Find receiver
	var receiverGUID int64
	err = cdb.QueryRowContext(ctx, "SELECT guid FROM characters WHERE UPPER(name) = UPPER(?) LIMIT 1", targetName).Scan(&receiverGUID)
	if err != nil || receiverGUID == 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_SEND_MAIL_RESULT), buildSendMailResult(0, mailSend, mailErrRecipientNotFound, 0, 0, 0), true)
		return true
	}
	if uint64(receiverGUID) == s.playerGUID {
		_ = s.write(uint16(protocol.OpcodeSMSG_SEND_MAIL_RESULT), buildSendMailResult(0, mailSend, mailErrCannotSendToSelf, 0, 0, 0), true)
		return true
	}
	var receiverRace int64
	_ = cdb.QueryRowContext(ctx, "SELECT race FROM characters WHERE guid = ?", receiverGUID).Scan(&receiverRace)
	if s.player.Race != 0 && receiverRace != 0 && teamForRace(s.player.Race) != teamForRace(uint8(receiverRace)) {
		_ = s.write(uint16(protocol.OpcodeSMSG_SEND_MAIL_RESULT), buildSendMailResult(0, mailSend, mailErrNotYourTeam, 0, 0, 0), true)
		return true
	}
	var mailCount int64
	_ = cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM mail WHERE receiver = ?", receiverGUID).Scan(&mailCount)
	if mailCount >= 100 {
		_ = s.write(uint16(protocol.OpcodeSMSG_SEND_MAIL_RESULT), buildSendMailResult(0, mailSend, mailErrRecipientCapReached, 0, 0, 0), true)
		return true
	}
	if len(attachments) > 12 {
		_ = s.write(uint16(protocol.OpcodeSMSG_SEND_MAIL_RESULT), buildSendMailResult(0, mailSend, mailErrTooManyAttachments, 0, 0, 0), true)
		return true
	}
	if len(attachments) == 0 {
		cod = 0
	}
	for _, att := range attachments {
		var itemFlags int64
		_ = cdb.QueryRowContext(ctx, "SELECT flags FROM item_instance WHERE guid = ?", att.ItemGUID).Scan(&itemFlags)
		if itemFlags&1 != 0 {
			_ = s.write(uint16(protocol.OpcodeSMSG_SEND_MAIL_RESULT), buildSendMailResult(0, mailSend, mailErrEquipError, equipErrMailBoundItem, 0, 0), true)
			return true
		}
	}
	postageFee := uint32(30)
	if len(attachments) > 0 {
		postageFee = uint32(30 * len(attachments))
	}
	totalRequired := postageFee + money
	if s.player.Money >= totalRequired && postageFee > 0 {
		s.updateAchievementCriteria(criteriaTypeGoldSpentForMail, 0, postageFee)
	}
	if s.player.Money < totalRequired {
		_ = s.write(uint16(protocol.OpcodeSMSG_SEND_MAIL_RESULT), buildSendMailResult(0, mailSend, mailErrNotEnoughMoney, 0, 0, 0), true)
		return true
	}
	s.player.Money -= totalRequired
	_, _ = cdb.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", s.player.Money, s.playerGUID)
	now := time.Now().Unix()
	expire := now + 30*86400
	hasItems := 0
	if len(attachments) > 0 {
		hasItems = 1
	}
	var nextMailID int64
	_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) + 1 FROM mail").Scan(&nextMailID)
	if nextMailID <= 0 {
		nextMailID = 1
	}
	// TrinityCore MailDraft::SendMailTo stores checked as MAIL_CHECK_MASK_HAS_BODY (0x10)
	// when a body text is present and MAIL_CHECK_MASK_COPIED (0x04) otherwise.
	checked := uint32(0x04)
	if body != "" {
		checked = 0x10
	}
	_, err = cdb.ExecContext(ctx, `INSERT INTO mail (id, messageType, stationery, mailTemplateId, sender, receiver, subject, body, has_items, expire_time, deliver_time, money, cod, checked)
		VALUES (?, 0, ?, 0, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		nextMailID, stationery, s.playerGUID, receiverGUID, subject, body, hasItems, expire, now, money, cod, checked)
	if err != nil {
		return true
	}
	for _, att := range attachments {
		var itemEntry int64
		_ = cdb.QueryRowContext(ctx, "SELECT itemEntry FROM item_instance WHERE guid = ?", att.ItemGUID).Scan(&itemEntry)
		_, _ = cdb.ExecContext(ctx, "DELETE FROM character_inventory WHERE guid = ? AND item = ?", s.playerGUID, att.ItemGUID)
		s.despawnItem(att.ItemGUID)
		_, _ = cdb.ExecContext(ctx, "UPDATE item_instance SET owner_guid = ? WHERE guid = ?", receiverGUID, att.ItemGUID)
		_, _ = cdb.ExecContext(ctx, "INSERT INTO mail_items (mail_id, item_guid, item_template, receiver) VALUES (?, ?, ?, ?)", nextMailID, att.ItemGUID, itemEntry, receiverGUID)
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_SEND_MAIL_RESULT), buildSendMailResult(uint32(nextMailID), mailSend, mailOk, 0, 0, 0), true)
	_ = s.sendInventoryItems(ctx)
	s.sendPlayerMoneyUpdate()
	s.sendPlayerUpdate()
	s.sendMailNotify(uint64(receiverGUID))
	s.debug("mail sent successfully", "from", s.accountName, "to", targetName, "mail_id", nextMailID)
	return true
}

func (s *session) handleMailTakeMoney(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 12 {
		return true
	}
	reader := protocol.NewReader(payload)
	_, _ = reader.ReadU64()
	mailID, err := reader.ReadU32()
	if err != nil {
		return false
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}
	var money int64
	err = cdb.QueryRowContext(ctx, "SELECT money FROM mail WHERE id = ? AND receiver = ? LIMIT 1", mailID, s.playerGUID).Scan(&money)
	if err != nil || money <= 0 {
		return true
	}
	s.player.Money += uint32(money)
	_, _ = cdb.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", s.player.Money, s.playerGUID)
	_, _ = cdb.ExecContext(ctx, "UPDATE mail SET money = 0 WHERE id = ?", mailID)
	_ = s.write(uint16(protocol.OpcodeSMSG_SEND_MAIL_RESULT), buildSendMailResult(mailID, mailMoneyTaken, mailOk, 0, 0, 0), true)
	s.sendPlayerMoneyUpdate()
	s.sendPlayerUpdate()
	s.debug("mail money collected", "account", s.accountName, "mail_id", mailID, "money", money)
	return true
}

func (s *session) handleMailTakeItem(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 16 {
		return true
	}
	reader := protocol.NewReader(payload)
	_, _ = reader.ReadU64()
	mailID, err := reader.ReadU32()
	if err != nil {
		return false
	}
	attachID, err := reader.ReadU32()
	if err != nil {
		return false
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}
	var itemEntry, senderGUID, cod int64
	var subject string
	err = cdb.QueryRowContext(ctx, `SELECT m.sender, m.subject, m.cod, i.item_template
		FROM mail_items AS i
		JOIN mail AS m ON m.id = i.mail_id
		WHERE i.mail_id = ? AND i.item_guid = ? LIMIT 1`, mailID, attachID).Scan(&senderGUID, &subject, &cod, &itemEntry)
	if err != nil || itemEntry == 0 {
		return true
	}
	// Check COD (Cash On Delivery) payment
	if cod > 0 {
		if s.player.Money < uint32(cod) {
			_ = s.write(uint16(protocol.OpcodeSMSG_SEND_MAIL_RESULT), buildSendMailResult(mailID, mailItemTaken, mailErrNotEnoughMoney, 0, 0, 0), true)
			return true
		}
	}
	// Find free inventory slot (backpack slots 23..38 or equipped bags 19..22)
	freeBagKey, freeClientBag, freeSlot, ok := s.findFreeInventorySlot(ctx, s.playerGUID)
	if !ok {
		_ = s.write(uint16(protocol.OpcodeSMSG_SEND_MAIL_RESULT), buildSendMailResult(mailID, mailItemTaken, mailErrEquipError, uint32(equipErrInvFull), 0, 0), true)
		return true
	}

	// If mail has COD, charge player and send payment mail to sender (TC: MailDraft::SendMailTo with MAIL_CHECK_MASK_COD_PAYMENT)
	if cod > 0 {
		s.player.Money -= uint32(cod)
		_, _ = cdb.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", s.player.Money, s.playerGUID)
		_, _ = cdb.ExecContext(ctx, "UPDATE mail SET cod = 0 WHERE id = ?", mailID)

		now := time.Now().Unix()
		var nextMailID int64
		_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) + 1 FROM mail").Scan(&nextMailID)
		if nextMailID <= 0 {
			nextMailID = 1
		}
		codSubject := "COD Payment: " + subject
		_, _ = cdb.ExecContext(ctx, `INSERT INTO mail (id, messageType, stationery, mailTemplateId, sender, receiver, subject, body, has_items, expire_time, deliver_time, money, cod, checked)
			VALUES (?, 0, 41, 0, ?, ?, ?, '', 0, ?, ?, ?, 0, 0x04)`,
			nextMailID, s.playerGUID, senderGUID, codSubject, now+30*86400, now, cod)
		s.sendMailNotify(uint64(senderGUID))
		s.sendPlayerMoneyUpdate()
	}

	_, _ = cdb.ExecContext(ctx, "INSERT INTO character_inventory (guid, bag, slot, item) VALUES (?, ?, ?, ?)", s.playerGUID, freeBagKey, freeSlot, attachID)
	_, _ = cdb.ExecContext(ctx, "UPDATE item_instance SET owner_guid = ? WHERE guid = ?", s.playerGUID, attachID)
	_, _ = cdb.ExecContext(ctx, "DELETE FROM mail_items WHERE mail_id = ? AND item_guid = ?", mailID, attachID)
	// Check if any items left
	var remainingCount int64
	_ = cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM mail_items WHERE mail_id = ?", mailID).Scan(&remainingCount)
	if remainingCount == 0 {
		_, _ = cdb.ExecContext(ctx, "UPDATE mail SET has_items = 0 WHERE id = ?", mailID)
	}
	var itemCount int64
	_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(count, 1) FROM item_instance WHERE guid = ?", attachID).Scan(&itemCount)
	if itemCount <= 0 {
		itemCount = 1
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_SEND_MAIL_RESULT), buildSendMailResult(mailID, mailItemTaken, mailOk, 0, attachID, uint32(itemCount)), true)
	_ = s.sendInventoryItems(ctx)
	s.sendPlayerUpdate()
	s.debug("mail item collected", "account", s.accountName, "mail_id", mailID, "item", itemEntry, "slot", freeSlot, "bag", freeClientBag)
	return true
}

func (s *session) handleMailDelete(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 12 {
		return true
	}
	reader := protocol.NewReader(payload)
	_, _ = reader.ReadU64()
	mailID, err := reader.ReadU32()
	if err != nil {
		return false
	}
	cdb := s.server.CharactersStore.DB
	if cdb != nil {
		_, _ = cdb.ExecContext(ctx, "DELETE FROM mail WHERE id = ? AND receiver = ?", mailID, s.playerGUID)
		_, _ = cdb.ExecContext(ctx, "DELETE FROM mail_items WHERE mail_id = ?", mailID)
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_SEND_MAIL_RESULT), buildSendMailResult(mailID, mailDeleted, mailOk, 0, 0, 0), true)
	s.debug("mail deleted", "account", s.accountName, "mail_id", mailID)
	return true
}

func (s *session) handleMailMarkAsRead(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 12 {
		return true
	}
	reader := protocol.NewReader(payload)
	_, _ = reader.ReadU64()
	mailID, err := reader.ReadU32()
	if err != nil {
		return false
	}
	if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE mail SET checked = (checked | 1) WHERE id = ? AND receiver = ?", mailID, s.playerGUID)
	}
	return true
}

func (s *session) handleQueryNextMailTime(ctx context.Context) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	// TrinityCore HandleQueryNextMailTime: unread and already delivered mails
	// (checked & MAIL_CHECK_MASK_READ) == 0 are listed once per sender (max 3
	// entries); when none exist the client is told -DAY so no mail icon shows.
	type nextMailEntry struct {
		Sender      uint64
		AltSender   uint32
		MessageType uint8
		Stationery  uint32
		TimeLeft    float32
	}
	var entries []nextMailEntry
	if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		rows, err := s.server.CharactersStore.DB.QueryContext(ctx, `SELECT messageType, stationery, sender, deliver_time
			FROM mail WHERE receiver = ? AND (checked & 1) = 0 AND deliver_time <= ? ORDER BY id DESC`, s.playerGUID, time.Now().Unix())
		if err == nil {
			seenSenders := make(map[uint64]struct{})
			for rows.Next() {
				var msgType, stationery, sender int64
				var deliverTime int64
				if err := rows.Scan(&msgType, &stationery, &sender, &deliverTime); err != nil {
					continue
				}
				senderGUID := uint64(sender)
				if _, dup := seenSenders[senderGUID]; dup {
					continue
				}
				seenSenders[senderGUID] = struct{}{}
				altSender := uint32(0)
				if msgType != 0 {
					altSender = uint32(sender)
				}
				entries = append(entries, nextMailEntry{
					Sender:      senderGUID,
					AltSender:   altSender,
					MessageType: uint8(msgType),
					Stationery:  uint32(stationery),
					TimeLeft:    float32(deliverTime - time.Now().Unix()),
				})
				if len(seenSenders) > 2 {
					break
				}
			}
			rows.Close()
		}
	}
	buf := protocol.NewBuffer(32)
	if len(entries) > 0 {
		buf.WriteF32(0) // NextMailTime: mail is ready now
	} else {
		buf.WriteF32(-86400) // -DAY: no unread mail, hides the notification
	}
	buf.WriteU32(uint32(len(entries)))
	for _, entry := range entries {
		if entry.MessageType == 0 { // MAIL_NORMAL sends the full player GUID
			buf.WriteU64(entry.Sender)
		} else {
			buf.WriteU64(0)
		}
		if entry.MessageType != 0 {
			buf.WriteU32(entry.AltSender) // AltSenderID
			buf.WriteU32(uint32(entry.MessageType))
		} else {
			buf.WriteU32(0)
			buf.WriteU32(0)
		}
		buf.WriteU32(entry.Stationery)
		buf.WriteF32(entry.TimeLeft)
	}
	_ = s.write(uint16(protocol.OpcodeMSG_QUERY_NEXT_MAIL_TIME), buf.Bytes(), true)
	return true
}

// buildSendMailResult builds SMSG_SEND_MAIL_RESULT (0x239).
// Reference: WorldPackets::Mail::MailCommandResult::Write (MailPackets.cpp:204).
func buildSendMailResult(mailID, action, result, equipError, attachID, count uint32) []byte {
	buf := protocol.NewBuffer(24)
	buf.WriteU32(mailID)
	buf.WriteU32(action)
	buf.WriteU32(result)
	if result == mailErrEquipError {
		buf.WriteU32(equipError)
	}
	if action == mailItemTaken && (result == mailOk || result == mailErrItemHasExpired) {
		buf.WriteU32(attachID)
		buf.WriteU32(count)
	}
	return buf.Bytes()
}

// handleMailCreateTextItem processes CMSG_MAIL_CREATE_TEXT_ITEM (0x24A).
// Reference: WorldSession::HandleMailCreateTextItem (MailHandler.cpp:565).
func (s *session) handleMailCreateTextItem(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 12 {
		return true
	}
	r := protocol.NewReader(payload)
	_, _ = r.ReadU64() // mailbox GUID
	mailID, _ := r.ReadU32()

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		cdb := s.server.CharactersStore.DB

		// Check for free slot in backpack (slots 23..38)
		usedSlots := make(map[uint8]bool)
		rows, err := cdb.QueryContext(ctx, "SELECT slot FROM character_inventory WHERE guid = ? AND bag = 0", s.playerGUID)
		if err == nil {
			for rows.Next() {
				var sl int64
				if rows.Scan(&sl) == nil {
					usedSlots[uint8(sl)] = true
				}
			}
			rows.Close()
		}
		freeSlot := uint8(0xFF)
		for sl := uint8(23); sl <= 38; sl++ {
			if !usedSlots[sl] {
				freeSlot = sl
				break
			}
		}
		if freeSlot == 0xFF {
			// Inventory full error (MAIL_ERR_EQUIP_ERROR)
			_ = s.write(uint16(protocol.OpcodeSMSG_SEND_MAIL_RESULT), buildSendMailResult(mailID, mailMadePermanent, mailErrEquipError, uint32(equipErrInvFull), 0, 0), true)
			return true
		}

		var body string
		_ = cdb.QueryRowContext(ctx, "SELECT body FROM mail WHERE id = ?", mailID).Scan(&body)

		var nextGUID uint64
		_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(guid), 0) + 1 FROM item_instance").Scan(&nextGUID)
		if nextGUID == 0 {
			nextGUID = uint64(time.Now().UnixNano())
		}
		const mailBodyItemTemplate uint32 = 8383 // Plain Letter
		_, _ = cdb.ExecContext(ctx, "INSERT INTO item_instance (guid, itemEntry, owner_guid, creatorGuid, count, duration, charges, flags, enchantments, randomPropertyId, durability, playedTime, text) VALUES (?, ?, ?, 0, 1, 0, '', 1, '', 0, 0, 0, ?)", nextGUID, mailBodyItemTemplate, s.playerGUID, body)
		_, _ = cdb.ExecContext(ctx, "INSERT INTO character_inventory (guid, bag, slot, item) VALUES (?, 0, ?, ?)", s.playerGUID, freeSlot, nextGUID)
		_, _ = cdb.ExecContext(ctx, "UPDATE mail SET checked = checked | 4 WHERE id = ?", mailID) // MAIL_CHECK_MASK_COPIED = 4
		_ = s.sendItemCreate(nextGUID, mailBodyItemTemplate, 1, 0, freeSlot)
		_ = s.sendInventoryItems(ctx)
		s.sendPlayerUpdate()
	}

	// Send result: action 5 (MAIL_MADE_PERMANENT), result 0 (MAIL_OK)
	_ = s.write(uint16(protocol.OpcodeSMSG_SEND_MAIL_RESULT), buildSendMailResult(mailID, mailMadePermanent, mailOk, 0, 0, 0), true)
	return true
}

// handleMailReturnToSender processes CMSG_MAIL_RETURN_TO_SENDER (0x248).
// Reference: WorldSession::HandleMailReturnToSender (MailHandler.cpp:351).
func (s *session) handleMailReturnToSender(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 12 {
		return true
	}
	r := protocol.NewReader(payload)
	_, _ = r.ReadU64() // mailbox GUID
	mailID, err := r.ReadU32()
	if err != nil {
		return false
	}

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		cdb := s.server.CharactersStore.DB

		var senderGUID, receiverGUID int64
		var messageType int
		err := cdb.QueryRowContext(ctx, "SELECT sender, receiver, messageType FROM mail WHERE id = ? AND receiver = ? LIMIT 1", mailID, s.playerGUID).Scan(&senderGUID, &receiverGUID, &messageType)
		if err != nil {
			_ = s.write(uint16(protocol.OpcodeSMSG_SEND_MAIL_RESULT), buildSendMailResult(mailID, mailReturnedToSender, mailErrInternalError, 0, 0, 0), true)
			return true
		}

		// Only return normal mail if the original sender exists
		if messageType == 0 && senderGUID > 0 {
			var origSenderExists int64
			_ = cdb.QueryRowContext(ctx, "SELECT guid FROM characters WHERE guid = ? LIMIT 1", senderGUID).Scan(&origSenderExists)
			if origSenderExists == 0 {
				// Sender character no longer exists; delete mail and attached items
				_, _ = cdb.ExecContext(ctx, "DELETE FROM item_instance WHERE guid IN (SELECT item_guid FROM mail_items WHERE mail_id = ?)", mailID)
				_, _ = cdb.ExecContext(ctx, "DELETE FROM mail_items WHERE mail_id = ?", mailID)
				_, _ = cdb.ExecContext(ctx, "DELETE FROM mail WHERE id = ?", mailID)
				_ = s.write(uint16(protocol.OpcodeSMSG_SEND_MAIL_RESULT), buildSendMailResult(mailID, mailReturnedToSender, mailOk, 0, 0, 0), true)
				return true
			}

			// Update attached items ownership back to the original sender
			_, _ = cdb.ExecContext(ctx, "UPDATE item_instance SET owner_guid = ? WHERE guid IN (SELECT item_guid FROM mail_items WHERE mail_id = ?)", senderGUID, mailID)
			_, _ = cdb.ExecContext(ctx, "UPDATE mail_items SET receiver = ? WHERE mail_id = ?", senderGUID, mailID)

			now := time.Now().Unix()
			expireTime := now + 30*86400 // 30 days
			_, _ = cdb.ExecContext(ctx, `UPDATE mail SET
				receiver = ?,
				sender = ?,
				messageType = 0,
				checked = 2,
				deliver_time = ?,
				expire_time = ?
				WHERE id = ?`, senderGUID, receiverGUID, now, expireTime, mailID)

			s.sendMailNotify(uint64(senderGUID))
		} else {
			// Not normal player mail (e.g. auction/creature mail without sender), just delete
			_, _ = cdb.ExecContext(ctx, "DELETE FROM item_instance WHERE guid IN (SELECT item_guid FROM mail_items WHERE mail_id = ?)", mailID)
			_, _ = cdb.ExecContext(ctx, "DELETE FROM mail_items WHERE mail_id = ?", mailID)
			_, _ = cdb.ExecContext(ctx, "DELETE FROM mail WHERE id = ?", mailID)
		}
	}

	_ = s.write(uint16(protocol.OpcodeSMSG_SEND_MAIL_RESULT), buildSendMailResult(mailID, mailReturnedToSender, mailOk, 0, 0, 0), true)
	return true
}

// expireOldMails sweeps expired mails in characters DB.
// Reference: ObjectMgr::ReturnOrDeleteOldMails (ObjectMgr.cpp:6308).
func (s *session) expireOldMails(ctx context.Context) {
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	cdb := s.server.CharactersStore.DB
	now := time.Now().Unix()
	rows, err := cdb.QueryContext(ctx, `SELECT id, messageType, sender, receiver, has_items, checked FROM mail WHERE expire_time <= ?`, now)
	if err != nil {
		return
	}
	type expiredMail struct {
		id, msgType, sender, receiver, hasItems, checked int64
	}
	var expired []expiredMail
	for rows.Next() {
		var em expiredMail
		if err := rows.Scan(&em.id, &em.msgType, &em.sender, &em.receiver, &em.hasItems, &em.checked); err == nil {
			expired = append(expired, em)
		}
	}
	rows.Close()

	for _, em := range expired {
		if em.hasItems > 0 {
			// If not normal player mail, or already returned / COD payment, delete attached items and mail
			if em.msgType != 0 || (em.checked&(2|0x08)) != 0 {
				_, _ = cdb.ExecContext(ctx, "DELETE FROM item_instance WHERE guid IN (SELECT item_guid FROM mail_items WHERE mail_id = ?)", em.id)
				_, _ = cdb.ExecContext(ctx, "DELETE FROM mail_items WHERE mail_id = ?", em.id)
				_, _ = cdb.ExecContext(ctx, "DELETE FROM mail WHERE id = ?", em.id)
			} else {
				// Return mail to sender
				var senderExists int64
				_ = cdb.QueryRowContext(ctx, "SELECT guid FROM characters WHERE guid = ? LIMIT 1", em.sender).Scan(&senderExists)
				if senderExists == 0 {
					_, _ = cdb.ExecContext(ctx, "DELETE FROM item_instance WHERE guid IN (SELECT item_guid FROM mail_items WHERE mail_id = ?)", em.id)
					_, _ = cdb.ExecContext(ctx, "DELETE FROM mail_items WHERE mail_id = ?", em.id)
					_, _ = cdb.ExecContext(ctx, "DELETE FROM mail WHERE id = ?", em.id)
				} else {
					_, _ = cdb.ExecContext(ctx, "UPDATE item_instance SET owner_guid = ? WHERE guid IN (SELECT item_guid FROM mail_items WHERE mail_id = ?)", em.sender, em.id)
					_, _ = cdb.ExecContext(ctx, "UPDATE mail_items SET receiver = ? WHERE mail_id = ?", em.sender, em.id)
					expireTime := now + 30*86400
					_, _ = cdb.ExecContext(ctx, `UPDATE mail SET
						receiver = ?,
						sender = ?,
						messageType = 0,
						checked = 2,
						deliver_time = ?,
						expire_time = ?
						WHERE id = ?`, em.sender, em.receiver, now, expireTime, em.id)
					s.sendMailNotify(uint64(em.sender))
				}
			}
		} else {
			// No items attached, delete expired mail
			_, _ = cdb.ExecContext(ctx, "DELETE FROM mail WHERE id = ?", em.id)
		}
	}
}
