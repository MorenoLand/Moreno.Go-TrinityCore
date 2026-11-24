package world

import (
	"context"
	"fmt"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
)

// luaVehicleRef is the live handle behind a Lua Vehicle object
// (VehicleMethods.h). It re-resolves the kit on every call instead of
// holding a *VehicleKit: a removed kit reads as gone (reads nil/0/false,
// writes no-op), matching Eluna's Invalidate-on-Remove without a
// dedicated invalidation flag. The kit is normally keyed on the base's
// current map/instance, but a player base that far-teleports leaves its
// kit keyed on the origin map/instance (the findVehicleKits comment in
// vehicle.go), so a same-key miss falls back to the GUID scan, the same
// twin the logout path uses. The only Eluna entry point to a Vehicle
// object is Unit:GetVehicleKit (LuaFunctions.cpp registers it): a unit
// with no kit yields nil, never a vehicle object over a kit-less base.
// The Unit:GetVehicle / GetVehicleBase / GetTransSeat twins (commented
// out in LuaFunctions.cpp) are built on the player, motion-creature and
// session-creature surfaces beside GetVehicleKit; they resolve through
// luaVehicleObject / luaUnitObjectForGUID on the VehicleGUID stamp.
type luaVehicleRef struct {
	server     *Server
	mapID      uint32
	instanceID uint32
	baseGUID   uint64
}

// resolve returns the live kit and the key it was found under. The
// stored key wins; the GUID scan covers the teleported-base stale key.
func (r *luaVehicleRef) resolve() (*VehicleKit, uint32, uint32) {
	if r == nil || r.server == nil || r.baseGUID == 0 {
		return nil, 0, 0
	}
	if kit := r.server.getVehicleKit(r.mapID, r.instanceID, r.baseGUID); kit != nil {
		return kit, r.mapID, r.instanceID
	}
	for _, loc := range r.server.findVehicleKits(r.baseGUID) {
		return loc.kit, loc.key.MapID, loc.key.InstanceID
	}
	return nil, 0, 0
}

// luaVehicleObject builds the Eluna Vehicle object for the kit on
// baseGUID, or nil when no kit exists. GetEntry is the Vehicle.dbc id
// (Vehicle::GetVehicleInfo()->ID, Vehicle.h): createVehicleKit stores
// exactly that id as VehicleID, so no separate DBC lookup is needed.
// GetOwner is Vehicle::GetBase, the base unit itself, resolved through
// the unit-object funnel (player sessions first, then motions).
func (s *Server) luaVehicleObject(mapID, instanceID uint32, baseGUID uint64) *scripting.Object {
	if s == nil || baseGUID == 0 {
		return nil
	}
	ref := &luaVehicleRef{server: s, mapID: mapID, instanceID: instanceID, baseGUID: baseGUID}
	if kit, _, _ := ref.resolve(); kit == nil {
		return nil
	}
	methods := map[string]scripting.ObjectMethod{}
	methods["GetObjectType"] = luaNoArgs(func() any { return "Vehicle" })
	methods["GetOwner"] = func(_ context.Context, _ []any) ([]any, error) {
		kit, effMap, effInstance := ref.resolve()
		if kit == nil {
			return []any{nil}, nil
		}
		return []any{s.luaUnitObjectForGUID(effMap, effInstance, baseGUID)}, nil
	}
	methods["GetEntry"] = func(_ context.Context, _ []any) ([]any, error) {
		kit, _, _ := ref.resolve()
		if kit == nil {
			return []any{uint32(0)}, nil
		}
		return []any{kit.VehicleID}, nil
	}
	methods["GetPassenger"] = func(_ context.Context, args []any) ([]any, error) {
		seat, err := luaVehicleSeatArg(args, 0)
		if err != nil {
			return nil, err
		}
		kit, effMap, effInstance := ref.resolve()
		if kit == nil {
			return []any{nil}, nil
		}
		passengerGUID := kit.GetPassenger(seat)
		if passengerGUID == 0 {
			return []any{nil}, nil
		}
		return []any{s.luaUnitObjectForGUID(effMap, effInstance, passengerGUID)}, nil
	}
	methods["IsOnBoard"] = func(_ context.Context, args []any) ([]any, error) {
		passengerGUID, err := luaVehicleUnitArg(args, 0)
		if err != nil {
			return nil, err
		}
		kit, effMap, effInstance := ref.resolve()
		if kit == nil {
			return []any{false}, nil
		}
		return []any{s.vehicleCarriesPassenger(effMap, effInstance, baseGUID, passengerGUID)}, nil
	}
	methods["AddPassenger"] = func(ctx context.Context, args []any) ([]any, error) {
		passengerGUID, err := luaVehicleUnitArg(args, 0)
		if err != nil {
			return nil, err
		}
		seat, err := luaVehicleSeatArg(args, 1)
		if err != nil {
			return nil, err
		}
		s.luaVehicleAddPassenger(ctx, ref, passengerGUID, seat)
		return nil, nil
	}
	methods["RemovePassenger"] = func(ctx context.Context, args []any) ([]any, error) {
		passengerGUID, err := luaVehicleUnitArg(args, 0)
		if err != nil {
			return nil, err
		}
		s.luaVehicleRemovePassenger(ctx, ref, passengerGUID)
		return nil, nil
	}
	return &scripting.Object{Type: "Vehicle", Fields: map[string]any{"GUID": baseGUID}, Methods: methods}
}

