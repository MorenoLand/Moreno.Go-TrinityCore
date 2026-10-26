package world

import (
	"context"
	"math"
	"math/rand"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// creatureAIAreaGeometry is the shared radius/cone/center computation for
// the creature-caster area sweeps (Spell.cpp:1227-1293). It duplicates the
// geometry block of creatureAIAreaEnemyPlayers and of the player-path
// spellAreaEnemyTargets rather than refactoring either: both twins are
// live and behavior-verified, and the creature sweep below is the only
// new consumer. A later tick may fold the twins onto it.
func (s *Server) creatureAIAreaGeometry(motion *creatureMotion, spell wotlk.Spell, victim *playerPos) (radius float32, cone, backCone bool, centerX, centerY, centerZ float32) {
	destinationCenter := false
	// WorldObjectSpellConeTargetCheck::operator() (Spell.cpp:8432): the cone
	// direction is C++-custom-attr driven — CU_CONE_BACK (set in
	// SpellMgr.cpp:2840 when SpellVisual[0] == 3879) keeps targets in the back
	// cone; otherwise the front cone at the hardcoded M_PI/2 (same read as
	// the player-path area sweep).
	backCone = len(spell.SpellVisual) > 0 && spell.SpellVisual[0] == 3879
	// Spell.cpp:1282: when an effect has no radius entry (CalcRadius == 0),
	// the area radius falls back to the spell's max range — per effect via
	// GetMaxRange(IsPositiveEffect(effIndex)); the caster overload folds
	// SPELLMOD_RANGE via GetSpellModOwner (SpellInfo.cpp:3060-3072,
	// Object.cpp:2246-2268) — the pet/totem owner's session, nil for wild
	// casters.
	var maxRangePositive, maxRangeNegative float32
	modOwner := s.spellModOwnerSession(motion.Map, motion.InstanceID, motion.GUID)
	if s.Data != nil {
		if rangeEntry, ok, err := s.Data.SpellRange(spell.RangeIndex); err == nil && ok {
			maxRangePositive = float32(spellDBCMaxRange(true, rangeEntry))
			maxRangeNegative = float32(spellDBCMaxRange(false, rangeEntry))
			if modOwner != nil {
				maxRangePositive = float32(modOwner.applySpellModFloat(spell, spellModRange, float64(maxRangePositive)))
				maxRangeNegative = float32(modOwner.applySpellModFloat(spell, spellModRange, float64(maxRangeNegative)))
			}
		}
	}
	for i, eff := range spell.Effects {
		areaTargetA := isAreaEnemyTargetType(eff.ImplicitTargetA) || (eff.Effect == 27 && eff.ImplicitTargetA == 18)
		areaTargetB := isAreaEnemyTargetType(eff.ImplicitTargetB) || (eff.Effect == 27 && eff.ImplicitTargetB == 18)
		if eff.Effect == 0 || (!areaTargetA && !areaTargetB) {
			continue
		}
		for _, targetType := range []uint32{eff.ImplicitTargetA, eff.ImplicitTargetB} {
			if targetType == 24 || targetType == 54 || targetType == 104 {
				cone = true
			}
			if targetType == 16 || targetType == 18 || targetType == 28 {
				destinationCenter = true
			}
		}
		// SpellEffectInfo::CalcRadius(m_caster): per-level radius from the
		// DBC entry; the creature path has no client-side radius override.
		if s.Data != nil {
			if value, ok, err := s.Data.SpellRadius(eff.RadiusIndex, uint32(motion.Level)); err == nil && ok && value > 0 {
				// SpellEffectInfo::CalcRadius (SpellInfo.cpp:551-574): the
				// DBC radius folds SPELLMOD_RADIUS via the mod owner.
				if modOwner != nil {
					value = float32(modOwner.applySpellModFloat(spell, spellModRadius, float64(value)))
				}
				if value > radius {
					radius = value
				}
			} else {
				// Spell.cpp:1282: zero-radius fallback — per effect.
				effMaxRange := maxRangeNegative
				if spell.IsPositiveEffect(i) {
					effMaxRange = maxRangePositive
				}
				if effMaxRange > radius {
					radius = effMaxRange
				}
			}
		}
	}
	// SelectImplicitAreaTargets (Spell.cpp:1227-1270): CASTER/SRC reference
	// types center on the caster; DEST reference types center on the
	// destination. The creature path never resolves a dest packet field, so a
	// DEST-ref area falls back to the wire target's position — the same
	// fallback the player twin applies for target-dest on the creature
	// path.
	centerX, centerY, centerZ = motion.X, motion.Y, motion.Z
	if destinationCenter {
		centerX, centerY, centerZ = victim.X, victim.Y, victim.Z
	}
	return radius, cone, backCone, centerX, centerY, centerZ
}

// creatureAIAreaEnemyCreatures is the creature-motion twin of
// creatureAIAreaEnemyPlayers: Spell::SelectImplicitAreaTargets
// (Spell.cpp:1227) selects every enemy unit in the area, not just enemy
// players, so a creature standing in a pet/inline AoE is a hit target too.
// The player twin's session-only sweep left those creatures out entirely:
// no verdict, no hit, no return trip. Returns the extra creature hits (the
// wire victim and the caster are excluded) as GUIDs in an
// unspecified order. Callers re-resolve live state from the GUIDs — the
// sweep snapshot must not hand out live motion pointers. The gates mirror the player-path motion accept:
// same map/instance, alive, not a totem, not unit-flag blocked
// (spellTargetUnitBlocked, Spell.cpp:8316/2980), hostile by the
// creature-vs-creature template verdict (factionTemplatesHostile —
// reputation terms are player-only in C++, Object.cpp:2708-2840),
// cylinder area (Spell.cpp:8409), cone arc + line of sight
// (Spell.cpp:7479-7492). MaxAffectedTargets caps this list alone:
// C++ caps the merged player+creature area list (Spell.cpp:1293), so a
// mixed area can exceed the cap by the overlap — the same approximation
// the player twin already makes for its own list.
func (s *Server) creatureAIAreaEnemyCreatures(ctx context.Context, motion *creatureMotion, spell wotlk.Spell, victim *playerPos) []uint64 {
	if s == nil || motion == nil || victim == nil || !isAreaEnemySpell(spell) {
		return nil
	}
	// Spell::GetSearcherTypeMask (Spell.cpp:1836-1842): ONLY_TARGET_PLAYERS
	// / ONLY_TARGET_GHOSTS drop the CREATURE container, like the
	// player-path sweep's playersOnly arm.
	if spellSearchPlayersOnly(spell) {
		return nil
	}
	radius, cone, backCone, centerX, centerY, centerZ := s.creatureAIAreaGeometry(motion, spell, victim)
	if radius <= 0 {
		return nil
	}
	s.motionMu.Lock()
	motionMap := s.motionMapLocked(motion.Map, motion.InstanceID)
	candidates := make([]*creatureMotion, 0, len(motionMap))
	for _, cand := range motionMap {
		if cand != nil {
			candidates = append(candidates, cand)
		}
	}
	s.motionMu.Unlock()
	seen := map[uint64]struct{}{victim.GUID: {}, motion.GUID: {}}
	var hits []uint64
	for _, cand := range candidates {
		if cand.Map != motion.Map || cand.InstanceID != motion.InstanceID || cand.Health == 0 {
			continue
		}
		if _, dup := seen[cand.GUID]; dup {
			continue
		}
		if s.isTotemGUID(cand.GUID) || spellTargetUnitBlocked(spell, cand.UnitFlags, cand.FlagsExtra, false) {
			continue
		}
		if !s.factionTemplatesHostile(motion.Faction, cand.Faction) {
			continue
		}
		// WorldObjectSpellAreaTargetCheck::operator() (Spell.cpp:8409): the
		// area is a cylinder — 2D distance within the radius AND |dz| within
		// the radius — not a 3D sphere.
		dx, dy := float64(cand.X-centerX), float64(cand.Y-centerY)
		if dx*dx+dy*dy > float64(radius*radius) || math.Abs(float64(cand.Z-centerZ)) > float64(radius) {
			continue
		}
		if cone {
			if backCone {
				// WorldObject::isInBack (Object.cpp:1417): !HasInArc(2*pi - arc, target).
				if hasInArc(motion.Orientation, motion.X, motion.Y, cand.X, cand.Y, 2*math.Pi-math.Pi/2) {
					continue
				}
			} else if !hasInArc(motion.Orientation, motion.X, motion.Y, cand.X, cand.Y, math.Pi/2) {
				continue
			}
			// Spell::CheckEffectTarget default arm (Spell.cpp:7483-7492):
			// cone selection passes losPosition == nullptr, so every cone
			// candidate needs line of sight from the caster (the motion),
			// unless SPELL_ATTR2_CAN_TARGET_NOT_IN_LOS is set.
			if spell.AttributesEx1&spellAttr2CanTargetNotInLOS == 0 &&
				!s.hasLineOfSightSpellCheck(motion.Map, motion.X, motion.Y, motion.Z, cand.X, cand.Y, cand.Z) {
				continue
			}
		} else if spell.AttributesEx1&spellAttr2CanTargetNotInLOS == 0 &&
			!s.hasLineOfSightSpellCheck(motion.Map, centerX, centerY, centerZ, cand.X, cand.Y, cand.Z) {
			// Spell::CheckEffectTarget default arm (Spell.cpp:7479-7481):
			// area selection passes losPosition == center, so every area
			// candidate needs line of sight from the area center.
			continue
		}
		seen[cand.GUID] = struct{}{}
		hits = append(hits, cand.GUID)
	}
	// Spell.cpp:1293: MaxAffectedTargets caps the area list, then
	// Trinity::Containers::RandomResize keeps that many chosen uniformly at
	// random (shuffle + truncate draws the same uniform subset). The motion
	// has no aura model, so no SPELL_AURA_MOD_MAX_AFFECTED_TARGETS term
	// applies.
	if maxTargets := spell.MaxTargets; maxTargets > 0 && uint32(len(hits)) > maxTargets {
		rand.Shuffle(len(hits), func(a, b int) { hits[a], hits[b] = hits[b], hits[a] })
		hits = hits[:maxTargets]
	}
	return hits
}

// creatureAIAreaEnemyPlayers mirrors Spell::SelectImplicitAreaTargets
// (Spell.cpp:1227) for the inline AI victim-spell path. The inline path is a
// shortcut that never runs SelectSpellTargets, so a creature-cast AoE applied
// its damage only to the wire victim; in C++ the creature's cast runs the
// full selection and every enemy unit in the area becomes a hit target.
// Returns the extra player hits (the wire victim is excluded) as positions
// with sessions attached, in an unspecified order.
func (s *Server) creatureAIAreaEnemyPlayers(ctx context.Context, motion *creatureMotion, spell wotlk.Spell, victim *playerPos) []*playerPos {
	if s == nil || motion == nil || victim == nil || !isAreaEnemySpell(spell) {
		return nil
	}
	radius := float32(0)
	cone := false
	destinationCenter := false
	// WorldObjectSpellConeTargetCheck::operator() (Spell.cpp:8432): the cone
	// direction is C++-custom-attr driven — CU_CONE_BACK (set in
	// SpellMgr.cpp:2840 when SpellVisual[0] == 3879) keeps targets in the back
	// cone; otherwise the front cone at the hardcoded M_PI/2 (same read as
	// the player-path area sweep).
	backCone := len(spell.SpellVisual) > 0 && spell.SpellVisual[0] == 3879
	// Spell.cpp:1282: when an effect has no radius entry (CalcRadius == 0),
	// the area radius falls back to the spell's max range — per effect via
	// GetMaxRange(IsPositiveEffect(effIndex)); the caster overload folds
	// SPELLMOD_RANGE via GetSpellModOwner (SpellInfo.cpp:3060-3072,
	// Object.cpp:2246-2268) — the pet/totem owner's session, nil for wild
	// casters.
	var maxRangePositive, maxRangeNegative float32
	modOwner := s.spellModOwnerSession(motion.Map, motion.InstanceID, motion.GUID)
	if s.Data != nil {
		if rangeEntry, ok, err := s.Data.SpellRange(spell.RangeIndex); err == nil && ok {
			maxRangePositive = float32(spellDBCMaxRange(true, rangeEntry))
			maxRangeNegative = float32(spellDBCMaxRange(false, rangeEntry))
			if modOwner != nil {
				maxRangePositive = float32(modOwner.applySpellModFloat(spell, spellModRange, float64(maxRangePositive)))
				maxRangeNegative = float32(modOwner.applySpellModFloat(spell, spellModRange, float64(maxRangeNegative)))
			}
		}
	}
	for i, eff := range spell.Effects {
		areaTargetA := isAreaEnemyTargetType(eff.ImplicitTargetA) || (eff.Effect == 27 && eff.ImplicitTargetA == 18)
		areaTargetB := isAreaEnemyTargetType(eff.ImplicitTargetB) || (eff.Effect == 27 && eff.ImplicitTargetB == 18)
		if eff.Effect == 0 || (!areaTargetA && !areaTargetB) {
			continue
		}
		for _, targetType := range []uint32{eff.ImplicitTargetA, eff.ImplicitTargetB} {
			if targetType == 24 || targetType == 54 || targetType == 104 {
				cone = true
			}
			if targetType == 16 || targetType == 18 || targetType == 28 {
				destinationCenter = true
			}
		}
		// SpellEffectInfo::CalcRadius(m_caster): per-level radius from the
		// DBC entry; the inline path has no client-side radius override.
		if s.Data != nil {
			if value, ok, err := s.Data.SpellRadius(eff.RadiusIndex, uint32(motion.Level)); err == nil && ok && value > 0 {
				// SpellEffectInfo::CalcRadius (SpellInfo.cpp:551-574): the
				// DBC radius folds SPELLMOD_RADIUS via the mod owner.
				if modOwner != nil {
					value = float32(modOwner.applySpellModFloat(spell, spellModRadius, float64(value)))
				}
				if value > radius {
					radius = value
				}
			} else {
				// Spell.cpp:1282: zero-radius fallback — per effect.
				effMaxRange := maxRangeNegative
				if spell.IsPositiveEffect(i) {
					effMaxRange = maxRangePositive
				}
				if effMaxRange > radius {
					radius = effMaxRange
				}
			}
		}
	}
	if radius <= 0 {
		return nil
	}
	// SelectImplicitAreaTargets (Spell.cpp:1227-1270): CASTER/SRC reference
	// types center on the caster; DEST reference types center on the
	// destination. The inline path never resolves a dest packet field, so a
	// DEST-ref area falls back to the wire target's position — the same
	// fallback the dest-family twin applies for target-dest on the creature
	// path.
	centerX, centerY, centerZ := motion.X, motion.Y, motion.Z
	if destinationCenter {
		centerX, centerY, centerZ = victim.X, victim.Y, victim.Z
	}
	seen := map[uint64]struct{}{victim.GUID: {}}
	var hits []*playerPos
	s.sessionsMu.RLock()
	for targetSession := range s.sessions {
		if targetSession == nil || !targetSession.authed || !targetSession.worldReady.Load() || targetSession.player == nil || targetSession.player.Health == 0 {
			continue
		}
		if targetSession.player.Map != motion.Map || targetSession.player.InstanceID != motion.InstanceID {
			continue
		}
		if _, dup := seen[targetSession.playerGUID]; dup {
			continue
		}
		// The player-path area sweep applies no unit-flag block to player
		// candidates (the C++ totem/unattackable arms can't match a player
		// session); the faction gate below carries the enemy check.
		candidate := playerPos{Map: targetSession.player.Map, InstanceID: targetSession.player.InstanceID, X: targetSession.player.X, Y: targetSession.player.Y, Z: targetSession.player.Z, GUID: targetSession.playerGUID, Race: targetSession.player.Race, Class: targetSession.player.Class, Level: targetSession.player.Level, FactionTemplate: targetSession.effectiveFactionTemplate(targetSession.player.Race), Reputations: playerReputationMap(targetSession.player.Reputations), Sess: targetSession}
		// WorldObjectSpellTargetCheck::operator() (Spell.cpp:8316):
		// TARGET_CHECK_ENEMY — the same call shape as the creature's own
		// aggro pick (creaturemotion.go canCreatureStartAttack).
		if !s.isAttackableFaction(motion.Faction, candidate) {
			continue
		}
		// WorldObjectSpellAreaTargetCheck::operator() (Spell.cpp:8409): the
		// area is a cylinder — 2D distance within the radius AND |dz| within
		// the radius — not a 3D sphere.
		dx, dy := float64(candidate.X-centerX), float64(candidate.Y-centerY)
		if dx*dx+dy*dy > float64(radius*radius) || math.Abs(float64(candidate.Z-centerZ)) > float64(radius) {
			continue
		}
		if cone {
			if backCone {
				// WorldObject::isInBack (Object.cpp:1417): !HasInArc(2*pi - arc, target).
				if hasInArc(motion.Orientation, motion.X, motion.Y, candidate.X, candidate.Y, 2*math.Pi-math.Pi/2) {
					continue
				}
			} else if !hasInArc(motion.Orientation, motion.X, motion.Y, candidate.X, candidate.Y, math.Pi/2) {
				continue
			}
			// Spell::CheckEffectTarget default arm (Spell.cpp:7483-7492):
			// cone selection passes losPosition == nullptr, so every cone
			// candidate needs line of sight from the caster (the motion),
			// unless SPELL_ATTR2_CAN_TARGET_NOT_IN_LOS is set.
			if spell.AttributesEx1&spellAttr2CanTargetNotInLOS == 0 &&
				!s.hasLineOfSightSpellCheck(motion.Map, motion.X, motion.Y, motion.Z, candidate.X, candidate.Y, candidate.Z) {
				continue
			}
		} else if spell.AttributesEx1&spellAttr2CanTargetNotInLOS == 0 &&
			!s.hasLineOfSightSpellCheck(motion.Map, centerX, centerY, centerZ, candidate.X, candidate.Y, candidate.Z) {
			// Spell::CheckEffectTarget default arm (Spell.cpp:7479-7481):
			// area selection passes losPosition == center, so every area
			// candidate needs line of sight from the area center.
			continue
		}
		seen[targetSession.playerGUID] = struct{}{}
		cp := candidate
		hits = append(hits, &cp)
	}
	s.sessionsMu.RUnlock()
	// Spell.cpp:1293: MaxAffectedTargets caps the area list, then
	// Trinity::Containers::RandomResize keeps that many chosen uniformly at
	// random (shuffle + truncate draws the same uniform subset). The motion
	// has no aura model, so no SPELL_AURA_MOD_MAX_AFFECTED_TARGETS term
	// applies.
	if maxTargets := spell.MaxTargets; maxTargets > 0 && uint32(len(hits)) > maxTargets {
		rand.Shuffle(len(hits), func(a, b int) { hits[a], hits[b] = hits[b], hits[a] })
		hits = hits[:maxTargets]
	}
	return hits
}

// applyInlineCreatureSpellHit runs the per-hit legs of the inline AI
// victim-spell path for one player hit: the AutoBalance damage scaling,
// Unit::DealDamage (Unit.cpp:735-973), the SMSG_SPELLNONMELEEDAMAGELOG
// (Spell.cpp:2542), the victim-side proc half (Spell.cpp:2581-2586), the
// stand-up arm (Spell.cpp:2609-2617), the caster-side SpellHitTarget Eluna
// hook (Spell.cpp:2639-2650), the per-hit trigger consumer
// (Spell.cpp:2652/2913-2957) and the spell_linked type-0 tail
// (Spell.cpp:3503-3512). In C++ these run inside DoTargetSpellHit for every
// target SelectImplicitAreaTargets (Spell.cpp:1227) selected — not just the
// wire victim — which is why this helper exists separately from the cast.
func (s *Server) applyInlineCreatureSpellHit(ctx context.Context, motion *creatureMotion, p *playerPos, spellID, damage uint32, schoolMask uint8, now time.Time) {
	// Spell::EffectSchoolDMG (SpellEffects.cpp:785): the victim-side taken
	// pct legs run on every inline spell hit, ahead of the taken-mitigation
	// block — Unit::SpellDamageBonusTaken (Unit.cpp:7052). The caster is a
	// wild motion (no session), so it arrives as nil.
	var inlineSpell wotlk.Spell
	inlineSpellKnown := false
	if s != nil && s.Data != nil {
		if sp, found, err := s.Data.Spell(spellID); err == nil && found {
			inlineSpell, inlineSpellKnown = sp, true
		}
	}
	var absorbed, resisted, blocked uint32
	// Unit::CalculateSpellDamageTaken (Unit.cpp:987-1132) arm order on the
	// direct-spell path: victim-side taken pct legs (EffectSchoolDMG) run
	// first, then the physical-school armor arm (Unit.cpp:1002-1006),
	// then the DmgClass switch (crit — wild mobs roll none, Unit.cpp:7167;
	// the player-owned crit arm below — plus the melee/ranged block arm and
	// resilience), then
	// sScriptMgr->ModifySpellDamageTaken (Unit.cpp:1115-1117, the AutoBalance
	// _Modifer_DealDamage hook, AutoBalance.cpp:236), and finally the
	// CalcAbsorbResist tail (Unit.cpp:1121-1130: resist, then absorb).
	// Unit::DealDamageMods (Unit.cpp:686) is the evade/flight zeroing arm,
	// not a damage-modifier hook — it never fires on this path.
	takenBlockRan := false
	// Spell::DoEffectOnLaunchTarget (Spell.cpp:7790): the crit bit lands in
	// the damage log; wild mobs roll no crits (Unit.cpp:7167), so this
	// stays 0 unless the player-owned crit arm below fires.
	isCrit := false
	hitInfo := uint32(0)
	if p.Sess != nil && p.Sess.player != nil && damage > 0 {
		takenBlockRan = true
		if inlineSpellKnown {
			damage = spellDamageBonusTaken(damage, inlineSpell, uint32(schoolMask), p.Sess, nil, 1.0)
		}
		// Unit::IsDamageReducedByArmor (Unit.cpp:1618-1625): physical
		// schools only, skipped by SPELL_ATTR0_CU_IGNORE_ARMOR; a null
		// spellInfo runs the arm, so an unknown spell does too. The
		// SPELLMOD_IGNORE_ARMOR / player-ARP / npcbot legs are
		// player-or-bot-gated in C++ — a creature caster gets none.
		// Hybrid (melee+magic) spells skip this arm: C++ strips the NORMAL
		// bit at load (SpellMgr.cpp:2830), so IsDamageReducedByArmor is
		// false for them, and the armor comparison moves into
		// CalcSpellResistedDamage (Unit.cpp:1764-1770) — which runs even
		// under CU_IGNORE_ARMOR.
		isHybridSpell := inlineSpellKnown && inlineSpell.SchoolMask&spellSchoolMaskNormal != 0 && inlineSpell.SchoolMask&0x7E != 0
		if damage > 0 && schoolMask&spellSchoolMaskNormal != 0 &&
			!isHybridSpell && (!inlineSpellKnown || inlineSpell.Attributes&spellAttr0CUIgnoreArmor == 0) {
			armor := s.creatureIgnoreTargetResistArmorForSpell(creatureAuraKeyForMotion(motion), float64(p.Sess.player.Armor), inlineSpell, inlineSpellKnown)
			damage = calcArmorReducedDamage(armor, uint8(maxUint32(motion.Level, 1)), p.Sess.player.Level, damage)
		}
		// Unit::SpellCritChanceDone (Unit.cpp:7167-7212): wild mobs can't
		// crit with spells; only a caster with a non-null GetSpellModOwner
		// (Object.cpp:2246-2261 — a player, a pet, or a player-owned
		// totem) can. A player-charmed wild mob has a player charmer
		// (OwnerGUID) but is neither pet nor totem, so C++ gives it no
		// crits; the GetSpellModOwner twin below resolves it to nil and
		// the whole arm stays closed, same as a wild creature. The crit
		// bonus sits
		// in the DmgClass switch (Unit.cpp:1023-1040), post-armor,
		// pre-resilience, and the taken arms feed the roll
		// (Unit.cpp:7228-7240). The melee/ranged-DmgClass taken leg rides
		// Unit::GetUnitCriticalChanceTaken (Unit.cpp:2858-2901) via
		// creatureSpellCritChanceTaken — the caster-side scripted arms
		// (Shatter) read caster OVERRIDE_CLASS_SCRIPTS auras the
		// creature store never carries and stay unbridged.
		var owner *session
		if motion != nil {
			owner = s.spellModOwnerSession(motion.Map, motion.InstanceID, motion.GUID)
		}
		if damage > 0 && inlineSpellKnown && motion != nil && owner != nil {
			chance := s.creatureSpellCritChanceDone(creatureAuraKeyForMotion(motion), motion.FlagsExtra&0x20000 != 0, schoolMask, inlineSpell, true)
			// Unit::SpellCritChanceDone (Unit.cpp:7209-7210): the mod
			// owner's SPELLMOD_CRITICAL_CHANCE folds into the done
			// chance, and its SPELLMOD_CRIT_DAMAGE_BONUS into the
			// crit-bonus arms below — owner is the GetSpellModOwner
			// twin, never the raw charmer GUID.
			// Spell::DoEffectOnLaunchTarget (Spell.cpp:7782-7790): the taken
			// fold runs unconditionally — there is no done-gate before it.
			if owner != nil && owner.player != nil {
				chance = float64(owner.applySpellMod(inlineSpell, spellModCriticalChance, int32(math.Round(chance*100)))) / 100.0
			}
			// Unit::SpellCritChanceTaken (Unit.cpp:7228-7240): MAGIC
			// takes 179 pre-resilience and 197 post, MELEE/RANGED take
			// the GetUnitCriticalChanceTaken leg (Unit.cpp:2858-2901).
			// Unit::CanApplyResilience (Unit.cpp:12333) runs on the caster:
			// the owner must be a player — a creature-charmed caster
			// resolves to nil here and skips the reduction — and the
			// caster must not be a vehicle (!IsVehicle, the
			// UNIT_MASK_VEHICLE / vehicle-kit arm).
			chance = s.creatureSpellCritChanceTaken(chance, p.Sess, creatureAuraKey{}, true, motion.GUID, motion.Level, uint32(p.Sess.player.Level), schoolMask, inlineSpell, s.creatureCasterCanApplyResilience(motion))
			isCrit = chance > 0 && rand.Float64() < chance
			if isCrit {
				// Unit::SpellCriticalDamageBonus (Unit.cpp:7411-7449) /
				// the melee/ranged arm (Unit.cpp:1045-1062): the caster's
				// 163/169 crit-damage auras now fold from the creature
				// store (the old nil-session call carried only the base
				// half/double).
				mask, _ := p.Sess.targetCreatureTypeMask(ctx, p.GUID)
				if inlineSpell.DefenseType == spellDamageClassMagic {
					damage = s.creatureSpellCriticalDamageBonus(creatureAuraKeyForMotion(motion), uint32(schoolMask), damage, mask, inlineSpell, owner)
				} else {
					damage = s.creatureSpellWeaponCritDamageBonus(creatureAuraKeyForMotion(motion), creatureAuraKey{}, p.Sess, true, uint32(schoolMask), damage, inlineSpell, mask, owner)
				}
				hitInfo = 0x02
			}
		}
		// Unit::CalculateSpellDamageTaken block arm (Unit.cpp:1013-1062)
		// with Unit::isSpellBlocked (Unit.cpp:2412-2430): melee/ranged
		// DmgClass physical spells can be blocked even when the caster is
		// a wild motion — the block chance and shield value are victim-side
		// (the player's CanBlock gate rejects creature victims implicitly:
		// this path only hits players). Sits after the crit fold and before
		// resilience, matching the C++ arm order.
		if damage > 0 && inlineSpellKnown && motion != nil {
			victimTarget := combatTarget{
				GUID:        p.GUID,
				X:           p.Sess.player.X,
				Y:           p.Sess.player.Y,
				Orientation: p.Sess.player.Orientation,
				Level:       p.Sess.player.Level,
				Armor:       p.Sess.player.Armor,
			}
			if b, ok := s.spellTakenBlockAmountFrom(victimTarget, true, p.Sess, inlineSpell, damage, uint8(maxUint32(motion.Level, 1)), motion.X, motion.Y); ok {
				blocked = b
				damage -= b
			}
		}
		// Unit::CalculateSpellDamageTaken resilience arm (Unit.cpp:1079,
		// 1105): Unit::CanApplyResilience (Unit.cpp:12333) runs on the
		// caster — GetOwnerGUID().IsPlayer() with !IsVehicle(), charm
		// resolving to the pre-charm owner (UNIT_FIELD_SUMMONEDBY). A wild
		// creature's owner is no player, and neither is a creature charmer,
		// so those casts never reduce the victim by resilience; a
		// player-owned non-vehicle motion (pet/charm/totem) does.
		if damage > 0 && s.creatureCasterCanApplyResilience(motion) {
			// Unit::CalculateSpellDamageTaken (Unit.cpp:1079-1080): the
			// MELEE/RANGED-DmgClass arm reduces by the melee/ranged taken
			// rating, not the spell one (Unit.cpp:1105-1107 keeps
			// CR_CRIT_TAKEN_SPELL for the MAGIC arm).
			resilCR := uint8(CombatRatingCritTakenSpell)
			if inlineSpellKnown && inlineSpell.DefenseType != spellDamageClassMagic {
				if spellWeaponAttackType(inlineSpell) == protocol.RangedAttack {
					resilCR = CombatRatingCritTakenRanged
				} else {
					resilCR = CombatRatingCritTakenMelee
				}
			}
			p.Sess.applyResilienceToDamage(true, &damage, isCrit, resilCR)
		}
		// ModifySpellDamageTaken: the AutoBalance damage multiplier scales
		// the post-switch damage, ahead of the resist/absorb tail.
		damage = s.autoBalanceModifyDealDamage(motion, p.Sess, damage)
		// Unit::CalcAbsorbResist (Unit.cpp:1977-2060) resistance leg:
		// partial resists run for magic schools only. The creature caster's
		// SPELL_AURA_MOD_TARGET_RESISTANCE (123) misc-mask sum lands on the
		// template resistance (Unit.cpp:1791-1792); the spell-derived
		// ATTR4/binary/hybrid arms ride the spellResistInput.
		preResistDmg := damage
		if damage > 0 && schoolMask > 1 {
			mask123 := uint32(schoolMask)
			if mask123&0x7E != 0 {
				mask123 &^= 1
			}
			var targetResistMod int32
			for _, amount := range s.creatureAuraTypeAmountsByMiscMask(creatureAuraKeyForMotion(motion), spellAuraModTargetResistance, mask123) {
				targetResistMod += amount
			}
			// Unit::CalcSpellResistedDamage (Unit.cpp:1764-1770): the hybrid
			// armor figure is evaluated on the post-crit damage, like the
			// C++ leg inside DealDamage.
			hybridArmorReduced := uint32(0)
			if isHybridSpell {
				armor := s.creatureIgnoreTargetResistArmorForSpell(creatureAuraKeyForMotion(motion), float64(p.Sess.player.Armor), inlineSpell, inlineSpellKnown)
				hybridArmorReduced = calcArmorReducedDamage(armor, uint8(maxUint32(motion.Level, 1)), p.Sess.player.Level, damage)
			}
			resisted, damage = calcMagicSpellResistance(damage, schoolMask, p.Sess.player.Resistances, uint8(maxUint32(motion.Level, 1)), p.Sess.player.Level, false, false, 0, spellResistInput{
				ignoreResistances:  inlineSpellKnown && inlineSpell.AttributesEx4&spellAttr4IgnoreResistances != 0,
				binary:             inlineSpellKnown && inlineSpell.AttributesCu&spellAttr0CuBinarySpell != 0,
				hybrid:             isHybridSpell,
				hybridArmorReduced: hybridArmorReduced,
				targetResistMod:    targetResistMod,
			})
		}
		// Unit::CalcAbsorbResist (Unit.cpp:1839-1857) absorb leg: max(194
		// misc-masked pct, 245 ability pct affecting the cast spell) of the
		// damage bypasses the victim's shields.
		if damage > 0 {
			// C++ takes the bypass pct on the pre-resist damage (Unit.cpp:1853) and re-adds it after the absorb loops (:1975); the pool is the post-resist damage less the bypass, floored at zero (ModifyDamage clamp, Unit.cpp:194-196). Nothing sits between resist and absorb here, so preResistDmg is the CalcAbsorbResist entry damage.
			bypass := absorbIgnoreBypass(preResistDmg, s.creatureAbsorbIgnorePctForSpell(creatureAuraKeyForMotion(motion), uint32(schoolMask), spellID))
			pool := damage
			if bypass > pool {
				pool = 0
			} else {
				pool -= bypass
			}
			absorbed, damage = p.Sess.applyAbsorptionShields(pool, schoolMask)
			damage += bypass
		}
	}
	if !takenBlockRan {
		damage = s.autoBalanceModifyDealDamage(motion, p.Sess, damage)
	}
	overkill := uint32(0)
	// Spell::EffectHealthLeech leech basis (SpellEffects.cpp:1539-1540):
	// the damage the target actually lost — overkill excluded, post-absorb
	// — captured before the damage legs mutate health; the caster-heal arm
	// lands at the end of this function with the other cast-completion legs.
	leechDealt := uint32(0)
	if p.Sess != nil && p.Sess.player != nil {
		// Unit::DealDamage (Unit.cpp:735-737): CHEAT_GOD victims take no
		// damage — the kill/health legs are skipped explicitly (not via a
		// damage==0 gate) because absorbed-to-zero damage still runs them
		// in C++.
		godNegated := p.Sess.godCheatActive()
		// Unit::CalcAbsorbResist (Unit.cpp:1977-2060): split-damage auras
		// (81/153) redirect part of the damage to the aura's caster, ahead
		// of the 300 share leg. The damage log below keeps the post-split
		// amount (the split hook sends its own log for the redirected
		// portion); only the god arm predates the log, matching C++
		// sending SMSG_SPELLNONMELEEDAMAGELOG before DealDamage
		// (Spell.cpp:2542).
		if !godNegated && damage > 0 {
			damage = p.Sess.server.splitDamageToCaster(ctx, p.GUID, true, creatureAuraKey{}, motion.GUID, damage, uint32(schoolMask))
		}
		// Unit::DealDamage (Unit.cpp:766-788):
		// SPELL_AURA_SHARE_DAMAGE_PCT copies CalculatePct(damage,
		// amount) to the aura's caster.
		if !godNegated && damage > 0 {
			p.Sess.splitShareDamagePct(ctx, p.GUID, true, creatureAuraKey{}, motion.GUID, damage, uint32(schoolMask))
		}
		// Unit::DealDamage (Unit.cpp:869-870): HIGHEST_HIT_RECEIVED
		// fires for any damage to a player victim, creature-caster
		// spells included.
		p.Sess.setAchievementCriteria(criteriaTypeHighestHitReceived, 0, damage)
		victimHealth := p.Sess.player.Health
		if !godNegated {
			leechDealt = damage
			if leechDealt > victimHealth {
				leechDealt = victimHealth
			}
		}
		// Duel defeat (Unit.cpp:825-853, 957-973): a charmed creature's
		// spell lands as a duel defeat when its charmer is the duel
		// opponent (GetControllingPlayer, Unit.cpp:5996); a wild
		// creature's exactly-health-1 hit still completes the duel as
		// won, while its lethal hit kills and interrupts.
		if !godNegated && duelDefeatOnDamage(p.Sess, p.Sess.controllingPlayerGUID(motion.GUID), damage, victimHealth) {
			// Duel defeat consumed the hit — loser at 1 HP, duel complete.
		} else if !godNegated && damage >= victimHealth {
			overkill = damage - victimHealth
			p.Sess.player.Health = 0
			p.IsDead = true
			p.Sess.updateAchievementCriteria(criteriaTypeKilledByCreature, uint32((motion.GUID>>24)&0xFFFFFF), 1)
			// GetCharmerOrOwnerPlayerOrPlayerItself (Unit.cpp:11279):
			// a player-owned pet's kill credits its owner; a wild
			// creature resolves to no player.
			var creatureKiller *session
			if motion.OwnerGUID != 0 {
				creatureKiller = s.findSessionByGUID(motion.OwnerGUID)
			}
			p.Sess.killPlayer(ctx, creatureKiller, false, true)
			// Eluna CREATURE_EVENT_ON_TARGET_DIED (3): C++ Unit::Kill
			// player-victim branch — attacker is a wild creature (no
			// owner player, so no pet arm):
			// attacker->AI()->KilledUnit(victim) (Unit.cpp:11359-11361).
			s.fireCreatureTargetDied(ctx, motion, p.Sess.luaPlayer())
			// Eluna PLAYER_EVENT_ON_KILLED_BY_CREATURE (8): Unit::Kill tail
			// (Unit.cpp:11466-11480), attacker is the creature.
			s.fireKilledByCreatureHook(ctx, motion, p.Sess)
			if motion.BossAI != nil {
				motion.BossAI.OnKillPlayer(ctx, s, motion, p.GUID)
			}
			// Unit::Kill (Unit.cpp:11283-11290): the PROC_FLAG_KILL leg
			// fires for the killing-blow unit regardless of damage
			// source — the melee path bridges it at its own kill site,
			// and this spell-hit site was missing it; the KILLED/DEATH
			// legs already ran inside killPlayer (death.go).
			s.fireCreatureMeleeAuraProcs(ctx, motion, p.GUID, procEventInfo{
				typeMask:       procFlagKill,
				spellTypeMask:  procSpellTypeMaskAll,
				spellPhaseMask: procSpellPhaseNone,
				hitMask:        procHitNone,
				actorGUID:      motion.GUID,
			})
		} else if !godNegated {
			p.Sess.player.Health -= damage
			// Unit::DealDamage (Unit.cpp:915-924): rage from damage received.
			p.Sess.grantRageFromDamageTaken(ctx, damage)
			// Unit::DealDamage (Unit.cpp:906-913): random durability
			// loss on HIT TAKEN — the victim is a player.
			p.Sess.rollDurabilityLossOnHit(ctx, damage)
			// Unit::DealDamage (Unit.cpp:936-937): a damage spell carrying
			// SPELL_ATTR7_NO_PUSHBACK_ON_DAMAGE or
			// SPELL_ATTR3_TREAT_AS_PERIODIC never delays the victim's
			// cast or channel — the !spellProto arm pushes back when the
			// DBC row is unavailable, and the pushback leg requires
			// non-zero damage. The victim != attacker arm is vacuous
			// here (creature caster, player victim).
			pushesBack := true
			if s != nil && s.Data != nil {
				if spellInfo, found, err := s.Data.Spell(spellID); err == nil && found {
					pushesBack = spellInfo.AttributesEx3&spellAttr3TreatAsPeriodic == 0 &&
						spellInfo.AttributesEx7&spellAttr7NoPushbackOnDamage == 0
				}
			}
			if damage > 0 && pushesBack {
				// Reference Unit::DealDamage -> Spell::Delayed / DelayedChannel
				p.Sess.delayCurrentCast()
				p.Sess.delayCurrentChannel()
			}
			p.Sess.procSpellDirectDamageAuras(spellID)
		}
		logPkt := buildSpellNonMeleeDamageLog(p.GUID, motion.GUID, spellID, damage, overkill, schoolMask, absorbed, resisted, hitInfo, blocked)
		_ = p.Sess.write(uint16(protocol.OpcodeSMSG_SPELLNONMELEEDAMAGELOG), logPkt, true)
		if s != nil {
			s.broadcastToInstance(motion.Map, motion.InstanceID, uint16(protocol.OpcodeSMSG_SPELLNONMELEEDAMAGELOG), logPkt, p.Sess)
		}
		p.Sess.lastCombatTime = now
		if p.Sess.player.UnitFlags&unitFlagInCombat == 0 {
			p.Sess.player.UnitFlags |= unitFlagInCombat
		}
		p.Sess.sendPlayerUpdate()
		// Unit::DealDamage (Unit.cpp:728-733): the victim's controlled
		// creatures are signaled OwnerAttackedBy on any non-DoT damage.
		if s != nil {
			s.triggerPetDefensive(ctx, p.Map, p.InstanceID, p.GUID, motion.GUID, scripting.CreatureEventOnOwnerAttackedAt)
		}
	}
	// Spell::TargetInfo::DoDamageAndTriggers (Spell.cpp:2581-2586):
	// Unit::ProcSkillsAndAuras (Unit.cpp:5355-5366) fires the victim-
	// side half with the taken pre-fill plus PROC_FLAG_TAKEN_DAMAGE
	// ORed by the damage arm (Spell.cpp:2545) — the player victim's
	// own auras evaluate the hit as a taken-proc event
	// (PROC_SPELL_TYPE_DAMAGE, PROC_SPELL_PHASE_HIT; Unit.cpp:
	// 10385-10418). The canEffectTrigger gate (Spell.cpp:2440-2442):
	// SPELL_ATTR3_CANT_TRIGGER_PROC never reaches the loop (checked
	// inside procSpellHitTakenAuraTriggers), the m_procDeep recursion
	// guard is the victim session's procDeep (Unit::CanProc,
	// Unit.h:1609), and the CanExecuteTriggersOnHit(EffectMask) arm
	// folds into the non-zero-effect model — this path applies every
	// non-zero effect to every hit p. Runs after the damage
	// legs and before the stand-up arm, matching C++ order (2581
	// precedes 2609), and fires even under the god cheat — C++'s
	// CHEAT_GOD skip lives inside DealDamage, not the proc call.
	// The creature actor has no auras, so only the taken leg runs.
	if p.Sess != nil && p.Sess.player != nil && p.Sess.procDeep == 0 {
		p.Sess.procSpellHitTakenAuraTriggers(ctx, motion.GUID, spellID, true, false, false, false, false, 0, damage, false, false, 0)
	}
	// Spell::TargetInfo::DoDamageAndTriggers (Spell.cpp:2609-2617): a
	// non-friendly non-positive (or dispel-effect) spell hit stands
	// the target up. The EVADE-miss arm is vacuous (no miss model on
	// this path) and the friendly gate rides the path — the tick only
	// casts hostile AI spells at its attack victim. The AtTargetAttacked
	// half is covered by the combat-state writes in the damage block
	// above (lastCombatTime / UNIT_FLAG_IN_COMBAT); its threat, loot-
	// recipient and PvP legs need a player threat manager, a player
	// caster, or a player owner — none of which a wild creature caster
	// has — documented no-bridge.
	//
	// Spell::TargetInfo::DoDamageAndTriggers AI legs (Spell.cpp:2639-
	// 2650): the caster-side SpellHitTarget Eluna hook — the same arm
	// bridged in castCreatureSpellInternal (boss_ai.go); this inline
	// AI victim-spell path is a second hit site for it. Both arms run
	// after the damage legs above, matching C++ order (damage is dealt
	// before 2609/2639), and both gate on a DBC-known spell the way
	// the castCreatureSpellInternal bridge does.
	if s != nil && s.Data != nil {
		if spellInfo, found, err := s.Data.Spell(spellID); err == nil && found {
			if p.Sess != nil && p.Sess.player != nil && p.Sess.player.StandState != 0 &&
				(!spellIsPositive(spellInfo) || spellHasEffect(spellInfo, spellEffectDispel)) {
				p.Sess.player.StandState = 0
				p.Sess.sendPlayerUpdate()
			}
			s.fireCreatureSpellHitTarget(ctx, motion, p.GUID, spellID)
			// Spell::DoTriggersOnSpellHit (Spell.cpp:2652/2913-2957):
			// the per-hit trigger consumer — the ADD_TARGET_TRIGGER
			// snapshot plus the spell_linked type-1 rows — runs
			// inside DoTargetSpellHit per hit target, ahead of the
			// _cast tail, so it fires here on the inline path too
			// (castCreatureSpellInternal runs the same consumer for
			// its own casts).
			s.fireCreatureHitTriggers(ctx, motion, spellInfo, spellID, []uint64{p.GUID})
			// Spell::_cast spell_linked tail (Spell.cpp:3503-3512):
			// type-0 rows fire at cast completion; the inline path
			// has no _cast, so the tail runs here after the per-hit
			// consumer, matching C++ relative order.
			s.fireCreatureSpellLinkedTriggers(ctx, motion, spellID, p.GUID)
			// Spell::EffectHealthLeech (SpellEffects.cpp:1513-1553) for a
			// creature caster: the caster heals for the damage the target
			// actually lost (leechDealt above) times the effect's
			// CalcValueMultiplier (eff.Amplitude, SpellInfo.cpp:344),
			// then the SpellHealingBonusDone/Taken self folds
			// (Unit.cpp:7505-7632, 7714-7759). The C++ gates on the caster
			// being alive and the unit target existing.
			if spellHasEffect(spellInfo, spellEffectHealthLeech) && motion != nil && leechDealt > 0 {
				leechIdx := -1
				for i := range spellInfo.Effects {
					if spellInfo.Effects[i].Effect == spellEffectHealthLeech {
						leechIdx = i
						break
					}
				}
				// SpellEffectInfo::CalcValueMultiplier (SpellInfo.cpp:528-536):
				// the caster mod-owner's SPELLMOD_VALUE_MULTIPLIER folds via
				// GetSpellModOwner — a pet/totem motion's owner player; a wild
				// or charmed caster resolves nil (the C++ null-modOwner path).
				modOwner := s.spellModOwnerSession(motion.Map, motion.InstanceID, motion.GUID)
				multiplier := calcValueMultiplier(spellInfo, leechIdx, modOwner)
				if heal := effectValueMultiplied(leechDealt, multiplier); heal > 0 {
					// Unit::SpellHealingBonusDone (Unit.cpp:7505-7632): for
					// HEALTH_LEECH spells the flat DoneTotal is zeroed
					// (Unit.cpp:7605-7610), so only the DoneTotalMod percent
					// leg over the caster's 136 auras survives, followed by
					// the mod-owner's SPELLMOD_DAMAGE fold (Unit.cpp:7624;
					// HEAL is not DOT, so the DOT op never applies here).
					heal = uint32(float64(heal) * creatureHealingDonePctMultiplier(s, creatureAuraKeyForMotion(motion), spellInfo))
					if modOwner != nil {
						if sm := modOwner.applySpellMod(spellInfo, uint8(spellModDamage), int32(heal)); sm > 0 {
							heal = uint32(sm)
						} else {
							heal = 0
						}
					}
					// Unit::SpellHealingBonusTaken (Unit.cpp:7714-7759): the
					// direct-heal arms — MOD_HEALING_PCT min/max and the
					// caster-matched MOD_HEALING_RECEIVED — on the caster's
					// own aura store (the heal target is the caster).
					heal = creatureHealingTakenBonus(s, creatureAuraKeyForMotion(motion), motion.GUID, spellInfo, true, heal, false)
					// Spell::EffectHealthLeech (SpellEffects.cpp:1543-1548):
					// the caster self-heal runs through Unit::HealBySpell, so
					// CalcHealAbsorb and sScriptMgr->ModifyHealRecieved —
					// AutoBalance_UnitScript::_Modifer_DealDamage scaling the
					// creature caster's own heal — apply ahead of the health
					// gain.
					leechHeal := s.creatureApplyHealAbsorb(creatureAuraKeyForMotion(motion), uint32(schoolMask), heal)
					scaledHeal := heal - leechHeal
					scaledHeal = s.autoBalanceModifyDealDamageOnMap(motion, motion.Map, scaledHeal)
					// Unit::DealHeal (Unit.cpp:6299-6304): the self-heal fires
					// both AI hooks on the caster motion — C++ has no
					// healer==victim filter on HealReceived/HealDone.
					fireDealHealAIHooks(ctx, s, motion, motion, motion.GUID, motion.GUID, &scaledHeal)
					s.motionMu.Lock()
					if live := s.findCreatureMotionLocked(motion.Map, motion.InstanceID, motion.GUID); live != nil && live.Health > 0 {
						live.Health += scaledHeal
						if live.MaxHealth > 0 && live.Health > live.MaxHealth {
							live.Health = live.MaxHealth
						}
					}
					s.motionMu.Unlock()
				}
			}
		}
	}
}

// applyInlineCreatureSpellHitCreature is the creature-victim twin of
// applyInlineCreatureSpellHit for the inline AI victim-spell path: the
// area-selected enemy creatures from creatureAIAreaEnemyCreatures take
// their own per-hit legs (Spell.cpp:1227 selects every enemy unit, not
// just players). The pipeline mirrors the return-trip pipeline in
// applyInlineReflectReturnTrip (the creature-victim order: taken, armor,
// block, resilience, AutoBalance, resist, absorb) plus the pet-path
// creature-victim legs (split, share, DamageTaken hooks, rage, strips,
// taken procs, stand-up, OnHitBySpell/SpellHitTarget hooks, triggers,
// leech heal). Differences from the player twin are structural, not
// C++-driven: the damage log fans out via broadcastToInstance (a creature
// victim has no session to write to), the tap leg is skipped (a wild
// attacker taps nothing, Unit.cpp:872-876), and the threat/engage legs
// are skipped (the pet branch's threat amount is session-driven and does
// not transfer to a wild caster). The share leg runs through the wire
// victim's session as vehicle when one exists — splitShareDamagePct reads
// no session fields when victimIsPlayer is false (the return-trip
// precedent uses the reflector session the same way) — and the crit-mask
// lookup uses the same vehicle. A damage-immune victim takes nothing and
// sends no log (DoDamageAndTriggers' first arm, Spell.cpp:2538) but still
// runs the proc/stand-up/hook/trigger tail, like the return trip.
func (s *Server) applyInlineCreatureSpellHitCreature(ctx context.Context, motion *creatureMotion, victimGUID uint64, spellID, damage uint32, schoolMask uint8, vehicle *session) {
	if s == nil || motion == nil || victimGUID == 0 || victimGUID == motion.GUID {
		return
	}
	victim := s.findCreatureMotion(motion.Map, motion.InstanceID, victimGUID)
	if victim == nil || victim.Health == 0 {
		return
	}
	victimKey := creatureAuraKeyForMotion(victim)
	casterKey := creatureAuraKeyForMotion(motion)
	victimHealth := victim.Health
	victimLevel := uint8(maxUint32(victim.Level, 1))
	attackerLevel := uint8(maxUint32(motion.Level, 1))
	stats := s.loadCreatureStats(ctx, victim.Entry)
	var inlineSpell wotlk.Spell
	inlineSpellKnown := false
	if s.Data != nil {
		if sp, found, err := s.Data.Spell(spellID); err == nil && found {
			inlineSpell, inlineSpellKnown = sp, true
		}
	}
	var absorbed, resisted, blocked uint32
	isCrit := false
	hitInfo := uint32(0)
	damageImmune := false
	if inlineSpellKnown {
		damageImmune = creatureImmuneToDamageSpell(s, victimKey, inlineSpell, uint32(schoolMask))
	}
	if !damageImmune {
		if damage > 0 && inlineSpellKnown {
			damage = creatureSpellDamageBonusTaken(s, damage, inlineSpell, uint32(schoolMask), victimKey, nil, 1.0)
		}
		isHybrid := schoolMask&spellSchoolMaskNormal != 0 && schoolMask&0x7E != 0
		if damage > 0 && schoolMask&spellSchoolMaskNormal != 0 && !isHybrid &&
			(!inlineSpellKnown || inlineSpell.Attributes&spellAttr0CUIgnoreArmor == 0) {
			armor := s.creatureIgnoreTargetResistArmorForSpell(casterKey, s.liveCreatureArmor(victimKey, victim, float64(stats.Armor)), inlineSpell, inlineSpellKnown)
			damage = calcArmorReducedDamage(armor, attackerLevel, victimLevel, damage)
		}
		// Unit::SpellCritChanceDone (Unit.cpp:7167-7212): wild mobs roll no
		// crits; only a caster with a non-null GetSpellModOwner (a
		// player-owned motion) rolls, mirroring the player twin's arm.
		if damage > 0 && inlineSpellKnown {
			if owner := s.spellModOwnerSession(motion.Map, motion.InstanceID, motion.GUID); owner != nil {
				chance := s.creatureSpellCritChanceDone(casterKey, motion.FlagsExtra&0x20000 != 0, schoolMask, inlineSpell, true)
				if owner.player != nil {
					chance = float64(owner.applySpellMod(inlineSpell, spellModCriticalChance, int32(math.Round(chance*100)))) / 100.0
				}
				chance = s.creatureSpellCritChanceTaken(chance, nil, victimKey, false, motion.GUID, motion.Level, uint32(victimLevel), schoolMask, inlineSpell, s.creatureCasterCanApplyResilience(motion))
				isCrit = chance > 0 && rand.Float64() < chance
				if isCrit {
					var mask uint32
					if vehicle != nil {
						mask, _ = vehicle.targetCreatureTypeMask(ctx, victimGUID)
					}
					if inlineSpell.DefenseType == spellDamageClassMagic {
						damage = s.creatureSpellCriticalDamageBonus(casterKey, uint32(schoolMask), damage, mask, inlineSpell, owner)
					} else {
						damage = s.creatureSpellWeaponCritDamageBonus(casterKey, victimKey, nil, false, uint32(schoolMask), damage, inlineSpell, mask, owner)
					}
					hitInfo = 0x02
				}
			}
		}
		if damage > 0 && inlineSpellKnown {
			victimTarget := combatTarget{
				GUID:        victimGUID,
				Map:         motion.Map,
				InstanceID:  motion.InstanceID,
				X:           victim.X,
				Y:           victim.Y,
				Orientation: victim.Orientation,
				Level:       victimLevel,
			}
			if b, ok := s.spellTakenBlockAmountFrom(victimTarget, false, nil, inlineSpell, damage, attackerLevel, motion.X, motion.Y); ok {
				blocked = b
				damage -= b
			}
		}
		if damage > 0 {
			resilCR := uint8(CombatRatingCritTakenSpell)
			if inlineSpellKnown && inlineSpell.DefenseType != spellDamageClassMagic {
				if spellWeaponAttackType(inlineSpell) == protocol.RangedAttack {
					resilCR = CombatRatingCritTakenRanged
				} else {
					resilCR = CombatRatingCritTakenMelee
				}
			}
			if s.creatureVictimTakesResilience(victim) && s.creatureCasterCanApplyResilience(motion) {
				if ownerSess := s.ownerPlayerSessionOfMotion(victim); ownerSess != nil {
					ownerSess.applyResilienceToDamage(true, &damage, isCrit, resilCR)
				}
			}
		}
		damage = s.autoBalanceModifyDealDamageOnMap(motion, motion.Map, damage)
		preResistDmg := damage
		if damage > 0 && schoolMask > 1 {
			mask123 := uint32(schoolMask)
			if mask123&0x7E != 0 {
				mask123 &^= 1
			}
			var targetResistMod int32
			for _, amount := range s.creatureAuraTypeAmountsByMiscMask(casterKey, spellAuraModTargetResistance, mask123) {
				targetResistMod += amount
			}
			hybridArmorReduced := uint32(0)
			if isHybrid {
				armor := s.creatureIgnoreTargetResistArmorForSpell(casterKey, s.liveCreatureArmor(victimKey, victim, float64(stats.Armor)), inlineSpell, inlineSpellKnown)
				hybridArmorReduced = calcArmorReducedDamage(armor, attackerLevel, victimLevel, damage)
			}
			resistances := s.petFoldedResistances(victimKey, stats.Resistances)
			resisted, damage = calcMagicSpellResistance(damage, schoolMask, resistances, attackerLevel, victimLevel, true, false, 0, spellResistInput{
				ignoreResistances:  inlineSpellKnown && inlineSpell.AttributesEx4&spellAttr4IgnoreResistances != 0,
				binary:             inlineSpellKnown && inlineSpell.AttributesCu&spellAttr0CuBinarySpell != 0,
				hybrid:             isHybrid,
				hybridArmorReduced: hybridArmorReduced,
				targetResistMod:    targetResistMod,
			})
		}
		if damage > 0 {
			bypass := absorbIgnoreBypass(preResistDmg, s.creatureAbsorbIgnorePctForSpell(casterKey, uint32(schoolMask), spellID))
			pool := damage
			if bypass > pool {
				pool = 0
			} else {
				pool -= bypass
			}
			absorbed, damage = s.applyCreatureAbsorptionShields(victimKey, pool, schoolMask)
			damage += bypass
		}
	} else {
		damage = 0
	}
	// Unit::CalcAbsorbResist (Unit.cpp:1977-2060) split legs, then the
	// Unit::DealDamage 300 share leg (Unit.cpp:766-788) — the same order
	// as the player twin.
	if damage > 0 {
		damage = s.splitDamageToCaster(ctx, victimGUID, false, victimKey, motion.GUID, damage, uint32(schoolMask))
	}
	if damage > 0 && vehicle != nil {
		vehicle.splitShareDamagePct(ctx, victimGUID, false, victimKey, motion.GUID, damage, uint32(schoolMask))
	}
	// The log carries the post-split, pre-hook damage (C++ sends it before
	// DealDamage, Spell.cpp:2542); the hooks below may still adjust the
	// damage the proc event and strips see, like the pet branch.
	logDamage := damage
	// Spell::EffectHealthLeech basis (SpellEffects.cpp:1539-1540): the
	// damage actually lost — overkill excluded, post-absorb — captured
	// before the health mutation below.
	leechDealt := uint32(0)
	if !damageImmune {
		leechDealt = damage
		if leechDealt > victimHealth {
			leechDealt = victimHealth
		}
	}
	overkill := uint32(0)
	var killed bool
	var newHealth, maxHealth uint32
	var liveX, liveY, liveZ float32
	var liveFlags uint32
	var rageNext uint32
	var rageChanged bool
	var live *creatureMotion
	s.motionMu.Lock()
	if m := s.findCreatureMotionLocked(motion.Map, motion.InstanceID, victimGUID); m != nil && m.Health > 0 {
		live = m
		if live.BossAI == nil {
			live.BossAI = getBossAIForCreature(live, live.ScriptName)
		}
		fireDealDamageAIHooks(ctx, s, live, motion, motion.GUID, victimGUID, &damage)
		maxHealth = live.MaxHealth
		if damage >= live.Health {
			overkill = damage - live.Health
			live.Health = 0
			live.DynamicFlags |= unitDynFlagLootable
			live.InCombat = false
			live.TargetGUID = 0
			live.Moving = false
			if live.ThreatMgr != nil {
				live.ThreatMgr.ClearThreat()
			}
			killed = true
		} else {
			live.Health -= damage
			if next, changed := s.addCreatureRageLocked(live, live.Level, damage+absorbed); changed {
				rageNext, rageChanged = next, true
			}
		}
		newHealth = live.Health
		liveX, liveY, liveZ = live.X, live.Y, live.Z
		liveFlags = live.DynamicFlags
	}
	s.motionMu.Unlock()
	if live == nil {
		return
	}
	// The inline twins log after the mutation; an immune hit sends no log
	// (Spell.cpp:2538 first arm).
	if !damageImmune {
		logPkt := buildSpellNonMeleeDamageLog(victimGUID, motion.GUID, spellID, logDamage, overkill, schoolMask, absorbed, resisted, hitInfo, blocked)
		s.broadcastToInstance(motion.Map, motion.InstanceID, uint16(protocol.OpcodeSMSG_SPELLNONMELEEDAMAGELOG), logPkt, nil)
	}
	if killed {
		// Unit::DealDamage (Unit.cpp:738-748 + :882-889): the TAKE_DAMAGE
		// strip fires before the lethal check, like the pet branch.
		s.procCreatureLethalDamageAuras(victimKey, spellID, damage, maxHealth)
		s.stopCreatureMotionInInstance(motion.Map, motion.InstanceID, victimGUID, liveX, liveY, liveZ)
		s.broadcastCreatureValuesUpdateInInstance(motion.Map, motion.InstanceID, victimGUID, map[int]uint32{unitFieldHealth: 0, unitFieldDynamicFlags: liveFlags})
		s.broadcastThreatClearInInstance(motion.Map, motion.InstanceID, victimGUID)
	} else {
		healthUpdate := map[int]uint32{unitFieldHealth: newHealth}
		if rageChanged {
			healthUpdate[unitFieldPower1+powerRage] = rageNext
		}
		s.broadcastCreatureValuesUpdateInInstance(motion.Map, motion.InstanceID, victimGUID, healthUpdate)
		// Unit::DealDamage (Unit.cpp:743-748 + 897-898): the TAKE_DAMAGE
		// and DIRECT_DAMAGE strips, like the pet branch.
		s.procCreatureDirectDamageAuras(victimKey, spellID, damage, maxHealth)
	}
	// Spell::TargetInfo::DoDamageAndTriggers (Spell.cpp:2581-2586): the
	// victim-side taken proc half, with PROC_FLAG_TAKEN_DAMAGE ORed by
	// the damage arm (Spell.cpp:2545). Runs on lethal and non-lethal
	// arms alike, like the pet creature branch (the return trip's lethal
	// skip is self-hit-specific: the caster's own auras are stripped
	// before DoDamageAndTriggers runs).
	if inlineSpellKnown && inlineSpell.AttributesEx3&spellAttr3CantTriggerProc == 0 {
		if typeMask := spellDamageTakenProcTypeMask(inlineSpell) | procFlagTakenDamage; typeMask != 0 {
			spellCopy := inlineSpell
			s.fireCreatureAuraProcLoop(ctx, live, motion.GUID, procEventInfo{
				typeMask:       typeMask,
				schoolMask:     uint32(schoolMask),
				spellTypeMask:  procSpellTypeDamage,
				spellPhaseMask: procSpellPhaseHit,
				hitMask:        procHitNormal,
				triggered:      false,
				eventSpell:     &spellCopy,
				actorGUID:      motion.GUID,
				damage:         damage,
			})
		}
	}
	// Spell::TargetInfo::DoDamageAndTriggers AI legs (Spell.cpp:2609-2650)
	// in C++ order: stand-up, the victim-creature SpellHit hook
	// (2629-2638, the _spellHitTarget->ToCreature() gate), the
	// caster-side SpellHitTarget hook, then the per-hit trigger consumer
	// (Spell.cpp:2652).
	if inlineSpellKnown {
		if (!spellIsPositive(inlineSpell) || spellHasEffect(inlineSpell, spellEffectDispel)) && !s.creatureFriendlyToTarget(motion, victimGUID) {
			s.standCreatureMotionUp(live)
		}
		s.fireCreatureLuaEvent(ctx, live, scripting.CreatureEventOnHitBySpell, s.luaMotionCreature(motion), spellID)
		s.fireCreatureSpellHitTarget(ctx, motion, victimGUID, spellID)
		s.fireCreatureHitTriggers(ctx, motion, inlineSpell, spellID, []uint64{victimGUID})
	}
	// Spell::EffectHealthLeech (SpellEffects.cpp:1513-1553) for the
	// creature caster, at the cast-completion position like the player
	// twin: the caster heals for the damage actually lost, through the
	// Done/Taken folds, HealAbsorb, AutoBalance and the DealHeal hooks.
	if inlineSpellKnown && spellHasEffect(inlineSpell, spellEffectHealthLeech) && leechDealt > 0 {
		leechIdx := -1
		for i := range inlineSpell.Effects {
			if inlineSpell.Effects[i].Effect == spellEffectHealthLeech {
				leechIdx = i
				break
			}
		}
		modOwner := s.spellModOwnerSession(motion.Map, motion.InstanceID, motion.GUID)
		multiplier := calcValueMultiplier(inlineSpell, leechIdx, modOwner)
		if heal := effectValueMultiplied(leechDealt, multiplier); heal > 0 {
			heal = uint32(float64(heal) * creatureHealingDonePctMultiplier(s, casterKey, inlineSpell))
			if modOwner != nil {
				if sm := modOwner.applySpellMod(inlineSpell, uint8(spellModDamage), int32(heal)); sm > 0 {
					heal = uint32(sm)
				} else {
					heal = 0
				}
			}
			heal = creatureHealingTakenBonus(s, casterKey, motion.GUID, inlineSpell, true, heal, false)
			leechHeal := s.creatureApplyHealAbsorb(casterKey, uint32(schoolMask), heal)
			scaledHeal := heal - leechHeal
			scaledHeal = s.autoBalanceModifyDealDamageOnMap(motion, motion.Map, scaledHeal)
			fireDealHealAIHooks(ctx, s, motion, motion, motion.GUID, motion.GUID, &scaledHeal)
			s.motionMu.Lock()
			if liveCaster := s.findCreatureMotionLocked(motion.Map, motion.InstanceID, motion.GUID); liveCaster != nil && liveCaster.Health > 0 {
				liveCaster.Health += scaledHeal
				if liveCaster.MaxHealth > 0 && liveCaster.Health > liveCaster.MaxHealth {
					liveCaster.Health = liveCaster.MaxHealth
				}
			}
			s.motionMu.Unlock()
		}
	}
}

// serverInlineReflectVerdict mirrors the AddUnitTarget reflect arm
// (Spell.cpp:2152, 2173-2188; WorldObject::SpellHitResult, Object.cpp:2617-2666)
// for the inline AI victim-spell path (creaturemotion.go). The victim is always
// a player here, rolling the reflect arm via checkSpellReflection — C++ offers
// reflect to any Unit victim, not just players. The offer gate is m_canReflect
// (spellCanBeReflected) plus the !(IsPositive() && m_caster->IsFriendlyTo(target))
// carve-out (Spell.cpp:2152), resolved through the motion's faction
// (isFriendlyFaction) since the caster is a wild creature motion, not a session.
// The this == victim early-out (Object.cpp:2633) sits ahead of the canReflect arm,
// so a self-targeted inline cast never reflects. On success the second return is
// the return-trip ReflectResult (Spell.cpp:2178): the motion's own SpellHitResult
// with canReflect=false, which can only report IMMUNE — the immune arms precede
// the this == victim short-circuit (Object.cpp:2617-2633), the positive arm
// resolves to NONE, and the magic hit roll sits behind the short-circuit and
// never runs. A non-NONE status nulls the return trip (Spell.cpp:2350, 7740-7743):
// the GO packet still reports REFLECT with the status, but the motion takes nothing.
func (s *Server) serverInlineReflectVerdict(ctx context.Context, motion *creatureMotion, spell wotlk.Spell, victim *playerPos) (bool, uint8) {
	if s == nil || motion == nil || victim == nil || victim.GUID == 0 || victim.GUID == motion.GUID {
		return false, protocol.SpellMissNone
	}
	if !spellCanBeReflected(spell) {
		return false, protocol.SpellMissNone
	}
	if spell.AttributesCu&spellAttr0CuNegativeMask == 0 && s.isFriendlyFaction(motion.Faction, *victim) {
		return false, protocol.SpellMissNone
	}
	if victim.Sess == nil || !victim.Sess.checkSpellReflection(spell) {
		return false, protocol.SpellMissNone
	}
	return true, s.serverCreatureReflectReturnStatus(ctx, motion, spell)
}

// serverCreatureReflectReturnStatus is the inline-path form of the ReflectResult
// computation (Spell.cpp:2178): unitCaster->SpellHitResult(unitCaster, spellInfo,
// false) — the motion's own hit result with canReflect=false. Only the immune arms
// can fire: IsImmunedToSpell (the template/effect/mechanic/school folds) and the
// damage-only IsImmunedToDamage arm (spellHasOnlyDamageEffects). The return-trip
// caster IS the motion, so the "target friendly to caster" term of the school fold
// (Unit.cpp:7909) is trivially true — threaded through an always-friendly
// callback, since the motion has no player session to stand in as caster.
func (s *Server) serverCreatureReflectReturnStatus(ctx context.Context, motion *creatureMotion, spell wotlk.Spell) uint8 {
	if s == nil || motion == nil {
		return protocol.SpellMissNone
	}
	key := creatureAuraKeyForMotion(motion)
	stats := s.loadCreatureStats(ctx, motion.Entry)
	if creatureImmuneToSpell(s, key, spell, stats, s.isTotemGUID(motion.GUID), nil, motion.Faction, func(uint32) bool { return true }) {
		return protocol.SpellMissImmune
	}
	if spellHasOnlyDamageEffects(spell) && creatureImmuneToDamageSpell(s, key, spell, spell.SchoolMask) {
		return protocol.SpellMissImmune
	}
	return protocol.SpellMissNone
}

// serverCastCreatureReflectVerdict mirrors the AddUnitTarget reflect arm
// (Spell.cpp:2152, 2173-2188; WorldObject::SpellHitResult, Object.cpp:2617-2666)
// for castCreatureSpellInternal (boss_ai.go): the generic creature-cast
// path, whose victims can be players or creatures (combat picks, self
// buffs, spell_linked triggers) — C++ offers reflect to any Unit victim.
// Player victims roll via checkSpellReflection, creature victims via
// creatureCheckSpellReflection. The offer gate is spellCanBeReflected plus
// the !(IsPositive() && m_caster->IsFriendlyTo(target)) carve-out, resolved
// through creatureFriendlyToTarget on the motion's faction (charm sets it
// to the charmer's faction, Unit.cpp:11830). The this == victim early-out
// (Object.cpp:2633) sits ahead of the canReflect arm, so a self-targeted
// cast never reflects. The second return is the return-trip ReflectResult
// via serverCreatureReflectReturnStatus (IMMUNE arms only); it rides the GO
// packet's miss status — this path applies no damage legs, so there is no
// return-trip funnel to consume it.
// The evaded return reports a creature victim in evade mode:
// WorldObject::SpellHitResult (Object.cpp:2637-2639) returns
// SPELL_MISS_EVADE ahead of the reflect roll, so an evading creature
// never rolls (or consumes) its reflect aura.
func (s *Server) serverCastCreatureReflectVerdict(ctx context.Context, m *creatureMotion, spell wotlk.Spell, targetGUID uint64) (reflected bool, evaded bool, status uint8) {
	if s == nil || m == nil || targetGUID == 0 || targetGUID == m.GUID {
		return false, false, protocol.SpellMissNone
	}
	if !spellCanBeReflected(spell) {
		return false, false, protocol.SpellMissNone
	}
	if spell.AttributesCu&spellAttr0CuNegativeMask == 0 && s.creatureFriendlyToTarget(m, targetGUID) {
		return false, false, protocol.SpellMissNone
	}
	if ts := s.findSessionByGUID(targetGUID); ts != nil {
		if !ts.checkSpellReflection(spell) {
			return false, false, protocol.SpellMissNone
		}
	} else {
		if s.isCreatureEvadingInInstance(m.Map, m.InstanceID, targetGUID) {
			return false, true, protocol.SpellMissNone
		}
		var key creatureAuraKey
		if tm := s.findCreatureMotion(m.Map, m.InstanceID, targetGUID); tm != nil {
			key = creatureAuraKeyForMotion(tm)
		}
		if !s.creatureCheckSpellReflection(key, spell) {
			return false, false, protocol.SpellMissNone
		}
	}
	return true, false, s.serverCreatureReflectReturnStatus(ctx, m, spell)
}

// armCastCreatureReflectProcDelayed mirrors the ProcReflectDelayed arm
// (Spell.cpp:2181) for the castCreatureSpellInternal path: at
// outbound-missile arrival the reflector's taken-side auras proc with
// PROC_HIT_REFLECT. The delay is the missile TimeDelay (dist/speed*1000
// floored, min 5y, 0 for instant — Spell.cpp:2157-2171), measured from the
// motion to the reflector's combat position; the session resolves at fire
// time, the same shape as the inline-path and pet-path bridges. Creature
// reflectors have no Go event-list model, so the taken-side proc loop only
// runs for player reflectors — but the charge arm
// (consumeCreatureReflectAuraCharge) runs for creature reflectors on the
// same timer, matching C++ which schedules ProcReflectDelayed on the
// creature's own event list.
func (s *Server) armCastCreatureReflectProcDelayed(ctx context.Context, motion *creatureMotion, reflectorGUID uint64, spell wotlk.Spell) {
	if s == nil || motion == nil || reflectorGUID == 0 {
		return
	}
	delayMs := 0
	if spell.Speed > 0 {
		dist := 20.0
		if rs := s.findSessionByGUID(reflectorGUID); rs != nil && rs.player != nil {
			dx := float64(rs.player.X - motion.X)
			dy := float64(rs.player.Y - motion.Y)
			dz := float64(rs.player.Z - motion.Z)
			if d := math.Sqrt(dx*dx + dy*dy + dz*dz); d > 5.0 {
				dist = d
			} else {
				dist = 5.0
			}
		} else if tm := s.findCreatureMotion(motion.Map, motion.InstanceID, reflectorGUID); tm != nil {
			dx := float64(tm.X - motion.X)
			dy := float64(tm.Y - motion.Y)
			dz := float64(tm.Z - motion.Z)
			if d := math.Sqrt(dx*dx + dy*dy + dz*dz); d > 5.0 {
				dist = d
			} else {
				dist = 5.0
			}
		}
		delayMs = int(math.Floor(dist / float64(spell.Speed) * 1000.0))
	}
	attackerGUID := motion.GUID
	reflectCreatureKey := creatureAuraKey{Map: motion.Map, InstanceID: motion.InstanceID, GUID: reflectorGUID}
	casterMap, casterInstance := motion.Map, motion.InstanceID
	time.AfterFunc(time.Duration(delayMs)*time.Millisecond, func() {
		if s == nil {
			return
		}
		// ProcReflectDelayed::Execute (Spell.cpp:2077-2078): the arrival
		// proc and the charge drop are skipped when the original caster
		// no longer resolves (ObjectAccessor::GetUnit null arm) —
		// despawned creatures leave the motion map.
		if s.findCreatureMotion(casterMap, casterInstance, attackerGUID) == nil {
			return
		}
		if rsess := s.findSessionByGUID(reflectorGUID); rsess != nil {
			rsess.procSpellReflectTakenAuraTriggers(context.Background(), attackerGUID, spell, false)
			// ProcReflectDelayed charge arm (Spell.cpp:2181).
			rsess.consumeReflectAuraCharge()
		} else {
			s.consumeCreatureReflectAuraCharge(reflectCreatureKey)
		}
	})
}

// inlineReflectReflector carries the reflector-side legs of an inline
// reflect return trip: the stand-up target is the reflector, the damage log
// goes to the reflector's viewers, and a player reflector takes kill credit.
// A creature reflector stands via standCreatureMotionUp, its log fans out via
// broadcastToInstance (it has no session to write to, like the creature-victim
// twin), and it takes no kill credit (the creature-victim twin credits
// nobody either); the share leg runs through shareVehicle, which reads no
// session fields when victimIsPlayer is false.
type inlineReflectReflector struct {
	standUp      func()
	deliverLog   func(payload []byte)
	killCredit   func(ctx context.Context, target combatTarget)
	shareVehicle *session
}

// applyInlineReflectReturnTripCore runs the reflect return trip for the inline
// AI victim-spell path (Spell::PreprocessTarget, Spell.cpp:2350, and
// DoEffectOnLaunchTarget, Spell.cpp:7740-7743): with ReflectResult == NONE the
// reflected spell's effects land on the caster motion instead of the reflector.
// C++ deals the caster's own spell back through Unit::DealDamage; the Go twin
// reduces the motion's health under motionMu, broadcasts the new health, writes
// the return-trip damage log to the reflector, and runs the standard creature
// death chain on a lethal hit (the rage.go share-damage funnel shape). The
// return-trip damage is the already-computed hit damage — C++ recalculates it,
// but the inline path computes damage once per cast and the pet-path return trip
// reuses the same value. Deviation: C++ delivers the return trip 1.5x TimeDelay
// after the outbound missile (Spell.cpp:2184); Go lands it synchronously like
// the pet-path bridge.
func (s *Server) applyInlineReflectReturnTripCore(ctx context.Context, motion *creatureMotion, spell wotlk.Spell, damage uint32, schoolMask uint8, reflector inlineReflectReflector) {
	if s == nil || motion == nil {
		return
	}
	// Spell::TargetInfo::DoDamageAndTriggers (Spell.cpp:2609-2617,
	// 2639-2650) on the return trip: the stand-up arm fires on
	// TargetInfo::unit — the reflector, not _spellHitTarget (the
	// caster) — so a non-positive (or dispel-effect) reflected hit
	// stands the reflector up; the friendly gate rides the path
	// (the tick only casts hostile AI spells at its attack
	// victim), as on the direct hit. The caster-side SpellHitTarget
	// Eluna hook fires with _spellHitTarget, the motion itself
	// (Spell.cpp:2631-2632), which fireCreatureSpellHitTarget
	// resolves through findCreatureMotion. Neither leg gates on
	// damage or on the caster surviving — C++ runs both after
	// DealDamage unconditionally (only MissCondition != EVADE and
	// a non-null _spellHitTarget gate them) — so the closure runs
	// on the lethal, immune and zero-damage paths too.
	standUpReflectorAndFireHook := func() {
		if reflector.standUp != nil {
			reflector.standUp()
		}
		if spell.ID != 0 {
			s.fireCreatureSpellHitTarget(ctx, motion, motion.GUID, spell.ID)
		}
	}
	// Spell::EffectInstaKill (SpellEffects.cpp:274-296) on the return
	// trip: the reflected spell's effects land on the caster
	// (PreprocessTarget, Spell.cpp:2350; DoProcessTargetContainer runs
	// the effect dispatch ahead of DoDamageAndTriggers, Spell.cpp:3555-
	// 3566), so a reflected effect-1 spell kills the motion through the
	// shared creature-caster twin (boss_ai.go) instead of dealing it the
	// outbound fallback damage. Runs ahead of the damage pipeline below:
	// C++ kills through DealDamage NODAMAGE with no damage log, and a
	// lethal trip skips the proc/trigger legs that follow (the kill
	// strips the caster's auras first). A zero-damage trip with no
	// effect-1 still returns — the old damage==0 guard.
	if spell.ID != 0 && spellHasEffect(spell, spellEffectInstaKill) {
		s.creatureEffectInstaKill(ctx, motion, spell.ID, []uint64{motion.GUID}, nil, nil)
		if live := s.findCreatureMotion(motion.Map, motion.InstanceID, motion.GUID); live == nil || live.Health == 0 {
			return
		}
	} else if damage == 0 {
		// A zero-damage return trip still runs the full
		// DoDamageAndTriggers tail: C++ takes the !hasDamage &&
		// !hasHealing branch (Spell.cpp:2565-2575) and fires
		// NODAMAGE procs (PROC_SPELL_TYPE_NO_DMG_HEAL, no
		// PROC_FLAG_TAKEN_DAMAGE — the taken-damage flag is ORed
		// only by the damage arm), then the stand-up and hook legs
		// (neither gates on damage), then the trigger consumer.
		// Only the damage pipeline itself is skipped. This is the
		// reflected-non-damage-spell path: creatureSpellDamage
		// returns 0 for spells with no damage effect, so a
		// reflected debuff lands here, not in the pipeline.
		if spell.AttributesEx3&spellAttr3CantTriggerProc == 0 {
			typeMask := spellDamageProcTypeMask(spell) | spellDamageTakenProcTypeMask(spell)
			if typeMask != 0 {
				spellCopy := spell
				s.fireCreatureAuraProcLoop(ctx, motion, motion.GUID, procEventInfo{
					typeMask:       typeMask,
					schoolMask:     uint32(schoolMask),
					spellTypeMask:  procSpellTypeNoDmgHeal,
					spellPhaseMask: procSpellPhaseHit,
					hitMask:        procHitNormal,
					triggered:      false,
					eventSpell:     &spellCopy,
					actorGUID:      motion.GUID,
					damage:         0,
				})
			}
		}
		standUpReflectorAndFireHook()
		if spell.ID != 0 {
			s.fireCreatureHitTriggers(ctx, motion, spell, spell.ID, []uint64{motion.GUID})
		}
		return
	}
	// Spell::TargetInfo::DoDamageAndTriggers (Spell.cpp:2527-2560): the
	// return trip is a full direct-damage hit on the caster itself — the
	// attacker (m_originalCaster) and the victim (the retargeted
	// _spellHitTarget, Spell.cpp:2350) are both the motion, so the
	// CalculateSpellDamageTaken pipeline runs on the motion's own
	// legs, in the pet-path creature-victim order (pet_combat.go):
	// taken multiplier, armor, block, resilience, AutoBalance, then
	// the CalcAbsorbResist tail (resist, absorb). The old code dealt
	// the raw base damage with zero absorbed/resisted in the log.
	motionKey := creatureAuraKeyForMotion(motion)
	stats := s.loadCreatureStats(ctx, motion.Entry)
	// DoDamageAndTriggers' first arm (Spell.cpp:2538-2546): a
	// damage-immune victim takes nothing and sends no log. The
	// spell-immune trips never reach here — a non-NONE ReflectResult
	// already nulled them in the verdict — so this only catches the
	// mixed-effect damage-immune edge. An immune trip still runs the
	// proc loop below, like C++'s trigger block on the zeroed hit.
	damageImmune := creatureImmuneToDamageSpell(s, motionKey, spell, uint32(schoolMask))
	var absorbed, resisted, blocked uint32
	if !damageImmune {
		if spell.ID != 0 {
			damage = creatureSpellDamageBonusTaken(s, damage, spell, uint32(schoolMask), motionKey, nil, 1.0)
		}
		isHybrid := schoolMask&spellSchoolMaskNormal != 0 && schoolMask&0x7E != 0
		if damage > 0 && schoolMask&spellSchoolMaskNormal != 0 && !isHybrid &&
			(spell.ID == 0 || spell.Attributes&spellAttr0CUIgnoreArmor == 0) {
			armor := s.creatureIgnoreTargetResistArmorForSpell(motionKey, s.liveCreatureArmor(motionKey, motion, float64(stats.Armor)), spell, spell.ID != 0)
			damage = calcArmorReducedDamage(armor, uint8(maxUint32(motion.Level, 1)), uint8(motion.Level), damage)
		}
		if damage > 0 && spell.ID != 0 {
			// Position.cpp:122-124: a unit is always in its own arc,
			// so the self-attacker sits in front by definition; the
			// helper's arc test cannot express identity, hence the
			// one-yard facing offset on the attacker position.
			victimTarget := combatTarget{
				GUID:        motion.GUID,
				Map:         motion.Map,
				InstanceID:  motion.InstanceID,
				X:           motion.X,
				Y:           motion.Y,
				Orientation: motion.Orientation,
				Level:       uint8(motion.Level),
			}
			attackerX := motion.X + float32(math.Cos(float64(motion.Orientation)))
			attackerY := motion.Y + float32(math.Sin(float64(motion.Orientation)))
			if b, ok := s.spellTakenBlockAmountFrom(victimTarget, false, nil, spell, damage, uint8(maxUint32(motion.Level, 1)), attackerX, attackerY); ok {
				blocked = b
				damage -= b
			}
		}
		if damage > 0 {
			resilCR := uint8(CombatRatingCritTakenSpell)
			if spell.ID != 0 && spell.DefenseType != spellDamageClassMagic {
				if spellWeaponAttackType(spell) == protocol.RangedAttack {
					resilCR = CombatRatingCritTakenRanged
				} else {
					resilCR = CombatRatingCritTakenMelee
				}
			}
			if s.creatureVictimTakesResilience(motion) && s.creatureCasterCanApplyResilience(motion) {
				if ownerSess := s.ownerPlayerSessionOfMotion(motion); ownerSess != nil {
					ownerSess.applyResilienceToDamage(true, &damage, false, resilCR)
				}
			}
		}
		damage = s.autoBalanceModifyDealDamageOnMap(motion, motion.Map, damage)
		preResistDmg := damage
		if damage > 0 && schoolMask > 1 {
			mask123 := uint32(schoolMask)
			if mask123&0x7E != 0 {
				mask123 &^= 1
			}
			var targetResistMod int32
			for _, amount := range s.creatureAuraTypeAmountsByMiscMask(motionKey, spellAuraModTargetResistance, mask123) {
				targetResistMod += amount
			}
			hybridArmorReduced := uint32(0)
			if isHybrid {
				armor := s.creatureIgnoreTargetResistArmorForSpell(motionKey, s.liveCreatureArmor(motionKey, motion, float64(stats.Armor)), spell, spell.ID != 0)
				hybridArmorReduced = calcArmorReducedDamage(armor, uint8(maxUint32(motion.Level, 1)), uint8(motion.Level), damage)
			}
			resistances := s.petFoldedResistances(motionKey, stats.Resistances)
			resisted, damage = calcMagicSpellResistance(damage, schoolMask, resistances, uint8(maxUint32(motion.Level, 1)), uint8(motion.Level), true, false, 0, spellResistInput{
				ignoreResistances:  spell.ID != 0 && spell.AttributesEx4&spellAttr4IgnoreResistances != 0,
				binary:             spell.ID != 0 && spell.AttributesCu&spellAttr0CuBinarySpell != 0,
				hybrid:             isHybrid,
				hybridArmorReduced: hybridArmorReduced,
				targetResistMod:    targetResistMod,
			})
		}
		if damage > 0 {
			bypass := absorbIgnoreBypass(preResistDmg, s.creatureAbsorbIgnorePctForSpell(motionKey, uint32(schoolMask), spell.ID))
			pool := damage
			if bypass > pool {
				pool = 0
			} else {
				pool -= bypass
			}
			absorbed, damage = s.applyCreatureAbsorptionShields(motionKey, pool, schoolMask)
			damage += bypass
		}
		// CalcAbsorbResist's split legs are vacuous here: the split
		// helper early-returns when victim == attacker, and both are
		// the motion. The 300 share leg is not: it runs on the
		// post-absorb damage via the reflector's share vehicle
		// (victimIsPlayer=false, so no session fields are
		// read), mirroring the inline direct-hit order (share ahead
		// of the health mutation).
		if damage > 0 && reflector.shareVehicle != nil {
			reflector.shareVehicle.splitShareDamagePct(ctx, motion.GUID, false, motionKey, motion.GUID, damage, uint32(schoolMask))
		}
	} else {
		damage = 0
	}
	// Spell::EffectHealthLeech (SpellEffects.cpp:1513-1553) on the
	// return trip: the leech effect runs in the effect dispatch
	// (DoTargetSpellHit), ahead of DoDamageAndTriggers, so the
	// caster heals for the damage it is about to take — before the
	// health mutation below, not after it as a tail. That order is
	// observable here because caster == victim: a leech heal can
	// keep a would-be-lethal trip non-lethal, and the MaxHealth cap
	// applies to the pre-damage health. The basis mirrors the
	// direct hit: the post-pipeline damage actually lost (overkill
	// excluded, post-absorb), so an immune trip (damage zeroed
	// above) heals nothing. Folds match the direct-hit leech block
	// (CalcValueMultiplier, Done pct, Taken bonus, HealAbsorb,
	// AutoBalance, DealHeal AI hooks).
	if spell.ID != 0 && spellHasEffect(spell, spellEffectHealthLeech) && damage > 0 {
		leechIdx := -1
		for i := range spell.Effects {
			if spell.Effects[i].Effect == spellEffectHealthLeech {
				leechIdx = i
				break
			}
		}
		leechDealt := damage
		if leechDealt > motion.Health {
			leechDealt = motion.Health
		}
		if leechDealt > 0 {
			modOwner := s.spellModOwnerSession(motion.Map, motion.InstanceID, motion.GUID)
			multiplier := calcValueMultiplier(spell, leechIdx, modOwner)
			if heal := effectValueMultiplied(leechDealt, multiplier); heal > 0 {
				heal = uint32(float64(heal) * creatureHealingDonePctMultiplier(s, creatureAuraKeyForMotion(motion), spell))
				if modOwner != nil {
					if sm := modOwner.applySpellMod(spell, uint8(spellModDamage), int32(heal)); sm > 0 {
						heal = uint32(sm)
					} else {
						heal = 0
					}
				}
				heal = creatureHealingTakenBonus(s, creatureAuraKeyForMotion(motion), motion.GUID, spell, true, heal, false)
				leechHeal := s.creatureApplyHealAbsorb(creatureAuraKeyForMotion(motion), uint32(schoolMask), heal)
				scaledHeal := heal - leechHeal
				scaledHeal = s.autoBalanceModifyDealDamageOnMap(motion, motion.Map, scaledHeal)
				fireDealHealAIHooks(ctx, s, motion, motion, motion.GUID, motion.GUID, &scaledHeal)
				s.motionMu.Lock()
				if live := s.findCreatureMotionLocked(motion.Map, motion.InstanceID, motion.GUID); live != nil && live.Health > 0 {
					live.Health += scaledHeal
					if live.MaxHealth > 0 && live.Health > live.MaxHealth {
						live.Health = live.MaxHealth
					}
				}
				s.motionMu.Unlock()
			}
		}
	}
	s.motionMu.Lock()
	live := s.findCreatureMotionLocked(motion.Map, motion.InstanceID, motion.GUID)
	if live == nil || live.Health == 0 {
		s.motionMu.Unlock()
		return
	}
	healthBefore := live.Health
	killed := damage >= live.Health
	if killed {
		live.Health = 0
		live.DynamicFlags |= unitDynFlagLootable
		live.InCombat = false
		live.TargetGUID = 0
		live.Moving = false
		if live.ThreatMgr != nil {
			live.ThreatMgr.ClearThreat()
		}
	} else {
		live.Health -= damage
	}
	mapID, instanceID, guid := live.Map, live.InstanceID, live.GUID
	x, y, z := live.X, live.Y, live.Z
	newHealth := live.Health
	s.motionMu.Unlock()
	overkill := uint32(0)
	if killed {
		overkill = damage - healthBefore
	}
	if !damageImmune {
		logPacket := buildSpellNonMeleeDamageLog(guid, guid, spell.ID, damage, overkill, schoolMask, absorbed, resisted, 0, blocked)
		if reflector.deliverLog != nil {
			reflector.deliverLog(logPacket)
		}
	}
	if killed {
		s.stopCreatureMotionInInstance(mapID, instanceID, guid, x, y, z)
		s.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid, map[int]uint32{
			unitFieldHealth:       0,
			unitFieldDynamicFlags: unitDynFlagLootable,
		})
		s.broadcastThreatClearInInstance(mapID, instanceID, guid)
		if reflector.killCredit != nil {
			reflector.killCredit(ctx, combatTarget{GUID: guid, Map: mapID, InstanceID: instanceID})
		}
		// Lethal trips still run the stand-up and hook legs: the
		// stand-up target is the reflector (alive), and the hook
		// has no caster-alive gate in C++ (Spell.cpp:2631-2632).
		standUpReflectorAndFireHook()
		return
	}
	if !damageImmune {
		s.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid, map[int]uint32{unitFieldHealth: newHealth})
	}
	// Spell::TargetInfo::DoDamageAndTriggers (Spell.cpp:2581-2586): the
	// return trip's _spellHitTarget is the caster itself (Spell.cpp:2350),
	// so Unit::ProcSkillsAndAuras(caster, caster, procAttacker, procVictim,
	// PROC_SPELL_TYPE_DAMAGE, PROC_SPELL_PHASE_HIT, ...) evaluates the
	// motion's own auras with the done-side prefill (the
	// prepareDataForTriggerSystem fill, spellDamageProcTypeMask), the
	// taken-side prefill (spellDamageTakenProcTypeMask) plus
	// PROC_FLAG_TAKEN_DAMAGE ORed by the damage arm (Spell.cpp:2545) —
	// the parked done-side self-proc leg. The canEffectTrigger gate
	// (Spell.cpp:2440) drops out here: the attr3 arm is checked inline
	// (the creature loop has no attr3 gate), CanExecuteTriggersOnHit
	// folds into the path's non-zero-effect model, and the motion has no
	// procDeep model (standing limitation, same as the _cast leg). The
	// loop fires once with the combined mask — C++ runs the done and
	// taken calls separately, so an aura carrying both done AND taken
	// flags could proc twice there and procs once here; no DBC aura
	// carries both for one spell today. Lethal trips skip this: C++
	// strips the caster's auras in Unit::Kill before DoDamageAndTriggers
	// runs, so the event evaluates an empty set (net no-op); the
	// KILL/KILLED/DEATH legs (Unit.cpp:11283-11290) are a standing gap —
	// no creature death path in Go fires them.
	if spell.AttributesEx3&spellAttr3CantTriggerProc == 0 {
		typeMask := spellDamageProcTypeMask(spell) | spellDamageTakenProcTypeMask(spell) | procFlagTakenDamage
		if typeMask != 0 {
			spellCopy := spell
			s.fireCreatureAuraProcLoop(ctx, live, guid, procEventInfo{
				typeMask:       typeMask,
				schoolMask:     uint32(schoolMask),
				spellTypeMask:  procSpellTypeDamage,
				spellPhaseMask: procSpellPhaseHit,
				hitMask:        procHitNormal,
				triggered:      false,
				eventSpell:     &spellCopy,
				actorGUID:      guid,
				damage:         damage,
			})
		}
	}
	// C++ order (Spell.cpp:2609 precedes 2639 precedes 2652): the
	// stand-up and hook legs run between the proc loop above and
	// the trigger consumer below.
	standUpReflectorAndFireHook()
	// Spell::TargetInfo::DoDamageAndTriggers (Spell.cpp:2652): the
	// return trip's _spellHitTarget is the caster itself (Spell.cpp:2350),
	// so the per-hit trigger consumer runs on the motion, like the
	// castCreatureSpellInternal retarget (boss_ai.go) and unlike the old
	// return trip, which ran the proc loop but no trigger consumer — a
	// reflected hit fired no ADD_TARGET_TRIGGER or spell_linked type-1
	// row. Lethal trips never reach here (the kill branch above returns),
	// and a nulled trip never reaches this function (the verdict gate).
	if spell.ID != 0 {
		s.fireCreatureHitTriggers(ctx, motion, spell, spell.ID, []uint64{motion.GUID})
	}
}

