package world

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// lookup command ports lookup_commandscript (cs_lookup.cpp), the TWENTY-SECOND
// Commands group in loader call order (AddSC_learn_commandscript() is call 84,
// this is call 85). All 19 arms search the data the Go tree actually holds:
// DBC files through server.Data, world tables through WorldStore, game_event
// through the shared GameEventMgr snapshot helpers, and account/character rows
// through AuthStore/CharactersStore.
//
// Fidelity notes (all documented gaps, not stubs): the C++ arms match against
// every DBC locale and fall back across locales; Go searches enUS only. The
// C++ faction/quest/skill/spell/title arms enrich results with the selected
// player's live state (reputation, quest status, skill values, known spells);
// Go renders the portions with a bridge and omits the rest. C++-exact item:
// HandleLookupMapIdCommand sends LANG_COMMAND_NOSPELLFOUND when the map name
// is empty — the quirk is mirrored. The console (non-session) message
// branches are moot: Go commands are always sessioned, so only chat formats
// are emitted, with LANG texts inlined from TDB enUS recall (no in-tree
// trinity_string seed), per tree convention.

// spellAttr0Passive mirrors SPELL_ATTR0_PASSIVE (SharedDefines.h:418);
// spellEffectLearnSpell is the pre-existing const in spells.go (= 36).
const spellAttr0Passive = 0x40

// spellAttr0HiddenClientside mirrors SPELL_ATTR0_HIDDEN_CLIENTSIDE
// (SharedDefines.h:419).
const spellAttr0HiddenClientside = 0x80

// lookupRecord is the DBC record surface the lookup arms read (id, name and
// arm-specific extra fields); dbc.Record implements it directly.
type lookupRecord interface {
	Uint32(field int) (uint32, error)
	String(field int) (string, error)
	Float32(field int) (float32, error)
}

// lookupMaxResultsMsg mirrors LANG_COMMAND_LOOKUP_MAX_RESULTS.
func lookupMaxResultsMsg(maxResults uint32) string {
	return fmt.Sprintf("Showing only the first %d results.", maxResults)
}

// lookupSelectedPlayer mirrors ChatHandler::getSelectedPlayer
// (cs_lookup.cpp spell/skill/title/faction arms): the selected player, nil
// only for an unresolvable selection (the C++ also allows NULL at console
// call; Go commands are always sessioned). The helper keeps its nil-on-empty
// contract for its other (lfg/list) callers, which carry their own fallbacks;
// the lookup arms use lookupSelectedPlayerOrSelf for the C++ in-session
// contract (Chat.cpp:300: no selection resolves to the invoker).
func (s *session) lookupSelectedPlayer() *session {
	if s.selection == 0 || s.server == nil {
		return nil
	}
	ts := s.server.playerSessionForGUID(s.selection)
	if ts == nil || ts.player == nil {
		return nil
	}
	return ts
}

// lookupSelectedPlayerOrSelf adapts lookupSelectedPlayer to the C++
// getSelectedPlayer contract (Chat.cpp:300): an empty selection resolves to
// the invoker, so only an unresolvable selection yields nil.
func (s *session) lookupSelectedPlayerOrSelf() *session {
	if target := s.lookupSelectedPlayer(); target != nil || s.selection != 0 {
		return target
	}
	return s
}

// lookupDBC iterates a DBC file's records in index order and sends one line
// per record whose enUS name (nameField) contains the query, honoring the
// MaxResultsLookupCommands cap exactly like the C++ arms (cs_lookup.cpp:
// `if (maxResults && count++ == maxResults)`). Locales beyond enUS are not
// searched (documented gap).
func (s *session) lookupDBC(fileName, dataName string, nameField int, query, noMatchMsg string, line func(id uint32, name string, rec lookupRecord) string) {
	if s.server == nil || s.server.Data == nil {
		s.sendSysMessage(dataName + " data is unavailable.")
		return
	}
	file, err := s.server.Data.File(fileName)
	if err != nil || file == nil {
		s.sendSysMessage(dataName + " data is unavailable.")
		return
	}
	namePart := strings.ToLower(query)
	maxResults := s.server.Config.MaxResultsLookupCommands
	var count uint32
	found := false
	for i := 0; i < file.Records(); i++ {
		rec, err := file.Record(i)
		if err != nil {
			continue
		}
		id, err := rec.Uint32(0)
		if err != nil || id == 0 {
			continue
		}
		name, _ := rec.String(nameField)
		if name == "" {
			continue
		}
		if !strings.Contains(strings.ToLower(name), namePart) {
			continue
		}
		if maxResults != 0 && count == maxResults {
			s.sendSysMessage(lookupMaxResultsMsg(maxResults))
			return
		}
		count++
		s.sendSysMessage(line(id, name, rec))
		found = true
	}
	if !found {
		s.sendSysMessage(noMatchMsg)
	}
}

