package world

import (
	"context"
	"strconv"
	"strings"
)

func (s *session) setBattlegroundEntryPoint() {
	if s == nil || s.server == nil || s.player == nil {
		return
	}
	state := s.player
	setPosition := func(mapID uint32, x, y, z, orientation float32) {
		s.bgData.JoinMap, s.bgData.JoinX, s.bgData.JoinY, s.bgData.JoinZ, s.bgData.JoinO = mapID, x, y, z, orientation
	}
	s.bgData.TaxiStart, s.bgData.TaxiEnd = 0, 0
	if state.TaxiPath != "" {
		path := strings.Fields(state.TaxiPath)
		if len(path) >= 3 {
			if start, err := strconv.ParseUint(path[1], 10, 32); err == nil {
				s.bgData.TaxiStart = uint32(start)
			}
			if end, err := strconv.ParseUint(path[2], 10, 32); err == nil {
				s.bgData.TaxiEnd = uint32(end)
			}
		}
		s.bgData.MountSpell = 0
		setPosition(state.Map, state.X, state.Y, state.Z, state.Orientation)
		return
	}
	s.bgData.MountSpell = s.currentMountedAuraSpell()
	if _, _, _, inBattlefield := battlegroundTypeForMap(state.Map); inBattlefield {
		return
	}
	if s.server.Data != nil {
		if mapInfo, found, err := s.server.Data.Map(state.Map); err == nil && found {
			if mapInfo.IsDungeon() {
				if location, found := s.server.closestGraveyard(context.Background(), state.X, state.Y, state.Z, state.Map, state.Zone, teamForRace(state.Race)); found {
					setPosition(location.MapID, location.X, location.Y, location.Z, 0)
					return
				}
				s.setBattlegroundHomebind(context.Background())
				return
			}
			setPosition(state.Map, state.X, state.Y, state.Z, state.Orientation)
			return
		}
	}
	s.setBattlegroundHomebind(context.Background())
}

func (s *session) setBattlegroundHomebind(ctx context.Context) {
	if s == nil || s.server == nil || s.player == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	var mapID uint32
	var x, y, z, orientation float32
	if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT mapId, posX, posY, posZ, orientation FROM character_homebind WHERE guid = ?", s.playerGUID).Scan(&mapID, &x, &y, &z, &orientation); err == nil {
		s.bgData.JoinMap, s.bgData.JoinX, s.bgData.JoinY, s.bgData.JoinZ, s.bgData.JoinO = mapID, x, y, z, orientation
	}
}

func (s *session) currentMountedAuraSpell() uint32 {
	if s == nil || s.server == nil || s.server.Data == nil {
		return 0
	}
	for _, aura := range s.loadedAuras() {
		if aura == nil || aura.Stopped {
			continue
		}
		spell, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		for index, effect := range spell.Effects {
			if effect.Aura == spellAuraMounted && aura.EffectMask&(1<<uint(index)) != 0 {
				return aura.SpellID
			}
		}
	}
	return 0
}
