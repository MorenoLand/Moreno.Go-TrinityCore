package world

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// send command port: send_commandscript (cs_send.cpp), the "send" root with
// 4 arms (items, mail, message, money). THIRTY-FIFTH of 40 Commands groups
// (cs_script_loader.cpp decl 53 / call 98; call order reset(97) ->
// send(98)). Trinity checks permission only on the invoker leaf node
// (ChatCommand.cpp:487), so each arm gates exactly its own C++ permission
// (RBAC.h:351-355); the root permission 483 is DEAD — the root uses the
// deprecated nullptr+subtable ChatCommandBuilder overload (ChatCommand.h),
// which drops the RBACPermissions param, so bare ".send" prints the syntax
// line ungated (same dead-root pattern as the mmap/modify/rbac/reload/reset
// ports); permissionCommandSend stays in permissions.go as documentation.
//
// All four arms are native on the Go mail system (mail.go: the mail /
// mail_items / item_instance tables, GM stationery + 90-day expiry for
// GM-sent mail per MailDraft::SendMailTo):
//   - `mail` inserts a GM-stationery mail row for the resolved receiver
//     (online or offline, via miscResolvePlayerTarget; the 499 not-found text
//     is printed once by the resolver, == C++ extractPlayerTarget).
//   - `items` validates each item entry against item_template, splits counts
//     across max stacks per ItemTemplate::GetMaxStackSize (ItemTemplate.h:686:
//     Stackable <= 0 or 2147483647 means effectively unlimited, not 1),
//     enforces the 12-item mail cap, creates the item_instance rows, and
//     attaches them via mail_items. Zero item tokens sends an empty mail ==
//     C++ (no guard on the tail).
//   - `money` inserts a GM-stationery mail row carrying the copper amount.
//   - `message` requires an online player and delivers the two C++ texts via
//     SMSG_AREA_TRIGGER_MESSAGE (== WorldSession::SendAreaTriggerMessage,
//     MiscHandler.cpp:628; message first, then the administrator tag, ==
//     cs_send.cpp:273-274).
//
// Quoted subject/text parsing mirrors extractQuotedArg (Chat.cpp:714): the
// token must open with a quote (unquoted subject/text fails the command ==
// C++), the value runs to the next quote or the end of the token, and a ""
// pair yields an empty string. LANG texts are inlined from the TDB enUS
// recall (no in-tree trinity_string seed).
//
// Documented no-bridge/deltas (not stubs): the message arm's isLogingOut
// gate has no Go bridge (no session logout state); negative item counts
// clamp to 1 (C++ wraps to a huge uint32 and trips the 12-item cap, same
// net refusal for stackables); player links render as plain names per the
// tree's miscPlayerLink convention; console-vs-chat branches moot (Go
// commands always sessioned).

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

// sendQuotedArg mirrors ChatHandler::extractQuotedArg (Chat.cpp:714): the
// token must open with a quote; the value runs to the next quote or the end
// of the token (a "" pair yields an empty string).
func sendQuotedArg(tok string) (string, bool) {
	if !strings.HasPrefix(tok, "\"") {
		return "", false
	}
	rest := tok[1:]
	if i := strings.IndexByte(rest, '"'); i >= 0 {
		return rest[:i], true
	}
	return rest, true
}

// sendMailArgs parses: name "subject text" "mail text" [rest...].
func sendMailArgs(args []string) (name, subject, text string, rest []string, ok bool) {
	toks := splitQuotedArgs(args)
	if len(toks) < 3 {
		return "", "", "", nil, false
	}
	subject, ok = sendQuotedArg(toks[1])
	if !ok {
		return "", "", "", nil, false
	}
	text, ok = sendQuotedArg(toks[2])
	if !ok {
		return "", "", "", nil, false
	}
	return toks[0], subject, text, toks[3:], true
}

// handleCmdSend dispatches the "send" root (cs_send.cpp:43-46).
func (s *session) handleCmdSend(ctx context.Context, args []string) {
	if len(args) == 0 {
		// The root's own 483 perm is dead (deprecated nullptr+subtable
		// overload drops it); bare ".send" prints the syntax line ungated.
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
	if !ok {
		return // miscResolvePlayerTarget already reported LANG 499 == C++
	}
	if !s.sendMailInsert(ctx, guid, subject, text, 0, nil) {
		return
	}
	// cs_send.cpp HandleSendMailCommand: MailReceiver(target, ...) with an
	// online target fires AddNewMailDeliverTime(deliver_time) (immediate here),
	// i.e. SendNewMail()/++unReadMails — the online receiver gets the icon.
	s.sendMailNotify(guid)
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
	if !ok {
		return // miscResolvePlayerTarget already reported LANG 499 == C++
	}
	if !s.sendMailInsert(ctx, guid, subject, text, uint32(money), nil) {
		return
	}
	// Same AddNewMailDeliverTime notify as HandleSendMailCommand: the GM
	// money mail is delivered immediately (cs_send.cpp HandleSendMoneyCommand).
	s.sendMailNotify(guid)
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
	if !ok {
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
		if maxStack <= 0 || maxStack == 2147483647 {
			maxStack = 0x7FFFFFFE // GetMaxStackSize: Stackable <= 0 or INT32_MAX means unlimited
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
	if !ok {
		return // miscResolvePlayerTarget already reported LANG 499 == C++
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
	// Same AddNewMailDeliverTime notify: the GM item mail is delivered
	// immediately (cs_send.cpp HandleSendItemsCommand).
	s.sendMailNotify(guid)
	s.sendSysMessage(fmt.Sprintf("Mail sent to %s.", targetName)) // LANG_MAIL_SENT 169
}

// handleSendMessageCmd mirrors HandleSendMessageCommand (cs_send.cpp:261):
// the target must be online; the two C++ texts go out as
// SMSG_AREA_TRIGGER_MESSAGE (== WorldSession::SendAreaTriggerMessage,
// MiscHandler.cpp:628).
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
	// == WorldSession::SendAreaTriggerMessage (MiscHandler.cpp:628): message
	// first, then the administrator tag (cs_send.cpp:273-274).
	_ = target.write(uint16(protocol.OpcodeSMSG_AREA_TRIGGER_MESSAGE), protocol.BuildAreaTriggerMessage(msg), true)
	_ = target.write(uint16(protocol.OpcodeSMSG_AREA_TRIGGER_MESSAGE), protocol.BuildAreaTriggerMessage("|cffff0000[Message from administrator]:|r"), true)
	s.sendSysMessage(fmt.Sprintf("Message sent to %s: %s.", target.player.Name, msg)) // LANG_SENDMESSAGE 1102
}
