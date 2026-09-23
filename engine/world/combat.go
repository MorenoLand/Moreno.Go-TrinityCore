package world

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"math/rand/v2"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	meleeAttackRange               = 5.0
	creatureFlagExtraNoParryHasten = 0x00000008
	attackDisplayDelay             = 200 * time.Millisecond
)

// haveOffhandWeapon checks if the player has an offhand weapon equipped.
func (s *session) haveOffhandWeapon() bool {
	if s == nil || s.player == nil {
		return false
	}
	return s.player.OffhandAttackTime > 0 || s.player.MaxOffhandDamage > 0
}

// calcParryHastedRemaining computes new remaining attack swing time when a defender parries,
// matching TrinityCore Unit.cpp:1480-1510.
// If remaining time is <= 20% of weapon speed, it is unchanged.
// If remaining time is between 20% and 60%, it is set to 20%.
// If remaining time is > 60%, it is reduced by 40% (2 * 20%).
func calcParryHastedRemaining(remaining, attackTime time.Duration) time.Duration {
	if attackTime <= 0 || remaining <= 0 {
		return remaining
	}
	percent20 := attackTime / 5
	percent60 := 3 * percent20
	if remaining > percent20 && remaining <= percent60 {
		return percent20
	}
	if remaining > percent60 {
		hasted := remaining - 2*percent20
		if hasted < percent20 {
			return percent20
		}
		return hasted
	}
	return remaining
}

type combatTarget struct {
	GUID        uint64
	Map         uint32
	X           float32
	Y           float32
	Z           float32
	Orientation float32
	Health      uint32
	MaxHealth   uint32
	Armor       uint32
	Resistances [7]uint32
	MinDamage   float32
	MaxDamage   float32
	Level       uint8
	UnitFlags   uint32
	FlagsExtra  uint32
	Faction     uint32
	CombatReach float32
}

// calcMeleeRange computes maximum melee attack distance between attacker and target,
// matching TrinityCore Unit::GetMeleeRange (Unit.cpp:614-618):
// max(attacker.CombatReach + target.CombatReach + 4.0/3.0, NOMINAL_MELEE_RANGE)
func calcMeleeRange(attackerReach, victimReach float32) float64 {
	if attackerReach <= 0 {
		attackerReach = 1.5
	}
	if victimReach <= 0 {
		victimReach = 1.5
	}
	rangeVal := float64(attackerReach + victimReach + 4.0/3.0)
	if rangeVal < 5.0 {
		return 5.0
	}
	return rangeVal
}

func (s *session) getCombatTarget(ctx context.Context, guid uint64) (combatTarget, bool) {
	if ctx == nil || ctx.Err() != nil {
		ctx = context.Background()
	}
	// Check if target is an online player (e.g. duel opponent or PvP target)
	if s.server != nil {
		if playerSess := s.server.findSessionByGUID(guid); playerSess != nil && playerSess.player != nil {
			reach := playerSess.player.CombatReach
			if reach <= 0 {
				reach = 1.5
			}
			return combatTarget{
				GUID:        guid,
				Map:         playerSess.player.Map,
				X:           playerSess.player.X,
				Y:           playerSess.player.Y,
				Z:           playerSess.player.Z,
				Orientation: playerSess.player.Orientation,
				Health:      playerSess.player.Health,
				MaxHealth:   playerSess.player.MaxHealth,
				Armor:       playerSess.player.Armor,
				Resistances: playerSess.player.Resistances,
				MinDamage:   playerSess.player.MinDamage,
				MaxDamage:   playerSess.player.MaxDamage,
				Level:       playerSess.player.Level,
				UnitFlags:   playerSess.player.UnitFlags,
				FlagsExtra:  0,
				Faction:     0,
				CombatReach: reach,
			}, true
		}
	}
	s.server.motionMu.Lock()
	if s.server.creatureMotion != nil {
		motion := s.server.creatureMotion[guid]
		if motion == nil {
			low := uint32(guid & 0x00FFFFFF)
			entry := uint32((guid >> 24) & 0x00FFFFFF)
			stdKey := creatureWorldGUID(low, entry)
			motion = s.server.creatureMotion[stdKey]
		}
		if motion != nil {
			reach := motion.CombatReach
			if reach <= 0 {
				reach = 1.5
			}
			target := combatTarget{
				GUID:        guid,
				Map:         motion.Map,
				X:           motion.X,
				Y:           motion.Y,
				Z:           motion.Z,
				Orientation: motion.Orientation,
				Health:      motion.Health,
				MaxHealth:   motion.MaxHealth,
				Armor:       motion.Armor,
				Resistances: motion.Resistances,
				MinDamage:   motion.MinDamage,
				MaxDamage:   motion.MaxDamage,
				Level:       uint8(motion.Level),
				UnitFlags:   motion.UnitFlags,
				FlagsExtra:  motion.FlagsExtra,
				Faction:     motion.Faction,
				CombatReach: reach,
			}
			s.server.motionMu.Unlock()
			return target, true
		}
	}
	s.server.motionMu.Unlock()

	target, err := s.loadCombatTarget(ctx, guid)
	if err != nil {
		return combatTarget{}, false
	}
	return target, true
}

func (s *session) handleAttackSwing(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	if s.isDeadOrGhost() {
		s.attackTarget = 0
		return true
	}
	if s.player.UnitFlags&(unitFlagConfused|unitFlagFleeing) != 0 || s.hasAuraType(spellAuraCharm) {
		s.attackTarget = 0
		return true
	}
	reader := protocol.NewReader(payload)
	victim, err := reader.ReadU64()
	if err != nil {
		s.debug("attack rejected", "account", s.accountName, "error", err)
		return false
	}
	target, ok := s.getCombatTarget(ctx, victim)
	if !ok {
		s.debug("attack target not found", "account", s.accountName, "victim", victim)
		return true
	}
	if target.Health == 0 {
		s.attackTarget = 0
		return s.write(uint16(protocol.OpcodeSMSG_ATTACK_SWING_DEAD_TARGET), nil, true) == nil
	}
	if creatureCombatDisabled(target.UnitFlags, target.FlagsExtra) {
		s.attackTarget = 0
		return s.sendAttackStop(victim, false) == nil
	}
	if target.Map != s.player.Map {
		s.attackTarget = 0
		return s.sendAttackStop(victim, false) == nil
	}
	if s.attackTarget != 0 && s.attackTarget != victim {
		if err := s.sendAttackStop(s.attackTarget, false); err != nil {
			return false
		}
	}
	s.attackTarget = victim
	s.lastCombatTime = time.Now()
	if s.player != nil && s.player.UnitFlags&unitFlagInCombat == 0 {
		s.player.UnitFlags |= unitFlagInCombat
		s.sendPlayerUpdate()
	}
	s.debug("attack started", "account", s.accountName, "guid", victim)
	startPayload := buildAttackStart(s.playerGUID, victim)
	_ = s.write(uint16(protocol.OpcodeSMSG_ATTACK_START), startPayload, true)
	if s.server != nil {
		s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_ATTACK_START), startPayload, s)
	}
	reach := float32(1.5)
	if s.player.CombatReach > 0 {
		reach = s.player.CombatReach
	}
	allowedRange := calcMeleeRange(reach, target.CombatReach) + 2.0
	if distance3D(s.player.X, s.player.Y, s.player.Z, target.X, target.Y, target.Z) <= allowedRange {
		s.executeMeleeSwing(ctx, target, protocol.BaseAttack)
	}
	// If dual wielding, delay offhand swing by 50% of base attack time (TrinityCore Unit.cpp:5745)
	if s.haveOffhandWeapon() {
		baseSpeed := time.Duration(s.player.AttackTime) * time.Millisecond
		if baseSpeed <= 0 {
			baseSpeed = 2 * time.Second
		}
		offSpeed := time.Duration(s.player.OffhandAttackTime) * time.Millisecond
		if offSpeed <= 0 {
			offSpeed = 2 * time.Second
		}
		s.lastOffhandSwing = time.Now().Add(-(offSpeed - baseSpeed/2))
	}
	return true
}

