package world

import (
	"context"
	"math"
	"sync"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// VehicleSeatAddon represents orientation and exit position offsets from vehicle_seat_addon table.
// Reference: TrinityCore VehicleDefines.h:76-89.
type VehicleSeatAddon struct {
	SeatEntry             uint32
	SeatOrientationOffset float32
	ExitParamX            float32
	ExitParamY            float32
	ExitParamZ            float32
	ExitParamO            float32
	ExitParameter         uint8 // 0=None, 1=Offset, 2=Dest
}

// VehicleAccessory represents an NPC mounted to a vehicle seat.
// Reference: TrinityCore VehicleDefines.h:105-114.
type VehicleAccessory struct {
	AccessoryEntry uint32
	SeatID         int8
	IsMinion       bool
	SummonType     uint8
	SummonTime     uint32
}

// PassengerInfo stores passenger data in a vehicle seat.
// Reference: TrinityCore VehicleDefines.h:64-74.
type PassengerInfo struct {
	GUID           uint64
	IsUnselectable bool
}

// Reset clears passenger information from the seat.
func (p *PassengerInfo) Reset() {
	p.GUID = 0
	p.IsUnselectable = false
}

// VehicleSeat represents a single seat on a vehicle kit.
// Reference: TrinityCore VehicleDefines.h:91-103.
type VehicleSeat struct {
	SeatInfo  *wotlk.VehicleSeatEntry
	SeatAddon *VehicleSeatAddon
	Passenger PassengerInfo
}

// IsEmpty returns true if no passenger is currently occupying the seat.
func (s *VehicleSeat) IsEmpty() bool {
	return s.Passenger.GUID == 0
}

// VehicleKit manages seats, passengers, and accessories for a vehicle unit.
// Reference: TrinityCore Vehicle.h / Vehicle.cpp.
type VehicleKit struct {
	mu                   sync.RWMutex
	VehicleGUID          uint64
	VehicleID            uint32
	CreatureEntry        uint32
	IsPlayer             bool
	VehicleInfo          *wotlk.VehicleEntry
	Seats                map[int8]*VehicleSeat
	UsableSeatNum        uint32
	Accessories          []VehicleAccessory
	InstalledAccessories []uint64
	LastShootPos         [3]float32
}

// HasEmptySeat checks if the specified seat exists and is vacant.
// Reference: Vehicle::HasEmptySeat (Vehicle.cpp:275-281).
func (v *VehicleKit) HasEmptySeat(seatID int8) bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	seat, ok := v.Seats[seatID]
	if !ok {
		return false
	}
	return seat.IsEmpty()
}

// GetPassenger returns the GUID of the passenger on the specified seat, or 0 if vacant.
// Reference: Vehicle::GetPassenger (Vehicle.cpp:296-303).
func (v *VehicleKit) GetPassenger(seatID int8) uint64 {
	v.mu.RLock()
	defer v.mu.RUnlock()
	seat, ok := v.Seats[seatID]
	if !ok {
		return 0
	}
	return seat.Passenger.GUID
}

// GetSeatForPassenger locates the seat and metadata for the given passenger GUID.
// Reference: Vehicle::GetSeatForPassenger (Vehicle.cpp:646-653).
func (v *VehicleKit) GetSeatForPassenger(passengerGUID uint64) (int8, *wotlk.VehicleSeatEntry, *VehicleSeatAddon) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	for id, s := range v.Seats {
		if !s.IsEmpty() && s.Passenger.GUID == passengerGUID {
			return id, s.SeatInfo, s.SeatAddon
		}
	}
	return -1, nil, nil
}

// GetSeatAddonForSeatOfPassenger returns seat addon data for a passenger.
// Reference: Vehicle::GetSeatAddonForSeatOfPassenger (Vehicle.cpp:360-367).
func (v *VehicleKit) GetSeatAddonForSeatOfPassenger(passengerGUID uint64) *VehicleSeatAddon {
	v.mu.RLock()
	defer v.mu.RUnlock()
	for _, s := range v.Seats {
		if !s.IsEmpty() && s.Passenger.GUID == passengerGUID {
			return s.SeatAddon
		}
	}
	return nil
}

// GetAvailableSeatCount returns the number of vacant seats usable by players.
// Reference: Vehicle::GetAvailableSeatCount (Vehicle.cpp:689-698).
func (v *VehicleKit) GetAvailableSeatCount() uint8 {
	v.mu.RLock()
	defer v.mu.RUnlock()
	count := uint8(0)
	for _, s := range v.Seats {
		if s.IsEmpty() && s.SeatInfo != nil && (s.SeatInfo.CanEnterOrExit() || s.SeatInfo.IsUsableByOverride()) {
			count++
		}
	}
	return count
}

