package world

import (
	"context"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
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

// spellHitCanTriggerItemProcs mirrors the canTrigger gate on item combat
// spells (Player::CastItemCombatSpell, Player.cpp:8109): the item-spell
// table is only evaluated when the hit mask intersects
// PROC_HIT_NORMAL | PROC_HIT_CRITICAL | PROC_HIT_ABSORB. A missed spell
// carries PROC_HIT_MISS (createProcHitMask, Unit.cpp:10179), an immune
// target PROC_HIT_IMMUNE (Spell.cpp:2581), and a fully resisted hit
// PROC_HIT_FULL_RESIST — none of which intersect, so no item procs fire.
// A fully absorbed hit still carries PROC_HIT_ABSORB and fires, so a
// nonzero absorb keeps the gate open. The same default hit mask applies
// inside SpellMgr::CanSpellTriggerProcOnEvent (SpellMgr.cpp:562-576).
func spellHitCanTriggerItemProcs(isHit, immune, fullyResisted bool, absorbed uint32) bool {
	if !isHit || immune || (fullyResisted && absorbed == 0) {
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

// ---------------------------------------------------------------------------
// Aura proc pipeline (Aura::GetProcEffectMask / SpellMgr::CanSpellTriggerProcOnEvent)
// ---------------------------------------------------------------------------

// Proc event type flags (SpellMgr.h:114-186). Go's Spell.ProcTypeMask is
// Spell.dbc field 34, which C++ loads as SpellInfo::ProcFlags
// (SpellInfo.cpp:822).
const (
	procFlagNone                       uint32 = 0x00000000
	procFlagKilled                     uint32 = 0x00000001
	procFlagKill                       uint32 = 0x00000002
	procFlagDoneMeleeAutoAttack        uint32 = 0x00000004
	procFlagTakenMeleeAutoAttack       uint32 = 0x00000008
	procFlagDoneSpellMeleeDmgClass     uint32 = 0x00000010
	procFlagTakenSpellMeleeDmgClass    uint32 = 0x00000020
	procFlagDoneRangedAutoAttack       uint32 = 0x00000040
	procFlagTakenRangedAutoAttack      uint32 = 0x00000080
	procFlagDoneSpellRangedDmgClass    uint32 = 0x00000100
	procFlagTakenSpellRangedDmgClass   uint32 = 0x00000200
	procFlagDoneSpellNoneDmgClassPos   uint32 = 0x00000400
	procFlagTakenSpellNoneDmgClassPos  uint32 = 0x00000800
	procFlagDoneSpellNoneDmgClassNeg   uint32 = 0x00001000
	procFlagTakenSpellNoneDmgClassNeg  uint32 = 0x00002000
	procFlagDoneSpellMagicDmgClassPos  uint32 = 0x00004000
	procFlagTakenSpellMagicDmgClassPos uint32 = 0x00008000
	procFlagDoneSpellMagicDmgClassNeg  uint32 = 0x00010000
	procFlagTakenSpellMagicDmgClassNeg uint32 = 0x00020000
	procFlagDonePeriodic               uint32 = 0x00040000
	procFlagTakenPeriodic              uint32 = 0x00080000
	procFlagTakenDamage                uint32 = 0x00100000
	procFlagDoneTrapActivation         uint32 = 0x00200000
	procFlagDoneMainhandAttack         uint32 = 0x00400000
	procFlagDoneOffhandAttack          uint32 = 0x00800000
	procFlagDeath                      uint32 = 0x01000000
)

// Proc flag masks (SpellMgr.h:188-216).
const (
	procSpellProcFlagMask = procFlagDoneSpellMeleeDmgClass | procFlagTakenSpellMeleeDmgClass |
		procFlagDoneRangedAutoAttack | procFlagTakenRangedAutoAttack |
		procFlagDoneSpellRangedDmgClass | procFlagTakenSpellRangedDmgClass |
		procFlagDoneSpellNoneDmgClassPos | procFlagTakenSpellNoneDmgClassPos |
		procFlagDoneSpellNoneDmgClassNeg | procFlagTakenSpellNoneDmgClassNeg |
		procFlagDoneSpellMagicDmgClassPos | procFlagTakenSpellMagicDmgClassPos |
		procFlagDoneSpellMagicDmgClassNeg | procFlagTakenSpellMagicDmgClassNeg |
		procFlagDonePeriodic | procFlagTakenPeriodic | procFlagDoneTrapActivation
	procDoneHitProcFlagMask = procFlagDoneMeleeAutoAttack | procFlagDoneRangedAutoAttack |
		procFlagDoneSpellMeleeDmgClass | procFlagDoneSpellRangedDmgClass |
		procFlagDoneSpellNoneDmgClassPos | procFlagDoneSpellNoneDmgClassNeg |
		procFlagDoneSpellMagicDmgClassPos | procFlagDoneSpellMagicDmgClassNeg |
		procFlagDonePeriodic | procFlagDoneTrapActivation |
		procFlagDoneMainhandAttack | procFlagDoneOffhandAttack
	procTakenHitProcFlagMask = procFlagTakenMeleeAutoAttack | procFlagTakenRangedAutoAttack |
		procFlagTakenSpellMeleeDmgClass | procFlagTakenSpellRangedDmgClass |
		procFlagTakenSpellNoneDmgClassPos | procFlagTakenSpellNoneDmgClassNeg |
		procFlagTakenSpellMagicDmgClassPos | procFlagTakenSpellMagicDmgClassNeg |
		procFlagTakenPeriodic | procFlagTakenDamage
	procReqSpellPhaseProcFlagMask = procSpellProcFlagMask & procDoneHitProcFlagMask
)

// autoAttackProcFlagMask (SpellMgr.h:156): auto-attack events are exempt
// from the triggered-cast suppression in CanSpellTriggerProcOnEvent.
const autoAttackProcFlagMask = procFlagDoneMeleeAutoAttack | procFlagTakenMeleeAutoAttack |
	procFlagDoneRangedAutoAttack | procFlagTakenRangedAutoAttack

// Triggered-cast attributes (SharedDefines.h:516/532/539). ATTR2 is Go's
// AttributesEx1 (Spell.dbc field 6); ATTR3 is Go's AttributesEx3
// (Spell.dbc field 7).
const (
	spellAttr2TriggeredCanTriggerProc  uint32 = 0x40000000
	spellAttr3TriggeredCanTriggerProc2 uint32 = 0x00000200
	spellAttr3CantTriggerProc          uint32 = 0x00010000
	spellAttr2AutoRepeatFlag           uint32 = 0x00000020 // SharedDefines.h:491
)

// ITEM_SUBCLASS_WEAPON_WAND (ItemTemplate.h:368); itemClassWeapon = 2 lives
// in spells.go. C++ builds EquippedItemSubClassMask straight from the DBC
// field (SpellInfo.cpp:843), so the wand check is a bit test on the raw
// field value.
const itemSubclassWeaponWand = 19

// Proc spell type and phase (SpellMgr.h:224-238).
const (
	procSpellTypeNone      uint32 = 0x0000000
	procSpellTypeDamage    uint32 = 0x0000001
	procSpellTypeHeal      uint32 = 0x0000002
	procSpellTypeNoDmgHeal uint32 = 0x0000004
	procSpellTypeMaskAll   uint32 = 0x0000007
	procSpellPhaseNone     uint32 = 0x0000000
	procSpellPhaseCast     uint32 = 0x0000001
	procSpellPhaseHit      uint32 = 0x0000002
	procSpellPhaseFinish   uint32 = 0x0000004
)

// Proc hit results (SpellMgr.h:240-258).
const (
	procHitNone       uint32 = 0x0000000
	procHitNormal     uint32 = 0x0000001
	procHitCritical   uint32 = 0x0000002
	procHitMiss       uint32 = 0x0000004
	procHitFullResist uint32 = 0x0000008
	procHitDodge      uint32 = 0x0000010
	procHitParry      uint32 = 0x0000020
	procHitBlock      uint32 = 0x0000040
	procHitEvade      uint32 = 0x0000080
	procHitImmune     uint32 = 0x0000100
	procHitDeflect    uint32 = 0x0000200
	procHitAbsorb     uint32 = 0x0000400
	procHitReflect    uint32 = 0x0000800
	procHitInterrupt  uint32 = 0x0001000
	procHitFullBlock  uint32 = 0x0002000
)

// Proc attributes (SpellMgr.h:260-268).
const (
	procAttrReqExpOrHonor        uint32 = 0x0000001
	procAttrTriggeredCanProc     uint32 = 0x0000002
	procAttrReqManaCost          uint32 = 0x0000004
	procAttrReqSpellmod          uint32 = 0x0000008
	procAttrReduceProc60         uint32 = 0x0000080
	procAttrCantProcFromItemCast uint32 = 0x0000100
)

// Proc-trigger aura types (SpellAuraDefines.h:84/122/123/311).
const (
	spellAuraProcTriggerSpell          = 42
	spellAuraProcTriggerDamage         = 43
	spellAuraProcTriggerSpellWithValue = 231
)

// isProcTriggerAuraType mirrors the LoadSpellProc trigger-aura subset whose
// C++ HandleProc arm fires a spell or damage (SpellMgr.cpp:1686-1729,
// SpellAuraEffects.cpp:1010-1043): dummy, proc-trigger-spell,
// proc-trigger-damage, and proc-trigger-spell-with-value. The remaining
// isTriggerAura types (reflect, stealth-break, charge-drop auras) have no Go
// HandleProc arm yet.
func isProcTriggerAuraType(auraType uint32) bool {
	switch auraType {
	case spellAuraDummy, spellAuraProcTriggerSpell, spellAuraProcTriggerDamage, spellAuraProcTriggerSpellWithValue:
		return true
	}
	return false
}

// spellProcEntry mirrors SpellProcEntry (SpellMgr.h:270-284) for the
// generated-default branch of SpellMgr::LoadSpellProc (SpellMgr.cpp:1816-1869).
// This server has no spell_proc DB table, so every aura spell carrying DBC
// ProcFlags (Spell.dbc field 34) plus a proc-trigger aura effect gets the
// generated entry, exactly as C++ does when the table holds no row for it.
type spellProcEntry struct {
	SchoolMask      uint32
	SpellFamilyName uint32
	SpellFamilyMask [3]uint32
	ProcFlags       uint32
	SpellTypeMask   uint32
	SpellPhaseMask  uint32
	HitMask         uint32
	AttributesMask  uint32
	Chance          uint32
	Charges         uint32
}

// spellProcEntryFor builds the generated spell_proc entry for an aura spell.
// Returns the entry and the DBC spell; ok is false when the spell has no DBC
// ProcFlags or no proc-trigger aura effect (SpellMgr.cpp:1755-1814).
func (s *session) spellProcEntryFor(auraSpellID uint32) (entry spellProcEntry, auraSpell wotlk.Spell, ok bool) {
	if s == nil || s.server == nil || s.server.Data == nil {
		return entry, auraSpell, false
	}
	spell, found, err := s.server.Data.Spell(auraSpellID)
	if err != nil || !found || spell.ProcTypeMask == 0 {
		return entry, auraSpell, false
	}
	hasTrigger := false
	for i := range spell.Effects {
		eff := &spell.Effects[i]
		if eff.Effect == 0 || !isProcTriggerAuraType(eff.Aura) {
			continue
		}
		hasTrigger = true
		for k := 0; k < 3; k++ {
			entry.SpellFamilyMask[k] |= eff.SpellClassMask[k]
		}
	}
	if !hasTrigger {
		return entry, auraSpell, false
	}
	entry.ProcFlags = spell.ProcTypeMask
	if entry.SpellFamilyMask != [3]uint32{} {
		entry.SpellFamilyName = spell.SpellFamilyName
	}
	entry.SpellTypeMask = procSpellTypeMaskAll
	entry.SpellPhaseMask = procSpellPhaseHit
	entry.Chance = spell.ProcChance
	entry.Charges = spell.ProcCharges
	if spell.ProcTypeMask&procFlagKill != 0 {
		entry.AttributesMask |= procAttrReqExpOrHonor
	}
	// LoadSpellProc's taken-flag fallback (SpellMgr.cpp:1788-1798): proc
	// trigger auras on taken-flagged spells proc from triggered hits anyway.
	if spell.ProcTypeMask&procTakenHitProcFlagMask != 0 {
		entry.AttributesMask |= procAttrTriggeredCanProc
	}
	return entry, spell, true
}

// procEventInfo carries the CanSpellTriggerProcOnEvent inputs (SpellMgr.cpp:502).
// The melee paths fill the masks only; spell-driven paths additionally fill
// eventSpell (the casting spell) and triggered (Spell::IsTriggered,
// Spell.cpp:7501-7504) so the mana-cost, spell-family, and triggered-cast
// gates engage exactly.
type procEventInfo struct {
	typeMask        uint32
	schoolMask      uint32
	spellTypeMask   uint32
	spellPhaseMask  uint32
	hitMask         uint32
	xpOrHonorTarget bool
	triggered       bool
	eventSpell      *wotlk.Spell
}

// canSpellTriggerProcOnEvent mirrors SpellMgr::CanSpellTriggerProcOnEvent
// (SpellMgr.cpp:502-583): the entry's ProcFlags must intersect the event's
// type mask, then the attribute, school, spell-family, spell-type,
// spell-phase, and hit-mask gates apply in C++ order.
func canSpellTriggerProcOnEvent(entry spellProcEntry, ev procEventInfo) bool {
	if ev.typeMask&entry.ProcFlags == 0 {
		return false
	}
	if entry.AttributesMask&procAttrReqExpOrHonor != 0 && !ev.xpOrHonorTarget {
		return false
	}
	if entry.AttributesMask&procAttrReqManaCost != 0 && ev.eventSpell != nil {
		if ev.eventSpell.ManaCost == 0 && ev.eventSpell.ManaCostPct == 0 {
			return false
		}
	}
	if ev.typeMask&(procFlagKilled|procFlagKill|procFlagDeath) != 0 {
		return true
	}
	// Triggered-cast suppression (SpellMgr.cpp:526-539): a triggered event
	// spell cannot proc an entry unless the entry allows it
	// (PROC_ATTR_TRIGGERED_CAN_PROC), the event is an auto-attack
	// (AUTO_ATTACK_PROC_FLAG_MASK, SpellMgr.h:156), or the triggered spell
	// itself carries SPELL_ATTR2_TRIGGERED_CAN_TRIGGER_PROC
	// (SharedDefines.h:516) or SPELL_ATTR3_TRIGGERED_CAN_TRIGGER_PROC_2
	// (SharedDefines.h:532).
	if entry.AttributesMask&procAttrTriggeredCanProc == 0 && ev.typeMask&autoAttackProcFlagMask == 0 &&
		ev.eventSpell != nil && ev.triggered && !eventSpellCanTriggerProc(ev.eventSpell) {
		return false
	}
	if entry.SchoolMask != 0 && ev.schoolMask&entry.SchoolMask == 0 {
		return false
	}
	if ev.typeMask&procSpellProcFlagMask != 0 {
		if ev.eventSpell != nil && !spellAffectedBySpellFamilyMask(entry.SpellFamilyName, entry.SpellFamilyMask, *ev.eventSpell) {
			return false
		}
		if entry.SpellTypeMask != 0 && ev.spellTypeMask&entry.SpellTypeMask == 0 {
			return false
		}
	}
	if ev.typeMask&procReqSpellPhaseProcFlagMask != 0 {
		if ev.spellPhaseMask&entry.SpellPhaseMask == 0 {
			return false
		}
	}
	if ev.typeMask&procTakenHitProcFlagMask != 0 || (ev.typeMask&procDoneHitProcFlagMask != 0 && ev.spellPhaseMask&procSpellPhaseCast == 0) {
		hitMask := entry.HitMask
		if hitMask == 0 {
			if ev.typeMask&procTakenHitProcFlagMask != 0 {
				hitMask = procHitNormal | procHitCritical
			} else {
				hitMask = procHitNormal | procHitCritical | procHitAbsorb
			}
		}
		if ev.hitMask&hitMask == 0 {
			return false
		}
	}
	return true
}

// eventSpellCanTriggerProc mirrors the SpellInfo attribute arm of the
// triggered-cast suppression (SpellMgr.cpp:531-535): a triggered spell
// carrying SPELL_ATTR2_TRIGGERED_CAN_TRIGGER_PROC or
// SPELL_ATTR3_TRIGGERED_CAN_TRIGGER_PROC_2 may proc other auras.
func eventSpellCanTriggerProc(spell *wotlk.Spell) bool {
	return spell != nil &&
		(spell.AttributesEx1&spellAttr2TriggeredCanTriggerProc != 0 ||
			spell.AttributesEx3&spellAttr3TriggeredCanTriggerProc2 != 0)
}

// spellDamageProcTypeMask mirrors Spell::prepareDataForTriggerSystem
// (Spell.cpp:1999-2055): melee damage-class spells use their class flag plus
// the mainhand bit; ranged damage-class spells with SPELL_ATTR2_AUTOREPEAT_FLAG
// (Auto Shot, spell 75) use DONE_RANGED_AUTO_ATTACK, other ranged spells their
// class flag; wand auto attacks (Shoot, spell 5019 — weapon class, wand
// subclass mask bit, AUTOREPEAT_FLAG) use DONE_RANGED_AUTO_ATTACK; every other
// spell on the damage path is negative, magic class or none class per its
// DmgClass (Spell.dbc field 213).
func spellDamageProcTypeMask(spell wotlk.Spell) uint32 {
	switch spell.DefenseType {
	case spellDamageClassMelee:
		// Spell::prepareDataForTriggerSystem ORs the mainhand/offhand bit
		// (Spell.cpp:2009-2012); the direct-spell-damage path always uses
		// the base attack type, so the mainhand bit applies.
		return procFlagDoneSpellMeleeDmgClass | procFlagDoneMainhandAttack
	case spellDamageClassRanged:
		// Spell.cpp:2018-2023 — auto attack.
		if spell.AttributesEx1&spellAttr2AutoRepeatFlag != 0 {
			return procFlagDoneRangedAutoAttack
		}
		return procFlagDoneSpellRangedDmgClass
	default:
		// Spell.cpp:2026-2034 — wands auto attack.
		if spell.EquippedItemClass == itemClassWeapon &&
			spell.EquippedItemSubClass&(1<<itemSubclassWeaponWand) != 0 &&
			spell.AttributesEx1&spellAttr2AutoRepeatFlag != 0 {
			return procFlagDoneRangedAutoAttack
		}
		if spell.DefenseType == spellDamageClassMagic {
			return procFlagDoneSpellMagicDmgClassNeg
		}
		return procFlagDoneSpellNoneDmgClassNeg
	}
}

// spellHealProcTypeMask mirrors the done-side half of the
// DoDamageAndTriggers type-mask fallback (Spell.cpp:2458-2492) for the heal
// path: a direct heal runs with m_healing > 0, so the spell is positive by
// construction and magic damage-class spells (Spell.dbc field 213) use
// DONE_SPELL_MAGIC_DMG_CLASS_POS while none-class spells use
// DONE_SPELL_NONE_DMG_CLASS_POS. Spell::prepareDataForTriggerSystem
// (Spell.cpp:1999-2036) fills the flags for melee/ranged/wand spells before
// the fallback runs, so those damage classes never reach this branch for
// heals; fail closed.
func spellHealProcTypeMask(spell wotlk.Spell) uint32 {
	switch spell.DefenseType {
	case spellDamageClassMagic:
		return procFlagDoneSpellMagicDmgClassPos
	case spellDamageClassNone:
		return procFlagDoneSpellNoneDmgClassPos
	default:
		return procFlagNone
	}
}

// spellHealTakenProcTypeMask mirrors the taken-side half of the
// DoDamageAndTriggers type-mask fallback (Spell.cpp:2462-2473) for the heal
// path: the positivity fallback assigns the TAKEN_SPELL_*_DMG_CLASS_POS flags
// to the heal target's auras alongside the done-side flags. Fails closed for
// non-magic/none damage classes, matching the done-side mask.
func spellHealTakenProcTypeMask(spell wotlk.Spell) uint32 {
	switch spell.DefenseType {
	case spellDamageClassMagic:
		return procFlagTakenSpellMagicDmgClassPos
	case spellDamageClassNone:
		return procFlagTakenSpellNoneDmgClassPos
	default:
		return procFlagNone
	}
}

// spellNoDmgHealPositive mirrors the per-effect positivity fallback for the
// no-damage arm (Spell.cpp:2447-2457): with zero healing the positivity is
// not assumed from m_healing but read from IsPositiveEffect over the effects
// in the mask. executeSpellHeal is only reached by heal effect types, and
// _isPositiveEffectImpl (SpellInfo.cpp:3501-3505, 3571-3577) reports
// SPELL_EFFECT_HEAL/HEAL_PCT/HEAL_MAX_HEALTH/HEAL_MECHANICAL as positive; the
// one DBC-visible exception is SPELL_ATTR0_NEGATIVE_1, which forces the whole
// spell negative (SpellInfo.cpp:3397-3398). The custom CU_NEGATIVE_EFF bits
// (computed at C++ load time from spell_custom_attr / triggered-spell
// recursion) and the full EffectMask sweep over sibling effects have no Go
// model; non-heal effect types fail closed here.
func spellNoDmgHealPositive(spell wotlk.Spell, effIndex int) bool {
	if effIndex < 0 || effIndex >= len(spell.Effects) {
		return false
	}
	switch spell.Effects[effIndex].Effect {
	case 10, // SPELL_EFFECT_HEAL
		spellEffectHealMaxHealth,
		spellEffectHealMechanical,
		spellEffectHealPct:
		return spell.Attributes&spellAttr0Negative1 == 0
	default:
		return false
	}
}

// spellNoDmgHealProcTypeMask mirrors the done-side half of the
// DoDamageAndTriggers type-mask fallback (Spell.cpp:2458-2492) for the
// no-damage arm: magic and none damage classes take the POS or NEG flag pair
// per the per-effect positivity above; melee/ranged/wand classes were
// pre-filled by Spell::prepareDataForTriggerSystem (Spell.cpp:1999-2036) and
// fail closed here, matching the heal-arm masks.
func spellNoDmgHealProcTypeMask(spell wotlk.Spell, positive bool) uint32 {
	switch spell.DefenseType {
	case spellDamageClassMagic:
		if positive {
			return procFlagDoneSpellMagicDmgClassPos
		}
		return procFlagDoneSpellMagicDmgClassNeg
	case spellDamageClassNone:
		if positive {
			return procFlagDoneSpellNoneDmgClassPos
		}
		return procFlagDoneSpellNoneDmgClassNeg
	default:
		return procFlagNone
	}
}

// spellNoDmgHealTakenProcTypeMask mirrors the taken-side half of the
// type-mask fallback for the no-damage arm: the same positivity fallback
// assigns the TAKEN_SPELL_*_DMG_CLASS_POS/NEG flags to the target's auras
// alongside the done-side flags. Fails closed for non-magic/none damage
// classes, matching the done-side mask.
func spellNoDmgHealTakenProcTypeMask(spell wotlk.Spell, positive bool) uint32 {
	switch spell.DefenseType {
	case spellDamageClassMagic:
		if positive {
			return procFlagTakenSpellMagicDmgClassPos
		}
		return procFlagTakenSpellMagicDmgClassNeg
	case spellDamageClassNone:
		if positive {
			return procFlagTakenSpellNoneDmgClassPos
		}
		return procFlagTakenSpellNoneDmgClassNeg
	default:
		return procFlagNone
	}
}

// procSpellNoDmgHealAuraTriggers evaluates real aura procs on the done side
// of a zero-heal spell: the no-damage arm of Unit::ProcDamageAndSpellFor via
// Spell::TargetInfo::DoDamageAndTriggers (Spell.cpp:2563-2579, 2581-2586).
// The event carries PROC_SPELL_TYPE_NO_DMG_HEAL ("other spells",
// SpellMgr.h:206) with PROC_SPELL_PHASE_HIT; the type mask comes from the
// per-effect positivity fallback rather than the heal arm's assumed POS.
// Heals always land in Go's model, so the hit mask is PROC_HIT_NORMAL —
// matching C++ createProcHitMask (Unit.cpp:10224-10234) for a landed hit
// with zero damage (MissCondition == SPELL_MISS_NONE, no block/absorb/crit
// bits). Spells with SPELL_ATTR3_CANT_TRIGGER_PROC never reach the loop
// (Spell.cpp:2441). The event carries the casting spell and the triggered
// state (Spell::IsTriggered, Spell.cpp:7501-7504) so the
// CanSpellTriggerProcOnEvent mana-cost, spell-family, and triggered-cast
// gates engage exactly. The trigger spell targets the spell target.
func (s *session) procSpellNoDmgHealAuraTriggers(ctx context.Context, targetGUID uint64, spellID uint32, effIndex int) {
	if s == nil || s.server == nil || s.server.Data == nil {
		return
	}
	spell, found, err := s.server.Data.Spell(spellID)
	if err != nil || !found {
		return
	}
	if spell.AttributesEx3&spellAttr3CantTriggerProc != 0 {
		return
	}
	typeMask := spellNoDmgHealProcTypeMask(spell, spellNoDmgHealPositive(spell, effIndex))
	if typeMask == procFlagNone {
		return
	}
	schoolMask := spell.SchoolMask
	if schoolMask == 0 {
		schoolMask = 1
	}
	spellCopy := spell
	s.procAuraTriggerLoop(ctx, targetGUID, procEventInfo{
		typeMask:       typeMask,
		schoolMask:     schoolMask,
		spellTypeMask:  procSpellTypeNoDmgHeal,
		spellPhaseMask: procSpellPhaseHit,
		hitMask:        procHitNormal,
		triggered:      s.triggeredNoProcEvents > 0,
		eventSpell:     &spellCopy,
	})
}

// procSpellNoDmgHealTakenAuraTriggers evaluates real aura procs on the taken
// side of a zero-heal spell: the victim-side half of the no-damage arm
// (Spell.cpp:2462-2473, 2563-2579, 2581-2586). The event mirrors the
// done-side no-damage event exactly (PROC_SPELL_TYPE_NO_DMG_HEAL,
// PROC_SPELL_PHASE_HIT, PROC_HIT_NORMAL, the casting spell, and the
// triggered state) but the type mask is the taken-side positivity fallback,
// so the target's TAKEN_SPELL_*_DMG_CLASS_POS/NEG auras gate against it.
// Runs on the target's session so its own auras gate; the trigger spell
// targets the caster.
func (s *session) procSpellNoDmgHealTakenAuraTriggers(ctx context.Context, casterGUID uint64, spellID uint32, effIndex int) {
	if s == nil || s.server == nil || s.server.Data == nil {
		return
	}
	spell, found, err := s.server.Data.Spell(spellID)
	if err != nil || !found {
		return
	}
	if spell.AttributesEx3&spellAttr3CantTriggerProc != 0 {
		return
	}
	typeMask := spellNoDmgHealTakenProcTypeMask(spell, spellNoDmgHealPositive(spell, effIndex))
	if typeMask == procFlagNone {
		return
	}
	schoolMask := spell.SchoolMask
	if schoolMask == 0 {
		schoolMask = 1
	}
	spellCopy := spell
	s.procAuraTriggerLoop(ctx, casterGUID, procEventInfo{
		typeMask:       typeMask,
		schoolMask:     schoolMask,
		spellTypeMask:  procSpellTypeNoDmgHeal,
		spellPhaseMask: procSpellPhaseHit,
		hitMask:        procHitNormal,
		triggered:      s.triggeredNoProcEvents > 0,
		eventSpell:     &spellCopy,
	})
}

// spellDamageProcHitMask mirrors the hit-mask derivation for spell damage
// events (DamageInfo ctor from SpellNonMeleeDamage, Unit.cpp:183-192;
// createProcHitMask, Unit.cpp:10179-10247): miss, immunity, and full resist
// map to their bits, crits to the critical bit, everything else to normal,
// with the absorb bit ORed in whenever any damage was absorbed. A full
// absorb (damage - absorb == 0 -> HITINFO_FULL_ABSORB, Unit.cpp:1128)
// nullifies the hit: the normal/critical bit is suppressed while the
// absorb bit is kept (createProcHitMask's damageNullified arm).
func spellDamageProcHitMask(isHit, immune, fullyResisted, fullAbsorb, crit bool, absorbed uint32) uint32 {
	var hitMask uint32
	switch {
	case !isHit:
		hitMask = procHitMiss
	case immune:
		hitMask = procHitImmune
	case fullyResisted:
		hitMask = procHitFullResist
	case fullAbsorb:
		hitMask = procHitNone
	case crit:
		hitMask = procHitCritical
	default:
		hitMask = procHitNormal
	}
	if absorbed > 0 {
		hitMask |= procHitAbsorb
	}
	return hitMask
}

// procSpellHitAuraTriggers evaluates real aura procs on the done side of a
// spell damage hit: the spell-damage arm of Unit::ProcDamageAndSpellFor via
// Spell::TargetInfo::DoDamageAndTriggers (Spell.cpp:2427-2540). The event
// carries the casting spell and the triggered state (Spell::IsTriggered,
// Spell.cpp:7501-7504), so the CanSpellTriggerProcOnEvent mana-cost,
// spell-family, and triggered-cast gates engage exactly. Spells with
// SPELL_ATTR3_CANT_TRIGGER_PROC never reach the loop (Spell.cpp:2441).
// The trigger spell targets the victim.
func (s *session) procSpellHitAuraTriggers(ctx context.Context, targetGUID uint64, spellID uint32, isHit, immune, fullyResisted, fullAbsorb, crit bool, absorbed uint32) {
	if s == nil || s.server == nil || s.server.Data == nil {
		return
	}
	spell, found, err := s.server.Data.Spell(spellID)
	if err != nil || !found {
		return
	}
	if spell.AttributesEx3&spellAttr3CantTriggerProc != 0 {
		return
	}
	schoolMask := spell.SchoolMask
	if schoolMask == 0 {
		schoolMask = 1
	}
	spellCopy := spell
	s.procAuraTriggerLoop(ctx, targetGUID, procEventInfo{
		typeMask:       spellDamageProcTypeMask(spell),
		schoolMask:     schoolMask,
		spellTypeMask:  procSpellTypeDamage,
		spellPhaseMask: procSpellPhaseHit,
		hitMask:        spellDamageProcHitMask(isHit, immune, fullyResisted, fullAbsorb, crit, absorbed),
		triggered:      s.triggeredNoProcEvents > 0,
		eventSpell:     &spellCopy,
	})
}

// procSpellHealAuraTriggers evaluates real aura procs on the done side of a
// direct heal: the heal arm of Unit::ProcDamageAndSpellFor via
// Spell::TargetInfo::DoDamageAndTriggers (Spell.cpp:2493-2513, 2581-2586).
// The event mirrors C++ exactly: the spell type is PROC_SPELL_TYPE_HEAL with
// PROC_SPELL_PHASE_HIT, crits carry PROC_HIT_CRITICAL and other heals
// PROC_HIT_NORMAL (Spell.cpp:2497-2507), and the type mask comes from the
// heal positivity fallback. Spells with SPELL_ATTR3_CANT_TRIGGER_PROC never
// reach the loop (Spell.cpp:2440), and a zero heal takes C++'s no-damage
// arm (PROC_SPELL_TYPE_NO_DMG_HEAL, Spell.cpp:2563-2579), not the heal arm.
// The event carries the casting spell and the triggered state
// (Spell::IsTriggered, Spell.cpp:7501-7504) so the
// CanSpellTriggerProcOnEvent mana-cost, spell-family, and triggered-cast
// gates engage exactly. The trigger spell targets the heal target.
func (s *session) procSpellHealAuraTriggers(ctx context.Context, targetGUID uint64, spellID uint32, heal uint32, crit bool) {
	if s == nil || s.server == nil || s.server.Data == nil {
		return
	}
	if heal == 0 {
		return
	}
	spell, found, err := s.server.Data.Spell(spellID)
	if err != nil || !found {
		return
	}
	if spell.AttributesEx3&spellAttr3CantTriggerProc != 0 {
		return
	}
	typeMask := spellHealProcTypeMask(spell)
	if typeMask == procFlagNone {
		return
	}
	schoolMask := spell.SchoolMask
	if schoolMask == 0 {
		schoolMask = 1
	}
	hitMask := uint32(procHitNormal)
	if crit {
		hitMask = procHitCritical
	}
	spellCopy := spell
	s.procAuraTriggerLoop(ctx, targetGUID, procEventInfo{
		typeMask:       typeMask,
		schoolMask:     schoolMask,
		spellTypeMask:  procSpellTypeHeal,
		spellPhaseMask: procSpellPhaseHit,
		hitMask:        hitMask,
		triggered:      s.triggeredNoProcEvents > 0,
		eventSpell:     &spellCopy,
	})
}

// procSpellHealTakenAuraTriggers evaluates real aura procs on the taken side
// of a direct heal: the victim-side half of Unit::ProcDamageAndSpellFor via
// Spell::TargetInfo::DoDamageAndTriggers (Spell.cpp:2462-2473, 2581-2586).
// The event mirrors the done-side heal event exactly (PROC_SPELL_TYPE_HEAL,
// PROC_SPELL_PHASE_HIT, crit -> PROC_HIT_CRITICAL else PROC_HIT_NORMAL, the
// casting spell, and the triggered state) but the type mask is the taken-side
// positivity fallback, so the heal target's TAKEN_SPELL_*_DMG_CLASS_POS auras
// gate against it. Runs on the target's session so its own auras gate; the
// trigger spell targets the healer. Spells with SPELL_ATTR3_CANT_TRIGGER_PROC
// never reach the loop (Spell.cpp:2440), and a zero heal takes C++'s
// no-damage arm (PROC_SPELL_TYPE_NO_DMG_HEAL, Spell.cpp:2563-2579), not the
// heal arm.
func (s *session) procSpellHealTakenAuraTriggers(ctx context.Context, healerGUID uint64, spellID uint32, heal uint32, crit bool) {
	if s == nil || s.server == nil || s.server.Data == nil {
		return
	}
	if heal == 0 {
		return
	}
	spell, found, err := s.server.Data.Spell(spellID)
	if err != nil || !found {
		return
	}
	if spell.AttributesEx3&spellAttr3CantTriggerProc != 0 {
		return
	}
	typeMask := spellHealTakenProcTypeMask(spell)
	if typeMask == procFlagNone {
		return
	}
	schoolMask := spell.SchoolMask
	if schoolMask == 0 {
		schoolMask = 1
	}
	hitMask := uint32(procHitNormal)
	if crit {
		hitMask = procHitCritical
	}
	spellCopy := spell
	s.procAuraTriggerLoop(ctx, healerGUID, procEventInfo{
		typeMask:       typeMask,
		schoolMask:     schoolMask,
		spellTypeMask:  procSpellTypeHeal,
		spellPhaseMask: procSpellPhaseHit,
		hitMask:        hitMask,
		triggered:      s.triggeredNoProcEvents > 0,
		eventSpell:     &spellCopy,
	})
}

// meleeOutcomeProcHitMask mirrors the DamageInfo constructor's hit-mask
// derivation for melee outcomes (Unit.cpp:155-179): block, crushing, and
// glancing blows count as normal hits; only crits set the critical bit.
func meleeOutcomeProcHitMask(outcome protocol.MeleeHitOutcome) uint32 {
	switch outcome {
	case protocol.MeleeHitMiss:
		return procHitMiss
	case protocol.MeleeHitDodge:
		return procHitDodge
	case protocol.MeleeHitParry:
		return procHitParry
	case protocol.MeleeHitEvade:
		return procHitEvade
	case protocol.MeleeHitCrit:
		return procHitCritical
	case protocol.MeleeHitNormal, protocol.MeleeHitBlock, protocol.MeleeHitGlancing, protocol.MeleeHitCrushing:
		return procHitNormal
	}
	return procHitNone
}

// rollAuraProcChance mirrors Aura::CalcProcChance (SpellAuras.cpp:2164-2190)
// for generated entries: DBC ProcChance, the SPELLMOD_CHANCE_OF_SUCCESS
// modifier, and the over-60 level reduction. Generated entries never carry
// ProcsPerMinute, so the PPM arm is vacuous.
func (s *session) rollAuraProcChance(entry spellProcEntry, auraSpell wotlk.Spell) bool {
	chance := float64(entry.Chance)
	if s != nil {
		chance = float64(s.applySpellMod(auraSpell, spellModChanceOfSuccess, int32(chance)))
		if entry.AttributesMask&procAttrReduceProc60 != 0 && s.player != nil && s.player.Level > 60 {
			chance = math.Max(0, (1-float64(s.player.Level-60)/30)*chance)
		}
	}
	return rand.Float64()*100 < chance
}

// procAuraTriggers evaluates real aura procs on a melee hit: the done-side
// half of Unit::ProcDamageAndSpellFor's aura loop (Unit.cpp:10355-10380 via
// TriggerAurasProcOnEvent). The trigger spell targets the victim.
func (s *session) procAuraTriggers(ctx context.Context, target combatTarget, attType protocol.WeaponAttackType, outcome protocol.MeleeHitOutcome) {
	typeMask := procFlagDoneMeleeAutoAttack | procFlagDoneMainhandAttack
	if attType == protocol.OffAttack {
		typeMask = procFlagDoneMeleeAutoAttack | procFlagDoneOffhandAttack
	}
	s.procAuraTriggerLoop(ctx, target.GUID, procEventInfo{
		typeMask:       typeMask,
		schoolMask:     spellSchoolMaskNormal,
		spellTypeMask:  procSpellTypeNone,
		spellPhaseMask: procSpellPhaseNone,
		hitMask:        meleeOutcomeProcHitMask(outcome),
	})
}

// procVictimAuraTriggers evaluates real aura procs on the taken side of a
// melee hit: the victim-side half of Unit::ProcDamageAndSpellFor's aura loop
// (Unit.cpp:1194-1198 — ProcVictim = PROC_FLAG_TAKEN_MELEE_AUTO_ATTACK for
// BASE_ATTACK and OFF_ATTACK, with no mainhand/offhand arm on the victim
// side; Unit.cpp:10385-10448 TriggerAurasProcOnEvent). The trigger spell
// targets the attacker.
func (s *session) procVictimAuraTriggers(ctx context.Context, attackerGUID uint64, outcome protocol.MeleeHitOutcome) {
	s.procAuraTriggerLoop(ctx, attackerGUID, procEventInfo{
		typeMask:       procFlagTakenMeleeAutoAttack,
		schoolMask:     spellSchoolMaskNormal,
		spellTypeMask:  procSpellTypeNone,
		spellPhaseMask: procSpellPhaseNone,
		hitMask:        meleeOutcomeProcHitMask(outcome),
	})
}

// procRangedAutoAttackAuraTriggers evaluates real aura procs on the done side
// of a ranged auto attack (Auto Shot / Shoot): the spell-damage arm of
// Unit::ProcDamageAndSpellFor via Spell::TargetInfo::DoDamageAndTriggers
// (Spell.cpp:2427-2540) with m_procAttacker = DONE_RANGED_AUTO_ATTACK from
// Spell::prepareDataForTriggerSystem (Spell.cpp:2018-2034). The event carries
// the auto-shot spell and the triggered state (Spell::IsTriggered,
// Spell.cpp:7501-7504), so the CanSpellTriggerProcOnEvent mana-cost,
// spell-family, and triggered-cast gates engage exactly; the ranged path rolls
// melee-style outcomes, so the hit mask mirrors the DamageInfo melee ctor
// (Unit.cpp:155-179). Spells with SPELL_ATTR3_CANT_TRIGGER_PROC never reach
// the loop (Spell.cpp:2441). The trigger spell targets the victim.
func (s *session) procRangedAutoAttackAuraTriggers(ctx context.Context, targetGUID uint64, spellID uint32, outcome protocol.MeleeHitOutcome) {
	if s == nil || s.server == nil || s.server.Data == nil {
		return
	}
	spell, found, err := s.server.Data.Spell(spellID)
	if err != nil || !found {
		return
	}
	if spell.AttributesEx3&spellAttr3CantTriggerProc != 0 {
		return
	}
	schoolMask := spell.SchoolMask
	if schoolMask == 0 {
		schoolMask = 1
	}
	spellCopy := spell
	s.procAuraTriggerLoop(ctx, targetGUID, procEventInfo{
		typeMask:       spellDamageProcTypeMask(spell),
		schoolMask:     schoolMask,
		spellTypeMask:  procSpellTypeDamage,
		spellPhaseMask: procSpellPhaseHit,
		hitMask:        meleeOutcomeProcHitMask(outcome),
		triggered:      s.triggeredNoProcEvents > 0,
		eventSpell:     &spellCopy,
	})
}

// procRangedVictimAuraTriggers evaluates real aura procs on the taken side of
// a ranged auto attack (Spell.cpp:2020/2032 — ProcVictim =
// PROC_FLAG_TAKEN_RANGED_AUTO_ATTACK, with no mainhand/offhand arm on the
// victim side; the damage arm of DoDamageAndTriggers ORs
// PROC_FLAG_TAKEN_DAMAGE, Spell.cpp:2545). The event carries the auto-shot
// spell's school mask, matching the damage event's school on both sides of
// DoDamageAndTriggers. The trigger spell targets the attacker.
func (s *session) procRangedVictimAuraTriggers(ctx context.Context, attackerGUID uint64, spellID uint32, outcome protocol.MeleeHitOutcome) {
	if s == nil || s.server == nil || s.server.Data == nil {
		return
	}
	spell, found, err := s.server.Data.Spell(spellID)
	if err != nil || !found {
		return
	}
	schoolMask := spell.SchoolMask
	if schoolMask == 0 {
		schoolMask = 1
	}
	s.procAuraTriggerLoop(ctx, attackerGUID, procEventInfo{
		typeMask:       procFlagTakenRangedAutoAttack | procFlagTakenDamage,
		schoolMask:     schoolMask,
		spellTypeMask:  procSpellTypeNone,
		spellPhaseMask: procSpellPhaseNone,
		hitMask:        meleeOutcomeProcHitMask(outcome),
	})
}

// procAuraTriggerLoop runs one aura-proc pass over the player's active auras:
// each aura with a generated spell_proc entry runs the
// CanSpellTriggerProcOnEvent gate against the event; on pass, the chance roll
// fires the aura effect's trigger spell on triggerTargetGUID
// (AuraEffect::HandleProcTriggerSpellAuraProc, SpellAuraEffects.cpp:5654-5713).
func (s *session) procAuraTriggerLoop(ctx context.Context, triggerTargetGUID uint64, ev procEventInfo) {
	if s == nil || s.player == nil || len(s.activeAuras) == 0 {
		return
	}
	auras := make([]*activeAura, 0, len(s.activeAuras))
	for _, aura := range s.activeAuras {
		auras = append(auras, aura)
	}
	for _, aura := range auras {
		if aura == nil || aura.Stopped {
			continue
		}
		entry, auraSpell, ok := s.spellProcEntryFor(aura.SpellID)
		if !ok {
			continue
		}
		if entry.Charges > 0 && aura.RemainingCharges == 0 {
			continue
		}
		if !canSpellTriggerProcOnEvent(entry, ev) {
			continue
		}
		var triggerSpell uint32
		var withValue bool
		for i := range auraSpell.Effects {
			eff := &auraSpell.Effects[i]
			if eff.Effect == 0 || !isProcTriggerAuraType(eff.Aura) {
				continue
			}
			if eff.Aura == spellAuraProcTriggerDamage {
				continue
			}
			triggerSpell = eff.TriggerSpell
			withValue = eff.Aura == spellAuraProcTriggerSpellWithValue
			break
		}
		if triggerSpell == 0 {
			continue
		}
		if !s.rollAuraProcChance(entry, auraSpell) {
			continue
		}
		if entry.Charges > 0 {
			aura.RemainingCharges--
			if aura.RemainingCharges == 0 {
				s.removeAura(aura.SpellID)
			}
		}
		if withValue {
			s.castSpellDirectWithBasePoint(ctx, triggerSpell, triggerTargetGUID, aura.Amount)
		} else {
			s.castSpellDirect(ctx, triggerSpell, triggerTargetGUID)
		}
	}
}