func (s *session) executeMeleeSwing(ctx context.Context, target combatTarget, attType protocol.WeaponAttackType) {
	if s.player == nil || s.isDeadOrGhost() || target.Health == 0 {
		return
	}
	now := time.Now()
	var minDmg, maxDmg float32
	var attTime uint32

	if attType == protocol.OffAttack {
		s.lastOffhandSwing = now
		minDmg = s.player.MinOffhandDamage
		maxDmg = s.player.MaxOffhandDamage
		attTime = s.player.OffhandAttackTime
	} else {
		s.lastSwing = now
		minDmg = s.player.MinDamage
		maxDmg = s.player.MaxDamage
		attTime = s.player.AttackTime
	}

	damage := uint32(20 + int(s.player.Level)*5)
	if maxDmg > minDmg && minDmg > 0 {
		attSpeed := float64(attTime) / 1000.0
		if attSpeed <= 0 {
			attSpeed = 2.0
		}
		apBonus := (float64(s.player.AttackPower) * attSpeed) / 14.0
		variance := float64(maxDmg - minDmg)
		baseDmg := float64(minDmg)
		if variance > 0 {
			baseDmg += rand.Float64() * variance
		}
		damage = uint32(baseDmg + apBonus)
	}

	// Off-hand weapon attacks suffer a 50% damage penalty by default (TrinityCore Unit.cpp:353)
	if attType == protocol.OffAttack {
		damage = uint32(math.Round(float64(damage) * 0.5))
	}

	// Apply target armor damage reduction
	if target.Armor > 0 {
		damage = calcArmorReducedDamage(float64(target.Armor), s.player.Level, damage, s.getArmorPenPct())
	}
	if damage < 1 {
		damage = 1
	}

	isPlayerVictim := s.server != nil && s.server.findSessionByGUID(target.GUID) != nil
	outcome := protocol.MeleeHitNormal
	hitInfo := protocol.HitInfoAffectsVictim
	targetState := protocol.VictimStateHit

	isDualWielding := s.haveOffhandWeapon()
	canBlock := false
	canParry := target.Level >= 10 || isPlayerVictim
	canDodge := true
	var critReductionBP int32
	if isPlayerVictim && s.server != nil {
		if vicSess := s.server.findSessionByGUID(target.GUID); vicSess != nil && vicSess.player != nil {
			canBlock = vicSess.player.Block > 0
			critChanceBP := int32(500)
			vicSess.applyResilienceToMeleeCritChance(true, CombatRatingCritTakenMelee, &critChanceBP)
			critReductionBP = 500 - critChanceBP
		}
	}

	// Positional defense checks (TrinityCore Unit::RollMeleeOutcomeAgainst):
	// A defender can only parry or block attacks from within their front 180° arc (M_PI).
	// Player victims cannot dodge attacks from behind. (NPCs can dodge from behind).
	attackerInFront := hasInArc(target.Orientation, target.X, target.Y, s.player.X, s.player.Y, math.Pi)
	if !attackerInFront {
		canBlock = false
		canParry = false
		if isPlayerVictim {
			canDodge = false
		}
	}

	if s.player.Level > 0 {
		hitBonusBP := int32(math.Round(s.getMeleeHitPct() * 100))
		critBonusBP := int32(math.Round(s.getMeleeCritPct() * 100))
		expertiseBP := int32(math.Round(s.getExpertiseDodgeParryReductionPct() * 100))
		outcome, hitInfo, targetState = rollMeleeOutcome(s.player.Level, target.Level, true, isPlayerVictim, isDualWielding, canBlock, canParry, canDodge, critReductionBP, hitBonusBP, critBonusBP, expertiseBP)
	}
	if isPlayerVictim && s.server != nil {
		if vicSess := s.server.findSessionByGUID(target.GUID); vicSess != nil {
			if vicSess.isImmuneToDamage(1) {
				outcome = protocol.MeleeHitImmune
				hitInfo = protocol.HitInfoMiss
				targetState = protocol.VictimStateIsImmune
			}
		}
	} else if !isPlayerVictim && s.server != nil && s.server.isCreatureEvading(target.GUID) {
		outcome = protocol.MeleeHitEvade
		hitInfo = protocol.HitInfoMiss
		targetState = protocol.VictimStateEvades
	}
	if attType == protocol.OffAttack {
		hitInfo |= protocol.HitInfoOffHand
	}
	blocked := uint32(0)

	switch outcome {
	case protocol.MeleeHitMiss, protocol.MeleeHitDodge, protocol.MeleeHitParry, protocol.MeleeHitEvade, protocol.MeleeHitImmune:
		damage = 0
	case protocol.MeleeHitBlock:
		blocked = damage / 4
		if blocked < 1 {
			blocked = 1
		}
		damage -= blocked
	case protocol.MeleeHitCrit:
		damage *= 2
	case protocol.MeleeHitGlancing:
		damage = uint32(float64(damage) * 0.75)
		if damage < 1 {
			damage = 1
		}
	case protocol.MeleeHitCrushing:
		damage = uint32(float64(damage) * 1.5)
	}

	if isPlayerVictim && s.server != nil {
		if vicSess := s.server.findSessionByGUID(target.GUID); vicSess != nil {
			vicSess.applyResilienceToDamage(true, &damage, outcome == protocol.MeleeHitCrit, CombatRatingCritTakenMelee)
			if damage > 0 {
				absorbed, remaining := vicSess.applyAbsorptionShields(damage, 1)
				damage = remaining
				if remaining == 0 && absorbed > 0 {
					hitInfo |= protocol.HitInfoFullAbsorb
				} else if absorbed > 0 {
					hitInfo |= protocol.HitInfoPartialAbsorb
				}
			}
		}
	} else if !isPlayerVictim && s.server != nil && damage > 0 {
		absorbed, remaining := s.server.applyCreatureAbsorptionShields(target.GUID, damage, 1)
		damage = remaining
		if remaining == 0 && absorbed > 0 {
			hitInfo |= protocol.HitInfoFullAbsorb
		} else if absorbed > 0 {
			hitInfo |= protocol.HitInfoPartialAbsorb
		}
	}

	// Handle parry haste: if defender parried, haste defender's next attack!
	// Matching TrinityCore Unit.cpp:1480-1510.
	if targetState == protocol.VictimStateParry && s.server != nil {
		if playerVictim := s.server.findSessionByGUID(target.GUID); playerVictim != nil && playerVictim.player != nil {
			vMainSpeed := time.Duration(playerVictim.player.AttackTime) * time.Millisecond
			if vMainSpeed <= 0 {
				vMainSpeed = 2 * time.Second
			}
			elapsed := now.Sub(playerVictim.lastSwing)
			if elapsed < vMainSpeed {
				rem := vMainSpeed - elapsed
				hasted := calcParryHastedRemaining(rem, vMainSpeed)
				playerVictim.lastSwing = now.Add(-(vMainSpeed - hasted))
			}
		} else {
			s.server.motionMu.Lock()
			if motion := s.server.creatureMotion[target.GUID]; motion != nil {
				if motion.FlagsExtra&creatureFlagExtraNoParryHasten == 0 {
					cSpeed := time.Duration(motion.AttackTime) * time.Millisecond
					if cSpeed <= 0 {
						cSpeed = 2 * time.Second
					}
					elapsed := now.Sub(motion.LastAttack)
					if elapsed < cSpeed {
						rem := cSpeed - elapsed
						hasted := calcParryHastedRemaining(rem, cSpeed)
						motion.LastAttack = now.Add(-(cSpeed - hasted))
					}
				}
			}
			s.server.motionMu.Unlock()
		}
	}

	overkill := uint32(0)
	if damage >= target.Health && target.Health > 0 {
		overkill = damage - target.Health
	}
	asuPayload := protocol.BuildAttackerStateUpdate(s.playerGUID, target.GUID, damage, overkill, hitInfo, targetState, blocked)
	_ = s.write(uint16(protocol.OpcodeSMSG_ATTACKERSTATEUPDATE), asuPayload, true)
	if s.server != nil {
		s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_ATTACKERSTATEUPDATE), asuPayload, s)
	}

	// Attacker enters combat on swing
	if s.player != nil && s.player.UnitFlags&unitFlagInCombat == 0 {
		s.player.UnitFlags |= unitFlagInCombat
	}
	s.lastCombatTime = time.Now()

	// Trigger weapon enchantment and trinket procs on hit (TrinityCore Unit::ProcDamageAndSpellFor)
	s.procWeaponEnchantments(ctx, target, attType, outcome)
	s.procItemAndTrinketEffects(ctx, target, attType, outcome)

	if s.server != nil {
		s.server.triggerPetDefensive(s.playerGUID, target.GUID)
	}

	// If target is an online player (e.g. duel opponent or PvP)
	if s.server != nil {
		if playerSess := s.server.findSessionByGUID(target.GUID); playerSess != nil && playerSess.player != nil {
			if playerSess.player.UnitFlags&unitFlagInCombat == 0 {
				playerSess.player.UnitFlags |= unitFlagInCombat
			}
			playerSess.lastCombatTime = time.Now()
			if damage > 0 {
				if damage >= playerSess.player.Health {
					if s.duelPartner == target.GUID && s.player.DuelTeam != 0 {
						// Duel defeat: loser drops to 1 HP and kneels (TC: Player::DuelComplete)
						playerSess.player.Health = 1
						playerSess.sendPlayerUpdate()
						s.endDuel(true, s.playerGUID, false)
					} else {
						playerSess.player.Health = 0
						playerSess.sendPlayerUpdate()
						s.server.creditHonorableKill(s, playerSess)
						playerSess.killPlayer(ctx)
					}
					_ = s.sendAttackStop(target.GUID, true)
					s.attackTarget = 0
				} else {
					playerSess.player.Health -= damage
					playerSess.delayCurrentCast()
					playerSess.delayCurrentChannel()
					playerSess.procDamageAuras(true, damage)
					playerSess.sendPlayerUpdate()
				}
			}
			return
		}
	}

	if damage == 0 {
		if s.server != nil && (isPlayerVictim || !s.server.isCreatureEvading(target.GUID)) {
			s.server.triggerCreatureAggro(ctx, target.GUID, s.playerGUID)
		}
		return
	}

	low := uint32(target.GUID & 0x00FFFFFF)
	entry := uint32((target.GUID >> 24) & 0x00FFFFFF)
	stdKey := creatureWorldGUID(low, entry)

	if damage >= target.Health {
		// Target dies
		s.server.motionMu.Lock()
		motion := s.server.creatureMotion[target.GUID]
		if motion == nil {
			motion = s.server.creatureMotion[stdKey]
		}
		if motion != nil {
			motion.Health = 0
			motion.InCombat = false
			motion.TargetGUID = 0
			motion.Moving = false
			if motion.ThreatMgr != nil {
				motion.ThreatMgr.ClearThreat()
			}
		}
		s.server.motionMu.Unlock()

		s.server.stopCreatureMotion(target.Map, target.GUID, target.X, target.Y, target.Z)
		s.server.broadcastCreatureValuesUpdate(target.Map, target.GUID, map[int]uint32{
			unitFieldHealth:       0,
			unitFieldDynamicFlags: 1, // UNIT_DYNFLAG_LOOTABLE
		})
		s.server.broadcastThreatClear(target.Map, target.GUID)
		_ = s.sendAttackStop(target.GUID, true)
		s.attackTarget = 0
		s.onCreatureKilled(ctx, target)
		s.debug("target slain by auto-attack", "account", s.accountName, "guid", target.GUID)
	} else {
		newHealth := target.Health - damage
		s.server.motionMu.Lock()
		motion := s.server.creatureMotion[target.GUID]
		if motion == nil {
			motion = s.server.creatureMotion[stdKey]
		}
		if motion != nil {
			motion.Health = newHealth
			if motion.ThreatMgr == nil {
				motion.ThreatMgr = NewThreatManager(target.GUID)
			}
			if motion.BossAI == nil {
				motion.BossAI = getBossAIForCreature(motion, motion.ScriptName)
			}
			dist := distance3D(s.player.X, s.player.Y, s.player.Z, motion.X, motion.Y, motion.Z)
			inMelee := dist <= meleeAttackRange
			threat := float32(damage) * s.getThreatMultiplier(1)
			switched, newVictim := motion.ThreatMgr.AddThreat(s.playerGUID, threat, inMelee)
			if switched && newVictim != motion.TargetGUID {
				motion.TargetGUID = newVictim
				entries := motion.ThreatMgr.SortedEntries()
				s.server.broadcastHighestThreatUpdate(motion.Map, motion.GUID, newVictim, entries)
			}
			if motion.BossAI != nil {
				motion.BossAI.OnDamageTaken(ctx, s.server, motion, s.playerGUID, damage)
			}
		}
		s.server.motionMu.Unlock()

		s.server.broadcastCreatureValuesUpdate(target.Map, target.GUID, map[int]uint32{
			unitFieldHealth: newHealth,
		})
		s.server.procCreatureDamageAuras(target.GUID, true, damage, target.MaxHealth)
		s.server.triggerCreatureAggro(ctx, target.GUID, s.playerGUID)
	}
}

