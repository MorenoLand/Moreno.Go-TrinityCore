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
	// terrainGroundTolerance is GROUND_HEIGHT_TOLERANCE (SharedDefines.h:26).
	terrainGroundTolerance = float32(0.05)
	// terrainDefaultHeightSearch is DEFAULT_HEIGHT_SEARCH (Map.h:287).
	terrainDefaultHeightSearch = float32(50)
	// terrainMaxHeight is MAX_HEIGHT (Map.h:284).
	terrainMaxHeight = float32(100000)
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
		// Player::DestroyZoneLimitedItem (Player.cpp:12920) destroys each
		// out-of-zone item via DestroyItem, so OnItemRemove fires per item.
		s.fireItemRemoveHook(ctx, item.guid)
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
		// GetData, which resolves through the graveyard, not the workshop
		// (BattlefieldWG.cpp:1134-1149): GetSpiritGraveyardId maps the
		// area to a graveyard, and the graveyard's control is the crossed
		// workshop's team -- workshop GiveControlTo flips GY[WorkshopId]
		// (BattlefieldWG.cpp:1800-1801), so the Eastpark area reads the
		// Sunken Ring workshop's team, not Eastspark's. Reading workshop
		// control directly inverts all four areas.
		gy, ok := wgSpiritGraveyardID(areaID)
		if !ok {
			return false
		}
		wg.mu.Lock()
		team := wg.Graveyards[gy].TeamControl
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

