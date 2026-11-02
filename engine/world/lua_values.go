package world

import (
	"context"
	"fmt"
	"math"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
)

// Indexed-value access (Object::Get/Set*Value, ObjectMethods.h) over the
// named fields Go models, following the player flag-trio precedent: a slot
// with a live backing field reads/writes it, a slot without one reads 0
// and ignores writes. C++ stores every slot in one uint32 block, so the
// typed accessors are bit reinterpretations of the slot value: float
// slots read/write float32 bits, byte/uint16 slots read/write sub-fields
// of the slot, uint64 slots span two adjacent slots. Set* writes are
// change-only with a values-update broadcast (Object.cpp Set* arms);
// UpdateUInt32Value broadcasts even when unchanged (its C++ arm marks
// dirty unconditionally). Out-of-range byte/uint16 offsets error: C++
// asserts on them in debug, and the release read would be adjacent-slot
// garbage, so failing fast beats returning it.
// Gameobject indexed-value field indices (UpdateFields.h, OBJECT_END
// = 6): the slots lua_values.go's GO backend reads/writes that have
// no existing Go constant. DISPLAYID/FLAGS/BYTES_1 already have
// gameObject* constants in gameobjects.go.
const (
	gameObjectFieldCreatedBy      = 6
	gameObjectFieldParentRotation = 10
	gameObjectFieldFaction        = 15
)

type indexedValueBackend struct {
	read      func(index int) (uint32, bool)
	write     func(index int, update func(old uint32) uint32) (map[int]uint32, bool, bool)
	broadcast func(fields map[int]uint32)
}

func replaceValueByte(old uint32, offset int, value uint8) uint32 {
	shift := uint(offset * 8)
	return old&^(uint32(0xFF)<<shift) | uint32(value)<<shift
}

func replaceValueU16(old uint32, offset int, value uint16) uint32 {
	shift := uint(offset * 16)
	return old&^(uint32(0xFFFF)<<shift) | uint32(value)<<shift
}

func luaGenericFloat32Arg(args []any, index int) (float32, error) {
	if index >= len(args) {
		return 0, fmt.Errorf("argument %d is required", index+1)
	}
	var value float64
	switch current := args[index].(type) {
	case float64:
		value = current
	case float32:
		value = float64(current)
	case int:
		value = float64(current)
	case int64:
		value = float64(current)
	case uint32:
		value = float64(current)
	case uint64:
		value = float64(current)
	default:
		return 0, fmt.Errorf("argument %d must be number", index+1)
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("argument %d must be finite", index+1)
	}
	return float32(value), nil
}

func luaInt32Arg(args []any, index int) (int32, error) {
	value, err := luaInt64Arg(args, index)
	if err != nil {
		return 0, err
	}
	if value < math.MinInt32 || value > math.MaxInt32 {
		return 0, fmt.Errorf("argument %d must be int32", index+1)
	}
	return int32(value), nil
}

