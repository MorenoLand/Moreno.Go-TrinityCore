package protocol

const (
	GroupUpdateFlagStatus           uint32 = 0x00000001
	GroupUpdateFlagCurrentHealth    uint32 = 0x00000002
	GroupUpdateFlagMaximumHealth    uint32 = 0x00000004
	GroupUpdateFlagPowerType        uint32 = 0x00000008
	GroupUpdateFlagCurrentPower     uint32 = 0x00000010
	GroupUpdateFlagMaximumPower     uint32 = 0x00000020
	GroupUpdateFlagLevel            uint32 = 0x00000040
	GroupUpdateFlagZone             uint32 = 0x00000080
	GroupUpdateFlagPosition         uint32 = 0x00000100
	GroupUpdateFlagAuras            uint32 = 0x00000200
	GroupUpdateFlagPetGUID          uint32 = 0x00000400
	GroupUpdateFlagPetName          uint32 = 0x00000800
	GroupUpdateFlagPetModelID       uint32 = 0x00001000
	GroupUpdateFlagPetCurrentHealth uint32 = 0x00002000
	GroupUpdateFlagPetMaximumHealth uint32 = 0x00004000
	GroupUpdateFlagPetPowerType     uint32 = 0x00008000
	GroupUpdateFlagPetCurrentPower  uint32 = 0x00010000
	GroupUpdateFlagPetMaximumPower  uint32 = 0x00020000
	GroupUpdateFlagPetAuras         uint32 = 0x00040000
	GroupUpdateFlagVehicleSeat      uint32 = 0x00080000
)

type PartyMemberAura struct {
	SpellID uint32
	Flags   uint8
}

type PartyMemberStatsFull struct {
	GUID             uint64
	UpdateFlags      uint32
	Status           uint16
	Health           uint32
	MaximumHealth    uint32
	PowerType        uint8
	CurrentPower     uint16
	MaximumPower     uint16
	Level            uint16
	Zone             uint16
	X                uint16
	Y                uint16
	AuraMask         uint64
	Auras            [64]PartyMemberAura
	PetGUID          uint64
	PetName          string
	PetModelID       uint16
	PetHealth        uint32
	PetMaximumHealth uint32
	PetPowerType     uint8
	PetCurrentPower  uint16
	PetMaximumPower  uint16
	PetAuraMask      uint64
	PetAuras         [64]PartyMemberAura
	VehicleSeatID    uint32
}

func BuildPartyMemberStatsFull(value PartyMemberStatsFull) []byte {
	packet := NewBuffer(256)
	packet.WriteU8(0)
	packet.WritePackedGUID(value.GUID)
	packet.WriteU32(value.UpdateFlags)
	if value.UpdateFlags&GroupUpdateFlagStatus != 0 {
		packet.WriteU16(value.Status)
	}
	if value.UpdateFlags&GroupUpdateFlagCurrentHealth != 0 {
		packet.WriteU32(value.Health)
	}
	if value.UpdateFlags&GroupUpdateFlagMaximumHealth != 0 {
		packet.WriteU32(value.MaximumHealth)
	}
	if value.UpdateFlags&GroupUpdateFlagPowerType != 0 {
		packet.WriteU8(value.PowerType)
	}
	if value.UpdateFlags&GroupUpdateFlagCurrentPower != 0 {
		packet.WriteU16(value.CurrentPower)
	}
	if value.UpdateFlags&GroupUpdateFlagMaximumPower != 0 {
		packet.WriteU16(value.MaximumPower)
	}
	if value.UpdateFlags&GroupUpdateFlagLevel != 0 {
		packet.WriteU16(value.Level)
	}
	if value.UpdateFlags&GroupUpdateFlagZone != 0 {
		packet.WriteU16(value.Zone)
	}
	if value.UpdateFlags&GroupUpdateFlagPosition != 0 {
		packet.WriteU16(value.X)
		packet.WriteU16(value.Y)
	}
	if value.UpdateFlags&GroupUpdateFlagAuras != 0 {
		packet.WriteU64(value.AuraMask)
		for slot, aura := range value.Auras {
			if value.AuraMask&(uint64(1)<<uint(slot)) != 0 {
				packet.WriteU32(aura.SpellID)
				packet.WriteU8(aura.Flags)
			}
		}
	}
	if value.UpdateFlags&GroupUpdateFlagPetGUID != 0 {
		packet.WriteU64(value.PetGUID)
	}
	if value.UpdateFlags&GroupUpdateFlagPetName != 0 {
		packet.WriteCString(value.PetName)
	}
	if value.UpdateFlags&GroupUpdateFlagPetModelID != 0 {
		packet.WriteU16(value.PetModelID)
	}
	if value.UpdateFlags&GroupUpdateFlagPetCurrentHealth != 0 {
		packet.WriteU32(value.PetHealth)
	}
	if value.UpdateFlags&GroupUpdateFlagPetMaximumHealth != 0 {
		packet.WriteU32(value.PetMaximumHealth)
	}
	if value.UpdateFlags&GroupUpdateFlagPetPowerType != 0 {
		packet.WriteU8(value.PetPowerType)
	}
	if value.UpdateFlags&GroupUpdateFlagPetCurrentPower != 0 {
		packet.WriteU16(value.PetCurrentPower)
	}
	if value.UpdateFlags&GroupUpdateFlagPetMaximumPower != 0 {
		packet.WriteU16(value.PetMaximumPower)
	}
	if value.UpdateFlags&GroupUpdateFlagPetAuras != 0 {
		packet.WriteU64(value.PetAuraMask)
		for slot, aura := range value.PetAuras {
			if value.PetAuraMask&(uint64(1)<<uint(slot)) != 0 {
				packet.WriteU32(aura.SpellID)
				packet.WriteU8(aura.Flags)
			}
		}
	}
	if value.UpdateFlags&GroupUpdateFlagVehicleSeat != 0 {
		packet.WriteU32(value.VehicleSeatID)
	}
	return packet.Bytes()
}
