package world

import "math/rand"

// skillDiscoveryEntry mirrors SkillDiscoveryEntry (SkillDiscovery.cpp):
// the spell a reqSpell can discover, the skill value it requires, and
// its discovery chance.
type skillDiscoveryEntry struct {
	spellID       uint32
	reqSkillValue uint32
	chance        float64
}

// skillDiscoverySpells mirrors SkillDiscoveryStore
// (SkillDiscovery.cpp LoadSkillDiscoveryTable): reqSpell -> the
// spells it can discover, from TDB 335.25101 skill_discovery_template
// (reqSpell > 0 rows only; reqSpell == 0 rows are keyed by negative
// skill line and never match a spell-ID lookup in
// HasDiscoveredAllSpells). C++ drops rows whose reqSpell lacks
// MECHANIC_DISCOVERY and is not an explicit-discovery spell; that
// gate is Spell.dbc-bound and unverifiable in-tree (no Spell.dbc),
// so the table is carried as TDB states it.
var skillDiscoverySpells = map[uint32][]skillDiscoveryEntry{
	28571: {{41500, 0, 30}},
	28572: {{41501, 0, 30}},
	28573: {{41502, 0, 30}},
	28575: {{41458, 0, 30}},
	28576: {{41503, 0, 30}},
	60350: {{53771, 400, 50}, {53773, 400, 50}, {53774, 400, 50}, {53775, 400, 50}, {53776, 400, 50}, {53777, 400, 50}, {53779, 400, 50}, {53780, 400, 50}, {53781, 400, 50}, {53782, 400, 50}, {53783, 400, 50}, {53784, 400, 50}},
	60893: {{53895, 400, 100}, {53904, 400, 100}, {54220, 400, 100}, {54221, 400, 100}, {54222, 400, 100}, {56519, 400, 100}, {60354, 400, 100}, {60355, 400, 100}, {60356, 400, 100}, {60357, 400, 100}, {60365, 400, 100}, {60366, 400, 100}, {62410, 400, 100}},
	61177: {{56944, 385, 100}, {56946, 385, 100}, {56947, 385, 100}, {56949, 385, 100}, {56950, 385, 100}, {56954, 385, 100}, {56958, 385, 100}, {56960, 385, 100}, {56975, 385, 100}, {56977, 385, 100}, {56980, 385, 100}, {56983, 385, 100}, {56986, 385, 100}, {56987, 385, 100}, {56988, 385, 100}, {56989, 385, 100}, {56996, 385, 100}, {56998, 385, 100}, {56999, 385, 100}, {57006, 385, 100}, {57010, 385, 100}, {57011, 385, 100}, {57012, 385, 100}, {57013, 385, 100}, {57014, 385, 100}, {57019, 385, 100}, {57021, 385, 100}, {57028, 385, 100}, {57034, 385, 100}, {57035, 385, 100}, {57036, 385, 100}, {57112, 385, 100}, {57115, 385, 100}, {57116, 385, 100}, {57117, 385, 100}, {57124, 385, 100}, {57126, 385, 100}, {57127, 385, 100}, {57128, 385, 100}, {57130, 385, 100}, {57152, 385, 100}, {57153, 385, 100}, {57155, 385, 100}, {57159, 385, 100}, {57160, 385, 100}, {57164, 385, 100}, {57166, 385, 100}, {57169, 385, 100}, {57170, 385, 100}, {57181, 385, 100}, {57189, 385, 100}, {57190, 385, 100}, {57191, 385, 100}, {57193, 385, 100}, {57195, 385, 100}, {57198, 385, 100}, {57199, 385, 100}, {57202, 385, 100}, {57207, 385, 100}, {57208, 385, 100}, {57211, 385, 100}, {57212, 385, 100}, {57214, 385, 100}, {57218, 385, 100}, {57220, 385, 100}, {57223, 385, 100}, {57225, 385, 100}, {57232, 385, 100}, {57233, 385, 100}, {57234, 385, 100}, {57235, 385, 100}, {57237, 385, 100}, {57243, 385, 100}, {57247, 385, 100}, {57248, 385, 100}, {57250, 385, 100}, {57258, 385, 100}, {57260, 385, 100}, {57261, 385, 100}, {57263, 385, 100}, {57264, 385, 100}, {57267, 385, 100}, {57268, 385, 100}, {57273, 385, 100}, {57276, 385, 100}, {57719, 385, 100}, {58322, 385, 100}, {58341, 385, 100}, {59559, 385, 100}, {59560, 385, 100}, {59561, 385, 100}, {61677, 385, 100}},
	61288: {{56965, 350, 100}, {56990, 350, 100}, {57209, 325, 100}, {57215, 300, 100}, {57217, 325, 100}, {57228, 300, 100}, {57229, 300, 100}, {57230, 300, 100}, {57253, 350, 100}, {58286, 100, 100}, {58287, 150, 100}, {58288, 100, 100}, {58289, 75, 100}, {58296, 75, 100}, {58297, 200, 100}, {58298, 75, 100}, {58299, 75, 100}, {58300, 75, 100}, {58301, 75, 100}, {58302, 150, 100}, {58303, 75, 100}, {58305, 100, 100}, {58306, 75, 100}, {58307, 125, 100}, {58308, 75, 100}, {58310, 75, 100}, {58311, 100, 100}, {58312, 75, 100}, {58313, 75, 100}, {58314, 75, 100}, {58315, 100, 100}, {58316, 150, 100}, {58317, 75, 100}, {58318, 75, 100}, {58319, 175, 100}, {58320, 100, 100}, {58321, 150, 100}, {58322, 350, 100}, {58323, 75, 100}, {58324, 125, 100}, {58325, 100, 100}, {58326, 75, 100}, {58327, 200, 100}, {58328, 125, 100}, {58329, 150, 100}, {58330, 150, 100}, {58331, 125, 100}, {58332, 100, 100}, {58333, 150, 100}, {58336, 100, 100}, {58337, 75, 100}, {58338, 150, 100}, {58339, 150, 100}, {58340, 125, 100}, {58341, 350, 100}, {58342, 75, 100}, {58343, 75, 100}, {58344, 75, 100}, {58345, 100, 100}, {58346, 75, 100}, {58347, 325, 100}, {59315, 150, 100}, {59326, 100, 100}, {68166, 350, 100}},
	61756: {{56944, 0, 100}, {56946, 0, 100}, {56947, 0, 100}, {56949, 0, 100}, {56950, 0, 100}, {56954, 0, 100}, {56958, 0, 100}, {56960, 0, 100}, {56975, 0, 100}, {56977, 0, 100}, {56980, 0, 100}, {56983, 0, 100}, {56986, 0, 100}, {56987, 0, 100}, {56988, 0, 100}, {56989, 0, 100}, {56996, 0, 100}, {56998, 0, 100}, {56999, 0, 100}, {57006, 0, 100}, {57010, 0, 100}, {57011, 0, 100}, {57012, 0, 100}, {57013, 0, 100}, {57014, 0, 100}, {57019, 0, 100}, {57021, 0, 100}, {57028, 0, 100}, {57034, 0, 100}, {57035, 0, 100}, {57036, 0, 100}, {57112, 0, 100}, {57115, 0, 100}, {57116, 0, 100}, {57117, 0, 100}, {57124, 0, 100}, {57126, 0, 100}, {57127, 0, 100}, {57128, 0, 100}, {57130, 0, 100}, {57152, 0, 100}, {57153, 0, 100}, {57155, 0, 100}, {57159, 0, 100}, {57160, 0, 100}, {57164, 0, 100}, {57166, 0, 100}, {57169, 0, 100}, {57170, 0, 100}, {57181, 0, 100}, {57189, 0, 100}, {57190, 0, 100}, {57191, 0, 100}, {57193, 0, 100}, {57195, 0, 100}, {57198, 0, 100}, {57199, 0, 100}, {57202, 0, 100}, {57207, 0, 100}, {57208, 0, 100}, {57211, 0, 100}, {57212, 0, 100}, {57214, 0, 100}, {57218, 0, 100}, {57220, 0, 100}, {57223, 0, 100}, {57225, 0, 100}, {57232, 0, 100}, {57233, 0, 100}, {57234, 0, 100}, {57235, 0, 100}, {57237, 0, 100}, {57243, 0, 100}, {57247, 0, 100}, {57248, 0, 100}, {57250, 0, 100}, {57258, 0, 100}, {57260, 0, 100}, {57261, 0, 100}, {57263, 0, 100}, {57264, 0, 100}, {57267, 0, 100}, {57268, 0, 100}, {57273, 0, 100}, {57276, 0, 100}, {57719, 0, 100}, {58322, 0, 100}, {58341, 0, 100}, {59559, 0, 100}, {59560, 0, 100}, {59561, 0, 100}, {61677, 0, 100}},
	64323: {{64246, 0, 100}, {64247, 0, 100}, {64248, 0, 100}, {64249, 0, 100}, {64250, 0, 100}, {64251, 0, 100}, {64252, 0, 100}, {64253, 0, 100}, {64254, 0, 100}, {64255, 0, 100}, {64256, 0, 100}, {64257, 0, 100}, {64268, 0, 100}, {64270, 0, 100}, {64271, 0, 100}, {64273, 0, 100}, {64274, 0, 100}, {64275, 0, 100}, {64276, 0, 100}, {64277, 0, 100}, {64278, 0, 100}, {64279, 0, 100}, {64280, 0, 100}, {64281, 0, 100}, {64282, 0, 100}, {64283, 0, 100}, {64284, 0, 100}, {64285, 0, 100}, {64286, 0, 100}, {64287, 0, 100}, {64288, 0, 100}, {64289, 0, 100}, {64291, 0, 100}, {64294, 0, 100}, {64295, 0, 100}, {64296, 0, 100}, {64297, 0, 100}, {64298, 0, 100}, {64299, 0, 100}, {64300, 0, 100}, {64302, 0, 100}, {64303, 0, 100}, {64304, 0, 100}, {64305, 0, 100}, {64307, 0, 100}, {64308, 0, 100}, {64309, 0, 100}, {64310, 0, 100}, {64311, 0, 100}, {64312, 0, 100}, {64313, 0, 100}, {64314, 0, 100}, {64315, 0, 100}, {64316, 0, 100}, {64317, 0, 100}, {64318, 0, 100}, {65245, 0, 100}},
}