func registerIndexedValueMethods(methods map[string]scripting.ObjectMethod, backend indexedValueBackend) {
	setSlot := func(index int, value uint32, always bool) error {
		fields, changed, ok := backend.write(index, func(uint32) uint32 { return value })
		if !ok {
			return nil
		}
		if (always || changed) && len(fields) > 0 {
			backend.broadcast(fields)
		}
		return nil
	}
	setSubSlot := func(index, offset, width int, value uint32) error {
		fields, changed, ok := backend.write(index, func(old uint32) uint32 {
			if width == 1 {
				return replaceValueByte(old, offset, uint8(value))
			}
			return replaceValueU16(old, offset, uint16(value))
		})
		if !ok {
			return nil
		}
		if changed && len(fields) > 0 {
			backend.broadcast(fields)
		}
		return nil
	}
	methods["GetInt32Value"] = func(_ context.Context, args []any) ([]any, error) {
		index, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		value, _ := backend.read(int(index))
		return []any{int32(value)}, nil
	}
	methods["GetUInt32Value"] = func(_ context.Context, args []any) ([]any, error) {
		index, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		value, _ := backend.read(int(index))
		return []any{value}, nil
	}
	methods["GetFloatValue"] = func(_ context.Context, args []any) ([]any, error) {
		index, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		value, _ := backend.read(int(index))
		return []any{float64(math.Float32frombits(value))}, nil
	}
	methods["GetByteValue"] = func(_ context.Context, args []any) ([]any, error) {
		index, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		offset, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		if offset >= 4 {
			return []any{uint32(0)}, nil
		}
		value, _ := backend.read(int(index))
		return []any{uint32(uint8(value >> (offset * 8)))}, nil
	}
	methods["GetUInt16Value"] = func(_ context.Context, args []any) ([]any, error) {
		index, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		offset, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		if offset >= 2 {
			return []any{uint32(0)}, nil
		}
		value, _ := backend.read(int(index))
		return []any{uint32(uint16(value >> (offset * 16)))}, nil
	}
	methods["GetUInt64Value"] = func(_ context.Context, args []any) ([]any, error) {
		index, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		lo, _ := backend.read(int(index))
		hi, _ := backend.read(int(index) + 1)
		return []any{uint64(lo) | uint64(hi)<<32}, nil
	}
	methods["SetInt32Value"] = func(_ context.Context, args []any) ([]any, error) {
		index, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		value, err := luaInt32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		return nil, setSlot(int(index), uint32(value), false)
	}
	methods["SetUInt32Value"] = func(_ context.Context, args []any) ([]any, error) {
		index, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		value, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		return nil, setSlot(int(index), value, false)
	}
	methods["UpdateUInt32Value"] = func(_ context.Context, args []any) ([]any, error) {
		index, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		value, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		return nil, setSlot(int(index), value, true)
	}
	methods["SetFloatValue"] = func(_ context.Context, args []any) ([]any, error) {
		index, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		value, err := luaGenericFloat32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		return nil, setSlot(int(index), math.Float32bits(value), false)
	}
	methods["SetByteValue"] = func(_ context.Context, args []any) ([]any, error) {
		index, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		offset, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		if offset >= 4 {
			return nil, fmt.Errorf("argument 2 must be a byte offset 0-3")
		}
		value, err := luaUint32Arg(args, 2)
		if err != nil {
			return nil, err
		}
		if value > math.MaxUint8 {
			return nil, fmt.Errorf("argument 3 must be uint8")
		}
		return nil, setSubSlot(int(index), int(offset), 1, value)
	}
	methods["SetUInt16Value"] = func(_ context.Context, args []any) ([]any, error) {
		index, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		offset, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		if offset >= 2 {
			return nil, fmt.Errorf("argument 2 must be a uint16 offset 0-1")
		}
		value, err := luaUint32Arg(args, 2)
		if err != nil {
			return nil, err
		}
		if value > math.MaxUint16 {
			return nil, fmt.Errorf("argument 3 must be uint16")
		}
		return nil, setSubSlot(int(index), int(offset), 2, value)
	}
	methods["SetInt16Value"] = func(_ context.Context, args []any) ([]any, error) {
		index, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		offset, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		if offset >= 2 {
			return nil, fmt.Errorf("argument 2 must be a uint16 offset 0-1")
		}
		value, err := luaInt64Arg(args, 2)
		if err != nil {
			return nil, err
		}
		if value < math.MinInt16 || value > math.MaxInt16 {
			return nil, fmt.Errorf("argument 3 must be int16")
		}
		return nil, setSubSlot(int(index), int(offset), 2, uint32(uint16(int16(value))))
	}
	methods["SetUInt64Value"] = func(_ context.Context, args []any) ([]any, error) {
		index, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if len(args) < 2 {
			return nil, fmt.Errorf("argument 2 is required")
		}
		value, err := luaUint64(args[1])
		if err != nil {
			return nil, err
		}
		loFields, loChanged, loOK := backend.write(int(index), func(uint32) uint32 { return uint32(value) })
		hiFields, hiChanged, hiOK := backend.write(int(index)+1, func(uint32) uint32 { return uint32(value >> 32) })
		if !loOK && !hiOK {
			return nil, nil
		}
		if loChanged || hiChanged {
			merged := map[int]uint32{}
			for slot, field := range loFields {
				merged[slot] = field
			}
			for slot, field := range hiFields {
				merged[slot] = field
			}
			if len(merged) > 0 {
				backend.broadcast(merged)
			}
		}
		return nil, nil
	}
}

func playerLuaDisplayID(s *session, state *playerState) uint32 {
	displayID := uint32(0)
	if s.server != nil && s.server.Data != nil {
		if race, found, err := s.server.Data.Race(uint32(state.Race)); err == nil && found {
			displayID = race.MaleDisplayID
			if state.Gender != 0 {
				displayID = race.FemaleDisplayID
			}
		}
	}
	if state.TransformDisplayID != 0 {
		displayID = state.TransformDisplayID
	}
	return displayID
}

func playerLuaNativeDisplayID(s *session, state *playerState) uint32 {
	displayID := uint32(0)
	if s.server != nil && s.server.Data != nil {
		if race, found, err := s.server.Data.Race(uint32(state.Race)); err == nil && found {
			displayID = race.MaleDisplayID
			if state.Gender != 0 {
				displayID = race.FemaleDisplayID
			}
		}
	}
	return displayID
}