// GetNextEmptySeat finds the next or previous empty seat from currSeatID.
// Reference: Vehicle::GetNextEmptySeat (Vehicle.cpp:319-345).
func (v *VehicleKit) GetNextEmptySeat(currSeatID int8, forward bool) (int8, bool) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	n := int8(len(v.Seats))
	if n == 0 {
		return -1, false
	}
	step := int8(1)
	if !forward {
		step = -1
	}
	cur := currSeatID
	for i := int8(0); i < n; i++ {
		cur = (cur + step + n) % n
		if s, ok := v.Seats[cur]; ok && s.IsEmpty() && s.SeatInfo != nil && (s.SeatInfo.CanEnterOrExit() || s.SeatInfo.IsUsableByOverride()) {
			return cur, true
		}
	}
	return -1, false
}

// AddPassenger attempts to board a passenger onto the vehicle at seatID (-1 for first available).
// Reference: Vehicle::AddPassenger (Vehicle.cpp:426-484).
func (v *VehicleKit) AddPassenger(passengerGUID uint64, seatID int8) (assignedSeat int8, seatInfo *wotlk.VehicleSeatEntry, ok bool) {
	v.mu.Lock()
	defer v.mu.Unlock()

	// If already in vehicle, return current seat
	for id, s := range v.Seats {
		if s.Passenger.GUID == passengerGUID {
			return id, s.SeatInfo, true
		}
	}

	if seatID < 0 {
		for id := int8(0); id < int8(len(v.Seats)); id++ {
			s, exists := v.Seats[id]
			if exists && s.IsEmpty() && s.SeatInfo != nil && (s.SeatInfo.CanEnterOrExit() || s.SeatInfo.IsUsableByOverride()) {
				seatID = id
				break
			}
		}
		if seatID < 0 {
			return -1, nil, false
		}
	}

	seat, exists := v.Seats[seatID]
	if !exists || !seat.IsEmpty() {
		return -1, nil, false
	}

	seat.Passenger.GUID = passengerGUID
	if seat.SeatInfo != nil && seat.SeatInfo.CanEnterOrExit() && v.UsableSeatNum > 0 {
		v.UsableSeatNum--
	}
	return seatID, seat.SeatInfo, true
}

// RemovePassenger removes the passenger from the vehicle kit.
// Reference: Vehicle::RemovePassenger (Vehicle.cpp:498-543).
func (v *VehicleKit) RemovePassenger(passengerGUID uint64) (seatID int8, seatInfo *wotlk.VehicleSeatEntry, ok bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	for id, s := range v.Seats {
		if s.Passenger.GUID == passengerGUID {
			s.Passenger.Reset()
			if s.SeatInfo != nil && s.SeatInfo.CanEnterOrExit() {
				v.UsableSeatNum++
			}
			return id, s.SeatInfo, true
		}
	}
	return -1, nil, false
}

// SwitchSeat moves a passenger to newSeatID, validating CanSwitchFromSeat.
// Reference: VehicleHandler.cpp:64-123.
func (v *VehicleKit) SwitchSeat(passengerGUID uint64, newSeatID int8) (assignedSeat int8, seatInfo *wotlk.VehicleSeatEntry, ok bool) {
	v.mu.Lock()
	defer v.mu.Unlock()

	currSeatID := int8(-1)
	var currSeat *VehicleSeat
	for id, s := range v.Seats {
		if s.Passenger.GUID == passengerGUID {
			currSeatID = id
			currSeat = s
			break
		}
	}
	if currSeat == nil {
		return -1, nil, false
	}

	if currSeat.SeatInfo != nil && !currSeat.SeatInfo.CanSwitchFromSeat() {
		return currSeatID, currSeat.SeatInfo, false
	}

	destSeat, exists := v.Seats[newSeatID]
	if !exists || !destSeat.IsEmpty() {
		return currSeatID, currSeat.SeatInfo, false
	}

	destSeat.Passenger = currSeat.Passenger
	currSeat.Passenger.Reset()

	return newSeatID, destSeat.SeatInfo, true
}

// IsVehicleInUse returns true if any passenger is currently boarded.
// Reference: Vehicle::IsVehicleInUse (Vehicle.cpp:590-597).
func (v *VehicleKit) IsVehicleInUse() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	for _, s := range v.Seats {
		if !s.IsEmpty() {
			return true
		}
	}
	return false
}