// lookupWorldTable ports the C++ item_template/creature_template/
// gameobject_template name searches (cs_lookup.cpp:404, 165, 577): a full
// ordered scan of the world table with a case-insensitive substring match on
// the enUS name column (item/gameobject locales are unmodeled).
func (s *session) lookupWorldTable(table, nameCol, query, noMatchMsg string, line func(entry uint32, name string) string) {
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		s.sendSysMessage("World data is unavailable.")
		return
	}
	rows, err := s.server.WorldStore.DB.QueryContext(context.Background(), "SELECT entry, "+nameCol+" FROM "+table+" ORDER BY entry")
	if err != nil {
		s.sendSysMessage("World data is unavailable.")
		return
	}
	defer rows.Close()
	namePart := strings.ToLower(query)
	maxResults := s.server.Config.MaxResultsLookupCommands
	var count uint32
	found := false
	for rows.Next() {
		var entry uint32
		var name string
		if err := rows.Scan(&entry, &name); err != nil {
			continue
		}
		if name == "" {
			continue
		}
		if !strings.Contains(strings.ToLower(name), namePart) {
			continue
		}
		if maxResults != 0 && count == maxResults {
			s.sendSysMessage(lookupMaxResultsMsg(maxResults))
			return
		}
		count++
		s.sendSysMessage(line(entry, name))
		found = true
	}
	if !found {
		s.sendSysMessage(noMatchMsg)
	}
}

