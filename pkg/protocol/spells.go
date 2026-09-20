package protocol

const (
	SpellTargetFlagUnit           uint32 = 0x00000002
	SpellTargetFlagItem           uint32 = 0x00000010
	SpellTargetFlagSourceLocation uint32 = 0x00000020
	SpellTargetFlagDestLocation   uint32 = 0x00000040
	SpellTargetFlagCorpseEnemy    uint32 = 0x00000200
	SpellTargetFlagGameObject     uint32 = 0x00000800
	SpellTargetFlagTradeItem      uint32 = 0x00001000
	SpellTargetFlagString         uint32 = 0x00002000
	SpellTargetFlagCorpseAlly     uint32 = 0x00008000
	SpellTargetFlagUnitMinipet    uint32 = 0x00010000
	SpellTargetFlagDestTarget     uint32 = 0x040000
	SpellTargetFlagUnitWireMask          = SpellTargetFlagUnit | SpellTargetFlagUnitMinipet | SpellTargetFlagGameObject | SpellTargetFlagCorpseEnemy | SpellTargetFlagCorpseAlly
	SpellTargetFlagItemWireMask          = SpellTargetFlagItem | SpellTargetFlagTradeItem
	SpellCastFlagVisualChain      uint32 = 0x00080000

	SpellMissNone    uint8 = 0
	SpellMissMiss    uint8 = 1
	SpellMissResist  uint8 = 2
	SpellMissDodge   uint8 = 3
	SpellMissParry   uint8 = 4
	SpellMissBlock   uint8 = 5
	SpellMissEvade   uint8 = 6
	SpellMissImmune  uint8 = 7
	SpellMissImmune2 uint8 = 8
	SpellMissDeflect uint8 = 9
	SpellMissAbsorb  uint8 = 10
	SpellMissReflect uint8 = 11

	SpellAuraPeriodicDamage        uint32 = 3
	SpellAuraPeriodicHeal          uint32 = 8
	SpellAuraObsModHealth          uint32 = 20
	SpellAuraObsModPower           uint32 = 21
	SpellAuraPeriodicEnergize      uint32 = 24
	SpellAuraPeriodicLeech         uint32 = 53
	SpellAuraPeriodicDamagePercent uint32 = 89

	AuraFlagEffIndex0 uint8 = 0x01
	AuraFlagEffIndex1 uint8 = 0x02
	AuraFlagEffIndex2 uint8 = 0x04
	AuraFlagCaster    uint8 = 0x08
	AuraFlagPositive  uint8 = 0x10
	AuraFlagDuration  uint8 = 0x20
	AuraFlagNegative  uint8 = 0x80
)

type SpellTargetLocation struct {
	Transport uint64
	X         float32
	Y         float32
	Z         float32
}

type SpellTargetData struct {
	Flags        uint32
	UnitGUID     uint64
	ItemGUID     uint64
	Source       SpellTargetLocation
	Destination  SpellTargetLocation
	StringTarget string
}

type SpellMissStatus struct {
	TargetGUID    uint64
	Reason        uint8
	ReflectStatus uint8
}

func ReadSpellTargetData(reader *Buffer) (SpellTargetData, error) {
	var target SpellTargetData
	var err error
	if target.Flags, err = reader.ReadU32(); err != nil {
		return target, err
	}
	if target.Flags&SpellTargetFlagUnitWireMask != 0 {
		if target.UnitGUID, err = reader.ReadPackedGUID(); err != nil {
			return target, err
		}
	}
	if target.Flags&SpellTargetFlagItemWireMask != 0 {
		if target.ItemGUID, err = reader.ReadPackedGUID(); err != nil {
			return target, err
		}
	}
	if target.Flags&SpellTargetFlagSourceLocation != 0 {
		if target.Source, err = readSpellTargetLocation(reader); err != nil {
			return target, err
		}
	}
	if target.Flags&SpellTargetFlagDestLocation != 0 {
		if target.Destination, err = readSpellTargetLocation(reader); err != nil {
			return target, err
		}
	}
	if target.Flags&SpellTargetFlagString != 0 {
		if target.StringTarget, err = reader.ReadCString(); err != nil {
			return target, err
		}
	}
	return target, nil
}

func readSpellTargetLocation(reader *Buffer) (SpellTargetLocation, error) {
	var location SpellTargetLocation
	var err error
	if location.Transport, err = reader.ReadPackedGUID(); err != nil {
		return location, err
	}
	if location.X, err = reader.ReadF32(); err != nil {
		return location, err
	}
	if location.Y, err = reader.ReadF32(); err != nil {
		return location, err
	}
	if location.Z, err = reader.ReadF32(); err != nil {
		return location, err
	}
	return location, nil
}

func BuildSpellStart(casterGUID, casterUnitGUID uint64, castID uint8, spellID, castFlags, castTime uint32, target SpellTargetData) []byte {
	packet := NewBuffer(64)
	writeSpellCastHeader(packet, casterGUID, casterUnitGUID, castID, spellID, castFlags, castTime)
	writeSpellTargetData(packet, target)
	writeSpellCastTrailer(packet, castFlags, target.Flags)
	return packet.Bytes()
}

