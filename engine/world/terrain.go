package world

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/config"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	terrainGridSize             = float64(533.3333)
	terrainCenterGrid           = 32.0
	terrainMapHalfSize          = float32(0.5 * 64.0 * 533.33333333)
	terrainAreaNoArea    uint16 = 0x0001
	terrainModelM2              = uint32(0x00000001)
	terrainModelHasBound        = uint32(0x00000004)
)

type terrainVector struct{ X, Y, Z float32 }
type terrainSpawn struct {
	Flags      uint32
	ADTID      uint16
	ID         uint32
	Position   terrainVector
	Rotation   terrainVector
	Scale      float32
	BoundsLow  terrainVector
	BoundsHigh terrainVector
	Name       string
}
type terrainTriangle struct{ A, B, C uint32 }
type terrainGroup struct {
	Low       terrainVector
	High      terrainVector
	Flags     uint32
	ID        uint32
	Vertices  []terrainVector
	Triangles []terrainTriangle
}
type terrainModel struct {
	Root   uint32
	Groups []terrainGroup
}
type terrainWMOHit struct {
	Root   uint32
	ADT    uint16
	Group  uint32
	Ground float32
}

type spellAreaRule struct {
	Area             uint32
	QuestStart       uint32
	QuestEnd         uint32
	AuraSpell        int32
	RaceMask         uint32
	Gender           uint8
	Autocast         bool
	QuestStartStatus uint32
	QuestEndStatus   uint32
}

func ResolveTerrainZoneAndArea(dataDir string, mapID uint32, x, y, z float32, fallback uint32) (uint32, uint32, error) {
	if dataDir == "" {
		return 0, 0, fmt.Errorf("terrain data directory is required")
	}
	server := &Server{Config: config.Config{GameDataDir: dataDir}, Data: wotlk.NewStore(filepath.Join(dataDir, "dbc")), terrainTiles: make(map[uint64][]terrainSpawn), terrainTileKnown: make(map[uint64]bool), terrainModels: make(map[string]*terrainModel)}
	zoneID, areaID := server.zoneAndAreaID(mapID, x, y, z, fallback)
	return zoneID, areaID, nil
}

func (s *Server) zoneAndAreaID(mapID uint32, x, y, z float32, _ uint32) (uint32, uint32) {
	areaID, found := s.mapWMOAreaID(mapID, x, y, z)
	if !found {
		areaID, found = s.mapAreaID(mapID, x, y)
	}
	if !found && s.Data != nil {
		if mapEntry, ok, err := s.Data.Map(mapID); err == nil && ok {
			areaID = mapEntry.AreaTableID
			found = areaID != 0
		}
	}
	zoneID := areaID
	if s.Data != nil && areaID != 0 {
		if area, ok, err := s.Data.Area(areaID); err == nil && ok && area.ParentAreaID != 0 {
			zoneID = area.ParentAreaID
		}
	}
	return zoneID, areaID
}

func (s *session) updateAreaDependentAuras(ctx context.Context, zoneID, areaID uint32) bool {
	if s == nil || s.server == nil || s.server.Data == nil || s.player == nil {
		return false
	}
	changed := false
	for _, aura := range s.loadedAuras() {
		if aura == nil {
			continue
		}
		spell, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		if spell.AreaGroupID > 0 {
			allowed, known, groupErr := s.server.Data.AreaGroupAllows(uint32(spell.AreaGroupID), zoneID, areaID)
			if groupErr != nil || !known {
				continue
			}
			if !allowed {
				s.removeAreaRestrictedAura(ctx, aura.SpellID)
				changed = true
				continue
			}
		}
		rules, hasRules := s.spellAreaRules(ctx, aura.SpellID)
		if !hasRules || s.spellAreaRulesFit(ctx, aura.SpellID, rules, zoneID, areaID) {
			continue
		}
		s.removeAreaRestrictedAura(ctx, aura.SpellID)
		changed = true
	}
	s.applySpellAreaAutocasts(ctx, zoneID, areaID)
	return changed
}

func (s *session) destroyZoneLimitedItems(ctx context.Context, zoneID uint32) bool {
	if s == nil || s.player == nil || s.player.Health == 0 || s.player.PlayerFlags&playerFlagGhost != 0 || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return false
	}
	rows, err := s.server.CharactersStore.DB.QueryContext(ctx, `SELECT ci.item, ii.itemEntry, ii.count FROM character_inventory AS ci JOIN item_instance AS ii ON ii.guid = ci.item JOIN item_template AS it ON it.entry = ii.itemEntry WHERE ci.guid = ? AND ((COALESCE(it.Map, 0) <> 0 AND it.Map <> ?) OR (COALESCE(it.area, 0) <> 0 AND it.area <> ?))`, s.playerGUID, s.player.Map, zoneID)
	if err != nil {
		return false
	}
	type zoneLimitedItem struct{ guid, entry, count uint64 }
	items := make([]zoneLimitedItem, 0)
	for rows.Next() {
		var item zoneLimitedItem
		if rows.Scan(&item.guid, &item.entry, &item.count) == nil && item.guid != 0 && item.entry != 0 && item.count != 0 {
			items = append(items, item)
		}
	}
	rows.Close()
	if len(items) == 0 {
		return false
	}
	for _, item := range items {
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "DELETE FROM character_inventory WHERE guid = ? AND item = ?", s.playerGUID, item.guid)
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "DELETE FROM item_instance WHERE guid = ?", item.guid)
		s.adjustQuestItemCount(ctx, uint32(item.entry), uint32(item.count), false)
		s.despawnItem(item.guid)
	}
	s.syncEquipmentCache(ctx)
	_ = s.sendInventoryItems(ctx)
	s.sendPlayerUpdate()
	return true
}

func (s *session) autoUnequipOffhandIfNeeded(ctx context.Context) bool {
	return s.autoUnequipOffhandIfNeededForced(ctx, false)
}