func (s *session) executeRangedAttack(ctx context.Context, target combatTarget, spellID uint32) {
	if s.player == nil || s.isDeadOrGhost() || target.Health == 0 {
		return
	}
	now := time.Now()
	s.lastRangedSwing = now

	// Consume ammo for hunter bow/gun/crossbow (Spell 75) (TC Spell::TakeAmmo)
	if spellID == 75 && s.player.AmmoID > 0 && s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		ammoEntry := s.player.AmmoID
		var itemGUID, count int64
		err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT ii.guid, ii.count FROM character_inventory ci JOIN item_instance ii ON ci.item = ii.guid WHERE ci.guid = ? AND ii.itemEntry = ? AND ii.count > 0 LIMIT 1", s.playerGUID, ammoEntry).Scan(&itemGUID, &count)
		if err == nil {
			if count <= 1 {
				_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "DELETE FROM character_inventory WHERE item = ?", itemGUID)
				_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "DELETE FROM item_instance WHERE guid = ?", itemGUID)
				s.player.AmmoID = 0
				_ = s.calculatePlayerStats(ctx, s.player)
				s.sendPlayerUpdate()
				s.autoRepeatSpell = 0
				s.autoRepeatTarget = 0
				buf := protocol.NewBuffer(9)
				buf.WritePackedGUID(s.playerGUID)
				_ = s.write(uint16(protocol.OpcodeSMSG_CANCEL_AUTO_REPEAT), buf.Bytes(), true)
				_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(1, spellID, 75), true) // SPELL_FAILED_NO_AMMO = 75
			} else {
				_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE item_instance SET count = count - 1 WHERE guid = ?", itemGUID)
			}
			s.refreshQuestItemCounts(ctx, ammoEntry, false)
		}
	}

	// Visual: broadcast SMSG_SPELL_GO (TC Spell::SendSpellGo)
	castID := uint8(1)
	castTimeStamp := uint32(now.UnixMilli())
	hitTargets := []uint64{target.GUID}
	spellTarget := protocol.SpellTargetData{Flags: protocol.SpellTargetFlagUnitWireMask, UnitGUID: target.GUID}
	goPkt := protocol.BuildSpellGo(s.playerGUID, s.playerGUID, castID, spellID, spellCastFlagGo, castTimeStamp, hitTargets, nil, spellTarget)
	_ = s.write(uint16(protocol.OpcodeSMSG_SPELL_GO), goPkt, true)
	if s.server != nil {
		s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELL_GO), goPkt, s)
		s.server.triggerPetDefensive(s.playerGUID, target.GUID)
	}

	// Outcome: ranged attacks can be dodged or blocked, but cannot be parried (TC rollMeleeOutcome)
	isPlayerVictim := s.server != nil && s.server.findSessionByGUID(target.GUID) != nil
	canBlock := false
	canDodge := true
	var critReductionBP int32
	if isPlayerVictim && s.server != nil {
		if vicSess := s.server.findSessionByGUID(target.GUID); vicSess != nil && vicSess.player != nil {
			canBlock = vicSess.player.Block > 0
			critChanceBP := int32(500)
			vicSess.applyResilienceToMeleeCritChance(true, CombatRatingCritTakenRanged, &critChanceBP)
			critReductionBP = 500 - critChanceBP
		}
	}
	attackerInFront := hasInArc(target.Orientation, target.X, target.Y, s.player.X, s.player.Y, math.Pi)
	if !attackerInFront {
		canBlock = false
		if isPlayerVictim {
			canDodge = false
		}
	}
	hitBonusBP := int32(math.Round(s.getRangedHitPct() * 100))
	critBonusBP := int32(math.Round(s.getRangedCritPct() * 100))
	expertiseBP := int32(math.Round(s.getExpertiseDodgeParryReductionPct() * 100))
	outcome, _, _ := rollMeleeOutcome(s.player.Level, target.Level, true, isPlayerVictim, false, canBlock, false, canDodge, critReductionBP, hitBonusBP, critBonusBP, expertiseBP)
	if isPlayerVictim && s.server != nil {
		if vicSess := s.server.findSessionByGUID(target.GUID); vicSess != nil {
			if vicSess.isImmuneToDamage(1) {
				outcome = protocol.MeleeHitImmune
			}
		}
	} else if !isPlayerVictim && s.server != nil && s.server.isCreatureEvading(target.GUID) {
		outcome = protocol.MeleeHitEvade
	}

	minDmg := s.player.MinRangedDamage
	maxDmg := s.player.MaxRangedDamage
	if maxDmg < minDmg || minDmg <= 0 {
		minDmg = 1.0
		maxDmg = 2.0
	}
	damage := uint32(minDmg + rand.Float32()*(maxDmg-minDmg))
	if damage < 1 {
		damage = 1
	}

	blocked := uint32(0)
	switch outcome {
	case protocol.MeleeHitMiss, protocol.MeleeHitDodge, protocol.MeleeHitParry, protocol.MeleeHitEvade, protocol.MeleeHitImmune:
		damage = 0
	case protocol.MeleeHitBlock:
		blocked = damage / 4
		if blocked < 1 {
			blocked = 1
		}
		damage -= blocked
	case protocol.MeleeHitCrit:
		damage *= 2
	}

	if isPlayerVictim && s.server != nil {
		if vicSess := s.server.findSessionByGUID(target.GUID); vicSess != nil {
			vicSess.applyResilienceToDamage(true, &damage, outcome == protocol.MeleeHitCrit, CombatRatingCritTakenRanged)
		}
	}

	schoolMask := uint8(1) // Physical default
	if s.server != nil && s.server.Data != nil {
		if spellInfo, found, err := s.server.Data.Spell(spellID); err == nil && found && spellInfo.SchoolMask != 0 {
			schoolMask = uint8(spellInfo.SchoolMask)
		}
	}
	// Apply target armor damage reduction for physical ranged attacks
	if schoolMask == 1 && target.Armor > 0 && damage > 0 {
		damage = calcArmorReducedDamage(float64(target.Armor), s.player.Level, damage, s.getArmorPenPct())
	}

	absorbed := uint32(0)
	if isPlayerVictim && s.server != nil && damage > 0 {
		if vicSess := s.server.findSessionByGUID(target.GUID); vicSess != nil {
			absorbed, damage = vicSess.applyAbsorptionShields(damage, schoolMask)
		}
	} else if !isPlayerVictim && s.server != nil && damage > 0 {
		absorbed, damage = s.server.applyCreatureAbsorptionShields(target.GUID, damage, schoolMask)
	}

	overkill := uint32(0)
	if damage >= target.Health && target.Health > 0 {
		overkill = damage - target.Health
	}
	logPkt := buildSpellNonMeleeDamageLog(target.GUID, s.playerGUID, spellID, damage, overkill, schoolMask, absorbed)
	_ = s.write(uint16(protocol.OpcodeSMSG_SPELLNONMELEEDAMAGELOG), logPkt, true)
	if s.server != nil {
		s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELLNONMELEEDAMAGELOG), logPkt, s)
	}

	if s.player.UnitFlags&unitFlagInCombat == 0 {
		s.player.UnitFlags |= unitFlagInCombat
		s.sendPlayerUpdate()
	}
	s.lastCombatTime = now

	if isPlayerVictim && s.server != nil {
		if vicSess := s.server.findSessionByGUID(target.GUID); vicSess != nil && vicSess.player != nil {
			if vicSess.player.UnitFlags&unitFlagInCombat == 0 {
				vicSess.player.UnitFlags |= unitFlagInCombat
			}
			vicSess.lastCombatTime = now
			if damage > 0 {
				s.server.updateArenaDamageScore(s, damage)
				if damage >= vicSess.player.Health {
					if arena := s.server.findArenaState(s.player.Map, 0); arena != nil {
						arena.mu.Lock()
						if sc, ok := arena.Scores[s.playerGUID]; ok {
							sc.KillingBlows++
						}
						arena.mu.Unlock()
					}
					vicSess.player.Health = 0
					vicSess.sendPlayerUpdate()
					if s.server != nil {
						s.server.creditHonorableKill(s, vicSess)
					}
					vicSess.killPlayer(ctx)
					s.server.handleWGPlayerDeath(vicSess, s)
					s.autoRepeatSpell = 0
					s.autoRepeatTarget = 0
					buf := protocol.NewBuffer(9)
					buf.WritePackedGUID(s.playerGUID)
					_ = s.write(uint16(protocol.OpcodeSMSG_CANCEL_AUTO_REPEAT), buf.Bytes(), true)
				} else {
					vicSess.player.Health -= damage
					vicSess.delayCurrentCast()
					vicSess.delayCurrentChannel()
					vicSess.procDamageAuras(true, damage)
					vicSess.sendPlayerUpdate()
				}
			}
		}
		return
	}

	low := uint32(target.GUID & 0x00FFFFFF)
	entry := uint32((target.GUID >> 24) & 0x00FFFFFF)
	stdKey := creatureWorldGUID(low, entry)

	if damage >= target.Health {
		// Target dies
		s.server.motionMu.Lock()
		motion := s.server.creatureMotion[target.GUID]
		if motion == nil {
			motion = s.server.creatureMotion[stdKey]
		}
		if motion != nil {
			motion.Health = 0
			motion.InCombat = false
			motion.TargetGUID = 0
			motion.Moving = false
			if motion.ThreatMgr != nil {
				motion.ThreatMgr.ClearThreat()
			}
		}
		s.server.motionMu.Unlock()

		s.server.stopCreatureMotion(target.Map, target.GUID, target.X, target.Y, target.Z)
		s.server.broadcastCreatureValuesUpdate(target.Map, target.GUID, map[int]uint32{
			unitFieldHealth:       0,
			unitFieldDynamicFlags: 1, // UNIT_DYNFLAG_LOOTABLE
		})
		s.server.broadcastThreatClear(target.Map, target.GUID)
		s.autoRepeatSpell = 0
		s.autoRepeatTarget = 0
		buf := protocol.NewBuffer(9)
		buf.WritePackedGUID(s.playerGUID)
		_ = s.write(uint16(protocol.OpcodeSMSG_CANCEL_AUTO_REPEAT), buf.Bytes(), true)
		s.onCreatureKilled(ctx, target)
		s.debug("target slain", "account", s.accountName, "guid", target.GUID)
	} else {
		if !isPlayerVictim && s.server != nil && s.server.isCreatureEvading(target.GUID) {
			return
		}
		newHealth := target.Health - damage
		s.server.motionMu.Lock()
		motion := s.server.creatureMotion[target.GUID]
		if motion == nil {
			motion = s.server.creatureMotion[stdKey]
		}
		if motion != nil {
			motion.Health = newHealth
			if motion.ThreatMgr == nil {
				motion.ThreatMgr = NewThreatManager(target.GUID)
			}
			if motion.BossAI == nil {
				motion.BossAI = getBossAIForCreature(motion, motion.ScriptName)
			}
			threat := float32(damage) * s.getThreatMultiplier(uint32(schoolMask))
			switched, newVictim := motion.ThreatMgr.AddThreat(s.playerGUID, threat, false)
			if switched && newVictim != motion.TargetGUID {
				motion.TargetGUID = newVictim
				entries := motion.ThreatMgr.SortedEntries()
				s.server.broadcastHighestThreatUpdate(motion.Map, motion.GUID, newVictim, entries)
			}
			if motion.BossAI != nil {
				motion.BossAI.OnDamageTaken(ctx, s.server, motion, s.playerGUID, damage)
			}
		}
		s.server.motionMu.Unlock()

		s.server.broadcastCreatureValuesUpdate(target.Map, target.GUID, map[int]uint32{
			unitFieldHealth: newHealth,
		})
		s.server.triggerCreatureAggro(ctx, target.GUID, s.playerGUID)
	}
}

