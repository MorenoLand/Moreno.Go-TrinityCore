package world

import (
	"context"
	"math/rand"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

func playerRestingInZoneArea(team uint8, zoneFlags, areaFlags uint32, hostile, sanctuary bool) bool {
	restAreaFlag := wotlk.AreaFlagRestZoneAlliance
	if team != 0 {
		restAreaFlag = wotlk.AreaFlagRestZoneHorde
	}
	return areaFlags&restAreaFlag != 0 || zoneFlags&wotlk.AreaFlagCapital != 0 && (!hostile || sanctuary)
}

func (s *session) setRestingFlag(state *playerState, resting bool) {
	if state == nil {
		return
	}
	wasResting := state.PlayerFlags&playerFlagResting != 0
	if resting {
		state.PlayerFlags |= playerFlagResting
	} else {
		state.PlayerFlags &^= playerFlagResting
	}
	if s == nil {
		return
	}
	if !wasResting && resting {
		s.lastRestBonusUpdate = time.Unix(time.Now().Unix(), 0)
	} else if wasResting && !resting {
		s.lastRestBonusUpdate = time.Time{}
	}
}

func (s *session) inTavernResting() bool {
	if s == nil || s.innTriggerID == 0 {
		return false
	}
	if s.player == nil || s.server == nil || s.server.Data == nil {
		s.innTriggerID = 0
		return false
	}
	trigger, found, err := s.server.Data.AreaTrigger(s.innTriggerID)
	if err == nil && found && trigger.IsInAreaTriggerRadius(s.player.Map, s.player.X, s.player.Y, s.player.Z) {
		return true
	}
	s.innTriggerID = 0
	return false
}

func (s *Server) updateRestedBonuses(ctx context.Context, now time.Time) {
	if s == nil {
		return
	}
	s.sessionsMu.RLock()
	sessions := make([]*session, 0, len(s.sessions))
	for sess := range s.sessions {
		if sess.worldReady.Load() && sess.player != nil {
			sessions = append(sessions, sess)
		}
	}
	s.sessionsMu.RUnlock()
	for _, sess := range sessions {
		if sess.innTriggerID != 0 && !sess.inTavernResting() {
			sess.updateZoneAndArea(ctx, true)
			if sess.applyZoneState(ctx, sess.player, sess.player.Zone, sess.areaID) {
				sess.sendPlayerUpdate()
			}
		}
		if sess.player.PlayerFlags&playerFlagResting == 0 {
			sess.lastRestBonusUpdate = time.Time{}
			continue
		}
		if sess.lastRestBonusUpdate.IsZero() {
			sess.lastRestBonusUpdate = now
			continue
		}
		elapsed := now.Unix() - sess.lastRestBonusUpdate.Unix()
		if elapsed < 10 || rand.Intn(100) >= 3 {
			continue
		}
		sess.lastRestBonusUpdate = time.Unix(now.Unix(), 0)
		player := sess.player
		oldBonus, oldRestState := uint32(player.RestBonus), player.RestState
		bonus := player.RestBonus + float32(elapsed)*(float32(playerNextLevelXP(player.Level))/72000)*(0.125*float32(s.Config.RestInGameRate))
		sess.setRestBonus(player, bonus)
		fields := make(map[int]uint32, 2)
		if oldBonus != uint32(player.RestBonus) {
			fields[playerFieldRestStateExperience] = uint32(player.RestBonus)
		}
		if oldRestState != player.RestState {
			fields[unitFieldPlayerBytes2] = uint32(player.FacialStyle) | uint32(player.BankBagSlots)<<16 | uint32(player.RestState)<<24
		}
		sess.sendPlayerValuesUpdate(fields)
	}
}