// handleCmdLookup ports the lookup command table (cs_lookup.cpp:61-84): 19
// arms with Trinity per-level prefix matching, each gated on its own RBAC
// permission (ChatCommand.cpp:487 checks only the invoker leaf node).
func (s *session) handleCmdLookup(ctx context.Context, args []string) {
	const syntax = "Syntax: .lookup area|creature|event|faction|item [id|set]|object|quest [id]|player ip|account|email|skill|spell [id]|taxinode|tele|title|map [id] <name>"
	if len(args) == 0 {
		s.sendSysMessage(syntax)
		return
	}
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return
	}
	deny := func(perm uint32) bool {
		if !s.commandAllowed(ctx, perm) {
			s.sendNotification("You do not have permission to use that command.")
			return true
		}
		return false
	}
	sub := strings.ToLower(args[0])
	rest := args[1:]
	joinRest := func() string { return strings.Join(rest, " ") }
	switch {
	case strings.HasPrefix("area", sub):
		if deny(permissionCommandLookupArea) {
			return
		}
		if len(rest) == 0 {
			s.sendSysMessage("Syntax: .lookup area <name>")
			return
		}
		s.handleLookupArea(joinRest())
	case strings.HasPrefix("creature", sub):
		if deny(permissionCommandLookupCreature) {
			return
		}
		if len(rest) == 0 {
			s.sendSysMessage("Syntax: .lookup creature <name>")
			return
		}
		s.handleLookupCreature(joinRest())
	case strings.HasPrefix("event", sub):
		if deny(permissionCommandLookupEvent) {
			return
		}
		if len(rest) == 0 {
			s.sendSysMessage("Syntax: .lookup event <name>")
			return
		}
		s.handleLookupEvent(ctx, joinRest())
	case strings.HasPrefix("faction", sub):
		if deny(permissionCommandLookupFaction) {
			return
		}
		if len(rest) == 0 {
			s.sendSysMessage("Syntax: .lookup faction <name>")
			return
		}
		s.handleLookupFaction(joinRest())
	case strings.HasPrefix("item", sub):
		if len(rest) > 0 && strings.HasPrefix("id", strings.ToLower(rest[0])) {
			if deny(permissionCommandLookupItemID) {
				return
			}
			if len(rest) < 2 {
				s.sendSysMessage("Syntax: .lookup item id <id>")
				return
			}
			s.handleLookupItemID(rest[1])
			return
		}
		if len(rest) > 0 && strings.HasPrefix("set", strings.ToLower(rest[0])) {
			if deny(permissionCommandLookupItemSet) {
				return
			}
			if len(rest) < 2 {
				s.sendSysMessage("Syntax: .lookup item set <name>")
				return
			}
			s.handleLookupItemSet(strings.Join(rest[1:], " "))
			return
		}
		if deny(permissionCommandLookupItem) {
			return
		}
		if len(rest) == 0 {
			s.sendSysMessage("Syntax: .lookup item <name>")
			return
		}
		s.handleLookupItem(joinRest())
	case strings.HasPrefix("object", sub):
		if deny(permissionCommandLookupObject) {
			return
		}
		if len(rest) == 0 {
			s.sendSysMessage("Syntax: .lookup object <name>")
			return
		}
		s.handleLookupObject(joinRest())
	case strings.HasPrefix("quest", sub):
		if len(rest) > 0 && strings.HasPrefix("id", strings.ToLower(rest[0])) {
			if deny(permissionCommandLookupQuestID) {
				return
			}
			if len(rest) < 2 {
				s.sendSysMessage("Syntax: .lookup quest id <id>")
				return
			}
			s.handleLookupQuestID(ctx, rest[1])
			return
		}
		if deny(permissionCommandLookupQuest) {
			return
		}
		if len(rest) == 0 {
			s.sendSysMessage("Syntax: .lookup quest <name>")
			return
		}
		s.handleLookupQuest(ctx, joinRest())
	case strings.HasPrefix("player", sub):
		if len(rest) == 0 {
			s.sendSysMessage("Syntax: .lookup player ip|account|email <arg> [limit]")
			return
		}
		playerSub := strings.ToLower(rest[0])
		playerRest := rest[1:]
		switch {
		case strings.HasPrefix("ip", playerSub):
			if deny(permissionCommandLookupPlayerIP) {
				return
			}
			s.handleLookupPlayerIP(ctx, playerRest)
		case strings.HasPrefix("account", playerSub):
			if deny(permissionCommandLookupPlayerAccount) {
				return
			}
			s.handleLookupPlayerAccount(ctx, playerRest)
		case strings.HasPrefix("email", playerSub):
			if deny(permissionCommandLookupPlayerEmail) {
				return
			}
			s.handleLookupPlayerEmail(ctx, playerRest)
		default:
			s.sendSysMessage("Syntax: .lookup player ip|account|email <arg> [limit]")
		}
	case strings.HasPrefix("skill", sub):
		if deny(permissionCommandLookupSkill) {
			return
		}
		if len(rest) == 0 {
			s.sendSysMessage("Syntax: .lookup skill <name>")
			return
		}
		s.handleLookupSkill(joinRest())
	case strings.HasPrefix("spell", sub):
		if len(rest) > 0 && strings.HasPrefix("id", strings.ToLower(rest[0])) {
			if deny(permissionCommandLookupSpellID) {
				return
			}
			if len(rest) < 2 {
				s.sendSysMessage("Syntax: .lookup spell id <id>")
				return
			}
			s.handleLookupSpellID(ctx, rest[1])
			return
		}
		if deny(permissionCommandLookupSpell) {
			return
		}
		if len(rest) == 0 {
			s.sendSysMessage("Syntax: .lookup spell <name>")
			return
		}
		s.handleLookupSpell(joinRest())
	case strings.HasPrefix("taxinode", sub):
		if deny(permissionCommandLookupTaxinode) {
			return
		}
		if len(rest) == 0 {
			s.sendSysMessage("Syntax: .lookup taxinode <name>")
			return
		}
		s.handleLookupTaxinode(joinRest())
	case strings.HasPrefix("tele", sub):
		if deny(permissionCommandLookupTele) {
			return
		}
		if len(rest) == 0 {
			s.sendSysMessage("Syntax: .lookup tele <name>")
			return
		}
		s.handleLookupTele(ctx, rest[0])
	case strings.HasPrefix("title", sub):
		if deny(permissionCommandLookupTitle) {
			return
		}
		if len(rest) == 0 {
			s.sendSysMessage("Syntax: .lookup title <name>")
			return
		}
		s.handleLookupTitle(joinRest())
	case strings.HasPrefix("map", sub):
		if len(rest) > 0 && strings.HasPrefix("id", strings.ToLower(rest[0])) {
			if deny(permissionCommandLookupMapID) {
				return
			}
			if len(rest) < 2 {
				s.sendSysMessage("Syntax: .lookup map id <id>")
				return
			}
			s.handleLookupMapID(rest[1])
			return
		}
		if deny(permissionCommandLookupMap) {
			return
		}
		if len(rest) == 0 {
			s.sendSysMessage("Syntax: .lookup map <name>")
			return
		}
		s.handleLookupMap(joinRest())
	default:
		s.sendSysMessage(syntax)
	}
}

// handleLookupArea ports HandleLookupAreaCommand (cs_lookup.cpp:90-163):
// AreaTable.dbc search, "id - [name]" lines. Only enUS is searched.
func (s *session) handleLookupArea(query string) {
	// AreaTableEntry.Name is the enUS string at field 11 (DBCStructure.h:176,
	// wotlk.Store.Area).
	s.lookupDBC("AreaTable", "Area", 11, query, "No area found.",
		func(id uint32, name string, rec lookupRecord) string {
			return fmt.Sprintf("%d - |cffffffff|Harea:%d|h[%s enUS]|h|r", id, id, name)
		})
}