// autoUnequipOffhandIfNeededForced mirrors Player::AutoUnequipOffhandIfNeed
// (Player.cpp:23877-23910): the offhand weapon leaves when the player can no
// longer dual wield, or when a 2h weapon without Titan's Grip sits in either
// hand. The inventory search mirrors CanStoreItem(NULL_BAG, NULL_SLOT): the
// backpack AND all equipped bags are searched, not the backpack alone; when
// the whole inventory is full the item is mailed to self with the
// LANG_NOT_EQUIPPED_ITEM subject, GM stationery and MAIL_CHECK_MASK_COPIED.
func (s *session) autoUnequipOffhandIfNeededForced(ctx context.Context, force bool) bool {
	if s == nil || s.player == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return false
	}
	cdb := s.server.CharactersStore.DB
	var offItem, offEntry, offInvType, mainInvType int64
	if err := cdb.QueryRowContext(ctx, `SELECT ci.item, ii.itemEntry, COALESCE(it.InventoryType, 0) FROM character_inventory AS ci JOIN item_instance AS ii ON ii.guid = ci.item JOIN item_template AS it ON it.entry = ii.itemEntry WHERE ci.guid = ? AND ci.bag = 0 AND ci.slot = ? LIMIT 1`, s.playerGUID, equipSlotOffhand).Scan(&offItem, &offEntry, &offInvType); err != nil || offItem == 0 {
		return false
	}
	_ = cdb.QueryRowContext(ctx, `SELECT COALESCE(it.InventoryType, 0) FROM character_inventory AS ci JOIN item_instance AS ii ON ii.guid = ci.item JOIN item_template AS it ON it.entry = ii.itemEntry WHERE ci.guid = ? AND ci.bag = 0 AND ci.slot = ? LIMIT 1`, s.playerGUID, equipSlotMainhand).Scan(&mainInvType)
	canDualWield := false
	canTitanGrip := false
	for _, learned := range s.player.Spells {
		if !learned.Active || learned.Disabled {
			continue
		}
		if learned.ID == 674 {
			canDualWield = true
		}
		if s.server.Data != nil {
			if spell, found, err := s.server.Data.Spell(learned.ID); err == nil && found {
				for _, effect := range spell.Effects {
					if effect.Effect == 155 {
						canTitanGrip = true
					}
				}
			}
		}
	}
	force = force || (!canDualWield && (offInvType == invTypeWeapon || offInvType == invTypeWeaponOffhand))
	if !force && (canTitanGrip || (offInvType != invType2HWeapon && mainInvType != invType2HWeapon)) {
		return false
	}
	if bagKey, _, freeSlot, ok := s.findFreeInventorySlot(ctx, s.playerGUID); ok {
		if _, err := cdb.ExecContext(ctx, "UPDATE character_inventory SET bag = ?, slot = ? WHERE guid = ? AND item = ?", bagKey, freeSlot, s.playerGUID, offItem); err != nil {
			return false
		}
	} else if !s.mailSelfUnstoreableOffhand(ctx, cdb, uint64(offItem)) {
		return false
	}
	s.syncEquipmentCache(ctx)
	_ = s.sendInventoryItems(ctx)
	s.sendPlayerUpdate()
	return true
}

// mailSelfUnstoreableOffhand mirrors the no-space else arm of
// Player::AutoUnequipOffhandIfNeed (Player.cpp:23899-23908): the offhand
// item leaves the inventory (MoveItemFromInventory + DeleteFromInventoryDB +
// SaveToDB standalone) and is mailed to self with the LANG_NOT_EQUIPPED_ITEM
// subject, MAIL_STATIONERY_GM and MAIL_CHECK_MASK_COPIED, in one transaction.
func (s *session) mailSelfUnstoreableOffhand(ctx context.Context, cdb *sql.DB, itemGUID uint64) bool {
	if cdb == nil || itemGUID == 0 || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return false
	}
	var subject string
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT content_default FROM trinity_string WHERE entry = ?", langNotEquippedItem).Scan(&subject); err != nil {
		return false
	}
	tx, err := cdb.BeginTx(ctx, nil)
	if err != nil {
		return false
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "DELETE FROM character_inventory WHERE guid = ? AND item = ?", s.playerGUID, itemGUID); err != nil {
		return false
	}
	if _, err := tx.ExecContext(ctx, "UPDATE item_instance SET owner_guid = ? WHERE guid = ?", s.playerGUID, itemGUID); err != nil {
		return false
	}
	mailID := s.server.generateMailID()
	now := time.Now().Unix()
	if _, err := tx.ExecContext(ctx, `INSERT INTO mail (id, messageType, stationery, mailTemplateId, sender, receiver, subject, body, has_items, expire_time, deliver_time, money, cod, checked)
		VALUES (?, 0, ?, 0, ?, ?, ?, ?, 1, ?, ?, 0, 0, ?)`, mailID, gmStationery, s.playerGUID, s.playerGUID, subject, "There were problems with equipping one or several items.", now+mailExpiration, now, mailCheckCopied); err != nil {
		return false
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO mail_items (mail_id, item_guid, receiver) VALUES (?, ?, ?)", mailID, itemGUID, s.playerGUID); err != nil {
		return false
	}
	if err := tx.Commit(); err != nil {
		return false
	}
	return true
}

func (s *session) removeAreaRestrictedAura(ctx context.Context, spellID uint32) {
	s.removeAura(spellID)
	if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "DELETE FROM character_aura WHERE guid = ? AND spell = ?", s.playerGUID, spellID)
	}
}