// luaVehicleSeatArg is the CHECKVAL<int8> twin for the Vehicle seat
// arguments: a Lua number must fit int8, where -1 means first available
// (the kit AddPassenger scan) for AddPassenger and no seat for
// GetPassenger (Vehicle::GetPassenger returns null for unknown seats).
func luaVehicleSeatArg(args []any, index int) (int8, error) {
	value, err := luaInt64Arg(args, index)
	if err != nil {
		return 0, err
	}
	if value < -128 || value > 127 {
		return 0, fmt.Errorf("argument %d must be int8", index+1)
	}
	return int8(value), nil
}

// luaVehicleUnitArg is the CHECKOBJ<Unit> twin: the passenger argument
// must be a Lua unit object carrying a GUID field (player and creature
// objects both do). A missing or non-unit argument errors, as Eluna's
// CHECKOBJ does, rather than boarding GUID 0.
func luaVehicleUnitArg(args []any, index int) (uint64, error) {
	if index >= len(args) {
		return 0, fmt.Errorf("argument %d is required", index+1)
	}
	obj, ok := args[index].(*scripting.Object)
	if !ok || obj == nil {
		return 0, fmt.Errorf("argument %d must be a unit", index+1)
	}
	guid, found := objectUint64Field(obj, "GUID")
	if !found || guid == 0 {
		return 0, fmt.Errorf("argument %d must be a unit", index+1)
	}
	return guid, nil
}

// vehicleCarriesPassenger is Unit::IsOnVehicle (Unit.cpp:12074-12077)
// for a passenger against a base: the passenger's VehicleGUID stamp
// equals the base GUID. Player sessions carry the stamp on the player
// row, creature motions on the motion; the map/instance are the kit's
// effective key, so a creature passenger is looked up where the kit
// lives, not where the caller assumed.
func (s *Server) vehicleCarriesPassenger(mapID, instanceID uint32, baseGUID, passengerGUID uint64) bool {
	if s == nil || baseGUID == 0 || passengerGUID == 0 {
		return false
	}
	if sess := s.findSessionByGUID(passengerGUID); sess != nil && sess.player != nil {
		return sess.player.VehicleGUID == baseGUID
	}
	if motion := s.findCreatureMotion(mapID, instanceID, passengerGUID); motion != nil {
		return motion.VehicleGUID == baseGUID
	}
	return false
}

