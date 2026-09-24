package world

import (
	"context"
	"math"
	"sync"
	"time"

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
)

type TotemDef struct {
	SpellID    uint32
	SlotID     uint8
	Entry      uint32
	DurationMs uint32
	PulseType  TotemPulseType
	BuffSpell  uint32
	PulseSpell uint32
	Radius     float32
	PulseSecs  int
}

type activeTotem struct {
	mu         sync.Mutex
	SlotID     uint8
	SpellID    uint32
	TotemGUID  uint64
	Entry      uint32
	OwnerGUID  uint64
	Map        uint32
	X, Y, Z    float32
	DurationMs uint32
	BuffSpell  uint32
	CreatedAt  time.Time
	StopChan   chan struct{}
	Stopped    bool
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
	5730: {SpellID: 5730, SlotID: TotemSlotEarth, Entry: 3579, DurationMs: 15000, PulseType: TotemPulseNone, Radius: 8.0, PulseSecs: 2},

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

	// Send SMSG_TOTEM_CREATED
	resp := protocol.NewBuffer(17)
	resp.WriteU8(slot)
	resp.WriteU64(totemGUID)
	resp.WriteU32(def.DurationMs)
	resp.WriteU32(def.SpellID)
	_ = s.write(uint16(protocol.OpcodeSMSG_TOTEM_CREATED), resp.Bytes(), true)

	totem := &activeTotem{
		SlotID:     slot,
		SpellID:    def.SpellID,
		TotemGUID:  totemGUID,
		Entry:      def.Entry,
		OwnerGUID:  s.playerGUID,
		Map:        s.player.Map,
		X:          s.player.X,
		Y:          s.player.Y,
		Z:          s.player.Z,
		DurationMs: def.DurationMs,
		BuffSpell:  def.BuffSpell,
		CreatedAt:  time.Now(),
		StopChan:   make(chan struct{}),
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

		// Register creature in creatureMotion
		s.server.motionMu.Lock()
		if s.server.creatureMotion == nil {
			s.server.creatureMotion = make(map[uint64]*creatureMotion)
		}
		s.server.creatureMotion[totemGUID] = &creatureMotion{
			GUID:      totemGUID,
			Map:       s.player.Map,
			X:         s.player.X,
			Y:         s.player.Y,
			Z:         s.player.Z,
			Health:    100,
			MaxHealth: 100,
		}
		s.server.motionMu.Unlock()
	}

	// Apply immediate initial pulse
	s.executeTotemPulse(ctx, def, totem)

	// Launch pulse / lifecycle loop
	go s.runTotemLifecycle(def, totem)
}

