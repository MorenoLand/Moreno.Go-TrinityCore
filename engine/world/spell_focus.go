package world

import (
	"context"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

func (s *session) spellFocusFound(ctx context.Context, spell wotlk.Spell) bool {
	if spell.RequiresSpellFocus == 0 {
		return true
	}
	player := s.player
	visibility := float64(s.server.Config.VisibilityDistanceContinents)
	if visibility <= 0 {
		visibility = 90
	}
	for _, dyn := range s.server.gameObjectStatesInInstance(player.Map, player.InstanceID) {
		if dyn.Hidden || dyn.Type != GameObjectTypeSpellFocus {
			continue
		}
		focusID, dist, ok := s.spellFocusTemplateData(ctx, dyn.Entry)
		if !ok || focusID != spell.RequiresSpellFocus {
			continue
		}
		if distance3D(player.X, player.Y, player.Z, dyn.X, dyn.Y, dyn.Z) <= float64(dist) {
			return true
		}
	}
	if s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return false
	}
	goArgs := make([]any, 0, 4)
	eventClause := gameEventSpawnClause("geg.eventEntry", s.server.activeEventList(ctx), &goArgs)
	query := `SELECT g.guid, g.id, g.position_x, g.position_y, g.position_z, COALESCE(t.data0, 0), COALESCE(t.data1, 0)
		FROM gameobject AS g
		JOIN gameobject_template AS t ON t.entry = g.id
		LEFT JOIN game_event_gameobject AS geg ON geg.guid = g.guid
		WHERE g.map = ? AND t.type = 8
		AND g.position_x BETWEEN ? AND ? AND g.position_y BETWEEN ? AND ?
		AND (g.spawnMask = 0 OR (g.spawnMask & 1) <> 0)
		AND ` + eventClause
	args := append([]any{player.Map, float64(player.X) - visibility, float64(player.X) + visibility, float64(player.Y) - visibility, float64(player.Y) + visibility}, goArgs...)
	rows, err := s.server.WorldStore.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var guid, entry int64
		var x, y, z, focusID, dist float64
		if err := rows.Scan(&guid, &entry, &x, &y, &z, &focusID, &dist); err != nil {
			continue
		}
		if uint32(focusID) != spell.RequiresSpellFocus {
			continue
		}
		if s.server.isGameObjectHiddenInInstance(player.Map, player.InstanceID, gameObjectGUID(uint32(guid), uint32(entry))) {
			continue
		}
		if distance3D(player.X, player.Y, player.Z, float32(x), float32(y), float32(z)) <= dist {
			return true
		}
	}
	return false
}

func (s *session) spellFocusTemplateData(ctx context.Context, entry uint32) (uint32, float32, bool) {
	if s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return 0, 0, false
	}
	var focusID, dist float64
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(data0, 0), COALESCE(data1, 0) FROM gameobject_template WHERE entry = ? AND type = 8", entry).Scan(&focusID, &dist); err != nil {
		return 0, 0, false
	}
	return uint32(focusID), float32(dist), true
}
