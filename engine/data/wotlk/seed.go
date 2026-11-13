package wotlk

import _ "embed"

// Minimal embedded Spell.dbc seed (tools/spelldbc_seed/main.go).
// The seed covers only the Wowhead-verified spell rows the Lua/Go legs
// currently cast (Mar'li hatch chain, Vezax set, Oculus set); it is a
// stopgap until real client DBC data is deployed. Disk DBCs always win:
// Store.File consults the seed only when dbc.Open fails, per file, so
// deploying a real Spell.dbc replaces the seed rows wholesale while
// leaving the other seeded tables in place until they too are replaced.
//
//go:embed seeddbc/Spell.dbc
var seedSpellDBC []byte

//go:embed seeddbc/SpellDuration.dbc
var seedSpellDurationDBC []byte

//go:embed seeddbc/SpellRadius.dbc
var seedSpellRadiusDBC []byte

//go:embed seeddbc/SpellCastTimes.dbc
var seedSpellCastTimesDBC []byte

//go:embed seeddbc/SpellRange.dbc
var seedSpellRangeDBC []byte

//go:embed seeddbc/SummonProperties.dbc
var seedSummonPropertiesDBC []byte

func seedDBC(name string) ([]byte, bool) {
	switch name {
	case "Spell":
		return seedSpellDBC, true
	case "SpellDuration":
		return seedSpellDurationDBC, true
	case "SpellRadius":
		return seedSpellRadiusDBC, true
	case "SpellCastTimes":
		return seedSpellCastTimesDBC, true
	case "SpellRange":
		return seedSpellRangeDBC, true
	case "SummonProperties":
		return seedSummonPropertiesDBC, true
	}
	return nil, false
}