// fetchSpellAreaRules reads the raw spell_area rows for a spell; both the
// session twin (player casts) and the creature-path twin (null player) build
// on it. ok is false when the query errors or no rows exist.
func fetchSpellAreaRules(ctx context.Context, db *sql.DB, spellID uint32) ([]spellAreaRule, bool) {
	if db == nil {
		return nil, false
	}
	rows, err := db.QueryContext(ctx, `SELECT area, quest_start, quest_end, aura_spell, racemask, gender, autocast, quest_start_status, quest_end_status FROM spell_area WHERE spell = ?`, spellID)
	if err != nil {
		return nil, false
	}
	defer rows.Close()
	rules := make([]spellAreaRule, 0)
	for rows.Next() {
		var rule spellAreaRule
		var auraSpell, gender, autocast int64
		if err := rows.Scan(&rule.Area, &rule.QuestStart, &rule.QuestEnd, &auraSpell, &rule.RaceMask, &gender, &autocast, &rule.QuestStartStatus, &rule.QuestEndStatus); err != nil {
			continue
		}
		rule.AuraSpell, rule.Gender, rule.Autocast = int32(auraSpell), uint8(gender), autocast != 0
		rules = append(rules, rule)
	}
	return rules, rows.Err() == nil && len(rules) > 0
}

func (s *session) spellAreaRules(ctx context.Context, spellID uint32) ([]spellAreaRule, bool) {
	if s == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return nil, false
	}
	return fetchSpellAreaRules(ctx, s.server.WorldStore.DB, spellID)
}

func (s *session) spellAreaQuestStatus(ctx context.Context, questID uint32) int64 {
	if questID == 0 {
		return 0
	}
	status, _ := s.characterQuestStatus(ctx, questID)
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		var rewarded int64
		if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_queststatus_rewarded WHERE guid = ? AND quest = ?", s.playerGUID, questID).Scan(&rewarded); err == nil && rewarded > 0 {
			return 6
		}
	}
	return status
}

func (s *session) spellAreaRuleFits(ctx context.Context, rule spellAreaRule, zoneID, areaID uint32) bool {
	if rule.Area != 0 && rule.Area != zoneID && rule.Area != areaID {
		return false
	}
	if rule.RaceMask != 0 && rule.RaceMask&playerCreateMask(s.player.Race) == 0 {
		return false
	}
	if rule.Gender != 2 && rule.Gender != s.player.Gender {
		return false
	}
	if rule.QuestStart != 0 && rule.QuestStartStatus&(uint32(1)<<uint(s.spellAreaQuestStatus(ctx, rule.QuestStart))) == 0 {
		return false
	}
	if rule.QuestEnd != 0 && rule.QuestEndStatus&(uint32(1)<<uint(s.spellAreaQuestStatus(ctx, rule.QuestEnd))) == 0 {
		return false
	}
	if rule.AuraSpell > 0 && !s.hasAura(uint32(rule.AuraSpell)) {
		return false
	}
	if rule.AuraSpell < 0 && s.hasAura(uint32(-rule.AuraSpell)) {
		return false
	}
	return true
}

func (s *session) spellAreaRulesFit(ctx context.Context, spellID uint32, rules []spellAreaRule, zoneID, areaID uint32) bool {
	// Extra-conditions switch (SpellArea::IsFitToRequirements tail,
	// SpellMgr.cpp:718-781): it fails for the whole spell, not per row —
	// C++ runs it inside every row's IsFitToRequirements, so a failing
	// switch fails every row. Evaluating it once per spell is equivalent.
	if spellAreaExtraConditionSpell(spellID) && !s.spellAreaExtraConditionFits(spellID, zoneID, areaID) {
		return false
	}
	for _, rule := range rules {
		if s.spellAreaRuleFits(ctx, rule, zoneID, areaID) {
			return true
		}
	}
	return false
}

// wgForZone mirrors sBattlefieldMgr->GetBattlefieldToZoneId for Wintergrasp:
// the Go world tracks only the WG battlefield, so any other zone resolves to
// nil exactly like a null Battlefield* in C++.
func (s *session) wgForZone(zoneID uint32) *wgBattlegroundState {
	if s == nil || s.server == nil {
		return nil
	}
	s.server.wgMu.RLock()
	wg := s.server.wgState
	s.server.wgMu.RUnlock()
	if wg == nil || wg.ZoneID != zoneID {
		return nil
	}
	return wg
}

// wgWorkshopForAreaID maps the four park/graveyard areas to Go workshop slots
// the way BattlefieldWG::GetData(area) resolves them through
// GetSpiritGraveyardId (BattlefieldWG.cpp:1134-1149): "Graveyards and
// Workshops are controlled by the same team", so the workshop's TeamControl
// is the faithful GetData value.
func wgWorkshopForAreaID(areaID uint32) uint8 {
	switch areaID {
	case 4538: // AREA_THE_SUNKEN_RING (SharedDefines.h:3835)
		return WGWorkshopNE
	case 4539: // AREA_THE_BROKEN_TEMPLATE (SharedDefines.h:3836)
		return WGWorkshopNW
	case 4611: // AREA_WESTPARK_WORKSHOP (SharedDefines.h:3839)
		return WGWorkshopSW
	case 4612: // AREA_EASTPARK_WORKSHOP (SharedDefines.h:3840)
		return WGWorkshopSE
	}
	return WGMaxWorkshops
}