// luaVehicleAddPassenger bridges Vehicle::AddPassenger
// (Vehicle.cpp:426-484) through the two existing boarding funnels.
// Player passengers go through enterVehicle (the _EnterVehicle twin:
// occupied-seat eject, control/pet legs, ev5 for creature bases), which
// looks the kit up on the passenger's own map/instance, so a passenger
// on another map than the kit is a no-op rather than a cross-map board.
// Creature passengers go through the kit call directly (the accessory
// install twin): an occupied specific seat ejects its occupant first
// (Vehicle.cpp:471-478, the ejectVehicleSeatOccupant twin),
// an already-seated passenger keeps its seat, and a passenger seated on
// another vehicle is unseated from it first (the _EnterVehicle
// exit-first arm), with ev6 on the old creature base and ev5 on the new
// one, matching the RemovePassenger/AddPassenger event sequence.
// Relocation rides the base's own fan-out (creature sync, player
// relocate) so the new passenger lands on its seat offsets at once.
func (s *Server) luaVehicleAddPassenger(ctx context.Context, ref *luaVehicleRef, passengerGUID uint64, seat int8) {
	if s == nil || ref == nil {
		return
	}
	kit, effMap, effInstance := ref.resolve()
	if kit == nil {
		return
	}
	if sess := s.findSessionByGUID(passengerGUID); sess != nil && sess.player != nil {
		if sess.player.Map != effMap || sess.player.InstanceID != effInstance {
			return
		}
		sess.enterVehicle(ctx, ref.baseGUID, seat)
		return
	}
	motion := s.findCreatureMotion(effMap, effInstance, passengerGUID)
	if motion == nil {
		return
	}

	// VehicleJoinEvent::Execute (Vehicle.cpp:808-813): a passenger
	// that died before the board aborts instead of seating.
	if motion.Health <= 0 {
		return
	}
	// Unit::_EnterVehicle (Unit.cpp:13089): a passenger that IS the
	// vehicle base — the base kit resolved above, so a passenger whose
	// GUID is the base GUID is its own kit — or whose vehicle the base
	// already rides never boards. The cycle leg needs the passenger's
	// own kit (vehicle->GetBase()->IsOnVehicle(this), Unit.cpp:12074-12077):
	// the base seated in it, of either kind.
	if ref.baseGUID == passengerGUID {
		return
	}
	if ownKit := s.getVehicleKit(effMap, effInstance, passengerGUID); ownKit != nil {
		if baseMotion := s.findCreatureMotion(effMap, effInstance, ref.baseGUID); baseMotion != nil && baseMotion.VehicleGUID == passengerGUID {
			return
		} else if baseSess := s.findSessionByGUID(ref.baseGUID); baseSess != nil && baseSess.player != nil && baseSess.player.VehicleGUID == passengerGUID {
			return
		}
	}
	if motion.VehicleGUID != 0 && motion.VehicleGUID != ref.baseGUID {
		s.luaVehicleRemovePassenger(ctx, &luaVehicleRef{server: s, mapID: effMap, instanceID: effInstance, baseGUID: motion.VehicleGUID}, passengerGUID)
	}
	// VehicleJoinEvent::Execute (Vehicle.cpp:813-815): a passenger
	// already on a vehicle exits it first — including a same-base
	// seat change, where the JoinEvent exits then boards the
	// requested seat. The unseat clears VehicleGUID, so the eject
	// below still runs for the new seat. Same-seat re-entry keeps
	// the seat (Unit.cpp:13095-13096): kit.AddPassenger returns
	// the current seat, so the board below no-ops into it.
	if motion.VehicleGUID == ref.baseGUID && seat >= 0 && seat != motion.VehicleSeat {
		s.luaVehicleRemovePassenger(ctx, ref, passengerGUID)
	}
	// Vehicle::AddPassenger (Vehicle.cpp:471-478): boarding a specific
	// occupied seat ejects its current occupant first, of either kind;
	// the kit allocator fails on an occupied seat without the eject.
	// Skipped when already seated on this base on the requested seat:
	// the board no-ops into the current seat, so there is no seat to take.
	if seat >= 0 && motion.VehicleGUID != ref.baseGUID {
		s.ejectVehicleSeatOccupant(ctx, effMap, effInstance, kit, ref.baseGUID, seat, passengerGUID)
	}
	wasUnselectable := motion.UnitFlags&0x02000000 != 0
	assigned, seatInfo, ok := kit.AddPassenger(passengerGUID, seat, wasUnselectable)
	if !ok {
		// VehicleJoinEvent::Abort (Vehicle.cpp:937, :943-944): a
		// failed board strips the control-vehicle aura from the
		// base and despawns an in-world accessory passenger.
		s.removeVehicleControlAuraFromBase(effMap, effInstance, ref.baseGUID, passengerGUID)
		if motion.VehicleAccessoryMinion {
			s.expireTempSummonGuardian(ctx, motion)
		}
		return
	}
	s.motionMu.Lock()
	motion.VehicleGUID = ref.baseGUID
	motion.VehicleSeat = assigned
	motion.Moving = false
	s.motionMu.Unlock()
	// VehicleJoinEvent::Execute (Vehicle.cpp:825-832): boarding the
	// last usable seat clears the base's spell-click flag.
	s.syncVehicleBaseNPCFlag(effMap, effInstance, ref.baseGUID)
	// VehicleJoinEvent::Execute (Vehicle.cpp:895-896): every creature
	// the passenger threatens gains the vehicle base on its threat list
	// with the passenger's threat value, ahead of the AI PassengerBoarded
	// / OnAddPassenger hooks below. Runs for a player base too — C++
	// transfers to Target->GetBase() regardless of its type.
	// VehicleJoinEvent::Execute (Vehicle.cpp:883): the board breaks
	// the passenger's target for everyone in range.
	// VehicleJoinEvent::Execute (Vehicle.cpp:836-837): the board
	// interrupts the passenger's non-melee casts and strips its
	// mount aura, ahead of the clear-target below.
	s.interruptBoardingCreaturePassenger(motion)
	s.sendPassengerClearTarget(effMap, effInstance, passengerGUID, motion.X, motion.Y, motion.Z, motion.PhaseMask)
	// VehicleJoinEvent::Execute (Vehicle.cpp:884, :887-892): the
	// board roots the passenger and runs the seat-offset spline
	// with the transport-enter flag, ahead of the threat transfer
	// below; a base that never resolves still roots (the root has
	// no position dependency) and skips the spline.
	_, _, seatAddon := kit.GetSeatForPassenger(passengerGUID)
	// VehicleJoinEvent::Execute (Vehicle.cpp:865-869): the board
	// stamps the passenger's transport with the seat offsets.
	s.motionMu.Lock()
	stampCreatureVehicleTransport(motion, ref.baseGUID, assigned, seatInfo, seatAddon)
	s.motionMu.Unlock()
	baseX, baseY, baseZ, baseO, hasBase := float32(0), float32(0), float32(0), float32(0), false
	if base := s.findCreatureMotion(effMap, effInstance, ref.baseGUID); base != nil {
		baseX, baseY, baseZ, baseO, hasBase = base.X, base.Y, base.Z, base.Orientation, true
	} else if baseSess := s.findSessionByGUID(ref.baseGUID); baseSess != nil && baseSess.player != nil {
		baseX, baseY, baseZ, baseO, hasBase = baseSess.player.X, baseSess.player.Y, baseSess.player.Z, baseSess.player.Orientation, true
	}
	s.rootAndSplineBoardingCreaturePassenger(motion, ref.baseGUID, seatInfo, seatAddon, baseX, baseY, baseZ, baseO, hasBase)
	s.transferPassengerThreatToVehicleBase(effMap, effInstance, passengerGUID, ref.baseGUID)
	if base := s.findCreatureMotion(effMap, effInstance, ref.baseGUID); base != nil {
		// VehicleJoinEvent::Execute (Vehicle.cpp:900-902): the AI
		// PassengerBoarded(true) call precedes the OnAddPassenger
		// script hook.
		s.smartPassengerBoarded(ctx, base, passengerGUID, assigned, true)
		s.triggerVehicleEvent(scripting.VehicleEventOnAddPassenger, s.luaMotionCreature(base), s.luaMotionCreature(motion), assigned)
		s.syncCreatureVehiclePassengers(base)
		return
	}
	if baseSess := s.findSessionByGUID(ref.baseGUID); baseSess != nil && baseSess.player != nil {
		s.relocatePassengers(effMap, effInstance, ref.baseGUID, baseSess.player.X, baseSess.player.Y, baseSess.player.Z, baseSess.player.Orientation)
	}
}

