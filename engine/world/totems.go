package world

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// Totem slot constants matching WoW 3.3.5 / TrinityCore (SharedDefines.h).
const (
	TotemSlotEarth uint8 = 0
	TotemSlotFire  uint8 = 1
	TotemSlotWater uint8 = 2
	TotemSlotAir   uint8 = 3
)

type TotemPulseType uint8

const (
	TotemPulseNone TotemPulseType = iota
	TotemPulseBuff
	TotemPulseHeal
	TotemPulseAoEDamage
	TotemPulseSingleTarget
	TotemPulseDispel
	TotemPulseSlow
	// TotemPulseSentry bridges the sentry totem (entry 3968) minimap ping:
	// TotemAI::AttackStart (TotemAI.cpp:84-96) sends MSG_MINIMAP_PING with the
	// totem's position to the owner when a hostile unit enters aggro range
	// (the sentry totem is REACT_AGGRESSIVE, Totem.cpp:103-104, so sighting a
	// hostile drives MoveInLineOfSight -> AttackStart).
	TotemPulseSentry
)

type TotemDef struct {
	SpellID     uint32
	SlotID      uint8
	Entry       uint32
	DurationMs  uint32
	PulseType   TotemPulseType
	BuffSpell   uint32
	DebuffSpell uint32
	PulseSpell  uint32
	Radius      float32
	PulseSecs   int
}

type activeTotem struct {
	mu          sync.Mutex
	SlotID      uint8
	SpellID     uint32
	TotemGUID   uint64
	Entry       uint32
	OwnerGUID   uint64
	Map         uint32
	InstanceID  uint32
	X, Y, Z     float32
	DurationMs  uint32
	BuffSpell   uint32
	DebuffSpell uint32
	CreatedAt   time.Time
	StopChan    chan struct{}
	Stopped     bool
	// LastPingVictim is the sentry-totem ping arm's acquisition memory
	// (TotemAI::AttackStart, TotemAI.cpp:84-96): the ping fires once per
	// newly acquired hostile, so the GUID is remembered until the totem
	// has no hostile in range again.
	LastPingVictim uint64
	// LastVictimGUID is the TotemAI::_victimGUID twin (TotemAI.cpp:44,66-82):
	// the damage totem remembers its acquired victim across pulses and reuses
	// it while it is still attackable, in range, and hostile; only then does
	// a new nearest-attackable search run.
	LastVictimGUID uint64
	// MagnetUsed marks a Grounding Totem whose SPELL_AURA_SPELL_MAGNET
	// charge has been consumed by the SelectExplicitTargets redirect
	// (WorldObject::GetMagicHitRedirectTarget, Object.cpp:3188-3217):
	// one redirect per totem, mirroring the C++ DropCharge.
	MagnetUsed bool
}

