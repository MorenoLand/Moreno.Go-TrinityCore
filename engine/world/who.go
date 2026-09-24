package world

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

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

		// Level range
		lvl := uint32(target.Level)
		if lvl < levelMin || lvl > levelMax {
			continue
		}

		// Class mask
		if classMask != 0 && (classMask&(1<<target.Class)) == 0 {
			continue
		}

		// Race mask
		if raceMask != 0 && (raceMask&(1<<target.Race)) == 0 {
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
	if charName == "" {
		s.sendNotification("You must specify a character name.")
		return true
	}
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

	// Distance check: INSPECT_DISTANCE = 28.0 yards (ObjectDefines.h:26)
	dx := float64(s.player.X - target.X)
	dy := float64(s.player.Y - target.Y)
	dz := float64(s.player.Z - target.Z)
	dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
	if dist > inspectDistance {
		return true
	}

	// Hostility check: cannot inspect valid attack targets (opposite faction without GM)
	if s.security == 0 && playerTeam(s.player.Race) != playerTeam(target.Race) {
		return true
	}

	buf := protocol.NewBuffer(64)
	buf.WritePackedGUID(targetGUID)

	// Talents info (unspentTalentPoints: u32, talentGroupCount: u8, talentGroupIndex: u8)
	if len(target.Talents) > 0 {
		buf.WriteU32(targetSession.freeTalentPoints())
		buf.WriteU8(1) // talentGroupCount
		buf.WriteU8(0) // talentGroupIndex

		buf.WriteU8(uint8(len(target.Talents)))
		for tid, rank := range target.Talents {
			buf.WriteU32(tid)
			buf.WriteU8(rank)
		}
		buf.WriteU8(maxGlyphSlotIndex)
		for i := uint8(0); i < maxGlyphSlotIndex; i++ {
			glyphID := uint16(0)
			spec := target.ActiveTalentGroup
			if int(spec) < len(target.Glyphs) && int(i) < len(target.Glyphs[spec]) {
				glyphID = target.Glyphs[spec][i]
			}
			buf.WriteU16(glyphID)
		}
	} else {
		buf.WriteU32(0) // unspentTalentPoints
		buf.WriteU8(0)  // talentGroupCount
		buf.WriteU8(0)  // talentGroupIndex
	}

	// Enchantments info data: BuildEnchantmentsInfoData (Player.cpp:25920)
	var slotUsedMask uint32
	type equippedItem struct {
		slot    int
		itemID  uint32
		enchant uint32
	}
	var items []equippedItem
	equipment := strings.Fields(target.Equipment)
	for slot := 0; slot < playerVisibleItemCount; slot++ {
		base := slot * 2
		if base >= len(equipment) {
			break
		}
		itemID, err := strconv.ParseUint(equipment[base], 10, 32)
		if err != nil || itemID == 0 {
			continue
		}
		var enchant uint32
		if base+1 < len(equipment) {
			if enc, err := strconv.ParseUint(equipment[base+1], 10, 32); err == nil {
				enchant = uint32(enc)
			}
		}
		slotUsedMask |= 1 << slot
		items = append(items, equippedItem{slot: slot, itemID: uint32(itemID), enchant: enchant})
	}

	buf.WriteU32(slotUsedMask)
	for _, item := range items {
		buf.WriteU32(item.itemID)
		if item.enchant != 0 {
			buf.WriteU16(1) // enchantmentMask bit 0 set
			buf.WriteU16(uint16(item.enchant))
		} else {
			buf.WriteU16(0) // enchantmentMask = 0
		}
	}

	return s.write(uint16(protocol.OpcodeSMSG_INSPECT_TALENT), buf.Bytes(), true) == nil
}