// handleLookupCreature ports HandleLookupCreatureCommand
// (cs_lookup.cpp:165-241): creature_template search, "id - [name]" lines.
func (s *session) handleLookupCreature(query string) {
	s.lookupWorldTable("creature_template", "name", query, "No creature found.",
		func(entry uint32, name string) string {
			return fmt.Sprintf("%d - |cffffffff|Hcreature_entry:%d|h[%s]|h|r", entry, entry, name)
		})
}

// handleLookupEvent ports HandleLookupEventCommand (cs_lookup.cpp:243-296):
// game_event description search, ascending id order, with the active marker
// from the cached active-event set.
func (s *session) handleLookupEvent(ctx context.Context, query string) {
	if s.server == nil {
		return
	}
	events := s.server.loadGameEventDataMap(ctx)
	active := s.server.cachedActiveGameEvents(ctx)
	ids := make([]int64, 0, len(events))
	for id := range events {
		ids = append(ids, id)
	}
	// ascending id order like the C++ index loop
	for i := 1; i < len(ids); i++ {
		for j := i; j > 0 && ids[j] < ids[j-1]; j-- {
			ids[j], ids[j-1] = ids[j-1], ids[j]
		}
	}
	namePart := strings.ToLower(query)
	maxResults := s.server.Config.MaxResultsLookupCommands
	var count uint32
	found := false
	for _, id := range ids {
		descr := events[id].Description
		if descr == "" {
			continue
		}
		if !strings.Contains(strings.ToLower(descr), namePart) {
			continue
		}
		if maxResults != 0 && count == maxResults {
			s.sendSysMessage(lookupMaxResultsMsg(maxResults))
			return
		}
		count++
		activeStr := ""
		if _, ok := active[id]; ok {
			activeStr = " Active"
		}
		s.sendSysMessage(fmt.Sprintf("%d - |cffffffff|Hgameevent:%d|h[%s]|h|r%s", id, id, descr, activeStr))
		found = true
	}
	if !found {
		s.sendSysMessage("No event found.")
	}
}

// handleLookupFaction ports HandleLookupFactionCommand (cs_lookup.cpp:298-398):
// Faction.dbc search. The C++ appends the target's reputation state; the Go
// tree has no per-faction reputation bridge, so the no-reputation form is
// always shown (documented gap).
func (s *session) handleLookupFaction(query string) {
	// FactionEntry.Name is the enUS string at field 23 (DBCStructure.h:671).
	s.lookupDBC("Faction", "Faction", 23, query, "No faction found.",
		func(id uint32, name string, rec lookupRecord) string {
			return fmt.Sprintf("%d - |cffffffff|Hfaction:%d|h[%s enUS]|h|r [no reputation]", id, id, name)
		})
}

// handleLookupItem ports HandleLookupItemCommand (cs_lookup.cpp:400-476):
// item_template search, "id - [name]" lines.
func (s *session) handleLookupItem(query string) {
	s.lookupWorldTable("item_template", "name", query, "No item found.",
		func(entry uint32, name string) string {
			return fmt.Sprintf("%d - |cffffffff|Hitem:%d|h[%s]|h|r", entry, entry, name)
		})
}

// handleLookupItemID ports HandleLookupItemIdCommand (cs_lookup.cpp:478-504).
func (s *session) handleLookupItemID(arg string) {
	// C++ is uint32 id = atoi(args) (cs_lookup.cpp:482); cAtoi keeps the
	// prefix-digit/negative/whitespace behavior ParseUint would reject.
	id := uint32(cAtoi(arg))
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		s.sendSysMessage("World data is unavailable.")
		return
	}
	var name string
	if err := s.server.WorldStore.DB.QueryRowContext(context.Background(), "SELECT name FROM item_template WHERE entry = ?", id).Scan(&name); err != nil || name == "" {
		s.sendSysMessage("No item found.")
		return
	}
	s.sendSysMessage(fmt.Sprintf("%d - |cffffffff|Hitem:%d|h[%s]|h|r", id, id, name))
}

// handleLookupItemSet ports HandleLookupItemSetCommand (cs_lookup.cpp:506-575):
// ItemSet.dbc search, "id - [name]" lines.
func (s *session) handleLookupItemSet(query string) {
	// ItemSetEntry.Name is the enUS string at field 1 (DBCStructure.h:983).
	s.lookupDBC("ItemSet", "Item set", 1, query, "No item set found.",
		func(id uint32, name string, rec lookupRecord) string {
			return fmt.Sprintf("%d - |cffffffff|Hitemset:%d|h[%s]|h|r [enUS]", id, id, name)
		})
}

