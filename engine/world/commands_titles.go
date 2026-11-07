package world

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// titles command port: titles_commandscript (cs_titles.cpp), the "titles"
// root with 4 arms (add, current, remove, set[mask]). THIRTY-NINTH of 40
// Commands groups (cs_script_loader.cpp decl 57 / call 102; call order
// ticket(101) -> titles(102) -> wp(103)). Trinity checks permission only on
// the invoker leaf node (ChatCommand.cpp:487), so the add/current/remove/mask
// arms gate exactly their own C++ permissions (RBAC.h:629-634); the root
// permission 761 and the "set" node's 765 are DEAD in C++ — both use the
// deprecated 6-arg nullptr+subtable ChatCommandBuilder overload
// (ChatCommand.h:259-263 drops RBACPermissions, pure SubCommandEntry), so
// bare ".titles" and bare ".titles set" print the syntax listing ungated
// (same dead-root-perm pattern as the ticket/quest/reset ports);
// permissionCommandTitles/permissionCommandTitlesSet stay in permissions.go
// as documentation.
//
// All four arms are native on the Go title model (player_state.go): the
// CharTitles.dbc bridge (wotlk.Store.CharTitle, id -> Name/Name1/MaskID)
// supplies the title entries, KnownTitles[6]uint32 carries the MaskID bits,
// and ChosenTitle carries the active MaskID — exactly the C++
// PLAYER__FIELD_KNOWN_TITLES / PLAYER_CHOSEN_TITLE semantics. Title changes
// persist to the characters table (chosenTitle / knownTitles columns) and
// are broadcast with buildPlayerValuesUpdate, like handleSetTitle.
//
// Targeting mirrors getSelectedPlayer (Chat.cpp:301-309: no selection falls
// back to the invoker; a set-but-offline selection reports
// LANG_NO_CHAR_SELECTED) plus the HasLowerSecurity message leg
// (s.security < target.security; HasLowerSecurity prints, Chat.cpp:96-99).
// LANG texts are inlined from the TDB enUS recall (no in-tree trinity_string seed).
//
// R13: the root and all 4 arms return bool — bare/unknown subcommands
// return false (the framework shows the help listing), denial is a silent
// false (the C++ framework denies before invoking the handler), empty args
// are silent false (extractKeyFromLink("") returns null with no message),
// wrong-link-type prints the arm syntax + true (tick-314 quest pattern:
// C++ sends LANG_WRONG_LINK_TYPE 511 + framework help), and message legs
// return true (SetSentErrorMessage in C++).

// strtokToken mirrors C strtok(s, delims): skips leading delimiters and
// returns the token up to the next delimiter plus the remainder after it.
func strtokToken(s, delims string) (token, rest string, ok bool) {
	i := strings.IndexFunc(s, func(r rune) bool { return !strings.ContainsRune(delims, r) })
	if i < 0 {
		return "", "", false
	}
	s = s[i:]
	if j := strings.IndexAny(s, delims); j >= 0 {
		return s[:j], s[j+1:], true
	}
	return s, "", true
}

// parseTitleLink mirrors extractKeyFromLink("Htitle") + atoi (Chat.cpp:362)
// token by token: strtok(text, "|") skips the color, strtok(nullptr, ":")
// takes the link type, strtok(nullptr, "|") takes the key segment, and
// strtok(keySegment, ":|") takes the key. A plain title id or a
// |Htitle:title_id|h[name]|h|r shift-click link both work. The signed atoi
// value is preserved (C++ reports LANG_INVALID_TITLE_ID with the raw id,
// so "-5" prints -5, not 0). A wrong link type sends LANG_WRONG_LINK_TYPE
// (511, unseeded in-tree, not guessed) and returns null — callers print
// the arm syntax as the help equivalent (tick-314 quest pattern).
// R-TITLES-LINK-1: an empty key does NOT return null. For the colored form
// |cffffffff|Htitle:|h[x]|h|r the key segment is the display text "h[x]",
// so atoi gives 0 and the id<=0 leg reports it (verified by running the
// C++ strtok sequence); the colorless form |Htitle:|h[x]|h|r fails the
// link-type token instead and keeps the wrong-link-type disposition.
func parseTitleLink(arg string) (int32, bool) {
	if !strings.HasPrefix(arg, "|") {
		return int32(cAtoi(arg)), true
	}
	// strtok(text, "|"): skip the color.
	_, rest, ok := strtokToken(arg, "|")
	if !ok {
		return 0, false
	}
	// strtok(nullptr, ":"): the link type.
	linkType, rest, ok := strtokToken(rest, ":")
	if !ok || linkType != "Htitle" {
		return 0, false // wrong link type: null in the C++ path too
	}
	// strtok(nullptr, "|"): the key segment.
	keys, _, ok := strtokToken(rest, "|")
	if !ok {
		return 0, false
	}
	// strtok(keySegment, ":|"): the key.
	key, _, ok := strtokToken(keys, ":|")
	if !ok {
		return 0, false
	}
	return int32(cAtoi(key)), true
}

