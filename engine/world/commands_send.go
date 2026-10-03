package world

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// send command port: send_commandscript (cs_send.cpp), the "send" root with
// 4 arms (items, mail, message, money). THIRTY-FOURTH of 39 Commands groups
// (cs_script_loader.cpp decl 53 / call 98; call order reset(97) ->
// send(98)). Trinity checks permission only on the invoker leaf node
// (ChatCommand.cpp:487), so each arm gates exactly its own C++ permission
// (RBAC.h:351-355); the root permission 483 covers the bare ".send".
//
// All four arms are native on the Go mail system (mail.go: the mail /
// mail_items / item_instance tables, GM stationery + 90-day expiry for
// GM-sent mail per MailDraft::SendMailTo):
//   - `mail` inserts a GM-stationery mail row for the resolved receiver
//     (online or offline, via miscResolvePlayerTarget).
//   - `items` validates each item entry against item_template, splits counts
//     across max stacks, enforces the 12-item mail cap, creates the
//     item_instance rows, and attaches them via mail_items.
//   - `money` inserts a GM-stationery mail row carrying the copper amount.
//   - `message` requires an online player and delivers the two C++
//     area-trigger texts through SMSG_MESSAGECHAT system chat (the Go
//     equivalent of Player::SendAreaTriggerMessage).
//
// Quoted subject/text parsing mirrors extractQuotedArg via the existing
// splitQuotedArgs + unquoteCommandToken helpers. LANG texts are inlined from
// the TDB enUS recall (no in-tree trinity_string seed).

// sendMailTarget resolves the receiver for the send arms (online session or
// offline guid), mirroring ChatHandler::extractPlayerTarget.
func (s *session) sendMailTarget(ctx context.Context, args []string) (online *session, guid uint64, name string, ok bool) {
	return s.miscResolvePlayerTarget(ctx, args)
}

// sendMailInsert mirrors the MailDraft::SendMailTo INSERT for GM-sent command
// mail (cs_send.cpp): MAIL_NORMAL, MAIL_STATIONERY_GM (41), 90-day expiry,
// immediate delivery, checked = HAS_BODY when a body is present.
func (s *session) sendMailInsert(ctx context.Context, receiverGUID uint64, subject, body string, money uint32, itemGUIDs []uint64) bool {
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return false
	}
	var nextMailID int64
	_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) + 1 FROM mail").Scan(&nextMailID)
	if nextMailID <= 0 {
		nextMailID = 1
	}
	now := time.Now().Unix()
	hasItems := 0
	if len(itemGUIDs) > 0 {
		hasItems = 1
	}
	checked := uint32(0x04)
	if body != "" {
		checked = 0x10
	}
	_, err := cdb.ExecContext(ctx, `INSERT INTO mail (id, messageType, stationery, mailTemplateId, sender, receiver, subject, body, has_items, expire_time, deliver_time, money, cod, checked)
		VALUES (?, 0, 41, 0, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?)`,
		nextMailID, s.playerGUID, receiverGUID, subject, body, hasItems, now+90*24*3600, now, money, checked)
	if err != nil {
		return false
	}
	for _, ig := range itemGUIDs {
		_, _ = cdb.ExecContext(ctx, "INSERT INTO mail_items (mail_id, item_guid, receiver) VALUES (?, ?, ?)", nextMailID, ig, receiverGUID)
	}
	return true
}

// sendMailArgs parses: name "subject text" "mail text" [rest...].
func sendMailArgs(args []string) (name, subject, text string, rest []string, ok bool) {
	toks := splitQuotedArgs(args)
	if len(toks) < 3 {
		return "", "", "", nil, false
	}
	return toks[0], unquoteCommandToken(toks[1]), unquoteCommandToken(toks[2]), toks[3:], true
}

// handleCmdSend dispatches the "send" root (cs_send.cpp:43-46).
func (s *session) handleCmdSend(ctx context.Context, args []string) {
	if len(args) == 0 {
		if s.miscDeny(ctx, permissionCommandSend) {
			return
		}
		s.sendSysMessage("Syntax: .send items|mail|message|money")
		return
	}
	sub := strings.ToLower(args[0])
	rest := args[1:]
	// The Trinity parser prefix-matches command names at every nesting
	// level; match the arm the same way here.
	switch {
	case strings.HasPrefix("items", sub):
		s.handleSendItems(ctx, rest)
	case strings.HasPrefix("mail", sub):
		s.handleSendMailCmd(ctx, rest)
	case strings.HasPrefix("message", sub):
		s.handleSendMessageCmd(ctx, rest)
	case strings.HasPrefix("money", sub):
		s.handleSendMoney(ctx, rest)
	default:
		s.sendSysMessage("Syntax: .send items|mail|message|money")
	}
}

// handleSendMailCmd mirrors HandleSendMailCommand (cs_send.cpp:57).
func (s *session) handleSendMailCmd(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandSendMail) {
		return
	}
	name, subject, text, _, ok := sendMailArgs(args)
	if !ok {
		s.sendSysMessage(`Syntax: .send mail <name> "subject" "text"`)
		return
	}
	_, guid, targetName, ok := s.sendMailTarget(ctx, []string{name})
	if !ok || guid == 0 {
		s.sendSysMessage("Player not found.") // LANG_PLAYER_NOT_FOUND 499
		return
	}
	if !s.sendMailInsert(ctx, guid, subject, text, 0, nil) {
		return
	}
	s.sendSysMessage(fmt.Sprintf("Mail sent to %s.", targetName)) // LANG_MAIL_SENT 169
}