var totemDefinitions = map[uint32]TotemDef{
	// --- Earth Totems (Slot 0) ---
	// Stoneskin Totem
	8071:  {SpellID: 8071, SlotID: TotemSlotEarth, Entry: 3573, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 8072, Radius: 30.0, PulseSecs: 2},
	8154:  {SpellID: 8154, SlotID: TotemSlotEarth, Entry: 3573, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 8072, Radius: 30.0, PulseSecs: 2},
	8155:  {SpellID: 8155, SlotID: TotemSlotEarth, Entry: 3573, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 8072, Radius: 30.0, PulseSecs: 2},
	10406: {SpellID: 10406, SlotID: TotemSlotEarth, Entry: 3573, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 8072, Radius: 30.0, PulseSecs: 2},
	10407: {SpellID: 10407, SlotID: TotemSlotEarth, Entry: 3573, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 8072, Radius: 30.0, PulseSecs: 2},
	10408: {SpellID: 10408, SlotID: TotemSlotEarth, Entry: 3573, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 8072, Radius: 30.0, PulseSecs: 2},
	25508: {SpellID: 25508, SlotID: TotemSlotEarth, Entry: 3573, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 8072, Radius: 30.0, PulseSecs: 2},
	25509: {SpellID: 25509, SlotID: TotemSlotEarth, Entry: 3573, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 8072, Radius: 30.0, PulseSecs: 2},
	58751: {SpellID: 58751, SlotID: TotemSlotEarth, Entry: 3573, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 8072, Radius: 30.0, PulseSecs: 2},
	58753: {SpellID: 58753, SlotID: TotemSlotEarth, Entry: 3573, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 8072, Radius: 30.0, PulseSecs: 2},

	// Strength of Earth Totem
	8075:  {SpellID: 8075, SlotID: TotemSlotEarth, Entry: 5873, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 8076, Radius: 30.0, PulseSecs: 2},
	8160:  {SpellID: 8160, SlotID: TotemSlotEarth, Entry: 5873, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 8076, Radius: 30.0, PulseSecs: 2},
	8161:  {SpellID: 8161, SlotID: TotemSlotEarth, Entry: 5873, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 8076, Radius: 30.0, PulseSecs: 2},
	10442: {SpellID: 10442, SlotID: TotemSlotEarth, Entry: 5873, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 8076, Radius: 30.0, PulseSecs: 2},
	25361: {SpellID: 25361, SlotID: TotemSlotEarth, Entry: 5873, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 8076, Radius: 30.0, PulseSecs: 2},
	25528: {SpellID: 25528, SlotID: TotemSlotEarth, Entry: 5873, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 8076, Radius: 30.0, PulseSecs: 2},
	58643: {SpellID: 58643, SlotID: TotemSlotEarth, Entry: 5873, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 8076, Radius: 30.0, PulseSecs: 2},

	// Tremor Totem
	8143: {SpellID: 8143, SlotID: TotemSlotEarth, Entry: 5913, DurationMs: 300000, PulseType: TotemPulseDispel, Radius: 30.0, PulseSecs: 3},

	// Earthbind Totem
	2484: {SpellID: 2484, SlotID: TotemSlotEarth, Entry: 2630, DurationMs: 45000, PulseType: TotemPulseSlow, PulseSpell: 3600, Radius: 10.0, PulseSecs: 3},

	// Stoneclaw Totem
	5730:  {SpellID: 5730, SlotID: TotemSlotEarth, Entry: 3579, DurationMs: 15000, PulseType: TotemPulseNone, Radius: 8.0, PulseSecs: 2},
	6390:  {SpellID: 6390, SlotID: TotemSlotEarth, Entry: 3911, DurationMs: 15000, PulseType: TotemPulseNone, Radius: 8.0, PulseSecs: 2},
	6391:  {SpellID: 6391, SlotID: TotemSlotEarth, Entry: 3912, DurationMs: 15000, PulseType: TotemPulseNone, Radius: 8.0, PulseSecs: 2},
	6392:  {SpellID: 6392, SlotID: TotemSlotEarth, Entry: 3913, DurationMs: 15000, PulseType: TotemPulseNone, Radius: 8.0, PulseSecs: 2},
	10427: {SpellID: 10427, SlotID: TotemSlotEarth, Entry: 7398, DurationMs: 15000, PulseType: TotemPulseNone, Radius: 8.0, PulseSecs: 2},
	10428: {SpellID: 10428, SlotID: TotemSlotEarth, Entry: 7399, DurationMs: 15000, PulseType: TotemPulseNone, Radius: 8.0, PulseSecs: 2},
	25525: {SpellID: 25525, SlotID: TotemSlotEarth, Entry: 15478, DurationMs: 15000, PulseType: TotemPulseNone, Radius: 8.0, PulseSecs: 2},
	58580: {SpellID: 58580, SlotID: TotemSlotEarth, Entry: 31120, DurationMs: 15000, PulseType: TotemPulseNone, Radius: 8.0, PulseSecs: 2},
	58581: {SpellID: 58581, SlotID: TotemSlotEarth, Entry: 31121, DurationMs: 15000, PulseType: TotemPulseNone, Radius: 8.0, PulseSecs: 2},
	58582: {SpellID: 58582, SlotID: TotemSlotEarth, Entry: 31122, DurationMs: 15000, PulseType: TotemPulseNone, Radius: 8.0, PulseSecs: 2},

	// Earth Elemental Totem
	2062: {SpellID: 2062, SlotID: TotemSlotEarth, Entry: 15353, DurationMs: 120000, PulseType: TotemPulseNone, Radius: 0, PulseSecs: 0},

	// --- Fire Totems (Slot 1) ---
	// Searing Totem
	3599:  {SpellID: 3599, SlotID: TotemSlotFire, Entry: 2523, DurationMs: 60000, PulseType: TotemPulseSingleTarget, PulseSpell: 3606, Radius: 20.0, PulseSecs: 2},
	6363:  {SpellID: 6363, SlotID: TotemSlotFire, Entry: 2523, DurationMs: 60000, PulseType: TotemPulseSingleTarget, PulseSpell: 3606, Radius: 20.0, PulseSecs: 2},
	6364:  {SpellID: 6364, SlotID: TotemSlotFire, Entry: 2523, DurationMs: 60000, PulseType: TotemPulseSingleTarget, PulseSpell: 3606, Radius: 20.0, PulseSecs: 2},
	6365:  {SpellID: 6365, SlotID: TotemSlotFire, Entry: 2523, DurationMs: 60000, PulseType: TotemPulseSingleTarget, PulseSpell: 3606, Radius: 20.0, PulseSecs: 2},
	10437: {SpellID: 10437, SlotID: TotemSlotFire, Entry: 2523, DurationMs: 60000, PulseType: TotemPulseSingleTarget, PulseSpell: 3606, Radius: 20.0, PulseSecs: 2},
	10438: {SpellID: 10438, SlotID: TotemSlotFire, Entry: 2523, DurationMs: 60000, PulseType: TotemPulseSingleTarget, PulseSpell: 3606, Radius: 20.0, PulseSecs: 2},
	25533: {SpellID: 25533, SlotID: TotemSlotFire, Entry: 2523, DurationMs: 60000, PulseType: TotemPulseSingleTarget, PulseSpell: 3606, Radius: 20.0, PulseSecs: 2},
	58703: {SpellID: 58703, SlotID: TotemSlotFire, Entry: 2523, DurationMs: 60000, PulseType: TotemPulseSingleTarget, PulseSpell: 3606, Radius: 20.0, PulseSecs: 2},
	58704: {SpellID: 58704, SlotID: TotemSlotFire, Entry: 2523, DurationMs: 60000, PulseType: TotemPulseSingleTarget, PulseSpell: 3606, Radius: 20.0, PulseSecs: 2},

	// Magma Totem
	8190:  {SpellID: 8190, SlotID: TotemSlotFire, Entry: 5929, DurationMs: 20000, PulseType: TotemPulseAoEDamage, PulseSpell: 8187, Radius: 8.0, PulseSecs: 2},
	10585: {SpellID: 10585, SlotID: TotemSlotFire, Entry: 5929, DurationMs: 20000, PulseType: TotemPulseAoEDamage, PulseSpell: 8187, Radius: 8.0, PulseSecs: 2},
	10586: {SpellID: 10586, SlotID: TotemSlotFire, Entry: 5929, DurationMs: 20000, PulseType: TotemPulseAoEDamage, PulseSpell: 8187, Radius: 8.0, PulseSecs: 2},
	10587: {SpellID: 10587, SlotID: TotemSlotFire, Entry: 5929, DurationMs: 20000, PulseType: TotemPulseAoEDamage, PulseSpell: 8187, Radius: 8.0, PulseSecs: 2},
	25552: {SpellID: 25552, SlotID: TotemSlotFire, Entry: 5929, DurationMs: 20000, PulseType: TotemPulseAoEDamage, PulseSpell: 8187, Radius: 8.0, PulseSecs: 2},
	58731: {SpellID: 58731, SlotID: TotemSlotFire, Entry: 5929, DurationMs: 20000, PulseType: TotemPulseAoEDamage, PulseSpell: 8187, Radius: 8.0, PulseSecs: 2},
	58734: {SpellID: 58734, SlotID: TotemSlotFire, Entry: 5929, DurationMs: 20000, PulseType: TotemPulseAoEDamage, PulseSpell: 8187, Radius: 8.0, PulseSecs: 2},

	// Flametongue Totem
	8227:  {SpellID: 8227, SlotID: TotemSlotFire, Entry: 5950, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 52109, Radius: 30.0, PulseSecs: 2},
	8249:  {SpellID: 8249, SlotID: TotemSlotFire, Entry: 5950, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 52109, Radius: 30.0, PulseSecs: 2},
	10526: {SpellID: 10526, SlotID: TotemSlotFire, Entry: 5950, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 52109, Radius: 30.0, PulseSecs: 2},
	16387: {SpellID: 16387, SlotID: TotemSlotFire, Entry: 5950, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 52109, Radius: 30.0, PulseSecs: 2},
	25557: {SpellID: 25557, SlotID: TotemSlotFire, Entry: 5950, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 52109, Radius: 30.0, PulseSecs: 2},
	58656: {SpellID: 58656, SlotID: TotemSlotFire, Entry: 5950, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 52109, Radius: 30.0, PulseSecs: 2},

	// Fire Elemental Totem
	2894: {SpellID: 2894, SlotID: TotemSlotFire, Entry: 15439, DurationMs: 120000, PulseType: TotemPulseNone, Radius: 0, PulseSecs: 0},

	// Frost Resistance Totem
	8181: {SpellID: 8181, SlotID: TotemSlotFire, Entry: 5927, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 8182, Radius: 30.0, PulseSecs: 2},
	// Frost Resistance ranks 2-6 (trainer reqSpell chain 8181->10478->10479->25560->58741->58745, levels 38/54/67/75/80); entry + buff follow the shared rank-1 convention of the Stoneskin/Strength multi-rank defs, no per-rank creature evidence in-tree.
	10478: {SpellID: 10478, SlotID: TotemSlotFire, Entry: 5927, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 8182, Radius: 30.0, PulseSecs: 2},
	10479: {SpellID: 10479, SlotID: TotemSlotFire, Entry: 5927, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 8182, Radius: 30.0, PulseSecs: 2},
	25560: {SpellID: 25560, SlotID: TotemSlotFire, Entry: 5927, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 8182, Radius: 30.0, PulseSecs: 2},
	58741: {SpellID: 58741, SlotID: TotemSlotFire, Entry: 5927, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 8182, Radius: 30.0, PulseSecs: 2},
	58745: {SpellID: 58745, SlotID: TotemSlotFire, Entry: 5927, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 8182, Radius: 30.0, PulseSecs: 2},

	// Totem of Wrath (Fire slot per glyph script m_SummonSlot[1];
	// aura radius 40 per the aura pages; aura IDs cross-checked against
	// bot_shaman_ai.cpp commented ranks 57658/57660/57662/57663)
	30706: {SpellID: 30706, SlotID: TotemSlotFire, Entry: 17539, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 57658, DebuffSpell: 30708, Radius: 40.0, PulseSecs: 2},
	57720: {SpellID: 57720, SlotID: TotemSlotFire, Entry: 30652, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 57660, DebuffSpell: 30708, Radius: 40.0, PulseSecs: 2},
	57721: {SpellID: 57721, SlotID: TotemSlotFire, Entry: 30653, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 57662, DebuffSpell: 30708, Radius: 40.0, PulseSecs: 2},
	57722: {SpellID: 57722, SlotID: TotemSlotFire, Entry: 30654, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 57663, DebuffSpell: 30708, Radius: 40.0, PulseSecs: 2},

	// --- Water Totems (Slot 2) ---
	// Healing Stream Totem
	5394:  {SpellID: 5394, SlotID: TotemSlotWater, Entry: 3572, DurationMs: 300000, PulseType: TotemPulseHeal, PulseSpell: 52042, Radius: 30.0, PulseSecs: 2},
	6375:  {SpellID: 6375, SlotID: TotemSlotWater, Entry: 3572, DurationMs: 300000, PulseType: TotemPulseHeal, PulseSpell: 52042, Radius: 30.0, PulseSecs: 2},
	6377:  {SpellID: 6377, SlotID: TotemSlotWater, Entry: 3572, DurationMs: 300000, PulseType: TotemPulseHeal, PulseSpell: 52042, Radius: 30.0, PulseSecs: 2},
	10462: {SpellID: 10462, SlotID: TotemSlotWater, Entry: 3572, DurationMs: 300000, PulseType: TotemPulseHeal, PulseSpell: 52042, Radius: 30.0, PulseSecs: 2},
	10463: {SpellID: 10463, SlotID: TotemSlotWater, Entry: 3572, DurationMs: 300000, PulseType: TotemPulseHeal, PulseSpell: 52042, Radius: 30.0, PulseSecs: 2},
	25567: {SpellID: 25567, SlotID: TotemSlotWater, Entry: 3572, DurationMs: 300000, PulseType: TotemPulseHeal, PulseSpell: 52042, Radius: 30.0, PulseSecs: 2},
	58755: {SpellID: 58755, SlotID: TotemSlotWater, Entry: 3572, DurationMs: 300000, PulseType: TotemPulseHeal, PulseSpell: 52042, Radius: 30.0, PulseSecs: 2},
	58756: {SpellID: 58756, SlotID: TotemSlotWater, Entry: 3572, DurationMs: 300000, PulseType: TotemPulseHeal, PulseSpell: 52042, Radius: 30.0, PulseSecs: 2},
	58757: {SpellID: 58757, SlotID: TotemSlotWater, Entry: 3572, DurationMs: 300000, PulseType: TotemPulseHeal, PulseSpell: 52042, Radius: 30.0, PulseSecs: 2},

	// Mana Spring Totem
	5675:  {SpellID: 5675, SlotID: TotemSlotWater, Entry: 3571, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 5677, Radius: 30.0, PulseSecs: 2},
	10495: {SpellID: 10495, SlotID: TotemSlotWater, Entry: 3571, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 5677, Radius: 30.0, PulseSecs: 2},
	10496: {SpellID: 10496, SlotID: TotemSlotWater, Entry: 3571, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 5677, Radius: 30.0, PulseSecs: 2},
	10497: {SpellID: 10497, SlotID: TotemSlotWater, Entry: 3571, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 5677, Radius: 30.0, PulseSecs: 2},
	25570: {SpellID: 25570, SlotID: TotemSlotWater, Entry: 3571, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 5677, Radius: 30.0, PulseSecs: 2},
	58771: {SpellID: 58771, SlotID: TotemSlotWater, Entry: 3571, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 5677, Radius: 30.0, PulseSecs: 2},
	58773: {SpellID: 58773, SlotID: TotemSlotWater, Entry: 3571, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 5677, Radius: 30.0, PulseSecs: 2},
	58774: {SpellID: 58774, SlotID: TotemSlotWater, Entry: 3571, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 5677, Radius: 30.0, PulseSecs: 2},

	// Cleansing Totem
	8170: {SpellID: 8170, SlotID: TotemSlotWater, Entry: 5925, DurationMs: 300000, PulseType: TotemPulseDispel, Radius: 30.0, PulseSecs: 3},

	// Mana Tide Totem
	16190: {SpellID: 16190, SlotID: TotemSlotWater, Entry: 10467, DurationMs: 12000, PulseType: TotemPulseBuff, BuffSpell: 16178, Radius: 30.0, PulseSecs: 3},

	// Fire Resistance Totem
	8184: {SpellID: 8184, SlotID: TotemSlotWater, Entry: 5928, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 8185, Radius: 30.0, PulseSecs: 2},
	// Fire Resistance ranks 2-6 (trainer reqSpell chain 8184->10537->10538->25563->58737->58739, levels 42/58/68/75/80); shared rank-1 entry + buff, same convention as above.
	10537: {SpellID: 10537, SlotID: TotemSlotWater, Entry: 5928, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 8185, Radius: 30.0, PulseSecs: 2},
	10538: {SpellID: 10538, SlotID: TotemSlotWater, Entry: 5928, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 8185, Radius: 30.0, PulseSecs: 2},
	25563: {SpellID: 25563, SlotID: TotemSlotWater, Entry: 5928, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 8185, Radius: 30.0, PulseSecs: 2},
	58737: {SpellID: 58737, SlotID: TotemSlotWater, Entry: 5928, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 8185, Radius: 30.0, PulseSecs: 2},
	58739: {SpellID: 58739, SlotID: TotemSlotWater, Entry: 5928, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 8185, Radius: 30.0, PulseSecs: 2},

	// --- Air Totems (Slot 3) ---
	// Windfury Totem
	8512:  {SpellID: 8512, SlotID: TotemSlotAir, Entry: 6112, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 8515, Radius: 30.0, PulseSecs: 2},
	10613: {SpellID: 10613, SlotID: TotemSlotAir, Entry: 6112, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 8515, Radius: 30.0, PulseSecs: 2},
	10614: {SpellID: 10614, SlotID: TotemSlotAir, Entry: 6112, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 8515, Radius: 30.0, PulseSecs: 2},
	25585: {SpellID: 25585, SlotID: TotemSlotAir, Entry: 6112, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 8515, Radius: 30.0, PulseSecs: 2},
	25587: {SpellID: 25587, SlotID: TotemSlotAir, Entry: 6112, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 8515, Radius: 30.0, PulseSecs: 2},

	// Wrath of Air Totem
	3738: {SpellID: 3738, SlotID: TotemSlotAir, Entry: 15447, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 2895, Radius: 30.0, PulseSecs: 2},

	// Grounding Totem
	8177: {SpellID: 8177, SlotID: TotemSlotAir, Entry: 5924, DurationMs: 45000, PulseType: TotemPulseNone, Radius: 30.0, PulseSecs: 0},

	// Nature Resistance Totem
	10595: {SpellID: 10595, SlotID: TotemSlotAir, Entry: 5926, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 10596, Radius: 30.0, PulseSecs: 2},
	// Nature Resistance ranks 2-6 (trainer reqSpell chain 10595->10600->10601->25574->58746->58749, levels 44/60/69/75/80); shared rank-1 entry + buff, same convention as above.
	10600: {SpellID: 10600, SlotID: TotemSlotAir, Entry: 5926, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 10596, Radius: 30.0, PulseSecs: 2},
	10601: {SpellID: 10601, SlotID: TotemSlotAir, Entry: 5926, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 10596, Radius: 30.0, PulseSecs: 2},
	25574: {SpellID: 25574, SlotID: TotemSlotAir, Entry: 5926, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 10596, Radius: 30.0, PulseSecs: 2},
	58746: {SpellID: 58746, SlotID: TotemSlotAir, Entry: 5926, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 10596, Radius: 30.0, PulseSecs: 2},
	58749: {SpellID: 58749, SlotID: TotemSlotAir, Entry: 5926, DurationMs: 300000, PulseType: TotemPulseBuff, BuffSpell: 10596, Radius: 30.0, PulseSecs: 2},

	// Sentry Totem (Totem::Summon sets REACT_AGGRESSIVE, Totem.cpp:103-104;
	// 5-minute duration; the ping arm scans the standard aggro radius).
	6495: {SpellID: 6495, SlotID: TotemSlotAir, Entry: 3968, DurationMs: 300000, PulseType: TotemPulseSentry, Radius: 20.0, PulseSecs: 2},
}

func isTotemSpell(spellID uint32) bool {
	_, ok := totemDefinitions[spellID]
	return ok
}

func getTotemDef(spellID uint32) (TotemDef, bool) {
	def, ok := totemDefinitions[spellID]
	return def, ok
}

func (s *Server) nextDynamicCreatureLowGUID() uint32 {
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	s.nextDynamicCreatureGUID++
	if s.nextDynamicCreatureGUID < 2000000 {
		s.nextDynamicCreatureGUID = 2000000
	}
	return s.nextDynamicCreatureGUID
}

func (s *Server) totemCreatureTargets(totem *activeTotem, radius float64) []*creatureMotion {
	if s == nil || totem == nil {
		return nil
	}
	s.motionMu.Lock()
	targets := make([]*creatureMotion, 0)
	for _, motion := range s.motionMapLocked(totem.Map, totem.InstanceID) {
		if motion != nil && motion.GUID != totem.TotemGUID && motion.Map == totem.Map && motion.InstanceID == totem.InstanceID && motion.Health > 0 && distance3D(motion.X, motion.Y, motion.Z, totem.X, totem.Y, totem.Z) <= radius {
			targets = append(targets, motion)
		}
	}
	s.motionMu.Unlock()
	return targets
}