// spellAreaExtraConditionFits mirrors the extra-conditions switch at the tail
// of SpellArea::IsFitToRequirements (SpellMgr.cpp:718-781): after the
// gender/race/area/quest/aura terms, these spells carry extra player-gated
// conditions. Every case opens with a null-player guard (vacuous here — the
// player-side fits path always has a player; the creature twin handles the
// null case in creature_spell_area.go). The Battleground leg just ahead of
// the switch (player->GetBattleground() -> bg->IsSpellAllowed,
// SpellMgr.cpp:714-717) has no Go model — there is no live Battleground
// instance (battleground.go documents the arena/BG port as no-bridge) —
// so it is not bridged.
func (s *session) spellAreaExtraConditionFits(spellID, zoneID, areaID uint32) bool {
	switch spellID {
	case 58600: // No fly Zone - Dalaran
		// C++ re-reads the player's CURRENT area (player->GetAreaId()) and
		// fails closed on a missing AreaTable row (pArea && is tested as one
		// condition). The Go autocast/removal paths pass the freshly computed
		// area, which is that same value at this point in the update.
		area, found, areaErr := s.server.Data.Area(areaID)
		if areaErr != nil || !found || area.Flags&areaFlagNoFlyZone == 0 {
			return false
		}
		return s.hasAuraType(spellAuraModIncreaseMountedFlightSpeed) || s.hasAuraType(spellAuraFly)
	case 58730: // No fly Zone - Wintergrasp
		wg := s.wgForZone(zoneID)
		if wg == nil || wg.wgCanFlyIn() {
			return false
		}
		return s.hasAuraType(spellAuraModIncreaseMountedFlightSpeed) || s.hasAuraType(spellAuraFly)
	case 56618, 56617: // Horde/Alliance Controls Factory Phase Shift
		wg := s.wgForZone(zoneID)
		if wg == nil {
			return false
		}
		// bf->GetTypeId() == BATTLEFIELD_WG is structural: the Go WG state is
		// always the WG battlefield. C++ passes newArea (the rule's area) to
		// GetData; the Go path carries the same areaID.
		workshopID := wgWorkshopForAreaID(areaID)
		if workshopID >= WGMaxWorkshops {
			return false
		}
		wg.mu.Lock()
		team := wg.Workshops[workshopID].TeamControl
		wg.mu.Unlock()
		if team == WGTeamHorde {
			return spellID == 56618
		}
		if team == WGTeamAlliance {
			return spellID == 56617
		}
		// Neutral or unknown control: C++ breaks out of the switch, which
		// falls through to return true.
		return true
	case 57940, 58045: // Essence of Wintergrasp - Northrend / Wintergrasp
		wg := s.wgForZone(zoneID)
		if wg == nil {
			return true // C++ breaks out of the switch -> return true
		}
		wg.mu.Lock()
		defer wg.mu.Unlock()
		// player->GetTeamId() is teamForRace in Go (0 alliance / 1 horde,
		// matching WGTeamAlliance/WGTeamHorde); IsWarTime is wg.IsActive
		// (Battlefield.h:246).
		return wg.Enabled && teamForRace(s.player.Race) == wg.DefenderTeam && !wg.IsActive
	case 74411: // Battleground - Dampening
		wg := s.wgForZone(zoneID)
		if wg == nil {
			return true // C++ breaks out of the switch -> return true
		}
		wg.mu.Lock()
		defer wg.mu.Unlock()
		return wg.IsActive
	}
	return true
}

func (s *session) applySpellAreaAutocasts(ctx context.Context, zoneID, areaID uint32) {
	if s == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil || s.player == nil {
		return
	}
	rows, err := s.server.WorldStore.DB.QueryContext(ctx, "SELECT DISTINCT spell FROM spell_area WHERE autocast <> 0 AND (area = 0 OR area = ? OR area = ?)", zoneID, areaID)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var spellID uint32
		if rows.Scan(&spellID) != nil || spellID == 0 || s.hasAura(spellID) {
			continue
		}
		rules, hasRules := s.spellAreaRules(ctx, spellID)
		if !hasRules || !s.spellAreaRulesFit(ctx, spellID, rules, zoneID, areaID) {
			continue
		}
		if _, found, spellErr := s.server.Data.Spell(spellID); spellErr == nil && found {
			s.castSpellDirect(ctx, spellID, s.playerGUID)
		}
	}
}

func (s *session) updateZoneAndArea(ctx context.Context, force bool) {
	if s == nil || s.server == nil || s.player == nil || !s.playerLoaded {
		return
	}
	now := time.Now()
	zoneID, areaID := s.server.zoneAndAreaID(s.player.Map, s.player.X, s.player.Y, s.player.Z, s.player.Zone)
	s.exploreArea(ctx, areaID)
	if !force && !s.lastZoneUpdate.IsZero() && now.Sub(s.lastZoneUpdate) < time.Second {
		return
	}
	oldZone, oldArea, oldMap := s.player.Zone, s.areaID, s.player.Map
	s.player.Zone, s.areaID, s.lastZoneUpdate = zoneID, areaID, now
	stateChanged := false
	if oldZone != s.player.Zone || oldArea != areaID {
		stateChanged = s.updateAreaDependentAuras(ctx, s.player.Zone, areaID)
		if oldZone != s.player.Zone {
			stateChanged = s.destroyZoneLimitedItems(ctx, s.player.Zone) || stateChanged
			stateChanged = s.autoUnequipOffhandIfNeeded(ctx) || stateChanged
		}
		stateChanged = s.applyZoneState(ctx, s.player, s.player.Zone, areaID) || stateChanged
	}
	if oldZone != s.player.Zone {
		if oldMap == WGMapID && oldZone == WGZoneID {
			s.server.handleWGPlayerLeave(s)
		}
		if s.player.Map == WGMapID && s.player.Zone == WGZoneID {
			s.server.handleWGPlayerEnter(s)
		}
		if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
			_, _ = s.server.CharactersStore.ExecStatement(ctx, "CHAR_UPD_ZONE", s.player.Zone, s.playerGUID)
		}
		s.server.ensureZoneWeather(ctx, s.player.Zone, s)
		s.updateLocalChannels(s.player.Zone)
		s.sendLoadedGroup()
		_ = s.write(uint16(protocol.OpcodeSMSG_INIT_WORLD_STATES), buildInitWorldStates(*s.player, areaID, s.server.Config.ArenaSeasonID, s.server.Config.ArenaSeasonInProgress), true)
		if s.player.Map == WSGMapID {
			// BattlegroundWS::FillInitialWorldStates (BattlegroundWS.cpp:820-858):
			// the live WSG world states follow the generic init block.
			s.server.sendWSGInitWorldStatesTo(s)
		}
		if s.player.Map == EOTSMapID {
			// BattlegroundEY::FillInitialWorldStates (BattlegroundEY.cpp:834-874):
			// the live EotS world states follow the generic init block.
			s.server.sendEOTSInitWorldStatesTo(s)
		}
		if s.player.Map == ABMapID {
			// BattlegroundAB::FillInitialWorldStates (BattlegroundAB.cpp:306-340):
			// the live AB world states follow the generic init block.
			s.server.sendABInitWorldStatesTo(s)
		}
		if s.player.Map == ICMapID {
			// BattlegroundIC::FillInitialWorldStates (BattlegroundIC.cpp:293-305):
			// the live IoC world states follow the generic init block.
			s.server.sendICInitialWorldStates(s)
			// BattlegroundIC::AddPlayer (BattlegroundIC.cpp:249-259): entering the
			// battleground applies the quarry/refinery buffs when the player's team
			// controls those nodes.
			s.server.applyICNodeAuras(s)
		}
	}
	if oldZone != s.player.Zone || oldArea != areaID {
		s.triggerPlayerEvent(ctx, scripting.PlayerEventUpdateZone, s.luaPlayer(), s.player.Zone, areaID)
	}
	if stateChanged || (oldArea != areaID && oldZone == s.player.Zone) {
		s.sendPlayerUpdate()
	}
}

