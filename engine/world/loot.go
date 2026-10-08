package world

import (
	"context"
	"database/sql"
	"errors"
	"math/rand"
	"sort"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

type lootItem struct {
	Slot          uint8
	ItemEntry     uint32
	Count         uint32
	DisplayInfoID uint32
	Quality       uint32
	IsBlocked     bool
	RollWinner    uint64
	// NeedsQuest mirrors LootItem::needs_quest (Loot.h:140): the
	// creature/gameobject_loot_template QuestRequired column. Quest-required
	// rows roll into the separate QuestItems list, never the normal Items,
	// and are only shown to viewers whose quest state allows them
	// (LootItem::AllowedForPlayer, Loot.cpp:57-95).
	NeedsQuest  bool
	StartQuest  uint32
	CustomFlags uint32
}

// itemFlagsCuIgnoreQuestStatus / itemFlagsCuFollowLootRules mirror
// ITEM_FLAGS_CU_IGNORE_QUEST_STATUS and ITEM_FLAGS_CU_FOLLOW_LOOT_RULES
// (ItemTemplate.h:225-226).
const (
	itemFlagsCuIgnoreQuestStatus uint32 = 0x0002
	itemFlagsCuFollowLootRules   uint32 = 0x0004
)

// lootTypeSkinning mirrors LOOT_SKINNING (Loot.h:89).
const lootTypeSkinning uint8 = 6

// lootTypeCorpse mirrors LOOT_CORPSE (Loot.h:82): the loot type Go stores on
// creature and gameobject loot opened via the CMSG_LOOT path.
const lootTypeCorpse uint8 = 1

// lootTypePickpocketing mirrors LOOT_PICKPOCKETING (Loot.h:83).
const lootTypePickpocketing uint8 = 2

// lootErrorAlreadyPickpocketed mirrors LOOT_ERROR_ALREADY_PICKPOCKETED
// (Loot.h:113): "Your target has already had its pockets picked".
const lootErrorAlreadyPickpocketed uint8 = 15

// pickpocketRefillSeconds mirrors CONFIG_CREATURE_PICKPOCKET_REFILL's default
// (World.cpp:1229): Creature.PickPocketRefillDelay = 10 * MINUTE. Go reads no
// world-int config, so the C++ default is the constant.
const pickpocketRefillSeconds = 600

// maxQuestLootItems mirrors MAX_NR_QUEST_ITEMS (Loot.h:57).
const maxQuestLootItems = 32

// maxNormalLootItems mirrors MAX_NR_LOOT_ITEMS (Loot.h:55): Loot::AddItem
// caps the normal items vector at 18 rows.
const maxNormalLootItems = 18

// lootModeDefault mirrors LOOT_MODE_DEFAULT (SharedDefines.h:42): the loot
// mode every Go template fill passes, since Go models no creature loot modes
// (C++ passes creature->GetLootMode(), Unit.cpp:11250).
const lootModeDefault = 1

type activeLootState struct {
	TargetGUID uint64
	MapID      uint32
	InstanceID uint32
	LootType   uint8
	Money      uint32
	Items      map[uint8]lootItem
	// QuestItems mirrors Loot::quest_items (Loot.h): quest-required rolls live
	// in their own index space. A viewer sees them at display slot
	// NormalSlotCount + position-in-their-quest-list (Loot.cpp:704).
	QuestItems map[uint8]lootItem
	// NormalSlotCount mirrors Loot::items.size(): the constant display-slot
	// base for quest items (C++ items.size() never shrinks on take; the Go
	// Items map does, so the base is captured at fill time).
	NormalSlotCount  uint8
	RoundRobinPlayer uint64
	Viewers          map[uint64]*session
}

// storeLootTemplateRow routes one rolled loot-template row into Items or
// QuestItems, mirroring Loot::AddItem (Loot.cpp:141-152) where needs_quest
// rows go to quest_items with the MAX_NR_QUEST_ITEMS cap.
func storeLootTemplateRow(loot *activeLootState, slot, qidx *uint8, itemID, count, displayID, quality, startQuest, customFlags uint32, questRequired bool) {
	if questRequired {
		if *qidx >= maxQuestLootItems {
			return
		}
		if loot.QuestItems == nil {
			loot.QuestItems = make(map[uint8]lootItem)
		}
		loot.QuestItems[*qidx] = lootItem{
			Slot:          *qidx,
			ItemEntry:     itemID,
			Count:         count,
			DisplayInfoID: displayID,
			Quality:       quality,
			NeedsQuest:    true,
			StartQuest:    startQuest,
			CustomFlags:   customFlags,
		}
		*qidx++
		return
	}
	if *slot >= maxNormalLootItems {
		return
	}
	loot.Items[*slot] = lootItem{
		Slot:          *slot,
		ItemEntry:     itemID,
		Count:         count,
		DisplayInfoID: displayID,
		Quality:       quality,
		StartQuest:    startQuest,
		CustomFlags:   customFlags,
	}
	*slot++
}

// maxLootReferenceDepth caps recursive reference_loot_template expansion in
// fillLootTemplate. C++ imposes no cap (reference cycles are a DB error); the
// cap only guards a corrupt reference chain from recursing without bound.
const maxLootReferenceDepth = 8

// lootTemplateRow is one row of a *_loot_template table as loaded for
// generation, mirroring LootStoreItem (LootMgr.h).
type lootTemplateRow struct {
	itemID        uint32
	reference     uint32
	chance        float64
	minCount      uint32
	maxCount      uint32
	displayID     uint32
	quality       uint32
	questRequired bool
	startQuest    uint32
	customFlags   uint32
	lootMode      uint32
	groupID       uint8
	maxStack      uint32
}

// fillLootTemplate mirrors LootTemplate::Process (LootMgr.cpp:562-600) driving
// Loot::AddItem (Loot.cpp:141-186): rows whose lootmode bit is not set for the
// requested mode are skipped (SharedDefines.h LootModes); non-grouped rows are
// rolled independently, reference rows expanding reference_loot_template;
// then each loot group rolls exactly one entry (LootGroup::Roll/Process,
// LootMgr.cpp:375-397/421-426). table is one of the *_loot_template tables;
// loot must be freshly cleared — slot counters restart at 0 and
// NormalSlotCount is set on return.
func (s *Server) fillLootTemplate(ctx context.Context, wdb *sql.DB, table string, lootID int64, lootMode uint32, loot *activeLootState) {
	var slot, qidx uint8
	s.fillLootTemplateDepth(ctx, wdb, table, lootID, lootMode, loot, &slot, &qidx, 0)
	loot.NormalSlotCount = slot
}

func (s *Server) fillLootTemplateDepth(ctx context.Context, wdb *sql.DB, table string, lootID int64, lootMode uint32, loot *activeLootState, slot, qidx *uint8, depth int) {
	switch table {
	case "creature_loot_template", "gameobject_loot_template", "skinning_loot_template",
		"pickpocketing_loot_template", "reference_loot_template":
	default:
		return
	}
	if depth > maxLootReferenceDepth || wdb == nil {
		return
	}
	rows := loadLootTemplateRows(ctx, wdb, table, lootID, lootMode)

	// Rolling non-grouped items first (LootMgr.cpp:574-597); references never
	// join groups (LootTemplate::AddEntry, LootMgr.cpp:522-535).
	var groups []*lootGroup
	groupIndex := make(map[uint8]*lootGroup)
	for i := range rows {
		row := &rows[i]
		if row.lootMode&lootMode == 0 {
			continue
		}
		if row.groupID == 0 {
			if row.reference > 0 {
				if !lootRowTakesChance(row.chance) {
					continue
				}
				// Reference multiplicator: maxcount loops over the referenced
				// template (LootMgr.cpp:587-591); RATE_DROP_ITEM_REFERENCED and
				// RATE_DROP_ITEM_REFERENCED_AMOUNT have no Go model.
				for n := uint32(0); n < row.maxCount; n++ {
					s.fillLootTemplateDepth(ctx, wdb, "reference_loot_template", int64(row.reference), lootMode, loot, slot, qidx, depth+1)
				}
				continue
			}
			if !lootRowTakesChance(row.chance) {
				continue
			}
			addLootTemplateRow(loot, slot, qidx, row)
			continue
		}
		g := groupIndex[row.groupID]
		if g == nil {
			g = &lootGroup{groupID: row.groupID}
			groupIndex[row.groupID] = g
			groups = append(groups, g)
		}
		if row.chance != 0 {
			g.explicit = append(g.explicit, row)
		} else {
			g.equal = append(g.equal, row)
		}
	}

	// Now processing groups, in ascending groupid order like the C++ Groups
	// vector (LootMgr.cpp:599-601).
	sort.Slice(groups, func(i, j int) bool { return groups[i].groupID < groups[j].groupID })
	for _, g := range groups {
		if row := g.roll(loot); row != nil {
			addLootTemplateRow(loot, slot, qidx, row)
		}
	}
}

// lootGroup mirrors LootTemplate::LootGroup (LootMgr.h): entries with an
// explicit chance roll sequentially, zero-chance entries share one equal
// pick. At most one entry per group drops.
type lootGroup struct {
	groupID  uint8
	explicit []*lootTemplateRow
	equal    []*lootTemplateRow
}

// roll mirrors LootTemplate::LootGroup::Roll (LootMgr.cpp:375-397): the first
// explicitly-chanced entry whose running chance subtraction drops below zero
// wins (chance >= 100 wins immediately); otherwise one equal-chanced entry is
// picked at random. Entries already present maxDuplicates (1) times in the
// loot are excluded, mirroring LootGroupInvalidSelector (LootMgr.cpp:58-76);
// the lootmode arm of that selector is applied at load time and conditions
// have no Go model.
func (g *lootGroup) roll(loot *activeLootState) *lootTemplateRow {
	if len(g.explicit) > 0 {
		roll := rand.Float64() * 100
		for _, row := range g.explicit {
			if lootHasItem(loot, row.itemID) {
				continue
			}
			if row.chance >= 100 {
				return row
			}
			roll -= row.chance
			if roll < 0 {
				return row
			}
		}
	}
	var candidates []*lootTemplateRow
	for _, row := range g.equal {
		if !lootHasItem(loot, row.itemID) {
			candidates = append(candidates, row)
		}
	}
	if len(candidates) > 0 {
		return candidates[rand.Intn(len(candidates))]
	}
	return nil
}

// lootHasItem reports whether itemID already occupies a normal loot slot,
// for the LootGroupInvalidSelector maxDuplicates arm (LootMgr.cpp:66-71).
func lootHasItem(loot *activeLootState, itemID uint32) bool {
	for _, it := range loot.Items {
		if it.ItemEntry == itemID {
			return true
		}
	}
	return false
}

// lootRowTakesChance mirrors LootStoreItem::Roll (LootMgr.cpp:281-295) minus
// the world rates (RATE_DROP_ITEM_REFERENCED and the qualityToRate table have
// no Go model): a flat roll against the row chance.
func lootRowTakesChance(chance float64) bool {
	if chance >= 100 {
		return true
	}
	return rand.Float64()*100 < chance
}

// addLootTemplateRow mirrors the count/stack arms of Loot::AddItem
// (Loot.cpp:144-152): the rolled count is split into max-stack-sized rows
// (ItemTemplate::GetMaxStackSize, ItemTemplate.h:688 — Stackable <= 0 means
// effectively unlimited, so an unknown maxStack disables splitting).
func addLootTemplateRow(loot *activeLootState, slot, qidx *uint8, row *lootTemplateRow) {
	count := row.minCount
	if row.maxCount > row.minCount {
		count += uint32(rand.Intn(int(row.maxCount - row.minCount + 1)))
	}
	if count == 0 {
		count = 1
	}
	stacks := uint32(1)
	if row.maxStack > 1 && count > row.maxStack {
		stacks = count / row.maxStack
		if count%row.maxStack != 0 {
			stacks++
		}
	}
	for i := uint32(0); i < stacks; i++ {
		c := count - i*row.maxStack
		if row.maxStack > 0 && c > row.maxStack {
			c = row.maxStack
		}
		storeLootTemplateRow(loot, slot, qidx, row.itemID, c, row.displayID, row.quality, row.startQuest, row.customFlags, row.questRequired)
	}
}

// loadLootTemplateRows loads one loot template table's rows for an entry.
// The full column set degrades through the two historical fallbacks when the
// world schema lacks the newer columns (matching the pre-existing
// isMissingColumn fallbacks at the call sites).
func loadLootTemplateRows(ctx context.Context, wdb *sql.DB, table string, lootID int64, lootMode uint32) []lootTemplateRow {
	full := `SELECT l.Item, l.Reference, l.Chance, l.MinCount, l.MaxCount, COALESCE(t.displayid, 0), COALESCE(t.Quality, 0),
			COALESCE(l.QuestRequired, 0), COALESCE(t.StartQuest, 0), COALESCE(t.flagsCustom, 0),
			COALESCE(l.LootMode, 1), COALESCE(l.GroupId, 0), COALESCE(t.Stackable, 0)
		FROM ` + table + ` AS l
		LEFT JOIN item_template AS t ON t.entry = l.Item
		WHERE l.Entry = ? ORDER BY l.Item LIMIT 48`
	rows, err := wdb.QueryContext(ctx, full, lootID)
	if err != nil && isMissingColumn(err) {
		rows, err = wdb.QueryContext(ctx, `SELECT l.Item, l.Chance, l.MinCount, l.MaxCount, COALESCE(t.displayid, 0), COALESCE(t.Quality, 0),
				COALESCE(l.QuestRequired, 0), COALESCE(t.StartQuest, 0), COALESCE(t.flagsCustom, 0)
			FROM `+table+` AS l
			LEFT JOIN item_template AS t ON t.entry = l.Item
			WHERE l.Entry = ? ORDER BY l.Item LIMIT 48`, lootID)
	}
	if err != nil && isMissingColumn(err) {
		rows, err = wdb.QueryContext(ctx, `SELECT l.Item, l.Chance, l.MinCount, l.MaxCount, COALESCE(t.displayid, 0), 0,
					0, 0, 0
			FROM `+table+` AS l
			LEFT JOIN item_template AS t ON t.entry = l.Item
			WHERE l.Entry = ? ORDER BY l.Item LIMIT 48`, lootID)
	}
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []lootTemplateRow
	for rows.Next() {
		var r lootTemplateRow
		var itemID, reference, minCount, maxCount, displayID, quality int64
		var questRequired, startQuest, customFlags, lootModeRow, groupID, maxStack int64
		var chance float64
		cols, colErr := rows.Columns()
		if colErr != nil {
			continue
		}
		var scanErr error
		if len(cols) >= 13 {
			scanErr = rows.Scan(&itemID, &reference, &chance, &minCount, &maxCount, &displayID, &quality,
				&questRequired, &startQuest, &customFlags, &lootModeRow, &groupID, &maxStack)
		} else {
			// Either historical fallback shape (9 selected columns); the
			// unselected generation columns keep their neutral defaults.
			scanErr = rows.Scan(&itemID, &chance, &minCount, &maxCount, &displayID, &quality,
				&questRequired, &startQuest, &customFlags)
			lootModeRow = int64(lootMode)
		}
		if scanErr != nil {
			continue
		}
		r.itemID = uint32(itemID)
		r.reference = uint32(reference)
		r.chance = chance
		r.minCount = uint32(minCount)
		r.maxCount = uint32(maxCount)
		r.displayID = uint32(displayID)
		r.quality = uint32(quality)
		r.questRequired = questRequired != 0
		r.startQuest = uint32(startQuest)
		r.customFlags = uint32(customFlags)
		r.lootMode = uint32(lootModeRow)
		r.groupID = uint8(groupID)
		r.maxStack = uint32(maxStack)
		out = append(out, r)
	}
	return out
}

type lootObjectKey struct {
	MapID      uint32
	InstanceID uint32
	GUID       uint64
}

type lootRollKey struct {
	Object lootObjectKey
	Slot   uint32
}

func (l *activeLootState) objectKey() lootObjectKey {
	return lootObjectKey{MapID: l.MapID, InstanceID: l.InstanceID, GUID: l.TargetGUID}
}

type lootOwnerState struct {
	PlayerGUID uint64
	GroupID    uint64
}

func (s *Server) creatureLootAllowed(mapID, instanceID uint32, targetGUID, standardGUID, playerGUID, groupID uint64) bool {
	if s == nil {
		return false
	}
	s.lootMu.Lock()
	owner, found := s.creatureLootOwners[lootObjectKey{MapID: mapID, InstanceID: instanceID, GUID: targetGUID}]
	if !found {
		owner, found = s.creatureLootOwners[lootObjectKey{MapID: mapID, InstanceID: instanceID, GUID: standardGUID}]
	}
	s.lootMu.Unlock()
	if !found {
		return true
	}
	if owner.GroupID != 0 {
		return groupID == owner.GroupID
	}
	return playerGUID == owner.PlayerGUID
}

// withinLootDistance mirrors WorldObject::IsWithinDistInMap(obj,
// INTERACTION_DISTANCE) (Object.cpp:1147): the 5.0-yard interaction check
// adds both combat reaches (Object.cpp:1149-1152), so a large creature stays
// lootable from farther out than a flat 5.0 center-distance check allows.
func withinLootDistance(s *session, target combatTarget) bool {
	targetReach := float64(target.CombatReach)
	if targetReach <= 0 {
		targetReach = 1.5
	}
	playerReach := 1.5
	if s != nil && s.player != nil && s.player.CombatReach > 0 {
		playerReach = float64(s.player.CombatReach)
	}
	return distance3D(s.player.X, s.player.Y, s.player.Z, target.X, target.Y, target.Z) <= 5.0+playerReach+targetReach
}

// sendLootReleaseResponse mirrors Player::SendLootRelease (Player.cpp:8519):
// SMSG_LOOT_RELEASE_RESPONSE carrying the guid and a 1 byte. Player::SendLoot
// answers missing/alive-state/distance failures on the open path with this
// silent release, not a SendLootError.
func (s *session) sendLootReleaseResponse(guid uint64) error {
	buf := protocol.NewBuffer(9)
	buf.WriteU64(guid)
	buf.WriteU8(1)
	return s.write(uint16(protocol.OpcodeSMSG_LOOT_RELEASE_RESPONSE), buf.Bytes(), true)
}

func (l *activeLootState) addViewer(s *session) {
	if l.Viewers == nil {
		l.Viewers = make(map[uint64]*session)
	}
	l.Viewers[s.playerGUID] = s
}

func (l *activeLootState) removeViewer(guid uint64) {
	if l.Viewers != nil {
		delete(l.Viewers, guid)
	}
}

func (l *activeLootState) broadcastRemoved(slot uint8) {
	buf := protocol.NewBuffer(1)
	buf.WriteU8(slot)
	for _, sess := range l.Viewers {
		if sess != nil {
			_ = sess.write(uint16(protocol.OpcodeSMSG_LOOT_REMOVED), buf.Bytes(), true)
		}
	}
}

// hasQuestForItem mirrors Player::HasQuestForItem (Player.cpp:16945-16998)
// with turnIn=false: true when the session holds an incomplete quest that
// still needs itemID, either as a RequiredItemId objective (quest-log item
// count below the required count) or as an ItemDrop source item the player
// does not yet own enough of.
func (s *session) hasQuestForItem(ctx context.Context, itemID uint32) bool {
	if s == nil || s.player == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return false
	}
	wdb := s.server.WorldStore.DB
	for qi := range s.player.QuestLog {
		entry := &s.player.QuestLog[qi]
		if entry.QuestID == 0 {
			continue
		}
		status, err := s.characterQuestStatus(ctx, entry.QuestID)
		if err != nil || status != questStatusIncomplete {
			continue
		}
		var reqIDs [6]int64
		var reqCounts [6]int64
		var dropIDs [4]int64
		var dropQty [4]int64
		if err := wdb.QueryRowContext(ctx, `SELECT
				COALESCE(RequiredItemId1, 0), COALESCE(RequiredItemId2, 0), COALESCE(RequiredItemId3, 0),
				COALESCE(RequiredItemId4, 0), COALESCE(RequiredItemId5, 0), COALESCE(RequiredItemId6, 0),
				COALESCE(RequiredItemCount1, 0), COALESCE(RequiredItemCount2, 0), COALESCE(RequiredItemCount3, 0),
				COALESCE(RequiredItemCount4, 0), COALESCE(RequiredItemCount5, 0), COALESCE(RequiredItemCount6, 0),
				COALESCE(ItemDrop1, 0), COALESCE(ItemDrop2, 0), COALESCE(ItemDrop3, 0), COALESCE(ItemDrop4, 0),
				COALESCE(ItemDropQuantity1, 0), COALESCE(ItemDropQuantity2, 0), COALESCE(ItemDropQuantity3, 0), COALESCE(ItemDropQuantity4, 0)
			FROM quest_template WHERE ID = ?`, entry.QuestID).Scan(
			&reqIDs[0], &reqIDs[1], &reqIDs[2], &reqIDs[3], &reqIDs[4], &reqIDs[5],
			&reqCounts[0], &reqCounts[1], &reqCounts[2], &reqCounts[3], &reqCounts[4], &reqCounts[5],
			&dropIDs[0], &dropIDs[1], &dropIDs[2], &dropIDs[3],
			&dropQty[0], &dropQty[1], &dropQty[2], &dropQty[3]); err != nil {
			continue
		}
		for j := 0; j < 6; j++ {
			if uint32(reqIDs[j]) == itemID && entry.ItemCounts[j] < uint16(reqCounts[j]) {
				return true
			}
		}
		needsDrop := false
		for j := 0; j < 4; j++ {
			if uint32(dropIDs[j]) == itemID {
				needsDrop = true
				break
			}
		}
		if !needsDrop {
			continue
		}
		var maxCount, stackable int64
		if err := wdb.QueryRowContext(ctx, `SELECT COALESCE(MaxCount, 0), COALESCE(Stackable, 1) FROM item_template WHERE entry = ?`, itemID).Scan(&maxCount, &stackable); err != nil {
			continue
		}
		var owned int64
		if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
			_ = s.server.CharactersStore.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(ii.count), 0) FROM character_inventory AS ci
				JOIN item_instance AS ii ON ii.guid = ci.item
				WHERE ci.guid = ? AND ii.itemEntry = ?`, s.playerGUID, itemID).Scan(&owned)
		}
		for j := 0; j < 4; j++ {
			if uint32(dropIDs[j]) != itemID {
				continue
			}
			if maxCount > 0 && owned < maxCount {
				return true
			}
			if dropQty[j] > 0 {
				if owned < dropQty[j] {
					return true
				}
			} else if stackable > 0 && owned < stackable {
				return true
			}
		}
	}
	return false
}

// lootItemAllowedForPlayer mirrors LootItem::AllowedForPlayer (Loot.cpp:57-95)
// minus the DB-conditions arm (no Go model). The faction arm precedes the
// master-looter leg: faction-flagged items hide from the opposite team
// outright. The master looter sees every non-quest-gated item (the recipe
// arms sit after that early-true leg), and givenByMasterLooter skips the
// master-looter leg per HandleLootMasterGiveOpcode (LootHandler.cpp:454).
// Recipe items hide from players lacking the profession skill or who already
// know the spell, and quest-gated items hide unless the player has a quest
// for the item. A template-lookup failure keeps the item visible: C++
// returns false there, but Go's lookup is a live DB query and a transient
// failure must not delete legit loot (documented delta).
func (s *session) lootItemAllowedForPlayer(ctx context.Context, item lootItem, givenByMasterLooter bool) bool {
	if s == nil || s.player == nil {
		return false
	}
	tmpl, err := s.loadItemQueryData(ctx, item.ItemEntry)
	if err != nil {
		return true
	}
	// Loot.cpp:65-70: not show loot for not own team.
	team := playerTeam(s.player.Race)
	if (tmpl.Flags2&0x01 != 0 && team != teamHorde) || (tmpl.Flags2&0x02 != 0 && team != teamAlliance) {
		return false
	}
	ignoreQuest := item.CustomFlags&itemFlagsCuIgnoreQuestStatus != 0
	// Loot.cpp:73-81: the master looter can see certain items even if the
	// character can't loot them — everything but quest-gated items.
	if !givenByMasterLooter && s.groupID != 0 && s.server != nil {
		s.server.groupsMu.Lock()
		grp := s.server.groups[s.groupID]
		s.server.groupsMu.Unlock()
		if grp != nil && grp.MasterLooter == s.playerGUID {
			if !ignoreQuest && (item.NeedsQuest || item.StartQuest != 0) {
				return false
			}
			return true
		}
	}
	// Loot.cpp:83-84: don't allow loot for players without the profession or
	// those who already know the recipe.
	var recipeSpell uint32
	if len(tmpl.Spells) > 1 && tmpl.Spells[1].ID > 0 {
		recipeSpell = uint32(tmpl.Spells[1].ID)
	}
	hasSkill := func(skillID uint32) bool {
		if skillID == 0 {
			return false
		}
		for _, skill := range s.player.Skills {
			if uint32(skill.Skill) == skillID {
				return true
			}
		}
		return false
	}
	if tmpl.Flags&0x02000000 != 0 { // ITEM_FLAG_HIDE_UNUSABLE_RECIPE
		if !hasSkill(tmpl.RequiredSkill) || playerHasSpell(s.player, recipeSpell) {
			return false
		}
	}
	// Loot.cpp:87-88: don't allow to loot soulbound recipes the player has
	// already learned.
	if tmpl.Class == 9 && tmpl.Bonding == 1 && recipeSpell != 0 && playerHasSpell(s.player, recipeSpell) {
		return false
	}
	// Loot.cpp:92-93: quest-gated items (needs_quest, or StartQuest items for
	// a quest the player has started) are hidden unless the player has a
	// quest that needs the item.
	if !ignoreQuest && (item.NeedsQuest || (item.StartQuest != 0 && s.questStatusNotNone(ctx, item.StartQuest))) && !s.hasQuestForItem(ctx, item.ItemEntry) {
		return false
	}
	return true
}

// lootQuestItemAllowed keeps the old entry point for the quest-item paths;
// it now carries the full AllowedForPlayer model (faction, master-looter,
// recipe, quest arms).
func (s *session) lootQuestItemAllowed(ctx context.Context, item lootItem, givenByMasterLooter bool) bool {
	return s.lootItemAllowedForPlayer(ctx, item, givenByMasterLooter)
}

// questStatusNotNone mirrors the GetQuestStatus(...) != QUEST_STATUS_NONE
// half of the StartQuest arm (Loot.cpp:93): any recorded status (incomplete,
// complete, failed, rewarded) hides the quest-starting item.
func (s *session) questStatusNotNone(ctx context.Context, questID uint32) bool {
	status, err := s.characterQuestStatus(ctx, questID)
	return err == nil && status != 0
}

// viewerQuestLootList mirrors Loot::FillQuestLoot (Loot.cpp:296-332): the
// sorted quest_items indices this viewer may see, gated by AllowedForPlayer
// or the follow-loot-rules group arm (Loot.cpp:307) that lets the master
// looter (or any member under non-master methods) distribute them.
func (s *session) viewerQuestLootList(ctx context.Context, loot *activeLootState) []uint8 {
	if s == nil || loot == nil || len(loot.QuestItems) == 0 {
		return nil
	}
	var grp *groupState
	if s.server != nil && s.groupID != 0 {
		s.server.groupsMu.Lock()
		grp = s.server.groups[s.groupID]
		s.server.groupsMu.Unlock()
	}
	indices := make([]uint8, 0, len(loot.QuestItems))
	for idx, item := range loot.QuestItems {
		if s.lootQuestItemAllowed(ctx, item, false) {
			indices = append(indices, idx)
			continue
		}
		if item.CustomFlags&itemFlagsCuFollowLootRules != 0 && grp != nil &&
			((grp.LootMethod == 2 && grp.MasterLooter == s.playerGUID) || grp.LootMethod != 2) {
			indices = append(indices, idx)
		}
	}
	sort.Slice(indices, func(i, j int) bool { return indices[i] < indices[j] })
	return indices
}

// broadcastQuestRemoved mirrors Loot::NotifyQuestItemRemoved (Loot.cpp:399):
// every looting player gets SMSG_LOOT_REMOVED with the slot from their own
// quest list (items.size() + their position), since the display slot is
// per-viewer.
func (l *activeLootState) broadcastQuestRemoved(ctx context.Context, questIndex uint8) {
	for _, sess := range l.Viewers {
		if sess == nil {
			continue
		}
		for pos, idx := range sess.viewerQuestLootList(ctx, l) {
			if idx == questIndex {
				buf := protocol.NewBuffer(1)
				buf.WriteU8(l.NormalSlotCount + uint8(pos))
				_ = sess.write(uint16(protocol.OpcodeSMSG_LOOT_REMOVED), buf.Bytes(), true)
				break
			}
		}
	}
}

// Loot::NotifyMoneyRemoved (Loot.cpp:384): SMSG_LOOT_CLEAR_MONEY goes to
// every player currently viewing the loot, not just the one taking the money.
func (l *activeLootState) broadcastMoneyRemoved() {
	for _, sess := range l.Viewers {
		if sess != nil {
			_ = sess.write(uint16(protocol.OpcodeSMSG_LOOT_CLEAR_MONEY), nil, true)
		}
	}
}

func (l *activeLootState) hasOverThresholdItem(threshold uint8) bool {
	for _, item := range l.Items {
		if item.Quality >= uint32(threshold) {
			return true
		}
	}
	return false
}

func sortedLootItems(items map[uint8]lootItem) []lootItem {
	result := make([]lootItem, 0, len(items))
	for _, item := range items {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Slot < result[j].Slot })
	return result
}

func buildLootLooterPacket(loot *activeLootState, grp *groupState) []byte {
	packet := protocol.NewBuffer(24)
	packet.WriteU64(loot.TargetGUID)
	if grp != nil && grp.LootMethod == 2 && grp.MasterLooter != 0 && loot.hasOverThresholdItem(grp.LootThreshold) {
		packet.WritePackedGUID(grp.MasterLooter)
	} else {
		packet.WriteU8(0)
	}
	packet.WriteU8(0)
	return packet.Bytes()
}

const (
	rollPass       uint8 = 0
	rollNeed       uint8 = 1
	rollGreed      uint8 = 2
	rollDisenchant uint8 = 3

	rollFlagTypePass       uint8 = 0x01
	rollFlagTypeNeed       uint8 = 0x02
	rollFlagTypeGreed      uint8 = 0x04
	rollFlagTypeDisenchant uint8 = 0x08
	rollAllTypeMask        uint8 = 0x0F
)

type activeGroupRoll struct {
	SourceGUID          uint64
	Slot                uint32
	ItemEntry           uint32
	ItemCount           uint32
	RandomSuffix        uint32
	RandomPropID        uint32
	RollVoteMask        uint8
	GroupID             uint64
	MapID               uint32
	InstanceID          uint32
	EligiblePlayers     map[uint64]struct{}
	StartedAt           time.Time
	Duration            time.Duration
	TotalPlayersRolling int
	TotalNeed           int
	TotalGreed          int
	TotalPass           int
	Votes               map[uint64]uint8
	Rolls               map[uint64]uint8
	Timer               *time.Timer
}

func rollLootObjectKey(roll *activeGroupRoll) lootObjectKey {
	return lootObjectKey{MapID: roll.MapID, InstanceID: roll.InstanceID, GUID: roll.SourceGUID}
}

// openGameObjectLoot mirrors the GO arm of Player::SendLoot
// (Player.cpp:8537-8662): the gameobject's loot is generated from its
// gameobject_template loot id (data1, falling back to the entry) on first
// open, then the stored window is reused; the round-robin looter is set for
// grouped players. lootType carries the caller — LOOT_CORPSE (1) for the
// CMSG_LOOT path, LOOT_SKINNING (6) for the disarm-trap EffectOpenLock arm
// (SpellEffects.cpp:2031) — and maxDist the C++ distance arm: 20.0 yards
// for the disarm arm (Player.cpp:8549) vs INTERACTION_DISTANCE otherwise.
// Documented deltas: the GO loot-regen arm (GO_ACTIVATED + respawn-delay
// re-roll, Player.cpp:8574-8578), the chest groupLootRules distribution
// (GroupLoot/NeedBeforeGreed/MasterLoot), the battleground CanActivateGO
// gate, and the fishing/fishing-hole/fishing-junk arms have no Go model.
func (s *session) openGameObjectLoot(ctx context.Context, targetGUID uint64, lootType uint8, maxDist float64) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	wdb := s.server.WorldStore.DB
	if wdb == nil {
		return true
	}
	lowGUID := uint32(targetGUID & 0x00FFFFFF)
	entry := uint32((targetGUID >> 24) & 0x00FFFFFF)

	var goMap uint32
	var goX, goY, goZ float32
	var data1 int64
	err := wdb.QueryRowContext(ctx, `SELECT g.map, g.position_x, g.position_y, g.position_z, COALESCE(t.data1, 0)
		FROM gameobject AS g
		JOIN gameobject_template AS t ON t.entry = g.id
		WHERE g.guid = ? AND g.id = ? LIMIT 1`, lowGUID, entry).Scan(&goMap, &goX, &goY, &goZ, &data1)
	if err != nil {
		_ = wdb.QueryRowContext(ctx, `SELECT g.map, g.position_x, g.position_y, g.position_z, COALESCE(t.data1, 0)
			FROM gameobject AS g
			JOIN gameobject_template AS t ON t.entry = g.id
			WHERE g.guid = ? LIMIT 1`, lowGUID).Scan(&goMap, &goX, &goY, &goZ, &data1)
	}
	if goMap != s.player.Map || distance3D(s.player.X, s.player.Y, s.player.Z, goX, goY, goZ) > maxDist {
		return s.sendLootReleaseResponse(targetGUID) == nil
	}
	lootID := data1
	if lootID == 0 {
		lootID = int64(entry)
	}

	key := lootObjectKey{MapID: goMap, InstanceID: s.player.InstanceID, GUID: targetGUID}
	s.server.lootMu.Lock()
	if s.server.creatureLoot == nil {
		s.server.creatureLoot = make(map[lootObjectKey]*activeLootState)
	}
	loot := s.server.creatureLoot[key]
	newLoot := loot == nil
	if newLoot {
		loot = &activeLootState{TargetGUID: targetGUID, MapID: goMap, InstanceID: s.player.InstanceID, LootType: lootType, Items: make(map[uint8]lootItem)}
		s.server.creatureLoot[key] = loot
	}
	s.server.lootMu.Unlock()

	if !newLoot {
		loot.addViewer(s)
		s.activeLoot = loot
		s.interruptCurrentCast()
		return s.finishLootOpen(ctx, loot)
	}

	s.server.fillLootTemplate(ctx, wdb, "gameobject_loot_template", lootID, lootModeDefault, loot)
	if s.server != nil && s.groupID != 0 {
		s.server.groupsMu.Lock()
		grp := s.server.groups[s.groupID]
		if grp != nil && loot.RoundRobinPlayer == 0 && grp.LootMethod != 0 {
			grp.updateLooter(s.server, goMap, goX, goY, goZ)
			loot.RoundRobinPlayer = grp.LooterGUID
		}
		s.server.groupsMu.Unlock()
	}
	loot.addViewer(s)
	s.activeLoot = loot
	s.interruptCurrentCast()
	return s.finishLootOpen(ctx, loot)
}

func (s *session) handleLoot(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 1 {
		return true
	}
	if s.isDeadOrGhost() {
		return true
	}
	reader := protocol.NewReader(payload)
	targetGUID, err := reader.ReadPackedGUID()
	if err != nil {
		return false
	}
	wdb := s.server.WorldStore.DB
	if wdb == nil {
		return true
	}
	high := uint16(targetGUID >> 48)

	// Player::SendLoot (Player.cpp:8526-8527): opening new loot releases the
	// previously open loot first (DoLootRelease legs). Skipped for a
	// same-target re-open so the round-robin owner assigned at first open
	// survives the refresh, and for player-corpse GUIDs, which never reach
	// SendLoot in C++ (dropped by the HandleLootOpcode cheat gate above).
	if high != 0xF101 {
		if prev := s.activeLoot; prev != nil && prev.TargetGUID != targetGUID {
			s.doLootRelease(prev)
		}
	}

	if high == 0xF110 {
		return s.openGameObjectLoot(ctx, targetGUID, lootTypeCorpse, 10.0)
	}

	// HandleLootOpcode cheat gate (LootHandler.cpp:229-239): CMSG_LOOT with a
	// non creature/vehicle GUID is dropped silently — no release response.
	// Player-corpse (bones) GUIDs (0xF101, ObjectGuid.h HighGuid::Corpse)
	// fall here: the SendLoot LOOT_CORPSE/LOOT_INSIGNIA arm
	// (Player.cpp:8712-8749) is server-driven only, via
	// Player::RemovedInsignia (Player.cpp:8486, bones conversion +
	// CORPSE_DYNFLAG_LOOTABLE + level stashed in loot.gold + lootRecipient)
	// reached from Spell::EffectSkinPlayerCorpse (SpellEffects.cpp:5126).
	// Go has no EffectSkinPlayerCorpse trigger, no corpse INSERT on death,
	// and no per-corpse loot store, so the insignia/bones regen path
	// (AV/Wintergrasp FillLoot(PLAYER_CORPSE_LOOT_ENTRY, LootTemplates_Creature)
	// + gold = urand(50,150)*0.016*pow(level/5.76,2.5)*RATE_DROP_MONEY,
	// Player.cpp:8727-8742; OWNER/NONE permission on lootRecipient;
	// HandleLootMoney corpse arm LootHandler.cpp:128-139 shareMoney=false;
	// DoLootRelease corpse arm LootHandler.cpp:311-322 clearing loot and
	// CORPSE_DYNFLAG_LOOTABLE) has no bridge until a bones-loot trigger
	// exists; the cheat-gate silent return is the only reachable arm.
	if high == 0xF101 {
		return true
	}

	target, ok := s.getCombatTarget(ctx, targetGUID)
	// DIAGNOSTIC: log loot attempts to diagnose window-not-opening
	s.debug("loot attempt", "account", s.accountName, "guid", targetGUID, "found", ok, "target_health", target.Health, "player_map", s.player.Map, "target_map", target.Map)
	// HandleLootOpcode cheat gate, second half (LootHandler.cpp:232-233):
	// CMSG_LOOT with a non creature/vehicle GUID is dropped silently. The
	// gameobject (0xF110) and corpse (0xF101) arms above are Go-routed entry
	// points; anything else that is not a creature (0xF130) or vehicle
	// (0xF150) GUID never reaches SendLoot in C++.
	if high != 0xF130 && high != 0xF150 {
		return true
	}
	if !ok || target.Map != s.player.Map || target.InstanceID != s.player.InstanceID || !withinLootDistance(s, target) {
		s.debug("loot rejected", "account", s.accountName, "reason", "target-not-found-or-invalid")
		// HandleLootOpcode (LootHandler.cpp:237-239) interrupts the current
		// cast after SendLoot returns, whatever its internal outcome was.
		s.interruptCurrentCast()
		return s.sendLootReleaseResponse(targetGUID) == nil
	}
	if target.Health != 0 {
		s.interruptCurrentCast()
		return s.sendLootReleaseResponse(targetGUID) == nil
	}
	guid := uint32(targetGUID & 0x00FFFFFF)
	creatureEntry := uint32((targetGUID >> 24) & 0x00FFFFFF)
	stdKey := creatureWorldGUID(guid, creatureEntry)
	if !s.server.creatureLootAllowed(target.Map, target.InstanceID, targetGUID, stdKey, s.playerGUID, s.groupID) {
		s.interruptCurrentCast()
		return s.sendLootError(targetGUID, 0) == nil
	}

	s.server.lootMu.Lock()
	key := lootObjectKey{MapID: target.Map, InstanceID: target.InstanceID, GUID: targetGUID}
	standardKey := lootObjectKey{MapID: target.Map, InstanceID: target.InstanceID, GUID: stdKey}
	if s.server.creatureLoot == nil {
		s.server.creatureLoot = make(map[lootObjectKey]*activeLootState)
	}
	loot := s.server.creatureLoot[key]
	if loot == nil {
		loot = s.server.creatureLoot[standardKey]
	}
	newLoot := loot == nil
	if newLoot {
		loot = &activeLootState{TargetGUID: targetGUID, MapID: target.Map, InstanceID: target.InstanceID, LootType: 1, Items: make(map[uint8]lootItem)}
		s.server.creatureLoot[key] = loot
		s.server.creatureLoot[standardKey] = loot
	}
	s.server.lootMu.Unlock()
	if !newLoot {
		if !s.server.creatureLootAllowed(target.Map, target.InstanceID, targetGUID, stdKey, s.playerGUID, s.groupID) {
			s.interruptCurrentCast()
			return s.sendLootError(targetGUID, 0) == nil
		}
		loot.addViewer(s)
		s.activeLoot = loot
		s.interruptCurrentCast()
		return s.finishLootOpen(ctx, loot)
	}
	// Query min/max gold and lootid from creature_template
	var minGold, maxGold, lootID int64
	_ = wdb.QueryRowContext(ctx, "SELECT minGold, maxGold FROM creature_template WHERE entry = ? LIMIT 1", creatureEntry).Scan(&minGold, &maxGold)
	_ = wdb.QueryRowContext(ctx, "SELECT lootid FROM creature_template WHERE entry = ? LIMIT 1", creatureEntry).Scan(&lootID)
	if lootID == 0 {
		lootID = int64(creatureEntry)
	}
	if maxGold > minGold && maxGold > 0 {
		loot.Money = uint32(minGold + int64(rand.Intn(int(maxGold-minGold+1))))
	} else if minGold > 0 {
		loot.Money = uint32(minGold)
	}
	// Loot::FillLoot (Loot.cpp:188) drives LootTemplate::Process over the
	// creature template; Go has no creature loot modes, so the default mode
	// (Unit.cpp:11250 passes creature->GetLootMode() in C++).
	s.server.fillLootTemplate(ctx, wdb, "creature_loot_template", lootID, lootModeDefault, loot)
	if s.server != nil && s.groupID != 0 {
		s.server.groupsMu.Lock()
		grp := s.server.groups[s.groupID]
		if grp != nil && loot.RoundRobinPlayer == 0 && grp.LootMethod != 0 {
			grp.updateLooter(s.server, target.Map, target.X, target.Y, target.Z)
			loot.RoundRobinPlayer = grp.LooterGUID
		}
		s.server.groupsMu.Unlock()
	}
	loot.addViewer(s)
	s.activeLoot = loot
	s.interruptCurrentCast()
	return s.finishLootOpen(ctx, loot)
}

func (s *session) handleFishingNodeUse(ctx context.Context, payload []byte, goState *dynamicGameObjectState) bool {
	return s.handleFishingUse(ctx, payload, goState, true, false)
}

func (s *session) handleFishingHoleUse(ctx context.Context, payload []byte, goState *dynamicGameObjectState) bool {
	return s.handleFishingUse(ctx, payload, goState, false, true)
}

func (s *session) handleFishingUse(ctx context.Context, payload []byte, goState *dynamicGameObjectState, requireOwner, forceSuccess bool) bool {
	if !s.playerLoaded || s.player == nil || goState == nil || (requireOwner && goState.OwnerGUID != s.playerGUID) {
		return true
	}
	reader := protocol.NewReader(payload)
	targetGUID, err := reader.ReadU64()
	if err != nil || targetGUID != goState.GUID {
		return true
	}
	if !forceSuccess && goState.State != GameObjectStateReady {
		_ = s.write(uint16(protocol.OpcodeSMSG_FISH_NOT_HOOKED), nil, true)
		return true
	}
	if goState.Map != s.player.Map || goState.InstanceID != s.player.InstanceID || distance3D(s.player.X, s.player.Y, s.player.Z, goState.X, goState.Y, goState.Z) > 10.0 {
		return s.sendLootError(targetGUID, 4) == nil
	}
	if !requireOwner && goState.FishingMaxOpens > 0 && goState.FishingUses >= goState.FishingMaxOpens {
		return true
	}
	if requireOwner {
		goState.FishingHandled = true
	}
	loot := &activeLootState{TargetGUID: targetGUID, MapID: goState.Map, InstanceID: goState.InstanceID, LootType: 3, Items: make(map[uint8]lootItem)}
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return s.sendLootResponse(ctx, loot) == nil
	}
	var zoneSkill int64
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT skill FROM skill_fishing_base_level WHERE entry = ?", s.player.Zone).Scan(&zoneSkill); err != nil && !missingTable(err) {
		zoneSkill = 0
	}
	fishingSkill := uint16(0)
	fishingMax := uint16(0)
	for _, skill := range s.player.Skills {
		if skill.Skill == 356 {
			fishingSkill, fishingMax = skill.Value, skill.Max
			break
		}
	}
	if !forceSuccess && fishingMax > fishingSkill && fishingSkill > 0 {
		stepsNeeded := uint16(1)
		if fishingSkill >= 75 && fishingSkill <= 300 {
			stepsNeeded = fishingSkill / 44
		} else if fishingSkill > 300 {
			stepsNeeded = fishingSkill / 31
		}
		if stepsNeeded == 0 {
			stepsNeeded = 1
		}
		s.player.FishingSteps++
		if uint16(s.player.FishingSteps) >= stepsNeeded {
			s.player.FishingSteps = 0
			for index := range s.player.Skills {
				if s.player.Skills[index].Skill == 356 && s.player.Skills[index].Value < fishingMax {
					s.player.Skills[index].Value++
					if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
						_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE character_skills SET value = ? WHERE guid = ? AND skill = 356", s.player.Skills[index].Value, s.playerGUID)
					}
					break
				}
			}
		}
	}
	chance := 100
	if zoneSkill > 0 && int64(fishingSkill) < zoneSkill {
		chance = int(float64(fishingSkill) / float64(zoneSkill) * 100)
		if chance < 1 {
			chance = 1
		}
	}
	success := forceSuccess || s.fishingHoleNearby(ctx, goState) || rand.Intn(100)+1 <= chance
	loadRows := func(entry uint32, lootMode uint32) error {
		rows, queryErr := s.server.WorldStore.DB.QueryContext(ctx, `SELECT l.Item, l.Chance, l.MinCount, l.MaxCount, COALESCE(t.displayid, 0), COALESCE(t.Quality, 0)
			FROM fishing_loot_template AS l LEFT JOIN item_template AS t ON t.entry = l.Item
			WHERE l.Entry = ? AND (COALESCE(l.LootMode, 1) & ?) <> 0 ORDER BY l.Item LIMIT 16`, entry, lootMode)
		if queryErr != nil {
			return queryErr
		}
		defer rows.Close()
		var slot uint8
		for rows.Next() && slot < 16 {
			var itemID int64
			var chance float64
			var minCount, maxCount, displayID, quality int64
			if scanErr := rows.Scan(&itemID, &chance, &minCount, &maxCount, &displayID, &quality); scanErr != nil {
				continue
			}
			if chance > 0 && rand.Float64()*100 > chance {
				continue
			}
			count := uint32(minCount)
			if maxCount > minCount {
				count += uint32(rand.Intn(int(maxCount - minCount + 1)))
			}
			if count == 0 {
				count = 1
			}
			loot.Items[slot] = lootItem{Slot: slot, ItemEntry: uint32(itemID), Count: count, DisplayInfoID: uint32(displayID), Quality: uint32(quality)}
			slot++
		}
		return rows.Err()
	}
	lootMode := uint32(1)
	if !success {
		lootMode = 0x8000
	}
	if err := loadRows(s.player.Zone, lootMode); err != nil && !missingTable(err) {
		return true
	}
	if len(loot.Items) == 0 && s.player.Zone != 1 {
		_ = loadRows(1, lootMode)
	}
	s.server.lootMu.Lock()
	if s.server.creatureLoot == nil {
		s.server.creatureLoot = make(map[lootObjectKey]*activeLootState)
	}
	s.server.creatureLoot[loot.objectKey()] = loot
	s.server.lootMu.Unlock()
	loot.addViewer(s)
	s.activeLoot = loot
	s.interruptCurrentCast()
	if err := s.sendLootResponse(ctx, loot); err != nil {
		return false
	}
	// The fishing-hole use-count and loot-state transitions moved to the
	// release arm (releaseGameObjectLoot): C++ counts the use on DoLootRelease
	// (LootHandler.cpp:290), not on open, and only for fully-looted holes.
	// The bobber despawn stays at open — it matches the release-time
	// GO_JUST_DEACTIVATED terminal state a release away from the window.
	if requireOwner {
		s.server.despawnDynamicGameObjectInInstance(goState.Map, goState.InstanceID, targetGUID)
	}
	return true
}

func (s *session) fishingHoleNearby(ctx context.Context, bobber *dynamicGameObjectState) bool {
	if s == nil || s.server == nil || bobber == nil {
		return false
	}
	s.server.objectsMu.RLock()
	for _, object := range s.server.dynamicGameObjects {
		if object != nil && object.Type == GameObjectTypeFishingHole && object.Map == bobber.Map && distance3D(object.X, object.Y, object.Z, bobber.X, bobber.Y, bobber.Z) <= 20.0 {
			s.server.objectsMu.RUnlock()
			return true
		}
	}
	s.server.objectsMu.RUnlock()
	if s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return false
	}
	var found int
	err := s.server.WorldStore.DB.QueryRowContext(ctx, `SELECT 1
		FROM gameobject AS g JOIN gameobject_template AS t ON t.entry = g.id
		WHERE g.map = ? AND t.type = ? AND g.position_x BETWEEN ? AND ? AND g.position_y BETWEEN ? AND ?
		LIMIT 1`, bobber.Map, GameObjectTypeFishingHole, bobber.X-20, bobber.X+20, bobber.Y-20, bobber.Y+20).Scan(&found)
	return err == nil && found == 1
}

// finishLootOpen completes a successful corpse-loot open: after the response is
// on the wire it sets the UNIT_FLAG_LOOTING bit (UnitDefines.h:134), the
// Player::SendLoot arm at Player.cpp:8906-8910 (loot_type == LOOT_CORPSE &&
// !guid.IsItem()); the handleLoot paths are corpse loot of creatures and
// gameobjects only, so the !IsItem leg is vacuous here. Item loot (items.go)
// and fishing (LOOT_FISHING) never take this path, matching C++.
func (s *session) finishLootOpen(ctx context.Context, loot *activeLootState) bool {
	if s.sendLootResponse(ctx, loot) != nil {
		return false
	}
	if s.player != nil && s.player.UnitFlags&unitFlagLooting == 0 {
		s.player.UnitFlags |= unitFlagLooting
		s.sendPlayerUpdate()
	}
	return true
}

func (s *session) sendLootResponse(ctx context.Context, loot *activeLootState) error {
	var grp *groupState
	if s.server != nil && s.groupID != 0 {
		s.server.groupsMu.Lock()
		grp = s.server.groups[s.groupID]
		s.server.groupsMu.Unlock()
	}

	// LootView (Loot.cpp:596): quest items are appended after the normal
	// items at slots items.size() + position-in-the-viewer's-quest-list.
	questList := s.viewerQuestLootList(ctx, loot)

	items := sortedLootItems(loot.Items)
	// Player::SendLoot permission model (Player.cpp:8784-8862, Loot.cpp:672):
	// solo creature/item loot carries OWNER_PERMISSION, rendered as
	// LOOT_SLOT_TYPE_OWNER; ungrouped gameobject loot carries ALL_PERMISSION
	// (plain allow-loot).
	ownerSlot := uint8(0) // LOOT_SLOT_TYPE_ALLOW_LOOT
	if uint16(loot.TargetGUID>>48) != 0xF110 {
		ownerSlot = 4 // LOOT_SLOT_TYPE_OWNER
	}
	// The gameobject group-permission ladder (Player.cpp:8640-8661) applies
	// whenever a GO loot window opens in a group — even with loot_type
	// LOOT_SKINNING, since chests open via EffectOpenLock
	// (SpellEffects.cpp:2031). Only creature skinning/pickpocketing skip the
	// ladder (Player.cpp:8770-8790, never touching the group ladder).
	groupLadder := grp != nil && !((loot.LootType == lootTypeSkinning || loot.LootType == lootTypePickpocketing) &&
		uint16(loot.TargetGUID>>48) != 0xF110)
	// Under the ladder the default slot is ALLOW_LOOT (Loot.cpp:596-706);
	// the OWNER base only applies outside it.
	baseSlot := ownerSlot
	if groupLadder {
		baseSlot = 0
	}
	type renderedLootRow struct {
		item     lootItem
		slotType uint8
	}
	// LootView (Loot.cpp:618-703): rows failing AllowedForPlayer are not
	// displayed at all, and a completed roll's item only shows to its
	// winner (Loot.cpp:652-658) — so the window is decided in one pass and
	// the item count is written after, matching C++'s count_pos patch.
	var shown []renderedLootRow
	for _, it := range items {
		if !s.lootItemAllowedForPlayer(ctx, it, false) {
			continue
		}
		var slotType uint8 = baseSlot
		if groupLadder {
			isOverThreshold := it.Quality >= uint32(grp.LootThreshold)
			switch grp.LootMethod {
			case 1: // Round Robin
				if loot.RoundRobinPlayer != 0 && s.playerGUID != loot.RoundRobinPlayer {
					slotType = 3 // LOOT_SLOT_TYPE_LOCKED
				}
			case 2: // Master Loot
				if isOverThreshold {
					if s.playerGUID == grp.MasterLooter {
						slotType = 2 // LOOT_SLOT_TYPE_MASTER
					} else {
						slotType = 3 // LOOT_SLOT_TYPE_LOCKED
					}
				}
				// Under-threshold items are ALLOW_LOOT for every viewer
				// (Loot.cpp:657: the !is_underthreshold arm; MasterLoot sets
				// is_blocked = !is_underthreshold, Group.cpp:1413).
			case 3, 4: // Group Loot / Need Before Greed
				if isOverThreshold {
					rollKey := lootRollKey{Object: loot.objectKey(), Slot: uint32(it.Slot)}
					s.server.lootMu.Lock()
					activeRoll := s.server.groupRolls[rollKey]
					s.server.lootMu.Unlock()
					if activeRoll != nil || it.IsBlocked {
						slotType = 1 // LOOT_SLOT_TYPE_ROLL_ONGOING
					} else if it.RollWinner != 0 {
						if it.RollWinner == s.playerGUID {
							slotType = 4 // LOOT_SLOT_TYPE_OWNER
						} else {
							// Loot.cpp:652-658: a completed roll hides the
							// item from everyone but the winner.
							continue
						}
					}
				}
				// Under-threshold: ALLOW_LOOT for every viewer (same arm).
			}
		}
		shown = append(shown, renderedLootRow{item: it, slotType: slotType})
	}
	packet := protocol.NewBuffer(8 + 1 + 4 + 1 + (len(shown)+len(questList))*22)
	packet.WriteU64(loot.TargetGUID)
	packet.WriteU8(loot.LootType)
	packet.WriteU32(loot.Money)
	packet.WriteU8(uint8(len(shown) + len(questList)))
	for _, row := range shown {
		it := row.item
		packet.WriteU8(it.Slot)
		packet.WriteU32(it.ItemEntry)
		packet.WriteU32(it.Count)
		packet.WriteU32(it.DisplayInfoID)
		packet.WriteU32(0) // RandomSuffix (Loot.cpp:589: randomSuffix before randomPropertyId)
		packet.WriteU32(0) // RandomPropertyId
		packet.WriteU8(row.slotType)
	}
	// LootView quest arm (Loot.cpp:703-745): the viewer's quest items follow
	// the normal items. follow_loot_rules items take the master/locked arms
	// under master loot; otherwise they ride the permission default
	// (ownerSlot, matching the normal-item legs above). Go has no
	// quest-item block model, so the GROUP/NBG ROLL_ONGOING leg has
	// no bridge: follow_loot_rules quest items under group/nbg stay directly
	// lootable (documented delta).
	for pos, qidx := range questList {
		qit := loot.QuestItems[qidx]
		// Loot.cpp:709-711: the quest section rides the permission default
		// (OWNER/ALLOW) unless follow_loot_rules pulls it into the master
		// arm, which always renders MASTER for the master looter's view.
		// The default follows baseSlot: under the group ladder the
		// permission arms render ALLOW_LOOT (Loot.cpp:709:
		// OWNER_PERMISSION ? OWNER : ALLOW_LOOT).
		var qSlotType uint8 = baseSlot
		if qit.CustomFlags&itemFlagsCuFollowLootRules != 0 && grp != nil && grp.LootMethod == 2 {
			if s.playerGUID == grp.MasterLooter {
				qSlotType = 2 // LOOT_SLOT_TYPE_MASTER
			} else {
				qSlotType = 3 // LOOT_SLOT_TYPE_LOCKED
			}
		}
		packet.WriteU8(loot.NormalSlotCount + uint8(pos))
		packet.WriteU32(qit.ItemEntry)
		packet.WriteU32(qit.Count)
		packet.WriteU32(qit.DisplayInfoID)
		packet.WriteU32(0) // RandomSuffix (Loot.cpp:589: randomSuffix before randomPropertyId)
		packet.WriteU32(0) // RandomPropertyId
		packet.WriteU8(qSlotType)
	}
	if err := s.write(uint16(protocol.OpcodeSMSG_LOOT_RESPONSE), packet.Bytes(), true); err != nil {
		return err
	}
	s.debug("loot response sent", "account", s.accountName, "target", loot.TargetGUID, "money", loot.Money, "items", len(loot.Items))

	if grp != nil {
		if grp.LootMethod == 2 { // Master Loot
			s.sendLootMasterList(loot)
		} else if grp.LootMethod == 3 || grp.LootMethod == 4 { // Group Loot / Need Before Greed
			for _, it := range sortedLootItems(loot.Items) {
				if it.Quality >= uint32(grp.LootThreshold) {
					s.server.startGroupLootRoll(loot.TargetGUID, uint32(it.Slot), it.ItemEntry, it.Count, loot.MapID, loot.InstanceID, s.groupID, s)
				}
			}
		}
	}
	return nil
}

func (s *session) sendLootMasterList(loot *activeLootState) {
	if s.server == nil || s.groupID == 0 {
		return
	}
	s.server.groupsMu.Lock()
	grp := s.server.groups[s.groupID]
	s.server.groupsMu.Unlock()
	if grp == nil || grp.LootMethod != 2 { // 2 = MASTER_LOOT
		return
	}
	members := s.server.getGroupSessions(s.groupID)
	var nearMembers []*session
	for _, m := range members {
		if m.player != nil && m.player.Map == s.player.Map && distance3D(s.player.X, s.player.Y, s.player.Z, m.player.X, m.player.Y, m.player.Z) <= 100.0 {
			nearMembers = append(nearMembers, m)
		}
	}
	buf := protocol.NewBuffer(1 + len(nearMembers)*8)
	buf.WriteU8(uint8(len(nearMembers)))
	for _, m := range nearMembers {
		buf.WriteU64(m.playerGUID)
	}
	for _, m := range nearMembers {
		_ = m.write(uint16(protocol.OpcodeSMSG_LOOT_MASTER_LIST), buf.Bytes(), true)
	}
}

func (s *session) sendLootError(guid uint64, code uint8) error {
	packet := protocol.NewBuffer(10)
	packet.WriteU64(guid)
	packet.WriteU8(0)
	packet.WriteU8(code)
	return s.write(uint16(protocol.OpcodeSMSG_LOOT_RESPONSE), packet.Bytes(), true)
}

func (s *session) clearCreatureLoot(loot *activeLootState) {
	if loot == nil || s.server == nil {
		return
	}
	high := uint16(loot.TargetGUID >> 48)
	guids := []uint64{loot.TargetGUID}
	if high != 0xF110 {
		guid := uint32(loot.TargetGUID & 0x00FFFFFF)
		creatureEntry := uint32((loot.TargetGUID >> 24) & 0x00FFFFFF)
		guids = append(guids, creatureWorldGUID(guid, creatureEntry))
	}
	s.server.clearLootState(loot.MapID, loot.InstanceID, guids...)
	if high != 0xF110 {
		guid := uint32(loot.TargetGUID & 0x00FFFFFF)
		creatureEntry := uint32((loot.TargetGUID >> 24) & 0x00FFFFFF)
		stdGUID := creatureWorldGUID(guid, creatureEntry)
		// Loot::isLooted (Loot.h:236) turns true when the corpse's loot fully
		// empties: clearCreatureLoot only runs on empty-loot paths, so the
		// motion flips to looted here (respawn clears it in kill.go).
		if motion := s.server.findCreatureMotion(loot.MapID, loot.InstanceID, stdGUID); motion != nil {
			motion.Looted = true
			// Creature::AllLootRemovedFromCorpse skinnable arm (Creature.cpp:2827-2833):
			// a fully-looted non-skinning corpse with a skinning loot id becomes skinnable.
			if loot.LootType != lootTypeSkinning {
				s.maybeSetMotionSkinnable(motion, loot.MapID, loot.InstanceID, loot.TargetGUID, stdGUID)
			}
		}
		s.server.broadcastCreatureValuesUpdateInInstance(loot.MapID, loot.InstanceID, loot.TargetGUID, map[int]uint32{unitFieldDynamicFlags: 0})
		s.server.broadcastCreatureValuesUpdateInInstance(loot.MapID, loot.InstanceID, stdGUID, map[int]uint32{unitFieldDynamicFlags: 0})
	}
}

// maybeSetMotionSkinnable mirrors the SetFlag arm of
// Creature::AllLootRemovedFromCorpse (Creature.cpp:2827-2833): a fully
// looted (motion.Looted) non-skinning corpse becomes skinnable when the
// template names a skinning loot id that actually has rows in
// skinning_loot_template (the HaveLootFor arm) and someone still holds the
// loot rights (the hasLootRecipient arm). The !IsPet() arm is structural:
// creature loot states are only ever created for world-creature GUIDs, so
// pet motions never reach here. The RATE_CORPSE_DECAY_LOOTED shortening
// (Creature.cpp:2835-2845) has no Go analog — Go has no corpse-removal
// phase; the respawn timer is spawntimesecs from the kill (kill.go), so
// there is no corpse phase to shorten.
func (s *session) maybeSetMotionSkinnable(motion *creatureMotion, mapID, instanceID uint32, targetGUID, stdGUID uint64) {
	if s == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil || motion == nil {
		return
	}
	wdb := s.server.WorldStore.DB
	ctx := context.Background()
	var skinLootID int64
	if err := wdb.QueryRowContext(ctx, "SELECT COALESCE(SkinLootId, 0) FROM creature_template WHERE entry = ? LIMIT 1", motion.Entry).Scan(&skinLootID); err != nil || skinLootID == 0 {
		return
	}
	var rows int64
	if err := wdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM skinning_loot_template WHERE Entry = ?", skinLootID).Scan(&rows); err != nil || rows == 0 {
		return
	}
	s.server.lootMu.Lock()
	_, found := s.server.creatureLootOwners[lootObjectKey{MapID: mapID, InstanceID: instanceID, GUID: targetGUID}]
	if !found {
		_, found = s.server.creatureLootOwners[lootObjectKey{MapID: mapID, InstanceID: instanceID, GUID: stdGUID}]
	}
	s.server.lootMu.Unlock()
	if !found {
		return
	}
	s.server.motionMu.Lock()
	motion.UnitFlags |= unitFlagSkinnable
	flags := motion.UnitFlags
	s.server.motionMu.Unlock()
	s.server.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, motion.GUID, map[int]uint32{unitFieldFlags: flags})
}

// openSkinningLoot mirrors the LOOT_SKINNING arm of Player::SendLoot
// (Player.cpp:8834-8843) as driven by Spell::EffectSkinning
// (SpellEffects.cpp:4464-4492): the creature's loot is cleared and refilled
// from skinning_loot_template[SkinLootId] (no entry fallback — C++ passes
// SkinLootId straight through), the loot type is LOOT_SKINNING, and the
// recipient is the skinner alone (SetLootRecipient(this, false)); group
// rights are skipped (Player.cpp:8849). The OWNER permission and the
// recipient-only CanLoot arm (Player.cpp:18118) ride the existing
// creatureLootAllowed owner check: a GroupID-0 owner reduces it to the
// player-GUID match, exactly the C++ arm.
func (s *session) openSkinningLoot(ctx context.Context, targetGUID uint64, entry uint32) bool {
	if s == nil || s.player == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return true
	}
	wdb := s.server.WorldStore.DB
	guid := uint32(targetGUID & 0x00FFFFFF)
	stdKey := creatureWorldGUID(guid, entry)
	s.server.lootMu.Lock()
	key := lootObjectKey{MapID: s.player.Map, InstanceID: s.player.InstanceID, GUID: targetGUID}
	standardKey := lootObjectKey{MapID: s.player.Map, InstanceID: s.player.InstanceID, GUID: stdKey}
	if s.server.creatureLoot == nil {
		s.server.creatureLoot = make(map[lootObjectKey]*activeLootState)
	}
	loot := s.server.creatureLoot[key]
	if loot == nil {
		loot = s.server.creatureLoot[standardKey]
	}
	if loot == nil {
		loot = &activeLootState{TargetGUID: targetGUID, MapID: s.player.Map, InstanceID: s.player.InstanceID, Items: make(map[uint8]lootItem)}
		s.server.creatureLoot[key] = loot
		s.server.creatureLoot[standardKey] = loot
	}
	// Loot::clear() + SendLoot's loot->loot_type = LOOT_SKINNING (Player.cpp:8894).
	loot.Items = make(map[uint8]lootItem)
	loot.QuestItems = nil
	loot.Money = 0
	loot.LootType = lootTypeSkinning
	loot.NormalSlotCount = 0
	loot.RoundRobinPlayer = 0
	loot.Viewers = make(map[uint64]*session)
	if s.server.creatureLootOwners == nil {
		s.server.creatureLootOwners = make(map[lootObjectKey]lootOwnerState)
	}
	owner := lootOwnerState{PlayerGUID: s.playerGUID}
	s.server.creatureLootOwners[key] = owner
	s.server.creatureLootOwners[standardKey] = owner
	s.server.lootMu.Unlock()
	var skinLootID int64
	_ = wdb.QueryRowContext(ctx, "SELECT COALESCE(SkinLootId, 0) FROM creature_template WHERE entry = ? LIMIT 1", entry).Scan(&skinLootID)
	if skinLootID != 0 {
		s.server.fillLootTemplate(ctx, wdb, "skinning_loot_template", skinLootID, lootModeDefault, loot)
	}
	loot.addViewer(s)
	s.activeLoot = loot
	s.interruptCurrentCast()
	return s.finishLootOpen(ctx, loot)
}

// openPickpocketLoot mirrors the LOOT_PICKPOCKETING arm of Player::SendLoot
// (Player.cpp:8754-8790) as driven by Spell::EffectPickPocket
// (SpellEffects.cpp:2563-2578). The open gates (living creature,
// humanoid-or-undead mask, hostile, INTERACTION_DISTANCE) already ran in
// handleEffectPickPocket; this is the SendLoot body. Pickpocket loot is
// generated once: a second cast before the refill timer elapses answers
// LOOT_ERROR_ALREADY_PICKPOCKETED instead of regenerating
// (Creature::CanGeneratePickPocketLoot, Creature.cpp:3384), and an
// already-generated window is kept as-is (C++ "still has pickpocket loot
// generated & not fully taken"). Generation clears the loot, fills from
// pickpocketing_loot_template[pickpocketLootId] (no entry fallback — C++
// passes pickpocketLootId straight through), sets gold = 10 *
// (urand(0, creatureLvl/2) + urand(0, playerLvl/2)) (RATE_DROP_MONEY has no Go
// model, matching the existing unrated-gold convention), stores
// loot_type = LOOT_PICKPOCKETING (Player.cpp:8894), and assigns the
// pickpocketer as the sole recipient (SetLootRecipient(this, false)); the
// OWNER permission rides the existing creatureLootAllowed owner check with
// a GroupID-0 owner, and the pickpocket arm never touches the group ladder
// (Player.cpp:8792-8875 sit in the non-pickpocket else), so the group
// slot-type block in sendLootResponse skips LOOT_PICKPOCKETING. The refill
// timer (Creature::StartPickPocketRefillTimer, Creature.cpp:3379) is keyed
// on the per-instance loot key — Go stores no per-Creature field.
func (s *session) openPickpocketLoot(ctx context.Context, targetGUID uint64, entry, creatureLevel uint32) bool {
	if s == nil || s.player == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return true
	}
	wdb := s.server.WorldStore.DB
	guid := uint32(targetGUID & 0x00FFFFFF)
	stdKey := creatureWorldGUID(guid, entry)
	s.server.lootMu.Lock()
	key := lootObjectKey{MapID: s.player.Map, InstanceID: s.player.InstanceID, GUID: targetGUID}
	standardKey := lootObjectKey{MapID: s.player.Map, InstanceID: s.player.InstanceID, GUID: stdKey}
	if s.server.creatureLoot == nil {
		s.server.creatureLoot = make(map[lootObjectKey]*activeLootState)
	}
	loot := s.server.creatureLoot[key]
	if loot == nil {
		loot = s.server.creatureLoot[standardKey]
	}
	if loot != nil && loot.LootType == lootTypePickpocketing {
		s.server.lootMu.Unlock()
		loot.addViewer(s)
		s.activeLoot = loot
		s.interruptCurrentCast()
		return s.finishLootOpen(ctx, loot)
	}
	// Creature::CanGeneratePickPocketLoot (Creature.cpp:3384).
	if s.server.pickpocketLootRestore[key] > time.Now().Unix() {
		s.server.lootMu.Unlock()
		return s.sendLootError(targetGUID, lootErrorAlreadyPickpocketed) == nil
	}
	if loot == nil {
		loot = &activeLootState{TargetGUID: targetGUID, MapID: s.player.Map, InstanceID: s.player.InstanceID, Items: make(map[uint8]lootItem)}
		s.server.creatureLoot[key] = loot
		s.server.creatureLoot[standardKey] = loot
	}
	// Loot::clear() + loot->loot_type = LOOT_PICKPOCKETING
	// (Player.cpp:8894); Creature::StartPickPocketRefillTimer
	// (Creature.cpp:3379).
	loot.Items = make(map[uint8]lootItem)
	loot.QuestItems = nil
	loot.Money = 0
	loot.LootType = lootTypePickpocketing
	loot.NormalSlotCount = 0
	loot.RoundRobinPlayer = 0
	loot.Viewers = make(map[uint64]*session)
	if s.server.creatureLootOwners == nil {
		s.server.creatureLootOwners = make(map[lootObjectKey]lootOwnerState)
	}
	owner := lootOwnerState{PlayerGUID: s.playerGUID}
	s.server.creatureLootOwners[key] = owner
	s.server.creatureLootOwners[standardKey] = owner
	if s.server.pickpocketLootRestore == nil {
		s.server.pickpocketLootRestore = make(map[lootObjectKey]int64)
	}
	s.server.pickpocketLootRestore[key] = time.Now().Unix() + pickpocketRefillSeconds
	s.server.lootMu.Unlock()
	var pickpocketLootID int64
	_ = wdb.QueryRowContext(ctx, "SELECT COALESCE(pickpocketLootId, 0) FROM creature_template WHERE entry = ? LIMIT 1", entry).Scan(&pickpocketLootID)
	if pickpocketLootID != 0 {
		s.server.fillLootTemplate(ctx, wdb, "pickpocketing_loot_template", pickpocketLootID, lootModeDefault, loot)
	}
	// Player.cpp:8781-8783: gold = 10 * (urand(0, creatureLvl/2) +
	// urand(0, playerLvl/2)) * RATE_DROP_MONEY; the rate has no Go model.
	loot.Money = uint32(10 * (rand.Intn(int(creatureLevel)/2+1) + rand.Intn(int(s.player.Level)/2+1)))
	loot.addViewer(s)
	s.activeLoot = loot
	s.interruptCurrentCast()
	return s.finishLootOpen(ctx, loot)
}

func (s *Server) clearLootState(mapID, instanceID uint32, guids ...uint64) {
	if s == nil {
		return
	}
	s.lootMu.Lock()
	for _, guid := range guids {
		key := lootObjectKey{MapID: mapID, InstanceID: instanceID, GUID: guid}
		delete(s.creatureLoot, key)
		delete(s.creatureLootOwners, key)
	}
	for key, roll := range s.groupRolls {
		if roll == nil || key.Object.MapID != mapID || key.Object.InstanceID != instanceID {
			continue
		}
		for _, guid := range guids {
			if key.Object.GUID == guid {
				if roll.Timer != nil {
					roll.Timer.Stop()
				}
				delete(s.groupRolls, key)
				break
			}
		}
	}
	s.lootMu.Unlock()
}

func (s *session) handleLootMoney(ctx context.Context) bool {
	if !s.playerLoaded || s.player == nil || s.activeLoot == nil || s.activeLoot.Money == 0 {
		return true
	}
	if s.activeLoot.MapID != s.player.Map || s.activeLoot.InstanceID != s.player.InstanceID {
		return s.sendLootError(s.activeLoot.TargetGUID, 4) == nil
	}
	// HandleLootMoneyOpcode (LootHandler.cpp:132-142) Unit/Vehicle arm: the
	// creature must still be dead and lootable and within INTERACTION_DISTANCE,
	// else LOOT_ERROR_DIDNT_KILL / LOOT_ERROR_TOO_FAR. Gameobject loot keeps the
	// existing behavior (no owner model for the owned-bobber arm).
	// shareMoney starts true and the Unit/Vehicle arm below clears it for
	// pickpocket money (LootHandler.cpp:150-158).
	shareMoney := true
	if uint16(s.activeLoot.TargetGUID>>48) == 0xF110 {
		// HandleLootMoneyOpcode (LootHandler.cpp:116-126) GameObject arm:
		// the money is only reachable when the GO is still there for the
		// player — owned GOs (fishing bobbers) skip the distance check, all
		// other types must be within GameObject::IsWithinDistInMap
		// (interact distance, approximated with the 8.0 convention of
		// lootReleaseCleanupAllowed). A blocked take is silent: C++ leaves
		// loot null and skips the money block without an error. Static GOs
		// have no server-side dynamic state, so a nil state allows the take.
		// shareMoney stays true for GOs — only corpse/item/pickpocket clear
		// it (LootHandler.cpp:157 comment).
		if goState := s.server.gameObjectState(s.activeLoot.MapID, s.activeLoot.InstanceID, s.activeLoot.TargetGUID); goState != nil && goState.OwnerGUID != s.playerGUID &&
			(s.player == nil || distance3D(s.player.X, s.player.Y, s.player.Z, goState.X, goState.Y, goState.Z) > 8.0) {
			return true
		}
	}
	if high := uint16(s.activeLoot.TargetGUID >> 48); high != 0xF110 {
		target, ok := s.getCombatTarget(ctx, s.activeLoot.TargetGUID)
		guid := uint32(s.activeLoot.TargetGUID & 0x00FFFFFF)
		entry := uint32((s.activeLoot.TargetGUID >> 24) & 0x00FFFFFF)
		// LootHandler.cpp:150-158 (HandleLootMoneyOpcode) Unit/Vehicle arm:
		// alive iff rogue pickpocketing (same gate as HandleAutostoreLootItem
		// above); pickpocket money is never shared
		// ("item, pickpocket and players can be looted only single player").
		isRoguePickpocket := s.player.Class == 4 && s.activeLoot.LootType == lootTypePickpocketing
		lootAllowed := ok && (target.Health != 0) == isRoguePickpocket && s.server.creatureLootAllowed(s.activeLoot.MapID, s.activeLoot.InstanceID, s.activeLoot.TargetGUID, creatureWorldGUID(guid, entry), s.playerGUID, s.groupID)
		if !lootAllowed || !withinLootDistance(s, target) {
			code := uint8(0)
			if lootAllowed {
				code = 4
			}
			return s.sendLootError(s.activeLoot.TargetGUID, code) == nil
		}
		shareMoney = !isRoguePickpocket
	}
	copper := s.activeLoot.Money
	s.activeLoot.Money = 0

	// HandleLootMoneyOpcode (LootHandler.cpp:160): NotifyMoneyRemoved fires
	// before the split — viewers see SMSG_LOOT_CLEAR_MONEY ahead of the
	// SMSG_LOOT_MONEY_NOTIFY distribution, not after.
	s.activeLoot.broadcastMoneyRemoved()
	// LootHandler.cpp:157 (HandleLootMoneyOpcode): shareMoney is false for
	// item, pickpocket, and player-corpse loot — the split below only runs
	// for shared money.
	var nearMembers []*session
	if s.groupID != 0 && s.server != nil {
		allGroupSess := s.server.getGroupSessions(s.groupID)
		// Player::IsAtGroupRewardDistance (Player.cpp:24160): same map and
		// (dungeon, always) or within CONFIG_GROUP_XP_DISTANCE (default 100).
		inDungeon := s.isDungeonMap(s.player.Map)
		for _, m := range allGroupSess {
			if m.player != nil && m.player.Map == s.player.Map && m.player.InstanceID == s.player.InstanceID && (inDungeon || distance3D(s.player.X, s.player.Y, s.player.Z, m.player.X, m.player.Y, m.player.Z) <= 100.0) {
				nearMembers = append(nearMembers, m)
			}
		}
	}

	if len(nearMembers) > 1 && shareMoney {
		copperPerPlayer := copper / uint32(len(nearMembers))
		for _, m := range nearMembers {
			m.player.Money += copperPerPlayer
			// HandleLootMoneyOpcode (LootHandler.cpp:104): the achievement
			// fires per recipient, including the split path.
			m.updateAchievementCriteria(criteriaTypeLootMoney, 0, copperPerPlayer)
			if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
				_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", m.player.Money, m.playerGUID)
			}
			notify := protocol.NewBuffer(5)
			notify.WriteU32(copperPerPlayer)
			notify.WriteU8(0) // 0 = "Your share is..."
			_ = m.write(uint16(protocol.OpcodeSMSG_LOOT_MONEY_NOTIFY), notify.Bytes(), true)
			m.sendPlayerUpdate()
		}
	} else {
		s.player.Money += copper
		s.updateAchievementCriteria(criteriaTypeLootMoney, 0, copper)
		if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
			_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", s.player.Money, s.playerGUID)
		}
		notify := protocol.NewBuffer(5)
		notify.WriteU32(copper)
		notify.WriteU8(1) // 1 = "You loot..."
		_ = s.write(uint16(protocol.OpcodeSMSG_LOOT_MONEY_NOTIFY), notify.Bytes(), true)
		s.sendPlayerUpdate()
	}

	// Loot::isLooted (Loot.h:236) is gold == 0 && unlootedCount == 0, and the
	// quest-item fill arm counts quest items into unlootedCount
	// (Loot.cpp:316): money-take with quest loot remaining must not clear
	// the corpse.
	if s.activeLoot.Money == 0 && len(s.activeLoot.Items) == 0 && len(s.activeLoot.QuestItems) == 0 {
		s.clearCreatureLoot(s.activeLoot)
	}
	s.debug("loot money collected", "account", s.accountName, "copper", copper)
	return true
}

func (s *session) handleAutostoreLootItem(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || s.activeLoot == nil || len(payload) < 1 {
		return true
	}
	if s.activeLoot.MapID != s.player.Map || s.activeLoot.InstanceID != s.player.InstanceID {
		return s.sendLootError(s.activeLoot.TargetGUID, 4) == nil
	}
	if s.isDeadOrGhost() {
		return true
	}
	lootSlot := payload[0]
	it, ok := s.activeLoot.Items[lootSlot]
	// Loot::LootItemInSlot (Loot.cpp:446): slots at/above items.size() index
	// into the viewer's quest list, not the raw quest_items vector.
	questIndex := uint8(0)
	isQuestItem := false
	if !ok && lootSlot >= s.activeLoot.NormalSlotCount {
		questList := s.viewerQuestLootList(ctx, s.activeLoot)
		if pos := int(lootSlot - s.activeLoot.NormalSlotCount); pos < len(questList) {
			questIndex = questList[pos]
			if qit, found := s.activeLoot.QuestItems[questIndex]; found {
				it = qit
				isQuestItem = true
			}
		}
	}
	// Player::StoreLootItem (Player.cpp:25059-25063): a missing or already
	// taken slot answers EQUIP_ERR_ALREADY_LOOTED, not a silent drop.
	if !ok && !isQuestItem {
		s.sendEquipError(equipErrAlreadyLooted, 0)
		return true
	}
	high := uint16(s.activeLoot.TargetGUID >> 48)
	if high == 0xF110 {
		if s.activeLoot.MapID != s.player.Map || s.activeLoot.InstanceID != s.player.InstanceID {
			return s.sendLootError(s.activeLoot.TargetGUID, 4) == nil
		}
	} else {
		target, validTarget := s.getCombatTarget(ctx, s.activeLoot.TargetGUID)
		// LootHandler.cpp:87-97 (HandleAutostoreLootItem) Unit/Vehicle arm:
		// the creature must be alive iff the taker is a rogue lifting an
		// already-open pickpocket window (loot_type == LOOT_PICKPOCKETING);
		// every other loot take still requires the corpse. CLASS_ROGUE = 4
		// (SharedDefines.h). The error code splits on the gate that failed:
		// a failed alive-gate is LOOT_ERROR_DIDNT_KILL, a passed gate with
		// a dead/lost target out of range is LOOT_ERROR_TOO_FAR
		// (lootAllowed ? LOOT_ERROR_TOO_FAR : LOOT_ERROR_DIDNT_KILL).
		isRoguePickpocket := s.player.Class == 4 && s.activeLoot.LootType == lootTypePickpocketing
		lootAllowed := validTarget && target.Map == s.player.Map && target.InstanceID == s.player.InstanceID && (target.Health != 0) == isRoguePickpocket
		if !lootAllowed {
			return s.sendLootError(s.activeLoot.TargetGUID, 0) == nil
		}
		if !withinLootDistance(s, target) {
			return s.sendLootError(s.activeLoot.TargetGUID, 4) == nil
		}
		// Take-time recipient re-check: C++ has none in this handler, but
		// StoreLootItem's AllowedForPlayer-fail arm (Player.cpp:25065-25068)
		// answers with SendLootRelease, so a revoked permission releases
		// the window instead of erroring.
		guid := uint32(s.activeLoot.TargetGUID & 0x00FFFFFF)
		entry := uint32((s.activeLoot.TargetGUID >> 24) & 0x00FFFFFF)
		if !s.server.creatureLootAllowed(s.activeLoot.MapID, s.activeLoot.InstanceID, s.activeLoot.TargetGUID, creatureWorldGUID(guid, entry), s.playerGUID, s.groupID) {
			return s.sendLootReleaseResponse(s.activeLoot.TargetGUID) == nil
		}
	}

	// Player::StoreLootItem (Player.cpp:25071-25075): the AllowedForPlayer
	// gate is re-checked at take time; a quest item the viewer no longer
	// qualifies for answers with a silent loot release, not an error.
	if isQuestItem && !s.lootQuestItemAllowed(ctx, it, false) {
		return s.sendLootReleaseResponse(s.activeLoot.TargetGUID) == nil
	}

	if s.server != nil && s.groupID != 0 {
		s.server.groupsMu.Lock()
		grp := s.server.groups[s.groupID]
		s.server.groupsMu.Unlock()
		if grp != nil {
			isOverThreshold := it.Quality >= uint32(grp.LootThreshold)
			// Loot.cpp:307: only follow_loot_rules quest items are
			// master-distributed; plain quest items stay directly lootable
			// by quest-holding members under master loot. (Go anti-cheat
			// gate: C++ hides these slots instead of rejecting the take.)
			if grp.LootMethod == 2 && isOverThreshold &&
				!(isQuestItem && it.CustomFlags&itemFlagsCuFollowLootRules == 0) {
				// Player::StoreLootItem (Player.cpp:25076-25080): a blocked
				// item answers with SendLootRelease, not a silent drop.
				return s.sendLootReleaseResponse(s.activeLoot.TargetGUID) == nil
			}
			if (grp.LootMethod == 3 || grp.LootMethod == 4) && isOverThreshold {
				rollKey := lootRollKey{Object: s.activeLoot.objectKey(), Slot: uint32(it.Slot)}
				s.server.lootMu.Lock()
				activeRoll := s.server.groupRolls[rollKey]
				s.server.lootMu.Unlock()
				// Player::StoreLootItem (Player.cpp:25077-25080): a
				// blocked item answers with SendLootRelease, not a
				// silent drop.
				if activeRoll != nil || it.IsBlocked {
					return s.sendLootReleaseResponse(s.activeLoot.TargetGUID) == nil
				}
				// Player::StoreLootItem (Player.cpp:25082-25086): a roll
				// won by someone else releases the window.
				if it.RollWinner != 0 && it.RollWinner != s.playerGUID {
					return s.sendLootReleaseResponse(s.activeLoot.TargetGUID) == nil
				}
			} else if grp.LootMethod == 1 {
				// Loot.cpp:675-689 (ROUND_ROBIN_PERMISSION): only the
				// round-robin loot owner may take; under group/need-greed
				// (GROUP_PERMISSION, Loot.cpp:657) under-threshold items
				// are ALLOW_LOOT for every viewer, so the gate must not
				// fire for methods 3/4.
				if s.activeLoot.RoundRobinPlayer != 0 && s.activeLoot.RoundRobinPlayer != s.playerGUID {
					return true
				}
			}
		}
	}

	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}
	res, err := s.storeOrStackItem(ctx, s.playerGUID, it.ItemEntry, it.Count)
	if err != nil {
		if errors.Is(err, errInventoryFull) {
			s.sendEquipError(equipErrInvFull, 0)
		}
		return true
	}
	s.updateAchievementCriteria(criteriaTypeLootItem, it.ItemEntry, it.Count)
	s.updateAchievementCriteria(criteriaTypeLootType, uint32(s.activeLoot.LootType), it.Count)
	if it.Quality >= 4 {
		s.updateAchievementCriteria(criteriaTypeLootEpicItem, it.ItemEntry, it.Count)
		s.updateAchievementCriteria(criteriaTypeReceiveEpicItem, it.ItemEntry, it.Count)
	}
	// Player::StoreLootItem (Player.cpp:25122-25124): a taken quest item is
	// marked looted for everyone (Go has no ITEM_FLAG_MULTI_DROP free-for-all
	// quest model, so the per-player qitem copy leg is a documented delta).
	// The removal broadcast goes out before the delete because each viewer
	// gets the slot from their own quest list.
	if isQuestItem {
		s.activeLoot.broadcastQuestRemoved(ctx, questIndex)
		delete(s.activeLoot.QuestItems, questIndex)
	} else {
		delete(s.activeLoot.Items, lootSlot)
		s.activeLoot.broadcastRemoved(lootSlot)
	}
	_ = s.sendInventoryItems(ctx)

	slotForPush := uint32(res.Slot)
	if res.IsStack {
		slotForPush = 0xFFFFFFFF
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_ITEM_PUSH_RESULT), buildLootItemPushResult(s.playerGUID, res.ClientBag, slotForPush, it.ItemEntry, it.Count, res.InventoryCount), true)
	s.sendPlayerUpdate()
	if s.activeLoot.Money == 0 && len(s.activeLoot.Items) == 0 && len(s.activeLoot.QuestItems) == 0 {
		s.clearCreatureLoot(s.activeLoot)
	}
	s.debug("loot item stored", "account", s.accountName, "item", it.ItemEntry, "slot", res.Slot, "bag", res.ClientBag, "stacked", res.IsStack)
	return true
}

func buildLootItemPushResult(playerGUID uint64, bag uint8, slot, entry, count, inventoryCount uint32) []byte {
	packet := protocol.NewBuffer(48)
	packet.WriteU64(playerGUID)
	packet.WriteU32(0)
	packet.WriteU32(0)
	packet.WriteU32(1)
	packet.WriteU8(bag)
	packet.WriteU32(slot)
	packet.WriteU32(entry)
	packet.WriteU32(0)
	packet.WriteI32(0)
	packet.WriteU32(count)
	packet.WriteU32(inventoryCount)
	return packet.Bytes()
}

func (s *session) handleLootRelease(payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 1 {
		return true
	}
	reader := protocol.NewReader(payload)
	targetGUID, err := reader.ReadPackedGUID()
	if err != nil {
		return false
	}
	if s.activeLoot == nil || s.activeLoot.TargetGUID != targetGUID {
		return true
	}
	s.doLootRelease(s.activeLoot)
	return true
}

// doLootRelease runs the WorldSession::DoLootRelease (LootHandler.cpp:258)
// legs for an already-open loot: the unconditional release (loot-GUID
// clear, release response, UNIT_FLAG_LOOTING removal), the type-specific
// cleanup gate, and the round-robin looter broadcast on the
// not-fully-looted creature arm. Player::SendLoot (Player.cpp:8526-8527)
// runs this for the previously open loot before opening a new one.
func (s *session) doLootRelease(loot *activeLootState) {
	if s == nil || loot == nil {
		return
	}
	targetGUID := loot.TargetGUID
	// WorldSession::DoLootRelease (LootHandler.cpp:258-264): the loot-GUID
	// clear, the release response, and the UNIT_FLAG_LOOTING removal run
	// unconditionally — the map/instance/distance checks only gate the
	// fully-looted cleanup legs (dynflag clear, AllLootRemovedFromCorpse,
	// loot.clear). A release after a map change therefore still completes;
	// it never answers LOOT_ERROR_TOO_FAR.
	releasedRoundRobin := loot.RoundRobinPlayer == s.playerGUID
	cleanupAllowed := s.lootReleaseCleanupAllowed(loot)
	if uint16(targetGUID>>48) == 0xF110 {
		// DoLootRelease's GameObject arm (LootHandler.cpp:270-313) gates
		// its cleanup legs (door use, fishing-hole use-count, loot-state
		// transitions, loot.clear) on GO reachability; the unconditional
		// legs above still run on a blocked release.
		cleanupAllowed = s.releaseGameObjectLoot(loot, targetGUID)
	}
	// DoLootRelease (LootHandler.cpp:349-368): Group::SendLooter fires only
	// on the not-fully-looted arm — a fully-looted or blocked release
	// clears instead of announcing a looter.
	fullyLooted := loot.Money == 0 && len(loot.Items) == 0 && len(loot.QuestItems) == 0
	s.releaseActiveLootCleanup(cleanupAllowed)
	if releasedRoundRobin && cleanupAllowed && !fullyLooted && s.server != nil && s.groupID != 0 && uint16(loot.TargetGUID>>48) != 0xF110 {
		s.server.groupsMu.Lock()
		grp := s.server.groups[s.groupID]
		s.server.groupsMu.Unlock()
		if grp != nil {
			s.server.broadcastToGroup(s.groupID, uint16(protocol.OpcodeSMSG_LOOT_LIST), buildLootLooterPacket(loot, grp))
		}
	}
	release := protocol.NewBuffer(9)
	release.WriteU64(targetGUID)
	release.WriteU8(1)
	_ = s.write(uint16(protocol.OpcodeSMSG_LOOT_RELEASE_RESPONSE), release.Bytes(), true)
	s.debug("loot released", "account", s.accountName, "target", targetGUID)
}

// releaseGameObjectLoot mirrors the GameObject arm of WorldSession::DoLootRelease
// (LootHandler.cpp:270-313) and returns whether the fully-looted cleanup legs
// may run. The distance gate skips owned GOs (fishing bobbers) and fishing
// holes (LootHandler.cpp:274); every other type must be within
// GameObject::IsWithinDistInMap — IsAtInteractDistance with
// GetInteractionDistance (5.0 + object radius for chests/doors), approximated
// with the 8.0 convention of lootReleaseCleanupAllowed. Static GOs have no
// server-side dynamic state (their world presence is per-client), so a nil
// state allows the cleanup — the C++ !go gate has no Go registry to check.
// Door GOs are re-used instead of marked looted (UseDoorOrButton,
// LootHandler.cpp:281-285); fishing holes count one more use per fully-looted
// release and deactivate at max opens (despawned next tick) or return to ready
// (LootHandler.cpp:289-296); the bobber always despawns on release
// (LootHandler.cpp:287, type == FISHINGNODE enters the despawn branch even
// with loot remaining). Chest GO_JUST_DEACTIVATED respawn timing has no Go
// model — the loot row is still deleted by clearCreatureLoot.
func (s *session) releaseGameObjectLoot(loot *activeLootState, targetGUID uint64) bool {
	if s == nil || s.server == nil {
		return true
	}
	goState := s.server.gameObjectState(loot.MapID, loot.InstanceID, targetGUID)
	if goState == nil {
		return true
	}
	if goState.OwnerGUID != s.playerGUID && goState.Type != GameObjectTypeFishingHole &&
		(s.player == nil || distance3D(s.player.X, s.player.Y, s.player.Z, goState.X, goState.Y, goState.Z) > 8.0) {
		return false
	}
	if goState.Type == GameObjectTypeDoor {
		s.server.useDoorOrButton(goState.Map, goState.InstanceID, goState.GUID)
	}
	if loot.Money != 0 || len(loot.Items) != 0 {
		return true
	}
	switch goState.Type {
	case GameObjectTypeFishingHole:
		s.server.objectsMu.Lock()
		goState.FishingUses++
		uses, maxOpens := goState.FishingUses, goState.FishingMaxOpens
		s.server.objectsMu.Unlock()
		if maxOpens > 0 && uses >= maxOpens {
			s.server.despawnDynamicGameObjectInInstance(goState.Map, goState.InstanceID, goState.GUID)
		} else {
			s.server.setGameObjectStateInInstance(goState.Map, goState.InstanceID, goState.GUID, GameObjectStateReady)
		}
	case GameObjectTypeFishingNode:
		s.server.despawnDynamicGameObjectInInstance(goState.Map, goState.InstanceID, goState.GUID)
	}
	return true
}

// handleLootMasterGive processes CMSG_LOOT_MASTER_GIVE (0x2A3).
// Reference: WorldSession::HandleLootMasterGiveOpcode (LootHandler.cpp:392).
func (s *session) handleLootMasterGive(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 17 {
		return true
	}
	r := protocol.NewReader(payload)
	lootGUID, _ := r.ReadU64()
	slotID, _ := r.ReadU8()
	targetGUID, _ := r.ReadU64()

	if s.server == nil || s.groupID == 0 {
		_ = s.sendLootError(lootGUID, 0) // LOOT_ERROR_DIDNT_KILL
		return true
	}
	s.server.groupsMu.Lock()
	grp := s.server.groups[s.groupID]
	s.server.groupsMu.Unlock()
	if grp == nil || grp.LootMethod != 2 || grp.MasterLooter != s.playerGUID {
		_ = s.sendLootError(lootGUID, 0) // LOOT_ERROR_DIDNT_KILL
		return true
	}

	targetSess := s.server.findSessionByGUID(targetGUID)
	if targetSess == nil || targetSess.player == nil {
		_ = s.sendLootError(lootGUID, 10) // LOOT_ERROR_PLAYER_NOT_FOUND
		return true
	}
	// HandleLootMasterGiveOpcode (LootHandler.cpp:417-421): the giver's own
	// loot GUID must match the packet's before eligibility is checked.
	if s.activeLoot == nil || s.activeLoot.TargetGUID != lootGUID || s.activeLoot.MapID != s.player.Map || s.activeLoot.InstanceID != s.player.InstanceID {
		_ = s.sendLootError(lootGUID, 0) // LOOT_ERROR_DIDNT_KILL
		return true
	}
	// HandleLootMasterGiveOpcode (LootHandler.cpp:423-429):
	// !_player->IsInRaidWith(target) || !_player->IsInMap(target) — the
	// target must be in the same group and on the same map instance, with
	// NO distance gate (C++ allows a give to a same-map member at any
	// range; the old 100-yard check wrongly rejected that).
	if targetSess.groupID != s.groupID || targetSess.player.Map != s.player.Map || targetSess.player.InstanceID != s.player.InstanceID {
		_ = s.sendLootError(lootGUID, 14) // LOOT_ERROR_MASTER_OTHER
		return true
	}
	// HandleLootMasterGiveOpcode (LootHandler.cpp:448-452): the slot range
	// covers items.size() + quest_items.size(); slots at/above items.size()
	// index into the master looter's quest list.
	it, ok := s.activeLoot.Items[slotID]
	questIndex := uint8(0)
	isQuestItem := false
	if !ok && slotID >= s.activeLoot.NormalSlotCount {
		questList := s.viewerQuestLootList(ctx, s.activeLoot)
		if pos := int(slotID - s.activeLoot.NormalSlotCount); pos < len(questList) {
			questIndex = questList[pos]
			if qit, found := s.activeLoot.QuestItems[questIndex]; found {
				it = qit
				isQuestItem = true
			}
		}
	}
	if !ok && !isQuestItem {
		return true
	}

	// HandleLootMasterGiveOpcode (LootHandler.cpp:454-458): the recipient is
	// gated by AllowedForPlayer with isGivenByMasterLooter=true; a quest
	// item the target has no quest for maps
	// EQUIP_ERR_YOU_CAN_NEVER_USE_THAT_ITEM to LOOT_ERROR_MASTER_OTHER (14).
	if isQuestItem && !targetSess.lootQuestItemAllowed(ctx, it, true) {
		_ = s.sendLootError(lootGUID, 14)
		return true
	}

	res, err := targetSess.storeOrStackItem(ctx, targetGUID, it.ItemEntry, it.Count)
	if err != nil {
		// HandleLootMasterGiveOpcode (LootHandler.cpp:460-471): inventory
		// full maps to LOOT_ERROR_MASTER_INV_FULL (12), any other store
		// failure to LOOT_ERROR_MASTER_OTHER (14); the loot stays put. The
		// CANT_CARRY_MORE_OF_THIS -> LOOT_ERROR_MASTER_UNIQUE_ITEM (13) leg
		// has no Go analog (storeOrStackItem models no unique-item check).
		code := uint8(14)
		if errors.Is(err, errInventoryFull) {
			code = 12
		}
		_ = s.sendLootError(lootGUID, code)
		return true
	}
	// HandleLootMasterGiveOpcode (LootHandler.cpp:476-479): the recipient —
	// not the master looter — earns the loot achievement criteria; the
	// RECEIVE_EPIC_ITEM leg comes from the target->StoreNewItem call
	// (Player.cpp:12144). Mirrors the autostore arm above, including the
	// quality>=4 gate Go uses for the epic pair.
	targetSess.updateAchievementCriteria(criteriaTypeLootItem, it.ItemEntry, it.Count)
	targetSess.updateAchievementCriteria(criteriaTypeLootType, uint32(s.activeLoot.LootType), it.Count)
	if it.Quality >= 4 {
		targetSess.updateAchievementCriteria(criteriaTypeLootEpicItem, it.ItemEntry, it.Count)
		targetSess.updateAchievementCriteria(criteriaTypeReceiveEpicItem, it.ItemEntry, it.Count)
	}
	// HandleLootMasterGiveOpcode (LootHandler.cpp:484-486): the given item is
	// marked looted (count zeroed in C++; the Go analog deletes the row) and
	// every viewer is notified with their own slot.
	if isQuestItem {
		s.activeLoot.broadcastQuestRemoved(ctx, questIndex)
		delete(s.activeLoot.QuestItems, questIndex)
	} else {
		delete(s.activeLoot.Items, slotID)
		s.activeLoot.broadcastRemoved(slotID)
	}
	_ = targetSess.sendInventoryItems(ctx)
	targetSess.sendPlayerUpdate()
	slotForPush := uint32(res.Slot)
	if res.IsStack {
		slotForPush = 0xFFFFFFFF
	}
	_ = targetSess.write(uint16(protocol.OpcodeSMSG_ITEM_PUSH_RESULT), buildLootItemPushResult(targetGUID, res.ClientBag, slotForPush, it.ItemEntry, it.Count, res.InventoryCount), true)

	if s.activeLoot.Money == 0 && len(s.activeLoot.Items) == 0 && len(s.activeLoot.QuestItems) == 0 {
		s.clearCreatureLoot(s.activeLoot)
	}
	return true
}

func buildLootRollPayload(itemGUID uint64, slot uint32, rollType uint8) []byte {
	buf := protocol.NewBuffer(13)
	buf.WriteU64(itemGUID)
	buf.WriteU32(slot)
	buf.WriteU8(rollType)
	return buf.Bytes()
}

func buildLootStartRollPacket(sourceGUID uint64, mapID, slot, itemEntry, randomSuffix, randomPropID, itemCount, countdown uint32, rollVoteMask uint8) []byte {
	buf := protocol.NewBuffer(8 + 4 + 4 + 4 + 4 + 4 + 4 + 4 + 1)
	buf.WriteU64(sourceGUID)
	buf.WriteU32(mapID)
	buf.WriteU32(slot)
	buf.WriteU32(itemEntry)
	buf.WriteU32(randomSuffix)
	buf.WriteU32(randomPropID)
	buf.WriteU32(itemCount)
	buf.WriteU32(countdown)
	buf.WriteU8(rollVoteMask)
	return buf.Bytes()
}

func (s *Server) startGroupLootRoll(sourceGUID uint64, slot uint32, itemEntry uint32, itemCount uint32, mapID, instanceID uint32, groupID uint64, looter *session) {
	if groupID == 0 {
		return
	}
	members := s.getGroupSessions(groupID)
	if len(members) == 0 {
		return
	}
	// Player::IsAtGroupRewardDistance (Player.cpp:24160): same map instance,
	// then dungeon-always, else within CONFIG_GROUP_XP_DISTANCE (100yd) of
	// the looted object. Go proxies the object position with the looter's —
	// the looter opened the window within ~5yd of the corpse, and
	// sendLootMasterList uses the same proxy for the MasterLoot list.
	inDungeon := looter.isDungeonMap(mapID)
	var eligible []*session
	for _, m := range members {
		if m.player == nil || m.player.Map != mapID || m.player.InstanceID != instanceID {
			continue
		}
		if !inDungeon && looter.player != nil && distance3D(m.player.X, m.player.Y, m.player.Z, looter.player.X, looter.player.Y, looter.player.Z) > 100.0 {
			continue
		}
		eligible = append(eligible, m)
	}
	if len(eligible) == 0 {
		return
	}

	objectKey := lootObjectKey{MapID: mapID, InstanceID: instanceID, GUID: sourceGUID}
	rollKey := lootRollKey{Object: objectKey, Slot: slot}
	s.lootMu.Lock()
	if s.groupRolls == nil {
		s.groupRolls = make(map[lootRollKey]*activeGroupRoll)
	}
	if existing := s.groupRolls[rollKey]; existing != nil {
		s.lootMu.Unlock()
		return
	}

	s.groupsMu.Lock()
	grp := s.groups[groupID]
	s.groupsMu.Unlock()

	var disenchantID uint32
	var requiredDisenchantSkill uint32
	var allowableClass uint32 = 0xFFFFFFFF
	if s.WorldStore != nil && s.WorldStore.DB != nil {
		_ = s.WorldStore.DB.QueryRowContext(context.Background(), "SELECT DisenchantID, RequiredDisenchantSkill, AllowableClass FROM item_template WHERE entry = ?", itemEntry).Scan(&disenchantID, &requiredDisenchantSkill, &allowableClass)
	}

	baseMask := rollFlagTypePass | rollFlagTypeNeed | rollFlagTypeGreed
	// Group::GroupLoot / NeedBeforeGreed (Group.cpp:1146/1296): the disenchant
	// option is offered only when a group member's enchanting skill covers
	// the item's RequiredDisenchantSkill (m_maxEnchantingLevel, maintained on
	// group join/leave in group.go).
	if disenchantID > 0 && grp != nil && grp.MaxEnchantingLevel >= uint16(requiredDisenchantSkill) {
		baseMask |= rollFlagTypeDisenchant
	}

	roll := &activeGroupRoll{
		SourceGUID:          sourceGUID,
		Slot:                slot,
		ItemEntry:           itemEntry,
		ItemCount:           itemCount,
		RollVoteMask:        baseMask,
		GroupID:             groupID,
		MapID:               mapID,
		InstanceID:          instanceID,
		EligiblePlayers:     make(map[uint64]struct{}, len(eligible)),
		StartedAt:           time.Now(),
		Duration:            60 * time.Second,
		TotalPlayersRolling: len(eligible),
		Votes:               make(map[uint64]uint8),
		Rolls:               make(map[uint64]uint8),
	}
	for _, m := range eligible {
		roll.EligiblePlayers[m.playerGUID] = struct{}{}
	}
	if cLoot := s.creatureLoot[objectKey]; cLoot != nil {
		if li, ok := cLoot.Items[uint8(slot)]; ok {
			li.IsBlocked = true
			cLoot.Items[uint8(slot)] = li
		}
	}
	s.groupRolls[rollKey] = roll
	s.lootMu.Unlock()

	// Send personalized SMSG_LOOT_START_ROLL to each eligible member (TC Group.cpp:971)
	for _, m := range eligible {
		memberMask := baseMask
		if grp != nil && grp.LootMethod == 4 { // Need Before Greed
			if allowableClass > 0 && allowableClass != 0xFFFFFFFF && m.player != nil && m.player.Class > 0 {
				playerClassMask := uint32(1 << (m.player.Class - 1))
				if (allowableClass & playerClassMask) == 0 {
					memberMask &= ^rollFlagTypeNeed // Ineligible to roll Need
				}
			}
		}

		buf := buildLootStartRollPacket(sourceGUID, mapID, slot, itemEntry, 0, 0, itemCount, 60000, memberMask)
		_ = m.write(uint16(protocol.OpcodeSMSG_LOOT_START_ROLL), buf, true)

		// Auto-pass check for players with PassOnGroupLoot
		if m.player != nil && m.player.PassOnGroupLoot {
			m.handleLootRoll(context.Background(), buildLootRollPayload(sourceGUID, slot, rollPass))
		}
	}

	// Arm 60s countdown
	roll.Timer = time.AfterFunc(60*time.Second, func() {
		s.resolveGroupLootRoll(rollKey)
	})
}

func (s *Server) resolveGroupLootRoll(rollKey lootRollKey) {
	s.lootMu.Lock()
	roll := s.groupRolls[rollKey]
	if roll == nil {
		s.lootMu.Unlock()
		return
	}
	delete(s.groupRolls, rollKey)
	if roll.Timer != nil {
		roll.Timer.Stop()
	}
	s.lootMu.Unlock()

	var winnerGUID uint64
	var maxRoll uint8
	var winningType uint8

	if roll.TotalNeed > 0 {
		for guid, vote := range roll.Votes {
			if vote == 1 { // NEED
				rNum := roll.Rolls[guid]
				if rNum == 0 {
					rNum = uint8(rand.Intn(100) + 1)
					roll.Rolls[guid] = rNum
					rBuf := protocol.NewBuffer(35)
					rBuf.WriteU64(roll.SourceGUID)
					rBuf.WriteU32(roll.Slot)
					rBuf.WriteU64(guid)
					rBuf.WriteU32(roll.ItemEntry)
					rBuf.WriteU32(roll.RandomSuffix)
					rBuf.WriteU32(roll.RandomPropID)
					rBuf.WriteU8(rNum)
					rBuf.WriteU8(1) // NEED
					rBuf.WriteU8(0)
					s.broadcastToGroup(roll.GroupID, uint16(protocol.OpcodeSMSG_LOOT_ROLL), rBuf.Bytes())
				}
				if rNum > maxRoll || winnerGUID == 0 {
					maxRoll = rNum
					winnerGUID = guid
					winningType = 1
				}
			}
		}
	} else if roll.TotalGreed > 0 {
		for guid, vote := range roll.Votes {
			if vote == 2 || vote == 3 { // GREED or DISENCHANT
				rNum := roll.Rolls[guid]
				if rNum == 0 {
					rNum = uint8(rand.Intn(100) + 1)
					roll.Rolls[guid] = rNum
					rBuf := protocol.NewBuffer(35)
					rBuf.WriteU64(roll.SourceGUID)
					rBuf.WriteU32(roll.Slot)
					rBuf.WriteU64(guid)
					rBuf.WriteU32(roll.ItemEntry)
					rBuf.WriteU32(roll.RandomSuffix)
					rBuf.WriteU32(roll.RandomPropID)
					rBuf.WriteU8(rNum)
					rBuf.WriteU8(vote)
					rBuf.WriteU8(0)
					s.broadcastToGroup(roll.GroupID, uint16(protocol.OpcodeSMSG_LOOT_ROLL), rBuf.Bytes())
				}
				if rNum > maxRoll || winnerGUID == 0 {
					maxRoll = rNum
					winnerGUID = guid
					winningType = vote
				}
			}
		}
	}

	if winnerGUID != 0 {
		wonBuf := protocol.NewBuffer(34)
		wonBuf.WriteU64(roll.SourceGUID)
		wonBuf.WriteU32(roll.Slot)
		wonBuf.WriteU32(roll.ItemEntry)
		wonBuf.WriteU32(roll.RandomSuffix)
		wonBuf.WriteU32(roll.RandomPropID)
		wonBuf.WriteU64(winnerGUID)
		wonBuf.WriteU8(maxRoll)
		wonBuf.WriteU8(winningType)
		s.broadcastToGroup(roll.GroupID, uint16(protocol.OpcodeSMSG_LOOT_ROLL_WON), wonBuf.Bytes())

		// Reference Group.cpp:1557/1621: the roll winner is credited with the
		// winning roll value against the minimum-roll threshold.
		if winnerSess := s.findSessionByGUID(winnerGUID); winnerSess != nil {
			if winningType == 1 { // need
				winnerSess.setAchievementCriteria(criteriaTypeRollNeed, roll.ItemEntry, uint32(maxRoll))
			} else {
				winnerSess.setAchievementCriteria(criteriaTypeRollGreed, roll.ItemEntry, uint32(maxRoll))
			}
		}
		s.deliverGroupLootItem(roll, winnerGUID, winningType)
	} else {
		passBuf := protocol.NewBuffer(24)
		passBuf.WriteU64(roll.SourceGUID)
		passBuf.WriteU32(roll.Slot)
		passBuf.WriteU32(roll.ItemEntry)
		passBuf.WriteU32(roll.RandomPropID)
		passBuf.WriteU32(roll.RandomSuffix)
		s.broadcastToGroup(roll.GroupID, uint16(protocol.OpcodeSMSG_LOOT_ALL_PASSED), passBuf.Bytes())

		s.lootMu.Lock()
		if cLoot := s.creatureLoot[rollLootObjectKey(roll)]; cLoot != nil {
			if li, ok := cLoot.Items[uint8(roll.Slot)]; ok {
				li.IsBlocked = false
				cLoot.Items[uint8(roll.Slot)] = li
			}
		}
		s.lootMu.Unlock()
	}
}

func (s *Server) deliverGroupLootItem(roll *activeGroupRoll, winnerGUID uint64, winningType uint8) {
	winnerSess := s.findSessionByGUID(winnerGUID)
	if winnerSess == nil || winnerSess.player == nil || winnerSess.player.Map != roll.MapID || winnerSess.player.InstanceID != roll.InstanceID || s.CharactersStore == nil || s.CharactersStore.DB == nil {
		s.lootMu.Lock()
		if cLoot := s.creatureLoot[rollLootObjectKey(roll)]; cLoot != nil {
			if li, ok := cLoot.Items[uint8(roll.Slot)]; ok {
				li.IsBlocked = false
				li.RollWinner = winnerGUID
				cLoot.Items[uint8(roll.Slot)] = li
			}
		}
		s.lootMu.Unlock()
		return
	}
	ctx := context.Background()

	// Group::CountTheRoll disenchant arm (Group.cpp:1648-1680) gets its own
	// path: AutoStoreLoot converts the win over the whole
	// disenchant_loot_template, not a single top-chance row.
	if winningType == rollDisenchant {
		s.deliverDisenchantMats(ctx, roll, winnerGUID, winnerSess)
		return
	}

	deliveredItem := roll.ItemEntry
	deliveredCount := roll.ItemCount

	res, err := winnerSess.storeOrStackItem(ctx, winnerGUID, deliveredItem, deliveredCount)
	if err != nil {
		winnerSess.sendEquipError(equipErrInvFull, 0)
		s.lootMu.Lock()
		if cLoot := s.creatureLoot[rollLootObjectKey(roll)]; cLoot != nil {
			if li, ok := cLoot.Items[uint8(roll.Slot)]; ok {
				li.IsBlocked = false
				li.RollWinner = winnerGUID
				cLoot.Items[uint8(roll.Slot)] = li
			}
		}
		s.lootMu.Unlock()
		return
	}
	_ = winnerSess.sendInventoryItems(ctx)
	winnerSess.sendPlayerUpdate()
	slotForPush := uint32(res.Slot)
	if res.IsStack {
		slotForPush = 0xFFFFFFFF
	}
	_ = winnerSess.write(uint16(protocol.OpcodeSMSG_ITEM_PUSH_RESULT), buildLootItemPushResult(winnerGUID, res.ClientBag, slotForPush, deliveredItem, deliveredCount, res.InventoryCount), true)

	s.lootMu.Lock()
	cLoot := s.creatureLoot[rollLootObjectKey(roll)]
	if cLoot != nil {
		delete(cLoot.Items, uint8(roll.Slot))
	}
	s.lootMu.Unlock()

	if cLoot != nil {
		cLoot.broadcastRemoved(uint8(roll.Slot))
	} else {
		remBuf := protocol.NewBuffer(1)
		remBuf.WriteU8(uint8(roll.Slot))
		s.broadcastToGroup(roll.GroupID, uint16(protocol.OpcodeSMSG_LOOT_REMOVED), remBuf.Bytes())
	}
}

// deliverDisenchantMats bridges the Group::CountTheRoll disenchant arm
// (Group.cpp:1648-1680): the win is converted via Player::AutoStoreLoot over
// the whole disenchant_loot_template — every chance-rolled material is
// stored, a per-material equip error skips just that material
// (Player.cpp:25030-25057), and the disenchant cast credits the winner with
// ACHIEVEMENT_CRITERIA_TYPE_CAST_SPELL 13262 (Group.cpp:1654). A full bag
// mails the materials in C++ (SendItemRetrievalMail); Go has no player-mail
// bridge (item 13 partial), so a winner who stores nothing falls back to the
// RollWinner path and can take the item on a later loot open.
func (s *Server) deliverDisenchantMats(ctx context.Context, roll *activeGroupRoll, winnerGUID uint64, winnerSess *session) {
	// Group.cpp:1654 — the disenchant cast credits the connected winner.
	winnerSess.updateAchievementCriteria(criteriaTypeCastSpell, 13262, 0)

	type deMat struct {
		item  uint32
		count uint32
	}
	var mats []deMat
	if s.WorldStore != nil && s.WorldStore.DB != nil {
		var disenchantID uint32
		_ = s.WorldStore.DB.QueryRowContext(ctx, "SELECT DisenchantID FROM item_template WHERE entry = ?", roll.ItemEntry).Scan(&disenchantID)
		if disenchantID > 0 {
			rows, err := s.WorldStore.DB.QueryContext(ctx, "SELECT Item, MinCount, MaxCount, Chance FROM disenchant_loot_template WHERE Entry = ?", disenchantID)
			if err == nil {
				for rows.Next() {
					var item, minCount, maxCount uint32
					var chance float64
					if err := rows.Scan(&item, &minCount, &maxCount, &chance); err != nil || item == 0 {
						continue
					}
					if rand.Float64()*100.0 >= chance {
						continue
					}
					count := minCount
					if maxCount > minCount {
						count += uint32(rand.Intn(int(maxCount-minCount) + 1))
					}
					if count == 0 {
						count = 1
					}
					mats = append(mats, deMat{item: item, count: count})
				}
				rows.Close()
			}
		}
	}

	storedAny := false
	for _, m := range mats {
		res, err := winnerSess.storeOrStackItem(ctx, winnerGUID, m.item, m.count)
		if err != nil {
			winnerSess.sendEquipError(equipErrInvFull, 0)
			continue
		}
		storedAny = true
		_ = winnerSess.sendInventoryItems(ctx)
		winnerSess.sendPlayerUpdate()
		slotForPush := uint32(res.Slot)
		if res.IsStack {
			slotForPush = 0xFFFFFFFF
		}
		_ = winnerSess.write(uint16(protocol.OpcodeSMSG_ITEM_PUSH_RESULT), buildLootItemPushResult(winnerGUID, res.ClientBag, slotForPush, m.item, m.count, res.InventoryCount), true)
	}

	if len(mats) == 0 || !storedAny {
		s.lootMu.Lock()
		if cLoot := s.creatureLoot[rollLootObjectKey(roll)]; cLoot != nil {
			if li, ok := cLoot.Items[uint8(roll.Slot)]; ok {
				li.IsBlocked = false
				li.RollWinner = winnerGUID
				cLoot.Items[uint8(roll.Slot)] = li
			}
		}
		s.lootMu.Unlock()
		return
	}

	s.lootMu.Lock()
	cLoot := s.creatureLoot[rollLootObjectKey(roll)]
	if cLoot != nil {
		delete(cLoot.Items, uint8(roll.Slot))
	}
	s.lootMu.Unlock()

	if cLoot != nil {
		cLoot.broadcastRemoved(uint8(roll.Slot))
	} else {
		remBuf := protocol.NewBuffer(1)
		remBuf.WriteU8(uint8(roll.Slot))
		s.broadcastToGroup(roll.GroupID, uint16(protocol.OpcodeSMSG_LOOT_REMOVED), remBuf.Bytes())
	}
}

// handleLootRoll processes CMSG_LOOT_ROLL (0x2A0).
// Reference: WorldSession::HandleLootRoll (GroupHandler.cpp:470), Group::SendLootRoll (Group.cpp:995), Group::CountRollVote (Group.cpp:1452).
func (s *session) handleLootRoll(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 13 {
		return true
	}
	r := protocol.NewReader(payload)
	itemGUID, _ := r.ReadU64()
	itemSlot, _ := r.ReadU32()
	rollType, _ := r.ReadU8() // 0 = pass, 1 = need, 2 = greed, 3 = disenchant

	if s.server == nil || s.groupID == 0 {
		return true
	}

	rollKey := lootRollKey{Object: lootObjectKey{MapID: s.player.Map, InstanceID: s.player.InstanceID, GUID: itemGUID}, Slot: itemSlot}
	s.server.lootMu.Lock()
	roll := s.server.groupRolls[rollKey]
	if roll == nil {
		for key, other := range s.server.groupRolls {
			if other != nil && other.GroupID == s.groupID && key.Object.MapID == s.player.Map && key.Object.GUID == itemGUID && key.Slot == itemSlot && key.Object.InstanceID != s.player.InstanceID {
				s.server.lootMu.Unlock()
				return true
			}
		}
		var itemEntry uint32
		if loot := s.server.creatureLoot[rollKey.Object]; loot != nil {
			if li, ok := loot.Items[uint8(itemSlot)]; ok {
				itemEntry = li.ItemEntry
			}
		}
		s.server.lootMu.Unlock()

		rollNumber := uint8(128) // pass
		if rollType > 0 {
			rollNumber = uint8(rand.Intn(100) + 1) // 1..100
		}

		buf := protocol.NewBuffer(35)
		buf.WriteU64(itemGUID)     // sourceGuid (guid of loot object)
		buf.WriteU32(itemSlot)     // slot
		buf.WriteU64(s.playerGUID) // targetGuid (rolling player)
		buf.WriteU32(itemEntry)    // itemEntryId
		buf.WriteU32(0)            // randomSuffix
		buf.WriteU32(0)            // randomPropId
		buf.WriteU8(rollNumber)    // rollNumber
		buf.WriteU8(rollType)      // rollType (0: pass, 1: need, 2: greed, 3: disenchant)
		buf.WriteU8(0)             // autoPass
		s.server.broadcastToGroup(s.groupID, uint16(protocol.OpcodeSMSG_LOOT_ROLL), buf.Bytes())
		return true
	}
	if roll.GroupID != s.groupID || roll.MapID != s.player.Map || roll.InstanceID != s.player.InstanceID {
		s.server.lootMu.Unlock()
		return true
	}
	if _, eligible := roll.EligiblePlayers[s.playerGUID]; !eligible {
		s.server.lootMu.Unlock()
		return true
	}

	// If player already voted on this roll, ignore duplicate
	if _, voted := roll.Votes[s.playerGUID]; voted {
		s.server.lootMu.Unlock()
		return true
	}

	// Group::CountRollVote (Group.cpp:1452-1494): the vote switch has no
	// default arm — a rollType outside 0..3 (pass/need/greed/disenchant)
	// records no vote, counts nothing, and fires no achievement.
	if rollType > rollDisenchant {
		s.server.lootMu.Unlock()
		return true
	}

	// In Need Before Greed, verify that player can need if they chose NEED (TC GroupHandler.cpp:487)
	if rollType == rollNeed && s.groupID != 0 {
		s.server.groupsMu.Lock()
		grp := s.server.groups[s.groupID]
		s.server.groupsMu.Unlock()
		if grp != nil && grp.LootMethod == 4 && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
			var allowableClass uint32 = 0xFFFFFFFF
			_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT AllowableClass FROM item_template WHERE entry = ?", roll.ItemEntry).Scan(&allowableClass)
			if allowableClass > 0 && allowableClass != 0xFFFFFFFF && s.player.Class > 0 {
				playerClassMask := uint32(1 << (s.player.Class - 1))
				if (allowableClass & playerClassMask) == 0 {
					rollType = rollPass // Ineligible to roll Need
				}
			}
		}
	}

	roll.Votes[s.playerGUID] = rollType

	switch rollType {
	case 0:
		roll.TotalPass++
	case 1:
		roll.TotalNeed++
		s.updateAchievementCriteria(criteriaTypeRollNeedCount, 0, 1)
	case 2:
		roll.TotalGreed++
		s.updateAchievementCriteria(criteriaTypeRollGreedCount, 0, 1)
	case 3: // DISENCHANT shares the greed total (Group.cpp:1484).
		// C++ HandleLootRoll fires no achievement for a disenchant vote
		// (the NEED/GREED-only switch, GroupHandler.cpp:483-490); the
		// disenchant cast credits the winner at delivery (Group.cpp:1654).
		roll.TotalGreed++
	default:
		roll.TotalGreed++
		s.updateAchievementCriteria(criteriaTypeRollGreedCount, 0, 1)
	}

	itemEntry := roll.ItemEntry
	randomSuffix := roll.RandomSuffix
	randomPropID := roll.RandomPropID
	totalDone := (roll.TotalPass + roll.TotalNeed + roll.TotalGreed) >= roll.TotalPlayersRolling
	s.server.lootMu.Unlock()

	voteRollNumber := uint8(128)
	voteRollType := rollType
	if rollType == 1 { // NEED
		voteRollNumber = 0
		voteRollType = 0
	} else if rollType == 0 { // PASS
		voteRollNumber = 128
		voteRollType = 0
	}

	buf := protocol.NewBuffer(35)
	buf.WriteU64(itemGUID)
	buf.WriteU32(itemSlot)
	buf.WriteU64(s.playerGUID)
	buf.WriteU32(itemEntry)
	buf.WriteU32(randomSuffix)
	buf.WriteU32(randomPropID)
	buf.WriteU8(voteRollNumber)
	buf.WriteU8(voteRollType)
	buf.WriteU8(0) // autoPass
	s.server.broadcastToGroup(s.groupID, uint16(protocol.OpcodeSMSG_LOOT_ROLL), buf.Bytes())

	if totalDone {
		s.server.resolveGroupLootRoll(rollKey)
	}

	return true
}

// handleOptOutOfLoot processes CMSG_OPT_OUT_OF_LOOT (0x409).
// Reference: WorldSession::HandleOptOutOfLootOpcode (GroupHandler.cpp:1066).
func (s *session) handleOptOutOfLoot(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 4 {
		return true
	}
	r := protocol.NewReader(payload)
	passOnLoot, _ := r.ReadU32()
	s.player.PassOnGroupLoot = (passOnLoot != 0)
	return true
}

// onPlayerLeaveGroupRolls automatically records a pass vote for a player who leaves,
// gets kicked from, or disbands a group during active loot rolls, preventing stalled roll countdowns.
// Mirrors TrinityCore Group::CountRollVote and Group::RemoveMember (Group.cpp:780-810, 1450-1490).
func (s *Server) onPlayerLeaveGroupRolls(leavingGUID uint64, groupID uint64) {
	if s == nil || leavingGUID == 0 || groupID == 0 {
		return
	}
	s.lootMu.Lock()
	var keysToResolve []lootRollKey
	for key, roll := range s.groupRolls {
		if roll != nil && roll.GroupID == groupID {
			if _, eligible := roll.EligiblePlayers[leavingGUID]; !eligible {
				continue
			}
			if _, voted := roll.Votes[leavingGUID]; !voted {
				roll.Votes[leavingGUID] = rollPass
				roll.TotalPass++
			}
			if (roll.TotalPass + roll.TotalNeed + roll.TotalGreed) >= roll.TotalPlayersRolling {
				keysToResolve = append(keysToResolve, key)
			}
		}
	}
	s.lootMu.Unlock()

	for _, key := range keysToResolve {
		s.resolveGroupLootRoll(key)
	}
}