// applyInlineReflectReturnTrip runs the reflect return trip for a player
// reflector on the inline AI victim-spell path: the stand-up arm fires on
// TargetInfo::unit — the reflector, not _spellHitTarget (the caster) — so a
// non-positive (or dispel-effect) reflected hit stands the reflector up; the
// friendly gate rides the path (the tick only casts hostile AI spells at its
// attack victim), as on the direct hit. The log goes to the reflector's
// session and fans out nearby; a lethal trip credits the reflector's session.
func (s *Server) applyInlineReflectReturnTrip(ctx context.Context, motion *creatureMotion, reflector *playerPos, spell wotlk.Spell, damage uint32, schoolMask uint8) {
	if s == nil || motion == nil || reflector == nil {
		return
	}
	s.applyInlineReflectReturnTripCore(ctx, motion, spell, damage, schoolMask, inlineReflectReflector{
		standUp: func() {
			if spell.ID != 0 && (!spellIsPositive(spell) || spellHasEffect(spell, spellEffectDispel)) {
				if reflector.Sess != nil && reflector.Sess.player != nil && reflector.Sess.player.StandState != 0 {
					reflector.Sess.player.StandState = 0
					reflector.Sess.sendPlayerUpdate()
				}
			}
		},
		deliverLog: func(payload []byte) {
			if reflector.Sess != nil {
				_ = reflector.Sess.write(uint16(protocol.OpcodeSMSG_SPELLNONMELEEDAMAGELOG), payload, true)
			}
			s.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELLNONMELEEDAMAGELOG), payload, reflector.Sess)
		},
		killCredit: func(ctx context.Context, target combatTarget) {
			if reflector.Sess != nil {
				reflector.Sess.onCreatureKilled(ctx, target, nil)
			}
		},
		shareVehicle: reflector.Sess,
	})
}

