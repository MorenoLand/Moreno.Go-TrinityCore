package world

import (
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

// spellSearchPlayersOnly ports the attribute arms of
// Spell::GetSearcherTypeMask (Spell.cpp:1836-1842) for unit-container
// searches (area, nearby, chain, and the trajectory candidate sweep):
// SPELL_ATTR3_ONLY_TARGET_PLAYERS narrows the mask to PLAYER|CORPSE and
// SPELL_ATTR3_ONLY_TARGET_GHOSTS narrows it to PLAYER, so the CREATURE
// container drops out of every unit search. Corpses have no Go model, so
// both arms collapse to players-only here. The trajectory search uses
// GRID_MAP_TYPE_MASK_ALL in C++ (Spell.cpp:1643), not GetSearcherTypeMask,
// but its per-candidate SpellInfo::CheckTarget gate (implicit, Spell.cpp:8316
// -> SpellInfo.cpp:1654-1662/1712-1713) rejects non-player and non-ghost
// candidates the same way, so the same players-only collapse applies.
// The ONLY_TARGET_GHOSTS per-candidate ghost-aura gate is a documented
// no-bridge: ghost players are dead (health 0) and Go's session sweeps skip
// dead players — there is no dead-player container to search.
func spellSearchPlayersOnly(spell wotlk.Spell) bool {
	return spell.AttributesEx3&(spellAttr3OnlyTargetPlayers|spellAttr3OnlyTargetGhosts) != 0
}
