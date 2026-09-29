package world

import (
	"context"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

// spellFocusObject returns the first spell-focus gameobject matching the
// spell's RequiresSpellFocus within range, mirroring Spell::SearchSpellFocus
// (Spell.cpp:1987) + GameObjectFocusCheck (GridNotifiers.h:663): runtime
// dynamic spawns first, then static rows, template data0 = focus id and
// data1 = range.
func (s *session) spellFocusObject(ctx context.Context, spell wotlk.Spell) (guid uint64, x, y, z float32, ok bool) {
	if s == nil || s.player == nil || s.server == nil || spell.RequiresSpellFocus == 0 {
		return 0, 0, 0, 0, false
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
			return dyn.GUID, dyn.X, dyn.Y, dyn.Z, true
		}
	}
	if s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return 0, 0, 0, 0, false
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
		return 0, 0, 0, 0, false
	}
	defer rows.Close()
	for rows.Next() {
		var low, entry int64
		var x, y, z, focusID, dist float64
		if err := rows.Scan(&low, &entry, &x, &y, &z, &focusID, &dist); err != nil {
			continue
		}
		if uint32(focusID) != spell.RequiresSpellFocus {
			continue
		}
		rawGUID := gameObjectGUID(uint32(low), uint32(entry))
		if s.server.isGameObjectHiddenInInstance(player.Map, player.InstanceID, rawGUID) {
			continue
		}
		if distance3D(player.X, player.Y, player.Z, float32(x), float32(y), float32(z)) <= dist {
			return rawGUID, float32(x), float32(y), float32(z), true
		}
	}
	return 0, 0, 0, 0, false
}

func (s *session) spellFocusFound(ctx context.Context, spell wotlk.Spell) bool {
	if spell.RequiresSpellFocus == 0 {
		return true
	}
	_, _, _, _, ok := s.spellFocusObject(ctx, spell)
	return ok
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