// terrainBoundedModelHit mirrors StaticMapTree::getHeight (MapTree.cpp:225-236)
// for one spawn: the downward ray from point, bounded by maxSearchDist, with
// the nearest hit (highest world Z) winning. It differs from terrainModelHit
// in three C++-dictated ways: the ray starts exactly at point (no +0.1 lift,
// so a surface above point never hits), groups are tested by segment-bounds
// overlap rather than point containment (a floor below point is a hit), and
// M2 doodad spawns count (getHeight passes ModelIgnoreFlags::Nothing, unlike
// the WMO-interior area query). The local (0,0,-1) ray follows the
// terrainModelHit precedent: exact for yaw-only spawns, approximate for
// pitched ones, with the world-Z gate below enforcing the C++ bounded ray
// in world units either way.
func terrainBoundedModelHit(spawn terrainSpawn, model *terrainModel, point terrainVector, maxSearchDist float32) (float32, bool) {
	if model == nil || spawn.Flags&terrainModelHasBound == 0 {
		return 0, false
	}
	end := terrainVector{point.X, point.Y, point.Z - maxSearchDist}
	if !terrainSegmentIntersectsBounds(point, end, spawn.BoundsLow, spawn.BoundsHigh) {
		return 0, false
	}
	scale := spawn.Scale
	if scale == 0 {
		scale = 1
	}
	local := terrainModelPoint(spawn, point)
	localLimit := float64(maxSearchDist) / float64(scale)
	localEnd := terrainVector{local.X, local.Y, local.Z - float32(localLimit)}
	best, found := float32(0), false
	for _, group := range model.Groups {
		if !terrainSegmentIntersectsBounds(local, localEnd, group.Low, group.High) {
			continue
		}
		bestDistance, hit := localLimit, false
		for _, triangle := range group.Triangles {
			if distance, ok := terrainTriangleDistance(local, terrainVector{0, 0, -1}, triangle, group.Vertices, bestDistance); ok {
				bestDistance, hit = distance, true
			}
		}
		if !hit {
			continue
		}
		world := terrainWorldPoint(spawn, terrainVector{local.X, local.Y, local.Z - float32(bestDistance)})
		if !found || world.Z > best {
			best, found = world.Z, true
		}
	}
	if !found || best > point.Z || best < point.Z-maxSearchDist {
		return 0, false
	}
	return best, true
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

// mapVMapHeight mirrors VMapManager2::getHeight (VMapManager2.cpp:217-237) over
// the single tile's static tree: the bounded from-z ray across every spawn
// (WMO and M2 alike), nearest hit winning. No height-calc toggle exists in
// Go; an absent tile file reads no hit, the same as a disabled manager.
// The phasemask overload's dynamic-tree leg (GetGameObjectFloor, Map.h:576)
// is not modeled: Go has no dynamic GO collision tree.
func (s *Server) mapVMapHeight(mapID uint32, x, y, z, maxSearchDist float32) (float32, bool) {
	tileX := int(float64(x)/terrainGridSize + terrainCenterGrid)
	tileY := int(float64(y)/terrainGridSize + terrainCenterGrid)
	if tileX < 0 || tileX >= 64 || tileY < 0 || tileY >= 64 {
		return 0, false
	}
	point := terrainVector{terrainMapHalfSize - x, terrainMapHalfSize - y, z}
	best, found := float32(0), false
	for _, spawn := range s.terrainTile(mapID, 63-tileX, 63-tileY) {
		if height, ok := terrainBoundedModelHit(spawn, s.terrainModel(spawn.Name), point, maxSearchDist); ok && (!found || height > best) {
			best, found = height, true
		}
	}
	return best, found
}

// mapHeight mirrors Map::GetHeight (Map.cpp:2489-2527), the engine-internal
// form: the .map grid height counts only when z sits no more than the ground
// tolerance below it, the vmap ray counts only within maxSearchDist below z,
// and the selection prefers the vmap height when it is above the grid height
// or nearer to z. The G3D fuzzy epsilon on the tolerance gate is ignored
// (sub-millimeter against the 0.05 tolerance). The second return is false
// for the INVALID_HEIGHT sentinel, which callers must not use as a Z.
func (s *Server) mapHeight(mapID uint32, x, y, z float32, checkVMap bool, maxSearchDist float32) (float32, bool) {
	gridHeight, gridOK := s.mapGridHeight(mapID, x, y)
	mapHeight, mapOK := float32(0), false
	if gridOK && z >= gridHeight-terrainGroundTolerance {
		mapHeight, mapOK = gridHeight, true
	}
	if checkVMap {
		if vmapHeight, ok := s.mapVMapHeight(mapID, x, y, z, maxSearchDist); ok {
			if mapOK {
				if vmapHeight > mapHeight || math.Abs(float64(mapHeight-z)) > math.Abs(float64(vmapHeight-z)) {
					return vmapHeight, true
				}
				return mapHeight, true
			}
			return vmapHeight, true
		}
	}
	return mapHeight, mapOK
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

// terrainInvalidHeight is INVALID_HEIGHT (Map.h:285), the sentinel
// GridMap::getHeight returns for a missing tile, a hole, or a flat tile
// whose extractor wrote no height; Eluna Map:GetHeight pushes nil for it.
const terrainInvalidHeight = float32(-100000)

const (
	terrainHeightNoHeight = 0x0001
	terrainHeightAsInt16  = 0x0002
	terrainHeightAsInt8   = 0x0004
)

const (
	terrainLiquidNoType   = 0x0001
	terrainLiquidNoHeight = 0x0002
)

// ZLiquidStatus values (Map.h:145-149) and MAP_LIQUID_TYPE_* bits
// (Map.h:155-163) for GridMap::GetLiquidStatus.
const (
	liquidStatusNoWater    uint32 = 0x00000000
	liquidStatusAboveWater uint32 = 0x00000001
	liquidStatusWaterWalk  uint32 = 0x00000002
	liquidStatusInWater    uint32 = 0x00000004
	liquidStatusUnderWater uint32 = 0x00000008

	liquidTypeWater     uint8 = 0x01
	liquidTypeOcean     uint8 = 0x02
	liquidTypeMagma     uint8 = 0x04
	liquidTypeSlime     uint8 = 0x08
	liquidTypeAll       uint8 = 0x0F
	liquidTypeDarkWater uint8 = 0x10
)

// terrainDefaultCollisionHeight is DEFAULT_COLLISION_HEIGHT (Object.h:69),
// the collision height Map::GetLiquidStatus callers pass when they have no
// unit-specific value.
const terrainDefaultCollisionHeight = float32(2.03128)

var terrainHoleTabH = [4]uint16{0x1111, 0x2222, 0x4444, 0x8888}
var terrainHoleTabV = [4]uint16{0x000F, 0x00F0, 0x0F00, 0xF000}

// mapGridHeight is GridMap::getHeight (Map.cpp:2070-2295) over the .map
// v1.9 MHGT chunk: the 44-byte file header carries the height chunk at
// [20:24] (map_fileheader, Map.h:90), the chunk header is fourcc MHGT +
// flags + gridHeight + gridMaxHeight (map_heightHeader, Map.h:119), and
// the V9 (129x129) / V8 (128x128) arrays follow in float, uint16, or
// uint8 form selected by the flags (checked in the C++ order: int16,
// then int8, then float). A zero height offset means no height data
// and reads invalid, the same as a missing tile file; any short or
// mis-magic read also reads invalid, matching loadData failing the
// whole grid back to the flat-INVALID default. The holes chunk (256
// uint16 at the header holes offset, read only when holesSize is set,
// exactly as loadData gates it) makes its cells read invalid.
func (s *Server) mapGridHeight(mapID uint32, x, y float32) (float32, bool) {
	if s == nil {
		return 0, false
	}
	gridX := int(float64(x)/terrainGridSize + terrainCenterGrid)
	gridY := int(float64(y)/terrainGridSize + terrainCenterGrid)
	if gridX < 0 || gridX >= 64 || gridY < 0 || gridY >= 64 {
		return 0, false
	}
	path := filepath.Join(s.Config.GameDataDir, "maps", fmt.Sprintf("%03d%02d%02d.map", mapID, 63-gridX, 63-gridY))
	data, err := os.ReadFile(path)
	if err != nil || len(data) < 44 || string(data[:4]) != "MAPS" || string(data[4:8]) != "v1.9" {
		return 0, false
	}
	heightOffset := binary.LittleEndian.Uint32(data[20:24])
	if heightOffset == 0 || int(heightOffset)+16 > len(data) || string(data[heightOffset:heightOffset+4]) != "MHGT" {
		return 0, false
	}
	flags := binary.LittleEndian.Uint32(data[heightOffset+4 : heightOffset+8])
	gridHeight := math.Float32frombits(binary.LittleEndian.Uint32(data[heightOffset+8 : heightOffset+12]))
	gridMaxHeight := math.Float32frombits(binary.LittleEndian.Uint32(data[heightOffset+12 : heightOffset+16]))
	if flags&terrainHeightNoHeight != 0 {
		return gridHeight, true
	}
	pos := int(heightOffset) + 16
	var v9, v8 func(int) float32
	scale, offset := float32(1), float32(0)
	switch {
	case flags&terrainHeightAsInt16 != 0:
		if pos+129*129*2+128*128*2 > len(data) {
			return 0, false
		}
		raw9 := make([]uint16, 129*129)
		for i := range raw9 {
			raw9[i] = binary.LittleEndian.Uint16(data[pos+i*2:])
		}
		pos += 129 * 129 * 2
		raw8 := make([]uint16, 128*128)
		for i := range raw8 {
			raw8[i] = binary.LittleEndian.Uint16(data[pos+i*2:])
		}
		v9 = func(i int) float32 { return float32(raw9[i]) }
		v8 = func(i int) float32 { return float32(raw8[i]) }
		scale, offset = (gridMaxHeight-gridHeight)/65535, gridHeight
	case flags&terrainHeightAsInt8 != 0:
		if pos+129*129+128*128 > len(data) {
			return 0, false
		}
		raw9 := make([]uint8, 129*129)
		copy(raw9, data[pos:pos+129*129])
		pos += 129 * 129
		raw8 := make([]uint8, 128*128)
		copy(raw8, data[pos:pos+128*128])
		v9 = func(i int) float32 { return float32(raw9[i]) }
		v8 = func(i int) float32 { return float32(raw8[i]) }
		scale, offset = (gridMaxHeight-gridHeight)/255, gridHeight
	default:
		if pos+129*129*4+128*128*4 > len(data) {
			return 0, false
		}
		raw9 := make([]float32, 129*129)
		for i := range raw9 {
			raw9[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[pos+i*4:]))
		}
		pos += 129 * 129 * 4
		raw8 := make([]float32, 128*128)
		for i := range raw8 {
			raw8[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[pos+i*4:]))
		}
		v9 = func(i int) float32 { return raw9[i] }
		v8 = func(i int) float32 { return raw8[i] }
	}
	var holes []uint16
	if binary.LittleEndian.Uint32(data[40:44]) != 0 {
		holesOffset := binary.LittleEndian.Uint32(data[36:40])
		if int(holesOffset)+512 > len(data) {
			return 0, false
		}
		holes = make([]uint16, 256)
		for i := range holes {
			holes[i] = binary.LittleEndian.Uint16(data[int(holesOffset)+i*2:])
		}
	}
	return terrainInterpolateGridHeight(v9, v8, holes, x, y)*scale + offset, true
}