func (s *session) handleAttackStop() bool {
	if !s.playerLoaded {
		return true
	}
	victim := s.attackTarget
	s.attackTarget = 0
	if s.autoRepeatSpell != 0 {
		s.autoRepeatSpell = 0
		s.autoRepeatTarget = 0
		buf := protocol.NewBuffer(9)
		buf.WritePackedGUID(s.playerGUID)
		_ = s.write(uint16(protocol.OpcodeSMSG_CANCEL_AUTO_REPEAT), buf.Bytes(), true)
	}
	if err := s.sendAttackStop(victim, false); err != nil {
		s.debug("attack stop failed", "account", s.accountName, "error", err)
		return false
	}
	s.debug("attack stopped", "account", s.accountName, "guid", victim)
	return true
}

func (s *session) stopPvPCombatForSanctuary() {
	if s == nil || s.player == nil || s.server == nil || s.duelPartner != 0 || s.attackTarget == 0 {
		return
	}
	opponent := s.server.findSessionByGUID(s.attackTarget)
	if opponent == nil || opponent == s || !opponent.playerLoaded || opponent.player == nil {
		return
	}
	victim := s.attackTarget
	s.attackTarget = 0
	_ = s.sendAttackStop(victim, false)
	s.player.UnitFlags &^= unitFlagInCombat
	s.sendPlayerUpdate()
	if opponent.attackTarget == s.playerGUID {
		opponent.attackTarget = 0
		_ = opponent.sendAttackStop(s.playerGUID, false)
		if opponent.player != nil {
			opponent.player.UnitFlags &^= unitFlagInCombat
			opponent.sendPlayerUpdate()
		}
	}
}