// IsControllable returns true if any seat on the vehicle has CanControl flag.
// Reference: Vehicle::IsControllableVehicle (Vehicle.cpp:599-606).
func (v *VehicleKit) IsControllable() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	for _, s := range v.Seats {
		if s.SeatInfo != nil && s.SeatInfo.CanControl() {
			return true
		}
	}
	return false
}

// CalculatePassengerPosition transforms transport offsets into global coordinates.
// Reference: TransportBase::CalculatePassengerPosition (VehicleDefines.h:134-143).
func CalculatePassengerPosition(transX, transY, transZ, transO float32, inX, inY, inZ, inO float32) (outX, outY, outZ, outO float32) {
	cosO := float32(math.Cos(float64(transO)))
	sinO := float32(math.Sin(float64(transO)))
	outX = transX + inX*cosO - inY*sinO
	outY = transY + inY*cosO + inX*sinO
	outZ = transZ + inZ
	outO = normalizeOrientation(transO + inO)
	return
}

// CalculatePassengerOffset transforms global coordinates into local transport offsets.
// Reference: TransportBase::CalculatePassengerOffset (VehicleDefines.h:145-156).
func CalculatePassengerOffset(transX, transY, transZ, transO float32, globalX, globalY, globalZ, globalO float32) (outX, outY, outZ, outO float32) {
	outO = normalizeOrientation(globalO - transO)
	z := globalZ - transZ
	y := globalY - transY
	x := globalX - transX
	cosO := float32(math.Cos(float64(transO)))
	sinO := float32(math.Sin(float64(transO)))
	tanO := float32(math.Tan(float64(transO)))
	denom := cosO + sinO*tanO
	if math.Abs(float64(denom)) > 1e-6 {
		outY = (y - x*tanO) / denom
		outX = (x + y*tanO) / denom
	} else {
		outX = x*cosO + y*sinO
		outY = y*cosO - x*sinO
	}
	outZ = z
	return
}

// createVehicleKit initializes a VehicleKit for a given GUID, caching seats and accessories.
// Reference: Unit::CreateVehicleKit (Unit.cpp:8320-8336) & Vehicle::Vehicle (Vehicle.cpp:36-58).
func (s *Server) createVehicleKit(vehicleGUID uint64, vehicleID uint32, creatureEntry uint32, isPlayer bool) *VehicleKit {
	if s == nil {
		return nil
	}
	s.vehicleMu.Lock()
	defer s.vehicleMu.Unlock()

	if s.vehicleKits == nil {
		s.vehicleKits = make(map[uint64]*VehicleKit)
	}

	kit := &VehicleKit{
		VehicleGUID:   vehicleGUID,
		VehicleID:     vehicleID,
		CreatureEntry: creatureEntry,
		IsPlayer:      isPlayer,
		Seats:         make(map[int8]*VehicleSeat),
	}

	if s.Data != nil && vehicleID != 0 {
		if vehEntry, ok, err := s.Data.Vehicle(vehicleID); err == nil && ok {
			kit.VehicleInfo = &vehEntry
			for seatIdx := int8(0); seatIdx < wotlk.MaxVehicleSeats; seatIdx++ {
				seatDBCID := vehEntry.SeatIDs[seatIdx]
				if seatDBCID == 0 {
					continue
				}
				if seatEntry, ok, err := s.Data.VehicleSeat(seatDBCID); err == nil && ok {
					var addon *VehicleSeatAddon
					if s.vehicleSeatAddons != nil {
						addon = s.vehicleSeatAddons[seatDBCID]
					}
					kit.Seats[seatIdx] = &VehicleSeat{
						SeatInfo:  &seatEntry,
						SeatAddon: addon,
					}
					if seatEntry.CanEnterOrExit() {
						kit.UsableSeatNum++
					}
				}
			}
		}
	}

	// Default fallback seats if no DBC entry was loaded
	if len(kit.Seats) == 0 {
		for i := int8(0); i < 8; i++ {
			flags := wotlk.VehicleSeatFlagCanEnterOrExit | wotlk.VehicleSeatFlagCanSwitch
			if i == 0 {
				flags |= wotlk.VehicleSeatFlagCanControl
			}
			flagsB := wotlk.VehicleSeatFlagBEjectable
			kit.Seats[i] = &VehicleSeat{
				SeatInfo: &wotlk.VehicleSeatEntry{
					ID:     uint32(i + 1),
					Flags:  flags,
					FlagsB: flagsB,
				},
			}
			kit.UsableSeatNum++
		}
	}

	if s.vehicleAccessories != nil && creatureEntry != 0 {
		if accs, ok := s.vehicleAccessories[creatureEntry]; ok {
			kit.Accessories = append(kit.Accessories, accs...)
		}
	}

	s.vehicleKits[vehicleGUID] = kit
	return kit
}

