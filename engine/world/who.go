package world

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	maxWhoDefault   uint32  = 49
	inspectDistance float64 = 28.0
)

// handleWho processes CMSG_WHO (0x062).
// Reference: WorldSession::HandleWhoOpcode (MiscHandler.cpp:233).
func (s *session) handleWho(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return false
	}
	r := protocol.NewReader(payload)
	levelMin, err := r.ReadU32()
	if err != nil {
		return false
	}
	levelMax, err := r.ReadU32()
	if err != nil {
		return false
	}
	packetPlayerName, err := r.ReadCString()
	if err != nil {
		return false
	}
	packetGuildName, err := r.ReadCString()
	if err != nil {
		return false
	}
	raceMask, err := r.ReadU32()
	if err != nil {
		return false
	}
	classMask, err := r.ReadU32()
	if err != nil {
		return false
	}
	zonesCount, err := r.ReadU32()
	if err != nil {
		return false
	}
	if zonesCount > 10 {
		return false
	}
	zoneIDs := make([]uint32, zonesCount)
	for i := uint32(0); i < zonesCount; i++ {
		zid, err := r.ReadU32()
		if err != nil {
			return false
		}
		zoneIDs[i] = zid
	}
	strCount, err := r.ReadU32()
	if err != nil {
		return false
	}
	if strCount > 4 {
		return false
	}
	searchStrings := make([]string, strCount)
	for i := uint32(0); i < strCount; i++ {
		str, err := r.ReadCString()
		if err != nil {
			return false
		}
		searchStrings[i] = strings.ToLower(str)
	}

	if levelMax >= 100 {
		levelMax = 255
	}

	wPlayerName := strings.ToLower(packetPlayerName)
	wGuildName := strings.ToLower(packetGuildName)

	type whoMatch struct {
		name      string
		guildName string
		level     uint32
		class     uint32
		race      uint32
		gender    uint8
		zone      uint32
	}

	var matches []whoMatch
	var matchCount uint32
	maxWho := maxWhoDefault

	s.server.sessionsMu.RLock()
	allSessions := make([]*session, 0, len(s.server.sessions))
	for sess := range s.server.sessions {
		allSessions = append(allSessions, sess)
	}
	s.server.sessionsMu.RUnlock()

	for _, targetSession := range allSessions {
		if !targetSession.worldReady.Load() || targetSession.player == nil {
			continue
		}
		target := targetSession.player

		// GM invisibility check: if target is GM invisible, only GMs can see them
		if target.ExtraFlags&playerExtraGMInvisible != 0 {
			if s.security == 0 || targetSession.security > s.security {
				continue
			}
		}

		// Cross-team and GM-level visibility mirror HandleWhoOpcode's opening filters:
		// opposite-team players are hidden without RBAC_PERM_TWO_SIDE_WHO_LIST
		// (off by default), and console-level accounts stay hidden from players
		// without RBAC_PERM_WHO_SEE_ALL_SEC_LEVELS (default GM.InWhoList.Level = 3).
		if s.security == 0 {
			if teamForRace(target.Race) != teamForRace(s.player.Race) {
				continue
			}
			if targetSession.security > 3 {
				continue
			}
		}

		// Level range
		lvl := uint32(target.Level)
		if lvl < levelMin || lvl > levelMax {
			continue
		}

		// Class mask: C++ applies the mask unconditionally, so a zero mask matches nobody.
		if (classMask & (1 << target.Class)) == 0 {
			continue
		}

		// Race mask: same unconditional-mask semantics.
		if (raceMask & (1 << target.Race)) == 0 {
			continue
		}

		// Zones filter
		if zonesCount > 0 {
			zoneFound := false
			for _, zid := range zoneIDs {
				if zid == target.Zone {
					zoneFound = true
					break
				}
			}
			if !zoneFound {
				continue
			}
		}

		// Name filter
		targetNameLower := strings.ToLower(target.Name)
		if wPlayerName != "" && !strings.Contains(targetNameLower, wPlayerName) {
			continue
		}

		// Guild name
		guildName := ""
		if target.GuildID != 0 {
			guildName = s.server.getGuildName(ctx, target.GuildID)
		}
		guildNameLower := strings.ToLower(guildName)
		if wGuildName != "" && !strings.Contains(guildNameLower, wGuildName) {
			continue
		}

		// Search patterns filter
		patternMatch := true
		var areaName string
		if s.server.Data != nil {
			if area, ok, err := s.server.Data.Area(target.Zone); err == nil && ok {
				areaName = strings.ToLower(area.Name)
			}
		}
		for _, pat := range searchStrings {
			if pat == "" {
				continue
			}
			if !strings.Contains(targetNameLower, pat) &&
				!strings.Contains(guildNameLower, pat) &&
				(areaName == "" || !strings.Contains(areaName, pat)) {
				patternMatch = false
				break
			}
		}
		if !patternMatch {
			continue
		}

		matchCount++
		if uint32(len(matches)) < maxWho {
			matches = append(matches, whoMatch{
				name:      target.Name,
				guildName: guildName,
				level:     lvl,
				class:     uint32(target.Class),
				race:      uint32(target.Race),
				gender:    target.Gender,
				zone:      target.Zone,
			})
		}
	}

	displayCount := uint32(len(matches))
	buf := protocol.NewBuffer(256)
	buf.WriteU32(displayCount) // offset 0: displayCount (reference MiscHandler.cpp:401)
	buf.WriteU32(matchCount)   // offset 4: matchCount   (reference MiscHandler.cpp:402)
	for _, m := range matches {
		buf.WriteCString(m.name)
		buf.WriteCString(m.guildName)
		buf.WriteU32(m.level)
		buf.WriteU32(m.class)
		buf.WriteU32(m.race)
		buf.WriteU8(m.gender)
		buf.WriteU32(m.zone)
	}

	return s.write(uint16(protocol.OpcodeSMSG_WHO), buf.Bytes(), true) == nil
}

