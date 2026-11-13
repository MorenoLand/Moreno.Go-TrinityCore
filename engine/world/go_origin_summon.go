package world

import (
	"context"
	"math"
	"math/rand"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// serverSummonFromGameObject is the GO-origin twin of Spell::EffectSummonType
// (SpellEffects.cpp:2194-2440) for a gameobject caster: the HIT-mode arm that
// spawns the SUMMON effect's MiscValue creature entry at the GO's position.
// It is the goober-spell leg of GameObject::Use (GameObject.cpp:2075-2097):
// a GO casting its template spell (e.g. a spider egg's 24081) summons through
// the GO itself, not through the player that used it. The entry resolves
// generically from the spell's effect-28 MiscValue (24081 -> 15041) and the
// properties from MiscValueB (24081 -> 64) via the engine's spell-data
// accessors, so no spell or entry is hardcoded; with no Spell.dbc the
// accessors resolve nothing and the helper returns nil. The summon rides
// the shared summonTempCreature funnel, so the create block, owner/creator
// stamps (GO GUID), lifetime, and phase (the GO's PhaseMask) match the
// creature path; the funnel's event-19 fire stays silent for a GO origin
// (SummonerMotion is nil), and the egg's JustSummoned forward is wired
// below via forwardGameObjectSummon (lua_creature_events.go). A GO caster has no unit
// caster (GetUnitCasterForEffectHandlers is nil), so the arms C++ gates on
// one — VEHICLE title, TOTEM/LIGHTWELL, MINIPET, PUPPET, VEHICLE control
// (SpellEffects.cpp:2273-2301, 2353-2400) — stay out by C++'s own rule.
// duration is the summon lifetime; a non-positive duration falls back to
// the spell's DurationIndex (5 minutes for 24081), and a zero
// duration means TEMPSUMMON_DEAD_DESPAWN like C++ (SpellEffects.cpp:2323).
// The GO has no level, so the effect value and radius resolve at level 80,
// the creature path's fallback. Returns the first summon, or nil.
func (s *Server) serverSummonFromGameObject(ctx context.Context, obj *dynamicGameObjectState, spellID uint32, duration time.Duration) *creatureMotion {
	if s == nil || obj == nil || s.Data == nil {
		return nil
	}
	spell, ok, err := s.Data.Spell(spellID)
	if err != nil || !ok {
		return nil
	}
	var durationMs uint32
	if duration > 0 {
		ms := duration.Milliseconds()
		if ms > 4294967295 {
			ms = 4294967295
		}
		durationMs = uint32(ms)
	} else if spell.DurationIndex > 0 {
		if dur, ok, derr := s.Data.SpellDuration(spell.DurationIndex, 80); derr == nil && ok && dur > 0 {
			durationMs = uint32(dur)
		}
	}
	summonType := uint8(tempSummonDeadDespawn)
	if durationMs > 0 {
		summonType = uint8(tempSummonTimedDespawn)
	}
	origin := tempSummonOrigin{Map: obj.Map, InstanceID: obj.InstanceID, GUID: obj.GUID, X: obj.X, Y: obj.Y, Z: obj.Z, O: obj.Orientation, Phase: obj.PhaseMask, HasPhase: true}
	randomPoint := func(cx, cy, cz float32, dist float32) (float32, float32, float32) {
		if dist <= 0 {
			return cx, cy, cz
		}
		angle := rand.Float32() * 2 * float32(math.Pi)
		nd := rand.Float32() + rand.Float32()
		if nd > 1 {
			nd -= 2
		}
		d := dist * nd
		return cx + d*float32(math.Cos(float64(angle))), cy + d*float32(math.Sin(float64(angle))), cz
	}
	var first *creatureMotion
	for i := range spell.Effects {
		eff := spell.Effects[i]
		if eff.Effect != spellEffectSummon {
			continue
		}
		if eff.MiscValue <= 0 {
			continue
		}
		entry := uint32(eff.MiscValue)
		props, found, err := s.Data.SummonProperties(uint32(eff.MiscValueB))
		if err != nil || !found {
			continue
		}
		// No unit caster: the VEHICLE-control and PUPPET arms, and the
		// VEHICLE/TOTEM/LIGHTWELL/MINIPET titles under WILD/ALLY/UNK,
		// all return early in C++ when unitCaster is nil.
		if props.Control == summonCategoryPuppet || props.Control == summonCategoryVehicle {
			continue
		}
		if props.Control == summonCategoryWild || props.Control == summonCategoryAlly || props.Control == summonCategoryUnk {
			switch props.Title {
			case summonTitleVehicle, summonTitleVehicle2, summonTitleTotem, summonTitleLightwell, summonTitleMinipet:
				continue
			}
		}
		guardian := summonPropertiesIsGuardian(props)
		// C++ damage (SpellEffects.cpp:2225-2242): the effect's calculated
		// value at the caster's level keys the multi-summon property-ID
		// list; 64 is on it, but 24081's base points are 0, so one spawn.
		value := eff.CalcValueForLevel(spell, 80)
		numSummons := uint32(1)
		switch props.ID {
		case 64, 61, 1101, 66, 648, 2301, 1061, 1261, 629, 181, 715, 1562, 833, 1161, 713:
			if value > 0 {
				numSummons = uint32(value)
			}
		}
		radius := float32(0)
		if v, ok, rerr := s.Data.SpellRadius(eff.RadiusIndex, 80); rerr == nil && ok {
			radius = v
		}
		for count := uint32(0); count < numSummons; count++ {
			x, y, z := obj.X, obj.Y, obj.Z
			if count > 0 {
				x, y, z = randomPoint(obj.X, obj.Y, obj.Z, radius)
			}
			summoned := s.summonTempCreature(ctx, origin, entry, x, y, z, obj.Orientation, summonType, durationMs)
			if summoned == nil {
				continue
			}
			if guardian {
				s.motionMu.Lock()
				summoned.IsGuardian = true
				s.motionMu.Unlock()
			}
			// Default leg (SpellEffects.cpp:2333-2337): an ALLY summon
			// takes the caster's owner and faction; the funnel already
			// stamps OwnerGUID from the GO GUID.
			if props.Control == summonCategoryAlly {
				s.motionMu.Lock()
				summoned.Faction = obj.Faction
				s.motionMu.Unlock()
			}
			// ExecuteLogEffectSummonObject (SpellEffects.cpp:2340-2341),
			// same SMSG_SPELLLOGEXECUTE write as the creature path.
			log := protocol.NewBuffer(32)
			log.WritePackedGUID(obj.GUID)
			log.WriteU32(spell.ID)
			log.WriteU32(1)
			log.WriteU32(spellEffectSummon)
			log.WriteU32(1)
			log.WritePackedGUID(summoned.GUID)
			s.broadcastToVisibleSetFromPhase(uint16(protocol.OpcodeSMSG_SPELLLOGEXECUTE), log.Bytes(), obj.Map, obj.InstanceID, obj.X, obj.Y, obj.Z, obj.PhaseMask)
			s.forwardGameObjectSummon(obj, summoned)
			if first == nil {
				first = summoned
			}
		}
	}
	return first
}