func (s *Server) terrainTile(mapID uint32, tileX, tileY int) []terrainSpawn {
	key := uint64(mapID)<<32 | uint64(tileX&0xFFFF)<<16 | uint64(tileY&0xFFFF)
	s.terrainMu.Lock()
	if s.terrainTileKnown[key] {
		result := s.terrainTiles[key]
		s.terrainMu.Unlock()
		return result
	}
	s.terrainTileKnown[key] = true
	s.terrainMu.Unlock()
	path := filepath.Join(s.Config.GameDataDir, "vmaps", fmt.Sprintf("%03d_%02d_%02d.vmtile", mapID, tileY, tileX))
	data, err := os.ReadFile(path)
	if err != nil || len(data) < 12 || string(data[:8]) != "VMAP_4.7" {
		s.terrainMu.Lock()
		s.terrainTiles[key] = nil
		s.terrainMu.Unlock()
		return nil
	}
	reader := bytes.NewReader(data[8:])
	var count uint32
	if binary.Read(reader, binary.LittleEndian, &count) != nil || count > 1000000 {
		return nil
	}
	spawns := make([]terrainSpawn, 0, count)
	for i := uint32(0); i < count; i++ {
		spawn, spawnErr := readTerrainSpawn(reader)
		if spawnErr != nil {
			spawns = nil
			break
		}
		var nodeIndex uint32
		if binary.Read(reader, binary.LittleEndian, &nodeIndex) != nil {
			spawns = nil
			break
		}
		spawns = append(spawns, spawn)
	}
	s.terrainMu.Lock()
	s.terrainTiles[key] = spawns
	s.terrainMu.Unlock()
	return spawns
}

func readTerrainSpawn(reader *bytes.Reader) (terrainSpawn, error) {
	var spawn terrainSpawn
	if err := binary.Read(reader, binary.LittleEndian, &spawn.Flags); err != nil {
		return spawn, err
	}
	if err := binary.Read(reader, binary.LittleEndian, &spawn.ADTID); err != nil {
		return spawn, err
	}
	if err := binary.Read(reader, binary.LittleEndian, &spawn.ID); err != nil {
		return spawn, err
	}
	if err := binary.Read(reader, binary.LittleEndian, &spawn.Position); err != nil {
		return spawn, err
	}
	if err := binary.Read(reader, binary.LittleEndian, &spawn.Rotation); err != nil {
		return spawn, err
	}
	if err := binary.Read(reader, binary.LittleEndian, &spawn.Scale); err != nil {
		return spawn, err
	}
	if spawn.Flags&terrainModelHasBound != 0 {
		if err := binary.Read(reader, binary.LittleEndian, &spawn.BoundsLow); err != nil {
			return spawn, err
		}
		if err := binary.Read(reader, binary.LittleEndian, &spawn.BoundsHigh); err != nil {
			return spawn, err
		}
	}
	var nameLen uint32
	if err := binary.Read(reader, binary.LittleEndian, &nameLen); err != nil || nameLen > 1024 || uint64(nameLen) > uint64(reader.Len()) {
		return spawn, fmt.Errorf("invalid terrain model name")
	}
	name := make([]byte, nameLen)
	if _, err := io.ReadFull(reader, name); err != nil {
		return spawn, err
	}
	spawn.Name = string(name)
	return spawn, nil
}

func (s *Server) terrainModel(name string) *terrainModel {
	s.terrainMu.Lock()
	if model, found := s.terrainModels[name]; found {
		s.terrainMu.Unlock()
		return model
	}
	s.terrainMu.Unlock()
	model, err := readTerrainModel(filepath.Join(s.Config.GameDataDir, "vmaps", filepath.FromSlash(strings.ReplaceAll(name, "\\", "/"))+".vmo"))
	s.terrainMu.Lock()
	s.terrainModels[name] = model
	s.terrainMu.Unlock()
	if err != nil {
		return nil
	}
	return model
}

