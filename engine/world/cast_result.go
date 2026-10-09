package world

import (
	"context"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// Failure-result payload parity — Spell::WriteCastResultInfo (Spell.cpp:3974).
// C++ appends per-result extended data after castCount/spellID/result on
// SMSG_CAST_FAILED (and SMSG_PET_CAST_FAILED); Go's buildCastFailed wrote
// only the 6 base bytes, so the client got truncated payloads for results
// that carry parameters (focus id, totem ids, missing reagent, item class).

const spellFailedUnitNotInFront uint8 = 134 // SPELL_FAILED_UNIT_NOT_INFRONT (SharedDefines.h:1160)

func buildCastFailedParams(castID uint8, spellID uint32, result uint8, params ...uint32) []byte {
	buf := protocol.NewBuffer(6 + 4*len(params))
	buf.WriteU8(castID)
	buf.WriteU32(spellID)
	buf.WriteU8(result)
	for _, p := range params {
		buf.WriteU32(p)
	}
	return buf.Bytes()
}

// castFailedExtParams mirrors the WriteCastResultInfo switch for the results
// Go actually sends, resolved from the loaded Spell.dbc record like C++.
// The REQUIRES_AREA hardcoded spell/area pairs have no Go send sites (the
// uint32(0) default arm is bridged); CUSTOM_ERROR customError rides
// buildCastFailedParams at its send sites; param1/param2 override paths have
// no Go send sites. TRIGGERED_DONT_REPORT_CAST_ERROR is vacuous (triggered
// casts bypass handleCastSpell).
func (s *session) castFailedExtParams(ctx context.Context, spell wotlk.Spell, result uint8) []uint32 {
	switch result {
	case spellFailedRequiresSpellFocus:
		return []uint32{spell.RequiresSpellFocus}
	case spellFailedTotems:
		var out []uint32
		for _, totem := range spell.Totem {
			if totem != 0 {
				out = append(out, totem)
			}
		}
		return out
	case spellFailedTotemCategory:
		var out []uint32
		for _, category := range spell.RequiredTotemCategory {
			if category != 0 {
				out = append(out, category)
			}
		}
		return out
	case spellFailedEquippedItemClass, spellFailedEquippedItemClassMainhand, spellFailedEquippedItemClassOffhand:
		return []uint32{uint32(spell.EquippedItemClass), spell.EquippedItemSubClass}
	case spellFailedReagents: // SPELL_FAILED_REAGENTS: first missing reagent item id
		return []uint32{s.firstMissingSpellReagent(ctx, spell)}
	case spellFailedTooManyOfItem:
		// WriteCastResultInfo (Spell.cpp:4056-4064): the trailer is the
		// created item's ItemLimitCategory — and no trailer at all when the
		// item has none (the C++ arm writes only when LimitCategory != 0).
		for i := 0; i < len(spell.Effects); i++ {
			if spell.Effects[i].ItemType != 0 {
				if s != nil && s.server != nil {
					if info, ok := s.server.getItemStoreTemplateInfo(ctx, spell.Effects[i].ItemType); ok && info.LimitCategory != 0 {
						return []uint32{info.LimitCategory}
					}
				}
				break
			}
		}
		return nil
	case spellFailedRequiresArea:
		// WriteCastResultInfo (Spell.cpp:4020-4042): the hardcoded area ids
		// (3905/3842/4075) belong to spells Go never sends; every other
		// spell falls through to the uint32(0) default arm.
		return []uint32{0}
	}
	return nil
}

// firstMissingSpellReagent mirrors the SPELL_FAILED_REAGENTS branch of
// WriteCastResultInfo: skip Reagent[i] <= 0, return the first item whose
// inventory count is short (0 when none, which C++ also sends as 0).
func (s *session) firstMissingSpellReagent(ctx context.Context, spell wotlk.Spell) uint32 {
	if s == nil || s.player == nil {
		return 0
	}
	for i := range spell.Reagent {
		if spell.Reagent[i] <= 0 {
			continue
		}
		itemID := uint32(spell.Reagent[i])
		need := spell.ReagentCount[i]
		if need == 0 {
			need = 1
		}
		if s.playerItemCount(ctx, itemID) < int64(need) {
			return itemID
		}
	}
	return 0
}

// sendCastFailed writes SMSG_CAST_FAILED with the C++ extended payload for
// results that carry one. Call sites already hold the loaded spell record.
func (s *session) sendCastFailed(ctx context.Context, castID uint8, spell wotlk.Spell, result uint8) {
	params := s.castFailedExtParams(ctx, spell, result)
	_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailedParams(castID, spell.ID, result, params...), true)
}
