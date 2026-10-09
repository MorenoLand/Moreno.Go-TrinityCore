package world

import (
	"context"
	"strconv"
	"strings"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

// Chat link color constants (SharedDefines.h: ChatLinkColors).
const (
	chatLinkColorTrade       uint32 = 0xffffd000
	chatLinkColorTalent      uint32 = 0xff4e96f7
	chatLinkColorSpell       uint32 = 0xff71d5ff
	chatLinkColorEnchant     uint32 = 0xffffd000
	chatLinkColorAchievement uint32 = 0xffffff00
	chatLinkColorGlyph       uint32 = 0xff66bbff
)

// ItemQualityColors (SharedDefines.h:384) lives in commands_list.go as
// itemQualityColors and is reused by the item link validator.
// QuestDifficultyColors (SharedDefines.h:397), used by the quest link
// validator.
var questDifficultyColors = [5]uint32{
	0xff40c040,
	0xff808080,
	0xffffff00,
	0xffff8040,
	0xffff2020,
}

// hyperlinkInfo mirrors Trinity::Hyperlinks::HyperlinkInfo (Hyperlinks.h).
type hyperlinkInfo struct {
	ok    bool
	tail  string
	color uint32
	tag   string
	data  string
	text  string
}

// hyperlinkHexValue mirrors the toHex helper in Hyperlinks.cpp: only
// lowercase hex digits are valid; '0' yields 0x10 so the digit still
// contributes a zero nibble.
func hyperlinkHexValue(c byte) uint8 {
	switch {
	case c >= '0' && c <= '9':
		return c - '0' + 0x10
	case c >= 'a' && c <= 'f':
		return c - 'a' + 0x1a
	}
	return 0x00
}

// parseSingleHyperlink mirrors Trinity::Hyperlinks::ParseSingleHyperlink
// (Hyperlinks.cpp): |c<8 hex AARRGGBB>|H<tag>[:<data>]|h[<text>]|h|r.
func parseSingleHyperlink(str string) hyperlinkInfo {
	invalid := hyperlinkInfo{}
	if len(str) < 2 || str[0] != '|' || str[1] != 'c' {
		return invalid
	}
	str = str[2:]
	if len(str) < 8 {
		return invalid
	}
	var color uint32
	for i := 0; i < 8; i++ {
		hex := hyperlinkHexValue(str[i])
		if hex == 0 {
			return invalid
		}
		color = (color << 4) | uint32(hex&0xf)
	}
	str = str[8:]
	if len(str) < 2 || str[0] != '|' || str[1] != 'H' {
		return invalid
	}
	str = str[2:]
	delim := strings.IndexByte(str, '|')
	if delim < 0 {
		return invalid
	}
	tag := str[:delim]
	str = str[delim+1:]
	data := ""
	if i := strings.IndexByte(tag, ':'); i >= 0 {
		data = tag[i+1:]
		tag = tag[:i]
	}
	if len(str) < 1 || str[0] != 'h' {
		return invalid
	}
	str = str[1:]
	end := strings.IndexByte(str, '|')
	if end < 0 || len(str) < end+4 || str[end:end+4] != "|h|r" {
		return invalid
	}
	if end < 2 || str[0] != '[' || str[end-1] != ']' {
		return invalid
	}
	text := str[1 : end-1]
	tail := str[end+4:]
	return hyperlinkInfo{ok: true, tail: tail, color: color, tag: tag, data: data, text: text}
}

// checkAllHyperlinks mirrors Trinity::Hyperlinks::CheckAllLinks
// (Hyperlinks.cpp:367): step 1 rejects any control sequence other than
// ||, |H, |h, |c, |r; step 2 parses every link and runs validate on it.
func checkAllHyperlinks(str string, validate func(hyperlinkInfo) bool) bool {
	for pos := 0; pos < len(str); {
		i := strings.IndexByte(str[pos:], '|')
		if i < 0 {
			break
		}
		pos += i + 1
		if pos >= len(str) {
			return false
		}
		switch str[pos] {
		case 'H', 'h', 'c', 'r', '|':
			pos++
		default:
			return false
		}
	}
	for {
		pos := strings.IndexByte(str, '|')
		if pos < 0 {
			break
		}
		if pos+1 < len(str) && str[pos+1] == '|' {
			str = str[pos+2:]
			continue
		}
		info := parseSingleHyperlink(str[pos:])
		if !info.ok || !validate(info) {
			return false
		}
		str = info.tail
	}
	return true
}

// hyperlinkDataTokenizer mirrors the HyperlinkDataTokenizer in
// HyperlinkTags.cpp: ':'-separated tokens, empty remainder fails.
type hyperlinkDataTokenizer struct {
	str string
}

func (t *hyperlinkDataTokenizer) next() (string, bool) {
	if t.str == "" {
		return "", false
	}
	if i := strings.IndexByte(t.str, ':'); i >= 0 {
		tok := t.str[:i]
		t.str = t.str[i+1:]
		return tok, true
	}
	tok := t.str
	t.str = ""
	return tok, true
}

func (t *hyperlinkDataTokenizer) empty() bool { return t.str == "" }

func hyperlinkParseUint(tok string, bits int) (uint64, bool) {
	v, err := strconv.ParseUint(tok, 10, bits)
	if err != nil {
		return 0, false
	}
	return v, true
}

func hyperlinkParseInt(tok string, bits int) (int64, bool) {
	v, err := strconv.ParseInt(tok, 10, bits)
	if err != nil {
		return 0, false
	}
	return v, true
}

// hyperlinkParseBool mirrors Trinity's StringTo<bool> (StringConvert.h:146):
// 1/y/on/yes/true and 0/n/off/no/false, case-insensitive.
func hyperlinkParseBool(tok string) (bool, bool) {
	switch strings.ToLower(tok) {
	case "1", "y", "on", "yes", "true":
		return true, true
	case "0", "n", "off", "no", "false":
		return false, true
	}
	return false, false
}

func hyperlinkParseHex64(tok string) (uint64, bool) {
	v, err := strconv.ParseUint(tok, 16, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// validateHyperlinkInfo mirrors the ValidateLinkInfo/TryValidateAs dispatch
// in Hyperlinks.cpp plus the severity gating in ValidateAs: severity >= 0
// enforces color, severity >= 1 additionally enforces text. Tags without a
// dedicated validator (the base_tag list) only need their data to parse.
func (s *session) validateHyperlinkInfo(ctx context.Context, info hyperlinkInfo) bool {
	severity := 0
	if s.server != nil {
		severity = s.server.Config.ChatStrictLinkCheckingSeverity
	}
	t := &hyperlinkDataTokenizer{str: info.data}
	var colorOK, textOK func() bool
	switch info.tag {
	case "achievement":
		d, ok := s.parseAchievementLinkData(t)
		if !ok || !t.empty() {
			return false
		}
		colorOK = func() bool { return info.color == chatLinkColorAchievement }
		textOK = func() bool { return info.text != "" && info.text == d.title }
	case "enchant":
		spellID, name, ok := s.parseEnchantLinkData(t)
		if !ok || !t.empty() {
			return false
		}
		colorOK = func() bool { return info.color == chatLinkColorEnchant }
		textOK = func() bool { return s.enchantLinkTextValid(spellID, name, info.text) }
	case "glyph":
		spellID, ok := s.parseGlyphLinkData(t)
		if !ok || !t.empty() {
			return false
		}
		colorOK = func() bool { return info.color == chatLinkColorGlyph }
		textOK = func() bool {
			store := s.hyperlinkStore()
			if store == nil {
				return false
			}
			name, _, found, err := store.SpellName(spellID)
			return err == nil && found && info.text == name
		}
	case "item":
		d, ok := s.parseItemLinkData(ctx, t)
		if !ok || !t.empty() {
			return false
		}
		colorOK = func() bool {
			return d.quality < uint32(len(itemQualityColors)) && info.color == itemQualityColors[d.quality]
		}
		textOK = func() bool { return s.itemLinkTextValid(d, info.text) }
	case "quest":
		d, ok := s.parseQuestLinkData(ctx, t)
		if !ok || !t.empty() {
			return false
		}
		colorOK = func() bool {
			for _, c := range questDifficultyColors {
				if info.color == c {
					return true
				}
			}
			return false
		}
		textOK = func() bool { return info.text != "" && info.text == d.title }
	case "spell":
		name, ok := s.parseSpellLinkData(t)
		if !ok || !t.empty() {
			return false
		}
		colorOK = func() bool { return info.color == chatLinkColorSpell }
		textOK = func() bool { return info.text == name }
	case "talent":
		name, ok := s.parseTalentLinkData(t)
		if !ok || !t.empty() {
			return false
		}
		colorOK = func() bool { return info.color == chatLinkColorTalent }
		textOK = func() bool { return info.text == name }
	case "trade":
		name, ok := s.parseTradeLinkData(t)
		if !ok || !t.empty() {
			return false
		}
		colorOK = func() bool { return info.color == chatLinkColorTrade }
		textOK = func() bool { return info.text == name }
	default:
		if !s.parseBaseLinkData(t, info.tag) {
			return false
		}
		// base_tag types have no LinkValidator specialization: both
		// IsTextValid and IsColorValid default to true.
		return true
	}
	if severity >= 0 {
		if !colorOK() {
			return false
		}
		if severity >= 1 && !textOK() {
			return false
		}
	}
	return true
}

// parseBaseLinkData mirrors the base_tag StoreTo overloads for the 13
// simple tags (Hyperlinks.h): area, areatrigger, creature, creature_entry,
// gameevent, gameobject, gameobject_entry, itemset, player, skill, taxinode,
// tele, title. Unknown tags fail like ValidateLinkInfo's trailing
// return false.
func (s *session) parseBaseLinkData(t *hyperlinkDataTokenizer, tag string) bool {
	tok, ok := t.next()
	if !ok || !t.empty() {
		return false
	}
	switch tag {
	case "area", "areatrigger", "creature_entry", "gameobject_entry", "itemset", "skill", "taxinode", "tele", "title":
		_, ok = hyperlinkParseUint(tok, 32)
		return ok
	case "gameevent":
		_, ok = hyperlinkParseUint(tok, 16)
		return ok
	case "creature", "gameobject":
		_, ok = hyperlinkParseUint(tok, 64)
		return ok
	case "player":
		return true
	}
	return false
}

// validateHyperlinksAndMaybeKick mirrors
// WorldSession::ValidateHyperlinksAndMaybeKick (WorldSession.cpp:660): an
// invalid link drops the message and, when ChatStrictLinkChecking.Kick is
// set, disconnects the sender (KickPlayer equivalent: kickSession on self,
// same as movement.go:1147).
func (s *session) validateHyperlinksAndMaybeKick(ctx context.Context, msg string) bool {
	if checkAllHyperlinks(msg, func(info hyperlinkInfo) bool {
		return s.validateHyperlinkInfo(ctx, info)
	}) {
		return true
	}
	name := ""
	if s.player != nil {
		name = s.player.Name
	}
	s.debug("chat rejected", "account", s.accountName, "character", name, "reason", "invalid hyperlink", "message", msg)
	if s.server != nil && s.server.Config.ChatStrictLinkCheckingKick {
		s.kickSession(s)
	}
	return false
}

// ---- complex tag data parsers (HyperlinkTags.cpp) ----

type achievementLinkData struct {
	title string
}

// parseAchievementLinkData mirrors LinkTags::achievement::StoreTo
// (HyperlinkTags.cpp:59).
func (s *session) parseAchievementLinkData(t *hyperlinkDataTokenizer) (achievementLinkData, bool) {
	var d achievementLinkData
	tok, ok := t.next()
	if !ok {
		return d, false
	}
	id, ok := hyperlinkParseUint(tok, 32)
	if !ok {
		return d, false
	}
	entry, found := achievementIndex.achieveByID[uint32(id)]
	if !found {
		return d, false
	}
	d.title = entry.Title
	if tok, ok = t.next(); !ok {
		return d, false
	} else if _, ok = hyperlinkParseHex64(tok); !ok {
		return d, false
	}
	if tok, ok = t.next(); !ok {
		return d, false
	} else if finished, ok := hyperlinkParseBool(tok); !ok {
		return d, false
	} else {
		if tok, ok = t.next(); !ok {
			return d, false
		} else if month, ok := hyperlinkParseUint(tok, 8); !ok || month > 12 {
			return d, false
		}
		if tok, ok = t.next(); !ok {
			return d, false
		} else if day, ok := hyperlinkParseUint(tok, 8); !ok || day > 31 {
			return d, false
		}
		if tok, ok = t.next(); !ok {
			return d, false
		} else if year, ok := hyperlinkParseInt(tok, 8); !ok {
			return d, false
		} else if finished && year < 0 {
			return d, false
		}
	}
	for i := 0; i < 4; i++ {
		if tok, ok = t.next(); !ok {
			return d, false
		} else if _, ok = hyperlinkParseUint(tok, 32); !ok {
			return d, false
		}
	}
	return d, true
}

// parseEnchantLinkData mirrors LinkTags::enchant::StoreTo
// (HyperlinkTags.cpp:88): the spell must exist and carry
// SPELL_ATTR0_TRADESPELL.
func (s *session) parseEnchantLinkData(t *hyperlinkDataTokenizer) (spellID uint32, spellName string, ok bool) {
	tok, ok := t.next()
	if !ok {
		return 0, "", false
	}
	id, ok := hyperlinkParseUint(tok, 32)
	if !ok {
		return 0, "", false
	}
	store := s.hyperlinkStore()
	if store == nil {
		return 0, "", false
	}
	spell, found, err := store.Spell(uint32(id))
	if err != nil || !found {
		return 0, "", false
	}
	if spell.Attributes&spellAttr0Tradespell == 0 {
		return 0, "", false
	}
	name, _, found, err := store.SpellName(uint32(id))
	if err != nil || !found {
		return 0, "", false
	}
	return uint32(id), name, true
}

// enchantLinkTextValid mirrors LinkValidator<LinkTags::enchant>::IsTextValid
// (Hyperlinks.cpp): the spell name, or the alternate
// "[Skill Name: Spell Name]" form over the spell's SkillLine abilities.
func (s *session) enchantLinkTextValid(spellID uint32, spellName, text string) bool {
	if text == spellName {
		return true
	}
	store := s.hyperlinkStore()
	if store == nil {
		return false
	}
	abilities, ok, err := store.SkillLineAbilities(spellID)
	if err != nil || !ok {
		return false
	}
	skillFile, err := store.File("SkillLine")
	if err != nil {
		return false
	}
	for _, ability := range abilities {
		rec, ok := skillFile.Find(ability.SkillLine)
		if !ok {
			return false
		}
		skillName, err := rec.String(3)
		if err != nil || skillName == "" {
			continue
		}
		if len(text) == len(skillName)+2+len(spellName) &&
			text[:len(skillName)] == skillName &&
			text[len(skillName):len(skillName)+2] == ": " &&
			text[len(skillName)+2:] == spellName {
			return true
		}
	}
	return false
}

// parseGlyphLinkData mirrors LinkTags::glyph::StoreTo
// (HyperlinkTags.cpp:97): slot and property must both resolve; returns the
// glyph's spell id for the text validator.
func (s *session) parseGlyphLinkData(t *hyperlinkDataTokenizer) (uint32, bool) {
	tok, ok := t.next()
	if !ok {
		return 0, false
	}
	slot, ok := hyperlinkParseUint(tok, 32)
	if !ok {
		return 0, false
	}
	if tok, ok = t.next(); !ok {
		return 0, false
	}
	prop, ok := hyperlinkParseUint(tok, 32)
	if !ok {
		return 0, false
	}
	store := s.hyperlinkStore()
	if store == nil {
		return 0, false
	}
	if file, err := store.File("GlyphSlot"); err != nil {
		return 0, false
	} else if _, ok := file.Find(uint32(slot)); !ok {
		return 0, false
	}
	propEntry, found, err := store.GlyphProperties(uint32(prop))
	if err != nil || !found {
		return 0, false
	}
	return propEntry.SpellID, true
}

// parseSpellLinkData mirrors LinkTags::spell::StoreTo
// (HyperlinkTags.cpp:183).
func (s *session) parseSpellLinkData(t *hyperlinkDataTokenizer) (string, bool) {
	tok, ok := t.next()
	if !ok {
		return "", false
	}
	id, ok := hyperlinkParseUint(tok, 32)
	if !ok {
		return "", false
	}
	store := s.hyperlinkStore()
	if store == nil {
		return "", false
	}
	name, _, found, err := store.SpellName(uint32(id))
	if err != nil || !found {
		return "", false
	}
	return name, true
}

// parseTalentLinkData mirrors LinkTags::talent::StoreTo
// (HyperlinkTags.cpp:192): rank is the learned rank minus one, stored as
// rank+1; a positive rank requires the rank's spell to exist.
func (s *session) parseTalentLinkData(t *hyperlinkDataTokenizer) (string, bool) {
	tok, ok := t.next()
	if !ok {
		return "", false
	}
	talentID, ok := hyperlinkParseUint(tok, 32)
	if !ok {
		return "", false
	}
	if tok, ok = t.next(); !ok {
		return "", false
	}
	rank, ok := hyperlinkParseInt(tok, 8)
	if !ok || rank < -1 || rank >= int64(maxTalentRank) {
		return "", false
	}
	store := s.hyperlinkStore()
	if store == nil {
		return "", false
	}
	entry, found, err := store.Talent(uint32(talentID))
	if err != nil || !found {
		return "", false
	}
	var spellID uint32
	if rank+1 > 0 {
		spellID = entry.SpellRank[rank]
		if spellID == 0 {
			return "", false
		}
	} else {
		spellID = entry.SpellRank[0]
	}
	name, _, found, err := store.SpellName(spellID)
	if err != nil || !found {
		return "", false
	}
	return name, true
}

// parseTradeLinkData mirrors LinkTags::trade::StoreTo
// (HyperlinkTags.cpp:221): the spell must exist with a TRADE_SKILL first
// effect.
func (s *session) parseTradeLinkData(t *hyperlinkDataTokenizer) (string, bool) {
	tok, ok := t.next()
	if !ok {
		return "", false
	}
	spellID, ok := hyperlinkParseUint(tok, 32)
	if !ok {
		return "", false
	}
	store := s.hyperlinkStore()
	if store == nil {
		return "", false
	}
	spell, found, err := store.Spell(uint32(spellID))
	if err != nil || !found {
		return "", false
	}
	if spell.Effects[0].Effect != spellEffectTradeSkill {
		return "", false
	}
	if tok, ok = t.next(); !ok {
		return "", false
	} else if _, ok = hyperlinkParseUint(tok, 16); !ok {
		return "", false
	}
	if tok, ok = t.next(); !ok {
		return "", false
	} else if _, ok = hyperlinkParseUint(tok, 16); !ok {
		return "", false
	}
	if tok, ok = t.next(); !ok {
		return "", false
	} else if _, ok = hyperlinkParseHex64(tok); !ok {
		return "", false
	}
	if _, ok = t.next(); !ok {
		return "", false
	}
	name, _, found, err := store.SpellName(uint32(spellID))
	if err != nil || !found {
		return "", false
	}
	return name, true
}

// hyperlinkStore returns the DBC store used by the link validators.
func (s *session) hyperlinkStore() *wotlk.Store {
	if s == nil || s.server == nil {
		return nil
	}
	return s.server.Data
}

// itemHyperlinkTemplate mirrors the ItemTemplate fields the item link
// validator needs (HyperlinkTags.cpp:110): display name, quality, and the
// random-property/suffix flags.
type itemHyperlinkTemplate struct {
	name           string
	quality        uint32
	randomProperty uint32
	randomSuffix   uint32
}

// itemHyperlinkTemplateByEntry loads and caches the item_template row for
// link validation, mirroring getItemTemplateClassInfo's caching pattern.
func (sv *Server) itemHyperlinkTemplateByEntry(ctx context.Context, entry uint32) (itemHyperlinkTemplate, bool) {
	if entry == 0 {
		return itemHyperlinkTemplate{}, false
	}
	sv.itemHyperlinkMu.RLock()
	if sv.itemHyperlinkTemplates != nil {
		if info, ok := sv.itemHyperlinkTemplates[entry]; ok {
			sv.itemHyperlinkMu.RUnlock()
			return info, true
		}
	}
	sv.itemHyperlinkMu.RUnlock()
	if sv.WorldStore == nil || sv.WorldStore.DB == nil {
		return itemHyperlinkTemplate{}, false
	}
	var info itemHyperlinkTemplate
	err := sv.WorldStore.DB.QueryRowContext(ctx,
		"SELECT name, Quality, RandomProperty, RandomSuffix FROM item_template WHERE entry = ? LIMIT 1",
		entry).Scan(&info.name, &info.quality, &info.randomProperty, &info.randomSuffix)
	if err != nil {
		return itemHyperlinkTemplate{}, false
	}
	sv.itemHyperlinkMu.Lock()
	if sv.itemHyperlinkTemplates == nil {
		sv.itemHyperlinkTemplates = make(map[uint32]itemHyperlinkTemplate)
	}
	sv.itemHyperlinkTemplates[entry] = info
	sv.itemHyperlinkMu.Unlock()
	return info, true
}

type itemLinkData struct {
	name                string
	quality             uint32
	randomProperty      uint32
	randomSuffix        uint32
	randomSuffixBaseAmt uint32
	buggedInspectLink   bool
}

// parseItemLinkData mirrors LinkTags::item::StoreTo (HyperlinkTags.cpp:110),
// including the bugged-inspect-link detection: a randomPropertyId in
// (int16max, uint16max] is the client's sign-extension bug and is recast to
// int16, with the suffix-name check disabled.
func (s *session) parseItemLinkData(ctx context.Context, t *hyperlinkDataTokenizer) (itemLinkData, bool) {
	var d itemLinkData
	nextUint32 := func() (uint32, bool) {
		tok, ok := t.next()
		if !ok {
			return 0, false
		}
		v, ok := hyperlinkParseUint(tok, 32)
		return uint32(v), ok
	}
	itemID, ok := nextUint32()
	if !ok {
		return d, false
	}
	if _, ok = nextUint32(); !ok { // enchantId
		return d, false
	}
	for i := 0; i < 3; i++ {
		if _, ok = nextUint32(); !ok { // gem enchant ids
			return d, false
		}
	}
	dummy, ok := nextUint32()
	if !ok || dummy != 0 {
		return d, false
	}
	tok, ok := t.next()
	if !ok {
		return d, false
	}
	randomPropertyID, ok := hyperlinkParseInt(tok, 32)
	if !ok {
		return d, false
	}
	baseAmt, ok := nextUint32()
	if !ok {
		return d, false
	}
	if _, ok = nextUint32(); !ok { // renderLevel
		return d, false
	}
	if !t.empty() {
		return d, false
	}
	if s.server == nil {
		return d, false
	}
	info, ok := s.server.itemHyperlinkTemplateByEntry(ctx, uint32(itemID))
	if !ok {
		return d, false
	}
	d.name = info.name
	d.quality = info.quality
	d.randomSuffixBaseAmt = uint32(baseAmt)
	if randomPropertyID > 32767 && randomPropertyID <= 65535 {
		randomPropertyID = int64(int16(randomPropertyID))
		d.buggedInspectLink = true
	}
	store := s.hyperlinkStore()
	if randomPropertyID < 0 {
		if info.randomSuffix == 0 {
			return d, false
		}
		if store != nil {
			if file, err := store.File("ItemRandomSuffix"); err == nil && randomPropertyID < -int64(file.Records()) {
				return d, false
			}
		}
		if store == nil {
			return d, false
		}
		if _, found, err := store.ItemRandomSuffix(uint32(-randomPropertyID)); err != nil || !found {
			return d, false
		}
		d.randomSuffix = uint32(-randomPropertyID)
	} else if randomPropertyID > 0 {
		if info.randomProperty == 0 {
			return d, false
		}
		if store == nil {
			return d, false
		}
		if _, found, err := store.ItemRandomProperties(uint32(randomPropertyID)); err != nil || !found {
			return d, false
		}
		d.randomProperty = uint32(randomPropertyID)
	}
	if (d.randomSuffix != 0 && d.randomSuffixBaseAmt == 0) || (d.randomSuffixBaseAmt != 0 && d.randomSuffix == 0) {
		return d, false
	}
	return d, true
}

// itemLinkTextValid mirrors LinkValidator<LinkTags::item>::IsTextValid
// (Hyperlinks.cpp): the item name, optionally followed by " <random
// suffix/property name>". A bugged inspect link drops the suffix arm.
func (s *session) itemLinkTextValid(d itemLinkData, text string) bool {
	suffixName := ""
	if !d.buggedInspectLink {
		if d.randomProperty != 0 {
			suffixName = s.itemRandomPropertyName(d.randomProperty)
		} else if d.randomSuffix != 0 {
			suffixName = s.itemRandomSuffixName(d.randomSuffix)
		}
	}
	if suffixName != "" {
		return len(text) == len(d.name)+1+len(suffixName) &&
			strings.HasPrefix(text, d.name) &&
			text[len(d.name)] == ' ' &&
			text[len(d.name)+1:] == suffixName
	}
	return text == d.name
}

// itemRandomPropertyName reads the default-locale name from
// ItemRandomProperties.dbc field 7 (DBCStructure.h:961).
func (s *session) itemRandomPropertyName(id uint32) string {
	store := s.hyperlinkStore()
	if store == nil {
		return ""
	}
	file, err := store.File("ItemRandomProperties")
	if err != nil {
		return ""
	}
	rec, ok := file.Find(id)
	if !ok {
		return ""
	}
	name, err := rec.String(7)
	if err != nil {
		return ""
	}
	return name
}

// itemRandomSuffixName reads the default-locale name from
// ItemRandomSuffix.dbc field 1 (DBCStructure.h:968).
func (s *session) itemRandomSuffixName(id uint32) string {
	store := s.hyperlinkStore()
	if store == nil {
		return ""
	}
	file, err := store.File("ItemRandomSuffix")
	if err != nil {
		return ""
	}
	rec, ok := file.Find(id)
	if !ok {
		return ""
	}
	name, err := rec.String(1)
	if err != nil {
		return ""
	}
	return name
}

type questLinkData struct {
	title string
}

// parseQuestLinkData mirrors LinkTags::quest::StoreTo
// (HyperlinkTags.cpp:174): the quest template must exist and the link's
// quest level must be >= -1.
func (s *session) parseQuestLinkData(ctx context.Context, t *hyperlinkDataTokenizer) (questLinkData, bool) {
	var d questLinkData
	tok, ok := t.next()
	if !ok {
		return d, false
	}
	questID, ok := hyperlinkParseUint(tok, 32)
	if !ok {
		return d, false
	}
	if tok, ok = t.next(); !ok {
		return d, false
	}
	questLevel, ok := hyperlinkParseInt(tok, 16)
	if !ok || questLevel < -1 {
		return d, false
	}
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return d, false
	}
	var level int32
	err := s.server.WorldStore.DB.QueryRowContext(ctx,
		"SELECT COALESCE(LogTitle, ''), QuestLevel FROM quest_template WHERE ID = ? LIMIT 1",
		uint32(questID)).Scan(&d.title, &level)
	if err != nil {
		return d, false
	}
	return d, true
}