func readTerrainModel(path string) (*terrainModel, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	reader := bytes.NewReader(data)
	magic := make([]byte, 8)
	if _, err := io.ReadFull(reader, magic); err != nil || string(magic) != "VMAP_4.7" {
		return nil, fmt.Errorf("invalid terrain model")
	}
	chunk, size, err := readTerrainChunk(reader)
	if err != nil || chunk != "WMOD" || size < 8 {
		return nil, fmt.Errorf("invalid terrain model header")
	}
	var root uint32
	if binary.Read(reader, binary.LittleEndian, &root) != nil {
		return nil, fmt.Errorf("invalid terrain model root")
	}
	model := &terrainModel{Root: root}
	if reader.Len() == 0 {
		return model, nil
	}
	chunk, _, err = readTerrainChunk(reader)
	if err != nil || chunk != "GMOD" {
		return model, nil
	}
	var count uint32
	if binary.Read(reader, binary.LittleEndian, &count) != nil || count > 100000 {
		return nil, fmt.Errorf("invalid terrain group count")
	}
	model.Groups = make([]terrainGroup, count)
	for i := range model.Groups {
		group := &model.Groups[i]
		if binary.Read(reader, binary.LittleEndian, &group.Low) != nil || binary.Read(reader, binary.LittleEndian, &group.High) != nil || binary.Read(reader, binary.LittleEndian, &group.Flags) != nil || binary.Read(reader, binary.LittleEndian, &group.ID) != nil {
			return nil, fmt.Errorf("invalid terrain group header")
		}
		chunk, _, err = readTerrainChunk(reader)
		if err != nil || chunk != "VERT" {
			return nil, fmt.Errorf("invalid terrain vertices")
		}
		var vertices uint32
		if binary.Read(reader, binary.LittleEndian, &vertices) != nil || vertices > 10000000 {
			return nil, fmt.Errorf("invalid terrain vertex count")
		}
		group.Vertices = make([]terrainVector, vertices)
		if binary.Read(reader, binary.LittleEndian, &group.Vertices) != nil {
			return nil, fmt.Errorf("invalid terrain vertices")
		}
		chunk, _, err = readTerrainChunk(reader)
		if err != nil || chunk != "TRIM" {
			return nil, fmt.Errorf("invalid terrain triangles")
		}
		var triangles uint32
		if binary.Read(reader, binary.LittleEndian, &triangles) != nil || triangles > 10000000 {
			return nil, fmt.Errorf("invalid terrain triangle count")
		}
		group.Triangles = make([]terrainTriangle, triangles)
		for index := range group.Triangles {
			if binary.Read(reader, binary.LittleEndian, &group.Triangles[index].A) != nil || binary.Read(reader, binary.LittleEndian, &group.Triangles[index].B) != nil || binary.Read(reader, binary.LittleEndian, &group.Triangles[index].C) != nil {
				return nil, fmt.Errorf("invalid terrain triangle")
			}
		}
		chunk, _, err = readTerrainChunk(reader)
		if err != nil || chunk != "MBIH" || skipTerrainBIH(reader) != nil {
			return nil, fmt.Errorf("invalid terrain mesh index")
		}
		chunk, liquidSize, err := readTerrainChunk(reader)
		if err != nil || chunk != "LIQU" || uint64(liquidSize) > uint64(reader.Len()) {
			return nil, fmt.Errorf("invalid terrain liquid")
		}
		if _, err := reader.Seek(int64(liquidSize), io.SeekCurrent); err != nil {
			return nil, err
		}
	}
	chunk, _, err = readTerrainChunk(reader)
	if err != nil || chunk != "GBIH" || skipTerrainBIH(reader) != nil {
		return nil, fmt.Errorf("invalid terrain group index")
	}
	return model, nil
}

func readTerrainChunk(reader *bytes.Reader) (string, uint32, error) {
	name := make([]byte, 4)
	if _, err := io.ReadFull(reader, name); err != nil {
		return "", 0, err
	}
	var size uint32
	if binary.Read(reader, binary.LittleEndian, &size) != nil {
		return "", 0, io.ErrUnexpectedEOF
	}
	return string(name), size, nil
}

func skipTerrainBIH(reader *bytes.Reader) error {
	var low, high terrainVector
	if binary.Read(reader, binary.LittleEndian, &low) != nil || binary.Read(reader, binary.LittleEndian, &high) != nil {
		return io.ErrUnexpectedEOF
	}
	var treeCount uint32
	if binary.Read(reader, binary.LittleEndian, &treeCount) != nil || treeCount > 100000000 || uint64(treeCount)*4 > uint64(reader.Len()) {
		return fmt.Errorf("invalid terrain tree")
	}
	if _, err := reader.Seek(int64(treeCount)*4, io.SeekCurrent); err != nil {
		return err
	}
	var objectCount uint32
	if binary.Read(reader, binary.LittleEndian, &objectCount) != nil || objectCount > 100000000 || uint64(objectCount)*4 > uint64(reader.Len()) {
		return fmt.Errorf("invalid terrain objects")
	}
	_, err := reader.Seek(int64(objectCount)*4, io.SeekCurrent)
	return err
}

func terrainContains(low, high, value terrainVector) bool {
	return value.X >= low.X && value.X <= high.X && value.Y >= low.Y && value.Y <= high.Y && value.Z >= low.Z && value.Z <= high.Z
}
func terrainSub(a, b terrainVector) terrainVector {
	return terrainVector{a.X - b.X, a.Y - b.Y, a.Z - b.Z}
}
func terrainAdd(a, b terrainVector) terrainVector {
	return terrainVector{a.X + b.X, a.Y + b.Y, a.Z + b.Z}
}
func terrainScale(a terrainVector, scale float32) terrainVector {
	return terrainVector{a.X * scale, a.Y * scale, a.Z * scale}
}
func terrainDot(a, b terrainVector) float64 {
	return float64(a.X)*float64(b.X) + float64(a.Y)*float64(b.Y) + float64(a.Z)*float64(b.Z)
}
func terrainCross(a, b terrainVector) terrainVector {
	return terrainVector{a.Y*b.Z - a.Z*b.Y, a.Z*b.X - a.X*b.Z, a.X*b.Y - a.Y*b.X}
}

func terrainRotation(rotation terrainVector) [3][3]float64 {
	x, y, z := float64(rotation.X)*math.Pi/180, float64(rotation.Y)*math.Pi/180, float64(rotation.Z)*math.Pi/180
	sx, cx, sy, cy, sz, cz := math.Sin(x), math.Cos(x), math.Sin(y), math.Cos(y), math.Sin(z), math.Cos(z)
	return [3][3]float64{{cy * cz, cz*sx*sy - cx*sz, cx*cz*sy + sx*sz}, {cy * sz, cx*cz + sx*sy*sz, -cz*sx + cx*sy*sz}, {-sy, cy * sx, cx * cy}}
}