// getVehicleKit retrieves an existing VehicleKit by vehicle GUID.
func (s *Server) getVehicleKit(vehicleGUID uint64) *VehicleKit {
	if s == nil {
		return nil
	}
	s.vehicleMu.RLock()
	defer s.vehicleMu.RUnlock()
	return s.vehicleKits[vehicleGUID]
}

// removeVehicleKit disassembles and deletes a VehicleKit.
// Reference: Unit::RemoveVehicleKit (Unit.cpp:8395).
func (s *Server) removeVehicleKit(vehicleGUID uint64) {
	if s == nil {
		return
	}
	s.vehicleMu.Lock()
	delete(s.vehicleKits, vehicleGUID)
	s.vehicleMu.Unlock()
}

// loadVehicleSeatAddons queries vehicle_seat_addon from world database.
// Reference: ObjectMgr::LoadVehicleSeatAddons (ObjectMgr.cpp:3638-3685).
func (s *Server) loadVehicleSeatAddons(ctx context.Context) {
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil {
		return
	}
	rows, err := s.WorldStore.DB.QueryContext(ctx, "SELECT SeatEntry, SeatOrientation, ExitParamX, ExitParamY, ExitParamZ, ExitParamO, ExitParamValue FROM vehicle_seat_addon")
	if err != nil {
		return
	}
	defer rows.Close()

	s.vehicleMu.Lock()
	defer s.vehicleMu.Unlock()
	if s.vehicleSeatAddons == nil {
		s.vehicleSeatAddons = make(map[uint32]*VehicleSeatAddon)
	}
	for rows.Next() {
		var addon VehicleSeatAddon
		var paramVal int
		if err := rows.Scan(&addon.SeatEntry, &addon.SeatOrientationOffset, &addon.ExitParamX, &addon.ExitParamY, &addon.ExitParamZ, &addon.ExitParamO, &paramVal); err == nil {
			addon.ExitParameter = uint8(paramVal)
			s.vehicleSeatAddons[addon.SeatEntry] = &addon
		}
	}
}

// loadVehicleAccessories queries vehicle_template_accessory from world database.
// Reference: ObjectMgr::LoadVehicleTemplateAccessories (ObjectMgr.cpp:3540-3585).
func (s *Server) loadVehicleAccessories(ctx context.Context) {
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil {
		return
	}
	rows, err := s.WorldStore.DB.QueryContext(ctx, "SELECT entry, accessory_entry, seat_id, minion, summontype, summontimer FROM vehicle_template_accessory")
	if err != nil {
		return
	}
	defer rows.Close()

	s.vehicleMu.Lock()
	defer s.vehicleMu.Unlock()
	if s.vehicleAccessories == nil {
		s.vehicleAccessories = make(map[uint32][]VehicleAccessory)
	}
	for rows.Next() {
		var entry, accEntry, timer uint32
		var seatID int8
		var minion bool
		var summonType uint8
		if err := rows.Scan(&entry, &accEntry, &seatID, &minion, &summonType, &timer); err == nil {
			s.vehicleAccessories[entry] = append(s.vehicleAccessories[entry], VehicleAccessory{
				AccessoryEntry: accEntry,
				SeatID:         seatID,
				IsMinion:       minion,
				SummonType:     summonType,
				SummonTime:     timer,
			})
		}
	}
}