// hasDiscoveredAllSpells mirrors HasDiscoveredAllSpells
// (SkillDiscovery.cpp:195-208): no store entry means nothing is left to
// discover (true); otherwise every listed spell must be known. Known
// uses the Lua-visible HasSpell predicate (Active and not Disabled),
// matching the adjacent method in lua_player.go.
func hasDiscoveredAllSpells(state *playerState, spellID uint32) bool {
	if state == nil {
		return true
	}
	spells, ok := skillDiscoverySpells[spellID]
	if !ok {
		return true
	}
	for _, entry := range spells {
		if !discoverySpellKnown(state, entry.spellID) {
			return false
		}
	}
	return true
}

// hasDiscoveredAnySpell mirrors HasDiscoveredAnySpell
// (SkillDiscovery.cpp:210-222): no store entry means nothing was ever
// discoverable (false); otherwise any one listed spell known is enough.
func hasDiscoveredAnySpell(state *playerState, spellID uint32) bool {
	if state == nil {
		return false
	}
	spells, ok := skillDiscoverySpells[spellID]
	if !ok {
		return false
	}
	for _, entry := range spells {
		if discoverySpellKnown(state, entry.spellID) {
			return true
		}
	}
	return false
}

// getExplicitDiscoverySpell mirrors GetExplicitDiscoverySpell
// (SkillDiscovery.cpp:158-192): a weighted pick among the reqSpell's
// undiscovered entries whose reqSkillValue the player's skill meets.
// The skill line comes from the reqSpell's first SkillLineAbility row
// (no row, or no DBC at runtime, reads skill 0, the C++ bounds-empty
// twin); the skill value is Player::GetSkillValue
// (playerSkillTotalValue). The roll is rand_chance() * fullChance/100
// (rand.Float64()*100 is the rand_chance twin, procs.go:245), walked
// the C++ way, so with the TDB all-100 chances behind the research
// spells the pick is uniform among the eligible undiscovered spells.
// 0 means no eligible undiscovered spell.
func (s *session) getExplicitDiscoverySpell(reqSpell uint32) uint32 {
	if s == nil || s.player == nil {
		return 0
	}
	state := s.player
	entries, ok := skillDiscoverySpells[reqSpell]
	if !ok {
		return 0
	}
	skillValue := uint32(0)
	if s != nil && s.server != nil && s.server.Data != nil {
		if abilities, found, err := s.server.Data.SkillLineAbilities(reqSpell); err == nil && found && len(abilities) > 0 {
			skillValue = uint32(s.playerSkillTotalValue(abilities[0].SkillLine))
		}
	}
	fullChance := float64(0)
	for _, entry := range entries {
		if entry.reqSkillValue > skillValue {
			continue
		}
		if discoverySpellKnown(state, entry.spellID) {
			continue
		}
		fullChance += entry.chance
	}
	roll := rand.Float64() * 100 * (fullChance / 100)
	for _, entry := range entries {
		if entry.reqSkillValue > skillValue {
			continue
		}
		if discoverySpellKnown(state, entry.spellID) {
			continue
		}
		if entry.chance > roll {
			return entry.spellID
		}
		roll -= entry.chance
	}
	return 0
}