// handleWhoIs processes CMSG_WHOIS (0x064).
// Reference: WorldSession::HandleWhoIsOpcode (MiscHandler.cpp:1090).
func (s *session) handleWhoIs(ctx context.Context, payload []byte) bool {
	r := protocol.NewReader(payload)
	charName, err := r.ReadCString()
	if err != nil {
		return false
	}
	if s.server.AuthStore == nil || s.server.AuthStore.DB == nil {
		s.sendNotification("You do not have permission to use that command.")
		return true
	}
	allowed, err := accountHasPermission(ctx, s.server.AuthStore.DB, s.accountID, s.server.RealmID, s.security, permissionOpcodeWhois)
	if err != nil || !allowed {
		s.sendNotification("You do not have permission to use that command.")
		return true
	}
	if charName == "" || !utf8.ValidString(charName) {
		s.sendNotification("You must specify a character name.")
		return true
	}
	// MiscHandler.cpp:1097-1101: the lookup and the reply both use the
	// normalized name (ObjectMgr.cpp:141 fails only on empty/invalid UTF-8).
	charName = normalizePlayerName(charName)
	targetSession := s.server.findSessionByName(charName)
	if targetSession == nil || !targetSession.worldReady.Load() || targetSession.player == nil {
		s.sendNotification(fmt.Sprintf("Character '%s' is not online.", charName))
		return true
	}

	var acc, email, lastIP string
	err = s.server.AuthStore.DB.QueryRowContext(ctx, "SELECT username, email, last_ip FROM account WHERE id = ?", targetSession.accountID).Scan(&acc, &email, &lastIP)
	if err != nil {
		s.sendNotification(fmt.Sprintf("Could not find account info for player '%s'.", charName))
		return true
	}
	if acc == "" {
		acc = "Unknown"
	}
	if email == "" {
		email = "Unknown"
	}
	if lastIP == "" {
		lastIP = "Unknown"
	}

	msg := fmt.Sprintf("%s's account is %s, e-mail: %s, last ip: %s", targetSession.player.Name, acc, email, lastIP)
	buf := protocol.NewBuffer(len(msg) + 1)
	buf.WriteCString(msg)
	return s.write(uint16(protocol.OpcodeSMSG_WHOIS), buf.Bytes(), true) == nil
}

