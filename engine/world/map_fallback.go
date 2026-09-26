package world

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

type mapFallbackLocation struct {
	Map  uint32
	Zone uint32
	X    float32
	Y    float32
	Z    float32
}

func validTrinityMapCoordinates(x, y, z, orientation float32) bool {
	const maxMapCoordinate = 17066.166015625
	return !math.IsNaN(float64(x)) && !math.IsInf(float64(x), 0) && math.Abs(float64(x)) <= maxMapCoordinate &&
		!math.IsNaN(float64(y)) && !math.IsInf(float64(y), 0) && math.Abs(float64(y)) <= maxMapCoordinate &&
		!math.IsNaN(float64(z)) && !math.IsInf(float64(z), 0) && math.Abs(float64(z)) <= maxMapCoordinate &&
		!math.IsNaN(float64(orientation)) && !math.IsInf(float64(orientation), 0)
}

func (s *session) validTrinityMapID(mapID uint32) bool {
	if s == nil || s.server == nil || s.server.Data == nil {
		return false
	}
	_, found, err := s.server.Data.Map(mapID)
	return err == nil && found
}

func (s *session) validTrinityMapLocation(mapID uint32, x, y, z, orientation float32) bool {
	return validTrinityMapCoordinates(x, y, z, orientation) && s.validTrinityMapID(mapID)
}

func (s *session) recoverLoginMapCreationFailure(ctx context.Context, state *playerState) error {
	if s == nil || state == nil || s.server == nil || s.server.Data == nil {
		return nil
	}
	mapEntry, found, err := s.server.Data.Map(state.Map)
	if err != nil {
		return err
	}
	if !found {
		return s.recoverLoginHomebindOrStart(ctx, state)
	}
	if !mapEntry.IsDungeon() || state.InstanceID == 0 {
		return nil
	}
	if s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return errors.New("login instance fallback requires a character database")
	}
	bound, err := s.hasCreatableLoginInstance(ctx, *state, mapEntry)
	if err != nil {
		return err
	}
	if bound {
		return nil
	}
	active := s.hasActiveWorldportInstance(state.Map, state.InstanceID)
	var saved int64
	saveErr := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT 1 FROM instance WHERE id = ? AND map = ?", state.InstanceID, state.Map).Scan(&saved)
	if saveErr != nil && !errors.Is(saveErr, sql.ErrNoRows) && !missingTable(saveErr) {
		return saveErr
	}
	if active && saveErr == nil {
		return nil
	}
	if active {
		if location, ok := s.mapEntranceTriggerLocation(ctx, state.Map); ok {
			state.Zone, state.X, state.Y, state.Z = location.Zone, location.X, location.Y, location.Z
			s.debug("saved instance save unavailable; moved to map entrance trigger", "guid", state.GUID, "map", mapEntry.ID, "instance", state.InstanceID)
			return nil
		}
	}
	return s.recoverLoginInstanceMapFailure(ctx, state)
}

func (s *session) recoverLoginInstanceMapFailure(ctx context.Context, state *playerState) error {
	if s == nil || state == nil {
		return nil
	}
	mapID := state.Map
	if location, ok := s.goBackTriggerLocation(ctx, mapID); ok {
		state.Map, state.Zone, state.X, state.Y, state.Z = location.Map, location.Zone, location.X, location.Y, location.Z
		state.InstanceID = 0
		state.TransportGUID, state.TransportX, state.TransportY, state.TransportZ, state.TransportO = 0, 0, 0, 0, 0
		s.debug("instance map unavailable; moved to go-back trigger", "guid", state.GUID, "map", mapID, "destination", location.Map)
		return nil
	}
	return s.recoverLoginHomebindOrStart(ctx, state)
}

func (s *session) recoverLoginHomebindOrStart(ctx context.Context, state *playerState) error {
	homebind := mapFallbackLocation{Map: state.HomebindMap, X: state.HomebindX, Y: state.HomebindY, Z: state.HomebindZ}
	if s.validMapFallbackLocation(homebind) {
		state.Map, state.Zone, state.X, state.Y, state.Z = homebind.Map, 0, homebind.X, homebind.Y, homebind.Z
		state.InstanceID = 0
		state.TransportGUID, state.TransportX, state.TransportY, state.TransportZ, state.TransportO = 0, 0, 0, 0, 0
		return nil
	}
	location, err := s.raceClassStartLocation(ctx, *state)
	if err != nil {
		return err
	}
	if !s.validMapFallbackLocation(location) {
		return fmt.Errorf("race/class start location is invalid for character %d", state.GUID)
	}
	state.Map, state.Zone, state.X, state.Y, state.Z, state.Orientation = location.Map, location.Zone, location.X, location.Y, location.Z, 0
	state.InstanceID = 0
	state.TransportGUID, state.TransportX, state.TransportY, state.TransportZ, state.TransportO = 0, 0, 0, 0, 0
	return nil
}