// relocatePassengers updates global coordinates for all passengers on vehicle.
// Reference: Vehicle::RelocatePassengers (Vehicle.cpp:554-578).
func (s *Server) relocatePassengers(vehicleGUID uint64, transX, transY, transZ, transO float32) {
	if s == nil {
		return
	}
	kit := s.getVehicleKit(vehicleGUID)
	if kit == nil {
		return
	}

	kit.mu.RLock()
	type passPos struct {
		guid       uint64
		x, y, z, o float32
	}
	var relocations []passPos
	for _, seat := range kit.Seats {
		if !seat.IsEmpty() {
			pGuid := seat.Passenger.GUID
			attX := float32(0)
			attY := float32(0)
			attZ := float32(0)
			attO := float32(0)
			if seat.SeatInfo != nil {
				attX = seat.SeatInfo.AttachmentOffsetX
				attY = seat.SeatInfo.AttachmentOffsetY
				attZ = seat.SeatInfo.AttachmentOffsetZ
			}
			if seat.SeatAddon != nil {
				attO = seat.SeatAddon.SeatOrientationOffset
			}
			outX, outY, outZ, outO := CalculatePassengerPosition(transX, transY, transZ, transO, attX, attY, attZ, attO)
			relocations = append(relocations, passPos{guid: pGuid, x: outX, y: outY, z: outZ, o: outO})
		}
	}
	kit.mu.RUnlock()

	for _, rel := range relocations {
		if sess := s.findSessionByGUID(rel.guid); sess != nil && sess.playerLoaded && sess.player != nil {
			sess.player.X = rel.x
			sess.player.Y = rel.y
			sess.player.Z = rel.z
			sess.player.Orientation = rel.o
		}
	}
}

// sendPlayerVehicleData broadcasts SMSG_PLAYER_VEHICLE_DATA (0x4A7).
// Reference: Unit.cpp:8325-8328 & SpellAuraEffects.cpp:5026-5029.
func (s *session) sendPlayerVehicleData(vehicleID uint32) {
	if s == nil || s.player == nil {
		return
	}
	buf := protocol.NewBuffer(12)
	buf.WritePackedGUID(s.playerGUID)
	buf.WriteU32(vehicleID)
	_ = s.write(uint16(protocol.OpcodeSMSG_PLAYER_VEHICLE_DATA), buf.Bytes(), true)
	if s.server != nil {
		s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_PLAYER_VEHICLE_DATA), buf.Bytes(), s)
	}
}

// sendClientControl sends SMSG_CLIENT_CONTROL_UPDATE (0x159).
// Reference: Player::SetClientControl (Player.cpp:24256-24260).
func (s *session) sendClientControl(targetGUID uint64, allowMove bool) {
	if s == nil || s.conn == nil {
		return
	}
	buf := protocol.NewBuffer(10)
	buf.WritePackedGUID(targetGUID)
	if allowMove {
		buf.WriteU8(1)
	} else {
		buf.WriteU8(0)
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_CLIENT_CONTROL_UPDATE), buf.Bytes(), true)
}

// sendVehiclePetSpells generates SMSG_PET_SPELLS (0x179) with vehicle action bar.
// Reference: Player::VehicleSpellInitialize (Player.cpp:21183-21224).
func (s *session) sendVehiclePetSpells(vehicleGUID uint64, spells []uint32) {
	if s == nil || s.conn == nil {
		return
	}
	if vehicleGUID == 0 {
		buf := protocol.NewBuffer(8)
		buf.WriteU64(0)
		_ = s.write(uint16(protocol.OpcodeSMSG_PET_SPELLS), buf.Bytes(), true)
		return
	}

	buf := protocol.NewBuffer(64)
	buf.WriteU64(vehicleGUID)
	buf.WriteU16(0)     // pet family (0 for all vehicles)
	buf.WriteU32(0)     // duration
	buf.WriteU8(0)      // react state (REACT_PASSIVE = 0)
	buf.WriteU8(0)      // command state
	buf.WriteU16(0x800) // disableActions (0x800 for all vehicles)

	for i := 0; i < 10; i++ {
		if i < len(spells) && spells[i] > 0 {
			buf.WriteU32(spells[i] | (uint32(i+8) << 24))
		} else {
			buf.WriteU32(0)
		}
	}

	buf.WriteU8(0) // extra spells count
	buf.WriteU8(0) // cooldowns count

	_ = s.write(uint16(protocol.OpcodeSMSG_PET_SPELLS), buf.Bytes(), true)
}

func (s *session) sendCharmPetSpells(creatureGUID uint64, spells []uint32, reactState, commandState uint8) {
	if s == nil || s.conn == nil || creatureGUID == 0 {
		return
	}
	buf := protocol.NewBuffer(64)
	buf.WriteU64(creatureGUID)
	buf.WriteU16(0)
	buf.WriteU32(0)
	buf.WriteU8(reactState)
	buf.WriteU8(commandState)
	buf.WriteU16(0)
	for i := 0; i < 10; i++ {
		if i < len(spells) && spells[i] != 0 {
			buf.WriteU32(spells[i] | (uint32(i+8) << 24))
		} else {
			buf.WriteU32(0)
		}
	}
	buf.WriteU8(0)
	buf.WriteU8(0)
	_ = s.write(uint16(protocol.OpcodeSMSG_PET_SPELLS), buf.Bytes(), true)
}