func (s *session) handleSetSheathed(payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 4 {
		return true
	}
	reader := protocol.NewReader(payload)
	state, err := reader.ReadU32()
	if err != nil {
		return false
	}
	s.player.SheathState = uint8(state)
	s.sendPlayerUpdate()
	s.debug("sheath state changed", "account", s.accountName, "state", state)
	return true
}

func (s *session) sendAttackStop(victim uint64, nowDead bool) error {
	payload := buildAttackStop(s.playerGUID, victim, nowDead)
	_ = s.write(uint16(protocol.OpcodeSMSG_ATTACK_STOP), payload, true)
	if s.server != nil {
		s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_ATTACK_STOP), payload, s)
	}
	return nil
}

func buildAttackStart(attacker, victim uint64) []byte {
	packet := protocol.NewBuffer(16)
	packet.WriteU64(attacker)
	packet.WriteU64(victim)
	return packet.Bytes()
}

func buildAttackStop(attacker, victim uint64, nowDead bool) []byte {
	packet := protocol.NewBuffer(24)
	packet.WritePackedGUID(attacker)
	packet.WritePackedGUID(victim)
	if nowDead {
		packet.WriteU32(1)
	} else {
		packet.WriteU32(0)
	}
	return packet.Bytes()
}

// calcArmorReducedDamage computes physical damage reduction based on victim armor,
// attacker level, and optional attacker armor penetration percentage (ArP).
// Matching TrinityCore Unit::CalcArmorReducedDamage (Unit.cpp:1600-1650).
func calcArmorReducedDamage(armor float64, attackerLevel uint8, damage uint32, armorPenPct ...float64) uint32 {
	if armor <= 0 || damage == 0 {
		return damage
	}
	levelModifier := float64(attackerLevel)
	if levelModifier < 1 {
		levelModifier = 1
	}
	if levelModifier > 59.0 {
		levelModifier += 4.5 * (levelModifier - 59.0)
	}

	effectiveArmor := armor
	if len(armorPenPct) > 0 && armorPenPct[0] > 0 {
		arp := armorPenPct[0]
		if arp > 100.0 {
			arp = 100.0
		}
		// WotLK 3.3.5 Armor Penetration cap formula:
		// maxArmorPen = (armor + 400.0 + 85.0 * levelModifier) / 3.0
		// Reference: TrinityCore Unit::CalcArmorReducedDamage
		maxArmorPen := (armor + 400.0 + 85.0*levelModifier) / 3.0
		penetrated := math.Min(armor, maxArmorPen) * (arp / 100.0)
		effectiveArmor -= penetrated
		if effectiveArmor < 0 {
			effectiveArmor = 0
		}
	}

	damageReduction := 0.1 * effectiveArmor / (8.5*levelModifier + 40.0)
	damageReduction /= (1.0 + damageReduction)
	if damageReduction < 0 {
		damageReduction = 0
	} else if damageReduction > 0.75 {
		damageReduction = 0.75
	}
	reduced := uint32(math.Round(float64(damage) * (1.0 - damageReduction)))
	if reduced < 1 {
		reduced = 1
	}
	return reduced
}

type creatureStats struct {
	Level           uint32
	Health          uint32
	MaxHealth       uint32
	Armor           uint32
	Resistances     [7]uint32
	MinDamage       float32
	MaxDamage       float32
	AttackTime      uint32
	BoundingRadius  float32
	CombatReach     float32
	UnitFlags       uint32
	FlagsExtra      uint32
	ReactState      uint8
	ReactStateKnown bool
}

func (s *Server) loadCreatureStats(ctx context.Context, entry uint32) creatureStats {
	if s == nil {
		return creatureStats{
			Level:          1,
			Health:         100,
			MaxHealth:      100,
			Armor:          10,
			MinDamage:      1.0,
			MaxDamage:      2.0,
			AttackTime:     2000,
			BoundingRadius: 0.306349,
			CombatReach:    1.5,
			ReactState:     creatureReactAggressive,
		}
	}
	s.statsMu.RLock()
	if s.creatureStatsCache != nil {
		if st, ok := s.creatureStatsCache[entry]; ok {
			s.statsMu.RUnlock()
			return st
		}
	}
	s.statsMu.RUnlock()

	stats := creatureStats{
		Level:          1,
		Health:         100,
		MaxHealth:      100,
		Armor:          10,
		MinDamage:      1.0,
		MaxDamage:      2.0,
		AttackTime:     2000,
		BoundingRadius: 0.306349,
		CombatReach:    1.5,
		ReactState:     creatureReactAggressive,
	}
	if s.WorldStore == nil || s.WorldStore.DB == nil {
		return stats
	}

	var maxlevel, unitClass, exp, baseAttackTime, unitFlags, flagsExtra int64
	var healthMod, armorMod, damageMod float64

	row := s.WorldStore.DB.QueryRowContext(ctx, `SELECT 
		COALESCE(maxlevel, 1), 
		COALESCE(unit_class, 1), 
		COALESCE(exp, 0), 
		COALESCE(BaseAttackTime, 2000), 
		COALESCE(HealthModifier, 1.0), 
		COALESCE(ArmorModifier, 1.0), 
		COALESCE(DamageModifier, 1.0),
		COALESCE(unit_flags, 0),
		COALESCE(flags_extra, 0)
		FROM creature_template WHERE entry = ?`, entry)
	if err := row.Scan(&maxlevel, &unitClass, &exp, &baseAttackTime, &healthMod, &armorMod, &damageMod, &unitFlags, &flagsExtra); err != nil {
		return stats
	}
	if reactState, known := s.loadCreatureReaction(ctx, entry); known {
		stats.ReactState = reactState
		stats.ReactStateKnown = true
	}

	if maxlevel < 1 {
		maxlevel = 1
	}
	if unitClass <= 0 {
		unitClass = 1
	}
	if baseAttackTime <= 0 {
		baseAttackTime = 2000
	}
	if healthMod <= 0 {
		healthMod = 1.0
	}
	if armorMod <= 0 {
		armorMod = 1.0
	}
	if damageMod <= 0 {
		damageMod = 1.0
	}

	stats.Level = uint32(maxlevel)
	stats.AttackTime = uint32(baseAttackTime)
	stats.UnitFlags = uint32(unitFlags)
	stats.FlagsExtra = uint32(flagsExtra)

	// Fallback values based on level
	fallbackHealth := uint32(maxlevel * 30)
	if fallbackHealth < 42 {
		fallbackHealth = 42
	}
	stats.Health = fallbackHealth
	stats.MaxHealth = fallbackHealth
	stats.Armor = uint32(maxlevel * 10)
	attSpeed := float32(baseAttackTime) / 1000.0
	stats.MinDamage = float32(maxlevel) * 0.75 * attSpeed
	stats.MaxDamage = float32(maxlevel) * 1.25 * attSpeed

	// Query creature_classlevelstats
	var basehp0, basehp1, basehp2, basearmor int64
	var dmgBase, dmgExp1, dmgExp2 float64
	err := s.WorldStore.DB.QueryRowContext(ctx, `SELECT 
		basehp0, basehp1, basehp2, basearmor, damage_base, damage_exp1, damage_exp2 
		FROM creature_classlevelstats WHERE level = ? AND class = ?`, maxlevel, unitClass).
		Scan(&basehp0, &basehp1, &basehp2, &basearmor, &dmgBase, &dmgExp1, &dmgExp2)
	if err == nil {
		var selectedHP int64
		var selectedDmg float64
		switch {
		case exp >= 2:
			selectedHP = basehp2
			selectedDmg = dmgExp2
		case exp == 1:
			selectedHP = basehp1
			selectedDmg = dmgExp1
		default:
			selectedHP = basehp0
			selectedDmg = dmgBase
		}
		if selectedHP > 0 {
			hp := uint32(math.Ceil(float64(selectedHP) * healthMod))
			if hp > 0 {
				stats.Health = hp
				stats.MaxHealth = hp
			}
		}
		if basearmor > 0 {
			stats.Armor = uint32(math.Ceil(float64(basearmor) * armorMod))
		}
		if selectedDmg > 0 {
			dmg := float32(selectedDmg * damageMod)
			if dmg > 0 {
				stats.MinDamage = dmg
				stats.MaxDamage = dmg * 1.5
			}
		}
	}
	if stats.MinDamage < 1.0 {
		stats.MinDamage = 1.0
	}
	if stats.MaxDamage < stats.MinDamage {
		stats.MaxDamage = stats.MinDamage + 1.0
	}

	var modelBoundingRadius, modelCombatReach sql.NullFloat64
	_ = s.WorldStore.DB.QueryRowContext(ctx, "SELECT cmi.BoundingRadius, cmi.CombatReach FROM creature_template ct JOIN creature_model_info cmi ON ct.modelid1 = cmi.DisplayID WHERE ct.entry = ?", entry).Scan(&modelBoundingRadius, &modelCombatReach)
	if modelBoundingRadius.Valid && modelBoundingRadius.Float64 > 0 {
		stats.BoundingRadius = float32(modelBoundingRadius.Float64)
	}
	if modelCombatReach.Valid && modelCombatReach.Float64 > 0 {
		stats.CombatReach = float32(modelCombatReach.Float64)
	}

	s.statsMu.Lock()
	if s.creatureStatsCache == nil {
		s.creatureStatsCache = make(map[uint32]creatureStats)
	}
	s.creatureStatsCache[entry] = stats
	s.statsMu.Unlock()

	return stats
}