// mapWaterLevel is Map::GetWaterLevel (Map.cpp:2879-2884) over the .map
// v1.9 MLIQ chunk: the 44-byte file header carries the liquid chunk at
// [28:32] (map_fileheader, Map.h:90), the chunk header is fourcc MLIQ +
// flags + liquidFlags + liquidType + offsetX + offsetY + width + height +
// liquidLevel (map_liquidHeader, Map.h:130), followed by the 256-entry
// type arrays unless MAP_LIQUID_NO_TYPE and the width*height float level
// map unless MAP_LIQUID_NO_HEIGHT (loadLiquidData, Map.cpp:2007-2040).
// The second return is false only when the tile file itself is missing,
// the C++ grid-not-loaded arm where GetWaterLevel returns 0; every other
// failure reads the INVALID_HEIGHT sentinel, matching a loadData failure
// leaving the default-constructed grid in place (Map.cpp:237-240) and a
// missing liquid chunk leaving _liquidMap nil (getLiquidLevel,
// Map.cpp:2340-2343 returns the global level, itself INVALID by default).
// A present-but-corrupt height header also reads INVALID, since the
// height load runs before the liquid load and its failure aborts the
// whole grid; a short height array is the one unmodeled corner there.
// The cell math is getLiquidLevel verbatim: the grid-local coordinates
// are MAP_RESOLUTION * (CENTER_GRID_ID - coord / SIZE_OF_GRIDS), masked
// to the tile, then shifted by the liquid offsets, and an out-of-rect
// cell reads INVALID. Liquid type and flag arrays are not consulted:
// getLiquidLevel reads levels only, unlike GetLiquidStatus.
func (s *Server) mapWaterLevel(mapID uint32, x, y float32) (float32, bool) {
	if s == nil {
		return 0, false
	}
	gridX := int(float64(x)/terrainGridSize + terrainCenterGrid)
	gridY := int(float64(y)/terrainGridSize + terrainCenterGrid)
	if gridX < 0 || gridX >= 64 || gridY < 0 || gridY >= 64 {
		return 0, false
	}
	path := filepath.Join(s.Config.GameDataDir, "maps", fmt.Sprintf("%03d%02d%02d.map", mapID, 63-gridX, 63-gridY))
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	if len(data) < 44 || string(data[:4]) != "MAPS" || string(data[4:8]) != "v1.9" {
		return terrainInvalidHeight, true
	}
	heightOffset := binary.LittleEndian.Uint32(data[20:24])
	if heightOffset != 0 && (int(heightOffset)+16 > len(data) || string(data[heightOffset:heightOffset+4]) != "MHGT") {
		return terrainInvalidHeight, true
	}
	liquidOffset := binary.LittleEndian.Uint32(data[28:32])
	if liquidOffset == 0 {
		return terrainInvalidHeight, true
	}
	if int(liquidOffset)+16 > len(data) || string(data[liquidOffset:liquidOffset+4]) != "MLIQ" {
		return terrainInvalidHeight, true
	}
	flags := data[liquidOffset+4]
	offX := data[liquidOffset+8]
	offY := data[liquidOffset+9]
	width := data[liquidOffset+10]
	height := data[liquidOffset+11]
	liquidLevel := math.Float32frombits(binary.LittleEndian.Uint32(data[liquidOffset+12 : liquidOffset+16]))
	pos := int(liquidOffset) + 16
	if flags&terrainLiquidNoType == 0 {
		if pos+256*2+256 > len(data) {
			return terrainInvalidHeight, true
		}
		pos += 256*2 + 256
	}
	if flags&terrainLiquidNoHeight != 0 {
		return liquidLevel, true
	}
	if pos+int(width)*int(height)*4 > len(data) {
		return terrainInvalidHeight, true
	}
	fx := float32(128) * (32 - x/533.3333)
	fy := float32(128) * (32 - y/533.3333)
	cxInt := int32(fx)&127 - int32(offY)
	cyInt := int32(fy)&127 - int32(offX)
	if cxInt < 0 || cxInt >= int32(height) {
		return terrainInvalidHeight, true
	}
	if cyInt < 0 || cyInt >= int32(width) {
		return terrainInvalidHeight, true
	}
	cell := pos + (int(cxInt)*int(width)+int(cyInt))*4
	return math.Float32frombits(binary.LittleEndian.Uint32(data[cell : cell+4])), true
}