// knowsTitleBit mirrors Player::HasTitle(uint32 bitIndex)
// (Player.cpp:24799): the KnownTitles bit for a MaskID. It is NOT
// playerHasTitle (chosen-or-known): passing the target's own ChosenTitle to
// playerHasTitle is tautologically true, so the chosen-reset legs in
// remove/setMask could never fire.
func (s *session) knowsTitleBit(maskID uint32) bool {
	if s.player == nil {
		return false
	}
	idx := maskID / 32
	if int(idx) >= len(s.player.KnownTitles) {
		return false
	}
	return s.player.KnownTitles[idx]&(uint32(1)<<(maskID%32)) != 0
}

// sendTitleEarned mirrors the SMSG_TITLE_EARNED leg of Player::SetTitle
// (Player.cpp:24814-24834): MaskID plus 1 for earned, 0 for lost, sent to
// the target's session. SetTitle only sends it on a bit change, so callers
// gate on the pre-change bit.
func (s *session) sendTitleEarned(maskID uint32, earned bool) {
	packet := protocol.NewBuffer(8)
	packet.WriteU32(maskID)
	if earned {
		packet.WriteU32(1)
	} else {
		packet.WriteU32(0)
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_TITLE_EARNED), packet.Bytes(), true)
}

// parseSScanfU64 mirrors sscanf(args, UI64FMTD) ("%llu", cs_titles.cpp:229):
// leading whitespace skipped, optional sign, leading digit run, trailing
// junk ignored, no digits -> 0, a leading '-' wraps modulo 2^64, overflow
// clamps to ^uint64(0) (strtoull). ParseUint base 10 was wrong here:
// "12abc" parses 12 in C++ (Go: error -> 0) and "-1" wraps to max (Go: 0).
func parseSScanfU64(s string) uint64 {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\v' || s[i] == '\f' || s[i] == '\r') {
		i++
	}
	neg := false
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		neg = s[i] == '-'
		i++
	}
	var n uint64
	digits := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		d := uint64(s[i] - '0')
		if n > (^uint64(0)-d)/10 {
			n = ^uint64(0)
		} else {
			n = n*10 + d
		}
		digits++
		i++
	}
	if digits == 0 {
		return 0
	}
	if neg {
		return -n
	}
	return n
}

// titlesTarget mirrors getSelectedPlayer (Chat.cpp:301-309): no selection
// resolves to the invoker (self fallback); only a set-but-unresolvable
// selection reports LANG_NO_CHAR_SELECTED. The HasLowerSecurity silent fail
// is the s.security < target.security comparison.
func (s *session) titlesTarget() *session {
	if s.selection != 0 && s.server != nil {
		if ts := s.server.playerSessionForGUID(s.selection); ts != nil && ts.player != nil {
			if s.security < ts.security {
				// HasLowerSecurity prints LANG_YOURS_SECURITY_IS_LOW
				// (Chat.cpp:96-99).
				s.sendSysMessage("Your security level is too low.")
				return nil
			}
			return ts
		}
		s.sendSysMessage("No character selected.") // LANG_NO_CHAR_SELECTED 116
		return nil
	}
	if s.player == nil {
		// getSelectedPlayer is null without a session player
		// (Chat.cpp:300-303): console reports LANG_NO_CHAR_SELECTED.
		s.sendSysMessage("No character selected.") // LANG_NO_CHAR_SELECTED 116
		return nil
	}
	return s
}