// sendCancelExpectedRideVehicleAura sends SMSG_ON_CANCEL_EXPECTED_RIDE_VEHICLE_AURA (0x49D).
// Reference: Unit.cpp:8330-8331.
func (s *session) sendCancelExpectedRideVehicleAura() {
	if s == nil || s.conn == nil {
		return
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_ON_CANCEL_EXPECTED_RIDE_VEHICLE_AURA), nil, true)
}

// dropBattlegroundFlagIfCarried drops carried flag upon entering a vehicle.
// Reference: VehicleJoinEvent::Execute (Vehicle.cpp:846-848).
func (s *session) dropBattlegroundFlagIfCarried() {
	if s == nil || s.player == nil || s.server == nil {
		return
	}
	if s.player.Map == WSGMapID {
		s.server.handleWSGPlayerLeave(s)
	} else if s.player.Map == EOTSMapID {
		s.server.handleEOTSPlayerLeave(s)
	}
}

// enterVehicle places the session's player into the specified vehicle and seat.
// Reference: Unit::_EnterVehicle (Unit.cpp:8320) & VehicleJoinEvent::Execute (Vehicle.cpp:790-910).
func (s *session) enterVehicle(vehicleGUID uint64, seatID int8) {
	if s == nil || s.player == nil {
		return
	}

	// Arena check: players cannot board player vehicles in arena
	if s.server != nil && IsArenaMap(s.player.Map) {
		if targetSess := s.server.findSessionByGUID(vehicleGUID); targetSess != nil {
			return
		}
	}

	s.dropBattlegroundFlagIfCarried()
	s.player.MountDisplayID = 0

	actualSeat := seatID
	playerVehicleID := uint32(0)
	if s.server != nil {
		if kit := s.server.getVehicleKit(vehicleGUID); kit != nil {
			assigned, seatInfo, ok := kit.AddPassenger(s.playerGUID, seatID)
			if !ok {
				return
			}
			actualSeat = assigned
			if kit.IsPlayer {
				playerVehicleID = kit.VehicleID
			}
			if seatInfo != nil {
				if seatInfo.CanControl() {
					s.sendClientControl(vehicleGUID, true)
					spells := s.server.loadCreatureSpells(context.Background(), kit.CreatureEntry)
					s.sendVehiclePetSpells(vehicleGUID, spells)
				}
				if seatInfo.HasFlag(wotlk.VehicleSeatFlagPassengerNotSelectable) {
					s.player.UnitFlags |= 0x02000000 // UNIT_FLAG_NOT_SELECTABLE
				}
			}
		}
	}

	s.player.VehicleGUID = vehicleGUID
	s.player.VehicleSeat = actualSeat
	if playerVehicleID != 0 {
		s.sendPlayerVehicleData(playerVehicleID)
	}
	s.sendCancelExpectedRideVehicleAura()
	s.sendPlayerMountUpdate()
	s.sendPlayerUpdate()
}

// exitVehicle removes the session's player from their current vehicle.
// Reference: Unit::_ExitVehicle (Unit.cpp:8365) & Vehicle::RemovePassenger (Vehicle.cpp:498-543).
func (s *session) exitVehicle() {
	if s == nil || s.player == nil || s.player.VehicleGUID == 0 {
		return
	}

	oldVehGUID := s.player.VehicleGUID
	playerVehicleID := uint32(0)
	if s.server != nil {
		if kit := s.server.getVehicleKit(oldVehGUID); kit != nil {
			if kit.IsPlayer {
				playerVehicleID = kit.VehicleID
			}
			_, seatInfo, _ := kit.RemovePassenger(s.playerGUID)
			if seatInfo != nil {
				if seatInfo.CanControl() {
					s.sendClientControl(oldVehGUID, false)
					s.sendVehiclePetSpells(0, nil)
				}
				if seatInfo.HasFlag(wotlk.VehicleSeatFlagPassengerNotSelectable) {
					s.player.UnitFlags &^= 0x02000000
				}
				addon := kit.GetSeatAddonForSeatOfPassenger(s.playerGUID)
				if addon != nil && addon.ExitParameter == wotlk.VehicleExitParamDest {
					s.player.X = addon.ExitParamX
					s.player.Y = addon.ExitParamY
					s.player.Z = addon.ExitParamZ
					s.player.Orientation = addon.ExitParamO
				}
			}
		}
	}

	s.player.VehicleGUID = 0
	s.player.VehicleSeat = 0
	if playerVehicleID != 0 {
		s.sendPlayerVehicleData(0)
	}
	s.sendPlayerUpdate()
}