// liquidData mirrors LiquidData (Map.h): the entry id, the computed
// type_flags, the liquid surface level, and the ground level under it.
type liquidData struct {
	entry      uint32
	typeFlags  uint8
	level      float32
	depthLevel float32
}

// mapGridLiquidStatus is the grid leg of Map::GetLiquidStatus
// (Map.cpp:2649-2740 via GridMap::GetLiquidStatus, Map.cpp:2360-2446) over
// the .map v1.9 MLIQ chunk: the per-cell entry (uint16) and flags (uint8)
// arrays, or the header liquidType/liquidFlags globals when
// MAP_LIQUID_NO_TYPE is set; the SoundBank-derived type bit plus the
// area LiquidTypeID override (entry < 21, parent-area fallback) when the
// DBC store resolves the entry; the requested-type mask; the level map
// or the header liquidLevel when MAP_LIQUID_NO_HEIGHT is set; and the
// ground gate against the grid height, with the delta thresholds
// (UNDER above collisionHeight, IN above 0, WATER_WALK above -0.1,
// otherwise ABOVE_WATER). A missing tile file, a missing liquid chunk,
// or a corrupt height chunk reads NO_WATER: in C++ any of those leaves
// the grid default-constructed, so the no-liquid-flags arm fires first.
// Deliberately NOT the Map-level function: the vmap leg (VMO liquid,
// checked first in C++, and skipping grid liquid inside WMO interiors)
// has no Go reader, so callers get the grid leg only; indoor pools and
// WMO water read NO_WATER here where C++ reads the vmap level. A
// missing DBC store or an unresolvable entry keeps the raw per-cell
// flags byte, the C++ unresolved-entry arm; the area override is
// skipped when the area map cannot be read, where C++ would still
// apply the tile's grid-area override.
func (s *Server) mapGridLiquidStatus(mapID uint32, x, y, z float32, reqLiquidType uint8, collisionHeight float32) (uint32, liquidData) {
	if s == nil {
		return liquidStatusNoWater, liquidData{}
	}
	gridX := int(float64(x)/terrainGridSize + terrainCenterGrid)
	gridY := int(float64(y)/terrainGridSize + terrainCenterGrid)
	if gridX < 0 || gridX >= 64 || gridY < 0 || gridY >= 64 {
		return liquidStatusNoWater, liquidData{}
	}
	path := filepath.Join(s.Config.GameDataDir, "maps", fmt.Sprintf("%03d%02d%02d.map", mapID, 63-gridX, 63-gridY))
	data, err := os.ReadFile(path)
	if err != nil || len(data) < 44 || string(data[:4]) != "MAPS" || string(data[4:8]) != "v1.9" {
		return liquidStatusNoWater, liquidData{}
	}
	heightOffset := binary.LittleEndian.Uint32(data[20:24])
	if heightOffset != 0 && (int(heightOffset)+16 > len(data) || string(data[heightOffset:heightOffset+4]) != "MHGT") {
		return liquidStatusNoWater, liquidData{}
	}
	liquidOffset := binary.LittleEndian.Uint32(data[28:32])
	if liquidOffset == 0 || int(liquidOffset)+16 > len(data) || string(data[liquidOffset:liquidOffset+4]) != "MLIQ" {
		return liquidStatusNoWater, liquidData{}
	}
	flags := data[liquidOffset+4]
	globalFlags := data[liquidOffset+5]
	globalEntry := uint32(binary.LittleEndian.Uint16(data[liquidOffset+6 : liquidOffset+8]))
	offX := data[liquidOffset+8]
	offY := data[liquidOffset+9]
	width := data[liquidOffset+10]
	height := data[liquidOffset+11]
	globalLevel := math.Float32frombits(binary.LittleEndian.Uint32(data[liquidOffset+12 : liquidOffset+16]))
	pos := int(liquidOffset) + 16
	var entryArr []uint16
	var flagsArr []uint8
	if flags&terrainLiquidNoType == 0 {
		if pos+256*2+256 > len(data) {
			return liquidStatusNoWater, liquidData{}
		}
		entryArr = make([]uint16, 256)
		for i := range entryArr {
			entryArr[i] = binary.LittleEndian.Uint16(data[pos+i*2:])
		}
		flagsArr = make([]uint8, 256)
		copy(flagsArr, data[pos+512:pos+512+256])
		pos += 256*2 + 256
	}
	if globalFlags == 0 && flagsArr == nil {
		return liquidStatusNoWater, liquidData{}
	}
	fx := float32(128) * (32 - x/533.3333)
	fy := float32(128) * (32 - y/533.3333)
	xInt := int32(fx) & 127
	yInt := int32(fy) & 127
	idx := (xInt>>3)*16 + (yInt >> 3)
	rawType := globalFlags
	entry := globalEntry
	if flagsArr != nil {
		rawType = flagsArr[idx]
		entry = uint32(entryArr[idx])
	}
	liquidType := rawType
	if s.Data != nil {
		if soundBank, ok := s.Data.LiquidTypeSoundBank(entry); ok {
			liquidType = rawType & liquidTypeDarkWater
			if entry < 21 && soundBank < 4 {
				if areaID, ok := s.mapAreaID(mapID, x, y); ok {
					if override := liquidTypeOverride(s, areaID, soundBank); override != 0 {
						if overrideBank, ok := s.Data.LiquidTypeSoundBank(override); ok {
							entry = override
							soundBank = overrideBank
						}
					}
				}
			}
			liquidType |= uint8(uint32(1) << soundBank)
		}
	}
	if liquidType == 0 {
		return liquidStatusNoWater, liquidData{}
	}
	if reqLiquidType != 0 && reqLiquidType&liquidType == 0 {
		return liquidStatusNoWater, liquidData{}
	}
	lxInt := xInt - int32(offY)
	lyInt := yInt - int32(offX)
	if lxInt < 0 || lxInt >= int32(height) || lyInt < 0 || lyInt >= int32(width) {
		return liquidStatusNoWater, liquidData{}
	}
	var liquidLevel float32
	if flags&terrainLiquidNoHeight != 0 {
		liquidLevel = globalLevel
	} else {
		if pos+int(width)*int(height)*4 > len(data) {
			return liquidStatusNoWater, liquidData{}
		}
		cell := pos + (int(lxInt)*int(width)+int(lyInt))*4
		liquidLevel = math.Float32frombits(binary.LittleEndian.Uint32(data[cell : cell+4]))
	}
	var ground float32
	if heightOffset == 0 {
		ground = 0
	} else {
		var ok bool
		ground, ok = s.mapGridHeight(mapID, x, y)
		if !ok {
			return liquidStatusNoWater, liquidData{}
		}
	}
	if liquidLevel < ground || z < ground {
		return liquidStatusNoWater, liquidData{}
	}
	out := liquidData{entry: entry, typeFlags: liquidType, level: liquidLevel, depthLevel: ground}
	delta := liquidLevel - z
	switch {
	case delta > collisionHeight:
		return liquidStatusUnderWater, out
	case delta > 0:
		return liquidStatusInWater, out
	case delta > -0.1:
		return liquidStatusWaterWalk, out
	default:
		return liquidStatusAboveWater, out
	}
}

