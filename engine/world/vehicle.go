package world

import (
	"context"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
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
	mu            sync.RWMutex
	VehicleGUID   uint64
	VehicleID     uint32
	CreatureEntry uint32
	IsPlayer      bool
	VehicleInfo   *wotlk.VehicleEntry
	// BaseMovementFlags2 is the Vehicle::InitMovementInfoForBase
	// (Vehicle.cpp:617-631) twin: the Vehicle.dbc Flags mapped onto
	// MovementFlags2 at kit creation (the Vehicle ctor calls
	// InitMovementInfoForBase, Vehicle.cpp:57). Set once in
	// createVehicleKit; read by the base's movement-info consumers
	// (player create block, creature teleport twin). Set from the DBC
	// flags in hand at creation, like the VehicleInfo rows above, so
	// readers take no extra lock.
	BaseMovementFlags2   uint16
	Seats                map[int8]*VehicleSeat
	UsableSeatNum        uint32
	Accessories          []VehicleAccessory
	InstalledAccessories []uint64
	AccessoriesInstalled bool // install latch: the spawn InstallAllAccessories attempt runs once per kit; the evade Reset(true) reinstall bypasses it (resetVehicleKitOnReachHome, minion rows only)
	InstallEventFired    bool // install latch: VEHICLE_EVENT_ON_INSTALL fired once per kit, on the first passenger sync (Creature::AddToWorld Install twin)
	// uninstalling is Vehicle::Uninstall's STATUS_UNINSTALLING
	// (Vehicle.cpp:108-118): set when the kit leaves the server maps,
	// before the passenger exit sweep runs. C++ is single-threaded,
	// so the status checks in AddPassenger (:428-429) and
	// InstallAccessory (:388-389) are the only guard; Go's map delete
	// alone leaves a stale kit pointer boardable after the sweep
	// snapshotted its seats. Guarded by mu.
	uninstalling         bool
	LastShootPos         [3]float32
	X, Y, Z, Orientation float32
	// movementInfo is the driven vehicle's m_movementInfo
	// (MovementHandler.cpp:389, MiscHandler.cpp:1398): the acked flags /
	// time / wholesale packet of the vehicle mover, kept off the driver's
	// session store so player-scoped readers keep the frozen pre-boarding
	// flags. Guarded by mu.
	movementInfo    movementInfo
	movementInfoSet bool
}

