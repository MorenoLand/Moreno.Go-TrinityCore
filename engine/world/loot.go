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
	// FreeForAll mirrors LootItem::freeforall (Loot.h:136), set from the
	// item template's ITEM_FLAG_MULTI_DROP (ItemTemplate.h:163) in the
	// LootItem constructor (Loot.cpp:41): looting the row does not remove
	// it for other viewers. Free-for-all rows are excluded from the
	// permission-ladder arms of LootView and rendered separately
	// (Loot.cpp:624/679/698 vs 743-757), never start group rolls and are
	// never blocked (Group.cpp:1106/1261/1410), and each viewer's take
	// marks only their own PlayerFFAItems entry (Player.cpp:25107-25113).
	FreeForAll bool
}

// itemFlagsCuIgnoreQuestStatus / itemFlagsCuFollowLootRules mirror
// ITEM_FLAGS_CU_IGNORE_QUEST_STATUS and ITEM_FLAGS_CU_FOLLOW_LOOT_RULES
// (ItemTemplate.h:225-226).
const (
	itemFlagsCuIgnoreQuestStatus uint32 = 0x0002
	itemFlagsCuFollowLootRules   uint32 = 0x0004
)

// itemFlagMultiDrop mirrors ITEM_FLAG_MULTI_DROP (ItemTemplate.h:163):
// looting the item does not remove it from available loot.
const itemFlagMultiDrop uint32 = 0x00000800

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
	// FFATaken records per-viewer takes of free-for-all (multi-drop) rows,
	// mirroring the per-viewer is_looted in Loot::PlayerFFAItems
	// (Loot.cpp:271-293, Player.cpp:25107-25113): the shared Items row is
	// never deleted by a take, so each viewer's copy is tracked here by
	// slot. Viewers who release without taking drop out of the map with
	// removeViewer.
	FFATaken map[uint8]map[uint64]bool
	// QuestFFATaken is the same per-viewer take model for free-for-all
	// quest rows, keyed by quest index into QuestItems: a free-for-all
	// quest row survives a take (Player::StoreLootItem, Player.cpp:25097,
	// item->is_looted stays false for freeforall) so every other viewer
	// keeps their own copy (Loot::NotifyQuestItemRemoved stays the
	// taker-only SMSG_LOOT_REMOVED in that arm, Player.cpp:25099).
	QuestFFATaken map[uint8]map[uint64]bool
	// GOLootRules mirrors chest.groupLootRules (gameobject_template.data15,
	// GameObjectData.h:104): only chests with the flag set run the group
	// distribution calls (GroupLoot / NeedBeforeGreed / MasterLoot) and the
	// round-robin looter rotation at open (Player.cpp:8590-8627). Chests
	// without the flag still ride the group permission ladder
	// (Player.cpp:8641-8662) but never start rolls or receive the
	// master-loot list. Non-GO loot leaves this false and is unaffected.
	GOLootRules bool
}

// storeLootTemplateRow routes one rolled loot-template row into Items or
// QuestItems, mirroring Loot::AddItem (Loot.cpp:141-152) where needs_quest
// rows go to quest_items with the MAX_NR_QUEST_ITEMS cap.
func storeLootTemplateRow(loot *activeLootState, slot, qidx *uint8, itemID, count, displayID, quality, startQuest, customFlags uint32, questRequired, freeForAll bool) {
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
			FreeForAll:    freeForAll,
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
		FreeForAll:    freeForAll,
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
	flags         uint32
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
	// Loot::AddItem (Loot.cpp:143-149): count = urand(mincount, maxcount)
	// with no minimum-1 clamp — a zero roll yields zero stacks and the row
	// is dropped entirely.
	if count == 0 {
		return
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
		storeLootTemplateRow(loot, slot, qidx, row.itemID, c, row.displayID, row.quality, row.startQuest, row.customFlags, row.questRequired, row.flags&itemFlagMultiDrop != 0)
	}
}

