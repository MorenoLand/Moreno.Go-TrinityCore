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
// implicit target (76/106). The unit value (77) is not a dest target: on the
// player path it routes through channelTargetForSpell / periodicTriggerTarget,
// and on the creature paths it resolves via creatureChannelUnitTarget
// (creature_dest_targets.go) in the twin, inline, and pet hit lists.
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
					// C++ GetWorldObject resolves units and gameobjects;
					// units resolve above, static gameobjects via their
					// DB row below (dynamic GOs have no row and stay
					// unresolved).
					if ts := s.server.findSessionByGUID(channel.ChannelObjectGUID); ts != nil && ts.player != nil {
						return ts.player.X, ts.player.Y, ts.player.Z, true
					}
					if tgt, found := s.getCombatTarget(ctx, channel.ChannelObjectGUID); found {
						return tgt.X, tgt.Y, tgt.Z, true
					}
					if uint16(channel.ChannelObjectGUID>>48) == 0xF110 && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
						var goMap int64
						var gx, gy, gz float64
						if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT map, position_x, position_y, position_z FROM gameobject WHERE guid = ?", uint32(channel.ChannelObjectGUID&0xFFFFFF)).Scan(&goMap, &gx, &gy, &gz); err == nil && uint32(goMap) == s.player.Map {
							return float32(gx), float32(gy), float32(gz), true
						}
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

// playerChannelUnitTarget mirrors the unit half of
// Spell::SelectImplicitChannelTargets (Spell.cpp:996-1004) for the
// triggered funnel: the live channel's object GUID, resolved as a unit.
// Unlike channelTargetForSpell it carries no spell-ID match — C++ reads
// the caster's CURRENT_CHANNELED_SPELL whatever the triggered spell's
// ID, so a proc, item cast, or effect-triggered 77 spell fired during
// a channel resolves the channel target, not just the channeled spell
// itself. A gameobject channel object is excluded (C++ GetUnit returns
// null for it, same as the creature twin's gate), and a target that no
// longer resolves on the caster's map/instance is skipped (C++ logs
// "cannot find channel spell target" and adds nothing — the
// teleported-out case). The session branch needs its own map/instance
// match: combatTargetByGUID resolves sessions map-blind, while the
// motion and DB-row branches are already map/instance-scoped. A
// channeled spell's own 77 effects are excluded by the caller, since
// its own channel object is not stamped until after selection — the
// creature twin's gate (creature_dest_targets.go).
func (s *session) playerChannelUnitTarget(ctx context.Context) (uint64, bool) {
	if s == nil || s.player == nil || s.server == nil {
		return 0, false
	}
	s.castMu.Lock()
	channel := s.activeChannel
	s.castMu.Unlock()
	if channel == nil || channel.Stopped || channel.ChannelObjectGUID == 0 {
		return 0, false
	}
	guid := channel.ChannelObjectGUID
	if uint16(guid>>48) == 0xF110 {
		return 0, false
	}
	if ts := s.server.findSessionByGUID(guid); ts != nil && ts.player != nil {
		if ts.player.Map == s.player.Map && ts.player.InstanceID == s.player.InstanceID {
			return guid, true
		}
		return 0, false
	}
	if _, ok := s.getCombatTarget(ctx, guid); !ok {
		return 0, false
	}
	return guid, true
}