// applyTotemEnemyDebuff registers one Totem of Wrath enemy-debuff record
// on a hostile creature motion, caster-keyed to the totem: the record feeds
// creatureAuraModifierSum for SPELL_AURA_MOD_ATTACKER_SPELL_AND_WEAPON_CRIT_CHANCE
// (197) once the DBC row resolves, mirroring the pet-aura insertion shape
// (same-spell replacement, both aura maps). No expiry timer: the pulse
// re-applies it and destroyTotemGUID sweeps it, matching the area-aura
// lifetime (it lives while the totem lives).
func (s *session) applyTotemEnemyDebuff(totem *activeTotem, m *creatureMotion, spellID uint32, ownerLevel uint8) {
	if s == nil || s.server == nil || totem == nil || m == nil || spellID == 0 {
		return
	}
	key := creatureAuraKeyForMotion(m)
	if key.GUID == 0 {
		return
	}
	s.server.auraMu.Lock()
	if s.server.activeCreatureAuras == nil {
		s.server.activeCreatureAuras = make(map[creatureAuraKey]map[uint32]*activeAura)
	}
	if s.server.activeCreatureAuras[key] == nil {
		s.server.activeCreatureAuras[key] = make(map[uint32]*activeAura)
	}
	if s.server.creatureAuras == nil {
		s.server.creatureAuras = make(map[creatureAuraKey]map[uint32]struct{})
	}
	if s.server.creatureAuras[key] == nil {
		s.server.creatureAuras[key] = make(map[uint32]struct{})
	}
	slot := uint8(len(s.server.activeCreatureAuras[key]))
	if previous := s.server.activeCreatureAuras[key][spellID]; previous != nil {
		previous.Stopped = true
		if previous.Timer != nil {
			previous.Timer.Stop()
		}
		if previous.TickTimer != nil {
			previous.TickTimer.Stop()
		}
		s.server.unregisterSingleCastAura(previous)
		slot = previous.Slot
	}
	aura := &activeAura{SpellID: spellID, AuraType: spellAuraModAttackerSpellAndWeaponCritChance, EffectMask: 1, CasterGUID: totem.TotemGUID, TargetGUID: m.GUID, TargetKey: key, Amount: 3, CasterLevel: ownerLevel, StoredCasterLevel: ownerLevel, DurationMs: totem.DurationMs, RemainingMs: totem.DurationMs, Positive: false, StackCount: 1, Slot: slot}
	aura.Amounts[0] = 3
	s.server.activeCreatureAuras[key][spellID] = aura
	s.server.creatureAuras[key][spellID] = struct{}{}
	s.server.auraMu.Unlock()
}

// totemEnemyCreatureValid is the creature half of the TARGET_CHECK_ENEMY
// verdict for totem-caster area effects (Spell.cpp:8337-8341 +
// Object.cpp:2930 IsValidAttackTarget, totem caster): the IsTotem reject
// (an enemy player's totem motion is hostile-faction but never a valid
// enemy target), the untargetable/unattackable flag set, the
// immune-to-NPC leg (the totem caster carries no PLAYER_CONTROLLED flag,
// so only that immunity leg can fire), and the faction hostility verdict
// itself. Deliberately absent, matching the session twin: GM, UBER and
// sanctuary arms need a player on one side, and visibility has no Go
// motion model. Dead motions fail here too, which the sweep inherits:
// C++ UpdateTargetMap drops dead targets (30708 allows no dead targets).
func (s *Server) totemEnemyCreatureValid(m *creatureMotion, owner playerPos) bool {
	if s == nil || m == nil || m.Health == 0 {
		return false
	}
	if s.isTotemGUID(m.GUID) {
		return false
	}
	if m.UnitFlags&(unitFlagNonAttackable|unitFlagTaxiFlight|unitFlagNotAttackable1|unitFlagNonAttackable2|unitFlagNotSelectable) != 0 {
		return false
	}
	if m.UnitFlags&unitFlagImmuneToNPC != 0 {
		return false
	}
	return s.isHostileFaction(m.Faction, owner)
}

// sweepTotemCreatureDebuffOutOfRange is the creature half of the
// Aura::UpdateTargetMap drop (SpellAuras.cpp:643-673): the C++ area-aura
// refresh rebuilds the target list each interval and removes every
// application whose target is no longer in it, and the Go 2s pulse is
// that refresh. Records are caster-keyed, so a second live totem's
// debuff on the same creature survives; a motion that is gone entirely
// is left alone, since the despawn path clears its auras itself. The
// rebuild also drops targets failing CanBeAppliedOn, which in the Go
// model is the totemEnemyCreatureValid verdict (same call as the apply arm).
func (s *session) sweepTotemCreatureDebuffOutOfRange(totem *activeTotem, spellID uint32, radius float64) {
	if s == nil || s.server == nil || totem == nil || spellID == 0 {
		return
	}
	s.server.auraMu.Lock()
	var stale []creatureAuraKey
	for key, auras := range s.server.activeCreatureAuras {
		if aura, ok := auras[spellID]; ok && aura != nil && aura.CasterGUID == totem.TotemGUID {
			stale = append(stale, key)
		}
	}
	s.server.auraMu.Unlock()
	owner := totemOwnerPos(s)
	for _, key := range stale {
		m := s.server.findCreatureMotion(key.Map, key.InstanceID, key.GUID)
		if m == nil {
			continue
		}
		if distance3D(m.X, m.Y, m.Z, totem.X, totem.Y, totem.Z) > radius || !s.server.totemEnemyCreatureValid(m, owner) {
			s.server.removeCreatureAura(key, spellID)
		}
	}
}

// sweepTotemSessionAuraOutOfRange is the session half of the same
// UpdateTargetMap drop: a session aura cast by this totem (buff or
// debuff spell) drops once the session leaves the totem's map or walks
// beyond its radius. Caster-keyed like the destroy sweep, so a second
// live totem's aura survives; the destroy sweep still covers totem
// death with no radius gate. The caller adds the CanBeAppliedOn leg
// via extraDrop: the debuff drops when the session is no longer
// hostile, the buff when it is no longer in the owner's group.
func (s *session) sweepTotemSessionAuraOutOfRange(totem *activeTotem, spellID uint32, radius float64, extraDrop func(other *session) bool) {
	if s == nil || s.server == nil || totem == nil || spellID == 0 {
		return
	}
	s.server.sessionsMu.RLock()
	var doomed []*session
	for other := range s.server.sessions {
		if other == nil || other.player == nil {
			continue
		}
		other.castMu.Lock()
		aura := other.activeAuras[spellID]
		hit := aura != nil && !aura.Stopped && aura.CasterGUID == totem.TotemGUID
		other.castMu.Unlock()
		if !hit {
			continue
		}
		if other.player.Map != totem.Map || other.player.InstanceID != totem.InstanceID ||
			distance3D(other.player.X, other.player.Y, other.player.Z, totem.X, totem.Y, totem.Z) > radius ||
			(extraDrop != nil && extraDrop(other)) {
			doomed = append(doomed, other)
		}
	}
	s.server.sessionsMu.RUnlock()
	for _, other := range doomed {
		other.removeAura(spellID)
	}
}

// totemEnemySessionHostile is the session half of the TARGET_CHECK_ENEMY
// verdict for the Totem of Wrath enemy leg (Spell.cpp:8316, Object.cpp:2977):
// the enemy sweep's player leg keys hostility on team, not on the
// creature-faction templates — opposite team, or a duel opponent even on
// the same team (isFriendlyToPlayer's duel term, Object.cpp:2744-2746) —
// then applies the player-vs-player attack terms (sanctuary, PvP flag,
// FFA) via explicitTargetAttackPvPBlocked. The GM, taxi-flight,
// non-attackable and dead gates mirror the session leg of
// spellAreaEnemyTargets. The UBER term is deliberately absent: the pulse
// caster is the totem creature, whose ToPlayer is null, so the
// player-caster UBER arm cannot fire (creature-path VERIFIED-OUT).
func (s *session) totemEnemySessionHostile(other *session) bool {
	if s == nil || other == nil || other == s || !other.authed || !other.worldReady.Load() || other.player == nil {
		return false
	}
	if other.player.Health == 0 || other.player.TaxiPath != "" || other.player.UnitFlags&unitFlagNonAttackable2 != 0 || sessionIsGameMaster(other) {
		return false
	}
	if s.isFriendlyToPlayer(other) {
		return false
	}
	duelHostile := s.duelPartner != 0 && s.duelPartner == other.playerGUID && s.player != nil && s.player.DuelTeam != 0 && other.player.DuelTeam != 0
	return !s.explicitTargetAttackPvPBlocked(other, duelHostile)
}

// totemDisplayForRace bridges the race-dependent display arm of
// Totem::InitStats (Totem.cpp:88-90): sObjectMgr->GetModelForTotem
// (ObjectMgr.cpp:1747-1753) reads player_totem_model keyed by
// (SummonSlot, race) (SharedDefines.h:3325-3334); with no row the totem
// keeps its template model ("Set to default", Totem.cpp:90-92). The Go
// slot ids are Earth=0/Fire=1/Water=2/Air=3; the C++ SummonSlot ids are
// Fire=1/Earth=2/Water=3/Air=4.
func (s *session) totemDisplayForRace(ctx context.Context, slot uint8, entry uint32) uint32 {
	var model int64
	if s != nil && s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(NULLIF(modelid1, 0), 1) FROM creature_template WHERE entry = ?", entry).Scan(&model)
		var cppSlot uint8
		switch slot {
		case TotemSlotFire:
			cppSlot = 1
		case TotemSlotEarth:
			cppSlot = 2
		case TotemSlotWater:
			cppSlot = 3
		case TotemSlotAir:
			cppSlot = 4
		}
		if cppSlot != 0 && s.player != nil {
			var display int64
			if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT DisplayId FROM player_totem_model WHERE TotemSlot = ? AND RaceId = ?", cppSlot, s.player.Race).Scan(&display); err == nil && display > 0 {
				model = display
			}
		}
	}
	if model <= 0 {
		model = 1
	}
	return uint32(model)
}

// totemTemplateFaction mirrors Creature::InitStats' template faction for
// the totem entry; totems not in creature_template inherit the owner's
// race faction (pets.go pattern).
func (s *session) totemTemplateFaction(ctx context.Context, entry uint32) uint32 {
	faction := uint32(0)
	if s != nil && s.server != nil {
		faction = s.server.raceFaction(s.player.Race)
		if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
			var templateFaction int64
			if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(faction, 0) FROM creature_template WHERE entry = ?", entry).Scan(&templateFaction); err == nil && templateFaction > 0 {
				faction = uint32(templateFaction)
			}
		}
	}
	return faction
}

// totemTemplateSpeeds mirrors the template walk/run rates carried into
// buildCreatureUpdate (Object.cpp:337-345 order).
func (s *session) totemTemplateSpeeds(ctx context.Context, entry uint32) (float32, float32) {
	walk, run := float32(1.0), float32(1.0)
	if s != nil && s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		var walkSpeed, runSpeed float64
		if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(speed_walk, 1), COALESCE(speed_run, 1) FROM creature_template WHERE entry = ?", entry).Scan(&walkSpeed, &runSpeed); err == nil {
			walk, run = float32(walkSpeed), float32(runSpeed)
		}
	}
	return walk, run
}

