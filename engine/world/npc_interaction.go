package world

import (
	"context"
	"database/sql"
	"errors"
)

const unitNPCFlagVendor uint32 = 0x00000080

// unitNPCFlagRepair mirrors UNIT_NPC_FLAG_REPAIR (UnitDefines.h:198).
const unitNPCFlagRepair uint32 = 0x00001000

// unitNPCFlagInnkeeper mirrors UNIT_NPC_FLAG_INNKEEPER (UnitDefines.h:202).
const unitNPCFlagInnkeeper uint32 = 0x00010000

// unitNPCFlagBanker mirrors UNIT_NPC_FLAG_BANKER (UnitDefines.h:203).
const unitNPCFlagBanker uint32 = 0x00020000

// unitNPCFlagTabardDesigner mirrors UNIT_NPC_FLAG_TABARDDESIGNER (UnitDefines.h:205).
const unitNPCFlagTabardDesigner uint32 = 0x00080000

// unitNPCFlagBattlemaster mirrors UNIT_NPC_FLAG_BATTLEMASTER (UnitDefines.h:206).
const unitNPCFlagBattlemaster uint32 = 0x00100000

const gameObjectTypeGuildBank uint32 = 34

// canInteractWithGameObject mirrors Player::GetGameObjectIfCanInteractWith
// (Player.cpp:2363-2398): the gameobject must exist, carry the required
// GO type, not use the "Point" icon, and be within interact distance on the
// player's map. GAMEOBJECT_TYPE_GUILD_BANK is SharedDefines.h:1628.
func (s *session) canInteractWithGameObject(ctx context.Context, guid uint64, goType uint32) bool {
	if s == nil || s.player == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil || uint16(guid>>48) != 0xF110 {
		return false
	}
	low := uint32(guid & 0x00FFFFFF)
	var mapID int64
	var x, y, z float32
	var iconName string
	err := s.server.WorldStore.DB.QueryRowContext(ctx, `SELECT g.map, g.position_x, g.position_y, g.position_z, COALESCE(t.IconName, '')
		FROM gameobject AS g JOIN gameobject_template AS t ON t.entry = g.id WHERE g.guid = ? AND t.type = ?`, low, goType).Scan(&mapID, &x, &y, &z, &iconName)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false
		}
		if missingTable(err) || isMissingColumn(err) {
			return true
		}
		return false
	}
	if iconName == "Point" {
		return false
	}
	if uint32(mapID) != s.player.Map || distance3D(s.player.X, s.player.Y, s.player.Z, x, y, z) > 5.0 {
		return false
	}
	return true
}

func (s *session) canInteractWithNPC(ctx context.Context, guid, requiredFlags uint64) bool {
	if s == nil || s.player == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil || uint16(guid>>48) != 0xF130 {
		return false
	}
	// Player::GetNPCIfCanInteractWith (Player.cpp:2314): no NPC interaction
	// while the player is in flight.
	if s.isInFlight() {
		return false
	}
	low := uint32(guid & 0x00FFFFFF)
	entry := uint32((guid >> 24) & 0x00FFFFFF)
	var mapID, npcFlags int64
	var x, y, z float32
	err := s.server.WorldStore.DB.QueryRowContext(ctx, `SELECT c.map, c.position_x, c.position_y, c.position_z, t.npcflag
		FROM creature AS c JOIN creature_template AS t ON t.entry = c.id WHERE c.guid = ? AND c.id = ?`, low, entry).Scan(&mapID, &x, &y, &z, &npcFlags)
	if err != nil {
		if missingTable(err) || isMissingColumn(err) {
			return true
		}
		return false
	}
	if uint32(mapID) != s.player.Map || distance3D(s.player.X, s.player.Y, s.player.Z, x, y, z) > 5.0 {
		return false
	}
	return requiredFlags == 0 || uint32(npcFlags)&uint32(requiredFlags) != 0
}