// skillDiscoveryRate mirrors RATE_SKILL_DISCOVERY (World.cpp:535):
// Rate.Skill.Discovery, default 1.0. Go reads no world rates, so the
// default is the value.
const skillDiscoveryRate = 1.0

// skillDiscoveryBySkillLine mirrors the reqSpell == 0 rows of
// skill_discovery_template (SkillDiscovery.cpp LoadSkillDiscoveryTable):
// keyed by skill line. No such rows are carried in-tree (the table above
// holds reqSpell > 0 rows only, and world.db's skill_discovery_template
// is empty), so the map is empty and the skill-line case below never
// fires until TDB rows are loaded.
var skillDiscoveryBySkillLine = map[uint32][]skillDiscoveryEntry{}

// getSkillDiscoverySpell mirrors GetSkillDiscoverySpell
// (SkillDiscovery.cpp:224-257): the first undiscovered entry for the
// crafted spell whose reqSkillValue the player's total skill value meets
// and whose chance * RATE_SKILL_DISCOVERY roll passes. The roll is
// evaluated first per entry, matching the C++ && order (a failed roll
// consumes the RNG before the skill and known gates are read). The
// spell case returns 0 on a miss with no fallthrough to the skill-line
// case; the skill-line case runs only when no spell-case tab exists
// and the skill line is nonzero. 0 means no discovery.
func (s *session) getSkillDiscoverySpell(skillID, spellID uint32) uint32 {
	if s == nil || s.player == nil {
		return 0
	}
	skillValue := uint32(0)
	if skillID != 0 {
		skillValue = uint32(s.playerSkillTotalValue(skillID))
	}
	if entries, ok := skillDiscoverySpells[spellID]; ok {
		for _, entry := range entries {
			if rand.Float64()*100 < entry.chance*skillDiscoveryRate && entry.reqSkillValue <= skillValue && !discoverySpellKnown(s.player, entry.spellID) {
				return entry.spellID
			}
		}
		return 0
	}
	if skillID == 0 {
		return 0
	}
	for _, entry := range skillDiscoveryBySkillLine[skillID] {
		if rand.Float64()*100 < entry.chance*skillDiscoveryRate && entry.reqSkillValue <= skillValue && !discoverySpellKnown(s.player, entry.spellID) {
			return entry.spellID
		}
	}
	return 0
}

// discoverySpellKnown is the HasSpell predicate shared by the
// discovery helpers: Active and not Disabled.
func discoverySpellKnown(state *playerState, spellID uint32) bool {
	for _, s := range state.Spells {
		if s.ID == spellID && s.Active && !s.Disabled {
			return true
		}
	}
	return false
}
