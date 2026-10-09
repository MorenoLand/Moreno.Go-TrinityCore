package world

func (s *Server) isCreatureEvadingInInstance(mapID, instanceID uint32, guid uint64) bool {
	if s == nil {
		return false
	}
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	motion := s.findCreatureMotionLocked(mapID, instanceID, guid)
	return motion != nil && motion.Evading
}

// creatureLastSanctuaryTime reads the creatureMotion analog of
// Unit::m_lastSanctuaryTime (Unit.h:1459) for the DoTargetSpellHit
// sanctuary arm (Spell.cpp:2403).
func (s *Server) creatureLastSanctuaryTime(mapID, instanceID uint32, guid uint64) uint32 {
	if s == nil {
		return 0
	}
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	motion := s.findCreatureMotionLocked(mapID, instanceID, guid)
	if motion == nil {
		return 0
	}
	return motion.LastSanctuaryTime
}
