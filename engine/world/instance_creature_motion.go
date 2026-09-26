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