// handleLookupObject ports HandleLookupObjectCommand (cs_lookup.cpp:577-651):
// gameobject_template search, "id - [name]" lines.
func (s *session) handleLookupObject(query string) {
	s.lookupWorldTable("gameobject_template", "name", query, "No gameobject found.",
		func(entry uint32, name string) string {
			return fmt.Sprintf("%d - |cffffffff|Hgameobject_entry:%d|h[%s]|h|r", entry, entry, name)
		})
}

// lookupQuestStatusStr mirrors the status suffix of the C++ quest lookup arms
// (cs_lookup.cpp:653-771): Complete/Active from the target's quest status,
// Rewarded from character_queststatus_rewarded.
func (s *session) lookupQuestStatusStr(ctx context.Context, target *session, questID uint32) string {
	status, err := target.characterQuestStatus(ctx, questID)
	if err != nil {
		return ""
	}
	switch status {
	case questStatusComplete:
		return " Complete"
	case questStatusIncomplete:
		return " Active"
	}
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return ""
	}
	var rewarded int64
	if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT COUNT(1) FROM character_queststatus_rewarded WHERE guid = ? AND quest = ?", target.playerGUID, questID).Scan(&rewarded); err == nil && rewarded > 0 {
		return " Rewarded"
	}
	return ""
}

// handleLookupQuest ports HandleLookupQuestCommand (cs_lookup.cpp:653-771):
// quest_template title search with the selected-or-self target's quest
// status. Quest locales are unmodeled (enUS Title only).
func (s *session) handleLookupQuest(ctx context.Context, query string) {
	// getSelectedPlayerOrSelf (Chat.cpp:344): no selection or an unresolvable
	// selection both fall back to the invoker; never nil in-session.
	target := s.lookupSelectedPlayerOrSelf()
	if target == nil {
		target = s
	}
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		s.sendSysMessage("World data is unavailable.")
		return
	}
	rows, err := s.server.WorldStore.DB.QueryContext(ctx, "SELECT entry, Title, QuestLevel FROM quest_template ORDER BY entry")
	if err != nil {
		s.sendSysMessage("World data is unavailable.")
		return
	}
	defer rows.Close()
	namePart := strings.ToLower(query)
	maxResults := s.server.Config.MaxResultsLookupCommands
	var count uint32
	found := false
	for rows.Next() {
		var entry, level uint32
		var title string
		if err := rows.Scan(&entry, &title, &level); err != nil {
			continue
		}
		if title == "" {
			continue
		}
		if !strings.Contains(strings.ToLower(title), namePart) {
			continue
		}
		if maxResults != 0 && count == maxResults {
			s.sendSysMessage(lookupMaxResultsMsg(maxResults))
			return
		}
		count++
		s.sendSysMessage(fmt.Sprintf("%d - |cffffffff|Hquest:%d:%d|h[%s]|h|r%s", entry, entry, level, title, s.lookupQuestStatusStr(ctx, target, entry)))
		found = true
	}
	if !found {
		s.sendSysMessage("No quest found.")
	}
}

// handleLookupQuestID ports HandleLookupQuestIdCommand (cs_lookup.cpp:773-821).
func (s *session) handleLookupQuestID(ctx context.Context, arg string) {
	// getSelectedPlayerOrSelf (Chat.cpp:344): no selection or an unresolvable
	// selection both fall back to the invoker; never nil in-session.
	target := s.lookupSelectedPlayerOrSelf()
	if target == nil {
		target = s
	}
	// C++ is uint32 id = atoi(args) (cs_lookup.cpp:777); cAtoi keeps the
	// prefix-digit/negative/whitespace behavior ParseUint would reject.
	id := uint32(cAtoi(arg))
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		s.sendSysMessage("World data is unavailable.")
		return
	}
	var title string
	var level uint32
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT Title, QuestLevel FROM quest_template WHERE entry = ?", id).Scan(&title, &level); err != nil || title == "" {
		s.sendSysMessage("No quest found.")
		return
	}
	s.sendSysMessage(fmt.Sprintf("%d - |cffffffff|Hquest:%d:%d|h[%s]|h|r%s", id, id, level, title, s.lookupQuestStatusStr(ctx, target, id)))
}