func (s *session) summonTotem(ctx context.Context, spellID uint32) {
	if s == nil || s.player == nil {
		return
	}
	def, ok := getTotemDef(spellID)
	if !ok {
		return
	}

	slot := def.SlotID
	if slot >= 4 {
		return
	}

	// Destroy existing totem in this slot
	s.destroyTotem(slot)

	var nextLow uint32
	if s.server != nil {
		nextLow = s.server.nextDynamicCreatureLowGUID()
	} else {
		nextLow = 2000001
	}
	totemGUID := creatureWorldGUID(nextLow, def.Entry)

	s.player.TotemSlots[slot] = totemGUID

	// Spell::EffectSummonType's TOTEM/LIGHTWELL arm (SpellEffects.cpp:2319-2342)
	// falls through to the tail that logs the summon via
	// ExecuteLogEffectSummonObject (SpellEffects.cpp:2400-2403).
	s.sendSummonObjectLog(spellID, spellEffectSummon, totemGUID)

	// Send SMSG_TOTEM_CREATED: Slot (u8), packed totem GUID, Duration
	// (u32), creating SpellID (u32) — TotemPackets.cpp:27-30 writes the
	// GUID packed (ObjectGuid), not as a fixed u64.
	resp := protocol.NewBuffer(17)
	resp.WriteU8(slot)
	resp.WritePackedGUID(totemGUID)
	resp.WriteU32(def.DurationMs)
	resp.WriteU32(def.SpellID)
	_ = s.write(uint16(protocol.OpcodeSMSG_TOTEM_CREATED), resp.Bytes(), true)

	totemX, totemY, totemZ := s.player.X, s.player.Y, s.player.Z
	if s.server != nil && s.server.Data != nil {
		if spell, ok, err := s.server.Data.Spell(spellID); err == nil && ok {
			if targetType, radiusIndex, found := totemSpellDestTarget(spell); found {
				combatReach := s.player.CombatReach
				if combatReach <= 0 {
					combatReach = 1.5
				}
				totemX, totemY, totemZ = resolveCasterDestPosition(s.server.Data, targetType, radiusIndex, uint32(s.player.Level), s.player.X, s.player.Y, s.player.Z, s.player.Orientation, combatReach, func(r float32) float32 {
					// SpellInfo.cpp:551-574: CalcRadius(m_caster) folds SPELLMOD_RADIUS
					// through the player's spell-mod owner.
					return float32(s.applySpellModFloat(spell, spellModRadius, float64(r)))
				})
			}
		}
	}

	totem := &activeTotem{
		SlotID:      slot,
		SpellID:     def.SpellID,
		TotemGUID:   totemGUID,
		Entry:       def.Entry,
		OwnerGUID:   s.playerGUID,
		Map:         s.player.Map,
		InstanceID:  s.player.InstanceID,
		X:           totemX,
		Y:           totemY,
		Z:           totemZ,
		DurationMs:  def.DurationMs,
		BuffSpell:   def.BuffSpell,
		DebuffSpell: def.DebuffSpell,
		CreatedAt:   time.Now(),
		StopChan:    make(chan struct{}),
	}

	if s.server != nil {
		s.server.totemMu.Lock()
		if s.server.activeTotems == nil {
			s.server.activeTotems = make(map[uint64][4]*activeTotem)
		}
		current := s.server.activeTotems[s.playerGUID]
		current[slot] = totem
		s.server.activeTotems[s.playerGUID] = current
		s.server.totemMu.Unlock()

		// Spell::EffectSummonType's TOTEM arm (SpellEffects.cpp:2326-2341):
		// Mana Tide Totem (16190) spawns with health at 10% of the caster's
		// max health instead of the full effect damage.
		totemHealth := uint32(100)
		if spellID == 16190 && s.player.MaxHealth > 0 {
			if pct := s.player.MaxHealth / 10; pct > 0 {
				totemHealth = pct
			}
		}
		// Register creature in creatureMotion
		s.server.motionMu.Lock()
		s.server.motionMapLocked(s.player.Map, s.player.InstanceID)[totemGUID] = &creatureMotion{
			GUID:       totemGUID,
			Entry:      def.Entry,
			Map:        s.player.Map,
			InstanceID: s.player.InstanceID,
			X:          totemX,
			Y:          totemY,
			Z:          totemZ,
			Health:     totemHealth,
			MaxHealth:  totemHealth,
			// Totem::Summon (Totem.cpp:99): the totem's level is the owner's.
			Level: maxUint32(uint32(s.player.Level), 1),
		}
		s.server.motionMu.Unlock()

		// Sentry Totem (Totem.cpp:103-104): Totem::InitStats sets
		// REACT_AGGRESSIVE for SENTRY_TOTEM_ENTRY.
		if def.PulseType == TotemPulseSentry {
			s.server.motionMu.Lock()
			if m := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, totemGUID); m != nil {
				m.ReactState = creatureReactAggressive
				m.ReactStateKnown = true
			}
			s.server.motionMu.Unlock()
		}

		// Totem::InitStats (Totem.cpp:72-105): after SMSG_TOTEM_CREATED the
		// totem materializes as a visible creature via Creature::AddToWorld.
		// The Go twin sends the create block to the owner and nearby
		// players (pets.go pattern); the display id is race-dependent via
		// ObjectMgr::GetModelForTotem (Totem.cpp:88-90), the level is the
		// owner's (Totem.cpp:99).
		spawn := creatureSpawn{
			GUID:           uint32(nextLow),
			RawGUID:        totemGUID,
			Entry:          def.Entry,
			Map:            s.player.Map,
			X:              totemX,
			Y:              totemY,
			Z:              totemZ,
			Orientation:    s.player.Orientation,
			Model:          s.totemDisplayForRace(ctx, slot, def.Entry),
			Faction:        s.totemTemplateFaction(ctx, def.Entry),
			Level:          maxUint32(uint32(s.player.Level), 1),
			Health:         totemHealth,
			MaxHealth:      totemHealth,
			CreatedBySpell: def.SpellID,
		}
		stats := s.server.loadCreatureStats(ctx, def.Entry)
		spawn.UnitFlags = stats.UnitFlags
		spawn.BoundingRadius = stats.BoundingRadius
		spawn.CombatReach = stats.CombatReach
		spawn.WalkSpeed, spawn.RunSpeed = s.totemTemplateSpeeds(ctx, def.Entry)
		updates := protocol.NewUpdateData()
		updates.AddUpdateBlock(buildCreatureUpdate(ctx, spawn, s))
		if packet, err := updates.BuildPacket(0); err == nil && packet != nil {
			_ = s.write(packet.Opcode, packet.Payload.Bytes(), true)
			s.server.broadcastToNearby(packet.Opcode, packet.Payload.Bytes(), s)
		}
	}

	// Apply immediate initial pulse
	s.executeTotemPulse(ctx, def, totem)

	// Glyph of Totem of Wrath (spell_shaman.cpp
	// spell_sha_glyph_of_totem_of_wrath): the glyph aura (63280) procs on
	// a totem cast whose spell icon is Totem of Wrath (2019) — the four
	// ToW summon defs — and casts the spell-power buff (63283) on the
	// caster with BP0/BP1 = pct of the totem aura spell's effect 0/1
	// values (CalculatePct, Util.h:66). The totem must exist (fire slot
	// creature), so the proc rides the completed summon; a missing DBC
	// row for the aura spell skips it (data gate, inert against the
	// shipped DBs).
	if spellID == 30706 || spellID == 57720 || spellID == 57721 || spellID == 57722 {
		s.castMu.Lock()
		glyph, ok := s.activeAuras[63280]
		var glyphAmount int32
		var glyphLive bool
		if ok && glyph != nil {
			glyphLive = !glyph.Stopped
			glyphAmount = glyph.Amounts[0]
		}
		s.castMu.Unlock()
		if glyphLive && glyphAmount > 0 && s.server != nil && s.server.Data != nil && def.BuffSpell > 0 {
			if auraSpell, found, err := s.server.Data.Spell(def.BuffSpell); err == nil && found && len(auraSpell.Effects) >= 2 {
				bp0 := int32(int64(auraSpell.Effects[0].CalcValueForLevel(auraSpell, uint32(s.player.Level))) * int64(glyphAmount) / 100)
				bp1 := int32(int64(auraSpell.Effects[1].CalcValueForLevel(auraSpell, uint32(s.player.Level))) * int64(glyphAmount) / 100)
				s.castSpellDirectWithBasePoints(ctx, 63283, s.playerGUID, []int32{bp0, bp1})
			}
		}
	}

	// Launch pulse / lifecycle loop
	go s.runTotemLifecycle(def, totem)
}

// sessionNoSlotTotemEffects returns the spell's SPELL_EFFECT_SUMMON effects
// that C++ routes through the TOTEM arm (SpellEffects.cpp:2284-2342) but
// that the slotted summonTotem path does not cover: Title TOTEM, Control
// WILD/ALLY/UNK, no Guardian flag (512), and a SummonProperties Slot
// outside the C++ totem-slot range 1..4 (SharedDefines.h:3324-3334).
// Slotted spells missing from totemDefinitions are NOT returned here:
// C++ occupies the owner's summon slot and sends SMSG_TOTEM_CREATED for
// them (Totem::InitStats, Totem.cpp:78-84), which is a missing-definitions
// data gap, not a no-slot producer gap. Lightwell (Title 11) stays out
// per the standing no-bridge note.
func (s *session) sessionNoSlotTotemEffects(spell wotlk.Spell) []wotlk.SpellEffect {
	if s == nil || s.server == nil || s.server.Data == nil || isTotemSpell(spell.ID) {
		return nil
	}
	var out []wotlk.SpellEffect
	for _, eff := range spell.Effects {
		if eff.Effect != spellEffectSummon || eff.MiscValue == 0 {
			continue
		}
		props, found, err := s.server.Data.SummonProperties(uint32(eff.MiscValueB))
		if err != nil || !found {
			continue
		}
		switch props.Control {
		case summonCategoryWild, summonCategoryAlly, summonCategoryUnk:
		default:
			continue
		}
		if props.Flags&512 != 0 || props.Title != summonTitleTotem {
			continue
		}
		if props.Slot >= 1 && props.Slot <= 4 {
			continue
		}
		out = append(out, eff)
	}
	return out
}

// summonSessionNoSlotTotem mirrors the TOTEM arm of
// Spell::EffectSummonType (SpellEffects.cpp:2319-2342) for a player caster
// whose spell carries no C++ totem slot (e.g. a death knight's Anti-Magic
// Zone, properties 647): Map::SummonCreature still creates a Totem object
// (Object.cpp:1917-1939) even though Totem::InitStats sends no
// SMSG_TOTEM_CREATED and occupies no summon slot for it (Totem.cpp:78-84,
// slot-range gate). So, unlike summonTotem, this producer touches neither
// TotemSlots nor activeTotems: the motion is the totem, the gate set
// (registerCreatureTotemGUID) is its only registry, and a second cast
// does not destroy the first. Minion::InitStats
// (TemporarySummon.cpp:350-356) stamps creator + owner faction; the level
// is the owner's (Totem.cpp:99); the health is the effect's damage value
// when positive (SpellEffects.cpp:2335-2339), else the slotted path's
// flat 100. A zero/unknown duration means C++ unsummons on the totem's
// first Update (Totem.cpp:44-56), so only the summon log is sent (the
// InitSummon passive cast has no model on this path). Positive durations
// fold SPELLMOD_DURATION (SpellEffects.cpp:2214-2215) and arm the expiry
// in expireSessionNoSlotTotem.
func (s *session) summonSessionNoSlotTotem(ctx context.Context, spell wotlk.Spell, eff wotlk.SpellEffect) {
	if s == nil || s.player == nil || s.server == nil {
		return
	}
	entry := uint32(eff.MiscValue)
	if entry == 0 {
		return
	}
	durationMs := int32(0)
	if spell.DurationIndex > 0 && s.server.Data != nil {
		if dur, ok, err := s.server.Data.SpellDuration(spell.DurationIndex, uint32(s.player.Level)); err == nil && ok && dur > 0 {
			durationMs = dur
		}
	}
	if modded := s.applySpellMod(spell, spellModDuration, durationMs); modded > 0 {
		durationMs = modded
	}
	nextLow := s.server.nextDynamicCreatureLowGUID()
	totemGUID := creatureWorldGUID(nextLow, entry)
	if durationMs <= 0 {
		s.sendSummonObjectLog(spell.ID, spellEffectSummon, totemGUID)
		return
	}
	totemX, totemY, totemZ := s.player.X, s.player.Y, s.player.Z
	if targetType, radiusIndex, found := totemSpellDestTarget(spell); found {
		combatReach := s.player.CombatReach
		if combatReach <= 0 {
			combatReach = 1.5
		}
		totemX, totemY, totemZ = resolveCasterDestPosition(s.server.Data, targetType, radiusIndex, uint32(s.player.Level), s.player.X, s.player.Y, s.player.Z, s.player.Orientation, combatReach, func(r float32) float32 {
			return float32(s.applySpellModFloat(spell, spellModRadius, float64(r)))
		})
	}
	totemHealth := uint32(100)
	if v := eff.CalcValueForLevel(spell, uint32(s.player.Level)); v > 0 {
		totemHealth = uint32(v)
	}
	totemFaction := s.totemTemplateFaction(ctx, entry)
	s.server.motionMu.Lock()
	s.server.motionMapLocked(s.player.Map, s.player.InstanceID)[totemGUID] = &creatureMotion{
		GUID:        totemGUID,
		Entry:       entry,
		Map:         s.player.Map,
		InstanceID:  s.player.InstanceID,
		HomeX:       totemX,
		HomeY:       totemY,
		HomeZ:       totemZ,
		HomeO:       s.player.Orientation,
		X:           totemX,
		Y:           totemY,
		Z:           totemZ,
		Orientation: s.player.Orientation,
		Health:      totemHealth,
		MaxHealth:   totemHealth,
		Level:       maxUint32(uint32(s.player.Level), 1),
		Faction:     totemFaction,
		OwnerGUID:   s.playerGUID,
	}
	s.server.motionMu.Unlock()
	s.server.registerCreatureTotemGUID(totemGUID)
	s.sendSummonObjectLog(spell.ID, spellEffectSummon, totemGUID)
	// Totem::InitStats (Totem.cpp:72-105) materializes the totem as a
	// visible creature via Creature::AddToWorld; the Go twin sends the
	// create block to the owner and nearby players (summonTotem
	// pattern). Slot 255 maps to no C++ totem slot in
	// totemDisplayForRace, so the race-model lookup is skipped and the
	// template model stands — matching GetModelForTotem's miss path for
	// a slot outside the totem range (Totem.cpp:88-92).
	spawn := creatureSpawn{
		GUID:           nextLow,
		RawGUID:        totemGUID,
		Entry:          entry,
		Map:            s.player.Map,
		X:              totemX,
		Y:              totemY,
		Z:              totemZ,
		Orientation:    s.player.Orientation,
		Model:          s.totemDisplayForRace(ctx, 255, entry),
		Faction:        s.totemTemplateFaction(ctx, entry),
		Level:          maxUint32(uint32(s.player.Level), 1),
		Health:         totemHealth,
		MaxHealth:      totemHealth,
		CreatedBySpell: spell.ID,
	}
	stats := s.server.loadCreatureStats(ctx, entry)
	spawn.UnitFlags = stats.UnitFlags
	spawn.BoundingRadius = stats.BoundingRadius
	spawn.CombatReach = stats.CombatReach
	spawn.WalkSpeed, spawn.RunSpeed = s.totemTemplateSpeeds(ctx, entry)
	updates := protocol.NewUpdateData()
	updates.AddUpdateBlock(buildCreatureUpdate(ctx, spawn, s))
	if packet, err := updates.BuildPacket(0); err == nil && packet != nil {
		_ = s.write(packet.Opcode, packet.Payload.Bytes(), true)
		s.server.broadcastToNearby(packet.Opcode, packet.Payload.Bytes(), s)
	}
	srv, mapID, instanceID := s.server, s.player.Map, s.player.InstanceID
	go func() {
		time.Sleep(time.Duration(durationMs) * time.Millisecond)
		srv.expireSessionNoSlotTotem(mapID, instanceID, totemGUID)
	}()
}