// Player indexed values: every slot Go already sends in the player
// values-update builder (player_state.go) reads that same source, so a
// script read matches the wire. Identity and derived slots (GUID, type,
// entry, race/class/gender/power bytes, race faction, native display,
// next-level XP, rest XP, packet-derived bytes, config max level,
// builder-constant attack times and bounding radius) read real values
// but ignore writes: no backing store exists to take them. Player scale
// reads the session scale slot while set, else the aura fold, and writes
// go to the session slot exactly like SetScale (no broadcast either way).
func playerIndexedValueBackend(s *session, state *playerState) indexedValueBackend {
	read := func(index int) (uint32, bool) {
		switch index {
		case 0:
			return uint32(state.GUID), true
		case 1:
			return uint32(state.GUID >> 32), true
		case objectFieldType:
			return 4, true
		case objectFieldEntry:
			return 0, true
		case objectFieldScale:
			if s.scale != 0 {
				return math.Float32bits(s.scale), true
			}
			return math.Float32bits(s.auraObjectScale()), true
		case 10:
			return uint32(state.CritterGUID), true
		case 11:
			return uint32(state.CritterGUID >> 32), true
		case unitFieldTarget:
			return uint32(state.Selection), true
		case unitFieldTarget + 1:
			return uint32(state.Selection >> 32), true
		case unitFieldBytes0:
			return uint32(state.Race) | uint32(state.Class)<<8 | uint32(state.Gender)<<16 | uint32(playerPowerType(state))<<24, true
		case unitFieldHealth:
			return state.Health, true
		case unitFieldMaxHealth:
			return state.MaxHealth, true
		case unitFieldLevel:
			return uint32(state.Level), true
		case unitFieldFaction:
			if s.server != nil {
				return s.server.raceFaction(state.Race), true
			}
			return 0, true
		case unitFieldFlags:
			return state.UnitFlags, true
		case unitFieldFlags2:
			return state.UnitFlags2, true
		case unitFieldAttackTime, unitFieldAttackTimeOffhand:
			return 2000, true
		case unitFieldBoundingRadius:
			return math.Float32bits(0.306349), true
		case unitFieldCombatReach:
			return math.Float32bits(state.CombatReach), true
		case unitFieldDisplayID:
			return playerLuaDisplayID(s, state), true
		case unitFieldNativeDisplayID:
			return playerLuaNativeDisplayID(s, state), true
		case unitFieldMountDisplayID:
			return state.MountDisplayID, true
		case unitFieldBytes1:
			return uint32(state.StandState) | uint32(state.StandFlags)<<16, true
		case unitFieldNPCEmoteState:
			return state.EmoteState, true
		case unitFieldBytes2:
			return uint32(state.SheathState) | uint32(state.PVPFlags)<<8 | uint32(state.ShapeshiftForm)<<24, true
		case unitFieldPlayerFlags:
			return state.PlayerFlags, true
		case unitFieldGuildID:
			return state.GuildID, true
		case unitFieldGuildRank:
			return uint32(state.GuildRank), true
		case unitFieldPlayerBytes:
			return uint32(state.Skin) | uint32(state.Face)<<8 | uint32(state.HairStyle)<<16 | uint32(state.HairColor)<<24, true
		case unitFieldPlayerBytes2:
			return uint32(state.FacialStyle) | uint32(state.BankBagSlots)<<16 | uint32(state.RestState)<<24, true
		case unitFieldPlayerBytes3:
			return uint32(state.Gender) | uint32(uint8(state.DrunkenState))<<8, true
		case unitFieldChosenTitle:
			return state.ChosenTitle, true
		case playerFieldFakeInebriation:
			return state.FakeInebriation, true
		case unitFieldBaseMana:
			return state.BaseMana, true
		case unitFieldPlayerFieldBytes:
			return playerFieldBytesValue(*state), true
		case unitFieldPlayerSelfResSpell:
			return state.SelfResSpell, true
		case unitFieldAmmoID:
			return state.AmmoID, true
		case unitFieldWatchedFaction:
			return state.WatchedFaction, true
		case playerFieldBytes2:
			return uint32(state.AuraVision) << 24, true
		case unitFieldMaxLevel:
			maxLevel := uint32(80)
			if s.server != nil && s.server.Config.MaxPlayerLevel > 0 && s.server.Config.MaxPlayerLevel <= 100 {
				maxLevel = s.server.Config.MaxPlayerLevel
			}
			return maxLevel, true
		case playerFieldDuelArbiter:
			return uint32(state.DuelArbiter), true
		case playerFieldDuelArbiter + 1:
			return uint32(state.DuelArbiter >> 32), true
		case playerFieldDuelTeam:
			return state.DuelTeam, true
		case unitFieldXP:
			return state.XP, true
		case unitFieldNextLevelXP:
			if state.Level > 0 && int(state.Level) < len(xpCurve) {
				return playerNextLevelXP(state.Level), true
			}
			return 0, true
		case unitFieldCoinage:
			return state.Money, true
		case playerFieldRestStateExperience:
			if state.RestBonus > 0 {
				return uint32(state.RestBonus), true
			}
			return 0, true
		case playerFieldFarsight:
			return uint32(state.FarsightGUID), true
		case playerFieldFarsight + 1:
			return uint32(state.FarsightGUID >> 32), true
		case unitFieldKnownCurrencies:
			return uint32(state.KnownCurrency), true
		case unitFieldKnownCurrencies + 1:
			return uint32(state.KnownCurrency >> 32), true
		}
		if index >= unitFieldPower1 && index < unitFieldPower1+7 {
			return state.Powers[index-unitFieldPower1], true
		}
		if index >= unitFieldMaxPower1 && index < unitFieldMaxPower1+7 {
			return state.MaxPowers[index-unitFieldMaxPower1], true
		}
		if index >= unitFieldStat0 && index < unitFieldStat0+5 {
			return state.Stats[index-unitFieldStat0], true
		}
		if index >= unitFieldResistances && index < unitFieldResistances+7 {
			return state.Resistances[index-unitFieldResistances], true
		}
		if index >= playerFieldKnownTitles && index < playerFieldKnownTitles+6 {
			return state.KnownTitles[index-playerFieldKnownTitles], true
		}
		if index >= playerExploredZonesStart && index < playerExploredZonesStart+playerExploredZonesCount {
			return state.ExploredZones[index-playerExploredZonesStart], true
		}
		return 0, false
	}
	write := func(index int, update func(old uint32) uint32) (map[int]uint32, bool, bool) {
		old, ok := read(index)
		if !ok {
			return nil, false, false
		}
		value := update(old)
		switch index {
		case objectFieldScale:
			s.scale = math.Float32frombits(value)
		case unitFieldTarget:
			state.Selection = state.Selection&0xFFFFFFFF00000000 | uint64(value)
		case unitFieldTarget + 1:
			state.Selection = state.Selection&0xFFFFFFFF | uint64(value)<<32
		case unitFieldHealth:
			state.Health = value
		case unitFieldMaxHealth:
			state.MaxHealth = value
		case unitFieldLevel:
			state.Level = uint8(value)
		case unitFieldFlags:
			state.UnitFlags = value
		case unitFieldFlags2:
			state.UnitFlags2 = value
		case unitFieldCombatReach:
			state.CombatReach = math.Float32frombits(value)
		case unitFieldDisplayID:
			state.TransformDisplayID = value
		case unitFieldMountDisplayID:
			state.MountDisplayID = value
		case unitFieldBytes1:
			state.StandState = uint8(value)
			state.StandFlags = uint8(value >> 16)
		case unitFieldNPCEmoteState:
			state.EmoteState = value
		case unitFieldBytes2:
			state.SheathState = uint8(value)
			state.PVPFlags = uint8(value >> 8)
			state.ShapeshiftForm = uint8(value >> 24)
		case unitFieldPlayerFlags:
			state.PlayerFlags = value
		case unitFieldGuildID:
			state.GuildID = value
		case unitFieldGuildRank:
			state.GuildRank = uint8(value)
		case unitFieldPlayerBytes:
			state.Skin = uint8(value)
			state.Face = uint8(value >> 8)
			state.HairStyle = uint8(value >> 16)
			state.HairColor = uint8(value >> 24)
		case unitFieldPlayerBytes2:
			state.FacialStyle = uint8(value)
			state.BankBagSlots = uint8(value >> 16)
			state.RestState = uint8(value >> 24)
		case unitFieldPlayerBytes3:
			state.Gender = uint8(value)
			state.DrunkenState = state.DrunkenState&0xFF00 | uint16(uint8(value>>8))
		case unitFieldChosenTitle:
			state.ChosenTitle = value
		case playerFieldFakeInebriation:
			state.FakeInebriation = value
		case unitFieldBaseMana:
			state.BaseMana = value
		case unitFieldPlayerSelfResSpell:
			state.SelfResSpell = value
		case unitFieldAmmoID:
			state.AmmoID = value
		case unitFieldWatchedFaction:
			state.WatchedFaction = value
		case playerFieldBytes2:
			state.AuraVision = uint8(value >> 24)
		case playerFieldDuelArbiter:
			state.DuelArbiter = state.DuelArbiter&0xFFFFFFFF00000000 | uint64(value)
		case playerFieldDuelArbiter + 1:
			state.DuelArbiter = state.DuelArbiter&0xFFFFFFFF | uint64(value)<<32
		case playerFieldDuelTeam:
			state.DuelTeam = value
		case unitFieldXP:
			state.XP = value
		case unitFieldCoinage:
			state.Money = value
		case playerFieldFarsight:
			state.FarsightGUID = state.FarsightGUID&0xFFFFFFFF00000000 | uint64(value)
		case playerFieldFarsight + 1:
			state.FarsightGUID = state.FarsightGUID&0xFFFFFFFF | uint64(value)<<32
		case unitFieldKnownCurrencies:
			state.KnownCurrency = state.KnownCurrency&0xFFFFFFFF00000000 | uint64(value)
		case unitFieldKnownCurrencies + 1:
			state.KnownCurrency = state.KnownCurrency&0xFFFFFFFF | uint64(value)<<32
		default:
			if index >= unitFieldPower1 && index < unitFieldPower1+7 {
				state.Powers[index-unitFieldPower1] = value
			} else if index >= unitFieldMaxPower1 && index < unitFieldMaxPower1+7 {
				state.MaxPowers[index-unitFieldMaxPower1] = value
			} else if index >= unitFieldStat0 && index < unitFieldStat0+5 {
				state.Stats[index-unitFieldStat0] = value
			} else if index >= unitFieldResistances && index < unitFieldResistances+7 {
				state.Resistances[index-unitFieldResistances] = value
			} else if index >= playerFieldKnownTitles && index < playerFieldKnownTitles+6 {
				state.KnownTitles[index-playerFieldKnownTitles] = value
			} else if index >= playerExploredZonesStart && index < playerExploredZonesStart+playerExploredZonesCount {
				state.ExploredZones[index-playerExploredZonesStart] = value
			} else {
				return nil, false, false
			}
		}
		stored, _ := read(index)
		if index == objectFieldScale {
			if stored == old {
				return nil, false, true
			}
			return nil, true, true
		}
		return map[int]uint32{index: stored}, stored != old, true
	}
	return indexedValueBackend{
		read:      read,
		write:     write,
		broadcast: func(fields map[int]uint32) { s.sendPlayerValuesUpdate(fields) },
	}
}