func BuildSpellGo(casterGUID, casterUnitGUID uint64, castID uint8, spellID, castFlags, castTime uint32, hitTargets []uint64, missStatus []SpellMissStatus, target SpellTargetData) []byte {
	packet := NewBuffer(96)
	writeSpellCastHeader(packet, casterGUID, casterUnitGUID, castID, spellID, castFlags, castTime)
	if len(hitTargets) > 255 {
		hitTargets = hitTargets[:255]
	}
	packet.WriteU8(uint8(len(hitTargets)))
	for _, guid := range hitTargets {
		packet.WriteU64(guid)
	}
	if len(missStatus) > 255 {
		missStatus = missStatus[:255]
	}
	packet.WriteU8(uint8(len(missStatus)))
	for _, status := range missStatus {
		packet.WriteU64(status.TargetGUID)
		packet.WriteU8(status.Reason)
		if status.Reason == SpellMissReflect {
			packet.WriteU8(status.ReflectStatus)
		}
	}
	writeSpellTargetData(packet, target)
	writeSpellCastTrailer(packet, castFlags, target.Flags)
	return packet.Bytes()
}

func BuildSpellFailure(castID uint8, spellID uint32, result uint8) []byte {
	packet := NewBuffer(6)
	packet.WriteU8(castID)
	packet.WriteU32(spellID)
	packet.WriteU8(result)
	return packet.Bytes()
}

func writeSpellCastHeader(packet *Buffer, casterGUID, casterUnitGUID uint64, castID uint8, spellID, castFlags, castTime uint32) {
	packet.WritePackedGUID(casterGUID)
	packet.WritePackedGUID(casterUnitGUID)
	packet.WriteU8(castID)
	packet.WriteU32(spellID)
	packet.WriteU32(castFlags)
	packet.WriteU32(castTime)
}

// WriteSpellTargetData writes serialized SpellTargetData into packet.
func WriteSpellTargetData(packet *Buffer, target SpellTargetData) {
	writeSpellTargetData(packet, target)
}

func writeSpellTargetData(packet *Buffer, target SpellTargetData) {
	packet.WriteU32(target.Flags)
	if target.Flags&SpellTargetFlagUnitWireMask != 0 {
		packet.WritePackedGUID(target.UnitGUID)
	}
	if target.Flags&SpellTargetFlagItemWireMask != 0 {
		packet.WritePackedGUID(target.ItemGUID)
	}
	if target.Flags&SpellTargetFlagSourceLocation != 0 {
		writeSpellTargetLocation(packet, target.Source)
	}
	if target.Flags&SpellTargetFlagDestLocation != 0 {
		writeSpellTargetLocation(packet, target.Destination)
	}
	if target.Flags&SpellTargetFlagString != 0 {
		packet.WriteCString(target.StringTarget)
	}
}

func writeSpellTargetLocation(packet *Buffer, location SpellTargetLocation) {
	packet.WritePackedGUID(location.Transport)
	packet.WriteF32(location.X)
	packet.WriteF32(location.Y)
	packet.WriteF32(location.Z)
}

func writeSpellCastTrailer(packet *Buffer, castFlags, targetFlags uint32) {
	if castFlags&SpellCastFlagVisualChain != 0 {
		packet.WriteU32(0)
		packet.WriteU32(0)
	}
	if targetFlags&SpellTargetFlagDestLocation != 0 {
		packet.WriteU8(0)
	}
}

// BuildPeriodicAuraLogDamage builds SMSG_PERIODICAURALOG for periodic damage / periodic damage percent.
// Reference: TrinityCore Unit::SendPeriodicAuraLog (Unit.cpp:5372).
func BuildPeriodicAuraLogDamage(targetGUID, casterGUID uint64, spellID, auraType, damage, overkill, schoolMask, absorb, resist uint32, critical bool) []byte {
	buf := NewBuffer(36)
	buf.WritePackedGUID(targetGUID)
	buf.WritePackedGUID(casterGUID)
	buf.WriteU32(spellID)
	buf.WriteU32(1) // count
	buf.WriteU32(auraType)
	buf.WriteU32(damage)
	buf.WriteU32(overkill)
	buf.WriteU32(schoolMask)
	buf.WriteU32(absorb)
	buf.WriteU32(resist)
	if critical {
		buf.WriteU8(1)
	} else {
		buf.WriteU8(0)
	}
	return buf.Bytes()
}

// BuildPeriodicAuraLogHeal builds SMSG_PERIODICAURALOG for periodic heal / obs mod health.
// Reference: TrinityCore Unit::SendPeriodicAuraLog (Unit.cpp:5372).
func BuildPeriodicAuraLogHeal(targetGUID, casterGUID uint64, spellID, auraType, heal, overheal, absorb uint32, critical bool) []byte {
	buf := NewBuffer(32)
	buf.WritePackedGUID(targetGUID)
	buf.WritePackedGUID(casterGUID)
	buf.WriteU32(spellID)
	buf.WriteU32(1) // count
	buf.WriteU32(auraType)
	buf.WriteU32(heal)
	buf.WriteU32(overheal)
	buf.WriteU32(absorb)
	if critical {
		buf.WriteU8(1)
	} else {
		buf.WriteU8(0)
	}
	return buf.Bytes()
}