// persistTitles writes the target's KnownTitles/ChosenTitle to the characters
// table and broadcasts the unit-field update.
func (s *session) persistTitles(ctx context.Context, target *session) {
	if target.player == nil {
		return
	}
	cdb := s.server.CharactersStore.DB
	if cdb != nil {
		parts := make([]string, len(target.player.KnownTitles))
		for i, v := range target.player.KnownTitles {
			parts[i] = strconv.FormatUint(uint64(v), 10)
		}
		_, _ = cdb.ExecContext(ctx, "UPDATE characters SET chosenTitle = ?, knownTitles = ? WHERE guid = ?",
			target.player.ChosenTitle, strings.Join(parts, " "), target.playerGUID)
	}
	fields := map[int]uint32{unitFieldChosenTitle: target.player.ChosenTitle}
	for i, v := range target.player.KnownTitles {
		fields[playerFieldKnownTitles+i] = v
	}
	if pVal, pErr := s.server.buildPlayerValuesUpdate(target.playerGUID, fields); pErr == nil && pVal != nil {
		_ = target.write(pVal.Opcode, pVal.Payload.Bytes(), true)
	}
	target.sendPlayerUpdate()
}

// titleName formats the gendered title name like the C++ snprintf.
func titleName(target *session, name, name1 string) string {
	if target.player != nil && target.player.Gender == 1 { // GENDER_FEMALE
		return fmt.Sprintf(name1, target.player.Name)
	}
	if target.player != nil {
		return fmt.Sprintf(name, target.player.Name)
	}
	return name
}

// handleCharacterTitles mirrors HandleCharacterTitlesCommand
// (cs_character.cpp:239): lists the target's known CharTitles. Previously
// documented-blocked on the CharTitles DBC store; the wotlk.Store.CharTitle
// bridge now exists, so the arm is native.
func (s *session) handleCharacterTitles(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandCharacterTitles) {
		return false
	}
	t, rest, ok := s.resolveCharacterTarget(ctx, args)
	if !ok {
		return true
	}
	// The C++ takes only Optional<PlayerIdentifier>; a trailing token fails
	// the whole-string-consumed check (ChatCommand.h:112).
	if len(rest) != 0 {
		return false
	}
	var target *session
	if t.online != nil {
		target = t.online
	} else {
		s.sendSysMessage("Player not found.") // LANG_PLAYER_NOT_FOUND 499
		return true
	}
	if target.player == nil || s.server.Data == nil {
		return false
	}
	file, err := s.server.Data.File("CharTitles")
	if err != nil {
		return false
	}
	// C++ iterates id < GetNumRows() with LookupEntry(id)
	// (cs_character.cpp:255-258), so a CharTitles entry whose ID is at or
	// above the row count is never listed; the loop below mirrors that
	// instead of walking every record.
	for id := uint32(0); id < uint32(file.Records()); id++ {
		rec, ok := file.Find(id)
		if !ok {
			continue
		}
		maskID, merr := rec.Uint32(36)
		if merr != nil {
			continue
		}
		if int(maskID/32) >= len(target.player.KnownTitles) ||
			target.player.KnownTitles[maskID/32]&(uint32(1)<<(maskID%32)) == 0 {
			continue
		}
		name, _ := rec.String(2)
		name1, _ := rec.String(19)
		// The C++ falls back to the default locale when the session-locale
		// name is empty, then skips a still-empty name (cs_character.cpp:
		// 261-265); Go reads the enUS fields only, so an empty name skips.
		title := titleName(target, name, name1)
		if title == "" {
			continue
		}
		active := ""
		if target.player.ChosenTitle == maskID {
			active = " (active)"
		}
		// LANG_TITLE_LIST_CHAT shape: id (idx:mask) - [name locale] Known
		// + active marker; the locale word is enUS (no session-locale
		// bridge, D7) and "Known" is the inlined LANG_KNOWN text.
		s.sendSysMessage(fmt.Sprintf("%d (idx:%d) - [%s enUS] Known%s", id, maskID, title, active))
	}
	return true
}

// handleCmdTitles dispatches the "titles" root (cs_titles.cpp:49-52).
func (s *session) handleCmdTitles(ctx context.Context, args []string) bool {
	if len(args) == 0 {
		// Dead root perm 761: the "titles" root is a pure SubCommandEntry
		// container (deprecated 6-arg overload) with no handler, so the
		// framework shows the help listing.
		return false
	}
	sub := strings.ToLower(args[0])
	rest := args[1:]
	// The Trinity parser prefix-matches command names at every nesting
	// level; match the arm the same way here.
	switch {
	case strings.HasPrefix("add", sub):
		return s.handleTitlesAdd(ctx, rest)
	case strings.HasPrefix("current", sub):
		return s.handleTitlesCurrent(ctx, rest)
	case strings.HasPrefix("remove", sub):
		return s.handleTitlesRemove(ctx, rest)
	case strings.HasPrefix("set", sub):
		return s.handleTitlesSet(ctx, rest)
	default:
		return false
	}
}