// expireSessionNoSlotTotem is the Totem::UnSummon twin (Totem.cpp:122-162)
// for no-slot player totems: the motion is removed, the gate-set entry
// dropped, the totem's creature auras cleared, and SMSG_DESTROY_OBJECT
// broadcast — mirroring expireTempSummonGuardian's teardown order
// (motion delete, then gate unregister). Server-side only, so it runs
// even after the owner's session is gone.
func (s *Server) expireSessionNoSlotTotem(mapID, instanceID uint32, guid uint64) {
	if s == nil || guid == 0 {
		return
	}
	s.motionMu.Lock()
	if s.findCreatureMotionLocked(mapID, instanceID, guid) == nil {
		s.motionMu.Unlock()
		return
	}
	delete(s.motionMapLocked(mapID, instanceID), guid)
	s.motionMu.Unlock()
	s.unregisterCreatureTotemGUID(guid)
	s.clearCreatureAuras(creatureAuraKey{Map: mapID, InstanceID: instanceID, GUID: guid})
	destroyBuf := protocol.NewBuffer(9)
	destroyBuf.WriteU64(guid)
	destroyBuf.WriteU8(0)
	s.broadcastToInstance(mapID, instanceID, uint16(protocol.OpcodeSMSG_DESTROY_OBJECT), destroyBuf.Bytes(), nil)
}

func (s *session) executeTotemPulse(ctx context.Context, def TotemDef, totem *activeTotem) {
	if s == nil || s.player == nil || totem == nil {
		return
	}

	switch def.PulseType {
	case TotemPulseBuff:
		if def.BuffSpell > 0 {
			// Totem::InitSummon casts the buff with the totem as caster
			// (Totem.cpp:115), so the aura record and wire carry the totem
			// GUID and the totem's (owner's) level — not the owner's session
			// GUID. spellModOwnerSession resolves the totem back to the
			// owner for mod lookups, like C++ GetSpellModOwner.
			var ownerLevel uint8
			if s.player != nil {
				ownerLevel = s.player.Level
			}
			// The OWNER arm of FillTargetMap checks IsWithinDistInMap
			// (SpellAuras.cpp FillTargetMap), so the owner's own buff is
			// radius-gated like every other area-aura application: an
			// owner who walks out of range loses it to the sweep below
			// instead of being re-applied unconditionally each pulse.
			if s.player.Map == totem.Map && s.player.InstanceID == totem.InstanceID &&
				distance3D(s.player.X, s.player.Y, s.player.Z, totem.X, totem.Y, totem.Z) <= float64(def.Radius) {
				s.applyAuraFromCaster(def.BuffSpell, 1800000, totem.TotemGUID, ownerLevel)
			}
			// Apply to nearby party members
			if s.server != nil && s.groupID != 0 {
				s.server.sessionsMu.RLock()
				for other := range s.server.sessions {
					if other != s && other.worldReady.Load() && other.player != nil && other.groupID == s.groupID && other.player.Map == totem.Map && other.player.InstanceID == totem.InstanceID {
						if distance3D(other.player.X, other.player.Y, other.player.Z, totem.X, totem.Y, totem.Z) <= float64(def.Radius) {
							other.applyAuraFromCaster(def.BuffSpell, 1800000, totem.TotemGUID, ownerLevel)
						}
					}
				}
				s.server.sessionsMu.RUnlock()
			}
		}
		// Totem of Wrath enemy leg: Totem::InitSummon casts the totem's
		// second template spell on itself (Totem.cpp:118-120), and spell
		// 30708 is the WotLK enemy debuff (chance to be critically hit
		// +3%, 40yd). The C++ area-aura model applies it to enemies in
		// radius while the totem lives, so each pulse re-applies it to
		// hostiles in radius (covering enemies that enter later), and
		// destroyTotemGUID sweeps it when the totem dies.
		if def.DebuffSpell > 0 && s.server != nil {
			var ownerLevel uint8
			if s.player != nil {
				ownerLevel = s.player.Level
			}
			owner := totemOwnerPos(s)
			for _, m := range s.server.totemCreatureTargets(totem, float64(def.Radius)) {
				if !s.server.totemEnemyCreatureValid(m, owner) {
					continue
				}
				s.applyTotemEnemyDebuff(totem, m, def.DebuffSpell, ownerLevel)
			}
			// Enemy-player leg: the same area aura reaches hostile
			// sessions in radius, with no group term (the buff arm's
			// group gate is party-only). applyAuraFromCaster carries
			// the totem GUID, so the destroy sweep below can key on
			// it; the 2s pulse re-applies, refreshing the duration
			// timer the same way the creature record is refreshed.
			// The session victim's crit-taken fold reads this aura
			// via playerAuraModifier(197) (combat.go), the session
			// twin of the creature record's creatureAuraModifierSum.
			if s.server != nil {
				s.server.sessionsMu.RLock()
				for other := range s.server.sessions {
					if other != nil && other.player != nil && other.player.Map == totem.Map && other.player.InstanceID == totem.InstanceID &&
						distance3D(other.player.X, other.player.Y, other.player.Z, totem.X, totem.Y, totem.Z) <= float64(def.Radius) &&
						s.totemEnemySessionHostile(other) {
						other.applyAuraFromCaster(def.DebuffSpell, totem.DurationMs, totem.TotemGUID, ownerLevel)
					}
				}
				s.server.sessionsMu.RUnlock()
			}
		}
		// UpdateTargetMap drop (SpellAuras.cpp:643): each pulse rebuilds
		// the area target list, so applications whose target left the
		// radius drop here instead of lingering until the totem dies.
		// The sweeps run after the apply arms above, which only touch
		// in-radius targets, so apply and sweep never fight.
		if def.DebuffSpell > 0 {
			s.sweepTotemCreatureDebuffOutOfRange(totem, def.DebuffSpell, float64(def.Radius))
			s.sweepTotemSessionAuraOutOfRange(totem, def.DebuffSpell, float64(def.Radius), func(other *session) bool {
				return !s.totemEnemySessionHostile(other)
			})
		}
		if def.BuffSpell > 0 {
			s.sweepTotemSessionAuraOutOfRange(totem, def.BuffSpell, float64(def.Radius), func(other *session) bool {
				return other != s && !(s.groupID != 0 && other.groupID == s.groupID)
			})
		}

	case TotemPulseHeal:
		// AuraEffect::HandlePeriodicHealAuraTick (SpellAuraEffects.cpp:5375-5440)
		// twin: Healing Stream's totem spell (52042) is a passive periodic-heal
		// aura (Totem.cpp:115-116 triggered cast, aura caster = the totem), so
		// each tick is a periodic heal, not a direct heal. The old arm ran the
		// direct-heal path with the owner as healer (SMSG_SPELLHEALLOG, owner
		// spell power, direct-heal procs, owner threat) — every one of those
		// legs differs from C++.
		s.totemHealPulseTick(ctx, def, totem, s)
		if s.server != nil && s.groupID != 0 {
			s.server.sessionsMu.RLock()
			for other := range s.server.sessions {
				if other != s && other.worldReady.Load() && other.player != nil && other.groupID == s.groupID && other.player.Map == totem.Map && other.player.InstanceID == totem.InstanceID {
					if distance3D(other.player.X, other.player.Y, other.player.Z, totem.X, totem.Y, totem.Z) <= float64(def.Radius) {
						s.totemHealPulseTick(ctx, def, totem, other)
					}
				}
			}
			s.server.sessionsMu.RUnlock()
		}

	case TotemPulseAoEDamage:
		if s.server != nil {
			// C++ twin: Totem::InitSummon triggered-casts the passive aura
			// (Totem.cpp:115-117); its periodic ticks fire proc events with
			// PROC_FLAG_DONE_PERIODIC/TAKEN_PERIODIC
			// (SpellAuraEffects.cpp:5224), so no DISALLOW_PROC_EVENTS arm.
			// The aura's damage effect targets enemies only, mirrored by
			// the hostile filter below.
			owner := totemOwnerPos(s)
			dmg := uint32(75)
			for _, m := range s.server.totemCreatureTargets(totem, float64(def.Radius)) {
				if !s.server.totemEnemyCreatureValid(m, owner) {
					continue
				}
				s.executeSpellDamage(ctx, m.GUID, def.PulseSpell, dmg, 0)
			}
		}

	case TotemPulseSingleTarget:
		if s.server != nil {
			// C++ twin: TotemAI::UpdateAI (TotemAI.cpp:53-82) — plain
			// me->CastSpell(victim, GetSpell()), non-triggered, so proc
			// events fire (no DISALLOW_PROC_EVENTS arm). The victim is
			// remembered in _victimGUID and reused while still attackable,
			// in range, and hostile; only then does a new
			// nearest-attackable-unit search run (TotemAI.cpp:66-76).
			owner := totemOwnerPos(s)
			var victimGUID uint64
			totem.mu.Lock()
			remembered := totem.LastVictimGUID
			totem.mu.Unlock()
			if remembered != 0 && remembered != totem.TotemGUID {
				// Remembered-victim revalidation (TotemAI.cpp:66): the
				// victim drops when no longer targetable, out of range,
				// or friendly. The creature leg is the
				// totemEnemyCreatureValid verdict (IsTotem/flag/immune
				// gates + faction hostility), not bare faction: a
				// flagged or immune creature, or another totem, was
				// kept as victim until it died or walked out.
				if m := s.server.findCreatureMotion(totem.Map, totem.InstanceID, remembered); m != nil &&
					distance3D(m.X, m.Y, m.Z, totem.X, totem.Y, totem.Z) <= float64(def.Radius) &&
					s.server.totemEnemyCreatureValid(m, owner) {
					victimGUID = m.GUID
				} else if other := s.server.findSessionByGUID(remembered); other != nil && other.player != nil &&
					other.player.Map == totem.Map && other.player.InstanceID == totem.InstanceID &&
					distance3D(other.player.X, other.player.Y, other.player.Z, totem.X, totem.Y, totem.Z) <= float64(def.Radius) &&
					s.totemEnemySessionHostile(other) {
					victimGUID = other.playerGUID
				}
			}
			if victimGUID == 0 {
				// Nearest-attackable search (TotemAI.cpp:68-73 +
				// GridNotifiers.h:1064): Cell::VisitAllObjects visits
				// players as well as creatures, and the searcher
				// shrinks to the nearest hit, so the nearest
				// creature and the nearest hostile session compete
				// on distance; a session-only search never fired
				// before, so enemy players in range were never hit.
				minDist := float64(math.MaxFloat64)
				for _, m := range s.server.totemCreatureTargets(totem, float64(def.Radius)) {
					if !s.server.totemEnemyCreatureValid(m, owner) {
						continue
					}
					if d := distance3D(m.X, m.Y, m.Z, totem.X, totem.Y, totem.Z); d < minDist {
						minDist = d
						victimGUID = m.GUID
					}
				}
				s.server.sessionsMu.RLock()
				for other := range s.server.sessions {
					if other == nil || other.player == nil || other.player.Map != totem.Map || other.player.InstanceID != totem.InstanceID ||
						!s.totemEnemySessionHostile(other) {
						continue
					}
					if d := distance3D(other.player.X, other.player.Y, other.player.Z, totem.X, totem.Y, totem.Z); d <= float64(def.Radius) && d < minDist {
						minDist = d
						victimGUID = other.playerGUID
					}
				}
				s.server.sessionsMu.RUnlock()
			}
			totem.mu.Lock()
			totem.LastVictimGUID = victimGUID
			totem.mu.Unlock()

			if victimGUID != 0 {
				s.executeSpellDamage(ctx, victimGUID, def.PulseSpell, uint32(60), 0)
			}
		}

	case TotemPulseDispel:
		s.totemDispelPulse(ctx, def, totem, s)
		if s.server != nil && s.groupID != 0 {
			s.server.sessionsMu.RLock()
			for other := range s.server.sessions {
				if other != s && other.worldReady.Load() && other.player != nil && other.groupID == s.groupID && other.player.Map == totem.Map && other.player.InstanceID == totem.InstanceID {
					if distance3D(other.player.X, other.player.Y, other.player.Z, totem.X, totem.Y, totem.Z) <= float64(def.Radius) {
						s.totemDispelPulse(ctx, def, totem, other)
					}
				}
			}
			s.server.sessionsMu.RUnlock()
		}

	case TotemPulseSlow:
		s.totemSlowPulse(ctx, def, totem)

	case TotemPulseSentry:
		s.sentryTotemPing(def, totem)
	}
}