// applyInlineCreatureReflectReturnTrip runs the reflect return trip for a
// creature reflector on the inline AI victim-spell path — the area-selected
// enemy creature whose reflect roll succeeded (serverCastCreatureReflectVerdict).
// It shares the player trip's core, so the reflected spell's effect-1 instakill,
// zero-damage NODAMAGE tail, damage pipeline, lethal stand-up/hook, proc loop
// and trigger consumer all land on the caster motion exactly as for a player
// reflector; only the reflector-side legs differ. The stand-up arm mirrors the
// creature-victim twin (non-positive or dispel-effect, not friendly to the
// caster, resolved at fire time so a despawned reflector simply stands
// nothing up); the log fans out via broadcastToInstance; no kill credit (a
// creature reflector credits no player); the share leg runs through the wire
// victim's session as vehicle, like the creature-victim twin.
func (s *Server) applyInlineCreatureReflectReturnTrip(ctx context.Context, motion *creatureMotion, reflectorGUID uint64, spell wotlk.Spell, damage uint32, schoolMask uint8, vehicle *session) {
	if s == nil || motion == nil || reflectorGUID == 0 {
		return
	}
	s.applyInlineReflectReturnTripCore(ctx, motion, spell, damage, schoolMask, inlineReflectReflector{
		standUp: func() {
			if spell.ID == 0 || (spellIsPositive(spell) && !spellHasEffect(spell, spellEffectDispel)) {
				return
			}
			if s.creatureFriendlyToTarget(motion, reflectorGUID) {
				return
			}
			if tm := s.findCreatureMotion(motion.Map, motion.InstanceID, reflectorGUID); tm != nil {
				s.standCreatureMotionUp(tm)
			}
		},
		deliverLog: func(payload []byte) {
			s.broadcastToInstance(motion.Map, motion.InstanceID, uint16(protocol.OpcodeSMSG_SPELLNONMELEEDAMAGELOG), payload, nil)
		},
		shareVehicle: vehicle,
	})
}