// Motion-creature indexed values: every slot the motion record carries
// reads/writes it, with the Known-flag twins the spawn overlay gates on
// (DisplayID, Mount, NPCFlags, EmoteState, StandState, SheathState, and
// the Bytes1 cache) set on write so rebuilds keep the scripted value.
// Power 0 mirrors Mana/MaxMana alongside Powers[0]/MaxPowers[0]. Scale
// reads the GetScale base and writes through the SetScale field effect,
// broadcasting the aura-folded effective value exactly like SetScale.
// Identity slots (GUID, type) and the entry slot (UpdateEntry owns it)
// read real values but ignore writes.
func motionIndexedValueBackend(s *Server, motion *creatureMotion) indexedValueBackend {
	readLocked := func(index int) (uint32, bool) {
		switch index {
		case 0:
			return uint32(motion.GUID), true
		case 1:
			return uint32(motion.GUID >> 32), true
		case objectFieldType:
			return 3, true
		case objectFieldEntry:
			return motion.Entry, true
		case objectFieldScale:
			if motion.ScriptScaleKnown {
				return math.Float32bits(motion.ScriptScale), true
			}
			if motion.BaseScale > 0 {
				return math.Float32bits(motion.BaseScale), true
			}
			return math.Float32bits(1), true
		case unitFieldTarget:
			return uint32(motion.ScriptTargetGUID), true
		case unitFieldTarget + 1:
			return uint32(motion.ScriptTargetGUID >> 32), true
		case unitFieldBytes0:
			return uint32(motion.PowerType) << 24, true
		case unitFieldHealth:
			return motion.Health, true
		case unitFieldMaxHealth:
			return motion.MaxHealth, true
		case unitFieldLevel:
			return motion.Level, true
		case unitFieldFaction:
			return motion.Faction, true
		case unitFieldFlags:
			return motion.UnitFlags, true
		case unitFieldFlags2:
			return motion.UnitFlags2, true
		case unitFieldAttackTime:
			return motion.AttackTime, true
		case unitFieldCombatReach:
			return math.Float32bits(motion.CombatReach), true
		case unitFieldDisplayID:
			return motion.DisplayID, true
		case unitFieldMountDisplayID:
			return motion.Mount, true
		case unitFieldMinDamage:
			return math.Float32bits(motion.MinDamage), true
		case unitFieldMaxDamage:
			return math.Float32bits(motion.MaxDamage), true
		case unitFieldBytes1:
			value := uint32(0)
			if motion.bytes1CacheKnown {
				value = motion.bytes1Cache
			}
			if motion.StandStateKnown {
				value = value&^0xFF | uint32(motion.StandState)
			}
			return value, true
		case unitFieldDynamicFlags:
			return motion.DynamicFlags, true
		case unitFieldNPCFlags:
			return motion.NPCFlags, true
		case unitFieldNPCEmoteState:
			return motion.EmoteState, true
		case unitFieldBytes2:
			return uint32(motion.SheathState), true
		}
		if index >= unitFieldPower1 && index < unitFieldPower1+7 {
			return motion.Powers[index-unitFieldPower1], true
		}
		if index >= unitFieldMaxPower1 && index < unitFieldMaxPower1+7 {
			return motion.MaxPowers[index-unitFieldMaxPower1], true
		}
		if index >= unitFieldStat0 && index < unitFieldStat0+5 {
			return motion.Stats[index-unitFieldStat0], true
		}
		if index >= unitFieldResistances && index < unitFieldResistances+7 {
			return motion.Resistances[index-unitFieldResistances], true
		}
		return 0, false
	}
	read := func(index int) (uint32, bool) {
		s.motionMu.Lock()
		defer s.motionMu.Unlock()
		return readLocked(index)
	}
	write := func(index int, update func(old uint32) uint32) (map[int]uint32, bool, bool) {
		s.motionMu.Lock()
		old, ok := readLocked(index)
		if !ok {
			s.motionMu.Unlock()
			return nil, false, false
		}
		value := update(old)
		switch index {
		case objectFieldScale:
			scale := math.Float32frombits(value)
			if scale <= 0 {
				motion.ScriptScale = 0
				motion.ScriptScaleKnown = false
			} else {
				motion.ScriptScale = scale
				motion.ScriptScaleKnown = true
			}
		case unitFieldTarget:
			motion.ScriptTargetGUID = motion.ScriptTargetGUID&0xFFFFFFFF00000000 | uint64(value)
			motion.ScriptTargetKnown = true
		case unitFieldTarget + 1:
			motion.ScriptTargetGUID = motion.ScriptTargetGUID&0xFFFFFFFF | uint64(value)<<32
			motion.ScriptTargetKnown = true
		case unitFieldBytes0:
			motion.PowerType = uint32(uint8(value >> 24))
		case unitFieldHealth:
			motion.Health = value
		case unitFieldMaxHealth:
			motion.MaxHealth = value
		case unitFieldLevel:
			motion.Level = value
		case unitFieldFaction:
			motion.Faction = value
		case unitFieldFlags:
			motion.UnitFlags = value
		case unitFieldFlags2:
			motion.UnitFlags2 = value
		case unitFieldAttackTime:
			motion.AttackTime = value
		case unitFieldCombatReach:
			motion.CombatReach = math.Float32frombits(value)
		case unitFieldDisplayID:
			motion.DisplayID = value
			motion.DisplayIDKnown = true
		case unitFieldMountDisplayID:
			motion.Mount = value
			motion.MountKnown = true
		case unitFieldMinDamage:
			motion.MinDamage = math.Float32frombits(value)
		case unitFieldMaxDamage:
			motion.MaxDamage = math.Float32frombits(value)
		case unitFieldBytes1:
			motion.StandState = uint8(value)
			motion.StandStateKnown = true
			motion.bytes1Cache = motion.bytes1Cache&0xFF | value&0xFFFFFF00
			motion.bytes1CacheKnown = true
		case unitFieldDynamicFlags:
			motion.DynamicFlags = value
		case unitFieldNPCFlags:
			motion.NPCFlags = value
			motion.NPCFlagsKnown = true
		case unitFieldNPCEmoteState:
			motion.EmoteState = value
			motion.EmoteStateKnown = true
		case unitFieldBytes2:
			motion.SheathState = uint8(value)
			motion.SheathStateKnown = true
		default:
			if index >= unitFieldPower1 && index < unitFieldPower1+7 {
				motion.Powers[index-unitFieldPower1] = value
				if index == unitFieldPower1 {
					motion.Mana = value
				}
			} else if index >= unitFieldMaxPower1 && index < unitFieldMaxPower1+7 {
				motion.MaxPowers[index-unitFieldMaxPower1] = value
				if index == unitFieldMaxPower1 {
					motion.MaxMana = value
				}
			} else if index >= unitFieldStat0 && index < unitFieldStat0+5 {
				motion.Stats[index-unitFieldStat0] = value
			} else if index >= unitFieldResistances && index < unitFieldResistances+7 {
				motion.Resistances[index-unitFieldResistances] = value
			} else {
				s.motionMu.Unlock()
				return nil, false, false
			}
		}
		stored, _ := readLocked(index)
		scaleKey := creatureAuraKeyForMotion(motion)
		base := motion.BaseScale
		if motion.ScriptScaleKnown {
			base = motion.ScriptScale
		}
		if base <= 0 {
			base = 1
		}
		s.motionMu.Unlock()
		if index == objectFieldScale {
			effective := float64(base) + float64(s.creatureAuraScaleSum(scaleKey))/100
			if effective < 0.01 {
				effective = 0.01
			}
			return map[int]uint32{objectFieldScale: math.Float32bits(float32(effective))}, stored != old, true
		}
		return map[int]uint32{index: stored}, stored != old, true
	}
	return indexedValueBackend{
		read:  read,
		write: write,
		broadcast: func(fields map[int]uint32) {
			s.broadcastCreatureValuesUpdateInInstance(motion.Map, motion.InstanceID, motion.GUID, fields)
		},
	}
}