func (s *session) hasCreatableLoginInstance(ctx context.Context, state playerState, mapEntry wotlk.MapEntry) (bool, error) {
	if s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return false, nil
	}
	difficulty := uint8(state.DungeonDifficulty)
	if mapEntry.IsRaid() {
		difficulty = state.RaidDifficulty
	}
	var found int64
	err := s.server.CharactersStore.DB.QueryRowContext(ctx, `SELECT 1 FROM character_instance AS ci JOIN instance AS i ON i.id = ci.instance
		WHERE ci.guid = ? AND i.map = ? AND i.difficulty = ? AND (ci.permanent <> 0 OR ci.instance = ?) LIMIT 1`, state.GUID, state.Map, difficulty, state.InstanceID).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) || missingTable(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return found != 0, nil
}

func (s *session) goBackTriggerLocation(ctx context.Context, mapID uint32) (mapFallbackLocation, bool) {
	if s == nil || s.server == nil || s.server.Data == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return mapFallbackLocation{}, false
	}
	mapEntry, found, err := s.server.Data.Map(mapID)
	if err != nil || !found || mapEntry.CorpseMapID < 0 {
		return mapFallbackLocation{}, false
	}
	targetMap := uint32(mapEntry.CorpseMapID)
	if mapEntry.IsDungeon() {
		var parent int64
		if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT parent FROM instance_template WHERE map = ?", mapID).Scan(&parent); err != nil || parent < 0 {
			return mapFallbackLocation{}, false
		}
		targetMap = uint32(parent)
	}
	rows, err := s.server.WorldStore.DB.QueryContext(ctx, `SELECT id, target_map, target_position_x, target_position_y, target_position_z
		FROM areatrigger_teleport WHERE target_map = ?`, targetMap)
	if err != nil {
		return mapFallbackLocation{}, false
	}
	defer rows.Close()
	for rows.Next() {
		var id, destinationMap int64
		var x, y, z float64
		if rows.Scan(&id, &destinationMap, &x, &y, &z) != nil || id <= 0 || id > int64(^uint32(0)) || destinationMap < 0 || destinationMap > int64(^uint32(0)) || (x == 0 && y == 0 && z == 0) {
			continue
		}
		trigger, found, err := s.server.Data.AreaTrigger(uint32(id))
		location := mapFallbackLocation{Map: uint32(destinationMap), X: float32(x), Y: float32(y), Z: float32(z)}
		if err == nil && found && trigger.ContinentID == mapID && s.validMapFallbackLocation(location) {
			return location, true
		}
	}
	return mapFallbackLocation{}, false
}

func (s *session) mapEntranceTriggerLocation(ctx context.Context, mapID uint32) (mapFallbackLocation, bool) {
	if s == nil || s.server == nil || s.server.Data == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return mapFallbackLocation{}, false
	}
	rows, err := s.server.WorldStore.DB.QueryContext(ctx, `SELECT id, target_position_x, target_position_y, target_position_z
		FROM areatrigger_teleport WHERE target_map = ?`, mapID)
	if err != nil {
		return mapFallbackLocation{}, false
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var x, y, z float64
		if rows.Scan(&id, &x, &y, &z) != nil || id <= 0 || id > int64(^uint32(0)) || (x == 0 && y == 0 && z == 0) {
			continue
		}
		if _, found, err := s.server.Data.AreaTrigger(uint32(id)); err != nil || !found {
			continue
		}
		location := mapFallbackLocation{Map: mapID, X: float32(x), Y: float32(y), Z: float32(z)}
		if s.validMapFallbackLocation(location) {
			return location, true
		}
	}
	return mapFallbackLocation{}, false
}

func (s *session) raceClassStartLocation(ctx context.Context, state playerState) (mapFallbackLocation, error) {
	if s == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return mapFallbackLocation{}, errors.New("race/class start location database is unavailable")
	}
	var location mapFallbackLocation
	err := s.server.WorldStore.DB.QueryRowContext(ctx, `SELECT map, zone, position_x, position_y, position_z
		FROM playercreateinfo WHERE race = ? AND class = ?`, state.Race, state.Class).Scan(&location.Map, &location.Zone, &location.X, &location.Y, &location.Z)
	if err != nil {
		return mapFallbackLocation{}, err
	}
	return location, nil
}

func (s *session) validMapFallbackLocation(location mapFallbackLocation) bool {
	return s.validTrinityMapLocation(location.Map, location.X, location.Y, location.Z, 0)
}
