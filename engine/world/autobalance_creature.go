package world

import (
	"context"
	"math"
)

// AutoBalance creature slice (AutoBalance.cpp, second slice):
//   - ForcedID*/DisabledID lists -> GetForcedNumPlayers (config wiring is in
//     engine/config; the parsed map lives on AutoBalanceConfig).
//   - AutoBalance_UnitScript::_Modifer_DealDamage, wired at the creature
//     melee and creature spell-damage fire sites. C++ also fires it from
//     ModifyPeriodicDamageAurasTick and ModifyHealRecieved; Go models no
//     creature-originated periodic ticks and no creature healing, so those
//     two arms have no fire site (documented no-bridge).
//   - AutoBalance_AllCreatureScript::ModifyCreatureAttributes, fired from
//     creature spawn (Creature_SelectLevel analog, resetSelLevel=true) and
//     from the active-creature update loop (OnAllCreatureUpdate analog).
//     The tanh count scaling, level scaling with higher/lower offsets,
//     forced-ID overrides, LevelEndGameBoost, and the base-stat rebuild are
//     C++-exact against creature_classlevelstats + creature_template; the
//     newDmgBase raw-vs-GenerateBaseDamage asymmetry is preserved verbatim.
//
// Deliberate deltas from C++:
//   - getAreaLevel's AreaTable fallback has no bridge: Go's AreaTableEntry
//     carries no ExplorationLevel, so only the LFGDungeons arm runs.
//   - LFGDungeons has no TargetLevel column, so areaMaxLvl = MaxLevel.
//   - SetStatFlatModifier has no model in Go; the ENERGY/RAGE 100 base and
//     the HEALTH/MANA flat values land directly on the motion's power and
//     max-health/mana fields.
//   - ResetPlayerDamageReq/SetLastDamagedTime have no bridge (established
//     at creaturemotion.go).
//   - SetCreateHealth/SetCreateMana fold into MaxHealth/MaxMana; the
//     POWER_MANA else-branch (SetPowerType(pType)) is a no-op since Go
//     never changes the motion's power type here.
//   - The instance difficulty is read from the first session present on
//     (map, instance); with no session present it defaults to normal.

// autoBalanceCreatureInfo mirrors AutoBalanceCreatureInfo: the per-creature
// AutoBalance state keyed by the creature's world GUID.
type autoBalanceCreatureInfo struct {
	entry               uint32
	instancePlayerCount uint32
	selectedLevel       uint8
	damageMultiplier    float64
	healthMultiplier    float64
	manaMultiplier      float64
	armorMultiplier     float64
}

func (s *Server) autoBalanceCreature(guid uint64) *autoBalanceCreatureInfo {
	s.autoBalanceMu.Lock()
	defer s.autoBalanceMu.Unlock()
	if s.autoBalanceCreatures == nil {
		s.autoBalanceCreatures = make(map[uint64]*autoBalanceCreatureInfo)
	}
	info, ok := s.autoBalanceCreatures[guid]
	if !ok {
		info = &autoBalanceCreatureInfo{
			damageMultiplier: 1,
			healthMultiplier: 1,
			manaMultiplier:   1,
			armorMultiplier:  1,
		}
		s.autoBalanceCreatures[guid] = info
	}
	return info
}

// autoBalanceForcedNumPlayers mirrors GetForcedNumPlayers: -1 when the entry
// is in no list, 0 when DisabledID covers it (no scaling), otherwise the
// forced player count.
func (s *Server) autoBalanceForcedNumPlayers(entry uint32) int {
	if s == nil {
		return -1
	}
	if n, ok := s.Config.AutoBalance.ForcedCreaturePlayers[int(entry)]; ok {
		return n
	}
	return -1
}

// autoBalanceCheckLevelOffset mirrors
// AutoBalance_AllCreatureScript::checkLevelOffset.
func autoBalanceCheckLevelOffset(selectedLevel, targetLevel uint8, higherOffset, lowerOffset int) bool {
	sel, tgt := int(selectedLevel), int(targetLevel)
	return selectedLevel != 0 && ((tgt >= sel && tgt <= sel+higherOffset) ||
		(tgt <= sel && tgt >= sel-lowerOffset))
}