// setPosition records the vehicle base's canonical world position.
// Reference: Unit::UpdatePosition (Unit.cpp:13395-13424) — the driven
// vehicle is the mover, so its packets drive the base's position.
func (v *VehicleKit) setPosition(x, y, z, o float32) {
	if v == nil {
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	v.X, v.Y, v.Z, v.Orientation = x, y, z, o
}

// setMovementInfo stores a wholesale movement packet on the vehicle kit
// (MovementHandler.cpp:389: mover->m_movementInfo = movementInfo).
func (v *VehicleKit) setMovementInfo(info movementInfo) {
	if v == nil {
		return
	}
	if info.Transport != nil {
		transport := *info.Transport
		info.Transport = &transport
	}
	v.mu.Lock()
	v.movementInfo, v.movementInfoSet = info, true
	v.mu.Unlock()
}

// movementInfoSnapshot returns the kit's stored movement info, if any.
// Read by exitVehicle for the Vehicle::RemovePassenger transport arm
// (Vehicle.cpp:525-528).
func (v *VehicleKit) movementInfoSnapshot() (movementInfo, bool) {
	if v == nil {
		return movementInfo{}, false
	}
	v.mu.RLock()
	info, exists := v.movementInfo, v.movementInfoSet
	v.mu.RUnlock()
	if info.Transport != nil {
		transport := *info.Transport
		info.Transport = &transport
	}
	return info, exists
}

// setMovementFlags replaces only the flags of the stored movement info
// (MiscHandler.cpp:1398 copies flags, not the whole packet).
func (v *VehicleKit) setMovementFlags(flags uint32) {
	if v == nil {
		return
	}
	v.mu.Lock()
	v.movementInfo.Flags = flags
	v.movementInfoSet = true
	v.mu.Unlock()
}

// addMovementInfoTime advances the stored movement time
// (MovementHandler.cpp:651: mover->m_movementInfo.time += timeSkipped).
func (v *VehicleKit) addMovementInfoTime(delta uint32) {
	if v == nil {
		return
	}
	v.mu.Lock()
	v.movementInfo.Time += delta
	v.movementInfoSet = true
	v.mu.Unlock()
}

type vehicleKitKey struct {
	MapID, InstanceID uint32
	GUID              uint64
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

// vehicleSeatAddonForPassenger returns the seat addon for a passenger seated on
// the given vehicle base, across every kit registered under the base GUID. A
// passenger seated in a seat with no addon row reads nil, the ExitParamNone
// equivalent. The caller captures this BEFORE RemovePassenger: after removal the
// seat is empty and the lookup finds nothing.
// Reference: Unit::_ExitVehicle (Unit.cpp:13193) + Vehicle::GetSeatAddonForSeatOfPassenger (Vehicle.cpp:360-367).
func (s *Server) vehicleSeatAddonForPassenger(vehicleGUID, passengerGUID uint64) *VehicleSeatAddon {
	if s == nil || vehicleGUID == 0 || passengerGUID == 0 {
		return nil
	}
	for _, loc := range s.findVehicleKits(vehicleGUID) {
		if loc.kit == nil {
			continue
		}
		if _, seatInfo, addon := loc.kit.GetSeatForPassenger(passengerGUID); seatInfo != nil {
			return addon
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
	if len(v.Seats) == 0 {
		return -1, false
	}
	// Vehicle::GetNextEmptySeat (Vehicle.cpp:319-345) walks the SeatMap
	// (std::map, ascending seat id) from the current seat, wrapping at
	// the ends, and returns end at once when the current seat is absent
	// from the map. The old index-modulo walk assumed dense 0..n-1 keys,
	// so a kit whose Vehicle.dbc row skips a SeatIDs slot (createVehicleKit
	// `continue`s on a 0 id) never reached seats past the gap, and it
	// never returned the start seat itself when empty and enterable.
	// HasPendingEventForSeat has no Go model and stays unmodeled.
	keys := make([]int8, 0, len(v.Seats))
	for id := range v.Seats {
		keys = append(keys, id)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	start := -1
	for i, id := range keys {
		if id == currSeatID {
			start = i
			break
		}
	}
	if start < 0 {
		return -1, false
	}
	step := 1
	if !forward {
		step = -1
	}
	cur := start
	for {
		s := v.Seats[keys[cur]]
		if s != nil && s.IsEmpty() && s.SeatInfo != nil && (s.SeatInfo.CanEnterOrExit() || s.SeatInfo.IsUsableByOverride()) {
			return keys[cur], true
		}
		cur = (cur + step + len(keys)) % len(keys)
		if cur == start {
			return -1, false
		}
	}
}

// isUninstalling reports whether the kit is being uninstalled
// (Vehicle::Uninstall, Vehicle.cpp:118 sets STATUS_UNINSTALLING
// before RemoveAllPassengers). A board racing the removal reads it
// through AddPassenger / the install sites below.
func (v *VehicleKit) isUninstalling() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.uninstalling
}

// AddPassenger attempts to board a passenger onto the vehicle at seatID (-1 for first available).
// Reference: Vehicle::AddPassenger (Vehicle.cpp:426-484).
func (v *VehicleKit) AddPassenger(passengerGUID uint64, seatID int8, wasUnselectable bool) (assignedSeat int8, seatInfo *wotlk.VehicleSeatEntry, ok bool) {
	v.mu.Lock()
	defer v.mu.Unlock()

	// Vehicle::AddPassenger (Vehicle.cpp:428-434): no boarding while
	// the vehicle is uninstalling — the status check runs before the
	// already-seated scan, as in C++. A same-seat re-entry on a kit
	// mid-removal fails here instead of re-running the JoinEvent legs
	// on a detached kit; the exit sweep ejects the passenger.
	if v.uninstalling {
		return -1, nil, false
	}

	// If already in vehicle, return current seat
	for id, s := range v.Seats {
		if s.Passenger.GUID == passengerGUID {
			return id, s.SeatInfo, true
		}
	}

	if seatID < 0 {
		// Vehicle::AddPassenger (Vehicle.cpp:453-460) scans the SeatMap in
		// key order for the first empty enterable seat; the old 0..len-1
		// scan missed seats past a skipped SeatIDs slot, same gap as
		// GetNextEmptySeat above.
		keys := make([]int8, 0, len(v.Seats))
		for id := range v.Seats {
			keys = append(keys, id)
		}
		sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
		for _, id := range keys {
			s := v.Seats[id]
			if s != nil && s.IsEmpty() && s.SeatInfo != nil && (s.SeatInfo.CanEnterOrExit() || s.SeatInfo.IsUsableByOverride()) {
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

	// Vehicle::RemovePassenger (Vehicle.cpp:928-930): the exit leg needs to
	// know whether the passenger carried UNIT_FLAG_NOT_SELECTABLE before
	// boarding, so the pre-boarding state is recorded here.
	seat.Passenger = PassengerInfo{GUID: passengerGUID, IsUnselectable: wasUnselectable}
	if seat.SeatInfo != nil && seat.SeatInfo.CanEnterOrExit() && v.UsableSeatNum > 0 {
		v.UsableSeatNum--
	}
	return seatID, seat.SeatInfo, true
}

// RemovePassenger removes the passenger from the vehicle kit.
// Reference: Vehicle::RemovePassenger (Vehicle.cpp:498-543).
func (v *VehicleKit) RemovePassenger(passengerGUID uint64) (seatID int8, seatInfo *wotlk.VehicleSeatEntry, wasUnselectable bool, ok bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	for id, s := range v.Seats {
		if s.Passenger.GUID == passengerGUID {
			// C++ finds the seat iterator before resetting the passenger;
			// the recorded pre-boarding unselectable state rides back out
			// so the exit leg can restore it exactly (Vehicle.cpp:928-930).
			wasUnselectable = s.Passenger.IsUnselectable
			s.Passenger.Reset()
			if s.SeatInfo != nil && s.SeatInfo.CanEnterOrExit() {
				v.UsableSeatNum++
			}
			return id, s.SeatInfo, wasUnselectable, true
		}
	}
	return -1, nil, false, false
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

	// Unit::ChangeSeat (Unit.cpp:13133-13170) re-applies the ride aura on
	// the new seat, which runs RemovePassenger + AddPassenger — so the
	// usable-seat count moves when the switch crosses the
	// CanEnterOrExit boundary: leaving a usable seat frees one,
	// taking a usable seat consumes one.
	if currSeat.SeatInfo != nil && destSeat.SeatInfo != nil {
		if currSeat.SeatInfo.CanEnterOrExit() && !destSeat.SeatInfo.CanEnterOrExit() {
			v.UsableSeatNum++
		} else if !currSeat.SeatInfo.CanEnterOrExit() && destSeat.SeatInfo.CanEnterOrExit() {
			if v.UsableSeatNum > 0 {
				v.UsableSeatNum--
			}
		}
	}

	destSeat.Passenger = currSeat.Passenger
	currSeat.Passenger.Reset()

	return newSeatID, destSeat.SeatInfo, true
}

// passengerWasUnselectable returns the pre-boarding UNIT_FLAG_NOT_SELECTABLE
// state AddPassenger recorded for the passenger (Vehicle.cpp:822),
// which Vehicle::RemovePassenger (Vehicle.cpp:512-514) consults when
// deciding whether the flag clears on exit. SwitchSeat carries the
// passenger record — IsUnselectable included — to the new seat, so the
// stored value survives a switch.
func (v *VehicleKit) passengerWasUnselectable(passengerGUID uint64) bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	for _, s := range v.Seats {
		if s.Passenger.GUID == passengerGUID {
			return s.Passenger.IsUnselectable
		}
	}
	return false
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

// CharmerGUID returns the GUID of the vehicle's controlling passenger: the
// occupant of a CAN_CONTROL seat (VehicleJoinEvent::Execute,
// Vehicle.cpp:871-881 grants CHARM_TYPE_VEHICLE on boarding such a seat when
// the base is a creature). This is the Go twin of Unit::GetCharmer for the
// vehicle-mounted duel leg (Unit.cpp:841).
func (v *VehicleKit) CharmerGUID() uint64 {
	v.mu.RLock()
	defer v.mu.RUnlock()
	for _, seat := range v.Seats {
		if seat.SeatInfo != nil && seat.SeatInfo.CanControl() && seat.Passenger.GUID != 0 {
			return seat.Passenger.GUID
		}
	}
	return 0
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

// triggerVehicleEvent dispatches an Eluna VEHICLE_EVENT_* hook
// (RegisterVehicleEvent) as (event, vehicle, ...args), mirroring
// Eluna::VehicleHooks (VehicleHooks.cpp) via ScriptMgr::OnInstall /
// OnUninstall / OnAddPassenger / OnRemovePassenger / OnInstallAccessory.
// C++ pushes a Vehicle userdata; Go has no Vehicle object type, so the
// vehicle argument is the base creature object. A nil vehicle object means
// no handler can see the base, so the event does not fire — C++ always
// has a live base. These hooks never cancel: the C++ call sites use
// CallAllFunctions, not the Bool form. Callers fire only for creature
// bases (Vehicle.cpp gates every fire on TYPEID_UNIT); player bases
// never fire.
func (srv *Server) triggerVehicleEvent(event int, vehicle *scripting.Object, args ...any) {
	if srv == nil || vehicle == nil || srv.Features == nil || srv.Features.Scripts == nil {
		return
	}
	if _, err := srv.Features.Scripts.TriggerVehicleEvent(context.Background(), event, append([]any{vehicle}, args...)...); err != nil {
		srv.debug("lua vehicle event failed", "event", event, "error", err)
	}
}

// vehicleBaseMovementFlags2 maps Vehicle.dbc Flags onto the MovementFlags2
// bits Vehicle::InitMovementInfoForBase (Vehicle.cpp:617-631) ORs into the
// base's m_movementInfo.flags2. The mapping is NOT 1:1 — note the cross:
// VEHICLE_FLAG_ALLOW_PITCHING (0x10) sets MOVEMENTFLAG2_ALWAYS_ALLOW_PITCHING
// (0x20), while VEHICLE_FLAG_FULLSPEEDPITCHING (0x20) sets
// MOVEMENTFLAG2_FULL_SPEED_PITCHING (0x10) (VehicleDefines.h:39-43,
// UnitDefines.h:273-278). The wotlk VehicleFlag constants already carry the
// DBC values (engine/data/wotlk/vehicle.go:17-21).
func vehicleBaseMovementFlags2(vehicleFlags uint32) uint16 {
	var flags2 uint16
	if vehicleFlags&wotlk.VehicleFlagNoStrafe != 0 {
		flags2 |= movement2NoStrafe
	}
	if vehicleFlags&wotlk.VehicleFlagNoJumping != 0 {
		flags2 |= movement2NoJumping
	}
	if vehicleFlags&wotlk.VehicleFlagFullSpeedTurning != 0 {
		flags2 |= movement2FullSpeedTurning
	}
	if vehicleFlags&wotlk.VehicleFlagAllowPitching != 0 {
		flags2 |= movement2AlwaysAllowPitching
	}
	if vehicleFlags&wotlk.VehicleFlagFullSpeedPitching != 0 {
		flags2 |= movement2FullSpeedPitching
	}
	return flags2
}

// createVehicleKit initializes a VehicleKit for a given GUID, caching seats and accessories.
// Reference: Unit::CreateVehicleKit (Unit.cpp:8320-8336) & Vehicle::Vehicle (Vehicle.cpp:36-58).
func (s *Server) createVehicleKit(mapID, instanceID uint32, vehicleGUID uint64, vehicleID uint32, creatureEntry uint32, isPlayer bool) *VehicleKit {
	if s == nil {
		return nil
	}
	s.vehicleMu.Lock()
	defer s.vehicleMu.Unlock()

	key := vehicleKitKey{MapID: mapID, InstanceID: instanceID, GUID: vehicleGUID}

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

	if isPlayer {
		if s.vehicleKits == nil {
			s.vehicleKits = make(map[uint64]*VehicleKit)
		}
		if s.instanceVehicleKits == nil {
			s.instanceVehicleKits = make(map[vehicleKitKey]*VehicleKit)
		}
		s.vehicleKits[vehicleGUID] = kit
		s.instanceVehicleKits[key] = kit
	} else {
		if s.instanceVehicleKits == nil {
			s.instanceVehicleKits = make(map[vehicleKitKey]*VehicleKit)
		}
		s.instanceVehicleKits[key] = kit
	}
	// Vehicle::InitMovementInfoForBase (Vehicle.cpp:617-631), called from
	// the Vehicle ctor (Vehicle.cpp:57): the base's m_movementInfo.flags2
	// gains the DBC-mapped bits at kit creation. Stored on the kit, not
	// written into the base's motion/session store here: the
	// creature-materialization call sites run under motionMu, which a
	// motion-store write would need to re-take, and the kit is the
	// existing home for creation-time DBC facts (VehicleInfo above); the
	// base's movement-info consumers OR the bits in at read time.
	if kit.VehicleInfo != nil {
		kit.BaseMovementFlags2 = vehicleBaseMovementFlags2(kit.VehicleInfo.Flags)
	}
	return kit
}

func (s *Server) getVehicleKit(mapID, instanceID uint32, vehicleGUID uint64) *VehicleKit {
	if s == nil {
		return nil
	}
	s.vehicleMu.RLock()
	defer s.vehicleMu.RUnlock()
	if kit := s.instanceVehicleKits[vehicleKitKey{MapID: mapID, InstanceID: instanceID, GUID: vehicleGUID}]; kit != nil {
		return kit
	}
	return nil
}

// setPlayerVehicleNPCFlag mirrors the player half of the Vehicle NPC-flag
// lifecycle (Vehicle.cpp:52-55 ctor, :508-509 RemovePassenger, :825-832
// VehicleJoinEvent::Execute, Unit.cpp:12070-12071 RemoveVehicleKit): a
// player base carries UNIT_NPC_FLAG_PLAYER_VEHICLE exactly while a usable
// (CanEnterOrExit) seat is free. A nil kit means the kit is gone, so the
// flag clears. The bit lives on playerState.NPCFlags and rides the create
// block (buildPlayerUpdateForRecipient) and the values update pushed here
// to self and observers on change — UNIT_NPC_FLAGS is DYNAMIC
// (UpdateFields.h), so C++ sends it to every viewer (Object.cpp:481-498
// _fieldNotifyFlags leg). Call with neither playerStateMu nor vehicleMu held.
func (s *session) setPlayerVehicleNPCFlag(kit *VehicleKit) {
	if s == nil || s.player == nil || s.server == nil {
		return
	}
	usable := false
	if kit != nil {
		kit.mu.RLock()
		usable = kit.UsableSeatNum > 0
		kit.mu.RUnlock()
	}
	s.playerStateMu.Lock()
	changed := false
	if usable {
		if s.player.NPCFlags&unitNPCFlagPlayerVehicle == 0 {
			s.player.NPCFlags |= unitNPCFlagPlayerVehicle
			changed = true
		}
	} else {
		if s.player.NPCFlags&unitNPCFlagPlayerVehicle != 0 {
			s.player.NPCFlags &^= unitNPCFlagPlayerVehicle
			changed = true
		}
	}
	flags := s.player.NPCFlags
	s.playerStateMu.Unlock()
	if !changed {
		return
	}
	packet, err := s.server.buildPlayerValuesUpdate(s.playerGUID, map[int]uint32{unitFieldNPCFlags: flags})
	if err != nil {
		return
	}
	_ = s.write(packet.Opcode, packet.Payload.Bytes(), true)
	s.server.broadcastPlayerValuesUpdateFromSession(s, map[int]uint32{unitFieldNPCFlags: flags})
}

// syncVehicleBaseNPCFlag mirrors the Vehicle NPC-flag lifecycle for a
// creature base (Vehicle.cpp:52-55 ctor, :508-509 RemovePassenger,
// :825-832 VehicleJoinEvent::Execute): the base carries
// UNIT_NPC_FLAG_SPELLCLICK exactly while a usable (CanEnterOrExit) seat
// is free — the client shows the spell-click cursor only then. Boarding
// the last usable seat clears it; freeing one sets it again. The write
// latches NPCFlagsKnown (the Lua SetFlag precedent) so the spawn rebuild
// keeps the live value instead of re-affirming the template, and the
// change broadcasts like any SetFlag. A player base carries
// UNIT_NPC_FLAG_PLAYER_VEHICLE the same way in C++; that half rides the
// session (playerState.NPCFlags via setPlayerVehicleNPCFlag) since players
// have no creature motion. A missing base motion and no session is a
// no-op. Callers run it after a board/unseat that may move
// UsableSeatNum across zero; it is idempotent, so no caller needs to
// pre-check the transition. Call with neither motionMu nor vehicleMu
// held: the kit lookup resolves a base whose kit is keyed on another
// map (a teleported base) through the findVehicleKits fallback.
func (s *Server) syncVehicleBaseNPCFlag(mapID, instanceID uint32, baseGUID uint64) {
	if s == nil || baseGUID == 0 {
		return
	}
	kit := s.getVehicleKit(mapID, instanceID, baseGUID)
	kitMapID, kitInstanceID := mapID, instanceID
	if kit == nil {
		for _, loc := range s.findVehicleKits(baseGUID) {
			kit = loc.kit
			kitMapID, kitInstanceID = loc.key.MapID, loc.key.InstanceID
			break
		}
		if kit == nil {
			// No kit: a player base drops the flag (Unit::RemoveVehicleKit,
			// Unit.cpp:12070-12071); a creature base has no motion to clear
			// through here.
			if sess := s.findSessionByGUID(baseGUID); sess != nil {
				sess.setPlayerVehicleNPCFlag(nil)
			}
			return
		}
	}
	kit.mu.RLock()
	usable := kit.UsableSeatNum > 0
	kit.mu.RUnlock()
	base := s.findCreatureMotion(kitMapID, kitInstanceID, baseGUID)
	if base == nil {
		// Player base: the creature motion lookup misses, so the
		// UNIT_NPC_FLAG_PLAYER_VEHICLE half rides the session instead.
		if sess := s.findSessionByGUID(baseGUID); sess != nil {
			sess.setPlayerVehicleNPCFlag(kit)
		}
		return
	}
	s.motionMu.Lock()
	changed := false
	if usable {
		if base.NPCFlags&unitNPCFlagSpellclick == 0 {
			base.NPCFlags |= unitNPCFlagSpellclick
			changed = true
		}
	} else {
		if base.NPCFlags&unitNPCFlagSpellclick != 0 {
			base.NPCFlags &^= unitNPCFlagSpellclick
			changed = true
		}
	}
	var flags uint32
	if changed {
		base.NPCFlagsKnown = true
		flags = base.NPCFlags
	}
	s.motionMu.Unlock()
	if changed {
		s.broadcastCreatureValuesUpdateInInstance(kitMapID, kitInstanceID, baseGUID, map[int]uint32{unitFieldNPCFlags: flags})
	}
}

// applyVehicleCtorNPCFlag mirrors the Vehicle ctor flag arm
// (Vehicle.cpp:52-55) for a base whose kit was just created at spawn:
// the base carries UNIT_NPC_FLAG_SPELLCLICK exactly while a usable
// (CanEnterOrExit) seat is free, overwriting the template value. The
// spawn materialization callers hold motionMu, so the flag is set
// directly on the fresh motion instead of through
// syncVehicleBaseNPCFlag; the spawn create-block is built from these
// fields afterwards. NPCFlagsKnown latches the ctor value the way the
// sync helper latches the live one.
func (s *Server) applyVehicleCtorNPCFlag(motion *creatureMotion, kit *VehicleKit) {
	if motion == nil || kit == nil {
		return
	}
	kit.mu.RLock()
	usable := kit.UsableSeatNum > 0
	kit.mu.RUnlock()
	if usable {
		motion.NPCFlags |= unitNPCFlagSpellclick
	} else {
		motion.NPCFlags &^= unitNPCFlagSpellclick
	}
	motion.NPCFlagsKnown = true
}

// applyVehicleImmunities mirrors Vehicle::ApplyAllImmunities
// (Vehicle.cpp:160-223): the immunity + root arms of Vehicle::Reset.
// Every vehicle base is immune to the knockback effects; a mechanical
// non-boss base additionally takes the heal/dispel effect rows, the
// periodic-heal / school-immunity / unattackable / absorb /
// damage-shield / split-damage / resistance / stat / damage-taken state
// rows, and the banish / shield / immune-shield mechanic rows — all
// granted by spell 0, so addCreatureSpellImmuneRow dedupes re-application.
// The rows land in the ScriptSpellImmune store the creature per-effect
// eval consults (immunity.go creatureImmuneToSpellEffect: IMMUNITY_EFFECT,
// IMMUNITY_MECHANIC, IMMUNITY_STATE); the mechanical heal row overlaps
// the template arm (Creature.cpp:2336-2341) and is applied anyway,
// matching C++ order. Fixed-position vehicles (cannons) and the hardcoded
// cannon entries root the base via ScriptRooted, the SetControlled
// (UNIT_STATE_ROOT) twin; the siege-vehicle ids root and take the
// slow-aura immunity, while the salvaged ids drop the damage-taken row
// the mechanical arm just added (apply=false).
// Callers are the Reset fire sites: AIM_Initialize
// (Creature.cpp:1085, AddToWorld — both modes) at kit creation, the
// JustAppeared tick (Creature.cpp:699-704, compat mode only) via
// fireCreatureSpawned, and the HomeMovementGenerator finalize via
// resetVehicleKitOnReachHome (Vehicle::Reset(true)). No compat gate on
// the re-apply: C++ gates the JustAppeared Reset on compat mode only
// because non-compat respawns rebuild the object (AIM_Initialize
// re-applies); Go reuses the motion, so the re-apply runs for both
// modes — the rows dedupe and the root is re-armed after the
// JustRespawned clear. The InstallAllAccessories half of Reset is not
// repeated here: at first spawn no passengers exist yet and the install
// rides the step sync (syncCreatureVehiclePassengers), and on evade it
// rides resetVehicleKitOnReachHome; on respawn the kit and its seated
// accessories persist in Go (no removeVehicleKit on death), so there is
// nothing to reinstall — the remove+reinstall fate of a despawned
// accessory is an open follow-up, not a blind port. The OnReset script
// hook has no Eluna event (VehicleEvents 3 is UNUSED) and no C++
// VehicleScript implementor, so it is vacuous.
func (s *Server) applyVehicleImmunities(motion *creatureMotion) {
	if s == nil || motion == nil {
		return
	}
	s.motionMu.Lock()
	guid, mapID, instanceID := motion.GUID, motion.Map, motion.InstanceID
	entry, creatureType, typeFlags := motion.Entry, motion.CreatureType, motion.TypeFlags
	s.motionMu.Unlock()
	kit := s.getVehicleKit(mapID, instanceID, guid)
	if kit == nil {
		return
	}
	key := creatureAuraKey{Map: mapID, InstanceID: instanceID, GUID: guid}
	add := func(op, immType uint32) {
		s.addCreatureSpellImmuneRow(key, op, immType, 0)
	}
	add(immunityOpEffect, spellEffectKnockBack)
	add(immunityOpEffect, spellEffectKnockBackDest)
	if creatureType == creatureTypeMechanical && typeFlags&creatureTypeFlagBossMob == 0 {
		add(immunityOpEffect, spellEffectHeal)
		add(immunityOpEffect, spellEffectHealPct)
		add(immunityOpEffect, spellEffectDispel)
		add(immunityOpState, spellAuraPeriodicHeal)
		add(immunityOpState, spellAuraSchoolImmunity)
		add(immunityOpState, spellAuraModUnattackable)
		add(immunityOpState, SpellAuraSchoolAbsorb)
		add(immunityOpMechanic, mechanicBanish)
		add(immunityOpMechanic, mechanicShield)
		add(immunityOpMechanic, mechanicImmuneShield)
		add(immunityOpState, spellAuraDamageShield)
		add(immunityOpState, spellAuraSplitDamagePct)
		add(immunityOpState, spellAuraModResistance)
		add(immunityOpState, spellAuraModStat)
		add(immunityOpState, spellAuraModDamagePercentTaken)
	}
	rooted := entry == 30236 || entry == 39759
	if kit.VehicleInfo != nil {
		if kit.VehicleInfo.Flags&wotlk.VehicleFlagFixedPosition != 0 {
			rooted = true
		}
		switch kit.VehicleInfo.ID {
		case 160, 244, 510, 452, 543:
			rooted = true
			add(immunityOpState, spellAuraModDecreaseSpeed)
		case 335, 336, 338:
			s.removeCreatureSpellImmuneRows(key, immunityOpState, spellAuraModDamagePercentTaken, 0)
		}
	}
	if rooted {
		s.motionMu.Lock()
		motion.ScriptRooted = true
		s.motionMu.Unlock()
	}
}

// resetVehicleKitOnReachHome mirrors the Vehicle::Reset(true) leg of
// HomeMovementGenerator<Creature>::DoFinalize (HomeMovementGenerator.cpp:139-151):
// SetSpawnHealth and LoadCreaturesAddon run ahead of it (the evade arms
// restore health at evade start; the addon auras re-apply via
// reapplyCreatureAddonAuras), then
// Reset(true) re-applies ApplyAllImmunities and, while the base is alive,
// InstallAllAccessories(true) — MINION accessories only, with NO
// RemoveAllPassengers first (Vehicle.cpp:84-100: the removal runs only
// for player bases or !evading). The OnReset script hook is vacuous
// (VehicleEvents OnReset has no C++ implementors; Hooks.h 3 UNUSED), so
// nothing fires for it. Called at every Go DoFinalize twin: the three
// evade-home arms, the script MoveHome spline landing (MoveTargetedHome
// launches the same generator), and the critter flee-expiry failed
// launch — all finalize through the same C++ body. A base with no kit
// is a no-op: getVehicleKit returns nil and the call returns.
func (s *Server) resetVehicleKitOnReachHome(ctx context.Context, motion *creatureMotion) {
	if s == nil || motion == nil {
		return
	}
	kit := s.getVehicleKit(motion.Map, motion.InstanceID, motion.GUID)
	if kit == nil {
		return
	}
	s.applyVehicleImmunities(motion)
	if motion.Health <= 0 {
		return
	}
	s.reinstallVehicleMinionAccessories(ctx, motion, kit)
}

// reinstallVehicleMinionAccessories bridges Vehicle::InstallAllAccessories(true)
// (Vehicle.cpp:84-100) for the evade leg: only IsMinion rows reinstall, and
// the once-per-kit AccessoriesInstalled latch does NOT gate it — the latch
// guards the spawn install (installVehicleAccessoriesFromOrigin), while C++
// re-runs InstallAllAccessories on every evade. The latch is neither
// consulted nor set here, so a later first passenger sync still installs
// the non-minion rows. Per minion row (Vehicle.cpp:386-412): TempSummon
// at the base position, UNIT_MASK_ACCESSORY, then the HandleSpellClick
// twin boards it — and Vehicle::AddPassenger (Vehicle.cpp:471-478) ejects
// the seat's current occupant first when the seat is occupied, through
// the full _ExitVehicle twin (exit position, fall, tail: a
// UNIT_MASK_ACCESSORY minion unsummons 2s later while the base is alive,
// Unit.cpp:13258-13265), and the fresh summon boards in its place. A
// player seated on a minion seat is ejected through the session
// exitVehicle leg, as in C++. A missing seat aborts the board like the
// spawn install (the VehicleJoinEvent::Abort twin despawns the fresh summon).
func (s *Server) reinstallVehicleMinionAccessories(ctx context.Context, motion *creatureMotion, kit *VehicleKit) {
	if s == nil || motion == nil || kit == nil {
		return
	}
	kit.mu.RLock()
	var minions []VehicleAccessory
	for _, acc := range kit.Accessories {
		if acc.IsMinion {
			minions = append(minions, acc)
		}
	}
	kit.mu.RUnlock()
	if len(minions) == 0 {
		return
	}
	origin := s.summonOriginFromMotion(motion)
	baseGUID := motion.GUID
	for _, acc := range minions {
		// Vehicle::InstallAccessory (Vehicle.cpp:388-395) refuses
		// to install while the vehicle is uninstalling — before
		// summoning, so no accessory exists to strand. The
		// AddPassenger refusal below is the backstop for a
		// removal landing mid-row.
		if kit.isUninstalling() {
			continue
		}
		summoned := s.summonTempCreature(ctx, origin, acc.AccessoryEntry, motion.X, motion.Y, motion.Z, motion.Orientation, acc.SummonType, acc.SummonTime)
		if summoned == nil {
			continue
		}
		// Vehicle::AddPassenger (Vehicle.cpp:471-478) kicks the occupant
		// off an occupied target seat before the new passenger boards,
		// via the full _ExitVehicle twin: the bare kit.RemovePassenger
		// this replaces bypassed the relocate/fall legs
		// (Unit.cpp:13215-13249) and the tail, leaving the ejected
		// minion at its stale seat-offset position in the air. A player
		// on a minion seat is ejected the same way (the session exit
		// leg), as in C++.
		s.ejectVehicleSeatOccupant(ctx, motion.Map, motion.InstanceID, kit, baseGUID, acc.SeatID, summoned.GUID)
		wasUnselectable := summoned.UnitFlags&0x02000000 != 0
		assigned, seatInfo, ok := kit.AddPassenger(summoned.GUID, acc.SeatID, wasUnselectable)
		if !ok {
			// VehicleJoinEvent::Abort (Vehicle.cpp:943-944)
			// despawns only UNIT_MASK_ACCESSORY passengers —
			// InstallAccessory sets the mask for minions only
			// (Vehicle.cpp:403-404), so a non-minion accessory
			// that fails to board stays summoned.
			if acc.IsMinion {
				s.expireTempSummonGuardian(ctx, summoned)
			}
			continue
		}
		s.motionMu.Lock()
		summoned.VehicleGUID = baseGUID
		summoned.VehicleSeat = assigned
		summoned.VehicleAccessoryMinion = acc.IsMinion
		summoned.Moving = false
		s.motionMu.Unlock()
		// VehicleJoinEvent::Execute (Vehicle.cpp:855-856): a seat
		// carrying VEHICLE_SEAT_FLAG_PASSENGER_NOT_SELECTABLE sets
		// UNIT_FLAG_NOT_SELECTABLE on the passenger.
		s.setCreatureVehicleNotSelectable(summoned, seatInfo)
		kit.mu.Lock()
		kit.InstalledAccessories = append(kit.InstalledAccessories, summoned.GUID)
		kit.mu.Unlock()
		// VehicleJoinEvent::Execute (Vehicle.cpp:895-896): every
		// creature the passenger threatens gains the vehicle base on
		// its threat list with the passenger's threat value, ahead
		// of the AI PassengerBoarded / OnAddPassenger hooks below.
		// VehicleJoinEvent::Execute (Vehicle.cpp:883): the board
		// breaks the passenger's target for everyone in range.
		// VehicleJoinEvent::Execute (Vehicle.cpp:836-837): the board
		// interrupts the passenger's non-melee casts and strips its
		// mount aura, ahead of the clear-target below.
		s.interruptBoardingCreaturePassenger(summoned)
		s.sendPassengerClearTarget(origin.Map, origin.InstanceID, summoned.GUID, summoned.X, summoned.Y, summoned.Z, summoned.PhaseMask)
		// VehicleJoinEvent::Execute (Vehicle.cpp:884, :887-892): the
		// board roots the passenger and runs the seat-offset spline
		// with the transport-enter flag, ahead of the threat
		// transfer below.
		_, _, seatAddon := kit.GetSeatForPassenger(summoned.GUID)
		// VehicleJoinEvent::Execute (Vehicle.cpp:865-869): the board
		// stamps the passenger's transport with the seat offsets.
		s.motionMu.Lock()
		stampCreatureVehicleTransport(summoned, baseGUID, assigned, seatInfo, seatAddon)
		s.motionMu.Unlock()
		s.rootAndSplineBoardingCreaturePassenger(summoned, baseGUID, seatInfo, seatAddon, motion.X, motion.Y, motion.Z, motion.Orientation, true)
		s.transferPassengerThreatToVehicleBase(origin.Map, origin.InstanceID, summoned.GUID, baseGUID)
		if base := s.findCreatureMotion(origin.Map, origin.InstanceID, baseGUID); base != nil {
			// VehicleJoinEvent::Execute (Vehicle.cpp:900-902): the
			// AI PassengerBoarded(true) call precedes the
			// OnAddPassenger script hook.
			s.smartPassengerBoarded(ctx, base, summoned.GUID, assigned, true)
			s.triggerVehicleEvent(scripting.VehicleEventOnAddPassenger, s.luaMotionCreature(base), s.luaMotionCreature(summoned), assigned)
			if acc.IsMinion {
				s.triggerVehicleEvent(scripting.VehicleEventOnInstallAccessory, s.luaMotionCreature(base), s.luaMotionCreature(summoned))
			}
		}
	}
	// VehicleJoinEvent::Execute (Vehicle.cpp:825-832): installing the
	// accessories may take the last usable seat — the base's
	// spell-click flag follows the final count, not the template.
	// A player base syncs its UNIT_NPC_FLAG_PLAYER_VEHICLE half through
	// its session in the helper.
	s.syncVehicleBaseNPCFlag(origin.Map, origin.InstanceID, baseGUID)
}

func (s *Server) removeVehicleKit(mapID, instanceID uint32, vehicleGUID uint64) {
	s.removeVehicleKitWithBase(mapID, instanceID, vehicleGUID, nil)
}

// removeVehicleKitWithBase is removeVehicleKit for a base whose motion
// is already deleted (despawnCreatureMotion, pruneCreatureMotion,
// despawnVehicleMotion). C++ still fires OnRemovePassenger there:
// Unit::RemoveFromWorld drops the kit while the base is still in the
// world (Unit.cpp:9789, before WorldObject::RemoveFromWorld), so the
// departing motion stands in for the base object the hook fires on.
func (s *Server) removeVehicleKitWithBase(mapID, instanceID uint32, vehicleGUID uint64, base *creatureMotion) {
	if s == nil {
		return
	}
	s.vehicleMu.Lock()
	key := vehicleKitKey{MapID: mapID, InstanceID: instanceID, GUID: vehicleGUID}
	kit := s.instanceVehicleKits[key]
	if kit != nil && s.vehicleKits[vehicleGUID] == kit {
		delete(s.vehicleKits, vehicleGUID)
	}
	delete(s.instanceVehicleKits, key)
	if kit != nil {
		// Vehicle::Uninstall (Vehicle.cpp:118) sets
		// STATUS_UNINSTALLING before RemoveAllPassengers runs: the
		// flag is set under vehicleMu beside the map delete, so a
		// board racing the removal on another goroutine sees it
		// (AddPassenger refuses) and the exit sweep below
		// snapshots a seat set no later board can join. Lock order
		// is vehicleMu before the kit's mu: no VehicleKit method
		// takes vehicleMu, and no caller holds the kit's mu across
		// a server call, so the order never inverts.
		kit.mu.Lock()
		kit.uninstalling = true
		kit.mu.Unlock()
	}
	s.vehicleMu.Unlock()
	if kit != nil {
		s.exitCreatureVehiclePassengers(mapID, instanceID, kit, false, base)
	}
	// Unit::RemoveVehicleKit (Unit.cpp:12070-12071) clears
	// UNIT_NPC_FLAG_SPELLCLICK | UNIT_NPC_FLAG_PLAYER_VEHICLE: the player
	// half clears through the session here (the creature half rides the
	// motion, which the despawn callers already dropped; a live creature
	// base keeps its flag until its own sync runs).
	if sess := s.findSessionByGUID(vehicleGUID); sess != nil {
		sess.setPlayerVehicleNPCFlag(nil)
	}
}

// vehicleKitLocation pairs a kit with the instance key it lives under.
// A kit is normally keyed on its base's current map/instance, but a
// player base that far-teleports leaves its kit keyed on the origin
// map/instance: teleportTo exits the teleporter as passenger only, it
// never re-keys the teleporter's own kit. The GUID scan below is the
// twin of C++'s m_vehicleKit, which lives on the unit, not on a map
// bucket, so a same-map lookup at logout misses the origin-keyed kit.
type vehicleKitLocation struct {
	key vehicleKitKey
	kit *VehicleKit
}

// findVehicleKits returns every kit registered under vehicleGUID, in
// (map, instance) key order. Normally exactly one; a teleported base
// plus a relogin recreation can leave a stale origin-keyed kit beside
// the live one, and logout must drop both, not just the current-key
// one.
func (s *Server) findVehicleKits(vehicleGUID uint64) []vehicleKitLocation {
	if s == nil || vehicleGUID == 0 {
		return nil
	}
	s.vehicleMu.RLock()
	defer s.vehicleMu.RUnlock()
	var found []vehicleKitLocation
	for key, kit := range s.instanceVehicleKits {
		if key.GUID == vehicleGUID && kit != nil {
			found = append(found, vehicleKitLocation{key: key, kit: kit})
		}
	}
	sort.Slice(found, func(i, j int) bool {
		if found[i].key.MapID != found[j].key.MapID {
			return found[i].key.MapID < found[j].key.MapID
		}
		return found[i].key.InstanceID < found[j].key.InstanceID
	})
	return found
}

// conditionSourceCreatureTemplateVehicle is
// CONDITION_SOURCE_TYPE_CREATURE_TEMPLATE_VEHICLE (ConditionMgr.h:139). It
// cannot carry a SourceGroup (ConditionMgr.cpp:950), so its rows are keyed
// on SourceEntry = creature entry alone, and it offers two targets
// (Condition.cpp:794): 0 = the passenger player, 1 = the vehicle.
const conditionSourceCreatureTemplateVehicle = 16

// vehicleHasConditions bridges VehicleAI::LoadConditions
// (CombatAI.cpp:308-311): HasConditionsForNotGroupedEntry for the
// vehicle's entry. A missing table or query error reads as no conditions.
func (s *Server) vehicleHasConditions(ctx context.Context, entry uint32) bool {
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil || entry == 0 {
		return false
	}
	var one int
	if err := s.WorldStore.DB.QueryRowContext(ctx, "SELECT 1 FROM conditions WHERE SourceTypeOrReferenceId = ? AND SourceEntry = ? LIMIT 1", conditionSourceCreatureTemplateVehicle, entry).Scan(&one); err != nil {
		return false
	}
	return true
}

// loadVehicleConditionRows loads the entry's vehicle-condition rows for
// meetVehicleConditions. A missing table or query error yields no rows,
// which evaluate as met (the check retries next second).
func (s *Server) loadVehicleConditionRows(ctx context.Context, entry uint32) []conditionRow {
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil {
		return nil
	}
	rows, err := s.WorldStore.DB.QueryContext(ctx,
		"SELECT ElseGroup, ConditionTypeOrReference, ConditionTarget, ConditionValue1, ConditionValue2, ConditionValue3, NegativeCondition FROM conditions WHERE SourceTypeOrReferenceId = ? AND SourceEntry = ? ORDER BY ElseGroup",
		conditionSourceCreatureTemplateVehicle, entry)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var conditions []conditionRow
	for rows.Next() {
		var row conditionRow
		if err := rows.Scan(&row.ElseGroup, &row.ConditionType, &row.ConditionTarget, &row.Value1, &row.Value2, &row.Value3, &row.Negative); err != nil {
			return nil
		}
		conditions = append(conditions, row)
	}
	if err := rows.Err(); err != nil {
		return nil
	}
	return conditions
}

// meetVehicleConditions bridges VehicleAI::CheckConditions'
// IsObjectMeetingNotGroupedConditions (CombatAI.cpp:326,
// ConditionMgr.cpp:993): the passenger session evaluates the entry's rows
// with target 0 = the passenger and target 1 = the vehicle creature, the
// same target mapping as meetSpellClickConditions.
func (s *session) meetVehicleConditions(ctx context.Context, entry uint32, vehicleGUID uint64) (bool, error) {
	if s == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return true, nil
	}
	rows := s.server.loadVehicleConditionRows(ctx, entry)
	return s.evalConditionGroups(ctx, rows, func(ctx context.Context, row conditionRow) (bool, error) {
		return s.evalCondition(ctx, row, entry, vehicleGUID)
	})
}

// vehiclePassengerSeat is one occupied vehicle seat with its passenger.
type vehiclePassengerSeat struct {
	seat int8
	guid uint64
}

// seatedPassengerSeats returns the occupied seats with their passenger
// GUIDs in seat-id order, the seat-aware twin of seatedPassengerGUIDs
// (SMART_TARGET_VEHICLE_PASSENGER needs the seat id for its seatMask).
func (v *VehicleKit) seatedPassengerSeats() []vehiclePassengerSeat {
	if v == nil {
		return nil
	}
	v.mu.RLock()
	defer v.mu.RUnlock()
	ids := make([]int8, 0, len(v.Seats))
	for id := range v.Seats {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var out []vehiclePassengerSeat
	for _, id := range ids {
		if seat := v.Seats[id]; seat != nil && seat.Passenger.GUID != 0 {
			out = append(out, vehiclePassengerSeat{seat: id, guid: seat.Passenger.GUID})
		}
	}
	return out
}

// seatedPassengerGUIDs returns the GUIDs of seated passengers in seat-id
// order, the deterministic twin of VehicleAI::CheckConditions' seat scan
// (CombatAI.cpp:320-338 iterates Vehicle::Seats; the C++ order is
// unspecified, Go fixes seat order so the first failing passenger is
// stable). It reads the passenger GUIDs directly under the kit read lock
// rather than via IsEmpty, which would re-acquire it.
func (v *VehicleKit) seatedPassengerGUIDs() []uint64 {
	if v == nil {
		return nil
	}
	v.mu.RLock()
	defer v.mu.RUnlock()
	ids := make([]int8, 0, len(v.Seats))
	for id := range v.Seats {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var guids []uint64
	for _, id := range ids {
		if seat := v.Seats[id]; seat != nil && seat.Passenger.GUID != 0 {
			guids = append(guids, seat.Passenger.GUID)
		}
	}
	return guids
}

// despawnVehicleMotion bridges the VehicleAI::UpdateAI dismiss expiry
// (CombatAI.cpp:285-292: DespawnOrUnsummon): the motion leaves the world
// immediately with a destroy broadcast and the remove hook, and its kit is
// dropped so a respawn builds a fresh one. A DB-spawned vehicle respawns
// on its normal delay; a summoned one is gone, mirroring the Lua
// Creature::DespawnOrUnsummon bridge.
func (s *Server) despawnVehicleMotion(ctx context.Context, motion *creatureMotion) {
	if s == nil || motion == nil {
		return
	}
	s.motionMu.Lock()
	motions := s.motionMapLocked(motion.Map, motion.InstanceID)
	m, ok := motions[motion.GUID]
	if !ok {
		s.motionMu.Unlock()
		return
	}
	delete(motions, motion.GUID)
	mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
	maxHealth := m.MaxHealth
	isTempSummon, ownerGUID := m.IsTempSummon, m.OwnerGUID
	x, y, z := m.X, m.Y, m.Z
	s.motionMu.Unlock()
	// CleanupBeforeRemoveFromMap (Unit.cpp:9826-9829) cancels a
	// cast in flight before RemoveFromWorld, ahead of destroy.
	s.interruptCreatureCastsOnRemove(m)
	// Unit::RemoveVehicleKit (Unit.cpp:12060-12062) no-ops without a kit,
	// so OnUninstall fires only when one existed.
	hadKit := s.getVehicleKit(mapID, instanceID, guid) != nil
	s.removeVehicleKitWithBase(mapID, instanceID, guid, m)
	// Vehicle::Uninstall (Vehicle.cpp:123-124) fires OnUninstall for
	// creature bases after RemoveAllPassengers — the ev6 fires above
	// ride removeVehicleKit's exitCreatureVehiclePassengers, so this
	// lands after them, in C++ order. The motion struct is still valid
	// for the object snapshot after its map deletion above.
	if hadKit {
		s.triggerVehicleEvent(scripting.VehicleEventOnUninstall, s.luaMotionCreature(m))
	}
	destroyBuf := protocol.NewBuffer(9)
	destroyBuf.WriteU64(guid)
	destroyBuf.WriteU8(0)
	s.broadcastToVisibleSetFromPhase(uint16(protocol.OpcodeSMSG_DESTROY_OBJECT), destroyBuf.Bytes(), mapID, instanceID, x, y, z, m.PhaseMask)
	// DespawnOrUnsummon on a TempSummon runs UnSummon
	// (TemporarySummon.cpp:274-284) FIRST, firing
	// SummonedCreatureDespawn on the summoner creature's AI;
	// the Eluna twin is event 20 on the summoner (the
	// expireTempSummonGuardian precedent). The dismiss leg has no
	// spawn-source gate in C++ — VehicleAI::UpdateAI calls
	// DespawnOrUnsummon on any vehicle creature, so a summoned
	// vehicle (AIName VehicleAI or kit-held, isVehicleAI has no
	// DB-only gate) reaches this leg; pets report IsTempSummon
	// false and player owners resolve to no creature motion.
	// MotionMu is released above, as fireCreatureSummoned requires.
	if isTempSummon {
		if ownerMotion := s.findCreatureMotion(mapID, instanceID, ownerGUID); ownerMotion != nil {
			s.fireCreatureSummoned(ctx, ownerMotion, scripting.CreatureEventOnSummonedCreatureDespawn, m)
			s.smartSummonDespawned(ctx, ownerMotion, m)
		}
	}
	// Eluna WORLD_EVENT_ON_DELETE_CREATURE (31): this function is the
	// VehicleAI dismiss-expiry DespawnOrUnsummon bridge (see its header
	// comment), so the vehicle creature's delete queues here too. The
	// helper fires 31 then the RemoveFromWorld leg 37, in C++ order
	// (previously this leg fired 37 before 31).
	s.fireServerDeleteCreature(ctx, m)
	if maxHealth == 0 {
		maxHealth = 1
	}
	dbGUID := uint32(guid & 0x00FFFFFF)
	dbSpawned := false
	if s.WorldStore != nil && s.WorldStore.DB != nil {
		var one int
		dbSpawned = s.WorldStore.DB.QueryRowContext(ctx, "SELECT 1 FROM creature WHERE guid = ?", dbGUID).Scan(&one) == nil
	}
	if !dbSpawned {
		return
	}
	if instanceID == 0 {
		_, _ = s.WorldStore.DB.ExecContext(ctx, "UPDATE creature SET curhealth = 0 WHERE guid = ?", dbGUID)
		s.scheduleCreatureRespawn(ctx, dbGUID, maxHealth, time.Now())
	} else {
		s.scheduleInstanceCreatureRespawn(ctx, combatTarget{GUID: guid, Map: mapID, InstanceID: instanceID, X: x, Y: y, Z: z, MaxHealth: maxHealth}, maxHealth, time.Now())
	}
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

// installVehicleAccessories bridges Vehicle::InstallAllAccessories
// (Vehicle.cpp:84-100) for the non-evading case: Creature::AIM_Initialize
// (Creature.cpp:1085) and LoadResetScript call Vehicle::Reset, which
// installs every template accessory at spawn. Go kits copy the
// vehicle_template_accessory rows as data only, so this runs on the
// first passenger sync for the base: the step paths all funnel through
// sync, and summoning needs motionMu, which the kit-creation sites
// hold while the sync callers do not. Per accessory (Vehicle.cpp:386-412):
// TempSummon at the base position via the summonTempCreature twin,
// UNIT_MASK_ACCESSORY for minions, then the HandleSpellClick twin:
// kit.AddPassenger seats the motion (the player-only enterVehicle
// cannot board creatures), and this sync's own relocate fan-out below
// moves it onto the seat offsets. An occupied target seat ejects its
// occupant first (Vehicle.cpp:471-478, the ejectVehicleSeatOccupant
// twin): C++ clears passengers before the spawn install
// (Vehicle.cpp:86-87), but Go installs a tick after spawn, when a
// player may already have boarded the accessory's seat, so the
// AddPassenger eject leg is live here. A failed board
// despawns the just-summoned accessory (the VehicleJoinEvent::Abort
// twin). GUID-based vehicle_accessory rows are not loaded in Go, so
// the install covers the template list only.
func (s *Server) installVehicleAccessories(ctx context.Context, motion *creatureMotion, kit *VehicleKit) {
	if s == nil || motion == nil || kit == nil {
		return
	}
	s.installVehicleAccessoriesFromOrigin(ctx, s.summonOriginFromMotion(motion), motion.GUID, motion.X, motion.Y, motion.Z, motion.Orientation, kit)
}

// installVehicleAccessoriesFromOrigin is the shared InstallAllAccessories
// (Vehicle.cpp:84-100) body for a base of either kind: the latch, the
// per-accessory TempSummon + AddPassenger + seat bookkeeping, and the
// failed-board despawn are identical; only the origin differs. The
// creature caller passes a motion snapshot, the player caller a session
// snapshot, because a player base has no creatureMotion.
func (s *Server) installVehicleAccessoriesFromOrigin(ctx context.Context, origin tempSummonOrigin, baseGUID uint64, x, y, z, o float32, kit *VehicleKit) {
	if s == nil || kit == nil {
		return
	}
	kit.mu.Lock()
	if kit.AccessoriesInstalled || len(kit.Accessories) == 0 {
		kit.mu.Unlock()
		return
	}
	accessories := append([]VehicleAccessory(nil), kit.Accessories...)
	kit.AccessoriesInstalled = true
	kit.mu.Unlock()
	for _, acc := range accessories {
		// Vehicle::InstallAccessory (Vehicle.cpp:388-395) refuses
		// to install while the vehicle is uninstalling — before
		// summoning, so no accessory exists to strand. The
		// AddPassenger refusal below is the backstop for a
		// removal landing mid-row.
		if kit.isUninstalling() {
			continue
		}
		summoned := s.summonTempCreature(ctx, origin, acc.AccessoryEntry, x, y, z, o, acc.SummonType, acc.SummonTime)
		if summoned == nil {
			continue
		}
		// Vehicle::AddPassenger (Vehicle.cpp:471-478) kicks the occupant
		// off an occupied target seat before the new passenger boards,
		// via the full _ExitVehicle twin: the spawn install runs a tick
		// after spawn, when a player may already have boarded the
		// accessory's seat, and without the eject the kit board fails
		// and the fresh summon despawns below. A player on the seat
		// exits through the session exitVehicle leg, as in C++. The
		// player-base mount install shares this body with a fresh kit,
		// so the eject no-ops there.
		s.ejectVehicleSeatOccupant(ctx, origin.Map, origin.InstanceID, kit, baseGUID, acc.SeatID, summoned.GUID)
		wasUnselectable := summoned.UnitFlags&0x02000000 != 0
		assigned, seatInfo, ok := kit.AddPassenger(summoned.GUID, acc.SeatID, wasUnselectable)
		if !ok {
			// VehicleJoinEvent::Abort (Vehicle.cpp:943-944)
			// despawns only UNIT_MASK_ACCESSORY passengers —
			// InstallAccessory sets the mask for minions only
			// (Vehicle.cpp:403-404), so a non-minion accessory
			// that fails to board stays summoned.
			if acc.IsMinion {
				s.expireTempSummonGuardian(ctx, summoned)
			}
			continue
		}
		s.motionMu.Lock()
		summoned.VehicleGUID = baseGUID
		summoned.VehicleSeat = assigned
		summoned.VehicleAccessoryMinion = acc.IsMinion
		summoned.Moving = false
		s.motionMu.Unlock()
		// VehicleJoinEvent::Execute (Vehicle.cpp:855-856): a seat
		// carrying VEHICLE_SEAT_FLAG_PASSENGER_NOT_SELECTABLE sets
		// UNIT_FLAG_NOT_SELECTABLE on the passenger.
		s.setCreatureVehicleNotSelectable(summoned, seatInfo)
		kit.mu.Lock()
		kit.InstalledAccessories = append(kit.InstalledAccessories, summoned.GUID)
		kit.mu.Unlock()
		// Vehicle::AddPassenger (Vehicle.cpp:899-906) fires OnAddPassenger
		// for creature bases, then OnInstallAccessory when the passenger
		// carries UNIT_MASK_ACCESSORY — which InstallAccessory sets only
		// for minions (Vehicle.cpp:404). The player-base install path
		// shares this body, so the base-motion gate keeps it silent
		// there, matching the C++ TYPEID_UNIT gate.
		// VehicleJoinEvent::Execute (Vehicle.cpp:895-896): every
		// creature the passenger threatens gains the vehicle base on
		// its threat list with the passenger's threat value, ahead
		// of the AI PassengerBoarded / OnAddPassenger hooks below.
		// VehicleJoinEvent::Execute (Vehicle.cpp:883): the board
		// breaks the passenger's target for everyone in range.
		// VehicleJoinEvent::Execute (Vehicle.cpp:836-837): the board
		// interrupts the passenger's non-melee casts and strips its
		// mount aura, ahead of the clear-target below.
		s.interruptBoardingCreaturePassenger(summoned)
		s.sendPassengerClearTarget(origin.Map, origin.InstanceID, summoned.GUID, summoned.X, summoned.Y, summoned.Z, summoned.PhaseMask)
		// VehicleJoinEvent::Execute (Vehicle.cpp:884, :887-892): the
		// board roots the passenger and runs the seat-offset spline
		// with the transport-enter flag, ahead of the threat
		// transfer below.
		_, _, seatAddon := kit.GetSeatForPassenger(summoned.GUID)
		// VehicleJoinEvent::Execute (Vehicle.cpp:865-869): the board
		// stamps the passenger's transport with the seat offsets.
		s.motionMu.Lock()
		stampCreatureVehicleTransport(summoned, baseGUID, assigned, seatInfo, seatAddon)
		s.motionMu.Unlock()
		s.rootAndSplineBoardingCreaturePassenger(summoned, baseGUID, seatInfo, seatAddon, x, y, z, o, true)
		s.transferPassengerThreatToVehicleBase(origin.Map, origin.InstanceID, summoned.GUID, baseGUID)
		if base := s.findCreatureMotion(origin.Map, origin.InstanceID, baseGUID); base != nil {
			// VehicleJoinEvent::Execute (Vehicle.cpp:900-902): the
			// AI PassengerBoarded(true) call precedes the
			// OnAddPassenger script hook.
			s.smartPassengerBoarded(ctx, base, summoned.GUID, assigned, true)
			s.triggerVehicleEvent(scripting.VehicleEventOnAddPassenger, s.luaMotionCreature(base), s.luaMotionCreature(summoned), assigned)
			if acc.IsMinion {
				s.triggerVehicleEvent(scripting.VehicleEventOnInstallAccessory, s.luaMotionCreature(base), s.luaMotionCreature(summoned))
			}
		}
	}
	// VehicleJoinEvent::Execute (Vehicle.cpp:825-832): installing the
	// accessories may take the last usable seat — the base's
	// spell-click flag follows the final count, not the template.
	// A player base syncs its UNIT_NPC_FLAG_PLAYER_VEHICLE half through
	// its session in the helper.
	s.syncVehicleBaseNPCFlag(origin.Map, origin.InstanceID, baseGUID)
}

// installPlayerVehicleAccessories bridges Vehicle::InstallAllAccessories
// (Vehicle.cpp:84-100) for a player mount base, the Unit::Mount tail
// (Unit.cpp:8334): the kit was just created with the mount creature
// entry, so its template accessories are the mount entry's rows. The
// install runs at Mount time, not on a later sync — the kit is fresh,
// so C++'s RemoveAllPassengers first is vacuous here, unlike the
// creature path's tick-after-spawn skip. The relocate fan-out below
// seats the accessories on their offsets at once (the creature sync's
// own fan-out twin); later player moves relocate them through
// movement.go's base-GUID arm. Seated-AI suppression and kit-drop fate
// ride the shared gates: the stepCreatureMotion VehicleGUID gate and
// removeVehicleKit's exitCreatureVehiclePassengers.
// unsummonEjectedVehicleAccessory bridges the Unit::_ExitVehicle
// (Unit.cpp:13258-13265) fate for an ejected minion accessory: the
// control-aura removal behind RemoveAllPassengers (respawn Reset) and the
// AddPassenger seat-eject (evade reinstall) both run _ExitVehicle, and a
// UNIT_MASK_ACCESSORY minion exits with UnSummon(2000) while the base is
// alive — the "Approximation" delay in C++. Non-minion passengers stay in
// the world. The timer is identity-safe: expireTempSummonGuardian no-ops
// when the motion already left the map.
func (s *Server) unsummonEjectedVehicleAccessory(motion *creatureMotion, wasAccessoryMinion bool) {
	if s == nil || motion == nil || !wasAccessoryMinion {
		return
	}
	time.AfterFunc(2*time.Second, func() {
		s.expireTempSummonGuardian(context.Background(), motion)
	})
}

// resetVehicleKitOnRespawn bridges the InstallAllAccessories(false) half of
// the respawn Vehicle::Reset (Creature::Respawn re-inits the AI;
// AIM_Initialize (Creature.cpp:1085) / JustAppeared tick (Creature.cpp:699-704)
// run Reset, which first removes all passengers then installs every template
// accessory). InstallAllAccessories(false) (Vehicle.cpp:84-100) ejects EVERY
// passenger first — RemoveAllPassengers runs for !evading, removing the
// control-vehicle aura — then summons a fresh TempSummon per row with full
// SummonTime and boards it. It replaces the missing-only refill (781fae1d),
// which diverged on still-seated accessories: C++ gives them a fresh GUID +
// full SummonTime and ejects a live player still seated on the respawned base
// (Go never ejected them at death), while the refill kept the stale accessory
// + remaining timer and left the player riding. The eject leg follows the
// evade reinstall's RemovePassenger pattern for creatures, with the
// Unit::_ExitVehicle (Unit.cpp:13215-13249) exit-position + fall legs —
// the ejected creature relocates to the base position (+ seat addon)
// and falls on a SetFall spline when airborne — and the
// (Unit.cpp:13258-13265) fate for minion accessories:
// the aura removal behind RemoveAllPassengers runs _ExitVehicle, and a
// UNIT_MASK_ACCESSORY minion unsummons (UnSummon(2000)) while the base is
// alive — only non-minion passengers stay in the world. The player leg
// rides the session exitVehicle (control release, duel forfeit,
// exit position, and the VehicleAI::OnCharmed dismiss arm via RemoveCharmedBy
// — Vehicle.cpp:518 + CombatAI.cpp:295). The latch is set, not consulted:
// C++ re-runs InstallAllAccessories on every respawn, so the step sync must
// not re-install afterwards. Called only from the respawn scheduler (kill.go
// world + instance paths), never from fireCreatureSpawned — first spawn
// installs through the step sync.
func (s *Server) resetVehicleKitOnRespawn(ctx context.Context, motion *creatureMotion) {
	if s == nil || motion == nil {
		return
	}
	kit := s.getVehicleKit(motion.Map, motion.InstanceID, motion.GUID)
	if kit == nil || motion.Health <= 0 {
		return
	}
	kit.mu.Lock()
	// No early return on an empty accessory list: InstallAllAccessories
	// (Vehicle.cpp:86-87) removes all passengers on every respawn
	// regardless of whether any rows install after — the eject below is
	// the RemoveAllPassengers half, and the reinstall loop no-ops on an
	// empty list.
	accessories := append([]VehicleAccessory(nil), kit.Accessories...)
	kit.AccessoriesInstalled = true
	kit.mu.Unlock()
	// Vehicle::InstallAllAccessories(false) (Vehicle.cpp:86-87):
	// RemoveAllPassengers first — every seated passenger leaves before
	// any row reinstalls.
	for _, guid := range kit.seatedPassengerGUIDs() {
		if sess := s.findSessionByGUID(guid); sess != nil {
			sess.exitVehicle(ctx)
			// A stale seat (the passenger's VehicleGUID already 0)
			// makes exitVehicle a no-op; C++ RemoveAllPassengers
			// clears the seat regardless, so drop the bookkeeping
			// entry — and fire OnRemovePassenger here, since the
			// exit leg fires it only when it removes the seat itself.
			if seatID, seatInfo, wasUnselectable, ok := kit.RemovePassenger(guid); ok {
				// Vehicle::RemovePassenger (Vehicle.cpp:512-514,
				// :517-518, :533): the stale-seat exit never ran the
				// session arm, so the flag restore, charm drop, and
				// parachute run here, ahead of the hooks.
				sess.applyVehicleSeatNotSelectable(seatInfo, nil, wasUnselectable)
				if seatInfo != nil && seatInfo.CanControl() && !kit.IsPlayer {
					sess.sendClientControl(kit.VehicleGUID, false)
					sess.sendVehiclePetSpells(ctx, 0, 0, nil)
				}
				sess.castVehicleExitParachute(ctx, kit.VehicleGUID)
				if base := s.findCreatureMotion(motion.Map, motion.InstanceID, motion.GUID); base != nil {
					// Vehicle::RemovePassenger (Vehicle.cpp:536-539):
					// the AI PassengerBoarded(false) call precedes
					// the OnRemovePassenger script hook.
					s.smartPassengerBoarded(ctx, base, guid, seatID, false)
					s.triggerVehicleEvent(scripting.VehicleEventOnRemovePassenger, s.luaMotionCreature(base), sess.luaPlayer())
				}
			}
			continue
		}
		occupantMotion := s.findCreatureMotion(motion.Map, motion.InstanceID, guid)
		// Unit::_ExitVehicle (Unit.cpp:13215-13234): the exit position is
		// the vehicle base position with the passenger's orientation,
		// then the ModifyVehiclePassengerExitPos override, then the seat
		// addon's Offset/Dest transform — captured BEFORE the seat frees
		// below (Unit.cpp:13193): after RemovePassenger the addon lookup
		// finds nothing. The old leg left the ejected passenger at its
		// stale seat-offset position; C++ relocates it to the exit
		// position on every eject, respawn included.
		var exitX, exitY, exitZ, exitO float32
		if occupantMotion != nil {
			exitX, exitY, exitZ, exitO = s.creatureVehicleExitPosition(motion.GUID, occupantMotion, motion)
		}
		seatID, seatInfo, wasUnselectable, seatOK := kit.RemovePassenger(guid)
		// Vehicle::RemovePassenger (Vehicle.cpp:512-514, :533): the
		// NOT_SELECTABLE flag restores and a flying passenger is
		// handed the parachute spell, ahead of the hooks below.
		s.clearCreatureVehicleNotSelectable(occupantMotion, seatInfo, wasUnselectable)
		s.castCreatureVehicleExitParachute(ctx, motion.GUID, occupantMotion)
		if seatOK {
			s.smartPassengerBoarded(ctx, motion, guid, seatID, false)
		}
		if occupantMotion == nil {
			continue
		}
		wasAccessoryMinion := occupantMotion.VehicleAccessoryMinion
		if seatOK {
			s.triggerVehicleEvent(scripting.VehicleEventOnRemovePassenger, s.luaMotionCreature(motion), s.luaMotionCreature(occupantMotion))
		}
		// The exit position lands here, before the fall below, so the
		// fall spline launches from it (Unit.cpp:13238-13249), as in the
		// death/kit-drop twin (exitCreatureVehiclePassengers).
		s.motionMu.Lock()
		occupantMotion.X, occupantMotion.Y, occupantMotion.Z, occupantMotion.Orientation = exitX, exitY, exitZ, exitO
		phase := occupantMotion.PhaseMask
		occupantMotion.VehicleGUID = 0
		occupantMotion.VehicleSeat = 0
		occupantMotion.VehicleAccessoryMinion = false
		occupantMotion.Moving = false
		s.motionMu.Unlock()
		// Vehicle::RemovePassenger (Vehicle.cpp:518-529): the
		// transport leg runs here, after the seat frees and
		// before the fall below.
		s.inheritOrResetCreatureVehicleTransport(motion.Map, motion.InstanceID, occupantMotion, motion.GUID)
		// Unit::_ExitVehicle (Unit.cpp:13238-13249): a ground creature
		// ejected mid-air falls to the ground on a SetFall spline
		// instead of hanging at the exit position in the air. The fall
		// launches before the 2s unsummon below (:13265), as in C++.
		// The launch lifts the dead refusal: C++ _ExitVehicle falls a
		// corpse (MotionMaster.cpp:919-955 has no dead gate), and the
		// shared helper runs the JUST_DIED leg on a dead passenger.
		// Without a fall the move to the exit position rides the
		// teleport broadcast, as in the Lua ExitVehicle arm.
		if !s.fallAfterVehicleExitAllowDead(motion.Map, motion.InstanceID, motion.GUID, occupantMotion, true) {
			_, nearby := buildTeleportMovementPackets(guid, movementInfo{Time: s.gameTimeMilliseconds(), X: exitX, Y: exitY, Z: exitZ, Orientation: exitO})
			s.broadcastCreatureTeleportMovement(motion.Map, motion.InstanceID, guid, exitX, exitY, exitZ, nearby, phase)
		}
		s.unsummonEjectedVehicleAccessory(occupantMotion, wasAccessoryMinion)
	}
	origin := s.summonOriginFromMotion(motion)
	baseGUID := motion.GUID
	for _, acc := range accessories {
		// Vehicle::InstallAccessory (Vehicle.cpp:388-395) refuses
		// to install while the vehicle is uninstalling — before
		// summoning, so no accessory exists to strand. The
		// AddPassenger refusal below is the backstop for a
		// removal landing mid-row.
		if kit.isUninstalling() {
			continue
		}
		summoned := s.summonTempCreature(ctx, origin, acc.AccessoryEntry, motion.X, motion.Y, motion.Z, motion.Orientation, acc.SummonType, acc.SummonTime)
		if summoned == nil {
			continue
		}
		wasUnselectable := summoned.UnitFlags&0x02000000 != 0
		assigned, seatInfo, ok := kit.AddPassenger(summoned.GUID, acc.SeatID, wasUnselectable)
		if !ok {
			// VehicleJoinEvent::Abort (Vehicle.cpp:943-944)
			// despawns only UNIT_MASK_ACCESSORY passengers —
			// InstallAccessory sets the mask for minions only
			// (Vehicle.cpp:403-404), so a non-minion accessory
			// that fails to board stays summoned.
			if acc.IsMinion {
				s.expireTempSummonGuardian(ctx, summoned)
			}
			continue
		}
		s.motionMu.Lock()
		summoned.VehicleGUID = baseGUID
		summoned.VehicleSeat = assigned
		summoned.VehicleAccessoryMinion = acc.IsMinion
		summoned.Moving = false
		s.motionMu.Unlock()
		// VehicleJoinEvent::Execute (Vehicle.cpp:855-856): a seat
		// carrying VEHICLE_SEAT_FLAG_PASSENGER_NOT_SELECTABLE sets
		// UNIT_FLAG_NOT_SELECTABLE on the passenger.
		s.setCreatureVehicleNotSelectable(summoned, seatInfo)
		_, _, seatAddon := kit.GetSeatForPassenger(summoned.GUID)
		// VehicleJoinEvent::Execute (Vehicle.cpp:865-869): the board
		// stamps the passenger's transport with the seat offsets.
		s.motionMu.Lock()
		stampCreatureVehicleTransport(summoned, baseGUID, assigned, seatInfo, seatAddon)
		s.motionMu.Unlock()
		kit.mu.Lock()
		kit.InstalledAccessories = append(kit.InstalledAccessories, summoned.GUID)
		kit.mu.Unlock()
		// VehicleJoinEvent::Execute (Vehicle.cpp:895-896): every
		// creature the passenger threatens gains the vehicle base on
		// its threat list with the passenger's threat value, ahead
		// of the AI PassengerBoarded / OnAddPassenger hooks below.
		// VehicleJoinEvent::Execute (Vehicle.cpp:883): the board
		// breaks the passenger's target for everyone in range.
		s.sendPassengerClearTarget(origin.Map, origin.InstanceID, summoned.GUID, summoned.X, summoned.Y, summoned.Z, summoned.PhaseMask)
		s.transferPassengerThreatToVehicleBase(origin.Map, origin.InstanceID, summoned.GUID, baseGUID)
		if base := s.findCreatureMotion(origin.Map, origin.InstanceID, baseGUID); base != nil {
			// VehicleJoinEvent::Execute (Vehicle.cpp:900-902): the
			// AI PassengerBoarded(true) call precedes the
			// OnAddPassenger script hook.
			s.smartPassengerBoarded(ctx, base, summoned.GUID, assigned, true)
			s.triggerVehicleEvent(scripting.VehicleEventOnAddPassenger, s.luaMotionCreature(base), s.luaMotionCreature(summoned), assigned)
			if acc.IsMinion {
				s.triggerVehicleEvent(scripting.VehicleEventOnInstallAccessory, s.luaMotionCreature(base), s.luaMotionCreature(summoned))
			}
		}
	}
	// VehicleJoinEvent::Execute (Vehicle.cpp:825-832): installing the
	// accessories may take the last usable seat — the base's
	// spell-click flag follows the final count, not the template.
	// A player base syncs its UNIT_NPC_FLAG_PLAYER_VEHICLE half through
	// its session in the helper.
	s.syncVehicleBaseNPCFlag(origin.Map, origin.InstanceID, baseGUID)
}

func (s *Server) installPlayerVehicleAccessories(ctx context.Context, sess *session, kit *VehicleKit) {
	if s == nil || sess == nil || sess.player == nil || kit == nil {
		return
	}
	origin := tempSummonOrigin{Map: sess.player.Map, InstanceID: sess.player.InstanceID, GUID: sess.playerGUID, X: sess.player.X, Y: sess.player.Y, Z: sess.player.Z, O: sess.player.Orientation}
	s.installVehicleAccessoriesFromOrigin(ctx, origin, sess.playerGUID, sess.player.X, sess.player.Y, sess.player.Z, sess.player.Orientation, kit)
	kit.setPosition(sess.player.X, sess.player.Y, sess.player.Z, sess.player.Orientation)
	s.relocatePassengers(sess.player.Map, sess.player.InstanceID, sess.playerGUID, sess.player.X, sess.player.Y, sess.player.Z, sess.player.Orientation)
}

// removeCreatureVehicleSeat mirrors the Unit::setDeathState ExitVehicle
// leg (Unit.cpp:8915-8916) for a dying creature passenger: C++ runs the
// FULL _ExitVehicle, not a plain unseat — ExitVehicle removes the
// SPELL_AURA_CONTROL_VEHICLE aura and the remove leg runs
// caster->_ExitVehicle() (SpellAuraEffects.cpp:3000), which frees the
// seat (Vehicle::RemovePassenger, Vehicle.cpp:497-543: seat reset :515,
// OnRemovePassenger :539), relocates to the exit position (base
// position + ModifyVehiclePassengerExitPos + seat addon, Unit.cpp:
// 13215-13234), and falls to the ground on a SetFall spline when the
// passenger cannot fly (Unit.cpp:13238-13249). The SmartAI
// passenger-boarded (false) event rides the hook, and a player base
// stays silent per the C++ TYPEID_UNIT gate. The _ExitVehicle TAIL
// (Unit.cpp:13251-13266) runs in its live-base accessory half only:
// a minion accessory unsummons (UnSummon(2000)) when the base is alive.
// The dead-base half (setDeathState(JUST_DIED) on the passenger) is
// NOT run here — the passenger is already dying, and Go's
// applyCreatureDeathState would re-fire the JustDied hooks, where
// C++'s re-entrant setDeathState is a no-op in effect on an
// already-JUST_DIED unit. The minion-base half (DespawnOrUnsummon(1ms)
// on a base owned by the exiting passenger) stays out: a dying
// passenger owning its own minion vehicle base has no in-tree
// precedent. Returns false when the creature was
// not seated in the named base's kit — the seat may already be gone
// when a death sweep re-enters through applyCreatureDeathState, and
// the fire must not double. A base that teleported away keys its kit
// under another map, so every kit for the base GUID is swept, not just
// the passenger's own map.
// removeDepartingCreatureVehicleSeat unseats a creature passenger that
// is itself leaving the world (despawnCreatureMotion, pruneCreatureMotion).
// Unit::RemoveFromWorld (Unit.cpp:9798) runs ExitVehicle() before the
// object leaves the world, so Vehicle::RemovePassenger fires its hooks
// there: the AI PassengerBoarded(false) call and OnRemovePassenger
// (Vehicle.cpp:536-539), gated only on a creature base — neither is
// gated on the passenger's or the base's IsInWorld. The despawn/prune
// paths only dropped the departing unit's OWN kit (its base side),
// leaving the passenger's seat occupied on its live base with no hook
// fires — the base-side twin (removeVehicleKitWithBase, departingBase)
// already covers the reverse. The departing motion stands in for the
// passenger object, as the departing base does for the kit-drop. No
// movement tail runs here: the _ExitVehicle relocate/fall legs are
// unobservable once the destroy packet goes out, and broadcasting them
// for a deleted object would be noise. The parachute leg
// (Vehicle.cpp:533) is likewise skipped — its cast targets a unit
// mid-RemoveFromWorld.
func (s *Server) removeDepartingCreatureVehicleSeat(ctx context.Context, mapID, instanceID uint32, passenger *creatureMotion) {
	if s == nil || passenger == nil || passenger.VehicleGUID == 0 {
		return
	}
	baseGUID := passenger.VehicleGUID
	for _, loc := range s.findVehicleKits(baseGUID) {
		baseMotion := s.findCreatureMotion(loc.key.MapID, loc.key.InstanceID, baseGUID)
		seatID, _, _, ok := loc.kit.RemovePassenger(passenger.GUID)
		if !ok {
			continue
		}
		if baseMotion != nil {
			// Vehicle::RemovePassenger (Vehicle.cpp:536-539): the
			// AI PassengerBoarded(false) call precedes the
			// OnRemovePassenger script hook.
			s.smartPassengerBoarded(ctx, baseMotion, passenger.GUID, seatID, false)
			s.triggerVehicleEvent(scripting.VehicleEventOnRemovePassenger, s.luaMotionCreature(baseMotion), s.luaMotionCreature(passenger))
		}
	}
}

// propagatePlayerPhaseToVehicleRoot mirrors the vehicle leg of
// Player::SetPhaseMask (Player.cpp:22809-22810): a player's phase-mask
// change propagates to the root of the vehicle chain they ride, so the
// base stays in the rider's phase. HandlePhase reaches it through
// player->SetPhaseMask(newPhase, false) (SpellAuraEffects.cpp:1604);
// the GM and .modify phase paths reach it through SetGameMaster / the
// override slot (Player.cpp:2456). The walk climbs creature bases via
// their VehicleGUID (nested vehicles); the write lands in the runtime
// 261-phase override map — the lever Go's visibility actually reads
// (creatureEffectivePhaseMask, creaturePhaseOverrideGUIDs) — not
// motion.PhaseMask, which the creature queries never consult. The entry
// is deliberately never cleared on unseat: C++ never resets the base's
// phase either, and the base's own 261 recompute overwrites the key
// when its own phase next changes. A player base at the root keeps its
// derived phase — Go has no stored player phase mask to write, and
// forcing an override would outlive the ride. The climb is capped:
// the board cycle guards (Unit.cpp:12074-12077) forbid a loop, but a
// corrupted chain must not hang a packet handler the way C++'s
// unbounded GetVehicleRoot loop would.
func (s *Server) propagatePlayerPhaseToVehicleRoot(sess *session) {
	if s == nil || sess == nil || sess.player == nil || sess.player.VehicleGUID == 0 {
		return
	}
	mask := sess.currentPlayerPhaseMask()
	guid := sess.player.VehicleGUID
	for i := 0; guid != 0 && i < 64; i++ {
		var base *creatureMotion
		for _, loc := range s.findVehicleKits(guid) {
			if m := s.findCreatureMotion(loc.key.MapID, loc.key.InstanceID, guid); m != nil {
				base = m
				break
			}
		}
		if base == nil {
			return
		}
		s.motionMu.Lock()
		next := base.VehicleGUID
		s.motionMu.Unlock()
		if next == 0 {
			key := creatureAuraKeyForMotion(base)
			s.auraMu.Lock()
			if s.creaturePhaseOverride == nil {
				s.creaturePhaseOverride = make(map[creatureAuraKey]uint32)
			}
			s.creaturePhaseOverride[key] = mask
			s.auraMu.Unlock()
			return
		}
		guid = next
	}
}

// setCreaturePhaseMask mirrors Creature::SetPhaseMask
// (Creature.cpp:656-668): a no-op mask change neither rewrites the slot
// nor propagates; otherwise the slot is written and every seated
// passenger gets the same mask through its own SetPhaseMask (a player
// passenger through the Player::SetPhaseMask twin, a creature passenger
// recursively, so a nested vehicle base re-propagates to its own seats).
// Go keeps the phase in two levers — the stored motion.PhaseMask and the
// runtime 261-phase override map the visibility reads prefer
// (creatureEffectivePhaseMask) — so both are written; the base's own 261
// recompute overwrites the override key when its own phase next changes,
// the same way the player-side twin relies on it. The visited set caps
// the climb: the board guards forbid a seat cycle, but a corrupted chain
// must not recurse forever the way C++'s unbounded loop would.
func (s *Server) setCreaturePhaseMask(motion *creatureMotion, mask uint32) {
	if s == nil || motion == nil {
		return
	}
	s.setCreaturePhaseMaskVisited(motion, mask, map[uint64]struct{}{})
}

func (s *Server) setCreaturePhaseMaskVisited(motion *creatureMotion, mask uint32, visited map[uint64]struct{}) {
	if s == nil || motion == nil || visited == nil {
		return
	}
	s.motionMu.Lock()
	guid, mapID, instanceID, stored := motion.GUID, motion.Map, motion.InstanceID, motion.PhaseMask
	s.motionMu.Unlock()
	if _, ok := visited[guid]; ok {
		return
	}
	visited[guid] = struct{}{}
	if stored == mask && s.creatureEffectivePhaseMask(motion) == mask {
		return
	}
	s.motionMu.Lock()
	motion.PhaseMask = mask
	motion.PhaseMaskKnown = true
	s.motionMu.Unlock()
	key := creatureAuraKeyForMotion(motion)
	s.auraMu.Lock()
	if s.creaturePhaseOverride == nil {
		s.creaturePhaseOverride = make(map[creatureAuraKey]uint32)
	}
	s.creaturePhaseOverride[key] = mask
	s.auraMu.Unlock()
	// Unit::SetPhaseMask (Unit.cpp:12474-12485) runs inside the creature
	// twin before the vehicle passenger leg: controlled units and summon
	// slots take the mask, then single-target auras from casters out of
	// the new phase drop.
	s.propagateUnitPhaseMask(guid, mapID, instanceID, mask, visited)
	s.removeNotOwnSingleTargetAuras(guid, mapID, instanceID, mask)
	kit := s.getVehicleKit(mapID, instanceID, guid)
	if kit == nil {
		return
	}
	kit.mu.RLock()
	guids := make([]uint64, 0, len(kit.Seats))
	for _, seat := range kit.Seats {
		if seat != nil && seat.Passenger.GUID != 0 {
			guids = append(guids, seat.Passenger.GUID)
		}
	}
	kit.mu.RUnlock()
	for _, passengerGUID := range guids {
		if sess := s.findSessionByGUID(passengerGUID); sess != nil && sess.player != nil {
			s.miscSetPhaseMaskVisited(sess, mask, visited)
			continue
		}
		if passenger := s.findCreatureMotion(mapID, instanceID, passengerGUID); passenger != nil {
			s.setCreaturePhaseMaskVisited(passenger, mask, visited)
		}
	}
}

// propagateUnitPhaseMask mirrors the IsInWorld controlled/summon legs of
// Unit::SetPhaseMask (Unit.cpp:12474-12485): every unit this unit controls
// takes the same mask through its own SetPhaseMask (update=true, whose
// visibility tail Go has no twin for). m_Controlled is the minion set
// (Unit::SetMinion, Unit.cpp:6052) plus the charmed set
// (Unit::SetCharmerGUID, Unit.cpp:6240); Go reads both off the motion map
// (OwnerGUID / CharmerGUID), plus the pet slot
// (m_SummonSlot[SUMMON_SLOT_PET]) and the totem slots
// (SUMMON_SLOT_TOTEM_FIRE..AIR, SharedDefines.h:3326-3330) off the session
// or the motion. The visited set is shared with the caller: the pet sits
// in both the controlled set and the summon slot, and C++'s same-mask
// early return makes the second call a no-op there. Blocked legs: a
// charmed player (sessions carry no CharmerGUID), the MINIPET/QUEST
// summon slots (no Go model), and the npcbot UpdatePhaseForBots leg (no
// npcbot).
func (s *Server) propagateUnitPhaseMask(unitGUID uint64, mapID, instanceID uint32, mask uint32, visited map[uint64]struct{}) {
	if s == nil || unitGUID == 0 || visited == nil {
		return
	}
	var guids []uint64
	if sess := s.findSessionByGUID(unitGUID); sess != nil && sess.player != nil {
		if sess.player.PetGUID != 0 {
			guids = append(guids, sess.player.PetGUID)
		}
		for _, totemGUID := range sess.player.TotemSlots {
			if totemGUID != 0 {
				guids = append(guids, totemGUID)
			}
		}
	} else if motion := s.findCreatureMotion(mapID, instanceID, unitGUID); motion != nil {
		s.motionMu.Lock()
		petGUID := motion.PetGUID
		s.motionMu.Unlock()
		if petGUID != 0 {
			guids = append(guids, petGUID)
		}
	}
	s.motionMu.Lock()
	for _, other := range s.motionMapLocked(mapID, instanceID) {
		if other != nil && (other.OwnerGUID == unitGUID || other.CharmerGUID == unitGUID) {
			guids = append(guids, other.GUID)
		}
	}
	s.motionMu.Unlock()
	for _, guid := range guids {
		if sess := s.findSessionByGUID(guid); sess != nil && sess.player != nil {
			s.miscSetPhaseMaskVisited(sess, mask, visited)
			continue
		}
		if motion := s.findCreatureMotion(mapID, instanceID, guid); motion != nil {
			s.setCreaturePhaseMaskVisited(motion, mask, visited)
		}
	}
}

// unitPhaseOverlaps is the InSamePhase(uint32) twin (Object.h:324:
// (GetPhaseMask() & phasemask) != 0) for a unit resolved by GUID on one
// map/instance. A caster that resolves nowhere — logged out, despawned,
// or on another map — reads as not in phase, matching C++
// Aura::GetCaster() returning null there.
func (s *Server) unitPhaseOverlaps(guid uint64, phase, mapID, instanceID uint32) bool {
	if s == nil || guid == 0 || phase == 0 {
		return false
	}
	if sess := s.findSessionByGUID(guid); sess != nil && sess.player != nil && sess.player.Map == mapID && sess.player.InstanceID == instanceID {
		return sess.currentPlayerPhaseMask()&phase != 0
	}
	if motion := s.findCreatureMotion(mapID, instanceID, guid); motion != nil {
		return s.creatureEffectivePhaseMask(motion)&phase != 0
	}
	return false
}

// removeNotOwnSingleTargetAuras mirrors Unit::RemoveNotOwnSingleTargetAuras
// (Unit.cpp:4099-4137) for a unit that just changed phase. Loop one drops
// the unit's own single-target auras cast by someone else when the new
// phase is 0 or the caster is out of it; loop two drops single-target
// auras this unit cast on others whose owner is out of the new phase.
// Player records come from the session's activeAuras (CasterGUID +
// SingleTarget on the record); creature records from activeCreatureAuras
// under auraMu. The presence-only creatureAuras map carries neither a
// caster nor a single-target flag, so auras recorded only there are left
// alone — they cannot be proven foreign. Every map is snapshotted under
// its lock and the phase test and removal run after the unlock, so no
// lock order inverts against the motionMu -> auraMu readers.
func (s *Server) removeNotOwnSingleTargetAuras(unitGUID uint64, mapID, instanceID uint32, newPhase uint32) {
	if s == nil || unitGUID == 0 {
		return
	}
	type ownedAura struct {
		spellID    uint32
		casterGUID uint64
	}
	if sess := s.findSessionByGUID(unitGUID); sess != nil && sess.player != nil {
		sess.castMu.Lock()
		var owned []ownedAura
		for spellID, aura := range sess.activeAuras {
			if aura != nil && !aura.Stopped && aura.SingleTarget && aura.CasterGUID != unitGUID {
				owned = append(owned, ownedAura{spellID, aura.CasterGUID})
			}
		}
		sess.castMu.Unlock()
		for _, aura := range owned {
			if newPhase == 0 || !s.unitPhaseOverlaps(aura.casterGUID, newPhase, mapID, instanceID) {
				sess.removeAura(aura.spellID)
			}
		}
	} else if motion := s.findCreatureMotion(mapID, instanceID, unitGUID); motion != nil {
		key := creatureAuraKeyForMotion(motion)
		s.auraMu.Lock()
		var owned []ownedAura
		for spellID, aura := range s.activeCreatureAuras[key] {
			if aura != nil && !aura.Stopped && aura.SingleTarget && aura.CasterGUID != unitGUID {
				owned = append(owned, ownedAura{spellID, aura.CasterGUID})
			}
		}
		s.auraMu.Unlock()
		for _, aura := range owned {
			if newPhase == 0 || !s.unitPhaseOverlaps(aura.casterGUID, newPhase, mapID, instanceID) {
				s.removeCreatureAura(key, aura.spellID)
			}
		}
	}
	s.sessionsMu.RLock()
	var players []*session
	for sess := range s.sessions {
		if sess != nil && sess.player != nil && sess.playerGUID != unitGUID {
			players = append(players, sess)
		}
	}
	s.sessionsMu.RUnlock()
	for _, sess := range players {
		sess.castMu.Lock()
		var drop []uint32
		for spellID, aura := range sess.activeAuras {
			if aura != nil && !aura.Stopped && aura.SingleTarget && aura.CasterGUID == unitGUID {
				drop = append(drop, spellID)
			}
		}
		phase := sess.currentPlayerPhaseMask()
		sess.castMu.Unlock()
		if phase&newPhase != 0 {
			continue
		}
		for _, spellID := range drop {
			sess.removeAura(spellID)
		}
	}
	s.auraMu.Lock()
	type creatureDrop struct {
		key     creatureAuraKey
		spellID uint32
	}
	var creatureDrops []creatureDrop
	for key, auras := range s.activeCreatureAuras {
		if key.GUID == unitGUID {
			continue
		}
		for spellID, aura := range auras {
			if aura != nil && !aura.Stopped && aura.SingleTarget && aura.CasterGUID == unitGUID {
				creatureDrops = append(creatureDrops, creatureDrop{key, spellID})
			}
		}
	}
	s.auraMu.Unlock()
	for _, d := range creatureDrops {
		phase := uint32(0)
		if motion := s.findCreatureMotion(d.key.Map, d.key.InstanceID, d.key.GUID); motion != nil {
			phase = s.creatureEffectivePhaseMask(motion)
		}
		if phase&newPhase == 0 {
			s.removeCreatureAura(d.key, d.spellID)
		}
	}
}

// setCreatureVehicleNotSelectable mirrors the VehicleJoinEvent::Execute
// NOT_SELECTABLE leg (Vehicle.cpp:855-856) for a creature passenger: a
// seat carrying VEHICLE_SEAT_FLAG_PASSENGER_NOT_SELECTABLE sets
// UNIT_FLAG_NOT_SELECTABLE on the passenger. The player board
// (enterVehicle) already sets it; the creature boards (Lua AddPassenger,
// the accessory installs, the 236 aura-apply leg via
// luaVehicleAddPassenger) never did, so a seated creature stayed
// selectable. The flag rides the values-update broadcast, as other
// creature UnitFlags changes do.
func (s *Server) setCreatureVehicleNotSelectable(motion *creatureMotion, seatInfo *wotlk.VehicleSeatEntry) {
	if s == nil || motion == nil || seatInfo == nil || !seatInfo.HasFlag(wotlk.VehicleSeatFlagPassengerNotSelectable) {
		return
	}
	s.motionMu.Lock()
	motion.UnitFlags |= 0x02000000 // UNIT_FLAG_NOT_SELECTABLE
	flags := motion.UnitFlags
	s.motionMu.Unlock()
	s.broadcastCreatureValuesUpdateInInstance(motion.Map, motion.InstanceID, motion.GUID, map[int]uint32{unitFieldFlags: flags})
}

// clearCreatureVehicleNotSelectable mirrors Vehicle::RemovePassenger
// (Vehicle.cpp:512-514): the flag clears only when the seat carried it
// and the passenger did not carry it before boarding (the IsUnselectable
// recorded by AddPassenger). The player exit (exitVehicle) already
// restores it; the creature exits never did.
func (s *Server) clearCreatureVehicleNotSelectable(motion *creatureMotion, seatInfo *wotlk.VehicleSeatEntry, wasUnselectable bool) {
	if s == nil || motion == nil || seatInfo == nil || wasUnselectable || !seatInfo.HasFlag(wotlk.VehicleSeatFlagPassengerNotSelectable) {
		return
	}
	s.motionMu.Lock()
	motion.UnitFlags &^= 0x02000000 // UNIT_FLAG_NOT_SELECTABLE
	flags := motion.UnitFlags
	s.motionMu.Unlock()
	s.broadcastCreatureValuesUpdateInInstance(motion.Map, motion.InstanceID, motion.GUID, map[int]uint32{unitFieldFlags: flags})
}

func (s *Server) removeCreatureVehicleSeat(ctx context.Context, mapID, instanceID uint32, passengerGUID, baseGUID uint64) bool {
	if s == nil || passengerGUID == 0 || baseGUID == 0 {
		return false
	}
	removed := false
	var exitMotion *creatureMotion
	var exitX, exitY, exitZ, exitO float32
	var exitMapID, exitInstanceID, exitPhase uint32
	// tailBase is the first base motion the sweep resolves, for the
	// _ExitVehicle tail's live-base test below; a base that never
	// resolves (despawned, kit already dropped) leaves the tail to the
	// kit-drop sweep, which already ran it.
	var tailBase *creatureMotion
	for _, loc := range s.findVehicleKits(baseGUID) {
		baseMotion := s.findCreatureMotion(loc.key.MapID, loc.key.InstanceID, baseGUID)
		if tailBase == nil {
			tailBase = baseMotion
		}
		passengerMotion := s.findCreatureMotion(loc.key.MapID, loc.key.InstanceID, passengerGUID)
		// Unit::_ExitVehicle (Unit.cpp:13193): the seat addon is read
		// BEFORE RemovePassenger — after removal the seat is empty
		// and the lookup finds nothing — so the exit position is
		// captured here, ahead of the seat free below.
		var px, py, pz, po float32
		if passengerMotion != nil {
			px, py, pz, po = s.creatureVehicleExitPosition(baseGUID, passengerMotion, baseMotion)
		}
		seatID, seatInfo, wasUnselectable, ok := loc.kit.RemovePassenger(passengerGUID)
		if !ok {
			continue
		}
		removed = true
		// Vehicle::RemovePassenger (Vehicle.cpp:512-514, :533): the
		// NOT_SELECTABLE flag restores — the seat carried it and the
		// passenger did not before boarding — and a flying passenger
		// is handed the parachute spell, both ahead of the remove
		// hooks below.
		s.clearCreatureVehicleNotSelectable(passengerMotion, seatInfo, wasUnselectable)
		s.castCreatureVehicleExitParachute(ctx, baseGUID, passengerMotion)
		if baseMotion != nil {
			// Vehicle::RemovePassenger (Vehicle.cpp:536-539): the
			// AI PassengerBoarded(false) call precedes the
			// OnRemovePassenger script hook.
			s.smartPassengerBoarded(ctx, baseMotion, passengerGUID, seatID, false)
			if passengerMotion != nil {
				s.triggerVehicleEvent(scripting.VehicleEventOnRemovePassenger, s.luaMotionCreature(baseMotion), s.luaMotionCreature(passengerMotion))
			}
		}
		if exitMotion == nil && passengerMotion != nil {
			exitMotion = passengerMotion
			exitX, exitY, exitZ, exitO = px, py, pz, po
			s.motionMu.Lock()
			exitMapID, exitInstanceID, exitPhase = passengerMotion.Map, passengerMotion.InstanceID, passengerMotion.PhaseMask
			s.motionMu.Unlock()
		}
	}
	if removed {
		if motion := s.findCreatureMotion(mapID, instanceID, passengerGUID); motion != nil {
			s.motionMu.Lock()
			motion.VehicleGUID = 0
			motion.VehicleSeat = 0
			motion.Moving = false
			s.motionMu.Unlock()
			// Vehicle::RemovePassenger (Vehicle.cpp:518-529): the
			// transport leg runs here, after the seat frees and
			// before the unroot below.
			s.inheritOrResetCreatureVehicleTransport(mapID, instanceID, motion, baseGUID)
		}
		// Unit::_ExitVehicle (Unit.cpp:13202-13210): the seat freed
		// above, the board root clears unless a MOD_ROOT aura or the
		// movement template holds it, and exactly one
		// SMSG_SPLINE_MOVE_UNROOT goes out — via SetRooted for a
		// non-stunned passenger, via the _ExitVehicle arm when
		// stunned — ahead of the exit-position relocate below, as in C++.
		s.unrootExitingCreaturePassenger(exitMotion)
		if exitMotion != nil {
			// Unit::_ExitVehicle (Unit.cpp:13215-13249): the dying
			// passenger relocates to the exit position (base position
			// + ModifyVehiclePassengerExitPos + seat addon, captured
			// above) and, when it cannot fly and the exit Z plus the
			// base's collision height is above the ground, falls to
			// it on a SetFall spline (EVENT_VEHICLE_EXIT) instead of
			// hanging at the seat-offset position in mid-air. Without
			// a fall the move rides the teleport broadcast, as in the
			// Lua ExitVehicle arm. The launch lifts the Go-side dead
			// refusal: MotionMaster::MoveFall has no dead gate
			// (MotionMaster.cpp:919-955).
			s.motionMu.Lock()
			exitMotion.VehicleGUID = 0
			exitMotion.VehicleSeat = 0
			exitMotion.Moving = false
			exitMotion.X, exitMotion.Y, exitMotion.Z, exitMotion.Orientation = exitX, exitY, exitZ, exitO
			s.motionMu.Unlock()
			if !s.fallAfterVehicleExitAllowDead(exitMapID, exitInstanceID, baseGUID, exitMotion, true) {
				_, nearby := buildTeleportMovementPackets(passengerGUID, movementInfo{Time: s.gameTimeMilliseconds(), X: exitX, Y: exitY, Z: exitZ, Orientation: exitO})
				s.broadcastCreatureTeleportMovement(exitMapID, exitInstanceID, passengerGUID, exitX, exitY, exitZ, nearby, exitPhase)
			}
		}
		// Unit::_ExitVehicle tail (Unit.cpp:13254-13256): a minion
		// vehicle base owned by the exiting passenger despawns 1ms
		// after the exit. The Lua ExitVehicle arm and the session
		// exitVehicle run this leg through exitVehicleTail; this
		// server-side unseat is the third _ExitVehicle caller and
		// carried only the accessory half below.
		if exitMotion != nil && tailBase != nil && tailBase.IsMinion && tailBase.OwnerGUID == passengerGUID {
			bmap, binst, bguid := tailBase.Map, tailBase.InstanceID, tailBase.GUID
			time.AfterFunc(time.Millisecond, func() {
				s.despawnCreatureMotion(context.Background(), bmap, binst, bguid)
			})
		}
		if exitMotion != nil && exitMotion.VehicleAccessoryMinion {
			// Unit::_ExitVehicle tail (Unit.cpp:13258-13266): a minion
			// accessory unsummons (UnSummon(2000)) when the base is
			// alive — the 2s delayed expiry, shared with the
			// evade-reinstall and respawn eject paths. The dead-base
			// branch is skipped per the header: the passenger is
			// already dying. Base liveness mirrors exitVehicleTail's
			// dead test (Health 0 or a non-ALIVE DeathState; a player
			// base via its session); an unresolvable base skips the
			// tail — its kit-drop already ran it through
			// exitCreatureVehiclePassengers.
			baseDead := false
			baseKnown := false
			if tailBase != nil {
				baseDead = tailBase.Health == 0 || tailBase.DeathState != creatureDeathAlive
				baseKnown = true
			} else if baseSess := s.findSessionByGUID(baseGUID); baseSess != nil && baseSess.player != nil {
				baseDead = baseSess.player.Health == 0
				baseKnown = true
			}
			if baseKnown && !baseDead {
				s.unsummonEjectedVehicleAccessory(exitMotion, true)
			}
		}
		// Vehicle NPC-flag lifecycle (Vehicle.cpp:508-509, :825-832):
		// freeing a usable seat restores the base's spell-click flag;
		// the helper resolves a base whose kit is keyed on another map.
		s.syncVehicleBaseNPCFlag(mapID, instanceID, baseGUID)
	}
	return removed
}

// exitDeadPlayerBasePassengers mirrors Vehicle::RemoveAllPassengers
// (Vehicle.cpp:231-257) for a dying player base: every passenger casts a
// SPELL_AURA_CONTROL_VEHICLE aura on the base (Vehicle.cpp:256-257) and
// RemoveAllAurasOnDeath strips it (Unit.cpp:4353-4372), whose remove leg
// (SpellAuraEffects.cpp:2987-3008) runs caster->_ExitVehicle() — but Go
// models no ride aura for creature passengers and a player passenger
// may board aura-less through Lua, so the kit is swept explicitly after
// the death aura strip: player passengers ride the session exitVehicle,
// creature passengers ride exitCreatureVehiclePassengers with
// baseJustDied=true (the base is JUST_DIED while the strip runs,
// Unit.cpp:8903/8921). A player base fires no OnRemovePassenger per the
// C++ TYPEID_UNIT gate — the creature helper skips the fire when no base
// motion exists.
func (s *Server) exitDeadPlayerBasePassengers(ctx context.Context, mapID, instanceID uint32, playerGUID uint64) {
	if s == nil || playerGUID == 0 {
		return
	}
	kit := s.getVehicleKit(mapID, instanceID, playerGUID)
	if kit == nil {
		return
	}
	for _, pg := range kit.seatedPassengerGUIDs() {
		if ps := s.findSessionByGUID(pg); ps != nil && ps.player != nil {
			ps.exitVehicle(ctx)
		}
	}
	s.exitCreatureVehiclePassengers(mapID, instanceID, kit, true, nil)
}

// fallAfterVehicleExitAllowDead mirrors the Unit::_ExitVehicle fall leg
// (Unit.cpp:13238-13249) for a creature passenger leaving a base through
// the server-side eject paths: a creature that cannot fly falls from the
// exit position to the ground on a SetFall spline (EVENT_VEHICLE_EXIT,
// id 1008) when the exit Z plus the base's collision height is above the
// water-or-ground level, reusing the Lua ExitVehicle leg's helpers. It
// reports whether a fall spline launched, so the caller can broadcast the
// exit-position teleport when no fall runs, as the Lua ExitVehicle arm
// does. allowDead lifts the Go-side Health==0 refusal in the launch:
// the dying-passenger leg (Unit::setDeathState, Unit.cpp:8915-8916)
// passes true because the JUST_DIED _ExitVehicle fall launches
// (MotionMaster::MoveFall has no dead gate, MotionMaster.cpp:919-955),
// and the eject/kit-drop callers pass true too — C++ _ExitVehicle falls
// a seated corpse, and the lift is harmless when none is seated: the
// kill funnels unseat a dying passenger at death time (session via
// kill.go onCreatureKilled, the rest via clearCreatureDeathState).
// Root/stun/not-move-aura and drop <0.1 still refuse, as in C++.
func (s *Server) fallAfterVehicleExitAllowDead(mapID, instanceID uint32, baseGUID uint64, motion *creatureMotion, allowDead bool) bool {
	if s == nil || motion == nil {
		return false
	}
	s.motionMu.Lock()
	x, y, z := motion.X, motion.Y, motion.Z
	canFly := motion.CanFly || motion.Flying
	s.motionMu.Unlock()
	if canFly {
		return false
	}
	height := z + s.vehicleBaseCollisionHeight(mapID, instanceID, baseGUID)
	level, ground := s.mapWaterOrGroundLevel(mapID, x, y, height, terrainDefaultCollisionHeight)
	if level > terrainInvalidHeight && height > level {
		return s.launchCreatureFallSplineAllowDead(motion, x, y, ground, movementInformVehicleExit, allowDead)
	}
	return false
}

// creatureVehicleExitPosition resolves a creature passenger's exit
// position for the server-side eject: the vehicle BASE position with the
// passenger's orientation, then the ModifyVehiclePassengerExitPos
// override, then the seat addon's Offset/Dest transform.
// Reference: Unit::_ExitVehicle (Unit.cpp:13215-13234). The Lua
// ExitVehicle arm (lua_creature_events.go) runs the same three legs; the
// player arm (applyVehicleExitPosition) is session-bound and
// player-base-only, so the creature leg mirrors the Lua arm here and
// shares its addon math (applyVehicleSeatExitAddon). The base is the
// motion the caller already resolved — live, or the departing motion a
// despawn/prune kit-drop stands in with (removeVehicleKitWithBase):
// C++ drops the kit while the base is still in the world
// (Unit.cpp:9789), so the departing motion's last position is the exit
// source there. The addon is read before the caller frees the seat:
// after RemovePassenger the lookup finds nothing (Unit.cpp:13193). A base that does not resolve on the
// passenger's map/instance keeps the passenger's own position, which
// the passenger sync keeps at the base while seated.
func (s *Server) creatureVehicleExitPosition(baseGUID uint64, motion, baseMotion *creatureMotion) (x, y, z, o float32) {
	if s == nil || motion == nil {
		return 0, 0, 0, 0
	}
	s.motionMu.Lock()
	x, y, z, o = motion.X, motion.Y, motion.Z, motion.Orientation
	entry := motion.Entry
	guid := motion.GUID
	if baseMotion != nil {
		x, y, z = baseMotion.X, baseMotion.Y, baseMotion.Z
	}
	s.motionMu.Unlock()
	if baseMotion == nil {
		if baseSess := s.findSessionByGUID(baseGUID); baseSess != nil && baseSess.player != nil {
			x, y, z = baseSess.player.X, baseSess.player.Y, baseSess.player.Z
		}
	}
	if exit, ok := vehiclePassengerExitPosOverrides[entry]; ok {
		x, y, z, o = exit[0], exit[1], exit[2], exit[3]
	}
	return applyVehicleSeatExitAddon(x, y, z, o, s.vehicleSeatAddonForPassenger(baseGUID, guid))
}

// exitCreatureVehiclePassengers bridges the Unit::_ExitVehicle
// exit-position (Unit.cpp:13215-13234) and tail (Unit.cpp:13252-13266)
// legs for creature passengers leaving a base: a
// minion accessory (UNIT_MASK_ACCESSORY) dies with a just-died base
// (Unit.cpp:13261-13262) or unsummons 2000ms after the exit when the
// base is alive or a corpse (Unit.cpp:13265), the exitVehicleTail
// twin, while a non-minion accessory simply unseats and stays in the
// world at the exit position. baseJustDied is true only
// for the death-time eject (clearCreatureAurasOnDeath, the
// RemoveAllAurasOnDeath twin, which runs while the base is still
// JUST_DIED); kit drops (removeVehicleKit, the RemoveFromWorld
// twin) always pass false — a dead base there is CORPSE, never
// JUST_DIED (Creature.cpp:2115 promotes after the strip). The base
// motion cannot tell the phases apart — Health 0 reads dead in
// both — so the caller passes the phase. Player seats are
// untouched: the player eject/logout arms own those sessions.
func (s *Server) exitCreatureVehiclePassengers(mapID, instanceID uint32, kit *VehicleKit, baseJustDied bool, departingBase *creatureMotion) {
	if s == nil || kit == nil {
		return
	}
	// Vehicle::Uninstall (Vehicle.cpp:123) removes all passengers before
	// OnUninstall fires, and each RemovePassenger fires OnRemovePassenger
	// for creature bases (Vehicle.cpp:538-539); a player base stays silent
	// per the C++ TYPEID_UNIT gate, so a nil base object skips the fire.
	// departingBase is the base motion when it was already deleted
	// (despawn/prune kit-drop): the live lookup misses, so the departing
	// motion stands in for the base object, as C++ fires the hook while
	// the base is still in the world.
	var vehicleObj *scripting.Object
	var baseMotion *creatureMotion
	if base := s.findCreatureMotion(mapID, instanceID, kit.VehicleGUID); base != nil {
		baseMotion = base
		vehicleObj = s.luaMotionCreature(base)
	} else if departingBase != nil {
		baseMotion = departingBase
		vehicleObj = s.luaMotionCreature(departingBase)
	}
	for _, guid := range kit.seatedPassengerGUIDs() {
		if sess := s.findSessionByGUID(guid); sess != nil && sess.player != nil {
			// A live base's players eject through the session arm
			// (exitVehicle fires ev6 itself while the base motion
			// still resolves) — including the no-aura boardings
			// (Lua Vehicle:AddPassenger) the death aura strip
			// never reaches: RemoveAllPassengers (Vehicle.cpp:
			// 231-257) removes CONTROL_VEHICLE auras, but a
			// passenger with no 236 aura has nothing to strip, so
			// skipping here stranded them seated on a dead base
			// with no ev6. The 236-boarded are already out by the
			// time this sweep runs (the strip's remove leg exits
			// them first), so this is a no-op for them, not a
			// second fire. A departing base's players have no
			// other eject — the kit-drop is their _ExitVehicle, so
			// the seat frees first (Vehicle.cpp:518), ev6 fires on
			// the departing base (:538-539), and the session tail
			// runs with its own fire arm no-op'd (seat gone, kit
			// deregistered, base motion deleted).
			if departingBase == nil {
				sess.exitVehicle(context.Background())
				continue
			}
			seatID, seatInfo, wasUnselectable, ok := kit.RemovePassenger(guid)
			if !ok {
				continue
			}
			// Vehicle::RemovePassenger (Vehicle.cpp:512-514, :517-518,
			// :533): the kit-drop is the passenger's _ExitVehicle,
			// but the session tail below no-ops its own RemovePassenger
			// legs (seat gone, kit deregistered, base motion deleted),
			// so the flag restore, charm drop, and parachute run here,
			// ahead of the hooks.
			sess.applyVehicleSeatNotSelectable(seatInfo, nil, wasUnselectable)
			if seatInfo != nil && seatInfo.CanControl() && !kit.IsPlayer {
				sess.sendClientControl(kit.VehicleGUID, false)
				sess.sendVehiclePetSpells(context.Background(), 0, 0, nil)
			}
			sess.castVehicleExitParachute(context.Background(), kit.VehicleGUID)
			if vehicleObj != nil {
				// Vehicle::RemovePassenger (Vehicle.cpp:536-539):
				// the AI PassengerBoarded(false) call precedes the
				// OnRemovePassenger script hook.
				s.smartPassengerBoarded(context.Background(), baseMotion, guid, seatID, false)
				s.triggerVehicleEvent(scripting.VehicleEventOnRemovePassenger, vehicleObj, sess.luaPlayer())
			}
			sess.exitVehicle(context.Background())
			continue
		}
		motion := s.findCreatureMotion(mapID, instanceID, guid)
		if motion == nil {
			continue
		}
		// Unit::_ExitVehicle (Unit.cpp:13215-13234): the exit position
		// is the vehicle base position with the passenger's
		// orientation, then the ModifyVehiclePassengerExitPos
		// override, then the seat addon's Offset/Dest transform.
		// Captured BEFORE the seat frees below (Unit.cpp:13193):
		// after RemovePassenger the addon lookup finds nothing.
		// Until now the passenger kept its last relocated
		// (seat-offset) position and orientation through the eject.
		exitX, exitY, exitZ, exitO := s.creatureVehicleExitPosition(kit.VehicleGUID, motion, baseMotion)
		// Unit::_ExitVehicle (Unit.cpp:13196) frees the seat first —
		// Vehicle::RemovePassenger resets it (Vehicle.cpp:518)
		// before the OnRemovePassenger hook (:538-539) — so a kit
		// that survives the exit (a dead base's kit, dropped only at
		// RemoveFromWorld) does not keep listing the departed
		// passenger; the resetVehicleKitOnRespawn twin already drops
		// stale seats the same way.
		seatID, seatInfo, wasUnselectable, seatOK := kit.RemovePassenger(guid)
		// Vehicle::RemovePassenger (Vehicle.cpp:512-514, :533): the
		// NOT_SELECTABLE flag restores and a flying passenger is
		// handed the parachute spell, ahead of the hooks below.
		s.clearCreatureVehicleNotSelectable(motion, seatInfo, wasUnselectable)
		s.castCreatureVehicleExitParachute(context.Background(), kit.VehicleGUID, motion)
		if vehicleObj != nil && seatOK {
			// Vehicle::RemovePassenger (Vehicle.cpp:536-539): the
			// AI PassengerBoarded(false) call precedes the
			// OnRemovePassenger script hook. A seat already freed
			// fires nothing: C++ asserts the seat exists, and the
			// kit-drop sweep already fired ev6 for it.
			s.smartPassengerBoarded(context.Background(), baseMotion, guid, seatID, false)
			s.triggerVehicleEvent(scripting.VehicleEventOnRemovePassenger, vehicleObj, s.luaMotionCreature(motion))
		}
		// Unit::_ExitVehicle (Unit.cpp:13202-13210): the seat freed
		// above, the board root clears unless a MOD_ROOT aura or the
		// movement template holds it, and exactly one
		// SMSG_SPLINE_MOVE_UNROOT goes out — via SetRooted for a
		// non-stunned passenger, via the _ExitVehicle arm when
		// stunned — ahead of the exit-position relocate below, as in C++.
		s.unrootExitingCreaturePassenger(motion)
		// The exit position lands here, before the tail below, so the
		// fall spline launches from it (Unit.cpp:13238-13249) and the
		// corpse of a just-died accessory rests at it (:13261-13262).
		s.motionMu.Lock()
		motion.X, motion.Y, motion.Z, motion.Orientation = exitX, exitY, exitZ, exitO
		phase := motion.PhaseMask
		s.motionMu.Unlock()
		if motion.VehicleAccessoryMinion {
			// Unit::_ExitVehicle (Unit.cpp:13258-13266): a minion
			// accessory dies with a JUST_DIED base and unsummons
			// (UnSummon(2000)) on any other exit. The two call
			// sites observe different phases: the death-time eject
			// runs while the base is still JUST_DIED —
			// Unit::setDeathState sets JUST_DIED before the strip
			// (Unit.cpp:8903/8921), and Creature::setDeathState
			// promotes to CORPSE only after (Creature.cpp:2115) —
			// so baseJustDied is true there and the accessory
			// dies. A kit drop always observes CORPSE or a live
			// base, so baseJustDied is false and the accessory
			// takes the 2s unsummon.
			if baseJustDied {
				// No fall spline here: C++ launches it in _ExitVehicle
				// (Unit.cpp:13238-13249) and the JUST_DIED arm then
				// clears the motion master (Unit.cpp:8927-8940), so the
				// spline never runs — the corpse stays at the exit
				// position. The move there rides the teleport
				// broadcast, as in the Lua ExitVehicle arm.
				_, nearby := buildTeleportMovementPackets(guid, movementInfo{Time: s.gameTimeMilliseconds(), X: exitX, Y: exitY, Z: exitZ, Orientation: exitO})
				s.broadcastCreatureTeleportMovement(mapID, instanceID, guid, exitX, exitY, exitZ, nearby, phase)
				s.applyCreatureDeathState(context.Background(), motion, creatureDeathJustDied)
			} else {
				// The kit is dropped, so the passenger has exited:
				// unseat now and let the unsummon ride the 2s delay
				// instead of expiring at once.
				s.motionMu.Lock()
				motion.VehicleGUID = 0
				motion.VehicleSeat = 0
				motion.Moving = false
				s.motionMu.Unlock()
				// Vehicle::RemovePassenger (Vehicle.cpp:518-529): the
				// transport leg runs here, after the seat frees and
				// before the fall below.
				s.inheritOrResetCreatureVehicleTransport(mapID, instanceID, motion, kit.VehicleGUID)
				// Unit::_ExitVehicle (Unit.cpp:13238-13249): the fall
				// spline launches before the 2s unsummon, as in C++,
				// so the accessory falls for its remaining lifetime.
				// The launch lifts the dead refusal (see the
				// fallAfterVehicleExitAllowDead twin): C++ _ExitVehicle
				// falls a seated corpse. Without a fall the move to the
				// exit position rides the teleport broadcast, as in the
				// Lua ExitVehicle arm.
				if !s.fallAfterVehicleExitAllowDead(mapID, instanceID, kit.VehicleGUID, motion, true) {
					_, nearby := buildTeleportMovementPackets(guid, movementInfo{Time: s.gameTimeMilliseconds(), X: exitX, Y: exitY, Z: exitZ, Orientation: exitO})
					s.broadcastCreatureTeleportMovement(mapID, instanceID, guid, exitX, exitY, exitZ, nearby, phase)
				}
				s.unsummonEjectedVehicleAccessory(motion, true)
			}
			continue
		}
		s.motionMu.Lock()
		motion.VehicleGUID = 0
		motion.VehicleSeat = 0
		motion.Moving = false
		s.motionMu.Unlock()
		// Vehicle::RemovePassenger (Vehicle.cpp:518-529): the
		// transport leg runs here, after the seat frees and
		// before the fall below.
		s.inheritOrResetCreatureVehicleTransport(mapID, instanceID, motion, kit.VehicleGUID)
		// Unit::_ExitVehicle (Unit.cpp:13238-13249): a ground creature
		// ejected mid-air falls to the ground on a SetFall spline
		// instead of hanging at the exit position in the air. The
		// launch lifts the dead refusal (see the
		// fallAfterVehicleExitAllowDead twin): C++ _ExitVehicle falls
		// a seated corpse. Without a fall the move to the exit
		// position rides the teleport broadcast, as in the Lua
		// ExitVehicle arm.
		if !s.fallAfterVehicleExitAllowDead(mapID, instanceID, kit.VehicleGUID, motion, true) {
			_, nearby := buildTeleportMovementPackets(guid, movementInfo{Time: s.gameTimeMilliseconds(), X: exitX, Y: exitY, Z: exitZ, Orientation: exitO})
			s.broadcastCreatureTeleportMovement(mapID, instanceID, guid, exitX, exitY, exitZ, nearby, phase)
		}
	}
	// Vehicle NPC-flag lifecycle (Vehicle.cpp:508-509, :825-832): the
	// sweep freed every usable seat, so the base's spell-click flag is
	// restored; a departing base resolves to no motion and no-ops.
	s.syncVehicleBaseNPCFlag(mapID, instanceID, kit.VehicleGUID)
}

// syncCreatureVehiclePassengers moves a creature base's passengers with it.
// Reference: Map::CreatureRelocation (Map.cpp:1136) and
// Map::MoveAllCreaturesInMoveList (Map.cpp:1294) — every creature
// relocation fans out through Vehicle::RelocatePassengers
// (Vehicle.cpp:554-578), and Unit::UpdateOrientation/UpdateHeight do
// the same for turn/height-only moves (Unit.cpp:13438-13446). The
// player-packet path already does this (movement.go), but creature
// bases move server-side (AI step, Lua Teleport, spell teleport) and
// no creaturemotion path called relocatePassengers, so a passenger's
// server coordinates stayed at the boarding point while the base
// wandered, chased, or teleported away. The kit's canonical position
// tracks the motion for the same reason: createVehicleKit never seeds
// it, and only the player-packet path called setPosition, so a
// creature base's kit sat at 0,0,0. Transport-riding bases stay out:
// their passengers relocate through the transport arm (transports.go).
// A base riding a vehicle keeps syncing: its TransportGUID is the
// stamped vehicle base (VehicleJoinEvent :865-869), not a real transport.
func (s *Server) syncCreatureVehiclePassengers(motion *creatureMotion) {
	if s == nil || motion == nil || (motion.TransportGUID != 0 && motion.VehicleGUID == 0) {
		return
	}
	kit := s.getVehicleKit(motion.Map, motion.InstanceID, motion.GUID)
	if kit == nil {
		return
	}
	// Vehicle::Install (Vehicle.cpp:78-82) fires OnInstall when a creature
	// base enters the world; Go kits materialize without an AddToWorld
	// twin, so the first passenger sync — which every vehicle base's step
	// path funnels through, after motionMu is released — fires it once.
	fireInstall := false
	kit.mu.Lock()
	if !kit.InstallEventFired {
		kit.InstallEventFired = true
		fireInstall = true
	}
	kit.mu.Unlock()
	if fireInstall {
		s.triggerVehicleEvent(scripting.VehicleEventOnInstall, s.luaMotionCreature(motion))
	}
	s.installVehicleAccessories(context.Background(), motion, kit)
	kit.setPosition(motion.X, motion.Y, motion.Z, motion.Orientation)
	s.relocatePassengers(motion.Map, motion.InstanceID, motion.GUID, motion.X, motion.Y, motion.Z, motion.Orientation)
}

// relocatePassengers updates global coordinates for all passengers on vehicle.
// Reference: Vehicle::RelocatePassengers (Vehicle.cpp:554-578).
// The C++ fan-out does not stop at coordinates: each passenger's
// UpdatePosition runs Map::PlayerRelocation (Map.cpp:1086-1112), which
// sends UpdatePositionData and refreshes object visibility, and the
// passenger's own position tick then picks up zone/area (the 1s
// ZONE_UPDATE_INTERVAL timer, Player.cpp:1218-1242) and area exploration
// (Player::UpdatePosition, Player.cpp:6472-6480). Go has no per-tick
// player update, so the follow-ups ride here: zone/area (throttled to
// 1s inside updateZoneAndArea, matching the C++ interval), duel bounds
// (the driver arm in movement.go already runs both for the mover), and
// the >30-unit nearby-object stream (the mover arm in movement.go and
// the transport arm in transports.go both stream at that threshold;
// vehicle passengers are excluded from the mover stream, so without
// this a carried passenger's own client never learns the new
// neighbourhood). The mover's own arms still run after this call and
// are idempotent under the throttle, so a driver seated on its own
// vehicle is not double-processed.
func (s *Server) relocatePassengers(mapID, instanceID uint32, vehicleGUID uint64, transX, transY, transZ, transO float32) {
	if s == nil {
		return
	}
	kit := s.getVehicleKit(mapID, instanceID, vehicleGUID)
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
		if sess := s.findSessionByGUID(rel.guid); sess != nil && sess.worldReady.Load() && sess.player != nil {
			sess.player.X = rel.x
			sess.player.Y = rel.y
			sess.player.Z = rel.z
			sess.player.Orientation = rel.o
			sess.updateZoneAndArea(context.Background(), false)
			sess.checkDuelBounds()
			dx := float64(rel.x - sess.lastStreamX)
			dy := float64(rel.y - sess.lastStreamY)
			if dx*dx+dy*dy > 30*30 {
				sess.streamNearbyObjects(context.Background())
			}
		} else {
			s.relocateCreaturePassenger(mapID, instanceID, rel.guid, rel.x, rel.y, rel.z, rel.o)
		}
	}
}

// relocateCreaturePassenger moves a creature passenger with its base.
// Reference: Vehicle::RelocatePassengers (Vehicle.cpp:554-578) resolves
// every seat through ObjectAccessor::GetUnit, not players only, so a
// creature passenger's UpdatePosition follows the base. Go seats are
// player-only today (the sole AddPassenger call site boards
// s.playerGUID; template accessories are data-only in createVehicleKit
// and InstallAccessory has no bridge), so this arm is currently
// unreachable — it closes the fan-out at the shared helper so a future
// creature-boarding path cannot silently strand its passengers.
// Creatures carry no zone/duel/stream state, so only the coords move,
// plus a MonsterMoveStop at the new position (the
// instantaneous-reposition twin the Lua Teleport bridge uses) so nearby
// clients learn the new spot. Locking: findSessionByGUID releases
// sessionsMu before this runs, and motionMu is released before the
// broadcast takes sessionsMu, so the two locks never nest.
func (s *Server) relocateCreaturePassenger(mapID, instanceID uint32, guid uint64, x, y, z, o float32) {
	if s == nil {
		return
	}
	s.motionMu.Lock()
	motion := s.findCreatureMotionLocked(mapID, instanceID, guid)
	if motion == nil {
		s.motionMu.Unlock()
		return
	}
	motion.X, motion.Y, motion.Z, motion.Orientation = x, y, z, o
	phase := motion.PhaseMask
	transportGUID := motion.TransportGUID
	s.motionMu.Unlock()
	s.broadcastMonsterMoveStopInInstance(mapID, instanceID, guid, x, y, z, phase, transportGUID)
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
		s.server.broadcastToVisibleSetFromSession(s, uint16(protocol.OpcodeSMSG_PLAYER_VEHICLE_DATA), buf.Bytes())
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
func (s *session) sendVehiclePetSpells(ctx context.Context, vehicleGUID uint64, vehicleEntry uint32, spells []uint32) {
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
	buf.WriteU16(0) // pet family (0 for all vehicles)
	// Duration (Player.cpp:21188): temp-summon timer for summoned vehicles;
	// Go has no summon-timer model on vehicle kits, and non-summoned vehicle
	// bases carry 0 in C++ too.
	buf.WriteU32(0)
	// React state (Player.cpp:21190): vehicle->GetReactState(). Creatures
	// default to REACT_AGGRESSIVE (Creature.cpp:257) and Go has no react-state
	// model that could change it, so the C++ default is sent.
	buf.WriteU8(2)      // REACT_AGGRESSIVE
	buf.WriteU8(0)      // command state
	buf.WriteU16(0x800) // disableActions (0x800 for all vehicles)

	// Player.cpp:21193-21208: the bar carries MAX_CREATURE_SPELLS (8)
	// creature spells packed with MAKE_UNIT_ACTION_BUTTON(spellId, i+8); the
	// last two of the 10 slots are always zero. Spells past index 7 never
	// reach the client in C++. An empty/invalid slot still carries the i+8
	// type byte (the u16(0)/u8(0)/u8(i+8) miss arm), not a bare zero; the
	// condition-fail arm shares that shape; the vehicle-spell condition
	// gate (ConditionMgr.cpp:1039-1053) sends that same miss arm for a
	// spell whose conditions fail — TDB 26091 carries 5
	// SourceTypeOrReferenceId 21 rows (Oculus drake spells
	// 49592/50253/50344 gated on Urom DONE via CONDITION_INSTANCE_INFO,
	// ICC abomination 71516 gated on item 49888 equipped), so the gate
	// is a real gap, not vacuous.
	for i := 0; i < 10; i++ {
		if i < 8 && i < len(spells) && spells[i] > 0 {
			if meets, err := s.meetVehicleSpellConditions(ctx, vehicleEntry, spells[i], vehicleGUID); err != nil || !meets {
				buf.WriteU32(uint32(i+8) << 24)
				continue
			}
			buf.WriteU32(spells[i] | (uint32(i+8) << 24))
		} else if i < 8 {
			buf.WriteU32(uint32(i+8) << 24)
		} else {
			buf.WriteU32(0)
		}
	}

	buf.WriteU8(0) // extra spells count
	// Player.cpp:21222 writes vehicle->GetSpellHistory()->WritePacket<Pet>
	// here: the live cooldown count + per-entry (spell, category,
	// cooldown, category cooldown) in SpellHistory.cpp:203-237's shape
	// — the same tail the pet twin (sendPetSpells, pets.go) writes from
	// the motion's cooldown maps. The vehicle base is a creature motion,
	// so its history lives in SpellCooldowns / SpellCooldownCategories /
	// SpellCooldownCategoryEnds, pruned first like every creature
	// cooldown read. No live motion (or no player position) reads as
	// zero cooldowns, the old hardcoded shape.
	type vehicleCooldown struct {
		spell, category  uint32
		end, categoryEnd time.Time
	}
	cooldowns := make([]vehicleCooldown, 0)
	if s.player != nil && s.server != nil {
		now := time.Now()
		s.server.motionMu.Lock()
		if motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, vehicleGUID); motion != nil {
			s.server.pruneCreatureSpellCooldowns(motion, now)
			for spellID, end := range motion.SpellCooldowns {
				categoryID := motion.SpellCooldownCategories[spellID]
				cooldowns = append(cooldowns, vehicleCooldown{spell: spellID, category: categoryID, end: end, categoryEnd: motion.SpellCooldownCategoryEnds[spellID]})
			}
		}
		s.server.motionMu.Unlock()
		sort.Slice(cooldowns, func(i, j int) bool { return cooldowns[i].spell < cooldowns[j].spell })
		buf.WriteU8(uint8(len(cooldowns)))
		for _, cooldown := range cooldowns {
			buf.WriteU32(cooldown.spell)
			buf.WriteU16(uint16(cooldown.category))
			cooldownMs := petCooldownMilliseconds(cooldown.end, now)
			categoryMs := petCooldownMilliseconds(cooldown.categoryEnd, now)
			if cooldownMs == 0 {
				buf.WriteU32(0)
				buf.WriteU32(0)
			} else if categoryMs > 0 {
				buf.WriteU32(0)
				buf.WriteU32(categoryMs)
			} else {
				buf.WriteU32(cooldownMs)
				buf.WriteU32(0)
			}
		}
	} else {
		buf.WriteU8(0) // cooldowns count
	}

	_ = s.write(uint16(protocol.OpcodeSMSG_PET_SPELLS), buf.Bytes(), true)
}

const (
	charmActCommand  uint8 = 0x07 // ACT_COMMAND (Unit.h: ActiveStates)
	charmActReaction uint8 = 0x06 // ACT_REACTION
	charmActDisabled uint8 = 0x81 // ACT_DISABLED
	charmActEnabled  uint8 = 0xC1 // ACT_ENABLED
	charmActPassive  uint8 = 0x01 // ACT_PASSIVE
)

// buildCharmActionBar is the CharmInfo::InitPetActionBar +
// InitCharmCreateSpells fold shared by sendCharmPetSpells and the
// charmed-creature set-action seed: the default command/reaction bar with
// the creature's own spells (MAX_SPELL_CHARM = 4) folded into the middle
// spell slots via AddSpellToActionBar (Unit.cpp:10053). It returns the bar
// and the _charmspells addlist (Player.cpp:21263-21271).
func buildCharmActionBar(spells []uint32, spellFn func(uint32) (wotlk.Spell, bool)) ([10]uint32, []uint32) {
	bar := [10]uint32{0x07000002, 0x07000001, 0x07000000, 0x01000000, 0x01000000, 0x01000000, 0x01000000, 0x06000002, 0x06000001, 0x06000000}
	var charmSpells []uint32
	n := len(spells)
	if n > 4 {
		n = 4
	}
	for x := 0; x < n; x++ {
		spellID := spells[x]
		if spellID == 0 {
			continue
		}
		spell, found := spellFn(spellID)
		if !found {
			// No DBC row: CharmInfo::InitCharmCreateSpells still records the
			// charm spell as ACT_DISABLED and skips the action bar.
			charmSpells = append(charmSpells, spellID|(uint32(charmActDisabled)<<24))
			continue
		}
		if spell.Attributes&spellAttributePassive != 0 {
			charmSpells = append(charmSpells, spellID|(uint32(charmActPassive)<<24))
			continue
		}
		charmSpells = append(charmSpells, spellID|(uint32(charmActDisabled)<<24))
		newState := charmActPassive
		if spell.AttributesEx&spellAttr1UnautocastableByPet == 0 { // SpellInfo::IsAutocastable (SpellInfo.cpp:1101)
			if spellNeedsExplicitUnitTarget(spell) {
				newState = charmActEnabled
			} else {
				newState = charmActDisabled
			}
		}
		// AddSpellToActionBar: first empty spell-type slot (the middle four;
		// IsActionBarForSpell admits ACT_DISABLED/ENABLED/PASSIVE, Unit.h:594).
		for j := 3; j < 7; j++ {
			if bar[j]&0x00FFFFFF == 0 {
				bar[j] = spellID | (uint32(newState) << 24)
				break
			}
		}
	}
	return bar, charmSpells
}

// buildPossessActionBar is the CharmInfo::InitEmptyActionBar +
// InitPossessCreateSpells fold (Unit.cpp:9967-10001): the attack command
// at slot 0, then the creature's own spells (MAX_CREATURE_SPELLS = 8) as
// ACT_PASSIVE — AddSpellToActionBar skips the COMMAND slot 0 and fills
// the first empty spell-type slot, so spell i lands at slot i+1.
func buildPossessActionBar(spells []uint32) [10]uint32 {
	bar := [10]uint32{0x07000002, 0x01000000, 0x01000000, 0x01000000, 0x01000000, 0x01000000, 0x01000000, 0x01000000, 0x01000000, 0x01000000}
	n := len(spells)
	if n > 8 {
		n = 8
	}
	for x := 0; x < n; x++ {
		if spells[x] == 0 {
			continue
		}
		bar[1+x] = spells[x] | (uint32(charmActPassive) << 24)
	}
	return bar
}

// charmedActionBarSeed returns the bar a charm/possess re-send would
// carry for a wild creature: a player-rearranged bar when one is stored,
// else the freshly built bar for the creature's current charm type.
func (s *Server) charmedActionBarSeed(motion *creatureMotion) [10]uint32 {
	if motion == nil {
		return [10]uint32{}
	}
	if motion.CharmActionBarSet {
		return motion.CharmActionBar
	}
	if motion.Possessed {
		return buildPossessActionBar(motion.Spells)
	}
	bar, _ := buildCharmActionBar(motion.Spells, func(id uint32) (wotlk.Spell, bool) {
		if s == nil || s.Data == nil {
			return wotlk.Spell{}, false
		}
		spell, found, err := s.Data.Spell(id)
		if err != nil {
			return wotlk.Spell{}, false
		}
		return spell, found
	})
	return bar
}

// setCharmedActionBarSpellState mirrors CharmInfo::SetSpellAutocast
// (Unit.cpp:10160): the bar slot carrying the spell flips to
// ACT_ENABLED/ACT_DISABLED. A toggle with no stored bar materializes
// the override from the current built bar, because C++ mutates live
// CharmInfo state — the next charm/possess re-send carries the flip
// even when the player never dragged a button.
func (s *Server) setCharmedActionBarSpellState(mapID, instanceID uint32, guid uint64, spellID uint32, enabled bool) {
	if s == nil || guid == 0 || spellID == 0 {
		return
	}
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	motion := s.findCreatureMotionLocked(mapID, instanceID, guid)
	if motion == nil || !motion.Charmed || motionIsPet(motion) {
		return
	}
	bar := s.charmedActionBarSeed(motion)
	newType := uint32(charmActDisabled)
	if enabled {
		newType = uint32(charmActEnabled)
	}
	flipped := false
	for i := 0; i < 10 && !flipped; i++ {
		// IsActionBarForSpell (Unit.h:594) admits ACT_DISABLED/ENABLED/
		// PASSIVE; a command/reaction slot carrying the same action id
		// is not a spell button and must not flip.
		if bar[i]&0x00FFFFFF != spellID {
			continue
		}
		switch bar[i] >> 24 {
		case uint32(charmActEnabled), uint32(charmActDisabled), uint32(charmActPassive):
			bar[i] = spellID | (newType << 24)
			flipped = true
		}
	}
	motion.CharmActionBar = bar
	motion.CharmActionBarSet = true
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
	// Player::CharmSpellInitialize (Player.cpp:21226-21277): the bar starts as
	// the default pet command bar (CharmInfo::InitPetActionBar, Unit.cpp:9942 —
	// attack/follow/stay as ACT_COMMAND, four empty spell slots, three
	// reactions), then CharmInfo::InitCharmCreateSpells (Unit.cpp:10003) folds
	// the creature's own spells (MAX_SPELL_CHARM = 4) into the middle spell
	// slots via AddSpellToActionBar (Unit.cpp:10053).
	bar, charmSpells := buildCharmActionBar(spells, func(id uint32) (wotlk.Spell, bool) {
		if s.server == nil || s.server.Data == nil {
			return wotlk.Spell{}, false
		}
		spell, found, err := s.server.Data.Spell(id)
		if err != nil {
			return wotlk.Spell{}, false
		}
		return spell, found
	})
	// A player-rearranged bar (HandlePetSetAction SetActionBar on the
	// CharmInfo) replaces the rebuilt bar wholesale; the addlist above
	// still derives from the creature's spells, as C++ keeps _charmspells
	// apart from the action bar.
	if m := s.findCreatureMotion(creatureGUID); m != nil && m.CharmActionBarSet {
		bar = m.CharmActionBar
	}
	for i := 0; i < 10; i++ {
		buf.WriteU32(bar[i])
	}
	// The addlist: charm spells carrying an action, packed
	// (Player.cpp:21263-21271).
	buf.WriteU8(uint8(len(charmSpells)))
	for _, packed := range charmSpells {
		buf.WriteU32(packed)
	}
	buf.WriteU8(0) // cooldowns count
	_ = s.write(uint16(protocol.OpcodeSMSG_PET_SPELLS), buf.Bytes(), true)
}

// sendCharmPetActionBar sends the charm bar for a PET target: like the
// possess arm, C++ Unit::SetCharmedBy skips InitCharmCreateSpells for pets,
// so Player::CharmSpellInitialize (Player.cpp:21226) sends the pet's stored
// action bar via CharmInfo::BuildActionBar. The _charmspells addlist stays
// zero for a pet (the ctor banks all four entries action-less and only
// InitCharmCreateSpells fills them), so the packet carries the stored bar
// plus zero addlist and cooldown counts under the live react/command bytes.
func (s *session) sendCharmPetActionBar(ctx context.Context, creatureGUID uint64, petID uint32, reactState, commandState uint8) {
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
	bar := s.petActionBarSlots(ctx, petID)
	for i := 0; i < 10; i++ {
		buf.WriteU32(bar[i])
	}
	buf.WriteU8(0) // spells count
	buf.WriteU8(0) // cooldowns count
	_ = s.write(uint16(protocol.OpcodeSMSG_PET_SPELLS), buf.Bytes(), true)
}

// sendPossessPetSpells generates SMSG_PET_SPELLS (0x179) with the possess
// action bar — Player::PossessSpellInitialize (Player.cpp:21147-21171):
// GUID, u16 0, u32 0, u32 0 (react/command/disable all zero, unlike the
// charm bar's live react/command bytes), then CharmInfo::BuildActionBar,
// then zero spell and cooldown counts (no addlist, unlike charm).
// The bar is CharmInfo::InitEmptyActionBar plus InitPossessCreateSpells
// (Unit.cpp:9967-10001): the attack command at slot 0, then the
// creature's own spells (MAX_CREATURE_SPELLS = 8) as ACT_PASSIVE —
// AddSpellToActionBar skips the COMMAND slot 0 and fills the first empty
// spell-type slot, so spell i lands at slot i+1. The five
// no-InitEmptyActionBar entries (23575/24783/27664/40281/28511) keep an
// attack-free bar in C++; Go keeps the attack slot for all entries, the
// same simplification the charm bar already makes.
// sendPossessPetActionBar sends the possess bar for a PET target: C++
// Unit::SetCharmedBy (Unit.cpp:11862-11868) skips InitPossessCreateSpells
// for pets (their CharmInfo already exists), so Player::PossessSpellInitialize
// (Player.cpp:21147) sends the pet's stored action bar via
// CharmInfo::BuildActionBar — the same slots PetSpellInitialize sends —
// under the possess header (zero family/duration/react/command) with zero
// spell and cooldown counts (no addlist, unlike PetSpellInitialize). The
// spell-list sendPossessPetSpells below is the non-pet arm only: it would
// rebuild a pet's bar from creature_template_spell order at slots 1..8,
// wiping the follow/stay commands, the reactions, the abdata layout and
// the per-spell active states.
func (s *session) sendPossessPetActionBar(ctx context.Context, creatureGUID uint64, petID uint32) {
	if s == nil || s.conn == nil || creatureGUID == 0 {
		return
	}
	buf := protocol.NewBuffer(64)
	buf.WriteU64(creatureGUID)
	buf.WriteU16(0)
	buf.WriteU32(0)
	buf.WriteU32(0)
	bar := s.petActionBarSlots(ctx, petID)
	for i := 0; i < 10; i++ {
		buf.WriteU32(bar[i])
	}
	buf.WriteU8(0) // spells count
	buf.WriteU8(0) // cooldowns count
	_ = s.write(uint16(protocol.OpcodeSMSG_PET_SPELLS), buf.Bytes(), true)
}

func (s *session) sendPossessPetSpells(creatureGUID uint64, spells []uint32) {
	if s == nil || s.conn == nil || creatureGUID == 0 {
		return
	}
	buf := protocol.NewBuffer(64)
	buf.WriteU64(creatureGUID)
	buf.WriteU16(0)
	buf.WriteU32(0)
	buf.WriteU32(0)
	bar := buildPossessActionBar(spells)
	// A player-rearranged bar replaces the rebuilt bar wholesale, as in
	// the charm send above.
	if m := s.findCreatureMotion(creatureGUID); m != nil && m.CharmActionBarSet {
		bar = m.CharmActionBar
	}
	for i := 0; i < 10; i++ {
		buf.WriteU32(bar[i])
	}
	buf.WriteU8(0) // spells count
	buf.WriteU8(0) // cooldowns count
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
	if wsgMap := s.server.carriedWSGMap(s.playerGUID); wsgMap != 0 {
		s.server.handleWSGPlayerLeave(s, wsgMap)
	} else if s.player.Map == WSGMapID {
		s.server.handleWSGPlayerLeave(s, WSGMapID)
	}
	if eotsMap := s.server.carriedEOTSMap(s.playerGUID); eotsMap != 0 {
		s.server.handleEOTSPlayerLeave(s, eotsMap)
	} else if s.player.Map == EOTSMapID {
		s.server.handleEOTSPlayerLeave(s, EOTSMapID)
	}
}

// sendPassengerClearTarget ports Unit::SendClearTarget
// (Unit.cpp:13574-13579) for a boarded passenger
// (VehicleJoinEvent::Execute, Vehicle.cpp:883): SMSG_BREAK_TARGET with
// the passenger's packed GUID, broadcast to every session in range on
// the passenger's map/instance — SendMessageToSet, not the hostile-only
// ForceDeselect fan-out. A 0 phase reads as PHASEMASK_NORMAL.
func (s *Server) sendPassengerClearTarget(mapID, instanceID uint32, passengerGUID uint64, x, y, z float32, phase uint32) {
	if s == nil || passengerGUID == 0 {
		return
	}
	if phase == 0 {
		phase = 1
	}
	breakPkt := protocol.NewBuffer(9)
	breakPkt.WritePackedGUID(passengerGUID)
	s.sessionsMu.RLock()
	targets := make([]*session, 0, len(s.sessions))
	for target := range s.sessions {
		if target == nil || !target.authed || !target.worldReady.Load() || target.player == nil ||
			target.player.Map != mapID || target.player.InstanceID != instanceID {
			continue
		}
		if distance3D(target.player.X, target.player.Y, target.player.Z, x, y, z) > 100.0 {
			continue
		}
		targets = append(targets, target)
	}
	s.sessionsMu.RUnlock()
	for _, target := range targets {
		if phase&target.currentPlayerPhaseMask() == 0 {
			continue
		}
		_ = target.write(uint16(protocol.OpcodeSMSG_BREAK_TARGET), breakPkt.Bytes(), true)
	}
}

// sendCreatureSplineMoveRoot broadcasts SMSG_SPLINE_MOVE_ROOT for a
// creature passenger — the Unit::SetRooted(true) sender (Unit.cpp:11649-
// 11680: a creature roots with SMSG_SPLINE_MOVE_ROOT, a player with
// SMSG_FORCE_MOVE_ROOT), the packet half of the
// VehicleJoinEvent::Execute board root (Vehicle.cpp:884).
func (s *Server) sendCreatureSplineMoveRoot(mapID, instanceID uint32, guid uint64, x, y, z float32, phase uint32) {
	if s == nil || guid == 0 {
		return
	}
	if phase == 0 {
		phase = 1
	}
	pkt := protocol.NewBuffer(9)
	pkt.WritePackedGUID(guid)
	s.broadcastToVisibleSetFromPhase(uint16(protocol.OpcodeSMSG_SPLINE_MOVE_ROOT), pkt.Bytes(), mapID, instanceID, x, y, z, phase)
}

// sendCreatureSplineMoveUnroot broadcasts SMSG_SPLINE_MOVE_UNROOT for a
// creature passenger — the Unit::SetRooted(false) sender (Unit.cpp:11649-
// 11680), the packet half of the Unit::_ExitVehicle unroot leg
// (Unit.cpp:13202-13210).
func (s *Server) sendCreatureSplineMoveUnroot(mapID, instanceID uint32, guid uint64, x, y, z float32, phase uint32) {
	if s == nil || guid == 0 {
		return
	}
	if phase == 0 {
		phase = 1
	}
	pkt := protocol.NewBuffer(9)
	pkt.WritePackedGUID(guid)
	s.broadcastToVisibleSetFromPhase(uint16(protocol.OpcodeSMSG_SPLINE_MOVE_UNROOT), pkt.Bytes(), mapID, instanceID, x, y, z, phase)
}

// interruptBoardingCreaturePassenger runs the VehicleJoinEvent::Execute
// interrupt legs (Vehicle.cpp:836-837) for a creature passenger that just
// seated: InterruptNonMeleeSpells(false) plus the SPELL_AURA_MOUNTED
// strip. The interrupt twin runs under motionMu (the SetStunned twin's
// contract in lua_creature_events.go); its packets go out after the
// unlock, as there.
func (s *Server) interruptBoardingCreaturePassenger(motion *creatureMotion) {
	if s == nil || motion == nil {
		return
	}
	s.motionMu.Lock()
	interrupted := s.interruptCreatureNonMeleeCasts(motion)
	s.motionMu.Unlock()
	for _, arm := range interrupted {
		s.sendCreatureInterruptPackets(motion, arm)
	}
	s.removeCreatureAurasByType(creatureAuraKeyForMotion(motion), spellAuraMounted)
}

// stampCreatureVehicleTransport runs the VehicleJoinEvent::Execute
// transport stamp (Vehicle.cpp:865-869) for a creature passenger that
// just seated: MOVEMENTFLAG_ONTRANSPORT with the seat attachment
// offsets, the seat addon's orientation offset, the assigned seat, and
// the base GUID. The player twin is the enterVehicle stamp; a creature
// board without it leaves TransportGUID 0 while VehicleGUID is set, so
// the movement packets and the exit inherit leg cannot tell the
// passenger rides anything. Caller holds motionMu.
func stampCreatureVehicleTransport(motion *creatureMotion, baseGUID uint64, seat int8, seatInfo *wotlk.VehicleSeatEntry, addon *VehicleSeatAddon) {
	if motion == nil || seatInfo == nil {
		return
	}
	o := float32(0)
	if addon != nil {
		o = addon.SeatOrientationOffset
	}
	motion.TransportGUID = baseGUID
	motion.TransportX, motion.TransportY, motion.TransportZ = seatInfo.AttachmentOffsetX, seatInfo.AttachmentOffsetY, seatInfo.AttachmentOffsetZ
	motion.TransportO = o
	motion.TransportSeat = seat
}

// inheritOrResetCreatureVehicleTransport runs the Vehicle::RemovePassenger
// transport leg (Vehicle.cpp:518-529) for a creature passenger leaving a
// vehicle: a base that itself rides a transport hands the passenger the
// base's transport (GUID + offsets + seat), otherwise the passenger's
// transport resets. The base's transport is recoverable only when it was
// stamped at board (base.VehicleGUID != 0): a base riding a real
// transport carries no offsets on its motion, so the passenger resets
// instead of keeping the exited vehicle's stale stamp — a stale GUID
// would park the creature behind the stepCreatureMotion transport gate
// forever. Runs after the seat frees, before the OnRemovePassenger hooks,
// as in C++.
func (s *Server) inheritOrResetCreatureVehicleTransport(mapID, instanceID uint32, motion *creatureMotion, baseGUID uint64) {
	if s == nil || motion == nil {
		return
	}
	var inheritGUID uint64
	var ix, iy, iz, io float32
	var iseat int8
	if baseGUID != 0 {
		if base := s.findCreatureMotion(mapID, instanceID, baseGUID); base != nil {
			s.motionMu.Lock()
			if base.TransportGUID != 0 && base.VehicleGUID != 0 {
				inheritGUID, ix, iy, iz, io, iseat = base.TransportGUID, base.TransportX, base.TransportY, base.TransportZ, base.TransportO, base.TransportSeat
			}
			s.motionMu.Unlock()
		}
	}
	s.motionMu.Lock()
	if inheritGUID != 0 {
		motion.TransportGUID, motion.TransportX, motion.TransportY, motion.TransportZ, motion.TransportO, motion.TransportSeat = inheritGUID, ix, iy, iz, io, iseat
	} else {
		motion.TransportGUID, motion.TransportX, motion.TransportY, motion.TransportZ, motion.TransportO, motion.TransportSeat = 0, 0, 0, 0, 0, 0
	}
	s.motionMu.Unlock()
}

// rootAndSplineBoardingCreaturePassenger runs the VehicleJoinEvent::Execute
// root and board-spline legs (Vehicle.cpp:884, :887-892) for a creature
// passenger that just seated, after the clear-target (:883) and before the
// threat transfer (:895-896): SetControlled(true, UNIT_STATE_ROOT) —
// ScriptRooted plus SMSG_SPLINE_MOVE_ROOT (Unit::SetRooted,
// Unit.cpp:11649-11680) — then the MoveTo spline to the seat-offset
// position with the transport-enter flag, SMSG_MONSTER_MOVE_TRANSPORT
// via the base GUID as the packet's transport arg. The server snaps the
// passenger to the seat position at launch (the atomic-spline model, as
// in the home leg); the following relocate's MonsterMoveStop is the
// spline's terminator. A base whose position cannot be resolved skips
// the spline but still roots: the root has no position dependency.
func (s *Server) rootAndSplineBoardingCreaturePassenger(motion *creatureMotion, baseGUID uint64, seatInfo *wotlk.VehicleSeatEntry, addon *VehicleSeatAddon, baseX, baseY, baseZ, baseO float32, hasBase bool) {
	if s == nil || motion == nil {
		return
	}
	s.motionMu.Lock()
	motion.ScriptRooted = true
	mapID, instanceID := motion.Map, motion.InstanceID
	startX, startY, startZ, phase := motion.X, motion.Y, motion.Z, motion.PhaseMask
	s.motionMu.Unlock()
	s.sendCreatureSplineMoveRoot(mapID, instanceID, motion.GUID, startX, startY, startZ, phase)
	if !hasBase || seatInfo == nil {
		return
	}
	attO := float32(0)
	if addon != nil {
		attO = addon.SeatOrientationOffset
	}
	destX, destY, destZ, destO := CalculatePassengerPosition(baseX, baseY, baseZ, baseO, seatInfo.AttachmentOffsetX, seatInfo.AttachmentOffsetY, seatInfo.AttachmentOffsetZ, attO)
	if speed := creatureSplineVelocityExplicit(motion, false); speed >= 0.001 {
		duration := splineDurationMs(float64(distance3D(startX, startY, startZ, destX, destY, destZ)), speed)
		s.broadcastMonsterMoveInInstance(mapID, instanceID, motion.GUID, startX, startY, startZ, destX, destY, destZ, duration, false, destO, true, phase, baseGUID)
	}
	s.motionMu.Lock()
	motion.X, motion.Y, motion.Z, motion.Orientation = destX, destY, destZ, destO
	s.motionMu.Unlock()
}

// unrootExitingCreaturePassenger runs the Unit::_ExitVehicle unroot leg
// (Unit.cpp:13202-13210) for a creature passenger leaving a vehicle:
// SetControlled(false, UNIT_STATE_ROOT) clears the board root unless a
// SPELL_AURA_MOD_ROOT aura holds it or the movement template roots the
// creature (Unit.cpp:11532-11535), and the exit then sends
// SMSG_SPLINE_MOVE_UNROOT whenever the root flag survives SetRooted —
// SetRooted(false) sends it and clears the flag for a non-stunned
// creature (Unit.cpp:11649-11680), while a stunned creature keeps the
// flag and the _ExitVehicle arm sends it instead, so exactly one packet
// goes out either way. The board leg
// (rootAndSplineBoardingCreaturePassenger, Vehicle.cpp:884) is its pair;
// a passenger that never took the board root clears nothing and sends
// nothing. Callers run it after the seat frees, as _ExitVehicle does.
func (s *Server) unrootExitingCreaturePassenger(motion *creatureMotion) {
	if s == nil || motion == nil {
		return
	}
	s.motionMu.Lock()
	wasRooted := motion.ScriptRooted
	moveRooted := motion.MoveRooted
	mapID, instanceID := motion.Map, motion.InstanceID
	x, y, z, phase := motion.X, motion.Y, motion.Z, motion.PhaseMask
	key := creatureAuraKeyForMotion(motion)
	s.motionMu.Unlock()
	if !wasRooted || moveRooted {
		return
	}
	if s.creatureHasAuraType(key, spellAuraModRoot) {
		return
	}
	s.motionMu.Lock()
	motion.ScriptRooted = false
	s.motionMu.Unlock()
	s.sendCreatureSplineMoveUnroot(mapID, instanceID, motion.GUID, x, y, z, phase)
}

// removeVehicleControlAuraFromBase strips SPELL_AURA_CONTROL_VEHICLE
// auras cast by the passenger from a vehicle base — the
// VehicleJoinEvent::Abort leg (Vehicle.cpp:937) for a failed board. A
// player base holds the aura on its session, a creature base on its
// aura key.
func (s *Server) removeVehicleControlAuraFromBase(mapID, instanceID uint32, baseGUID, casterGUID uint64) {
	if s == nil || baseGUID == 0 {
		return
	}
	if bs := s.findSessionByGUID(baseGUID); bs != nil && bs.player != nil {
		bs.removeAurasByTypeFromCaster(spellAuraControlVehicle, casterGUID)
		return
	}
	s.removeCreatureAurasByTypeFromCaster(creatureAuraKey{Map: mapID, InstanceID: instanceID, GUID: baseGUID}, spellAuraControlVehicle, casterGUID)
}

// removeVehicleControlAuraFromBaseExcept strips SPELL_AURA_CONTROL_VEHICLE
// auras cast by the passenger from a vehicle base except the named spell —
// the Unit::_EnterVehicle seat-change arm (Unit.cpp:13100), which clears
// the rider's older ride auras when a new one boards, sparing the aura
// being applied. A player base holds the aura on its session, a creature
// base on its aura key.
func (s *Server) removeVehicleControlAuraFromBaseExcept(mapID, instanceID uint32, baseGUID, casterGUID uint64, exceptSpellID uint32) {
	if s == nil || baseGUID == 0 {
		return
	}
	if bs := s.findSessionByGUID(baseGUID); bs != nil && bs.player != nil {
		bs.removeAurasByTypeFromCasterExcept(spellAuraControlVehicle, casterGUID, exceptSpellID)
		return
	}
	s.removeCreatureAurasByTypeFromCasterExcept(creatureAuraKey{Map: mapID, InstanceID: instanceID, GUID: baseGUID}, spellAuraControlVehicle, casterGUID, exceptSpellID)
}

// stopPlayerCharmOnVehicleBoard mirrors the Player::StopCastingCharm
// leg of VehicleJoinEvent::Execute (Vehicle.cpp:849;
// Player.cpp:20940-20967): boarding a vehicle releases every unit the
// player currently charms. Unit::RemoveCharmAuras (Unit.cpp:6525-6531)
// strips the charm auras from the charmed unit, and their remove legs
// uncharm it; a charm with no aura row left (debug/Lua charm) still
// uncharms via SetCharm(charm, false) (:20960-20966). The IsGhouled
// leg has no Go model (RemoveGhoul has no analog, death.go) and a
// puppet carries no per-motion marker, so it takes the uncharm path
// instead of UnSummon; a charmed player target has no Go charm model
// either (checkCharmCast). The charmed-vehicle ExitVehicle leg is a
// no-op here: the _EnterVehicle exit-first arm above already seated
// the player out of any previous vehicle. The 236 ride aura never
// sets CharmerGUID on the boarded base, so the scan cannot catch the
// base being boarded — only a separately mind-controlled unit.
func (s *session) stopPlayerCharmOnVehicleBoard(ctx context.Context) {
	if s == nil || s.player == nil || s.server == nil {
		return
	}
	s.server.motionMu.Lock()
	var charmed []creatureAuraKey
	for _, motion := range s.server.motionMapLocked(s.player.Map, s.player.InstanceID) {
		if motion != nil && motion.Charmed && motion.CharmerGUID == s.playerGUID {
			charmed = append(charmed, creatureAuraKey{Map: motion.Map, InstanceID: motion.InstanceID, GUID: motion.GUID})
		}
	}
	s.server.motionMu.Unlock()
	for _, key := range charmed {
		s.server.removeCreatureAurasByTypeFromCaster(key, spellAuraCharm, s.playerGUID)
		s.server.removeCreatureAurasByTypeFromCaster(key, spellAuraModPossessPet, s.playerGUID)
		s.server.removeCreatureAurasByTypeFromCaster(key, spellAuraModPossess, s.playerGUID)
		s.server.removeCreatureAurasByTypeFromCaster(key, spellAuraAoeCharm, s.playerGUID)
		if m := s.server.findCreatureMotion(key.Map, key.InstanceID, key.GUID); m != nil && m.Charmed && m.CharmerGUID == s.playerGUID {
			s.server.uncharmCreature(key, s.playerGUID)
		}
	}
}

// enterVehicle places the session's player into the specified vehicle and seat.
// Reference: Unit::_EnterVehicle (Unit.cpp:8320) & VehicleJoinEvent::Execute (Vehicle.cpp:790-910).
func (s *session) enterVehicle(ctx context.Context, vehicleGUID uint64, seatID int8) {
	if s == nil || s.player == nil {
		return
	}

	// Unit::_EnterVehicle (Unit.cpp:13095-13096): a same-vehicle
	// same-seat re-entry keeps the seat — AddPassenger returns the
	// current seat and no JoinEvent fires, so the Execute-only legs
	// below (charm/bind-sight stops) must not run for it. Captured
	// before the exit-first arm below can mutate the seat.
	sameSeatReentry := s.player.VehicleGUID == vehicleGUID && seatID >= 0 && seatID == s.player.VehicleSeat

	// VehicleJoinEvent::Execute (Vehicle.cpp:808-813): a passenger
	// that died before the board aborts instead of seating.
	if s.player.Health == 0 {
		return
	}

	// Unit::_EnterVehicle (Unit.cpp:13089): a passenger that IS the
	// vehicle base — its own kit is the target vehicle — or whose
	// vehicle the base already rides never boards. GetVehicleKit()
	// is null for a non-base, so both legs need the passenger's own
	// kit; vehicle->GetBase()->IsOnVehicle(this) (Unit.cpp:12074-12077)
	// is the base seated in that kit, of either kind.
	if s.server != nil {
		if ownKit := s.server.getVehicleKit(s.player.Map, s.player.InstanceID, s.playerGUID); ownKit != nil {
			if vehicleGUID == s.playerGUID {
				return
			}
			if motion := s.server.findCreatureMotion(s.player.Map, s.player.InstanceID, vehicleGUID); motion != nil && motion.VehicleGUID == s.playerGUID {
				return
			} else if baseSess := s.server.findSessionByGUID(vehicleGUID); baseSess != nil && baseSess.player != nil && baseSess.player.VehicleGUID == s.playerGUID {
				return
			}
		}
	}

	// Arena check: players cannot board player vehicles in arena
	if s.server != nil && IsArenaMap(s.player.Map) {
		if targetSess := s.server.findSessionByGUID(vehicleGUID); targetSess != nil {
			return
		}
	}

	// Unit::_EnterVehicle (Unit.cpp:13088-13101): boarding a different
	// vehicle exits the current one first through ExitVehicle() — the
	// ride aura strips from the old base and its remove leg runs
	// _ExitVehicle — instead of unseating directly, which would leave
	// the stale ride aura on the old base and the dummy aura on the
	// caster. A seat with no ride aura (Lua/debug board) keeps the
	// direct exit as its fallback. A same-vehicle re-entry on a
	// different seat exits first too, but its :13100 strip already ran
	// in the aura-apply callers (spells.go, except the aura being
	// applied), so the branch below only fires for aura-less seats and
	// keeps the direct exit. Same-seat re-entry keeps the seat
	// (Unit.cpp:13095-13096).
	if s.player.VehicleGUID != 0 && s.player.VehicleGUID != vehicleGUID {
		oldVehGUID := s.player.VehicleGUID
		if s.server != nil {
			s.server.removeVehicleControlAuraFromBase(s.player.Map, s.player.InstanceID, oldVehGUID, s.playerGUID)
		}
		if s.player.VehicleGUID != 0 {
			s.exitVehicle(ctx)
		}
	} else if s.player.VehicleGUID == vehicleGUID && seatID >= 0 && seatID != s.player.VehicleSeat {
		s.exitVehicle(ctx)
	}

	// Unit::_EnterVehicle (Unit.cpp:13103-13107): a player boarding a
	// player-base vehicle while in combat never boards — the ride aura is
	// stripped from the base and the board aborts. Same-seat re-entry
	// returns before the gate (Unit.cpp:13095-13096), so the gate only
	// runs when the board would actually move the passenger; a
	// different-vehicle or different-seat board already exited above,
	// matching the C++ order where the exit precedes the gate.
	if !(s.player.VehicleGUID == vehicleGUID && seatID >= 0 && seatID == s.player.VehicleSeat) {
		if s.server != nil {
			if kit := s.server.getVehicleKit(s.player.Map, s.player.InstanceID, vehicleGUID); kit != nil && kit.IsPlayer && s.isInCombat() {
				s.server.removeVehicleControlAuraFromBase(s.player.Map, s.player.InstanceID, vehicleGUID, s.playerGUID)
				return
			}
		}
	}

	// Unit::_EnterVehicle (Unit.cpp:13095-13096): same vehicle, same seat
	// — the board is a no-op return before any JoinEvent is scheduled, so
	// none of the Execute legs run: no threat transfer (an additive
	// AddThreat — re-running it would stack the passenger's threat onto
	// the base again), no OnAddPassenger hook re-fire, no cast interrupt,
	// no pet unsummon, no cancel packet. AddPassenger below would no-op
	// into the current seat and the tail would re-run the whole JoinEvent.
	if sameSeatReentry {
		return
	}

	actualSeat := seatID
	playerVehicleID := uint32(0)
	var seatInfo *wotlk.VehicleSeatEntry
	var seatAddon *VehicleSeatAddon
	if s.server != nil {
		if kit := s.server.getVehicleKit(s.player.Map, s.player.InstanceID, vehicleGUID); kit != nil {
			// Unit::_EnterVehicle (Unit.cpp:13109-13115): a player
			// boarding a creature-base vehicle that is part of a
			// formation removes the base from its formation before
			// AddPassenger; a player base (mount kit) never carries
			// one. Runs before the seat eject below, as in C++.
			if !kit.IsPlayer {
				if base := s.server.findCreatureMotion(s.player.Map, s.player.InstanceID, vehicleGUID); base != nil {
					s.server.removeFormationMember(ctx, base)
				}
			}
			// Vehicle::AddPassenger (Vehicle.cpp:471-478): boarding a
			// specific occupied seat ejects its current occupant first,
			// of either kind — a creature occupant blocks the board
			// outright without the eject, the same leg the Lua
			// AddPassenger and debug boards ride. Skipped on a
			// same-vehicle re-entry: the board no-ops into the
			// current seat, so there is no seat to take.
			if seatID >= 0 && s.player.VehicleGUID != vehicleGUID {
				s.server.ejectVehicleSeatOccupant(ctx, s.player.Map, s.player.InstanceID, kit, vehicleGUID, seatID, s.playerGUID)
			}
			wasUnselectable := s.player.UnitFlags&0x02000000 != 0 // UNIT_FLAG_NOT_SELECTABLE
			assigned, si, ok := kit.AddPassenger(s.playerGUID, seatID, wasUnselectable)
			if !ok {
				// VehicleJoinEvent::Abort (Vehicle.cpp:937): a failed
				// board strips the control-vehicle aura from the base.
				s.server.removeVehicleControlAuraFromBase(s.player.Map, s.player.InstanceID, vehicleGUID, s.playerGUID)
				return
			}
			if !sameSeatReentry {
				// VehicleJoinEvent::Execute (Vehicle.cpp:837, :846-848):
				// the board strips the passenger's mounted aura and
				// drops a carried battleground flag — but only once
				// the board succeeds. AddPassenger schedules the
				// JoinEvent; a failed board aborts it instead (Abort
				// strips only the control aura), and a same-seat
				// re-entry never schedules one (Unit.cpp:13095-13096).
				// Dropping before the board lost the flag — and the
				// mount display — on a failed board.
				s.player.MountDisplayID = 0
				s.dropBattlegroundFlagIfCarried()
			}
			actualSeat = assigned
			seatInfo = si
			_, _, seatAddon = kit.GetSeatForPassenger(s.playerGUID)
			if kit.IsPlayer {
				playerVehicleID = kit.VehicleID
			}
			if seatInfo != nil {
				// VehicleJoinEvent::Execute (Vehicle.cpp:871-876): the
				// board charms only on a creature base — a player
				// kit never charms its passenger.
				if seatInfo.CanControl() && !kit.IsPlayer {
					s.sendClientControl(vehicleGUID, true)
					spells := s.server.loadCreatureSpells(context.Background(), kit.CreatureEntry)
					s.sendVehiclePetSpells(ctx, vehicleGUID, kit.CreatureEntry, spells)
				}
				if seatInfo.HasFlag(wotlk.VehicleSeatFlagPassengerNotSelectable) {
					s.player.UnitFlags |= 0x02000000 // UNIT_FLAG_NOT_SELECTABLE
				}
			}
			// VehicleJoinEvent::Execute (Vehicle.cpp:895-896): every
			// creature the passenger threatens gains the vehicle base
			// on its threat list with the passenger's threat value,
			// ahead of the AI PassengerBoarded / OnAddPassenger hooks
			// below. Runs for a player base too — C++ transfers to
			// Target->GetBase() regardless of its type.
			// VehicleJoinEvent::Execute (Vehicle.cpp:883): the board
			// breaks the passenger's target for everyone in range.
			s.server.sendPassengerClearTarget(s.player.Map, s.player.InstanceID, s.playerGUID, s.player.X, s.player.Y, s.player.Z, s.currentPlayerPhaseMask())
			s.server.transferPassengerThreatToVehicleBase(s.player.Map, s.player.InstanceID, s.playerGUID, vehicleGUID)
			// Vehicle::AddPassenger (Vehicle.cpp:899-902) fires
			// OnAddPassenger for creature bases only; player bases
			// (mount kits) stay silent per the C++ TYPEID_UNIT gate.
			if !kit.IsPlayer {
				if base := s.server.findCreatureMotion(s.player.Map, s.player.InstanceID, vehicleGUID); base != nil {
					// VehicleJoinEvent::Execute (Vehicle.cpp:900-902):
					// the AI PassengerBoarded(true) call precedes
					// the OnAddPassenger script hook.
					s.server.smartPassengerBoarded(ctx, base, s.playerGUID, assigned, true)
					s.server.triggerVehicleEvent(scripting.VehicleEventOnAddPassenger, s.server.luaMotionCreature(base), s.luaPlayer(), assigned)
				}
			}
			// VehicleJoinEvent::Execute (Vehicle.cpp:825-832): boarding
			// the last usable seat clears the base's spell-click flag;
			// the helper no-ops for a player base (no creature motion,
			// no UNIT_NPC_FLAGS field in Go).
			s.server.syncVehicleBaseNPCFlag(s.player.Map, s.player.InstanceID, vehicleGUID)
		}
	}

	// VehicleAI::OnCharmed (CombatAI.cpp:295-306), charmed branch: a
	// vehicle in use again is no longer marked for dismiss, so a
	// re-boarding passenger clears the timer armed by exitVehicle.
	if s.server != nil {
		if motion := s.server.findCreatureMotion(s.player.Map, s.player.InstanceID, vehicleGUID); motion != nil && s.server.isVehicleAI(motion) {
			motion.VehicleDismiss = false
			motion.VehicleDismissAt = time.Time{}
		}
	}

	// VehicleJoinEvent::Execute (Vehicle.cpp:836-837): boarding interrupts the
	// passenger's non-melee spells, and (Vehicle.cpp:837) strips mounted
	// auras; (Vehicle.cpp:849-850) the board stops the player's charm and
	// bind-sight — a mind-controlled unit is released, a farsight/possess
	// viewpoint is dropped — ahead of the pet unsummon below; the pet is
	// temporarily unsummoned unless the seat carries
	// VEHICLE_SEAT_FLAG_B_KEEP_PET (Vehicle.cpp:852-853).
	s.interruptCurrentCast()
	s.interruptCurrentChannel()
	s.clearOtherMountedAuras(0)
	if !sameSeatReentry {
		s.stopCastingBindSight()
		s.stopPlayerCharmOnVehicleBoard(ctx)
	}
	if seatInfo == nil || !seatInfo.HasFlag(wotlk.VehicleSeatFlagBKeepPet) {
		s.temporarilyUnsummonPet(ctx)
	}

	s.player.VehicleGUID = vehicleGUID
	s.player.VehicleSeat = actualSeat
	// VehicleJoinEvent::Execute (Vehicle.cpp:865-869): the board stamps
	// the passenger's movement info with the transport seat offsets
	// (time 0, the assigned seat, the base GUID). The create block
	// carries it (movementInfoForCreate, movement.go:1482-1493:
	// MOVEMENTFLAG_ONTRANSPORT plus the GUID, offsets and seat), and
	// the exit leg resets it (the inherit-or-reset arm in exitVehicle)
	// or the next client movement ack overwrites it
	// (movement.go:964-985).
	if seatInfo != nil {
		s.player.TransportGUID = vehicleGUID
		s.player.TransportX, s.player.TransportY, s.player.TransportZ = seatInfo.AttachmentOffsetX, seatInfo.AttachmentOffsetY, seatInfo.AttachmentOffsetZ
		s.player.TransportO = 0
		if seatAddon != nil {
			s.player.TransportO = seatAddon.SeatOrientationOffset
		}
		s.player.TransportSeat = actualSeat
	}
	// VehicleJoinEvent::Execute (Vehicle.cpp:881): the board roots the
	// passenger (SetControlled(true, UNIT_STATE_ROOT), SMSG_FORCE_ROOT).
	s.setRooted(true)
	if playerVehicleID != 0 {
		s.sendPlayerVehicleData(playerVehicleID)
	}
	s.sendCancelExpectedRideVehicleAura()
	s.sendPlayerMountUpdate()
	s.sendPlayerUpdate()
}

// exitVehicleTail ports the Unit::_ExitVehicle tail (Unit.cpp:13251-13266),
// shared by the player session exit (exitVehicle below) and the creature
// ExitVehicle arm (lua_creature_events.go): after the MoveTo spline the
// tail runs for BOTH passenger kinds — only the resummon is player-gated
// (Unit.cpp:13251-13252, already ported as resummonTemporaryPet), so the
// minion and accessory legs below take the passenger GUID, not a session.
func (s *Server) exitVehicleTail(ctx context.Context, mapID, instanceID uint32, baseGUID, passengerGUID uint64) {
	if s == nil || baseGUID == 0 || passengerGUID == 0 {
		return
	}
	base := s.findCreatureMotion(mapID, instanceID, baseGUID)
	// Unit::_ExitVehicle (Unit.cpp:13254-13256): a minion vehicle base
	// owned by the exiting unit despawns 1ms after the exit. IsMinion is
	// the UNIT_MASK_MINION bit (creaturemotion.go); OwnerGUID is the
	// Minion::GetOwner twin — summonTempCreature stamps the summoner GUID
	// (lua_creature_events.go) — and the TYPEID_UNIT gate rides the
	// creature-motion lookup: a player base resolves to no motion and
	// skips the leg, as in C++. DespawnOrUnsummon(1ms)
	// (Creature.cpp:2271-2276) runs UnSummon on a TempSummon and
	// ForcedDespawn otherwise; both end at despawnCreatureMotion
	// (lua_creature_events.go), so the 1ms delay rides time.AfterFunc
	// like creature:Despawn(ms).
	if base != nil && base.IsMinion && base.OwnerGUID == passengerGUID {
		bmap, binst, bguid := base.Map, base.InstanceID, base.GUID
		time.AfterFunc(time.Millisecond, func() {
			s.despawnCreatureMotion(context.Background(), bmap, binst, bguid)
		})
	}
	// Unit::_ExitVehicle (Unit.cpp:13258-13266): an accessory passenger
	// dies with a just-died base, else unsummons after 2000ms. The
	// UNIT_MASK_ACCESSORY bit is the passenger motion's
	// VehicleAccessoryMinion (Vehicle.cpp:404 — InstallAccessory sets it
	// for minion accessories only), so a player passenger never carries
	// it and this leg is a no-op on the player path by construction.
	passenger := s.findCreatureMotion(mapID, instanceID, passengerGUID)
	if passenger == nil || !passenger.VehicleAccessoryMinion {
		return
	}
	// C++ reads the base's getDeathState() == JUST_DIED, but Go never
	// stores JUST_DIED — applyCreatureDeathState promotes it to CORPSE
	// at once (creaturemotion.go) — so the observable twin of a just-died
	// base is a dead base: Health 0 or a non-ALIVE DeathState.
	// A player base has no motion; its Health read is the same dead test.
	baseDead := false
	if base != nil {
		baseDead = base.Health == 0 || base.DeathState != creatureDeathAlive
	} else if baseSess := s.findSessionByGUID(baseGUID); baseSess != nil && baseSess.player != nil {
		baseDead = baseSess.player.Health == 0
	}
	if baseDead {
		// Unit.cpp:13261-13262: setDeathState(JUST_DIED) — the
		// applyCreatureDeathState twin, which runs the Creature
		// JUST_DIED arm and promotes to CORPSE.
		s.applyCreatureDeathState(ctx, passenger, creatureDeathJustDied)
	} else {
		// Unit.cpp:13265: ToTempSummon()->UnSummon(2000) — the
		// unsummonEjectedVehicleAccessory twin (2s delayed expiry),
		// shared with the evade-reinstall and respawn eject paths.
		s.unsummonEjectedVehicleAccessory(passenger, true)
	}
}

// ejectVehicleSeatOccupant ejects the occupant of one specific seat via
// the full Unit::_ExitVehicle twin (Unit.cpp:13191-13266), shared by
// every AddPassenger caller that boards a specific seat:
// Vehicle::AddPassenger (Vehicle.cpp:471-478) ejects the seat's current
// occupant first — passenger->ExitVehicle() — before the new passenger
// boards, for a passenger of EITHER kind. The kit allocator cannot do
// it (no server handle), so the eject lives here and the callers run it
// before kit.AddPassenger, the same order as C++ (eject at AddPassenger
// time, the VehicleJoinEvent boards after). A player occupant exits
// through the session exitVehicle (duel forfeit, control release, exit
// position, tail); a creature occupant exits through the Lua ExitVehicle
// no-arg twin (lua_creature_events.go): exit position captured BEFORE
// the seat frees (Unit.cpp:13193), kit removal with ev6, fall-or-
// teleport, then exitVehicleTail. A stale seat — a GUID with neither
// session nor motion — frees the seat with the SmartAI
// passenger-removed event only, the reinstall twin. C++ would ASSERT
// on the missing unit (Vehicle.cpp:476; Errors.h WPAssert aborts, a
// PERFORMANCE_PROFILING build compiles it out and the null deref
// crashes instead) — the abort stays out, a stale seat is a
// bookkeeping bug, not a reason to kill the server, but the ASSERT's
// observability leg is ported: the violation is logged with the seat
// context before degrading to the seat free. Callers skip the
// call when the incoming passenger is already seated on this base: the
// board no-ops into its current seat, so there is no seat to take.
// exceptGUID is the incoming passenger; re-boarding its own seat is
// not an eject.
func (s *Server) ejectVehicleSeatOccupant(ctx context.Context, mapID, instanceID uint32, kit *VehicleKit, baseGUID uint64, seatID int8, exceptGUID uint64) {
	if s == nil || kit == nil || baseGUID == 0 || seatID < 0 {
		return
	}
	occupant := kit.GetPassenger(seatID)
	if occupant == 0 || occupant == exceptGUID {
		return
	}
	if occSess := s.findSessionByGUID(occupant); occSess != nil {
		// Vehicle::AddPassenger (Vehicle.cpp:471-478) ejects the
		// seat's occupant through ExitVehicle() — the ride aura
		// strips from the base first and its remove leg runs
		// _ExitVehicle — instead of unseating directly, which would
		// leave the stale ride aura on the base and the dummy aura
		// on the caster. A seat with no ride aura (Lua/debug board)
		// keeps the direct exit as its fallback.
		s.removeVehicleControlAuraFromBase(mapID, instanceID, baseGUID, occupant)
		if occSess.player != nil && occSess.player.VehicleGUID != 0 {
			occSess.exitVehicle(ctx)
		}
		return
	}
	occMotion := s.findCreatureMotion(mapID, instanceID, occupant)
	if occMotion == nil {
		// Vehicle::AddPassenger (Vehicle.cpp:471-478): C++ ASSERTs
		// the occupant resolves to a unit before ejecting it; the
		// abort stays out (see the function header), but the
		// violation is logged with the seat context before
		// degrading to the seat free.
		if s.Logger != nil {
			s.Logger.Error("stale vehicle seat occupant has no unit", "baseGUID", baseGUID, "seat", seatID, "occupantGUID", occupant)
		}
		freedSeat, _, _, _ := kit.RemovePassenger(occupant)
		if base := s.findCreatureMotion(mapID, instanceID, baseGUID); base != nil {
			s.smartPassengerBoarded(ctx, base, occupant, freedSeat, false)
		}
		return
	}
	// Vehicle::AddPassenger (Vehicle.cpp:471-478) ejects the seat's
	// occupant through ExitVehicle() — the ride aura strips from the
	// base first and its remove leg runs _ExitVehicle through
	// removeCreatureVehicleSeat — instead of unseating directly. A
	// strip-unseated occupant already rode the exit position,
	// fall-or-teleport and tail legs inside that path, so only the
	// passenger-list sync remains; a seat with no ride aura
	// (Lua/debug board) keeps the direct _ExitVehicle twin below as
	// its fallback.
	s.removeVehicleControlAuraFromBase(mapID, instanceID, baseGUID, occupant)
	s.motionMu.Lock()
	stillSeated := false
	if m, ok := s.motionMapLocked(mapID, instanceID)[occupant]; ok && m != nil && m.VehicleGUID == baseGUID {
		stillSeated = true
	}
	s.motionMu.Unlock()
	if !stillSeated {
		s.syncCreatureVehiclePassengers(occMotion)
		return
	}
	// Unit::_ExitVehicle (Unit.cpp:13215-13234): the exit position is
	// the vehicle base position with the passenger's orientation, then
	// the ModifyVehiclePassengerExitPos override, then the seat addon's
	// Offset/Dest transform — captured BEFORE the seat frees below
	// (Unit.cpp:13193), via the creatureVehicleExitPosition twin.
	baseMotion := s.findCreatureMotion(mapID, instanceID, baseGUID)
	exitX, exitY, exitZ, exitO := s.creatureVehicleExitPosition(baseGUID, occMotion, baseMotion)
	phase := uint32(0)
	s.motionMu.Lock()
	phase = occMotion.PhaseMask
	s.motionMu.Unlock()
	s.luaVehicleRemovePassenger(ctx, &luaVehicleRef{server: s, mapID: mapID, instanceID: instanceID, baseGUID: baseGUID}, occupant)
	s.motionMu.Lock()
	m2, m2ok := s.motionMapLocked(mapID, instanceID)[occupant]
	canFly := false
	if m2ok {
		m2.X, m2.Y, m2.Z, m2.Orientation = exitX, exitY, exitZ, exitO
		m2.Moving = false
		canFly = m2.CanFly || m2.Flying
	}
	s.motionMu.Unlock()
	// Unit::_ExitVehicle (Unit.cpp:13238-13249): the fall-or-teleport
	// twin from the Lua ExitVehicle arm (lua_creature_events.go).
	falling := false
	if m2ok && !canFly {
		height := exitZ + s.vehicleBaseCollisionHeight(mapID, instanceID, baseGUID)
		level, ground := s.mapWaterOrGroundLevel(mapID, exitX, exitY, height, terrainDefaultCollisionHeight)
		if level > terrainInvalidHeight && height > level {
			falling = s.launchCreatureFallSpline(m2, exitX, exitY, ground, movementInformVehicleExit)
		}
	}
	if !falling {
		_, nearby := buildTeleportMovementPackets(occupant, movementInfo{Time: s.gameTimeMilliseconds(), X: exitX, Y: exitY, Z: exitZ, Orientation: exitO})
		s.broadcastCreatureTeleportMovement(mapID, instanceID, occupant, exitX, exitY, exitZ, nearby, phase)
	}
	s.syncCreatureVehiclePassengers(occMotion)
	// Unit::_ExitVehicle tail (Unit.cpp:13251-13266), shared with the
	// player path via exitVehicleTail.
	s.exitVehicleTail(ctx, mapID, instanceID, baseGUID, occupant)
}

// exitVehicle removes the session's player from their current vehicle.
// Reference: Unit::_ExitVehicle (Unit.cpp:8365) & Vehicle::RemovePassenger (Vehicle.cpp:498-543).
func (s *session) exitVehicle(ctx context.Context) {
	if s == nil || s.player == nil || s.player.VehicleGUID == 0 {
		return
	}

	oldVehGUID := s.player.VehicleGUID
	playerVehicleID := uint32(0)
	var seatAddon *VehicleSeatAddon
	seatedOnOtherMap := false
	if s.server != nil {
		kits := s.server.findVehicleKits(oldVehGUID)
		// The seat addon and control handling ride the kit the
		// passenger is actually seated in, preferring the
		// passenger's own map/instance key; a kit keyed elsewhere
		// (a base that teleported away) still seats the passenger,
		// so its seat is removed below even though the same-map
		// lookup the old code used missed it entirely.
		// The old same-map lookup sent the vehicle-data clear for a
		// player kit even when the passenger was not seated in it
		// (stale VehicleGUID); keep that arm keyed the same way.
		for _, loc := range kits {
			if loc.key.MapID == s.player.Map && loc.key.InstanceID == s.player.InstanceID && loc.kit.IsPlayer {
				playerVehicleID = loc.kit.VehicleID
				break
			}
		}
		var primary *VehicleKit
		primarySameKey := false
		for _, loc := range kits {
			if _, seatInfo, _ := loc.kit.GetSeatForPassenger(s.playerGUID); seatInfo == nil {
				continue
			}
			if loc.key.MapID == s.player.Map && loc.key.InstanceID == s.player.InstanceID {
				primary = loc.kit
				primarySameKey = true
				break
			}
			if primary == nil {
				primary = loc.kit
			}
		}
		if primary != nil {
			if primary.IsPlayer && playerVehicleID == 0 {
				playerVehicleID = primary.VehicleID
			}
			// Unit::_ExitVehicle (Unit.cpp:13193): the seat addon is
			// captured BEFORE RemovePassenger — after removal the seat is
			// empty and the lookup finds nothing.
			seatAddon = primary.GetSeatAddonForSeatOfPassenger(s.playerGUID)
			seatedOnOtherMap = !primarySameKey
		}
		var seatInfo *wotlk.VehicleSeatEntry
		var seatID int8
		wasUnselectable := false
		for _, loc := range kits {
			removedSeat, si, wu, ok := loc.kit.RemovePassenger(s.playerGUID)
			if ok && loc.kit == primary {
				seatID, seatInfo, wasUnselectable = removedSeat, si, wu
			}
		}
		// Vehicle::RemovePassenger (Vehicle.cpp:508-509): freeing a
		// usable seat restores the base's spell-click flag.
		s.server.syncVehicleBaseNPCFlag(s.player.Map, s.player.InstanceID, oldVehGUID)
		// Vehicle::RemovePassenger (Vehicle.cpp:538-539) fires
		// OnRemovePassenger for creature bases only; seatInfo is set
		// exactly when the primary-kit removal succeeded.
		if primary != nil && !primary.IsPlayer && seatInfo != nil {
			if base := s.server.findCreatureMotion(s.player.Map, s.player.InstanceID, oldVehGUID); base != nil {
				// Vehicle::RemovePassenger (Vehicle.cpp:536-539):
				// the AI PassengerBoarded(false) call precedes the
				// OnRemovePassenger script hook.
				s.server.smartPassengerBoarded(ctx, base, s.playerGUID, seatID, false)
				s.server.triggerVehicleEvent(scripting.VehicleEventOnRemovePassenger, s.server.luaMotionCreature(base), s.luaPlayer())
			}
		}
		if seatInfo != nil {
			// Vehicle::RemovePassenger (Vehicle.cpp:517-518): the
			// charm drops only off a creature base — a player kit
			// never charmed, so there is nothing to release.
			if seatInfo.CanControl() && primary != nil && !primary.IsPlayer {
				s.sendClientControl(oldVehGUID, false)
				s.sendVehiclePetSpells(ctx, 0, 0, nil)
			}
			// Vehicle::RemovePassenger (Vehicle.cpp:928-930): the
			// NOT_SELECTABLE flag is only cleared when the passenger
			// did not carry it before boarding.
			if seatInfo.HasFlag(wotlk.VehicleSeatFlagPassengerNotSelectable) && !wasUnselectable {
				s.player.UnitFlags &^= 0x02000000
			}
		}
		// Vehicle::RemovePassenger (Vehicle.cpp:518-529): leaving a
		// vehicle that itself rides a transport hands the passenger
		// the base's transport (GUID + offsets), so the passenger
		// stays aboard; leaving a vehicle that rides nothing resets
		// the passenger's transport. The base's transport is its
		// stored m_movementInfo (the kit snapshot, acked from the
		// driver's packets) — or, when the driver never moved, the
		// base motion's stamped transport (a vehicle passenger's seat
		// offsets, VehicleJoinEvent :865-869) or its static-passenger
		// TransportGUID, whose offsets Go cannot recover, so the
		// passenger keeps its own. The snapshot and the stamped motion
		// both carry offsets, so both arm the inherit leg; an
		// unstamped motion GUID — or a player base's session
		// TransportGUID — only vetoes the reset.
		if primary != nil {
			baseOnTransport := false
			var baseTransport *transportMovement
			if snap, ok := primary.movementInfoSnapshot(); ok && snap.Transport != nil && snap.Transport.GUID != 0 {
				baseTransport = snap.Transport
				baseOnTransport = true
			}
			if !baseOnTransport {
				if motion := s.server.findCreatureMotion(s.player.Map, s.player.InstanceID, oldVehGUID); motion != nil && motion.TransportGUID != 0 {
					// A base whose transport was stamped at board (a
					// vehicle passenger, VehicleJoinEvent :865-869)
					// arms the inherit leg with its stamp; a real-
					// transport base carries no offsets on its motion
					// and only vetoes the reset, as before.
					if motion.VehicleGUID != 0 {
						s.server.motionMu.Lock()
						if motion.TransportGUID != 0 {
							baseTransport = &transportMovement{GUID: motion.TransportGUID, X: motion.TransportX, Y: motion.TransportY, Z: motion.TransportZ, Orientation: motion.TransportO, Seat: motion.TransportSeat}
						}
						s.server.motionMu.Unlock()
					}
					baseOnTransport = true
				} else if baseSess := s.server.findSessionByGUID(oldVehGUID); baseSess != nil && baseSess.player != nil && baseSess.player.TransportGUID != 0 {
					baseOnTransport = true
				}
			}
			if baseTransport != nil {
				s.player.TransportGUID = baseTransport.GUID
				s.player.TransportX, s.player.TransportY, s.player.TransportZ, s.player.TransportO = baseTransport.X, baseTransport.Y, baseTransport.Z, baseTransport.Orientation
				s.player.TransportSeat = baseTransport.Seat
			} else if !baseOnTransport {
				s.player.TransportGUID = 0
				s.player.TransportX, s.player.TransportY, s.player.TransportZ, s.player.TransportO = 0, 0, 0, 0
				s.player.TransportSeat = 0
			}
		}
		// Vehicle::RemovePassenger (Vehicle.cpp:533): a flying
		// passenger is handed the parachute spell as a triggered
		// cast from the base.
		s.castVehicleExitParachute(ctx, oldVehGUID)
	}

	// Unit::_ExitVehicle (Unit.cpp:13199-13200): a player leaving a vehicle
	// mid-mounted-duel forfeits it — DuelComplete(DUEL_FLED), partner as
	// winner — matching the out-of-bounds fled arm's endDuel(false,
	// duelPartner, true) call. Runs after RemovePassenger above, as in C++.
	if s.duelPartner != 0 && s.duelMounted {
		s.endDuel(false, s.duelPartner, true)
	}

	// VehicleAI::OnCharmed (CombatAI.cpp:295-306): when the last passenger
	// leaves a conditioned vehicle that is no longer in use and no longer
	// charmed, the vehicle is marked for dismiss. Go sets no charm state
	// on boarding, so the exit itself is the OnCharmed twin; the 5s timer
	// is armed here and consumed by the vehicle step, and boarding again
	// clears it (enterVehicle).
	if s.server != nil && oldVehGUID != 0 {
		kits := s.server.findVehicleKits(oldVehGUID)
		inUse := false
		motionMap, motionInstance := s.player.Map, s.player.InstanceID
		for _, loc := range kits {
			if loc.kit.IsVehicleInUse() {
				inUse = true
			}
			if loc.key.MapID == s.player.Map && loc.key.InstanceID == s.player.InstanceID {
				motionMap, motionInstance = loc.key.MapID, loc.key.InstanceID
			}
		}
		if len(kits) > 0 && !inUse {
			if motion := s.server.findCreatureMotion(motionMap, motionInstance, oldVehGUID); motion != nil && s.server.isVehicleAI(motion) && s.server.vehicleHasConditions(ctx, motion.Entry) {
				motion.VehicleDismiss = true
				motion.VehicleDismissAt = time.Now().Add(5 * time.Second)
			}
		}
	}

	// Unit::_ExitVehicle (Unit.cpp:13207): the fall baseline resets to
	// the exit position.
	s.lastFallZ = s.player.Z
	s.lastFallTime = 0

	// Unit::_ExitVehicle (Unit.cpp:13215-13234): the exit position is the
	// vehicle base position with the passenger's orientation, then the
	// seat addon's Offset/Dest arms. A passenger seated in a kit keyed
	// on another map/instance keeps its own coordinates: the base
	// position on that other map is not a position on the passenger's
	// map, so applying it would strand the passenger at foreign
	// coordinates on its own map.
	// A far-teleport-pending passenger's coords are its teleport
	// destination, folded in at initiation (commands.go) — C++ keeps the
	// pending destination apart from position (m_teleport_dest,
	// Player.cpp:1796; the ack moves to GetTeleportDest(),
	// MovementHandler.cpp:55), so _ExitVehicle's exit position never
	// clobbers it. The other-map skip above covers a kit keyed on the
	// origin map; this covers a passenger re-boarded onto a
	// destination-map kit while pending, where the kit key matches and
	// the exit position would otherwise overwrite the destination the
	// ack still has to land on.
	if !seatedOnOtherMap && !s.farTeleportPending {
		s.applyVehicleExitPosition(oldVehGUID, seatAddon)
	}

	// Unit::_ExitVehicle (Unit.cpp:13251-13252): a temporarily-unsummoned
	// pet is resummoned after the MoveTo spline launch (:13246-13249),
	// so it runs after the exit position above, not before it.
	s.resummonTemporaryPet(ctx)

	// Unit::_ExitVehicle tail (Unit.cpp:13254-13266): the tail follows the
	// MoveTo spline launch (:13246-13249), so it runs after the exit
	// position above, not before it. A minion vehicle base owned by the
	// exiting player despawns 1ms after the exit; the accessory legs
	// cannot fire for a player passenger (a player never carries
	// UNIT_MASK_ACCESSORY) and ride the same helper for the creature
	// ExitVehicle arm, which calls it once on its own path.
	s.server.exitVehicleTail(ctx, s.player.Map, s.player.InstanceID, oldVehGUID, s.playerGUID)

	s.player.VehicleGUID = 0
	s.player.VehicleSeat = 0
	// VehicleJoinEvent::Execute (Vehicle.cpp:881) roots the passenger on
	// board; the exit clears it.
	s.setRooted(false)
	if playerVehicleID != 0 {
		s.sendPlayerVehicleData(0)
	}
	s.sendPlayerUpdate()
}

// ejectVehiclePassengersAtMapLeave mirrors the far-teleport map-leave in
// Player::TeleportTo (Player.cpp:1855-1860): oldmap->RemovePlayerFromMap ->
// Player::RemoveFromWorld (Player.cpp:1992) -> Unit::RemoveFromWorld
// (Unit.cpp:9789) -> RemoveVehicleKit -> Uninstall -> RemoveAllPassengers
// (Vehicle.cpp:253), which removes CONTROL_VEHICLE auras on the base and
// whose unapply exits each passenger. The near branch has no
// RemovePlayerFromMap, so it never runs this. Go has no RemoveFromWorld
// step, so the caller runs this in the far branch BEFORE the Map/coords
// overwrite, while the base still stands on the origin map: exit positions
// read the base session's current coords (applyVehicleExitPosition), so
// an ack-time eject would strand passengers at destination coords on the
// origin map. A kit already dropped (chained teleport, logout drain) reads
// as no kits and no 236 auras, so the helper is a safe no-op there.
func (s *session) ejectVehiclePassengersAtMapLeave(ctx context.Context) {
	if s == nil || s.player == nil || s.server == nil {
		return
	}
	// Vehicle::RemoveAllPassengers order: the 236 auras on the base go
	// first, and their remove tails exit the aura-boarded passengers;
	// the snapshot-then-remove shape mirrors
	// removeAurasWithInterruptFlags (movement.go).
	var controlSpells []uint32
	s.castMu.Lock()
	var snapshot []*activeAura
	for _, aura := range s.activeAuras {
		if aura != nil {
			snapshot = append(snapshot, aura)
		}
	}
	s.castMu.Unlock()
	for _, aura := range snapshot {
		if s.activeAuraHasEffect(aura, spellAuraControlVehicle) {
			controlSpells = append(controlSpells, aura.SpellID)
		}
	}
	for _, spellID := range controlSpells {
		s.removeAura(spellID)
	}
	// Packet/spell-click boardings carry no aura, so any passenger still
	// seated after the aura sweep exits directly; the GUID scan covers a
	// kit keyed on the origin map, not just the base's current key.
	for _, loc := range s.server.findVehicleKits(s.playerGUID) {
		for _, pg := range loc.kit.seatedPassengerGUIDs() {
			if pg == s.playerGUID {
				continue
			}
			if ps := s.server.findSessionByGUID(pg); ps != nil && ps.player != nil {
				ps.exitVehicle(ctx)
			}
		}
		s.server.removeVehicleKit(loc.key.MapID, loc.key.InstanceID, s.playerGUID)
	}
}

// releaseVehicleStateAtLogout mirrors Unit::RemoveFromWorld
// (Unit.cpp:9786-9802): a leaving vehicle base drops its kit first
// (RemoveVehicleKit -> Uninstall -> RemoveAllPassengers), then a leaving
// passenger exits (ExitVehicle). It runs after the logout save, matching
// C++ SaveToDB-before-RemovePlayerFromMap, so the saved position is the
// riding position, not the exit position. The passenger arm deliberately
// does NOT call exitVehicle: a full exit would resummon a
// temporarily-unsummoned pet into a discarded session (spawnPet) and send
// control/update packets to a departing client, while the seat release
// itself is the only durable effect — the logger's player object is
// discarded below. A 236-boarded logger is already freed before this runs
// by the leave-world sweep (the base's remove tail exits the caster), so
// this arm covers the packet/spell-click boardings that carry no aura.
func (s *session) releaseVehicleStateAtLogout(ctx context.Context) {
	if s == nil || s.player == nil || s.server == nil {
		return
	}
	// A base that far-teleported before logging out left its kit keyed
	// on the origin map/instance, so the base's current key misses it
	// and the origin kit — ghost seats and all — would survive the
	// session discard. The GUID scan finds every kit for the base,
	// ejects its seated passengers from each, and drops each kit under
	// its own key; a kit on the current key is the normal case and
	// behaves exactly as before.
	for _, loc := range s.server.findVehicleKits(s.playerGUID) {
		for _, pg := range loc.kit.seatedPassengerGUIDs() {
			if pg == s.playerGUID {
				continue
			}
			if ps := s.server.findSessionByGUID(pg); ps != nil && ps.player != nil {
				ps.exitVehicle(ctx)
			}
		}
		s.server.removeVehicleKit(loc.key.MapID, loc.key.InstanceID, s.playerGUID)
	}
	if s.player.VehicleGUID != 0 {
		vehGUID := s.player.VehicleGUID
		// The logger-as-passenger arm is GUID-scoped for the mirror
		// case: a logger drained from a pending far teleport carries
		// the destination map with the origin instance, which misses
		// the origin-keyed kit under the logger's own key. The
		// conditioned-vehicle dismiss motion lookup rides the kit's
		// own key for the same reason.
		kits := s.server.findVehicleKits(vehGUID)
		vehicleFired := false
		for _, loc := range kits {
			if seatID, _, _, ok := loc.kit.RemovePassenger(s.playerGUID); ok && !vehicleFired && !loc.kit.IsPlayer {
				// Logout removal is still a Vehicle::RemovePassenger
				// fire for creature bases (Vehicle.cpp:538-539); the
				// first successful removal fires once, matching the
				// single C++ vehicle the passenger sat in.
				if base := s.server.findCreatureMotion(loc.key.MapID, loc.key.InstanceID, vehGUID); base != nil {
					// Vehicle::RemovePassenger (Vehicle.cpp:536-539):
					// the AI PassengerBoarded(false) call precedes
					// the OnRemovePassenger script hook.
					s.server.smartPassengerBoarded(ctx, base, s.playerGUID, seatID, false)
					s.server.triggerVehicleEvent(scripting.VehicleEventOnRemovePassenger, s.server.luaMotionCreature(base), s.luaPlayer())
					vehicleFired = true
				}
				// Vehicle::RemovePassenger (Vehicle.cpp:508-509):
				// freeing a usable seat restores the base's
				// spell-click flag; the helper resolves a kit
				// keyed on another map.
				s.server.syncVehicleBaseNPCFlag(s.player.Map, s.player.InstanceID, vehGUID)
			}
		}
		if len(kits) > 0 {
			inUse := false
			motionMap, motionInstance := kits[0].key.MapID, kits[0].key.InstanceID
			for _, loc := range kits {
				if loc.kit.IsVehicleInUse() {
					inUse = true
				}
				if loc.key.MapID == s.player.Map && loc.key.InstanceID == s.player.InstanceID {
					motionMap, motionInstance = loc.key.MapID, loc.key.InstanceID
				}
			}
			if !inUse {
				if motion := s.server.findCreatureMotion(motionMap, motionInstance, vehGUID); motion != nil && s.server.isVehicleAI(motion) && s.server.vehicleHasConditions(ctx, motion.Entry) {
					motion.VehicleDismiss = true
					motion.VehicleDismissAt = time.Now().Add(5 * time.Second)
				}
			}
		}
		s.player.VehicleGUID = 0
		s.player.VehicleSeat = 0
	}
}

// applyVehicleSeatExitAddon applies a seat addon's exit parameters to an exit
// position whose orientation is the passenger's: Offset rotates the addon XYZO
// by that orientation (Position::RelocateOffset, Position.cpp:36-42), Dest
// replaces the position outright, None leaves it alone.
// Reference: Unit::_ExitVehicle (Unit.cpp:13229-13234).
func applyVehicleSeatExitAddon(x, y, z, o float32, addon *VehicleSeatAddon) (float32, float32, float32, float32) {
	if addon == nil {
		return x, y, z, o
	}
	switch addon.ExitParameter {
	case wotlk.VehicleExitParamOffset:
		// Position::RelocateOffset (Position.cpp:36-42): sin(O+PI) is -sin(O).
		cosO := float32(math.Cos(float64(o)))
		sinO := float32(math.Sin(float64(o)))
		x += addon.ExitParamX*cosO - addon.ExitParamY*sinO
		y += addon.ExitParamY*cosO + addon.ExitParamX*sinO
		z += addon.ExitParamZ
		o = normalizeOrientation(o + addon.ExitParamO)
	case wotlk.VehicleExitParamDest:
		x, y, z, o = addon.ExitParamX, addon.ExitParamY, addon.ExitParamZ, addon.ExitParamO
	}
	return x, y, z, o
}

// applyVehicleExitPosition resolves the passenger's exit coordinates.
// Reference: Unit::_ExitVehicle (Unit.cpp:13215-13234).
func (s *session) applyVehicleExitPosition(vehicleGUID uint64, addon *VehicleSeatAddon) {
	if s == nil || s.player == nil {
		return
	}
	baseX, baseY, baseZ := s.player.X, s.player.Y, s.player.Z
	if s.server != nil {
		if baseSess := s.server.findSessionByGUID(vehicleGUID); baseSess != nil && baseSess.player != nil {
			baseX, baseY, baseZ = baseSess.player.X, baseSess.player.Y, baseSess.player.Z
		}
	}
	o := s.player.Orientation
	baseX, baseY, baseZ, o = applyVehicleSeatExitAddon(baseX, baseY, baseZ, o, addon)
	s.player.X, s.player.Y, s.player.Z, s.player.Orientation = baseX, baseY, baseZ, o
}

// castVehicleExitParachute mirrors the flyable-vehicle leg of
// Vehicle::RemovePassenger (Vehicle.cpp:533): a flying passenger is handed
// the parachute spell (VEHICLE_SPELL_PARACHUTE, 45472) as a triggered cast
// from the vehicle base. The flying check is Unit::IsFlying (Unit.h:1644) —
// FLYING or DISABLE_GRAVITY on the passenger's movement info, the same
// predicate player:IsFlying reads. The cast rides the triggered funnel, so
// it lands as the slow-fall aura (SPELL_AURA_FEATHER_FALL) once the 45472
// row is seeded; the seed DBC carries no 45472 row today, so the funnel
// fails closed until the seed axis resumes.
func (s *session) castVehicleExitParachute(ctx context.Context, baseGUID uint64) {
	if s == nil || s.player == nil || s.server == nil || baseGUID == 0 {
		return
	}
	mi, ok := s.lastMovementInfoSnapshot()
	if !ok || mi.Flags&(movementFlying|movementDisableGravity) == 0 {
		return
	}
	if motion := s.server.findCreatureMotion(s.player.Map, s.player.InstanceID, baseGUID); motion != nil {
		s.server.castCreatureSpellTriggered(ctx, motion, wotlk.VehicleSpellParachute, s.playerGUID)
		return
	}
	if baseSess := s.server.findSessionByGUID(baseGUID); baseSess != nil && baseSess.player != nil {
		baseSess.castSpellDirect(ctx, wotlk.VehicleSpellParachute, s.playerGUID)
		return
	}
	s.castSpellDirect(ctx, wotlk.VehicleSpellParachute, s.playerGUID)
}

// castCreatureVehicleExitParachute is the creature-passenger twin of
// castVehicleExitParachute: Vehicle::RemovePassenger (Vehicle.cpp:533)
// hands a flying passenger the parachute spell (VEHICLE_SPELL_PARACHUTE,
// 45472) as a triggered cast from the vehicle base. The flying check is
// Unit::IsFlying (Unit.h:1644) — the CanFly/Flying twin the board legs
// already use. The cast rides the triggered funnel, so it lands as the
// slow-fall aura once the 45472 row is seeded; the seed DBC carries no
// 45472 row today, so the funnel fails closed until the seed axis
// resumes — the same dormancy as the player leg.
func (s *Server) castCreatureVehicleExitParachute(ctx context.Context, baseGUID uint64, passenger *creatureMotion) {
	if s == nil || baseGUID == 0 || passenger == nil {
		return
	}
	if !passenger.CanFly && !passenger.Flying {
		return
	}
	if motion := s.findCreatureMotion(passenger.Map, passenger.InstanceID, baseGUID); motion != nil {
		s.castCreatureSpellTriggered(ctx, motion, wotlk.VehicleSpellParachute, passenger.GUID)
		return
	}
	if baseSess := s.findSessionByGUID(baseGUID); baseSess != nil && baseSess.player != nil {
		baseSess.castSpellDirect(ctx, wotlk.VehicleSpellParachute, passenger.GUID)
		return
	}
}

// applyControlVehicleSeatChange runs the literal C++ ChangeAmount seat
// switch (AuraEffect::ChangeAmount, SpellAuraEffects.cpp:662-683) for a
// SPELL_AURA_CONTROL_VEHICLE re-cast merge: the unapply leg removes the
// passenger from the kit first - Vehicle::RemovePassenger only, no
// _ExitVehicle (SpellAuraEffects.cpp:3002) - and the apply leg re-boards
// through Unit::_EnterVehicle (Unit.cpp:13088-13130) into
// Vehicle::AddPassenger (Vehicle.cpp:426-484). That is NOT the
// Unit::ChangeSeat model strips 51-52 used (resolveChangeSeatID +
// SwitchSeat): a specific occupied seat EJECTS its occupant
// (Vehicle.cpp:471-478, the ejectVehicleSeatOccupant twin) instead of
// refusing, and a merged amount of 0 (seat -1) scans lowest-first from
// the SeatMap head (Vehicle.cpp:453-460, inside AddPassenger) instead
// of stepping forward from the current seat. ChangeSeat's refuse /
// next-forward rules belong to the CMSG path only, where ChangeSeat
// pre-validates the seat before ChangeAmount runs (Unit.cpp:13144-13146).
// Same-seat merges are no-ops (Unit.cpp:13095-13096); a re-board that
// fails leaves the passenger off the vehicle - the unapply already
// fired - so the stamps clear and only the remove hooks have run.
func (s *session) applyControlVehicleSeatChange(ctx context.Context, baseGUID uint64, newSeat int8) {
	if s == nil || s.server == nil || s.player == nil || baseGUID == 0 {
		return
	}
	if s.player.VehicleGUID != baseGUID {
		return
	}
	kit := s.server.getVehicleKit(s.player.Map, s.player.InstanceID, baseGUID)
	if kit == nil {
		return
	}
	oldSeat := s.player.VehicleSeat
	if newSeat >= 0 && newSeat == oldSeat {
		return
	}
	_, oldSeatInfo, wasUnselectable, removed := kit.RemovePassenger(s.playerGUID)
	if !removed {
		return
	}
	// Vehicle::RemovePassenger (Vehicle.cpp:512-514): the unapply
	// restores the NOT_SELECTABLE flag when the old seat carried it
	// and the passenger did not before boarding.
	s.applyVehicleSeatNotSelectable(oldSeatInfo, nil, wasUnselectable)
	// Vehicle::RemovePassenger (Vehicle.cpp:533): the unapply leg hands
	// a flying passenger the parachute spell before the remove hooks
	// below, as in C++.
	s.castVehicleExitParachute(ctx, baseGUID)
	s.fireVehicleSeatRemove(ctx, kit, oldSeat)
	if newSeat >= 0 {
		s.server.ejectVehicleSeatOccupant(ctx, s.player.Map, s.player.InstanceID, kit, baseGUID, newSeat, s.playerGUID)
	}
	assigned, seatInfo, ok := kit.AddPassenger(s.playerGUID, newSeat, wasUnselectable)
	if !ok {
		// A failed re-board leaves the passenger off the vehicle, so
		// the stamps clear and the transport follows
		// Vehicle::RemovePassenger (Vehicle.cpp:518-529): a base that
		// itself rides a transport hands its transport over, otherwise
		// the transport resets - the exitVehicle arm's check, on this
		// kit's snapshot.
		s.player.VehicleGUID = 0
		s.player.VehicleSeat = 0
		baseOnTransport := false
		var baseTransport *transportMovement
		if snap, ok := kit.movementInfoSnapshot(); ok && snap.Transport != nil && snap.Transport.GUID != 0 {
			baseTransport = snap.Transport
			baseOnTransport = true
		}
		if !baseOnTransport {
			if motion := s.server.findCreatureMotion(s.player.Map, s.player.InstanceID, baseGUID); motion != nil && motion.TransportGUID != 0 {
				// A base whose transport was stamped at board (a
				// vehicle passenger, VehicleJoinEvent :865-869) arms
				// the inherit leg with its stamp; a real-transport
				// base carries no offsets on its motion and only
				// vetoes the reset, as before.
				if motion.VehicleGUID != 0 {
					s.server.motionMu.Lock()
					if motion.TransportGUID != 0 {
						baseTransport = &transportMovement{GUID: motion.TransportGUID, X: motion.TransportX, Y: motion.TransportY, Z: motion.TransportZ, Orientation: motion.TransportO, Seat: motion.TransportSeat}
					}
					s.server.motionMu.Unlock()
				}
				baseOnTransport = true
			} else if baseSess := s.server.findSessionByGUID(baseGUID); baseSess != nil && baseSess.player != nil && baseSess.player.TransportGUID != 0 {
				baseOnTransport = true
			}
		}
		if baseTransport != nil {
			s.player.TransportGUID = baseTransport.GUID
			s.player.TransportX, s.player.TransportY, s.player.TransportZ, s.player.TransportO = baseTransport.X, baseTransport.Y, baseTransport.Z, baseTransport.Orientation
			s.player.TransportSeat = baseTransport.Seat
		} else if !baseOnTransport {
			s.player.TransportGUID = 0
			s.player.TransportX, s.player.TransportY, s.player.TransportZ, s.player.TransportO = 0, 0, 0, 0
			s.player.TransportSeat = 0
		}
		s.sendPlayerUpdate()
		s.server.syncVehicleBaseNPCFlag(s.player.Map, s.player.InstanceID, baseGUID)
		return
	}
	// VehicleJoinEvent::Execute (Vehicle.cpp:855-856): the re-board
	// sets UNIT_FLAG_NOT_SELECTABLE when the new seat carries it.
	s.applyVehicleSeatNotSelectable(nil, seatInfo, wasUnselectable)
	s.player.VehicleSeat = assigned
	// VehicleJoinEvent::Execute (Vehicle.cpp:865-869): the re-board is a
	// new JoinEvent (Unit::ChangeSeat, Unit.cpp:13133-13170), so the
	// transport re-stamps with the new seat's offsets — without this the
	// merge arm leaves TransportSeat (and the offsets) on the old seat.
	// The creature twin (applyCreatureControlVehicleSeatChange) already
	// re-stamps; this is the same stamp as the enterVehicle board leg.
	if _, newSeatInfo, newSeatAddon := kit.GetSeatForPassenger(s.playerGUID); newSeatInfo != nil {
		s.player.TransportGUID = baseGUID
		s.player.TransportX, s.player.TransportY, s.player.TransportZ = newSeatInfo.AttachmentOffsetX, newSeatInfo.AttachmentOffsetY, newSeatInfo.AttachmentOffsetZ
		s.player.TransportO = 0
		if newSeatAddon != nil {
			s.player.TransportO = newSeatAddon.SeatOrientationOffset
		}
		s.player.TransportSeat = assigned
	}
	// Vehicle::RemovePassenger (Vehicle.cpp:517-518) and
	// VehicleJoinEvent::Execute (Vehicle.cpp:871-876): the charm legs
	// fire only on a creature base — a player kit never charms.
	if seatInfo != nil && !kit.IsPlayer && seatInfo.CanControl() {
		s.sendClientControl(kit.VehicleGUID, true)
		spells := s.server.loadCreatureSpells(ctx, kit.CreatureEntry)
		s.sendVehiclePetSpells(ctx, kit.VehicleGUID, kit.CreatureEntry, spells)
	} else if oldSeatInfo != nil && !kit.IsPlayer && oldSeatInfo.CanControl() {
		// Vehicle::RemovePassenger (Vehicle.cpp:517-518) drops the
		// charm when the old seat could control and the new seat
		// cannot; the JoinEvent re-charms only on a control seat.
		s.sendClientControl(kit.VehicleGUID, false)
		s.sendVehiclePetSpells(ctx, 0, 0, nil)
	}
	s.fireVehicleSeatAdd(ctx, kit, assigned)
	s.sendPlayerUpdate()
	s.server.syncVehicleBaseNPCFlag(s.player.Map, s.player.InstanceID, s.player.VehicleGUID)
}

// applyCreatureControlVehicleSeatChange is the creature-passenger twin of
// applyControlVehicleSeatChange: the same literal ChangeAmount path
// (RemovePassenger + _EnterVehicle/AddPassenger, eject-on-occupied,
// -1 lowest-first; SpellAuraEffects.cpp:662-683, Vehicle.cpp:453-460,
// :471-478). The CanControl leg is absent by C++ design: the JoinEvent
// charm arm only fires for a player passenger on a creature base
// (Vehicle.cpp:871-876), so a creature passenger takes no control. A
// failed re-board leaves the creature off the vehicle with only the
// remove hooks fired, then runs the RemovePassenger transport leg
// (inherit-or-reset, Vehicle.cpp:518-529); a successful re-board
// re-stamps the transport with the new seat's offsets
// (VehicleJoinEvent, Vehicle.cpp:865-869).
func (s *Server) applyCreatureControlVehicleSeatChange(ctx context.Context, mapID, instanceID uint32, baseGUID, casterGUID uint64, newSeat int8) {
	if s == nil || baseGUID == 0 || casterGUID == 0 {
		return
	}
	motion := s.findCreatureMotion(mapID, instanceID, casterGUID)
	if motion == nil || motion.VehicleGUID != baseGUID {
		return
	}
	kit := s.getVehicleKit(mapID, instanceID, baseGUID)
	if kit == nil {
		return
	}
	oldSeat := motion.VehicleSeat
	if newSeat >= 0 && newSeat == oldSeat {
		return
	}
	_, oldSeatInfo, wasUnselectable, removed := kit.RemovePassenger(casterGUID)
	if !removed {
		return
	}
	// Vehicle::RemovePassenger (Vehicle.cpp:512-514, :533): the unapply
	// restores the NOT_SELECTABLE flag and hands a flying passenger
	// the parachute spell, ahead of the remove hooks.
	s.clearCreatureVehicleNotSelectable(motion, oldSeatInfo, wasUnselectable)
	s.castCreatureVehicleExitParachute(ctx, baseGUID, motion)
	s.fireCreatureVehicleSeatRemove(ctx, mapID, instanceID, kit, baseGUID, casterGUID, oldSeat)
	if newSeat >= 0 {
		s.ejectVehicleSeatOccupant(ctx, mapID, instanceID, kit, baseGUID, newSeat, casterGUID)
	}
	assigned, seatInfo, ok := kit.AddPassenger(casterGUID, newSeat, wasUnselectable)
	if !ok {
		s.motionMu.Lock()
		motion.VehicleGUID = 0
		motion.VehicleSeat = 0
		s.motionMu.Unlock()
		// Vehicle::RemovePassenger (Vehicle.cpp:518-529): the
		// failed re-board leaves the creature off the vehicle,
		// so the transport leg runs here.
		s.inheritOrResetCreatureVehicleTransport(mapID, instanceID, motion, baseGUID)
		s.syncVehicleBaseNPCFlag(mapID, instanceID, baseGUID)
		return
	}
	// VehicleJoinEvent::Execute (Vehicle.cpp:855-856): the re-board
	// sets UNIT_FLAG_NOT_SELECTABLE when the new seat carries it.
	s.setCreatureVehicleNotSelectable(motion, seatInfo)
	s.motionMu.Lock()
	motion.VehicleSeat = assigned
	s.motionMu.Unlock()
	// The re-board is a new JoinEvent (Unit::ChangeSeat,
	// Unit.cpp:13133-13170), so the transport re-stamps
	// with the new seat's offsets (Vehicle.cpp:865-869).
	_, seatInfo, seatAddon := kit.GetSeatForPassenger(casterGUID)
	s.motionMu.Lock()
	stampCreatureVehicleTransport(motion, baseGUID, assigned, seatInfo, seatAddon)
	s.motionMu.Unlock()
	s.fireCreatureVehicleSeatAdd(ctx, mapID, instanceID, kit, baseGUID, casterGUID, assigned)
	s.syncVehicleBaseNPCFlag(mapID, instanceID, baseGUID)
}

// fireVehicleSeatRemove fires the Vehicle::RemovePassenger hook half of
// a seat switch: the AI PassengerBoarded(false) call precedes the
// OnRemovePassenger script hook (Vehicle.cpp:536-539). A player base
// stays silent per the TYPEID_UNIT gate: it resolves to no creature
// motion. Split from fireVehicleSeatSwitch so the ChangeAmount merge
// arm can fire it right after the unapply RemovePassenger, before the
// eject and the re-board (AuraEffect::ChangeAmount,
// SpellAuraEffects.cpp:662-683).
func (s *session) fireVehicleSeatRemove(ctx context.Context, kit *VehicleKit, oldSeat int8) {
	if s == nil || s.server == nil || s.player == nil || kit == nil || kit.IsPlayer {
		return
	}
	base := s.server.findCreatureMotion(s.player.Map, s.player.InstanceID, s.player.VehicleGUID)
	if base == nil {
		return
	}
	s.server.smartPassengerBoarded(ctx, base, s.playerGUID, oldSeat, false)
	s.server.triggerVehicleEvent(scripting.VehicleEventOnRemovePassenger, s.server.luaMotionCreature(base), s.luaPlayer())
}

// fireVehicleSeatAdd fires the Vehicle::AddPassenger hook half of a seat
// switch, the VehicleJoinEvent::Execute legs
// (Vehicle.cpp:883, :895-896, :900-902): the re-board breaks the
// passenger's target for everyone in range, transfers the passenger's
// threat to the base, then the AI PassengerBoarded(true) call and the
// OnAddPassenger script hook.
func (s *session) fireVehicleSeatAdd(ctx context.Context, kit *VehicleKit, newSeat int8) {
	if s == nil || s.server == nil || s.player == nil || kit == nil || kit.IsPlayer {
		return
	}
	base := s.server.findCreatureMotion(s.player.Map, s.player.InstanceID, s.player.VehicleGUID)
	if base == nil {
		return
	}
	// VehicleJoinEvent::Execute (Vehicle.cpp:895-896): the seat change
	// re-boards through a new JoinEvent (Unit::ChangeSeat,
	// Unit.cpp:13133-13170), so the threat transfer runs again ahead of
	// the AI PassengerBoarded / OnAddPassenger hooks - the base gains
	// the passenger's threat value a second time, as in C++.
	// VehicleJoinEvent::Execute (Vehicle.cpp:883): the re-board breaks
	// the passenger's target for everyone in range.
	s.server.sendPassengerClearTarget(s.player.Map, s.player.InstanceID, s.playerGUID, s.player.X, s.player.Y, s.player.Z, s.currentPlayerPhaseMask())
	s.server.transferPassengerThreatToVehicleBase(s.player.Map, s.player.InstanceID, s.playerGUID, s.player.VehicleGUID)
	s.server.smartPassengerBoarded(ctx, base, s.playerGUID, newSeat, true)
	s.server.triggerVehicleEvent(scripting.VehicleEventOnAddPassenger, s.server.luaMotionCreature(base), s.luaPlayer(), newSeat)
}

// fireVehicleSeatSwitch fires the Vehicle::RemovePassenger /
// Vehicle::AddPassenger hook pair for a seat switch, the combined call
// the CMSG ChangeSeat path rides: ChangeSeat pre-validates the seat
// (Unit.cpp:13144-13146) and ChangeAmount runs the pair
// (SpellAuraEffects.cpp:662-683). The merge arm calls the halves
// separately instead, so the remove hooks fire before the eject.
func (s *session) fireVehicleSeatSwitch(ctx context.Context, kit *VehicleKit, oldSeat, newSeat int8) {
	if oldSeat == newSeat {
		return
	}
	s.fireVehicleSeatRemove(ctx, kit, oldSeat)
	s.fireVehicleSeatAdd(ctx, kit, newSeat)
}

// sendVehicleSeatControlChange runs the charm half of a CMSG seat switch:
// Unit::ChangeSeat (Unit.cpp:13133-13170) re-applies the ride aura, whose
// ChangeAmount unapply leg drops the charm when the old seat could control
// (Vehicle::RemovePassenger, Vehicle.cpp:517-518) and whose re-board
// re-charms only on a control seat (VehicleJoinEvent::Execute,
// Vehicle.cpp:871-876). A switch onto a control seat grants control; a
// switch off one releases it and clears the vehicle action bar. Both
// C++ legs gate the charm on the base being a creature (TYPEID_UNIT)
// with a player passenger on a CAN_CONTROL seat — a player base never
// charms (Vehicle.cpp:517-518, :871-876) — so a player kit stays silent
// here; the session passenger is a player by construction and
// CanControl is checked per leg below.
func (s *session) sendVehicleSeatControlChange(ctx context.Context, kit *VehicleKit, oldSeatInfo, newSeatInfo *wotlk.VehicleSeatEntry) {
	if s == nil || kit == nil || kit.IsPlayer {
		return
	}
	if newSeatInfo != nil && newSeatInfo.CanControl() {
		s.sendClientControl(kit.VehicleGUID, true)
		spells := s.server.loadCreatureSpells(ctx, kit.CreatureEntry)
		s.sendVehiclePetSpells(ctx, kit.VehicleGUID, kit.CreatureEntry, spells)
	} else if oldSeatInfo != nil && oldSeatInfo.CanControl() {
		s.sendClientControl(kit.VehicleGUID, false)
		s.sendVehiclePetSpells(ctx, 0, 0, nil)
	}
}

// fireCreatureVehicleSeatRemove is the creature-passenger twin of
// fireVehicleSeatRemove: the Vehicle::RemovePassenger hook half of a
// creature seat switch (Vehicle.cpp:536-539). A player base stays silent
// per the TYPEID_UNIT gate: it resolves to no creature motion.
func (s *Server) fireCreatureVehicleSeatRemove(ctx context.Context, mapID, instanceID uint32, kit *VehicleKit, baseGUID, passengerGUID uint64, oldSeat int8) {
	if s == nil || kit == nil || kit.IsPlayer {
		return
	}
	base := s.findCreatureMotion(mapID, instanceID, baseGUID)
	if base == nil {
		return
	}
	passenger := s.findCreatureMotion(mapID, instanceID, passengerGUID)
	if passenger == nil {
		return
	}
	s.smartPassengerBoarded(ctx, base, passengerGUID, oldSeat, false)
	s.triggerVehicleEvent(scripting.VehicleEventOnRemovePassenger, s.luaMotionCreature(base), s.luaMotionCreature(passenger))
}

// fireCreatureVehicleSeatAdd is the creature-passenger twin of
// fireVehicleSeatAdd: the Vehicle::AddPassenger (JoinEvent::Execute)
// hook half of a creature seat switch
// (Vehicle.cpp:883, :895-896, :900-902).
func (s *Server) fireCreatureVehicleSeatAdd(ctx context.Context, mapID, instanceID uint32, kit *VehicleKit, baseGUID, passengerGUID uint64, newSeat int8) {
	if s == nil || kit == nil || kit.IsPlayer {
		return
	}
	base := s.findCreatureMotion(mapID, instanceID, baseGUID)
	if base == nil {
		return
	}
	passenger := s.findCreatureMotion(mapID, instanceID, passengerGUID)
	if passenger == nil {
		return
	}
	s.sendPassengerClearTarget(mapID, instanceID, passengerGUID, passenger.X, passenger.Y, passenger.Z, passenger.PhaseMask)
	s.transferPassengerThreatToVehicleBase(mapID, instanceID, passengerGUID, baseGUID)
	s.smartPassengerBoarded(ctx, base, passengerGUID, newSeat, true)
	s.triggerVehicleEvent(scripting.VehicleEventOnAddPassenger, s.luaMotionCreature(base), s.luaMotionCreature(passenger), newSeat)
}

// handleChangeSeatsOnControlledVehicle processes CMSG_CHANGE_SEATS_ON_CONTROLLED_VEHICLE (0x49B).
// Reference: WorldSession::HandleChangeSeatsOnControlledVehicle (VehicleHandler.cpp:52-127).
// Packet layout: packed vehicle GUID, MovementInfo, packed accessory GUID, int8 seat id.
func (s *session) handleChangeSeatsOnControlledVehicle(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) == 0 {
		return true
	}
	b := protocol.NewReader(payload)
	vehGUID, err := b.ReadPackedGUID()
	if err != nil {
		return true
	}
	// vehicle_base->m_movementInfo = movementInfo (VehicleHandler.cpp:87):
	// stored on the base's mover store before the seat/guid validation
	// below, as in C++ — a creature base's motion store, a player
	// base's session store (the kit's IsPlayer flag discriminates).
	// Stored as parsed (client time), matching C++'s direct assignment
	// rather than the generic path's clock-rewritten store.
	info, err := readMovementInfo(b)
	if err != nil {
		return true
	}
	if s.server != nil {
		baseIsPlayer := false
		for _, loc := range s.server.findVehicleKits(vehGUID) {
			if loc.kit != nil && loc.kit.IsPlayer {
				baseIsPlayer = true
			}
		}
		if baseIsPlayer {
			if baseSess := s.server.findSessionByGUID(vehGUID); baseSess != nil {
				baseSess.setLastMovementInfo(info)
			}
		} else {
			s.server.setCreatureMovementInfo(s.player.Map, s.player.InstanceID, vehGUID, info)
		}
	}
	accessory, err := b.ReadPackedGUID()
	if err != nil {
		return true
	}
	seatByte, err := b.ReadU8()
	if err != nil {
		return true
	}
	seatID := int8(seatByte)

	// if (vehicle_base->GetGUID() != guid) return (VehicleHandler.cpp:116-117).
	if s.player.VehicleGUID == 0 || vehGUID != s.player.VehicleGUID {
		return true
	}

	// Accessory arm (VehicleHandler.cpp:100-104): boarding a seat on another
	// unit's vehicle via Unit::HandleSpellClick(GetPlayer(), seatId).
	if accessory != 0 {
		if s.server != nil {
			if kit := s.server.getVehicleKit(s.player.Map, s.player.InstanceID, s.player.VehicleGUID); kit != nil {
				// The CanSwitchFromSeat pre-gate (VehicleHandler.cpp:63-64)
				// applies to the accessory arm too.
				_, seatInfo, _ := kit.GetSeatForPassenger(s.playerGUID)
				if seatInfo != nil && seatInfo.CanSwitchFromSeat() {
					if targetKit := s.server.getVehicleKit(s.player.Map, s.player.InstanceID, accessory); targetKit != nil && targetKit.HasEmptySeat(seatID) {
						s.spellClickEnterVehicleSeat(ctx, accessory, seatID)
					}
				}
			}
		}
		return true
	}

	// ChangeSeat(-1, seatId > 0): previous/next empty seat (VehicleHandler.cpp:99).
	// The CanSwitchFromSeat pre-gate (VehicleHandler.cpp:63-64) lives inside
	// VehicleKit.SwitchSeat.
	next := seatID > 0
	if s.server != nil {
		if kit := s.server.getVehicleKit(s.player.Map, s.player.InstanceID, s.player.VehicleGUID); kit != nil {
			_, oldSeatInfo, _ := kit.GetSeatForPassenger(s.playerGUID)
			if nextSeat, ok := kit.GetNextEmptySeat(s.player.VehicleSeat, next); ok {
				if assigned, seatInfo, switched := kit.SwitchSeat(s.playerGUID, nextSeat); switched {
					oldSeat := s.player.VehicleSeat
					s.player.VehicleSeat = assigned
					s.sendVehicleSeatControlChange(ctx, kit, oldSeatInfo, seatInfo)
					// Unit::ChangeSeat (Unit.cpp:13133-13170) re-applies the
					// ride aura on the new seat (ChangeAmount), so the old
					// seat's NOT_SELECTABLE flag restores and the new
					// seat's sets (Vehicle.cpp:512-514, :855-856).
					s.applyVehicleSeatNotSelectable(oldSeatInfo, seatInfo, kit.passengerWasUnselectable(s.playerGUID))
					// Vehicle::RemovePassenger (Vehicle.cpp:533): the
					// ChangeAmount unapply behind the switch hands a
					// flying passenger the parachute spell.
					s.castVehicleExitParachute(ctx, kit.VehicleGUID)
					s.fireVehicleSeatSwitch(ctx, kit, oldSeat, assigned)
					s.sendPlayerUpdate()
					// A switch across the CanEnterOrExit boundary moves
					// UsableSeatNum (see SwitchSeat); the base's
					// spell-click flag follows it.
					s.server.syncVehicleBaseNPCFlag(s.player.Map, s.player.InstanceID, s.player.VehicleGUID)
				}
			}
			return true
		}
	}

	// No vehicle kit (VehicleHandler.cpp:56-60): C++ silently drops when the
	// vehicle base is unavailable — the old VehicleSeat++/-- fallback was an
	// invented state mutation with no C++ counterpart.
	return true
}

// handleControllerEjectPassenger processes CMSG_CONTROLLER_EJECT_PASSENGER (0x4A9).
// Reference: WorldSession::HandleEjectPassenger (VehicleHandler.cpp:151-188).
// The ejected GUID is a RAW 8-byte read (VehicleHandler.cpp:159: `data >> guid`
// — ObjectGuid.cpp:76-80 raw semantics, not ReadAsPacked), so Go reads it with
// ReadU64; the old ReadPackedGUID misparsed every eject request.
func (s *session) handleControllerEjectPassenger(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) == 0 {
		return true
	}
	r := protocol.NewReader(payload)
	passGUID, err := r.ReadU64()
	if err != nil || passGUID == 0 {
		return true
	}

	if s.server == nil {
		return true
	}
	// VehicleHandler.cpp:157: the ejector must carry a vehicle kit of their own
	// (Unit::GetVehicleKit returns m_vehicleKit — the kit of a unit that IS a
	// vehicle base, not of a mere passenger). A controlling passenger riding
	// somebody else's vehicle cannot eject in C++; the old code let any
	// CanControl-seat passenger eject.
	kit := s.server.getVehicleKit(s.player.Map, s.player.InstanceID, s.playerGUID)
	if kit == nil {
		return true
	}
	// The ejected unit must ride this same vehicle (VehicleHandler.cpp:169).
	// Creature passengers have no Go model (AddPassenger is player-only), so
	// only player sessions are ejectable here.
	passSess := s.server.findSessionByGUID(passGUID)
	if passSess == nil || passSess.player.Map != s.player.Map || passSess.player.InstanceID != s.player.InstanceID || passSess.player.VehicleGUID != s.playerGUID {
		return true
	}
	_, passengerSeat, _ := kit.GetSeatForPassenger(passGUID)
	if passengerSeat == nil || !passengerSeat.IsEjectable() {
		return true
	}
	passSess.exitVehicle(ctx)
	return true
}

// controlsVehicle reports whether the session's player is the controller of
// the vehicle they ride: the Go bridge for the GetCharmedGUID() gate
// (VehicleHandler.cpp:33-38). The CHARM_TYPE_VEHICLE charm is granted on
// boarding a seat carrying the CAN_CONTROL flag, but only when the vehicle
// base is a creature (VehicleJoinEvent::Execute, Vehicle.cpp:871-881 —
// SetCharmedBy is gated on GetTypeId() == TYPEID_UNIT). A player vehicle
// base never charms its controller, so its dismiss packet dies at the C++
// gate the same way.

// moverGUID returns the GUID the client's movement packets carry: the
// vehicle GUID while driving a control seat (mover =
// _player->GetUnitBeingMoved(), MovementHandler.cpp:261), else the player.
func (s *session) moverGUID() uint64 {
	if s == nil || s.player == nil {
		return 0
	}
	if s.controlsVehicle() {
		return s.player.VehicleGUID
	}
	return s.playerGUID
}

// possessedCreatureGUID resolves Unit::GetUnitBeingMoved for a possessing
// player (Player::SetClientControl -> SetMovedUnit, Player.cpp:24239-24272):
// CHARM_TYPE_POSSESS hands the client control of the possessed unit, so its
// knockback acks carry the creature GUID, not the player GUID. Plain charm
// never calls SetClientControl, so only Possessed motions qualify; an
// ambiguous pair returns 0 and callers fail closed, mirroring
// charmedCreatureGUID. It intentionally does NOT feed moverGUID: the
// item/object/spell mover gates assume self-movement, and widening them to
// possessed creatures is a separate unit.
func (s *session) possessedCreatureGUID() uint64 {
	if s == nil || s.server == nil || s.player == nil {
		return 0
	}
	s.server.motionMu.Lock()
	defer s.server.motionMu.Unlock()
	var found uint64
	for _, motion := range s.server.motionMapLocked(s.player.Map, s.player.InstanceID) {
		if motion != nil && motion.Charmed && motion.Possessed && motion.CharmerGUID == s.playerGUID {
			if found != 0 {
				return 0
			}
			found = motion.GUID
		}
	}
	return found
}

func (s *session) controlsVehicle() bool {
	if s == nil || s.player == nil || s.server == nil || s.player.VehicleGUID == 0 {
		return false
	}
	kit := s.server.getVehicleKit(s.player.Map, s.player.InstanceID, s.player.VehicleGUID)
	if kit == nil || kit.IsPlayer {
		return false
	}
	_, seatInfo, _ := kit.GetSeatForPassenger(s.playerGUID)
	return seatInfo != nil && seatInfo.CanControl()
}

// handleDismissControlledVehicle processes CMSG_DISMISS_CONTROLLED_VEHICLE (0x46D).
// Reference: WorldSession::HandleDismissControlledVehicle (VehicleHandler.cpp:27-50).
// Packet layout: packed charmed-vehicle GUID, MovementInfo.
func (s *session) handleDismissControlledVehicle(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	// The GetCharmedGUID() gate (VehicleHandler.cpp:33-38) runs before any
	// packet read: a packet from a non-charming player is rfinish'd out and
	// never reaches ExitVehicle. The Go bridge is the control-seat check — a
	// passenger on a non-control seat (or anyone off a vehicle) is dropped
	// the same way.
	if !s.controlsVehicle() {
		return true
	}
	if len(payload) > 0 {
		b := protocol.NewReader(payload)
		if _, err := b.ReadPackedGUID(); err == nil {
			// _player->m_movementInfo = mi (VehicleHandler.cpp:47), before
			// ExitVehicle — so Unit::IsFalling reads the dismissed info's
			// FALLING/FALLING_FAR flags until the next movement packet.
			// The info runs the same ReadMovementInfo sanitize the
			// knockback/not-active-mover arms apply before it is stored.
			if info, err := readMovementInfo(b); err == nil {
				info.Flags = s.sanitizeMovementFlags(info.Flags)
				s.setLastMovementInfo(info)
				s.isFalling = info.Flags&movementFalling != 0
				s.isFallingFar = info.Flags&movementFallingFar != 0
			}
		}
	}
	// Unit::ExitVehicle (Unit.cpp:13165-13182): the exit strips the
	// ride aura from the base, whose unapply runs _ExitVehicle. The
	// strip runs first so the aura is gone when the passenger leaves;
	// the direct exit below is the aura-less-board fallback — a
	// passenger seated without a ride aura (Lua AddPassenger) still
	// exits, and a second exitVehicle no-ops on the cleared VehicleGUID.
	if s.server != nil && s.player.VehicleGUID != 0 {
		s.server.removeVehicleControlAuraFromBase(s.player.Map, s.player.InstanceID, s.player.VehicleGUID, s.playerGUID)
	}
	s.exitVehicle(ctx)
	return true
}

// handlePlayerVehicleEnter processes CMSG_PLAYER_VEHICLE_ENTER (0x4A8).
// Reference: WorldSession::HandleEnterPlayerVehicle (VehicleHandler.cpp:129-149).
func (s *session) handlePlayerVehicleEnter(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) == 0 {
		return true
	}
	r := protocol.NewReader(payload)
	// VehicleHandler.cpp:132 reads the player GUID raw (`data >> guid` —
	// ObjectGuid.cpp:76-80 raw 8-byte semantics, not ReadAsPacked), so Go
	// reads it with ReadU64; the old ReadPackedGUID misparsed the packet and
	// player-vehicle entry never fired.
	vehGUID, err := r.ReadU64()
	if err != nil || vehGUID == 0 {
		return true
	}
	if s.server == nil || IsArenaMap(s.player.Map) || s.server.findArenaStateForPlayer(s.playerGUID) != nil {
		return true
	}
	target := s.server.findSessionByGUID(vehGUID)
	if !playerVehicleEntryAllowed(s, target) {
		return true
	}
	kit := s.server.getVehicleKit(target.player.Map, target.player.InstanceID, vehGUID)
	if kit == nil || !kit.IsPlayer {
		return true
	}
	// Unit::EnterVehicle (Unit.cpp:13073-13078): boarding a player
	// vehicle casts the ride spell (VEHICLE_SPELL_RIDE_HARDCODED,
	// 46598) triggered with bp0 = seatId+1 = 0, whose apply leg boards
	// at amount-1 = -1; the direct enterVehicle call skipped the ride
	// aura the board rides on.
	s.castSpellDirect(ctx, wotlk.VehicleSpellRideHardcoded, vehGUID)
	return true
}

func playerVehicleEntryAllowed(player, target *session) bool {
	if player == nil || target == nil || player.player == nil || target.player == nil || player != target && (player.groupID == 0 || player.groupID != target.groupID) {
		return false
	}
	if player.player.Map != target.player.Map || player.player.InstanceID != target.player.InstanceID {
		return false
	}
	interactionDistance := 5.0 // INTERACTION_DISTANCE, flat (ObjectDefines.h:24); no combat-reach fold.
	return distance3D(player.player.X, player.player.Y, player.player.Z, target.player.X, target.player.Y, target.player.Z) <= interactionDistance
}

// handleRequestVehicleExit processes CMSG_REQUEST_VEHICLE_EXIT (0x476).
// Reference: WorldSession::HandleRequestVehicleExit (VehicleHandler.cpp:190-205).
func (s *session) handleRequestVehicleExit(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	if s.server != nil && s.player.VehicleGUID != 0 {
		if kit := s.server.getVehicleKit(s.player.Map, s.player.InstanceID, s.player.VehicleGUID); kit != nil {
			_, seatInfo, _ := kit.GetSeatForPassenger(s.playerGUID)
			if seatInfo != nil && !seatInfo.CanEnterOrExit() {
				return true
			}
		}
	}
	// Unit::ExitVehicle (Unit.cpp:13165-13182): the exit strips the
	// ride aura from the base, whose unapply runs _ExitVehicle. The
	// strip runs first so the aura is gone when the passenger leaves;
	// the direct exit below is the aura-less-board fallback — a
	// passenger seated without a ride aura (Lua AddPassenger) still
	// exits, and a second exitVehicle no-ops on the cleared VehicleGUID.
	if s.server != nil && s.player.VehicleGUID != 0 {
		s.server.removeVehicleControlAuraFromBase(s.player.Map, s.player.InstanceID, s.player.VehicleGUID, s.playerGUID)
	}
	s.exitVehicle(ctx)
	return true
}

// handleRequestVehicleNextSeat processes CMSG_REQUEST_VEHICLE_NEXT_SEAT (0x478).
// Reference: WorldSession::HandleChangeSeatsOnControlledVehicle (VehicleHandler.cpp:77).
func (s *session) handleRequestVehicleNextSeat(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	if s.server != nil && s.player.VehicleGUID != 0 {
		if kit := s.server.getVehicleKit(s.player.Map, s.player.InstanceID, s.player.VehicleGUID); kit != nil {
			_, oldSeatInfo, _ := kit.GetSeatForPassenger(s.playerGUID)
			if nextSeat, ok := kit.GetNextEmptySeat(s.player.VehicleSeat, true); ok {
				if assigned, seatInfo, switched := kit.SwitchSeat(s.playerGUID, nextSeat); switched {
					oldSeat := s.player.VehicleSeat
					s.player.VehicleSeat = assigned
					s.sendVehicleSeatControlChange(ctx, kit, oldSeatInfo, seatInfo)
					// Unit::ChangeSeat (Unit.cpp:13133-13170) re-applies the
					// ride aura on the new seat (ChangeAmount), so the old
					// seat's NOT_SELECTABLE flag restores and the new
					// seat's sets (Vehicle.cpp:512-514, :855-856).
					s.applyVehicleSeatNotSelectable(oldSeatInfo, seatInfo, kit.passengerWasUnselectable(s.playerGUID))
					// Vehicle::RemovePassenger (Vehicle.cpp:533): the
					// ChangeAmount unapply behind the switch hands a
					// flying passenger the parachute spell.
					s.castVehicleExitParachute(ctx, kit.VehicleGUID)
					s.fireVehicleSeatSwitch(ctx, kit, oldSeat, assigned)
					s.sendPlayerUpdate()
					// A switch across the CanEnterOrExit boundary moves
					// UsableSeatNum (see SwitchSeat); the base's
					// spell-click flag follows it.
					s.server.syncVehicleBaseNPCFlag(s.player.Map, s.player.InstanceID, s.player.VehicleGUID)
				}
			}
			return true
		}
	}
	// VehicleHandler.cpp:56-60: no vehicle base -> rfinish + return, no
	// seat mutation and no update sent.
	return true
}

// handleRequestVehiclePrevSeat processes CMSG_REQUEST_VEHICLE_PREV_SEAT (0x477).
// Reference: WorldSession::HandleChangeSeatsOnControlledVehicle (VehicleHandler.cpp:74).
func (s *session) handleRequestVehiclePrevSeat(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	if s.server != nil && s.player.VehicleGUID != 0 {
		if kit := s.server.getVehicleKit(s.player.Map, s.player.InstanceID, s.player.VehicleGUID); kit != nil {
			_, oldSeatInfo, _ := kit.GetSeatForPassenger(s.playerGUID)
			if prevSeat, ok := kit.GetNextEmptySeat(s.player.VehicleSeat, false); ok {
				if assigned, seatInfo, switched := kit.SwitchSeat(s.playerGUID, prevSeat); switched {
					oldSeat := s.player.VehicleSeat
					s.player.VehicleSeat = assigned
					s.sendVehicleSeatControlChange(ctx, kit, oldSeatInfo, seatInfo)
					// Unit::ChangeSeat (Unit.cpp:13133-13170) re-applies the
					// ride aura on the new seat (ChangeAmount), so the old
					// seat's NOT_SELECTABLE flag restores and the new
					// seat's sets (Vehicle.cpp:512-514, :855-856).
					s.applyVehicleSeatNotSelectable(oldSeatInfo, seatInfo, kit.passengerWasUnselectable(s.playerGUID))
					// Vehicle::RemovePassenger (Vehicle.cpp:533): the
					// ChangeAmount unapply behind the switch hands a
					// flying passenger the parachute spell.
					s.castVehicleExitParachute(ctx, kit.VehicleGUID)
					s.fireVehicleSeatSwitch(ctx, kit, oldSeat, assigned)
					s.sendPlayerUpdate()
					// A switch across the CanEnterOrExit boundary moves
					// UsableSeatNum (see SwitchSeat); the base's
					// spell-click flag follows it.
					s.server.syncVehicleBaseNPCFlag(s.player.Map, s.player.InstanceID, s.player.VehicleGUID)
				}
			}
			return true
		}
	}
	// VehicleHandler.cpp:56-60: no vehicle base -> rfinish + return, no
	// seat mutation and no update sent.
	return true
}

// handleRequestVehicleSwitchSeat processes CMSG_REQUEST_VEHICLE_SWITCH_SEAT (0x479).
// Reference: WorldSession::HandleChangeSeatsOnControlledVehicle (VehicleHandler.cpp:108-123).
// applyVehicleSeatNotSelectable runs the NOT_SELECTABLE legs of a seat
// change (Unit::ChangeSeat, Unit.cpp:13133-13170, via ChangeAmount): the
// unapply's RemovePassenger restores the flag (Vehicle.cpp:512-514) and
// the re-board's JoinEvent sets it (Vehicle.cpp:855-856). Either half
// may be nil when only the other leg runs.
func (s *session) applyVehicleSeatNotSelectable(oldSeatInfo, seatInfo *wotlk.VehicleSeatEntry, wasUnselectable bool) {
	if s == nil || s.player == nil {
		return
	}
	if oldSeatInfo != nil && oldSeatInfo.HasFlag(wotlk.VehicleSeatFlagPassengerNotSelectable) && !wasUnselectable {
		s.player.UnitFlags &^= 0x02000000 // UNIT_FLAG_NOT_SELECTABLE
	}
	if seatInfo != nil && seatInfo.HasFlag(wotlk.VehicleSeatFlagPassengerNotSelectable) {
		s.player.UnitFlags |= 0x02000000 // UNIT_FLAG_NOT_SELECTABLE
	}
}

func (s *session) handleRequestVehicleSwitchSeat(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 1 {
		return true
	}
	var seat int8
	// Packet layout is a packed vehicle GUID followed by the int8 seat id
	// (VehicleHandler.cpp:111-114: `recvData >> guid.ReadAsPacked()` then
	// `recvData >> seatId`).
	r := protocol.NewReader(payload)
	vehGUID, err := r.ReadPackedGUID()
	if err != nil {
		return true
	}
	if sByte, err := r.ReadU8(); err == nil {
		seat = int8(sByte)
	} else {
		return true
	}

	// VehicleHandler.cpp:54-59: with no vehicle base the whole packet is
	// dropped (rfinish) — never a seat assignment.
	if s.player.VehicleGUID == 0 {
		return true
	}
	// VehicleHandler.cpp:108-121: when the packet names a different vehicle
	// base, C++ boards the named unit's vehicle through
	// Unit::HandleSpellClick(GetPlayer(), seatId).
	if vehGUID != s.player.VehicleGUID {
		if s.server != nil {
			if kit := s.server.getVehicleKit(s.player.Map, s.player.InstanceID, s.player.VehicleGUID); kit != nil {
				// CanSwitchFromSeat pre-gate (VehicleHandler.cpp:63-64).
				_, seatInfo, _ := kit.GetSeatForPassenger(s.playerGUID)
				if seatInfo != nil && seatInfo.CanSwitchFromSeat() {
					if targetKit := s.server.getVehicleKit(s.player.Map, s.player.InstanceID, vehGUID); targetKit != nil && targetKit.HasEmptySeat(seat) {
						s.spellClickEnterVehicleSeat(ctx, vehGUID, seat)
					}
				}
			}
		}
		return true
	}

	if s.server != nil {
		if kit := s.server.getVehicleKit(s.player.Map, s.player.InstanceID, s.player.VehicleGUID); kit != nil {
			_, oldSeatInfo, _ := kit.GetSeatForPassenger(s.playerGUID)
			if assigned, seatInfo, switched := kit.SwitchSeat(s.playerGUID, seat); switched {
				oldSeat := s.player.VehicleSeat
				s.player.VehicleSeat = assigned
				s.sendVehicleSeatControlChange(ctx, kit, oldSeatInfo, seatInfo)
				// Unit::ChangeSeat (Unit.cpp:13133-13170) re-applies the
				// ride aura on the new seat (ChangeAmount), so the old
				// seat's NOT_SELECTABLE flag restores and the new
				// seat's sets (Vehicle.cpp:512-514, :855-856).
				s.applyVehicleSeatNotSelectable(oldSeatInfo, seatInfo, kit.passengerWasUnselectable(s.playerGUID))
				// Vehicle::RemovePassenger (Vehicle.cpp:533): the
				// ChangeAmount unapply behind the switch hands a
				// flying passenger the parachute spell.
				s.castVehicleExitParachute(ctx, kit.VehicleGUID)
				s.fireVehicleSeatSwitch(ctx, kit, oldSeat, assigned)
				s.sendPlayerUpdate()
				// A switch across the CanEnterOrExit boundary moves
				// UsableSeatNum (see SwitchSeat); the base's
				// spell-click flag follows it.
				s.server.syncVehicleBaseNPCFlag(s.player.Map, s.player.InstanceID, s.player.VehicleGUID)
			}
			return true
		}
	}

	// Unit::ChangeSeat (Unit.cpp:13135-13136): with no vehicle kit the change
	// is a no-op — never a seat assignment.
	return true
}

// spellClickEnterVehicleSeat mirrors the seatId arm of Unit::HandleSpellClick
// (Unit.cpp:12982-13045): the vehicle seat-change handlers board the player
// onto a seat of another unit's vehicle through its npc_spellclick_spells
// rows (VehicleHandler.cpp:100-104, 118-121). The caller runs the
// HasEmptySeat gate; the seat amount rides the spell cast as
// SPELLVALUE_BASE_POINT0+i = seatId+1 on the CONTROL_VEHICLE effect
// (Unit.cpp:13029-13031), and the aura apply leg boards the caster at
// amount-1 (spells.go HandleAuraControlVehicle leg).
func (s *session) spellClickEnterVehicleSeat(ctx context.Context, vehUnitGUID uint64, seatID int8) {
	if s == nil || s.player == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return
	}
	kit := s.server.getVehicleKit(s.player.Map, s.player.InstanceID, vehUnitGUID)
	// Unit::HandleSpellClick (Unit.cpp:12985): the click entry is the kit's
	// creature entry; entry 0 has no npc_spellclick_spells rows, matching the
	// C++ empty-bounds no-op.
	if kit == nil || kit.CreatureEntry == 0 {
		return
	}
	clickEntry := kit.CreatureEntry

	// SpellClickInfo::IsFitToRequirements (ObjectMgr.cpp:194-224), following
	// the established handleSpellClick approximations: userType >=
	// SPELL_CLICK_USER_MAX (4) is skipped at DB load in C++; for creature
	// units FRIEND means not hostile and RAID/PARTY never match (creatures
	// are never in groups); the pet arm hits the this==unit case so every
	// userType passes.
	isPet := s.player.PetGUID != 0 && vehUnitGUID == s.player.PetGUID
	motion := s.findCreatureMotion(vehUnitGUID)

	rows, err := s.server.WorldStore.DB.QueryContext(ctx, "SELECT spell_id, cast_flags, user_type FROM npc_spellclick_spells WHERE npc_entry = ?", clickEntry)
	if err != nil {
		return
	}
	defer rows.Close()

	type spellClick struct {
		spellID   uint32
		castFlags uint8
		userType  uint8
	}
	var clicks []spellClick
	for rows.Next() {
		var sp, cf, ut uint32
		if err := rows.Scan(&sp, &cf, &ut); err == nil && sp > 0 {
			clicks = append(clicks, spellClick{spellID: sp, castFlags: uint8(cf), userType: uint8(ut)})
		}
	}

	haveData := s.server.Data != nil
	for _, click := range clicks {
		if click.userType >= 4 {
			continue
		}
		if !isPet && motion != nil {
			switch click.userType {
			case 1: // SPELL_CLICK_USER_FRIEND
				player := playerPos{Race: s.player.Race, Class: s.player.Class, FactionTemplate: s.effectiveFactionTemplate(s.player.Race), Reputations: playerReputationMap(s.player.Reputations), Sess: s}
				if s.server.isHostileFaction(motion.Faction, player) {
					continue
				}
			case 2, 3: // SPELL_CLICK_USER_RAID / SPELL_CLICK_USER_PARTY
				continue
			}
		}
		// ConditionMgr::IsObjectMeetingSpellClickConditions
		// (ConditionMgr.cpp:1008).
		if meets, err := s.meetSpellClickConditions(ctx, clickEntry, click.spellID, vehUnitGUID); err != nil || !meets {
			continue
		}
		// Seat arm (Unit.cpp:13006-13024): the spell must carry a
		// SPELL_AURA_CONTROL_VEHICLE effect — C++ logs a sql error and
		// continues otherwise.
		if !haveData {
			continue
		}
		spell, found, err := s.server.Data.Spell(click.spellID)
		if err != nil || !found || len(spell.Effects) == 0 {
			continue
		}
		effIndex := -1
		for i, eff := range spell.Effects {
			if eff.Aura == spellAuraControlVehicle {
				effIndex = i
				break
			}
		}
		if effIndex < 0 {
			continue
		}
		if click.castFlags&0x01 == 0 {
			// NPC_CLICK_CAST_CASTER_CLICKER unset: the creature casts. With
			// caster==target the C++ aura apply is a no-op
			// (SpellAuraEffects.cpp:2975); the reverse arm (the creature
			// boarding the clicker's own kit) has no Go model —
			// AddPassenger is player-only — so the row is skipped.
			continue
		}
		targetGUID := vehUnitGUID
		if click.castFlags&0x02 != 0 { // NPC_CLICK_CAST_TARGET_CLICKER
			targetGUID = s.playerGUID
		}
		// Only effect effIndex is overridden; the other base points ride
		// the inverse of the funnel's DieSides adjustment
		// (castSpellDirectWithOverrides, Spell.cpp:7871-7877) so they land
		// back on their DBC values.
		bp := make([]int32, len(spell.Effects))
		for j, e := range spell.Effects {
			bp[j] = e.BasePoints
			if e.DieSides != 0 {
				bp[j]++
			}
		}
		bp[effIndex] = int32(seatID) + 1
		if spell.Effects[effIndex].DieSides != 0 {
			bp[effIndex]++
		}
		s.castSpellDirectWithBasePoints(ctx, click.spellID, targetGUID, bp)
	}
}