// handleLookupPlayerIP ports HandleLookupPlayerIpCommand
// (cs_lookup.cpp:1426-1454). With no args the selected player's session IP is
// used (the selection is required and must not be the invoker, exactly like
// the C++); otherwise the first arg is the IP and the optional second the
// character row limit.
func (s *session) handleLookupPlayerIP(ctx context.Context, args []string) {
	if len(args) == 0 {
		if s.selection == 0 || s.server == nil {
			s.sendSysMessage("Syntax: .lookup player ip [<ip> [limit]]")
			return
		}
		ts := s.server.playerSessionForGUID(s.selection)
		if ts == nil || ts.player == nil || ts.playerGUID == s.playerGUID {
			s.sendSysMessage("Syntax: .lookup player ip [<ip> [limit]]")
			return
		}
		s.lookupPlayerSearchCommand(ctx, "SELECT id, username FROM account WHERE last_ip = ?", []any{ts.remoteIP()}, -1)
		return
	}
	limit := -1
	if len(args) > 1 {
		if l, err := strconv.Atoi(args[1]); err == nil {
			limit = l
		}
	}
	s.lookupPlayerSearchCommand(ctx, "SELECT id, username FROM account WHERE last_ip = ?", []any{args[0]}, limit)
}

// handleLookupPlayerAccount ports HandleLookupPlayerAccountCommand
// (cs_lookup.cpp:1456-1474): account name lookup (upper-cased like the C++
// Utf8ToUpperOnlyLatin).
func (s *session) handleLookupPlayerAccount(ctx context.Context, args []string) {
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .lookup player account <account> [limit]")
		return
	}
	limit := -1
	if len(args) > 1 {
		if l, err := strconv.Atoi(args[1]); err == nil {
			limit = l
		}
	}
	s.lookupPlayerSearchCommand(ctx, "SELECT id, username FROM account WHERE username = ?", []any{strings.ToUpper(args[0])}, limit)
}

// handleLookupPlayerEmail ports HandleLookupPlayerEmailCommand
// (cs_lookup.cpp:1476-1494).
func (s *session) handleLookupPlayerEmail(ctx context.Context, args []string) {
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .lookup player email <email> [limit]")
		return
	}
	limit := -1
	if len(args) > 1 {
		if l, err := strconv.Atoi(args[1]); err == nil {
			limit = l
		}
	}
	s.lookupPlayerSearchCommand(ctx, "SELECT id, username FROM account WHERE email = ?", []any{args[0]}, limit)
}

// lookupPlayerSearchCommand ports LookupPlayerSearchCommand
// (cs_lookup.cpp:1496-1545): for each matching login account, the account line
// then one line per character (CHAR_SEL_CHAR_GUID_NAME_BY_ACC). A zero
// character count reports "No players found."
func (s *session) lookupPlayerSearchCommand(ctx context.Context, accountQuery string, accountArgs []any, limit int) {
	if s.server == nil || s.server.AuthStore == nil || s.server.AuthStore.DB == nil ||
		s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		s.sendSysMessage("No players found.")
		return
	}
	accountRows, err := s.server.AuthStore.DB.QueryContext(ctx, accountQuery, accountArgs...)
	if err != nil {
		s.sendSysMessage("No players found.")
		return
	}
	defer accountRows.Close()
	maxResults := s.server.Config.MaxResultsLookupCommands
	var count uint32
	counter := 0
	for accountRows.Next() {
		if maxResults != 0 && count == maxResults {
			s.sendSysMessage(lookupMaxResultsMsg(maxResults))
			return
		}
		count++
		var accountID uint32
		var accountName string
		if err := accountRows.Scan(&accountID, &accountName); err != nil {
			continue
		}
		charRows, err := s.server.CharactersStore.DB.QueryContext(ctx, "SELECT guid, name, online FROM characters WHERE account = ?", accountID)
		if err != nil {
			continue
		}
		s.sendSysMessage(fmt.Sprintf("Account: %s (ID: %d)", accountName, accountID))
		for charRows.Next() {
			if limit != -1 && counter >= limit {
				break
			}
			var guid uint32
			var name string
			var online int64
			if err := charRows.Scan(&guid, &name, &online); err != nil {
				continue
			}
			onlineStr := ""
			if online != 0 {
				onlineStr = " Online"
			}
			s.sendSysMessage(fmt.Sprintf(" %s (GUID: %d)%s", name, guid, onlineStr))
			counter++
		}
		charRows.Close()
	}
	if counter == 0 {
		s.sendSysMessage("No players found.")
	}
}

// handleLookupSkill ports HandleLookupSkillCommand (cs_lookup.cpp:823-909):
// SkillLine.dbc search. The C++ shows the target's skill values when the
// target knows the skill; the Go tree has no skill-value bridge, so only the
// name lines are emitted (documented gap).
func (s *session) handleLookupSkill(query string) {
	// SkillLineEntry.DisplayName is the enUS string at field 3
	// (DBCStructure.h:1316).
	s.lookupDBC("SkillLine", "Skill", 3, query, "No skill found.",
		func(id uint32, name string, rec lookupRecord) string {
			return fmt.Sprintf("%d - |cffffffff|Hskill:%d|h[%s enUS]|h|r", id, id, name)
		})
}