// totemHealPulseTick runs one Healing Stream-style totem heal tick on the
// target session with the totem as the heal's caster, mirroring
// AuraEffect::HandlePeriodicHealAuraTick (SpellAuraEffects.cpp:5375-5440)
// for a totem-caster aura:
//
//   - no SpellHealingBonusDone leg: C++ runs it only for DYNOBJ_AURA_TYPE
//     ticks (SpellAuraEffects.cpp:5384); the flat aura amount stands (the
//     totem carries no spell power). The DBC base points of the pulse
//     spell's periodic-heal effect feed the amount when the row resolves;
//     otherwise the legacy flat 50 stands in.
//   - sScriptMgr->ModifyPeriodicDamageAurasTick ahead of the taken leg
//     (SpellAuraEffects.cpp:5376): the totem caster's AutoBalance damage
//     multiplier scales the pulse heal.
//   - SpellHealingBonusTaken with DOT type (MOD_HOT_PCT leg on,
//     SpellAuraEffects.cpp:5386-5390), keyed on the totem GUID so
//     MOD_HEALING_RECEIVED auras cast by the totem match.
//   - no crit roll: GetCritChanceFor(totem, target) is 0 (52042 also
//     carries the can't-crit attribute per the SpellMgr.cpp fix list).
//   - CalcHealAbsorb, then SMSG_SPELL_PERIODICAURALOG with the totem as
//     caster (Unit::SendPeriodicAuraLog, Unit.cpp:5368) — not
//     SMSG_SPELLHEALLOG.
//   - assist threat to the totem (the aura's caster), effectiveHeal * 0.5
//     (SpellAuraEffects.cpp:5406), gated on SPELL_ATTR1_NO_THREAT.
//   - periodic heal procs: DONE_PERIODIC on the totem's own auras,
//     TAKEN_PERIODIC on the target's (SpellAuraEffects.cpp:5423-5433).
//   - achievement credit folds totem -> owner (Unit::DealHeal,
//     Unit.cpp:6311-6312); the arena healing score credits the owner.
func (s *session) totemHealPulseTick(ctx context.Context, def TotemDef, totem *activeTotem, target *session) {
	if s == nil || s.server == nil || totem == nil || target == nil || target.player == nil {
		return
	}
	if target.player.Health == 0 {
		return
	}
	ownerLevel := uint32(1)
	if s.player != nil {
		ownerLevel = maxUint32(uint32(s.player.Level), 1)
	}
	var healSpell wotlk.Spell
	healKnown := false
	if s.server.Data != nil {
		if sp, found, err := s.server.Data.Spell(def.PulseSpell); err == nil && found {
			healSpell, healKnown = sp, true
		}
	}
	// Flat aura amount: the totem's triggered cast computes it from the DBC
	// base points at the totem's (owner's) level.
	heal := uint32(50)
	if healKnown {
		for _, eff := range healSpell.Effects {
			if eff.Aura == spellAuraPeriodicHeal {
				if v := eff.CalcValueForLevel(healSpell, ownerLevel); v > 0 {
					heal = uint32(v)
				}
				break
			}
		}
	}
	schoolMask := uint32(1 << 3) // SPELL_SCHOOL_MASK_NATURE
	if healKnown {
		schoolMask = healSpell.SchoolMask
	}
	// AuraEffect::HandlePeriodicHealAurasTick (SpellAuraEffects.cpp:5376):
	// sScriptMgr->ModifyPeriodicDamageAurasTick — AutoBalance_UnitScript's
	// _Modifer_DealDamage scales the totem caster's pulse heal ahead of
	// the taken leg, like the player-path HoT ticks.
	if totemMotion := s.server.findCreatureMotion(totem.Map, totem.InstanceID, totem.TotemGUID); totemMotion != nil {
		heal = s.server.autoBalanceModifyDealDamageOnMap(totemMotion, target.player.Map, heal)
	}
	// SpellHealingBonusTaken, DOT type (Unit.cpp:7714-7759).
	heal = target.healingTakenBonus(target, totem.TotemGUID, def.PulseSpell, heal, true)
	absorbed := uint32(0)
	if heal > 0 {
		absorbed = target.applyHealAbsorb(schoolMask, heal)
		heal -= absorbed
	}
	effectiveHeal := heal
	overheal := uint32(0)
	if target.player.Health+heal > target.player.MaxHealth {
		effectiveHeal = target.player.MaxHealth - target.player.Health
		overheal = heal - effectiveHeal
		target.player.Health = target.player.MaxHealth
	} else {
		target.player.Health += heal
	}
	// Unit::DealHeal (Unit.cpp:6311-6315): ACHIEVEMENT_CRITERIA_TYPE_HEALING_DONE
	// advances on the actual gain — overheal excluded — and skips a zero
	// gain; HIGHEST_HEAL_CAST keeps the pulse's addhealth. The totem-healer
	// rule (Unit.cpp:6306-6307, owner credited) holds structurally: s is
	// the owner's session.
	if effectiveHeal > 0 {
		s.updateAchievementCriteria(criteriaTypeHealingDone, 0, effectiveHeal)
	}
	s.setAchievementCriteria(criteriaTypeHighestHealCasted, 0, heal)
	target.updateAchievementCriteria(criteriaTypeTotalHealingReceived, 0, effectiveHeal)
	target.setAchievementCriteria(criteriaTypeHighestHealingRecv, 0, heal)
	if s.server != nil {
		s.server.updateArenaHealingScore(s, effectiveHeal)
	}
	logPkt := protocol.BuildPeriodicAuraLogHeal(target.playerGUID, totem.TotemGUID, def.PulseSpell, spellAuraPeriodicHeal, heal, overheal, absorbed, false)
	// Unit::SendPeriodicAuraLog (Unit.cpp:5368): the log rides
	// SendMessageToSet on the aura owner — the healed target's visibility
	// set, including the target itself.
	_ = target.write(uint16(protocol.OpcodeSMSG_PERIODICAURALOG), logPkt, true)
	s.server.broadcastToVisibleSet(uint16(protocol.OpcodeSMSG_PERIODICAURALOG), logPkt, target.player.Map, target.player.InstanceID, target.player.X, target.player.Y, target.player.Z, target)
	target.sendPlayerUpdate()
	if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE characters SET health = ? WHERE guid = ?", target.player.Health, target.playerGUID)
	}
	if effectiveHeal > 0 {
		// The ThreatManager.cpp:664-665 NO_THREAT shortcut is checked
		// inside distributeCreatureHealingThreat.
		s.server.distributeCreatureHealingThreat(ctx, totem.Map, totem.InstanceID, totem.TotemGUID, target.playerGUID, effectiveHeal, def.PulseSpell)
	}
	// Periodic-heal procs (SpellAuraEffects.cpp:5423-5433): %-based ticks
	// never proc — the totem pulse is a flat heal, so that gate is vacuous.
	if healKnown {
		totemMotion := s.server.findCreatureMotion(totem.Map, totem.InstanceID, totem.TotemGUID)
		s.server.fireCreaturePeriodicHealTickProcs(ctx, totemMotion, nil, target.playerGUID, totem.TotemGUID, healSpell, schoolMask, false)
		target.procPeriodicHealTickTakenAuraTriggers(ctx, healSpell, schoolMask, totem.TotemGUID, false)
	}
}

// totemDispelPulse bridges the totem dispel pulses (Cleansing Totem 8170,
// Tremor Totem 8143). C++ Cleansing: the periodic pulse spell (8171) carries
// a DUMMY effect whose script casts the dispel spell on each hit unit with
// the totem as caster and the shaman as original caster
// (spell_shaman.cpp:397-407, SPELL_SHAMAN_CLEANSING_TOTEM_EFFECT = 52025),
// fixing SPELLVALUE_BASE_POINT1 = 1 — Spell::EffectDispel dispels `damage`
// auras (SpellEffects.cpp:2453), so one aura per pulse, not three. Routing
// through dispelEffectAsCaster carries the DBC dispel-type mask, the dispel
// success roll, SMSG_SPELLDISPELLOG with the totem as dispeller, and the
// Unstable Affliction / Vampiric Touch backlash onto the totem. Tremor's
// triggered dispel spell is DBC-only (no C++ script names it), so it keeps
// the legacy path — with the count corrected to one per the tooltip.
func (s *session) totemDispelPulse(ctx context.Context, def TotemDef, totem *activeTotem, target *session) {
	if s == nil || totem == nil || target == nil || target.player == nil {
		return
	}
	if def.SpellID == 8170 && s.server != nil && s.server.Data != nil {
		if dispelSpell, found, err := s.server.Data.Spell(52025); err == nil && found {
			routed := false
			for i, eff := range dispelSpell.Effects {
				if eff.Effect != spellEffectDispel {
					continue
				}
				if i == 0 {
					// spell_sha_cleansing_totem_pulse sets
					// SPELLVALUE_BASE_POINT1 = 1 on the 52025 cast.
					eff.BasePoints = 1
				}
				target.dispelEffectAsCaster(ctx, totem.TotemGUID, target.playerGUID, dispelSpell, eff, nil, false)
				routed = true
			}
			if routed {
				return
			}
		}
	}
	target.removeHarmfulDebuffs(1)
}

// totemSlowPulse bridges the Earthbind Totem pulse (2484 -> 3600). C++: the
// totem's spell is a passive enemy area aura (Totem.cpp:115-117); there is no
// TotemAI cast, and the old Go arm had no TotemPulseSlow case at all, so
// Earthbind pulses did nothing. The Go pulse model re-applies the slow aura
// to hostile creatures in radius each tick through the creature-aura path
// with the totem's motion as caster (applyPetAura carries the no-stack
// purge, the aura-update wire, and the duration timer). The duration bridges
// two pulses so the slow neither flickers between ticks nor lingers long
// after the totem dies or the target leaves the radius — C++ drops the aura
// when the area aura no longer reaches the target.
func (s *session) totemSlowPulse(ctx context.Context, def TotemDef, totem *activeTotem) {
	if s == nil || s.server == nil || totem == nil || def.PulseSpell == 0 || s.server.Data == nil {
		return
	}
	slowSpell, found, err := s.server.Data.Spell(def.PulseSpell)
	if err != nil || !found {
		return
	}
	totemMotion := s.server.findCreatureMotion(totem.Map, totem.InstanceID, totem.TotemGUID)
	if totemMotion == nil {
		return
	}
	owner := totemOwnerPos(s)
	intervalMs := def.PulseSecs * 1000
	if intervalMs <= 0 {
		intervalMs = 2000
	}
	for _, eff := range slowSpell.Effects {
		if eff.Effect != spellEffectApplyAura {
			continue
		}
		amount := eff.CalcValueForLevel(slowSpell, uint32(totemMotion.Level))
		for _, m := range s.server.totemCreatureTargets(totem, float64(def.Radius)) {
			if m == nil || m.Health == 0 || !s.server.isHostileFaction(m.Faction, owner) {
				continue
			}
			s.applyPetAura(ctx, totemMotion, slowSpell, eff, m.GUID, uint32(intervalMs*2), uint32(amount))
		}
	}
}

// totemOwnerPos builds the owner-side playerPos for totem pulse faction
// checks. TotemAI::UpdateAI searches attackable units against
// GetCharmerOrOwnerOrSelf() (TotemAI.cpp:71), and the passive damage auras
// target enemies the same way; executeTotemPulse runs on the owner's
// session, so the owner is the reference unit.
func totemOwnerPos(s *session) playerPos {
	return playerPos{Map: s.player.Map, InstanceID: s.player.InstanceID, X: s.player.X, Y: s.player.Y, Z: s.player.Z, GUID: s.playerGUID, Race: s.player.Race, Class: s.player.Class, Level: s.player.Level, FactionTemplate: s.effectiveFactionTemplate(s.player.Race), Reputations: playerReputationMap(s.player.Reputations), Sess: s}
}