// Session-creature indexed values: the session builder (lua_objects.go)
// is a second view of the same live creature the motion builder owns,
// so reads and writes delegate to the motion backend whenever the
// motion still resolves — the two views then agree, including the
// NPCFlags live-override twin. When the motion is gone (despawned
// between build and call), the snapshot slots read their build-time
// values and writes are ignored: the snapshot has no write-back, and
// a write to a despawned creature is a C++ no-op anyway (the object
// would be gone). Slots the snapshot does not carry read 0.
func sessionCreatureIndexedValueBackend(s *session, state luaCreatureState) indexedValueBackend {
	motionBackend := func() (indexedValueBackend, bool) {
		if s == nil || s.server == nil {
			return indexedValueBackend{}, false
		}
		motion := s.luaCreatureMotion(state.GUID, state.Entry)
		if motion == nil {
			return indexedValueBackend{}, false
		}
		return motionIndexedValueBackend(s.server, motion), true
	}
	read := func(index int) (uint32, bool) {
		if backend, ok := motionBackend(); ok {
			return backend.read(index)
		}
		switch index {
		case 0:
			return uint32(state.GUID), true
		case 1:
			return uint32(state.GUID >> 32), true
		case objectFieldType:
			return 3, true
		case objectFieldEntry:
			return state.Entry, true
		case unitFieldHealth:
			return state.Health, true
		case unitFieldMaxHealth:
			return state.MaxHealth, true
		case unitFieldLevel:
			return state.Level, true
		case unitFieldFlags:
			return state.UnitFlags, true
		case unitFieldDisplayID:
			return state.DisplayID, true
		case unitFieldNPCFlags:
			return state.NPCFlags, true
		}
		return 0, false
	}
	write := func(index int, update func(old uint32) uint32) (map[int]uint32, bool, bool) {
		if backend, ok := motionBackend(); ok {
			return backend.write(index, update)
		}
		return nil, false, false
	}
	return indexedValueBackend{
		read:  read,
		write: write,
		broadcast: func(fields map[int]uint32) {
			if backend, ok := motionBackend(); ok {
				backend.broadcast(fields)
			}
		},
	}
}