// armInlineReflectProcDelayed mirrors the ProcReflectDelayed arm (Spell.cpp:2181)
// for the inline AI victim-spell path: at outbound-missile arrival the reflector's
// taken-side auras proc with PROC_HIT_REFLECT. C++ schedules it on the target's
// event list with delay TimeDelay (0 for instant spells, Spell.cpp:2157-2171);
// Go arms the same timer and resolves the reflector's session at fire time, the
// same shape as the player-path doReflect and the pet-path bridge. Creature
// reflectors have no Go event-list model, so the arm only runs for player
// reflectors — and every victim on this path is a player.
func (s *Server) armInlineReflectProcDelayed(motion *creatureMotion, reflector *playerPos, spell wotlk.Spell) {
	if s == nil || motion == nil || reflector == nil || reflector.Sess == nil {
		return
	}
	delayMs := 0
	if spell.Speed > 0 {
		dx := float64(reflector.X - motion.X)
		dy := float64(reflector.Y - motion.Y)
		dz := float64(reflector.Z - motion.Z)
		dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
		if dist < 5.0 {
			dist = 5.0
		}
		delayMs = int(math.Floor(dist / float64(spell.Speed) * 1000.0))
	}
	attackerGUID := motion.GUID
	reflectorGUID := reflector.GUID
	casterMap, casterInstance := motion.Map, motion.InstanceID
	time.AfterFunc(time.Duration(delayMs)*time.Millisecond, func() {
		if s == nil {
			return
		}
		// ProcReflectDelayed::Execute (Spell.cpp:2077-2078): skip the
		// arrival proc and charge drop when the original caster no longer
		// resolves (ObjectAccessor::GetUnit null arm).
		if s.findCreatureMotion(casterMap, casterInstance, attackerGUID) == nil {
			return
		}
		if rsess := s.findSessionByGUID(reflectorGUID); rsess != nil {
			rsess.procSpellReflectTakenAuraTriggers(context.Background(), attackerGUID, spell, false)
			// ProcReflectDelayed charge arm (Spell.cpp:2181).
			rsess.consumeReflectAuraCharge()
		}
	})
}