// liquidTypeOverride is the AreaTable LiquidTypeID leg of
// GridMap::GetLiquidStatus (Map.cpp:2386-2400): the area's override for
// the entry's SoundBank slot, falling back to the parent area when the
// area override is zero. Zero means no override.
func liquidTypeOverride(s *Server, areaID uint32, soundBank uint32) uint32 {
	if s == nil || s.Data == nil {
		return 0
	}
	area, found, err := s.Data.Area(areaID)
	if err != nil || !found {
		return 0
	}
	if override := area.LiquidTypeID[soundBank]; override != 0 {
		return override
	}
	if area.ParentAreaID == 0 {
		return 0
	}
	parent, found, err := s.Data.Area(area.ParentAreaID)
	if err != nil || !found {
		return 0
	}
	return parent.LiquidTypeID[soundBank]
}

// mapIsInWater mirrors Map::IsInWater (Map.cpp:2916-2921): the grid-leg
// liquid status over all liquid types, in or under water.
func (s *Server) mapIsInWater(mapID uint32, x, y, z float32) bool {
	status, _ := s.mapGridLiquidStatus(mapID, x, y, z, liquidTypeAll, terrainDefaultCollisionHeight)
	return status&(liquidStatusInWater|liquidStatusUnderWater) != 0
}