// handleInspect processes CMSG_INSPECT (0x114).
// Reference: WorldSession::HandleInspectOpcode (MiscHandler.cpp:1003).
// inspectWithinDistance mirrors the range arm of the inspect-family handlers
// (MiscHandler.cpp:1018/1050/1423, ArenaTeamHandler.cpp:48):
// IsWithinDistInMap(player, INSPECT_DISTANCE, false) — the false selects the 2D
// distance. Bounding radii have no Go model.
func inspectWithinDistance(requester, target *playerState) bool {
	dx := float64(requester.X - target.X)
	dy := float64(requester.Y - target.Y)
	return math.Sqrt(dx*dx+dy*dy) <= inspectDistance
}

// inspectBlockedByHostility mirrors the player-vs-player PvP arm of
// WorldObject::IsValidAttackTarget (Object.cpp:3006-3017): same-team pairs are
// never valid attack targets (the duel/arena arms need duel state Go does not
// feed here); a cross-team target blocks inspect only while it IsPvP()
// (PLAYER_FLAGS_IN_PVP is Go's modeled proxy — the UNIT byte flag and the
// EndTimer/hostile-area machinery have no Go model), both sides carry
// UNIT_BYTE2_FLAG_FFA_PVP, or either side carries UNIT_BYTE2_FLAG_UNK1 (no Go
// model). Sanctuary, GM-mode targets and the UNK1 byte flag stay documented gaps.
func inspectBlockedByHostility(requester, target *playerState) bool {
	if playerTeam(requester.Race) == playerTeam(target.Race) {
		return false
	}
	if target.PlayerFlags&playerFlagInPVP != 0 {
		return true
	}
	if requester.PVPFlags&pvpFlagFFA != 0 && target.PVPFlags&pvpFlagFFA != 0 {
		return true
	}
	return false
}

// inspectEquipmentRow carries one equipped item for the inspect packet.
type inspectEquipmentRow struct {
	slot         int
	entry        uint32
	enchantMask  uint16
	enchantIDs   []uint16
	randomPropID int16
	creatorGUID  uint64
	suffixFactor uint32
}

