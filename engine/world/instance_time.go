package world

import (
	"context"
	"time"
)

func ResolveAccountInstanceEnterTime(previous map[uint32]int64, instanceID uint32, enteredAt time.Time) map[uint32]int64 {
	entries := make(map[uint32]int64, len(previous)+1)
	for id, releaseTime := range previous {
		if releaseTime >= enteredAt.Unix() {
			entries[id] = releaseTime
		}
	}
	if instanceID != 0 {
		if _, exists := entries[instanceID]; !exists {
			entries[instanceID] = enteredAt.Add(time.Hour).Unix()
		}
	}
	return entries
}

func (s *session) recordInstanceEnterTime(ctx context.Context, enteredAt time.Time) {
	if s == nil || s.server == nil || s.server.Data == nil || s.player == nil || s.player.InstanceID == 0 {
		return
	}
	mapInfo, found, err := s.server.Data.Map(s.player.Map)
	if err != nil || !found || !mapInfo.IsDungeon() {
		return
	}
	if s.groupID != 0 {
		if group := s.server.getGroup(s.groupID); group != nil && group.IsLFG {
			return
		}
	}
	s.instanceLockTimes = ResolveAccountInstanceEnterTime(s.instanceLockTimes, s.player.InstanceID, enteredAt)
}