func (s *Server) loadCreatureReaction(ctx context.Context, entry uint32) (uint8, bool) {
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil {
		return creatureReactAggressive, false
	}
	var creatureType, npcFlags, flagsExtra int64
	var aiName string
	if err := s.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(type, 0), COALESCE(npcflag, 0), COALESCE(flags_extra, 0), COALESCE(AIName, '') FROM creature_template WHERE entry = ?", entry).Scan(&creatureType, &npcFlags, &flagsExtra, &aiName); err != nil {
		return creatureReactAggressive, false
	}
	return creatureReactState(uint32(creatureType), uint32(npcFlags), uint32(flagsExtra), aiName), true
}

func (s *session) loadCombatTarget(ctx context.Context, guid uint64) (combatTarget, error) {
	var target combatTarget
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return target, fmt.Errorf("world store DB not initialized")
	}
	var low, entry, mapID int64
	var curHealth sql.NullInt64
	lowGUID := uint32(guid & 0x00FFFFFF)
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT c.guid, c.id, c.map, c.position_x, c.position_y, c.position_z, c.curhealth FROM creature AS c WHERE c.guid = ?", lowGUID).Scan(&low, &entry, &mapID, &target.X, &target.Y, &target.Z, &curHealth); err != nil {
		return target, err
	}
	target.GUID = creatureWorldGUID(uint32(low), uint32(entry))
	target.Map = uint32(mapID)

	var ori sql.NullFloat64
	if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT c.orientation FROM creature AS c WHERE c.guid = ?", lowGUID).Scan(&ori)
		if ori.Valid {
			target.Orientation = float32(ori.Float64)
		}
	}

	st := s.server.loadCreatureStats(ctx, uint32(entry))
	target.UnitFlags = st.UnitFlags
	target.FlagsExtra = st.FlagsExtra
	_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(faction, 0) FROM creature_template WHERE entry = ?", uint32(entry)).Scan(&target.Faction)
	target.Armor = st.Armor
	target.Resistances = st.Resistances
	target.MinDamage = st.MinDamage
	target.MaxDamage = st.MaxDamage
	target.Level = uint8(st.Level)
	target.MaxHealth = st.MaxHealth
	target.CombatReach = st.CombatReach
	if curHealth.Valid {
		if curHealth.Int64 > 0 {
			target.Health = uint32(curHealth.Int64)
		} else {
			target.Health = st.Health
		}
	} else {
		target.Health = st.Health
	}
	if target.MaxHealth > 0 && target.Health > target.MaxHealth {
		target.Health = target.MaxHealth
	}

	s.server.motionMu.Lock()
	if s.server.creatureMotion == nil {
		s.server.creatureMotion = make(map[uint64]*creatureMotion)
	}
	if motion := s.server.creatureMotion[target.GUID]; motion != nil {
		target.X, target.Y, target.Z, target.Orientation = motion.X, motion.Y, motion.Z, motion.Orientation
		target.UnitFlags, target.FlagsExtra = motion.UnitFlags, motion.FlagsExtra
		target.Health = motion.Health
		if motion.Armor > 0 {
			target.Armor = motion.Armor
		}
		target.Resistances = motion.Resistances
		if motion.MinDamage > 0 {
			target.MinDamage = motion.MinDamage
			target.MaxDamage = motion.MaxDamage
		}
		if motion.CombatReach > 0 {
			target.CombatReach = motion.CombatReach
		} else {
			motion.CombatReach = target.CombatReach
		}
	} else {
		motion := &creatureMotion{
			GUID:        target.GUID,
			Entry:       uint32(entry),
			Map:         target.Map,
			HomeX:       target.X,
			HomeY:       target.Y,
			HomeZ:       target.Z,
			X:           target.X,
			Y:           target.Y,
			Z:           target.Z,
			Speed:       2.5,
			RunSpeed:    7.0,
			UnitFlags:   target.UnitFlags,
			FlagsExtra:  target.FlagsExtra,
			Health:      target.Health,
			MaxHealth:   target.MaxHealth,
			Armor:       target.Armor,
			Resistances: target.Resistances,
			MinDamage:   target.MinDamage,
			MaxDamage:   target.MaxDamage,
			Level:       uint32(target.Level),
			AttackTime:  st.AttackTime,
			CombatReach: target.CombatReach,
			Refreshed:   time.Now(),
		}
		s.server.creatureMotion[target.GUID] = motion
		if guid != target.GUID {
			s.server.creatureMotion[guid] = motion
		}
	}
	s.server.motionMu.Unlock()
	return target, nil
}

func (s *session) stopAttacksForFaction(ctx context.Context, factionID uint32) {
	if s == nil || s.player == nil || factionID == 0 || s.server == nil {
		return
	}
	if s.attackTarget != 0 {
		if target, ok := s.getCombatTarget(ctx, s.attackTarget); ok && target.Faction == factionID {
			_ = s.handleAttackStop()
		}
	}
	stops := make([]uint64, 0)
	s.server.motionMu.Lock()
	for _, motion := range s.server.creatureMotion {
		if motion == nil || motion.TargetGUID != s.playerGUID || motion.Faction != factionID {
			continue
		}
		motion.TargetGUID = 0
		motion.InCombat = false
		motion.Moving = false
		if motion.ThreatMgr != nil {
			motion.ThreatMgr.RemoveThreat(s.playerGUID)
		}
		stops = append(stops, motion.GUID)
	}
	s.server.motionMu.Unlock()
	for _, guid := range stops {
		packet := buildAttackStop(guid, s.playerGUID, false)
		_ = s.write(uint16(protocol.OpcodeSMSG_ATTACK_STOP), packet, true)
		s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_ATTACK_STOP), packet, s)
	}
}

