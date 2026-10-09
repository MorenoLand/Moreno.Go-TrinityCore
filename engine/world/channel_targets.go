package world

import (
	"context"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

// C++ owners: Spell::SelectImplicitChannelTargets (Spell.cpp:980-1032),
// SpellImplicitTargetInfo static table (SpellInfo.cpp:296-297, 326): all three
// carry TARGET_SELECT_CATEGORY_CHANNEL with TARGET_REFERENCE_TYPE_CASTER.
const (
	implicitTargetDestChannelTarget uint32 = 76  // TARGET_DEST_CHANNEL_TARGET (SharedDefines.h:1512)
	implicitTargetUnitChannelTarget uint32 = 77  // TARGET_UNIT_CHANNEL_TARGET (SharedDefines.h:1513)
	implicitTargetDestChannelCaster uint32 = 106 // TARGET_DEST_CHANNEL_CASTER (SharedDefines.h:1542)
)

// spellHasChannelDestTarget reports whether any effect carries a channel-dest
// implicit target (76/106). The unit value (77) is already resolved through
// the channelTargetForSpell / periodicTriggerTarget path.
func spellHasChannelDestTarget(spell wotlk.Spell) bool {
	for _, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		if eff.ImplicitTargetA == implicitTargetDestChannelTarget || eff.ImplicitTargetB == implicitTargetDestChannelTarget ||
			eff.ImplicitTargetA == implicitTargetDestChannelCaster || eff.ImplicitTargetB == implicitTargetDestChannelCaster {
			return true
		}
	}
	return false
}

// channelDestForSpell mirrors the destination half of
// Spell::SelectImplicitChannelTargets (Spell.cpp:980-1032): a triggered
// spell's 76/106 implicit target resolves against the session's currently
// channeled spell.
//   - 76 (TARGET_DEST_CHANNEL_TARGET): the channeled spell's own destination
//     first (channeledSpell->m_targets.HasDst(); Go records it as
//     activeChannelState.HasDest at channel start), else the channel-object
//     target's position (C++ falls back to m_originalCaster->
//     GetChannelObjectGuid() via ObjectAccessor::GetWorldObject).
//   - 106 (TARGET_DEST_CHANNEL_CASTER): the channeled spell's caster position
//     (Spell.cpp:1025); Go's channeling caster is always the session player.
//
// The C++ null gate is m_originalCaster->GetCurrentSpell(CURRENT_CHANNELED_SPELL):
// without a live channel nothing is resolved. Script destination hooks
// (CallScriptDestinationTargetSelectHandlers) have no Go infra.
func (s *session) channelDestForSpell(ctx context.Context, spell wotlk.Spell) (x, y, z float32, ok bool) {
	if s == nil || s.player == nil || s.server == nil {
		return 0, 0, 0, false
	}
	s.castMu.Lock()
	channel := s.activeChannel
	s.castMu.Unlock()
	if channel == nil || channel.Stopped {
		return 0, 0, 0, false
	}
	for _, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		for _, targetType := range []uint32{eff.ImplicitTargetA, eff.ImplicitTargetB} {
			switch targetType {
			case implicitTargetDestChannelTarget:
				if channel.HasDest {
					return channel.DestX, channel.DestY, channel.DestZ, true
				}
				if channel.ChannelObjectGUID != 0 {
					// C++ GetWorldObject resolves units and gameobjects; Go
					// has no GO-target model, so players and creatures are
					// covered.
					if ts := s.server.findSessionByGUID(channel.ChannelObjectGUID); ts != nil && ts.player != nil {
						return ts.player.X, ts.player.Y, ts.player.Z, true
					}
					if tgt, found := s.getCombatTarget(ctx, channel.ChannelObjectGUID); found {
						return tgt.X, tgt.Y, tgt.Z, true
					}
				}
				return 0, 0, 0, false
			case implicitTargetDestChannelCaster:
				return s.player.X, s.player.Y, s.player.Z, true
			}
		}
	}
	return 0, 0, 0, false
}