func terrainModelPoint(spawn terrainSpawn, point terrainVector) terrainVector {
	m := terrainRotation(spawn.Rotation)
	delta := terrainSub(point, spawn.Position)
	scale := spawn.Scale
	if scale == 0 {
		scale = 1
	}
	return terrainVector{float32((m[0][0]*float64(delta.X) + m[1][0]*float64(delta.Y) + m[2][0]*float64(delta.Z)) / float64(scale)), float32((m[0][1]*float64(delta.X) + m[1][1]*float64(delta.Y) + m[2][1]*float64(delta.Z)) / float64(scale)), float32((m[0][2]*float64(delta.X) + m[1][2]*float64(delta.Y) + m[2][2]*float64(delta.Z)) / float64(scale))}
}

func terrainWorldPoint(spawn terrainSpawn, point terrainVector) terrainVector {
	m := terrainRotation(spawn.Rotation)
	scale := spawn.Scale
	if scale == 0 {
		scale = 1
	}
	return terrainAdd(spawn.Position, terrainVector{float32((m[0][0]*float64(point.X) + m[0][1]*float64(point.Y) + m[0][2]*float64(point.Z)) * float64(scale)), float32((m[1][0]*float64(point.X) + m[1][1]*float64(point.Y) + m[1][2]*float64(point.Z)) * float64(scale)), float32((m[2][0]*float64(point.X) + m[2][1]*float64(point.Y) + m[2][2]*float64(point.Z)) * float64(scale))})
}

func terrainTriangleDistance(origin, direction terrainVector, triangle terrainTriangle, vertices []terrainVector, limit float64) (float64, bool) {
	if int(triangle.A) >= len(vertices) || int(triangle.B) >= len(vertices) || int(triangle.C) >= len(vertices) {
		return 0, false
	}
	e1 := terrainSub(vertices[triangle.B], vertices[triangle.A])
	e2 := terrainSub(vertices[triangle.C], vertices[triangle.A])
	p := terrainCross(direction, e2)
	a := terrainDot(e1, p)
	if math.Abs(a) < 1e-5 {
		return 0, false
	}
	f := 1 / a
	u := f * terrainDot(terrainSub(origin, vertices[triangle.A]), p)
	if u < 0 || u > 1 {
		return 0, false
	}
	q := terrainCross(terrainSub(origin, vertices[triangle.A]), e1)
	v := f * terrainDot(direction, q)
	if v < 0 || u+v > 1 {
		return 0, false
	}
	t := f * terrainDot(e2, q)
	return t, t > 0 && t < limit
}

func terrainModelHit(spawn terrainSpawn, model *terrainModel, point terrainVector) (terrainWMOHit, bool) {
	if model == nil || spawn.Flags&terrainModelM2 != 0 || spawn.Flags&terrainModelHasBound == 0 || !terrainContains(spawn.BoundsLow, spawn.BoundsHigh, point) {
		return terrainWMOHit{}, false
	}
	local := terrainModelPoint(spawn, point)
	best := terrainWMOHit{Root: model.Root, ADT: spawn.ADTID, Ground: -float32(math.Inf(1))}
	found := false
	for _, group := range model.Groups {
		if !terrainContains(group.Low, group.High, local) {
			continue
		}
		origin := local
		origin.Z += 0.1
		bestDistance := math.Inf(1)
		for _, triangle := range group.Triangles {
			if distance, ok := terrainTriangleDistance(origin, terrainVector{0, 0, -1}, triangle, group.Vertices, bestDistance); ok {
				bestDistance = distance
			}
		}
		if math.IsInf(bestDistance, 1) {
			continue
		}
		world := terrainWorldPoint(spawn, terrainAdd(origin, terrainVector{0, 0, float32(-bestDistance)}))
		if !found || world.Z > best.Ground {
			best.Group, best.Ground, found = group.ID, world.Z, true
		}
	}
	return best, found
}

func terrainSegmentIntersectsBounds(start, end, low, high terrainVector) bool {
	origin := [3]float64{float64(start.X), float64(start.Y), float64(start.Z)}
	direction := [3]float64{float64(end.X - start.X), float64(end.Y - start.Y), float64(end.Z - start.Z)}
	minimum := [3]float64{float64(low.X), float64(low.Y), float64(low.Z)}
	maximum := [3]float64{float64(high.X), float64(high.Y), float64(high.Z)}
	tMin, tMax := 0.0, 1.0
	for axis := range origin {
		if math.Abs(direction[axis]) < 1e-8 {
			if origin[axis] < minimum[axis] || origin[axis] > maximum[axis] {
				return false
			}
			continue
		}
		t1, t2 := (minimum[axis]-origin[axis])/direction[axis], (maximum[axis]-origin[axis])/direction[axis]
		if t1 > t2 {
			t1, t2 = t2, t1
		}
		if t1 > tMin {
			tMin = t1
		}
		if t2 < tMax {
			tMax = t2
		}
		if tMin > tMax {
			return false
		}
	}
	return tMax >= 0 && tMin <= 1
}

func (s *Server) hasLineOfSight(mapID uint32, x1, y1, z1, x2, y2, z2 float32) bool {
	return s.hasLineOfSightFlags(mapID, x1, y1, z1, x2, y2, z2, false)
}

// hasLineOfSightSpellCheck mirrors the spell CheckCast LOS arms
// (Spell.cpp:5394, 5406-5411): C++ passes VMAP::ModelIgnoreFlags::M2, so
// doodad (M2) geometry — trees, fences, tents — never fails a spell's line
// of sight. Chain-target LOS (Spell.cpp:1955-1973) carries the same flag.
// Creature aggro / assist / melee checks use the default (no-ignore) form.
func (s *Server) hasLineOfSightSpellCheck(mapID uint32, x1, y1, z1, x2, y2, z2 float32) bool {
	return s.hasLineOfSightFlags(mapID, x1, y1, z1, x2, y2, z2, true)
}