func distance3D(x1, y1, z1, x2, y2, z2 float32) float64 {
	dx := float64(x1 - x2)
	dy := float64(y1 - y2)
	dz := float64(z1 - z2)
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// rollMeleeOutcome implements TrinityCore's single-roll melee attack table:
// MISS > DODGE > PARRY > GLANCING > BLOCK > CRIT > CRUSHING > HIT
// Reference: Unit::RollMeleeOutcomeAgainst (Unit.cpp:2189-2320).
// Optional modifiers:
// [0] critReductionBP (from defender resilience)
// [1] hitBonusBP (from attacker hit rating)
// [2] critBonusBP (from attacker crit rating & agility)
// [3] expertiseBP (reduces defender dodge and parry)
func rollMeleeOutcome(attackerLevel, victimLevel uint8, isPlayerAttacker, isPlayerVictim bool, isDualWielding bool, canBlock, canParry, canDodge bool, modifiers ...int32) (protocol.MeleeHitOutcome, uint32, uint8) {
	if attackerLevel == 0 {
		attackerLevel = 1
	}
	if victimLevel == 0 {
		victimLevel = 1
	}

	var critReductionBP, hitBonusBP, critBonusBP, expertiseBP int32
	if len(modifiers) > 0 {
		critReductionBP = modifiers[0]
	}
	if len(modifiers) > 1 {
		hitBonusBP = modifiers[1]
	}
	if len(modifiers) > 2 {
		critBonusBP = modifiers[2]
	}
	if len(modifiers) > 3 {
		expertiseBP = modifiers[3]
	}

	leveldif := int32(victimLevel) - int32(attackerLevel)

	// 1. Miss chance
	var missChance int32
	if isPlayerVictim {
		missChance = 500
		if leveldif > 0 {
			missChance += leveldif * 40
		} else {
			missChance += leveldif * 20
		}
	} else {
		// PvE against creatures
		if leveldif > 10 {
			missChance = 100 + (leveldif-10)*400
		} else if leveldif > 0 {
			missChance = 500 + leveldif*100
		} else {
			missChance = 500 + leveldif*100
		}
		// Low level mob scaling matching TC: if victimLevel < 10, missChance *= victimLevel / 10
		if victimLevel < 10 {
			missChance = int32(float64(missChance) * (float64(victimLevel) / 10.0))
		}
	}
	// Dual-wielding auto-attacks have +19% chance to miss (+1900 / 10000)
	// Reference: TrinityCore Unit::MeleeSpellMissChance (Unit.cpp:12425).
	if isDualWielding {
		missChance += 1900
	}
	if hitBonusBP > 0 {
		missChance -= hitBonusBP
	}
	if missChance < 0 {
		missChance = 0
	}

	// 2. Dodge chance: base 5% (500/10000)
	dodgeChance := int32(0)
	if canDodge && (isPlayerVictim || victimLevel >= 10) {
		dodgeChance = 500
		if leveldif > 0 {
			dodgeChance += leveldif * 10
		}
		if expertiseBP > 0 {
			dodgeChance -= expertiseBP
		}
		if dodgeChance < 0 {
			dodgeChance = 0
		}
	}

	// 3. Parry chance: base 5% (500/10000) if victim can parry
	parryChance := int32(0)
	if canParry && (isPlayerVictim || victimLevel >= 10) {
		parryChance = 500
		if leveldif > 0 {
			parryChance += leveldif * 10
		}
		if expertiseBP > 0 {
			parryChance -= expertiseBP
		}
		if parryChance < 0 {
			parryChance = 0
		}
	}

	// 4. Glancing blow: players/pets against higher level mobs
	glancingChance := int32(0)
	if isPlayerAttacker && !isPlayerVictim && victimLevel > attackerLevel {
		glancingChance = 600 + (int32(victimLevel)-int32(attackerLevel))*600
		if glancingChance > 4000 {
			glancingChance = 4000
		}
	}

	// 5. Block chance: base 5% (500/10000) if victim can block
	blockChance := int32(0)
	if canBlock {
		blockChance = 500
	}

	// 6. Crit chance: base 5% (500/10000)
	critChance := int32(500)
	if leveldif > 2 {
		critChance -= (leveldif - 2) * 100
	} else if leveldif > 0 {
		critChance -= leveldif * 20
	}
	if critBonusBP > 0 {
		critChance += critBonusBP
	}
	if critReductionBP > 0 {
		critChance -= critReductionBP
	}
	if critChance < 0 {
		critChance = 0
	}

	// 7. Crushing blow: mob attacking player 4+ levels below mob
	crushingChance := int32(0)
	if !isPlayerAttacker && isPlayerVictim && attackerLevel >= victimLevel+4 {
		crushingChance = (int32(attackerLevel)-int32(victimLevel)-4)*200 + 1500
	}

	roll := rand.IntN(10000)
	sum := int32(0)

	// 1. MISS
	sum += missChance
	if roll < int(sum) {
		return protocol.MeleeHitMiss, protocol.HitInfoMiss, protocol.VictimStateIntact
	}

	// 2. DODGE
	sum += dodgeChance
	if roll < int(sum) {
		return protocol.MeleeHitDodge, protocol.HitInfoNormalSwing, protocol.VictimStateDodge
	}

	// 3. PARRY
	if parryChance > 0 {
		sum += parryChance
		if roll < int(sum) {
			return protocol.MeleeHitParry, protocol.HitInfoNormalSwing, protocol.VictimStateParry
		}
	}

	// 4. GLANCING
	if glancingChance > 0 {
		sum += glancingChance
		if roll < int(sum) {
			return protocol.MeleeHitGlancing, protocol.HitInfoAffectsVictim | protocol.HitInfoGlancing, protocol.VictimStateHit
		}
	}

	// 5. BLOCK
	if blockChance > 0 {
		sum += blockChance
		if roll < int(sum) {
			return protocol.MeleeHitBlock, protocol.HitInfoAffectsVictim | protocol.HitInfoBlock, protocol.VictimStateHit
		}
	}

	// 6. CRIT
	if critChance > 0 {
		sum += critChance
		if roll < int(sum) {
			return protocol.MeleeHitCrit, protocol.HitInfoAffectsVictim | protocol.HitInfoCriticalHit, protocol.VictimStateHit
		}
	}

	// 7. CRUSHING
	if crushingChance > 0 {
		sum += crushingChance
		if roll < int(sum) {
			return protocol.MeleeHitCrushing, protocol.HitInfoAffectsVictim | protocol.HitInfoCrushing, protocol.VictimStateHit
		}
	}

	return protocol.MeleeHitNormal, protocol.HitInfoAffectsVictim, protocol.VictimStateHit
}

func buildAttackerStateUpdate(attacker, victim uint64, damage, overkill uint32) []byte {
	return protocol.BuildAttackerStateUpdate(attacker, victim, damage, overkill, protocol.HitInfoAffectsVictim, protocol.VictimStateHit, 0)
}

// handleDuelAccepted processes CMSG_DUEL_ACCEPTED (0x16C).
// Reference: WorldSession::HandleDuelAcceptedOpcode (DuelHandler.cpp:25).
func (s *session) handleDuelAccepted(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	buf := protocol.NewBuffer(4)
	buf.WriteU32(3000) // 3000ms duel countdown
	_ = s.write(uint16(protocol.OpcodeSMSG_DUEL_COUNTDOWN), buf.Bytes(), true)
	var partner *session
	if s.duelPartner != 0 && s.server != nil {
		partner = s.server.findSessionByGUID(s.duelPartner)
		if partner != nil {
			_ = partner.write(uint16(protocol.OpcodeSMSG_DUEL_COUNTDOWN), buf.Bytes(), true)
		}
	}

	// Initialize arbiter coordinates if not yet set
	if s.duelArbiterX == 0 && s.duelArbiterY == 0 && partner != nil && partner.player != nil {
		midX := s.player.X + (partner.player.X-s.player.X)/2
		midY := s.player.Y + (partner.player.Y-s.player.Y)/2
		midZ := s.player.Z
		s.duelArbiterX, s.duelArbiterY, s.duelArbiterZ = midX, midY, midZ
		partner.duelArbiterX, partner.duelArbiterY, partner.duelArbiterZ = midX, midY, midZ
	}

	// After 3-second countdown, set PLAYER_DUEL_TEAM to start the duel! (TC: Player::UpdateDuelFlag)
	time.AfterFunc(3*time.Second, func() {
		if s.duelPartner == 0 || s.player == nil {
			return
		}
		s.player.DuelTeam = 1
		s.sendPlayerUpdate()
		if partner != nil && partner.player != nil {
			partner.player.DuelTeam = 2
			partner.sendPlayerUpdate()
		}
	})
	return true
}

// handleDuelCancelled processes CMSG_DUEL_CANCELLED (0x16D).
// Reference: WorldSession::HandleDuelCancelledOpcode (DuelHandler.cpp:53).
func (s *session) handleDuelCancelled(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	// Player surrendered in an active duel using /forfeit (TC: HandleDuelCancelledOpcode:66)
	if s.player.DuelTeam != 0 && s.duelPartner != 0 {
		s.endDuel(true, s.duelPartner, false)
		return true
	}
	s.endDuel(false, 0, false)
	return true
}

// DuelCompleteType mirrors TrinityCore's DuelCompleteType enum.
type DuelCompleteType uint8

const (
	DuelInterrupted DuelCompleteType = 0
	DuelWon         DuelCompleteType = 1
	DuelFled        DuelCompleteType = 2
)

// buildDuelWinner builds SMSG_DUEL_WINNER (0x16B).
// Reference: Player::DuelComplete (Player.cpp:7353-7358).
func buildDuelWinner(fled bool, winnerName, loserName string) []byte {
	packet := protocol.NewBuffer(1 + len(winnerName) + 1 + len(loserName) + 1)
	if fled {
		packet.WriteU8(1)
	} else {
		packet.WriteU8(0)
	}
	packet.WriteCString(winnerName)
	packet.WriteCString(loserName)
	return packet.Bytes()
}

// castVisualSpell broadcasts SMSG_SPELL_GO for instant non-combat visual spells (e.g. kneel 7267, victory cheer 52852).
func (s *session) castVisualSpell(spellID uint32) {
	if s == nil || s.player == nil {
		return
	}
	castPkt := protocol.NewBuffer(16)
	castPkt.WritePackedGUID(s.playerGUID)
	castPkt.WritePackedGUID(s.playerGUID)
	castPkt.WriteU8(1)
	castPkt.WriteU32(spellID)
	castPkt.WriteU32(0)
	_ = s.write(uint16(protocol.OpcodeSMSG_SPELL_GO), castPkt.Bytes(), true)
	if s.server != nil {
		s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELL_GO), castPkt.Bytes(), s)
	}
}