// parseSpellRank parses the DBC rank subtext ("Rank 3") into the rank number
// the C++ SpellInfo::GetRank extracts.
func parseSpellRank(rankText string) uint32 {
	fields := strings.Fields(rankText)
	if len(fields) == 0 {
		return 0
	}
	rank, err := strconv.ParseUint(fields[len(fields)-1], 10, 32)
	if err != nil {
		return 0
	}
	return uint32(rank)
}

// lookupSpellLine renders one spell lookup result in the C++
// "id - [name, rank N] [talent] [passive] [learn] [known] [active]" format
// (cs_lookup.cpp:911-1021). The talent mark is omitted: GetTalentSpellCost has
// no Go model (documented gap, same as the learn port).
func (s *session) lookupSpellLine(id uint32, name, rankText string, target *session) string {
	line := fmt.Sprintf("%d - |cffffffff|Hspell:%d|h[%s", id, id, name)
	rank := parseSpellRank(rankText)
	learn := false
	passive := false
	if s.server != nil && s.server.Data != nil {
		if info, found, err := s.server.Data.Spell(id); err == nil && found && len(info.Effects) > 0 {
			passive = info.AttributesEx&spellAttr0Passive != 0
			if info.Effects[0].Effect == spellEffectLearnSpell {
				learn = true
				if trigName, trigRank, trigFound, trigErr := s.server.Data.SpellName(info.Effects[0].TriggerSpell); trigErr == nil && trigFound && trigName != "" {
					rank = parseSpellRank(trigRank)
				}
			}
		}
	}
	if rank > 0 {
		line += fmt.Sprintf(", Rank %d", rank)
	}
	line += " enUS]|h|r"
	if passive {
		line += " Passive"
	}
	if learn {
		line += " Learn"
	}
	if target != nil && target.player != nil && playerHasSpell(target.player, id) {
		line += " Known"
	}
	if target != nil && target.playerHasAura(id) {
		line += " Active"
	}
	return line
}

// handleLookupSpell ports HandleLookupSpellCommand (cs_lookup.cpp:911-1021):
// Spell.dbc name search. The known/active marks use the selected player; the
// C++ also allows NULL (console), so a nil selection simply omits them.
func (s *session) handleLookupSpell(query string) {
	target := s.lookupSelectedPlayerOrSelf()
	// SpellName is the enUS string at field 136 (wotlk.Store.SpellName).
	s.lookupDBC("Spell", "Spell", 136, query, "No spell found.",
		func(id uint32, name string, rec lookupRecord) string {
			rankText, _ := rec.String(153)
			return s.lookupSpellLine(id, name, rankText, target)
		})
}

// handleLookupSpellID ports HandleLookupSpellIdCommand (cs_lookup.cpp:1023-1091).
func (s *session) handleLookupSpellID(ctx context.Context, arg string) {
	// C++ is uint32 id = atoi(args) (cs_lookup.cpp:1027); cAtoi keeps the
	// prefix-digit/negative/whitespace behavior ParseUint would reject.
	id := uint32(cAtoi(arg))
	if s.server == nil || s.server.Data == nil {
		s.sendSysMessage("Spell data is unavailable.")
		return
	}
	name, rankText, found, err := s.server.Data.SpellName(id)
	if err != nil || !found || name == "" {
		s.sendSysMessage("No spell found.")
		return
	}
	s.sendSysMessage(s.lookupSpellLine(id, name, rankText, s.lookupSelectedPlayerOrSelf()))
}

// handleLookupTaxinode ports HandleLookupTaxiNodeCommand (cs_lookup.cpp:1093-1165):
// TaxiNodes.dbc search, "id - [name] (Map:m X:x Y:y Z:z)" lines.
func (s *session) handleLookupTaxinode(query string) {
	// TaxiNodesEntry.Name is the enUS string at field 5 (DBCStructure.h:1698).
	s.lookupDBC("TaxiNodes", "Taxi node", 5, query, "No taxinode found.",
		func(id uint32, name string, rec lookupRecord) string {
			continent, _ := rec.Uint32(1)
			x, _ := rec.Float32(2)
			y, _ := rec.Float32(3)
			z, _ := rec.Float32(4)
			return fmt.Sprintf("%d - |cffffffff|Htaxinode:%d|h[%s]|h|r [enUS] (Map:%d X:%f Y:%f Z:%f)", id, id, name, continent, x, y, z)
		})
}