// loadInspectEquipment mirrors the item loop of Player::BuildEnchantmentsInfoData
// (Player.cpp): one row per equipped slot with the 12-slot enchantment mask
// (enchantments column is slot*3 + (id, duration, charges) triplets), the
// random-property id, creator GUID and suffix factor the client parses after
// the enchantment list. Falls back to the cached Equipment string when the
// characters DB is unavailable.
func (s *session) loadInspectEquipment(ctx context.Context, charGUID uint64, equipment string) []inspectEquipmentRow {
	var rows []inspectEquipmentRow
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		dbRows, err := s.server.CharactersStore.DB.QueryContext(ctx,
			`SELECT ci.slot, ii.itemEntry, COALESCE(ii.enchantments, ''), COALESCE(ii.randomPropertyId, 0), COALESCE(ii.creatorGuid, 0)
			 FROM character_inventory AS ci JOIN item_instance AS ii ON ii.guid = ci.item
			 WHERE ci.guid = ? AND ci.bag = 0 AND ci.slot < ? ORDER BY ci.slot`, charGUID, playerVisibleItemCount)
		if err == nil {
			seen := make(map[int]bool)
			for dbRows.Next() {
				var slot, entry, randomProp, creator int64
				var enchantments string
				if dbRows.Scan(&slot, &entry, &enchantments, &randomProp, &creator) != nil ||
					slot < 0 || slot >= playerVisibleItemCount || entry <= 0 || seen[int(slot)] {
					continue
				}
				seen[int(slot)] = true
				row := inspectEquipmentRow{slot: int(slot), entry: uint32(entry), randomPropID: int16(randomProp), creatorGUID: uint64(creator)}
				fields := strings.Fields(enchantments)
				for j := 0; j < 12; j++ {
					idx := j * 3
					if idx >= len(fields) {
						break
					}
					id, err := strconv.ParseUint(fields[idx], 10, 32)
					if err != nil || id == 0 {
						continue
					}
					row.enchantMask |= 1 << uint(j)
					row.enchantIDs = append(row.enchantIDs, uint16(id))
				}
				row.suffixFactor = s.inspectSuffixFactor(ctx, uint32(entry), row.randomPropID)
				rows = append(rows, row)
			}
			dbRows.Close()
		}
	}
	if rows == nil {
		parts := strings.Fields(equipment)
		for slot := 0; slot < playerVisibleItemCount; slot++ {
			base := slot * 2
			if base >= len(parts) {
				break
			}
			entry, err := strconv.ParseUint(parts[base], 10, 32)
			if err != nil || entry == 0 {
				continue
			}
			row := inspectEquipmentRow{slot: slot, entry: uint32(entry)}
			if base+1 < len(parts) {
				// PackVisibleEnchantments carries PERM in the low 16 bits.
				if enc, err := strconv.ParseUint(parts[base+1], 10, 32); err == nil && enc&0xFFFF != 0 {
					row.enchantMask = 1
					row.enchantIDs = []uint16{uint16(enc & 0xFFFF)}
				}
			}
			rows = append(rows, row)
		}
	}
	return rows
}

// inspectSuffixFactor mirrors Item::GetItemSuffixFactor (Item.h:139 —
// ITEM_FIELD_PROPERTY_SEED), set only for random-suffix items via
// UpdateItemSuffixFactor/GenerateEnchSuffixFactor (Item.cpp:624-632).
func (s *session) inspectSuffixFactor(ctx context.Context, entry uint32, randomPropID int16) uint32 {
	if randomPropID >= 0 || s.server == nil || s.server.Data == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return 0
	}
	var itemLevel, quality, invType, randomSuffix int64
	if err := s.server.WorldStore.DB.QueryRowContext(ctx,
		`SELECT ItemLevel, Quality, InventoryType, RandomSuffix FROM item_template WHERE entry = ? LIMIT 1`,
		entry).Scan(&itemLevel, &quality, &invType, &randomSuffix); err != nil {
		return 0
	}
	return ResolveItemSuffixFactor(s.server.Data, uint32(itemLevel), uint32(quality), uint32(invType), uint32(randomSuffix))
}

// writeInspectTalentData mirrors Player::BuildPlayerTalentsInfoData (Player.cpp):
// u32 unspent points, u8 spec count, u8 active spec, then per spec the u8 talent
// count, (u32 talentId, u8 rank) pairs in talent-ID order, the u8 glyph count
// and u16 glyph IDs. Non-active specs are loaded from character_talent — the
// session only carries the active group in memory.
func (s *session) writeInspectTalentData(ctx context.Context, buf *protocol.Buffer, targetSession *session, target *playerState) {
	specs := target.TalentGroupsCount
	if specs < 1 {
		specs = 1
	} else if specs > 2 {
		specs = 2
	}
	active := target.ActiveTalentGroup
	if active >= specs {
		active = 0
	}
	buf.WriteU32(targetSession.freeTalentPoints())
	buf.WriteU8(specs)
	buf.WriteU8(active)
	for spec := uint8(0); spec < specs; spec++ {
		talents := target.Talents
		if spec != target.ActiveTalentGroup {
			talents = s.talentsForGroup(ctx, target.GUID, spec)
		}
		ids := make([]uint32, 0, len(talents))
		for tid := range talents {
			ids = append(ids, tid)
		}
		sort.Slice(ids, func(a, b int) bool { return ids[a] < ids[b] })
		buf.WriteU8(uint8(len(ids)))
		for _, tid := range ids {
			buf.WriteU32(tid)
			buf.WriteU8(talents[tid])
		}
		buf.WriteU8(maxGlyphSlotIndex)
		for i := uint8(0); i < maxGlyphSlotIndex; i++ {
			var glyphID uint16
			if int(spec) < len(target.Glyphs) && int(i) < len(target.Glyphs[spec]) {
				glyphID = target.Glyphs[spec][i]
			}
			buf.WriteU16(glyphID)
		}
	}
}

