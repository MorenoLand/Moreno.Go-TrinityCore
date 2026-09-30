package world

import (
	"context"
	"math/rand"
	"strconv"
	"strings"
	"time"

	protocol "github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// Weapon enchantment IDs matching SpellItemEnchantment.dbc
const (
	EnchantIDFieryWeapon = 803
	EnchantIDCrusader    = 1900
	EnchantIDMongoose    = 2673
	EnchantIDExecutioner = 3225
	EnchantIDBerserking  = 3789
	EnchantIDBlackMagic  = 3790
	EnchantIDCrippling   = 22
	EnchantIDInstantPois = 3729
	EnchantIDDeadlyPois  = 3731
	EnchantIDWoundPois   = 3734
)

// Proc spell IDs triggered by weapon enchantments, trinkets, and talents
const (
	ProcSpellFieryWeapon = 13897 // Fiery Weapon damage
	ProcSpellCrusader    = 20007 // Holy Strength (+100 Str + Heal)
	ProcSpellMongoose    = 28093 // Lightning Speed (+120 Agi + 2% Haste)
	ProcSpellExecutioner = 42976 // Executioner (+120 ArP)
	ProcSpellBerserking  = 59620 // Berserking (+400 AP)
	ProcSpellBlackMagic  = 59626 // Black Magic (+250 Haste)
	ProcSpellCrippling   = 3408  // Crippling Poison (-70% speed)
	ProcSpellInstantPois = 57965 // Instant Poison IX
	ProcSpellDeadlyPois  = 57970 // Deadly Poison IX
	ProcSpellWoundPois   = 57975 // Wound Poison VII

	// Trinket Proc Spells & ICDs
	// Deathbringer's Will (DBW) - 45s ICD
	ProcSpellDBWAgilityNorm  = 71485 // +600 Agi (Taunka)
	ProcSpellDBWStrengthNorm = 71487 // +600 Str (Vrykul)
	ProcSpellDBWAPNorm       = 71484 // +1200 AP (Iron Dwarf)
	ProcSpellDBWAgilityHero  = 71491 // +700 Agi
	ProcSpellDBWStrengthHero = 71492 // +700 Str
	ProcSpellDBWAPHero       = 71560 // +1400 AP

	// Whispering Fanged Skull (WFS) - 45s ICD
	ProcSpellWFSNorm = 71401 // +1100 AP
	ProcSpellWFSHero = 71403 // +1250 AP

	// Death's Choice / Death's Verdict - 45s ICD
	ProcSpellDeathsChoiceNorm = 67703 // +450 Str/Agi
	ProcSpellDeathsChoiceHero = 67772 // +510 Str/Agi

	// Darkmoon Card: Greatness - 45s ICD
	ProcSpellDMCGStrength  = 60229 // +300 Str
	ProcSpellDMCGAgility   = 60233 // +300 Agi
	ProcSpellDMCGIntellect = 60234 // +300 Int
	ProcSpellDMCGSpirit    = 60235 // +300 Spi

	// Mjolnir Runestone - 45s ICD
	ProcSpellMjolnirRunestone = 60298 // +665 ArP

	// Ashen Band (Ashen Verdict Exalted Rings) - 60s ICD
	ProcSpellAshenBandMight       = 71562 // +480 AP
	ProcSpellAshenBandDestruction = 71563 // +285 SP

	// Sundial of the Exiled - 45s ICD
	ProcSpellSundialOfTheExiled = 59626 // +590 SP

	// Reign of the Dead / Reign of the Unliving - 2s ICD
	ProcSpellReignOfTheDeadNorm = 67758 // Mote of Anger
	ProcSpellReignOfTheDeadHero = 67759

	// Talent / Class Ability ICDs
	ProcSpellSuddenDeath = 52437 // Sudden Death (Warrior, 10s ICD)
	ProcSpellSwordSpec   = 12281 // Sword Specialization (0.5s ICD)
	ProcSpellWindfury    = 25505 // Windfury Weapon (3.0s ICD)
)

// Item IDs for popular proc trinkets and rings
const (
	ItemDeathbringersWillNorm = 50362
	ItemDeathbringersWillHero = 50363

	ItemWhisperingFangedSkullNorm = 50342
	ItemWhisperingFangedSkullHero = 50343

	ItemDeathsChoiceNormA = 47115
	ItemDeathsChoiceHeroA = 47131
	ItemDeathsChoiceNormH = 47303
	ItemDeathsChoiceHeroH = 47464

	ItemDMCGStrength  = 44253
	ItemDMCGAgility   = 44254
	ItemDMCGIntellect = 44255
	ItemDMCGSpirit    = 44256

	ItemMjolnirRunestone = 45931

	ItemAshenBandMight277       = 50402
	ItemAshenBandMight268       = 50401
	ItemAshenBandDestruction277 = 50398
	ItemAshenBandDestruction268 = 50397

	ItemSundialOfTheExiled = 40682

	ItemReignOfTheDeadNorm = 47182
	ItemReignOfTheDeadHero = 47188
)

// spellAttr4CantTriggerItemSpells is SPELL_ATTR4_CANT_TRIGGER_ITEM_SPELLS
// (SharedDefines.h:583); ATTR4 is Go's AttributesEx4 (Spell.dbc field 8).
const spellAttr4CantTriggerItemSpells uint32 = 0x00800000

// spellHitMayFireItemProcs mirrors the spell-hit gate on item combat spells
// (Spell.cpp:2588-2596): on spell hits they fire only for melee/ranged
// damage-class spells without SPELL_ATTR0_STOP_ATTACK_TARGET or
// SPELL_ATTR4_CANT_TRIGGER_ITEM_SPELLS. The fork loads DmgClass from
// Spell.dbc field 213 (SpellInfo.cpp:856), which Go parses as
// Spell.DefenseType. Unresolvable DBC data fails open, preserving the
// previous behavior.
func (s *session) spellHitMayFireItemProcs(spellID uint32) bool {
	if s == nil || s.server == nil || s.server.Data == nil {
		return true
	}
	spell, ok, err := s.server.Data.Spell(spellID)
	if err != nil || !ok {
		return true
	}
	if spell.DefenseType != spellDamageClassMelee && spell.DefenseType != spellDamageClassRanged {
		return false
	}
	if spell.Attributes&spellAttr0StopAttackTarget != 0 {
		return false
	}
	if spell.AttributesEx4&spellAttr4CantTriggerItemSpells != 0 {
		return false
	}
	return true
}

// RollPPMChance rolls whether a weapon proc occurs based on Procs Per Minute (PPM)
// and weapon attack speed in milliseconds.
// Formula: chance = (weaponSpeedMs * PPM) / 60000.0
// Reference: TrinityCore Unit::GetPPMProcChance (Unit.cpp:8820).
func RollPPMChance(ppm float64, weaponSpeedMs uint32) bool {
	if ppm <= 0 || weaponSpeedMs == 0 {
		return false
	}
	chance := (float64(weaponSpeedMs) * ppm) / 60000.0
	if chance <= 0 {
		return false
	}
	if chance >= 1.0 {
		return true
	}
	return rand.Float64() < chance
}

// procChanceDefault resolves the proc roll chance for a triggered proc spell.
// Mirrors TrinityCore SpellMgr::LoadSpellProc (SpellMgr.cpp:1597): a proc entry
// with no explicit Chance and no ProcsPerMinute falls back to the proc spell's
// Spell.dbc ProcChance (field 35). Go's per-item chances stand in for the
// spell_proc table rows, so they win when set; the DBC field is the default.
func (s *session) procChanceDefault(procSpellID uint32, configuredChance float64) float64 {
	if configuredChance > 0 {
		return configuredChance
	}
	if s == nil || s.server == nil || s.server.Data == nil {
		return 0
	}
	spell, ok, err := s.server.Data.Spell(procSpellID)
	if err != nil || !ok {
		return 0
	}
	return float64(spell.ProcChance) / 100.0
}

// isProcOnCooldown checks whether an internal cooldown (ICD) is active for the given proc ID.
func (s *session) isProcOnCooldown(procID uint32) bool {
	if s == nil || s.procICD == nil {
		return false
	}
	expires, ok := s.procICD[procID]
	if !ok {
		return false
	}
	return time.Now().Before(expires)
}

// triggerProcCooldown registers an internal cooldown for the given proc ID.
func (s *session) triggerProcCooldown(procID uint32, duration time.Duration) {
	if s == nil || duration <= 0 {
		return
	}
	if s.procICD == nil {
		s.procICD = make(map[uint32]time.Time)
	}
	s.procICD[procID] = time.Now().Add(duration)
}

// getProcRemainingCooldown returns the remaining duration of an active ICD.
func (s *session) getProcRemainingCooldown(procID uint32) time.Duration {
	if s == nil || s.procICD == nil {
		return 0
	}
	expires, ok := s.procICD[procID]
	if !ok {
		return 0
	}
	now := time.Now()
	if now.Before(expires) {
		return expires.Sub(now)
	}
	return 0
}

// getEquipmentEnchant returns the enchantment ID present on the given equipment slot.
func (s *session) getEquipmentEnchant(slot uint8) uint32 {
	if s == nil || s.player == nil || s.player.Equipment == "" {
		return 0
	}
	fields := strings.Fields(s.player.Equipment)
	encIdx := int(slot)*2 + 1
	if encIdx >= len(fields) {
		return 0
	}
	encID, err := strconv.ParseUint(fields[encIdx], 10, 32)
	if err != nil {
		return 0
	}
	return uint32(encID)
}

// getEquipmentItem returns the item entry ID present on the given equipment slot.
func (s *session) getEquipmentItem(slot uint8) uint32 {
	if s == nil || s.player == nil || s.player.Equipment == "" {
		return 0
	}
	fields := strings.Fields(s.player.Equipment)
	itemIdx := int(slot) * 2
	if itemIdx >= len(fields) {
		return 0
	}
	itemID, err := strconv.ParseUint(fields[itemIdx], 10, 32)
	if err != nil {
		return 0
	}
	return uint32(itemID)
}

// procWeaponEnchantments evaluates and triggers weapon enchantment procs upon a successful melee hit.
// Mirrors TrinityCore Unit::ProcDamageAndSpellFor (Unit.cpp:10800-11200).
func (s *session) procWeaponEnchantments(ctx context.Context, target combatTarget, attType protocol.WeaponAttackType, outcome protocol.MeleeHitOutcome) {
	if s == nil || s.player == nil || target.Health == 0 {
		return
	}

	// Only hits, crits, blocks, and glancing blows can proc on-hit effects
	switch outcome {
	case protocol.MeleeHitNormal, protocol.MeleeHitCrit, protocol.MeleeHitBlock, protocol.MeleeHitGlancing, protocol.MeleeHitCrushing:
		// valid hit outcome
	default:
		return
	}

	slot := uint8(15) // equipSlotMainHand
	attTime := s.player.AttackTime
	if attType == protocol.OffAttack {
		slot = 16 // equipSlotOffHand
		attTime = s.player.OffhandAttackTime
	}
	if attTime == 0 {
		attTime = 2000
	}

	encID := s.getEquipmentEnchant(slot)
	if encID == 0 {
		return
	}

	switch encID {
	case EnchantIDBerserking:
		if RollPPMChance(1.0, attTime) {
			s.castSpellDirect(ctx, ProcSpellBerserking, s.playerGUID)
		}
	case EnchantIDMongoose:
		if RollPPMChance(1.0, attTime) {
			s.castSpellDirect(ctx, ProcSpellMongoose, s.playerGUID)
		}
	case EnchantIDExecutioner:
		if RollPPMChance(1.0, attTime) {
			s.castSpellDirect(ctx, ProcSpellExecutioner, s.playerGUID)
		}
	case EnchantIDCrusader:
		if RollPPMChance(1.0, attTime) {
			s.castSpellDirect(ctx, ProcSpellCrusader, s.playerGUID)
		}
	case EnchantIDBlackMagic:
		if !s.isProcOnCooldown(ProcSpellBlackMagic) && RollPPMChance(1.0, attTime) {
			s.castSpellDirect(ctx, ProcSpellBlackMagic, s.playerGUID)
			s.triggerProcCooldown(ProcSpellBlackMagic, 35*time.Second)
		}
	case EnchantIDFieryWeapon:
		if RollPPMChance(6.0, attTime) {
			s.castSpellDirect(ctx, ProcSpellFieryWeapon, target.GUID)
		}
	case EnchantIDInstantPois:
		if rand.Float64() < s.procChanceDefault(ProcSpellInstantPois, 0.20) {
			s.castSpellDirect(ctx, ProcSpellInstantPois, target.GUID)
		}
	case EnchantIDDeadlyPois:
		if rand.Float64() < s.procChanceDefault(ProcSpellDeadlyPois, 0.30) {
			s.castSpellDirect(ctx, ProcSpellDeadlyPois, target.GUID)
		}
	case EnchantIDWoundPois:
		if rand.Float64() < s.procChanceDefault(ProcSpellWoundPois, 0.50) {
			s.castSpellDirect(ctx, ProcSpellWoundPois, target.GUID)
		}
	case EnchantIDCrippling:
		if rand.Float64() < s.procChanceDefault(ProcSpellCrippling, 0.50) {
			s.castSpellDirect(ctx, ProcSpellCrippling, target.GUID)
		}
	}
}

// procItemAndTrinketEffects evaluates and triggers equipped trinkets and rings on melee/ranged attacks.
// Respects exact TrinityCore 3.3.5 Internal Cooldowns (ICD) and proc chances.
// Reference: TrinityCore Unit::ProcDamageAndSpellFor (Unit.cpp:10950-11250).
func (s *session) procItemAndTrinketEffects(ctx context.Context, target combatTarget, attType protocol.WeaponAttackType, outcome protocol.MeleeHitOutcome) {
	if s == nil || s.player == nil || target.Health == 0 {
		return
	}

	switch outcome {
	case protocol.MeleeHitNormal, protocol.MeleeHitCrit, protocol.MeleeHitBlock, protocol.MeleeHitGlancing, protocol.MeleeHitCrushing:
		// valid hit outcome
	default:
		return
	}

	// Check Trinket 1 (slot 12), Trinket 2 (slot 13), Finger 1 (slot 10), Finger 2 (slot 11)
	slots := []uint8{equipSlotTrinket1, equipSlotTrinket2, equipSlotFinger1, equipSlotFinger2}
	for _, slot := range slots {
		itemID := s.getEquipmentItem(slot)
		if itemID == 0 {
			continue
		}

		switch itemID {
		case ItemDeathbringersWillNorm:
			// 35% chance on attack, 45s ICD
			if !s.isProcOnCooldown(ProcSpellDBWAgilityNorm) && rand.Float64() < s.procChanceDefault(ProcSpellDBWAgilityNorm, 0.35) {
				// Pick one of the 3 forms: Agi (71485), Str (71487), AP (71484)
				forms := []uint32{ProcSpellDBWAgilityNorm, ProcSpellDBWStrengthNorm, ProcSpellDBWAPNorm}
				chosen := forms[rand.Intn(len(forms))]
				s.castSpellDirect(ctx, chosen, s.playerGUID)
				s.triggerProcCooldown(ProcSpellDBWAgilityNorm, 45*time.Second)
			}

		case ItemDeathbringersWillHero:
			// 35% chance on attack, 45s ICD
			if !s.isProcOnCooldown(ProcSpellDBWAgilityHero) && rand.Float64() < s.procChanceDefault(ProcSpellDBWAgilityHero, 0.35) {
				forms := []uint32{ProcSpellDBWAgilityHero, ProcSpellDBWStrengthHero, ProcSpellDBWAPHero}
				chosen := forms[rand.Intn(len(forms))]
				s.castSpellDirect(ctx, chosen, s.playerGUID)
				s.triggerProcCooldown(ProcSpellDBWAgilityHero, 45*time.Second)
			}

		case ItemWhisperingFangedSkullNorm:
			// 35% chance on attack, 45s ICD
			if !s.isProcOnCooldown(ProcSpellWFSNorm) && rand.Float64() < s.procChanceDefault(ProcSpellWFSNorm, 0.35) {
				s.castSpellDirect(ctx, ProcSpellWFSNorm, s.playerGUID)
				s.triggerProcCooldown(ProcSpellWFSNorm, 45*time.Second)
			}

		case ItemWhisperingFangedSkullHero:
			if !s.isProcOnCooldown(ProcSpellWFSHero) && rand.Float64() < s.procChanceDefault(ProcSpellWFSHero, 0.35) {
				s.castSpellDirect(ctx, ProcSpellWFSHero, s.playerGUID)
				s.triggerProcCooldown(ProcSpellWFSHero, 45*time.Second)
			}

		case ItemDeathsChoiceNormA, ItemDeathsChoiceNormH:
			if !s.isProcOnCooldown(ProcSpellDeathsChoiceNorm) && rand.Float64() < s.procChanceDefault(ProcSpellDeathsChoiceNorm, 0.35) {
				s.castSpellDirect(ctx, ProcSpellDeathsChoiceNorm, s.playerGUID)
				s.triggerProcCooldown(ProcSpellDeathsChoiceNorm, 45*time.Second)
			}

		case ItemDeathsChoiceHeroA, ItemDeathsChoiceHeroH:
			if !s.isProcOnCooldown(ProcSpellDeathsChoiceHero) && rand.Float64() < s.procChanceDefault(ProcSpellDeathsChoiceHero, 0.35) {
				s.castSpellDirect(ctx, ProcSpellDeathsChoiceHero, s.playerGUID)
				s.triggerProcCooldown(ProcSpellDeathsChoiceHero, 45*time.Second)
			}

		case ItemDMCGStrength:
			if !s.isProcOnCooldown(ProcSpellDMCGStrength) && rand.Float64() < s.procChanceDefault(ProcSpellDMCGStrength, 0.35) {
				s.castSpellDirect(ctx, ProcSpellDMCGStrength, s.playerGUID)
				s.triggerProcCooldown(ProcSpellDMCGStrength, 45*time.Second)
			}

		case ItemDMCGAgility:
			if !s.isProcOnCooldown(ProcSpellDMCGAgility) && rand.Float64() < s.procChanceDefault(ProcSpellDMCGAgility, 0.35) {
				s.castSpellDirect(ctx, ProcSpellDMCGAgility, s.playerGUID)
				s.triggerProcCooldown(ProcSpellDMCGAgility, 45*time.Second)
			}

		case ItemMjolnirRunestone:
			if !s.isProcOnCooldown(ProcSpellMjolnirRunestone) && rand.Float64() < s.procChanceDefault(ProcSpellMjolnirRunestone, 0.15) {
				s.castSpellDirect(ctx, ProcSpellMjolnirRunestone, s.playerGUID)
				s.triggerProcCooldown(ProcSpellMjolnirRunestone, 45*time.Second)
			}

		case ItemAshenBandMight277, ItemAshenBandMight268:
			if !s.isProcOnCooldown(ProcSpellAshenBandMight) && rand.Float64() < s.procChanceDefault(ProcSpellAshenBandMight, 0.30) {
				s.castSpellDirect(ctx, ProcSpellAshenBandMight, s.playerGUID)
				s.triggerProcCooldown(ProcSpellAshenBandMight, 60*time.Second)
			}
		}
	}
}

// procSpellCastAndHitEffects evaluates caster trinkets and rings upon direct spell damage or healing.
// Reference: TrinityCore Unit::ProcDamageAndSpellFor (Unit.cpp:11300-11500).
func (s *session) procSpellCastAndHitEffects(ctx context.Context, target combatTarget, spellID uint32) {
	if s == nil || s.player == nil {
		return
	}

	slots := []uint8{equipSlotTrinket1, equipSlotTrinket2, equipSlotFinger1, equipSlotFinger2}
	for _, slot := range slots {
		itemID := s.getEquipmentItem(slot)
		if itemID == 0 {
			continue
		}

		switch itemID {
		case ItemSundialOfTheExiled:
			// 10% chance on damaging spell, 45s ICD
			if !s.isProcOnCooldown(ProcSpellSundialOfTheExiled) && rand.Float64() < s.procChanceDefault(ProcSpellSundialOfTheExiled, 0.10) {
				s.castSpellDirect(ctx, ProcSpellSundialOfTheExiled, s.playerGUID)
				s.triggerProcCooldown(ProcSpellSundialOfTheExiled, 45*time.Second)
			}

		case ItemAshenBandDestruction277, ItemAshenBandDestruction268:
			// 10% chance on spell cast, 60s ICD
			if !s.isProcOnCooldown(ProcSpellAshenBandDestruction) && rand.Float64() < s.procChanceDefault(ProcSpellAshenBandDestruction, 0.10) {
				s.castSpellDirect(ctx, ProcSpellAshenBandDestruction, s.playerGUID)
				s.triggerProcCooldown(ProcSpellAshenBandDestruction, 60*time.Second)
			}

		case ItemReignOfTheDeadNorm:
			// Mote generation on spell crit (or hit), 2s ICD
			if !s.isProcOnCooldown(ProcSpellReignOfTheDeadNorm) {
				s.castSpellDirect(ctx, ProcSpellReignOfTheDeadNorm, s.playerGUID)
				s.triggerProcCooldown(ProcSpellReignOfTheDeadNorm, 2*time.Second)
			}

		case ItemReignOfTheDeadHero:
			if !s.isProcOnCooldown(ProcSpellReignOfTheDeadHero) {
				s.castSpellDirect(ctx, ProcSpellReignOfTheDeadHero, s.playerGUID)
				s.triggerProcCooldown(ProcSpellReignOfTheDeadHero, 2*time.Second)
			}
		}
	}
}