// BuildPeriodicAuraLogEnergize builds SMSG_PERIODICAURALOG for periodic energize / obs mod power.
// Reference: TrinityCore Unit::SendPeriodicAuraLog (Unit.cpp:5372).
func BuildPeriodicAuraLogEnergize(targetGUID, casterGUID uint64, spellID, auraType, powerType, amount uint32) []byte {
	buf := NewBuffer(28)
	buf.WritePackedGUID(targetGUID)
	buf.WritePackedGUID(casterGUID)
	buf.WriteU32(spellID)
	buf.WriteU32(1) // count
	buf.WriteU32(auraType)
	buf.WriteU32(powerType)
	buf.WriteU32(amount)
	return buf.Bytes()
}

// BuildAuraUpdate builds SMSG_AURA_UPDATE (0x496) payload.
// Reference: TrinityCore AuraApplication::BuildUpdatePacket (SpellAuras.cpp:230-252).
func BuildAuraUpdate(targetGUID, casterGUID uint64, slot uint8, spellID uint32, remove, positive bool, maxDurationMs, durationMs uint32, casterLevel uint8) []byte {
	return BuildAuraUpdateWithStack(targetGUID, casterGUID, slot, spellID, remove, positive, maxDurationMs, durationMs, casterLevel, 1)
}

type AuraUpdateRecord struct {
	CasterGUID    uint64
	Slot          uint8
	SpellID       uint32
	EffectMask    uint8
	Positive      bool
	MaxDurationMs uint32
	DurationMs    uint32
	CasterLevel   uint8
	StackCount    uint8
}

func BuildAuraUpdateWithStack(targetGUID, casterGUID uint64, slot uint8, spellID uint32, remove, positive bool, maxDurationMs, durationMs uint32, casterLevel, stackCount uint8) []byte {
	return BuildAuraUpdateWithStackEffect(targetGUID, casterGUID, slot, spellID, remove, positive, maxDurationMs, durationMs, casterLevel, stackCount, 0x01)
}

func BuildAuraUpdateWithStackEffect(targetGUID, casterGUID uint64, slot uint8, spellID uint32, remove, positive bool, maxDurationMs, durationMs uint32, casterLevel, stackCount, effectMask uint8) []byte {
	buf := NewBuffer(36)
	buf.WritePackedGUID(targetGUID)
	if remove {
		buf.WriteU8(slot)
		buf.WriteU32(0)
		return buf.Bytes()
	}
	writeAuraUpdateRecord(buf, targetGUID, AuraUpdateRecord{CasterGUID: casterGUID, Slot: slot, SpellID: spellID, EffectMask: effectMask, Positive: positive, MaxDurationMs: maxDurationMs, DurationMs: durationMs, CasterLevel: casterLevel, StackCount: stackCount})
	return buf.Bytes()
}

func BuildAuraUpdateAll(targetGUID uint64, records []AuraUpdateRecord) []byte {
	buf := NewBuffer(16 + len(records)*24)
	buf.WritePackedGUID(targetGUID)
	for _, record := range records {
		writeAuraUpdateRecord(buf, targetGUID, record)
	}
	return buf.Bytes()
}

func writeAuraUpdateRecord(buf *Buffer, targetGUID uint64, record AuraUpdateRecord) {
	buf.WriteU8(record.Slot)
	buf.WriteU32(record.SpellID)
	flags := uint8(0)
	if record.EffectMask&0x01 != 0 {
		flags |= AuraFlagEffIndex0
	}
	if record.EffectMask&0x02 != 0 {
		flags |= AuraFlagEffIndex1
	}
	if record.EffectMask&0x04 != 0 {
		flags |= AuraFlagEffIndex2
	}
	if flags&0x07 == 0 {
		flags |= AuraFlagEffIndex0
	}
	if record.CasterGUID == 0 || record.CasterGUID == targetGUID {
		flags |= AuraFlagCaster
	}
	if record.Positive {
		flags |= AuraFlagPositive
	} else {
		flags |= AuraFlagNegative
	}
	if record.MaxDurationMs > 0 {
		flags |= AuraFlagDuration
	}
	buf.WriteU8(flags)
	if record.CasterLevel == 0 {
		record.CasterLevel = 1
	}
	buf.WriteU8(record.CasterLevel)
	if record.StackCount == 0 {
		record.StackCount = 1
	}
	buf.WriteU8(record.StackCount)
	if flags&AuraFlagCaster == 0 { // not self-cast
		buf.WritePackedGUID(record.CasterGUID)
	}
	if record.MaxDurationMs > 0 {
		buf.WriteU32(record.MaxDurationMs)
		buf.WriteU32(record.DurationMs)
	}
}