func (s *Server) hasLineOfSightFlags(mapID uint32, x1, y1, z1, x2, y2, z2 float32, ignoreM2 bool) bool {
	if s == nil || s.terrainTileKnown == nil || s.terrainTiles == nil || s.terrainModels == nil || x1 == x2 && y1 == y2 && z1 == z2 {
		return true
	}
	gridX1 := int(float64(x1)/terrainGridSize + terrainCenterGrid)
	gridY1 := int(float64(y1)/terrainGridSize + terrainCenterGrid)
	gridX2 := int(float64(x2)/terrainGridSize + terrainCenterGrid)
	gridY2 := int(float64(y2)/terrainGridSize + terrainCenterGrid)
	minX, maxX := min(gridX1, gridX2), max(gridX1, gridX2)
	minY, maxY := min(gridY1, gridY2), max(gridY1, gridY2)
	if minX < 0 || maxX >= 64 || minY < 0 || maxY >= 64 {
		return true
	}
	start := terrainVector{terrainMapHalfSize - x1, terrainMapHalfSize - y1, z1}
	end := terrainVector{terrainMapHalfSize - x2, terrainMapHalfSize - y2, z2}
	for gridX := minX; gridX <= maxX; gridX++ {
		for gridY := minY; gridY <= maxY; gridY++ {
			for _, spawn := range s.terrainTile(mapID, 63-gridX, 63-gridY) {
				// Spell CheckCast LOS ignores M2 doodad models
				// (VMAP::ModelIgnoreFlags::M2, Spell.cpp:5394, 5406-5411);
				// the default (aggro/assist/melee) form keeps them.
				if spawn.Flags&terrainModelHasBound == 0 || (ignoreM2 && spawn.Flags&terrainModelM2 != 0) || !terrainSegmentIntersectsBounds(start, end, spawn.BoundsLow, spawn.BoundsHigh) {
					continue
				}
				model := s.terrainModel(spawn.Name)
				if model == nil {
					continue
				}
				localStart, localEnd := terrainModelPoint(spawn, start), terrainModelPoint(spawn, end)
				direction := terrainSub(localEnd, localStart)
				for _, group := range model.Groups {
					if !terrainSegmentIntersectsBounds(localStart, localEnd, group.Low, group.High) {
						continue
					}
					for _, triangle := range group.Triangles {
						if distance, ok := terrainTriangleDistance(localStart, direction, triangle, group.Vertices, 1); ok && distance > 0.001 && distance < 0.999 {
							return false
						}
					}
				}
			}
		}
	}
	return true
}

func (s *Server) mapWMOAreaID(mapID uint32, x, y, z float32) (uint32, bool) {
	tileX := int(float64(x)/terrainGridSize + terrainCenterGrid)
	tileY := int(float64(y)/terrainGridSize + terrainCenterGrid)
	if tileX < 0 || tileX >= 64 || tileY < 0 || tileY >= 64 {
		return 0, false
	}
	point := terrainVector{terrainMapHalfSize - x, terrainMapHalfSize - y, z}
	best := terrainWMOHit{Ground: -float32(math.Inf(1))}
	found := false
	for _, spawn := range s.terrainTile(mapID, 63-tileX, 63-tileY) {
		if spawn.Flags&terrainModelM2 != 0 || spawn.Flags&terrainModelHasBound == 0 {
			continue
		}
		model := s.terrainModel(spawn.Name)
		hit, ok := terrainModelHit(spawn, model, point)
		if ok && (!found || hit.Ground > best.Ground) {
			best, found = hit, true
		}
	}
	if !found || z < best.Ground-2 {
		return 0, false
	}
	areaID, ok, err := s.Data.WMOArea(int32(best.Root), int32(best.ADT), int32(best.Group))
	return areaID, ok && err == nil
}

func (s *Server) mapAreaID(mapID uint32, x, y float32) (uint32, bool) {
	gridX := int(float64(x)/terrainGridSize + terrainCenterGrid)
	gridY := int(float64(y)/terrainGridSize + terrainCenterGrid)
	if gridX < 0 || gridX >= 64 || gridY < 0 || gridY >= 64 {
		return 0, false
	}
	path := filepath.Join(s.Config.GameDataDir, "maps", fmt.Sprintf("%03d%02d%02d.map", mapID, 63-gridX, 63-gridY))
	file, err := os.Open(path)
	if err != nil {
		return 0, false
	}
	defer file.Close()
	header := make([]byte, 44)
	if _, err := io.ReadFull(file, header); err != nil || string(header[:4]) != "MAPS" || string(header[4:8]) != "v1.9" {
		return 0, false
	}
	areaOffset := binary.LittleEndian.Uint32(header[12:16])
	if _, err := file.Seek(int64(areaOffset), io.SeekStart); err != nil {
		return 0, false
	}
	areaHeader := make([]byte, 8)
	if _, err := io.ReadFull(file, areaHeader); err != nil || string(areaHeader[:4]) != "AREA" {
		return 0, false
	}
	flags := binary.LittleEndian.Uint16(areaHeader[4:6])
	gridArea := uint32(binary.LittleEndian.Uint16(areaHeader[6:8]))
	if flags&terrainAreaNoArea != 0 {
		return gridArea, gridArea != 0
	}
	areas := make([]byte, 16*16*2)
	if _, err := io.ReadFull(file, areas); err != nil {
		return 0, false
	}
	localX := int(16*(terrainCenterGrid-float64(x)/terrainGridSize)) & 15
	localY := int(16*(terrainCenterGrid-float64(y)/terrainGridSize)) & 15
	index := (localX*16 + localY) * 2
	areaID := uint32(binary.LittleEndian.Uint16(areas[index : index+2]))
	return areaID, areaID != 0
}