// loadLootTemplateRows loads one loot template table's rows for an entry.
// The full column set degrades through the two historical fallbacks when the
// world schema lacks the newer columns (matching the pre-existing
// isMissingColumn fallbacks at the call sites).
func loadLootTemplateRows(ctx context.Context, wdb *sql.DB, table string, lootID int64, lootMode uint32) []lootTemplateRow {
	full := `SELECT l.Item, l.Reference, l.Chance, l.MinCount, l.MaxCount, COALESCE(t.displayid, 0), COALESCE(t.Quality, 0),
			COALESCE(l.QuestRequired, 0), COALESCE(t.StartQuest, 0), COALESCE(t.flagsCustom, 0),
			COALESCE(l.LootMode, 1), COALESCE(l.GroupId, 0), COALESCE(t.Stackable, 0), COALESCE(t.Flags, 0)
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
		var questRequired, startQuest, customFlags, lootModeRow, groupID, maxStack, flags int64
		var chance float64
		cols, colErr := rows.Columns()
		if colErr != nil {
			continue
		}
		var scanErr error
		if len(cols) >= 14 {
			scanErr = rows.Scan(&itemID, &reference, &chance, &minCount, &maxCount, &displayID, &quality,
				&questRequired, &startQuest, &customFlags, &lootModeRow, &groupID, &maxStack, &flags)
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
		r.flags = uint32(flags)
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

// creatureLootSparkleVisible mirrors Player::isAllowedToLoot (Player.cpp:18104-18150)
// as consulted by Unit::BuildValuesUpdateBlockForPlayer's UNIT_DYNAMIC_FLAGS arm
// (Unit.cpp:13921-13931): the UNIT_DYNFLAG_LOOTABLE bit is masked per viewer, so a
// corpse only sparkles for players who may actually loot it. Only the arms Go can
// model are evaluated:
//   - the fully-looted arm (Player.cpp:18109): motion.Looted (set by clearCreatureLoot)
//     or a loot store with no rows left hides the sparkle for everyone;
//   - the group loot-method ladder (Player.cpp:18125-18147): MASTER_LOOT and
//     FREE_FOR_ALL show the sparkle to every group member; ROUND_ROBIN shows it to
//     the round-robin holder (or everyone while no holder is assigned / anyone with
//     a personal row); GROUP_LOOT and NEED_BEFORE_GREED show it to the round-robin
//     holder, to everyone while an over-threshold row stands (rolls are about to
//     launch), or to anyone with a personal row. A solo tap shows the sparkle to
//     the tapper only (Player.cpp:18124); the skinning arm (Player.cpp:18118) rides
//     the same leg because Go stores the skinner as a GroupID-0 owner.
//   - no owner on record defaults to visible, matching creatureLootAllowed.
//
// Loot::hasItemForAll / hasItemFor / hasOverThresholdItem (Loot.cpp:516-585) feed the
// personal-row arms from the server loot store; when the store has no rows yet (Go
// fills loot lazily at first open) the personal arms default to visible.
// Documented no-bridge arms: HasPendingBind (Player.cpp:18107, no Go bind model),
// LootItem conditions (Loot.cpp:519, no Go condition model — every row counts as
// unconditional), the isDead arm (Go only broadcasts LOOTABLE on death paths, where
// C++'s arm is definitionally true), and IsDamageEnoughForLootingAndReward (the kill
// path already sheds LOOTABLE|TAPPED on a failed requirement, kill.go).
func (s *Server) creatureLootSparkleVisible(mapID, instanceID uint32, targetGUID, viewerGUID, viewerGroupID uint64) bool {
	if s == nil {
		return true
	}
	if motion := s.findCreatureMotion(mapID, instanceID, targetGUID); motion != nil && motion.Looted {
		return false
	}
	s.lootMu.Lock()
	key := lootObjectKey{MapID: mapID, InstanceID: instanceID, GUID: targetGUID}
	owner, found := s.creatureLootOwners[key]
	loot := s.creatureLoot[key]
	if low, entry := uint32(targetGUID&0x00FFFFFF), uint32(targetGUID>>24&0x00FFFFFF); !found || loot == nil {
		if std := creatureWorldGUID(low, entry); std != targetGUID {
			skey := lootObjectKey{MapID: mapID, InstanceID: instanceID, GUID: std}
			if o, ok := s.creatureLootOwners[skey]; ok && !found {
				owner, found = o, true
			}
			if l := s.creatureLoot[skey]; l != nil && loot == nil {
				loot = l
			}
		}
	}
	s.lootMu.Unlock()
	if loot != nil && lootFullyLooted(loot) {
		return false
	}
	// No loot left for this viewer at all (Player.cpp:18113-18114). Skipped
	// while the store has no rows: Go fills lazily at first open, so an
	// unfilled store must not hide the sparkle.
	if loot != nil && !lootHasItemForAll(loot) && !lootHasItemForViewer(loot, viewerGUID) {
		return false
	}
	if !found {
		return true
	}
	if owner.GroupID != 0 {
		if viewerGroupID == 0 || viewerGroupID != owner.GroupID {
			return false
		}
		grp := s.getGroup(viewerGroupID)
		if grp == nil {
			return true
		}
		switch grp.LootMethod {
		case 2, 0: // MASTER_LOOT, FREE_FOR_ALL (Player.cpp:18129-18131)
			return true
		case 1: // ROUND_ROBIN (Player.cpp:18132-18137)
			if loot == nil || loot.RoundRobinPlayer == 0 || loot.RoundRobinPlayer == viewerGUID {
				return true
			}
			return lootHasItemForViewer(loot, viewerGUID)
		default: // GROUP_LOOT, NEED_BEFORE_GREED (Player.cpp:18138-18147)
			if loot == nil || loot.RoundRobinPlayer == 0 || loot.RoundRobinPlayer == viewerGUID {
				return true
			}
			if loot.hasOverThresholdItem(grp.LootThreshold) {
				return true
			}
			return lootHasItemForViewer(loot, viewerGUID)
		}
	}
	return viewerGUID == owner.PlayerGUID
}

// lootHasItemForAll mirrors Loot::hasItemForAll (Loot.cpp:516-526): gold or any
// unlooted, non-free-for-all row. Go deletes taken rows from the Items map, so
// presence implies !is_looted.
func lootHasItemForAll(loot *activeLootState) bool {
	if loot == nil {
		return false
	}
	if loot.Money != 0 {
		return true
	}
	for _, it := range loot.Items {
		if !it.FreeForAll {
			return true
		}
	}
	return false
}

// lootHasItemForViewer mirrors Loot::hasItemFor (Loot.cpp:529-564): any
// free-for-all row the viewer hasn't taken, or any quest row. Per-viewer quest
// visibility (AllowedForPlayer) is approximated by row presence — the real check
// needs a DB-backed quest evaluation per broadcast viewer.
func lootHasItemForViewer(loot *activeLootState, viewerGUID uint64) bool {
	if loot == nil {
		return false
	}
	if len(loot.QuestItems) != 0 {
		return true
	}
	for slot, it := range loot.Items {
		if it.FreeForAll && !loot.FFATaken[slot][viewerGUID] {
			return true
		}
	}
	return false
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
		// A free-for-all quest row stays in QuestItems after a take; the
		// viewer's own copy is gone once QuestFFATaken records it
		// (Player.cpp:25097-25104), so it must not render for them again.
		if item.FreeForAll && loot.QuestFFATaken[idx] != nil && loot.QuestFFATaken[idx][s.playerGUID] {
			continue
		}
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
	// Loot::FillQuestLoot (Loot.cpp:296-315): no quest rows render once the
	// normal rows fill the 18-row window, and the per-viewer quest list
	// stops once normal + quest rows hit 18. NormalSlotCount mirrors
	// C++ items.size() (captured at fill time; the C++ vector never shrinks
	// on take either).
	room := int(maxNormalLootItems) - int(loot.NormalSlotCount)
	if room <= 0 {
		return nil
	}
	if len(indices) > room {
		indices = indices[:room]
	}
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
		// Loot::hasOverThresholdItem (Loot.cpp:573-580): free-for-all rows
		// never count as over-threshold.
		if !item.FreeForAll && item.Quality >= uint32(threshold) {
			return true
		}
	}
	return false
}

// lootFullyLooted mirrors Loot::isLooted (Loot.h:236): gold == 0 and
// unlootedCount == 0. unlootedCount counts one entry per viewer per
// free-for-all row (Loot::FillFFALoot, Loot.cpp:271-293), so a multi-drop
// row keeps the loot open until every current viewer has taken their copy;
// non-free-for-all rows are deleted from Items at take time, so any that
// remain mean the loot is not fully taken. Viewers who released without
// taking drop out of the check with removeViewer (C++ keeps their
// PlayerFFAItems entries past release — a latent leak there — so Go clears
// slightly earlier than C++ in that corner).
func lootFullyLooted(l *activeLootState) bool {
	if l == nil || l.Money != 0 {
		return false
	}
	for qidx, qit := range l.QuestItems {
		if !qit.FreeForAll {
			return false
		}
		// Loot::isLooted via the FillQuestLoot per-viewer count
		// (Loot.cpp:306-313, `item.freeforall || !item.is_blocked`): a
		// free-for-all quest row keeps the loot open until every current
		// viewer has taken their own copy (QuestFFATaken), exactly like
		// the per-viewer counting for normal FFA rows below.
		taken := l.QuestFFATaken[qidx]
		for guid := range l.Viewers {
			if !taken[guid] {
				return false
			}
		}
	}
	for slot, it := range l.Items {
		if !it.FreeForAll {
			return false
		}
		taken := l.FFATaken[slot]
		for guid := range l.Viewers {
			if !taken[guid] {
				return false
			}
		}
	}
	return true
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
// re-roll, Player.cpp:8574-8578), the battleground CanActivateGO gate,
// the respawn-time release arm (Player.cpp:8560, needs live GO respawn
// timers; static GOs are per-client), the per-viewer GO-flag masking for
// groupLootRules chests (GameObject.cpp:2543/2606, no per-viewer GO-flag
// model), and the fishing/fishing-hole/fishing-junk arms have no Go model.
// generateMoneyLootValue mirrors Loot::generateMoneyLoot (Loot.cpp:433):
// the maxAmount<=minAmount arm takes maxAmount (not minAmount), and a zero
// max means no money at all. The (maxAmount-minAmount)>=32700 arm is a
// 32-bit urand overflow guard with no Go analog (rand.Intn runs on 64-bit
// ints), and RATE_DROP_MONEY has no Go model (rate 1).
func generateMoneyLootValue(minGold, maxGold int64) uint32 {
	if maxGold <= 0 {
		return 0
	}
	if maxGold > minGold {
		return uint32(minGold + int64(rand.Intn(int(maxGold-minGold+1))))
	}
	return uint32(maxGold)
}

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
	var goType uint8
	var goData15 int64
	err := wdb.QueryRowContext(ctx, `SELECT g.map, g.position_x, g.position_y, g.position_z, COALESCE(t.data1, 0), COALESCE(t.type, 0), COALESCE(t.data15, 0)
		FROM gameobject AS g
		JOIN gameobject_template AS t ON t.entry = g.id
		WHERE g.guid = ? AND g.id = ? LIMIT 1`, lowGUID, entry).Scan(&goMap, &goX, &goY, &goZ, &data1, &goType, &goData15)
	if err != nil {
		_ = wdb.QueryRowContext(ctx, `SELECT g.map, g.position_x, g.position_y, g.position_z, COALESCE(t.data1, 0), COALESCE(t.type, 0), COALESCE(t.data15, 0)
			FROM gameobject AS g
			JOIN gameobject_template AS t ON t.entry = g.id
			WHERE g.guid = ? LIMIT 1`, lowGUID).Scan(&goMap, &goX, &goY, &goZ, &data1, &goType, &goData15)
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
		loot = &activeLootState{TargetGUID: targetGUID, MapID: goMap, InstanceID: s.player.InstanceID, LootType: lootType, Items: make(map[uint8]lootItem),
			// Player.cpp:8590: the group distribution/round-robin arms run
			// only for chests with chest.groupLootRules (data15).
			GOLootRules: goType == GameObjectTypeChest && goData15 != 0}
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
	// Player::SendLoot GO arm (Player.cpp:8607-8609): Loot::generateMoneyLoot
	// rolls the gameobject_template_addon min/max gold once the loot exists
	// (the C++ lootMode>0 gate has no Go model; lootable GOs always qualify).
	var addonMinGold, addonMaxGold int64
	if err = wdb.QueryRowContext(ctx, `SELECT COALESCE(mingold, 0), COALESCE(maxgold, 0) FROM gameobject_template_addon WHERE entry = ?`, entry).Scan(&addonMinGold, &addonMaxGold); err == nil {
		if money := generateMoneyLootValue(addonMinGold, addonMaxGold); money != 0 {
			loot.Money = money
		}
	}
	s.server.autoStoreLootCurrencyTokens(ctx, loot, s)
	// Player::SendLoot GO arm (Player.cpp:8592-8606): the round-robin looter
	// update before the fill and the unconditional advance after it run only
	// when groupRules holds (chest with chest.groupLootRules); a chest
	// without the flag never rotates the group looter.
	if s.server != nil && s.groupID != 0 && loot.GOLootRules {
		s.server.groupsMu.Lock()
		grp := s.server.groups[s.groupID]
		if grp != nil && loot.RoundRobinPlayer == 0 && grp.LootMethod != 0 {
			grp.updateLooter(s.server, goMap, s.player.InstanceID, goX, goY, goZ)
			loot.RoundRobinPlayer = grp.LooterGUID
			// Unit.cpp:11270-11271 / Player.cpp:8603 (the !loot->empty()
			// gate): after the fill, C++ unconditionally advances the
			// group looter for the next loot, so consecutive loots rotate
			// even while the current looter stays in range. Go fills
			// lazily at first open, so the advance runs here, right after
			// this loot's looter is captured; a loot that is never opened
			// never advances the role.
			if loot.Money != 0 || len(loot.Items) != 0 {
				grp.advanceLooter(s.server, goMap, s.player.InstanceID, goX, goY, goZ)
			}
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
	loot.Money = generateMoneyLootValue(minGold, maxGold)
	// Loot::FillLoot (Loot.cpp:188) drives LootTemplate::Process over the
	// creature template; Go has no creature loot modes, so the default mode
	// (Unit.cpp:11250 passes creature->GetLootMode() in C++).
	s.server.fillLootTemplate(ctx, wdb, "creature_loot_template", lootID, lootModeDefault, loot)
	s.server.autoStoreLootCurrencyTokens(ctx, loot, s)
	if s.server != nil && s.groupID != 0 {
		s.server.groupsMu.Lock()
		grp := s.server.groups[s.groupID]
		if grp != nil && loot.RoundRobinPlayer == 0 && grp.LootMethod != 0 {
			grp.updateLooter(s.server, target.Map, target.InstanceID, target.X, target.Y, target.Z)
			loot.RoundRobinPlayer = grp.LooterGUID
			// Unit.cpp:11270-11271 (the !loot->empty() gate): after the
			// kill-time fill, C++ unconditionally advances the group
			// looter for the next kill's loot, so consecutive loots
			// rotate even while the current looter stays in range. Go
			// fills lazily at first open, so the advance runs here,
			// right after this loot's looter is captured; a loot that is
			// never opened never advances the role.
			if loot.Money != 0 || len(loot.Items) != 0 {
				grp.advanceLooter(s.server, target.Map, target.InstanceID, target.X, target.Y, target.Z)
			}
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
	fishingZoneSkill := func(entry uint32) int64 {
		var v int64
		if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT skill FROM skill_fishing_base_level WHERE entry = ?", entry).Scan(&v); err != nil && !missingTable(err) {
			return 0
		}
		return v
	}
	// GameObject.cpp:1737-1739: the fishing base skill level is looked up for
	// the subzone first, falling back to the zone.
	zoneSkill = fishingZoneSkill(s.areaID)
	if zoneSkill == 0 {
		zoneSkill = fishingZoneSkill(s.player.Zone)
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
		// GameObject.cpp:1748: chance = int32(pow((double)skill/zone_skill, 2) * 100).
		ratio := float64(fishingSkill) / float64(zoneSkill)
		chance = int(ratio * ratio * 100)
		if chance < 1 {
			chance = 1
		}
	}
	success := forceSuccess || s.fishingHoleNearby(ctx, goState) || rand.Intn(100)+1 <= chance
	loadRows := func(entry uint32, lootMode uint32) error {
		rows, queryErr := s.server.WorldStore.DB.QueryContext(ctx, `SELECT l.Item, l.Chance, l.MinCount, l.MaxCount, COALESCE(t.displayid, 0), COALESCE(t.Quality, 0), COALESCE(t.Flags, 0)
			FROM fishing_loot_template AS l LEFT JOIN item_template AS t ON t.entry = l.Item
			WHERE l.Entry = ? AND (COALESCE(l.LootMode, 1) & ?) <> 0 ORDER BY l.Item LIMIT 18`, entry, lootMode)
		if queryErr != nil {
			return queryErr
		}
		defer rows.Close()
		var slot uint8
		for rows.Next() && slot < maxNormalLootItems {
			var itemID int64
			var chance float64
			var minCount, maxCount, displayID, quality, flags int64
			if scanErr := rows.Scan(&itemID, &chance, &minCount, &maxCount, &displayID, &quality, &flags); scanErr != nil {
				continue
			}
			if chance > 0 && rand.Float64()*100 > chance {
				continue
			}
			count := uint32(minCount)
			if maxCount > minCount {
				count += uint32(rand.Intn(int(maxCount - minCount + 1)))
			}
			// Loot::AddItem (Loot.cpp:143-149): no minimum-1 clamp — a zero
			// count roll drops the row entirely instead of forcing one.
			if count == 0 {
				continue
			}
			loot.Items[slot] = lootItem{Slot: slot, ItemEntry: uint32(itemID), Count: count, DisplayInfoID: uint32(displayID), Quality: uint32(quality), FreeForAll: uint32(flags)&itemFlagMultiDrop != 0}
			slot++
		}
		return rows.Err()
	}
	lootMode := uint32(1)
	if !success {
		lootMode = 0x8000
	}
	// GameObject::getFishLoot / getFishLootJunk (GameObject.cpp:943-980): the
	// fishing loot template entry is resolved subzone first, then zone, then
	// zone 1 as the default.
	if err := loadRows(s.areaID, lootMode); err != nil && !missingTable(err) {
		return true
	}
	if len(loot.Items) == 0 && s.areaID != s.player.Zone {
		if err := loadRows(s.player.Zone, lootMode); err != nil && !missingTable(err) {
			return true
		}
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
		// GameObject.cpp:1763: LookupFishingHoleAround(20.0f + CONTACT_DISTANCE).
		if object != nil && object.Type == GameObjectTypeFishingHole && object.Map == bobber.Map && distance3D(object.X, object.Y, object.Z, bobber.X, bobber.Y, bobber.Z) <= 20.5 {
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
		LIMIT 1`, bobber.Map, GameObjectTypeFishingHole, bobber.X-20.5, bobber.X+20.5, bobber.Y-20.5, bobber.Y+20.5).Scan(&found)
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
		// LootView (Loot.cpp:624/679/698): free-for-all (multi-drop) rows
		// are excluded from every permission-ladder arm and rendered in
		// the FFA section below (Loot.cpp:743-757).
		if it.FreeForAll {
			continue
		}
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
	// LootView FFA arm (Loot.cpp:743-757): multi-drop rows render after the
	// quest section at their raw slot with the permission default
	// (OWNER/ALLOW_LOOT — never the ladder's locked/roll types), one copy
	// per viewer; a viewer who already took their copy (FFATaken) no
	// longer sees the row, mirroring the per-viewer is_looted in
	// Loot::PlayerFFAItems.
	var ffaShown []lootItem
	for _, it := range items {
		if !it.FreeForAll {
			continue
		}
		if taken := loot.FFATaken[it.Slot]; taken[s.playerGUID] {
			continue
		}
		if !s.lootItemAllowedForPlayer(ctx, it, false) {
			continue
		}
		ffaShown = append(ffaShown, it)
	}
	packet := protocol.NewBuffer(8 + 1 + 4 + 1 + (len(shown)+len(questList)+len(ffaShown))*22)
	packet.WriteU64(loot.TargetGUID)
	packet.WriteU8(loot.LootType)
	packet.WriteU32(loot.Money)
	packet.WriteU8(uint8(len(shown) + len(questList) + len(ffaShown)))
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
	// under master loot; under group loot they render ROLL_ONGOING while a
	// roll blocks them (Loot.cpp:731-735) and ALLOW_LOOT otherwise;
	// otherwise they ride the permission default (ownerSlot, matching the
	// normal-item legs above).
	for pos, qidx := range questList {
		qit := loot.QuestItems[qidx]
		// Loot.cpp:709-711: the quest section rides the permission default
		// (OWNER/ALLOW) unless follow_loot_rules pulls it into the master
		// arm, which always renders MASTER for the master looter's view.
		// The default follows baseSlot: under the group ladder the
		// permission arms render ALLOW_LOOT (Loot.cpp:709:
		// OWNER_PERMISSION ? OWNER : ALLOW_LOOT).
		var qSlotType uint8 = baseSlot
		if qit.CustomFlags&itemFlagsCuFollowLootRules != 0 && grp != nil {
			switch grp.LootMethod {
			case 2: // Master Loot
				if s.playerGUID == grp.MasterLooter {
					qSlotType = 2 // LOOT_SLOT_TYPE_MASTER
				} else {
					qSlotType = 3 // LOOT_SLOT_TYPE_LOCKED
				}
			case 3, 4: // Group Loot / Need Before Greed
				// Loot.cpp:731-735: follow_loot_rules quest items under
				// GROUP/ROUND_ROBIN permission render ROLL_ONGOING while
				// blocked, ALLOW_LOOT once the roll clears.
				qSlotType = 0 // LOOT_SLOT_TYPE_ALLOW_LOOT
				if qit.IsBlocked {
					qSlotType = 1 // LOOT_SLOT_TYPE_ROLL_ONGOING
				}
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
	// LootView FFA arm rows (Loot.cpp:743-757): raw slot, the LootItem row,
	// then the permission default — baseSlot rides the group ladder
	// (ALLOW_LOOT under GROUP/MASTER/RESTRICTED/ROUND_ROBIN, matching
	// Loot.cpp:701's OWNER_PERMISSION ? OWNER : ALLOW_LOOT) and is OWNER
	// for solo creature loot.
	for _, it := range ffaShown {
		packet.WriteU8(it.Slot)
		packet.WriteU32(it.ItemEntry)
		packet.WriteU32(it.Count)
		packet.WriteU32(it.DisplayInfoID)
		packet.WriteU32(0) // RandomSuffix (Loot.cpp:589: randomSuffix before randomPropertyId)
		packet.WriteU32(0) // RandomPropertyId
		packet.WriteU8(baseSlot)
	}
	if err := s.write(uint16(protocol.OpcodeSMSG_LOOT_RESPONSE), packet.Bytes(), true); err != nil {
		return err
	}
	s.debug("loot response sent", "account", s.accountName, "target", loot.TargetGUID, "money", loot.Money, "items", len(loot.Items))

	// Player::SendLoot GO arm (Player.cpp:8614-8627): the GroupLoot /
	// NeedBeforeGreed / MasterLoot distribution calls (roll starts and the
	// SMSG_LOOT_MASTER_LIST) run only for chests with chest.groupLootRules.
	// A GO chest without the flag keeps the group permission ladder in the
	// render above but never starts rolls or receives the master list.
	// Creature loot always qualifies.
	groupDistribute := uint16(loot.TargetGUID>>48) != 0xF110 || loot.GOLootRules
	if grp != nil && groupDistribute {
		if grp.LootMethod == 2 { // Master Loot
			s.sendLootMasterList(loot)
		} else if grp.LootMethod == 3 || grp.LootMethod == 4 { // Group Loot / Need Before Greed
			for _, it := range sortedLootItems(loot.Items) {
				// Group::GroupLoot / NeedBeforeGreed (Group.cpp:1106/1261):
				// free-for-all rows never start rolls.
				if !it.FreeForAll && it.Quality >= uint32(grp.LootThreshold) {
					s.server.startGroupLootRoll(loot.TargetGUID, uint32(it.Slot), it.ItemEntry, it.Count, loot.MapID, loot.InstanceID, s.groupID, s)
				}
			}
			// Group::GroupLoot / NeedBeforeGreed quest-item loops
			// (Group.cpp:1192-1252/1337-1380): follow_loot_rules quest items
			// roll under group methods regardless of threshold, in
			// items.size()+index slot order (NormalSlotCount + qidx).
			qindices := make([]uint8, 0, len(loot.QuestItems))
			for qidx := range loot.QuestItems {
				qindices = append(qindices, qidx)
			}
			sort.Slice(qindices, func(i, j int) bool { return qindices[i] < qindices[j] })
			for _, qidx := range qindices {
				qit := loot.QuestItems[qidx]
				if qit.CustomFlags&itemFlagsCuFollowLootRules == 0 {
					continue
				}
				s.server.startGroupQuestLootRoll(loot.TargetGUID, uint32(loot.NormalSlotCount)+uint32(qidx), qidx, qit.ItemEntry, qit.Count, loot.MapID, loot.InstanceID, s.groupID, s)
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
		// Group::MasterLoot (Group.cpp:1435-1447): the master-looter list
		// carries the members at Player::IsAtGroupRewardDistance
		// (Player.cpp:24160) of the looted object — same map and instance,
		// within MaxGroupXPDistance (default 74, not 100).
		if m.player != nil && m.player.Map == s.player.Map && m.player.InstanceID == s.player.InstanceID && distance3D(s.player.X, s.player.Y, s.player.Z, m.player.X, m.player.Y, m.player.Z) <= s.server.Config.MaxGroupXPDistance {
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
	// Loot::clear (Loot.cpp:117-138) also drops the per-viewer FFA takes —
	// the old corpse loot's FFATaken must not leak into the skinning window.
	loot.Items = make(map[uint8]lootItem)
	loot.QuestItems = nil
	loot.FFATaken = nil
	loot.QuestFFATaken = nil
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
		s.server.autoStoreLootCurrencyTokens(ctx, loot, s)
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
	// (Creature.cpp:3379). Loot::clear (Loot.cpp:117-138) also drops the
	// per-viewer FFA takes — the old loot's FFATaken must not leak into
	// the pickpocket window.
	loot.Items = make(map[uint8]lootItem)
	loot.QuestItems = nil
	loot.FFATaken = nil
	loot.QuestFFATaken = nil
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
		s.server.autoStoreLootCurrencyTokens(ctx, loot, s)
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
		// instance, then dungeon-always, else within MaxGroupXPDistance
		// (CONFIG_GROUP_XP_DISTANCE, default 74).
		inDungeon := s.isDungeonMap(s.player.Map)
		for _, m := range allGroupSess {
			if m.player != nil && m.player.Map == s.player.Map && m.player.InstanceID == s.player.InstanceID && (inDungeon || distance3D(s.player.X, s.player.Y, s.player.Z, m.player.X, m.player.Y, m.player.Z) <= s.server.Config.MaxGroupXPDistance) {
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
	// the corpse. lootFullyLooted extends the check to multi-drop rows.
	if lootFullyLooted(s.activeLoot) {
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
	// gate is re-checked at take time for every item, not just quest
	// rows; an item the viewer no longer qualifies for (faction recipe
	// learned, quest completed since the window opened, a raw take
	// opcode on a hidden row) answers with a silent loot release,
	// not an error.
	if !s.lootItemAllowedForPlayer(ctx, it, false) {
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
			// Group::MasterLoot (Group.cpp:1408-1413) never blocks
			// free-for-all rows, so they stay takeable by everyone.
			if grp.LootMethod == 2 && isOverThreshold && !it.FreeForAll &&
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
				// fire for methods 3/4. Free-for-all rows ignore the
				// round-robin owner (LootView FFA arm, Loot.cpp:743-757).
				if !it.FreeForAll && s.activeLoot.RoundRobinPlayer != 0 && s.activeLoot.RoundRobinPlayer != s.playerGUID {
					return true
				}
			}
		}
	}

	return s.storeTakenLootRow(ctx, s.activeLoot, lootSlot, it, isQuestItem, questIndex)
}

// storeTakenLootRow runs the post-gate take of one loot row
// (Player::StoreLootItem, Player.cpp:25057-25136): the inventory store,
// the loot achievement criteria, the free-for-all / quest / normal
// mark-and-notify arm, the item push result, and the fully-looted close.
func (s *session) storeTakenLootRow(ctx context.Context, loot *activeLootState, lootSlot uint8, it lootItem, isQuestItem bool, questIndex uint8) bool {
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}
	// Loot::LootItemInSlot (Loot.cpp:466-480/491-494): a repeat take on an
	// already-taken free-for-all copy (qitem->is_looted on the quest leg,
	// ffaitem->is_looted on the normal leg, both per-viewer) returns
	// nullptr, so StoreLootItem answers EQUIP_ERR_ALREADY_LOOTED before any
	// inventory store — the check must precede the store, never follow it.
	if it.FreeForAll {
		if isQuestItem {
			if loot.QuestFFATaken[questIndex][s.playerGUID] {
				s.sendEquipError(equipErrAlreadyLooted, 0)
				return true
			}
		} else if loot.FFATaken[lootSlot][s.playerGUID] {
			s.sendEquipError(equipErrAlreadyLooted, 0)
			return true
		}
	}
	res, err := s.storeOrStackItem(ctx, s.playerGUID, it.ItemEntry, it.Count)
	if err != nil {
		if errors.Is(err, errInventoryFull) {
			s.sendEquipError(equipErrInvFull, 0)
		}
		return true
	}
	s.updateAchievementCriteria(criteriaTypeLootItem, it.ItemEntry, it.Count)
	s.updateAchievementCriteria(criteriaTypeLootType, uint32(loot.LootType), it.Count)
	if it.Quality >= 4 {
		s.updateAchievementCriteria(criteriaTypeLootEpicItem, it.ItemEntry, it.Count)
		s.updateAchievementCriteria(criteriaTypeReceiveEpicItem, it.ItemEntry, it.Count)
	}
	// Player::StoreLootItem (Player.cpp:25097-25124): the quest take arm
	// marks the taker's per-viewer qitem copy; a non-free-for-all quest
	// row is gone for everyone (Go deletes the shared row), while a
	// free-for-all quest row survives the take so the other viewers keep
	// their own copies.
	if it.FreeForAll && !isQuestItem {
		// Player::StoreLootItem free-for-all arm (Player.cpp:25107-25113):
		// the take marks only the taker's PlayerFFAItems entry
		// (Loot::LootItemInSlot, Loot.cpp:466-480); the shared row stays
		// for the other viewers and SMSG_LOOT_REMOVED goes to the taker
		// alone. The repeat-take guard runs before the store above.
		if loot.FFATaken == nil {
			loot.FFATaken = make(map[uint8]map[uint64]bool)
		}
		taken := loot.FFATaken[lootSlot]
		if taken == nil {
			taken = make(map[uint64]bool)
			loot.FFATaken[lootSlot] = taken
		}
		taken[s.playerGUID] = true
		notify := protocol.NewBuffer(1)
		notify.WriteU8(lootSlot)
		_ = s.write(uint16(protocol.OpcodeSMSG_LOOT_REMOVED), notify.Bytes(), true)
	} else if isQuestItem {
		if it.FreeForAll {
			// Player::StoreLootItem free-for-all quest arm
			// (Player.cpp:25097-25104): the take marks only the taker's
			// per-viewer qitem copy (qitem->is_looted); the shared quest
			// row stays so the other viewers keep their copies, and the
			// row only leaves the window when everyone has taken it
			// (lootFullyLooted's per-viewer count). The removal goes to
			// the taker alone at the requested slot; a repeat take on an
			// already-taken copy is answered above.
			if loot.QuestFFATaken == nil {
				loot.QuestFFATaken = make(map[uint8]map[uint64]bool)
			}
			taken := loot.QuestFFATaken[questIndex]
			if taken == nil {
				taken = make(map[uint64]bool)
				loot.QuestFFATaken[questIndex] = taken
			}
			taken[s.playerGUID] = true
			notify := protocol.NewBuffer(1)
			notify.WriteU8(lootSlot)
			_ = s.write(uint16(protocol.OpcodeSMSG_LOOT_REMOVED), notify.Bytes(), true)
		} else {
			// The removal broadcast goes out before the delete because
			// each viewer gets the slot from their own quest list.
			loot.broadcastQuestRemoved(ctx, questIndex)
			delete(loot.QuestItems, questIndex)
		}
	} else {
		delete(loot.Items, lootSlot)
		loot.broadcastRemoved(lootSlot)
	}
	_ = s.sendInventoryItems(ctx)

	slotForPush := uint32(res.Slot)
	if res.IsStack {
		slotForPush = 0xFFFFFFFF
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_ITEM_PUSH_RESULT), buildLootItemPushResult(s.playerGUID, res.ClientBag, slotForPush, it.ItemEntry, it.Count, res.InventoryCount), true)
	s.sendPlayerUpdate()
	// Loot::isLooted (Loot.h:236) via lootFullyLooted: a multi-drop row
	// keeps the loot open until every current viewer takes their copy.
	if lootFullyLooted(loot) {
		s.clearCreatureLoot(loot)
	}
	s.debug("loot item stored", "account", s.accountName, "item", it.ItemEntry, "slot", res.Slot, "bag", res.ClientBag, "stacked", res.IsStack)
	return true
}

// autoStoreLootCurrencyTokens mirrors the auto-store leg of
// Loot::FillNotNormalLootFor (Loot.cpp:246-266): right after the loot is
// filled, unlooted free-for-all rows that are currency tokens
// (ItemTemplate::IsCurrencyToken = BagFamily & BAG_FAMILY_MASK_CURRENCY_TOKENS,
// ItemTemplate.h:684) and pass LootItem::AllowedForPlayer for a present
// looter are stored straight into that player's bags via the normal take
// path — the currency never renders in the loot window for them. The C++
// scan covers GetMaxSlotInLootFor (normal rows plus the player's
// quest-item list), so quest rows are scanned too.
// presentAtLooting is true for the opener; grouped members qualify through
// Player::IsAtGroupRewardDistance (Player.cpp:24160): same map and instance,
// then dungeon-always, else within MaxGroupXPDistance
// (CONFIG_GROUP_XP_DISTANCE, default 74) of the looter
// (Go proxies the corpse position with the opener's, as the roll-start code
// does). Deltas: the tokens land through Go's normal inventory store (no
// currency-tab placement model), and members who never open the window get
// their tokens at their own open, not at fill time.
func (s *Server) autoStoreLootCurrencyTokens(ctx context.Context, loot *activeLootState, opener *session) {
	if s == nil || loot == nil || opener == nil || opener.player == nil {
		return
	}
	var present []*session
	present = append(present, opener)
	if opener.groupID != 0 {
		inDungeon := opener.isDungeonMap(opener.player.Map)
		for _, m := range s.getGroupSessions(opener.groupID) {
			if m == nil || m == opener || m.player == nil || m.player.Map != loot.MapID || m.player.InstanceID != loot.InstanceID {
				continue
			}
			if !inDungeon && distance3D(opener.player.X, opener.player.Y, opener.player.Z, m.player.X, m.player.Y, m.player.Z) > s.Config.MaxGroupXPDistance {
				continue
			}
			present = append(present, m)
		}
	}
	for slot, it := range loot.Items {
		if !it.FreeForAll || it.ItemEntry == 0 {
			continue
		}
		data, err := opener.loadItemQueryData(ctx, it.ItemEntry)
		if err != nil || data.BagFamily&itemBagFamilyCurrency == 0 {
			continue
		}
		for _, member := range present {
			if member == nil || member.player == nil {
				continue
			}
			if loot.FFATaken[slot] != nil && loot.FFATaken[slot][member.playerGUID] {
				continue
			}
			if !member.lootItemAllowedForPlayer(ctx, it, false) {
				continue
			}
			member.storeTakenLootRow(ctx, loot, slot, it, false, 0)
		}
	}
	// Loot::FillNotNormalLootFor scans GetMaxSlotInLootFor(player) — the
	// normal rows plus the player's quest-item list — so free-for-all
	// currency-token quest rows auto-store too, each member storing their
	// own per-viewer copy (Player::StoreLootItem, Player.cpp:25097-25104).
	for qidx, it := range loot.QuestItems {
		if !it.FreeForAll || it.ItemEntry == 0 {
			continue
		}
		data, err := opener.loadItemQueryData(ctx, it.ItemEntry)
		if err != nil || data.BagFamily&itemBagFamilyCurrency == 0 {
			continue
		}
		for _, member := range present {
			if member == nil || member.player == nil {
				continue
			}
			if loot.QuestFFATaken[qidx] != nil && loot.QuestFFATaken[qidx][member.playerGUID] {
				continue
			}
			if !member.lootItemAllowedForPlayer(ctx, it, false) {
				continue
			}
			// The taker-only removal needs the member's own display
			// slot (NormalSlotCount + position in their quest list),
			// not the raw quest index.
			questList := member.viewerQuestLootList(ctx, loot)
			displaySlot := loot.NormalSlotCount + qidx
			for pos, idx := range questList {
				if idx == qidx {
					displaySlot = loot.NormalSlotCount + uint8(pos)
					break
				}
			}
			member.storeTakenLootRow(ctx, loot, displaySlot, it, true, qidx)
		}
	}
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
	// clears instead of announcing a looter. lootFullyLooted mirrors
	// Loot::isLooted (Loot.h:236) including the multi-drop rows.
	fullyLooted := lootFullyLooted(loot)
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
	// DoLootRelease (LootHandler.cpp:287): a fishing bobber despawns on
	// release even with the catch still in the window — FISHINGNODE enters
	// the despawn/clear branch regardless of loot->isLooted(). The fishing
	// hole and every other GO wait for Loot::isLooted (Loot.h:236: gold == 0
	// and unlootedCount == 0, counting quest rows and one entry per viewer
	// per free-for-all row), which lootFullyLooted mirrors — the old inline
	// check missed quest-only remainders.
	if goState.Type == GameObjectTypeFishingNode {
		s.server.despawnDynamicGameObjectInInstance(goState.Map, goState.InstanceID, goState.GUID)
		return true
	}
	if !lootFullyLooted(loot) {
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
	// gated by LootItem::AllowedForPlayer(target, isGivenByMasterLooter=true)
	// for EVERY item — normal items included, not just quest items (C++ runs
	// the check on LootItem& item before the error mapping, so a faction or
	// recipe gate on a normal item answers LOOT_ERROR_MASTER_OTHER and the
	// item stays). A failing check maps EQUIP_ERR_YOU_CAN_NEVER_USE_THAT_ITEM
	// to LOOT_ERROR_MASTER_OTHER (14).
	if !targetSess.lootItemAllowedForPlayer(ctx, it, true) {
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
	// then dungeon-always, else within MaxGroupXPDistance
	// (CONFIG_GROUP_XP_DISTANCE, default 74) of the looted object. Go
	// proxies the object position with the looter's — the looter opened the
	// window within ~5yd of the corpse, and sendLootMasterList uses the same
	// proxy for the MasterLoot list.
	inDungeon := looter.isDungeonMap(mapID)
	var eligible []*session
	for _, m := range members {
		if m.player == nil || m.player.Map != mapID || m.player.InstanceID != instanceID {
			continue
		}
		if !inDungeon && looter.player != nil && distance3D(m.player.X, m.player.Y, m.player.Z, looter.player.X, looter.player.Y, looter.player.Z) > s.Config.MaxGroupXPDistance {
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
	var maxCount uint32
	var flagsExtra uint32
	var allowableClass uint32 = 0xFFFFFFFF
	if s.WorldStore != nil && s.WorldStore.DB != nil {
		_ = s.WorldStore.DB.QueryRowContext(context.Background(), "SELECT DisenchantID, RequiredDisenchantSkill, AllowableClass, MaxCount, FlagsExtra FROM item_template WHERE entry = ?", itemEntry).Scan(&disenchantID, &requiredDisenchantSkill, &allowableClass, &maxCount, &flagsExtra)
	}

	baseMask := rollFlagTypePass | rollFlagTypeNeed | rollFlagTypeGreed
	// Group::GroupLoot / NeedBeforeGreed (Group.cpp:1146/1296): the disenchant
	// option is offered only when a group member's enchanting skill covers
	// the item's RequiredDisenchantSkill (m_maxEnchantingLevel, maintained on
	// group join/leave in group.go).
	if disenchantID > 0 && grp != nil && grp.MaxEnchantingLevel >= uint16(requiredDisenchantSkill) {
		baseMask |= rollFlagTypeDisenchant
	}
	// Group::NeedBeforeGreed (Group.cpp:1299): ITEM_FLAG2_CAN_ONLY_ROLL_GREED
	// (ItemTemplate.h:196) clears the NEED bit from the roll vote mask.
	if grp != nil && grp.LootMethod == 4 && flagsExtra&0x100 != 0 {
		baseMask &^= rollFlagTypeNeed
	}

	// Group::GroupLoot (Group.cpp:1133-1136): PassOnGroupLoot and the
	// CanRollOnItem gate record PASS at roll start. Precomputed here so the
	// all-pass arm below can decide before any packet goes out.
	autoPass := make(map[uint64]bool, len(eligible))
	allPass := len(eligible) > 0
	for _, m := range eligible {
		ap := m.player != nil && m.player.PassOnGroupLoot
		if !ap {
			ap = !canRollOnItem(m, itemEntry, itemCount, maxCount)
		}
		autoPass[m.playerGUID] = ap
		if !ap {
			allPass = false
		}
	}

	// Group::GroupLoot (Group.cpp:1155-1159): when every eligible member
	// auto-passes, the roll is deleted without starting — no
	// SMSG_LOOT_START_ROLL, no SMSG_LOOT_ALL_PASSED, no timer. The
	// is_blocked set stands (Group.cpp:1148, ahead of the check), so the
	// item stays blocked until the loot is released, and the auto-passes
	// are broadcast with autoPass=1. NeedBeforeGreed has no such arm: it
	// always starts the roll.
	if grp != nil && grp.LootMethod == 3 && allPass {
		if cLoot := s.creatureLoot[objectKey]; cLoot != nil {
			if li, ok := cLoot.Items[uint8(slot)]; ok {
				li.IsBlocked = true
				cLoot.Items[uint8(slot)] = li
			}
		}
		s.lootMu.Unlock()
		for _, m := range eligible {
			buf := protocol.NewBuffer(35)
			buf.WriteU64(sourceGUID)
			buf.WriteU32(slot)
			buf.WriteU64(m.playerGUID)
			buf.WriteU32(itemEntry)
			buf.WriteU32(0) // randomSuffix
			buf.WriteU32(0) // randomPropId
			buf.WriteU8(128)
			buf.WriteU8(rollPass)
			buf.WriteU8(1) // autoPass
			_ = m.write(uint16(protocol.OpcodeSMSG_LOOT_ROLL), buf.Bytes(), true)
		}
		return
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

		// Auto-pass check: PassOnGroupLoot, plus the CanRollOnItem gate
		// (Group.cpp:1081-1095, applied at Group.cpp:1133-1136 / 1283-1286) —
		// unique-max holders and AllowedForPlayer failures are recorded PASS
		// at roll start. Documented delta: C++ GroupLoot broadcasts these
		// auto-passes with autoPass=1 before the start-roll; Go records them
		// through the vote path (autoPass=0), matching NBG's post-start
		// broadcast order exactly and GroupLoot's packet shape only.
		if autoPass[m.playerGUID] {
			m.handleLootRoll(context.Background(), buildLootRollPayload(sourceGUID, slot, rollPass))
		}
	}

	// Arm 60s countdown
	roll.Timer = time.AfterFunc(60*time.Second, func() {
		// The per-roll timer stands in for Group::EndRoll (Creature.cpp:793),
		// which passes the loot's map as allowedMap.
		s.resolveGroupLootRoll(rollKey, true)
	})
}

// questRollSlot resolves a roll slot into the quest_items index space:
// C++ registers quest-item rolls at itemSlot = items.size() + raw index
// (Group.cpp:1226/1366); Go captures items.size() as NormalSlotCount at
// fill time, so any roll slot at/above it is a quest slot.
func questRollSlot(cLoot *activeLootState, slot uint32) (lootItem, uint8, bool) {
	if cLoot == nil || slot < uint32(cLoot.NormalSlotCount) {
		return lootItem{}, 0, false
	}
	qidx := uint8(slot - uint32(cLoot.NormalSlotCount))
	qi, ok := cLoot.QuestItems[qidx]
	return qi, qidx, ok
}

// startGroupQuestLootRoll mirrors the follow_loot_rules quest-item loops in
// Group::GroupLoot (Group.cpp:1192-1252) and Group::NeedBeforeGreed
// (Group.cpp:1337-1380): quest items roll under group methods regardless of
// threshold, in items.size()+index slot order. Deltas vs startGroupLootRoll:
// the vote default is plain NOT_EMITED_YET (no PassOnGroupLoot default,
// Group.cpp:1211/1353), the vote mask is the constructor default
// ROLL_ALL_TYPE_NO_DISENCHANT (no disenchant arm, no CAN_ONLY_ROLL_GREED
// clear — the quest loops never touch the mask), and a missing item template
// skips the roll under GroupLoot (Group.cpp:1196-1203) but not under NBG
// (no null check in the NBG quest loop).
func (s *Server) startGroupQuestLootRoll(sourceGUID uint64, qslot uint32, qidx uint8, itemEntry, itemCount, mapID, instanceID uint32, groupID uint64, looter *session) {
	if groupID == 0 {
		return
	}
	members := s.getGroupSessions(groupID)
	if len(members) == 0 {
		return
	}
	// Same IsAtGroupRewardDistance proxy as startGroupLootRoll.
	inDungeon := looter.isDungeonMap(mapID)
	var eligible []*session
	for _, m := range members {
		if m.player == nil || m.player.Map != mapID || m.player.InstanceID != instanceID {
			continue
		}
		if !inDungeon && looter.player != nil && distance3D(m.player.X, m.player.Y, m.player.Z, looter.player.X, looter.player.Y, looter.player.Z) > s.Config.MaxGroupXPDistance {
			continue
		}
		eligible = append(eligible, m)
	}
	if len(eligible) == 0 {
		return
	}

	objectKey := lootObjectKey{MapID: mapID, InstanceID: instanceID, GUID: sourceGUID}
	rollKey := lootRollKey{Object: objectKey, Slot: qslot}

	s.groupsMu.Lock()
	grp := s.groups[groupID]
	s.groupsMu.Unlock()

	var allowableClass uint32 = 0xFFFFFFFF
	var maxCount uint32
	var templateFound bool
	if s.WorldStore != nil && s.WorldStore.DB != nil {
		if err := s.WorldStore.DB.QueryRowContext(context.Background(), "SELECT AllowableClass, MaxCount FROM item_template WHERE entry = ?", itemEntry).Scan(&allowableClass, &maxCount); err == nil {
			templateFound = true
		}
	}
	// Group::GroupLoot quest loop (Group.cpp:1196-1203): missing item
	// prototype skips the roll. The NBG quest loop has no such check.
	if grp != nil && grp.LootMethod == 3 && !templateFound {
		return
	}

	// ROLL_ALL_TYPE_NO_DISENCHANT: pass|need|greed. The quest loops never
	// set the disenchant bit (Roll ctor default, Group.cpp:48) and never
	// clear need for CAN_ONLY_ROLL_GREED (no arm at 1192-1252/1337-1380).
	baseMask := rollFlagTypePass | rollFlagTypeNeed | rollFlagTypeGreed

	roll := &activeGroupRoll{
		SourceGUID:          sourceGUID,
		Slot:                qslot,
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
	s.lootMu.Lock()
	if s.groupRolls == nil {
		s.groupRolls = make(map[lootRollKey]*activeGroupRoll)
	}
	if existing := s.groupRolls[rollKey]; existing != nil {
		s.lootMu.Unlock()
		return
	}
	// Group.cpp:1226/1366: the quest item is blocked while the roll runs.
	if cLoot := s.creatureLoot[objectKey]; cLoot != nil {
		if qi, ok := cLoot.QuestItems[qidx]; ok {
			qi.IsBlocked = true
			cLoot.QuestItems[qidx] = qi
		}
	}
	s.groupRolls[rollKey] = roll
	s.lootMu.Unlock()

	// Personalized SMSG_LOOT_START_ROLL per member; NBG keeps the
	// CanRollForItemInLFG personalization (Group.cpp:1360).
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

		buf := buildLootStartRollPacket(sourceGUID, mapID, qslot, itemEntry, 0, 0, itemCount, 60000, memberMask)
		_ = m.write(uint16(protocol.OpcodeSMSG_LOOT_START_ROLL), buf, true)

		// Quest loops default to NOT_EMITED_YET (Group.cpp:1211/1353): no
		// PassOnGroupLoot default — only the CanRollOnItem gate auto-passes.
		if !canRollOnItem(m, itemEntry, itemCount, maxCount) {
			m.handleLootRoll(context.Background(), buildLootRollPayload(sourceGUID, qslot, rollPass))
		}
	}

	// Arm 60s countdown (same m_groupLootTimer/lootingGroupLowGUID stand-in).
	roll.Timer = time.AfterFunc(60*time.Second, func() {
		s.resolveGroupLootRoll(rollKey, true)
	})
}

// canRollOnItem mirrors the file-static CanRollOnItem (Group.cpp:1081-1095):
// players can't roll on a unique item once they already hold the max, and
// LootItem::AllowedForPlayer applies (Go's template-lookup-failure
// keeps-visible delta applies here too, per lootItemAllowedForPlayer). The
// owned-count model is the same character_inventory/item_instance join Go
// already uses for unique checks elsewhere.
func canRollOnItem(m *session, itemEntry, itemCount, maxCount uint32) bool {
	if m == nil || m.player == nil {
		return false
	}
	if maxCount > 0 && m.server != nil && m.server.CharactersStore != nil && m.server.CharactersStore.DB != nil {
		var owned uint32
		_ = m.server.CharactersStore.DB.QueryRowContext(context.Background(), `SELECT COALESCE(SUM(ii.count), 0) FROM character_inventory AS ci
			JOIN item_instance AS ii ON ii.guid = ci.item
			WHERE ci.guid = ? AND ii.itemEntry = ?`, m.playerGUID, itemEntry).Scan(&owned)
		if owned >= maxCount {
			return false
		}
	}
	return m.lootItemAllowedForPlayer(context.Background(), lootItem{ItemEntry: itemEntry, Count: itemCount}, false)
}

// resolveGroupLootRoll mirrors Group::CountTheRoll (Group.cpp:1510-1690).
// enforceMap mirrors the allowedMap parameter: the vote-completion path
// (Group::CountRollVote, Group.cpp:1493) passes nullptr, while the
// corpse-timer path (Group::EndRoll via Creature/GameObject Update,
// Creature.cpp:793, GameObject.cpp:721) passes the loot's map, excluding
// voters who left it.
func (s *Server) resolveGroupLootRoll(rollKey lootRollKey, enforceMap bool) {
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

	// Group.cpp:1527-1531, 1589-1593: voters whose player object is gone
	// (and, on the EndRoll path, who left the loot's map) are skipped from
	// the tier — they roll no number and cannot win, so an emptied tier
	// falls through to greed, then to all-passed.
	present := func(guid uint64) bool {
		sess := s.findSessionByGUID(guid)
		if sess == nil || sess.player == nil {
			return false
		}
		if enforceMap && (sess.player.Map != roll.MapID || sess.player.InstanceID != roll.InstanceID) {
			return false
		}
		return true
	}

	// Roll::PlayerVote is std::map<ObjectGuid, RollVote> (Group.h:141), so
	// C++ iterates GUID-ascending and the strict maxresul < randomN keeps
	// the lowest GUID on ties; Go map order is random, so sort first.
	var needVoters, greedVoters []uint64
	for guid, vote := range roll.Votes {
		if !present(guid) {
			continue
		}
		switch vote {
		case rollNeed:
			needVoters = append(needVoters, guid)
		case rollGreed, rollDisenchant:
			greedVoters = append(greedVoters, guid)
		}
	}
	sort.Slice(needVoters, func(i, j int) bool { return needVoters[i] < needVoters[j] })
	sort.Slice(greedVoters, func(i, j int) bool { return greedVoters[i] < greedVoters[j] })

	var winnerGUID uint64
	var maxRoll uint8
	var winningType uint8

	// Note the if/if, not if/else-if (Group.cpp:1577): a need tier emptied
	// by the presence skip above falls into the greed tier.
	if len(needVoters) > 0 {
		for _, guid := range needVoters {
			rNum := uint8(rand.Intn(100) + 1)
			roll.Rolls[guid] = rNum
			rBuf := protocol.NewBuffer(35)
			rBuf.WriteU64(0) // Group.cpp:1542 — ObjectGuid::Empty, not the item guid
			rBuf.WriteU32(roll.Slot)
			rBuf.WriteU64(guid)
			rBuf.WriteU32(roll.ItemEntry)
			rBuf.WriteU32(roll.RandomSuffix)
			rBuf.WriteU32(roll.RandomPropID)
			rBuf.WriteU8(rNum)
			rBuf.WriteU8(rollNeed)
			rBuf.WriteU8(0)
			s.broadcastToGroup(roll.GroupID, uint16(protocol.OpcodeSMSG_LOOT_ROLL), rBuf.Bytes())
			if rNum > maxRoll {
				maxRoll = rNum
				winnerGUID = guid
				winningType = rollNeed
			}
		}
	}
	if winnerGUID == 0 && len(greedVoters) > 0 {
		for _, guid := range greedVoters {
			vote := roll.Votes[guid]
			rNum := uint8(rand.Intn(100) + 1)
			roll.Rolls[guid] = rNum
			rBuf := protocol.NewBuffer(35)
			rBuf.WriteU64(0) // Group.cpp:1605 — ObjectGuid::Empty, not the item guid
			rBuf.WriteU32(roll.Slot)
			rBuf.WriteU64(guid)
			rBuf.WriteU32(roll.ItemEntry)
			rBuf.WriteU32(roll.RandomSuffix)
			rBuf.WriteU32(roll.RandomPropID)
			rBuf.WriteU8(rNum)
			rBuf.WriteU8(vote)
			rBuf.WriteU8(0)
			s.broadcastToGroup(roll.GroupID, uint16(protocol.OpcodeSMSG_LOOT_ROLL), rBuf.Bytes())
			if rNum > maxRoll {
				maxRoll = rNum
				winnerGUID = guid
				winningType = vote
			}
		}
	}

	if winnerGUID != 0 {
		wonBuf := protocol.NewBuffer(34)
		wonBuf.WriteU64(0) // Group.cpp:1552,1616 — ObjectGuid::Empty, not the item guid
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
			if winningType == rollNeed {
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

		// Group.cpp:1683-1688: remove is_blocked so the item is lootable
		// by all players.
		s.lootMu.Lock()
		if cLoot := s.creatureLoot[rollLootObjectKey(roll)]; cLoot != nil {
			if qi, qidx, ok := questRollSlot(cLoot, roll.Slot); ok {
				qi.IsBlocked = false
				cLoot.QuestItems[qidx] = qi
			} else if li, ok := cLoot.Items[uint8(roll.Slot)]; ok {
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
			// Group.cpp:1567-1570: full bags unblock the item and record the
			// winner for a later take. Quest-slot rolls live in QuestItems.
			if qi, qidx, ok := questRollSlot(cLoot, roll.Slot); ok {
				qi.IsBlocked = false
				qi.RollWinner = winnerGUID
				cLoot.QuestItems[qidx] = qi
			} else if li, ok := cLoot.Items[uint8(roll.Slot)]; ok {
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
			if qi, qidx, ok := questRollSlot(cLoot, roll.Slot); ok {
				qi.IsBlocked = false
				qi.RollWinner = winnerGUID
				cLoot.QuestItems[qidx] = qi
			} else if li, ok := cLoot.Items[uint8(roll.Slot)]; ok {
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
		// Group.cpp:1561-1566: the delivered item is marked looted and
		// removed from its slot — quest-slot wins leave QuestItems.
		if _, qidx, ok := questRollSlot(cLoot, roll.Slot); ok {
			delete(cLoot.QuestItems, qidx)
		} else {
			delete(cLoot.Items, uint8(roll.Slot))
		}
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
					// Loot::AddItem (Loot.cpp:143-149): no minimum-1 clamp —
					// a zero count roll yields no row at all.
					if count == 0 {
						continue
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
			if qi, qidx, ok := questRollSlot(cLoot, roll.Slot); ok {
				qi.IsBlocked = false
				qi.RollWinner = winnerGUID
				cLoot.QuestItems[qidx] = qi
			} else if li, ok := cLoot.Items[uint8(roll.Slot)]; ok {
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
		if _, qidx, ok := questRollSlot(cLoot, roll.Slot); ok {
			delete(cLoot.QuestItems, qidx)
		} else {
			delete(cLoot.Items, uint8(roll.Slot))
		}
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

	// Group::NeedBeforeGreed (Group.cpp:1299): ITEM_FLAG2_CAN_ONLY_ROLL_GREED
	// clears NEED from the roll vote mask, so a NEED vote on such a roll
	// drops to pass (the client UI can't offer need, and the server follows
	// the mask). Kept next to the class-eligibility downgrade above.
	if rollType == rollNeed && roll.RollVoteMask&rollFlagTypeNeed == 0 {
		rollType = rollPass
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
	buf.WriteU64(0) // Group.cpp:1471-1486 — CountRollVote broadcasts ObjectGuid::Empty
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
		// Group::CountRollVote passes nullptr for allowedMap (Group.cpp:1493).
		s.server.resolveGroupLootRoll(rollKey, false)
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
		// Mirrors the Group::Update member-removal path: the leaver's vote is
		// gone and CountRollVote re-checks totals with allowedMap=nullptr
		// (Group.cpp:699, 1493).
		s.resolveGroupLootRoll(key, false)
	}
}
