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

// enchantProcAttrWhiteHit is ENCHANT_PROC_ATTR_WHITE_HIT (SpellMgr.h:270):
// the enchant shall only proc off white hits, never from abilities/spells.
const enchantProcAttrWhiteHit uint32 = 0x00000001

// getEnchantProcAttr returns the spell_enchant_proc_data AttributesMask for an
// enchant ID, mirroring SpellMgr::LoadSpellEnchantProcData (SpellMgr.cpp:2080).
// A missing table or query error degrades to an empty map, matching C++'s
// load-empty behavior; ok is false when no row exists.
func (s *Server) getEnchantProcAttr(enchantID uint32) (uint32, bool) {
	if s == nil {
		return 0, false
	}
	s.enchantProcAttrMu.RLock()
	if s.enchantProcAttrLoaded {
		attr, ok := s.enchantProcAttrs[enchantID]
		s.enchantProcAttrMu.RUnlock()
		return attr, ok
	}
	s.enchantProcAttrMu.RUnlock()

	s.enchantProcAttrMu.Lock()
	defer s.enchantProcAttrMu.Unlock()
	if s.enchantProcAttrLoaded {
		attr, ok := s.enchantProcAttrs[enchantID]
		return attr, ok
	}
	s.enchantProcAttrs = make(map[uint32]uint32)
	s.enchantProcAttrLoaded = true
	if s.WorldStore != nil && s.WorldStore.DB != nil {
		rows, err := s.WorldStore.DB.Query(`SELECT EnchantID, AttributesMask FROM spell_enchant_proc_data`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var entry, attr uint32
				if err := rows.Scan(&entry, &attr); err == nil && entry > 0 {
					s.enchantProcAttrs[entry] = attr
				}
			}
		}
	}
	attr, ok := s.enchantProcAttrs[enchantID]
	return attr, ok
}

// enchantProcWhiteHitOnly reports whether the enchant's spell_enchant_proc_data
// row flags ENCHANT_PROC_ATTR_WHITE_HIT (SpellMgr.h:270). C++ applies the arm
// only when a row exists (Player.cpp:8180: entry && ...), so a missing row
// fails open.
func (s *session) enchantProcWhiteHitOnly(enchantID uint32) bool {
	if s == nil || s.server == nil {
		return false
	}
	attr, ok := s.server.getEnchantProcAttr(enchantID)
	return ok && attr&enchantProcAttrWhiteHit != 0
}

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
	// Deathbringer's Will (DBW) - 45s ICD. Buff IDs are C++-exact
	// vs the DeathbringersWill enum (spell_item.cpp:543-554):
	// normal Str 71484 / Agi 71485 / AP 71486 / Crit 71491 /
	// Haste 71492, heroic Agi 71556 / AP 71558 / Crit 71559 /
	// Haste 71560 / Str 71561.
	ProcSpellDBWAgilityNorm  = 71485 // +600 Agi (Taunka)
	ProcSpellDBWStrengthNorm = 71484 // +600 Str (Vrykul)
	ProcSpellDBWAPNorm       = 71486 // +1200 AP (Iron Dwarf)
	ProcSpellDBWCritNorm     = 71491 // +600 Crit (Iron Dwarf)
	ProcSpellDBWHasteNorm    = 71492 // +600 Haste (Vrykul)
	ProcSpellDBWAgilityHero  = 71556 // +700 Agi
	ProcSpellDBWStrengthHero = 71561 // +700 Str
	ProcSpellDBWAPHero       = 71558 // +1400 AP
	ProcSpellDBWCritHero     = 71559 // +700 Crit
	ProcSpellDBWHasteHero    = 71560 // +700 Haste

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
// Formula: chance = floor((weaponSpeedMs * PPM) / 600.0) percent, rolled as
// roll_chance_f (rand*100 < chance).
// Reference: TrinityCore Unit::GetPPMProcChance (Unit.cpp:8296-8308) — the
// percent is floored before the roll, so a 2.6s weapon at 1.0 PPM procs at
// 4.0%, not 4.33%.
func RollPPMChance(ppm float64, weaponSpeedMs uint32) bool {
	if ppm <= 0 || weaponSpeedMs == 0 {
		return false
	}
	chancePct := math.Floor((float64(weaponSpeedMs) * ppm) / 600.0)
	if chancePct <= 0 {
		return false
	}
	if chancePct >= 100 {
		return true
	}
	return rand.Float64()*100 < chancePct
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

// advantageT10ProcAllowed mirrors spell_dk_advantage_t10_4p::CheckProc
// (spell_dk.cpp:229-247): the T10 4P melee bonus procs only when the
// actor is a Death Knight player with every rune on cooldown
// (GetRuneCooldown(i) != 0 for all MAX_RUNES). The default trigger
// (70657) is DBC-driven, so this gate alone is the port; Lua has no
// rune-cooldown bridge, so there is no ev6 twin.
func (s *session) advantageT10ProcAllowed(ev procEventInfo) bool {
	if s == nil || s.player == nil || s.player.Class != classDeathKnight {
		return false
	}
	if ev.actorGUID != s.playerGUID {
		return false
	}
	if s.runes == nil {
		return false
	}
	nowMs := time.Now().UnixMilli()
	for i := 0; i < maxRunes; i++ {
		if s.runes.runeCooldownRemainingMs(i, nowMs) == 0 {
			return false
		}
	}
	return true
}

// hungeringColdProcAllowed mirrors spell_dk_hungering_cold::CheckProc
// (spell_dk.cpp:1371-1379): melee damage (no triggering spell) always
// procs; a triggering spell procs unless its Dispel is DISPEL_DISEASE.
// The default trigger is DBC-driven, so this gate alone is the port,
// like the 70656 gate below. DORMANT until Spell.dbc ships the 51209
// proc entry.
func (s *session) hungeringColdProcAllowed(ev procEventInfo) bool {
	if ev.eventSpell == nil {
		return true
	}
	return ev.eventSpell.DispelType != dispelDisease
}

// burningDeterminationProcAllowed mirrors
// spell_mage_burning_determination::CheckProc (spell_mage.cpp:272-281):
// the aura only procs when the triggering spell's mechanic mask
// carries MECHANIC_INTERRUPT (26) or MECHANIC_SILENCE (9)
// (SharedDefines.h:1366,1383). The mask is the OR over the triggering
// spell's effect mechanics, the Second Wind precedent in
// scriptAuraProcCast below. The default trigger is DBC-driven, so this
// gate alone is the port. Both ranks share the loader
// (spell_mage.cpp:261,1486). DORMANT until Spell.dbc ships the
// 54747/54748 proc entries.
func (s *session) burningDeterminationProcAllowed(ev procEventInfo) bool {
	if ev.eventSpell == nil {
		return false
	}
	var mechanicMask uint32
	for _, eff := range ev.eventSpell.Effects {
		if eff.Mechanic < 32 {
			mechanicMask |= uint32(1) << eff.Mechanic
		}
	}
	return mechanicMask&((uint32(1)<<26)|(uint32(1)<<9)) != 0
}

// combustionCheckProc mirrors spell_mage_combustion::CheckProc
// (spell_mage.cpp:395-409): a non-critical hit casts the Combustion
// proc aura (28682) on the actor, triggered, and takes no charge; a
// critical hit lets the default proc action run. The cast rides the
// aura-triggered guards the proc arms use, so the 28682 cast cannot
// re-proc Combustion itself. DORMANT until Spell.dbc ships the 11129
// proc entry.
func (s *session) combustionCheckProc(ctx context.Context, ev procEventInfo) bool {
	if ev.hitMask&procHitCritical != 0 {
		return true
	}
	if s == nil || s.player == nil || s.server == nil || ev.actorGUID == 0 {
		return false
	}
	s.triggeredByAuraCast = true
	s.triggeredByAuraSpell = 11129
	s.server.fireScriptProcCast(ctx, s.player.Map, s.player.InstanceID, scriptProcCast{spellID: 28682, casterGUID: ev.actorGUID, targetGUID: ev.actorGUID})
	s.triggeredByAuraCast = false
	s.triggeredByAuraSpell = 0
	return false
}

// focusMagicProcAllowed mirrors spell_mage_focus_magic::CheckProc
// (spell_mage.cpp:737-741): the proc target is the aura's caster (the
// mage who cast Focus Magic on the holder) and the proc only fires while
// that caster is alive. The caster is always a mage player, so the
// session lookup covers the live cases; an offline caster has no session
// and the gate stays closed, matching C++ GetCaster() returning null.
// DORMANT until Spell.dbc ships the 54646 proc entry.
func (s *session) focusMagicProcAllowed(aura *activeAura) bool {
	if aura == nil || aura.CasterGUID == 0 {
		return false
	}
	if s == nil || s.server == nil {
		return false
	}
	if casterSess := s.server.findSessionByGUID(aura.CasterGUID); casterSess != nil && casterSess.player != nil {
		return casterSess.player.Health > 0
	}
	return false
}

// genExtraEffectsProcAllowed mirrors
// spell_mage_gen_extra_effects::CheckProc (spell_mage.cpp:784-799): the
// actor never procs from an event whose proc target is itself (the
// Arcane Missiles double-proc guard), and a live T8 4P bonus aura
// (64869 EFFECT_0) suppresses the proc on its own chance roll
// (roll_chance_i on the effect amount; a missing aura never suppresses).
// DORMANT until Spell.dbc ships the 48108 proc entry.
func (s *session) genExtraEffectsProcAllowed(ev procEventInfo, triggerTargetGUID uint64) bool {
	if ev.actorGUID != 0 && ev.actorGUID == triggerTargetGUID {
		return false
	}
	if s == nil || s.server == nil || ev.actorGUID == 0 {
		return true
	}
	if actorSess := s.server.findSessionByGUID(ev.actorGUID); actorSess != nil {
		if amt, ok := actorSess.auraEffectAmount(64869, 0); ok && amt > 0 && rand.Intn(100) < int(amt) {
			return false
		}
	}
	return true
}

// glyphOfArcaneShotProcAllowed mirrors
// spell_hun_glyph_of_arcane_shot::CheckProc (spell_hunter.cpp:430-451):
// the proc target must carry a sting cast by the holder — hunter family,
// SpellFamilyFlags HasFlag(0xC000, 0x1080) (Util.h:401-404, OR across
// parts: Serpent, Viper, Scorpid, Wyvern Sting). The aura scan is the
// chimera-shot scan (spells.go) over the hit unit's auras, candidates
// collected under the aura lock and DBC-read after it.
func (s *session) glyphOfArcaneShotProcAllowed(targetGUID uint64, casterGUID uint64) bool {
	if s == nil || s.server == nil || s.server.Data == nil || targetGUID == 0 {
		return false
	}
	var candidates []uint32
	if targetSess := s.server.findSessionByGUID(targetGUID); targetSess != nil && targetSess.player != nil {
		targetSess.castMu.Lock()
		for _, aura := range targetSess.activeAuras {
			if aura == nil || aura.CasterGUID != casterGUID {
				continue
			}
			candidates = append(candidates, aura.SpellID)
		}
		targetSess.castMu.Unlock()
	} else if motion := s.server.findCreatureMotion(s.player.Map, s.player.InstanceID, targetGUID); motion != nil {
		key := creatureAuraKeyForMotion(motion)
		s.server.auraMu.Lock()
		for _, aura := range s.server.activeCreatureAuras[key] {
			if aura == nil || aura.CasterGUID != casterGUID {
				continue
			}
			candidates = append(candidates, aura.SpellID)
		}
		s.server.auraMu.Unlock()
	} else {
		return false
	}
	for _, spellID := range candidates {
		spell, found, err := s.server.Data.Spell(spellID)
		if err != nil || !found {
			continue
		}
		if spell.SpellFamilyName == spellFamilyHunter && len(spell.SpellFamilyFlags) >= 2 &&
			(spell.SpellFamilyFlags[0]&0xC000 != 0 || spell.SpellFamilyFlags[1]&0x1080 != 0) {
			return true
		}
	}
	return false
}

// roarOfSacrificeProcAllowed mirrors
// spell_hun_roar_of_sacrifice::CheckProc (spell_hunter.cpp:1279-1292):
// the proc needs a damage event (DamageInfo non-null in C++; the
// ev.damage == 0 proxy stands in, as in checkEffectProcGates), a
// damage school intersecting the EFFECT_1 MiscValue mask, and a live
// aura caster (GetCaster() non-null).
func (s *session) roarOfSacrificeProcAllowed(aura *activeAura, auraSpell wotlk.Spell, ev procEventInfo) bool {
	if aura == nil || aura.CasterGUID == 0 {
		return false
	}
	if ev.damage == 0 {
		return false
	}
	if len(auraSpell.Effects) < 2 {
		return false
	}
	return ev.schoolMask&uint32(auraSpell.Effects[1].MiscValue) != 0
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
	// Player::CastItemCombatSpell (Player.cpp:8055): the item-template
	// CHANCE_ON_HIT arm runs on every damaging melee event, independent of
	// whether the swinging weapon carries a combat enchant.
	s.procItemCombatSpells(ctx, target, attType)
	if encID == 0 {
		return
	}
	s.fireWeaponEnchantProc(ctx, target, encID, attTime)
}

// itemSpellTriggerChanceOnHit mirrors ITEM_SPELLTRIGGER_CHANCE_ON_HIT
// (ItemTemplate.h:82): template spells evaluated by
// Player::CastItemCombatSpell (Player.cpp:8105) on damaging melee events.
const itemSpellTriggerChanceOnHit = 2

// weaponProcChancePercent mirrors Unit::GetWeaponProcChance (Unit.cpp:8285-8294):
// the normalized proc chance for weapon attack speed. In the swing path the
// just-fired attack's timer is still ready, so attType selects the arm
// directly: mainhand swings use attackTime*1.8/1000, offhand swings use
// attackTime*1.6/1000 (the C++ isAttackReady/haveOffhandWeapon gates collapse
// to this at the call site inside AttackerStateUpdate).
func weaponProcChancePercent(attType protocol.WeaponAttackType, mainTime, offTime uint32) float64 {
	if attType == protocol.OffAttack {
		return float64(offTime) * 1.6 / 1000.0
	}
	return float64(mainTime) * 1.8 / 1000.0
}

// itemCombatSpellEntry is one item_template spell row relevant to
// Player::CastItemCombatSpell.
type itemCombatSpellEntry struct {
	spellID uint32
	trigger uint32
	ppm     float64
}

// itemCombatSpellInfo caches an item template's class plus its spell rows
// for the CastItemCombatSpell evaluation.
type itemCombatSpellInfo struct {
	class  uint32
	spells [5]itemCombatSpellEntry
}

// getItemCombatSpellInfo returns the cached class + spell rows for an item
// template, loading them from the world DB's item_template on first use.
// Mirrors the ItemTemplate::Spells data Player::CastItemCombatSpell
// (Player.cpp:8055-8148) reads.
func (s *Server) getItemCombatSpellInfo(ctx context.Context, entry uint32) *itemCombatSpellInfo {
	if s == nil || entry == 0 {
		return nil
	}
	s.itemCombatSpellMu.RLock()
	if s.itemCombatSpells != nil {
		if info, ok := s.itemCombatSpells[entry]; ok {
			s.itemCombatSpellMu.RUnlock()
			return info
		}
	}
	s.itemCombatSpellMu.RUnlock()
	if s.WorldStore == nil || s.WorldStore.DB == nil || ctx == nil {
		return nil
	}
	var class uint32
	var ids, triggers [5]uint32
	var ppms [5]float64
	err := s.WorldStore.DB.QueryRowContext(ctx, `SELECT class,
		spellid_1, spelltrigger_1, spellppmRate_1,
		spellid_2, spelltrigger_2, spellppmRate_2,
		spellid_3, spelltrigger_3, spellppmRate_3,
		spellid_4, spelltrigger_4, spellppmRate_4,
		spellid_5, spelltrigger_5, spellppmRate_5
		FROM item_template WHERE entry = ? LIMIT 1`, entry).Scan(&class,
		&ids[0], &triggers[0], &ppms[0],
		&ids[1], &triggers[1], &ppms[1],
		&ids[2], &triggers[2], &ppms[2],
		&ids[3], &triggers[3], &ppms[3],
		&ids[4], &triggers[4], &ppms[4])
	if err != nil {
		return nil
	}
	info := &itemCombatSpellInfo{class: class}
	for i := 0; i < 5; i++ {
		info.spells[i] = itemCombatSpellEntry{spellID: ids[i], trigger: triggers[i], ppm: ppms[i]}
	}
	s.itemCombatSpellMu.Lock()
	if s.itemCombatSpells == nil {
		s.itemCombatSpells = make(map[uint32]*itemCombatSpellInfo)
	}
	s.itemCombatSpells[entry] = info
	s.itemCombatSpellMu.Unlock()
	return info
}

// procItemCombatSpells mirrors the item-template arm of
// Player::CastItemCombatSpell (Player.cpp:8055-8148): every equipped item's
// ITEM_SPELLTRIGGER_CHANCE_ON_HIT template spells are evaluated on the
// damaging melee event. Broken items are skipped; weapon-class items only
// proc from the slot matching the swing (mainhand/offhand/ranged). The
// extra-attacks gate (m_extraAttacks + SPELL_EFFECT_ADD_EXTRA_ATTACKS,
// Player.cpp:8135) aborts the whole evaluation. Chance resolution:
// SpellPPMRate -> PPM chance on the swing's weapon speed, else the DBC
// ProcChance, else (chance > 100, i.e. unset) the GetWeaponProcChance
// normalized fallback. Documented deltas: the CanUseAttackType disarm
// gate has no Go disarm-flag model; the BASE_ATTACK
// IsUseEquipedWeapon/feral-form arm is skipped (Go's shapeshift weapon
// usability is not modeled per swing).
func (s *session) procItemCombatSpells(ctx context.Context, target combatTarget, attType protocol.WeaponAttackType) {
	if s == nil || s.player == nil || target.GUID == 0 || target.GUID == s.playerGUID || ctx == nil {
		return
	}
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return
	}
	type slotEntry struct {
		slot  uint8
		entry uint32
		info  *itemCombatSpellInfo
	}
	var candidates []slotEntry
	for slot := uint8(0); slot < uint8(equipmentSlotEnd); slot++ {
		entry := s.getEquipmentItem(slot)
		if entry == 0 {
			continue
		}
		info := s.server.getItemCombatSpellInfo(ctx, entry)
		if info == nil {
			continue
		}
		hasOnHit := false
		for i := 0; i < 5; i++ {
			if info.spells[i].spellID != 0 && info.spells[i].trigger == itemSpellTriggerChanceOnHit {
				hasOnHit = true
				break
			}
		}
		if !hasOnHit {
			continue
		}
		candidates = append(candidates, slotEntry{slot: slot, entry: entry, info: info})
	}
	if len(candidates) == 0 {
		return
	}
	// Broken-item skip (Player.cpp:8063: !item->IsBroken()): one durability
	// lookup for the equipped candidates.
	broken := make(map[uint8]bool)
	guidBySlot := make(map[uint8]uint64)
	if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		rows, err := s.server.CharactersStore.DB.QueryContext(ctx, `SELECT ci.slot, ii.durability, ci.item
			FROM character_inventory AS ci
			JOIN item_instance AS ii ON ii.guid = ci.item
			WHERE ci.guid = ? AND ci.bag = 0 AND ci.slot < ?`, s.playerGUID, equipmentSlotEnd)
		if err == nil {
			durBySlot := make(map[uint8]uint32)
			for rows.Next() {
				var slot, itemGUID int64
				var dur uint32
				if rows.Scan(&slot, &dur, &itemGUID) == nil && slot >= 0 && slot < int64(equipmentSlotEnd) {
					durBySlot[uint8(slot)] = dur
					if itemGUID != 0 {
						guidBySlot[uint8(slot)] = uint64(itemGUID)
					}
				}
			}
			rows.Close()
			for _, c := range candidates {
				if tmpl, ok := s.server.getItemStoreTemplateInfo(ctx, c.entry); ok && tmpl.MaxDurability > 0 {
					if dur, found := durBySlot[c.slot]; found && dur == 0 {
						broken[c.slot] = true
					}
				}
			}
		}
	}
	// Weapon-class slot match (Player.cpp:8068-8089): the swinging slot's
	// weapon alone procs — mainhand for BASE, offhand for OFF, ranged for RANGED.
	wantSlot := uint8(255)
	if attType == protocol.BaseAttack {
		wantSlot = 15 // EQUIPMENT_SLOT_MAINHAND
	} else if attType == protocol.OffAttack {
		wantSlot = 16 // EQUIPMENT_SLOT_OFFHAND
	} else if attType == protocol.RangedAttack {
		wantSlot = 17 // EQUIPMENT_SLOT_RANGED
	}
	mainTime := s.player.AttackTime
	if mainTime == 0 {
		mainTime = 2000
	}
	offTime := s.player.OffhandAttackTime
	if offTime == 0 {
		offTime = 2000
	}
	for _, c := range candidates {
		if broken[c.slot] {
			continue
		}
		if c.info.class == itemClassWeapon && c.slot != wantSlot {
			continue
		}
		for i := 0; i < 5; i++ {
			sp := c.info.spells[i]
			if sp.spellID == 0 || sp.trigger != itemSpellTriggerChanceOnHit {
				continue
			}
			spell, ok, err := s.server.Data.Spell(sp.spellID)
			if err != nil || !ok {
				continue
			}
			// Player.cpp:8135-8136: an extra-attack swing never procs an
			// extra-attack spell — aborts the whole evaluation.
			if s.extraAttacks > 0 && spellHasEffect(spell, spellEffectAddExtraAttacks) {
				return
			}
			chance := float64(spell.ProcChance)
			if sp.ppm > 0 {
				// Player.cpp:8140-8143: PPM chance on the swinging weapon's speed.
				speed := mainTime
				if attType == protocol.OffAttack {
					speed = offTime
				}
				if !RollPPMChance(sp.ppm, speed) {
					continue
				}
			} else {
				// Player.cpp:8144-8145: unset ProcChance (>100 in C++'s
				// float compare) falls back to the normalized weapon proc
				// chance.
				if chance > 100 {
					chance = weaponProcChancePercent(attType, mainTime, offTime)
				}
				if chance <= 0 || rand.Float64()*100 >= chance {
					continue
				}
			}
			// CastSpell(victim, spellId, item) — triggered (Player.cpp:8146):
			// the weapon rides as the cast item (CastSpellExtraArgs(Item*),
			// SpellDefines.h:165 -> Object.cpp:2900), so prepare's CheckCast
			// runs the CheckItems charge gate (Spell.cpp:6699-6703), bridged
			// in castSpellDirectWithOverrides. A missing row keeps the
			// item-less direct cast.
			s.castSpellDirectWithItem(ctx, sp.spellID, target.GUID, guidBySlot[c.slot])
		}
	}
}