// handleLookupTele ports HandleLookupTeleCommand (cs_lookup.cpp:1167-1223):
// game_tele search on the first token only, name-ordered, with the lookup
// result cap applied like the C++ limitReached path. The C++ matches against
// the pre-lowercased in-memory map; the Go query is case-insensitive for
// ASCII through the DB LIKE, which is the faithful available equivalent.
func (s *session) handleLookupTele(ctx context.Context, token string) {
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		s.sendSysMessage("World data is unavailable.")
		return
	}
	rows, err := s.server.WorldStore.DB.QueryContext(ctx, "SELECT id, name FROM game_tele WHERE name LIKE ? ORDER BY name", "%"+token+"%")
	if err != nil {
		s.sendSysMessage("No location found.")
		return
	}
	defer rows.Close()
	maxResults := s.server.Config.MaxResultsLookupCommands
	var count uint32
	limitReached := false
	var reply strings.Builder
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			continue
		}
		if maxResults != 0 && count == maxResults {
			limitReached = true
			break
		}
		count++
		// The C++ link target is the tele id (itr->first of the
		// unordered_map<uint32, GameTele>); |Htele:id| links resolve by id.
		fmt.Fprintf(&reply, "  |cffffffff|Htele:%d|h[%s]|h|r\n", id, name)
	}
	if reply.Len() == 0 {
		s.sendSysMessage("No location found.")
		return
	}
	s.sendSysMessage(reply.String())
	if limitReached {
		s.sendSysMessage(lookupMaxResultsMsg(maxResults))
	}
}

// handleLookupTitle ports HandleLookupTitleCommand (cs_lookup.cpp:1225-1308):
// CharTitles.dbc search, "id (idx:idx) - [name]" lines with the known/active
// marks from the selected player's titles (female name variant unmodeled,
// like the C++ @todo).
func (s *session) handleLookupTitle(query string) {
	target := s.lookupSelectedPlayerOrSelf()
	targetName := "NAME"
	if target != nil && target.player != nil {
		targetName = target.player.Name
	}
	// CharTitlesEntry.Name is the enUS string at field 2, MaskID at field 36
	// (wotlk.Store.CharTitle).
	s.lookupDBC("CharTitles", "Title", 2, query, "No title found.",
		func(id uint32, name string, rec lookupRecord) string {
			maskID, _ := rec.Uint32(36)
			titleName := strings.Replace(name, "%s", targetName, -1)
			knownStr := ""
			activeStr := ""
			if target != nil {
				if target.playerHasTitle(maskID) {
					knownStr = " Known"
				}
				if target.player != nil && target.player.ChosenTitle == maskID {
					activeStr = " Active"
				}
			}
			return fmt.Sprintf("%d (idx:%d) - |cffffffff|Htitle:%d|h[%s]|h|r [enUS]%s%s", id, maskID, id, titleName, knownStr, activeStr)
		})
}

// lookupMapLine renders the C++ "id - [name]" map line with the continent and
// instance-type suffixes (cs_lookup.cpp:1310-1424). MapEntry::IsContinent is
// ID 0/1/530/571 (DBCStructure.h:1117-1120).
func lookupMapLine(id uint32, name string, instType uint32) string {
	line := fmt.Sprintf("%d - [%s]", id, name)
	if id == 0 || id == 1 || id == 530 || id == 571 {
		line += " Continent"
	}
	switch instType {
	case 1:
		line += " Instance"
	case 2:
		line += " Raid"
	case 3:
		line += " Battleground"
	case 4:
		line += " Arena"
	}
	return line
}

// handleLookupMap ports HandleLookupMapCommand (cs_lookup.cpp:1310-1377):
// Map.dbc name search.
func (s *session) handleLookupMap(query string) {
	// MapEntry.MapName is the enUS string at field 5, InstanceType at field 2
	// (wotlk.Store.Map).
	s.lookupDBC("Map", "Map", 5, query, "No map found.",
		func(id uint32, name string, rec lookupRecord) string {
			instType, _ := rec.Uint32(2)
			return lookupMapLine(id, name, instType)
		})
}

// handleLookupMapID ports HandleLookupMapIdCommand (cs_lookup.cpp:1379-1424).
// The C++ sends LANG_COMMAND_NOSPELLFOUND when the entry exists but its name
// is empty (cs_lookup.cpp:1408); the quirk is mirrored.
func (s *session) handleLookupMapID(arg string) {
	// C++ is uint32 id = atoi(args) (cs_lookup.cpp:1383); cAtoi keeps the
	// prefix-digit/negative/whitespace behavior ParseUint would reject.
	id := uint32(cAtoi(arg))
	if s.server == nil || s.server.Data == nil {
		s.sendSysMessage("Map data is unavailable.")
		return
	}
	info, found, err := s.server.Data.Map(id)
	if err != nil || !found {
		s.sendSysMessage("No map found.")
		return
	}
	if info.MapName == "" {
		s.sendSysMessage("No spell found.")
		return
	}
	s.sendSysMessage(lookupMapLine(id, info.MapName, info.InstanceType))
}
