package world

import (
	"context"
	"math"
	"time"
)

// visibilityDirtyEntry is one deferred Unit::UpdateObjectVisibility mark:
// the unit's map/instance and position at mark time, for the forced
// fan-out the drain runs.
type visibilityDirtyEntry struct {
	mapID, instanceID uint32
	x, y              float32
}

// updateUnitVisibility mirrors Unit::UpdateObjectVisibility
// (Unit.cpp:12497-12509). Non-forced only marks the unit dirty — the C++
// AddToNotify(NOTIFY_VISIBILITY_CHANGED) flag, consumed by the next grid
// visit there and by the next updateActiveCreatures pass here
// (drainVisibilityDirty). Forced runs the fan-out immediately: the
// VisibleChangesNotifier leg (Object.cpp:3543, send create/destroy
// updates to nearby players) plus the AIRelocationNotifier leg
// (Unit.cpp:12506-12508, re-run MoveInLineOfSight for nearby creatures).
func (s *Server) updateUnitVisibility(ctx context.Context, mapID, instanceID uint32, guid uint64, x, y float32, forced bool) {
	if s == nil {
		return
	}
	if !forced {
		s.visibilityDirtyMu.Lock()
		if s.visibilityDirty == nil {
			s.visibilityDirty = make(map[uint64]visibilityDirtyEntry)
		}
		s.visibilityDirty[guid] = visibilityDirtyEntry{mapID: mapID, instanceID: instanceID, x: x, y: y}
		s.visibilityDirtyMu.Unlock()
		return
	}
	s.updateUnitVisibilityForced(ctx, mapID, instanceID, x, y)
}

// updateUnitVisibilityForced runs the forced arm of
// Unit::UpdateObjectVisibility immediately for the unit at (x, y).
func (s *Server) updateUnitVisibilityForced(ctx context.Context, mapID, instanceID uint32, x, y float32) {
	if s == nil {
		return
	}
	// VisibleChangesNotifier leg: Go keeps no persistent per-player
	// visibility set, so the rebuild runs for every observer, as the
	// existing invisibility/phase-change call sites do.
	s.refreshPlayerVisibility()
	players := s.snapshotPlayerPositions()
	if len(players) == 0 {
		return
	}
	s.relocateVisibilityMIOS(ctx, mapID, instanceID, x, y, players, time.Now())
}

// drainVisibilityDirty consumes the deferred UpdateObjectVisibility marks:
// one VisibleChangesNotifier pass covers every dirty unit (the Go rebuild
// is observer-global, not range-limited like C++'s), then the
// AIRelocationNotifier leg runs per mark. Called from
// updateActiveCreatures on the world tick.
func (s *Server) drainVisibilityDirty(ctx context.Context) {
	if s == nil {
		return
	}
	s.visibilityDirtyMu.Lock()
	if len(s.visibilityDirty) == 0 {
		s.visibilityDirtyMu.Unlock()
		return
	}
	entries := make([]visibilityDirtyEntry, 0, len(s.visibilityDirty))
	for _, e := range s.visibilityDirty {
		entries = append(entries, e)
	}
	s.visibilityDirty = make(map[uint64]visibilityDirtyEntry)
	s.visibilityDirtyMu.Unlock()
	s.refreshPlayerVisibility()
	players := s.snapshotPlayerPositions()
	if len(players) == 0 {
		return
	}
	now := time.Now()
	for _, e := range entries {
		s.relocateVisibilityMIOS(ctx, e.mapID, e.instanceID, e.x, e.y, players, now)
	}
}

// relocateVisibilityMIOS is the AIRelocationNotifier leg of
// Unit::UpdateObjectVisibility (Unit.cpp:12506-12508): re-run
// MoveInLineOfSight for creature motions within visibility range of
// (x, y), via the per-tick proximity step.
func (s *Server) relocateVisibilityMIOS(ctx context.Context, mapID, instanceID uint32, x, y float32, players []playerPos, now time.Time) {
	if s == nil || len(players) == 0 {
		return
	}
	visRange := float32(s.visibilityRangeForMap(mapID))
	if visRange <= 0 {
		visRange = 100.0
	}
	s.motionMu.Lock()
	motions := make([]*creatureMotion, 0)
	for _, m := range s.motionMapLocked(mapID, instanceID) {
		if m == nil {
			continue
		}
		if math.Hypot(float64(m.X-x), float64(m.Y-y)) <= float64(visRange) {
			motions = append(motions, m)
		}
	}
	s.motionMu.Unlock()
	for _, m := range motions {
		s.stepCreatureMoveInLOSProximity(ctx, m, players, now)
	}
}

// snapshotPlayerPositions is the online-player snapshot updateActiveCreatures
// builds for the creature sweep: every world-ready session with its
// position, GM/dead flags, faction template and reputations.
func (s *Server) snapshotPlayerPositions() []playerPos {
	if s == nil {
		return nil
	}
	var players []playerPos
	s.sessionsMu.RLock()
	for sess := range s.sessions {
		if sess.worldReady.Load() && sess.player != nil {
			isGM := (sess.player.ExtraFlags&playerExtraGMOn != 0) || (sess.player.PlayerFlags&playerFlagGM != 0)
			// Phase-anywhere rides on GM_ON; serverside-hidden visibility
			// rides on the GM visibility detect (see creatures.go) — a GM
			// after `.gm visible on` keeps phase-anywhere but loses hidden.
			seesHidden := sess.gmVisibilityDetect != 0
			isDead := (sess.player.Health == 0 && sess.player.MaxHealth > 0) || sess.player.PlayerFlags&playerFlagGhost != 0
			players = append(players, playerPos{
				Map:             sess.player.Map,
				InstanceID:      sess.player.InstanceID,
				X:               sess.player.X,
				Y:               sess.player.Y,
				Z:               sess.player.Z,
				GUID:            sess.playerGUID,
				Race:            sess.player.Race,
				Class:           sess.player.Class,
				Level:           sess.player.Level,
				IsGM:            isGM,
				SeesHidden:      seesHidden,
				IsDead:          isDead,
				FactionTemplate: sess.effectiveFactionTemplate(sess.player.Race),
				Reputations:     playerReputationMap(sess.player.Reputations),
				Sess:            sess,
			})
		}
	}
	s.sessionsMu.RUnlock()
	return players
}