// fireWeaponEnchantProc evaluates the PPM/chance roll for one weapon enchant ID
// and casts the proc spell. Shared by the white-hit path and the spell-driven
// path (which pre-applies the ENCHANT_PROC_ATTR_WHITE_HIT gate).
func (s *session) fireWeaponEnchantProc(ctx context.Context, target combatTarget, encID uint32, attTime uint32) {
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

// procWeaponEnchantProcsFromSpellHit mirrors the spell-driven arm of
// Player::CastItemCombatSpell (Spell.cpp:2588-2596): melee/ranged
// damage-class spell hits evaluate weapon combat-spell enchants too, but an
// enchant flagged ENCHANT_PROC_ATTR_WHITE_HIT in spell_enchant_proc_data
// never fires from a spell hit (Player.cpp:8180). Go's spell pipeline carries
// no per-spell attack type (C++ Spell::m_attackType), so the mainhand slot
// drives the lookup, matching the common BASE_ATTACK case; offhand-driven
// instant strikes (Shiv) keep their white-hit-path procs only.
func (s *session) procWeaponEnchantProcsFromSpellHit(ctx context.Context, target combatTarget, targetAlive bool) {
	if s == nil || s.player == nil || !targetAlive || target.GUID == 0 || target.GUID == s.playerGUID {
		return
	}
	attTime := s.player.AttackTime
	if attTime == 0 {
		attTime = 2000
	}
	// The item-template CHANCE_ON_HIT arm of Player::CastItemCombatSpell
	// (Player.cpp:8055) also evaluates on melee/ranged damage-class spell
	// hits; the mainhand slot drives the weapon match, like the enchant
	// arm below. It runs independent of the enchant lookup.
	s.procItemCombatSpells(ctx, target, protocol.BaseAttack)
	encID := s.getEquipmentEnchant(uint8(15)) // equipSlotMainHand
	if encID == 0 || s.enchantProcWhiteHitOnly(encID) {
		return
	}
	s.fireWeaponEnchantProc(ctx, target, encID, attTime)
}

// deathbringersWillForms mirrors spell_item_deathbringers_will::
// HandleProc (spell_item.cpp:584-620): the class table picks one of
// three buffs uniformly — warrior/paladin/death knight take
// Strength/Critical/Haste, hunter Agility/Critical/AP, rogue/shaman
// Agility/Haste/AP, druid Strength/Agility/Haste; priest, mage and
// warlock take none (C++ returns early on the empty list). Class
// numbers are SharedDefines.h (1 warrior, 2 paladin, 3 hunter,
// 4 rogue, 5 priest, 6 death knight, 7 shaman, 8 mage, 9 warlock,
// 11 druid).
func deathbringersWillForms(class uint8, heroic bool) []uint32 {
	var str, agi, ap, crit, haste uint32 = ProcSpellDBWStrengthNorm, ProcSpellDBWAgilityNorm, ProcSpellDBWAPNorm, ProcSpellDBWCritNorm, ProcSpellDBWHasteNorm
	if heroic {
		str, agi, ap, crit, haste = ProcSpellDBWStrengthHero, ProcSpellDBWAgilityHero, ProcSpellDBWAPHero, ProcSpellDBWCritHero, ProcSpellDBWHasteHero
	}
	switch class {
	case 1, 2, 6:
		return []uint32{str, crit, haste}
	case 3:
		return []uint32{agi, crit, ap}
	case 4, 7:
		return []uint32{agi, haste, ap}
	case 11:
		return []uint32{str, agi, haste}
	default:
		return nil
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
			// 35% chance on attack, 45s ICD. The buff is one of the
			// three class-table forms (deathbringersWillForms); a
			// class with no forms never procs.
			if forms := deathbringersWillForms(s.player.Class, false); len(forms) > 0 && !s.isProcOnCooldown(ProcSpellDBWAgilityNorm) && rand.Float64() < s.procChanceDefault(ProcSpellDBWAgilityNorm, 0.35) {
				chosen := forms[rand.Intn(len(forms))]
				s.castSpellDirect(ctx, chosen, s.playerGUID)
				s.triggerProcCooldown(ProcSpellDBWAgilityNorm, 45*time.Second)
			}

		case ItemDeathbringersWillHero:
			// 35% chance on attack, 45s ICD. The buff is one of the
			// three class-table forms (deathbringersWillForms); a
			// class with no forms never procs.
			if forms := deathbringersWillForms(s.player.Class, true); len(forms) > 0 && !s.isProcOnCooldown(ProcSpellDBWAgilityHero) && rand.Float64() < s.procChanceDefault(ProcSpellDBWAgilityHero, 0.35) {
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
	spellAttr3CanProcWithTriggered     uint32 = 0x04000000 // SharedDefines.h:549 — aura may proc from triggered spells
	spellAttr3CantTriggerProc          uint32 = 0x00010000
	spellAttr3DisableProc              uint32 = 0x00080000 // SharedDefines.h:542 — no procs while this aura's proc trigger runs
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

// Proc-trigger aura types (SpellAuraDefines.h:84/122/123/311/313).
const (
	spellAuraProcTriggerSpell            = 42
	spellAuraProcTriggerDamage           = 43
	spellAuraProcTriggerSpellWithValue   = 231
	spellAuraRaidProcFromCharge          = 223
	spellAuraRaidProcFromChargeWithValue = 225
)

// isAlwaysTriggeredAura mirrors the LoadSpellProc always-triggered set
// (SpellMgr.cpp:1736-1743): an aura spell carrying any of these effects
// gets PROC_ATTR_TRIGGERED_CAN_PROC on its generated entry regardless of
// its proc flags (SpellMgr.cpp:1777-1782, 1866-1867).
const (
	spellAuraModStealth           = 16  // SPELL_AURA_MOD_STEALTH (SpellAuraDefines.h:96)
	spellAuraModInvisibility      = 18  // SPELL_AURA_MOD_INVISIBILITY (SpellAuraDefines.h:98)
	spellAuraOverrideClassScripts = 112 // SPELL_AURA_OVERRIDE_CLASS_SCRIPTS (SpellAuraDefines.h:192)
)

// spellAuraAddTargetTrigger mirrors SPELL_AURA_ADD_TARGET_TRIGGER
// (SpellAuraDefines.h:189).
const spellAuraAddTargetTrigger = 109

// spellAuraAbilityConsumeNoAmmo mirrors SPELL_AURA_ABILITY_CONSUME_NO_AMMO
// (SpellAuraDefines.h:354): HandleLaunchPhase (Spell.cpp:7705) skips
// TakeAmmo for spells it affects.
const spellAuraAbilityConsumeNoAmmo = 274

// spellVampiricAura is the dreadlord bot aura (botcommon.h:652) whose proc
// trigger spell is triggerReplenishLifeSpell: HandleProcTriggerSpellAuraProc
// carries a fork special case (SpellAuraEffects.cpp:5670-5705) that fires
// the heal with the proc damage as base points instead of the normal
// triggered-by-aura cast.
const spellVampiricAura = 20810

// isProcTriggerAuraType mirrors the LoadSpellProc trigger-aura subset whose
// C++ HandleProc arm fires a spell, damage, or a charge-jump
// (SpellMgr.cpp:1686-1729, SpellAuraEffects.cpp:1010-1043): dummy,
// proc-trigger-spell, proc-trigger-damage, proc-trigger-spell-with-value,
// and the raid-proc-from-charge pair, plus ADD_FLAT_MODIFIER
// (SpellMgr.cpp:1716 lists it in isTriggerAura): its only C++ arm is the
// script hook — the Cobra Strikes stack drop (spell_hunter.cpp:361-364) —
// and the default leg no-ops on the TriggerSpell == 0 gate below, so the
// type enters the loop for its hook alone. OVERRIDE_CLASS_SCRIPTS
// (SpellMgr.cpp:1708) enters the same way, for the Improved Mana Gems
// arm (spell_mage.cpp:515-526) — its default leg likewise no-ops on
// TriggerSpell == 0. MOD_SPELL_CRIT_CHANCE (SpellMgr.cpp:1718) enters
// the same way, for the Focus Magic arm (spell_mage.cpp:743-747); its
// CheckEffectProc gate (SpellAuraEffects.cpp:1023-1026, skip spells that
// can't crit) already lives in checkEffectProcGates. The remaining
// isTriggerAura types (reflect, stealth-break, ADD_PCT_MODIFIER) have no
// Go HandleProc arm yet.
func isProcTriggerAuraType(auraType uint32) bool {
	switch auraType {
	case spellAuraDummy, spellAuraProcTriggerSpell, spellAuraProcTriggerDamage, spellAuraProcTriggerSpellWithValue,
		spellAuraRaidProcFromCharge, spellAuraRaidProcFromChargeWithValue, spellAuraAddFlatModifier, spellAuraAddPctModifier, spellAuraOverrideClassScripts,
		spellAuraModSpellCritChance:
		return true
	}
	return false
}

// scriptProcCast describes one AuraScript proc arm the generic trigger-spell
// loop cannot express: a SPELL_AURA_DUMMY proc effect with no DBC trigger
// spell, whose C++ OnEffectProc handler casts a fixed spell with a
// damage-derived base point. The generic loop reaches such effects (dummy is
// a proc-trigger aura type) but skips them at the TriggerSpell == 0 gate,
// so without this dispatch the script arm never fires even when the aura
// procs, rolls, and spends its charge.
type scriptProcCast struct {
	spellID    uint32
	casterGUID uint64
	targetGUID uint64
	bp0        uint32
	bp0Set     bool
	bp1        uint32
	bp1Set     bool
}

// procTargetVitals resolves a proc target's (GetProcTarget, Unit.h:474)
// vitals for the judgement proc arms below: max health, base mana
// (Unit::GetCreateMana), and power type. ProcEventInfo builds the
// proc target as the other unit on both sides (Unit.cpp:10388, 10412),
// which is exactly the triggerTargetGUID both proc loops pass, so the
// caller passes that GUID. Players read the live state (BaseMana is
// the player_classlevelstats base); creatures have no CreateMana in
// Go, so MaxMana stands in for it (judgement procs on mana creatures
// are the only consumers). A zero GUID or an unresolvable unit reads
// not-ok, and the caller skips the leg like a failed C++ gate.
func (s *Server) procTargetVitals(mapID, instanceID uint32, guid uint64) (maxHealth, baseMana uint32, powerType uint8, ok bool) {
	if s == nil || guid == 0 {
		return 0, 0, 0, false
	}
	if sess := s.findSessionByGUID(guid); sess != nil && sess.player != nil {
		return sess.player.MaxHealth, energizeCreateMana(sess), playerPowerType(sess.player), true
	}
	if m := s.findCreatureMotion(mapID, instanceID, guid); m != nil {
		return m.MaxHealth, m.MaxMana, uint8(m.PowerType), true
	}
	return 0, 0, 0, false
}

// procTargetFullHealth reports whether a proc target sits at full health
// (Unit::GetHealth() >= GetMaxHealth(), Unit.h:872): the overheal gate of
// spell_pri_t5_heal_2p_bonus::CheckProc (spell_priest.cpp:1504-1513), read
// post-heal on the done side (the heal is applied before the proc loop
// runs, spells.go). Players read the live state; creatures the motion
// health. A zero GUID, an unresolvable unit, or a zero max health reads
// not-full, and the caller skips the leg like a failed C++ gate.
func (s *Server) procTargetFullHealth(mapID, instanceID uint32, guid uint64) bool {
	if s == nil || guid == 0 {
		return false
	}
	if sess := s.findSessionByGUID(guid); sess != nil && sess.player != nil {
		return sess.player.MaxHealth > 0 && sess.player.Health >= sess.player.MaxHealth
	}
	if m := s.findCreatureMotion(mapID, instanceID, guid); m != nil {
		return m.MaxHealth > 0 && m.Health >= m.MaxHealth
	}
	return false
}

// procTargetClass resolves a proc target's class (Unit::GetClass,
// Unit.h:850: the UNIT_FIELD_BYTES_0 class byte): the player's class
// for player targets, the template unit_class (Creature.cpp:494) for
// creatures. A zero GUID or an unresolvable unit reads not-ok, and the
// caller skips the leg like the C++ default case.
func (s *Server) procTargetClass(mapID, instanceID uint32, guid uint64) (uint8, bool) {
	if s == nil || guid == 0 {
		return 0, false
	}
	if sess := s.findSessionByGUID(guid); sess != nil && sess.player != nil {
		return sess.player.Class, true
	}
	if m := s.findCreatureMotion(mapID, instanceID, guid); m != nil {
		return uint8(m.UnitClass), true
	}
	return 0, false
}

// procActionTargetGUID mirrors ProcEventInfo::GetActionTarget
// (Unit.h:473): the actor-side event's action target is the other unit
// (Unit.cpp:10388: actor=this, actionTarget=other, procTarget=other),
// the target-side event's action target is the aura holder itself
// (Unit.cpp:10412: actor=attacker, actionTarget=holder, procTarget=
// attacker). The side is the event actor: actorGUID == holderGUID is the
// actor-side event; anything else is the target-side event. The loops
// pass triggerTargetGUID as the other unit on both sides, so it is the
// action target exactly on the actor side.
func procActionTargetGUID(actorGUID, holderGUID, triggerTargetGUID uint64) uint64 {
	if actorGUID != 0 && actorGUID == holderGUID {
		return triggerTargetGUID
	}
	return holderGUID
}

// scriptAuraProcCast resolves the script arm for a dummy proc effect.
// C++ owners: spell_deathwhisper_vampiric_might::HandleProc
// (boss_lady_deathwhisper.cpp:1019-1032),
// spell_gen_vampiric_touch::HandleProc (spell_generic.cpp:3479-3495),
// spell_uk_second_wind::HandleProc (utgarde_keep.cpp:198-240), and
// spell_dk_mark_of_blood::HandleProc (spell_dk.cpp:1716-1720).
// A zero-damage event fires nothing for the first two (the C++ early
// return); the effect index gates to the registered effect (70674
// EFFECT_1, 52723/60501 EFFECT_0, 42770 EFFECT_0) so sibling dummy
// effects of the same aura never double-fire. Second Wind carries no
// damage gate in C++ (its CheckProc reads only the triggering spell's
// mechanic mask), so its case bypasses the damage gate; its trigger
// spell rides DBC base points (the C++ aurEff overload marks the cast
// aura-triggered only), so bp0Set stays false and no base-point
// override is applied. Second Wind and Vampiric Touch cast from the
// proc ACTOR (eventInfo.GetActor / the damage dealer); Second Wind's
// target is the ACTION TARGET, which the caller passes as
// triggerTargetGUID — on a taken event that is the holder, on a done
// event (a root/stun the holder cast, e.g. Charge) it is the enemy, and
// the C++ cast lands on the enemy (utgarde_keep.cpp:225-227).
// bp1/bp1Set are the EFFECT_1 twin of bp0 (spell_pal_glyph_of_divinity,
// 54939: BP1 = twice the triggering spell's EFFECT_1 CalcValue); the
// fire arm folds either override into the cast's base-point map.
func scriptAuraProcCast(auraSpellID uint32, effIndex int, effAura uint32, damage uint32, effAmount uint32, actorGUID, holderGUID, triggerTargetGUID uint64, triggerSpell *wotlk.Spell) (scriptProcCast, bool) {
	if effAura != spellAuraDummy {
		return scriptProcCast{}, false
	}
	switch auraSpellID {
	case 70674: // Vampiric Might, EFFECT_1: holder casts 70677 on self, BP0 = pct of damage.
		if damage == 0 || effIndex != 1 {
			return scriptProcCast{}, false
		}
		return scriptProcCast{spellID: 70677, casterGUID: holderGUID, targetGUID: holderGUID, bp0: uint32(uint64(damage) * uint64(effAmount) / 100), bp0Set: true}, true
	case 52723, 60501: // Vampiric Touch, EFFECT_0: proc actor casts 52724 on self, BP0 = damage / 2.
		if damage == 0 || effIndex != 0 {
			return scriptProcCast{}, false
		}
		return scriptProcCast{spellID: 52724, casterGUID: actorGUID, targetGUID: actorGUID, bp0: damage / 2, bp0Set: true}, true
	case 42770: // Second Wind, EFFECT_0: action target casts 42771 on self when the triggering spell roots or stuns.
		if effIndex != 0 || triggerSpell == nil {
			return scriptProcCast{}, false
		}
		var mechanicMask uint32
		for _, eff := range triggerSpell.Effects {
			if eff.Mechanic < 32 {
				mechanicMask |= uint32(1) << eff.Mechanic
			}
		}
		if mechanicMask&((uint32(1)<<7)|(uint32(1)<<12)) == 0 { // MECHANIC_ROOT, MECHANIC_STUN (SharedDefines.h:1364,1369)
			return scriptProcCast{}, false
		}
		actionTarget := procActionTargetGUID(actorGUID, holderGUID, triggerTargetGUID)
		if actionTarget == 0 {
			return scriptProcCast{}, false
		}
		return scriptProcCast{spellID: 42771, casterGUID: actionTarget, targetGUID: actionTarget}, true
	case 49005: // Mark of Blood, EFFECT_0: the marked enemy (proc actor) casts 61607 on the damage victim; no damage gate in C++.
		if effIndex != 0 || actorGUID == 0 || triggerTargetGUID == 0 {
			return scriptProcCast{}, false
		}
		return scriptProcCast{spellID: 61607, casterGUID: actorGUID, targetGUID: triggerTargetGUID}, true
	case 58642: // Glyph of Scourge Strike, EFFECT_0: the proc actor casts 69961 on the proc target; PreventDefaultAction.
		if effIndex != 0 || actorGUID == 0 || triggerTargetGUID == 0 {
			return scriptProcCast{}, false
		}
		return scriptProcCast{spellID: 69961, casterGUID: actorGUID, targetGUID: triggerTargetGUID}, true
	case 56800: // Glyph of Backstab, EFFECT_0: the proc actor casts 63975 on the proc target with the aura effect (triggered); PreventDefaultAction.
		if effIndex != 0 || actorGUID == 0 || triggerTargetGUID == 0 {
			return scriptProcCast{}, false
		}
		return scriptProcCast{spellID: 63975, casterGUID: actorGUID, targetGUID: triggerTargetGUID}, true
	case 61257: // Runic Power Back on Snare/Root (PvP 4P), EFFECT_0: the action target casts 61258 on itself when the triggering spell roots or snares.
		if effIndex != 0 || triggerSpell == nil {
			return scriptProcCast{}, false
		}
		var mechanicMask uint32
		for _, eff := range triggerSpell.Effects {
			if eff.Mechanic < 32 {
				mechanicMask |= uint32(1) << eff.Mechanic
			}
		}
		if mechanicMask&((uint32(1)<<7)|(uint32(1)<<11)) == 0 { // MECHANIC_ROOT, MECHANIC_SNARE (SharedDefines.h:1364,1368)
			return scriptProcCast{}, false
		}
		actionTarget := procActionTargetGUID(actorGUID, holderGUID, triggerTargetGUID)
		if actionTarget == 0 {
			return scriptProcCast{}, false
		}
		return scriptProcCast{spellID: 61258, casterGUID: actionTarget, targetGUID: actionTarget}, true
	case 40470: // Paladin Tier 6 Trinket, EFFECT_0: Holy Light / Flash of Light (family flags[0] & 0xC0000000) rolls 15% for Enduring Light (40471); Judgements (flags[0] & 0x00800000) roll 50% for Enduring Judgement (40472); anything else returns. The proc actor casts the winner on the proc target with the aura effect (roll_chance_i, spell_paladin.cpp:1466-1493). DORMANT until Spell.dbc ships the 40470 proc entry.
		if effIndex != 0 || triggerSpell == nil || actorGUID == 0 || triggerTargetGUID == 0 {
			return scriptProcCast{}, false
		}
		if len(triggerSpell.SpellFamilyFlags) < 1 {
			return scriptProcCast{}, false
		}
		var spellID uint32
		chance := 0
		if triggerSpell.SpellFamilyFlags[0]&0xC0000000 != 0 {
			spellID = 40471
			chance = 15
		} else if triggerSpell.SpellFamilyFlags[0]&0x00800000 != 0 {
			spellID = 40472
			chance = 50
		} else {
			return scriptProcCast{}, false
		}
		if rand.Intn(100) >= chance {
			return scriptProcCast{}, false
		}
		return scriptProcCast{spellID: spellID, casterGUID: actorGUID, targetGUID: triggerTargetGUID}, true
	case 40463: // Shaman Tier 6 Trinket, EFFECT_0: Lesser Healing Wave (family flags[0] & 0x80) rolls 10% for Energy Surge (40465); Lightning Bolt (flags[0] & 0x01) rolls 15% for Energy Surge; Stormstrike (flags[1] & 0x10) rolls 50% for Power Surge (40466); anything else returns. The proc actor casts the winner on itself with the aura effect (roll_chance_i, spell_shaman.cpp:1380-1414). DORMANT until Spell.dbc ships the 40463 proc entry.
		if effIndex != 0 || triggerSpell == nil || actorGUID == 0 {
			return scriptProcCast{}, false
		}
		if len(triggerSpell.SpellFamilyFlags) < 1 {
			return scriptProcCast{}, false
		}
		var spellID uint32
		chance := 0
		if triggerSpell.SpellFamilyFlags[0]&0x00000080 != 0 {
			spellID = 40465
			chance = 10
		} else if triggerSpell.SpellFamilyFlags[0]&0x00000001 != 0 {
			spellID = 40465
			chance = 15
		} else if len(triggerSpell.SpellFamilyFlags) >= 2 && triggerSpell.SpellFamilyFlags[1]&0x00000010 != 0 {
			spellID = 40466
			chance = 50
		} else {
			return scriptProcCast{}, false
		}
		if rand.Intn(100) >= chance {
			return scriptProcCast{}, false
		}
		return scriptProcCast{spellID: spellID, casterGUID: actorGUID, targetGUID: actorGUID}, true
	}
	return scriptProcCast{}, false
}

// scriptProcEffAmount reads an aura effect's amount the way the
// proc-trigger-with-value arm does: the per-effect amount when tracked,
// else the aura amount.
func scriptProcEffAmount(aura *activeAura, effIndex int) uint32 {
	if aura == nil {
		return 0
	}
	if effIndex >= 0 && effIndex < len(aura.Amounts) && aura.Amounts[effIndex] > 0 {
		return uint32(aura.Amounts[effIndex])
	}
	return aura.Amount
}

// fireScriptProcCast casts a script-proc arm's spell from its C++ caster: a
// player caster casts through its session, a creature caster through the
// creature cast path on the holder's map/instance. An unresolvable caster
// (zero GUID, offline player, unloaded creature) fizzles silently.
func (s *Server) fireScriptProcCast(ctx context.Context, holderMap uint32, holderInstance uint32, cast scriptProcCast) {
	if s == nil || cast.spellID == 0 || cast.casterGUID == 0 {
		return
	}
	if sess := s.findSessionByGUID(cast.casterGUID); sess != nil {
		if cast.bp0Set || cast.bp1Set {
			overrides := make(map[int]int32)
			if cast.bp0Set {
				overrides[0] = int32(cast.bp0)
			}
			if cast.bp1Set {
				overrides[1] = int32(cast.bp1)
			}
			sess.castSpellDirectWithOverrides(ctx, cast.spellID, cast.targetGUID, false, overrides, 0, nil, 0)
		} else {
			sess.castSpellDirectWithOverrides(ctx, cast.spellID, cast.targetGUID, false, nil, 0, nil, 0)
		}
		return
	}
	if motion := s.findCreatureMotion(holderMap, holderInstance, cast.casterGUID); motion != nil {
		var overrides map[int]int32
		if cast.bp0Set || cast.bp1Set {
			overrides = make(map[int]int32)
			if cast.bp0Set {
				overrides[0] = int32(cast.bp0)
			}
			if cast.bp1Set {
				overrides[1] = int32(cast.bp1)
			}
		}
		s.castCreatureSpellInternalWithBasePoints(ctx, motion, cast.spellID, cast.targetGUID, true, overrides, nil, false, 0)
	}
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
	// CooldownMs is SpellProcEntry::Cooldown (SpellMgr.h:262), loaded from
	// spell_proc field 14 (SpellMgr.cpp:1578); the generated-default branch
	// zeroes it (SpellMgr.cpp:1871), and this server builds generated
	// entries only, so it is 0 until a spell_proc table import exists.
	CooldownMs uint32
}

// serverSpellProcEntryFor builds the generated spell_proc entry for an aura
// spell. Returns the entry and the DBC spell; ok is false when the spell
// has no DBC ProcFlags or no proc-trigger aura effect
// (SpellMgr::LoadSpellProc, SpellMgr.cpp:1755-1869). The session twin
// delegates here: the logic needs only the DBC spell store, so the
// creature proc path (boss_ai.go) reuses it exactly.
func (s *Server) serverSpellProcEntryFor(auraSpellID uint32) (entry spellProcEntry, auraSpell wotlk.Spell, ok bool) {
	if s == nil || s.Data == nil {
		return entry, auraSpell, false
	}
	spell, found, err := s.Data.Spell(auraSpellID)
	if err != nil || !found || spell.ProcTypeMask == 0 {
		return entry, auraSpell, false
	}
	hasTrigger := false
	hasTriggerSpellOrDamage := false
	alwaysTriggered := false
	hitMaskDone := false
	var hitMask uint32
	for i := range spell.Effects {
		eff := &spell.Effects[i]
		if eff.Effect == 0 {
			continue
		}
		// SpellMgr.cpp:1777-1782 — the always-triggered arm runs over every
		// effect, not just trigger auras: any always-triggered effect sets
		// the entry's TRIGGERED_CAN_PROC attribute below.
		switch eff.Aura {
		case spellAuraOverrideClassScripts, spellAuraModStealth, spellAuraModConfuse,
			spellAuraModFear, spellAuraModRoot, spellAuraModStun, spellAuraTransform,
			spellAuraModInvisibility:
			alwaysTriggered = true
		}
		// SpellMgr.cpp:1834-1863 — the HitMask scan runs over aura effects
		// in order and stops at the first matching effect (the loop break
		// after the switch), even when a MOD_HIT_CHANCE effect does not
		// qualify (CalcValue > -100 leaves HitMask NONE but still stops
		// the scan). Reflect, crit-percent, block-percent, and the
		// qualifying hit-chance arms all terminate the scan alike.
		if !hitMaskDone && eff.Aura != 0 {
			switch eff.Aura {
			case spellAuraReflectSpells, spellAuraReflectSpellsSchool:
				hitMask = procHitReflect
				hitMaskDone = true
			case spellAuraModWeaponCritPercent:
				hitMask = procHitCritical
				hitMaskDone = true
			case spellAuraModBlockPercent:
				hitMask = procHitBlock
				hitMaskDone = true
			case spellAuraModHitChance:
				if eff.CalcValue() <= -100 {
					hitMask = procHitMiss
				}
				hitMaskDone = true
			}
		}
		triggerAura := isProcTriggerAuraType(eff.Aura)
		// SpellMgr.cpp:1697/1704 — reflect auras (28/74) sit in isTriggerAura
		// even though Go has no HandleProc arm for them (documented at
		// isProcTriggerAuraType); they still qualify the spell for a
		// generated entry.
		if eff.Aura == spellAuraReflectSpells || eff.Aura == spellAuraReflectSpellsSchool {
			triggerAura = true
		}
		if !triggerAura {
			continue
		}
		hasTrigger = true
		if eff.Aura == spellAuraProcTriggerSpell || eff.Aura == spellAuraProcTriggerDamage {
			hasTriggerSpellOrDamage = true
		}
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
	// The HitMask scan above already holds the first matching effect's
	// mask (NONE when a MOD_HIT_CHANCE effect did not qualify); a zero
	// HitMask keeps the CanSpellTriggerProcOnEvent default gate.
	entry.HitMask = hitMask
	// SpellMgr.cpp:1866-1867 — the always-triggered arm sets
	// TRIGGERED_CAN_PROC on the entry regardless of proc flags.
	if alwaysTriggered {
		entry.AttributesMask |= procAttrTriggeredCanProc
	}
	if spell.ProcTypeMask&procFlagKill != 0 {
		entry.AttributesMask |= procAttrReqExpOrHonor
	}
	// LoadSpellProc's taken-flag fallback (SpellMgr.cpp:1783-1797): proc
	// trigger auras on taken-flagged spells proc from triggered hits anyway.
	// C++ applies this only to SPELL_AURA_PROC_TRIGGER_SPELL (42) and
	// SPELL_AURA_PROC_TRIGGER_DAMAGE (43) — never to dummy (4) or
	// proc-trigger-spell-with-value (231).
	if hasTriggerSpellOrDamage && spell.ProcTypeMask&procTakenHitProcFlagMask != 0 {
		entry.AttributesMask |= procAttrTriggeredCanProc
	}
	return entry, spell, true
}

// spellProcEntryFor builds the generated spell_proc entry for an aura spell.
// Returns the entry and the DBC spell; ok is false when the spell has no DBC
// ProcFlags or no proc-trigger aura effect (SpellMgr.cpp:1755-1814).
func (s *session) spellProcEntryFor(auraSpellID uint32) (entry spellProcEntry, auraSpell wotlk.Spell, ok bool) {
	if s == nil || s.server == nil {
		return entry, auraSpell, false
	}
	return s.server.serverSpellProcEntryFor(auraSpellID)
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
	actorGUID       uint64
	damage          uint32
	// heal / effectiveHeal carry HealInfo::GetHeal / GetEffectiveHeal
	// (Spell.cpp:2512) on heal proc events: the pre-absorb, pre-overheal
	// heal and the health actually gained. Zero on every non-heal event
	// and on heal events whose amount was never threaded (periodic /
	// funnel / leech ticks) — the 54937 leg reads the zero as the C++
	// null/zero-heal early return.
	heal          uint32
	effectiveHeal uint32
	// triggeredByAuraSpell is Spell::m_triggeredByAuraSpell (Spell.cpp:3068):
	// the aura spell whose proc trigger cast the event's spell, 0 when the
	// cast was not triggered by an aura. Aura::GetProcEffectMask
	// (SpellAuras.cpp:2057) never lets an aura proc from a spell its own
	// trigger fired (Spell::IsTriggeredByAura, Spell.h:544).
	triggeredByAuraSpell uint32
}

// procActorIsPlayerGUID reports whether a proc actor GUID belongs to a
// player: player GUIDs ride the zero high word (HighGuid::HIGHGUID_PLAYER,
// 0x0000); creatures and pets carry 0xF130/0xF140 (creatureWorldGUID,
// petGUIDFor). A zero GUID is no actor at all.
func procActorIsPlayerGUID(guid uint64) bool {
	return guid != 0 && uint16(guid>>48) == 0
}

// canSpellTriggerProcOnEvent mirrors SpellMgr::CanSpellTriggerProcOnEvent
// (SpellMgr.cpp:502-583): the entry's ProcFlags must intersect the event's
// type mask, then the attribute, school, spell-family, spell-type,
// spell-phase, and hit-mask gates apply in C++ order.
// auraCanProcWithTriggered is the aura spell's
// SPELL_ATTR3_CAN_PROC_WITH_TRIGGERED (SharedDefines.h:549): the
// Aura::GetProcEffectMask arm of the triggered-cast suppression
// (SpellAuras.cpp:2058-2062) — an aura carrying it may proc from triggered
// spells even without PROC_ATTR_TRIGGERED_CAN_PROC on the entry.
func canSpellTriggerProcOnEvent(entry spellProcEntry, ev procEventInfo, auraCanProcWithTriggered bool) bool {
	if ev.typeMask&entry.ProcFlags == 0 {
		return false
	}
	// XP/honor gate (SpellMgr.cpp:508-511): C++ applies it only when the proc
	// event's actor is a player — eventInfo.GetActor()->ToPlayer() is null
	// for creature and pet actors, so the check never engages there. A nil
	// action target passes the C++ null check; player-actor KILL sites fill
	// xpOrHonorTarget from Player::isHonorOrXPTarget.
	if entry.AttributesMask&procAttrReqExpOrHonor != 0 && procActorIsPlayerGUID(ev.actorGUID) && !ev.xpOrHonorTarget {
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
	// (AUTO_ATTACK_PROC_FLAG_MASK, SpellMgr.h:156), the triggered spell
	// itself carries SPELL_ATTR2_TRIGGERED_CAN_TRIGGER_PROC
	// (SharedDefines.h:516) or SPELL_ATTR3_TRIGGERED_CAN_TRIGGER_PROC_2
	// (SharedDefines.h:532), or the aura's own spell carries
	// SPELL_ATTR3_CAN_PROC_WITH_TRIGGERED (SharedDefines.h:549 — the
	// Aura::GetProcEffectMask arm, SpellAuras.cpp:2058-2062).
	if entry.AttributesMask&procAttrTriggeredCanProc == 0 && ev.typeMask&autoAttackProcFlagMask == 0 &&
		ev.eventSpell != nil && ev.triggered && !eventSpellCanTriggerProc(ev.eventSpell) && !auraCanProcWithTriggered {
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
// DmgClass (Spell.dbc field 213). The hunter-trap and Hellfire arms sit after
// the DmgClass switch in C++ (Spell.cpp:2038-2057) and apply on the hit phase
// too: traps OR the activation flag (plus the magic NEG pair, which also
// keeps the DoDamageAndTriggers fallback gate !procAttacker && !procVictim
// closed) on top of the DmgClass fill, and Hellfire ASSIGNS the periodic
// pair, replacing it. Trap is checked first, matching C++ order.
func spellDamageProcTypeMask(spell wotlk.Spell) uint32 {
	var mask uint32
	switch spell.DefenseType {
	case spellDamageClassMelee:
		// Spell::prepareDataForTriggerSystem ORs the mainhand/offhand bit
		// (Spell.cpp:2009-2012); the direct-spell-damage path always uses
		// the base attack type, so the mainhand bit applies.
		mask = procFlagDoneSpellMeleeDmgClass | procFlagDoneMainhandAttack
	case spellDamageClassRanged:
		// Spell.cpp:2018-2023 — auto attack.
		if spell.AttributesEx1&spellAttr2AutoRepeatFlag != 0 {
			mask = procFlagDoneRangedAutoAttack
		} else {
			mask = procFlagDoneSpellRangedDmgClass
		}
	default:
		// Spell.cpp:2026-2034 — wands auto attack.
		if spell.EquippedItemClass == itemClassWeapon &&
			spell.EquippedItemSubClass&(1<<itemSubclassWeaponWand) != 0 &&
			spell.AttributesEx1&spellAttr2AutoRepeatFlag != 0 {
			mask = procFlagDoneRangedAutoAttack
		} else if spell.DefenseType == spellDamageClassMagic {
			mask = procFlagDoneSpellMagicDmgClassNeg
		} else {
			mask = procFlagDoneSpellNoneDmgClassNeg
		}
	}
	if spell.SpellFamilyName == spellFamilyHunter &&
		(spell.SpellFamilyFlags[0]&0x18 != 0 || spell.ID == 57879 ||
			spell.SpellFamilyFlags[2]&0x00024000 != 0) {
		mask |= procFlagDoneTrapActivation | procFlagDoneSpellMagicDmgClassNeg
	}
	if spell.SpellFamilyName == spellFamilyWarlock && spell.SpellFamilyFlags[0]&0x00000040 != 0 {
		mask = procFlagDonePeriodic
	}
	return mask
}

// spellDamageTakenProcTypeMask mirrors the victim-side half of
// Spell::prepareDataForTriggerSystem (Spell.cpp:2006-2035) for the damage
// path: melee damage-class spells pre-fill TAKEN_SPELL_MELEE_DMG_CLASS (no
// mainhand/offhand arm on the victim side — Spell.cpp:2012 fills only
// procAttacker), ranged AUTOREPEAT_FLAG and wand auto attacks pre-fill
// TAKEN_RANGED_AUTO_ATTACK, other ranged spells TAKEN_SPELL_RANGED_DMG_CLASS,
// and magic/none-class spells fall through to the DoDamageAndTriggers
// positivity fallback (Spell.cpp:2447-2492), negative on the damage arm. The
// hunter-trap and Hellfire arms (Spell.cpp:2038-2057) apply after the
// DmgClass fill: traps OR the magic NEG victim flag, Hellfire assigns the
// periodic victim flag — trap checked first, matching C++ order. The
// damage arm's procVictim |= PROC_FLAG_TAKEN_DAMAGE (Spell.cpp:2545) is ORed
// at the call site, gated on non-immunity — the immune branch bypasses it.
func spellDamageTakenProcTypeMask(spell wotlk.Spell) uint32 {
	var mask uint32
	switch spell.DefenseType {
	case spellDamageClassMelee:
		mask = procFlagTakenSpellMeleeDmgClass
	case spellDamageClassRanged:
		// Spell.cpp:2018-2023 — auto attack.
		if spell.AttributesEx1&spellAttr2AutoRepeatFlag != 0 {
			mask = procFlagTakenRangedAutoAttack
		} else {
			mask = procFlagTakenSpellRangedDmgClass
		}
	default:
		// Spell.cpp:2026-2034 — wands auto attack.
		if spell.EquippedItemClass == itemClassWeapon &&
			spell.EquippedItemSubClass&(1<<itemSubclassWeaponWand) != 0 &&
			spell.AttributesEx1&spellAttr2AutoRepeatFlag != 0 {
			mask = procFlagTakenRangedAutoAttack
		} else if spell.DefenseType == spellDamageClassMagic {
			mask = procFlagTakenSpellMagicDmgClassNeg
		} else {
			mask = procFlagTakenSpellNoneDmgClassNeg
		}
	}
	// Spell.cpp:2038-2050 — hunter traps OR the magic NEG victim flag on
	// top of the DmgClass fill; Spell.cpp:2052-2057 — Hellfire ASSIGNS the
	// periodic victim flag, replacing the fill. Trap is checked first,
	// matching C++ order.
	if spell.SpellFamilyName == spellFamilyHunter &&
		(spell.SpellFamilyFlags[0]&0x18 != 0 || spell.ID == 57879 ||
			spell.SpellFamilyFlags[2]&0x00024000 != 0) {
		mask |= procFlagTakenSpellMagicDmgClassNeg
	}
	if spell.SpellFamilyName == spellFamilyWarlock && spell.SpellFamilyFlags[0]&0x00000040 != 0 {
		mask = procFlagTakenPeriodic
	}
	return mask
}

// spellFinishPhaseProcTypeMask mirrors the procAttacker leg of
// Spell::_handle_finish_phase (Spell.cpp:3764-3776): the finish-phase
// ProcSkillsAndAuras call reuses the m_procAttacker fill from
// Spell::prepareDataForTriggerSystem (Spell.cpp:1999-2056) and only falls
// back to the magic/none positivity split when that fill left nothing.
// The melee/ranged/wand auto-attack arms match spellDamageProcTypeMask's
// reading of the fill; the hunter-trap arm (PROC_FLAG_DONE_TRAP_ACTIVATION
// ORed with DONE_SPELL_MAGIC_DMG_CLASS_NEG, Spell.cpp:2040-2049) and the
// Hellfire arm (PROC_FLAG_DONE_PERIODIC by assignment, Spell.cpp:2052-2057)
// ride on top. Positivity is SpellInfo::IsPositive (SpellInfo.cpp:1205:
// !SPELL_ATTR0_CU_NEGATIVE), which the fork defines as the OR of the three
// CU_NEGATIVE_EFF bits (SpellInfo.h:203) — exactly the
// spellDamageNoDmgPositive sweep over IsPositiveEffect.
func spellFinishPhaseProcTypeMask(spell wotlk.Spell) uint32 {
	switch spell.DefenseType {
	case spellDamageClassMelee:
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
	}
	if spell.SpellFamilyName == spellFamilyHunter &&
		(spell.SpellFamilyFlags[0]&0x18 != 0 || spell.ID == 57879 ||
			spell.SpellFamilyFlags[2]&0x00024000 != 0) {
		return procFlagDoneTrapActivation | procFlagDoneSpellMagicDmgClassNeg
	}
	if spell.SpellFamilyName == spellFamilyWarlock && spell.SpellFamilyFlags[0]&0x00000040 != 0 {
		return procFlagDonePeriodic
	}
	positive := spellDamageNoDmgPositive(spell)
	if spell.DefenseType == spellDamageClassMagic {
		if positive {
			return procFlagDoneSpellMagicDmgClassPos
		}
		return procFlagDoneSpellMagicDmgClassNeg
	}
	if positive {
		return procFlagDoneSpellNoneDmgClassPos
	}
	return procFlagDoneSpellNoneDmgClassNeg
}

// procSpellFinishAuraTriggers bridges the on-finish proc leg of
// Spell::_handle_finish_phase (Spell.cpp:3764-3777): once every target is
// processed, the original caster's auras evaluate one proc event with
// PROC_SPELL_PHASE_FINISH, PROC_SPELL_TYPE_MASK_ALL, and the accumulated
// hit mask, ProcSkillsAndAuras(m_originalCaster, nullptr, procAttacker,
// PROC_FLAG_NONE, ...). C++ passes the action target as nullptr, so the
// trigger target is null and triggered spells resolve their own implicit
// targets — Go passes triggerTargetGUID 0, which castSpellDirect handles
// the same way. The event's triggered state mirrors the hit path
// (s.triggeredNoProcEvents), and the caster's own session runs the loop
// (Go has no creature casters; m_originalCaster is the casting player).
// Documented deltas: the hit mask is the cast's accumulated m_hitMask
// (s.castProcHitMask, ORed per target in procSpellHitAuraTriggers /
// procSpellHealAuraTriggers, Spell.cpp:2603), carried RAW — the
// PROC_HIT_NORMAL normalization (Spell.cpp:3541-3543) is a _cast-phase leg
// only; the empty-target arm (Spell.cpp:3605) seeds PROC_HIT_NORMAL.
// The generated proc model (spellProcEntryFor, mirroring SpellMgr.cpp:1828)
// only carries PROC_SPELL_PHASE_HIT, so like a C++ build with no
// spell_proc DB rows bearing PHASE_FINISH, the event currently triggers
// nothing — the bridge is structural, and any future PHASE_FINISH entry
// fires through the real CanSpellTriggerProcOnEvent phase gate.
func (s *session) procSpellFinishAuraTriggers(ctx context.Context, spell wotlk.Spell) {
	if s == nil || s.server == nil || s.server.Data == nil {
		return
	}
	typeMask := spellFinishPhaseProcTypeMask(spell)
	if typeMask == procFlagNone {
		return
	}
	schoolMask := spell.SchoolMask
	if schoolMask == 0 {
		schoolMask = 1
	}
	spellCopy := spell
	// Spell::_handle_finish_phase (Spell.cpp:3777) carries the cast's
	// accumulated m_hitMask RAW — the PROC_HIT_NORMAL normalization
	// (Spell.cpp:3541-3543) is a _cast-phase leg only. A cast whose targets
	// were all e.g. fully resisted lands on the FINISH event with just
	// PROC_HIT_FULL_RESIST (createProcHitMask sets no hit/crit bit for
	// nullified damage, Unit.cpp:10232-10241); the empty-target arm
	// (Spell.cpp:3604-3605) seeds PROC_HIT_NORMAL before any processing.
	s.procAuraTriggerLoop(ctx, 0, procEventInfo{
		typeMask:             typeMask,
		schoolMask:           schoolMask,
		spellTypeMask:        procSpellTypeMaskAll,
		spellPhaseMask:       procSpellPhaseFinish,
		hitMask:              s.castProcHitMask,
		triggered:            s.triggeredNoProcEvents > 0,
		eventSpell:           &spellCopy,
		triggeredByAuraSpell: s.castTriggerAuraSpell,
		actorGUID:            s.playerGUID,
	})
}

// procSpellCastPhaseAuraTriggers bridges the on-cast proc leg at the end of
// Spell::_cast (Spell.cpp:3525-3545): after the spell_linked_spell tail and
// SetExecutedCurrently(false), the original caster's auras evaluate one proc
// event with PROC_SPELL_PHASE_CAST, PROC_SPELL_TYPE_MASK_ALL, and the
// accumulated hit mask, ProcSkillsAndAuras(m_originalCaster, nullptr,
// procAttacker, PROC_FLAG_NONE, ...). The type mask reuses
// spellFinishPhaseProcTypeMask: C++ runs the same m_procAttacker fallback
// (the prepareDataForTriggerSystem fill, else the magic/none positivity
// split) at both _cast legs (Spell.cpp:3529-3542 vs 3764-3776). The hit mask
// is the cast's accumulated m_hitMask (s.castProcHitMask), normalized to
// PROC_HIT_NORMAL when no target crit (Spell.cpp:3537-3538) — the _cast
// leg normalizes; the FINISH leg carries the mask raw (Spell.cpp:3777).
// The m_originalCaster early-return gate is vacuous here (every cast runs on
// a player session; Go has no creature casters), and the CreatureAI
// OnSpellCastFinished hook has no bridge (no creature casters or AI). The
// event's triggered state mirrors the other phases
// (s.triggeredNoProcEvents), so triggered casts pass through the same
// CanSpellTriggerProcOnEvent gate. Generated proc entries only carry
// PROC_SPELL_PHASE_HIT (spellProcEntryFor, mirroring SpellMgr.cpp:1828) with
// no spell_proc DB table — like a C++ build with no PHASE_CAST rows, the
// event currently triggers nothing; the bridge is structural.
func (s *session) procSpellCastPhaseAuraTriggers(ctx context.Context, spell wotlk.Spell) {
	if s == nil || s.server == nil || s.server.Data == nil {
		return
	}
	typeMask := spellFinishPhaseProcTypeMask(spell)
	if typeMask == procFlagNone {
		return
	}
	schoolMask := spell.SchoolMask
	if schoolMask == 0 {
		schoolMask = 1
	}
	spellCopy := spell
	// Spell::_cast (Spell.cpp:3537-3538): the event carries the cast's
	// accumulated m_hitMask, normalized to PROC_HIT_NORMAL when no target
	// crit — same normalization as the FINISH-phase leg above.
	hitMask := s.castProcHitMask
	if hitMask&procHitCritical == 0 {
		hitMask |= procHitNormal
	}
	s.procAuraTriggerLoop(ctx, 0, procEventInfo{
		typeMask:             typeMask,
		schoolMask:           schoolMask,
		spellTypeMask:        procSpellTypeMaskAll,
		spellPhaseMask:       procSpellPhaseCast,
		hitMask:              hitMask,
		triggered:            s.triggeredNoProcEvents > 0,
		eventSpell:           &spellCopy,
		triggeredByAuraSpell: s.castTriggerAuraSpell,
		actorGUID:            s.playerGUID,
	})
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
// in the mask. The CU_NEGATIVE_EFF bits (SpellInfo.h:190-192) are computed
// at spell load from the DBC-visible arms of _isPositiveEffectImpl
// (SpellInfo.cpp:3385-3847) in wotlk.initializeSpellPositivity, so this is
// the Go SpellInfo::IsPositiveEffect read (SpellInfo.cpp:1213-1222).
func spellNoDmgHealPositive(spell wotlk.Spell, effIndex int) bool {
	return spell.IsPositiveEffect(effIndex)
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
// per-effect positivity fallback (Spell.cpp:2447-2457) swept over every
// spell effect — a negative sibling effect flips the arm to the NEG flags,
// exactly as C++ sweeps the full EffectMask rather than the triggering
// effect alone. Heals always land in Go's model, so the hit mask is
// PROC_HIT_NORMAL — matching C++ createProcHitMask (Unit.cpp:10224-10234)
// for a landed hit with zero damage (MissCondition == SPELL_MISS_NONE, no
// block/absorb/crit bits). Spells with SPELL_ATTR3_CANT_TRIGGER_PROC never
// reach the loop (Spell.cpp:2441). The event carries the casting spell and
// the triggered state (Spell::IsTriggered, Spell.cpp:7501-7504) so the
// CanSpellTriggerProcOnEvent mana-cost, spell-family, and triggered-cast
// gates engage exactly. The trigger spell targets the spell target.
func (s *session) procSpellNoDmgHealAuraTriggers(ctx context.Context, targetGUID uint64, spellID uint32) {
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
	typeMask := spellNoDmgHealProcTypeMask(spell, spellDamageNoDmgPositive(spell))
	if typeMask == procFlagNone {
		return
	}
	schoolMask := spell.SchoolMask
	if schoolMask == 0 {
		schoolMask = 1
	}
	spellCopy := spell
	s.procAuraTriggerLoop(ctx, targetGUID, procEventInfo{
		typeMask:             typeMask,
		schoolMask:           schoolMask,
		spellTypeMask:        procSpellTypeNoDmgHeal,
		spellPhaseMask:       procSpellPhaseHit,
		hitMask:              procHitNormal,
		triggered:            s.triggeredNoProcEvents > 0,
		eventSpell:           &spellCopy,
		triggeredByAuraSpell: s.castTriggerAuraSpell,
		actorGUID:            s.playerGUID,
	})
}

// procSpellNoDmgHealTakenAuraTriggers evaluates real aura procs on the taken
// side of a zero-heal spell: the victim-side half of the no-damage arm
// (Spell.cpp:2462-2473, 2563-2579, 2581-2586). The event mirrors the
// done-side no-damage event exactly (PROC_SPELL_TYPE_NO_DMG_HEAL,
// PROC_SPELL_PHASE_HIT, PROC_HIT_NORMAL, the casting spell, and the
// triggered state) and the type mask shares the done side's full-effect
// positivity sweep — C++ computes one positive value for both the
// procAttacker and procVictim flags — so the target's
// TAKEN_SPELL_*_DMG_CLASS_POS/NEG auras gate against it. Runs on the
// target's session so its own auras gate; the trigger spell targets the
// caster. The triggered state and the triggering-aura attribution come from
// the caster's cast, not the victim's session, matching Spell::IsTriggered
// and Spell::m_triggeredByAuraSpell on the shared spell (Spell.cpp:3068).
func (s *session) procSpellNoDmgHealTakenAuraTriggers(ctx context.Context, casterGUID uint64, spellID uint32, casterTriggered bool, triggerAuraSpell uint32) {
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
	typeMask := spellNoDmgHealTakenProcTypeMask(spell, spellDamageNoDmgPositive(spell))
	if typeMask == procFlagNone {
		return
	}
	schoolMask := spell.SchoolMask
	if schoolMask == 0 {
		schoolMask = 1
	}
	spellCopy := spell
	s.procAuraTriggerLoop(ctx, casterGUID, procEventInfo{
		typeMask:             typeMask,
		schoolMask:           schoolMask,
		spellTypeMask:        procSpellTypeNoDmgHeal,
		spellPhaseMask:       procSpellPhaseHit,
		hitMask:              procHitNormal,
		triggered:            casterTriggered,
		eventSpell:           &spellCopy,
		triggeredByAuraSpell: triggerAuraSpell,
		actorGUID:            casterGUID,
	})
}

// spellHasHealEffect reports whether any of the spell's effects is a heal
// effect type (SPELL_EFFECT_HEAL/HEAL_PCT/HEAL_MAX_HEALTH/HEAL_MECHANICAL,
// SpellInfo.cpp:3501-3505, 3571-3577). DoDamageAndTriggers keys hasHealing
// off spell->m_healing; in Go the heal path (executeSpellHeal) owns the
// trigger decision for any spell carrying heal effects, so the damage path
// takes the no-damage arm only for spells without them.
func spellHasHealEffect(spell wotlk.Spell) bool {
	for i := range spell.Effects {
		switch spell.Effects[i].Effect {
		case 10, // SPELL_EFFECT_HEAL
			spellEffectHealMaxHealth,
			spellEffectHealMechanical,
			spellEffectHealPct:
			return true
		}
	}
	return false
}

// spellDamageNoDmgPositive folds the per-effect positivity fallback
// (Spell.cpp:2447-2457) over every spell effect for the no-damage arm: a
// single negative effect flips the arm negative, matching C++'s EffectMask
// sweep over IsPositiveEffect. Used on both the damage path (zero incoming
// damage) and the heal path (zero healing) — C++ runs one shared fallback
// in Spell::TargetInfo::DoDamageAndTriggers, never the triggering effect
// alone. The per-effect read is spellNoDmgHealPositive above, backed by the
// CU_NEGATIVE_EFF bits computed at spell load.
func spellDamageNoDmgPositive(spell wotlk.Spell) bool {
	for i := range spell.Effects {
		if !spellNoDmgHealPositive(spell, i) {
			return false
		}
	}
	return true
}

// procSpellDamageNoDmgAuraTriggers evaluates real aura procs on the done
// side of a spell whose incoming damage is zero and which has no healing
// effects: the no-damage arm of Unit::ProcDamageAndSpellFor via
// Spell::TargetInfo::DoDamageAndTriggers (Spell.cpp:2563-2579, 2581-2586).
// C++ keys the arm off the incoming damage (if (spell->m_damage > 0)
// hasDamage = true, Spell.cpp:2519) — damage later reduced to zero by
// absorb or resist stays on the damage arm — and keys the type mask off
// the per-effect positivity fallback (Spell.cpp:2447-2457), not the damage
// arm's assumed NEG. The event carries PROC_SPELL_TYPE_NO_DMG_HEAL
// (SpellMgr.h:206) with PROC_SPELL_PHASE_HIT; the hit mask is
// PROC_HIT_NORMAL for a landed hit — the arm builds a fresh
// SpellNonMeleeDamage (HitInfo 0, absorb 0, no block), so createProcHitMask
// (Unit.cpp:10179-10234) emits neither the crit bit (fresh HitInfo carries
// no SPELL_HIT_TYPE_CRIT) nor the absorb/full-absorb bits — and the miss
// and immune arms are shared with the damage path. A full resist is
// unreachable here (calcMagicSpellResistance early-outs on zero damage),
// as is absorption (the absorption guards require damage > 0). Spells with
// heal effects fire nothing on the damage side: the heal path owns their
// trigger decision, matching C++ where hasHealing suppresses the arm.
// Spells with SPELL_ATTR3_CANT_TRIGGER_PROC never reach the loop
// (Spell.cpp:2441). The event carries the casting spell and the triggered
// state (Spell::IsTriggered, Spell.cpp:7501-7504) so the
// CanSpellTriggerProcOnEvent mana-cost, spell-family, and triggered-cast
// gates engage exactly. The trigger spell targets the victim.
func (s *session) procSpellDamageNoDmgAuraTriggers(ctx context.Context, targetGUID uint64, spellID uint32, miss uint8, immune, reflected bool) {
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
	if spellHasHealEffect(spell) {
		return
	}
	typeMask := spellNoDmgHealProcTypeMask(spell, spellDamageNoDmgPositive(spell))
	if typeMask == procFlagNone {
		return
	}
	schoolMask := spell.SchoolMask
	if schoolMask == 0 {
		schoolMask = 1
	}
	spellCopy := spell
	s.procAuraTriggerLoop(ctx, targetGUID, procEventInfo{
		typeMask:             typeMask,
		schoolMask:           schoolMask,
		spellTypeMask:        procSpellTypeNoDmgHeal,
		spellPhaseMask:       procSpellPhaseHit,
		hitMask:              spellNoDmgMissProcHitMask(miss, immune, reflected),
		triggered:            s.triggeredNoProcEvents > 0,
		eventSpell:           &spellCopy,
		triggeredByAuraSpell: s.castTriggerAuraSpell,
		actorGUID:            s.playerGUID,
	})
}

// procSpellDamageNoDmgTakenAuraTriggers evaluates real aura procs on the
// taken side of a spell whose incoming damage is zero and which has no
// healing effects: the victim-side half of the no-damage arm of
// Unit::ProcDamageAndSpellFor via Spell::TargetInfo::DoDamageAndTriggers
// (Spell.cpp:2563-2579, 2581-2586; Unit::TriggerAurasProcOnEvent,
// Unit.cpp:10385-10418). The event mirrors the done-side no-damage event
// (PROC_SPELL_TYPE_NO_DMG_HEAL, PROC_SPELL_PHASE_HIT, the casting spell, the
// miss/immune/shared hit mask) but the type mask is the taken-side
// positivity fallback; the no-damage arm never ORs PROC_FLAG_TAKEN_DAMAGE
// into procVictim (Spell.cpp:2545 sits inside the damage arm only). Spells
// with heal effects fire nothing on the damage side — the heal path owns
// their trigger decision, matching C++ where hasHealing suppresses the arm.
// Runs on the victim's session so its own auras gate; the trigger spell
// targets the caster (Unit.cpp:10413). The triggered state comes from the
// caster's cast, not the victim's session, matching Spell::IsTriggered on
// the shared spell.
func (s *session) procSpellDamageNoDmgTakenAuraTriggers(ctx context.Context, casterGUID uint64, spellID uint32, miss uint8, immune, casterTriggered, reflected bool, triggerAuraSpell uint32) {
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
	if spellHasHealEffect(spell) {
		return
	}
	typeMask := spellNoDmgHealTakenProcTypeMask(spell, spellDamageNoDmgPositive(spell))
	if typeMask == procFlagNone {
		return
	}
	schoolMask := spell.SchoolMask
	if schoolMask == 0 {
		schoolMask = 1
	}
	spellCopy := spell
	s.procAuraTriggerLoop(ctx, casterGUID, procEventInfo{
		typeMask:             typeMask,
		schoolMask:           schoolMask,
		spellTypeMask:        procSpellTypeNoDmgHeal,
		spellPhaseMask:       procSpellPhaseHit,
		hitMask:              spellNoDmgMissProcHitMask(miss, immune, reflected),
		triggered:            casterTriggered,
		eventSpell:           &spellCopy,
		triggeredByAuraSpell: triggerAuraSpell,
		actorGUID:            casterGUID,
	})
}

// fireCreatureMissNodmgTakenProcs is the creature-victim twin of
// procSpellDamageNoDmgTakenAuraTriggers for a missed target's no-damage arm
// (Spell.cpp:2563-2579): the victim creature's own auras proc with the
// taken-side positivity fallback, PROC_SPELL_TYPE_NO_DMG_HEAL,
// PROC_SPELL_PHASE_HIT, and the miss-kind hit mask, and the trigger spell
// targets the caster. The caller gates on motion high words (0xF130
// creature, 0xF140 pet, 0xF150 vehicle): pet motions are keyed by petGUID
// in the same motion map (registerPetMotion) and findCreatureMotion
// resolves them by direct map hit, the 11959 hit-event precedent, so no
// separate pet proc loop is needed; the creature path carries no
// per-motion procDeep model (boss_ai.go), so no CanProc gate applies here,
// matching the other creature taken legs.
func (s *Server) fireCreatureMissNodmgTakenProcs(ctx context.Context, motion *creatureMotion, casterGUID uint64, spell wotlk.Spell, miss uint8, triggered bool, triggerAuraSpell uint32) {
	if s == nil || motion == nil {
		return
	}
	if spell.AttributesEx3&spellAttr3CantTriggerProc != 0 {
		return
	}
	if spellHasHealEffect(spell) {
		return
	}
	typeMask := spellNoDmgHealTakenProcTypeMask(spell, spellDamageNoDmgPositive(spell))
	if typeMask == procFlagNone {
		return
	}
	schoolMask := spell.SchoolMask
	if schoolMask == 0 {
		schoolMask = 1
	}
	spellCopy := spell
	s.fireCreatureAuraProcLoop(ctx, motion, casterGUID, procEventInfo{
		typeMask:             typeMask,
		schoolMask:           schoolMask,
		spellTypeMask:        procSpellTypeNoDmgHeal,
		spellPhaseMask:       procSpellPhaseHit,
		hitMask:              spellNoDmgMissProcHitMask(miss, false, false),
		triggered:            triggered,
		eventSpell:           &spellCopy,
		triggeredByAuraSpell: triggerAuraSpell,
		actorGUID:            casterGUID,
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
func spellDamageProcHitMask(isHit, immune, fullyResisted, fullAbsorb, crit bool, absorbed uint32, reflected bool) uint32 {
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
	// createProcHitMask (Unit.cpp:10179-10247) on a reflected hit: the
	// TargetInfo's MissCondition stays SPELL_MISS_REFLECT into
	// DoDamageAndTriggers (Spell.cpp:2544), so the missCondition != NONE
	// switch returns PROC_HIT_REFLECT alone — the missCondition == NONE
	// arm (normal/critical, absorb, block, full-resist, nullification)
	// never runs, no matter how the return-trip damage crit, absorbed,
	// or resisted. The damage arm's immunity check instead ASSIGNS
	// PROC_HIT_IMMUNE (Spell.cpp:2540-2546); the no-damage arm carries
	// no immunity check, so its callers pass false for immune and the
	// immune-first fold above is damage-arm-only semantics.
	// The heal arm never calls createProcHitMask, so heals take no
	// reflect bit.
	if reflected {
		if immune {
			return procHitImmune
		}
		return procHitReflect
	}
	return hitMask
}

// spellMissProcHitMask mirrors the missCondition switch of createProcHitMask
// (Unit.cpp:10179-10247) for the no-damage arm: the arm builds a fresh
// SpellNonMeleeDamage (HitInfo 0, absorb 0, no block), so the miss kind
// alone decides the mask — SPELL_MISS_NONE lands as NORMAL, SPELL_MISS_BLOCK
// yields BLOCK|FULL_BLOCK ("spells can't be partially blocked"), and any
// unknown kind yields PROC_HIT_NONE (the C++ default arm).
func spellMissProcHitMask(miss uint8) uint32 {
	switch miss {
	case protocol.SpellMissNone:
		return procHitNormal
	case protocol.SpellMissMiss:
		return procHitMiss
	case protocol.SpellMissDodge:
		return procHitDodge
	case protocol.SpellMissParry:
		return procHitParry
	case protocol.SpellMissBlock:
		return procHitBlock | procHitFullBlock
	case protocol.SpellMissEvade:
		return procHitEvade
	case protocol.SpellMissImmune, protocol.SpellMissImmune2:
		return procHitImmune
	case protocol.SpellMissDeflect:
		return procHitDeflect
	case protocol.SpellMissAbsorb:
		return procHitAbsorb
	case protocol.SpellMissReflect:
		return procHitReflect
	case protocol.SpellMissResist:
		return procHitFullResist
	default:
		return procHitNone
	}
}

// spellNoDmgMissProcHitMask folds a miss kind with the damage-arm-only
// immune/reflected overrides of spellDamageProcHitMask: the no-damage arm
// carries no IsImmunedToDamage check (Spell.cpp:2563-2575), so its callers
// pass false for immune and the immune fold below only preserves the
// shared helper's shape for the reflected arm (Spell.cpp:2540-2546).
func spellNoDmgMissProcHitMask(miss uint8, immune, reflected bool) uint32 {
	if reflected {
		if immune {
			return procHitImmune
		}
		return procHitReflect
	}
	if immune {
		return procHitImmune
	}
	return spellMissProcHitMask(miss)
}

// procSpellHitAuraTriggers evaluates real aura procs on the done side of a
// spell damage hit: the spell-damage arm of Unit::ProcDamageAndSpellFor via
// Spell::TargetInfo::DoDamageAndTriggers (Spell.cpp:2427-2540). The event
// carries the casting spell and the triggered state (Spell::IsTriggered,
// Spell.cpp:7501-7504), so the CanSpellTriggerProcOnEvent mana-cost,
// spell-family, and triggered-cast gates engage exactly. Spells with
// SPELL_ATTR3_CANT_TRIGGER_PROC never reach the loop (Spell.cpp:2441).
// The trigger spell targets the victim.
func (s *session) procSpellHitAuraTriggers(ctx context.Context, targetGUID uint64, spellID uint32, isHit, immune, fullyResisted, fullAbsorb, crit bool, absorbed, damage uint32, reflected, blocked, fullBlock bool) {
	if s == nil || s.server == nil || s.server.Data == nil {
		return
	}
	// Spell::m_hitMask (Spell.cpp:2603): the per-target mask accumulates
	// unconditionally — the |= sits outside the canEffectTrigger gate, so
	// even suppressed or CANT_TRIGGER_PROC targets feed the FINISH/CAST
	// phase mask.
	hitMask := spellDamageProcHitMask(isHit, immune, fullyResisted, fullAbsorb, crit, absorbed, reflected)
	// createProcHitMask block arm (Unit.cpp:10227-10246), same fold as the
	// taken helper: partial block ORs BLOCK, full block replaces the
	// normal/critical bit with BLOCK | FULL_BLOCK (absorb rides along);
	// immune and reflected trips take neither.
	if !reflected && !immune {
		if fullBlock {
			hitMask = procHitBlock | procHitFullBlock
			if absorbed > 0 {
				hitMask |= procHitAbsorb
			}
		} else if blocked {
			hitMask |= procHitBlock
		}
	}
	s.castProcHitMask |= hitMask
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
		typeMask:             spellDamageProcTypeMask(spell),
		schoolMask:           schoolMask,
		spellTypeMask:        procSpellTypeDamage,
		spellPhaseMask:       procSpellPhaseHit,
		hitMask:              hitMask,
		triggered:            s.triggeredNoProcEvents > 0,
		eventSpell:           &spellCopy,
		triggeredByAuraSpell: s.castTriggerAuraSpell,
		actorGUID:            s.playerGUID,
		damage:               damage,
	})
}

// procSpellHitTakenAuraTriggers evaluates real aura procs on the taken side
// of a spell damage hit: the victim-side half of Unit::ProcDamageAndSpellFor
// via Spell::TargetInfo::DoDamageAndTriggers (Spell.cpp:2427-2540, 2581-2586;
// Unit::TriggerAurasProcOnEvent, Unit.cpp:10385-10418). The event mirrors
// the done-side damage event (PROC_SPELL_TYPE_DAMAGE, PROC_SPELL_PHASE_HIT,
// the casting spell, the same hit mask) but the type mask is the victim-side
// pre-fill/fallback, and the damage arm ORs PROC_FLAG_TAKEN_DAMAGE into
// procVictim (Spell.cpp:2545) — skipped on the immune arm, whose branch
// bypasses the TAKEN_DAMAGE OR. Block bits ride the
// createProcHitMask arm (Unit.cpp:10227-10246): blocked ORs PROC_HIT_BLOCK
// and fullBlock replaces the normal/critical bit with PROC_HIT_BLOCK |
// PROC_HIT_FULL_BLOCK (absorb rides along); immune and reflected trips
// take neither. Spells with SPELL_ATTR3_CANT_TRIGGER_PROC
// never reach the loop (Spell.cpp:2441). Runs on the victim's session so its
// own auras gate; the trigger spell targets the caster (Unit.cpp:10413). The
// triggered state comes from the caster's cast, not the victim's session,
// matching Spell::IsTriggered on the shared spell.
func (s *session) procSpellHitTakenAuraTriggers(ctx context.Context, casterGUID uint64, spellID uint32, isHit, immune, fullyResisted, fullAbsorb, crit bool, absorbed, damage uint32, casterTriggered, reflected bool, triggerAuraSpell uint32, blocked, fullBlock bool) {
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
	typeMask := spellDamageTakenProcTypeMask(spell)
	if !immune {
		typeMask |= procFlagTakenDamage
	}
	schoolMask := spell.SchoolMask
	if schoolMask == 0 {
		schoolMask = 1
	}
	spellCopy := spell
	// createProcHitMask (Unit.cpp:10227-10246) on the missCondition == NONE
	// arm: a blocked hit carries PROC_HIT_BLOCK, and a fully blocked hit
	// carries PROC_HIT_BLOCK | PROC_HIT_FULL_BLOCK in place of the
	// normal/critical bit (the absorb bit rides along); the immune arm
	// assigns IMMUNE and the reflected arm yields REFLECT alone, so
	// neither takes block bits — the same fold the creature-victim twin
	// applies outside this helper.
	hitMask := spellDamageProcHitMask(isHit, immune, fullyResisted, fullAbsorb, crit, absorbed, reflected)
	if !reflected && !immune {
		if fullBlock {
			hitMask = procHitBlock | procHitFullBlock
			if absorbed > 0 {
				hitMask |= procHitAbsorb
			}
		} else if blocked {
			hitMask |= procHitBlock
		}
	}
	s.procAuraTriggerLoop(ctx, casterGUID, procEventInfo{
		typeMask:             typeMask,
		schoolMask:           schoolMask,
		spellTypeMask:        procSpellTypeDamage,
		spellPhaseMask:       procSpellPhaseHit,
		hitMask:              hitMask,
		triggered:            casterTriggered,
		eventSpell:           &spellCopy,
		triggeredByAuraSpell: triggerAuraSpell,
		actorGUID:            casterGUID,
		damage:               damage,
	})
}

// procPeriodicDamageTickAuraTriggers evaluates real aura procs on the done
// side of a periodic damage tick (SpellAuraEffects.cpp:5210-5225): the tail of
// AuraEffect::HandlePeriodicDamageAurasTick (and the leech tick at :5295-5305)
// runs Unit::ProcSkillsAndAuras(caster, target, PROC_FLAG_DONE_PERIODIC,
// PROC_FLAG_TAKEN_PERIODIC | (damage ? PROC_FLAG_TAKEN_DAMAGE : 0),
// PROC_SPELL_TYPE_DAMAGE, PROC_SPELL_PHASE_HIT, hitMask, nullptr, &damageInfo,
// nullptr). C++ passes no Spell, so ProcEventInfo::GetProcSpell is nil and the
// triggered-cast suppression in CanSpellTriggerProcOnEvent (SpellMgr.cpp:526)
// never engages — the tick event is deliberately NOT marked triggered. There
// is no SPELL_ATTR3_CANT_TRIGGER_PROC arm on the tick path (that gate lives in
// Spell::TargetInfo::DoDamageAndTriggers, Spell.cpp:2440), so none is checked
// here. The hit mask starts from the absorb/resist state (C++
// damageInfo.GetHitMask() after CalcAbsorbResist) with PROC_HIT_NORMAL /
// PROC_HIT_CRITICAL ORed in only when damage landed; the absorb bit rides
// along via spellDamageProcHitMask like the direct-damage path. Runs on the
// caster's own session; offline casters (no session) skip the done side —
// documented delta.
func (s *session) procPeriodicDamageTickAuraTriggers(ctx context.Context, tickSpell wotlk.Spell, schoolMask uint32, targetGUID uint64, crit bool, absorbed, resisted, damage uint32) {
	if s == nil || s.server == nil || s.server.Data == nil {
		return
	}
	spellCopy := tickSpell
	s.procAuraTriggerLoop(ctx, targetGUID, procEventInfo{
		typeMask:       procFlagDonePeriodic,
		schoolMask:     schoolMask,
		spellTypeMask:  procSpellTypeDamage,
		spellPhaseMask: procSpellPhaseHit,
		hitMask:        spellDamageProcHitMask(true, false, resisted > 0 && damage == 0, absorbed > 0 && damage == 0, crit, absorbed, false),
		triggered:      false,
		eventSpell:     &spellCopy,
		actorGUID:      s.playerGUID,
		damage:         damage,
	})
}

// procPeriodicDamageTickTakenAuraTriggers is the victim-side half of the
// periodic damage tick proc (SpellAuraEffects.cpp:5210-5225): the target's own
// session runs the loop so its TAKEN_PERIODIC / TAKEN_DAMAGE auras gate
// (Unit::TriggerAurasProcOnEvent, Unit.cpp:10413-10418); the trigger targets
// the caster. PROC_FLAG_TAKEN_DAMAGE is ORed in only when damage landed, like
// C++. Creature victims have no aura plumbing in Go, so only online players
// run the taken pass, like the direct-damage path.
func (s *session) procPeriodicDamageTickTakenAuraTriggers(ctx context.Context, tickSpell wotlk.Spell, schoolMask uint32, casterGUID uint64, crit bool, absorbed, resisted, damage uint32) {
	if s == nil || s.server == nil || s.server.Data == nil {
		return
	}
	typeMask := procFlagTakenPeriodic
	if damage > 0 {
		typeMask |= procFlagTakenDamage
	}
	spellCopy := tickSpell
	s.procAuraTriggerLoop(ctx, casterGUID, procEventInfo{
		typeMask:       typeMask,
		schoolMask:     schoolMask,
		spellTypeMask:  procSpellTypeDamage,
		spellPhaseMask: procSpellPhaseHit,
		hitMask:        spellDamageProcHitMask(true, false, resisted > 0 && damage == 0, absorbed > 0 && damage == 0, crit, absorbed, false),
		triggered:      false,
		eventSpell:     &spellCopy,
		actorGUID:      casterGUID,
		damage:         damage,
	})
}

// procPeriodicPowerBurnTickAuraTriggers evaluates real aura procs on the done
// side of a power-burn tick (SpellAuraEffects.cpp:5583-5623): the tail of
// AuraEffect::HandlePeriodicPowerBurnAuraTick runs Unit::ProcSkillsAndAuras(caster,
// target, PROC_FLAG_DONE_PERIODIC, PROC_FLAG_TAKEN_PERIODIC | (damage ?
// PROC_FLAG_TAKEN_DAMAGE : 0), PROC_SPELL_TYPE_NO_DMG_HEAL | (damage ?
// PROC_SPELL_TYPE_DAMAGE : 0), PROC_SPELL_PHASE_HIT, hitMask, nullptr,
// &dotDamageInfo, nullptr). A burn that deals no damage still procs as a
// no-damage event (C++ always runs the call); the hit mask is built the same
// way as the damage tick, with no crit model on the burn path (matching C++,
// which never crits the burn). The tick event is deliberately NOT marked
// triggered (C++ passes no Spell).
func (s *session) procPeriodicPowerBurnTickAuraTriggers(ctx context.Context, burnSpell wotlk.Spell, schoolMask uint32, targetGUID uint64, absorbed, resisted, damage uint32) {
	if s == nil || s.server == nil || s.server.Data == nil {
		return
	}
	spellTypeMask := procSpellTypeNoDmgHeal
	if damage > 0 {
		spellTypeMask |= procSpellTypeDamage
	}
	spellCopy := burnSpell
	s.procAuraTriggerLoop(ctx, targetGUID, procEventInfo{
		typeMask:       procFlagDonePeriodic,
		schoolMask:     schoolMask,
		spellTypeMask:  spellTypeMask,
		spellPhaseMask: procSpellPhaseHit,
		hitMask:        spellDamageProcHitMask(true, false, resisted > 0 && damage == 0, absorbed > 0 && damage == 0, false, absorbed, false),
		triggered:      false,
		eventSpell:     &spellCopy,
		actorGUID:      s.playerGUID,
		damage:         damage,
	})
}

// procPeriodicPowerBurnTickTakenAuraTriggers is the victim-side half of the
// power-burn tick proc (SpellAuraEffects.cpp:5583-5623): the victim's own
// session runs the loop so its TAKEN_PERIODIC / TAKEN_DAMAGE auras gate; the
// trigger targets the caster. PROC_FLAG_TAKEN_DAMAGE and
// PROC_SPELL_TYPE_DAMAGE are ORed in only when damage landed, like C++.
func (s *session) procPeriodicPowerBurnTickTakenAuraTriggers(ctx context.Context, burnSpell wotlk.Spell, schoolMask uint32, casterGUID uint64, absorbed, resisted, damage uint32) {
	if s == nil || s.server == nil || s.server.Data == nil {
		return
	}
	typeMask := procFlagTakenPeriodic
	spellTypeMask := procSpellTypeNoDmgHeal
	if damage > 0 {
		typeMask |= procFlagTakenDamage
		spellTypeMask |= procSpellTypeDamage
	}
	spellCopy := burnSpell
	s.procAuraTriggerLoop(ctx, casterGUID, procEventInfo{
		typeMask:       typeMask,
		schoolMask:     schoolMask,
		spellTypeMask:  spellTypeMask,
		spellPhaseMask: procSpellPhaseHit,
		hitMask:        spellDamageProcHitMask(true, false, resisted > 0 && damage == 0, absorbed > 0 && damage == 0, false, absorbed, false),
		triggered:      false,
		eventSpell:     &spellCopy,
		actorGUID:      casterGUID,
		damage:         damage,
	})
}

// procPeriodicHealthFunnelAuraTriggers evaluates real aura procs on the done
// side of a Health Funnel tick heal (SpellAuraEffects.cpp:5350):
// Unit::ProcSkillsAndAuras(caster, target, PROC_FLAG_DONE_PERIODIC,
// PROC_FLAG_TAKEN_PERIODIC, PROC_SPELL_TYPE_HEAL, PROC_SPELL_PHASE_HIT,
// PROC_HIT_NORMAL, nullptr, nullptr, &healInfo). No crit, no absorb, no damage
// mask on the taken side — C++ never ORs TAKEN_DAMAGE here. Runs on the
// caster's own session; offline casters skip the done side.
func (s *session) procPeriodicHealthFunnelAuraTriggers(ctx context.Context, funnelSpell wotlk.Spell, schoolMask uint32, targetGUID uint64) {
	if s == nil || s.server == nil || s.server.Data == nil {
		return
	}
	spellCopy := funnelSpell
	s.procAuraTriggerLoop(ctx, targetGUID, procEventInfo{
		typeMask:       procFlagDonePeriodic,
		schoolMask:     schoolMask,
		spellTypeMask:  procSpellTypeHeal,
		spellPhaseMask: procSpellPhaseHit,
		hitMask:        procHitNormal,
		triggered:      false,
		eventSpell:     &spellCopy,
		actorGUID:      s.playerGUID,
	})
}

// procPeriodicHealthFunnelTakenAuraTriggers is the victim-side half of the
// Health Funnel tick heal proc (SpellAuraEffects.cpp:5350): the heal target's
// own session runs the loop so its TAKEN_PERIODIC auras gate; the trigger
// targets the caster. Creature targets have no aura plumbing in Go.
func (s *session) procPeriodicHealthFunnelTakenAuraTriggers(ctx context.Context, funnelSpell wotlk.Spell, schoolMask uint32, casterGUID uint64) {
	if s == nil || s.server == nil || s.server.Data == nil {
		return
	}
	spellCopy := funnelSpell
	s.procAuraTriggerLoop(ctx, casterGUID, procEventInfo{
		typeMask:       procFlagTakenPeriodic,
		schoolMask:     schoolMask,
		spellTypeMask:  procSpellTypeHeal,
		spellPhaseMask: procSpellPhaseHit,
		hitMask:        procHitNormal,
		triggered:      false,
		eventSpell:     &spellCopy,
		actorGUID:      casterGUID,
	})
}

// procPeriodicHealTickAuraTriggers evaluates real aura procs on the done side
// of a periodic heal tick (SpellAuraEffects.cpp:5423-5433): the tail of
// AuraEffect::HandlePeriodicHealAurasTick runs Unit::ProcSkillsAndAuras(caster,
// target, PROC_FLAG_DONE_PERIODIC, PROC_FLAG_TAKEN_PERIODIC,
// PROC_SPELL_TYPE_HEAL, PROC_SPELL_PHASE_HIT, hitMask, nullptr, nullptr,
// &healInfo) where hitMask = PROC_HIT_CRITICAL on a tick crit and
// PROC_HIT_NORMAL otherwise. C++ passes no Spell, so ProcEventInfo::GetProcSpell
// is nil and the triggered-cast suppression in CanSpellTriggerProcOnEvent
// (SpellMgr.cpp:526) never engages — the tick event is deliberately NOT marked
// triggered. Runs on the caster's own session; offline casters (no session)
// skip the done side — documented delta, same as the damage-tick legs.
func (s *session) procPeriodicHealTickAuraTriggers(ctx context.Context, tickSpell wotlk.Spell, schoolMask uint32, targetGUID uint64, crit bool) {
	if s == nil || s.server == nil || s.server.Data == nil {
		return
	}
	hitMask := procHitNormal
	if crit {
		hitMask = procHitCritical
	}
	spellCopy := tickSpell
	s.procAuraTriggerLoop(ctx, targetGUID, procEventInfo{
		typeMask:       procFlagDonePeriodic,
		schoolMask:     schoolMask,
		spellTypeMask:  procSpellTypeHeal,
		spellPhaseMask: procSpellPhaseHit,
		hitMask:        hitMask,
		triggered:      false,
		eventSpell:     &spellCopy,
		actorGUID:      s.playerGUID,
	})
}

// procPeriodicHealTickTakenAuraTriggers is the victim-side half of the
// periodic heal tick proc (SpellAuraEffects.cpp:5423-5433): the heal target's
// own session runs the loop so its TAKEN_PERIODIC auras gate; the trigger
// targets the caster. No TAKEN_DAMAGE arm — C++ ORs none on the heal path.
// Creature targets have no aura plumbing in Go.
func (s *session) procPeriodicHealTickTakenAuraTriggers(ctx context.Context, tickSpell wotlk.Spell, schoolMask uint32, casterGUID uint64, crit bool) {
	if s == nil || s.server == nil || s.server.Data == nil {
		return
	}
	hitMask := procHitNormal
	if crit {
		hitMask = procHitCritical
	}
	spellCopy := tickSpell
	s.procAuraTriggerLoop(ctx, casterGUID, procEventInfo{
		typeMask:       procFlagTakenPeriodic,
		schoolMask:     schoolMask,
		spellTypeMask:  procSpellTypeHeal,
		spellPhaseMask: procSpellPhaseHit,
		hitMask:        hitMask,
		triggered:      false,
		eventSpell:     &spellCopy,
		actorGUID:      casterGUID,
	})
}

// procPeriodicLeechSelfHealAuraTriggers fires both legs of the
// PERIODIC_LEECH self-heal proc (SpellAuraEffects.cpp:5316-5320): the tail of
// AuraEffect::HandlePeriodicHealthLeechAuraTick runs
// Unit::ProcSkillsAndAuras(caster, caster, PROC_FLAG_DONE_PERIODIC,
// PROC_FLAG_TAKEN_PERIODIC, PROC_SPELL_TYPE_HEAL, PROC_SPELL_PHASE_HIT,
// hitMask, nullptr, nullptr, &healInfo) — both legs evaluate on the caster's
// own auras with the caster itself as trigger target and attacker. C++ fires
// it unconditionally of the healed amount (no heal==0 gate); the caller
// carries the unknown-spell gate like the rest of the tick's proc legs. The
// hit mask reuses the damage half's in C++ (absorb/resist bits included),
// so Go builds the same mask the damage-tick legs use rather than the
// crit-conditional normal/critical heal-tick mask.
func (s *session) procPeriodicLeechSelfHealAuraTriggers(ctx context.Context, leechSpell wotlk.Spell, schoolMask uint32, casterGUID uint64, crit bool, absorbed, resisted, damage uint32) {
	if s == nil || s.server == nil || s.server.Data == nil {
		return
	}
	hitMask := spellDamageProcHitMask(true, false, resisted > 0 && damage == 0, absorbed > 0 && damage == 0, crit, absorbed, false)
	spellCopy := leechSpell
	s.procAuraTriggerLoop(ctx, casterGUID, procEventInfo{
		typeMask:       procFlagDonePeriodic,
		schoolMask:     schoolMask,
		spellTypeMask:  procSpellTypeHeal,
		spellPhaseMask: procSpellPhaseHit,
		hitMask:        hitMask,
		triggered:      false,
		eventSpell:     &spellCopy,
		actorGUID:      s.playerGUID,
	})
	s.procAuraTriggerLoop(ctx, casterGUID, procEventInfo{
		typeMask:       procFlagTakenPeriodic,
		schoolMask:     schoolMask,
		spellTypeMask:  procSpellTypeHeal,
		spellPhaseMask: procSpellPhaseHit,
		hitMask:        hitMask,
		triggered:      false,
		eventSpell:     &spellCopy,
		actorGUID:      casterGUID,
	})
}

// procSpellReflectTakenAuraTriggers fires the reflector-side proc for a
// reflected spell: ProcReflectDelayed (Spell.cpp:2069-2090, scheduled at
// Spell.cpp:2181) runs Unit::ProcSkillsAndAuras(caster, reflector,
// PROC_FLAG_NONE, PROC_FLAG_TAKEN_SPELL_MAGIC_DMG_CLASS_NEG |
// PROC_FLAG_TAKEN_SPELL_NONE_DMG_CLASS_NEG, PROC_SPELL_TYPE_DAMAGE |
// PROC_SPELL_TYPE_NO_DMG_HEAL, PROC_SPELL_PHASE_NONE, PROC_HIT_REFLECT) at
// outbound-missile arrival. The actor mask is NONE so only the reflector's
// taken-side auras evaluate, carrying the REFLECT hit bit. Runs on the
// reflector's session so its own auras gate; the trigger resolves against
// the original caster, and the triggered state is captured from the
// reflect-causing cast (Spell::IsTriggered, Spell.cpp:7501-7504).
func (s *session) procSpellReflectTakenAuraTriggers(ctx context.Context, casterGUID uint64, spell wotlk.Spell, triggered bool) {
	if s == nil || s.server == nil || s.server.Data == nil {
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
	s.procAuraTriggerLoop(ctx, casterGUID, procEventInfo{
		typeMask:       procFlagTakenSpellMagicDmgClassNeg | procFlagTakenSpellNoneDmgClassNeg,
		schoolMask:     schoolMask,
		spellTypeMask:  procSpellTypeDamage | procSpellTypeNoDmgHeal,
		spellPhaseMask: procSpellPhaseNone,
		hitMask:        procHitReflect,
		triggered:      triggered,
		eventSpell:     &spellCopy,
		actorGUID:      casterGUID,
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
func (s *session) procSpellHealAuraTriggers(ctx context.Context, targetGUID uint64, spellID uint32, heal uint32, effectiveHeal uint32, crit bool) {
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
	// Spell::m_hitMask (Spell.cpp:2506-2509): heal crits OR into the same
	// per-target mask the FINISH/CAST phase events read, so the heal arm
	// accumulates like the damage arm.
	s.castProcHitMask |= hitMask
	spellCopy := spell
	s.procAuraTriggerLoop(ctx, targetGUID, procEventInfo{
		typeMask:             typeMask,
		schoolMask:           schoolMask,
		spellTypeMask:        procSpellTypeHeal,
		spellPhaseMask:       procSpellPhaseHit,
		hitMask:              hitMask,
		triggered:            s.triggeredNoProcEvents > 0,
		eventSpell:           &spellCopy,
		triggeredByAuraSpell: s.castTriggerAuraSpell,
		actorGUID:            s.playerGUID,
		heal:                 heal,
		effectiveHeal:        effectiveHeal,
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
// heal arm. The triggered state and the triggering-aura attribution come from
// the caster's cast, not the victim's session, matching Spell::IsTriggered
// and Spell::m_triggeredByAuraSpell on the shared spell (Spell.cpp:3068).
func (s *session) procSpellHealTakenAuraTriggers(ctx context.Context, healerGUID uint64, spellID uint32, heal uint32, effectiveHeal uint32, crit bool, casterTriggered bool, triggerAuraSpell uint32) {
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
		typeMask:             typeMask,
		schoolMask:           schoolMask,
		spellTypeMask:        procSpellTypeHeal,
		spellPhaseMask:       procSpellPhaseHit,
		hitMask:              hitMask,
		triggered:            casterTriggered,
		eventSpell:           &spellCopy,
		triggeredByAuraSpell: triggerAuraSpell,
		actorGUID:            healerGUID,
		heal:                 heal,
		effectiveHeal:        effectiveHeal,
	})
}

// meleeOutcomeProcHitMask mirrors the DamageInfo constructor's hit-mask
// derivation (Unit.cpp:130-180): the victim-state arms (immune, full
// block), the absorb bits from HitInfo, the block bit from the blocked
// amount, and the damageNullified suppression of the normal/critical bit
// when the damage was fully absorbed, fully resisted, immuned, or fully
// blocked. Block, crushing, and glancing blows count as normal hits; only
// crits set the critical bit. MeleeHitImmune is a Go-side outcome for the
// immune early-return (CalculateMeleeDamage leaves HitOutCome at
// MELEE_HIT_EVADE, Unit.cpp:1178/1213-1220), so it carries the evade bit
// alongside immune, exactly as the C++ ctor does.
func meleeOutcomeProcHitMask(outcome protocol.MeleeHitOutcome, hitInfo uint32, targetState uint8, blocked uint32) uint32 {
	hitMask := procHitNone
	switch targetState {
	case protocol.VictimStateIsImmune:
		hitMask |= procHitImmune
	case protocol.VictimStateBlocks:
		hitMask |= procHitFullBlock
	}
	if hitInfo&(protocol.HitInfoPartialAbsorb|protocol.HitInfoFullAbsorb) != 0 {
		hitMask |= procHitAbsorb
	}
	if hitInfo&protocol.HitInfoFullResist != 0 {
		hitMask |= procHitFullResist
	}
	if blocked > 0 {
		hitMask |= procHitBlock
	}
	damageNullified := hitInfo&(protocol.HitInfoFullAbsorb|protocol.HitInfoFullResist) != 0 ||
		hitMask&(procHitImmune|procHitFullBlock) != 0
	switch outcome {
	case protocol.MeleeHitMiss:
		hitMask |= procHitMiss
	case protocol.MeleeHitDodge:
		hitMask |= procHitDodge
	case protocol.MeleeHitParry:
		hitMask |= procHitParry
	case protocol.MeleeHitEvade, protocol.MeleeHitImmune:
		hitMask |= procHitEvade
	case protocol.MeleeHitCrit:
		if !damageNullified {
			hitMask |= procHitCritical
		}
	case protocol.MeleeHitNormal, protocol.MeleeHitBlock, protocol.MeleeHitGlancing, protocol.MeleeHitCrushing:
		if !damageNullified {
			hitMask |= procHitNormal
		}
	}
	return hitMask
}

// rangedAutoProcHitMask derives the proc hit mask for a ranged auto attack.
// C++ routes ranged auto-shot through the spell arm, not the melee DamageInfo
// ctor: Spell::TargetInfo::DoDamageAndTriggers (Spell.cpp:2527-2531) sets
// hitMask = PROC_HIT_IMMUNE outright on damage immunity, and createProcHitMask
// (Unit.cpp:10206-10209) maps SPELL_MISS_IMMUNE/IMMUNE2 to PROC_HIT_IMMUNE
// alone — the evade bit that accompanies melee immunity (the C++ melee
// early-return leaves HitOutCome at MELEE_HIT_EVADE) never appears. All other
// outcomes follow the melee ctor via meleeOutcomeProcHitMask.
func rangedAutoProcHitMask(outcome protocol.MeleeHitOutcome, hitInfo uint32, targetState uint8, blocked uint32) uint32 {
	if outcome == protocol.MeleeHitImmune {
		return procHitImmune
	}
	return meleeOutcomeProcHitMask(outcome, hitInfo, targetState, blocked)
}

// rangedAutoProcHitState derives the DamageInfo-style mask inputs for a
// ranged auto attack from the values the ranged path tracks. Absorb bits
// come from the absorbed amount (full absorb when nothing of the damage
// remains, the same absorbed > 0 && damage == 0 determination the spell
// path uses); the immune outcome maps to the immune target state. The
// ranged path rolls melee-style outcomes; the mask itself is derived by
// rangedAutoProcHitMask (spell-arm semantics) rather than the melee ctor.
func rangedAutoProcHitState(outcome protocol.MeleeHitOutcome, absorbed, blocked, damage uint32) (uint32, uint8) {
	var hitInfo uint32
	if absorbed > 0 {
		hitInfo |= protocol.HitInfoPartialAbsorb
		if damage == 0 {
			hitInfo |= protocol.HitInfoFullAbsorb
		}
	}
	targetState := uint8(protocol.VictimStateHit)
	if outcome == protocol.MeleeHitImmune {
		targetState = protocol.VictimStateIsImmune
	}
	return hitInfo, targetState
}

// procEventKeepsProcCharges mirrors the SPELL_ATTR6_DONT_CONSUME_PROC_CHARGES
// arm of Aura::PrepareProcToTrigger (SpellAuras.cpp:2032): when the event's
// triggering spell carries the attribute, the aura's proc charges survive
// the proc. Events without a spell (melee swings) always consume.
func procEventKeepsProcCharges(ev procEventInfo) bool {
	return ev.eventSpell != nil && ev.eventSpell.AttributesEx6&spellAttr6DontConsumeProcCharges != 0
}

// rollAuraProcChance mirrors Aura::CalcProcChance (SpellAuras.cpp:2164-2190)
// for generated entries: DBC ProcChance, the SPELLMOD_CHANCE_OF_SUCCESS
// modifier, and the over-60 level reduction. Generated entries never carry
// ProcsPerMinute, so the PPM arm is vacuous.
func (s *session) rollAuraProcChance(entry spellProcEntry, auraSpell wotlk.Spell) bool {
	chance := float64(entry.Chance)
	if s != nil {
		chance = s.applySpellModFloat(auraSpell, spellModChanceOfSuccess, chance)
		if entry.AttributesMask&procAttrReduceProc60 != 0 && s.player != nil && s.player.Level > 60 {
			chance = math.Max(0, (1-float64(s.player.Level-60)/30)*chance)
		}
	}
	return rand.Float64()*100 < chance
}

// rollCreatureProcChance mirrors Aura::CalcProcChance (SpellAuras.cpp:2164-2190)
// for the creature proc path: the DBC ProcChance roll, folded through the
// aura caster's spell-mod owner (SPELLMOD_CHANCE_OF_SUCCESS,
// SpellAuras.cpp:2181 via WorldObject::GetSpellModOwner, Object.cpp:2246-2261):
// a true pet's aura caster resolves to the owner player's session, so owner
// talents modifying the aura's proc chance apply; a player caster resolves
// to its own session; a wild creature's caster resolves to nil and the roll
// stays plain. The PPM arm is vacuous (generated entries carry no
// ProcsPerMinute) and the REDUCE_PROC_60 arm is vacuous too (generated
// entries never set the attribute).
func (s *Server) rollCreatureProcChance(mapID, instanceID uint32, casterGUID uint64, entry spellProcEntry, auraSpell wotlk.Spell) bool {
	chance := float64(entry.Chance)
	if modOwner := s.spellModOwnerSession(mapID, instanceID, casterGUID); modOwner != nil {
		chance = modOwner.applySpellModFloat(auraSpell, spellModChanceOfSuccess, chance)
	}
	return rand.Float64()*100 < chance
}

// procAuraTriggers evaluates real aura procs on a melee hit: the done-side
// half of Unit::ProcDamageAndSpellFor's aura loop (Unit.cpp:10355-10380 via
// TriggerAurasProcOnEvent). The trigger spell targets the victim.
func (s *session) procAuraTriggers(ctx context.Context, target combatTarget, attType protocol.WeaponAttackType, outcome protocol.MeleeHitOutcome, hitInfo uint32, targetState uint8, blocked, damage uint32) {
	typeMask := procFlagDoneMeleeAutoAttack | procFlagDoneMainhandAttack
	if attType == protocol.OffAttack {
		typeMask = procFlagDoneMeleeAutoAttack | procFlagDoneOffhandAttack
	}
	s.procAuraTriggerLoop(ctx, target.GUID, procEventInfo{
		typeMask:       typeMask,
		schoolMask:     spellSchoolMaskNormal,
		spellTypeMask:  procSpellTypeNone,
		spellPhaseMask: procSpellPhaseNone,
		hitMask:        meleeOutcomeProcHitMask(outcome, hitInfo, targetState, blocked),
		actorGUID:      s.playerGUID,
		damage:         damage,
	})
}

// procKillAuraTriggers evaluates the kill/death proc legs of Unit::Kill
// (Unit.cpp:11257-11272): the KILL leg on the killer/owner side and the
// KILLED/DEATH legs on the victim side. C++ passes PROC_SPELL_TYPE_MASK_ALL,
// PROC_SPELL_PHASE_NONE, PROC_HIT_NONE, no spell, and no school mask — the
// generated proc model already carries the matching early-exit for
// kill-flagged events (canSpellTriggerProcOnEvent), so the school and
// triggered-suppression gates are bypassed exactly as in C++.
// triggerTargetGUID is the kill victim for the killer's KILL event and the
// killer for the victim's KILLED event; the DEATH event targets the victim
// itself (ProcSkillsAndAuras(victim, victim, NONE, DEATH)). actorGUID follows
// the damage-path convention: the aura owner's GUID on the done side, the
// attacker's on the taken side. xpOrHonorTarget carries the
// Player::isHonorOrXPTarget verdict (Player.cpp:24064) for the event's action
// target — the C++ gate (SpellMgr.cpp:508-511) engages only for player
// actors, which the shared gate derives from actorGUID.
func (s *session) procKillAuraTriggers(ctx context.Context, triggerTargetGUID uint64, actorGUID uint64, typeMask uint32, xpOrHonorTarget bool) {
	s.procAuraTriggerLoop(ctx, triggerTargetGUID, procEventInfo{
		typeMask:        typeMask,
		schoolMask:      0,
		spellTypeMask:   procSpellTypeMaskAll,
		spellPhaseMask:  procSpellPhaseNone,
		hitMask:         procHitNone,
		xpOrHonorTarget: xpOrHonorTarget,
		actorGUID:       actorGUID,
	})
}

// procVictimAuraTriggers evaluates real aura procs on the taken side of a
// melee hit: the victim-side half of Unit::ProcDamageAndSpellFor's aura loop
// (Unit.cpp:1194-1198 — ProcVictim = PROC_FLAG_TAKEN_MELEE_AUTO_ATTACK for
// BASE_ATTACK and OFF_ATTACK, with no mainhand/offhand arm on the victim
// side; Unit.cpp:10385-10448 TriggerAurasProcOnEvent). The trigger spell
// targets the attacker.
func (s *session) procVictimAuraTriggers(ctx context.Context, attackerGUID uint64, outcome protocol.MeleeHitOutcome, hitInfo uint32, targetState uint8, blocked, damage uint32) {
	s.procAuraTriggerLoop(ctx, attackerGUID, procEventInfo{
		typeMask:       procFlagTakenMeleeAutoAttack,
		schoolMask:     spellSchoolMaskNormal,
		spellTypeMask:  procSpellTypeNone,
		spellPhaseMask: procSpellPhaseNone,
		hitMask:        meleeOutcomeProcHitMask(outcome, hitInfo, targetState, blocked),
		actorGUID:      attackerGUID,
		damage:         damage,
	})
}

// procRangedAutoAttackAuraTriggers evaluates real aura procs on the done side
// of a ranged auto attack (Auto Shot / Shoot): the spell-damage arm of
// Unit::ProcDamageAndSpellFor via Spell::TargetInfo::DoDamageAndTriggers
// (Spell.cpp:2427-2540) with m_procAttacker = DONE_RANGED_AUTO_ATTACK from
// Spell::prepareDataForTriggerSystem (Spell.cpp:2018-2034). The event carries
// the auto-shot spell and the triggered state (Spell::IsTriggered,
// Spell.cpp:7501-7504), so the CanSpellTriggerProcOnEvent mana-cost,
// spell-family, and triggered-cast gates engage exactly; the hit mask follows
// the spell arm via rangedAutoProcHitMask (IMMUNE alone on immunity, never
// the melee ctor's evade bit). Spells with SPELL_ATTR3_CANT_TRIGGER_PROC never
// reach the loop (Spell.cpp:2441). The trigger spell targets the victim.
func (s *session) procRangedAutoAttackAuraTriggers(ctx context.Context, targetGUID uint64, spellID uint32, outcome protocol.MeleeHitOutcome, absorbed, blocked, damage uint32) {
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
	hitInfo, targetState := rangedAutoProcHitState(outcome, absorbed, blocked, damage)
	s.procAuraTriggerLoop(ctx, targetGUID, procEventInfo{
		typeMask:             spellDamageProcTypeMask(spell),
		schoolMask:           schoolMask,
		spellTypeMask:        procSpellTypeDamage,
		spellPhaseMask:       procSpellPhaseHit,
		hitMask:              rangedAutoProcHitMask(outcome, hitInfo, targetState, blocked),
		triggered:            s.triggeredNoProcEvents > 0,
		eventSpell:           &spellCopy,
		triggeredByAuraSpell: s.castTriggerAuraSpell,
		actorGUID:            s.playerGUID,
		damage:               damage,
	})
}

// procRangedVictimAuraTriggers evaluates real aura procs on the taken side of
// a ranged auto attack (Spell.cpp:2020/2032 — ProcVictim =
// PROC_FLAG_TAKEN_RANGED_AUTO_ATTACK, with no mainhand/offhand arm on the
// victim side; the damage arm of DoDamageAndTriggers ORs
// PROC_FLAG_TAKEN_DAMAGE, Spell.cpp:2545). The event carries the auto-shot
// spell's school mask, matching the damage event's school on both sides of
// DoDamageAndTriggers. The trigger spell targets the attacker.
func (s *session) procRangedVictimAuraTriggers(ctx context.Context, attackerGUID uint64, spellID uint32, outcome protocol.MeleeHitOutcome, absorbed, blocked, damage uint32) {
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
	hitInfo, targetState := rangedAutoProcHitState(outcome, absorbed, blocked, damage)
	s.procAuraTriggerLoop(ctx, attackerGUID, procEventInfo{
		typeMask:       procFlagTakenRangedAutoAttack | procFlagTakenDamage,
		schoolMask:     schoolMask,
		spellTypeMask:  procSpellTypeNone,
		spellPhaseMask: procSpellPhaseNone,
		hitMask:        rangedAutoProcHitMask(outcome, hitInfo, targetState, blocked),
		actorGUID:      attackerGUID,
		damage:         damage,
	})
}

// checkEffectProc mirrors AuraEffect::CheckEffectProc
// (SpellAuraEffects.cpp:933-999): per-effect conditions that must hold for
// the effect to join the proc effect mask. AuraScript check handlers have no
// Go model and are treated as passing. SPELL_AURA_MOD_SPELL_CRIT_CHANCE's
// no-crit arm is unmodeled by design: that aura type never enters Go's proc
// pipeline (isProcTriggerAuraType), and in C++ its HandleProc does nothing
// anyway (default arm).
// checkEffectProcGates is the shared body of AuraEffect::CheckEffectProc
// (SpellAuraEffects.cpp:835-1043) behind the session and creature twins.
// extraAttacks is the aura target's banked extra-attack counter (player
// sessions bank it; creatures never do, so the creature path passes 0).
func checkEffectProcGates(srv *Server, extraAttacks uint32, aura *activeAura, eff *wotlk.SpellEffect, ev procEventInfo) bool {
	if aura == nil || eff == nil {
		return false
	}
	switch eff.Aura {
	case spellAuraModConfuse, spellAuraModFear, spellAuraModStun, spellAuraModRoot, spellAuraTransform:
		// CC auras only proc on damaging events; the aura's own damage at
		// apply time (duration untouched) never breaks it.
		if ev.damage == 0 {
			return false
		}
		if ev.eventSpell != nil && ev.eventSpell.ID == aura.SpellID && aura.RemainingMs == aura.DurationMs {
			return false
		}
	case spellAuraMechanicImmunity, spellAuraModMechanicResistance:
		// compare mechanic
		if ev.eventSpell == nil || int32(ev.eventSpell.Mechanic) != eff.MiscValue {
			return false
		}
	case spellAuraCastingSpeedNotStack:
		// skip melee hits and instant cast spells
		if ev.eventSpell == nil {
			return false
		}
		castTime := int32(0)
		if srv != nil && srv.Data != nil {
			if ct, _, err := srv.Data.SpellCastTime(ev.eventSpell.ID); err == nil {
				castTime = ct
			}
		}
		if castTime == 0 {
			return false
		}
	case spellAuraModDamageFromCaster:
		// Compare casters
		if aura.CasterGUID != ev.actorGUID {
			return false
		}
	case spellAuraModPowerCostSchool, spellAuraModPowerCostSchoolPct:
		// Skip melee hits and spells with wrong school or zero cost
		if ev.eventSpell == nil || (ev.eventSpell.ManaCost == 0 && ev.eventSpell.ManaCostPct == 0) ||
			ev.eventSpell.SchoolMask&uint32(eff.MiscValue) == 0 {
			return false
		}
	case spellAuraReflectSpellsSchool:
		// Skip melee hits and spells with wrong school
		if ev.eventSpell == nil || ev.eventSpell.SchoolMask&uint32(eff.MiscValue) == 0 {
			return false
		}
	case spellAuraModSpellCritChance:
		// Skip spells that can't crit (SpellAuraEffects.cpp:1023-1026);
		// melee events carry no spell, so they never pass this arm.
		if ev.eventSpell == nil || ev.eventSpell.Attributes&spellAttr0CuCanCrit == 0 {
			return false
		}
	case spellAuraProcTriggerSpell, spellAuraProcTriggerSpellWithValue:
		// C++ AuraEffect::CheckEffectProc (SpellAuraEffects.cpp:978-984):
		// don't proc extra attacks while the aura target is already
		// processing an extra-attack spell (Unit::m_extraAttacks non-zero).
		// The session loop evaluates the session player's own auras, so it
		// passes the player's banked counter; the creature path passes 0 —
		// creatures never bank extra swings, matching a zero bank.
		if eff.TriggerSpell != 0 && extraAttacks != 0 && srv != nil && srv.Data != nil {
			if trig, found, err := srv.Data.Spell(eff.TriggerSpell); err == nil && found &&
				spellHasEffect(trig, spellEffectAddExtraAttacks) {
				return false
			}
		}
	default:
	}
	return true
}

// checkEffectProc delegates the session's AuraEffect::CheckEffectProc gate
// to the shared body with the player's banked extra-attack counter.
func (s *session) checkEffectProc(aura *activeAura, eff *wotlk.SpellEffect, ev procEventInfo) bool {
	var extraAttacks uint32
	var srv *Server
	if s != nil {
		extraAttacks = s.extraAttacks
		srv = s.server
	}
	return checkEffectProcGates(srv, extraAttacks, aura, eff, ev)
}

// checkCreatureEffectProc is the creature-proc twin of
// session.checkEffectProc (AuraEffect::CheckEffectProc,
// SpellAuraEffects.cpp:835-1043): the shared gates with a zero
// extra-attack bank, since the Go creature path has no extra-swing model.
func (s *Server) checkCreatureEffectProc(aura *activeAura, eff *wotlk.SpellEffect, ev procEventInfo) bool {
	return checkEffectProcGates(s, 0, aura, eff, ev)
}

// procAuraTriggerLoop runs one aura-proc pass over the player's active auras:
// each aura with a generated spell_proc entry runs the
// CanSpellTriggerProcOnEvent gate against the event; on pass, the chance roll
// fires each eligible aura effect's trigger spell on triggerTargetGUID
// (Aura::TriggerProcOnEvent, SpellAuras.cpp:2192-2212, calls
// AuraEffect::HandleProc per effect in the proc effect mask,
// SpellAuraEffects.cpp:1010-1043). The modOwner arm of
// Unit::TriggerAurasProcOnEvent (Unit.cpp:10395-10408) — "needed for example
// for Cobra Strikes, pet does the attack, but aura is on owner" — runs as
// procAuraTriggerLoopFiltered from the pet spell proc sites
// (firePetModOwnerAuraProcs, pet_combat.go): only the owner's auras
// registered on the triggering cast's applied-mods registry join the
// triggering set, evaluated against the same event. Pet melee/kill events
// carry no spell, so the arm's spell != nil gate keeps them out there.
func (s *session) procAuraTriggerLoop(ctx context.Context, triggerTargetGUID uint64, ev procEventInfo) {
	s.procAuraTriggerLoopFiltered(ctx, triggerTargetGUID, ev, nil)
}

// procAuraTriggerLoopFiltered is procAuraTriggerLoop over a filtered aura
// set: a nil filter evaluates every active aura, a non-nil filter keeps
// only the auras it accepts (the mod-owner arm's registry join).
func (s *session) procAuraTriggerLoopFiltered(ctx context.Context, triggerTargetGUID uint64, ev procEventInfo, filter func(*activeAura) bool) {
	if s == nil || s.player == nil || len(s.activeAuras) == 0 {
		return
	}
	// Unit::TriggerAurasProcOnEvent (Unit.cpp:10424-10428): when the
	// triggering spell disallows proc events (Spell::IsProcDisabled —
	// every Go triggered direct cast carries
	// TRIGGERED_DISALLOW_PROC_EVENTS, SpellDefines.h:146), the evaluated
	// unit cannot proc again for the whole evaluation (Unit::CanProc,
	// Unit.h:1609) — the same-unit recursion guard Spell::DoDamageAndTriggers
	// consults per hit target (Spell.cpp:2441).
	if ev.triggered {
		s.procDeep++
		defer func() { s.procDeep-- }()
	}
	auras := make([]*activeAura, 0, len(s.activeAuras))
	for _, aura := range s.activeAuras {
		if filter != nil && !filter(aura) {
			continue
		}
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
		// Aura::GetProcEffectMask (SpellAuras.cpp:2055-2057): an aura never
		// procs from a spell its own trigger fired
		// (Spell::IsTriggeredByAura, Spell.h:544) — the recursion
		// terminator for triggered-proc chains. Placed before the charge
		// check, matching C++ order (PrepareProcToTrigger runs only after
		// GetProcEffectMask passes).
		if ev.eventSpell != nil && ev.triggeredByAuraSpell != 0 && ev.triggeredByAuraSpell == auraSpell.ID {
			continue
		}
		if entry.Charges > 0 && aura.RemainingCharges == 0 {
			continue
		}
		// C++ Aura::GetProcEffectMask (SpellAuras.cpp:2089-2091): a proc on
		// cooldown (Aura::IsProcOnCooldown, SpellAuras.cpp:2010-2013,
		// m_procCooldown > now) gates before the chance roll, per aura —
		// "cooldowns should be added to the whole aura" (SpellAuras.cpp:2042).
		if time.Now().Before(aura.ProcCooldownUntil) {
			continue
		}
		// C++ Aura::GetProcEffectMask (SpellAuras.cpp:2082-2086): a
		// charge-using aura with PROC_ATTR_REQ_SPELLMOD only procs when the
		// triggering spell's applied-mods registry holds the aura. The Go
		// registry is the innermost live taking context of the cast whose
		// phase is firing this event (each Go cast opens its own context,
		// matching the per-Spell m_appliedMods; nested triggered casts see
		// their own). With no proc spell or no live window the check fails,
		// matching an empty m_appliedMods. Documented delta: delayed-phase
		// events run in the arrival closure's fresh registry, so a
		// REQ_SPELLMOD aura whose mods were applied at cast time does not
		// fire there — C++ keeps the same Spell object across the phases.
		if entry.AttributesMask&procAttrReqSpellmod != 0 {
			if ev.eventSpell == nil {
				continue
			}
			if !s.spellModTakingApplied(s.spellModTakingCurrent(), aura) {
				continue
			}
		}
		if !canSpellTriggerProcOnEvent(entry, ev, auraSpell.AttributesEx3&spellAttr3CanProcWithTriggered != 0) {
			continue
		}
		// spell_dk_advantage_t10_4p::CheckProc (spell_dk.cpp:209-252):
		// the T10 4P gate runs before the chance roll, like the C++
		// CheckProc in GetProcEffectMask.
		if auraSpell.ID == 70656 && !s.advantageT10ProcAllowed(ev) {
			continue
		}
		// spell_dk_hungering_cold::CheckProc (spell_dk.cpp:1371-1379):
		// the disease-dispel gate runs before the chance roll, like the
		// C++ CheckProc in GetProcEffectMask (70656 precedent above).
		if auraSpell.ID == 51209 && !s.hungeringColdProcAllowed(ev) {
			continue
		}
		// spell_dk_improved_blood_presence_triggered::CheckProc
		// (spell_dk.cpp:1512-1517): only a player actor procs the
		// heal; a creature or pet dealing the damage never fires it.
		if auraSpell.ID == 63611 && !procActorIsPlayerGUID(ev.actorGUID) {
			continue
		}
		// spell_hun_glyph_of_arcane_shot::CheckProc
		// (spell_hunter.cpp:430-451): the glyph only procs when the
		// proc target carries a sting cast by the holder (Serpent,
		// Viper, Scorpid, Wyvern: hunter family, flags 0xC000/0x1080).
		// The gate runs before the chance roll, like the C++ CheckProc
		// in GetProcEffectMask (70656 precedent above).
		if auraSpell.ID == 56841 && !s.glyphOfArcaneShotProcAllowed(triggerTargetGUID, s.playerGUID) {
			continue
		}
		// spell_hun_roar_of_sacrifice::CheckProc
		// (spell_hunter.cpp:1279-1292): the proc only fires on damage
		// events whose school matches the EFFECT_1 MiscValue mask, with
		// a live aura caster. The damage-info null leg rides the
		// ev.damage == 0 proxy (the CC-aura gate in
		// checkEffectProcGates uses the same proxy); the school mask is
		// the proc event's, the MiscValue the DBC effect-1 value.
		// Placed before the chance roll, like the C++ CheckProc in
		// GetProcEffectMask (70656 precedent above).
		if auraSpell.ID == 53480 && !s.roarOfSacrificeProcAllowed(aura, auraSpell, ev) {
			continue
		}
		// spell_hun_t9_4p_bonus::CheckProc (spell_hunter.cpp:1591-1596):
		// the T9 4P bonus only procs when the actor is a player with a
		// pet out. The gate runs before the chance roll, like the C++
		// CheckProc in GetProcEffectMask (70656 precedent above).
		if auraSpell.ID == 67151 && !(procActorIsPlayerGUID(ev.actorGUID) && s.player != nil && s.player.PetGUID != 0) {
			continue
		}
		// spell_item_zandalarian_charm::CheckProc
		// (spell_item.cpp:4124-4131): the charm only procs on a
		// triggering spell other than itself (24658); a proc event
		// with no spell info never procs. The gate runs before the
		// chance roll, like the C++ CheckProc in GetProcEffectMask
		// (70656 precedent above).
		if auraSpell.ID == 24658 && (ev.eventSpell == nil || ev.eventSpell.ID == 24658) {
			continue
		}
		// spell_item_zandalarian_charm::CheckProc, Restless Strength
		// twin (spell_item.cpp:4124-4131, registered
		// spell_item.cpp:4414 with stack aura 24662): the charm only
		// procs on a triggering spell other than itself (24661); a
		// proc event with no spell info never procs.
		if auraSpell.ID == 24661 && (ev.eventSpell == nil || ev.eventSpell.ID == 24661) {
			continue
		}
		// spell_mage_burning_determination::CheckProc
		// (spell_mage.cpp:272-281): the aura only procs when the
		// triggering spell carries an interrupt or silence mechanic.
		// The gate runs before the chance roll, like the C++
		// CheckProc in GetProcEffectMask (70656 precedent above).
		// Both ranks share the loader (spell_mage.cpp:261, 1486).
		// DORMANT until Spell.dbc ships the 54747/54748 proc entries.
		if (auraSpell.ID == 54747 || auraSpell.ID == 54748) && !s.burningDeterminationProcAllowed(ev) {
			continue
		}
		// spell_mage_combustion::CheckProc (spell_mage.cpp:395-409):
		// a non-critical hit casts the Combustion proc aura (28682)
		// on the actor, triggered, and takes no charge; a critical
		// hit lets the default proc action run. The gate runs before
		// the chance roll, like the C++ CheckProc in
		// GetProcEffectMask (70656 precedent above). DORMANT until
		// Spell.dbc ships the 11129 proc entry.
		if auraSpell.ID == 11129 && !s.combustionCheckProc(ctx, ev) {
			continue
		}
		// spell_mage_focus_magic::CheckProc (spell_mage.cpp:737-741):
		// the proc only fires while the aura's caster (the mage) is
		// alive. The gate runs before the chance roll, like the C++
		// CheckProc in GetProcEffectMask (70656 precedent above).
		// DORMANT until Spell.dbc ships the 54646 proc entry.
		if auraSpell.ID == 54646 && !s.focusMagicProcAllowed(aura) {
			continue
		}
		// spell_mage_gen_extra_effects::CheckProc
		// (spell_mage.cpp:784-799): no self-target proc events (the
		// Arcane Missiles double-proc guard) and the T8 4P bonus roll
		// suppresses the proc. The gate runs before the chance roll,
		// like the C++ CheckProc in GetProcEffectMask (70656 precedent
		// above). DORMANT until Spell.dbc ships the 48108 proc entry.
		if auraSpell.ID == 48108 && !s.genExtraEffectsProcAllowed(ev, triggerTargetGUID) {
			continue
		}
		// C++ GetProcEffectMask/TriggerProcOnEvent (SpellAuras.cpp:2045-2129,
		// 2192-2212): the proc effect mask can hold several effects and every
		// one fires its own trigger spell — collect all eligible effects,
		// don't stop at the first.
		eligible := make([]int, 0, len(auraSpell.Effects))
		for i := range auraSpell.Effects {
			eff := &auraSpell.Effects[i]
			if eff.Effect == 0 || !isProcTriggerAuraType(eff.Aura) {
				continue
			}
			if !s.checkEffectProc(aura, eff, ev) {
				continue
			}
			eligible = append(eligible, i)
		}
		if len(eligible) == 0 {
			continue
		}
		if !s.rollAuraProcChance(entry, auraSpell) {
			continue
		}
		// C++ PrepareProcToTrigger (SpellAuras.cpp:2032): a charge is taken
		// only when the triggering spell is absent (melee swings carry no
		// spell info) or lacks SPELL_ATTR6_DONT_CONSUME_PROC_CHARGES.
		chargesDepleted := false
		if entry.Charges > 0 && !procEventKeepsProcCharges(ev) {
			aura.RemainingCharges--
			chargesDepleted = aura.RemainingCharges == 0
		}
		// C++ Aura::PrepareProcToTrigger (SpellAuras.cpp:2042):
		// AddProcCooldown(now + procEntry->Cooldown) — set only after the
		// chance roll succeeded (GetProcEffectMask returned nonzero).
		if entry.CooldownMs > 0 {
			aura.ProcCooldownUntil = time.Now().Add(time.Duration(entry.CooldownMs) * time.Millisecond)
		}
		// Unit::TriggerAurasProcOnEvent (Unit.cpp:10433-10440): an aura
		// carrying SPELL_ATTR3_DISABLE_PROC elevates the no-proc guard
		// around its own trigger — a spell it casts cannot proc the same
		// unit's auras again (Unit::CanProc, Unit.h:1609).
		disableProc := auraSpell.AttributesEx3&spellAttr3DisableProc != 0
		if disableProc {
			s.procDeep++
		}
		for _, i := range eligible {
			eff := &auraSpell.Effects[i]
			// AuraScript OnEffectProc (AuraEventOnProc, lua_aura.go):
			// fires per eligible effect after the gates and the charge
			// consume, before the default leg; a true return is
			// PreventDefaultAction and skips that leg for this effect.
			// The HasHook gate keeps the aura/actor object builds out
			// of the hot path when no handler is registered.
			if s.server.hasAuraProcHook(aura.SpellID) {
				auraObj := (&luaAuraRef{server: s.server, spellID: aura.SpellID, player: s, mapID: s.player.Map, instanceID: s.player.InstanceID}).luaAuraObject()
				if s.server.fireAuraProc(ctx, aura, auraObj, s.luaPlayer(), s.server.luaUnitObjectForGUID(s.player.Map, s.player.InstanceID, ev.actorGUID), s.server.luaUnitObjectForGUID(s.player.Map, s.player.InstanceID, triggerTargetGUID), ev.damage) {
					continue
				}
			}
			if auraSpell.ID == 49028 && i == 1 && eff.Aura == spellAuraDummy {
				// spell_dk_dancing_rune_weapon::HandleProc
				// (spell_dk.cpp:763-793): PreventDefaultAction; the
				// caster's controlled Dancing Rune Weapon (27893)
				// mirrors half the triggering spell's damage onto its
				// own victim — SendSpellNonMeleeDamageLog with the
				// triggering spell's id/school, then DealDamage
				// (SPELL_DIRECT_DAMAGE, triggered=true). No Lua ev6
				// port: ev6 args carry no proc spell id/school, Lua
				// has no controlled-unit enumeration, and the Lua
				// DealDamage spell leg deals 0 by design
				// (lua_unit_combat.go) — so this Go twin is the only
				// leg, not a Lua-off fallback. DORMANT until Spell.dbc
				// ships the 49028 proc entry; the DRW itself rides the
				// guardian summon funnel (owner faction stamped,
				// 27893 display arm stays out per boss_ai.go).
				if ev.eventSpell == nil || ev.damage == 0 {
					continue
				}
				amount := ev.damage / 2
				if amount == 0 {
					continue
				}
				s.server.motionMu.Lock()
				var drw *creatureMotion
				for _, m := range s.server.motionMapLocked(s.player.Map, s.player.InstanceID) {
					if m != nil && m.Entry == 27893 && m.OwnerGUID == s.playerGUID {
						drw = m
						break
					}
				}
				s.server.motionMu.Unlock()
				if drw == nil {
					continue
				}
				s.server.motionMu.Lock()
				victimGUID, dx, dy, dz := drw.TargetGUID, drw.X, drw.Y, drw.Z
				s.server.motionMu.Unlock()
				if victimGUID == 0 {
					continue
				}
				spellID := ev.eventSpell.ID
				schoolMask := uint32(ev.eventSpell.SchoolMask)
				if ts := s.server.findSessionByGUID(victimGUID); ts != nil && ts.player != nil {
					var overkill uint32
					if victimHealth := ts.player.Health; amount >= victimHealth && victimHealth > 0 {
						overkill = amount - victimHealth
					}
					logPkt := buildSpellNonMeleeDamageLog(victimGUID, drw.GUID, spellID, amount, overkill, uint8(schoolMask), 0, 0, 0, 0)
					_ = ts.write(uint16(protocol.OpcodeSMSG_SPELLNONMELEEDAMAGELOG), logPkt, true)
					s.server.broadcastToVisibleSet(uint16(protocol.OpcodeSMSG_SPELLNONMELEEDAMAGELOG), logPkt, s.player.Map, s.player.InstanceID, dx, dy, dz, ts)
					s.server.dealDirectDamageToPlayer(ctx, drw.GUID, nil, ts, spellID, schoolMask, amount)
				} else if vm := s.server.findCreatureMotion(s.player.Map, s.player.InstanceID, victimGUID); vm != nil {
					s.server.motionMu.Lock()
					victimHealth := vm.Health
					s.server.motionMu.Unlock()
					var overkill uint32
					if amount >= victimHealth && victimHealth > 0 {
						overkill = amount - victimHealth
					}
					logPkt := buildSpellNonMeleeDamageLog(victimGUID, drw.GUID, spellID, amount, overkill, uint8(schoolMask), 0, 0, 0, 0)
					s.server.broadcastToVisibleSet(uint16(protocol.OpcodeSMSG_SPELLNONMELEEDAMAGELOG), logPkt, s.player.Map, s.player.InstanceID, dx, dy, dz, nil)
					s.server.dealDirectDamageToCreature(ctx, drw.GUID, nil, vm, creatureAuraKeyForMotion(vm), spellID, schoolMask, true, amount)
				}
				continue
			}
			if auraSpell.ID == 62259 && i == 0 && eff.Aura == spellAuraProcTriggerSpell {
				// spell_dk_glyph_of_death_grip::HandleProc
				// (spell_dk.cpp:1234-1238): PreventDefaultAction; the
				// glyph resets the cooldown of Death Grip Initial
				// (49576). No Lua ev6 twin: spell_dk.lua holds no 62259
				// handler, and the Lua ev6 veto above would fire first
				// if one ever registers. DORMANT until Spell.dbc ships
				// the 62259 proc entry.
				s.resetPlayerCooldown(49576, true)
				continue
			}
			if auraSpell.ID == 63611 && i == 1 && eff.Aura == spellAuraProcTriggerSpell {
				// spell_dk_improved_blood_presence_triggered::HandleProc
				// (spell_dk.cpp:1519-1529): PreventDefaultAction; the
				// actor casts Improved Blood Presence Heal (50475) on
				// itself with BP0 = pct of the triggering damage by the
				// effect amount. No damage gate in C++ (a null
				// DamageInfo skips the cast; Go cannot distinguish a
				// null info from zero damage, so a zero-damage event
				// still fires with BP0 0). No Lua ev6 twin: spell_dk.lua
				// holds no 63611 handler (it only casts 63611 from the
				// blood-presence apply leg), and the Lua ev6 veto above
				// would fire first if one ever registers. DORMANT until
				// Spell.dbc ships the 63611 proc entry.
				s.server.fireScriptProcCast(ctx, s.player.Map, s.player.InstanceID, scriptProcCast{spellID: 50475, casterGUID: ev.actorGUID, targetGUID: ev.actorGUID, bp0: uint32(uint64(ev.damage) * uint64(scriptProcEffAmount(aura, i)) / 100), bp0Set: true})
				continue
			}
			if auraSpell.ID == 58642 && i == 0 && eff.Aura == spellAuraDummy {
				// spell_dk_glyph_of_scourge_strike::HandleProc
				// (spell_dk.cpp:1260-1270): PreventDefaultAction; the
				// actor casts the script spell (69961) on the proc
				// target, triggered with the aura effect. No Lua ev6
				// twin: spell_dk.lua holds no 58642 handler, and the
				// Lua ev6 veto above would fire first if one ever
				// registers. DORMANT until Spell.dbc ships the 58642
				// proc entry.
				if triggerTargetGUID == 0 {
					continue
				}
				s.server.fireScriptProcCast(ctx, s.player.Map, s.player.InstanceID, scriptProcCast{spellID: 69961, casterGUID: ev.actorGUID, targetGUID: triggerTargetGUID})
				continue
			}
			if auraSpell.ID == 50240 && i == 2 && eff.Aura == spellAuraProcTriggerSpell {
				// spell_oculus_evasive_maneuvers::HandleProc
				// (oculus.cpp:488-493): PreventDefaultAction; the holder
				// loses one Ruby Evasive Charges (50241) stack, and the
				// maneuvers aura itself goes when the last charge is
				// gone. The player-side twin of the creature-loop arm:
				// Go runs vehicle-bar casts as the player (the
				// vehicle-caster delta, spells.go), so a self-targeted
				// 50240 lands on the rider instead of the drake. Placed
				// before the generic arm, which would cast the DBC
				// trigger instead. Lua-off fallback twin of the
				// oculus.lua event-6 handler: the Lua hook vetoes first
				// when loaded, so this never double-fires.
				if s.removePlayerAuraFromStack(50241) {
					s.removeAura(auraSpell.ID)
				}
				continue
			}
			if auraSpell.ID == 50421 && i == 0 && eff.Aura == spellAuraProcTriggerSpell {
				// spell_dk_scent_of_blood::HandleProc (spell_dk.cpp:2205-2210):
				// PreventDefaultAction; the holder casts Scent of Blood
				// (50422) on itself and loses one stack of the talent
				// aura. Placed before the generic arm, which would cast
				// the DBC trigger on the proc target instead. The cast
				// carries the triggering-aura attribution (the C++ aurEff
				// overload), mirroring the generic leg below. No Lua ev6
				// twin: spell_dk.lua holds no 50421 handler, and the Lua
				// ev6 veto above would fire first if one ever registers.
				// DORMANT until Spell.dbc ships the 50421 proc entry.
				s.triggeredByAuraCast = true
				s.triggeredByAuraSpell = auraSpell.ID
				s.castSpellDirect(ctx, 50422, s.playerGUID)
				s.triggeredByAuraCast = false
				s.triggeredByAuraSpell = 0
				_ = s.removePlayerAuraFromStack(50421)
				continue
			}
			if auraSpell.ID == 23552 && i == 0 && eff.Aura == spellAuraProcTriggerSpell {
				// spell_sha_item_lightning_shield_trigger::HandleProc
				// (spell_shaman.cpp:1302-1311): PreventDefaultAction; the
				// holder casts Lightning Shield T2 damage (27635) on
				// itself. Placed before the generic arm, which would cast
				// the DBC trigger on the proc target instead. The cast
				// carries the triggering-aura attribution (the C++ aurEff
				// overload), mirroring the generic leg below (50421
				// precedent). No Lua ev6 twin: spell_shaman.lua holds no
				// 23552 handler. DORMANT until Spell.dbc ships the 23552
				// proc entry.
				s.triggeredByAuraCast = true
				s.triggeredByAuraSpell = auraSpell.ID
				s.castSpellDirect(ctx, 27635, s.playerGUID)
				s.triggeredByAuraCast = false
				s.triggeredByAuraSpell = 0
				continue
			}
			if auraSpell.ID == 23572 && i == 0 && eff.Aura == spellAuraProcTriggerSpell {
				// spell_sha_item_mana_surge::HandleProc
				// (spell_shaman.cpp:1336-1355): PreventDefaultAction; the
				// holder casts Mana Surge (23571) on itself with BP0 =
				// 35% of the triggering spell's power cost. Placed before
				// the generic arm, which would cast the DBC trigger on the
				// proc target with no base points. The cast carries the
				// triggering-aura attribution (the C++ aurEff overload),
				// mirroring the generic leg below (56841 precedent for the
				// power-cost read). No Lua ev6 twin: spell_shaman.lua holds
				// no 23572 handler. DORMANT until Spell.dbc ships the 23572
				// proc entry.
				if ev.eventSpell == nil {
					continue
				}
				mana := int64(s.calculateSpellPowerCost(*ev.eventSpell)) * 35 / 100
				if mana < 0 {
					mana = 0
				}
				s.triggeredByAuraCast = true
				s.triggeredByAuraSpell = auraSpell.ID
				s.castSpellDirectWithBasePoint(ctx, 23571, s.playerGUID, uint32(mana))
				s.triggeredByAuraCast = false
				s.triggeredByAuraSpell = 0
				continue
			}
			if auraSpell.ID == 30823 && i == 0 && eff.Aura == spellAuraProcTriggerSpell {
				// spell_sha_shamanistic_rage::HandleProc
				// (spell_shaman.cpp:1826-1834): PreventDefaultAction;
				// the holder casts Shamanistic Rage Proc (30824) on
				// itself with BP0 = pct of its total base attack
				// power by the effect amount (CastSpellExtraArgs
				// (aurEff)). Placed before the generic arm, which
				// would cast the DBC trigger on the proc target with
				// no base points. The cast carries the
				// triggering-aura attribution (the C++ aurEff
				// overload), mirroring the generic leg below (23572
				// precedent). No Lua ev6 twin: spell_shaman.lua holds
				// no 30823 handler — the census lua_hit is a comment
				// mention only. DORMANT until Spell.dbc ships the
				// 30823 proc entry.
				amount := int64(int32(s.totalAttackPowerValue(false))) * int64(scriptProcEffAmount(aura, i)) / 100
				if amount < 0 {
					amount = 0
				}
				s.triggeredByAuraCast = true
				s.triggeredByAuraSpell = auraSpell.ID
				s.castSpellDirectWithBasePoint(ctx, 30824, s.playerGUID, uint32(amount))
				s.triggeredByAuraCast = false
				s.triggeredByAuraSpell = 0
				continue
			}
			if auraSpell.ID == 55198 && i == 0 && eff.Aura == spellAuraDummy {
				// spell_sha_tidal_force_dummy::HandleProc
				// (spell_shaman.cpp:1988-1993): PreventDefaultAction;
				// the proc actor loses one stack of Tidal Force Crit
				// (55166). The actor is the holder for this aura's
				// healing-spell procs, so the player-side stack drop
				// is the C++ RemoveAuraFromStack(55166) (74396
				// precedent). No Lua ev6 twin: ev6 carries no
				// RemoveAuraFromStack bridge. DORMANT until Spell.dbc
				// ships the 55198 proc entry.
				_ = s.removePlayerAuraFromStack(55166)
				continue
			}
			if auraSpell.ID == 67151 && i == 0 && eff.Aura == spellAuraProcTriggerSpell {
				// spell_hun_t9_4p_bonus::HandleProc
				// (spell_hunter.cpp:1598-1607): PreventDefaultAction; the
				// actor casts T9 4P Greatness (68130) on its pet with the
				// aura effect. Placed before the generic arm, which would
				// cast the DBC trigger on the proc target instead. The
				// cast carries the triggering-aura attribution (the C++
				// aurEff overload), mirroring the generic leg below
				// (50421 precedent). No Lua ev6 twin: the cast target is
				// the pet, and Lua has no player-side pet-object bridge
				// (only GetPetGUID, lua_player.go:1466; a GUID does not
				// survive the Lua float64 round-trip, lua_spell.go).
				// DORMANT until Spell.dbc ships the 67151 proc entry.
				s.triggeredByAuraCast = true
				s.triggeredByAuraSpell = auraSpell.ID
				s.server.fireScriptProcCast(ctx, s.player.Map, s.player.InstanceID, scriptProcCast{spellID: 68130, casterGUID: ev.actorGUID, targetGUID: s.player.PetGUID})
				s.triggeredByAuraCast = false
				s.triggeredByAuraSpell = 0
				continue
			}
			if (auraSpell.ID == 71519 || auraSpell.ID == 71562) && i == 0 && eff.Aura == spellAuraDummy {
				// spell_item_deathbringers_will::HandleProc
				// (spell_item.cpp:586-612): PreventDefaultAction; the
				// actor casts one of three stat buffs on itself with
				// the aura effect, picked uniformly by class — the
				// normal (71519) and heroic (71562) trigger sets
				// differ. Priest/mage/warlock tables are empty in C++
				// (no cast). Placed before the generic arm, which would
				// cast the DBC trigger on the proc target instead. The
				// cast carries the triggering-aura attribution (the C++
				// aurEff overload), mirroring the generic leg below
				// (67151 precedent). The census go_hit is a false
				// positive: 71562 in-tree is the Ashen Band proc
				// constant (procs.go:125), not this trinket. DORMANT
				// until Spell.dbc ships the 71519/71562 proc entries.
				if s.player == nil {
					continue
				}
				var pool []uint32
				if auraSpell.ID == 71562 {
					// Heroic trigger set (spell_item.cpp:550-554).
					switch s.player.Class {
					case 1, 2, 6: // CLASS_WARRIOR, CLASS_PALADIN, CLASS_DEATH_KNIGHT
						pool = []uint32{71561, 71559, 71560}
					case 3: // CLASS_HUNTER
						pool = []uint32{71556, 71559, 71558}
					case 4, 7: // CLASS_ROGUE, CLASS_SHAMAN
						pool = []uint32{71556, 71560, 71558}
					case 11: // CLASS_DRUID
						pool = []uint32{71561, 71556, 71560}
					}
				} else {
					// Normal trigger set (spell_item.cpp:544-548).
					switch s.player.Class {
					case 1, 2, 6: // CLASS_WARRIOR, CLASS_PALADIN, CLASS_DEATH_KNIGHT
						pool = []uint32{71484, 71491, 71492}
					case 3: // CLASS_HUNTER
						pool = []uint32{71485, 71491, 71486}
					case 4, 7: // CLASS_ROGUE, CLASS_SHAMAN
						pool = []uint32{71485, 71492, 71486}
					case 11: // CLASS_DRUID
						pool = []uint32{71484, 71485, 71492}
					}
				}
				if len(pool) > 0 {
					s.triggeredByAuraCast = true
					s.triggeredByAuraSpell = auraSpell.ID
					s.server.fireScriptProcCast(ctx, s.player.Map, s.player.InstanceID, scriptProcCast{spellID: pool[rand.Intn(len(pool))], casterGUID: ev.actorGUID, targetGUID: ev.actorGUID})
					s.triggeredByAuraCast = false
					s.triggeredByAuraSpell = 0
				}
				continue
			}
			if auraSpell.ID == 69739 && i == 0 && eff.Aura == spellAuraDummy {
				// spell_item_shard_of_the_scale::HandleProc
				// (spell_item.cpp:3444-3456): PreventDefaultAction; a
				// DONE_SPELL_MAGIC_DMG_CLASS_POS event casts the heal
				// 69734 on the proc target, a NEG event the damage
				// 69730, both with the aura effect. The two legs are
				// independent in C++ (two ifs), so both fire if both
				// flags are set. No Lua ev6 twin: ev6 args carry no
				// proc type mask. DORMANT until Spell.dbc ships the
				// 69739 proc entry.
				if ev.typeMask&procFlagDoneSpellMagicDmgClassPos != 0 {
					s.triggeredByAuraCast = true
					s.triggeredByAuraSpell = auraSpell.ID
					s.server.fireScriptProcCast(ctx, s.player.Map, s.player.InstanceID, scriptProcCast{spellID: 69734, casterGUID: ev.actorGUID, targetGUID: triggerTargetGUID})
					s.triggeredByAuraCast = false
					s.triggeredByAuraSpell = 0
				}
				if ev.typeMask&procFlagDoneSpellMagicDmgClassNeg != 0 {
					s.triggeredByAuraCast = true
					s.triggeredByAuraSpell = auraSpell.ID
					s.server.fireScriptProcCast(ctx, s.player.Map, s.player.InstanceID, scriptProcCast{spellID: 69730, casterGUID: ev.actorGUID, targetGUID: triggerTargetGUID})
					s.triggeredByAuraCast = false
					s.triggeredByAuraSpell = 0
				}
				continue
			}
			if auraSpell.ID == 69755 && i == 0 && eff.Aura == spellAuraDummy {
				// spell_item_shard_of_the_scale::HandleProc, purified
				// rank (spell_item.cpp:3444-3456, template args
				// SPELL_PURIFIED_CAUTERIZING_HEAL 69733 /
				// SPELL_PURIFIED_SEARING_FLAMES 69729): same POS/NEG
				// split as the shiny 69739 arm above — a
				// DONE_SPELL_MAGIC_DMG_CLASS_POS event casts the heal
				// 69733 on the proc target, a NEG event the damage
				// 69729, both with the aura effect, independent legs.
				// No Lua ev6 twin: ev6 args carry no proc type mask.
				// DORMANT until Spell.dbc ships the 69755 proc entry.
				if ev.typeMask&procFlagDoneSpellMagicDmgClassPos != 0 {
					s.triggeredByAuraCast = true
					s.triggeredByAuraSpell = auraSpell.ID
					s.server.fireScriptProcCast(ctx, s.player.Map, s.player.InstanceID, scriptProcCast{spellID: 69733, casterGUID: ev.actorGUID, targetGUID: triggerTargetGUID})
					s.triggeredByAuraCast = false
					s.triggeredByAuraSpell = 0
				}
				if ev.typeMask&procFlagDoneSpellMagicDmgClassNeg != 0 {
					s.triggeredByAuraCast = true
					s.triggeredByAuraSpell = auraSpell.ID
					s.server.fireScriptProcCast(ctx, s.player.Map, s.player.InstanceID, scriptProcCast{spellID: 69729, casterGUID: ev.actorGUID, targetGUID: triggerTargetGUID})
					s.triggeredByAuraCast = false
					s.triggeredByAuraSpell = 0
				}
				continue
			}
			if auraSpell.ID == 24658 && i == 0 && eff.Aura == spellAuraDummy {
				// spell_item_zandalarian_charm::HandleStackDrop
				// (spell_item.cpp:4133-4137): PreventDefaultAction;
				// the holder loses one stack of the Unstable Power
				// stack aura 24659. No Lua ev6 twin: ev6 args carry
				// no triggering spell id, so the CheckProc gate is
				// unimplementable in Lua. DORMANT until Spell.dbc
				// ships the 24658 proc entry.
				_ = s.removePlayerAuraFromStack(24659)
				continue
			}
			if auraSpell.ID == 24661 && i == 0 && eff.Aura == spellAuraDummy {
				// spell_item_zandalarian_charm::HandleStackDrop,
				// Restless Strength twin (spell_item.cpp:4133-4137):
				// PreventDefaultAction; the holder loses one stack of
				// the Restless Strength stack aura 24662. No Lua ev6
				// twin: ev6 args carry no triggering spell id, so the
				// CheckProc gate is unimplementable in Lua. DORMANT
				// until Spell.dbc ships the 24661 proc entry.
				_ = s.removePlayerAuraFromStack(24662)
				continue
			}
			if (auraSpell.ID == 37447 || auraSpell.ID == 61062) && i == 1 && eff.Aura == spellAuraOverrideClassScripts {
				// spell_mage_imp_mana_gems::HandleProc
				// (spell_mage.cpp:515-526): PreventDefaultAction; the
				// proc actor casts Mana Surge (37445) on nil (self),
				// triggered by the aura effect. Both ranks share the
				// loader (spell_mage.cpp:505-506). No Lua ev6 twin:
				// the arm rides the aura-triggered cast the generic
				// path cannot express. DORMANT until Spell.dbc ships
				// the 37447/61062 proc entries.
				if ev.actorGUID != 0 {
					s.triggeredByAuraCast = true
					s.triggeredByAuraSpell = auraSpell.ID
					s.server.fireScriptProcCast(ctx, s.player.Map, s.player.InstanceID, scriptProcCast{spellID: 37445, casterGUID: ev.actorGUID, targetGUID: ev.actorGUID})
					s.triggeredByAuraCast = false
					s.triggeredByAuraSpell = 0
				}
				continue
			}
			if auraSpell.ID == 74396 && i == 0 && eff.Aura == spellAuraDummy {
				// spell_mage_fingers_of_frost::HandleDummy
				// (spell_mage.cpp:600-604): PreventDefaultAction; the
				// proc actor loses one stack of Fingers of Frost
				// (74396). The actor is the holder for this aura's
				// own-crit procs, so the player-side stack drop is
				// the C++ RemoveAuraFromStack(GetId()). The mode-3
				// OnRemove leg (drop 44544) stays the Lua leg in
				// spell_mage.lua. No Lua ev6 twin: ev6 carries no
				// RemoveAuraFromStack bridge. DORMANT until Spell.dbc
				// ships the 74396 proc entry.
				_ = s.removePlayerAuraFromStack(74396)
				continue
			}
			if auraSpell.ID == 54646 && i == 0 && eff.Aura == spellAuraModSpellCritChance {
				// spell_mage_focus_magic::HandleProc
				// (spell_mage.cpp:743-747): PreventDefaultAction; the
				// holder casts Focus Magic Proc (54648) on the aura's
				// caster (the mage), with the aura effect. The gate
				// above already verified the caster is alive. No Lua
				// ev6 twin: ev6 carries no aura-caster GUID. DORMANT
				// until Spell.dbc ships the 54646 proc entry.
				if ev.actorGUID != 0 && aura.CasterGUID != 0 {
					s.triggeredByAuraCast = true
					s.triggeredByAuraSpell = auraSpell.ID
					s.server.fireScriptProcCast(ctx, s.player.Map, s.player.InstanceID, scriptProcCast{spellID: 54648, casterGUID: s.playerGUID, targetGUID: aura.CasterGUID})
					s.triggeredByAuraCast = false
					s.triggeredByAuraSpell = 0
				}
				continue
			}
			if auraSpell.ID == 48108 && i == 0 && eff.Aura == spellAuraAddFlatModifier {
				// spell_mage_gen_extra_effects::HandleProc
				// (spell_mage.cpp:801-807): no PreventDefaultAction,
				// so the default 57761 Fireball! trigger still fires
				// via the generic leg below; the proc actor casts
				// the T10 2P bonus effect (70753) on
				// nil (self), triggered, when it carries the T10 2P
				// bonus aura (70752). The C++ script registers whole-
				// aura OnProc; 48108 carries exactly one DBC effect
				// (EFFECT_0 ADD_FLAT_MODIFIER, SpellModOp 10
				// CASTING_TIME for Pyroblast, Wowhead WotLK
				// 2026-11-10), so the effect-0 gate fires once per
				// proc. ADD_FLAT_MODIFIER is already a proc-trigger
				// type (Cobra Strikes precedent). No Lua ev6 twin:
				// ev6 carries no HasAura check. DORMANT until
				// Spell.dbc ships the 48108 proc entry.
				if ev.actorGUID != 0 {
					actorHasT10 := false
					if actorSess := s.server.findSessionByGUID(ev.actorGUID); actorSess != nil {
						actorHasT10 = actorSess.hasAura(70752)
					}
					if actorHasT10 {
						s.triggeredByAuraCast = true
						s.triggeredByAuraSpell = auraSpell.ID
						s.server.fireScriptProcCast(ctx, s.player.Map, s.player.InstanceID, scriptProcCast{spellID: 70753, casterGUID: ev.actorGUID, targetGUID: ev.actorGUID})
						s.triggeredByAuraCast = false
						s.triggeredByAuraSpell = 0
					}
				}
			}
			if auraSpell.ID == 56375 && i == 0 && eff.Aura == spellAuraDummy {
				// spell_mage_glyph_of_polymorph::HandleProc
				// (spell_mage.cpp:831-840): PreventDefaultAction; the
				// polymorph target loses every periodic-damage aura
				// except Shadow Word: Death (32409), plus every
				// periodic-damage-percent and periodic-leech aura.
				// C++ uses the RemoveAurasByType defaults
				// (negative=true, positive=true, Unit.h:1263), so
				// the plain type helpers match. No
				// Lua ev6 twin: ev6 carries no proc-target aura
				// removal. DORMANT until Spell.dbc ships the 56375
				// proc entry.
				if triggerTargetGUID != 0 {
					if ts := s.server.findSessionByGUID(triggerTargetGUID); ts != nil && ts.player != nil {
						ts.removeAurasByTypeExcept(spellAuraPeriodicDamage, 32409)
						ts.removeAurasByType(spellAuraPeriodicDamagePercent)
						ts.removeAurasByType(spellAuraPeriodicLeech)
					} else if m := s.server.findCreatureMotion(s.player.Map, s.player.InstanceID, triggerTargetGUID); m != nil {
						key := creatureAuraKeyForMotion(m)
						s.server.removeCreatureAurasByTypeExcept(key, spellAuraPeriodicDamage, 32409)
						s.server.removeCreatureAurasByType(key, spellAuraPeriodicDamagePercent)
						s.server.removeCreatureAurasByType(key, spellAuraPeriodicLeech)
					}
				}
				continue
			}
			if auraSpell.ID == 56374 && i == 0 && eff.Aura == spellAuraDummy {
				// spell_mage_glyph_of_icy_veins::HandleProc
				// (spell_mage.cpp:862-870): PreventDefaultAction; the
				// actor loses every haste-spells aura (negative-only:
				// C++ passes true, false) and every decrease-speed
				// aura (both polarities: C++ uses the
				// RemoveAurasByType defaults, negative=true,
				// positive=true, Unit.h:1263). The haste leg keeps
				// the inline Positive filter so a positive haste
				// buff survives; the snare leg uses the plain type
				// helper, which removes both polarities. The actor
				// is the holder for this self-cast glyph proc. No
				// Lua ev6 twin: ev6 carries no aura-type removal.
				// DORMANT until Spell.dbc ships the 56374 proc
				// entry.
				var ids []uint32
				s.castMu.Lock()
				for _, a := range s.activeAuras {
					if a != nil && !a.Positive && a.AuraType == spellAuraHasteSpells {
						ids = append(ids, a.SpellID)
					}
				}
				s.castMu.Unlock()
				for _, id := range ids {
					s.removeAura(id)
				}
				s.removeAurasByType(spellAuraModDecreaseSpeed)
				continue
			}
			if auraSpell.ID == 56372 && i == 0 && eff.Aura == spellAuraDummy {
				// spell_mage_glyph_of_ice_block::HandleProc
				// (spell_mage.cpp:892-904): PreventDefaultAction; the
				// proc actor's Mage-family cooldowns whose family
				// flags[0] carry 0x40 finish (Frost Nova). The actor
				// is the holder for this self-cast glyph proc; the
				// session lookup covers an actor that is not the
				// holder. No Lua ev6 twin: ev6 carries no cooldown
				// reset. DORMANT until Spell.dbc ships the 56372
				// proc entry.
				actorSess := s
				if ev.actorGUID != 0 {
					if as := s.server.findSessionByGUID(ev.actorGUID); as != nil {
						actorSess = as
					}
				}
				actorSess.glyphOfIceBlockProcReset()
				continue
			}
			if auraSpell.ID == 49592 && i == 2 && eff.Aura == spellAuraDummy {
				// spell_oculus_temporal_rift::HandleProc
				// (oculus.cpp:606-621): PreventDefaultAction; the
				// triggering damage accumulates in the effect amount
				// (a zero-damage event adds nothing and returns before
				// the threshold check); at 15000 the aura caster casts
				// Amber Shock Charge (49836) on the holder and the
				// counter drops by 15000. The amount lives in the
				// per-effect slot, not the aura-level Amount; the
				// unlocked write matches the loop's RemainingCharges
				// mutation above. The player-side twin of the
				// creature-loop arm, for a player holder. Lua-off
				// fallback twin of the oculus.lua event-6 handler
				// (same veto-first ordering).
				if ev.damage == 0 {
					continue
				}
				amount := aura.Amounts[i] + int32(ev.damage)
				fire := amount >= 15000
				if fire {
					amount -= 15000
				}
				aura.Amounts[i] = amount
				if fire {
					s.server.fireScriptProcCast(ctx, s.player.Map, s.player.InstanceID, scriptProcCast{spellID: 49836, casterGUID: aura.CasterGUID, targetGUID: s.playerGUID})
				}
				continue
			}
			if auraSpell.ID == 49194 && i == 0 && eff.Aura == spellAuraDummy {
				// spell_dk_unholy_blight::HandleProc (spell_dk.cpp:2527-2549):
				// PreventDefaultAction; on a damaging proc the actor casts
				// Unholy Blight damage (50536) on the proc target with BP0
				// = pct(damage, eff0 amount), plus the Glyph of Unholy
				// Blight (63332, EFFECT_0 on the actor) pct, divided by
				// the damage spell's max ticks. The zero-damage gate is
				// the C++ early return (!damageInfo || !GetDamage()).
				// No Lua ev6 twin: spell_dk.lua holds no 49194 handler.
				// DORMANT until Spell.dbc ships the 49194 proc entry and
				// the 50536 duration row; a missing 50536 row falls
				// through to the generic dummy dispatch below, which
				// no-ops for 49194.
				if ev.damage == 0 || triggerTargetGUID == 0 || ev.actorGUID == 0 {
					continue
				}
				amount := int32(int64(ev.damage) * int64(scriptProcEffAmount(aura, i)) / 100)
				if actorSess := s.server.findSessionByGUID(ev.actorGUID); actorSess != nil {
					if glyphAmt, ok := actorSess.auraEffectAmount(63332, 0); ok {
						amount += amount * glyphAmt / 100
					}
				}
				if s.server != nil && s.server.Data != nil {
					if dmgSpell, found, err := s.server.Data.Spell(50536); err == nil && found {
						if ticks := s.spellMaxTicks(dmgSpell); ticks > 0 {
							amount /= int32(ticks)
							s.server.fireScriptProcCast(ctx, s.player.Map, s.player.InstanceID, scriptProcCast{spellID: 50536, casterGUID: ev.actorGUID, targetGUID: triggerTargetGUID, bp0: uint32(amount), bp0Set: true})
							continue
						}
					}
				}
			}
			if auraSpell.ID == 56841 && i == 0 && eff.Aura == spellAuraDummy {
				// spell_hun_glyph_of_arcane_shot::HandleProc
				// (spell_hunter.cpp:454-468): PreventDefaultAction; the
				// refunded mana is pct of the triggering spell's power
				// cost by the effect amount, cast as 61389 on the holder
				// with BP0. The CheckProc sting gate ran before the
				// chance roll above. No Lua ev6 twin: the hook carries
				// no proc-spell object, so Lua cannot read the
				// triggering cost, and it has no family-flag aura scan.
				// DORMANT until Spell.dbc ships the 56841 proc entry.
				// C++ casts 61389 non-triggered ("is this intended?"
				// in the source); Go has no non-triggered direct-cast
				// twin, so the triggered twin rides
				// castSpellDirectWithBasePoint (the 43714 precedent).
				if ev.eventSpell == nil {
					continue
				}
				mana := int64(s.calculateSpellPowerCost(*ev.eventSpell)) * int64(scriptProcEffAmount(aura, i)) / 100
				if mana < 0 {
					mana = 0
				}
				s.castSpellDirectWithBasePoint(ctx, 61389, s.playerGUID, uint32(mana))
				continue
			}
			if auraSpell.ID == 54939 && i == 0 && eff.Aura == spellAuraAddPctModifier {
				// spell_pal_glyph_of_divinity::OnProc
				// (spell_paladin.cpp:819-833): Lay on Hands (Rank 1) has
				// no mana effect — the gate is the triggering spell's
				// EFFECT_1 being SPELL_EFFECT_ENERGIZE; a cast on the
				// proc target itself (self-cast LoH) takes nothing. The
				// actor casts Glyph of Divinity Proc (54986) on nil
				// (self) with BP1 = twice the triggering spell's
				// EFFECT_1 CalcValue (no-caster CalcValue,
				// SpellInfo.cpp:402 — no level scaling, no mods). No
				// PreventDefaultAction in C++, but HandleProc's default
				// arm is a no-op for ADD_PCT_MODIFIER
				// (SpellAuraEffects.cpp:1010-1043), so the `continue`
				// past the generic leg below is exact. No Lua ev6 twin:
				// ev6 carries no proc spell. DORMANT until Spell.dbc
				// ships the 54939 proc entry.
				if ev.eventSpell == nil || len(ev.eventSpell.Effects) < 2 || ev.eventSpell.Effects[1].Effect != spellEffectEnergize {
					continue
				}
				if ev.actorGUID == 0 || ev.actorGUID == triggerTargetGUID {
					continue
				}
				bp1 := ev.eventSpell.Effects[1].CalcValue() * 2
				if bp1 < 0 {
					bp1 = 0
				}
				s.triggeredByAuraCast = true
				s.triggeredByAuraSpell = auraSpell.ID
				s.server.fireScriptProcCast(ctx, s.player.Map, s.player.InstanceID, scriptProcCast{spellID: 54986, casterGUID: ev.actorGUID, targetGUID: ev.actorGUID, bp1: uint32(bp1), bp1Set: true})
				s.triggeredByAuraCast = false
				s.triggeredByAuraSpell = 0
				continue
			}
			if auraSpell.ID == 54937 && i == 0 && eff.Aura == spellAuraDummy {
				// spell_pal_glyph_of_holy_light_dummy::HandleProc
				// (spell_paladin.cpp:898-912): PreventDefaultAction; on a
				// heal proc the actor casts Glyph of Holy Light Heal
				// (54968) on the proc target with BP0 = pct of the heal
				// amount by the effect amount, where the amount is the
				// effective heal when it reaches the triggering spell's
				// EFFECT_0 BasePoints + DieSides, else the raw heal
				// (HealInfo::GetHeal / GetEffectiveHeal,
				// Spell.cpp:2512). The proc loop carries no heal info on
				// non-heal events, so a zero heal is the C++ null/zero
				// early return. No Lua ev6 twin: ev6 carries no heal
				// amounts. DORMANT until Spell.dbc ships the 54937 proc
				// entry; periodic / funnel / leech heal events carry no
				// amounts in Go, so they skip here where C++ would fire
				// with the tick amount.
				if ev.heal == 0 || ev.eventSpell == nil || len(ev.eventSpell.Effects) == 0 {
					continue
				}
				basePoints := ev.eventSpell.Effects[0].BasePoints + ev.eventSpell.Effects[0].DieSides
				healAmount := ev.heal
				if int64(ev.effectiveHeal) >= int64(basePoints) {
					healAmount = ev.effectiveHeal
				}
				if triggerTargetGUID == 0 || ev.actorGUID == 0 {
					continue
				}
				s.triggeredByAuraCast = true
				s.triggeredByAuraSpell = auraSpell.ID
				s.server.fireScriptProcCast(ctx, s.player.Map, s.player.InstanceID, scriptProcCast{spellID: 54968, casterGUID: ev.actorGUID, targetGUID: triggerTargetGUID, bp0: uint32(uint64(healAmount) * uint64(scriptProcEffAmount(aura, i)) / 100), bp0Set: true})
				s.triggeredByAuraCast = false
				s.triggeredByAuraSpell = 0
				continue
			}
			if auraSpell.ID == 37705 && i == 0 && eff.Aura == spellAuraProcTriggerSpell {
				// spell_pal_item_healing_discount::HandleProc
				// (spell_paladin.cpp:1424-1431): PreventDefaultAction; the
				// aura holder casts Healing Trance (37706) on itself with
				// the aura effect. The generic leg below would cast the
				// DBC trigger spell on the proc target instead — the
				// holder-self target is the deviation this case exists
				// for. No Lua ev6 twin: ev6 carries no proc spell.
				// DORMANT until Spell.dbc ships the 37705 proc entry.
				s.triggeredByAuraCast = true
				s.triggeredByAuraSpell = auraSpell.ID
				s.castSpellDirect(ctx, 37706, s.playerGUID)
				s.triggeredByAuraCast = false
				s.triggeredByAuraSpell = 0
				continue
			}
			if auraSpell.ID == 20185 && i == 0 && eff.Aura == spellAuraDummy {
				// spell_pal_judgement_of_light_heal::HandleProc
				// (spell_paladin.cpp:1610-1631): PreventDefaultAction;
				// the proc target (GetProcTarget, the other unit on
				// both sides — triggerTargetGUID here) casts
				// Judgement of Light Heal (20267) on itself with BP0
				// = pct of its max health by the effect amount. No
				// Lua ev6 twin: ev6 carries no max-health value. The
				// C++ OriginalCaster (the judging paladin) has no Go
				// bridge — the heal rides the proc target as caster.
				// DORMANT until Spell.dbc ships the 20185 proc entry.
				if triggerTargetGUID == 0 {
					continue
				}
				maxHealth, _, _, ok := s.server.procTargetVitals(s.player.Map, s.player.InstanceID, triggerTargetGUID)
				if !ok || maxHealth == 0 {
					continue
				}
				s.triggeredByAuraCast = true
				s.triggeredByAuraSpell = auraSpell.ID
				s.server.fireScriptProcCast(ctx, s.player.Map, s.player.InstanceID, scriptProcCast{spellID: 20267, casterGUID: triggerTargetGUID, targetGUID: triggerTargetGUID, bp0: uint32(uint64(maxHealth) * uint64(scriptProcEffAmount(aura, i)) / 100), bp0Set: true})
				s.triggeredByAuraCast = false
				s.triggeredByAuraSpell = 0
				continue
			}
			if auraSpell.ID == 20186 && i == 0 && eff.Aura == spellAuraDummy {
				// spell_pal_judgement_of_wisdom_mana::HandleProc
				// (spell_paladin.cpp:1649-1679): PreventDefaultAction;
				// the CheckProc mana-power gate runs first (a rage /
				// energy / runic-power proc target takes nothing),
				// then the proc target casts Judgement of Wisdom Mana
				// (20268) on itself with BP0 = pct of its CreateMana
				// by the 20268 EFFECT_0 CalcValue. Same no-Original-
				// Caster and no-ev6 notes as the 20185 leg above. A
				// missing 20268 DBC row skips where C++ would assert.
				// DORMANT until Spell.dbc ships the 20186 proc entry.
				if triggerTargetGUID == 0 {
					continue
				}
				_, baseMana, powerType, ok := s.server.procTargetVitals(s.player.Map, s.player.InstanceID, triggerTargetGUID)
				if !ok || powerType != 0 { // POWER_MANA (SharedDefines.h:1186)
					continue
				}
				manaSpell, found, err := s.server.Data.Spell(20268)
				if err != nil || !found || len(manaSpell.Effects) == 0 {
					continue
				}
				amount := int64(baseMana) * int64(manaSpell.Effects[0].CalcValue()) / 100
				if amount < 0 {
					amount = 0
				}
				s.triggeredByAuraCast = true
				s.triggeredByAuraSpell = auraSpell.ID
				s.server.fireScriptProcCast(ctx, s.player.Map, s.player.InstanceID, scriptProcCast{spellID: 20268, casterGUID: triggerTargetGUID, targetGUID: triggerTargetGUID, bp0: uint32(amount), bp0Set: true})
				s.triggeredByAuraCast = false
				s.triggeredByAuraSpell = 0
				continue
			}
			if auraSpell.ID == 53651 && i == 0 && eff.Aura == spellAuraDummy {
				// spell_pal_light_s_beacon::HandleProc
				// (spell_paladin.cpp:1825-1865): PreventDefaultAction;
				// the 53651 dummy aura rides every raid member in the
				// beacon's range (the 53563 periodic trigger applies
				// it), so this is a heal-TAKEN proc on the holder. The
				// CheckProc veto fires when the holder itself carries
				// the beacon (53563) from the healer — healing the
				// beacon target directly never re-triggers. The actor
				// (healer) casts the beacon heal on the beacon target
				// (the aura caster, which must still carry 53563 from
				// the actor) with BP0 = pct of the heal by the effect
				// amount: 53652 for a Holy Light rank, 53654 for any
				// other heal (53653 is validated in C++ but never
				// cast). The rank check rides spellFirstRank; an
				// unchained rank falls through to 53654. No Lua ev6
				// twin: ev6 carries no heal amount or proc spell.
				// DORMANT until Spell.dbc ships the 53651 proc entry.
				if ev.actorGUID == 0 || ev.eventSpell == nil || ev.heal == 0 {
					continue
				}
				if s.unitHasAuraFromCaster(s.playerGUID, 53563, ev.actorGUID) {
					continue
				}
				healSpellID := uint32(53654)
				if s.server.spellFirstRank(ev.eventSpell.ID) == s.server.spellFirstRank(635) {
					healSpellID = 53652
				}
				beaconTarget := aura.CasterGUID
				if beaconTarget == 0 || !s.unitHasAuraFromCaster(beaconTarget, 53563, ev.actorGUID) {
					continue
				}
				s.triggeredByAuraCast = true
				s.triggeredByAuraSpell = auraSpell.ID
				s.server.fireScriptProcCast(ctx, s.player.Map, s.player.InstanceID, scriptProcCast{spellID: healSpellID, casterGUID: ev.actorGUID, targetGUID: beaconTarget, bp0: uint32(uint64(ev.heal) * uint64(scriptProcEffAmount(aura, i)) / 100), bp0Set: true})
				s.triggeredByAuraCast = false
				s.triggeredByAuraSpell = 0
				continue
			}
			if auraSpell.ID == 53601 && i == 0 && eff.Aura == spellAuraDummy {
				// spell_pal_sacred_shield_dummy::HandleProc
				// (spell_paladin.cpp:2049-2066): PreventDefaultAction;
				// the caster (the paladin) casts Sacred Shield (58597)
				// on the action target (the healed unit) with the
				// triggering aura. The cooldown is the effect amount
				// in seconds, overridden by the T8 Holy 4P bonus
				// (64895) EFFECT_0 amount on the caster; the per-aura
				// ProcCooldownUntil is the Go twin of the C++
				// _cooldownEnd member (the loop gate above already
				// skips while it is in the future). No Lua ev6 twin:
				// ev6 carries no cooldown state. DORMANT until
				// Spell.dbc ships the 53601 proc entry.
				casterGUID := aura.CasterGUID
				if casterGUID == 0 {
					continue
				}
				cooldown := time.Duration(scriptProcEffAmount(aura, i)) * time.Second
				if amt, ok := s.auraEffectAmountFromCaster(64895, 0, casterGUID); ok {
					cooldown = time.Duration(amt) * time.Second
				}
				aura.ProcCooldownUntil = time.Now().Add(cooldown)
				if actionTarget := procActionTargetGUID(ev.actorGUID, s.playerGUID, triggerTargetGUID); actionTarget != 0 {
					s.triggeredByAuraCast = true
					s.triggeredByAuraSpell = auraSpell.ID
					s.server.fireScriptProcCast(ctx, s.player.Map, s.player.InstanceID, scriptProcCast{spellID: 58597, casterGUID: casterGUID, targetGUID: actionTarget})
					s.triggeredByAuraCast = false
					s.triggeredByAuraSpell = 0
				}
				continue
			}
			if (auraSpell.ID == 31801 || auraSpell.ID == 53736) && i == 0 && eff.Aura == spellAuraDummy {
				// spell_pal_seal_of_vengeance::HandleApplyDoT +
				// HandleSeal (spell_paladin.cpp:2180-2231): both
				// OnEffectProc legs on EFFECT_0 DUMMY, each with
				// PreventDefaultAction, so they run independently on
				// the same proc event. HandleApplyDoT applies the seal
				// DoT (Holy Vengeance 31803 / Blood Corruption 53742)
				// on the proc target: only auto-attacks, or Hammer of
				// the Righteous (icon 3023) otherwise. HandleSeal deals
				// the stack-scaled damage (Seal of Vengeance 42463 /
				// Seal of Corruption 53739): it needs the target's live
				// periodic-damage seal DoT from the actor, and below max
				// stacks it only fires on spell-melee-damage-class
				// events; the amount is the DBC EFFECT_0 CalcValue
				// scaled by stacks/maxStacks. TRIGGERED_DONT_RESET_
				// PERIODIC_TIMER has no twin in the Go script-proc cast
				// path. No Lua ev6 twin: ev6 carries no stack counts
				// or type masks. DORMANT until Spell.dbc ships the
				// 31801/53736 proc entries.
				if ev.actorGUID == 0 || triggerTargetGUID == 0 {
					continue
				}
				dotSpellID := uint32(31803)
				damageSpellID := uint32(42463)
				if auraSpell.ID == 53736 {
					dotSpellID = 53742
					damageSpellID = 53739
				}
				if ev.typeMask&procFlagDoneMeleeAutoAttack != 0 || (ev.eventSpell != nil && ev.eventSpell.SpellIconID == 3023) {
					s.triggeredByAuraCast = true
					s.triggeredByAuraSpell = auraSpell.ID
					s.server.fireScriptProcCast(ctx, s.player.Map, s.player.InstanceID, scriptProcCast{spellID: dotSpellID, casterGUID: ev.actorGUID, targetGUID: triggerTargetGUID})
					s.triggeredByAuraCast = false
					s.triggeredByAuraSpell = 0
				}
				if stacks, maxStacks, ok := s.server.sealOfVengeanceDoTStacks(s.player.Map, s.player.InstanceID, triggerTargetGUID, ev.actorGUID); ok {
					if stacks >= maxStacks || ev.typeMask&procFlagDoneSpellMeleeDmgClass != 0 {
						if dmgSpell, found, err := s.server.Data.Spell(damageSpellID); err == nil && found && len(dmgSpell.Effects) > 0 {
							amount := int64(dmgSpell.Effects[0].CalcValue()) * int64(stacks) / int64(maxStacks)
							if amount < 0 {
								amount = 0
							}
							s.triggeredByAuraCast = true
							s.triggeredByAuraSpell = auraSpell.ID
							s.server.fireScriptProcCast(ctx, s.player.Map, s.player.InstanceID, scriptProcCast{spellID: damageSpellID, casterGUID: ev.actorGUID, targetGUID: triggerTargetGUID, bp0: uint32(amount), bp0Set: true})
							s.triggeredByAuraCast = false
							s.triggeredByAuraSpell = 0
						}
					}
				}
				continue
			}
			if auraSpell.ID == 28789 && i == 0 && eff.Aura == spellAuraDummy {
				// spell_pal_t3_6p_bonus::HandleProc
				// (spell_paladin.cpp:2380-2423): PreventDefaultAction;
				// the actor casts the class-bonus spell on the proc
				// target by its class: MP5 (28795) for paladin /
				// priest / shaman / druid, spell power (28793) for
				// mage / warlock, attack power (28791) for hunter /
				// rogue, armor (28790) for warrior; any other class
				// casts nothing. No Lua ev6 twin: ev6 carries no
				// target class. DORMANT until Spell.dbc ships the
				// 28789 proc entry.
				if ev.actorGUID == 0 || triggerTargetGUID == 0 {
					continue
				}
				classID, ok := s.server.procTargetClass(s.player.Map, s.player.InstanceID, triggerTargetGUID)
				if !ok {
					continue
				}
				var bonusSpellID uint32
				switch classID {
				case 2, 5, 7, 11: // CLASS_PALADIN, CLASS_PRIEST, CLASS_SHAMAN, CLASS_DRUID
					bonusSpellID = 28795
				case 8, 9: // CLASS_MAGE, CLASS_WARLOCK
					bonusSpellID = 28793
				case 3, 4: // CLASS_HUNTER, CLASS_ROGUE
					bonusSpellID = 28791
				case 1: // CLASS_WARRIOR
					bonusSpellID = 28790
				default:
					continue
				}
				s.triggeredByAuraCast = true
				s.triggeredByAuraSpell = auraSpell.ID
				s.server.fireScriptProcCast(ctx, s.player.Map, s.player.InstanceID, scriptProcCast{spellID: bonusSpellID, casterGUID: ev.actorGUID, targetGUID: triggerTargetGUID})
				s.triggeredByAuraCast = false
				s.triggeredByAuraSpell = 0
				continue
			}
			if auraSpell.ID == 26169 && i == 0 && eff.Aura == spellAuraDummy {
				// spell_pri_aq_3p_bonus::HandleProc
				// (spell_priest.cpp:159-173): PreventDefaultAction; on a
				// heal proc the actor casts Oracular Heal (26170) on
				// itself with BP0 = 10% of the heal — unless the actor
				// healed itself, or the heal is null/zero. No Lua ev6
				// twin: ev6 carries no heal amounts. DORMANT until
				// Spell.dbc ships the 26169 proc entry; periodic /
				// funnel / leech heal events carry no amounts in Go,
				// so they skip here where C++ would fire with the tick
				// amount (the 54937 precedent).
				if ev.heal == 0 || ev.actorGUID == 0 || ev.actorGUID == triggerTargetGUID {
					continue
				}
				s.triggeredByAuraCast = true
				s.triggeredByAuraSpell = auraSpell.ID
				s.server.fireScriptProcCast(ctx, s.player.Map, s.player.InstanceID, scriptProcCast{spellID: 26170, casterGUID: ev.actorGUID, targetGUID: ev.actorGUID, bp0: uint32(uint64(ev.heal) * 10 / 100), bp0Set: true})
				s.triggeredByAuraCast = false
				s.triggeredByAuraSpell = 0
				continue
			}
			if auraSpell.ID == 55440 && i == 0 && eff.Aura == spellAuraDummy {
				// spell_sha_glyph_of_healing_wave::HandleProc
				// (spell_shaman.cpp:951-965): PreventDefaultAction; on a
				// heal proc the actor casts Glyph of Healing Wave Heal
				// (55533) on itself with BP0 = pct of the heal by the
				// effect amount — unless the actor healed itself, or
				// the heal is null/zero. No Lua ev6 twin: ev6 carries
				// no heal amounts. DORMANT until Spell.dbc ships the
				// 55440 proc entry; periodic / funnel / leech heal
				// events carry no amounts in Go, so they skip here
				// where C++ would fire with the tick amount (the
				// 54937 precedent).
				if ev.heal == 0 || ev.actorGUID == 0 || ev.actorGUID == triggerTargetGUID {
					continue
				}
				s.triggeredByAuraCast = true
				s.triggeredByAuraSpell = auraSpell.ID
				s.server.fireScriptProcCast(ctx, s.player.Map, s.player.InstanceID, scriptProcCast{spellID: 55533, casterGUID: ev.actorGUID, targetGUID: ev.actorGUID, bp0: uint32(uint64(ev.heal) * uint64(scriptProcEffAmount(aura, i)) / 100), bp0Set: true})
				s.triggeredByAuraCast = false
				s.triggeredByAuraSpell = 0
				continue
			}
			if auraSpell.ID == 70811 && i == 0 && eff.Aura == spellAuraDummy {
				// spell_sha_item_t10_elemental_2p_bonus::HandleEffectProc
				// (spell_shaman.cpp:1446-1452): PreventDefaultAction; the
				// target player (the aura holder in this loop) has
				// Elemental Mastery (16166) shifted by -EFFECT_0 amount.
				// The C++ ToPlayer gate makes this a player-only leg:
				// no creature twin (creatures carry no spell history).
				// No Lua ev6 twin: spell_shaman.lua holds no 70811
				// handler. DORMANT until Spell.dbc ships the 70811 proc
				// entry.
				s.modifyPlayerSpellCooldown(16166, -int32(scriptProcEffAmount(aura, i)))
				continue
			}
			if auraSpell.ID == 55677 && i == 0 && eff.Aura == spellAuraDummy {
				// spell_pri_glyph_of_dispel_magic::HandleProc
				// (spell_priest.cpp:439-457): PreventDefaultAction; on a
				// Dispel Magic (or Abolish Disease — both share spell
				// icon 74) proc the actor casts Glyph of Dispel Magic
				// Heal (56131) on the proc target with BP0 = the effect
				// amount pct of the target's max health. The icon gate
				// is the C++ SpellIconID check on the triggering spell;
				// a dispel that dispels nothing still procs in C++, so
				// there is no dispelled-count gate here. No Lua ev6
				// twin: ev6 carries no proc spell or max health.
				// DORMANT until Spell.dbc ships the 55677 proc entry.
				if ev.eventSpell == nil || ev.eventSpell.SpellIconID != 74 || triggerTargetGUID == 0 || ev.actorGUID == 0 {
					continue
				}
				maxHealth, _, _, ok := s.server.procTargetVitals(s.player.Map, s.player.InstanceID, triggerTargetGUID)
				if !ok || maxHealth == 0 {
					continue
				}
				s.triggeredByAuraCast = true
				s.triggeredByAuraSpell = auraSpell.ID
				s.server.fireScriptProcCast(ctx, s.player.Map, s.player.InstanceID, scriptProcCast{spellID: 56131, casterGUID: ev.actorGUID, targetGUID: triggerTargetGUID, bp0: uint32(uint64(maxHealth) * uint64(scriptProcEffAmount(aura, i)) / 100), bp0Set: true})
				s.triggeredByAuraCast = false
				s.triggeredByAuraSpell = 0
				continue
			}
			if auraSpell.ID == 55680 && i == 0 && eff.Aura == spellAuraDummy {
				// spell_pri_glyph_of_prayer_of_healing::HandleProc
				// (spell_priest.cpp:480-501): PreventDefaultAction; on a
				// heal proc the aura HOLDER casts Glyph of Prayer of
				// Healing Heal (56161) on the proc target with BP0 = pct
				// of the heal by the effect amount, divided by the
				// triggered spell's max ticks. The zero-heal gate is the
				// C++ null/zero-heal early return. No Lua ev6 twin: ev6
				// carries no heal amounts. DORMANT until Spell.dbc ships
				// the 55680 proc entry and the 56161 duration row; a
				// missing 56161 row skips where C++ would assert (the
				// 49194 precedent).
				if ev.heal == 0 || triggerTargetGUID == 0 {
					continue
				}
				if s.server == nil || s.server.Data == nil {
					continue
				}
				healSpell, found, err := s.server.Data.Spell(56161)
				if err != nil || !found {
					continue
				}
				ticks := s.spellMaxTicks(healSpell)
				if ticks == 0 {
					continue
				}
				amount := int64(ev.heal) * int64(scriptProcEffAmount(aura, i)) / 100 / int64(ticks)
				if amount < 0 {
					amount = 0
				}
				s.triggeredByAuraCast = true
				s.triggeredByAuraSpell = auraSpell.ID
				s.server.fireScriptProcCast(ctx, s.player.Map, s.player.InstanceID, scriptProcCast{spellID: 56161, casterGUID: s.playerGUID, targetGUID: triggerTargetGUID, bp0: uint32(amount), bp0Set: true})
				s.triggeredByAuraCast = false
				s.triggeredByAuraSpell = 0
				continue
			}
			if auraSpell.ID == 40438 && i == 0 && eff.Aura == spellAuraDummy {
				// spell_pri_item_t6_trinket::HandleProc
				// (spell_priest.cpp:696-705): PreventDefaultAction; on a
				// heal proc the actor casts Divine Blessing (40440) on
				// null, on a damage proc Divine Wrath (40441) on null —
				// both spells are self-targeted, so the null target reads
				// as the actor (the 26170 self-cast precedent). Both
				// arms can fire on one event when the mask carries both
				// bits. The spell-type mask is the C++ GetSpellTypeMask
				// twin: HIT-phase events carry Damage or Heal (melee
				// events carry None and fire neither); the generated
				// entry's HIT-only phase mask keeps CAST/FINISH events
				// out, so no extra phase gate is needed. No Lua ev6
				// twin: ev6 carries no spell-type mask. DORMANT until
				// Spell.dbc ships the 40438 proc entry.
				if ev.actorGUID == 0 {
					continue
				}
				s.triggeredByAuraCast = true
				s.triggeredByAuraSpell = auraSpell.ID
				if ev.spellTypeMask&procSpellTypeHeal != 0 {
					s.server.fireScriptProcCast(ctx, s.player.Map, s.player.InstanceID, scriptProcCast{spellID: 40440, casterGUID: ev.actorGUID, targetGUID: ev.actorGUID})
				}
				if ev.spellTypeMask&procSpellTypeDamage != 0 {
					s.server.fireScriptProcCast(ctx, s.player.Map, s.player.InstanceID, scriptProcCast{spellID: 40441, casterGUID: ev.actorGUID, targetGUID: ev.actorGUID})
				}
				s.triggeredByAuraCast = false
				s.triggeredByAuraSpell = 0
				continue
			}
			if auraSpell.ID == 15286 && i == 0 && eff.Aura == spellAuraDummy {
				// spell_pri_vampiric_embrace::HandleProc
				// (spell_priest.cpp:1363-1378): PreventDefaultAction; on a
				// damage proc the actor casts Vampiric Embrace Heal
				// (15290) on null with BP0 = selfHeal/5 (integer division)
				// and BP1 = selfHeal, where selfHeal is pct of the damage
				// by the effect amount. The null target is the spell's own
				// implicit targets (TARGET_UNIT_CASTER_AREA_PARTY) — the
				// zero target rides the cast path's fallback legs, not the
				// 40438 self-cast precedent. No Lua ev6 twin: ev6 carries
				// no damage amounts. DORMANT until Spell.dbc ships the
				// 15286 proc entry.
				if ev.damage == 0 || ev.actorGUID == 0 {
					continue
				}
				selfHeal := uint32(uint64(ev.damage) * uint64(scriptProcEffAmount(aura, i)) / 100)
				partyHeal := selfHeal / 5
				s.triggeredByAuraCast = true
				s.triggeredByAuraSpell = auraSpell.ID
				s.server.fireScriptProcCast(ctx, s.player.Map, s.player.InstanceID, scriptProcCast{spellID: 15290, casterGUID: ev.actorGUID, targetGUID: 0, bp0: partyHeal, bp0Set: true, bp1: selfHeal, bp1Set: true})
				s.triggeredByAuraCast = false
				s.triggeredByAuraSpell = 0
				continue
			}
			if auraSpell.ID == 28809 && i == 0 && eff.Aura == spellAuraDummy {
				// spell_pri_t3_4p_bonus::HandleProc
				// (spell_priest.cpp:1467-1472): PreventDefaultAction; the
				// actor casts Armor of Faith (28810) on the proc target
				// with the aura effect. No Lua ev6 twin: ev6 carries no
				// proc-target routing for this arm. DORMANT until
				// Spell.dbc ships the 28809 proc entry.
				if ev.actorGUID == 0 || triggerTargetGUID == 0 {
					continue
				}
				s.triggeredByAuraCast = true
				s.triggeredByAuraSpell = auraSpell.ID
				s.server.fireScriptProcCast(ctx, s.player.Map, s.player.InstanceID, scriptProcCast{spellID: 28810, casterGUID: ev.actorGUID, targetGUID: triggerTargetGUID})
				s.triggeredByAuraCast = false
				s.triggeredByAuraSpell = 0
				continue
			}
			if auraSpell.ID == 37594 && i == 0 && eff.Aura == spellAuraProcTriggerSpell {
				// spell_pri_t5_heal_2p_bonus::HandleProc + CheckProc
				// (spell_priest.cpp:1504-1524): PreventDefaultAction; on a
				// heal proc whose effective heal is positive and whose
				// heal target sits at full health (the overheal gate) the
				// aura HOLDER casts Item Efficiency (37595) on itself
				// with the aura effect. Placed before the generic
				// PROC_TRIGGER_SPELL arm, which would cast the DBC
				// trigger instead (the 62259 precedent); the
				// TriggerSpell == 0 gate below never sees it. The heal
				// is already applied when the proc loop runs (spells.go
				// heal leg), so the full-health read is post-heal, like
				// C++. No Lua ev6 twin: ev6 carries no heal amounts.
				// DORMANT until Spell.dbc ships the 37594 proc entry.
				if ev.effectiveHeal == 0 || triggerTargetGUID == 0 {
					continue
				}
				if !s.server.procTargetFullHealth(s.player.Map, s.player.InstanceID, triggerTargetGUID) {
					continue
				}
				s.triggeredByAuraCast = true
				s.triggeredByAuraSpell = auraSpell.ID
				s.server.fireScriptProcCast(ctx, s.player.Map, s.player.InstanceID, scriptProcCast{spellID: 37595, casterGUID: s.playerGUID, targetGUID: s.playerGUID})
				s.triggeredByAuraCast = false
				s.triggeredByAuraSpell = 0
				continue
			}
			if auraSpell.ID == 70770 && i == 0 && eff.Aura == spellAuraDummy {
				// spell_pri_t10_heal_2p_bonus::HandleProc
				// (spell_priest.cpp:1547-1576): PreventDefaultAction; on a
				// heal proc the actor casts Blessed Healing (70772) on the
				// proc target with BP0 = pct of the heal by the effect
				// amount, divided by the triggered spell's max ticks. The
				// zero-heal gate is the C++ null/zero-heal early return; a
				// missing 70772 row skips where C++ would assert (the
				// 49194 precedent). No Lua ev6 twin: ev6 carries no heal
				// amounts. DORMANT until Spell.dbc ships the 70770 proc
				// entry and the 70772 duration row.
				if ev.heal == 0 || ev.actorGUID == 0 || triggerTargetGUID == 0 {
					continue
				}
				if s.server == nil || s.server.Data == nil {
					continue
				}
				healSpell, found, err := s.server.Data.Spell(70772)
				if err != nil || !found {
					continue
				}
				ticks := s.spellMaxTicks(healSpell)
				if ticks == 0 {
					continue
				}
				amount := int64(ev.heal) * int64(scriptProcEffAmount(aura, i)) / 100 / int64(ticks)
				if amount < 0 {
					amount = 0
				}
				s.triggeredByAuraCast = true
				s.triggeredByAuraSpell = auraSpell.ID
				s.server.fireScriptProcCast(ctx, s.player.Map, s.player.InstanceID, scriptProcCast{spellID: 70772, casterGUID: ev.actorGUID, targetGUID: triggerTargetGUID, bp0: uint32(amount), bp0Set: true})
				s.triggeredByAuraCast = false
				s.triggeredByAuraSpell = 0
				continue
			}
			if eff.Aura == spellAuraDummy {
				if cast, ok := scriptAuraProcCast(auraSpell.ID, i, eff.Aura, ev.damage, scriptProcEffAmount(aura, i), ev.actorGUID, s.playerGUID, triggerTargetGUID, ev.eventSpell); ok {
					s.server.fireScriptProcCast(ctx, s.player.Map, s.player.InstanceID, cast)
					continue
				}
			}
			if eff.Aura == spellAuraProcTriggerDamage {
				// AuraEffect::HandleProcTriggerDamageAuraProc
				// (SpellAuraEffects.cpp:5738-5762) deals the effect's own
				// amount as direct spell damage — no triggered spell to
				// look up, no hit or crit roll.
				s.procTriggerDamageAuraProc(ctx, aura, auraSpell, i, triggerTargetGUID)
				continue
			}
			if eff.Aura == spellAuraRaidProcFromCharge || eff.Aura == spellAuraRaidProcFromChargeWithValue {
				// AuraEffect::HandleRaidProcFromChargeAuraProc /
				// HandleRaidProcFromChargeWithValueAuraProc
				// (SpellAuraEffects.cpp:5765-5858): the charge-jump proc.
				// The generic charge consume above already ran (C++
				// PrepareProcToTrigger, SpellAuras.cpp:2025-2043), so
				// aura.RemainingCharges is the post-prepare count C++
				// reads as jumps.
				s.procRaidProcFromCharge(ctx, aura, auraSpell, eff, i)
				continue
			}
			if eff.TriggerSpell == 0 {
				// C++ HandleProcTriggerSpellAuraProc warns and returns when
				// the effect carries no triggered spell (SpellAuraEffects.cpp:5654-5661).
				continue
			}
			if auraSpell.ID == 71770 {
				// spell_putricide_ooze_tank_protection::HandleProc
				// (boss_professor_putricide.cpp:1017-1040): PreventDefaultAction
				// + the ACTION TARGET casts the DBC trigger spell on null
				// (self). The generic arm below would cast from the holder
				// onto the proc target instead.
				if actionTarget := procActionTargetGUID(ev.actorGUID, s.playerGUID, triggerTargetGUID); actionTarget != 0 {
					s.server.fireScriptProcCast(ctx, s.player.Map, s.player.InstanceID, scriptProcCast{spellID: eff.TriggerSpell, casterGUID: actionTarget, targetGUID: actionTarget})
				}
				continue
			}
			if eff.Aura == spellAuraProcTriggerSpell && eff.TriggerSpell == triggerReplenishLifeSpell && auraSpell.ID == spellVampiricAura {
				// SpellAuraEffects.cpp:5670-5705 — npcbot fork: Vampiric
				// Aura's proc trigger replaces the normal triggered-by-aura
				// cast with a plain triggered cast of 25155 carrying the
				// proc damage as base points (full when the trigger target
				// is the aura caster, quartered otherwise); zero damage
				// fizzles the trigger. No triggeredByAuraCast: C++ passes
				// plain extra args, not the aura effect.
				if ev.damage == 0 {
					continue
				}
				basePoint := ev.damage
				if triggerTargetGUID != aura.CasterGUID {
					basePoint /= 4
				}
				s.castSpellDirectWithBasePoint(ctx, eff.TriggerSpell, triggerTargetGUID, basePoint)
				continue
			}
			if eff.Aura == spellAuraProcTriggerSpellWithValue {
				// HandleProcTriggerSpellWithValueAuraProc passes the
				// triggering effect's own amount (SpellAuraEffects.cpp:5726-5728).
				basePoint := aura.Amount
				if i < len(aura.Amounts) && aura.Amounts[i] > 0 {
					basePoint = uint32(aura.Amounts[i])
				}
				// HandleProcTriggerSpellAuraProc passes the triggering aura
				// effect (SpellAuraEffects.cpp:5706), so m_triggeredByAuraSpell
				// is set — the DR group resolves the triggered variant
				// (Spell.cpp:2798).
				s.triggeredByAuraCast = true
				s.triggeredByAuraSpell = auraSpell.ID
				s.castSpellDirectWithBasePoint(ctx, eff.TriggerSpell, triggerTargetGUID, basePoint)
				s.triggeredByAuraCast = false
				s.triggeredByAuraSpell = 0
			} else {
				s.triggeredByAuraCast = true
				s.triggeredByAuraSpell = auraSpell.ID
				s.castSpellDirect(ctx, eff.TriggerSpell, triggerTargetGUID)
				s.triggeredByAuraCast = false
				s.triggeredByAuraSpell = 0
			}
		}
		if disableProc {
			s.procDeep--
		}
		// Aura::TriggerProcOnEvent (SpellAuras.cpp:2209-2211): the aura is
		// removed only after all proc effects and after-proc handlers
		// complete — never before the effects fire.
		if chargesDepleted {
			s.removeAura(aura.SpellID)
		}
	}
}

// procTriggerDamageAuraProc mirrors
// AuraEffect::HandleProcTriggerDamageAuraProc
// (SpellAuraEffects.cpp:5738-5762): a SPELL_AURA_PROC_TRIGGER_DAMAGE (43)
// effect deals its own amount as direct spell damage of the aura spell's
// school on the proc target. The C++ arm goes straight from
// SpellDamageBonusDone into CalculateSpellDamageTaken, the damage mods, and
// the non-melee damage log — never a hit roll, never a crit roll — which the
// NoCrit pipeline leg reproduces; DealSpellDamage's kill and proc aftermath
// flows through the same shared path. The C++ IsImmunedToDamage /
// SendTickImmune pre-check has no Go creature model; player-victim immunity
// is enforced inside the damage pipeline.
func (s *session) procTriggerDamageAuraProc(ctx context.Context, aura *activeAura, auraSpell wotlk.Spell, effIndex int, triggerTargetGUID uint64) {
	if aura == nil {
		return
	}
	basePoint := aura.Amount
	if effIndex >= 0 && effIndex < len(aura.Amounts) && aura.Amounts[effIndex] > 0 {
		basePoint = uint32(aura.Amounts[effIndex])
	}
	s.executeSpellDamageNoCrit(ctx, triggerTargetGUID, auraSpell.ID, basePoint, effIndex)
}

// procRaidProcFromCharge mirrors AuraEffect::HandleRaidProcFromChargeAuraProc
// (SpellAuraEffects.cpp:5765-5809) and
// HandleRaidProcFromChargeWithValueAuraProc (SpellAuraEffects.cpp:5811-5858):
// the 223/225 charge-jump proc (Shiver, Cold Stare, Prayer of Mending). The
// current aura expires on proc finish; when charges remain, the aura spell
// jumps to a random raid member or pet in radius with the remaining charge
// count, then the trigger spell fires on the current holder.
func (s *session) procRaidProcFromCharge(ctx context.Context, aura *activeAura, auraSpell wotlk.Spell, eff *wotlk.SpellEffect, effIndex int) {
	if s == nil || s.player == nil || aura == nil || eff == nil {
		return
	}
	withValue := eff.Aura == spellAuraRaidProcFromChargeWithValue
	var triggerSpellID uint32
	if !withValue {
		// Hardcoded jump table (SpellAuraEffects.cpp:5771-5785); unlisted
		// aura spells log and return in C++.
		switch auraSpell.ID {
		case 57949: // Shiver
			triggerSpellID = 57952
		case 59978: // Shiver
			triggerSpellID = 59979
		case 43593: // Cold Stare
			triggerSpellID = 43594
		default:
			return
		}
	} else {
		// Currently only Prayer of Mending: priest family, flags[1] & 0x20
		// (SpellAuraEffects.cpp:5815-5820).
		if auraSpell.SpellFamilyName != spellFamilyPriest || len(auraSpell.SpellFamilyFlags) < 2 || auraSpell.SpellFamilyFlags[1]&0x20 == 0 {
			return
		}
		triggerSpellID = 33110
	}
	// Capture live aura state before the expiry below drops the record.
	jumps := aura.RemainingCharges
	casterGUID := aura.CasterGUID
	spellID := aura.SpellID
	schoolMask := aura.SchoolMask
	durationMs := aura.DurationMs
	amount := aura.Amount
	if effIndex >= 0 && effIndex < len(aura.Amounts) && aura.Amounts[effIndex] > 0 {
		amount = uint32(aura.Amounts[effIndex])
	}
	// Current aura expires on proc finish (SetCharges(0) + SetUsingCharges,
	// SpellAuraEffects.cpp:5787-5788).
	s.removeAura(spellID)
	if jumps > 0 {
		// Next-target selection needs a live caster for CalcRadius
		// (SpellAuraEffects.cpp:5793-5794).
		if radius, ok := raidProcJumpRadius(s, casterGUID, eff); ok {
			if targetGUID := s.nextRaidMemberOrPetGUID(radius); targetGUID != 0 {
				// target->CastSpell(triggerTarget, GetId(), {this,
				// GetCasterGUID()}): the aura spell re-applies on the new
				// holder with the original caster. The 225 arm additionally
				// carries BP0 = the triggering amount and the original
				// caster (CastSpellExtraArgs, SpellAuraEffects.cpp:5791,
				// 5828-5830); Go's apply path takes the amount directly and
				// the explicit caster GUID.
				s.applyAuraToTarget(ctx, targetGUID, auraSpell, *eff, effIndex, durationMs, eff.AuraPeriod, amount, schoolMask, nil, false, casterGUID, false, 0, true, nil)
				// aura->SetCharges(jumps) on the freshly jumped aura
				// (SpellAuraEffects.cpp:5798-5799).
				if ts := s.server.findSessionByGUID(targetGUID); ts != nil {
					ts.castMu.Lock()
					if na := ts.activeAuras[auraSpell.ID]; na != nil && !na.Stopped {
						na.RemainingCharges = jumps
					}
					ts.castMu.Unlock()
				}
			}
		}
	}
	// target->CastSpell(target, triggerSpellId, {this, GetCasterGUID()}):
	// the trigger fires on the current holder. Go's cast path has no
	// OriginalCaster slot, so the trigger runs with the holder as caster;
	// the original caster GUID rides triggeredByAuraCasterGUID into
	// castTriggerAuraCasterGUID, where the Prayer of Mending Heal leg
	// reads it (the holder-as-caster delta otherwise stands).
	s.triggeredByAuraCast = true
	s.triggeredByAuraSpell = auraSpell.ID
	s.triggeredByAuraCasterGUID = casterGUID
	s.castSpellDirect(ctx, triggerSpellID, s.playerGUID)
	s.triggeredByAuraCast = false
	s.triggeredByAuraSpell = 0
	s.triggeredByAuraCasterGUID = 0
}

// raidProcJumpRadius resolves the jump radius for a raid-proc-from-charge
// aura: SpellInfo::Effects[effIndex].CalcRadius(caster)
// (SpellAuraEffects.cpp:5794). The jump only happens with a live caster,
// mirroring the C++ GetCaster() gate.
func raidProcJumpRadius(s *session, casterGUID uint64, eff *wotlk.SpellEffect) (float32, bool) {
	if s == nil || s.server == nil || s.server.Data == nil || s.player == nil || eff == nil || casterGUID == 0 {
		return 0, false
	}
	var level uint32
	if cs := s.server.findSessionByGUID(casterGUID); cs != nil && cs.player != nil {
		level = uint32(cs.player.Level)
	} else if uint16(casterGUID>>48) == 0xF130 {
		if m := s.server.findCreatureMotion(s.player.Map, s.player.InstanceID, casterGUID); m != nil {
			level = m.Level
		} else {
			return 0, false
		}
	} else {
		return 0, false
	}
	radius, ok, err := s.server.Data.SpellRadius(eff.RadiusIndex, level)
	if err != nil || !ok || radius <= 0 {
		return 0, false
	}
	return radius, true
}

// nextRaidMemberOrPetGUID mirrors Unit::GetNextRandomRaidMemberOrPet
// (Unit.cpp:6423): a random alive, non-hostile raid member or guardian pet
// within radius of the aura holder, excluding the holder itself. With no
// group, only the holder's pet qualifies.
func (s *session) nextRaidMemberOrPetGUID(radius float32) uint64 {
	if s == nil || s.player == nil || s.server == nil || radius <= 0 {
		return 0
	}
	var candidates []uint64
	considerPet := func(owner *session) {
		if owner == nil || owner.player == nil || owner.player.PetGUID == 0 {
			return
		}
		m := s.server.findCreatureMotion(s.player.Map, s.player.InstanceID, owner.player.PetGUID)
		if m == nil || m.Health == 0 {
			return
		}
		if dist := distance3D(s.player.X, s.player.Y, s.player.Z, m.X, m.Y, m.Z); dist <= float64(radius) {
			candidates = append(candidates, owner.player.PetGUID)
		}
	}
	if s.groupID != 0 {
		for _, m := range s.server.getGroupSessions(s.groupID) {
			if m == nil || m.player == nil || m.playerGUID == s.playerGUID {
				continue
			}
			if m.player.Map != s.player.Map || m.player.InstanceID != s.player.InstanceID {
				continue
			}
			if m.player.Health == 0 {
				continue
			}
			if distance3D(s.player.X, s.player.Y, s.player.Z, m.player.X, m.player.Y, m.player.Z) > float64(radius) {
				continue
			}
			candidates = append(candidates, m.playerGUID)
			considerPet(m)
		}
	} else {
		considerPet(s)
	}
	if len(candidates) == 0 {
		return 0
	}
	return candidates[rand.Intn(len(candidates))]
}
