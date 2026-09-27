package world

type NpcBotRuntimeState struct {
	GUID      uint64
	OwnerGUID uint64
	Entry     uint32
	PetID     uint32
}

func ResolveNpcBotRuntimeCount(ownerGUID uint64, entries map[uint32]struct{}, runtime []NpcBotRuntimeState) uint8 {
	seen := make(map[uint64]struct{}, len(runtime))
	var count uint8
	for _, bot := range runtime {
		if bot.GUID == 0 || bot.OwnerGUID != ownerGUID || bot.PetID != 0 {
			continue
		}
		if _, ok := entries[bot.Entry]; !ok {
			continue
		}
		if _, ok := seen[bot.GUID]; ok {
			continue
		}
		seen[bot.GUID] = struct{}{}
		if count < ^uint8(0) {
			count++
		}
	}
	return count
}

func (s *Server) runtimeNpcBotCountByOwner(ownerGUID uint64) uint8 {
	if s == nil || ownerGUID == 0 || s.Features == nil || s.Features.NPCBots == nil {
		return 0
	}
	return s.runtimeNpcBotCountForSession(s.findSessionByGUID(ownerGUID))
}

func (s *Server) runtimeNpcBotCountForSession(owner *session) uint8 {
	if s == nil || owner == nil || owner.player == nil || s.Features == nil || s.Features.NPCBots == nil {
		return 0
	}
	s.motionMu.Lock()
	runtime := make([]NpcBotRuntimeState, 0)
	for _, motion := range s.motionMapLocked(owner.player.Map, owner.player.InstanceID) {
		if motion != nil && motion.OwnerGUID == owner.playerGUID && motion.PetID == 0 {
			runtime = append(runtime, NpcBotRuntimeState{GUID: motion.GUID, OwnerGUID: motion.OwnerGUID, Entry: motion.Entry, PetID: motion.PetID})
		}
	}
	s.motionMu.Unlock()
	entries := make(map[uint32]struct{})
	for _, bot := range runtime {
		if _, ok := s.Features.NPCBots.Extras(bot.Entry); ok {
			entries[bot.Entry] = struct{}{}
		}
	}
	return ResolveNpcBotRuntimeCount(owner.playerGUID, entries, runtime)
}

func (s *Server) instanceOccupantCount(owner *session, countNpcBots bool) uint32 {
	if owner == nil {
		return 0
	}
	count := uint32(1)
	if countNpcBots {
		count += uint32(s.runtimeNpcBotCountForSession(owner))
	}
	return count
}