// titlesEntry loads and validates the CharTitles entry (LANG_INVALID_TITLE_ID 352).
func (s *session) titlesEntry(id uint32) (name, name1 string, maskID uint32, ok bool) {
	if s.server.Data == nil {
		return "", "", 0, false
	}
	title, found, err := s.server.Data.CharTitle(id)
	if err != nil || !found {
		return "", "", 0, false
	}
	return title.Name, title.Name1, title.MaskID, true
}

// handleTitlesAdd mirrors HandleTitlesAddCommand (cs_titles.cpp:123).
func (s *session) handleTitlesAdd(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandTitlesAdd) {
		return false
	}
	if len(args) == 0 {
		// C++ extractKeyFromLink("") returns null -> handler false ->
		// framework prints the arm help.
		return false
	}
	id, ok := parseTitleLink(args[0])
	if !ok {
		// Wrong link type: the C++ sends LANG_WRONG_LINK_TYPE (511) and
		// the handler returns false, so the arm syntax prints as the
		// framework-help equivalent (tick-314 quest pattern).
		s.sendSysMessage("Syntax: .titles add <titleId>")
		return true
	}
	// C++ checks id <= 0 before target selection (cs_titles.cpp:131-137).
	if id <= 0 {
		s.sendSysMessage(fmt.Sprintf("Invalid title id: %d.", id)) // LANG_INVALID_TITLE_ID 352
		return true
	}
	target := s.titlesTarget()
	if target == nil {
		return true
	}
	name, name1, maskID, ok := s.titlesEntry(uint32(id))
	if !ok {
		s.sendSysMessage(fmt.Sprintf("Invalid title id: %d.", id)) // LANG_INVALID_TITLE_ID 352
		return true
	}
	wasKnown := target.knowsTitleBit(maskID)
	if int(maskID/32) < len(target.player.KnownTitles) {
		target.player.KnownTitles[maskID/32] |= uint32(1) << (maskID % 32)
	}
	if !wasKnown && int(maskID/32) < len(target.player.KnownTitles) {
		target.sendTitleEarned(maskID, true)
	}
	s.persistTitles(ctx, target)
	s.sendSysMessage(fmt.Sprintf("Title %d (%s) added to %s.", id, titleName(target, name, name1), target.player.Name)) // LANG_TITLE_ADD_RES 353
	return true
}

// handleTitlesCurrent mirrors HandleTitlesCurrentCommand (cs_titles.cpp:84).
func (s *session) handleTitlesCurrent(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandTitlesCurrent) {
		return false
	}
	if len(args) == 0 {
		// C++ extractKeyFromLink("") returns null -> handler false ->
		// framework prints the arm help.
		return false
	}
	id, ok := parseTitleLink(args[0])
	if !ok {
		// Wrong link type: the C++ sends LANG_WRONG_LINK_TYPE (511) and
		// the handler returns false, so the arm syntax prints as the
		// framework-help equivalent (tick-314 quest pattern).
		s.sendSysMessage("Syntax: .titles current <titleId>")
		return true
	}
	// C++ checks id <= 0 before target selection (cs_titles.cpp:92-98).
	if id <= 0 {
		s.sendSysMessage(fmt.Sprintf("Invalid title id: %d.", id)) // LANG_INVALID_TITLE_ID 352
		return true
	}
	target := s.titlesTarget()
	if target == nil {
		return true
	}
	name, name1, maskID, ok := s.titlesEntry(uint32(id))
	if !ok {
		s.sendSysMessage(fmt.Sprintf("Invalid title id: %d.", id)) // LANG_INVALID_TITLE_ID 352
		return true
	}
	wasKnown := target.knowsTitleBit(maskID)
	if int(maskID/32) < len(target.player.KnownTitles) {
		target.player.KnownTitles[maskID/32] |= uint32(1) << (maskID % 32) // SetTitle: known for sure
	}
	if !wasKnown && int(maskID/32) < len(target.player.KnownTitles) {
		target.sendTitleEarned(maskID, true)
	}
	target.player.ChosenTitle = maskID
	s.persistTitles(ctx, target)
	s.sendSysMessage(fmt.Sprintf("Title %d (%s) set as current for %s.", id, titleName(target, name, name1), target.player.Name)) // LANG_TITLE_CURRENT_RES 355
	return true
}