// mapIsUnderWater mirrors Map::IsUnderWater (Map.cpp:2923-2926): the
// grid-leg liquid status over water and ocean only, under water.
func (s *Server) mapIsUnderWater(mapID uint32, x, y, z float32) bool {
	status, _ := s.mapGridLiquidStatus(mapID, x, y, z, liquidTypeWater|liquidTypeOcean, terrainDefaultCollisionHeight)
	return status&liquidStatusUnderWater != 0
}

// terrainLiquidCellKey packs the tile and the 8x8-cell liquid group a
// position falls in (the (x>>3)*16+(y>>3) index of
// GridMap::GetLiquidStatus) into one change-detection value for the
// dark-water re-evaluation in movement handling.
func terrainLiquidCellKey(x, y float32) int64 {
	fx := float32(128) * (32 - x/533.3333)
	fy := float32(128) * (32 - y/533.3333)
	gridX := int64(float64(x)/terrainGridSize + terrainCenterGrid)
	gridY := int64(float64(y)/terrainGridSize + terrainCenterGrid)
	return gridX | gridY<<6 | int64(int32(fx)&127>>3)<<12 | int64(int32(fy)&127>>3)<<16
}

// terrainInterpolateGridHeight is GridMap::getHeightFromFloat (and the
// shared triangle selection of the uint8/uint16 twins, Map.cpp:2070+):
// the grid-local coordinates are MAP_RESOLUTION * (CENTER_GRID_ID -
// coord / SIZE_OF_GRIDS), masked to the tile, and the height is the
// plane through the cell's V9 corners and its doubled V8 center. The
// int legs feed raw values here and scale the result in mapGridHeight,
// which equals the C++ (a*x + b*y + c) * multiplier + gridHeight order
// because the int values fit float32 exactly. A hole cell reads the
// invalid sentinel, the same as C++ isHole returning INVALID_HEIGHT.
func terrainInterpolateGridHeight(v9, v8 func(int) float32, holes []uint16, x, y float32) float32 {
	fx := float32(128) * (32 - x/533.3333)
	fy := float32(128) * (32 - y/533.3333)
	xInt := int32(fx) & 127
	yInt := int32(fy) & 127
	fracX := fx - float32(int32(fx))
	fracY := fy - float32(int32(fy))
	if holes != nil {
		cellRow, cellCol := xInt/8, yInt/8
		holeRow, holeCol := (xInt%8)/2, (yInt-cellCol*8)/2
		hole := holes[cellRow*16+cellCol]
		if hole&terrainHoleTabH[holeCol]&terrainHoleTabV[holeRow] != 0 {
			return terrainInvalidHeight
		}
	}
	var a, b, c float32
	if fracX+fracY < 1 {
		if fracX > fracY {
			h1 := v9(int(xInt)*129 + int(yInt))
			h2 := v9(int(xInt+1)*129 + int(yInt))
			h5 := 2 * v8(int(xInt)*128+int(yInt))
			a, b, c = h2-h1, h5-h1-h2, h1
		} else {
			h1 := v9(int(xInt)*129 + int(yInt))
			h3 := v9(int(xInt)*129 + int(yInt+1))
			h5 := 2 * v8(int(xInt)*128+int(yInt))
			a, b, c = h5-h1-h3, h3-h1, h1
		}
	} else {
		if fracX > fracY {
			h2 := v9(int(xInt+1)*129 + int(yInt))
			h4 := v9(int(xInt+1)*129 + int(yInt+1))
			h5 := 2 * v8(int(xInt)*128+int(yInt))
			a, b, c = h2+h4-h5, h4-h2, h5-h4
		} else {
			h3 := v9(int(xInt)*129 + int(yInt+1))
			h4 := v9(int(xInt+1)*129 + int(yInt+1))
			h5 := 2 * v8(int(xInt)*128+int(yInt))
			a, b, c = h4-h3, h3+h4-h5, h5-h4
		}
	}
	return a*fracX + b*fracY + c
}