// sentryTotemPing bridges the sentry-totem arm of TotemAI::AttackStart
// (TotemAI.cpp:84-96): when a hostile unit enters the sentry totem's aggro
// range, the totem's owner receives MSG_MINIMAP_PING carrying the totem's
// GUID and X/Y. The sentry totem is REACT_AGGRESSIVE (Totem::Summon,
// Totem.cpp:103-104), so acquisition runs the standard
// Creature::GetAttackDistance radius (Creature.cpp:2022-2058: 20yd at equal
// level, +/-1yd per level difference, clamped [5,45]) with the totem at the
// owner's level (Totem::Summon sets level from the owner, Totem.cpp:99);
// the RATE_CREATURE_AGGRO multiplier and aura detect-range terms are
// unmodeled, matching the aggro scan's documented deltas. The ping fires
// once per newly acquired hostile (the AttackStart-per-acquisition analog),
// so the victim GUID is remembered until the totem sees no hostile again.
// Go scans creature motions only — hostile players near the totem have no
// bridge (no enemy-player proximity model on totems).
func (s *session) sentryTotemPing(def TotemDef, totem *activeTotem) {
	if s == nil || s.player == nil || totem == nil || s.server == nil {
		return
	}
	owner := totemOwnerPos(s)
	var nearest *creatureMotion
	minDist := float64(math.MaxFloat64)
	for _, m := range s.server.totemCreatureTargets(totem, float64(def.Radius)) {
		if m == nil || m.Health == 0 || !s.server.isHostileFaction(m.Faction, owner) {
			continue
		}
		// Creature::GetAttackDistance from the totem's side (totem level =
		// owner level; 1.5yd default combat reach, no reach data on totems).
		levelDiff := int32(s.player.Level) - int32(m.Level)
		aggroDist := float32(20.0) - 1.5 + float32(levelDiff)
		if aggroDist < 5.0 {
			aggroDist = 5.0
		} else if aggroDist > 45.0 {
			aggroDist = 45.0
		}
		d := distance3D(m.X, m.Y, m.Z, totem.X, totem.Y, totem.Z)
		if float32(d) > aggroDist {
			continue
		}
		if d < minDist {
			minDist = d
			nearest = m
		}
	}
	totem.mu.Lock()
	defer totem.mu.Unlock()
	if nearest == nil {
		totem.LastPingVictim = 0
		return
	}
	if nearest.GUID == totem.LastPingVictim {
		return
	}
	totem.LastPingVictim = nearest.GUID
	buf := protocol.NewBuffer(16)
	buf.WriteU64(totem.TotemGUID)
	buf.WriteF32(totem.X)
	buf.WriteF32(totem.Y)
	_ = s.write(uint16(protocol.OpcodeMSG_MINIMAP_PING), buf.Bytes(), true)
}

func (s *session) removeHarmfulDebuffs(count int) {
	if s == nil {
		return
	}
	s.castMu.Lock()
	var toRemove []uint32
	for spellID, aura := range s.activeAuras {
		if aura != nil && !aura.Stopped && !aura.Positive {
			toRemove = append(toRemove, spellID)
			if len(toRemove) >= count {
				break
			}
		}
	}
	s.castMu.Unlock()
	for _, spellID := range toRemove {
		s.removeAura(spellID)
	}
}

func (s *session) runTotemLifecycle(def TotemDef, totem *activeTotem) {
	pulseInterval := time.Duration(def.PulseSecs) * time.Second
	if pulseInterval <= 0 {
		pulseInterval = 2 * time.Second
	}
	ticker := time.NewTicker(pulseInterval)
	defer ticker.Stop()

	durationTimer := time.NewTimer(time.Duration(def.DurationMs) * time.Millisecond)
	defer durationTimer.Stop()

	for {
		select {
		case <-totem.StopChan:
			return
		case <-durationTimer.C:
			s.destroyTotemGUID(def.SlotID, totem.TotemGUID)
			return
		case <-ticker.C:
			totem.mu.Lock()
			stopped := totem.Stopped
			totem.mu.Unlock()
			if stopped {
				return
			}
			if !s.worldReady.Load() || s.player == nil || s.player.Map != totem.Map || s.player.InstanceID != totem.InstanceID {
				s.destroyTotemGUID(def.SlotID, totem.TotemGUID)
				return
			}
			// Totem::InitSummon applies the passive buff once at summon
			// (Totem.cpp:113-117); TotemAI::UpdateAI returns early for
			// non-active totems, so C++ never re-applies it. The immediate
			// pulse at spawn is the InitSummon twin — skipping the buff here
			// avoids resetting the aura duration, re-firing the apply arms,
			// and re-sending the aura update every PulseSecs (and keeps a
			// dispelled buff dispelled, as in C++).
			if def.PulseType == TotemPulseBuff {
				continue
			}
			s.executeTotemPulse(context.Background(), def, totem)
		}
	}
}

func (s *session) destroyTotem(slotID uint8) {
	s.destroyTotemGUID(slotID, 0)
}

// liveTotemSpellID returns the creating spell id of the caster's live totem in
// the slot, or 0 when the slot is empty. Used by the DestroyAllTotems mana
// refund arm (SpellEffects.cpp:4820-4842), which sums the totem spells' mana
// costs before unsummoning them.
func (s *session) liveTotemSpellID(slotID uint8) uint32 {
	if s == nil || s.server == nil || slotID >= 4 {
		return 0
	}
	s.server.totemMu.RLock()
	defer s.server.totemMu.RUnlock()
	slots := s.server.activeTotems[s.playerGUID]
	totem := slots[slotID]
	if totem == nil {
		return 0
	}
	totem.mu.Lock()
	defer totem.mu.Unlock()
	if totem.Stopped {
		return 0
	}
	return totem.SpellID
}

// groundingTotemMagnetGUID mirrors the SPELL_DAMAGE_CLASS_MAGIC arm of
// Spell::SelectExplicitTargets (Spell.cpp:724-757) via
// WorldObject::GetMagicHitRedirectTarget (Object.cpp:3188-3217): while the
// victim carries the SPELL_AURA_SPELL_MAGNET aura (96, SpellAuraDefines.h:176)
// sourced from a live Grounding Totem, the next harmful magic spell with an
// explicit enemy (or non-friendly plain-unit) target is redirected onto the
// totem and the magnet is consumed. C++ drops one aura charge, delayed by the
// missile travel time when the spell has Speed (Object.cpp:3204-3211); Go
// consumes the totem's magnet at selection time.
//
// Go models no aura-96 row on players (grounding totem 8177 is summoned with
// TotemPulseNone), so the live totem in the victim's air slot is the magnet:
// one redirect per totem, then MagnetUsed. The melee/ranged arm
// (Unit::GetMeleeHitRedirectTarget, Unit.cpp:6349-6363 —
// SPELL_AURA_ADD_CASTER_HIT_TRIGGER, aura 111) is bridged separately at
// the auto-attack site (meleeHitRedirectGUID, combat.go); the Speed>0
// charge-delay nuance stays unbridged (Go consumes the magnet at
// selection time).
func (s *session) groundingTotemMagnetGUID(ctx context.Context, spell wotlk.Spell, victimGUID uint64) (uint64, bool) {
	if s == nil || s.server == nil || s.player == nil || victimGUID == 0 {
		return 0, false
	}
	// DmgClass gate: only SPELL_DAMAGE_CLASS_MAGIC redirects here
	// (Spell.cpp:739-741). Go's DefenseType is the DBC DmgClass field.
	if spell.DefenseType != spellDamageClassMagic {
		return 0, false
	}
	// Object.cpp:3191-3193: abilities, CANT_BE_REDIRECTED, and
	// UNAFFECTED_BY_INVULNERABILITY spells ignore magnets.
	if spell.Attributes&spellAttr0Ability != 0 || spell.AttributesEx&spellAttr1CantBeRedirected != 0 || spell.Attributes&spellAttr0UnaffectedByInvulnerability != 0 {
		return 0, false
	}
	// Spell.cpp:728-731: the redirect only runs for explicit enemy-unit
	// targets, or plain unit targets not friendly to the caster.
	explicitMask := spellExplicitUnitTargetMask(spell)
	var victimSess *session
	if victimGUID == s.playerGUID {
		victimSess = s
	} else {
		victimSess = s.server.findSessionByGUID(victimGUID)
	}
	if victimSess == nil {
		// The magnet aura lives on the totem's owner; without a live player
		// victim there is no magnet to consult.
		return 0, false
	}
	if explicitMask&targetFlagUnitEnemy == 0 {
		if explicitMask&targetFlagUnit == 0 || s.isFriendlyToTarget(victimGUID, victimSess) {
			return 0, false
		}
	}
	// The magnet: the victim's live Grounding Totem (air slot, spell 8177).
	s.server.totemMu.RLock()
	totem := s.server.activeTotems[victimGUID][TotemSlotAir]
	s.server.totemMu.RUnlock()
	if totem == nil || totem.SpellID != 8177 {
		return 0, false
	}
	totem.mu.Lock()
	totemGUID := totem.TotemGUID
	consumed := totem.Stopped || totem.MagnetUsed
	totem.mu.Unlock()
	if consumed {
		return 0, false
	}
	// Object.cpp:3200: the magnet must pass CheckExplicitTarget and
	// IsValidAttackTarget — Go's unit-block and faction gates on the totem.
	tgt, ok := s.getCombatTarget(ctx, totemGUID)
	if !ok || tgt.Health == 0 {
		return 0, false
	}
	if spellTargetUnitBlocked(spell, tgt.UnitFlags, tgt.FlagsExtra, false) {
		return 0, false
	}
	if s.explicitTargetFactionBlocked(explicitMask, totemGUID, tgt) {
		return 0, false
	}
	totem.mu.Lock()
	if totem.Stopped || totem.MagnetUsed {
		totem.mu.Unlock()
		return 0, false
	}
	totem.MagnetUsed = true
	totem.mu.Unlock()
	return totemGUID, true
}

// creatureGroundingTotemRedirect mirrors the SPELL_DAMAGE_CLASS_MAGIC arm of
// Spell::SelectExplicitTargets (Spell.cpp:724-757) via
// WorldObject::GetMagicHitRedirectTarget (Object.cpp:3188-3217) for a
// creature caster. SelectExplicitTargets carries no caster-type gate —
// SelectSpellTargets (Spell.cpp:761) runs it for creature casts through
// Spell::cast (Spell.cpp:3410) — so a creature's harmful magic cast at a
// player victim is pulled onto the victim's live Grounding Totem and the
// magnet is consumed, exactly like the player-cast path
// (groundingTotemMagnetGUID). Caster friendliness keys on the creature's
// faction template (isFriendlyFaction/isHostileFaction; duel not counted —
// player-vs-player only, like creatureRangeTargetHostile). The PvP,
// sanctuary, party/raid and passenger arms of CheckExplicitTarget
// (SpellInfo.cpp:1786-1805) and IsValidAttackTarget are player-side and
// vacuous for a creature target — they degrade to the faction verdict, the
// same degradation the session path documents for nil target sessions. The
// melee/ranged arm (Unit::GetMeleeHitRedirectTarget, aura 111) is bridged
// at the auto-attack site (meleeHitRedirectGUID, combat.go); the Speed>0
// charge-delay nuance stays unbridged (Go consumes the magnet at
// selection time).
func (s *Server) creatureGroundingTotemRedirect(ctx context.Context, m *creatureMotion, spell wotlk.Spell, victimGUID uint64) (uint64, bool) {
	if s == nil || m == nil || victimGUID == 0 {
		return 0, false
	}
	// DmgClass gate: only SPELL_DAMAGE_CLASS_MAGIC redirects here
	// (Spell.cpp:739-741). Go's DefenseType is the DBC DmgClass field.
	if spell.DefenseType != spellDamageClassMagic {
		return 0, false
	}
	// Object.cpp:3191-3193: abilities, CANT_BE_REDIRECTED, and
	// UNAFFECTED_BY_INVULNERABILITY spells ignore magnets.
	if spell.Attributes&spellAttr0Ability != 0 || spell.AttributesEx&spellAttr1CantBeRedirected != 0 || spell.Attributes&spellAttr0UnaffectedByInvulnerability != 0 {
		return 0, false
	}
	// Spell.cpp:728-731: the redirect only runs for explicit enemy-unit
	// targets, or plain unit targets not friendly to the caster.
	explicitMask := spellExplicitUnitTargetMask(spell)
	victimSess := s.findSessionByGUID(victimGUID)
	if victimSess == nil || victimSess.player == nil {
		// The magnet aura lives on the totem's owner; without a live player
		// victim there is no magnet to consult.
		return 0, false
	}
	if explicitMask&targetFlagUnitEnemy == 0 {
		if explicitMask&targetFlagUnit == 0 {
			return 0, false
		}
		tp := playerPos{
			FactionTemplate: s.raceFaction(victimSess.player.Race),
			Race:            victimSess.player.Race,
			Class:           victimSess.player.Class,
			Sess:            victimSess,
			Reputations:     playerReputationMap(victimSess.player.Reputations),
		}
		if s.isFriendlyFaction(m.Faction, tp) {
			return 0, false
		}
	}
	// The magnet: the victim's live Grounding Totem (air slot, spell 8177).
	s.totemMu.RLock()
	totem := s.activeTotems[victimGUID][TotemSlotAir]
	s.totemMu.RUnlock()
	if totem == nil || totem.SpellID != 8177 {
		return 0, false
	}
	totem.mu.Lock()
	totemGUID := totem.TotemGUID
	consumed := totem.Stopped || totem.MagnetUsed
	totem.mu.Unlock()
	if consumed {
		return 0, false
	}
	// The totem must be a live motion on the caster's map (C++ operates on
	// the in-map object; a despawned totem cannot redirect).
	tm := s.findCreatureMotion(m.Map, m.InstanceID, totemGUID)
	if tm == nil || tm.Health == 0 {
		return 0, false
	}
	// Object.cpp:3200: the magnet must pass CheckExplicitTarget and
	// IsValidAttackTarget — the unit-flag arm plus the creature-caster
	// faction terms (SpellInfo.cpp:1786-1805).
	if spellTargetUnitBlocked(spell, tm.UnitFlags, tm.FlagsExtra, false) {
		return 0, false
	}
	totemPos := playerPos{FactionTemplate: tm.Faction}
	if explicitMask&targetFlagUnitEnemy != 0 {
		if s.isFriendlyFaction(m.Faction, totemPos) || !s.isHostileFaction(m.Faction, totemPos) {
			return 0, false
		}
	} else if explicitMask&(targetFlagUnitAlly|targetFlagUnitParty|targetFlagUnitRaid) != 0 {
		if s.isHostileFaction(m.Faction, totemPos) {
			return 0, false
		}
	}
	totem.mu.Lock()
	if totem.Stopped || totem.MagnetUsed {
		totem.mu.Unlock()
		return 0, false
	}
	totem.MagnetUsed = true
	totem.mu.Unlock()
	return totemGUID, true
}

