package world

type creatureAuraKey struct {
	Map        uint32
	InstanceID uint32
	GUID       uint64
}

func creatureAuraKeyForTarget(target combatTarget) creatureAuraKey {
	return creatureAuraKey{Map: target.Map, InstanceID: target.InstanceID, GUID: target.GUID}
}

func creatureAuraKeyForPlayer(state playerState, guid uint64) creatureAuraKey {
	return creatureAuraKey{Map: state.Map, InstanceID: state.InstanceID, GUID: guid}
}

func creatureAuraKeyForMotion(motion *creatureMotion) creatureAuraKey {
	if motion == nil {
		return creatureAuraKey{}
	}
	return creatureAuraKey{Map: motion.Map, InstanceID: motion.InstanceID, GUID: motion.GUID}
}