// handleSendMoney mirrors HandleSendMoneyCommand (cs_send.cpp:210).
func (s *session) handleSendMoney(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandSendMoney) {
		return
	}
	name, subject, text, rest, ok := sendMailArgs(args)
	if !ok || len(rest) < 1 {
		s.sendSysMessage(`Syntax: .send money <name> "subject" "text" <money>`)
		return
	}
	money := cAtoi(rest[0])
	if money <= 0 {
		return
	}
	_, guid, targetName, ok := s.sendMailTarget(ctx, []string{name})
	if !ok || guid == 0 {
		s.sendSysMessage("Player not found.") // LANG_PLAYER_NOT_FOUND 499
		return
	}
	if !s.sendMailInsert(ctx, guid, subject, text, uint32(money), nil) {
		return
	}
	s.sendSysMessage(fmt.Sprintf("Mail sent to %s.", targetName)) // LANG_MAIL_SENT 169
}

// handleSendItems mirrors HandleSendItemsCommand (cs_send.cpp:102): each
// itemN[:countN] token is validated against item_template, split across max
// stacks, and capped at MAX_MAIL_ITEMS (12).
func (s *session) handleSendItems(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandSendItems) {
		return
	}
	name, subject, text, rest, ok := sendMailArgs(args)
	if !ok || len(rest) < 1 {
		s.sendSysMessage(`Syntax: .send items <name> "subject" "text" <item1[:count1]> ...`)
		return
	}
	type itemPair struct{ entry, count uint32 }
	var items []itemPair
	wdb := s.server.WorldStore.DB
	for _, tok := range rest {
		idStr, countStr := tok, "1"
		if i := strings.Index(tok, ":"); i >= 0 {
			idStr, countStr = tok[:i], tok[i+1:]
		}
		itemID := uint32(cAtoi(idStr))
		if itemID == 0 {
			return
		}
		var maxCount, stackable int64
		if wdb == nil || wdb.QueryRowContext(ctx, "SELECT COALESCE(MaxCount, 0), COALESCE(stackable, 1) FROM item_template WHERE entry = ?", itemID).Scan(&maxCount, &stackable) != nil {
			s.sendSysMessage(fmt.Sprintf("Item id %d is invalid.", itemID)) // LANG_COMMAND_ITEMIDINVALID 435
			return
		}
		itemCount := int64(cAtoi(countStr))
		if itemCount < 1 {
			itemCount = 1
		}
		if maxCount > 0 && itemCount > maxCount {
			s.sendSysMessage(fmt.Sprintf("Invalid item count %d for item %d.", itemCount, itemID)) // LANG_COMMAND_INVALID_ITEM_COUNT 52
			return
		}
		maxStack := stackable
		if maxStack < 1 {
			maxStack = 1
		}
		for itemCount > maxStack {
			items = append(items, itemPair{itemID, uint32(maxStack)})
			itemCount -= maxStack
		}
		items = append(items, itemPair{itemID, uint32(itemCount)})
		if len(items) > 12 { // MAX_MAIL_ITEMS
			s.sendSysMessage("You can't send more than 12 items.") // LANG_COMMAND_MAIL_ITEMS_LIMIT 53
			return
		}
	}
	_, guid, targetName, ok := s.sendMailTarget(ctx, []string{name})
	if !ok || guid == 0 {
		s.sendSysMessage("Player not found.") // LANG_PLAYER_NOT_FOUND 499
		return
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return
	}
	var itemGUIDs []uint64
	for _, it := range items {
		var nextItemGUID int64
		_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(guid), 0) + 1 FROM item_instance").Scan(&nextItemGUID)
		if nextItemGUID <= 0 {
			nextItemGUID = 1
		}
		if _, err := cdb.ExecContext(ctx, "INSERT INTO item_instance (guid, itemEntry, owner_guid, count) VALUES (?, ?, ?, ?)", nextItemGUID, it.entry, guid, it.count); err != nil {
			continue // Item::CreateItem failing drops the item, like C++
		}
		itemGUIDs = append(itemGUIDs, uint64(nextItemGUID))
	}
	if !s.sendMailInsert(ctx, guid, subject, text, 0, itemGUIDs) {
		return
	}
	s.sendSysMessage(fmt.Sprintf("Mail sent to %s.", targetName)) // LANG_MAIL_SENT 169
}

// handleSendMessageCmd mirrors HandleSendMessageCommand (cs_send.cpp:261):
// the target must be online; the two C++ area-trigger texts go out through
// SMSG_MESSAGECHAT system chat (the Go equivalent of SendAreaTriggerMessage).
func (s *session) handleSendMessageCmd(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandSendMessage) {
		return
	}
	if len(args) < 2 {
		s.sendSysMessage("Syntax: .send message <name> <message>")
		return
	}
	target := s.sessionForPlayerName(normalizePlayerName(args[0]))
	if target == nil || target.player == nil {
		s.sendSysMessage("Player not found.") // LANG_PLAYER_NOT_FOUND 499
		return
	}
	msg := strings.Join(args[1:], " ")
	_ = target.write(uint16(protocol.OpcodeSMSG_MESSAGECHAT), protocol.BuildSystemChatMessage(msg), true)
	_ = target.write(uint16(protocol.OpcodeSMSG_MESSAGECHAT), protocol.BuildSystemChatMessage("|cffff0000[Message from administrator]:|r"), true)
	s.sendSysMessage(fmt.Sprintf("Message sent to %s: %s.", target.player.Name, msg)) // LANG_SENDMESSAGE 1102
}