// handleChangeSeatsOnControlledVehicle processes CMSG_CHANGE_SEATS_ON_CONTROLLED_VEHICLE (0x49B).
// Reference: WorldSession::HandleChangeSeatsOnControlledVehicle (VehicleHandler.cpp:52-127).
func (s *session) handleChangeSeatsOnControlledVehicle(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) == 0 {
		return true
	}
	targetSeat := int8(payload[0])
	if len(payload) >= 9 {
		r := protocol.NewReader(payload)
		_, _ = r.ReadU64()
		if sByte, err := r.ReadU8(); err == nil {
			targetSeat = int8(sByte)
		}
	}

	if s.server != nil && s.player.VehicleGUID != 0 {
		if kit := s.server.getVehicleKit(s.player.VehicleGUID); kit != nil {
			newSeat, seatInfo, ok := kit.SwitchSeat(s.playerGUID, targetSeat)
			if !ok {
				return true
			}
			s.player.VehicleSeat = newSeat
			if seatInfo != nil && seatInfo.CanControl() {
				s.sendClientControl(kit.VehicleGUID, true)
				spells := s.server.loadCreatureSpells(ctx, kit.CreatureEntry)
				s.sendVehiclePetSpells(kit.VehicleGUID, spells)
			}
			s.sendPlayerUpdate()
			return true
		}
	}

	s.player.VehicleSeat = targetSeat
	s.sendPlayerUpdate()
	return true
}

// handleControllerEjectPassenger processes CMSG_CONTROLLER_EJECT_PASSENGER (0x4A9).
// Reference: WorldSession::HandleEjectPassenger (VehicleHandler.cpp:151-188).
func (s *session) handleControllerEjectPassenger(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	r := protocol.NewReader(payload)
	passGUID, err := r.ReadU64()
	if err != nil || passGUID == 0 {
		return true
	}

	if s.server != nil {
		vehGUID := s.playerGUID
		if s.player.VehicleGUID != 0 {
			vehGUID = s.player.VehicleGUID
		}
		if passSess := s.server.findSessionByGUID(passGUID); passSess != nil && passSess.playerLoaded && passSess.player != nil {
			if passSess.player.VehicleGUID == vehGUID || passSess.player.VehicleGUID == s.playerGUID {
				passSess.exitVehicle()
			}
		}
	}
	return true
}

// handleDismissControlledVehicle processes CMSG_DISMISS_CONTROLLED_VEHICLE (0x46D).
// Reference: WorldSession::HandleDismissControlledVehicle (VehicleHandler.cpp:27-50).
func (s *session) handleDismissControlledVehicle(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}

	vehGUID := s.playerGUID
	if s.player.VehicleGUID != 0 {
		vehGUID = s.player.VehicleGUID
	}

	s.exitVehicle()
	if s.server != nil {
		s.server.sessionsMu.RLock()
		for sess := range s.server.sessions {
			if sess != s && sess.playerLoaded && sess.player != nil && (sess.player.VehicleGUID == vehGUID || sess.player.VehicleGUID == s.playerGUID) {
				sess.exitVehicle()
			}
		}
		s.server.sessionsMu.RUnlock()

		s.server.removeVehicleKit(vehGUID)
		s.sendPlayerVehicleData(0)
	}
	return true
}

// handlePlayerVehicleEnter processes CMSG_PLAYER_VEHICLE_ENTER (0x46E).
// Reference: WorldSession::HandleEnterPlayerVehicle (VehicleHandler.cpp:129-149).
func (s *session) handlePlayerVehicleEnter(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	r := protocol.NewReader(payload)
	vehGUID, err := r.ReadU64()
	if err != nil || vehGUID == 0 {
		return true
	}
	seat := int8(1)
	if len(payload) >= 9 {
		sByte, err := r.ReadU8()
		if err == nil {
			seat = int8(sByte)
		}
	}
	s.enterVehicle(vehGUID, seat)
	return true
}