// autoBalanceModifyDealDamage mirrors
// AutoBalance_UnitScript::_Modifer_DealDamage: the creature's damage
// multiplier scales damage dealt by non-player attackers. The C++
// !attacker->IsInWorld() and TYPEID_PLAYER arms are structural here (the
// attacker is always a live world creature motion).
func (s *Server) autoBalanceModifyDealDamage(motion *creatureMotion, target *session, damage uint32) uint32 {
	if s == nil || s.Data == nil || motion == nil || damage == 0 {
		return damage
	}
	cfg := s.Config.AutoBalance
	if !cfg.Enable {
		return damage
	}
	info := s.autoBalanceCreature(motion.GUID)
	if info.damageMultiplier == 1 {
		return damage
	}
	if cfg.DungeonsOnly {
		targetMapID := uint32(0)
		if target != nil && target.player != nil {
			targetMapID = target.player.Map
		}
		attackerDungeon := false
		if entry, found, err := s.Data.Map(motion.Map); err == nil && found {
			attackerDungeon = entry.IsDungeon()
		}
		targetDungeon := false
		if entry, found, err := s.Data.Map(targetMapID); err == nil && found {
			targetDungeon = entry.IsDungeon()
		}
		_, _, _, attackerBG := battlegroundTypeForMap(motion.Map)
		_, _, _, targetBG := battlegroundTypeForMap(targetMapID)
		if !((targetDungeon && attackerDungeon) || (attackerBG && targetBG)) {
			return damage
		}
	}
	// (IsHunterPet() || IsPet() || IsSummon()) && IsControlledByPlayer():
	// player-owned attackers never scale.
	if motion.PetID != 0 || motion.UnitFlags&unitFlagPlayerControlled != 0 {
		return damage
	}
	return uint32(float64(damage) * info.damageMultiplier)
}

// autoBalanceInstanceDifficulty reads the difficulty pair from the first
// session present on (mapID, instanceID); empty when nobody is there.
func (s *Server) autoBalanceInstanceDifficulty(mapID, instanceID uint32) (dungeonDiff, raidDiff uint8) {
	if s == nil {
		return 0, 0
	}
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	for sess := range s.sessions {
		if sess.worldReady.Load() && sess.player != nil &&
			sess.player.Map == mapID && sess.player.InstanceID == instanceID {
			return sess.player.DungeonDifficulty, sess.player.RaidDifficulty
		}
	}
	return 0, 0
}

type autoBalanceTemplate struct {
	minLevel                                uint32
	maxLevel                                uint32
	unitClass                               uint32
	exp                                     uint32
	rank                                    uint32
	healthMod, manaMod, armorMod, damageMod float64
	typeFlags                               uint32
}

func (s *Server) autoBalanceCreatureTemplate(ctx context.Context, entry uint32) (autoBalanceTemplate, bool) {
	var tmpl autoBalanceTemplate
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil {
		return tmpl, false
	}
	var minLevel, maxLevel, unitClass, exp, rank int64
	var typeFlags int64
	var healthMod, manaMod, armorMod, damageMod float64
	err := s.WorldStore.DB.QueryRowContext(ctx, `SELECT
		COALESCE(minlevel, 1), COALESCE(maxlevel, 1), COALESCE(unit_class, 1),
		COALESCE(exp, 0), COALESCE(rank, 0),
		COALESCE(HealthModifier, 1.0), COALESCE(ManaModifier, 1.0),
		COALESCE(ArmorModifier, 1.0), COALESCE(DamageModifier, 1.0),
		COALESCE(type_flags, 0)
		FROM creature_template WHERE entry = ?`, entry).
		Scan(&minLevel, &maxLevel, &unitClass, &exp, &rank,
			&healthMod, &manaMod, &armorMod, &damageMod, &typeFlags)
	if err != nil {
		return tmpl, false
	}
	if maxLevel < 1 {
		maxLevel = 1
	}
	if minLevel < 1 {
		minLevel = 1
	}
	if unitClass < 1 {
		unitClass = 1
	}
	tmpl = autoBalanceTemplate{
		minLevel:  uint32(minLevel),
		maxLevel:  uint32(maxLevel),
		unitClass: uint32(unitClass),
		exp:       uint32(exp),
		rank:      uint32(rank),
		healthMod: healthMod,
		manaMod:   manaMod,
		armorMod:  armorMod,
		damageMod: damageMod,
		typeFlags: uint32(typeFlags),
	}
	return tmpl, true
}

