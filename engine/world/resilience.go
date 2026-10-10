package world

import (
	"math"
)

// Combat rating indices in Player.CombatRatings matching TrinityCore:
// CR_CRIT_TAKEN_MELEE = 14
// CR_CRIT_TAKEN_RANGED = 15
// CR_CRIT_TAKEN_SPELL = 16
const (
	CombatRatingCritTakenMelee  uint8 = 14
	CombatRatingCritTakenRanged uint8 = 15
	CombatRatingCritTakenSpell  uint8 = 16
)

// resilienceRatingPerPct stores the exact resilience rating required per 1.0% crit reduction
// for player levels 1 through 80, matching gtCombatRatings.dbc record entries (cr=14).
// TrinityCore: GtCombatRatingsEntry const* Rating = sGtCombatRatingsStore.LookupEntry(cr*100 + level - 1);
var resilienceRatingPerPct = [80]float32{
	14.375, 14.375, 14.375, 14.375, 14.375, 14.375, 14.375, 14.375, 14.375, 14.375,
	14.375, 14.375, 14.375, 14.375, 14.375, 14.375, 14.375, 14.375, 14.375, 14.375,
	14.375, 14.375, 14.375, 14.375, 14.375, 14.375, 14.375, 14.375, 14.375, 14.375,
	14.375, 14.375, 14.375, 14.375, 14.927885, 15.48077, 16.033655, 16.586538, 17.139423, 17.692308,
	18.245193, 18.798079, 19.350962, 19.903847, 20.456732, 21.009617, 21.5625, 22.115387, 22.668272, 23.221157,
	23.77404, 24.326925, 24.879808, 25.432693, 25.985579, 26.538464, 27.091347, 27.644234, 28.197117, 28.750002,
	29.841774, 31.019739, 32.294521, 33.678574, 35.186569, 36.835941, 38.647545, 40.646553, 42.86364, 45.336544,
	48.779964, 52.484921, 56.471275, 60.76041, 65.375305, 70.340721, 75.683273, 81.431602, 87.616531, 94.271225,
}

// getResilienceStats calculates:
// 1. critChanceReduction (%): reduces critical strike chance against the defender.
// 2. critDamageReduction (%): reduces critical strike damage taken by min(resilience% * 2.2, 33.0)%.
// 3. damageReduction (%): reduces all damage taken from players by min(resilience% * 2.0, 100.0)%.
// Reference: TrinityCore Unit.cpp:12357-12395, Unit.h:965-977.
func getResilienceStats(level uint8, rating uint32) (critChanceReduction float32, critDamageReduction float32, damageReduction float32) {
	if rating == 0 || level == 0 {
		return 0, 0, 0
	}
	lvlIdx := int(level) - 1
	if lvlIdx < 0 {
		lvlIdx = 0
	} else if lvlIdx >= len(resilienceRatingPerPct) {
		lvlIdx = len(resilienceRatingPerPct) - 1
	}

	scale := resilienceRatingPerPct[lvlIdx]
	if scale <= 0 {
		return 0, 0, 0
	}

	resiliencePct := float32(rating) / scale

	// 1. Crit chance reduction: resiliencePct %
	critChanceReduction = resiliencePct

	// 2. Crit damage reduction: min(resiliencePct * 2.2, 33.0)% (capped at 33.0% in 3.3.5)
	critDamageReduction = resiliencePct * 2.2
	if critDamageReduction > 33.0 {
		critDamageReduction = 33.0
	}

	// 3. All damage reduction: min(resiliencePct * 2.0, 100.0)%
	damageReduction = resiliencePct * 2.0
	if damageReduction > 100.0 {
		damageReduction = 100.0
	}

	return critChanceReduction, critDamageReduction, damageReduction
}

// applyResilienceToMeleeCritChance reduces melee/ranged crit chance against this session (in basis points, 100 bp = 1.0%).
// Reference: TrinityCore Unit::ApplyResilience (Unit.cpp:12361 / 12372).
func (s *session) applyResilienceToMeleeCritChance(attackerIsPlayer bool, cr uint8, critChanceBP *int32) {
	if !attackerIsPlayer || s == nil || s.player == nil || critChanceBP == nil || *critChanceBP <= 0 {
		return
	}
	if int(cr) >= len(s.player.CombatRatings) {
		return
	}
	rating := s.player.CombatRatings[cr]
	if rating == 0 {
		return
	}
	critRed, _, _ := getResilienceStats(s.player.Level, rating)
	reductionBP := int32(math.Round(float64(critRed * 100.0)))
	*critChanceBP -= reductionBP
	if *critChanceBP < 0 {
		*critChanceBP = 0
	}
}

// applyResilienceToDamage reduces player damage dealt to this session according to TrinityCore Unit::ApplyResilience.
// Reference: TrinityCore Unit.cpp:12363-12368, 12374-12379, 12385-12390.
func (s *session) applyResilienceToDamage(attackerIsPlayer bool, damage *uint32, isCrit bool, cr uint8) {
	if !attackerIsPlayer || s == nil || s.player == nil || damage == nil || *damage == 0 {
		return
	}
	if int(cr) >= len(s.player.CombatRatings) {
		return
	}
	rating := s.player.CombatRatings[cr]
	if rating == 0 {
		return
	}
	_, critDmgRed, dmgRed := getResilienceStats(s.player.Level, rating)

	curDmg := float64(*damage)

	// If attack was a crit, reduce damage by critDamageReduction %
	if isCrit && critDmgRed > 0 {
		critLoss := curDmg * float64(critDmgRed) / 100.0
		curDmg -= critLoss
	}

	// Reduce all damage by damageReduction %
	if dmgRed > 0 {
		dmgLoss := curDmg * float64(dmgRed) / 100.0
		curDmg -= dmgLoss
	}

	if curDmg < 1.0 {
		*damage = 1
	} else {
		*damage = uint32(math.Round(curDmg))
	}
}

// playerOwnerSessionOfCreature resolves the player session that owns the
// creature with the given GUID, or nil for wild creatures. Backs both the
// caster-side Unit::CanApplyResilience gate and the victim-side pet arm of
// Unit::GetCombatRatingReduction (Unit.cpp:12548-12553). Charm does not
// change the C++ owner field (UNIT_FIELD_SUMMONEDBY), so a charmed unit
// resolves to its pre-charm owner.
func (s *session) playerOwnerSessionOfCreature(guid uint64) *session {
	if s == nil || s.server == nil || guid == 0 {
		return nil
	}
	motion := s.findCreatureMotion(guid)
	if motion == nil {
		return nil
	}
	ownerGUID := motion.OwnerGUID
	if motion.Charmed {
		ownerGUID = motion.CharmOwnerGUID
	}
	if ownerGUID == 0 {
		return nil
	}
	return s.server.findSessionByGUID(ownerGUID)
}

// casterCanApplyResilience mirrors Unit::CanApplyResilience (Unit.cpp:12333)
// with the Player override (Player.h:1628): a player caster always applies
// resilience; any other caster applies it only when owned by a player (pets,
// guardians, totems). Charm does not change the C++ owner field
// (UNIT_FIELD_SUMMONEDBY), so a charmed unit resolves to its pre-charm owner.
// The vehicle arm (a vehicle caster never applies) has no Go model — vehicle
// occupants cast as their own GUID, which resolves to the player session.
func (s *session) casterCanApplyResilience(casterGUID uint64) bool {
	if s == nil || s.server == nil || casterGUID == 0 {
		return false
	}
	if caster := s.server.findSessionByGUID(casterGUID); caster != nil {
		return true
	}
	return s.playerOwnerSessionOfCreature(casterGUID) != nil
}