// checkDuelBounds checks distance to duel arbiter (flag).
// > 50yd sends SMSG_DUEL_OUTOFBOUNDS and starts 10s timer.
// <= 40yd sends SMSG_DUEL_INBOUNDS and resets timer.
// > 10s out of bounds ends duel as fled.
// Reference: Player::CheckDuelOutOfBounds (Player.cpp:7290-7321).
func (s *session) checkDuelBounds() {
	if s == nil || s.player == nil || s.player.DuelTeam == 0 || s.duelPartner == 0 || s.player.DuelArbiter == 0 {
		return
	}
	dist := distance3D(s.player.X, s.player.Y, s.player.Z, s.duelArbiterX, s.duelArbiterY, s.duelArbiterZ)
	now := time.Now()
	if s.duelOutOfBounds.IsZero() {
		if dist > 50.0 {
			s.duelOutOfBounds = now.Add(10 * time.Second)
			_ = s.write(uint16(protocol.OpcodeSMSG_DUEL_OUTOFBOUNDS), []byte{}, true)
			partner := s.duelPartner
			time.AfterFunc(10*time.Second, func() {
				if s.player != nil && s.player.DuelTeam != 0 && s.duelPartner == partner && !s.duelOutOfBounds.IsZero() && time.Now().After(s.duelOutOfBounds) {
					s.checkDuelBounds()
				}
			})
		}
	} else {
		if dist <= 40.0 {
			s.duelOutOfBounds = time.Time{}
			_ = s.write(uint16(protocol.OpcodeSMSG_DUEL_INBOUNDS), []byte{}, true)
		} else if now.After(s.duelOutOfBounds) {
			s.endDuel(false, s.duelPartner, true)
		}
	}
}

// endDuel cleans up duel flags, clears arbiter/team, and emits SMSG_DUEL_COMPLETE and SMSG_DUEL_WINNER.
// Reference: Player::DuelComplete (Player.cpp:7328-7442).
func (s *session) endDuel(won bool, winnerGUID uint64, fled bool) {
	partnerGUID := s.duelPartner
	var partner *session
	if partnerGUID != 0 && s.server != nil {
		partner = s.server.findSessionByGUID(partnerGUID)
	}

	duelEnded := won || fled
	completeResult := uint8(0)
	if duelEnded {
		completeResult = 1
	}

	buf := protocol.NewBuffer(1)
	buf.WriteU8(completeResult)
	_ = s.write(uint16(protocol.OpcodeSMSG_DUEL_COMPLETE), buf.Bytes(), true)
	if partner != nil {
		_ = partner.write(uint16(protocol.OpcodeSMSG_DUEL_COMPLETE), buf.Bytes(), true)
	}

	if duelEnded && winnerGUID != 0 {
		var winnerSess, loserSess *session
		if s.playerGUID == winnerGUID {
			winnerSess = s
			loserSess = partner
		} else {
			winnerSess = partner
			loserSess = s
		}

		winnerName := ""
		loserName := ""
		if winnerSess != nil && winnerSess.player != nil {
			winnerName = winnerSess.player.Name
		}
		if loserSess != nil && loserSess.player != nil {
			loserName = loserSess.player.Name
		}

		winnerPkt := buildDuelWinner(fled, winnerName, loserName)
		_ = s.write(uint16(protocol.OpcodeSMSG_DUEL_WINNER), winnerPkt, true)
		if partner != nil {
			_ = partner.write(uint16(protocol.OpcodeSMSG_DUEL_WINNER), winnerPkt, true)
		}
		if s.server != nil && s.player != nil {
			s.server.sessionsMu.RLock()
			for target := range s.server.sessions {
				if target == s || target == partner || !target.authed || !target.playerLoaded || target.player == nil {
					continue
				}
				if target.player.Map == s.player.Map {
					_ = target.write(uint16(protocol.OpcodeSMSG_DUEL_WINNER), winnerPkt, true)
				}
			}
			s.server.sessionsMu.RUnlock()
		}

		// Spells: Winner casts 52852 (Victory cheer)
		if winnerSess != nil {
			winnerSess.castVisualSpell(52852)
			winnerSess.updateAchievementCriteria(criteriaTypeWinDuel, 0, 1)
		}
		// Loser casts 7267 (Beg / surrender kneel) if won normally
		if !fled && loserSess != nil {
			loserSess.castVisualSpell(7267)
		}
		if loserSess != nil {
			loserSess.updateAchievementCriteria(criteriaTypeLoseDuel, 0, 1)
		}
	}

	// Stop combat on both
	_ = s.sendAttackStop(s.attackTarget, false)
	s.attackTarget = 0
	if partner != nil {
		_ = partner.sendAttackStop(partner.attackTarget, false)
		partner.attackTarget = 0
	}

	// Clean up fields on s
	if s.player != nil {
		s.player.DuelArbiter = 0
		s.player.DuelTeam = 0
		s.sendPlayerUpdate()
	}
	s.duelPartner = 0
	s.duelOutOfBounds = time.Time{}

	// Clean up fields on partner
	if partner != nil {
		if partner.player != nil {
			partner.player.DuelArbiter = 0
			partner.player.DuelTeam = 0
			partner.sendPlayerUpdate()
		}
		partner.duelPartner = 0
		partner.duelOutOfBounds = time.Time{}
	}
}

// isInCombat mirrors Unit::IsInCombat.
func (s *session) isInCombat() bool {
	if s == nil || s.player == nil {
		return false
	}
	return s.attackTarget != 0 || (s.player.UnitFlags&unitFlagInCombat != 0)
}
