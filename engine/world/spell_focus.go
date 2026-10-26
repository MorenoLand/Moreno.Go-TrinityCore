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
	if s == nil || s.server == nil {
		return 0, 0, false
	}
	return s.server.spellFocusTemplateDataAt(ctx, entry)
}

// spellFocusTemplateDataAt is the server-level focus-template lookup:
// template data0 = focus id, data1 = range (GameObjectFocusCheck,
// GridNotifiers.h:663).
func (s *Server) spellFocusTemplateDataAt(ctx context.Context, entry uint32) (uint32, float32, bool) {
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil {
		return 0, 0, false
	}
	var focusID, dist float64
	if err := s.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(data0, 0), COALESCE(data1, 0) FROM gameobject_template WHERE entry = ? AND type = 8", entry).Scan(&focusID, &dist); err != nil {
		return 0, 0, false
	}
	return uint32(focusID), float32(dist), true
}

// motionSpellFocusObject mirrors Spell::SearchSpellFocus (Spell.cpp:1987)
// anchored at a creature motion's position, returning the focus object's
// GUID and position: the TARGET_GAMEOBJECT_NEARBY_ENTRY emergency fallback
// (Spell.cpp:1065-1071) for a creature caster with no implicit-target
// conditions, and the GUID form behind motionSpellFocusPosition below.
// A focus object counts only when it is a spawned spell-focus GO whose
// template focus id matches the spell's RequiresSpellFocus and the caster
// is within the template range (GameObjectFocusCheck,
// GridNotifiers.h:663). Runtime dynamic spawns come first, then static
// rows, mirroring the player path's spellFocusObject.
// isSpawned() == !Hidden for the Go runtime spawns. Runs for triggered
// casts too (no C++ triggered exemption).
func (s *Server) motionSpellFocusObject(ctx context.Context, m *creatureMotion, spell wotlk.Spell) (uint64, float32, float32, float32, bool) {
	if s == nil || m == nil || spell.RequiresSpellFocus == 0 {
		return 0, 0, 0, 0, false
	}
	visibility := float64(s.Config.VisibilityDistanceContinents)
	if visibility <= 0 {
		visibility = 90
	}
	for _, dyn := range s.gameObjectStatesInInstance(m.Map, m.InstanceID) {
		if dyn.Hidden || dyn.Type != GameObjectTypeSpellFocus {
			continue
		}
		focusID, dist, ok := s.spellFocusTemplateDataAt(ctx, dyn.Entry)
		if !ok || focusID != spell.RequiresSpellFocus {
			continue
		}
		if distance3D(m.X, m.Y, m.Z, dyn.X, dyn.Y, dyn.Z) <= float64(dist) {
			return dyn.GUID, dyn.X, dyn.Y, dyn.Z, true
		}
	}
	if s.WorldStore == nil || s.WorldStore.DB == nil {
		return 0, 0, 0, 0, false
	}
	goArgs := make([]any, 0, 4)
	eventClause := gameEventSpawnClause("geg.eventEntry", s.activeEventList(ctx), &goArgs)
	query := `SELECT g.guid, g.id, g.position_x, g.position_y, g.position_z, COALESCE(t.data0, 0), COALESCE(t.data1, 0)
		FROM gameobject AS g
		JOIN gameobject_template AS t ON t.entry = g.id
		LEFT JOIN game_event_gameobject AS geg ON geg.guid = g.guid
		WHERE g.map = ? AND t.type = 8
		AND g.position_x BETWEEN ? AND ? AND g.position_y BETWEEN ? AND ?
		AND (g.spawnMask = 0 OR (g.spawnMask & 1) <> 0)
		AND ` + eventClause
	args := append([]any{m.Map, float64(m.X) - visibility, float64(m.X) + visibility, float64(m.Y) - visibility, float64(m.Y) + visibility}, goArgs...)
	rows, err := s.WorldStore.DB.QueryContext(ctx, query, args...)
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
		if s.isGameObjectHiddenInInstance(m.Map, m.InstanceID, rawGUID) {
			continue
		}
		if distance3D(m.X, m.Y, m.Z, float32(x), float32(y), float32(z)) <= dist {
			return rawGUID, float32(x), float32(y), float32(z), true
		}
	}
	return 0, 0, 0, 0, false
}

// motionSpellFocusPosition is the position form of
// motionSpellFocusObject, kept for the TARGET_DEST_NEARBY_ENTRY
// emergency fallback (Spell.cpp:1085) and motionSpellFocusFound below.
func (s *Server) motionSpellFocusPosition(ctx context.Context, m *creatureMotion, spell wotlk.Spell) (float32, float32, float32, bool) {
	_, x, y, z, ok := s.motionSpellFocusObject(ctx, m, spell)
	return x, y, z, ok
}

// motionSpellFocusFound is the pet CheckCast spell-focus arm
// (Spell.cpp:5477-5481) form of motionSpellFocusPosition: a spell with no
// focus requirement trivially counts as found.
func (s *Server) motionSpellFocusFound(ctx context.Context, m *creatureMotion, spell wotlk.Spell) bool {
	if s == nil || m == nil || spell.RequiresSpellFocus == 0 {
		return true
	}
	_, _, _, ok := s.motionSpellFocusPosition(ctx, m, spell)
	return ok
}