// autoBalanceBaseStats is the CreatureBaseStats row for (level, class) from
// creature_classlevelstats, the same table loadCreatureStats reads.
type autoBalanceBaseStats struct {
	baseHP    [3]int64
	baseArmor int64
	baseMana  int64
	dmg       [3]float64
}

func (s *Server) autoBalanceBaseStats(ctx context.Context, level, class uint32) (autoBalanceBaseStats, bool) {
	var st autoBalanceBaseStats
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil || level == 0 {
		return st, false
	}
	err := s.WorldStore.DB.QueryRowContext(ctx, `SELECT
		basehp0, basehp1, basehp2, basearmor, basemana, damage_base, damage_exp1, damage_exp2
		FROM creature_classlevelstats WHERE level = ? AND class = ?`, level, class).
		Scan(&st.baseHP[0], &st.baseHP[1], &st.baseHP[2], &st.baseArmor, &st.baseMana,
			&st.dmg[0], &st.dmg[1], &st.dmg[2])
	if err != nil {
		return st, false
	}
	return st, true
}

// autoBalanceBaseDamage mirrors CreatureBaseStats::GenerateBaseDamage: the
// exp-selected raw damage times the template damage modifier.
func autoBalanceBaseDamage(st autoBalanceBaseStats, exp uint32, damageMod float64) float64 {
	var raw float64
	switch {
	case exp >= 2:
		raw = st.dmg[2]
	case exp == 1:
		raw = st.dmg[1]
	default:
		raw = st.dmg[0]
	}
	return raw * damageMod
}

// autoBalanceAreaLevel mirrors getAreaLevel's LFGDungeons arm: min from
// MinLevel, max from TargetLevel (absent in Go's LFGDungeon) falling back to
// MaxLevel. The AreaTable/ExplorationLevel arm has no bridge (Go's
// AreaTableEntry carries no ExplorationLevel).
func (s *Server) autoBalanceAreaLevel(mapID uint32, difficulty uint8) (uint8, uint8) {
	s.autoBalanceMu.Lock()
	if s.autoBalanceLFG == nil {
		if s.Data != nil {
			if dungeons, err := s.Data.LFGDungeons(); err == nil {
				s.autoBalanceLFG = dungeons
			}
		}
	}
	dungeons := s.autoBalanceLFG
	s.autoBalanceMu.Unlock()
	var minLvl, maxLvl uint8
	for _, d := range dungeons {
		if d.MapID == int32(mapID) && uint32(d.Difficulty) == uint32(difficulty) {
			minLvl = uint8(d.MinLevel)
			maxLvl = uint8(d.MaxLevel)
			break
		}
	}
	return minLvl, maxLvl
}