// talentsForGroup loads one talent group's learned talents (talentId -> rank),
// mirroring loadPlayerTalents for a non-active group.
func (s *session) talentsForGroup(ctx context.Context, charGUID uint64, group uint8) map[uint32]uint8 {
	talents := make(map[uint32]uint8)
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || s.server.Data == nil {
		return talents
	}
	rows, err := s.server.CharactersStore.DB.QueryContext(ctx, "SELECT spell FROM character_talent WHERE guid = ? AND talentGroup = ?", charGUID, group)
	if err != nil {
		return talents
	}
	defer rows.Close()
	for rows.Next() {
		var spellID int64
		if err := rows.Scan(&spellID); err != nil || spellID <= 0 {
			continue
		}
		if tid, r, found := s.server.Data.TalentBySpell(uint32(spellID)); found {
			talents[tid] = r
		}
	}
	return talents
}

func (s *session) handleInspect(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return false
	}
	r := protocol.NewReader(payload)
	targetGUID, err := r.ReadU64()
	if err != nil {
		return false
	}
	targetSession := s.server.findSessionByGUID(targetGUID)
	if targetSession == nil || !targetSession.worldReady.Load() || targetSession.player == nil {
		return true
	}
	target := targetSession.player

	// Must be on the same map
	if target.Map != s.player.Map {
		return true
	}

	// IsWithinDistInMap(player, INSPECT_DISTANCE) + IsValidAttackTarget gates
	// (MiscHandler.cpp:1018-1021); C++ answers nothing when any fails.
	if !inspectWithinDistance(s.player, target) {
		return true
	}
	if inspectBlockedByHostility(s.player, target) {
		return true
	}

	buf := protocol.NewBuffer(64)
	buf.WritePackedGUID(targetGUID)

	// Talent visibility (MiscHandler.cpp:1024): CanBeGameMaster() is
	// RBAC_PERM_COMMAND_GM (held at security >= SEC_GAMEMASTER); with
	// CONFIG_TALENTS_INSPECTING at its default of 1, a same-team requester sees
	// talents while a cross-team non-GM gets the zero arm.
	if s.security >= 2 || playerTeam(s.player.Race) == playerTeam(target.Race) {
		s.writeInspectTalentData(ctx, buf, targetSession, target)
	} else {
		buf.WriteU32(0) // unspentTalentPoints
		buf.WriteU8(0)  // talentGroupCount
		buf.WriteU8(0)  // talentGroupIndex
	}

	// Enchantments info data: Player::BuildEnchantmentsInfoData (Player.cpp) —
	// per equipped slot: u32 entry, u16 enchantmentMask, u16 per set slot bit in
	// slot order, int16 randomPropertyId, packed creator GUID, u32 suffix factor.
	equip := s.loadInspectEquipment(ctx, target.GUID, target.Equipment)
	var slotUsedMask uint32
	for _, item := range equip {
		slotUsedMask |= 1 << uint(item.slot)
	}
	buf.WriteU32(slotUsedMask)
	for _, item := range equip {
		buf.WriteU32(item.entry)
		buf.WriteU16(item.enchantMask)
		for _, ench := range item.enchantIDs {
			buf.WriteU16(ench)
		}
		buf.WriteI16(item.randomPropID)
		buf.WritePackedGUID(item.creatorGUID)
		buf.WriteU32(item.suffixFactor)
	}

	return s.write(uint16(protocol.OpcodeSMSG_INSPECT_TALENT), buf.Bytes(), true) == nil
}