// handleRequestVehicleExit processes CMSG_REQUEST_VEHICLE_EXIT (0x46F).
// Reference: WorldSession::HandleRequestVehicleExit (VehicleHandler.cpp:190-205).
func (s *session) handleRequestVehicleExit(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	if s.server != nil && s.player.VehicleGUID != 0 {
		if kit := s.server.getVehicleKit(s.player.VehicleGUID); kit != nil {
			_, seatInfo, _ := kit.GetSeatForPassenger(s.playerGUID)
			if seatInfo != nil && !seatInfo.CanEnterOrExit() {
				return true
			}
		}
	}
	s.exitVehicle()
	return true
}

// handleRequestVehicleNextSeat processes CMSG_REQUEST_VEHICLE_NEXT_SEAT (0x470).
// Reference: WorldSession::HandleChangeSeatsOnControlledVehicle (VehicleHandler.cpp:77).
func (s *session) handleRequestVehicleNextSeat(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	if s.server != nil && s.player.VehicleGUID != 0 {
		if kit := s.server.getVehicleKit(s.player.VehicleGUID); kit != nil {
			if nextSeat, ok := kit.GetNextEmptySeat(s.player.VehicleSeat, true); ok {
				if assigned, seatInfo, switched := kit.SwitchSeat(s.playerGUID, nextSeat); switched {
					s.player.VehicleSeat = assigned
					if seatInfo != nil && seatInfo.CanControl() {
						s.sendClientControl(kit.VehicleGUID, true)
						spells := s.server.loadCreatureSpells(ctx, kit.CreatureEntry)
						s.sendVehiclePetSpells(kit.VehicleGUID, spells)
					}
					s.sendPlayerUpdate()
				}
			}
			return true
		}
	}
	s.player.VehicleSeat++
	s.sendPlayerUpdate()
	return true
}

// handleRequestVehiclePrevSeat processes CMSG_REQUEST_VEHICLE_PREV_SEAT (0x471).
// Reference: WorldSession::HandleChangeSeatsOnControlledVehicle (VehicleHandler.cpp:74).
func (s *session) handleRequestVehiclePrevSeat(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	if s.server != nil && s.player.VehicleGUID != 0 {
		if kit := s.server.getVehicleKit(s.player.VehicleGUID); kit != nil {
			if prevSeat, ok := kit.GetNextEmptySeat(s.player.VehicleSeat, false); ok {
				if assigned, seatInfo, switched := kit.SwitchSeat(s.playerGUID, prevSeat); switched {
					s.player.VehicleSeat = assigned
					if seatInfo != nil && seatInfo.CanControl() {
						s.sendClientControl(kit.VehicleGUID, true)
						spells := s.server.loadCreatureSpells(ctx, kit.CreatureEntry)
						s.sendVehiclePetSpells(kit.VehicleGUID, spells)
					}
					s.sendPlayerUpdate()
				}
			}
			return true
		}
	}
	if s.player.VehicleSeat > 0 {
		s.player.VehicleSeat--
		s.sendPlayerUpdate()
	}
	return true
}

// handleRequestVehicleSwitchSeat processes CMSG_REQUEST_VEHICLE_SWITCH_SEAT (0x479).
// Reference: WorldSession::HandleChangeSeatsOnControlledVehicle (VehicleHandler.cpp:108-123).
func (s *session) handleRequestVehicleSwitchSeat(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 1 {
		return true
	}
	seat := int8(payload[0])
	if len(payload) >= 2 {
		r := protocol.NewReader(payload)
		if _, err := r.ReadPackedGUID(); err == nil {
			if sByte, err := r.ReadU8(); err == nil {
				seat = int8(sByte)
			}
		} else if len(payload) >= 9 {
			r = protocol.NewReader(payload)
			_, _ = r.ReadU64()
			if sByte, err := r.ReadU8(); err == nil {
				seat = int8(sByte)
			}
		}
	}

	if s.server != nil && s.player.VehicleGUID != 0 {
		if kit := s.server.getVehicleKit(s.player.VehicleGUID); kit != nil {
			if assigned, seatInfo, switched := kit.SwitchSeat(s.playerGUID, seat); switched {
				s.player.VehicleSeat = assigned
				if seatInfo != nil && seatInfo.CanControl() {
					s.sendClientControl(kit.VehicleGUID, true)
					spells := s.server.loadCreatureSpells(ctx, kit.CreatureEntry)
					s.sendVehiclePetSpells(kit.VehicleGUID, spells)
				}
				s.sendPlayerUpdate()
			}
			return true
		}
	}

	s.player.VehicleSeat = seat
	s.sendPlayerUpdate()
	return true
}