// autoBalanceModifyCreatureAttributes mirrors
// AutoBalance_AllCreatureScript::ModifyCreatureAttributes: the tanh
// player-count scaling, forced-ID overrides, level scaling with offsets,
// and the base-stat rebuild. It runs at creature spawn (resetSelLevel=true,
// the Creature_SelectLevel analog) and on the active-creature tick
// (resetSelLevel=false, the OnAllCreatureUpdate analog); the recalc
// early-out keeps the tick cheap.
func (s *Server) autoBalanceModifyCreatureAttributes(ctx context.Context, motion *creatureMotion, resetSelLevel bool) {
	if s == nil || motion == nil || s.Data == nil {
		return
	}
	cfg := s.Config.AutoBalance
	if !cfg.Enable {
		return
	}
	mapEntry, found, err := s.Data.Map(motion.Map)
	if err != nil || !found {
		return
	}
	_, _, _, inBG := battlegroundTypeForMap(motion.Map)
	if cfg.DungeonsOnly && !mapEntry.IsDungeon() && !inBG {
		return
	}
	if motion.PetID != 0 || motion.UnitFlags&unitFlagPlayerControlled != 0 {
		return
	}

	s.autoBalanceMu.RLock()
	mapInfo := s.autoBalanceMaps[autoBalanceInstanceKey{mapID: motion.Map, instanceID: motion.InstanceID}]
	s.autoBalanceMu.RUnlock()
	if mapInfo == nil {
		return
	}
	s.autoBalanceMu.RLock()
	mapLevel, playerCount := mapInfo.mapLevel, mapInfo.playerCount
	s.autoBalanceMu.RUnlock()
	if mapLevel == 0 {
		return
	}

	tmpl, ok := s.autoBalanceCreatureTemplate(ctx, motion.Entry)
	if !ok {
		return
	}

	dungeonDiff, raidDiff := s.autoBalanceInstanceDifficulty(motion.Map, motion.InstanceID)
	difficulty := dungeonDiff
	isHeroic := dungeonDiff == 1
	if mapEntry.IsRaid() {
		difficulty = raidDiff
		isHeroic = raidDiff == 2 || raidDiff == 3
	}
	maxPlayers := autoBalanceInstanceMaxPlayers(ctx, s, mapEntry, difficulty)

	forcedNumPlayers := s.autoBalanceForcedNumPlayers(motion.Entry)
	if forcedNumPlayers > 0 {
		maxPlayers = uint32(forcedNumPlayers)
	} else if forcedNumPlayers == 0 {
		return
	}

	info := s.autoBalanceCreature(motion.GUID)
	s.autoBalanceMu.Lock()
	if (info.entry != 0 && info.entry != motion.Entry) || resetSelLevel {
		info.selectedLevel = 0
	}
	s.autoBalanceMu.Unlock()

	if motion.Health == 0 {
		return
	}

	curCount := int64(playerCount) + int64(cfg.PlayerCountDifficultyOffset)
	var bonusLevel uint8
	if tmpl.rank == 3 { // CREATURE_ELITE_WORLDBOSS
		bonusLevel = 3
	}

	level := mapLevel
	s.autoBalanceMu.RLock()
	selectedLevel := info.selectedLevel
	knownCount := info.instancePlayerCount
	s.autoBalanceMu.RUnlock()
	if selectedLevel > 0 {
		if cfg.LevelScaling != 0 {
			if autoBalanceCheckLevelOffset(level+bonusLevel, uint8(motion.Level), cfg.LevelHigherOffset, cfg.LevelLowerOffset) &&
				autoBalanceCheckLevelOffset(selectedLevel, uint8(motion.Level), cfg.LevelHigherOffset, cfg.LevelLowerOffset) &&
				knownCount == uint32(curCount) {
				return
			}
		} else if knownCount == uint32(curCount) {
			return
		}
	}
	if curCount <= 0 {
		return
	}
	s.autoBalanceMu.Lock()
	info.instancePlayerCount = uint32(curCount)
	s.autoBalanceMu.Unlock()

	originalLevel := tmpl.maxLevel
	areaMinLvl, areaMaxLvl := s.autoBalanceAreaLevel(motion.Map, difficulty)

	skipLevel := originalLevel <= 1 && areaMinLvl >= 5

	if cfg.LevelScaling != 0 && mapEntry.IsDungeon() && !skipLevel &&
		!autoBalanceCheckLevelOffset(level, uint8(originalLevel), cfg.LevelHigherOffset, cfg.LevelLowerOffset) {
		if level != selectedLevel || selectedLevel != uint8(motion.Level) {
			s.autoBalanceMu.Lock()
			info.selectedLevel = level + bonusLevel
			selectedLevel = info.selectedLevel
			s.autoBalanceMu.Unlock()
			motion.Level = uint32(selectedLevel)
		}
	} else {
		s.autoBalanceMu.Lock()
		info.selectedLevel = uint8(motion.Level)
		selectedLevel = info.selectedLevel
		s.autoBalanceMu.Unlock()
	}
	s.autoBalanceMu.Lock()
	info.entry = motion.Entry
	s.autoBalanceMu.Unlock()

	useDefStats := false
	if cfg.LevelUseDb && motion.Level >= tmpl.minLevel && motion.Level <= tmpl.maxLevel {
		useDefStats = true
	}

	orig, ok := s.autoBalanceBaseStats(ctx, originalLevel, tmpl.unitClass)
	if !ok {
		return
	}
	var origHP float64
	switch {
	case tmpl.exp >= 2:
		origHP = float64(orig.baseHP[2])
	case tmpl.exp == 1:
		origHP = float64(orig.baseHP[1])
	default:
		origHP = float64(orig.baseHP[0])
	}
	baseHealth := origHP * tmpl.healthMod             // GenerateHealth
	baseMana := float64(orig.baseMana) * tmpl.manaMod // GenerateMana
	origDmgBase := autoBalanceBaseDamage(orig, tmpl.exp, tmpl.damageMod)

	defaultMultiplier := 1.0
	if uint32(curCount) < maxPlayers {
		inflectionValue := float64(maxPlayers)
		if isHeroic {
			if mapEntry.IsRaid() {
				switch maxPlayers {
				case 10:
					inflectionValue *= cfg.InflectionPointRaid10MHeroic
				case 25:
					inflectionValue *= cfg.InflectionPointRaid25MHeroic
				default:
					inflectionValue *= cfg.InflectionPointRaidHeroic
				}
			} else {
				inflectionValue *= cfg.InflectionPointHeroic
			}
		} else if mapEntry.IsRaid() {
			switch maxPlayers {
			case 10:
				inflectionValue *= cfg.InflectionPointRaid10M
			case 25:
				inflectionValue *= cfg.InflectionPointRaid25M
			default:
				inflectionValue *= cfg.InflectionPointRaid
			}
		} else {
			inflectionValue *= cfg.InflectionPoint
		}
		if tmpl.typeFlags&0x4 != 0 { // CREATURE_TYPE_FLAG_BOSS_MOB
			inflectionValue *= cfg.BossInflectionMult
		}
		diff := (float64(maxPlayers) / 5) * 1.5
		defaultMultiplier = (math.Tanh((float64(curCount)-inflectionValue)/diff) + 1.0) / 2.0
	}

	healthMult := cfg.HealthMultiplier * defaultMultiplier * cfg.GlobalRate
	if healthMult <= cfg.MinHPModifier {
		healthMult = cfg.MinHPModifier
	}
	s.autoBalanceMu.Lock()
	info.healthMultiplier = healthMult
	s.autoBalanceMu.Unlock()

	hpStatsRate := 1.0
	if !useDefStats && cfg.LevelScaling != 0 && !skipLevel {
		if scaled, ok := s.autoBalanceBaseStats(ctx, uint32(selectedLevel), tmpl.unitClass); ok {
			var newBaseHealth float64
			switch {
			case level <= 60:
				newBaseHealth = float64(scaled.baseHP[0])
			case level <= 70:
				newBaseHealth = float64(scaled.baseHP[1])
			default:
				newBaseHealth = float64(scaled.baseHP[2])
				if cfg.LevelEndGameBoost && selectedLevel >= 75 && originalLevel < 75 {
					newBaseHealth *= float64(int(selectedLevel)-70) * 0.3
				}
			}
			newHealth := newBaseHealth * tmpl.healthMod
			if originalLevel >= uint32(areaMinLvl) && originalLevel < uint32(areaMaxLvl) && areaMaxLvl > areaMinLvl {
				reduction := newHealth / float64(areaMaxLvl-areaMinLvl) * (float64(uint32(areaMaxLvl)-originalLevel) * 0.3)
				if reduction > 0 && reduction < newHealth {
					newHealth -= reduction
				}
			}
			if baseHealth > 0 {
				hpStatsRate = newHealth / baseHealth
			}
		}
	}
	s.autoBalanceMu.Lock()
	info.healthMultiplier *= hpStatsRate
	healthMultiplier := info.healthMultiplier
	s.autoBalanceMu.Unlock()
	scaledHealth := uint32(math.Round(baseHealth*healthMultiplier + 1.0))

	manaStatsRate := 1.0
	if !useDefStats && cfg.LevelScaling != 0 && !skipLevel {
		if scaled, ok := s.autoBalanceBaseStats(ctx, uint32(selectedLevel), tmpl.unitClass); ok {
			newMana := float64(scaled.baseMana) * tmpl.manaMod
			if baseMana > 0 {
				manaStatsRate = newMana / baseMana
			}
		}
	}
	manaMult := manaStatsRate * cfg.ManaMultiplier * defaultMultiplier * cfg.GlobalRate
	if manaMult <= cfg.MinManaModifier {
		manaMult = cfg.MinManaModifier
	}
	s.autoBalanceMu.Lock()
	info.manaMultiplier = manaMult
	s.autoBalanceMu.Unlock()
	scaledMana := uint32(math.Round(baseMana * manaMult))

	damageMul := defaultMultiplier * cfg.GlobalRate * cfg.DamageMultiplier
	if damageMul <= cfg.MinDamageModifier {
		damageMul = cfg.MinDamageModifier
	}
	if !useDefStats && cfg.LevelScaling != 0 && !skipLevel {
		if scaled, ok := s.autoBalanceBaseStats(ctx, uint32(selectedLevel), tmpl.unitClass); ok {
			// C++-exact asymmetry: origDmgBase carries GenerateBaseDamage's
			// DamageModifier while newDmgBase uses the raw BaseDamage row.
			var newDmgBase float64
			switch {
			case level <= 60:
				newDmgBase = scaled.dmg[0]
			case level <= 70:
				newDmgBase = scaled.dmg[1]
			default:
				newDmgBase = scaled.dmg[2]
				if cfg.LevelEndGameBoost && !mapEntry.IsRaid() && selectedLevel >= 75 && originalLevel < 75 {
					newDmgBase *= float64(int(selectedLevel)-70) * 0.3
				}
			}
			if origDmgBase > 0 {
				damageMul *= newDmgBase / origDmgBase
			}
		}
	}
	s.autoBalanceMu.Lock()
	info.damageMultiplier = damageMul
	s.autoBalanceMu.Unlock()

	armorMult := defaultMultiplier * cfg.GlobalRate * cfg.ArmorMultiplier
	var armorBase float64
	if useDefStats || cfg.LevelScaling == 0 || skipLevel {
		armorBase = float64(orig.baseArmor) * tmpl.armorMod
	} else if scaled, ok := s.autoBalanceBaseStats(ctx, uint32(selectedLevel), tmpl.unitClass); ok {
		armorBase = float64(scaled.baseArmor) * tmpl.armorMod
	} else {
		armorBase = float64(orig.baseArmor) * tmpl.armorMod
	}
	newBaseArmor := uint32(math.Round(armorMult * armorBase))
	s.autoBalanceMu.Lock()
	info.armorMultiplier = armorMult
	s.autoBalanceMu.Unlock()

	prevMaxHealth := motion.MaxHealth
	prevHealth := motion.Health
	prevMaxMana := motion.MaxMana
	prevMana := motion.Powers[0]

	motion.Armor = newBaseArmor
	motion.MaxHealth = scaledHealth
	// SetStatFlatModifier(UNIT_MOD_HEALTH/UNIT_MOD_MANA, BASE_VALUE) has no
	// model in Go: the scaled values land directly on the motion fields.
	if prevHealth != 0 && prevMaxHealth != 0 {
		motion.Health = uint32(float64(scaledHealth) / float64(prevMaxHealth) * float64(prevHealth))
	} else {
		motion.Health = 0
	}
	motion.MaxMana = scaledMana
	motion.MaxPowers[1] = 100  // SetStatFlatModifier(UNIT_MOD_RAGE, BASE_VALUE, 100)
	motion.MaxPowers[3] = 100  // SetStatFlatModifier(UNIT_MOD_ENERGY, BASE_VALUE, 100)
	if motion.PowerType == 0 { // POWER_MANA
		if prevMana != 0 && prevMaxMana != 0 {
			motion.Powers[0] = uint32(float64(scaledMana) / float64(prevMaxMana) * float64(prevMana))
		} else {
			motion.Powers[0] = 0
		}
	}
	// else branch (SetPowerType(pType)) is a no-op: the power type is unchanged.

	s.broadcastCreatureValuesUpdateInInstance(motion.Map, motion.InstanceID, motion.GUID, map[int]uint32{
		unitFieldLevel:       motion.Level,
		unitFieldMaxHealth:   motion.MaxHealth,
		unitFieldHealth:      motion.Health,
		unitFieldResistances: motion.Armor,
		unitFieldMaxPower1:   motion.MaxPowers[0],
		unitFieldPower1:      motion.Powers[0],
	})
}