// handleEffectDestroyAllTotems processes SPELL_EFFECT_DESTROY_ALL_TOTEMS (110).
// Reference: Spell::EffectDestroyAllTotems (SpellEffects.cpp:4820-4842) runs
// once at SPELL_EFFECT_HANDLE_HIT on the caster: every live totem in the four
// summon slots is unsummoned, and the summed mana cost of the totem spells
// (flat ManaCost + ManaCostPercentage of the caster's create mana) is restored
// via a triggered 39104 cast with the ApplyPct(damage)-scaled amount as base
// point 0. Go's create-mana model is the session's BaseMana.
func (s *session) handleEffectDestroyAllTotems(ctx context.Context, spell wotlk.Spell, eff wotlk.SpellEffect) {
	if s == nil || s.server == nil || s.player == nil {
		return
	}
	var mana int64
	for slot := uint8(0); slot < 4; slot++ {
		spellID := s.liveTotemSpellID(slot)
		if spellID != 0 {
			if sp, found, err := s.server.Data.Spell(spellID); err == nil && found {
				mana += int64(sp.ManaCost)
				// CalculatePct(GetCreateMana(), ManaCostPercentage): integer
				// truncation matches the C++ float template's cast back.
				if sp.ManaCostPct > 0 && s.player.BaseMana > 0 {
					mana += int64(s.player.BaseMana) * int64(sp.ManaCostPct) / 100
				}
			}
		}
		s.destroyTotem(slot)
	}
	// ApplyPct(mana, damage) (Util.h:74-78): mana = mana * damage / 100
	// (e.g. Totemic Recall's damage=25 refunds 25%, not 125%).
	mana = mana * int64(eff.CalcValueForLevel(spell, uint32(s.player.Level))) / 100
	if mana > 0 {
		// TRIGGERED_FULL_MASK in C++: castSpellDirect is Go's triggered-cast
		// path, with the mana amount carried as SPELLVALUE_BASE_POINT0.
		s.castSpellDirectWithBasePoint(ctx, 39104, s.playerGUID, uint32(mana))
	}
}

// isTotemGUID reports whether guid is a live player totem (C++ Unit::IsTotem,
// used by gates like Spell::EffectTaunt's totem rejection,
// SpellEffects.cpp:3141-3143).
func (srv *Server) isTotemGUID(guid uint64) bool {
	if srv == nil || guid == 0 {
		return false
	}
	srv.totemMu.RLock()
	defer srv.totemMu.RUnlock()
	for _, slots := range srv.activeTotems {
		for _, t := range slots {
			if t == nil {
				continue
			}
			t.mu.Lock()
			live := !t.Stopped && t.TotemGUID == guid
			t.mu.Unlock()
			if live {
				return true
			}
		}
	}
	if _, ok := srv.creatureTotemGUIDs[guid]; ok {
		return true
	}
	return false
}

// registerCreatureTotemGUID records a creature-cast TOTEM-arm summon (or
// a player-cast no-slot totem, summonSessionNoSlotTotem) so the
// isTotemGUID gates cover it; unregistered on timed expiry
// (expireTempSummonGuardian, expireSessionNoSlotTotem). Guarded by
// totemMu, the totem lifecycle mutex (threat.go lock-order note: totemMu
// before motionMu).
func (srv *Server) registerCreatureTotemGUID(guid uint64) {
	if srv == nil || guid == 0 {
		return
	}
	srv.totemMu.Lock()
	defer srv.totemMu.Unlock()
	if srv.creatureTotemGUIDs == nil {
		srv.creatureTotemGUIDs = make(map[uint64]struct{})
	}
	srv.creatureTotemGUIDs[guid] = struct{}{}
}

// unregisterCreatureTotemGUID drops a creature-cast totem from the gate
// set; a no-op for GUIDs never registered.
func (srv *Server) unregisterCreatureTotemGUID(guid uint64) {
	if srv == nil || guid == 0 {
		return
	}
	srv.totemMu.Lock()
	defer srv.totemMu.Unlock()
	delete(srv.creatureTotemGUIDs, guid)
}

// totemOwnerSessionGUID returns the owner player GUID of a live totem, for
// the Unit::DealHeal totem-credit rule (Unit.cpp:6320-6322): when a totem is
// the healer, the achievement/arena credit lands on the totem's owner.
func (srv *Server) totemOwnerSessionGUID(totemGUID uint64) uint64 {
	if srv == nil || totemGUID == 0 {
		return 0
	}
	srv.totemMu.RLock()
	defer srv.totemMu.RUnlock()
	for _, slots := range srv.activeTotems {
		for _, t := range slots {
			if t == nil {
				continue
			}
			t.mu.Lock()
			live := !t.Stopped && t.TotemGUID == totemGUID
			ownerGUID := t.OwnerGUID
			t.mu.Unlock()
			if live {
				return ownerGUID
			}
		}
	}
	return 0
}

func (s *session) destroyTotemGUID(slotID uint8, expectedGUID uint64) {
	if s == nil || s.playerGUID == 0 || slotID >= 4 {
		return
	}

	var totemGUID uint64
	var buffSpell uint32
	var debuffSpell uint32
	var creatingSpellID uint32
	var totemEntry uint32
	var mapID, instanceID uint32
	matched := false

	if s.server != nil {
		s.server.totemMu.Lock()
		if s.server.activeTotems != nil {
			current := s.server.activeTotems[s.playerGUID]
			totem := current[slotID]
			if totem != nil && (expectedGUID == 0 || totem.TotemGUID == expectedGUID) {
				totem.mu.Lock()
				if !totem.Stopped {
					totem.Stopped = true
					close(totem.StopChan)
				}
				totemGUID = totem.TotemGUID
				buffSpell = totem.BuffSpell
				debuffSpell = totem.DebuffSpell
				creatingSpellID = totem.SpellID
				totemEntry = totem.Entry
				mapID, instanceID = totem.Map, totem.InstanceID
				matched = true
				totem.mu.Unlock()
				current[slotID] = nil
				s.server.activeTotems[s.playerGUID] = current
			}
		}
		s.server.totemMu.Unlock()

		if totemGUID != 0 {
			s.server.motionMu.Lock()
			delete(s.server.motionMapLocked(mapID, instanceID), totemGUID)
			s.server.motionMu.Unlock()
		}
	}
	if expectedGUID != 0 && !matched {
		return
	}

	if s.player != nil {
		s.player.TotemSlots[slotID] = 0
	}

	if buffSpell > 0 && s.player != nil {
		s.removeAura(buffSpell)
	}

	// Totem::UnSummon (Totem.cpp): destroying a Sentry Totem (entry 3968)
	// strips the owner's Sentry Totem aura (spell 6495, Totem.h:30-32).
	if totemEntry == 3968 && s.player != nil {
		s.removeAura(6495)
	}

	// Totem::UnSummon (Totem.cpp:150-162): the totem's aura leaves every
	// party member in the same map and subgroup too — the C++ sweep has no
	// radius gate (only the pulse application does), so the removal keys on
	// group + map/instance only. Go has no subgroup model; groupID is the
	// equivalent. Previously only the owner's buff was dropped, leaving
	// party members' 30-minute buff auras dangling after the totem died.
	if buffSpell > 0 && s.server != nil && s.groupID != 0 {
		s.server.sessionsMu.RLock()
		for other := range s.server.sessions {
			if other != nil && other != s && other.worldReady.Load() && other.player != nil &&
				other.groupID == s.groupID &&
				other.player.Map == mapID && other.player.InstanceID == instanceID {
				other.removeAura(buffSpell)
			}
		}
		s.server.sessionsMu.RUnlock()
	}

	// The enemy-debuff leg leaves with the totem: area-aura applications
	// drop when the caster aura dies, so every creature record cast by
	// this totem's debuff spell is removed here, keyed on caster GUID
	// plus spell. Removal runs outside auraMu since removeCreatureAura
	// takes it itself.
	if debuffSpell > 0 && totemGUID != 0 && s.server != nil {
		s.server.auraMu.Lock()
		var deadKeys []creatureAuraKey
		for key, auras := range s.server.activeCreatureAuras {
			if aura, ok := auras[debuffSpell]; ok && aura != nil && aura.CasterGUID == totemGUID {
				deadKeys = append(deadKeys, key)
			}
		}
		s.server.auraMu.Unlock()
		for _, key := range deadKeys {
			s.server.removeCreatureAura(key, debuffSpell)
		}
	}

	// Session leg of the same sweep: an enemy session's 30708 record is
	// keyed on spell only in activeAuras, so the totem-GUID check here
	// is what keeps a second live totem's debuff standing when this
	// totem dies (same-spell replacement would otherwise make a
	// spell-only sweep strip it). No radius or map gate: the pulse
	// application, not the victim's current position, decides what
	// this totem's death removes, matching the creature sweep above.
	if debuffSpell > 0 && totemGUID != 0 && s.server != nil {
		s.server.sessionsMu.RLock()
		var doomed []*session
		for other := range s.server.sessions {
			if other == nil || other.player == nil {
				continue
			}
			other.castMu.Lock()
			aura := other.activeAuras[debuffSpell]
			hit := aura != nil && !aura.Stopped && aura.CasterGUID == totemGUID
			other.castMu.Unlock()
			if hit {
				doomed = append(doomed, other)
			}
		}
		s.server.sessionsMu.RUnlock()
		for _, other := range doomed {
			other.removeAura(debuffSpell)
		}
	}

	// Totem::UnSummon (Totem.cpp:150-152): every totem unsummon — expiry,
	// kill, replacement, manual destroy — sends SMSG_COOLDOWN_EVENT for
	// the creating spell (startCooldown=false, server-side cooldowns
	// untouched). buildCooldownEvent matches the C++ wire order (spell
	// u32, raw u64 owner GUID; SpellHistory.cpp:386-388).
	if creatingSpellID != 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_COOLDOWN_EVENT), buildCooldownEvent(s.playerGUID, creatingSpellID), true)
	}

	// Send SMSG_TOTEM_CREATED with 0 duration to clear totem bar icon
	// (packed zero GUID = single 0x00 mask byte, same parse as C++).
	// C++ is a silent no-op on an empty slot (SpellHandler.cpp:591-594),
	// so the clear packet only goes out when a totem was actually matched.
	if matched {
		resp := protocol.NewBuffer(17)
		resp.WriteU8(slotID)
		resp.WritePackedGUID(0)
		resp.WriteU32(0)
		resp.WriteU32(0)
		_ = s.write(uint16(protocol.OpcodeSMSG_TOTEM_CREATED), resp.Bytes(), true)
	}

	// Totem::UnSummon (Totem.cpp:150-162) removes the totem from the world;
	// the client's creature object goes away via the normal destroy path.
	// destroyTotemGUID funnels every unsummon (expiry via runTotemLifecycle,
	// kill, replacement, manual destroy).
	if totemGUID != 0 {
		destroy := protocol.NewBuffer(9)
		destroy.WriteU64(totemGUID)
		destroy.WriteU8(0)
		_ = s.write(uint16(protocol.OpcodeSMSG_DESTROY_OBJECT), destroy.Bytes(), true)
		if s.server != nil {
			s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_DESTROY_OBJECT), destroy.Bytes(), s)
		}
	}
}

func (s *session) destroyAllTotems() {
	if s == nil || s.playerGUID == 0 {
		return
	}
	for slot := uint8(0); slot < 4; slot++ {
		s.destroyTotem(slot)
	}
}
