package protocol

// HitInfo bitmask constants.
// Reference: TrinityCore UnitDefines.h:291 (enum HitInfo).
const (
	HitInfoNormalSwing     uint32 = 0x00000000
	HitInfoUnk1            uint32 = 0x00000001
	HitInfoAffectsVictim   uint32 = 0x00000002
	HitInfoOffHand         uint32 = 0x00000004
	HitInfoUnk2            uint32 = 0x00000008
	HitInfoMiss            uint32 = 0x00000010
	HitInfoFullAbsorb      uint32 = 0x00000020
	HitInfoPartialAbsorb   uint32 = 0x00000040
	HitInfoFullResist      uint32 = 0x00000080
	HitInfoPartialResist   uint32 = 0x00000100
	HitInfoCriticalHit     uint32 = 0x00000200
	HitInfoBlock           uint32 = 0x00002000
	HitInfoGlancing        uint32 = 0x00010000
	HitInfoCrushing        uint32 = 0x00020000
	HitInfoNoAnimation     uint32 = 0x00040000
	HitInfoSwingNoHitSound uint32 = 0x00200000
	HitInfoRageGain        uint32 = 0x00800000
	HitInfoFakeDamage      uint32 = 0x01000000
)

// VictimState enum constants.
// Reference: TrinityCore Unit.h:43 (enum VictimState).
const (
	VictimStateIntact    uint8 = 0 // Set when attacker misses
	VictimStateHit       uint8 = 1 // Victim took damage / partial block
	VictimStateDodge     uint8 = 2
	VictimStateParry     uint8 = 3
	VictimStateInterrupt uint8 = 4
	VictimStateBlocks    uint8 = 5 // Full block
	VictimStateEvades    uint8 = 6
	VictimStateIsImmune  uint8 = 7
	VictimStateDeflects  uint8 = 8
)

// WeaponAttackType enum constants.
// Reference: TrinityCore UnitDefines.h:261 (enum WeaponAttackType).
type WeaponAttackType uint8

const (
	BaseAttack   WeaponAttackType = 0
	OffAttack    WeaponAttackType = 1
	RangedAttack WeaponAttackType = 2
)

// MeleeHitOutcome enum constants.
// Reference: TrinityCore Unit.h:356 (enum MeleeHitOutcome).
type MeleeHitOutcome uint8

const (
	MeleeHitEvade    MeleeHitOutcome = 0
	MeleeHitMiss     MeleeHitOutcome = 1
	MeleeHitDodge    MeleeHitOutcome = 2
	MeleeHitBlock    MeleeHitOutcome = 3
	MeleeHitParry    MeleeHitOutcome = 4
	MeleeHitGlancing MeleeHitOutcome = 5
	MeleeHitCrit     MeleeHitOutcome = 6
	MeleeHitCrushing MeleeHitOutcome = 7
	MeleeHitNormal   MeleeHitOutcome = 8
	MeleeHitImmune   MeleeHitOutcome = 10
)

// AttackerStateUpdateDamage is one sub-damage component of
// SMSG_ATTACKERSTATEUPDATE: TrinityCore CalcDamageInfo::Damages[i]
// (Unit.h:504-513) — the per-component school mask, post-absorb damage,
// absorbed amount and resisted amount.
type AttackerStateUpdateDamage struct {
	SchoolMask uint32
	Damage     uint32
	Absorb     uint32
	Resist     uint32
}

// BuildAttackerStateUpdate builds SMSG_ATTACKERSTATEUPDATE (0x14A) packet.
// Reference: TrinityCore Unit::SendAttackStateUpdate (Unit.cpp:5442).
func BuildAttackerStateUpdate(attacker, victim uint64, damage, overkill uint32, hitInfo uint32, targetState uint8, blocked uint32) []byte {
	return BuildAttackerStateUpdateFull(attacker, victim, []AttackerStateUpdateDamage{{SchoolMask: 1, Damage: damage}}, overkill, hitInfo, targetState, blocked)
}

// BuildAttackerStateUpdateFull builds SMSG_ATTACKERSTATEUPDATE (0x14A) with the
// full per-component layout of Unit::SendAttackStateUpdate (Unit.cpp:5442-5500):
// the sub-damage count is 2 only when the second component carries damage,
// absorb or resist; the absorb array rides only under HITINFO_FULL_ABSORB /
// HITINFO_PARTIAL_ABSORB and the resist array only under HITINFO_FULL_RESIST /
// HITINFO_PARTIAL_RESIST, one entry per sub-damage; HITINFO_RAGE_GAIN appends
// a zero u32 and HITINFO_UNK1 the debug block (u32 0, ten f32 0, u32 0).
func BuildAttackerStateUpdateFull(attacker, victim uint64, damages []AttackerStateUpdateDamage, overkill uint32, hitInfo uint32, targetState uint8, blocked uint32) []byte {
	if len(damages) == 0 {
		damages = []AttackerStateUpdateDamage{{SchoolMask: 1}}
	}
	count := 1
	if len(damages) > 1 && (damages[1].Damage != 0 || damages[1].Absorb != 0 || damages[1].Resist != 0) {
		count = 2
	}
	fullDamage := uint32(0)
	for i := 0; i < count; i++ {
		fullDamage += damages[i].Damage
	}
	packet := NewBuffer(64)
	packet.WriteU32(hitInfo)
	packet.WritePackedGUID(attacker)
	packet.WritePackedGUID(victim)
	packet.WriteU32(fullDamage)
	packet.WriteU32(overkill)
	packet.WriteU8(uint8(count))
	for i := 0; i < count; i++ {
		packet.WriteU32(damages[i].SchoolMask)
		packet.WriteF32(float32(damages[i].Damage))
		packet.WriteU32(damages[i].Damage)
	}
	if hitInfo&(HitInfoFullAbsorb|HitInfoPartialAbsorb) != 0 {
		for i := 0; i < count; i++ {
			packet.WriteU32(damages[i].Absorb)
		}
	}
	if hitInfo&(HitInfoFullResist|HitInfoPartialResist) != 0 {
		for i := 0; i < count; i++ {
			packet.WriteU32(damages[i].Resist)
		}
	}
	packet.WriteU8(targetState)
	packet.WriteU32(0) // Unknown attackerstate
	packet.WriteU32(0) // Melee spell ID
	if hitInfo&HitInfoBlock != 0 {
		packet.WriteU32(blocked)
	}
	if hitInfo&HitInfoRageGain != 0 {
		packet.WriteU32(0)
	}
	if hitInfo&HitInfoUnk1 != 0 {
		packet.WriteU32(0)
		for i := 0; i < 10; i++ {
			packet.WriteF32(0)
		}
		packet.WriteU32(0)
	}
	return packet.Bytes()
}