func (s *session) executeTotemPulse(ctx context.Context, def TotemDef, totem *activeTotem) {
	if s == nil || s.player == nil || totem == nil {
		return
	}

	switch def.PulseType {
	case TotemPulseBuff:
		if def.BuffSpell > 0 {
			s.applyAura(def.BuffSpell)
			// Apply to nearby party members
			if s.server != nil && s.groupID != 0 {
				s.server.sessionsMu.RLock()
				for other := range s.server.sessions {
					if other != s && other.worldReady.Load() && other.player != nil && other.groupID == s.groupID && other.player.Map == totem.Map {
						if distance3D(other.player.X, other.player.Y, other.player.Z, totem.X, totem.Y, totem.Z) <= float64(def.Radius) {
							other.applyAura(def.BuffSpell)
						}
					}
				}
				s.server.sessionsMu.RUnlock()
			}
		}

	case TotemPulseHeal:
		healAmt := uint32(50)
		s.executeSpellHeal(ctx, s.playerGUID, def.PulseSpell, healAmt)
		if s.server != nil && s.groupID != 0 {
			s.server.sessionsMu.RLock()
			for other := range s.server.sessions {
				if other != s && other.worldReady.Load() && other.player != nil && other.groupID == s.groupID && other.player.Map == totem.Map {
					if distance3D(other.player.X, other.player.Y, other.player.Z, totem.X, totem.Y, totem.Z) <= float64(def.Radius) {
						other.executeSpellHeal(ctx, other.playerGUID, def.PulseSpell, healAmt)
					}
				}
			}
			s.server.sessionsMu.RUnlock()
		}

	case TotemPulseAoEDamage:
		if s.server != nil {
			s.server.motionMu.Lock()
			var nearbyMobs []*creatureMotion
			for _, m := range s.server.creatureMotion {
				if m != nil && m.Health > 0 && m.Map == totem.Map {
					if distance3D(m.X, m.Y, m.Z, totem.X, totem.Y, totem.Z) <= float64(def.Radius) {
						nearbyMobs = append(nearbyMobs, m)
					}
				}
			}
			s.server.motionMu.Unlock()

			dmg := uint32(75)
			for _, m := range nearbyMobs {
				s.executeSpellDamage(ctx, m.GUID, def.PulseSpell, dmg)
			}
		}

	case TotemPulseSingleTarget:
		if s.server != nil {
			s.server.motionMu.Lock()
			var nearest *creatureMotion
			minDist := float64(math.MaxFloat64)
			for _, m := range s.server.creatureMotion {
				if m != nil && m.Health > 0 && m.Map == totem.Map {
					d := distance3D(m.X, m.Y, m.Z, totem.X, totem.Y, totem.Z)
					if d <= float64(def.Radius) && d < minDist {
						minDist = d
						nearest = m
					}
				}
			}
			s.server.motionMu.Unlock()

			if nearest != nil {
				dmg := uint32(60)
				s.executeSpellDamage(ctx, nearest.GUID, def.PulseSpell, dmg)
			}
		}

	case TotemPulseDispel:
		s.removeHarmfulDebuffs(3)
		if s.server != nil && s.groupID != 0 {
			s.server.sessionsMu.RLock()
			for other := range s.server.sessions {
				if other != s && other.worldReady.Load() && other.player != nil && other.groupID == s.groupID && other.player.Map == totem.Map {
					if distance3D(other.player.X, other.player.Y, other.player.Z, totem.X, totem.Y, totem.Z) <= float64(def.Radius) {
						other.removeHarmfulDebuffs(3)
					}
				}
			}
			s.server.sessionsMu.RUnlock()
		}
	}
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
			s.destroyTotem(def.SlotID)
			return
		case <-ticker.C:
			totem.mu.Lock()
			stopped := totem.Stopped
			totem.mu.Unlock()
			if stopped {
				return
			}
			s.executeTotemPulse(context.Background(), def, totem)
		}
	}
}

func (s *session) destroyTotem(slotID uint8) {
	if s == nil || s.player == nil || slotID >= 4 {
		return
	}

	var totemGUID uint64
	var buffSpell uint32

	if s.server != nil {
		s.server.totemMu.Lock()
		if s.server.activeTotems != nil {
			current := s.server.activeTotems[s.playerGUID]
			totem := current[slotID]
			if totem != nil {
				totem.mu.Lock()
				if !totem.Stopped {
					totem.Stopped = true
					close(totem.StopChan)
				}
				totemGUID = totem.TotemGUID
				buffSpell = totem.BuffSpell
				totem.mu.Unlock()
				current[slotID] = nil
				s.server.activeTotems[s.playerGUID] = current
			}
		}
		s.server.totemMu.Unlock()

		if totemGUID != 0 {
			s.server.motionMu.Lock()
			if s.server.creatureMotion != nil {
				delete(s.server.creatureMotion, totemGUID)
			}
			s.server.motionMu.Unlock()
		}
	}

	s.player.TotemSlots[slotID] = 0

	if buffSpell > 0 {
		s.removeAura(buffSpell)
	}

	// Send SMSG_TOTEM_CREATED with 0 duration to clear totem bar icon
	resp := protocol.NewBuffer(17)
	resp.WriteU8(slotID)
	resp.WriteU64(0)
	resp.WriteU32(0)
	resp.WriteU32(0)
	_ = s.write(uint16(protocol.OpcodeSMSG_TOTEM_CREATED), resp.Bytes(), true)
}

func (s *session) destroyAllTotems() {
	if s == nil || s.player == nil {
		return
	}
	for slot := uint8(0); slot < 4; slot++ {
		s.destroyTotem(slot)
	}
}