// Gameobject indexed values (UpdateFields.h: OBJECT_END = 6, then
// CREATED_BY 6-7, DISPLAYID 8, FLAGS 9, PARENTROTATION 10-13, DYNAMIC
// 14, FACTION 15, LEVEL 16, BYTES_1 17). Reads never materialize: a
// live dynamic record wins, else the spawn snapshot (plus the
// template_addon / destructible-override reads the HasFlag leg
// already uses); a pure probe leaves no live state behind. Writes
// materialize a static spawn first, following the SetGoState/flag
// precedent, and broadcast the written field. DYNAMIC is a computed
// per-viewer field (GameObject.cpp:2564: questgiver-activate /
// chest-lootable flags) with no stored slot, so it reads 0 and
// ignores writes; LEVEL is set only for transports
// (GameObject.cpp:365), which have no Go scripting object, so it
// reads 0 too. CREATED_BY writes are ignored: the owner link is
// fixed at summon, not by the raw value. A raw BYTES_1 state write
// does not fire event 10 — only SetGoState does (GameObject.cpp:
// 2407). Coupling, same class as the SetGoState tick: the grid
// stream sources flags/display from the DB row and overrides only
// State from dyn, so a scripted FLAGS/DISPLAYID write is
// client-visible via the values-update broadcast until the next
// grid re-stream.
func goIndexedValueBackend(srv *Server, state luaGameObjectState) indexedValueBackend {
	dyn := func() *dynamicGameObjectState {
		return srv.gameObjectState(state.Map, state.InstanceID, state.GUID)
	}
	read := func(index int) (uint32, bool) {
		record := dyn()
		switch index {
		case 0:
			return uint32(state.GUID), true
		case 1:
			return uint32(state.GUID >> 32), true
		case objectFieldType:
			return 5, true
		case objectFieldEntry:
			return state.Entry, true
		case objectFieldScale:
			if record != nil && record.Size > 0 {
				return math.Float32bits(record.Size), true
			}
			if state.Size > 0 {
				return math.Float32bits(state.Size), true
			}
			return math.Float32bits(1), true
		case gameObjectFieldCreatedBy:
			if record != nil {
				return uint32(record.OwnerGUID), true
			}
			return uint32(state.OwnerGUID), true
		case gameObjectFieldCreatedBy + 1:
			if record != nil {
				return uint32(record.OwnerGUID >> 32), true
			}
			return uint32(state.OwnerGUID >> 32), true
		case gameObjectDisplayID:
			if record != nil {
				return record.DisplayID, true
			}
			return state.DisplayID, true
		case gameObjectFlags:
			if record != nil {
				return record.Flags, true
			}
			if srv != nil {
				if over := srv.destructibleSpawnOverride(state.Map, state.InstanceID, state.LowGUID); over != nil {
					return over.Flags, true
				}
				return srv.gameObjectTemplateAddonFlags(state.Entry), true
			}
			return 0, true
		case gameObjectFieldFaction:
			if record != nil {
				return record.Faction, true
			}
			if srv != nil {
				return srv.gameObjectTemplateAddonFaction(state.Entry), true
			}
			return 0, true
		case gameObjectBytes1:
			if record != nil {
				return uint32(record.State) | uint32(record.Type)<<8 | uint32(record.ArtKit)<<16 | uint32(record.AnimProgress)<<24, true
			}
			return state.GoState | state.Type<<8, true
		}
		if record != nil && index >= gameObjectFieldParentRotation && index < gameObjectFieldParentRotation+4 {
			return math.Float32bits(record.ParentRotation[index-gameObjectFieldParentRotation]), true
		}
		if index >= gameObjectFieldParentRotation && index < gameObjectFieldParentRotation+4 {
			if index == gameObjectFieldParentRotation+3 {
				return math.Float32bits(1), true
			}
			return 0, true
		}
		return 0, false
	}
	write := func(index int, update func(old uint32) uint32) (map[int]uint32, bool, bool) {
		old, ok := read(index)
		if !ok || srv == nil {
			return nil, false, false
		}
		value := update(old)
		srv.objectsMu.Lock()
		record := srv.gameObjectStateLocked(state.Map, state.InstanceID, state.GUID)
		if record == nil {
			srv.objectsMu.Unlock()
			if srv.materializeStaticGameObjectState(state.Map, state.InstanceID, state.GUID) == nil {
				return nil, false, false
			}
			srv.objectsMu.Lock()
			record = srv.gameObjectStateLocked(state.Map, state.InstanceID, state.GUID)
			if record == nil {
				srv.objectsMu.Unlock()
				return nil, false, false
			}
		}
		switch index {
		case objectFieldScale:
			record.Size = math.Float32frombits(value)
		case gameObjectDisplayID:
			record.DisplayID = value
		case gameObjectFlags:
			record.Flags = value
		case gameObjectFieldFaction:
			record.Faction = value
		case gameObjectBytes1:
			record.State = uint8(value)
			record.Type = uint8(value >> 8)
			record.ArtKit = uint8(value >> 16)
			record.AnimProgress = uint8(value >> 24)
		default:
			if index >= gameObjectFieldParentRotation && index < gameObjectFieldParentRotation+4 {
				record.ParentRotation[index-gameObjectFieldParentRotation] = math.Float32frombits(value)
			} else {
				srv.objectsMu.Unlock()
				return nil, false, false
			}
		}
		var stored uint32
		switch index {
		case objectFieldScale:
			stored = math.Float32bits(record.Size)
		case gameObjectBytes1:
			stored = uint32(record.State) | uint32(record.Type)<<8 | uint32(record.ArtKit)<<16 | uint32(record.AnimProgress)<<24
		default:
			if index >= gameObjectFieldParentRotation && index < gameObjectFieldParentRotation+4 {
				stored = math.Float32bits(record.ParentRotation[index-gameObjectFieldParentRotation])
			} else {
				stored = value
			}
		}
		srv.objectsMu.Unlock()
		return map[int]uint32{index: stored}, stored != old, true
	}
	return indexedValueBackend{
		read:  read,
		write: write,
		broadcast: func(fields map[int]uint32) {
			srv.broadcastGameObjectValuesUpdateInInstance(state.Map, state.InstanceID, state.GUID, fields)
		},
	}
}