// handleTitlesRemove mirrors HandleTitlesRemoveCommand (cs_titles.cpp:164).
func (s *session) handleTitlesRemove(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandTitlesRemove) {
		return false
	}
	if len(args) == 0 {
		// C++ extractKeyFromLink("") returns null -> handler false ->
		// framework prints the arm help.
		return false
	}
	id, ok := parseTitleLink(args[0])
	if !ok {
		// Wrong link type: the C++ sends LANG_WRONG_LINK_TYPE (511) and
		// the handler returns false, so the arm syntax prints as the
		// framework-help equivalent (tick-314 quest pattern).
		s.sendSysMessage("Syntax: .titles remove <titleId>")
		return true
	}
	// C++ checks id <= 0 before target selection (cs_titles.cpp:172-178).
	if id <= 0 {
		s.sendSysMessage(fmt.Sprintf("Invalid title id: %d.", id)) // LANG_INVALID_TITLE_ID 352
		return true
	}
	target := s.titlesTarget()
	if target == nil {
		return true
	}
	name, name1, maskID, ok := s.titlesEntry(uint32(id))
	if !ok {
		s.sendSysMessage(fmt.Sprintf("Invalid title id: %d.", id)) // LANG_INVALID_TITLE_ID 352
		return true
	}
	wasKnown := target.knowsTitleBit(maskID)
	if int(maskID/32) < len(target.player.KnownTitles) {
		target.player.KnownTitles[maskID/32] &^= uint32(1) << (maskID % 32)
	}
	if wasKnown {
		target.sendTitleEarned(maskID, false)
	}
	s.persistTitles(ctx, target)
	s.sendSysMessage(fmt.Sprintf("Title %d (%s) removed from %s.", id, titleName(target, name, name1), target.player.Name)) // LANG_TITLE_REMOVE_RES 354
	if !target.knowsTitleBit(target.player.ChosenTitle) {
		target.player.ChosenTitle = 0
		s.persistTitles(ctx, target)
		s.sendSysMessage(fmt.Sprintf("Current title reset for %s.", target.player.Name)) // LANG_CURRENT_TITLE_RESET 356
	}
	return true
}

// handleTitlesSet dispatches the "titles set" sub-table (cs_titles.cpp:45):
// only "mask".
func (s *session) handleTitlesSet(ctx context.Context, args []string) bool {
	if len(args) == 0 {
		// Dead "set"-node perm 765: bare container (deprecated 6-arg
		// overload) with no handler, so the framework shows the help
		// listing.
		return false
	}
	if !strings.HasPrefix("mask", strings.ToLower(args[0])) {
		return false
	}
	return s.handleTitlesSetMask(ctx, args[1:])
}

// handleTitlesSetMask mirrors HandleTitlesSetMaskCommand (cs_titles.cpp:215):
// the raw 64-bit mask is written to KNOWN_TITLES after stripping bits that
// belong to no CharTitles entry.
func (s *session) handleTitlesSetMask(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandTitlesSetMask) {
		return false
	}
	if len(args) == 0 {
		// C++ !*args -> handler false -> framework prints the arm help.
		return false
	}
	// sscanf(UI64FMTD) leaves titles at 0 on a parse failure and continues.
	mask := parseSScanfU64(args[0])
	target := s.titlesTarget()
	if target == nil {
		return true
	}
	// Strip bits with no CharTitles entry (cs_titles.cpp:233-236): only bits
	// present in the DBC survive.
	if s.server.Data != nil {
		if file, ferr := s.server.Data.File("CharTitles"); ferr == nil {
			var valid uint64
			for i := 0; i < file.Records(); i++ {
				rec, rerr := file.Record(i)
				if rerr != nil {
					continue
				}
				if mid, merr := rec.Uint32(36); merr == nil && mid < 64 {
					valid |= uint64(1) << mid
				}
			}
			mask &= valid
		}
	}
	for i := 0; i < 2 && i < len(target.player.KnownTitles); i++ {
		target.player.KnownTitles[i] = uint32(mask >> (uint(i) * 32))
	}
	s.persistTitles(ctx, target)
	s.sendSysMessage("Done.") // LANG_DONE 43
	if !target.knowsTitleBit(target.player.ChosenTitle) {
		target.player.ChosenTitle = 0
		s.persistTitles(ctx, target)
		s.sendSysMessage(fmt.Sprintf("Current title reset for %s.", target.player.Name)) // LANG_CURRENT_TITLE_RESET 356
	}
	return true
}