// luaVehicleRemovePassenger bridges Vehicle::RemovePassenger
// (Vehicle.cpp:498-543) for one passenger. Player passengers go through
// exitVehicle (the _ExitVehicle twin) only when seated on this base; a
// player seated elsewhere is untouched. Creature passengers unseat
// through the kit call with ev6 on a creature base, the
// exitCreatureVehiclePassengers event shape without its kit-drop half:
// a minion accessory is NOT unsummoned here (that fate belongs to
// Uninstall/kit-drop, not to a single RemovePassenger), it simply
// unseats and stays in the world at its last relocated position.
func (s *Server) luaVehicleRemovePassenger(ctx context.Context, ref *luaVehicleRef, passengerGUID uint64) {
	if s == nil || ref == nil {
		return
	}
	kit, effMap, effInstance := ref.resolve()
	if sess := s.findSessionByGUID(passengerGUID); sess != nil && sess.player != nil {
		if sess.player.VehicleGUID == ref.baseGUID {
			sess.exitVehicle(ctx)
		}
		return
	}
	motion := s.findCreatureMotion(effMap, effInstance, passengerGUID)
	if motion == nil || motion.VehicleGUID != ref.baseGUID {
		return
	}
	var seatID int8
	seatOK := false
	if kit != nil {
		seatID, _, _, seatOK = kit.RemovePassenger(passengerGUID)
	}
	// Vehicle::RemovePassenger (Vehicle.cpp:508-509): freeing a usable
	// seat restores the base's spell-click flag.
	s.syncVehicleBaseNPCFlag(effMap, effInstance, ref.baseGUID)
	s.motionMu.Lock()
	motion.VehicleGUID = 0
	motion.VehicleSeat = 0
	motion.Moving = false
	s.motionMu.Unlock()
	// Vehicle::RemovePassenger (Vehicle.cpp:518-529): the
	// transport leg runs here, after the seat frees and
	// before the OnRemovePassenger hooks below.
	s.inheritOrResetCreatureVehicleTransport(effMap, effInstance, motion, ref.baseGUID)
	if base := s.findCreatureMotion(effMap, effInstance, ref.baseGUID); base != nil && seatOK {
		// Vehicle::RemovePassenger (Vehicle.cpp:536-539): the AI
		// PassengerBoarded(false) call precedes the OnRemovePassenger
		// script hook. A seat already freed fires nothing: C++
		// asserts the seat exists (Vehicle.cpp:501-504), and a
		// JUST_DIED accessory keeps a stale VehicleGUID after the
		// kit-drop sweep already fired ev6 for it.
		s.smartPassengerBoarded(ctx, base, passengerGUID, seatID, false)
		s.triggerVehicleEvent(scripting.VehicleEventOnRemovePassenger, s.luaMotionCreature(base), s.luaMotionCreature(motion))
	}
	// Unit::_ExitVehicle (Unit.cpp:13202-13210): the seat freed
	// above, the board root clears unless a MOD_ROOT aura or the
	// movement template holds it, and exactly one
	// SMSG_SPLINE_MOVE_UNROOT goes out — via SetRooted for a
	// non-stunned passenger, via the _ExitVehicle arm when stunned.
	// The eject path shares this body
	// (ejectVehicleSeatOccupant delegates here), so the install
	// eject loop gets the leg too.
	s.unrootExitingCreaturePassenger(motion)
}
